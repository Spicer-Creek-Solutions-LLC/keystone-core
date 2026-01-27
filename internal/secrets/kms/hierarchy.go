package kms

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/hkdf"
)

// KeyPurpose represents the purpose of a derived key.
type KeyPurpose string

const (
	KeyPurposeCache     KeyPurpose = "cache"     // Cache encryption key
	KeyPurposeAudit     KeyPurpose = "audit"     // Audit log encryption key
	KeyPurposeTransit   KeyPurpose = "transit"   // Transit encryption key
	KeyPurposeBackup    KeyPurpose = "backup"    // Backup encryption key
	KeyPurposeAgent     KeyPurpose = "agent"     // Agent communication key
	KeyPurposeInternal  KeyPurpose = "internal"  // Internal service key
)

// KeyHierarchyConfig configures the key hierarchy manager.
type KeyHierarchyConfig struct {
	// Provider is the KMS provider configuration.
	Provider ProviderConfig `json:"provider"`

	// MasterKeyID is the KMS key ID for the master key.
	MasterKeyID string `json:"master_key_id"`

	// RotationInterval is how often to rotate derived keys.
	RotationInterval time.Duration `json:"rotation_interval,omitempty"`

	// KeyDerivationInfo is additional info for key derivation.
	KeyDerivationInfo string `json:"key_derivation_info,omitempty"`

	// CacheDataKeyLocally caches derived data keys in memory.
	CacheDataKeyLocally bool `json:"cache_data_key_locally,omitempty"`

	// DataKeyCacheTTL is the TTL for cached data keys.
	DataKeyCacheTTL time.Duration `json:"data_key_cache_ttl,omitempty"`
}

// DefaultKeyHierarchyConfig returns default key hierarchy configuration.
func DefaultKeyHierarchyConfig() *KeyHierarchyConfig {
	return &KeyHierarchyConfig{
		RotationInterval:    24 * time.Hour,
		KeyDerivationInfo:   "keystone-core-key-hierarchy",
		CacheDataKeyLocally: true,
		DataKeyCacheTTL:     time.Hour,
	}
}

// DerivedKey represents a derived data encryption key.
type DerivedKey struct {
	// Purpose is the key purpose.
	Purpose KeyPurpose `json:"purpose"`

	// Version is the key version.
	Version int `json:"version"`

	// Ciphertext is the encrypted key material (wrapped by master key).
	Ciphertext []byte `json:"ciphertext"`

	// CreatedAt is when the key was created.
	CreatedAt time.Time `json:"created_at"`

	// ExpiresAt is when the key expires (zero = no expiry).
	ExpiresAt time.Time `json:"expires_at,omitempty"`

	// Context is the encryption context used for wrapping.
	Context map[string]string `json:"context,omitempty"`

	// plaintext is the decrypted key material (only in memory).
	plaintext []byte
}

// Zero clears the plaintext key from memory.
func (d *DerivedKey) Zero() {
	for i := range d.plaintext {
		d.plaintext[i] = 0
	}
	d.plaintext = nil
}

// KeyHierarchyManager manages a hierarchy of encryption keys.
type KeyHierarchyManager struct {
	config   *KeyHierarchyConfig
	provider Provider

	mu          sync.RWMutex
	derivedKeys map[KeyPurpose]*DerivedKey
	keyCache    map[string]*cachedDataKey

	closed  atomic.Bool
	closeCh chan struct{}
	wg      sync.WaitGroup
}

// cachedDataKey holds a cached data key with TTL.
type cachedDataKey struct {
	key       []byte
	expiresAt time.Time
}

// NewKeyHierarchyManager creates a new key hierarchy manager.
func NewKeyHierarchyManager(provider Provider, config *KeyHierarchyConfig) (*KeyHierarchyManager, error) {
	if provider == nil {
		return nil, errors.New("provider is required")
	}

	if config == nil {
		config = DefaultKeyHierarchyConfig()
	}

	if config.MasterKeyID == "" {
		return nil, errors.New("master_key_id is required")
	}

	mgr := &KeyHierarchyManager{
		config:      config,
		provider:    provider,
		derivedKeys: make(map[KeyPurpose]*DerivedKey),
		keyCache:    make(map[string]*cachedDataKey),
		closeCh:     make(chan struct{}),
	}

	return mgr, nil
}

// Start starts the key hierarchy manager.
func (m *KeyHierarchyManager) Start(ctx context.Context) error {
	// Verify master key is accessible
	if !m.provider.Healthy(ctx) {
		return ErrProviderUnavailable
	}

	// Start cache cleanup goroutine
	m.wg.Add(1)
	go m.cleanupLoop()

	return nil
}

