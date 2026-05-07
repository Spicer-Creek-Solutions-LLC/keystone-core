// Package cluster exposes REST routes for the cluster domain.
//
// v1.0 scaffold — concrete handlers ship with epic 13 (Clustering &
// HA). Stubs return 501 Not Implemented.
package cluster

import "net/http"

// Handler exposes REST routes for the cluster domain.
type Handler struct{}

// NewHandler returns a Handler.
func NewHandler() *Handler { return &Handler{} }

// Register installs the cluster-domain routes onto mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/cluster/status", notImplemented)
	mux.HandleFunc("GET /api/v1/cluster/leader", notImplemented)
	mux.HandleFunc("GET /api/v1/cluster/members", notImplemented)
	mux.HandleFunc("POST /api/v1/cluster/members", notImplemented)
	mux.HandleFunc("GET /api/v1/cluster/members/{id}", notImplemented)
	mux.HandleFunc("DELETE /api/v1/cluster/members/{id}", notImplemented)
	mux.HandleFunc("POST /api/v1/cluster/backup", notImplemented)
	mux.HandleFunc("POST /api/v1/cluster/restore", notImplemented)
}

func notImplemented(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "endpoint not yet implemented", http.StatusNotImplemented)
}
