package price

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jorgen-simonsson/sotehus-backend/internal/config"
	"github.com/jorgen-simonsson/sotehus-backend/internal/models"
	"github.com/jorgen-simonsson/sotehus-backend/internal/state"
	"github.com/jorgen-simonsson/sotehus-backend/internal/storage"
)

const (
	baseURL        = "https://www.elprisetjustnu.se/api/v1/prices"
	updateInterval = 15 * time.Minute // Check every 15 minutes to match price periods
)

// Service handles fetching spot prices from elprisetjustnu.se
type Service struct {
	region   string
	state    *state.Manager
	influxDB *storage.InfluxDBClient
	logger   *slog.Logger
	client   *http.Client
}

// NewService creates a new price service
func NewService(cfg *config.Config, state *state.Manager, influxDB *storage.InfluxDBClient, logger *slog.Logger) *Service {
	return &Service{
		region:   cfg.SpotPriceRegion,
		state:    state,
		influxDB: influxDB,
		logger:   logger,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Start begins the price fetching loop
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Starting price service", "region", s.region)

	// Fetch immediately on startup
	s.fetchAndUpdate()

	ticker := time.NewTicker(updateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Price service shutting down...")
			return nil
		case <-ticker.C:
			s.fetchAndUpdate()
		}
	}
}

func (s *Service) fetchAndUpdate() {
	prices, err := s.fetchPrices()
	if err != nil {
		s.logger.Error("Failed to fetch spot prices", "error", err)
		s.state.UpdatePriceError()
		return
	}

	currentPrice, err := s.getCurrentPrice(prices)
	if err != nil {
		s.logger.Error("Failed to get current price", "error", err)
		s.state.UpdatePriceError()
		return
	}

	s.logger.Info("Updated spot price", "price", currentPrice, "region", s.region)
	s.state.UpdatePrice(currentPrice)

	// Write to InfluxDB
	if s.influxDB != nil {
		if err := s.influxDB.WriteSpotPrice(currentPrice, time.Now()); err != nil {
			s.logger.Warn("Failed to write spot price to InfluxDB", "error", err)
		}
	}
}

func (s *Service) fetchPrices() ([]models.SpotPriceEntry, error) {
	now := time.Now()
	url := fmt.Sprintf("%s/%s/%s_%s.json",
		baseURL,
		now.Format("2006"),
		now.Format("01-02"),
		s.region,
	)

	s.logger.Debug("Fetching spot prices", "url", url)

	resp, err := s.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch prices: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var prices []models.SpotPriceEntry
	if err := json.NewDecoder(resp.Body).Decode(&prices); err != nil {
		return nil, fmt.Errorf("failed to decode prices: %w", err)
	}

	return prices, nil
}

func (s *Service) getCurrentPrice(prices []models.SpotPriceEntry) (float64, error) {
	if len(prices) == 0 {
		return 0, fmt.Errorf("no prices available")
	}

	now := time.Now()
	loc := now.Location()

	for _, entry := range prices {
		// Parse time strings
		startTime, err := time.Parse(time.RFC3339, entry.TimeStart)
		if err != nil {
			s.logger.Warn("Failed to parse time_start", "value", entry.TimeStart, "error", err)
			continue
		}

		endTime, err := time.Parse(time.RFC3339, entry.TimeEnd)
		if err != nil {
			s.logger.Warn("Failed to parse time_end", "value", entry.TimeEnd, "error", err)
			continue
		}

		// Convert to local timezone
		startTime = startTime.In(loc)
		endTime = endTime.In(loc)

		// Check if current time falls within this period
		if now.After(startTime) && now.Before(endTime) || now.Equal(startTime) {
			return entry.SEKPerKWh, nil
		}
	}

	return 0, fmt.Errorf("no matching price found for current time")
}
