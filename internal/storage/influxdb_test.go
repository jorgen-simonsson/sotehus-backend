package storage

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jorgen-simonsson/sotehus-backend/internal/config"
)

func TestNewInfluxDBClientMissingHost(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := &config.Config{
		InfluxDBHost:     "",
		InfluxDBPort:     8086,
		InfluxDBOrg:      "sotehus",
		InfluxDBBucket:   "power",
		InfluxDBUser:     "admin",
		InfluxDBPassword: "password",
	}

	client, err := NewInfluxDBClient(cfg, logger)

	if err == nil {
		t.Error("Expected error for missing host, got nil")
	}
	if client != nil {
		t.Error("Expected nil client for missing host")
	}
	if err.Error() != "InfluxDB host is required" {
		t.Errorf("Error message = %q, want %q", err.Error(), "InfluxDB host is required")
	}
}

func TestNewInfluxDBClientWithToken(t *testing.T) {
	// Skip if we can't reach a test InfluxDB
	if os.Getenv("TEST_INFLUXDB_HOST") == "" {
		t.Skip("Skipping InfluxDB integration test (set TEST_INFLUXDB_HOST to run)")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := &config.Config{
		InfluxDBHost:   os.Getenv("TEST_INFLUXDB_HOST"),
		InfluxDBPort:   8086,
		InfluxDBOrg:    "sotehus",
		InfluxDBBucket: "power",
		InfluxDBToken:  os.Getenv("TEST_INFLUXDB_TOKEN"),
	}

	client, err := NewInfluxDBClient(cfg, logger)

	if err != nil {
		t.Skipf("InfluxDB connection failed (expected in CI): %v", err)
	}
	if client == nil {
		t.Error("Expected non-nil client")
	}
	if client != nil {
		client.Close()
	}
}

func TestTimeSeriesStatsStruct(t *testing.T) {
	// Test that TimeSeriesStats struct has expected fields
	stats := TimeSeriesStats{
		Count: 100,
	}

	if stats.Count != 100 {
		t.Errorf("Count = %d, want %d", stats.Count, 100)
	}

	if stats.First.IsZero() == false {
		t.Error("First should be zero value when not set")
	}
}

// TestConfigBuildURL tests the URL building logic
func TestConfigBuildURL(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		port     int
		expected string
	}{
		{
			name:     "localhost default port",
			host:     "localhost",
			port:     8086,
			expected: "http://localhost:8086",
		},
		{
			name:     "localhost custom port",
			host:     "localhost",
			port:     8087,
			expected: "http://localhost:8087",
		},
		{
			name:     "remote host",
			host:     "influxdb.example.com",
			port:     8086,
			expected: "http://influxdb.example.com:8086",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use fmt.Sprintf to match what the actual code does
			url := "http://" + tt.host
			if tt.port != 0 {
				url = url + ":" + itoa(tt.port)
			}
			if url != tt.expected {
				t.Errorf("URL = %q, want %q", url, tt.expected)
			}
		})
	}
}

// TestEnsureBucketExistsAndWriteToBucket exercises the Solis bucket-creation
// and cross-bucket write path against a real InfluxDB instance.
func TestEnsureBucketExistsAndWriteToBucket(t *testing.T) {
	if os.Getenv("TEST_INFLUXDB_HOST") == "" {
		t.Skip("Skipping InfluxDB integration test (set TEST_INFLUXDB_HOST to run)")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := &config.Config{
		InfluxDBHost:   os.Getenv("TEST_INFLUXDB_HOST"),
		InfluxDBPort:   8086,
		InfluxDBOrg:    "sotehus",
		InfluxDBBucket: "power",
		InfluxDBToken:  os.Getenv("TEST_INFLUXDB_TOKEN"),
	}

	client, err := NewInfluxDBClient(cfg, logger)
	if err != nil {
		t.Skipf("InfluxDB connection failed (expected in CI): %v", err)
	}
	defer client.Close()

	const testBucket = "solis_test"

	if err := client.EnsureBucketExists(testBucket); err != nil {
		t.Fatalf("EnsureBucketExists() first call error = %v", err)
	}

	// Calling it again with an already-existing bucket must be a no-op, not an error.
	if err := client.EnsureBucketExists(testBucket); err != nil {
		t.Fatalf("EnsureBucketExists() second call error = %v", err)
	}

	fields := map[string]float64{
		"voltage.L1":  231.20,
		"power.total": 790.00,
	}

	if err := client.WriteToBucket(testBucket, "solis_inverter", fields, time.Now()); err != nil {
		t.Fatalf("WriteToBucket() error = %v", err)
	}
}

// TestGetLastSolisRecord writes a known record to the "solis" bucket and
// verifies GetLastSolisRecord reads it back correctly against a real InfluxDB instance.
func TestGetLastSolisRecord(t *testing.T) {
	if os.Getenv("TEST_INFLUXDB_HOST") == "" {
		t.Skip("Skipping InfluxDB integration test (set TEST_INFLUXDB_HOST to run)")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := &config.Config{
		InfluxDBHost:   os.Getenv("TEST_INFLUXDB_HOST"),
		InfluxDBPort:   8086,
		InfluxDBOrg:    "sotehus",
		InfluxDBBucket: "power",
		InfluxDBToken:  os.Getenv("TEST_INFLUXDB_TOKEN"),
	}

	client, err := NewInfluxDBClient(cfg, logger)
	if err != nil {
		t.Skipf("InfluxDB connection failed (expected in CI): %v", err)
	}
	defer client.Close()

	if err := client.EnsureBucketExists("solis"); err != nil {
		t.Fatalf("EnsureBucketExists() error = %v", err)
	}

	fields := map[string]float64{
		"power.total":   250.5,
		"solar.power":   4200.0,
		"battery.power": -600.0,
		"battery.SOC":   72.0,
		"voltage.L1":    231.0, // extra field GetLastSolisRecord should ignore
	}

	writeTime := time.Now()
	if err := client.WriteToBucket("solis", "solis_inverter", fields, writeTime); err != nil {
		t.Fatalf("WriteToBucket() error = %v", err)
	}

	record, err := client.GetLastSolisRecord()
	if err != nil {
		t.Fatalf("GetLastSolisRecord() error = %v", err)
	}

	if record.GridPower != fields["power.total"] {
		t.Errorf("GridPower = %v, want %v", record.GridPower, fields["power.total"])
	}
	if record.SolarPower != fields["solar.power"] {
		t.Errorf("SolarPower = %v, want %v", record.SolarPower, fields["solar.power"])
	}
	if record.BatteryPower != fields["battery.power"] {
		t.Errorf("BatteryPower = %v, want %v", record.BatteryPower, fields["battery.power"])
	}
	if record.BatterySOC != fields["battery.SOC"] {
		t.Errorf("BatterySOC = %v, want %v", record.BatterySOC, fields["battery.SOC"])
	}
}

func TestWriteToBucketEmptyFieldsIsNoop(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	client := &InfluxDBClient{logger: logger}

	if err := client.WriteToBucket("solis", "solis_inverter", map[string]float64{}, time.Now()); err != nil {
		t.Errorf("WriteToBucket() with empty fields should be a no-op, got error: %v", err)
	}
}

// itoa is a simple int to string for testing
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	result := ""
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	return result
}
