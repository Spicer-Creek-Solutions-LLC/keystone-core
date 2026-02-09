package kms

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/secrets"
)

// =============================================================================
// Mock KMS Provider
// =============================================================================

// mockProvider implements Provider for testing without actual KMS.
type mockProvider struct {
	name    string
	healthy bool
	keys    map[string]*mockKey
	mu      sync.RWMutex
}

type mockKey struct {
	id        string
	plaintext []byte
	metadata  *KeyMetadata
}

func newMockProvider() *mockProvider {
	return &mockProvider{
		name:    "mock-kms",
		healthy: true,
		keys:    make(map[string]*mockKey),
	}
}

func (p *mockProvider) Type() ProviderType {
	return ProviderType("mock")
}

func (p *mockProvider) Name() string {
	return p.name
}

func (p *mockProvider) Healthy(ctx context.Context) bool {
	return p.healthy
}

func (p *mockProvider) GetKeyMetadata(ctx context.Context, keyID string) (*KeyMetadata, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	key, ok := p.keys[keyID]
	if !ok {
		return nil, ErrKeyNotFound
	}
	return key.metadata, nil
}

func (p *mockProvider) Encrypt(ctx context.Context, req *EncryptRequest) (*EncryptResponse, error) {
	p.mu.RLock()
	key, ok := p.keys[req.KeyID]
	p.mu.RUnlock()

	if !ok {
		return nil, ErrKeyNotFound
	}

	block, err := aes.NewCipher(key.plaintext)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, req.Plaintext, nil)

	return &EncryptResponse{
		Ciphertext: ciphertext,
		KeyID:      req.KeyID,
	}, nil
}

func (p *mockProvider) Decrypt(ctx context.Context, req *DecryptRequest) (*DecryptResponse, error) {
	p.mu.RLock()
	key, ok := p.keys[req.KeyID]
	p.mu.RUnlock()

	if !ok {
		return nil, ErrKeyNotFound
	}

	block, err := aes.NewCipher(key.plaintext)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	if len(req.Ciphertext) < gcm.NonceSize() {
		return nil, ErrInvalidCiphertext
	}

	nonce := req.Ciphertext[:gcm.NonceSize()]
	ciphertext := req.Ciphertext[gcm.NonceSize():]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return &DecryptResponse{
		Plaintext: plaintext,
		KeyID:     req.KeyID,
	}, nil
}

func (p *mockProvider) GenerateDataKey(ctx context.Context, req *GenerateDataKeyRequest) (*DataKey, error) {
	p.mu.RLock()
	key, ok := p.keys[req.KeyID]
	p.mu.RUnlock()

	if !ok {
		return nil, ErrKeyNotFound
	}

	// Generate random data key
	keySize := 32
	if req.NumberOfBytes > 0 {
		keySize = req.NumberOfBytes
	}

	plaintext := make([]byte, keySize)
	if _, err := io.ReadFull(rand.Reader, plaintext); err != nil {
		return nil, err
	}

	// Wrap the data key
	encResp, err := p.Encrypt(ctx, &EncryptRequest{
		KeyID:     req.KeyID,
		Plaintext: plaintext,
		Context:   req.Context,
	})
	if err != nil {
		return nil, err
	}

	dataKey := &DataKey{
		Ciphertext:  encResp.Ciphertext,
		KeyID:       req.KeyID,
		Provider:    p.Type(),
		KeySpec:     KeySpecAES256,
		GeneratedAt: time.Now(),
	}

	if !req.WithoutPlaintext {
		dataKey.Plaintext = plaintext
	} else {
		for i := range plaintext {
			plaintext[i] = 0
		}
	}

	_ = key // Silence unused variable
	return dataKey, nil
}

func (p *mockProvider) WrapKey(ctx context.Context, req *WrapKeyRequest) (*WrapKeyResponse, error) {
	encResp, err := p.Encrypt(ctx, &EncryptRequest{
		KeyID:     req.WrapperKeyID,
		Plaintext: req.KeyToWrap,
		Context:   req.Context,
	})
	if err != nil {
		return nil, err
	}

	return &WrapKeyResponse{
		WrappedKey:   encResp.Ciphertext,
		WrapperKeyID: req.WrapperKeyID,
	}, nil
}

