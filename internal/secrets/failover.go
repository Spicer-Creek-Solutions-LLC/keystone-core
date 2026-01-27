package secrets

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
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

// CircuitBreaker implements the circuit breaker pattern for a backend.
type CircuitBreaker struct {
	config *CircuitBreakerConfig

	state           atomic.Int32
	failureCount    atomic.Int32
	successCount    atomic.Int32
	halfOpenCount   atomic.Int32
	lastFailureTime atomic.Int64
	lastStateChange atomic.Int64

	mu sync.RWMutex
}

// NewCircuitBreaker creates a new circuit breaker.
func NewCircuitBreaker(config *CircuitBreakerConfig) *CircuitBreaker {
	if config == nil {
		config = DefaultCircuitBreakerConfig()
	}

	cb := &CircuitBreaker{
		config: config,
	}
	cb.state.Store(int32(CircuitStateClosed))
	cb.lastStateChange.Store(time.Now().UnixNano())
	return cb
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() CircuitState {
	return CircuitState(cb.state.Load())
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
			cb.transitionTo(CircuitStateHalfOpen)
			return cb.tryHalfOpen()
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
		cb.failureCount.Store(0)

	case CircuitStateHalfOpen:
		count := cb.successCount.Add(1)
		if int(count) >= cb.config.SuccessThreshold {
			cb.transitionTo(CircuitStateClosed)
		}
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
			cb.transitionTo(CircuitStateOpen)
		}

	case CircuitStateHalfOpen:
		cb.transitionTo(CircuitStateOpen)
	}
}

// transitionTo transitions to a new state.
func (cb *CircuitBreaker) transitionTo(newState CircuitState) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	oldState := CircuitState(cb.state.Load())
	if oldState == newState {
		return
	}

	cb.state.Store(int32(newState))
	cb.lastStateChange.Store(time.Now().UnixNano())

	// Reset counters on state change
	cb.failureCount.Store(0)
	cb.successCount.Store(0)
	cb.halfOpenCount.Store(0)
}

// Reset resets the circuit breaker to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.transitionTo(CircuitStateClosed)
}

// Stats returns circuit breaker statistics.
func (cb *CircuitBreaker) Stats() *CircuitBreakerStats {
	return &CircuitBreakerStats{
		State:           cb.State(),
		FailureCount:    int(cb.failureCount.Load()),
		SuccessCount:    int(cb.successCount.Load()),
		LastFailureTime: time.Unix(0, cb.lastFailureTime.Load()),
		LastStateChange: time.Unix(0, cb.lastStateChange.Load()),
	}
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
		Timeout:           5 * time.Second,
		HealthyThreshold:   2,
		UnhealthyThreshold: 3,
		CircuitBreaker:     DefaultCircuitBreakerConfig(),
	}
}

// BackendHealth tracks the health of a single backend.
type BackendHealth struct {
	// Backend is the backend being monitored.
	Backend SecretBackend

	// State is the current health state.
	State HealthState

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
}

// NewHealthMonitor creates a new health monitor.
func NewHealthMonitor(config *HealthMonitorConfig) *HealthMonitor {
	if config == nil {
		config = DefaultHealthMonitorConfig()
	}

	return &HealthMonitor{
		backends: make(map[string]*BackendHealth),
		config:   config,
		stopCh:   make(chan struct{}),
	}
}

// Register registers a backend for health monitoring.
func (hm *HealthMonitor) Register(name string, backend SecretBackend) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	hm.backends[name] = &BackendHealth{
		Backend:        backend,
		State:          HealthStateUnknown,
		CircuitBreaker: NewCircuitBreaker(hm.config.CircuitBreaker),
	}
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

	hm.mu.Lock()
	defer hm.mu.Unlock()

	health.LastCheck = time.Now()
	health.TotalChecks++

	if healthy {
		health.ConsecutiveSuccesses++
		health.ConsecutiveFailures = 0
		health.LastHealthy = time.Now()
		health.LastError = nil

		if health.ConsecutiveSuccesses >= hm.config.HealthyThreshold {
			health.State = HealthStateHealthy
		}
	} else {
		health.ConsecutiveFailures++
		health.ConsecutiveSuccesses = 0
		health.TotalFailures++

		if health.ConsecutiveFailures >= hm.config.UnhealthyThreshold {
			health.State = HealthStateUnhealthy
		} else if health.State == HealthStateHealthy {
			health.State = HealthStateDegraded
		}
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

			if bg.healthMonitor != nil && !bg.healthMonitor.AllowRequest(name) {
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

		if bg.healthMonitor != nil && !bg.healthMonitor.AllowRequest(name) {
			continue
		}

		backend, ok := backendMap[name]
		if !ok {
			continue
		}

		err := op(backend)
		if err == nil {
			if bg.healthMonitor != nil {
				bg.healthMonitor.RecordSuccess(name)
			}
			return nil
		}

		lastErr = err
		attempts++

		if bg.healthMonitor != nil {
			bg.healthMonitor.RecordFailure(name, err)
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
	switch err {
	case ErrSecretNotFound, ErrAccessDenied, ErrInvalidPath, ErrLeaseNotFound:
		return false
	}

	// Retryable errors
	switch err {
	case ErrBackendUnavailable:
		return true
	}

	// Check for context errors
	if err == context.DeadlineExceeded || err == context.Canceled {
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
