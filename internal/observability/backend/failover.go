// Package backend provides failover and queuing for observability backends.
package backend

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// Errors returned by the failover system.
var (
	ErrNoHealthyBackend = errors.New("no healthy backend available")
	ErrQueueFull        = errors.New("queue is full")
	ErrBackendTimeout   = errors.New("backend timeout")
	ErrBackendClosed    = errors.New("backend closed")
)

// Backend represents an observability data backend.
type Backend interface {
	// Name returns the backend identifier.
	Name() string
	// Write sends data to the backend.
	Write(ctx context.Context, data []byte) error
	// Health returns the current health status.
	Health() Health
	// Close closes the backend connection.
	Close() error
}

// Health represents backend health status.
type Health struct {
	Healthy   bool
	LastCheck time.Time
	LastError error
	Latency   time.Duration
}

// Config holds failover configuration.
type Config struct {
	// QueueSize is the maximum number of items in the queue.
	QueueSize int
	// MaxRetries is the maximum number of retry attempts.
	MaxRetries int
	// RetryInterval is the base interval between retries.
	RetryInterval time.Duration
	// WriteTimeout is the timeout for write operations.
	WriteTimeout time.Duration
	// HealthCheckInterval is how often to check backend health.
	HealthCheckInterval time.Duration
	// FailoverThreshold is failures before failover.
	FailoverThreshold int
	// RecoveryThreshold is successes before recovery.
	RecoveryThreshold int
	// Strategy defines the failover strategy.
	Strategy FailoverStrategy
}

// FailoverStrategy defines how to select backends.
type FailoverStrategy string

const (
	// StrategyPrimary always tries primary first, then failover.
	StrategyPrimary FailoverStrategy = "primary"
	// StrategyRoundRobin distributes across healthy backends.
	StrategyRoundRobin FailoverStrategy = "round_robin"
	// StrategyLeastLatency picks the backend with lowest latency.
	StrategyLeastLatency FailoverStrategy = "least_latency"
)

// DefaultConfig returns a default configuration.
func DefaultConfig() *Config {
	return &Config{
		QueueSize:           10000,
		MaxRetries:          3,
		RetryInterval:       time.Second,
		WriteTimeout:        10 * time.Second,
		HealthCheckInterval: 30 * time.Second,
		FailoverThreshold:   3,
		RecoveryThreshold:   2,
		Strategy:            StrategyPrimary,
	}
}

// QueueItem represents an item in the write queue.
type QueueItem struct {
	Data      []byte
	Timestamp time.Time
	Retries   int
}

// State tracks the state of a backend.
type State struct {
	Backend         Backend
	Healthy         bool
	ConsecFailures  int
	ConsecSuccesses int
	TotalWrites     int64
	TotalErrors     int64
	LastError       error
	LastWrite       time.Time
	AverageLatency  time.Duration
	latencySum      time.Duration
	latencyCount    int64
	mu              sync.RWMutex
}

// FailoverEvent represents a failover-related event.
type FailoverEvent struct {
	Type      string
	Backend   string
	Timestamp time.Time
	Message   string
	Error     error
}

// Listener receives failover events.
type Listener func(*FailoverEvent)

// Manager manages backend failover and queuing.
type Manager struct {
	config    *Config
	backends  []*State
	queue     chan *QueueItem
	listeners []Listener
	primary   int
	current   int64
	closed    int32
	stopCh    chan struct{}
	wg        sync.WaitGroup
	mu        sync.RWMutex
}

// NewManager creates a new failover manager.
func NewManager(config *Config, backends ...Backend) *Manager {
	if config == nil {
		config = DefaultConfig()
	}

	states := make([]*State, len(backends))
	for i, b := range backends {
		states[i] = &State{
			Backend: b,
			Healthy: true,
		}
	}

	m := &Manager{
		config:   config,
		backends: states,
		queue:    make(chan *QueueItem, config.QueueSize),
		stopCh:   make(chan struct{}),
	}

	// Start background workers
	m.wg.Add(2)
	go m.processQueue()
	go m.healthCheckLoop()

	return m
}

// Write queues data to be written to backends.
func (m *Manager) Write(ctx context.Context, data []byte) error {
	if atomic.LoadInt32(&m.closed) == 1 {
		return ErrBackendClosed
	}

	item := &QueueItem{
		Data:      data,
		Timestamp: time.Now(),
	}

	select {
	case m.queue <- item:
		return nil
	default:
		return ErrQueueFull
	}
}

// WriteSync writes data synchronously to backends.
func (m *Manager) WriteSync(ctx context.Context, data []byte) error {
	if atomic.LoadInt32(&m.closed) == 1 {
		return ErrBackendClosed
	}

	backend := m.selectBackend()
	if backend == nil {
		return ErrNoHealthyBackend
	}

	return m.writeToBackend(ctx, backend, data)
}

func (m *Manager) selectBackend() *State {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.backends) == 0 {
		return nil
	}

	switch m.config.Strategy {
	case StrategyPrimary:
		return m.selectPrimary()
	case StrategyRoundRobin:
		return m.selectRoundRobin()
	case StrategyLeastLatency:
		return m.selectLeastLatency()
	default:
		return m.selectPrimary()
	}
}

