// Package vault provides a HashiCorp Vault backend for the secrets broker.
package vault

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/secrets"
)

// TransitKeyType represents the type of transit key.
type TransitKeyType string

const (
	TransitKeyTypeAES128GCM96    TransitKeyType = "aes128-gcm96"
	TransitKeyTypeAES256GCM96    TransitKeyType = "aes256-gcm96"
	TransitKeyTypeCHACHA20       TransitKeyType = "chacha20-poly1305"
	TransitKeyTypeED25519        TransitKeyType = "ed25519"
	TransitKeyTypeECDSAP256      TransitKeyType = "ecdsa-p256"
	TransitKeyTypeECDSAP384      TransitKeyType = "ecdsa-p384"
	TransitKeyTypeECDSAP521      TransitKeyType = "ecdsa-p521"
	TransitKeyTypeRSA2048        TransitKeyType = "rsa-2048"
	TransitKeyTypeRSA3072        TransitKeyType = "rsa-3072"
	TransitKeyTypeRSA4096        TransitKeyType = "rsa-4096"
	TransitKeyTypeHMAC           TransitKeyType = "hmac"
	TransitKeyTypeManagedKey     TransitKeyType = "managed_key"
)

// TransitHashAlgorithm represents hash algorithms for transit operations.
type TransitHashAlgorithm string

const (
	TransitHashSHA1   TransitHashAlgorithm = "sha1"
	TransitHashSHA224 TransitHashAlgorithm = "sha2-224"
	TransitHashSHA256 TransitHashAlgorithm = "sha2-256"
	TransitHashSHA384 TransitHashAlgorithm = "sha2-384"
	TransitHashSHA512 TransitHashAlgorithm = "sha2-512"
	TransitHashSHA3224 TransitHashAlgorithm = "sha3-224"
	TransitHashSHA3256 TransitHashAlgorithm = "sha3-256"
	TransitHashSHA3384 TransitHashAlgorithm = "sha3-384"
	TransitHashSHA3512 TransitHashAlgorithm = "sha3-512"
)

// TransitSignatureAlgorithm represents signature algorithms.
type TransitSignatureAlgorithm string

const (
	TransitSigPSS    TransitSignatureAlgorithm = "pss"
	TransitSigPKCS1  TransitSignatureAlgorithm = "pkcs1v15"
)

// TransitConfig configures a transit secret engine mount.
type TransitConfig struct {
	// MountPath is the mount path for the transit engine (default: "transit").
	MountPath string `json:"mount_path,omitempty"`
}

// TransitEngine provides methods for working with Vault's transit secret engine.
type TransitEngine struct {
	client *Client
	config *TransitConfig
}

// NewTransitEngine creates a new transit engine client.
func NewTransitEngine(client *Client, config *TransitConfig) *TransitEngine {
	if config == nil {
		config = &TransitConfig{}
	}
	if config.MountPath == "" {
		config.MountPath = "transit"
	}
	return &TransitEngine{
		client: client,
		config: config,
	}
}

// TransitKey represents a transit encryption key.
type TransitKey struct {
	// Name is the key name.
	Name string `json:"name"`

	// Type is the key type.
	Type TransitKeyType `json:"type"`

	// DeletionAllowed indicates if the key can be deleted.
	DeletionAllowed bool `json:"deletion_allowed"`

	// Derived indicates if the key is derived from another key.
	Derived bool `json:"derived"`

	// Exportable indicates if the key can be exported.
	Exportable bool `json:"exportable"`

	// AllowPlaintextBackup allows plaintext backup of the key.
	AllowPlaintextBackup bool `json:"allow_plaintext_backup"`

	// LatestVersion is the latest key version.
	LatestVersion int `json:"latest_version"`

	// MinDecryptionVersion is the minimum version for decryption.
	MinDecryptionVersion int `json:"min_decryption_version"`

	// MinEncryptionVersion is the minimum version for encryption.
	MinEncryptionVersion int `json:"min_encryption_version"`

	// SupportsEncryption indicates if the key supports encryption.
	SupportsEncryption bool `json:"supports_encryption"`

	// SupportsDecryption indicates if the key supports decryption.
	SupportsDecryption bool `json:"supports_decryption"`

	// SupportsSigning indicates if the key supports signing.
	SupportsSigning bool `json:"supports_signing"`

	// SupportsDerivation indicates if the key supports key derivation.
	SupportsDerivation bool `json:"supports_derivation"`

	// AutoRotatePeriod is the auto-rotation period.
	AutoRotatePeriod time.Duration `json:"auto_rotate_period,omitempty"`
}

