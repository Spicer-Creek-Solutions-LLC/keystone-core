package agent

import (
	"sync"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/identity"
	"github.com/shawnbutts/keystone-core/internal/secrets"
)

// =============================================================================
// Client Configuration Tests
// =============================================================================

func TestClientConfigFields(t *testing.T) {
	config := &ClientConfig{
		BrokerAddress:  "localhost:8080",
		AgentID:        "test-agent",
		SpiffeID:       "spiffe://example.org/agent/test",
		RequestTimeout: 10 * time.Second,
		CacheConfig: &CacheConfig{
			Enabled:       true,
			MemoryEnabled: true,
		},
	}

	if config.BrokerAddress != "localhost:8080" {
		t.Errorf("BrokerAddress = %q, want %q", config.BrokerAddress, "localhost:8080")
	}

	if config.AgentID != "test-agent" {
		t.Errorf("AgentID = %q, want %q", config.AgentID, "test-agent")
	}
}

// =============================================================================
// Memory Cache Tests
// =============================================================================

func TestMemoryCacheSetGet(t *testing.T) {
	cache := NewMemoryCache(100, time.Hour)

	secret := &secrets.Secret{
		Path: "test/path",
		Data: map[string]interface{}{"key": "value"},
	}

	cache.Set("test/path", secret)

	got, found := cache.Get("test/path")
	if !found {
		t.Fatal("expected to find cached secret")
	}
	if got.Path != secret.Path {
		t.Errorf("got path %q, want %q", got.Path, secret.Path)
	}
}

func TestMemoryCacheExpiration(t *testing.T) {
	cache := NewMemoryCache(100, 50*time.Millisecond)

	secret := &secrets.Secret{
		Path: "test/expiring",
		Data: map[string]interface{}{"key": "value"},
	}

	cache.Set("test/expiring", secret)

	_, found := cache.Get("test/expiring")
	if !found {
		t.Error("expected to find secret immediately after set")
	}

	time.Sleep(100 * time.Millisecond)

	_, found = cache.Get("test/expiring")
	if found {
		t.Error("expected secret to be expired")
	}
}

func TestMemoryCacheEviction(t *testing.T) {
	cache := NewMemoryCache(2, time.Hour)

	for i := 0; i < 3; i++ {
		secret := &secrets.Secret{
			Path: "test/path" + string(rune('0'+i)),
			Data: map[string]interface{}{"key": i},
		}
		cache.Set(secret.Path, secret)
		time.Sleep(10 * time.Millisecond)
	}

	if cache.Size() > 2 {
		t.Errorf("cache size %d exceeds max %d", cache.Size(), 2)
	}
}

func TestMemoryCacheDelete(t *testing.T) {
	cache := NewMemoryCache(100, time.Hour)

	secret := &secrets.Secret{
		Path: "test/delete",
		Data: map[string]interface{}{"key": "value"},
	}

	cache.Set("test/delete", secret)
	cache.Delete("test/delete")

	_, found := cache.Get("test/delete")
	if found {
		t.Error("expected secret to be deleted")
	}
}

func TestMemoryCacheClear(t *testing.T) {
	cache := NewMemoryCache(100, time.Hour)

	for i := 0; i < 5; i++ {
		secret := &secrets.Secret{
			Path: "test/clear" + string(rune('0'+i)),
		}
		cache.Set(secret.Path, secret)
	}

	if cache.Size() != 5 {
		t.Errorf("expected 5 entries, got %d", cache.Size())
	}

	cache.Clear()

	if cache.Size() != 0 {
		t.Errorf("expected 0 entries after clear, got %d", cache.Size())
	}
}

func TestMemoryCacheConcurrency(t *testing.T) {
	cache := NewMemoryCache(1000, time.Hour)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			secret := &secrets.Secret{
				Path: "concurrent/path" + string(rune(idx)),
			}
			cache.Set(secret.Path, secret)
			cache.Get(secret.Path)
			cache.Delete(secret.Path)
		}(i)
	}
	wg.Wait()
}

func TestMemoryCacheDefaultValues(t *testing.T) {
	// Zero maxEntries should use default
	cache := NewMemoryCache(0, 0)

	secret := &secrets.Secret{Path: "test/default"}
	cache.Set("test/default", secret)

	_, found := cache.Get("test/default")
	if !found {
		t.Error("cache should work with default values")
	}
}

