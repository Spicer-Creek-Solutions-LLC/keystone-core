// Package runbook provides HTTP handlers for runbook REST API endpoints.
package runbook

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	internalrunbook "github.com/shawnbutts/keystone-core/internal/runbook"
	rbaudit "github.com/shawnbutts/keystone-core/internal/runbook/audit"
	rbstorage "github.com/shawnbutts/keystone-core/internal/runbook/storage"
	"github.com/shawnbutts/keystone-core/internal/runbook/approval"
	"github.com/shawnbutts/keystone-core/internal/runbook/intervention"
	"github.com/shawnbutts/keystone-core/pkg/api/apierror"
)

// Repository provides access to runbook definitions.
type Repository interface {
	GetRunbook(name, version string) (*internalrunbook.Runbook, error)
	ListRunbooks() ([]*internalrunbook.Runbook, error)
}

// ExecutionStorage provides access to runbook execution records.
type ExecutionStorage interface {
	GetExecution(ctx context.Context, id string) (*internalrunbook.Execution, error)
	ListExecutions(ctx context.Context, opts rbstorage.ListOptions) ([]*internalrunbook.Execution, error)
}

// AuditQuerier provides access to audit events.
type AuditQuerier interface {
	Query(ctx context.Context, query *rbaudit.Query) ([]*rbaudit.Event, error)
	Count(ctx context.Context, query *rbaudit.Query) (int64, error)
}

// Executor executes runbooks.
type Executor interface {
	Execute(ctx context.Context, rb *internalrunbook.Runbook, inputs map[string]interface{}) (*internalrunbook.Execution, error)
	ExecuteAsync(ctx context.Context, rb *internalrunbook.Runbook, inputs map[string]interface{}) (string, error)
}

// Handler provides HTTP handlers for runbook API endpoints.
type Handler struct {
	approvalStorage     approval.Storage
	interventionStorage intervention.Storage
	approvalManager     *approval.Manager
	interventionManager *intervention.Manager

	runbookRepo  Repository
	execStorage  ExecutionStorage
	auditQuerier AuditQuerier
	executor     Executor
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

// SetRunbookDeps sets optional dependencies for runbook CRUD, execution, and audit endpoints.
func (h *Handler) SetRunbookDeps(repo Repository, execStorage ExecutionStorage, auditQuerier AuditQuerier, executor Executor) {
	h.runbookRepo = repo
	h.execStorage = execStorage
	h.auditQuerier = auditQuerier
	h.executor = executor
}

// RegisterRoutes registers the runbook API routes with the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Approval endpoints
	mux.HandleFunc("/api/v1/runbook/approvals", h.handleApprovals)
	mux.HandleFunc("/api/v1/runbook/approvals/", h.handleApproval)

	// Intervention endpoints
	mux.HandleFunc("/api/v1/runbook/interventions", h.handleInterventions)
	mux.HandleFunc("/api/v1/runbook/interventions/", h.handleIntervention)

	// Runbook CRUD/execution endpoints
	mux.HandleFunc("/api/v1/runbooks/executions/", h.handleExecution)
	mux.HandleFunc("/api/v1/runbooks/executions", h.handleExecutions)
	mux.HandleFunc("/api/v1/runbooks/audit", h.handleAudit)
	mux.HandleFunc("/api/v1/runbooks/", h.handleRunbookAction)
	mux.HandleFunc("/api/v1/runbooks", h.handleRunbooks)
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
		opts.State = intervention.State(state)
	}
	if execID := query.Get("execution_id"); execID != "" {
		opts.ExecutionID = execID
	}
	if intType := query.Get("type"); intType != "" {
		opts.Type = intervention.Type(intType)
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
// Runbook Response Types
// ============================================================================

// Summary represents a runbook in API responses.
type Summary struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace,omitempty"`
	Version     string            `json:"version,omitempty"`
	Description string            `json:"description,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	StepCount   int               `json:"step_count"`
	Inputs      int               `json:"inputs"`
	Timeout     string            `json:"timeout,omitempty"`
}

// SummaryList represents the list runbooks API response.
type SummaryList struct {
	Runbooks []Summary `json:"runbooks"`
	Total    int               `json:"total"`
}

// ExecutionResponse represents a runbook execution in API responses.
type ExecutionResponse struct {
	ID             string                 `json:"id"`
	RunbookName    string                 `json:"runbook_name"`
	RunbookVersion string                 `json:"runbook_version,omitempty"`
	State          string                 `json:"state"`
	Inputs         map[string]interface{} `json:"inputs,omitempty"`
	Outputs        map[string]interface{} `json:"outputs,omitempty"`
	StartedAt      *time.Time             `json:"started_at,omitempty"`
	CompletedAt    *time.Time             `json:"completed_at,omitempty"`
	Error          string                 `json:"error,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
}

