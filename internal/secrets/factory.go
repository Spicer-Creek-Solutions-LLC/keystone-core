package secrets

import (
	"context"
	"fmt"
	"time"
)

// BackendConfig is the unified configuration for all backends.
type BackendConfig struct {
	// Type is the backend type (vault, aws_secrets_manager, azure_keyvault, gcp_secret_manager).
	Type string `json:"type" yaml:"type"`

	// Name is the backend instance name.
	Name string `json:"name,omitempty" yaml:"name,omitempty"`

	// Enabled enables or disables this backend.
	Enabled bool `json:"enabled" yaml:"enabled"`

	// Primary marks this backend as primary in its group.
	Primary bool `json:"primary,omitempty" yaml:"primary,omitempty"`

	// Vault contains Vault-specific configuration.
	Vault *VaultBackendConfig `json:"vault,omitempty" yaml:"vault,omitempty"`

	// AWS contains AWS Secrets Manager-specific configuration.
	AWS *AWSBackendConfig `json:"aws,omitempty" yaml:"aws,omitempty"`

	// Azure contains Azure Key Vault-specific configuration.
	Azure *AzureBackendConfig `json:"azure,omitempty" yaml:"azure,omitempty"`

	// GCP contains GCP Secret Manager-specific configuration.
	GCP *GCPBackendConfig `json:"gcp,omitempty" yaml:"gcp,omitempty"`

	// Retry contains retry configuration.
	Retry *RetryConfig `json:"retry,omitempty" yaml:"retry,omitempty"`

	// PathPrefix is the path prefix for this backend.
	PathPrefix string `json:"path_prefix,omitempty" yaml:"path_prefix,omitempty"`
}

// VaultBackendConfig contains Vault-specific configuration.
type VaultBackendConfig struct {
	// Address is the Vault server address.
	Address string `json:"address" yaml:"address"`

	// Namespace is the Vault namespace (Enterprise only).
	Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`

	// Auth contains authentication configuration.
	Auth *VaultAuthConfig `json:"auth,omitempty" yaml:"auth,omitempty"`

	// TLS contains TLS configuration.
	TLS *TLSConfig `json:"tls,omitempty" yaml:"tls,omitempty"`

	// Timeout is the request timeout.
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`

	// MaxRetries is the maximum number of retries.
	MaxRetries int `json:"max_retries,omitempty" yaml:"max_retries,omitempty"`
}

// VaultAuthConfig contains Vault authentication configuration.
type VaultAuthConfig struct {
	// Method is the authentication method (token, approle, kubernetes, userpass, ldap, aws, gcp).
	Method string `json:"method" yaml:"method"`

	// Token is the Vault token (for token auth).
	Token string `json:"token,omitempty" yaml:"token,omitempty"`

	// TokenFile is the path to a file containing the token.
	TokenFile string `json:"token_file,omitempty" yaml:"token_file,omitempty"`

	// RoleID is the AppRole role ID.
	RoleID string `json:"role_id,omitempty" yaml:"role_id,omitempty"`

	// RoleIDFile is the path to a file containing the role ID.
	RoleIDFile string `json:"role_id_file,omitempty" yaml:"role_id_file,omitempty"`

	// SecretID is the AppRole secret ID.
	SecretID string `json:"secret_id,omitempty" yaml:"secret_id,omitempty"`

	// SecretIDFile is the path to a file containing the secret ID.
	SecretIDFile string `json:"secret_id_file,omitempty" yaml:"secret_id_file,omitempty"`

	// MountPath is the auth mount path.
	MountPath string `json:"mount_path,omitempty" yaml:"mount_path,omitempty"`

	// Role is the role name (for kubernetes, aws, gcp auth).
	Role string `json:"role,omitempty" yaml:"role,omitempty"`

	// JWTPath is the path to the JWT token file (for kubernetes auth).
	JWTPath string `json:"jwt_path,omitempty" yaml:"jwt_path,omitempty"`
}

