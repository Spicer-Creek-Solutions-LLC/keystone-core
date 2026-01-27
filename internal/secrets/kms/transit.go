// Package kms provides advanced transit encryption features including
// convergent encryption, batch operations, HMAC, and key export.
package kms

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/hkdf"
)

// TransitConfig contains configuration for the transit engine.
type TransitConfig struct {
	// DefaultAlgorithm is the default encryption algorithm.
	DefaultAlgorithm EncryptionAlgorithm `json:"default_algorithm,omitempty"`

	// DefaultKeySize is the default key size in bits.
	DefaultKeySize int `json:"default_key_size,omitempty"`

	// ConvergentNonceSize is the nonce size for convergent encryption.
	ConvergentNonceSize int `json:"convergent_nonce_size,omitempty"`

	// BatchSize is the default batch size for batch operations.
	BatchSize int `json:"batch_size,omitempty"`

	// MaxBatchSize is the maximum batch size allowed.
	MaxBatchSize int `json:"max_batch_size,omitempty"`

	// BatchParallelism is the number of parallel workers for batch operations.
	BatchParallelism int `json:"batch_parallelism,omitempty"`

	// EnableKeyExport allows key export operations.
	EnableKeyExport bool `json:"enable_key_export,omitempty"`

	// KeyExportRequiresWrapping requires keys to be wrapped before export.
	KeyExportRequiresWrapping bool `json:"key_export_requires_wrapping,omitempty"`
}

// DefaultTransitConfig returns default transit configuration.
func DefaultTransitConfig() *TransitConfig {
	return &TransitConfig{
		DefaultAlgorithm:          AlgorithmAESGCM,
		DefaultKeySize:            256,
		ConvergentNonceSize:       12,
		BatchSize:                 100,
		MaxBatchSize:              1000,
		BatchParallelism:          4,
		EnableKeyExport:           false,
		KeyExportRequiresWrapping: true,
	}
}

// EncryptionAlgorithm represents supported encryption algorithms.
type EncryptionAlgorithm string

const (
	AlgorithmAESGCM      EncryptionAlgorithm = "aes-gcm"
	AlgorithmAESCBC      EncryptionAlgorithm = "aes-cbc"
	AlgorithmChaCha20    EncryptionAlgorithm = "chacha20-poly1305"
	AlgorithmRSAOAEP     EncryptionAlgorithm = "rsa-oaep"
)

// HMACAlgorithm represents supported HMAC algorithms.
type HMACAlgorithm string

const (
	HMACAlgorithmSHA256 HMACAlgorithm = "hmac-sha256"
	HMACAlgorithmSHA384 HMACAlgorithm = "hmac-sha384"
	HMACAlgorithmSHA512 HMACAlgorithm = "hmac-sha512"
)

// TransitEngine provides advanced transit encryption features.
type TransitEngine struct {
	config   *TransitConfig
	provider Provider

	mu       sync.RWMutex
	keys     map[string]*TransitKey
	hmacKeys map[string]*HMACKey
}

// TransitKey represents a transit encryption key.
type TransitKey struct {
	Name             string              `json:"name"`
	Algorithm        EncryptionAlgorithm `json:"algorithm"`
	KeyMaterial      []byte              `json:"-"`
	KeySize          int                 `json:"key_size"`
	ConvergentKey    []byte              `json:"-"`
	SupportsConvergent bool              `json:"supports_convergent"`
	Exportable       bool                `json:"exportable"`
	CreatedAt        time.Time           `json:"created_at"`
	Version          int                 `json:"version"`
	MinDecryptVersion int                `json:"min_decrypt_version"`
}

// HMACKey represents an HMAC key.
type HMACKey struct {
	Name        string        `json:"name"`
	Algorithm   HMACAlgorithm `json:"algorithm"`
	KeyMaterial []byte        `json:"-"`
	KeySize     int           `json:"key_size"`
	CreatedAt   time.Time     `json:"created_at"`
	Version     int           `json:"version"`
}

// NewTransitEngine creates a new transit engine.
func NewTransitEngine(config *TransitConfig, provider Provider) *TransitEngine {
	if config == nil {
		config = DefaultTransitConfig()
	}
	return &TransitEngine{
		config:   config,
		provider: provider,
		keys:     make(map[string]*TransitKey),
		hmacKeys: make(map[string]*HMACKey),
	}
}

