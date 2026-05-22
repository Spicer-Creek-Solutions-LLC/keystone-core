# Epic 19: Test, Harden, & Release v1.0

**Phase**: N • **Estimate**: 2 weeks • **Depends on**: all preceding epics • **Blocks**: v1.0 release

## Goal

Take v1.0 from "all features done individually" to "production-ready release artifact." HA resilience suite green, performance SLOs verified, security baseline clean, single-topology E2E green, blueprint catalog round-trips, plugin system round-trips, backup/restore round-trips, documentation complete, goreleaser produces signed multi-arch tarballs.

## Scope (in)

### Test infrastructure

- **Single-topology E2E** (`test/e2e/`) — docker-compose with 1× kscore-server + 2× kscore-agent + Postgres + NATS. Tests:
  - Agent registration + heartbeat + stale eviction.
  - Single + batch command exec.
  - State apply + drift detection.
  - Blueprint apply (`demo`).
  - Module install + execute.
  - Secret read/write/lease/transit.
  - Audit log query + compliance report.
  - Outbound webhook delivery.
  - GitOps webhook + rollback.
- **HA E2E** (already in Epic 13) — verified passing on every PR:
  - NATSFailure, EtcdFailure, NetworkPartition, SplitBrain.
- **Performance SLO verification** in CI:
  - Cluster forms <10s, first leader <3s, failover detection <5s, agent reassign <10s, minority blocks writes <1s, recovery <15s.
  - Single-agent command latency <100ms (local NATS).
  - 1000-event emission throughput >10k events/s.
  - 100-batch command exec across 10 agents <2s.
- **Security baseline scans**:
  - gitleaks: no secrets in git history.
  - govulncheck: no known CVEs in dependencies.
  - gosec: no high/critical findings.
  - License check: all deps Apache-2.0/MIT/BSD-compatible.
- **Coverage gates**:
  - Critical packages >70%.
  - CLI packages >40%.
- **Race detector** on every CI run (`go test -race ./...`).
- **goleak** verification in integration tests.

### Documentation

- `README.md` updated with v1.0 quick start, feature list, links.
- `docs/content/en/docs/reference/cli.md` — every binary + every subcommand documented.
- `docs/content/en/docs/reference/api.md` — gRPC services + REST endpoints.
- `docs/content/en/docs/reference/configuration.md` — every config key with default + description.
- `docs/content/en/docs/getting-started/` — install, first cluster, first command, first state apply, first blueprint, first module.
- `docs/content/en/docs/operations/` — backup/restore, upgrade (placeholder for v1.5), monitoring.
- `RELEASE-PLAYBOOK.md` — v1.0 release process (single-signer for v1.0; multi-party for v1.2).
- `SECURITY.md` — vulnerability reporting + supply-chain verification.
- `COMPATIBILITY.md` — versioning policy.
- `CHANGELOG.md` — v1.0.0 entry.

### Release artifacts

- `make release-snapshot` produces:
  - linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64 tarballs.
  - SHA-256 checksum file.
  - SBOM (syft) — v1.1 ideal but v1.0 baseline is gitleaks + govulncheck + gosec.
- Goreleaser config covers all v1.0 binaries.
- Single-signer release (v1.0); multi-party signing ceremony deferred to v1.2.

### Hardening

- Final pass on every domain for:
  - Goroutine leaks (`goleak` in tests).
  - Connection leaks (NATS, etcd, Postgres, gRPC streams).
  - File-descriptor leaks (long-running agent test).
  - Default config production-warn paths emit clear warnings.
  - Error messages reference docs URLs where appropriate.
- Performance profiling pass with pprof; remediate hot spots > 5% CPU or > 50 MB allocation.
- Logging audit: no PII leaks, no plaintext secrets, correlation IDs everywhere.

## Scope (out / non-goals)

- Hugo docs site build pipeline — v1.1 (v1.0 ships markdown only).
- Full security scanning suite (semgrep, trivy, syft, grype, hadolint) — v1.1.
- Multi-party signing ceremony — v1.2.
- DNF / APT / MSI repositories — v1.2 (v1.0 ships tarballs only).
- VM-based bootstrap test harness — v1.2.
- Benchmark suite for regression detection — v1.2.

## Tasks

