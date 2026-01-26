package nats

import (
	"errors"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

func TestConnectionStateMachine_BasicTransitions(t *testing.T) {
	machine := NewConnectionStateMachine(nil)

	// Initial state should be disconnected
	if machine.State() != ConnectionStateDisconnected {
		t.Errorf("expected disconnected, got %v", machine.State())
	}

	// Connect
	if err := machine.Fire(EventConnect); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if machine.State() != ConnectionStateConnecting {
		t.Errorf("expected connecting, got %v", machine.State())
	}

	// Connected
	if err := machine.Fire(EventConnected); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if machine.State() != ConnectionStateConnected {
		t.Errorf("expected connected, got %v", machine.State())
	}

	// Disconnected (to reconnecting)
	if err := machine.Fire(EventDisconnected); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if machine.State() != ConnectionStateReconnecting {
		t.Errorf("expected reconnecting, got %v", machine.State())
	}

	// Reconnected
	if err := machine.Fire(EventReconnected); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if machine.State() != ConnectionStateConnected {
		t.Errorf("expected connected, got %v", machine.State())
	}
}

func TestConnectionStateMachine_FailurePath(t *testing.T) {
	machine := NewConnectionStateMachine(nil)

	machine.Fire(EventConnect)

	// Fail while connecting
	if err := machine.Fire(EventFailed); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if machine.State() != ConnectionStateDisconnected {
		t.Errorf("expected disconnected after failure, got %v", machine.State())
	}
}

func TestConnectionStateMachine_Close(t *testing.T) {
	tests := []struct {
		name         string
		initialEvent ConnStateEvent
	}{
		{"close from disconnected", ""},
		{"close from connecting", EventConnect},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			machine := NewConnectionStateMachine(nil)
			if tt.initialEvent != "" {
				machine.Fire(tt.initialEvent)
			}

			if err := machine.Fire(EventClose); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if machine.State() != ConnectionStateClosed {
				t.Errorf("expected closed, got %v", machine.State())
			}
		})
	}
}

func TestConnectionStateMachine_InvalidTransitions(t *testing.T) {
	machine := NewConnectionStateMachine(nil)

	// Can't fire Connected from Disconnected
	err := machine.Fire(EventConnected)
	if err == nil {
		t.Error("expected error for invalid transition")
	}
	if !errors.Is(err, statemachine.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestConnectionStateMachine_History(t *testing.T) {
	machine := NewConnectionStateMachine(nil)

	machine.Fire(EventConnect)
	machine.Fire(EventConnected)
	machine.Fire(EventDisconnected)
	machine.Fire(EventReconnected)

	history := machine.History()
	records := history.All()

	if len(records) != 4 {
		t.Errorf("expected 4 history records, got %d", len(records))
	}
}

func TestSMCircuitBreaker_BasicOperation(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold:    3,
		SuccessThreshold:    2,
		OpenDuration:        100 * time.Millisecond,
		HalfOpenMaxAttempts: 1,
	}

	var openCalled bool
	cb := NewSMCircuitBreaker(config,
		func() { openCalled = true },
		nil,
	)

	// Initial state should be closed
	if !cb.IsClosed() {
		t.Error("expected closed initial state")
	}

	// Should allow requests when closed
	if !cb.AllowRequest() {
		t.Error("should allow request when closed")
	}

	// Record failures until threshold
	cb.RecordFailure()
	cb.RecordFailure()
	if !cb.IsClosed() {
		t.Error("should still be closed after 2 failures")
	}

	cb.RecordFailure() // Third failure triggers open
	if !cb.IsOpen() {
		t.Error("should be open after 3 failures")
	}
	if !openCalled {
		t.Error("onOpen callback should have been called")
	}

	// Should block requests when open
	if cb.AllowRequest() {
		t.Error("should not allow request when open")
	}
}

func TestSMCircuitBreaker_TimeoutTransition(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold:    2,
		SuccessThreshold:    1,
		OpenDuration:        50 * time.Millisecond,
		HalfOpenMaxAttempts: 1,
	}

	cb := NewSMCircuitBreaker(config, nil, nil)

	// Open the circuit
	cb.RecordFailure()
	cb.RecordFailure()
	if !cb.IsOpen() {
		t.Fatal("circuit should be open")
	}

	start := time.Now()
	if err := helpers.WaitForTimeout(200*time.Millisecond, 5*time.Millisecond, func() (bool, error) {
		return time.Since(start) >= config.OpenDuration, nil
	}); err != nil {
		t.Fatalf("Timeout not reached: %v", err)
	}

	// AllowRequest should transition to half-open
	if !cb.AllowRequest() {
		t.Error("should allow request after timeout")
	}
	if !cb.IsHalfOpen() {
		t.Error("should be half-open after timeout")
	}
}

