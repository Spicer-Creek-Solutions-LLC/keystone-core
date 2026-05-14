// Package secrets is the v0.x reconstruction of Keystone Core's
// secrets surface per PROJECT-DETAILS §4.11. The epic-10 design is
// backend-pluggable by construction: every concrete store (encrypted
// file, HashiCorp Vault, future AWS/Azure/GCP — v2.0) implements
// [SecretBackend], and a single [SecretBroker] routes by path-prefix
// (longest-match-first; tasks 2-3) so higher layers — the REST/gRPC
// service, the `kscore-secrets` CLI, the in-process consumers in
// pkg/secrets — depend only on this package's types and never on a
// specific backend.
//
// Why types-first: secrets ops cross every boundary that matters for
// compliance (audit, mTLS-authenticated callers, leased dynamic
// credentials), so the wire-equivalent value types ([Secret], [Lease],
// [LeaseInfo]) and the seam they ride on ([SecretBackend]) need to be
// stable before any backend implementation lands. Task 1 ships those
// shapes; backend stubs that return [ErrNotImplementedYet] anchor the
// interface until tasks 4-7 fill them in.
//
// Task 1 lands the [SecretBackend] interface plus the core value types:
//
//   - [Secret] — payload, metadata, optional lease handle.
//   - [LeaseInfo] / [Lease] — lifecycle state for dynamic / leased
//     secrets; [RenewStrategy] (`eager` 50% TTL, `lazy` 90% TTL,
//     `on_demand`) drives the scheduler that arrives in task 6.
//   - [RotationPolicy] — v1.0 placeholder; full rotation orchestration
//     is v1.4 per `epics/10-secrets.md` Scope-out + `docs/project/ROADMAP.md`.
//   - [BackendCapability] — backends declare which interface methods
//     they meaningfully implement; the broker (task 3) uses this to
//     refuse routes a backend can't serve.
//
// Roadmap of the rest of the epic, anchored on the types declared here:
//
//   - Task 2 — path-prefix routing engine; longest-match-first; tests
//     for ambiguous configs.
//   - Task 3 — [SecretBroker] with cache integration + audit emission.
//   - Task 4 — encrypted-file backend (AES-GCM, write-ahead for crash
//     safety, zero external deps).
//   - Task 5 — HashiCorp Vault backend (KV v1/v2, dynamic secrets,
//     transit, namespace support, every auth method).
//   - Task 6 — `LeaseManager` (persistent SQLite store + scheduler
//     keyed on [RenewStrategy] + renewal callbacks).
//   - Task 7 — `TransitBackend` (encryption-as-a-service via Vault
//     transit; encrypt/decrypt/sign/verify/HMAC; batch; convergent;
//     key versioning).
//   - Task 8 — `SecretCache` (in-memory L1, AES-GCM at-rest, TTL
//     eviction, prefix-deletion, bounded LRU).
//   - Task 9 — gRPC `SecretsService` impl + REST handlers in
//     `pkg/api/secrets/` (currently a scaffold from epic 03).
//   - Task 10 — `cmd/kscore-secrets` CLI.
//   - Task 11 — audit emission integration (consumed by epic 12).
//   - Task 12 — integration tests: encrypted-file round-trip + Vault
//     backend against a `vault dev` server.
//
// The package wraps `crypto/aes` + `crypto/cipher` for backend-side
// authenticated encryption (task 4 onward) and `github.com/hashicorp/vault/api`
// (task 5) at the backend boundary, so a future cloud-KMS or HSM
// backend (v2.x) plugs in at the same seam without dragging SDK
// weight into v1.0 binaries.
package secrets
