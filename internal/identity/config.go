package identity

import (
	"fmt"
	"time"
)

// Config is the configuration for the identity system.
type Config struct {
	// Enabled enables the identity system. Default: true.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Provider specifies the identity provider to use.
	Provider ProviderConfig `yaml:"provider" json:"provider"`

	// TrustDomain is the SPIFFE trust domain.
	// For embedded provider, defaults to "kscore.local" if not set.
	TrustDomain string `yaml:"trust_domain" json:"trust_domain"`

	// SVID contains SVID-related configuration.
	SVID SVIDConfig `yaml:"svid" json:"svid"`

	// Attestation contains attestation-related configuration.
	Attestation AttestationConfig `yaml:"attestation" json:"attestation"`

	// CA contains CA-related configuration (for embedded provider).
	CA CAConfig `yaml:"ca" json:"ca"`

	// NATS contains NATS mTLS integration configuration.
	NATS NATSIdentityConfig `yaml:"nats" json:"nats"`

	// Federation contains trust federation configuration.
	Federation FederationConfig `yaml:"federation" json:"federation"`
}

// ProviderConfig configures the identity provider.
type ProviderConfig struct {
	// Type is the provider type. Default: "embedded".
	Type ProviderType `yaml:"type" json:"type"`

	// SPIRE contains SPIRE-specific configuration.
	SPIRE *SPIREConfig `yaml:"spire,omitempty" json:"spire,omitempty"`

	// AWS contains AWS-specific configuration.
	AWS *AWSIdentityConfig `yaml:"aws,omitempty" json:"aws,omitempty"`

	// GCP contains GCP-specific configuration.
	GCP *GCPIdentityConfig `yaml:"gcp,omitempty" json:"gcp,omitempty"`

	// Azure contains Azure-specific configuration.
	Azure *AzureIdentityConfig `yaml:"azure,omitempty" json:"azure,omitempty"`
}

// SVIDConfig contains SVID-related configuration.
type SVIDConfig struct {
	// DefaultTTL is the default SVID time-to-live. Default: 1h.
	DefaultTTL time.Duration `yaml:"default_ttl" json:"default_ttl"`

	// MaxTTL is the maximum allowed SVID TTL. Default: 24h.
	MaxTTL time.Duration `yaml:"max_ttl" json:"max_ttl"`

	// RotationInterval is how often to check for SVID rotation. Default: 30s.
	RotationInterval time.Duration `yaml:"rotation_interval" json:"rotation_interval"`

	// GracePeriod is how long to keep serving with an expired SVID. Default: 10m.
	GracePeriod time.Duration `yaml:"grace_period" json:"grace_period"`

	// IncludeDNSNames includes the agent hostname as a DNS SAN. Default: true.
	IncludeDNSNames bool `yaml:"include_dns_names" json:"include_dns_names"`

	// IncludeIPAddresses includes the agent IP addresses as SANs. Default: true.
	IncludeIPAddresses bool `yaml:"include_ip_addresses" json:"include_ip_addresses"`
}

// AttestationConfig contains attestation-related configuration.
type AttestationConfig struct {
	// AllowedAttestors lists which attestors are enabled. Default: ["join_token"].
	AllowedAttestors []string `yaml:"allowed_attestors" json:"allowed_attestors"`

	// AllowNone allows the "none" attestor (dev only). Default: false.
	AllowNone bool `yaml:"allow_none" json:"allow_none"`

	// EnableFallback enables automatic fallback to alternative attestors when
	// the primary attestation method fails. When enabled and attestation fails,
	// the engine will try each attestor in FallbackOrder until one succeeds.
	// Default: false.
	EnableFallback bool `yaml:"enable_fallback" json:"enable_fallback"`

	// FallbackOrder specifies the order in which to try attestors when using
	// auto-detection (type="auto") or when EnableFallback is true and the
	// primary attestor fails. If empty, uses AllowedAttestors order.
	// Example: ["k8s_sat", "aws_iid", "gcp_iit", "azure_imds", "join_token"]
	FallbackOrder []string `yaml:"fallback_order,omitempty" json:"fallback_order,omitempty"`

	// JoinToken contains join token attestor configuration.
	JoinToken JoinTokenConfig `yaml:"join_token" json:"join_token"`

	// AWS contains AWS attestor configuration.
	AWS *AWSAttestorConfig `yaml:"aws,omitempty" json:"aws,omitempty"`

	// GCP contains GCP attestor configuration.
	GCP *GCPAttestorConfig `yaml:"gcp,omitempty" json:"gcp,omitempty"`

	// Azure contains Azure attestor configuration.
	Azure *AzureAttestorConfig `yaml:"azure,omitempty" json:"azure,omitempty"`

	// K8s contains Kubernetes attestor configuration.
	K8s *K8sAttestorConfig `yaml:"k8s,omitempty" json:"k8s,omitempty"`
}

