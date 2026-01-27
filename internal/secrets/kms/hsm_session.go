// Package kms provides HSM session management with pooling, health checks, and automatic recovery.
package kms

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

// HSMSessionConfig contains configuration for HSM session management.
type HSMSessionConfig struct {
	// MinSessions is the minimum number of sessions to maintain.
	MinSessions int `json:"min_sessions,omitempty"`

	// MaxSessions is the maximum number of sessions allowed.
	MaxSessions int `json:"max_sessions,omitempty"`

	// IdleTimeout is the duration after which idle sessions are closed.
	IdleTimeout time.Duration `json:"idle_timeout,omitempty"`

	// MaxSessionAge is the maximum age of a session before it's recycled.
	MaxSessionAge time.Duration `json:"max_session_age,omitempty"`

	// HealthCheckInterval is the interval between health checks.
	HealthCheckInterval time.Duration `json:"health_check_interval,omitempty"`

	// AcquireTimeout is the timeout for acquiring a session.
	AcquireTimeout time.Duration `json:"acquire_timeout,omitempty"`

	// RetryAttempts is the number of retry attempts for failed operations.
	RetryAttempts int `json:"retry_attempts,omitempty"`

	// RetryDelay is the delay between retry attempts.
	RetryDelay time.Duration `json:"retry_delay,omitempty"`
}

// DefaultHSMSessionConfig returns default session configuration.
func DefaultHSMSessionConfig() *HSMSessionConfig {
	return &HSMSessionConfig{
		MinSessions:         2,
		MaxSessions:         10,
		IdleTimeout:         5 * time.Minute,
		MaxSessionAge:       30 * time.Minute,
		HealthCheckInterval: 30 * time.Second,
		AcquireTimeout:      10 * time.Second,
		RetryAttempts:       3,
		RetryDelay:          1 * time.Second,
	}
}

// HSMSessionState represents the state of an HSM session.
//
// State Diagram (Mermaid):
//
//	stateDiagram-v2
//	    [*] --> Idle
//	    Idle --> Active: acquire
//	    Active --> Idle: release
//	    Active --> Invalid: error
//	    Idle --> Closed: close
//	    Active --> Closed: close
//	    Invalid --> Closed: close
//	    Closed --> [*]
type HSMSessionState int

const (
	HSMSessionStateIdle HSMSessionState = iota
	HSMSessionStateActive
	HSMSessionStateInvalid
	HSMSessionStateClosed
)

