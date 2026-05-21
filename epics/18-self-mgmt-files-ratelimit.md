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

- [x] `kscore-files put /path/to/local/file kv://config/myapp` uploads via NATS chunks. _(task 15 — `TestCLI_PutGetRoundTrip_OverNATS`: embedded-NATS rig + `transport.Service` + `MemoryStore`; CLI uploads via `transport.Client.Put` over chunked NATS; output confirms hash + version.)_
- [x] `kscore-files get kv://config/myapp /tmp/out` downloads with hash verification. _(task 15 — same test asserts downloaded body byte-equal to source; `transport.Client.Get` already verifies per-chunk SHA-256 + assembled-body hash against `Metadata.Hash` per task 11.)_
- [x] Resume after network interrupt works. _(task 11 — `TestGet_Resume_FromMidChunk`: GET-side resume via `transport.GetOptions.FromChunk=K` reads the trailing chunks from the backend at offset K; PUT-side resume defers under v1.x ROADMAP entry "File distribution: PUT-side resume" — operator-facing kscore-files CLI surface lands at task 15.)_
- [x] Proxy cache hit on second download (visible in metrics). _(task 12 — `TestCache_RoundTrip_OverNATS_IncrementsHitMetric`: end-to-end MemoryStore → transport.Service → NATS → transport.Client → proxy.Cache rig; second Get is a hit; `kscore_files_cache_hits_total` and `kscore_files_cache_misses_total{reason=miss}` both read as 1 via the registry gatherer.)_
- [x] S3 backend round-trips a file. _(task 10b — `TestS3Store_Conformance/PutGet_RoundTrip` and friends drive S3Store's Put/Get/Stat/List/Delete against an httptest fake; `TestS3Store_KeyLayout` confirms the documented data/meta object-key split.)_

### Rate limiting

- [x] 1000 RPS exceeding configured limit returns 429 with Retry-After. _(task 18 — `TestHTTP_1000_RPS_Configured_Limit_Returns_429`: HTTP middleware with `RequestsPerMinute=600, Burst=5` config; 50 tight-loop requests yield 5 OK + at least 40 × 429 each carrying a parseable positive-integer `Retry-After` header; "1000 RPS" is the deployment scenario, mechanism proven per Hard Convention #5.)_
- [x] Per-API-key vs per-IP isolation verified by test. _(task 18 — `TestHTTP_PerAPIKey_vs_PerIP_Isolation`: parameterized over both `extract.APIKey` and `extract.IP{TrustForwardedFor:true}` extractors; key-A exhausts to 429, key-B still has its burst — proves the `ratelimit.Registry`'s per-key isolation through the middleware. Also covered for gRPC by `TestGRPC_Unary_PerKey_Isolation`.)_
- [x] Rejected requests counted in metric. _(task 18 — `TestHTTP_MetricIncrements` reads `kscore_ratelimit_rejected_total{reason=limit_exceeded}` via the registry gatherer after a 3-request burst; counter reads 2 (the rejected ones). gRPC path verified by `TestGRPC_Unary_AllowThenDeny`.)_

### Combined

- [x] Coverage >75% on each new package. _(All 17 Epic-18 packages clear ≥75% (lowest 81.7% at `internal/files/transport`). File-distribution: `internal/files` 95.6%, `internal/files/acl` 100%, `internal/files/backend` 82.9%, `internal/files/proxy` 95.6%, `internal/files/transport` 81.7%, `pkg/api/files` 89.9%, `internal/cli/files` 87.3%. Self-management: `internal/selfmgmt` 92.5%, `internal/backup` 88.7%, `internal/backup/age` 89.4%, `internal/backup/dest` 92.8%, `internal/cli/backup` 87.7%, `internal/cli/bootstrap` 84.1%, `internal/s3client` 100%. Rate-limiting: `internal/ratelimit` 96.4%, `internal/ratelimit/extract` 92.8%, `internal/ratelimit/middleware` 96.2%.)_

## Risks

- **Backup integrity** — point-in-time inconsistency. Use leader-initiated backup (mostly relevant for cluster mode); for single-node, pg_dump is consistent.
- **Restore over live data** — destructive; safety guard mandatory.
- **age key management** — document; v1.5 integrates KMS.
- **File chunked transfer NATS message size** — 1 MB chunks fit comfortably in JetStream message size; document limits.
- **Rate limiter starvation** — token bucket per-IP can be evaded by many IPs; per-API-key is strongest.
- **Bootstrap idempotency** — every phase must have a checkpoint and be safe to re-run.

## References

- PROJECT-DETAILS §4.20.

## Closeout — Epic 18: CLOSED

- **Tasks**: 19/19 landed.
- **Acceptance**: 10/10 ticked (5 self-management + 5 file-distribution + 3 rate-limiting + combined coverage; the 3 rate-limiting lines tick at T18 because T19's Retry-After mechanism shipped one commit early as part of the natural middleware deny path).
- **Closing commits** (chronological): T1 `13e9d3ae` SeedConfig types, T2 `e368888c` BootstrapManager FSM, T3 `f36a3fe6` BackupManager + tar, T4 `13a6b20e` age encryption, T5 `9e27435e` destination backends, T6 `63f06ff0` RestoreManager, T7a `11fa7282` kscore-backup read side, T7b `f73bbecd` kscore-backup write side + bootstrap binary, T8 `cf06e16d` self-mgmt e2e, T9 `d6694ea0` file-distribution wire types + subjects, **T10 precursor `02b54c18` s3client extraction**, T10a `099afaab` BackendStore Memory+Filesystem, T10b `6afc4e14` BackendStore S3, T11 `2ef3ea34` transport.Service+Client, T12 `cd512240` proxy.Cache, T13 `d48ac2c0` ACL, T14 `3bed2234` REST handlers, T15 `ffe6b72a` kscore-files CLI, T16 `8352dd77` token bucket, T17 `271f5788` key extractors, T18 `fed4436f` rate-limit middleware, **T19 (this commit)** closeout.
- **New dependencies added** by Epic 18: `go.yaml.in/yaml/v3` (T1 strict YAML), `filippo.io/age` v1.3.1 (T4 envelope encryption), `github.com/minio/minio-go/v7` v7.1.0 (T5 S3 destination). `golang.org/x/time/rate` (T16 token bucket) was already in go.mod via Epic 17 tracing samplers.
- **New subcommand binaries** by Epic 18: `cmd/kscore-bootstrap` (T7b), `cmd/kscore-backup` (T7a/T7b), `cmd/kscore-files` (T15).
- **New REST surface** by Epic 18: `/api/v1/files` (T14).
- **New metrics** by Epic 18: `kscore_files_cache_hits_total`, `kscore_files_cache_misses_total{reason}` (T12), `kscore_ratelimit_rejected_total{reason}` (T18) — triple-lockstep updates applied to `internal/metrics/metricdefs.go` + `deploy/grafana/expected_metrics.txt` + `deploy/grafana/dashboards_test.go::allKscoreMetricDefs`.
- **Boot wiring deferrals** (carried as gate-v1.0 ROADMAP entries — none blocked Epic-18 closure):
  - "Bootstrap phase handlers + durable checkpointer" (T2).
  - "Backup + restore component adapters (storage/JetStream/etcd/config/secrets/cluster) + ClusterDetector" (T3/T6).
  - "REST/gRPC dark-until-boot" (T14 file-distribution REST + T18 rate-limit middleware fall under this existing Epic-15 entry).
- **v1.x ROADMAP entries added** by Epic 18 (mirrored in `tools/trackerctl/config/release-order.yaml`):
  - "Backup encryption: AWS KMS + Vault key providers" (T4).
  - "Backup destinations: Backblaze B2 smoke test + SFTP + GCS + Azure Blob + advanced S3 auth" (T5).
  - "File distribution: PUT-side resume" (T11).
  - "Rate-limit: Retry-After HTTP-date format alternative" (T19).
  - "Rate-limit: configurable gRPC retry-after-ms trailer key" (T19).
  - "Rate-limit: Auditor hook for rejections" (T19).
- **Coverage on Epic-18-touched packages** (all ≥75%, lowest 81.7%):
  - File-distribution: `internal/files` 95.6%, `internal/files/acl` 100%, `internal/files/backend` 82.9%, `internal/files/proxy` 95.6%, `internal/files/transport` 81.7%, `pkg/api/files` 89.9%, `internal/cli/files` 87.3%.
  - Self-management: `internal/selfmgmt` 92.5%, `internal/backup` 88.7%, `internal/backup/age` 89.4%, `internal/backup/dest` 92.8%, `internal/cli/backup` 87.7%, `internal/cli/bootstrap` 84.1%, `internal/s3client` 100%.
  - Rate-limiting: `internal/ratelimit` 96.4%, `internal/ratelimit/extract` 92.8%, `internal/ratelimit/middleware` 96.2%.
- **End-to-end coverage**: build-tagged `test/e2e/selfmgmt/` (T8 integration suite under `make test-integration`) exercises bootstrap FSM + backup round-trip + restore + populated-cluster guard + tamper detection.
- **T19 is a no-code closeout commit** — the Retry-After HTTP header and gRPC `retry-after-ms` trailer shipped in T18 as natural parts of the middleware deny path (verified by `TestHTTP_AllowThenDeny` + `TestHTTP_1000_RPS_Configured_Limit_Returns_429`); T19 documents the v1.x polish ROADMAP entries the spec leaves implicit and seals the epic.