// CreateKey creates a new transit key.
func (t *TransitEngine) CreateKey(ctx context.Context, name string, algorithm EncryptionAlgorithm, convergent, exportable bool) (*TransitKey, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, exists := t.keys[name]; exists {
		return nil, fmt.Errorf("key %s already exists", name)
	}

	keySize := t.config.DefaultKeySize / 8
	if keySize == 0 {
		keySize = 32 // Default to 256 bits
	}
	keyMaterial := make([]byte, keySize)
	if _, err := rand.Read(keyMaterial); err != nil {
		return nil, fmt.Errorf("failed to generate key material: %w", err)
	}

	key := &TransitKey{
		Name:               name,
		Algorithm:          algorithm,
		KeyMaterial:        keyMaterial,
		KeySize:            t.config.DefaultKeySize,
		SupportsConvergent: convergent,
		Exportable:         exportable,
		CreatedAt:          time.Now(),
		Version:            1,
		MinDecryptVersion:  1,
	}

	if convergent {
		key.ConvergentKey = make([]byte, keySize)
		if _, err := rand.Read(key.ConvergentKey); err != nil {
			return nil, fmt.Errorf("failed to generate convergent key: %w", err)
		}
	}

	t.keys[name] = key
	return key, nil
}

// GetKey retrieves a transit key.
func (t *TransitEngine) GetKey(name string) (*TransitKey, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	key, exists := t.keys[name]
	if !exists {
		return nil, fmt.Errorf("key %s not found", name)
	}
	return key, nil
}

// ConvergentEncryptRequest represents a convergent encryption request.
type ConvergentEncryptRequest struct {
	KeyName   string `json:"key_name"`
	Plaintext []byte `json:"plaintext"`
	Context   []byte `json:"context,omitempty"`
}

// ConvergentEncryptResponse represents a convergent encryption response.
type ConvergentEncryptResponse struct {
	Ciphertext []byte `json:"ciphertext"`
	KeyVersion int    `json:"key_version"`
}

// ConvergentEncrypt performs convergent encryption.
// The same plaintext with the same context always produces the same ciphertext.
func (t *TransitEngine) ConvergentEncrypt(ctx context.Context, req *ConvergentEncryptRequest) (*ConvergentEncryptResponse, error) {
	key, err := t.GetKey(req.KeyName)
	if err != nil {
		return nil, err
	}

	if !key.SupportsConvergent {
		return nil, errors.New("key does not support convergent encryption")
	}

	derivedKey := t.deriveConvergentKey(key, req.Plaintext, req.Context)
	defer zeroBytes(derivedKey)

	nonce := t.deriveConvergentNonce(key, req.Plaintext, req.Context)

	block, err := aes.NewCipher(derivedKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, req.Plaintext, req.Context)

	versionPrefix := make([]byte, 4)
	binary.BigEndian.PutUint32(versionPrefix, uint32(key.Version))

	return &ConvergentEncryptResponse{
		Ciphertext: append(versionPrefix, ciphertext...),
		KeyVersion: key.Version,
	}, nil
}

