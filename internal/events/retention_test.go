// SPDX-License-Identifier: Apache-2.0

package events

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// retentionStubStore implements just the [EventStore] surface the
// retention enforcer uses. Records every ApplyRetention call so
// tests can assert against policy lists + call counts.
type retentionStubStore struct {
	mu            sync.Mutex
	applyCalls    int
	lastPolicies  []RetentionPolicy
	deleteCount   int           // value returned per call
	err           error         // optional error returned per call
	applySignaled chan struct{} // optional notify on each call
	applyHook     func()        // called inside ApplyRetention (for slow / blocking tests)
}

func (s *retentionStubStore) Store(context.Context, Event) error         { return nil }
func (s *retentionStubStore) StoreBatch(context.Context, []Event) error  { return nil }
func (s *retentionStubStore) Get(context.Context, string) (Event, error) { return Event{}, nil }
func (s *retentionStubStore) Query(context.Context, EventQuery) (EventPage, error) {
	return EventPage{}, nil
}
func (s *retentionStubStore) Count(context.Context, EventQuery) (int, error) { return 0, nil }
func (s *retentionStubStore) Delete(context.Context, string) error           { return nil }
func (s *retentionStubStore) ApplyRetention(_ context.Context, policies []RetentionPolicy) (int, error) {
	s.mu.Lock()
	s.applyCalls++
	// Defensive copy so test assertions stay stable even if the
	// enforcer were to mutate the slice between calls.
	s.lastPolicies = append([]RetentionPolicy(nil), policies...)
	deleted := s.deleteCount
	err := s.err
	hook := s.applyHook
	signal := s.applySignaled
	s.mu.Unlock()

	// Signal BEFORE the hook so blocking-hook tests can observe
	// that the worker reached ApplyRetention before the hook holds
	// it there. Non-blocking-hook tests still see the signal once
	// the function exits via the same channel.
	if signal != nil {
		select {
		case signal <- struct{}{}:
		default:
		}
	}
	if hook != nil {
		hook()
	}
	return deleted, err
}
func (s *retentionStubStore) Close() error { return nil }

func (s *retentionStubStore) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.applyCalls
}

func (s *retentionStubStore) policies() []RetentionPolicy {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]RetentionPolicy, len(s.lastPolicies))
	copy(out, s.lastPolicies)
	return out
}

// silentLogger discards all output so test logs stay readable.
func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(discardWriter{}, nil))
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

// ---- constructor ---------------------------------------------------------

func TestNewRetentionEnforcer_RequiresStore(t *testing.T) {
	t.Parallel()
	_, err := NewRetentionEnforcer()
	if err == nil {
		t.Fatalf("missing-store didn't error")
	}
}

func TestNewRetentionEnforcer_EmptyPoliciesFallsBackToCatchAll(t *testing.T) {
	t.Parallel()
	store := &retentionStubStore{}
	e, err := NewRetentionEnforcer(WithRetentionStore(store))
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(e.policies) != 1 {
		t.Fatalf("len(policies) = %d, want 1 (catch-all default)", len(e.policies))
	}
	got := e.policies[0]
	if got.Type != "" || got.MaxAge != DefaultCatchAllPolicy.MaxAge || got.MaxCount != DefaultCatchAllPolicy.MaxCount {
		t.Errorf("policy = %+v, want %+v", got, DefaultCatchAllPolicy)
	}
}

func TestNewRetentionEnforcer_DefaultsApplied(t *testing.T) {
	t.Parallel()
	store := &retentionStubStore{}
	e, err := NewRetentionEnforcer(WithRetentionStore(store))
	if err != nil {
		t.Fatalf("%v", err)
	}
	if e.interval != DefaultRetentionInterval {
		t.Errorf("interval = %v, want %v", e.interval, DefaultRetentionInterval)
	}
	if e.jitter != DefaultRetentionJitter {
		t.Errorf("jitter = %f, want %f", e.jitter, DefaultRetentionJitter)
	}
	if e.leaderCheck == nil {
		t.Errorf("leaderCheck nil")
	}
	if e.logger == nil {
		t.Errorf("logger nil")
	}
}

