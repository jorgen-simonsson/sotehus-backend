package storage

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/jorgen-simonsson/sotehus-backend/internal/config"
	"github.com/shopspring/decimal"
)

// ErrNoData is returned when a query finds no matching records.
var ErrNoData = errors.New("no data found")

// InfluxDBClient handles writing time series data to InfluxDB
type InfluxDBClient struct {
	client   influxdb2.Client
	writeAPI api.WriteAPIBlocking
	org      string
	bucket   string
	logger   *slog.Logger
}

// NewInfluxDBClient creates a new InfluxDB client
func NewInfluxDBClient(cfg *config.Config, logger *slog.Logger) (*InfluxDBClient, error) {
	if cfg.InfluxDBHost == "" {
		return nil, fmt.Errorf("InfluxDB host is required")
	}

	url := fmt.Sprintf("http://%s:%d", cfg.InfluxDBHost, cfg.InfluxDBPort)

	// Use token if provided, otherwise try username:password format
	// InfluxDB 2.x can use username:password as token for compatibility
	token := cfg.InfluxDBToken
	if token == "" && cfg.InfluxDBUser != "" && cfg.InfluxDBPassword != "" {
		// For InfluxDB 2.x with username/password, we need to use Basic auth
		// The Go client supports this by setting the token to empty and using
		// SetHTTPRequestMiddleware, or by generating a proper API token
		// Try using username:password as a basic auth token format
		token = cfg.InfluxDBUser + ":" + cfg.InfluxDBPassword
	}

	// Create client with options for basic auth fallback
	opts := influxdb2.DefaultOptions()
	opts.SetHTTPRequestTimeout(30)

	client := influxdb2.NewClientWithOptions(url, token, opts)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	health, err := client.Health(ctx)
	if err != nil {
		logger.Warn("InfluxDB health check failed", "error", err)
		// Continue anyway - InfluxDB might not be available yet
	} else {
		logger.Info("InfluxDB connected", "status", health.Status)
	}

	writeAPI := client.WriteAPIBlocking(cfg.InfluxDBOrg, cfg.InfluxDBBucket)

	return &InfluxDBClient{
		client:   client,
		writeAPI: writeAPI,
		org:      cfg.InfluxDBOrg,
		bucket:   cfg.InfluxDBBucket,
		logger:   logger,
	}, nil
}

