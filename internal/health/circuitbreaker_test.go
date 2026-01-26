package health

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
)

func TestNewCircuitBreaker(t *testing.T) {
	cb := NewCircuitBreaker(nil)
	if cb == nil {
		t.Fatal("NewCircuitBreaker returned nil")
	}

	if cb.State() != StateClosed {
		t.Errorf("Expected initial state %s, got %s", StateClosed, cb.State())
	}
}

func TestCircuitBreakerAllow(t *testing.T) {
	cb := NewCircuitBreaker(nil)

	err := cb.Allow()
	if err != nil {
		t.Errorf("Allow should not error in closed state: %v", err)
	}
}

func TestCircuitBreakerRecordSuccess(t *testing.T) {
	cb := NewCircuitBreaker(nil)

	cb.RecordSuccess()

	if cb.State() != StateClosed {
		t.Errorf("Expected state %s after success, got %s", StateClosed, cb.State())
	}

	cb.mu.RLock()
	if cb.failures != 0 {
		t.Errorf("Expected 0 failures after success, got %d", cb.failures)
	}
	cb.mu.RUnlock()
}

func TestCircuitBreakerRecordFailure(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold:    3,
		SuccessThreshold:    2,
		Timeout:             1 * time.Second,
		HalfOpenMaxRequests: 2,
	}

	cb := NewCircuitBreaker(config)

	// Record failures below threshold
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != StateClosed {
		t.Errorf("Expected state %s below threshold, got %s", StateClosed, cb.State())
	}

	// Record failure that exceeds threshold
	cb.RecordFailure()

	if cb.State() != StateOpen {
		t.Errorf("Expected state %s after threshold exceeded, got %s", StateOpen, cb.State())
	}
}

func TestCircuitBreakerOpenState(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold:    1,
		SuccessThreshold:    2,
		Timeout:             100 * time.Millisecond,
		HalfOpenMaxRequests: 2,
	}

	cb := NewCircuitBreaker(config)

	// Open the circuit
	cb.RecordFailure()

	if cb.State() != StateOpen {
		t.Errorf("Expected state %s, got %s", StateOpen, cb.State())
	}

	// Requests should be rejected
	err := cb.Allow()
	if err != ErrCircuitOpen {
		t.Errorf("Expected ErrCircuitOpen, got %v", err)
	}
}

func TestCircuitBreakerHalfOpenTransition(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold:    1,
		SuccessThreshold:    2,
		Timeout:             50 * time.Millisecond,
		HalfOpenMaxRequests: 2,
	}

	cb := NewCircuitBreaker(config)

	// Open the circuit
	cb.RecordFailure()

	if cb.State() != StateOpen {
		t.Errorf("Expected state %s, got %s", StateOpen, cb.State())
	}

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return cb.Allow() == nil && cb.State() == StateHalfOpen, nil
	}); err != nil {
		t.Fatalf("Expected half-open transition after timeout: %v", err)
	}
}

func TestCircuitBreakerHalfOpenSuccess(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold:    1,
		SuccessThreshold:    2,
		Timeout:             10 * time.Millisecond,
		HalfOpenMaxRequests: 3,
	}

	cb := NewCircuitBreaker(config)

	// Open the circuit
	cb.RecordFailure()

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return cb.Allow() == nil && cb.State() == StateHalfOpen, nil
	}); err != nil {
		t.Fatalf("Expected half-open transition after timeout: %v", err)
	}

	// Record successes
	cb.RecordSuccess()
	cb.RecordSuccess()

	// Should close after reaching success threshold
	if cb.State() != StateClosed {
		t.Errorf("Expected state %s after successes, got %s", StateClosed, cb.State())
	}
}

