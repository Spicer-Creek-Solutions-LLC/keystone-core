// Package gitops exposes REST routes for the GitOps integration
// domain.
//
// v1.0 scaffold — concrete handlers ship with epic 16 (GitOps
// Webhooks).
package gitops

import "net/http"

// Handler exposes REST routes for the gitops domain.
type Handler struct{}

// NewHandler returns a Handler.
func NewHandler() *Handler { return &Handler{} }

// Register installs the gitops-domain routes onto mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/gitops/repos", notImplemented)
	mux.HandleFunc("POST /api/v1/gitops/repos", notImplemented)
	mux.HandleFunc("DELETE /api/v1/gitops/repos/{id}", notImplemented)
	mux.HandleFunc("POST /api/v1/gitops/sync", notImplemented)
	mux.HandleFunc("POST /api/v1/gitops/webhook", notImplemented)
}

func notImplemented(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "endpoint not yet implemented", http.StatusNotImplemented)
}
