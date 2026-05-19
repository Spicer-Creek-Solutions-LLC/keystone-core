package verification

import (
	"context"
	"errors"
	"sync"
	"time"
)

// StoredVerification wraps a [WorkflowResult] with the identity and
// timestamp the `/api/v1/gitops/verifications` history surface needs.
// WorkflowResult itself gains no fields — id/app/time live here.
type StoredVerification struct {
	ID          string         `json:"id"`
	Application string         `json:"application,omitempty"`
	Result      WorkflowResult `json:"result"`
	CreatedAt   time.Time      `json:"created_at"`
}

// ResultStore persists verification workflow results for history /
// audit. The v1.0 in-memory [MemoryResultStore] is process-scoped;
// the durable [SQLiteResultStore] survives restarts. Get returns
// (nil,false,nil) when absent — the gitops-domain store convention.
type ResultStore interface {
	Save(ctx context.Context, sv *StoredVerification) error
	Get(ctx context.Context, id string) (*StoredVerification, bool, error)
	List(ctx context.Context) ([]*StoredVerification, error)
}

// MemoryResultStore is the in-memory [ResultStore] — the
// dark-until-boot default for the task-10 REST, symmetric with the
// rollback engine's in-memory store. Records are copied on the way
// in/out so callers cannot mutate stored state via the pointer.
type MemoryResultStore struct {
	mu  sync.RWMutex
	m   map[string]StoredVerification
	seq map[string]int
	n   int
}

// NewMemoryResultStore returns an empty store.
func NewMemoryResultStore() *MemoryResultStore {
	return &MemoryResultStore{m: make(map[string]StoredVerification), seq: make(map[string]int)}
}

func cloneStored(sv StoredVerification) StoredVerification {
	cp := sv
	cp.Result.Steps = append([]StepResult(nil), sv.Result.Steps...)
	return cp
}

// Save implements [ResultStore]. Nil record or empty ID is an error.
func (s *MemoryResultStore) Save(_ context.Context, sv *StoredVerification) error {
	if sv == nil || sv.ID == "" {
		return errors.New("verification: result store save: nil record or empty id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.seq[sv.ID]; !ok {
		s.n++
		s.seq[sv.ID] = s.n
	}
	s.m[sv.ID] = cloneStored(*sv)
	return nil
}

// Get implements [ResultStore].
func (s *MemoryResultStore) Get(_ context.Context, id string) (*StoredVerification, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sv, ok := s.m[id]
	if !ok {
		return nil, false, nil
	}
	c := cloneStored(sv)
	return &c, true, nil
}

// List implements [ResultStore], oldest insertion first.
func (s *MemoryResultStore) List(_ context.Context) ([]*StoredVerification, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*StoredVerification, 0, len(s.m))
	for _, sv := range s.m {
		c := cloneStored(sv)
		out = append(out, &c)
	}
	sortBySeq(out, s.seq)
	return out, nil
}

// sortBySeq orders records by their recorded insertion sequence
// (insertion-stable, matching the SQLite store's seq ordering).
func sortBySeq(out []*StoredVerification, seq map[string]int) {
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && seq[out[j-1].ID] > seq[out[j].ID]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
}
