package kms

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys"
)

// AzureConfig configures the Azure Key Vault KMS provider.
type AzureConfig struct {
	ProviderConfig

	// VaultURL is the URL of the Azure Key Vault (e.g., "https://myvault.vault.azure.net").
	VaultURL string `json:"vault_url"`

	// TenantID is the Azure AD tenant ID.
	TenantID string `json:"tenant_id,omitempty"`

	// ClientID is the client ID for authentication.
	ClientID string `json:"client_id,omitempty"`

	// ClientSecret is the client secret for service principal authentication.
	ClientSecret string `json:"client_secret,omitempty"`

	// UseManagedIdentity uses managed identity for authentication.
	UseManagedIdentity bool `json:"use_managed_identity,omitempty"`

	// UseHSM indicates if HSM-backed keys should be used.
	UseHSM bool `json:"use_hsm,omitempty"`

	// DefaultKeyName is the default key name to use.
	DefaultKeyName string `json:"default_key_name,omitempty"`

	// DefaultKeyVersion is the default key version to use (empty = latest).
	DefaultKeyVersion string `json:"default_key_version,omitempty"`
}

// DefaultAzureConfig returns default Azure Key Vault configuration.
func DefaultAzureConfig() *AzureConfig {
	return &AzureConfig{
		ProviderConfig: ProviderConfig{
			Name:       "azure-keyvault",
			Type:       ProviderTypeAzure,
			Timeout:    30 * time.Second,
			MaxRetries: 3,
		},
		UseHSM: true,
	}
}

// AzureProvider implements the KMS Provider interface for Azure Key Vault.
type AzureProvider struct {
	config     *AzureConfig
	client     *azkeys.Client
	credential azcore.TokenCredential
	closed     atomic.Bool
}

// NewAzureProvider creates a new Azure Key Vault provider.
func NewAzureProvider(ctx context.Context, cfg *AzureConfig) (*AzureProvider, error) {
	if cfg == nil {
		return nil, errors.New("config is required")
	}

	if cfg.VaultURL == "" {
		return nil, errors.New("vault_url is required")
	}

	// Ensure vault URL doesn't have trailing slash
	cfg.VaultURL = strings.TrimSuffix(cfg.VaultURL, "/")

	// Create credential
	cred, err := createAzureCredential(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create credential: %w", err)
	}

	// Build client options
	clientOpts := &azkeys.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Retry: policy.RetryOptions{
				MaxRetries: int32(cfg.MaxRetries),
			},
		},
	}

	// Create key client
	client, err := azkeys.NewClient(cfg.VaultURL, cred, clientOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create key client: %w", err)
	}

	return &AzureProvider{
		config:     cfg,
		client:     client,
		credential: cred,
	}, nil
}

// createAzureCredential creates an Azure credential based on configuration.
func createAzureCredential(cfg *AzureConfig) (azcore.TokenCredential, error) {
	if cfg.UseManagedIdentity {
		opts := &azidentity.ManagedIdentityCredentialOptions{}
		if cfg.ClientID != "" {
			opts.ID = azidentity.ClientID(cfg.ClientID)
		}
		return azidentity.NewManagedIdentityCredential(opts)
	}

	if cfg.ClientID != "" && cfg.ClientSecret != "" && cfg.TenantID != "" {
		return azidentity.NewClientSecretCredential(
			cfg.TenantID,
			cfg.ClientID,
			cfg.ClientSecret,
			nil,
		)
	}

	// Use default credential chain
	return azidentity.NewDefaultAzureCredential(nil)
}

// Type returns the provider type.
func (p *AzureProvider) Type() ProviderType {
	return ProviderTypeAzure
}

// Name returns the provider instance name.
func (p *AzureProvider) Name() string {
	return p.config.Name
}

// Healthy checks if the provider is healthy.
func (p *AzureProvider) Healthy(ctx context.Context) bool {
	if p.closed.Load() {
		return false
	}

	// Try to list keys to verify connectivity
	pager := p.client.NewListKeyPropertiesPager(nil)
	_, err := pager.NextPage(ctx)
	// Empty vault returns no error
	return err == nil
}

