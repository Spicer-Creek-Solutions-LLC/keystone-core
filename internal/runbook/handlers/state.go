package handlers

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/shawnbutts/keystone-core/internal/runbook"
	"github.com/shawnbutts/keystone-core/internal/statemgmt"
	"gopkg.in/yaml.v3"
)

// StateExecutor defines the interface for state execution.
type StateExecutor interface {
	// ExecuteState executes a state file and returns the results.
	ExecuteState(ctx context.Context, stateFile *statemgmt.StateFile) (*statemgmt.StateRun, error)
}

// StateHandler handles state step execution.
// Configuration:
//   - inline: Inline YAML state definitions
//   - file: Path to a state file
//   - dry_run: If true, only preview changes without applying
//   - fail_fast: If true, stop on first failure (default: true)
//   - target: Agent targeting expression (optional)
type StateHandler struct {
	executor StateExecutor
	dryRun   bool
}

// NewStateHandler creates a new state step handler.
func NewStateHandler(executor StateExecutor) *StateHandler {
	return &StateHandler{
		executor: executor,
	}
}

// NewStateHandlerWithOptions creates a new state handler with options.
func NewStateHandlerWithOptions(executor StateExecutor, dryRun bool) *StateHandler {
	return &StateHandler{
		executor: executor,
		dryRun:   dryRun,
	}
}

// Type returns the step type.
func (h *StateHandler) Type() runbook.StepType {
	return runbook.StepTypeState
}

// Validate validates the step configuration.
func (h *StateHandler) Validate(step *runbook.Step) error {
	config := step.Config

	// Must have either inline or file
	_, hasInline := config["inline"]
	_, hasFile := config["file"]

	if !hasInline && !hasFile {
		return fmt.Errorf("state step requires either 'inline' or 'file' configuration")
	}

	if hasInline && hasFile {
		return fmt.Errorf("state step cannot have both 'inline' and 'file' configuration")
	}

	// Validate inline YAML if provided
	if hasInline {
		inline, ok := config["inline"].(string)
		if !ok {
			return fmt.Errorf("inline must be a string")
		}
		// Try to parse the YAML to validate syntax
		var states map[string]interface{}
		if err := yaml.Unmarshal([]byte(inline), &states); err != nil {
			return fmt.Errorf("invalid inline YAML: %w", err)
		}
	}

	return nil
}

// Execute executes the state step.
func (h *StateHandler) Execute(ctx context.Context, step *runbook.Step, varCtx VariableContext) (*runbook.StepResult, error) {
	startTime := time.Now()

	// Check if executor is available
	if h.executor == nil {
		return &runbook.StepResult{
			Success:  false,
			Message:  "state executor not configured",
			Duration: time.Since(startTime),
		}, fmt.Errorf("state executor not configured")
	}

	// Build state file from configuration
	stateFile, err := h.buildStateFile(step, varCtx)
	if err != nil {
		return &runbook.StepResult{
			Success:  false,
			Message:  fmt.Sprintf("failed to build state file: %v", err),
			Duration: time.Since(startTime),
		}, err
	}

	// Execute state
	run, err := h.executor.ExecuteState(ctx, stateFile)
	if err != nil {
		return h.buildErrorResult(run, err, startTime), err
	}

	return h.buildResult(run, startTime), nil
}

