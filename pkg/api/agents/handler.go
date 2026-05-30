// SPDX-License-Identifier: Apache-2.0

// Package agents exposes REST routes for the agents domain.
//
// v0.x decision (issue #89): these routes are intentionally NOT part
// of the v0.1 REST surface. They return 410 Gone and point callers at
// the gRPC ControlPlaneService instead. The operator-path use case
// that originally motivated /api/v1/agents is now served by
// `kscorectl agent list` (issue #88, PR #139). If a concrete REST
// consumer surfaces later, revisit and implement passthrough handlers
// rather than silently un-410-ing them.
package agents

import "net/http"

// Handler exposes REST routes for the agents domain.
type Handler struct{}

// NewHandler returns a Handler.
func NewHandler() *Handler { return &Handler{} }

// Register installs the agents-domain routes onto mux. Each route
// responds 410 Gone with the gRPC alternative.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/agents", goneGRPCOnly)
	mux.HandleFunc("GET /api/v1/agents/{id}", goneGRPCOnly)
	mux.HandleFunc("DELETE /api/v1/agents/{id}", goneGRPCOnly)
}

// goneGRPCOnly is the package response for routes deliberately
// excluded from the v0.1 REST surface. The exact wording is asserted
// by handler_test.go.
func goneGRPCOnly(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "Not part of v0.1; use the gRPC ControlPlaneService instead.", http.StatusGone)
}
