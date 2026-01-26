// Package federation provides trust federation between identity providers.
// It enables cross-domain authentication by aggregating trust bundles
// from multiple trust domains and validating SVIDs across domain boundaries.
package federation

import (
	"context"
	"crypto/x509"
	"time"

	"github.com/shawnbutts/keystone-core/internal/identity"
)

// FederationState represents the state of a federation relationship.
type FederationState string

const (
	// FederationStatePending means the federation is awaiting approval.
	FederationStatePending FederationState = "pending"
	// FederationStateActive means the federation is active and working.
	FederationStateActive FederationState = "active"
	// FederationStateSuspended means the federation is temporarily suspended.
	FederationStateSuspended FederationState = "suspended"
	// FederationStateRevoked means the federation has been permanently revoked.
	FederationStateRevoked FederationState = "revoked"
	// FederationStateExpired means the federation has expired.
	FederationStateExpired FederationState = "expired"
)

// FederationType represents the type of federation relationship.
type FederationType string

const (
	// FederationTypeBidirectional means both domains trust each other.
	FederationTypeBidirectional FederationType = "bidirectional"
	// FederationTypeUnidirectional means only one domain trusts the other.
	FederationTypeUnidirectional FederationType = "unidirectional"
	// FederationTypeTransitive means trust can be inherited through chains.
	FederationTypeTransitive FederationType = "transitive"
)

// TrustPolicy defines access control for federated trust domains.
type TrustPolicy struct {
	// Name is a human-readable name for the policy.
	Name string

	// Description explains the policy purpose.
	Description string

	// AllowedPaths are SPIFFE ID paths that are trusted.
	// Supports glob patterns like "/agent/*" or "/service/**".
	AllowedPaths []string

	// DeniedPaths are SPIFFE ID paths that are explicitly denied.
	// Takes precedence over AllowedPaths.
	DeniedPaths []string

	// AllowedServices are service names that are trusted.
	AllowedServices []string

	// DeniedServices are service names that are explicitly denied.
	DeniedServices []string

	// RequireAttributes are required SVID attributes.
	RequireAttributes map[string]string

	// MaxTTL is the maximum TTL for SVIDs from this domain.
	// Zero means no limit.
	MaxTTL time.Duration

	// RequireMTLS requires mutual TLS for connections.
	RequireMTLS bool

	// AuditLevel controls logging for this policy.
	// Values: none, minimal, standard, verbose
	AuditLevel string
}

// FederatedDomain represents a trust relationship with another domain.
type FederatedDomain struct {
	// TrustDomain is the SPIFFE trust domain being federated.
	TrustDomain string

	// Type is the federation type.
	Type FederationType

	// State is the current federation state.
	State FederationState

	// TrustBundle is the trust bundle for this domain.
	TrustBundle *identity.TrustBundle

	// Policy defines access control for this domain.
	Policy *TrustPolicy

	// BundleEndpoint is the URL to fetch trust bundle updates (optional).
	BundleEndpoint string

	// BundleEndpointProfile is the profile for bundle endpoint authentication.
	// Values: https_web, https_spiffe
	BundleEndpointProfile string

	// EndpointSPIFFEID is the expected SPIFFE ID of the bundle endpoint server.
	EndpointSPIFFEID string

	// RefreshInterval is how often to refresh the trust bundle.
	RefreshInterval time.Duration

	// CreatedAt is when the federation was created.
	CreatedAt time.Time

	// UpdatedAt is when the federation was last updated.
	UpdatedAt time.Time

	// ExpiresAt is when the federation expires (zero means no expiry).
	ExpiresAt time.Time

	// ApprovedBy is who approved this federation.
	ApprovedBy string

	// Notes contains additional notes about the federation.
	Notes string
}

// IsExpired returns true if the federation has expired.
func (d *FederatedDomain) IsExpired() bool {
	if d.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(d.ExpiresAt)
}

// IsActive returns true if the federation is active and not expired.
func (d *FederatedDomain) IsActive() bool {
	return d.State == FederationStateActive && !d.IsExpired()
}

// FederationManager manages trust federation between domains.
type FederationManager interface {
	// AddFederatedDomain adds a new federated trust domain.
	AddFederatedDomain(ctx context.Context, domain *FederatedDomain) error

	// RemoveFederatedDomain removes a federated trust domain.
	RemoveFederatedDomain(ctx context.Context, trustDomain string) error

	// GetFederatedDomain retrieves a federated domain by trust domain.
	GetFederatedDomain(ctx context.Context, trustDomain string) (*FederatedDomain, error)

	// ListFederatedDomains lists all federated domains.
	ListFederatedDomains(ctx context.Context) ([]*FederatedDomain, error)

	// UpdateFederatedDomain updates a federated domain.
	UpdateFederatedDomain(ctx context.Context, domain *FederatedDomain) error

	// RefreshTrustBundle refreshes the trust bundle for a domain.
	RefreshTrustBundle(ctx context.Context, trustDomain string) error

	// GetAggregatedTrustBundle returns a combined trust bundle.
	GetAggregatedTrustBundle(ctx context.Context) (*identity.TrustBundle, error)

	// ValidateSVID validates an SVID against federated trust bundles.
	ValidateSVID(ctx context.Context, svid *identity.X509SVID) (*ValidationResult, error)

	// Start starts background trust bundle refresh.
	Start(ctx context.Context) error

	// Stop stops the federation manager.
	Stop(ctx context.Context) error
}