// GetKeyMetadata retrieves metadata for a key.
func (p *AzureProvider) GetKeyMetadata(ctx context.Context, keyID string) (*KeyMetadata, error) {
	if p.closed.Load() {
		return nil, ErrProviderUnavailable
	}

	result, err := p.client.GetKey(ctx, keyID, "", nil)
	if err != nil {
		return nil, translateAzureError(err)
	}

	meta := &KeyMetadata{
		KeyID:    keyID,
		ARN:      string(*result.Key.KID),
		Provider: ProviderTypeAzure,
		Enabled:  result.Attributes != nil && *result.Attributes.Enabled,
		Tags:     make(map[string]string),
	}

	if result.Attributes != nil && result.Attributes.Created != nil {
		meta.CreatedAt = *result.Attributes.Created
	}

	// Determine key type and spec from Azure key type
	if result.Key != nil && result.Key.Kty != nil {
		kty := *result.Key.Kty
		switch kty {
		case azkeys.KeyTypeRSA, azkeys.KeyTypeRSAHSM:
			meta.KeyType = KeyTypeAsymmetric
			if result.Key.N != nil {
				keySize := len(result.Key.N) * 8
				if keySize >= 4096 {
					meta.KeySpec = KeySpecRSA4096
				} else {
					meta.KeySpec = KeySpecRSA2048
				}
			}
		case azkeys.KeyTypeEC, azkeys.KeyTypeECHSM:
			meta.KeyType = KeyTypeAsymmetric
			if result.Key.Crv != nil {
				switch *result.Key.Crv {
				case azkeys.CurveNameP256:
					meta.KeySpec = KeySpecECCNISTP256
				case azkeys.CurveNameP384:
					meta.KeySpec = KeySpecECCNISTP384
				}
			}
		case azkeys.KeyTypeOct, azkeys.KeyTypeOctHSM:
			meta.KeyType = KeyTypeSymmetric
			meta.KeySpec = KeySpecAES256
		}

		// Determine key usage from operations
		if result.Key.KeyOps != nil {
			for _, opPtr := range result.Key.KeyOps {
				if opPtr == nil {
					continue
				}
				op := *opPtr
				switch op {
				case azkeys.KeyOperationEncrypt, azkeys.KeyOperationDecrypt, azkeys.KeyOperationWrapKey, azkeys.KeyOperationUnwrapKey:
					meta.KeyUsage = KeyUsageEncryptDecrypt
				case azkeys.KeyOperationSign, azkeys.KeyOperationVerify:
					meta.KeyUsage = KeyUsageSignVerify
				}
			}
		}
	}

	// Copy tags
	for k, v := range result.Tags {
		if v != nil {
			meta.Tags[k] = *v
		}
	}

	return meta, nil
}

// Encrypt encrypts plaintext data.
func (p *AzureProvider) Encrypt(ctx context.Context, req *EncryptRequest) (*EncryptResponse, error) {
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

	algorithm := azkeys.EncryptionAlgorithmA256GCM
	params := azkeys.KeyOperationParameters{
		Algorithm: &algorithm,
		Value:     req.Plaintext,
	}

	// Generate IV for AES-GCM
	iv := make([]byte, 12)
	if _, err := rand.Read(iv); err != nil {
		return nil, fmt.Errorf("failed to generate IV: %w", err)
	}
	params.IV = iv

	// Add AAD if context provided
	if len(req.Context) > 0 {
		aad := encodeContext(req.Context)
		params.AdditionalAuthenticatedData = aad
	}

	result, err := p.client.Encrypt(ctx, keyName, p.config.DefaultKeyVersion, params, nil)
	if err != nil {
		return nil, translateAzureError(err)
	}

	// Combine IV, authentication tag, and ciphertext for storage
	ciphertext := append(append(iv, result.AuthenticationTag...), result.Result...)

	return &EncryptResponse{
		Ciphertext: ciphertext,
		KeyID:      keyName,
	}, nil
}

// Decrypt decrypts ciphertext data.
func (p *AzureProvider) Decrypt(ctx context.Context, req *DecryptRequest) (*DecryptResponse, error) {
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

	// Extract IV, auth tag, and ciphertext
	if len(req.Ciphertext) < 28 { // 12 byte IV + 16 byte tag minimum
		return nil, ErrInvalidCiphertext
	}

	iv := req.Ciphertext[:12]
	authTag := req.Ciphertext[12:28]
	ciphertext := req.Ciphertext[28:]

	algorithm := azkeys.EncryptionAlgorithmA256GCM
	params := azkeys.KeyOperationParameters{
		Algorithm:         &algorithm,
		Value:             ciphertext,
		IV:                iv,
		AuthenticationTag: authTag,
	}

	// Add AAD if context provided
	if len(req.Context) > 0 {
		aad := encodeContext(req.Context)
		params.AdditionalAuthenticatedData = aad
	}

	result, err := p.client.Decrypt(ctx, keyName, p.config.DefaultKeyVersion, params, nil)
	if err != nil {
		return nil, translateAzureError(err)
	}

	return &DecryptResponse{
		Plaintext: result.Result,
		KeyID:     keyName,
	}, nil
}

