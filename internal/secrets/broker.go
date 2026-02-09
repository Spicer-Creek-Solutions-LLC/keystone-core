package secrets

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// SecretBroker provides a unified interface for accessing secrets from multiple backends.
// It handles path-based routing, caching, lease management, and audit logging.
type SecretBroker struct {
	mu sync.RWMutex

	// config is the broker configuration.
	config *BrokerConfig

	// backends maps backend names to their implementations.
	backends map[string]SecretBackend

	// defaultBackend is the default backend for unqualified paths.
	defaultBackend string

	// routingRules are the path-based routing rules, sorted by prefix length (longest first).
	routingRules []RoutingRule

	// cache is the secret cache.
	cache SecretCache

	// leaseManager manages dynamic secret leases.
	leaseManager LeaseManager

	// auditLogger logs secret access events.
	auditLogger SecretAuditLogger

	// closed indicates if the broker has been closed.
	closed bool
}

// NewSecretBroker creates a new secret broker with the given configuration.
func NewSecretBroker(config *BrokerConfig) *SecretBroker {
	if config == nil {
		config = &BrokerConfig{}
	}

	b := &SecretBroker{
		config:         config,
		backends:       make(map[string]SecretBackend),
		defaultBackend: config.DefaultBackend,
		routingRules:   config.Routing,
	}

	// Sort routing rules by prefix length (longest first) for most specific match
	b.sortRoutingRules()

	return b
}

// sortRoutingRules sorts routing rules by prefix length in descending order.
func (b *SecretBroker) sortRoutingRules() {
	sort.Slice(b.routingRules, func(i, j int) bool {
		return len(b.routingRules[i].Prefix) > len(b.routingRules[j].Prefix)
	})
}

// RegisterBackend registers a secret backend.
func (b *SecretBroker) RegisterBackend(name string, backend SecretBackend) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return fmt.Errorf("broker is closed")
	}

	if name == "" {
		return fmt.Errorf("backend name is required")
	}

	if backend == nil {
		return fmt.Errorf("backend is required")
	}

	b.backends[name] = backend

	// Set as default if no default is configured and this is the first backend
	if b.defaultBackend == "" && len(b.backends) == 1 {
		b.defaultBackend = name
	}

	return nil
}

// UnregisterBackend removes a secret backend.
func (b *SecretBroker) UnregisterBackend(name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, exists := b.backends[name]; !exists {
		return ErrBackendNotFound
	}

	delete(b.backends, name)

	// Clear default if we just removed it
	if b.defaultBackend == name {
		b.defaultBackend = ""
		// Set a new default if other backends exist
		for backendName := range b.backends {
			b.defaultBackend = backendName
			break
		}
	}

	return nil
}

// SetCache sets the secret cache.
func (b *SecretBroker) SetCache(cache SecretCache) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cache = cache
}

// SetLeaseManager sets the lease manager.
func (b *SecretBroker) SetLeaseManager(lm LeaseManager) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.leaseManager = lm
}

// SetAuditLogger sets the audit logger.
func (b *SecretBroker) SetAuditLogger(logger SecretAuditLogger) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.auditLogger = logger
}

// AddRoutingRule adds a routing rule.
func (b *SecretBroker) AddRoutingRule(rule RoutingRule) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.routingRules = append(b.routingRules, rule)
	b.sortRoutingRules()
}

// GetBackend returns a backend by name.
func (b *SecretBroker) GetBackend(name string) (SecretBackend, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	backend, exists := b.backends[name]
	if !exists {
		return nil, ErrBackendNotFound
	}

	return backend, nil
}

// ListBackends returns the names of all registered backends.
func (b *SecretBroker) ListBackends() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	names := make([]string, 0, len(b.backends))
	for name := range b.backends {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// resolveBackend determines which backend should handle a request based on the path.
func (b *SecretBroker) resolveBackend(path string) (SecretBackend, string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Check routing rules first (already sorted by prefix length)
	for _, rule := range b.routingRules {
		if rule.Prefix == "" || !strings.HasPrefix(path, rule.Prefix) {
			continue
		}
		backend, exists := b.backends[rule.Backend]
		if !exists {
			return nil, "", fmt.Errorf("%w: %s", ErrBackendNotFound, rule.Backend)
		}
		// Strip the prefix from the path for the backend
		strippedPath := strings.TrimPrefix(path, rule.Prefix)
		strippedPath = strings.TrimPrefix(strippedPath, "/")
		return backend, strippedPath, nil
	}

	// Try to extract backend from path (e.g., "vault/kv/myapp" -> backend="vault", path="kv/myapp")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) >= 1 {
		backendName := parts[0]
		if backend, exists := b.backends[backendName]; exists {
			remainingPath := ""
			if len(parts) > 1 {
				remainingPath = parts[1]
			}
			return backend, remainingPath, nil
		}
	}

	// Fall back to default backend
	if b.defaultBackend != "" {
		backend, exists := b.backends[b.defaultBackend]
		if exists {
			return backend, path, nil
		}
	}

	return nil, "", ErrBackendNotFound
}