1. **Single-topology E2E** docker-compose + test harness. _(landed — `test/e2e/single/`: docker-compose with 1× kscore-server + 2× kscore-agent + Postgres + NATS; multi-stage `Dockerfile.kscore` with `BIN` build-arg; `e2e-{build,up,down,logs,test}` Make targets; `harness_test.go` (`//go:build e2e`) asserts server `/health/live`+`/health/ready`, NATS `/healthz`, and Postgres schema applied. Feature scenarios are task 2.)_
2. **All v1.0 E2E scenarios** wired into CI. _(landed — 2a delivered scenarios 1–3 (registration via PSK bootstrap + heartbeats reaching `connected`, command exec, state apply) plus the foundational v1.0 wiring: agent-side bootstrap-register publish on `Start`, server-side `AgentHeartbeat` subscriber routing to `ConnectionManager.Heartbeat`, bootstrap-handler `OnAgentRegistered` callback feeding the conn-mgr cache, and `ListAgents`/`GetAgent` gRPC implementations. 2b delivered scenarios 4–6: `BlueprintService.ApplyBlueprint` against a distroless-compatible test catalog (`test/e2e/single/blueprints/e2e-noop`), multi-stdlib `ApplyState` proving the loader→runner chain (Option A — Option B / registry-based Starlark install+execute deferred to v1.x pending gate-v1.0 "Module system boot wiring"), and KV-only `SecretsService` round-trip; plus boot wiring for `BlueprintService` + a server-local Applier. 2c delivered scenarios 7–9 + the CI gate flip: `PolicyService.GetAuditLog` + `GetComplianceReport`, outbound webhook end-to-end (`httptest` receiver bound on `0.0.0.0`, host-gateway reachability, REST CRUD + test ping), GitOps inbound webhook ingest (HMAC-signed GitHub push payload to `:8081/webhooks`), and GitOps rollback FSM via REST (Pending → Rejected); plus boot wiring for the outbound webhook subsystem (SQLite store + HTTP dispatcher + event-bus subscription), GitOps inbound webhook receiver (its own listener), and GitOps rollback engine (SQLite store + Git-revert executor — ArgoCD + K8s rollout-undo stay gate-v1.0 deferrals). CI `e2e` job is now required (no `continue-on-error`).)_
3. **Performance SLO verification** scripts in `test/e2e/perf/`. _(landed — `test/e2e/perf/` `//go:build slo`: `TestSLO_CommandLatency_LocalNATS` (median over 100 in-process dispatches via the production `NATSBatchExecutor`; bound 100 ms; measured ~400 µs on dev), `TestSLO_EventThroughput` (1000 events through `JetStreamPublisher`; bound 10k/s; measured ~75k/s), `TestSLO_BatchExec_10Agents` (fan-out across 10 in-process agents; bound 2 s; measured ~12 ms). `make slo` extended to glob both `./test/e2e/{ha,perf}/...` — single CI gate; no new workflow job. In-process embedded-NATS harness mirrors the established `test/e2e/ha/` pattern; docker-compose deliberately avoided because bridge-NAT pollutes the 100 ms command-latency budget. Tail-latency (p95/p99) and load-curve sweeps stay deferred to a v1.x benchmark suite.)_
4. **Coverage gating** in CI. _(landed — `tools/covgate/` Go tool parses `coverage.out` and asserts per-package statement coverage: critical packages ≥70%, CLI packages ≥40% (PROJECT-DETAILS §5.3). `make coverage-gate` invokes it; CI's `test` job runs the gate immediately after `make test-coverage`. Classification rules + an explicit allowList live in `tools/covgate/config.go` with per-entry GRADUATE-BY notes. Initial allowList carries 4 sub-threshold cohorts (`internal/state`, `pkg/api/cluster`, `pkg/api/secrets`, `cmd/kscore-server`) plus 14 zero-coverage `cmd/*` thin-wrapper binaries — each entry rises off the list when its measured coverage crosses the threshold (the gate fails if an allowList number falls behind, forcing graduation). Policy + how-to lives at `docs/project/COVERAGE-GATES.md`. Pre-existing `tools/trackerctl` test failure (release-order.yaml backtick mismatch) fixed inline so the gate gets a clean profile.)_
5. **Race detector** on every test run. _(landed — verified the existing race posture (every `go test` invocation under `make test`/`test-verbose`/`test-coverage`/`test-integration` already runs `-race`); added `-race` to the two outliers (`make e2e-test`'s Go-side test code, `scripts/smoke-test.sh`'s sqlite-pragma probe); added `tools/racegate/` lint that scans the Makefile + smoke script and fails CI if any `go test` invocation drops `-race` without an explicit allowList entry. The only allowlisted target is `make slo` (race inflates wall-clock 2-10× and would invalidate the asserted SLO bounds). `make race-policy` runs in the CI `lint` job. Policy + exceptions + how-to-add-an-exception documented in `docs/project/TEST-POLICY.md`.)_
6. **`goleak`** integrated into integration test setup/teardown. _(landed — every package containing a `//go:build integration` test file now ships a TestMain wrapping `goleakhelper.VerifyTestMain` (`test/goleak/goleak.go`). 11 integration packages instrumented (cmd/kscore-server, cmd/kscore-migrate, internal/events, internal/nats, internal/secrets, internal/state, test/e2e/{ha,blueprint,module,selfmgmt,webhook}); each passes the goleak check on a fresh `-count=1 -p=1 make test-integration` run, with the shared base ignore set covering known third-party long-lived goroutines (modernc.org/sqlite driver, nats.go conn loops). No real leaks surfaced — the codebase was already careful about goroutine lifecycle. `tools/goleakgate/` lint walks the tree and fails CI if any future integration-tagged package lacks a TestMain wrapping the helper; `make goleak-policy` runs in CI's `lint` job. Policy + ignore-management + how-to-allowlist documented in `docs/project/TEST-POLICY.md`.)_
7. **Security baseline pipeline** — gitleaks, govulncheck, gosec; license check. _(landed — four scans gated by CI's `security` job: `make security-secrets` (gitleaks; full-history scan), `make security-vulns` (govulncheck — clean after `go 1.26.3` toolchain bump + `golang.org/x/crypto` v0.50.0→v0.52.0), `make security-sast` (gosec HIGH-only, G115 globally excluded — 16 non-G115 HIGH findings handled inline with `//#nosec` rationale annotations covering G404 backoff jitter, G402 caller-supplied TLS MinVersion, G101 SQL-column-list false positives, G703 author-CLI paths, G118 intentional ctx detachment, G122 author-CLI walks), `make security-licenses` (go-licenses strict — forbidden/restricted/unknown all fail, one documented `--ignore` for `modernc.org/mathutil` whose BSD-3-Clause LICENSE lacks an SPDX header). Policy + annotation convention + license-ignore convention + v1.x graduation path documented in `docs/project/SECURITY-GOVERNANCE.md`'s new "Security Baseline Pipeline" section; v1.x expansion (re-enable G115 with per-site audit, semgrep, trivy/grype, syft SBOM, hadolint) tracked under the new ROADMAP entry "Security baseline expansion.")_
8. **Hardening pass** — review every domain for leaks; pprof profiling pass. _(landed — audit outcomes in `docs/project/HARDENING-BASELINE.md`. Goroutine leaks: clean (task 6 mechanism + 57 prod `go` sites all bounded by `ctx.Done()` + `WaitGroup`). Connection lifecycle: audit table covers NATS, etcd, Postgres, gRPC, HTTP, SQLite stores — every long-lived connection has a `Stop()`/`Close()` in `Server.Stop` with documented ordering. Production-warn paths: three new dev-mode-only warnings added (`security.hmacsecret` static, `secrets.backends[].file.master_key: inline:`, `nats.bootstrap.enabled` with PSKs) with per-warning tests; the 8 total now covered. Performance profiling: new `make profile` against the perf SLO workload — baseline at `docs/project/PROFILING-BASELINE.md`, no project-code hot spot above 5% CPU or 50 MB alloc, top consumers are inherent NATS broker I/O + agent child-process wait. Logging audit: clean for v1.0 (one documented intentional exception — the dev-key boot WARN per `pkg/api/apikeys/dev.go` contract; no PII; correlation IDs present at request scope). Deferred to v1.x ROADMAP: fd-leak soak harness, sustained-load profiling, error-message docs URLs (post Hugo), context-aware threading of the 122 deep-helper non-context log sites.)_
9. **Documentation pass** — README, CLI ref, API ref, config ref, getting-started guides. _(landed — README rewritten for v1.0 (topology ASCII diagram, quickstart, doc index, what-v1.0-commits-to, what-v0.x-isn't, build/test surface, contributing). Three auto-generated reference docs land at `docs/project/{CLI,CONFIGURATION,API}-REFERENCE.md` from new `tools/gendocs/{cli,config,api}/` generators: CLI walks `cmd/kscore-*` + invokes `--help` recursively (567 lines), config parses `internal/config/*.go` AST + emits per-section tables (655 lines), API extracts services + RPCs from `.proto` + paths + methods from openapi-spec.yaml (228 lines). `docs/project/GETTING-STARTED.md` is the 30-minute fresh-VM walkthrough (epic acceptance line 116). `SECURITY.md` polished for v1.0 — supported-versions table, identity/HMAC/credential posture, four-scan baseline reference. New `make docs-sync` regenerates the three references; `make docs-sync-check` (in CI's `lint` job) fails if a source change drifts the checked-in output. Deferred to v1.x ROADMAP: Hugo docs site, expanded per-domain getting-started guides, operations-runbooks accuracy sweep.)_
10. **`RELEASE-PLAYBOOK.md` v1.0 version** — single-signer process; clear path to multi-party in v1.2. _(landed — new "Release lines at a glance" section opens the playbook with the v1.0/v1.1 = single-signer vs v1.2+ = multi-party matrix + one-paragraph simplified-ceremony summary + why-single-signer rationale (Multi-Role-single-Person posture preserves the v1.2 graduation path). Each of the 10 phases + the §1 quorum rules, §2 key ceremony, §14 patch releases, §15 key rotation now carries an inline `**v1.0 simplification**` callout where it differs from the multi-party form — including the new single-person dual-machine reproducible-build check in phase 4 that substitutes for the multi-builder cross-check, and the third-machine post-release verification re-run in phase 10. New "v1.2 graduation checklist" section enumerates 7 must-land items (onboard ≥2 signers, v1.2 key ceremony, document offline-machine setup, RC end-to-end, SECURITY.md update, playbook reissue, 30-day transition communication) — no partial graduation. Appendix A split into v1.0/v1.1 single-signer flow + v1.2+ multi-party flow + a "where to look" pointer at the release record's Participants block. `SECURITY.md` "Supply Chain Security & Release Verification" updated: short-form single-signer verification inline; links to the new playbook anchors.)_
11. **`CHANGELOG.md` v1.0.0** entry — full feature list with epic mapping.
12. **`.goreleaser.yaml` finalization** — all v1.0 binaries; multi-arch.
13. **Release dry-run** — `make release-dry-run` produces artifacts; manual smoke-test of each.
14. **v1.0.0-rc1** — internal test release; gather feedback.
15. **v1.0.0-rc2** — bug fixes from rc1.
16. **v1.0.0** — final release.

## Acceptance criteria

- [ ] All v1.0 E2E scenarios pass in CI.
- [ ] All HA resilience tests (Epic 13) pass.
- [ ] All performance SLOs met in CI.
- [ ] Security baseline clean: 0 secrets, 0 known CVEs, 0 gosec high/critical.
- [ ] Coverage gates pass: critical >70%, CLI >40%.
- [ ] No goroutine / connection / fd leaks in 1-hour soak test.
- [ ] All v1.0 binaries built for 5 platforms via `make release-snapshot`.
- [ ] SHA-256 checksum file generated.
- [ ] `kscorectl --help` lists all subcommands; each has help text.
- [ ] CLI reference docs cover every command.
- [ ] API reference docs cover every gRPC RPC + REST endpoint.
- [ ] Configuration reference covers every key with default + description.
- [ ] Quick-start guide can be followed end-to-end on a fresh Ubuntu VM in <30 minutes.
- [ ] Backup → fresh cluster → restore round-trip identical state (verified by hash).
- [ ] `RELEASE-PLAYBOOK.md`, `CHANGELOG.md` v1.0.0 entry, `SECURITY.md`, `COMPATIBILITY.md` complete and reviewed.
- [ ] v1.0.0 tag pushed; goreleaser produces release artifacts.

## Risks

- **Coverage chase** can become time sink — pragmatic 70/40 thresholds; don't gold-plate untested branches.
- **Performance SLO regression** late in cycle — profile early and often; flag regressions immediately.
- **Documentation drift** — pair every code PR with doc updates per AGENTS.md.
- **Last-mile bug fixes** stretch timeline — RC cycle absorbs but cap at 2 RCs; if rc3 needed, slip the date and document why.
- **CI flakiness** at the harness level (containers, ports) — invest in deterministic teardown; goleak helps diagnose.

## References

- All preceding epics; PROJECT-DETAILS §5 (cross-cutting), §6 (versioning).