// TLSConfig contains TLS configuration.
type TLSConfig struct {
	// CACert is the path to the CA certificate.
	CACert string `json:"ca_cert,omitempty" yaml:"ca_cert,omitempty"`

	// CACertData is the PEM-encoded CA certificate data.
	CACertData string `json:"ca_cert_data,omitempty" yaml:"ca_cert_data,omitempty"`

	// ClientCert is the path to the client certificate.
	ClientCert string `json:"client_cert,omitempty" yaml:"client_cert,omitempty"`

	// ClientKey is the path to the client key.
	ClientKey string `json:"client_key,omitempty" yaml:"client_key,omitempty"`

	// InsecureSkipVerify disables TLS verification.
	InsecureSkipVerify bool `json:"insecure_skip_verify,omitempty" yaml:"insecure_skip_verify,omitempty"`

	// ServerName is the expected server name for verification.
	ServerName string `json:"server_name,omitempty" yaml:"server_name,omitempty"`
}

// AWSBackendConfig contains AWS Secrets Manager-specific configuration.
type AWSBackendConfig struct {
	// Region is the AWS region.
	Region string `json:"region" yaml:"region"`

	// Auth contains authentication configuration.
	Auth *AWSAuthConfig `json:"auth,omitempty" yaml:"auth,omitempty"`

	// Endpoint is the custom endpoint URL (for testing/LocalStack).
	Endpoint string `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`

	// CacheTTL is the cache TTL for secrets.
	CacheTTL time.Duration `json:"cache_ttl,omitempty" yaml:"cache_ttl,omitempty"`

	// VersionStage is the default version stage (AWSCURRENT, AWSPREVIOUS).
	VersionStage string `json:"version_stage,omitempty" yaml:"version_stage,omitempty"`
}

// AWSAuthConfig contains AWS authentication configuration.
type AWSAuthConfig struct {
	// Method is the authentication method (default, static, assume_role, web_identity).
	Method string `json:"method,omitempty" yaml:"method,omitempty"`

	// AccessKeyID is the AWS access key ID (for static auth).
	AccessKeyID string `json:"access_key_id,omitempty" yaml:"access_key_id,omitempty"`

	// SecretAccessKey is the AWS secret access key (for static auth).
	SecretAccessKey string `json:"secret_access_key,omitempty" yaml:"secret_access_key,omitempty"`

	// RoleARN is the role ARN to assume.
	RoleARN string `json:"role_arn,omitempty" yaml:"role_arn,omitempty"`

	// SessionName is the session name for assume role.
	SessionName string `json:"session_name,omitempty" yaml:"session_name,omitempty"`

	// ExternalID is the external ID for assume role.
	ExternalID string `json:"external_id,omitempty" yaml:"external_id,omitempty"`

	// WebIdentityTokenFile is the path to the web identity token file.
	WebIdentityTokenFile string `json:"web_identity_token_file,omitempty" yaml:"web_identity_token_file,omitempty"`
}

// AzureBackendConfig contains Azure Key Vault-specific configuration.
type AzureBackendConfig struct {
	// VaultURL is the Key Vault URL.
	VaultURL string `json:"vault_url" yaml:"vault_url"`

	// Auth contains authentication configuration.
	Auth *AzureAuthConfig `json:"auth,omitempty" yaml:"auth,omitempty"`

	// CacheTTL is the cache TTL for secrets.
	CacheTTL time.Duration `json:"cache_ttl,omitempty" yaml:"cache_ttl,omitempty"`
}

// AzureAuthConfig contains Azure authentication configuration.
type AzureAuthConfig struct {
	// Method is the authentication method (default, managed_identity, service_principal, cli, workload_identity).
	Method string `json:"method,omitempty" yaml:"method,omitempty"`

	// TenantID is the Azure tenant ID.
	TenantID string `json:"tenant_id,omitempty" yaml:"tenant_id,omitempty"`

	// ClientID is the client/application ID.
	ClientID string `json:"client_id,omitempty" yaml:"client_id,omitempty"`

	// ClientSecret is the client secret (for service principal auth).
	ClientSecret string `json:"client_secret,omitempty" yaml:"client_secret,omitempty"`

	// ClientSecretFile is the path to a file containing the client secret.
	ClientSecretFile string `json:"client_secret_file,omitempty" yaml:"client_secret_file,omitempty"`

	// ClientCertificatePath is the path to the client certificate (for certificate auth).
	ClientCertificatePath string `json:"client_certificate_path,omitempty" yaml:"client_certificate_path,omitempty"`

	// ManagedIdentityClientID is the client ID for user-assigned managed identity.
	ManagedIdentityClientID string `json:"managed_identity_client_id,omitempty" yaml:"managed_identity_client_id,omitempty"`
}

