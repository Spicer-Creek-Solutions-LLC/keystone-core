// Package azure provides an Azure Key Vault backend for the secrets broker.
package azure

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azcertificates"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"

	"github.com/shawnbutts/keystone-core/internal/secrets"
)

// AuthMethod represents the authentication method for Azure.
type AuthMethod string

const (
	// AuthMethodDefault uses the Azure default credential chain.
	AuthMethodDefault AuthMethod = "default"

	// AuthMethodManagedIdentity uses managed identity (system or user-assigned).
	AuthMethodManagedIdentity AuthMethod = "managed_identity"

	// AuthMethodServicePrincipal uses a service principal with client secret.
	AuthMethodServicePrincipal AuthMethod = "service_principal"

	// AuthMethodCLI uses Azure CLI credentials.
	AuthMethodCLI AuthMethod = "cli"

	// AuthMethodEnvironment uses environment variables.
	AuthMethodEnvironment AuthMethod = "environment"

	// AuthMethodWorkloadIdentity uses workload identity federation.
	AuthMethodWorkloadIdentity AuthMethod = "workload_identity"
)

// ClientConfig configures the Azure Key Vault client.
type ClientConfig struct {
	// VaultURL is the URL of the Azure Key Vault (e.g., "https://myvault.vault.azure.net").
	VaultURL string `json:"vault_url"`

	// AuthMethod specifies the authentication method to use.
	AuthMethod AuthMethod `json:"auth_method,omitempty"`

	// TenantID is the Azure AD tenant ID.
	TenantID string `json:"tenant_id,omitempty"`

	// ClientID is the client ID for service principal or managed identity.
	ClientID string `json:"client_id,omitempty"`

	// ClientSecret is the client secret for service principal authentication.
	ClientSecret string `json:"client_secret,omitempty"`

	// Timeout is the timeout for API requests.
	Timeout time.Duration `json:"timeout,omitempty"`

	// RetryOptions configures retry behavior.
	RetryOptions *RetryOptions `json:"retry,omitempty"`

	// PrivateLinkEnabled indicates if Private Link is used.
	PrivateLinkEnabled bool `json:"private_link_enabled,omitempty"`

	// CustomEndpoint overrides the default vault endpoint for Private Link.
	CustomEndpoint string `json:"custom_endpoint,omitempty"`

	// DisableInstanceDiscovery disables instance discovery for air-gapped clouds.
	DisableInstanceDiscovery bool `json:"disable_instance_discovery,omitempty"`

	// AdditionallyAllowedTenants allows access to additional tenants.
	AdditionallyAllowedTenants []string `json:"additionally_allowed_tenants,omitempty"`

	// Cloud specifies the Azure cloud environment.
	Cloud string `json:"cloud,omitempty"` // "public", "government", "china"
}

// RetryOptions configures retry behavior.
type RetryOptions struct {
	// MaxRetries is the maximum number of retries.
	MaxRetries int `json:"max_retries,omitempty"`

	// RetryDelay is the initial delay between retries.
	RetryDelay time.Duration `json:"retry_delay,omitempty"`

	// MaxRetryDelay is the maximum delay between retries.
	MaxRetryDelay time.Duration `json:"max_retry_delay,omitempty"`
}

// DefaultClientConfig returns a client configuration with sensible defaults.
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		AuthMethod: AuthMethodDefault,
		Timeout:    30 * time.Second,
		Cloud:      "public",
		RetryOptions: &RetryOptions{
			MaxRetries:    3,
			RetryDelay:    time.Second,
			MaxRetryDelay: 30 * time.Second,
		},
	}
}

// Client is an Azure Key Vault client.
type Client struct {
	config       *ClientConfig
	secretClient *azsecrets.Client
	keyClient    *azkeys.Client
	certClient   *azcertificates.Client
	credential   azcore.TokenCredential
}

