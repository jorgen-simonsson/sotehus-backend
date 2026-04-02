package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jorgen-simonsson/sotehus-backend/internal/models"
	"github.com/jorgen-simonsson/sotehus-backend/internal/state"
	"github.com/jorgen-simonsson/sotehus-backend/internal/storage"
	"github.com/jorgen-simonsson/sotehus-backend/internal/storage/params"
	"github.com/shopspring/decimal"
)

func TestNewHandler(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mgr := state.NewManager()

	handler := NewHandler(mgr, nil, nil, logger)

	if handler == nil {
		t.Fatal("NewHandler returned nil")
	}
	if handler.state != mgr {
		t.Error("Handler state not set correctly")
	}
	if handler.logger != logger {
		t.Error("Handler logger not set correctly")
	}
}

func TestGetData(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()

	// Update state with test data
	mgr.UpdateGrid(1234.5, time.Now())
	mgr.UpdatePrice(0.89)
	mgr.UpdateSolar(500.0)

	handler := NewHandler(mgr, nil, nil, logger)

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	rec := httptest.NewRecorder()

	handler.GetData(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusOK)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
	}

	cors := rec.Header().Get("Access-Control-Allow-Origin")
	if cors != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", cors, "*")
	}

	var response models.APIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if !response.Grid.Valid {
		t.Error("Grid.Valid should be true")
	}
	if response.Grid.Power != 1234.5 {
		t.Errorf("Grid.Power = %f, want %f", response.Grid.Power, 1234.5)
	}
	if !response.Price.Valid {
		t.Error("Price.Valid should be true")
	}
	if response.Price.Price != 0.89 {
		t.Errorf("Price.Price = %f, want %f", response.Price.Price, 0.89)
	}
	if !response.Solar.Valid {
		t.Error("Solar.Valid should be true")
	}
	if response.Solar.Power != 500.0 {
		t.Errorf("Solar.Power = %f, want %f", response.Solar.Power, 500.0)
	}
}

func TestGetDataWithErrors(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()

	// Set error states
	mgr.UpdateGridError("Connection timeout")
	mgr.UpdatePriceError()
	mgr.UpdateSolarError("No sun")

	handler := NewHandler(mgr, nil, nil, logger)

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	rec := httptest.NewRecorder()

	handler.GetData(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusOK)
	}

	var response models.APIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.Grid.Valid {
		t.Error("Grid.Valid should be false")
	}
	if response.Grid.Message != "Connection timeout" {
		t.Errorf("Grid.Message = %q, want %q", response.Grid.Message, "Connection timeout")
	}
	if response.Price.Valid {
		t.Error("Price.Valid should be false")
	}
	if response.Solar.Valid {
		t.Error("Solar.Valid should be false")
	}
	if response.Solar.Message != "No sun" {
		t.Errorf("Solar.Message = %q, want %q", response.Solar.Message, "No sun")
	}
}

func TestHealthCheck(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()
	handler := NewHandler(mgr, nil, nil, logger)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.HealthCheck(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusOK)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
	}

	var response map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response["status"] != "ok" {
		t.Errorf("status = %q, want %q", response["status"], "ok")
	}
}

func TestGetTimeSeriesNoInfluxDB(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()
	handler := NewHandler(mgr, nil, nil, logger) // nil InfluxDB client

	req := httptest.NewRequest(http.MethodGet, "/api/timeseries", nil)
	rec := httptest.NewRecorder()

	handler.GetTimeSeries(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestSetupRoutes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()

	mux := NewRouter(mgr, nil, nil, logger)

	if mux == nil {
		t.Fatal("NewRouter returned nil")
	}

	// Test that routes are registered by making requests
	tests := []struct {
		path   string
		method string
	}{
		{"/api/data", http.MethodGet},
		{"/api/timeseries", http.MethodGet},
		{"/health", http.MethodGet},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()

			mux.ServeHTTP(rec, req)

			// Should not be 404 (route exists)
			if rec.Code == http.StatusNotFound {
				t.Errorf("Route %s not found", tt.path)
			}
		})
	}
}