// CreateKey creates a new transit key.
func (t *TransitEngine) CreateKey(ctx context.Context, name string, keyType TransitKeyType) error {
	return t.CreateKeyWithOptions(ctx, name, &CreateKeyOptions{Type: keyType})
}

// CreateKeyOptions specifies options for key creation.
type CreateKeyOptions struct {
	// Type is the key type.
	Type TransitKeyType `json:"type,omitempty"`

	// Convergent enables convergent encryption.
	Convergent bool `json:"convergent_encryption,omitempty"`

	// Derived enables key derivation.
	Derived bool `json:"derived,omitempty"`

	// Exportable allows key export.
	Exportable bool `json:"exportable,omitempty"`

	// AllowPlaintextBackup allows plaintext backup.
	AllowPlaintextBackup bool `json:"allow_plaintext_backup,omitempty"`

	// AutoRotatePeriod sets automatic key rotation period.
	AutoRotatePeriod time.Duration `json:"auto_rotate_period,omitempty"`
}

// CreateKeyWithOptions creates a key with specific options.
func (t *TransitEngine) CreateKeyWithOptions(ctx context.Context, name string, opts *CreateKeyOptions) error {
	if name == "" {
		return fmt.Errorf("key name is required")
	}

	path := fmt.Sprintf("%s/keys/%s", t.config.MountPath, name)

	data := make(map[string]interface{})
	if opts != nil {
		if opts.Type != "" {
			data["type"] = string(opts.Type)
		}
		if opts.Convergent {
			data["convergent_encryption"] = true
			data["derived"] = true // Required for convergent
		}
		if opts.Derived {
			data["derived"] = true
		}
		if opts.Exportable {
			data["exportable"] = true
		}
		if opts.AllowPlaintextBackup {
			data["allow_plaintext_backup"] = true
		}
		if opts.AutoRotatePeriod > 0 {
			data["auto_rotate_period"] = fmt.Sprintf("%ds", int(opts.AutoRotatePeriod.Seconds()))
		}
	}

	_, err := t.client.Write(ctx, path, data)
	return err
}

// GetKey retrieves information about a transit key.
func (t *TransitEngine) GetKey(ctx context.Context, name string) (*TransitKey, error) {
	if name == "" {
		return nil, fmt.Errorf("key name is required")
	}

	path := fmt.Sprintf("%s/keys/%s", t.config.MountPath, name)

	resp, err := t.client.Read(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to get key: %w", err)
	}

	return t.parseKey(name, resp)
}

// DeleteKey deletes a transit key.
func (t *TransitEngine) DeleteKey(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("key name is required")
	}

	// First enable deletion
	configPath := fmt.Sprintf("%s/keys/%s/config", t.config.MountPath, name)
	_, err := t.client.Write(ctx, configPath, map[string]interface{}{
		"deletion_allowed": true,
	})
	if err != nil {
		return fmt.Errorf("failed to enable deletion: %w", err)
	}

	// Then delete the key
	path := fmt.Sprintf("%s/keys/%s", t.config.MountPath, name)
	_, err = t.client.Delete(ctx, path)
	return err
}

// RotateKey rotates a transit key to a new version.
func (t *TransitEngine) RotateKey(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("key name is required")
	}

	path := fmt.Sprintf("%s/keys/%s/rotate", t.config.MountPath, name)
	_, err := t.client.Write(ctx, path, nil)
	return err
}

// ListKeys lists all transit keys.
func (t *TransitEngine) ListKeys(ctx context.Context) ([]string, error) {
	path := fmt.Sprintf("%s/keys", t.config.MountPath)
	return t.client.List(ctx, path)
}

// UpdateKeyConfig updates key configuration.
func (t *TransitEngine) UpdateKeyConfig(ctx context.Context, name string, minDecryptionVersion, minEncryptionVersion int, deletionAllowed bool) error {
	if name == "" {
		return fmt.Errorf("key name is required")
	}

	path := fmt.Sprintf("%s/keys/%s/config", t.config.MountPath, name)

	data := map[string]interface{}{
		"deletion_allowed": deletionAllowed,
	}

	if minDecryptionVersion > 0 {
		data["min_decryption_version"] = minDecryptionVersion
	}
	if minEncryptionVersion > 0 {
		data["min_encryption_version"] = minEncryptionVersion
	}

	_, err := t.client.Write(ctx, path, data)
	return err
}

