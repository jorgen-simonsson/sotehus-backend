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

// HealthCheck handles GET /health requests
func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// TimeSeriesResponse represents the response for /api/timeseries
type TimeSeriesResponse struct {
	First string `json:"first"`
	Last  string `json:"last"`
	Count int64  `json:"count"`
}

// GetTimeSeries handles GET /api/timeseries requests
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
