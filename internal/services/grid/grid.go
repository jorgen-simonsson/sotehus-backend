package grid

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
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
	aggregationTimeout    = 2 * time.Second  // Time to wait for all topic values before writing to InfluxDB
)

// TopicMapping defines an MQTT topic and its corresponding InfluxDB field name
type TopicMapping struct {
	Topic     string
	FieldName string
}

// GridTopics defines the static list of MQTT topics to subscribe to and their InfluxDB field names.
// Add new topics here to include them in the aggregated InfluxDB write.
var GridTopics = []TopicMapping{
	{Topic: "dsmr/reading/powerdelivered_netto", FieldName: "grid_power"},
	{Topic: "dsmr/reading/electricity_delivered_1", FieldName: "grid_enery_consumed_ack"},
	{Topic: "dsmr/reading/electricity_returned_1", FieldName: "grid_enery_sold_ack"},
}

// Service handles grid power consumption via MQTT
type Service struct {
	client          mqtt.Client
	brokerURL       string
	state           *state.Manager
	influxDB        *storage.InfluxDBClient
	logger          *slog.Logger
	lastMessageTime time.Time
	lastMessageMu   sync.RWMutex
	reconnectCount  int
	reconnectMu     sync.Mutex

	// Aggregation fields
	pendingValues    map[string]float64
	pendingMu        sync.Mutex
	aggregationTimer *time.Timer
	batchTimestamp   time.Time
	topicToField     map[string]string // maps topic -> fieldName for quick lookup
}

