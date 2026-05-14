package secrets

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/state"
)

func newLeaseStoreForTest(t *testing.T) state.Store {
	t.Helper()
	cfg := &state.Config{
		Backend: state.BackendSQLite,
		SQLite:  state.SQLiteConfig{Path: filepath.Join(t.TempDir(), "store.db")},
	}
	s, err := state.NewStore(cfg)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func newLeaseManagerForTest(t *testing.T) (*LeaseManager, state.LeaseStore) {
	t.Helper()
	store := newLeaseStoreForTest(t)
	lm, err := NewLeaseManager(LeaseManagerConfig{Store: store})
	if err != nil {
		t.Fatalf("NewLeaseManager: %v", err)
	}
	return lm, store
}

func TestNewLeaseManager_Validation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		cfg     LeaseManagerConfig
		wantSub string
	}{
		{
			name:    "nil store",
			cfg:     LeaseManagerConfig{},
			wantSub: "Store is required",
		},
		{
			name:    "negative poll interval",
			cfg:     LeaseManagerConfig{Store: nilStore{}, PollInterval: -1},
			wantSub: "PollInterval must be positive",
		},
		{
			name:    "jitter out of range high",
			cfg:     LeaseManagerConfig{Store: nilStore{}, Jitter: 0.9},
			wantSub: "Jitter must be in [0, 0.5]",
		},
		{
			name:    "jitter out of range negative",
			cfg:     LeaseManagerConfig{Store: nilStore{}, Jitter: -0.1},
			wantSub: "Jitter must be in [0, 0.5]",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewLeaseManager(tc.cfg)
			if err == nil {
				t.Fatalf("NewLeaseManager = nil err, want %q", tc.wantSub)
			}
			if !errors.Is(err, ErrInvalidBackend) {
				t.Errorf("err does not wrap ErrInvalidBackend: %v", err)
			}
		})
	}
}

func TestLeaseManager_DefaultStrategyIsLazy(t *testing.T) {
	t.Parallel()
	lm, _ := newLeaseManagerForTest(t)
	if lm.cfg.DefaultStrategy != RenewStrategyLazy {
		t.Errorf("DefaultStrategy = %v, want lazy", lm.cfg.DefaultStrategy)
	}
}

func TestLeaseManager_RecordLookupForget(t *testing.T) {
	t.Parallel()
	lm, _ := newLeaseManagerForTest(t)
	ctx := context.Background()

	err := lm.RecordWithError(ctx, "l-1", LeaseRecord{
		Backend: "vault", Path: "database/creds/app",
	})
	if err != nil {
		t.Fatalf("RecordWithError: %v", err)
	}

	got, ok := lm.Lookup("l-1")
	if !ok {
		t.Fatalf("Lookup(l-1) missed after Record")
	}
	if got.Backend != "vault" || got.Path != "database/creds/app" {
		t.Errorf("Lookup mismatch: %#v", got)
	}
	if got.Strategy != RenewStrategyLazy {
		t.Errorf("default strategy lost: got %v, want lazy", got.Strategy)
	}

	lm.Forget("l-1")
	if _, ok := lm.Lookup("l-1"); ok {
		t.Errorf("Lookup after Forget still hits")
	}
}

func TestLeaseManager_RecordHonorsExplicitStrategy(t *testing.T) {
	t.Parallel()
	lm, _ := newLeaseManagerForTest(t)

	err := lm.RecordWithError(context.Background(), "l-eager", LeaseRecord{
		Backend: "vault", Path: "p", Strategy: RenewStrategyEager,
	})
	if err != nil {
		t.Fatalf("RecordWithError: %v", err)
	}
	got, _ := lm.Lookup("l-eager")
	if got.Strategy != RenewStrategyEager {
		t.Errorf("strategy = %v, want eager", got.Strategy)
	}
}

