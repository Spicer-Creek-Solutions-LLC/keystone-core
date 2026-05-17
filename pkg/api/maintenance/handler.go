// Package maintenance exposes REST routes for the maintenance-window
// domain.
//
// v1.0 scaffold — concrete handlers ship in post-v1.0 (per
// PROJECT-DETAILS §4.5 the gRPC + REST land together at post-v1.0). The
// stub directory exists at v1.0 so the route surface is reserved.
package maintenance

import "net/http"

// Handler exposes REST routes for the maintenance domain.
type Handler struct{}

// NewHandler returns a Handler.
func NewHandler() *Handler { return &Handler{} }

// Register installs the maintenance-domain routes onto mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/maintenance/windows", notImplemented)
	mux.HandleFunc("POST /api/v1/maintenance/windows", notImplemented)
	mux.HandleFunc("GET /api/v1/maintenance/windows/{id}", notImplemented)
	mux.HandleFunc("DELETE /api/v1/maintenance/windows/{id}", notImplemented)
}

func notImplemented(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "endpoint not yet implemented", http.StatusNotImplemented)
}
