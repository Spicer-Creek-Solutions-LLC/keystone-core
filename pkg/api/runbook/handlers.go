// Package runbook provides HTTP handlers for runbook REST API endpoints.
package runbook

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/runbook/approval"
	"github.com/shawnbutts/keystone-core/internal/runbook/intervention"
)

// Handler provides HTTP handlers for runbook API endpoints.
type Handler struct {
	approvalStorage     approval.Storage
	interventionStorage intervention.Storage
	approvalManager     *approval.Manager
	interventionManager *intervention.Manager
}

// NewHandler creates a new runbook API handler.
func NewHandler(
	approvalStorage approval.Storage,
	interventionStorage intervention.Storage,
	approvalManager *approval.Manager,
	interventionManager *intervention.Manager,
) *Handler {
	return &Handler{
		approvalStorage:     approvalStorage,
		interventionStorage: interventionStorage,
		approvalManager:     approvalManager,
		interventionManager: interventionManager,
	}
}

// RegisterRoutes registers the runbook API routes with the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Approval endpoints
	mux.HandleFunc("/api/v1/runbook/approvals", h.handleApprovals)
	mux.HandleFunc("/api/v1/runbook/approvals/", h.handleApproval)

	// Intervention endpoints
	mux.HandleFunc("/api/v1/runbook/interventions", h.handleInterventions)
	mux.HandleFunc("/api/v1/runbook/interventions/", h.handleIntervention)
}

// ============================================================================
// Approval Response Types
// ============================================================================

// ApprovalResponse represents an approval request in API responses.
type ApprovalResponse struct {
	ID            string                 `json:"id"`
	ExecutionID   string                 `json:"execution_id"`
	StepName      string                 `json:"step_name"`
	State         string                 `json:"state"`
	Title         string                 `json:"title"`
	Description   string                 `json:"description,omitempty"`
	Approvers     []string               `json:"approvers"`
	Mode          string                 `json:"mode"`
	RequiredCount int                    `json:"required_count"`
	Responses     []ApprovalDecision     `json:"responses,omitempty"`
	ExpiresAt     *time.Time             `json:"expires_at,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
	CompletedAt   *time.Time             `json:"completed_at,omitempty"`
}

// ApprovalDecision represents an individual approval decision.
type ApprovalDecision struct {
	Approver    string    `json:"approver"`
	Decision    string    `json:"decision"`
	Comment     string    `json:"comment,omitempty"`
	RespondedAt time.Time `json:"responded_at"`
}

// ApprovalListResponse represents the list approvals API response.
type ApprovalListResponse struct {
	Approvals   []ApprovalResponse `json:"approvals"`
	Total       int                `json:"total"`
	Limit       int                `json:"limit"`
	Offset      int                `json:"offset"`
	RetrievedAt time.Time          `json:"retrieved_at"`
}

// ApproveRequest represents the request body for approving a request.
type ApproveRequest struct {
	Approver string `json:"approver"`
	Comment  string `json:"comment,omitempty"`
}

// RejectRequest represents the request body for rejecting a request.
type RejectRequest struct {
	Approver string `json:"approver"`
	Comment  string `json:"comment"`
}

// DelegateRequest represents the request body for delegating approval.
type DelegateRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// ============================================================================
// Intervention Response Types
// ============================================================================

// InterventionResponse represents an intervention request in API responses.
type InterventionResponse struct {
	ID          string                 `json:"id"`
	ExecutionID string                 `json:"execution_id"`
	StepName    string                 `json:"step_name"`
	Type        string                 `json:"type"`
	State       string                 `json:"state"`
	Title       string                 `json:"title"`
	Description string                 `json:"description,omitempty"`
	Prompts     []PromptFieldResponse  `json:"prompts,omitempty"`
	Response    *InterventionResp      `json:"response,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
}

