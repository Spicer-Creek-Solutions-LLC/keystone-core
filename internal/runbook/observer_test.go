package runbook

import (
	"context"
	"sync"
	"testing"
)

type recObserver struct {
	mu sync.Mutex
	ev []ObserverEvent
}

func (r *recObserver) OnTransition(_ context.Context, ev ObserverEvent) {
	r.mu.Lock()
	r.ev = append(r.ev, ev)
	r.mu.Unlock()
}

func TestExecutor_ObserverFanout(t *testing.T) {
	e, reg := testExecutor(t)
	rec := &recObserver{}
	e.Observer = rec
	mustRegister(t, reg, "ok", func(context.Context, StepContext) (StepOutput, error) {
		return StepOutput{}, nil
	})
	rb := &Runbook{Metadata: Metadata{Name: "rb"}, Spec: Spec{Steps: []Step{
		{Type: "ok", Name: "s1"},
		{Type: "ok", Name: "s2", DependsOn: []string{"s1"}},
	}}}

	exec, err := e.Execute(context.Background(), rb, nil)
	if err != nil {
		t.Fatal(err)
	}

	// One observer event per trail entry, same content.
	if len(rec.ev) != len(exec.Trail) {
		t.Fatalf("observer got %d events, trail has %d", len(rec.ev), len(exec.Trail))
	}
	var execStart, stepDone, execDone bool
	for _, ev := range rec.ev {
		if ev.ExecutionID != exec.ID || ev.Runbook != "rb" {
			t.Fatalf("event identity wrong: %+v", ev)
		}
		switch {
		case ev.Step == "" && ev.To == StatusRunning:
			execStart = true
		case ev.Step == "" && ev.To == StatusSucceeded:
			execDone = true
		case ev.Step == "s1" && ev.To == StatusSucceeded:
			stepDone = true
		}
	}
	if !execStart || !stepDone || !execDone {
		t.Fatalf("missing transitions: start=%v stepDone=%v done=%v", execStart, stepDone, execDone)
	}
}

func TestMultiObserver(t *testing.T) {
	a, b := &recObserver{}, &recObserver{}
	m := MultiObserver{a, nil, b} // nil element skipped
	m.OnTransition(context.Background(), ObserverEvent{Runbook: "r", To: StatusRunning})
	if len(a.ev) != 1 || len(b.ev) != 1 {
		t.Fatalf("fan-out failed: a=%d b=%d", len(a.ev), len(b.ev))
	}
}

func TestNoopObserver(t *testing.T) {
	// Default observer is the no-op; Execute must not panic with it.
	e, reg := testExecutor(t)
	mustRegister(t, reg, "ok", func(context.Context, StepContext) (StepOutput, error) {
		return StepOutput{}, nil
	})
	if _, err := e.Execute(context.Background(),
		&Runbook{Metadata: Metadata{Name: "rb"}, Spec: Spec{Steps: []Step{{Type: "ok", Name: "s"}}}},
		nil); err != nil {
		t.Fatal(err)
	}
}
