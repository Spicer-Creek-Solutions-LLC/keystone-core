package handlers

import (
	"context"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// mockStepExecutor implements StepExecutor for testing.
type mockStepExecutor struct {
	executedSteps []runbook.Step
	returnErr     error
}

func (m *mockStepExecutor) ExecuteSteps(ctx context.Context, steps []runbook.Step, varCtx VariableContext) error {
	m.executedSteps = append(m.executedSteps, steps...)
	return m.returnErr
}

// mockVarContextWithCondition extends mockVarContext with condition evaluation.
type mockVarContextWithCondition struct {
	inputs           map[string]interface{}
	stepOutputs      map[string]map[string]interface{}
	resolveResult    string
	resolveErr       error
	conditionResults map[string]bool
}

func newMockVarContextWithCondition() *mockVarContextWithCondition {
	return &mockVarContextWithCondition{
		inputs:           make(map[string]interface{}),
		stepOutputs:      make(map[string]map[string]interface{}),
		conditionResults: make(map[string]bool),
	}
}

func (m *mockVarContextWithCondition) GetInput(name string) (interface{}, bool) {
	v, ok := m.inputs[name]
	return v, ok
}

func (m *mockVarContextWithCondition) GetStepOutput(stepName, outputName string) (interface{}, bool) {
	if outputs, ok := m.stepOutputs[stepName]; ok {
		v, ok := outputs[outputName]
		return v, ok
	}
	return nil, false
}

func (m *mockVarContextWithCondition) ExecutionID() string {
	return "test-exec-id"
}

func (m *mockVarContextWithCondition) RunbookName() string {
	return "test-runbook"
}

func (m *mockVarContextWithCondition) Resolve(template string) (string, error) {
	if m.resolveResult != "" {
		return m.resolveResult, m.resolveErr
	}
	return template, m.resolveErr
}

func (m *mockVarContextWithCondition) ResolveValue(template string) (interface{}, error) {
	if m.resolveResult != "" {
		return m.resolveResult, m.resolveErr
	}
	return template, m.resolveErr
}

func (m *mockVarContextWithCondition) EvaluateCondition(expr string) (bool, error) {
	if result, ok := m.conditionResults[expr]; ok {
		return result, nil
	}
	return false, nil
}

func TestIfHandler(t *testing.T) {
	t.Run("validate_without_condition", func(t *testing.T) {
		h := NewIfHandler(nil)

		step := &runbook.Step{
			Name:   "test-if",
			Type:   runbook.StepTypeIf,
			Config: map[string]interface{}{},
		}

		err := h.Validate(step)
		if err == nil {
			t.Error("expected error for missing condition")
		}
	})

	t.Run("validate_without_then", func(t *testing.T) {
		h := NewIfHandler(nil)

		step := &runbook.Step{
			Name: "test-if",
			Type: runbook.StepTypeIf,
			Config: map[string]interface{}{
				"condition": `{{ eq .inputs.env "prod" }}`,
			},
		}

		err := h.Validate(step)
		if err == nil {
			t.Error("expected error for missing then")
		}
	})

	t.Run("validate_with_condition_and_then", func(t *testing.T) {
		h := NewIfHandler(nil)

		step := &runbook.Step{
			Name: "test-if",
			Type: runbook.StepTypeIf,
			Config: map[string]interface{}{
				"condition": `{{ eq .inputs.env "prod" }}`,
				"then": []interface{}{
					map[string]interface{}{
						"name":   "deploy",
						"type":   "command",
						"config": map[string]interface{}{"command": "deploy.sh"},
					},
				},
			},
		}

		err := h.Validate(step)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("execute_then_branch", func(t *testing.T) {
		executor := &mockStepExecutor{}
		h := NewIfHandler(executor)

		varCtx := newMockVarContextWithCondition()
		varCtx.conditionResults[`{{ eq .inputs.env "prod" }}`] = true

		step := &runbook.Step{
			Name: "test-if",
			Type: runbook.StepTypeIf,
			Config: map[string]interface{}{
				"condition": `{{ eq .inputs.env "prod" }}`,
				"then": []interface{}{
					map[string]interface{}{
						"name":   "deploy",
						"type":   "command",
						"config": map[string]interface{}{"command": "deploy.sh"},
					},
				},
				"else": []interface{}{
					map[string]interface{}{
						"name":   "skip",
						"type":   "noop",
						"config": map[string]interface{}{},
					},
				},
			},
		}

		result, err := h.Execute(context.Background(), step, varCtx)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Errorf("expected success, got failure: %s", result.Message)
		}
		if len(executor.executedSteps) != 1 {
			t.Errorf("expected 1 step executed, got %d", len(executor.executedSteps))
		}
		if executor.executedSteps[0].Name != "deploy" {
			t.Errorf("expected 'deploy' step, got %s", executor.executedSteps[0].Name)
		}
		if result.Outputs["branch"] != "then" {
			t.Errorf("expected branch 'then', got %v", result.Outputs["branch"])
		}
	})

	t.Run("execute_else_branch", func(t *testing.T) {
		executor := &mockStepExecutor{}
		h := NewIfHandler(executor)

		varCtx := newMockVarContextWithCondition()
		varCtx.conditionResults[`{{ eq .inputs.env "prod" }}`] = false

		step := &runbook.Step{
			Name: "test-if",
			Type: runbook.StepTypeIf,
			Config: map[string]interface{}{
				"condition": `{{ eq .inputs.env "prod" }}`,
				"then": []interface{}{
					map[string]interface{}{
						"name":   "deploy",
						"type":   "command",
						"config": map[string]interface{}{"command": "deploy.sh"},
					},
				},
				"else": []interface{}{
					map[string]interface{}{
						"name":   "skip",
						"type":   "noop",
						"config": map[string]interface{}{},
					},
				},
			},
		}

		result, err := h.Execute(context.Background(), step, varCtx)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Errorf("expected success, got failure: %s", result.Message)
		}
		if len(executor.executedSteps) != 1 {
			t.Errorf("expected 1 step executed, got %d", len(executor.executedSteps))
		}
		if executor.executedSteps[0].Name != "skip" {
			t.Errorf("expected 'skip' step, got %s", executor.executedSteps[0].Name)
		}
		if result.Outputs["branch"] != "else" {
			t.Errorf("expected branch 'else', got %v", result.Outputs["branch"])
		}
	})

	t.Run("execute_no_else_branch", func(t *testing.T) {
		executor := &mockStepExecutor{}
		h := NewIfHandler(executor)

		varCtx := newMockVarContextWithCondition()
		varCtx.conditionResults[`{{ eq .inputs.env "prod" }}`] = false

		step := &runbook.Step{
			Name: "test-if",
			Type: runbook.StepTypeIf,
			Config: map[string]interface{}{
				"condition": `{{ eq .inputs.env "prod" }}`,
				"then": []interface{}{
					map[string]interface{}{
						"name":   "deploy",
						"type":   "command",
						"config": map[string]interface{}{"command": "deploy.sh"},
					},
				},
			},
		}

		result, err := h.Execute(context.Background(), step, varCtx)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Errorf("expected success, got failure: %s", result.Message)
		}
		if len(executor.executedSteps) != 0 {
			t.Errorf("expected 0 steps executed, got %d", len(executor.executedSteps))
		}
		if result.Outputs["branch"] != "none" {
			t.Errorf("expected branch 'none', got %v", result.Outputs["branch"])
		}
	})
}

