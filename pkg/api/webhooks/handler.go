// Package webhooks exposes REST routes for the outbound-webhooks
// domain.
//
// v1.0 scaffold — concrete handlers ship with epic 14 (Outbound
// Webhooks).
package webhooks

import "net/http"

// Handler exposes REST routes for the webhooks domain.
type Handler struct{}

// NewHandler returns a Handler.
func NewHandler() *Handler { return &Handler{} }

// Register installs the webhooks-domain routes onto mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/webhooks", notImplemented)
	mux.HandleFunc("POST /api/v1/webhooks", notImplemented)
	mux.HandleFunc("GET /api/v1/webhooks/{id}", notImplemented)
	mux.HandleFunc("DELETE /api/v1/webhooks/{id}", notImplemented)
	mux.HandleFunc("POST /api/v1/webhooks/{id}/test", notImplemented)
}

func notImplemented(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "endpoint not yet implemented", http.StatusNotImplemented)
}
