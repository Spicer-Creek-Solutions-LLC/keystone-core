package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/shawnbutts/keystone-core/internal/runbook"
	"github.com/shawnbutts/keystone-core/internal/runbook/approval"
)

// ApprovalManager defines the interface for approval operations.
type ApprovalManager interface {
	// CreateRequest creates a new approval request.
	CreateRequest(ctx context.Context, config *approval.Config, executionID, stepName string, metadata map[string]interface{}) (*approval.Request, error)

	// WaitForApproval waits for an approval request to complete.
	WaitForApproval(ctx context.Context, requestID string) (*approval.Request, error)

	// GetRequestByExecution retrieves an approval request by execution ID and step name.
	GetRequestByExecution(ctx context.Context, executionID, stepName string) (*approval.Request, error)

	// Cancel cancels a pending approval request.
	Cancel(ctx context.Context, requestID string, reason string) (*approval.Request, error)
}

// ApprovalHandler handles approval step execution.
// Configuration:
//   - title: Brief description of what's being approved (required)
//   - description: Detailed context for the approval decision (optional)
//   - approvers: List of users or groups who can approve (required)
//   - mode: How multiple approvers are handled (any/all/count, default: any)
//   - requiredCount: Number of approvals needed for mode=count
//   - timeout: Duration to wait for approval (e.g., "1h", "24h")
//   - notifyChannels: Notification channels for approval requests
//   - reminderInterval: How often to send reminders
//   - escalateAfter: When to escalate if no response
//   - escalateTo: Users or groups to escalate to
type ApprovalHandler struct {
	manager ApprovalManager
}

// NewApprovalHandler creates a new approval step handler.
func NewApprovalHandler(manager ApprovalManager) *ApprovalHandler {
	return &ApprovalHandler{
		manager: manager,
	}
}

// Type returns the step type.
func (h *ApprovalHandler) Type() runbook.StepType {
	return runbook.StepTypeApproval
}

// Validate validates the step configuration.
func (h *ApprovalHandler) Validate(step *runbook.Step) error {
	if _, ok := step.Config["title"]; !ok {
		return fmt.Errorf("approval step requires 'title' configuration")
	}

	approvers, ok := step.Config["approvers"]
	if !ok {
		return fmt.Errorf("approval step requires 'approvers' configuration")
	}

	// Validate approvers is a non-empty list
	switch v := approvers.(type) {
	case []interface{}:
		if len(v) == 0 {
			return fmt.Errorf("approvers list cannot be empty")
		}
		for i, a := range v {
			if _, ok := a.(string); !ok {
				return fmt.Errorf("approvers[%d] must be a string", i)
			}
		}
	case []string:
		if len(v) == 0 {
			return fmt.Errorf("approvers list cannot be empty")
		}
	default:
		return fmt.Errorf("approvers must be a list of strings")
	}

	// Validate mode if specified
	if mode, ok := step.Config["mode"].(string); ok {
		m := approval.Mode(mode)
		if !m.IsValid() {
			return fmt.Errorf("invalid approval mode: %s (valid: any, all, count)", mode)
		}

		// Require requiredCount for count mode
		if m == approval.ModeCount {
			if _, ok := step.Config["requiredCount"]; !ok {
				return fmt.Errorf("requiredCount is required when mode is 'count'")
			}
		}
	}

	// Validate timeout format if specified
	if timeout, ok := step.Config["timeout"].(string); ok {
		if _, err := time.ParseDuration(timeout); err != nil {
			return fmt.Errorf("invalid timeout format: %w", err)
		}
	}

	return nil
}

// Execute executes the approval step.
func (h *ApprovalHandler) Execute(ctx context.Context, step *runbook.Step, varCtx VariableContext) (*runbook.StepResult, error) {
	startTime := time.Now()

	// Check if manager is available
	if h.manager == nil {
		return &runbook.StepResult{
			Success:  false,
			Message:  "approval manager not configured",
			Duration: time.Since(startTime),
		}, fmt.Errorf("approval manager not configured")
	}

	// Build approval config from step configuration
	config, err := h.buildConfig(step.Config, varCtx)
	if err != nil {
		return &runbook.StepResult{
			Success:  false,
			Message:  fmt.Sprintf("failed to build approval config: %v", err),
			Duration: time.Since(startTime),
		}, err
	}

	// Build metadata for the request
	metadata := make(map[string]interface{})
	metadata["runbook_name"] = varCtx.RunbookName()
	metadata["execution_id"] = varCtx.ExecutionID()
	metadata["step_name"] = step.Name

	// Check for existing request (resume case)
	existingReq, err := h.manager.GetRequestByExecution(ctx, varCtx.ExecutionID(), step.Name)
	if err != nil {
		return &runbook.StepResult{
			Success:  false,
			Message:  fmt.Sprintf("failed to check for existing request: %v", err),
			Duration: time.Since(startTime),
		}, err
	}

	var req *approval.Request
	if existingReq != nil {
		// Use existing request
		req = existingReq
	} else {
		// Create new approval request
		req, err = h.manager.CreateRequest(ctx, config, varCtx.ExecutionID(), step.Name, metadata)
		if err != nil {
			return &runbook.StepResult{
				Success:  false,
				Message:  fmt.Sprintf("failed to create approval request: %v", err),
				Duration: time.Since(startTime),
				Outputs: map[string]interface{}{
					"error": err.Error(),
				},
			}, err
		}
	}

	// If already terminal, return immediately
	if req.State.IsTerminal() {
		return h.buildResult(req, startTime), nil
	}

	// Store request ID before waiting (WaitForApproval may return nil on error)
	requestID := req.ID

	// Wait for approval
	req, err = h.manager.WaitForApproval(ctx, requestID)
	if err != nil {
		// Check if context was cancelled
		if ctx.Err() != nil {
			// Try to cancel the pending request using a background context
			// since the original context is already cancelled
			cancelCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second) //nolint:contextcheck // intentional: original ctx is cancelled, need new context for cleanup
			defer cancel()
			_, cancelErr := h.manager.Cancel(cancelCtx, requestID, "execution cancelled") //nolint:contextcheck // intentional: using cleanup context
			if cancelErr != nil {
				_ = cancelErr
			}
		}

		// Build outputs even when we only have the request ID
		outputs := map[string]interface{}{
			"request_id": requestID,
			"error":      err.Error(),
		}
		if req != nil {
			outputs["state"] = string(req.State)
		}

		return &runbook.StepResult{
			Success:  false,
			Message:  fmt.Sprintf("error waiting for approval: %v", err),
			Duration: time.Since(startTime),
			Outputs:  outputs,
		}, err
	}

	return h.buildResult(req, startTime), nil
}

