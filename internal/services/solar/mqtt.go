package solar

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
)

const (
	mqttTopic          = "solaredge/modbus/inverter"
	mqttInitReconnect  = 1 * time.Second
	mqttMaxReconnect   = 2 * time.Minute
	mqttKeepAlive      = 30 * time.Second
	mqttPingTimeout    = 10 * time.Second
	mqttConnectTimeout = 30 * time.Second
)

// inverterPayload represents the MQTT message from the local modbus-to-MQTT bridge.
// Only the fields we need are defined.
type inverterPayload struct {
	AC struct {
		Power struct {
			Actual float64 `json:"actual"`
		} `json:"power"`
		Frequency float64 `json:"frequency"`
	} `json:"ac"`
	EnergyTotal float64 `json:"energytotal"`
}

// MQTTService subscribes to local MQTT solar data from a modbus bridge
type MQTTService struct {
	client         mqtt.Client
	brokerURL      string
	state          *state.Manager
	logger         *slog.Logger
	reconnectMu    sync.Mutex
	reconnectCount int
}

// NewMQTTService creates a new MQTT-based solar service
func NewMQTTService(cfg *config.Config, state *state.Manager, logger *slog.Logger) (*MQTTService, error) {
	if cfg.MQTTBrokerHost == "" {
		return nil, fmt.Errorf("MQTT broker host is required")
	}

	brokerURL := fmt.Sprintf("tcp://%s:%d", cfg.MQTTBrokerHost, cfg.MQTTBrokerPort)

	s := &MQTTService{
		brokerURL: brokerURL,
		state:     state,
		logger:    logger,
	}

	opts := mqtt.NewClientOptions().
		AddBroker(brokerURL).
		SetClientID(fmt.Sprintf("sotehus-backend-solar-mqtt-%d", time.Now().UnixNano())).
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

// Start connects to MQTT and starts receiving solar data from the local modbus bridge
func (s *MQTTService) Start(ctx context.Context) error {
	s.logger.Info("Connecting to MQTT broker for local solar data...", "broker", s.brokerURL, "topic", mqttTopic)

	token := s.client.Connect()
	if token.WaitTimeout(mqttConnectTimeout) && token.Error() != nil {
		s.logger.Warn("Initial MQTT connection for solar failed, will retry in background", "error", token.Error())
	}

	<-ctx.Done()
	s.logger.Info("Solar MQTT service shutting down...")

	s.client.Disconnect(1000)
	return nil
}

func (s *MQTTService) onConnect(client mqtt.Client) {
	s.reconnectMu.Lock()
	s.reconnectCount = 0
	s.reconnectMu.Unlock()

	s.logger.Info("Connected to MQTT broker for solar, subscribing", "topic", mqttTopic)

	token := client.Subscribe(mqttTopic, 1, s.onMessage)
	if token.WaitTimeout(10*time.Second) && token.Error() != nil {
		s.logger.Error("Failed to subscribe to solar topic", "topic", mqttTopic, "error", token.Error())
	}
}

func (s *MQTTService) onConnectionLost(_ mqtt.Client, err error) {
	s.logger.Warn("Solar MQTT connection lost", "error", err)
}

func (s *MQTTService) onReconnecting(_ mqtt.Client, _ *mqtt.ClientOptions) {
	s.reconnectMu.Lock()
	s.reconnectCount++
	count := s.reconnectCount
	s.reconnectMu.Unlock()

	s.logger.Info("Reconnecting solar MQTT...", "attempt", count)
}

func (s *MQTTService) onMessage(_ mqtt.Client, msg mqtt.Message) {
	var payload inverterPayload
	if err := json.Unmarshal(msg.Payload(), &payload); err != nil {
		s.logger.Error("Failed to parse solar MQTT payload", "error", err)
		return
	}

	power := payload.AC.Power.Actual / 1000.0 // Convert W to kW
	s.logger.Info("Updated solar power from MQTT", "power_kw", power, "energy_total_wh", payload.EnergyTotal, "frequency", payload.AC.Frequency)
	s.state.UpdateSolar(power)
	s.state.UpdateSolarEnergyAck(payload.EnergyTotal)
	s.state.UpdateSolarFrequency(payload.AC.Frequency)
}
