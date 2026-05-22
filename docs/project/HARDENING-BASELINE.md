# Hardening baseline

Epic 19 task 8 — the v1.0 hardening pass. This document captures
the audit outcomes against the seven hardening items called out in
`epics/19-test-harden-release.md` §Hardening (lines 63–72).

Adjacent docs:

- [SECURITY-GOVERNANCE.md](SECURITY-GOVERNANCE.md) — the security
  baseline pipeline (gitleaks / govulncheck / gosec / licenses)
  that gates every PR (task 7).
- [TEST-POLICY.md](TEST-POLICY.md) — race detector + goleak
  policy (tasks 5, 6).
- [PROFILING-BASELINE.md](PROFILING-BASELINE.md) — the measured
  CPU / allocation baseline against the perf SLO workload.

## Goroutine leaks

**Status**: clean for v1.0.

- Task 6 added `goleakhelper.VerifyTestMain` to every package
  containing a `//go:build integration` test file. 11 integration
  packages instrumented; all pass on a fresh `make test-integration`
  run.
- `tools/goleakgate` lints in CI; any new integration-tagged
  package that omits goleak fails the lint.
- 57 `go ...` spawn sites in production code (`internal/` +
  `pkg/`); each is bounded by a `ctx.Done()` channel and tracked
  via a `*sync.WaitGroup` owned by the spawning component.

**Deferred**: 1-hour soak run that asserts no leaks over the long
horizon. Tracked under the new ROADMAP v1.x entry
*"Soak-test infrastructure for fd / connection / goroutine leaks"*.

## Connection lifecycle audit

Every long-lived connection has a matching `Close()` / `Stop()`
call in the shutdown path, sequenced in `pkg/api/server.Server.Stop`.

| Connection | Open | Close | Ordering |
|------------|------|-------|----------|
| NATS embedded / external (`*natsmgr.Manager`) | `cmd/kscore-server/main.go::natsmgr.New` + `.Start` | `Server.stopNATS` → `Manager.Shutdown` | Stopped after bootstrap + dispatcher + connMgr so consumers drain first |
| etcd (`*cluster.EtcdClient`) | `internal/cluster.Manager.New` (embedded mode) | `connMgr.Stop` → `Manager.Close` | Stopped after the cluster's `ConnectionManager` |
| Postgres (`*sql.DB` via `state.NewStore`) | `cmd/kscore-server/main.go::state.NewStore` | `Server.stopStore` → `state.Store.Close` | Stopped after NATS + dispatcher (no late writes) |
| gRPC server (`*grpc.Server`) | `Server.initSteps9to13::grpc.NewServer` | `Server.stopGRPC` → `GracefulStop` with bounded timeout | Stopped first — refuses new requests, lets in-flight drain |
| gRPC streams (client + server) | per-RPC | per-RPC `defer Close()` / `stream.CloseSend()` | Caller-managed; no shared lifetime |
| HTTP server (`*http.Server`) | `Server.initSteps15to18` | `Server.stopHTTP` → `Shutdown(ctx)` | Stopped after gRPC so `/health/*` keeps serving until the very end |
| SQLite stores (rollback, webhook, verification, agent local) | per-runtime `New*Store(path)` | per-runtime `*Store.Close` | Runtime-scoped; closed in the runtime's `stop(ctx)` |

**Deferred**: 1-hour soak test that snapshots `lsof` baselines and
diffs after N hours — tracked under the new ROADMAP v1.x entry
*"Soak-test infrastructure for fd / connection / goroutine leaks"*.

## File descriptor leaks

**Deferred** to v1.x. The mechanism is identical to the
goroutine-leak soak: a long-running agent under load + an `lsof`
baseline diff. Not in v1.0 scope; the per-component lifecycle
audits above are the v1.0 deliverable.

## Production-warning paths

