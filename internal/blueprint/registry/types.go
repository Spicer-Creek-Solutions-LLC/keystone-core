// Package registry provides registry client and server components for blueprint distribution.
package registry

import (
	"errors"
	"time"

	"github.com/shawnbutts/keystone-core/internal/blueprint"
)

// AuthType represents the type of authentication for registry access.
type AuthType string

const (
	// AuthTypeNone indicates no authentication.
	AuthTypeNone AuthType = "none"
	// AuthTypeBasic indicates HTTP Basic authentication.
	AuthTypeBasic AuthType = "basic"
	// AuthTypeBearer indicates Bearer token authentication.
	AuthTypeBearer AuthType = "bearer"
	// AuthTypeAPIKey indicates API key authentication.
	AuthTypeAPIKey AuthType = "apikey"
)

// AuthConfig holds authentication configuration for registry access.
type AuthConfig struct {
	// Type is the authentication type.
	Type AuthType `json:"type" yaml:"type"`

	// Username for basic auth.
	Username string `json:"username,omitempty" yaml:"username,omitempty"`

	// Password for basic auth.
	Password string `json:"password,omitempty" yaml:"password,omitempty"`

	// Token for bearer or API key auth.
	Token string `json:"token,omitempty" yaml:"token,omitempty"`

	// Header is the header name for API key auth (default: X-API-Key).
	Header string `json:"header,omitempty" yaml:"header,omitempty"`
}