// EncryptRequest specifies parameters for encryption.
type EncryptRequest struct {
	// KeyName is the name of the transit key.
	KeyName string `json:"key_name"`

	// Plaintext is the data to encrypt (will be base64 encoded).
	Plaintext []byte `json:"plaintext"`

	// Context is additional authenticated data for derived keys.
	Context []byte `json:"context,omitempty"`

	// KeyVersion is the key version to use (0 = latest).
	KeyVersion int `json:"key_version,omitempty"`

	// Nonce is the nonce for convergent encryption (must be base64).
	Nonce []byte `json:"nonce,omitempty"`

	// Type specifies ciphertext encoding (currently only "aes256-gcm96").
	Type string `json:"type,omitempty"`

	// ConvergentEncryption uses convergent encryption.
	ConvergentEncryption bool `json:"convergent_encryption,omitempty"`
}

// EncryptResponse contains the encryption result.
type EncryptResponse struct {
	// Ciphertext is the encrypted data (vault format: vault:vN:base64).
	Ciphertext string `json:"ciphertext"`

	// KeyVersion is the key version used.
	KeyVersion int `json:"key_version"`
}

// Encrypt encrypts plaintext data.
func (t *TransitEngine) Encrypt(ctx context.Context, req *EncryptRequest) (*EncryptResponse, error) {
	if req == nil || req.KeyName == "" {
		return nil, fmt.Errorf("key name is required")
	}
	if len(req.Plaintext) == 0 {
		return nil, fmt.Errorf("plaintext is required")
	}

	path := fmt.Sprintf("%s/encrypt/%s", t.config.MountPath, req.KeyName)

	data := map[string]interface{}{
		"plaintext": base64.StdEncoding.EncodeToString(req.Plaintext),
	}

	if len(req.Context) > 0 {
		data["context"] = base64.StdEncoding.EncodeToString(req.Context)
	}
	if req.KeyVersion > 0 {
		data["key_version"] = req.KeyVersion
	}
	if len(req.Nonce) > 0 {
		data["nonce"] = base64.StdEncoding.EncodeToString(req.Nonce)
	}
	if req.Type != "" {
		data["type"] = req.Type
	}
	if req.ConvergentEncryption {
		data["convergent_encryption"] = true
	}

	resp, err := t.client.Write(ctx, path, data)
	if err != nil {
		return nil, fmt.Errorf("encryption failed: %w", err)
	}

	return t.parseEncryptResponse(resp)
}

// DecryptRequest specifies parameters for decryption.
type DecryptRequest struct {
	// KeyName is the name of the transit key.
	KeyName string `json:"key_name"`

	// Ciphertext is the data to decrypt (vault format: vault:vN:base64).
	Ciphertext string `json:"ciphertext"`

	// Context is additional authenticated data for derived keys.
	Context []byte `json:"context,omitempty"`

	// Nonce is the nonce for convergent encryption.
	Nonce []byte `json:"nonce,omitempty"`
}

// DecryptResponse contains the decryption result.
type DecryptResponse struct {
	// Plaintext is the decrypted data.
	Plaintext []byte `json:"plaintext"`
}

// Decrypt decrypts ciphertext data.
func (t *TransitEngine) Decrypt(ctx context.Context, req *DecryptRequest) (*DecryptResponse, error) {
	if req == nil || req.KeyName == "" {
		return nil, fmt.Errorf("key name is required")
	}
	if req.Ciphertext == "" {
		return nil, fmt.Errorf("ciphertext is required")
	}

	path := fmt.Sprintf("%s/decrypt/%s", t.config.MountPath, req.KeyName)

	data := map[string]interface{}{
		"ciphertext": req.Ciphertext,
	}

	if len(req.Context) > 0 {
		data["context"] = base64.StdEncoding.EncodeToString(req.Context)
	}
	if len(req.Nonce) > 0 {
		data["nonce"] = base64.StdEncoding.EncodeToString(req.Nonce)
	}

	resp, err := t.client.Write(ctx, path, data)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return t.parseDecryptResponse(resp)
}

// Rewrap re-encrypts ciphertext with the latest key version.
func (t *TransitEngine) Rewrap(ctx context.Context, keyName, ciphertext string, context []byte) (*EncryptResponse, error) {
	if keyName == "" {
		return nil, fmt.Errorf("key name is required")
	}
	if ciphertext == "" {
		return nil, fmt.Errorf("ciphertext is required")
	}

	path := fmt.Sprintf("%s/rewrap/%s", t.config.MountPath, keyName)

	data := map[string]interface{}{
		"ciphertext": ciphertext,
	}

	if len(context) > 0 {
		data["context"] = base64.StdEncoding.EncodeToString(context)
	}

	resp, err := t.client.Write(ctx, path, data)
	if err != nil {
		return nil, fmt.Errorf("rewrap failed: %w", err)
	}

	return t.parseEncryptResponse(resp)
}

