// Package credentials provides secure credential management for proxy agents.
package credentials

import (
	"context"
	"sync"
	"time"
)

// CacheConfig configures the credential cache.
type CacheConfig struct {
	// DefaultTTL is the default time-to-live for cached credentials.
	DefaultTTL time.Duration
	// MaxEntries is the maximum number of cached entries.
	MaxEntries int
	// CleanupInterval is how often to run cache cleanup.
	CleanupInterval time.Duration
}

// DefaultCacheConfig returns a cache configuration with sensible defaults.
func DefaultCacheConfig() *CacheConfig {
	return &CacheConfig{
		DefaultTTL:      5 * time.Minute,
		MaxEntries:      1000,
		CleanupInterval: time.Minute,
	}
}

// cacheEntry represents a cached credential.
type cacheEntry struct {
	credential Credential
	expiresAt  time.Time
	accessTime time.Time
	accessCount int64
}

// isExpired returns true if the cache entry has expired.
func (e *cacheEntry) isExpired() bool {
	return time.Now().After(e.expiresAt)
}

// CredentialCache provides TTL-based caching for credentials.
type CredentialCache struct {
	mu       sync.RWMutex
	config   *CacheConfig
	entries  map[string]*cacheEntry
	backend  CredentialStore

	// For cleanup goroutine
	ctx    context.Context
	cancel context.CancelFunc
}

// NewCredentialCache creates a new credential cache.
func NewCredentialCache(config *CacheConfig, backend CredentialStore) *CredentialCache {
	if config == nil {
		config = DefaultCacheConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	cache := &CredentialCache{
		config:  config,
		entries: make(map[string]*cacheEntry),
		backend: backend,
		ctx:     ctx,
		cancel:  cancel,
	}

	// Start cleanup goroutine
	go cache.cleanupLoop()

	return cache
}

// Get retrieves a credential from the cache or backend.
func (c *CredentialCache) Get(ctx context.Context, id string) (Credential, error) {
	// Try cache first
	c.mu.RLock()
	entry, exists := c.entries[id]
	if exists && !entry.isExpired() {
		entry.accessTime = time.Now()
		entry.accessCount++
		cred := entry.credential
		c.mu.RUnlock()
		return cred, nil
	}
	c.mu.RUnlock()

	// Fetch from backend
	if c.backend == nil {
		if exists {
			// Entry exists but expired
			c.mu.Lock()
			delete(c.entries, id)
			c.mu.Unlock()
		}
		return nil, ErrCredentialNotFound
	}

	cred, err := c.backend.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	// Cache the result
	c.cacheCredential(id, cred, c.config.DefaultTTL)

	return cred, nil
}

// GetWithTTL retrieves a credential with a custom TTL for caching.
func (c *CredentialCache) GetWithTTL(ctx context.Context, id string, ttl time.Duration) (Credential, error) {
	// Try cache first
	c.mu.RLock()
	entry, exists := c.entries[id]
	if exists && !entry.isExpired() {
		entry.accessTime = time.Now()
		entry.accessCount++
		cred := entry.credential
		c.mu.RUnlock()
		return cred, nil
	}
	c.mu.RUnlock()

	// Fetch from backend
	if c.backend == nil {
		return nil, ErrCredentialNotFound
	}

	cred, err := c.backend.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	// Cache with custom TTL
	c.cacheCredential(id, cred, ttl)

	return cred, nil
}

// cacheCredential adds a credential to the cache.
func (c *CredentialCache) cacheCredential(id string, cred Credential, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Enforce max entries (LRU eviction)
	if len(c.entries) >= c.config.MaxEntries {
		c.evictOldest()
	}

	c.entries[id] = &cacheEntry{
		credential:  cred,
		expiresAt:   time.Now().Add(ttl),
		accessTime:  time.Now(),
		accessCount: 1,
	}
}

// Put adds or updates a credential in the cache and backend.
func (c *CredentialCache) Put(ctx context.Context, cred Credential) error {
	// Store in backend first
	if c.backend != nil {
		if err := c.backend.Store(ctx, cred); err != nil {
			return err
		}
	}

	// Update cache
	c.cacheCredential(cred.ID(), cred, c.config.DefaultTTL)

	return nil
}

// Invalidate removes a credential from the cache.
func (c *CredentialCache) Invalidate(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, id)
}

