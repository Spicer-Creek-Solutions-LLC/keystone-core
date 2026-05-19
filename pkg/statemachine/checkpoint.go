package statemachine

import (
	"context"
	"fmt"
	"sync"
)

// Snapshot is the persisted shape of a machine: its current state and
// full transition history.
type Snapshot[S comparable, E comparable] struct {
	State   S
	History []Record[S, E]
}

// Checkpointer is the optional persistence seam. v0.1 ships only the
// in-memory implementation ([NewMemoryCheckpointer]); durable backends
// (SQLite/etcd) are a ROADMAP item — see the package doc.
type Checkpointer[S comparable, E comparable] interface {
	// Save persists snap, replacing any prior snapshot.
	Save(ctx context.Context, snap Snapshot[S, E]) error
	// Load returns the last saved snapshot. found is false (with a nil
	// error) when nothing has been saved yet.
	Load(ctx context.Context) (snap Snapshot[S, E], found bool, err error)
}

// Checkpoint writes the machine's current state and history through
// the configured [Checkpointer]. It returns [ErrNoCheckpointer] when
// none was set on the builder.
func (m *Machine[S, E]) Checkpoint(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cp == nil {
		return ErrNoCheckpointer
	}
	hist := make([]Record[S, E], len(m.history))
	copy(hist, m.history)
	return m.cp.Save(ctx, Snapshot[S, E]{State: m.current, History: hist})
}

// RestoreFrom loads the last snapshot from the configured
// [Checkpointer] and adopts its state and history. It returns
// [ErrNoCheckpointer] when none was set, and [ErrUnknownState] (without
// mutating the machine) if the persisted state is not one this machine
// knows. found is false when the checkpointer holds no snapshot yet.
func (m *Machine[S, E]) RestoreFrom(ctx context.Context) (found bool, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cp == nil {
		return false, ErrNoCheckpointer
	}
	snap, found, err := m.cp.Load(ctx)
	if err != nil || !found {
		return found, err
	}
	if _, ok := m.states[snap.State]; !ok {
		return true, fmt.Errorf("%w: %v", ErrUnknownState, snap.State)
	}
	m.current = snap.State
	m.history = make([]Record[S, E], len(snap.History))
	copy(m.history, snap.History)
	return true, nil
}

// MemoryCheckpointer is an in-process [Checkpointer]. Safe for
// concurrent use.
type MemoryCheckpointer[S comparable, E comparable] struct {
	mu   sync.Mutex
	snap *Snapshot[S, E]
}

// NewMemoryCheckpointer returns an empty in-memory checkpointer.
func NewMemoryCheckpointer[S comparable, E comparable]() *MemoryCheckpointer[S, E] {
	return &MemoryCheckpointer[S, E]{}
}

// Save records a deep-enough copy of snap (the history slice is copied
// so later machine mutations don't alias the stored snapshot).
func (c *MemoryCheckpointer[S, E]) Save(_ context.Context, snap Snapshot[S, E]) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	hist := make([]Record[S, E], len(snap.History))
	copy(hist, snap.History)
	c.snap = &Snapshot[S, E]{State: snap.State, History: hist}
	return nil
}

// Load returns the stored snapshot, or found=false if none.
func (c *MemoryCheckpointer[S, E]) Load(_ context.Context) (Snapshot[S, E], bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.snap == nil {
		return Snapshot[S, E]{}, false, nil
	}
	hist := make([]Record[S, E], len(c.snap.History))
	copy(hist, c.snap.History)
	return Snapshot[S, E]{State: c.snap.State, History: hist}, true, nil
}