func TestSMCircuitBreaker_HalfOpenRecovery(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold:    2,
		SuccessThreshold:    2,
		OpenDuration:        10 * time.Millisecond,
		HalfOpenMaxAttempts: 1,
	}

	var closeCalled bool
	cb := NewSMCircuitBreaker(config, nil, func() { closeCalled = true })

	// Open the circuit
	cb.RecordFailure()
	cb.RecordFailure()

	start := time.Now()
	if err := helpers.WaitForTimeout(200*time.Millisecond, 5*time.Millisecond, func() (bool, error) {
		return time.Since(start) >= config.OpenDuration, nil
	}); err != nil {
		t.Fatalf("Timeout not reached: %v", err)
	}
	cb.AllowRequest() // Transitions to half-open

	// Record successes
	cb.RecordSuccess()
	if !cb.IsHalfOpen() {
		t.Error("should still be half-open after 1 success")
	}

	cb.RecordSuccess() // Second success closes the circuit
	if !cb.IsClosed() {
		t.Error("should be closed after 2 successes")
	}
	if !closeCalled {
		t.Error("onClose callback should have been called")
	}
}

func TestSMCircuitBreaker_HalfOpenFailure(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold:    2,
		SuccessThreshold:    2,
		OpenDuration:        10 * time.Millisecond,
		HalfOpenMaxAttempts: 1,
	}

	cb := NewSMCircuitBreaker(config, nil, nil)

	// Open the circuit
	cb.RecordFailure()
	cb.RecordFailure()

	start := time.Now()
	if err := helpers.WaitForTimeout(200*time.Millisecond, 5*time.Millisecond, func() (bool, error) {
		return time.Since(start) >= config.OpenDuration, nil
	}); err != nil {
		t.Fatalf("Timeout not reached: %v", err)
	}
	cb.AllowRequest() // Transitions to half-open

	// Failure in half-open goes back to open
	cb.RecordFailure()
	if !cb.IsOpen() {
		t.Error("should be open after failure in half-open")
	}
}

func TestSMCircuitBreaker_Reset(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		OpenDuration:     1 * time.Second,
	}

	cb := NewSMCircuitBreaker(config, nil, nil)

	// Open the circuit
	cb.RecordFailure()
	cb.RecordFailure()
	if !cb.IsOpen() {
		t.Fatal("circuit should be open")
	}

	cb.Reset()

	if !cb.IsClosed() {
		t.Error("should be closed after reset")
	}
}

func TestSMCircuitBreaker_NextRetryTime(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold: 2,
		OpenDuration:     100 * time.Millisecond,
	}

	cb := NewSMCircuitBreaker(config, nil, nil)

	// No retry time when closed
	if !cb.NextRetryTime().IsZero() {
		t.Error("should have zero retry time when closed")
	}

	// Open the circuit
	cb.RecordFailure()
	cb.RecordFailure()

	retryTime := cb.NextRetryTime()
	if retryTime.IsZero() {
		t.Error("should have retry time when open")
	}

	// Retry time should be approximately OpenDuration from now
	expectedRetry := time.Now().Add(100 * time.Millisecond)
	if retryTime.Sub(expectedRetry) > 10*time.Millisecond {
		t.Error("retry time is not approximately correct")
	}
}

func TestSMCircuitBreaker_History(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 1,
		OpenDuration:     10 * time.Millisecond,
	}

	cb := NewSMCircuitBreaker(config, nil, nil)

	// Generate some transitions
	cb.RecordFailure()
	cb.RecordFailure() // Opens
	start := time.Now()
	if err := helpers.WaitForTimeout(200*time.Millisecond, 5*time.Millisecond, func() (bool, error) {
		return time.Since(start) >= config.OpenDuration, nil
	}); err != nil {
		t.Fatalf("Timeout not reached: %v", err)
	}
	cb.AllowRequest()  // Half-open
	cb.RecordSuccess() // Closes

	history := cb.History()
	records := history.All()

	if len(records) < 3 {
		t.Errorf("expected at least 3 history records, got %d", len(records))
	}
}