// JoinTokenConfig contains join token attestor configuration.
type JoinTokenConfig struct {
	// Enabled enables the join token attestor. Default: true.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// DefaultTTL is the default token TTL. Default: 5m.
	DefaultTTL time.Duration `yaml:"default_ttl" json:"default_ttl"`

	// MaxTTL is the maximum token TTL. Default: 24h.
	MaxTTL time.Duration `yaml:"max_ttl" json:"max_ttl"`

	// TokenLength is the length of generated tokens. Default: 32.
	TokenLength int `yaml:"token_length" json:"token_length"`

	// OneTimeUse makes tokens single-use. Default: true.
	OneTimeUse bool `yaml:"one_time_use" json:"one_time_use"`

	// CleanupInterval is how often to clean up expired tokens. Default: 1h.
	CleanupInterval time.Duration `yaml:"cleanup_interval" json:"cleanup_interval"`
}

// CAConfig contains CA-related configuration for the embedded provider.
type CAConfig struct {
	// KeyType is the key algorithm. Default: "ecdsa-p256".
	// Options: "ecdsa-p256", "ecdsa-p384", "rsa-2048", "rsa-4096".
	KeyType string `yaml:"key_type" json:"key_type"`

	// RootCATTL is the root CA certificate TTL. Default: 10 years.
	RootCATTL time.Duration `yaml:"root_ca_ttl" json:"root_ca_ttl"`

	// SigningCATTL is the signing (intermediate) CA TTL. Default: 1 year.
	SigningCATTL time.Duration `yaml:"signing_ca_ttl" json:"signing_ca_ttl"`

	// RotateSigningCABefore rotates signing CA this long before expiry. Default: 30d.
	RotateSigningCABefore time.Duration `yaml:"rotate_signing_ca_before" json:"rotate_signing_ca_before"`

	// StoragePath is where CA keys are stored. Default: data/identity/ca.
	StoragePath string `yaml:"storage_path" json:"storage_path"`

	// EncryptionKey is the key for encrypting CA private keys at rest.
	// If empty, keys are stored unencrypted (not recommended for production).
	EncryptionKey string `yaml:"encryption_key" json:"encryption_key"`

	// EncryptionKeyFile is a file containing the encryption key.
	EncryptionKeyFile string `yaml:"encryption_key_file" json:"encryption_key_file"`

	// Subject contains the CA certificate subject fields.
	Subject CASubjectConfig `yaml:"subject" json:"subject"`
}

// CASubjectConfig contains CA certificate subject fields.
type CASubjectConfig struct {
	// Organization is the O field. Default: "Keystone Core".
	Organization string `yaml:"organization" json:"organization"`

	// OrganizationalUnit is the OU field. Default: "Identity".
	OrganizationalUnit string `yaml:"organizational_unit" json:"organizational_unit"`

	// Country is the C field. Default: empty.
	Country string `yaml:"country" json:"country"`

	// Province is the ST field. Default: empty.
	Province string `yaml:"province" json:"province"`

	// Locality is the L field. Default: empty.
	Locality string `yaml:"locality" json:"locality"`
}

