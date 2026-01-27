package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// SubRunbookHandler executes another runbook as a nested step.
// Configuration:
//   - runbook: Name of the runbook to execute (required)
//   - version: Version constraint (optional, default: latest)
//   - inputs: Map of input values to pass to the sub-runbook (optional)
//   - outputMapping: Map of sub-runbook outputs to step outputs (optional)
//   - async: Whether to execute asynchronously (default: false)
//   - timeout: Maximum execution time (optional)
type SubRunbookHandler struct {
	runbookLoader   RunbookLoader
	runbookExecutor RunbookExecutor
}

// RunbookLoader is the interface for loading runbooks.
type RunbookLoader interface {
	// Load loads a runbook by name and version.
	Load(ctx context.Context, name, version string) (*runbook.Runbook, error)
}

// RunbookExecutor is the interface for executing runbooks.
type RunbookExecutor interface {
	// Execute executes a runbook with the given inputs.
	Execute(ctx context.Context, rb *runbook.Runbook, inputs map[string]interface{}) (*runbook.Execution, error)
}

// NewSubRunbookHandler creates a new sub-runbook handler.
func NewSubRunbookHandler(loader RunbookLoader, executor RunbookExecutor) *SubRunbookHandler {
	return &SubRunbookHandler{
		runbookLoader:   loader,
		runbookExecutor: executor,
	}
}

// Type returns the step type.
func (h *SubRunbookHandler) Type() runbook.StepType {
	return runbook.StepTypeSubRunbook
}

// Validate validates the step configuration.
func (h *SubRunbookHandler) Validate(step *runbook.Step) error {
	if _, ok := step.Config["runbook"]; !ok {
		return fmt.Errorf("runbook step requires 'runbook' configuration")
	}

	return nil
}

// Execute executes the sub-runbook.
func (h *SubRunbookHandler) Execute(ctx context.Context, step *runbook.Step, varCtx VariableContext) (*runbook.StepResult, error) {
	startTime := time.Now()

	// Get runbook name
	runbookName, ok := step.Config["runbook"].(string)
	if !ok {
		return &runbook.StepResult{
			Success:  false,
			Message:  "runbook must be a string",
			Duration: time.Since(startTime),
		}, fmt.Errorf("runbook must be a string")
	}

	// Resolve runbook name (may be a template)
	resolvedName, err := varCtx.Resolve(runbookName)
	if err != nil {
		return &runbook.StepResult{
			Success:  false,
			Message:  fmt.Sprintf("failed to resolve runbook name: %v", err),
			Duration: time.Since(startTime),
		}, err
	}

	// Get version (optional)
	version := ""
	if v, ok := step.Config["version"].(string); ok {
		resolvedVersion, err := varCtx.Resolve(v)
		if err != nil {
			return &runbook.StepResult{
				Success:  false,
				Message:  fmt.Sprintf("failed to resolve version: %v", err),
				Duration: time.Since(startTime),
			}, err
		}
		version = resolvedVersion
	}

	// Check recursion depth
	depth := getRecursionDepth(varCtx)
	if depth > maxRecursionDepth {
		return &runbook.StepResult{
			Success:  false,
			Message:  fmt.Sprintf("maximum recursion depth (%d) exceeded", maxRecursionDepth),
			Duration: time.Since(startTime),
		}, fmt.Errorf("maximum recursion depth exceeded")
	}

	// Check if runbook loader is available
	if h.runbookLoader == nil {
		return &runbook.StepResult{
			Success:  false,
			Message:  "runbook loader not configured",
			Duration: time.Since(startTime),
			Outputs: map[string]interface{}{
				"runbook": resolvedName,
				"version": version,
			},
		}, fmt.Errorf("runbook loader not configured")
	}

	// Load the sub-runbook
	subRunbook, err := h.runbookLoader.Load(ctx, resolvedName, version)
	if err != nil {
		return &runbook.StepResult{
			Success:  false,
			Message:  fmt.Sprintf("failed to load runbook '%s': %v", resolvedName, err),
			Duration: time.Since(startTime),
			Outputs: map[string]interface{}{
				"runbook": resolvedName,
				"version": version,
			},
		}, err
	}

	// Build input mapping
	subInputs, err := h.buildInputs(step.Config, varCtx)
	if err != nil {
		return &runbook.StepResult{
			Success:  false,
			Message:  fmt.Sprintf("failed to build inputs: %v", err),
			Duration: time.Since(startTime),
			Outputs: map[string]interface{}{
				"runbook": resolvedName,
				"version": version,
			},
		}, err
	}

	// Check if runbook executor is available
	if h.runbookExecutor == nil {
		return &runbook.StepResult{
			Success:  false,
			Message:  "runbook executor not configured",
			Duration: time.Since(startTime),
			Outputs: map[string]interface{}{
				"runbook": resolvedName,
				"version": version,
			},
		}, fmt.Errorf("runbook executor not configured")
	}

	// Execute the sub-runbook
	execution, err := h.runbookExecutor.Execute(ctx, subRunbook, subInputs)
	if err != nil {
		return &runbook.StepResult{
			Success:  false,
			Message:  fmt.Sprintf("sub-runbook execution failed: %v", err),
			Duration: time.Since(startTime),
			Outputs: map[string]interface{}{
				"runbook":      resolvedName,
				"version":      version,
				"execution_id": getExecutionID(execution),
				"state":        getExecutionState(execution),
			},
		}, err
	}

	// Map outputs
	outputs := h.mapOutputs(execution, step.Config)
	outputs["runbook"] = resolvedName
	outputs["version"] = version
	outputs["execution_id"] = execution.ID
	outputs["state"] = string(execution.State)

	return &runbook.StepResult{
		Success:  execution.State == runbook.ExecutionStateCompleted,
		Message:  fmt.Sprintf("sub-runbook '%s' %s", resolvedName, execution.State),
		Duration: time.Since(startTime),
		Outputs:  outputs,
	}, nil
}

