// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"fmt"
	"strings"
	"time"
)

// LeaseState is the lifecycle stage a leased / dynamic secret sits in.
// The transitions per PROJECT-DETAILS §4.11 are:
//
//	Pending → Active → Renewing → Active (loop)
//	                ↘ Expired ↘
//	                 Revoked   → Cleanup → removed
type LeaseState uint8

const (
	// LeaseStateUnknown is the zero value; surfaces only as a serde
	// sentinel for malformed input.
	LeaseStateUnknown LeaseState = iota
	// LeaseStatePending — backend has accepted the lease request but
	// the credential is not yet usable (e.g. Vault PKI cert issuance
	// in-flight).
	LeaseStatePending
	// LeaseStateActive — credential is usable; renewal scheduler
	// (task 6) watches the TTL.
	LeaseStateActive
	// LeaseStateRenewing — renewal in flight. Transitions back to
	// Active on success, Expired on failure past the deadline.
	LeaseStateRenewing
	// LeaseStateExpired — TTL elapsed; credential no longer valid.
	// Cleanup loop removes the row after [LeaseInfo] tracking is no
	// longer needed.
	LeaseStateExpired
	// LeaseStateRevoked — explicitly revoked via
	// [SecretBackend.RevokeLease].
	LeaseStateRevoked
)

// String returns the canonical lowercase name for the state, suitable
// for log lines, audit entries, and JSON serialisation.
func (s LeaseState) String() string {
	switch s {
	case LeaseStatePending:
		return "pending"
	case LeaseStateActive:
		return "active"
	case LeaseStateRenewing:
		return "renewing"
	case LeaseStateExpired:
		return "expired"
	case LeaseStateRevoked:
		return "revoked"
	default:
		return "unknown"
	}
}

// ParseLeaseState is the inverse of [LeaseState.String]. Unknown
// inputs return [LeaseStateUnknown] and a wrapped [ErrInvalidBackend]
// error.
func ParseLeaseState(s string) (LeaseState, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "pending":
		return LeaseStatePending, nil
	case "active":
		return LeaseStateActive, nil
	case "renewing":
		return LeaseStateRenewing, nil
	case "expired":
		return LeaseStateExpired, nil
	case "revoked":
		return LeaseStateRevoked, nil
	default:
		return LeaseStateUnknown, fmt.Errorf("%w: unknown lease state %q", ErrInvalidBackend, s)
	}
}

// MarshalText emits the canonical lowercase name; pairs with
// [LeaseState.UnmarshalText] so the type round-trips through JSON,
// YAML, and config loaders without a custom marshaler at the use site.
func (s LeaseState) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

// UnmarshalText parses textual input into the receiver. Empty input
// decodes to [LeaseStateUnknown] without erroring so a missing field
// round-trips cleanly.
func (s *LeaseState) UnmarshalText(b []byte) error {
	if len(b) == 0 {
		*s = LeaseStateUnknown
		return nil
	}
	parsed, err := ParseLeaseState(string(b))
	if err != nil {
		return err
	}
	*s = parsed
	return nil
}

// RenewStrategy is how the lease manager (task 6) decides when to
// renew a lease. The three strategies cover the spectrum from
// "renew aggressively, never miss" through "renew lazily, save round
// trips" to "client drives, scheduler is passive."
type RenewStrategy uint8

const (
	// RenewStrategyUnknown is the zero value.
	RenewStrategyUnknown RenewStrategy = iota
	// RenewStrategyEager renews at 50% of the TTL. Suitable for
	// short-lived dynamic credentials where a missed renewal carries
	// real user impact.
	RenewStrategyEager
	// RenewStrategyLazy renews at 90% of the TTL. Default for
	// long-lived credentials where renewal cost matters more than
	// margin.
	RenewStrategyLazy
	// RenewStrategyOnDemand never renews proactively — the lease
	// manager only touches the lease when the client asks. Suitable
	// for credentials whose lifetimes are explicitly bounded by a
	// session or job.
	RenewStrategyOnDemand
)

// Threshold returns the fraction of the TTL at which the strategy
// triggers a renewal. RenewStrategyOnDemand returns 0 (never; the
// scheduler ignores leases under this strategy).
func (s RenewStrategy) Threshold() float64 {
	switch s {
	case RenewStrategyEager:
		return 0.5
	case RenewStrategyLazy:
		return 0.9
	default:
		return 0
	}
}

// String returns the canonical lowercase name.
func (s RenewStrategy) String() string {
	switch s {
	case RenewStrategyEager:
		return "eager"
	case RenewStrategyLazy:
		return "lazy"
	case RenewStrategyOnDemand:
		return "on_demand"
	default:
		return "unknown"
	}
}

