// Package policy exposes REST routes for the policy domain.
//
// v1.0 scaffold — concrete handlers ship with epic 12 (Audit &
// Policy). Audit-mode evaluation only at v1.0; full enforcement +
// CRUD at v1.8.
package policy

import "net/http"

// Handler exposes REST routes for the policy domain.
type Handler struct{}

// NewHandler returns a Handler.
func NewHandler() *Handler { return &Handler{} }

// Register installs the policy-domain routes onto mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/policies", notImplemented)
	mux.HandleFunc("GET /api/v1/policies/{id}", notImplemented)
	mux.HandleFunc("POST /api/v1/policies/evaluate", notImplemented)
	mux.HandleFunc("GET /api/v1/policies/violations", notImplemented)
	mux.HandleFunc("GET /api/v1/policies/compliance", notImplemented)
	mux.HandleFunc("GET /api/v1/policies/audit", notImplemented)
}

func notImplemented(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "endpoint not yet implemented", http.StatusNotImplemented)
}
