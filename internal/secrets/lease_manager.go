package secrets

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

// LeaseTransitionEvent represents events that trigger lease state transitions.
//
// State Diagram (Mermaid):
//
//	stateDiagram-v2
//	    [*] --> Pending
//	    Pending --> Active: activate
//	    Active --> Renewing: renew_start
//	    Renewing --> Active: renew_success
//	    Renewing --> Active: renew_failed (revert)
//	    Active --> Expired: expire
//	    Renewing --> Expired: expire
//	    Active --> Revoked: revoke
//	    Renewing --> Revoked: revoke
type LeaseTransitionEvent string

const (
	// LeaseTransitionEventActivate activates a pending lease.
	LeaseTransitionEventActivate LeaseTransitionEvent = "activate"

	// LeaseTransitionEventRenewStart starts a renewal operation.
	LeaseTransitionEventRenewStart LeaseTransitionEvent = "renew_start"

	// LeaseTransitionEventRenewSuccess completes a successful renewal.
	LeaseTransitionEventRenewSuccess LeaseTransitionEvent = "renew_success"

	// LeaseTransitionEventRenewFailed reverts state after failed renewal.
	LeaseTransitionEventRenewFailed LeaseTransitionEvent = "renew_failed"

	// LeaseTransitionEventExpire marks a lease as expired.
	LeaseTransitionEventExpire LeaseTransitionEvent = "expire"

	// LeaseTransitionEventRevoke revokes a lease.
	LeaseTransitionEventRevoke LeaseTransitionEvent = "revoke"
)

// leaseStateMachine is a shared state machine for validating lease transitions.
var leaseStateMachine = buildLeaseStateMachine()

// buildLeaseStateMachine creates a state machine for validating lease transitions.
func buildLeaseStateMachine() *statemachine.Machine[LeaseState, LeaseTransitionEvent] {
	builder := statemachine.New[LeaseState, LeaseTransitionEvent](LeaseStatePending).
		WithName("lease-validator")

	// Pending -> Active (on activate)
	builder.AddTransition(LeaseStatePending, LeaseTransitionEventActivate, LeaseStateActive)

	// Active -> Renewing (on renew_start)
	builder.AddTransition(LeaseStateActive, LeaseTransitionEventRenewStart, LeaseStateRenewing)

	// Renewing -> Active (on renew_success or renew_failed)
	builder.AddTransition(LeaseStateRenewing, LeaseTransitionEventRenewSuccess, LeaseStateActive)
	builder.AddTransition(LeaseStateRenewing, LeaseTransitionEventRenewFailed, LeaseStateActive)

	// Active/Renewing -> Expired (on expire)
	builder.AddTransition(LeaseStateActive, LeaseTransitionEventExpire, LeaseStateExpired)
	builder.AddTransition(LeaseStateRenewing, LeaseTransitionEventExpire, LeaseStateExpired)

	// Active/Renewing -> Revoked (on revoke)
	builder.AddTransition(LeaseStateActive, LeaseTransitionEventRevoke, LeaseStateRevoked)
	builder.AddTransition(LeaseStateRenewing, LeaseTransitionEventRevoke, LeaseStateRevoked)

	// Ignore events on terminal states
	builder.Ignore(LeaseStateExpired, LeaseTransitionEventExpire)
	builder.Ignore(LeaseStateExpired, LeaseTransitionEventRevoke)
	builder.Ignore(LeaseStateRevoked, LeaseTransitionEventRevoke)
	builder.Ignore(LeaseStateRevoked, LeaseTransitionEventExpire)

	return builder.MustBuild()
}

// CanTransitionLease checks if a lease can transition from its current state via an event.
func CanTransitionLease(from LeaseState, event LeaseTransitionEvent) bool {
	// Use the NextLeaseState function to check if a transition is valid
	_, ok := NextLeaseState(from, event)
	if ok {
		return true
	}

	// Also allow ignored events on terminal states
	if from == LeaseStateExpired || from == LeaseStateRevoked {
		// These events are ignored (no-op) on terminal states
		switch event {
		case LeaseTransitionEventExpire, LeaseTransitionEventRevoke:
			return true
		}
	}

	return false
}

