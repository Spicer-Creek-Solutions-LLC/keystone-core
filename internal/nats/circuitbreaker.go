package nats

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// Circuit Breaker - T8.4
// ============================================================================

// CircuitState represents the circuit breaker state
type CircuitState string

const (
	// CircuitStateClosed means the circuit is closed (normal operation)
	CircuitStateClosed CircuitState = "closed"
	// CircuitStateOpen means the circuit is open (failing fast)
	CircuitStateOpen CircuitState = "open"
	// CircuitStateHalfOpen means the circuit is half-open (testing recovery)
	CircuitStateHalfOpen CircuitState = "half_open"
)

var (
	// ErrCircuitOpen is returned when the circuit is open
	ErrCircuitOpen = errors.New("circuit breaker is open")
	// ErrCircuitHalfOpen is returned when too many requests during half-open
	ErrCircuitHalfOpen = errors.New("circuit breaker is half-open, limited requests allowed")
)

// AdvancedCircuitBreakerConfig holds advanced circuit breaker configuration
// with rate-based tripping and sampling window support
type AdvancedCircuitBreakerConfig struct {
	// FailureThreshold is the number of failures to trip the circuit
	FailureThreshold int

	// SuccessThreshold is the number of successes to close the circuit
	SuccessThreshold int

	// Timeout is how long to stay open before trying half-open
	Timeout time.Duration

	// HalfOpenMaxRequests is max concurrent requests in half-open state
	HalfOpenMaxRequests int

	// FailureRateThreshold is the failure rate (0-1) to trip the circuit
	FailureRateThreshold float64

	// MinimumRequests is the minimum requests before rate threshold applies
	MinimumRequests int

	// SamplingWindow is the time window for failure rate calculation
	SamplingWindow time.Duration

	// OnStateChange is called when state changes
	OnStateChange func(name string, from, to CircuitState)

	// IsFailure determines if an error should count as a failure
	IsFailure func(error) bool
}

// DefaultAdvancedCircuitBreakerConfig returns sensible defaults
func DefaultAdvancedCircuitBreakerConfig() *AdvancedCircuitBreakerConfig {
	return &AdvancedCircuitBreakerConfig{
		FailureThreshold:     5,
		SuccessThreshold:     3,
		Timeout:              30 * time.Second,
		HalfOpenMaxRequests:  1,
		FailureRateThreshold: 0.5,
		MinimumRequests:      10,
		SamplingWindow:       60 * time.Second,
		IsFailure:            defaultIsFailure,
	}
}

func defaultIsFailure(err error) bool {
	return err != nil
}

// Validate validates the configuration
func (c *AdvancedCircuitBreakerConfig) Validate() error {
	if c.FailureThreshold <= 0 {
		return errors.New("failure threshold must be positive")
	}
	if c.SuccessThreshold <= 0 {
		return errors.New("success threshold must be positive")
	}
	if c.Timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	if c.HalfOpenMaxRequests <= 0 {
		return errors.New("half-open max requests must be positive")
	}
	if c.FailureRateThreshold < 0 || c.FailureRateThreshold > 1 {
		return errors.New("failure rate threshold must be between 0 and 1")
	}
	return nil
}

// CircuitBreakerStats holds circuit breaker statistics
type CircuitBreakerStats struct {
	// State is the current state
	State CircuitState

	// TotalRequests is total requests
	TotalRequests int64

	// TotalSuccesses is total successes
	TotalSuccesses int64

	// TotalFailures is total failures
	TotalFailures int64

	// ConsecutiveSuccesses is current consecutive successes
	ConsecutiveSuccesses int64

	// ConsecutiveFailures is current consecutive failures
	ConsecutiveFailures int64

	// LastFailure is the last failure time
	LastFailure time.Time

	// LastSuccess is the last success time
	LastSuccess time.Time

	// LastStateChange is when state last changed
	LastStateChange time.Time

	// StateChanges is total state changes
	StateChanges int64

	// FailureRate is the current failure rate
	FailureRate float64

	// OpenDuration is total time spent in open state
	OpenDuration time.Duration
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	name   string
	config *AdvancedCircuitBreakerConfig

	// State
	state           CircuitState
	stateMu         sync.RWMutex
	lastStateChange time.Time

	// Counters
	consecutiveSuccesses int64
	consecutiveFailures  int64
	totalRequests        int64
	totalSuccesses       int64
	totalFailures        int64
	stateChanges         int64

	// Timestamps
	lastFailure time.Time
	lastSuccess time.Time

	// Half-open tracking
	halfOpenRequests int32

	// Sliding window for failure rate
	windowRequests []windowEntry
	windowMu       sync.Mutex

	// Open duration tracking
	openStart    time.Time
	openDuration time.Duration
}

