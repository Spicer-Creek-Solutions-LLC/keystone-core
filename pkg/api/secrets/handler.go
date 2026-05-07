// Package secrets exposes REST routes for the secrets domain.
//
// v1.0 scaffold — concrete handlers ship with epic 10 (Secrets
// Management). Secret paths are hierarchical
// (e.g., production/db/postgres) so the path-segment routes use
// Go 1.22+ ServeMux's wildcard pattern {path...} to match
// multi-segment paths.
package secrets

import "net/http"

// Handler exposes REST routes for the secrets domain.
type Handler struct{}

// NewHandler returns a Handler.
func NewHandler() *Handler { return &Handler{} }

// Register installs the secrets-domain routes onto mux.
func (h *Handler) Register(mux *http.ServeMux) {
	// KV ops (path can include slashes — wildcard match).
	mux.HandleFunc("GET /api/v1/secrets/{path...}", notImplemented)
	mux.HandleFunc("PUT /api/v1/secrets/{path...}", notImplemented)
	mux.HandleFunc("DELETE /api/v1/secrets/{path...}", notImplemented)

	// Lease ops.
	mux.HandleFunc("GET /api/v1/leases/{id}", notImplemented)
	mux.HandleFunc("GET /api/v1/leases", notImplemented)
	mux.HandleFunc("POST /api/v1/leases/{id}/renew", notImplemented)
	mux.HandleFunc("POST /api/v1/leases/{id}/revoke", notImplemented)

	// Transit ops.
	mux.HandleFunc("POST /api/v1/transit/encrypt", notImplemented)
	mux.HandleFunc("POST /api/v1/transit/decrypt", notImplemented)
	mux.HandleFunc("POST /api/v1/transit/sign", notImplemented)
	mux.HandleFunc("POST /api/v1/transit/verify", notImplemented)
}

func notImplemented(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "endpoint not yet implemented", http.StatusNotImplemented)
}
