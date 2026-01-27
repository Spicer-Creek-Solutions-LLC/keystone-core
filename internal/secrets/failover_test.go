package secrets

import (
	"context"
	"testing"
	"time"
)

func TestCircuitBreaker_InitialState(t *testing.T) {
	cb := NewCircuitBreaker(nil)
	if got := cb.State(); got != CircuitStateClosed {
		t.Errorf("Initial state = %v, want %v", got, CircuitStateClosed)
	}
}

func TestCircuitBreaker_ClosedAllowsRequests(t *testing.T) {
	cb := NewCircuitBreaker(nil)
	if !cb.AllowRequest() {
		t.Error("Closed circuit should allow requests")
	}
}

func TestCircuitBreaker_SuccessResetsFailureCount(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 5,
	})

	// Record some failures
	cb.RecordFailure()
	cb.RecordFailure()

	// Success should reset failure count
	cb.RecordSuccess()

	// Check internal state
	if got := cb.failureCount.Load(); got != 0 {
		t.Errorf("Failure count after success = %d, want 0", got)
	}
}

func TestCircuitBreaker_OpensAfterFailureThreshold(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 3,
		OpenDuration:     time.Minute,
	})

	// Record failures up to threshold
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	if got := cb.State(); got != CircuitStateOpen {
		t.Errorf("State after threshold failures = %v, want %v", got, CircuitStateOpen)
	}
}

func TestCircuitBreaker_OpenBlocksRequests(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 3,
		OpenDuration:     time.Hour, // Long duration so it doesn't transition
	})

	// Open the circuit
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	if cb.AllowRequest() {
		t.Error("Open circuit should block requests")
	}
}

func TestCircuitBreaker_TransitionsToHalfOpenAfterTimeout(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold:    3,
		OpenDuration:        10 * time.Millisecond,
		HalfOpenMaxRequests: 3,
	})

	// Open the circuit
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	// Wait for open duration
	time.Sleep(15 * time.Millisecond)

	// AllowRequest should trigger transition to half-open
	if !cb.AllowRequest() {
		t.Error("Should allow request after open duration")
	}

	if got := cb.State(); got != CircuitStateHalfOpen {
		t.Errorf("State after timeout = %v, want %v", got, CircuitStateHalfOpen)
	}
}

func TestCircuitBreaker_HalfOpenLimitsRequests(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold:    3,
		OpenDuration:        1 * time.Millisecond,
		HalfOpenMaxRequests: 2,
	})

	// Open and transition to half-open
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}
	time.Sleep(5 * time.Millisecond)

	// First two requests should be allowed
	if !cb.AllowRequest() {
		t.Error("First half-open request should be allowed")
	}
	if !cb.AllowRequest() {
		t.Error("Second half-open request should be allowed")
	}
	// Third request should be blocked
	if cb.AllowRequest() {
		t.Error("Third half-open request should be blocked (limit reached)")
	}
}

func TestCircuitBreaker_HalfOpenClosesOnSuccessThreshold(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold:    3,
		SuccessThreshold:    2,
		OpenDuration:        1 * time.Millisecond,
		HalfOpenMaxRequests: 5,
	})

	// Open and transition to half-open
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}
	time.Sleep(5 * time.Millisecond)
	cb.AllowRequest() // Trigger transition

	// Record successes up to threshold
	cb.RecordSuccess()
	cb.RecordSuccess()

	if got := cb.State(); got != CircuitStateClosed {
		t.Errorf("State after success threshold = %v, want %v", got, CircuitStateClosed)
	}
}

func TestCircuitBreaker_HalfOpenOpensOnFailure(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold:    3,
		SuccessThreshold:    2,
		OpenDuration:        1 * time.Millisecond,
		HalfOpenMaxRequests: 5,
	})

	// Open and transition to half-open
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}
	time.Sleep(5 * time.Millisecond)
	cb.AllowRequest() // Trigger transition

	// Any failure in half-open should reopen
	cb.RecordFailure()

	if got := cb.State(); got != CircuitStateOpen {
		t.Errorf("State after half-open failure = %v, want %v", got, CircuitStateOpen)
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 3,
		OpenDuration:     time.Hour,
	})

	// Open the circuit
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	if got := cb.State(); got != CircuitStateOpen {
		t.Fatalf("Precondition failed: expected Open, got %v", got)
	}

	// Reset should return to closed
	cb.Reset()

	if got := cb.State(); got != CircuitStateClosed {
		t.Errorf("State after reset = %v, want %v", got, CircuitStateClosed)
	}

	// Should allow requests again
	if !cb.AllowRequest() {
		t.Error("Reset circuit should allow requests")
	}
}