// SignRequest specifies parameters for signing.
type SignRequest struct {
	// KeyName is the name of the transit key.
	KeyName string `json:"key_name"`

	// Input is the data to sign.
	Input []byte `json:"input"`

	// KeyVersion is the key version to use (0 = latest).
	KeyVersion int `json:"key_version,omitempty"`

	// HashAlgorithm is the hash algorithm to use.
	HashAlgorithm TransitHashAlgorithm `json:"hash_algorithm,omitempty"`

	// SignatureAlgorithm is the signature algorithm (RSA keys only).
	SignatureAlgorithm TransitSignatureAlgorithm `json:"signature_algorithm,omitempty"`

	// Prehashed indicates if input is already hashed.
	Prehashed bool `json:"prehashed,omitempty"`

	// MarshalingAlgorithm for ECDSA (asn1 or jws).
	MarshalingAlgorithm string `json:"marshaling_algorithm,omitempty"`

	// SaltLength for RSA-PSS signatures.
	SaltLength string `json:"salt_length,omitempty"`
}

// SignResponse contains the signing result.
type SignResponse struct {
	// Signature is the generated signature (vault format: vault:vN:base64).
	Signature string `json:"signature"`

	// KeyVersion is the key version used.
	KeyVersion int `json:"key_version"`
}

// Sign creates a digital signature.
func (t *TransitEngine) Sign(ctx context.Context, req *SignRequest) (*SignResponse, error) {
	if req == nil || req.KeyName == "" {
		return nil, fmt.Errorf("key name is required")
	}
	if len(req.Input) == 0 {
		return nil, fmt.Errorf("input is required")
	}

	path := fmt.Sprintf("%s/sign/%s", t.config.MountPath, req.KeyName)

	data := map[string]interface{}{
		"input": base64.StdEncoding.EncodeToString(req.Input),
	}

	if req.KeyVersion > 0 {
		data["key_version"] = req.KeyVersion
	}
	if req.HashAlgorithm != "" {
		data["hash_algorithm"] = string(req.HashAlgorithm)
	}
	if req.SignatureAlgorithm != "" {
		data["signature_algorithm"] = string(req.SignatureAlgorithm)
	}
	if req.Prehashed {
		data["prehashed"] = true
	}
	if req.MarshalingAlgorithm != "" {
		data["marshaling_algorithm"] = req.MarshalingAlgorithm
	}
	if req.SaltLength != "" {
		data["salt_length"] = req.SaltLength
	}

	resp, err := t.client.Write(ctx, path, data)
	if err != nil {
		return nil, fmt.Errorf("signing failed: %w", err)
	}

	return t.parseSignResponse(resp)
}

// VerifyRequest specifies parameters for signature verification.
type VerifyRequest struct {
	// KeyName is the name of the transit key.
	KeyName string `json:"key_name"`

	// Input is the original data that was signed.
	Input []byte `json:"input"`

	// Signature is the signature to verify.
	Signature string `json:"signature"`

	// HashAlgorithm is the hash algorithm used.
	HashAlgorithm TransitHashAlgorithm `json:"hash_algorithm,omitempty"`

	// SignatureAlgorithm is the signature algorithm (RSA keys only).
	SignatureAlgorithm TransitSignatureAlgorithm `json:"signature_algorithm,omitempty"`

	// Prehashed indicates if input is already hashed.
	Prehashed bool `json:"prehashed,omitempty"`

	// MarshalingAlgorithm for ECDSA (asn1 or jws).
	MarshalingAlgorithm string `json:"marshaling_algorithm,omitempty"`

	// SaltLength for RSA-PSS signatures.
	SaltLength string `json:"salt_length,omitempty"`
}

// VerifyResponse contains the verification result.
type VerifyResponse struct {
	// Valid indicates if the signature is valid.
	Valid bool `json:"valid"`
}

