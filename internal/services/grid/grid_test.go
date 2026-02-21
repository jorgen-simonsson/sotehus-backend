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

func TestGridTopicsNotEmpty(t *testing.T) {
// Verify that GridTopics has at least one entry
if len(GridTopics) == 0 {
t.Error("GridTopics should not be empty")
}
}

func TestGridTopicsHaveValidFields(t *testing.T) {
// Verify each topic mapping has both topic and field name
for i, tm := range GridTopics {
if tm.Topic == "" {
t.Errorf("GridTopics[%d].Topic is empty", i)
}
if tm.FieldName == "" {
t.Errorf("GridTopics[%d].FieldName is empty", i)
}
}
}

func TestNewServiceSuccess(t *testing.T) {
logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
mgr := state.NewManager()

cfg := &config.Config{
MQTTBrokerHost: "localhost",
MQTTBrokerPort: 1883,
}

service, err := NewService(cfg, mgr, nil, logger)

if err != nil {
t.Fatalf("Unexpected error: %v", err)
}
if service == nil {
t.Fatal("Expected non-nil service")
}
if service.state != mgr {
t.Error("state not set correctly")
}
// Verify topic-to-field mapping is built correctly
if len(service.topicToField) != len(GridTopics) {
t.Errorf("topicToField has %d entries, want %d", len(service.topicToField), len(GridTopics))
}
}

func TestNewServiceWithAuthentication(t *testing.T) {
logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
mgr := state.NewManager()

cfg := &config.Config{
MQTTBrokerHost: "localhost",
MQTTBrokerPort: 1883,
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

func TestAggregationTimeoutConstant(t *testing.T) {
// Verify aggregation timeout is 2 seconds
if aggregationTimeout != 2*time.Second {
t.Errorf("aggregationTimeout = %v, want %v", aggregationTimeout, 2*time.Second)
}
}

func TestLastMessageTimeMutex(t *testing.T) {
logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
mgr := state.NewManager()

cfg := &config.Config{
MQTTBrokerHost: "localhost",
MQTTBrokerPort: 1883,
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
}

service, err := NewService(cfg, mgr, nil, logger)

if err != nil {
t.Fatalf("Unexpected error: %v", err)
}

if service.logger != logger {
t.Error("logger not set correctly")
}
if service.influxDB != nil {
t.Error("influxDB should be nil when not provided")
}
if service.pendingValues == nil {
t.Error("pendingValues should be initialized")
}
}

func TestGetTopicNames(t *testing.T) {
logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
mgr := state.NewManager()

cfg := &config.Config{
MQTTBrokerHost: "localhost",
MQTTBrokerPort: 1883,
}

service, _ := NewService(cfg, mgr, nil, logger)
names := service.getTopicNames()

if len(names) != len(GridTopics) {
t.Errorf("getTopicNames returned %d names, want %d", len(names), len(GridTopics))
}

// Verify names match GridTopics
for i, name := range names {
if name != GridTopics[i].Topic {
t.Errorf("getTopicNames()[%d] = %q, want %q", i, name, GridTopics[i].Topic)
}
}
}

func TestAllValuesReceived(t *testing.T) {
logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
mgr := state.NewManager()

cfg := &config.Config{
MQTTBrokerHost: "localhost",
MQTTBrokerPort: 1883,
}

service, _ := NewService(cfg, mgr, nil, logger)

// Initially should be false (no values)
if service.allValuesReceived() {
t.Error("allValuesReceived should be false when pendingValues is empty")
}

// Add all values
for _, tm := range GridTopics {
service.pendingValues[tm.FieldName] = 123.45
}

if !service.allValuesReceived() {
t.Error("allValuesReceived should be true when all values are present")
}
}
