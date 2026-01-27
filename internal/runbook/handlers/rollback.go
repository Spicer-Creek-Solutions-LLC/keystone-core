package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/shawnbutts/keystone-core/internal/gitops/orchestration"
	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// RollbackHandler handles rollback step execution.
// Configuration:
//   - orchestration_id: ID of the orchestration to rollback (required)
//     Can use template: {{ steps.deploy_step.outputs.orchestration_id }}
//   - reason: Reason for the rollback (optional)
//   - timeout: Rollback timeout duration (optional)
type RollbackHandler struct {
	orchestrator DeployOrchestrator
}

// NewRollbackHandler creates a new rollback step handler.
func NewRollbackHandler(orchestrator DeployOrchestrator) *RollbackHandler {
	return &RollbackHandler{
		orchestrator: orchestrator,
	}
}

// Type returns the step type.
func (h *RollbackHandler) Type() runbook.StepType {
	return runbook.StepTypeRollback
}

// Validate validates the step configuration.
func (h *RollbackHandler) Validate(step *runbook.Step) error {
	config := step.Config

	// Orchestration ID is required
	if _, ok := config["orchestration_id"]; !ok {
		return fmt.Errorf("rollback step requires 'orchestration_id' configuration")
	}

	return nil
}

// Execute executes the rollback step.
func (h *RollbackHandler) Execute(ctx context.Context, step *runbook.Step, varCtx VariableContext) (*runbook.StepResult, error) {
	startTime := time.Now()

	// Check if orchestrator is available
	if h.orchestrator == nil {
		return &runbook.StepResult{
			Success:  false,
			Message:  "deploy orchestrator not configured",
			Duration: time.Since(startTime),
		}, fmt.Errorf("deploy orchestrator not configured")
	}

	config := step.Config

	// Get orchestration ID
	orchestrationID, err := h.getOrchestrationID(config, varCtx)
	if err != nil {
		return &runbook.StepResult{
			Success:  false,
			Message:  fmt.Sprintf("failed to get orchestration ID: %v", err),
			Duration: time.Since(startTime),
		}, err
	}

	// Apply timeout if configured
	timeout := h.getTimeout(config)
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// Execute rollback
	result, err := h.orchestrator.Rollback(ctx, orchestrationID)
	if err != nil {
		return h.buildErrorResult(orchestrationID, err, startTime), err
	}

	return h.buildResult(result, orchestrationID, startTime), nil
}

// getOrchestrationID extracts and resolves the orchestration ID.
func (h *RollbackHandler) getOrchestrationID(config map[string]interface{}, varCtx VariableContext) (string, error) {
	idValue, ok := config["orchestration_id"]
	if !ok {
		return "", fmt.Errorf("orchestration_id is required")
	}

	idStr, ok := idValue.(string)
	if !ok {
		return "", fmt.Errorf("orchestration_id must be a string")
	}

	// Resolve template variables (e.g., {{ steps.deploy.outputs.orchestration_id }})
	resolved, err := varCtx.Resolve(idStr)
	if err != nil {
		return "", fmt.Errorf("resolve orchestration_id: %w", err)
	}

	if resolved == "" {
		return "", fmt.Errorf("orchestration_id resolved to empty string")
	}

	return resolved, nil
}

// getTimeout extracts timeout configuration.
func (h *RollbackHandler) getTimeout(config map[string]interface{}) time.Duration {
	if timeoutStr, ok := config["timeout"].(string); ok {
		if d, err := time.ParseDuration(timeoutStr); err == nil {
			return d
		}
	}
	return 0
}

// buildResult builds a step result from a rollback result.
func (h *RollbackHandler) buildResult(result *orchestration.OrchestrationResult, orchestrationID string, startTime time.Time) *runbook.StepResult {
	outputs := map[string]interface{}{
		"original_orchestration_id": orchestrationID,
	}

	if result != nil {
		outputs["rollback_id"] = result.ID
		outputs["status"] = string(result.Status)
		outputs["duration_ms"] = result.Duration.Milliseconds()

		// Group results
		groupSummary := make([]map[string]interface{}, 0, len(result.GroupResults))
		for _, gr := range result.GroupResults {
			groupSummary = append(groupSummary, map[string]interface{}{
				"group_name":  gr.GroupName,
				"status":      string(gr.Status),
				"duration_ms": gr.Duration.Milliseconds(),
			})
		}
		outputs["groups"] = groupSummary
	}

	success := result != nil && result.Status == orchestration.StatusCompleted
	var message string

	if success {
		message = fmt.Sprintf("rollback completed successfully for orchestration %s", orchestrationID)
	} else if result != nil {
		message = fmt.Sprintf("rollback %s for orchestration %s", result.Status, orchestrationID)
	} else {
		message = "rollback completed with unknown status"
	}

	return &runbook.StepResult{
		Success:  success,
		Message:  message,
		Duration: time.Since(startTime),
		Outputs:  outputs,
	}
}

// buildErrorResult builds an error result.
func (h *RollbackHandler) buildErrorResult(orchestrationID string, err error, startTime time.Time) *runbook.StepResult {
	return &runbook.StepResult{
		Success: false,
		Message: fmt.Sprintf("rollback failed for orchestration %s: %v", orchestrationID, err),
		Duration: time.Since(startTime),
		Outputs: map[string]interface{}{
			"original_orchestration_id": orchestrationID,
			"error":                     err.Error(),
		},
	}
}
