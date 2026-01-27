package secrets

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
)

// EncryptedSecretCache provides an in-memory cache for secrets with AES-GCM encryption.
type EncryptedSecretCache struct {
	mu sync.RWMutex

	// config is the cache configuration.
	config *CacheConfig

	// entries stores encrypted secret entries.
	entries map[string]*cacheEntry

	// gcm is the AES-GCM cipher for encryption.
	gcm cipher.AEAD

	// stats tracks cache statistics.
	hits       atomic.Int64
	misses     atomic.Int64
	evictions  atomic.Int64
	expirations atomic.Int64

	// cleanup goroutine management
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// cacheEntry represents an encrypted cached secret.
type cacheEntry struct {
	// encryptedData is the AES-GCM encrypted secret data.
	encryptedData []byte

	// nonce is the GCM nonce used for encryption.
	nonce []byte

	// expiresAt is when the entry expires.
	expiresAt time.Time

	// lastAccess is when the entry was last accessed.
	lastAccess time.Time

	// accessCount is the number of times the entry has been accessed.
	accessCount int64

	// size is the approximate size of the entry in bytes.
	size int
}

// isExpired returns true if the entry has expired.
func (e *cacheEntry) isExpired() bool {
	return time.Now().After(e.expiresAt)
}

// NewEncryptedSecretCache creates a new encrypted secret cache.
func NewEncryptedSecretCache(config *CacheConfig, encryptionKey []byte) (*EncryptedSecretCache, error) {
	if config == nil {
		config = DefaultCacheConfig()
	}

	if len(encryptionKey) == 0 {
		return nil, fmt.Errorf("encryption key is required")
	}

	// Create AES cipher
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	cache := &EncryptedSecretCache{
		config:  config,
		entries: make(map[string]*cacheEntry),
		gcm:     gcm,
		ctx:     ctx,
		cancel:  cancel,
	}

	// Start cleanup goroutine
	cache.wg.Add(1)
	go cache.cleanupLoop()

	return cache, nil
}

// Get retrieves a secret from the cache.
func (c *EncryptedSecretCache) Get(ctx context.Context, path string) (*Secret, bool) {
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
		// Trigger async cleanup
		go c.deleteExpired(path)
		return nil, false
	}

	// Update access time and count while holding read lock
	// (slight race condition acceptable for stats)
	entry.lastAccess = time.Now()
	entry.accessCount++

	encryptedData := entry.encryptedData
	nonce := entry.nonce
	c.mu.RUnlock()

	// Decrypt outside the lock
	plaintext, err := c.gcm.Open(nil, nonce, encryptedData, nil)
	if err != nil {
		c.misses.Add(1)
		// Decryption failed - remove corrupted entry
		go c.Delete(ctx, path)
		return nil, false
	}

	// Unmarshal secret
	var secret Secret
	if err := json.Unmarshal(plaintext, &secret); err != nil {
		c.misses.Add(1)
		return nil, false
	}

	c.hits.Add(1)
	return &secret, true
}

// Put stores a secret in the cache with the given TTL.
func (c *EncryptedSecretCache) Put(ctx context.Context, secret *Secret, ttl time.Duration) error {
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
		return fmt.Errorf("%w: failed to generate nonce: %v", ErrCacheEncryptionFailed, err)
	}

	// Encrypt
	ciphertext := c.gcm.Seal(nil, nonce, plaintext, nil)

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if we need to evict entries
	if len(c.entries) >= c.config.MaxEntries {
		c.evictLRU()
	}

	// Store entry
	now := time.Now()
	c.entries[secret.Path] = &cacheEntry{
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
func (c *EncryptedSecretCache) Delete(ctx context.Context, path string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, path)
	return nil
}

// DeleteByPrefix removes all secrets matching a path prefix.
func (c *EncryptedSecretCache) DeleteByPrefix(ctx context.Context, prefix string) (int, error) {
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
func (c *EncryptedSecretCache) Clear(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*cacheEntry)
	return nil
}