// NextLeaseState returns the target state for a lease transition.
func NextLeaseState(from LeaseState, event LeaseTransitionEvent) (LeaseState, bool) {
	transitions := map[LeaseState]map[LeaseTransitionEvent]LeaseState{
		LeaseStatePending: {
			LeaseTransitionEventActivate: LeaseStateActive,
		},
		LeaseStateActive: {
			LeaseTransitionEventRenewStart: LeaseStateRenewing,
			LeaseTransitionEventExpire:     LeaseStateExpired,
			LeaseTransitionEventRevoke:     LeaseStateRevoked,
		},
		LeaseStateRenewing: {
			LeaseTransitionEventRenewSuccess: LeaseStateActive,
			LeaseTransitionEventRenewFailed:  LeaseStateActive,
			LeaseTransitionEventExpire:       LeaseStateExpired,
			LeaseTransitionEventRevoke:       LeaseStateRevoked,
		},
	}

	if stateTransitions, ok := transitions[from]; ok {
		if target, ok := stateTransitions[event]; ok {
			return target, true
		}
	}
	return from, false
}

// PersistentLeaseManager provides a database-backed implementation of LeaseManager.
type PersistentLeaseManager struct {
	mu sync.RWMutex

	// store is the lease storage backend.
	store LeaseStore

	// config is the renewal configuration.
	config *LeaseRenewalConfig

	// backends maps backend types to their implementations for renewal.
	backends map[BackendType]SecretBackend

	// auditLogger logs lease events.
	auditLogger SecretAuditLogger

	// callbacks are functions called on lease lifecycle events.
	callbacks *LeaseCallbacks

	// stats tracks lease statistics.
	stats LeaseManagerStats

	// renewal goroutine management
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// started indicates if the renewal scheduler is running.
	started bool

	// cleanupInterval is how often to run cleanup.
	cleanupInterval time.Duration

	// renewalBatchSize is the max number of leases to renew per cycle.
	renewalBatchSize int

	// renewalConcurrency is the number of concurrent renewal operations.
	renewalConcurrency int
}

// LeaseManagerStats tracks lease manager statistics.
type LeaseManagerStats struct {
	mu             sync.RWMutex
	TotalRenewals  int64 `json:"total_renewals"`
	FailedRenewals int64 `json:"failed_renewals"`
	TotalRevokes   int64 `json:"total_revokes"`
	TotalExpired   int64 `json:"total_expired"`
	TotalCleanups  int64 `json:"total_cleanups"`
}

// LeaseCallbacks contains callback functions for lease lifecycle events.
type LeaseCallbacks struct {
	// OnRenew is called after a lease is renewed.
	OnRenew func(ctx context.Context, lease *Lease)

	// OnRenewFailed is called when a lease renewal fails.
	OnRenewFailed func(ctx context.Context, lease *Lease, err error)

	// OnExpire is called when a lease expires.
	OnExpire func(ctx context.Context, lease *Lease)

	// OnRevoke is called when a lease is revoked.
	OnRevoke func(ctx context.Context, lease *Lease)

	// OnCleanup is called when a lease is cleaned up.
	OnCleanup func(ctx context.Context, leaseID string)
}

// PersistentLeaseManagerConfig configures the persistent lease manager.
type PersistentLeaseManagerConfig struct {
	// Store is the lease storage backend.
	Store LeaseStore

	// RenewalConfig is the lease renewal configuration.
	RenewalConfig *LeaseRenewalConfig

	// CleanupInterval is how often to run cleanup.
	CleanupInterval time.Duration

	// RenewalBatchSize is the max number of leases to renew per cycle.
	RenewalBatchSize int

	// RenewalConcurrency is the number of concurrent renewal operations.
	RenewalConcurrency int

	// Callbacks are functions called on lease lifecycle events.
	Callbacks *LeaseCallbacks
}

