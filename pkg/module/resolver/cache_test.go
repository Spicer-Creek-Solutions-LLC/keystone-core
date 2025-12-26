package resolver

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestModuleCache_PutAndGet(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewModuleCache(CacheConfig{Dir: tmpDir})
	if err != nil {
		t.Fatalf("NewModuleCache() error = %v", err)
	}

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	testData := []byte("test module data")
	if err := os.WriteFile(testFile, testData, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Put module in cache
	module := ModuleReference{
		Name:    "test/module",
		Version: "1.0.0",
	}

	entry, err := cache.Put(module, testFile, true)
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	if entry.Module.Name != module.Name {
		t.Errorf("Put() module name = %v, want %v", entry.Module.Name, module.Name)
	}

	if entry.Module.Hash == "" {
		t.Error("Put() hash is empty")
	}

	// Get module from cache
	retrieved, err := cache.Get(entry.Module.Hash)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if retrieved.Module.Name != module.Name {
		t.Errorf("Get() module name = %v, want %v", retrieved.Module.Name, module.Name)
	}

	if retrieved.Module.Hash != entry.Module.Hash {
		t.Errorf("Get() hash = %v, want %v", retrieved.Module.Hash, entry.Module.Hash)
	}
}

func TestModuleCache_Has(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewModuleCache(CacheConfig{Dir: tmpDir})
	if err != nil {
		t.Fatalf("NewModuleCache() error = %v", err)
	}

	// Create and cache a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	module := ModuleReference{Name: "test/module", Version: "1.0.0"}
	entry, _ := cache.Put(module, testFile, true)

	// Check if exists
	if !cache.Has(entry.Module.Hash) {
		t.Error("Has() = false, want true")
	}

	// Check non-existent
	if cache.Has("nonexistent") {
		t.Error("Has(nonexistent) = true, want false")
	}
}

func TestModuleCache_Delete(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewModuleCache(CacheConfig{Dir: tmpDir})
	if err != nil {
		t.Fatalf("NewModuleCache() error = %v", err)
	}

	// Create and cache a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	module := ModuleReference{Name: "test/module", Version: "1.0.0"}
	entry, _ := cache.Put(module, testFile, true)

	// Delete
	if err := cache.Delete(entry.Module.Hash); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify deleted
	if cache.Has(entry.Module.Hash) {
		t.Error("Has() after delete = true, want false")
	}
}

func TestModuleCache_List(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewModuleCache(CacheConfig{Dir: tmpDir})
	if err != nil {
		t.Fatalf("NewModuleCache() error = %v", err)
	}

	// Add multiple modules with unique content (so unique hashes)
	for i := 0; i < 3; i++ {
		testFile := filepath.Join(tmpDir, "test", string(rune('a'+i))+".txt")
		os.MkdirAll(filepath.Dir(testFile), 0755)
		// Use unique content for each file so they have different hashes
		content := []byte(fmt.Sprintf("test module %d", i))
		if err := os.WriteFile(testFile, content, 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		module := ModuleReference{Name: "test/module", Version: "1.0.0"}
		cache.Put(module, testFile, true)
	}

	list := cache.List()
	if len(list) != 3 {
		t.Errorf("List() = %v entries, want 3", len(list))
	}
}

func TestModuleCache_Clean_MaxAge(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewModuleCache(CacheConfig{
		Dir:    tmpDir,
		MaxAge: 1 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewModuleCache() error = %v", err)
	}

	// Add a module
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	module := ModuleReference{Name: "test/module", Version: "1.0.0"}
	entry, _ := cache.Put(module, testFile, true)

	// Manually set old timestamp
	cache.mu.Lock()
	cache.index[entry.Module.Hash].CachedAt = time.Now().Add(-2 * time.Second)
	cache.mu.Unlock()

	// Clean should remove it
	if err := cache.Clean(); err != nil {
		t.Fatalf("Clean() error = %v", err)
	}

	if cache.Has(entry.Module.Hash) {
		t.Error("Module should be deleted after Clean() due to age")
	}
}

func TestModuleCache_Size(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewModuleCache(CacheConfig{Dir: tmpDir})
	if err != nil {
		t.Fatalf("NewModuleCache() error = %v", err)
	}

	// Add a module
	testFile := filepath.Join(tmpDir, "test.txt")
	testData := []byte("test module data")
	if err := os.WriteFile(testFile, testData, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	module := ModuleReference{Name: "test/module", Version: "1.0.0"}
	cache.Put(module, testFile, true)

	size := cache.Size()
	if size != int64(len(testData)) {
		t.Errorf("Size() = %v, want %v", size, len(testData))
	}
}

func TestModuleCache_Count(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewModuleCache(CacheConfig{Dir: tmpDir})
	if err != nil {
		t.Fatalf("NewModuleCache() error = %v", err)
	}

	if cache.Count() != 0 {
		t.Errorf("Count() = %v, want 0", cache.Count())
	}

	// Add a module
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	module := ModuleReference{Name: "test/module", Version: "1.0.0"}
	cache.Put(module, testFile, true)

	if cache.Count() != 1 {
		t.Errorf("Count() = %v, want 1", cache.Count())
	}
}

func TestModuleCache_Readonly(t *testing.T) {
	tmpDir := t.TempDir()

	// Create cache directory first
	os.MkdirAll(tmpDir, 0755)

	cache, err := NewModuleCache(CacheConfig{
		Dir:      tmpDir,
		Readonly: true,
	})
	if err != nil {
		t.Fatalf("NewModuleCache() error = %v", err)
	}

	// Try to put (should fail)
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	module := ModuleReference{Name: "test/module", Version: "1.0.0"}
	_, err = cache.Put(module, testFile, true)
	if err != ErrCacheReadonly {
		t.Errorf("Put() in readonly cache error = %v, want ErrCacheReadonly", err)
	}
}

func TestModuleCache_IndexPersistence(t *testing.T) {
	tmpDir := t.TempDir()

	// Create first cache instance
	cache1, err := NewModuleCache(CacheConfig{Dir: tmpDir})
	if err != nil {
		t.Fatalf("NewModuleCache() error = %v", err)
	}

	// Add a module
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	module := ModuleReference{Name: "test/module", Version: "1.0.0"}
	entry, _ := cache1.Put(module, testFile, true)

	// Create second cache instance (should load index)
	cache2, err := NewModuleCache(CacheConfig{Dir: tmpDir})
	if err != nil {
		t.Fatalf("NewModuleCache() error = %v", err)
	}

	// Check if module exists in new instance
	if !cache2.Has(entry.Module.Hash) {
		t.Error("Index not persisted across cache instances")
	}
}
