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
2. **All v1.0 E2E scenarios** wired into CI. _(2a landed — scenarios 1–3 over the single-topology harness from task 1: agent registration (PSK bootstrap + heartbeats reaching `connected`), single command exec via `ExecuteCommand`, and `ApplyState` round-trip. CI gains a non-blocking `e2e` job (`continue-on-error: true`) that runs `make e2e-test`. Required v1.0 wiring delivered as part of 2a: agent-side bootstrap-register publish on `Start`, server-side `AgentHeartbeat` subscriber routing to `ConnectionManager.Heartbeat`, bootstrap handler `OnAgentRegistered` callback feeding the conn-mgr cache, and `ListAgents`/`GetAgent` gRPC implementations. **Pending**: 2b — scenarios 4–6 (blueprint apply, module install/exec, secrets); 2c — scenarios 7–9 (audit query, outbound webhook, GitOps webhook+rollback) + flipping the CI job to required.)_
3. **Performance SLO verification** scripts in `test/e2e/perf/`.
4. **Coverage gating** in CI.
5. **Race detector** on every test run.
6. **`goleak`** integrated into integration test setup/teardown.
7. **Security baseline pipeline** — gitleaks, govulncheck, gosec; license check.
8. **Hardening pass** — review every domain for leaks; pprof profiling pass.
9. **Documentation pass** — README, CLI ref, API ref, config ref, getting-started guides.
10. **`RELEASE-PLAYBOOK.md` v1.0 version** — single-signer process; clear path to multi-party in v1.2.
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
