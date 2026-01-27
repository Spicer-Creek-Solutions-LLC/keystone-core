// Package aws provides an AWS Secrets Manager backend for the secrets broker.
package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/secrets"
)

// Backend implements the SecretBackend interface for AWS Secrets Manager.
type Backend struct {
	name   string
	client *Client
	config *BackendConfig
}

// BackendConfig configures the AWS Secrets Manager backend.
type BackendConfig struct {
	// ClientConfig is the client configuration.
	ClientConfig *ClientConfig `json:"client,omitempty"`

	// Name is the backend instance name.
	Name string `json:"name,omitempty"`

	// PathPrefix is the prefix to strip from secret paths.
	PathPrefix string `json:"path_prefix,omitempty"`

	// DefaultCacheTTL is the default TTL for caching retrieved secrets.
	DefaultCacheTTL time.Duration `json:"default_cache_ttl,omitempty"`

	// JSONKeys enables automatic JSON parsing and key extraction.
	JSONKeys bool `json:"json_keys,omitempty"`
}

// DefaultBackendConfig returns a backend configuration with sensible defaults.
func DefaultBackendConfig() *BackendConfig {
	return &BackendConfig{
		ClientConfig:    DefaultClientConfig(),
		Name:            "aws",
		PathPrefix:      "aws/",
		DefaultCacheTTL: 5 * time.Minute,
		JSONKeys:        true,
	}
}

// NewBackend creates a new AWS Secrets Manager backend.
func NewBackend(ctx context.Context, config *BackendConfig) (*Backend, error) {
	if config == nil {
		config = DefaultBackendConfig()
	}

	if config.ClientConfig == nil {
		config.ClientConfig = DefaultClientConfig()
	}

	client, err := NewClient(ctx, config.ClientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS client: %w", err)
	}

	name := config.Name
	if name == "" {
		name = "aws"
	}

	return &Backend{
		name:   name,
		client: client,
		config: config,
	}, nil
}

// Type returns the backend type.
func (b *Backend) Type() secrets.BackendType {
	return secrets.BackendTypeAWS
}

// Name returns the backend instance name.
func (b *Backend) Name() string {
	return b.name
}

// Healthy checks if the backend is healthy by describing a non-existent secret.
func (b *Backend) Healthy(ctx context.Context) bool {
	// Try to list secrets with a limit of 1 to verify connectivity
	_, err := b.client.ListSecrets(ctx, WithMaxResults(1))
	return err == nil
}

// Read reads a secret from AWS Secrets Manager.
func (b *Backend) Read(ctx context.Context, req *secrets.SecretRequest) (*secrets.Secret, error) {
	if req == nil {
		return nil, secrets.ErrInvalidPath
	}

	// Resolve the secret name from the path
	secretName := b.resolveSecretName(req.Path)

	// Build get options
	var opts []GetSecretOption
	if req.Version > 0 {
		// AWS uses string version IDs, but we'll check version stages first
		// If a specific version number is requested, we'll need the version ID
		// For now, we'll handle AWSCURRENT/AWSPREVIOUS based on version
		if req.Version == 1 {
			opts = append(opts, WithVersionStage("AWSCURRENT"))
		}
	}

	// Check for version stage in context
	if stage, ok := req.Context["version_stage"]; ok {
		opts = append(opts, WithVersionStage(stage))
	}
	if versionID, ok := req.Context["version_id"]; ok {
		opts = append(opts, WithVersionID(versionID))
	}

	// Get the secret
	value, err := b.client.GetSecret(ctx, secretName, opts...)
	if err != nil {
		return nil, err
	}

	// Convert to secrets.Secret
	secret := b.valueToSecret(req.Path, value)

	// Filter to requested keys if specified
	if len(req.Keys) > 0 && b.config.JSONKeys {
		filteredData := make(map[string]interface{})
		for _, key := range req.Keys {
			if v, ok := secret.Data[key]; ok {
				filteredData[key] = v
			}
		}
		secret.Data = filteredData
	}

	return secret, nil
}

// ReadDynamic reads or generates a dynamic secret.
// AWS Secrets Manager doesn't generate dynamic secrets like Vault does,
// but it can retrieve secrets that are rotated automatically.
func (b *Backend) ReadDynamic(ctx context.Context, req *secrets.SecretRequest) (*secrets.Secret, error) {
	// For AWS, dynamic secrets are just regular secrets with rotation enabled
	// We can check if the secret has rotation enabled and return appropriate metadata
	secret, err := b.Read(ctx, req)
	if err != nil {
		return nil, err
	}

	// Get metadata to check rotation status
	secretName := b.resolveSecretName(req.Path)
	metadata, err := b.client.DescribeSecret(ctx, secretName)
	if err == nil && metadata.RotationEnabled {
		// Add rotation information to the secret
		secret.Type = secrets.SecretTypeDynamic
		secret.Metadata["rotation_enabled"] = "true"
		if !metadata.LastRotatedDate.IsZero() {
			secret.Metadata["last_rotated"] = metadata.LastRotatedDate.Format(time.RFC3339)
		}
		if !metadata.NextRotationDate.IsZero() {
			secret.Metadata["next_rotation"] = metadata.NextRotationDate.Format(time.RFC3339)
			// Set expiration based on next rotation
			secret.ExpiresAt = metadata.NextRotationDate
		}
		if metadata.RotationRules != nil && metadata.RotationRules.AutomaticallyAfterDays > 0 {
			secret.Metadata["rotation_interval_days"] = fmt.Sprintf("%d", metadata.RotationRules.AutomaticallyAfterDays)
		}
	}

	return secret, nil
}

