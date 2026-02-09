// Package gcp provides a GCP Secret Manager backend for the secrets broker.
package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/secrets"
)

// Backend implements the SecretBackend interface for GCP Secret Manager.
type Backend struct {
	name   string
	client *Client
	config *BackendConfig
}

// BackendConfig configures the GCP Secret Manager backend.
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
		Name:            "gcp",
		PathPrefix:      "gcp/",
		DefaultCacheTTL: 5 * time.Minute,
		JSONKeys:        true,
	}
}

// NewBackend creates a new GCP Secret Manager backend.
func NewBackend(ctx context.Context, config *BackendConfig) (*Backend, error) {
	if config == nil {
		config = DefaultBackendConfig()
	}

	if config.ClientConfig == nil {
		config.ClientConfig = DefaultClientConfig()
	}

	client, err := NewClient(ctx, config.ClientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCP client: %w", err)
	}

	name := config.Name
	if name == "" {
		name = "gcp"
	}

	return &Backend{
		name:   name,
		client: client,
		config: config,
	}, nil
}

// Type returns the backend type.
func (b *Backend) Type() secrets.BackendType {
	return secrets.BackendTypeGCP
}

// Name returns the backend instance name.
func (b *Backend) Name() string {
	return b.name
}

// Healthy checks if the backend is healthy by attempting to list secrets.
func (b *Backend) Healthy(ctx context.Context) bool {
	// Try to list secrets (with pagination we'll only get first page)
	_, err := b.client.ListSecrets(ctx, WithPageSize(1))
	return err == nil
}