// ParseRenewStrategy is the inverse of [RenewStrategy.String].
// Accepts both "on_demand" and "on-demand" / "ondemand" for ergonomics
// in config files. Unknown inputs return [RenewStrategyUnknown] with
// a wrapped [ErrInvalidBackend].
func ParseRenewStrategy(s string) (RenewStrategy, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "eager":
		return RenewStrategyEager, nil
	case "lazy":
		return RenewStrategyLazy, nil
	case "on_demand", "on-demand", "ondemand":
		return RenewStrategyOnDemand, nil
	default:
		return RenewStrategyUnknown, fmt.Errorf("%w: unknown renew strategy %q", ErrInvalidBackend, s)
	}
}

// MarshalText emits the canonical lowercase name.
func (s RenewStrategy) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

// UnmarshalText parses textual input.
func (s *RenewStrategy) UnmarshalText(b []byte) error {
	if len(b) == 0 {
		*s = RenewStrategyUnknown
		return nil
	}
	parsed, err := ParseRenewStrategy(string(b))
	if err != nil {
		return err
	}
	*s = parsed
	return nil
}

// LeaseInfo is the persistent / wire-shape view of a lease — the
// shape stored in the [LeaseManager] SQLite table (task 6) and
// returned by [SecretBackend.RenewLease].
//
// Duration is the TTL granted at issue or renewal time; IssuedAt +
// Duration is the contract for ExpiresAt, but the backend's reported
// ExpiresAt wins on a clock skew.
//
// MaxTTL bounds total lease lifetime across renewals — 0 means
// "unbounded." Vault enforces this at the backend; the file backend
// honours it advisorily.
//
// Metadata is free-form operator info (issuing role, IAM scope, …)
// that the broker surfaces in list responses.
type LeaseInfo struct {
	ID            string            `json:"id"`
	SecretPath    string            `json:"secret_path"`
	Backend       string            `json:"backend"`
	IssuedAt      time.Time         `json:"issued_at"`
	ExpiresAt     time.Time         `json:"expires_at"`
	Duration      time.Duration     `json:"duration"`
	Renewable     bool              `json:"renewable"`
	MaxTTL        time.Duration     `json:"max_ttl,omitempty"`
	State         LeaseState        `json:"state"`
	LastRenewedAt time.Time         `json:"last_renewed_at,omitempty"`
	RenewCount    int               `json:"renew_count,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// Expired reports whether the lease has passed its expiry instant
// (relative to the supplied now). The check is `now.After(ExpiresAt)`
// — an exact match is still considered live so the scheduler has a
// well-defined boundary.
func (l LeaseInfo) Expired(now time.Time) bool {
	if l.ExpiresAt.IsZero() {
		return false
	}
	return now.After(l.ExpiresAt)
}

// TimeRemaining returns the duration until expiry, or 0 if the lease
// is already expired. A zero ExpiresAt (lease has no expiry) returns
// a sentinel of 0 — callers that need to distinguish must check
// ExpiresAt themselves.
func (l LeaseInfo) TimeRemaining(now time.Time) time.Duration {
	if l.ExpiresAt.IsZero() {
		return 0
	}
	remaining := l.ExpiresAt.Sub(now)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// ShouldRenew reports whether the scheduler should kick a renewal for
// this lease right now under the given strategy. Returns false for
// non-renewable leases, expired leases, RenewStrategyOnDemand (the
// scheduler is passive), and the unknown-strategy zero value.
//
// For eager / lazy, the check is "elapsed fraction of TTL ≥
// threshold." Implemented in pure arithmetic so the same code path
// works across mocked clocks in tests and real wall-clock now in
// production.
func (l LeaseInfo) ShouldRenew(now time.Time, strategy RenewStrategy) bool {
	if !l.Renewable {
		return false
	}
	threshold := strategy.Threshold()
	if threshold == 0 {
		return false
	}
	if l.Duration <= 0 || l.ExpiresAt.IsZero() {
		return false
	}
	if l.Expired(now) {
		return false
	}
	elapsed := l.Duration - l.TimeRemaining(now)
	return float64(elapsed) >= threshold*float64(l.Duration)
}

// Lease is the lease as observed by the broker / API server — embeds
// the persistent shape and adds the caller-attribution + revocation
// fields that aren't part of the backend's own state.
//
// IssuedFor is the SPIFFE ID of the caller that received the
// credential (string-typed at this layer so this package doesn't
// import `internal/identity`; the broker converts at the boundary).
// Empty when the issuing flow is not SPIFFE-authenticated (CLI calls
// from an operator console).
//
// RevokedAt is the wall-clock instant the lease moved to
// [LeaseStateRevoked]; nil for never-revoked leases.
type Lease struct {
	LeaseInfo

	IssuedFor string     `json:"issued_for,omitempty"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}
