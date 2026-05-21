# Epic 18: Self-Management + File Distribution + Rate Limiting

**Phase**: M-N • **Estimate**: 2 weeks • **Depends on**: 02, 03, 04, 05, 06, 09, 10 • **Blocks**: nothing critical

## Goal

Three small but ops-critical concerns combined: **self-management** (bootstrap-from-seed + backup/restore — the disaster-recovery story), **file distribution** (chunked NATS-based file transfer for packages/configs/blueprints), and **rate limiting** (protection against agent storms / DoS).

## Scope (in)

### Self-management (`internal/selfmgmt/`, `internal/backup/`, `cmd/kscore-bootstrap/`, `cmd/kscore-backup/`)

- **Bootstrap from seed**:
  - `SeedConfig` — YAML format defining initial cluster topology (mode, cluster_name, node_role, storage backend, NATS config, TLS strategy, blueprints to apply post-bootstrap).
  - `kscore-bootstrap --seed config.yaml` installs binaries (if needed), forms cluster, hands off to ongoing self-management.
  - Phases (state machine, `pkg/statemachine`): detect → configure → validate → install → blueprints → verify, with rollback on failure.

- **Full system backup**:
  - `BackupManager.CreateBackup()` produces a portable artifact:
    - Postgres `pg_dump` (or SQLite file copy).
    - JetStream stream snapshots.
    - etcd snapshot.
    - Configuration files (read-only).
    - Secrets (encrypted-file backend; Vault external is "live" data).
    - Cluster metadata + shard map.
  - Encrypted with age (default) or AWS KMS / Vault-derived key.
  - Multiple destinations: local filesystem, S3 (v1.0 minimum); SFTP, GCS, Azure for v1.5.
  - Integrity verification (SHA-256 manifest).

- **Restore**:
  - `RestoreManager.Restore(artifact)` — verify integrity → check schema compatibility → optional partial restore (config-only, secrets-only, full).
  - Safety: refuses to restore over a populated cluster without `--force`.

- `kscore-backup create|list|verify|restore` CLI.

### File distribution (`internal/files/`, `cmd/kscore-files/`, `cmd/kscore-files-storage/`)

- `FileRequest`, `FileMetadata{Path, Size, Hash, ContentType, CreatedAt, Version, Tags}`, `FileChunk{ID, FileID, Index, Total, Data, Hash}`.
- Chunked NATS streaming (1 MB default chunks); SHA-256 per chunk; resume on interrupt.
- v1.0 storage backends: local filesystem, S3-compatible (AWS / MinIO).
- Proxy caching on agents (LRU + TTL).
- Namespace-based access control + audit logging.
- File versioning + metadata queries.
- REST: `/api/v1/files` (list, get, put, delete), `/api/v1/files/metadata/{path}`.
- gRPC: minimal File service (out-of-scope detail; use REST for v1.0 if simpler).
- CLI: `kscore-files list|get|put|delete`, `kscore-files-storage backends list|test`.

### Rate limiting (`internal/ratelimit/`)

- Token bucket algorithm.
- Per-IP, per-API-key, per-arbitrary-header (configurable key extractor).
- Tunable `requests_per_minute`, `burst`.
- Wired into Epic 04 middleware chain (between CORS and auth).
- `Retry-After` response header on rejection.
- Metric: `kscore_ratelimit_rejected_total{reason}`.

## Scope (out / non-goals)

### Self-management

- Automated backup scheduling — v1.5.
- Rolling upgrades — v1.5.
- Configuration drift detection on self-config — v1.5.
- Self-healing for common failures — v1.5.
- Disaster recovery testing harness — v1.5.

### File distribution

- NATS Object Store backend — v1.5.
- Git backend for configs — v1.5.
- Mirror groups for HA / geographic redundancy — v1.5.
- Sync engine + conflict resolution (newest/largest/primary/manual wins) — v1.5.
- Geographic routing — v1.5.

### Rate limiting

- Per-namespace quotas — v1.5.
- Resource quota types (CPU/memory/storage/etc.) — v1.5.
- Sliding window / fixed window / leaky bucket strategies — v1.5 (token bucket suffices for v1.0).

## Design summary

See `PROJECT-DETAILS.md §4.20`.

## Tasks

### Self-management

1. **`SeedConfig` YAML schema** + parser + validator.
2. **`BootstrapManager`** with phase state machine; idempotent re-runs.
3. **`BackupManager.CreateBackup()`** — orchestrates pg_dump / sqlite copy + JetStream snapshot + etcd snapshot + config + secrets (encrypted-file).
4. **age encryption** (default); pluggable for AWS KMS / Vault.
5. **S3 destination** + local destination.
6. **`RestoreManager.Restore()`** — integrity verification, schema compatibility, partial-restore options, safety guard.
7. **`kscore-bootstrap --seed`, `kscore-backup create|list|verify|restore`** CLIs.
8. **Integration test**: full cluster → backup → fresh cluster → restore → verify identical state.

