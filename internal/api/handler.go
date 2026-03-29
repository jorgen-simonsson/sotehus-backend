package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jorgen-simonsson/sotehus-backend/internal/state"
	"github.com/jorgen-simonsson/sotehus-backend/internal/storage"
	"github.com/jorgen-simonsson/sotehus-backend/internal/storage/params"
	"github.com/shopspring/decimal"
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

// EnergyCostBlock represents a single time block where the spot price was constant
type EnergyCostBlock struct {
	SpotPrice   float64 `json:"spot_price" example:"0.45"`
	AddedPrices float64 `json:"added_prices" example:"0.70"`
	TotalPrice  float64 `json:"total_price" example:"1.15"`
	ConsumedKWh float64 `json:"consumed_kwh" example:"12.34"`
	ProducedKWh float64 `json:"produced_kwh" example:"1.50"`
	Cost        float64 `json:"cost" example:"14.22"`
	Start       string  `json:"start" example:"2026-02-22T00:00:00+01:00"`
	Stop        string  `json:"stop" example:"2026-02-22T01:00:00+01:00"`
}

// EnergyCostResponse represents the response for /api/energy/cost
type EnergyCostResponse struct {
	PeriodStart       string            `json:"period_start" example:"2026-02-22T00:00:00+01:00"`
	PeriodStop        string            `json:"period_stop" example:"2026-02-22T12:00:00+01:00"`
	TotalConsumedKWh  float64           `json:"total_consumed_kwh" example:"156.78"`
	TotalProducedKWh  float64           `json:"total_produced_kwh" example:"23.45"`
	CostBeforeVAT     float64           `json:"cost_before_vat" example:"180.73"`
	VATPercent        float64           `json:"vat_percent" example:"25"`
	ProductionBenefit float64           `json:"production_benefit" example:"5.67"`
	TotalCost         float64           `json:"total_cost" example:"220.24"`
	Unit              string            `json:"unit" example:"SEK"`
	Blocks            []EnergyCostBlock `json:"blocks"`
}

