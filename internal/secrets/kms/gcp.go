package kms

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	kms "cloud.google.com/go/kms/apiv1"
	"cloud.google.com/go/kms/apiv1/kmspb"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// GCPConfig configures the GCP Cloud KMS provider.
type GCPConfig struct {
	ProviderConfig

	// ProjectID is the GCP project ID.
	ProjectID string `json:"project_id"`

	// Location is the GCP region (e.g., "us-central1", "global").
	Location string `json:"location"`

	// KeyRing is the key ring name.
	KeyRing string `json:"key_ring"`

	// ServiceAccountKeyFile is the path to the service account key file.
	ServiceAccountKeyFile string `json:"service_account_key_file,omitempty"`

	// ServiceAccountKeyJSON is the raw service account key JSON.
	ServiceAccountKeyJSON []byte `json:"-"`

	// ImpersonateServiceAccount is the service account to impersonate.
	ImpersonateServiceAccount string `json:"impersonate_service_account,omitempty"`

	// DefaultKeyName is the default key name to use.
	DefaultKeyName string `json:"default_key_name,omitempty"`

	// Endpoint overrides the default API endpoint.
	Endpoint string `json:"endpoint,omitempty"`
}

// DefaultGCPConfig returns default GCP Cloud KMS configuration.
func DefaultGCPConfig() *GCPConfig {
	return &GCPConfig{
		ProviderConfig: ProviderConfig{
			Name:       "gcp-cloudkms",
			Type:       ProviderTypeGCP,
			Timeout:    30 * time.Second,
			MaxRetries: 3,
		},
		Location: "global",
	}
}

// GCPProvider implements the KMS Provider interface for GCP Cloud KMS.
type GCPProvider struct {
	config *GCPConfig
	client *kms.KeyManagementClient
	closed atomic.Bool
}

// NewGCPProvider creates a new GCP Cloud KMS provider.
func NewGCPProvider(ctx context.Context, cfg *GCPConfig) (*GCPProvider, error) {
	if cfg == nil {
		return nil, errors.New("config is required")
	}

	if cfg.ProjectID == "" {
		return nil, errors.New("project_id is required")
	}

	if cfg.KeyRing == "" {
		return nil, errors.New("key_ring is required")
	}

	// Build client options
	opts, err := buildGCPClientOptions(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to build client options: %w", err)
	}

	// Create KMS client
	client, err := kms.NewKeyManagementClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create KMS client: %w", err)
	}

	return &GCPProvider{
		config: cfg,
		client: client,
	}, nil
}

// buildGCPClientOptions builds the client options based on configuration.
func buildGCPClientOptions(cfg *GCPConfig) ([]option.ClientOption, error) {
	var opts []option.ClientOption

	// Set endpoint if specified
	if cfg.Endpoint != "" {
		opts = append(opts, option.WithEndpoint(cfg.Endpoint))
	}

	// Configure authentication
	if len(cfg.ServiceAccountKeyJSON) > 0 {
		opts = append(opts, option.WithCredentialsJSON(cfg.ServiceAccountKeyJSON))
	} else if cfg.ServiceAccountKeyFile != "" {
		opts = append(opts, option.WithCredentialsFile(cfg.ServiceAccountKeyFile))
	}

	// Handle impersonation
	if cfg.ImpersonateServiceAccount != "" {
		opts = append(opts, option.ImpersonateCredentials(cfg.ImpersonateServiceAccount))
	}

	return opts, nil
}

// keyPath constructs the full key resource path.
func (p *GCPProvider) keyPath(keyName string) string {
	return fmt.Sprintf("projects/%s/locations/%s/keyRings/%s/cryptoKeys/%s",
		p.config.ProjectID, p.config.Location, p.config.KeyRing, keyName)
}

// keyRingPath constructs the key ring resource path.
func (p *GCPProvider) keyRingPath() string {
	return fmt.Sprintf("projects/%s/locations/%s/keyRings/%s",
		p.config.ProjectID, p.config.Location, p.config.KeyRing)
}

// Type returns the provider type.
func (p *GCPProvider) Type() ProviderType {
	return ProviderTypeGCP
}

// Name returns the provider instance name.
func (p *GCPProvider) Name() string {
	return p.config.Name
}

