# Sotehus Backend

A Go application that provides real-time energy monitoring data via a REST API.

## Table of Contents

- [Overview](#overview)
- [Swagger UI](#swagger-ui)
- [API Endpoints](#api-endpoints)
  - [GET /api/data](#get-apidata)
  - [GET /api/timeseries](#get-apitimeseries)
  - [GET /api/version](#get-apiversion)
  - [GET /api/energy/consumed](#get-apienergyconsumed)
  - [GET /api/energy/sold](#get-apienergysold)
  - [GET /api/energy/cost](#get-apienergycost)
  - [GET /health](#get-health)
- [Persistent Parameters](#persistent-parameters)
  - [Overview](#parameters-overview)
  - [Default Parameters](#default-parameters)
  - [GET /api/params](#get-apiparams)
  - [GET /api/params/{key}](#get-apiparamskey)
  - [POST /api/params](#post-apiparams)
  - [PUT /api/params/{key}](#put-apiparamskey)
- [Architecture](#architecture)
  - [Grid Process](#grid-process)
  - [Price Process](#price-process)
  - [Solar Process](#solar-process)
  - [FFR Process](#ffr-process)
- [Configuration](#configuration)
- [Project Structure](#project-structure)
- [Building and Running](#building-and-running)
- [Testing](#testing)
- [Changelog](#changelog)

## Overview

This backend service aggregates data from multiple sources to provide:
- **Grid Consumption** - Real-time power consumption from smart meter via MQTT
- **Electricity Price** - Current spot price per kWh from Swedish electricity market
- **Solar Production** - Current solar panel output from SolarEdge API
- **Grid Frequency** - Real-time grid frequency from FFR collector via MQTT

## Swagger UI

Interactive API documentation is available at:
```
http://localhost:8080/swagger/index.html
```

## API Endpoints

### `GET /api/data`

Returns current status of all energy sources.

**Response:**
```json
{
    "grid": {
        "valid": true,
        "power": 245.5,
        "lastUpdate": "2025-12-07T16:30:00+01:00",
        "message": ""
    },
    "price": {
        "valid": true,
        "price": 1.20,
        "lastUpdate": "2025-12-07T16:15:00+01:00"
    },
    "solar": {
        "valid": false,
        "power": 0,
        "lastUpdate": "2025-12-07T15:00:00+01:00",
        "message": "No sun"
    },
    "frequency": {
        "valid": true,
        "frequency": 50.01,
        "lastUpdate": "2025-12-07T16:30:00+01:00"
    }
}
```

**Field descriptions:**
- `valid` - Whether the data is current and reliable
- `power` - Power in Watts
- `price` - Price in SEK per kWh
- `frequency` - Grid frequency in Hz (e.g., 50.01)
- `lastUpdate` - Timestamp of last successful data update
- `message` - Error or status message (e.g., "No sun", "No current data for grid consumption")

### `GET /api/timeseries`

Returns statistics about the historical data stored in InfluxDB.

**Response:**
```json
{
    "first": "2025-11-01T16:10",
    "last": "2025-12-07T16:30",
    "count": 523847
}
```

**Field descriptions:**
- `first` - Timestamp of the oldest entry in the database (local time)
- `last` - Timestamp of the newest entry in the database (local time)
- `count` - Total number of grid power entries

### `GET /api/version`

Returns the API version.

**Response:**
```json
{
    "version": "1.4.0"
}
```

### `GET /api/energy/consumed`

Returns the energy consumed in kWh between two timestamps.

**Parameters:**
- `start` - Start timestamp in RFC3339 format (e.g., `2026-02-01T00:00:00+01:00`)
- `stop` - Stop timestamp in RFC3339 format (e.g., `2026-02-21T00:00:00+01:00`)

**Response:**
```json
{
    "start": "2026-02-01T00:00:00+01:00",
    "stop": "2026-02-21T00:00:00+01:00",
    "actual_start": "2026-02-01T00:01:23+01:00",
    "actual_stop": "2026-02-20T23:59:45+01:00",
    "consumed": 523.45,
    "unit": "kWh"
}
```

**Field descriptions:**
- `start` / `stop` - Requested time range
- `actual_start` / `actual_stop` - Actual timestamps where data was found
- `consumed` - Energy consumed in kWh (difference between accumulator values)

**Errors:**
- Returns 500 if no data exists at or before the start or stop timestamp

### `GET /api/energy/sold`

Returns the energy sold (exported to grid) in kWh between two timestamps.

**Parameters:**
- `start` - Start timestamp in RFC3339 format (e.g., `2026-02-01T00:00:00+01:00`)
- `stop` - Stop timestamp in RFC3339 format (e.g., `2026-02-21T00:00:00+01:00`)

**Response:**
```json
{
    "start": "2026-02-01T00:00:00+01:00",
    "stop": "2026-02-21T00:00:00+01:00",
    "actual_start": "2026-02-01T00:01:23+01:00",
    "actual_stop": "2026-02-20T23:59:45+01:00",
    "sold": 123.45,
    "unit": "kWh"
}
```

**Field descriptions:**
- `start` / `stop` - Requested time range
- `actual_start` / `actual_stop` - Actual timestamps where data was found
- `sold` - Energy sold in kWh (difference between accumulator values)

**Errors:**
- Returns 500 if no data exists at or before the start or stop timestamp

### `GET /api/energy/cost`

Returns the actual energy cost for a period, broken down by spot-price blocks.

The endpoint queries InfluxDB for `spot_price` and consumed-energy accumulator readings in the requested time range, groups consecutive records that share the same spot price into blocks, and computes per-block kWh and cost. Configured price additions (transfer fee, energy tax, dynamic and static additions) are added on top of the spot price for each block. Finally, VAT is applied to the total.

If no spot price is recorded for a given time window, the most recent previous value is carried forward (InfluxDB `fill(usePrevious: true)`).

**Parameters:**
- `start` – Start timestamp in RFC3339 format (e.g., `2026-02-01T00:00:00+01:00`)
- `stop` – Stop timestamp in RFC3339 format (e.g., `2026-02-21T00:00:00+01:00`)

**Response:**
```json
{
    "period_start": "2026-02-01T00:00:00+01:00",
    "period_stop": "2026-02-21T00:00:00+01:00",
    "total_consumed_kwh": 156.78,
    "cost_before_vat": 180.73,
    "vat_percent": 25,
    "total_cost": 225.91,
    "unit": "SEK",
    "blocks": [
        {
            "spot_price": 0.45,
            "added_prices": 0.7026,
            "total_price": 1.1526,
            "consumed_kwh": 12.34,
            "cost": 14.22,
            "start": "2026-02-01T00:00:00+01:00",
            "stop": "2026-02-01T01:00:00+01:00"
        }
    ]
}
```

**Field descriptions:**
- `period_start` / `period_stop` – Requested time range
- `total_consumed_kwh` – Total energy consumed in the period (kWh)
- `cost_before_vat` – Sum of all block costs before VAT (SEK)
- `vat_percent` – VAT percentage applied (from persistent parameters)
- `total_cost` – Final cost including VAT (SEK)
- `blocks` – Array of per-price-block breakdowns:
  - `spot_price` – Spot price during this block (SEK/kWh)
  - `added_prices` – Sum of transfer, energy tax, dynamic and static additions (SEK/kWh)
  - `total_price` – `spot_price + added_prices` (SEK/kWh)
  - `consumed_kwh` – Energy consumed in this block (kWh)
  - `cost` – `consumed_kwh × total_price` (SEK, before VAT)
  - `start` / `stop` – Time boundaries of this block

**Errors:**
- `400` – Missing or invalid start/stop parameter, or stop is before start
- `500` – Failed to fetch parameters or query InfluxDB
- `503` – InfluxDB or parameter store not configured

### `GET /health`

Health check endpoint.

**Response:**
```json
{
    "status": "ok"
}
```

## Persistent Parameters

### Parameters Overview

The application includes a persistent parameter store backed by SQLite. Parameters are key-value configuration entries stored in a local database file, allowing runtime configuration that survives restarts.

Each parameter has the following fields:

| Field | Type | Description |
|-------|------|-------------|
| `id` | UUID string | Auto-generated unique identifier |
| `key` | string | Unique key identifying the parameter |
| `description` | string | Human-readable description |
| `content` | JSON string | Parameter value as a JSON string |
| `changed` | timestamp | Last modification time |

The SQLite database is stored at the path configured by `SQLITE_DB_PATH` (default: `./data/params.db`). When running with Docker, a volume mount ensures the database persists across container restarts.

### Default Parameters

The following parameters are automatically seeded on first startup if they don't already exist:

| Key | Description | Default Content |
|-----|-------------|----------------|
| `TransferAddPrice` | Electricity transfer addition to price | `{"value": 0.2584}` |
| `EnergyTaxAddPrice` | Energy tax addition to price | `{"value": 0.36}` |
| `DynamicAddPrice` | Dynamic addition to price | `{"value": 0.0442}` |
| `StaticAddPrice` | Static addition to price | `{"value": 0.04}` |
| `VAT` | VAT percent | `{"value": 25}` |

Default parameters are defined in `internal/storage/params/model.go` and can be extended by adding entries to the `DefaultParams` slice.

### `GET /api/params`

Returns all persistent parameters.

**Response:**
```json
[
    {
        "id": "550e8400-e29b-41d4-a716-446655440000",
        "key": "DynamicAddPrice",
        "description": "Dynamic addition to price",
        "content": "{\"value\": 0.04}",
        "changed": "2026-01-15T10:30:00+01:00"
    },
    {
        "id": "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
        "key": "StaticAddPrice",
        "description": "Static addition to price",
        "content": "{\"value\": 0.06}",
        "changed": "2026-01-15T10:30:00+01:00"
    }
]
```

### `GET /api/params/{key}`

Returns a single parameter by its key.

**Path parameters:**
- `key` - The parameter key (e.g., `DynamicAddPrice`)

**Response (200):**
```json
{
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "key": "DynamicAddPrice",
    "description": "Dynamic addition to price",
    "content": "{\"value\": 0.04}",
    "changed": "2026-01-15T10:30:00+01:00"
}
```

**Errors:**
- `404` - Parameter not found

### `POST /api/params`

Creates a new parameter. Returns `409 Conflict` if a parameter with the same key already exists.

**Request body:**
```json
{
    "key": "MyNewParam",
    "description": "A custom parameter",
    "content": "{\"enabled\": true, \"threshold\": 0.5}"
}
```

**Response (201):**
```json
{
    "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "key": "MyNewParam",
    "description": "A custom parameter",
    "content": "{\"enabled\": true, \"threshold\": 0.5}",
    "changed": "2026-02-22T14:00:00+01:00"
}
```

**Errors:**
- `400` - Invalid request body or missing key
- `409` - Parameter with this key already exists

### `PUT /api/params/{key}`

Updates an existing parameter's description and content.

**Path parameters:**
- `key` - The parameter key to update

**Request body:**
```json
{
    "description": "Updated description",
    "content": "{\"value\": 0.10}"
}
```

**Response (200):**
```json
{
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "key": "DynamicAddPrice",
    "description": "Updated description",
    "content": "{\"value\": 0.10}",
    "changed": "2026-02-22T14:05:00+01:00"
}
```

**Errors:**
- `400` - Invalid request body
- `404` - Parameter not found

## Architecture

The application runs three background processes alongside the HTTP server:

### Grid Process
- Subscribes to multiple MQTT topics defined in `GridTopics` for real-time power and energy data
- Aggregates values from all topics before writing to InfluxDB
- Waits up to 2 seconds for all topic values to arrive, then writes a single record
- Logs a warning if timeout occurs before all values are received (incomplete data is still written)
- Writes each aggregated data point to InfluxDB for historical tracking
- Marks data as invalid if no MQTT message received for 1 minute

#### Adding New Grid Topics

To add more MQTT topics to the grid data collection, edit the `GridTopics` slice in `internal/services/grid/grid.go`:

```go
var GridTopics = []TopicMapping{
    {Topic: "dsmr/reading/powerdelivered_netto", FieldName: "grid_power"},
    {Topic: "dsmr/reading/electricity_delivered_1", FieldName: "grid_energy_consumed"},
    {Topic: "dsmr/reading/electricity_returned_1", FieldName: "grid_energy_sold"},
    // Add new topics here:
    // {Topic: "dsmr/reading/voltage_l1", FieldName: "grid_voltage_l1"},
}
```

Each entry maps an MQTT topic to an InfluxDB field name. All values are aggregated into a single InfluxDB record.

### Price Process
- Fetches spot prices from [elprisetjustnu.se](https://www.elprisetjustnu.se) API
- Prices are provided in 15-minute intervals
- Extracts current price based on local time
- Updates every 15 minutes

### Solar Process
- Fetches current production from SolarEdge Monitoring API
- Respects API rate limits (300 calls/day)
- Calculates optimal polling interval based on daylight hours
- Polls from 1 hour before sunrise until 1 hour after sunset
- Shows "No sun" message during nighttime

### FFR Process
- Subscribes to the `ffr_collector` MQTT topic for real-time grid frequency data
- Parses 4-character payloads (e.g., "5001" → 50.01 Hz)
- Validates frequency is within expected range (48–52 Hz)
- Handles high-frequency data (many updates per second) thread-safely
- The current frequency is included in InfluxDB writes alongside grid power data

## Configuration

Environment variables (`.env` file):

```bash
# Server Configuration
SERVER_PORT=8080

# MQTT Configuration
MQTT_BROKER_HOST=your_mqtt_host
MQTT_BROKER_PORT=1883
MQTT_USERNAME=your_username
MQTT_PASSWORD=your_password
# Note: Grid topics are defined statically in internal/services/grid/grid.go

# InfluxDB Configuration
INFLUXDB2_HOST=your_influxdb_host
INFLUXDB2_PORT=8086
INFLUXDB2_USER=your_user
INFLUXDB2_PASSWORD=your_password
INFLUXDB2_ORG=sotehus
INFLUXDB2_BUCKET=sotehus_bucket
INFLUXDB2_TOKEN=your_api_token

# SolarEdge Configuration
SOLAREDGE_API_KEY=your_api_key
SOLAREDGE_SITE_ID=your_site_id

# Spot Price Configuration
SPOTPRICE_REGION=SE4

# SQLite Configuration
SQLITE_DB_PATH=./data/params.db
```

## Project Structure

```
├── cmd/
│   └── server/
│       └── main.go              # Application entry point
├── internal/
│   ├── api/
│   │   ├── handler.go           # HTTP handlers
│   │   ├── router.go            # Route definitions
│   │   └── version.go           # API version constant and endpoint
│   ├── config/
│   │   └── config.go            # Environment configuration
│   ├── models/
│   │   └── data.go              # Data structures
│   ├── services/
│   │   ├── ffr/
│   │   │   └── ffr.go           # MQTT subscriber for grid frequency (FFR)
│   │   ├── grid/
│   │   │   └── grid.go          # MQTT subscriber for grid consumption
│   │   ├── price/
│   │   │   └── price.go         # Spot price fetcher
│   │   └── solar/
│   │       └── solar.go         # SolarEdge API client
│   ├── storage/
│   │   ├── influxdb.go          # InfluxDB client
│   │   └── params/
│   │       ├── errors.go        # Sentinel errors
│   │       ├── model.go         # Parameter model and defaults
│   │       └── store.go         # SQLite parameter store
│   └── state/
│       └── manager.go           # Thread-safe state management
├── docker-compose.yml
├── Dockerfile
├── go.mod
├── go.sum
├── .env.example
├── Makefile
└── README.md
```

## Building and Running

```bash
# Install dependencies
make deps

# Build the application
make build

# Run the application
make run

# Or run in development mode
make dev
```

### Docker

```bash
# Build and start with Docker Compose
docker compose up --build -d

# View logs
docker compose logs -f

# Stop
docker compose down
```

> **Note:** The `docker-compose.yml` mounts `./data:/app/data` to persist the SQLite parameter database across container restarts.

## Testing

The project includes comprehensive unit tests for all packages.

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run tests with coverage report
go test -cover ./...

# Run tests for a specific package
go test -v ./internal/config
go test -v ./internal/state
go test -v ./internal/api
go test -v ./internal/storage/params
```

### Test Coverage

| Package | Coverage | Description |
|---------|----------|-------------|
| `internal/config` | 100% | Configuration loading and environment variables |
| `internal/state` | 100% | Thread-safe state management |
| `internal/api` | ~67% | HTTP handlers, routing, and CORS |
| `internal/storage/params` | ~90% | SQLite parameter store (in-memory tests) |
| `internal/services/solar` | ~46% | SolarEdge client and sunrise/sunset calculations |
| `internal/services/price` | ~30% | Spot price fetching and matching |
| `internal/services/grid` | ~19% | MQTT subscription (requires broker for full testing) |
| `internal/services/ffr` | ~30% | FFR frequency parsing and MQTT subscription |
| `internal/storage` | ~3% | InfluxDB client (requires database for full testing) |
| `internal/models` | N/A | Data structures (no executable code) |

Note: Some packages have lower coverage because they require external services (MQTT broker, InfluxDB, HTTP APIs) for complete integration testing.

## Changelog

### 2026-02-22 Ver 1.4.0
- New endpoint: `GET /api/energy/cost` – calculates actual energy cost for a period
  - Groups records into blocks of constant spot price
  - Adds configured price components (transfer, energy tax, dynamic, static)
  - Applies VAT from persistent parameters
  - Returns per-block breakdown with kWh and cost
- Added `ParseContentValue` helper for extracting numeric values from parameter JSON
- Added `GetFieldsInRange` InfluxDB query combining spot price and consumed energy
- Extended default parameters: `TransferAddPrice`, `EnergyTaxAddPrice`, `VAT`
- Consolidated InfluxDB writes: all fields written in a single record per cycle
- Comprehensive unit tests for energy cost calculation and parameter parsing
- Updated Swagger documentation

### 2026-02-22 Ver 1.3.0
- Added persistent parameter store backed by SQLite (`internal/storage/params/`)
- New endpoints: `GET /api/params`, `GET /api/params/{key}`, `POST /api/params`, `PUT /api/params/{key}`
- Default parameters seeded on startup (`DynamicAddPrice`, `StaticAddPrice`)
- Added `SQLITE_DB_PATH` configuration option (default: `./data/params.db`)
- Docker volume mount (`./data:/app/data`) for SQLite persistence
- Integration and unit tests for parameter handling
- Updated Swagger documentation with parameter endpoints
- Reconstructed README with table of contents
