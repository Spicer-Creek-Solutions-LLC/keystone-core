package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/shawnbutts/keystone-core/internal/gitops/orchestration"
	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// DeployOrchestrator defines the interface for deployment orchestration.
type DeployOrchestrator interface {
	// Execute executes an orchestration request and returns the result.
	Execute(ctx context.Context, req *orchestration.Request) (*orchestration.Result, error)

	// GetPlan retrieves an orchestration plan by name.
	GetPlan(name string) (*orchestration.Plan, bool)

	// Rollback rolls back an orchestration to a previous state.
	Rollback(ctx context.Context, orchestrationID string) (*orchestration.Result, error)
}

// DeployHandler handles deploy step execution.
// Configuration:
//   - plan: Name of the orchestration plan to execute (required)
//   - revisions: Map of repository name to revision to deploy (optional)
//   - reason: Reason for the deployment (optional)
//   - dry_run: If true, only validate without deploying (default: false)
//   - force: Bypass approval if true (default: false)
//   - groups_to_skip: List of deployment groups to skip (optional)
//   - rollback_on_failure: If true, rollback on failure (default: false)
//   - timeout: Deployment timeout duration (optional)
type DeployHandler struct {
	orchestrator DeployOrchestrator
	requestedBy  string
}

// NewDeployHandler creates a new deploy step handler.
func NewDeployHandler(orchestrator DeployOrchestrator) *DeployHandler {
	return &DeployHandler{
		orchestrator: orchestrator,
		requestedBy:  "runbook",
	}
}

// NewDeployHandlerWithIdentity creates a new deploy handler with a specific identity.
func NewDeployHandlerWithIdentity(orchestrator DeployOrchestrator, requestedBy string) *DeployHandler {
	return &DeployHandler{
		orchestrator: orchestrator,
		requestedBy:  requestedBy,
	}
}

// Type returns the step type.
func (h *DeployHandler) Type() runbook.StepType {
	return runbook.StepTypeDeploy
}

// Validate validates the step configuration.
func (h *DeployHandler) Validate(step *runbook.Step) error {
	config := step.Config

	// Plan is required
	plan, ok := config["plan"]
	if !ok {
		return fmt.Errorf("deploy step requires 'plan' configuration")
	}

	if _, ok := plan.(string); !ok {
		return fmt.Errorf("plan must be a string")
	}

	// Validate revisions if provided
	if revisions, ok := config["revisions"]; ok {
		if _, ok := revisions.(map[string]interface{}); !ok {
			return fmt.Errorf("revisions must be a map")
		}
	}

	// Validate groups_to_skip if provided
	if groups, ok := config["groups_to_skip"]; ok {
		if _, ok := groups.([]interface{}); !ok {
			return fmt.Errorf("groups_to_skip must be a list")
		}
	}

	return nil
}

// Execute executes the deploy step.
func (h *DeployHandler) Execute(ctx context.Context, step *runbook.Step, varCtx VariableContext) (*runbook.StepResult, error) {
	startTime := time.Now()

	// Check if orchestrator is available
	if h.orchestrator == nil {
		return &runbook.StepResult{
			Success:  false,
			Message:  "deploy orchestrator not configured",
			Duration: time.Since(startTime),
		}, fmt.Errorf("deploy orchestrator not configured")
	}

	// Build orchestration request from configuration
	req, err := h.buildRequest(step, varCtx)
	if err != nil {
		return &runbook.StepResult{
			Success:  false,
			Message:  fmt.Sprintf("failed to build deploy request: %v", err),
			Duration: time.Since(startTime),
		}, err
	}

	// Verify plan exists
	if _, exists := h.orchestrator.GetPlan(req.PlanName); !exists {
		return &runbook.StepResult{
			Success:  false,
			Message:  fmt.Sprintf("orchestration plan not found: %s", req.PlanName),
			Duration: time.Since(startTime),
			Outputs: map[string]interface{}{
				"plan_name": req.PlanName,
				"error":     "plan not found",
			},
		}, fmt.Errorf("orchestration plan not found: %s", req.PlanName)
	}

	// Apply timeout if configured
	timeout := h.getTimeout(step.Config)
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// Execute deployment
	result, err := h.orchestrator.Execute(ctx, req)
	if err != nil {
		return h.buildErrorResult(result, err, startTime, step.Config), err
	}

	// Check if deployment failed and rollback is requested
	if result.Status == orchestration.StatusFailed {
		if h.shouldRollback(step.Config) {
			return h.handleRollback(ctx, result, startTime)
		}
	}

	return h.buildResult(result, startTime), nil
}