// Verify verifies a digital signature.
func (t *TransitEngine) Verify(ctx context.Context, req *VerifyRequest) (*VerifyResponse, error) {
	if req == nil || req.KeyName == "" {
		return nil, fmt.Errorf("key name is required")
	}
	if len(req.Input) == 0 {
		return nil, fmt.Errorf("input is required")
	}
	if req.Signature == "" {
		return nil, fmt.Errorf("signature is required")
	}

	path := fmt.Sprintf("%s/verify/%s", t.config.MountPath, req.KeyName)

	data := map[string]interface{}{
		"input":     base64.StdEncoding.EncodeToString(req.Input),
		"signature": req.Signature,
	}

	if req.HashAlgorithm != "" {
		data["hash_algorithm"] = string(req.HashAlgorithm)
	}
	if req.SignatureAlgorithm != "" {
		data["signature_algorithm"] = string(req.SignatureAlgorithm)
	}
	if req.Prehashed {
		data["prehashed"] = true
	}
	if req.MarshalingAlgorithm != "" {
		data["marshaling_algorithm"] = req.MarshalingAlgorithm
	}
	if req.SaltLength != "" {
		data["salt_length"] = req.SaltLength
	}

	resp, err := t.client.Write(ctx, path, data)
	if err != nil {
		return nil, fmt.Errorf("verification failed: %w", err)
	}

	return t.parseVerifyResponse(resp)
}

// HMACRequest specifies parameters for HMAC generation.
type HMACRequest struct {
	// KeyName is the name of the transit key.
	KeyName string `json:"key_name"`

	// Input is the data to generate HMAC for.
	Input []byte `json:"input"`

	// KeyVersion is the key version to use (0 = latest).
	KeyVersion int `json:"key_version,omitempty"`

	// Algorithm is the hash algorithm (sha2-224, sha2-256, sha2-384, sha2-512).
	Algorithm TransitHashAlgorithm `json:"algorithm,omitempty"`
}

// HMACResponse contains the HMAC result.
type HMACResponse struct {
	// HMAC is the generated HMAC (vault format: vault:vN:base64).
	HMAC string `json:"hmac"`
}

// HMAC generates an HMAC.
func (t *TransitEngine) HMAC(ctx context.Context, req *HMACRequest) (*HMACResponse, error) {
	if req == nil || req.KeyName == "" {
		return nil, fmt.Errorf("key name is required")
	}
	if len(req.Input) == 0 {
		return nil, fmt.Errorf("input is required")
	}

	algorithm := req.Algorithm
	if algorithm == "" {
		algorithm = TransitHashSHA256
	}

	path := fmt.Sprintf("%s/hmac/%s/%s", t.config.MountPath, req.KeyName, algorithm)

	data := map[string]interface{}{
		"input": base64.StdEncoding.EncodeToString(req.Input),
	}

	if req.KeyVersion > 0 {
		data["key_version"] = req.KeyVersion
	}

	resp, err := t.client.Write(ctx, path, data)
	if err != nil {
		return nil, fmt.Errorf("HMAC generation failed: %w", err)
	}

	return t.parseHMACResponse(resp)
}

// VerifyHMAC verifies an HMAC.
func (t *TransitEngine) VerifyHMAC(ctx context.Context, keyName string, input []byte, hmac string, algorithm TransitHashAlgorithm) (bool, error) {
	if keyName == "" {
		return false, fmt.Errorf("key name is required")
	}
	if len(input) == 0 {
		return false, fmt.Errorf("input is required")
	}
	if hmac == "" {
		return false, fmt.Errorf("hmac is required")
	}

	if algorithm == "" {
		algorithm = TransitHashSHA256
	}

	path := fmt.Sprintf("%s/verify/%s/%s", t.config.MountPath, keyName, algorithm)

	data := map[string]interface{}{
		"input": base64.StdEncoding.EncodeToString(input),
		"hmac":  hmac,
	}

	resp, err := t.client.Write(ctx, path, data)
	if err != nil {
		return false, fmt.Errorf("HMAC verification failed: %w", err)
	}

	if data, ok := resp["data"].(map[string]interface{}); ok {
		if valid, ok := data["valid"].(bool); ok {
			return valid, nil
		}
	}

	return false, fmt.Errorf("invalid verification response")
}

// HashRequest specifies parameters for hashing.
type HashRequest struct {
	// Input is the data to hash.
	Input []byte `json:"input"`

	// Algorithm is the hash algorithm.
	Algorithm TransitHashAlgorithm `json:"algorithm,omitempty"`

	// Format is the output format (hex or base64).
	Format string `json:"format,omitempty"`
}

// HashResponse contains the hash result.
type HashResponse struct {
	// Sum is the generated hash.
	Sum string `json:"sum"`
}