// GenerateDataKey generates a data encryption key.
func (p *AzureProvider) GenerateDataKey(ctx context.Context, req *GenerateDataKeyRequest) (*DataKey, error) {
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

	// Generate random data key
	plaintext := make([]byte, keySize)
	if _, err := rand.Read(plaintext); err != nil {
		return nil, fmt.Errorf("failed to generate data key: %w", err)
	}

	// Wrap the data key
	wrapResp, err := p.WrapKey(ctx, &WrapKeyRequest{
		WrapperKeyID: keyName,
		KeyToWrap:    plaintext,
		Context:      req.Context,
	})
	if err != nil {
		return nil, err
	}

	dataKey := &DataKey{
		Ciphertext:  wrapResp.WrappedKey,
		KeyID:       keyName,
		Provider:    ProviderTypeAzure,
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
func (p *AzureProvider) WrapKey(ctx context.Context, req *WrapKeyRequest) (*WrapKeyResponse, error) {
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

	algorithm := azkeys.EncryptionAlgorithmA256KW
	params := azkeys.KeyOperationParameters{
		Algorithm: &algorithm,
		Value:     req.KeyToWrap,
	}

	result, err := p.client.WrapKey(ctx, keyName, p.config.DefaultKeyVersion, params, nil)
	if err != nil {
		return nil, translateAzureError(err)
	}

	return &WrapKeyResponse{
		WrappedKey:   result.Result,
		WrapperKeyID: keyName,
	}, nil
}

// UnwrapKey unwraps (decrypts) a key with the KMS key.
func (p *AzureProvider) UnwrapKey(ctx context.Context, req *UnwrapKeyRequest) (*UnwrapKeyResponse, error) {
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

	algorithm := azkeys.EncryptionAlgorithmA256KW
	params := azkeys.KeyOperationParameters{
		Algorithm: &algorithm,
		Value:     req.WrappedKey,
	}

	result, err := p.client.UnwrapKey(ctx, keyName, p.config.DefaultKeyVersion, params, nil)
	if err != nil {
		return nil, translateAzureError(err)
	}

	return &UnwrapKeyResponse{
		PlaintextKey: result.Result,
		WrapperKeyID: keyName,
	}, nil
}

// Sign signs data with the KMS key.
func (p *AzureProvider) Sign(ctx context.Context, req *SignRequest) (*SignResponse, error) {
	if p.closed.Load() {
		return nil, ErrProviderUnavailable
	}

	algorithm := mapSigningAlgorithm(req.Algorithm)
	params := azkeys.SignParameters{
		Algorithm: &algorithm,
		Value:     req.Message,
	}

	result, err := p.client.Sign(ctx, req.KeyID, p.config.DefaultKeyVersion, params, nil)
	if err != nil {
		return nil, translateAzureError(err)
	}

	return &SignResponse{
		Signature: result.Result,
		KeyID:     req.KeyID,
		Algorithm: req.Algorithm,
	}, nil
}

// Verify verifies a signature with the KMS key.
func (p *AzureProvider) Verify(ctx context.Context, req *VerifyRequest) (*VerifyResponse, error) {
	if p.closed.Load() {
		return nil, ErrProviderUnavailable
	}

	algorithm := mapSigningAlgorithm(req.Algorithm)
	params := azkeys.VerifyParameters{
		Algorithm: &algorithm,
		Digest:    req.Message,
		Signature: req.Signature,
	}

	result, err := p.client.Verify(ctx, req.KeyID, p.config.DefaultKeyVersion, params, nil)
	if err != nil {
		return nil, translateAzureError(err)
	}

	return &VerifyResponse{
		Valid: *result.Value,
		KeyID: req.KeyID,
	}, nil
}

// EnableKeyRotation enables automatic key rotation.
func (p *AzureProvider) EnableKeyRotation(ctx context.Context, keyID string) error {
	if p.closed.Load() {
		return ErrProviderUnavailable
	}

	// Azure Key Vault uses rotation policies
	// For now, return unsupported as this requires more complex policy setup
	return ErrUnsupportedOperation
}

// DisableKeyRotation disables automatic key rotation.
func (p *AzureProvider) DisableKeyRotation(ctx context.Context, keyID string) error {
	if p.closed.Load() {
		return ErrProviderUnavailable
	}

	return ErrUnsupportedOperation
}

// RotateKey manually rotates a key.
func (p *AzureProvider) RotateKey(ctx context.Context, keyID string) error {
	if p.closed.Load() {
		return ErrProviderUnavailable
	}

	_, err := p.client.RotateKey(ctx, keyID, nil)
	return translateAzureError(err)
}

// ListKeyVersions lists all versions of a key.
func (p *AzureProvider) ListKeyVersions(ctx context.Context, keyID string) ([]KeyVersion, error) {
	if p.closed.Load() {
		return nil, ErrProviderUnavailable
	}

	var versions []KeyVersion

	pager := p.client.NewListKeyPropertiesVersionsPager(keyID, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, translateAzureError(err)
		}

		for _, item := range page.Value {
			version := KeyVersion{
				Version: extractVersionFromKID(*item.KID),
			}

			if item.Attributes != nil {
				if item.Attributes.Created != nil {
					version.CreatedAt = *item.Attributes.Created
				}
				if item.Attributes.Enabled != nil && *item.Attributes.Enabled {
					version.State = "enabled"
				} else {
					version.State = "disabled"
				}
			}

			versions = append(versions, version)
		}
	}

	// Mark the first version as primary (latest)
	if len(versions) > 0 {
		versions[0].Primary = true
	}

	return versions, nil
}

// Close closes the provider.
func (p *AzureProvider) Close() error {
	p.closed.Store(true)
	return nil
}

// extractVersionFromKID extracts the version from a Key ID URL.
func extractVersionFromKID(kid azkeys.ID) string {
	s := string(kid)
	parts := strings.Split(s, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

// encodeContext encodes encryption context as AAD bytes.
func encodeContext(ctx map[string]string) []byte {
	var parts []string
	for k, v := range ctx {
		parts = append(parts, k+"="+v)
	}
	return []byte(strings.Join(parts, ";"))
}

// mapSigningAlgorithm maps algorithm string to Azure signing algorithm.
func mapSigningAlgorithm(algorithm string) azkeys.SignatureAlgorithm {
	switch algorithm {
	case "RS256", "RSASSA_PKCS1_V1_5_SHA_256":
		return azkeys.SignatureAlgorithmRS256
	case "RS384", "RSASSA_PKCS1_V1_5_SHA_384":
		return azkeys.SignatureAlgorithmRS384
	case "RS512", "RSASSA_PKCS1_V1_5_SHA_512":
		return azkeys.SignatureAlgorithmRS512
	case "PS256", "RSASSA_PSS_SHA_256":
		return azkeys.SignatureAlgorithmPS256
	case "PS384", "RSASSA_PSS_SHA_384":
		return azkeys.SignatureAlgorithmPS384
	case "PS512", "RSASSA_PSS_SHA_512":
		return azkeys.SignatureAlgorithmPS512
	case "ES256", "ECDSA_SHA_256":
		return azkeys.SignatureAlgorithmES256
	case "ES384", "ECDSA_SHA_384":
		return azkeys.SignatureAlgorithmES384
	case "ES512", "ECDSA_SHA_512":
		return azkeys.SignatureAlgorithmES512
	default:
		return azkeys.SignatureAlgorithmRS256
	}
}

// translateAzureError translates Azure errors to KMS package errors.
func translateAzureError(err error) error {
	if err == nil {
		return nil
	}

	errMsg := err.Error()

	switch {
	case strings.Contains(errMsg, "KeyNotFound"):
		return ErrKeyNotFound
	case strings.Contains(errMsg, "Forbidden"):
		return ErrAccessDenied
	case strings.Contains(errMsg, "Unauthorized"):
		return ErrAccessDenied
	case strings.Contains(errMsg, "BadParameter"):
		return ErrInvalidKey
	case strings.Contains(errMsg, "KeyDisabled"):
		return ErrKeyDisabled
	case strings.Contains(errMsg, "InvalidParameter"):
		return ErrInvalidCiphertext
	default:
		return fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
	}
}

// GenerateRandomBytes generates random bytes using Azure Key Vault.
func (p *AzureProvider) GenerateRandomBytes(ctx context.Context, count int) ([]byte, error) {
	if p.closed.Load() {
		return nil, ErrProviderUnavailable
	}

	result, err := p.client.GetRandomBytes(ctx, azkeys.GetRandomBytesParameters{
		Count: int32Ptr(int32(count)),
	}, nil)
	if err != nil {
		return nil, translateAzureError(err)
	}

	// Decode base64 result
	decoded, err := base64.RawURLEncoding.DecodeString(string(result.Value))
	if err != nil {
		return result.Value, nil // Return raw if decoding fails
	}

	return decoded, nil
}

func int32Ptr(i int32) *int32 {
	return &i
}

// Ensure AzureProvider implements interfaces.
var (
	_ Provider         = (*AzureProvider)(nil)
	_ SigningProvider  = (*AzureProvider)(nil)
	_ RotatingProvider = (*AzureProvider)(nil)
)