// buildStateFile builds a StateFile from step configuration.
func (h *StateHandler) buildStateFile(step *runbook.Step, varCtx VariableContext) (*statemgmt.StateFile, error) {
	config := step.Config
	stateFile := &statemgmt.StateFile{
		States: make(map[string][]statemgmt.StateDeclaration),
	}

	// Handle inline YAML
	if inline, ok := config["inline"].(string); ok {
		// Resolve template variables in inline YAML
		resolved, err := varCtx.Resolve(inline)
		if err != nil {
			return nil, fmt.Errorf("resolve inline template: %w", err)
		}

		// Parse the YAML
		var states map[string]map[string]interface{}
		if err := yaml.Unmarshal([]byte(resolved), &states); err != nil {
			return nil, fmt.Errorf("parse inline YAML: %w", err)
		}

		// Convert to state declarations organized by module
		for module, moduleStates := range states {
			for name, stateConfig := range moduleStates {
				configMap, ok := stateConfig.(map[string]interface{})
				if !ok {
					configMap = map[string]interface{}{"state": stateConfig}
				}

				decl := statemgmt.StateDeclaration{
					ID:         fmt.Sprintf("%s_%s", module, name),
					Module:     module,
					Parameters: configMap,
				}

				// Extract state from config if present
				if state, ok := configMap["state"].(string); ok {
					decl.State = state
				}

				stateFile.States[module] = append(stateFile.States[module], decl)
			}
		}

		stateFile.Path = "<inline>"
	}

	// Handle file path
	if filePath, ok := config["file"].(string); ok {
		resolved, err := varCtx.Resolve(filePath)
		if err != nil {
			return nil, fmt.Errorf("resolve file path: %w", err)
		}

		// Load state file from path using loader
		loaded, err := h.loadStateFile(resolved)
		if err != nil {
			return nil, fmt.Errorf("load state file %s: %w", resolved, err)
		}
		stateFile = loaded
	}

	// Store target in variables if specified (StateFile uses Variables for context)
	if target, ok := config["target"].(string); ok {
		resolved, err := varCtx.Resolve(target)
		if err != nil {
			return nil, fmt.Errorf("resolve target: %w", err)
		}
		if stateFile.Variables == nil {
			stateFile.Variables = make(map[string]interface{})
		}
		stateFile.Variables["__target__"] = resolved
	}

	return stateFile, nil
}

// loadStateFile loads a state file from disk.
func (h *StateHandler) loadStateFile(path string) (*statemgmt.StateFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var stateFile statemgmt.StateFile
	if err := yaml.Unmarshal(data, &stateFile); err != nil {
		return nil, err
	}

	stateFile.Path = path
	return &stateFile, nil
}

// buildResult builds a step result from a state run.
func (h *StateHandler) buildResult(run *statemgmt.StateRun, startTime time.Time) *runbook.StepResult {
	outputs := map[string]interface{}{
		"run_id": run.RunID,
	}

	var success bool
	var message string

	if run.Summary != nil {
		outputs["total"] = run.Summary.Total
		outputs["succeeded"] = run.Summary.Succeeded
		outputs["failed"] = run.Summary.Failed
		outputs["changed"] = run.Summary.Changed
		outputs["unchanged"] = run.Summary.Unchanged
		outputs["success"] = run.Summary.Success

		success = run.Summary.Success
		if success {
			message = fmt.Sprintf("state applied successfully: %d states, %d changed", run.Summary.Total, run.Summary.Changed)
		} else {
			message = fmt.Sprintf("state apply failed: %d/%d states failed", run.Summary.Failed, run.Summary.Total)
		}
	} else {
		success = false
		message = "state run completed but no summary available"
	}

	// Include individual state results
	stateResults := make([]map[string]interface{}, 0, len(run.Results))
	for _, result := range run.Results {
		stateResult := map[string]interface{}{
			"state_id": result.StateID,
			"module":   result.Module,
			"success":  result.Success,
			"changed":  result.Changed,
			"comment":  result.Comment,
		}
		if result.Error != nil {
			stateResult["error"] = result.Error.Error()
		}
		if len(result.Changes) > 0 {
			stateResult["changes"] = result.Changes
		}
		stateResults = append(stateResults, stateResult)
	}
	outputs["results"] = stateResults

	return &runbook.StepResult{
		Success:  success,
		Message:  message,
		Duration: time.Since(startTime),
		Outputs:  outputs,
	}
}

// buildErrorResult builds an error result from a state run.
func (h *StateHandler) buildErrorResult(run *statemgmt.StateRun, err error, startTime time.Time) *runbook.StepResult {
	outputs := map[string]interface{}{
		"error": err.Error(),
	}

	if run != nil {
		outputs["run_id"] = run.RunID
		if run.Summary != nil {
			outputs["total"] = run.Summary.Total
			outputs["succeeded"] = run.Summary.Succeeded
			outputs["failed"] = run.Summary.Failed
		}
	}

	return &runbook.StepResult{
		Success:  false,
		Message:  fmt.Sprintf("state execution failed: %v", err),
		Duration: time.Since(startTime),
		Outputs:  outputs,
	}
}
