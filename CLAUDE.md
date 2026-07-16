# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build
make build          # produces bin/sotehus-backend
make build-all      # cross-compile for linux/darwin × amd64/arm64

# Run
make run            # build then run
make dev            # hot-reload via `air` if installed, else `go run ./cmd/server`

# Test
go test ./...                        # all packages
go test -v ./internal/api            # single package
go test -v -run TestFoo ./internal/api  # single test
make test-coverage                   # coverage HTML report

# Dependencies
make deps           # go mod download + tidy

# Docker
docker compose up --build -d
docker compose logs -f
```

Configuration is loaded from a `.env` file (see `.env.example`). The server starts fine without MQTT/InfluxDB — those services are disabled gracefully when env vars are absent.

Swagger UI is at `http://localhost:8080/swagger/index.html`.

## Architecture

The entry point (`cmd/server/main.go`) wires up all components and starts four independent background goroutines alongside the HTTP server. Each goroutine is driven by a `context.Context` and stops cleanly on SIGINT/SIGTERM.

### Shared state

`internal/state/Manager` is the single source of truth passed to all services. It is the only mutable shared object and all access is guarded by `sync.RWMutex`. Services write to it; HTTP handlers read from it. There is no channel-based message passing between services.

### Background services

| Service | Package | Data source | Writes to InfluxDB? |
|---------|---------|-------------|---------------------|
| Grid | `internal/services/grid` | MQTT topics in `GridTopics` slice | Yes — single aggregated record every ~5 s |
| FFR | `internal/services/ffr` | MQTT topic `ffr_collector` | No (values carried by Grid write cycle) |
| Price | `internal/services/price` | `elprisetjustnu.se` REST API | No |
| Solar (cloud) | `internal/services/solar/solar.go` | SolarEdge Monitoring API | No |
| Solar (MQTT) | `internal/services/solar/mqtt.go` | MQTT `solaredge/modbus/inverter` | No |

The **Grid service** is the InfluxDB writer for the whole system. It aggregates fields from multiple MQTT topics (defined in `grid.GridTopics`) and also pulls `solarEnergyAck` and `solarFrequency` from the state manager to include in each write.

The **FFR service** accumulates frequency samples into the state manager. `state.Manager.GetAndResetAverageFrequency()` is called by the Grid service each write cycle — it computes the average of all samples since the last call and resets the accumulator.

The **Solar MQTT service** and **Solar cloud service** are mutually exclusive. Which one runs is determined at startup by the `use_local_mqtt_solar` SQLite parameter (default `true`).

### Storage

- **InfluxDB** (`internal/storage/influxdb.go`) — all time-series data, measurement `power_monitoring`. Optional at startup.
- **SQLite** (`internal/storage/params/`) — persistent key-value parameter store. `params.Store` seeds default parameters on first run (defined in `model.go:DefaultParams`). `content` column is a raw JSON string; helpers `ParseContentValue` (float64) and `ParseContentBool` (bool) extract values.

### HTTP layer

`internal/api/router.go` wires routes onto a stdlib `http.ServeMux` with CORS and logging middleware. Handlers live in `handler.go` and receive `*state.Manager`, `*storage.InfluxDBClient`, and `*params.Store` as dependencies.

The energy cost endpoint (`GET /api/energy/cost`) is the most complex handler — it issues a Flux query that uses `aggregateWindow(every: 1h, fn: last)` **before** `fill(usePrevious: true)` to downsample ~500k rows to ~720 before the expensive pivot and spot-price block grouping.

### Tools

`tools/tailflux/` and `tools/modtest/` are standalone CLI programs (separate `main.go` files) for local development/debugging. They are not part of the main server binary.

## Key conventions

- All InfluxDB fields are written as a single point per cycle to the `power_monitoring` measurement — never add a second `WriteAPI` call for new fields; extend the existing point in `internal/services/grid/grid.go`.
- Grid MQTT topics are statically declared in `grid.GridTopics`; adding a new sensor field means appending a `TopicMapping` entry there.
- Default parameters are seeded from `internal/storage/params/model.go:DefaultParams` — extend that slice when adding new configurable values.
- The API version string lives in `internal/api/version.go`; bump it there when making API changes.