// maxRecursionDepth is the maximum allowed nesting depth for sub-runbooks.
const maxRecursionDepth = 10

// getRecursionDepth extracts the current recursion depth from the context.
func getRecursionDepth(varCtx VariableContext) int {
	// Check for a _recursion_depth input
	if depth, ok := varCtx.GetInput("_recursion_depth"); ok {
		switch v := depth.(type) {
		case int:
			return v
		case float64:
			return int(v)
		}
	}
	return 0
}

// buildInputs builds the input map for the sub-runbook.
func (h *SubRunbookHandler) buildInputs(config map[string]interface{}, varCtx VariableContext) (map[string]interface{}, error) {
	inputs := make(map[string]interface{})

	// Add recursion depth tracking
	inputs["_recursion_depth"] = getRecursionDepth(varCtx) + 1

	// Add parent context info
	inputs["_parent_execution_id"] = varCtx.ExecutionID()
	inputs["_parent_runbook"] = varCtx.RunbookName()

	// Get explicit inputs from config
	if inputsConfig, ok := config["inputs"].(map[string]interface{}); ok {
		for name, value := range inputsConfig {
			// Resolve value if it's a string (template)
			if strVal, ok := value.(string); ok {
				resolved, err := varCtx.ResolveValue(strVal)
				if err != nil {
					return nil, fmt.Errorf("failed to resolve input '%s': %w", name, err)
				}
				inputs[name] = resolved
			} else {
				inputs[name] = value
			}
		}
	}

	return inputs, nil
}

// mapOutputs maps sub-runbook outputs to step outputs.
func (h *SubRunbookHandler) mapOutputs(execution *runbook.Execution, config map[string]interface{}) map[string]interface{} {
	outputs := make(map[string]interface{})

	if execution == nil {
		return outputs
	}

	// Get output mapping from config
	mapping, _ := config["outputMapping"].(map[string]interface{})

	// If no mapping, include all sub-runbook outputs
	if mapping == nil {
		for name, value := range execution.Outputs {
			outputs[name] = value
		}
		return outputs
	}

	// Apply mapping
	for stepOutput, subOutput := range mapping {
		if subOutputStr, ok := subOutput.(string); ok {
			if value, ok := execution.Outputs[subOutputStr]; ok {
				outputs[stepOutput] = value
			}
		}
	}

	return outputs
}

// getExecutionID safely extracts the execution ID.
func getExecutionID(exec *runbook.Execution) string {
	if exec == nil {
		return ""
	}
	return exec.ID
}

// getExecutionState safely extracts the execution state.
func getExecutionState(exec *runbook.Execution) string {
	if exec == nil {
		return ""
	}
	return string(exec.State)
}
