// SPDX-License-Identifier: Apache-2.0

// Package execution exposes REST routes for the command execution
// domain.
//
// v0.x decision (issue #89): these routes are intentionally NOT part
// of the v0.1 REST surface. They return 410 Gone and point callers at
// the gRPC ControlPlaneService (Execute / BatchExecute / status /
// cancel RPCs) instead. If a concrete REST consumer surfaces later,
// revisit and implement passthrough handlers rather than silently
// un-410-ing them.
package execution

import "net/http"

// Handler exposes REST routes for the execution domain.
type Handler struct{}

// NewHandler returns a Handler.
func NewHandler() *Handler { return &Handler{} }

// Register installs the execution-domain routes onto mux. Each route
// responds 410 Gone with the gRPC alternative.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/execution/commands", goneGRPCOnly)
	mux.HandleFunc("POST /api/v1/execution/batch", goneGRPCOnly)
	mux.HandleFunc("GET /api/v1/execution/commands", goneGRPCOnly)
	mux.HandleFunc("GET /api/v1/execution/commands/{id}", goneGRPCOnly)
	mux.HandleFunc("DELETE /api/v1/execution/commands/{id}", goneGRPCOnly)
}

// goneGRPCOnly is the package response for routes deliberately
// excluded from the v0.1 REST surface. The exact wording is asserted
// by handler_test.go.
func goneGRPCOnly(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "Not part of v0.1; use the gRPC ControlPlaneService instead.", http.StatusGone)
}