// ExecutionListResponse represents the list executions API response.
type ExecutionListResponse struct {
	Executions []ExecutionResponse `json:"executions"`
	Total      int                 `json:"total"`
}

// ExecuteRequest represents the request body for executing a runbook.
type ExecuteRequest struct {
	Version string                 `json:"version,omitempty"`
	Inputs  map[string]interface{} `json:"inputs,omitempty"`
	Async   bool                   `json:"async,omitempty"`
}

// ExecuteResponse represents the response from executing a runbook.
type ExecuteResponse struct {
	ExecutionID string             `json:"execution_id"`
	State       string             `json:"state"`
	Execution   *ExecutionResponse `json:"execution,omitempty"`
}

// AuditEventResponse represents an audit event in API responses.
type AuditEventResponse struct {
	ID             string                 `json:"id"`
	Timestamp      time.Time              `json:"timestamp"`
	Type           string                 `json:"type"`
	ExecutionID    string                 `json:"execution_id"`
	RunbookName    string                 `json:"runbook_name"`
	RunbookVersion string                 `json:"runbook_version,omitempty"`
	StepName       string                 `json:"step_name,omitempty"`
	Actor          string                 `json:"actor,omitempty"`
	Details        map[string]interface{} `json:"details,omitempty"`
	Outcome        string                 `json:"outcome,omitempty"`
	Duration       string                 `json:"duration,omitempty"`
	Error          string                 `json:"error,omitempty"`
}

// AuditListResponse represents the list audit events API response.
type AuditListResponse struct {
	Events []AuditEventResponse `json:"events"`
	Total  int64                `json:"total"`
}

// ============================================================================
// Runbook Handlers
// ============================================================================

func (h *Handler) handleRunbooks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.listRunbooks(w, r)
}

func (h *Handler) handleRunbookAction(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/runbooks/")
	if path == "" {
		writeError(w, http.StatusBadRequest, "Runbook name required")
		return
	}

	parts := strings.SplitN(path, "/", 2)
	name := parts[0]

	if len(parts) == 2 && parts[1] == "execute" {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.executeRunbook(w, r, name)
		return
	}

	writeError(w, http.StatusNotFound, "Not found")
}

func (h *Handler) handleExecutions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.listExecutions(w, r)
}

func (h *Handler) handleExecution(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/v1/runbooks/executions/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Execution ID required")
		return
	}
	h.getExecution(w, r, id)
}

func (h *Handler) handleAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.listAuditEvents(w, r)
}

func (h *Handler) listRunbooks(w http.ResponseWriter, _ *http.Request) {
	if h.runbookRepo == nil {
		writeError(w, http.StatusServiceUnavailable, "Runbook repository not available")
		return
	}

	runbooks, err := h.runbookRepo.ListRunbooks()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list runbooks: "+err.Error())
		return
	}

	responses := make([]Summary, 0, len(runbooks))
	for _, rb := range runbooks {
		responses = append(responses, convertRunbook(rb))
	}

	writeJSON(w, http.StatusOK, SummaryList{
		Runbooks: responses,
		Total:    len(responses),
	})
}

func (h *Handler) executeRunbook(w http.ResponseWriter, r *http.Request, name string) {
	if h.runbookRepo == nil || h.executor == nil {
		writeError(w, http.StatusServiceUnavailable, "Runbook execution not available")
		return
	}

	var req ExecuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	rb, err := h.runbookRepo.GetRunbook(name, req.Version)
	if err != nil {
		writeError(w, http.StatusNotFound, "Runbook not found: "+err.Error())
		return
	}

	ctx := r.Context()

	if req.Async {
		execID, asyncErr := h.executor.ExecuteAsync(ctx, rb, req.Inputs)
		if asyncErr != nil {
			writeError(w, http.StatusInternalServerError, "Failed to start execution: "+asyncErr.Error())
			return
		}
		writeJSON(w, http.StatusAccepted, ExecuteResponse{
			ExecutionID: execID,
			State:       string(internalrunbook.ExecutionStatePending),
		})
		return
	}

	exec, execErr := h.executor.Execute(ctx, rb, req.Inputs)
	if execErr != nil {
		writeError(w, http.StatusInternalServerError, "Execution failed: "+execErr.Error())
		return
	}

	execResp := convertExecution(exec)
	writeJSON(w, http.StatusOK, ExecuteResponse{
		ExecutionID: exec.ID,
		State:       string(exec.State),
		Execution:   &execResp,
	})
}

