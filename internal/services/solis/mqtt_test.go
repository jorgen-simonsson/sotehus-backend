package solis

import (
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/jorgen-simonsson/sotehus-backend/internal/config"
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

	service, err := NewService(cfg, nil, logger)
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

	service, err := NewService(cfg, nil, logger)

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

	service, err := NewService(cfg, nil, logger)

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

	service, err := NewService(cfg, nil, logger)

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
	if mqttTopic != "solis/modbus" {
		t.Errorf("mqttTopic = %q, want %q", mqttTopic, "solis/modbus")
	}
	if influxBucket != "solis" {
		t.Errorf("influxBucket = %q, want %q", influxBucket, "solis")
	}
	if influxMeasurement != "solis_inverter" {
		t.Errorf("influxMeasurement = %q, want %q", influxMeasurement, "solis_inverter")
	}
}

// TestOnMessageWithoutInfluxDB verifies onMessage parses payloads and returns
// without panicking when no InfluxDB client is configured (the state the
// service is in whenever InfluxDB is disabled at startup).
func TestOnMessageWithoutInfluxDB(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{
			name: "full sample payload from SOLIS_MQTT_PAYLOAD.md",
			payload: `{
				"voltage.L1": 231.20,
				"current.L1": 2.34,
				"voltage.L2": 230.80,
				"current.L2": 1.87,
				"voltage.L3": 229.90,
				"current.L3": 2.01,
				"power.L1": 512.00,
				"power.L2": 398.00,
				"power.L3": -120.00,
				"power.total": 790.00,
				"meter.type": 258.00,
				"battery.SOC": 85.00,
				"battery.power": -430.00,
				"solar.power": 3120.00
			}`,
		},
		{
			name:    "partial payload (one cluster failed to read)",
			payload: `{"voltage.L1": 231.20, "current.L1": 2.34}`,
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
			payload: `{"voltage.L1": "not a number"}`,
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

// TestSamplePayloadParsing verifies the exact sample payload documented in
// SOLIS_MQTT_PAYLOAD.md decodes into the flat field map onMessage relies on,
// with every dot-containing key preserved literally (not nested) and no
// precision lost.
func TestSamplePayloadParsing(t *testing.T) {
	sample := `{
		"voltage.L1": 231.20,
		"current.L1": 2.34,
		"voltage.L2": 230.80,
		"current.L2": 1.87,
		"voltage.L3": 229.90,
		"current.L3": 2.01,
		"power.L1": 512.00,
		"power.L2": 398.00,
		"power.L3": -120.00,
		"power.total": 790.00,
		"meter.type": 258.00,
		"battery.SOC": 85.00,
		"battery.power": -430.00,
		"solar.power": 3120.00
	}`

	var payload map[string]float64
	if err := json.Unmarshal([]byte(sample), &payload); err != nil {
		t.Fatalf("failed to unmarshal sample payload: %v", err)
	}

	want := map[string]float64{
		"voltage.L1":    231.20,
		"current.L1":    2.34,
		"voltage.L2":    230.80,
		"current.L2":    1.87,
		"voltage.L3":    229.90,
		"current.L3":    2.01,
		"power.L1":      512.00,
		"power.L2":      398.00,
		"power.L3":      -120.00,
		"power.total":   790.00,
		"meter.type":    258.00,
		"battery.SOC":   85.00,
		"battery.power": -430.00,
		"solar.power":   3120.00,
	}

	if len(payload) != len(want) {
		t.Fatalf("got %d fields, want %d", len(payload), len(want))
	}
	for k, wantV := range want {
		gotV, ok := payload[k]
		if !ok {
			t.Errorf("missing field %q", k)
			continue
		}
		if gotV != wantV {
			t.Errorf("field %q = %v, want %v", k, gotV, wantV)
		}
	}
}

func TestOnMessageNilInfluxDBIsSafe(t *testing.T) {
	service := newTestService(t)
	if service.influxDB != nil {
		t.Fatal("expected test service to have a nil InfluxDB client")
	}

	msg := &fakeMessage{
		topic:   mqttTopic,
		payload: []byte(`{"power.total": 790.00}`),
	}

	// onMessage must check for a nil influxDB before attempting to write.
	service.onMessage(nil, msg)
}

// Verify fakeMessage satisfies the mqtt.Message interface at compile time.
var _ mqtt.Message = (*fakeMessage)(nil)