// ConvergentDecrypt performs convergent decryption.
func (t *TransitEngine) ConvergentDecrypt(ctx context.Context, keyName string, ciphertext, context []byte) ([]byte, error) {
	key, err := t.GetKey(keyName)
	if err != nil {
		return nil, err
	}

	if !key.SupportsConvergent {
		return nil, errors.New("key does not support convergent encryption")
	}

	if len(ciphertext) < 4 {
		return nil, errors.New("ciphertext too short")
	}

	version := binary.BigEndian.Uint32(ciphertext[:4])
	if int(version) < key.MinDecryptVersion {
		return nil, fmt.Errorf("ciphertext version %d is below minimum %d", version, key.MinDecryptVersion)
	}

	actualCiphertext := ciphertext[4:]

	derivedKey := t.deriveConvergentKeyForDecrypt(key, actualCiphertext, context)
	defer zeroBytes(derivedKey)

	block, err := aes.NewCipher(derivedKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(actualCiphertext) < nonceSize {
		return nil, errors.New("ciphertext too short for nonce")
	}

	plaintext, err := gcm.Open(nil, actualCiphertext[:nonceSize], actualCiphertext[nonceSize:], context)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}

// deriveConvergentKey derives a deterministic key for convergent encryption.
func (t *TransitEngine) deriveConvergentKey(key *TransitKey, plaintext, context []byte) []byte {
	h := sha256.New()
	h.Write(key.ConvergentKey)
	h.Write(plaintext)
	if len(context) > 0 {
		h.Write(context)
	}
	derived := h.Sum(nil)

	reader := hkdf.New(sha256.New, key.KeyMaterial, derived, []byte("convergent-encryption"))
	derivedKey := make([]byte, len(key.KeyMaterial))
	io.ReadFull(reader, derivedKey)
	return derivedKey
}

// deriveConvergentKeyForDecrypt derives the key for decryption (needs plaintext from ciphertext).
func (t *TransitEngine) deriveConvergentKeyForDecrypt(key *TransitKey, ciphertext, context []byte) []byte {
	h := sha256.New()
	h.Write(key.ConvergentKey)
	h.Write(ciphertext)
	if len(context) > 0 {
		h.Write(context)
	}
	derived := h.Sum(nil)

	reader := hkdf.New(sha256.New, key.KeyMaterial, derived, []byte("convergent-decryption"))
	derivedKey := make([]byte, len(key.KeyMaterial))
	io.ReadFull(reader, derivedKey)
	return derivedKey
}

// deriveConvergentNonce derives a deterministic nonce for convergent encryption.
func (t *TransitEngine) deriveConvergentNonce(key *TransitKey, plaintext, context []byte) []byte {
	h := sha256.New()
	h.Write(key.ConvergentKey)
	h.Write([]byte("nonce"))
	h.Write(plaintext)
	if len(context) > 0 {
		h.Write(context)
	}
	return h.Sum(nil)[:t.config.ConvergentNonceSize]
}

// BatchEncryptRequest represents a batch encryption request.
type BatchEncryptRequest struct {
	KeyName string             `json:"key_name"`
	Items   []BatchEncryptItem `json:"items"`
}

// BatchEncryptItem represents a single item in a batch encryption request.
type BatchEncryptItem struct {
	Plaintext []byte            `json:"plaintext"`
	Context   map[string]string `json:"context,omitempty"`
	Reference string            `json:"reference,omitempty"`
}

// BatchEncryptResponse represents a batch encryption response.
type BatchEncryptResponse struct {
	Results    []BatchEncryptResult `json:"results"`
	Succeeded  int                  `json:"succeeded"`
	Failed     int                  `json:"failed"`
	TotalTime  time.Duration        `json:"total_time"`
}

// BatchEncryptResult represents the result of a single batch encryption.
type BatchEncryptResult struct {
	Ciphertext []byte `json:"ciphertext,omitempty"`
	Error      string `json:"error,omitempty"`
	Reference  string `json:"reference,omitempty"`
}

// BatchEncrypt encrypts multiple items in parallel.
func (t *TransitEngine) BatchEncrypt(ctx context.Context, req *BatchEncryptRequest) (*BatchEncryptResponse, error) {
	if len(req.Items) > t.config.MaxBatchSize {
		return nil, fmt.Errorf("batch size %d exceeds maximum %d", len(req.Items), t.config.MaxBatchSize)
	}

	key, err := t.GetKey(req.KeyName)
	if err != nil {
		return nil, err
	}

	startTime := time.Now()
	results := make([]BatchEncryptResult, len(req.Items))
	var succeeded, failed int32

	parallelism := t.config.BatchParallelism
	if parallelism > len(req.Items) {
		parallelism = len(req.Items)
	}

	itemCh := make(chan int, len(req.Items))
	for i := range req.Items {
		itemCh <- i
	}
	close(itemCh)

	var wg sync.WaitGroup
	for w := 0; w < parallelism; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range itemCh {
				select {
				case <-ctx.Done():
					results[i] = BatchEncryptResult{
						Error:     ctx.Err().Error(),
						Reference: req.Items[i].Reference,
					}
					atomic.AddInt32(&failed, 1)
					continue
				default:
				}

				ciphertext, err := t.encryptSingle(key, req.Items[i].Plaintext, req.Items[i].Context)
				if err != nil {
					results[i] = BatchEncryptResult{
						Error:     err.Error(),
						Reference: req.Items[i].Reference,
					}
					atomic.AddInt32(&failed, 1)
				} else {
					results[i] = BatchEncryptResult{
						Ciphertext: ciphertext,
						Reference:  req.Items[i].Reference,
					}
					atomic.AddInt32(&succeeded, 1)
				}
			}
		}()
	}

	wg.Wait()

	return &BatchEncryptResponse{
		Results:   results,
		Succeeded: int(succeeded),
		Failed:    int(failed),
		TotalTime: time.Since(startTime),
	}, nil
}

