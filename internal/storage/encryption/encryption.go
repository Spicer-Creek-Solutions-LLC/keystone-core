// Package encryption provides encryption at rest for Keystone storage backends.
package encryption

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

// Common errors.
var (
	ErrKeyNotFound       = errors.New("encryption key not found")
	ErrKeyExpired        = errors.New("encryption key has expired")
	ErrDecryptionFailed  = errors.New("decryption failed")
	ErrInvalidCiphertext = errors.New("invalid ciphertext")
	ErrKeyProviderFailed = errors.New("key provider failed")
)

// Algorithm represents an encryption algorithm.
type Algorithm string

const (
	// AlgorithmAES256GCM uses AES-256 in GCM mode.
	AlgorithmAES256GCM Algorithm = "AES-256-GCM"
	// AlgorithmAES256CBC uses AES-256 in CBC mode.
	AlgorithmAES256CBC Algorithm = "AES-256-CBC"
	// AlgorithmChaCha20Poly1305 uses ChaCha20-Poly1305.
	AlgorithmChaCha20Poly1305 Algorithm = "ChaCha20-Poly1305"
)

// KeyType represents the type of encryption key.
type KeyType string

const (
	// KeyTypeMaster is a master encryption key.
	KeyTypeMaster KeyType = "master"
	// KeyTypeData is a data encryption key.
	KeyTypeData KeyType = "data"
	// KeyTypeWrapping is a key wrapping key.
	KeyTypeWrapping KeyType = "wrapping"
)

// Key represents an encryption key.
type Key struct {
	ID          string    `json:"id"`
	Type        KeyType   `json:"type"`
	Algorithm   Algorithm `json:"algorithm"`
	Material    []byte    `json:"-"` // Never serialized
	CreatedAt   time.Time `json:"createdAt"`
	ExpiresAt   time.Time `json:"expiresAt,omitempty"`
	RotatedFrom string    `json:"rotatedFrom,omitempty"`
	Version     int       `json:"version"`
}

// IsExpired returns true if the key has expired.
func (k *Key) IsExpired() bool {
	if k.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(k.ExpiresAt)
}

// KeyProvider provides encryption keys.
type KeyProvider interface {
	// GetKey retrieves a key by ID.
	GetKey(ctx context.Context, keyID string) (*Key, error)
	// GetCurrentKey retrieves the current key for a type.
	GetCurrentKey(ctx context.Context, keyType KeyType) (*Key, error)
	// RotateKey creates a new key and marks it as current.
	RotateKey(ctx context.Context, keyType KeyType) (*Key, error)
	// ListKeys lists all keys of a type.
	ListKeys(ctx context.Context, keyType KeyType) ([]*Key, error)
}

// EncryptedData represents encrypted data with metadata.
type EncryptedData struct {
	KeyID      string    `json:"keyId"`
	Algorithm  Algorithm `json:"algorithm"`
	Ciphertext []byte    `json:"ciphertext"`
	Nonce      []byte    `json:"nonce,omitempty"`
	IV         []byte    `json:"iv,omitempty"`
	Tag        []byte    `json:"tag,omitempty"`
	AAD        []byte    `json:"aad,omitempty"` // Additional authenticated data
}

// Encryptor provides encryption services.
type Encryptor struct {
	keyProvider KeyProvider
	algorithm   Algorithm
	listeners   []EncryptionEventListener
	mu          sync.RWMutex
}

