package cache

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewFileCache(t *testing.T) {
	tests := []struct {
		name    string
		config  *CacheConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: &CacheConfig{
				Path: t.TempDir(),
			},
			wantErr: false,
		},
		{
			name: "missing path",
			config: &CacheConfig{
				MaxSize: 1000,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache, err := NewFileCache(tt.config)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if cache == nil {
					t.Error("expected cache, got nil")
				}
				if cache != nil {
					cache.Close()
				}
			}
		})
	}
}

func TestFileCache_PutGet(t *testing.T) {
	cache, err := NewFileCache(&CacheConfig{
		Path:            t.TempDir(),
		CleanupInterval: time.Hour, // Don't run cleanup during test
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Close()

	ctx := context.Background()

	// Test Put
	content := []byte("Hello, Cache!")
	entry := &CacheEntry{
		ContentType:   "text/plain",
		SourceBackend: "test",
		ETag:          "test-etag",
	}

	err = cache.Put(ctx, "/test/file.txt", bytes.NewReader(content), entry)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Verify entry was updated
	if entry.Size != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), entry.Size)
	}
	if entry.Checksum == "" {
		t.Error("expected checksum, got empty")
	}

	// Test Get
	result, err := cache.Get(ctx, "/test/file.txt")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	defer result.Reader.Close()

	data, _ := io.ReadAll(result.Reader)
	if !bytes.Equal(data, content) {
		t.Errorf("expected content %q, got %q", content, data)
	}

	// Verify entry metadata
	if result.Entry.ContentType != "text/plain" {
		t.Errorf("expected content type 'text/plain', got '%s'", result.Entry.ContentType)
	}
	if result.Entry.ETag != "test-etag" {
		t.Errorf("expected etag 'test-etag', got '%s'", result.Entry.ETag)
	}
}

