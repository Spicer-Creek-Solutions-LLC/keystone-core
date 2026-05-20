package outbound

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/events"
)

// fakeDispatcher captures every Deliver call and returns a canned
// (status, err). delay throttles each call so tests can observe the
// concurrency cap.
type fakeDispatcher struct {
	mu       sync.Mutex
	gotSubs  []string
	gotIDs   []string
	gotPayls [][]byte
	status   int
	err      error
	delay    time.Duration
	inFlight int32
	peak     int32
}

func (f *fakeDispatcher) Deliver(_ context.Context, sub *Subscription, payload []byte, deliveryID string) (int, error) {
	n := atomic.AddInt32(&f.inFlight, 1)
	f.mu.Lock()
	if n > f.peak {
		f.peak = n
	}
	f.mu.Unlock()
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	f.mu.Lock()
	f.gotSubs = append(f.gotSubs, sub.ID)
	f.gotIDs = append(f.gotIDs, deliveryID)
	f.gotPayls = append(f.gotPayls, append([]byte(nil), payload...))
	f.mu.Unlock()
	atomic.AddInt32(&f.inFlight, -1)
	return f.status, f.err
}

func newManager(t *testing.T, d *fakeDispatcher, opts ...func(*Manager)) *Manager {
	t.Helper()
	m := &Manager{
		Store:                   NewMemoryStore(),
		Dispatcher:              d,
		MaxConcurrentDeliveries: 4,
	}
	for _, o := range opts {
		o(m)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = m.Stop(ctx)
	})
	return m
}

func newSub(id string, enabled bool, patterns ...string) *Subscription {
	ts := time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC)
	return &Subscription{
		ID: id, Name: id, URL: "https://x", Enabled: enabled, Events: patterns,
		MaxRetries: 0, TimeoutSec: 5, CreatedAt: ts, UpdatedAt: ts,
	}
}

func mkEvent(t *testing.T, typ events.EventType) events.Event {
	t.Helper()
	ev, err := events.NewEvent(typ, "test")
	if err != nil {
		t.Fatalf("NewEvent(%q): %v", typ, err)
	}
	return ev
}

func waitFor(t *testing.T, predicate func() bool, label string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if predicate() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", label)
}

func TestManager_Match_GlobAndDisabled(t *testing.T) {
	t.Parallel()
	d := &fakeDispatcher{status: 200}
	m := newManager(t, d)
	ctx := context.Background()
	_ = m.Store.CreateSubscription(ctx, newSub("yes", true, "state.*"))
	_ = m.Store.CreateSubscription(ctx, newSub("no-pattern", true, "agent.*"))
	_ = m.Store.CreateSubscription(ctx, newSub("disabled", false, "state.*"))
	_ = m.Store.CreateSubscription(ctx, newSub("empty-events", true)) // empty = match nothing
	if err := m.Refresh(ctx); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	m.Handle(ctx, mkEvent(t, "state.drift"))
	waitFor(t, func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return len(d.gotSubs) == 1
	}, "one delivery")
	if d.gotSubs[0] != "yes" {
		t.Errorf("delivered to %q, want yes only (no-pattern/disabled/empty must NOT deliver)", d.gotSubs[0])
	}
}

func TestManager_FanOut_MultipleSubs(t *testing.T) {
	t.Parallel()
	d := &fakeDispatcher{status: 200}
	m := newManager(t, d)
	ctx := context.Background()
	_ = m.Store.CreateSubscription(ctx, newSub("a", true, "state.drift"))
	_ = m.Store.CreateSubscription(ctx, newSub("b", true, "state.*"))
	_ = m.Store.CreateSubscription(ctx, newSub("c", true, "*.drift"))
	if err := m.Refresh(ctx); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	m.Handle(ctx, mkEvent(t, "state.drift"))
	waitFor(t, func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return len(d.gotSubs) == 3
	}, "three deliveries")

	list, _ := m.Store.ListDeliveries(ctx, "", 0)
	if len(list) != 3 {
		t.Errorf("store has %d deliveries, want 3", len(list))
	}
	for _, r := range list {
		if r.Status != DeliverySuccess || r.StatusCode != 200 {
			t.Errorf("delivery %s: status=%s code=%d, want success/200", r.ID, r.Status, r.StatusCode)
		}
	}
}

