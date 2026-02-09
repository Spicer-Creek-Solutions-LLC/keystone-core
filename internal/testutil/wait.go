// Package testutil provides testing utilities for condition-based waiting.
package testutil

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/wait"
)

// DefaultTimeout is the default timeout for wait operations.
const DefaultTimeout = 5 * time.Second

// DefaultPollInterval is the default interval between condition checks.
const DefaultPollInterval = 10 * time.Millisecond

// WaitConfig configures wait behavior.
type WaitConfig struct {
	Timeout      time.Duration
	PollInterval time.Duration
}

// DefaultWaitConfig returns the default wait configuration.
func DefaultWaitConfig() WaitConfig {
	return WaitConfig{
		Timeout:      DefaultTimeout,
		PollInterval: DefaultPollInterval,
	}
}

// WaitFor waits for a condition to become true within the timeout.
// Returns an error if the condition is not met within the timeout.
func WaitFor(condition func() bool, opts ...WaitConfig) error {
	cfg := DefaultWaitConfig()
	if len(opts) > 0 {
		cfg = opts[0]
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()
	return wait.ForCondition(ctx, cfg.PollInterval, func() (bool, error) {
		return condition(), nil
	})
}

// WaitForWithContext waits for a condition with a provided context.
func WaitForWithContext(ctx context.Context, condition func() bool, pollInterval time.Duration) error {
	return wait.ForCondition(ctx, pollInterval, func() (bool, error) {
		return condition(), nil
	})
}

// Eventually retries a function until it returns true or times out.
// Unlike WaitFor, this is useful when the check itself may need retrying.
func Eventually(f func() bool, timeout, interval time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if f() {
			return true
		}
		wait.ForDuration(interval)
	}
	return false
}

// Never checks that a condition never becomes true within the timeout.
// Returns true if the condition remained false.
func Never(condition func() bool, duration time.Duration) bool {
	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		if condition() {
			return false
		}
		wait.ForDuration(10 * time.Millisecond)
	}
	return true
}

// Counter is a thread-safe counter for test coordination.
type Counter struct {
	mu    sync.Mutex
	cond  *sync.Cond
	value int
}

// NewCounter creates a new counter.
func NewCounter() *Counter {
	c := &Counter{}
	c.cond = sync.NewCond(&c.mu)
	return c
}

// Inc increments the counter and signals waiters.
func (c *Counter) Inc() {
	c.mu.Lock()
	c.value++
	c.cond.Broadcast()
	c.mu.Unlock()
}

// Add adds n to the counter and signals waiters.
func (c *Counter) Add(n int) {
	c.mu.Lock()
	c.value += n
	c.cond.Broadcast()
	c.mu.Unlock()
}

// Value returns the current counter value.
func (c *Counter) Value() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.value
}

// Reset resets the counter to zero.
func (c *Counter) Reset() {
	c.mu.Lock()
	c.value = 0
	c.mu.Unlock()
}

// WaitForValue waits for the counter to reach at least the target value.
func (c *Counter) WaitForValue(target int, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		c.mu.Lock()
		for c.value < target {
			c.cond.Wait()
		}
		c.mu.Unlock()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		c.cond.Broadcast() // Wake up waiting goroutine
		return fmt.Errorf("counter did not reach %d within %v (current: %d)", target, timeout, c.Value())
	}
}

// Signal is a thread-safe signal for test coordination.
type Signal struct {
	ch   chan struct{}
	once sync.Once
}

// NewSignal creates a new signal.
func NewSignal() *Signal {
	return &Signal{
		ch: make(chan struct{}),
	}
}

// Send sends the signal. Can only be called once.
func (s *Signal) Send() {
	s.once.Do(func() {
		close(s.ch)
	})
}

// Wait waits for the signal with timeout.
func (s *Signal) Wait(timeout time.Duration) error {
	if wait.ForSignal(s.ch, timeout) {
		return fmt.Errorf("signal not received within %v", timeout)
	}
	return nil
}

// Done returns a channel that is closed when the signal is sent.
func (s *Signal) Done() <-chan struct{} {
	return s.ch
}

// Barrier synchronizes multiple goroutines at a point.
type Barrier struct {
	mu      sync.Mutex
	cond    *sync.Cond
	count   int
	target  int
	release bool
}

// NewBarrier creates a barrier for n goroutines.
func NewBarrier(n int) *Barrier {
	b := &Barrier{target: n}
	b.cond = sync.NewCond(&b.mu)
	return b
}

// Wait waits at the barrier. When n goroutines have called Wait,
// all are released.
func (b *Barrier) Wait(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		b.mu.Lock()
		b.count++
		if b.count >= b.target {
			b.release = true
			b.cond.Broadcast()
		}
		for !b.release {
			b.cond.Wait()
		}
		b.mu.Unlock()
		done <- nil
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		b.cond.Broadcast()
		return fmt.Errorf("barrier wait timed out after %v", timeout)
	}
}

// WaitGroup wraps sync.WaitGroup with timeout support.
type WaitGroup struct {
	wg sync.WaitGroup
}

// Add adds to the wait group counter.
func (w *WaitGroup) Add(n int) {
	w.wg.Add(n)
}

// Done decrements the counter.
func (w *WaitGroup) Done() {
	w.wg.Done()
}

// WaitWithTimeout waits for all goroutines with a timeout.
func (w *WaitGroup) WaitWithTimeout(timeout time.Duration) error {
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()

	if wait.ForSignal(done, timeout) {
		return fmt.Errorf("wait group did not complete within %v", timeout)
	}
	return nil
}

// Poll continuously checks a condition at the given interval until
// the condition returns true or the context is cancelled.
func Poll(ctx context.Context, interval time.Duration, condition func() (bool, error)) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		done, err := condition()
		if err != nil {
			return err
		}
		if done {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// RetryWithBackoff retries a function with exponential backoff.
func RetryWithBackoff(ctx context.Context, maxAttempts int, initialDelay time.Duration, f func() error) error {
	delay := initialDelay
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		err := f()
		if err == nil {
			return nil
		}
		lastErr = err

		if err := wait.ForContext(ctx, delay); err != nil {
			return err
		}
		delay *= 2
	}

	return fmt.Errorf("failed after %d attempts: %w", maxAttempts, lastErr)
}