func TestManagedEndpoint_Creation(t *testing.T) {
	endpoint := &Endpoint{
		Host: "localhost",
		Port: 4222,
	}

	me := NewManagedEndpoint(endpoint, nil, nil, nil)

	if me.ConnectionState() != ConnectionStateDisconnected {
		t.Errorf("expected disconnected, got %v", me.ConnectionState())
	}

	if !me.circuitBreaker.IsClosed() {
		t.Error("circuit breaker should be closed")
	}
}

func TestManagedEndpoint_CanConnect(t *testing.T) {
	endpoint := &Endpoint{Host: "localhost", Port: 4222}
	me := NewManagedEndpoint(endpoint, &CircuitBreakerConfig{
		FailureThreshold: 2,
		OpenDuration:     1 * time.Second,
	}, nil, nil)

	// Should be able to connect initially
	if !me.CanConnect() {
		t.Error("should be able to connect initially")
	}

	// Open circuit breaker
	me.RecordConnectionFailure(errors.New("fail"))
	me.RecordConnectionFailure(errors.New("fail"))

	// Should not be able to connect with open circuit
	if me.CanConnect() {
		t.Error("should not be able to connect with open circuit")
	}
}

func TestManagedEndpoint_TransitionTo(t *testing.T) {
	endpoint := &Endpoint{Host: "localhost", Port: 4222}
	me := NewManagedEndpoint(endpoint, nil, nil, nil)

	// Transition to connecting
	if err := me.TransitionTo(EventConnect); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if me.ConnectionState() != ConnectionStateConnecting {
		t.Errorf("expected connecting, got %v", me.ConnectionState())
	}

	// State field should be updated
	if me.State != ConnectionStateConnecting {
		t.Errorf("state field should be updated to connecting")
	}

	// Invalid transition
	err := me.TransitionTo(EventReconnected)
	if err == nil {
		t.Error("expected error for invalid transition")
	}
}

func TestManagedEndpoint_RecordSuccess(t *testing.T) {
	endpoint := &Endpoint{Host: "localhost", Port: 4222}
	me := NewManagedEndpoint(endpoint, nil, nil, nil)

	me.RecordConnectionSuccess(100 * time.Millisecond)

	if me.SuccessCount != 1 {
		t.Errorf("expected success count 1, got %d", me.SuccessCount)
	}
	if me.TotalLatency != 100*time.Millisecond {
		t.Errorf("expected latency 100ms, got %v", me.TotalLatency)
	}
	if me.LastConnected.IsZero() {
		t.Error("last connected should be set")
	}
}

func TestManagedEndpoint_RecordFailure(t *testing.T) {
	endpoint := &Endpoint{Host: "localhost", Port: 4222}
	me := NewManagedEndpoint(endpoint, nil, nil, nil)

	testErr := errors.New("connection failed")
	me.RecordConnectionFailure(testErr)

	if me.FailureCount != 1 {
		t.Errorf("expected failure count 1, got %d", me.FailureCount)
	}
	if me.LastError != testErr {
		t.Error("last error should be set")
	}
	if me.LastErrorTime.IsZero() {
		t.Error("last error time should be set")
	}
}

func TestManagedEndpoint_IsHealthy(t *testing.T) {
	endpoint := &Endpoint{Host: "localhost", Port: 4222}
	config := &CircuitBreakerConfig{FailureThreshold: 2}
	me := NewManagedEndpoint(endpoint, config, nil, nil)

	// Not healthy initially (disconnected)
	if me.IsHealthy() {
		t.Error("should not be healthy when disconnected")
	}

	// Transition to connected
	me.TransitionTo(EventConnect)
	me.TransitionTo(EventConnected)

	// Now healthy
	if !me.IsHealthy() {
		t.Error("should be healthy when connected")
	}

	// Open circuit breaker
	me.RecordConnectionFailure(errors.New("fail"))
	me.RecordConnectionFailure(errors.New("fail"))

	// Not healthy with open circuit
	if me.IsHealthy() {
		t.Error("should not be healthy with open circuit")
	}
}

func TestManagedEndpoint_ConnectionHistory(t *testing.T) {
	endpoint := &Endpoint{Host: "localhost", Port: 4222}
	me := NewManagedEndpoint(endpoint, nil, nil, nil)

	me.TransitionTo(EventConnect)
	me.TransitionTo(EventConnected)

	history := me.ConnectionHistory()
	if history == nil {
		t.Fatal("history should not be nil")
	}

	records := history.All()
	if len(records) != 2 {
		t.Errorf("expected 2 history records, got %d", len(records))
	}
}