func TestManager_DispatcherError_RecordsFailed(t *testing.T) {
	t.Parallel()
	d := &fakeDispatcher{status: 502, err: errors.New("bad gateway")}
	m := newManager(t, d)
	ctx := context.Background()
	_ = m.Store.CreateSubscription(ctx, newSub("a", true, "policy.violation"))
	_ = m.Refresh(ctx)

	m.Handle(ctx, mkEvent(t, "policy.violation"))
	waitFor(t, func() bool {
		list, _ := m.Store.ListDeliveries(ctx, "a", 0)
		return len(list) >= 1 && list[0].Status == DeliveryFailed
	}, "failed delivery persisted")

	list, _ := m.Store.ListDeliveries(ctx, "a", 0)
	if list[0].StatusCode != 502 || !strings.Contains(list[0].Error, "bad gateway") {
		t.Errorf("delivery = %+v", list[0])
	}
}

func TestManager_PayloadTooLarge(t *testing.T) {
	t.Parallel()
	d := &fakeDispatcher{status: 200}
	m := newManager(t, d, func(m *Manager) { m.MaxPayloadBytes = 32 })
	ctx := context.Background()
	_ = m.Store.CreateSubscription(ctx, newSub("a", true, "*.*"))
	_ = m.Refresh(ctx)

	ev := mkEvent(t, "state.drift")
	ev.Data = map[string]any{"big": strings.Repeat("X", 4096)}
	m.Handle(ctx, ev)

	waitFor(t, func() bool {
		list, _ := m.Store.ListDeliveries(ctx, "", 0)
		return len(list) == 1
	}, "synthetic oversize record")
	list, _ := m.Store.ListDeliveries(ctx, "", 0)
	if list[0].Status != DeliveryFailed || !strings.Contains(list[0].Error, "payload") {
		t.Errorf("oversize record = %+v", list[0])
	}
	// Crucial: no fan-out to the dispatcher when oversized.
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.gotSubs) != 0 {
		t.Errorf("dispatcher called %d times for oversize event, want 0", len(d.gotSubs))
	}
}

func TestManager_ConcurrencyCap(t *testing.T) {
	t.Parallel()
	d := &fakeDispatcher{status: 200, delay: 30 * time.Millisecond}
	m := newManager(t, d, func(m *Manager) { m.MaxConcurrentDeliveries = 2 })
	ctx := context.Background()
	for i := 0; i < 8; i++ {
		_ = m.Store.CreateSubscription(ctx, newSub(string(rune('a'+i)), true, "state.drift"))
	}
	_ = m.Refresh(ctx)

	m.Handle(ctx, mkEvent(t, "state.drift"))
	waitFor(t, func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return len(d.gotSubs) == 8
	}, "all 8 deliveries")

	d.mu.Lock()
	peak := d.peak
	d.mu.Unlock()
	if peak > 2 {
		t.Errorf("peak concurrency = %d, want <= 2 (MaxConcurrentDeliveries)", peak)
	}
}

func TestManager_StopDrainsInFlight(t *testing.T) {
	t.Parallel()
	d := &fakeDispatcher{status: 200, delay: 20 * time.Millisecond}
	m := &Manager{Store: NewMemoryStore(), Dispatcher: d, MaxConcurrentDeliveries: 4}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		_ = m.Store.CreateSubscription(ctx, newSub(string(rune('a'+i)), true, "state.drift"))
	}
	_ = m.Refresh(ctx)
	m.Handle(ctx, mkEvent(t, "state.drift"))

	stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := m.Stop(stopCtx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.gotSubs) != 4 {
		t.Errorf("Stop did not drain: %d/4 delivered", len(d.gotSubs))
	}
}

func TestManager_RefreshPicksUpNewSubs(t *testing.T) {
	t.Parallel()
	d := &fakeDispatcher{status: 200}
	m := newManager(t, d)
	ctx := context.Background()

	m.Handle(ctx, mkEvent(t, "state.drift"))
	time.Sleep(20 * time.Millisecond)
	d.mu.Lock()
	if len(d.gotSubs) != 0 {
		d.mu.Unlock()
		t.Fatalf("no subs registered yet but got %d deliveries", len(d.gotSubs))
	}
	d.mu.Unlock()

	_ = m.Store.CreateSubscription(ctx, newSub("late", true, "state.drift"))
	if err := m.Refresh(ctx); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	m.Handle(ctx, mkEvent(t, "state.drift"))
	waitFor(t, func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return len(d.gotSubs) == 1
	}, "late sub picked up after Refresh")
}