func TestCORSPreflight(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()
	mux := NewRouter(mgr, nil, nil, logger)

	req := httptest.NewRequest(http.MethodOptions, "/api/data", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	// OPTIONS should return 200 OK
	if rec.Code != http.StatusOK {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusOK)
	}

	// Check CORS headers
	if cors := rec.Header().Get("Access-Control-Allow-Origin"); cors != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", cors, "*")
	}
}

// --- Parameter integration tests ---

func newTestParamsStore(t *testing.T) *params.Store {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	store, err := params.NewStore(":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create params store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func newTestRouter(t *testing.T) (http.Handler, *params.Store) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()
	store := newTestParamsStore(t)
	router := NewRouter(mgr, nil, store, logger)
	return router, store
}

func TestGetAllParams(t *testing.T) {
	router, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/params", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status code = %d, want %d", rec.Code, http.StatusOK)
	}

	var result []params.PersistentParam
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if len(result) != 9 {
		t.Fatalf("Expected 9 default params, got %d", len(result))
	}

	keys := make(map[string]bool)
	for _, p := range result {
		keys[p.Key] = true
	}
	if !keys["DynamicAddPrice"] {
		t.Error("Missing default param DynamicAddPrice")
	}
	if !keys["StaticAddPrice"] {
		t.Error("Missing default param StaticAddPrice")
	}
	if !keys["TransferAddPrice"] {
		t.Error("Missing default param TransferAddPrice")
	}
	if !keys["EnergyTaxAddPrice"] {
		t.Error("Missing default param EnergyTaxAddPrice")
	}
	if !keys["VAT"] {
		t.Error("Missing default param VAT")
	}
}

func TestGetParamByKey_Found(t *testing.T) {
	router, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/params/DynamicAddPrice", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status code = %d, want %d", rec.Code, http.StatusOK)
	}

	var result params.PersistentParam
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if result.Key != "DynamicAddPrice" {
		t.Errorf("Key = %q, want %q", result.Key, "DynamicAddPrice")
	}
	if result.Content != `{"value": 0.0442}` {
		t.Errorf("Content = %q, want %q", result.Content, `{"value": 0.0442}`)
	}
}

