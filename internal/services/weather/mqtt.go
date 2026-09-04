package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/jorgen-simonsson/sotehus-backend/internal/config"
	"github.com/jorgen-simonsson/sotehus-backend/internal/models"
	"github.com/jorgen-simonsson/sotehus-backend/internal/state"
	"github.com/jorgen-simonsson/sotehus-backend/internal/storage"
)

const (
	mqttTopic          = "weather/gw1200/json"
	influxBucket       = "weather"
	influxMeasurement  = "weather_station"
	mqttInitReconnect  = 1 * time.Second
	mqttMaxReconnect   = 2 * time.Minute
	mqttKeepAlive      = 30 * time.Second
	mqttPingTimeout    = 10 * time.Second
	mqttConnectTimeout = 30 * time.Second
)

// reading matches the {value, unitOfMeasure} shape published for every
// property by the gw1200 weather station poller (see ref/main.go).
type reading struct {
	Value         float64 `json:"value"`
	UnitOfMeasure string  `json:"unitOfMeasure"`
}

// Service subscribes to local weather station data published by the gw1200
// poller, keeps the latest readings in the state manager, and writes the
// numeric value of every reading as a single record in the "weather"
// InfluxDB bucket.
type Service struct {
	client         mqtt.Client
	brokerURL      string
	state          *state.Manager
	influxDB       *storage.InfluxDBClient
	logger         *slog.Logger
	reconnectMu    sync.Mutex
	reconnectCount int
}

// NewService creates a new weather MQTT service
func NewService(cfg *config.Config, state *state.Manager, influxDB *storage.InfluxDBClient, logger *slog.Logger) (*Service, error) {
	if cfg.MQTTBrokerHost == "" {
		return nil, fmt.Errorf("MQTT broker host is required")
	}

	brokerURL := fmt.Sprintf("tcp://%s:%d", cfg.MQTTBrokerHost, cfg.MQTTBrokerPort)

	s := &Service{
		brokerURL: brokerURL,
		state:     state,
		influxDB:  influxDB,
		logger:    logger,
	}

	opts := mqtt.NewClientOptions().
		AddBroker(brokerURL).
		SetClientID(fmt.Sprintf("sotehus-backend-weather-%d", time.Now().UnixNano())).
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

// Start ensures the target InfluxDB bucket exists, connects to MQTT and starts receiving weather data
func (s *Service) Start(ctx context.Context) error {
	if s.influxDB != nil {
		if err := s.influxDB.EnsureBucketExists(influxBucket); err != nil {
			s.logger.Error("Failed to ensure weather InfluxDB bucket exists", "bucket", influxBucket, "error", err)
		}
	}

	s.logger.Info("Connecting to MQTT broker for weather data...", "broker", s.brokerURL, "topic", mqttTopic)

	token := s.client.Connect()
	if token.WaitTimeout(mqttConnectTimeout) && token.Error() != nil {
		s.logger.Warn("Initial MQTT connection for weather failed, will retry in background", "error", token.Error())
	}

	<-ctx.Done()
	s.logger.Info("Weather service shutting down...")

	s.client.Disconnect(1000)
	return nil
}

func (s *Service) onConnect(client mqtt.Client) {
	s.reconnectMu.Lock()
	s.reconnectCount = 0
	s.reconnectMu.Unlock()

	s.logger.Info("Connected to MQTT broker for weather, subscribing", "topic", mqttTopic)

	token := client.Subscribe(mqttTopic, 1, s.onMessage)
	if token.WaitTimeout(10*time.Second) && token.Error() != nil {
		s.logger.Error("Failed to subscribe to weather topic", "topic", mqttTopic, "error", token.Error())
	}
}

func (s *Service) onConnectionLost(_ mqtt.Client, err error) {
	s.logger.Warn("Weather MQTT connection lost", "error", err)
}

func (s *Service) onReconnecting(_ mqtt.Client, _ *mqtt.ClientOptions) {
	s.reconnectMu.Lock()
	s.reconnectCount++
	count := s.reconnectCount
	s.reconnectMu.Unlock()

	s.logger.Info("Reconnecting weather MQTT...", "attempt", count)
}

// onMessage parses the {value, unitOfMeasure} payload published by the
// gw1200 poller, stores the readings (with units) in the state manager for
// the "last received" endpoint, and writes every reading's numeric value as
// a field in a single InfluxDB record.
func (s *Service) onMessage(_ mqtt.Client, msg mqtt.Message) {
	var payload map[string]reading
	if err := json.Unmarshal(msg.Payload(), &payload); err != nil {
		s.logger.Error("Failed to parse weather MQTT payload", "error", err)
		return
	}

	if len(payload) == 0 {
		return
	}

	s.logger.Debug("Received weather data", "fields", len(payload))

	readings := make(map[string]models.WeatherReading, len(payload))
	fields := make(map[string]float64, len(payload))
	for name, r := range payload {
		readings[name] = models.WeatherReading{Value: r.Value, UnitOfMeasure: r.UnitOfMeasure}
		fields[name] = r.Value
	}

	s.state.UpdateWeather(readings)

	if s.influxDB == nil {
		return
	}

	if err := s.influxDB.WriteToBucket(influxBucket, influxMeasurement, fields, time.Now()); err != nil {
		s.logger.Warn("Failed to write weather data to InfluxDB", "error", err)
	}
}
