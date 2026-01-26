// Package storage provides storage backend failover with queuing for file distribution.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/wait"
)

// Backend represents a storage backend
type Backend interface {
	// Name returns the backend identifier
	Name() string

	// Put stores data with the given key
	Put(ctx context.Context, key string, data io.Reader, size int64) error

	// Get retrieves data for the given key
	Get(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete removes the data for the given key
	Delete(ctx context.Context, key string) error

	// Exists checks if the key exists
	Exists(ctx context.Context, key string) (bool, error)

	// List lists keys with the given prefix
	List(ctx context.Context, prefix string) ([]string, error)

	// Ping checks if the backend is available
	Ping(ctx context.Context) error
}

// BackendStatus represents the health status of a backend
type BackendStatus string

const (
	// StatusHealthy indicates the backend is healthy
	StatusHealthy BackendStatus = "healthy"
	// StatusDegraded indicates the backend has issues but is usable
	StatusDegraded BackendStatus = "degraded"
	// StatusUnhealthy indicates the backend is not usable
	StatusUnhealthy BackendStatus = "unhealthy"
)

// BackendHealth tracks the health of a backend
type BackendHealth struct {
	Backend             Backend
	Status              BackendStatus
	LastCheck           time.Time
	LastSuccess         time.Time
	LastError           error
	ConsecutiveFailures int
	FailureCount        int64
	SuccessCount        int64
	Latency             time.Duration
}

// Config configures the failover manager
type Config struct {
	// HealthCheckInterval is the interval between health checks
	HealthCheckInterval time.Duration

	// HealthCheckTimeout is the timeout for health checks
	HealthCheckTimeout time.Duration

	// MaxConsecutiveFailures is the number of failures before marking unhealthy
	MaxConsecutiveFailures int

	// RecoveryThreshold is the number of successes needed to recover
	RecoveryThreshold int

	// QueueSize is the maximum size of the operation queue
	QueueSize int

	// QueueTimeout is the timeout for queued operations
	QueueTimeout time.Duration

	// RetryAttempts is the number of retry attempts for failed operations
	RetryAttempts int

	// RetryDelay is the delay between retry attempts
	RetryDelay time.Duration

	// EnableQueue enables operation queuing during failover
	EnableQueue bool
}

// DefaultConfig returns a default failover configuration
func DefaultConfig() *Config {
	return &Config{
		HealthCheckInterval:    30 * time.Second,
		HealthCheckTimeout:     5 * time.Second,
		MaxConsecutiveFailures: 3,
		RecoveryThreshold:      2,
		QueueSize:              1000,
		QueueTimeout:           5 * time.Minute,
		RetryAttempts:          3,
		RetryDelay:             1 * time.Second,
		EnableQueue:            true,
	}
}

// OperationType represents the type of storage operation
type OperationType string

const (
	// OpPut is a put operation
	OpPut OperationType = "put"
	// OpGet is a get operation
	OpGet OperationType = "get"
	// OpDelete is a delete operation
	OpDelete OperationType = "delete"
)

// QueuedOperation represents an operation waiting in the queue
type QueuedOperation struct {
	Type      OperationType
	Key       string
	Data      []byte
	Size      int64
	QueuedAt  time.Time
	Attempts  int
	LastError error
	Result    chan error
}

// Manager manages failover between storage backends
type Manager struct {
	config   *Config
	backends []*BackendHealth
	primary  int
	mu       sync.RWMutex
	queue    chan *QueuedOperation
	stats    *Stats
	stopCh   chan struct{}
	wg       sync.WaitGroup
	running  atomic.Bool
}

// NewManager creates a new failover manager
func NewManager(config *Config) *Manager {
	if config == nil {
		config = DefaultConfig()
	}

	m := &Manager{
		config:   config,
		backends: make([]*BackendHealth, 0),
		queue:    make(chan *QueuedOperation, config.QueueSize),
		stats:    NewStats(),
		stopCh:   make(chan struct{}),
	}

	return m
}

// AddBackend adds a storage backend to the manager
func (m *Manager) AddBackend(backend Backend) {
	m.mu.Lock()
	defer m.mu.Unlock()

	health := &BackendHealth{
		Backend:   backend,
		Status:    StatusHealthy,
		LastCheck: time.Now(),
	}

	m.backends = append(m.backends, health)
}

// Start starts the failover manager
func (m *Manager) Start(ctx context.Context) error {
	if !m.running.CompareAndSwap(false, true) {
		return errors.New("manager already running")
	}

	// Start health checker
	m.wg.Add(1)
	go m.healthCheckLoop(ctx)

	// Start queue processor
	if m.config.EnableQueue {
		m.wg.Add(1)
		go m.processQueue(ctx)
	}

	return nil
}

// Stop stops the failover manager
func (m *Manager) Stop() {
	if !m.running.CompareAndSwap(true, false) {
		return
	}

	close(m.stopCh)
	m.wg.Wait()
}

func (m *Manager) healthCheckLoop(ctx context.Context) {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.HealthCheckInterval)
	defer ticker.Stop()

	// Initial health check
	m.checkAllBackends(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.checkAllBackends(ctx)
		}
	}
}

