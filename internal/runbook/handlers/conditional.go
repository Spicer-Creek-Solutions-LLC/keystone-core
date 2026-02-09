package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// IfHandler executes different steps based on a condition.
// Configuration:
//   - condition: Expression to evaluate (required)
//   - then: Steps to execute if condition is true (required)
//   - else: Steps to execute if condition is false (optional)
type IfHandler struct {
	stepExecutor StepExecutor
}

// StepExecutor is the interface for executing nested steps.
type StepExecutor interface {
	ExecuteSteps(ctx context.Context, steps []runbook.Step, varCtx VariableContext) error
}

// NewIfHandler creates a new if handler.
func NewIfHandler(executor StepExecutor) *IfHandler {
	return &IfHandler{stepExecutor: executor}
}

// Type returns the step type.
func (h *IfHandler) Type() runbook.StepType {
	return runbook.StepTypeIf
}

// Validate validates the step configuration.
func (h *IfHandler) Validate(step *runbook.Step) error {
	if _, ok := step.Config["condition"]; !ok {
		return fmt.Errorf("if step requires 'condition' configuration")
	}

	if _, ok := step.Config["then"]; !ok {
		return fmt.Errorf("if step requires 'then' configuration")
	}

	return nil
}

// Execute executes the if step.
func (h *IfHandler) Execute(ctx context.Context, step *runbook.Step, varCtx VariableContext) (*runbook.StepResult, error) {
	startTime := time.Now()

	// Get condition expression
	condition, ok := step.Config["condition"].(string)
	if !ok {
		return &runbook.StepResult{
			Success:  false,
			Message:  "condition must be a string",
			Duration: time.Since(startTime),
		}, fmt.Errorf("condition must be a string")
	}

	// Evaluate condition
	result, err := varCtx.EvaluateCondition(condition)
	if err != nil {
		return &runbook.StepResult{
			Success:  false,
			Message:  fmt.Sprintf("condition evaluation failed: %v", err),
			Duration: time.Since(startTime),
		}, err
	}

	var branchSteps []runbook.Step
	branchName := "then"

	if result {
		// Execute 'then' branch
		thenSteps, err := parseStepList(step.Config["then"])
		if err != nil {
			return &runbook.StepResult{
				Success:  false,
				Message:  fmt.Sprintf("invalid 'then' steps: %v", err),
				Duration: time.Since(startTime),
			}, err
		}
		branchSteps = thenSteps
	} else {
		// Execute 'else' branch if present
		if elseConfig, ok := step.Config["else"]; ok {
			elseSteps, err := parseStepList(elseConfig)
			if err != nil {
				return &runbook.StepResult{
					Success:  false,
					Message:  fmt.Sprintf("invalid 'else' steps: %v", err),
					Duration: time.Since(startTime),
				}, err
			}
			branchSteps = elseSteps
			branchName = "else"
		} else {
			// No else branch, return success
			return &runbook.StepResult{
				Success:  true,
				Message:  "condition evaluated to false, no else branch",
				Duration: time.Since(startTime),
				Outputs: map[string]interface{}{
					"branch":    "none",
					"condition": result,
				},
			}, nil
		}
	}

	// Execute branch steps
	if h.stepExecutor != nil && len(branchSteps) > 0 {
		if err := h.stepExecutor.ExecuteSteps(ctx, branchSteps, varCtx); err != nil {
			return &runbook.StepResult{
				Success:  false,
				Message:  fmt.Sprintf("branch '%s' execution failed: %v", branchName, err),
				Duration: time.Since(startTime),
				Outputs: map[string]interface{}{
					"branch":    branchName,
					"condition": result,
				},
			}, err
		}
	}

	return &runbook.StepResult{
		Success:  true,
		Message:  fmt.Sprintf("executed '%s' branch", branchName),
		Duration: time.Since(startTime),
		Outputs: map[string]interface{}{
			"branch":    branchName,
			"condition": result,
		},
	}, nil
}

// SwitchHandler executes different steps based on a value matching cases.
// Configuration:
//   - value: Expression to evaluate (required)
//   - cases: Map of case values to step lists (required)
//   - default: Steps to execute if no case matches (optional)
type SwitchHandler struct {
	stepExecutor StepExecutor
}

// NewSwitchHandler creates a new switch handler.
func NewSwitchHandler(executor StepExecutor) *SwitchHandler {
	return &SwitchHandler{stepExecutor: executor}
}

// Type returns the step type.
func (h *SwitchHandler) Type() runbook.StepType {
	return runbook.StepTypeSwitch
}

// Validate validates the step configuration.
func (h *SwitchHandler) Validate(step *runbook.Step) error {
	if _, ok := step.Config["value"]; !ok {
		return fmt.Errorf("switch step requires 'value' configuration")
	}

	if _, ok := step.Config["cases"]; !ok {
		return fmt.Errorf("switch step requires 'cases' configuration")
	}

	return nil
}

