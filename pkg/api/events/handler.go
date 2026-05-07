// Package events exposes REST routes for the events domain.
//
// v1.0 scaffold — concrete handlers ship with epic 11 (Event System).
package events

import "net/http"

// Handler exposes REST routes for the events domain.
type Handler struct{}

// NewHandler returns a Handler.
func NewHandler() *Handler { return &Handler{} }

// Register installs the events-domain routes onto mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/events", notImplemented)
	mux.HandleFunc("GET /api/v1/events/{id}", notImplemented)
	mux.HandleFunc("POST /api/v1/events", notImplemented)
	mux.HandleFunc("GET /api/v1/events/types", notImplemented)
	mux.HandleFunc("GET /api/v1/events/stats", notImplemented)
}

func notImplemented(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "endpoint not yet implemented", http.StatusNotImplemented)
}
