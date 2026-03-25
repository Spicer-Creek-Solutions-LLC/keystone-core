package secrets

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ConnectionPool manages a pool of connections to a secret backend.
type ConnectionPool struct {
	config  *PoolConfig
	factory ConnectionFactory

	mu             sync.Mutex
	connections    []*pooledConnection
	available      chan *pooledConnection
	created        int32
	closed         bool
	cancelMaintain context.CancelFunc

	stats PoolStats
}

// ConnectionFactory creates new connections for the pool.
type ConnectionFactory interface {
	// Create creates a new connection.
	Create(ctx context.Context) (Connection, error)

	// Validate checks if a connection is still valid.
	Validate(ctx context.Context, conn Connection) bool
}

// Connection represents a pooled connection.
type Connection interface {
	// Close closes the connection.
	Close() error

	// IsHealthy returns true if the connection is healthy.
	IsHealthy(ctx context.Context) bool
}

// pooledConnection wraps a connection with pool metadata.
type pooledConnection struct {
	conn       Connection
	createdAt  time.Time
	lastUsedAt time.Time
	useCount   int64
	healthy    bool
}

// PoolConfig configures a connection pool.
type PoolConfig struct {
	// MinConnections is the minimum number of connections to maintain.
	MinConnections int `json:"min_connections" yaml:"min_connections"`

	// MaxConnections is the maximum number of connections allowed.
	MaxConnections int `json:"max_connections" yaml:"max_connections"`

	// MaxIdleTime is the maximum time a connection can be idle before being closed.
	MaxIdleTime time.Duration `json:"max_idle_time" yaml:"max_idle_time"`

	// MaxLifetime is the maximum lifetime of a connection.
	MaxLifetime time.Duration `json:"max_lifetime" yaml:"max_lifetime"`

	// AcquireTimeout is the maximum time to wait for a connection.
	AcquireTimeout time.Duration `json:"acquire_timeout" yaml:"acquire_timeout"`

	// ValidationInterval is how often to validate idle connections.
	ValidationInterval time.Duration `json:"validation_interval" yaml:"validation_interval"`

	// HealthCheckOnAcquire validates connections before returning them.
	HealthCheckOnAcquire bool `json:"health_check_on_acquire" yaml:"health_check_on_acquire"`
}

// DefaultPoolConfig returns a pool configuration with sensible defaults.
func DefaultPoolConfig() *PoolConfig {
	return &PoolConfig{
		MinConnections:       2,
		MaxConnections:       10,
		MaxIdleTime:          5 * time.Minute,
		MaxLifetime:          30 * time.Minute,
		AcquireTimeout:       10 * time.Second,
		ValidationInterval:   30 * time.Second,
		HealthCheckOnAcquire: true,
	}
}

// PoolStats contains connection pool statistics.
type PoolStats struct {
	// TotalCreated is the total number of connections created.
	TotalCreated int64 `json:"total_created"`

	// TotalClosed is the total number of connections closed.
	TotalClosed int64 `json:"total_closed"`

	// CurrentSize is the current pool size.
	CurrentSize int `json:"current_size"`

	// AvailableConnections is the number of available connections.
	AvailableConnections int `json:"available_connections"`

	// InUse is the number of connections currently in use.
	InUse int `json:"in_use"`

	// AcquireCount is the total number of acquire operations.
	AcquireCount int64 `json:"acquire_count"`

	// AcquireTimeouts is the number of acquire timeouts.
	AcquireTimeouts int64 `json:"acquire_timeouts"`

	// AcquireLatencyMs is the average acquire latency in milliseconds.
	AcquireLatencyMs float64 `json:"acquire_latency_ms"`

	// IdleConnectionsClosed is connections closed due to idle timeout.
	IdleConnectionsClosed int64 `json:"idle_connections_closed"`

	// MaxLifetimeConnectionsClosed is connections closed due to max lifetime.
	MaxLifetimeConnectionsClosed int64 `json:"max_lifetime_connections_closed"`

	// HealthCheckFailures is the number of health check failures.
	HealthCheckFailures int64 `json:"health_check_failures"`
}

