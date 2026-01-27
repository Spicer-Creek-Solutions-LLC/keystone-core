package execution

import (
	"context"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

func TestExecutionMachine_Transitions(t *testing.T) {
	ctx := context.Background()

	t.Run("pending to running", func(t *testing.T) {
		m := NewExecutionMachine()

		if m.State() != runbook.ExecutionStatePending {
			t.Errorf("initial state = %v, want %v", m.State(), runbook.ExecutionStatePending)
		}

		if err := m.Start(ctx); err != nil {
			t.Errorf("Start() error = %v", err)
		}

		if m.State() != runbook.ExecutionStateRunning {
			t.Errorf("state after Start() = %v, want %v", m.State(), runbook.ExecutionStateRunning)
		}
	})

	t.Run("running to completed", func(t *testing.T) {
		m := NewExecutionMachine()
		_ = m.Start(ctx)

		if err := m.Complete(ctx); err != nil {
			t.Errorf("Complete() error = %v", err)
		}

		if m.State() != runbook.ExecutionStateCompleted {
			t.Errorf("state = %v, want %v", m.State(), runbook.ExecutionStateCompleted)
		}

		if !m.IsTerminal() {
			t.Error("IsTerminal() = false, want true")
		}
	})

	t.Run("running to failed", func(t *testing.T) {
		m := NewExecutionMachine()
		_ = m.Start(ctx)

		if err := m.Fail(ctx); err != nil {
			t.Errorf("Fail() error = %v", err)
		}

		if m.State() != runbook.ExecutionStateFailed {
			t.Errorf("state = %v, want %v", m.State(), runbook.ExecutionStateFailed)
		}
	})

	t.Run("pending to cancelled", func(t *testing.T) {
		m := NewExecutionMachine()

		if err := m.Cancel(ctx); err != nil {
			t.Errorf("Cancel() error = %v", err)
		}

		if m.State() != runbook.ExecutionStateCancelled {
			t.Errorf("state = %v, want %v", m.State(), runbook.ExecutionStateCancelled)
		}
	})

	t.Run("running to cancelled", func(t *testing.T) {
		m := NewExecutionMachine()
		_ = m.Start(ctx)

		if err := m.Cancel(ctx); err != nil {
			t.Errorf("Cancel() error = %v", err)
		}

		if m.State() != runbook.ExecutionStateCancelled {
			t.Errorf("state = %v, want %v", m.State(), runbook.ExecutionStateCancelled)
		}
	})

	t.Run("invalid transition", func(t *testing.T) {
		m := NewExecutionMachine()

		// Cannot complete from pending
		if err := m.Complete(ctx); err == nil {
			t.Error("Complete() from pending should error")
		}
	})
}

func TestExecutionMachine_Helpers(t *testing.T) {
	ctx := context.Background()

	t.Run("CanStart", func(t *testing.T) {
		m := NewExecutionMachine()

		if !m.CanStart() {
			t.Error("CanStart() = false, want true")
		}

		_ = m.Start(ctx)

		if m.CanStart() {
			t.Error("CanStart() after Start() = true, want false")
		}
	})

	t.Run("CanCancel", func(t *testing.T) {
		m := NewExecutionMachine()

		if !m.CanCancel() {
			t.Error("CanCancel() = false, want true")
		}

		_ = m.Start(ctx)

		if !m.CanCancel() {
			t.Error("CanCancel() when running = false, want true")
		}

		_ = m.Complete(ctx)

		if m.CanCancel() {
			t.Error("CanCancel() when completed = true, want false")
		}
	})

	t.Run("IsPending", func(t *testing.T) {
		m := NewExecutionMachine()

		if !m.IsPending() {
			t.Error("IsPending() = false, want true")
		}

		_ = m.Start(ctx)

		if m.IsPending() {
			t.Error("IsPending() when running = true, want false")
		}
	})

	t.Run("IsRunning", func(t *testing.T) {
		m := NewExecutionMachine()

		if m.IsRunning() {
			t.Error("IsRunning() when pending = true, want false")
		}

		_ = m.Start(ctx)

		if !m.IsRunning() {
			t.Error("IsRunning() = false, want true")
		}
	})
}

func TestExecutionMachine_WithCallbacks(t *testing.T) {
	ctx := context.Background()

	var callbackCalled bool
	var lastFrom, lastTo runbook.ExecutionState
	var lastEvent ExecutionEvent

	callback := func(ctx context.Context, from, to runbook.ExecutionState, event ExecutionEvent) {
		callbackCalled = true
		lastFrom = from
		lastTo = to
		lastEvent = event
	}

	m := NewExecutionMachineWithCallbacks(callback)
	_ = m.Start(ctx)

	if !callbackCalled {
		t.Error("callback not called")
	}
	if lastFrom != runbook.ExecutionStatePending {
		t.Errorf("lastFrom = %v, want %v", lastFrom, runbook.ExecutionStatePending)
	}
	if lastTo != runbook.ExecutionStateRunning {
		t.Errorf("lastTo = %v, want %v", lastTo, runbook.ExecutionStateRunning)
	}
	if lastEvent != EventStart {
		t.Errorf("lastEvent = %v, want %v", lastEvent, EventStart)
	}
}

func TestStepMachine_Transitions(t *testing.T) {
	ctx := context.Background()

	t.Run("pending to running to completed", func(t *testing.T) {
		m := NewStepMachine("test-step")

		if m.State() != runbook.StepStatePending {
			t.Errorf("initial state = %v, want %v", m.State(), runbook.StepStatePending)
		}

		if m.Name() != "test-step" {
			t.Errorf("Name() = %v, want %v", m.Name(), "test-step")
		}

		if err := m.Start(ctx); err != nil {
			t.Errorf("Start() error = %v", err)
		}

		if m.State() != runbook.StepStateRunning {
			t.Errorf("state = %v, want %v", m.State(), runbook.StepStateRunning)
		}

		if err := m.Complete(ctx); err != nil {
			t.Errorf("Complete() error = %v", err)
		}

		if m.State() != runbook.StepStateCompleted {
			t.Errorf("state = %v, want %v", m.State(), runbook.StepStateCompleted)
		}
	})

	t.Run("pending to skipped", func(t *testing.T) {
		m := NewStepMachine("skip-step")

		if err := m.Skip(ctx); err != nil {
			t.Errorf("Skip() error = %v", err)
		}

		if m.State() != runbook.StepStateSkipped {
			t.Errorf("state = %v, want %v", m.State(), runbook.StepStateSkipped)
		}

		if !m.IsSkipped() {
			t.Error("IsSkipped() = false, want true")
		}
	})

	t.Run("running to failed", func(t *testing.T) {
		m := NewStepMachine("fail-step")
		_ = m.Start(ctx)

		if err := m.Fail(ctx); err != nil {
			t.Errorf("Fail() error = %v", err)
		}

		if m.State() != runbook.StepStateFailed {
			t.Errorf("state = %v, want %v", m.State(), runbook.StepStateFailed)
		}

		if !m.IsFailed() {
			t.Error("IsFailed() = false, want true")
		}
	})
}

func TestStepMachine_Helpers(t *testing.T) {
	ctx := context.Background()

	t.Run("CanStart and CanSkip", func(t *testing.T) {
		m := NewStepMachine("test")

		if !m.CanStart() {
			t.Error("CanStart() = false, want true")
		}

		if !m.CanSkip() {
			t.Error("CanSkip() = false, want true")
		}

		_ = m.Start(ctx)

		if m.CanStart() {
			t.Error("CanStart() when running = true, want false")
		}

		if m.CanSkip() {
			t.Error("CanSkip() when running = true, want false")
		}
	})

	t.Run("IsTerminal", func(t *testing.T) {
		m := NewStepMachine("test")

		if m.IsTerminal() {
			t.Error("IsTerminal() when pending = true, want false")
		}

		_ = m.Start(ctx)

		if m.IsTerminal() {
			t.Error("IsTerminal() when running = true, want false")
		}

		_ = m.Complete(ctx)

		if !m.IsTerminal() {
			t.Error("IsTerminal() when completed = false, want true")
		}
	})
}

func TestStepMachine_WithCallbacks(t *testing.T) {
	ctx := context.Background()

	var callbackCalled bool
	callback := func(ctx context.Context, from, to runbook.StepState, event StepEvent) {
		callbackCalled = true
	}

	m := NewStepMachineWithCallbacks("test", callback)
	_ = m.Start(ctx)

	if !callbackCalled {
		t.Error("callback not called")
	}
}
