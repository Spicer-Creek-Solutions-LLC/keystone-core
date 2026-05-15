package audit_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"

	"go.keystone-core.io/keystone-core/internal/audit"
)

type stubAuditStore struct {
	mu       sync.Mutex
	stored   []audit.AuditEntry
	storeErr error
}

func (s *stubAuditStore) Store(_ context.Context, e audit.AuditEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.storeErr != nil {
		return s.storeErr
	}
	s.stored = append(s.stored, e)
	return nil
}

func (s *stubAuditStore) StoreBatch(context.Context, []audit.AuditEntry) error { return nil }
func (s *stubAuditStore) Get(context.Context, string) (audit.AuditEntry, error) {
	return audit.AuditEntry{}, nil
}
func (s *stubAuditStore) Query(context.Context, audit.AuditQuery) (audit.AuditPage, error) {
	return audit.AuditPage{}, nil
}
func (s *stubAuditStore) Count(context.Context, audit.AuditQuery) (int, error) { return 0, nil }
func (s *stubAuditStore) Delete(context.Context, string) error                 { return nil }
func (s *stubAuditStore) ApplyRetention(context.Context, audit.RetentionPolicy) (int, error) {
	return 0, nil
}
func (s *stubAuditStore) Summarize(context.Context, audit.AuditQuery) (audit.AuditSummary, error) {
	return audit.AuditSummary{}, nil
}
func (s *stubAuditStore) Close() error { return nil }

func (s *stubAuditStore) entries() []audit.AuditEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]audit.AuditEntry, len(s.stored))
	copy(out, s.stored)
	return out
}

func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestStoreAuditor_EmitPersists(t *testing.T) {
	t.Parallel()
	store := &stubAuditStore{}
	a := audit.NewStoreAuditor(store, silentLogger())
	e := audit.MustNewAuditEntry(audit.AuditEntryInput{Action: "x"})

	a.Emit(context.Background(), e)
	got := store.entries()
	if len(got) != 1 || got[0].ID != e.ID {
		t.Errorf("entries = %+v", got)
	}
	if a.FailedStores() != 0 {
		t.Errorf("FailedStores = %d", a.FailedStores())
	}
}

func TestStoreAuditor_StoreErrorCountedNotPropagated(t *testing.T) {
	t.Parallel()
	store := &stubAuditStore{storeErr: errors.New("sim disk full")}
	a := audit.NewStoreAuditor(store, silentLogger())

	a.Emit(context.Background(), audit.MustNewAuditEntry(audit.AuditEntryInput{Action: "x"}))
	a.Emit(context.Background(), audit.MustNewAuditEntry(audit.AuditEntryInput{Action: "y"}))
	if a.FailedStores() != 2 {
		t.Errorf("FailedStores = %d, want 2", a.FailedStores())
	}
	if len(store.entries()) != 0 {
		t.Errorf("entries leaked through error path: %+v", store.entries())
	}
}

func TestStoreAuditor_NilReceiverNoOp(t *testing.T) {
	t.Parallel()
	var a *audit.StoreAuditor
	a.Emit(context.Background(), audit.AuditEntry{}) // must not panic
	if a.FailedStores() != 0 {
		t.Errorf("FailedStores on nil receiver = %d", a.FailedStores())
	}
}

func TestStoreAuditor_NilStoreNoOp(t *testing.T) {
	t.Parallel()
	a := audit.NewStoreAuditor(nil, silentLogger())
	a.Emit(context.Background(), audit.MustNewAuditEntry(audit.AuditEntryInput{Action: "x"}))
	if a.FailedStores() != 0 {
		t.Errorf("FailedStores with nil store = %d", a.FailedStores())
	}
}

func TestStoreAuditor_NilLoggerFallsBack(t *testing.T) {
	t.Parallel()
	store := &stubAuditStore{storeErr: errors.New("x")}
	a := audit.NewStoreAuditor(store, nil) // logger nil; must not panic
	a.Emit(context.Background(), audit.MustNewAuditEntry(audit.AuditEntryInput{Action: "x"}))
	if a.FailedStores() != 1 {
		t.Errorf("FailedStores = %d", a.FailedStores())
	}
}

func TestStoreAuditor_ConcurrentEmitAtomic(t *testing.T) {
	t.Parallel()
	store := &stubAuditStore{storeErr: errors.New("x")}
	a := audit.NewStoreAuditor(store, silentLogger())

	const n = 100
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a.Emit(context.Background(), audit.MustNewAuditEntry(audit.AuditEntryInput{Action: "x"}))
		}()
	}
	wg.Wait()
	if a.FailedStores() != int64(n) {
		t.Errorf("FailedStores = %d, want %d", a.FailedStores(), n)
	}
}
