package audit_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/audit"
)

func seedN(t *testing.T, as audit.AuditStore, n int) []audit.AuditEntry {
	t.Helper()
	out := make([]audit.AuditEntry, n)
	for i := 0; i < n; i++ {
		e := audit.MustNewAuditEntry(audit.AuditEntryInput{Action: "x"})
		out[i] = e
		if err := as.Store(context.Background(), e); err != nil {
			t.Fatalf("Store %d: %v", i, err)
		}
		// 1ms spacing so UUIDv7 IDs strictly increase.
		time.Sleep(1 * time.Millisecond)
	}
	return out
}

func TestIterateAll_HappyPath(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	stored := seedN(t, as, 25)

	var got []string
	err := audit.IterateAll(context.Background(), as, audit.AuditQuery{Limit: 10},
		func(e audit.AuditEntry) error {
			got = append(got, e.ID)
			return nil
		})
	if err != nil {
		t.Fatalf("IterateAll: %v", err)
	}
	if len(got) != 25 {
		t.Errorf("got %d entries, want 25", len(got))
	}
	for i, e := range stored {
		if got[i] != e.ID {
			t.Errorf("entry %d: got %s, want %s", i, got[i], e.ID)
		}
	}
}

func TestIterateAll_StopsOnFnError(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	seedN(t, as, 10)

	sentinel := errors.New("stop")
	calls := 0
	err := audit.IterateAll(context.Background(), as, audit.AuditQuery{Limit: 3},
		func(e audit.AuditEntry) error {
			calls++
			if calls == 4 {
				return sentinel
			}
			return nil
		})
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want sentinel", err)
	}
	if calls != 4 {
		t.Errorf("calls = %d, want 4 (stopped at first error)", calls)
	}
}

func TestIterateAll_CtxCancelBetweenPages(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	seedN(t, as, 10)

	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := audit.IterateAll(ctx, as, audit.AuditQuery{Limit: 3},
		func(e audit.AuditEntry) error {
			calls++
			if calls == 3 {
				cancel() // cancel after first full page
			}
			return nil
		})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	// Iterated through page 1 (3 entries) before cancel checked.
	if calls < 3 {
		t.Errorf("calls = %d, want >= 3", calls)
	}
}

func TestIterateAll_MaxPagesCap(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	seedN(t, as, 10)

	calls := 0
	err := audit.IterateAll(context.Background(), as, audit.AuditQuery{Limit: 1},
		func(e audit.AuditEntry) error {
			calls++
			return nil
		},
		audit.WithIterateMaxPages(3),
	)
	if !errors.Is(err, audit.ErrIterateMaxPages) {
		t.Errorf("err = %v, want ErrIterateMaxPages", err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3 (cap)", calls)
	}
}

func TestIterateAll_EmptySetReturnsNil(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	calls := 0
	err := audit.IterateAll(context.Background(), as, audit.AuditQuery{},
		func(e audit.AuditEntry) error {
			calls++
			return nil
		})
	if err != nil {
		t.Errorf("empty set: %v", err)
	}
	if calls != 0 {
		t.Errorf("empty set called fn %d times", calls)
	}
}

func TestIterateAll_SinglePageNoCursor(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	seedN(t, as, 5)

	calls := 0
	err := audit.IterateAll(context.Background(), as, audit.AuditQuery{Limit: 10},
		func(e audit.AuditEntry) error {
			calls++
			return nil
		})
	if err != nil {
		t.Errorf("%v", err)
	}
	if calls != 5 {
		t.Errorf("calls = %d, want 5", calls)
	}
}

func TestIterateAll_NilStoreRejected(t *testing.T) {
	t.Parallel()
	err := audit.IterateAll(context.Background(), nil, audit.AuditQuery{},
		func(audit.AuditEntry) error { return nil })
	if err == nil {
		t.Errorf("nil store accepted")
	}
}

func TestIterateAll_NilFnRejected(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	err := audit.IterateAll(context.Background(), as, audit.AuditQuery{}, nil)
	if err == nil {
		t.Errorf("nil fn accepted")
	}
}

func TestIterateAll_DescendingWalks(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	stored := seedN(t, as, 10)

	var got []string
	err := audit.IterateAll(context.Background(), as, audit.AuditQuery{Limit: 3, Descending: true},
		func(e audit.AuditEntry) error {
			got = append(got, e.ID)
			return nil
		})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(got) != 10 {
		t.Errorf("got %d, want 10", len(got))
	}
	// First entry returned is the LAST stored.
	if got[0] != stored[9].ID {
		t.Errorf("first descending entry = %s, want %s", got[0], stored[9].ID)
	}
	if got[9] != stored[0].ID {
		t.Errorf("last descending entry = %s, want %s", got[9], stored[0].ID)
	}
}

func TestWithIterateMaxPages_RejectsNonPositive(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	seedN(t, as, 5)

	// Non-positive cap is ignored — default kicks in, full walk
	// completes.
	calls := 0
	err := audit.IterateAll(context.Background(), as, audit.AuditQuery{Limit: 2},
		func(e audit.AuditEntry) error {
			calls++
			return nil
		},
		audit.WithIterateMaxPages(0),
		audit.WithIterateMaxPages(-3),
	)
	if err != nil {
		t.Errorf("%v", err)
	}
	if calls != 5 {
		t.Errorf("calls = %d, want 5", calls)
	}
}
