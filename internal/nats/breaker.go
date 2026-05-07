package nats

// Breaker is the per-endpoint circuit breaker contract used by
// ConnectionManager. Task 7 implements the closed→open→half-open→
// closed state machine; this file ships the interface plus a noop
// stub so ConnectionManager (Task 2) can be reviewed without waiting
// for Task 7.
//
// Methods are deliberately tiny: the breaker does not see Endpoint or
// any nats-level type. ConnectionManager owns one breaker per Endpoint
// and threads success/failure events from connect callbacks.
type Breaker interface {
	// Allow reports whether ConnectionManager should attempt this
	// endpoint right now. Closed → true; Open → false; HalfOpen → true
	// for at most HalfOpenMaxAttempts probes.
	Allow() bool

	// OnSuccess records a successful operation. Drives Open→HalfOpen→
	// Closed in the real impl; noop here.
	OnSuccess()

	// OnFailure records a failure. Drives Closed→Open in the real
	// impl; noop here.
	OnFailure()

	// Status returns the current circuit state for Snapshot.
	Status() CircuitStatus
}

// noopBreaker permits every operation and reports CircuitClosed.
// ConnectionManager defaults to this until Task 7 lands the real
// breaker; the swap is a single newBreaker() factory change.
type noopBreaker struct{}

func (noopBreaker) Allow() bool          { return true }
func (noopBreaker) OnSuccess()           {}
func (noopBreaker) OnFailure()           {}
func (noopBreaker) Status() CircuitStatus { return CircuitClosed }

// newBreaker is the factory used by ConnectionManager. Task 7 swaps
// the body for a real state-machine constructor; the rest of the
// package goes through this entry point so the change is local.
func newBreaker() Breaker {
	return noopBreaker{}
}
