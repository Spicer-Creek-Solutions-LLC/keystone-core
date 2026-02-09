package secrets

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// Ensure interfaces are implemented correctly.
var (
	_ SecretBackend     = (*mockBackend)(nil)
	_ SecretCache       = (*EncryptedSecretCache)(nil)
	_ SecretCache       = (*InMemorySecretCache)(nil)
	_ LeaseManager      = (*InMemoryLeaseManager)(nil)
	_ SecretAuditLogger = (*InMemorySecretAuditLogger)(nil)
	_ SecretAuditLogger = (*NoopSecretAuditLogger)(nil)
	_ SecretAuditLogger = (*MultiSecretAuditLogger)(nil)
	_ SecretAuditLogger = (*ChannelSecretAuditLogger)(nil)
)

// mockBackend is a mock implementation of SecretBackend for testing.
type mockBackend struct {
	name        string
	backendType BackendType
	healthy     bool
	secrets     map[string]*Secret
	leases      map[string]*Lease
}

func newMockBackend(name string, backendType BackendType) *mockBackend {
	return &mockBackend{
		name:        name,
		backendType: backendType,
		healthy:     true,
		secrets:     make(map[string]*Secret),
		leases:      make(map[string]*Lease),
	}
}

func (m *mockBackend) Type() BackendType                { return m.backendType }
func (m *mockBackend) Name() string                     { return m.name }
func (m *mockBackend) Healthy(ctx context.Context) bool { return m.healthy }

func (m *mockBackend) Read(ctx context.Context, req *SecretRequest) (*Secret, error) {
	secret, ok := m.secrets[req.Path]
	if !ok {
		return nil, ErrSecretNotFound
	}
	return secret, nil
}

func (m *mockBackend) ReadDynamic(ctx context.Context, req *SecretRequest) (*Secret, error) {
	secret := &Secret{
		Path:    req.Path,
		Backend: m.backendType,
		Type:    SecretTypeDynamic,
		Data: map[string]interface{}{
			"username": "dynamic-user",
			"password": "dynamic-pass",
		},
		Lease: &Lease{
			ID:        "lease-" + req.Path,
			TTL:       req.TTL,
			Renewable: req.Renewable,
			IssuedAt:  time.Now(),
			ExpiresAt: time.Now().Add(req.TTL),
		},
	}
	m.leases[secret.Lease.ID] = secret.Lease
	return secret, nil
}

func (m *mockBackend) List(ctx context.Context, prefix string) ([]string, error) {
	paths := make([]string, 0, len(m.secrets))
	for path := range m.secrets {
		paths = append(paths, path)
	}
	return paths, nil
}

func (m *mockBackend) RenewLease(ctx context.Context, leaseID string, increment time.Duration) (*Lease, error) {
	lease, ok := m.leases[leaseID]
	if !ok {
		return nil, ErrLeaseNotFound
	}
	lease.ExpiresAt = time.Now().Add(increment)
	lease.TTL = increment
	return lease, nil
}

func (m *mockBackend) RevokeLease(ctx context.Context, leaseID string) error {
	delete(m.leases, leaseID)
	return nil
}

func (m *mockBackend) Close() error { return nil }

func (m *mockBackend) addSecret(path string, data map[string]interface{}) {
	m.secrets[path] = &Secret{
		Path:    path,
		Backend: m.backendType,
		Type:    SecretTypeStatic,
		Data:    data,
	}
}

