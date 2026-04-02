package params

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// PersistentParam represents a persistent configuration parameter stored in SQLite
type PersistentParam struct {
	ID          string    `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Key         string    `json:"key" example:"DynamicAddPrice"`
	Description string    `json:"description" example:"Dynamic addition to price"`
	Content     string    `json:"content" example:"{\"value\": 0.04}"`
	Changed     time.Time `json:"changed" example:"2026-01-15T10:30:00+01:00"`
}

// CreateParamRequest represents the request body for creating a parameter
type CreateParamRequest struct {
	Key         string `json:"key" example:"DynamicAddPrice"`
	Description string `json:"description" example:"Dynamic addition to price"`
	Content     string `json:"content" example:"{\"value\": 0.04}"`
}

// UpdateParamRequest represents the request body for updating a parameter
type UpdateParamRequest struct {
	Description string `json:"description" example:"Dynamic addition to price"`
	Content     string `json:"content" example:"{\"value\": 0.05}"`
}

// DefaultParams defines the default rows that should exist in the parameters table
var DefaultParams = []PersistentParam{
	{Key: "TransferAddPrice", Description: "Electricity transfer addition to price", Content: `{"value": 0.2584}`},
	{Key: "EnergyTaxAddPrice", Description: "Energy tax  addition to price", Content: `{"value": 0.36}`},
	{Key: "DynamicAddPrice", Description: "Dynamic addition to price", Content: `{"value": 0.0442}`},
	{Key: "StaticAddPrice", Description: "Static addition to price", Content: `{"value": 0.04}`},
	{Key: "VAT", Description: "VAT percent", Content: `{"value": 25}`},
	{Key: "grid_benefit", Description: "Grid production benefit", Content: `{"value": 0.0844}`},
	{Key: "eon_added", Description: "EON production addition", Content: `{"value": 0.02}`},
	{Key: "location_name", Description: "Location name", Content: `{"value": "Sotehus"}`},
	{Key: "use_local_mqtt_solar", Description: "Use local MQTT data for solar production", Content: `{"value": true}`},
}

// ParseContentValue extracts the numeric "value" field from a parameter's JSON content string.
// Content is expected to be in the format: {"value": <number>}
func ParseContentValue(content string) (decimal.Decimal, error) {
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return decimal.Zero, fmt.Errorf("invalid JSON content: %w", err)
	}

	raw, ok := parsed["value"]
	if !ok {
		return decimal.Zero, fmt.Errorf("missing \"value\" key in content")
	}

	// Try parsing as a decimal number directly from the raw JSON token
	var num json.Number
	if err := json.Unmarshal(raw, &num); err != nil {
		return decimal.Zero, fmt.Errorf("\"value\" is not a number")
	}

	d, err := decimal.NewFromString(num.String())
	if err != nil {
		return decimal.Zero, fmt.Errorf("\"value\" is not a valid decimal: %w", err)
	}
	return d, nil
}

// ParseContentBool extracts the boolean "value" field from a parameter's JSON content string.
// Content is expected to be in the format: {"value": true} or {"value": false}
func ParseContentBool(content string) (bool, error) {
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return false, fmt.Errorf("invalid JSON content: %w", err)
	}

	raw, ok := parsed["value"]
	if !ok {
		return false, fmt.Errorf("missing \"value\" key in content")
	}

	var val bool
	if err := json.Unmarshal(raw, &val); err != nil {
		return false, fmt.Errorf("\"value\" is not a boolean: %w", err)
	}
	return val, nil
}
