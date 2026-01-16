// Package identity provides SPIFFE-based workload identity for Keystone Core.
// It supports both an embedded identity provider (zero-config default) and
// integration with external SPIFFE-compatible systems (SPIRE, cloud, mesh).
package identity

import (
	"context"
	"crypto"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// SPIFFEID represents a SPIFFE identity in the format spiffe://trust-domain/path
type SPIFFEID struct {
	TrustDomain string
	Path        string
}

// String returns the full SPIFFE ID as a URI string.
func (id SPIFFEID) String() string {
	return fmt.Sprintf("spiffe://%s%s", id.TrustDomain, id.Path)
}

// URL returns the SPIFFE ID as a *url.URL.
func (id SPIFFEID) URL() *url.URL {
	return &url.URL{
		Scheme: "spiffe",
		Host:   id.TrustDomain,
		Path:   id.Path,
	}
}

// ParseSPIFFEID parses a SPIFFE ID string into a SPIFFEID struct.
func ParseSPIFFEID(s string) (SPIFFEID, error) {
	u, err := url.Parse(s)
	if err != nil {
		return SPIFFEID{}, fmt.Errorf("invalid SPIFFE ID: %w", err)
	}
	if u.Scheme != "spiffe" {
		return SPIFFEID{}, fmt.Errorf("invalid SPIFFE ID scheme: %s (expected spiffe)", u.Scheme)
	}
	if u.Host == "" {
		return SPIFFEID{}, fmt.Errorf("invalid SPIFFE ID: missing trust domain")
	}
	return SPIFFEID{
		TrustDomain: u.Host,
		Path:        u.Path,
	}, nil
}

// NewAgentSPIFFEID creates a SPIFFE ID for an agent.
func NewAgentSPIFFEID(trustDomain, agentID string) SPIFFEID {
	return SPIFFEID{
		TrustDomain: trustDomain,
		Path:        fmt.Sprintf("/agent/%s", agentID),
	}
}

// NewServerSPIFFEID creates a SPIFFE ID for a server.
func NewServerSPIFFEID(trustDomain, serverID string) SPIFFEID {
	return SPIFFEID{
		TrustDomain: trustDomain,
		Path:        fmt.Sprintf("/server/%s", serverID),
	}
}

// NewServiceSPIFFEID creates a SPIFFE ID for a service.
func NewServiceSPIFFEID(trustDomain, serviceName string) SPIFFEID {
	return SPIFFEID{
		TrustDomain: trustDomain,
		Path:        fmt.Sprintf("/service/%s", serviceName),
	}
}

// SVIDType represents the type of SVID (X.509 or JWT).
type SVIDType string

const (
	// SVIDX509 is an X.509 certificate-based SVID.
	SVIDX509 SVIDType = "x509"
	// SVIDJWT is a JWT-based SVID.
	SVIDJWT SVIDType = "jwt"
)

// X509SVID represents an X.509 SPIFFE Verifiable Identity Document.
type X509SVID struct {
	// SPIFFEID is the SPIFFE identity encoded in the certificate.
	SPIFFEID SPIFFEID

	// Certificate is the X.509 certificate chain (leaf first).
	Certificates []*x509.Certificate

	// PrivateKey is the private key for the leaf certificate.
	PrivateKey crypto.PrivateKey

	// ExpiresAt is when the SVID expires.
	ExpiresAt time.Time

	// IssuedAt is when the SVID was issued.
	IssuedAt time.Time

	// Hint is an optional hint for the SVID (e.g., for key rotation).
	Hint string
}

// Expired returns true if the SVID has expired.
func (s *X509SVID) Expired() bool {
	return time.Now().After(s.ExpiresAt)
}

// TimeUntilExpiry returns the duration until the SVID expires.
func (s *X509SVID) TimeUntilExpiry() time.Duration {
	return time.Until(s.ExpiresAt)
}

// ShouldRotate returns true if the SVID should be rotated (past halfway to expiry).
func (s *X509SVID) ShouldRotate() bool {
	lifetime := s.ExpiresAt.Sub(s.IssuedAt)
	halfLife := s.IssuedAt.Add(lifetime / 2)
	return time.Now().After(halfLife)
}

// JWTSVID represents a JWT-based SPIFFE Verifiable Identity Document.
type JWTSVID struct {
	// SPIFFEID is the SPIFFE identity encoded in the JWT.
	SPIFFEID SPIFFEID

	// Token is the signed JWT string.
	Token string

	// ExpiresAt is when the JWT expires.
	ExpiresAt time.Time

	// IssuedAt is when the JWT was issued.
	IssuedAt time.Time

	// Audience is the intended audience for the JWT.
	Audience []string

	// Claims contains additional claims in the JWT.
	Claims map[string]interface{}
}

// Expired returns true if the JWT SVID has expired.
func (s *JWTSVID) Expired() bool {
	return time.Now().After(s.ExpiresAt)
}

// TrustBundle represents a bundle of trusted CA certificates for a trust domain.
type TrustBundle struct {
	// TrustDomain is the trust domain this bundle is for.
	TrustDomain string

	// X509Authorities are the trusted CA certificates.
	X509Authorities []*x509.Certificate

	// JWTAuthorities are the trusted JWT signing keys (JWKs).
	JWTAuthorities []JWTAuthority

	// RefreshHint suggests how often to refresh the bundle.
	RefreshHint time.Duration

	// SequenceNumber is incremented on each update.
	SequenceNumber uint64

	// UpdatedAt is when the bundle was last updated.
	UpdatedAt time.Time
}

// JWTAuthority represents a trusted JWT signing key.
type JWTAuthority struct {
	// KeyID is the key identifier.
	KeyID string

	// PublicKey is the public key for verification.
	PublicKey crypto.PublicKey

	// ExpiresAt is when this key should no longer be trusted.
	ExpiresAt time.Time
}

// ProviderType represents the type of identity provider.
type ProviderType string

const (
	// ProviderTypeEmbedded is the built-in identity provider.
	ProviderTypeEmbedded ProviderType = "embedded"
	// ProviderTypeSPIRE uses an external SPIRE server.
	ProviderTypeSPIRE ProviderType = "spire"
	// ProviderTypeAWS uses AWS identity (IRSA, EC2).
	ProviderTypeAWS ProviderType = "aws"
	// ProviderTypeGCP uses GCP Workload Identity.
	ProviderTypeGCP ProviderType = "gcp"
	// ProviderTypeAzure uses Azure Managed Identity.
	ProviderTypeAzure ProviderType = "azure"
	// ProviderTypeIstio uses Istio service mesh identity.
	ProviderTypeIstio ProviderType = "istio"
	// ProviderTypeConsul uses Consul Connect identity.
	ProviderTypeConsul ProviderType = "consul"
	// ProviderTypeLinkerd uses Linkerd identity.
	ProviderTypeLinkerd ProviderType = "linkerd"
)

// ProviderStatus represents the health status of an identity provider.
type ProviderStatus string

const (
	// ProviderStatusUnknown is the initial status.
	ProviderStatusUnknown ProviderStatus = "unknown"
	// ProviderStatusHealthy means the provider is working correctly.
	ProviderStatusHealthy ProviderStatus = "healthy"
	// ProviderStatusDegraded means the provider is partially working.
	ProviderStatusDegraded ProviderStatus = "degraded"
	// ProviderStatusUnhealthy means the provider is not working.
	ProviderStatusUnhealthy ProviderStatus = "unhealthy"
)

// ProviderInfo contains information about an identity provider.
type ProviderInfo struct {
	// Type is the provider type.
	Type ProviderType

	// Status is the current health status.
	Status ProviderStatus

	// TrustDomain is the trust domain this provider manages.
	TrustDomain string

	// Capabilities lists what this provider supports.
	Capabilities []string

	// LastHealthCheck is when the provider was last checked.
	LastHealthCheck time.Time

	// ErrorMessage contains any error details if unhealthy.
	ErrorMessage string

	// Metadata contains provider-specific information.
	Metadata map[string]string
}

// IdentityProvider is the interface for all identity providers.
type IdentityProvider interface {
	// Type returns the provider type.
	Type() ProviderType

	// Start initializes and starts the provider.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the provider.
	Stop(ctx context.Context) error

	// Health returns the current health status.
	Health(ctx context.Context) ProviderStatus

	// Info returns detailed information about the provider.
	Info(ctx context.Context) ProviderInfo

	// TrustDomain returns the trust domain managed by this provider.
	TrustDomain() string

	// GetTrustBundle returns the current trust bundle.
	GetTrustBundle(ctx context.Context) (*TrustBundle, error)

	// WatchTrustBundle watches for trust bundle updates.
	WatchTrustBundle(ctx context.Context) (<-chan *TrustBundle, error)
}

// SVIDIssuer is the interface for issuing SVIDs.
type SVIDIssuer interface {
	// IssueX509SVID issues an X.509 SVID for the given SPIFFE ID.
	IssueX509SVID(ctx context.Context, req *X509SVIDRequest) (*X509SVID, error)

	// IssueJWTSVID issues a JWT SVID for the given SPIFFE ID.
	IssueJWTSVID(ctx context.Context, req *JWTSVIDRequest) (*JWTSVID, error)

	// RenewX509SVID renews an existing X.509 SVID.
	RenewX509SVID(ctx context.Context, current *X509SVID) (*X509SVID, error)
}

// X509SVIDRequest contains parameters for X.509 SVID issuance.
type X509SVIDRequest struct {
	// SPIFFEID is the requested SPIFFE identity.
	SPIFFEID SPIFFEID

	// CSR is an optional certificate signing request.
	// If nil, the issuer generates a new key pair.
	CSR []byte

	// TTL is the requested time-to-live for the SVID.
	// If zero, the default TTL is used.
	TTL time.Duration

	// DNSNames are optional DNS SANs to include.
	DNSNames []string

	// IPAddresses are optional IP SANs to include.
	IPAddresses []string

	// Hint is an optional hint for the SVID.
	Hint string
}

// JWTSVIDRequest contains parameters for JWT SVID issuance.
type JWTSVIDRequest struct {
	// SPIFFEID is the requested SPIFFE identity.
	SPIFFEID SPIFFEID

	// Audience is the intended audience for the JWT.
	Audience []string

	// TTL is the requested time-to-live for the JWT.
	// If zero, the default TTL is used.
	TTL time.Duration

	// ExtraClaims are additional claims to include in the JWT.
	ExtraClaims map[string]interface{}
}

// Attestor is the interface for workload attestation.
type Attestor interface {
	// Name returns the attestor name.
	Name() string

	// CanAttest returns true if this attestor can handle the given evidence.
	CanAttest(ctx context.Context, evidence *AttestationEvidence) bool

	// Attest verifies the attestation evidence and returns the result.
	Attest(ctx context.Context, evidence *AttestationEvidence) (*AttestationResult, error)
}

// AttestationEvidence contains evidence for workload attestation.
type AttestationEvidence struct {
	// Type identifies the type of attestation evidence.
	Type string

	// Data contains the attestation evidence (type-specific).
	Data []byte

	// Metadata contains additional context about the attestation.
	Metadata map[string]string
}

// MarshalJSON implements json.Marshaler.
func (e *AttestationEvidence) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type     string            `json:"type"`
		Data     string            `json:"data"`
		Metadata map[string]string `json:"metadata,omitempty"`
	}{
		Type:     e.Type,
		Data:     string(e.Data),
		Metadata: e.Metadata,
	})
}

