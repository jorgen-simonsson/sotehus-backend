package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/jorgen-simonsson/sotehus-backend/internal/state"
	"github.com/jorgen-simonsson/sotehus-backend/internal/storage"
)

// APIVersion is the current version of the API
const APIVersion = "1.2.0"

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

// VersionResponse represents the version response
type VersionResponse struct {
	Version string `json:"version" example:"1.2.0"`
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
	_ = json.NewEncoder(w).Encode(HealthResponse{Status: "ok"})
}

// GetVersion handles GET /api/version requests
// @Summary Get API version
// @Description Returns the current version of the API
// @Tags system
// @Produce json
// @Success 200 {object} VersionResponse
// @Router /api/version [get]
func (h *Handler) GetVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(VersionResponse{Version: APIVersion})
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

// EnergyConsumedResponse represents the response for /api/energy/consumed
type EnergyConsumedResponse struct {
	Start       string  `json:"start" example:"2026-02-01T00:00:00+01:00"`
	Stop        string  `json:"stop" example:"2026-02-21T00:00:00+01:00"`
	ActualStart string  `json:"actual_start" example:"2026-02-01T00:01:23+01:00"`
	ActualStop  string  `json:"actual_stop" example:"2026-02-20T23:59:45+01:00"`
	Consumed    float64 `json:"consumed" example:"523.45"`
	Unit        string  `json:"unit" example:"kWh"`
}

// GetEnergyConsumed handles GET /api/energy/consumed requests
// @Summary Get consumed energy for a period
// @Description Returns the energy consumed in kWh between two timestamps
// @Tags energy
// @Produce json
// @Param start query string true "Start timestamp (RFC3339 format, e.g. 2026-02-01T00:00:00+01:00)"
// @Param stop query string true "Stop timestamp (RFC3339 format, e.g. 2026-02-21T00:00:00+01:00)"
// @Success 200 {object} EnergyConsumedResponse
// @Failure 400 {string} string "Invalid start/stop parameter"
// @Failure 500 {string} string "Failed to calculate energy consumption"
// @Failure 503 {string} string "InfluxDB not configured"
// @Router /api/energy/consumed [get]
func (h *Handler) GetEnergyConsumed(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if h.influxDB == nil {
		h.logger.Error("InfluxDB client not available")
		http.Error(w, "InfluxDB not configured", http.StatusServiceUnavailable)
		return
	}

	// Parse start parameter
	startStr := r.URL.Query().Get("start")
	if startStr == "" {
		http.Error(w, "Missing required parameter: start", http.StatusBadRequest)
		return
	}
	startTime, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		h.logger.Error("Invalid start parameter", "value", startStr, "error", err)
		http.Error(w, "Invalid start parameter. Use RFC3339 format (e.g. 2026-02-01T00:00:00+01:00)", http.StatusBadRequest)
		return
	}

	// Parse stop parameter
	stopStr := r.URL.Query().Get("stop")
	if stopStr == "" {
		http.Error(w, "Missing required parameter: stop", http.StatusBadRequest)
		return
	}
	stopTime, err := time.Parse(time.RFC3339, stopStr)
	if err != nil {
		h.logger.Error("Invalid stop parameter", "value", stopStr, "error", err)
		http.Error(w, "Invalid stop parameter. Use RFC3339 format (e.g. 2026-02-21T00:00:00+01:00)", http.StatusBadRequest)
		return
	}

	// Validate that stop is after start
	if !stopTime.After(startTime) {
		http.Error(w, "Stop timestamp must be after start timestamp", http.StatusBadRequest)
		return
	}

	// Get consumed energy from InfluxDB
	consumed, actualStart, actualStop, err := h.influxDB.GetConsumedEnergy(startTime, stopTime)
	if err != nil {
		h.logger.Error("Failed to get consumed energy", "start", startStr, "stop", stopStr, "error", err)
		http.Error(w, "Failed to calculate energy consumption: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := EnergyConsumedResponse{
		Start:       startTime.Format(time.RFC3339),
		Stop:        stopTime.Format(time.RFC3339),
		ActualStart: actualStart.Format(time.RFC3339),
		ActualStop:  actualStop.Format(time.RFC3339),
		Consumed:    consumed,
		Unit:        "kWh",
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error("Failed to encode response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// EnergySoldResponse represents the response for /api/energy/sold
type EnergySoldResponse struct {
	Start       string  `json:"start" example:"2026-02-01T00:00:00+01:00"`
	Stop        string  `json:"stop" example:"2026-02-21T00:00:00+01:00"`
	ActualStart string  `json:"actual_start" example:"2026-02-01T00:01:23+01:00"`
	ActualStop  string  `json:"actual_stop" example:"2026-02-20T23:59:45+01:00"`
	Sold        float64 `json:"sold" example:"123.45"`
	Unit        string  `json:"unit" example:"kWh"`
}

// GetEnergySold handles GET /api/energy/sold requests
// @Summary Get sold energy for a period
// @Description Returns the energy sold in kWh between two timestamps
// @Tags energy
// @Produce json
// @Param start query string true "Start timestamp (RFC3339 format, e.g. 2026-02-01T00:00:00+01:00)"
// @Param stop query string true "Stop timestamp (RFC3339 format, e.g. 2026-02-21T00:00:00+01:00)"
// @Success 200 {object} EnergySoldResponse
// @Failure 400 {string} string "Invalid start/stop parameter"
// @Failure 500 {string} string "Failed to calculate energy sold"
// @Failure 503 {string} string "InfluxDB not configured"
// @Router /api/energy/sold [get]
func (h *Handler) GetEnergySold(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if h.influxDB == nil {
		h.logger.Error("InfluxDB client not available")
		http.Error(w, "InfluxDB not configured", http.StatusServiceUnavailable)
		return
	}

	// Parse start parameter
	startStr := r.URL.Query().Get("start")
	if startStr == "" {
		http.Error(w, "Missing required parameter: start", http.StatusBadRequest)
		return
	}
	startTime, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		h.logger.Error("Invalid start parameter", "value", startStr, "error", err)
		http.Error(w, "Invalid start parameter. Use RFC3339 format (e.g. 2026-02-01T00:00:00+01:00)", http.StatusBadRequest)
		return
	}

	// Parse stop parameter
	stopStr := r.URL.Query().Get("stop")
	if stopStr == "" {
		http.Error(w, "Missing required parameter: stop", http.StatusBadRequest)
		return
	}
	stopTime, err := time.Parse(time.RFC3339, stopStr)
	if err != nil {
		h.logger.Error("Invalid stop parameter", "value", stopStr, "error", err)
		http.Error(w, "Invalid stop parameter. Use RFC3339 format (e.g. 2026-02-21T00:00:00+01:00)", http.StatusBadRequest)
		return
	}

	// Validate that stop is after start
	if !stopTime.After(startTime) {
		http.Error(w, "Stop timestamp must be after start timestamp", http.StatusBadRequest)
		return
	}

	// Get sold energy from InfluxDB
	sold, actualStart, actualStop, err := h.influxDB.GetSoldEnergy(startTime, stopTime)
	if err != nil {
		h.logger.Error("Failed to get sold energy", "start", startStr, "stop", stopStr, "error", err)
		http.Error(w, "Failed to calculate energy sold: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := EnergySoldResponse{
		Start:       startTime.Format(time.RFC3339),
		Stop:        stopTime.Format(time.RFC3339),
		ActualStart: actualStart.Format(time.RFC3339),
		ActualStop:  actualStop.Format(time.RFC3339),
		Sold:        sold,
		Unit:        "kWh",
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error("Failed to encode response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}