func TestLeaseManager_RevokedLookupMisses(t *testing.T) {
	t.Parallel()
	lm, _ := newLeaseManagerForTest(t)
	ctx := context.Background()

	_ = lm.RecordWithError(ctx, "l-rev", LeaseRecord{Backend: "vault", Path: "p"})
	if err := lm.MarkRevoked(ctx, "l-rev"); err != nil {
		t.Fatalf("MarkRevoked: %v", err)
	}
	if _, ok := lm.Lookup("l-rev"); ok {
		t.Errorf("Lookup on revoked lease hit; want miss")
	}
}

func TestLeaseManager_Lifecycle(t *testing.T) {
	t.Parallel()
	lm, _ := newLeaseManagerForTest(t)
	ctx := context.Background()

	if err := lm.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := lm.Start(ctx); err == nil {
		t.Errorf("double Start = nil err")
	}

	stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := lm.Stop(stopCtx); err != nil {
		t.Errorf("Stop: %v", err)
	}
	if err := lm.Stop(stopCtx); err != nil {
		t.Errorf("double Stop: %v", err)
	}
	if err := lm.Start(ctx); err == nil {
		t.Errorf("Start after Stop = nil err")
	}
}

func TestLeaseManager_SchedulerRenewsAtThreshold(t *testing.T) {
	t.Parallel()

	store := newLeaseStoreForTest(t)
	now := time.Now().UTC().Truncate(time.Second)
	clock := newTestClock(now)

	lm, err := NewLeaseManager(LeaseManagerConfig{
		Store:        store,
		PollInterval: 5 * time.Millisecond,
		Jitter:       0, // deterministic
		Clock:        clock.Now,
	})
	if err != nil {
		t.Fatalf("NewLeaseManager: %v", err)
	}

	// Seed an eager (50%) lease at 1h TTL where 31 minutes have
	// elapsed → ShouldRenew true.
	rec := &state.LeaseStoreRecord{
		ID:         "l-eager-1",
		Backend:    "vault",
		SecretPath: "database/creds/app",
		IssuedAt:   now.Add(-31 * time.Minute),
		ExpiresAt:  now.Add(29 * time.Minute),
		Duration:   time.Hour,
		Renewable:  true,
		State:      "active",
		Strategy:   "eager",
	}
	if err := store.CreateLease(context.Background(), rec); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var renewCount int32
	renewed := make(chan struct{}, 1)
	lm.SetRenewer(func(_ context.Context, req RenewLeaseRequest) (*LeaseInfo, error) {
		atomic.AddInt32(&renewCount, 1)
		// New TTL = same hour.
		return &LeaseInfo{
			ID:        req.LeaseID,
			Duration:  time.Hour,
			ExpiresAt: clock.Now().Add(time.Hour),
			Renewable: true,
		}, nil
	})

	gotRenewed := make(chan LifecycleEvent, 1)
	lm.OnLifecycle(func(_ context.Context, evt LifecycleEvent) {
		if evt.Type == LifecycleEventRenewed {
			select {
			case gotRenewed <- evt:
				close(renewed)
			default:
			}
		}
	})

	if err := lm.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = lm.Stop(stopCtx)
	}()

	select {
	case <-renewed:
	case <-time.After(2 * time.Second):
		t.Fatalf("scheduler did not trigger renewal in 2s")
	}

	if atomic.LoadInt32(&renewCount) == 0 {
		t.Errorf("renewer not called")
	}

	// Verify the store row reflects the renewal.
	got, _ := store.GetLease(context.Background(), "l-eager-1")
	if got.RenewCount < 1 {
		t.Errorf("store RenewCount = %d, want ≥1", got.RenewCount)
	}
}