// ValidationResult contains the result of SVID validation.
type ValidationResult struct {
	// Valid indicates whether the SVID is valid.
	Valid bool

	// SPIFFEID is the validated SPIFFE identity.
	SPIFFEID identity.SPIFFEID

	// TrustDomain is the trust domain the SVID belongs to.
	TrustDomain string

	// IsFederated indicates if the SVID is from a federated domain.
	IsFederated bool

	// FederationType is the type of federation (if federated).
	FederationType FederationType

	// MatchedPolicy is the policy that allowed/denied the SVID.
	MatchedPolicy string

	// CertificateChain is the validated certificate chain.
	CertificateChain []*x509.Certificate

	// ExpiresAt is when the SVID expires.
	ExpiresAt time.Time

	// Error contains any validation error message.
	Error string

	// ValidatedAt is when the validation was performed.
	ValidatedAt time.Time
}

// BundleFetcher fetches trust bundles from remote endpoints.
type BundleFetcher interface {
	// Fetch retrieves a trust bundle from the given endpoint.
	Fetch(ctx context.Context, endpoint string, profile string) (*identity.TrustBundle, error)
}

// FederationStore persists federation state.
type FederationStore interface {
	// Save saves a federated domain.
	Save(ctx context.Context, domain *FederatedDomain) error

	// Load loads a federated domain.
	Load(ctx context.Context, trustDomain string) (*FederatedDomain, error)

	// Delete deletes a federated domain.
	Delete(ctx context.Context, trustDomain string) error

	// List lists all federated domains.
	List(ctx context.Context) ([]*FederatedDomain, error)
}

// FederationEvent represents a federation-related event.
type FederationEvent struct {
	// Type is the event type.
	Type FederationEventType

	// TrustDomain is the affected trust domain.
	TrustDomain string

	// Timestamp is when the event occurred.
	Timestamp time.Time

	// Details contains additional event details.
	Details map[string]string
}

// FederationEventType is the type of federation event.
type FederationEventType string

const (
	// FederationEventAdded means a domain was added.
	FederationEventAdded FederationEventType = "added"
	// FederationEventRemoved means a domain was removed.
	FederationEventRemoved FederationEventType = "removed"
	// FederationEventUpdated means a domain was updated.
	FederationEventUpdated FederationEventType = "updated"
	// FederationEventRefreshed means a trust bundle was refreshed.
	FederationEventRefreshed FederationEventType = "refreshed"
	// FederationEventExpired means a federation expired.
	FederationEventExpired FederationEventType = "expired"
	// FederationEventSuspended means a federation was suspended.
	FederationEventSuspended FederationEventType = "suspended"
	// FederationEventReactivated means a federation was reactivated.
	FederationEventReactivated FederationEventType = "reactivated"
	// FederationEventValidationFailed means SVID validation failed.
	FederationEventValidationFailed FederationEventType = "validation_failed"
)

// FederationEventCallback is called when federation events occur.
type FederationEventCallback func(event *FederationEvent)

// FederationConfig configures the federation manager.
type FederationConfig struct {
	// LocalTrustDomain is this server's trust domain.
	LocalTrustDomain string

	// LocalTrustBundle is this server's trust bundle.
	LocalTrustBundle *identity.TrustBundle

	// DefaultRefreshInterval is the default refresh interval for trust bundles.
	// Default: 5 minutes
	DefaultRefreshInterval time.Duration

	// MaxFederatedDomains is the maximum number of federated domains.
	// Zero means unlimited.
	MaxFederatedDomains int

	// EnableTransitiveTrust allows transitive trust relationships.
	EnableTransitiveTrust bool

	// DefaultPolicy is the default policy for new federations.
	DefaultPolicy *TrustPolicy

	// Store is the persistence store for federation state.
	Store FederationStore

	// BundleFetcher is used to fetch remote trust bundles.
	BundleFetcher BundleFetcher

	// EventCallback is called when federation events occur.
	EventCallback FederationEventCallback

	// RequireApproval requires manual approval for new federations.
	RequireApproval bool

	// AuditLog enables audit logging for federation events.
	AuditLog bool
}

// DefaultFederationConfig returns a FederationConfig with default values.
func DefaultFederationConfig(trustDomain string) *FederationConfig {
	return &FederationConfig{
		LocalTrustDomain:       trustDomain,
		DefaultRefreshInterval: 5 * time.Minute,
		MaxFederatedDomains:    100,
		EnableTransitiveTrust:  false,
		RequireApproval:        true,
		AuditLog:               true,
		DefaultPolicy: &TrustPolicy{
			Name:        "default",
			Description: "Default federation policy",
			AuditLevel:  "standard",
		},
	}
}