// Hash generates a cryptographic hash.
func (t *TransitEngine) Hash(ctx context.Context, req *HashRequest) (*HashResponse, error) {
	if req == nil || len(req.Input) == 0 {
		return nil, fmt.Errorf("input is required")
	}

	algorithm := req.Algorithm
	if algorithm == "" {
		algorithm = TransitHashSHA256
	}

	path := fmt.Sprintf("%s/hash/%s", t.config.MountPath, algorithm)

	data := map[string]interface{}{
		"input": base64.StdEncoding.EncodeToString(req.Input),
	}

	if req.Format != "" {
		data["format"] = req.Format
	}

	resp, err := t.client.Write(ctx, path, data)
	if err != nil {
		return nil, fmt.Errorf("hashing failed: %w", err)
	}

	return t.parseHashResponse(resp)
}

// GenerateRandomBytes generates random bytes.
func (t *TransitEngine) GenerateRandomBytes(ctx context.Context, byteCount int, format string) ([]byte, error) {
	if byteCount <= 0 {
		return nil, fmt.Errorf("byte count must be positive")
	}

	path := fmt.Sprintf("%s/random/%d", t.config.MountPath, byteCount)

	data := make(map[string]interface{})
	if format != "" {
		data["format"] = format
	}

	resp, err := t.client.Write(ctx, path, data)
	if err != nil {
		return nil, fmt.Errorf("random generation failed: %w", err)
	}

	if respData, ok := resp["data"].(map[string]interface{}); ok {
		if randomBytes, ok := respData["random_bytes"].(string); ok {
			return base64.StdEncoding.DecodeString(randomBytes)
		}
	}

	return nil, fmt.Errorf("invalid random response")
}

// GenerateDataKey generates a data encryption key.
func (t *TransitEngine) GenerateDataKey(ctx context.Context, keyName string, bits int, context []byte) (plaintext, ciphertext []byte, err error) {
	if keyName == "" {
		return nil, nil, fmt.Errorf("key name is required")
	}

	path := fmt.Sprintf("%s/datakey/plaintext/%s", t.config.MountPath, keyName)

	data := make(map[string]interface{})
	if bits > 0 {
		data["bits"] = bits
	}
	if len(context) > 0 {
		data["context"] = base64.StdEncoding.EncodeToString(context)
	}

	resp, err := t.client.Write(ctx, path, data)
	if err != nil {
		return nil, nil, fmt.Errorf("data key generation failed: %w", err)
	}

	if respData, ok := resp["data"].(map[string]interface{}); ok {
		if pt, ok := respData["plaintext"].(string); ok {
			plaintext, _ = base64.StdEncoding.DecodeString(pt)
		}
		if ct, ok := respData["ciphertext"].(string); ok {
			ciphertext = []byte(ct)
		}
	}

	if len(plaintext) == 0 || len(ciphertext) == 0 {
		return nil, nil, fmt.Errorf("invalid data key response")
	}

	return plaintext, ciphertext, nil
}

// GenerateWrappedDataKey generates a wrapped data key (ciphertext only).
func (t *TransitEngine) GenerateWrappedDataKey(ctx context.Context, keyName string, bits int, context []byte) ([]byte, error) {
	if keyName == "" {
		return nil, fmt.Errorf("key name is required")
	}

	path := fmt.Sprintf("%s/datakey/wrapped/%s", t.config.MountPath, keyName)

	data := make(map[string]interface{})
	if bits > 0 {
		data["bits"] = bits
	}
	if len(context) > 0 {
		data["context"] = base64.StdEncoding.EncodeToString(context)
	}

	resp, err := t.client.Write(ctx, path, data)
	if err != nil {
		return nil, fmt.Errorf("wrapped data key generation failed: %w", err)
	}

	if respData, ok := resp["data"].(map[string]interface{}); ok {
		if ct, ok := respData["ciphertext"].(string); ok {
			return []byte(ct), nil
		}
	}

	return nil, fmt.Errorf("invalid wrapped data key response")
}

// Batch operations

// BatchEncryptItem represents an item for batch encryption.
type BatchEncryptItem struct {
	Plaintext  []byte `json:"plaintext"`
	Context    []byte `json:"context,omitempty"`
	KeyVersion int    `json:"key_version,omitempty"`
}