// AttestationResult contains the result of attestation.
type AttestationResult struct {
	// Success indicates whether attestation succeeded.
	Success bool

	// SPIFFEID is the assigned SPIFFE identity (if successful).
	SPIFFEID SPIFFEID

	// Selectors are key-value pairs describing the workload.
	Selectors map[string]string

	// ExpiresAt is when this attestation expires.
	ExpiresAt time.Time

	// Error contains any error message (if failed).
	Error string

	// Attestor is the name of the attestor that handled this.
	Attestor string
}

// AttestationEvidenceType constants for built-in attestors.
const (
	// AttestationTypeJoinToken uses a one-time join token.
	AttestationTypeJoinToken = "join_token"
	// AttestationTypeAWSIID uses AWS Instance Identity Document.
	AttestationTypeAWSIID = "aws_iid"
	// AttestationTypeGCPIIT uses GCP Instance Identity Token.
	AttestationTypeGCPIIT = "gcp_iit"
	// AttestationTypeAzureIMDS uses Azure Instance Metadata Service.
	AttestationTypeAzureIMDS = "azure_imds"
	// AttestationTypeK8sSAT uses Kubernetes Service Account Token.
	AttestationTypeK8sSAT = "k8s_sat"
	// AttestationTypeConsulConnect uses Consul Connect certificate.
	AttestationTypeConsulConnect = "consul_connect"
	// AttestationTypeNone allows unauthenticated attestation (dev only).
	AttestationTypeNone = "none"
)

