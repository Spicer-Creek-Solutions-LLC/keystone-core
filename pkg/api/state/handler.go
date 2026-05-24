// SPDX-License-Identifier: Apache-2.0

// Package state exposes REST routes for the state-management domain.
//
// v1.0 scaffold — concrete handlers ship with epic 08 (State
// Management & Stdlib).
//
// Naming note: this package name collides with internal/state. Files
// importing both must alias one (e.g.,
// `coreState "go.keystone-core.io/keystone-core/internal/state"`).
package state

import "net/http"

// Handler exposes REST routes for the state-management domain.
type Handler struct{}

// NewHandler returns a Handler.
func NewHandler() *Handler { return &Handler{} }

// Register installs the state-management routes onto mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/state/apply", notImplemented)
	mux.HandleFunc("POST /api/v1/state/check", notImplemented)
	mux.HandleFunc("POST /api/v1/state/drift", notImplemented)
	mux.HandleFunc("GET /api/v1/state/runs", notImplemented)
	mux.HandleFunc("GET /api/v1/state/runs/{id}", notImplemented)
}

func notImplemented(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "endpoint not yet implemented", http.StatusNotImplemented)
}