func (h *Handler) listExecutions(w http.ResponseWriter, r *http.Request) {
	if h.execStorage == nil {
		writeError(w, http.StatusServiceUnavailable, "Execution storage not available")
		return
	}

	query := r.URL.Query()
	opts := rbstorage.ListOptions{}

	if name := query.Get("runbook"); name != "" {
		opts.RunbookName = name
	}
	if state := query.Get("state"); state != "" {
		opts.State = internalrunbook.ExecutionState(state)
	}
	if limitStr := query.Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			opts.Limit = l
		}
	}
	if opts.Limit == 0 {
		opts.Limit = 50
	}
	if offsetStr := query.Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			opts.Offset = o
		}
	}
	if sinceStr := query.Get("since"); sinceStr != "" {
		if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			opts.Since = &t
		}
	}

	ctx := r.Context()
	executions, err := h.execStorage.ListExecutions(ctx, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list executions: "+err.Error())
		return
	}

	responses := make([]ExecutionResponse, 0, len(executions))
	for _, exec := range executions {
		responses = append(responses, convertExecution(exec))
	}

	writeJSON(w, http.StatusOK, ExecutionListResponse{
		Executions: responses,
		Total:      len(responses),
	})
}

func (h *Handler) getExecution(w http.ResponseWriter, r *http.Request, id string) {
	if h.execStorage == nil {
		writeError(w, http.StatusServiceUnavailable, "Execution storage not available")
		return
	}

	ctx := r.Context()
	exec, err := h.execStorage.GetExecution(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get execution: "+err.Error())
		return
	}
	if exec == nil {
		writeError(w, http.StatusNotFound, "Execution not found")
		return
	}

	writeJSON(w, http.StatusOK, convertExecution(exec))
}

func (h *Handler) listAuditEvents(w http.ResponseWriter, r *http.Request) {
	if h.auditQuerier == nil {
		writeError(w, http.StatusServiceUnavailable, "Audit storage not available")
		return
	}

	queryParams := r.URL.Query()
	q := &rbaudit.Query{}

	if execID := queryParams.Get("execution_id"); execID != "" {
		q.ExecutionID = execID
	}
	if rbName := queryParams.Get("runbook"); rbName != "" {
		q.RunbookName = rbName
	}
	if actor := queryParams.Get("actor"); actor != "" {
		q.Actor = actor
	}
	if outcome := queryParams.Get("outcome"); outcome != "" {
		q.Outcome = outcome
	}
	if limitStr := queryParams.Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			q.Limit = l
		}
	}
	if q.Limit == 0 {
		q.Limit = 50
	}
	if offsetStr := queryParams.Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			q.Offset = o
		}
	}
	if startStr := queryParams.Get("start"); startStr != "" {
		if t, err := time.Parse(time.RFC3339, startStr); err == nil {
			q.StartTime = &t
		}
	}
	if endStr := queryParams.Get("end"); endStr != "" {
		if t, err := time.Parse(time.RFC3339, endStr); err == nil {
			q.EndTime = &t
		}
	}

	ctx := r.Context()
	events, err := h.auditQuerier.Query(ctx, q)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to query audit events: "+err.Error())
		return
	}

	total, _ := h.auditQuerier.Count(ctx, q)

	responses := make([]AuditEventResponse, 0, len(events))
	for _, ev := range events {
		responses = append(responses, convertAuditEvent(ev))
	}

	writeJSON(w, http.StatusOK, AuditListResponse{
		Events: responses,
		Total:  total,
	})
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
	apierror.Write(w, status, message)
}

func convertRunbook(rb *internalrunbook.Runbook) Summary {
	return Summary{
		Name:        rb.Metadata.Name,
		Namespace:   rb.Metadata.Namespace,
		Version:     rb.Metadata.Version,
		Description: rb.Metadata.Description,
		Labels:      rb.Metadata.Labels,
		StepCount:   len(rb.Spec.Steps),
		Inputs:      len(rb.Spec.Inputs),
		Timeout:     rb.Spec.Timeout,
	}
}

func convertExecution(exec *internalrunbook.Execution) ExecutionResponse {
	return ExecutionResponse{
		ID:             exec.ID,
		RunbookName:    exec.RunbookName,
		RunbookVersion: exec.RunbookVersion,
		State:          string(exec.State),
		Inputs:         exec.Inputs,
		Outputs:        exec.Outputs,
		StartedAt:      exec.StartedAt,
		CompletedAt:    exec.CompletedAt,
		Error:          exec.Error,
		CreatedAt:      exec.CreatedAt,
	}
}

func convertAuditEvent(ev *rbaudit.Event) AuditEventResponse {
	resp := AuditEventResponse{
		ID:             ev.ID,
		Timestamp:      ev.Timestamp,
		Type:           string(ev.Type),
		ExecutionID:    ev.ExecutionID,
		RunbookName:    ev.RunbookName,
		RunbookVersion: ev.RunbookVersion,
		StepName:       ev.StepName,
		Actor:          ev.Actor,
		Details:        ev.Details,
		Outcome:        ev.Outcome,
		Error:          ev.Error,
	}
	if ev.Duration > 0 {
		resp.Duration = ev.Duration.String()
	}
	return resp
}
