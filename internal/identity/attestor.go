package identity

import (
	"context"
	"errors"
)

// Attestor is the v0.1 pluggable attestation surface. Each
// concrete attestor handles one [AttestorType]; [EmbeddedProvider]
// dispatches incoming [AttestRequest]s to the matching impl by
// type. Adding a new attestor (TPM, kubelet, IMDS, …) is a new
// file + a single entry in [EmbeddedProviderConfig.Attestors].
//
// v0.1 ships exactly one Attestor — [JoinTokenAttestor] —
// because it's the only attestation strategy the embedded
// provider needs to satisfy the §4.10 acceptance bar. Future
// attestors are v1.1+ scope.
type Attestor interface {
	// Type identifies the AttestRequest.Type values this attestor
	// handles. Two attestors registered with the provider MUST
	// NOT share a Type — [NewEmbeddedProvider] rejects duplicates.
	Type() AttestorType

	// Attest validates the attestor-specific Data payload and
	// returns the attested SPIFFE ID + the selectors the attestor
	// extracted from the evidence. Returns a non-nil error
	// wrapping [ErrAttestation] on any rejection.
	Attest(ctx context.Context, data []byte) (*AttestResult, error)
}

// ErrAttestation wraps every attestor rejection. Each attestor
// MAY wrap a more specific sub-sentinel (e.g.
// [ErrJoinTokenExpired]) so call sites can distinguish.
var ErrAttestation = errors.New("identity: attestation rejected")

// ErrAttestorNotConfigured is returned by
// [EmbeddedProvider.Attest] when no attestor in the provider's
// registry handles the requested type. Distinct from
// [ErrAttestation] — the request was not attempted, the operator
// just hasn't wired up that attestor.
var ErrAttestorNotConfigured = errors.New("identity: no attestor configured for type")
