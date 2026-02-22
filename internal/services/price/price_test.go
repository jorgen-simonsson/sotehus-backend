package price

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jorgen-simonsson/sotehus-backend/internal/config"
	"github.com/jorgen-simonsson/sotehus-backend/internal/models"
	"github.com/jorgen-simonsson/sotehus-backend/internal/state"
)

func TestNewService(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()

	cfg := &config.Config{
		SpotPriceRegion: "SE4",
	}

	service := NewService(cfg, mgr, logger)

	if service == nil {
		t.Fatal("Expected non-nil service")
	}
	if service.region != "SE4" {
		t.Errorf("region = %q, want %q", service.region, "SE4")
	}
	if service.state != mgr {
		t.Error("state not set correctly")
	}
}

func TestGetCurrentPriceEmptyList(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()

	cfg := &config.Config{
		SpotPriceRegion: "SE4",
	}

	service := NewService(cfg, mgr, logger)

	prices := []models.SpotPriceEntry{}

	_, err := service.getCurrentPrice(prices)

	if err == nil {
		t.Error("Expected error for empty price list")
	}
	if err.Error() != "no prices available" {
		t.Errorf("Error message = %q, want %q", err.Error(), "no prices available")
	}
}

func TestGetCurrentPriceMatchingHour(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()

	cfg := &config.Config{
		SpotPriceRegion: "SE4",
	}

	service := NewService(cfg, mgr, logger)

	now := time.Now()
	hourStart := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, now.Location())
	hourEnd := hourStart.Add(time.Hour)

	prices := []models.SpotPriceEntry{
		{
			SEKPerKWh: 0.89,
			EURPerKWh: 0.08,
			TimeStart: hourStart.Format(time.RFC3339),
			TimeEnd:   hourEnd.Format(time.RFC3339),
		},
	}

	price, err := service.getCurrentPrice(prices)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if price != 0.89 {
		t.Errorf("price = %f, want %f", price, 0.89)
	}
}

func TestGetCurrentPriceNoMatch(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()

	cfg := &config.Config{
		SpotPriceRegion: "SE4",
	}

	service := NewService(cfg, mgr, logger)

	// Create prices for yesterday
	yesterday := time.Now().AddDate(0, 0, -1)
	hourStart := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 12, 0, 0, 0, yesterday.Location())
	hourEnd := hourStart.Add(time.Hour)

	prices := []models.SpotPriceEntry{
		{
			SEKPerKWh: 0.89,
			EURPerKWh: 0.08,
			TimeStart: hourStart.Format(time.RFC3339),
			TimeEnd:   hourEnd.Format(time.RFC3339),
		},
	}

	_, err := service.getCurrentPrice(prices)

	if err == nil {
		t.Error("Expected error for no matching price")
	}
	if err.Error() != "no matching price found for current time" {
		t.Errorf("Error message = %q, want %q", err.Error(), "no matching price found for current time")
	}
}

func TestGetCurrentPriceInvalidTimeFormat(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()

	cfg := &config.Config{
		SpotPriceRegion: "SE4",
	}

	service := NewService(cfg, mgr, logger)

	prices := []models.SpotPriceEntry{
		{
			SEKPerKWh: 0.89,
			EURPerKWh: 0.08,
			TimeStart: "invalid-time",
			TimeEnd:   "2025-12-07T17:00:00+01:00",
		},
	}

	_, err := service.getCurrentPrice(prices)

	// Should fail because no valid entries found
	if err == nil {
		t.Error("Expected error for invalid time format")
	}
}

func TestGetCurrentPriceMultipleHours(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()

	cfg := &config.Config{
		SpotPriceRegion: "SE4",
	}

	service := NewService(cfg, mgr, logger)

	now := time.Now()
	currentHourStart := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, now.Location())
	currentHourEnd := currentHourStart.Add(time.Hour)

	previousHourStart := currentHourStart.Add(-time.Hour)
	previousHourEnd := currentHourStart

	nextHourStart := currentHourEnd
	nextHourEnd := nextHourStart.Add(time.Hour)

	prices := []models.SpotPriceEntry{
		{
			SEKPerKWh: 0.50, // Previous hour
			EURPerKWh: 0.05,
			TimeStart: previousHourStart.Format(time.RFC3339),
			TimeEnd:   previousHourEnd.Format(time.RFC3339),
		},
		{
			SEKPerKWh: 0.89, // Current hour - should match
			EURPerKWh: 0.08,
			TimeStart: currentHourStart.Format(time.RFC3339),
			TimeEnd:   currentHourEnd.Format(time.RFC3339),
		},
		{
			SEKPerKWh: 1.20, // Next hour
			EURPerKWh: 0.11,
			TimeStart: nextHourStart.Format(time.RFC3339),
			TimeEnd:   nextHourEnd.Format(time.RFC3339),
		},
	}

	price, err := service.getCurrentPrice(prices)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if price != 0.89 {
		t.Errorf("price = %f, want %f", price, 0.89)
	}
}

func TestUpdateIntervalConstant(t *testing.T) {
	// Verify update interval is 15 minutes
	if updateInterval != 15*time.Minute {
		t.Errorf("updateInterval = %v, want %v", updateInterval, 15*time.Minute)
	}
}

func TestBaseURLConstant(t *testing.T) {
	expectedURL := "https://www.elprisetjustnu.se/api/v1/prices"
	if baseURL != expectedURL {
		t.Errorf("baseURL = %q, want %q", baseURL, expectedURL)
	}
}
