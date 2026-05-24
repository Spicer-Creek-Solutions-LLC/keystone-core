// SPDX-License-Identifier: Apache-2.0

package apikeys

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"go.keystone-core.io/keystone-core/internal/state"
)

// Handler exposes REST routes for API key CRUD:
//
//	POST   /api/v1/apikeys           create + return cleartext (once!)
//	GET    /api/v1/apikeys           list (metadata only)
//	GET    /api/v1/apikeys/{id}      get one (metadata only)
//	DELETE /api/v1/apikeys/{id}      delete
//
// Authentication + RBAC are enforced by the auth interceptor /
// middleware (epic 03 task 4); this handler trusts incoming requests
// to have already passed those checks.
type Handler struct {
	store state.APIKeyStore
	now   func() time.Time
}

// NewHandler returns a Handler backed by store.
func NewHandler(store state.APIKeyStore) *Handler {
	return &Handler{store: store, now: time.Now}
}

// SetClock overrides the clock used to stamp CreatedAt on new keys.
// Tests only.
func (h *Handler) SetClock(now func() time.Time) {
	h.now = now
}

// Register installs the four routes onto mux. The Go 1.22+ method-aware
// patterns (e.g., "POST /...") gate by HTTP verb; mux returns 405 on
// mismatched verbs without the handler having to check.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/apikeys", h.handleCreate)
	mux.HandleFunc("GET /api/v1/apikeys", h.handleList)
	mux.HandleFunc("GET /api/v1/apikeys/{id}", h.handleGet)
	mux.HandleFunc("DELETE /api/v1/apikeys/{id}", h.handleDelete)
}

// ---- request/response shapes ----------------------------------------------

// createRequest is the POST body shape.
type createRequest struct {
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

// apiKeyResponse is the metadata-only response shape (GET / list).
// Cleartext + key_hash are NEVER in this shape.
type apiKeyResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	LastUsed  time.Time `json:"last_used,omitempty"`
}

// createResponse is the POST response. Embeds the metadata fields and
// adds `key` — the cleartext, returned exactly once at creation.
type createResponse struct {
	apiKeyResponse
	Key string `json:"key"` // cleartext; returned once on creation
}

// listResponse wraps the list result.
type listResponse struct {
	APIKeys []apiKeyResponse `json:"apikeys"`
}

// errorResponse is the JSON error shape.
type errorResponse struct {
	Error string `json:"error"`
}

// ---- handlers -------------------------------------------------------------

func (h *Handler) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	gen, err := generateAt(req.Name, req.Role, req.ExpiresAt, h.now)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.store.CreateAPIKey(r.Context(), gen.Record()); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "create: "+err.Error())
		return
	}

	resp := createResponse{
		apiKeyResponse: toAPIKeyResponse(gen.Record()),
		Key:            gen.Cleartext,
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	rec, err := h.store.ListAPIKeys(r.Context(), state.APIKeyFilter{})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "list: "+err.Error())
		return
	}
	out := make([]apiKeyResponse, len(rec))
	for i, k := range rec {
		out[i] = toAPIKeyResponse(k)
	}
	writeJSON(w, http.StatusOK, listResponse{APIKeys: out})
}

func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "missing id")
		return
	}
	rec, err := h.store.GetAPIKey(r.Context(), id)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, fmt.Sprintf("apikey %q not found", id))
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "get: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toAPIKeyResponse(rec))
}

func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "missing id")
		return
	}
	if err := h.store.DeleteAPIKey(r.Context(), id); err != nil {
		if errors.Is(err, state.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, fmt.Sprintf("apikey %q not found", id))
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "delete: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- helpers --------------------------------------------------------------

// toAPIKeyResponse builds the metadata-only response shape from a
// stored record. Excludes KeyHash by construction so it can never
// leak through the REST surface.
func toAPIKeyResponse(rec *state.APIKeyRecord) apiKeyResponse {
	return apiKeyResponse{
		ID:        rec.ID,
		Name:      rec.Name,
		Role:      rec.Role,
		CreatedAt: rec.CreatedAt,
		ExpiresAt: rec.ExpiresAt,
		LastUsed:  rec.LastUsed,
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}