func (p *mockProvider) UnwrapKey(ctx context.Context, req *UnwrapKeyRequest) (*UnwrapKeyResponse, error) {
	decResp, err := p.Decrypt(ctx, &DecryptRequest{
		KeyID:      req.WrapperKeyID,
		Ciphertext: req.WrappedKey,
		Context:    req.Context,
	})
	if err != nil {
		return nil, err
	}

	return &UnwrapKeyResponse{
		PlaintextKey: decResp.Plaintext,
		WrapperKeyID: req.WrapperKeyID,
	}, nil
}

func (p *mockProvider) Close() error {
	return nil
}

func (p *mockProvider) addKey(keyID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	keyMaterial := make([]byte, 32)
	rand.Read(keyMaterial)

	p.keys[keyID] = &mockKey{
		id:        keyID,
		plaintext: keyMaterial,
		metadata: &KeyMetadata{
			KeyID:    keyID,
			Provider: p.Type(),
			KeyType:  KeyTypeSymmetric,
			KeySpec:  KeySpecAES256,
			KeyUsage: KeyUsageEncryptDecrypt,
			Enabled:  true,
		},
	}
}

// =============================================================================
// Provider Tests
// =============================================================================

func TestProviderType(t *testing.T) {
	tests := []struct {
		pt    ProviderType
		valid bool
	}{
		{ProviderTypeAWS, true},
		{ProviderTypeAzure, true},
		{ProviderTypeGCP, true},
		{ProviderType("invalid"), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.pt), func(t *testing.T) {
			if got := tt.pt.Valid(); got != tt.valid {
				t.Errorf("Valid() = %v, want %v", got, tt.valid)
			}
		})
	}
}

func TestMockProvider_Encrypt_Decrypt(t *testing.T) {
	ctx := context.Background()
	provider := newMockProvider()
	provider.addKey("test-key")

	plaintext := []byte("hello, world")

	// Encrypt
	encResp, err := provider.Encrypt(ctx, &EncryptRequest{
		KeyID:     "test-key",
		Plaintext: plaintext,
	})
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if len(encResp.Ciphertext) == 0 {
		t.Fatal("Ciphertext is empty")
	}

	// Decrypt
	decResp, err := provider.Decrypt(ctx, &DecryptRequest{
		KeyID:      "test-key",
		Ciphertext: encResp.Ciphertext,
	})
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if string(decResp.Plaintext) != string(plaintext) {
		t.Errorf("Plaintext mismatch: got %s, want %s", decResp.Plaintext, plaintext)
	}
}

func TestMockProvider_GenerateDataKey(t *testing.T) {
	ctx := context.Background()
	provider := newMockProvider()
	provider.addKey("master-key")

	// Generate data key
	dataKey, err := provider.GenerateDataKey(ctx, &GenerateDataKeyRequest{
		KeyID:   "master-key",
		KeySpec: KeySpecAES256,
	})
	if err != nil {
		t.Fatalf("GenerateDataKey failed: %v", err)
	}

	if len(dataKey.Plaintext) != 32 {
		t.Errorf("Plaintext length = %d, want 32", len(dataKey.Plaintext))
	}

	if len(dataKey.Ciphertext) == 0 {
		t.Error("Ciphertext is empty")
	}

	// Verify we can unwrap the key
	unwrapped, err := provider.UnwrapKey(ctx, &UnwrapKeyRequest{
		WrapperKeyID: "master-key",
		WrappedKey:   dataKey.Ciphertext,
	})
	if err != nil {
		t.Fatalf("UnwrapKey failed: %v", err)
	}

	if string(unwrapped.PlaintextKey) != string(dataKey.Plaintext) {
		t.Error("Unwrapped key doesn't match original")
	}
}

