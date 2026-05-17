package secrets

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// BackendCapability advertises which interface methods a backend
// meaningfully implements. The encrypted-file backend (task 4) reports
// [CapKV] / [CapList]; the Vault backend (task 5) adds [CapDynamic] /
// [CapLeaseRenew] / [CapLeaseRevoke] / [CapTransit]. The broker (task
// 3) consults the set before routing a method that the backend can't
// serve — returning [ErrInvalidBackend] before dispatch so the audit
// trail records a precise reason.
type BackendCapability uint8

const (
	// CapKVUnknown is the zero value.
	CapKVUnknown BackendCapability = iota
	// CapKV signals static key/value reads + writes + deletes.
	CapKV
	// CapList signals enumerating paths under a prefix.
	CapList
	// CapDynamic signals issuing leased dynamic secrets
	// (database creds, IAM creds, PKI certs, SSH OTPs).
	CapDynamic
	// CapLeaseRenew signals the backend honours
	// [SecretBackend.RenewLease].
	CapLeaseRenew
	// CapLeaseRevoke signals the backend honours
	// [SecretBackend.RevokeLease].
	CapLeaseRevoke
	// CapTransit signals encryption-as-a-service (Vault transit).
	CapTransit
)

// String returns the canonical lowercase name.
func (c BackendCapability) String() string {
	switch c {
	case CapKV:
		return "kv"
	case CapList:
		return "list"
	case CapDynamic:
		return "dynamic"
	case CapLeaseRenew:
		return "lease_renew"
	case CapLeaseRevoke:
		return "lease_revoke"
	case CapTransit:
		return "transit"
	default:
		return "unknown"
	}
}

// ParseBackendCapability is the inverse of [BackendCapability.String].
// Unknown inputs return [CapKVUnknown] and a wrapped [ErrInvalidBackend].
func ParseBackendCapability(s string) (BackendCapability, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "kv":
		return CapKV, nil
	case "list":
		return CapList, nil
	case "dynamic":
		return CapDynamic, nil
	case "lease_renew", "lease-renew":
		return CapLeaseRenew, nil
	case "lease_revoke", "lease-revoke":
		return CapLeaseRevoke, nil
	case "transit":
		return CapTransit, nil
	default:
		return CapKVUnknown, fmt.Errorf("%w: unknown backend capability %q", ErrInvalidBackend, s)
	}
}

// HasCapability reports whether the set contains target. Linear scan
// — the set is tiny by construction (≤ 6 values) so an O(n) check is
// simpler than maintaining a map.
func HasCapability(set []BackendCapability, target BackendCapability) bool {
	for _, c := range set {
		if c == target {
			return true
		}
	}
	return false
}

// GetSecretRequest drives [SecretBackend.GetSecret]. Version selects a
// specific historical version for KV-v2-style backends; 0 means "the
// current version." Refresh forces a backend round-trip even when a
// cache (task 8) would otherwise satisfy the call; the broker (task 3)
// honours it before consulting the cache.
type GetSecretRequest struct {
	Path    string
	Version uint64
	Refresh bool
}

// WriteSecretRequest drives [SecretBackend.WriteSecret].
//
// Data is the cleartext payload. CAS, when non-nil, is a
// compare-and-swap version check honoured by KV-v2-style backends
// (Vault KV v2's `cas`). Backends without CAS support ignore the field.
type WriteSecretRequest struct {
	Path     string
	Data     map[string]any
	Metadata map[string]string
	CAS      *uint64
}

// ListSecretsRequest drives [SecretBackend.ListSecrets]. Prefix is
// matched at path-segment boundaries; the response is metadata-only
// per the v1.0 contract that cleartext never appears in list responses.
type ListSecretsRequest struct {
	Prefix string
	Limit  int
	Cursor string
}

// ListSecretsResponse carries the metadata view returned from
// [SecretBackend.ListSecrets]. Entries describe paths + metadata only;
// to retrieve a payload, callers re-issue [SecretBackend.GetSecret].
// NextCursor is the opaque pagination token; empty when the listing
// is complete.
type ListSecretsResponse struct {
	Entries    []ListEntry
	NextCursor string
}