func TestLeaseManager_TerminalExpiredMarksRow(t *testing.T) {
	t.Parallel()

	store := newLeaseStoreForTest(t)
	now := time.Now().UTC().Truncate(time.Second)
	clock := newTestClock(now)

	lm, err := NewLeaseManager(LeaseManagerConfig{
		Store:        store,
		PollInterval: 5 * time.Millisecond,
		Jitter:       0,
		Clock:        clock.Now,
	})
	if err != nil {
		t.Fatalf("NewLeaseManager: %v", err)
	}

	rec := &state.LeaseStoreRecord{
		ID:         "l-exp",
		Backend:    "vault",
		SecretPath: "p",
		IssuedAt:   now.Add(-31 * time.Minute),
		ExpiresAt:  now.Add(29 * time.Minute),
		Duration:   time.Hour,
		Renewable:  true,
		State:      "active",
		Strategy:   "eager",
	}
	_ = store.CreateLease(context.Background(), rec)

	lm.SetRenewer(func(_ context.Context, _ RenewLeaseRequest) (*LeaseInfo, error) {
		return nil, fmt.Errorf("backend: %w", ErrLeaseExpired)
	})

	expired := make(chan struct{}, 1)
	lm.OnLifecycle(func(_ context.Context, evt LifecycleEvent) {
		if evt.Type == LifecycleEventExpired {
			select {
			case expired <- struct{}{}:
			default:
			}
		}
	})

	if err := lm.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = lm.Stop(stopCtx)
	}()

	select {
	case <-expired:
	case <-time.After(2 * time.Second):
		t.Fatalf("LifecycleEventExpired never fired")
	}

	got, _ := store.GetLease(context.Background(), "l-exp")
	if got.State != "expired" {
		t.Errorf("state = %q, want expired", got.State)
	}
}

func TestLeaseManager_NotRenewableMarksRow(t *testing.T) {
	t.Parallel()

	store := newLeaseStoreForTest(t)
	now := time.Now().UTC().Truncate(time.Second)
	clock := newTestClock(now)

	lm, err := NewLeaseManager(LeaseManagerConfig{
		Store:        store,
		PollInterval: 5 * time.Millisecond,
		Jitter:       0,
		Clock:        clock.Now,
	})
	if err != nil {
		t.Fatalf("NewLeaseManager: %v", err)
	}

	rec := &state.LeaseStoreRecord{
		ID:         "l-nr",
		Backend:    "vault",
		SecretPath: "p",
		IssuedAt:   now.Add(-31 * time.Minute),
		ExpiresAt:  now.Add(29 * time.Minute),
		Duration:   time.Hour,
		Renewable:  true,
		State:      "active",
		Strategy:   "eager",
	}
	_ = store.CreateLease(context.Background(), rec)

	lm.SetRenewer(func(_ context.Context, _ RenewLeaseRequest) (*LeaseInfo, error) {
		return nil, fmt.Errorf("backend: %w", ErrLeaseNotRenewable)
	})

	nr := make(chan struct{}, 1)
	lm.OnLifecycle(func(_ context.Context, evt LifecycleEvent) {
		if evt.Type == LifecycleEventNotRenewable {
			select {
			case nr <- struct{}{}:
			default:
			}
		}
	})

	if err := lm.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = lm.Stop(stopCtx)
	}()

	select {
	case <-nr:
	case <-time.After(2 * time.Second):
		t.Fatalf("LifecycleEventNotRenewable never fired")
	}

	got, _ := store.GetLease(context.Background(), "l-nr")
	if got.Renewable {
		t.Errorf("Renewable still true; want false")
	}
}

func TestLeaseManager_TransientFailureRetries(t *testing.T) {
	t.Parallel()

	store := newLeaseStoreForTest(t)
	now := time.Now().UTC().Truncate(time.Second)
	clock := newTestClock(now)

	lm, err := NewLeaseManager(LeaseManagerConfig{
		Store:        store,
		PollInterval: 5 * time.Millisecond,
		Jitter:       0,
		Clock:        clock.Now,
	})
	if err != nil {
		t.Fatalf("NewLeaseManager: %v", err)
	}

	rec := &state.LeaseStoreRecord{
		ID:         "l-tx",
		Backend:    "vault",
		SecretPath: "p",
		IssuedAt:   now.Add(-31 * time.Minute),
		ExpiresAt:  now.Add(29 * time.Minute),
		Duration:   time.Hour,
		Renewable:  true,
		State:      "active",
		Strategy:   "eager",
	}
	_ = store.CreateLease(context.Background(), rec)

	failures := make(chan struct{}, 4)
	lm.SetRenewer(func(_ context.Context, _ RenewLeaseRequest) (*LeaseInfo, error) {
		return nil, errors.New("transient: connection refused")
	})
	lm.OnLifecycle(func(_ context.Context, evt LifecycleEvent) {
		if evt.Type == LifecycleEventRenewalFailed {
			select {
			case failures <- struct{}{}:
			default:
			}
		}
	})

	if err := lm.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = lm.Stop(stopCtx)
	}()

	// Expect at least two retry attempts.
	for i := 0; i < 2; i++ {
		select {
		case <-failures:
		case <-time.After(2 * time.Second):
			t.Fatalf("got %d failure event(s), want ≥2", i)
		}
	}

	// State must still be active — transient failure doesn't mark expired.
	got, _ := store.GetLease(context.Background(), "l-tx")
	if got.State != "active" {
		t.Errorf("state = %q, want active (transient failure should not mark expired)", got.State)
	}
}