func (m *Manager) checkAllBackends(ctx context.Context) {
	m.mu.RLock()
	backends := make([]*BackendHealth, len(m.backends))
	copy(backends, m.backends)
	m.mu.RUnlock()

	for _, health := range backends {
		m.checkBackend(ctx, health)
	}

	// Update primary if needed
	m.selectPrimary()
}

func (m *Manager) checkBackend(ctx context.Context, health *BackendHealth) {
	checkCtx, cancel := context.WithTimeout(ctx, m.config.HealthCheckTimeout)
	defer cancel()

	start := time.Now()
	err := health.Backend.Ping(checkCtx)
	latency := time.Since(start)

	m.mu.Lock()
	defer m.mu.Unlock()

	health.LastCheck = time.Now()
	health.Latency = latency

	if err != nil {
		health.LastError = err
		health.ConsecutiveFailures++
		health.FailureCount++

		if health.ConsecutiveFailures >= m.config.MaxConsecutiveFailures {
			health.Status = StatusUnhealthy
		} else if health.ConsecutiveFailures > 0 {
			health.Status = StatusDegraded
		}

		m.stats.RecordHealthCheck(health.Backend.Name(), false)
	} else {
		health.LastSuccess = time.Now()
		health.LastError = nil
		health.SuccessCount++

		// Check recovery threshold
		if health.Status == StatusUnhealthy {
			health.ConsecutiveFailures = 0
			health.Status = StatusDegraded
		} else if health.Status == StatusDegraded {
			health.ConsecutiveFailures = 0
			health.Status = StatusHealthy
		} else {
			health.ConsecutiveFailures = 0
		}

		m.stats.RecordHealthCheck(health.Backend.Name(), true)
	}
}

func (m *Manager) selectPrimary() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Find first healthy backend
	for i, health := range m.backends {
		if health.Status == StatusHealthy {
			if m.primary != i {
				m.stats.RecordFailover(m.backends[m.primary].Backend.Name(), health.Backend.Name())
				m.primary = i
			}
			return
		}
	}

	// Fall back to first degraded backend
	for i, health := range m.backends {
		if health.Status == StatusDegraded {
			if m.primary != i {
				m.stats.RecordFailover(m.backends[m.primary].Backend.Name(), health.Backend.Name())
				m.primary = i
			}
			return
		}
	}

	// No healthy backends - keep current primary
}

// getPrimaryBackend returns the current primary backend
func (m *Manager) getPrimaryBackend() (*BackendHealth, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.backends) == 0 {
		return nil, errors.New("no backends configured")
	}

	if m.primary >= len(m.backends) {
		m.primary = 0
	}

	return m.backends[m.primary], nil
}

// getHealthyBackends returns all healthy backends
func (m *Manager) getHealthyBackends() []*BackendHealth {
	m.mu.RLock()
	defer m.mu.RUnlock()

	healthy := make([]*BackendHealth, 0, len(m.backends))
	for _, h := range m.backends {
		if h.Status == StatusHealthy || h.Status == StatusDegraded {
			healthy = append(healthy, h)
		}
	}
	return healthy
}