// NewConnectionPool creates a new connection pool.
func NewConnectionPool(config *PoolConfig, factory ConnectionFactory) (*ConnectionPool, error) {
	if config == nil {
		config = DefaultPoolConfig()
	}
	if factory == nil {
		return nil, fmt.Errorf("connection factory is required")
	}

	if config.MaxConnections <= 0 {
		config.MaxConnections = 10
	}
	if config.MinConnections < 0 {
		config.MinConnections = 0
	}
	if config.MinConnections > config.MaxConnections {
		config.MinConnections = config.MaxConnections
	}

	pool := &ConnectionPool{
		config:      config,
		factory:     factory,
		connections: make([]*pooledConnection, 0, config.MaxConnections),
		available:   make(chan *pooledConnection, config.MaxConnections),
	}

	return pool, nil
}

// Start initializes the pool with minimum connections.
func (p *ConnectionPool) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return fmt.Errorf("pool is closed")
	}

	// Create minimum connections
	for i := 0; i < p.config.MinConnections; i++ {
		conn, err := p.createConnection(ctx)
		if err != nil {
			return fmt.Errorf("failed to create initial connection %d: %w", i, err)
		}
		p.connections = append(p.connections, conn)
		p.available <- conn
	}

	// Start background maintenance with cancellable context
	maintainCtx, cancel := context.WithCancel(ctx)
	p.cancelMaintain = cancel
	go p.maintenanceLoop(maintainCtx)

	return nil
}

// Acquire gets a connection from the pool.
func (p *ConnectionPool) Acquire(ctx context.Context) (Connection, error) {
	startTime := time.Now()
	atomic.AddInt64(&p.stats.AcquireCount, 1)

	// Try to get from available connections first
	timeout := p.config.AcquireTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()

		case <-timer.C:
			atomic.AddInt64(&p.stats.AcquireTimeouts, 1)
			return nil, fmt.Errorf("acquire timeout after %v", timeout)

		case conn := <-p.available:
			// Validate connection if configured
			if p.config.HealthCheckOnAcquire {
				if !p.validateConnection(ctx, conn) {
					// Connection is bad, close it and try again
					p.closeConnection(conn)
					continue
				}
			}

			// Check if connection has exceeded max lifetime
			if p.config.MaxLifetime > 0 && time.Since(conn.createdAt) > p.config.MaxLifetime {
				atomic.AddInt64(&p.stats.MaxLifetimeConnectionsClosed, 1)
				p.closeConnection(conn)
				continue
			}

			conn.lastUsedAt = time.Now()
			conn.useCount++
			p.updateAcquireLatency(time.Since(startTime))
			return conn.conn, nil

		default:
			// No connections available, try to create one
			p.mu.Lock()
			if int(p.created) < p.config.MaxConnections {
				conn, err := p.createConnection(ctx)
				if err != nil {
					p.mu.Unlock()
					// Wait a bit and retry
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case <-time.After(50 * time.Millisecond):
						continue
					}
				}
				p.connections = append(p.connections, conn)
				conn.lastUsedAt = time.Now()
				conn.useCount++
				p.mu.Unlock()
				p.updateAcquireLatency(time.Since(startTime))
				return conn.conn, nil
			}
			p.mu.Unlock()

			// Pool is at capacity, wait for a connection
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(10 * time.Millisecond):
				// Try again
			}
		}
	}
}

// Release returns a connection to the pool.
func (p *ConnectionPool) Release(conn Connection) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		conn.Close()
		return
	}

	// Find the pooled connection wrapper
	for _, pc := range p.connections {
		if pc.conn == conn {
			pc.lastUsedAt = time.Now()
			select {
			case p.available <- pc:
				// Returned to pool
			default:
				// Pool is full, close connection
				p.closeConnectionLocked(pc)
			}
			return
		}
	}

	// Connection not from this pool, close it
	conn.Close()
}

// Stats returns the current pool statistics.
func (p *ConnectionPool) Stats() PoolStats {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.stats.CurrentSize = len(p.connections)
	p.stats.AvailableConnections = len(p.available)
	p.stats.InUse = p.stats.CurrentSize - p.stats.AvailableConnections

	return p.stats
}