// NewClient creates a new Azure Key Vault client.
func NewClient(ctx context.Context, cfg *ClientConfig) (*Client, error) {
	if cfg == nil {
		return nil, errors.New("config is required")
	}

	if cfg.VaultURL == "" {
		return nil, errors.New("vault_url is required")
	}

	// Ensure vault URL doesn't have trailing slash
	cfg.VaultURL = strings.TrimSuffix(cfg.VaultURL, "/")

	// Create credential based on auth method
	cred, err := createCredential(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create credential: %w", err)
	}

	// Build client options
	clientOpts := buildClientOptions(cfg)

	// Determine vault URL (use custom endpoint for Private Link)
	vaultURL := cfg.VaultURL
	if cfg.CustomEndpoint != "" {
		vaultURL = cfg.CustomEndpoint
	}

	// Create secret client
	secretClient, err := azsecrets.NewClient(vaultURL, cred, &azsecrets.ClientOptions{
		ClientOptions: clientOpts,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create secret client: %w", err)
	}

	// Create key client
	keyClient, err := azkeys.NewClient(vaultURL, cred, &azkeys.ClientOptions{
		ClientOptions: clientOpts,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create key client: %w", err)
	}

	// Create certificate client
	certClient, err := azcertificates.NewClient(vaultURL, cred, &azcertificates.ClientOptions{
		ClientOptions: clientOpts,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate client: %w", err)
	}

	return &Client{
		config:       cfg,
		secretClient: secretClient,
		keyClient:    keyClient,
		certClient:   certClient,
		credential:   cred,
	}, nil
}

// createCredential creates an Azure credential based on the auth method.
func createCredential(cfg *ClientConfig) (azcore.TokenCredential, error) {
	switch cfg.AuthMethod {
	case AuthMethodDefault, "":
		opts := &azidentity.DefaultAzureCredentialOptions{}
		if cfg.DisableInstanceDiscovery {
			opts.DisableInstanceDiscovery = cfg.DisableInstanceDiscovery
		}
		if cfg.TenantID != "" {
			opts.TenantID = cfg.TenantID
		}
		if len(cfg.AdditionallyAllowedTenants) > 0 {
			opts.AdditionallyAllowedTenants = cfg.AdditionallyAllowedTenants
		}
		return azidentity.NewDefaultAzureCredential(opts)

	case AuthMethodManagedIdentity:
		miOpts := &azidentity.ManagedIdentityCredentialOptions{}
		if cfg.ClientID != "" {
			miOpts.ID = azidentity.ClientID(cfg.ClientID)
		}
		return azidentity.NewManagedIdentityCredential(miOpts)

	case AuthMethodServicePrincipal:
		if cfg.TenantID == "" {
			return nil, errors.New("tenant_id is required for service principal authentication")
		}
		if cfg.ClientID == "" {
			return nil, errors.New("client_id is required for service principal authentication")
		}
		if cfg.ClientSecret == "" {
			return nil, errors.New("client_secret is required for service principal authentication")
		}
		spOpts := &azidentity.ClientSecretCredentialOptions{}
		if cfg.DisableInstanceDiscovery {
			spOpts.DisableInstanceDiscovery = cfg.DisableInstanceDiscovery
		}
		if len(cfg.AdditionallyAllowedTenants) > 0 {
			spOpts.AdditionallyAllowedTenants = cfg.AdditionallyAllowedTenants
		}
		return azidentity.NewClientSecretCredential(cfg.TenantID, cfg.ClientID, cfg.ClientSecret, spOpts)

	case AuthMethodCLI:
		cliOpts := &azidentity.AzureCLICredentialOptions{}
		if cfg.TenantID != "" {
			cliOpts.TenantID = cfg.TenantID
		}
		if len(cfg.AdditionallyAllowedTenants) > 0 {
			cliOpts.AdditionallyAllowedTenants = cfg.AdditionallyAllowedTenants
		}
		return azidentity.NewAzureCLICredential(cliOpts)

	case AuthMethodEnvironment:
		envOpts := &azidentity.EnvironmentCredentialOptions{}
		if cfg.DisableInstanceDiscovery {
			envOpts.DisableInstanceDiscovery = cfg.DisableInstanceDiscovery
		}
		return azidentity.NewEnvironmentCredential(envOpts)

	case AuthMethodWorkloadIdentity:
		wiOpts := &azidentity.WorkloadIdentityCredentialOptions{}
		if cfg.TenantID != "" {
			wiOpts.TenantID = cfg.TenantID
		}
		if cfg.ClientID != "" {
			wiOpts.ClientID = cfg.ClientID
		}
		if cfg.DisableInstanceDiscovery {
			wiOpts.DisableInstanceDiscovery = cfg.DisableInstanceDiscovery
		}
		if len(cfg.AdditionallyAllowedTenants) > 0 {
			wiOpts.AdditionallyAllowedTenants = cfg.AdditionallyAllowedTenants
		}
		return azidentity.NewWorkloadIdentityCredential(wiOpts)

	default:
		return nil, fmt.Errorf("unsupported auth method: %s", cfg.AuthMethod)
	}
}

// buildClientOptions builds common client options.
func buildClientOptions(cfg *ClientConfig) azcore.ClientOptions {
	opts := azcore.ClientOptions{}

	if cfg.Timeout > 0 {
		opts.Transport = &http.Client{
			Timeout: cfg.Timeout,
		}
	}

	if cfg.RetryOptions != nil {
		opts.Retry = policy.RetryOptions{
			MaxRetries:    int32(cfg.RetryOptions.MaxRetries),
			RetryDelay:    cfg.RetryOptions.RetryDelay,
			MaxRetryDelay: cfg.RetryOptions.MaxRetryDelay,
		}
	}

	return opts
}

// SecretValue represents a secret value from Azure Key Vault.
type SecretValue struct {
	// Value is the secret value.
	Value string `json:"value"`

	// ID is the full secret identifier URL.
	ID string `json:"id"`

	// Name is the secret name.
	Name string `json:"name"`

	// Version is the secret version.
	Version string `json:"version"`

	// ContentType is the content type of the secret.
	ContentType string `json:"content_type,omitempty"`

	// Tags are user-defined key-value pairs.
	Tags map[string]string `json:"tags,omitempty"`

	// Attributes contains secret attributes.
	Attributes *SecretAttributes `json:"attributes,omitempty"`
}

// SecretAttributes contains secret attributes.
type SecretAttributes struct {
	// Enabled indicates if the secret is enabled.
	Enabled bool `json:"enabled"`

	// Created is when the secret was created.
	Created time.Time `json:"created,omitempty"`

	// Updated is when the secret was last updated.
	Updated time.Time `json:"updated,omitempty"`

	// NotBefore is the not-before time.
	NotBefore time.Time `json:"not_before,omitempty"`

	// Expires is the expiration time.
	Expires time.Time `json:"expires,omitempty"`

	// RecoveryLevel is the recovery level.
	RecoveryLevel string `json:"recovery_level,omitempty"`

	// RecoverableDays is the number of days the secret can be recovered.
	RecoverableDays int `json:"recoverable_days,omitempty"`
}

// IsExpired returns true if the secret has expired.
func (s *SecretValue) IsExpired() bool {
	if s.Attributes == nil || s.Attributes.Expires.IsZero() {
		return false
	}
	return time.Now().After(s.Attributes.Expires)
}

// IsActive returns true if the secret is currently active.
func (s *SecretValue) IsActive() bool {
	if s.Attributes == nil {
		return true
	}
	if !s.Attributes.Enabled {
		return false
	}
	now := time.Now()
	if !s.Attributes.NotBefore.IsZero() && now.Before(s.Attributes.NotBefore) {
		return false
	}
	if !s.Attributes.Expires.IsZero() && now.After(s.Attributes.Expires) {
		return false
	}
	return true
}

// GetJSON parses the secret value as JSON into the provided interface.
func (s *SecretValue) GetJSON(v interface{}) error {
	return json.Unmarshal([]byte(s.Value), v)
}

// GetMap parses the secret value as a JSON object and returns it as a map.
func (s *SecretValue) GetMap() (map[string]interface{}, error) {
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(s.Value), &result); err != nil {
		return nil, err
	}
	return result, nil
}

// SecretVersionInfo represents information about a secret version.
type SecretVersionInfo struct {
	// ID is the full secret identifier URL.
	ID string `json:"id"`

	// Version is the version identifier.
	Version string `json:"version"`

	// Attributes contains version attributes.
	Attributes *SecretAttributes `json:"attributes,omitempty"`

	// Tags are user-defined key-value pairs.
	Tags map[string]string `json:"tags,omitempty"`

	// ContentType is the content type.
	ContentType string `json:"content_type,omitempty"`
}

// SecretListEntry represents a secret in the list.
type SecretListEntry struct {
	// ID is the full secret identifier URL.
	ID string `json:"id"`

	// Name is the secret name.
	Name string `json:"name"`

	// Attributes contains secret attributes.
	Attributes *SecretAttributes `json:"attributes,omitempty"`

	// Tags are user-defined key-value pairs.
	Tags map[string]string `json:"tags,omitempty"`

	// ContentType is the content type.
	ContentType string `json:"content_type,omitempty"`
}

// KeyInfo represents information about a key.
type KeyInfo struct {
	// ID is the full key identifier URL.
	ID string `json:"id"`

	// Name is the key name.
	Name string `json:"name"`

	// Version is the key version.
	Version string `json:"version"`

	// KeyType is the type of key (RSA, EC, oct, etc.).
	KeyType string `json:"key_type"`

	// KeyOps are the permitted key operations.
	KeyOps []string `json:"key_ops,omitempty"`

	// Attributes contains key attributes.
	Attributes *KeyAttributes `json:"attributes,omitempty"`

	// Tags are user-defined key-value pairs.
	Tags map[string]string `json:"tags,omitempty"`
}

// KeyAttributes contains key attributes.
type KeyAttributes struct {
	// Enabled indicates if the key is enabled.
	Enabled bool `json:"enabled"`

	// Created is when the key was created.
	Created time.Time `json:"created,omitempty"`

	// Updated is when the key was last updated.
	Updated time.Time `json:"updated,omitempty"`

	// NotBefore is the not-before time.
	NotBefore time.Time `json:"not_before,omitempty"`

	// Expires is the expiration time.
	Expires time.Time `json:"expires,omitempty"`

	// RecoveryLevel is the recovery level.
	RecoveryLevel string `json:"recovery_level,omitempty"`

	// Exportable indicates if the key can be exported.
	Exportable bool `json:"exportable,omitempty"`
}

// CertificateInfo represents information about a certificate.
type CertificateInfo struct {
	// ID is the full certificate identifier URL.
	ID string `json:"id"`

	// Name is the certificate name.
	Name string `json:"name"`

	// Version is the certificate version.
	Version string `json:"version"`

	// Thumbprint is the certificate thumbprint.
	Thumbprint string `json:"thumbprint,omitempty"`

	// X509Thumbprint is the X.509 thumbprint.
	X509Thumbprint string `json:"x509_thumbprint,omitempty"`

	// SecretID is the ID of the secret containing the certificate.
	SecretID string `json:"secret_id,omitempty"`

	// KeyID is the ID of the key backing the certificate.
	KeyID string `json:"key_id,omitempty"`

	// Attributes contains certificate attributes.
	Attributes *CertificateAttributes `json:"attributes,omitempty"`

	// Tags are user-defined key-value pairs.
	Tags map[string]string `json:"tags,omitempty"`
}

// CertificateAttributes contains certificate attributes.
type CertificateAttributes struct {
	// Enabled indicates if the certificate is enabled.
	Enabled bool `json:"enabled"`

	// Created is when the certificate was created.
	Created time.Time `json:"created,omitempty"`

	// Updated is when the certificate was last updated.
	Updated time.Time `json:"updated,omitempty"`

	// NotBefore is the not-before time.
	NotBefore time.Time `json:"not_before,omitempty"`

	// Expires is the expiration time.
	Expires time.Time `json:"expires,omitempty"`

	// RecoveryLevel is the recovery level.
	RecoveryLevel string `json:"recovery_level,omitempty"`
}

// GetSecret retrieves a secret from Azure Key Vault.
func (c *Client) GetSecret(ctx context.Context, name string, opts ...GetSecretOption) (*SecretValue, error) {
	options := &getSecretOptions{}
	for _, opt := range opts {
		opt(options)
	}

	resp, err := c.secretClient.GetSecret(ctx, name, options.version, nil)
	if err != nil {
		return nil, translateError(err)
	}

	return secretFromResponse(&resp), nil
}

// GetSecretOption configures GetSecret behavior.
type GetSecretOption func(*getSecretOptions)

type getSecretOptions struct {
	version string
}

// WithVersion retrieves a specific version of the secret.
func WithVersion(version string) GetSecretOption {
	return func(o *getSecretOptions) {
		o.version = version
	}
}

// ListSecrets lists secrets in the vault.
func (c *Client) ListSecrets(ctx context.Context) ([]*SecretListEntry, error) {
	var entries []*SecretListEntry

	pager := c.secretClient.NewListSecretPropertiesPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, translateError(err)
		}

		for _, item := range page.Value {
			entry := &SecretListEntry{}
			if item.ID != nil {
				entry.ID = string(*item.ID)
				entry.Name = extractNameFromID(entry.ID)
			}
			if item.ContentType != nil {
				entry.ContentType = *item.ContentType
			}
			if item.Tags != nil {
				entry.Tags = make(map[string]string)
				for k, v := range item.Tags {
					if v != nil {
						entry.Tags[k] = *v
					}
				}
			}
			if item.Attributes != nil {
				entry.Attributes = attributesFromSecret(item.Attributes)
			}
			entries = append(entries, entry)
		}
	}

	return entries, nil
}