func TestManager_StartIdempotentAndValidation(t *testing.T) {
	t.Parallel()
	// Missing Store / Dispatcher are programming errors caught at Start.
	if err := (&Manager{Dispatcher: &fakeDispatcher{}}).Start(context.Background()); err == nil {
		t.Error("Start with nil Store = nil, want error")
	}
	if err := (&Manager{Store: NewMemoryStore()}).Start(context.Background()); err == nil {
		t.Error("Start with nil Dispatcher = nil, want error")
	}

	d := &fakeDispatcher{status: 200}
	m := &Manager{Store: NewMemoryStore(), Dispatcher: d}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start #1: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start #2 (idempotent): %v", err)
	}
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop #1: %v", err)
	}
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop #2 (idempotent): %v", err)
	}
}

func TestMatches(t *testing.T) {
	t.Parallel()
	cases := []struct {
		typ   string
		patts []string
		want  bool
	}{
		{"state.drift", []string{"state.*"}, true},
		{"state.apply.start", []string{"state.apply.*"}, true},
		{"state.drift", []string{"agent.*"}, false},
		{"runbook.step.fail", []string{"runbook.*.fail"}, true},
		{"state.drift", []string{}, false},    // empty = no match
		{"state.drift", []string{"["}, false}, // bad glob: filepath.Match returns err, treated as no-match
		{"state.drift", []string{"a", "state.*"}, true},
	}
	for _, c := range cases {
		if got := matches(c.typ, c.patts); got != c.want {
			t.Errorf("matches(%q, %v) = %v, want %v", c.typ, c.patts, got, c.want)
		}
	}
}

// flakyDispatcher fails the first failBeforeSuccess attempts then
// succeeds. Records every attempt's payload.
type flakyDispatcher struct {
	mu                sync.Mutex
	failBeforeSuccess int
	attempts          int
}

func (f *flakyDispatcher) Deliver(_ context.Context, _ *Subscription, _ []byte, _ string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attempts++
	if f.attempts <= f.failBeforeSuccess {
		return 502, errors.New("transient")
	}
	return 200, nil
}

// recordingStore wraps an inner SubscriptionStore and counts
// SaveDelivery calls so tests can assert intermediate `retrying`
// upserts actually happen.
type recordingStore struct {
	SubscriptionStore
	mu     sync.Mutex
	saves  int
	states []DeliveryStatus
}

func (r *recordingStore) SaveDelivery(ctx context.Context, d *DeliveryRecord) error {
	r.mu.Lock()
	r.saves++
	r.states = append(r.states, d.Status)
	r.mu.Unlock()
	return r.SubscriptionStore.SaveDelivery(ctx, d)
}

func TestManager_Retry_SucceedsOnNthAttempt(t *testing.T) {
	t.Parallel()
	fd := &flakyDispatcher{failBeforeSuccess: 2}
	rec := &recordingStore{SubscriptionStore: NewMemoryStore()}
	m := &Manager{
		Store:                   rec,
		Dispatcher:              fd,
		MaxConcurrentDeliveries: 4,
		Retry:                   RetryPolicy{BaseBackoff: time.Millisecond, MaxBackoff: 2 * time.Millisecond},
		Jitterer:                func() float64 { return 0 },
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = m.Stop(ctx)
	})

	ctx := context.Background()
	sub := newSub("flaky", true, "state.drift")
	sub.MaxRetries = 3
	_ = rec.CreateSubscription(ctx, sub)
	_ = m.Refresh(ctx)

	m.Handle(ctx, mkEvent(t, "state.drift"))
	waitFor(t, func() bool {
		list, _ := rec.ListDeliveries(ctx, "flaky", 0)
		return len(list) == 1 && list[0].Status == DeliverySuccess
	}, "success after retries")

	list, _ := rec.ListDeliveries(ctx, "flaky", 0)
	if list[0].Attempt != 3 || list[0].Status != DeliverySuccess {
		t.Errorf("delivery = %+v, want attempt=3 / success", list[0])
	}

	// The §4.14 `retrying` intermediate state must be persisted
	// between attempts.
	rec.mu.Lock()
	gotRetrying := false
	for _, s := range rec.states {
		if s == DeliveryRetrying {
			gotRetrying = true
		}
	}
	saves := rec.saves
	rec.mu.Unlock()
	if !gotRetrying {
		t.Errorf("no `retrying` state was persisted; states = %v", rec.states)
	}
	if saves < 4 { // 1 Pending + ≥2 Retrying + 1 Success
		t.Errorf("saves = %d, want >=4", saves)
	}
}