func TestNewRetentionEnforcer_InvalidValuesFallback(t *testing.T) {
	t.Parallel()
	store := &retentionStubStore{}
	e, err := NewRetentionEnforcer(
		WithRetentionStore(store),
		WithRetentionInterval(0),
		WithRetentionInterval(-time.Second),
		WithRetentionJitter(-0.1),
		WithRetentionJitter(0.99), // out of [0, 0.5]
		WithRetentionLeaderCheck(nil),
		WithRetentionLogger(nil),
	)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if e.interval != DefaultRetentionInterval {
		t.Errorf("interval fallback failed: %v", e.interval)
	}
	if e.jitter != DefaultRetentionJitter {
		t.Errorf("jitter fallback failed: %f", e.jitter)
	}
	if e.leaderCheck == nil {
		t.Errorf("leaderCheck nil after WithRetentionLeaderCheck(nil)")
	}
	if e.logger == nil {
		t.Errorf("logger nil after WithRetentionLogger(nil)")
	}
}

func TestNewRetentionEnforcer_OptionsOverride(t *testing.T) {
	t.Parallel()
	store := &retentionStubStore{}
	called := false
	customPolicies := []RetentionPolicy{
		{Type: "agent.connect", MaxAge: 24 * time.Hour},
		{Type: "job.start", MaxCount: 100},
	}
	e, err := NewRetentionEnforcer(
		WithRetentionStore(store),
		WithRetentionPolicies(customPolicies),
		WithRetentionInterval(2*time.Hour),
		WithRetentionJitter(0.3),
		WithRetentionLeaderCheck(func() bool { called = true; return false }),
		WithRetentionLogger(silentLogger()),
	)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if e.interval != 2*time.Hour {
		t.Errorf("interval = %v", e.interval)
	}
	if e.jitter != 0.3 {
		t.Errorf("jitter = %f", e.jitter)
	}
	if len(e.policies) != 2 {
		t.Errorf("policies len = %d, want 2", len(e.policies))
	}
	// Invoke leaderCheck to trigger the side effect we're testing —
	// our custom check returns false, which proves the option's
	// function is the one stored.
	if got := e.leaderCheck(); got {
		t.Errorf("leaderCheck returned true; want false from custom")
	}
	if !called {
		t.Errorf("leaderCheck not invoked")
	}
}

func TestNewRetentionEnforcer_PoliciesDefensiveCopy(t *testing.T) {
	t.Parallel()
	store := &retentionStubStore{}
	in := []RetentionPolicy{{Type: "agent.connect", MaxAge: time.Hour}}
	e, err := NewRetentionEnforcer(WithRetentionStore(store), WithRetentionPolicies(in))
	if err != nil {
		t.Fatalf("%v", err)
	}
	// Mutating the caller's slice must not alter the enforcer's
	// stored policy.
	in[0].MaxAge = time.Minute
	if e.policies[0].MaxAge != time.Hour {
		t.Errorf("caller mutation leaked into enforcer: %v", e.policies[0].MaxAge)
	}
}

// ---- RunOnce ------------------------------------------------------------

func TestRunOnce_HappyPath(t *testing.T) {
	t.Parallel()
	store := &retentionStubStore{deleteCount: 7}
	e, _ := NewRetentionEnforcer(WithRetentionStore(store), WithRetentionLogger(silentLogger()))

	deleted, err := e.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("%v", err)
	}
	if deleted != 7 {
		t.Errorf("deleted = %d, want 7", deleted)
	}
	if store.calls() != 1 {
		t.Errorf("ApplyRetention called %d times, want 1", store.calls())
	}
	if e.LastRunDeleted() != 7 {
		t.Errorf("LastRunDeleted = %d", e.LastRunDeleted())
	}
	if e.TotalDeleted() != 7 {
		t.Errorf("TotalDeleted = %d", e.TotalDeleted())
	}
	if e.RunsFailed() != 0 {
		t.Errorf("RunsFailed = %d", e.RunsFailed())
	}
	if e.LastRunAt().IsZero() {
		t.Errorf("LastRunAt zero after successful run")
	}
}

