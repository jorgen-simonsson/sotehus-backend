package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jorgen-simonsson/sotehus-backend/internal/api"
	"github.com/jorgen-simonsson/sotehus-backend/internal/config"
	"github.com/jorgen-simonsson/sotehus-backend/internal/services/grid"
	"github.com/jorgen-simonsson/sotehus-backend/internal/services/price"
	"github.com/jorgen-simonsson/sotehus-backend/internal/services/solar"
	"github.com/jorgen-simonsson/sotehus-backend/internal/state"
	"github.com/jorgen-simonsson/sotehus-backend/internal/storage"
)

func main() {
	// Setup structured logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	logger.Info("Starting sotehus-backend")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Create state manager
	stateManager := state.NewManager()

	// Create InfluxDB client (optional - continue if not available)
	var influxDB *storage.InfluxDBClient
	if cfg.InfluxDBHost != "" {
		influxDB, err = storage.NewInfluxDBClient(cfg, logger)
		if err != nil {
			logger.Warn("Failed to create InfluxDB client, continuing without persistence", "error", err)
		}
	} else {
		logger.Info("InfluxDB not configured, skipping persistence")
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// WaitGroup to track running services
	var wg sync.WaitGroup

	// Start Grid service (MQTT)
	if cfg.MQTTBrokerHost != "" && cfg.MQTTTopic != "" {
		gridService, err := grid.NewService(cfg, stateManager, influxDB, logger)
		if err != nil {
			logger.Error("Failed to create grid service", "error", err)
		} else {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := gridService.Start(ctx); err != nil {
					logger.Error("Grid service error", "error", err)
				}
			}()
			logger.Info("Grid service started")
		}
	} else {
		logger.Info("MQTT not configured, grid service disabled")
	}

	// Start Price service
	priceService := price.NewService(cfg, stateManager, influxDB, logger)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := priceService.Start(ctx); err != nil {
			logger.Error("Price service error", "error", err)
		}
	}()
	logger.Info("Price service started")

	// Start Solar service
	if cfg.SolarEdgeAPIKey != "" && cfg.SolarEdgeSiteID != "" {
		solarService, err := solar.NewService(cfg, stateManager, influxDB, logger)
		if err != nil {
			logger.Error("Failed to create solar service", "error", err)
		} else {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := solarService.Start(ctx); err != nil {
					logger.Error("Solar service error", "error", err)
				}
			}()
			logger.Info("Solar service started")
		}
	} else {
		logger.Info("SolarEdge not configured, solar service disabled")
	}

	// Create HTTP router
	router := api.NewRouter(stateManager, influxDB, logger)

	// Create HTTP server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.ServerPort),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start HTTP server in goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Info("Starting HTTP server", "port", cfg.ServerPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error", "error", err)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan
	logger.Info("Received signal, shutting down", "signal", sig)

	// Cancel context to stop all services
	cancel()

	// Gracefully shutdown HTTP server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown error", "error", err)
	}

	// Close InfluxDB client
	if influxDB != nil {
		influxDB.Close()
	}

	// Wait for all services to stop
	wg.Wait()
	logger.Info("Shutdown complete")
}