// Healthy checks if the provider is healthy.
func (p *GCPProvider) Healthy(ctx context.Context) bool {
	if p.closed.Load() {
		return false
	}

	// Try to get key ring to verify connectivity
	_, err := p.client.GetKeyRing(ctx, &kmspb.GetKeyRingRequest{
		Name: p.keyRingPath(),
	})
	return err == nil
}

// GetKeyMetadata retrieves metadata for a key.
func (p *GCPProvider) GetKeyMetadata(ctx context.Context, keyID string) (*KeyMetadata, error) {
	if p.closed.Load() {
		return nil, ErrProviderUnavailable
	}

	key, err := p.client.GetCryptoKey(ctx, &kmspb.GetCryptoKeyRequest{
		Name: p.keyPath(keyID),
	})
	if err != nil {
		return nil, translateGCPError(err)
	}

	meta := &KeyMetadata{
		KeyID:       keyID,
		ARN:         key.Name,
		Provider:    ProviderTypeGCP,
		CreatedAt:   key.CreateTime.AsTime(),
		Tags:        key.Labels,
	}

	// Determine key type and spec
	switch key.Purpose {
	case kmspb.CryptoKey_ENCRYPT_DECRYPT:
		meta.KeyUsage = KeyUsageEncryptDecrypt
	case kmspb.CryptoKey_ASYMMETRIC_SIGN:
		meta.KeyUsage = KeyUsageSignVerify
	case kmspb.CryptoKey_ASYMMETRIC_DECRYPT:
		meta.KeyUsage = KeyUsageEncryptDecrypt
		meta.KeyType = KeyTypeAsymmetric
	}

	// Get primary version to determine key spec
	if key.Primary != nil {
		switch key.Primary.Algorithm {
		case kmspb.CryptoKeyVersion_GOOGLE_SYMMETRIC_ENCRYPTION:
			meta.KeyType = KeyTypeSymmetric
			meta.KeySpec = KeySpecAES256
		case kmspb.CryptoKeyVersion_RSA_DECRYPT_OAEP_2048_SHA256,
			kmspb.CryptoKeyVersion_RSA_DECRYPT_OAEP_2048_SHA1,
			kmspb.CryptoKeyVersion_RSA_SIGN_PKCS1_2048_SHA256:
			meta.KeyType = KeyTypeAsymmetric
			meta.KeySpec = KeySpecRSA2048
		case kmspb.CryptoKeyVersion_RSA_DECRYPT_OAEP_4096_SHA256,
			kmspb.CryptoKeyVersion_RSA_DECRYPT_OAEP_4096_SHA512,
			kmspb.CryptoKeyVersion_RSA_SIGN_PKCS1_4096_SHA256:
			meta.KeyType = KeyTypeAsymmetric
			meta.KeySpec = KeySpecRSA4096
		case kmspb.CryptoKeyVersion_EC_SIGN_P256_SHA256:
			meta.KeyType = KeyTypeAsymmetric
			meta.KeySpec = KeySpecECCNISTP256
		case kmspb.CryptoKeyVersion_EC_SIGN_P384_SHA384:
			meta.KeyType = KeyTypeAsymmetric
			meta.KeySpec = KeySpecECCNISTP384
		}

		meta.Enabled = key.Primary.State == kmspb.CryptoKeyVersion_ENABLED
	}

	// Check rotation schedule
	if key.NextRotationTime != nil {
		meta.RotationEnabled = true
		meta.NextRotationDate = key.NextRotationTime.AsTime()
	}

	return meta, nil
}

// Encrypt encrypts plaintext data.
func (p *GCPProvider) Encrypt(ctx context.Context, req *EncryptRequest) (*EncryptResponse, error) {
	if p.closed.Load() {
		return nil, ErrProviderUnavailable
	}

	keyName := req.KeyID
	if keyName == "" {
		keyName = p.config.DefaultKeyName
	}
	if keyName == "" {
		return nil, ErrInvalidKey
	}

	encryptReq := &kmspb.EncryptRequest{
		Name:      p.keyPath(keyName),
		Plaintext: req.Plaintext,
	}

	// Add AAD if context provided
	if len(req.Context) > 0 {
		encryptReq.AdditionalAuthenticatedData = encodeGCPContext(req.Context)
	}

	result, err := p.client.Encrypt(ctx, encryptReq)
	if err != nil {
		return nil, translateGCPError(err)
	}

	return &EncryptResponse{
		Ciphertext: result.Ciphertext,
		KeyID:      keyName,
	}, nil
}

