// Package file is the zero-deps encrypted-file [SecretBackend] per
// PROJECT-DETAILS §4.11 — the v1.0 trial-day-1 backend that doesn't
// require Vault. Authenticated encryption (AES-256-GCM) protects the
// cleartext at rest; a single framed envelope per file means
// crash-safe atomic writes via temp-file + `os.Rename`.
//
// The cleartext schema is a small JSON document: `fileState{Version,
// Secrets map[path]storedSecret}`. The backend holds the decoded
// state in memory while running (same posture as Vault unseal — any
// secrets system has secrets in process memory) and rewrites the
// whole file on every mutation. Realistic v1.0 deployments hold
// hundreds of secrets, not millions; the rewrite cost is dwarfed by
// the disk fsync and a per-path layout adds complexity without a v1.0
// win.
//
// Master-key resolution lives in [ResolveMasterKey]; v1.0 schemes are
// `env:`, `file:`, `inline:`. Cloud KMS schemes (`gcp-kms:` /
// `aws-kms:` / `azure-kv:`) are detected and rejected with a v2.x+
// pointer per FEATURES.md.
//
// What this backend supports:
//
//   - [secrets.CapKV]: GetSecret / WriteSecret / DeleteSecret.
//   - [secrets.CapList]: ListSecrets with prefix + cursor pagination.
//
// What it does NOT support (and explicitly returns
// [secrets.ErrInvalidBackend] for, defense-in-depth against direct
// callers that bypass the broker's capability check):
//
//   - Dynamic secrets — Vault backend (task 5).
//   - Transit ops — Vault backend (task 7).
//   - Leases — the file backend issues only static KV; renew / revoke
//     are not applicable.
//
// Crash safety: every mutation writes to `<path>.tmp` → fsync → close
// → `os.Rename(tmp, path)` (atomic on POSIX). On Start, any leftover
// `<path>.tmp` is removed as a half-written write. The temp file IS
// the write-ahead log — no separate WAL needed.
//
// Per-path history (KV v2 multi-version semantics) is intentionally
// out of scope: operators that need history use the Vault backend.
//
// Online master-key rotation is tracked under the gate-v1.0
// `docs/project/ROADMAP.md` entry "Encrypted-file master-key rotation
// tooling" — v0.1 procedure is operator-driven (stop → re-encrypt via
// CLI → start with new key).
package file