// NewPersistentLeaseManager creates a new persistent lease manager.
func NewPersistentLeaseManager(config *PersistentLeaseManagerConfig) (*PersistentLeaseManager, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}

	if config.Store == nil {
		return nil, fmt.Errorf("store is required")
	}

	renewalConfig := config.RenewalConfig
	if renewalConfig == nil {
		renewalConfig = DefaultLeaseRenewalConfig()
	}

	cleanupInterval := config.CleanupInterval
	if cleanupInterval == 0 {
		cleanupInterval = 5 * time.Minute
	}

	renewalBatchSize := config.RenewalBatchSize
	if renewalBatchSize == 0 {
		renewalBatchSize = 100
	}

	renewalConcurrency := config.RenewalConcurrency
	if renewalConcurrency == 0 {
		renewalConcurrency = 10
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &PersistentLeaseManager{
		store:              config.Store,
		config:             renewalConfig,
		backends:           make(map[BackendType]SecretBackend),
		callbacks:          config.Callbacks,
		ctx:                ctx,
		cancel:             cancel,
		cleanupInterval:    cleanupInterval,
		renewalBatchSize:   renewalBatchSize,
		renewalConcurrency: renewalConcurrency,
	}, nil
}

// RegisterBackend registers a backend for lease renewal.
func (m *PersistentLeaseManager) RegisterBackend(backendType BackendType, backend SecretBackend) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.backends[backendType] = backend
}

// SetAuditLogger sets the audit logger.
func (m *PersistentLeaseManager) SetAuditLogger(logger SecretAuditLogger) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.auditLogger = logger
}

// SetCallbacks sets the lease callbacks.
func (m *PersistentLeaseManager) SetCallbacks(callbacks *LeaseCallbacks) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callbacks = callbacks
}

// Track starts tracking a lease.
func (m *PersistentLeaseManager) Track(ctx context.Context, lease *Lease) error {
	if lease == nil || lease.ID == "" {
		return fmt.Errorf("lease ID is required")
	}

	// Set initial state using state machine transition
	if lease.State == "" {
		lease.State = LeaseStatePending
	}
	if lease.State == LeaseStatePending {
		if newState, ok := NextLeaseState(lease.State, LeaseTransitionEventActivate); ok {
			lease.State = newState
		}
	}
	if lease.IssuedAt.IsZero() {
		lease.IssuedAt = time.Now()
	}

	err := m.store.Create(ctx, lease)
	if err != nil {
		return fmt.Errorf("failed to track lease: %w", err)
	}

	m.logLeaseTransitionEvent(ctx, lease, AuditActionLeaseCreate, nil)
	return nil
}

// Get retrieves a tracked lease by ID.
func (m *PersistentLeaseManager) Get(ctx context.Context, leaseID string) (*Lease, error) {
	return m.store.Get(ctx, leaseID)
}

// List lists all tracked leases.
func (m *PersistentLeaseManager) List(ctx context.Context) ([]*Lease, error) {
	return m.store.List(ctx, &LeaseFilter{
		States: []LeaseState{LeaseStateActive, LeaseStateRenewing},
	})
}

// ListByPath lists leases for a specific secret path.
func (m *PersistentLeaseManager) ListByPath(ctx context.Context, path string) ([]*Lease, error) {
	return m.store.List(ctx, &LeaseFilter{
		SecretPath: path,
		States:     []LeaseState{LeaseStateActive, LeaseStateRenewing},
	})
}

// ListByBackend lists leases for a specific backend.
func (m *PersistentLeaseManager) ListByBackend(ctx context.Context, backend BackendType) ([]*Lease, error) {
	return m.store.List(ctx, &LeaseFilter{
		Backend: backend,
		States:  []LeaseState{LeaseStateActive, LeaseStateRenewing},
	})
}

// ListExpiring lists leases expiring within the given duration.
func (m *PersistentLeaseManager) ListExpiring(ctx context.Context, within time.Duration) ([]*Lease, error) {
	return m.store.List(ctx, &LeaseFilter{
		State:          LeaseStateActive,
		ExpiringBefore: time.Now().Add(within),
	})
}

