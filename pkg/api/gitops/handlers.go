// Package gitops provides HTTP handlers for GitOps REST API endpoints.
package gitops

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/gitops/rollback"
	"github.com/shawnbutts/keystone-core/internal/gitops/verification"
)

// Handler provides HTTP handlers for GitOps API endpoints.
type Handler struct {
	verificationEngine *verification.Engine
	rollbackEngine     *rollback.Engine

	// Store verification results for listing
	verificationResults map[string]*verification.WorkflowResult
}

// NewHandler creates a new GitOps API handler.
func NewHandler(verificationEngine *verification.Engine, rollbackEngine *rollback.Engine) *Handler {
	return &Handler{
		verificationEngine:  verificationEngine,
		rollbackEngine:      rollbackEngine,
		verificationResults: make(map[string]*verification.WorkflowResult),
	}
}

// RegisterRoutes registers the GitOps API routes with the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/gitops/verifications", h.handleVerifications)
	mux.HandleFunc("/api/v1/gitops/verifications/", h.handleVerification)
	mux.HandleFunc("/api/v1/gitops/rollback", h.handleRollback)
	mux.HandleFunc("/api/v1/gitops/rollbacks", h.handleRollbacks)
	mux.HandleFunc("/api/v1/gitops/rollbacks/", h.handleRollbackByID)
}

// VerificationResponse represents a verification result in API responses.
type VerificationResponse struct {
	ID           string                              `json:"id"`
	WorkflowName string                              `json:"workflow_name"`
	Success      bool                                `json:"success"`
	Steps        []VerificationStepResponse          `json:"steps"`
	TotalSteps   int                                 `json:"total_steps"`
	PassedSteps  int                                 `json:"passed_steps"`
	FailedSteps  int                                 `json:"failed_steps"`
	SkippedSteps int                                 `json:"skipped_steps"`
	Duration     string                              `json:"duration"`
	StartTime    time.Time                           `json:"start_time"`
	EndTime      time.Time                           `json:"end_time"`
}