// WriteGridPower writes grid power consumption to InfluxDB
// Uses "power_monitoring" measurement with "grid_power" field to match Python app
// If frequency is valid (> 0), it will also be included in the point
func (c *InfluxDBClient) WriteGridPower(power float64, frequency float64, timestamp time.Time) error {
	fields := map[string]interface{}{"grid_power": power}

	// Include frequency if valid (non-zero)
	if frequency > 0 {
		fields["grid_frequency"] = frequency
	}

	p := influxdb2.NewPoint(
		"power_monitoring",
		nil,
		fields,
		timestamp,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.writeAPI.WritePoint(ctx, p); err != nil {
		c.logger.Error("Failed to write grid power to InfluxDB", "error", err)
		return err
	}

	if frequency > 0 {
		c.logger.Debug("Wrote grid power and frequency to InfluxDB", "power", power, "frequency", frequency)
	} else {
		c.logger.Debug("Wrote grid power to InfluxDB", "power", power)
	}
	return nil
}

// WriteGridData writes multiple grid data fields to InfluxDB in a single point.
// Uses "power_monitoring" measurement with dynamic fields from the provided map.
// This enables aggregating multiple MQTT topic values into a single InfluxDB record.
func (c *InfluxDBClient) WriteGridData(data map[string]float64, timestamp time.Time) error {
	if len(data) == 0 {
		return nil
	}

	fields := make(map[string]interface{}, len(data))
	for k, v := range data {
		fields[k] = v
	}

	p := influxdb2.NewPoint(
		"power_monitoring",
		nil,
		fields,
		timestamp,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.writeAPI.WritePoint(ctx, p); err != nil {
		c.logger.Error("Failed to write grid data to InfluxDB", "error", err, "fields", len(data))
		return err
	}

	c.logger.Debug("Wrote grid data to InfluxDB", "fields", len(data))
	return nil
}

// EnsureBucketExists creates the given bucket in the client's org if it does not already exist.
func (c *InfluxDBClient) EnsureBucketExists(bucketName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	bucketsAPI := c.client.BucketsAPI()

	if _, err := bucketsAPI.FindBucketByName(ctx, bucketName); err == nil {
		return nil
	}

	org, err := c.client.OrganizationsAPI().FindOrganizationByName(ctx, c.org)
	if err != nil {
		return fmt.Errorf("failed to find organization %q: %w", c.org, err)
	}

	if _, err := bucketsAPI.CreateBucketWithName(ctx, org, bucketName); err != nil {
		return fmt.Errorf("failed to create bucket %q: %w", bucketName, err)
	}

	c.logger.Info("Created InfluxDB bucket", "bucket", bucketName)
	return nil
}

// WriteToBucket writes a set of fields as a single point to the given bucket and measurement.
func (c *InfluxDBClient) WriteToBucket(bucket, measurement string, fields map[string]float64, timestamp time.Time) error {
	if len(fields) == 0 {
		return nil
	}

	pointFields := make(map[string]interface{}, len(fields))
	for k, v := range fields {
		pointFields[k] = v
	}

	p := influxdb2.NewPoint(measurement, nil, pointFields, timestamp)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	writeAPI := c.client.WriteAPIBlocking(c.org, bucket)
	if err := writeAPI.WritePoint(ctx, p); err != nil {
		c.logger.Error("Failed to write data to InfluxDB", "bucket", bucket, "error", err, "fields", len(fields))
		return err
	}

	c.logger.Debug("Wrote data to InfluxDB", "bucket", bucket, "fields", len(fields))
	return nil
}

// TimeSeriesStats holds statistics about the time series data
type TimeSeriesStats struct {
	First time.Time `json:"first"`
	Last  time.Time `json:"last"`
	Count int64     `json:"count"`
}

// GetTimeSeriesStats returns the first and last timestamps and total count of entries
func (c *InfluxDBClient) GetTimeSeriesStats() (*TimeSeriesStats, error) {
	queryAPI := c.client.QueryAPI(c.org)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	stats := &TimeSeriesStats{}

	// Query for first entry (oldest) - use power_monitoring measurement from Python app
	firstQuery := fmt.Sprintf(`
		from(bucket: "%s")
		|> range(start: 0)
		|> filter(fn: (r) => r._measurement == "power_monitoring")
		|> first()
		|> keep(columns: ["_time"])
	`, c.bucket)

	result, err := queryAPI.Query(ctx, firstQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query first entry: %w", err)
	}

	for result.Next() {
		stats.First = result.Record().Time()
	}
	if result.Err() != nil {
		return nil, fmt.Errorf("error reading first entry result: %w", result.Err())
	}

	// Query for last entry (newest)
	lastQuery := fmt.Sprintf(`
		from(bucket: "%s")
		|> range(start: 0)
		|> filter(fn: (r) => r._measurement == "power_monitoring")
		|> last()
		|> keep(columns: ["_time"])
	`, c.bucket)

	result, err = queryAPI.Query(ctx, lastQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query last entry: %w", err)
	}

	for result.Next() {
		stats.Last = result.Record().Time()
	}
	if result.Err() != nil {
		return nil, fmt.Errorf("error reading last entry result: %w", result.Err())
	}

	// Query for count
	countQuery := fmt.Sprintf(`
		from(bucket: "%s")
		|> range(start: 0)
		|> filter(fn: (r) => r._measurement == "power_monitoring")
		|> filter(fn: (r) => r._field == "grid_power")
		|> count()
	`, c.bucket)

	result, err = queryAPI.Query(ctx, countQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query count: %w", err)
	}

	for result.Next() {
		if v, ok := result.Record().Value().(int64); ok {
			stats.Count = v
		}
	}
	if result.Err() != nil {
		return nil, fmt.Errorf("error reading count result: %w", result.Err())
	}

	return stats, nil
}

// GetLastValueBefore returns the last value of a field at or before the given timestamp.
func (c *InfluxDBClient) GetLastValueBefore(fieldName string, targetTime time.Time) (float64, time.Time, error) {
	queryAPI := c.client.QueryAPI(c.org)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Query to find the last value at or before the target time
	// We use range from epoch start to just after target time, then get last
	query := fmt.Sprintf(`
		from(bucket: "%s")
		|> range(start: 0, stop: %s)
		|> filter(fn: (r) => r._measurement == "power_monitoring")
		|> filter(fn: (r) => r._field == "%s")
		|> last()
	`, c.bucket, targetTime.Add(time.Second).Format(time.RFC3339), fieldName)

	result, err := queryAPI.Query(ctx, query)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("failed to query last value before: %w", err)
	}

	var value float64
	var actualTime time.Time
	found := false

	for result.Next() {
		record := result.Record()
		if v, ok := record.Value().(float64); ok {
			value = v
			actualTime = record.Time()
			found = true
		}
	}
	if result.Err() != nil {
		return 0, time.Time{}, fmt.Errorf("error reading last value result: %w", result.Err())
	}

	if !found {
		return 0, time.Time{}, fmt.Errorf("no data found at or before %v", targetTime)
	}

	return value, actualTime, nil
}

// GetConsumedEnergy calculates the energy consumed between two timestamps.
// It finds the last grid_enery_consumed_ack value at or before start and stop,
// and returns the difference in kWh. Returns an error if no data exists
// at or before either timestamp.
func (c *InfluxDBClient) GetConsumedEnergy(start, stop time.Time) (float64, time.Time, time.Time, error) {
	startValue, startActual, err := c.GetLastValueBefore("grid_enery_consumed_ack", start)
	if err != nil {
		return 0, time.Time{}, time.Time{}, fmt.Errorf("no data found at or before start time %v", start)
	}

	stopValue, stopActual, err := c.GetLastValueBefore("grid_enery_consumed_ack", stop)
	if err != nil {
		return 0, time.Time{}, time.Time{}, fmt.Errorf("no data found at or before stop time %v", stop)
	}

	consumed := stopValue - startValue
	return consumed, startActual, stopActual, nil
}

// GetSoldEnergy calculates the energy sold between two timestamps.
// It finds the last grid_enery_sold_ack value at or before start and stop,
// and returns the difference in kWh. Returns an error if no data exists
// at or before either timestamp.
func (c *InfluxDBClient) GetSoldEnergy(start, stop time.Time) (float64, time.Time, time.Time, error) {
	startValue, startActual, err := c.GetLastValueBefore("grid_enery_sold_ack", start)
	if err != nil {
		return 0, time.Time{}, time.Time{}, fmt.Errorf("no data found at or before start time %v", start)
	}

	stopValue, stopActual, err := c.GetLastValueBefore("grid_enery_sold_ack", stop)
	if err != nil {
		return 0, time.Time{}, time.Time{}, fmt.Errorf("no data found at or before stop time %v", stop)
	}

	sold := stopValue - startValue
	return sold, startActual, stopActual, nil
}

// SolisRecord represents the fields of the most recent record written to the
// "solis" bucket by the Solis MQTT service.
type SolisRecord struct {
	Time         time.Time
	GridPower    float64 // "power.total" — positive = export to grid, negative = import
	SolarPower   float64 // "solar.power" — inverter AC output
	BatteryPower float64 // "battery.power" — positive = charging, negative = discharging
	BatterySOC   float64 // "battery.SOC" — percent, 0-100
}

// GetLastSolisRecord returns the most recent record from the "solis" bucket's
// "solis_inverter" measurement. Fields are combined via pivot since they are
// all written as part of the same point (same timestamp) by the Solis service.
func (c *InfluxDBClient) GetLastSolisRecord() (*SolisRecord, error) {
	queryAPI := c.client.QueryAPI(c.org)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := `
		from(bucket: "solis")
		|> range(start: 0)
		|> filter(fn: (r) => r._measurement == "solis_inverter")
		|> filter(fn: (r) => r._field == "power.total" or r._field == "solar.power" or r._field == "battery.power" or r._field == "battery.SOC")
		|> last()
		|> pivot(rowKey: ["_time"], columnKey: ["_field"], valueColumn: "_value")
	`

	result, err := queryAPI.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query last Solis record: %w", err)
	}

	var record *SolisRecord
	for result.Next() {
		r := result.Record()
		rec := &SolisRecord{Time: r.Time()}
		if v, ok := r.ValueByKey("power.total").(float64); ok {
			rec.GridPower = v
		}
		if v, ok := r.ValueByKey("solar.power").(float64); ok {
			rec.SolarPower = v
		}
		if v, ok := r.ValueByKey("battery.power").(float64); ok {
			rec.BatteryPower = v
		}
		if v, ok := r.ValueByKey("battery.SOC").(float64); ok {
			rec.BatterySOC = v
		}
		record = rec
	}
	if result.Err() != nil {
		return nil, fmt.Errorf("error reading last Solis record: %w", result.Err())
	}
	if record == nil {
		return nil, ErrNoData
	}

	return record, nil
}

