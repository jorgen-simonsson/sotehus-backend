package storage

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/jorgen-simonsson/sotehus-backend/internal/config"
)

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

// WriteSpotPrice writes spot price to InfluxDB
// Uses "power_monitoring" measurement with "spot_price" field to match Python app
func (c *InfluxDBClient) WriteSpotPrice(price float64, timestamp time.Time) error {
	p := influxdb2.NewPoint(
		"power_monitoring",
		nil,
		map[string]interface{}{"spot_price": price},
		timestamp,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.writeAPI.WritePoint(ctx, p); err != nil {
		c.logger.Error("Failed to write spot price to InfluxDB", "error", err)
		return err
	}

	c.logger.Debug("Wrote spot price to InfluxDB", "price", price)
	return nil
}

// WriteSolarPower writes solar power production to InfluxDB
// Uses "power_monitoring" measurement with "solar_production" field to match Python app
func (c *InfluxDBClient) WriteSolarPower(power float64, timestamp time.Time) error {
	p := influxdb2.NewPoint(
		"power_monitoring",
		nil,
		map[string]interface{}{"solar_production": power},
		timestamp,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.writeAPI.WritePoint(ctx, p); err != nil {
		c.logger.Error("Failed to write solar power to InfluxDB", "error", err)
		return err
	}

	c.logger.Debug("Wrote solar power to InfluxDB", "power", power)
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

// Close closes the InfluxDB client connection
func (c *InfluxDBClient) Close() {
	c.client.Close()
}
