package nats

import (
	"sync"
	"time"

	"go.keystone-core.io/keystone-core/internal/config"
)

// Breaker is the per-endpoint circuit breaker contract used by
// ConnectionManager. The state machine is closed → open → half-open
// → closed (PROJECT-DETAILS §4.2). Task 7 implements the real
// circuitBreaker; the noopBreaker remains the disabled-config path.
//
// v1.0 wiring: ConnectionManager calls OnSuccess / OnFailure from
// the disconnect/reconnect callbacks, and Health degrades when
// every endpoint's breaker is OPEN. Active dial-time eviction
// (skipping OPEN endpoints when picking the next reconnect target)
// requires replacing nats.go's native multi-URL failover with a
// per-endpoint dial loop — a substantial refactor deferred to v0.x.
type Breaker interface {
	// Allow reports whether ConnectionManager should attempt this
	// endpoint right now. Closed → true; Open → false until
	// OpenDuration elapses, at which point the next call transitions
	// to HalfOpen and returns true; HalfOpen → true for ≤
	// HalfOpenMaxAttempts before requiring a state resolution.
	Allow() bool

	// OnSuccess records a successful operation. Closed → noop.
	// HalfOpen → bumps the success counter; at SuccessThreshold the
	// breaker closes. Open → noop (we shouldn't be operating).
	OnSuccess()

	// OnFailure records a failure. Closed → bumps the failure
	// counter; at FailureThreshold the breaker opens. HalfOpen → any
	// failure reverts to Open and resets the OpenDuration timer.
	// Open → noop.
	OnFailure()

	// Status returns the current circuit state for Snapshot.
	Status() CircuitStatus
}

// noopBreaker permits every operation and reports CircuitClosed.
// Used when CircuitBreakerConfig.Enabled is false — operators may
// disable the breaker for trial / debugging deployments.
type noopBreaker struct{}

func (noopBreaker) Allow() bool           { return true }
func (noopBreaker) OnSuccess()            {}
func (noopBreaker) OnFailure()            {}
func (noopBreaker) Status() CircuitStatus { return CircuitClosed }

// circuitBreaker is the standard hystrix-style implementation. All
// state is guarded by mu; methods are safe to call concurrently.
//
// State invariants:
//   - closed:    consecutiveFail counts toward FailureThreshold;
//                openedAt unset; halfOpen counters zero.
//   - open:      openedAt is non-zero; Allow() flips to half-open
//                only after OpenDuration elapses.
//   - half-open: halfOpenAttempt counts attempts allowed (capped at
//                HalfOpenMaxAttempts); halfOpenSuccess counts
//                successes (closes at SuccessThreshold). A single
//                failure reverts to open.
type circuitBreaker struct {
	cfg config.CircuitBreakerConfig
	now func() time.Time

	mu              sync.Mutex
	state           CircuitStatus
	consecutiveFail int
	halfOpenAttempt int
	halfOpenSuccess int
	openedAt        time.Time
}

// newBreaker builds a Breaker for one endpoint. Returns noopBreaker
// when the config is disabled — ConnectionManager doesn't need to
// nil-check the result and the always-allow contract is preserved.
func newBreaker(cfg config.CircuitBreakerConfig, now func() time.Time) Breaker {
	if !cfg.Enabled {
		return noopBreaker{}
	}
	if now == nil {
		now = time.Now
	}
	return &circuitBreaker{
		cfg:   cfg,
		now:   now,
		state: CircuitClosed,
	}
}

// Allow drives the time-based open → half-open transition. Each
// half-open attempt slot is consumed on Allow (whether or not a
// subsequent success/failure resolves it), so a runaway caller
// cannot starve the breaker.
func (b *circuitBreaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		if b.now().Sub(b.openedAt) < b.cfg.OpenDuration {
			return false
		}
		b.toHalfOpenLocked()
		// fall through to the half-open branch
		fallthrough
	case CircuitHalfOpen:
		if b.halfOpenAttempt >= b.cfg.HalfOpenMaxAttempts {
			return false
		}
		b.halfOpenAttempt++
		return true
	default:
		return false
	}
}

// OnSuccess records a successful operation.
func (b *circuitBreaker) OnSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case CircuitClosed:
		b.consecutiveFail = 0
	case CircuitHalfOpen:
		b.halfOpenSuccess++
		if b.halfOpenSuccess >= b.cfg.SuccessThreshold {
			b.toClosedLocked()
		}
	}
}

// OnFailure records a failure.
func (b *circuitBreaker) OnFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case CircuitClosed:
		b.consecutiveFail++
		if b.consecutiveFail >= b.cfg.FailureThreshold {
			b.toOpenLocked()
		}
	case CircuitHalfOpen:
		// Any failure during half-open trips the breaker again.
		// Re-stamp openedAt so the OpenDuration timer restarts.
		b.toOpenLocked()
	}
}

// Status returns the current state.
func (b *circuitBreaker) Status() CircuitStatus {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

func (b *circuitBreaker) toOpenLocked() {
	b.state = CircuitOpen
	b.openedAt = b.now()
	b.consecutiveFail = 0
	b.halfOpenAttempt = 0
	b.halfOpenSuccess = 0
}

func (b *circuitBreaker) toHalfOpenLocked() {
	b.state = CircuitHalfOpen
	b.halfOpenAttempt = 0
	b.halfOpenSuccess = 0
}

func (b *circuitBreaker) toClosedLocked() {
	b.state = CircuitClosed
	b.consecutiveFail = 0
	b.halfOpenAttempt = 0
	b.halfOpenSuccess = 0
	b.openedAt = time.Time{}
}
