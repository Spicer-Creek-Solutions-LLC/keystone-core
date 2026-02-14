package secrets

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/secrets"
)

// handleRotationsList handles GET/POST /api/v1/rotations (exact path).
func (h *Handler) handleRotationsList(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/v1/rotations" {
		h.handleRotationsRoute(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.handleListRotations(w, r)
	case http.MethodPost:
		h.handleStartRotation(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Handler) handleListRotations(w http.ResponseWriter, _ *http.Request) {
	if h.orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "rotation orchestrator not available")
		return
	}

	managed := h.orchestrator.ListRotations()
	rotations := make([]RotationResponse, 0, len(managed))
	for _, mr := range managed {
		rotations = append(rotations, managedRotationToResponse(mr))
	}

	writeJSON(w, http.StatusOK, RotationListResponse{
		Rotations: rotations,
		Total:     len(rotations),
	})
}

// handleRotationsRoute handles routes under /api/v1/rotations/{id}...
func (h *Handler) handleRotationsRoute(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/rotations/")
	if path == "" {
		writeError(w, http.StatusBadRequest, "rotation ID is required")
		return
	}

	parts := strings.SplitN(path, "/", 2)
	rotationID := parts[0]

	if len(parts) == 2 {
		switch parts[1] {
		case "cancel":
			h.handleCancelRotation(w, r, rotationID) //nolint:contextcheck // Cancel uses state machine internally
			return
		case "rollback":
			h.handleRollbackRotation(w, r, rotationID) //nolint:contextcheck // Rollback uses state machine internally
			return
		case "pause":
			h.handlePauseRotation(w, r, rotationID)
			return
		case "resume":
			h.handleResumeRotation(w, r, rotationID)
			return
		case "trigger":
			h.handleTriggerRotation(w, r, rotationID)
			return
		}
	}

	// GET /api/v1/rotations/{id}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	h.handleGetRotation(w, r, rotationID)
}

func (h *Handler) handleStartRotation(w http.ResponseWriter, r *http.Request) {
	if h.orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "rotation orchestrator not available")
		return
	}

	var req StartRotationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ID == "" || req.SecretPath == "" || req.Config.Strategy == "" {
		writeError(w, http.StatusBadRequest, "id, secret_path, and config.strategy are required")
		return
	}

	config := &secrets.RotationConfig{
		Strategy:          secrets.RotationStrategy(req.Config.Strategy),
		BatchSize:         req.Config.BatchSize,
		BatchDelay:        time.Duration(req.Config.BatchDelaySeconds) * time.Second,
		RollbackOnFailure: req.Config.RollbackOnFailure,
		FailureThreshold:  req.Config.FailureThreshold,
		GracePeriod:       time.Duration(req.Config.GracePeriodSeconds) * time.Second,
	}

	targets := make([]*secrets.RotationTarget, len(req.Targets))
	for i, t := range req.Targets {
		targets[i] = &secrets.RotationTarget{
			ID:       t.ID,
			Name:     t.Name,
			Type:     t.Type,
			AgentID:  t.AgentID,
			Endpoint: t.Endpoint,
		}
	}

	managed, err := h.orchestrator.StartRotation(r.Context(), req.ID, req.SecretPath, config, targets)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, managedRotationToResponse(managed))
}

func (h *Handler) handleGetRotation(w http.ResponseWriter, _ *http.Request, rotationID string) {
	if h.orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "rotation orchestrator not available")
		return
	}

	managed, ok := h.orchestrator.GetRotation(rotationID)
	if !ok {
		writeError(w, http.StatusNotFound, "rotation not found")
		return
	}

	writeJSON(w, http.StatusOK, managedRotationToResponse(managed))
}

func (h *Handler) handleCancelRotation(w http.ResponseWriter, r *http.Request, rotationID string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "rotation orchestrator not available")
		return
	}

	if err := h.orchestrator.CancelRotation(rotationID); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, RotationActionResponse{
		RotationID: rotationID,
		Action:     "cancel",
		Success:    true,
	})
}

func (h *Handler) handleRollbackRotation(w http.ResponseWriter, r *http.Request, rotationID string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "rotation orchestrator not available")
		return
	}

	if err := h.orchestrator.RollbackRotation(rotationID); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, RotationActionResponse{
		RotationID: rotationID,
		Action:     "rollback",
		Success:    true,
	})
}

func (h *Handler) handlePauseRotation(w http.ResponseWriter, r *http.Request, rotationID string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "rotation orchestrator not available")
		return
	}

	if _, ok := h.orchestrator.GetRotation(rotationID); !ok {
		writeError(w, http.StatusNotFound, "rotation not found")
		return
	}

	writeError(w, http.StatusNotImplemented, "pause is not yet supported by the rotation engine")
}

func (h *Handler) handleResumeRotation(w http.ResponseWriter, r *http.Request, rotationID string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "rotation orchestrator not available")
		return
	}

	if _, ok := h.orchestrator.GetRotation(rotationID); !ok {
		writeError(w, http.StatusNotFound, "rotation not found")
		return
	}

	writeError(w, http.StatusNotImplemented, "resume is not yet supported by the rotation engine")
}

func (h *Handler) handleTriggerRotation(w http.ResponseWriter, r *http.Request, rotationID string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "rotation orchestrator not available")
		return
	}

	if _, ok := h.orchestrator.GetRotation(rotationID); !ok {
		writeError(w, http.StatusNotFound, "rotation not found")
		return
	}

	writeError(w, http.StatusNotImplemented, "trigger is not yet supported; rotation scheduler not wired")
}

// managedRotationToResponse builds a response using ManagedRotation's
// thread-safe accessors to avoid racing with the async execution goroutine.
func managedRotationToResponse(mr *secrets.ManagedRotation) RotationResponse {
	progress := mr.GetProgress()
	state := mr.State()

	// Rotation.ID, SecretPath, Strategy are immutable after creation.
	resp := RotationResponse{
		ID:             mr.Rotation.ID,
		SecretPath:     mr.Rotation.SecretPath,
		State:          string(state),
		Strategy:       string(mr.Rotation.Strategy),
		StartedAt:      mr.Rotation.StartedAt,
		TotalTargets:   progress.TotalTargets,
		UpdatedTargets: progress.UpdatedTargets,
		FailedTargets:  progress.FailedTargets,
	}

	if err := mr.Error(); err != nil {
		resp.Error = err.Error()
	}

	return resp
}
