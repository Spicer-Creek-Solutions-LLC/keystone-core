// Package cache provides caching functionality for file distribution.
package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Config is the configuration for the file cache.
type Config struct {
	// Path is the directory to store cached files.
	Path string `yaml:"path"`

	// MaxSize is the maximum cache size in bytes (0 for unlimited).
	MaxSize int64 `yaml:"max_size,omitempty"`

	// MaxAge is the maximum age for cached files (0 for unlimited).
	MaxAge time.Duration `yaml:"max_age,omitempty"`

	// MaxFiles is the maximum number of files to cache (0 for unlimited).
	MaxFiles int `yaml:"max_files,omitempty"`

	// CleanupInterval is how often to run cleanup (default: 1h).
	CleanupInterval time.Duration `yaml:"cleanup_interval,omitempty"`

	// EvictionPolicy determines how files are evicted (lru, lfu, fifo).
	EvictionPolicy EvictionPolicy `yaml:"eviction_policy,omitempty"`
}

// EvictionPolicy determines how files are evicted when cache is full.
type EvictionPolicy string

const (
	// EvictionLRU removes least recently used files first.
	EvictionLRU EvictionPolicy = "lru"

	// EvictionLFU removes least frequently used files first.
	EvictionLFU EvictionPolicy = "lfu"

	// EvictionFIFO removes oldest files first.
	EvictionFIFO EvictionPolicy = "fifo"
)

// Entry represents a cached file.
type Entry struct {
	// Key is the cache key (usually file path).
	Key string

	// Checksum is the SHA-256 checksum of the cached content.
	Checksum string

	// Size is the file size in bytes.
	Size int64

	// ContentType is the MIME type.
	ContentType string

	// CachedAt is when the file was cached.
	CachedAt time.Time

	// LastAccessed is when the file was last accessed.
	LastAccessed time.Time

	// AccessCount is how many times the file was accessed.
	AccessCount int64

	// SourceBackend is the backend the file came from.
	SourceBackend string

	// ETag is the entity tag from the source.
	ETag string

	// Metadata contains additional metadata.
	Metadata map[string]string
}

// Stats contains cache statistics.
type Stats struct {
	// TotalSize is the total size of cached files.
	TotalSize int64

	// FileCount is the number of cached files.
	FileCount int

	// Hits is the number of cache hits.
	Hits int64

	// Misses is the number of cache misses.
	Misses int64

	// Evictions is the number of files evicted.
	Evictions int64

	// HitRate is the cache hit rate (0-1).
	HitRate float64
}

// Cache is a file cache interface.
type Cache interface {
	// Get retrieves a file from the cache.
	Get(ctx context.Context, key string) (*Result, error)

	// Put stores a file in the cache.
	Put(ctx context.Context, key string, reader io.Reader, entry *Entry) error

	// Delete removes a file from the cache.
	Delete(ctx context.Context, key string) error

	// Exists checks if a file is cached.
	Exists(ctx context.Context, key string) (bool, error)

	// GetEntry returns cache entry metadata without the content.
	GetEntry(ctx context.Context, key string) (*Entry, error)

	// Invalidate removes files matching a pattern.
	Invalidate(ctx context.Context, pattern string) (int, error)

	// Clear removes all cached files.
	Clear(ctx context.Context) error

	// Stats returns cache statistics.
	Stats(ctx context.Context) (*Stats, error)

	// Cleanup removes expired or excess files.
	Cleanup(ctx context.Context) error

	// Close closes the cache.
	Close() error
}

// Result is the result of a cache get operation.
type Result struct {
	// Reader streams the cached content.
	Reader io.ReadCloser

	// Entry is the cache entry metadata.
	Entry *Entry
}

// FileCache implements Cache using the filesystem.
type FileCache struct {
	config  *Config
	entries map[string]*Entry
	mu      sync.RWMutex
	stats   Stats

	stopCleanup chan struct{}
	cleanupDone chan struct{}
}

// NewFileCache creates a new filesystem-based cache.
func NewFileCache(config *Config) (*FileCache, error) {
	if config.Path == "" {
		return nil, fmt.Errorf("cache: path is required")
	}

	// Create cache directory
	//nolint:gosec // G301: cache directory needs to be accessible by service user
	if err := os.MkdirAll(config.Path, 0o755); err != nil {
		return nil, fmt.Errorf("cache: failed to create directory: %w", err)
	}

	// Set defaults
	if config.EvictionPolicy == "" {
		config.EvictionPolicy = EvictionLRU
	}
	if config.CleanupInterval == 0 {
		config.CleanupInterval = time.Hour
	}

	cache := &FileCache{
		config:      config,
		entries:     make(map[string]*Entry),
		stopCleanup: make(chan struct{}),
		cleanupDone: make(chan struct{}),
	}

	// Load existing entries from disk
	if err := cache.loadEntries(); err != nil {
		// Non-fatal, just log and continue
		fmt.Printf("cache: failed to load entries: %v\n", err)
	}

	// Start cleanup goroutine
	go cache.cleanupLoop()

	return cache, nil
}

