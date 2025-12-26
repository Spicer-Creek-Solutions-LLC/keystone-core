package edge

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileCache implements Cache interface using filesystem
type FileCache struct {
	basePath string
	mu       sync.RWMutex
	stats    *CacheStats
}

// NewFileCache creates a new file-based cache
func NewFileCache(basePath string) (*FileCache, error) {
	// Create cache directory if it doesn't exist
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	return &FileCache{
		basePath: basePath,
		stats: &CacheStats{
			HitCount:      0,
			MissCount:     0,
			EvictionCount: 0,
		},
	}, nil
}

// Set stores an entry in the cache
func (c *FileCache) Set(entry *CacheEntry) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Create entry directory if needed
	entryDir := filepath.Join(c.basePath, entry.Type)
	if err := os.MkdirAll(entryDir, 0755); err != nil {
		return fmt.Errorf("failed to create entry directory: %w", err)
	}

	// Marshal entry to JSON
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal entry: %w", err)
	}

	// Write to file
	entryPath := filepath.Join(entryDir, entry.ID+".json")
	if err := os.WriteFile(entryPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache entry: %w", err)
	}

	return nil
}

// Get retrieves an entry from the cache
func (c *FileCache) Get(id string) (*CacheEntry, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Try to find entry in state or command directories
	for _, entryType := range []string{"state", "command"} {
		entryPath := filepath.Join(c.basePath, entryType, id+".json")

		data, err := os.ReadFile(entryPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("failed to read cache entry: %w", err)
		}

		var entry CacheEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			return nil, fmt.Errorf("failed to unmarshal entry: %w", err)
		}

		// Check if expired
		if time.Now().After(entry.ExpiresAt) {
			c.stats.MissCount++
			return nil, fmt.Errorf("cache entry expired")
		}

		c.stats.HitCount++
		return &entry, nil
	}

	c.stats.MissCount++
	return nil, fmt.Errorf("cache entry not found")
}

// Delete removes an entry from the cache
func (c *FileCache) Delete(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Try to delete from both directories
	for _, entryType := range []string{"state", "command"} {
		entryPath := filepath.Join(c.basePath, entryType, id+".json")
		if err := os.Remove(entryPath); err != nil {
			if !os.IsNotExist(err) {
				return fmt.Errorf("failed to delete cache entry: %w", err)
			}
		}
	}

	return nil
}

// List returns all cache entries
func (c *FileCache) List() ([]*CacheEntry, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var entries []*CacheEntry

	// List entries from both directories
	for _, entryType := range []string{"state", "command"} {
		dirPath := filepath.Join(c.basePath, entryType)

		files, err := os.ReadDir(dirPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("failed to read directory: %w", err)
		}

		for _, file := range files {
			if file.IsDir() {
				continue
			}

			filePath := filepath.Join(dirPath, file.Name())
			data, err := os.ReadFile(filePath)
			if err != nil {
				continue
			}

			var entry CacheEntry
			if err := json.Unmarshal(data, &entry); err != nil {
				continue
			}

			entries = append(entries, &entry)
		}
	}

	return entries, nil
}

// Clear removes all entries from the cache
func (c *FileCache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Remove all entries from both directories
	for _, entryType := range []string{"state", "command"} {
		dirPath := filepath.Join(c.basePath, entryType)
		if err := os.RemoveAll(dirPath); err != nil {
			if !os.IsNotExist(err) {
				return fmt.Errorf("failed to clear directory: %w", err)
			}
		}
		// Recreate the directory
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			return fmt.Errorf("failed to recreate directory: %w", err)
		}
	}

	c.stats.EvictionCount++
	return nil
}

// Prune removes expired entries
func (c *FileCache) Prune() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	prunedCount := 0

	// Prune entries from both directories
	for _, entryType := range []string{"state", "command"} {
		dirPath := filepath.Join(c.basePath, entryType)

		files, err := os.ReadDir(dirPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("failed to read directory: %w", err)
		}

		for _, file := range files {
			if file.IsDir() {
				continue
			}

			filePath := filepath.Join(dirPath, file.Name())
			data, err := os.ReadFile(filePath)
			if err != nil {
				continue
			}

			var entry CacheEntry
			if err := json.Unmarshal(data, &entry); err != nil {
				continue
			}

			// Remove if expired
			if now.After(entry.ExpiresAt) {
				if err := os.Remove(filePath); err != nil {
					continue
				}
				prunedCount++
			}
		}
	}

	c.stats.EvictionCount += int64(prunedCount)
	return nil
}

// GetSize returns total cache size in bytes
func (c *FileCache) GetSize() (int64, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var totalSize int64

	// Calculate size from both directories
	for _, entryType := range []string{"state", "command"} {
		dirPath := filepath.Join(c.basePath, entryType)

		files, err := os.ReadDir(dirPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return 0, fmt.Errorf("failed to read directory: %w", err)
		}

		for _, file := range files {
			if file.IsDir() {
				continue
			}

			info, err := file.Info()
			if err != nil {
				continue
			}

			totalSize += info.Size()
		}
	}

	return totalSize, nil
}

// GetStats returns cache statistics
func (c *FileCache) GetStats() (*CacheStats, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entries, err := c.List()
	if err != nil {
		return nil, err
	}

	stats := &CacheStats{
		TotalEntries:  len(entries),
		HitCount:      c.stats.HitCount,
		MissCount:     c.stats.MissCount,
		EvictionCount: c.stats.EvictionCount,
	}

	// Calculate total size and find oldest/newest
	for i, entry := range entries {
		stats.TotalSize += entry.Size

		if i == 0 {
			stats.OldestEntry = entry.CreatedAt
			stats.NewestEntry = entry.CreatedAt
		} else {
			if entry.CreatedAt.Before(stats.OldestEntry) {
				stats.OldestEntry = entry.CreatedAt
			}
			if entry.CreatedAt.After(stats.NewestEntry) {
				stats.NewestEntry = entry.CreatedAt
			}
		}
	}

	return stats, nil
}