// BatchDecryptRequest represents a batch decryption request.
type BatchDecryptRequest struct {
	KeyName string             `json:"key_name"`
	Items   []BatchDecryptItem `json:"items"`
}

// BatchDecryptItem represents a single item in a batch decryption request.
type BatchDecryptItem struct {
	Ciphertext []byte            `json:"ciphertext"`
	Context    map[string]string `json:"context,omitempty"`
	Reference  string            `json:"reference,omitempty"`
}

// BatchDecryptResponse represents a batch decryption response.
type BatchDecryptResponse struct {
	Results   []BatchDecryptResult `json:"results"`
	Succeeded int                  `json:"succeeded"`
	Failed    int                  `json:"failed"`
	TotalTime time.Duration        `json:"total_time"`
}

// BatchDecryptResult represents the result of a single batch decryption.
type BatchDecryptResult struct {
	Plaintext []byte `json:"plaintext,omitempty"`
	Error     string `json:"error,omitempty"`
	Reference string `json:"reference,omitempty"`
}

// BatchDecrypt decrypts multiple items in parallel.
func (t *TransitEngine) BatchDecrypt(ctx context.Context, req *BatchDecryptRequest) (*BatchDecryptResponse, error) {
	if len(req.Items) > t.config.MaxBatchSize {
		return nil, fmt.Errorf("batch size %d exceeds maximum %d", len(req.Items), t.config.MaxBatchSize)
	}

	key, err := t.GetKey(req.KeyName)
	if err != nil {
		return nil, err
	}

	startTime := time.Now()
	results := make([]BatchDecryptResult, len(req.Items))
	var succeeded, failed int32

	parallelism := t.config.BatchParallelism
	if parallelism > len(req.Items) {
		parallelism = len(req.Items)
	}

	itemCh := make(chan int, len(req.Items))
	for i := range req.Items {
		itemCh <- i
	}
	close(itemCh)

	var wg sync.WaitGroup
	for w := 0; w < parallelism; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range itemCh {
				select {
				case <-ctx.Done():
					results[i] = BatchDecryptResult{
						Error:     ctx.Err().Error(),
						Reference: req.Items[i].Reference,
					}
					atomic.AddInt32(&failed, 1)
					continue
				default:
				}

				plaintext, err := t.decryptSingle(key, req.Items[i].Ciphertext, req.Items[i].Context)
				if err != nil {
					results[i] = BatchDecryptResult{
						Error:     err.Error(),
						Reference: req.Items[i].Reference,
					}
					atomic.AddInt32(&failed, 1)
				} else {
					results[i] = BatchDecryptResult{
						Plaintext: plaintext,
						Reference: req.Items[i].Reference,
					}
					atomic.AddInt32(&succeeded, 1)
				}
			}
		}()
	}

	wg.Wait()

	return &BatchDecryptResponse{
		Results:   results,
		Succeeded: int(succeeded),
		Failed:    int(failed),
		TotalTime: time.Since(startTime),
	}, nil
}

// encryptSingle encrypts a single plaintext.
func (t *TransitEngine) encryptSingle(key *TransitKey, plaintext []byte, context map[string]string) ([]byte, error) {
	block, err := aes.NewCipher(key.KeyMaterial)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}

	var aad []byte
	if len(context) > 0 {
		aad, _ = json.Marshal(context)
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, aad)

	versionPrefix := make([]byte, 4)
	binary.BigEndian.PutUint32(versionPrefix, uint32(key.Version))

	return append(versionPrefix, ciphertext...), nil
}

