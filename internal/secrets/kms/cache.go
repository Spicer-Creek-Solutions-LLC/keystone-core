package kms

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shawnbutts/keystone-core/internal/secrets"
)

// KMSSecretCacheConfig configures a KMS-backed secret cache.
type KMSSecretCacheConfig struct {
	// MaxEntries is the maximum number of cached entries.
	MaxEntries int `json:"max_entries,omitempty"`

	// DefaultTTL is the default TTL for cached secrets.
	DefaultTTL time.Duration `json:"default_ttl,omitempty"`

	// CleanupInterval is how often to run cache cleanup.
	CleanupInterval time.Duration `json:"cleanup_interval,omitempty"`

	// KeyPurpose is the key purpose for cache encryption.
	KeyPurpose KeyPurpose `json:"key_purpose,omitempty"`

	// RotateKeyOnStart generates a new data key on startup.
	RotateKeyOnStart bool `json:"rotate_key_on_start,omitempty"`
}

// DefaultKMSSecretCacheConfig returns default configuration.
func DefaultKMSSecretCacheConfig() *KMSSecretCacheConfig {
	return &KMSSecretCacheConfig{
		MaxEntries:      10000,
		DefaultTTL:      5 * time.Minute,
		CleanupInterval: time.Minute,
		KeyPurpose:      KeyPurposeCache,
	}
}

// KMSSecretCache provides a secret cache with KMS-backed encryption.
type KMSSecretCache struct {
	mu sync.RWMutex

	config   *KMSSecretCacheConfig
	manager  *KeyHierarchyManager
	gcm      cipher.AEAD
	keyInfo  *DerivedKey
	entries  map[string]*kmsCacheEntry

	hits        atomic.Int64
	misses      atomic.Int64
	evictions   atomic.Int64
	expirations atomic.Int64

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// kmsCacheEntry represents an encrypted cached secret.
type kmsCacheEntry struct {
	encryptedData []byte
	nonce         []byte
	expiresAt     time.Time
	lastAccess    time.Time
	accessCount   int64
	size          int
}

// isExpired returns true if the entry has expired.
func (e *kmsCacheEntry) isExpired() bool {
	return time.Now().After(e.expiresAt)
}

// NewKMSSecretCache creates a new KMS-backed secret cache.
func NewKMSSecretCache(ctx context.Context, manager *KeyHierarchyManager, config *KMSSecretCacheConfig) (*KMSSecretCache, error) {
	if manager == nil {
		return nil, fmt.Errorf("key hierarchy manager is required")
	}

	if config == nil {
		config = DefaultKMSSecretCacheConfig()
	}

	// Get or generate the cache encryption key
	var keyInfo *DerivedKey
	var err error

	if config.RotateKeyOnStart {
		keyInfo, err = manager.RotateDataKey(ctx, config.KeyPurpose)
	} else {
		keyInfo, err = manager.GetDataKey(ctx, config.KeyPurpose)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get cache encryption key: %w", err)
	}

	// Create AES-GCM cipher
	block, err := aes.NewCipher(keyInfo.plaintext)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	cacheCtx, cancel := context.WithCancel(context.Background())

	cache := &KMSSecretCache{
		config:  config,
		manager: manager,
		gcm:     gcm,
		keyInfo: keyInfo,
		entries: make(map[string]*kmsCacheEntry),
		ctx:     cacheCtx,
		cancel:  cancel,
	}

	// Start cleanup goroutine
	cache.wg.Add(1)
	go cache.cleanupLoop()

	return cache, nil
}

// Get retrieves a secret from the cache.
func (c *KMSSecretCache) Get(ctx context.Context, path string) (*secrets.Secret, bool) {
	c.mu.RLock()
	entry, exists := c.entries[path]
	if !exists {
		c.mu.RUnlock()
		c.misses.Add(1)
		return nil, false
	}

	if entry.isExpired() {
		c.mu.RUnlock()
		c.misses.Add(1)
		go c.deleteExpired(path)
		return nil, false
	}

	entry.lastAccess = time.Now()
	entry.accessCount++

	encryptedData := entry.encryptedData
	nonce := entry.nonce
	c.mu.RUnlock()

	// Decrypt outside the lock
	plaintext, err := c.gcm.Open(nil, nonce, encryptedData, nil)
	if err != nil {
		c.misses.Add(1)
		go c.Delete(ctx, path)
		return nil, false
	}

	var secret secrets.Secret
	if err := json.Unmarshal(plaintext, &secret); err != nil {
		c.misses.Add(1)
		return nil, false
	}

	c.hits.Add(1)
	return &secret, true
}

// Put stores a secret in the cache with the given TTL.
func (c *KMSSecretCache) Put(ctx context.Context, secret *secrets.Secret, ttl time.Duration) error {
	if secret == nil {
		return fmt.Errorf("secret is required")
	}

	if secret.Path == "" {
		return fmt.Errorf("secret path is required")
	}

	// Serialize secret
	plaintext, err := json.Marshal(secret)
	if err != nil {
		return fmt.Errorf("failed to serialize secret: %w", err)
	}

	// Generate nonce
	nonce := make([]byte, c.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt
	ciphertext := c.gcm.Seal(nil, nonce, plaintext, nil)

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if we need to evict entries
	if len(c.entries) >= c.config.MaxEntries {
		c.evictLRU()
	}

	now := time.Now()
	c.entries[secret.Path] = &kmsCacheEntry{
		encryptedData: ciphertext,
		nonce:         nonce,
		expiresAt:     now.Add(ttl),
		lastAccess:    now,
		accessCount:   1,
		size:          len(ciphertext) + len(nonce),
	}

	return nil
}

// Delete removes a secret from the cache.
func (c *KMSSecretCache) Delete(ctx context.Context, path string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, path)
	return nil
}

// DeleteByPrefix removes all secrets matching a path prefix.
func (c *KMSSecretCache) DeleteByPrefix(ctx context.Context, prefix string) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	count := 0
	for path := range c.entries {
		if strings.HasPrefix(path, prefix) {
			delete(c.entries, path)
			count++
		}
	}

	return count, nil
}