// PromptFieldResponse represents a prompt field in API responses.
type PromptFieldResponse struct {
	Name        string              `json:"name"`
	Label       string              `json:"label,omitempty"`
	Type        string              `json:"type"`
	Required    bool                `json:"required"`
	Default     interface{}         `json:"default,omitempty"`
	Description string              `json:"description,omitempty"`
	Options     []OptionResponse    `json:"options,omitempty"`
	Validation  *ValidationResponse `json:"validation,omitempty"`
}

// OptionResponse represents a select option in API responses.
type OptionResponse struct {
	Value interface{} `json:"value"`
	Label string      `json:"label"`
}

// ValidationResponse represents field validation in API responses.
type ValidationResponse struct {
	Pattern string   `json:"pattern,omitempty"`
	Min     *float64 `json:"min,omitempty"`
	Max     *float64 `json:"max,omitempty"`
}

// InterventionResp represents an intervention response in API responses.
type InterventionResp struct {
	Operator    string                 `json:"operator"`
	Confirmed   bool                   `json:"confirmed"`
	Values      map[string]interface{} `json:"values,omitempty"`
	Comment     string                 `json:"comment,omitempty"`
	RespondedAt time.Time              `json:"responded_at"`
}

// InterventionListResponse represents the list interventions API response.
type InterventionListResponse struct {
	Interventions []InterventionResponse `json:"interventions"`
	Total         int                    `json:"total"`
	Limit         int                    `json:"limit"`
	Offset        int                    `json:"offset"`
	RetrievedAt   time.Time              `json:"retrieved_at"`
}

// RespondRequest represents the request body for responding to an intervention.
type RespondRequest struct {
	Operator  string                 `json:"operator"`
	Confirmed bool                   `json:"confirmed"`
	Values    map[string]interface{} `json:"values,omitempty"`
	Comment   string                 `json:"comment,omitempty"`
}

// ============================================================================
// Approval Handlers
// ============================================================================

// handleApprovals handles GET /api/v1/runbook/approvals
func (h *Handler) handleApprovals(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	h.listApprovals(w, r)
}

// handleApproval handles GET/POST /api/v1/runbook/approvals/{id}[/action]
func (h *Handler) handleApproval(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/runbook/approvals/")
	parts := strings.Split(path, "/")

	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusBadRequest, "Approval ID required")
		return
	}

	approvalID := parts[0]

	if len(parts) == 1 {
		// GET /api/v1/runbook/approvals/{id}
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.getApproval(w, r, approvalID)
		return
	}

	// Action endpoints
	action := parts[1]
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	switch action {
	case "approve":
		h.approveRequest(w, r, approvalID)
	case "reject":
		h.rejectRequest(w, r, approvalID)
	case "delegate":
		h.delegateRequest(w, r, approvalID)
	default:
		writeError(w, http.StatusNotFound, "Unknown action: "+action)
	}
}

func (h *Handler) listApprovals(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query()

	// Parse filters
	opts := approval.ListOptions{}
	if state := query.Get("state"); state != "" {
		opts.State = approval.RequestState(state)
	}
	if execID := query.Get("execution_id"); execID != "" {
		opts.ExecutionID = execID
	}
	if approver := query.Get("approver"); approver != "" {
		opts.Approver = approver
	}

	// Parse pagination
	limit := 50
	if limitStr := query.Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	offset := 0
	if offsetStr := query.Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}
	opts.Limit = limit
	opts.Offset = offset

	// Query approvals
	requests, err := h.approvalManager.ListRequests(ctx, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list approvals: "+err.Error())
		return
	}

	// Build response
	approvalResponses := make([]ApprovalResponse, 0, len(requests))
	for _, req := range requests {
		approvalResponses = append(approvalResponses, convertApproval(req))
	}

	resp := ApprovalListResponse{
		Approvals:   approvalResponses,
		Total:       len(requests),
		Limit:       limit,
		Offset:      offset,
		RetrievedAt: time.Now().UTC(),
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) getApproval(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()

	req, err := h.approvalManager.GetRequest(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get approval: "+err.Error())
		return
	}
	if req == nil {
		writeError(w, http.StatusNotFound, "Approval not found")
		return
	}

	writeJSON(w, http.StatusOK, convertApproval(req))
}

