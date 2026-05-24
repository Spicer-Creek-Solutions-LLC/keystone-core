// SPDX-License-Identifier: Apache-2.0

package outbound

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrCircuitOpen is returned by [CircuitBreaker.Deliver] when the
// per-endpoint breaker is open (fast-fail). Manager records this as
// a regular failure on the [DeliveryRecord] — §4.14's status set
// does not include an "open" state.
var ErrCircuitOpen = errors.New("outbound: circuit breaker open")

// §4.14 defaults.
const (
	DefaultFailureThreshold  = 5
	DefaultOpenDuration      = 30 * time.Second
	DefaultHalfOpenSuccesses = 2
)

// cbState is the per-endpoint state of one breaker.
type cbState int

const (
	cbClosed cbState = iota
	cbOpen
	cbHalfOpen
)

// breakerState holds the per-endpoint counters + mutex. One instance
// per sub.URL, stored in [CircuitBreaker.breakers].
type breakerState struct {
	mu           sync.Mutex
	state        cbState
	failures     int
	halfOpenWins int
	openedAt     time.Time
}

// CircuitBreaker wraps a [Dispatcher] with a per-endpoint state
// machine: Closed → Open after FailureThreshold consecutive failures
// → HalfOpen after OpenDuration → Closed after HalfOpenSuccesses
// consecutive successes (any failure in HalfOpen sends it back to
// Open). Implements [Dispatcher] so it composes orthogonally with
// the task-14 retry loop in Manager.
//
// Per §4.14, "circuit-breaker false positives — network timeouts
// count as failures; transient 4xx can briefly flip a healthy
// receiver to open" is the documented v1.0 trade-off: failure here
// is exactly `err != nil` from the inner dispatcher.
type CircuitBreaker struct {
	Inner             Dispatcher
	FailureThreshold  int           // 0 → DefaultFailureThreshold
	OpenDuration      time.Duration // 0 → DefaultOpenDuration
	HalfOpenSuccesses int           // 0 → DefaultHalfOpenSuccesses
	Now               func() time.Time

	breakers sync.Map // string (sub.URL) → *breakerState
}

func (cb *CircuitBreaker) failureThreshold() int {
	if cb.FailureThreshold <= 0 {
		return DefaultFailureThreshold
	}
	return cb.FailureThreshold
}

func (cb *CircuitBreaker) openDuration() time.Duration {
	if cb.OpenDuration <= 0 {
		return DefaultOpenDuration
	}
	return cb.OpenDuration
}

func (cb *CircuitBreaker) halfOpenSuccesses() int {
	if cb.HalfOpenSuccesses <= 0 {
		return DefaultHalfOpenSuccesses
	}
	return cb.HalfOpenSuccesses
}

func (cb *CircuitBreaker) now() time.Time {
	if cb.Now != nil {
		return cb.Now()
	}
	return time.Now()
}

// state returns (or atomically creates) the breaker entry for url.
// sync.Map.LoadOrStore handles concurrent first-touch.
func (cb *CircuitBreaker) state(url string) *breakerState {
	if v, ok := cb.breakers.Load(url); ok {
		return v.(*breakerState)
	}
	v, _ := cb.breakers.LoadOrStore(url, &breakerState{})
	return v.(*breakerState)
}

// admit checks the gate before calling Inner. Returns (allow, false)
// when the request should fast-fail (Open + still in cooldown).
// Transitions Open → HalfOpen when OpenDuration has elapsed.
func (cb *CircuitBreaker) admit(b *breakerState) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == cbOpen {
		if cb.now().Sub(b.openedAt) >= cb.openDuration() {
			b.state = cbHalfOpen
			b.halfOpenWins = 0
			return true
		}
		return false
	}
	return true
}

// record updates breaker state from the inner call's outcome and
// returns the (statusCode, err) unchanged so callers see the real
// response.
func (cb *CircuitBreaker) record(b *breakerState, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err == nil {
		switch b.state {
		case cbClosed:
			b.failures = 0
		case cbHalfOpen:
			b.halfOpenWins++
			if b.halfOpenWins >= cb.halfOpenSuccesses() {
				b.state = cbClosed
				b.failures = 0
				b.halfOpenWins = 0
			}
		}
		return
	}
	// failure
	switch b.state {
	case cbClosed:
		b.failures++
		if b.failures >= cb.failureThreshold() {
			b.state = cbOpen
			b.openedAt = cb.now()
		}
	case cbHalfOpen:
		// Any failure in HalfOpen re-opens the breaker with a fresh
		// cooldown window — the receiver has demonstrated it is not
		// yet healthy.
		b.state = cbOpen
		b.openedAt = cb.now()
		b.halfOpenWins = 0
	}
}

// Deliver implements [Dispatcher].
func (cb *CircuitBreaker) Deliver(ctx context.Context, sub *Subscription, payload []byte, deliveryID string) (int, error) {
	b := cb.state(sub.URL)
	if !cb.admit(b) {
		return 0, ErrCircuitOpen
	}
	code, err := cb.Inner.Deliver(ctx, sub, payload, deliveryID)
	cb.record(b, err)
	return code, err
}

// compile-time assertion: CircuitBreaker satisfies Dispatcher.
var _ Dispatcher = (*CircuitBreaker)(nil)
