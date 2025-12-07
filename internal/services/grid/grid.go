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
	dataTimeout           = 1 * time.Minute  // Mark data as invalid if no message received within this duration
	initialReconnectDelay = 1 * time.Second  // Initial delay before reconnect attempt
	maxReconnectDelay     = 2 * time.Minute  // Maximum delay between reconnect attempts
	keepAliveInterval     = 30 * time.Second // MQTT keep-alive ping interval
	pingTimeout           = 10 * time.Second // Timeout for keep-alive ping response
	connectTimeout        = 30 * time.Second // Timeout for initial connection
	maxReconnectAttempts  = 0                // 0 = unlimited reconnect attempts
)

// Service handles grid power consumption via MQTT
type Service struct {
	client          mqtt.Client
	topic           string
	brokerURL       string
	state           *state.Manager
	influxDB        *storage.InfluxDBClient
	logger          *slog.Logger
	lastMessageTime time.Time
	lastMessageMu   sync.RWMutex
	reconnectCount  int
	reconnectMu     sync.Mutex
}

// NewService creates a new grid service
func NewService(cfg *config.Config, state *state.Manager, influxDB *storage.InfluxDBClient, logger *slog.Logger) (*Service, error) {
	if cfg.MQTTBrokerHost == "" {
		return nil, fmt.Errorf("MQTT broker host is required")
	}
	if cfg.MQTTTopic == "" {
		return nil, fmt.Errorf("MQTT topic is required")
	}

	brokerURL := fmt.Sprintf("tcp://%s:%d", cfg.MQTTBrokerHost, cfg.MQTTBrokerPort)

	s := &Service{
		topic:     cfg.MQTTTopic,
		brokerURL: brokerURL,
		state:     state,
		influxDB:  influxDB,
		logger:    logger,
	}

	opts := mqtt.NewClientOptions().
		AddBroker(brokerURL).
		SetClientID(fmt.Sprintf("sotehus-backend-grid-%d", time.Now().UnixNano())).
		// Reconnection settings
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(initialReconnectDelay).
		SetMaxReconnectInterval(maxReconnectDelay).
		// Keep-alive settings
		SetKeepAlive(keepAliveInterval).
		SetPingTimeout(pingTimeout).
		SetConnectTimeout(connectTimeout).
		// Clean session - start fresh on each connection
		SetCleanSession(true).
		// Buffer settings for offline messages
		SetWriteTimeout(10 * time.Second).
		SetOrderMatters(false).
		// Handlers
		SetOnConnectHandler(s.onConnect).
		SetConnectionLostHandler(s.onConnectionLost).
		SetReconnectingHandler(s.onReconnecting)

	if cfg.MQTTUsername != "" && cfg.MQTTPassword != "" {
		opts.SetUsername(cfg.MQTTUsername)
		opts.SetPassword(cfg.MQTTPassword)
	}

	s.client = mqtt.NewClient(opts)

	return s, nil
}

// Start connects to MQTT and starts receiving grid power data
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Connecting to MQTT broker...", "broker", s.brokerURL, "topic", s.topic)

	token := s.client.Connect()
	if token.WaitTimeout(connectTimeout) && token.Error() != nil {
		s.logger.Warn("Initial MQTT connection failed, will retry in background", "error", token.Error())
		s.state.UpdateGridError("Connecting to MQTT broker...")
		// Don't return error - the auto-reconnect will keep trying
	}

	// Start timeout checker goroutine
	go s.checkDataTimeout(ctx)

	// Start connection monitor goroutine
	go s.monitorConnection(ctx)

	// Wait for context cancellation
	<-ctx.Done()
	s.logger.Info("Grid service shutting down...")

	s.client.Disconnect(1000)
	return nil
}

// monitorConnection periodically checks connection status and logs it
func (s *Service) monitorConnection(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !s.client.IsConnected() {
				s.logger.Debug("MQTT client not connected, auto-reconnect is active")
			}
		}
	}
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
	// Reset reconnect counter on successful connection
	s.reconnectMu.Lock()
	s.reconnectCount = 0
	s.reconnectMu.Unlock()

	s.logger.Info("Connected to MQTT broker, subscribing to topic", "topic", s.topic, "broker", s.brokerURL)

	token := client.Subscribe(s.topic, 1, s.onMessage)
	if token.Wait() && token.Error() != nil {
		s.logger.Error("Failed to subscribe to topic", "topic", s.topic, "error", token.Error())
		s.state.UpdateGridError("Failed to subscribe to MQTT topic")
		return
	}

	s.logger.Info("Subscribed to MQTT topic", "topic", s.topic)
}

func (s *Service) onConnectionLost(client mqtt.Client, err error) {
	s.logger.Warn("MQTT connection lost, will attempt to reconnect", "error", err, "broker", s.brokerURL)
	s.state.UpdateGridError("MQTT connection lost - reconnecting...")
}

func (s *Service) onReconnecting(client mqtt.Client, opts *mqtt.ClientOptions) {
	s.reconnectMu.Lock()
	s.reconnectCount++
	count := s.reconnectCount
	s.reconnectMu.Unlock()

	s.logger.Info("Attempting to reconnect to MQTT broker", "attempt", count, "broker", s.brokerURL)
	s.state.UpdateGridError(fmt.Sprintf("Reconnecting to MQTT (attempt %d)...", count))
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