func TestRunOnce_StoreErrorRecorded(t *testing.T) {
	t.Parallel()
	store := &retentionStubStore{err: errors.New("simulated store failure")}
	e, _ := NewRetentionEnforcer(WithRetentionStore(store), WithRetentionLogger(silentLogger()))

	_, err := e.RunOnce(context.Background())
	if err == nil {
		t.Fatalf("expected error")
	}
	if e.RunsFailed() != 1 {
		t.Errorf("RunsFailed = %d, want 1", e.RunsFailed())
	}
	// LastRunDeleted should NOT update on failure.
	if e.LastRunDeleted() != 0 {
		t.Errorf("LastRunDeleted updated on failure: %d", e.LastRunDeleted())
	}
}

func TestRunOnce_LeaderCheckFalseSkips(t *testing.T) {
	t.Parallel()
	store := &retentionStubStore{deleteCount: 5}
	e, _ := NewRetentionEnforcer(
		WithRetentionStore(store),
		WithRetentionLeaderCheck(func() bool { return false }),
		WithRetentionLogger(silentLogger()),
	)
	deleted, err := e.RunOnce(context.Background())
	if err != nil {
		t.Errorf("RunOnce returned err: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0 (skipped)", deleted)
	}
	if store.calls() != 0 {
		t.Errorf("ApplyRetention called %d times, want 0 (not leader)", store.calls())
	}
}

func TestRunOnce_PassesPoliciesToStore(t *testing.T) {
	t.Parallel()
	store := &retentionStubStore{}
	custom := []RetentionPolicy{
		{Type: "agent.connect", MaxAge: 24 * time.Hour},
		{Type: "job.start", MaxCount: 100},
	}
	e, _ := NewRetentionEnforcer(
		WithRetentionStore(store),
		WithRetentionPolicies(custom),
		WithRetentionLogger(silentLogger()),
	)
	if _, err := e.RunOnce(context.Background()); err != nil {
		t.Fatalf("%v", err)
	}
	got := store.policies()
	if len(got) != 2 {
		t.Fatalf("got %d policies, want 2", len(got))
	}
	if got[0].Type != "agent.connect" || got[1].Type != "job.start" {
		t.Errorf("policies mis-ordered: %+v", got)
	}
}

// ---- Scheduler ----------------------------------------------------------

func TestScheduler_FirstTickFiresAfterInterval(t *testing.T) {
	t.Parallel()
	store := &retentionStubStore{
		deleteCount:   3,
		applySignaled: make(chan struct{}, 4),
	}
	e, _ := NewRetentionEnforcer(
		WithRetentionStore(store),
		WithRetentionInterval(50*time.Millisecond),
		WithRetentionJitter(0), // exact timing
		WithRetentionLogger(silentLogger()),
	)
	if err := e.Start(context.Background()); err != nil {
		t.Fatalf("%v", err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = e.Stop(stopCtx)
	})

	// No immediate tick: confirm no call within 10ms.
	select {
	case <-store.applySignaled:
		t.Fatalf("ApplyRetention fired before first interval (boot storm)")
	case <-time.After(10 * time.Millisecond):
	}
	// First tick should fire by ~60ms (interval + slack).
	select {
	case <-store.applySignaled:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("first tick never fired")
	}
}

func TestScheduler_TicksRepeatedly(t *testing.T) {
	t.Parallel()
	store := &retentionStubStore{
		applySignaled: make(chan struct{}, 8),
	}
	e, _ := NewRetentionEnforcer(
		WithRetentionStore(store),
		WithRetentionInterval(20*time.Millisecond),
		WithRetentionJitter(0),
		WithRetentionLogger(silentLogger()),
	)
	_ = e.Start(context.Background())
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = e.Stop(stopCtx)
	})

	for i := 0; i < 3; i++ {
		select {
		case <-store.applySignaled:
		case <-time.After(200 * time.Millisecond):
			t.Fatalf("tick %d never fired", i)
		}
	}
	if store.calls() < 3 {
		t.Errorf("calls = %d, want >= 3", store.calls())
	}
}

