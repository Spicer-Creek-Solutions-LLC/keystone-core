package files

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/testing/helpers"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		config  *ClientConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: &ClientConfig{
				ClusterID: "test-cluster",
				AgentID:   "agent-1",
			},
			wantErr: false,
		},
		{
			name: "missing cluster ID",
			config: &ClientConfig{
				AgentID: "agent-1",
			},
			wantErr: true,
		},
		{
			name: "missing agent ID",
			config: &ClientConfig{
				ClusterID: "test-cluster",
			},
			wantErr: true,
		},
		{
			name: "config with custom values",
			config: &ClientConfig{
				ClusterID:        "test-cluster",
				AgentID:          "agent-1",
				DefaultNamespace: "packages",
				RequestTimeout:   10 * time.Minute,
				ChunkTimeout:     1 * time.Minute,
				MaxRetries:       5,
				RetryDelay:       10 * time.Second,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewClient(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil {
				if c.config.ClusterID != tt.config.ClusterID {
					t.Errorf("ClusterID = %v, want %v", c.config.ClusterID, tt.config.ClusterID)
				}
				if c.config.AgentID != tt.config.AgentID {
					t.Errorf("AgentID = %v, want %v", c.config.AgentID, tt.config.AgentID)
				}
				c.Close()
			}
		})
	}
}

func TestClientConfigDefaults(t *testing.T) {
	config := &ClientConfig{
		ClusterID: "test-cluster",
		AgentID:   "agent-1",
	}

	c, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// Verify defaults
	if c.config.DefaultNamespace != "default" {
		t.Errorf("Default DefaultNamespace = %v, want 'default'", c.config.DefaultNamespace)
	}
	if c.config.RequestTimeout != 5*time.Minute {
		t.Errorf("Default RequestTimeout = %v, want %v", c.config.RequestTimeout, 5*time.Minute)
	}
	if c.config.ChunkTimeout != 30*time.Second {
		t.Errorf("Default ChunkTimeout = %v, want %v", c.config.ChunkTimeout, 30*time.Second)
	}
	if c.config.MaxRetries != DefaultRetryAttempts {
		t.Errorf("Default MaxRetries = %v, want %v", c.config.MaxRetries, DefaultRetryAttempts)
	}
	if c.config.RetryDelay != DefaultRetryDelay {
		t.Errorf("Default RetryDelay = %v, want %v", c.config.RetryDelay, DefaultRetryDelay)
	}
}

func TestClientWithCache(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "client-cache-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	config := &ClientConfig{
		ClusterID: "test-cluster",
		AgentID:   "agent-1",
		CacheDir:  tmpDir,
		CacheSize: 100 << 20, // 100MB
		CacheTTL:  1 * time.Hour,
	}

	c, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if c.cache == nil {
		t.Error("Cache should be initialized")
	}
}

func TestClientMetrics(t *testing.T) {
	config := &ClientConfig{
		ClusterID: "test-cluster",
		AgentID:   "agent-1",
	}

	c, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// Initial metrics should be zero
	metrics := c.Metrics()
	if metrics.RequestsTotal != 0 {
		t.Errorf("Initial RequestsTotal = %v, want 0", metrics.RequestsTotal)
	}
	if metrics.RequestsSucceeded != 0 {
		t.Errorf("Initial RequestsSucceeded = %v, want 0", metrics.RequestsSucceeded)
	}
	if metrics.RequestsFailed != 0 {
		t.Errorf("Initial RequestsFailed = %v, want 0", metrics.RequestsFailed)
	}
	if metrics.BytesReceived != 0 {
		t.Errorf("Initial BytesReceived = %v, want 0", metrics.BytesReceived)
	}
	if metrics.CacheHits != 0 {
		t.Errorf("Initial CacheHits = %v, want 0", metrics.CacheHits)
	}
	if metrics.CacheMisses != 0 {
		t.Errorf("Initial CacheMisses = %v, want 0", metrics.CacheMisses)
	}
}

