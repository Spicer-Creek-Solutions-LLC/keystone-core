// Package circuitbreaker provides circuit breaker patterns for NATS messaging.
package circuitbreaker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Common errors.
var (
	ErrCircuitOpen     = errors.New("circuit breaker is open")
	ErrTooManyRequests = errors.New("too many requests in half-open state")
)

// State represents the circuit breaker state.
type State int32

const (
	// StateClosed allows requests to flow normally.
	StateClosed State = iota
	// StateOpen blocks all requests.
	StateOpen
	// StateHalfOpen allows limited requests to test recovery.
	StateHalfOpen
)

// String returns the string representation of the state.
func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Config configures a circuit breaker.
type Config struct {
	Name               string        `json:"name"`
	MaxFailures        int64         `json:"maxFailures"`
	Timeout            time.Duration `json:"timeout"`
	HalfOpenMaxRequests int64        `json:"halfOpenMaxRequests"`
	SuccessThreshold   int64         `json:"successThreshold"`
	FailureRateThreshold float64     `json:"failureRateThreshold"`
	MinRequests        int64         `json:"minRequests"`
	WindowSize         time.Duration `json:"windowSize"`
}

// DefaultConfig returns a default configuration.
func DefaultConfig() *Config {
	return &Config{
		Name:                "default",
		MaxFailures:         5,
		Timeout:             30 * time.Second,
		HalfOpenMaxRequests: 3,
		SuccessThreshold:    3,
		FailureRateThreshold: 0.5,
		MinRequests:         10,
		WindowSize:          60 * time.Second,
	}
}

// Counts contains request counts.
type Counts struct {
	Requests   int64 `json:"requests"`
	Successes  int64 `json:"successes"`
	Failures   int64 `json:"failures"`
	Timeouts   int64 `json:"timeouts"`
	Rejections int64 `json:"rejections"`
}

// FailureRate returns the failure rate.
func (c *Counts) FailureRate() float64 {
	if c.Requests == 0 {
		return 0
	}
	return float64(c.Failures) / float64(c.Requests)
}

// Reset resets all counts.
func (c *Counts) Reset() {
	c.Requests = 0
	c.Successes = 0
	c.Failures = 0
	c.Timeouts = 0
	c.Rejections = 0
}

// Breaker implements the circuit breaker pattern.
type Breaker struct {
	config    *Config
	state     int32 // atomic State
	counts    Counts
	lastFailure time.Time
	openedAt  time.Time
	mu        sync.RWMutex
	listeners []StateChangeListener
	halfOpenRequests int64 // atomic
}

// StateChangeEvent represents a state change event.
type StateChangeEvent struct {
	Name      string    `json:"name"`
	From      State     `json:"from"`
	To        State     `json:"to"`
	Counts    Counts    `json:"counts"`
	Timestamp time.Time `json:"timestamp"`
}

// StateChangeListener is called when state changes.
type StateChangeListener func(*StateChangeEvent)

// NewBreaker creates a new circuit breaker.
func NewBreaker(config *Config) *Breaker {
	if config == nil {
		config = DefaultConfig()
	}
	if config.MaxFailures <= 0 {
		config.MaxFailures = 5
	}
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}
	if config.HalfOpenMaxRequests <= 0 {
		config.HalfOpenMaxRequests = 3
	}
	if config.SuccessThreshold <= 0 {
		config.SuccessThreshold = 3
	}

	return &Breaker{
		config: config,
	}
}

// State returns the current state.
func (b *Breaker) State() State {
	return State(atomic.LoadInt32(&b.state))
}

// Name returns the breaker name.
func (b *Breaker) Name() string {
	return b.config.Name
}

// Counts returns the current counts.
func (b *Breaker) Counts() Counts {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.counts
}

// AddListener adds a state change listener.
func (b *Breaker) AddListener(listener StateChangeListener) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.listeners = append(b.listeners, listener)
}