// decryptSingle decrypts a single ciphertext.
func (t *TransitEngine) decryptSingle(key *TransitKey, ciphertext []byte, context map[string]string) ([]byte, error) {
	if len(ciphertext) < 4 {
		return nil, errors.New("ciphertext too short")
	}

	version := binary.BigEndian.Uint32(ciphertext[:4])
	if int(version) < key.MinDecryptVersion {
		return nil, fmt.Errorf("ciphertext version %d is below minimum %d", version, key.MinDecryptVersion)
	}

	actualCiphertext := ciphertext[4:]

	block, err := aes.NewCipher(key.KeyMaterial)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(actualCiphertext) < nonceSize {
		return nil, errors.New("ciphertext too short for nonce")
	}

	var aad []byte
	if len(context) > 0 {
		aad, _ = json.Marshal(context)
	}

	return gcm.Open(nil, actualCiphertext[:nonceSize], actualCiphertext[nonceSize:], aad)
}

// TransitDataKeyRequest represents a request to generate a data encryption key.
type TransitDataKeyRequest struct {
	KeyName       string `json:"key_name"`
	KeySize       int    `json:"key_size,omitempty"`
	Context       []byte `json:"context,omitempty"`
	Nonce         []byte `json:"nonce,omitempty"`
}

// GenerateDataKeyResponse represents a generated data encryption key.
type GenerateDataKeyResponse struct {
	Plaintext      []byte    `json:"plaintext"`
	Ciphertext     []byte    `json:"ciphertext"`
	KeyVersion     int       `json:"key_version"`
	GeneratedAt    time.Time `json:"generated_at"`
}

// GenerateDataKeyForTransit generates a data encryption key wrapped by a transit key.
func (t *TransitEngine) GenerateDataKeyForTransit(ctx context.Context, req *TransitDataKeyRequest) (*GenerateDataKeyResponse, error) {
	key, err := t.GetKey(req.KeyName)
	if err != nil {
		return nil, err
	}

	keySize := req.KeySize
	if keySize == 0 {
		keySize = 32
	}

	plaintext := make([]byte, keySize)
	if _, err := rand.Read(plaintext); err != nil {
		return nil, fmt.Errorf("failed to generate data key: %w", err)
	}

	ciphertext, err := t.encryptSingle(key, plaintext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to wrap data key: %w", err)
	}

	return &GenerateDataKeyResponse{
		Plaintext:   plaintext,
		Ciphertext:  ciphertext,
		KeyVersion:  key.Version,
		GeneratedAt: time.Now(),
	}, nil
}

// CreateHMACKey creates a new HMAC key.
func (t *TransitEngine) CreateHMACKey(ctx context.Context, name string, algorithm HMACAlgorithm) (*HMACKey, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, exists := t.hmacKeys[name]; exists {
		return nil, fmt.Errorf("HMAC key %s already exists", name)
	}

	var keySize int
	switch algorithm {
	case HMACAlgorithmSHA256:
		keySize = 32
	case HMACAlgorithmSHA384:
		keySize = 48
	case HMACAlgorithmSHA512:
		keySize = 64
	default:
		return nil, fmt.Errorf("unsupported HMAC algorithm: %s", algorithm)
	}

	keyMaterial := make([]byte, keySize)
	if _, err := rand.Read(keyMaterial); err != nil {
		return nil, fmt.Errorf("failed to generate HMAC key: %w", err)
	}

	key := &HMACKey{
		Name:        name,
		Algorithm:   algorithm,
		KeyMaterial: keyMaterial,
		KeySize:     keySize * 8,
		CreatedAt:   time.Now(),
		Version:     1,
	}

	t.hmacKeys[name] = key
	return key, nil
}

// GetHMACKey retrieves an HMAC key.
func (t *TransitEngine) GetHMACKey(name string) (*HMACKey, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	key, exists := t.hmacKeys[name]
	if !exists {
		return nil, fmt.Errorf("HMAC key %s not found", name)
	}
	return key, nil
}

// HMACSignRequest represents an HMAC signing request.
type HMACSignRequest struct {
	KeyName string `json:"key_name"`
	Input   []byte `json:"input"`
}

// HMACSignResponse represents an HMAC signing response.
type HMACSignResponse struct {
	HMAC       []byte `json:"hmac"`
	KeyVersion int    `json:"key_version"`
}