// List lists secrets under a path prefix.
func (b *Backend) List(ctx context.Context, prefix string) ([]string, error) {
	// Resolve the prefix
	secretPrefix := b.resolveSecretName(prefix)

	// List secrets with the prefix filter
	entries, err := b.client.ListSecrets(ctx, WithNamePrefix(secretPrefix))
	if err != nil {
		return nil, err
	}

	// Extract names and remove the prefix
	var names []string
	for _, entry := range entries {
		name := entry.Name
		// Remove the path prefix to return relative names
		if b.config.PathPrefix != "" && strings.HasPrefix(name, secretPrefix) {
			name = strings.TrimPrefix(name, secretPrefix)
		}
		names = append(names, name)
	}

	return names, nil
}

// RenewLease renews a lease.
// AWS Secrets Manager doesn't have a lease concept like Vault.
// Secrets are either rotated on a schedule or accessed on demand.
func (b *Backend) RenewLease(ctx context.Context, leaseID string, increment time.Duration) (*secrets.Lease, error) {
	// AWS doesn't have leases - secrets are either static or auto-rotated
	return nil, secrets.ErrLeaseNotFound
}

// RevokeLease revokes a lease.
// AWS Secrets Manager doesn't have a lease concept.
func (b *Backend) RevokeLease(ctx context.Context, leaseID string) error {
	// AWS doesn't have leases
	return secrets.ErrLeaseNotFound
}

// Close closes the backend.
func (b *Backend) Close() error {
	if b.client != nil {
		return b.client.Close()
	}
	return nil
}

// GetMetadata retrieves metadata about a secret.
func (b *Backend) GetMetadata(ctx context.Context, path string) (*SecretMetadata, error) {
	secretName := b.resolveSecretName(path)
	return b.client.DescribeSecret(ctx, secretName)
}

// ListVersions lists all versions of a secret.
func (b *Backend) ListVersions(ctx context.Context, path string) ([]*SecretVersionInfo, error) {
	secretName := b.resolveSecretName(path)
	return b.client.ListSecretVersions(ctx, secretName)
}

// GetVersion retrieves a specific version of a secret.
func (b *Backend) GetVersion(ctx context.Context, path, versionID string) (*secrets.Secret, error) {
	secretName := b.resolveSecretName(path)

	value, err := b.client.GetSecret(ctx, secretName, WithVersionID(versionID))
	if err != nil {
		return nil, err
	}

	return b.valueToSecret(path, value), nil
}

// GetVersionByStage retrieves a secret version by staging label.
func (b *Backend) GetVersionByStage(ctx context.Context, path, stage string) (*secrets.Secret, error) {
	secretName := b.resolveSecretName(path)

	value, err := b.client.GetSecret(ctx, secretName, WithVersionStage(stage))
	if err != nil {
		return nil, err
	}

	return b.valueToSecret(path, value), nil
}

// GetCurrentVersion retrieves the current version (AWSCURRENT) of a secret.
func (b *Backend) GetCurrentVersion(ctx context.Context, path string) (*secrets.Secret, error) {
	return b.GetVersionByStage(ctx, path, "AWSCURRENT")
}

// GetPreviousVersion retrieves the previous version (AWSPREVIOUS) of a secret.
func (b *Backend) GetPreviousVersion(ctx context.Context, path string) (*secrets.Secret, error) {
	return b.GetVersionByStage(ctx, path, "AWSPREVIOUS")
}

// TriggerRotation triggers rotation for a secret.
func (b *Backend) TriggerRotation(ctx context.Context, path string, immediate bool) (*RotateSecretResult, error) {
	secretName := b.resolveSecretName(path)

	var opts []RotateSecretOption
	if immediate {
		opts = append(opts, WithRotateImmediately())
	}

	return b.client.RotateSecret(ctx, secretName, opts...)
}

// IsRotationEnabled checks if rotation is enabled for a secret.
func (b *Backend) IsRotationEnabled(ctx context.Context, path string) (bool, error) {
	metadata, err := b.GetMetadata(ctx, path)
	if err != nil {
		return false, err
	}
	return metadata.RotationEnabled, nil
}

