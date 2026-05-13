package saga

import (
	"context"
	"sync"
)

// inMemoryLog is the default [Log] implementation. It is safe for
// concurrent use by multiple coordinators (operators have no use
// case for that today, but the safety is cheap).
type inMemoryLog struct {
	mu    sync.Mutex
	byID  map[string]*Execution
	order []string // insertion order for ListExecutions
}

// NewInMemoryLog returns a fresh in-memory [Log]. Use it for tests,
// for callers that only want the saga semantics without persistence,
// and as the [Coordinator.Log] when no other backend is available.
func NewInMemoryLog() Log {
	return &inMemoryLog{byID: map[string]*Execution{}}
}

func (l *inMemoryLog) SaveExecution(_ context.Context, e *Execution) error {
	if e == nil || e.ID == "" {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, seen := l.byID[e.ID]; !seen {
		l.order = append(l.order, e.ID)
	}
	// Defensive copy so the caller mutating the returned Execution
	// after Run doesn't affect what the log holds.
	l.byID[e.ID] = cloneExecution(e)
	return nil
}

func (l *inMemoryLog) GetExecution(_ context.Context, id string) (*Execution, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.byID[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneExecution(e), nil
}

func (l *inMemoryLog) ListExecutions(_ context.Context) ([]*Execution, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]*Execution, 0, len(l.order))
	for _, id := range l.order {
		out = append(out, cloneExecution(l.byID[id]))
	}
	return out, nil
}

// cloneExecution returns a deep-ish copy: the Steps slice is copied
// so callers can't mutate the log's internal state by appending,
// but Step.Error / Data values are shared references.
func cloneExecution(e *Execution) *Execution {
	if e == nil {
		return nil
	}
	out := *e
	if len(e.Steps) > 0 {
		out.Steps = make([]StepResult, len(e.Steps))
		copy(out.Steps, e.Steps)
	}
	if len(e.CompensateErrors) > 0 {
		out.CompensateErrors = make([]error, len(e.CompensateErrors))
		copy(out.CompensateErrors, e.CompensateErrors)
	}
	return &out
}
