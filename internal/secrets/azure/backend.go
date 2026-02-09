// Package azure provides an Azure Key Vault backend for the secrets broker.
package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/secrets"
)

// Backend implements the SecretBackend interface for Azure Key Vault.
type Backend struct {
	name   string
	client *Client
	config *BackendConfig
}

// BackendConfig configures the Azure Key Vault backend.
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
		Name:            "azure",
		PathPrefix:      "azure/",
		DefaultCacheTTL: 5 * time.Minute,
		JSONKeys:        true,
	}
}

// NewBackend creates a new Azure Key Vault backend.
func NewBackend(ctx context.Context, config *BackendConfig) (*Backend, error) {
	if config == nil {
		config = DefaultBackendConfig()
	}

	if config.ClientConfig == nil {
		config.ClientConfig = DefaultClientConfig()
	}

	client, err := NewClient(ctx, config.ClientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure client: %w", err)
	}

	name := config.Name
	if name == "" {
		name = "azure"
	}

	return &Backend{
		name:   name,
		client: client,
		config: config,
	}, nil
}

// Type returns the backend type.
func (b *Backend) Type() secrets.BackendType {
	return secrets.BackendTypeAzure
}

// Name returns the backend instance name.
func (b *Backend) Name() string {
	return b.name
}

// Healthy checks if the backend is healthy by attempting to list secrets.
func (b *Backend) Healthy(ctx context.Context) bool {
	// Try to list secrets (with pagination we'll only get first page)
	_, err := b.client.ListSecrets(ctx)
	return err == nil
}