// Read reads a secret from GCP Secret Manager.
func (b *Backend) Read(ctx context.Context, req *secrets.SecretRequest) (*secrets.Secret, error) {
	if req == nil {
		return nil, secrets.ErrInvalidPath
	}

	// Resolve the secret name from the path
	secretName := b.resolveSecretName(req.Path)

	// Build get options
	var opts []GetSecretOption
	if req.Version > 0 {
		opts = append(opts, WithVersion(fmt.Sprintf("%d", req.Version)))
	} else if version, ok := req.Context["version"]; ok {
		opts = append(opts, WithVersion(version))
	} else {
		opts = append(opts, WithLatestVersion())
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

// ReadDynamic reads a dynamic secret.
// GCP Secret Manager doesn't generate dynamic secrets, but secrets can have rotation.
func (b *Backend) ReadDynamic(ctx context.Context, req *secrets.SecretRequest) (*secrets.Secret, error) {
	// Get the secret
	secret, err := b.Read(ctx, req)
	if err != nil {
		return nil, err
	}

	// Get metadata to check rotation status
	secretName := b.resolveSecretName(req.Path)
	metadata, err := b.client.GetSecretMetadata(ctx, secretName)
	if err == nil && metadata.Rotation != nil {
		secret.Type = secrets.SecretTypeDynamic
		if !metadata.Rotation.NextRotationTime.IsZero() {
			secret.Metadata["next_rotation"] = metadata.Rotation.NextRotationTime.Format(time.RFC3339)
			secret.ExpiresAt = metadata.Rotation.NextRotationTime
		}
		if metadata.Rotation.RotationPeriod > 0 {
			secret.Metadata["rotation_period"] = metadata.Rotation.RotationPeriod.String()
		}
	}

	return secret, nil
}

// List lists secrets under a path prefix.
func (b *Backend) List(ctx context.Context, prefix string) ([]string, error) {
	// Build filter if prefix is specified
	var opts []ListSecretsOption
	resolvedPrefix := b.resolveSecretName(prefix)
	// Note: GCP uses filter expressions (e.g., labels.env=prod), not prefix filtering.
	// We filter client-side since direct prefix filtering isn't supported.

	// List all secrets
	entries, err := b.client.ListSecrets(ctx, opts...)
	if err != nil {
		return nil, err
	}

	// Filter and collect names
	var names []string
	for _, entry := range entries {
		name := entry.ShortName
		// Filter by prefix if specified
		if resolvedPrefix != "" && !strings.HasPrefix(name, resolvedPrefix) {
			continue
		}
		// Remove the prefix to return relative names
		if resolvedPrefix != "" {
			name = strings.TrimPrefix(name, resolvedPrefix)
			name = strings.TrimPrefix(name, "/")
		}
		names = append(names, name)
	}

	return names, nil
}

// RenewLease renews a lease.
// GCP Secret Manager doesn't have a lease concept.
func (b *Backend) RenewLease(ctx context.Context, leaseID string, increment time.Duration) (*secrets.Lease, error) {
	return nil, secrets.ErrLeaseNotFound
}

// RevokeLease revokes a lease.
// GCP Secret Manager doesn't have a lease concept.
func (b *Backend) RevokeLease(ctx context.Context, leaseID string) error {
	return secrets.ErrLeaseNotFound
}

// Close closes the backend.
func (b *Backend) Close() error {
	if b.client != nil {
		return b.client.Close()
	}
	return nil
}

// GetSecretMetadata retrieves metadata about a secret.
func (b *Backend) GetSecretMetadata(ctx context.Context, path string) (*SecretMetadata, error) {
	secretName := b.resolveSecretName(path)
	return b.client.GetSecretMetadata(ctx, secretName)
}

// GetSecretVersion retrieves a specific version of a secret.
func (b *Backend) GetSecretVersion(ctx context.Context, path, version string) (*secrets.Secret, error) {
	secretName := b.resolveSecretName(path)

	value, err := b.client.GetSecret(ctx, secretName, WithVersion(version))
	if err != nil {
		return nil, err
	}

	return b.valueToSecret(path, value), nil
}

// GetLatestVersion retrieves the latest version of a secret.
func (b *Backend) GetLatestVersion(ctx context.Context, path string) (*secrets.Secret, error) {
	return b.GetSecretVersion(ctx, path, "latest")
}

// ListSecretVersions lists all versions of a secret.
func (b *Backend) ListSecretVersions(ctx context.Context, path string) ([]*SecretVersionInfo, error) {
	secretName := b.resolveSecretName(path)
	return b.client.ListSecretVersions(ctx, secretName)
}

// GetVersionMetadata retrieves metadata about a specific secret version.
func (b *Backend) GetVersionMetadata(ctx context.Context, path, version string) (*SecretVersionInfo, error) {
	secretName := b.resolveSecretName(path)
	return b.client.GetSecretVersionMetadata(ctx, secretName, version)
}

// EnableVersion enables a disabled secret version.
func (b *Backend) EnableVersion(ctx context.Context, path, version string) (*SecretVersionInfo, error) {
	secretName := b.resolveSecretName(path)
	return b.client.EnableSecretVersion(ctx, secretName, version)
}

// DisableVersion disables an enabled secret version.
func (b *Backend) DisableVersion(ctx context.Context, path, version string) (*SecretVersionInfo, error) {
	secretName := b.resolveSecretName(path)
	return b.client.DisableSecretVersion(ctx, secretName, version)
}

// DestroyVersion destroys a secret version.
func (b *Backend) DestroyVersion(ctx context.Context, path, version string) (*SecretVersionInfo, error) {
	secretName := b.resolveSecretName(path)
	return b.client.DestroySecretVersion(ctx, secretName, version)
}

// GetRotationConfig retrieves rotation configuration for a secret.
func (b *Backend) GetRotationConfig(ctx context.Context, path string) (*RotationConfig, error) {
	metadata, err := b.GetSecretMetadata(ctx, path)
	if err != nil {
		return nil, err
	}
	return metadata.Rotation, nil
}

// GetCMEKConfig retrieves CMEK configuration for a secret.
func (b *Backend) GetCMEKConfig(ctx context.Context, path string) (*CMEKConfig, error) {
	metadata, err := b.GetSecretMetadata(ctx, path)
	if err != nil {
		return nil, err
	}
	return metadata.CustomerManagedEncryption, nil
}

// GetReplicationConfig retrieves replication configuration for a secret.
func (b *Backend) GetReplicationConfig(ctx context.Context, path string) (*ReplicationConfig, error) {
	metadata, err := b.GetSecretMetadata(ctx, path)
	if err != nil {
		return nil, err
	}
	return metadata.Replication, nil
}

// GetTopics retrieves Pub/Sub topics configured for a secret.
func (b *Backend) GetTopics(ctx context.Context, path string) ([]string, error) {
	metadata, err := b.GetSecretMetadata(ctx, path)
	if err != nil {
		return nil, err
	}
	return metadata.Topics, nil
}

// GetVersionAliases retrieves version aliases for a secret.
func (b *Backend) GetVersionAliases(ctx context.Context, path string) (map[string]int64, error) {
	metadata, err := b.GetSecretMetadata(ctx, path)
	if err != nil {
		return nil, err
	}
	return metadata.VersionAliases, nil
}

// IsRotationEnabled checks if rotation is enabled for a secret.
func (b *Backend) IsRotationEnabled(ctx context.Context, path string) (bool, error) {
	rotation, err := b.GetRotationConfig(ctx, path)
	if err != nil {
		return false, err
	}
	return rotation != nil && !rotation.NextRotationTime.IsZero(), nil
}

// resolveSecretName resolves the GCP secret name from a path.
func (b *Backend) resolveSecretName(path string) string {
	// Remove the path prefix if present
	name := path
	if b.config.PathPrefix != "" && strings.HasPrefix(path, b.config.PathPrefix) {
		name = strings.TrimPrefix(path, b.config.PathPrefix)
	}
	return strings.Trim(name, "/")
}

// valueToSecret converts a GCP SecretValue to a secrets.Secret.
func (b *Backend) valueToSecret(path string, value *SecretValue) *secrets.Secret {
	secret := &secrets.Secret{
		Path:     path,
		Backend:  secrets.BackendTypeGCP,
		Type:     secrets.SecretTypeStatic,
		Data:     make(map[string]interface{}),
		Metadata: make(map[string]string),
	}

	// Set metadata
	secret.Metadata["name"] = value.Name
	secret.Metadata["version"] = value.Version
	secret.Metadata["state"] = string(value.State)
	if !value.CreateTime.IsZero() {
		secret.CreatedAt = value.CreateTime
	}
	if value.Etag != "" {
		secret.Metadata["etag"] = value.Etag
	}

	// Parse the secret data
	if b.config.JSONKeys && len(value.Data) > 0 {
		// Try to parse as JSON
		var jsonData map[string]interface{}
		if err := json.Unmarshal(value.Data, &jsonData); err == nil {
			secret.Data = jsonData
		} else {
			// Not JSON, store as raw value
			secret.Data["value"] = value.GetString()
		}
	} else if len(value.Data) > 0 {
		secret.Data["value"] = value.GetString()
	}

	return secret
}

// CrossProjectConfig configures cross-project secret access.
type CrossProjectConfig struct {
	// ProjectID is the target project ID.
	ProjectID string `json:"project_id"`

	// ImpersonateServiceAccount is the service account to impersonate.
	ImpersonateServiceAccount string `json:"impersonate_service_account,omitempty"`
}

// NewCrossProjectBackend creates a backend for accessing secrets in another project.
func NewCrossProjectBackend(ctx context.Context, crossProjectConfig *CrossProjectConfig, baseConfig *BackendConfig) (*Backend, error) {
	if crossProjectConfig == nil {
		return nil, fmt.Errorf("cross project config is required")
	}
	if crossProjectConfig.ProjectID == "" {
		return nil, fmt.Errorf("project_id is required for cross-project access")
	}

	if baseConfig == nil {
		baseConfig = DefaultBackendConfig()
	}

	clientConfig := baseConfig.ClientConfig
	if clientConfig == nil {
		clientConfig = DefaultClientConfig()
	}

	clientConfig.ProjectID = crossProjectConfig.ProjectID
	if crossProjectConfig.ImpersonateServiceAccount != "" {
		clientConfig.AuthMethod = AuthMethodImpersonation
		clientConfig.ImpersonateServiceAccount = crossProjectConfig.ImpersonateServiceAccount
	}

	baseConfig.ClientConfig = clientConfig

	return NewBackend(ctx, baseConfig)
}

// VPCServiceControlsConfig configures VPC Service Controls settings.
type VPCServiceControlsConfig struct {
	// Enabled indicates if VPC Service Controls are enabled.
	Enabled bool `json:"enabled"`

	// Endpoint is the VPC-SC restricted endpoint.
	Endpoint string `json:"endpoint,omitempty"`
}

// NewVPCServiceControlsBackend creates a backend configured for VPC Service Controls.
func NewVPCServiceControlsBackend(ctx context.Context, vpcConfig *VPCServiceControlsConfig, baseConfig *BackendConfig) (*Backend, error) {
	if vpcConfig == nil {
		return nil, fmt.Errorf("vpc service controls config is required")
	}

	if baseConfig == nil {
		baseConfig = DefaultBackendConfig()
	}

	clientConfig := baseConfig.ClientConfig
	if clientConfig == nil {
		clientConfig = DefaultClientConfig()
	}

	if vpcConfig.Endpoint != "" {
		clientConfig.Endpoint = vpcConfig.Endpoint
	} else if vpcConfig.Enabled {
		// Use the default VPC-SC endpoint
		clientConfig.Endpoint = "secretmanager.googleapis.com:443"
	}

	baseConfig.ClientConfig = clientConfig

	return NewBackend(ctx, baseConfig)
}
