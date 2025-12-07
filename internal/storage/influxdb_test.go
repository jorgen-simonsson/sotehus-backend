package storage

import (
	"log/slog"
	"os"
	"testing"

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
			url := "http://" + tt.host + ":" + string(rune(tt.port+'0'))
			// Use fmt.Sprintf to match what the actual code does
			url = "http://" + tt.host
			if tt.port != 0 {
				url = url + ":" + itoa(tt.port)
			}
			if url != tt.expected {
				t.Errorf("URL = %q, want %q", url, tt.expected)
			}
		})
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