// Decrypt decrypts ciphertext data.
func (p *GCPProvider) Decrypt(ctx context.Context, req *DecryptRequest) (*DecryptResponse, error) {
	if p.closed.Load() {
		return nil, ErrProviderUnavailable
	}

	keyName := req.KeyID
	if keyName == "" {
		keyName = p.config.DefaultKeyName
	}
	if keyName == "" {
		return nil, ErrInvalidKey
	}

	decryptReq := &kmspb.DecryptRequest{
		Name:       p.keyPath(keyName),
		Ciphertext: req.Ciphertext,
	}

	// Add AAD if context provided
	if len(req.Context) > 0 {
		decryptReq.AdditionalAuthenticatedData = encodeGCPContext(req.Context)
	}

	result, err := p.client.Decrypt(ctx, decryptReq)
	if err != nil {
		return nil, translateGCPError(err)
	}

	return &DecryptResponse{
		Plaintext: result.Plaintext,
		KeyID:     keyName,
	}, nil
}

// GenerateDataKey generates a data encryption key.
func (p *GCPProvider) GenerateDataKey(ctx context.Context, req *GenerateDataKeyRequest) (*DataKey, error) {
	if p.closed.Load() {
		return nil, ErrProviderUnavailable
	}

	keyName := req.KeyID
	if keyName == "" {
		keyName = p.config.DefaultKeyName
	}
	if keyName == "" {
		return nil, ErrInvalidKey
	}

	// Determine key size
	keySize := 32 // Default to AES-256
	if req.NumberOfBytes > 0 {
		keySize = req.NumberOfBytes
	}

	// Generate random data key locally
	plaintext := make([]byte, keySize)
	if _, err := rand.Read(plaintext); err != nil {
		return nil, fmt.Errorf("failed to generate data key: %w", err)
	}

	// Encrypt the data key with the KEK
	encryptReq := &kmspb.EncryptRequest{
		Name:      p.keyPath(keyName),
		Plaintext: plaintext,
	}

	if len(req.Context) > 0 {
		encryptReq.AdditionalAuthenticatedData = encodeGCPContext(req.Context)
	}

	result, err := p.client.Encrypt(ctx, encryptReq)
	if err != nil {
		// Zero out plaintext on error
		for i := range plaintext {
			plaintext[i] = 0
		}
		return nil, translateGCPError(err)
	}

	dataKey := &DataKey{
		Ciphertext:  result.Ciphertext,
		KeyID:       keyName,
		Provider:    ProviderTypeGCP,
		KeySpec:     req.KeySpec,
		GeneratedAt: time.Now(),
	}

	if !req.WithoutPlaintext {
		dataKey.Plaintext = plaintext
	} else {
		// Zero out plaintext
		for i := range plaintext {
			plaintext[i] = 0
		}
	}

	return dataKey, nil
}

// WrapKey wraps (encrypts) a key with the KMS key.
func (p *GCPProvider) WrapKey(ctx context.Context, req *WrapKeyRequest) (*WrapKeyResponse, error) {
	if p.closed.Load() {
		return nil, ErrProviderUnavailable
	}

	keyName := req.WrapperKeyID
	if keyName == "" {
		keyName = p.config.DefaultKeyName
	}
	if keyName == "" {
		return nil, ErrInvalidKey
	}

	encryptReq := &kmspb.EncryptRequest{
		Name:      p.keyPath(keyName),
		Plaintext: req.KeyToWrap,
	}

	if len(req.Context) > 0 {
		encryptReq.AdditionalAuthenticatedData = encodeGCPContext(req.Context)
	}

	result, err := p.client.Encrypt(ctx, encryptReq)
	if err != nil {
		return nil, translateGCPError(err)
	}

	return &WrapKeyResponse{
		WrappedKey:   result.Ciphertext,
		WrapperKeyID: keyName,
	}, nil
}