// BatchEncrypt encrypts multiple items in a single request.
func (t *TransitEngine) BatchEncrypt(ctx context.Context, keyName string, items []BatchEncryptItem) ([]string, error) {
	if keyName == "" {
		return nil, fmt.Errorf("key name is required")
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("items are required")
	}

	path := fmt.Sprintf("%s/encrypt/%s", t.config.MountPath, keyName)

	batchInput := make([]map[string]interface{}, len(items))
	for i, item := range items {
		batchInput[i] = map[string]interface{}{
			"plaintext": base64.StdEncoding.EncodeToString(item.Plaintext),
		}
		if len(item.Context) > 0 {
			batchInput[i]["context"] = base64.StdEncoding.EncodeToString(item.Context)
		}
		if item.KeyVersion > 0 {
			batchInput[i]["key_version"] = item.KeyVersion
		}
	}

	data := map[string]interface{}{
		"batch_input": batchInput,
	}

	resp, err := t.client.Write(ctx, path, data)
	if err != nil {
		return nil, fmt.Errorf("batch encryption failed: %w", err)
	}

	return t.parseBatchEncryptResponse(resp)
}

// BatchDecryptItem represents an item for batch decryption.
type BatchDecryptItem struct {
	Ciphertext string `json:"ciphertext"`
	Context    []byte `json:"context,omitempty"`
}

// BatchDecrypt decrypts multiple items in a single request.
func (t *TransitEngine) BatchDecrypt(ctx context.Context, keyName string, items []BatchDecryptItem) ([][]byte, error) {
	if keyName == "" {
		return nil, fmt.Errorf("key name is required")
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("items are required")
	}

	path := fmt.Sprintf("%s/decrypt/%s", t.config.MountPath, keyName)

	batchInput := make([]map[string]interface{}, len(items))
	for i, item := range items {
		batchInput[i] = map[string]interface{}{
			"ciphertext": item.Ciphertext,
		}
		if len(item.Context) > 0 {
			batchInput[i]["context"] = base64.StdEncoding.EncodeToString(item.Context)
		}
	}

	data := map[string]interface{}{
		"batch_input": batchInput,
	}

	resp, err := t.client.Write(ctx, path, data)
	if err != nil {
		return nil, fmt.Errorf("batch decryption failed: %w", err)
	}

	return t.parseBatchDecryptResponse(resp)
}

// Response parsing helpers

func (t *TransitEngine) parseKey(name string, resp map[string]interface{}) (*TransitKey, error) {
	if resp == nil {
		return nil, secrets.ErrSecretNotFound
	}

	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid key response")
	}

	key := &TransitKey{Name: name}

	if keyType, ok := data["type"].(string); ok {
		key.Type = TransitKeyType(keyType)
	}
	if deletionAllowed, ok := data["deletion_allowed"].(bool); ok {
		key.DeletionAllowed = deletionAllowed
	}
	if derived, ok := data["derived"].(bool); ok {
		key.Derived = derived
	}
	if exportable, ok := data["exportable"].(bool); ok {
		key.Exportable = exportable
	}
	if allowPlaintextBackup, ok := data["allow_plaintext_backup"].(bool); ok {
		key.AllowPlaintextBackup = allowPlaintextBackup
	}
	if latestVersion, ok := data["latest_version"].(float64); ok {
		key.LatestVersion = int(latestVersion)
	}
	if minDecryptionVersion, ok := data["min_decryption_version"].(float64); ok {
		key.MinDecryptionVersion = int(minDecryptionVersion)
	}
	if minEncryptionVersion, ok := data["min_encryption_version"].(float64); ok {
		key.MinEncryptionVersion = int(minEncryptionVersion)
	}
	if supportsEncryption, ok := data["supports_encryption"].(bool); ok {
		key.SupportsEncryption = supportsEncryption
	}
	if supportsDecryption, ok := data["supports_decryption"].(bool); ok {
		key.SupportsDecryption = supportsDecryption
	}
	if supportsSigning, ok := data["supports_signing"].(bool); ok {
		key.SupportsSigning = supportsSigning
	}
	if supportsDerivation, ok := data["supports_derivation"].(bool); ok {
		key.SupportsDerivation = supportsDerivation
	}

	return key, nil
}

func (t *TransitEngine) parseEncryptResponse(resp map[string]interface{}) (*EncryptResponse, error) {
	if resp == nil {
		return nil, fmt.Errorf("empty response")
	}

	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid encrypt response")
	}

	result := &EncryptResponse{}

	if ciphertext, ok := data["ciphertext"].(string); ok {
		result.Ciphertext = ciphertext
		// Extract key version from ciphertext (format: vault:vN:base64)
		parts := strings.Split(ciphertext, ":")
		if len(parts) >= 2 && strings.HasPrefix(parts[1], "v") {
			fmt.Sscanf(parts[1], "v%d", &result.KeyVersion)
		}
	}

	return result, nil
}

