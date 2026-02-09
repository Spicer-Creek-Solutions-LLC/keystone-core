package secrets

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

// HealthState represents the health state of a backend.
type HealthState int32

const (
	// HealthStateUnknown indicates the health state is unknown.
	HealthStateUnknown HealthState = iota
	// HealthStateHealthy indicates the backend is healthy.
	HealthStateHealthy
	// HealthStateDegraded indicates the backend is experiencing issues but still usable.
	HealthStateDegraded
	// HealthStateUnhealthy indicates the backend is unhealthy.
	HealthStateUnhealthy
)

// String returns the string representation of the health state.
func (s HealthState) String() string {
	switch s {
	case HealthStateHealthy:
		return "healthy"
	case HealthStateDegraded:
		return "degraded"
	case HealthStateUnhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}

// HealthEvent represents events that trigger health state transitions.
//
// State Diagram (Mermaid):
//
//	stateDiagram-v2
//	    [*] --> Unknown
//	    Unknown --> Healthy: check_success (threshold met)
//	    Unknown --> Unhealthy: check_failure (threshold met)
//	    Healthy --> Degraded: check_failure
//	    Healthy --> Healthy: check_success
//	    Degraded --> Healthy: check_success (threshold met)
//	    Degraded --> Unhealthy: check_failure (threshold met)
//	    Unhealthy --> Degraded: check_success
//	    Unhealthy --> Unhealthy: check_failure
type HealthEvent string

const (
	// HealthEventCheckSuccess records a successful health check.
	HealthEventCheckSuccess HealthEvent = "check_success"

	// HealthEventCheckFailure records a failed health check.
	HealthEventCheckFailure HealthEvent = "check_failure"
)

// HealthMonitorCallbacks defines callbacks for health state transitions.
type HealthMonitorCallbacks struct {
	// OnStateChange is called when a backend's health state changes.
	OnStateChange func(backendName string, from, to HealthState)

	// OnHealthy is called when a backend becomes healthy.
	OnHealthy func(backendName string)

	// OnUnhealthy is called when a backend becomes unhealthy.
	OnUnhealthy func(backendName string)

	// OnDegraded is called when a backend becomes degraded.
	OnDegraded func(backendName string)
}

// CircuitState represents the state of the circuit breaker.
type CircuitState int32

const (
	// CircuitStateClosed indicates the circuit is closed (normal operation).
	CircuitStateClosed CircuitState = iota
	// CircuitStateOpen indicates the circuit is open (failing fast).
	CircuitStateOpen
	// CircuitStateHalfOpen indicates the circuit is testing if the backend recovered.
	CircuitStateHalfOpen
)

// String returns the string representation of the circuit state.
func (s CircuitState) String() string {
	switch s {
	case CircuitStateClosed:
		return "closed"
	case CircuitStateOpen:
		return "open"
	case CircuitStateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitEvent represents events that trigger circuit breaker state transitions.
//
// State Diagram (Mermaid):
//
//	stateDiagram-v2
//	    [*] --> Closed
//	    Closed --> Open: failure_threshold
//	    Open --> HalfOpen: timeout
//	    HalfOpen --> Closed: success_threshold
//	    HalfOpen --> Open: failure
//	    Closed --> Closed: success
//	    Closed --> Closed: failure_below_threshold
type CircuitEvent string

const (
	// CircuitEventFailure records a failed request.
	CircuitEventFailure CircuitEvent = "failure"

	// CircuitEventSuccess records a successful request.
	CircuitEventSuccess CircuitEvent = "success"

	// CircuitEventTimeout triggers transition from open to half-open.
	CircuitEventTimeout CircuitEvent = "timeout"

	// CircuitEventReset resets the circuit to closed state.
	CircuitEventReset CircuitEvent = "reset"
)

// CircuitBreakerConfig configures the circuit breaker.
type CircuitBreakerConfig struct {
	// FailureThreshold is the number of failures before opening the circuit.
	FailureThreshold int `json:"failure_threshold,omitempty"`

	// SuccessThreshold is the number of successes needed to close the circuit.
	SuccessThreshold int `json:"success_threshold,omitempty"`

	// OpenDuration is how long to keep the circuit open before testing.
	OpenDuration time.Duration `json:"open_duration,omitempty"`

	// HalfOpenMaxRequests is the max requests allowed in half-open state.
	HalfOpenMaxRequests int `json:"half_open_max_requests,omitempty"`
}

// DefaultCircuitBreakerConfig returns default circuit breaker configuration.
func DefaultCircuitBreakerConfig() *CircuitBreakerConfig {
	return &CircuitBreakerConfig{
		FailureThreshold:    5,
		SuccessThreshold:    3,
		OpenDuration:        30 * time.Second,
		HalfOpenMaxRequests: 3,
	}
}

// CircuitBreakerCallbacks defines callbacks for circuit breaker state transitions.
type CircuitBreakerCallbacks struct {
	// OnStateChange is called when the circuit state changes.
	OnStateChange func(from, to CircuitState)

	// OnOpen is called when the circuit opens due to failures.
	OnOpen func(failureCount int)

	// OnClose is called when the circuit closes after recovery.
	OnClose func()

	// OnHalfOpen is called when the circuit enters half-open state.
	OnHalfOpen func()
}

// CircuitBreaker implements the circuit breaker pattern for a backend.
type CircuitBreaker struct {
	config *CircuitBreakerConfig

	// machine is the state machine managing transitions.
	machine *statemachine.Machine[CircuitState, CircuitEvent]

	// counters track failure/success counts
	failureCount    atomic.Int32
	successCount    atomic.Int32
	halfOpenCount   atomic.Int32
	lastFailureTime atomic.Int64

	// callbacks for state change notifications
	callbacks *CircuitBreakerCallbacks
}

// NewCircuitBreaker creates a new circuit breaker.
func NewCircuitBreaker(config *CircuitBreakerConfig) *CircuitBreaker {
	return NewCircuitBreakerWithCallbacks(config, nil)
}

// NewCircuitBreakerWithCallbacks creates a new circuit breaker with callbacks.
func NewCircuitBreakerWithCallbacks(config *CircuitBreakerConfig, callbacks *CircuitBreakerCallbacks) *CircuitBreaker {
	if config == nil {
		config = DefaultCircuitBreakerConfig()
	}

	cb := &CircuitBreaker{
		config:    config,
		callbacks: callbacks,
	}
	cb.machine = cb.buildStateMachine()
	return cb
}

// buildStateMachine creates the circuit breaker state machine.
func (cb *CircuitBreaker) buildStateMachine() *statemachine.Machine[CircuitState, CircuitEvent] {
	builder := statemachine.New[CircuitState, CircuitEvent](CircuitStateClosed).
		WithHistory(25).
		WithName("circuit-breaker")

	// Closed -> Open: failure threshold reached
	builder.AddTransition(CircuitStateClosed, CircuitEventFailure, CircuitStateOpen).
		WithGuard(func(_ context.Context, _ CircuitState, _ CircuitEvent) bool {
			return int(cb.failureCount.Load()) >= cb.config.FailureThreshold
		})

	// Closed stays Closed on failure below threshold (ignore the event)
	builder.Ignore(CircuitStateClosed, CircuitEventSuccess)

	// Open -> HalfOpen: timeout elapsed
	builder.AddTransition(CircuitStateOpen, CircuitEventTimeout, CircuitStateHalfOpen)

	// HalfOpen -> Closed: success threshold reached
	builder.AddTransition(CircuitStateHalfOpen, CircuitEventSuccess, CircuitStateClosed).
		WithGuard(func(_ context.Context, _ CircuitState, _ CircuitEvent) bool {
			return int(cb.successCount.Load()) >= cb.config.SuccessThreshold
		})

	// HalfOpen -> Open: any failure
	builder.AddTransition(CircuitStateHalfOpen, CircuitEventFailure, CircuitStateOpen)

	// Reset transitions - can reset from any state
	builder.AddTransition(CircuitStateClosed, CircuitEventReset, CircuitStateClosed)
	builder.AddTransition(CircuitStateOpen, CircuitEventReset, CircuitStateClosed)
	builder.AddTransition(CircuitStateHalfOpen, CircuitEventReset, CircuitStateClosed)

	// Callbacks for state entry
	builder.OnEnter(CircuitStateClosed, func(_ context.Context, _ CircuitState, from CircuitState) {
		cb.onEnterClosed(from)
	})

	builder.OnEnter(CircuitStateOpen, func(_ context.Context, _ CircuitState, from CircuitState) {
		cb.onEnterOpen(from)
	})

	builder.OnEnter(CircuitStateHalfOpen, func(_ context.Context, _ CircuitState, from CircuitState) {
		cb.onEnterHalfOpen(from)
	})

	// Global transition callback
	builder.OnTransition(func(_ context.Context, from, to CircuitState, _ CircuitEvent) {
		if cb.callbacks != nil && cb.callbacks.OnStateChange != nil && from != to {
			cb.callbacks.OnStateChange(from, to)
		}
	})

	return builder.MustBuild()
}

// onEnterClosed is called when entering the Closed state.
func (cb *CircuitBreaker) onEnterClosed(from CircuitState) {
	cb.failureCount.Store(0)
	cb.successCount.Store(0)
	cb.halfOpenCount.Store(0)

	if cb.callbacks != nil && cb.callbacks.OnClose != nil && from != CircuitStateClosed {
		cb.callbacks.OnClose()
	}
}

// onEnterOpen is called when entering the Open state.
func (cb *CircuitBreaker) onEnterOpen(from CircuitState) {
	failCount := int(cb.failureCount.Load())
	cb.failureCount.Store(0)
	cb.successCount.Store(0)
	cb.halfOpenCount.Store(0)

	if cb.callbacks != nil && cb.callbacks.OnOpen != nil {
		cb.callbacks.OnOpen(failCount)
	}
}

// onEnterHalfOpen is called when entering the HalfOpen state.
func (cb *CircuitBreaker) onEnterHalfOpen(from CircuitState) {
	cb.failureCount.Store(0)
	cb.successCount.Store(0)
	cb.halfOpenCount.Store(0)

	if cb.callbacks != nil && cb.callbacks.OnHalfOpen != nil {
		cb.callbacks.OnHalfOpen()
	}
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() CircuitState {
	return cb.machine.State()
}

// AllowRequest returns true if a request should be allowed.
func (cb *CircuitBreaker) AllowRequest() bool {
	state := cb.State()

	switch state {
	case CircuitStateClosed:
		return true

	case CircuitStateOpen:
		// Check if we should transition to half-open
		lastFailure := time.Unix(0, cb.lastFailureTime.Load())
		if time.Since(lastFailure) >= cb.config.OpenDuration {
			// Try to transition to half-open via state machine
			if err := cb.machine.Fire(CircuitEventTimeout); err == nil {
				return cb.tryHalfOpen()
			}
		}
		return false

	case CircuitStateHalfOpen:
		return cb.tryHalfOpen()

	default:
		return true
	}
}

// tryHalfOpen attempts to allow a request in half-open state.
func (cb *CircuitBreaker) tryHalfOpen() bool {
	count := cb.halfOpenCount.Add(1)
	return int(count) <= cb.config.HalfOpenMaxRequests
}

// RecordSuccess records a successful request.
func (cb *CircuitBreaker) RecordSuccess() {
	state := cb.State()

	switch state {
	case CircuitStateClosed:
		// Reset failure count on success in closed state
		cb.failureCount.Store(0)

	case CircuitStateHalfOpen:
		count := cb.successCount.Add(1)
		if int(count) >= cb.config.SuccessThreshold {
			// Try to transition to closed via state machine
			_ = cb.machine.Fire(CircuitEventSuccess)
		}
	default:
	}
}

// RecordFailure records a failed request.
func (cb *CircuitBreaker) RecordFailure() {
	cb.lastFailureTime.Store(time.Now().UnixNano())
	state := cb.State()

	switch state {
	case CircuitStateClosed:
		count := cb.failureCount.Add(1)
		if int(count) >= cb.config.FailureThreshold {
			// Try to transition to open via state machine
			_ = cb.machine.Fire(CircuitEventFailure)
		}

	case CircuitStateHalfOpen:
		// Any failure in half-open goes back to open
		_ = cb.machine.Fire(CircuitEventFailure)
	default:
	}
}

// Reset resets the circuit breaker to closed state.
func (cb *CircuitBreaker) Reset() {
	_ = cb.machine.Fire(CircuitEventReset)
}

// Stats returns circuit breaker statistics.
func (cb *CircuitBreaker) Stats() *CircuitBreakerStats {
	return &CircuitBreakerStats{
		State:           cb.State(),
		FailureCount:    int(cb.failureCount.Load()),
		SuccessCount:    int(cb.successCount.Load()),
		LastFailureTime: time.Unix(0, cb.lastFailureTime.Load()),
		LastStateChange: cb.machine.StateEnteredAt(),
	}
}

// History returns the state transition history.
func (cb *CircuitBreaker) History() *statemachine.History[CircuitState, CircuitEvent] {
	return cb.machine.History()
}

// CanTransition returns true if the given event can trigger a transition.
func (cb *CircuitBreaker) CanTransition(event CircuitEvent) bool {
	return cb.machine.CanFire(event)
}

// CircuitBreakerStats contains circuit breaker statistics.
type CircuitBreakerStats struct {
	State           CircuitState `json:"state"`
	FailureCount    int          `json:"failure_count"`
	SuccessCount    int          `json:"success_count"`
	LastFailureTime time.Time    `json:"last_failure_time,omitempty"`
	LastStateChange time.Time    `json:"last_state_change"`
}

// HealthMonitor monitors the health of backends.
type HealthMonitor struct {
	mu sync.RWMutex

	// backends maps backend names to their health state.
	backends map[string]*BackendHealth

	// config is the health monitor configuration.
	config *HealthMonitorConfig

	// callbacks for state change notifications.
	callbacks *HealthMonitorCallbacks

	// stopCh signals the monitor to stop.
	stopCh chan struct{}

	// running indicates if the monitor is running.
	running bool
}

// HealthMonitorConfig configures the health monitor.
type HealthMonitorConfig struct {
	// CheckInterval is how often to check backend health.
	CheckInterval time.Duration `json:"check_interval,omitempty"`

	// Timeout is the timeout for health checks.
	Timeout time.Duration `json:"timeout,omitempty"`

	// HealthyThreshold is the number of consecutive successes to mark healthy.
	HealthyThreshold int `json:"healthy_threshold,omitempty"`

	// UnhealthyThreshold is the number of consecutive failures to mark unhealthy.
	UnhealthyThreshold int `json:"unhealthy_threshold,omitempty"`

	// CircuitBreaker configures the circuit breaker for each backend.
	CircuitBreaker *CircuitBreakerConfig `json:"circuit_breaker,omitempty"`
}

// DefaultHealthMonitorConfig returns default health monitor configuration.
func DefaultHealthMonitorConfig() *HealthMonitorConfig {
	return &HealthMonitorConfig{
		CheckInterval:      30 * time.Second,
		Timeout:            5 * time.Second,
		HealthyThreshold:   2,
		UnhealthyThreshold: 3,
		CircuitBreaker:     DefaultCircuitBreakerConfig(),
	}
}

// BackendHealth tracks the health of a single backend.
type BackendHealth struct {
	// Backend is the backend being monitored.
	Backend SecretBackend

	// name is the backend name for callbacks.
	name string

	// State is the current health state.
	State HealthState

	// machine is the state machine managing health transitions.
	machine *statemachine.Machine[HealthState, HealthEvent]

	// CircuitBreaker is the circuit breaker for this backend.
	CircuitBreaker *CircuitBreaker

	// LastCheck is when the health was last checked.
	LastCheck time.Time

	// LastHealthy is when the backend was last healthy.
	LastHealthy time.Time

	// ConsecutiveSuccesses is the number of consecutive successful checks.
	ConsecutiveSuccesses int

	// ConsecutiveFailures is the number of consecutive failed checks.
	ConsecutiveFailures int

	// TotalChecks is the total number of health checks.
	TotalChecks int64

	// TotalFailures is the total number of failed checks.
	TotalFailures int64

	// LastError is the last error encountered.
	LastError error

	// config holds the health monitor configuration.
	config *HealthMonitorConfig

	// callbacks for state change notifications.
	callbacks *HealthMonitorCallbacks

	mu sync.RWMutex
}

// NewHealthMonitor creates a new health monitor.
func NewHealthMonitor(config *HealthMonitorConfig) *HealthMonitor {
	return NewHealthMonitorWithCallbacks(config, nil)
}

// NewHealthMonitorWithCallbacks creates a new health monitor with callbacks.
func NewHealthMonitorWithCallbacks(config *HealthMonitorConfig, callbacks *HealthMonitorCallbacks) *HealthMonitor {
	if config == nil {
		config = DefaultHealthMonitorConfig()
	}

	return &HealthMonitor{
		backends:  make(map[string]*BackendHealth),
		config:    config,
		callbacks: callbacks,
		stopCh:    make(chan struct{}),
	}
}

// buildHealthStateMachine creates a health state machine for a backend.
func (hm *HealthMonitor) buildHealthStateMachine(health *BackendHealth) *statemachine.Machine[HealthState, HealthEvent] {
	builder := statemachine.New[HealthState, HealthEvent](HealthStateUnknown).
		WithHistory(25).
		WithName("health-" + health.name)

	// Unknown -> Healthy: consecutive successes meet threshold
	builder.AddTransition(HealthStateUnknown, HealthEventCheckSuccess, HealthStateHealthy).
		WithGuard(func(_ context.Context, _ HealthState, _ HealthEvent) bool {
			health.mu.RLock()
			defer health.mu.RUnlock()
			return health.ConsecutiveSuccesses >= hm.config.HealthyThreshold
		})

	// Unknown -> Unhealthy: consecutive failures meet threshold
	builder.AddTransition(HealthStateUnknown, HealthEventCheckFailure, HealthStateUnhealthy).
		WithGuard(func(_ context.Context, _ HealthState, _ HealthEvent) bool {
			health.mu.RLock()
			defer health.mu.RUnlock()
			return health.ConsecutiveFailures >= hm.config.UnhealthyThreshold
		})

	// Healthy -> Degraded: first failure after healthy
	builder.AddTransition(HealthStateHealthy, HealthEventCheckFailure, HealthStateDegraded)

	// Healthy stays Healthy on success (ignore)
	builder.Ignore(HealthStateHealthy, HealthEventCheckSuccess)

	// Degraded -> Healthy: consecutive successes meet threshold
	builder.AddTransition(HealthStateDegraded, HealthEventCheckSuccess, HealthStateHealthy).
		WithGuard(func(_ context.Context, _ HealthState, _ HealthEvent) bool {
			health.mu.RLock()
			defer health.mu.RUnlock()
			return health.ConsecutiveSuccesses >= hm.config.HealthyThreshold
		})

	// Degraded -> Unhealthy: consecutive failures meet threshold
	builder.AddTransition(HealthStateDegraded, HealthEventCheckFailure, HealthStateUnhealthy).
		WithGuard(func(_ context.Context, _ HealthState, _ HealthEvent) bool {
			health.mu.RLock()
			defer health.mu.RUnlock()
			return health.ConsecutiveFailures >= hm.config.UnhealthyThreshold
		})

	// Unhealthy -> Degraded: first success after unhealthy
	builder.AddTransition(HealthStateUnhealthy, HealthEventCheckSuccess, HealthStateDegraded)

	// Unhealthy stays Unhealthy on failure (ignore)
	builder.Ignore(HealthStateUnhealthy, HealthEventCheckFailure)

	// State entry callbacks
	builder.OnEnter(HealthStateHealthy, func(_ context.Context, _ HealthState, from HealthState) {
		health.mu.Lock()
		health.State = HealthStateHealthy
		health.LastHealthy = time.Now()
		health.mu.Unlock()

		if hm.callbacks != nil && hm.callbacks.OnHealthy != nil && from != HealthStateHealthy {
			hm.callbacks.OnHealthy(health.name)
		}
	})

	builder.OnEnter(HealthStateDegraded, func(_ context.Context, _ HealthState, from HealthState) {
		health.mu.Lock()
		health.State = HealthStateDegraded
		health.mu.Unlock()

		if hm.callbacks != nil && hm.callbacks.OnDegraded != nil && from != HealthStateDegraded {
			hm.callbacks.OnDegraded(health.name)
		}
	})

	builder.OnEnter(HealthStateUnhealthy, func(_ context.Context, _ HealthState, from HealthState) {
		health.mu.Lock()
		health.State = HealthStateUnhealthy
		health.mu.Unlock()

		if hm.callbacks != nil && hm.callbacks.OnUnhealthy != nil && from != HealthStateUnhealthy {
			hm.callbacks.OnUnhealthy(health.name)
		}
	})

	// Global transition callback
	builder.OnTransition(func(_ context.Context, from, to HealthState, _ HealthEvent) {
		if hm.callbacks != nil && hm.callbacks.OnStateChange != nil && from != to {
			hm.callbacks.OnStateChange(health.name, from, to)
		}
	})

	return builder.MustBuild()
}

// Register registers a backend for health monitoring.
func (hm *HealthMonitor) Register(name string, backend SecretBackend) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	health := &BackendHealth{
		Backend:        backend,
		name:           name,
		State:          HealthStateUnknown,
		CircuitBreaker: NewCircuitBreaker(hm.config.CircuitBreaker),
		config:         hm.config,
		callbacks:      hm.callbacks,
	}
	health.machine = hm.buildHealthStateMachine(health)
	hm.backends[name] = health
}

// Unregister removes a backend from health monitoring.
func (hm *HealthMonitor) Unregister(name string) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	delete(hm.backends, name)
}

// GetHealth returns the health of a backend.
func (hm *HealthMonitor) GetHealth(name string) (*BackendHealth, bool) {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	health, ok := hm.backends[name]
	return health, ok
}

// GetAllHealth returns the health of all backends.
func (hm *HealthMonitor) GetAllHealth() map[string]*BackendHealth {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	result := make(map[string]*BackendHealth, len(hm.backends))
	for name, health := range hm.backends {
		result[name] = health
	}
	return result
}

// IsHealthy returns true if the backend is healthy.
func (hm *HealthMonitor) IsHealthy(name string) bool {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	health, ok := hm.backends[name]
	if !ok {
		return false
	}

	return health.State == HealthStateHealthy || health.State == HealthStateDegraded
}

// AllowRequest returns true if a request to the backend should be allowed.
func (hm *HealthMonitor) AllowRequest(name string) bool {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	health, ok := hm.backends[name]
	if !ok {
		return false
	}

	return health.CircuitBreaker.AllowRequest()
}

// RecordSuccess records a successful request to a backend.
func (hm *HealthMonitor) RecordSuccess(name string) {
	hm.mu.RLock()
	health, ok := hm.backends[name]
	hm.mu.RUnlock()

	if ok {
		health.CircuitBreaker.RecordSuccess()
	}
}

// RecordFailure records a failed request to a backend.
func (hm *HealthMonitor) RecordFailure(name string, err error) {
	hm.mu.Lock()
	health, ok := hm.backends[name]
	if ok {
		health.CircuitBreaker.RecordFailure()
		health.LastError = err
	}
	hm.mu.Unlock()
}

// Start starts the health monitor.
func (hm *HealthMonitor) Start(ctx context.Context) error {
	hm.mu.Lock()
	if hm.running {
		hm.mu.Unlock()
		return nil
	}
	hm.running = true
	hm.stopCh = make(chan struct{})
	hm.mu.Unlock()

	// Do initial health check
	hm.checkAllBackends(ctx)

	// Start background health checks
	go hm.run(ctx)

	return nil
}

// Stop stops the health monitor.
func (hm *HealthMonitor) Stop() error {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	if !hm.running {
		return nil
	}

	close(hm.stopCh)
	hm.running = false
	return nil
}

// run is the main health check loop.
func (hm *HealthMonitor) run(ctx context.Context) {
	ticker := time.NewTicker(hm.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-hm.stopCh:
			return
		case <-ticker.C:
			hm.checkAllBackends(ctx)
		}
	}
}

// checkAllBackends checks the health of all backends.
func (hm *HealthMonitor) checkAllBackends(ctx context.Context) {
	hm.mu.RLock()
	backends := make(map[string]*BackendHealth, len(hm.backends))
	for name, health := range hm.backends {
		backends[name] = health
	}
	hm.mu.RUnlock()

	for name, health := range backends {
		hm.checkBackend(ctx, name, health)
	}
}

// checkBackend checks the health of a single backend.
func (hm *HealthMonitor) checkBackend(ctx context.Context, name string, health *BackendHealth) {
	checkCtx, cancel := context.WithTimeout(ctx, hm.config.Timeout)
	defer cancel()

	healthy := health.Backend.Healthy(checkCtx)

	health.mu.Lock()
	health.LastCheck = time.Now()
	health.TotalChecks++

	if healthy {
		health.ConsecutiveSuccesses++
		health.ConsecutiveFailures = 0
		health.LastHealthy = time.Now()
		health.LastError = nil
	} else {
		health.ConsecutiveFailures++
		health.ConsecutiveSuccesses = 0
		health.TotalFailures++
	}
	health.mu.Unlock()

	// Fire state machine event (callbacks will update State)
	if healthy {
		_ = health.machine.Fire(HealthEventCheckSuccess)
	} else {
		_ = health.machine.Fire(HealthEventCheckFailure)
	}
}

// FailoverPolicy defines how failover should occur.
type FailoverPolicy struct {
	// Enabled enables automatic failover.
	Enabled bool `json:"enabled"`

	// MaxAttempts is the maximum number of backends to try.
	MaxAttempts int `json:"max_attempts,omitempty"`

	// RetryDelay is the delay between failover attempts.
	RetryDelay time.Duration `json:"retry_delay,omitempty"`

	// PreferHealthy prefers healthy backends over unhealthy ones.
	PreferHealthy bool `json:"prefer_healthy"`

	// FailbackEnabled enables automatic failback to primary when it recovers.
	FailbackEnabled bool `json:"failback_enabled"`

	// FailbackDelay is how long to wait before failing back.
	FailbackDelay time.Duration `json:"failback_delay,omitempty"`
}

// DefaultFailoverPolicy returns a default failover policy.
func DefaultFailoverPolicy() *FailoverPolicy {
	return &FailoverPolicy{
		Enabled:         true,
		MaxAttempts:     3,
		RetryDelay:      100 * time.Millisecond,
		PreferHealthy:   true,
		FailbackEnabled: true,
		FailbackDelay:   5 * time.Minute,
	}
}

// BackendGroup manages a group of backends with failover support.
type BackendGroup struct {
	mu sync.RWMutex

	// name is the group name.
	name string

	// backends is the ordered list of backends (primary first).
	backends []string

	// backendMap maps backend names to backends.
	backendMap map[string]SecretBackend

	// healthMonitor monitors backend health.
	healthMonitor *HealthMonitor

	// policy is the failover policy.
	policy *FailoverPolicy

	// activeBackend is the currently active backend.
	activeBackend string

	// failoverTime is when failover last occurred.
	failoverTime time.Time
}

// NewBackendGroup creates a new backend group.
func NewBackendGroup(name string, healthMonitor *HealthMonitor, policy *FailoverPolicy) *BackendGroup {
	if policy == nil {
		policy = DefaultFailoverPolicy()
	}

	return &BackendGroup{
		name:          name,
		backends:      make([]string, 0),
		backendMap:    make(map[string]SecretBackend),
		healthMonitor: healthMonitor,
		policy:        policy,
	}
}

// AddBackend adds a backend to the group.
func (bg *BackendGroup) AddBackend(name string, backend SecretBackend, isPrimary bool) {
	bg.mu.Lock()
	defer bg.mu.Unlock()

	bg.backendMap[name] = backend

	if isPrimary {
		bg.backends = append([]string{name}, bg.backends...)
		if bg.activeBackend == "" {
			bg.activeBackend = name
		}
	} else {
		bg.backends = append(bg.backends, name)
	}

	if bg.healthMonitor != nil {
		bg.healthMonitor.Register(name, backend)
	}
}

// RemoveBackend removes a backend from the group.
func (bg *BackendGroup) RemoveBackend(name string) {
	bg.mu.Lock()
	defer bg.mu.Unlock()

	delete(bg.backendMap, name)

	newBackends := make([]string, 0, len(bg.backends)-1)
	for _, b := range bg.backends {
		if b != name {
			newBackends = append(newBackends, b)
		}
	}
	bg.backends = newBackends

	if bg.activeBackend == name {
		bg.activeBackend = ""
		if len(bg.backends) > 0 {
			bg.activeBackend = bg.backends[0]
		}
	}

	if bg.healthMonitor != nil {
		bg.healthMonitor.Unregister(name)
	}
}

// GetActiveBackend returns the currently active backend.
func (bg *BackendGroup) GetActiveBackend() (SecretBackend, string, error) {
	bg.mu.RLock()
	defer bg.mu.RUnlock()

	if bg.activeBackend == "" {
		return nil, "", ErrBackendNotFound
	}

	backend, ok := bg.backendMap[bg.activeBackend]
	if !ok {
		return nil, "", ErrBackendNotFound
	}

	return backend, bg.activeBackend, nil
}

// SelectBackend selects the best available backend.
func (bg *BackendGroup) SelectBackend(ctx context.Context) (SecretBackend, string, error) {
	bg.mu.Lock()
	defer bg.mu.Unlock()

	// Check if we should failback to primary
	if bg.policy.FailbackEnabled && len(bg.backends) > 0 {
		primary := bg.backends[0]
		if bg.activeBackend != primary && !bg.failoverTime.IsZero() {
			if time.Since(bg.failoverTime) >= bg.policy.FailbackDelay {
				if bg.healthMonitor != nil && bg.healthMonitor.IsHealthy(primary) {
					bg.activeBackend = primary
					bg.failoverTime = time.Time{}
				}
			}
		}
	}

	// Try backends in order, preferring healthy ones
	var lastErr error
	attempts := 0

	// First pass: try healthy backends
	if bg.policy.PreferHealthy {
		for _, name := range bg.backends {
			if attempts >= bg.policy.MaxAttempts {
				break
			}

			if bg.healthMonitor != nil && !bg.healthMonitor.AllowRequest(name) { //nolint:contextcheck // AllowRequest uses internal state machine that doesn't need context
				continue
			}

			backend, ok := bg.backendMap[name]
			if !ok {
				continue
			}

			// Quick health check
			if backend.Healthy(ctx) {
				if bg.activeBackend != name {
					bg.failoverTime = time.Now()
				}
				bg.activeBackend = name
				return backend, name, nil
			}

			attempts++
		}
	}

	// Second pass: try any available backend
	for _, name := range bg.backends {
		if attempts >= bg.policy.MaxAttempts {
			break
		}

		backend, ok := bg.backendMap[name]
		if !ok {
			continue
		}

		if bg.activeBackend != name {
			bg.failoverTime = time.Now()
		}
		bg.activeBackend = name
		return backend, name, nil
	}

	if lastErr != nil {
		return nil, "", lastErr
	}
	return nil, "", ErrBackendNotFound
}

// ExecuteWithFailover executes an operation with automatic failover.
func (bg *BackendGroup) ExecuteWithFailover(ctx context.Context, op func(backend SecretBackend) error) error {
	bg.mu.RLock()
	backends := make([]string, len(bg.backends))
	copy(backends, bg.backends)
	backendMap := make(map[string]SecretBackend, len(bg.backendMap))
	for k, v := range bg.backendMap {
		backendMap[k] = v
	}
	bg.mu.RUnlock()

	var lastErr error
	attempts := 0

	// Sort backends by health if preferred
	if bg.policy.PreferHealthy && bg.healthMonitor != nil {
		sortedBackends := make([]string, 0, len(backends))
		unhealthy := make([]string, 0)

		for _, name := range backends {
			if bg.healthMonitor.IsHealthy(name) {
				sortedBackends = append(sortedBackends, name)
			} else {
				unhealthy = append(unhealthy, name)
			}
		}
		sortedBackends = append(sortedBackends, unhealthy...)
		backends = sortedBackends
	}

	for _, name := range backends {
		if attempts >= bg.policy.MaxAttempts {
			break
		}

		if bg.healthMonitor != nil && !bg.healthMonitor.AllowRequest(name) { //nolint:contextcheck // AllowRequest uses internal state machine that doesn't need context
			continue
		}

		backend, ok := backendMap[name]
		if !ok {
			continue
		}

		err := op(backend)
		if err == nil {
			if bg.healthMonitor != nil {
				bg.healthMonitor.RecordSuccess(name) //nolint:contextcheck // RecordSuccess updates internal state, doesn't need context
			}
			return nil
		}

		lastErr = err
		attempts++

		if bg.healthMonitor != nil {
			bg.healthMonitor.RecordFailure(name, err) //nolint:contextcheck // RecordFailure updates internal state, doesn't need context
		}

		// Don't retry for non-retryable errors
		if !isRetryableError(err) {
			return err
		}

		if bg.policy.RetryDelay > 0 && attempts < bg.policy.MaxAttempts {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(bg.policy.RetryDelay):
			}
		}
	}

	if lastErr != nil {
		return lastErr
	}
	return ErrBackendNotFound
}