// Stop stops the key hierarchy manager.
func (m *KeyHierarchyManager) Stop() error {
	if m.closed.Swap(true) {
		return nil
	}

	close(m.closeCh)
	m.wg.Wait()

	// Clear all cached keys
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, dk := range m.derivedKeys {
		dk.Zero()
	}
	m.derivedKeys = make(map[KeyPurpose]*DerivedKey)

	for _, ck := range m.keyCache {
		for i := range ck.key {
			ck.key[i] = 0
		}
	}
	m.keyCache = make(map[string]*cachedDataKey)

	return nil
}

// GenerateDataKey generates a new data encryption key for a purpose.
func (m *KeyHierarchyManager) GenerateDataKey(ctx context.Context, purpose KeyPurpose) (*DerivedKey, error) {
	if m.closed.Load() {
		return nil, errors.New("manager is closed")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Determine version (increment from existing)
	version := 1
	if existing, ok := m.derivedKeys[purpose]; ok {
		version = existing.Version + 1
	}

	// Create encryption context
	encContext := map[string]string{
		"purpose": string(purpose),
		"version": fmt.Sprintf("%d", version),
		"info":    m.config.KeyDerivationInfo,
	}

	// Generate data key using KMS
	dataKey, err := m.provider.GenerateDataKey(ctx, &GenerateDataKeyRequest{
		KeyID:         m.config.MasterKeyID,
		KeySpec:       KeySpecAES256,
		Context:       encContext,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate data key: %w", err)
	}

	now := time.Now()
	derivedKey := &DerivedKey{
		Purpose:    purpose,
		Version:    version,
		Ciphertext: dataKey.Ciphertext,
		CreatedAt:  now,
		Context:    encContext,
		plaintext:  dataKey.Plaintext,
	}

	if m.config.RotationInterval > 0 {
		derivedKey.ExpiresAt = now.Add(m.config.RotationInterval)
	}

	// Store the derived key
	m.derivedKeys[purpose] = derivedKey

	return derivedKey, nil
}

// GetDataKey retrieves the current data key for a purpose.
func (m *KeyHierarchyManager) GetDataKey(ctx context.Context, purpose KeyPurpose) (*DerivedKey, error) {
	if m.closed.Load() {
		return nil, errors.New("manager is closed")
	}

	m.mu.RLock()
	dk, exists := m.derivedKeys[purpose]
	m.mu.RUnlock()

	if !exists {
		// Generate a new key if none exists
		return m.GenerateDataKey(ctx, purpose)
	}

	// Check if key needs rotation
	if !dk.ExpiresAt.IsZero() && time.Now().After(dk.ExpiresAt) {
		return m.RotateDataKey(ctx, purpose)
	}

	// Ensure plaintext is available
	if len(dk.plaintext) == 0 {
		// Unwrap the key
		unwrapped, err := m.provider.UnwrapKey(ctx, &UnwrapKeyRequest{
			WrapperKeyID: m.config.MasterKeyID,
			WrappedKey:   dk.Ciphertext,
			Context:      dk.Context,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to unwrap data key: %w", err)
		}

		m.mu.Lock()
		dk.plaintext = unwrapped.PlaintextKey
		m.mu.Unlock()
	}

	return dk, nil
}

// GetKeyMaterial retrieves the plaintext key material for a purpose.
func (m *KeyHierarchyManager) GetKeyMaterial(ctx context.Context, purpose KeyPurpose) ([]byte, error) {
	dk, err := m.GetDataKey(ctx, purpose)
	if err != nil {
		return nil, err
	}

	// Return a copy of the plaintext
	key := make([]byte, len(dk.plaintext))
	copy(key, dk.plaintext)
	return key, nil
}

// RotateDataKey rotates the data key for a purpose.
func (m *KeyHierarchyManager) RotateDataKey(ctx context.Context, purpose KeyPurpose) (*DerivedKey, error) {
	if m.closed.Load() {
		return nil, errors.New("manager is closed")
	}

	// Generate new key (automatically increments version)
	newKey, err := m.GenerateDataKey(ctx, purpose)
	if err != nil {
		return nil, err
	}

	return newKey, nil
}

// DeriveKey derives a purpose-specific key from the data key using HKDF.
func (m *KeyHierarchyManager) DeriveKey(ctx context.Context, purpose KeyPurpose, info []byte, keyLen int) ([]byte, error) {
	dk, err := m.GetDataKey(ctx, purpose)
	if err != nil {
		return nil, err
	}

	// Create cache key
	cacheKey := fmt.Sprintf("%s:%d:%x", purpose, dk.Version, info)

	// Check cache
	if m.config.CacheDataKeyLocally {
		m.mu.RLock()
		if cached, ok := m.keyCache[cacheKey]; ok && time.Now().Before(cached.expiresAt) {
			key := make([]byte, len(cached.key))
			copy(key, cached.key)
			m.mu.RUnlock()
			return key, nil
		}
		m.mu.RUnlock()
	}

	// Derive key using HKDF
	hkdfReader := hkdf.New(sha256.New, dk.plaintext, nil, info)
	derivedKey := make([]byte, keyLen)
	if _, err := io.ReadFull(hkdfReader, derivedKey); err != nil {
		return nil, fmt.Errorf("failed to derive key: %w", err)
	}

	// Cache the derived key
	if m.config.CacheDataKeyLocally {
		m.mu.Lock()
		m.keyCache[cacheKey] = &cachedDataKey{
			key:       derivedKey,
			expiresAt: time.Now().Add(m.config.DataKeyCacheTTL),
		}
		m.mu.Unlock()
	}

	// Return a copy
	key := make([]byte, len(derivedKey))
	copy(key, derivedKey)
	return key, nil
}

// ImportDataKey imports an existing wrapped data key.
func (m *KeyHierarchyManager) ImportDataKey(ctx context.Context, purpose KeyPurpose, wrappedKey []byte, version int, context map[string]string) error {
	if m.closed.Load() {
		return errors.New("manager is closed")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.derivedKeys[purpose] = &DerivedKey{
		Purpose:    purpose,
		Version:    version,
		Ciphertext: wrappedKey,
		CreatedAt:  time.Now(),
		Context:    context,
	}

	return nil
}

// ExportDataKey exports a wrapped data key (for backup/replication).
func (m *KeyHierarchyManager) ExportDataKey(ctx context.Context, purpose KeyPurpose) ([]byte, error) {
	if m.closed.Load() {
		return nil, errors.New("manager is closed")
	}

	m.mu.RLock()
	dk, exists := m.derivedKeys[purpose]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("no data key for purpose: %s", purpose)
	}

	// Return the wrapped key (ciphertext)
	result := make([]byte, len(dk.Ciphertext))
	copy(result, dk.Ciphertext)
	return result, nil
}

// ListDataKeys lists all managed data keys.
func (m *KeyHierarchyManager) ListDataKeys() []DerivedKeyInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var keys []DerivedKeyInfo
	for _, dk := range m.derivedKeys {
		keys = append(keys, DerivedKeyInfo{
			Purpose:   dk.Purpose,
			Version:   dk.Version,
			CreatedAt: dk.CreatedAt,
			ExpiresAt: dk.ExpiresAt,
		})
	}

	return keys
}

// DerivedKeyInfo contains information about a derived key (without sensitive data).
type DerivedKeyInfo struct {
	Purpose   KeyPurpose `json:"purpose"`
	Version   int        `json:"version"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at,omitempty"`
}

// cleanupLoop periodically cleans up expired cache entries.
func (m *KeyHierarchyManager) cleanupLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-m.closeCh:
			return
		case <-ticker.C:
			m.cleanupCache()
		}
	}
}