// Put stores data with automatic failover
func (m *Manager) Put(ctx context.Context, key string, data io.Reader, size int64) error {
	// Read all data if we need to retry
	var dataBytes []byte
	var err error

	if m.config.RetryAttempts > 1 {
		dataBytes, err = io.ReadAll(data)
		if err != nil {
			return fmt.Errorf("failed to read data: %w", err)
		}
	}

	backends := m.getHealthyBackends()
	if len(backends) == 0 {
		if m.config.EnableQueue {
			return m.queueOperation(&QueuedOperation{
				Type:     OpPut,
				Key:      key,
				Data:     dataBytes,
				Size:     size,
				QueuedAt: time.Now(),
			})
		}
		return errors.New("no healthy backends available")
	}

	for attempt := 0; attempt < m.config.RetryAttempts; attempt++ {
		for _, health := range backends {
			var reader io.Reader
			if dataBytes != nil {
				reader = io.NopCloser(io.NewSectionReader(
					&bytesReaderAt{data: dataBytes},
					0,
					int64(len(dataBytes)),
				))
			} else {
				reader = data
			}

			err = health.Backend.Put(ctx, key, reader, size)
			if err == nil {
				m.stats.RecordOperation(OpPut, health.Backend.Name(), true)
				return nil
			}

			m.stats.RecordOperation(OpPut, health.Backend.Name(), false)
		}

		if attempt < m.config.RetryAttempts-1 {
			if err := waitForRetry(ctx, m.config.RetryDelay); err != nil {
				return err
			}
		}
	}

	// Queue if all attempts failed
	if m.config.EnableQueue && dataBytes != nil {
		return m.queueOperation(&QueuedOperation{
			Type:      OpPut,
			Key:       key,
			Data:      dataBytes,
			Size:      size,
			QueuedAt:  time.Now(),
			LastError: err,
		})
	}

	return fmt.Errorf("all backends failed: %w", err)
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	return wait.ForContext(ctx, delay)
}

// Get retrieves data with automatic failover
func (m *Manager) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	backends := m.getHealthyBackends()
	if len(backends) == 0 {
		return nil, errors.New("no healthy backends available")
	}

	var lastErr error
	for _, health := range backends {
		reader, err := health.Backend.Get(ctx, key)
		if err == nil {
			m.stats.RecordOperation(OpGet, health.Backend.Name(), true)
			return reader, nil
		}
		lastErr = err
		m.stats.RecordOperation(OpGet, health.Backend.Name(), false)
	}

	return nil, fmt.Errorf("all backends failed: %w", lastErr)
}

// Delete removes data with automatic failover
func (m *Manager) Delete(ctx context.Context, key string) error {
	backends := m.getHealthyBackends()
	if len(backends) == 0 {
		if m.config.EnableQueue {
			return m.queueOperation(&QueuedOperation{
				Type:     OpDelete,
				Key:      key,
				QueuedAt: time.Now(),
			})
		}
		return errors.New("no healthy backends available")
	}

	var lastErr error
	for _, health := range backends {
		err := health.Backend.Delete(ctx, key)
		if err == nil {
			m.stats.RecordOperation(OpDelete, health.Backend.Name(), true)
			return nil
		}
		lastErr = err
		m.stats.RecordOperation(OpDelete, health.Backend.Name(), false)
	}

	// Queue if all attempts failed
	if m.config.EnableQueue {
		return m.queueOperation(&QueuedOperation{
			Type:      OpDelete,
			Key:       key,
			QueuedAt:  time.Now(),
			LastError: lastErr,
		})
	}

	return fmt.Errorf("all backends failed: %w", lastErr)
}

// Exists checks if a key exists
func (m *Manager) Exists(ctx context.Context, key string) (bool, error) {
	backends := m.getHealthyBackends()
	if len(backends) == 0 {
		return false, errors.New("no healthy backends available")
	}

	var lastErr error
	for _, health := range backends {
		exists, err := health.Backend.Exists(ctx, key)
		if err == nil {
			return exists, nil
		}
		lastErr = err
	}

	return false, fmt.Errorf("all backends failed: %w", lastErr)
}

