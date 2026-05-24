package secrets

import "errors"

// ErrInvalidBackend is the family root for [SecretBackend] constructor
// rejections, malformed requests, and any error a backend wants to
// surface as a generic "this didn't make it past validation." Backends
// wrap context with `fmt.Errorf("%w: ...", ErrInvalidBackend)` so call
// sites match with [errors.Is].
var ErrInvalidBackend = errors.New("secrets: invalid backend")

// ErrBackendNotStarted is the sentinel a backend returns when one of
// its operational methods is called before [SecretBackend.Start] or
// after [SecretBackend.Stop].
var ErrBackendNotStarted = errors.New("secrets: backend not started")

// ErrSecretNotFound is the backend-agnostic miss for [SecretBackend.GetSecret]
// and the broker lookup path. Backends translate native "no such key" /
// "404" / "not found" responses into this so the broker and policy
// layer have a single error to match against.
var ErrSecretNotFound = errors.New("secrets: secret not found")

// ErrLeaseNotFound is the lease-side counterpart to [ErrSecretNotFound]
// — returned by [SecretBackend.RenewLease] and [SecretBackend.RevokeLease]
// when the lease ID isn't tracked.
var ErrLeaseNotFound = errors.New("secrets: lease not found")

// ErrLeaseExpired is returned when a renew arrives after the lease's
// expiry instant. Distinct from [ErrLeaseNotFound] so the broker can
// emit a different audit reason.
var ErrLeaseExpired = errors.New("secrets: lease expired")

// ErrLeaseNotRenewable is returned when a caller asks to renew a lease
// whose [LeaseInfo.Renewable] field is false. Backends that don't
// support renewal at all (Vault static KV, the encrypted-file backend
// for non-dynamic paths) also use this sentinel.
var ErrLeaseNotRenewable = errors.New("secrets: lease not renewable")

// ErrNotImplementedYet is the sentinel test doubles return for
// SecretBackend methods that aren't wired in a given test fixture.
// Epic 10 backends (encrypted-file, Vault) no longer return this
// from production code; the sentinel survives because it's the
// canonical "method-not-wired" signal that test rigs assert against
// via errors.Is.
var ErrNotImplementedYet = errors.New("secrets: backend method not wired (test fixture)")
