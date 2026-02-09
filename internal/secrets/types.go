// Package secrets provides enterprise secret management integration for Keystone Core.
// It supports multiple secret backends (HashiCorp Vault, AWS Secrets Manager, Azure Key Vault,
// GCP Secret Manager) with dynamic secrets, lease management, rotation orchestration,
// transit encryption, and credential brokering.
package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// BackendType identifies the type of secret backend.
type BackendType string

// BackendTypeVault constants define the supported types.
const (
	BackendTypeVault BackendType = "vault"
	BackendTypeAWS   BackendType = "aws_secrets_manager"
	BackendTypeAzure BackendType = "azure_keyvault"
	BackendTypeGCP   BackendType = "gcp_secret_manager"
)

// String returns the string representation of the backend type.
func (b BackendType) String() string {
	return string(b)
}

// Valid returns true if the backend type is valid.
func (b BackendType) Valid() bool {
	switch b {
	case BackendTypeVault, BackendTypeAWS, BackendTypeAzure, BackendTypeGCP:
		return true
	default:
		return false
	}
}

// SecretType identifies the type of secret.
type SecretType string

// SecretTypeStatic constants define the supported types.
const (
	SecretTypeStatic     SecretType = "static"     // Static secret (KV store)
	SecretTypeDynamic    SecretType = "dynamic"    // Dynamic secret (generated on demand)
	SecretTypePKI        SecretType = "pki"        // PKI certificate
	SecretTypeTransit    SecretType = "transit"    // Transit encryption key
	SecretTypeDatabase   SecretType = "database"   // Database credentials
	SecretTypeCloudIAM   SecretType = "cloud_iam"  // Cloud IAM credentials
	SecretTypeSSH        SecretType = "ssh"        // SSH certificate
	SecretTypeKubernetes SecretType = "kubernetes" // Kubernetes service account
)

// String returns the string representation of the secret type.
func (s SecretType) String() string {
	return string(s)
}

// Secret represents a secret retrieved from a backend.
type Secret struct {
	// Path is the full path to the secret.
	Path string `json:"path"`

	// Backend is the backend that served this secret.
	Backend BackendType `json:"backend"`

	// Type is the type of secret.
	Type SecretType `json:"type"`

	// Data contains the secret data as key-value pairs.
	Data map[string]interface{} `json:"data"`

	// Metadata contains additional metadata about the secret.
	Metadata map[string]string `json:"metadata,omitempty"`

	// Version is the secret version (if versioned).
	Version int `json:"version,omitempty"`

	// CreatedAt is when the secret was created.
	CreatedAt time.Time `json:"created_at,omitempty"`

	// ExpiresAt is when the secret expires (zero = no expiry).
	ExpiresAt time.Time `json:"expires_at,omitempty"`

	// Lease contains lease information for dynamic secrets.
	Lease *Lease `json:"lease,omitempty"`

	// Renewable indicates if the secret can be renewed.
	Renewable bool `json:"renewable,omitempty"`
}

// Get returns a value from the secret data by key.
func (s *Secret) Get(key string) (interface{}, bool) {
	if s.Data == nil {
		return nil, false
	}
	v, ok := s.Data[key]
	return v, ok
}

// GetString returns a string value from the secret data.
func (s *Secret) GetString(key string) (string, bool) {
	v, ok := s.Get(key)
	if !ok {
		return "", false
	}
	str, ok := v.(string)
	return str, ok
}

// GetBytes returns a byte slice value from the secret data.
func (s *Secret) GetBytes(key string) ([]byte, bool) {
	v, ok := s.Get(key)
	if !ok {
		return nil, false
	}
	switch val := v.(type) {
	case []byte:
		return val, true
	case string:
		return []byte(val), true
	default:
		return nil, false
	}
}

// IsExpired returns true if the secret has expired.
func (s *Secret) IsExpired() bool {
	if s.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(s.ExpiresAt)
}

// HasLease returns true if the secret has an associated lease.
func (s *Secret) HasLease() bool {
	return s.Lease != nil && s.Lease.ID != ""
}