func TestMockProvider_GenerateDataKey_WithoutPlaintext(t *testing.T) {
	ctx := context.Background()
	provider := newMockProvider()
	provider.addKey("master-key")

	dataKey, err := provider.GenerateDataKey(ctx, &GenerateDataKeyRequest{
		KeyID:            "master-key",
		WithoutPlaintext: true,
	})
	if err != nil {
		t.Fatalf("GenerateDataKey failed: %v", err)
	}

	if len(dataKey.Plaintext) != 0 {
		t.Error("Plaintext should be empty when WithoutPlaintext is true")
	}

	if len(dataKey.Ciphertext) == 0 {
		t.Error("Ciphertext is empty")
	}
}

func TestMockProvider_KeyNotFound(t *testing.T) {
	ctx := context.Background()
	provider := newMockProvider()

	_, err := provider.Encrypt(ctx, &EncryptRequest{
		KeyID:     "nonexistent",
		Plaintext: []byte("test"),
	})

	if !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("Expected ErrKeyNotFound, got %v", err)
	}
}

// =============================================================================
// Key Hierarchy Tests
// =============================================================================

func TestKeyHierarchyManager_GenerateDataKey(t *testing.T) {
	ctx := context.Background()
	provider := newMockProvider()
	provider.addKey("master-key")

	mgr, err := NewKeyHierarchyManager(provider, &KeyHierarchyConfig{
		MasterKeyID: "master-key",
	})
	if err != nil {
		t.Fatalf("NewKeyHierarchyManager failed: %v", err)
	}
	defer mgr.Stop()

	// Generate data key
	dk, err := mgr.GenerateDataKey(ctx, KeyPurposeCache)
	if err != nil {
		t.Fatalf("GenerateDataKey failed: %v", err)
	}

	if dk.Purpose != KeyPurposeCache {
		t.Errorf("Purpose = %s, want %s", dk.Purpose, KeyPurposeCache)
	}

	if dk.Version != 1 {
		t.Errorf("Version = %d, want 1", dk.Version)
	}

	if len(dk.plaintext) != 32 {
		t.Errorf("Plaintext length = %d, want 32", len(dk.plaintext))
	}
}

func TestKeyHierarchyManager_GetDataKey(t *testing.T) {
	ctx := context.Background()
	provider := newMockProvider()
	provider.addKey("master-key")

	mgr, err := NewKeyHierarchyManager(provider, &KeyHierarchyConfig{
		MasterKeyID: "master-key",
	})
	if err != nil {
		t.Fatalf("NewKeyHierarchyManager failed: %v", err)
	}
	defer mgr.Stop()

	// First get should generate a key
	dk1, err := mgr.GetDataKey(ctx, KeyPurposeCache)
	if err != nil {
		t.Fatalf("GetDataKey failed: %v", err)
	}

	// Second get should return the same key
	dk2, err := mgr.GetDataKey(ctx, KeyPurposeCache)
	if err != nil {
		t.Fatalf("GetDataKey failed: %v", err)
	}

	if dk1.Version != dk2.Version {
		t.Error("Second GetDataKey should return the same key")
	}
}

func TestKeyHierarchyManager_RotateDataKey(t *testing.T) {
	ctx := context.Background()
	provider := newMockProvider()
	provider.addKey("master-key")

	mgr, err := NewKeyHierarchyManager(provider, &KeyHierarchyConfig{
		MasterKeyID: "master-key",
	})
	if err != nil {
		t.Fatalf("NewKeyHierarchyManager failed: %v", err)
	}
	defer mgr.Stop()

	// Generate initial key
	dk1, err := mgr.GetDataKey(ctx, KeyPurposeCache)
	if err != nil {
		t.Fatalf("GetDataKey failed: %v", err)
	}

	// Rotate
	dk2, err := mgr.RotateDataKey(ctx, KeyPurposeCache)
	if err != nil {
		t.Fatalf("RotateDataKey failed: %v", err)
	}

	if dk2.Version != dk1.Version+1 {
		t.Errorf("Version = %d, want %d", dk2.Version, dk1.Version+1)
	}

	if string(dk2.plaintext) == string(dk1.plaintext) {
		t.Error("Rotated key should be different")
	}
}