// HMACSign generates an HMAC for the input data.
func (t *TransitEngine) HMACSign(ctx context.Context, req *HMACSignRequest) (*HMACSignResponse, error) {
	key, err := t.GetHMACKey(req.KeyName)
	if err != nil {
		return nil, err
	}

	var h hash.Hash
	switch key.Algorithm {
	case HMACAlgorithmSHA256:
		h = hmac.New(sha256.New, key.KeyMaterial)
	case HMACAlgorithmSHA384:
		h = hmac.New(sha512.New384, key.KeyMaterial)
	case HMACAlgorithmSHA512:
		h = hmac.New(sha512.New, key.KeyMaterial)
	default:
		return nil, fmt.Errorf("unsupported HMAC algorithm: %s", key.Algorithm)
	}

	h.Write(req.Input)

	return &HMACSignResponse{
		HMAC:       h.Sum(nil),
		KeyVersion: key.Version,
	}, nil
}

// HMACVerifyRequest represents an HMAC verification request.
type HMACVerifyRequest struct {
	KeyName string `json:"key_name"`
	Input   []byte `json:"input"`
	HMAC    []byte `json:"hmac"`
}

// HMACVerifyResponse represents an HMAC verification response.
type HMACVerifyResponse struct {
	Valid bool `json:"valid"`
}

// HMACVerify verifies an HMAC.
func (t *TransitEngine) HMACVerify(ctx context.Context, req *HMACVerifyRequest) (*HMACVerifyResponse, error) {
	signResp, err := t.HMACSign(ctx, &HMACSignRequest{
		KeyName: req.KeyName,
		Input:   req.Input,
	})
	if err != nil {
		return nil, err
	}

	return &HMACVerifyResponse{
		Valid: hmac.Equal(signResp.HMAC, req.HMAC),
	}, nil
}

// KeyExportRequest represents a key export request.
type KeyExportRequest struct {
	KeyName       string `json:"key_name"`
	KeyType       string `json:"key_type"`
	WrapperKeyID  string `json:"wrapper_key_id,omitempty"`
}

// KeyExportResponse represents an exported key.
type KeyExportResponse struct {
	KeyName       string    `json:"key_name"`
	KeyType       string    `json:"key_type"`
	KeyMaterial   []byte    `json:"key_material,omitempty"`
	WrappedKey    []byte    `json:"wrapped_key,omitempty"`
	WrapperKeyID  string    `json:"wrapper_key_id,omitempty"`
	KeyVersion    int       `json:"key_version"`
	Algorithm     string    `json:"algorithm"`
	ExportedAt    time.Time `json:"exported_at"`
}