func (h *Handler) approveRequest(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()

	var req ApproveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	if req.Approver == "" {
		writeError(w, http.StatusBadRequest, "approver is required")
		return
	}

	result, err := h.approvalManager.Respond(ctx, id, req.Approver, approval.DecisionApproved, req.Comment)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, convertApproval(result))
}

func (h *Handler) rejectRequest(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()

	var req RejectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	if req.Approver == "" {
		writeError(w, http.StatusBadRequest, "approver is required")
		return
	}
	if req.Comment == "" {
		writeError(w, http.StatusBadRequest, "comment is required")
		return
	}

	result, err := h.approvalManager.Respond(ctx, id, req.Approver, approval.DecisionRejected, req.Comment)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, convertApproval(result))
}

func (h *Handler) delegateRequest(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()

	var req DelegateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	if req.From == "" {
		writeError(w, http.StatusBadRequest, "from is required")
		return
	}
	if req.To == "" {
		writeError(w, http.StatusBadRequest, "to is required")
		return
	}

	// Get the current request
	approvalReq, err := h.approvalManager.GetRequest(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get approval: "+err.Error())
		return
	}
	if approvalReq == nil {
		writeError(w, http.StatusNotFound, "Approval not found")
		return
	}

	if approvalReq.State != approval.RequestStatePending {
		writeError(w, http.StatusBadRequest, "Can only delegate pending requests")
		return
	}

	// Add the delegate to approvers
	approvalReq.Approvers = append(approvalReq.Approvers, req.To)
	approvalReq.UpdatedAt = time.Now()

	// Record delegation in metadata
	if approvalReq.Metadata == nil {
		approvalReq.Metadata = make(map[string]interface{})
	}
	delegations, _ := approvalReq.Metadata["delegations"].([]interface{})
	delegations = append(delegations, map[string]interface{}{
		"from":         req.From,
		"to":           req.To,
		"delegated_at": time.Now().Format(time.RFC3339),
	})
	approvalReq.Metadata["delegations"] = delegations

	// Save the updated request
	if err := h.approvalStorage.SaveRequest(ctx, approvalReq); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to save delegation: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, convertApproval(approvalReq))
}

// ============================================================================
// Intervention Handlers
// ============================================================================

// handleInterventions handles GET /api/v1/runbook/interventions
func (h *Handler) handleInterventions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	h.listInterventions(w, r)
}

// handleIntervention handles GET/POST /api/v1/runbook/interventions/{id}[/action]
func (h *Handler) handleIntervention(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/runbook/interventions/")
	parts := strings.Split(path, "/")

	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusBadRequest, "Intervention ID required")
		return
	}

	interventionID := parts[0]

	if len(parts) == 1 {
		// GET /api/v1/runbook/interventions/{id}
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.getIntervention(w, r, interventionID)
		return
	}

	// Action endpoints
	action := parts[1]
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	switch action {
	case "respond":
		h.respondToIntervention(w, r, interventionID)
	case "cancel":
		h.cancelIntervention(w, r, interventionID)
	default:
		writeError(w, http.StatusNotFound, "Unknown action: "+action)
	}
}