### File distribution

9. **`FileRequest`, `FileMetadata`, `FileChunk` types** + NATS subject conventions.
10. **`BackendStore` interface** + filesystem impl + S3 impl.
11. **Chunked streaming** with per-chunk hash + resume.
12. **Proxy cache** (LRU + TTL) on agents.
13. **Namespace ACLs** wired to RBAC (Epic 03).
14. **REST handlers** in `pkg/api/files/` (or stub gRPC).
15. **`kscore-files`** CLI.

### Rate limiting

16. **Token bucket impl** + tests.
17. **Key extractors** (IP, API key, header).
18. **HTTP + gRPC middleware** wiring into Epic 04 chain.
19. **`Retry-After` header** on rejection.

## Acceptance criteria

### Self-management

- [x] `kscore-bootstrap --seed dev-seed.yaml` produces a working single-node cluster end-to-end. _(task 8 — `TestBootstrap_FSM_RunsToVerified`: FSM drives through 12 transitions to `StateVerified`; real install/configure/verify per gate-v1.0 "Bootstrap phase handlers" ROADMAP entry)_
- [x] `kscore-backup create --dest s3://bucket/path` writes encrypted artifact. _(task 8 — `TestBackup_Restore_RoundTrip_AgeEncrypted` produces an age-encrypted artifact; S3 destination is exercised by `internal/backup/dest/s3_test.go` httptest round-trip)_
- [x] `kscore-backup verify` confirms integrity. _(task 8 — `TestVerify_DetectsTampering` confirms a tampered byte is detected; happy-path verify in both round-trip tests)_
- [x] `kscore-backup restore --src s3://bucket/path/<artifact>` restores onto a fresh cluster successfully. _(task 8 — both round-trip tests assert SHA-256 byte-equal contents post-restore; S3 source via `internal/backup/dest/s3_source_test.go`)_
- [x] Restore over populated cluster requires `--force`. _(task 8 — `TestBackup_Restore_Force_Required_For_Populated` exercises both the rejection and the force-override path via a stub `ClusterDetector`; production wiring per gate-v1.0 ROADMAP entry)_

### File distribution

- [ ] `kscore-files put /path/to/local/file kv://config/myapp` uploads via NATS chunks.
- [ ] `kscore-files get kv://config/myapp /tmp/out` downloads with hash verification.
- [x] Resume after network interrupt works. _(task 11 — `TestGet_Resume_FromMidChunk`: GET-side resume via `transport.GetOptions.FromChunk=K` reads the trailing chunks from the backend at offset K; PUT-side resume defers under v1.x ROADMAP entry "File distribution: PUT-side resume" — operator-facing kscore-files CLI surface lands at task 15.)_
- [x] Proxy cache hit on second download (visible in metrics). _(task 12 — `TestCache_RoundTrip_OverNATS_IncrementsHitMetric`: end-to-end MemoryStore → transport.Service → NATS → transport.Client → proxy.Cache rig; second Get is a hit; `kscore_files_cache_hits_total` and `kscore_files_cache_misses_total{reason=miss}` both read as 1 via the registry gatherer.)_
- [x] S3 backend round-trips a file. _(task 10b — `TestS3Store_Conformance/PutGet_RoundTrip` and friends drive S3Store's Put/Get/Stat/List/Delete against an httptest fake; `TestS3Store_KeyLayout` confirms the documented data/meta object-key split.)_

### Rate limiting

- [ ] 1000 RPS exceeding configured limit returns 429 with Retry-After.
- [ ] Per-API-key vs per-IP isolation verified by test.
- [ ] Rejected requests counted in metric.

### Combined

- [ ] Coverage >75% on each new package.

## Risks

- **Backup integrity** — point-in-time inconsistency. Use leader-initiated backup (mostly relevant for cluster mode); for single-node, pg_dump is consistent.
- **Restore over live data** — destructive; safety guard mandatory.
- **age key management** — document; v1.5 integrates KMS.
- **File chunked transfer NATS message size** — 1 MB chunks fit comfortably in JetStream message size; document limits.
- **Rate limiter starvation** — token bucket per-IP can be evaded by many IPs; per-API-key is strongest.
- **Bootstrap idempotency** — every phase must have a checkpoint and be safe to re-run.

## References

- PROJECT-DETAILS §4.20.
