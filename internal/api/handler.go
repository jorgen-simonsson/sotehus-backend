package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jorgen-simonsson/sotehus-backend/internal/state"
	"github.com/jorgen-simonsson/sotehus-backend/internal/storage"
)

// Handler holds HTTP handlers
type Handler struct {
	state    *state.Manager
	influxDB *storage.InfluxDBClient
	logger   *slog.Logger
}

// NewHandler creates a new API handler
func NewHandler(state *state.Manager, influxDB *storage.InfluxDBClient, logger *slog.Logger) *Handler {
	return &Handler{
		state:    state,
		influxDB: influxDB,
		logger:   logger,
	}
}

// GetData handles GET /api/data requests
// @Summary Get current energy data
// @Description Returns current status of grid consumption, electricity price, and solar production
// @Tags energy
// @Produce json
// @Success 200 {object} models.APIResponse
// @Router /api/data [get]
func (h *Handler) GetData(w http.ResponseWriter, r *http.Request) {
	response := h.state.GetAPIResponse()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error("Failed to encode response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status string `json:"status" example:"ok"`
}

// HealthCheck handles GET /health requests
// @Summary Health check
// @Description Returns the health status of the API
// @Tags system
// @Produce json
// @Success 200 {object} HealthResponse
// @Router /health [get]
func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(HealthResponse{Status: "ok"})
}

// TimeSeriesResponse represents the response for /api/timeseries
type TimeSeriesResponse struct {
	First string `json:"first" example:"2025-11-01T16:10"`
	Last  string `json:"last" example:"2025-12-07T16:30"`
	Count int64  `json:"count" example:"523847"`
}

// GetTimeSeries handles GET /api/timeseries requests
// @Summary Get time series statistics
// @Description Returns statistics about the historical data stored in InfluxDB
// @Tags energy
// @Produce json
// @Success 200 {object} TimeSeriesResponse
// @Failure 500 {string} string "Failed to query time series data"
// @Failure 503 {string} string "InfluxDB not configured"
// @Router /api/timeseries [get]
func (h *Handler) GetTimeSeries(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if h.influxDB == nil {
		h.logger.Error("InfluxDB client not available")
		http.Error(w, "InfluxDB not configured", http.StatusServiceUnavailable)
		return
	}

	stats, err := h.influxDB.GetTimeSeriesStats()
	if err != nil {
		h.logger.Error("Failed to get time series stats", "error", err)
		http.Error(w, "Failed to query time series data", http.StatusInternalServerError)
		return
	}

	// Convert UTC times from InfluxDB to local time
	localFirst := stats.First.Local()
	localLast := stats.Last.Local()

	response := TimeSeriesResponse{
		First: localFirst.Format("2006-01-02T15:04"),
		Last:  localLast.Format("2006-01-02T15:04"),
		Count: stats.Count,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error("Failed to encode response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}