// Renew renews a lease.
func (m *PersistentLeaseManager) Renew(ctx context.Context, leaseID string, increment time.Duration) (*Lease, error) {
	// Get the lease
	lease, err := m.store.Get(ctx, leaseID)
	if err != nil {
		return nil, err
	}

	if !lease.Renewable {
		return nil, ErrLeaseNotRenewable
	}

	if lease.IsExpired() {
		// Transition to expired using state machine
		if newState, ok := NextLeaseState(lease.State, LeaseTransitionEventExpire); ok {
			lease.State = newState
		}
		_ = m.store.Update(ctx, lease)
		return nil, ErrLeaseExpired
	}

	// Get backend
	m.mu.RLock()
	backend, backendExists := m.backends[lease.Backend]
	m.mu.RUnlock()

	if !backendExists {
		return nil, fmt.Errorf("%w: %s", ErrBackendNotFound, lease.Backend)
	}

	// Validate transition to renewing
	if !CanTransitionLease(lease.State, LeaseTransitionEventRenewStart) {
		return nil, fmt.Errorf("cannot start renewal from state %s", lease.State)
	}

	// Mark as renewing using state machine
	if newState, ok := NextLeaseState(lease.State, LeaseTransitionEventRenewStart); ok {
		lease.State = newState
	}
	if err := m.store.Update(ctx, lease); err != nil {
		return nil, err
	}

	// Renew at backend
	renewedLease, err := backend.RenewLease(ctx, leaseID, increment)
	if err != nil {
		// Revert state using state machine (RenewFailed returns to Active)
		if newState, ok := NextLeaseState(lease.State, LeaseTransitionEventRenewFailed); ok {
			lease.State = newState
		}
		_ = m.store.Update(ctx, lease)

		m.stats.mu.Lock()
		m.stats.FailedRenewals++
		m.stats.mu.Unlock()

		m.logLeaseTransitionEvent(ctx, lease, AuditActionLeaseRenewFailed, err)

		if m.callbacks != nil && m.callbacks.OnRenewFailed != nil {
			m.callbacks.OnRenewFailed(ctx, lease, err)
		}

		return nil, err
	}

	// Update tracked lease using state machine transition
	if newState, ok := NextLeaseState(lease.State, LeaseTransitionEventRenewSuccess); ok {
		lease.State = newState
	}
	lease.TTL = renewedLease.TTL
	lease.ExpiresAt = renewedLease.ExpiresAt
	lease.LastRenewal = time.Now()
	lease.RenewalCount++

	if err := m.store.Update(ctx, lease); err != nil {
		return nil, err
	}

	m.stats.mu.Lock()
	m.stats.TotalRenewals++
	m.stats.mu.Unlock()

	m.logLeaseTransitionEvent(ctx, lease, AuditActionLeaseRenew, nil)

	if m.callbacks != nil && m.callbacks.OnRenew != nil {
		m.callbacks.OnRenew(ctx, lease)
	}

	return lease, nil
}

// Revoke revokes a lease.
func (m *PersistentLeaseManager) Revoke(ctx context.Context, leaseID string) error {
	lease, err := m.store.Get(ctx, leaseID)
	if err != nil {
		return err
	}

	// Transition to revoked using state machine
	if newState, ok := NextLeaseState(lease.State, LeaseTransitionEventRevoke); ok {
		lease.State = newState
	}
	if err := m.store.Update(ctx, lease); err != nil {
		return err
	}

	m.stats.mu.Lock()
	m.stats.TotalRevokes++
	m.stats.mu.Unlock()

	m.logLeaseTransitionEvent(ctx, lease, AuditActionLeaseRevoke, nil)

	if m.callbacks != nil && m.callbacks.OnRevoke != nil {
		m.callbacks.OnRevoke(ctx, lease)
	}

	return nil
}

// RevokeByPath revokes all leases for a path prefix.
func (m *PersistentLeaseManager) RevokeByPath(ctx context.Context, pathPrefix string) (int, error) {
	count, err := m.store.UpdateState(ctx, &LeaseFilter{
		PathPrefix: pathPrefix,
		State:      LeaseStateActive,
	}, LeaseStateRevoked)

	if err != nil {
		return 0, err
	}

	m.stats.mu.Lock()
	m.stats.TotalRevokes += count
	m.stats.mu.Unlock()

	return int(count), nil
}

// RevokeByBackend revokes all leases for a backend.
func (m *PersistentLeaseManager) RevokeByBackend(ctx context.Context, backend BackendType) (int, error) {
	count, err := m.store.UpdateState(ctx, &LeaseFilter{
		Backend: backend,
		State:   LeaseStateActive,
	}, LeaseStateRevoked)

	if err != nil {
		return 0, err
	}

	return int(count), nil
}