// cleanupCache removes expired cache entries.
func (m *KeyHierarchyManager) cleanupCache() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for key, cached := range m.keyCache {
		if now.After(cached.expiresAt) {
			// Zero the key before deletion
			for i := range cached.key {
				cached.key[i] = 0
			}
			delete(m.keyCache, key)
		}
	}
}

// Stats returns key hierarchy statistics.
func (m *KeyHierarchyManager) Stats() KeyHierarchyStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := KeyHierarchyStats{
		DerivedKeyCount: len(m.derivedKeys),
		CachedKeyCount:  len(m.keyCache),
		KeysByPurpose:   make(map[KeyPurpose]int),
	}

	for purpose := range m.derivedKeys {
		stats.KeysByPurpose[purpose]++
	}

	return stats
}

// KeyHierarchyStats contains key hierarchy statistics.
type KeyHierarchyStats struct {
	DerivedKeyCount int                  `json:"derived_key_count"`
	CachedKeyCount  int                  `json:"cached_key_count"`
	KeysByPurpose   map[KeyPurpose]int   `json:"keys_by_purpose"`
}

// =============================================================================
// Envelope Encryption Helpers
// =============================================================================

// EnvelopeEncryptor provides envelope encryption using the key hierarchy.
type EnvelopeEncryptor struct {
	manager *KeyHierarchyManager
	purpose KeyPurpose
}

// NewEnvelopeEncryptor creates a new envelope encryptor.
func NewEnvelopeEncryptor(manager *KeyHierarchyManager, purpose KeyPurpose) *EnvelopeEncryptor {
	return &EnvelopeEncryptor{
		manager: manager,
		purpose: purpose,
	}
}

// EncryptedEnvelope represents encrypted data with the wrapped data key.
type EncryptedEnvelope struct {
	// Ciphertext is the encrypted data.
	Ciphertext []byte `json:"ciphertext"`

	// WrappedKey is the encrypted data key.
	WrappedKey []byte `json:"wrapped_key"`

	// KeyVersion is the key version used.
	KeyVersion int `json:"key_version"`

	// Nonce is the encryption nonce.
	Nonce []byte `json:"nonce"`

	// Algorithm is the encryption algorithm.
	Algorithm string `json:"algorithm"`
}

