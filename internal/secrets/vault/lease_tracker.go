// Package vault provides a HashiCorp Vault backend for the secrets broker.
package vault

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/internal/secrets"
)

// LeaseCallback is called when a lease event occurs.
type LeaseCallback func(ctx context.Context, lease *TrackedLease, event LeaseEvent)

// LeaseEvent represents a lease lifecycle event.
type LeaseEvent string

// LeaseEventTracked constants define the events.
const (
	LeaseEventTracked    LeaseEvent = "tracked"
	LeaseEventRenewed    LeaseEvent = "renewed"
	LeaseEventExpiring   LeaseEvent = "expiring"
	LeaseEventExpired    LeaseEvent = "expired"
	LeaseEventRevoked    LeaseEvent = "revoked"
	LeaseEventRenewError LeaseEvent = "renew_error"
)

// TrackedLease contains a lease with additional tracking metadata.
type TrackedLease struct {
	*secrets.Lease

	// Role is the Vault role that generated this credential.
	Role string `json:"role,omitempty"`

	// Engine is the secret engine type (kv, database, pki, etc.).
	Engine string `json:"engine,omitempty"`

	// MountPath is the engine mount path.
	MountPath string `json:"mount_path,omitempty"`

	// AgentID is the agent that requested this credential.
	AgentID string `json:"agent_id,omitempty"`

	// RequestID is the original request ID.
	RequestID string `json:"request_id,omitempty"`

	// Tags are user-defined tags for filtering.
	Tags map[string]string `json:"tags,omitempty"`

	// LastRenewError is the error from the last renewal attempt.
	LastRenewError string `json:"last_renew_error,omitempty"`

	// RenewAttempts is the number of consecutive failed renewal attempts.
	RenewAttempts int `json:"renew_attempts"`

	// MaxRenewAttempts is the maximum consecutive failed attempts before giving up.
	MaxRenewAttempts int `json:"max_renew_attempts"`
}

// LeaseTrackerConfig configures the lease tracker.
type LeaseTrackerConfig struct {
	// RenewalStrategy determines when to renew leases.
	RenewalStrategy secrets.RenewalStrategy `json:"renewal_strategy,omitempty"`

	// RenewalThreshold is the threshold for renewal (0-1).
	RenewalThreshold float64 `json:"renewal_threshold,omitempty"`

	// CheckInterval is how often to check for expiring leases.
	CheckInterval time.Duration `json:"check_interval,omitempty"`

	// RenewalTimeout is the timeout for renewal requests.
	RenewalTimeout time.Duration `json:"renewal_timeout,omitempty"`

	// MaxRenewAttempts is the max consecutive failed attempts.
	MaxRenewAttempts int `json:"max_renew_attempts,omitempty"`

	// GracePeriod is buffer time added to expiration warnings.
	GracePeriod time.Duration `json:"grace_period,omitempty"`

	// ExpiringWarningThreshold is when to start warning about expiring leases.
	ExpiringWarningThreshold time.Duration `json:"expiring_warning_threshold,omitempty"`

	// CleanupInterval is how often to clean up expired/revoked leases.
	CleanupInterval time.Duration `json:"cleanup_interval,omitempty"`

	// RetainExpired is how long to keep expired leases for audit.
	RetainExpired time.Duration `json:"retain_expired,omitempty"`
}

// DefaultLeaseTrackerConfig returns default configuration.
func DefaultLeaseTrackerConfig() *LeaseTrackerConfig {
	return &LeaseTrackerConfig{
		RenewalStrategy:          secrets.RenewalStrategyEager,
		RenewalThreshold:         0.5,
		CheckInterval:            30 * time.Second,
		RenewalTimeout:           30 * time.Second,
		MaxRenewAttempts:         3,
		GracePeriod:              30 * time.Second,
		ExpiringWarningThreshold: 5 * time.Minute,
		CleanupInterval:          5 * time.Minute,
		RetainExpired:            time.Hour,
	}
}