// Read reads a secret from the appropriate backend.
func (b *SecretBroker) Read(ctx context.Context, req *SecretRequest) (*Secret, error) {
	if req == nil || req.Path == "" {
		return nil, ErrInvalidPath
	}

	startTime := time.Now()
	var secret *Secret
	var err error
	var cacheHit bool

	defer func() {
		b.logAccess(ctx, req, secret, cacheHit, err, startTime)
	}()

	// Check cache first
	if b.cache != nil {
		if cached, ok := b.cache.Get(ctx, req.Path); ok {
			cacheHit = true
			return cached, nil
		}
	}

	// Resolve backend
	backend, backendPath, err := b.resolveBackend(req.Path)
	if err != nil {
		return nil, err
	}

	// Check backend health
	if !backend.Healthy(ctx) {
		return nil, fmt.Errorf("%w: %s", ErrBackendUnavailable, backend.Name())
	}

	// Create request for backend with resolved path
	backendReq := &SecretRequest{
		Path:      backendPath,
		Keys:      req.Keys,
		Version:   req.Version,
		TTL:       req.TTL,
		Renewable: req.Renewable,
		AgentID:   req.AgentID,
		SPIFFEID:  req.SPIFFEID,
		RequestID: req.RequestID,
		Context:   req.Context,
	}

	// Read from backend
	secret, err = backend.Read(ctx, backendReq)
	if err != nil {
		return nil, err
	}

	// Set full path and backend on returned secret
	secret.Path = req.Path
	secret.Backend = backend.Type()

	// Cache the result
	if b.cache != nil && secret != nil {
		ttl := b.config.Cache.DefaultTTL
		if ttl == 0 {
			ttl = 5 * time.Minute
		}
		// Don't cache if secret has a shorter TTL
		if !secret.ExpiresAt.IsZero() {
			secretTTL := time.Until(secret.ExpiresAt)
			if secretTTL > 0 && secretTTL < ttl {
				ttl = secretTTL
			}
		}
		_ = b.cache.Put(ctx, secret, ttl)
	}

	return secret, nil
}

// ReadDynamic reads or generates a dynamic secret.
func (b *SecretBroker) ReadDynamic(ctx context.Context, req *SecretRequest) (*Secret, error) {
	if req == nil || req.Path == "" {
		return nil, ErrInvalidPath
	}

	startTime := time.Now()
	var secret *Secret
	var err error

	defer func() {
		b.logAccess(ctx, req, secret, false, err, startTime)
	}()

	// Resolve backend
	backend, backendPath, err := b.resolveBackend(req.Path)
	if err != nil {
		return nil, err
	}

	// Check backend health
	if !backend.Healthy(ctx) {
		return nil, fmt.Errorf("%w: %s", ErrBackendUnavailable, backend.Name())
	}

	// Create request for backend with resolved path
	backendReq := &SecretRequest{
		Path:      backendPath,
		Keys:      req.Keys,
		TTL:       req.TTL,
		Renewable: req.Renewable,
		AgentID:   req.AgentID,
		SPIFFEID:  req.SPIFFEID,
		RequestID: req.RequestID,
		Context:   req.Context,
	}

	// Read dynamic secret from backend
	secret, err = backend.ReadDynamic(ctx, backendReq)
	if err != nil {
		return nil, err
	}

	// Set full path and backend on returned secret
	secret.Path = req.Path
	secret.Backend = backend.Type()

	// Track lease if present
	if secret.HasLease() && b.leaseManager != nil {
		secret.Lease.SecretPath = req.Path
		secret.Lease.Backend = backend.Type()
		_ = b.leaseManager.Track(ctx, secret.Lease) // best-effort tracking, secret was still retrieved
	}

	// Cache dynamic secrets with shorter TTL based on lease
	if b.cache != nil && secret != nil {
		ttl := time.Minute // Shorter default for dynamic secrets
		if secret.Lease != nil && secret.Lease.TTL > 0 {
			// Cache for at most half the lease TTL
			ttl = secret.Lease.TTL / 2
			if ttl < 30*time.Second {
				ttl = 30 * time.Second
			}
		}
		_ = b.cache.Put(ctx, secret, ttl)
	}

	return secret, nil
}

// List lists secrets under a path prefix.
func (b *SecretBroker) List(ctx context.Context, path string) ([]string, error) {
	// Resolve backend
	backend, backendPath, err := b.resolveBackend(path)
	if err != nil {
		return nil, err
	}

	// Check backend health
	if !backend.Healthy(ctx) {
		return nil, fmt.Errorf("%w: %s", ErrBackendUnavailable, backend.Name())
	}

	return backend.List(ctx, backendPath)
}

// RenewLease renews a lease.
func (b *SecretBroker) RenewLease(ctx context.Context, leaseID string, increment time.Duration) (*Lease, error) {
	if b.leaseManager == nil {
		return nil, fmt.Errorf("lease manager not configured")
	}

	return b.leaseManager.Renew(ctx, leaseID, increment)
}

