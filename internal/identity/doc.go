// Package identity is the v0.x reconstruction of Keystone Core's
// identity surface per PROJECT-DETAILS §4.10. The epic-09 design is
// two-mode by construction: an **embedded** provider ships in v0.1
// (built-in CA, SVID issuer, attestation engine, join-token store);
// a **SPIRE** provider lands in v1.3 and swaps in behind the same
// [Provider] interface (tasks 7-8). Higher layers — the API auth
// pipeline, gRPC interceptors, NATS bootstrap, mTLS peer extraction
// — depend only on this package's types and never on a specific
// backend.
//
// Why SPIFFE-shaped from day 1: even with the embedded backend, every
// x509 SVID carries a SPIFFE URI SAN, every JWT SVID carries a SPIFFE
// audience, every join token records the SPIFFE ID it will issue.
// When v1.3 brings SPIRE, the consumer types do not move — only the
// [Provider] implementation changes.
//
// Task 1 lands the [SPIFFEID] value type that the rest of the epic
// builds on:
//
//   - Task 2 — [X509SVID]{cert chain, private key, expiry, IssuedAt, Hint}
//     plus [X509SVID.Expired] / [X509SVID.ShouldRotate].
//   - Task 3 — [JWTSVID] + signing / verification helpers.
//   - Task 4 — [TrustBundle]{X509Authorities, JWTAuthorities,
//     RefreshHint, SequenceNumber}.
//   - Task 5 — [CAManager] (Initialize / GetTrustChain / IssueCertificate
//     with SPIFFE URI SAN / ShouldRotateSigningCA / RotateSigningCA).
//   - Task 6 — background CA rotation.
//   - Task 7 — [EmbeddedProvider] that composes the pieces and
//     implements the [Provider] interface.
//   - Task 8 — pluggable attestation; v0.1 default is the join-token
//     attestor.
//   - Tasks 9-11 — [JoinToken] storage + lifecycle (TTL, max-uses,
//     hourly cleanup on the cluster leader).
//   - Task 12 — `kscore-identity` CLI (`token`, `ca`, `status`).
//   - Tasks 13-14 — integration with `pkg/api/auth` (mTLS SPIFFE peer
//     extraction) and the NATS bootstrap handler (Epic 05).
//
// This file declares the package; the actual type lives in
// spiffeid.go. The package wraps [github.com/spiffe/go-spiffe/v2]'s
// canonical parser at the type boundary so the grammar stays
// standards-compliant and a future SPIRE provider (which speaks the
// same upstream types natively) can convert at the seam without
// shape mismatches.
package identity