// Clear removes all secrets from the cache.
func (c *KMSSecretCache) Clear(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*kmsCacheEntry)
	return nil
}

// Stats returns cache statistics.
func (c *KMSSecretCache) Stats() *secrets.CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var memoryBytes int64
	for _, entry := range c.entries {
		memoryBytes += int64(entry.size)
	}

	return &secrets.CacheStats{
		Entries:      len(c.entries),
		MaxEntries:   c.config.MaxEntries,
		Hits:         c.hits.Load(),
		Misses:       c.misses.Load(),
		Evictions:    c.evictions.Load(),
		ExpiredCount: c.expirations.Load(),
		MemoryBytes:  memoryBytes,
	}
}

// Close closes the cache and releases resources.
func (c *KMSSecretCache) Close() error {
	c.cancel()
	c.wg.Wait()

	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*kmsCacheEntry)
	return nil
}

// RotateKey rotates the cache encryption key.
func (c *KMSSecretCache) RotateKey(ctx context.Context) error {
	// Get new key
	newKeyInfo, err := c.manager.RotateDataKey(ctx, c.config.KeyPurpose)
	if err != nil {
		return fmt.Errorf("failed to rotate key: %w", err)
	}

	// Create new cipher
	block, err := aes.NewCipher(newKeyInfo.plaintext)
	if err != nil {
		return fmt.Errorf("failed to create AES cipher: %w", err)
	}

	newGCM, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("failed to create GCM: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Re-encrypt all entries with the new key
	oldGCM := c.gcm

	for path, entry := range c.entries {
		// Decrypt with old key
		plaintext, err := oldGCM.Open(nil, entry.nonce, entry.encryptedData, nil)
		if err != nil {
			// Remove corrupted entries
			delete(c.entries, path)
			continue
		}

		// Generate new nonce
		nonce := make([]byte, newGCM.NonceSize())
		if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
			delete(c.entries, path)
			continue
		}

		// Re-encrypt with new key
		ciphertext := newGCM.Seal(nil, nonce, plaintext, nil)

		entry.encryptedData = ciphertext
		entry.nonce = nonce
		entry.size = len(ciphertext) + len(nonce)
	}

	c.gcm = newGCM
	c.keyInfo = newKeyInfo

	return nil
}

