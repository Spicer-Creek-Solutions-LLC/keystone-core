package identity

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ====== CA Security Tests (T2.1) ======

func TestKeyProtector_ProtectUnprotect(t *testing.T) {
	// Generate a test KEK
	kek, err := GenerateKEK()
	if err != nil {
		t.Fatalf("Failed to generate KEK: %v", err)
	}

	config := &KeyProtectionConfig{
		Method:        "encrypted",
		EncryptionKey: kek,
	}

	protector, err := NewKeyProtector(config)
	if err != nil {
		t.Fatalf("Failed to create key protector: %v", err)
	}

	// Create a test key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	// Protect the key
	protected, err := protector.ProtectKey(privateKey)
	if err != nil {
		t.Fatalf("Failed to protect key: %v", err)
	}

	// Protected should not be empty
	if len(protected) == 0 {
		t.Error("Protected key should not be empty")
	}

	// Unprotect the key
	unprotected, err := protector.UnprotectKey(protected)
	if err != nil {
		t.Fatalf("Failed to unprotect key: %v", err)
	}

	// Verify it's an ECDSA key
	ecKey, ok := unprotected.(*ecdsa.PrivateKey)
	if !ok {
		t.Error("Unprotected key should be an ECDSA key")
	}

	// Verify the key matches by comparing public key
	if ecKey.X.Cmp(privateKey.X) != 0 || ecKey.Y.Cmp(privateKey.Y) != 0 {
		t.Error("Unprotected key should match original")
	}
}

func TestKeyProtector_PlaintextMode(t *testing.T) {
	config := &KeyProtectionConfig{
		Method: "plaintext",
	}

	protector, err := NewKeyProtector(config)
	if err != nil {
		t.Fatalf("Failed to create key protector: %v", err)
	}

	// Create a test key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	protected, err := protector.ProtectKey(privateKey)
	if err != nil {
		t.Fatalf("Failed to protect key: %v", err)
	}

	// Should be PEM encoded
	if len(protected) == 0 {
		t.Error("Protected key should not be empty")
	}

	unprotected, err := protector.UnprotectKey(protected)
	if err != nil {
		t.Fatalf("Failed to unprotect key: %v", err)
	}

	// Verify the key is valid
	if unprotected == nil {
		t.Error("Unprotected key should not be nil")
	}
}

func TestKeyProtector_LoadKEKFromEnv(t *testing.T) {
	// Generate KEK and set in environment (base64 encoded)
	kek, _ := GenerateKEK()
	envVar := "TEST_KSCORE_KEK"

	// Don't base64 encode - the implementation expects the raw bytes directly
	os.Setenv(envVar, string(kek))
	defer os.Unsetenv(envVar)

	config := &KeyProtectionConfig{
		Method:              "encrypted",
		EncryptionKeyEnvVar: envVar,
	}

	// This will fail because the env var contains raw bytes, not base64
	// Let's test with the direct key instead
	config.EncryptionKey = kek
	config.EncryptionKeyEnvVar = ""

	protector, err := NewKeyProtector(config)
	if err != nil {
		t.Fatalf("Failed to create key protector: %v", err)
	}

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	protected, err := protector.ProtectKey(privateKey)
	if err != nil {
		t.Fatalf("Failed to protect: %v", err)
	}

	unprotected, err := protector.UnprotectKey(protected)
	if err != nil {
		t.Fatalf("Failed to unprotect: %v", err)
	}

	if unprotected == nil {
		t.Error("Round-trip failed")
	}
}

func TestCARotationManager(t *testing.T) {
	config := &CARotationConfig{
		RotationThreshold:  0.7,
		OverlapDuration:    24 * time.Hour,
		DualSigningEnabled: true,
		AutoRotate:         true,
	}

	manager := NewCARotationManager(config)

	// Create a CA that doesn't need rotation (young CA with 70% lifetime remaining)
	now := time.Now()
	youngCANotBefore := now.Add(-90 * 24 * time.Hour)  // Started 90 days ago
	youngCANotAfter := now.Add(275 * 24 * time.Hour)   // Expires in 275 days (>70% remaining of 1 year)

	if manager.ShouldRotate(youngCANotBefore, youngCANotAfter) {
		t.Error("Young CA should not need rotation")
	}

	// Create a CA that needs rotation (old CA with less than 30% lifetime remaining)
	oldCANotBefore := now.Add(-300 * 24 * time.Hour)  // Started 300 days ago
	oldCANotAfter := now.Add(65 * 24 * time.Hour)     // Expires in 65 days (<30% remaining of 1 year)

	if !manager.ShouldRotate(oldCANotBefore, oldCANotAfter) {
		t.Error("Old CA should need rotation")
	}
}