// Allow checks if a request is allowed.
func (b *Breaker) Allow() error {
	state := b.State()

	switch state {
	case StateClosed:
		return nil

	case StateOpen:
		// Check if timeout has passed
		b.mu.RLock()
		openedAt := b.openedAt
		b.mu.RUnlock()

		if time.Since(openedAt) >= b.config.Timeout {
			b.transitionTo(StateHalfOpen)
			return b.Allow()
		}
		b.mu.Lock()
		b.counts.Rejections++
		b.mu.Unlock()
		return ErrCircuitOpen

	case StateHalfOpen:
		current := atomic.AddInt64(&b.halfOpenRequests, 1)
		if current > b.config.HalfOpenMaxRequests {
			atomic.AddInt64(&b.halfOpenRequests, -1)
			b.mu.Lock()
			b.counts.Rejections++
			b.mu.Unlock()
			return ErrTooManyRequests
		}
		return nil

	default:
		return nil
	}
}

// RecordSuccess records a successful request.
func (b *Breaker) RecordSuccess() {
	b.mu.Lock()
	b.counts.Requests++
	b.counts.Successes++
	counts := b.counts
	b.mu.Unlock()

	state := b.State()

	if state == StateHalfOpen {
		if counts.Successes >= b.config.SuccessThreshold {
			b.transitionTo(StateClosed)
		}
	}
}

// RecordFailure records a failed request.
func (b *Breaker) RecordFailure() {
	b.mu.Lock()
	b.counts.Requests++
	b.counts.Failures++
	b.lastFailure = time.Now()
	counts := b.counts
	b.mu.Unlock()

	state := b.State()

	switch state {
	case StateClosed:
		// Check if we should open
		if b.shouldTrip(counts) {
			b.transitionTo(StateOpen)
		}

	case StateHalfOpen:
		// Any failure in half-open returns to open
		b.transitionTo(StateOpen)
	}
}

// RecordTimeout records a timeout.
func (b *Breaker) RecordTimeout() {
	b.mu.Lock()
	b.counts.Timeouts++
	b.mu.Unlock()
	b.RecordFailure()
}

// Execute executes a function with circuit breaker protection.
func (b *Breaker) Execute(fn func() error) error {
	if err := b.Allow(); err != nil {
		return err
	}

	err := fn()
	if err != nil {
		b.RecordFailure()
		return err
	}

	b.RecordSuccess()
	return nil
}

// ExecuteWithContext executes a function with context and circuit breaker protection.
func (b *Breaker) ExecuteWithContext(ctx context.Context, fn func(context.Context) error) error {
	if err := b.Allow(); err != nil {
		return err
	}

	err := fn(ctx)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			b.RecordTimeout()
		} else {
			b.RecordFailure()
		}
		return err
	}

	b.RecordSuccess()
	return nil
}

// Reset resets the circuit breaker to closed state.
func (b *Breaker) Reset() {
	oldState := b.State()
	atomic.StoreInt32(&b.state, int32(StateClosed))
	atomic.StoreInt64(&b.halfOpenRequests, 0)

	b.mu.Lock()
	b.counts.Reset()
	b.mu.Unlock()

	if oldState != StateClosed {
		b.emit(&StateChangeEvent{
			Name:      b.config.Name,
			From:      oldState,
			To:        StateClosed,
			Timestamp: time.Now(),
		})
	}
}

func (b *Breaker) shouldTrip(counts Counts) bool {
	// Check absolute failure count
	if counts.Failures >= b.config.MaxFailures {
		return true
	}

	// Check failure rate if enough requests
	if b.config.MinRequests > 0 && counts.Requests >= b.config.MinRequests {
		if b.config.FailureRateThreshold > 0 && counts.FailureRate() >= b.config.FailureRateThreshold {
			return true
		}
	}

	return false
}