type windowEntry struct {
	timestamp time.Time
	success   bool
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(name string, config *AdvancedCircuitBreakerConfig) (*CircuitBreaker, error) {
	if name == "" {
		return nil, errors.New("name is required")
	}
	if config == nil {
		config = DefaultAdvancedCircuitBreakerConfig()
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &CircuitBreaker{
		name:            name,
		config:          config,
		state:           CircuitStateClosed,
		lastStateChange: time.Now(),
		windowRequests:  make([]windowEntry, 0, config.MinimumRequests),
	}, nil
}

// Name returns the circuit breaker name
func (cb *CircuitBreaker) Name() string {
	return cb.name
}

// State returns the current state
func (cb *CircuitBreaker) State() CircuitState {
	cb.stateMu.RLock()
	defer cb.stateMu.RUnlock()
	return cb.state
}

// Allow checks if a request should be allowed
func (cb *CircuitBreaker) Allow() error {
	cb.stateMu.RLock()
	state := cb.state
	cb.stateMu.RUnlock()

	switch state {
	case CircuitStateClosed:
		return nil

	case CircuitStateOpen:
		// Check if timeout has elapsed
		cb.stateMu.Lock()
		if time.Since(cb.lastStateChange) >= cb.config.Timeout {
			cb.transitionTo(CircuitStateHalfOpen)
			cb.stateMu.Unlock()
			return nil
		}
		cb.stateMu.Unlock()
		return ErrCircuitOpen

	case CircuitStateHalfOpen:
		// Allow limited requests in half-open
		current := atomic.LoadInt32(&cb.halfOpenRequests)
		if current >= int32(cb.config.HalfOpenMaxRequests) {
			return ErrCircuitHalfOpen
		}
		atomic.AddInt32(&cb.halfOpenRequests, 1)
		return nil

	default:
		return nil
	}
}

// Execute executes a function with circuit breaker protection
func (cb *CircuitBreaker) Execute(fn func() error) error {
	if err := cb.Allow(); err != nil {
		return err
	}

	err := fn()
	cb.RecordResult(err)
	return err
}

// ExecuteWithContext executes a function with context and circuit breaker
func (cb *CircuitBreaker) ExecuteWithContext(ctx context.Context, fn func(context.Context) error) error {
	if err := cb.Allow(); err != nil {
		return err
	}

	err := fn(ctx)
	cb.RecordResult(err)
	return err
}

// RecordResult records the result of an operation
func (cb *CircuitBreaker) RecordResult(err error) {
	atomic.AddInt64(&cb.totalRequests, 1)

	isFailure := cb.config.IsFailure != nil && cb.config.IsFailure(err)

	cb.stateMu.Lock()
	defer cb.stateMu.Unlock()

	if isFailure {
		cb.recordFailure()
	} else {
		cb.recordSuccess()
	}
}

func (cb *CircuitBreaker) recordSuccess() {
	atomic.AddInt64(&cb.totalSuccesses, 1)
	atomic.AddInt64(&cb.consecutiveSuccesses, 1)
	atomic.StoreInt64(&cb.consecutiveFailures, 0)
	cb.lastSuccess = time.Now()

	// Add to window
	cb.addWindowEntry(true)

	switch cb.state {
	case CircuitStateHalfOpen:
		atomic.AddInt32(&cb.halfOpenRequests, -1)
		if atomic.LoadInt64(&cb.consecutiveSuccesses) >= int64(cb.config.SuccessThreshold) {
			cb.transitionTo(CircuitStateClosed)
		}
	}
}

func (cb *CircuitBreaker) recordFailure() {
	atomic.AddInt64(&cb.totalFailures, 1)
	atomic.AddInt64(&cb.consecutiveFailures, 1)
	atomic.StoreInt64(&cb.consecutiveSuccesses, 0)
	cb.lastFailure = time.Now()

	// Add to window
	cb.addWindowEntry(false)

	switch cb.state {
	case CircuitStateClosed:
		// Check failure threshold
		if atomic.LoadInt64(&cb.consecutiveFailures) >= int64(cb.config.FailureThreshold) {
			cb.transitionTo(CircuitStateOpen)
			return
		}

		// Check failure rate
		if cb.shouldTripByRate() {
			cb.transitionTo(CircuitStateOpen)
		}

	case CircuitStateHalfOpen:
		atomic.AddInt32(&cb.halfOpenRequests, -1)
		cb.transitionTo(CircuitStateOpen)
	}
}

func (cb *CircuitBreaker) addWindowEntry(success bool) {
	cb.windowMu.Lock()
	defer cb.windowMu.Unlock()

	now := time.Now()
	cb.windowRequests = append(cb.windowRequests, windowEntry{
		timestamp: now,
		success:   success,
	})

	// Remove old entries
	cutoff := now.Add(-cb.config.SamplingWindow)
	kept := cb.windowRequests[:0]
	for _, entry := range cb.windowRequests {
		if entry.timestamp.After(cutoff) {
			kept = append(kept, entry)
		}
	}
	cb.windowRequests = kept
}

func (cb *CircuitBreaker) shouldTripByRate() bool {
	cb.windowMu.Lock()
	defer cb.windowMu.Unlock()

	if len(cb.windowRequests) < cb.config.MinimumRequests {
		return false
	}

	var failures int
	for _, entry := range cb.windowRequests {
		if !entry.success {
			failures++
		}
	}

	rate := float64(failures) / float64(len(cb.windowRequests))
	return rate >= cb.config.FailureRateThreshold
}

func (cb *CircuitBreaker) transitionTo(newState CircuitState) {
	oldState := cb.state
	if oldState == newState {
		return
	}

	now := time.Now()

	// Track open duration
	if oldState == CircuitStateOpen {
		cb.openDuration += now.Sub(cb.openStart)
	}
	if newState == CircuitStateOpen {
		cb.openStart = now
	}

	cb.state = newState
	cb.lastStateChange = now
	atomic.AddInt64(&cb.stateChanges, 1)

	// Reset counters on state change
	atomic.StoreInt64(&cb.consecutiveSuccesses, 0)
	atomic.StoreInt64(&cb.consecutiveFailures, 0)
	atomic.StoreInt32(&cb.halfOpenRequests, 0)

	// Notify callback
	if cb.config.OnStateChange != nil {
		go cb.config.OnStateChange(cb.name, oldState, newState)
	}
}

// Reset resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.stateMu.Lock()
	defer cb.stateMu.Unlock()

	cb.transitionTo(CircuitStateClosed)
	atomic.StoreInt64(&cb.consecutiveSuccesses, 0)
	atomic.StoreInt64(&cb.consecutiveFailures, 0)

	cb.windowMu.Lock()
	cb.windowRequests = cb.windowRequests[:0]
	cb.windowMu.Unlock()
}

// Trip manually trips the circuit breaker
func (cb *CircuitBreaker) Trip() {
	cb.stateMu.Lock()
	defer cb.stateMu.Unlock()

	if cb.state != CircuitStateOpen {
		cb.transitionTo(CircuitStateOpen)
	}
}

// GetStats returns circuit breaker statistics
func (cb *CircuitBreaker) GetStats() CircuitBreakerStats {
	cb.stateMu.RLock()
	defer cb.stateMu.RUnlock()

	var failureRate float64
	cb.windowMu.Lock()
	if len(cb.windowRequests) > 0 {
		var failures int
		for _, entry := range cb.windowRequests {
			if !entry.success {
				failures++
			}
		}
		failureRate = float64(failures) / float64(len(cb.windowRequests))
	}
	cb.windowMu.Unlock()

	openDuration := cb.openDuration
	if cb.state == CircuitStateOpen {
		openDuration += time.Since(cb.openStart)
	}

	return CircuitBreakerStats{
		State:                cb.state,
		TotalRequests:        atomic.LoadInt64(&cb.totalRequests),
		TotalSuccesses:       atomic.LoadInt64(&cb.totalSuccesses),
		TotalFailures:        atomic.LoadInt64(&cb.totalFailures),
		ConsecutiveSuccesses: atomic.LoadInt64(&cb.consecutiveSuccesses),
		ConsecutiveFailures:  atomic.LoadInt64(&cb.consecutiveFailures),
		LastFailure:          cb.lastFailure,
		LastSuccess:          cb.lastSuccess,
		LastStateChange:      cb.lastStateChange,
		StateChanges:         atomic.LoadInt64(&cb.stateChanges),
		FailureRate:          failureRate,
		OpenDuration:         openDuration,
	}
}

// ============================================================================
// Circuit Breaker Manager - Per-Endpoint Circuit Breakers
// ============================================================================

// CircuitBreakerManager manages multiple circuit breakers
type CircuitBreakerManager struct {
	config *AdvancedCircuitBreakerConfig

	breakers map[string]*CircuitBreaker
	mu       sync.RWMutex

	// Callbacks
	onOpen  func(name string)
	onClose func(name string)
}

// NewCircuitBreakerManager creates a new circuit breaker manager
func NewCircuitBreakerManager(config *AdvancedCircuitBreakerConfig) *CircuitBreakerManager {
	if config == nil {
		config = DefaultAdvancedCircuitBreakerConfig()
	}

	// Wrap the state change callback
	originalCallback := config.OnStateChange
	config.OnStateChange = nil // We'll handle this in the manager

	manager := &CircuitBreakerManager{
		config:   config,
		breakers: make(map[string]*CircuitBreaker),
	}

	// Create wrapper callback
	config.OnStateChange = func(name string, from, to CircuitState) {
		if originalCallback != nil {
			originalCallback(name, from, to)
		}

		switch to {
		case CircuitStateOpen:
			if manager.onOpen != nil {
				manager.onOpen(name)
			}
		case CircuitStateClosed:
			if from == CircuitStateHalfOpen && manager.onClose != nil {
				manager.onClose(name)
			}
		}
	}

	return manager
}

// GetOrCreate gets or creates a circuit breaker for an endpoint
func (m *CircuitBreakerManager) GetOrCreate(name string) *CircuitBreaker {
	m.mu.RLock()
	if cb, exists := m.breakers[name]; exists {
		m.mu.RUnlock()
		return cb
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if cb, exists := m.breakers[name]; exists {
		return cb
	}

	cb, _ := NewCircuitBreaker(name, m.config)
	m.breakers[name] = cb
	return cb
}

// Get gets a circuit breaker by name
func (m *CircuitBreakerManager) Get(name string) *CircuitBreaker {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.breakers[name]
}

// Remove removes a circuit breaker
func (m *CircuitBreakerManager) Remove(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.breakers, name)
}

// ResetAll resets all circuit breakers
func (m *CircuitBreakerManager) ResetAll() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, cb := range m.breakers {
		cb.Reset()
	}
}