// RegistryConfig holds configuration for a blueprint registry client.
type RegistryConfig struct {
	// URL is the registry base URL.
	URL string `json:"url" yaml:"url"`

	// Auth is the authentication configuration.
	Auth *AuthConfig `json:"auth,omitempty" yaml:"auth,omitempty"`

	// Timeout is the HTTP client timeout.
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`

	// RetryAttempts is the number of retry attempts for failed requests.
	RetryAttempts int `json:"retry_attempts,omitempty" yaml:"retry_attempts,omitempty"`

	// RetryDelay is the delay between retry attempts.
	RetryDelay time.Duration `json:"retry_delay,omitempty" yaml:"retry_delay,omitempty"`

	// InsecureSkipVerify skips TLS certificate verification.
	InsecureSkipVerify bool `json:"insecure_skip_verify,omitempty" yaml:"insecure_skip_verify,omitempty"`

	// Namespace is the blueprint namespace prefix (default: "blueprints").
	Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
}

// DefaultRegistryConfig returns a RegistryConfig with default values.
func DefaultRegistryConfig() *RegistryConfig {
	return &RegistryConfig{
		Timeout:       30 * time.Second,
		RetryAttempts: 3,
		RetryDelay:    1 * time.Second,
		Namespace:     "blueprints",
	}
}

// BlueprintInfo contains metadata about a blueprint in the registry.
type BlueprintInfo struct {
	// Name is the full blueprint name (vendor/name).
	Name string `json:"name"`

	// Version is the blueprint version.
	Version string `json:"version"`

	// Published is the publication timestamp.
	Published time.Time `json:"published"`

	// Checksum is the SHA-256 hash of the blueprint archive.
	Checksum string `json:"checksum"`

	// Size is the size of the blueprint archive in bytes.
	Size int64 `json:"size"`

	// Metadata contains blueprint metadata from the manifest.
	Metadata *blueprint.Metadata `json:"metadata,omitempty"`

	// Dependencies lists blueprint dependencies.
	Dependencies []string `json:"dependencies,omitempty"`

	// Compatibility contains compatibility requirements.
	Compatibility *blueprint.Compatibility `json:"compatibility,omitempty"`

	// Signature is the cosign signature if signed.
	Signature string `json:"signature,omitempty"`

	// SignedBy is the identity that signed the blueprint.
	SignedBy string `json:"signed_by,omitempty"`
}

// VersionInfo contains information about a specific blueprint version.
type VersionInfo struct {
	// Version is the semantic version.
	Version string `json:"version"`

	// Published is the publication timestamp.
	Published time.Time `json:"published"`

	// Deprecated indicates if this version is deprecated.
	Deprecated bool `json:"deprecated,omitempty"`

	// DeprecationMessage explains why the version is deprecated.
	DeprecationMessage string `json:"deprecation_message,omitempty"`

	// Retracted indicates if this version has been retracted.
	Retracted bool `json:"retracted,omitempty"`

	// RetractionReason explains why the version was retracted.
	RetractionReason string `json:"retraction_reason,omitempty"`
}

// PublishRequest contains the data needed to publish a blueprint.
type PublishRequest struct {
	// Name is the full blueprint name (vendor/name).
	Name string `json:"name"`

	// Version is the blueprint version.
	Version string `json:"version"`

	// Manifest is the blueprint manifest content.
	Manifest []byte `json:"manifest"`

	// Archive is the blueprint archive content (tar.gz).
	Archive []byte `json:"archive"`

	// Checksum is the SHA-256 hash of the archive.
	Checksum string `json:"checksum"`

	// Signature is the optional cosign signature.
	Signature []byte `json:"signature,omitempty"`

	// Certificate is the optional signing certificate.
	Certificate []byte `json:"certificate,omitempty"`

	// Force overwrites an existing version (requires special permissions).
	Force bool `json:"force,omitempty"`
}

// PublishResult contains the result of a publish operation.
type PublishResult struct {
	// Name is the published blueprint name.
	Name string `json:"name"`

	// Version is the published version.
	Version string `json:"version"`

	// Published is the publication timestamp.
	Published time.Time `json:"published"`

	// URL is the download URL for the blueprint.
	URL string `json:"url"`

	// Checksum is the SHA-256 hash of the archive.
	Checksum string `json:"checksum"`

	// SumDBEntry is the transparency log entry if SumDB is enabled.
	SumDBEntry string `json:"sumdb_entry,omitempty"`
}

// SearchQuery contains search criteria for blueprints.
type SearchQuery struct {
	// Query is the search query string.
	Query string `json:"query,omitempty"`

	// Vendor filters by vendor name.
	Vendor string `json:"vendor,omitempty"`

	// Tags filters by tags.
	Tags []string `json:"tags,omitempty"`

	// MinKSCoreVersion filters by minimum kscore version compatibility.
	MinKSCoreVersion string `json:"min_kscore_version,omitempty"`

	// Platform filters by supported platform.
	Platform string `json:"platform,omitempty"`

	// Limit is the maximum number of results.
	Limit int `json:"limit,omitempty"`

	// Offset is the result offset for pagination.
	Offset int `json:"offset,omitempty"`

	// SortBy is the field to sort by (name, published, downloads).
	SortBy string `json:"sort_by,omitempty"`

	// SortOrder is the sort order (asc, desc).
	SortOrder string `json:"sort_order,omitempty"`
}

// SearchResult contains search results.
type SearchResult struct {
	// Blueprints is the list of matching blueprints.
	Blueprints []*BlueprintInfo `json:"blueprints"`

	// Total is the total number of matching blueprints.
	Total int `json:"total"`

	// Limit is the limit used in the query.
	Limit int `json:"limit"`

	// Offset is the offset used in the query.
	Offset int `json:"offset"`
}

// IndexEntry represents a blueprint in the registry index.
type IndexEntry struct {
	// Name is the full blueprint name (namespace/shortname).
	Name string `json:"name"`

	// Namespace is the vendor/organization namespace.
	Namespace string `json:"namespace,omitempty"`

	// ShortName is the blueprint name without namespace.
	ShortName string `json:"short_name,omitempty"`

	// LatestVersion is the latest available version.
	LatestVersion string `json:"latest_version"`

	// AllVersions lists all available versions.
	AllVersions []string `json:"all_versions,omitempty"`

	// Versions is the list of available versions (alias for AllVersions).
	Versions []string `json:"versions,omitempty"`

	// Description is the blueprint description.
	Description string `json:"description,omitempty"`

	// Author is the blueprint author.
	Author string `json:"author,omitempty"`

	// Tags are the blueprint tags.
	Tags []string `json:"tags,omitempty"`

	// Category is the blueprint category.
	Category string `json:"category,omitempty"`

	// Dependencies lists blueprint dependencies.
	Dependencies []string `json:"dependencies,omitempty"`

	// Keywords are additional searchable terms.
	Keywords []string `json:"keywords,omitempty"`

	// License is the blueprint license.
	License string `json:"license,omitempty"`

	// Repository is the source repository URL.
	Repository string `json:"repository,omitempty"`

	// Homepage is the project homepage URL.
	Homepage string `json:"homepage,omitempty"`

	// Downloads is the total download count.
	Downloads int64 `json:"downloads"`

	// Stars is the star/favorite count.
	Stars int `json:"stars,omitempty"`

	// Verified indicates if the blueprint is verified.
	Verified bool `json:"verified,omitempty"`

	// Deprecated indicates if the blueprint is deprecated.
	Deprecated bool `json:"deprecated,omitempty"`

	// DeprecationMessage explains why it's deprecated.
	DeprecationMessage string `json:"deprecation_message,omitempty"`

	// CreatedAt is when the blueprint was first indexed.
	CreatedAt time.Time `json:"created_at,omitempty"`

	// UpdatedAt is when the blueprint was last updated.
	UpdatedAt time.Time `json:"updated_at,omitempty"`

	// LastUpdated is when the blueprint was last updated (alias for UpdatedAt).
	LastUpdated time.Time `json:"last_updated,omitempty"`

	// MinKSCoreVersion is the minimum Keystone Core version.
	MinKSCoreVersion string `json:"min_kscore_version,omitempty"`

	// Platforms lists supported platforms.
	Platforms []string `json:"platforms,omitempty"`

	// Digest is the latest version digest.
	Digest string `json:"digest,omitempty"`

	// SignatureStatus indicates signature verification status.
	SignatureStatus string `json:"signature_status,omitempty"`

	// SignerIdentity is the verified signer identity.
	SignerIdentity string `json:"signer_identity,omitempty"`
}

// RegistryClient defines the interface for blueprint registry operations.
type RegistryClient interface {
	// ListVersions returns all available versions for a blueprint.
	ListVersions(name string) ([]string, error)

	// GetBlueprintInfo returns information about a specific blueprint version.
	GetBlueprintInfo(name, version string) (*BlueprintInfo, error)

	// GetLatestVersion returns the latest version of a blueprint.
	GetLatestVersion(name string) (string, error)

	// DownloadBlueprint downloads a blueprint archive.
	DownloadBlueprint(name, version string) ([]byte, error)

	// GetManifest returns the blueprint manifest.
	GetManifest(name, version string) (*blueprint.Blueprint, error)
}

// PublishableRegistry extends RegistryClient with publishing capabilities.
type PublishableRegistry interface {
	RegistryClient

	// PublishBlueprint publishes a new blueprint version.
	PublishBlueprint(req *PublishRequest) (*PublishResult, error)

	// DeleteBlueprint deletes a blueprint version.
	DeleteBlueprint(name, version string) error

	// DeprecateVersion marks a version as deprecated.
	DeprecateVersion(name, version, message string) error

	// RetractVersion retracts a version (makes it unavailable).
	RetractVersion(name, version, reason string) error

	// SetAuth sets the authentication configuration.
	SetAuth(auth *AuthConfig)
}

// SearchableRegistry extends RegistryClient with search capabilities.
type SearchableRegistry interface {
	RegistryClient

	// Search searches for blueprints matching the query.
	Search(query *SearchQuery) (*SearchResult, error)

	// GetIndex returns the full registry index.
	GetIndex() ([]*IndexEntry, error)

	// GetVendors returns all vendors in the registry.
	GetVendors() ([]string, error)

	// GetTags returns all tags used in the registry.
	GetTags() ([]string, error)
}

// FullRegistry combines all registry capabilities.
type FullRegistry interface {
	PublishableRegistry
	SearchableRegistry
}

// Common errors.
var (
	// ErrBlueprintNotFound indicates the blueprint was not found in the registry.
	ErrBlueprintNotFound = errors.New("blueprint not found")

	// ErrVersionNotFound indicates the version was not found.
	ErrVersionNotFound = errors.New("version not found")

	// ErrVersionExists indicates the version already exists.
	ErrVersionExists = errors.New("version already exists")

	// ErrUnauthorized indicates authentication failed.
	ErrUnauthorized = errors.New("unauthorized")

	// ErrForbidden indicates the operation is not permitted.
	ErrForbidden = errors.New("forbidden")

	// ErrInvalidManifest indicates the manifest is invalid.
	ErrInvalidManifest = errors.New("invalid manifest")

	// ErrChecksumMismatch indicates the checksum doesn't match.
	ErrChecksumMismatch = errors.New("checksum mismatch")

	// ErrSignatureInvalid indicates the signature is invalid.
	ErrSignatureInvalid = errors.New("signature invalid")

	// ErrVersionRetracted indicates the version has been retracted.
	ErrVersionRetracted = errors.New("version retracted")

	// ErrRegistryUnavailable indicates the registry is unavailable.
	ErrRegistryUnavailable = errors.New("registry unavailable")
)

// RegistryError represents an error from the registry.
type RegistryError struct {
	// StatusCode is the HTTP status code.
	StatusCode int `json:"status_code"`

	// Code is the error code.
	Code string `json:"code"`

	// Message is the error message.
	Message string `json:"message"`

	// Details contains additional error details.
	Details map[string]interface{} `json:"details,omitempty"`
}

// Error implements the error interface.
func (e *RegistryError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

// IsNotFound returns true if the error indicates a not found condition.
func (e *RegistryError) IsNotFound() bool {
	return e.StatusCode == 404 || e.Code == "not_found"
}

// IsUnauthorized returns true if the error indicates an authorization failure.
func (e *RegistryError) IsUnauthorized() bool {
	return e.StatusCode == 401 || e.Code == "unauthorized"
}

// IsForbidden returns true if the error indicates a forbidden operation.
func (e *RegistryError) IsForbidden() bool {
	return e.StatusCode == 403 || e.Code == "forbidden"
}

// IsConflict returns true if the error indicates a conflict (version exists).
func (e *RegistryError) IsConflict() bool {
	return e.StatusCode == 409 || e.Code == "conflict"
}