func TestKeyHierarchyManager_DeriveKey(t *testing.T) {
	ctx := context.Background()
	provider := newMockProvider()
	provider.addKey("master-key")

	mgr, err := NewKeyHierarchyManager(provider, &KeyHierarchyConfig{
		MasterKeyID:         "master-key",
		CacheDataKeyLocally: true,
		DataKeyCacheTTL:     time.Minute,
	})
	if err != nil {
		t.Fatalf("NewKeyHierarchyManager failed: %v", err)
	}
	defer mgr.Stop()

	// Derive keys with different info
	key1, err := mgr.DeriveKey(ctx, KeyPurposeCache, []byte("purpose1"), 32)
	if err != nil {
		t.Fatalf("DeriveKey failed: %v", err)
	}

	key2, err := mgr.DeriveKey(ctx, KeyPurposeCache, []byte("purpose2"), 32)
	if err != nil {
		t.Fatalf("DeriveKey failed: %v", err)
	}

	if string(key1) == string(key2) {
		t.Error("Derived keys with different info should be different")
	}

	// Same derivation should return same key
	key1b, err := mgr.DeriveKey(ctx, KeyPurposeCache, []byte("purpose1"), 32)
	if err != nil {
		t.Fatalf("DeriveKey failed: %v", err)
	}

	if string(key1) != string(key1b) {
		t.Error("Same derivation should return same key")
	}
}

func TestKeyHierarchyManager_ListDataKeys(t *testing.T) {
	ctx := context.Background()
	provider := newMockProvider()
	provider.addKey("master-key")

	mgr, err := NewKeyHierarchyManager(provider, &KeyHierarchyConfig{
		MasterKeyID: "master-key",
	})
	if err != nil {
		t.Fatalf("NewKeyHierarchyManager failed: %v", err)
	}
	defer mgr.Stop()

	// Generate keys for different purposes
	mgr.GenerateDataKey(ctx, KeyPurposeCache)
	mgr.GenerateDataKey(ctx, KeyPurposeAudit)

	keys := mgr.ListDataKeys()
	if len(keys) != 2 {
		t.Errorf("Expected 2 keys, got %d", len(keys))
	}
}

// =============================================================================
// Envelope Encryption Tests
// =============================================================================

func TestEnvelopeEncryptor(t *testing.T) {
	ctx := context.Background()
	provider := newMockProvider()
	provider.addKey("master-key")

	mgr, err := NewKeyHierarchyManager(provider, &KeyHierarchyConfig{
		MasterKeyID: "master-key",
	})
	if err != nil {
		t.Fatalf("NewKeyHierarchyManager failed: %v", err)
	}
	defer mgr.Stop()

	encryptor := NewEnvelopeEncryptor(mgr, KeyPurposeCache)

	plaintext := []byte("sensitive data")

	// Encrypt
	envelope, err := encryptor.Encrypt(ctx, plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if len(envelope.Ciphertext) == 0 {
		t.Error("Ciphertext is empty")
	}

	if len(envelope.WrappedKey) == 0 {
		t.Error("WrappedKey is empty")
	}

	// Decrypt
	decrypted, err := encryptor.Decrypt(ctx, envelope)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("Plaintext mismatch: got %s, want %s", decrypted, plaintext)
	}
}

// =============================================================================
// KMS Secret Cache Tests
// =============================================================================

func TestSecretCache_PutGet(t *testing.T) {
	ctx := context.Background()
	provider := newMockProvider()
	provider.addKey("master-key")

	mgr, err := NewKeyHierarchyManager(provider, &KeyHierarchyConfig{
		MasterKeyID: "master-key",
	})
	if err != nil {
		t.Fatalf("NewKeyHierarchyManager failed: %v", err)
	}
	defer mgr.Stop()

	cache, err := NewSecretCache(ctx, mgr, nil)
	if err != nil {
		t.Fatalf("NewSecretCache failed: %v", err)
	}
	defer cache.Close()

	secret := &secrets.Secret{
		Path: "test/secret",
		Data: map[string]interface{}{
			"username": "admin",
			"password": "secret123",
		},
	}

	// Put
	err = cache.Put(ctx, secret, 5*time.Minute)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Get
	retrieved, ok := cache.Get(ctx, "test/secret")
	if !ok {
		t.Fatal("Get returned false")
	}

	if retrieved.Path != secret.Path {
		t.Errorf("Path mismatch: got %s, want %s", retrieved.Path, secret.Path)
	}

	username, _ := retrieved.GetString("username")
	if username != "admin" {
		t.Errorf("username = %s, want admin", username)
	}
}