// GCPBackendConfig contains GCP Secret Manager-specific configuration.
type GCPBackendConfig struct {
	// ProjectID is the GCP project ID.
	ProjectID string `json:"project_id" yaml:"project_id"`

	// Auth contains authentication configuration.
	Auth *GCPAuthConfig `json:"auth,omitempty" yaml:"auth,omitempty"`

	// Endpoint is the custom endpoint URL (for testing).
	Endpoint string `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`

	// CacheTTL is the cache TTL for secrets.
	CacheTTL time.Duration `json:"cache_ttl,omitempty" yaml:"cache_ttl,omitempty"`
}

// GCPAuthConfig contains GCP authentication configuration.
type GCPAuthConfig struct {
	// Method is the authentication method (default, service_account, workload_identity, impersonation).
	Method string `json:"method,omitempty" yaml:"method,omitempty"`

	// CredentialsFile is the path to the service account key file.
	CredentialsFile string `json:"credentials_file,omitempty" yaml:"credentials_file,omitempty"`

	// ImpersonateServiceAccount is the service account to impersonate.
	ImpersonateServiceAccount string `json:"impersonate_service_account,omitempty" yaml:"impersonate_service_account,omitempty"`
}

// SecretsConfig is the top-level secrets configuration.
type SecretsConfig struct {
	// DefaultBackend is the default backend name.
	DefaultBackend string `json:"default_backend,omitempty" yaml:"default_backend,omitempty"`

	// Backends contains backend configurations.
	Backends map[string]*BackendConfig `json:"backends,omitempty" yaml:"backends,omitempty"`

	// Cache contains cache configuration.
	Cache *CacheConfig `json:"cache,omitempty" yaml:"cache,omitempty"`

	// LeaseRenewal contains lease renewal configuration.
	LeaseRenewal *LeaseRenewalConfig `json:"lease_renewal,omitempty" yaml:"lease_renewal,omitempty"`

	// Routing contains routing rules.
	Routing []RoutingRule `json:"routing,omitempty" yaml:"routing,omitempty"`

	// HealthMonitor contains health monitoring configuration.
	HealthMonitor *HealthMonitorConfig `json:"health_monitor,omitempty" yaml:"health_monitor,omitempty"`

	// Failover contains failover configuration.
	Failover *FailoverPolicy `json:"failover,omitempty" yaml:"failover,omitempty"`
}

// BackendFactory creates secret backends from configuration.
type BackendFactory struct {
	// constructors maps backend types to their constructors.
	constructors map[string]BackendConstructor
}

// BackendConstructor is a function that creates a backend from configuration.
type BackendConstructor func(ctx context.Context, name string, config *BackendConfig) (SecretBackend, error)

// NewBackendFactory creates a new backend factory.
func NewBackendFactory() *BackendFactory {
	f := &BackendFactory{
		constructors: make(map[string]BackendConstructor),
	}

	// Register default constructors
	f.Register("vault", f.createVaultBackend)
	f.Register("aws_secrets_manager", f.createAWSBackend)
	f.Register("azure_keyvault", f.createAzureBackend)
	f.Register("gcp_secret_manager", f.createGCPBackend)

	return f
}

// Register registers a backend constructor.
func (f *BackendFactory) Register(backendType string, constructor BackendConstructor) {
	f.constructors[backendType] = constructor
}

// Create creates a backend from configuration.
func (f *BackendFactory) Create(ctx context.Context, name string, config *BackendConfig) (SecretBackend, error) {
	if config == nil {
		return nil, fmt.Errorf("backend config is required")
	}

	if config.Type == "" {
		return nil, fmt.Errorf("backend type is required")
	}

	constructor, ok := f.constructors[config.Type]
	if !ok {
		return nil, fmt.Errorf("unknown backend type: %s", config.Type)
	}

	backend, err := constructor(ctx, name, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s backend: %w", config.Type, err)
	}

	// Wrap with retry support if configured
	if config.Retry != nil {
		backend = NewRetryableBackend(backend, config.Retry)
	}

	return backend, nil
}

