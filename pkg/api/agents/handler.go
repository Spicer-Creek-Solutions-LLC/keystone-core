// SPDX-License-Identifier: Apache-2.0

// Package agents exposes REST routes for the agents domain.
//
// v1.0 scaffold — concrete handlers ship with epic 06 (Agent Runtime).
// The stubs here register the v1.0 path set with 501 Not Implemented
// so the route surface is probeable + reviewable in advance.
package agents

import "net/http"

// Handler exposes REST routes for the agents domain.
type Handler struct{}

// NewHandler returns a Handler. Owning epic extends the constructor
// with its real dependencies (state.AgentStore etc.) when the
// concrete implementation lands.
func NewHandler() *Handler { return &Handler{} }

// Register installs the agents-domain routes onto mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/agents", notImplemented)
	mux.HandleFunc("GET /api/v1/agents/{id}", notImplemented)
	mux.HandleFunc("DELETE /api/v1/agents/{id}", notImplemented)
}

func notImplemented(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "endpoint not yet implemented", http.StatusNotImplemented)
}
