// SPDX-License-Identifier: Apache-2.0

package nats

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/config"
)

func defaultBreakerConfig() config.CircuitBreakerConfig {
	return config.CircuitBreakerConfig{
		Enabled:             true,
		FailureThreshold:    3,
		SuccessThreshold:    2,
		OpenDuration:        time.Second,
		HalfOpenMaxAttempts: 3,
	}
}

// breakerClock is a manually-advanced fake clock for the time-bound
// open → half-open transition. Drives the breaker's now func.
type breakerClock struct {
	mu  sync.Mutex
	cur time.Time
}

func newBreakerClock() *breakerClock {
	return &breakerClock{cur: time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)}
}

func (c *breakerClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cur
}

func (c *breakerClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cur = c.cur.Add(d)
}

func TestNewBreaker_DisabledReturnsNoop(t *testing.T) {
	cfg := defaultBreakerConfig()
	cfg.Enabled = false
	b := newBreaker(cfg, time.Now)
	if _, ok := b.(noopBreaker); !ok {
		t.Errorf("disabled config returned %T, want noopBreaker", b)
	}
}

func TestNoopBreaker_AlwaysAllows(t *testing.T) {
	b := noopBreaker{}
	if !b.Allow() {
		t.Error("Allow = false, want true")
	}
	b.OnFailure()
	b.OnFailure()
	b.OnFailure()
	if !b.Allow() {
		t.Error("Allow after failures = false; noop must not advance state")
	}
	if b.Status() != CircuitClosed {
		t.Errorf("Status = %q, want closed", b.Status())
	}
}

func TestCircuitBreaker_StartsClosed(t *testing.T) {
	b := newBreaker(defaultBreakerConfig(), time.Now)
	if b.Status() != CircuitClosed {
		t.Errorf("initial Status = %q, want closed", b.Status())
	}
	if !b.Allow() {
		t.Error("Allow = false in initial closed state")
	}
}

func TestCircuitBreaker_ClosedToOpen(t *testing.T) {
	clk := newBreakerClock()
	b := newBreaker(defaultBreakerConfig(), clk.Now)
	for i := 0; i < 3; i++ { // FailureThreshold
		b.OnFailure()
	}
	if b.Status() != CircuitOpen {
		t.Errorf("Status = %q, want open after %d failures", b.Status(), 3)
	}
	if b.Allow() {
		t.Error("Allow = true while open before OpenDuration elapsed")
	}
}

func TestCircuitBreaker_ClosedSuccessResetsFailures(t *testing.T) {
	cfg := defaultBreakerConfig() // FailureThreshold=3
	b := newBreaker(cfg, time.Now)
	b.OnFailure()
	b.OnFailure()
	b.OnSuccess() // resets counter
	b.OnFailure()
	b.OnFailure()
	if b.Status() != CircuitClosed {
		t.Errorf("Status = %q, want still closed (success should have reset counter)", b.Status())
	}
}

func TestCircuitBreaker_OpenToHalfOpenAfterDuration(t *testing.T) {
	clk := newBreakerClock()
	cfg := defaultBreakerConfig()
	b := newBreaker(cfg, clk.Now)
	for i := 0; i < cfg.FailureThreshold; i++ {
		b.OnFailure()
	}

	// Before OpenDuration elapses: still open.
	clk.Advance(cfg.OpenDuration - time.Millisecond)
	if b.Allow() {
		t.Error("Allow = true before OpenDuration elapsed")
	}

	// After OpenDuration: Allow flips to half-open and returns true.
	clk.Advance(2 * time.Millisecond)
	if !b.Allow() {
		t.Error("Allow = false after OpenDuration elapsed")
	}
	if b.Status() != CircuitHalfOpen {
		t.Errorf("Status = %q, want half-open", b.Status())
	}
}

func TestCircuitBreaker_HalfOpenSuccessClosesAtThreshold(t *testing.T) {
	clk := newBreakerClock()
	cfg := defaultBreakerConfig() // SuccessThreshold=2, HalfOpenMaxAttempts=3
	b := newBreaker(cfg, clk.Now)
	for i := 0; i < cfg.FailureThreshold; i++ {
		b.OnFailure()
	}
	clk.Advance(cfg.OpenDuration + time.Millisecond)

	// Trigger transition into half-open.
	if !b.Allow() {
		t.Fatal("Allow = false at expected half-open transition")
	}
	b.OnSuccess()
	if b.Status() != CircuitHalfOpen {
		t.Errorf("Status after 1 success = %q, want still half-open", b.Status())
	}
	if !b.Allow() {
		t.Fatal("Allow = false on second half-open attempt")
	}
	b.OnSuccess()
	if b.Status() != CircuitClosed {
		t.Errorf("Status after %d successes = %q, want closed", cfg.SuccessThreshold, b.Status())
	}
}