// RevokeByAgentID revokes all leases for an agent.
func (m *PersistentLeaseManager) RevokeByAgentID(ctx context.Context, agentID string) (int, error) {
	count, err := m.store.UpdateState(ctx, &LeaseFilter{
		AgentID: agentID,
		State:   LeaseStateActive,
	}, LeaseStateRevoked)

	if err != nil {
		return 0, err
	}

	return int(count), nil
}

// Remove removes a lease from tracking.
func (m *PersistentLeaseManager) Remove(ctx context.Context, leaseID string) error {
	return m.store.Delete(ctx, leaseID)
}

// Stats returns lease manager statistics.
func (m *PersistentLeaseManager) Stats(ctx context.Context) (*LeaseStats, error) {
	// Get counts from store
	activeCount, err := m.store.Count(ctx, &LeaseFilter{State: LeaseStateActive})
	if err != nil {
		return nil, err
	}

	expiringCount, err := m.store.Count(ctx, &LeaseFilter{
		State:          LeaseStateActive,
		ExpiringBefore: time.Now().Add(time.Hour),
	})
	if err != nil {
		return nil, err
	}

	expiredCount, err := m.store.Count(ctx, &LeaseFilter{State: LeaseStateExpired})
	if err != nil {
		return nil, err
	}

	revokedCount, err := m.store.Count(ctx, &LeaseFilter{State: LeaseStateRevoked})
	if err != nil {
		return nil, err
	}

	// Get backend counts
	leasesByBackend := make(map[BackendType]int)
	for _, bt := range []BackendType{BackendTypeVault, BackendTypeAWS, BackendTypeAzure, BackendTypeGCP} {
		count, err := m.store.Count(ctx, &LeaseFilter{
			Backend: bt,
			States:  []LeaseState{LeaseStateActive, LeaseStateRenewing},
		})
		if err == nil && count > 0 {
			leasesByBackend[bt] = int(count)
		}
	}

	m.stats.mu.RLock()
	totalRenewals := m.stats.TotalRenewals
	failedRenewals := m.stats.FailedRenewals
	m.stats.mu.RUnlock()

	return &LeaseStats{
		ActiveLeases:    int(activeCount),
		ExpiringLeases:  int(expiringCount),
		ExpiredLeases:   int(expiredCount),
		RevokedLeases:   int(revokedCount),
		TotalRenewals:   totalRenewals,
		FailedRenewals:  failedRenewals,
		LeasesByBackend: leasesByBackend,
	}, nil
}

// Start starts the lease renewal scheduler.
func (m *PersistentLeaseManager) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return nil
	}
	m.started = true
	m.mu.Unlock()

	m.wg.Add(2)
	go m.renewalLoop()
	go m.cleanupLoop()

	return nil
}

// Stop stops the lease renewal scheduler.
func (m *PersistentLeaseManager) Stop() error {
	m.cancel()
	m.wg.Wait()

	m.mu.Lock()
	m.started = false
	m.mu.Unlock()

	return nil
}

// renewalLoop periodically checks for leases needing renewal.
func (m *PersistentLeaseManager) renewalLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.RetryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.renewExpiring()
		}
	}
}

// cleanupLoop periodically cleans up expired and revoked leases.
func (m *PersistentLeaseManager) cleanupLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.cleanup()
		}
	}
}

// renewExpiring renews all leases that need renewal.
func (m *PersistentLeaseManager) renewExpiring() {
	threshold := m.config.Threshold
	if threshold == 0 {
		threshold = m.config.Strategy.Threshold()
	}

	// Calculate renewal window
	renewalWindow := time.Duration(float64(time.Hour) * (1 - threshold))

	// Get leases expiring within the window
	leases, err := m.store.List(m.ctx, &LeaseFilter{
		State:          LeaseStateActive,
		ExpiringBefore: time.Now().Add(renewalWindow),
		Limit:          m.renewalBatchSize,
	})
	if err != nil {
		return
	}

	// Filter for renewable leases
	var needsRenewal []*Lease
	for _, lease := range leases {
		if lease.Renewable && lease.NeedsRenewal(threshold) {
			needsRenewal = append(needsRenewal, lease)
		}
	}

	if len(needsRenewal) == 0 {
		return
	}

	// Renew with limited concurrency
	sem := make(chan struct{}, m.renewalConcurrency)
	var wg sync.WaitGroup

	for _, lease := range needsRenewal {
		wg.Add(1)
		sem <- struct{}{}

		go func(l *Lease) {
			defer wg.Done()
			defer func() { <-sem }()

			ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
			_, _ = m.Renew(ctx, l.ID, l.TTL)
			cancel()
		}(lease)
	}

	wg.Wait()
}