func (m *Manager) selectPrimary() *State {
	// Try primary first
	if m.primary < len(m.backends) {
		state := m.backends[m.primary]
		state.mu.RLock()
		healthy := state.Healthy
		state.mu.RUnlock()
		if healthy {
			return state
		}
	}

	// Fall back to first healthy backend
	for _, state := range m.backends {
		state.mu.RLock()
		healthy := state.Healthy
		state.mu.RUnlock()
		if healthy {
			return state
		}
	}

	return nil
}

func (m *Manager) selectRoundRobin() *State {
	n := len(m.backends)
	for i := 0; i < n; i++ {
		idx := int(atomic.AddInt64(&m.current, 1)-1) % n
		state := m.backends[idx]
		state.mu.RLock()
		healthy := state.Healthy
		state.mu.RUnlock()
		if healthy {
			return state
		}
	}
	return nil
}

func (m *Manager) selectLeastLatency() *State {
	var best *State
	var bestLatency time.Duration

	for _, state := range m.backends {
		state.mu.RLock()
		healthy := state.Healthy
		latency := state.AverageLatency
		state.mu.RUnlock()

		if !healthy {
			continue
		}

		if best == nil || latency < bestLatency {
			best = state
			bestLatency = latency
		}
	}

	return best
}

func (m *Manager) writeToBackend(ctx context.Context, state *State, data []byte) error {
	// Apply timeout
	writeCtx, cancel := context.WithTimeout(ctx, m.config.WriteTimeout)
	defer cancel()

	start := time.Now()
	err := state.Backend.Write(writeCtx, data)
	latency := time.Since(start)

	state.mu.Lock()
	defer state.mu.Unlock()

	state.LastWrite = time.Now()
	state.latencySum += latency
	state.latencyCount++
	state.AverageLatency = state.latencySum / time.Duration(state.latencyCount)

	if err != nil {
		state.TotalErrors++
		state.LastError = err
		state.ConsecFailures++
		state.ConsecSuccesses = 0

		if state.ConsecFailures >= m.config.FailoverThreshold && state.Healthy {
			state.Healthy = false
			m.emitEvent(&FailoverEvent{
				Type:      "backend_unhealthy",
				Backend:   state.Backend.Name(),
				Timestamp: time.Now(),
				Message:   "backend marked unhealthy after consecutive failures",
				Error:     err,
			})
		}

		return err
	}

	state.TotalWrites++
	state.ConsecSuccesses++
	state.ConsecFailures = 0

	if !state.Healthy && state.ConsecSuccesses >= m.config.RecoveryThreshold {
		state.Healthy = true
		m.emitEvent(&FailoverEvent{
			Type:      "backend_recovered",
			Backend:   state.Backend.Name(),
			Timestamp: time.Now(),
			Message:   "backend recovered after consecutive successes",
		})
	}

	return nil
}

func (m *Manager) processQueue() {
	defer m.wg.Done()

	for {
		select {
		case item := <-m.queue:
			m.processItem(item)
		case <-m.stopCh:
			// Drain remaining items
			for {
				select {
				case item := <-m.queue:
					m.processItem(item)
				default:
					return
				}
			}
		}
	}
}

func (m *Manager) processItem(item *QueueItem) {
	ctx := context.Background()
	var lastErr error

	for item.Retries <= m.config.MaxRetries {
		backend := m.selectBackend()
		if backend == nil {
			// No healthy backend, requeue with delay
			if !waitForRetryOrStop(m.stopCh, m.config.RetryInterval) {
				return
			}
			item.Retries++
			continue
		}

		err := m.writeToBackend(ctx, backend, item.Data)
		if err == nil {
			return
		}

		lastErr = err
		item.Retries++

		// Exponential backoff
		//nolint:gosec // G115: item.Retries is bounded by MaxRetries config, fits in uint
		backoff := m.config.RetryInterval * time.Duration(1<<uint(item.Retries-1))
		if backoff > time.Minute {
			backoff = time.Minute
		}
		if !waitForRetryOrStop(m.stopCh, backoff) {
			return
		}
	}

	m.emitEvent(&FailoverEvent{
		Type:      "item_dropped",
		Timestamp: time.Now(),
		Message:   "item dropped after max retries",
		Error:     lastErr,
	})
}

func waitForRetryOrStop(stopCh <-chan struct{}, delay time.Duration) bool {
	if delay <= 0 {
		return true
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-stopCh:
		return false
	case <-timer.C:
		return true
	}
}

func (m *Manager) healthCheckLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.checkHealth()
		case <-m.stopCh:
			return
		}
	}
}