// UnwrapKey unwraps (decrypts) a key with the KMS key.
func (p *GCPProvider) UnwrapKey(ctx context.Context, req *UnwrapKeyRequest) (*UnwrapKeyResponse, error) {
	if p.closed.Load() {
		return nil, ErrProviderUnavailable
	}

	keyName := req.WrapperKeyID
	if keyName == "" {
		keyName = p.config.DefaultKeyName
	}
	if keyName == "" {
		return nil, ErrInvalidKey
	}

	decryptReq := &kmspb.DecryptRequest{
		Name:       p.keyPath(keyName),
		Ciphertext: req.WrappedKey,
	}

	if len(req.Context) > 0 {
		decryptReq.AdditionalAuthenticatedData = encodeGCPContext(req.Context)
	}

	result, err := p.client.Decrypt(ctx, decryptReq)
	if err != nil {
		return nil, translateGCPError(err)
	}

	return &UnwrapKeyResponse{
		PlaintextKey: result.Plaintext,
		WrapperKeyID: keyName,
	}, nil
}

// Sign signs data with the KMS key.
func (p *GCPProvider) Sign(ctx context.Context, req *SignRequest) (*SignResponse, error) {
	if p.closed.Load() {
		return nil, ErrProviderUnavailable
	}

	// Get the primary version for signing
	key, err := p.client.GetCryptoKey(ctx, &kmspb.GetCryptoKeyRequest{
		Name: p.keyPath(req.KeyID),
	})
	if err != nil {
		return nil, translateGCPError(err)
	}

	signReq := &kmspb.AsymmetricSignRequest{
		Name: key.Primary.Name,
	}

	// Handle message type
	if req.MessageType == "DIGEST" {
		signReq.Digest = &kmspb.Digest{
			Digest: &kmspb.Digest_Sha256{
				Sha256: req.Message,
			},
		}
	} else {
		signReq.Data = req.Message
	}

	result, err := p.client.AsymmetricSign(ctx, signReq)
	if err != nil {
		return nil, translateGCPError(err)
	}

	return &SignResponse{
		Signature: result.Signature,
		KeyID:     req.KeyID,
		Algorithm: req.Algorithm,
	}, nil
}

// Verify verifies a signature with the KMS key.
func (p *GCPProvider) Verify(ctx context.Context, req *VerifyRequest) (*VerifyResponse, error) {
	if p.closed.Load() {
		return nil, ErrProviderUnavailable
	}

	// Get the primary version for verification
	key, err := p.client.GetCryptoKey(ctx, &kmspb.GetCryptoKeyRequest{
		Name: p.keyPath(req.KeyID),
	})
	if err != nil {
		return nil, translateGCPError(err)
	}

	// Get the public key
	pubKeyResp, err := p.client.GetPublicKey(ctx, &kmspb.GetPublicKeyRequest{
		Name: key.Primary.Name,
	})
	if err != nil {
		return nil, translateGCPError(err)
	}

	// Use MacVerify for symmetric keys or verify locally for asymmetric
	// GCP Cloud KMS doesn't have a direct Verify API for asymmetric keys,
	// we need to verify locally using the public key
	_ = pubKeyResp // Would use for local verification

	// For this implementation, we'll return that verification is supported
	// but would require local crypto verification with the public key
	return nil, ErrUnsupportedOperation
}

// EnableKeyRotation enables automatic key rotation.
func (p *GCPProvider) EnableKeyRotation(ctx context.Context, keyID string) error {
	if p.closed.Load() {
		return ErrProviderUnavailable
	}

	// GCP requires setting a rotation schedule through UpdateCryptoKey
	// This operation requires more configuration than a simple enable call
	return ErrUnsupportedOperation
}

// DisableKeyRotation disables automatic key rotation.
func (p *GCPProvider) DisableKeyRotation(ctx context.Context, keyID string) error {
	if p.closed.Load() {
		return ErrProviderUnavailable
	}

	// GCP doesn't have a way to disable rotation without removing the schedule
	return ErrUnsupportedOperation
}

// RotateKey manually rotates a key.
func (p *GCPProvider) RotateKey(ctx context.Context, keyID string) error {
	if p.closed.Load() {
		return ErrProviderUnavailable
	}

	_, err := p.client.CreateCryptoKeyVersion(ctx, &kmspb.CreateCryptoKeyVersionRequest{
		Parent: p.keyPath(keyID),
	})
	return translateGCPError(err)
}