func (t *TransitEngine) parseDecryptResponse(resp map[string]interface{}) (*DecryptResponse, error) {
	if resp == nil {
		return nil, fmt.Errorf("empty response")
	}

	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid decrypt response")
	}

	result := &DecryptResponse{}

	if plaintext, ok := data["plaintext"].(string); ok {
		decoded, err := base64.StdEncoding.DecodeString(plaintext)
		if err != nil {
			return nil, fmt.Errorf("failed to decode plaintext: %w", err)
		}
		result.Plaintext = decoded
	}

	return result, nil
}

func (t *TransitEngine) parseSignResponse(resp map[string]interface{}) (*SignResponse, error) {
	if resp == nil {
		return nil, fmt.Errorf("empty response")
	}

	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid sign response")
	}

	result := &SignResponse{}

	if signature, ok := data["signature"].(string); ok {
		result.Signature = signature
		// Extract key version from signature (format: vault:vN:base64)
		parts := strings.Split(signature, ":")
		if len(parts) >= 2 && strings.HasPrefix(parts[1], "v") {
			fmt.Sscanf(parts[1], "v%d", &result.KeyVersion)
		}
	}

	return result, nil
}

func (t *TransitEngine) parseVerifyResponse(resp map[string]interface{}) (*VerifyResponse, error) {
	if resp == nil {
		return nil, fmt.Errorf("empty response")
	}

	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid verify response")
	}

	result := &VerifyResponse{}

	if valid, ok := data["valid"].(bool); ok {
		result.Valid = valid
	}

	return result, nil
}

func (t *TransitEngine) parseHMACResponse(resp map[string]interface{}) (*HMACResponse, error) {
	if resp == nil {
		return nil, fmt.Errorf("empty response")
	}

	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid HMAC response")
	}

	result := &HMACResponse{}

	if hmac, ok := data["hmac"].(string); ok {
		result.HMAC = hmac
	}

	return result, nil
}

func (t *TransitEngine) parseHashResponse(resp map[string]interface{}) (*HashResponse, error) {
	if resp == nil {
		return nil, fmt.Errorf("empty response")
	}

	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid hash response")
	}

	result := &HashResponse{}

	if sum, ok := data["sum"].(string); ok {
		result.Sum = sum
	}

	return result, nil
}

func (t *TransitEngine) parseBatchEncryptResponse(resp map[string]interface{}) ([]string, error) {
	if resp == nil {
		return nil, fmt.Errorf("empty response")
	}

	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid batch encrypt response")
	}

	batchResults, ok := data["batch_results"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("no batch results")
	}

	results := make([]string, len(batchResults))
	for i, item := range batchResults {
		if m, ok := item.(map[string]interface{}); ok {
			if ciphertext, ok := m["ciphertext"].(string); ok {
				results[i] = ciphertext
			}
			if errMsg, ok := m["error"].(string); ok && errMsg != "" {
				return nil, fmt.Errorf("batch item %d failed: %s", i, errMsg)
			}
		}
	}

	return results, nil
}

func (t *TransitEngine) parseBatchDecryptResponse(resp map[string]interface{}) ([][]byte, error) {
	if resp == nil {
		return nil, fmt.Errorf("empty response")
	}

	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid batch decrypt response")
	}

	batchResults, ok := data["batch_results"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("no batch results")
	}

	results := make([][]byte, len(batchResults))
	for i, item := range batchResults {
		if m, ok := item.(map[string]interface{}); ok {
			if plaintext, ok := m["plaintext"].(string); ok {
				decoded, err := base64.StdEncoding.DecodeString(plaintext)
				if err != nil {
					return nil, fmt.Errorf("failed to decode item %d: %w", i, err)
				}
				results[i] = decoded
			}
			if errMsg, ok := m["error"].(string); ok && errMsg != "" {
				return nil, fmt.Errorf("batch item %d failed: %s", i, errMsg)
			}
		}
	}

	return results, nil
}

// ToTransitRequest converts request to secrets.TransitRequest.
func (req *EncryptRequest) ToTransitRequest() *secrets.TransitRequest {
	return &secrets.TransitRequest{
		Operation:  secrets.TransitOperationEncrypt,
		KeyName:    req.KeyName,
		KeyVersion: req.KeyVersion,
		Plaintext:  req.Plaintext,
		Context:    req.Context,
		Convergent: req.ConvergentEncryption,
	}
}

// ToTransitResponse converts response to secrets.TransitResponse.
func (resp *EncryptResponse) ToTransitResponse() *secrets.TransitResponse {
	return &secrets.TransitResponse{
		Ciphertext: resp.Ciphertext,
		KeyVersion: resp.KeyVersion,
	}
}