// GetEnergyCost handles GET /api/energy/cost requests
// @Summary Get energy cost for a period
// @Description Calculates the actual energy cost for a period, accounting for changing spot prices.
// @Description Groups records into blocks of constant spot price, computes per-block kWh and cost,
// @Description adds configured price components (transfer, energy tax, dynamic and static additions),
// @Description and applies VAT.
// @Tags energy
// @Produce json
// @Param start query string true "Start timestamp (RFC3339 format, e.g. 2026-02-01T00:00:00+01:00)"
// @Param stop query string true "Stop timestamp (RFC3339 format, e.g. 2026-02-21T00:00:00+01:00)"
// @Success 200 {object} EnergyCostResponse
// @Failure 400 {string} string "Invalid start/stop parameter"
// @Failure 500 {string} string "Failed to calculate energy cost"
// @Failure 503 {string} string "InfluxDB not configured"
// @Router /api/energy/cost [get]
func (h *Handler) GetEnergyCost(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

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

	// Check infrastructure availability
	if h.influxDB == nil {
		h.logger.Error("InfluxDB client not available")
		http.Error(w, "InfluxDB not configured", http.StatusServiceUnavailable)
		return
	}

	if h.paramsStore == nil {
		h.logger.Error("Parameter store not available")
		http.Error(w, "Parameter store not configured", http.StatusServiceUnavailable)
		return
	}

	// Fetch price add-on parameters from SQLite
	addPrices, err := h.fetchAddPrices()
	if err != nil {
		h.logger.Error("Failed to fetch price parameters", "error", err)
		http.Error(w, "Failed to fetch price parameters: "+err.Error(), http.StatusInternalServerError)
		return
	}

	vatPercent, err := h.fetchParamValue("VAT")
	if err != nil {
		h.logger.Error("Failed to fetch VAT parameter", "error", err)
		http.Error(w, "Failed to fetch VAT parameter: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Query InfluxDB for records with spot_price and grid_enery_consumed_ack
	records, err := h.influxDB.GetFieldsInRange(startTime, stopTime)
	if err != nil {
		h.logger.Error("Failed to query energy data", "start", startStr, "stop", stopStr, "error", err)
		http.Error(w, "Failed to query energy data: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// If no records, return zero-cost response
	if len(records) == 0 {
		response := EnergyCostResponse{
			PeriodStart:       startTime.Format(time.RFC3339),
			PeriodStop:        stopTime.Format(time.RFC3339),
			TotalConsumedKWh:  0,
			TotalProducedKWh:  0,
			CostBeforeVAT:     0,
			VATPercent:        vatPercent.InexactFloat64(),
			ProductionBenefit: 0,
			TotalCost:         0,
			Unit:              "SEK",
			Blocks:            []EnergyCostBlock{},
		}

		if err := json.NewEncoder(w).Encode(response); err != nil {
			h.logger.Error("Failed to encode response", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	// Fetch production benefit parameters
	gridBenefit, err := h.fetchParamValue("grid_benefit")
	if err != nil {
		h.logger.Error("Failed to fetch grid_benefit parameter", "error", err)
		http.Error(w, "Failed to fetch grid_benefit parameter: "+err.Error(), http.StatusInternalServerError)
		return
	}

	eonAdded, err := h.fetchParamValue("eon_added")
	if err != nil {
		h.logger.Error("Failed to fetch eon_added parameter", "error", err)
		http.Error(w, "Failed to fetch eon_added parameter: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Group records into blocks of constant spot_price and calculate costs
	blocks := h.calculateCostBlocks(records, addPrices, gridBenefit, eonAdded)

	// Sum totals using decimal
	totalConsumed := decimal.Zero
	totalProduced := decimal.Zero
	costBeforeVAT := decimal.Zero
	productionBenefit := decimal.Zero
	for _, b := range blocks {
		totalConsumed = totalConsumed.Add(decimal.NewFromFloat(b.ConsumedKWh))
		totalProduced = totalProduced.Add(decimal.NewFromFloat(b.ProducedKWh))

		// Compute consumption cost from block fields for VAT calculation
		// (b.Cost is net and cannot be used directly since VAT applies only to consumption)
		consumptionCost := decimal.NewFromFloat(b.ConsumedKWh).Mul(decimal.NewFromFloat(b.TotalPrice))
		costBeforeVAT = costBeforeVAT.Add(consumptionCost)

		// Production benefit per block: (spot_price + grid_benefit + eon_added) * produced_kwh
		blockProduced := decimal.NewFromFloat(b.ProducedKWh)
		blockSpot := decimal.NewFromFloat(b.SpotPrice)
		blockDecrease := blockSpot.Add(gridBenefit).Add(eonAdded).Mul(blockProduced)
		productionBenefit = productionBenefit.Add(blockDecrease)
	}

	vatMultiplier := decimal.NewFromInt(1).Add(vatPercent.Div(decimal.NewFromInt(100)))
	totalCost := costBeforeVAT.Mul(vatMultiplier).Sub(productionBenefit).Round(2)

	response := EnergyCostResponse{
		PeriodStart:       startTime.Format(time.RFC3339),
		PeriodStop:        stopTime.Format(time.RFC3339),
		TotalConsumedKWh:  totalConsumed.Round(2).InexactFloat64(),
		TotalProducedKWh:  totalProduced.Round(2).InexactFloat64(),
		CostBeforeVAT:     costBeforeVAT.Round(2).InexactFloat64(),
		VATPercent:        vatPercent.InexactFloat64(),
		ProductionBenefit: productionBenefit.Round(2).InexactFloat64(),
		TotalCost:         totalCost.InexactFloat64(),
		Unit:              "SEK",
		Blocks:            blocks,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error("Failed to encode response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// fetchParamValue retrieves a single parameter's numeric value from the store.
func (h *Handler) fetchParamValue(key string) (decimal.Decimal, error) {
	p, err := h.paramsStore.GetByKey(key)
	if err != nil {
		return decimal.Zero, fmt.Errorf("parameter %q not found: %w", key, err)
	}
	val, err := params.ParseContentValue(p.Content)
	if err != nil {
		return decimal.Zero, fmt.Errorf("parameter %q has invalid content: %w", key, err)
	}
	return val, nil
}

// fetchAddPrices retrieves and sums the four add-on price parameters.
func (h *Handler) fetchAddPrices() (decimal.Decimal, error) {
	keys := []string{"TransferAddPrice", "EnergyTaxAddPrice", "DynamicAddPrice", "StaticAddPrice"}
	total := decimal.Zero
	for _, key := range keys {
		val, err := h.fetchParamValue(key)
		if err != nil {
			return decimal.Zero, err
		}
		total = total.Add(val)
	}
	return total, nil
}

// calculateCostBlocks groups time-ordered records into blocks of constant spot_price
// and calculates the energy consumed, produced, and cost for each block.
// All arithmetic is performed with decimal.Decimal; final values are rounded to 2 decimal places.
func (h *Handler) calculateCostBlocks(records []storage.TimeFieldRecord, addPrices, gridBenefit, eonAdded decimal.Decimal) []EnergyCostBlock {
	if len(records) == 0 {
		return nil
	}

	var blocks []EnergyCostBlock

	blockStart := 0
	currentPrice := records[0].SpotPrice

	for i := 1; i <= len(records); i++ {
		// End of data or price changed — close current block
		if i == len(records) || !records[i].SpotPrice.Equal(currentPrice) {
			firstRec := records[blockStart]
			lastRec := records[i-1]

			consumedKWh := lastRec.ConsumedAck.Sub(firstRec.ConsumedAck)
			producedKWh := lastRec.SoldAck.Sub(firstRec.SoldAck)
			totalPrice := currentPrice.Add(addPrices)
			consumptionCost := consumedKWh.Mul(totalPrice)
			blockBenefit := producedKWh.Mul(currentPrice.Add(gridBenefit).Add(eonAdded))
			cost := consumptionCost.Sub(blockBenefit)

			blocks = append(blocks, EnergyCostBlock{
				SpotPrice:   currentPrice.Round(2).InexactFloat64(),
				AddedPrices: addPrices.Round(2).InexactFloat64(),
				TotalPrice:  totalPrice.Round(2).InexactFloat64(),
				ConsumedKWh: consumedKWh.Round(2).InexactFloat64(),
				ProducedKWh: producedKWh.Round(2).InexactFloat64(),
				Cost:        cost.Round(2).InexactFloat64(),
				Start:       firstRec.Time.Format(time.RFC3339),
				Stop:        lastRec.Time.Format(time.RFC3339),
			})

			if i < len(records) {
				blockStart = i
				currentPrice = records[i].SpotPrice
			}
		}
	}

	return blocks
}