// KeyVersion returns the current cache key version.
func (c *KMSSecretCache) KeyVersion() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.keyInfo.Version
}

// evictLRU evicts the least recently used entry.
func (c *KMSSecretCache) evictLRU() {
	var oldestPath string
	var oldestTime time.Time

	for path, entry := range c.entries {
		if oldestPath == "" || entry.lastAccess.Before(oldestTime) {
			oldestPath = path
			oldestTime = entry.lastAccess
		}
	}

	if oldestPath != "" {
		delete(c.entries, oldestPath)
		c.evictions.Add(1)
	}
}

// deleteExpired removes an expired entry.
func (c *KMSSecretCache) deleteExpired(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, exists := c.entries[path]; exists && entry.isExpired() {
		delete(c.entries, path)
		c.expirations.Add(1)
	}
}

// cleanupLoop periodically removes expired entries.
func (c *KMSSecretCache) cleanupLoop() {
	defer c.wg.Done()

	interval := c.config.CleanupInterval
	if interval == 0 {
		interval = time.Minute
	}

	ticker := time.NewTicker(interval)
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

// cleanup removes all expired entries.
func (c *KMSSecretCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for path, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, path)
			c.expirations.Add(1)
		}
	}
}

// =============================================================================
// Multi-Tier Cache
// =============================================================================

// MultiTierCacheConfig configures a multi-tier cache.
type MultiTierCacheConfig struct {
	// L1Config configures the L1 (in-memory) cache.
	L1Config *L1CacheConfig `json:"l1,omitempty"`

	// L2Config configures the L2 (KMS-encrypted) cache.
	L2Config *KMSSecretCacheConfig `json:"l2,omitempty"`
}

// L1CacheConfig configures the L1 in-memory cache.
type L1CacheConfig struct {
	// MaxEntries is the maximum number of L1 entries.
	MaxEntries int `json:"max_entries,omitempty"`

	// TTL is the L1 cache TTL.
	TTL time.Duration `json:"ttl,omitempty"`
}

// DefaultMultiTierCacheConfig returns default multi-tier cache configuration.
func DefaultMultiTierCacheConfig() *MultiTierCacheConfig {
	return &MultiTierCacheConfig{
		L1Config: &L1CacheConfig{
			MaxEntries: 1000,
			TTL:        30 * time.Second,
		},
		L2Config: DefaultKMSSecretCacheConfig(),
	}
}

// MultiTierSecretCache provides a two-tier cache with L1 in-memory and L2 KMS-encrypted.
type MultiTierSecretCache struct {
	config *MultiTierCacheConfig
	l1     *l1Cache
	l2     *KMSSecretCache
}

// l1Cache is a simple in-memory cache for frequently accessed secrets.
type l1Cache struct {
	mu      sync.RWMutex
	entries map[string]*l1Entry
	config  *L1CacheConfig

	hits   atomic.Int64
	misses atomic.Int64
}

type l1Entry struct {
	secret    *secrets.Secret
	expiresAt time.Time
}

func newL1Cache(config *L1CacheConfig) *l1Cache {
	return &l1Cache{
		entries: make(map[string]*l1Entry),
		config:  config,
	}
}

func (c *l1Cache) Get(path string) (*secrets.Secret, bool) {
	c.mu.RLock()
	entry, exists := c.entries[path]
	c.mu.RUnlock()

	if !exists {
		c.misses.Add(1)
		return nil, false
	}

	if time.Now().After(entry.expiresAt) {
		c.misses.Add(1)
		c.mu.Lock()
		delete(c.entries, path)
		c.mu.Unlock()
		return nil, false
	}

	c.hits.Add(1)
	return entry.secret, true
}

