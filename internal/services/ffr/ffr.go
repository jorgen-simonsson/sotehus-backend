package ffr

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/jorgen-simonsson/sotehus-backend/internal/config"
	"github.com/jorgen-simonsson/sotehus-backend/internal/state"
)

const (
	topic                 = "ffr_frequency"
	initialReconnectDelay = 1 * time.Second
	maxReconnectDelay     = 2 * time.Minute
	keepAliveInterval     = 30 * time.Second
	pingTimeout           = 10 * time.Second
	connectTimeout        = 30 * time.Second
)

// Service handles FFR (Frequency Containment Reserve) data via MQTT
type Service struct {
	client         mqtt.Client
	brokerURL      string
	state          *state.Manager
	logger         *slog.Logger
	reconnectMu    sync.Mutex
	reconnectCount int
}

// NewService creates a new FFR service
func NewService(cfg *config.Config, state *state.Manager, logger *slog.Logger) (*Service, error) {
	if cfg.MQTTBrokerHost == "" {
		return nil, fmt.Errorf("MQTT broker host is required")
	}

	brokerURL := fmt.Sprintf("tcp://%s:%d", cfg.MQTTBrokerHost, cfg.MQTTBrokerPort)

	s := &Service{
		brokerURL: brokerURL,
		state:     state,
		logger:    logger,
	}

	opts := mqtt.NewClientOptions().
		AddBroker(brokerURL).
		SetClientID(fmt.Sprintf("sotehus-backend-ffr-%d", time.Now().UnixNano())).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(initialReconnectDelay).
		SetMaxReconnectInterval(maxReconnectDelay).
		SetKeepAlive(keepAliveInterval).
		SetPingTimeout(pingTimeout).
		SetConnectTimeout(connectTimeout).
		SetCleanSession(true).
		SetWriteTimeout(10 * time.Second).
		SetOrderMatters(false).
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

// Start connects to MQTT and starts receiving FFR frequency data
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Connecting to MQTT broker for FFR...", "broker", s.brokerURL, "topic", topic)

	token := s.client.Connect()
	if token.WaitTimeout(connectTimeout) && token.Error() != nil {
		s.logger.Warn("Initial MQTT connection for FFR failed, will retry in background", "error", token.Error())
	}

	// Wait for context cancellation
	<-ctx.Done()
	s.logger.Info("FFR service shutting down...")

	s.client.Disconnect(1000)
	return nil
}

func (s *Service) onConnect(client mqtt.Client) {
	s.reconnectMu.Lock()
	s.reconnectCount = 0
	s.reconnectMu.Unlock()

	s.logger.Info("Connected to MQTT broker for FFR, subscribing to topic", "topic", topic, "broker", s.brokerURL)

	token := client.Subscribe(topic, 1, s.onMessage)
	if token.Wait() && token.Error() != nil {
		s.logger.Error("Failed to subscribe to FFR topic", "topic", topic, "error", token.Error())
		return
	}

	s.logger.Info("Subscribed to FFR topic", "topic", topic)
}

func (s *Service) onConnectionLost(client mqtt.Client, err error) {
	s.logger.Warn("FFR MQTT connection lost, will attempt to reconnect", "error", err, "broker", s.brokerURL)
}

func (s *Service) onReconnecting(client mqtt.Client, opts *mqtt.ClientOptions) {
	s.reconnectMu.Lock()
	s.reconnectCount++
	count := s.reconnectCount
	s.reconnectMu.Unlock()

	s.logger.Info("Attempting to reconnect to MQTT broker for FFR", "attempt", count, "broker", s.brokerURL)
}

func (s *Service) onMessage(client mqtt.Client, msg mqtt.Message) {
	payload := string(msg.Payload())

	// The payload is a 4 char string like "5001" representing 50.01 Hz
	// Convert to decimal frequency
	rawValue, err := strconv.ParseInt(payload, 10, 64)
	if err != nil {
		s.logger.Debug("Failed to parse FFR payload", "payload", payload, "error", err)
		return
	}

	// Convert to frequency (e.g., 5001 -> 50.01)
	frequency := float64(rawValue) / 100.0

	// Sanity check - frequency should be roughly around 50 Hz (48-52 Hz range)
	if frequency < 48.0 || frequency > 52.0 {
		s.logger.Debug("FFR frequency out of expected range", "frequency", frequency, "raw", payload)
		return
	}

	// Update state (thread-safe)
	s.state.UpdateFrequency(frequency)
}
