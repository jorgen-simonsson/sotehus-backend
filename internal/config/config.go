package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all configuration values for the application
type Config struct {
	// Server configuration
	ServerPort string

	// MQTT configuration
	MQTTBrokerHost string
	MQTTBrokerPort int
	MQTTUsername   string
	MQTTPassword   string
	MQTTTopic      string

	// InfluxDB configuration
	InfluxDBHost     string
	InfluxDBPort     int
	InfluxDBUser     string
	InfluxDBPassword string
	InfluxDBOrg      string
	InfluxDBBucket   string
	InfluxDBToken    string

	// SolarEdge configuration
	SolarEdgeAPIKey string
	SolarEdgeSiteID string

	// Spot price configuration
	SpotPriceRegion string
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	// Load .env file if it exists (ignore error if not found)
	_ = godotenv.Load()

	cfg := &Config{
		// Server defaults
		ServerPort: getEnvOrDefault("SERVER_PORT", "8080"),

		// MQTT
		MQTTBrokerHost: os.Getenv("MQTT_BROKER_HOST"),
		MQTTBrokerPort: getEnvAsInt("MQTT_BROKER_PORT", 1883),
		MQTTUsername:   os.Getenv("MQTT_USERNAME"),
		MQTTPassword:   os.Getenv("MQTT_PASSWORD"),
		MQTTTopic:      os.Getenv("MQTT_TOPIC"),

		// InfluxDB
		InfluxDBHost:     os.Getenv("INFLUXDB2_HOST"),
		InfluxDBPort:     getEnvAsInt("INFLUXDB2_PORT", 8086),
		InfluxDBUser:     os.Getenv("INFLUXDB2_USER"),
		InfluxDBPassword: os.Getenv("INFLUXDB2_PASSWORD"),
		InfluxDBOrg:      getEnvOrDefault("INFLUXDB2_ORG", "sotehus"),
		InfluxDBBucket:   getEnvOrDefault("INFLUXDB2_BUCKET", "sotehus_bucket"),
		InfluxDBToken:    os.Getenv("INFLUXDB2_TOKEN"),

		// SolarEdge
		SolarEdgeAPIKey: os.Getenv("SOLAREDGE_API_KEY"),
		SolarEdgeSiteID: os.Getenv("SOLAREDGE_SITE_ID"),

		// Spot price
		SpotPriceRegion: getEnvOrDefault("SPOTPRICE_REGION", "SE4"),
	}

	return cfg, nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
