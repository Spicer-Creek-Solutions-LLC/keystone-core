// SPDX-License-Identifier: Apache-2.0

// Package maintenance exposes REST routes for the maintenance-window
// domain.
//
// v0.x decision (issue #89): these routes are intentionally NOT part
// of the v0.1 REST surface. Per PROJECT-DETAILS §4.5 the maintenance-
// window domain (both gRPC and REST) ships post-v1.0; there is no
// gRPC alternative today either. The routes return 410 Gone with a
// post-v1.0 marker. If the domain lands earlier, lift the 410 in the
// same change that wires the real handlers.
package maintenance

import "net/http"

// Handler exposes REST routes for the maintenance domain.
type Handler struct{}

// NewHandler returns a Handler.
func NewHandler() *Handler { return &Handler{} }

// Register installs the maintenance-domain routes onto mux. Each
// route responds 410 Gone with the post-v1.0 deferral marker.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/maintenance/windows", gonePostV1)
	mux.HandleFunc("POST /api/v1/maintenance/windows", gonePostV1)
	mux.HandleFunc("GET /api/v1/maintenance/windows/{id}", gonePostV1)
	mux.HandleFunc("DELETE /api/v1/maintenance/windows/{id}", gonePostV1)
}

// gonePostV1 is the package response for routes deliberately excluded
// from the v0.1 REST surface where no gRPC alternative exists either.
// The exact wording is asserted by handler_test.go.
func gonePostV1(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "Not part of v0.1; maintenance windows ship post-v1.0.", http.StatusGone)
}
