package kms

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// AWSConfig configures the AWS KMS provider.
type AWSConfig struct {
	ProviderConfig

	// Region is the AWS region.
	Region string `json:"region,omitempty"`

	// AccessKeyID is the AWS access key ID (for static credentials).
	AccessKeyID string `json:"access_key_id,omitempty"`

	// SecretAccessKey is the AWS secret access key (for static credentials).
	SecretAccessKey string `json:"secret_access_key,omitempty"`

	// SessionToken is an optional session token (for temporary credentials).
	SessionToken string `json:"session_token,omitempty"`

	// AssumeRoleARN is the ARN of the role to assume.
	AssumeRoleARN string `json:"assume_role_arn,omitempty"`

	// ExternalID is the external ID for role assumption.
	ExternalID string `json:"external_id,omitempty"`

	// RoleSessionName is the session name for role assumption.
	RoleSessionName string `json:"role_session_name,omitempty"`

	// Endpoint is a custom endpoint URL (for testing with LocalStack).
	Endpoint string `json:"endpoint,omitempty"`

	// DefaultKeyID is the default KMS key to use.
	DefaultKeyID string `json:"default_key_id,omitempty"`
}

// DefaultAWSConfig returns default AWS KMS configuration.
func DefaultAWSConfig() *AWSConfig {
	return &AWSConfig{
		ProviderConfig: ProviderConfig{
			Name:       "aws-kms",
			Type:       ProviderTypeAWS,
			Timeout:    30 * time.Second,
			MaxRetries: 3,
		},
	}
}

// AWSProvider implements the KMS Provider interface for AWS KMS.
type AWSProvider struct {
	config *AWSConfig
	client *kms.Client
	closed atomic.Bool
}

// NewAWSProvider creates a new AWS KMS provider.
func NewAWSProvider(ctx context.Context, cfg *AWSConfig) (*AWSProvider, error) {
	if cfg == nil {
		cfg = DefaultAWSConfig()
	}

	awsCfg, err := buildAWSConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to build AWS config: %w", err)
	}

	client := kms.NewFromConfig(awsCfg, func(o *kms.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
	})

	return &AWSProvider{
		config: cfg,
		client: client,
	}, nil
}

// buildAWSConfig builds the AWS SDK configuration.
func buildAWSConfig(ctx context.Context, cfg *AWSConfig) (aws.Config, error) {
	var opts []func(*config.LoadOptions) error

	if cfg.Region != "" {
		opts = append(opts, config.WithRegion(cfg.Region))
	}

	if cfg.MaxRetries > 0 {
		opts = append(opts, config.WithRetryMaxAttempts(cfg.MaxRetries))
	}

	// Handle static credentials
	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		creds := credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID,
			cfg.SecretAccessKey,
			cfg.SessionToken,
		)
		opts = append(opts, config.WithCredentialsProvider(creds))
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return aws.Config{}, err
	}

	// Handle role assumption
	if cfg.AssumeRoleARN != "" {
		stsClient := sts.NewFromConfig(awsCfg)
		assumeRoleOpts := []func(*stscreds.AssumeRoleOptions){}

		if cfg.ExternalID != "" {
			assumeRoleOpts = append(assumeRoleOpts, func(o *stscreds.AssumeRoleOptions) {
				o.ExternalID = aws.String(cfg.ExternalID)
			})
		}
		if cfg.RoleSessionName != "" {
			assumeRoleOpts = append(assumeRoleOpts, func(o *stscreds.AssumeRoleOptions) {
				o.RoleSessionName = cfg.RoleSessionName
			})
		}

		creds := stscreds.NewAssumeRoleProvider(stsClient, cfg.AssumeRoleARN, assumeRoleOpts...)
		awsCfg.Credentials = aws.NewCredentialsCache(creds)
	}

	return awsCfg, nil
}

// Type returns the provider type.
func (p *AWSProvider) Type() ProviderType {
	return ProviderTypeAWS
}

// Name returns the provider instance name.
func (p *AWSProvider) Name() string {
	return p.config.Name
}

// Healthy checks if the provider is healthy.
func (p *AWSProvider) Healthy(ctx context.Context) bool {
	if p.closed.Load() {
		return false
	}

	// Try to list keys to verify connectivity
	_, err := p.client.ListKeys(ctx, &kms.ListKeysInput{
		Limit: aws.Int32(1),
	})
	return err == nil
}