// CreateAll creates all backends from configuration.
func (f *BackendFactory) CreateAll(ctx context.Context, configs map[string]*BackendConfig) (map[string]SecretBackend, error) {
	backends := make(map[string]SecretBackend)

	for name, config := range configs {
		if !config.Enabled {
			continue
		}

		backend, err := f.Create(ctx, name, config)
		if err != nil {
			// Close any backends we've already created
			for _, b := range backends {
				_ = b.Close()
			}
			return nil, fmt.Errorf("failed to create backend %s: %w", name, err)
		}

		backends[name] = backend
	}

	return backends, nil
}

// createVaultBackend creates a Vault backend.
func (f *BackendFactory) createVaultBackend(ctx context.Context, name string, config *BackendConfig) (SecretBackend, error) {
	if config.Vault == nil {
		return nil, fmt.Errorf("vault configuration is required")
	}

	// This would normally call the vault package constructor
	// For now, return an error indicating Vault backend creation
	// The actual implementation would import internal/secrets/vault
	return nil, fmt.Errorf("vault backend creation requires vault package import (use vault.NewBackend)")
}

// createAWSBackend creates an AWS Secrets Manager backend.
func (f *BackendFactory) createAWSBackend(ctx context.Context, name string, config *BackendConfig) (SecretBackend, error) {
	if config.AWS == nil {
		return nil, fmt.Errorf("aws configuration is required")
	}

	// This would normally call the aws package constructor
	// For now, return an error indicating AWS backend creation
	// The actual implementation would import internal/secrets/aws
	return nil, fmt.Errorf("aws backend creation requires aws package import (use aws.NewBackend)")
}

// createAzureBackend creates an Azure Key Vault backend.
func (f *BackendFactory) createAzureBackend(ctx context.Context, name string, config *BackendConfig) (SecretBackend, error) {
	if config.Azure == nil {
		return nil, fmt.Errorf("azure configuration is required")
	}

	// This would normally call the azure package constructor
	// For now, return an error indicating Azure backend creation
	// The actual implementation would import internal/secrets/azure
	return nil, fmt.Errorf("azure backend creation requires azure package import (use azure.NewBackend)")
}

// createGCPBackend creates a GCP Secret Manager backend.
func (f *BackendFactory) createGCPBackend(ctx context.Context, name string, config *BackendConfig) (SecretBackend, error) {
	if config.GCP == nil {
		return nil, fmt.Errorf("gcp configuration is required")
	}

	// This would normally call the gcp package constructor
	// For now, return an error indicating GCP backend creation
	// The actual implementation would import internal/secrets/gcp
	return nil, fmt.Errorf("gcp backend creation requires gcp package import (use gcp.NewBackend)")
}

// BrokerBuilder helps construct a SecretBroker from configuration.
type BrokerBuilder struct {
	factory       *BackendFactory
	healthMonitor *HealthMonitor
	config        *SecretsConfig
}

// NewBrokerBuilder creates a new broker builder.
func NewBrokerBuilder() *BrokerBuilder {
	return &BrokerBuilder{
		factory: NewBackendFactory(),
	}
}

// WithConfig sets the secrets configuration.
func (bb *BrokerBuilder) WithConfig(config *SecretsConfig) *BrokerBuilder {
	bb.config = config
	return bb
}

// WithFactory sets the backend factory.
func (bb *BrokerBuilder) WithFactory(factory *BackendFactory) *BrokerBuilder {
	bb.factory = factory
	return bb
}

// WithHealthMonitor sets the health monitor.
func (bb *BrokerBuilder) WithHealthMonitor(hm *HealthMonitor) *BrokerBuilder {
	bb.healthMonitor = hm
	return bb
}

// Build builds the secret broker.
func (bb *BrokerBuilder) Build(ctx context.Context) (*SecretBroker, error) {
	if bb.config == nil {
		bb.config = &SecretsConfig{}
	}

	// Create broker config
	brokerConfig := &BrokerConfig{
		DefaultBackend: bb.config.DefaultBackend,
		Cache:          bb.config.Cache,
		LeaseRenewal:   bb.config.LeaseRenewal,
		Routing:        bb.config.Routing,
	}

	// Create broker
	broker := NewSecretBroker(brokerConfig)

	// Create and register backends
	for name, backendConfig := range bb.config.Backends {
		if !backendConfig.Enabled {
			continue
		}

		backend, err := bb.factory.Create(ctx, name, backendConfig)
		if err != nil {
			_ = broker.Close()
			return nil, fmt.Errorf("failed to create backend %s: %w", name, err)
		}

		if err := broker.RegisterBackend(name, backend); err != nil {
			_ = backend.Close()
			_ = broker.Close()
			return nil, fmt.Errorf("failed to register backend %s: %w", name, err)
		}

		// Register with health monitor if provided
		if bb.healthMonitor != nil {
			bb.healthMonitor.Register(name, backend)
		}
	}

	// Set up cache if configured
	if bb.config.Cache != nil && bb.config.Cache.Enabled {
		cache := NewEncryptedCache(bb.config.Cache)
		broker.SetCache(cache)
	}

	return broker, nil
}

