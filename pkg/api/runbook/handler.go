// Package runbook exposes REST routes for the runbook domain.
//
// v1.0 scaffold — concrete handlers ship with epic 15 (Blueprints &
// Runbooks).
package runbook

import "net/http"

// Handler exposes REST routes for the runbook domain.
type Handler struct{}

// NewHandler returns a Handler.
func NewHandler() *Handler { return &Handler{} }

// Register installs the runbook-domain routes onto mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/runbooks", notImplemented)
	mux.HandleFunc("GET /api/v1/runbooks/{id}", notImplemented)
	mux.HandleFunc("POST /api/v1/runbooks", notImplemented)
	mux.HandleFunc("DELETE /api/v1/runbooks/{id}", notImplemented)
	mux.HandleFunc("POST /api/v1/runbooks/{id}/run", notImplemented)
}

func notImplemented(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "endpoint not yet implemented", http.StatusNotImplemented)
}