// Close closes the pool and all connections.
func (p *ConnectionPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}
	p.closed = true

	// Stop the maintenance goroutine
	if p.cancelMaintain != nil {
		p.cancelMaintain()
	}

	// Close all connections
	close(p.available)
	for _, conn := range p.connections {
		conn.conn.Close()
		atomic.AddInt64(&p.stats.TotalClosed, 1)
	}
	p.connections = nil

	return nil
}

func (p *ConnectionPool) createConnection(ctx context.Context) (*pooledConnection, error) {
	conn, err := p.factory.Create(ctx)
	if err != nil {
		return nil, err
	}

	atomic.AddInt32(&p.created, 1)
	atomic.AddInt64(&p.stats.TotalCreated, 1)

	return &pooledConnection{
		conn:       conn,
		createdAt:  time.Now(),
		lastUsedAt: time.Now(),
		healthy:    true,
	}, nil
}

func (p *ConnectionPool) validateConnection(ctx context.Context, conn *pooledConnection) bool {
	if !p.factory.Validate(ctx, conn.conn) {
		atomic.AddInt64(&p.stats.HealthCheckFailures, 1)
		conn.healthy = false
		return false
	}
	conn.healthy = true
	return true
}

func (p *ConnectionPool) closeConnection(conn *pooledConnection) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeConnectionLocked(conn)
}

func (p *ConnectionPool) closeConnectionLocked(conn *pooledConnection) {
	conn.conn.Close()
	atomic.AddInt32(&p.created, -1)
	atomic.AddInt64(&p.stats.TotalClosed, 1)

	// Remove from connections slice
	for i, c := range p.connections {
		if c == conn {
			p.connections = append(p.connections[:i], p.connections[i+1:]...)
			break
		}
	}
}

func (p *ConnectionPool) updateAcquireLatency(d time.Duration) {
	// Simple exponential moving average
	ms := float64(d.Milliseconds())
	p.mu.Lock()
	if p.stats.AcquireLatencyMs == 0 {
		p.stats.AcquireLatencyMs = ms
	} else {
		p.stats.AcquireLatencyMs = p.stats.AcquireLatencyMs*0.9 + ms*0.1
	}
	p.mu.Unlock()
}

func (p *ConnectionPool) maintenanceLoop(ctx context.Context) {
	interval := p.config.ValidationInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.performMaintenance(ctx)
		}
	}
}

func (p *ConnectionPool) performMaintenance(ctx context.Context) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}

	now := time.Now()
	var toClose []*pooledConnection
	var toValidate []*pooledConnection

	// Check for idle and expired connections
	availableConns := make([]*pooledConnection, 0)
drainLoop:
	for {
		select {
		case conn := <-p.available:
			// Check idle timeout
			if p.config.MaxIdleTime > 0 && now.Sub(conn.lastUsedAt) > p.config.MaxIdleTime {
				// Only close if above minimum
				if len(p.connections)-len(toClose) > p.config.MinConnections {
					toClose = append(toClose, conn)
					atomic.AddInt64(&p.stats.IdleConnectionsClosed, 1)
					continue
				}
			}

			// Check max lifetime
			if p.config.MaxLifetime > 0 && now.Sub(conn.createdAt) > p.config.MaxLifetime {
				toClose = append(toClose, conn)
				atomic.AddInt64(&p.stats.MaxLifetimeConnectionsClosed, 1)
				continue
			}

			toValidate = append(toValidate, conn)
		default:
			break drainLoop
		}
	}
	p.mu.Unlock()

	// Validate connections outside the lock
	for _, conn := range toValidate {
		if p.factory.Validate(ctx, conn.conn) {
			availableConns = append(availableConns, conn)
		} else {
			toClose = append(toClose, conn)
			atomic.AddInt64(&p.stats.HealthCheckFailures, 1)
		}
	}

	// Close bad connections
	for _, conn := range toClose {
		p.closeConnection(conn)
	}

	// Return valid connections to the pool
	p.mu.Lock()
	for _, conn := range availableConns {
		if !p.closed {
			select {
			case p.available <- conn:
			default:
			}
		}
	}

	// Ensure minimum connections
	for len(p.connections) < p.config.MinConnections && !p.closed {
		conn, err := p.createConnection(ctx)
		if err != nil {
			break
		}
		p.connections = append(p.connections, conn)
		select {
		case p.available <- conn:
		default:
		}
	}
	p.mu.Unlock()
}

