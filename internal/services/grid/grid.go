package grid

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/jorgen-simonsson/sotehus-backend/internal/config"
	"github.com/jorgen-simonsson/sotehus-backend/internal/state"
	"github.com/jorgen-simonsson/sotehus-backend/internal/storage"
)

const (
	dataTimeout = 1 * time.Minute // Mark data as invalid if no message received within this duration
)

// Service handles grid power consumption via MQTT
type Service struct {
	client          mqtt.Client
	topic           string
	state           *state.Manager
	influxDB        *storage.InfluxDBClient
	logger          *slog.Logger
	lastMessageTime time.Time
	lastMessageMu   sync.RWMutex
}

// NewService creates a new grid service
func NewService(cfg *config.Config, state *state.Manager, influxDB *storage.InfluxDBClient, logger *slog.Logger) (*Service, error) {
	if cfg.MQTTBrokerHost == "" {
		return nil, fmt.Errorf("MQTT broker host is required")
	}
	if cfg.MQTTTopic == "" {
		return nil, fmt.Errorf("MQTT topic is required")
	}

	s := &Service{
		topic:    cfg.MQTTTopic,
		state:    state,
		influxDB: influxDB,
		logger:   logger,
	}

	opts := mqtt.NewClientOptions().
		AddBroker(fmt.Sprintf("tcp://%s:%d", cfg.MQTTBrokerHost, cfg.MQTTBrokerPort)).
		SetClientID("sotehus-backend-grid").
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetOnConnectHandler(s.onConnect).
		SetConnectionLostHandler(s.onConnectionLost)

	if cfg.MQTTUsername != "" && cfg.MQTTPassword != "" {
		opts.SetUsername(cfg.MQTTUsername)
		opts.SetPassword(cfg.MQTTPassword)
	}

	s.client = mqtt.NewClient(opts)

	return s, nil
}

// Start connects to MQTT and starts receiving grid power data
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Connecting to MQTT broker...")

	token := s.client.Connect()
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to connect to MQTT broker: %w", token.Error())
	}

	// Start timeout checker goroutine
	go s.checkDataTimeout(ctx)

	// Wait for context cancellation
	<-ctx.Done()
	s.logger.Info("Grid service shutting down...")

	s.client.Disconnect(1000)
	return nil
}

// checkDataTimeout periodically checks if data has timed out
func (s *Service) checkDataTimeout(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.lastMessageMu.RLock()
			lastTime := s.lastMessageTime
			s.lastMessageMu.RUnlock()

			// Only check if we've received at least one message
			if !lastTime.IsZero() && time.Since(lastTime) > dataTimeout {
				s.logger.Warn("No MQTT data received for over 1 minute")
				s.state.UpdateGridError("No current data for grid consumption")
			}
		}
	}
}

func (s *Service) onConnect(client mqtt.Client) {
	s.logger.Info("Connected to MQTT broker, subscribing to topic", "topic", s.topic)

	token := client.Subscribe(s.topic, 1, s.onMessage)
	if token.Wait() && token.Error() != nil {
		s.logger.Error("Failed to subscribe to topic", "topic", s.topic, "error", token.Error())
		s.state.UpdateGridError("Failed to subscribe to MQTT topic")
		return
	}

	s.logger.Info("Subscribed to MQTT topic", "topic", s.topic)
}

func (s *Service) onConnectionLost(client mqtt.Client, err error) {
	s.logger.Warn("MQTT connection lost", "error", err)
	s.state.UpdateGridError("MQTT connection lost")
}

func (s *Service) onMessage(client mqtt.Client, msg mqtt.Message) {
	payload := msg.Payload()

	// Try to parse as JSON first
	var power float64

	// Try JSON format: {"power": 123.45}
	var jsonPayload struct {
		Power float64 `json:"power"`
	}
	if err := json.Unmarshal(payload, &jsonPayload); err == nil {
		power = jsonPayload.Power
	} else {
		// Try plain number format
		if _, err := fmt.Sscanf(string(payload), "%f", &power); err != nil {
			s.logger.Warn("Failed to parse MQTT payload", "payload", string(payload), "error", err)
			return
		}
	}

	s.logger.Debug("Received grid power", "power", power)

	// Use same timestamp for state update and InfluxDB write
	timestamp := time.Now()

	// Update last message time
	s.lastMessageMu.Lock()
	s.lastMessageTime = timestamp
	s.lastMessageMu.Unlock()

	// Update state
	s.state.UpdateGrid(power, timestamp)

	// Write to InfluxDB
	if s.influxDB != nil {
		if err := s.influxDB.WriteGridPower(power, timestamp); err != nil {
			s.logger.Warn("Failed to write grid power to InfluxDB", "error", err)
		}
	}
}