func TestSwitchHandler(t *testing.T) {
	t.Run("validate_without_value", func(t *testing.T) {
		h := NewSwitchHandler(nil)

		step := &runbook.Step{
			Name:   "test-switch",
			Type:   runbook.StepTypeSwitch,
			Config: map[string]interface{}{},
		}

		err := h.Validate(step)
		if err == nil {
			t.Error("expected error for missing value")
		}
	})

	t.Run("validate_without_cases", func(t *testing.T) {
		h := NewSwitchHandler(nil)

		step := &runbook.Step{
			Name: "test-switch",
			Type: runbook.StepTypeSwitch,
			Config: map[string]interface{}{
				"value": "{{ .inputs.env }}",
			},
		}

		err := h.Validate(step)
		if err == nil {
			t.Error("expected error for missing cases")
		}
	})

	t.Run("validate_with_value_and_cases", func(t *testing.T) {
		h := NewSwitchHandler(nil)

		step := &runbook.Step{
			Name: "test-switch",
			Type: runbook.StepTypeSwitch,
			Config: map[string]interface{}{
				"value": "{{ .inputs.env }}",
				"cases": map[string]interface{}{
					"dev":  []interface{}{},
					"prod": []interface{}{},
				},
			},
		}

		err := h.Validate(step)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("execute_matching_case", func(t *testing.T) {
		executor := &mockStepExecutor{}
		h := NewSwitchHandler(executor)

		varCtx := newMockVarContextWithCondition()
		varCtx.resolveResult = "prod"

		step := &runbook.Step{
			Name: "test-switch",
			Type: runbook.StepTypeSwitch,
			Config: map[string]interface{}{
				"value": "{{ .inputs.env }}",
				"cases": map[string]interface{}{
					"dev": []interface{}{
						map[string]interface{}{
							"name":   "dev-deploy",
							"type":   "noop",
							"config": map[string]interface{}{},
						},
					},
					"prod": []interface{}{
						map[string]interface{}{
							"name":   "prod-deploy",
							"type":   "command",
							"config": map[string]interface{}{"command": "deploy-prod.sh"},
						},
					},
				},
			},
		}

		result, err := h.Execute(context.Background(), step, varCtx)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Errorf("expected success, got failure: %s", result.Message)
		}
		if len(executor.executedSteps) != 1 {
			t.Errorf("expected 1 step executed, got %d", len(executor.executedSteps))
		}
		if executor.executedSteps[0].Name != "prod-deploy" {
			t.Errorf("expected 'prod-deploy' step, got %s", executor.executedSteps[0].Name)
		}
		if result.Outputs["matched_case"] != "prod" {
			t.Errorf("expected matched_case 'prod', got %v", result.Outputs["matched_case"])
		}
	})

	t.Run("execute_default_case", func(t *testing.T) {
		executor := &mockStepExecutor{}
		h := NewSwitchHandler(executor)

		varCtx := newMockVarContextWithCondition()
		varCtx.resolveResult = "staging"

		step := &runbook.Step{
			Name: "test-switch",
			Type: runbook.StepTypeSwitch,
			Config: map[string]interface{}{
				"value": "{{ .inputs.env }}",
				"cases": map[string]interface{}{
					"dev": []interface{}{
						map[string]interface{}{
							"name":   "dev-deploy",
							"type":   "noop",
							"config": map[string]interface{}{},
						},
					},
					"prod": []interface{}{
						map[string]interface{}{
							"name":   "prod-deploy",
							"type":   "noop",
							"config": map[string]interface{}{},
						},
					},
				},
				"default": []interface{}{
					map[string]interface{}{
						"name":   "default-deploy",
						"type":   "noop",
						"config": map[string]interface{}{},
					},
				},
			},
		}

		result, err := h.Execute(context.Background(), step, varCtx)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Errorf("expected success, got failure: %s", result.Message)
		}
		if len(executor.executedSteps) != 1 {
			t.Errorf("expected 1 step executed, got %d", len(executor.executedSteps))
		}
		if executor.executedSteps[0].Name != "default-deploy" {
			t.Errorf("expected 'default-deploy' step, got %s", executor.executedSteps[0].Name)
		}
		if result.Outputs["matched_case"] != "default" {
			t.Errorf("expected matched_case 'default', got %v", result.Outputs["matched_case"])
		}
	})

	t.Run("execute_no_match_no_default", func(t *testing.T) {
		executor := &mockStepExecutor{}
		h := NewSwitchHandler(executor)

		varCtx := newMockVarContextWithCondition()
		varCtx.resolveResult = "staging"

		step := &runbook.Step{
			Name: "test-switch",
			Type: runbook.StepTypeSwitch,
			Config: map[string]interface{}{
				"value": "{{ .inputs.env }}",
				"cases": map[string]interface{}{
					"dev": []interface{}{
						map[string]interface{}{
							"name":   "dev-deploy",
							"type":   "noop",
							"config": map[string]interface{}{},
						},
					},
					"prod": []interface{}{
						map[string]interface{}{
							"name":   "prod-deploy",
							"type":   "noop",
							"config": map[string]interface{}{},
						},
					},
				},
			},
		}

		result, err := h.Execute(context.Background(), step, varCtx)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Errorf("expected success, got failure: %s", result.Message)
		}
		if len(executor.executedSteps) != 0 {
			t.Errorf("expected 0 steps executed, got %d", len(executor.executedSteps))
		}
		if result.Outputs["matched_case"] != "none" {
			t.Errorf("expected matched_case 'none', got %v", result.Outputs["matched_case"])
		}
	})
}

