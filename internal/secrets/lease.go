package secrets

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// InMemoryLeaseManager provides an in-memory implementation of LeaseManager.
type InMemoryLeaseManager struct {
	mu sync.RWMutex

	// leases stores leases by ID.
	leases map[string]*Lease

	// config is the renewal configuration.
	config *LeaseRenewalConfig

	// backends maps backend types to their implementations for renewal.
	backends map[BackendType]SecretBackend

	// auditLogger logs lease events.
	auditLogger SecretAuditLogger

	// stats tracks lease statistics.
	totalRenewals  int64
	failedRenewals int64

	// renewal goroutine management
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// started indicates if the renewal scheduler is running.
	started bool
}

// NewInMemoryLeaseManager creates a new in-memory lease manager.
func NewInMemoryLeaseManager(config *LeaseRenewalConfig) *InMemoryLeaseManager {
	if config == nil {
		config = DefaultLeaseRenewalConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &InMemoryLeaseManager{
		leases:   make(map[string]*Lease),
		config:   config,
		backends: make(map[BackendType]SecretBackend),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// RegisterBackend registers a backend for lease renewal.
func (m *InMemoryLeaseManager) RegisterBackend(backendType BackendType, backend SecretBackend) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.backends[backendType] = backend
}

// SetAuditLogger sets the audit logger.
func (m *InMemoryLeaseManager) SetAuditLogger(logger SecretAuditLogger) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.auditLogger = logger
}

// Track starts tracking a lease.
func (m *InMemoryLeaseManager) Track(ctx context.Context, lease *Lease) error {
	if lease == nil || lease.ID == "" {
		return fmt.Errorf("lease ID is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Make a copy to avoid external mutations
	leaseCopy := *lease
	leaseCopy.State = LeaseStateActive
	m.leases[lease.ID] = &leaseCopy

	return nil
}

// Get retrieves a tracked lease by ID.
func (m *InMemoryLeaseManager) Get(ctx context.Context, leaseID string) (*Lease, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	lease, exists := m.leases[leaseID]
	if !exists {
		return nil, ErrLeaseNotFound
	}

	// Return a copy
	leaseCopy := *lease
	return &leaseCopy, nil
}

// List lists all tracked leases.
func (m *InMemoryLeaseManager) List(ctx context.Context) ([]*Lease, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	leases := make([]*Lease, 0, len(m.leases))
	for _, lease := range m.leases {
		leaseCopy := *lease
		leases = append(leases, &leaseCopy)
	}

	return leases, nil
}

// ListByPath lists leases for a specific secret path.
func (m *InMemoryLeaseManager) ListByPath(ctx context.Context, path string) ([]*Lease, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var leases []*Lease
	for _, lease := range m.leases {
		if lease.SecretPath == path {
			leaseCopy := *lease
			leases = append(leases, &leaseCopy)
		}
	}

	return leases, nil
}

// ListByBackend lists leases for a specific backend.
func (m *InMemoryLeaseManager) ListByBackend(ctx context.Context, backend BackendType) ([]*Lease, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var leases []*Lease
	for _, lease := range m.leases {
		if lease.Backend == backend {
			leaseCopy := *lease
			leases = append(leases, &leaseCopy)
		}
	}

	return leases, nil
}

// ListExpiring lists leases expiring within the given duration.
func (m *InMemoryLeaseManager) ListExpiring(ctx context.Context, within time.Duration) ([]*Lease, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	deadline := time.Now().Add(within)
	var leases []*Lease

	for _, lease := range m.leases {
		if lease.State == LeaseStateActive && !lease.ExpiresAt.IsZero() && lease.ExpiresAt.Before(deadline) {
			leaseCopy := *lease
			leases = append(leases, &leaseCopy)
		}
	}

	return leases, nil
}

// Renew renews a lease.
func (m *InMemoryLeaseManager) Renew(ctx context.Context, leaseID string, increment time.Duration) (*Lease, error) {
	m.mu.Lock()
	lease, exists := m.leases[leaseID]
	if !exists {
		m.mu.Unlock()
		return nil, ErrLeaseNotFound
	}

	if !lease.Renewable {
		m.mu.Unlock()
		return nil, ErrLeaseNotRenewable
	}

	if lease.IsExpired() {
		lease.State = LeaseStateExpired
		m.mu.Unlock()
		return nil, ErrLeaseExpired
	}

	// Get backend
	backend, backendExists := m.backends[lease.Backend]
	if !backendExists {
		m.mu.Unlock()
		return nil, fmt.Errorf("%w: %s", ErrBackendNotFound, lease.Backend)
	}

	// Mark as renewing
	lease.State = LeaseStateRenewing
	m.mu.Unlock()

	// Renew at backend (outside lock)
	renewedLease, err := backend.RenewLease(ctx, leaseID, increment)
	if err != nil {
		m.mu.Lock()
		m.failedRenewals++
		if l, ok := m.leases[leaseID]; ok {
			l.State = LeaseStateActive // Revert state
		}
		m.mu.Unlock()

		m.logLeaseEvent(ctx, lease, AuditActionLeaseRenewFailed, err)
		return nil, err
	}

	// Update tracked lease
	m.mu.Lock()
	if l, ok := m.leases[leaseID]; ok {
		l.State = LeaseStateActive
		l.TTL = renewedLease.TTL
		l.ExpiresAt = renewedLease.ExpiresAt
		l.LastRenewal = time.Now()
		l.RenewalCount++
		m.totalRenewals++
		renewedLease = l
	}
	m.mu.Unlock()

	m.logLeaseEvent(ctx, renewedLease, AuditActionLeaseRenew, nil)
	return renewedLease, nil
}

// Revoke revokes a lease.
func (m *InMemoryLeaseManager) Revoke(ctx context.Context, leaseID string) error {
	m.mu.Lock()
	lease, exists := m.leases[leaseID]
	if !exists {
		m.mu.Unlock()
		return ErrLeaseNotFound
	}

	lease.State = LeaseStateRevoked
	m.mu.Unlock()

	m.logLeaseEvent(ctx, lease, AuditActionLeaseRevoke, nil)
	return nil
}

// RevokeByPath revokes all leases for a path prefix.
func (m *InMemoryLeaseManager) RevokeByPath(ctx context.Context, pathPrefix string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	for _, lease := range m.leases {
		if strings.HasPrefix(lease.SecretPath, pathPrefix) && lease.State == LeaseStateActive {
			lease.State = LeaseStateRevoked
			count++
		}
	}

	return count, nil
}

// Remove removes a lease from tracking.
func (m *InMemoryLeaseManager) Remove(ctx context.Context, leaseID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.leases, leaseID)
	return nil
}

// Stats returns lease manager statistics.
func (m *InMemoryLeaseManager) Stats(ctx context.Context) (*LeaseStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := &LeaseStats{
		LeasesByBackend: make(map[BackendType]int),
		TotalRenewals:   m.totalRenewals,
		FailedRenewals:  m.failedRenewals,
	}

	now := time.Now()
	expiringThreshold := now.Add(time.Hour)

	for _, lease := range m.leases {
		switch lease.State {
		case LeaseStateActive:
			stats.ActiveLeases++
			if !lease.ExpiresAt.IsZero() && lease.ExpiresAt.Before(expiringThreshold) {
				stats.ExpiringLeases++
			}
		case LeaseStateExpired:
			stats.ExpiredLeases++
		case LeaseStateRevoked:
			stats.RevokedLeases++
		}

		stats.LeasesByBackend[lease.Backend]++
	}

	return stats, nil
}

// Start starts the lease renewal scheduler.
func (m *InMemoryLeaseManager) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return nil
	}
	m.started = true
	m.mu.Unlock()

	m.wg.Add(1)
	go m.renewalLoop()

	return nil
}