// Stats returns cache statistics.
func (c *EncryptedSecretCache) Stats() *CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var memoryBytes int64
	for _, entry := range c.entries {
		memoryBytes += int64(entry.size)
	}

	return &CacheStats{
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
func (c *EncryptedSecretCache) Close() error {
	c.cancel()
	c.wg.Wait()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Clear all entries
	c.entries = make(map[string]*cacheEntry)

	return nil
}

// evictLRU evicts the least recently used entry.
// Must be called with lock held.
func (c *EncryptedSecretCache) evictLRU() {
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
func (c *EncryptedSecretCache) deleteExpired(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, exists := c.entries[path]; exists && entry.isExpired() {
		delete(c.entries, path)
		c.expirations.Add(1)
	}
}

// cleanupLoop periodically removes expired entries.
func (c *EncryptedSecretCache) cleanupLoop() {
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
func (c *EncryptedSecretCache) cleanup() {
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

// Ensure EncryptedSecretCache implements SecretCache.
var _ SecretCache = (*EncryptedSecretCache)(nil)

// InMemorySecretCache provides a simple in-memory cache without encryption.
// Use this only for testing or when encryption is handled at a different layer.
type InMemorySecretCache struct {
	mu sync.RWMutex

	config  *CacheConfig
	entries map[string]*plaintextCacheEntry

	hits        atomic.Int64
	misses      atomic.Int64
	evictions   atomic.Int64
	expirations atomic.Int64

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// plaintextCacheEntry represents a plaintext cached secret.
type plaintextCacheEntry struct {
	secret      *Secret
	expiresAt   time.Time
	lastAccess  time.Time
	accessCount int64
}

// isExpired returns true if the entry has expired.
func (e *plaintextCacheEntry) isExpired() bool {
	return time.Now().After(e.expiresAt)
}

// NewInMemorySecretCache creates a new in-memory secret cache without encryption.
func NewInMemorySecretCache(config *CacheConfig) *InMemorySecretCache {
	if config == nil {
		config = DefaultCacheConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	cache := &InMemorySecretCache{
		config:  config,
		entries: make(map[string]*plaintextCacheEntry),
		ctx:     ctx,
		cancel:  cancel,
	}

	// Start cleanup goroutine
	cache.wg.Add(1)
	go cache.cleanupLoop()

	return cache
}

// Get retrieves a secret from the cache.
func (c *InMemorySecretCache) Get(ctx context.Context, path string) (*Secret, bool) {
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
	secret := entry.secret
	c.mu.RUnlock()

	c.hits.Add(1)
	return secret, true
}

// Put stores a secret in the cache with the given TTL.
func (c *InMemorySecretCache) Put(ctx context.Context, secret *Secret, ttl time.Duration) error {
	if secret == nil {
		return fmt.Errorf("secret is required")
	}

	if secret.Path == "" {
		return fmt.Errorf("secret path is required")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if we need to evict entries
	if len(c.entries) >= c.config.MaxEntries {
		c.evictLRU()
	}

	now := time.Now()
	c.entries[secret.Path] = &plaintextCacheEntry{
		secret:      secret,
		expiresAt:   now.Add(ttl),
		lastAccess:  now,
		accessCount: 1,
	}

	return nil
}

// Delete removes a secret from the cache.
func (c *InMemorySecretCache) Delete(ctx context.Context, path string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, path)
	return nil
}

// DeleteByPrefix removes all secrets matching a path prefix.
func (c *InMemorySecretCache) DeleteByPrefix(ctx context.Context, prefix string) (int, error) {
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
func (c *InMemorySecretCache) Clear(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*plaintextCacheEntry)
	return nil
}

// Stats returns cache statistics.
func (c *InMemorySecretCache) Stats() *CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return &CacheStats{
		Entries:      len(c.entries),
		MaxEntries:   c.config.MaxEntries,
		Hits:         c.hits.Load(),
		Misses:       c.misses.Load(),
		Evictions:    c.evictions.Load(),
		ExpiredCount: c.expirations.Load(),
	}
}

// Close closes the cache.
func (c *InMemorySecretCache) Close() error {
	c.cancel()
	c.wg.Wait()

	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*plaintextCacheEntry)

	return nil
}

// evictLRU evicts the least recently used entry.
// Must be called with lock held.
func (c *InMemorySecretCache) evictLRU() {
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
func (c *InMemorySecretCache) deleteExpired(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, exists := c.entries[path]; exists && entry.isExpired() {
		delete(c.entries, path)
		c.expirations.Add(1)
	}
}

// cleanupLoop periodically removes expired entries.
func (c *InMemorySecretCache) cleanupLoop() {
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
func (c *InMemorySecretCache) cleanup() {
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

// Ensure InMemorySecretCache implements SecretCache.
var _ SecretCache = (*InMemorySecretCache)(nil)

// GenerateCacheKey generates a random 256-bit key suitable for cache encryption.
func GenerateCacheKey() ([]byte, error) {
	key := make([]byte, 32) // 256 bits
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}
	return key, nil
}
