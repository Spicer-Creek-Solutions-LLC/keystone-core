package saga

import (
	"context"
	"fmt"
	"sync"
)

// MemoryLog is an in-memory Log implementation for testing and short-lived processes.
type MemoryLog struct {
	mu    sync.Mutex
	execs map[string]*Execution
}

// NewMemoryLog creates a new in-memory saga log.
func NewMemoryLog() *MemoryLog {
	return &MemoryLog{execs: make(map[string]*Execution)}
}

// Save persists an execution (deep copy to avoid external mutation).
func (m *MemoryLog) Save(_ context.Context, exec *Execution) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Deep copy to avoid external mutation.
	cp := *exec
	steps := make([]StepRecord, len(exec.Steps))
	copy(steps, exec.Steps)
	cp.Steps = steps
	if exec.Data != nil {
		cp.Data = make(map[string]any, len(exec.Data))
		for k, v := range exec.Data {
			cp.Data[k] = v
		}
	}
	m.execs[exec.ID] = &cp
	return nil
}

// Get retrieves an execution by ID.
func (m *MemoryLog) Get(_ context.Context, id string) (*Execution, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	exec, ok := m.execs[id]
	if !ok {
		return nil, fmt.Errorf("execution %q not found", id)
	}
	return exec, nil
}

// List returns executions filtered by name with an optional limit.
func (m *MemoryLog) List(_ context.Context, name string, limit int) ([]*Execution, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []*Execution
	for _, exec := range m.execs {
		if name != "" && exec.Name != name {
			continue
		}
		result = append(result, exec)
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result, nil
}
