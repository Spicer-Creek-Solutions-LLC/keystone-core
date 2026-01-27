package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// mockRunbookLoader implements RunbookLoader for testing.
type mockRunbookLoader struct {
	runbooks map[string]*runbook.Runbook
	loadErr  error
}

func (m *mockRunbookLoader) Load(ctx context.Context, name, version string) (*runbook.Runbook, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	if rb, ok := m.runbooks[name]; ok {
		return rb, nil
	}
	return nil, errors.New("runbook not found")
}

// mockRunbookExecutor implements RunbookExecutor for testing.
type mockRunbookExecutor struct {
	execution *runbook.Execution
	execErr   error
}

func (m *mockRunbookExecutor) Execute(ctx context.Context, rb *runbook.Runbook, inputs map[string]interface{}) (*runbook.Execution, error) {
	if m.execErr != nil {
		return nil, m.execErr
	}
	return m.execution, nil
}

func TestSubRunbookHandler(t *testing.T) {
	t.Run("validate_without_runbook", func(t *testing.T) {
		h := NewSubRunbookHandler(nil, nil)

		step := &runbook.Step{
			Name:   "test-subrunbook",
			Type:   runbook.StepTypeSubRunbook,
			Config: map[string]interface{}{},
		}

		err := h.Validate(step)
		if err == nil {
			t.Error("expected error for missing runbook")
		}
	})

	t.Run("validate_with_runbook", func(t *testing.T) {
		h := NewSubRunbookHandler(nil, nil)

		step := &runbook.Step{
			Name: "test-subrunbook",
			Type: runbook.StepTypeSubRunbook,
			Config: map[string]interface{}{
				"runbook": "child-runbook",
			},
		}

		err := h.Validate(step)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("execute_without_loader", func(t *testing.T) {
		h := NewSubRunbookHandler(nil, nil)

		varCtx := newMockVarContextWithCondition()

		step := &runbook.Step{
			Name: "test-subrunbook",
			Type: runbook.StepTypeSubRunbook,
			Config: map[string]interface{}{
				"runbook": "child-runbook",
			},
		}

		result, err := h.Execute(context.Background(), step, varCtx)

		if err == nil {
			t.Error("expected error for missing loader")
		}
		if result.Success {
			t.Error("expected failure")
		}
	})

	t.Run("execute_runbook_not_found", func(t *testing.T) {
		loader := &mockRunbookLoader{
			runbooks: make(map[string]*runbook.Runbook),
		}
		h := NewSubRunbookHandler(loader, nil)

		varCtx := newMockVarContextWithCondition()

		step := &runbook.Step{
			Name: "test-subrunbook",
			Type: runbook.StepTypeSubRunbook,
			Config: map[string]interface{}{
				"runbook": "nonexistent",
			},
		}

		result, err := h.Execute(context.Background(), step, varCtx)

		if err == nil {
			t.Error("expected error for runbook not found")
		}
		if result.Success {
			t.Error("expected failure")
		}
	})

	t.Run("execute_without_executor", func(t *testing.T) {
		loader := &mockRunbookLoader{
			runbooks: map[string]*runbook.Runbook{
				"child-runbook": {
					Metadata: runbook.Metadata{Name: "child-runbook", Version: "1.0.0"},
					Spec:     runbook.RunbookSpec{},
				},
			},
		}
		h := NewSubRunbookHandler(loader, nil)

		varCtx := newMockVarContextWithCondition()

		step := &runbook.Step{
			Name: "test-subrunbook",
			Type: runbook.StepTypeSubRunbook,
			Config: map[string]interface{}{
				"runbook": "child-runbook",
			},
		}

		result, err := h.Execute(context.Background(), step, varCtx)

		if err == nil {
			t.Error("expected error for missing executor")
		}
		if result.Success {
			t.Error("expected failure")
		}
	})

	t.Run("execute_successful", func(t *testing.T) {
		loader := &mockRunbookLoader{
			runbooks: map[string]*runbook.Runbook{
				"child-runbook": {
					Metadata: runbook.Metadata{Name: "child-runbook", Version: "1.0.0"},
					Spec:     runbook.RunbookSpec{},
				},
			},
		}
		executor := &mockRunbookExecutor{
			execution: &runbook.Execution{
				ID:    "child-exec-123",
				State: runbook.ExecutionStateCompleted,
				Outputs: map[string]interface{}{
					"result": "success",
				},
			},
		}
		h := NewSubRunbookHandler(loader, executor)

		varCtx := newMockVarContextWithCondition()

		step := &runbook.Step{
			Name: "test-subrunbook",
			Type: runbook.StepTypeSubRunbook,
			Config: map[string]interface{}{
				"runbook": "child-runbook",
			},
		}

		result, err := h.Execute(context.Background(), step, varCtx)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Errorf("expected success, got failure: %s", result.Message)
		}
		if result.Outputs["execution_id"] != "child-exec-123" {
			t.Errorf("expected execution_id 'child-exec-123', got %v", result.Outputs["execution_id"])
		}
		if result.Outputs["result"] != "success" {
			t.Errorf("expected result 'success', got %v", result.Outputs["result"])
		}
	})

	t.Run("execute_with_inputs", func(t *testing.T) {
		var capturedInputs map[string]interface{}

		loader := &mockRunbookLoader{
			runbooks: map[string]*runbook.Runbook{
				"child-runbook": {
					Metadata: runbook.Metadata{Name: "child-runbook", Version: "1.0.0"},
					Spec:     runbook.RunbookSpec{},
				},
			},
		}
		executor := &mockRunbookExecutor{
			execution: &runbook.Execution{
				ID:    "child-exec-123",
				State: runbook.ExecutionStateCompleted,
			},
		}

		// Wrap executor to capture inputs
		originalExecute := executor.Execute
		executor.execErr = nil
		capturedInputs = make(map[string]interface{})

		h := &SubRunbookHandler{
			runbookLoader: loader,
			runbookExecutor: &inputCapturingExecutor{
				wrapped:   executor,
				captured:  &capturedInputs,
				original:  originalExecute,
			},
		}

		varCtx := newMockVarContextWithCondition()

		step := &runbook.Step{
			Name: "test-subrunbook",
			Type: runbook.StepTypeSubRunbook,
			Config: map[string]interface{}{
				"runbook": "child-runbook",
				"inputs": map[string]interface{}{
					"server": "prod-01",
					"port":   8080,
				},
			},
		}

		_, _ = h.Execute(context.Background(), step, varCtx)

		if capturedInputs["server"] != "prod-01" {
			t.Errorf("expected server 'prod-01', got %v", capturedInputs["server"])
		}
		if capturedInputs["port"] != 8080 {
			t.Errorf("expected port 8080, got %v", capturedInputs["port"])
		}
	})

	t.Run("execute_with_output_mapping", func(t *testing.T) {
		loader := &mockRunbookLoader{
			runbooks: map[string]*runbook.Runbook{
				"child-runbook": {
					Metadata: runbook.Metadata{Name: "child-runbook", Version: "1.0.0"},
					Spec:     runbook.RunbookSpec{},
				},
			},
		}
		executor := &mockRunbookExecutor{
			execution: &runbook.Execution{
				ID:    "child-exec-123",
				State: runbook.ExecutionStateCompleted,
				Outputs: map[string]interface{}{
					"internal_result": "value1",
					"internal_status": "done",
				},
			},
		}
		h := NewSubRunbookHandler(loader, executor)

		varCtx := newMockVarContextWithCondition()

		step := &runbook.Step{
			Name: "test-subrunbook",
			Type: runbook.StepTypeSubRunbook,
			Config: map[string]interface{}{
				"runbook": "child-runbook",
				"outputMapping": map[string]interface{}{
					"result": "internal_result",
					"status": "internal_status",
				},
			},
		}

		result, err := h.Execute(context.Background(), step, varCtx)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result.Outputs["result"] != "value1" {
			t.Errorf("expected result 'value1', got %v", result.Outputs["result"])
		}
		if result.Outputs["status"] != "done" {
			t.Errorf("expected status 'done', got %v", result.Outputs["status"])
		}
	})

	t.Run("execute_failed_runbook", func(t *testing.T) {
		loader := &mockRunbookLoader{
			runbooks: map[string]*runbook.Runbook{
				"child-runbook": {
					Metadata: runbook.Metadata{Name: "child-runbook", Version: "1.0.0"},
					Spec:     runbook.RunbookSpec{},
				},
			},
		}
		executor := &mockRunbookExecutor{
			execution: &runbook.Execution{
				ID:    "child-exec-123",
				State: runbook.ExecutionStateFailed,
				Error: "something went wrong",
			},
		}
		h := NewSubRunbookHandler(loader, executor)

		varCtx := newMockVarContextWithCondition()

		step := &runbook.Step{
			Name: "test-subrunbook",
			Type: runbook.StepTypeSubRunbook,
			Config: map[string]interface{}{
				"runbook": "child-runbook",
			},
		}

		result, err := h.Execute(context.Background(), step, varCtx)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result.Success {
			t.Error("expected failure for failed sub-runbook")
		}
		if result.Outputs["state"] != "failed" {
			t.Errorf("expected state 'failed', got %v", result.Outputs["state"])
		}
	})

	t.Run("recursion_depth_exceeded", func(t *testing.T) {
		loader := &mockRunbookLoader{
			runbooks: map[string]*runbook.Runbook{
				"child-runbook": {
					Metadata: runbook.Metadata{Name: "child-runbook", Version: "1.0.0"},
					Spec:     runbook.RunbookSpec{},
				},
			},
		}
		h := NewSubRunbookHandler(loader, nil)

		varCtx := newMockVarContextWithCondition()
		varCtx.inputs["_recursion_depth"] = 15 // Exceeds maxRecursionDepth (10)

		step := &runbook.Step{
			Name: "test-subrunbook",
			Type: runbook.StepTypeSubRunbook,
			Config: map[string]interface{}{
				"runbook": "child-runbook",
			},
		}

		result, err := h.Execute(context.Background(), step, varCtx)

		if err == nil {
			t.Error("expected error for recursion depth exceeded")
		}
		if result.Success {
			t.Error("expected failure")
		}
	})
}

// inputCapturingExecutor wraps a RunbookExecutor to capture inputs.
type inputCapturingExecutor struct {
	wrapped  *mockRunbookExecutor
	captured *map[string]interface{}
	original func(ctx context.Context, rb *runbook.Runbook, inputs map[string]interface{}) (*runbook.Execution, error)
}

func (e *inputCapturingExecutor) Execute(ctx context.Context, rb *runbook.Runbook, inputs map[string]interface{}) (*runbook.Execution, error) {
	for k, v := range inputs {
		(*e.captured)[k] = v
	}
	return e.wrapped.execution, e.wrapped.execErr
}

func TestGetRecursionDepth(t *testing.T) {
	t.Run("no_depth", func(t *testing.T) {
		varCtx := newMockVarContextWithCondition()
		depth := getRecursionDepth(varCtx)
		if depth != 0 {
			t.Errorf("expected 0, got %d", depth)
		}
	})

	t.Run("int_depth", func(t *testing.T) {
		varCtx := newMockVarContextWithCondition()
		varCtx.inputs["_recursion_depth"] = 5
		depth := getRecursionDepth(varCtx)
		if depth != 5 {
			t.Errorf("expected 5, got %d", depth)
		}
	})

	t.Run("float64_depth", func(t *testing.T) {
		varCtx := newMockVarContextWithCondition()
		varCtx.inputs["_recursion_depth"] = 3.0
		depth := getRecursionDepth(varCtx)
		if depth != 3 {
			t.Errorf("expected 3, got %d", depth)
		}
	})
}