// ListSecretVersions lists all versions of a secret.
func (c *Client) ListSecretVersions(ctx context.Context, name string) ([]*SecretVersionInfo, error) {
	var versions []*SecretVersionInfo

	pager := c.secretClient.NewListSecretPropertiesVersionsPager(name, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, translateError(err)
		}

		for _, item := range page.Value {
			version := &SecretVersionInfo{}
			if item.ID != nil {
				version.ID = string(*item.ID)
				version.Version = extractVersionFromID(version.ID)
			}
			if item.ContentType != nil {
				version.ContentType = *item.ContentType
			}
			if item.Tags != nil {
				version.Tags = make(map[string]string)
				for k, v := range item.Tags {
					if v != nil {
						version.Tags[k] = *v
					}
				}
			}
			if item.Attributes != nil {
				version.Attributes = attributesFromSecret(item.Attributes)
			}
			versions = append(versions, version)
		}
	}

	return versions, nil
}

// GetDeletedSecret retrieves a soft-deleted secret.
func (c *Client) GetDeletedSecret(ctx context.Context, name string) (*SecretValue, error) {
	resp, err := c.secretClient.GetDeletedSecret(ctx, name, nil)
	if err != nil {
		return nil, translateError(err)
	}

	sv := &SecretValue{}
	if resp.ID != nil {
		sv.ID = string(*resp.ID)
		sv.Name = extractNameFromID(sv.ID)
		sv.Version = extractVersionFromID(sv.ID)
	}
	if resp.Value != nil {
		sv.Value = *resp.Value
	}
	if resp.ContentType != nil {
		sv.ContentType = *resp.ContentType
	}
	if resp.Tags != nil {
		sv.Tags = make(map[string]string)
		for k, v := range resp.Tags {
			if v != nil {
				sv.Tags[k] = *v
			}
		}
	}
	if resp.Attributes != nil {
		sv.Attributes = attributesFromSecret(resp.Attributes)
	}

	return sv, nil
}