func TestGetParamByKey_NotFound(t *testing.T) {
	router, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/params/NonExistent", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestCreateParam_Success(t *testing.T) {
	router, _ := newTestRouter(t)

	body := `{"key": "TestParam", "description": "A test parameter", "content": "{\"value\": 42}"}`
	req := httptest.NewRequest(http.MethodPost, "/api/params", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("Status code = %d, want %d. Body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var result params.PersistentParam
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if result.Key != "TestParam" {
		t.Errorf("Key = %q, want %q", result.Key, "TestParam")
	}
	if result.ID == "" {
		t.Error("Expected non-empty ID")
	}
	if result.Changed.IsZero() {
		t.Error("Expected non-zero Changed timestamp")
	}

	// Verify it shows up in GetAll
	reqAll := httptest.NewRequest(http.MethodGet, "/api/params", nil)
	recAll := httptest.NewRecorder()
	router.ServeHTTP(recAll, reqAll)

	var allParams []params.PersistentParam
	if err := json.Unmarshal(recAll.Body.Bytes(), &allParams); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	if len(allParams) != 10 {
		t.Errorf("Expected 10 params after create, got %d", len(allParams))
	}
}

func TestCreateParam_DuplicateKey(t *testing.T) {
	router, _ := newTestRouter(t)

	body := `{"key": "DynamicAddPrice", "description": "Duplicate", "content": "{\"value\": 99}"}`
	req := httptest.NewRequest(http.MethodPost, "/api/params", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestCreateParam_MissingKey(t *testing.T) {
	router, _ := newTestRouter(t)

	body := `{"description": "No key", "content": "{}"}`
	req := httptest.NewRequest(http.MethodPost, "/api/params", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateParam_InvalidBody(t *testing.T) {
	router, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/api/params", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdateParam_Success(t *testing.T) {
	router, _ := newTestRouter(t)

	body := `{"description": "Updated description", "content": "{\"value\": 0.10}"}`
	req := httptest.NewRequest(http.MethodPut, "/api/params/DynamicAddPrice", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status code = %d, want %d. Body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var result params.PersistentParam
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if result.Key != "DynamicAddPrice" {
		t.Errorf("Key = %q, want %q", result.Key, "DynamicAddPrice")
	}
	if result.Description != "Updated description" {
		t.Errorf("Description = %q, want %q", result.Description, "Updated description")
	}
	if result.Content != `{"value": 0.10}` {
		t.Errorf("Content = %q, want %q", result.Content, `{"value": 0.10}`)
	}

	// Verify update persisted via GET
	reqGet := httptest.NewRequest(http.MethodGet, "/api/params/DynamicAddPrice", nil)
	recGet := httptest.NewRecorder()
	router.ServeHTTP(recGet, reqGet)

	var fetched params.PersistentParam
	if err := json.Unmarshal(recGet.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	if fetched.Content != `{"value": 0.10}` {
		t.Errorf("Persisted Content = %q, want %q", fetched.Content, `{"value": 0.10}`)
	}
}

func TestUpdateParam_NotFound(t *testing.T) {
	router, _ := newTestRouter(t)

	body := `{"description": "Does not matter", "content": "{}"}`
	req := httptest.NewRequest(http.MethodPut, "/api/params/NonExistent", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestUpdateParam_InvalidBody(t *testing.T) {
	router, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodPut, "/api/params/DynamicAddPrice", strings.NewReader("bad"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestParamEndpoints_NoStore(t *testing.T) {
	// When paramsStore is nil, all param endpoints should return 503
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()
	handler := NewHandler(mgr, nil, nil, logger)

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"GetAll", http.MethodGet, "/api/params", ""},
		{"GetByKey", http.MethodGet, "/api/params/DynamicAddPrice", ""},
		{"Create", http.MethodPost, "/api/params", `{"key":"x","content":"{}"}`},
		{"Update", http.MethodPut, "/api/params/DynamicAddPrice", `{"content":"{}"}`},
	}

	handlers := map[string]http.HandlerFunc{
		"GetAll":   handler.GetAllParams,
		"GetByKey": handler.GetParamByKey,
		"Create":   handler.CreateParam,
		"Update":   handler.UpdateParam,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			if tt.body != "" {
				req = httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tt.method, tt.path, nil)
			}
			rec := httptest.NewRecorder()

			handlers[tt.name](rec, req)

			if rec.Code != http.StatusServiceUnavailable {
				t.Errorf("Status code = %d, want %d", rec.Code, http.StatusServiceUnavailable)
			}
		})
	}
}

func TestParamRoutes_Registered(t *testing.T) {
	router, _ := newTestRouter(t)

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/params"},
		{http.MethodGet, "/api/params/DynamicAddPrice"},
		{http.MethodPost, "/api/params"},
		{http.MethodPut, "/api/params/DynamicAddPrice"},
	}

	for _, tt := range routes {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			var req *http.Request
			if tt.method == http.MethodPost || tt.method == http.MethodPut {
				req = httptest.NewRequest(tt.method, tt.path, strings.NewReader(`{"key":"x","description":"d","content":"{}"}`))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tt.method, tt.path, nil)
			}
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code == http.StatusNotFound || rec.Code == http.StatusMethodNotAllowed {
				t.Errorf("Route %s %s not registered", tt.method, tt.path)
			}
		})
	}
}

func TestCreateThenGetThenUpdateFlow(t *testing.T) {
	router, _ := newTestRouter(t)

	// Step 1: Create a new parameter
	createBody := `{"key": "FlowTest", "description": "Flow test param", "content": "{\"step\": 1}"}`
	reqCreate := httptest.NewRequest(http.MethodPost, "/api/params", strings.NewReader(createBody))
	reqCreate.Header.Set("Content-Type", "application/json")
	recCreate := httptest.NewRecorder()
	router.ServeHTTP(recCreate, reqCreate)

	if recCreate.Code != http.StatusCreated {
		t.Fatalf("Create: status = %d, want %d", recCreate.Code, http.StatusCreated)
	}

	var created params.PersistentParam
	if err := json.Unmarshal(recCreate.Body.Bytes(), &created); err != nil {
		t.Fatalf("Create: failed to unmarshal: %v", err)
	}

	// Step 2: Get it by key
	reqGet := httptest.NewRequest(http.MethodGet, "/api/params/FlowTest", nil)
	recGet := httptest.NewRecorder()
	router.ServeHTTP(recGet, reqGet)

	if recGet.Code != http.StatusOK {
		t.Fatalf("Get: status = %d, want %d", recGet.Code, http.StatusOK)
	}

	var fetched params.PersistentParam
	if err := json.Unmarshal(recGet.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("Get: failed to unmarshal: %v", err)
	}
	if fetched.ID != created.ID {
		t.Errorf("Get: ID = %q, want %q", fetched.ID, created.ID)
	}
	if fetched.Content != `{"step": 1}` {
		t.Errorf("Get: Content = %q, want %q", fetched.Content, `{"step": 1}`)
	}

	// Step 3: Update it
	updateBody := `{"description": "Updated flow test", "content": "{\"step\": 2}"}`
	reqUpdate := httptest.NewRequest(http.MethodPut, "/api/params/FlowTest", strings.NewReader(updateBody))
	reqUpdate.Header.Set("Content-Type", "application/json")
	recUpdate := httptest.NewRecorder()
	router.ServeHTTP(recUpdate, reqUpdate)

	if recUpdate.Code != http.StatusOK {
		t.Fatalf("Update: status = %d, want %d", recUpdate.Code, http.StatusOK)
	}

	var updated params.PersistentParam
	if err := json.Unmarshal(recUpdate.Body.Bytes(), &updated); err != nil {
		t.Fatalf("Update: failed to unmarshal: %v", err)
	}
	if updated.ID != created.ID {
		t.Error("Update: ID should not change")
	}
	if updated.Description != "Updated flow test" {
		t.Errorf("Update: Description = %q, want %q", updated.Description, "Updated flow test")
	}
	if updated.Content != `{"step": 2}` {
		t.Errorf("Update: Content = %q, want %q", updated.Content, `{"step": 2}`)
	}
	if !updated.Changed.After(created.Changed) && updated.Changed != created.Changed {
		t.Error("Update: Changed timestamp should be updated")
	}

	// Step 4: Verify update persisted
	reqVerify := httptest.NewRequest(http.MethodGet, "/api/params/FlowTest", nil)
	recVerify := httptest.NewRecorder()
	router.ServeHTTP(recVerify, reqVerify)

	var verified params.PersistentParam
	if err := json.Unmarshal(recVerify.Body.Bytes(), &verified); err != nil {
		t.Fatalf("Verify: failed to unmarshal: %v", err)
	}
	if verified.Content != `{"step": 2}` {
		t.Errorf("Verify: Content = %q, want %q", verified.Content, `{"step": 2}`)
	}

	// Step 5: Try to create a duplicate
	reqDup := httptest.NewRequest(http.MethodPost, "/api/params", strings.NewReader(createBody))
	reqDup.Header.Set("Content-Type", "application/json")
	recDup := httptest.NewRecorder()
	router.ServeHTTP(recDup, reqDup)

	if recDup.Code != http.StatusConflict {
		t.Errorf("Duplicate create: status = %d, want %d", recDup.Code, http.StatusConflict)
	}
}

// --- Energy cost endpoint tests ---

func TestGetEnergyCost_NoInfluxDB(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()
	store := newTestParamsStore(t)
	handler := NewHandler(mgr, nil, store, logger)

	req := httptest.NewRequest(http.MethodGet, "/api/energy/cost?start=2026-02-01T00:00:00Z&stop=2026-02-02T00:00:00Z", nil)
	rec := httptest.NewRecorder()

	handler.GetEnergyCost(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestGetEnergyCost_NoParamsStore(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()
	// handler with nil paramsStore and nil influxDB - influxDB is checked first
	handler := NewHandler(mgr, nil, nil, logger)

	req := httptest.NewRequest(http.MethodGet, "/api/energy/cost?start=2026-02-01T00:00:00Z&stop=2026-02-02T00:00:00Z", nil)
	rec := httptest.NewRecorder()

	handler.GetEnergyCost(rec, req)

	// InfluxDB nil is checked first, so this returns 503 for InfluxDB
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestGetEnergyCost_MissingStart(t *testing.T) {
	router, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/energy/cost?stop=2026-02-02T00:00:00Z", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetEnergyCost_MissingStop(t *testing.T) {
	router, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/energy/cost?start=2026-02-01T00:00:00Z", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetEnergyCost_InvalidStartFormat(t *testing.T) {
	router, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/energy/cost?start=bad&stop=2026-02-02T00:00:00Z", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetEnergyCost_InvalidStopFormat(t *testing.T) {
	router, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/energy/cost?start=2026-02-01T00:00:00Z&stop=bad", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetEnergyCost_StopBeforeStart(t *testing.T) {
	router, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/energy/cost?start=2026-02-02T00:00:00Z&stop=2026-02-01T00:00:00Z", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetEnergyCost_RouteRegistered(t *testing.T) {
	router, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/energy/cost?start=2026-02-01T00:00:00Z&stop=2026-02-02T00:00:00Z", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	// Should not be 404 (route exists — will be 503 because InfluxDB is nil)
	if rec.Code == http.StatusNotFound {
		t.Error("Route /api/energy/cost not registered")
	}
	// With nil InfluxDB, expect 503
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Status code = %d, want %d (nil InfluxDB)", rec.Code, http.StatusServiceUnavailable)
	}
}

// --- calculateCostBlocks unit tests ---

func TestCalculateCostBlocks_Empty(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()
	handler := NewHandler(mgr, nil, nil, logger)

	blocks := handler.calculateCostBlocks(nil, decimal.NewFromFloat(0.5), decimal.Zero, decimal.Zero)
	if blocks != nil {
		t.Errorf("Expected nil for empty records, got %d blocks", len(blocks))
	}
}

func TestCalculateCostBlocks_SinglePrice(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()
	handler := NewHandler(mgr, nil, nil, logger)

	now := time.Now()
	records := []storage.TimeFieldRecord{
		{Time: now, SpotPrice: decimal.NewFromFloat(0.50), ConsumedAck: decimal.NewFromFloat(100.0), SoldAck: decimal.NewFromFloat(10.0)},
		{Time: now.Add(1 * time.Minute), SpotPrice: decimal.NewFromFloat(0.50), ConsumedAck: decimal.NewFromFloat(100.5), SoldAck: decimal.NewFromFloat(10.2)},
		{Time: now.Add(2 * time.Minute), SpotPrice: decimal.NewFromFloat(0.50), ConsumedAck: decimal.NewFromFloat(101.0), SoldAck: decimal.NewFromFloat(10.5)},
	}

	addPrices := decimal.NewFromFloat(0.70) // sum of all add prices
	blocks := handler.calculateCostBlocks(records, addPrices, decimal.Zero, decimal.Zero)

	if len(blocks) != 1 {
		t.Fatalf("Expected 1 block, got %d", len(blocks))
	}

	b := blocks[0]
	// 101.0 - 100.0 = 1.0 kWh consumed, 0.5 kWh produced
	// price = 0.50 + 0.70 = 1.20
	// net cost = 1.0 * 1.20 - 0.5 * 0.50 = 0.95
	if b.SpotPrice != 0.50 {
		t.Errorf("SpotPrice = %f, want %f", b.SpotPrice, 0.50)
	}
	if b.ConsumedKWh != 1.0 {
		t.Errorf("ConsumedKWh = %f, want %f", b.ConsumedKWh, 1.0)
	}
	if b.ProducedKWh != 0.5 {
		t.Errorf("ProducedKWh = %f, want %f", b.ProducedKWh, 0.5)
	}
	if b.Cost != 0.95 {
		t.Errorf("Cost = %f, want %f", b.Cost, 0.95)
	}
	if b.TotalPrice != 1.2 {
		t.Errorf("TotalPrice = %f, want %f", b.TotalPrice, 1.2)
	}
}

func TestCalculateCostBlocks_MultiplePrices(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()
	handler := NewHandler(mgr, nil, nil, logger)

	now := time.Now()
	records := []storage.TimeFieldRecord{
		// Block 1: price 0.50, 3 records
		{Time: now, SpotPrice: decimal.NewFromFloat(0.50), ConsumedAck: decimal.NewFromFloat(100.0), SoldAck: decimal.NewFromFloat(10.0)},
		{Time: now.Add(1 * time.Minute), SpotPrice: decimal.NewFromFloat(0.50), ConsumedAck: decimal.NewFromFloat(102.0), SoldAck: decimal.NewFromFloat(10.5)},
		{Time: now.Add(2 * time.Minute), SpotPrice: decimal.NewFromFloat(0.50), ConsumedAck: decimal.NewFromFloat(105.0), SoldAck: decimal.NewFromFloat(11.0)},
		// Block 2: price 0.80, 2 records
		{Time: now.Add(3 * time.Minute), SpotPrice: decimal.NewFromFloat(0.80), ConsumedAck: decimal.NewFromFloat(105.5), SoldAck: decimal.NewFromFloat(11.0)},
		{Time: now.Add(4 * time.Minute), SpotPrice: decimal.NewFromFloat(0.80), ConsumedAck: decimal.NewFromFloat(108.0), SoldAck: decimal.NewFromFloat(11.5)},
		// Block 3: price 0.30, 2 records
		{Time: now.Add(5 * time.Minute), SpotPrice: decimal.NewFromFloat(0.30), ConsumedAck: decimal.NewFromFloat(108.5), SoldAck: decimal.NewFromFloat(11.5)},
		{Time: now.Add(6 * time.Minute), SpotPrice: decimal.NewFromFloat(0.30), ConsumedAck: decimal.NewFromFloat(110.0), SoldAck: decimal.NewFromFloat(12.0)},
	}

	addPrices := decimal.NewFromFloat(0.50)
	blocks := handler.calculateCostBlocks(records, addPrices, decimal.Zero, decimal.Zero)

	if len(blocks) != 3 {
		t.Fatalf("Expected 3 blocks, got %d", len(blocks))
	}

	// Block 1: consumed=5.0, produced=1.0, spot=0.50, totalPrice=1.00
	// net = 5.0*1.00 - 1.0*0.50 = 4.50
	if blocks[0].ConsumedKWh != 5.0 {
		t.Errorf("Block 0 ConsumedKWh = %f, want %f", blocks[0].ConsumedKWh, 5.0)
	}
	if blocks[0].ProducedKWh != 1.0 {
		t.Errorf("Block 0 ProducedKWh = %f, want %f", blocks[0].ProducedKWh, 1.0)
	}
	if blocks[0].Cost != 4.5 {
		t.Errorf("Block 0 Cost = %f, want %f", blocks[0].Cost, 4.5)
	}

	// Block 2: consumed=2.5, produced=0.5, spot=0.80, totalPrice=1.30
	// net = 2.5*1.30 - 0.5*0.80 = 2.85
	if blocks[1].ConsumedKWh != 2.5 {
		t.Errorf("Block 1 ConsumedKWh = %f, want %f", blocks[1].ConsumedKWh, 2.5)
	}
	if blocks[1].ProducedKWh != 0.5 {
		t.Errorf("Block 1 ProducedKWh = %f, want %f", blocks[1].ProducedKWh, 0.5)
	}
	if blocks[1].Cost != 2.85 {
		t.Errorf("Block 1 Cost = %f, want %f", blocks[1].Cost, 2.85)
	}

	// Block 3: consumed=1.5, produced=0.5, spot=0.30, totalPrice=0.80
	// net = 1.5*0.80 - 0.5*0.30 = 1.05
	if blocks[2].ConsumedKWh != 1.5 {
		t.Errorf("Block 2 ConsumedKWh = %f, want %f", blocks[2].ConsumedKWh, 1.5)
	}
	if blocks[2].ProducedKWh != 0.5 {
		t.Errorf("Block 2 ProducedKWh = %f, want %f", blocks[2].ProducedKWh, 0.5)
	}
	if blocks[2].Cost != 1.05 {
		t.Errorf("Block 2 Cost = %f, want %f", blocks[2].Cost, 1.05)
	}
}

func TestCalculateCostBlocks_SingleRecord(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()
	handler := NewHandler(mgr, nil, nil, logger)

	records := []storage.TimeFieldRecord{
		{Time: time.Now(), SpotPrice: decimal.NewFromFloat(0.50), ConsumedAck: decimal.NewFromFloat(100.0), SoldAck: decimal.NewFromFloat(10.0)},
	}

	blocks := handler.calculateCostBlocks(records, decimal.NewFromFloat(0.70), decimal.Zero, decimal.Zero)

	if len(blocks) != 1 {
		t.Fatalf("Expected 1 block, got %d", len(blocks))
	}
	// Single record means 0 kWh consumed and 0 kWh produced (same first and last)
	if blocks[0].ConsumedKWh != 0 {
		t.Errorf("ConsumedKWh = %f, want 0", blocks[0].ConsumedKWh)
	}
	if blocks[0].ProducedKWh != 0 {
		t.Errorf("ProducedKWh = %f, want 0", blocks[0].ProducedKWh)
	}
	if blocks[0].Cost != 0 {
		t.Errorf("Cost = %f, want 0", blocks[0].Cost)
	}
}

// --- fetchAddPrices and fetchParamValue tests ---

func TestFetchAddPrices_WithDefaults(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()
	store := newTestParamsStore(t)
	handler := NewHandler(mgr, nil, store, logger)

	addPrices, err := handler.fetchAddPrices()
	if err != nil {
		t.Fatalf("fetchAddPrices failed: %v", err)
	}

	// TransferAddPrice: 0.2584, EnergyTaxAddPrice: 0.36, DynamicAddPrice: 0.0442, StaticAddPrice: 0.04
	expected := decimal.NewFromFloat(0.2584).Add(decimal.NewFromFloat(0.36)).Add(decimal.NewFromFloat(0.0442)).Add(decimal.NewFromFloat(0.04))
	if !addPrices.Equal(expected) {
		t.Errorf("addPrices = %s, want %s", addPrices, expected)
	}
}

func TestProductionBenefit(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()
	store := newTestParamsStore(t)
	handler := NewHandler(mgr, nil, store, logger)

	now := time.Now()
	records := []storage.TimeFieldRecord{
		// Block 1: spot 0.50, consumed 5 kWh, produced 1 kWh
		{Time: now, SpotPrice: decimal.NewFromFloat(0.50), ConsumedAck: decimal.NewFromFloat(100.0), SoldAck: decimal.NewFromFloat(10.0)},
		{Time: now.Add(1 * time.Minute), SpotPrice: decimal.NewFromFloat(0.50), ConsumedAck: decimal.NewFromFloat(105.0), SoldAck: decimal.NewFromFloat(11.0)},
		// Block 2: spot 0.80, consumed 3 kWh, produced 0.5 kWh
		{Time: now.Add(2 * time.Minute), SpotPrice: decimal.NewFromFloat(0.80), ConsumedAck: decimal.NewFromFloat(105.0), SoldAck: decimal.NewFromFloat(11.0)},
		{Time: now.Add(3 * time.Minute), SpotPrice: decimal.NewFromFloat(0.80), ConsumedAck: decimal.NewFromFloat(108.0), SoldAck: decimal.NewFromFloat(11.5)},
	}

	// Fetch the same params the handler uses
	addPrices, err := handler.fetchAddPrices()
	if err != nil {
		t.Fatalf("fetchAddPrices: %v", err)
	}
	vatPercent, err := handler.fetchParamValue("VAT")
	if err != nil {
		t.Fatalf("fetchParamValue(VAT): %v", err)
	}
	gridBenefit, err := handler.fetchParamValue("grid_benefit")
	if err != nil {
		t.Fatalf("fetchParamValue(grid_benefit): %v", err)
	}
	eonAdded, err := handler.fetchParamValue("eon_added")
	if err != nil {
		t.Fatalf("fetchParamValue(eon_added): %v", err)
	}

	blocks := handler.calculateCostBlocks(records, addPrices, gridBenefit, eonAdded)
	if len(blocks) != 2 {
		t.Fatalf("Expected 2 blocks, got %d", len(blocks))
	}

	// Replicate the GetEnergyCost summation logic
	totalConsumed := decimal.Zero
	totalProduced := decimal.Zero
	costBeforeVAT := decimal.Zero
	productionBenefit := decimal.Zero
	for _, b := range blocks {
		totalConsumed = totalConsumed.Add(decimal.NewFromFloat(b.ConsumedKWh))
		totalProduced = totalProduced.Add(decimal.NewFromFloat(b.ProducedKWh))

		// Compute consumption cost from block fields for VAT calculation
		consumptionCost := decimal.NewFromFloat(b.ConsumedKWh).Mul(decimal.NewFromFloat(b.TotalPrice))
		costBeforeVAT = costBeforeVAT.Add(consumptionCost)

		blockProduced := decimal.NewFromFloat(b.ProducedKWh)
		blockSpot := decimal.NewFromFloat(b.SpotPrice)
		blockDecrease := blockSpot.Add(gridBenefit).Add(eonAdded).Mul(blockProduced)
		productionBenefit = productionBenefit.Add(blockDecrease)
	}
	vatMultiplier := decimal.NewFromInt(1).Add(vatPercent.Div(decimal.NewFromInt(100)))
	totalCost := costBeforeVAT.Mul(vatMultiplier).Sub(productionBenefit).Round(2)

	// Verify totals
	if !totalConsumed.Equal(decimal.NewFromFloat(8.0)) {
		t.Errorf("totalConsumed = %s, want 8.0", totalConsumed)
	}
	if !totalProduced.Equal(decimal.NewFromFloat(1.5)) {
		t.Errorf("totalProduced = %s, want 1.5", totalProduced)
	}

	// Block1 decrease: (0.50 + 0.0844 + 0.02) * 1.0 = 0.6044
	// Block2 decrease: (0.80 + 0.0844 + 0.02) * 0.5 = 0.4522
	// Total production benefit: 1.0566
	expectedBenefit := decimal.NewFromFloat(0.50).Add(gridBenefit).Add(eonAdded).Mul(decimal.NewFromFloat(1.0)).
		Add(decimal.NewFromFloat(0.80).Add(gridBenefit).Add(eonAdded).Mul(decimal.NewFromFloat(0.5)))
	if !productionBenefit.Equal(expectedBenefit) {
		t.Errorf("productionBenefit = %s, want %s", productionBenefit, expectedBenefit)
	}

	// total_cost = costBeforeVAT * 1.25 - productionBenefit
	expectedTotalCost := costBeforeVAT.Mul(vatMultiplier).Sub(productionBenefit).Round(2)
	if !totalCost.Equal(expectedTotalCost) {
		t.Errorf("totalCost = %s, want %s", totalCost, expectedTotalCost)
	}

	// Verify production benefit is subtracted (without VAT)
	costWithVAT := costBeforeVAT.Mul(vatMultiplier)
	if totalCost.GreaterThanOrEqual(costWithVAT) {
		t.Error("totalCost should be less than costBeforeVAT*vatMultiplier when production > 0")
	}
}

func TestFetchParamValue_VAT(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()
	store := newTestParamsStore(t)
	handler := NewHandler(mgr, nil, store, logger)

	vat, err := handler.fetchParamValue("VAT")
	if err != nil {
		t.Fatalf("fetchParamValue failed: %v", err)
	}

	if !vat.Equal(decimal.NewFromInt(25)) {
		t.Errorf("VAT = %s, want 25", vat)
	}
}

func TestFetchParamValue_NotFound(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := state.NewManager()
	store := newTestParamsStore(t)
	handler := NewHandler(mgr, nil, store, logger)

	_, err := handler.fetchParamValue("NonExistent")
	if err == nil {
		t.Fatal("Expected error for non-existent parameter")
	}
}