// ListKeyVersions lists all versions of a key.
func (p *GCPProvider) ListKeyVersions(ctx context.Context, keyID string) ([]KeyVersion, error) {
	if p.closed.Load() {
		return nil, ErrProviderUnavailable
	}

	var versions []KeyVersion

	it := p.client.ListCryptoKeyVersions(ctx, &kmspb.ListCryptoKeyVersionsRequest{
		Parent: p.keyPath(keyID),
	})

	// Get the key to find primary version
	key, err := p.client.GetCryptoKey(ctx, &kmspb.GetCryptoKeyRequest{
		Name: p.keyPath(keyID),
	})
	if err != nil {
		return nil, translateGCPError(err)
	}
	primaryName := ""
	if key.Primary != nil {
		primaryName = key.Primary.Name
	}

	for {
		version, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, translateGCPError(err)
		}

		v := KeyVersion{
			Version:   extractVersionNumber(version.Name),
			CreatedAt: version.CreateTime.AsTime(),
			Primary:   version.Name == primaryName,
		}

		switch version.State {
		case kmspb.CryptoKeyVersion_ENABLED:
			v.State = "enabled"
		case kmspb.CryptoKeyVersion_DISABLED:
			v.State = "disabled"
		case kmspb.CryptoKeyVersion_DESTROYED:
			v.State = "destroyed"
		case kmspb.CryptoKeyVersion_DESTROY_SCHEDULED:
			v.State = "pending_destruction"
		default:
			v.State = "unknown"
		}

		versions = append(versions, v)
	}

	return versions, nil
}

// Close closes the provider.
func (p *GCPProvider) Close() error {
	p.closed.Store(true)
	if p.client != nil {
		return p.client.Close()
	}
	return nil
}

// DestroyKeyVersion schedules a key version for destruction.
func (p *GCPProvider) DestroyKeyVersion(ctx context.Context, keyID, versionID string) error {
	if p.closed.Load() {
		return ErrProviderUnavailable
	}

	versionPath := fmt.Sprintf("%s/cryptoKeyVersions/%s", p.keyPath(keyID), versionID)

	_, err := p.client.DestroyCryptoKeyVersion(ctx, &kmspb.DestroyCryptoKeyVersionRequest{
		Name: versionPath,
	})
	return translateGCPError(err)
}

// RestoreKeyVersion restores a key version scheduled for destruction.
func (p *GCPProvider) RestoreKeyVersion(ctx context.Context, keyID, versionID string) error {
	if p.closed.Load() {
		return ErrProviderUnavailable
	}

	versionPath := fmt.Sprintf("%s/cryptoKeyVersions/%s", p.keyPath(keyID), versionID)

	_, err := p.client.RestoreCryptoKeyVersion(ctx, &kmspb.RestoreCryptoKeyVersionRequest{
		Name: versionPath,
	})
	return translateGCPError(err)
}

// UpdatePrimaryVersion sets the primary key version.
func (p *GCPProvider) UpdatePrimaryVersion(ctx context.Context, keyID, versionID string) error {
	if p.closed.Load() {
		return ErrProviderUnavailable
	}

	versionPath := fmt.Sprintf("%s/cryptoKeyVersions/%s", p.keyPath(keyID), versionID)

	_, err := p.client.UpdateCryptoKeyPrimaryVersion(ctx, &kmspb.UpdateCryptoKeyPrimaryVersionRequest{
		Name:               p.keyPath(keyID),
		CryptoKeyVersionId: versionPath,
	})
	return translateGCPError(err)
}

// GenerateRandomBytes generates random bytes using Cloud KMS.
func (p *GCPProvider) GenerateRandomBytes(ctx context.Context, length int) ([]byte, error) {
	if p.closed.Load() {
		return nil, ErrProviderUnavailable
	}

	result, err := p.client.GenerateRandomBytes(ctx, &kmspb.GenerateRandomBytesRequest{
		Location:        fmt.Sprintf("projects/%s/locations/%s", p.config.ProjectID, p.config.Location),
		LengthBytes:     int32(length),
		ProtectionLevel: kmspb.ProtectionLevel_HSM,
	})
	if err != nil {
		return nil, translateGCPError(err)
	}

	return result.Data, nil
}