// ExportKey exports a key (if allowed).
func (t *TransitEngine) ExportKey(ctx context.Context, req *KeyExportRequest) (*KeyExportResponse, error) {
	if !t.config.EnableKeyExport {
		return nil, errors.New("key export is disabled")
	}

	var keyMaterial []byte
	var algorithm string
	var version int

	switch req.KeyType {
	case "encryption", "transit":
		key, err := t.GetKey(req.KeyName)
		if err != nil {
			return nil, err
		}
		if !key.Exportable {
			return nil, fmt.Errorf("key %s is not exportable", req.KeyName)
		}
		keyMaterial = key.KeyMaterial
		algorithm = string(key.Algorithm)
		version = key.Version

	case "hmac":
		key, err := t.GetHMACKey(req.KeyName)
		if err != nil {
			return nil, err
		}
		keyMaterial = key.KeyMaterial
		algorithm = string(key.Algorithm)
		version = key.Version

	default:
		return nil, fmt.Errorf("unknown key type: %s", req.KeyType)
	}

	resp := &KeyExportResponse{
		KeyName:    req.KeyName,
		KeyType:    req.KeyType,
		KeyVersion: version,
		Algorithm:  algorithm,
		ExportedAt: time.Now(),
	}

	if t.config.KeyExportRequiresWrapping {
		if req.WrapperKeyID == "" {
			return nil, errors.New("wrapper key ID required for key export")
		}

		if t.provider == nil {
			return nil, errors.New("no KMS provider configured for key wrapping")
		}

		wrapResp, err := t.provider.WrapKey(ctx, &WrapKeyRequest{
			WrapperKeyID: req.WrapperKeyID,
			KeyToWrap:    keyMaterial,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to wrap key: %w", err)
		}

		resp.WrappedKey = wrapResp.WrappedKey
		resp.WrapperKeyID = req.WrapperKeyID
	} else {
		resp.KeyMaterial = make([]byte, len(keyMaterial))
		copy(resp.KeyMaterial, keyMaterial)
	}

	return resp, nil
}

// ImportKeyRequest represents a key import request.
type ImportKeyRequest struct {
	KeyName       string              `json:"key_name"`
	KeyType       string              `json:"key_type"`
	KeyMaterial   []byte              `json:"key_material,omitempty"`
	WrappedKey    []byte              `json:"wrapped_key,omitempty"`
	WrapperKeyID  string              `json:"wrapper_key_id,omitempty"`
	Algorithm     EncryptionAlgorithm `json:"algorithm,omitempty"`
	Exportable    bool                `json:"exportable,omitempty"`
}

// ImportKey imports a key.
func (t *TransitEngine) ImportKey(ctx context.Context, req *ImportKeyRequest) error {
	var keyMaterial []byte

	if len(req.WrappedKey) > 0 {
		if req.WrapperKeyID == "" {
			return errors.New("wrapper key ID required for wrapped key import")
		}
		if t.provider == nil {
			return errors.New("no KMS provider configured for key unwrapping")
		}

		unwrapResp, err := t.provider.UnwrapKey(ctx, &UnwrapKeyRequest{
			WrapperKeyID: req.WrapperKeyID,
			WrappedKey:   req.WrappedKey,
		})
		if err != nil {
			return fmt.Errorf("failed to unwrap key: %w", err)
		}
		keyMaterial = unwrapResp.PlaintextKey
	} else if len(req.KeyMaterial) > 0 {
		keyMaterial = make([]byte, len(req.KeyMaterial))
		copy(keyMaterial, req.KeyMaterial)
	} else {
		return errors.New("either key_material or wrapped_key must be provided")
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	switch req.KeyType {
	case "encryption", "transit":
		if _, exists := t.keys[req.KeyName]; exists {
			return fmt.Errorf("key %s already exists", req.KeyName)
		}

		algorithm := req.Algorithm
		if algorithm == "" {
			algorithm = AlgorithmAESGCM
		}

		t.keys[req.KeyName] = &TransitKey{
			Name:        req.KeyName,
			Algorithm:   algorithm,
			KeyMaterial: keyMaterial,
			KeySize:     len(keyMaterial) * 8,
			Exportable:  req.Exportable,
			CreatedAt:   time.Now(),
			Version:     1,
		}

	case "hmac":
		if _, exists := t.hmacKeys[req.KeyName]; exists {
			return fmt.Errorf("HMAC key %s already exists", req.KeyName)
		}

		var algo HMACAlgorithm
		switch len(keyMaterial) {
		case 32:
			algo = HMACAlgorithmSHA256
		case 48:
			algo = HMACAlgorithmSHA384
		case 64:
			algo = HMACAlgorithmSHA512
		default:
			algo = HMACAlgorithmSHA256
		}

		t.hmacKeys[req.KeyName] = &HMACKey{
			Name:        req.KeyName,
			Algorithm:   algo,
			KeyMaterial: keyMaterial,
			KeySize:     len(keyMaterial) * 8,
			CreatedAt:   time.Now(),
			Version:     1,
		}

	default:
		return fmt.Errorf("unknown key type: %s", req.KeyType)
	}

	return nil
}

// RotateKey rotates a transit key.
func (t *TransitEngine) RotateKey(ctx context.Context, keyName string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	key, exists := t.keys[keyName]
	if !exists {
		return fmt.Errorf("key %s not found", keyName)
	}

	newKeyMaterial := make([]byte, len(key.KeyMaterial))
	if _, err := rand.Read(newKeyMaterial); err != nil {
		return fmt.Errorf("failed to generate new key material: %w", err)
	}

	key.KeyMaterial = newKeyMaterial
	key.Version++

	if key.SupportsConvergent && key.ConvergentKey != nil {
		newConvergentKey := make([]byte, len(key.ConvergentKey))
		if _, err := rand.Read(newConvergentKey); err != nil {
			return fmt.Errorf("failed to generate new convergent key: %w", err)
		}
		key.ConvergentKey = newConvergentKey
	}

	return nil
}

// zeroBytes zeroes a byte slice.
func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
