package solar

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jorgen-simonsson/sotehus-backend/internal/config"
	"github.com/jorgen-simonsson/sotehus-backend/internal/state"
)

func TestNewServiceMissingAPIKey(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()

	cfg := &config.Config{
		SolarEdgeAPIKey: "",
		SolarEdgeSiteID: "12345",
	}

	service, err := NewService(cfg, mgr, logger)

	if err == nil {
		t.Error("Expected error for missing API key")
	}
	if service != nil {
		t.Error("Expected nil service for missing API key")
	}
	if err.Error() != "SolarEdge API key is required" {
		t.Errorf("Error message = %q, want %q", err.Error(), "SolarEdge API key is required")
	}
}

func TestNewServiceMissingSiteID(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()

	cfg := &config.Config{
		SolarEdgeAPIKey: "test-api-key",
		SolarEdgeSiteID: "",
	}

	service, err := NewService(cfg, mgr, logger)

	if err == nil {
		t.Error("Expected error for missing site ID")
	}
	if service != nil {
		t.Error("Expected nil service for missing site ID")
	}
	if err.Error() != "SolarEdge site ID is required" {
		t.Errorf("Error message = %q, want %q", err.Error(), "SolarEdge site ID is required")
	}
}

func TestNewServiceSuccess(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()

	cfg := &config.Config{
		SolarEdgeAPIKey: "test-api-key",
		SolarEdgeSiteID: "12345",
	}

	service, err := NewService(cfg, mgr, logger)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if service == nil {
		t.Fatal("Expected non-nil service")
	}
	if service.apiKey != "test-api-key" {
		t.Errorf("apiKey = %q, want %q", service.apiKey, "test-api-key")
	}
	if service.siteID != "12345" {
		t.Errorf("siteID = %q, want %q", service.siteID, "12345")
	}
}

func TestCalculateSunriseSunsetSummer(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()

	cfg := &config.Config{
		SolarEdgeAPIKey: "test-api-key",
		SolarEdgeSiteID: "12345",
	}

	service, _ := NewService(cfg, mgr, logger)

	// Test midsummer (June 21)
	date := time.Date(2025, 6, 21, 12, 0, 0, 0, time.Local)
	sunrise, sunset := service.calculateSunriseSunset(date)

	// In Stockholm, summer sunrise should be early (around 3-4 AM)
	if sunrise.Hour() > 6 {
		t.Errorf("Summer sunrise hour = %d, expected < 6", sunrise.Hour())
	}

	// Summer sunset should be late (around 10 PM)
	if sunset.Hour() < 20 {
		t.Errorf("Summer sunset hour = %d, expected >= 20", sunset.Hour())
	}

	// Daylight should be long in summer
	daylightHours := sunset.Sub(sunrise).Hours()
	if daylightHours < 15 {
		t.Errorf("Summer daylight hours = %.1f, expected > 15", daylightHours)
	}
}

func TestCalculateSunriseSunsetWinter(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()

	cfg := &config.Config{
		SolarEdgeAPIKey: "test-api-key",
		SolarEdgeSiteID: "12345",
	}

	service, _ := NewService(cfg, mgr, logger)

	// Test midwinter (December 21)
	date := time.Date(2025, 12, 21, 12, 0, 0, 0, time.Local)
	sunrise, sunset := service.calculateSunriseSunset(date)

	// In Stockholm, winter sunrise should be late (around 8-9 AM)
	if sunrise.Hour() < 7 {
		t.Errorf("Winter sunrise hour = %d, expected >= 7", sunrise.Hour())
	}

	// Winter sunset should be early (around 3 PM)
	if sunset.Hour() > 16 {
		t.Errorf("Winter sunset hour = %d, expected <= 16", sunset.Hour())
	}

	// Daylight should be short in winter
	daylightHours := sunset.Sub(sunrise).Hours()
	if daylightHours > 10 {
		t.Errorf("Winter daylight hours = %.1f, expected < 10", daylightHours)
	}
}

func TestIsSunUpMidday(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()

	cfg := &config.Config{
		SolarEdgeAPIKey: "test-api-key",
		SolarEdgeSiteID: "12345",
	}

	service, _ := NewService(cfg, mgr, logger)

	// Test at noon - should always be considered sun up (in Stockholm at least)
	// This tests the logic but uses current time
	isUp := service.isSunUp()

	// Just verify it returns a boolean without error
	_ = isUp
}

func TestCalculateUpdateInterval(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()

	cfg := &config.Config{
		SolarEdgeAPIKey: "test-api-key",
		SolarEdgeSiteID: "12345",
	}

	service, _ := NewService(cfg, mgr, logger)

	interval := service.calculateUpdateInterval()

	// Interval should be at least minimum (5 minutes)
	if interval < 5*time.Minute {
		t.Errorf("Interval = %v, expected >= 5 minutes", interval)
	}

	// Interval should be reasonable (not more than 30 minutes)
	if interval > 30*time.Minute {
		t.Errorf("Interval = %v, expected <= 30 minutes", interval)
	}
}

func TestNightBufferHours(t *testing.T) {
	// Verify constant is set correctly
	if nightBufferHours != 1 {
		t.Errorf("nightBufferHours = %d, want 1", nightBufferHours)
	}
}

func TestMaxDailyCallsConstant(t *testing.T) {
	// Verify API limit constant
	if maxDailyCalls != 300 {
		t.Errorf("maxDailyCalls = %d, want 300", maxDailyCalls)
	}
}

func TestUsagePercentConstant(t *testing.T) {
	// Verify we're only using 90% of allowed calls
	if usagePercent != 0.9 {
		t.Errorf("usagePercent = %f, want 0.9", usagePercent)
	}
}

func TestMinIntervalMinsConstant(t *testing.T) {
	// Verify minimum interval between calls
	if minIntervalMins != 5 {
		t.Errorf("minIntervalMins = %d, want 5", minIntervalMins)
	}
}

func TestStockholmCoordinates(t *testing.T) {
	// Verify Stockholm coordinates are reasonable
	if stockholmLat < 59 || stockholmLat > 60 {
		t.Errorf("stockholmLat = %f, expected ~59.3", stockholmLat)
	}
	if stockholmLon < 17 || stockholmLon > 19 {
		t.Errorf("stockholmLon = %f, expected ~18.0", stockholmLon)
	}
}