// LeaseState represents the state of a lease.
type LeaseState string

// LeaseState constants define the possible states.
const (
	LeaseStatePending  LeaseState = "pending"
	LeaseStateActive   LeaseState = "active"
	LeaseStateRenewing LeaseState = "renewing"
	LeaseStateExpired  LeaseState = "expired"
	LeaseStateRevoked  LeaseState = "revoked"
)

// Lease represents a lease for a dynamic secret.
type Lease struct {
	// ID is the unique lease identifier.
	ID string `json:"id"`

	// SecretPath is the path of the associated secret.
	SecretPath string `json:"secret_path"`

	// Backend is the backend that issued this lease.
	Backend BackendType `json:"backend"`

	// State is the current lease state.
	State LeaseState `json:"state"`

	// TTL is the time-to-live for this lease.
	TTL time.Duration `json:"ttl"`

	// MaxTTL is the maximum TTL even with renewals.
	MaxTTL time.Duration `json:"max_ttl,omitempty"`

	// IssuedAt is when the lease was issued.
	IssuedAt time.Time `json:"issued_at"`

	// ExpiresAt is when the lease expires.
	ExpiresAt time.Time `json:"expires_at"`

	// LastRenewal is when the lease was last renewed.
	LastRenewal time.Time `json:"last_renewal,omitempty"`

	// RenewalCount is how many times the lease has been renewed.
	RenewalCount int `json:"renewal_count"`

	// Renewable indicates if the lease can be renewed.
	Renewable bool `json:"renewable"`

	// Revocable indicates if the lease can be explicitly revoked.
	Revocable bool `json:"revocable"`

	// Metadata contains additional lease metadata.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// TimeRemaining returns the time remaining until the lease expires.
func (l *Lease) TimeRemaining() time.Duration {
	if l.ExpiresAt.IsZero() {
		return 0
	}
	remaining := time.Until(l.ExpiresAt)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// IsExpired returns true if the lease has expired.
func (l *Lease) IsExpired() bool {
	if l.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(l.ExpiresAt)
}

// NeedsRenewal returns true if the lease needs renewal based on the threshold.
// threshold is a value between 0 and 1 (e.g., 0.5 = renew at 50% of TTL).
func (l *Lease) NeedsRenewal(threshold float64) bool {
	if !l.Renewable || l.IsExpired() {
		return false
	}
	remaining := l.TimeRemaining()
	renewalThreshold := time.Duration(float64(l.TTL) * (1 - threshold))
	return remaining <= renewalThreshold
}

// RenewalStrategy defines how leases should be renewed.
type RenewalStrategy string

// RenewalStrategyEager constants define the strategies.
const (
	RenewalStrategyEager    RenewalStrategy = "eager"     // Renew at 50% of TTL
	RenewalStrategyLazy     RenewalStrategy = "lazy"      // Renew at 90% of TTL
	RenewalStrategyOnDemand RenewalStrategy = "on_demand" // Renew only when accessed
)

// Threshold returns the renewal threshold for this strategy.
func (r RenewalStrategy) Threshold() float64 {
	switch r {
	case RenewalStrategyEager:
		return 0.5
	case RenewalStrategyLazy:
		return 0.9
	case RenewalStrategyOnDemand:
		return 1.0
	default:
		return 0.5
	}
}

// SecretRequest represents a request to fetch a secret.
type SecretRequest struct {
	// Path is the secret path (e.g., "vault/kv/myapp/config").
	Path string `json:"path"`

	// Keys specifies specific keys to retrieve (empty = all keys).
	Keys []string `json:"keys,omitempty"`

	// Version specifies a specific version to retrieve (0 = latest).
	Version int `json:"version,omitempty"`

	// TTL is the requested TTL for dynamic secrets.
	TTL time.Duration `json:"ttl,omitempty"`

	// Renewable indicates if a dynamic secret should be renewable.
	Renewable bool `json:"renewable,omitempty"`

	// AgentID is the requesting agent's ID.
	AgentID string `json:"agent_id,omitempty"`

	// SPIFFEID is the requesting agent's SPIFFE identity.
	SPIFFEID string `json:"spiffe_id,omitempty"`

	// RequestID is a unique identifier for this request.
	RequestID string `json:"request_id,omitempty"`

	// Context contains additional request context.
	Context map[string]string `json:"context,omitempty"`
}

// SecretBackend defines the interface for secret backends.
type SecretBackend interface {
	// Type returns the backend type.
	Type() BackendType

	// Name returns the backend instance name.
	Name() string

	// Healthy returns true if the backend is healthy.
	Healthy(ctx context.Context) bool

	// Read reads a secret from the backend.
	Read(ctx context.Context, req *SecretRequest) (*Secret, error)

	// ReadDynamic reads or generates a dynamic secret.
	ReadDynamic(ctx context.Context, req *SecretRequest) (*Secret, error)

	// List lists secrets under a path prefix.
	List(ctx context.Context, prefix string) ([]string, error)

	// RenewLease renews a lease.
	RenewLease(ctx context.Context, leaseID string, increment time.Duration) (*Lease, error)

	// RevokeLease revokes a lease.
	RevokeLease(ctx context.Context, leaseID string) error

	// Close closes the backend connection.
	Close() error
}

// LeaseManager manages the lifecycle of secret leases.
type LeaseManager interface {
	// Track starts tracking a lease.
	Track(ctx context.Context, lease *Lease) error

	// Get retrieves a tracked lease by ID.
	Get(ctx context.Context, leaseID string) (*Lease, error)

	// List lists all tracked leases.
	List(ctx context.Context) ([]*Lease, error)

	// ListByPath lists leases for a specific secret path.
	ListByPath(ctx context.Context, path string) ([]*Lease, error)

	// ListByBackend lists leases for a specific backend.
	ListByBackend(ctx context.Context, backend BackendType) ([]*Lease, error)

	// ListExpiring lists leases expiring within the given duration.
	ListExpiring(ctx context.Context, within time.Duration) ([]*Lease, error)

	// Renew renews a lease.
	Renew(ctx context.Context, leaseID string, increment time.Duration) (*Lease, error)

	// Revoke revokes a lease.
	Revoke(ctx context.Context, leaseID string) error

	// RevokeByPath revokes all leases for a path prefix.
	RevokeByPath(ctx context.Context, pathPrefix string) (int, error)

	// Remove removes a lease from tracking (after expiry/revocation).
	Remove(ctx context.Context, leaseID string) error

	// Stats returns lease manager statistics.
	Stats(ctx context.Context) (*LeaseStats, error)

	// Start starts the lease renewal scheduler.
	Start(ctx context.Context) error

	// Stop stops the lease renewal scheduler.
	Stop() error
}

// LeaseStats contains lease manager statistics.
type LeaseStats struct {
	// ActiveLeases is the number of active leases.
	ActiveLeases int `json:"active_leases"`

	// ExpiringLeases is the number of leases expiring soon.
	ExpiringLeases int `json:"expiring_leases"`

	// ExpiredLeases is the number of expired leases pending cleanup.
	ExpiredLeases int `json:"expired_leases"`

	// RevokedLeases is the number of revoked leases pending cleanup.
	RevokedLeases int `json:"revoked_leases"`

	// TotalRenewals is the total number of lease renewals.
	TotalRenewals int64 `json:"total_renewals"`

	// FailedRenewals is the number of failed renewal attempts.
	FailedRenewals int64 `json:"failed_renewals"`

	// LeasesByBackend is the count of leases by backend type.
	LeasesByBackend map[BackendType]int `json:"leases_by_backend"`
}

// SecretCache provides caching for secrets with encryption.
type SecretCache interface {
	// Get retrieves a secret from the cache.
	Get(ctx context.Context, path string) (*Secret, bool)

	// Put stores a secret in the cache with the given TTL.
	Put(ctx context.Context, secret *Secret, ttl time.Duration) error

	// Delete removes a secret from the cache.
	Delete(ctx context.Context, path string) error

	// DeleteByPrefix removes all secrets matching a path prefix.
	DeleteByPrefix(ctx context.Context, prefix string) (int, error)

	// Clear removes all secrets from the cache.
	Clear(ctx context.Context) error

	// Stats returns cache statistics.
	Stats() *CacheStats

	// Close closes the cache and releases resources.
	Close() error
}

// CacheStats contains cache statistics.
type CacheStats struct {
	// Entries is the current number of cached entries.
	Entries int `json:"entries"`

	// MaxEntries is the maximum allowed entries.
	MaxEntries int `json:"max_entries"`

	// Hits is the number of cache hits.
	Hits int64 `json:"hits"`

	// Misses is the number of cache misses.
	Misses int64 `json:"misses"`

	// Evictions is the number of evicted entries.
	Evictions int64 `json:"evictions"`

	// ExpiredCount is the number of expired entries removed.
	ExpiredCount int64 `json:"expired_count"`

	// MemoryBytes is the estimated memory usage in bytes.
	MemoryBytes int64 `json:"memory_bytes"`
}

// HitRate returns the cache hit rate as a percentage.
func (s *CacheStats) HitRate() float64 {
	total := s.Hits + s.Misses
	if total == 0 {
		return 0
	}
	return float64(s.Hits) / float64(total) * 100
}

// TransitOperation represents a transit encryption operation type.
type TransitOperation string

// TransitOperationEncrypt constants define the operators.
const (
	TransitOperationEncrypt TransitOperation = "encrypt"
	TransitOperationDecrypt TransitOperation = "decrypt"
	TransitOperationRewrap  TransitOperation = "rewrap"
	TransitOperationSign    TransitOperation = "sign"
	TransitOperationVerify  TransitOperation = "verify"
	TransitOperationHMAC    TransitOperation = "hmac"
	TransitOperationHash    TransitOperation = "hash"
)

// TransitRequest represents a request for transit encryption operations.
type TransitRequest struct {
	// Operation is the type of operation to perform.
	Operation TransitOperation `json:"operation"`

	// KeyName is the name of the transit key.
	KeyName string `json:"key_name"`

	// KeyVersion is the key version to use (0 = latest for encrypt, required for decrypt).
	KeyVersion int `json:"key_version,omitempty"`

	// Plaintext is the data to encrypt (for encrypt operation).
	Plaintext []byte `json:"plaintext,omitempty"`

	// Ciphertext is the data to decrypt (for decrypt operation).
	Ciphertext string `json:"ciphertext,omitempty"`

	// Context is additional authenticated data (AAD) for encryption.
	Context []byte `json:"context,omitempty"`

	// Input is the input data for sign/verify/hmac/hash operations.
	Input []byte `json:"input,omitempty"`

	// Signature is the signature to verify (for verify operation).
	Signature string `json:"signature,omitempty"`

	// Algorithm is the hash algorithm for hmac/hash operations.
	Algorithm string `json:"algorithm,omitempty"`

	// Batch indicates if this is part of a batch operation.
	Batch bool `json:"batch,omitempty"`

	// Convergent enables convergent encryption (same plaintext = same ciphertext).
	Convergent bool `json:"convergent,omitempty"`
}

// TransitResponse represents the response from a transit operation.
type TransitResponse struct {
	// Ciphertext is the encrypted data (for encrypt operation).
	Ciphertext string `json:"ciphertext,omitempty"`

	// Plaintext is the decrypted data (for decrypt operation).
	Plaintext []byte `json:"plaintext,omitempty"`

	// Signature is the generated signature (for sign operation).
	Signature string `json:"signature,omitempty"`

	// Valid indicates if the signature is valid (for verify operation).
	Valid bool `json:"valid,omitempty"`

	// HMAC is the generated HMAC (for hmac operation).
	HMAC string `json:"hmac,omitempty"`

	// Hash is the generated hash (for hash operation).
	Hash string `json:"hash,omitempty"`

	// KeyVersion is the key version used.
	KeyVersion int `json:"key_version,omitempty"`
}

// TransitBackend defines the interface for transit encryption backends.
type TransitBackend interface {
	// Encrypt encrypts plaintext data.
	Encrypt(ctx context.Context, req *TransitRequest) (*TransitResponse, error)

	// Decrypt decrypts ciphertext data.
	Decrypt(ctx context.Context, req *TransitRequest) (*TransitResponse, error)

	// Rewrap re-encrypts ciphertext with the latest key version.
	Rewrap(ctx context.Context, req *TransitRequest) (*TransitResponse, error)

	// Sign signs input data.
	Sign(ctx context.Context, req *TransitRequest) (*TransitResponse, error)

	// Verify verifies a signature.
	Verify(ctx context.Context, req *TransitRequest) (*TransitResponse, error)

	// HMAC generates an HMAC.
	HMAC(ctx context.Context, req *TransitRequest) (*TransitResponse, error)

	// Hash generates a hash.
	Hash(ctx context.Context, req *TransitRequest) (*TransitResponse, error)

	// BatchEncrypt encrypts multiple plaintexts.
	BatchEncrypt(ctx context.Context, reqs []*TransitRequest) ([]*TransitResponse, error)

	// BatchDecrypt decrypts multiple ciphertexts.
	BatchDecrypt(ctx context.Context, reqs []*TransitRequest) ([]*TransitResponse, error)
}

// RotationStrategy defines how secrets should be rotated.
type RotationStrategy string

// RotationStrategyBlueGreen constants define the strategies.
const (
	RotationStrategyBlueGreen RotationStrategy = "blue_green" // Generate new, switch atomically
	RotationStrategyRolling   RotationStrategy = "rolling"    // Update consumers one-by-one
	RotationStrategyCanary    RotationStrategy = "canary"     // Update subset first
	RotationStrategyImmediate RotationStrategy = "immediate"  // Update all at once
)

// RotationState represents the state of a rotation operation.
type RotationState string

// RotationState constants define the possible states.
const (
	RotationStatePending    RotationState = "pending"
	RotationStateInProgress RotationState = "in_progress"
	RotationStateVerifying  RotationState = "verifying"
	RotationStateCompleted  RotationState = "completed"
	RotationStateFailed     RotationState = "failed"
	RotationStateRolledBack RotationState = "rolled_back"
	RotationStateCancelled  RotationState = "cancelled"
)

// RotationConfig defines configuration for secret rotation.
type RotationConfig struct {
	// Strategy is the rotation strategy to use.
	Strategy RotationStrategy `json:"strategy"`

	// Schedule is the cron-like schedule for rotation (empty = manual only).
	Schedule string `json:"schedule,omitempty"`

	// BatchSize is the number of targets to update per batch (for rolling).
	BatchSize int `json:"batch_size,omitempty"`

	// BatchDelay is the delay between batches.
	BatchDelay time.Duration `json:"batch_delay,omitempty"`

	// CanaryPercentage is the percentage of targets to update first (for canary strategy).
	// Default is 10% if not specified.
	CanaryPercentage int `json:"canary_percentage,omitempty"`

	// CanaryDelay is how long to wait after canary verification before proceeding.
	CanaryDelay time.Duration `json:"canary_delay,omitempty"`

	// HealthCheck defines health check configuration for verification.
	HealthCheck *HealthCheckConfig `json:"health_check,omitempty"`

	// RollbackOnFailure enables automatic rollback on failure.
	RollbackOnFailure bool `json:"rollback_on_failure"`

	// FailureThreshold is the failure percentage that triggers rollback.
	FailureThreshold float64 `json:"failure_threshold,omitempty"`

	// GracePeriod is how long to keep old credentials valid after rotation.
	GracePeriod time.Duration `json:"grace_period,omitempty"`
}

// HealthCheckConfig defines health check configuration.
type HealthCheckConfig struct {
	// Type is the type of health check (http, tcp, exec).
	Type string `json:"type"`

	// Endpoint is the health check endpoint (for http/tcp type).
	Endpoint string `json:"endpoint,omitempty"`

	// ExpectedStatus is the expected HTTP status code.
	ExpectedStatus int `json:"expected_status,omitempty"`

	// Command is the command to execute (for exec type).
	// Supports placeholders: {target_id}, {target_name}, {agent_id}
	Command string `json:"command,omitempty"`

	// Timeout is the health check timeout.
	Timeout time.Duration `json:"timeout,omitempty"`

	// Interval is the interval between retries.
	Interval time.Duration `json:"interval,omitempty"`

	// Retries is the number of retries before considering unhealthy.
	Retries int `json:"retries,omitempty"`
}

// Rotation represents an in-progress or completed rotation operation.
type Rotation struct {
	// ID is the unique rotation identifier.
	ID string `json:"id"`

	// SecretPath is the path of the secret being rotated.
	SecretPath string `json:"secret_path"`

	// State is the current rotation state.
	State RotationState `json:"state"`

	// Strategy is the rotation strategy used.
	Strategy RotationStrategy `json:"strategy"`

	// StartedAt is when the rotation started.
	StartedAt time.Time `json:"started_at"`

	// CompletedAt is when the rotation completed.
	CompletedAt time.Time `json:"completed_at,omitempty"`

	// TotalTargets is the total number of targets to update.
	TotalTargets int `json:"total_targets"`

	// UpdatedTargets is the number of targets successfully updated.
	UpdatedTargets int `json:"updated_targets"`

	// FailedTargets is the number of targets that failed to update.
	FailedTargets int `json:"failed_targets"`

	// Error is the error message if the rotation failed.
	Error string `json:"error,omitempty"`

	// OldVersion is the old secret version.
	OldVersion int `json:"old_version,omitempty"`

	// NewVersion is the new secret version.
	NewVersion int `json:"new_version,omitempty"`
}

// BrokerConfig configures the secret broker.
type BrokerConfig struct {
	// DefaultBackend is the default backend for unqualified paths.
	DefaultBackend string `json:"default_backend,omitempty"`

	// Cache configures the secret cache.
	Cache *CacheConfig `json:"cache,omitempty"`

	// LeaseRenewal configures lease renewal behavior.
	LeaseRenewal *LeaseRenewalConfig `json:"lease_renewal,omitempty"`

	// Routing configures path-based routing rules.
	Routing []RoutingRule `json:"routing,omitempty"`
}

// CacheConfig configures the secret cache.
type CacheConfig struct {
	// Enabled enables caching.
	Enabled bool `json:"enabled"`

	// MaxEntries is the maximum number of cached entries.
	MaxEntries int `json:"max_entries,omitempty"`

	// DefaultTTL is the default TTL for cached secrets.
	DefaultTTL time.Duration `json:"default_ttl,omitempty"`

	// EncryptionKeySource is where to get the cache encryption key.
	EncryptionKeySource string `json:"encryption_key_source,omitempty"` // "local" or "kms"

	// CleanupInterval is how often to run cache cleanup.
	CleanupInterval time.Duration `json:"cleanup_interval,omitempty"`
}

// DefaultCacheConfig returns a cache configuration with sensible defaults.
func DefaultCacheConfig() *CacheConfig {
	return &CacheConfig{
		Enabled:         true,
		MaxEntries:      10000,
		DefaultTTL:      5 * time.Minute,
		CleanupInterval: time.Minute,
	}
}

// LeaseRenewalConfig configures lease renewal behavior.
type LeaseRenewalConfig struct {
	// Strategy is the renewal strategy to use.
	Strategy RenewalStrategy `json:"strategy,omitempty"`

	// Threshold is the renewal threshold (0-1).
	Threshold float64 `json:"threshold,omitempty"`

	// GracePeriod is buffer time for renewal failures.
	GracePeriod time.Duration `json:"grace_period,omitempty"`

	// MaxRetries is the maximum renewal retry attempts.
	MaxRetries int `json:"max_retries,omitempty"`

	// RetryInterval is the interval between retry attempts.
	RetryInterval time.Duration `json:"retry_interval,omitempty"`
}

// DefaultLeaseRenewalConfig returns a lease renewal config with sensible defaults.
func DefaultLeaseRenewalConfig() *LeaseRenewalConfig {
	return &LeaseRenewalConfig{
		Strategy:      RenewalStrategyEager,
		Threshold:     0.5,
		GracePeriod:   30 * time.Second,
		MaxRetries:    3,
		RetryInterval: 5 * time.Second,
	}
}

// RoutingRule defines a path-based routing rule.
type RoutingRule struct {
	// Prefix is the path prefix to match.
	Prefix string `json:"prefix,omitempty"`

	// Tag is the tag to match.
	Tag string `json:"tag,omitempty"`

	// Backend is the backend to route to.
	Backend string `json:"backend"`
}

// Common errors for secret operations.
var (
	// ErrSecretNotFound indicates the requested secret was not found.
	ErrSecretNotFound = errors.New("secret not found")

	// ErrSecretExpired indicates the secret has expired.
	ErrSecretExpired = errors.New("secret expired")

	// ErrBackendNotFound indicates the requested backend was not found.
	ErrBackendNotFound = errors.New("backend not found")

	// ErrBackendUnavailable indicates the backend is temporarily unavailable.
	ErrBackendUnavailable = errors.New("backend unavailable")

	// ErrLeaseNotFound indicates the requested lease was not found.
	ErrLeaseNotFound = errors.New("lease not found")

	// ErrLeaseExpired indicates the lease has expired.
	ErrLeaseExpired = errors.New("lease expired")

	// ErrLeaseNotRenewable indicates the lease cannot be renewed.
	ErrLeaseNotRenewable = errors.New("lease not renewable")

	// ErrAccessDenied indicates access to the secret was denied.
	ErrAccessDenied = errors.New("access denied")

	// ErrInvalidPath indicates the secret path is invalid.
	ErrInvalidPath = errors.New("invalid secret path")

	// ErrCacheEncryptionFailed indicates cache encryption failed.
	ErrCacheEncryptionFailed = errors.New("cache encryption failed")

	// ErrCacheDecryptionFailed indicates cache decryption failed.
	ErrCacheDecryptionFailed = errors.New("cache decryption failed")

	// ErrRotationInProgress indicates a rotation is already in progress.
	ErrRotationInProgress = errors.New("rotation already in progress")

	// ErrRotationFailed indicates the rotation failed.
	ErrRotationFailed = errors.New("rotation failed")

	// ErrTransitOperationFailed indicates a transit operation failed.
	ErrTransitOperationFailed = errors.New("transit operation failed")
)

// ParseBackendType parses a backend type from a string.
func ParseBackendType(s string) (BackendType, error) {
	bt := BackendType(s)
	if !bt.Valid() {
		return "", errors.New("invalid backend type: " + s)
	}
	return bt, nil
}

// MarshalJSON implements json.Marshaler for Secret.
func (s *Secret) MarshalJSON() ([]byte, error) {
	type Alias Secret
	ttl := ""
	if s.Lease != nil {
		ttl = s.Lease.TTL.String()
	}
	return json.Marshal(&struct {
		*Alias
		TTL string `json:"ttl,omitempty"`
	}{
		Alias: (*Alias)(s),
		TTL:   ttl,
	})
}

// MarshalJSON implements json.Marshaler for Lease.
func (l *Lease) MarshalJSON() ([]byte, error) {
	type Alias Lease
	return json.Marshal(&struct {
		*Alias
		TTLSeconds    int64 `json:"ttl_seconds"`
		MaxTTLSeconds int64 `json:"max_ttl_seconds,omitempty"`
	}{
		Alias:         (*Alias)(l),
		TTLSeconds:    int64(l.TTL.Seconds()),
		MaxTTLSeconds: int64(l.MaxTTL.Seconds()),
	})
}