// Execute executes the switch step.
func (h *SwitchHandler) Execute(ctx context.Context, step *runbook.Step, varCtx VariableContext) (*runbook.StepResult, error) {
	startTime := time.Now()

	// Get value expression
	valueExpr, ok := step.Config["value"].(string)
	if !ok {
		return &runbook.StepResult{
			Success:  false,
			Message:  "value must be a string expression",
			Duration: time.Since(startTime),
		}, fmt.Errorf("value must be a string expression")
	}

	// Resolve value
	resolvedValue, err := varCtx.Resolve(valueExpr)
	if err != nil {
		return &runbook.StepResult{
			Success:  false,
			Message:  fmt.Sprintf("value resolution failed: %v", err),
			Duration: time.Since(startTime),
		}, err
	}

	// Get cases
	casesConfig, ok := step.Config["cases"].(map[string]interface{})
	if !ok {
		return &runbook.StepResult{
			Success:  false,
			Message:  "cases must be a map",
			Duration: time.Since(startTime),
		}, fmt.Errorf("cases must be a map")
	}

	// Find matching case
	var matchedCase string
	var matchedSteps []runbook.Step

	for caseName, caseSteps := range casesConfig {
		if caseName != resolvedValue {
			continue
		}
		matchedCase = caseName
		steps, err := parseStepList(caseSteps)
		if err != nil {
			return &runbook.StepResult{
				Success:  false,
				Message:  fmt.Sprintf("invalid steps for case '%s': %v", caseName, err),
				Duration: time.Since(startTime),
			}, err
		}
		matchedSteps = steps
		break
	}

	// If no case matched, try default
	if matchedCase == "" {
		if defaultConfig, ok := step.Config["default"]; ok {
			matchedCase = "default"
			steps, err := parseStepList(defaultConfig)
			if err != nil {
				return &runbook.StepResult{
					Success:  false,
					Message:  fmt.Sprintf("invalid 'default' steps: %v", err),
					Duration: time.Since(startTime),
				}, err
			}
			matchedSteps = steps
		} else {
			// No match and no default
			return &runbook.StepResult{
				Success:  true,
				Message:  fmt.Sprintf("no case matched for value '%s', no default", resolvedValue),
				Duration: time.Since(startTime),
				Outputs: map[string]interface{}{
					"matched_case": "none",
					"value":        resolvedValue,
				},
			}, nil
		}
	}

	// Execute matched steps
	if h.stepExecutor != nil && len(matchedSteps) > 0 {
		if err := h.stepExecutor.ExecuteSteps(ctx, matchedSteps, varCtx); err != nil {
			return &runbook.StepResult{
				Success:  false,
				Message:  fmt.Sprintf("case '%s' execution failed: %v", matchedCase, err),
				Duration: time.Since(startTime),
				Outputs: map[string]interface{}{
					"matched_case": matchedCase,
					"value":        resolvedValue,
				},
			}, err
		}
	}

	return &runbook.StepResult{
		Success:  true,
		Message:  fmt.Sprintf("executed case '%s'", matchedCase),
		Duration: time.Since(startTime),
		Outputs: map[string]interface{}{
			"matched_case": matchedCase,
			"value":        resolvedValue,
		},
	}, nil
}

// parseStepList parses a step list from interface{}.
func parseStepList(v interface{}) ([]runbook.Step, error) {
	// Handle slice of interface{}
	list, ok := v.([]interface{})
	if !ok {
		return nil, fmt.Errorf("expected a list of steps")
	}

	var steps []runbook.Step
	for i, item := range list {
		stepMap, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("step %d is not a map", i)
		}

		step, err := parseStep(stepMap)
		if err != nil {
			return nil, fmt.Errorf("step %d: %w", i, err)
		}
		steps = append(steps, step)
	}

	return steps, nil
}

// parseStep parses a single step from a map.
func parseStep(m map[string]interface{}) (runbook.Step, error) {
	step := runbook.Step{
		Config: make(map[string]interface{}),
	}

	// Parse required fields
	if name, ok := m["name"].(string); ok {
		step.Name = name
	} else {
		return step, fmt.Errorf("step missing 'name'")
	}

	if stepType, ok := m["type"].(string); ok {
		step.Type = runbook.StepType(stepType)
	} else {
		return step, fmt.Errorf("step missing 'type'")
	}

	// Parse optional fields
	if desc, ok := m["description"].(string); ok {
		step.Description = desc
	}

	if condition, ok := m["condition"].(string); ok {
		step.Condition = condition
	}

	if timeout, ok := m["timeout"].(string); ok {
		step.Timeout = timeout
	}

	if continueOnError, ok := m["continueOnError"].(bool); ok {
		step.ContinueOnError = continueOnError
	}

	// Parse dependsOn
	if deps, ok := m["dependsOn"].([]interface{}); ok {
		for _, dep := range deps {
			if depStr, ok := dep.(string); ok {
				step.DependsOn = append(step.DependsOn, depStr)
			}
		}
	}

	// Parse config
	if config, ok := m["config"].(map[string]interface{}); ok {
		step.Config = config
	}

	// Parse outputs
	if outputs, ok := m["outputs"].([]interface{}); ok {
		for _, out := range outputs {
			outMap, ok := out.(map[string]interface{})
			if !ok {
				continue
			}
			output := runbook.OutputDef{}
			if name, ok := outMap["name"].(string); ok {
				output.Name = name
			}
			if source, ok := outMap["source"].(string); ok {
				output.Source = runbook.OutputSource(source)
			}
			if parser, ok := outMap["parser"].(string); ok {
				output.Parser = runbook.OutputParser(parser)
			}
			if path, ok := outMap["path"].(string); ok {
				output.Path = path
			}
			if def, ok := outMap["default"]; ok {
				output.Default = def
			}
			step.Outputs = append(step.Outputs, output)
		}
	}

	// Parse retries
	if retries, ok := m["retries"].(map[string]interface{}); ok {
		step.Retries = &runbook.RetryConfig{}
		switch maxAttempts := retries["maxAttempts"].(type) {
		case int:
			step.Retries.MaxAttempts = maxAttempts
		case float64:
			step.Retries.MaxAttempts = int(maxAttempts)
		}
		if delay, ok := retries["delay"].(string); ok {
			step.Retries.Delay = delay
		}
		if maxDelay, ok := retries["maxDelay"].(string); ok {
			step.Retries.MaxDelay = maxDelay
		}
		if backoff, ok := retries["backoff"].(string); ok {
			step.Retries.Backoff = runbook.BackoffType(backoff)
		}
	}

	return step, nil
}