// ValidateConfig validates the secrets configuration.
func ValidateConfig(config *SecretsConfig) error {
	if config == nil {
		return nil
	}

	// Validate backends
	for name, backendConfig := range config.Backends {
		if err := validateBackendConfig(name, backendConfig); err != nil {
			return err
		}
	}

	// Validate routing rules
	for i, rule := range config.Routing {
		if rule.Backend == "" {
			return fmt.Errorf("routing rule %d: backend is required", i)
		}
		if rule.Prefix == "" && rule.Tag == "" {
			return fmt.Errorf("routing rule %d: either prefix or tag is required", i)
		}
	}

	// Validate default backend exists
	if config.DefaultBackend != "" {
		if _, ok := config.Backends[config.DefaultBackend]; !ok {
			return fmt.Errorf("default backend %q not found in backends", config.DefaultBackend)
		}
	}

	return nil
}

// validateBackendConfig validates a single backend configuration.
func validateBackendConfig(name string, config *BackendConfig) error {
	if config == nil {
		return fmt.Errorf("backend %s: config is nil", name)
	}

	if config.Type == "" {
		return fmt.Errorf("backend %s: type is required", name)
	}

	switch config.Type {
	case "vault":
		if config.Vault == nil {
			return fmt.Errorf("backend %s: vault config is required for type vault", name)
		}
		if config.Vault.Address == "" {
			return fmt.Errorf("backend %s: vault address is required", name)
		}

	case "aws_secrets_manager":
		if config.AWS == nil {
			return fmt.Errorf("backend %s: aws config is required for type aws_secrets_manager", name)
		}
		if config.AWS.Region == "" {
			return fmt.Errorf("backend %s: aws region is required", name)
		}

	case "azure_keyvault":
		if config.Azure == nil {
			return fmt.Errorf("backend %s: azure config is required for type azure_keyvault", name)
		}
		if config.Azure.VaultURL == "" {
			return fmt.Errorf("backend %s: azure vault_url is required", name)
		}

	case "gcp_secret_manager":
		if config.GCP == nil {
			return fmt.Errorf("backend %s: gcp config is required for type gcp_secret_manager", name)
		}
		if config.GCP.ProjectID == "" {
			return fmt.Errorf("backend %s: gcp project_id is required", name)
		}

	default:
		return fmt.Errorf("backend %s: unknown type %q", name, config.Type)
	}

	return nil
}

// EncryptedCache is a placeholder for the encrypted cache implementation.
// The actual implementation is in cache.go.
type EncryptedCache struct {
	config *CacheConfig
}

// NewEncryptedCache creates a new encrypted cache.
func NewEncryptedCache(config *CacheConfig) *EncryptedCache {
	return &EncryptedCache{config: config}
}

// Get retrieves a secret from the cache.
func (c *EncryptedCache) Get(ctx context.Context, path string) (*Secret, bool) {
	return nil, false
}

// Put stores a secret in the cache.
func (c *EncryptedCache) Put(ctx context.Context, secret *Secret, ttl time.Duration) error {
	return nil
}

// Delete removes a secret from the cache.
func (c *EncryptedCache) Delete(ctx context.Context, path string) error {
	return nil
}

// DeleteByPrefix removes all secrets matching a path prefix.
func (c *EncryptedCache) DeleteByPrefix(ctx context.Context, prefix string) (int, error) {
	return 0, nil
}

// Clear removes all secrets from the cache.
func (c *EncryptedCache) Clear(ctx context.Context) error {
	return nil
}

// Stats returns cache statistics.
func (c *EncryptedCache) Stats() *CacheStats {
	return &CacheStats{}
}

// Close closes the cache.
func (c *EncryptedCache) Close() error {
	return nil
}

// Ensure EncryptedCache implements SecretCache.
var _ SecretCache = (*EncryptedCache)(nil)