// buildRequest builds a Request from step configuration.
func (h *DeployHandler) buildRequest(step *runbook.Step, varCtx VariableContext) (*orchestration.Request, error) {
	config := step.Config
	req := &orchestration.Request{
		RequestedBy: h.requestedBy,
		Revisions:   make(map[string]string),
	}

	// Plan name (required)
	if planName, ok := config["plan"].(string); ok {
		resolved, err := varCtx.Resolve(planName)
		if err != nil {
			return nil, fmt.Errorf("resolve plan name: %w", err)
		}
		req.PlanName = resolved
	}

	// Reason (optional)
	if reason, ok := config["reason"].(string); ok {
		resolved, err := varCtx.Resolve(reason)
		if err != nil {
			return nil, fmt.Errorf("resolve reason: %w", err)
		}
		req.Reason = resolved
	} else {
		req.Reason = fmt.Sprintf("runbook execution: %s", varCtx.RunbookName())
	}

	// Revisions (optional)
	if revisions, ok := config["revisions"].(map[string]interface{}); ok {
		for repo, rev := range revisions {
			if revStr, ok := rev.(string); ok {
				resolved, err := varCtx.Resolve(revStr)
				if err != nil {
					return nil, fmt.Errorf("resolve revision for %s: %w", repo, err)
				}
				req.Revisions[repo] = resolved
			}
		}
	}

	// Dry run (optional)
	if dryRun, ok := config["dry_run"].(bool); ok {
		req.DryRun = dryRun
	}

	// Force (optional)
	if force, ok := config["force"].(bool); ok {
		req.Force = force
	}

	// Groups to skip (optional)
	if groups, ok := config["groups_to_skip"].([]interface{}); ok {
		for _, g := range groups {
			if groupStr, ok := g.(string); ok {
				resolved, err := varCtx.Resolve(groupStr)
				if err != nil {
					return nil, fmt.Errorf("resolve group to skip: %w", err)
				}
				req.GroupsToSkip = append(req.GroupsToSkip, resolved)
			}
		}
	}

	// Add execution ID to requested_by for tracking
	req.RequestedBy = fmt.Sprintf("%s (execution: %s)", h.requestedBy, varCtx.ExecutionID())

	return req, nil
}

// getTimeout extracts timeout configuration.
func (h *DeployHandler) getTimeout(config map[string]interface{}) time.Duration {
	if timeoutStr, ok := config["timeout"].(string); ok {
		if d, err := time.ParseDuration(timeoutStr); err == nil {
			return d
		}
	}
	return 0 // No timeout
}

// shouldRollback checks if rollback is configured.
func (h *DeployHandler) shouldRollback(config map[string]interface{}) bool {
	if rollback, ok := config["rollback_on_failure"].(bool); ok {
		return rollback
	}
	return false
}

// handleRollback handles rollback on deployment failure.
func (h *DeployHandler) handleRollback(ctx context.Context, failedResult *orchestration.Result, startTime time.Time) (*runbook.StepResult, error) {
	// Attempt rollback
	rollbackResult, rollbackErr := h.orchestrator.Rollback(ctx, failedResult.ID)

	outputs := h.extractOutputs(failedResult)
	outputs["rollback_attempted"] = true

	if rollbackErr != nil {
		outputs["rollback_error"] = rollbackErr.Error()
		outputs["rollback_success"] = false

		return &runbook.StepResult{
			Success:  false,
			Message:  fmt.Sprintf("deployment failed and rollback failed: %v", rollbackErr),
			Duration: time.Since(startTime),
			Outputs:  outputs,
		}, fmt.Errorf("deployment failed, rollback failed: %w", rollbackErr)
	}

	if rollbackResult != nil {
		outputs["rollback_id"] = rollbackResult.ID
		outputs["rollback_status"] = string(rollbackResult.Status)
		outputs["rollback_success"] = rollbackResult.Status == orchestration.StatusCompleted
	}

	return &runbook.StepResult{
		Success:  false,
		Message:  "deployment failed, rollback completed",
		Duration: time.Since(startTime),
		Outputs:  outputs,
	}, fmt.Errorf("deployment failed, rolled back")
}