func TestCircuitBreaker_Stats(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 5,
	})

	cb.RecordFailure()
	cb.RecordFailure()

	stats := cb.Stats()

	if stats.State != CircuitStateClosed {
		t.Errorf("Stats.State = %v, want %v", stats.State, CircuitStateClosed)
	}
	if stats.FailureCount != 2 {
		t.Errorf("Stats.FailureCount = %d, want 2", stats.FailureCount)
	}
}

func TestCircuitBreaker_History(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold:    2,
		SuccessThreshold:    1,
		OpenDuration:        1 * time.Millisecond,
		HalfOpenMaxRequests: 5,
	})

	// Trigger some transitions
	cb.RecordFailure()
	cb.RecordFailure() // Opens circuit

	time.Sleep(5 * time.Millisecond)
	cb.AllowRequest() // Transitions to half-open

	cb.RecordSuccess() // Closes circuit

	history := cb.History()
	if history == nil {
		t.Fatal("History should not be nil")
	}

	entries := history.All()
	if len(entries) < 3 {
		t.Errorf("Expected at least 3 history entries, got %d", len(entries))
	}
}

func TestCircuitBreaker_Callbacks(t *testing.T) {
	openCalled := false
	closeCalled := false
	halfOpenCalled := false
	stateChanges := 0

	callbacks := &CircuitBreakerCallbacks{
		OnStateChange: func(from, to CircuitState) {
			stateChanges++
		},
		OnOpen: func(failureCount int) {
			openCalled = true
		},
		OnClose: func() {
			closeCalled = true
		},
		OnHalfOpen: func() {
			halfOpenCalled = true
		},
	}

	cb := NewCircuitBreakerWithCallbacks(&CircuitBreakerConfig{
		FailureThreshold:    2,
		SuccessThreshold:    1,
		OpenDuration:        1 * time.Millisecond,
		HalfOpenMaxRequests: 5,
	}, callbacks)

	// Trigger Open
	cb.RecordFailure()
	cb.RecordFailure()
	if !openCalled {
		t.Error("OnOpen callback not called")
	}

	// Trigger HalfOpen
	time.Sleep(5 * time.Millisecond)
	cb.AllowRequest()
	if !halfOpenCalled {
		t.Error("OnHalfOpen callback not called")
	}

	// Trigger Close
	cb.RecordSuccess()
	if !closeCalled {
		t.Error("OnClose callback not called")
	}

	if stateChanges != 3 {
		t.Errorf("Expected 3 state changes, got %d", stateChanges)
	}
}

func TestCircuitBreaker_CanTransition(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 2,
	})

	// In closed state, can't fire timeout
	if cb.CanTransition(CircuitEventTimeout) {
		t.Error("Closed state should not allow timeout event")
	}

	// Can always reset
	if !cb.CanTransition(CircuitEventReset) {
		t.Error("Should always be able to reset")
	}
}