// JoinToken represents a one-time token for agent registration.
type JoinToken struct {
	// Token is the plaintext token value. Only populated when initially created
	// and returned to the caller. Never stored or retrieved after creation.
	// When listing tokens, this field will be empty.
	Token string

	// TokenHash is the salted SHA-256 hash of the token (hex encoded).
	// This is what gets stored instead of the raw token.
	TokenHash string

	// Salt is the random salt used for hashing (hex encoded).
	Salt string

	// TokenPrefix is the first 8 characters of the token for identification
	// purposes when listing tokens.
	TokenPrefix string

	// AgentID is the expected agent ID (optional).
	AgentID string

	// ExpiresAt is when the token expires.
	ExpiresAt time.Time

	// CreatedAt is when the token was created.
	CreatedAt time.Time

	// Used indicates whether the token has been used.
	Used bool

	// UsedAt is when the token was used (if Used is true).
	UsedAt time.Time

	// UsedBy is the agent that used the token (if Used is true).
	UsedBy string

	// Metadata contains additional token metadata.
	Metadata map[string]string
}

// IsValid returns true if the token is valid (not expired and not used).
func (t *JoinToken) IsValid() bool {
	return !t.Used && time.Now().Before(t.ExpiresAt)
}

// JoinTokenStore is the interface for managing join tokens.
type JoinTokenStore interface {
	// Create creates a new join token.
	Create(ctx context.Context, token *JoinToken) error

	// Get retrieves a join token by its value.
	Get(ctx context.Context, tokenValue string) (*JoinToken, error)

	// MarkUsed marks a token as used.
	MarkUsed(ctx context.Context, tokenValue, usedBy string) error

	// Delete deletes a token.
	Delete(ctx context.Context, tokenValue string) error

	// List lists all tokens (for admin purposes).
	List(ctx context.Context) ([]*JoinToken, error)

	// Cleanup removes expired and used tokens.
	Cleanup(ctx context.Context) (int, error)
}