func TestLeaseManager_PreExpiredMarksOnTick(t *testing.T) {
	t.Parallel()

	store := newLeaseStoreForTest(t)
	now := time.Now().UTC().Truncate(time.Second)
	clock := newTestClock(now)

	lm, err := NewLeaseManager(LeaseManagerConfig{
		Store:        store,
		PollInterval: 5 * time.Millisecond,
		Jitter:       0,
		Clock:        clock.Now,
	})
	if err != nil {
		t.Fatalf("NewLeaseManager: %v", err)
	}

	// Already past expiry per local check.
	rec := &state.LeaseStoreRecord{
		ID:         "l-pre",
		Backend:    "vault",
		SecretPath: "p",
		IssuedAt:   now.Add(-2 * time.Hour),
		ExpiresAt:  now.Add(-time.Minute),
		Duration:   time.Hour,
		Renewable:  true,
		State:      "active",
		Strategy:   "eager",
	}
	_ = store.CreateLease(context.Background(), rec)

	expired := make(chan struct{}, 1)
	lm.OnLifecycle(func(_ context.Context, evt LifecycleEvent) {
		if evt.Type == LifecycleEventExpired {
			select {
			case expired <- struct{}{}:
			default:
			}
		}
	})

	// No renewer wired — the local check must catch it without ever
	// calling Vault.
	if err := lm.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = lm.Stop(stopCtx)
	}()

	select {
	case <-expired:
	case <-time.After(2 * time.Second):
		t.Fatalf("local-expiry check did not fire LifecycleEventExpired")
	}
}

func TestLeaseManager_List(t *testing.T) {
	t.Parallel()
	lm, _ := newLeaseManagerForTest(t)
	ctx := context.Background()

	for _, id := range []string{"a", "b", "c"} {
		_ = lm.RecordWithError(ctx, id, LeaseRecord{Backend: "vault", Path: "p/" + id})
	}

	got, err := lm.List(ctx, state.LeaseFilter{Backend: "vault"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("len = %d, want 3", len(got))
	}
}

func TestLeaseManager_SetStrategy(t *testing.T) {
	t.Parallel()
	lm, store := newLeaseManagerForTest(t)
	ctx := context.Background()

	_ = lm.RecordWithError(ctx, "s-1", LeaseRecord{Backend: "vault", Path: "p"})

	if err := lm.SetStrategy(ctx, "s-1", RenewStrategyEager); err != nil {
		t.Fatalf("SetStrategy: %v", err)
	}
	rec, _ := store.GetLease(ctx, "s-1")
	if rec.Strategy != "eager" {
		t.Errorf("strategy = %q, want eager", rec.Strategy)
	}

	if err := lm.SetStrategy(ctx, "s-1", RenewStrategyUnknown); err == nil {
		t.Errorf("SetStrategy(Unknown) = nil err, want rejection")
	}
}

func TestLeaseManager_ExpireCleanup(t *testing.T) {
	t.Parallel()
	lm, store := newLeaseManagerForTest(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	// Expired + past expiry — eligible for cleanup.
	rec := &state.LeaseStoreRecord{
		ID:         "exp",
		Backend:    "vault",
		SecretPath: "p",
		IssuedAt:   now.Add(-2 * time.Hour),
		ExpiresAt:  now.Add(-time.Hour),
		Duration:   time.Hour,
		State:      "expired",
		Strategy:   "lazy",
	}
	_ = store.CreateLease(ctx, rec)

	n, err := lm.ExpireCleanup(ctx, now)
	if err != nil {
		t.Fatalf("ExpireCleanup: %v", err)
	}
	if n != 1 {
		t.Errorf("cleaned = %d, want 1", n)
	}
}

func TestLeaseManager_ConcurrentRecordLookup(t *testing.T) {
	t.Parallel()
	lm, _ := newLeaseManagerForTest(t)

	const n = 25
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		i := i
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = lm.RecordWithError(context.Background(), fmt.Sprintf("c-%d", i),
				LeaseRecord{Backend: "vault", Path: fmt.Sprintf("p-%d", i)})
		}()
		go func() {
			defer wg.Done()
			_, _ = lm.Lookup(fmt.Sprintf("c-%d", (i+10)%n))
		}()
	}
	wg.Wait()
}