// RevokeLease revokes a lease.
func (b *SecretBroker) RevokeLease(ctx context.Context, leaseID string) error {
	if b.leaseManager == nil {
		return fmt.Errorf("lease manager not configured")
	}

	// Get lease to find which backend to revoke from
	lease, err := b.leaseManager.Get(ctx, leaseID)
	if err != nil {
		return err
	}

	// Find backend by type
	var backend SecretBackend
	b.mu.RLock()
	for _, be := range b.backends {
		if be.Type() == lease.Backend {
			backend = be
			break
		}
	}
	b.mu.RUnlock()

	if backend == nil {
		return fmt.Errorf("%w: %s", ErrBackendNotFound, lease.Backend)
	}

	// Revoke at backend
	if err := backend.RevokeLease(ctx, leaseID); err != nil {
		return err
	}

	// Remove from tracking
	return b.leaseManager.Revoke(ctx, leaseID)
}

// Invalidate removes a secret from the cache.
func (b *SecretBroker) Invalidate(ctx context.Context, path string) error {
	if b.cache == nil {
		return nil
	}
	return b.cache.Delete(ctx, path)
}

// InvalidatePrefix removes all secrets matching a prefix from the cache.
func (b *SecretBroker) InvalidatePrefix(ctx context.Context, prefix string) (int, error) {
	if b.cache == nil {
		return 0, nil
	}
	return b.cache.DeleteByPrefix(ctx, prefix)
}

// Healthy returns true if the broker and its backends are healthy.
func (b *SecretBroker) Healthy(ctx context.Context) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return false
	}

	// Check at least one backend is healthy
	for _, backend := range b.backends {
		if backend.Healthy(ctx) {
			return true
		}
	}

	return len(b.backends) == 0 // Healthy if no backends configured (just not useful)
}

// BackendHealth returns the health status of each backend.
func (b *SecretBroker) BackendHealth(ctx context.Context) map[string]bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	health := make(map[string]bool)
	for name, backend := range b.backends {
		health[name] = backend.Healthy(ctx)
	}
	return health
}

// Stats returns broker statistics.
func (b *SecretBroker) Stats(ctx context.Context) *BrokerStats {
	b.mu.RLock()
	defer b.mu.RUnlock()

	stats := &BrokerStats{
		BackendCount:  len(b.backends),
		BackendHealth: make(map[string]bool),
	}

	for name, backend := range b.backends {
		stats.BackendHealth[name] = backend.Healthy(ctx)
	}

	if b.cache != nil {
		stats.CacheStats = b.cache.Stats()
	}

	if b.leaseManager != nil {
		leaseStats, err := b.leaseManager.Stats(ctx)
		if err == nil {
			stats.LeaseStats = leaseStats
		}
	}

	return stats
}

// BrokerStats contains broker statistics.
type BrokerStats struct {
	// BackendCount is the number of registered backends.
	BackendCount int `json:"backend_count"`

	// BackendHealth is the health status of each backend.
	BackendHealth map[string]bool `json:"backend_health"`

	// CacheStats contains cache statistics.
	CacheStats *CacheStats `json:"cache_stats,omitempty"`

	// LeaseStats contains lease manager statistics.
	LeaseStats *LeaseStats `json:"lease_stats,omitempty"`
}

// Close closes the broker and all backends.
func (b *SecretBroker) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}
	b.closed = true

	var lastErr error

	// Close all backends
	for name, backend := range b.backends {
		if err := backend.Close(); err != nil {
			lastErr = fmt.Errorf("failed to close backend %s: %w", name, err)
		}
	}

	// Close cache
	if b.cache != nil {
		if err := b.cache.Close(); err != nil {
			lastErr = fmt.Errorf("failed to close cache: %w", err)
		}
	}

	// Stop lease manager
	if b.leaseManager != nil {
		if err := b.leaseManager.Stop(); err != nil {
			lastErr = fmt.Errorf("failed to stop lease manager: %w", err)
		}
	}

	return lastErr
}

// logAccess logs a secret access event.
func (b *SecretBroker) logAccess(ctx context.Context, req *SecretRequest, secret *Secret, cacheHit bool, err error, startTime time.Time) {
	if b.auditLogger == nil {
		return
	}

	event := &SecretAccessEvent{
		SecretPath: req.Path,
		AgentID:    req.AgentID,
		SPIFFEID:   req.SPIFFEID,
		RequestID:  req.RequestID,
		Timestamp:  startTime,
		Duration:   time.Since(startTime),
		CacheHit:   cacheHit,
		Success:    err == nil,
	}

	if secret != nil {
		event.Backend = string(secret.Backend)
		event.SecretType = string(secret.Type)
		if secret.Lease != nil {
			event.LeaseID = secret.Lease.ID
		}
	}

	switch {
	case err != nil:
		event.Error = err.Error()
		event.Action = AuditActionReadFailed
	case cacheHit:
		event.Action = AuditActionCacheHit
	default:
		event.Action = AuditActionRead
	}

	_ = b.auditLogger.LogSecretAccess(ctx, event)
}
