package models

import "time"

// GridData represents current grid power consumption
type GridData struct {
	Valid      bool      `json:"valid" example:"true"`
	Power      float64   `json:"power" example:"245.5"`
	LastUpdate time.Time `json:"lastUpdate" example:"2025-12-07T16:30:00+01:00"`
	Message    string    `json:"message" example:""`
}

// PriceData represents current electricity spot price
type PriceData struct {
	Valid      bool      `json:"valid" example:"true"`
	Price      float64   `json:"price" example:"1.20"`
	LastUpdate time.Time `json:"lastUpdate" example:"2025-12-07T16:15:00+01:00"`
}

// SolarData represents current solar power production
type SolarData struct {
	Valid      bool      `json:"valid" example:"false"`
	Power      float64   `json:"power" example:"0"`
	LastUpdate time.Time `json:"lastUpdate" example:"2025-12-07T15:00:00+01:00"`
	Message    string    `json:"message" example:"No sun"`
}

// FrequencyData represents current grid frequency from FFR collector
type FrequencyData struct {
	Valid      bool      `json:"valid" example:"true"`
	Frequency  float64   `json:"frequency" example:"50.01"`
	LastUpdate time.Time `json:"lastUpdate" example:"2025-12-07T16:30:00+01:00"`
}

// APIResponse represents the combined response for the /api/data endpoint
type APIResponse struct {
	Grid      GridData      `json:"grid"`
	Price     PriceData     `json:"price"`
	Solar     SolarData     `json:"solar"`
	Frequency FrequencyData `json:"frequency"`
}

// SpotPriceEntry represents a single price entry from the elprisetjustnu.se API
type SpotPriceEntry struct {
	SEKPerKWh float64 `json:"SEK_per_kWh"`
	EURPerKWh float64 `json:"EUR_per_kWh"`
	EXR       float64 `json:"EXR"`
	TimeStart string  `json:"time_start"`
	TimeEnd   string  `json:"time_end"`
}

// SolarEdgePowerFlow represents the response from SolarEdge currentPowerFlow API
type SolarEdgePowerFlow struct {
	SiteCurrentPowerFlow struct {
		UpdateRefreshRate int    `json:"updateRefreshRate"`
		Unit              string `json:"unit"`
		Connections       []struct {
			From string `json:"from"`
			To   string `json:"to"`
		} `json:"connections"`
		Grid struct {
			Status       string  `json:"status"`
			CurrentPower float64 `json:"currentPower"`
		} `json:"GRID"`
		Load struct {
			Status       string  `json:"status"`
			CurrentPower float64 `json:"currentPower"`
		} `json:"LOAD"`
		PV struct {
			Status       string  `json:"status"`
			CurrentPower float64 `json:"currentPower"`
		} `json:"PV"`
	} `json:"siteCurrentPowerFlow"`
}

// MQTTPayload represents the expected MQTT message format
type MQTTPayload struct {
	Power float64 `json:"power"`
}

// WeatherReading represents a single value/unit pair from the local weather station.
type WeatherReading struct {
	Value         float64 `json:"value" example:"21.5"`
	UnitOfMeasure string  `json:"unitOfMeasure" example:"C"`
}

// WeatherData represents the latest set of readings received from the weather station over MQTT
type WeatherData struct {
	Valid      bool                      `json:"valid" example:"true"`
	Readings   map[string]WeatherReading `json:"readings"`
	LastUpdate time.Time                 `json:"lastUpdate" example:"2025-12-07T16:30:00+01:00"`
}