// buildResult builds a step result from an orchestration result.
func (h *DeployHandler) buildResult(result *orchestration.Result, startTime time.Time) *runbook.StepResult {
	outputs := h.extractOutputs(result)

	success := result.Status == orchestration.StatusCompleted
	var message string

	if success {
		message = fmt.Sprintf("deployment completed successfully: %d groups deployed", len(result.GroupResults))
	} else {
		message = fmt.Sprintf("deployment %s: %s", result.Status, h.getFailureReason(result))
	}

	return &runbook.StepResult{
		Success:  success,
		Message:  message,
		Duration: time.Since(startTime),
		Outputs:  outputs,
	}
}

// buildErrorResult builds an error result from an orchestration result.
func (h *DeployHandler) buildErrorResult(result *orchestration.Result, err error, startTime time.Time, config map[string]interface{}) *runbook.StepResult {
	outputs := map[string]interface{}{
		"error": err.Error(),
	}

	if result != nil {
		outputs = h.extractOutputs(result)
		outputs["error"] = err.Error()
	}

	return &runbook.StepResult{
		Success:  false,
		Message:  fmt.Sprintf("deployment execution failed: %v", err),
		Duration: time.Since(startTime),
		Outputs:  outputs,
	}
}

// extractOutputs extracts outputs from an orchestration result.
func (h *DeployHandler) extractOutputs(result *orchestration.Result) map[string]interface{} {
	outputs := map[string]interface{}{
		"orchestration_id": result.ID,
		"status":           string(result.Status),
		"duration_ms":      result.Duration.Milliseconds(),
	}

	if result.Plan != nil {
		outputs["plan_name"] = result.Plan.Name
		outputs["plan_version"] = result.Plan.Version
	}

	// Execution order
	outputs["execution_order"] = result.ExecutionOrder
	outputs["group_count"] = len(result.GroupResults)
	outputs["failed_groups"] = result.FailedGroups

	// Group results summary
	groupSummary := make([]map[string]interface{}, 0, len(result.GroupResults))
	for _, gr := range result.GroupResults {
		summary := map[string]interface{}{
			"group_name":  gr.GroupName,
			"status":      string(gr.Status),
			"duration_ms": gr.Duration.Milliseconds(),
		}
		if len(gr.RepoResults) > 0 {
			repoSummary := make([]map[string]interface{}, 0, len(gr.RepoResults))
			for _, rr := range gr.RepoResults {
				repoSummary = append(repoSummary, map[string]interface{}{
					"repo":     rr.RepoName,
					"status":   string(rr.Status),
					"revision": rr.Revision,
				})
			}
			summary["repos"] = repoSummary
		}
		if gr.VerificationResult != nil {
			summary["verification_passed"] = gr.VerificationResult.Passed
		}
		groupSummary = append(groupSummary, summary)
	}
	outputs["groups"] = groupSummary

	// Approval info if present
	if result.ApprovalInfo != nil {
		outputs["approval_required"] = result.ApprovalInfo.Required
		outputs["approval_approved"] = result.ApprovalInfo.ApprovedBy != ""
		outputs["approval_by"] = result.ApprovalInfo.ApprovedBy
	}

	return outputs
}

// getFailureReason extracts a failure reason from the result.
func (h *DeployHandler) getFailureReason(result *orchestration.Result) string {
	if len(result.FailedGroups) > 0 {
		return fmt.Sprintf("failed groups: %v", result.FailedGroups)
	}
	for _, gr := range result.GroupResults {
		if gr.Status == orchestration.StatusFailed {
			if gr.Error != "" {
				return gr.Error
			}
			for _, rr := range gr.RepoResults {
				if rr.Status == orchestration.StatusFailed {
					return fmt.Sprintf("repo %s: %s", rr.RepoName, rr.Error)
				}
			}
		}
	}
	return "unknown failure"
}