// NATSIdentityConfig contains NATS mTLS integration configuration.
type NATSIdentityConfig struct {
	// Enabled enables NATS mTLS with SVIDs. Default: true.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// RequireMTLS requires mTLS for all NATS connections. Default: true.
	RequireMTLS bool `yaml:"require_mtls" json:"require_mtls"`

	// VerifyClientCert verifies client certificates. Default: true.
	VerifyClientCert bool `yaml:"verify_client_cert" json:"verify_client_cert"`

	// AllowedSPIFFEIDPrefixes restricts which SPIFFE IDs can connect.
	// Empty means all IDs in the trust domain are allowed.
	AllowedSPIFFEIDPrefixes []string `yaml:"allowed_spiffe_id_prefixes" json:"allowed_spiffe_id_prefixes"`
}

// FederationConfig contains trust federation configuration.
type FederationConfig struct {
	// Enabled enables trust federation. Default: false.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// TrustedDomains lists external trust domains to federate with.
	TrustedDomains []TrustedDomainConfig `yaml:"trusted_domains" json:"trusted_domains"`

	// BundleEndpoint exposes the trust bundle at this endpoint.
	BundleEndpoint string `yaml:"bundle_endpoint" json:"bundle_endpoint"`

	// RefreshInterval is how often to refresh external bundles. Default: 1h.
	RefreshInterval time.Duration `yaml:"refresh_interval" json:"refresh_interval"`
}

// TrustedDomainConfig configures a trusted external domain.
type TrustedDomainConfig struct {
	// TrustDomain is the external trust domain name.
	TrustDomain string `yaml:"trust_domain" json:"trust_domain"`

	// BundleEndpoint is the URL to fetch the trust bundle.
	BundleEndpoint string `yaml:"bundle_endpoint" json:"bundle_endpoint"`

	// BundleEndpointProfile is the profile for the endpoint. Default: "https_web".
	// Options: "https_web", "https_spiffe".
	BundleEndpointProfile string `yaml:"bundle_endpoint_profile" json:"bundle_endpoint_profile"`
}

// SPIREConfig contains SPIRE-specific configuration.
type SPIREConfig struct {
	// AgentSocketPath is the path to the SPIRE Agent socket.
	// Default: "/run/spire/agent/sockets/agent.sock".
	AgentSocketPath string `yaml:"agent_socket_path" json:"agent_socket_path"`

	// ServerAddress is the SPIRE Server address (for admin operations).
	ServerAddress string `yaml:"server_address" json:"server_address"`

	// TrustDomain is the SPIRE trust domain (must match SPIRE server).
	TrustDomain string `yaml:"trust_domain" json:"trust_domain"`
}

// AWSIdentityConfig contains AWS identity provider configuration.
type AWSIdentityConfig struct {
	// Region is the AWS region. If empty, uses instance metadata.
	Region string `yaml:"region" json:"region"`

	// RoleARN is the IAM role to assume (for IRSA).
	RoleARN string `yaml:"role_arn" json:"role_arn"`

	// UseIMDSv2 requires IMDSv2 for instance identity. Default: true.
	UseIMDSv2 bool `yaml:"use_imdsv2" json:"use_imdsv2"`
}

// GCPIdentityConfig contains GCP identity provider configuration.
type GCPIdentityConfig struct {
	// ProjectID is the GCP project ID.
	ProjectID string `yaml:"project_id" json:"project_id"`

	// ServiceAccountEmail is the service account to impersonate.
	ServiceAccountEmail string `yaml:"service_account_email" json:"service_account_email"`
}

// AzureIdentityConfig contains Azure identity provider configuration.
type AzureIdentityConfig struct {
	// TenantID is the Azure AD tenant ID.
	TenantID string `yaml:"tenant_id" json:"tenant_id"`

	// ClientID is the managed identity client ID.
	ClientID string `yaml:"client_id" json:"client_id"`
}

// AWSAttestorConfig contains AWS attestor configuration.
type AWSAttestorConfig struct {
	// Enabled enables the AWS attestor. Default: true (if provider is embedded).
	Enabled bool `yaml:"enabled" json:"enabled"`

	// AllowedAccountIDs restricts which AWS accounts can attest.
	AllowedAccountIDs []string `yaml:"allowed_account_ids" json:"allowed_account_ids"`

	// AllowedRegions restricts which regions can attest.
	AllowedRegions []string `yaml:"allowed_regions" json:"allowed_regions"`

	// AllowedInstanceTags restricts attestation by instance tags.
	AllowedInstanceTags map[string][]string `yaml:"allowed_instance_tags" json:"allowed_instance_tags"`
}

