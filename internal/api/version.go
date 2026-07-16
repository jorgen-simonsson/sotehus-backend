package api

import (
	"encoding/json"
	"net/http"
)

// APIVersion is the current version of the API
const APIVersion = "2.2.0"

// VersionResponse represents the version response
type VersionResponse struct {
	Version string `json:"version" example:"1.2.0"`
}

// GetVersion handles GET /api/version requests
// @Summary Get API version
// @Description Returns the current version of the API
// @Tags system
// @Produce json
// @Success 200 {object} VersionResponse
// @Router /api/version [get]
func (h *Handler) GetVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(VersionResponse{Version: APIVersion})
}