// LeaseTracker tracks and manages Vault leases with automatic renewal.
type LeaseTracker struct {
	mu sync.RWMutex

	client  *Client
	config  *LeaseTrackerConfig
	leases  map[string]*TrackedLease
	byPath  map[string][]string // path -> lease IDs
	byAgent map[string][]string // agentID -> lease IDs
	byTag   map[string][]string // tag:value -> lease IDs

	callbacks []LeaseCallback

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	started bool

	// Stats
	totalTracked   int64
	totalRenewals  int64
	totalRevoked   int64
	totalExpired   int64
	failedRenewals int64
}

// NewLeaseTracker creates a new lease tracker.
func NewLeaseTracker(client *Client, config *LeaseTrackerConfig) *LeaseTracker {
	if config == nil {
		config = DefaultLeaseTrackerConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &LeaseTracker{
		client:  client,
		config:  config,
		leases:  make(map[string]*TrackedLease),
		byPath:  make(map[string][]string),
		byAgent: make(map[string][]string),
		byTag:   make(map[string][]string),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// OnLeaseEvent registers a callback for lease events.
func (t *LeaseTracker) OnLeaseEvent(callback LeaseCallback) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.callbacks = append(t.callbacks, callback)
}

// Track starts tracking a lease.
func (t *LeaseTracker) Track(ctx context.Context, lease *secrets.Lease, opts ...TrackOption) error {
	if lease == nil || lease.ID == "" {
		return fmt.Errorf("lease ID is required")
	}

	tracked := &TrackedLease{
		Lease:            lease,
		MaxRenewAttempts: t.config.MaxRenewAttempts,
		Tags:             make(map[string]string),
	}

	for _, opt := range opts {
		opt(tracked)
	}

	t.mu.Lock()
	t.leases[lease.ID] = tracked
	t.totalTracked++

	// Index by path
	t.byPath[lease.SecretPath] = append(t.byPath[lease.SecretPath], lease.ID)

	// Index by agent
	if tracked.AgentID != "" {
		t.byAgent[tracked.AgentID] = append(t.byAgent[tracked.AgentID], lease.ID)
	}

	// Index by tags
	for k, v := range tracked.Tags {
		key := k + ":" + v
		t.byTag[key] = append(t.byTag[key], lease.ID)
	}
	t.mu.Unlock()

	t.notifyCallbacks(ctx, tracked, LeaseEventTracked)

	return nil
}

// TrackOption configures a tracked lease.
type TrackOption func(*TrackedLease)

// WithRole sets the role for a tracked lease.
func WithRole(role string) TrackOption {
	return func(tl *TrackedLease) {
		tl.Role = role
	}
}

// WithEngine sets the engine for a tracked lease.
func WithEngine(engine string) TrackOption {
	return func(tl *TrackedLease) {
		tl.Engine = engine
	}
}

// WithMountPath sets the mount path for a tracked lease.
func WithMountPath(path string) TrackOption {
	return func(tl *TrackedLease) {
		tl.MountPath = path
	}
}

// WithAgentID sets the agent ID for a tracked lease.
func WithAgentID(agentID string) TrackOption {
	return func(tl *TrackedLease) {
		tl.AgentID = agentID
	}
}

// WithRequestID sets the request ID for a tracked lease.
func WithRequestID(requestID string) TrackOption {
	return func(tl *TrackedLease) {
		tl.RequestID = requestID
	}
}

// WithTags sets tags for a tracked lease.
func WithTags(tags map[string]string) TrackOption {
	return func(tl *TrackedLease) {
		for k, v := range tags {
			tl.Tags[k] = v
		}
	}
}

// Get retrieves a tracked lease.
func (t *LeaseTracker) Get(ctx context.Context, leaseID string) (*TrackedLease, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	lease, exists := t.leases[leaseID]
	if !exists {
		return nil, secrets.ErrLeaseNotFound
	}

	return t.copyLease(lease), nil
}

// List lists all tracked leases.
func (t *LeaseTracker) List(ctx context.Context) ([]*TrackedLease, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	leases := make([]*TrackedLease, 0, len(t.leases))
	for _, lease := range t.leases {
		leases = append(leases, t.copyLease(lease))
	}

	// Sort by expiration
	sort.Slice(leases, func(i, j int) bool {
		return leases[i].ExpiresAt.Before(leases[j].ExpiresAt)
	})

	return leases, nil
}

// ListByPath lists leases for a path.
func (t *LeaseTracker) ListByPath(ctx context.Context, path string) ([]*TrackedLease, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	ids := t.byPath[path]
	leases := make([]*TrackedLease, 0, len(ids))
	for _, id := range ids {
		if lease, exists := t.leases[id]; exists {
			leases = append(leases, t.copyLease(lease))
		}
	}

	return leases, nil
}

// ListByPathPrefix lists leases matching a path prefix.
func (t *LeaseTracker) ListByPathPrefix(ctx context.Context, prefix string) ([]*TrackedLease, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var leases []*TrackedLease
	for _, lease := range t.leases {
		if strings.HasPrefix(lease.SecretPath, prefix) {
			leases = append(leases, t.copyLease(lease))
		}
	}

	return leases, nil
}

// ListByAgent lists leases for an agent.
func (t *LeaseTracker) ListByAgent(ctx context.Context, agentID string) ([]*TrackedLease, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	ids := t.byAgent[agentID]
	leases := make([]*TrackedLease, 0, len(ids))
	for _, id := range ids {
		if lease, exists := t.leases[id]; exists {
			leases = append(leases, t.copyLease(lease))
		}
	}

	return leases, nil
}

// ListByTag lists leases with a specific tag.
func (t *LeaseTracker) ListByTag(ctx context.Context, key, value string) ([]*TrackedLease, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	tagKey := key + ":" + value
	ids := t.byTag[tagKey]
	leases := make([]*TrackedLease, 0, len(ids))
	for _, id := range ids {
		if lease, exists := t.leases[id]; exists {
			leases = append(leases, t.copyLease(lease))
		}
	}

	return leases, nil
}

// ListExpiring lists leases expiring within a duration.
func (t *LeaseTracker) ListExpiring(ctx context.Context, within time.Duration) ([]*TrackedLease, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	deadline := time.Now().Add(within)
	var leases []*TrackedLease

	for _, lease := range t.leases {
		if lease.State == secrets.LeaseStateActive && !lease.ExpiresAt.IsZero() && lease.ExpiresAt.Before(deadline) {
			leases = append(leases, t.copyLease(lease))
		}
	}

	return leases, nil
}

// Renew renews a lease.
func (t *LeaseTracker) Renew(ctx context.Context, leaseID string, increment time.Duration) (*TrackedLease, error) {
	t.mu.Lock()
	lease, exists := t.leases[leaseID]
	if !exists {
		t.mu.Unlock()
		return nil, secrets.ErrLeaseNotFound
	}

	if !lease.Renewable {
		t.mu.Unlock()
		return nil, secrets.ErrLeaseNotRenewable
	}

	if lease.IsExpired() {
		lease.State = secrets.LeaseStateExpired
		t.mu.Unlock()
		t.notifyCallbacks(ctx, lease, LeaseEventExpired)
		return nil, secrets.ErrLeaseExpired
	}

	lease.State = secrets.LeaseStateRenewing
	t.mu.Unlock()

	// Renew at Vault
	data := map[string]interface{}{
		"lease_id":  leaseID,
		"increment": int(increment.Seconds()),
	}

	renewCtx, cancel := context.WithTimeout(ctx, t.config.RenewalTimeout)
	defer cancel()

	resp, err := t.client.Write(renewCtx, "sys/leases/renew", data)
	if err != nil {
		t.mu.Lock()
		lease.LastRenewError = err.Error()
		lease.RenewAttempts++
		lease.State = secrets.LeaseStateActive
		t.failedRenewals++

		if lease.RenewAttempts >= lease.MaxRenewAttempts {
			t.mu.Unlock()
			t.notifyCallbacks(ctx, lease, LeaseEventRenewError)
			return nil, fmt.Errorf("max renewal attempts exceeded: %w", err)
		}

		t.mu.Unlock()
		t.notifyCallbacks(ctx, lease, LeaseEventRenewError)
		return nil, fmt.Errorf("renewal failed: %w", err)
	}

	// Update lease
	t.mu.Lock()
	if leaseDuration, ok := resp["lease_duration"].(float64); ok {
		lease.TTL = time.Duration(leaseDuration) * time.Second
		lease.ExpiresAt = time.Now().Add(lease.TTL)
	}
	if renewable, ok := resp["renewable"].(bool); ok {
		lease.Renewable = renewable
	}

	lease.State = secrets.LeaseStateActive
	lease.LastRenewal = time.Now()
	lease.RenewalCount++
	lease.LastRenewError = ""
	lease.RenewAttempts = 0
	t.totalRenewals++
	t.mu.Unlock()

	t.notifyCallbacks(ctx, lease, LeaseEventRenewed)

	return t.copyLease(lease), nil
}

// Revoke revokes a lease.
func (t *LeaseTracker) Revoke(ctx context.Context, leaseID string) error {
	t.mu.Lock()
	lease, exists := t.leases[leaseID]
	if !exists {
		t.mu.Unlock()
		return secrets.ErrLeaseNotFound
	}
	t.mu.Unlock()

	// Revoke at Vault
	data := map[string]interface{}{
		"lease_id": leaseID,
	}

	_, err := t.client.Write(ctx, "sys/leases/revoke", data)
	if err != nil {
		return fmt.Errorf("revocation failed: %w", err)
	}

	t.mu.Lock()
	lease.State = secrets.LeaseStateRevoked
	t.totalRevoked++
	t.mu.Unlock()

	t.notifyCallbacks(ctx, lease, LeaseEventRevoked)

	return nil
}

// RevokeByPath revokes all leases for a path prefix.
func (t *LeaseTracker) RevokeByPath(ctx context.Context, pathPrefix string) (int, error) {
	leases, err := t.ListByPathPrefix(ctx, pathPrefix)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, lease := range leases {
		if lease.State == secrets.LeaseStateActive {
			if err := t.Revoke(ctx, lease.ID); err == nil {
				count++
			}
		}
	}

	return count, nil
}

// RevokeByAgent revokes all leases for an agent.
func (t *LeaseTracker) RevokeByAgent(ctx context.Context, agentID string) (int, error) {
	leases, err := t.ListByAgent(ctx, agentID)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, lease := range leases {
		if lease.State == secrets.LeaseStateActive {
			if err := t.Revoke(ctx, lease.ID); err == nil {
				count++
			}
		}
	}

	return count, nil
}

// RevokeByTag revokes all leases with a tag.
func (t *LeaseTracker) RevokeByTag(ctx context.Context, key, value string) (int, error) {
	leases, err := t.ListByTag(ctx, key, value)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, lease := range leases {
		if lease.State == secrets.LeaseStateActive {
			if err := t.Revoke(ctx, lease.ID); err == nil {
				count++
			}
		}
	}

	return count, nil
}

// Remove removes a lease from tracking without revoking.
func (t *LeaseTracker) Remove(ctx context.Context, leaseID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	lease, exists := t.leases[leaseID]
	if !exists {
		return nil
	}

	// Remove from indexes
	t.removeFromIndex(t.byPath, lease.SecretPath, leaseID)
	if lease.AgentID != "" {
		t.removeFromIndex(t.byAgent, lease.AgentID, leaseID)
	}
	for k, v := range lease.Tags {
		t.removeFromIndex(t.byTag, k+":"+v, leaseID)
	}

	delete(t.leases, leaseID)

	return nil
}

// removeFromIndex removes a lease ID from an index.
func (t *LeaseTracker) removeFromIndex(index map[string][]string, key, leaseID string) {
	ids := index[key]
	for i, id := range ids {
		if id == leaseID {
			index[key] = append(ids[:i], ids[i+1:]...)
			break
		}
	}
	if len(index[key]) == 0 {
		delete(index, key)
	}
}

// Stats returns tracker statistics.
func (t *LeaseTracker) Stats(ctx context.Context) *LeaseTrackerStats {
	t.mu.RLock()
	defer t.mu.RUnlock()

	stats := &LeaseTrackerStats{
		TotalTracked:   t.totalTracked,
		TotalRenewals:  t.totalRenewals,
		TotalRevoked:   t.totalRevoked,
		TotalExpired:   t.totalExpired,
		FailedRenewals: t.failedRenewals,
		ByEngine:       make(map[string]int),
		ByAgent:        make(map[string]int),
	}

	now := time.Now()
	expiringThreshold := now.Add(t.config.ExpiringWarningThreshold)

	for _, lease := range t.leases {
		switch lease.State {
		case secrets.LeaseStateActive:
			stats.ActiveLeases++
			if !lease.ExpiresAt.IsZero() && lease.ExpiresAt.Before(expiringThreshold) {
				stats.ExpiringLeases++
			}
		case secrets.LeaseStateExpired:
			stats.ExpiredLeases++
		case secrets.LeaseStateRevoked:
			stats.RevokedLeases++
		default:
		}

		if lease.Engine != "" {
			stats.ByEngine[lease.Engine]++
		}
		if lease.AgentID != "" {
			stats.ByAgent[lease.AgentID]++
		}
	}

	return stats
}

// LeaseTrackerStats contains tracker statistics.
type LeaseTrackerStats struct {
	TotalTracked   int64          `json:"total_tracked"`
	TotalRenewals  int64          `json:"total_renewals"`
	TotalRevoked   int64          `json:"total_revoked"`
	TotalExpired   int64          `json:"total_expired"`
	FailedRenewals int64          `json:"failed_renewals"`
	ActiveLeases   int            `json:"active_leases"`
	ExpiringLeases int            `json:"expiring_leases"`
	ExpiredLeases  int            `json:"expired_leases"`
	RevokedLeases  int            `json:"revoked_leases"`
	ByEngine       map[string]int `json:"by_engine"`
	ByAgent        map[string]int `json:"by_agent"`
}

// Start starts the automatic renewal background process.
func (t *LeaseTracker) Start(ctx context.Context) error {
	t.mu.Lock()
	if t.started {
		t.mu.Unlock()
		return nil
	}
	t.started = true
	t.mu.Unlock()

	t.wg.Add(2)
	go t.renewalLoop()  //nolint:contextcheck // background loop uses internal context
	go t.cleanupLoop()  //nolint:contextcheck // background loop uses internal context

	return nil
}

// Stop stops the tracker.
func (t *LeaseTracker) Stop() error {
	t.cancel()
	t.wg.Wait()

	t.mu.Lock()
	t.started = false
	t.mu.Unlock()

	return nil
}

// renewalLoop periodically checks for leases needing renewal.
func (t *LeaseTracker) renewalLoop() {
	defer t.wg.Done()

	ticker := time.NewTicker(t.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			t.renewExpiring()
		}
	}
}

// renewExpiring renews all leases that need renewal.
func (t *LeaseTracker) renewExpiring() {
	t.mu.RLock()
	var needsRenewal []*TrackedLease
	threshold := t.config.RenewalThreshold
	if threshold == 0 {
		threshold = t.config.RenewalStrategy.Threshold()
	}

	for _, lease := range t.leases {
		if lease.State == secrets.LeaseStateActive && lease.Renewable && lease.NeedsRenewal(threshold) {
			needsRenewal = append(needsRenewal, t.copyLease(lease))
		}
	}
	t.mu.RUnlock()

	// Renew each lease
	for _, lease := range needsRenewal {
		ctx, cancel := context.WithTimeout(t.ctx, t.config.RenewalTimeout)
		_, _ = t.Renew(ctx, lease.ID, lease.TTL)
		cancel()
	}

	// Check for expiring leases to warn about
	t.checkExpiring()
}

// checkExpiring checks for leases that are about to expire and notifies.
func (t *LeaseTracker) checkExpiring() {
	t.mu.RLock()
	var expiring []*TrackedLease
	deadline := time.Now().Add(t.config.ExpiringWarningThreshold)

	for _, lease := range t.leases {
		if lease.State == secrets.LeaseStateActive && !lease.ExpiresAt.IsZero() {
			if lease.ExpiresAt.Before(deadline) {
				expiring = append(expiring, t.copyLease(lease))
			}
		}
	}
	t.mu.RUnlock()

	for _, lease := range expiring {
		t.notifyCallbacks(t.ctx, lease, LeaseEventExpiring)
	}
}

// cleanupLoop periodically cleans up expired and revoked leases.
func (t *LeaseTracker) cleanupLoop() {
	defer t.wg.Done()

	ticker := time.NewTicker(t.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			t.cleanup()
		}
	}
}

