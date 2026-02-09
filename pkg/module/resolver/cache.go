package resolver

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ModuleCache implements content-addressed module caching
type ModuleCache struct {
	config CacheConfig
	index  map[string]*CacheEntry // hash -> entry
	mu     sync.RWMutex
}

// NewModuleCache creates a new module cache
func NewModuleCache(config CacheConfig) (*ModuleCache, error) {
	if config.Dir == "" {
		return nil, fmt.Errorf("cache directory cannot be empty")
	}

	// Create cache directory if it doesn't exist
	if !config.Readonly {
		//nolint:gosec // G301: cache directory needs to be accessible by service user
		if err := os.MkdirAll(config.Dir, 0o755); err != nil {
			return nil, &CacheError{
				Operation: "create",
				Path:      config.Dir,
				Err:       err,
			}
		}
	}

	cache := &ModuleCache{
		config: config,
		index:  make(map[string]*CacheEntry),
	}

	// Load existing cache index
	if err := cache.loadIndex(); err != nil {
		// If index doesn't exist, that's fine - we'll create it
		if !os.IsNotExist(err) {
			return nil, err
		}
	}

	return cache, nil
}

// Get retrieves a module from the cache by hash
func (c *ModuleCache) Get(hash string) (*CacheEntry, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.index[hash]
	if !exists {
		return nil, fmt.Errorf("%w: module with hash %s not found in cache", ErrModuleNotFound, hash)
	}

	// Check if the cached module still exists on disk
	if _, err := os.Stat(entry.Path); os.IsNotExist(err) {
		return nil, &CacheError{
			Operation: "get",
			Module:    entry.Module.Name,
			Path:      entry.Path,
			Err:       err,
		}
	}

	return entry, nil
}

// Put adds a module to the cache
func (c *ModuleCache) Put(module ModuleReference, sourcePath string, verified bool) (*CacheEntry, error) {
	if c.config.Readonly {
		return nil, ErrCacheReadonly
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Compute hash if not provided
	hash := module.Hash
	if hash == "" {
		h, err := computeFileHash(sourcePath)
		if err != nil {
			return nil, &CacheError{
				Operation: "put",
				Module:    module.Name,
				Err:       err,
			}
		}
		hash = h
	}

	// Create content-addressed path: cache/hash[:2]/hash
	destDir := filepath.Join(c.config.Dir, hash[:2])
	destPath := filepath.Join(destDir, hash)

	// Create directory
	//nolint:gosec // G301: cache directory needs to be accessible by service user
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, &CacheError{
			Operation: "put",
			Module:    module.Name,
			Path:      destDir,
			Err:       err,
		}
	}

	// Copy file to cache
	if err := copyFile(sourcePath, destPath); err != nil {
		return nil, &CacheError{
			Operation: "put",
			Module:    module.Name,
			Path:      destPath,
			Err:       err,
		}
	}

	// Get file size
	info, err := os.Stat(destPath)
	if err != nil {
		return nil, &CacheError{
			Operation: "put",
			Module:    module.Name,
			Path:      destPath,
			Err:       err,
		}
	}

	// Create cache entry
	entry := &CacheEntry{
		Module: ModuleReference{
			Name:     module.Name,
			Version:  module.Version,
			Resolved: module.Resolved,
			Hash:     hash,
		},
		Path:     destPath,
		CachedAt: time.Now(),
		Size:     info.Size(),
		Verified: verified,
	}

	// Add to index
	c.index[hash] = entry

	// Save index
	if err := c.saveIndex(); err != nil {
		return nil, err
	}

	return entry, nil
}

// Has checks if a module with the given hash exists in the cache
func (c *ModuleCache) Has(hash string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.index[hash]
	if !exists {
		return false
	}

	// Check if file still exists
	_, err := os.Stat(entry.Path)
	return err == nil
}

