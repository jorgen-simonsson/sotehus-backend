package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jorgen-simonsson/sotehus-backend/internal/models"
	"github.com/jorgen-simonsson/sotehus-backend/internal/state"
)

func TestNewHandler(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mgr := state.NewManager()

	handler := NewHandler(mgr, nil, logger)

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

	handler := NewHandler(mgr, nil, logger)

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

	handler := NewHandler(mgr, nil, logger)

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
	handler := NewHandler(mgr, nil, logger)

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
	handler := NewHandler(mgr, nil, logger) // nil InfluxDB client

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

	mux := NewRouter(mgr, nil, logger)

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
	mux := NewRouter(mgr, nil, logger)

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