// GetKeyMetadata retrieves metadata for a key.
func (p *AWSProvider) GetKeyMetadata(ctx context.Context, keyID string) (*KeyMetadata, error) {
	if p.closed.Load() {
		return nil, ErrProviderUnavailable
	}

	result, err := p.client.DescribeKey(ctx, &kms.DescribeKeyInput{
		KeyId: aws.String(keyID),
	})
	if err != nil {
		return nil, translateAWSError(err)
	}

	meta := &KeyMetadata{
		KeyID:       aws.ToString(result.KeyMetadata.KeyId),
		ARN:         aws.ToString(result.KeyMetadata.Arn),
		Provider:    ProviderTypeAWS,
		Enabled:     result.KeyMetadata.Enabled,
		Description: aws.ToString(result.KeyMetadata.Description),
		CreatedAt:   aws.ToTime(result.KeyMetadata.CreationDate),
		Tags:        make(map[string]string),
	}

	// Map key spec
	switch result.KeyMetadata.KeySpec {
	case types.KeySpecSymmetricDefault:
		meta.KeySpec = KeySpecSymmetricDefault
		meta.KeyType = KeyTypeSymmetric
	case types.KeySpecRsa2048:
		meta.KeySpec = KeySpecRSA2048
		meta.KeyType = KeyTypeAsymmetric
	case types.KeySpecRsa4096:
		meta.KeySpec = KeySpecRSA4096
		meta.KeyType = KeyTypeAsymmetric
	case types.KeySpecEccNistP256:
		meta.KeySpec = KeySpecECCNISTP256
		meta.KeyType = KeyTypeAsymmetric
	case types.KeySpecEccNistP384:
		meta.KeySpec = KeySpecECCNISTP384
		meta.KeyType = KeyTypeAsymmetric
	}

	// Map key usage
	switch result.KeyMetadata.KeyUsage {
	case types.KeyUsageTypeEncryptDecrypt:
		meta.KeyUsage = KeyUsageEncryptDecrypt
	case types.KeyUsageTypeSignVerify:
		meta.KeyUsage = KeyUsageSignVerify
	}

	// Get key aliases
	aliasesResult, err := p.client.ListAliases(ctx, &kms.ListAliasesInput{
		KeyId: aws.String(keyID),
	})
	if err == nil && len(aliasesResult.Aliases) > 0 {
		meta.Alias = aws.ToString(aliasesResult.Aliases[0].AliasName)
	}

	// Get key rotation status
	rotationResult, err := p.client.GetKeyRotationStatus(ctx, &kms.GetKeyRotationStatusInput{
		KeyId: aws.String(keyID),
	})
	if err == nil {
		meta.RotationEnabled = rotationResult.KeyRotationEnabled
		if rotationResult.NextRotationDate != nil {
			meta.NextRotationDate = aws.ToTime(rotationResult.NextRotationDate)
		}
	}

	// Get tags
	tagsResult, err := p.client.ListResourceTags(ctx, &kms.ListResourceTagsInput{
		KeyId: aws.String(keyID),
	})
	if err == nil {
		for _, tag := range tagsResult.Tags {
			meta.Tags[aws.ToString(tag.TagKey)] = aws.ToString(tag.TagValue)
		}
	}

	return meta, nil
}

// Encrypt encrypts plaintext data.
func (p *AWSProvider) Encrypt(ctx context.Context, req *EncryptRequest) (*EncryptResponse, error) {
	if p.closed.Load() {
		return nil, ErrProviderUnavailable
	}

	keyID := req.KeyID
	if keyID == "" {
		keyID = p.config.DefaultKeyID
	}
	if keyID == "" {
		return nil, ErrInvalidKey
	}

	input := &kms.EncryptInput{
		KeyId:     aws.String(keyID),
		Plaintext: req.Plaintext,
	}

	if len(req.Context) > 0 {
		input.EncryptionContext = req.Context
	}

	result, err := p.client.Encrypt(ctx, input)
	if err != nil {
		return nil, translateAWSError(err)
	}

	return &EncryptResponse{
		Ciphertext: result.CiphertextBlob,
		KeyID:      aws.ToString(result.KeyId),
	}, nil
}

// Decrypt decrypts ciphertext data.
func (p *AWSProvider) Decrypt(ctx context.Context, req *DecryptRequest) (*DecryptResponse, error) {
	if p.closed.Load() {
		return nil, ErrProviderUnavailable
	}

	input := &kms.DecryptInput{
		CiphertextBlob: req.Ciphertext,
	}

	if req.KeyID != "" {
		input.KeyId = aws.String(req.KeyID)
	}

	if len(req.Context) > 0 {
		input.EncryptionContext = req.Context
	}

	result, err := p.client.Decrypt(ctx, input)
	if err != nil {
		return nil, translateAWSError(err)
	}

	return &DecryptResponse{
		Plaintext: result.Plaintext,
		KeyID:     aws.ToString(result.KeyId),
	}, nil
}

