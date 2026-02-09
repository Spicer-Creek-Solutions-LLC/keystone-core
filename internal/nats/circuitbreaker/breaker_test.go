package circuitbreaker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
)

func TestState_String(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half-open"},
		{State(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.expected {
			t.Errorf("State(%d).String() = %v, want %v", tt.state, got, tt.expected)
		}
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.MaxFailures <= 0 {
		t.Error("MaxFailures should be positive")
	}
	if config.Timeout <= 0 {
		t.Error("Timeout should be positive")
	}
	if config.HalfOpenMaxRequests <= 0 {
		t.Error("HalfOpenMaxRequests should be positive")
	}
}

func TestCounts_FailureRate(t *testing.T) {
	tests := []struct {
		counts   Counts
		expected float64
	}{
		{Counts{Requests: 0}, 0},
		{Counts{Requests: 10, Failures: 5}, 0.5},
		{Counts{Requests: 10, Failures: 0}, 0},
		{Counts{Requests: 10, Failures: 10}, 1.0},
	}

	for _, tt := range tests {
		if got := tt.counts.FailureRate(); got != tt.expected {
			t.Errorf("FailureRate() = %v, want %v", got, tt.expected)
		}
	}
}

func TestBreaker_InitialState(t *testing.T) {
	config := DefaultConfig()
	breaker := NewBreaker(config)

	if breaker.State() != StateClosed {
		t.Errorf("Initial state = %v, want closed", breaker.State())
	}
}

func TestBreaker_Allow_Closed(t *testing.T) {
	breaker := NewBreaker(DefaultConfig())

	for i := 0; i < 10; i++ {
		if err := breaker.Allow(); err != nil {
			t.Errorf("Allow() in closed state should not fail: %v", err)
		}
	}
}

func TestBreaker_TransitionToOpen(t *testing.T) {
	config := DefaultConfig()
	config.MaxFailures = 3

	breaker := NewBreaker(config)

	// Record failures to trip the breaker
	for i := 0; i < 3; i++ {
		breaker.Allow()
		breaker.RecordFailure()
	}

	if breaker.State() != StateOpen {
		t.Errorf("State = %v, want open after %d failures", breaker.State(), config.MaxFailures)
	}

	// Should be rejected
	err := breaker.Allow()
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("Allow() = %v, want ErrCircuitOpen", err)
	}
}

func TestBreaker_TransitionToHalfOpen(t *testing.T) {
	config := DefaultConfig()
	config.MaxFailures = 2
	config.Timeout = 50 * time.Millisecond

	breaker := NewBreaker(config)

	// Trip the breaker
	breaker.Allow()
	breaker.RecordFailure()
	breaker.Allow()
	breaker.RecordFailure()

	if breaker.State() != StateOpen {
		t.Fatalf("State = %v, want open", breaker.State())
	}

	start := time.Now()
	if err := helpers.WaitForTimeout(200*time.Millisecond, 5*time.Millisecond, func() (bool, error) {
		return time.Since(start) >= config.Timeout, nil
	}); err != nil {
		t.Fatalf("Timeout not reached: %v", err)
	}

	// Should transition to half-open
	err := breaker.Allow()
	if err != nil {
		t.Errorf("Allow() after timeout should succeed: %v", err)
	}

	if breaker.State() != StateHalfOpen {
		t.Errorf("State = %v, want half-open", breaker.State())
	}
}

func TestBreaker_RecoveryFromHalfOpen(t *testing.T) {
	config := DefaultConfig()
	config.MaxFailures = 2
	config.Timeout = 10 * time.Millisecond
	config.SuccessThreshold = 2

	breaker := NewBreaker(config)

	// Trip the breaker
	breaker.Allow()
	breaker.RecordFailure()
	breaker.Allow()
	breaker.RecordFailure()

	start := time.Now()
	if err := helpers.WaitForTimeout(200*time.Millisecond, 5*time.Millisecond, func() (bool, error) {
		return time.Since(start) >= config.Timeout, nil
	}); err != nil {
		t.Fatalf("Timeout not reached: %v", err)
	}

	// Trigger transition to half-open
	breaker.Allow()

	// Record successes to recover
	breaker.RecordSuccess()
	breaker.Allow()
	breaker.RecordSuccess()

	if breaker.State() != StateClosed {
		t.Errorf("State = %v, want closed after recovery", breaker.State())
	}
}

func TestBreaker_FailureInHalfOpen(t *testing.T) {
	config := DefaultConfig()
	config.MaxFailures = 2
	config.Timeout = 10 * time.Millisecond

	breaker := NewBreaker(config)

	// Trip the breaker
	breaker.Allow()
	breaker.RecordFailure()
	breaker.Allow()
	breaker.RecordFailure()

	start := time.Now()
	if err := helpers.WaitForTimeout(200*time.Millisecond, 5*time.Millisecond, func() (bool, error) {
		return time.Since(start) >= config.Timeout, nil
	}); err != nil {
		t.Fatalf("Timeout not reached: %v", err)
	}

	// Trigger transition to half-open
	breaker.Allow()

	// Record failure
	breaker.RecordFailure()

	if breaker.State() != StateOpen {
		t.Errorf("State = %v, want open after failure in half-open", breaker.State())
	}
}

func TestBreaker_HalfOpenMaxRequests(t *testing.T) {
	config := DefaultConfig()
	config.MaxFailures = 2
	config.Timeout = 10 * time.Millisecond
	config.HalfOpenMaxRequests = 2

	breaker := NewBreaker(config)

	// Trip the breaker
	breaker.Allow()
	breaker.RecordFailure()
	breaker.Allow()
	breaker.RecordFailure()

	start := time.Now()
	if err := helpers.WaitForTimeout(200*time.Millisecond, 5*time.Millisecond, func() (bool, error) {
		return time.Since(start) >= config.Timeout, nil
	}); err != nil {
		t.Fatalf("Timeout not reached: %v", err)
	}

	// Use up allowed requests
	breaker.Allow() // triggers half-open
	breaker.Allow() // 2nd request

	// 3rd request should fail
	err := breaker.Allow()
	if !errors.Is(err, ErrTooManyRequests) {
		t.Errorf("Allow() = %v, want ErrTooManyRequests", err)
	}
}

func TestBreaker_Execute(t *testing.T) {
	config := DefaultConfig()
	config.MaxFailures = 2

	breaker := NewBreaker(config)

	// Successful execution
	err := breaker.Execute(func() error {
		return nil
	})
	if err != nil {
		t.Errorf("Execute() with success = %v", err)
	}

	counts := breaker.Counts()
	if counts.Successes != 1 {
		t.Errorf("Successes = %d, want 1", counts.Successes)
	}

	// Failed execution
	testErr := errors.New("test error")
	err = breaker.Execute(func() error {
		return testErr
	})
	if !errors.Is(err, testErr) {
		t.Errorf("Execute() with failure = %v, want %v", err, testErr)
	}

	counts = breaker.Counts()
	if counts.Failures != 1 {
		t.Errorf("Failures = %d, want 1", counts.Failures)
	}
}

func TestBreaker_ExecuteWithContext(t *testing.T) {
	config := DefaultConfig()
	breaker := NewBreaker(config)

	ctx := context.Background()

	err := breaker.ExecuteWithContext(ctx, func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Errorf("ExecuteWithContext() = %v", err)
	}

	// Test timeout recording
	err = breaker.ExecuteWithContext(ctx, func(ctx context.Context) error {
		return context.DeadlineExceeded
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("ExecuteWithContext() = %v, want DeadlineExceeded", err)
	}

	counts := breaker.Counts()
	if counts.Timeouts != 1 {
		t.Errorf("Timeouts = %d, want 1", counts.Timeouts)
	}
}

func TestBreaker_Reset(t *testing.T) {
	config := DefaultConfig()
	config.MaxFailures = 2

	breaker := NewBreaker(config)

	// Trip the breaker
	breaker.Allow()
	breaker.RecordFailure()
	breaker.Allow()
	breaker.RecordFailure()

	if breaker.State() != StateOpen {
		t.Fatalf("State = %v, want open", breaker.State())
	}

	breaker.Reset()

	if breaker.State() != StateClosed {
		t.Errorf("State after reset = %v, want closed", breaker.State())
	}

	counts := breaker.Counts()
	if counts.Failures != 0 {
		t.Errorf("Failures after reset = %d, want 0", counts.Failures)
	}
}

func TestBreaker_StateChangeListener(t *testing.T) {
	config := DefaultConfig()
	config.MaxFailures = 2

	breaker := NewBreaker(config)

	var events []*StateChangeEvent
	var mu sync.Mutex

	breaker.AddListener(func(event *StateChangeEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	// Trip the breaker
	breaker.Allow()
	breaker.RecordFailure()
	breaker.Allow()
	breaker.RecordFailure()

	mu.Lock()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}
	if events[0].From != StateClosed {
		t.Errorf("From = %v, want closed", events[0].From)
	}
	if events[0].To != StateOpen {
		t.Errorf("To = %v, want open", events[0].To)
	}
	mu.Unlock()
}

func TestBreaker_FailureRateThreshold(t *testing.T) {
	config := DefaultConfig()
	config.MaxFailures = 100 // High max failures
	config.FailureRateThreshold = 0.5
	config.MinRequests = 4

	breaker := NewBreaker(config)

	// Record 2 successes and 2 failures (50% failure rate)
	breaker.Allow()
	breaker.RecordSuccess()
	breaker.Allow()
	breaker.RecordSuccess()
	breaker.Allow()
	breaker.RecordFailure()
	breaker.Allow()
	breaker.RecordFailure()

	if breaker.State() != StateOpen {
		t.Errorf("State = %v, want open at 50%% failure rate", breaker.State())
	}
}

func TestBreaker_ConcurrentAccess(t *testing.T) {
	config := DefaultConfig()
	config.MaxFailures = 100

	breaker := NewBreaker(config)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			breaker.Allow()
			breaker.RecordSuccess()
		}()
	}
	wg.Wait()

	counts := breaker.Counts()
	if counts.Successes != 100 {
		t.Errorf("Successes = %d, want 100", counts.Successes)
	}
}

func TestRegistry(t *testing.T) {
	registry := NewRegistry(DefaultConfig())

	// Get creates new breaker
	b1 := registry.Get("service1")
	if b1.Name() != "service1" {
		t.Errorf("Name = %v, want service1", b1.Name())
	}

	// Get returns same breaker
	b2 := registry.Get("service1")
	if b1 != b2 {
		t.Error("Get should return same breaker instance")
	}

	// List all breakers
	breakers := registry.List()
	if len(breakers) != 1 {
		t.Errorf("List length = %d, want 1", len(breakers))
	}

	// Remove breaker
	registry.Remove("service1")
	breakers = registry.List()
	if len(breakers) != 0 {
		t.Errorf("List length after remove = %d, want 0", len(breakers))
	}
}

func TestRegistry_GetWithConfig(t *testing.T) {
	registry := NewRegistry(DefaultConfig())

	config := &Config{
		MaxFailures: 10,
		Timeout:     time.Minute,
	}

	b := registry.GetWithConfig("custom", config)
	if b.config.MaxFailures != 10 {
		t.Errorf("MaxFailures = %d, want 10", b.config.MaxFailures)
	}
}

func TestRegistry_Stats(t *testing.T) {
	registry := NewRegistry(DefaultConfig())

	b := registry.Get("test")
	b.RecordSuccess()
	b.RecordFailure()

	stats := registry.Stats()
	if len(stats) != 1 {
		t.Errorf("Stats length = %d, want 1", len(stats))
	}

	testStats := stats["test"]
	if testStats.State != "closed" {
		t.Errorf("State = %v, want closed", testStats.State)
	}
	if testStats.Counts.Successes != 1 {
		t.Errorf("Successes = %d, want 1", testStats.Counts.Successes)
	}
}

func TestRegistry_ResetAll(t *testing.T) {
	config := DefaultConfig()
	config.MaxFailures = 2

	registry := NewRegistry(config)

	// Trip both breakers
	b1 := registry.Get("service1")
	b2 := registry.Get("service2")

	b1.RecordFailure()
	b1.RecordFailure()
	b2.RecordFailure()
	b2.RecordFailure()

	if b1.State() != StateOpen || b2.State() != StateOpen {
		t.Fatal("Both breakers should be open")
	}

	registry.ResetAll()

	if b1.State() != StateClosed || b2.State() != StateClosed {
		t.Error("Both breakers should be closed after reset")
	}
}

func TestWrap(t *testing.T) {
	config := DefaultConfig()
	breaker := NewBreaker(config)

	wrapped := Wrap(breaker, func() (string, error) {
		return "result", nil
	})

	result, err := wrapped.Execute()
	if err != nil {
		t.Errorf("Execute() error = %v", err)
	}
	if result != "result" {
		t.Errorf("Execute() result = %v, want result", result)
	}
}

func TestWindowedBreaker(t *testing.T) {
	config := DefaultConfig()
	config.MaxFailures = 3
	config.MinRequests = 3
	config.WindowSize = time.Second
	config.Timeout = 50 * time.Millisecond

	breaker := NewWindowedBreaker(config)

	// Record failures to trip
	breaker.Allow()
	breaker.RecordFailure()
	breaker.Allow()
	breaker.RecordFailure()
	breaker.Allow()
	breaker.RecordFailure()

	if breaker.State() != StateOpen {
		t.Errorf("State = %v, want open", breaker.State())
	}

	start := time.Now()
	if err := helpers.WaitForTimeout(200*time.Millisecond, 5*time.Millisecond, func() (bool, error) {
		return time.Since(start) >= config.Timeout, nil
	}); err != nil {
		t.Fatalf("Timeout not reached: %v", err)
	}

	// Should transition to half-open
	err := breaker.Allow()
	if err != nil {
		t.Errorf("Allow() after timeout = %v", err)
	}

	if breaker.State() != StateHalfOpen {
		t.Errorf("State = %v, want half-open", breaker.State())
	}
}

func TestWindowedBreaker_WindowExpiry(t *testing.T) {
	config := DefaultConfig()
	config.MaxFailures = 3
	config.MinRequests = 3
	config.WindowSize = 50 * time.Millisecond

	breaker := NewWindowedBreaker(config)

	// Record some failures
	breaker.Allow()
	breaker.RecordFailure()
	breaker.Allow()
	breaker.RecordFailure()

	start := time.Now()
	if err := helpers.WaitForTimeout(200*time.Millisecond, 5*time.Millisecond, func() (bool, error) {
		return time.Since(start) >= config.WindowSize, nil
	}); err != nil {
		t.Fatalf("Window did not expire: %v", err)
	}

	// Record another failure - should not trip because old failures expired
	breaker.Allow()
	breaker.RecordFailure()

	if breaker.State() != StateClosed {
		t.Errorf("State = %v, want closed (old failures should have expired)", breaker.State())
	}
}

func TestNATSBreaker(t *testing.T) {
	config := DefaultConfig()
	config.MaxFailures = 2

	nb := NewNATSBreaker(config)

	// Test subject breakers
	err := nb.AllowPublish("test.subject")
	if err != nil {
		t.Errorf("AllowPublish() = %v", err)
	}

	nb.RecordPublishSuccess("test.subject")
	nb.RecordPublishFailure("test.subject")
	nb.RecordPublishFailure("test.subject")

	// Should be open now
	err = nb.AllowPublish("test.subject")
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("AllowPublish() = %v, want ErrCircuitOpen", err)
	}

	// Different subject should still work
	err = nb.AllowPublish("other.subject")
	if err != nil {
		t.Errorf("AllowPublish(other) = %v", err)
	}

	// Check stats
	stats := nb.Stats()
	if len(stats) != 2 {
		t.Errorf("Stats length = %d, want 2", len(stats))
	}
}

func TestBreaker_Rejections(t *testing.T) {
	config := DefaultConfig()
	config.MaxFailures = 1
	config.Timeout = time.Hour // Long timeout

	breaker := NewBreaker(config)

	// Trip the breaker
	breaker.Allow()
	breaker.RecordFailure()

	// Attempts should be rejected and counted
	for i := 0; i < 5; i++ {
		breaker.Allow()
	}

	counts := breaker.Counts()
	if counts.Rejections != 5 {
		t.Errorf("Rejections = %d, want 5", counts.Rejections)
	}
}

func TestBreaker_CircuitOpenError(t *testing.T) {
	config := DefaultConfig()
	config.MaxFailures = 1

	breaker := NewBreaker(config)

	// Trip the breaker
	breaker.Allow()
	breaker.RecordFailure()

	// Execute should return circuit open error
	var callCount int32
	err := breaker.Execute(func() error {
		atomic.AddInt32(&callCount, 1)
		return nil
	})

	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("Execute() = %v, want ErrCircuitOpen", err)
	}

	if atomic.LoadInt32(&callCount) != 0 {
		t.Error("Function should not have been called")
	}
}