func TestCircuitState_String(t *testing.T) {
	tests := []struct {
		state CircuitState
		want  string
	}{
		{CircuitStateClosed, "closed"},
		{CircuitStateOpen, "open"},
		{CircuitStateHalfOpen, "half-open"},
		{CircuitState(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("%v.String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestHealthState_String(t *testing.T) {
	tests := []struct {
		state HealthState
		want  string
	}{
		{HealthStateHealthy, "healthy"},
		{HealthStateDegraded, "degraded"},
		{HealthStateUnhealthy, "unhealthy"},
		{HealthStateUnknown, "unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("%v.String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestCircuitBreaker_InvalidTransitions(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 3,
	})

	// In closed state, timeout event should not cause a transition
	initialState := cb.State()
	_ = cb.machine.Fire(CircuitEventTimeout) // This should fail (no transition defined)
	if cb.State() != initialState {
		t.Error("Invalid event should not change state")
	}
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold:    100,
		SuccessThreshold:    10,
		OpenDuration:        1 * time.Millisecond,
		HalfOpenMaxRequests: 50,
	})

	done := make(chan struct{})

	// Concurrent failures
	go func() {
		for i := 0; i < 50; i++ {
			cb.RecordFailure()
		}
		done <- struct{}{}
	}()

	// Concurrent successes
	go func() {
		for i := 0; i < 50; i++ {
			cb.RecordSuccess()
		}
		done <- struct{}{}
	}()

	// Concurrent AllowRequest
	go func() {
		for i := 0; i < 50; i++ {
			cb.AllowRequest()
		}
		done <- struct{}{}
	}()

	// Concurrent Stats
	go func() {
		for i := 0; i < 50; i++ {
			cb.Stats()
		}
		done <- struct{}{}
	}()

	// Wait for all goroutines
	for i := 0; i < 4; i++ {
		<-done
	}

	// Should not panic and should be in a valid state
	state := cb.State()
	if state != CircuitStateClosed && state != CircuitStateOpen && state != CircuitStateHalfOpen {
		t.Errorf("Invalid state after concurrent access: %v", state)
	}
}

func TestDefaultCircuitBreakerConfig(t *testing.T) {
	config := DefaultCircuitBreakerConfig()

	if config.FailureThreshold != 5 {
		t.Errorf("FailureThreshold = %d, want 5", config.FailureThreshold)
	}
	if config.SuccessThreshold != 3 {
		t.Errorf("SuccessThreshold = %d, want 3", config.SuccessThreshold)
	}
	if config.OpenDuration != 30*time.Second {
		t.Errorf("OpenDuration = %v, want 30s", config.OpenDuration)
	}
	if config.HalfOpenMaxRequests != 3 {
		t.Errorf("HalfOpenMaxRequests = %d, want 3", config.HalfOpenMaxRequests)
	}
}

// failoverMockBackend is a test backend for health monitoring tests.
type failoverMockBackend struct {
	healthy bool
	name    string
}

func (m *failoverMockBackend) Type() BackendType             { return BackendTypeVault }
func (m *failoverMockBackend) Name() string                  { return m.name }
func (m *failoverMockBackend) Healthy(_ context.Context) bool { return m.healthy }
func (m *failoverMockBackend) Read(_ context.Context, _ *SecretRequest) (*Secret, error) {
	return nil, nil
}
func (m *failoverMockBackend) ReadDynamic(_ context.Context, _ *SecretRequest) (*Secret, error) {
	return nil, nil
}
func (m *failoverMockBackend) List(_ context.Context, _ string) ([]string, error) { return nil, nil }
func (m *failoverMockBackend) RenewLease(_ context.Context, _ string, _ time.Duration) (*Lease, error) {
	return nil, nil
}
func (m *failoverMockBackend) RevokeLease(_ context.Context, _ string) error { return nil }
func (m *failoverMockBackend) Close() error                                   { return nil }

func TestHealthMonitor_InitialState(t *testing.T) {
	hm := NewHealthMonitor(nil)
	backend := &failoverMockBackend{healthy: true, name: "test"}
	hm.Register("test", backend)

	health, ok := hm.GetHealth("test")
	if !ok {
		t.Fatal("Backend not registered")
	}
	if health.State != HealthStateUnknown {
		t.Errorf("Initial state = %v, want %v", health.State, HealthStateUnknown)
	}
}

func TestHealthMonitor_TransitionsToHealthy(t *testing.T) {
	hm := NewHealthMonitor(&HealthMonitorConfig{
		HealthyThreshold:   2,
		UnhealthyThreshold: 2,
		Timeout:            time.Second,
	})
	backend := &failoverMockBackend{healthy: true, name: "test"}
	hm.Register("test", backend)

	health, _ := hm.GetHealth("test")

	// Simulate health checks
	hm.checkBackend(context.Background(), "test", health)
	hm.checkBackend(context.Background(), "test", health)

	if health.State != HealthStateHealthy {
		t.Errorf("State after 2 successes = %v, want %v", health.State, HealthStateHealthy)
	}
}

func TestHealthMonitor_TransitionsToUnhealthy(t *testing.T) {
	hm := NewHealthMonitor(&HealthMonitorConfig{
		HealthyThreshold:   2,
		UnhealthyThreshold: 2,
		Timeout:            time.Second,
	})
	backend := &failoverMockBackend{healthy: false, name: "test"}
	hm.Register("test", backend)

	health, _ := hm.GetHealth("test")

	// Simulate health checks
	hm.checkBackend(context.Background(), "test", health)
	hm.checkBackend(context.Background(), "test", health)

	if health.State != HealthStateUnhealthy {
		t.Errorf("State after 2 failures = %v, want %v", health.State, HealthStateUnhealthy)
	}
}

func TestHealthMonitor_HealthyToDegraded(t *testing.T) {
	hm := NewHealthMonitor(&HealthMonitorConfig{
		HealthyThreshold:   2,
		UnhealthyThreshold: 3,
		Timeout:            time.Second,
	})
	backend := &failoverMockBackend{healthy: true, name: "test"}
	hm.Register("test", backend)

	health, _ := hm.GetHealth("test")

	// Get to healthy state
	hm.checkBackend(context.Background(), "test", health)
	hm.checkBackend(context.Background(), "test", health)

	if health.State != HealthStateHealthy {
		t.Fatalf("Precondition failed: expected Healthy, got %v", health.State)
	}

	// First failure should transition to degraded
	backend.healthy = false
	hm.checkBackend(context.Background(), "test", health)

	if health.State != HealthStateDegraded {
		t.Errorf("State after failure = %v, want %v", health.State, HealthStateDegraded)
	}
}

func TestHealthMonitor_DegradedToHealthy(t *testing.T) {
	hm := NewHealthMonitor(&HealthMonitorConfig{
		HealthyThreshold:   2,
		UnhealthyThreshold: 3,
		Timeout:            time.Second,
	})
	backend := &failoverMockBackend{healthy: true, name: "test"}
	hm.Register("test", backend)

	health, _ := hm.GetHealth("test")

	// Get to healthy state
	hm.checkBackend(context.Background(), "test", health)
	hm.checkBackend(context.Background(), "test", health)

	// First failure -> degraded
	backend.healthy = false
	hm.checkBackend(context.Background(), "test", health)

	if health.State != HealthStateDegraded {
		t.Fatalf("Precondition failed: expected Degraded, got %v", health.State)
	}

	// Recover
	backend.healthy = true
	hm.checkBackend(context.Background(), "test", health)
	hm.checkBackend(context.Background(), "test", health)

	if health.State != HealthStateHealthy {
		t.Errorf("State after recovery = %v, want %v", health.State, HealthStateHealthy)
	}
}

func TestHealthMonitor_DegradedToUnhealthy(t *testing.T) {
	hm := NewHealthMonitor(&HealthMonitorConfig{
		HealthyThreshold:   2,
		UnhealthyThreshold: 2,
		Timeout:            time.Second,
	})
	backend := &failoverMockBackend{healthy: true, name: "test"}
	hm.Register("test", backend)

	health, _ := hm.GetHealth("test")

	// Get to healthy state
	hm.checkBackend(context.Background(), "test", health)
	hm.checkBackend(context.Background(), "test", health)

	// First failure -> degraded
	backend.healthy = false
	hm.checkBackend(context.Background(), "test", health)

	if health.State != HealthStateDegraded {
		t.Fatalf("Precondition failed: expected Degraded, got %v", health.State)
	}

	// Second failure -> unhealthy
	hm.checkBackend(context.Background(), "test", health)

	if health.State != HealthStateUnhealthy {
		t.Errorf("State after 2 failures = %v, want %v", health.State, HealthStateUnhealthy)
	}
}

func TestHealthMonitor_UnhealthyToDegraded(t *testing.T) {
	hm := NewHealthMonitor(&HealthMonitorConfig{
		HealthyThreshold:   2,
		UnhealthyThreshold: 2,
		Timeout:            time.Second,
	})
	backend := &failoverMockBackend{healthy: false, name: "test"}
	hm.Register("test", backend)

	health, _ := hm.GetHealth("test")

	// Get to unhealthy state
	hm.checkBackend(context.Background(), "test", health)
	hm.checkBackend(context.Background(), "test", health)

	if health.State != HealthStateUnhealthy {
		t.Fatalf("Precondition failed: expected Unhealthy, got %v", health.State)
	}

	// First success -> degraded
	backend.healthy = true
	hm.checkBackend(context.Background(), "test", health)

	if health.State != HealthStateDegraded {
		t.Errorf("State after first success = %v, want %v", health.State, HealthStateDegraded)
	}
}

func TestHealthMonitor_Callbacks(t *testing.T) {
	healthyCalled := false
	unhealthyCalled := false
	degradedCalled := false
	stateChanges := 0

	callbacks := &HealthMonitorCallbacks{
		OnStateChange: func(name string, from, to HealthState) {
			stateChanges++
		},
		OnHealthy: func(name string) {
			healthyCalled = true
		},
		OnUnhealthy: func(name string) {
			unhealthyCalled = true
		},
		OnDegraded: func(name string) {
			degradedCalled = true
		},
	}

	hm := NewHealthMonitorWithCallbacks(&HealthMonitorConfig{
		HealthyThreshold:   2,
		UnhealthyThreshold: 2,
		Timeout:            time.Second,
	}, callbacks)
	backend := &failoverMockBackend{healthy: true, name: "test"}
	hm.Register("test", backend)

	health, _ := hm.GetHealth("test")

	// Get to healthy
	hm.checkBackend(context.Background(), "test", health)
	hm.checkBackend(context.Background(), "test", health)
	if !healthyCalled {
		t.Error("OnHealthy callback not called")
	}

	// Get to degraded
	backend.healthy = false
	hm.checkBackend(context.Background(), "test", health)
	if !degradedCalled {
		t.Error("OnDegraded callback not called")
	}

	// Get to unhealthy
	hm.checkBackend(context.Background(), "test", health)
	if !unhealthyCalled {
		t.Error("OnUnhealthy callback not called")
	}

	if stateChanges < 3 {
		t.Errorf("Expected at least 3 state changes, got %d", stateChanges)
	}
}

func TestHealthMonitor_IsHealthy(t *testing.T) {
	hm := NewHealthMonitor(&HealthMonitorConfig{
		HealthyThreshold:   2,
		UnhealthyThreshold: 2,
		Timeout:            time.Second,
	})
	backend := &failoverMockBackend{healthy: true, name: "test"}
	hm.Register("test", backend)

	// Not healthy initially (unknown state)
	if hm.IsHealthy("test") {
		t.Error("Should not be healthy in unknown state")
	}

	health, _ := hm.GetHealth("test")

	// Get to healthy
	hm.checkBackend(context.Background(), "test", health)
	hm.checkBackend(context.Background(), "test", health)

	if !hm.IsHealthy("test") {
		t.Error("Should be healthy after successful checks")
	}
}

func TestHealthMonitor_UnregisterBackend(t *testing.T) {
	hm := NewHealthMonitor(nil)
	backend := &failoverMockBackend{healthy: true, name: "test"}
	hm.Register("test", backend)

	hm.Unregister("test")

	_, ok := hm.GetHealth("test")
	if ok {
		t.Error("Backend should be unregistered")
	}
}

func TestHealthMonitor_GetAllHealth(t *testing.T) {
	hm := NewHealthMonitor(nil)
	hm.Register("backend1", &failoverMockBackend{healthy: true, name: "backend1"})
	hm.Register("backend2", &failoverMockBackend{healthy: true, name: "backend2"})

	all := hm.GetAllHealth()
	if len(all) != 2 {
		t.Errorf("Expected 2 backends, got %d", len(all))
	}
}

func TestDefaultHealthMonitorConfig(t *testing.T) {
	config := DefaultHealthMonitorConfig()

	if config.CheckInterval != 30*time.Second {
		t.Errorf("CheckInterval = %v, want 30s", config.CheckInterval)
	}
	if config.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v, want 5s", config.Timeout)
	}
	if config.HealthyThreshold != 2 {
		t.Errorf("HealthyThreshold = %d, want 2", config.HealthyThreshold)
	}
	if config.UnhealthyThreshold != 3 {
		t.Errorf("UnhealthyThreshold = %d, want 3", config.UnhealthyThreshold)
	}
}