func (b *Breaker) transitionTo(newState State) {
	oldState := State(atomic.SwapInt32(&b.state, int32(newState)))
	if oldState == newState {
		return
	}

	b.mu.Lock()
	counts := b.counts

	switch newState {
	case StateClosed:
		b.counts.Reset()
		atomic.StoreInt64(&b.halfOpenRequests, 0)

	case StateOpen:
		b.openedAt = time.Now()

	case StateHalfOpen:
		b.counts.Reset()
		atomic.StoreInt64(&b.halfOpenRequests, 0)
	}
	b.mu.Unlock()

	b.emit(&StateChangeEvent{
		Name:      b.config.Name,
		From:      oldState,
		To:        newState,
		Counts:    counts,
		Timestamp: time.Now(),
	})
}

func (b *Breaker) emit(event *StateChangeEvent) {
	b.mu.RLock()
	listeners := b.listeners
	b.mu.RUnlock()

	for _, listener := range listeners {
		listener(event)
	}
}

// Registry manages multiple circuit breakers.
type Registry struct {
	breakers map[string]*Breaker
	mu       sync.RWMutex
	defaultConfig *Config
}

// NewRegistry creates a new registry.
func NewRegistry(defaultConfig *Config) *Registry {
	if defaultConfig == nil {
		defaultConfig = DefaultConfig()
	}
	return &Registry{
		breakers:      make(map[string]*Breaker),
		defaultConfig: defaultConfig,
	}
}

// Get returns a circuit breaker by name, creating if needed.
func (r *Registry) Get(name string) *Breaker {
	r.mu.RLock()
	breaker, ok := r.breakers[name]
	r.mu.RUnlock()

	if ok {
		return breaker
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double check
	if breaker, ok := r.breakers[name]; ok {
		return breaker
	}

	config := *r.defaultConfig
	config.Name = name
	breaker = NewBreaker(&config)
	r.breakers[name] = breaker

	return breaker
}

// GetWithConfig returns a circuit breaker with custom config.
func (r *Registry) GetWithConfig(name string, config *Config) *Breaker {
	r.mu.Lock()
	defer r.mu.Unlock()

	if breaker, ok := r.breakers[name]; ok {
		return breaker
	}

	config.Name = name
	breaker := NewBreaker(config)
	r.breakers[name] = breaker

	return breaker
}

// List returns all breakers.
func (r *Registry) List() []*Breaker {
	r.mu.RLock()
	defer r.mu.RUnlock()

	breakers := make([]*Breaker, 0, len(r.breakers))
	for _, b := range r.breakers {
		breakers = append(breakers, b)
	}
	return breakers
}

// Remove removes a circuit breaker.
func (r *Registry) Remove(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.breakers, name)
}

// ResetAll resets all circuit breakers.
func (r *Registry) ResetAll() {
	r.mu.RLock()
	breakers := make([]*Breaker, 0, len(r.breakers))
	for _, b := range r.breakers {
		breakers = append(breakers, b)
	}
	r.mu.RUnlock()

	for _, b := range breakers {
		b.Reset()
	}
}

// Stats returns statistics for all breakers.
func (r *Registry) Stats() map[string]BreakerStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := make(map[string]BreakerStats)
	for name, b := range r.breakers {
		stats[name] = BreakerStats{
			Name:   name,
			State:  b.State().String(),
			Counts: b.Counts(),
		}
	}
	return stats
}

// BreakerStats contains breaker statistics.
type BreakerStats struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Counts Counts `json:"counts"`
}

// WrappedFunc wraps a function with circuit breaker protection.
type WrappedFunc[T any] struct {
	breaker *Breaker
	fn      func() (T, error)
}

// Wrap creates a wrapped function.
func Wrap[T any](breaker *Breaker, fn func() (T, error)) *WrappedFunc[T] {
	return &WrappedFunc[T]{
		breaker: breaker,
		fn:      fn,
	}
}

// Execute executes the wrapped function.
func (w *WrappedFunc[T]) Execute() (T, error) {
	var zero T

	if err := w.breaker.Allow(); err != nil {
		return zero, err
	}

	result, err := w.fn()
	if err != nil {
		w.breaker.RecordFailure()
		return zero, err
	}

	w.breaker.RecordSuccess()
	return result, nil
}

