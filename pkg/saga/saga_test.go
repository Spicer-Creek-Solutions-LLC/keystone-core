package saga

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

var (
	_ Log = (*MemoryLog)(nil)
	_ Log = (*SQLiteLog)(nil)
)

func TestHappyPath(t *testing.T) {
	log := NewMemoryLog()
	coord := New("test", WithLog(log))

	coord.AddStep(Step{
		Name:   "step1",
		Action: func(_ context.Context, _ map[string]any) error { return nil },
	})
	coord.AddStep(Step{
		Name:   "step2",
		Action: func(_ context.Context, _ map[string]any) error { return nil },
	})
	coord.AddStep(Step{
		Name:   "step3",
		Action: func(_ context.Context, _ map[string]any) error { return nil },
	})

	exec, err := coord.Execute(context.Background(), "run-1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exec.Status != StatusCompleted {
		t.Errorf("got status %q, want %q", exec.Status, StatusCompleted)
	}
	for i, sr := range exec.Steps {
		if sr.Status != StepCompleted {
			t.Errorf("step %d: got status %q, want %q", i, sr.Status, StepCompleted)
		}
	}
}

func TestMidStepFailure_Compensated(t *testing.T) {
	var compensated []string
	coord := New("test", WithLog(NewMemoryLog()))

	coord.AddStep(Step{
		Name:       "step1",
		Action:     func(_ context.Context, _ map[string]any) error { return nil },
		Compensate: func(_ context.Context, _ map[string]any) error { compensated = append(compensated, "step1"); return nil },
	})
	coord.AddStep(Step{
		Name:   "step2",
		Action: func(_ context.Context, _ map[string]any) error { return errors.New("step2 failed") },
	})
	coord.AddStep(Step{
		Name:   "step3",
		Action: func(_ context.Context, _ map[string]any) error { return nil },
	})

	exec, err := coord.Execute(context.Background(), "run-1", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if exec.Status != StatusCompensated {
		t.Errorf("got status %q, want %q", exec.Status, StatusCompensated)
	}
	if len(compensated) != 1 || compensated[0] != "step1" {
		t.Errorf("got compensated %v, want [step1]", compensated)
	}
	if exec.Steps[0].Status != StepCompensated {
		t.Errorf("step1: got %q, want %q", exec.Steps[0].Status, StepCompensated)
	}
	if exec.Steps[1].Status != StepFailed {
		t.Errorf("step2: got %q, want %q", exec.Steps[1].Status, StepFailed)
	}
	if exec.Steps[2].Status != StepSkipped {
		t.Errorf("step3: got %q, want %q", exec.Steps[2].Status, StepSkipped)
	}
}

func TestCompensationFailure_StatusFailed(t *testing.T) {
	coord := New("test", WithLog(NewMemoryLog()))

	coord.AddStep(Step{
		Name:       "step1",
		Action:     func(_ context.Context, _ map[string]any) error { return nil },
		Compensate: func(_ context.Context, _ map[string]any) error { return errors.New("compensate failed") },
	})
	coord.AddStep(Step{
		Name:   "step2",
		Action: func(_ context.Context, _ map[string]any) error { return errors.New("step2 failed") },
	})

	exec, err := coord.Execute(context.Background(), "run-1", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if exec.Status != StatusFailed {
		t.Errorf("got status %q, want %q", exec.Status, StatusFailed)
	}
}

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	coord := New("test", WithLog(NewMemoryLog()))

	coord.AddStep(Step{
		Name: "step1",
		Action: func(_ context.Context, _ map[string]any) error {
			cancel()
			return nil
		},
	})
	coord.AddStep(Step{
		Name:   "step2",
		Action: func(_ context.Context, _ map[string]any) error { return nil },
	})

	exec, err := coord.Execute(ctx, "run-1", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if exec.Status != StatusFailed {
		t.Errorf("got status %q, want %q", exec.Status, StatusFailed)
	}
}

func TestDataSharing(t *testing.T) {
	coord := New("test", WithLog(NewMemoryLog()))

	coord.AddStep(Step{
		Name: "producer",
		Action: func(_ context.Context, data map[string]any) error {
			data["key"] = "value-from-step1"
			return nil
		},
	})
	coord.AddStep(Step{
		Name: "consumer",
		Action: func(_ context.Context, data map[string]any) error {
			v, ok := data["key"]
			if !ok {
				return errors.New("key not found")
			}
			if v != "value-from-step1" {
				return errors.New("unexpected value")
			}
			return nil
		},
	})

	exec, err := coord.Execute(context.Background(), "run-1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exec.Status != StatusCompleted {
		t.Errorf("got status %q, want %q", exec.Status, StatusCompleted)
	}
}

func TestPersistence_SaveCalledPerStep(t *testing.T) {
	var saveCount atomic.Int32
	log := &countingLog{inner: NewMemoryLog(), saves: &saveCount}
	coord := New("test", WithLog(log))

	coord.AddStep(Step{
		Name:   "step1",
		Action: func(_ context.Context, _ map[string]any) error { return nil },
	})
	coord.AddStep(Step{
		Name:   "step2",
		Action: func(_ context.Context, _ map[string]any) error { return nil },
	})

	_, err := coord.Execute(context.Background(), "run-1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expected saves: initial + (running + completed) per step + final completed
	// = 1 + 2*2 + 1 = 6
	count := saveCount.Load()
	if count < 6 {
		t.Errorf("expected at least 6 saves, got %d", count)
	}
}

func TestEmptySaga(t *testing.T) {
	coord := New("empty", WithLog(NewMemoryLog()))

	exec, err := coord.Execute(context.Background(), "run-1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exec.Status != StatusCompleted {
		t.Errorf("got status %q, want %q", exec.Status, StatusCompleted)
	}
}

func TestResume(t *testing.T) {
	log := NewMemoryLog()
	var callCount int

	coord := New("test", WithLog(log))
	coord.AddStep(Step{
		Name:   "step1",
		Action: func(_ context.Context, _ map[string]any) error { return nil },
	})
	coord.AddStep(Step{
		Name: "step2",
		Action: func(_ context.Context, _ map[string]any) error {
			callCount++
			if callCount == 1 {
				return errors.New("transient failure")
			}
			return nil
		},
		Compensate: func(_ context.Context, _ map[string]any) error { return nil },
	})

	// First execution fails at step2.
	exec, _ := coord.Execute(context.Background(), "resume-1", nil)
	if exec.Status != StatusCompensated {
		t.Fatalf("got status %q, want %q", exec.Status, StatusCompensated)
	}

	// Manually set execution state to simulate resume-ready state.
	saved, _ := log.Get(context.Background(), "resume-1")
	saved.Status = StatusRunning
	saved.Steps[0].Status = StepCompleted
	saved.Steps[1].Status = StepPending
	saved.EndedAt = nil
	saved.Error = ""
	_ = log.Save(context.Background(), saved)

	exec, err := coord.Resume(context.Background(), "resume-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exec.Status != StatusCompleted {
		t.Errorf("got status %q, want %q", exec.Status, StatusCompleted)
	}
}

func TestResume_NoLog(t *testing.T) {
	coord := New("test")
	_, err := coord.Resume(context.Background(), "any")
	if err == nil {
		t.Fatal("expected error when resuming without log")
	}
}

func TestResume_CompletedExecution(t *testing.T) {
	log := NewMemoryLog()
	coord := New("test", WithLog(log))
	coord.AddStep(Step{
		Name:   "step1",
		Action: func(_ context.Context, _ map[string]any) error { return nil },
	})

	exec, _ := coord.Execute(context.Background(), "done-1", nil)
	if exec.Status != StatusCompleted {
		t.Fatalf("got %q, want %q", exec.Status, StatusCompleted)
	}

	// Resume a completed execution should return it as-is.
	exec, err := coord.Resume(context.Background(), "done-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exec.Status != StatusCompleted {
		t.Errorf("got %q, want %q", exec.Status, StatusCompleted)
	}
}

type countingLog struct {
	inner Log
	saves *atomic.Int32
}

func (c *countingLog) Save(ctx context.Context, exec *Execution) error {
	c.saves.Add(1)
	return c.inner.Save(ctx, exec)
}

func (c *countingLog) Get(ctx context.Context, id string) (*Execution, error) {
	return c.inner.Get(ctx, id)
}

func (c *countingLog) List(ctx context.Context, name string, limit int) ([]*Execution, error) {
	return c.inner.List(ctx, name, limit)
}