func TestCircuitBreakerHalfOpenFailure(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold:    1,
		SuccessThreshold:    2,
		Timeout:             10 * time.Millisecond,
		HalfOpenMaxRequests: 3,
	}

	cb := NewCircuitBreaker(config)

	// Open the circuit
	cb.RecordFailure()

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return cb.Allow() == nil && cb.State() == StateHalfOpen, nil
	}); err != nil {
		t.Fatalf("Expected half-open transition after timeout: %v", err)
	}

	// Record failure
	cb.RecordFailure()

	// Should reopen
	if cb.State() != StateOpen {
		t.Errorf("Expected state %s after failure in half-open, got %s", StateOpen, cb.State())
	}
}

func TestCircuitBreakerReset(t *testing.T) {
	cb := NewCircuitBreaker(nil)

	// Open the circuit
	for i := 0; i < 10; i++ {
		cb.RecordFailure()
	}

	if cb.State() != StateOpen {
		t.Errorf("Expected state %s, got %s", StateOpen, cb.State())
	}

	// Reset
	cb.Reset()

	if cb.State() != StateClosed {
		t.Errorf("Expected state %s after reset, got %s", StateClosed, cb.State())
	}

	cb.mu.RLock()
	if cb.failures != 0 {
		t.Errorf("Expected 0 failures after reset, got %d", cb.failures)
	}
	cb.mu.RUnlock()
}

func TestCircuitBreakerExecute(t *testing.T) {
	cb := NewCircuitBreaker(nil)
	ctx := context.Background()

	executed := false
	fn := func(ctx context.Context) error {
		executed = true
		return nil
	}

	err := cb.Execute(ctx, fn)
	if err != nil {
		t.Errorf("Execute should not error: %v", err)
	}

	if !executed {
		t.Error("Function was not executed")
	}

	if cb.State() != StateClosed {
		t.Errorf("Expected state %s after successful execute, got %s", StateClosed, cb.State())
	}
}

func TestCircuitBreakerExecuteFailure(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold:    1,
		SuccessThreshold:    2,
		Timeout:             1 * time.Second,
		HalfOpenMaxRequests: 2,
	}

	cb := NewCircuitBreaker(config)
	ctx := context.Background()

	testErr := errors.New("test error")
	fn := func(ctx context.Context) error {
		return testErr
	}

	err := cb.Execute(ctx, fn)
	if err != testErr {
		t.Errorf("Expected error %v, got %v", testErr, err)
	}

	if cb.State() != StateOpen {
		t.Errorf("Expected state %s after failed execute, got %s", StateOpen, cb.State())
	}
}

func TestCircuitBreakerOnStateChange(t *testing.T) {
	cb := NewCircuitBreaker(nil)

	var mu sync.Mutex
	var fromState, toState CircuitState
	called := false

	cb.OnStateChange(func(from, to CircuitState) {
		mu.Lock()
		defer mu.Unlock()
		fromState = from
		toState = to
		called = true
	})

	// Trigger state change by recording failures
	for i := 0; i < 10; i++ {
		cb.RecordFailure()
	}

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		mu.Lock()
		defer mu.Unlock()
		return called, nil
	}); err != nil {
		t.Fatalf("OnStateChange callback was not called: %v", err)
	}

	mu.Lock()
	wasCalled := called
	actualFromState := fromState
	actualToState := toState
	mu.Unlock()

	if !wasCalled {
		t.Error("OnStateChange callback was not called")
	}

	if actualFromState != StateClosed {
		t.Errorf("Expected fromState %s, got %s", StateClosed, actualFromState)
	}

	if actualToState != StateOpen {
		t.Errorf("Expected toState %s, got %s", StateOpen, actualToState)
	}
}

func TestCircuitBreakerStats(t *testing.T) {
	cb := NewCircuitBreaker(nil)

	cb.RecordFailure()
	cb.RecordFailure()

	stats := cb.Stats()

	if stats["state"] != StateClosed {
		t.Errorf("Expected state %s in stats, got %v", StateClosed, stats["state"])
	}

	if stats["failures"] != 2 {
		t.Errorf("Expected 2 failures in stats, got %v", stats["failures"])
	}

	if stats["successes"] != 0 {
		t.Errorf("Expected 0 successes in stats, got %v", stats["successes"])
	}
}