// VerificationStepResponse represents a single step result.
type VerificationStepResponse struct {
	StepName  string                 `json:"step_name"`
	Success   bool                   `json:"success"`
	Message   string                 `json:"message,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Duration  string                 `json:"duration"`
	Timestamp time.Time              `json:"timestamp"`
	Error     string                 `json:"error,omitempty"`
	Retries   int                    `json:"retries,omitempty"`
}

// VerificationListResponse represents the list verifications API response.
type VerificationListResponse struct {
	Verifications []VerificationResponse `json:"verifications"`
	Total         int                    `json:"total"`
	Limit         int                    `json:"limit"`
	Offset        int                    `json:"offset"`
	RetrievedAt   time.Time              `json:"retrieved_at"`
}

// RollbackRequest represents the request body for POST /api/v1/gitops/rollback.
type RollbackRequest struct {
	// Application name
	Application string `json:"application"`

	// Namespace (optional)
	Namespace string `json:"namespace,omitempty"`

	// Type of rollback (argocd, flux, git, manual)
	Type string `json:"type"`

	// Strategy (previous, specific, last_known_good)
	Strategy string `json:"strategy,omitempty"`

	// Revision to rollback to (for specific strategy)
	Revision string `json:"revision,omitempty"`

	// Reason for the rollback
	Reason string `json:"reason"`

	// RequestedBy is the user requesting the rollback
	RequestedBy string `json:"requested_by,omitempty"`

	// SkipVerification skips post-rollback verification
	SkipVerification bool `json:"skip_verification,omitempty"`

	// RequireApproval forces approval requirement
	RequireApproval bool `json:"require_approval,omitempty"`
}

// RollbackResponse represents a rollback result in API responses.
type RollbackResponse struct {
	ID               string                `json:"id"`
	Application      string                `json:"application"`
	Namespace        string                `json:"namespace,omitempty"`
	Type             string                `json:"type"`
	Strategy         string                `json:"strategy"`
	Status           string                `json:"status"`
	PreviousRevision string                `json:"previous_revision,omitempty"`
	CurrentRevision  string                `json:"current_revision,omitempty"`
	Message          string                `json:"message,omitempty"`
	Error            string                `json:"error,omitempty"`
	Duration         string                `json:"duration,omitempty"`
	StartTime        time.Time             `json:"start_time"`
	EndTime          time.Time             `json:"end_time,omitempty"`
	ApprovalInfo     *ApprovalInfoResponse `json:"approval_info,omitempty"`
}

// ApprovalInfoResponse represents approval information in API responses.
type ApprovalInfoResponse struct {
	Required   bool      `json:"required"`
	Status     string    `json:"status"`
	ApprovedBy string    `json:"approved_by,omitempty"`
	ApprovedAt time.Time `json:"approved_at,omitempty"`
	RejectedBy string    `json:"rejected_by,omitempty"`
	RejectedAt time.Time `json:"rejected_at,omitempty"`
	Reason     string    `json:"reason,omitempty"`
}

// RollbackListResponse represents the list rollbacks API response.
type RollbackListResponse struct {
	Rollbacks   []RollbackResponse `json:"rollbacks"`
	Total       int                `json:"total"`
	Limit       int                `json:"limit"`
	Offset      int                `json:"offset"`
	RetrievedAt time.Time          `json:"retrieved_at"`
}

// ApprovalRequest represents a request to approve/reject a rollback.
type ApprovalRequest struct {
	Approved   bool   `json:"approved"`
	ApprovedBy string `json:"approved_by"`
	Reason     string `json:"reason,omitempty"`
}

// handleVerifications handles GET /api/v1/gitops/verifications
func (h *Handler) handleVerifications(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	query := r.URL.Query()

	// Parse pagination
	limit := 50 // default
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

	// Filter by success
	var filterSuccess *bool
	if successStr := query.Get("success"); successStr != "" {
		success := successStr == "true"
		filterSuccess = &success
	}

	// Filter by workflow name
	workflowFilter := query.Get("workflow")

	// Build response from stored results
	allResults := make([]VerificationResponse, 0)
	for id, result := range h.verificationResults {
		// Apply filters
		if filterSuccess != nil && result.Success != *filterSuccess {
			continue
		}
		if workflowFilter != "" && result.WorkflowName != workflowFilter {
			continue
		}

		allResults = append(allResults, convertWorkflowResult(id, result))
	}

	// Apply pagination
	total := len(allResults)
	start := offset
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	pagedResults := allResults[start:end]

	resp := VerificationListResponse{
		Verifications: pagedResults,
		Total:         total,
		Limit:         limit,
		Offset:        offset,
		RetrievedAt:   time.Now().UTC(),
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleVerification handles GET /api/v1/gitops/verifications/{id}
func (h *Handler) handleVerification(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract verification ID from path
	verificationID := strings.TrimPrefix(r.URL.Path, "/api/v1/gitops/verifications/")
	if verificationID == "" {
		writeError(w, http.StatusBadRequest, "Verification ID required")
		return
	}

	// Get verification result
	result, ok := h.verificationResults[verificationID]
	if !ok {
		writeError(w, http.StatusNotFound, "Verification not found")
		return
	}

	resp := convertWorkflowResult(verificationID, result)
	writeJSON(w, http.StatusOK, resp)
}

// handleRollback handles POST /api/v1/gitops/rollback
func (h *Handler) handleRollback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RollbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	// Validate request
	if req.Application == "" {
		writeError(w, http.StatusBadRequest, "application is required")
		return
	}
	if req.Type == "" {
		writeError(w, http.StatusBadRequest, "type is required")
		return
	}
	if req.Reason == "" {
		writeError(w, http.StatusBadRequest, "reason is required")
		return
	}

	// Convert request type to RollbackType
	rollbackType := rollback.RollbackType(req.Type)

	// Default strategy
	strategy := rollback.StrategyPreviousRevision
	if req.Strategy != "" {
		strategy = rollback.RollbackStrategy(req.Strategy)
	}

	// Build rollback config
	config := &rollback.RollbackConfig{
		Name:            req.Application + "-rollback",
		Type:            rollbackType,
		Strategy:        strategy,
		Trigger:         rollback.TriggerManual,
		Application:     req.Application,
		Namespace:       req.Namespace,
		Revision:        req.Revision,
		RequireApproval: req.RequireApproval,
	}

	// Build rollback request
	rollbackReq := &rollback.RollbackRequest{
		ConfigName:       config.Name,
		Reason:           req.Reason,
		RequestedBy:      req.RequestedBy,
		OverrideRevision: req.Revision,
		SkipVerification: req.SkipVerification,
	}

	// Execute rollback
	ctx := r.Context()
	result, err := h.rollbackEngine.Execute(ctx, config, rollbackReq)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to execute rollback: "+err.Error())
		return
	}

	resp := convertRollbackResult(result)
	writeJSON(w, http.StatusAccepted, resp)
}

// handleRollbacks handles GET /api/v1/gitops/rollbacks
func (h *Handler) handleRollbacks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	query := r.URL.Query()

	// Parse pagination
	limit := 50 // default
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

	// Filter by status
	statusFilter := query.Get("status")

	// Filter by application
	appFilter := query.Get("application")

	// Get all rollbacks
	allRollbacks := h.rollbackEngine.ListRollbacks()

	// Apply filters and convert
	responses := make([]RollbackResponse, 0)
	for _, rb := range allRollbacks {
		// Apply filters
		if statusFilter != "" && string(rb.Status) != statusFilter {
			continue
		}
		if appFilter != "" && rb.Config.Application != appFilter {
			continue
		}

		responses = append(responses, convertRollbackResult(rb))
	}

	// Apply pagination
	total := len(responses)
	start := offset
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	pagedResults := responses[start:end]

	resp := RollbackListResponse{
		Rollbacks:   pagedResults,
		Total:       total,
		Limit:       limit,
		Offset:      offset,
		RetrievedAt: time.Now().UTC(),
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleRollbackByID handles GET/POST /api/v1/gitops/rollbacks/{id}
func (h *Handler) handleRollbackByID(w http.ResponseWriter, r *http.Request) {
	// Extract rollback ID from path
	rollbackID := strings.TrimPrefix(r.URL.Path, "/api/v1/gitops/rollbacks/")

	// Check for approve/reject suffix
	if strings.HasSuffix(rollbackID, "/approve") {
		rollbackID = strings.TrimSuffix(rollbackID, "/approve")
		h.handleRollbackApproval(w, r, rollbackID)
		return
	}

	if rollbackID == "" {
		writeError(w, http.StatusBadRequest, "Rollback ID required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Get rollback by ID
		result, ok := h.rollbackEngine.GetRollback(rollbackID)
		if !ok {
			writeError(w, http.StatusNotFound, "Rollback not found")
			return
		}

		resp := convertRollbackResult(result)
		writeJSON(w, http.StatusOK, resp)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleRollbackApproval handles POST /api/v1/gitops/rollbacks/{id}/approve
func (h *Handler) handleRollbackApproval(w http.ResponseWriter, r *http.Request, rollbackID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ApprovalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	// Validate request
	if req.ApprovedBy == "" {
		writeError(w, http.StatusBadRequest, "approved_by is required")
		return
	}

	// Build approval request
	approvalReq := &rollback.ApprovalRequest{
		RollbackID: rollbackID,
		Approved:   req.Approved,
		ApprovedBy: req.ApprovedBy,
		Reason:     req.Reason,
	}

	// Approve/reject rollback
	ctx := r.Context()
	if err := h.rollbackEngine.ApproveRollback(ctx, approvalReq); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to process approval: "+err.Error())
		return
	}

	// Return updated rollback
	result, ok := h.rollbackEngine.GetRollback(rollbackID)
	if !ok {
		writeError(w, http.StatusNotFound, "Rollback not found")
		return
	}

	resp := convertRollbackResult(result)
	writeJSON(w, http.StatusOK, resp)
}

// StoreVerificationResult stores a verification result for later retrieval.
func (h *Handler) StoreVerificationResult(id string, result *verification.WorkflowResult) {
	h.verificationResults[id] = result
}

// Helper functions

func convertWorkflowResult(id string, result *verification.WorkflowResult) VerificationResponse {
	steps := make([]VerificationStepResponse, 0, len(result.Steps))
	for _, step := range result.Steps {
		errStr := ""
		if step.Error != nil {
			errStr = step.Error.Error()
		}
		steps = append(steps, VerificationStepResponse{
			StepName:  step.StepName,
			Success:   step.Success,
			Message:   step.Message,
			Data:      step.Data,
			Duration:  step.Duration.String(),
			Timestamp: step.Timestamp,
			Error:     errStr,
			Retries:   step.Retries,
		})
	}

	return VerificationResponse{
		ID:           id,
		WorkflowName: result.WorkflowName,
		Success:      result.Success,
		Steps:        steps,
		TotalSteps:   result.TotalSteps,
		PassedSteps:  result.PassedSteps,
		FailedSteps:  result.FailedSteps,
		SkippedSteps: result.SkippedSteps,
		Duration:     result.Duration.String(),
		StartTime:    result.StartTime,
		EndTime:      result.EndTime,
	}
}

func convertRollbackResult(result *rollback.RollbackResult) RollbackResponse {
	resp := RollbackResponse{
		ID:               result.ID,
		Application:      result.Config.Application,
		Namespace:        result.Config.Namespace,
		Type:             string(result.Config.Type),
		Strategy:         string(result.Config.Strategy),
		Status:           string(result.Status),
		PreviousRevision: result.PreviousRevision,
		CurrentRevision:  result.CurrentRevision,
		Message:          result.Message,
		StartTime:        result.StartTime,
	}

	if !result.EndTime.IsZero() {
		resp.EndTime = result.EndTime
		resp.Duration = result.Duration.String()
	}

	if result.Error != nil {
		resp.Error = result.Error.Error()
	}

	if result.ApprovalInfo != nil {
		resp.ApprovalInfo = &ApprovalInfoResponse{
			Required:   result.ApprovalInfo.Required,
			Status:     string(result.ApprovalInfo.Status),
			ApprovedBy: result.ApprovalInfo.ApprovedBy,
			ApprovedAt: result.ApprovalInfo.ApprovedAt,
			RejectedBy: result.ApprovalInfo.RejectedBy,
			RejectedAt: result.ApprovalInfo.RejectedAt,
			Reason:     result.ApprovalInfo.Reason,
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