// Delete removes a module from the cache
func (c *ModuleCache) Delete(hash string) error {
	if c.config.Readonly {
		return ErrCacheReadonly
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.index[hash]
	if !exists {
		return nil // Already deleted
	}

	// Remove file
	if err := os.Remove(entry.Path); err != nil && !os.IsNotExist(err) {
		return &CacheError{
			Operation: "delete",
			Module:    entry.Module.Name,
			Path:      entry.Path,
			Err:       err,
		}
	}

	// Remove from index
	delete(c.index, hash)

	// Save index
	return c.saveIndex()
}

// List returns all cached modules
func (c *ModuleCache) List() []*CacheEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entries := make([]*CacheEntry, 0, len(c.index))
	for _, entry := range c.index {
		entries = append(entries, entry)
	}

	return entries
}

// Clean removes old or oversized cache entries
func (c *ModuleCache) Clean() error {
	if c.config.Readonly {
		return ErrCacheReadonly
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	toDelete := make([]string, 0)

	// Find entries to delete based on age
	if c.config.MaxAge > 0 {
		for hash, entry := range c.index {
			if now.Sub(entry.CachedAt) > c.config.MaxAge {
				toDelete = append(toDelete, hash)
			}
		}
	}

	// Check total cache size
	if c.config.MaxSize > 0 {
		totalSize := int64(0)
		for _, entry := range c.index {
			totalSize += entry.Size
		}

		// If over size, delete oldest entries until under limit
		if totalSize > c.config.MaxSize {
			// Sort by age
			type entryAge struct {
				hash string
				age  time.Time
				size int64
			}
			entries := make([]entryAge, 0, len(c.index))
			for hash, entry := range c.index {
				entries = append(entries, entryAge{
					hash: hash,
					age:  entry.CachedAt,
					size: entry.Size,
				})
			}

			// Sort by oldest first
			for i := 0; i < len(entries); i++ {
				for j := i + 1; j < len(entries); j++ {
					if entries[i].age.After(entries[j].age) {
						entries[i], entries[j] = entries[j], entries[i]
					}
				}
			}

			// Delete oldest until under size limit
			for _, e := range entries {
				if totalSize <= c.config.MaxSize {
					break
				}
				toDelete = append(toDelete, e.hash)
				totalSize -= e.size
			}
		}
	}

	// Delete entries
	for _, hash := range toDelete {
		if entry, exists := c.index[hash]; exists {
			os.Remove(entry.Path) // Ignore errors
			delete(c.index, hash)
		}
	}

	if len(toDelete) > 0 {
		return c.saveIndex()
	}

	return nil
}

// Size returns the total size of the cache in bytes
func (c *ModuleCache) Size() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := int64(0)
	for _, entry := range c.index {
		total += entry.Size
	}

	return total
}

// Count returns the number of cached modules
func (c *ModuleCache) Count() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.index)
}

// loadIndex loads the cache index from disk
func (c *ModuleCache) loadIndex() error {
	indexPath := filepath.Join(c.config.Dir, "index.json")

	data, err := os.ReadFile(indexPath)
	if err != nil {
		return err
	}

	var index map[string]*CacheEntry
	if err := json.Unmarshal(data, &index); err != nil {
		return &CacheError{
			Operation: "load",
			Path:      indexPath,
			Err:       err,
		}
	}

	c.index = index
	return nil
}

// saveIndex saves the cache index to disk
func (c *ModuleCache) saveIndex() error {
	indexPath := filepath.Join(c.config.Dir, "index.json")

	data, err := json.MarshalIndent(c.index, "", "  ")
	if err != nil {
		return &CacheError{
			Operation: "save",
			Path:      indexPath,
			Err:       err,
		}
	}

	//nolint:gosec // G306: cache index needs to be readable by module resolver
	if err := os.WriteFile(indexPath, data, 0o644); err != nil {
		return &CacheError{
			Operation: "save",
			Path:      indexPath,
			Err:       err,
		}
	}

	return nil
}

// computeFileHash computes the SHA256 hash of a file
func computeFileHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(hash[:]), nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	//nolint:gosec // G306: cached module files need to be readable by module loader
	return os.WriteFile(dst, data, 0o644)
}
