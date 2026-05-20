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