// Close closes the InfluxDB client connection
func (c *InfluxDBClient) Close() {
	c.client.Close()
}

// TimeFieldRecord represents a single InfluxDB record with spot_price, grid_enery_consumed_ack and grid_enery_sold_ack fields.
type TimeFieldRecord struct {
	Time        time.Time       `json:"time"`
	SpotPrice   decimal.Decimal `json:"spot_price"`
	ConsumedAck decimal.Decimal `json:"consumed_ack"`
	SoldAck     decimal.Decimal `json:"sold_ack"`
}

// GetFieldsInRange returns time-ordered records with spot_price and grid_enery_consumed_ack
// for the given time range. Uses Flux pivot to combine both fields into single rows,
// fill(usePrevious: true) to carry forward the last known spot_price when missing,
// and aggregateWindow to downsample to hourly records for performance.
func (c *InfluxDBClient) GetFieldsInRange(start, stop time.Time) ([]TimeFieldRecord, error) {
	queryAPI := c.client.QueryAPI(c.org)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Downsample to 1h windows first (reducing ~500k rows to ~720 per field),
	// then fill forward missing spot_price on the small result set, pivot and sort.
	// Spot prices change at hour boundaries so 1h aggregation preserves full accuracy.
	query := fmt.Sprintf(`
		from(bucket: "%s")
		|> range(start: %s, stop: %s)
		|> filter(fn: (r) => r._measurement == "power_monitoring")
		|> filter(fn: (r) => r._field == "spot_price" or r._field == "grid_enery_consumed_ack" or r._field == "grid_enery_sold_ack")
		|> aggregateWindow(every: 1h, fn: last, createEmpty: false)
		|> fill(usePrevious: true)
		|> pivot(rowKey: ["_time"], columnKey: ["_field"], valueColumn: "_value")
		|> filter(fn: (r) => exists r.grid_enery_consumed_ack and exists r.spot_price)
		|> sort(columns: ["_time"])
	`, c.bucket, start.Format(time.RFC3339), stop.Format(time.RFC3339))

	result, err := queryAPI.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query fields in range: %w", err)
	}

	var records []TimeFieldRecord
	for result.Next() {
		record := result.Record()

		var spotPrice, consumedAck, soldAck decimal.Decimal

		if v, ok := record.ValueByKey("spot_price").(float64); ok {
			spotPrice = decimal.NewFromFloat(v)
		}
		if v, ok := record.ValueByKey("grid_enery_consumed_ack").(float64); ok {
			consumedAck = decimal.NewFromFloat(v)
		}
		if v, ok := record.ValueByKey("grid_enery_sold_ack").(float64); ok {
			soldAck = decimal.NewFromFloat(v)
		}

		records = append(records, TimeFieldRecord{
			Time:        record.Time(),
			SpotPrice:   spotPrice,
			ConsumedAck: consumedAck,
			SoldAck:     soldAck,
		})
	}
	if result.Err() != nil {
		return nil, fmt.Errorf("error reading fields in range result: %w", result.Err())
	}

	return records, nil
}