// Get retrieves a file from the cache.
func (c *FileCache) Get(ctx context.Context, key string) (*Result, error) {
	c.mu.Lock()
	entry, ok := c.entries[key]
	if !ok {
		c.stats.Misses++
		c.mu.Unlock()
		return nil, nil // Cache miss, not an error
	}

	// Check if expired
	if c.config.MaxAge > 0 && time.Since(entry.CachedAt) > c.config.MaxAge {
		c.stats.Misses++
		c.mu.Unlock()
		// Delete expired entry
		_ = c.Delete(ctx, key)
		return nil, nil
	}

	// Update access stats
	entry.LastAccessed = time.Now()
	entry.AccessCount++
	c.stats.Hits++
	c.mu.Unlock()

	// Open the cached file
	filePath := c.filePath(key)
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Entry exists but file doesn't - clean up
			c.mu.Lock()
			delete(c.entries, key)
			c.mu.Unlock()
			return nil, nil
		}
		return nil, fmt.Errorf("cache: failed to open file: %w", err)
	}

	return &Result{
		Reader: file,
		Entry:  entry,
	}, nil
}

// Put stores a file in the cache.
func (c *FileCache) Put(ctx context.Context, key string, reader io.Reader, entry *Entry) error {
	filePath := c.filePath(key)

	// Ensure directory exists
	//nolint:gosec // G301: cache directory needs to be accessible by service user
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return fmt.Errorf("cache: failed to create directory: %w", err)
	}

	// Write to temp file first
	tempFile, err := os.CreateTemp(filepath.Dir(filePath), ".cache-*")
	if err != nil {
		return fmt.Errorf("cache: failed to create temp file: %w", err)
	}
	tempPath := tempFile.Name()

	// Calculate checksum while writing
	hash := sha256.New()
	writer := io.MultiWriter(tempFile, hash)

	written, err := io.Copy(writer, reader)
	tempFile.Close()
	if err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("cache: failed to write file: %w", err)
	}

	// Rename temp file to final path
	if err := os.Rename(tempPath, filePath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("cache: failed to rename file: %w", err)
	}

	// Update entry
	entry.Key = key
	entry.Size = written
	entry.Checksum = hex.EncodeToString(hash.Sum(nil))
	entry.CachedAt = time.Now()
	entry.LastAccessed = entry.CachedAt
	if entry.AccessCount == 0 {
		entry.AccessCount = 1
	}

	c.mu.Lock()
	c.entries[key] = entry
	c.mu.Unlock()

	// Enforce capacity limits after adding the new entry
	// This will evict older entries if needed, but not the one we just added
	if err := c.enforceCapacity(ctx, key); err != nil {
		return fmt.Errorf("cache: failed to enforce capacity: %w", err)
	}

	return nil
}

// Delete removes a file from the cache.
func (c *FileCache) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	_, ok := c.entries[key]
	if ok {
		delete(c.entries, key)
	}
	c.mu.Unlock()

	filePath := c.filePath(key)
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cache: failed to delete file: %w", err)
	}

	return nil
}

// Exists checks if a file is cached.
func (c *FileCache) Exists(ctx context.Context, key string) (bool, error) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if !ok {
		return false, nil
	}

	// Check if expired
	if c.config.MaxAge > 0 && time.Since(entry.CachedAt) > c.config.MaxAge {
		return false, nil
	}

	// Verify file exists on disk
	filePath := c.filePath(key)
	_, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		// Clean up orphaned entry
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return false, nil
	}

	return err == nil, err
}

// GetEntry returns cache entry metadata without the content.
func (c *FileCache) GetEntry(ctx context.Context, key string) (*Entry, error) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if !ok {
		return nil, nil
	}

	// Check if expired
	if c.config.MaxAge > 0 && time.Since(entry.CachedAt) > c.config.MaxAge {
		return nil, nil
	}

	return entry, nil
}

// Invalidate removes files matching a pattern.
func (c *FileCache) Invalidate(ctx context.Context, pattern string) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var toDelete []string
	for key := range c.entries {
		matched, err := filepath.Match(pattern, key)
		if err != nil {
			return 0, fmt.Errorf("cache: invalid pattern: %w", err)
		}
		if matched {
			toDelete = append(toDelete, key)
		}
	}

	for _, key := range toDelete {
		delete(c.entries, key)
		filePath := c.filePath(key)
		os.Remove(filePath)
	}

	return len(toDelete), nil
}

// Clear removes all cached files.
func (c *FileCache) Clear(ctx context.Context) error {
	c.mu.Lock()
	c.entries = make(map[string]*Entry)
	c.mu.Unlock()

	// Remove all files in cache directory
	entries, err := os.ReadDir(c.config.Path)
	if err != nil {
		return fmt.Errorf("cache: failed to read directory: %w", err)
	}

	for _, entry := range entries {
		path := filepath.Join(c.config.Path, entry.Name())
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("cache: failed to remove %s: %w", path, err)
		}
	}

	return nil
}