// ListDeletedSecrets lists soft-deleted secrets.
func (c *Client) ListDeletedSecrets(ctx context.Context) ([]*SecretListEntry, error) {
	var entries []*SecretListEntry

	pager := c.secretClient.NewListDeletedSecretPropertiesPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, translateError(err)
		}

		for _, item := range page.Value {
			entry := &SecretListEntry{}
			if item.ID != nil {
				entry.ID = string(*item.ID)
				entry.Name = extractNameFromID(entry.ID)
			}
			if item.ContentType != nil {
				entry.ContentType = *item.ContentType
			}
			if item.Tags != nil {
				entry.Tags = make(map[string]string)
				for k, v := range item.Tags {
					if v != nil {
						entry.Tags[k] = *v
					}
				}
			}
			if item.Attributes != nil {
				entry.Attributes = attributesFromSecret(item.Attributes)
			}
			entries = append(entries, entry)
		}
	}

	return entries, nil
}

// RecoverDeletedSecret recovers a soft-deleted secret.
func (c *Client) RecoverDeletedSecret(ctx context.Context, name string) (*SecretValue, error) {
	resp, err := c.secretClient.RecoverDeletedSecret(ctx, name, nil)
	if err != nil {
		return nil, translateError(err)
	}

	return secretFromRecoveryResponse(&resp), nil
}