func (c *l1Cache) Put(secret *secrets.Secret) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Simple eviction if at capacity
	if len(c.entries) >= c.config.MaxEntries {
		// Remove one random entry
		for path := range c.entries {
			delete(c.entries, path)
			break
		}
	}

	c.entries[secret.Path] = &l1Entry{
		secret:    secret,
		expiresAt: time.Now().Add(c.config.TTL),
	}
}

func (c *l1Cache) Delete(path string) {
	c.mu.Lock()
	delete(c.entries, path)
	c.mu.Unlock()
}

func (c *l1Cache) Clear() {
	c.mu.Lock()
	c.entries = make(map[string]*l1Entry)
	c.mu.Unlock()
}

// NewMultiTierSecretCache creates a new multi-tier secret cache.
func NewMultiTierSecretCache(ctx context.Context, manager *KeyHierarchyManager, config *MultiTierCacheConfig) (*MultiTierSecretCache, error) {
	if config == nil {
		config = DefaultMultiTierCacheConfig()
	}

	l2, err := NewKMSSecretCache(ctx, manager, config.L2Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create L2 cache: %w", err)
	}

	return &MultiTierSecretCache{
		config: config,
		l1:     newL1Cache(config.L1Config),
		l2:     l2,
	}, nil
}

// Get retrieves a secret from the cache, checking L1 first.
func (c *MultiTierSecretCache) Get(ctx context.Context, path string) (*secrets.Secret, bool) {
	// Check L1 first
	if secret, ok := c.l1.Get(path); ok {
		return secret, true
	}

	// Check L2
	secret, ok := c.l2.Get(ctx, path)
	if ok {
		// Promote to L1
		c.l1.Put(secret)
		return secret, true
	}

	return nil, false
}

// Put stores a secret in both cache tiers.
func (c *MultiTierSecretCache) Put(ctx context.Context, secret *secrets.Secret, ttl time.Duration) error {
	// Put in L2 first (persistent)
	if err := c.l2.Put(ctx, secret, ttl); err != nil {
		return err
	}

	// Put in L1
	c.l1.Put(secret)

	return nil
}

// Delete removes a secret from both cache tiers.
func (c *MultiTierSecretCache) Delete(ctx context.Context, path string) error {
	c.l1.Delete(path)
	return c.l2.Delete(ctx, path)
}

// DeleteByPrefix removes all secrets matching a prefix from both tiers.
func (c *MultiTierSecretCache) DeleteByPrefix(ctx context.Context, prefix string) (int, error) {
	// Clear L1 (simple clear since we don't track by prefix)
	c.l1.Clear()
	return c.l2.DeleteByPrefix(ctx, prefix)
}

// Clear removes all secrets from both cache tiers.
func (c *MultiTierSecretCache) Clear(ctx context.Context) error {
	c.l1.Clear()
	return c.l2.Clear(ctx)
}

// Stats returns combined cache statistics.
func (c *MultiTierSecretCache) Stats() *secrets.CacheStats {
	l2Stats := c.l2.Stats()

	return &secrets.CacheStats{
		Entries:      l2Stats.Entries,
		MaxEntries:   l2Stats.MaxEntries,
		Hits:         c.l1.hits.Load() + l2Stats.Hits,
		Misses:       c.l1.misses.Load() + l2Stats.Misses,
		Evictions:    l2Stats.Evictions,
		ExpiredCount: l2Stats.ExpiredCount,
		MemoryBytes:  l2Stats.MemoryBytes,
	}
}

// Close closes the cache.
func (c *MultiTierSecretCache) Close() error {
	c.l1.Clear()
	return c.l2.Close()
}

// Ensure interfaces are implemented.
var (
	_ secrets.SecretCache = (*KMSSecretCache)(nil)
	_ secrets.SecretCache = (*MultiTierSecretCache)(nil)
)