// List lists keys with the given prefix
func (m *Manager) List(ctx context.Context, prefix string) ([]string, error) {
	backends := m.getHealthyBackends()
	if len(backends) == 0 {
		return nil, errors.New("no healthy backends available")
	}

	var lastErr error
	for _, health := range backends {
		keys, err := health.Backend.List(ctx, prefix)
		if err == nil {
			return keys, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("all backends failed: %w", lastErr)
}

func (m *Manager) queueOperation(op *QueuedOperation) error {
	op.Result = make(chan error, 1)

	select {
	case m.queue <- op:
		m.stats.RecordQueued()
		return nil // Queued successfully
	default:
		return errors.New("operation queue is full")
	}
}

func (m *Manager) processQueue(ctx context.Context) {
	defer m.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			// Drain remaining queue items
			m.drainQueue(ctx)
			return
		case op := <-m.queue:
			m.processQueuedOperation(ctx, op)
		}
	}
}

func (m *Manager) drainQueue(ctx context.Context) {
	for {
		select {
		case op := <-m.queue:
			m.processQueuedOperation(ctx, op)
		default:
			return
		}
	}
}

func (m *Manager) processQueuedOperation(ctx context.Context, op *QueuedOperation) {
	// Check if operation has expired
	if time.Since(op.QueuedAt) > m.config.QueueTimeout {
		m.stats.RecordQueueExpired()
		if op.Result != nil {
			op.Result <- errors.New("operation expired in queue")
		}
		return
	}

	backends := m.getHealthyBackends()
	if len(backends) == 0 {
		// Re-queue if still no backends
		op.Attempts++
		if op.Attempts < m.config.RetryAttempts {
			select {
			case m.queue <- op:
				return
			default:
				// Queue full, drop operation
			}
		}
		if op.Result != nil {
			op.Result <- errors.New("no healthy backends available")
		}
		return
	}

	var err error
	switch op.Type {
	case OpPut:
		if op.Data != nil {
			reader := io.NopCloser(io.NewSectionReader(
				&bytesReaderAt{data: op.Data},
				0,
				int64(len(op.Data)),
			))
			err = backends[0].Backend.Put(ctx, op.Key, reader, op.Size)
		}
	case OpDelete:
		err = backends[0].Backend.Delete(ctx, op.Key)
	}

	if err != nil {
		op.Attempts++
		op.LastError = err
		if op.Attempts < m.config.RetryAttempts {
			select {
			case m.queue <- op:
				return
			default:
				// Queue full
			}
		}
	} else {
		m.stats.RecordQueueProcessed()
	}

	if op.Result != nil {
		op.Result <- err
	}
}

// GetBackendHealth returns the health of all backends
func (m *Manager) GetBackendHealth() []*BackendHealth {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*BackendHealth, len(m.backends))
	for i, h := range m.backends {
		result[i] = &BackendHealth{
			Backend:             h.Backend,
			Status:              h.Status,
			LastCheck:           h.LastCheck,
			LastSuccess:         h.LastSuccess,
			LastError:           h.LastError,
			ConsecutiveFailures: h.ConsecutiveFailures,
			FailureCount:        h.FailureCount,
			SuccessCount:        h.SuccessCount,
			Latency:             h.Latency,
		}
	}
	return result
}

// GetPrimaryBackendName returns the name of the current primary backend
func (m *Manager) GetPrimaryBackendName() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.backends) == 0 {
		return ""
	}
	return m.backends[m.primary].Backend.Name()
}

// GetQueueSize returns the current queue size
func (m *Manager) GetQueueSize() int {
	return len(m.queue)
}

// Stats returns the current statistics
func (m *Manager) Stats() *Stats {
	return m.stats
}

// bytesReaderAt implements io.ReaderAt for a byte slice
type bytesReaderAt struct {
	data []byte
}

