# Sotehus Backend

A Go application that provides real-time energy monitoring data via a REST API.

## Overview

This backend service aggregates data from multiple sources to provide:
- **Grid Consumption** - Real-time power consumption from smart meter via MQTT
- **Electricity Price** - Current spot price per kWh from Swedish electricity market
- **Solar Production** - Current solar panel output from SolarEdge API

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
    }
}
```

**Field descriptions:**
- `valid` - Whether the data is current and reliable
- `power` - Power in Watts
- `price` - Price in SEK per kWh
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

### `GET /health`

Health check endpoint.

**Response:**
```json
{
    "status": "ok"
}
```

## Architecture

The application runs three background processes alongside the HTTP server:

### Grid Process
- Subscribes to a local MQTT broker for real-time power consumption
- Writes each received data point to InfluxDB for historical tracking
- Marks data as invalid if no MQTT message received for 1 minute

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
MQTT_TOPIC=your/power/topic

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
```

## Project Structure

```
├── cmd/
│   └── server/
│       └── main.go              # Application entry point
├── internal/
│   ├── api/
│   │   ├── handler.go           # HTTP handlers
│   │   └── router.go            # Route definitions
│   ├── config/
│   │   └── config.go            # Environment configuration
│   ├── models/
│   │   └── data.go              # Data structures
│   ├── services/
│   │   ├── grid/
│   │   │   └── grid.go          # MQTT subscriber for grid consumption
│   │   ├── price/
│   │   │   └── price.go         # Spot price fetcher
│   │   └── solar/
│   │       └── solar.go         # SolarEdge API client
│   ├── storage/
│   │   └── influxdb.go          # InfluxDB client
│   └── state/
│       └── manager.go           # Thread-safe state management
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

## Reference Implementation

The `python/` directory contains a reference implementation of this backend in Python.