// Read reads a secret from Azure Key Vault.
func (b *Backend) Read(ctx context.Context, req *secrets.SecretRequest) (*secrets.Secret, error) {
	if req == nil {
		return nil, secrets.ErrInvalidPath
	}

	// Resolve the secret name from the path
	secretName := b.resolveSecretName(req.Path)

	// Build get options
	var opts []GetSecretOption
	if req.Version > 0 {
		// Azure uses string version IDs, convert version number
		// In practice, we would need the actual version string from context
		if version, ok := req.Context["version"]; ok {
			opts = append(opts, WithVersion(version))
		}
	}
	// Check for version in context
	if version, ok := req.Context["version"]; ok {
		opts = append(opts, WithVersion(version))
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
// Azure Key Vault doesn't generate dynamic secrets, but secrets can have expiration.
func (b *Backend) ReadDynamic(ctx context.Context, req *secrets.SecretRequest) (*secrets.Secret, error) {
	// For Azure, we just return the secret with its expiration metadata
	secret, err := b.Read(ctx, req)
	if err != nil {
		return nil, err
	}

	// Check if the secret has expiration and mark it appropriately
	if !secret.ExpiresAt.IsZero() {
		secret.Type = secrets.SecretTypeDynamic
	}

	return secret, nil
}

// List lists secrets under a path prefix.
func (b *Backend) List(ctx context.Context, prefix string) ([]string, error) {
	// List all secrets and filter by prefix
	entries, err := b.client.ListSecrets(ctx)
	if err != nil {
		return nil, err
	}

	// Resolve the prefix
	secretPrefix := b.resolveSecretName(prefix)

	// Filter and collect names
	var names []string
	for _, entry := range entries {
		name := entry.Name
		// Filter by prefix if specified
		if secretPrefix != "" && !strings.HasPrefix(name, secretPrefix) {
			continue
		}
		// Remove the prefix to return relative names
		if secretPrefix != "" {
			name = strings.TrimPrefix(name, secretPrefix)
		}
		names = append(names, name)
	}

	return names, nil
}

// RenewLease renews a lease.
// Azure Key Vault doesn't have a lease concept.
func (b *Backend) RenewLease(ctx context.Context, leaseID string, increment time.Duration) (*secrets.Lease, error) {
	return nil, secrets.ErrLeaseNotFound
}

// RevokeLease revokes a lease.
// Azure Key Vault doesn't have a lease concept.
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

// GetSecret retrieves a secret with full metadata.
func (b *Backend) GetSecret(ctx context.Context, path string, opts ...GetSecretOption) (*SecretValue, error) {
	secretName := b.resolveSecretName(path)
	return b.client.GetSecret(ctx, secretName, opts...)
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

// ListSecretVersions lists all versions of a secret.
func (b *Backend) ListSecretVersions(ctx context.Context, path string) ([]*SecretVersionInfo, error) {
	secretName := b.resolveSecretName(path)
	return b.client.ListSecretVersions(ctx, secretName)
}

// GetDeletedSecret retrieves a soft-deleted secret.
func (b *Backend) GetDeletedSecret(ctx context.Context, path string) (*SecretValue, error) {
	secretName := b.resolveSecretName(path)
	return b.client.GetDeletedSecret(ctx, secretName)
}

// ListDeletedSecrets lists soft-deleted secrets.
func (b *Backend) ListDeletedSecrets(ctx context.Context) ([]*SecretListEntry, error) {
	return b.client.ListDeletedSecrets(ctx)
}

// RecoverDeletedSecret recovers a soft-deleted secret.
func (b *Backend) RecoverDeletedSecret(ctx context.Context, path string) (*SecretValue, error) {
	secretName := b.resolveSecretName(path)
	return b.client.RecoverDeletedSecret(ctx, secretName)
}

// PurgeDeletedSecret permanently deletes a soft-deleted secret.
func (b *Backend) PurgeDeletedSecret(ctx context.Context, path string) error {
	secretName := b.resolveSecretName(path)
	return b.client.PurgeDeletedSecret(ctx, secretName)
}

// GetKey retrieves a key.
func (b *Backend) GetKey(ctx context.Context, name string, opts ...GetKeyOption) (*KeyInfo, error) {
	return b.client.GetKey(ctx, name, opts...)
}

// ListKeys lists keys.
func (b *Backend) ListKeys(ctx context.Context) ([]*KeyInfo, error) {
	return b.client.ListKeys(ctx)
}

// ListKeyVersions lists all versions of a key.
func (b *Backend) ListKeyVersions(ctx context.Context, name string) ([]*KeyInfo, error) {
	return b.client.ListKeyVersions(ctx, name)
}

// GetCertificate retrieves a certificate.
func (b *Backend) GetCertificate(ctx context.Context, name string, opts ...GetCertificateOption) (*CertificateInfo, error) {
	return b.client.GetCertificate(ctx, name, opts...)
}

// ListCertificates lists certificates.
func (b *Backend) ListCertificates(ctx context.Context) ([]*CertificateInfo, error) {
	return b.client.ListCertificates(ctx)
}

// ListCertificateVersions lists all versions of a certificate.
func (b *Backend) ListCertificateVersions(ctx context.Context, name string) ([]*CertificateInfo, error) {
	return b.client.ListCertificateVersions(ctx, name)
}

// resolveSecretName resolves the Azure secret name from a path.
func (b *Backend) resolveSecretName(path string) string {
	// Remove the path prefix if present
	name := path
	if b.config.PathPrefix != "" && strings.HasPrefix(path, b.config.PathPrefix) {
		name = strings.TrimPrefix(path, b.config.PathPrefix)
	}
	// Azure secret names can have hyphens but not slashes, so we replace slashes
	// However, we should generally preserve the name as-is after removing prefix
	return strings.Trim(name, "/")
}

// valueToSecret converts an Azure SecretValue to a secrets.Secret.
func (b *Backend) valueToSecret(path string, value *SecretValue) *secrets.Secret {
	secret := &secrets.Secret{
		Path:     path,
		Backend:  secrets.BackendTypeAzure,
		Type:     secrets.SecretTypeStatic,
		Data:     make(map[string]interface{}),
		Metadata: make(map[string]string),
	}

	// Set metadata
	secret.Metadata["id"] = value.ID
	secret.Metadata["version"] = value.Version
	if value.ContentType != "" {
		secret.Metadata["content_type"] = value.ContentType
	}

	// Copy tags to metadata
	for k, v := range value.Tags {
		secret.Metadata["tag:"+k] = v
	}

	// Set attributes
	if value.Attributes != nil {
		if !value.Attributes.Created.IsZero() {
			secret.CreatedAt = value.Attributes.Created
		}
		if !value.Attributes.Expires.IsZero() {
			secret.ExpiresAt = value.Attributes.Expires
		}
		if value.Attributes.RecoveryLevel != "" {
			secret.Metadata["recovery_level"] = value.Attributes.RecoveryLevel
		}
		if value.Attributes.RecoverableDays > 0 {
			secret.Metadata["recoverable_days"] = fmt.Sprintf("%d", value.Attributes.RecoverableDays)
		}
		secret.Metadata["enabled"] = fmt.Sprintf("%t", value.Attributes.Enabled)
	}

	// Parse the secret data
	if b.config.JSONKeys && value.Value != "" {
		// Try to parse as JSON
		var jsonData map[string]interface{}
		if err := json.Unmarshal([]byte(value.Value), &jsonData); err == nil {
			secret.Data = jsonData
		} else {
			// Not JSON, store as raw value
			secret.Data["value"] = value.Value
		}
	} else if value.Value != "" {
		secret.Data["value"] = value.Value
	}

	return secret
}

// MultiTenantConfig configures multi-tenant access.
type MultiTenantConfig struct {
	// TenantID is the target tenant ID.
	TenantID string `json:"tenant_id"`

	// AdditionallyAllowedTenants are additional tenants to allow.
	AdditionallyAllowedTenants []string `json:"additionally_allowed_tenants,omitempty"`
}

// NewMultiTenantBackend creates a backend for accessing secrets in another tenant.
func NewMultiTenantBackend(ctx context.Context, tenantConfig *MultiTenantConfig, baseConfig *BackendConfig) (*Backend, error) {
	if tenantConfig == nil {
		return nil, fmt.Errorf("tenant config is required")
	}
	if tenantConfig.TenantID == "" {
		return nil, fmt.Errorf("tenant_id is required for multi-tenant access")
	}

	if baseConfig == nil {
		baseConfig = DefaultBackendConfig()
	}

	clientConfig := baseConfig.ClientConfig
	if clientConfig == nil {
		clientConfig = DefaultClientConfig()
	}

	clientConfig.TenantID = tenantConfig.TenantID
	clientConfig.AdditionallyAllowedTenants = tenantConfig.AdditionallyAllowedTenants

	baseConfig.ClientConfig = clientConfig

	return NewBackend(ctx, baseConfig)
}

// PrivateLinkConfig configures Private Link access.
type PrivateLinkConfig struct {
	// Enabled indicates if Private Link is enabled.
	Enabled bool `json:"enabled"`

	// CustomEndpoint is the private endpoint URL.
	CustomEndpoint string `json:"custom_endpoint,omitempty"`

	// DisableInstanceDiscovery disables AAD instance discovery for air-gapped networks.
	DisableInstanceDiscovery bool `json:"disable_instance_discovery,omitempty"`
}

// NewPrivateLinkBackend creates a backend for accessing Key Vault via Private Link.
func NewPrivateLinkBackend(ctx context.Context, privateLinkConfig *PrivateLinkConfig, baseConfig *BackendConfig) (*Backend, error) {
	if privateLinkConfig == nil {
		return nil, fmt.Errorf("private link config is required")
	}

	if baseConfig == nil {
		baseConfig = DefaultBackendConfig()
	}

	clientConfig := baseConfig.ClientConfig
	if clientConfig == nil {
		clientConfig = DefaultClientConfig()
	}

	clientConfig.PrivateLinkEnabled = privateLinkConfig.Enabled
	if privateLinkConfig.CustomEndpoint != "" {
		clientConfig.CustomEndpoint = privateLinkConfig.CustomEndpoint
	}
	clientConfig.DisableInstanceDiscovery = privateLinkConfig.DisableInstanceDiscovery

	baseConfig.ClientConfig = clientConfig

	return NewBackend(ctx, baseConfig)
}