// cleanup removes expired and revoked leases.
func (m *PersistentLeaseManager) cleanup() {
	// First, mark expired leases
	_, _ = m.store.UpdateState(m.ctx, &LeaseFilter{
		State:          LeaseStateActive,
		ExpiringBefore: time.Now(),
	}, LeaseStateExpired)

	// Get expired and revoked leases for callback
	if m.callbacks != nil && m.callbacks.OnExpire != nil {
		expiredLeases, err := m.store.List(m.ctx, &LeaseFilter{
			State: LeaseStateExpired,
			Limit: 100,
		})
		if err == nil {
			for _, lease := range expiredLeases {
				m.callbacks.OnExpire(m.ctx, lease)
			}
		}
	}

	// Delete expired leases older than grace period
	gracePeriod := m.config.GracePeriod
	if gracePeriod == 0 {
		gracePeriod = time.Hour
	}

	deleted, _ := m.store.DeleteByFilter(m.ctx, &LeaseFilter{
		States:         []LeaseState{LeaseStateExpired, LeaseStateRevoked},
		ExpiringBefore: time.Now().Add(-gracePeriod),
	})

	if deleted > 0 {
		m.stats.mu.Lock()
		m.stats.TotalCleanups += deleted
		m.stats.mu.Unlock()
	}
}

// logLeaseTransitionEvent logs a lease event.
func (m *PersistentLeaseManager) logLeaseTransitionEvent(ctx context.Context, lease *Lease, action string, err error) {
	m.mu.RLock()
	logger := m.auditLogger
	m.mu.RUnlock()

	if logger == nil {
		return
	}

	event := &SecretAccessEvent{
		SecretPath: lease.SecretPath,
		Backend:    string(lease.Backend),
		LeaseID:    lease.ID,
		Action:     action,
		Timestamp:  time.Now(),
		Success:    err == nil,
	}

	if err != nil {
		event.Error = err.Error()
	}

	_ = logger.LogSecretAccess(ctx, event)
}

// BulkRenew renews multiple leases.
func (m *PersistentLeaseManager) BulkRenew(ctx context.Context, leaseIDs []string) (map[string]error, error) {
	results := make(map[string]error)

	sem := make(chan struct{}, m.renewalConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, leaseID := range leaseIDs {
		wg.Add(1)
		sem <- struct{}{}

		go func(id string) {
			defer wg.Done()
			defer func() { <-sem }()

			lease, err := m.store.Get(ctx, id)
			if err != nil {
				mu.Lock()
				results[id] = err
				mu.Unlock()
				return
			}

			_, err = m.Renew(ctx, id, lease.TTL)
			mu.Lock()
			results[id] = err
			mu.Unlock()
		}(leaseID)
	}

	wg.Wait()
	return results, nil
}

// BulkRevoke revokes multiple leases.
func (m *PersistentLeaseManager) BulkRevoke(ctx context.Context, leaseIDs []string) (map[string]error, error) {
	results := make(map[string]error)

	for _, leaseID := range leaseIDs {
		err := m.Revoke(ctx, leaseID)
		results[leaseID] = err
	}

	return results, nil
}

// GetLeaseHistory retrieves the history of a lease (if store supports it).
func (m *PersistentLeaseManager) GetLeaseHistory(ctx context.Context, leaseID string, limit int) ([]*LeaseEvent, error) {
	sqlStore, ok := m.store.(*SQLiteLeaseStore)
	if !ok {
		return nil, fmt.Errorf("lease history not supported by store")
	}

	return sqlStore.GetLeaseEvents(ctx, leaseID, limit)
}

// Ensure PersistentLeaseManager implements LeaseManager.
var _ LeaseManager = (*PersistentLeaseManager)(nil)