// =============================================================================
// Pooled Backend Wrapper
// =============================================================================

// PooledBackend wraps a SecretBackend with connection pooling.
type PooledBackend struct {
	backend SecretBackend
	pool    *ConnectionPool
}

// BackendConnectionFactory creates connections for a secret backend.
type BackendConnectionFactory struct {
	backend SecretBackend
}

// BackendConnection represents a connection to a secret backend.
type BackendConnection struct {
	backend SecretBackend
}

// NewBackendConnectionFactory creates a new backend connection factory.
func NewBackendConnectionFactory(backend SecretBackend) *BackendConnectionFactory {
	return &BackendConnectionFactory{backend: backend}
}

// Create creates a new backend connection.
func (f *BackendConnectionFactory) Create(ctx context.Context) (Connection, error) {
	return &BackendConnection{backend: f.backend}, nil
}

// Validate checks if a backend connection is valid.
func (f *BackendConnectionFactory) Validate(ctx context.Context, conn Connection) bool {
	bc, ok := conn.(*BackendConnection)
	if !ok {
		return false
	}
	return bc.backend.Healthy(ctx)
}

// Close is a no-op — backend lifecycle is managed by the connection pool,
// not by individual connections.
func (c *BackendConnection) Close() error {
	return nil
}

// IsHealthy checks if the backend connection is healthy.
func (c *BackendConnection) IsHealthy(ctx context.Context) bool {
	return c.backend.Healthy(ctx)
}

// NewPooledBackend creates a new pooled backend.
func NewPooledBackend(backend SecretBackend, config *PoolConfig) (*PooledBackend, error) {
	factory := NewBackendConnectionFactory(backend)
	pool, err := NewConnectionPool(config, factory)
	if err != nil {
		return nil, err
	}

	return &PooledBackend{
		backend: backend,
		pool:    pool,
	}, nil
}

// Start starts the pooled backend.
func (p *PooledBackend) Start(ctx context.Context) error {
	return p.pool.Start(ctx)
}

// Type returns the backend type.
func (p *PooledBackend) Type() BackendType {
	return p.backend.Type()
}

// Name returns the backend name.
func (p *PooledBackend) Name() string {
	return p.backend.Name()
}

// Healthy returns true if the backend is healthy.
func (p *PooledBackend) Healthy(ctx context.Context) bool {
	return p.backend.Healthy(ctx)
}

// Read reads a secret using a pooled connection.
func (p *PooledBackend) Read(ctx context.Context, req *SecretRequest) (*Secret, error) {
	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer p.pool.Release(conn)

	return p.backend.Read(ctx, req)
}

// ReadDynamic reads a dynamic secret using a pooled connection.
func (p *PooledBackend) ReadDynamic(ctx context.Context, req *SecretRequest) (*Secret, error) {
	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer p.pool.Release(conn)

	return p.backend.ReadDynamic(ctx, req)
}

// List lists secrets using a pooled connection.
func (p *PooledBackend) List(ctx context.Context, prefix string) ([]string, error) {
	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer p.pool.Release(conn)

	return p.backend.List(ctx, prefix)
}

// RenewLease renews a lease using a pooled connection.
func (p *PooledBackend) RenewLease(ctx context.Context, leaseID string, increment time.Duration) (*Lease, error) {
	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer p.pool.Release(conn)

	return p.backend.RenewLease(ctx, leaseID, increment)
}

// RevokeLease revokes a lease using a pooled connection.
func (p *PooledBackend) RevokeLease(ctx context.Context, leaseID string) error {
	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer p.pool.Release(conn)

	return p.backend.RevokeLease(ctx, leaseID)
}

// Close closes the pooled backend.
func (p *PooledBackend) Close() error {
	if err := p.pool.Close(); err != nil {
		return err
	}
	return p.backend.Close()
}

// PoolStats returns the connection pool statistics.
func (p *PooledBackend) PoolStats() PoolStats {
	return p.pool.Stats()
}