// PurgeDeletedSecret permanently deletes a soft-deleted secret.
func (c *Client) PurgeDeletedSecret(ctx context.Context, name string) error {
	_, err := c.secretClient.PurgeDeletedSecret(ctx, name, nil)
	if err != nil {
		return translateError(err)
	}
	return nil
}

// GetKey retrieves a key from Azure Key Vault.
func (c *Client) GetKey(ctx context.Context, name string, opts ...GetKeyOption) (*KeyInfo, error) {
	options := &getKeyOptions{}
	for _, opt := range opts {
		opt(options)
	}

	resp, err := c.keyClient.GetKey(ctx, name, options.version, nil)
	if err != nil {
		return nil, translateError(err)
	}

	return keyFromResponse(&resp), nil
}

// GetKeyOption configures GetKey behavior.
type GetKeyOption func(*getKeyOptions)

type getKeyOptions struct {
	version string
}

// WithKeyVersion retrieves a specific version of the key.
func WithKeyVersion(version string) GetKeyOption {
	return func(o *getKeyOptions) {
		o.version = version
	}
}

// ListKeys lists keys in the vault.
func (c *Client) ListKeys(ctx context.Context) ([]*KeyInfo, error) {
	var keys []*KeyInfo

	pager := c.keyClient.NewListKeyPropertiesPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, translateError(err)
		}

		for _, item := range page.Value {
			key := &KeyInfo{}
			if item.KID != nil {
				key.ID = string(*item.KID)
				key.Name = extractNameFromID(key.ID)
			}
			if item.Tags != nil {
				key.Tags = make(map[string]string)
				for k, v := range item.Tags {
					if v != nil {
						key.Tags[k] = *v
					}
				}
			}
			if item.Attributes != nil {
				key.Attributes = attributesFromKey(item.Attributes)
			}
			keys = append(keys, key)
		}
	}

	return keys, nil
}

// ListKeyVersions lists all versions of a key.
func (c *Client) ListKeyVersions(ctx context.Context, name string) ([]*KeyInfo, error) {
	var versions []*KeyInfo

	pager := c.keyClient.NewListKeyPropertiesVersionsPager(name, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, translateError(err)
		}

		for _, item := range page.Value {
			key := &KeyInfo{}
			if item.KID != nil {
				key.ID = string(*item.KID)
				key.Name = extractNameFromID(key.ID)
				key.Version = extractVersionFromID(key.ID)
			}
			if item.Tags != nil {
				key.Tags = make(map[string]string)
				for k, v := range item.Tags {
					if v != nil {
						key.Tags[k] = *v
					}
				}
			}
			if item.Attributes != nil {
				key.Attributes = attributesFromKey(item.Attributes)
			}
			versions = append(versions, key)
		}
	}

	return versions, nil
}

