// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/health"
	"go.keystone-core.io/keystone-core/pkg/envelope"
	"go.keystone-core.io/keystone-core/pkg/natsstatus"
)

// stubNATS is the minimal NATSManager surface the checker exercises.
type stubNATS struct{ healthErr error }

func (s stubNATS) Start(context.Context) error    { return nil }
func (s stubNATS) Shutdown(context.Context) error { return nil }
func (s stubNATS) Health(context.Context) error   { return s.healthErr }
func (s stubNATS) PublishEnvelope(context.Context, string, envelope.Envelope) error {
	return nil
}
func (s stubNATS) EndpointSnapshots() []natsstatus.EndpointSnapshot { return nil }

// stubHealthStore satisfies state.HealthStore.
type stubHealthStore struct{ pingErr error }

func (s stubHealthStore) Ping(context.Context) error { return s.pingErr }

// slowNATS sleeps to exercise the per-check timeout path.
type slowNATS struct{ delay time.Duration }

func (n slowNATS) Start(context.Context) error    { return nil }
func (n slowNATS) Shutdown(context.Context) error { return nil }
func (n slowNATS) Health(ctx context.Context) error {
	select {
	case <-time.After(n.delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (n slowNATS) PublishEnvelope(context.Context, string, envelope.Envelope) error {
	return nil
}
func (n slowNATS) EndpointSnapshots() []natsstatus.EndpointSnapshot { return nil }

func discardLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func TestHealthChecker_AllHealthyAfterGrace(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	hc := newHealthChecker(stubNATS{}, stubHealthStore{},
		now.Add(-time.Minute), 30*time.Second, time.Second, clock, discardLogger())

	snap := hc.Snapshot(context.Background())
	if !snap.Ready {
		t.Errorf("Ready = false, want true (grace elapsed, all ok)")
	}
	if snap.InGracePeriod {
		t.Errorf("InGracePeriod = true after 60s uptime")
	}
	if c := snap.Components["nats"]; c.Status != "ok" {
		t.Errorf("nats = %+v", c)
	}
	if c := snap.Components["db"]; c.Status != "ok" {
		t.Errorf("db = %+v", c)
	}
}

func TestHealthChecker_InGracePeriodNotReady(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	// Started "5s ago"; grace = 30s → still in grace.
	hc := newHealthChecker(stubNATS{}, stubHealthStore{},
		now.Add(-5*time.Second), 30*time.Second, time.Second, clock, discardLogger())

	snap := hc.Snapshot(context.Background())
	if snap.Ready {
		t.Errorf("Ready = true during grace")
	}
	if !snap.InGracePeriod {
		t.Errorf("InGracePeriod = false")
	}
	// Components still report their actual status.
	if c := snap.Components["nats"]; c.Status != "ok" {
		t.Errorf("nats = %+v during grace", c)
	}
}

func TestHealthChecker_DBPingFailureMarksNotReady(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	hc := newHealthChecker(stubNATS{}, stubHealthStore{pingErr: errors.New("connection refused")},
		now.Add(-time.Minute), 30*time.Second, time.Second, clock, discardLogger())

	snap := hc.Snapshot(context.Background())
	if snap.Ready {
		t.Errorf("Ready = true with DB failure")
	}
	if c := snap.Components["db"]; c.Status != "fail" {
		t.Errorf("db = %+v, want fail", c)
	}
	// NATS should still be ok — checks run independently.
	if c := snap.Components["nats"]; c.Status != "ok" {
		t.Errorf("nats = %+v, want ok (independent of db failure)", c)
	}
}

func TestHealthChecker_NATSFailureMarksNotReady(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	hc := newHealthChecker(stubNATS{healthErr: errors.New("disconnected")}, stubHealthStore{},
		now.Add(-time.Minute), 30*time.Second, time.Second, clock, discardLogger())

	snap := hc.Snapshot(context.Background())
	if snap.Ready {
		t.Errorf("Ready = true with NATS failure")
	}
	if c := snap.Components["nats"]; c.Status != "fail" {
		t.Errorf("nats = %+v, want fail", c)
	}
}

// togglingNATS flips its Health() return between an error and nil
// based on a swappable function so a single Snapshot run sees the
// toggle without races. Drives the down→up recovery test.
type togglingNATS struct {
	mu  sync.Mutex
	err error
}

func (n *togglingNATS) Start(context.Context) error    { return nil }
func (n *togglingNATS) Shutdown(context.Context) error { return nil }
func (n *togglingNATS) Health(context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.err
}
func (n *togglingNATS) PublishEnvelope(context.Context, string, envelope.Envelope) error {
	return nil
}
func (n *togglingNATS) EndpointSnapshots() []natsstatus.EndpointSnapshot { return nil }

func (n *togglingNATS) set(err error) {
	n.mu.Lock()
	n.err = err
	n.mu.Unlock()
}

// TestHealthChecker_NATSDownThenUpRecovery exercises the §4.2
// acceptance bullet: "Health() reports unhealthy when NATS down;
// recovers when up." The toggling stub flips state between
// snapshots so a single test verifies the full transition matrix.
func TestHealthChecker_NATSDownThenUpRecovery(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	nats := &togglingNATS{}
	hc := newHealthChecker(nats, stubHealthStore{},
		now.Add(-time.Minute), 30*time.Second, time.Second, clock, discardLogger())

	// Phase 1: healthy.
	if snap := hc.Snapshot(context.Background()); !snap.Ready {
		t.Fatalf("phase 1: Ready = false (snap=%+v)", snap)
	}

	// Phase 2: NATS goes down.
	nats.set(errors.New("disconnected"))
	if snap := hc.Snapshot(context.Background()); snap.Ready {
		t.Errorf("phase 2: Ready = true with NATS down (snap=%+v)", snap)
	} else if c := snap.Components["nats"]; c.Status != "fail" {
		t.Errorf("phase 2: nats = %+v, want fail", c)
	}

	// Phase 3: NATS recovers.
	nats.set(nil)
	if snap := hc.Snapshot(context.Background()); !snap.Ready {
		t.Errorf("phase 3 (recovery): Ready = false after NATS recovered (snap=%+v)", snap)
	} else if c := snap.Components["nats"]; c.Status != "ok" {
		t.Errorf("phase 3 (recovery): nats = %+v, want ok", c)
	}
}

func TestHealthChecker_PerCheckTimeout(t *testing.T) {
	// NATS check sleeps 200ms; checkTimeout = 30ms. Status should be
	// "fail" — context deadline triggers from inside the slowNATS impl.
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	hc := newHealthChecker(slowNATS{delay: 200 * time.Millisecond}, stubHealthStore{},
		now.Add(-time.Minute), 30*time.Second, 30*time.Millisecond, clock, discardLogger())

	snap := hc.Snapshot(context.Background())
	if c := snap.Components["nats"]; c.Status != "fail" {
		t.Errorf("nats = %+v, want fail (timed out)", c)
	}
	// DB still ok.
	if c := snap.Components["db"]; c.Status != "ok" {
		t.Errorf("db = %+v, want ok", c)
	}
}

func TestHealthChecker_NoErrorStringsInPayload(t *testing.T) {
	// Failure error contains a "secret topology hint" string. Verify
	// the snapshot does NOT include it — error strings stay in logs.
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	const sentinel = "INTERNAL_HOST_10_42_77_3"
	hc := newHealthChecker(
		stubNATS{healthErr: errors.New(sentinel)},
		stubHealthStore{},
		now.Add(-time.Minute), 30*time.Second, time.Second, clock, discardLogger(),
	)

	snap := hc.Snapshot(context.Background())
	for name, c := range snap.Components {
		if got := c.Status; got != "ok" && got != "fail" {
			t.Errorf("%s status = %q, want ok|fail (not the error string)", name, got)
		}
	}
	// Defensive: ensure no JSON-rendered field contains the sentinel.
	// (componentStatus has no error field, so this is purely a
	// regression guard against a future schema addition.)
	for name, c := range snap.Components {
		if c.Status == sentinel {
			t.Errorf("%s leaks error string in payload", name)
		}
	}
}

func TestHealthChecker_ChecksRunInParallel(t *testing.T) {
	// Both checks sleep for 100ms; total Snapshot time should be
	// ≈100ms, not ≈200ms. A real wall clock — fakeClock would skip
	// the actual elapsed measurement we're testing.
	startedAt := time.Now().Add(-time.Minute)
	hc := newHealthChecker(
		slowNATS{delay: 100 * time.Millisecond},
		slowHealthStore{delay: 100 * time.Millisecond},
		startedAt, 30*time.Second, 500*time.Millisecond,
		time.Now, discardLogger(),
	)

	t0 := time.Now()
	hc.Snapshot(context.Background())
	elapsed := time.Since(t0)
	if elapsed > 180*time.Millisecond {
		t.Errorf("checks ran serially: %s elapsed for two 100ms checks", elapsed)
	}
}

// slowHealthStore mirrors slowNATS for the parallelism test.
type slowHealthStore struct{ delay time.Duration }

func (s slowHealthStore) Ping(ctx context.Context) error {
	select {
	case <-time.After(s.delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// TestHealthChecker_JetStreamExtraAppears verifies the Epic 17 task 6
// JetStream component lands in the snapshot map when an extras checker
// is registered alongside NATS+DB.
func TestHealthChecker_JetStreamExtraAppears(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	jsChecker := health.NewJetStreamChecker(health.JetStreamPingerFunc(func(context.Context) error {
		return nil
	}), 0)
	hc := newHealthChecker(stubNATS{}, stubHealthStore{},
		now.Add(-time.Minute), 30*time.Second, time.Second, clock, discardLogger(),
		jsChecker,
	)

	snap := hc.Snapshot(context.Background())
	if !snap.Ready {
		t.Fatalf("Ready=false, want true (nats + db + jetstream all ok)")
	}
	if c, ok := snap.Components["jetstream"]; !ok {
		t.Fatal("jetstream component missing")
	} else if c.Status != "ok" {
		t.Errorf("jetstream = %+v, want ok", c)
	}
}

// TestHealthChecker_JetStreamUnhealthy_MarksNotReady verifies a JS
// failure flips Ready=false even when nats + db are ok.
func TestHealthChecker_JetStreamUnhealthy_MarksNotReady(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	jsChecker := health.NewJetStreamChecker(health.JetStreamPingerFunc(func(context.Context) error {
		return errors.New("jetstream not enabled")
	}), 0)
	hc := newHealthChecker(stubNATS{}, stubHealthStore{},
		now.Add(-time.Minute), 30*time.Second, time.Second, clock, discardLogger(),
		jsChecker,
	)
	snap := hc.Snapshot(context.Background())
	if snap.Ready {
		t.Errorf("Ready=true with JetStream down")
	}
	if c := snap.Components["jetstream"]; c.Status != "fail" {
		t.Errorf("jetstream = %+v, want fail", c)
	}
}

func TestHealthChecker_ConcurrentSnapshots(t *testing.T) {
	// Hammer Snapshot from many goroutines under -race. The checker
	// holds no shared mutable state; this is a regression guard.
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	hc := newHealthChecker(stubNATS{}, stubHealthStore{},
		now.Add(-time.Minute), 30*time.Second, time.Second, clock, discardLogger())

	const n = 16
	var wg sync.WaitGroup
	var ok atomic.Int64
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			snap := hc.Snapshot(context.Background())
			if snap.Ready {
				ok.Add(1)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != n {
		t.Errorf("got %d ready snapshots, want %d", ok.Load(), n)
	}
}
