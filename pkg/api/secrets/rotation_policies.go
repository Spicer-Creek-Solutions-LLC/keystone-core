package secrets

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/credentials"
	"github.com/shawnbutts/keystone-core/internal/credentials/rotation"
)

// RotationPolicyManager defines the interface for rotation policy operations.
type RotationPolicyManager interface {
	AddPolicy(policy *rotation.Policy) error
	GetPolicy(id string) (*rotation.Policy, bool)
	ListPolicies() []*rotation.Policy
	RemovePolicy(id string) error
	UpdatePolicy(policy *rotation.Policy) error
}

// handleRotationPoliciesList handles GET/POST /api/v1/secrets/rotation/policies.
func (h *Handler) handleRotationPoliciesList(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/v1/secrets/rotation/policies" {
		h.handleRotationPolicyRoute(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.handleListRotationPolicies(w, r)
	case http.MethodPost:
		h.handleCreateRotationPolicy(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleRotationPolicyRoute handles routes under /api/v1/secrets/rotation/policies/{id}...
func (h *Handler) handleRotationPolicyRoute(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/secrets/rotation/policies/")
	if path == "" {
		h.handleListRotationPolicies(w, r)
		return
	}

	parts := strings.SplitN(path, "/", 2)
	policyID := parts[0]

	if len(parts) == 2 {
		switch parts[1] {
		case "enable":
			h.handleEnableRotationPolicy(w, r, policyID)
			return
		case "disable":
			h.handleDisableRotationPolicy(w, r, policyID)
			return
		}
	}

	switch r.Method {
	case http.MethodGet:
		h.handleGetRotationPolicy(w, r, policyID)
	case http.MethodDelete:
		h.handleDeleteRotationPolicy(w, r, policyID)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Handler) handleListRotationPolicies(w http.ResponseWriter, _ *http.Request) {
	if h.rotationPolicyEngine == nil {
		writeError(w, http.StatusServiceUnavailable, "rotation policy engine not available")
		return
	}

	policies := h.rotationPolicyEngine.ListPolicies()
	result := make([]RotationPolicyResponse, 0, len(policies))
	for _, p := range policies {
		result = append(result, policyToResponse(p))
	}

	writeJSON(w, http.StatusOK, RotationPolicyListResponse{
		Policies: result,
		Total:    len(result),
	})
}

func (h *Handler) handleGetRotationPolicy(w http.ResponseWriter, _ *http.Request, id string) {
	if h.rotationPolicyEngine == nil {
		writeError(w, http.StatusServiceUnavailable, "rotation policy engine not available")
		return
	}

	p, ok := h.rotationPolicyEngine.GetPolicy(id)
	if !ok {
		writeError(w, http.StatusNotFound, "policy not found")
		return
	}

	writeJSON(w, http.StatusOK, policyToResponse(p))
}

func (h *Handler) handleCreateRotationPolicy(w http.ResponseWriter, r *http.Request) {
	if h.rotationPolicyEngine == nil {
		writeError(w, http.StatusServiceUnavailable, "rotation policy engine not available")
		return
	}

	var req CreateRotationPolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" || req.MaxAge == "" {
		writeError(w, http.StatusBadRequest, "name and max_age are required")
		return
	}

	maxAge, err := parseDurationString(req.MaxAge)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid max_age: %v", err))
		return
	}

	var warningAge time.Duration
	if req.WarningAge != "" {
		warningAge, err = parseDurationString(req.WarningAge)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid warning_age: %v", err))
			return
		}
	}

	credTypes := make([]credentials.CredentialType, 0, len(req.CredentialTypes))
	for _, ct := range req.CredentialTypes {
		credTypes = append(credTypes, credentials.CredentialType(ct))
	}
	if len(credTypes) == 0 {
		credTypes = []credentials.CredentialType{"*"}
	}

	policy := &rotation.Policy{
		ID:              req.ID,
		Name:            req.Name,
		CredentialTypes:  credTypes,
		MaxAge:          maxAge,
		WarningAge:      warningAge,
		Schedule:        req.Schedule,
		AutoRotate:      req.AutoRotate,
		RollbackOnFail:  req.RollbackOnFail,
		Enabled:         req.Enabled,
	}

	if err := h.rotationPolicyEngine.AddPolicy(policy); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, policyToResponse(policy))
}

func (h *Handler) handleDeleteRotationPolicy(w http.ResponseWriter, _ *http.Request, id string) {
	if h.rotationPolicyEngine == nil {
		writeError(w, http.StatusServiceUnavailable, "rotation policy engine not available")
		return
	}

	if err := h.rotationPolicyEngine.RemovePolicy(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, RotationPolicyActionResponse{
		PolicyID: id,
		Action:   "delete",
		Success:  true,
	})
}

func (h *Handler) handleEnableRotationPolicy(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.rotationPolicyEngine == nil {
		writeError(w, http.StatusServiceUnavailable, "rotation policy engine not available")
		return
	}

	p, ok := h.rotationPolicyEngine.GetPolicy(id)
	if !ok {
		writeError(w, http.StatusNotFound, "policy not found")
		return
	}

	p.Enabled = true
	if err := h.rotationPolicyEngine.UpdatePolicy(p); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, RotationPolicyActionResponse{
		PolicyID: id,
		Action:   "enable",
		Success:  true,
	})
}

func (h *Handler) handleDisableRotationPolicy(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.rotationPolicyEngine == nil {
		writeError(w, http.StatusServiceUnavailable, "rotation policy engine not available")
		return
	}

	p, ok := h.rotationPolicyEngine.GetPolicy(id)
	if !ok {
		writeError(w, http.StatusNotFound, "policy not found")
		return
	}

	p.Enabled = false
	if err := h.rotationPolicyEngine.UpdatePolicy(p); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, RotationPolicyActionResponse{
		PolicyID: id,
		Action:   "disable",
		Success:  true,
	})
}

func policyToResponse(p *rotation.Policy) RotationPolicyResponse {
	credTypes := make([]string, 0, len(p.CredentialTypes))
	for _, ct := range p.CredentialTypes {
		credTypes = append(credTypes, string(ct))
	}

	return RotationPolicyResponse{
		ID:              p.ID,
		Name:            p.Name,
		CredentialTypes: credTypes,
		MaxAge:          p.MaxAge.String(),
		WarningAge:      p.WarningAge.String(),
		Schedule:        p.Schedule,
		AutoRotate:      p.AutoRotate,
		RollbackOnFail:  p.RollbackOnFail,
		Enabled:         p.Enabled,
	}
}

// parseDurationString parses durations like "90d", "30d", "24h", "1h30m".
func parseDurationString(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		s = strings.TrimSuffix(s, "d")
		var days int
		if _, err := fmt.Sscanf(s, "%d", &days); err != nil {
			return 0, fmt.Errorf("invalid day duration: %s", s)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
