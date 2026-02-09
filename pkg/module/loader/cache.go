package loader

import (
	"sync"
	"time"
)

// InMemoryModuleCache is a simple in-memory cache for loaded modules
type InMemoryModuleCache struct {
	mu      sync.RWMutex
	modules map[string]*cacheEntry
	maxSize int
	ttl     time.Duration
}

type cacheEntry struct {
	result      *LoadResult
	insertTime  time.Time
	accessTime  time.Time
	accessCount int
}

// NewInMemoryModuleCache creates a new in-memory module cache
func NewInMemoryModuleCache(maxSize int, ttl time.Duration) *InMemoryModuleCache {
	return &InMemoryModuleCache{
		modules: make(map[string]*cacheEntry),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

// Get retrieves a cached module by path and hash
func (c *InMemoryModuleCache) Get(modulePath, contentHash string) (*LoadResult, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := cacheKey(modulePath, contentHash)
	entry, found := c.modules[key]
	if !found {
		return nil, false
	}

	// Check if entry is expired
	if c.ttl > 0 && time.Since(entry.insertTime) > c.ttl {
		// Expired - will be removed on next cleanup
		return nil, false
	}

	// Update access statistics
	entry.accessTime = time.Now()
	entry.accessCount++

	return entry.result, true
}

// Put stores a loaded module in cache
func (c *InMemoryModuleCache) Put(modulePath, contentHash string, result *LoadResult) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict if cache is full
	if len(c.modules) >= c.maxSize {
		c.evictLRU()
	}

	key := cacheKey(modulePath, contentHash)
	c.modules[key] = &cacheEntry{
		result:      result,
		insertTime:  time.Now(),
		accessTime:  time.Now(),
		accessCount: 0,
	}
}

// Invalidate removes a module from cache
func (c *InMemoryModuleCache) Invalidate(modulePath string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Remove all entries for this module path (any hash)
	for key := range c.modules {
		if matchesPath(key, modulePath) {
			delete(c.modules, key)
		}
	}
}

// Clear removes all cached modules
func (c *InMemoryModuleCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.modules = make(map[string]*cacheEntry)
}

// Size returns the current number of cached modules
func (c *InMemoryModuleCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.modules)
}

// evictLRU evicts the least recently used entry
func (c *InMemoryModuleCache) evictLRU() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range c.modules {
		if oldestKey == "" || entry.accessTime.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.accessTime
		}
	}

	if oldestKey != "" {
		delete(c.modules, oldestKey)
	}
}

// CleanupExpired removes expired entries
func (c *InMemoryModuleCache) CleanupExpired() {
	if c.ttl == 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, entry := range c.modules {
		if now.Sub(entry.insertTime) > c.ttl {
			delete(c.modules, key)
		}
	}
}

// cacheKey generates a cache key from module path and content hash
func cacheKey(modulePath, contentHash string) string {
	return modulePath + ":" + contentHash
}

// matchesPath checks if a cache key matches a module path
func matchesPath(key, path string) bool {
	// Simple prefix match - in production would parse the key properly
	return len(key) > len(path) && key[:len(path)] == path
}
