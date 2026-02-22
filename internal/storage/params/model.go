package params

import "time"

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
	{Key: "DynamicAddPrice", Description: "Dynamic addition to price", Content: `{"value": 0.04}`},
	{Key: "StaticAddPrice", Description: "Static addition to price", Content: `{"value": 0.06}`},
}
