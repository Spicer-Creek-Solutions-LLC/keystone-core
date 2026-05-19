package blueprint

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrRunNotFound is returned by AppliedStore.Get for an unknown run ID.
var ErrRunNotFound = errors.New("blueprint: applied run not found")

// AppliedRun is the record of one blueprint apply (or rollback),
// enough to drive a later rollback: which blueprint/version, where it
// was loaded from, the namespace, the entrypoint that ran, and the
// resolved parameter + feature context.
//
// Params holds RESOLVED values including any secret-sourced ones.
// The in-memory store keeps them in process memory only (same trust
// boundary as the running executor); the durable store is a
// gate-v1.0 ROADMAP item and MUST mask sensitive params at rest.
type AppliedRun struct {
	ID         string
	ParentID   string // set on rollback records → the apply they revert
	Blueprint  string
	Version    string
	SourcePath string
	Namespace  string
	Entrypoint string
	Params     map[string]any
	Features   map[string]bool
	Status     string // "succeeded" | "failed"
	StartedAt  time.Time
	EndedAt    time.Time
}

// AppliedStore persists AppliedRun records. v1.0 ships the in-memory
// implementation; a durable backend is deferred (see ROADMAP
// "Blueprint applied-runs store (durable)", gate-v1.0).
type AppliedStore interface {
	Save(ctx context.Context, r AppliedRun) error
	Get(ctx context.Context, id string) (AppliedRun, error)
	List(ctx context.Context) ([]AppliedRun, error)
}

// MemoryAppliedStore is the default in-process AppliedStore. Safe for
// concurrent use.
type MemoryAppliedStore struct {
	mu    sync.Mutex
	byID  map[string]AppliedRun
	order []string
}

// NewMemoryAppliedStore returns an empty in-memory store.
func NewMemoryAppliedStore() *MemoryAppliedStore {
	return &MemoryAppliedStore{byID: make(map[string]AppliedRun)}
}

// Save writes r under r.ID (overwriting on repeat).
func (s *MemoryAppliedStore) Save(_ context.Context, r AppliedRun) error {
	if r.ID == "" {
		return errors.New("blueprint: applied run has no ID")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, seen := s.byID[r.ID]; !seen {
		s.order = append(s.order, r.ID)
	}
	s.byID[r.ID] = r
	return nil
}

// Get returns the run for id, or ErrRunNotFound.
func (s *MemoryAppliedStore) Get(_ context.Context, id string) (AppliedRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.byID[id]
	if !ok {
		return AppliedRun{}, ErrRunNotFound
	}
	return r, nil
}

// List returns runs in insertion order (oldest first).
func (s *MemoryAppliedStore) List(_ context.Context) ([]AppliedRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]AppliedRun, 0, len(s.order))
	for _, id := range s.order {
		out = append(out, s.byID[id])
	}
	return out, nil
}