// WindowedBreaker uses a sliding window for failure tracking.
type WindowedBreaker struct {
	config    *Config
	state     int32 // atomic State
	windows   []windowEntry
	windowMu  sync.RWMutex
	openedAt  time.Time
	listeners []StateChangeListener
	listenerMu sync.RWMutex
	halfOpenRequests int64 // atomic
}

type windowEntry struct {
	timestamp time.Time
	success   bool
}

// NewWindowedBreaker creates a new windowed circuit breaker.
func NewWindowedBreaker(config *Config) *WindowedBreaker {
	if config == nil {
		config = DefaultConfig()
	}
	if config.WindowSize <= 0 {
		config.WindowSize = 60 * time.Second
	}

	return &WindowedBreaker{
		config:  config,
		windows: make([]windowEntry, 0),
	}
}

// State returns the current state.
func (b *WindowedBreaker) State() State {
	return State(atomic.LoadInt32(&b.state))
}

// Allow checks if a request is allowed.
func (b *WindowedBreaker) Allow() error {
	state := b.State()

	switch state {
	case StateClosed:
		return nil

	case StateOpen:
		b.windowMu.RLock()
		openedAt := b.openedAt
		b.windowMu.RUnlock()

		if time.Since(openedAt) >= b.config.Timeout {
			b.transitionTo(StateHalfOpen)
			return b.Allow()
		}
		return ErrCircuitOpen

	case StateHalfOpen:
		current := atomic.AddInt64(&b.halfOpenRequests, 1)
		if current > b.config.HalfOpenMaxRequests {
			atomic.AddInt64(&b.halfOpenRequests, -1)
			return ErrTooManyRequests
		}
		return nil

	default:
		return nil
	}
}

// RecordSuccess records a successful request.
func (b *WindowedBreaker) RecordSuccess() {
	b.windowMu.Lock()
	b.windows = append(b.windows, windowEntry{
		timestamp: time.Now(),
		success:   true,
	})
	b.cleanWindow()
	b.windowMu.Unlock()

	state := b.State()
	if state == StateHalfOpen {
		b.windowMu.RLock()
		successes := b.countSuccesses()
		b.windowMu.RUnlock()

		if successes >= b.config.SuccessThreshold {
			b.transitionTo(StateClosed)
		}
	}
}

// RecordFailure records a failed request.
func (b *WindowedBreaker) RecordFailure() {
	b.windowMu.Lock()
	b.windows = append(b.windows, windowEntry{
		timestamp: time.Now(),
		success:   false,
	})
	b.cleanWindow()
	shouldTrip := b.shouldTrip()
	b.windowMu.Unlock()

	state := b.State()

	switch state {
	case StateClosed:
		if shouldTrip {
			b.transitionTo(StateOpen)
		}

	case StateHalfOpen:
		b.transitionTo(StateOpen)
	}
}

// AddListener adds a state change listener.
func (b *WindowedBreaker) AddListener(listener StateChangeListener) {
	b.listenerMu.Lock()
	defer b.listenerMu.Unlock()
	b.listeners = append(b.listeners, listener)
}

// Reset resets the breaker.
func (b *WindowedBreaker) Reset() {
	oldState := b.State()
	atomic.StoreInt32(&b.state, int32(StateClosed))
	atomic.StoreInt64(&b.halfOpenRequests, 0)

	b.windowMu.Lock()
	b.windows = make([]windowEntry, 0)
	b.windowMu.Unlock()

	if oldState != StateClosed {
		b.emit(&StateChangeEvent{
			Name:      b.config.Name,
			From:      oldState,
			To:        StateClosed,
			Timestamp: time.Now(),
		})
	}
}

func (b *WindowedBreaker) cleanWindow() {
	cutoff := time.Now().Add(-b.config.WindowSize)

	// Remove old entries
	newWindows := make([]windowEntry, 0, len(b.windows))
	for _, entry := range b.windows {
		if entry.timestamp.After(cutoff) {
			newWindows = append(newWindows, entry)
		}
	}
	b.windows = newWindows
}