func TestManager_Retry_ExhaustedKeepsFailedRecord(t *testing.T) {
	t.Parallel()
	fd := &flakyDispatcher{failBeforeSuccess: 9999} // always fails
	m := newManager(t, nil, func(m *Manager) {
		m.Dispatcher = fd
		m.Retry = RetryPolicy{BaseBackoff: time.Millisecond, MaxBackoff: 2 * time.Millisecond}
		m.Jitterer = func() float64 { return 0 }
	})

	ctx := context.Background()
	sub := newSub("doomed", true, "policy.violation")
	sub.MaxRetries = 2 // 1 initial + 2 retries = 3 attempts
	_ = m.Store.CreateSubscription(ctx, sub)
	_ = m.Refresh(ctx)

	m.Handle(ctx, mkEvent(t, "policy.violation"))
	waitFor(t, func() bool {
		list, _ := m.Store.ListDeliveries(ctx, "doomed", 0)
		return len(list) == 1 && list[0].Status == DeliveryFailed
	}, "failed after exhausting retries")

	list, _ := m.Store.ListDeliveries(ctx, "doomed", 0)
	if list[0].Attempt != 3 {
		t.Errorf("attempt = %d, want 3 (1+MaxRetries)", list[0].Attempt)
	}
	if list[0].Status != DeliveryFailed {
		t.Errorf("status = %s, want failed (record retained per §4.14)", list[0].Status)
	}
	fd.mu.Lock()
	defer fd.mu.Unlock()
	if fd.attempts != 3 {
		t.Errorf("dispatcher attempts = %d, want 3", fd.attempts)
	}
}

func TestManager_Retry_CtxCancelMidBackoff_TerminalFailed(t *testing.T) {
	t.Parallel()
	fd := &flakyDispatcher{failBeforeSuccess: 9999}
	m := newManager(t, nil, func(m *Manager) {
		m.Dispatcher = fd
		// Big backoff so we can cancel mid-sleep.
		m.Retry = RetryPolicy{BaseBackoff: 5 * time.Second, MaxBackoff: 10 * time.Second}
		m.Jitterer = func() float64 { return 0 }
	})

	ctx, cancel := context.WithCancel(context.Background())
	sub := newSub("bg", true, "state.drift")
	sub.MaxRetries = 3
	_ = m.Store.CreateSubscription(ctx, sub)
	_ = m.Refresh(ctx)

	m.Handle(ctx, mkEvent(t, "state.drift"))
	// Wait for the first failed attempt to land + manager to enter
	// the backoff sleep, then cancel.
	waitFor(t, func() bool {
		list, _ := m.Store.ListDeliveries(ctx, "bg", 0)
		return len(list) == 1 && list[0].Status == DeliveryRetrying
	}, "retrying state mid-loop")
	cancel()

	waitFor(t, func() bool {
		list, _ := m.Store.ListDeliveries(context.Background(), "bg", 0)
		return len(list) == 1 && list[0].Status == DeliveryFailed
	}, "ctx-cancel mid-backoff → terminal failed")

	fd.mu.Lock()
	defer fd.mu.Unlock()
	if fd.attempts > 1 {
		t.Errorf("dispatcher attempts = %d, want exactly 1 (no retry after cancel)", fd.attempts)
	}
}

func TestManager_Retry_NoRetriesOnSubMaxRetriesZero(t *testing.T) {
	t.Parallel()
	fd := &flakyDispatcher{failBeforeSuccess: 9999}
	m := newManager(t, nil, func(m *Manager) {
		m.Dispatcher = fd
		m.Retry = RetryPolicy{BaseBackoff: time.Millisecond}
		m.Jitterer = func() float64 { return 0 }
	})

	ctx := context.Background()
	sub := newSub("one-shot", true, "*.fail")
	sub.MaxRetries = 0
	_ = m.Store.CreateSubscription(ctx, sub)
	_ = m.Refresh(ctx)

	m.Handle(ctx, mkEvent(t, "job.fail"))
	waitFor(t, func() bool {
		list, _ := m.Store.ListDeliveries(ctx, "one-shot", 0)
		return len(list) == 1 && list[0].Status == DeliveryFailed
	}, "single-shot failed")
	fd.mu.Lock()
	defer fd.mu.Unlock()
	if fd.attempts != 1 {
		t.Errorf("dispatcher attempts = %d, want 1 (MaxRetries=0)", fd.attempts)
	}
}