// Stop stops the lease renewal scheduler.
func (m *InMemoryLeaseManager) Stop() error {
	m.cancel()
	m.wg.Wait()

	m.mu.Lock()
	m.started = false
	m.mu.Unlock()

	return nil
}

// renewalLoop periodically checks for leases needing renewal.
func (m *InMemoryLeaseManager) renewalLoop() {
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

// renewExpiring renews all leases that need renewal.
func (m *InMemoryLeaseManager) renewExpiring() {
	m.mu.RLock()
	var needsRenewal []*Lease
	threshold := m.config.Threshold
	if threshold == 0 {
		threshold = m.config.Strategy.Threshold()
	}

	for _, lease := range m.leases {
		if lease.State == LeaseStateActive && lease.Renewable && lease.NeedsRenewal(threshold) {
			leaseCopy := *lease
			needsRenewal = append(needsRenewal, &leaseCopy)
		}
	}
	m.mu.RUnlock()

	// Renew each lease (outside lock)
	for _, lease := range needsRenewal {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		_, _ = m.Renew(ctx, lease.ID, lease.TTL)
		cancel()
	}
}

// cleanupExpired removes expired and revoked leases.
func (m *InMemoryLeaseManager) cleanupExpired() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, lease := range m.leases {
		if lease.State == LeaseStateExpired || lease.State == LeaseStateRevoked {
			delete(m.leases, id)
		} else if lease.IsExpired() {
			lease.State = LeaseStateExpired
			m.logLeaseEvent(context.Background(), lease, AuditActionLeaseExpire, nil)
		}
	}
}

// logLeaseEvent logs a lease event.
func (m *InMemoryLeaseManager) logLeaseEvent(ctx context.Context, lease *Lease, action string, err error) {
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

// Ensure InMemoryLeaseManager implements LeaseManager.
var _ LeaseManager = (*InMemoryLeaseManager)(nil)
