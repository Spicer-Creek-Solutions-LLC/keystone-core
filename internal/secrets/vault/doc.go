// Package vault is the HashiCorp Vault [SecretBackend] per
// PROJECT-DETAILS §4.11 — the high-ROI v1.0 backend for trial
// deployments that already run Vault. Talks the canonical Vault
// HTTP API via [github.com/hashicorp/vault/api]; auth methods are
// Token / AppRole / Kubernetes / LDAP at v1.0.
//
// What this backend supports:
//
//   - [secrets.CapKV]: KV v1 and KV v2. Mount-version routing is
//     config-driven via [MountConfig]; unlisted mounts default to
//     KV v2. CAS is honoured on KV v2; KV v1 silently ignores it.
//   - [secrets.CapList]: enumerate paths under a prefix; metadata-only.
//   - [secrets.CapDynamic]: issue against any Vault engine —
//     database / aws / gcp / pki / ssh — by passing the engine's
//     canonical path as `req.Path` (e.g. `database/creds/app`,
//     `pki/issue/role`). The backend is engine-agnostic at v1.0.
//   - [secrets.CapLeaseRenew] / [secrets.CapLeaseRevoke]: via
//     `sys/leases/renew` and `sys/leases/revoke`. Idempotent revoke.
//   - [secrets.CapTransit]: encryption-as-a-service per
//     [secrets.TransitBackend] — encrypt / decrypt / sign / verify /
//     HMAC / rewrap / data-key generation, each with single + batch
//     variants and convergent / key-version support.
//
// Auth methods deferred to ROADMAP:
//
//   - AWS IAM — pulls in `aws-sdk-go-v2/credentials/stscreds`;
//     gate-v1.0 entry "Vault AWS IAM auth method" tracks the work.
//
// Operational behavior:
//
//   - Background token renewer runs when the auth token is renewable;
//     renewal failures log WARN and the backend's `Health` reports
//     unhealthy. Auto re-authentication on token expiry is a v0.x
//     ROADMAP entry.
//   - `Stop` issues a best-effort `auth/token/revoke-self` so Vault's
//     audit trail stays clean.
//   - Vault Enterprise namespaces honour [Config.Namespace] (single
//     top-level namespace per backend; deeper hierarchies are v1.x).
//
// Error translation: the package's [translateError] funnels Vault's
// HTTP / API errors into the [secrets] sentinel family:
//
//   - 404 on KV reads → [secrets.ErrSecretNotFound]
//   - 403 anywhere → [secrets.ErrInvalidBackend] (permission denied)
//   - sys/leases failures → [secrets.ErrLeaseNotFound] /
//     [secrets.ErrLeaseExpired] / [secrets.ErrLeaseNotRenewable]
//   - other HTTP errors → [secrets.ErrInvalidBackend] with the
//     Vault response detail preserved
//
// Test posture: day-to-day unit tests run against `net/http/httptest`
// servers that return canned Vault responses. The
// `TestVaultBackend_Integration*` family is gated behind
// `KSCORE_TEST_VAULT_ADDR` and exercises a real `vault dev` server;
// CI integration ships with task 12 / Epic 19 (test harness).
package vault
