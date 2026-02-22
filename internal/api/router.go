package api

import (
	"log/slog"
	"net/http"

	"github.com/jorgen-simonsson/sotehus-backend/internal/state"
	"github.com/jorgen-simonsson/sotehus-backend/internal/storage"
	"github.com/jorgen-simonsson/sotehus-backend/internal/storage/params"
	httpSwagger "github.com/swaggo/http-swagger"
)

// NewRouter creates a new HTTP router with all routes configured
func NewRouter(state *state.Manager, influxDB *storage.InfluxDBClient, paramsStore *params.Store, logger *slog.Logger) http.Handler {
	handler := NewHandler(state, influxDB, paramsStore, logger)

	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("GET /api/data", handler.GetData)
	mux.HandleFunc("GET /api/timeseries", handler.GetTimeSeries)
	mux.HandleFunc("GET /api/version", handler.GetVersion)
	mux.HandleFunc("GET /api/energy/consumed", handler.GetEnergyConsumed)
	mux.HandleFunc("GET /api/energy/sold", handler.GetEnergySold)
	mux.HandleFunc("GET /health", handler.HealthCheck)

	// Parameter routes
	mux.HandleFunc("GET /api/params", handler.GetAllParams)
	mux.HandleFunc("GET /api/params/{key}", handler.GetParamByKey)
	mux.HandleFunc("POST /api/params", handler.CreateParam)
	mux.HandleFunc("PUT /api/params/{key}", handler.UpdateParam)

	// Swagger UI
	mux.Handle("GET /swagger/", httpSwagger.WrapHandler)

	// Add logging middleware
	return loggingMiddleware(logger, corsMiddleware(mux))
}

// corsMiddleware adds CORS headers to responses
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware logs HTTP requests
func loggingMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Debug("HTTP request", "method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}