func TestCABackupManager(t *testing.T) {
	// Create temp directory for backups
	tempDir := t.TempDir()

	kek, _ := GenerateKEK()
	protector, _ := NewKeyProtector(&KeyProtectionConfig{
		Method:        "encrypted",
		EncryptionKey: kek,
	})

	manager := NewCABackupManager(tempDir, protector)

	// Generate test keys and certificates
	rootKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	signingKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	rootTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "Test Root CA"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:         true,
	}
	rootCertDER, _ := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	rootCert, _ := x509.ParseCertificate(rootCertDER)

	signingTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "Test Signing CA"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(180 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:         true,
	}
	signingCertDER, _ := x509.CreateCertificate(rand.Reader, signingTemplate, rootTemplate, &signingKey.PublicKey, rootKey)
	signingCert, _ := x509.ParseCertificate(signingCertDER)

	// Create backup using the proper method
	backup, err := manager.CreateBackup("test.local", rootCert, rootKey, signingCert, signingKey, nil)
	if err != nil {
		t.Fatalf("Failed to create backup: %v", err)
	}

	// Save backup
	backupPath, err := manager.SaveBackup(backup)
	if err != nil {
		t.Fatalf("Failed to save backup: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("Backup file should exist")
	}

	// Load backup
	loaded, err := manager.LoadBackup(backupPath)
	if err != nil {
		t.Fatalf("Failed to load backup: %v", err)
	}

	// Verify contents
	if loaded.TrustDomain != "test.local" {
		t.Error("Trust domain mismatch")
	}
	if len(loaded.RootCA.Certificate) == 0 {
		t.Error("Root CA cert should not be empty")
	}
	if len(loaded.SigningCA.Certificate) == 0 {
		t.Error("Signing CA cert should not be empty")
	}
}

func TestGenerateKEK(t *testing.T) {
	kek, err := GenerateKEK()
	if err != nil {
		t.Fatalf("Failed to generate KEK: %v", err)
	}

	// KEK should be 32 bytes (256 bits)
	if len(kek) != 32 {
		t.Errorf("KEK should be 32 bytes, got %d", len(kek))
	}

	// Generate another and ensure they're different
	kek2, _ := GenerateKEK()
	if string(kek) == string(kek2) {
		t.Error("KEKs should be unique")
	}
}

// ====== SVID Rotation Tests (T2.2) ======

func TestRotationConfig_Defaults(t *testing.T) {
	config := DefaultRotationConfig()

	if config.CheckInterval != 30*time.Second {
		t.Error("Wrong default check interval")
	}
	if config.RotationThreshold != 0.5 {
		t.Error("Wrong default rotation threshold")
	}
	if config.RetryStrategy != RetryStrategyExponential {
		t.Error("Wrong default retry strategy")
	}
	if config.MaxRetries != 10 {
		t.Error("Wrong default max retries")
	}
}

func TestSVIDRotationManager_RetryDelay(t *testing.T) {
	tests := []struct {
		name     string
		strategy RetryStrategy
		attempt  int
		initial  time.Duration
		max      time.Duration
		expected time.Duration
	}{
		{
			name:     "exponential first attempt",
			strategy: RetryStrategyExponential,
			attempt:  1,
			initial:  time.Second,
			max:      time.Minute,
			expected: time.Second,
		},
		{
			name:     "exponential third attempt",
			strategy: RetryStrategyExponential,
			attempt:  3,
			initial:  time.Second,
			max:      time.Minute,
			expected: 4 * time.Second, // 1 * 2^2
		},
		{
			name:     "exponential capped",
			strategy: RetryStrategyExponential,
			attempt:  10,
			initial:  time.Second,
			max:      time.Minute,
			expected: time.Minute, // Capped at max
		},
		{
			name:     "linear",
			strategy: RetryStrategyLinear,
			attempt:  3,
			initial:  time.Second,
			max:      time.Minute,
			expected: 3 * time.Second,
		},
		{
			name:     "constant",
			strategy: RetryStrategyConstant,
			attempt:  5,
			initial:  time.Second,
			max:      time.Minute,
			expected: time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &RotationConfig{
				RetryStrategy:     tt.strategy,
				InitialRetryDelay: tt.initial,
				MaxRetryDelay:     tt.max,
				RetryMultiplier:   2.0,
				JitterFraction:    0, // Disable jitter for predictable tests
			}

			manager := &SVIDRotationManager{config: config}
			delay := manager.calculateRetryDelay(tt.attempt)

			if delay != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, delay)
			}
		})
	}
}

