// SPDX-License-Identifier: Apache-2.0

package runbook

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func testExecutor(t *testing.T) (*Executor, *Registry) {
	t.Helper()
	reg := NewRegistry()
	var n int
	return &Executor{
		Registry:    reg,
		Clock:       func() time.Time { return time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC) },
		Sleep:       func(context.Context, time.Duration) error { return nil }, // no real waits
		BackoffBase: time.Millisecond,
		NewID:       func() string { n++; return fmt.Sprintf("exec-%d", n) },
	}, reg
}

func mustRegister(t *testing.T, reg *Registry, typ string, f StepFunc) {
	t.Helper()
	if err := reg.Register(typ, f); err != nil {
		t.Fatal(err)
	}
}

func TestExecute_HappyPathWithTemplating(t *testing.T) {
	e, reg := testExecutor(t)
	mustRegister(t, reg, "produce", func(_ context.Context, _ StepContext) (StepOutput, error) {
		return StepOutput{Outputs: map[string]any{"pid": 42}}, nil
	})
	var sawPID any
	mustRegister(t, reg, "consume", func(_ context.Context, sc StepContext) (StepOutput, error) {
		sawPID = sc.Config["from"]
		return StepOutput{}, nil
	})

	rb := &Runbook{
		Metadata: Metadata{Name: "rb"},
		Spec: Spec{Steps: []Step{
			{Type: "produce", Name: "produce"},
			{Type: "consume", Name: "consume", DependsOn: []string{"produce"},
				Config: map[string]any{"from": "{{ .steps.produce.outputs.pid }}"}},
		}},
	}
	exec, err := e.Execute(context.Background(), rb, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if exec.Status != StatusSucceeded {
		t.Fatalf("status=%s", exec.Status)
	}
	if sawPID != "42" {
		t.Fatalf("templated config not threaded: %v", sawPID)
	}
	if exec.ID != "exec-1" || len(exec.Trail) == 0 {
		t.Fatalf("exec id/trail wrong: %+v", exec)
	}
}

func TestExecute_DAGOrder(t *testing.T) {
	e, reg := testExecutor(t)
	var mu sync.Mutex
	var order []string
	rec := func(name string) StepFunc {
		return func(context.Context, StepContext) (StepOutput, error) {
			mu.Lock()
			order = append(order, name)
			mu.Unlock()
			return StepOutput{}, nil
		}
	}
	mustRegister(t, reg, "a", rec("a"))
	mustRegister(t, reg, "b", rec("b"))
	mustRegister(t, reg, "c", rec("c"))
	rb := &Runbook{Metadata: Metadata{Name: "rb"}, Spec: Spec{Steps: []Step{
		{Type: "c", Name: "c", DependsOn: []string{"b"}},
		{Type: "b", Name: "b", DependsOn: []string{"a"}},
		{Type: "a", Name: "a"},
	}}}
	if _, err := e.Execute(context.Background(), rb, nil); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(order) != "[a b c]" {
		t.Fatalf("order=%v", order)
	}
}

func TestExecute_ConditionalSkip(t *testing.T) {
	e, reg := testExecutor(t)
	ran := false
	mustRegister(t, reg, "x", func(context.Context, StepContext) (StepOutput, error) {
		ran = true
		return StepOutput{}, nil
	})
	rb := &Runbook{Metadata: Metadata{Name: "rb"}, Spec: Spec{
		Inputs: []InputSpec{{Name: "go"}},
		Steps:  []Step{{Type: "x", Name: "x", Condition: "{{ .inputs.go }}"}},
	}}
	exec, err := e.Execute(context.Background(), rb, map[string]any{"go": "false"})
	if err != nil {
		t.Fatal(err)
	}
	if ran {
		t.Fatal("step ran despite falsey condition")
	}
	if exec.Steps[0].Status != StatusSkipped {
		t.Fatalf("status=%s want skipped", exec.Steps[0].Status)
	}
}

func TestExecute_RetryThenSucceed(t *testing.T) {
	e, reg := testExecutor(t)
	calls := 0
	mustRegister(t, reg, "flaky", func(context.Context, StepContext) (StepOutput, error) {
		calls++
		if calls < 3 {
			return StepOutput{}, errors.New("transient")
		}
		return StepOutput{}, nil
	})
	rb := &Runbook{Metadata: Metadata{Name: "rb"}, Spec: Spec{Steps: []Step{
		{Type: "flaky", Name: "flaky", Retries: 5},
	}}}
	exec, err := e.Execute(context.Background(), rb, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if calls != 3 || exec.Steps[0].Attempts != 3 || exec.Steps[0].Status != StatusSucceeded {
		t.Fatalf("calls=%d attempts=%d status=%s", calls, exec.Steps[0].Attempts, exec.Steps[0].Status)
	}
}

func TestExecute_RetryExhaustedFails(t *testing.T) {
	e, reg := testExecutor(t)
	sentinel := errors.New("always")
	mustRegister(t, reg, "bad", func(context.Context, StepContext) (StepOutput, error) {
		return StepOutput{}, sentinel
	})
	rb := &Runbook{Metadata: Metadata{Name: "rb"}, Spec: Spec{
		MaxRetries: 2,
		Steps:      []Step{{Type: "bad", Name: "bad"}},
	}}
	exec, err := e.Execute(context.Background(), rb, nil)
	if !errors.Is(err, ErrExecutionFailed) || !errors.Is(err, sentinel) {
		t.Fatalf("err=%v want wraps ErrExecutionFailed+sentinel", err)
	}
	if exec.Status != StatusFailed || exec.Steps[0].Attempts != 3 {
		t.Fatalf("status=%s attempts=%d", exec.Status, exec.Steps[0].Attempts)
	}
}

func TestExecute_OnFailureChain(t *testing.T) {
	e, reg := testExecutor(t)
	cleaned := false
	mustRegister(t, reg, "boom", func(context.Context, StepContext) (StepOutput, error) {
		return StepOutput{}, errors.New("kaboom")
	})
	mustRegister(t, reg, "cleanup", func(context.Context, StepContext) (StepOutput, error) {
		cleaned = true
		return StepOutput{}, nil
	})
	rb := &Runbook{Metadata: Metadata{Name: "rb"}, Spec: Spec{
		OnFailure: []string{"cleanup"},
		Steps: []Step{
			{Type: "boom", Name: "boom"},
			{Type: "cleanup", Name: "cleanup"},
		},
	}}
	exec, err := e.Execute(context.Background(), rb, nil)
	if !errors.Is(err, ErrExecutionFailed) {
		t.Fatalf("err=%v", err)
	}
	if !cleaned {
		t.Fatal("onFailure chain did not run")
	}
	if exec.Status != StatusFailed {
		t.Fatalf("status=%s", exec.Status)
	}
}

func TestExecute_OnSuccessChain(t *testing.T) {
	e, reg := testExecutor(t)
	notified := false
	mustRegister(t, reg, "work", func(context.Context, StepContext) (StepOutput, error) { return StepOutput{}, nil })
	mustRegister(t, reg, "notify", func(context.Context, StepContext) (StepOutput, error) {
		notified = true
		return StepOutput{}, nil
	})
	rb := &Runbook{Metadata: Metadata{Name: "rb"}, Spec: Spec{
		OnSuccess: []string{"notify"},
		Steps:     []Step{{Type: "work", Name: "work"}, {Type: "notify", Name: "notify"}},
	}}
	if _, err := e.Execute(context.Background(), rb, nil); err != nil {
		t.Fatal(err)
	}
	if !notified {
		t.Fatal("onSuccess chain did not run")
	}
}

func TestExecute_UnknownStepType(t *testing.T) {
	e, _ := testExecutor(t)
	rb := &Runbook{Metadata: Metadata{Name: "rb"}, Spec: Spec{Steps: []Step{{Type: "ghost", Name: "g"}}}}
	exec, err := e.Execute(context.Background(), rb, nil)
	if !errors.Is(err, ErrExecutionFailed) || !errors.Is(exec.Steps[0].Error, ErrUnknownStepType) {
		t.Fatalf("err=%v stepErr=%v", err, exec.Steps[0].Error)
	}
}

func TestExecute_InputResolution(t *testing.T) {
	e, reg := testExecutor(t)
	mustRegister(t, reg, "n", func(context.Context, StepContext) (StepOutput, error) { return StepOutput{}, nil })
	base := func() *Runbook {
		return &Runbook{Metadata: Metadata{Name: "rb"}, Spec: Spec{
			Inputs: []InputSpec{{Name: "a", Required: true}, {Name: "b", Default: "def"}},
			Steps:  []Step{{Type: "n", Name: "n"}},
		}}
	}

	t.Run("unknown input", func(t *testing.T) {
		_, err := e.Execute(context.Background(), base(), map[string]any{"a": 1, "x": 2})
		if !errors.Is(err, ErrUnknownInput) {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("missing required", func(t *testing.T) {
		_, err := e.Execute(context.Background(), base(), nil)
		if !errors.Is(err, ErrMissingInput) {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("default applied", func(t *testing.T) {
		exec, err := e.Execute(context.Background(), base(), map[string]any{"a": 1})
		if err != nil {
			t.Fatal(err)
		}
		if exec.Inputs["b"] != "def" {
			t.Fatalf("default not applied: %v", exec.Inputs)
		}
	})
}

func TestExecute_CycleDetected(t *testing.T) {
	e, reg := testExecutor(t)
	mustRegister(t, reg, "n", func(context.Context, StepContext) (StepOutput, error) { return StepOutput{}, nil })
	rb := &Runbook{Metadata: Metadata{Name: "rb"}, Spec: Spec{Steps: []Step{
		{Type: "n", Name: "a", DependsOn: []string{"b"}},
		{Type: "n", Name: "b", DependsOn: []string{"a"}},
	}}}
	_, err := e.Execute(context.Background(), rb, nil)
	if !errors.Is(err, ErrStepCycle) {
		t.Fatalf("err=%v want ErrStepCycle", err)
	}
}

func TestExecute_PerStepTimeout(t *testing.T) {
	e, reg := testExecutor(t)
	mustRegister(t, reg, "block", func(ctx context.Context, _ StepContext) (StepOutput, error) {
		<-ctx.Done()
		return StepOutput{}, ctx.Err()
	})
	rb := &Runbook{Metadata: Metadata{Name: "rb"}, Spec: Spec{Steps: []Step{
		{Type: "block", Name: "block", Timeout: "20ms"},
	}}}
	exec, err := e.Execute(context.Background(), rb, nil)
	if !errors.Is(err, ErrExecutionFailed) {
		t.Fatalf("err=%v", err)
	}
	if !errors.Is(exec.Steps[0].Error, context.DeadlineExceeded) {
		t.Fatalf("step error=%v want DeadlineExceeded", exec.Steps[0].Error)
	}
}

func TestRegistry_RegisterErrors(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Register("", StepFunc(nil)); err == nil {
		t.Error("empty type should error")
	}
	if err := reg.Register("t", nil); err == nil {
		t.Error("nil executor should error")
	}
	if err := reg.Register("t", StepFunc(func(context.Context, StepContext) (StepOutput, error) {
		return StepOutput{}, nil
	})); err != nil {
		t.Fatal(err)
	}
	if _, ok := reg.Lookup("t"); !ok {
		t.Error("lookup after register failed")
	}
}