func (s HSMSessionState) String() string {
	switch s {
	case HSMSessionStateIdle:
		return "idle"
	case HSMSessionStateActive:
		return "active"
	case HSMSessionStateInvalid:
		return "invalid"
	case HSMSessionStateClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// HSMSessionEvent represents events that trigger session state transitions.
type HSMSessionEvent string

const (
	// HSMSessionEventAcquire acquires a session from the pool.
	HSMSessionEventAcquire HSMSessionEvent = "acquire"

	// HSMSessionEventRelease returns a session to the pool.
	HSMSessionEventRelease HSMSessionEvent = "release"

	// HSMSessionEventError marks a session as invalid due to an error.
	HSMSessionEventError HSMSessionEvent = "error"

	// HSMSessionEventClose closes a session permanently.
	HSMSessionEventClose HSMSessionEvent = "close"
)

// HSMPooledSession wraps an HSM session with pool metadata.
type HSMPooledSession struct {
	Session    *Session
	Pool       *HSMSessionPool
	State      HSMSessionState
	machine    *statemachine.Machine[HSMSessionState, HSMSessionEvent]
	CreatedAt  time.Time
	LastUsedAt time.Time
	UseCount   uint64
	Errors     uint32
	element    *list.Element
}

// buildSessionStateMachine creates a state machine for a pooled session.
func buildSessionStateMachine(session *HSMPooledSession) *statemachine.Machine[HSMSessionState, HSMSessionEvent] {
	builder := statemachine.New[HSMSessionState, HSMSessionEvent](HSMSessionStateIdle).
		WithHistory(15).
		WithName("hsm-session")

	// Idle -> Active (acquire)
	builder.AddTransition(HSMSessionStateIdle, HSMSessionEventAcquire, HSMSessionStateActive)

	// Active -> Idle (release)
	builder.AddTransition(HSMSessionStateActive, HSMSessionEventRelease, HSMSessionStateIdle)

	// Active -> Invalid (error)
	builder.AddTransition(HSMSessionStateActive, HSMSessionEventError, HSMSessionStateInvalid)

	// Any state -> Closed
	builder.AddTransition(HSMSessionStateIdle, HSMSessionEventClose, HSMSessionStateClosed)
	builder.AddTransition(HSMSessionStateActive, HSMSessionEventClose, HSMSessionStateClosed)
	builder.AddTransition(HSMSessionStateInvalid, HSMSessionEventClose, HSMSessionStateClosed)

	// State entry callbacks
	builder.OnEnter(HSMSessionStateActive, func(_ context.Context, _ HSMSessionState, _ HSMSessionState) {
		session.LastUsedAt = time.Now()
		session.UseCount++
	})

	builder.OnEnter(HSMSessionStateInvalid, func(_ context.Context, _ HSMSessionState, _ HSMSessionState) {
		atomic.AddUint32(&session.Errors, 1)
	})

	// Global transition callback to sync State field
	builder.OnTransition(func(_ context.Context, _, to HSMSessionState, _ HSMSessionEvent) {
		session.State = to
	})

	return builder.MustBuild()
}

// IsValid checks if the session is still valid.
func (s *HSMPooledSession) IsValid() bool {
	if s.State == HSMSessionStateInvalid || s.State == HSMSessionStateClosed {
		return false
	}
	if s.Pool.config.MaxSessionAge > 0 && time.Since(s.CreatedAt) > s.Pool.config.MaxSessionAge {
		return false
	}
	return true
}

// IsIdle checks if the session is idle and can be closed.
func (s *HSMPooledSession) IsIdle() bool {
	if s.State != HSMSessionStateIdle {
		return false
	}
	return s.Pool.config.IdleTimeout > 0 && time.Since(s.LastUsedAt) > s.Pool.config.IdleTimeout
}

// Release returns the session to the pool.
func (s *HSMPooledSession) Release() {
	s.Pool.releaseSession(s)
}

// Invalidate marks the session as invalid.
func (s *HSMPooledSession) Invalidate() {
	_ = s.machine.Fire(HSMSessionEventError)
}

// SessionState returns the current state from the state machine.
func (s *HSMPooledSession) SessionState() HSMSessionState {
	return s.machine.State()
}

// History returns the session state transition history.
func (s *HSMPooledSession) History() *statemachine.History[HSMSessionState, HSMSessionEvent] {
	return s.machine.History()
}

// HSMSessionPool manages a pool of HSM sessions.
type HSMSessionPool struct {
	config *HSMSessionConfig
	iface  PKCS11Interface
	slotID uint32
	pin    string

	mu           sync.Mutex
	sessions     *list.List
	sessionMap   map[SessionHandle]*HSMPooledSession
	activeCount  int32
	totalCreated uint64
	totalClosed  uint64

	stopCh  chan struct{}
	stopped bool

	stats *HSMSessionStats
}

// HSMSessionStats contains session pool statistics.
type HSMSessionStats struct {
	mu sync.RWMutex

	TotalCreated      uint64        `json:"total_created"`
	TotalClosed       uint64        `json:"total_closed"`
	TotalAcquired     uint64        `json:"total_acquired"`
	TotalReleased     uint64        `json:"total_released"`
	TotalErrors       uint64        `json:"total_errors"`
	AverageAcquireTime time.Duration `json:"average_acquire_time"`

	CurrentActive int `json:"current_active"`
	CurrentIdle   int `json:"current_idle"`
	CurrentTotal  int `json:"current_total"`
}

// NewHSMSessionPool creates a new HSM session pool.
func NewHSMSessionPool(ctx context.Context, config *HSMSessionConfig, iface PKCS11Interface, slotID uint32, pin string) (*HSMSessionPool, error) {
	if config == nil {
		config = DefaultHSMSessionConfig()
	}
	if iface == nil {
		return nil, errors.New("PKCS#11 interface is required")
	}

	// Apply defaults for zero values
	if config.HealthCheckInterval == 0 {
		config.HealthCheckInterval = 30 * time.Second
	}
	if config.AcquireTimeout == 0 {
		config.AcquireTimeout = 10 * time.Second
	}

	pool := &HSMSessionPool{
		config:     config,
		iface:      iface,
		slotID:     slotID,
		pin:        pin,
		sessions:   list.New(),
		sessionMap: make(map[SessionHandle]*HSMPooledSession),
		stopCh:     make(chan struct{}),
		stats:      &HSMSessionStats{},
	}

	for i := 0; i < config.MinSessions; i++ {
		if _, err := pool.createSession(ctx); err != nil {
			pool.Close()
			return nil, fmt.Errorf("failed to create initial session: %w", err)
		}
	}

	go pool.maintenanceLoop()

	return pool, nil
}

// Acquire acquires a session from the pool.
func (p *HSMSessionPool) Acquire(ctx context.Context) (*HSMPooledSession, error) {
	startTime := time.Now()

	deadline := time.Now().Add(p.config.AcquireTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}

	for {
		session, err := p.tryAcquire(ctx)
		if err == nil {
			p.recordAcquire(time.Since(startTime))
			return session, nil
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timeout acquiring session: %w", err)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// tryAcquire attempts to acquire a session.
func (p *HSMSessionPool) tryAcquire(ctx context.Context) (*HSMPooledSession, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for e := p.sessions.Front(); e != nil; e = e.Next() {
		session := e.Value.(*HSMPooledSession)
		if session.State == HSMSessionStateIdle && session.IsValid() {
			if err := session.machine.Fire(HSMSessionEventAcquire); err == nil {
				atomic.AddInt32(&p.activeCount, 1)
				return session, nil
			}
		}
	}

	if p.sessions.Len() < p.config.MaxSessions {
		session, err := p.createSessionLocked(ctx)
		if err != nil {
			return nil, err
		}
		if err := session.machine.Fire(HSMSessionEventAcquire); err != nil {
			return nil, fmt.Errorf("failed to acquire new session: %w", err)
		}
		atomic.AddInt32(&p.activeCount, 1)
		return session, nil
	}

	return nil, errors.New("no sessions available")
}

// createSession creates a new session (acquires lock).
func (p *HSMSessionPool) createSession(ctx context.Context) (*HSMPooledSession, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.createSessionLocked(ctx)
}

// createSessionLocked creates a new session (must hold lock).
func (p *HSMSessionPool) createSessionLocked(ctx context.Context) (*HSMPooledSession, error) {
	session, err := p.iface.OpenSession(ctx, p.slotID, CKF_RW_SESSION|CKF_SERIAL_SESSION)
	if err != nil {
		return nil, fmt.Errorf("failed to open session: %w", err)
	}

	if p.pin != "" {
		if err := p.iface.Login(ctx, session.Handle, CKU_USER, p.pin); err != nil {
			var pkcs11Err *PKCS11Error
			if !errors.As(err, &pkcs11Err) || pkcs11Err.Code != CKR_USER_ALREADY_LOGGED_IN {
				p.iface.CloseSession(ctx, session.Handle)
				return nil, fmt.Errorf("failed to login: %w", err)
			}
		}
	}

	pooledSession := &HSMPooledSession{
		Session:    session,
		Pool:       p,
		State:      HSMSessionStateIdle,
		CreatedAt:  time.Now(),
		LastUsedAt: time.Now(),
	}
	pooledSession.machine = buildSessionStateMachine(pooledSession)

	pooledSession.element = p.sessions.PushBack(pooledSession)
	p.sessionMap[session.Handle] = pooledSession
	p.totalCreated++

	return pooledSession, nil
}

// releaseSession returns a session to the pool.
func (p *HSMSessionPool) releaseSession(session *HSMPooledSession) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if session.State != HSMSessionStateActive {
		return
	}

	session.LastUsedAt = time.Now()
	atomic.AddInt32(&p.activeCount, -1)

	if !session.IsValid() || session.Errors > 3 {
		p.closeSessionLocked(context.Background(), session)
		return
	}

	// Transition to idle via state machine
	if err := session.machine.Fire(HSMSessionEventRelease); err != nil {
		p.closeSessionLocked(context.Background(), session)
		return
	}
	p.sessions.MoveToFront(session.element)

	p.stats.mu.Lock()
	p.stats.TotalReleased++
	p.stats.mu.Unlock()
}

// closeSessionLocked closes a session (must hold lock).
func (p *HSMSessionPool) closeSessionLocked(ctx context.Context, session *HSMPooledSession) {
	if session.State == HSMSessionStateClosed {
		return
	}

	// Transition to closed via state machine
	_ = session.machine.Fire(HSMSessionEventClose)
	p.iface.CloseSession(ctx, session.Session.Handle)

	if session.element != nil {
		p.sessions.Remove(session.element)
	}
	delete(p.sessionMap, session.Session.Handle)
	p.totalClosed++
}

// maintenanceLoop performs periodic maintenance on the pool.
func (p *HSMSessionPool) maintenanceLoop() {
	ticker := time.NewTicker(p.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.performMaintenance()
		}
	}
}

// performMaintenance cleans up idle sessions and ensures minimum pool size.
func (p *HSMSessionPool) performMaintenance() {
	p.mu.Lock()
	defer p.mu.Unlock()

	var toClose []*HSMPooledSession
	for e := p.sessions.Back(); e != nil; e = e.Prev() {
		session := e.Value.(*HSMPooledSession)

		if session.State != HSMSessionStateIdle {
			continue
		}

		if session.IsIdle() && p.sessions.Len()-len(toClose) > p.config.MinSessions {
			toClose = append(toClose, session)
		}

		if !session.IsValid() {
			toClose = append(toClose, session)
		}
	}

	ctx := context.Background()
	for _, session := range toClose {
		p.closeSessionLocked(ctx, session)
	}

	for p.sessions.Len() < p.config.MinSessions {
		if _, err := p.createSessionLocked(ctx); err != nil {
			break
		}
	}

	p.updateStats()
}

// updateStats updates pool statistics.
func (p *HSMSessionPool) updateStats() {
	p.stats.mu.Lock()
	defer p.stats.mu.Unlock()

	p.stats.TotalCreated = p.totalCreated
	p.stats.TotalClosed = p.totalClosed
	p.stats.CurrentActive = int(atomic.LoadInt32(&p.activeCount))
	p.stats.CurrentTotal = p.sessions.Len()
	p.stats.CurrentIdle = p.stats.CurrentTotal - p.stats.CurrentActive
}

// recordAcquire records an acquire operation.
func (p *HSMSessionPool) recordAcquire(duration time.Duration) {
	p.stats.mu.Lock()
	defer p.stats.mu.Unlock()

	p.stats.TotalAcquired++
	if p.stats.TotalAcquired == 1 {
		p.stats.AverageAcquireTime = duration
	} else {
		p.stats.AverageAcquireTime = time.Duration(
			(int64(p.stats.AverageAcquireTime)*(int64(p.stats.TotalAcquired)-1) + int64(duration)) /
				int64(p.stats.TotalAcquired))
	}
}

// Stats returns the current pool statistics.
func (p *HSMSessionPool) Stats() HSMSessionStats {
	p.updateStats()
	p.stats.mu.RLock()
	defer p.stats.mu.RUnlock()
	return *p.stats
}

// Healthy checks if the pool is healthy.
func (p *HSMSessionPool) Healthy(ctx context.Context) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	for e := p.sessions.Front(); e != nil; e = e.Next() {
		session := e.Value.(*HSMPooledSession)
		if session.State == HSMSessionStateIdle && session.IsValid() {
			return true
		}
	}

	return false
}

// Close closes all sessions and stops the pool.
func (p *HSMSessionPool) Close() error {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return nil
	}
	p.stopped = true
	close(p.stopCh)

	ctx := context.Background()
	for e := p.sessions.Front(); e != nil; {
		session := e.Value.(*HSMPooledSession)
		next := e.Next()
		p.closeSessionLocked(ctx, session)
		e = next
	}
	p.mu.Unlock()

	return nil
}

// HSMSessionManager manages HSM session pools across multiple providers.
type HSMSessionManager struct {
	mu    sync.RWMutex
	pools map[string]*HSMSessionPool
}

// NewHSMSessionManager creates a new HSM session manager.
func NewHSMSessionManager() *HSMSessionManager {
	return &HSMSessionManager{
		pools: make(map[string]*HSMSessionPool),
	}
}

// RegisterPool registers a session pool with a name.
func (m *HSMSessionManager) RegisterPool(name string, pool *HSMSessionPool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.pools[name]; exists {
		return fmt.Errorf("pool %s already registered", name)
	}

	m.pools[name] = pool
	return nil
}

// UnregisterPool unregisters and closes a session pool.
func (m *HSMSessionManager) UnregisterPool(name string) error {
	m.mu.Lock()
	pool, exists := m.pools[name]
	if exists {
		delete(m.pools, name)
	}
	m.mu.Unlock()

	if !exists {
		return fmt.Errorf("pool %s not found", name)
	}

	return pool.Close()
}

// GetPool returns a session pool by name.
func (m *HSMSessionManager) GetPool(name string) (*HSMSessionPool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pool, exists := m.pools[name]
	if !exists {
		return nil, fmt.Errorf("pool %s not found", name)
	}

	return pool, nil
}

// AcquireSession acquires a session from a named pool.
func (m *HSMSessionManager) AcquireSession(ctx context.Context, poolName string) (*HSMPooledSession, error) {
	pool, err := m.GetPool(poolName)
	if err != nil {
		return nil, err
	}

	return pool.Acquire(ctx)
}

// Stats returns statistics for all pools.
func (m *HSMSessionManager) Stats() map[string]HSMSessionStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]HSMSessionStats)
	for name, pool := range m.pools {
		stats[name] = pool.Stats()
	}

	return stats
}

