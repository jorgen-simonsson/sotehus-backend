package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestGridDataJSON(t *testing.T) {
	timestamp := time.Date(2025, 12, 7, 16, 30, 0, 0, time.UTC)
	grid := GridData{
		Valid:      true,
		Power:      1234.5,
		LastUpdate: timestamp,
		Message:    "",
	}

	data, err := json.Marshal(grid)
	if err != nil {
		t.Fatalf("Failed to marshal GridData: %v", err)
	}

	var decoded GridData
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal GridData: %v", err)
	}

	if decoded.Valid != grid.Valid {
		t.Errorf("Valid = %v, want %v", decoded.Valid, grid.Valid)
	}
	if decoded.Power != grid.Power {
		t.Errorf("Power = %f, want %f", decoded.Power, grid.Power)
	}
	if !decoded.LastUpdate.Equal(grid.LastUpdate) {
		t.Errorf("LastUpdate = %v, want %v", decoded.LastUpdate, grid.LastUpdate)
	}
	if decoded.Message != grid.Message {
		t.Errorf("Message = %q, want %q", decoded.Message, grid.Message)
	}
}

func TestPriceDataJSON(t *testing.T) {
	timestamp := time.Date(2025, 12, 7, 16, 30, 0, 0, time.UTC)
	price := PriceData{
		Valid:      true,
		Price:      0.89,
		LastUpdate: timestamp,
	}

	data, err := json.Marshal(price)
	if err != nil {
		t.Fatalf("Failed to marshal PriceData: %v", err)
	}

	var decoded PriceData
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal PriceData: %v", err)
	}

	if decoded.Valid != price.Valid {
		t.Errorf("Valid = %v, want %v", decoded.Valid, price.Valid)
	}
	if decoded.Price != price.Price {
		t.Errorf("Price = %f, want %f", decoded.Price, price.Price)
	}
}

func TestSolarDataJSON(t *testing.T) {
	timestamp := time.Date(2025, 12, 7, 16, 30, 0, 0, time.UTC)
	solar := SolarData{
		Valid:      false,
		Power:      0,
		LastUpdate: timestamp,
		Message:    "No sun",
	}

	data, err := json.Marshal(solar)
	if err != nil {
		t.Fatalf("Failed to marshal SolarData: %v", err)
	}

	var decoded SolarData
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal SolarData: %v", err)
	}

	if decoded.Valid != solar.Valid {
		t.Errorf("Valid = %v, want %v", decoded.Valid, solar.Valid)
	}
	if decoded.Message != solar.Message {
		t.Errorf("Message = %q, want %q", decoded.Message, solar.Message)
	}
}

func TestAPIResponseJSON(t *testing.T) {
	timestamp := time.Date(2025, 12, 7, 16, 30, 0, 0, time.UTC)
	response := APIResponse{
		Grid: GridData{
			Valid:      true,
			Power:      1234.5,
			LastUpdate: timestamp,
			Message:    "",
		},
		Price: PriceData{
			Valid:      true,
			Price:      0.89,
			LastUpdate: timestamp,
		},
		Solar: SolarData{
			Valid:      false,
			Power:      0,
			LastUpdate: timestamp,
			Message:    "No sun",
		},
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal APIResponse: %v", err)
	}

	// Verify JSON structure
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	if _, ok := raw["grid"]; !ok {
		t.Error("JSON should contain 'grid' key")
	}
	if _, ok := raw["price"]; !ok {
		t.Error("JSON should contain 'price' key")
	}
	if _, ok := raw["solar"]; !ok {
		t.Error("JSON should contain 'solar' key")
	}
}

func TestSpotPriceEntryJSON(t *testing.T) {
	jsonData := `{
		"SEK_per_kWh": 0.89,
		"EUR_per_kWh": 0.08,
		"EXR": 11.25,
		"time_start": "2025-12-07T16:00:00+01:00",
		"time_end": "2025-12-07T17:00:00+01:00"
	}`

	var entry SpotPriceEntry
	if err := json.Unmarshal([]byte(jsonData), &entry); err != nil {
		t.Fatalf("Failed to unmarshal SpotPriceEntry: %v", err)
	}

	if entry.SEKPerKWh != 0.89 {
		t.Errorf("SEKPerKWh = %f, want %f", entry.SEKPerKWh, 0.89)
	}
	if entry.EURPerKWh != 0.08 {
		t.Errorf("EURPerKWh = %f, want %f", entry.EURPerKWh, 0.08)
	}
	if entry.TimeStart != "2025-12-07T16:00:00+01:00" {
		t.Errorf("TimeStart = %q, want %q", entry.TimeStart, "2025-12-07T16:00:00+01:00")
	}
}

func TestSolarEdgePowerFlowJSON(t *testing.T) {
	jsonData := `{
		"siteCurrentPowerFlow": {
			"updateRefreshRate": 3,
			"unit": "kW",
			"PV": {
				"status": "Active",
				"currentPower": 5.5
			},
			"LOAD": {
				"status": "Active",
				"currentPower": 3.2
			},
			"GRID": {
				"status": "Active",
				"currentPower": 2.3
			}
		}
	}`

	var powerFlow SolarEdgePowerFlow
	if err := json.Unmarshal([]byte(jsonData), &powerFlow); err != nil {
		t.Fatalf("Failed to unmarshal SolarEdgePowerFlow: %v", err)
	}

	if powerFlow.SiteCurrentPowerFlow.PV.CurrentPower != 5.5 {
		t.Errorf("PV.CurrentPower = %f, want %f", powerFlow.SiteCurrentPowerFlow.PV.CurrentPower, 5.5)
	}
	if powerFlow.SiteCurrentPowerFlow.PV.Status != "Active" {
		t.Errorf("PV.Status = %q, want %q", powerFlow.SiteCurrentPowerFlow.PV.Status, "Active")
	}
	if powerFlow.SiteCurrentPowerFlow.Unit != "kW" {
		t.Errorf("Unit = %q, want %q", powerFlow.SiteCurrentPowerFlow.Unit, "kW")
	}
}

func TestMQTTPayloadJSON(t *testing.T) {
	jsonData := `{"power": 1234.5}`

	var payload MQTTPayload
	if err := json.Unmarshal([]byte(jsonData), &payload); err != nil {
		t.Fatalf("Failed to unmarshal MQTTPayload: %v", err)
	}

	if payload.Power != 1234.5 {
		t.Errorf("Power = %f, want %f", payload.Power, 1234.5)
	}
}
