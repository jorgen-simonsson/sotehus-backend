package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jorgen-simonsson/sotehus-backend/internal/state"
	"github.com/jorgen-simonsson/sotehus-backend/internal/storage"
	"github.com/jorgen-simonsson/sotehus-backend/internal/storage/params"
)

// Handler holds HTTP handlers
type Handler struct {
	state       *state.Manager
	influxDB    *storage.InfluxDBClient
	paramsStore *params.Store
	logger      *slog.Logger
}

// NewHandler creates a new API handler
func NewHandler(state *state.Manager, influxDB *storage.InfluxDBClient, paramsStore *params.Store, logger *slog.Logger) *Handler {
	return &Handler{
		state:       state,
		influxDB:    influxDB,
		paramsStore: paramsStore,
		logger:      logger,
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
	_ = json.NewEncoder(w).Encode(HealthResponse{Status: "ok"})
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

// GetAllParams handles GET /api/params requests
// @Summary Get all persistent parameters
// @Description Returns all persistent configuration parameters
// @Tags parameters
// @Produce json
// @Success 200 {array} params.PersistentParam
// @Failure 500 {string} string "Failed to get parameters"
// @Router /api/params [get]
func (h *Handler) GetAllParams(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if h.paramsStore == nil {
		http.Error(w, "Parameter store not configured", http.StatusServiceUnavailable)
		return
	}

	allParams, err := h.paramsStore.GetAll()
	if err != nil {
		h.logger.Error("Failed to get parameters", "error", err)
		http.Error(w, "Failed to get parameters", http.StatusInternalServerError)
		return
	}

	if err := json.NewEncoder(w).Encode(allParams); err != nil {
		h.logger.Error("Failed to encode response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// GetParamByKey handles GET /api/params/{key} requests
// @Summary Get a parameter by key
// @Description Returns a single persistent parameter by its key
// @Tags parameters
// @Produce json
// @Param key path string true "Parameter key"
// @Success 200 {object} params.PersistentParam
// @Failure 404 {string} string "Parameter not found"
// @Failure 500 {string} string "Failed to get parameter"
// @Router /api/params/{key} [get]
func (h *Handler) GetParamByKey(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if h.paramsStore == nil {
		http.Error(w, "Parameter store not configured", http.StatusServiceUnavailable)
		return
	}

	key := r.PathValue("key")
	if key == "" {
		http.Error(w, "Missing parameter key", http.StatusBadRequest)
		return
	}

	p, err := h.paramsStore.GetByKey(key)
	if err != nil {
		if errors.Is(err, params.ErrNotFound) {
			http.Error(w, "Parameter not found", http.StatusNotFound)
			return
		}
		h.logger.Error("Failed to get parameter", "key", key, "error", err)
		http.Error(w, "Failed to get parameter", http.StatusInternalServerError)
		return
	}

	if err := json.NewEncoder(w).Encode(p); err != nil {
		h.logger.Error("Failed to encode response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// CreateParam handles POST /api/params requests
// @Summary Create a new parameter
// @Description Creates a new persistent parameter. Returns 409 if the key already exists.
// @Tags parameters
// @Accept json
// @Produce json
// @Param param body params.CreateParamRequest true "Parameter to create"
// @Success 201 {object} params.PersistentParam
// @Failure 400 {string} string "Invalid request body"
// @Failure 409 {string} string "Parameter with this key already exists"
// @Failure 500 {string} string "Failed to create parameter"
// @Router /api/params [post]
func (h *Handler) CreateParam(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if h.paramsStore == nil {
		http.Error(w, "Parameter store not configured", http.StatusServiceUnavailable)
		return
	}

	var req params.CreateParamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Key == "" {
		http.Error(w, "Parameter key is required", http.StatusBadRequest)
		return
	}

	p, err := h.paramsStore.Create(req)
	if err != nil {
		if errors.Is(err, params.ErrDuplicateKey) {
			http.Error(w, "Parameter with this key already exists", http.StatusConflict)
			return
		}
		h.logger.Error("Failed to create parameter", "key", req.Key, "error", err)
		http.Error(w, "Failed to create parameter", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(p); err != nil {
		h.logger.Error("Failed to encode response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// UpdateParam handles PUT /api/params/{key} requests
// @Summary Update an existing parameter
// @Description Updates the description and content of a parameter identified by key
// @Tags parameters
// @Accept json
// @Produce json
// @Param key path string true "Parameter key"
// @Param param body params.UpdateParamRequest true "Updated parameter data"
// @Success 200 {object} params.PersistentParam
// @Failure 400 {string} string "Invalid request body"
// @Failure 404 {string} string "Parameter not found"
// @Failure 500 {string} string "Failed to update parameter"
// @Router /api/params/{key} [put]
func (h *Handler) UpdateParam(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if h.paramsStore == nil {
		http.Error(w, "Parameter store not configured", http.StatusServiceUnavailable)
		return
	}

	key := r.PathValue("key")
	if key == "" {
		http.Error(w, "Missing parameter key", http.StatusBadRequest)
		return
	}

	var req params.UpdateParamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	p, err := h.paramsStore.Update(key, req)
	if err != nil {
		if errors.Is(err, params.ErrNotFound) {
			http.Error(w, "Parameter not found", http.StatusNotFound)
			return
		}
		h.logger.Error("Failed to update parameter", "key", key, "error", err)
		http.Error(w, "Failed to update parameter", http.StatusInternalServerError)
		return
	}

	if err := json.NewEncoder(w).Encode(p); err != nil {
		h.logger.Error("Failed to encode response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}