// Healthy checks if all pools are healthy.
func (m *HSMSessionManager) Healthy(ctx context.Context) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, pool := range m.pools {
		if !pool.Healthy(ctx) {
			return false
		}
	}

	return true
}

// Close closes all session pools.
func (m *HSMSessionManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var firstErr error
	for name, pool := range m.pools {
		if err := pool.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("failed to close pool %s: %w", name, err)
		}
	}
	m.pools = make(map[string]*HSMSessionPool)

	return firstErr
}

// WithSession executes a function with an acquired session.
func WithSession(ctx context.Context, pool *HSMSessionPool, fn func(session *HSMPooledSession) error) error {
	session, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer session.Release()

	return fn(session)
}

// WithRetry executes a function with retry on failure.
func WithRetry(ctx context.Context, pool *HSMSessionPool, config *HSMSessionConfig, fn func(session *HSMPooledSession) error) error {
	var lastErr error

	for attempt := 0; attempt <= config.RetryAttempts; attempt++ {
		session, err := pool.Acquire(ctx)
		if err != nil {
			lastErr = err
			if attempt < config.RetryAttempts {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(config.RetryDelay):
				}
			}
			continue
		}

		err = fn(session)
		if err == nil {
			session.Release()
			return nil
		}

		var pkcs11Err *PKCS11Error
		if errors.As(err, &pkcs11Err) {
			switch pkcs11Err.Code {
			case CKR_SESSION_CLOSED, CKR_SESSION_HANDLE_INVALID,
				CKR_DEVICE_ERROR, CKR_DEVICE_REMOVED, CKR_TOKEN_NOT_PRESENT:
				session.Invalidate()
			}
		}
		session.Release()

		lastErr = err
		if attempt < config.RetryAttempts {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(config.RetryDelay):
			}
		}
	}

	return fmt.Errorf("operation failed after %d attempts: %w", config.RetryAttempts+1, lastErr)
}