// Stats returns cache statistics.
func (c *FileCache) Stats(ctx context.Context) (*Stats, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := c.stats
	stats.FileCount = len(c.entries)

	var totalSize int64
	for _, entry := range c.entries {
		totalSize += entry.Size
	}
	stats.TotalSize = totalSize

	if stats.Hits+stats.Misses > 0 {
		stats.HitRate = float64(stats.Hits) / float64(stats.Hits+stats.Misses)
	}

	return &stats, nil
}

// Cleanup removes expired or excess files.
func (c *FileCache) Cleanup(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Remove expired entries
	if c.config.MaxAge > 0 {
		now := time.Now()
		for key, entry := range c.entries {
			if now.Sub(entry.CachedAt) > c.config.MaxAge {
				delete(c.entries, key)
				os.Remove(c.filePath(key))
				c.stats.Evictions++
			}
		}
	}

	// Check file count limit
	if c.config.MaxFiles > 0 && len(c.entries) > c.config.MaxFiles {
		toEvict := len(c.entries) - c.config.MaxFiles
		c.evictEntries(toEvict)
	}

	// Check size limit
	if c.config.MaxSize > 0 {
		var totalSize int64
		for _, entry := range c.entries {
			totalSize += entry.Size
		}

		for totalSize > c.config.MaxSize && len(c.entries) > 0 {
			evicted := c.evictEntries(1)
			if evicted == 0 {
				break
			}
			totalSize = 0
			for _, entry := range c.entries {
				totalSize += entry.Size
			}
		}
	}

	return nil
}

// Close closes the cache.
func (c *FileCache) Close() error {
	close(c.stopCleanup)
	<-c.cleanupDone
	return nil
}

// filePath returns the filesystem path for a cache key.
func (c *FileCache) filePath(key string) string {
	// Use SHA256 of key for filename to handle special characters
	hash := sha256.Sum256([]byte(key))
	hashStr := hex.EncodeToString(hash[:])

	// Use first 2 chars as subdirectory for better filesystem distribution
	return filepath.Join(c.config.Path, hashStr[:2], hashStr)
}

// loadEntries loads existing cache entries from disk.
func (c *FileCache) loadEntries() error {
	// Walk the cache directory
	return filepath.Walk(c.config.Path, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil //nolint:nilerr // skip errors in walk to continue
		}

		// Reconstruct key from path - this is a simplified version
		// In production, you'd store metadata alongside files
		relPath, _ := filepath.Rel(c.config.Path, path)
		if len(relPath) > 2 {
			// This is a cached file
			c.entries[path] = &Entry{
				Key:          path,
				Size:         info.Size(),
				CachedAt:     info.ModTime(),
				LastAccessed: info.ModTime(),
				AccessCount:  1,
			}
		}

		return nil
	})
}

// enforceCapacity enforces capacity limits, protecting the specified key from eviction.
func (c *FileCache) enforceCapacity(ctx context.Context, protectedKey string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check file count limit (we may have 1 more than MaxFiles temporarily)
	if c.config.MaxFiles > 0 && len(c.entries) > c.config.MaxFiles {
		c.evictEntriesExcluding(1, protectedKey)
	}

	// Check size limit
	if c.config.MaxSize > 0 {
		var totalSize int64
		for _, entry := range c.entries {
			totalSize += entry.Size
		}

		for totalSize > c.config.MaxSize && len(c.entries) > 1 {
			evicted := c.evictEntriesExcluding(1, protectedKey)
			if evicted == 0 {
				break
			}
			totalSize = 0
			for _, entry := range c.entries {
				totalSize += entry.Size
			}
		}
	}

	return nil
}

// evictEntries evicts entries based on the eviction policy.
func (c *FileCache) evictEntries(count int) int {
	return c.evictEntriesExcluding(count, "")
}

// evictEntriesExcluding evicts entries based on the eviction policy, excluding the specified key.
func (c *FileCache) evictEntriesExcluding(count int, excludeKey string) int {
	if len(c.entries) == 0 || count <= 0 {
		return 0
	}

	// Convert to slice for sorting, excluding protected key
	entries := make([]*Entry, 0, len(c.entries))
	for _, entry := range c.entries {
		if entry.Key != excludeKey {
			entries = append(entries, entry)
		}
	}

	if len(entries) == 0 {
		return 0
	}

	// Sort based on eviction policy
	switch c.config.EvictionPolicy {
	case EvictionLRU:
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].LastAccessed.Before(entries[j].LastAccessed)
		})
	case EvictionLFU:
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].AccessCount < entries[j].AccessCount
		})
	case EvictionFIFO:
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].CachedAt.Before(entries[j].CachedAt)
		})
	}

	// Evict the selected entries
	evicted := 0
	for i := 0; i < count && i < len(entries); i++ {
		entry := entries[i]
		delete(c.entries, entry.Key)
		os.Remove(c.filePath(entry.Key))
		c.stats.Evictions++
		evicted++
	}

	return evicted
}

// cleanupLoop runs periodic cleanup.
func (c *FileCache) cleanupLoop() {
	defer close(c.cleanupDone)

	ticker := time.NewTicker(c.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			_ = c.Cleanup(context.Background()) //nolint:errcheck // best-effort periodic cleanup
		case <-c.stopCleanup:
			return
		}
	}
}
