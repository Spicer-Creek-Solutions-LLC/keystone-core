// Package execution exposes REST routes for the command execution
// domain.
//
// v1.0 scaffold — concrete handlers ship with epic 07 (Remote
// Execution & Targeting).
package execution

import "net/http"

// Handler exposes REST routes for the execution domain.
type Handler struct{}

// NewHandler returns a Handler.
func NewHandler() *Handler { return &Handler{} }

// Register installs the execution-domain routes onto mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/execution/commands", notImplemented)
	mux.HandleFunc("POST /api/v1/execution/batch", notImplemented)
	mux.HandleFunc("GET /api/v1/execution/commands", notImplemented)
	mux.HandleFunc("GET /api/v1/execution/commands/{id}", notImplemented)
	mux.HandleFunc("DELETE /api/v1/execution/commands/{id}", notImplemented)
}

func notImplemented(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "endpoint not yet implemented", http.StatusNotImplemented)
}