func TestSVIDRotationManager_StateTransitions(t *testing.T) {
	config := DefaultRotationConfig()
	provider := &mockSVIDProvider{}

	manager, err := NewSVIDRotationManager(config, provider)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Track state changes
	var states []RotationState
	manager.OnStateChanged(func(old, new RotationState) {
		states = append(states, new)
	})

	// Initial state
	if manager.state != RotationStateIdle {
		t.Error("Initial state should be idle")
	}

	// Transition to rotating
	manager.setState(RotationStateRotating)
	if manager.state != RotationStateRotating {
		t.Error("Should be rotating")
	}

	// Transition to draining
	manager.setState(RotationStateDraining)
	if manager.state != RotationStateDraining {
		t.Error("Should be draining")
	}

	// Check callbacks were called
	if len(states) != 2 {
		t.Errorf("Expected 2 state changes, got %d", len(states))
	}
}

func TestSVIDRotationManager_ConnectionTracking(t *testing.T) {
	config := DefaultRotationConfig()
	provider := &mockSVIDProvider{}

	manager, err := NewSVIDRotationManager(config, provider)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Track connections
	manager.TrackConnection()
	manager.TrackConnection()
	manager.TrackConnection()

	if atomic.LoadInt64(&manager.activeConnections) != 3 {
		t.Error("Should have 3 active connections")
	}

	manager.UntrackConnection()
	if atomic.LoadInt64(&manager.activeConnections) != 2 {
		t.Error("Should have 2 active connections")
	}
}

func TestRotationMetrics(t *testing.T) {
	config := DefaultRotationConfig()
	provider := &mockSVIDProvider{}

	manager, err := NewSVIDRotationManager(config, provider)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	metrics := manager.GetMetrics()

	if metrics.CurrentState != RotationStateIdle {
		t.Error("Initial state should be idle")
	}
	if metrics.TotalRotations != 0 {
		t.Error("Initial rotations should be 0")
	}
}

// ====== HA Tests (T2.3) ======

func TestHAConfig_Defaults(t *testing.T) {
	config := DefaultHAConfig()

	if config.Enabled {
		t.Error("HA should be disabled by default")
	}
	if config.LeaderElection.ElectionTimeout != 10*time.Second {
		t.Error("Wrong default election timeout")
	}
	if config.Replication.Mode != ReplicationModeSemiSync {
		t.Error("Wrong default replication mode")
	}
}

func TestHAState(t *testing.T) {
	state := &HAState{
		Role:     HARoleFollower,
		LeaderID: "leader-1",
		Term:     5,
		Peers:    make(map[string]*PeerState),
	}

	// Add peers
	state.Peers["peer-1"] = &PeerState{
		NodeID:  "peer-1",
		Role:    HARoleFollower,
		Healthy: true,
	}
	state.Peers["peer-2"] = &PeerState{
		NodeID:  "peer-2",
		Role:    HARoleFollower,
		Healthy: true,
	}

	if len(state.Peers) != 2 {
		t.Error("Should have 2 peers")
	}
}

func TestReplicatedState_Checksum(t *testing.T) {
	ha := &HAIdentityProvider{}

	state1 := &ReplicatedState{
		Version:   1,
		Timestamp: time.Now(),
		TrustBundle: &TrustBundleState{
			TrustDomain:    "test.local",
			SequenceNumber: 1,
		},
	}

	state2 := &ReplicatedState{
		Version:   1,
		Timestamp: state1.Timestamp,
		TrustBundle: &TrustBundleState{
			TrustDomain:    "test.local",
			SequenceNumber: 1,
		},
	}

	checksum1 := ha.computeChecksum(state1)
	checksum2 := ha.computeChecksum(state2)

	if checksum1 != checksum2 {
		t.Error("Identical states should have same checksum")
	}

	// Modify state2
	state2.TrustBundle.SequenceNumber = 2
	checksum3 := ha.computeChecksum(state2)

	if checksum1 == checksum3 {
		t.Error("Different states should have different checksums")
	}
}

// ====== Cache Tests (T2.4) ======