// EncryptionEvent represents an encryption event.
type EncryptionEvent struct {
	Type      string    `json:"type"`
	KeyID     string    `json:"keyId"`
	Algorithm Algorithm `json:"algorithm"`
	Size      int       `json:"size"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// EncryptionEventListener is called when encryption events occur.
type EncryptionEventListener func(*EncryptionEvent)

// NewEncryptor creates a new encryptor.
func NewEncryptor(keyProvider KeyProvider, algorithm Algorithm) *Encryptor {
	if algorithm == "" {
		algorithm = AlgorithmAES256GCM
	}
	return &Encryptor{
		keyProvider: keyProvider,
		algorithm:   algorithm,
	}
}

// AddListener adds an event listener.
func (e *Encryptor) AddListener(listener EncryptionEventListener) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.listeners = append(e.listeners, listener)
}

// Encrypt encrypts plaintext using the current data encryption key.
func (e *Encryptor) Encrypt(ctx context.Context, plaintext []byte) (*EncryptedData, error) {
	return e.EncryptWithAAD(ctx, plaintext, nil)
}

// EncryptWithAAD encrypts plaintext with additional authenticated data.
func (e *Encryptor) EncryptWithAAD(ctx context.Context, plaintext, aad []byte) (*EncryptedData, error) {
	key, err := e.keyProvider.GetCurrentKey(ctx, KeyTypeData)
	if err != nil {
		e.emit(&EncryptionEvent{
			Type:      "encrypt",
			Algorithm: e.algorithm,
			Success:   false,
			Error:     err.Error(),
			Timestamp: time.Now(),
		})
		return nil, fmt.Errorf("get current key: %w", err)
	}

	if key.IsExpired() {
		return nil, ErrKeyExpired
	}

	var encrypted *EncryptedData
	switch e.algorithm {
	case AlgorithmAES256GCM:
		encrypted, err = encryptAESGCM(key, plaintext, aad)
	case AlgorithmAES256CBC:
		encrypted, err = encryptAESCBC(key, plaintext)
	default:
		err = fmt.Errorf("unsupported algorithm: %s", e.algorithm)
	}

	if err != nil {
		e.emit(&EncryptionEvent{
			Type:      "encrypt",
			KeyID:     key.ID,
			Algorithm: e.algorithm,
			Size:      len(plaintext),
			Success:   false,
			Error:     err.Error(),
			Timestamp: time.Now(),
		})
		return nil, err
	}

	e.emit(&EncryptionEvent{
		Type:      "encrypt",
		KeyID:     key.ID,
		Algorithm: e.algorithm,
		Size:      len(plaintext),
		Success:   true,
		Timestamp: time.Now(),
	})

	return encrypted, nil
}

// Decrypt decrypts encrypted data.
func (e *Encryptor) Decrypt(ctx context.Context, data *EncryptedData) ([]byte, error) {
	key, err := e.keyProvider.GetKey(ctx, data.KeyID)
	if err != nil {
		e.emit(&EncryptionEvent{
			Type:      "decrypt",
			KeyID:     data.KeyID,
			Algorithm: data.Algorithm,
			Success:   false,
			Error:     err.Error(),
			Timestamp: time.Now(),
		})
		return nil, fmt.Errorf("get key: %w", err)
	}

	var plaintext []byte
	switch data.Algorithm {
	case AlgorithmAES256GCM:
		plaintext, err = decryptAESGCM(key, data)
	case AlgorithmAES256CBC:
		plaintext, err = decryptAESCBC(key, data)
	default:
		err = fmt.Errorf("unsupported algorithm: %s", data.Algorithm)
	}

	if err != nil {
		e.emit(&EncryptionEvent{
			Type:      "decrypt",
			KeyID:     data.KeyID,
			Algorithm: data.Algorithm,
			Success:   false,
			Error:     err.Error(),
			Timestamp: time.Now(),
		})
		return nil, err
	}

	e.emit(&EncryptionEvent{
		Type:      "decrypt",
		KeyID:     data.KeyID,
		Algorithm: data.Algorithm,
		Size:      len(plaintext),
		Success:   true,
		Timestamp: time.Now(),
	})

	return plaintext, nil
}

// ReEncrypt re-encrypts data with the current key.
func (e *Encryptor) ReEncrypt(ctx context.Context, data *EncryptedData) (*EncryptedData, error) {
	plaintext, err := e.Decrypt(ctx, data)
	if err != nil {
		return nil, fmt.Errorf("decrypt for re-encryption: %w", err)
	}

	return e.EncryptWithAAD(ctx, plaintext, data.AAD)
}

func (e *Encryptor) emit(event *EncryptionEvent) {
	e.mu.RLock()
	listeners := e.listeners
	e.mu.RUnlock()

	for _, listener := range listeners {
		listener(event)
	}
}

func encryptAESGCM(key *Key, plaintext, aad []byte) (*EncryptedData, error) {
	block, err := aes.NewCipher(key.Material)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, aad)

	return &EncryptedData{
		KeyID:      key.ID,
		Algorithm:  AlgorithmAES256GCM,
		Ciphertext: ciphertext,
		Nonce:      nonce,
		AAD:        aad,
	}, nil
}

func decryptAESGCM(key *Key, data *EncryptedData) ([]byte, error) {
	block, err := aes.NewCipher(key.Material)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}

	if len(data.Nonce) != gcm.NonceSize() {
		return nil, ErrInvalidCiphertext
	}

	plaintext, err := gcm.Open(nil, data.Nonce, data.Ciphertext, data.AAD)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}

func encryptAESCBC(key *Key, plaintext []byte) (*EncryptedData, error) {
	block, err := aes.NewCipher(key.Material)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	// Pad plaintext to block size
	padding := aes.BlockSize - (len(plaintext) % aes.BlockSize)
	padded := make([]byte, len(plaintext)+padding)
	copy(padded, plaintext)
	for i := len(plaintext); i < len(padded); i++ {
		padded[i] = byte(padding)
	}

	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, fmt.Errorf("generate IV: %w", err)
	}

	// #nosec G407 -- IV is generated per-encryption using crypto/rand.
	mode := cipher.NewCBCEncrypter(block, iv)
	ciphertext := make([]byte, len(padded))
	mode.CryptBlocks(ciphertext, padded)

	return &EncryptedData{
		KeyID:      key.ID,
		Algorithm:  AlgorithmAES256CBC,
		Ciphertext: ciphertext,
		IV:         iv,
	}, nil
}

func decryptAESCBC(key *Key, data *EncryptedData) ([]byte, error) {
	block, err := aes.NewCipher(key.Material)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	if len(data.IV) != aes.BlockSize {
		return nil, ErrInvalidCiphertext
	}

	if len(data.Ciphertext)%aes.BlockSize != 0 {
		return nil, ErrInvalidCiphertext
	}

	mode := cipher.NewCBCDecrypter(block, data.IV)
	plaintext := make([]byte, len(data.Ciphertext))
	mode.CryptBlocks(plaintext, data.Ciphertext)

	// Remove padding
	padding := int(plaintext[len(plaintext)-1])
	if padding > aes.BlockSize || padding == 0 {
		return nil, ErrDecryptionFailed
	}
	for i := len(plaintext) - padding; i < len(plaintext); i++ {
		if plaintext[i] != byte(padding) {
			return nil, ErrDecryptionFailed
		}
	}

	return plaintext[:len(plaintext)-padding], nil
}

// InMemoryKeyProvider is an in-memory key provider for testing.
type InMemoryKeyProvider struct {
	keys        map[string]*Key
	currentKeys map[KeyType]string
	mu          sync.RWMutex
}

// NewInMemoryKeyProvider creates a new in-memory key provider.
func NewInMemoryKeyProvider() *InMemoryKeyProvider {
	return &InMemoryKeyProvider{
		keys:        make(map[string]*Key),
		currentKeys: make(map[KeyType]string),
	}
}

// AddKey adds a key to the provider.
func (p *InMemoryKeyProvider) AddKey(key *Key) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.keys[key.ID] = key
}

// SetCurrentKey sets the current key for a type.
func (p *InMemoryKeyProvider) SetCurrentKey(keyType KeyType, keyID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.currentKeys[keyType] = keyID
}

// GetKey retrieves a key by ID.
func (p *InMemoryKeyProvider) GetKey(ctx context.Context, keyID string) (*Key, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	key, ok := p.keys[keyID]
	if !ok {
		return nil, ErrKeyNotFound
	}

	return key, nil
}

// GetCurrentKey retrieves the current key for a type.
func (p *InMemoryKeyProvider) GetCurrentKey(ctx context.Context, keyType KeyType) (*Key, error) {
	p.mu.RLock()
	keyID, ok := p.currentKeys[keyType]
	p.mu.RUnlock()

	if !ok {
		return nil, ErrKeyNotFound
	}

	return p.GetKey(ctx, keyID)
}

// RotateKey creates a new key and marks it as current.
func (p *InMemoryKeyProvider) RotateKey(ctx context.Context, keyType KeyType) (*Key, error) {
	material := make([]byte, 32) // 256 bits
	if _, err := io.ReadFull(rand.Reader, material); err != nil {
		return nil, fmt.Errorf("generate key material: %w", err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Find previous key
	var version int
	var rotatedFrom string
	if currentID, ok := p.currentKeys[keyType]; ok {
		if current, ok := p.keys[currentID]; ok {
			version = current.Version + 1
			rotatedFrom = currentID
		}
	}

	key := &Key{
		ID:          fmt.Sprintf("%s-v%d-%d", keyType, version, time.Now().Unix()),
		Type:        keyType,
		Algorithm:   AlgorithmAES256GCM,
		Material:    material,
		CreatedAt:   time.Now(),
		RotatedFrom: rotatedFrom,
		Version:     version,
	}

	p.keys[key.ID] = key
	p.currentKeys[keyType] = key.ID

	return key, nil
}

// ListKeys lists all keys of a type.
func (p *InMemoryKeyProvider) ListKeys(ctx context.Context, keyType KeyType) ([]*Key, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var keys []*Key
	for _, key := range p.keys {
		if key.Type == keyType {
			keys = append(keys, key)
		}
	}

	return keys, nil
}

// DerivedKeyProvider derives keys from a master secret.
type DerivedKeyProvider struct {
	masterSecret []byte
	salt         []byte
	iterations   int
	keys         map[string]*Key
	currentKeys  map[KeyType]string
	mu           sync.RWMutex
}

// NewDerivedKeyProvider creates a new derived key provider.
func NewDerivedKeyProvider(masterSecret, salt []byte, iterations int) *DerivedKeyProvider {
	if iterations <= 0 {
		iterations = 100000
	}
	return &DerivedKeyProvider{
		masterSecret: masterSecret,
		salt:         salt,
		iterations:   iterations,
		keys:         make(map[string]*Key),
		currentKeys:  make(map[KeyType]string),
	}
}

// DeriveKey derives a new key.
func (p *DerivedKeyProvider) DeriveKey(ctx context.Context, keyType KeyType, info string) (*Key, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Derive key material using PBKDF2
	derivedSalt := append(p.salt, []byte(info)...)
	material := pbkdf2.Key(p.masterSecret, derivedSalt, p.iterations, 32, sha256.New)

	// Find previous key version
	var version int
	var rotatedFrom string
	if currentID, ok := p.currentKeys[keyType]; ok {
		if current, ok := p.keys[currentID]; ok {
			version = current.Version + 1
			rotatedFrom = currentID
		}
	}

	key := &Key{
		ID:          fmt.Sprintf("%s-v%d-%s", keyType, version, info),
		Type:        keyType,
		Algorithm:   AlgorithmAES256GCM,
		Material:    material,
		CreatedAt:   time.Now(),
		RotatedFrom: rotatedFrom,
		Version:     version,
	}

	p.keys[key.ID] = key
	p.currentKeys[keyType] = key.ID

	return key, nil
}

// GetKey retrieves a key by ID.
func (p *DerivedKeyProvider) GetKey(ctx context.Context, keyID string) (*Key, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	key, ok := p.keys[keyID]
	if !ok {
		return nil, ErrKeyNotFound
	}

	return key, nil
}

// GetCurrentKey retrieves the current key for a type.
func (p *DerivedKeyProvider) GetCurrentKey(ctx context.Context, keyType KeyType) (*Key, error) {
	p.mu.RLock()
	keyID, ok := p.currentKeys[keyType]
	p.mu.RUnlock()

	if !ok {
		return nil, ErrKeyNotFound
	}

	return p.GetKey(ctx, keyID)
}

// RotateKey rotates the key.
func (p *DerivedKeyProvider) RotateKey(ctx context.Context, keyType KeyType) (*Key, error) {
	info := fmt.Sprintf("%d", time.Now().UnixNano())
	return p.DeriveKey(ctx, keyType, info)
}

// ListKeys lists all keys of a type.
func (p *DerivedKeyProvider) ListKeys(ctx context.Context, keyType KeyType) ([]*Key, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var keys []*Key
	for _, key := range p.keys {
		if key.Type == keyType {
			keys = append(keys, key)
		}
	}

	return keys, nil
}

// EnvelopeEncryptor uses envelope encryption (wrapping keys).
type EnvelopeEncryptor struct {
	keyProvider KeyProvider
	algorithm   Algorithm
}

// EnvelopeEncryptedData represents envelope encrypted data.
type EnvelopeEncryptedData struct {
	WrappedDEK    []byte         `json:"wrappedDek"`
	WrapKeyID     string         `json:"wrapKeyId"`
	WrapAlgorithm Algorithm      `json:"wrapAlgorithm"`
	Data          *EncryptedData `json:"data"`
}

// NewEnvelopeEncryptor creates a new envelope encryptor.
func NewEnvelopeEncryptor(keyProvider KeyProvider, algorithm Algorithm) *EnvelopeEncryptor {
	if algorithm == "" {
		algorithm = AlgorithmAES256GCM
	}
	return &EnvelopeEncryptor{
		keyProvider: keyProvider,
		algorithm:   algorithm,
	}
}

// Encrypt encrypts using envelope encryption.
func (e *EnvelopeEncryptor) Encrypt(ctx context.Context, plaintext []byte) (*EnvelopeEncryptedData, error) {
	// Generate a random DEK
	dek := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return nil, fmt.Errorf("generate DEK: %w", err)
	}

	// Encrypt data with DEK
	dekKey := &Key{
		ID:        "ephemeral-dek",
		Type:      KeyTypeData,
		Algorithm: e.algorithm,
		Material:  dek,
	}

	encrypted, err := encryptAESGCM(dekKey, plaintext, nil)
	if err != nil {
		return nil, fmt.Errorf("encrypt data: %w", err)
	}

	// Wrap the DEK with the KEK
	kek, err := e.keyProvider.GetCurrentKey(ctx, KeyTypeWrapping)
	if err != nil {
		return nil, fmt.Errorf("get wrapping key: %w", err)
	}

	wrappedDEK, err := wrapKey(kek, dek)
	if err != nil {
		return nil, fmt.Errorf("wrap DEK: %w", err)
	}

	return &EnvelopeEncryptedData{
		WrappedDEK:    wrappedDEK,
		WrapKeyID:     kek.ID,
		WrapAlgorithm: kek.Algorithm,
		Data:          encrypted,
	}, nil
}

// Decrypt decrypts envelope encrypted data.
func (e *EnvelopeEncryptor) Decrypt(ctx context.Context, data *EnvelopeEncryptedData) ([]byte, error) {
	// Get the KEK
	kek, err := e.keyProvider.GetKey(ctx, data.WrapKeyID)
	if err != nil {
		return nil, fmt.Errorf("get wrapping key: %w", err)
	}

	// Unwrap the DEK
	dek, err := unwrapKey(kek, data.WrappedDEK)
	if err != nil {
		return nil, fmt.Errorf("unwrap DEK: %w", err)
	}

	// Decrypt data with DEK
	dekKey := &Key{
		ID:        "ephemeral-dek",
		Type:      KeyTypeData,
		Algorithm: data.Data.Algorithm,
		Material:  dek,
	}

	return decryptAESGCM(dekKey, data.Data)
}

func wrapKey(kek *Key, dek []byte) ([]byte, error) {
	block, err := aes.NewCipher(kek.Material)
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

	wrapped := gcm.Seal(nonce, nonce, dek, nil)
	return wrapped, nil
}

func unwrapKey(kek *Key, wrapped []byte) ([]byte, error) {
	block, err := aes.NewCipher(kek.Material)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	if len(wrapped) < gcm.NonceSize() {
		return nil, ErrInvalidCiphertext
	}

	nonce := wrapped[:gcm.NonceSize()]
	ciphertext := wrapped[gcm.NonceSize():]

	return gcm.Open(nil, nonce, ciphertext, nil)
}

// EncryptJSON encrypts a JSON-serializable value.
func EncryptJSON(ctx context.Context, encryptor *Encryptor, value interface{}) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}

	encrypted, err := encryptor.Encrypt(ctx, data)
	if err != nil {
		return "", err
	}

	encryptedJSON, err := json.Marshal(encrypted)
	if err != nil {
		return "", fmt.Errorf("marshal encrypted: %w", err)
	}

	return base64.StdEncoding.EncodeToString(encryptedJSON), nil
}

// DecryptJSON decrypts a JSON-serializable value.
func DecryptJSON(ctx context.Context, encryptor *Encryptor, encoded string, value interface{}) error {
	encryptedJSON, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("decode base64: %w", err)
	}

	var encrypted EncryptedData
	if err := json.Unmarshal(encryptedJSON, &encrypted); err != nil {
		return fmt.Errorf("unmarshal encrypted: %w", err)
	}

	plaintext, err := encryptor.Decrypt(ctx, &encrypted)
	if err != nil {
		return err
	}

	return json.Unmarshal(plaintext, value)
}