// =============================================================================
// Disk Cache Tests
// =============================================================================

func TestDiskCacheSetGet(t *testing.T) {
	dir := t.TempDir()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	cache, err := NewDiskCache(dir, key)
	if err != nil {
		t.Fatalf("failed to create disk cache: %v", err)
	}

	secret := &secrets.Secret{
		Path: "disk/test",
		Data: map[string]interface{}{"key": "encrypted-value"},
	}

	if err := cache.Set("disk/test", secret); err != nil {
		t.Fatalf("failed to set: %v", err)
	}

	got, err := cache.Get("disk/test")
	if err != nil {
		t.Fatalf("failed to get: %v", err)
	}

	if got.Path != secret.Path {
		t.Errorf("got path %q, want %q", got.Path, secret.Path)
	}
}

func TestDiskCacheDelete(t *testing.T) {
	dir := t.TempDir()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	cache, err := NewDiskCache(dir, key)
	if err != nil {
		t.Fatalf("failed to create disk cache: %v", err)
	}

	secret := &secrets.Secret{
		Path: "disk/delete",
		Data: map[string]interface{}{"key": "value"},
	}

	if err := cache.Set("disk/delete", secret); err != nil {
		t.Fatalf("failed to set: %v", err)
	}

	if err := cache.Delete("disk/delete"); err != nil {
		t.Fatalf("failed to delete: %v", err)
	}

	// DiskCache.Get returns (nil, nil) for missing files
	got, err := cache.Get("disk/delete")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Error("expected nil secret after delete")
	}
}

func TestDiskCacheNotFound(t *testing.T) {
	dir := t.TempDir()
	key := make([]byte, 32)

	cache, err := NewDiskCache(dir, key)
	if err != nil {
		t.Fatalf("failed to create disk cache: %v", err)
	}

	// DiskCache.Get returns (nil, nil) for missing files
	got, err := cache.Get("nonexistent/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Error("expected nil for nonexistent path")
	}
}

func TestDiskCacheAutoGenerateKey(t *testing.T) {
	dir := t.TempDir()

	// nil key should auto-generate
	cache, err := NewDiskCache(dir, nil)
	if err != nil {
		t.Fatalf("failed to create disk cache with auto key: %v", err)
	}

	secret := &secrets.Secret{Path: "auto/key"}
	if err := cache.Set("auto/key", secret); err != nil {
		t.Fatalf("set failed: %v", err)
	}

	got, err := cache.Get("auto/key")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if got.Path != secret.Path {
		t.Errorf("path mismatch")
	}
}

func TestDiskCachePersistence(t *testing.T) {
	dir := t.TempDir()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 100)
	}

	secret := &secrets.Secret{
		Path: "disk/persist",
		Data: map[string]interface{}{"persist": "test"},
	}

	cache1, err := NewDiskCache(dir, key)
	if err != nil {
		t.Fatalf("failed to create first cache: %v", err)
	}
	if err := cache1.Set("disk/persist", secret); err != nil {
		t.Fatalf("failed to set: %v", err)
	}

	cache2, err := NewDiskCache(dir, key)
	if err != nil {
		t.Fatalf("failed to create second cache: %v", err)
	}

	got, err := cache2.Get("disk/persist")
	if err != nil {
		t.Fatalf("failed to get from new instance: %v", err)
	}

	if got.Path != secret.Path {
		t.Errorf("persisted path mismatch: got %q, want %q", got.Path, secret.Path)
	}
}

func TestDiskCacheWrongKey(t *testing.T) {
	dir := t.TempDir()
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	for i := range key1 {
		key1[i] = byte(i)
		key2[i] = byte(255 - i)
	}

	cache1, err := NewDiskCache(dir, key1)
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	secret := &secrets.Secret{Path: "disk/wrongkey"}
	if err := cache1.Set("disk/wrongkey", secret); err != nil {
		t.Fatalf("failed to set: %v", err)
	}

	cache2, err := NewDiskCache(dir, key2)
	if err != nil {
		t.Fatalf("failed to create cache with different key: %v", err)
	}

	_, err = cache2.Get("disk/wrongkey")
	if err == nil {
		t.Error("expected error when decrypting with wrong key")
	}
}

func TestDiskCacheEmptyPath(t *testing.T) {
	_, err := NewDiskCache("", nil)
	if err == nil {
		t.Error("expected error for empty path")
	}
}