// GCPAttestorConfig contains GCP attestor configuration.
type GCPAttestorConfig struct {
	// Enabled enables the GCP attestor.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// AllowedProjectIDs restricts which projects can attest.
	AllowedProjectIDs []string `yaml:"allowed_project_ids" json:"allowed_project_ids"`

	// AllowedZones restricts which zones can attest.
	AllowedZones []string `yaml:"allowed_zones" json:"allowed_zones"`

	// AllowedServiceAccounts restricts which service accounts can attest.
	AllowedServiceAccounts []string `yaml:"allowed_service_accounts" json:"allowed_service_accounts"`
}

// AzureAttestorConfig contains Azure attestor configuration.
type AzureAttestorConfig struct {
	// Enabled enables the Azure attestor.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// AllowedSubscriptionIDs restricts which subscriptions can attest.
	AllowedSubscriptionIDs []string `yaml:"allowed_subscription_ids" json:"allowed_subscription_ids"`

	// AllowedResourceGroups restricts which resource groups can attest.
	AllowedResourceGroups []string `yaml:"allowed_resource_groups" json:"allowed_resource_groups"`
}

// K8sAttestorConfig contains Kubernetes attestor configuration.
type K8sAttestorConfig struct {
	// Enabled enables the Kubernetes attestor.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Audience is the expected audience for service account tokens.
	// Default: "kscore".
	Audience string `yaml:"audience" json:"audience"`

	// AllowedNamespaces restricts which namespaces can attest.
	AllowedNamespaces []string `yaml:"allowed_namespaces" json:"allowed_namespaces"`

	// AllowedServiceAccounts restricts which service accounts can attest.
	// Format: "namespace:service-account-name".
	AllowedServiceAccounts []string `yaml:"allowed_service_accounts" json:"allowed_service_accounts"`

	// ClusterName is used in the SPIFFE ID path.
	ClusterName string `yaml:"cluster_name" json:"cluster_name"`
}

// DefaultConfig returns the default identity configuration.
func DefaultConfig() *Config {
	return &Config{
		Enabled:     true,
		TrustDomain: "kscore.local",
		Provider: ProviderConfig{
			Type: ProviderTypeEmbedded,
		},
		SVID: SVIDConfig{
			DefaultTTL:         1 * time.Hour,
			MaxTTL:             24 * time.Hour,
			RotationInterval:   30 * time.Second,
			GracePeriod:        10 * time.Minute,
			IncludeDNSNames:    true,
			IncludeIPAddresses: true,
		},
		Attestation: AttestationConfig{
			AllowedAttestors: []string{AttestationTypeJoinToken},
			AllowNone:        false,
			JoinToken: JoinTokenConfig{
				Enabled:         true,
				DefaultTTL:      5 * time.Minute,
				MaxTTL:          24 * time.Hour,
				TokenLength:     32,
				OneTimeUse:      true,
				CleanupInterval: 1 * time.Hour,
			},
		},
		CA: CAConfig{
			KeyType:               "ecdsa-p256",
			RootCATTL:             10 * 365 * 24 * time.Hour, // 10 years
			SigningCATTL:          365 * 24 * time.Hour,      // 1 year
			RotateSigningCABefore: 30 * 24 * time.Hour,       // 30 days
			StoragePath:           "data/identity/ca",
			Subject: CASubjectConfig{
				Organization:       "Keystone Core",
				OrganizationalUnit: "Identity",
			},
		},
		NATS: NATSIdentityConfig{
			Enabled:          true,
			RequireMTLS:      true,
			VerifyClientCert: true,
		},
		Federation: FederationConfig{
			Enabled:         false,
			RefreshInterval: 1 * time.Hour,
		},
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil // No validation needed if disabled
	}

	if err := ValidateTrustDomain(c.TrustDomain); err != nil {
		return fmt.Errorf("invalid trust domain: %w", err)
	}

	if err := c.Provider.Validate(); err != nil {
		return fmt.Errorf("invalid provider config: %w", err)
	}

	if err := c.SVID.Validate(); err != nil {
		return fmt.Errorf("invalid SVID config: %w", err)
	}

	if err := c.Attestation.Validate(); err != nil {
		return fmt.Errorf("invalid attestation config: %w", err)
	}

	if c.Provider.Type == ProviderTypeEmbedded {
		if err := c.CA.Validate(); err != nil {
			return fmt.Errorf("invalid CA config: %w", err)
		}
	}

	return nil
}