// cleanup removes old expired and revoked leases.
func (t *LeaseTracker) cleanup() {
	t.mu.Lock()

	now := time.Now()
	retainUntil := now.Add(-t.config.RetainExpired)

	var toRemove []string
	var expired []*TrackedLease

	for id, lease := range t.leases {
		// Mark expired leases
		if lease.State == secrets.LeaseStateActive && lease.IsExpired() {
			lease.State = secrets.LeaseStateExpired
			t.totalExpired++
			expired = append(expired, t.copyLease(lease))
		}

		// Remove old expired/revoked leases
		if lease.State == secrets.LeaseStateExpired || lease.State == secrets.LeaseStateRevoked {
			if lease.ExpiresAt.Before(retainUntil) || (lease.State == secrets.LeaseStateRevoked && lease.LastRenewal.Before(retainUntil)) {
				toRemove = append(toRemove, id)
			}
		}
	}

	for _, id := range toRemove {
		lease := t.leases[id]
		t.removeFromIndex(t.byPath, lease.SecretPath, id)
		if lease.AgentID != "" {
			t.removeFromIndex(t.byAgent, lease.AgentID, id)
		}
		for k, v := range lease.Tags {
			t.removeFromIndex(t.byTag, k+":"+v, id)
		}
		delete(t.leases, id)
	}
	t.mu.Unlock()

	// Notify about expired leases outside the lock
	for _, lease := range expired {
		t.notifyCallbacks(context.Background(), lease, LeaseEventExpired)
	}
}

// notifyCallbacks notifies all registered callbacks.
func (t *LeaseTracker) notifyCallbacks(ctx context.Context, lease *TrackedLease, event LeaseEvent) {
	t.mu.RLock()
	callbacks := make([]LeaseCallback, len(t.callbacks))
	copy(callbacks, t.callbacks)
	t.mu.RUnlock()

	for _, cb := range callbacks {
		cb(ctx, lease, event)
	}
}

// copyLease creates a copy of a tracked lease.
func (t *LeaseTracker) copyLease(lease *TrackedLease) *TrackedLease {
	if lease == nil {
		return nil
	}

	leaseCopy := *lease.Lease
	copied := &TrackedLease{
		Lease:            &leaseCopy,
		Role:             lease.Role,
		Engine:           lease.Engine,
		MountPath:        lease.MountPath,
		AgentID:          lease.AgentID,
		RequestID:        lease.RequestID,
		Tags:             make(map[string]string),
		LastRenewError:   lease.LastRenewError,
		RenewAttempts:    lease.RenewAttempts,
		MaxRenewAttempts: lease.MaxRenewAttempts,
	}

	for k, v := range lease.Tags {
		copied.Tags[k] = v
	}

	return copied
}