// ListEntry is one row in a [ListSecretsResponse].
type ListEntry struct {
	Path      string            `json:"path"`
	Version   uint64            `json:"version,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	UpdatedAt time.Time         `json:"updated_at,omitempty"`
}

// DeleteSecretRequest drives [SecretBackend.DeleteSecret]. Version, when
// non-zero, deletes a specific historical version on KV-v2-style
// backends; 0 means "delete the current version (and tombstone)."
// Destroy, when true, requests a hard delete on backends that
// distinguish soft / hard deletes (Vault KV v2's `destroy` op).
type DeleteSecretRequest struct {
	Path    string
	Version uint64
	Destroy bool
}

// IssueDynamicSecretRequest drives [SecretBackend.IssueDynamicSecret].
// Role names the dynamic-secret role (Vault DB role, IAM role, PKI
// role, SSH role); the backend resolves the role against Path.
// TTL requests a specific lifetime; 0 means "backend default." MaxTTL
// caps the total lifetime across renewals; 0 means "backend default."
// Params is engine-specific (e.g. PKI `common_name`, `alt_names`).
type IssueDynamicSecretRequest struct {
	Path   string
	Role   string
	TTL    time.Duration
	MaxTTL time.Duration
	Params map[string]any
}

// RenewLeaseRequest drives [SecretBackend.RenewLease].
type RenewLeaseRequest struct {
	LeaseID   string
	Increment time.Duration // 0 = backend default
}

// RevokeLeaseRequest drives [SecretBackend.RevokeLease].
type RevokeLeaseRequest struct {
	LeaseID string
	// Force, when true, asks the backend to skip its
	// renewal-protection grace window. Vault honours it; the
	// encrypted-file backend ignores it.
	Force bool
}

// SecretBackend is the seam every concrete secret store implements:
// encrypted-file (task 4), HashiCorp Vault (task 5), and the v2.x+
// cloud backends (AWS Secrets Manager / Azure Key Vault / GCP Secret
// Manager) once they're un-gated. The broker (task 3), the gRPC
// service (task 9), and the CLI (task 10) depend only on this
// interface.
//
// Lifecycle: Start(ctx) → (running) → Stop(ctx). Start is one-shot.
// Stop is idempotent. Health(ctx) reports liveness; a backend that
// failed during Start surfaces the error here too.
//
// Capability surface: backends declare via Capabilities() which
// operational methods they implement; methods outside the declared
// set return [ErrInvalidBackend].
//
// Every method on this interface accepts a context. Backends MUST
// honour ctx.Done() — the in-flight epic-10 tasks (4-7) all use
// network or filesystem IO that needs cancellation.
type SecretBackend interface {
	// Name is the operator-facing backend name from config
	// (`secrets.backends[].name`). Stable across restarts so audit
	// entries and routing rules survive a config reload.
	Name() string

	// Capabilities lists which operational methods the backend
	// meaningfully implements; methods outside the set return
	// [ErrInvalidBackend] wrapped with capability context.
	Capabilities() []BackendCapability

	// Start brings the backend up — opens the encrypted file, dials
	// Vault, authenticates, warms any connection pools. Returns the
	// first error encountered.
	Start(ctx context.Context) error

	// Stop tears the backend down. Idempotent.
	Stop(ctx context.Context) error

	// Health returns nil when the backend is up and reachable.
	// Returns [ErrBackendNotStarted] before Start / after Stop.
	Health(ctx context.Context) error

	// GetSecret reads a single secret at req.Path. Returns
	// [ErrSecretNotFound] for misses.
	GetSecret(ctx context.Context, req GetSecretRequest) (*Secret, error)

	// WriteSecret writes (creates or updates) a secret. Returns the
	// stored shape — including any backend-assigned version.
	WriteSecret(ctx context.Context, req WriteSecretRequest) (*Secret, error)

	// ListSecrets enumerates paths under req.Prefix. Response is
	// metadata-only by contract.
	ListSecrets(ctx context.Context, req ListSecretsRequest) (*ListSecretsResponse, error)

	// DeleteSecret removes a secret (or a specific version, per
	// req.Version). Returns nil on success; [ErrSecretNotFound] if
	// the path is unknown.
	DeleteSecret(ctx context.Context, req DeleteSecretRequest) error

	// IssueDynamicSecret asks a dynamic engine for a fresh credential
	// (DB user, IAM token, PKI cert, SSH OTP). The returned [Secret]
	// carries a populated LeaseID and the broker registers it with
	// the [LeaseManager] (task 6).
	IssueDynamicSecret(ctx context.Context, req IssueDynamicSecretRequest) (*Secret, error)

	// RenewLease extends a lease's TTL. Returns the updated
	// [LeaseInfo]; [ErrLeaseNotFound] / [ErrLeaseExpired] /
	// [ErrLeaseNotRenewable] as appropriate.
	RenewLease(ctx context.Context, req RenewLeaseRequest) (*LeaseInfo, error)

	// RevokeLease tears a lease down out-of-band — the credential
	// becomes invalid immediately. Idempotent; revoking an
	// already-revoked / already-expired lease returns nil.
	RevokeLease(ctx context.Context, req RevokeLeaseRequest) error
}