func (m *Manager) checkHealth() {
	for _, state := range m.backends {
		health := state.Backend.Health()

		state.mu.Lock()
		wasHealthy := state.Healthy
		state.Healthy = health.Healthy

		if wasHealthy && !health.Healthy {
			m.emitEvent(&FailoverEvent{
				Type:      "health_check_failed",
				Backend:   state.Backend.Name(),
				Timestamp: time.Now(),
				Message:   "health check reported unhealthy",
				Error:     health.LastError,
			})
		} else if !wasHealthy && health.Healthy {
			m.emitEvent(&FailoverEvent{
				Type:      "health_check_recovered",
				Backend:   state.Backend.Name(),
				Timestamp: time.Now(),
				Message:   "health check reported healthy",
			})
		}
		state.mu.Unlock()
	}
}

// SetPrimary sets the primary backend index.
func (m *Manager) SetPrimary(index int) {
	m.mu.Lock()
	m.primary = index
	m.mu.Unlock()
}

// Stats returns current statistics.
func (m *Manager) Stats() *ManagerStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := &ManagerStats{
		QueueSize:  len(m.queue),
		QueueCap:   cap(m.queue),
		Backends:   make([]*Stats, len(m.backends)),
		Strategy:   m.config.Strategy,
		PrimaryIdx: m.primary,
	}

	for i, state := range m.backends {
		state.mu.RLock()
		stats.Backends[i] = &Stats{
			Name:           state.Backend.Name(),
			Healthy:        state.Healthy,
			TotalWrites:    state.TotalWrites,
			TotalErrors:    state.TotalErrors,
			AverageLatency: state.AverageLatency,
			LastError:      state.LastError,
			LastWrite:      state.LastWrite,
		}
		state.mu.RUnlock()
	}

	return stats
}

// ManagerStats holds manager statistics.
type ManagerStats struct {
	QueueSize  int
	QueueCap   int
	Backends   []*Stats
	Strategy   FailoverStrategy
	PrimaryIdx int
}

// Stats holds backend statistics.
type Stats struct {
	Name           string
	Healthy        bool
	TotalWrites    int64
	TotalErrors    int64
	AverageLatency time.Duration
	LastError      error
	LastWrite      time.Time
}

// AddListener adds an event listener.
func (m *Manager) AddListener(listener Listener) {
	m.mu.Lock()
	m.listeners = append(m.listeners, listener)
	m.mu.Unlock()
}

func (m *Manager) emitEvent(event *FailoverEvent) {
	m.mu.RLock()
	listeners := m.listeners
	m.mu.RUnlock()

	for _, listener := range listeners {
		listener(event)
	}
}

// Close stops the manager and waits for pending operations.
func (m *Manager) Close() error {
	if !atomic.CompareAndSwapInt32(&m.closed, 0, 1) {
		return ErrBackendClosed
	}

	close(m.stopCh)
	m.wg.Wait()

	// Close all backends
	for _, state := range m.backends {
		state.Backend.Close()
	}

	return nil
}

// MemoryBackend is an in-memory backend for testing.
type MemoryBackend struct {
	name       string
	data       [][]byte
	healthy    bool
	latency    time.Duration
	failAfter  int
	writeCount int
	mu         sync.Mutex
}

// NewMemoryBackend creates a new memory backend.
func NewMemoryBackend(name string) *MemoryBackend {
	return &MemoryBackend{
		name:    name,
		healthy: true,
	}
}

// Name returns the backend name.
func (b *MemoryBackend) Name() string {
	return b.name
}

// Write stores data in memory.
func (b *MemoryBackend) Write(ctx context.Context, data []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.latency > 0 {
		timer := time.NewTimer(b.latency)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}

	b.writeCount++
	if b.failAfter > 0 && b.writeCount > b.failAfter {
		return errors.New("simulated failure")
	}

	if !b.healthy {
		return errors.New("backend unhealthy")
	}

	b.data = append(b.data, data)
	return nil
}

// Health returns the health status.
func (b *MemoryBackend) Health() Health {
	b.mu.Lock()
	defer b.mu.Unlock()

	return Health{
		Healthy:   b.healthy,
		LastCheck: time.Now(),
		Latency:   b.latency,
	}
}

// Close closes the backend.
func (b *MemoryBackend) Close() error {
	return nil
}

// SetHealthy sets the health status.
func (b *MemoryBackend) SetHealthy(healthy bool) {
	b.mu.Lock()
	b.healthy = healthy
	b.mu.Unlock()
}

// SetLatency sets simulated latency.
func (b *MemoryBackend) SetLatency(d time.Duration) {
	b.mu.Lock()
	b.latency = d
	b.mu.Unlock()
}

// SetFailAfter makes the backend fail after n writes.
func (b *MemoryBackend) SetFailAfter(n int) {
	b.mu.Lock()
	b.failAfter = n
	b.mu.Unlock()
}

// Data returns all stored data.
func (b *MemoryBackend) Data() [][]byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	result := make([][]byte, len(b.data))
	copy(result, b.data)
	return result
}

// WriteCount returns the number of write attempts.
func (b *MemoryBackend) WriteCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.writeCount
}

// Reset clears the backend.
func (b *MemoryBackend) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = nil
	b.writeCount = 0
	b.failAfter = 0
	b.healthy = true
}