// buildConfig builds an approval config from step configuration.
func (h *ApprovalHandler) buildConfig(config map[string]interface{}, varCtx VariableContext) (*approval.Config, error) {
	ac := &approval.Config{}

	// Title (required)
	if title, ok := config["title"].(string); ok {
		resolved, err := varCtx.Resolve(title)
		if err != nil {
			return nil, fmt.Errorf("resolve title: %w", err)
		}
		ac.Title = resolved
	}

	// Description (optional)
	if desc, ok := config["description"].(string); ok {
		resolved, err := varCtx.Resolve(desc)
		if err != nil {
			return nil, fmt.Errorf("resolve description: %w", err)
		}
		ac.Description = resolved
	}

	// Approvers (required)
	switch v := config["approvers"].(type) {
	case []interface{}:
		for _, a := range v {
			if s, ok := a.(string); ok {
				resolved, err := varCtx.Resolve(s)
				if err != nil {
					return nil, fmt.Errorf("resolve approver: %w", err)
				}
				ac.Approvers = append(ac.Approvers, resolved)
			}
		}
	case []string:
		for _, s := range v {
			resolved, err := varCtx.Resolve(s)
			if err != nil {
				return nil, fmt.Errorf("resolve approver: %w", err)
			}
			ac.Approvers = append(ac.Approvers, resolved)
		}
	}

	// Mode (optional, default: any)
	if mode, ok := config["mode"].(string); ok {
		ac.Mode = approval.Mode(mode)
	}

	// RequiredCount (optional)
	switch count := config["requiredCount"].(type) {
	case int:
		ac.RequiredCount = count
	case float64:
		ac.RequiredCount = int(count)
	}

	// Timeout (optional)
	if timeout, ok := config["timeout"].(string); ok {
		ac.Timeout = timeout
	}

	// NotifyChannels (optional)
	if channels, ok := config["notifyChannels"].([]interface{}); ok {
		for _, c := range channels {
			if s, ok := c.(string); ok {
				ac.NotifyChannels = append(ac.NotifyChannels, s)
			}
		}
	}

	// ReminderInterval (optional)
	if interval, ok := config["reminderInterval"].(string); ok {
		ac.ReminderInterval = interval
	}

	// EscalateAfter (optional)
	if after, ok := config["escalateAfter"].(string); ok {
		ac.EscalateAfter = after
	}

	// EscalateTo (optional)
	if escalateTo, ok := config["escalateTo"].([]interface{}); ok {
		for _, e := range escalateTo {
			if s, ok := e.(string); ok {
				ac.EscalateTo = append(ac.EscalateTo, s)
			}
		}
	}

	return ac, nil
}

// buildResult builds a step result from an approval request.
func (h *ApprovalHandler) buildResult(req *approval.Request, startTime time.Time) *runbook.StepResult {
	outputs := map[string]interface{}{
		"request_id":      req.ID,
		"state":           string(req.State),
		"approval_count":  req.ApprovalCount(),
		"rejection_count": req.RejectionCount(),
		"total_responses": len(req.Responses),
	}

	// Include response details
	if len(req.Responses) > 0 {
		responses := make([]map[string]interface{}, len(req.Responses))
		for i, r := range req.Responses {
			responses[i] = map[string]interface{}{
				"approver":     r.Approver,
				"decision":     string(r.Decision),
				"comment":      r.Comment,
				"responded_at": r.RespondedAt.Format(time.RFC3339),
			}
		}
		outputs["responses"] = responses

		// Include the last decision details for convenience
		lastResp := req.Responses[len(req.Responses)-1]
		outputs["last_approver"] = lastResp.Approver
		outputs["last_decision"] = string(lastResp.Decision)
		outputs["last_comment"] = lastResp.Comment
	}

	var message string
	var success bool

	switch req.State {
	case approval.RequestStateApproved:
		success = true
		message = fmt.Sprintf("approved by %d approver(s)", req.ApprovalCount())
	case approval.RequestStateRejected:
		success = false
		message = fmt.Sprintf("rejected by %d approver(s)", req.RejectionCount())
	case approval.RequestStateExpired:
		success = false
		message = "approval request expired"
	case approval.RequestStateCancelled:
		success = false
		message = "approval request cancelled"
	default:
		success = false
		message = fmt.Sprintf("unexpected state: %s", req.State)
	}

	return &runbook.StepResult{
		Success:  success,
		Message:  message,
		Duration: time.Since(startTime),
		Outputs:  outputs,
	}
}