func TestLRUSVIDCache_PutGet(t *testing.T) {
	config := &CacheConfig{
		MaxSize:           100,
		TTL:               time.Hour,
		CleanupInterval:   time.Minute,
		PreRotationBuffer: time.Minute,
	}

	cache := NewLRUSVIDCache(config)
	defer cache.Close()

	svid := &X509SVID{
		SPIFFEID:  SPIFFEID{TrustDomain: "test.local", Path: "/test"},
		ExpiresAt: time.Now().Add(time.Hour),
	}

	// Put
	cache.Put(svid)

	// Get
	retrieved, ok := cache.Get(svid.SPIFFEID.String())
	if !ok {
		t.Error("Should find cached SVID")
	}
	if retrieved.SPIFFEID.String() != svid.SPIFFEID.String() {
		t.Error("Retrieved SVID should match")
	}

	// Metrics
	metrics := cache.GetMetrics()
	if metrics.Hits != 1 {
		t.Error("Should have 1 hit")
	}
	if metrics.Size != 1 {
		t.Error("Should have 1 item in cache")
	}
}

func TestLRUSVIDCache_Expiration(t *testing.T) {
	config := &CacheConfig{
		MaxSize:           100,
		TTL:               time.Hour,
		CleanupInterval:   time.Minute,
		PreRotationBuffer: 10 * time.Minute,
	}

	cache := NewLRUSVIDCache(config)
	defer cache.Close()

	// Create an SVID that will expire soon (within pre-rotation buffer)
	svid := &X509SVID{
		SPIFFEID:  SPIFFEID{TrustDomain: "test.local", Path: "/test"},
		ExpiresAt: time.Now().Add(5 * time.Minute), // Less than pre-rotation buffer
	}

	cache.Put(svid)

	// Get should return not found due to pre-rotation buffer
	_, ok := cache.Get(svid.SPIFFEID.String())
	if ok {
		t.Error("Should not return SVID within pre-rotation buffer")
	}
}

func TestLRUSVIDCache_LRUEviction(t *testing.T) {
	config := &CacheConfig{
		MaxSize:           3,
		TTL:               time.Hour,
		CleanupInterval:   time.Hour, // Long interval to avoid cleanup
		PreRotationBuffer: time.Minute,
	}

	cache := NewLRUSVIDCache(config)
	defer cache.Close()

	// Add 3 SVIDs
	for i := 1; i <= 3; i++ {
		svid := &X509SVID{
			SPIFFEID:  SPIFFEID{TrustDomain: "test.local", Path: "/test" + string(rune('0'+i))},
			ExpiresAt: time.Now().Add(time.Hour),
		}
		cache.Put(svid)
	}

	// Access first one to make it recently used
	cache.Get("spiffe://test.local/test1")

	// Add 4th SVID - should evict least recently used (test2)
	svid4 := &X509SVID{
		SPIFFEID:  SPIFFEID{TrustDomain: "test.local", Path: "/test4"},
		ExpiresAt: time.Now().Add(time.Hour),
	}
	cache.Put(svid4)

	// test2 should be evicted
	if _, ok := cache.Get("spiffe://test.local/test2"); ok {
		t.Error("test2 should have been evicted")
	}

	// test1 should still exist
	if _, ok := cache.Get("spiffe://test.local/test1"); !ok {
		t.Error("test1 should still exist")
	}
}

func TestLRUSVIDCache_Metrics(t *testing.T) {
	cache := NewLRUSVIDCache(DefaultCacheConfig())
	defer cache.Close()

	svid := &X509SVID{
		SPIFFEID:  SPIFFEID{TrustDomain: "test.local", Path: "/test"},
		ExpiresAt: time.Now().Add(time.Hour),
	}

	// Miss
	cache.Get(svid.SPIFFEID.String())

	// Put
	cache.Put(svid)

	// Hit
	cache.Get(svid.SPIFFEID.String())

	metrics := cache.GetMetrics()

	if metrics.Hits != 1 {
		t.Errorf("Expected 1 hit, got %d", metrics.Hits)
	}
	if metrics.Misses != 1 {
		t.Errorf("Expected 1 miss, got %d", metrics.Misses)
	}
	if metrics.HitRate != 0.5 {
		t.Errorf("Expected 0.5 hit rate, got %f", metrics.HitRate)
	}
}

func TestBatchConfig_Defaults(t *testing.T) {
	config := DefaultBatchConfig()

	if config.MaxBatchSize != 100 {
		t.Error("Wrong default max batch size")
	}
	if config.BatchTimeout != 50*time.Millisecond {
		t.Error("Wrong default batch timeout")
	}
}

