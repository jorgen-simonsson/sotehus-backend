package config

import (
	"os"
	"reflect"
	"testing"
)

func TestLoad(t *testing.T) {
	// Clear environment before test
	clearEnv()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	// Check defaults
	if cfg.ServerPort != "8080" {
		t.Errorf("ServerPort = %q, want %q", cfg.ServerPort, "8080")
	}
	if cfg.MQTTBrokerPort != 1883 {
		t.Errorf("MQTTBrokerPort = %d, want %d", cfg.MQTTBrokerPort, 1883)
	}
	if cfg.InfluxDBPort != 8086 {
		t.Errorf("InfluxDBPort = %d, want %d", cfg.InfluxDBPort, 8086)
	}
	if cfg.InfluxDBOrg != "sotehus" {
		t.Errorf("InfluxDBOrg = %q, want %q", cfg.InfluxDBOrg, "sotehus")
	}
	if cfg.InfluxDBBucket != "sotehus_bucket" {
		t.Errorf("InfluxDBBucket = %q, want %q", cfg.InfluxDBBucket, "sotehus_bucket")
	}
	if cfg.SpotPriceRegion != "SE4" {
		t.Errorf("SpotPriceRegion = %q, want %q", cfg.SpotPriceRegion, "SE4")
	}
}

func TestLoadWithEnvVars(t *testing.T) {
	// Clear and set environment
	clearEnv()
	t.Setenv("SERVER_PORT", "9090")
	t.Setenv("MQTT_BROKER_HOST", "mqtt.example.com")
	t.Setenv("MQTT_BROKER_PORT", "1884")
	t.Setenv("MQTT_USERNAME", "testuser")
	t.Setenv("MQTT_PASSWORD", "testpass")
	t.Setenv("MQTT_TOPIC", "test/topic")
	t.Setenv("INFLUXDB2_HOST", "influx.example.com")
	t.Setenv("INFLUXDB2_PORT", "8087")
	t.Setenv("INFLUXDB2_USER", "admin")
	t.Setenv("INFLUXDB2_PASSWORD", "secret")
	t.Setenv("INFLUXDB2_ORG", "myorg")
	t.Setenv("INFLUXDB2_BUCKET", "mybucket")
	t.Setenv("INFLUXDB2_TOKEN", "mytoken")
	t.Setenv("SOLAREDGE_API_KEY", "apikey123")
	t.Setenv("SOLAREDGE_SITE_ID", "site456")
	t.Setenv("SPOTPRICE_REGION", "SE3")

	defer clearEnv()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	tests := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"ServerPort", cfg.ServerPort, "9090"},
		{"MQTTBrokerHost", cfg.MQTTBrokerHost, "mqtt.example.com"},
		{"MQTTBrokerPort", cfg.MQTTBrokerPort, 1884},
		{"MQTTUsername", cfg.MQTTUsername, "testuser"},
		{"MQTTPassword", cfg.MQTTPassword, "testpass"},
		{"InfluxDBHost", cfg.InfluxDBHost, "influx.example.com"},
		{"InfluxDBPort", cfg.InfluxDBPort, 8087},
		{"InfluxDBUser", cfg.InfluxDBUser, "admin"},
		{"InfluxDBPassword", cfg.InfluxDBPassword, "secret"},
		{"InfluxDBOrg", cfg.InfluxDBOrg, "myorg"},
		{"InfluxDBBucket", cfg.InfluxDBBucket, "mybucket"},
		{"InfluxDBToken", cfg.InfluxDBToken, "mytoken"},
		{"SolarEdgeAPIKey", cfg.SolarEdgeAPIKey, "apikey123"},
		{"SolarEdgeSiteID", cfg.SolarEdgeSiteID, "site456"},
		{"SpotPriceRegion", cfg.SpotPriceRegion, "SE3"},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
		}
	}

	// Test MQTTTopics separately since it's a slice
	expectedTopics := []string{"test/topic"}
	if !reflect.DeepEqual(cfg.MQTTTopics, expectedTopics) {
		t.Errorf("MQTTTopics = %v, want %v", cfg.MQTTTopics, expectedTopics)
	}
}

func TestGetEnvOrDefault(t *testing.T) {
	clearEnv()

	// Test with missing env var
	result := getEnvOrDefault("NONEXISTENT_VAR", "default")
	if result != "default" {
		t.Errorf("getEnvOrDefault() = %q, want %q", result, "default")
	}

	// Test with existing env var
	t.Setenv("TEST_VAR", "value")

	result = getEnvOrDefault("TEST_VAR", "default")
	if result != "value" {
		t.Errorf("getEnvOrDefault() = %q, want %q", result, "value")
	}

	// Test with empty env var
	t.Setenv("EMPTY_VAR", "")

	result = getEnvOrDefault("EMPTY_VAR", "default")
	if result != "default" {
		t.Errorf("getEnvOrDefault() with empty var = %q, want %q", result, "default")
	}
}

func TestGetEnvAsInt(t *testing.T) {
	clearEnv()

	// Test with missing env var
	result := getEnvAsInt("NONEXISTENT_INT", 42)
	if result != 42 {
		t.Errorf("getEnvAsInt() = %d, want %d", result, 42)
	}

	// Test with valid int
	t.Setenv("TEST_INT", "123")

	result = getEnvAsInt("TEST_INT", 42)
	if result != 123 {
		t.Errorf("getEnvAsInt() = %d, want %d", result, 123)
	}

	// Test with invalid int
	t.Setenv("INVALID_INT", "not_a_number")

	result = getEnvAsInt("INVALID_INT", 42)
	if result != 42 {
		t.Errorf("getEnvAsInt() with invalid int = %d, want %d", result, 42)
	}
}

func clearEnv() {
	vars := []string{
		"SERVER_PORT",
		"MQTT_BROKER_HOST", "MQTT_BROKER_PORT", "MQTT_USERNAME", "MQTT_PASSWORD", "MQTT_TOPIC",
		"INFLUXDB2_HOST", "INFLUXDB2_PORT", "INFLUXDB2_USER", "INFLUXDB2_PASSWORD",
		"INFLUXDB2_ORG", "INFLUXDB2_BUCKET", "INFLUXDB2_TOKEN",
		"SOLAREDGE_API_KEY", "SOLAREDGE_SITE_ID",
		"SPOTPRICE_REGION",
	}
	for _, v := range vars {
		os.Unsetenv(v)
	}
}