func TestBackendType_Valid(t *testing.T) {
	tests := []struct {
		backendType BackendType
		want        bool
	}{
		{BackendTypeVault, true},
		{BackendTypeAWS, true},
		{BackendTypeAzure, true},
		{BackendTypeGCP, true},
		{BackendType("invalid"), false},
		{BackendType(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.backendType), func(t *testing.T) {
			if got := tt.backendType.Valid(); got != tt.want {
				t.Errorf("BackendType.Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSecret_Get(t *testing.T) {
	secret := &Secret{
		Data: map[string]interface{}{
			"username": "admin",
			"password": "secret123",
			"port":     5432,
		},
	}

	t.Run("get existing key", func(t *testing.T) {
		val, ok := secret.Get("username")
		if !ok {
			t.Error("expected to find key 'username'")
		}
		if val != "admin" {
			t.Errorf("expected 'admin', got %v", val)
		}
	})

	t.Run("get missing key", func(t *testing.T) {
		_, ok := secret.Get("nonexistent")
		if ok {
			t.Error("expected not to find key 'nonexistent'")
		}
	})

	t.Run("get string value", func(t *testing.T) {
		val, ok := secret.GetString("username")
		if !ok {
			t.Error("expected to find key 'username'")
		}
		if val != "admin" {
			t.Errorf("expected 'admin', got %v", val)
		}
	})

	t.Run("get non-string as string", func(t *testing.T) {
		_, ok := secret.GetString("port")
		if ok {
			t.Error("expected GetString to fail for non-string value")
		}
	})

	t.Run("nil data", func(t *testing.T) {
		nilSecret := &Secret{}
		_, ok := nilSecret.Get("any")
		if ok {
			t.Error("expected not to find key in nil data")
		}
	})
}

func TestSecret_IsExpired(t *testing.T) {
	t.Run("no expiry", func(t *testing.T) {
		secret := &Secret{}
		if secret.IsExpired() {
			t.Error("secret with no expiry should not be expired")
		}
	})

	t.Run("future expiry", func(t *testing.T) {
		secret := &Secret{ExpiresAt: time.Now().Add(time.Hour)}
		if secret.IsExpired() {
			t.Error("secret with future expiry should not be expired")
		}
	})

	t.Run("past expiry", func(t *testing.T) {
		secret := &Secret{ExpiresAt: time.Now().Add(-time.Hour)}
		if !secret.IsExpired() {
			t.Error("secret with past expiry should be expired")
		}
	})
}

func TestLease_NeedsRenewal(t *testing.T) {
	tests := []struct {
		name      string
		lease     *Lease
		threshold float64
		want      bool
	}{
		{
			name: "needs renewal at 50%",
			lease: &Lease{
				TTL:       time.Hour,
				ExpiresAt: time.Now().Add(20 * time.Minute), // 33% remaining
				Renewable: true,
			},
			threshold: 0.5,
			want:      true,
		},
		{
			name: "does not need renewal",
			lease: &Lease{
				TTL:       time.Hour,
				ExpiresAt: time.Now().Add(45 * time.Minute), // 75% remaining
				Renewable: true,
			},
			threshold: 0.5,
			want:      false,
		},
		{
			name: "not renewable",
			lease: &Lease{
				TTL:       time.Hour,
				ExpiresAt: time.Now().Add(10 * time.Minute),
				Renewable: false,
			},
			threshold: 0.5,
			want:      false,
		},
		{
			name: "already expired",
			lease: &Lease{
				TTL:       time.Hour,
				ExpiresAt: time.Now().Add(-time.Minute),
				Renewable: true,
			},
			threshold: 0.5,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.lease.NeedsRenewal(tt.threshold); got != tt.want {
				t.Errorf("Lease.NeedsRenewal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRenewalStrategy_Threshold(t *testing.T) {
	tests := []struct {
		strategy RenewalStrategy
		want     float64
	}{
		{RenewalStrategyEager, 0.5},
		{RenewalStrategyLazy, 0.9},
		{RenewalStrategyOnDemand, 1.0},
		{RenewalStrategy("unknown"), 0.5}, // default
	}

	for _, tt := range tests {
		t.Run(string(tt.strategy), func(t *testing.T) {
			if got := tt.strategy.Threshold(); got != tt.want {
				t.Errorf("RenewalStrategy.Threshold() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSecretBroker_RegisterBackend(t *testing.T) {
	broker := NewSecretBroker(nil)

	// Test registering a backend
	backend := newMockBackend("vault", BackendTypeVault)
	err := broker.RegisterBackend("vault", backend)
	if err != nil {
		t.Fatalf("failed to register backend: %v", err)
	}

	// Test first backend becomes default
	backends := broker.ListBackends()
	if len(backends) != 1 || backends[0] != "vault" {
		t.Errorf("expected ['vault'], got %v", backends)
	}

	// Test registering second backend
	awsBackend := newMockBackend("aws", BackendTypeAWS)
	err = broker.RegisterBackend("aws", awsBackend)
	if err != nil {
		t.Fatalf("failed to register second backend: %v", err)
	}

	backends = broker.ListBackends()
	if len(backends) != 2 {
		t.Errorf("expected 2 backends, got %d", len(backends))
	}

	// Test empty name error
	err = broker.RegisterBackend("", backend)
	if err == nil {
		t.Error("expected error for empty name")
	}

	// Test nil backend error
	err = broker.RegisterBackend("test", nil)
	if err == nil {
		t.Error("expected error for nil backend")
	}
}

func TestSecretBroker_UnregisterBackend(t *testing.T) {
	broker := NewSecretBroker(nil)

	backend := newMockBackend("vault", BackendTypeVault)
	_ = broker.RegisterBackend("vault", backend)

	// Test unregistering
	err := broker.UnregisterBackend("vault")
	if err != nil {
		t.Fatalf("failed to unregister backend: %v", err)
	}

	backends := broker.ListBackends()
	if len(backends) != 0 {
		t.Errorf("expected 0 backends, got %d", len(backends))
	}

	// Test unregistering non-existent
	err = broker.UnregisterBackend("nonexistent")
	if !errors.Is(err, ErrBackendNotFound) {
		t.Errorf("expected ErrBackendNotFound, got %v", err)
	}
}

func TestSecretBroker_Read(t *testing.T) {
	broker := NewSecretBroker(&BrokerConfig{
		Cache: &CacheConfig{Enabled: false},
	})

	backend := newMockBackend("vault", BackendTypeVault)
	backend.addSecret("kv/myapp/config", map[string]interface{}{
		"database_url": "postgres://localhost:5432/myapp",
	})
	_ = broker.RegisterBackend("vault", backend)

	ctx := context.Background()

	// Test reading with backend prefix
	secret, err := broker.Read(ctx, &SecretRequest{Path: "vault/kv/myapp/config"})
	if err != nil {
		t.Fatalf("failed to read secret: %v", err)
	}

	if secret.Path != "vault/kv/myapp/config" {
		t.Errorf("expected path 'vault/kv/myapp/config', got %s", secret.Path)
	}

	dbURL, ok := secret.GetString("database_url")
	if !ok || dbURL != "postgres://localhost:5432/myapp" {
		t.Errorf("unexpected database_url: %s", dbURL)
	}

	// Test reading non-existent secret
	_, err = broker.Read(ctx, &SecretRequest{Path: "vault/kv/nonexistent"})
	if !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("expected ErrSecretNotFound, got %v", err)
	}

	// Test invalid path
	_, err = broker.Read(ctx, &SecretRequest{Path: ""})
	if !errors.Is(err, ErrInvalidPath) {
		t.Errorf("expected ErrInvalidPath, got %v", err)
	}
}

func TestSecretBroker_ReadWithRouting(t *testing.T) {
	broker := NewSecretBroker(&BrokerConfig{
		Routing: []RoutingRule{
			{Prefix: "prod/", Backend: "vault"},
			{Prefix: "aws/", Backend: "aws"},
		},
	})

	vaultBackend := newMockBackend("vault", BackendTypeVault)
	vaultBackend.addSecret("secrets/db", map[string]interface{}{"password": "vault-secret"})
	_ = broker.RegisterBackend("vault", vaultBackend)

	awsBackend := newMockBackend("aws", BackendTypeAWS)
	awsBackend.addSecret("secrets/db", map[string]interface{}{"password": "aws-secret"})
	_ = broker.RegisterBackend("aws", awsBackend)

	ctx := context.Background()

	// Test routing by prefix
	secret, err := broker.Read(ctx, &SecretRequest{Path: "prod/secrets/db"})
	if err != nil {
		t.Fatalf("failed to read from vault: %v", err)
	}
	if pwd, _ := secret.GetString("password"); pwd != "vault-secret" {
		t.Errorf("expected vault-secret, got %s", pwd)
	}

	secret, err = broker.Read(ctx, &SecretRequest{Path: "aws/secrets/db"})
	if err != nil {
		t.Fatalf("failed to read from aws: %v", err)
	}
	if pwd, _ := secret.GetString("password"); pwd != "aws-secret" {
		t.Errorf("expected aws-secret, got %s", pwd)
	}
}

func TestSecretBroker_ReadDynamic(t *testing.T) {
	broker := NewSecretBroker(nil)

	backend := newMockBackend("vault", BackendTypeVault)
	_ = broker.RegisterBackend("vault", backend)

	leaseManager := NewInMemoryLeaseManager(nil)
	broker.SetLeaseManager(leaseManager)

	ctx := context.Background()

	// Test reading dynamic secret
	secret, err := broker.ReadDynamic(ctx, &SecretRequest{
		Path:      "vault/database/creds/app",
		TTL:       time.Hour,
		Renewable: true,
	})
	if err != nil {
		t.Fatalf("failed to read dynamic secret: %v", err)
	}

	if secret.Type != SecretTypeDynamic {
		t.Errorf("expected dynamic secret type, got %s", secret.Type)
	}

	if !secret.HasLease() {
		t.Error("expected secret to have a lease")
	}

	// Verify lease was tracked
	leases, _ := leaseManager.List(ctx)
	if len(leases) != 1 {
		t.Errorf("expected 1 tracked lease, got %d", len(leases))
	}
}

func TestSecretBroker_BackendHealth(t *testing.T) {
	broker := NewSecretBroker(nil)

	healthyBackend := newMockBackend("healthy", BackendTypeVault)
	healthyBackend.healthy = true
	_ = broker.RegisterBackend("healthy", healthyBackend)

	unhealthyBackend := newMockBackend("unhealthy", BackendTypeAWS)
	unhealthyBackend.healthy = false
	_ = broker.RegisterBackend("unhealthy", unhealthyBackend)

	ctx := context.Background()

	// Test overall health (at least one backend healthy)
	if !broker.Healthy(ctx) {
		t.Error("broker should be healthy with at least one healthy backend")
	}

	// Test individual backend health
	health := broker.BackendHealth(ctx)
	if !health["healthy"] {
		t.Error("expected 'healthy' backend to be healthy")
	}
	if health["unhealthy"] {
		t.Error("expected 'unhealthy' backend to be unhealthy")
	}

	// Test reading from unhealthy backend
	unhealthyBackend.addSecret("test", map[string]interface{}{"key": "value"})
	_, err := broker.Read(ctx, &SecretRequest{Path: "unhealthy/test"})
	if err == nil {
		t.Error("expected error when reading from unhealthy backend")
	}
}

func TestInMemorySecretCache(t *testing.T) {
	cache := NewInMemorySecretCache(&CacheConfig{
		MaxEntries:      10,
		DefaultTTL:      time.Minute,
		CleanupInterval: time.Hour, // Long interval for testing
	})
	defer cache.Close()

	ctx := context.Background()

	// Test put and get
	secret := &Secret{
		Path: "test/secret",
		Data: map[string]interface{}{"key": "value"},
	}

	err := cache.Put(ctx, secret, time.Minute)
	if err != nil {
		t.Fatalf("failed to put secret: %v", err)
	}

	cached, ok := cache.Get(ctx, "test/secret")
	if !ok {
		t.Fatal("expected to find cached secret")
	}
	if v, _ := cached.GetString("key"); v != "value" {
		t.Errorf("expected 'value', got %s", v)
	}

	// Test cache miss
	_, ok = cache.Get(ctx, "nonexistent")
	if ok {
		t.Error("expected cache miss for nonexistent key")
	}

	// Test stats
	stats := cache.Stats()
	if stats.Hits != 1 {
		t.Errorf("expected 1 hit, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("expected 1 miss, got %d", stats.Misses)
	}

	// Test delete
	err = cache.Delete(ctx, "test/secret")
	if err != nil {
		t.Fatalf("failed to delete: %v", err)
	}

	_, ok = cache.Get(ctx, "test/secret")
	if ok {
		t.Error("expected cache miss after delete")
	}
}

func TestInMemorySecretCache_Expiration(t *testing.T) {
	cache := NewInMemorySecretCache(&CacheConfig{
		MaxEntries:      10,
		CleanupInterval: time.Hour,
	})
	defer cache.Close()

	ctx := context.Background()

	// Put with very short TTL
	secret := &Secret{
		Path: "test/expiring",
		Data: map[string]interface{}{"key": "value"},
	}

	err := cache.Put(ctx, secret, time.Millisecond)
	if err != nil {
		t.Fatalf("failed to put secret: %v", err)
	}

	// Wait for expiration
	time.Sleep(10 * time.Millisecond)

	_, ok := cache.Get(ctx, "test/expiring")
	if ok {
		t.Error("expected cache miss for expired secret")
	}
}

func TestInMemorySecretCache_LRUEviction(t *testing.T) {
	cache := NewInMemorySecretCache(&CacheConfig{
		MaxEntries:      3,
		CleanupInterval: time.Hour,
	})
	defer cache.Close()

	ctx := context.Background()

	// Fill cache
	for i := 0; i < 3; i++ {
		secret := &Secret{
			Path: "test/secret" + string(rune('0'+i)),
			Data: map[string]interface{}{"key": i},
		}
		_ = cache.Put(ctx, secret, time.Hour)
		time.Sleep(time.Millisecond) // Ensure different access times
	}

	// Access first secret to make it recent
	_, _ = cache.Get(ctx, "test/secret0")
	time.Sleep(time.Millisecond)

	// Add new secret, should evict least recently used (secret1)
	secret := &Secret{
		Path: "test/secret3",
		Data: map[string]interface{}{"key": 3},
	}
	_ = cache.Put(ctx, secret, time.Hour)

	// secret1 should be evicted (oldest access)
	_, ok := cache.Get(ctx, "test/secret1")
	if ok {
		t.Error("expected secret1 to be evicted")
	}

	// secret0 should still exist (was accessed recently)
	_, ok = cache.Get(ctx, "test/secret0")
	if !ok {
		t.Error("expected secret0 to still exist")
	}
}

func TestInMemorySecretCache_DeleteByPrefix(t *testing.T) {
	cache := NewInMemorySecretCache(&CacheConfig{
		MaxEntries:      10,
		CleanupInterval: time.Hour,
	})
	defer cache.Close()

	ctx := context.Background()

	// Add secrets with different prefixes
	for _, path := range []string{"app/db", "app/redis", "other/secret"} {
		_ = cache.Put(ctx, &Secret{Path: path, Data: map[string]interface{}{}}, time.Hour)
	}

	// Delete by prefix
	count, err := cache.DeleteByPrefix(ctx, "app/")
	if err != nil {
		t.Fatalf("failed to delete by prefix: %v", err)
	}
	if count != 2 {
		t.Errorf("expected to delete 2, got %d", count)
	}

	// Verify
	_, ok := cache.Get(ctx, "app/db")
	if ok {
		t.Error("expected app/db to be deleted")
	}

	_, ok = cache.Get(ctx, "other/secret")
	if !ok {
		t.Error("expected other/secret to still exist")
	}
}

func TestEncryptedSecretCache(t *testing.T) {
	key, err := GenerateCacheKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	cache, err := NewEncryptedSecretCache(&CacheConfig{
		MaxEntries:      10,
		CleanupInterval: time.Hour,
	}, key)
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Close()

	ctx := context.Background()

	// Test put and get with encryption
	secret := &Secret{
		Path: "test/encrypted",
		Data: map[string]interface{}{
			"password": "super-secret",
			"api_key":  "abc123",
		},
	}

	err = cache.Put(ctx, secret, time.Minute)
	if err != nil {
		t.Fatalf("failed to put secret: %v", err)
	}

	cached, ok := cache.Get(ctx, "test/encrypted")
	if !ok {
		t.Fatal("expected to find cached secret")
	}

	password, ok := cached.GetString("password")
	if !ok || password != "super-secret" {
		t.Errorf("expected 'super-secret', got %s", password)
	}

	apiKey, ok := cached.GetString("api_key")
	if !ok || apiKey != "abc123" {
		t.Errorf("expected 'abc123', got %s", apiKey)
	}
}

func TestEncryptedSecretCache_RequiresKey(t *testing.T) {
	_, err := NewEncryptedSecretCache(nil, nil)
	if err == nil {
		t.Error("expected error when creating cache without key")
	}

	_, err = NewEncryptedSecretCache(nil, []byte{})
	if err == nil {
		t.Error("expected error when creating cache with empty key")
	}
}

func TestInMemoryLeaseManager(t *testing.T) {
	manager := NewInMemoryLeaseManager(nil)
	ctx := context.Background()

	// Test tracking a lease
	lease := &Lease{
		ID:         "lease-123",
		SecretPath: "vault/database/creds/app",
		Backend:    BackendTypeVault,
		TTL:        time.Hour,
		ExpiresAt:  time.Now().Add(time.Hour),
		Renewable:  true,
	}

	err := manager.Track(ctx, lease)
	if err != nil {
		t.Fatalf("failed to track lease: %v", err)
	}

	// Test getting lease
	tracked, err := manager.Get(ctx, "lease-123")
	if err != nil {
		t.Fatalf("failed to get lease: %v", err)
	}
	if tracked.SecretPath != lease.SecretPath {
		t.Errorf("expected path %s, got %s", lease.SecretPath, tracked.SecretPath)
	}

	// Test listing
	leases, err := manager.List(ctx)
	if err != nil {
		t.Fatalf("failed to list leases: %v", err)
	}
	if len(leases) != 1 {
		t.Errorf("expected 1 lease, got %d", len(leases))
	}

	// Test list by path
	leases, err = manager.ListByPath(ctx, "vault/database/creds/app")
	if err != nil {
		t.Fatalf("failed to list by path: %v", err)
	}
	if len(leases) != 1 {
		t.Errorf("expected 1 lease, got %d", len(leases))
	}

	// Test list by backend
	leases, err = manager.ListByBackend(ctx, BackendTypeVault)
	if err != nil {
		t.Fatalf("failed to list by backend: %v", err)
	}
	if len(leases) != 1 {
		t.Errorf("expected 1 lease, got %d", len(leases))
	}

	// Test stats
	stats, err := manager.Stats(ctx)
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}
	if stats.ActiveLeases != 1 {
		t.Errorf("expected 1 active lease, got %d", stats.ActiveLeases)
	}
}

func TestInMemoryLeaseManager_Revoke(t *testing.T) {
	manager := NewInMemoryLeaseManager(nil)
	ctx := context.Background()

	lease := &Lease{
		ID:         "lease-456",
		SecretPath: "vault/kv/secret",
		Backend:    BackendTypeVault,
		TTL:        time.Hour,
		ExpiresAt:  time.Now().Add(time.Hour),
	}

	_ = manager.Track(ctx, lease)

	// Revoke the lease
	err := manager.Revoke(ctx, "lease-456")
	if err != nil {
		t.Fatalf("failed to revoke lease: %v", err)
	}

	// Verify it's marked as revoked
	revoked, _ := manager.Get(ctx, "lease-456")
	if revoked.State != LeaseStateRevoked {
		t.Errorf("expected revoked state, got %s", revoked.State)
	}

	// Test revoke non-existent
	err = manager.Revoke(ctx, "nonexistent")
	if !errors.Is(err, ErrLeaseNotFound) {
		t.Errorf("expected ErrLeaseNotFound, got %v", err)
	}
}

func TestInMemoryLeaseManager_RevokeByPath(t *testing.T) {
	manager := NewInMemoryLeaseManager(nil)
	ctx := context.Background()

	// Add multiple leases
	for i := 0; i < 3; i++ {
		lease := &Lease{
			ID:         "lease-" + string(rune('a'+i)),
			SecretPath: "vault/database/creds/app",
			Backend:    BackendTypeVault,
			TTL:        time.Hour,
			ExpiresAt:  time.Now().Add(time.Hour),
		}
		_ = manager.Track(ctx, lease)
	}

	// Add lease with different path
	_ = manager.Track(ctx, &Lease{
		ID:         "lease-other",
		SecretPath: "vault/kv/other",
		Backend:    BackendTypeVault,
		TTL:        time.Hour,
		ExpiresAt:  time.Now().Add(time.Hour),
	})

	// Revoke by path prefix
	count, err := manager.RevokeByPath(ctx, "vault/database/")
	if err != nil {
		t.Fatalf("failed to revoke by path: %v", err)
	}
	if count != 3 {
		t.Errorf("expected to revoke 3, got %d", count)
	}

	// Verify stats
	stats, _ := manager.Stats(ctx)
	if stats.RevokedLeases != 3 {
		t.Errorf("expected 3 revoked, got %d", stats.RevokedLeases)
	}
	if stats.ActiveLeases != 1 {
		t.Errorf("expected 1 active, got %d", stats.ActiveLeases)
	}
}

func TestInMemorySecretAuditLogger(t *testing.T) {
	callback := make(chan *SecretAccessEvent, 10)
	logger := NewInMemorySecretAuditLogger(&InMemoryAuditLoggerConfig{
		MaxSize: 100,
		Callback: func(event *SecretAccessEvent) {
			callback <- event
		},
	})

	ctx := context.Background()

	// Log an event
	event := &SecretAccessEvent{
		SecretPath: "vault/kv/test",
		AgentID:    "agent-001",
		Action:     AuditActionRead,
		Timestamp:  time.Now(),
		Success:    true,
	}

	err := logger.LogSecretAccess(ctx, event)
	if err != nil {
		t.Fatalf("failed to log event: %v", err)
	}

	// Verify callback was called
	select {
	case received := <-callback:
		if received.SecretPath != event.SecretPath {
			t.Errorf("callback received wrong event")
		}
	case <-time.After(time.Second):
		t.Error("expected callback to be called")
	}

	// Verify event was stored
	events := logger.GetEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}

	// Test filtering by path
	events = logger.GetEventsByPath("vault/kv/test")
	if len(events) != 1 {
		t.Errorf("expected 1 event for path, got %d", len(events))
	}

	events = logger.GetEventsByPath("other/path")
	if len(events) != 0 {
		t.Errorf("expected 0 events for other path, got %d", len(events))
	}

	// Test filtering by agent
	events = logger.GetEventsByAgent("agent-001")
	if len(events) != 1 {
		t.Errorf("expected 1 event for agent, got %d", len(events))
	}

	// Test count
	if logger.GetEventCount() != 1 {
		t.Errorf("expected count 1, got %d", logger.GetEventCount())
	}

	// Test clear
	logger.Clear()
	if logger.GetEventCount() != 0 {
		t.Errorf("expected count 0 after clear, got %d", logger.GetEventCount())
	}
}

func TestInMemorySecretAuditLogger_MaxSize(t *testing.T) {
	logger := NewInMemorySecretAuditLogger(&InMemoryAuditLoggerConfig{
		MaxSize: 3,
	})

	ctx := context.Background()

	// Add more events than max size
	for i := 0; i < 5; i++ {
		_ = logger.LogSecretAccess(ctx, &SecretAccessEvent{
			SecretPath: "secret-" + string(rune('0'+i)),
			Timestamp:  time.Now(),
		})
	}

	// Verify only last 3 are kept (ring buffer)
	events := logger.GetEvents()
	if len(events) != 3 {
		t.Errorf("expected 3 events, got %d", len(events))
	}

	// First event should be secret-2 (oldest after eviction)
	if events[0].SecretPath != "secret-2" {
		t.Errorf("expected first event to be secret-2, got %s", events[0].SecretPath)
	}
}

func TestInMemorySecretAuditLogger_Summary(t *testing.T) {
	logger := NewInMemorySecretAuditLogger(nil)
	ctx := context.Background()

	// Add mix of events
	events := []*SecretAccessEvent{
		{SecretPath: "secret/a", Action: AuditActionRead, Success: true, CacheHit: false, Timestamp: time.Now()},
		{SecretPath: "secret/a", Action: AuditActionRead, Success: true, CacheHit: true, Timestamp: time.Now()},
		{SecretPath: "secret/b", Action: AuditActionRead, Success: false, CacheHit: false, Timestamp: time.Now()},
		{SecretPath: "secret/a", Action: AuditActionLeaseRenew, Success: true, Timestamp: time.Now()},
	}

	for _, e := range events {
		_ = logger.LogSecretAccess(ctx, e)
	}

	summary := logger.GetSummary()

	if summary.TotalEvents != 4 {
		t.Errorf("expected 4 total events, got %d", summary.TotalEvents)
	}
	if summary.SuccessCount != 3 {
		t.Errorf("expected 3 successes, got %d", summary.SuccessCount)
	}
	if summary.FailureCount != 1 {
		t.Errorf("expected 1 failure, got %d", summary.FailureCount)
	}
	if summary.CacheHits != 1 {
		t.Errorf("expected 1 cache hit, got %d", summary.CacheHits)
	}
	if summary.EventsByAction[AuditActionRead] != 3 {
		t.Errorf("expected 3 read events, got %d", summary.EventsByAction[AuditActionRead])
	}
	if summary.TopSecrets["secret/a"] != 3 {
		t.Errorf("expected secret/a accessed 3 times, got %d", summary.TopSecrets["secret/a"])
	}
}

func TestInMemorySecretAuditLogger_Filter(t *testing.T) {
	logger := NewInMemorySecretAuditLogger(nil)
	ctx := context.Background()

	now := time.Now()

	events := []*SecretAccessEvent{
		{SecretPath: "secret/a", AgentID: "agent-1", Action: AuditActionRead, Success: true, Timestamp: now.Add(-2 * time.Hour)},
		{SecretPath: "secret/b", AgentID: "agent-2", Action: AuditActionRead, Success: true, Timestamp: now.Add(-time.Hour)},
		{SecretPath: "secret/a", AgentID: "agent-1", Action: AuditActionRead, Success: false, Timestamp: now},
	}

	for _, e := range events {
		_ = logger.LogSecretAccess(ctx, e)
	}

	// Filter by path
	filtered := logger.GetEventsFiltered(&SecretAuditFilter{SecretPath: "secret/a"})
	if len(filtered) != 2 {
		t.Errorf("expected 2 events for secret/a, got %d", len(filtered))
	}

	// Filter by agent
	filtered = logger.GetEventsFiltered(&SecretAuditFilter{AgentID: "agent-1"})
	if len(filtered) != 2 {
		t.Errorf("expected 2 events for agent-1, got %d", len(filtered))
	}

	// Filter by success only
	filtered = logger.GetEventsFiltered(&SecretAuditFilter{SuccessOnly: true})
	if len(filtered) != 2 {
		t.Errorf("expected 2 successful events, got %d", len(filtered))
	}

	// Filter by failure only
	filtered = logger.GetEventsFiltered(&SecretAuditFilter{FailureOnly: true})
	if len(filtered) != 1 {
		t.Errorf("expected 1 failed event, got %d", len(filtered))
	}

	// Filter by time range
	filtered = logger.GetEventsFiltered(&SecretAuditFilter{
		StartTime: now.Add(-90 * time.Minute),
		EndTime:   now.Add(time.Minute),
	})
	if len(filtered) != 2 {
		t.Errorf("expected 2 events in time range, got %d", len(filtered))
	}

	// Test limit
	filtered = logger.GetEventsFiltered(&SecretAuditFilter{Limit: 2})
	if len(filtered) != 2 {
		t.Errorf("expected 2 events with limit, got %d", len(filtered))
	}

	// Test offset
	filtered = logger.GetEventsFiltered(&SecretAuditFilter{Offset: 1, Limit: 10})
	if len(filtered) != 2 {
		t.Errorf("expected 2 events with offset, got %d", len(filtered))
	}
}

func TestMultiSecretAuditLogger(t *testing.T) {
	logger1 := NewInMemorySecretAuditLogger(nil)
	logger2 := NewInMemorySecretAuditLogger(nil)

	multi := NewMultiSecretAuditLogger(logger1, logger2)

	ctx := context.Background()
	event := &SecretAccessEvent{
		SecretPath: "test",
		Timestamp:  time.Now(),
	}

	err := multi.LogSecretAccess(ctx, event)
	if err != nil {
		t.Fatalf("failed to log: %v", err)
	}

	// Both loggers should have the event
	if logger1.GetEventCount() != 1 {
		t.Errorf("expected logger1 to have 1 event")
	}
	if logger2.GetEventCount() != 1 {
		t.Errorf("expected logger2 to have 1 event")
	}
}

func TestChannelSecretAuditLogger(t *testing.T) {
	logger := NewChannelSecretAuditLogger(10)
	defer logger.Close()

	ctx := context.Background()
	event := &SecretAccessEvent{
		SecretPath: "test",
		Timestamp:  time.Now(),
	}

	err := logger.LogSecretAccess(ctx, event)
	if err != nil {
		t.Fatalf("failed to log: %v", err)
	}

	// Read from channel
	select {
	case received := <-logger.Events():
		if received.SecretPath != "test" {
			t.Errorf("unexpected event path: %s", received.SecretPath)
		}
	case <-time.After(time.Second):
		t.Error("expected to receive event from channel")
	}
}

func TestCacheStats_HitRate(t *testing.T) {
	tests := []struct {
		hits   int64
		misses int64
		want   float64
	}{
		{0, 0, 0},
		{100, 0, 100},
		{0, 100, 0},
		{50, 50, 50},
		{75, 25, 75},
	}

	for _, tt := range tests {
		stats := &CacheStats{Hits: tt.hits, Misses: tt.misses}
		got := stats.HitRate()
		if got != tt.want {
			t.Errorf("HitRate() for hits=%d, misses=%d: got %f, want %f", tt.hits, tt.misses, got, tt.want)
		}
	}
}

func TestGenerateCacheKey(t *testing.T) {
	key1, err := GenerateCacheKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	if len(key1) != 32 {
		t.Errorf("expected 32 byte key, got %d", len(key1))
	}

	// Generate another key, should be different
	key2, _ := GenerateCacheKey()
	if string(key1) == string(key2) {
		t.Error("expected different keys to be generated")
	}
}

func TestSecretBroker_List(t *testing.T) {
	broker := NewSecretBroker(nil)

	backend := newMockBackend("vault", BackendTypeVault)
	backend.addSecret("kv/secret1", map[string]interface{}{"key": "value1"})
	backend.addSecret("kv/secret2", map[string]interface{}{"key": "value2"})
	_ = broker.RegisterBackend("vault", backend)

	ctx := context.Background()

	paths, err := broker.List(ctx, "vault/kv")
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}

	if len(paths) != 2 {
		t.Errorf("expected 2 paths, got %d", len(paths))
	}
}

func TestSecretBroker_Invalidate(t *testing.T) {
	cache := NewInMemorySecretCache(&CacheConfig{MaxEntries: 100})
	defer cache.Close()

	broker := NewSecretBroker(&BrokerConfig{
		Cache: &CacheConfig{Enabled: true, DefaultTTL: time.Hour},
	})
	broker.SetCache(cache)

	ctx := context.Background()

	// Put a secret in cache
	_ = cache.Put(ctx, &Secret{Path: "test/secret"}, time.Hour)

	// Verify it's in cache
	_, ok := cache.Get(ctx, "test/secret")
	if !ok {
		t.Fatal("expected secret in cache")
	}

	// Invalidate
	err := broker.Invalidate(ctx, "test/secret")
	if err != nil {
		t.Fatalf("failed to invalidate: %v", err)
	}

	// Verify it's gone
	_, ok = cache.Get(ctx, "test/secret")
	if ok {
		t.Error("expected secret to be removed from cache")
	}
}

func TestSecretBroker_InvalidatePrefix(t *testing.T) {
	cache := NewInMemorySecretCache(&CacheConfig{MaxEntries: 100})
	defer cache.Close()

	broker := NewSecretBroker(nil)
	broker.SetCache(cache)

	ctx := context.Background()

	// Put secrets in cache
	_ = cache.Put(ctx, &Secret{Path: "app/secret1"}, time.Hour)
	_ = cache.Put(ctx, &Secret{Path: "app/secret2"}, time.Hour)
	_ = cache.Put(ctx, &Secret{Path: "other/secret"}, time.Hour)

	// Invalidate prefix
	count, err := broker.InvalidatePrefix(ctx, "app/")
	if err != nil {
		t.Fatalf("failed to invalidate prefix: %v", err)
	}
	if count != 2 {
		t.Errorf("expected to invalidate 2, got %d", count)
	}

	// Verify
	_, ok := cache.Get(ctx, "app/secret1")
	if ok {
		t.Error("expected app/secret1 to be removed")
	}

	_, ok = cache.Get(ctx, "other/secret")
	if !ok {
		t.Error("expected other/secret to still exist")
	}
}

func TestSecretBroker_Stats(t *testing.T) {
	broker := NewSecretBroker(nil)

	backend := newMockBackend("vault", BackendTypeVault)
	_ = broker.RegisterBackend("vault", backend)

	cache := NewInMemorySecretCache(&CacheConfig{MaxEntries: 100})
	defer cache.Close()
	broker.SetCache(cache)

	leaseManager := NewInMemoryLeaseManager(nil)
	broker.SetLeaseManager(leaseManager)

	ctx := context.Background()
	stats := broker.Stats(ctx)

	if stats.BackendCount != 1 {
		t.Errorf("expected 1 backend, got %d", stats.BackendCount)
	}

	if stats.CacheStats == nil {
		t.Error("expected cache stats")
	}

	if stats.LeaseStats == nil {
		t.Error("expected lease stats")
	}
}

func TestSecretBroker_Close(t *testing.T) {
	broker := NewSecretBroker(nil)

	backend := newMockBackend("vault", BackendTypeVault)
	_ = broker.RegisterBackend("vault", backend)

	cache := NewInMemorySecretCache(&CacheConfig{MaxEntries: 100})
	broker.SetCache(cache)

	leaseManager := NewInMemoryLeaseManager(nil)
	broker.SetLeaseManager(leaseManager)

	err := broker.Close()
	if err != nil {
		t.Fatalf("failed to close: %v", err)
	}

	// Verify broker is no longer healthy
	ctx := context.Background()
	if broker.Healthy(ctx) {
		t.Error("expected broker to be unhealthy after close")
	}
}

func TestSecretBroker_GetBackend(t *testing.T) {
	broker := NewSecretBroker(nil)

	backend := newMockBackend("vault", BackendTypeVault)
	_ = broker.RegisterBackend("vault", backend)

	// Test getting existing backend
	got, err := broker.GetBackend("vault")
	if err != nil {
		t.Fatalf("failed to get backend: %v", err)
	}
	if got != backend {
		t.Error("got wrong backend")
	}

	// Test getting non-existent backend
	_, err = broker.GetBackend("nonexistent")
	if !errors.Is(err, ErrBackendNotFound) {
		t.Errorf("expected ErrBackendNotFound, got %v", err)
	}
}

func TestSecretBroker_AddRoutingRule(t *testing.T) {
	broker := NewSecretBroker(nil)

	backend := newMockBackend("vault", BackendTypeVault)
	backend.addSecret("secrets/db", map[string]interface{}{"password": "test"})
	_ = broker.RegisterBackend("vault", backend)

	// Add routing rule
	broker.AddRoutingRule(RoutingRule{Prefix: "prod/", Backend: "vault"})

	ctx := context.Background()

	// Test routing works
	secret, err := broker.Read(ctx, &SecretRequest{Path: "prod/secrets/db"})
	if err != nil {
		t.Fatalf("failed to read with routing: %v", err)
	}
	if _, ok := secret.GetString("password"); !ok {
		t.Error("expected password in secret")
	}
}

func TestEncryptedSecretCache_Delete(t *testing.T) {
	key, _ := GenerateCacheKey()
	cache, _ := NewEncryptedSecretCache(&CacheConfig{MaxEntries: 100, CleanupInterval: time.Hour}, key)
	defer cache.Close()

	ctx := context.Background()

	// Put and then delete
	_ = cache.Put(ctx, &Secret{Path: "test/delete"}, time.Hour)

	_, ok := cache.Get(ctx, "test/delete")
	if !ok {
		t.Fatal("expected secret in cache")
	}

	err := cache.Delete(ctx, "test/delete")
	if err != nil {
		t.Fatalf("failed to delete: %v", err)
	}

	_, ok = cache.Get(ctx, "test/delete")
	if ok {
		t.Error("expected secret to be deleted")
	}
}

func TestEncryptedSecretCache_DeleteByPrefix(t *testing.T) {
	key, _ := GenerateCacheKey()
	cache, _ := NewEncryptedSecretCache(&CacheConfig{MaxEntries: 100, CleanupInterval: time.Hour}, key)
	defer cache.Close()

	ctx := context.Background()

	// Put secrets
	_ = cache.Put(ctx, &Secret{Path: "app/db"}, time.Hour)
	_ = cache.Put(ctx, &Secret{Path: "app/redis"}, time.Hour)
	_ = cache.Put(ctx, &Secret{Path: "other/secret"}, time.Hour)

	// Delete by prefix
	count, err := cache.DeleteByPrefix(ctx, "app/")
	if err != nil {
		t.Fatalf("failed to delete by prefix: %v", err)
	}
	if count != 2 {
		t.Errorf("expected to delete 2, got %d", count)
	}
}

func TestEncryptedSecretCache_Clear(t *testing.T) {
	key, _ := GenerateCacheKey()
	cache, _ := NewEncryptedSecretCache(&CacheConfig{MaxEntries: 100, CleanupInterval: time.Hour}, key)
	defer cache.Close()

	ctx := context.Background()

	// Put secrets
	_ = cache.Put(ctx, &Secret{Path: "secret1"}, time.Hour)
	_ = cache.Put(ctx, &Secret{Path: "secret2"}, time.Hour)

	// Clear
	err := cache.Clear(ctx)
	if err != nil {
		t.Fatalf("failed to clear: %v", err)
	}

	stats := cache.Stats()
	if stats.Entries != 0 {
		t.Errorf("expected 0 entries after clear, got %d", stats.Entries)
	}
}

func TestEncryptedSecretCache_LRUEviction(t *testing.T) {
	key, _ := GenerateCacheKey()
	cache, _ := NewEncryptedSecretCache(&CacheConfig{MaxEntries: 2, CleanupInterval: time.Hour}, key)
	defer cache.Close()

	ctx := context.Background()

	// Fill cache
	_ = cache.Put(ctx, &Secret{Path: "secret0"}, time.Hour)
	time.Sleep(time.Millisecond)
	_ = cache.Put(ctx, &Secret{Path: "secret1"}, time.Hour)
	time.Sleep(time.Millisecond)

	// Access first to make it recent
	_, _ = cache.Get(ctx, "secret0")
	time.Sleep(time.Millisecond)

	// Add new secret, should evict secret1 (oldest access)
	_ = cache.Put(ctx, &Secret{Path: "secret2"}, time.Hour)

	// secret1 should be evicted
	_, ok := cache.Get(ctx, "secret1")
	if ok {
		t.Error("expected secret1 to be evicted")
	}

	// secret0 should still exist
	_, ok = cache.Get(ctx, "secret0")
	if !ok {
		t.Error("expected secret0 to still exist")
	}
}

func TestInMemoryLeaseManager_ListExpiring(t *testing.T) {
	manager := NewInMemoryLeaseManager(nil)
	ctx := context.Background()

	// Add lease expiring soon
	_ = manager.Track(ctx, &Lease{
		ID:        "expiring-soon",
		ExpiresAt: time.Now().Add(30 * time.Minute),
	})

	// Add lease expiring later
	_ = manager.Track(ctx, &Lease{
		ID:        "expiring-later",
		ExpiresAt: time.Now().Add(2 * time.Hour),
	})

	// List expiring within 1 hour
	leases, err := manager.ListExpiring(ctx, time.Hour)
	if err != nil {
		t.Fatalf("failed to list expiring: %v", err)
	}

	if len(leases) != 1 {
		t.Errorf("expected 1 expiring lease, got %d", len(leases))
	}

	if leases[0].ID != "expiring-soon" {
		t.Errorf("expected expiring-soon, got %s", leases[0].ID)
	}
}

func TestInMemoryLeaseManager_Remove(t *testing.T) {
	manager := NewInMemoryLeaseManager(nil)
	ctx := context.Background()

	_ = manager.Track(ctx, &Lease{ID: "to-remove"})

	err := manager.Remove(ctx, "to-remove")
	if err != nil {
		t.Fatalf("failed to remove: %v", err)
	}

	_, err = manager.Get(ctx, "to-remove")
	if !errors.Is(err, ErrLeaseNotFound) {
		t.Errorf("expected ErrLeaseNotFound, got %v", err)
	}
}

func TestInMemoryLeaseManager_StartStop(t *testing.T) {
	manager := NewInMemoryLeaseManager(&LeaseRenewalConfig{
		RetryInterval: 100 * time.Millisecond,
	})

	ctx := context.Background()

	// Start
	err := manager.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start: %v", err)
	}

	// Start again (should be no-op)
	err = manager.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start again: %v", err)
	}

	// Stop
	err = manager.Stop()
	if err != nil {
		t.Fatalf("failed to stop: %v", err)
	}
}

func TestInMemoryLeaseManager_TrackErrors(t *testing.T) {
	manager := NewInMemoryLeaseManager(nil)
	ctx := context.Background()

	// Test nil lease
	err := manager.Track(ctx, nil)
	if err == nil {
		t.Error("expected error for nil lease")
	}

	// Test empty ID
	err = manager.Track(ctx, &Lease{ID: ""})
	if err == nil {
		t.Error("expected error for empty ID")
	}
}

func TestInMemoryLeaseManager_GetNonExistent(t *testing.T) {
	manager := NewInMemoryLeaseManager(nil)
	ctx := context.Background()

	_, err := manager.Get(ctx, "nonexistent")
	if !errors.Is(err, ErrLeaseNotFound) {
		t.Errorf("expected ErrLeaseNotFound, got %v", err)
	}
}

func TestSecretAuditLogger_GetEventsByTimeRange(t *testing.T) {
	logger := NewInMemorySecretAuditLogger(nil)
	ctx := context.Background()

	now := time.Now()

	// Add events at different times
	_ = logger.LogSecretAccess(ctx, &SecretAccessEvent{SecretPath: "old", Timestamp: now.Add(-2 * time.Hour)})
	_ = logger.LogSecretAccess(ctx, &SecretAccessEvent{SecretPath: "mid", Timestamp: now.Add(-time.Hour)})
	_ = logger.LogSecretAccess(ctx, &SecretAccessEvent{SecretPath: "new", Timestamp: now})

	// Get events in range
	events := logger.GetEventsByTimeRange(now.Add(-90*time.Minute), now.Add(time.Minute))
	if len(events) != 2 {
		t.Errorf("expected 2 events in range, got %d", len(events))
	}
}

func TestSecretAccessEvent_MarshalJSON(t *testing.T) {
	event := &SecretAccessEvent{
		SecretPath: "test/secret",
		Action:     AuditActionRead,
		Duration:   150 * time.Millisecond,
		Timestamp:  time.Now(),
	}

	data, err := event.MarshalJSON()
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Verify duration_ms is present
	if !strings.Contains(string(data), `"duration_ms":150`) {
		t.Errorf("expected duration_ms in JSON: %s", data)
	}
}

func TestNoopSecretAuditLogger(t *testing.T) {
	logger := &NoopSecretAuditLogger{}
	ctx := context.Background()

	err := logger.LogSecretAccess(ctx, &SecretAccessEvent{})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestSecret_GetBytes(t *testing.T) {
	secret := &Secret{
		Data: map[string]interface{}{
			"bytes_val":  []byte("hello"),
			"string_val": "world",
			"int_val":    123,
		},
	}

	// Test bytes value
	b, ok := secret.GetBytes("bytes_val")
	if !ok || string(b) != "hello" {
		t.Errorf("expected 'hello', got %s", string(b))
	}

	// Test string as bytes
	b, ok = secret.GetBytes("string_val")
	if !ok || string(b) != "world" {
		t.Errorf("expected 'world', got %s", string(b))
	}

	// Test non-convertible value
	_, ok = secret.GetBytes("int_val")
	if ok {
		t.Error("expected GetBytes to fail for int value")
	}
}

func TestSecret_HasLease(t *testing.T) {
	// No lease
	s1 := &Secret{}
	if s1.HasLease() {
		t.Error("expected no lease")
	}

	// Empty lease ID
	s2 := &Secret{Lease: &Lease{ID: ""}}
	if s2.HasLease() {
		t.Error("expected no lease with empty ID")
	}

	// Valid lease
	s3 := &Secret{Lease: &Lease{ID: "lease-123"}}
	if !s3.HasLease() {
		t.Error("expected to have lease")
	}
}

func TestLease_TimeRemaining(t *testing.T) {
	// No expiry
	l1 := &Lease{}
	if l1.TimeRemaining() != 0 {
		t.Error("expected 0 time remaining for no expiry")
	}

	// Future expiry
	l2 := &Lease{ExpiresAt: time.Now().Add(time.Hour)}
	remaining := l2.TimeRemaining()
	if remaining < 59*time.Minute || remaining > time.Hour {
		t.Errorf("unexpected time remaining: %v", remaining)
	}

	// Past expiry
	l3 := &Lease{ExpiresAt: time.Now().Add(-time.Hour)}
	if l3.TimeRemaining() != 0 {
		t.Error("expected 0 time remaining for past expiry")
	}
}

func TestLease_IsExpired(t *testing.T) {
	// No expiry
	l1 := &Lease{}
	if l1.IsExpired() {
		t.Error("lease with no expiry should not be expired")
	}

	// Future expiry
	l2 := &Lease{ExpiresAt: time.Now().Add(time.Hour)}
	if l2.IsExpired() {
		t.Error("lease with future expiry should not be expired")
	}

	// Past expiry
	l3 := &Lease{ExpiresAt: time.Now().Add(-time.Hour)}
	if !l3.IsExpired() {
		t.Error("lease with past expiry should be expired")
	}
}

func TestParseBackendType(t *testing.T) {
	tests := []struct {
		input   string
		want    BackendType
		wantErr bool
	}{
		{"vault", BackendTypeVault, false},
		{"aws_secrets_manager", BackendTypeAWS, false},
		{"azure_keyvault", BackendTypeAzure, false},
		{"gcp_secret_manager", BackendTypeGCP, false},
		{"invalid", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseBackendType(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseBackendType() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParseBackendType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSecretBroker_SetAuditLogger(t *testing.T) {
	broker := NewSecretBroker(nil)
	logger := NewInMemorySecretAuditLogger(nil)

	broker.SetAuditLogger(logger)

	// Verify audit logger is set by making a request
	backend := newMockBackend("vault", BackendTypeVault)
	backend.addSecret("test", map[string]interface{}{"key": "value"})
	_ = broker.RegisterBackend("vault", backend)

	ctx := context.Background()
	_, _ = broker.Read(ctx, &SecretRequest{Path: "vault/test"})

	// Check audit log has the event
	events := logger.GetEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 audit event, got %d", len(events))
	}
}

func TestSecretBroker_SetLeaseManager(t *testing.T) {
	broker := NewSecretBroker(nil)
	leaseManager := NewInMemoryLeaseManager(nil)

	broker.SetLeaseManager(leaseManager)

	// Verify by reading dynamic secret
	backend := newMockBackend("vault", BackendTypeVault)
	_ = broker.RegisterBackend("vault", backend)

	ctx := context.Background()
	_, err := broker.ReadDynamic(ctx, &SecretRequest{
		Path:      "vault/database/creds/app",
		TTL:       time.Hour,
		Renewable: true,
	})
	if err != nil {
		t.Fatalf("failed to read dynamic: %v", err)
	}

	// Check lease was tracked
	leases, _ := leaseManager.List(ctx)
	if len(leases) != 1 {
		t.Errorf("expected 1 lease, got %d", len(leases))
	}
}

func TestDefaultCacheConfig(t *testing.T) {
	config := DefaultCacheConfig()

	if !config.Enabled {
		t.Error("expected enabled by default")
	}
	if config.MaxEntries != 10000 {
		t.Errorf("expected 10000 max entries, got %d", config.MaxEntries)
	}
	if config.DefaultTTL != 5*time.Minute {
		t.Errorf("expected 5m TTL, got %v", config.DefaultTTL)
	}
}

func TestDefaultLeaseRenewalConfig(t *testing.T) {
	config := DefaultLeaseRenewalConfig()

	if config.Strategy != RenewalStrategyEager {
		t.Errorf("expected eager strategy, got %v", config.Strategy)
	}
	if config.Threshold != 0.5 {
		t.Errorf("expected 0.5 threshold, got %v", config.Threshold)
	}
	if config.MaxRetries != 3 {
		t.Errorf("expected 3 retries, got %d", config.MaxRetries)
	}
}

func TestSecretBroker_RenewLease(t *testing.T) {
	broker := NewSecretBroker(nil)

	// Test without lease manager
	_, err := broker.RenewLease(context.Background(), "lease-123", time.Hour)
	if err == nil {
		t.Error("expected error without lease manager")
	}

	// Setup with lease manager
	leaseManager := NewInMemoryLeaseManager(nil)
	broker.SetLeaseManager(leaseManager)

	// Test renewing non-existent lease
	_, err = broker.RenewLease(context.Background(), "nonexistent", time.Hour)
	if !errors.Is(err, ErrLeaseNotFound) {
		t.Errorf("expected ErrLeaseNotFound, got %v", err)
	}
}

func TestSecretBroker_RevokeLease(t *testing.T) {
	broker := NewSecretBroker(nil)

	// Test without lease manager
	err := broker.RevokeLease(context.Background(), "lease-123")
	if err == nil {
		t.Error("expected error without lease manager")
	}

	// Setup with lease manager and backend
	leaseManager := NewInMemoryLeaseManager(nil)
	broker.SetLeaseManager(leaseManager)

	backend := newMockBackend("vault", BackendTypeVault)
	_ = broker.RegisterBackend("vault", backend)
	leaseManager.RegisterBackend(BackendTypeVault, backend)

	ctx := context.Background()

	// Track a lease
	lease := &Lease{
		ID:         "lease-to-revoke",
		SecretPath: "vault/db/creds",
		Backend:    BackendTypeVault,
		TTL:        time.Hour,
		ExpiresAt:  time.Now().Add(time.Hour),
	}
	_ = leaseManager.Track(ctx, lease)
	backend.leases[lease.ID] = lease

	// Revoke via broker
	err = broker.RevokeLease(ctx, "lease-to-revoke")
	if err != nil {
		t.Fatalf("failed to revoke: %v", err)
	}

	// Verify revoked
	tracked, _ := leaseManager.Get(ctx, "lease-to-revoke")
	if tracked.State != LeaseStateRevoked {
		t.Errorf("expected revoked state, got %s", tracked.State)
	}
}

func TestInMemoryLeaseManager_Renew(t *testing.T) {
	manager := NewInMemoryLeaseManager(nil)
	ctx := context.Background()

	backend := newMockBackend("vault", BackendTypeVault)
	manager.RegisterBackend(BackendTypeVault, backend)

	// Add renewable lease
	lease := &Lease{
		ID:         "renewable-lease",
		SecretPath: "vault/db/creds",
		Backend:    BackendTypeVault,
		TTL:        time.Hour,
		ExpiresAt:  time.Now().Add(time.Hour),
		Renewable:  true,
	}
	_ = manager.Track(ctx, lease)
	backend.leases[lease.ID] = lease

	// Renew
	renewed, err := manager.Renew(ctx, "renewable-lease", 2*time.Hour)
	if err != nil {
		t.Fatalf("failed to renew: %v", err)
	}

	if renewed.TTL != 2*time.Hour {
		t.Errorf("expected 2h TTL, got %v", renewed.TTL)
	}

	if renewed.RenewalCount != 1 {
		t.Errorf("expected renewal count 1, got %d", renewed.RenewalCount)
	}
}

func TestInMemoryLeaseManager_RenewNonRenewable(t *testing.T) {
	manager := NewInMemoryLeaseManager(nil)
	ctx := context.Background()

	// Add non-renewable lease
	lease := &Lease{
		ID:        "non-renewable",
		Backend:   BackendTypeVault,
		ExpiresAt: time.Now().Add(time.Hour),
		Renewable: false,
	}
	_ = manager.Track(ctx, lease)

	// Attempt to renew
	_, err := manager.Renew(ctx, "non-renewable", time.Hour)
	if !errors.Is(err, ErrLeaseNotRenewable) {
		t.Errorf("expected ErrLeaseNotRenewable, got %v", err)
	}
}

func TestInMemoryLeaseManager_RenewExpired(t *testing.T) {
	manager := NewInMemoryLeaseManager(nil)
	ctx := context.Background()

	// Add expired lease
	lease := &Lease{
		ID:        "expired-lease",
		Backend:   BackendTypeVault,
		ExpiresAt: time.Now().Add(-time.Hour),
		Renewable: true,
	}
	_ = manager.Track(ctx, lease)

	// Attempt to renew
	_, err := manager.Renew(ctx, "expired-lease", time.Hour)
	if !errors.Is(err, ErrLeaseExpired) {
		t.Errorf("expected ErrLeaseExpired, got %v", err)
	}
}

func TestSecretBroker_ReadWithCache(t *testing.T) {
	cache := NewInMemorySecretCache(&CacheConfig{MaxEntries: 100, DefaultTTL: time.Hour, CleanupInterval: time.Hour})
	defer cache.Close()

	broker := NewSecretBroker(&BrokerConfig{
		Cache: &CacheConfig{Enabled: true, DefaultTTL: time.Hour},
	})
	broker.SetCache(cache)

	backend := newMockBackend("vault", BackendTypeVault)
	backend.addSecret("kv/test", map[string]interface{}{"key": "value"})
	_ = broker.RegisterBackend("vault", backend)

	ctx := context.Background()

	// First read - cache miss
	_, err := broker.Read(ctx, &SecretRequest{Path: "vault/kv/test"})
	if err != nil {
		t.Fatalf("first read failed: %v", err)
	}

	stats := cache.Stats()
	if stats.Misses != 1 {
		t.Errorf("expected 1 miss, got %d", stats.Misses)
	}

	// Second read - cache hit
	_, err = broker.Read(ctx, &SecretRequest{Path: "vault/kv/test"})
	if err != nil {
		t.Fatalf("second read failed: %v", err)
	}

	stats = cache.Stats()
	if stats.Hits != 1 {
		t.Errorf("expected 1 hit, got %d", stats.Hits)
	}
}

func TestSecretBroker_ReadWithCacheAndExpiry(t *testing.T) {
	cache := NewInMemorySecretCache(&CacheConfig{MaxEntries: 100, CleanupInterval: time.Hour})
	defer cache.Close()

	broker := NewSecretBroker(&BrokerConfig{
		Cache: &CacheConfig{Enabled: true, DefaultTTL: time.Hour},
	})
	broker.SetCache(cache)

	backend := newMockBackend("vault", BackendTypeVault)
	// Add secret with short expiry
	backend.secrets["kv/expiring"] = &Secret{
		Path:      "kv/expiring",
		Data:      map[string]interface{}{"key": "value"},
		ExpiresAt: time.Now().Add(time.Second), // Short expiry
	}
	_ = broker.RegisterBackend("vault", backend)

	ctx := context.Background()

	// Read should use shorter TTL based on secret expiry
	_, err := broker.Read(ctx, &SecretRequest{Path: "vault/kv/expiring"})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
}

func TestSecretBroker_DefaultBackendFallback(t *testing.T) {
	broker := NewSecretBroker(&BrokerConfig{
		DefaultBackend: "vault",
	})

	backend := newMockBackend("vault", BackendTypeVault)
	backend.addSecret("kv/test", map[string]interface{}{"key": "value"})
	_ = broker.RegisterBackend("vault", backend)

	ctx := context.Background()

	// Read with path that doesn't match any prefix - should use default
	secret, err := broker.Read(ctx, &SecretRequest{Path: "kv/test"})
	if err != nil {
		t.Fatalf("read with default backend failed: %v", err)
	}

	if _, ok := secret.GetString("key"); !ok {
		t.Error("expected key in secret")
	}
}

func TestChannelSecretAuditLogger_ContextCancellation(t *testing.T) {
	logger := NewChannelSecretAuditLogger(1)
	defer logger.Close()

	// Fill the buffer
	_ = logger.LogSecretAccess(context.Background(), &SecretAccessEvent{SecretPath: "first"})

	// Try to log with cancelled context - should not block
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := logger.LogSecretAccess(ctx, &SecretAccessEvent{SecretPath: "second"})
	if !errors.Is(err, context.Canceled) {
		// Could be nil if dropped, or canceled
		if err != nil {
			t.Logf("got error: %v", err)
		}
	}
}

func TestSecretType_String(t *testing.T) {
	if SecretTypeStatic.String() != "static" {
		t.Error("unexpected string for static")
	}
	if SecretTypeDynamic.String() != "dynamic" {
		t.Error("unexpected string for dynamic")
	}
}

func TestBackendType_String(t *testing.T) {
	if BackendTypeVault.String() != "vault" {
		t.Error("unexpected string for vault")
	}
	if BackendTypeAWS.String() != "aws_secrets_manager" {
		t.Error("unexpected string for aws")
	}
}