// InvalidateAll clears the entire cache.
func (c *CredentialCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*cacheEntry)
}

// evictOldest removes the oldest entry from the cache.
// Must be called with lock held.
func (c *CredentialCache) evictOldest() {
	var oldestID string
	var oldestTime time.Time

	for id, entry := range c.entries {
		if oldestID == "" || entry.accessTime.Before(oldestTime) {
			oldestID = id
			oldestTime = entry.accessTime
		}
	}

	if oldestID != "" {
		delete(c.entries, oldestID)
	}
}

// cleanupLoop periodically removes expired entries.
func (c *CredentialCache) cleanupLoop() {
	ticker := time.NewTicker(c.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.cleanup()
		}
	}
}

// cleanup removes expired entries from the cache.
func (c *CredentialCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for id, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, id)
		}
	}
}

// Close stops the cleanup goroutine.
func (c *CredentialCache) Close() error {
	c.cancel()
	return nil
}

// Stats returns cache statistics.
func (c *CredentialCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var totalAccesses int64
	var expiredCount int
	now := time.Now()

	for _, entry := range c.entries {
		totalAccesses += entry.accessCount
		if now.After(entry.expiresAt) {
			expiredCount++
		}
	}

	return CacheStats{
		Entries:       len(c.entries),
		ExpiredCount:  expiredCount,
		TotalAccesses: totalAccesses,
		MaxEntries:    c.config.MaxEntries,
	}
}

// CacheStats contains cache statistics.
type CacheStats struct {
	// Entries is the current number of cached entries.
	Entries int
	// ExpiredCount is the number of expired entries pending cleanup.
	ExpiredCount int
	// TotalAccesses is the total number of cache accesses.
	TotalAccesses int64
	// MaxEntries is the maximum allowed entries.
	MaxEntries int
}

// CachedCredentialStore wraps a CredentialStore with caching.
type CachedCredentialStore struct {
	cache   *CredentialCache
	backend CredentialStore
}

// NewCachedCredentialStore creates a new cached credential store.
func NewCachedCredentialStore(config *CacheConfig, backend CredentialStore) *CachedCredentialStore {
	return &CachedCredentialStore{
		cache:   NewCredentialCache(config, backend),
		backend: backend,
	}
}

// Get retrieves a credential, using cache when available.
func (s *CachedCredentialStore) Get(ctx context.Context, id string) (Credential, error) {
	return s.cache.Get(ctx, id)
}

// Store stores a credential and updates the cache.
func (s *CachedCredentialStore) Store(ctx context.Context, cred Credential) error {
	if err := s.backend.Store(ctx, cred); err != nil {
		return err
	}
	s.cache.cacheCredential(cred.ID(), cred, s.cache.config.DefaultTTL)
	return nil
}

// Delete deletes a credential and invalidates the cache.
func (s *CachedCredentialStore) Delete(ctx context.Context, id string) error {
	s.cache.Invalidate(id)
	return s.backend.Delete(ctx, id)
}

// List lists all credential IDs.
func (s *CachedCredentialStore) List(ctx context.Context) ([]string, error) {
	return s.backend.List(ctx)
}

// Exists checks if a credential exists.
func (s *CachedCredentialStore) Exists(ctx context.Context, id string) (bool, error) {
	return s.backend.Exists(ctx, id)
}

// Close closes the cache.
func (s *CachedCredentialStore) Close() error {
	return s.cache.Close()
}

// Cache returns the underlying cache for direct access.
func (s *CachedCredentialStore) Cache() *CredentialCache {
	return s.cache
}

// Ensure CachedCredentialStore implements CredentialStore.
var _ CredentialStore = (*CachedCredentialStore)(nil)