// =============================================================================
// Client State Tests
// =============================================================================

func TestClientStateConstants(t *testing.T) {
	states := []ClientState{
		ClientStateDisconnected,
		ClientStateConnecting,
		ClientStateConnected,
		ClientStateClosed,
	}

	for _, s := range states {
		if s == "" {
			t.Error("state should not be empty")
		}
	}

	seen := make(map[ClientState]bool)
	for _, s := range states {
		if seen[s] {
			t.Errorf("duplicate state: %s", s)
		}
		seen[s] = true
	}
}

// =============================================================================
// Refresh Scheduler Tests
// =============================================================================

func TestRefreshSchedulerSchedule(t *testing.T) {
	config := &RefreshConfig{
		Enabled:              true,
		RefreshThreshold:     0.75,
		CheckInterval:        time.Second,
		MaxConcurrentRefresh: 5,
	}

	scheduler := NewRefreshScheduler(nil, config)

	expiresAt := time.Now().Add(time.Hour)
	scheduler.Schedule("test/path", expiresAt)

	if scheduler.Pending() != 1 {
		t.Errorf("expected 1 pending, got %d", scheduler.Pending())
	}
}

func TestRefreshSchedulerCancel(t *testing.T) {
	config := &RefreshConfig{
		Enabled:              true,
		RefreshThreshold:     0.75,
		CheckInterval:        time.Second,
		MaxConcurrentRefresh: 5,
	}

	scheduler := NewRefreshScheduler(nil, config)

	expiresAt := time.Now().Add(time.Hour)
	scheduler.Schedule("test/cancel", expiresAt)
	scheduler.Cancel("test/cancel")

	if scheduler.Pending() != 0 {
		t.Errorf("expected 0 pending after cancel, got %d", scheduler.Pending())
	}
}

func TestRefreshSchedulerUpdateExisting(t *testing.T) {
	config := &RefreshConfig{
		Enabled:              true,
		RefreshThreshold:     0.75,
		CheckInterval:        time.Second,
		MaxConcurrentRefresh: 5,
	}

	scheduler := NewRefreshScheduler(nil, config)

	scheduler.Schedule("test/update", time.Now().Add(time.Hour))
	scheduler.Schedule("test/update", time.Now().Add(2*time.Hour))

	if scheduler.Pending() != 1 {
		t.Errorf("expected 1 pending after update, got %d", scheduler.Pending())
	}
}

func TestRefreshSchedulerZeroExpiry(t *testing.T) {
	config := &RefreshConfig{
		Enabled:              true,
		RefreshThreshold:     0.75,
		CheckInterval:        time.Second,
		MaxConcurrentRefresh: 5,
	}

	scheduler := NewRefreshScheduler(nil, config)
	scheduler.Schedule("test/zero", time.Time{})

	if scheduler.Pending() != 0 {
		t.Errorf("expected 0 pending for zero expiry, got %d", scheduler.Pending())
	}
}

func TestRefreshSchedulerPastExpiry(t *testing.T) {
	config := &RefreshConfig{
		Enabled:              true,
		RefreshThreshold:     0.75,
		CheckInterval:        time.Second,
		MaxConcurrentRefresh: 5,
	}

	scheduler := NewRefreshScheduler(nil, config)
	scheduler.Schedule("test/past", time.Now().Add(-time.Hour))

	if scheduler.Pending() != 0 {
		t.Errorf("expected 0 pending for past expiry, got %d", scheduler.Pending())
	}
}

func TestRefreshSchedulerDefaultConfig(t *testing.T) {
	// nil config should use defaults
	scheduler := NewRefreshScheduler(nil, nil)

	if scheduler.Pending() != 0 {
		t.Errorf("expected 0 pending initially")
	}

	expiresAt := time.Now().Add(time.Hour)
	scheduler.Schedule("test/default", expiresAt)

	if scheduler.Pending() != 1 {
		t.Errorf("expected 1 pending")
	}
}

// =============================================================================
// Request Batcher Tests
// =============================================================================

func TestRequestBatcherPending(t *testing.T) {
	config := &BatchConfig{
		Enabled:      true,
		MaxBatchSize: 10,
		BatchTimeout: 100 * time.Millisecond,
	}

	batcher := NewRequestBatcher(nil, config)

	if batcher.Pending() != 0 {
		t.Errorf("expected 0 pending initially, got %d", batcher.Pending())
	}
}