// MacSign generates a MAC signature.
func (p *GCPProvider) MacSign(ctx context.Context, keyID string, data []byte) ([]byte, error) {
	if p.closed.Load() {
		return nil, ErrProviderUnavailable
	}

	key, err := p.client.GetCryptoKey(ctx, &kmspb.GetCryptoKeyRequest{
		Name: p.keyPath(keyID),
	})
	if err != nil {
		return nil, translateGCPError(err)
	}

	result, err := p.client.MacSign(ctx, &kmspb.MacSignRequest{
		Name: key.Primary.Name,
		Data: data,
	})
	if err != nil {
		return nil, translateGCPError(err)
	}

	return result.Mac, nil
}

// MacVerify verifies a MAC signature.
func (p *GCPProvider) MacVerify(ctx context.Context, keyID string, data, mac []byte) (bool, error) {
	if p.closed.Load() {
		return false, ErrProviderUnavailable
	}

	key, err := p.client.GetCryptoKey(ctx, &kmspb.GetCryptoKeyRequest{
		Name: p.keyPath(keyID),
	})
	if err != nil {
		return false, translateGCPError(err)
	}

	result, err := p.client.MacVerify(ctx, &kmspb.MacVerifyRequest{
		Name: key.Primary.Name,
		Data: data,
		Mac:  mac,
	})
	if err != nil {
		return false, translateGCPError(err)
	}

	return result.Success, nil
}

// RawEncrypt performs raw encryption with asymmetric keys.
func (p *GCPProvider) RawEncrypt(ctx context.Context, keyID, versionID string, plaintext []byte) ([]byte, error) {
	if p.closed.Load() {
		return nil, ErrProviderUnavailable
	}

	versionPath := fmt.Sprintf("%s/cryptoKeyVersions/%s", p.keyPath(keyID), versionID)

	result, err := p.client.RawEncrypt(ctx, &kmspb.RawEncryptRequest{
		Name:      versionPath,
		Plaintext: plaintext,
	})
	if err != nil {
		return nil, translateGCPError(err)
	}

	return result.Ciphertext, nil
}

// RawDecrypt performs raw decryption with asymmetric keys.
func (p *GCPProvider) RawDecrypt(ctx context.Context, keyID, versionID string, ciphertext []byte) ([]byte, error) {
	if p.closed.Load() {
		return nil, ErrProviderUnavailable
	}

	versionPath := fmt.Sprintf("%s/cryptoKeyVersions/%s", p.keyPath(keyID), versionID)

	result, err := p.client.RawDecrypt(ctx, &kmspb.RawDecryptRequest{
		Name:       versionPath,
		Ciphertext: ciphertext,
	})
	if err != nil {
		return nil, translateGCPError(err)
	}

	return result.Plaintext, nil
}

// extractVersionNumber extracts the version number from a version name.
func extractVersionNumber(versionName string) string {
	parts := strings.Split(versionName, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

// encodeGCPContext encodes encryption context as AAD bytes.
func encodeGCPContext(ctx map[string]string) []byte {
	var parts []string
	for k, v := range ctx {
		parts = append(parts, k+"="+v)
	}
	return []byte(strings.Join(parts, ";"))
}

// translateGCPError translates GCP errors to KMS package errors.
func translateGCPError(err error) error {
	if err == nil {
		return nil
	}

	st, ok := status.FromError(err)
	if !ok {
		return fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
	}

	switch st.Code() {
	case codes.NotFound:
		return ErrKeyNotFound
	case codes.PermissionDenied:
		return ErrAccessDenied
	case codes.Unauthenticated:
		return ErrAccessDenied
	case codes.InvalidArgument:
		return ErrInvalidKey
	case codes.FailedPrecondition:
		if strings.Contains(st.Message(), "disabled") {
			return ErrKeyDisabled
		}
		return ErrInvalidCiphertext
	case codes.Unavailable:
		return ErrProviderUnavailable
	default:
		return fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
	}
}

// Helper for wrapping int values
func int32Wrapper(v int32) *wrapperspb.Int32Value {
	return wrapperspb.Int32(v)
}

// Ensure GCPProvider implements interfaces.
var (
	_ Provider         = (*GCPProvider)(nil)
	_ SigningProvider  = (*GCPProvider)(nil)
	_ RotatingProvider = (*GCPProvider)(nil)
)