// Validate validates the provider configuration.
func (c *ProviderConfig) Validate() error {
	switch c.Type {
	case ProviderTypeEmbedded:
		// No additional validation needed
	case ProviderTypeSPIRE:
		if c.SPIRE == nil {
			return fmt.Errorf("SPIRE config required for SPIRE provider")
		}
		if c.SPIRE.AgentSocketPath == "" {
			return fmt.Errorf("SPIRE agent socket path required")
		}
	case ProviderTypeAWS, ProviderTypeGCP, ProviderTypeAzure:
		// Cloud providers have optional config
	case ProviderTypeIstio, ProviderTypeConsul, ProviderTypeLinkerd:
		// Service mesh providers have optional config
	default:
		return fmt.Errorf("unknown provider type: %s", c.Type)
	}
	return nil
}

// Validate validates the SVID configuration.
func (c *SVIDConfig) Validate() error {
	if c.DefaultTTL <= 0 {
		return fmt.Errorf("default TTL must be positive")
	}
	if c.MaxTTL <= 0 {
		return fmt.Errorf("max TTL must be positive")
	}
	if c.DefaultTTL > c.MaxTTL {
		return fmt.Errorf("default TTL cannot exceed max TTL")
	}
	if c.RotationInterval <= 0 {
		return fmt.Errorf("rotation interval must be positive")
	}
	if c.GracePeriod < 0 {
		return fmt.Errorf("grace period cannot be negative")
	}
	return nil
}

// Validate validates the attestation configuration.
func (c *AttestationConfig) Validate() error {
	if len(c.AllowedAttestors) == 0 && !c.AllowNone {
		return fmt.Errorf("at least one attestor must be enabled")
	}

	for _, attestor := range c.AllowedAttestors {
		switch attestor {
		case AttestationTypeJoinToken:
			if err := c.JoinToken.Validate(); err != nil {
				return fmt.Errorf("invalid join token config: %w", err)
			}
		case AttestationTypeAWSIID, AttestationTypeGCPIIT, AttestationTypeAzureIMDS, AttestationTypeK8sSAT:
			// Cloud/K8s attestors don't require validation
		case AttestationTypeNone:
			if !c.AllowNone {
				return fmt.Errorf("none attestor not allowed")
			}
		default:
			return fmt.Errorf("unknown attestor: %s", attestor)
		}
	}
	return nil
}

// Validate validates the join token configuration.
func (c *JoinTokenConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.DefaultTTL <= 0 {
		return fmt.Errorf("default TTL must be positive")
	}
	if c.MaxTTL <= 0 {
		return fmt.Errorf("max TTL must be positive")
	}
	if c.DefaultTTL > c.MaxTTL {
		return fmt.Errorf("default TTL cannot exceed max TTL")
	}
	if c.TokenLength < 16 {
		return fmt.Errorf("token length must be at least 16")
	}
	return nil
}

// Validate validates the CA configuration.
func (c *CAConfig) Validate() error {
	switch c.KeyType {
	case "ecdsa-p256", "ecdsa-p384", "rsa-2048", "rsa-4096", "":
		// Valid key types
	default:
		return fmt.Errorf("invalid key type: %s", c.KeyType)
	}

	if c.RootCATTL <= 0 {
		return fmt.Errorf("root CA TTL must be positive")
	}
	if c.SigningCATTL <= 0 {
		return fmt.Errorf("signing CA TTL must be positive")
	}
	if c.SigningCATTL > c.RootCATTL {
		return fmt.Errorf("signing CA TTL cannot exceed root CA TTL")
	}
	if c.RotateSigningCABefore >= c.SigningCATTL {
		return fmt.Errorf("rotate signing CA before must be less than signing CA TTL")
	}

	return nil
}
