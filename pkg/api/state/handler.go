// SPDX-License-Identifier: Apache-2.0

// Package state exposes REST routes for the state-management domain.
//
// v0.x decision (issue #89): these routes are intentionally NOT part
// of the v0.1 REST surface. They return 410 Gone and point callers at
// the gRPC StateService instead. If a concrete REST consumer surfaces
// later, revisit and implement passthrough handlers rather than
// silently un-410-ing them.
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

// Register installs the state-management routes onto mux. Each route
// responds 410 Gone with the gRPC alternative.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/state/apply", goneGRPCOnly)
	mux.HandleFunc("POST /api/v1/state/check", goneGRPCOnly)
	mux.HandleFunc("POST /api/v1/state/drift", goneGRPCOnly)
	mux.HandleFunc("GET /api/v1/state/runs", goneGRPCOnly)
	mux.HandleFunc("GET /api/v1/state/runs/{id}", goneGRPCOnly)
}

// goneGRPCOnly is the package response for routes deliberately
// excluded from the v0.1 REST surface. The exact wording is asserted
// by handler_test.go.
func goneGRPCOnly(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "Not part of v0.1; use the gRPC StateService instead.", http.StatusGone)
}
