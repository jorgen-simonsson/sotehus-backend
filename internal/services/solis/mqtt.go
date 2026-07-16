package solis

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/jorgen-simonsson/sotehus-backend/internal/config"
	"github.com/jorgen-simonsson/sotehus-backend/internal/storage"
)

const (
	mqttTopic          = "solis/modbus"
	influxBucket       = "solis"
	influxMeasurement  = "solis_inverter"
	mqttInitReconnect  = 1 * time.Second
	mqttMaxReconnect   = 2 * time.Minute
	mqttKeepAlive      = 30 * time.Second
	mqttPingTimeout    = 10 * time.Second
	mqttConnectTimeout = 30 * time.Second
)

// Service subscribes to Solis inverter data published by solis2mqtt and
// writes every received payload as a record in the "solis" InfluxDB bucket.
type Service struct {
	client         mqtt.Client
	brokerURL      string
	influxDB       *storage.InfluxDBClient
	logger         *slog.Logger
	reconnectMu    sync.Mutex
	reconnectCount int
}

// NewService creates a new Solis MQTT service
func NewService(cfg *config.Config, influxDB *storage.InfluxDBClient, logger *slog.Logger) (*Service, error) {
	if cfg.MQTTBrokerHost == "" {
		return nil, fmt.Errorf("MQTT broker host is required")
	}

	brokerURL := fmt.Sprintf("tcp://%s:%d", cfg.MQTTBrokerHost, cfg.MQTTBrokerPort)

	s := &Service{
		brokerURL: brokerURL,
		influxDB:  influxDB,
		logger:    logger,
	}

	opts := mqtt.NewClientOptions().
		AddBroker(brokerURL).
		SetClientID(fmt.Sprintf("sotehus-backend-solis-%d", time.Now().UnixNano())).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(mqttInitReconnect).
		SetMaxReconnectInterval(mqttMaxReconnect).
		SetKeepAlive(mqttKeepAlive).
		SetPingTimeout(mqttPingTimeout).
		SetConnectTimeout(mqttConnectTimeout).
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

// Start ensures the target InfluxDB bucket exists, connects to MQTT and starts receiving Solis data
func (s *Service) Start(ctx context.Context) error {
	if s.influxDB != nil {
		if err := s.influxDB.EnsureBucketExists(influxBucket); err != nil {
			s.logger.Error("Failed to ensure Solis InfluxDB bucket exists", "bucket", influxBucket, "error", err)
		}
	}

	s.logger.Info("Connecting to MQTT broker for Solis data...", "broker", s.brokerURL, "topic", mqttTopic)

	token := s.client.Connect()
	if token.WaitTimeout(mqttConnectTimeout) && token.Error() != nil {
		s.logger.Warn("Initial MQTT connection for Solis failed, will retry in background", "error", token.Error())
	}

	<-ctx.Done()
	s.logger.Info("Solis service shutting down...")

	s.client.Disconnect(1000)
	return nil
}

func (s *Service) onConnect(client mqtt.Client) {
	s.reconnectMu.Lock()
	s.reconnectCount = 0
	s.reconnectMu.Unlock()

	s.logger.Info("Connected to MQTT broker for Solis, subscribing", "topic", mqttTopic)

	token := client.Subscribe(mqttTopic, 1, s.onMessage)
	if token.WaitTimeout(10*time.Second) && token.Error() != nil {
		s.logger.Error("Failed to subscribe to Solis topic", "topic", mqttTopic, "error", token.Error())
	}
}

func (s *Service) onConnectionLost(_ mqtt.Client, err error) {
	s.logger.Warn("Solis MQTT connection lost", "error", err)
}

func (s *Service) onReconnecting(_ mqtt.Client, _ *mqtt.ClientOptions) {
	s.reconnectMu.Lock()
	s.reconnectCount++
	count := s.reconnectCount
	s.reconnectMu.Unlock()

	s.logger.Info("Reconnecting Solis MQTT...", "attempt", count)
}

// onMessage parses the flat Solis payload (dot-containing keys such as
// "voltage.L1" are literal JSON keys, not nested objects) and writes every
// property in the payload as a single InfluxDB record.
func (s *Service) onMessage(_ mqtt.Client, msg mqtt.Message) {
	var payload map[string]float64
	if err := json.Unmarshal(msg.Payload(), &payload); err != nil {
		s.logger.Error("Failed to parse Solis MQTT payload", "error", err)
		return
	}

	if len(payload) == 0 {
		return
	}

	s.logger.Debug("Received Solis data", "fields", len(payload))

	if s.influxDB == nil {
		return
	}

	if err := s.influxDB.WriteToBucket(influxBucket, influxMeasurement, payload, time.Now()); err != nil {
		s.logger.Warn("Failed to write Solis data to InfluxDB", "error", err)
	}
}