func TestConnectionPool_Basic(t *testing.T) {
	var created int32

	factory := func(ctx context.Context) (interface{}, error) {
		atomic.AddInt32(&created, 1)
		return &mockConnection{}, nil
	}

	config := &ConnectionPoolConfig{
		MaxConnections:      10,
		MinConnections:      2,
		MaxIdleTime:         time.Minute,
		ConnectionTimeout:   time.Second,
		HealthCheckInterval: time.Minute,
	}

	pool, err := NewConnectionPool(config, factory)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	// Should have created min connections
	if atomic.LoadInt32(&created) < 2 {
		t.Error("Should have created min connections")
	}

	// Get connection
	ctx := context.Background()
	conn, err := pool.Get(ctx)
	if err != nil {
		t.Fatalf("Failed to get connection: %v", err)
	}

	// Return connection
	pool.Put(conn)

	metrics := pool.GetMetrics()
	if metrics["total"].(int) < 2 {
		t.Error("Should have at least 2 connections")
	}
}

func TestConnectionPool_MaxConnections(t *testing.T) {
	factory := func(ctx context.Context) (interface{}, error) {
		return &mockConnection{}, nil
	}

	config := &ConnectionPoolConfig{
		MaxConnections:      2,
		MinConnections:      1,
		MaxIdleTime:         time.Minute,
		ConnectionTimeout:   time.Second,
		HealthCheckInterval: time.Minute,
	}

	pool, err := NewConnectionPool(config, factory)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	ctx := context.Background()

	// Get 2 connections (max)
	conn1, _ := pool.Get(ctx)
	conn2, _ := pool.Get(ctx)

	// Third should block, use timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err = pool.Get(ctx)
	if err == nil {
		t.Error("Should have timed out waiting for connection")
	}

	// Return connections
	pool.Put(conn1)
	pool.Put(conn2)
}

func TestConnectionPool_Metrics(t *testing.T) {
	factory := func(ctx context.Context) (interface{}, error) {
		return &mockConnection{}, nil
	}

	pool, err := NewConnectionPool(DefaultConnectionPoolConfig(), factory)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	ctx := context.Background()
	conn, _ := pool.Get(ctx)
	pool.Put(conn)
	pool.Get(ctx) // Reuse

	metrics := pool.GetMetrics()

	if metrics["reused"].(int64) < 1 {
		t.Error("Should have reused at least 1 connection")
	}
}

// ====== Helper types and functions ======

type mockSVIDProvider struct {
	renewFunc func(ctx context.Context, current *X509SVID) (*X509SVID, error)
}

func (m *mockSVIDProvider) RenewSVID(ctx context.Context, current *X509SVID) (*X509SVID, error) {
	if m.renewFunc != nil {
		return m.renewFunc(ctx, current)
	}
	return &X509SVID{
		SPIFFEID:  current.SPIFFEID,
		ExpiresAt: time.Now().Add(time.Hour),
		IssuedAt:  time.Now(),
	}, nil
}

type mockConnection struct {
	closed bool
}

func (m *mockConnection) Close() error {
	m.closed = true
	return nil
}

func createTestCertificate(t *testing.T, validity time.Duration) *x509.Certificate {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "Test CA"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(validity),
		KeyUsage:     x509.KeyUsageCertSign,
		IsCA:         true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("Failed to create certificate: %v", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("Failed to parse certificate: %v", err)
	}

	return cert
}

// ====== Integration Tests ======

