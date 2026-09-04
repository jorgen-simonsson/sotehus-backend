package weather

import (
	"log/slog"
	"os"
	"testing"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/jorgen-simonsson/sotehus-backend/internal/config"
	"github.com/jorgen-simonsson/sotehus-backend/internal/state"
)

// fakeMessage implements mqtt.Message for exercising onMessage without a broker.
type fakeMessage struct {
	topic   string
	payload []byte
}

func (m *fakeMessage) Duplicate() bool   { return false }
func (m *fakeMessage) Qos() byte         { return 1 }
func (m *fakeMessage) Retained() bool    { return true }
func (m *fakeMessage) Topic() string     { return m.topic }
func (m *fakeMessage) MessageID() uint16 { return 0 }
func (m *fakeMessage) Payload() []byte   { return m.payload }
func (m *fakeMessage) Ack()              {}

func newTestService(t *testing.T) *Service {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := &config.Config{
		MQTTBrokerHost: "localhost",
		MQTTBrokerPort: 1883,
	}

	service, err := NewService(cfg, state.NewManager(), nil, logger)
	if err != nil {
		t.Fatalf("NewService() unexpected error: %v", err)
	}
	return service
}

func TestNewServiceMissingBrokerHost(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := &config.Config{
		MQTTBrokerHost: "",
		MQTTBrokerPort: 1883,
	}

	service, err := NewService(cfg, state.NewManager(), nil, logger)

	if err == nil {
		t.Error("Expected error for missing broker host")
	}
	if service != nil {
		t.Error("Expected nil service for missing broker host")
	}
	if err.Error() != "MQTT broker host is required" {
		t.Errorf("Error message = %q, want %q", err.Error(), "MQTT broker host is required")
	}
}

func TestNewServiceSuccess(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := &config.Config{
		MQTTBrokerHost: "localhost",
		MQTTBrokerPort: 1883,
	}

	service, err := NewService(cfg, state.NewManager(), nil, logger)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if service == nil {
		t.Fatal("Expected non-nil service")
	}
	if service.client == nil {
		t.Error("MQTT client should be created")
	}
	if service.logger != logger {
		t.Error("logger not set correctly")
	}
	if service.influxDB != nil {
		t.Error("influxDB should be nil when not provided")
	}
}

func TestNewServiceWithAuthentication(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := &config.Config{
		MQTTBrokerHost: "localhost",
		MQTTBrokerPort: 1883,
		MQTTUsername:   "user",
		MQTTPassword:   "pass",
	}

	service, err := NewService(cfg, state.NewManager(), nil, logger)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if service == nil {
		t.Fatal("Expected non-nil service")
	}
	if service.client == nil {
		t.Error("MQTT client should be created")
	}
}

func TestConstants(t *testing.T) {
	if mqttTopic != "weather/gw1200/json" {
		t.Errorf("mqttTopic = %q, want %q", mqttTopic, "weather/gw1200/json")
	}
	if influxBucket != "weather" {
		t.Errorf("influxBucket = %q, want %q", influxBucket, "weather")
	}
	if influxMeasurement != "weather_station" {
		t.Errorf("influxMeasurement = %q, want %q", influxMeasurement, "weather_station")
	}
}

// TestOnMessageWithoutInfluxDB verifies onMessage parses payloads and updates
// state without panicking when no InfluxDB client is configured (the state
// the service is in whenever InfluxDB is disabled at startup).
func TestOnMessageWithoutInfluxDB(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{
			name: "full sample payload from ref/main.go's buildReadings output",
			payload: `{
				"outdoor_temp": {"value": 21.5, "unitOfMeasure": "C"},
				"outdoor_humidity": {"value": 54, "unitOfMeasure": "%"},
				"wind_speed": {"value": 3.2, "unitOfMeasure": "m/s"},
				"solar_radiation": {"value": 512.3, "unitOfMeasure": "W/m2"}
			}`,
		},
		{
			name:    "partial payload (one cluster failed to read)",
			payload: `{"outdoor_temp": {"value": 21.5, "unitOfMeasure": "C"}}`,
		},
		{
			name:    "empty payload",
			payload: `{}`,
		},
		{
			name:    "invalid JSON",
			payload: `not json`,
		},
		{
			name:    "non-numeric field value",
			payload: `{"outdoor_temp": {"value": "not a number"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := newTestService(t)
			msg := &fakeMessage{topic: mqttTopic, payload: []byte(tt.payload)}

			// Must not panic regardless of payload shape.
			service.onMessage(nil, msg)
		})
	}
}

// TestOnMessageUpdatesState verifies a valid payload is stored in the state
// manager with both value and unit preserved.
func TestOnMessageUpdatesState(t *testing.T) {
	service := newTestService(t)
	msg := &fakeMessage{
		topic:   mqttTopic,
		payload: []byte(`{"outdoor_temp": {"value": 21.5, "unitOfMeasure": "C"}}`),
	}

	service.onMessage(nil, msg)

	data := service.state.GetWeatherData()
	if !data.Valid {
		t.Fatal("expected weather data to be marked valid after a message")
	}
	rd, ok := data.Readings["outdoor_temp"]
	if !ok {
		t.Fatal("expected outdoor_temp reading to be present")
	}
	if rd.Value != 21.5 || rd.UnitOfMeasure != "C" {
		t.Errorf("outdoor_temp = %+v, want {Value:21.5 UnitOfMeasure:C}", rd)
	}
}

func TestOnMessageNilInfluxDBIsSafe(t *testing.T) {
	service := newTestService(t)
	if service.influxDB != nil {
		t.Fatal("expected test service to have a nil InfluxDB client")
	}

	msg := &fakeMessage{
		topic:   mqttTopic,
		payload: []byte(`{"outdoor_temp": {"value": 21.5, "unitOfMeasure": "C"}}`),
	}

	// onMessage must check for a nil influxDB before attempting to write.
	service.onMessage(nil, msg)
}

// Verify fakeMessage satisfies the mqtt.Message interface at compile time.
var _ mqtt.Message = (*fakeMessage)(nil)