func TestScheduler_ErrorsDoNotStopScheduler(t *testing.T) {
	t.Parallel()
	store := &retentionStubStore{
		err: errors.New("transient store err"),
	}
	e, _ := NewRetentionEnforcer(
		WithRetentionStore(store),
		WithRetentionInterval(20*time.Millisecond),
		WithRetentionJitter(0),
		WithRetentionLogger(silentLogger()),
	)
	_ = e.Start(context.Background())
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = e.Stop(stopCtx)
	})

	// The scheduler must keep ticking even though every pass errors.
	// Poll runsFailed — the counter being asserted — rather than the
	// per-call entry signal: the stub signals on ApplyRetention ENTRY,
	// but the enforcer increments runsFailed only AFTER it returns, so
	// observing 3 entry signals and then reading RunsFailed raced the
	// 3rd increment (RunsFailed == 2 under load). The deadline just
	// waits longer on a slow runner; on success the loop exits as soon
	// as the count reaches 3 (~60ms at a 20ms interval).
	deadline := time.Now().Add(5 * time.Second)
	for e.RunsFailed() < 3 {
		if time.Now().After(deadline) {
			t.Fatalf("RunsFailed = %d after 5s, want >= 3 (errors stopped scheduler?)", e.RunsFailed())
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestScheduler_StopBeforeFirstTickIsClean(t *testing.T) {
	t.Parallel()
	store := &retentionStubStore{}
	e, _ := NewRetentionEnforcer(
		WithRetentionStore(store),
		WithRetentionInterval(1*time.Hour), // never tick during test
		WithRetentionLogger(silentLogger()),
	)
	_ = e.Start(context.Background())
	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := e.Stop(stopCtx); err != nil {
		t.Errorf("Stop: %v", err)
	}
	if store.calls() != 0 {
		t.Errorf("ApplyRetention fired %d times before first interval", store.calls())
	}
}

func TestScheduler_StopIsIdempotent(t *testing.T) {
	t.Parallel()
	store := &retentionStubStore{}
	e, _ := NewRetentionEnforcer(WithRetentionStore(store), WithRetentionLogger(silentLogger()))
	// Stop-before-Start is a no-op.
	if err := e.Stop(context.Background()); err != nil {
		t.Errorf("Stop before Start: %v", err)
	}
	_ = e.Start(context.Background())
	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := e.Stop(stopCtx); err != nil {
		t.Errorf("first Stop: %v", err)
	}
	if err := e.Stop(stopCtx); err != nil {
		t.Errorf("second Stop: %v", err)
	}
}

func TestScheduler_DoubleStartRejected(t *testing.T) {
	t.Parallel()
	store := &retentionStubStore{}
	e, _ := NewRetentionEnforcer(WithRetentionStore(store), WithRetentionLogger(silentLogger()))
	if err := e.Start(context.Background()); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })
	if err := e.Start(context.Background()); err == nil {
		t.Errorf("double Start returned nil err")
	}
}

func TestScheduler_StopReleasesInFlightApplyRetention(t *testing.T) {
	t.Parallel()
	// Hook the store to block until the test releases it; then Stop
	// should cancel the ctx and return promptly without waiting for
	// the blocked call to finish (we cap Stop's deadline tight).
	release := make(chan struct{})
	store := &retentionStubStore{
		applyHook: func() {
			<-release
		},
		applySignaled: make(chan struct{}, 1),
	}
	e, _ := NewRetentionEnforcer(
		WithRetentionStore(store),
		WithRetentionInterval(20*time.Millisecond),
		WithRetentionJitter(0),
		WithRetentionLogger(silentLogger()),
	)
	_ = e.Start(context.Background())

	// Wait for the worker to enter ApplyRetention (which now blocks).
	select {
	case <-store.applySignaled:
	case <-time.After(500 * time.Millisecond):
		close(release)
		t.Fatalf("first ApplyRetention never entered")
	}

	// Stop with a short deadline — the in-flight call is still
	// blocking on `release`, but Stop's ctx.cancel propagates to
	// the worker's context. We don't expect Stop to wait for the
	// store call to unblock — the store stub ignores ctx, so Stop's
	// deadline IS expected to fire.
	stopCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := e.Stop(stopCtx)
	close(release)
	// Deadline exceeded is the expected outcome here — Stop returns
	// ctx.Err() when the worker doesn't drain in time. The important
	// thing is Stop returned at all (didn't hang).
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Stop returned unexpected err: %v", err)
	}
}

