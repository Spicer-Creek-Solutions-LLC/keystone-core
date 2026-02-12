// Package config provides HTTP handlers for configuration REST API endpoints.
package config

import (
	"encoding/json"
	"net/http"

	"github.com/shawnbutts/keystone-core/internal/config"
	"github.com/shawnbutts/keystone-core/pkg/api/apierror"
)

// Handler provides HTTP handlers for configuration API endpoints.
type Handler struct {
	cfg *config.Config
}

// NewHandler creates a new config API handler.
func NewHandler(cfg *config.Config) *Handler {
	return &Handler{cfg: cfg}
}

// RegisterRoutes registers the config API routes with the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/config", h.handleGetConfig)
}

func (h *Handler) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apierror.Write(w, http.StatusMethodNotAllowed, "only GET is allowed")
		return
	}

	redacted := h.cfg.Redact()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(redacted)
}