func TestSecretCache_Expiration(t *testing.T) {
	ctx := context.Background()
	provider := newMockProvider()
	provider.addKey("master-key")

	mgr, err := NewKeyHierarchyManager(provider, &KeyHierarchyConfig{
		MasterKeyID: "master-key",
	})
	if err != nil {
		t.Fatalf("NewKeyHierarchyManager failed: %v", err)
	}
	defer mgr.Stop()

	cache, err := NewSecretCache(ctx, mgr, &SecretCacheConfig{
		MaxEntries:      100,
		CleanupInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewSecretCache failed: %v", err)
	}
	defer cache.Close()

	secret := &secrets.Secret{
		Path: "test/secret",
		Data: map[string]interface{}{"key": "value"},
	}

	// Put with very short TTL
	err = cache.Put(ctx, secret, 1*time.Millisecond)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Wait for expiration
	time.Sleep(50 * time.Millisecond)

	// Should not find expired entry
	_, ok := cache.Get(ctx, "test/secret")
	if ok {
		t.Error("Expected Get to return false for expired entry")
	}
}

func TestSecretCache_Delete(t *testing.T) {
	ctx := context.Background()
	provider := newMockProvider()
	provider.addKey("master-key")

	mgr, err := NewKeyHierarchyManager(provider, &KeyHierarchyConfig{
		MasterKeyID: "master-key",
	})
	if err != nil {
		t.Fatalf("NewKeyHierarchyManager failed: %v", err)
	}
	defer mgr.Stop()

	cache, err := NewSecretCache(ctx, mgr, nil)
	if err != nil {
		t.Fatalf("NewSecretCache failed: %v", err)
	}
	defer cache.Close()

	secret := &secrets.Secret{
		Path: "test/secret",
		Data: map[string]interface{}{"key": "value"},
	}

	cache.Put(ctx, secret, 5*time.Minute)
	cache.Delete(ctx, "test/secret")

	_, ok := cache.Get(ctx, "test/secret")
	if ok {
		t.Error("Expected Get to return false after delete")
	}
}

func TestSecretCache_DeleteByPrefix(t *testing.T) {
	ctx := context.Background()
	provider := newMockProvider()
	provider.addKey("master-key")

	mgr, err := NewKeyHierarchyManager(provider, &KeyHierarchyConfig{
		MasterKeyID: "master-key",
	})
	if err != nil {
		t.Fatalf("NewKeyHierarchyManager failed: %v", err)
	}
	defer mgr.Stop()

	cache, err := NewSecretCache(ctx, mgr, nil)
	if err != nil {
		t.Fatalf("NewSecretCache failed: %v", err)
	}
	defer cache.Close()

	// Add multiple secrets
	cache.Put(ctx, &secrets.Secret{Path: "app/config", Data: map[string]interface{}{}}, 5*time.Minute)
	cache.Put(ctx, &secrets.Secret{Path: "app/secrets", Data: map[string]interface{}{}}, 5*time.Minute)
	cache.Put(ctx, &secrets.Secret{Path: "other/config", Data: map[string]interface{}{}}, 5*time.Minute)

	// Delete by prefix
	count, err := cache.DeleteByPrefix(ctx, "app/")
	if err != nil {
		t.Fatalf("DeleteByPrefix failed: %v", err)
	}

	if count != 2 {
		t.Errorf("Expected 2 deletions, got %d", count)
	}

	// Verify
	_, ok := cache.Get(ctx, "app/config")
	if ok {
		t.Error("app/config should be deleted")
	}

	_, ok = cache.Get(ctx, "other/config")
	if !ok {
		t.Error("other/config should still exist")
	}
}

func TestSecretCache_RotateKey(t *testing.T) {
	ctx := context.Background()
	provider := newMockProvider()
	provider.addKey("master-key")

	mgr, err := NewKeyHierarchyManager(provider, &KeyHierarchyConfig{
		MasterKeyID: "master-key",
	})
	if err != nil {
		t.Fatalf("NewKeyHierarchyManager failed: %v", err)
	}
	defer mgr.Stop()

	cache, err := NewSecretCache(ctx, mgr, nil)
	if err != nil {
		t.Fatalf("NewSecretCache failed: %v", err)
	}
	defer cache.Close()

	secret := &secrets.Secret{
		Path: "test/secret",
		Data: map[string]interface{}{"key": "value"},
	}

	cache.Put(ctx, secret, 5*time.Minute)

	oldVersion := cache.KeyVersion()

	// Rotate key
	err = cache.RotateKey(ctx)
	if err != nil {
		t.Fatalf("RotateKey failed: %v", err)
	}

	newVersion := cache.KeyVersion()
	if newVersion <= oldVersion {
		t.Errorf("Key version should increase: old=%d, new=%d", oldVersion, newVersion)
	}

	// Verify we can still read the secret
	retrieved, ok := cache.Get(ctx, "test/secret")
	if !ok {
		t.Fatal("Get returned false after key rotation")
	}

	if retrieved.Path != secret.Path {
		t.Error("Secret corrupted after key rotation")
	}
}

func TestSecretCache_Stats(t *testing.T) {
	ctx := context.Background()
	provider := newMockProvider()
	provider.addKey("master-key")

	mgr, err := NewKeyHierarchyManager(provider, &KeyHierarchyConfig{
		MasterKeyID: "master-key",
	})
	if err != nil {
		t.Fatalf("NewKeyHierarchyManager failed: %v", err)
	}
	defer mgr.Stop()

	cache, err := NewSecretCache(ctx, mgr, nil)
	if err != nil {
		t.Fatalf("NewSecretCache failed: %v", err)
	}
	defer cache.Close()

	// Add some secrets
	for i := 0; i < 5; i++ {
		cache.Put(ctx, &secrets.Secret{
			Path: "test/secret" + string(rune('0'+i)),
			Data: map[string]interface{}{"key": "value"},
		}, 5*time.Minute)
	}

	// Generate some hits and misses
	cache.Get(ctx, "test/secret0")
	cache.Get(ctx, "test/secret1")
	cache.Get(ctx, "nonexistent")

	stats := cache.Stats()

	if stats.Entries != 5 {
		t.Errorf("Entries = %d, want 5", stats.Entries)
	}

	if stats.Hits != 2 {
		t.Errorf("Hits = %d, want 2", stats.Hits)
	}

	if stats.Misses != 1 {
		t.Errorf("Misses = %d, want 1", stats.Misses)
	}
}

// =============================================================================
// Multi-Tier Cache Tests
// =============================================================================

func TestMultiTierSecretCache_PutGet(t *testing.T) {
	ctx := context.Background()
	provider := newMockProvider()
	provider.addKey("master-key")

	mgr, err := NewKeyHierarchyManager(provider, &KeyHierarchyConfig{
		MasterKeyID: "master-key",
	})
	if err != nil {
		t.Fatalf("NewKeyHierarchyManager failed: %v", err)
	}
	defer mgr.Stop()

	cache, err := NewMultiTierSecretCache(ctx, mgr, nil)
	if err != nil {
		t.Fatalf("NewMultiTierSecretCache failed: %v", err)
	}
	defer cache.Close()

	secret := &secrets.Secret{
		Path: "test/secret",
		Data: map[string]interface{}{"key": "value"},
	}

	// Put
	err = cache.Put(ctx, secret, 5*time.Minute)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Get (should hit L1)
	retrieved, ok := cache.Get(ctx, "test/secret")
	if !ok {
		t.Fatal("Get returned false")
	}

	if retrieved.Path != secret.Path {
		t.Errorf("Path mismatch: got %s, want %s", retrieved.Path, secret.Path)
	}
}

// =============================================================================
// DerivedKey Serialization Tests
// =============================================================================

func TestDerivedKey_MarshalBinary(t *testing.T) {
	original := &DerivedKey{
		Purpose:    KeyPurposeCache,
		Version:    5,
		Ciphertext: []byte("encrypted-key-material"),
		CreatedAt:  time.Now().Truncate(time.Second),
		ExpiresAt:  time.Now().Add(24 * time.Hour).Truncate(time.Second),
	}

	data, err := original.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary failed: %v", err)
	}

	restored := &DerivedKey{}
	err = restored.UnmarshalBinary(data)
	if err != nil {
		t.Fatalf("UnmarshalBinary failed: %v", err)
	}

	if restored.Purpose != original.Purpose {
		t.Errorf("Purpose = %s, want %s", restored.Purpose, original.Purpose)
	}

	if restored.Version != original.Version {
		t.Errorf("Version = %d, want %d", restored.Version, original.Version)
	}

	if string(restored.Ciphertext) != string(original.Ciphertext) {
		t.Error("Ciphertext mismatch")
	}

	if !restored.CreatedAt.Equal(original.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", restored.CreatedAt, original.CreatedAt)
	}

	if !restored.ExpiresAt.Equal(original.ExpiresAt) {
		t.Errorf("ExpiresAt = %v, want %v", restored.ExpiresAt, original.ExpiresAt)
	}
}