// GetAllStats returns stats for all circuit breakers
func (m *CircuitBreakerManager) GetAllStats() map[string]CircuitBreakerStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]CircuitBreakerStats, len(m.breakers))
	for name, cb := range m.breakers {
		stats[name] = cb.GetStats()
	}
	return stats
}

// GetOpenCircuits returns names of open circuits
func (m *CircuitBreakerManager) GetOpenCircuits() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var open []string
	for name, cb := range m.breakers {
		if cb.State() == CircuitStateOpen {
			open = append(open, name)
		}
	}
	return open
}

// Count returns the number of circuit breakers
func (m *CircuitBreakerManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.breakers)
}

// SetOpenCallback sets the callback for circuit open events
func (m *CircuitBreakerManager) SetOpenCallback(fn func(name string)) {
	m.onOpen = fn
}

// SetCloseCallback sets the callback for circuit close events
func (m *CircuitBreakerManager) SetCloseCallback(fn func(name string)) {
	m.onClose = fn
}

// Execute executes with the appropriate circuit breaker
func (m *CircuitBreakerManager) Execute(name string, fn func() error) error {
	cb := m.GetOrCreate(name)
	return cb.Execute(fn)
}

// AllowAll returns true if all circuit breakers allow requests
func (m *CircuitBreakerManager) AllowAll() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, cb := range m.breakers {
		if err := cb.Allow(); err != nil {
			return false
		}
	}
	return true
}