// Encrypt encrypts data using a key.
func (c *Client) Encrypt(ctx context.Context, keyName, keyVersion string, algorithm azkeys.EncryptionAlgorithm, plaintext []byte) ([]byte, error) {
	params := azkeys.KeyOperationParameters{
		Algorithm: &algorithm,
		Value:     plaintext,
	}

	resp, err := c.keyClient.Encrypt(ctx, keyName, keyVersion, params, nil)
	if err != nil {
		return nil, translateError(err)
	}

	return resp.Result, nil
}

// Decrypt decrypts data using a key.
func (c *Client) Decrypt(ctx context.Context, keyName, keyVersion string, algorithm azkeys.EncryptionAlgorithm, ciphertext []byte) ([]byte, error) {
	params := azkeys.KeyOperationParameters{
		Algorithm: &algorithm,
		Value:     ciphertext,
	}

	resp, err := c.keyClient.Decrypt(ctx, keyName, keyVersion, params, nil)
	if err != nil {
		return nil, translateError(err)
	}

	return resp.Result, nil
}

// Sign signs data using a key.
func (c *Client) Sign(ctx context.Context, keyName, keyVersion string, algorithm azkeys.SignatureAlgorithm, digest []byte) ([]byte, error) {
	params := azkeys.SignParameters{
		Algorithm: &algorithm,
		Value:     digest,
	}

	resp, err := c.keyClient.Sign(ctx, keyName, keyVersion, params, nil)
	if err != nil {
		return nil, translateError(err)
	}

	return resp.Result, nil
}

// Verify verifies a signature using a key.
func (c *Client) Verify(ctx context.Context, keyName, keyVersion string, algorithm azkeys.SignatureAlgorithm, digest, signature []byte) (bool, error) {
	params := azkeys.VerifyParameters{
		Algorithm: &algorithm,
		Digest:    digest,
		Signature: signature,
	}

	resp, err := c.keyClient.Verify(ctx, keyName, keyVersion, params, nil)
	if err != nil {
		return false, translateError(err)
	}

	if resp.Value == nil {
		return false, nil
	}
	return *resp.Value, nil
}

// WrapKey wraps a key using another key.
func (c *Client) WrapKey(ctx context.Context, keyName, keyVersion string, algorithm azkeys.EncryptionAlgorithm, key []byte) ([]byte, error) {
	params := azkeys.KeyOperationParameters{
		Algorithm: &algorithm,
		Value:     key,
	}

	resp, err := c.keyClient.WrapKey(ctx, keyName, keyVersion, params, nil)
	if err != nil {
		return nil, translateError(err)
	}

	return resp.Result, nil
}

// UnwrapKey unwraps a key using another key.
func (c *Client) UnwrapKey(ctx context.Context, keyName, keyVersion string, algorithm azkeys.EncryptionAlgorithm, wrappedKey []byte) ([]byte, error) {
	params := azkeys.KeyOperationParameters{
		Algorithm: &algorithm,
		Value:     wrappedKey,
	}

	resp, err := c.keyClient.UnwrapKey(ctx, keyName, keyVersion, params, nil)
	if err != nil {
		return nil, translateError(err)
	}

	return resp.Result, nil
}

// GetCertificate retrieves a certificate from Azure Key Vault.
func (c *Client) GetCertificate(ctx context.Context, name string, opts ...GetCertificateOption) (*CertificateInfo, error) {
	options := &getCertificateOptions{}
	for _, opt := range opts {
		opt(options)
	}

	resp, err := c.certClient.GetCertificate(ctx, name, options.version, nil)
	if err != nil {
		return nil, translateError(err)
	}

	return certificateFromResponse(&resp), nil
}

// GetCertificateOption configures GetCertificate behavior.
type GetCertificateOption func(*getCertificateOptions)

type getCertificateOptions struct {
	version string
}

// WithCertificateVersion retrieves a specific version of the certificate.
func WithCertificateVersion(version string) GetCertificateOption {
	return func(o *getCertificateOptions) {
		o.version = version
	}
}