// ---- Concurrency safety -------------------------------------------------

func TestRetentionEnforcer_ConcurrentRunOnce(t *testing.T) {
	t.Parallel()
	// Concurrent RunOnce calls from many goroutines must update
	// counters cleanly. Race detector catches the failure mode.
	store := &retentionStubStore{deleteCount: 1}
	e, _ := NewRetentionEnforcer(WithRetentionStore(store), WithRetentionLogger(silentLogger()))

	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = e.RunOnce(context.Background())
		}()
	}
	wg.Wait()
	if e.TotalDeleted() != int64(n) {
		t.Errorf("TotalDeleted = %d, want %d", e.TotalDeleted(), n)
	}
	if store.calls() != n {
		t.Errorf("store calls = %d, want %d", store.calls(), n)
	}
}

// ---- nextWait jitter ----------------------------------------------------

func TestNextWait_RespectsJitterBound(t *testing.T) {
	t.Parallel()
	store := &retentionStubStore{}
	e, _ := NewRetentionEnforcer(
		WithRetentionStore(store),
		WithRetentionInterval(1*time.Second),
		WithRetentionJitter(0.2),
		WithRetentionLogger(silentLogger()),
	)
	// Sample many wait durations and check every one is within
	// [800ms, 1200ms].
	const samples = 200
	for i := 0; i < samples; i++ {
		d := e.nextWait()
		if d < 800*time.Millisecond || d > 1200*time.Millisecond {
			t.Fatalf("nextWait sample %d = %v outside ±20%% bound", i, d)
		}
	}
}

func TestNextWait_ZeroJitterIsExact(t *testing.T) {
	t.Parallel()
	store := &retentionStubStore{}
	e, _ := NewRetentionEnforcer(
		WithRetentionStore(store),
		WithRetentionInterval(time.Second),
		WithRetentionJitter(0),
		WithRetentionLogger(silentLogger()),
	)
	if d := e.nextWait(); d != time.Second {
		t.Errorf("zero jitter = %v, want exactly 1s", d)
	}
}

// ---- Atomic counter sanity ----------------------------------------------

func TestRetentionEnforcer_TotalDeletedIsAtomic(t *testing.T) {
	t.Parallel()
	store := &retentionStubStore{deleteCount: 3}
	e, _ := NewRetentionEnforcer(WithRetentionStore(store), WithRetentionLogger(silentLogger()))

	const n = 100
	var wg sync.WaitGroup
	var deletedSum atomic.Int64
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d, _ := e.RunOnce(context.Background())
			deletedSum.Add(int64(d))
		}()
	}
	wg.Wait()
	if got := e.TotalDeleted(); got != deletedSum.Load() {
		t.Errorf("TotalDeleted = %d, want %d", got, deletedSum.Load())
	}
}

// ---- String --------------------------------------------------------------

func TestRetentionEnforcer_String(t *testing.T) {
	t.Parallel()
	store := &retentionStubStore{}
	e, _ := NewRetentionEnforcer(
		WithRetentionStore(store),
		WithRetentionInterval(2*time.Hour),
		WithRetentionJitter(0.15),
	)
	got := e.String()
	for _, want := range []string{"interval=2h0m0s", "jitter=0.15", "policies=1"} {
		if !contains(got, want) {
			t.Errorf("String %q missing %q", got, want)
		}
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
