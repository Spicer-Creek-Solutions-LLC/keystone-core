# Epic 00: Meta — Reconstruction Plan

## Purpose

This directory contains the implementation epics for rebuilding Keystone Core from scratch as an "advanced MVP" targeting **commercial-trial-ready, sysadmin-attractive** v1.0 — clusterable from day 1, Salt-Project-shaped UX, ~90% of daily sysadmin needs covered.

The companion documents are:
- **`/FEATURES.md`** — complete feature inventory tagged by version (v1.0 / v1.x / v2.0 / v3+).
- **`/PROJECT-DETAILS.md`** — implementation reconstruction guide (architecture, types, protocols, gotchas, versioning strategy).

These three artifacts together are intended to be sufficient for an LLM (or human team) to reconstruct the project on the same Go stack.

## Epic Structure

| # | Epic | Phase | Estimated weeks |
|---|---|---|---|
| 01 | Foundations (build, config, logging, version, error model) | A | 2 |
| 02 | Storage layer (SQLite + Postgres + migration) | A | 1 |
| 03 | API surface (protos, codegen, auth, RBAC) | B | 2 |
| 04 | Control plane core (server bootstrap, middleware, listeners, shutdown) | B | 2 |
| 05 | NATS messaging (embedded + external + bootstrap) | C | 2 |
| 06 | Agent runtime + bootstrap UX | D | 2 |
| 07 | Remote execution + targeting | E | 1.5 |
| 08 | State management engine + 40-module base stdlib | E | 4 |
| 09 | Identity & auth (embedded CA, API keys, mTLS, JWT, join tokens) | F | 2 |
| 10 | Secrets management (broker, file + Vault, leases, transit) | F | 1.5 |
| 11 | Event system (bus + persistence + filter + service) | G | 1.5 |
| 12 | Audit log + policy engine (audit-mode) | G | 2 |
| 13 | Clustering & HA (etcd, election, sharding, failover, fencing) | H | 4 |
| 14 | Plugin / module system (Starlark + Cosign + filesystem registry) | I | 3 |
| 15 | Blueprints + runbooks + saga (basic) | J | 2 |
| 16 | GitOps integration + outbound webhooks | K | 2 |
| 17 | Observability (logs/metrics/traces/health/Grafana) | L | 1.5 |
| 18 | Self-management (bootstrap-from-seed + backup/restore) + file distribution + rate limiting | M-N | 2 |
| 19 | Test, harden, release v1.0 | N | 2 |

**Total estimate**: ~38 epic-weeks; with a 4-engineer team and parallelism, ~10–12 calendar weeks for v1.0. Fewer engineers extend proportionally.

## Sequencing Rules

1. **Phase A (foundations)** must complete before anything else — every other domain depends on config/logging/storage/version.
2. **Phase B + C run in parallel** — API surface and NATS messaging are independent.
3. **Phase D (agent)** depends on B + C.
4. **Phase E (control plane services)** depends on B + C + storage. State management can ship with a few modules first; full 40-module stdlib lands incrementally.
5. **Phase F (identity + secrets)** can run parallel to E once the API surface is stable.
6. **Phase G (events/audit/policy)** can run parallel to E and F once API + storage exist.
7. **Phase H (clustering)** is the gate to v1.0 — it must work end-to-end before v1.0 RC. **Do not defer.**
8. **Phase I (plugin system)** depends on E (state engine integration) and on identity (capability auth).
9. **Phases J, K, L, M, N** are mostly parallelizable once H is complete.

## Definition of Done — v1.0 Release

- [ ] All 19 epics complete with their acceptance criteria.
- [ ] Performance SLOs verified in CI: cluster forms <10s, leader elected <3s, failover detection <5s, agent reassign <10s, minority blocks writes <1s, recovery <15s.
- [ ] HA resilience tests pass on every PR (NATS-failure, etcd-failure, network-partition, split-brain).
- [ ] Coverage targets met: critical packages >70%, CLI packages >40%.
- [ ] Security scans clean: gitleaks (no secrets), govulncheck (no known CVEs), gosec (no high/critical).
- [ ] Single-topology E2E green (all-in-one docker-compose with kscore-server + agent + Postgres + NATS).
- [ ] Blueprint catalog (6) installs and applies successfully end-to-end.
- [ ] Plugin module example (Starlark) builds, signs, publishes, installs, executes.
- [ ] Backup → restore round-trip verified with no data loss.
- [ ] Documentation: README, CLI reference, API reference, configuration reference, security policy, release playbook.
- [ ] Goreleaser snapshot produces multi-arch tarballs (linux amd64/arm64, darwin amd64/arm64, windows amd64).

## Anti-Goals for v1.0

These are explicitly **not** in v1.0. Each has a target version in PROJECT-DETAILS.md §6.2:

- WASM module runtime (v1.1)
- Windows agent (v1.1)
- macOS agent (v1.2)
- TUI monitor (v1.2)
- K8s operator (v1.3)
- SPIRE identity / cloud workload identity (v1.3 / v2.0)
- Telemetry gateway (v1.4)
- **Policy enforcement (Enforce + Warn modes)** (v1.8) — v1.0 ships audit-mode-only
- Rotation orchestrator with strategies (v1.4)
- Air-gap baseline (v1.7)
- NATS supercluster, leaf nodes, WebSocket (v2.0)
- Federation (v2.0)
- DNS provider mgmt, advanced networking, proxy agents, MCP server (v2.0)
- Web UI, blueprint marketplace (v3+)

If during reconstruction a candidate v1.0 epic creeps toward one of these, **stop and review** — the v1.0 timebox is tight; carrying over deferred features costs the trial-readiness ship date.

## Working Conventions

- Each epic is self-contained: goals, scope, non-goals, design summary, tasks, acceptance criteria, risks.
- Where an epic references implementation detail in PROJECT-DETAILS.md, it cites the section (`PROJECT-DETAILS §4.X`) rather than duplicating.
- Tasks are sized for individual PR review — typically 1-3 days of work each.
- Each task has tests (unit + integration where applicable).
- DCO sign-off and AI-assistance disclosure required on every commit (carry forward from existing repo).
- Commit and push incrementally; do not batch a whole epic into one PR.