// SVIDRotationCallback is called when an SVID is rotated.
type SVIDRotationCallback func(oldSVID, newSVID *X509SVID)

// TrustBundleUpdateCallback is called when the trust bundle is updated.
type TrustBundleUpdateCallback func(bundle *TrustBundle)

// IdentityClient is the interface for agents to obtain identity.
type IdentityClient interface {
	// Connect connects to the identity provider.
	Connect(ctx context.Context) error

	// Close closes the connection.
	Close() error

	// FetchX509SVID fetches an X.509 SVID.
	FetchX509SVID(ctx context.Context) (*X509SVID, error)

	// FetchJWTSVID fetches a JWT SVID for the given audience.
	FetchJWTSVID(ctx context.Context, audience []string) (*JWTSVID, error)

	// FetchTrustBundle fetches the trust bundle.
	FetchTrustBundle(ctx context.Context) (*TrustBundle, error)

	// WatchX509SVID watches for X.509 SVID updates.
	WatchX509SVID(ctx context.Context, callback SVIDRotationCallback) error

	// WatchTrustBundle watches for trust bundle updates.
	WatchTrustBundle(ctx context.Context, callback TrustBundleUpdateCallback) error
}

// ValidateTrustDomain validates a trust domain name.
func ValidateTrustDomain(domain string) error {
	if domain == "" {
		return fmt.Errorf("trust domain cannot be empty")
	}
	// Trust domain should be a valid DNS name or similar
	if strings.ContainsAny(domain, " \t\n\r") {
		return fmt.Errorf("trust domain contains whitespace")
	}
	if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return fmt.Errorf("trust domain cannot start or end with a dot")
	}
	if strings.Contains(domain, "..") {
		return fmt.Errorf("trust domain cannot contain consecutive dots")
	}
	return nil
}

// ValidateSPIFFEID validates a SPIFFE ID.
func ValidateSPIFFEID(id SPIFFEID) error {
	if err := ValidateTrustDomain(id.TrustDomain); err != nil {
		return fmt.Errorf("invalid trust domain: %w", err)
	}
	if id.Path == "" {
		return fmt.Errorf("SPIFFE ID path cannot be empty")
	}
	if !strings.HasPrefix(id.Path, "/") {
		return fmt.Errorf("SPIFFE ID path must start with /")
	}
	if strings.Contains(id.Path, "//") {
		return fmt.Errorf("SPIFFE ID path cannot contain consecutive slashes")
	}
	return nil
}
