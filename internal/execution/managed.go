package execution

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// State is the lifecycle position of a managed execution.
type State int

const (
	StatePending State = iota
	StateRunning
	StateCompleted
	StateFailed
	StateTimeout
	StateCancelled
	StateRetrying
)

// String renders the canonical PROJECT-DETAILS §4.7 form.
func (s State) String() string {
	switch s {
	case StatePending:
		return "PENDING"
	case StateRunning:
		return "RUNNING"
	case StateCompleted:
		return "COMPLETED"
	case StateFailed:
		return "FAILED"
	case StateTimeout:
		return "TIMEOUT"
	case StateCancelled:
		return "CANCELLED"
	case StateRetrying:
		return "RETRYING"
	default:
		return fmt.Sprintf("State(%d)", int(s))
	}
}

// Callbacks observes lifecycle transitions. Any nil hook is skipped
// silently — callers can wire only what they care about.
//
// Sequencing for a successful single attempt:
//
//	OnStarted → OnCompleted
//
// For a retry-then-success:
//
//	OnStarted → OnFailed → OnRetrying(attempt=2, last, delay)
//	          → OnRetry(2) → OnCompleted
//
// For exhausted retries:
//
//	OnStarted → OnFailed → OnRetrying(attempt=2, last, delay)
//	          → OnRetry(2) → OnFailed
//
// OnTimeout / OnCancelled are terminal: no further hooks fire after
// either runs.
type Callbacks struct {
	OnStarted   func()
	OnCompleted func(ExecuteResult)
	OnFailed    func(ExecuteResult)
	OnTimeout   func(ExecuteResult)
	OnCancelled func()
	OnRetrying  func(nextAttempt int, last ExecuteResult, delay time.Duration)
	OnRetry     func(attempt int)
}

// RetryPolicy controls retry-on-failure. Defaults: MaxAttempts=1
// (no retry), InitialBackoff=100ms, BackoffMultiplier=2,
// MaxBackoff=10s.
type RetryPolicy struct {
	MaxAttempts       int
	InitialBackoff    time.Duration
	BackoffMultiplier float64
	MaxBackoff        time.Duration
}

func (p RetryPolicy) effective() RetryPolicy {
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = 1
	}
	if p.InitialBackoff <= 0 {
		p.InitialBackoff = 100 * time.Millisecond
	}
	if p.BackoffMultiplier < 1 {
		p.BackoffMultiplier = 2
	}
	if p.MaxBackoff <= 0 {
		p.MaxBackoff = 10 * time.Second
	}
	return p
}

// backoffFor returns the delay between attempt n and attempt n+1. n is
// the index of the attempt that just failed (1-based). Capped at
// MaxBackoff.
func (p RetryPolicy) backoffFor(attempt int) time.Duration {
	if attempt < 1 {
		return 0
	}
	d := p.InitialBackoff
	for i := 1; i < attempt; i++ {
		next := time.Duration(float64(d) * p.BackoffMultiplier)
		if next > p.MaxBackoff {
			return p.MaxBackoff
		}
		d = next
	}
	if d > p.MaxBackoff {
		return p.MaxBackoff
	}
	return d
}

// Config configures a ManagedExecution.
type Config struct {
	Executor  Executor
	Callbacks Callbacks
	Retry     RetryPolicy
	// Sleep overrides the default ctx-aware sleep. Tests use this to
	// avoid wall-clock waits.
	Sleep func(ctx context.Context, d time.Duration) error
}

// ManagedExecution wraps an Executor with a state machine, observable
// callbacks, and a retry policy. Cheap to construct; safe for
// concurrent Run calls because no per-Run state lives on the receiver.
type ManagedExecution struct {
	exec      Executor
	callbacks Callbacks
	retry     RetryPolicy
	sleep     func(ctx context.Context, d time.Duration) error
}

// New returns a ManagedExecution. The Executor is required; everything
// else has zero-value defaults.
func New(cfg Config) (*ManagedExecution, error) {
	if cfg.Executor == nil {
		return nil, errors.New("execution: managed: nil executor")
	}
	if cfg.Sleep == nil {
		cfg.Sleep = ctxSleep
	}
	return &ManagedExecution{
		exec:      cfg.Executor,
		callbacks: cfg.Callbacks,
		retry:     cfg.Retry.effective(),
		sleep:     cfg.Sleep,
	}, nil
}

// Run executes req under the configured policy. The returned State
// reflects the terminal lifecycle position; the result is the last
// attempt's ExecuteResult (zero-value if the run never reached an
// attempt due to a pre-cancelled context).
func (m *ManagedExecution) Run(ctx context.Context, req ExecuteRequest) (State, ExecuteResult) {
	if ctx.Err() != nil {
		return m.handleCtxFire(ctx, ExecuteResult{})
	}

	var last ExecuteResult
	for attempt := 1; attempt <= m.retry.MaxAttempts; attempt++ {
		if attempt == 1 {
			if cb := m.callbacks.OnStarted; cb != nil {
				cb()
			}
		} else if cb := m.callbacks.OnRetry; cb != nil {
			cb(attempt)
		}

		last = m.exec.Execute(ctx, req)

		if ctx.Err() != nil {
			return m.handleCtxFire(ctx, last)
		}
		if last.TimedOut {
			if cb := m.callbacks.OnTimeout; cb != nil {
				cb(last)
			}
			return StateTimeout, last
		}
		if last.Succeeded() {
			if cb := m.callbacks.OnCompleted; cb != nil {
				cb(last)
			}
			return StateCompleted, last
		}

		if cb := m.callbacks.OnFailed; cb != nil {
			cb(last)
		}
		if attempt >= m.retry.MaxAttempts {
			return StateFailed, last
		}

		delay := m.retry.backoffFor(attempt)
		if cb := m.callbacks.OnRetrying; cb != nil {
			cb(attempt+1, last, delay)
		}
		if err := m.sleep(ctx, delay); err != nil {
			return m.handleCtxFire(ctx, last)
		}
	}
	return StateFailed, last
}

// handleCtxFire converts a fired context into the right terminal
// state + callback. DeadlineExceeded → TIMEOUT; Canceled → CANCELLED.
func (m *ManagedExecution) handleCtxFire(ctx context.Context, last ExecuteResult) (State, ExecuteResult) {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		if cb := m.callbacks.OnTimeout; cb != nil {
			cb(last)
		}
		return StateTimeout, last
	}
	if cb := m.callbacks.OnCancelled; cb != nil {
		cb()
	}
	return StateCancelled, last
}

// ctxSleep returns nil after d, or ctx.Err() if the context fires
// first. d ≤ 0 returns immediately.
func ctxSleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