// isRetryableError returns true if the error is retryable.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Non-retryable errors
	if errors.Is(err, ErrSecretNotFound) || errors.Is(err, ErrAccessDenied) ||
		errors.Is(err, ErrInvalidPath) || errors.Is(err, ErrLeaseNotFound) {
		return false
	}

	// Retryable errors
	if errors.Is(err, ErrBackendUnavailable) {
		return true
	}

	// Check for context errors
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return false
	}

	// Default to retryable for unknown errors (network issues, etc.)
	return true
}

// BackendGroupStats contains backend group statistics.
type BackendGroupStats struct {
	// Name is the group name.
	Name string `json:"name"`

	// ActiveBackend is the currently active backend.
	ActiveBackend string `json:"active_backend"`

	// BackendCount is the total number of backends.
	BackendCount int `json:"backend_count"`

	// HealthyCount is the number of healthy backends.
	HealthyCount int `json:"healthy_count"`

	// FailoverTime is when failover last occurred.
	FailoverTime time.Time `json:"failover_time,omitempty"`

	// BackendHealth contains health info for each backend.
	BackendHealth map[string]*BackendHealthStats `json:"backend_health,omitempty"`
}

// BackendHealthStats contains health statistics for a backend.
type BackendHealthStats struct {
	// State is the current health state.
	State string `json:"state"`

	// LastCheck is when the health was last checked.
	LastCheck time.Time `json:"last_check,omitempty"`

	// LastHealthy is when the backend was last healthy.
	LastHealthy time.Time `json:"last_healthy,omitempty"`

	// TotalChecks is the total number of health checks.
	TotalChecks int64 `json:"total_checks"`

	// TotalFailures is the total number of failed checks.
	TotalFailures int64 `json:"total_failures"`

	// CircuitBreaker contains circuit breaker stats.
	CircuitBreaker *CircuitBreakerStats `json:"circuit_breaker,omitempty"`
}

// Stats returns backend group statistics.
func (bg *BackendGroup) Stats() *BackendGroupStats {
	bg.mu.RLock()
	defer bg.mu.RUnlock()

	stats := &BackendGroupStats{
		Name:          bg.name,
		ActiveBackend: bg.activeBackend,
		BackendCount:  len(bg.backends),
		FailoverTime:  bg.failoverTime,
		BackendHealth: make(map[string]*BackendHealthStats),
	}

	if bg.healthMonitor != nil {
		for _, name := range bg.backends {
			health, ok := bg.healthMonitor.GetHealth(name)
			if !ok {
				continue
			}

			if health.State == HealthStateHealthy {
				stats.HealthyCount++
			}

			stats.BackendHealth[name] = &BackendHealthStats{
				State:          health.State.String(),
				LastCheck:      health.LastCheck,
				LastHealthy:    health.LastHealthy,
				TotalChecks:    health.TotalChecks,
				TotalFailures:  health.TotalFailures,
				CircuitBreaker: health.CircuitBreaker.Stats(),
			}
		}
	}

	return stats
}