func (h *Handler) listInterventions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query()

	// Parse filters
	opts := intervention.ListOptions{}
	if state := query.Get("state"); state != "" {
		opts.State = intervention.InterventionState(state)
	}
	if execID := query.Get("execution_id"); execID != "" {
		opts.ExecutionID = execID
	}
	if intType := query.Get("type"); intType != "" {
		opts.Type = intervention.InterventionType(intType)
	}

	// Parse pagination
	limit := 50
	if limitStr := query.Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	offset := 0
	if offsetStr := query.Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}
	opts.Limit = limit
	opts.Offset = offset

	// Query interventions
	requests, err := h.interventionManager.ListRequests(ctx, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list interventions: "+err.Error())
		return
	}

	// Build response
	interventionResponses := make([]InterventionResponse, 0, len(requests))
	for _, req := range requests {
		interventionResponses = append(interventionResponses, convertIntervention(req))
	}

	resp := InterventionListResponse{
		Interventions: interventionResponses,
		Total:         len(requests),
		Limit:         limit,
		Offset:        offset,
		RetrievedAt:   time.Now().UTC(),
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) getIntervention(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()

	req, err := h.interventionManager.GetRequest(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get intervention: "+err.Error())
		return
	}
	if req == nil {
		writeError(w, http.StatusNotFound, "Intervention not found")
		return
	}

	writeJSON(w, http.StatusOK, convertIntervention(req))
}

func (h *Handler) respondToIntervention(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()

	var req RespondRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	if req.Operator == "" {
		writeError(w, http.StatusBadRequest, "operator is required")
		return
	}

	result, err := h.interventionManager.Respond(ctx, id, req.Operator, req.Values, req.Confirmed, req.Comment)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, convertIntervention(result))
}

func (h *Handler) cancelIntervention(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()

	var req struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	result, err := h.interventionManager.Cancel(ctx, id, req.Reason)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, convertIntervention(result))
}

// ============================================================================
// Helper Functions
// ============================================================================

func convertApproval(req *approval.Request) ApprovalResponse {
	resp := ApprovalResponse{
		ID:            req.ID,
		ExecutionID:   req.ExecutionID,
		StepName:      req.StepName,
		State:         string(req.State),
		Title:         req.Title,
		Description:   req.Description,
		Approvers:     req.Approvers,
		Mode:          string(req.Mode),
		RequiredCount: req.RequiredCount,
		Metadata:      req.Metadata,
		CreatedAt:     req.CreatedAt,
		UpdatedAt:     req.UpdatedAt,
		ExpiresAt:     req.ExpiresAt,
		CompletedAt:   req.CompletedAt,
	}

	for _, r := range req.Responses {
		resp.Responses = append(resp.Responses, ApprovalDecision{
			Approver:    r.Approver,
			Decision:    string(r.Decision),
			Comment:     r.Comment,
			RespondedAt: r.RespondedAt,
		})
	}

	return resp
}

func convertIntervention(req *intervention.Request) InterventionResponse {
	resp := InterventionResponse{
		ID:          req.ID,
		ExecutionID: req.ExecutionID,
		StepName:    req.StepName,
		Type:        string(req.Type),
		State:       string(req.State),
		Title:       req.Title,
		Description: req.Description,
		Metadata:    req.Metadata,
		CreatedAt:   req.CreatedAt,
		UpdatedAt:   req.UpdatedAt,
		CompletedAt: req.CompletedAt,
	}

	for _, p := range req.Prompts {
		pf := PromptFieldResponse{
			Name:        p.Name,
			Label:       p.Label,
			Type:        string(p.Type),
			Required:    p.Required,
			Default:     p.Default,
			Description: p.Description,
		}

		for _, o := range p.Options {
			pf.Options = append(pf.Options, OptionResponse{
				Value: o.Value,
				Label: o.Label,
			})
		}

		if p.Validation != nil {
			pf.Validation = &ValidationResponse{
				Pattern: p.Validation.Pattern,
				Min:     p.Validation.Min,
				Max:     p.Validation.Max,
			}
		}

		resp.Prompts = append(resp.Prompts, pf)
	}

	if req.Response != nil {
		resp.Response = &InterventionResp{
			Operator:    req.Response.Operator,
			Confirmed:   req.Response.Confirmed,
			Values:      req.Response.Values,
			Comment:     req.Response.Comment,
			RespondedAt: req.Response.RespondedAt,
		}
	}

	return resp
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