func (b *bytesReaderAt) ReadAt(p []byte, off int64) (n int, err error) {
	if off >= int64(len(b.data)) {
		return 0, io.EOF
	}
	n = copy(p, b.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// Stats tracks failover statistics
type Stats struct {
	mu               sync.Mutex
	TotalOperations  int64
	SuccessfulOps    int64
	FailedOps        int64
	FailoverCount    int64
	QueuedOps        int64
	QueueProcessed   int64
	QueueExpired     int64
	HealthChecks     int64
	HealthCheckFails int64
	ByBackend        map[string]*BackendStats
	ByOperation      map[OperationType]*OperationStats
}

// BackendStats tracks stats for a specific backend
type BackendStats struct {
	Operations   int64
	Successes    int64
	Failures     int64
	HealthChecks int64
	HealthFails  int64
}

// OperationStats tracks stats for an operation type
type OperationStats struct {
	Total     int64
	Successes int64
	Failures  int64
}

// NewStats creates a new stats tracker
func NewStats() *Stats {
	return &Stats{
		ByBackend:   make(map[string]*BackendStats),
		ByOperation: make(map[OperationType]*OperationStats),
	}
}

// RecordOperation records a storage operation
func (s *Stats) RecordOperation(opType OperationType, backend string, success bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.TotalOperations++
	if success {
		s.SuccessfulOps++
	} else {
		s.FailedOps++
	}

	// Backend stats
	if _, ok := s.ByBackend[backend]; !ok {
		s.ByBackend[backend] = &BackendStats{}
	}
	s.ByBackend[backend].Operations++
	if success {
		s.ByBackend[backend].Successes++
	} else {
		s.ByBackend[backend].Failures++
	}

	// Operation stats
	if _, ok := s.ByOperation[opType]; !ok {
		s.ByOperation[opType] = &OperationStats{}
	}
	s.ByOperation[opType].Total++
	if success {
		s.ByOperation[opType].Successes++
	} else {
		s.ByOperation[opType].Failures++
	}
}

// RecordHealthCheck records a health check result
func (s *Stats) RecordHealthCheck(backend string, success bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.HealthChecks++
	if !success {
		s.HealthCheckFails++
	}

	if _, ok := s.ByBackend[backend]; !ok {
		s.ByBackend[backend] = &BackendStats{}
	}
	s.ByBackend[backend].HealthChecks++
	if !success {
		s.ByBackend[backend].HealthFails++
	}
}

// RecordFailover records a failover event
func (s *Stats) RecordFailover(from, to string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.FailoverCount++
}

// RecordQueued records an operation being queued
func (s *Stats) RecordQueued() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.QueuedOps++
}

// RecordQueueProcessed records a queued operation being processed
func (s *Stats) RecordQueueProcessed() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.QueueProcessed++
}

// RecordQueueExpired records a queued operation expiring
func (s *Stats) RecordQueueExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.QueueExpired++
}

// Snapshot returns a copy of current stats
func (s *Stats) Snapshot() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()

	snapshot := Stats{
		TotalOperations:  s.TotalOperations,
		SuccessfulOps:    s.SuccessfulOps,
		FailedOps:        s.FailedOps,
		FailoverCount:    s.FailoverCount,
		QueuedOps:        s.QueuedOps,
		QueueProcessed:   s.QueueProcessed,
		QueueExpired:     s.QueueExpired,
		HealthChecks:     s.HealthChecks,
		HealthCheckFails: s.HealthCheckFails,
		ByBackend:        make(map[string]*BackendStats),
		ByOperation:      make(map[OperationType]*OperationStats),
	}

	for name, stats := range s.ByBackend {
		snapshot.ByBackend[name] = &BackendStats{
			Operations:   stats.Operations,
			Successes:    stats.Successes,
			Failures:     stats.Failures,
			HealthChecks: stats.HealthChecks,
			HealthFails:  stats.HealthFails,
		}
	}

	for op, stats := range s.ByOperation {
		snapshot.ByOperation[op] = &OperationStats{
			Total:     stats.Total,
			Successes: stats.Successes,
			Failures:  stats.Failures,
		}
	}

	return snapshot
}
