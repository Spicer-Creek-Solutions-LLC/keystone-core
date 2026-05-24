// SPDX-License-Identifier: Apache-2.0

package outbound

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// cannedDispatcher returns the same (code, err) for every call and
// counts how many times Inner was reached (so tests can distinguish
// fast-fail from a real call).
type cannedDispatcher struct {
	mu    sync.Mutex
	code  int
	err   error
	calls int
}

func (c *cannedDispatcher) Deliver(_ context.Context, _ *Subscription, _ []byte, _ string) (int, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	return c.code, c.err
}

// fakeClock is a deterministic clock the breaker uses to age its
// Open window without test sleep.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

func newBreaker(t *testing.T, inner Dispatcher, clk *fakeClock) *CircuitBreaker {
	t.Helper()
	return &CircuitBreaker{
		Inner:             inner,
		FailureThreshold:  3, // smaller than the §4.14 default for compact tests
		OpenDuration:      100 * time.Millisecond,
		HalfOpenSuccesses: 2,
		Now:               clk.Now,
	}
}

func subAt(url string) *Subscription { return &Subscription{ID: url, URL: url} }

func deliver(cb *CircuitBreaker, sub *Subscription) (int, error) {
	return cb.Deliver(context.Background(), sub, nil, "d")
}

func TestCB_Defaults(t *testing.T) {
	t.Parallel()
	cb := &CircuitBreaker{}
	if cb.failureThreshold() != DefaultFailureThreshold {
		t.Errorf("failureThreshold = %d, want %d", cb.failureThreshold(), DefaultFailureThreshold)
	}
	if cb.openDuration() != DefaultOpenDuration {
		t.Errorf("openDuration = %v, want %v", cb.openDuration(), DefaultOpenDuration)
	}
	if cb.halfOpenSuccesses() != DefaultHalfOpenSuccesses {
		t.Errorf("halfOpenSuccesses = %d, want %d", cb.halfOpenSuccesses(), DefaultHalfOpenSuccesses)
	}
	// Nil clock falls back to time.Now.
	if cb.now().IsZero() {
		t.Error("now() with nil clock returned zero time")
	}
}

func TestCB_OpensAfterThreshold(t *testing.T) {
	t.Parallel()
	inner := &cannedDispatcher{code: 500, err: errors.New("boom")}
	clk := &fakeClock{now: time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)}
	cb := newBreaker(t, inner, clk)
	sub := subAt("https://a")

	for i := 0; i < cb.failureThreshold(); i++ {
		if _, err := deliver(cb, sub); !errors.Is(err, inner.err) {
			t.Fatalf("attempt %d: err = %v, want inner err", i+1, err)
		}
	}
	// Threshold reached → next call fast-fails without reaching inner.
	before := inner.calls
	if _, err := deliver(cb, sub); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("post-threshold call err = %v, want ErrCircuitOpen", err)
	}
	if inner.calls != before {
		t.Errorf("inner was called during open state (calls before=%d after=%d)", before, inner.calls)
	}
}

func TestCB_HalfOpenRecoversToClosedAfterTwoSuccesses(t *testing.T) {
	t.Parallel()
	failing := &cannedDispatcher{err: errors.New("boom")}
	clk := &fakeClock{now: time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)}
	cb := newBreaker(t, failing, clk)
	sub := subAt("https://b")

	// Trip the breaker.
	for i := 0; i < cb.failureThreshold(); i++ {
		_, _ = deliver(cb, sub)
	}
	if _, err := deliver(cb, sub); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected open, got %v", err)
	}

	// Age past OpenDuration → next call transitions to HalfOpen and
	// reaches inner. Swap inner to a succeeding dispatcher first so
	// the probe succeeds and starts the HalfOpen counter.
	clk.Advance(101 * time.Millisecond)
	ok := &cannedDispatcher{code: http.StatusOK}
	cb.Inner = ok

	if _, err := deliver(cb, sub); err != nil {
		t.Fatalf("first half-open probe = %v, want nil", err)
	}
	// One more success closes the breaker (HalfOpenSuccesses = 2).
	if _, err := deliver(cb, sub); err != nil {
		t.Fatalf("second half-open probe = %v, want nil", err)
	}
	// Now breaker is Closed — even after another failure, the next
	// call must reach inner (no fast-fail).
	cb.Inner = failing
	before := failing.calls
	if _, err := deliver(cb, sub); !errors.Is(err, failing.err) {
		t.Errorf("post-close call err = %v, want inner err", err)
	}
	if failing.calls != before+1 {
		t.Errorf("inner not reached after close (delta = %d)", failing.calls-before)
	}
}