// Encrypt encrypts data using envelope encryption.
func (e *EnvelopeEncryptor) Encrypt(ctx context.Context, plaintext []byte) (*EncryptedEnvelope, error) {
	// Get the current data key
	dk, err := e.manager.GetDataKey(ctx, e.purpose)
	if err != nil {
		return nil, err
	}

	// Create AES-GCM cipher
	block, err := aes.NewCipher(dk.plaintext)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	return &EncryptedEnvelope{
		Ciphertext: ciphertext,
		WrappedKey: dk.Ciphertext,
		KeyVersion: dk.Version,
		Nonce:      nonce,
		Algorithm:  "AES-256-GCM",
	}, nil
}

// Decrypt decrypts an encrypted envelope.
func (e *EnvelopeEncryptor) Decrypt(ctx context.Context, envelope *EncryptedEnvelope) ([]byte, error) {
	// Unwrap the data key
	unwrapped, err := e.manager.provider.UnwrapKey(ctx, &UnwrapKeyRequest{
		WrapperKeyID: e.manager.config.MasterKeyID,
		WrappedKey:   envelope.WrappedKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to unwrap key: %w", err)
	}
	defer func() {
		for i := range unwrapped.PlaintextKey {
			unwrapped.PlaintextKey[i] = 0
		}
	}()

	// Create AES-GCM cipher
	block, err := aes.NewCipher(unwrapped.PlaintextKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Decrypt
	plaintext, err := gcm.Open(nil, envelope.Nonce, envelope.Ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}

// =============================================================================
// Serialization
// =============================================================================

// MarshalJSON implements json.Marshaler for DerivedKey.
func (d *DerivedKey) MarshalJSON() ([]byte, error) {
	type Alias DerivedKey
	return json.Marshal(&struct {
		*Alias
		// Exclude plaintext from serialization
	}{
		Alias: (*Alias)(d),
	})
}

// MarshalBinary serializes a derived key to binary format.
func (d *DerivedKey) MarshalBinary() ([]byte, error) {
	// Format: version (4 bytes) + purpose len (2 bytes) + purpose + ciphertext len (4 bytes) + ciphertext + created (8 bytes) + expires (8 bytes)
	purposeBytes := []byte(d.Purpose)

	buf := make([]byte, 4+2+len(purposeBytes)+4+len(d.Ciphertext)+8+8)
	offset := 0

	binary.BigEndian.PutUint32(buf[offset:], uint32(d.Version))
	offset += 4

	binary.BigEndian.PutUint16(buf[offset:], uint16(len(purposeBytes)))
	offset += 2

	copy(buf[offset:], purposeBytes)
	offset += len(purposeBytes)

	binary.BigEndian.PutUint32(buf[offset:], uint32(len(d.Ciphertext)))
	offset += 4

	copy(buf[offset:], d.Ciphertext)
	offset += len(d.Ciphertext)

	binary.BigEndian.PutUint64(buf[offset:], uint64(d.CreatedAt.Unix()))
	offset += 8

	binary.BigEndian.PutUint64(buf[offset:], uint64(d.ExpiresAt.Unix()))

	return buf, nil
}

// UnmarshalBinary deserializes a derived key from binary format.
func (d *DerivedKey) UnmarshalBinary(data []byte) error {
	if len(data) < 18 { // Minimum size
		return errors.New("data too short")
	}

	offset := 0

	d.Version = int(binary.BigEndian.Uint32(data[offset:]))
	offset += 4

	purposeLen := int(binary.BigEndian.Uint16(data[offset:]))
	offset += 2

	if len(data) < offset+purposeLen {
		return errors.New("data too short for purpose")
	}
	d.Purpose = KeyPurpose(data[offset : offset+purposeLen])
	offset += purposeLen

	if len(data) < offset+4 {
		return errors.New("data too short for ciphertext length")
	}
	ciphertextLen := int(binary.BigEndian.Uint32(data[offset:]))
	offset += 4

	if len(data) < offset+ciphertextLen {
		return errors.New("data too short for ciphertext")
	}
	d.Ciphertext = make([]byte, ciphertextLen)
	copy(d.Ciphertext, data[offset:offset+ciphertextLen])
	offset += ciphertextLen

	if len(data) < offset+16 {
		return errors.New("data too short for timestamps")
	}
	d.CreatedAt = time.Unix(int64(binary.BigEndian.Uint64(data[offset:])), 0)
	offset += 8

	expiresUnix := int64(binary.BigEndian.Uint64(data[offset:]))
	if expiresUnix > 0 {
		d.ExpiresAt = time.Unix(expiresUnix, 0)
	}

	return nil
}
