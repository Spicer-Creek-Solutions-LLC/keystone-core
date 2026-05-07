// Package schedule exposes REST routes for the scheduled-jobs
// domain.
//
// v1.0 scaffold — concrete handlers ship in v1.1 (per
// PROJECT-DETAILS §4.5). The stub directory exists at v1.0 so the
// route surface is reserved.
package schedule

import "net/http"

// Handler exposes REST routes for the schedule domain.
type Handler struct{}

// NewHandler returns a Handler.
func NewHandler() *Handler { return &Handler{} }

// Register installs the schedule-domain routes onto mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/schedules", notImplemented)
	mux.HandleFunc("POST /api/v1/schedules", notImplemented)
	mux.HandleFunc("GET /api/v1/schedules/{id}", notImplemented)
	mux.HandleFunc("DELETE /api/v1/schedules/{id}", notImplemented)
}

func notImplemented(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "endpoint not yet implemented", http.StatusNotImplemented)
}