func TestCB_HalfOpenFailureReopens(t *testing.T) {
	t.Parallel()
	failing := &cannedDispatcher{err: errors.New("boom")}
	clk := &fakeClock{now: time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)}
	cb := newBreaker(t, failing, clk)
	sub := subAt("https://c")

	for i := 0; i < cb.failureThreshold(); i++ {
		_, _ = deliver(cb, sub)
	}
	clk.Advance(cb.openDuration())

	// First half-open probe fails → breaker re-opens with a fresh
	// cooldown window.
	if _, err := deliver(cb, sub); !errors.Is(err, failing.err) {
		t.Fatalf("half-open probe err = %v, want inner err", err)
	}
	// Immediate next call must fast-fail again (re-opened).
	if _, err := deliver(cb, sub); !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("post-half-open-fail err = %v, want ErrCircuitOpen", err)
	}
	// Advance past the new cooldown → another half-open probe runs.
	before := failing.calls
	clk.Advance(cb.openDuration())
	_, _ = deliver(cb, sub)
	if failing.calls != before+1 {
		t.Errorf("post-cooldown probe did not reach inner (calls = %d, want +1)", failing.calls-before)
	}
}

func TestCB_PerEndpointIsolation(t *testing.T) {
	t.Parallel()
	inner := &cannedDispatcher{err: errors.New("boom")}
	clk := &fakeClock{now: time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)}
	cb := newBreaker(t, inner, clk)
	a := subAt("https://a")
	b := subAt("https://b")

	for i := 0; i < cb.failureThreshold(); i++ {
		_, _ = deliver(cb, a)
	}
	// A is open; B has independent breaker still closed.
	if _, err := deliver(cb, a); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("A: want ErrCircuitOpen, got %v", err)
	}
	if _, err := deliver(cb, b); !errors.Is(err, inner.err) {
		t.Errorf("B: want inner err (still closed), got %v", err)
	}
}

func TestCB_SuccessResetsClosedFailureCount(t *testing.T) {
	t.Parallel()
	failing := &cannedDispatcher{err: errors.New("boom")}
	clk := &fakeClock{now: time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)}
	cb := newBreaker(t, failing, clk)
	sub := subAt("https://d")

	// (threshold-1) failures, then a success: counter resets, and
	// the breaker should stay closed past what would have been the
	// trip point if the counter hadn't reset.
	for i := 0; i < cb.failureThreshold()-1; i++ {
		_, _ = deliver(cb, sub)
	}
	ok := &cannedDispatcher{code: http.StatusOK}
	cb.Inner = ok
	_, _ = deliver(cb, sub)

	cb.Inner = failing
	// Another (threshold-1) failures still must not trip — counter
	// was reset.
	for i := 0; i < cb.failureThreshold()-1; i++ {
		if _, err := deliver(cb, sub); !errors.Is(err, failing.err) {
			t.Fatalf("attempt %d: err = %v, want inner err", i+1, err)
		}
	}
	// One more failure trips.
	_, _ = deliver(cb, sub)
	if _, err := deliver(cb, sub); !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("post-final err = %v, want ErrCircuitOpen", err)
	}
}

func TestCB_ConcurrentDelivery_RaceClean(t *testing.T) {
	t.Parallel()
	inner := &cannedDispatcher{code: http.StatusOK}
	clk := &fakeClock{now: time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)}
	cb := newBreaker(t, inner, clk)

	var wg sync.WaitGroup
	var got int32
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			sub := subAt(string(rune('a' + n%4)))
			for j := 0; j < 8; j++ {
				_, _ = deliver(cb, sub)
				atomic.AddInt32(&got, 1)
			}
		}(i)
	}
	wg.Wait()
	if got != 32*8 {
		t.Errorf("calls counted = %d, want %d", got, 32*8)
	}
}