func TestClientGetFileWithoutConnect(t *testing.T) {
	config := &ClientConfig{
		ClusterID: "test-cluster",
		AgentID:   "agent-1",
	}

	c, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// GetFile without Connect should fail
	_, err = c.GetFile(nil, "/test/file.txt", nil)
	if err == nil {
		t.Error("GetFile() should fail without Connect")
	}
}

func TestClientGetMetadataWithoutConnect(t *testing.T) {
	config := &ClientConfig{
		ClusterID: "test-cluster",
		AgentID:   "agent-1",
	}

	c, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// GetMetadata without Connect should fail
	_, err = c.GetMetadata(nil, "/test/file.txt")
	if err == nil {
		t.Error("GetMetadata() should fail without Connect")
	}
}

func TestFileCacheOperations(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "file-cache-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cache, err := NewFileCache(&CacheConfig{
		Dir:     tmpDir,
		MaxSize: 100 << 20,
		TTL:     1 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Put in cache
	err = cache.Put("/test/file.txt", "v1", "abc123def456", testFile, 12)
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	// Get from cache
	entry, err := cache.Get("/test/file.txt", "v1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if entry.Path != "/test/file.txt" {
		t.Errorf("Path = %v, want %v", entry.Path, "/test/file.txt")
	}
	if entry.Version != "v1" {
		t.Errorf("Version = %v, want %v", entry.Version, "v1")
	}
	if entry.Checksum != "abc123def456" {
		t.Errorf("Checksum = %v, want %v", entry.Checksum, "abc123def456")
	}
}

func TestFileCacheGetMissing(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "file-cache-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cache, err := NewFileCache(&CacheConfig{
		Dir:     tmpDir,
		MaxSize: 100 << 20,
		TTL:     1 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()

	// Get non-existent entry
	_, err = cache.Get("/nonexistent.txt", "v1")
	if err == nil {
		t.Error("Get() should fail for non-existent entry")
	}
}

func TestFileCacheTTLExpiry(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "file-cache-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cache, err := NewFileCache(&CacheConfig{
		Dir:     tmpDir,
		MaxSize: 100 << 20,
		TTL:     1 * time.Millisecond, // Very short TTL
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Put in cache
	err = cache.Put("/test/file.txt", "v1", "abc123def456", testFile, 12)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for TTL to expire
	start := time.Now()
	if err := helpers.WaitForTimeout(2*time.Second, 1*time.Millisecond, func() (bool, error) {
		return time.Since(start) >= 10*time.Millisecond, nil
	}); err != nil {
		t.Fatalf("TTL wait did not elapse: %v", err)
	}

	// Get should fail due to expiry
	_, err = cache.Get("/test/file.txt", "v1")
	if err == nil {
		t.Error("Get() should fail for expired entry")
	}
}

func TestFileCacheKey(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "file-cache-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cache, err := NewFileCache(&CacheConfig{
		Dir:     tmpDir,
		MaxSize: 100 << 20,
		TTL:     1 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()

	tests := []struct {
		path     string
		version  string
		expected string
	}{
		{"/test/file.txt", "v1", "/test/file.txt@v1"},
		{"/test/file.txt", "", "/test/file.txt@latest"},
		{"/packages/nginx.deb", "1.24.0", "/packages/nginx.deb@1.24.0"},
	}

	for _, tt := range tests {
		result := cache.cacheKey(tt.path, tt.version)
		if result != tt.expected {
			t.Errorf("cacheKey(%q, %q) = %q, want %q", tt.path, tt.version, result, tt.expected)
		}
	}
}

func TestNewFileCacheMissingDir(t *testing.T) {
	_, err := NewFileCache(&CacheConfig{
		Dir: "",
	})
	if err == nil {
		t.Error("NewFileCache() should fail with empty dir")
	}
}

func TestClientCalculateChecksum(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "checksum-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}

	config := &ClientConfig{
		ClusterID: "test-cluster",
		AgentID:   "agent-1",
	}

	c, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	checksum, err := c.calculateChecksum(testFile)
	if err != nil {
		t.Fatalf("calculateChecksum() error = %v", err)
	}

	// SHA-256 of "hello world" is known
	expected := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
	if checksum != expected {
		t.Errorf("calculateChecksum() = %v, want %v", checksum, expected)
	}
}