func TestDerivedKey_Zero(t *testing.T) {
	dk := &DerivedKey{
		plaintext: []byte("sensitive-key-material"),
	}

	dk.Zero()

	for _, b := range dk.plaintext {
		if b != 0 {
			t.Error("Plaintext not zeroed")
			break
		}
	}
}

func TestDataKey_Zero(t *testing.T) {
	dk := &DataKey{
		Plaintext: []byte("sensitive-data-key"),
	}

	dk.Zero()

	for _, b := range dk.Plaintext {
		if b != 0 {
			t.Error("Plaintext not zeroed")
			break
		}
	}
}

// =============================================================================
// Benchmark Tests
// =============================================================================

func BenchmarkMockProvider_Encrypt(b *testing.B) {
	ctx := context.Background()
	provider := newMockProvider()
	provider.addKey("bench-key")

	plaintext := make([]byte, 1024)
	rand.Read(plaintext)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		provider.Encrypt(ctx, &EncryptRequest{
			KeyID:     "bench-key",
			Plaintext: plaintext,
		})
	}
}

func BenchmarkKeyHierarchy_DeriveKey(b *testing.B) {
	ctx := context.Background()
	provider := newMockProvider()
	provider.addKey("master-key")

	mgr, _ := NewKeyHierarchyManager(provider, &KeyHierarchyConfig{
		MasterKeyID:         "master-key",
		CacheDataKeyLocally: true,
		DataKeyCacheTTL:     time.Hour,
	})
	defer mgr.Stop()

	info := []byte("benchmark-info")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mgr.DeriveKey(ctx, KeyPurposeCache, info, 32)
	}
}

func BenchmarkSecretCache_Put(b *testing.B) {
	ctx := context.Background()
	provider := newMockProvider()
	provider.addKey("master-key")

	mgr, _ := NewKeyHierarchyManager(provider, &KeyHierarchyConfig{
		MasterKeyID: "master-key",
	})
	defer mgr.Stop()

	cache, _ := NewSecretCache(ctx, mgr, nil)
	defer cache.Close()

	secret := &secrets.Secret{
		Path: "bench/secret",
		Data: map[string]interface{}{"key": "value"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Put(ctx, secret, 5*time.Minute)
	}
}

func BenchmarkSecretCache_Get(b *testing.B) {
	ctx := context.Background()
	provider := newMockProvider()
	provider.addKey("master-key")

	mgr, _ := NewKeyHierarchyManager(provider, &KeyHierarchyConfig{
		MasterKeyID: "master-key",
	})
	defer mgr.Stop()

	cache, _ := NewSecretCache(ctx, mgr, nil)
	defer cache.Close()

	secret := &secrets.Secret{
		Path: "bench/secret",
		Data: map[string]interface{}{"key": "value"},
	}
	cache.Put(ctx, secret, 5*time.Minute)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get(ctx, "bench/secret")
	}
}
