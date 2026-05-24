// SPDX-License-Identifier: Apache-2.0

// Package gitops exposes the v1.0 GitOps REST routes per
// PROJECT-DETAILS §4.13 (Epic 16 task 10): rollback execute / list /
// get / approve / reject, plus verification history list / get.
// Backends are injected via [Providers]; a nil provider degrades its
// routes to 503 (the pkg/api/cluster / pkg/api/blueprint precedent —
// routes exist, light up when the server boot-wires real providers,
// tracked by the gate-v1.0 "GitOps rollback boot wiring" ROADMAP
// entry).
package gitops

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"go.keystone-core.io/keystone-core/internal/gitops/rollback"
	"go.keystone-core.io/keystone-core/internal/gitops/verification"
)

// RollbackEngine is the subset of [rollback.Engine] this handler
// uses. *rollback.Engine satisfies it.
type RollbackEngine interface {
	Execute(ctx context.Context, spec rollback.RollbackSpec) (*rollback.Rollback, error)
	ApproveRollback(ctx context.Context, id, approver string) (*rollback.Rollback, error)
	RejectRollback(ctx context.Context, id, approver, reason string) (*rollback.Rollback, error)
	GetRollback(ctx context.Context, id string) (*rollback.Rollback, bool, error)
	ListRollbacks(ctx context.Context) ([]*rollback.Rollback, error)
}

// Providers bundles the (individually nilable) backends.
type Providers struct {
	Rollback      RollbackEngine
	Verifications verification.ResultStore
}

// Handler exposes REST routes for the gitops domain.
type Handler struct{ p Providers }

// NewHandler returns a Handler. Pass a zero Providers for the
// not-yet-wired case (routes return 503).
func NewHandler(p Providers) *Handler { return &Handler{p: p} }

// Register installs the gitops-domain routes onto mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/gitops/rollback", h.rollbackExecute)
	mux.HandleFunc("GET /api/v1/gitops/rollbacks", h.rollbackList)
	mux.HandleFunc("GET /api/v1/gitops/rollbacks/{id}", h.rollbackGet)
	mux.HandleFunc("POST /api/v1/gitops/rollbacks/{id}/approve", h.rollbackApprove)
	mux.HandleFunc("POST /api/v1/gitops/rollbacks/{id}/reject", h.rollbackReject)
	mux.HandleFunc("GET /api/v1/gitops/verifications", h.verificationList)
	mux.HandleFunc("GET /api/v1/gitops/verifications/{id}", h.verificationGet)
}

// --- Rollback routes ---------------------------------------------------------

type rollbackRequestDTO struct {
	ExecutorType    string          `json:"executor_type"`
	Application     string          `json:"application"`
	Strategy        string          `json:"strategy"`
	Revision        string          `json:"revision,omitempty"`
	Reason          string          `json:"reason,omitempty"`
	RequireApproval bool            `json:"require_approval,omitempty"`
	Config          rollback.Config `json:"config,omitempty"`
}

type approveDTO struct {
	Approver string `json:"approver"`
}

type rejectDTO struct {
	Approver string `json:"approver"`
	Reason   string `json:"reason"`
}

func (h *Handler) rollbackExecute(w http.ResponseWriter, r *http.Request) {
	if h.p.Rollback == nil {
		writeUnavailable(w, "rollback engine not configured")
		return
	}
	var req rollbackRequestDTO
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.ExecutorType == "" || req.Application == "" || req.Strategy == "" {
		writeError(w, http.StatusBadRequest, "executor_type, application and strategy are required")
		return
	}
	rb, err := h.p.Rollback.Execute(r.Context(), rollback.RollbackSpec{
		ExecutorType:    req.ExecutorType,
		Config:          req.Config,
		RequireApproval: req.RequireApproval,
		Request: rollback.Request{
			Application: req.Application,
			Strategy:    rollback.Strategy(req.Strategy),
			Revision:    req.Revision,
			Reason:      req.Reason,
		},
	})
	if err != nil {
		if errors.Is(err, rollback.ErrUnknownExecutor) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, rb)
}

func (h *Handler) rollbackList(w http.ResponseWriter, r *http.Request) {
	if h.p.Rollback == nil {
		writeUnavailable(w, "rollback engine not configured")
		return
	}
	list, err := h.p.Rollback.ListRollbacks(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*rollback.Rollback{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) rollbackGet(w http.ResponseWriter, r *http.Request) {
	if h.p.Rollback == nil {
		writeUnavailable(w, "rollback engine not configured")
		return
	}
	rb, ok, err := h.p.Rollback.GetRollback(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "rollback not found")
		return
	}
	writeJSON(w, http.StatusOK, rb)
}

func (h *Handler) rollbackApprove(w http.ResponseWriter, r *http.Request) {
	if h.p.Rollback == nil {
		writeUnavailable(w, "rollback engine not configured")
		return
	}
	var body approveDTO
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rb, err := h.p.Rollback.ApproveRollback(r.Context(), r.PathValue("id"), body.Approver)
	switch {
	case errors.Is(err, rollback.ErrRollbackNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, rollback.ErrInvalidTransition):
		writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error())
	default:
		writeJSON(w, http.StatusOK, rb)
	}
}

func (h *Handler) rollbackReject(w http.ResponseWriter, r *http.Request) {
	if h.p.Rollback == nil {
		writeUnavailable(w, "rollback engine not configured")
		return
	}
	var body rejectDTO
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rb, err := h.p.Rollback.RejectRollback(r.Context(), r.PathValue("id"), body.Approver, body.Reason)
	switch {
	case errors.Is(err, rollback.ErrRollbackNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, rollback.ErrInvalidTransition):
		writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error())
	default:
		writeJSON(w, http.StatusOK, rb)
	}
}

// --- Verification history routes --------------------------------------------

func (h *Handler) verificationList(w http.ResponseWriter, r *http.Request) {
	if h.p.Verifications == nil {
		writeUnavailable(w, "verification result store not configured")
		return
	}
	list, err := h.p.Verifications.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*verification.StoredVerification{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) verificationGet(w http.ResponseWriter, r *http.Request) {
	if h.p.Verifications == nil {
		writeUnavailable(w, "verification result store not configured")
		return
	}
	sv, ok, err := h.p.Verifications.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "verification not found")
		return
	}
	writeJSON(w, http.StatusOK, sv)
}

// --- helpers -----------------------------------------------------------------

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func writeUnavailable(w http.ResponseWriter, msg string) {
	writeError(w, http.StatusServiceUnavailable, msg)
}