// NewService creates a new grid service
func NewService(cfg *config.Config, state *state.Manager, influxDB *storage.InfluxDBClient, logger *slog.Logger) (*Service, error) {
	if cfg.MQTTBrokerHost == "" {
		return nil, fmt.Errorf("MQTT broker host is required")
	}
	if len(GridTopics) == 0 {
		return nil, fmt.Errorf("no MQTT topics configured in GridTopics")
	}

	brokerURL := fmt.Sprintf("tcp://%s:%d", cfg.MQTTBrokerHost, cfg.MQTTBrokerPort)

	// Build topic-to-field lookup map
	topicToField := make(map[string]string, len(GridTopics))
	for _, tm := range GridTopics {
		topicToField[tm.Topic] = tm.FieldName
	}

	s := &Service{
		brokerURL:     brokerURL,
		state:         state,
		influxDB:      influxDB,
		logger:        logger,
		pendingValues: make(map[string]float64),
		topicToField:  topicToField,
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
	topicNames := s.getTopicNames()
	s.logger.Info("Connecting to MQTT broker...", "broker", s.brokerURL, "topics", strings.Join(topicNames, ", "))

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

	topicNames := s.getTopicNames()
	s.logger.Info("Connected to MQTT broker, subscribing to topics", "topics", strings.Join(topicNames, ", "), "broker", s.brokerURL)

	// Build subscription map with QoS 1 for all topics
	filters := make(map[string]byte, len(GridTopics))
	for _, tm := range GridTopics {
		filters[tm.Topic] = 1
	}

	token := client.SubscribeMultiple(filters, s.onMessage)
	if token.Wait() && token.Error() != nil {
		s.logger.Error("Failed to subscribe to topics", "topics", strings.Join(topicNames, ", "), "error", token.Error())
		s.state.UpdateGridError("Failed to subscribe to MQTT topic")
		return
	}

	s.logger.Info("Subscribed to MQTT topics", "topics", strings.Join(topicNames, ", "))
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
	topic := msg.Topic()
	payload := msg.Payload()

	// Look up the field name for this topic
	fieldName, ok := s.topicToField[topic]
	if !ok {
		s.logger.Warn("Received message for unknown topic", "topic", topic)
		return
	}

	// Parse the value from payload
	var value float64
	// Try JSON format first: {"value": 123.45} or {"power": 123.45}
	var jsonPayload map[string]interface{}
	if err := json.Unmarshal(payload, &jsonPayload); err == nil {
		// Try common field names
		for _, key := range []string{"value", "power", "Value", "Power"} {
			if v, exists := jsonPayload[key]; exists {
				if f, ok := v.(float64); ok {
					value = f
					break
				}
			}
		}
		// If no known key found, try to use the first numeric value
		if value == 0 {
			for _, v := range jsonPayload {
				if f, ok := v.(float64); ok {
					value = f
					break
				}
			}
		}
	} else {
		// Try plain number format
		if _, err := fmt.Sscanf(string(payload), "%f", &value); err != nil {
			s.logger.Warn("Failed to parse MQTT payload", "topic", topic, "payload", string(payload), "error", err)
			return
		}
	}

	s.logger.Debug("Received grid data", "topic", topic, "field", fieldName, "value", value)

	timestamp := time.Now()

	// Update last message time
	s.lastMessageMu.Lock()
	s.lastMessageTime = timestamp
	s.lastMessageMu.Unlock()

	// Add value to pending batch
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()

	// If this is the first value in a new batch, record the timestamp and start timer
	if len(s.pendingValues) == 0 {
		s.batchTimestamp = timestamp
		s.aggregationTimer = time.AfterFunc(aggregationTimeout, s.onAggregationTimeout)
	}

	s.pendingValues[fieldName] = value

	// Update state with grid power if that's what we received
	if fieldName == "grid_power" {
		s.state.UpdateGrid(value, timestamp)
	}

	// Check if we have all values
	if s.allValuesReceived() {
		s.flushToInfluxLocked()
	}
}

// getTopicNames returns a slice of topic names from GridTopics
func (s *Service) getTopicNames() []string {
	names := make([]string, len(GridTopics))
	for i, tm := range GridTopics {
		names[i] = tm.Topic
	}
	return names
}

// allValuesReceived checks if all expected topic values have been received
func (s *Service) allValuesReceived() bool {
	return len(s.pendingValues) >= len(GridTopics)
}

// onAggregationTimeout is called when the timeout expires before all values are received
func (s *Service) onAggregationTimeout() {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()

	if len(s.pendingValues) == 0 {
		return
	}

	// Log warning about missing values
	var missing []string
	for _, tm := range GridTopics {
		if _, ok := s.pendingValues[tm.FieldName]; !ok {
			missing = append(missing, tm.FieldName)
		}
	}
	s.logger.Warn("Aggregation timeout - writing incomplete data to InfluxDB",
		"received", len(s.pendingValues),
		"expected", len(GridTopics),
		"missing", strings.Join(missing, ", "))

	s.flushToInfluxLocked()
}

// flushToInfluxLocked writes pending values to InfluxDB and resets the batch.
// Must be called with pendingMu held.
func (s *Service) flushToInfluxLocked() {
	if s.aggregationTimer != nil {
		s.aggregationTimer.Stop()
		s.aggregationTimer = nil
	}

	if s.influxDB == nil || len(s.pendingValues) == 0 {
		s.pendingValues = make(map[string]float64)
		return
	}

	// Get average frequency since last write to include in the record
	freqData := s.state.GetAndResetAverageFrequency()
	if freqData.Valid && freqData.Frequency > 0 {
		s.pendingValues["grid_frequency"] = freqData.Frequency
	}

	// Get current spot price from state to include in the write
	priceData := s.state.GetPriceData()
	if priceData.Valid {
		s.pendingValues["spot_price"] = priceData.Price
	}

	// Get current solar production from state to include in the write
	solarData := s.state.GetSolarData()
	if solarData.Valid {
		s.pendingValues["solar_production"] = solarData.Power
	}

	// Get solar energy accumulator from state
	if energyAck, ok := s.state.GetSolarEnergyAck(); ok {
		s.pendingValues["solar_energy_ack"] = energyAck
	}

	// Get solar AC frequency from state
	if solarFreq, ok := s.state.GetSolarFrequency(); ok {
		s.pendingValues["solar_frequency"] = solarFreq
	}

	// Create a copy for the write
	fields := make(map[string]float64, len(s.pendingValues))
	for k, v := range s.pendingValues {
		fields[k] = v
	}

	s.logger.Debug("Writing aggregated data to InfluxDB", "fields", len(fields))

	if err := s.influxDB.WriteGridData(fields, s.batchTimestamp); err != nil {
		s.logger.Warn("Failed to write grid data to InfluxDB", "error", err)
	}

	// Reset pending values for next batch
	s.pendingValues = make(map[string]float64)
}
