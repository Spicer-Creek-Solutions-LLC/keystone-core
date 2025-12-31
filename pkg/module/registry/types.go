package registry

import (
	"time"

	"github.com/shawnbutts/keystone-core/pkg/module/manifest"
	"github.com/shawnbutts/keystone-core/pkg/module/resolver"
)

// PublishableRegistry extends RegistryClient with publishing capabilities
type PublishableRegistry interface {
	resolver.RegistryClient

	// PublishModule uploads a module to the registry
	PublishModule(req *PublishRequest) (*PublishResult, error)

	// DeleteModule removes a module version from the registry (if supported)
	DeleteModule(moduleName, version string) error

	// SetAuth sets authentication credentials
	SetAuth(auth *AuthConfig)
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	// Type is the authentication type (basic, bearer, api-key)
	Type AuthType

	// Username for basic auth
	Username string

	// Password for basic auth
	Password string

	// Token for bearer or API key auth
	Token string

	// Header is the header name for API key auth (default: Authorization)
	Header string
}

// AuthType represents the authentication type
type AuthType string

const (
	AuthTypeNone   AuthType = "none"
	AuthTypeBasic  AuthType = "basic"
	AuthTypeBearer AuthType = "bearer"
	AuthTypeAPIKey AuthType = "api-key"
)

// PublishRequest contains all data needed to publish a module
type PublishRequest struct {
	// ModulePath is the path to the module ZIP file
	ModulePath string

	// Manifest is the parsed module manifest
	Manifest *manifest.Manifest

	// SignaturePath is the optional path to the detached signature file
	SignaturePath string

	// Hash is the SHA256 hash of the module (computed if empty)
	Hash string

	// Force overwrites existing version if true
	Force bool

	// Tags are optional tags for the module version
	Tags []string

	// ReleaseNotes are optional release notes
	ReleaseNotes string
}

// PublishResult contains the result of a publish operation
type PublishResult struct {
	// ModuleName is the published module name
	ModuleName string

	// Version is the published version
	Version string

	// Hash is the module hash
	Hash string

	// URL is the registry URL for the published module
	URL string

	// PublishedAt is when the module was published
	PublishedAt time.Time

	// Size is the module size in bytes
	Size int64

	// SignatureVerified indicates if the signature was verified by the registry
	SignatureVerified bool

	// SumDBRecorded indicates if the hash was recorded in SumDB
	SumDBRecorded bool
}

// RegistryConfig holds registry connection configuration
type RegistryConfig struct {
	// URL is the base URL of the registry
	URL string

	// Auth is optional authentication configuration
	Auth *AuthConfig

	// Timeout is the HTTP timeout
	Timeout time.Duration

	// RetryAttempts is the number of retry attempts
	RetryAttempts int

	// RetryDelay is the delay between retries
	RetryDelay time.Duration

	// InsecureSkipVerify skips TLS verification (for testing only)
	InsecureSkipVerify bool

	// SumDBURL is the optional SumDB URL for hash recording
	SumDBURL string
}

// DefaultRegistryConfig returns a configuration with sensible defaults
func DefaultRegistryConfig(url string) *RegistryConfig {
	return &RegistryConfig{
		URL:           url,
		Timeout:       30 * time.Second,
		RetryAttempts: 3,
		RetryDelay:    time.Second,
	}
}

// RegistryError represents an error from the registry
type RegistryError struct {
	// StatusCode is the HTTP status code
	StatusCode int

	// Message is the error message
	Message string

	// Code is the registry-specific error code
	Code string
}

// Error implements the error interface
func (e *RegistryError) Error() string {
	if e.Code != "" {
		return e.Code + ": " + e.Message
	}
	return e.Message
}

// Common registry error codes
const (
	ErrCodeModuleNotFound    = "MODULE_NOT_FOUND"
	ErrCodeVersionNotFound   = "VERSION_NOT_FOUND"
	ErrCodeVersionExists     = "VERSION_EXISTS"
	ErrCodeUnauthorized      = "UNAUTHORIZED"
	ErrCodeForbidden         = "FORBIDDEN"
	ErrCodeInvalidModule     = "INVALID_MODULE"
	ErrCodeInvalidSignature  = "INVALID_SIGNATURE"
	ErrCodeQuotaExceeded     = "QUOTA_EXCEEDED"
	ErrCodeRateLimited       = "RATE_LIMITED"
	ErrCodeServerError       = "SERVER_ERROR"
)

// IsNotFoundError returns true if the error is a not found error
func IsNotFoundError(err error) bool {
	if regErr, ok := err.(*RegistryError); ok {
		return regErr.StatusCode == 404 ||
			regErr.Code == ErrCodeModuleNotFound ||
			regErr.Code == ErrCodeVersionNotFound
	}
	return false
}

// IsVersionExistsError returns true if the error indicates the version already exists
func IsVersionExistsError(err error) bool {
	if regErr, ok := err.(*RegistryError); ok {
		return regErr.Code == ErrCodeVersionExists
	}
	return false
}

// IsAuthError returns true if the error is an authentication error
func IsAuthError(err error) bool {
	if regErr, ok := err.(*RegistryError); ok {
		return regErr.StatusCode == 401 ||
			regErr.StatusCode == 403 ||
			regErr.Code == ErrCodeUnauthorized ||
			regErr.Code == ErrCodeForbidden
	}
	return false
}
