package solar

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"time"

	"github.com/jorgen-simonsson/sotehus-backend/internal/config"
	"github.com/jorgen-simonsson/sotehus-backend/internal/models"
	"github.com/jorgen-simonsson/sotehus-backend/internal/state"
	"github.com/jorgen-simonsson/sotehus-backend/internal/storage"
)

const (
	baseURL           = "https://monitoringapi.solaredge.com"
	maxDailyCalls     = 300
	usagePercent      = 0.9
	minIntervalMins   = 5
	stockholmLat      = 59.3293
	stockholmLon      = 18.0686
	nightBufferHours  = 1 // Don't poll 1 hour before sunrise or after sunset
)

// Service handles fetching solar production from SolarEdge API
type Service struct {
	apiKey   string
	siteID   string
	state    *state.Manager
	influxDB *storage.InfluxDBClient
	logger   *slog.Logger
	client   *http.Client
}

// NewService creates a new solar service
func NewService(cfg *config.Config, state *state.Manager, influxDB *storage.InfluxDBClient, logger *slog.Logger) (*Service, error) {
	if cfg.SolarEdgeAPIKey == "" {
		return nil, fmt.Errorf("SolarEdge API key is required")
	}
	if cfg.SolarEdgeSiteID == "" {
		return nil, fmt.Errorf("SolarEdge site ID is required")
	}

	return &Service{
		apiKey:   cfg.SolarEdgeAPIKey,
		siteID:   cfg.SolarEdgeSiteID,
		state:    state,
		influxDB: influxDB,
		logger:   logger,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// Start begins the solar production fetching loop
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Starting solar service", "siteID", s.siteID)

	for {
		// Check if sun is up
		if !s.isSunUp() {
			s.logger.Debug("Sun is down, skipping solar fetch")
			// Set solar data as invalid during nighttime
			s.state.UpdateSolarError("No sun")
			// Wait until next check (every 15 minutes during night)
			select {
			case <-ctx.Done():
				s.logger.Info("Solar service shutting down...")
				return nil
			case <-time.After(15 * time.Minute):
				continue
			}
		}

		// Fetch solar production
		s.fetchAndUpdate()

		// Calculate interval based on daylight hours
		interval := s.calculateUpdateInterval()
		s.logger.Debug("Next solar update", "interval", interval)

		select {
		case <-ctx.Done():
			s.logger.Info("Solar service shutting down...")
			return nil
		case <-time.After(interval):
		}
	}
}

func (s *Service) fetchAndUpdate() {
	power, err := s.fetchCurrentPower()
	if err != nil {
		s.logger.Error("Failed to fetch solar power", "error", err)
		s.state.UpdateSolarError("Failed to fetch solar data")
		return
	}

	s.logger.Info("Updated solar power", "power", power)
	s.state.UpdateSolar(power)

	// Write to InfluxDB
	if s.influxDB != nil {
		if err := s.influxDB.WriteSolarPower(power, time.Now()); err != nil {
			s.logger.Warn("Failed to write solar power to InfluxDB", "error", err)
		}
	}
}

func (s *Service) fetchCurrentPower() (float64, error) {
	url := fmt.Sprintf("%s/site/%s/currentPowerFlow?api_key=%s",
		baseURL, s.siteID, s.apiKey)

	s.logger.Debug("Fetching solar power", "url", fmt.Sprintf("%s/site/%s/currentPowerFlow", baseURL, s.siteID))

	resp, err := s.client.Get(url)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch solar power: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var powerFlow models.SolarEdgePowerFlow
	if err := json.NewDecoder(resp.Body).Decode(&powerFlow); err != nil {
		return 0, fmt.Errorf("failed to decode solar power: %w", err)
	}

	// Return PV (solar) power
	return powerFlow.SiteCurrentPowerFlow.PV.CurrentPower, nil
}

// isSunUp checks if we should poll for solar data
// Polls from 1 hour before sunrise until 1 hour after sunset
func (s *Service) isSunUp() bool {
	now := time.Now()
	sunrise, sunset := s.calculateSunriseSunset(now)

	// Start polling 1 hour before sunrise, stop 1 hour after sunset
	adjustedSunrise := sunrise.Add(-nightBufferHours * time.Hour)
	adjustedSunset := sunset.Add(nightBufferHours * time.Hour)

	return now.After(adjustedSunrise) && now.Before(adjustedSunset)
}

// calculateUpdateInterval calculates the optimal interval between API calls
func (s *Service) calculateUpdateInterval() time.Duration {
	now := time.Now()
	sunrise, sunset := s.calculateSunriseSunset(now)

	// Calculate daylight duration
	daylightDuration := sunset.Sub(sunrise)
	daylightMinutes := daylightDuration.Minutes()

	// Calculate allowed calls (90% of max)
	allowedCalls := float64(maxDailyCalls) * usagePercent

	// Calculate interval in minutes
	intervalMinutes := daylightMinutes / allowedCalls

	// Ensure minimum interval
	if intervalMinutes < minIntervalMins {
		intervalMinutes = minIntervalMins
	}

	return time.Duration(intervalMinutes) * time.Minute
}

// calculateSunriseSunset calculates sunrise and sunset times for Stockholm
// This is a simplified calculation - for production, consider using a proper astronomical library
func (s *Service) calculateSunriseSunset(date time.Time) (sunrise, sunset time.Time) {
	// Day of year (1-365)
	dayOfYear := float64(date.YearDay())

	// Calculate solar declination
	declination := 23.45 * math.Sin(2*math.Pi*(284+dayOfYear)/365)
	declinationRad := declination * math.Pi / 180

	// Latitude in radians
	latRad := stockholmLat * math.Pi / 180

	// Calculate hour angle
	cosHourAngle := -math.Tan(latRad) * math.Tan(declinationRad)

	// Clamp to valid range for polar regions
	if cosHourAngle > 1 {
		// Polar night - sun never rises
		// Return noon as both sunrise and sunset
		noon := time.Date(date.Year(), date.Month(), date.Day(), 12, 0, 0, 0, date.Location())
		return noon, noon
	}
	if cosHourAngle < -1 {
		// Midnight sun - sun never sets
		return time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location()),
			time.Date(date.Year(), date.Month(), date.Day(), 23, 59, 59, 0, date.Location())
	}

	hourAngle := math.Acos(cosHourAngle) * 180 / math.Pi

	// Convert to hours from solar noon
	hoursFromNoon := hourAngle / 15

	// Solar noon is approximately at 12:00 local time (simplified)
	// Adjust for longitude (Stockholm is at 18°E, so about +1.2 hours from UTC)
	solarNoon := 12.0

	sunriseHour := solarNoon - hoursFromNoon
	sunsetHour := solarNoon + hoursFromNoon

	sunriseTime := time.Date(date.Year(), date.Month(), date.Day(),
		int(sunriseHour), int((sunriseHour-float64(int(sunriseHour)))*60), 0, 0, date.Location())
	sunsetTime := time.Date(date.Year(), date.Month(), date.Day(),
		int(sunsetHour), int((sunsetHour-float64(int(sunsetHour)))*60), 0, 0, date.Location())

	return sunriseTime, sunsetTime
}