func TestParseStepList(t *testing.T) {
	t.Run("valid_step_list", func(t *testing.T) {
		input := []interface{}{
			map[string]interface{}{
				"name":   "step1",
				"type":   "noop",
				"config": map[string]interface{}{},
			},
			map[string]interface{}{
				"name":        "step2",
				"type":        "command",
				"description": "Test step",
				"timeout":     "30s",
				"config":      map[string]interface{}{"command": "echo hello"},
			},
		}

		steps, err := parseStepList(input)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(steps) != 2 {
			t.Errorf("expected 2 steps, got %d", len(steps))
		}
		if steps[0].Name != "step1" {
			t.Errorf("expected step1, got %s", steps[0].Name)
		}
		if steps[1].Description != "Test step" {
			t.Errorf("expected description 'Test step', got %s", steps[1].Description)
		}
	})

	t.Run("step_with_dependencies", func(t *testing.T) {
		input := []interface{}{
			map[string]interface{}{
				"name":      "step2",
				"type":      "noop",
				"dependsOn": []interface{}{"step1"},
				"config":    map[string]interface{}{},
			},
		}

		steps, err := parseStepList(input)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(steps[0].DependsOn) != 1 {
			t.Errorf("expected 1 dependency, got %d", len(steps[0].DependsOn))
		}
		if steps[0].DependsOn[0] != "step1" {
			t.Errorf("expected dependency 'step1', got %s", steps[0].DependsOn[0])
		}
	})

	t.Run("step_with_outputs", func(t *testing.T) {
		input := []interface{}{
			map[string]interface{}{
				"name":   "step1",
				"type":   "command",
				"config": map[string]interface{}{"command": "echo hello"},
				"outputs": []interface{}{
					map[string]interface{}{
						"name":   "result",
						"source": "stdout",
						"parser": "line",
						"path":   "first",
					},
				},
			},
		}

		steps, err := parseStepList(input)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(steps[0].Outputs) != 1 {
			t.Errorf("expected 1 output, got %d", len(steps[0].Outputs))
		}
		if steps[0].Outputs[0].Name != "result" {
			t.Errorf("expected output name 'result', got %s", steps[0].Outputs[0].Name)
		}
	})

	t.Run("step_with_retries", func(t *testing.T) {
		input := []interface{}{
			map[string]interface{}{
				"name":   "step1",
				"type":   "command",
				"config": map[string]interface{}{"command": "echo hello"},
				"retries": map[string]interface{}{
					"maxAttempts": 3,
					"delay":       "5s",
					"backoff":     "exponential",
				},
			},
		}

		steps, err := parseStepList(input)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if steps[0].Retries == nil {
			t.Error("expected retries config")
		}
		if steps[0].Retries.MaxAttempts != 3 {
			t.Errorf("expected maxAttempts 3, got %d", steps[0].Retries.MaxAttempts)
		}
	})

	t.Run("invalid_step_list_not_list", func(t *testing.T) {
		input := "not a list"

		_, err := parseStepList(input)
		if err == nil {
			t.Error("expected error for non-list input")
		}
	})

	t.Run("invalid_step_missing_name", func(t *testing.T) {
		input := []interface{}{
			map[string]interface{}{
				"type":   "noop",
				"config": map[string]interface{}{},
			},
		}

		_, err := parseStepList(input)
		if err == nil {
			t.Error("expected error for missing name")
		}
	})

	t.Run("invalid_step_missing_type", func(t *testing.T) {
		input := []interface{}{
			map[string]interface{}{
				"name":   "step1",
				"config": map[string]interface{}{},
			},
		}

		_, err := parseStepList(input)
		if err == nil {
			t.Error("expected error for missing type")
		}
	})
}