`internal/config.Config.ProductionWarnings()` returns a list of
human-readable warnings for dev-mode-only knobs detected in
`mode: production`. The set covered as of task 8:

| Knob | Warning |
|------|---------|
| `server.tls.enabled: false` | "TLS is disabled in production" |
| `storage.driver: sqlite` | "SQLite is not recommended for production (use postgres for HA)" |
| `cluster.etcd.mode: embedded` (cluster.enabled true) | "embedded etcd is fine for ≤3 members; use external etcd for 5+ member production clusters" |
| `server.cors.allowed_origins: ["*"]` | "CORS allows all origins (*) in production" |
| `gitops.webhook.sources.<provider>.method: none` | naming the unauthenticated sources |
| `security.hmacsecret` static value | naming the rotation graduation path |
| `secrets.backends[].file.master_key: inline:...` | naming the env: / file: alternative |
| `nats.bootstrap.enabled: true` | naming the identity-issued join-token graduation path |

`pkg/api/server.Server.ProductionWarnings` augments this list with
runtime-only signals (e.g., "auth disabled at runtime").

Tests in `internal/config/config_test.go::TestProductionWarnings_*`
cover each combination; a "fully safe" production config returns no
warnings.

## Error messages referencing docs URLs

**Deferred** to v1.x. The Hugo docs site is post-v1.0 per
`epics/19-test-harden-release.md` §Scope out. Adding
`https://keystone-core.io/docs/...` URLs to error strings before
the URLs exist would rot. Tracked under the new ROADMAP v1.x entry
*"Error-message docs URLs (post Hugo site)."*

## Performance profiling

See [PROFILING-BASELINE.md](PROFILING-BASELINE.md) for the measured
baseline + analysis.

**Status**: clean for v1.0 — no project-code hot spot above the
5% CPU or 50 MB allocation thresholds against the perf SLO
workload. `make profile` is the repeatable entrypoint.

## Logging audit

Three sub-pass results:

### PII

**Clean for v1.0.** No production log statement emits personal
data (`user.email`, `user.name`, etc.). The `internal/logging`
package masks regex-matched secrets at the slog handler layer per
PROJECT-DETAILS §5.2.

### Plaintext secrets

**Clean for v1.0** with one documented exception: the
`cmd/kscore-server` dev API key boot WARN logs the cleartext
exactly once per `pkg/api/apikeys/dev.go`'s contract. The warning
message itself instructs the operator to capture it. This is
intentional — operators have no other way to recover the cleartext.

No other production log statement emits cleartext credentials,
tokens, or keys. Helpers like `*Secret.MaskForLog()` and
`MaskedSecretValue` are used uniformly in the cli/secrets paths.

### Correlation IDs

**Clean for v1.0** at request scope. Every request handler obtains
its `ctx` via `internal/cli.RootCommand`'s setup which threads
`logging.WithCorrelationID(ctx)`; each scoped `slog.*Context` call
propagates it. 122 non-context `slog.Info/Warn/Error/Debug` calls
exist in deep helpers that don't have a `ctx` parameter in scope;
these are init / collector / shutdown paths where there's no
request to correlate.

**Deferred**: graduating those 122 sites to context-aware variants
is a v1.x cleanup tracked under the new ROADMAP entry
*"Logging: context-aware threading of deep helpers"*.

## Summary

| Hardening item | v1.0 status | Notes |
|----------------|-------------|-------|
| Goroutine leaks (`goleak` in tests) | done (task 6) | 11 packages instrumented |
| Connection leaks | done (audit table above) | soak run deferred |
| fd leaks (long-running test) | deferred to v1.x | soak harness is the mechanism |
| Default-config production-warn paths | done (3 new warnings + tests) | 8 dev-mode knobs covered total |
| Error messages reference docs URLs | deferred to v1.x | post-Hugo |
| Performance profiling pass | done | clean against SLO workload |
| Logging audit | done | 1 documented intentional exception; deep-helper context graduation deferred |