// ListCertificates lists certificates in the vault.
func (c *Client) ListCertificates(ctx context.Context) ([]*CertificateInfo, error) {
	var certs []*CertificateInfo

	pager := c.certClient.NewListCertificatePropertiesPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, translateError(err)
		}

		for _, item := range page.Value {
			cert := &CertificateInfo{}
			if item.ID != nil {
				cert.ID = string(*item.ID)
				cert.Name = extractNameFromID(cert.ID)
			}
			if item.X509Thumbprint != nil {
				cert.X509Thumbprint = base64.URLEncoding.EncodeToString(item.X509Thumbprint)
			}
			if item.Tags != nil {
				cert.Tags = make(map[string]string)
				for k, v := range item.Tags {
					if v != nil {
						cert.Tags[k] = *v
					}
				}
			}
			if item.Attributes != nil {
				cert.Attributes = attributesFromCertificate(item.Attributes)
			}
			certs = append(certs, cert)
		}
	}

	return certs, nil
}

// ListCertificateVersions lists all versions of a certificate.
func (c *Client) ListCertificateVersions(ctx context.Context, name string) ([]*CertificateInfo, error) {
	var versions []*CertificateInfo

	pager := c.certClient.NewListCertificatePropertiesVersionsPager(name, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, translateError(err)
		}

		for _, item := range page.Value {
			cert := &CertificateInfo{}
			if item.ID != nil {
				cert.ID = string(*item.ID)
				cert.Name = extractNameFromID(cert.ID)
				cert.Version = extractVersionFromID(cert.ID)
			}
			if item.X509Thumbprint != nil {
				cert.X509Thumbprint = base64.URLEncoding.EncodeToString(item.X509Thumbprint)
			}
			if item.Tags != nil {
				cert.Tags = make(map[string]string)
				for k, v := range item.Tags {
					if v != nil {
						cert.Tags[k] = *v
					}
				}
			}
			if item.Attributes != nil {
				cert.Attributes = attributesFromCertificate(item.Attributes)
			}
			versions = append(versions, cert)
		}
	}

	return versions, nil
}

// Close closes the client.
func (c *Client) Close() error {
	// Azure SDK clients don't need explicit cleanup
	return nil
}

// Helper functions

func secretFromResponse(resp *azsecrets.GetSecretResponse) *SecretValue {
	sv := &SecretValue{}
	if resp.ID != nil {
		sv.ID = string(*resp.ID)
		sv.Name = extractNameFromID(sv.ID)
		sv.Version = extractVersionFromID(sv.ID)
	}
	if resp.Value != nil {
		sv.Value = *resp.Value
	}
	if resp.ContentType != nil {
		sv.ContentType = *resp.ContentType
	}
	if resp.Tags != nil {
		sv.Tags = make(map[string]string)
		for k, v := range resp.Tags {
			if v != nil {
				sv.Tags[k] = *v
			}
		}
	}
	if resp.Attributes != nil {
		sv.Attributes = attributesFromSecret(resp.Attributes)
	}
	return sv
}

func secretFromRecoveryResponse(resp *azsecrets.RecoverDeletedSecretResponse) *SecretValue {
	sv := &SecretValue{}
	if resp.ID != nil {
		sv.ID = string(*resp.ID)
		sv.Name = extractNameFromID(sv.ID)
		sv.Version = extractVersionFromID(sv.ID)
	}
	if resp.ContentType != nil {
		sv.ContentType = *resp.ContentType
	}
	if resp.Tags != nil {
		sv.Tags = make(map[string]string)
		for k, v := range resp.Tags {
			if v != nil {
				sv.Tags[k] = *v
			}
		}
	}
	if resp.Attributes != nil {
		sv.Attributes = attributesFromSecret(resp.Attributes)
	}
	return sv
}

func attributesFromSecret(attrs *azsecrets.SecretAttributes) *SecretAttributes {
	sa := &SecretAttributes{}
	if attrs.Enabled != nil {
		sa.Enabled = *attrs.Enabled
	}
	if attrs.Created != nil {
		sa.Created = *attrs.Created
	}
	if attrs.Updated != nil {
		sa.Updated = *attrs.Updated
	}
	if attrs.NotBefore != nil {
		sa.NotBefore = *attrs.NotBefore
	}
	if attrs.Expires != nil {
		sa.Expires = *attrs.Expires
	}
	if attrs.RecoveryLevel != nil {
		sa.RecoveryLevel = string(*attrs.RecoveryLevel)
	}
	if attrs.RecoverableDays != nil {
		sa.RecoverableDays = int(*attrs.RecoverableDays)
	}
	return sa
}

func keyFromResponse(resp *azkeys.GetKeyResponse) *KeyInfo {
	ki := &KeyInfo{}
	if resp.Key != nil && resp.Key.KID != nil {
		ki.ID = string(*resp.Key.KID)
		ki.Name = extractNameFromID(ki.ID)
		ki.Version = extractVersionFromID(ki.ID)
		if resp.Key.Kty != nil {
			ki.KeyType = string(*resp.Key.Kty)
		}
		if resp.Key.KeyOps != nil {
			for _, op := range resp.Key.KeyOps {
				if op != nil {
					ki.KeyOps = append(ki.KeyOps, string(*op))
				}
			}
		}
	}
	if resp.Tags != nil {
		ki.Tags = make(map[string]string)
		for k, v := range resp.Tags {
			if v != nil {
				ki.Tags[k] = *v
			}
		}
	}
	if resp.Attributes != nil {
		ki.Attributes = attributesFromKey(resp.Attributes)
	}
	return ki
}