// GenerateDataKey generates a data encryption key.
func (p *AWSProvider) GenerateDataKey(ctx context.Context, req *GenerateDataKeyRequest) (*DataKey, error) {
	if p.closed.Load() {
		return nil, ErrProviderUnavailable
	}

	keyID := req.KeyID
	if keyID == "" {
		keyID = p.config.DefaultKeyID
	}
	if keyID == "" {
		return nil, ErrInvalidKey
	}

	if req.WithoutPlaintext {
		input := &kms.GenerateDataKeyWithoutPlaintextInput{
			KeyId: aws.String(keyID),
		}

		if req.NumberOfBytes > 0 {
			input.NumberOfBytes = aws.Int32(int32(req.NumberOfBytes))
		} else {
			input.KeySpec = mapKeySpec(req.KeySpec)
		}

		if len(req.Context) > 0 {
			input.EncryptionContext = req.Context
		}

		result, err := p.client.GenerateDataKeyWithoutPlaintext(ctx, input)
		if err != nil {
			return nil, translateAWSError(err)
		}

		return &DataKey{
			Ciphertext:  result.CiphertextBlob,
			KeyID:       aws.ToString(result.KeyId),
			Provider:    ProviderTypeAWS,
			KeySpec:     req.KeySpec,
			GeneratedAt: time.Now(),
		}, nil
	}

	input := &kms.GenerateDataKeyInput{
		KeyId: aws.String(keyID),
	}

	if req.NumberOfBytes > 0 {
		input.NumberOfBytes = aws.Int32(int32(req.NumberOfBytes))
	} else {
		input.KeySpec = mapKeySpec(req.KeySpec)
	}

	if len(req.Context) > 0 {
		input.EncryptionContext = req.Context
	}

	result, err := p.client.GenerateDataKey(ctx, input)
	if err != nil {
		return nil, translateAWSError(err)
	}

	return &DataKey{
		Plaintext:   result.Plaintext,
		Ciphertext:  result.CiphertextBlob,
		KeyID:       aws.ToString(result.KeyId),
		Provider:    ProviderTypeAWS,
		KeySpec:     req.KeySpec,
		GeneratedAt: time.Now(),
	}, nil
}

// WrapKey wraps (encrypts) a key with the KMS key.
func (p *AWSProvider) WrapKey(ctx context.Context, req *WrapKeyRequest) (*WrapKeyResponse, error) {
	if p.closed.Load() {
		return nil, ErrProviderUnavailable
	}

	keyID := req.WrapperKeyID
	if keyID == "" {
		keyID = p.config.DefaultKeyID
	}
	if keyID == "" {
		return nil, ErrInvalidKey
	}

	input := &kms.EncryptInput{
		KeyId:     aws.String(keyID),
		Plaintext: req.KeyToWrap,
	}

	if len(req.Context) > 0 {
		input.EncryptionContext = req.Context
	}

	result, err := p.client.Encrypt(ctx, input)
	if err != nil {
		return nil, translateAWSError(err)
	}

	return &WrapKeyResponse{
		WrappedKey:   result.CiphertextBlob,
		WrapperKeyID: aws.ToString(result.KeyId),
	}, nil
}

// UnwrapKey unwraps (decrypts) a key with the KMS key.
func (p *AWSProvider) UnwrapKey(ctx context.Context, req *UnwrapKeyRequest) (*UnwrapKeyResponse, error) {
	if p.closed.Load() {
		return nil, ErrProviderUnavailable
	}

	input := &kms.DecryptInput{
		CiphertextBlob: req.WrappedKey,
	}

	if req.WrapperKeyID != "" {
		input.KeyId = aws.String(req.WrapperKeyID)
	}

	if len(req.Context) > 0 {
		input.EncryptionContext = req.Context
	}

	result, err := p.client.Decrypt(ctx, input)
	if err != nil {
		return nil, translateAWSError(err)
	}

	return &UnwrapKeyResponse{
		PlaintextKey: result.Plaintext,
		WrapperKeyID: aws.ToString(result.KeyId),
	}, nil
}

// Sign signs data with the KMS key.
func (p *AWSProvider) Sign(ctx context.Context, req *SignRequest) (*SignResponse, error) {
	if p.closed.Load() {
		return nil, ErrProviderUnavailable
	}

	input := &kms.SignInput{
		KeyId:   aws.String(req.KeyID),
		Message: req.Message,
	}

	// Set message type
	if req.MessageType == "DIGEST" {
		input.MessageType = types.MessageTypeDigest
	} else {
		input.MessageType = types.MessageTypeRaw
	}

	// Set signing algorithm
	if req.Algorithm != "" {
		input.SigningAlgorithm = types.SigningAlgorithmSpec(req.Algorithm)
	}

	result, err := p.client.Sign(ctx, input)
	if err != nil {
		return nil, translateAWSError(err)
	}

	return &SignResponse{
		Signature: result.Signature,
		KeyID:     aws.ToString(result.KeyId),
		Algorithm: string(result.SigningAlgorithm),
	}, nil
}