func (b *WindowedBreaker) shouldTrip() bool {
	if len(b.windows) < int(b.config.MinRequests) {
		return false
	}

	failures := 0
	for _, entry := range b.windows {
		if !entry.success {
			failures++
		}
	}

	// Check absolute failure count
	if int64(failures) >= b.config.MaxFailures {
		return true
	}

	// Check failure rate
	if b.config.FailureRateThreshold > 0 {
		rate := float64(failures) / float64(len(b.windows))
		if rate >= b.config.FailureRateThreshold {
			return true
		}
	}

	return false
}

func (b *WindowedBreaker) countSuccesses() int64 {
	var count int64
	for _, entry := range b.windows {
		if entry.success {
			count++
		}
	}
	return count
}

func (b *WindowedBreaker) transitionTo(newState State) {
	oldState := State(atomic.SwapInt32(&b.state, int32(newState)))
	if oldState == newState {
		return
	}

	b.windowMu.Lock()
	switch newState {
	case StateClosed:
		b.windows = make([]windowEntry, 0)
		atomic.StoreInt64(&b.halfOpenRequests, 0)

	case StateOpen:
		b.openedAt = time.Now()

	case StateHalfOpen:
		b.windows = make([]windowEntry, 0)
		atomic.StoreInt64(&b.halfOpenRequests, 0)
	}
	b.windowMu.Unlock()

	b.emit(&StateChangeEvent{
		Name:      b.config.Name,
		From:      oldState,
		To:        newState,
		Timestamp: time.Now(),
	})
}

func (b *WindowedBreaker) emit(event *StateChangeEvent) {
	b.listenerMu.RLock()
	listeners := b.listeners
	b.listenerMu.RUnlock()

	for _, listener := range listeners {
		listener(event)
	}
}

// TwoStepBreaker provides separate allow and record steps.
type TwoStepBreaker interface {
	Allow() error
	RecordSuccess()
	RecordFailure()
	State() State
}

// Ensure types implement interface
var (
	_ TwoStepBreaker = (*Breaker)(nil)
	_ TwoStepBreaker = (*WindowedBreaker)(nil)
)

// NATSBreaker provides circuit breaker for NATS operations.
type NATSBreaker struct {
	breakers map[string]*Breaker
	mu       sync.RWMutex
	config   *Config
}

// NewNATSBreaker creates a new NATS circuit breaker.
func NewNATSBreaker(config *Config) *NATSBreaker {
	if config == nil {
		config = DefaultConfig()
	}
	return &NATSBreaker{
		breakers: make(map[string]*Breaker),
		config:   config,
	}
}

// GetSubjectBreaker returns a breaker for a NATS subject.
func (n *NATSBreaker) GetSubjectBreaker(subject string) *Breaker {
	n.mu.RLock()
	breaker, ok := n.breakers[subject]
	n.mu.RUnlock()

	if ok {
		return breaker
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	if breaker, ok := n.breakers[subject]; ok {
		return breaker
	}

	config := *n.config
	config.Name = fmt.Sprintf("nats:%s", subject)
	breaker = NewBreaker(&config)
	n.breakers[subject] = breaker

	return breaker
}

// AllowPublish checks if publishing to a subject is allowed.
func (n *NATSBreaker) AllowPublish(subject string) error {
	return n.GetSubjectBreaker(subject).Allow()
}

// RecordPublishSuccess records a successful publish.
func (n *NATSBreaker) RecordPublishSuccess(subject string) {
	n.GetSubjectBreaker(subject).RecordSuccess()
}

// RecordPublishFailure records a failed publish.
func (n *NATSBreaker) RecordPublishFailure(subject string) {
	n.GetSubjectBreaker(subject).RecordFailure()
}

// Stats returns all breaker statistics.
func (n *NATSBreaker) Stats() map[string]BreakerStats {
	n.mu.RLock()
	defer n.mu.RUnlock()

	stats := make(map[string]BreakerStats)
	for subject, b := range n.breakers {
		stats[subject] = BreakerStats{
			Name:   subject,
			State:  b.State().String(),
			Counts: b.Counts(),
		}
	}
	return stats
}