func TestLeaseManager_NextWaitJitterRange(t *testing.T) {
	t.Parallel()
	lm, _ := newLeaseManagerForTest(t)
	lm.cfg.PollInterval = 100 * time.Millisecond
	lm.cfg.Jitter = 0.2

	min := time.Duration(float64(lm.cfg.PollInterval) * 0.8)
	max := time.Duration(float64(lm.cfg.PollInterval) * 1.2)

	for i := 0; i < 100; i++ {
		got := lm.nextWait()
		if got < min || got > max {
			t.Errorf("nextWait[%d] = %v, want in [%v, %v]", i, got, min, max)
		}
	}
}

func TestLeaseManager_RenewerNotWired_LogsAndContinues(t *testing.T) {
	t.Parallel()

	store := newLeaseStoreForTest(t)
	now := time.Now().UTC().Truncate(time.Second)
	clock := newTestClock(now)

	lm, err := NewLeaseManager(LeaseManagerConfig{
		Store:        store,
		PollInterval: 5 * time.Millisecond,
		Jitter:       0,
		Clock:        clock.Now,
	})
	if err != nil {
		t.Fatalf("NewLeaseManager: %v", err)
	}

	// Lease that wants renewal but no renewer is wired.
	rec := &state.LeaseStoreRecord{
		ID:         "l-norenewer",
		Backend:    "vault",
		SecretPath: "p",
		IssuedAt:   now.Add(-31 * time.Minute),
		ExpiresAt:  now.Add(29 * time.Minute),
		Duration:   time.Hour,
		Renewable:  true,
		State:      "active",
		Strategy:   "eager",
	}
	_ = store.CreateLease(context.Background(), rec)

	if err := lm.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = lm.Stop(stopCtx)
	}()

	// Give the scheduler a few ticks to run; nothing should crash.
	time.Sleep(50 * time.Millisecond)

	// Lease should still be active (no renewer = no transition).
	got, _ := store.GetLease(context.Background(), "l-norenewer")
	if got.State != "active" {
		t.Errorf("state = %q, want active (no renewer should leave state alone)", got.State)
	}
}

// ---- helpers -----------------------------------------------------

type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func newTestClock(start time.Time) *testClock {
	return &testClock{now: start}
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// nilStore satisfies state.LeaseStore for the validation tests where
// we never call any method.
type nilStore struct{}

func (nilStore) CreateLease(context.Context, *state.LeaseStoreRecord) error { return nil }
func (nilStore) GetLease(context.Context, string) (*state.LeaseStoreRecord, error) {
	return nil, state.ErrNotFound
}
func (nilStore) UpdateLease(context.Context, *state.LeaseStoreRecord) error { return nil }
func (nilStore) ListLeases(context.Context, state.LeaseFilter) ([]*state.LeaseStoreRecord, error) {
	return nil, nil
}
func (nilStore) DeleteLease(context.Context, string) error                       { return nil }
func (nilStore) DeleteExpiredLeases(context.Context, time.Time) (int, error)     { return 0, nil }
