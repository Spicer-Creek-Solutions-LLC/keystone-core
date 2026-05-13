# Epic 09: Identity & Auth (Embedded CA, mTLS, Join Tokens)

**Phase**: F • **Estimate**: 2 weeks • **Depends on**: 02, 03 • **Blocks**: 10, 13 (mTLS for CoordinationService), 14 (capability auth)

## Goal

Real authentication and identity from day 1. Embedded CA + SVID issuer (SPIFFE-shaped from the start so v1.3 SPIRE swap-in is a provider change), API keys, mTLS, JWT, cluster join tokens, and the v1.0 minimum RBAC (admin/operator/readonly).

## Scope (in)

- `internal/identity/` — `Provider` interface (`Start/Stop`, `Health`, `TrustDomain`, `GetTrustBundle`, `WatchTrustBundle`, `Attest`, `IssueX509SVID`, `IssueJWTSVID`, `CreateJoinToken`, `ListJoinTokens`, `DeleteJoinToken`).
- **EmbeddedProvider** — wraps CA manager, SVID issuer, attestation engine, token store. Default trust domain `kscore.local`. Default SPIFFE IDs: `spiffe://kscore.local/server/control-plane`, `spiffe://kscore.local/agent/{id}`, `spiffe://kscore.local/service/{name}`.
- `internal/identity/ca.go` — `CAManager{rootCert, rootKey, signingCert, signingKey}`. Defaults: ECDSA-P256, 10y root TTL, 1y signing TTL, auto-rotate signing 30d before expiry. Methods: `Initialize`, `GetTrustChain`, `ShouldRotateSigningCA`, `RotateSigningCA`, `IssueCertificate(template, ttl)`.
- **Cert auto-rotation at ~50% lifetime** (agent-driven).
- TLS 1.3 default minimum; 1.2 opt-in for legacy.
- Cluster join tokens: SHA-256 + salt stored, plaintext returned on creation only; defaults TTL 5m, max-uses 1, 24h max TTL; one-time-use by default; hourly cleanup on leader.
- `kscore-identity` CLI: `token {create, list, revoke, cleanup}`, `ca {info, rotate-signing, export}`, `status`. v1.1 adds `federation {add-domain, list, fetch-bundle}`.
- Integration with `pkg/api/auth` (Epic 03) — wire embedded provider as the v1.0 default identity source.

## Scope (out / non-goals)

- Full RBAC role/permission CRUD with per-resource permissions — v1.2.
- Trust federation (bundle endpoint for cross-domain) — v1.1.
- SPIRE integration (external SPIRE server + agent socket) — v1.3.
- Cloud workload identity (AWS IAM/IRSA, GCP WI, Azure MI) — v2.0.
- Service mesh integration (Istio/Linkerd/Consul) — v2.x.
- Multi-party CA — v2.x.

## Design summary

See `PROJECT-DETAILS.md §4.10`.

## Tasks