func TestFileCache_Exists(t *testing.T) {
	cache, err := NewFileCache(&CacheConfig{
		Path:            t.TempDir(),
		CleanupInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Close()

	ctx := context.Background()

	// Test non-existent
	exists, err := cache.Exists(ctx, "/nonexistent.txt")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if exists {
		t.Error("expected false, got true")
	}

	// Add file
	cache.Put(ctx, "/test.txt", bytes.NewReader([]byte("test")), &CacheEntry{})

	// Test exists
	exists, err = cache.Exists(ctx, "/test.txt")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("expected true, got false")
	}
}

func TestFileCache_Delete(t *testing.T) {
	cache, err := NewFileCache(&CacheConfig{
		Path:            t.TempDir(),
		CleanupInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Close()

	ctx := context.Background()

	// Add file
	cache.Put(ctx, "/test.txt", bytes.NewReader([]byte("test")), &CacheEntry{})

	// Delete
	err = cache.Delete(ctx, "/test.txt")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deleted
	exists, _ := cache.Exists(ctx, "/test.txt")
	if exists {
		t.Error("expected file to be deleted")
	}

	// Delete nonexistent should not error
	err = cache.Delete(ctx, "/nonexistent.txt")
	if err != nil {
		t.Errorf("Delete nonexistent should not error: %v", err)
	}
}

func TestFileCache_GetEntry(t *testing.T) {
	cache, err := NewFileCache(&CacheConfig{
		Path:            t.TempDir(),
		CleanupInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Close()

	ctx := context.Background()

	// Test nonexistent
	entry, err := cache.GetEntry(ctx, "/nonexistent.txt")
	if err != nil {
		t.Fatalf("GetEntry failed: %v", err)
	}
	if entry != nil {
		t.Error("expected nil, got entry")
	}

	// Add file
	cache.Put(ctx, "/test.txt", bytes.NewReader([]byte("test")), &CacheEntry{
		ContentType: "text/plain",
	})

	// Get entry
	entry, err = cache.GetEntry(ctx, "/test.txt")
	if err != nil {
		t.Fatalf("GetEntry failed: %v", err)
	}
	if entry == nil {
		t.Error("expected entry, got nil")
	}
	if entry.ContentType != "text/plain" {
		t.Errorf("expected content type 'text/plain', got '%s'", entry.ContentType)
	}
}

func TestFileCache_Clear(t *testing.T) {
	cache, err := NewFileCache(&CacheConfig{
		Path:            t.TempDir(),
		CleanupInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Close()

	ctx := context.Background()

	// Add files
	cache.Put(ctx, "/file1.txt", bytes.NewReader([]byte("test1")), &CacheEntry{})
	cache.Put(ctx, "/file2.txt", bytes.NewReader([]byte("test2")), &CacheEntry{})

	// Clear
	err = cache.Clear(ctx)
	if err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	// Verify cleared
	exists1, _ := cache.Exists(ctx, "/file1.txt")
	exists2, _ := cache.Exists(ctx, "/file2.txt")
	if exists1 || exists2 {
		t.Error("expected cache to be cleared")
	}
}

func TestFileCache_Stats(t *testing.T) {
	cache, err := NewFileCache(&CacheConfig{
		Path:            t.TempDir(),
		CleanupInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Close()

	ctx := context.Background()

	// Add files
	cache.Put(ctx, "/file1.txt", bytes.NewReader([]byte("test1")), &CacheEntry{})
	cache.Put(ctx, "/file2.txt", bytes.NewReader([]byte("test2")), &CacheEntry{})

	// Get stats
	stats, err := cache.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}
	if stats.FileCount != 2 {
		t.Errorf("expected 2 files, got %d", stats.FileCount)
	}
	if stats.TotalSize != 10 { // 5 + 5 bytes
		t.Errorf("expected 10 bytes, got %d", stats.TotalSize)
	}

	// Access to generate hits
	cache.Get(ctx, "/file1.txt")
	cache.Get(ctx, "/nonexistent.txt") // miss

	stats, _ = cache.Stats(ctx)
	if stats.Hits != 1 {
		t.Errorf("expected 1 hit, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("expected 1 miss, got %d", stats.Misses)
	}
	if stats.HitRate != 0.5 {
		t.Errorf("expected hit rate 0.5, got %f", stats.HitRate)
	}
}

func TestFileCache_Invalidate(t *testing.T) {
	cache, err := NewFileCache(&CacheConfig{
		Path:            t.TempDir(),
		CleanupInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Close()

	ctx := context.Background()

	// Add files
	cache.Put(ctx, "/configs/app.yaml", bytes.NewReader([]byte("test")), &CacheEntry{})
	cache.Put(ctx, "/configs/db.yaml", bytes.NewReader([]byte("test")), &CacheEntry{})
	cache.Put(ctx, "/scripts/run.sh", bytes.NewReader([]byte("test")), &CacheEntry{})

	// Invalidate by pattern
	count, err := cache.Invalidate(ctx, "/configs/*")
	if err != nil {
		t.Fatalf("Invalidate failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 invalidated, got %d", count)
	}

	// Verify
	exists1, _ := cache.Exists(ctx, "/configs/app.yaml")
	exists2, _ := cache.Exists(ctx, "/scripts/run.sh")
	if exists1 {
		t.Error("expected /configs/app.yaml to be invalidated")
	}
	if !exists2 {
		t.Error("expected /scripts/run.sh to remain")
	}
}

func TestFileCache_MaxAge(t *testing.T) {
	cache, err := NewFileCache(&CacheConfig{
		Path:            t.TempDir(),
		MaxAge:          50 * time.Millisecond,
		CleanupInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Close()

	ctx := context.Background()

	// Add file
	cache.Put(ctx, "/test.txt", bytes.NewReader([]byte("test")), &CacheEntry{})

	// Should exist
	exists, _ := cache.Exists(ctx, "/test.txt")
	if !exists {
		t.Error("expected file to exist")
	}

	// Wait for expiry
	time.Sleep(100 * time.Millisecond)

	// Should not exist (expired)
	exists, _ = cache.Exists(ctx, "/test.txt")
	if exists {
		t.Error("expected file to be expired")
	}

	// Get should also return nil for expired
	result, err := cache.Get(ctx, "/test.txt")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if result != nil {
		t.Error("expected nil result for expired file")
	}
}

func TestFileCache_MaxFiles(t *testing.T) {
	cache, err := NewFileCache(&CacheConfig{
		Path:            t.TempDir(),
		MaxFiles:        2,
		EvictionPolicy:  EvictionFIFO,
		CleanupInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Close()

	ctx := context.Background()

	// Add 3 files (exceeds limit)
	cache.Put(ctx, "/file1.txt", bytes.NewReader([]byte("test1")), &CacheEntry{})
	time.Sleep(10 * time.Millisecond)
	cache.Put(ctx, "/file2.txt", bytes.NewReader([]byte("test2")), &CacheEntry{})
	time.Sleep(10 * time.Millisecond)
	cache.Put(ctx, "/file3.txt", bytes.NewReader([]byte("test3")), &CacheEntry{})

	// First file should be evicted (FIFO)
	exists1, _ := cache.Exists(ctx, "/file1.txt")
	exists3, _ := cache.Exists(ctx, "/file3.txt")
	if exists1 {
		t.Error("expected file1 to be evicted")
	}
	if !exists3 {
		t.Error("expected file3 to exist")
	}
}

func TestFileCache_MaxSize(t *testing.T) {
	cache, err := NewFileCache(&CacheConfig{
		Path:            t.TempDir(),
		MaxSize:         10, // 10 bytes
		EvictionPolicy:  EvictionFIFO,
		CleanupInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Close()

	ctx := context.Background()

	// Add files (5 bytes each)
	cache.Put(ctx, "/file1.txt", bytes.NewReader([]byte("12345")), &CacheEntry{})
	time.Sleep(10 * time.Millisecond)
	cache.Put(ctx, "/file2.txt", bytes.NewReader([]byte("67890")), &CacheEntry{})
	time.Sleep(10 * time.Millisecond)

	// This should trigger eviction of file1
	cache.Put(ctx, "/file3.txt", bytes.NewReader([]byte("abcde")), &CacheEntry{})

	// First file should be evicted
	exists1, _ := cache.Exists(ctx, "/file1.txt")
	exists3, _ := cache.Exists(ctx, "/file3.txt")
	if exists1 {
		t.Error("expected file1 to be evicted")
	}
	if !exists3 {
		t.Error("expected file3 to exist")
	}
}

func TestFileCache_EvictionLRU(t *testing.T) {
	cache, err := NewFileCache(&CacheConfig{
		Path:            t.TempDir(),
		MaxFiles:        2,
		EvictionPolicy:  EvictionLRU,
		CleanupInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Close()

	ctx := context.Background()

	// Add files
	cache.Put(ctx, "/file1.txt", bytes.NewReader([]byte("test1")), &CacheEntry{})
	cache.Put(ctx, "/file2.txt", bytes.NewReader([]byte("test2")), &CacheEntry{})

	// Access file1 to make it recently used
	time.Sleep(10 * time.Millisecond)
	cache.Get(ctx, "/file1.txt")

	// Add file3, should evict file2 (least recently used)
	time.Sleep(10 * time.Millisecond)
	cache.Put(ctx, "/file3.txt", bytes.NewReader([]byte("test3")), &CacheEntry{})

	// File2 should be evicted, file1 should remain
	exists1, _ := cache.Exists(ctx, "/file1.txt")
	exists2, _ := cache.Exists(ctx, "/file2.txt")
	if !exists1 {
		t.Error("expected file1 to exist (recently accessed)")
	}
	if exists2 {
		t.Error("expected file2 to be evicted (least recently used)")
	}
}

func TestFileCache_FilePath(t *testing.T) {
	cache, err := NewFileCache(&CacheConfig{
		Path:            t.TempDir(),
		CleanupInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Close()

	// Test that filePath produces consistent paths
	path1 := cache.filePath("/test/file.txt")
	path2 := cache.filePath("/test/file.txt")
	if path1 != path2 {
		t.Error("expected consistent file paths")
	}

	// Test that different keys produce different paths
	path3 := cache.filePath("/other/file.txt")
	if path1 == path3 {
		t.Error("expected different file paths for different keys")
	}

	// Verify path structure (subdirectory based on hash prefix)
	relPath, _ := filepath.Rel(cache.config.Path, path1)
	parts := filepath.SplitList(relPath)
	if len(parts) == 0 {
		// Check that the first directory is 2 chars (hash prefix)
		dir := filepath.Dir(relPath)
		if len(filepath.Base(dir)) != 2 {
			t.Errorf("expected 2-char hash prefix directory, got %s", dir)
		}
	}
}

func TestFileCache_CacheMiss(t *testing.T) {
	cache, err := NewFileCache(&CacheConfig{
		Path:            t.TempDir(),
		CleanupInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Close()

	ctx := context.Background()

	// Get nonexistent file should return nil, nil (miss, not error)
	result, err := cache.Get(ctx, "/nonexistent.txt")
	if err != nil {
		t.Errorf("expected no error for cache miss, got: %v", err)
	}
	if result != nil {
		t.Error("expected nil result for cache miss")
	}

	// Verify miss was recorded
	stats, _ := cache.Stats(ctx)
	if stats.Misses != 1 {
		t.Errorf("expected 1 miss, got %d", stats.Misses)
	}
}

func TestFileCache_Cleanup(t *testing.T) {
	cache, err := NewFileCache(&CacheConfig{
		Path:            t.TempDir(),
		MaxAge:          50 * time.Millisecond,
		CleanupInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Close()

	ctx := context.Background()

	// Add file
	cache.Put(ctx, "/test.txt", bytes.NewReader([]byte("test")), &CacheEntry{})

	// Wait for expiry
	time.Sleep(100 * time.Millisecond)

	// Run cleanup
	err = cache.Cleanup(ctx)
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	// Verify eviction was recorded
	stats, _ := cache.Stats(ctx)
	if stats.Evictions != 1 {
		t.Errorf("expected 1 eviction, got %d", stats.Evictions)
	}
}

func TestFileCache_OrphanedEntry(t *testing.T) {
	tempDir := t.TempDir()
	cache, err := NewFileCache(&CacheConfig{
		Path:            tempDir,
		CleanupInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Close()

	ctx := context.Background()

	// Add file
	cache.Put(ctx, "/test.txt", bytes.NewReader([]byte("test")), &CacheEntry{})

	// Get the file path and delete it directly (simulating corruption)
	filePath := cache.filePath("/test.txt")
	os.Remove(filePath)

	// Get should handle orphaned entry gracefully
	result, err := cache.Get(ctx, "/test.txt")
	if err != nil {
		t.Errorf("expected no error for orphaned entry, got: %v", err)
	}
	if result != nil {
		t.Error("expected nil result for orphaned entry")
	}

	// Entry should be cleaned up
	cache.mu.RLock()
	_, exists := cache.entries["/test.txt"]
	cache.mu.RUnlock()
	if exists {
		t.Error("expected orphaned entry to be removed")
	}
}
