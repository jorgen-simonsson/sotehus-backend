package grid

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jorgen-simonsson/sotehus-backend/internal/config"
	"github.com/jorgen-simonsson/sotehus-backend/internal/state"
)

func TestNewServiceMissingBrokerHost(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()

	cfg := &config.Config{
		MQTTBrokerHost: "",
		MQTTBrokerPort: 1883,
		MQTTTopic:      "test/topic",
	}

	service, err := NewService(cfg, mgr, nil, logger)

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

func TestNewServiceMissingTopic(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()

	cfg := &config.Config{
		MQTTBrokerHost: "localhost",
		MQTTBrokerPort: 1883,
		MQTTTopic:      "",
	}

	service, err := NewService(cfg, mgr, nil, logger)

	if err == nil {
		t.Error("Expected error for missing topic")
	}
	if service != nil {
		t.Error("Expected nil service for missing topic")
	}
	if err.Error() != "MQTT topic is required" {
		t.Errorf("Error message = %q, want %q", err.Error(), "MQTT topic is required")
	}
}

func TestNewServiceSuccess(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()

	cfg := &config.Config{
		MQTTBrokerHost: "localhost",
		MQTTBrokerPort: 1883,
		MQTTTopic:      "test/topic",
	}

	service, err := NewService(cfg, mgr, nil, logger)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if service == nil {
		t.Fatal("Expected non-nil service")
	}
	if service.topic != "test/topic" {
		t.Errorf("topic = %q, want %q", service.topic, "test/topic")
	}
	if service.state != mgr {
		t.Error("state not set correctly")
	}
}

func TestNewServiceWithAuthentication(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()

	cfg := &config.Config{
		MQTTBrokerHost: "localhost",
		MQTTBrokerPort: 1883,
		MQTTTopic:      "test/topic",
		MQTTUsername:   "user",
		MQTTPassword:   "pass",
	}

	service, err := NewService(cfg, mgr, nil, logger)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if service == nil {
		t.Fatal("Expected non-nil service")
	}
	// Client should be created with credentials
	if service.client == nil {
		t.Error("MQTT client should be created")
	}
}

func TestDataTimeoutConstant(t *testing.T) {
	// Verify data timeout is 1 minute
	if dataTimeout != 1*time.Minute {
		t.Errorf("dataTimeout = %v, want %v", dataTimeout, 1*time.Minute)
	}
}

func TestLastMessageTimeMutex(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()

	cfg := &config.Config{
		MQTTBrokerHost: "localhost",
		MQTTBrokerPort: 1883,
		MQTTTopic:      "test/topic",
	}

	service, _ := NewService(cfg, mgr, nil, logger)

	// Test thread-safe access to lastMessageTime
	done := make(chan bool)

	// Write goroutine
	go func() {
		for i := 0; i < 100; i++ {
			service.lastMessageMu.Lock()
			service.lastMessageTime = time.Now()
			service.lastMessageMu.Unlock()
		}
		done <- true
	}()

	// Read goroutine
	go func() {
		for i := 0; i < 100; i++ {
			service.lastMessageMu.RLock()
			_ = service.lastMessageTime
			service.lastMessageMu.RUnlock()
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done
}

func TestServiceFields(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()

	cfg := &config.Config{
		MQTTBrokerHost: "mqtt.example.com",
		MQTTBrokerPort: 8883,
		MQTTTopic:      "home/power/grid",
	}

	service, err := NewService(cfg, mgr, nil, logger)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if service.topic != "home/power/grid" {
		t.Errorf("topic = %q, want %q", service.topic, "home/power/grid")
	}
	if service.logger != logger {
		t.Error("logger not set correctly")
	}
	if service.influxDB != nil {
		t.Error("influxDB should be nil when not provided")
	}
}