func TestRequestBatcherDefaultConfig(t *testing.T) {
	// nil config should use defaults
	batcher := NewRequestBatcher(nil, nil)

	if batcher.Pending() != 0 {
		t.Errorf("expected 0 pending initially")
	}
}

// =============================================================================
// Broker Client Config Tests
// =============================================================================

func TestDefaultBrokerClientConfig(t *testing.T) {
	config := DefaultBrokerClientConfig()

	if config == nil {
		t.Fatal("DefaultBrokerClientConfig returned nil")
	}

	if len(config.NATSURLs) == 0 {
		t.Error("should have default NATS URLs")
	}

	if config.RequestTimeout <= 0 {
		t.Error("RequestTimeout should be positive")
	}

	if config.SubjectPrefix == "" {
		t.Error("SubjectPrefix should not be empty")
	}
}

func TestNATSBrokerClientCreation(t *testing.T) {
	config := DefaultBrokerClientConfig()
	client, err := NewNATSBrokerClient(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	if client == nil {
		t.Fatal("client should not be nil")
	}

	if client.Healthy() {
		t.Error("client should not be healthy before connect")
	}
}

func TestNATSBrokerClientNoURLs(t *testing.T) {
	config := &BrokerClientConfig{
		NATSURLs: []string{},
	}

	_, err := NewNATSBrokerClient(config)
	if err == nil {
		t.Error("expected error for empty NATS URLs")
	}
}

func TestNATSBrokerClientNilConfig(t *testing.T) {
	client, err := NewNATSBrokerClient(nil)
	if err != nil {
		t.Fatalf("failed with nil config: %v", err)
	}
	if client == nil {
		t.Fatal("client should not be nil")
	}
}

func TestNATSBrokerClientStats(t *testing.T) {
	config := DefaultBrokerClientConfig()
	client, _ := NewNATSBrokerClient(config)

	stats := client.Stats()
	if stats.ConnectAttempts != 0 {
		t.Errorf("initial connect attempts should be 0")
	}
}

// =============================================================================
// SPIFFE Authorizer Tests
// =============================================================================

func TestSPIFFEAuthorizerTrustDomain(t *testing.T) {
	auth := NewSPIFFEAuthorizer()
	auth.AddTrustDomain("example.org")

	tests := []struct {
		name     string
		trustDom string
		path     string
		wantErr  bool
	}{
		{
			name:     "allowed trust domain",
			trustDom: "example.org",
			path:     "/secret/test",
			wantErr:  false,
		},
		{
			name:     "disallowed trust domain",
			trustDom: "other.org",
			path:     "/secret/test",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := mockSPIFFEID(tt.trustDom, "/workload")
			err := auth.Authorize(id, tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("Authorize() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSPIFFEAuthorizerPathPolicy(t *testing.T) {
	auth := NewSPIFFEAuthorizer()
	auth.AddTrustDomain("example.org")
	auth.AddPathPolicy("/secrets/app/", []string{
		"spiffe://example.org/app/*",
	})

	tests := []struct {
		name     string
		trustDom string
		workload string
		path     string
		wantErr  bool
	}{
		{
			name:     "matching path and workload",
			trustDom: "example.org",
			workload: "/app/frontend",
			path:     "/secrets/app/database",
			wantErr:  false,
		},
		{
			name:     "mismatched workload",
			trustDom: "example.org",
			workload: "/other/workload",
			path:     "/secrets/app/database",
			wantErr:  true,
		},
		{
			name:     "no policy for path (allowed)",
			trustDom: "example.org",
			workload: "/any/workload",
			path:     "/secrets/other/data",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := mockSPIFFEID(tt.trustDom, tt.workload)
			err := auth.Authorize(id, tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("Authorize() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSPIFFEAuthorizerWildcard(t *testing.T) {
	auth := NewSPIFFEAuthorizer()
	auth.AddPathPolicy("/secrets/", []string{"*"})

	id := mockSPIFFEID("any.domain", "/any/workload")
	err := auth.Authorize(id, "/secrets/anything")
	if err != nil {
		t.Errorf("wildcard should allow: %v", err)
	}
}

func TestSPIFFEAuthorizerNoTrustDomainRestriction(t *testing.T) {
	auth := NewSPIFFEAuthorizer()
	// No trust domains added = all allowed

	id := mockSPIFFEID("any.domain", "/workload")
	err := auth.Authorize(id, "/secrets/test")
	if err != nil {
		t.Errorf("no trust domain restriction should allow all: %v", err)
	}
}

func TestSPIFFEAuthorizerExactMatch(t *testing.T) {
	auth := NewSPIFFEAuthorizer()
	auth.AddPathPolicy("/secrets/specific/", []string{
		"spiffe://example.org/specific/workload",
	})

	// Exact match should work
	id := mockSPIFFEID("example.org", "/specific/workload")
	err := auth.Authorize(id, "/secrets/specific/data")
	if err != nil {
		t.Errorf("exact match should work: %v", err)
	}
}

// =============================================================================
// Helper Functions
// =============================================================================

func mockSPIFFEID(trustDomain, path string) identity.SPIFFEID {
	return identity.SPIFFEID{
		TrustDomain: trustDomain,
		Path:        path,
	}
}

// =============================================================================
// Integration-Style Tests
// =============================================================================

func TestMemoryCacheWithRealSecrets(t *testing.T) {
	cache := NewMemoryCache(100, time.Hour)

	secretPaths := []string{
		"database/production/credentials",
		"api/external/token",
		"certificates/tls/server",
		"config/app/settings",
	}

	for _, path := range secretPaths {
		secret := &secrets.Secret{
			Path:      path,
			Version:   1,
			CreatedAt: time.Now(),
			Data: map[string]interface{}{
				"value": "secret-" + path,
			},
		}
		cache.Set(path, secret)
	}

	for _, path := range secretPaths {
		got, found := cache.Get(path)
		if !found {
			t.Errorf("failed to get %s", path)
			continue
		}
		if got.Path != path {
			t.Errorf("path mismatch: got %s, want %s", got.Path, path)
		}
	}
}

func TestDiskCacheWithComplexData(t *testing.T) {
	dir := t.TempDir()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i * 7)
	}

	cache, err := NewDiskCache(dir, key)
	if err != nil {
		t.Fatalf("failed to create disk cache: %v", err)
	}

	secret := &secrets.Secret{
		Path:    "complex/nested",
		Version: 5,
		Data: map[string]interface{}{
			"string":  "value",
			"number":  float64(42),
			"boolean": true,
			"nested": map[string]interface{}{
				"inner": "data",
			},
			"array": []interface{}{"a", "b", "c"},
		},
		Metadata: map[string]string{
			"owner":   "admin",
			"project": "test",
		},
	}

	if err := cache.Set("complex/nested", secret); err != nil {
		t.Fatalf("failed to set complex secret: %v", err)
	}

	got, err := cache.Get("complex/nested")
	if err != nil {
		t.Fatalf("failed to get complex secret: %v", err)
	}

	if got.Version != secret.Version {
		t.Errorf("version mismatch: got %d, want %d", got.Version, secret.Version)
	}
}

// =============================================================================
// Benchmark Tests
// =============================================================================

func BenchmarkMemoryCacheSet(b *testing.B) {
	cache := NewMemoryCache(10000, time.Hour)
	secret := &secrets.Secret{
		Path: "bench/path",
		Data: map[string]interface{}{"key": "value"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Set("bench/path", secret)
	}
}

func BenchmarkMemoryCacheGet(b *testing.B) {
	cache := NewMemoryCache(10000, time.Hour)
	secret := &secrets.Secret{
		Path: "bench/path",
		Data: map[string]interface{}{"key": "value"},
	}
	cache.Set("bench/path", secret)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get("bench/path")
	}
}

func BenchmarkDiskCacheSet(b *testing.B) {
	dir := b.TempDir()
	key := make([]byte, 32)
	cache, _ := NewDiskCache(dir, key)
	secret := &secrets.Secret{
		Path: "bench/disk",
		Data: map[string]interface{}{"key": "value"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Set("bench/disk", secret)
	}
}

func BenchmarkDiskCacheGet(b *testing.B) {
	dir := b.TempDir()
	key := make([]byte, 32)
	cache, _ := NewDiskCache(dir, key)
	secret := &secrets.Secret{
		Path: "bench/disk",
		Data: map[string]interface{}{"key": "value"},
	}
	cache.Set("bench/disk", secret)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get("bench/disk")
	}
}