func attributesFromKey(attrs *azkeys.KeyAttributes) *KeyAttributes {
	ka := &KeyAttributes{}
	if attrs.Enabled != nil {
		ka.Enabled = *attrs.Enabled
	}
	if attrs.Created != nil {
		ka.Created = *attrs.Created
	}
	if attrs.Updated != nil {
		ka.Updated = *attrs.Updated
	}
	if attrs.NotBefore != nil {
		ka.NotBefore = *attrs.NotBefore
	}
	if attrs.Expires != nil {
		ka.Expires = *attrs.Expires
	}
	if attrs.RecoveryLevel != nil {
		ka.RecoveryLevel = string(*attrs.RecoveryLevel)
	}
	if attrs.Exportable != nil {
		ka.Exportable = *attrs.Exportable
	}
	return ka
}

func certificateFromResponse(resp *azcertificates.GetCertificateResponse) *CertificateInfo {
	ci := &CertificateInfo{}
	if resp.ID != nil {
		ci.ID = string(*resp.ID)
		ci.Name = extractNameFromID(ci.ID)
		ci.Version = extractVersionFromID(ci.ID)
	}
	if resp.X509Thumbprint != nil {
		ci.X509Thumbprint = base64.URLEncoding.EncodeToString(resp.X509Thumbprint)
	}
	if resp.KID != nil {
		ci.KeyID = string(*resp.KID)
	}
	if resp.SID != nil {
		ci.SecretID = string(*resp.SID)
	}
	if resp.Tags != nil {
		ci.Tags = make(map[string]string)
		for k, v := range resp.Tags {
			if v != nil {
				ci.Tags[k] = *v
			}
		}
	}
	if resp.Attributes != nil {
		ci.Attributes = attributesFromCertificate(resp.Attributes)
	}
	return ci
}

func attributesFromCertificate(attrs *azcertificates.CertificateAttributes) *CertificateAttributes {
	ca := &CertificateAttributes{}
	if attrs.Enabled != nil {
		ca.Enabled = *attrs.Enabled
	}
	if attrs.Created != nil {
		ca.Created = *attrs.Created
	}
	if attrs.Updated != nil {
		ca.Updated = *attrs.Updated
	}
	if attrs.NotBefore != nil {
		ca.NotBefore = *attrs.NotBefore
	}
	if attrs.Expires != nil {
		ca.Expires = *attrs.Expires
	}
	if attrs.RecoveryLevel != nil {
		ca.RecoveryLevel = string(*attrs.RecoveryLevel)
	}
	return ca
}

// extractNameFromID extracts the name from an Azure Key Vault ID URL.
// Example: https://myvault.vault.azure.net/secrets/mysecret/version -> mysecret
// Example: https://myvault.vault.azure.net/secrets/mysecret -> mysecret
func extractNameFromID(id string) string {
	parts := strings.Split(id, "/")
	// Find the type indicator (secrets, keys, certificates)
	for i, part := range parts {
		if part == "secrets" || part == "keys" || part == "certificates" {
			if i+1 < len(parts) {
				return parts[i+1]
			}
		}
	}
	return ""
}

// extractVersionFromID extracts the version from an Azure Key Vault ID URL.
// Example: https://myvault.vault.azure.net/secrets/mysecret/abc123 -> abc123
// Example: https://myvault.vault.azure.net/secrets/mysecret -> "" (no version)
func extractVersionFromID(id string) string {
	parts := strings.Split(id, "/")
	// Find the type indicator (secrets, keys, certificates)
	for i, part := range parts {
		if part == "secrets" || part == "keys" || part == "certificates" {
			// Version is 2 positions after the type indicator
			if i+2 < len(parts) {
				return parts[i+2]
			}
		}
	}
	return ""
}

// translateError translates Azure errors to secrets package errors.
func translateError(err error) error {
	if err == nil {
		return nil
	}

	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) {
		switch respErr.StatusCode {
		case http.StatusNotFound:
			return secrets.ErrSecretNotFound
		case http.StatusForbidden, http.StatusUnauthorized:
			return secrets.ErrAccessDenied
		case http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return secrets.ErrBackendUnavailable
		case http.StatusBadRequest:
			return fmt.Errorf("%w: %s", secrets.ErrInvalidPath, respErr.Error())
		}
	}

	return err
}