// GetRotationStatus retrieves rotation status for a secret.
func (b *Backend) GetRotationStatus(ctx context.Context, path string) (*RotationStatus, error) {
	metadata, err := b.GetMetadata(ctx, path)
	if err != nil {
		return nil, err
	}

	status := &RotationStatus{
		Enabled: metadata.RotationEnabled,
	}

	if metadata.RotationEnabled {
		status.LambdaARN = metadata.RotationLambdaARN
		status.LastRotated = metadata.LastRotatedDate
		status.NextRotation = metadata.NextRotationDate
		if metadata.RotationRules != nil {
			status.AutoRotateDays = metadata.RotationRules.AutomaticallyAfterDays
			status.Schedule = metadata.RotationRules.ScheduleExpression
		}
	}

	return status, nil
}

// RotationStatus contains rotation status information.
type RotationStatus struct {
	// Enabled indicates if rotation is enabled.
	Enabled bool `json:"enabled"`

	// LambdaARN is the ARN of the rotation Lambda function.
	LambdaARN string `json:"lambda_arn,omitempty"`

	// LastRotated is when the secret was last rotated.
	LastRotated time.Time `json:"last_rotated,omitempty"`

	// NextRotation is when the next rotation is scheduled.
	NextRotation time.Time `json:"next_rotation,omitempty"`

	// AutoRotateDays is the number of days between rotations.
	AutoRotateDays int64 `json:"auto_rotate_days,omitempty"`

	// Schedule is the cron expression for rotation.
	Schedule string `json:"schedule,omitempty"`
}

// resolveSecretName resolves the AWS secret name from a path.
func (b *Backend) resolveSecretName(path string) string {
	// Remove the path prefix if present
	name := path
	if b.config.PathPrefix != "" && strings.HasPrefix(path, b.config.PathPrefix) {
		name = strings.TrimPrefix(path, b.config.PathPrefix)
	}
	// AWS secret names can have slashes, but we should trim leading/trailing ones
	return strings.Trim(name, "/")
}

// valueToSecret converts an AWS SecretValue to a secrets.Secret.
func (b *Backend) valueToSecret(path string, value *SecretValue) *secrets.Secret {
	secret := &secrets.Secret{
		Path:     path,
		Backend:  secrets.BackendTypeAWS,
		Type:     secrets.SecretTypeStatic,
		Data:     make(map[string]interface{}),
		Metadata: make(map[string]string),
	}

	// Set metadata
	secret.Metadata["arn"] = value.ARN
	secret.Metadata["version_id"] = value.VersionID
	if value.IsCurrentVersion() {
		secret.Metadata["version_stage"] = "AWSCURRENT"
	} else if value.IsPreviousVersion() {
		secret.Metadata["version_stage"] = "AWSPREVIOUS"
	} else if value.IsPendingVersion() {
		secret.Metadata["version_stage"] = "AWSPENDING"
	}
	if !value.CreatedDate.IsZero() {
		secret.CreatedAt = value.CreatedDate
	}

	// Parse the secret data
	if b.config.JSONKeys && value.SecretString != "" {
		// Try to parse as JSON
		var jsonData map[string]interface{}
		if err := json.Unmarshal([]byte(value.SecretString), &jsonData); err == nil {
			secret.Data = jsonData
		} else {
			// Not JSON, store as raw value
			secret.Data["value"] = value.SecretString
		}
	} else if value.SecretString != "" {
		secret.Data["value"] = value.SecretString
	} else if value.SecretBinary != nil {
		secret.Data["value"] = value.SecretBinary
	}

	return secret
}

// CrossAccountConfig configures cross-account secret access.
type CrossAccountConfig struct {
	// AccountID is the target AWS account ID.
	AccountID string `json:"account_id"`

	// RoleARN is the role to assume in the target account.
	RoleARN string `json:"role_arn"`

	// ExternalID is the external ID for role assumption.
	ExternalID string `json:"external_id,omitempty"`

	// Region is the region for the target account.
	Region string `json:"region,omitempty"`
}

// NewCrossAccountBackend creates a backend for accessing secrets in another AWS account.
func NewCrossAccountBackend(ctx context.Context, crossAccountConfig *CrossAccountConfig, baseConfig *BackendConfig) (*Backend, error) {
	if crossAccountConfig == nil {
		return nil, fmt.Errorf("cross account config is required")
	}
	if crossAccountConfig.RoleARN == "" {
		return nil, fmt.Errorf("role_arn is required for cross-account access")
	}

	if baseConfig == nil {
		baseConfig = DefaultBackendConfig()
	}

	// Configure assume role authentication
	clientConfig := baseConfig.ClientConfig
	if clientConfig == nil {
		clientConfig = DefaultClientConfig()
	}

	clientConfig.AuthMethod = AuthMethodAssumeRole
	clientConfig.AssumeRoleARN = crossAccountConfig.RoleARN
	clientConfig.ExternalID = crossAccountConfig.ExternalID
	if crossAccountConfig.Region != "" {
		clientConfig.Region = crossAccountConfig.Region
	}
	clientConfig.RoleSessionName = fmt.Sprintf("keystone-cross-account-%s", crossAccountConfig.AccountID)

	baseConfig.ClientConfig = clientConfig

	return NewBackend(ctx, baseConfig)
}