1. **`SPIFFEID` type** — trust domain + path; helpers for agent/server/service paths.
   _(landed: `internal/identity.SPIFFEID` wraps [`github.com/spiffe/go-spiffe/v2/spiffeid`](https://pkg.go.dev/github.com/spiffe/go-spiffe/v2/spiffeid) for canonical SPIFFE-ID grammar compliance. Public surface: `ParseSPIFFEID` / `MustParseSPIFFEID` / `NewSPIFFEID`; accessors `TrustDomain` / `Path` / `Segments` / `URI` / `String` / `IsZero` / `Equal` / `MemberOf`; `MarshalText` + `UnmarshalText`; JSON support with zero-value → `null` semantics. Standard-path helpers `AgentID(td, id)` / `ServerID(td, name)` / `ServiceID(td, name)` against the §4.10 path scheme (`/agent/<id>`, `/server/<name>`, `/service/<name>`). Default trust domain `kscore.local` exposed as `DefaultTrustDomain`. Every grammar rejection wraps `ErrInvalidSPIFFEID` so callers can branch with `errors.Is`. Note: per RFC 3986 + SPIFFE-ID spec, path segments are case-preserving (`Agent` is a valid segment); trust domains are lowercase-only. 100% test coverage.)_
2. **`X509SVID{cert chain, private key, expiry, IssuedAt, Hint}`** + `Expired()` + `ShouldRotate()` (~50% lifetime).
   _(landed: `internal/identity.X509SVID` value type wrapping `[leaf, intermediate...]` + `crypto.Signer` + `Hint`. `NewX509SVID(id, chain, key, hint)` validates the leaf has exactly one URI SAN that parses as a SPIFFE ID and `Equal`s the declared `id`; the key's public half matches the leaf via the `Equal(crypto.PublicKey) bool` interface (Go 1.15+ on ECDSA / RSA / Ed25519); the chain is non-empty with no nil entries; `NotBefore ≤ NotAfter`. Accessors: `SPIFFEID` / `Leaf` / `Chain` (defensive copy) / `PrivateKey` / `IssuedAt` / `ExpiresAt` / `Lifetime` / `Hint` / `IsZero` / `Equal`. Predicates take an explicit `now` (the auto-rotation loop task 6 injects a test clock): `Expired(now)` is boundary-inclusive at `ExpiresAt`; `ShouldRotate(now)` flips true at the 50%-of-lifetime midpoint per §4.10. Edge cases: `now < IssuedAt` (clock skew at boot) → `ShouldRotate=false` (wait); `Lifetime() == 0` → `ShouldRotate=true` (defensive — force rotation, don't spin). `ErrInvalidX509SVID` wraps every rejection. 100% coverage; `-race` clean.)_
3. **`JWTSVID`** + signing + verification helpers.
   _(landed: `internal/identity.JWTSVID` wraps a SPIFFE JWT-SVID with accessors `SPIFFEID` / `Audience` / `IssuedAt` / `ExpiresAt` / `Lifetime` / `Claims` (defensive copies) / `Hint` / `Token` / `IsZero` / `Equal` plus the same `Expired(now)` / `ShouldRotate(now)` predicates as `X509SVID`. `SignJWTSVID(req)` signs via `go-jose/v4` (added as direct dep `v4.1.4`); algorithm is derived from the key type (ECDSA-P256/P384/P521 → ES256/ES384/ES512; RSA → RS256). Ed25519 is **rejected at sign time** because go-spiffe's verifier doesn't permit EdDSA per the SPIFFE JWT-SVID spec. `KeyID` is required (the spec mandates `kid`). `ExtraClaims` are merged but cannot shadow reserved JWT claims (`sub` / `aud` / `exp` / `iat` / `iss` / `nbf` / `jti`). `Audience` entries are trimmed and rejected if all-empty. `Now` is honoured when non-zero so rotation-loop tests inject a clock. `ParseJWTSVID(token, audience, jwtbundle.Source)` verifies via `jwtsvid.ParseAndValidate` — the `Source` interface is supplied by task 4's `TrustBundle`. `ParseJWTSVIDInsecure(token, audience)` skips signature verification (audience + expiry still checked) for diagnostics. All rejections wrap `ErrInvalidJWTSVID`. 98.3% coverage (the uncovered branches are defensive: in-library go-jose / go-spiffe internal failures, and an `int64` arm of the `iat` claim type switch that go-jose stores as `float64`). `-race` clean, lint clean.)_
4. **`TrustBundle{X509Authorities, JWTAuthorities, RefreshHint, SequenceNumber}`**.
   _(landed: `internal/identity.TrustBundle` wraps `*spiffebundle.Bundle`. Constructors `NewTrustBundle(td)` / `TrustBundleFromAuthorities(td, x509, jwt)` / `ParseTrustBundle(td, jwksBytes)`. Full mutating lifecycle on X509Authorities and JWTAuthorities (`Add` / `Remove` / `Set` / `Has` / `Find`); `Add` rejects nil cert / nil key / empty kid (upstream silently accepts those). `RefreshHint` + `SequenceNumber` getters return `(value, ok)` matching the spec's "optional" semantics; `Set` + `Clear` round-trip cleanly. `Marshal` emits SPIFFE JWKS-extended form; `ParseTrustBundle` round-trips. `Clone` deep-copies; `Equal` handles nil pairs. **`*TrustBundle` directly satisfies `x509bundle.Source` AND `jwtbundle.Source`** — `ParseJWTSVID(token, audience, bundle)` (task 3) and `x509svid.Verify(chain, bundle)` (task 13+) both take it without adapters. Foreign-trust-domain lookups return `ErrInvalidTrustBundle` (federation is v1.1 per the epic's out-of-scope list). All rejections wrap `ErrInvalidTrustBundle`. 98.2% coverage (the uncovered branches are defensive: in-library `spiffebundle.Marshal` JWK-serialize failures and an `AddJWTAuthority` error path our nil-check pre-empts). `-race` clean; lint clean.)_
5. **`CAManager`** — Initialize generates root + signing CAs, persists to configured storage path (with optional encryption key). `IssueCertificate` for end-entity certs with SPIFFE URI SAN.
   _(landed: `internal/identity.CAManager` + `internal/identity.FileCAStorage`. Two-tier CA: long-lived root (default ECDSA-P256, 10y TTL, `MaxPathLen=1`) signs the signing CA (1y TTL, `MaxPathLen=0`); the signing CA signs leaves. `CAConfig` exposes the §4.10 knobs (`KeyType` / `RootCATTL` / `SigningCATTL` / `RotateBefore` / `DefaultSVIDTTL` / `MaxSVIDTTL` / `Clock`) with `DefaultCAConfig(td)` for the spec defaults; `withDefaults()` + `validate()` enforce `RotateBefore < SigningCATTL` and `MaxSVIDTTL ≤ SigningCATTL`. `Initialize(ctx)` is idempotent: loads existing CAs from storage, or generates fresh ones + persists. `IssueCertificate(req)` clamps `TTL` to `MaxSVIDTTL`, populates the SPIFFE URI SAN, defaults KeyUsage/ExtKeyUsage to the §4.10 mTLS profile, returns `IssuedCertificate{Chain: [leaf, signingCA], Leaf}` — direct input for `NewX509SVID`. `GetTrustChain()` returns just the root; `BuildTrustBundle()` seeds a `*TrustBundle` with it. `ShouldRotateSigningCA(now)` mirrors the X509SVID predicate (boundary-inclusive at `expiry − RotateBefore`). `RotateSigningCA(ctx)` mints a fresh signing CA under the unchanged root and persists; old leaves keep verifying because their `Chain` still includes the old signing-CA cert which the unchanged root anchors. `FileCAStorage` writes PEM (cert `0644`, key `0600` PKCS#8) under a `0700` directory; `CAStorage` interface is the seam for the encryption-at-rest gate-v0.5 ROADMAP entry. Concurrent `IssueCertificate` calls under a `sync.RWMutex`; 128-bit random serials; serials are confirmed distinct under N=20 parallel issuance. All four key types (ECDSA-P256/P384, RSA-2048/4096). 93.4% coverage; `-race` clean.)_
6. **CA rotation** — background loop checks `ShouldRotateSigningCA()` hourly; rotate generates new signing CA, retains old for grace period.
7. **`EmbeddedProvider`** wiring — composes CA + SVID issuer + attestation + token store; implements `Provider` interface.
8. **Attestation** — pluggable; v1.0 default is `join_token` attestor (validates token from store, returns SPIFFE ID + selectors).
9. **`JoinToken`** types + storage (extends `internal/state.Store` with `JoinTokenStore` sub-interface or in-memory for dev).
10. **`CreateJoinToken/Get/MarkUsed/Delete/List/Cleanup`** with TTL + max-uses enforcement.
11. **Background token cleanup** loop (hourly on cluster leader; v1.0 single-server runs it always).
12. **`kscore-identity` CLI** with `token`, `ca`, `status` subcommands.
13. **Integration** with `pkg/api/auth.MTLSAuthenticator` for SPIFFE-aware peer extraction.
14. **Integration with NATS bootstrap** (Epic 05) — bootstrap PSK validates against join token store; agent gets full credentials.

## Acceptance criteria

- [ ] `kscore-server run` initializes embedded CA on first run; `~/.kscore/identity/` (or configured path) contains root + signing certs.
- [ ] `kscore-identity ca info` shows root + signing cert details, expiry, key type.
- [ ] `kscore-identity token create --ttl 5m` returns plaintext token; only hash persisted.
- [ ] Token honors `--max-uses 1` (rejects second use).
- [ ] Cluster join uses token; receives SPIFFE-IDed agent cert; reconnects with full credentials.
- [ ] CA signing cert auto-rotates 30d before expiry (test with short TTLs).
- [ ] mTLS connections succeed with SPIFFE URI SAN on certs.
- [ ] TLS 1.3 enforced on gRPC; 1.2 only when explicitly configured.
- [ ] API key + JWT auth round-trip in integration test.
- [ ] Coverage >85% on `internal/identity`; >80% on `pkg/api/auth`.

## Risks

- **Cert rotation under clock skew** — grace period must exceed expected fleet skew. Document NTP.
- **API key timing attacks** — constant-time hash compare (use `crypto/subtle`).
- **mTLS with NATS** — NATS leaf certs (v2.0) are separate from agent identity certs; v1.0 doesn't have leaf so this is documented for future.
- **Token cleanup on cluster** — hourly cleanup runs on leader; v1.0 single-CP runs always; document.
- **JWT role claim** — missing → readonly fallback (with warning); invalid string → reject (don't default).

## References

- PROJECT-DETAILS §4.10.