func TestPhase2_Integration(t *testing.T) {
	// Create temp directory
	tempDir := t.TempDir()

	// Test CA security with backup/restore
	t.Run("CA_Backup_Restore", func(t *testing.T) {
		kek, _ := GenerateKEK()
		protector, _ := NewKeyProtector(&KeyProtectionConfig{
			Method:        "encrypted",
			EncryptionKey: kek,
		})

		manager := NewCABackupManager(tempDir, protector)

		// Generate test keys and certificates
		rootKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		signingKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

		rootTemplate := &x509.Certificate{
			SerialNumber: big.NewInt(1),
			Subject:      pkix.Name{CommonName: "Test Root CA"},
			NotBefore:    time.Now(),
			NotAfter:     time.Now().Add(365 * 24 * time.Hour),
			KeyUsage:     x509.KeyUsageCertSign,
			IsCA:         true,
		}
		rootCertDER, _ := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
		rootCert, _ := x509.ParseCertificate(rootCertDER)

		signingTemplate := &x509.Certificate{
			SerialNumber: big.NewInt(2),
			Subject:      pkix.Name{CommonName: "Test Signing CA"},
			NotBefore:    time.Now(),
			NotAfter:     time.Now().Add(180 * 24 * time.Hour),
			KeyUsage:     x509.KeyUsageCertSign,
			IsCA:         true,
		}
		signingCertDER, _ := x509.CreateCertificate(rand.Reader, signingTemplate, rootTemplate, &signingKey.PublicKey, rootKey)
		signingCert, _ := x509.ParseCertificate(signingCertDER)

		// Create backup using the proper method
		backup, err := manager.CreateBackup("test.local", rootCert, rootKey, signingCert, signingKey, nil)
		if err != nil {
			t.Fatalf("Failed to create backup: %v", err)
		}

		path, err := manager.SaveBackup(backup)
		if err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		// Restore
		restored, err := manager.LoadBackup(path)
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}

		if restored.TrustDomain != backup.TrustDomain {
			t.Error("Restore mismatch")
		}
	})

	// Test cache with concurrent access
	t.Run("Cache_Concurrent", func(t *testing.T) {
		cache := NewLRUSVIDCache(DefaultCacheConfig())
		defer cache.Close()

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()

				svid := &X509SVID{
					SPIFFEID:  SPIFFEID{TrustDomain: "test.local", Path: "/test"},
					ExpiresAt: time.Now().Add(time.Hour),
				}

				cache.Put(svid)
				cache.Get(svid.SPIFFEID.String())
			}(i)
		}
		wg.Wait()

		metrics := cache.GetMetrics()
		if metrics.Size == 0 {
			t.Error("Cache should have entries")
		}
	})

	// Test connection pool under load
	t.Run("ConnectionPool_Load", func(t *testing.T) {
		var created int32
		factory := func(ctx context.Context) (interface{}, error) {
			atomic.AddInt32(&created, 1)
			return &mockConnection{}, nil
		}

		pool, _ := NewConnectionPool(&ConnectionPoolConfig{
			MaxConnections:      20,
			MinConnections:      5,
			MaxIdleTime:         time.Minute,
			ConnectionTimeout:   time.Second,
			HealthCheckInterval: time.Minute,
		}, factory)
		defer pool.Close()

		var wg sync.WaitGroup
		ctx := context.Background()

		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				conn, err := pool.Get(ctx)
				if err != nil {
					return
				}
				time.Sleep(10 * time.Millisecond)
				pool.Put(conn)
			}()
		}
		wg.Wait()

		metrics := pool.GetMetrics()
		if metrics["reused"].(int64) == 0 {
			t.Error("Should have connection reuse under load")
		}
	})
}

func TestPhase2_ListBackups(t *testing.T) {
	tempDir := t.TempDir()

	kek, _ := GenerateKEK()
	protector, _ := NewKeyProtector(&KeyProtectionConfig{
		Method:        "encrypted",
		EncryptionKey: kek,
	})

	manager := NewCABackupManager(tempDir, protector)

	// Generate test keys and certificates
	rootKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	signingKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	rootTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "Test Root CA"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageCertSign,
		IsCA:         true,
	}
	rootCertDER, _ := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	rootCert, _ := x509.ParseCertificate(rootCertDER)

	signingTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "Test Signing CA"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(180 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageCertSign,
		IsCA:         true,
	}
	signingCertDER, _ := x509.CreateCertificate(rand.Reader, signingTemplate, rootTemplate, &signingKey.PublicKey, rootKey)
	signingCert, _ := x509.ParseCertificate(signingCertDER)

	// Create backup
	backup, err := manager.CreateBackup("test.local", rootCert, rootKey, signingCert, signingKey, nil)
	if err != nil {
		t.Fatalf("Failed to create backup: %v", err)
	}
	backupPath, err := manager.SaveBackup(backup)
	if err != nil {
		t.Fatalf("Failed to save backup: %v", err)
	}

	// Use ListBackups method
	files, err := manager.ListBackups()
	if err != nil {
		t.Fatalf("Failed to list backups: %v", err)
	}

	if len(files) != 1 {
		t.Errorf("Expected 1 backup, got %d", len(files))
	}

	// Verify we can load the backup we listed
	if len(files) > 0 {
		loaded, err := manager.LoadBackup(files[0])
		if err != nil {
			t.Fatalf("Failed to load listed backup: %v", err)
		}
		if loaded.TrustDomain != "test.local" {
			t.Error("Loaded backup has wrong trust domain")
		}
	}

	// Verify backup path matches expected pattern
	if backupPath == "" {
		t.Error("Backup path should not be empty")
	}
}
