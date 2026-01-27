package execution

import (
	"context"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

func TestExecutor_Execute_SimpleRunbook(t *testing.T) {
	rb := &runbook.Runbook{
		APIVersion: runbook.APIVersion,
		Kind:       runbook.Kind,
		Metadata: runbook.Metadata{
			Name: "simple-test",
		},
		Spec: runbook.RunbookSpec{
			Steps: []runbook.Step{
				{
					Name:   "step1",
					Type:   runbook.StepTypeNoop,
					Config: map[string]interface{}{},
				},
				{
					Name:   "step2",
					Type:   runbook.StepTypeNoop,
					Config: map[string]interface{}{},
				},
			},
		},
	}

	executor := NewExecutor()
	result, err := executor.Execute(context.Background(), rb, nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if result.State != runbook.ExecutionStateCompleted {
		t.Errorf("State = %v, want %v", result.State, runbook.ExecutionStateCompleted)
	}

	if len(result.Steps) != 2 {
		t.Errorf("len(Steps) = %d, want 2", len(result.Steps))
	}

	for name, step := range result.Steps {
		if step.State != runbook.StepStateCompleted {
			t.Errorf("step %s State = %v, want %v", name, step.State, runbook.StepStateCompleted)
		}
	}
}

func TestExecutor_Execute_WithDependencies(t *testing.T) {
	rb := &runbook.Runbook{
		APIVersion: runbook.APIVersion,
		Kind:       runbook.Kind,
		Metadata: runbook.Metadata{
			Name: "dep-test",
		},
		Spec: runbook.RunbookSpec{
			Steps: []runbook.Step{
				{
					Name:   "first",
					Type:   runbook.StepTypeNoop,
					Config: map[string]interface{}{},
				},
				{
					Name:      "second",
					Type:      runbook.StepTypeNoop,
					DependsOn: []string{"first"},
					Config:    map[string]interface{}{},
				},
				{
					Name:      "third",
					Type:      runbook.StepTypeNoop,
					DependsOn: []string{"first", "second"},
					Config:    map[string]interface{}{},
				},
			},
		},
	}

	executor := NewExecutor()
	result, err := executor.Execute(context.Background(), rb, nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if result.State != runbook.ExecutionStateCompleted {
		t.Errorf("State = %v, want %v", result.State, runbook.ExecutionStateCompleted)
	}
}

func TestExecutor_Execute_FailStep(t *testing.T) {
	rb := &runbook.Runbook{
		APIVersion: runbook.APIVersion,
		Kind:       runbook.Kind,
		Metadata: runbook.Metadata{
			Name: "fail-test",
		},
		Spec: runbook.RunbookSpec{
			Steps: []runbook.Step{
				{
					Name: "fail_step",
					Type: runbook.StepTypeFail,
					Config: map[string]interface{}{
						"message": "intentional failure",
					},
				},
			},
		},
	}

	executor := NewExecutor()
	result, err := executor.Execute(context.Background(), rb, nil)
	if err == nil {
		t.Fatal("Execute() expected error, got nil")
	}

	if result == nil {
		t.Fatalf("Execute() returned nil result with error: %v", err)
	}

	if result.State != runbook.ExecutionStateFailed {
		t.Errorf("State = %v, want %v", result.State, runbook.ExecutionStateFailed)
	}

	if result.Error == "" {
		t.Error("Error should not be empty")
	}

	if step, ok := result.Steps["fail_step"]; ok {
		if step.State != runbook.StepStateFailed {
			t.Errorf("fail_step State = %v, want %v", step.State, runbook.StepStateFailed)
		}
	} else {
		t.Error("fail_step not found in results")
	}
}

func TestExecutor_Execute_ContinueOnError(t *testing.T) {
	rb := &runbook.Runbook{
		APIVersion: runbook.APIVersion,
		Kind:       runbook.Kind,
		Metadata: runbook.Metadata{
			Name: "continue-test",
		},
		Spec: runbook.RunbookSpec{
			Steps: []runbook.Step{
				{
					Name:            "fail_step",
					Type:            runbook.StepTypeFail,
					ContinueOnError: true,
					Config: map[string]interface{}{
						"message": "expected failure",
					},
				},
				{
					Name:      "after_fail",
					Type:      runbook.StepTypeNoop,
					DependsOn: []string{"fail_step"},
					Config:    map[string]interface{}{},
				},
			},
		},
	}

	executor := NewExecutor()
	result, err := executor.Execute(context.Background(), rb, nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if result.State != runbook.ExecutionStateCompleted {
		t.Errorf("State = %v, want %v", result.State, runbook.ExecutionStateCompleted)
	}

	// Both steps should have executed
	if step, ok := result.Steps["fail_step"]; ok {
		if step.State != runbook.StepStateFailed {
			t.Errorf("fail_step State = %v, want %v", step.State, runbook.StepStateFailed)
		}
	}

	if step, ok := result.Steps["after_fail"]; ok {
		if step.State != runbook.StepStateCompleted {
			t.Errorf("after_fail State = %v, want %v", step.State, runbook.StepStateCompleted)
		}
	}
}

func TestExecutor_Execute_WithInputs(t *testing.T) {
	rb := &runbook.Runbook{
		APIVersion: runbook.APIVersion,
		Kind:       runbook.Kind,
		Metadata: runbook.Metadata{
			Name: "input-test",
		},
		Spec: runbook.RunbookSpec{
			Inputs: []runbook.InputDef{
				{Name: "target", Type: runbook.InputTypeString, Required: true},
				{Name: "count", Type: runbook.InputTypeInt, Default: 5},
			},
			Steps: []runbook.Step{
				{
					Name:   "step1",
					Type:   runbook.StepTypeNoop,
					Config: map[string]interface{}{},
				},
			},
		},
	}

	t.Run("missing required input", func(t *testing.T) {
		executor := NewExecutor()
		_, err := executor.Execute(context.Background(), rb, nil)
		if err == nil {
			t.Error("Execute() expected error for missing required input")
		}
	})

	t.Run("with required input", func(t *testing.T) {
		executor := NewExecutor()
		inputs := map[string]interface{}{
			"target": "localhost",
		}
		result, err := executor.Execute(context.Background(), rb, inputs)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		if result.State != runbook.ExecutionStateCompleted {
			t.Errorf("State = %v, want %v", result.State, runbook.ExecutionStateCompleted)
		}
	})
}

func TestExecutor_Execute_Cancellation(t *testing.T) {
	rb := &runbook.Runbook{
		APIVersion: runbook.APIVersion,
		Kind:       runbook.Kind,
		Metadata: runbook.Metadata{
			Name: "cancel-test",
		},
		Spec: runbook.RunbookSpec{
			Steps: []runbook.Step{
				{
					Name: "wait_step",
					Type: runbook.StepTypeWait,
					Config: map[string]interface{}{
						"duration": "10s",
					},
				},
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	executor := NewExecutor()
	result, err := executor.Execute(ctx, rb, nil)
	if err == nil {
		t.Fatal("Execute() expected error, got nil")
	}

	if result.State != runbook.ExecutionStateCancelled {
		t.Errorf("State = %v, want %v", result.State, runbook.ExecutionStateCancelled)
	}
}

func TestExecutor_Execute_OnSuccessHandler(t *testing.T) {
	var onSuccessCalled bool

	rb := &runbook.Runbook{
		APIVersion: runbook.APIVersion,
		Kind:       runbook.Kind,
		Metadata: runbook.Metadata{
			Name: "success-handler-test",
		},
		Spec: runbook.RunbookSpec{
			Steps: []runbook.Step{
				{
					Name:   "main_step",
					Type:   runbook.StepTypeNoop,
					Config: map[string]interface{}{},
				},
			},
			OnSuccess: []runbook.Step{
				{
					Name: "success_handler",
					Type: runbook.StepTypeNoop,
					Config: map[string]interface{}{
						"message": "success",
					},
				},
			},
		},
	}

	executor := NewExecutor(
		WithStepCallbacks(
			nil,
			func(ctx context.Context, exec *ExecutionContext, step *StepContext) {
				if step.Name() == "success_handler" {
					onSuccessCalled = true
				}
			},
		),
	)

	result, err := executor.Execute(context.Background(), rb, nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if result.State != runbook.ExecutionStateCompleted {
		t.Errorf("State = %v, want %v", result.State, runbook.ExecutionStateCompleted)
	}

	if !onSuccessCalled {
		t.Error("onSuccess handler was not called")
	}
}

func TestExecutor_Execute_OnFailureHandler(t *testing.T) {
	var onFailureCalled bool

	rb := &runbook.Runbook{
		APIVersion: runbook.APIVersion,
		Kind:       runbook.Kind,
		Metadata: runbook.Metadata{
			Name: "failure-handler-test",
		},
		Spec: runbook.RunbookSpec{
			Steps: []runbook.Step{
				{
					Name: "fail_step",
					Type: runbook.StepTypeFail,
					Config: map[string]interface{}{
						"message": "intentional failure",
					},
				},
			},
			OnFailure: []runbook.Step{
				{
					Name: "failure_handler",
					Type: runbook.StepTypeNoop,
					Config: map[string]interface{}{
						"message": "handling failure",
					},
				},
			},
		},
	}

	executor := NewExecutor(
		WithStepCallbacks(
			nil,
			func(ctx context.Context, exec *ExecutionContext, step *StepContext) {
				if step.Name() == "failure_handler" {
					onFailureCalled = true
				}
			},
		),
	)

	_, err := executor.Execute(context.Background(), rb, nil)
	if err == nil {
		t.Fatal("Execute() expected error")
	}

	if !onFailureCalled {
		t.Error("onFailure handler was not called")
	}
}

func TestTopologicalSort(t *testing.T) {
	t.Run("simple chain", func(t *testing.T) {
		steps := []runbook.Step{
			{Name: "a"},
			{Name: "b", DependsOn: []string{"a"}},
			{Name: "c", DependsOn: []string{"b"}},
		}

		graph := buildDependencyGraph(steps)
		order, err := topologicalSort(graph)
		if err != nil {
			t.Fatalf("topologicalSort() error = %v", err)
		}

		// a must come before b, b must come before c
		aIndex := indexOf(order, "a")
		bIndex := indexOf(order, "b")
		cIndex := indexOf(order, "c")

		if aIndex > bIndex {
			t.Error("a should come before b")
		}
		if bIndex > cIndex {
			t.Error("b should come before c")
		}
	})

	t.Run("parallel steps", func(t *testing.T) {
		steps := []runbook.Step{
			{Name: "a"},
			{Name: "b"},
			{Name: "c", DependsOn: []string{"a", "b"}},
		}

		graph := buildDependencyGraph(steps)
		order, err := topologicalSort(graph)
		if err != nil {
			t.Fatalf("topologicalSort() error = %v", err)
		}

		// a and b must come before c
		aIndex := indexOf(order, "a")
		bIndex := indexOf(order, "b")
		cIndex := indexOf(order, "c")

		if aIndex > cIndex {
			t.Error("a should come before c")
		}
		if bIndex > cIndex {
			t.Error("b should come before c")
		}
	})

	t.Run("no dependencies", func(t *testing.T) {
		steps := []runbook.Step{
			{Name: "a"},
			{Name: "b"},
			{Name: "c"},
		}

		graph := buildDependencyGraph(steps)
		order, err := topologicalSort(graph)
		if err != nil {
			t.Fatalf("topologicalSort() error = %v", err)
		}

		if len(order) != 3 {
			t.Errorf("len(order) = %d, want 3", len(order))
		}
	})
}

func indexOf(slice []string, item string) int {
	for i, v := range slice {
		if v == item {
			return i
		}
	}
	return -1
}

func TestExecutionContext(t *testing.T) {
	rb := &runbook.Runbook{
		Metadata: runbook.Metadata{
			Name:    "test-rb",
			Version: "1.0.0",
		},
		Spec: runbook.RunbookSpec{
			Steps: []runbook.Step{
				{Name: "step1", Type: runbook.StepTypeNoop, Config: map[string]interface{}{}},
			},
		},
	}

	inputs := map[string]interface{}{
		"target": "localhost",
		"count":  42,
	}

	ctx := NewExecutionContext("exec-123", rb, inputs)

	if ctx.ExecutionID() != "exec-123" {
		t.Errorf("ExecutionID() = %v, want %v", ctx.ExecutionID(), "exec-123")
	}

	if ctx.RunbookName() != "test-rb" {
		t.Errorf("RunbookName() = %v, want %v", ctx.RunbookName(), "test-rb")
	}

	if ctx.RunbookVersion() != "1.0.0" {
		t.Errorf("RunbookVersion() = %v, want %v", ctx.RunbookVersion(), "1.0.0")
	}

	if ctx.State() != runbook.ExecutionStatePending {
		t.Errorf("State() = %v, want %v", ctx.State(), runbook.ExecutionStatePending)
	}

	// Test inputs
	if v, ok := ctx.GetInput("target"); !ok || v != "localhost" {
		t.Errorf("GetInput(target) = %v, %v; want localhost, true", v, ok)
	}

	if v, ok := ctx.GetInput("count"); !ok || v != 42 {
		t.Errorf("GetInput(count) = %v, %v; want 42, true", v, ok)
	}

	// Test step context
	stepCtx, ok := ctx.GetStep("step1")
	if !ok {
		t.Fatal("GetStep(step1) not found")
	}

	if stepCtx.Name() != "step1" {
		t.Errorf("step.Name() = %v, want %v", stepCtx.Name(), "step1")
	}

	// Test state transitions
	if err := ctx.Start(context.Background()); err != nil {
		t.Errorf("Start() error = %v", err)
	}

	if ctx.State() != runbook.ExecutionStateRunning {
		t.Errorf("State() after Start() = %v, want %v", ctx.State(), runbook.ExecutionStateRunning)
	}

	if ctx.StartedAt() == nil {
		t.Error("StartedAt() should not be nil after Start()")
	}
}

func TestStepContext(t *testing.T) {
	step := &runbook.Step{
		Name:   "test-step",
		Type:   runbook.StepTypeCommand,
		Config: map[string]interface{}{"command": "echo hello"},
	}

	stepCtx := NewStepContext(step)

	if stepCtx.Name() != "test-step" {
		t.Errorf("Name() = %v, want %v", stepCtx.Name(), "test-step")
	}

	if stepCtx.State() != runbook.StepStatePending {
		t.Errorf("State() = %v, want %v", stepCtx.State(), runbook.StepStatePending)
	}

	// Test outputs
	stepCtx.SetOutput("result", "success")
	if v, ok := stepCtx.GetOutput("result"); !ok || v != "success" {
		t.Errorf("GetOutput(result) = %v, %v; want success, true", v, ok)
	}

	stepCtx.SetOutputs(map[string]interface{}{
		"code": 0,
		"msg":  "ok",
	})

	outputs := stepCtx.Outputs()
	if len(outputs) != 3 {
		t.Errorf("len(Outputs()) = %d, want 3", len(outputs))
	}

	// Test retry count
	if stepCtx.RetryCount() != 0 {
		t.Errorf("RetryCount() = %d, want 0", stepCtx.RetryCount())
	}

	stepCtx.IncrementRetry()
	if stepCtx.RetryCount() != 1 {
		t.Errorf("RetryCount() after increment = %d, want 1", stepCtx.RetryCount())
	}

	// Test state transitions
	ctx := context.Background()

	if err := stepCtx.Start(ctx); err != nil {
		t.Errorf("Start() error = %v", err)
	}

	if !stepCtx.machine.IsRunning() {
		t.Error("should be running after Start()")
	}

	if err := stepCtx.Complete(ctx); err != nil {
		t.Errorf("Complete() error = %v", err)
	}

	if !stepCtx.IsCompleted() {
		t.Error("should be completed after Complete()")
	}
}

func TestStepContext_ToStepExecution(t *testing.T) {
	step := &runbook.Step{
		Name:   "test-step",
		Type:   runbook.StepTypeNoop,
		Config: map[string]interface{}{},
	}

	stepCtx := NewStepContext(step)
	_ = stepCtx.Start(context.Background())
	stepCtx.SetOutput("result", "done")
	_ = stepCtx.Complete(context.Background())

	exec := stepCtx.ToStepExecution()

	if exec.Name != "test-step" {
		t.Errorf("Name = %v, want %v", exec.Name, "test-step")
	}

	if exec.Type != runbook.StepTypeNoop {
		t.Errorf("Type = %v, want %v", exec.Type, runbook.StepTypeNoop)
	}

	if exec.State != runbook.StepStateCompleted {
		t.Errorf("State = %v, want %v", exec.State, runbook.StepStateCompleted)
	}

	if exec.StartedAt == nil {
		t.Error("StartedAt should not be nil")
	}

	if exec.CompletedAt == nil {
		t.Error("CompletedAt should not be nil")
	}
}