// Verify verifies a signature with the KMS key.
func (p *AWSProvider) Verify(ctx context.Context, req *VerifyRequest) (*VerifyResponse, error) {
	if p.closed.Load() {
		return nil, ErrProviderUnavailable
	}

	input := &kms.VerifyInput{
		KeyId:     aws.String(req.KeyID),
		Message:   req.Message,
		Signature: req.Signature,
	}

	// Set message type
	if req.MessageType == "DIGEST" {
		input.MessageType = types.MessageTypeDigest
	} else {
		input.MessageType = types.MessageTypeRaw
	}

	// Set signing algorithm
	if req.Algorithm != "" {
		input.SigningAlgorithm = types.SigningAlgorithmSpec(req.Algorithm)
	}

	result, err := p.client.Verify(ctx, input)
	if err != nil {
		return nil, translateAWSError(err)
	}

	return &VerifyResponse{
		Valid: result.SignatureValid,
		KeyID: aws.ToString(result.KeyId),
	}, nil
}

// EnableKeyRotation enables automatic key rotation.
func (p *AWSProvider) EnableKeyRotation(ctx context.Context, keyID string) error {
	if p.closed.Load() {
		return ErrProviderUnavailable
	}

	_, err := p.client.EnableKeyRotation(ctx, &kms.EnableKeyRotationInput{
		KeyId: aws.String(keyID),
	})
	return translateAWSError(err)
}

// DisableKeyRotation disables automatic key rotation.
func (p *AWSProvider) DisableKeyRotation(ctx context.Context, keyID string) error {
	if p.closed.Load() {
		return ErrProviderUnavailable
	}

	_, err := p.client.DisableKeyRotation(ctx, &kms.DisableKeyRotationInput{
		KeyId: aws.String(keyID),
	})
	return translateAWSError(err)
}

// RotateKey manually rotates a key (creates new key material).
func (p *AWSProvider) RotateKey(ctx context.Context, keyID string) error {
	if p.closed.Load() {
		return ErrProviderUnavailable
	}

	_, err := p.client.RotateKeyOnDemand(ctx, &kms.RotateKeyOnDemandInput{
		KeyId: aws.String(keyID),
	})
	return translateAWSError(err)
}

// ListKeyVersions lists all versions of a key.
func (p *AWSProvider) ListKeyVersions(ctx context.Context, keyID string) ([]KeyVersion, error) {
	if p.closed.Load() {
		return nil, ErrProviderUnavailable
	}

	// AWS KMS doesn't have direct version listing for symmetric keys.
	// For asymmetric keys, we can get the current key material.
	meta, err := p.GetKeyMetadata(ctx, keyID)
	if err != nil {
		return nil, err
	}

	// Return a single version for symmetric keys
	versions := []KeyVersion{
		{
			Version:   "current",
			CreatedAt: meta.CreatedAt,
			State:     "enabled",
			Primary:   true,
		},
	}

	return versions, nil
}

// Close closes the provider.
func (p *AWSProvider) Close() error {
	p.closed.Store(true)
	return nil
}

// mapKeySpec maps our KeySpec to AWS DataKeySpec.
func mapKeySpec(spec KeySpec) types.DataKeySpec {
	switch spec {
	case KeySpecAES256:
		return types.DataKeySpecAes256
	default:
		return types.DataKeySpecAes256
	}
}

// translateAWSError translates AWS errors to KMS package errors.
func translateAWSError(err error) error {
	if err == nil {
		return nil
	}

	errMsg := err.Error()

	switch {
	case strings.Contains(errMsg, "NotFoundException"):
		return ErrKeyNotFound
	case strings.Contains(errMsg, "DisabledException"):
		return ErrKeyDisabled
	case strings.Contains(errMsg, "AccessDeniedException"):
		return ErrAccessDenied
	case strings.Contains(errMsg, "InvalidCiphertextException"):
		return ErrInvalidCiphertext
	case strings.Contains(errMsg, "InvalidKeyUsageException"):
		return ErrUnsupportedOperation
	case strings.Contains(errMsg, "KMSInvalidStateException"):
		return ErrKeyDisabled
	case strings.Contains(errMsg, "IncorrectKeyException"):
		return ErrInvalidKey
	case strings.Contains(errMsg, "InvalidGrantTokenException"):
		return ErrAccessDenied
	default:
		return fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
	}
}

// Ensure AWSProvider implements interfaces.
var (
	_ Provider         = (*AWSProvider)(nil)
	_ SigningProvider  = (*AWSProvider)(nil)
	_ RotatingProvider = (*AWSProvider)(nil)
)
