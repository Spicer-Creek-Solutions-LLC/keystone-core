# Epic 10: Secrets Management

**Phase**: F • **Estimate**: 1.5 weeks • **Depends on**: 02, 03, 09 • **Blocks**: 14, 15

## Goal

Unified broker for secret retrieval, lease management, and encryption-as-a-service. Encrypted-file backend (zero-deps) and HashiCorp Vault backend (most common in trial environments). Lease lifecycle, transit ops, encrypted in-memory cache, audit emission.

## Scope (in)

- `internal/secrets/` broker:
  - `SecretBroker` with **path-prefix routing** (longest-match-first).
  - `SecretBackend` interface: `GetSecret`, `WriteSecret`, `ListSecrets`, `DeleteSecret`, `IssueDynamicSecret`, `RenewLease`, `RevokeLease`.
  - Audit hook: every operation emits `secret.access` event with agent ID, SPIFFE ID, action, path, timestamp, duration; sensitive data masked via `LogMasker`.
- v1.0 backends:
  - **Encrypted-file** (`internal/secrets/file/`): AES-GCM, JSON serialization, master key from env or KMS. Zero external deps.
  - **HashiCorp Vault** (`internal/secrets/vault/`): KV v1/v2, dynamic secrets (DB, IAM, PKI, SSH), transit, namespace support, all auth methods (token, AppRole, K8s, AWS IAM, LDAP).
- `LeaseManager`:
  - Persistent SQLite store (`pkg/secrets`-shaped types `LeaseInfo`, `Lease`).
  - In-memory tracking; background scheduler with strategy `eager` (50% TTL), `lazy` (90% TTL), `on_demand`.
  - Renewal callbacks for lifecycle events.
  - Bulk ops: list, revoke, expire-cleanup.
- `TransitBackend` (Vault transit engine): encrypt/decrypt/sign/verify/HMAC; batch ops; key versioning; convergent option; data-key generation for envelope encryption; rewrap.
- `SecretCache`: in-memory L1, AES-GCM at-rest, TTL eviction (default 5m), prefix-deletion on revoke; bounded-LRU; stats (hits/misses/evictions/expired/memory).
- `pkg/secrets` client lib + types (`Secret`, `Lease`, errors) for in-process consumers.
- `pkg/api/secrets/` REST handlers + gRPC `SecretsService` impl: `GetSecret`, `ListSecrets`, `WriteSecret`, `DeleteSecret`, `GetLease`, `ListLeases`, `RenewLease`, `RevokeLease`, `Encrypt`, `Decrypt`, `Sign`, `Verify`.
- `cmd/kscore-secrets` CLI: `get`, `list`, `backends`, `audit`, `dynamic`, `leases`, `cache`, `encrypt`, `decrypt`, `template`.
- Secret masking in API responses (`***`) — cleartext only on creation/get-for-use; never in list responses.

## Scope (out / non-goals)

- Rotation orchestration with strategies (blue-green/rolling/canary/immediate; health checks; auto-rollback) — v1.4.
- Cron-based rotation scheduling + Slack/PagerDuty notifications — v1.4.
- Compliance reports + anomaly detection — v1.4.
- AWS Secrets Manager / Azure Key Vault / GCP Secret Manager backends — v2.0.
- Cloud KMS for master keys — v2.0.
- Hardware HSM (PKCS#11) — v2.x.
- L2 KMS-backed cache — v2.0.

## Design summary

See `PROJECT-DETAILS.md §4.11`.

## Tasks

1. **`SecretBackend` interface** + types (`Secret`, `Lease`, `RotationPolicy` placeholder).
2. **Path-prefix routing engine** — longest-match-first; tests for ambiguous configs.
3. **`SecretBroker`** with cache integration + audit emission.
4. **Encrypted-file backend** with AES-GCM encryption-at-rest + write-ahead approach for crash safety.
5. **Vault backend** — KV v1/v2 + dynamic secrets + transit + auth methods.
6. **`LeaseManager`** with persistent SQLite store + scheduler + strategies + callbacks.
7. **`TransitBackend`** with encrypt/decrypt/sign/verify/HMAC + batch + convergent + key versioning.
8. **`SecretCache`** with AES-GCM + TTL + LRU + prefix-delete + stats.
9. **gRPC `SecretsService` impl** + REST handlers.
10. **`cmd/kscore-secrets`** CLI.
11. **Audit emission** integration (Epic 12 uses these events).
12. **Integration test**: encrypted-file backend round-trip; Vault backend with `vault dev` server.

## Acceptance criteria

- [ ] Encrypted-file backend: write secret → read secret → delete secret round-trips with AES-GCM authenticated encryption.
- [ ] Vault backend (with `vault dev` server) — KV v2 read/write, dynamic DB credential issuance, lease renewal, lease revoke.
- [ ] Path-prefix routing: `kv/app/db` routes to file backend; `secret/data/foo` routes to Vault per config.
- [ ] Lease manager: dynamic secret issued → tracked → renewed at threshold → revoked on demand → removed from tracking.
- [ ] Transit ops: encrypt → decrypt round-trips; signed message verifies; batch encrypt 100 plaintexts in <1s with Vault.
- [ ] Cache hit-rate >80% on second access of same secret; cache invalidates on prefix-delete.
- [ ] All operations emit `secret.access` event in audit log with masked values.
- [ ] CLI: `kscore-secrets get path` returns cleartext (with audit log entry); `list` shows path + metadata only.
- [ ] Coverage >75% on `internal/secrets`; >80% on `pkg/secrets`.

## Risks

- **Cache invalidation on backend update** — explicit (refresh param or operator clear). Document.
- **Master-key rotation** breaks cache — mitigate with dual-key window.
- **Transit round-trips** add 50–200ms — batch ops materially help.
- **Lease renewal storms** — add jitter (default scheduler has it).
- **Audit log disk-full** — circuit-break + alert is mandatory.
- **Vault test dependency** — gate Vault tests behind `KSCORE_TEST_VAULT_ADDR`; CI runs `vault dev` in docker-compose.

## References

- PROJECT-DETAILS §4.11, §4.12 (audit).
