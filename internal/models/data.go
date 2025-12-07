package models

import "time"

// GridData represents current grid power consumption
type GridData struct {
	Valid      bool      `json:"valid"`
	Power      float64   `json:"power"`
	LastUpdate time.Time `json:"lastUpdate"`
	Message    string    `json:"message"`
}

// PriceData represents current electricity spot price
type PriceData struct {
	Valid      bool      `json:"valid"`
	Price      float64   `json:"price"`
	LastUpdate time.Time `json:"lastUpdate"`
}

// SolarData represents current solar power production
type SolarData struct {
	Valid      bool      `json:"valid"`
	Power      float64   `json:"power"`
	LastUpdate time.Time `json:"lastUpdate"`
	Message    string    `json:"message"`
}

// APIResponse represents the combined response for the /api/data endpoint
type APIResponse struct {
	Grid  GridData  `json:"grid"`
	Price PriceData `json:"price"`
	Solar SolarData `json:"solar"`
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