func TestCircuitBreaker_HalfOpenFailureReopens(t *testing.T) {
	clk := newBreakerClock()
	cfg := defaultBreakerConfig()
	b := newBreaker(cfg, clk.Now)
	for i := 0; i < cfg.FailureThreshold; i++ {
		b.OnFailure()
	}
	clk.Advance(cfg.OpenDuration + time.Millisecond)
	_ = b.Allow() // → half-open
	b.OnSuccess() // partial progress

	// Single failure during half-open trips the breaker.
	b.OnFailure()
	if b.Status() != CircuitOpen {
		t.Errorf("Status after half-open failure = %q, want open", b.Status())
	}
	// And the OpenDuration timer is reset — Allow stays false until
	// another OpenDuration passes.
	clk.Advance(cfg.OpenDuration / 2)
	if b.Allow() {
		t.Error("Allow = true before new OpenDuration elapsed")
	}
}

func TestCircuitBreaker_HalfOpenAttemptLimit(t *testing.T) {
	clk := newBreakerClock()
	cfg := defaultBreakerConfig()
	cfg.SuccessThreshold = 5
	cfg.HalfOpenMaxAttempts = 3
	// HalfOpenMax < SuccessThreshold would fail Validate, but newBreaker
	// trusts callers — test exercises Allow's gating directly.
	b := newBreaker(cfg, clk.Now)
	for i := 0; i < cfg.FailureThreshold; i++ {
		b.OnFailure()
	}
	clk.Advance(cfg.OpenDuration + time.Millisecond)

	// Three Allow() calls should succeed (= HalfOpenMaxAttempts).
	for i := 0; i < cfg.HalfOpenMaxAttempts; i++ {
		if !b.Allow() {
			t.Errorf("Allow[%d] = false, expected true within HalfOpenMaxAttempts", i)
		}
	}
	// Fourth call exceeds the limit — gated.
	if b.Allow() {
		t.Error("Allow past HalfOpenMaxAttempts returned true")
	}
}

func TestCircuitBreaker_OpenIgnoresOnSuccessOnFailure(t *testing.T) {
	clk := newBreakerClock()
	cfg := defaultBreakerConfig()
	b := newBreaker(cfg, clk.Now)
	for i := 0; i < cfg.FailureThreshold; i++ {
		b.OnFailure()
	}
	if b.Status() != CircuitOpen {
		t.Fatalf("setup: Status = %q, want open", b.Status())
	}

	openedAtBefore := time.Time{}
	if cb, ok := b.(*circuitBreaker); ok {
		cb.mu.Lock()
		openedAtBefore = cb.openedAt
		cb.mu.Unlock()
	}

	// Success and failure events while open must not move state and
	// must not re-stamp openedAt (which would extend the open
	// window incorrectly).
	b.OnSuccess()
	b.OnFailure()

	if b.Status() != CircuitOpen {
		t.Errorf("Status after open-state events = %q, want still open", b.Status())
	}
	if cb, ok := b.(*circuitBreaker); ok {
		cb.mu.Lock()
		got := cb.openedAt
		cb.mu.Unlock()
		if !got.Equal(openedAtBefore) {
			t.Errorf("openedAt drifted under open-state events: %v -> %v", openedAtBefore, got)
		}
	}
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	cfg := defaultBreakerConfig()
	cfg.FailureThreshold = 100
	cfg.SuccessThreshold = 50
	cfg.HalfOpenMaxAttempts = 100
	b := newBreaker(cfg, time.Now)

	const goroutines = 8
	const iterations = 200
	var wg sync.WaitGroup
	wg.Add(goroutines)
	var ops atomic.Int64
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				switch i % 3 {
				case 0:
					_ = b.Allow()
				case 1:
					b.OnSuccess()
				default:
					b.OnFailure()
				}
				ops.Add(1)
			}
		}()
	}
	wg.Wait()

	if got, want := ops.Load(), int64(goroutines*iterations); got != want {
		t.Errorf("ops counter = %d, want %d", got, want)
	}
	// Status must be one of the three valid states (no torn read).
	switch b.Status() {
	case CircuitClosed, CircuitOpen, CircuitHalfOpen:
	default:
		t.Errorf("torn Status = %q", b.Status())
	}
}

func TestNewBreakerFactory_NilNow(t *testing.T) {
	cfg := defaultBreakerConfig()
	b := newBreaker(cfg, nil)
	// Should default to time.Now without panicking; just exercise.
	if !b.Allow() {
		t.Error("Allow = false on fresh breaker")
	}
}
