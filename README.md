# Keystone Core

[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Status](https://img.shields.io/badge/Status-v0.1_reconstruction-orange)](docs/project/VERSIONING.md)
[![AI Contributions Welcome](https://img.shields.io/badge/AI_Contributions-Welcome-brightgreen)](docs/project/AI-CONTRIBUTIONS.md)

**GitOps deploys it. We keep it running.**

Keystone Core is the runtime operations control plane between deployment tooling (GitOps/IaC) and day-2 operations. Where Argo CD, Flux, and Terraform answer *what should be deployed*, Keystone Core answers *what is happening right now, is it drifting, and what should we do about it?*

Inspired by Salt Project's UX, built on a modern Go stack with cloud-native primitives — clusterable from day 1, security-real from day 1.

## Public hosting

- **Primary**: [`codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core`](https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core)
- **Mirror (code-only)**: [`github.com/Spicer-Creek-Solutions-LLC/keystone-core`](https://github.com/Spicer-Creek-Solutions-LLC/keystone-core)

File issues, discussions, and contributions on Codeberg. The GitHub mirror exists for discoverability and is not where development happens.

Project sponsor: **Spicer Creek Solutions LLC** ([`OWNERSHIP.md`](OWNERSHIP.md)).

## Project Status

> **Pre-1.0. Reconstruction in progress. Not installable yet.**

This repository was reset to a clean reconstruction baseline on 2026-05-05. The prior implementation — substantial but unshippable as a coherent first release — is preserved on the `archive/v0` branch (tag `archive/v0-final`).

The reset is deliberate: rather than carry forward technical debt, the codebase is being rebuilt against a tight feature scope, a clear MVP definition, and an explicit anti-scope. The full rationale and per-epic plan live in [`epics/00-meta-reconstruction-plan.md`](epics/00-meta-reconstruction-plan.md); the versioning scheme — three milestone tiers `v0.1` → `v0.5` (external-tester ready, all Linux) → `v1.0` (all 19 epics + SemVer stability) — is in [`docs/project/VERSIONING.md`](docs/project/VERSIONING.md).

If you were following the previous codebase: it isn't gone, it just isn't `main` anymore. `git fetch && git checkout archive/v0` to read it.

### Current state (epics 01–07 complete)

What's in `main` today:

- **Foundations** (epic 01): `pkg/{version,semver,wait,dbutil}`, `internal/{config,logging,cli}`, `pkg/api/{apierror,v1}` (proto codegen).
- **Storage layer** (epic 02): `internal/state` with SQLite + PostgreSQL backends. `Store` interface composes per-domain sub-interfaces (Agent, Command, BatchJob, APIKey, Health). Migrator runs on `Open`.
- **API surface** (epic 03): proto schemas + auth chain (`pkg/api/auth`: APIKey / JWT / mTLS authenticators, RBAC authorizer, sliding-window rate limiter). Per-domain REST handler stubs return 501 until their owning epic ships. OpenAPI 3.0 spec lints in CI.
- **Control plane** (epic 04): `kscore-server` is a real daemon. `internal/controlplane`: `ConnectionManager`, `CommandDispatcher`, `BatchDispatcher`. `pkg/api/server`: 21-step deterministic init, dual-stack listeners, auth middleware chain, `/health/{live,ready,status}` + `/api/status`. Dev mode auto-generates an admin API key once at boot.
- **NATS messaging** (epic 05): `internal/nats.Manager` (external client + embedded server modes), `SubjectBuilder` with `kscore.{cluster}.…` prefix enforced both sides, `Envelope` wire format with length-prefixed dedup, per-endpoint circuit breakers, JetStream stream provisioning, server-side bootstrap registration handler with PSK validator + API-key issuer.
- **Agent runtime** (epic 06): `kscore-agent` is real. Subscribes to its command subject; runs `Executor` (os/exec wrap with SIGTERM-grace-then-SIGKILL, hard-cap output truncation, optional uid switch), `MetadataCollector` (gopsutil-backed; distro / kernel / NIC / virt / CPU / memory / disk), `SecurityEnforcer` (HMAC-SHA-256 + principal/command allowlists + env filter). Drains in-flight commands on SIGTERM. systemd unit + non-interactive bootstrap flags.
- **Remote execution & targeting** (epic 07): operator-facing dispatch end-to-end.
  - `internal/targeting`: shorthand expression compiler (`expr-lang/expr` + `gobwas/glob`) → `Matcher.Match(AgentRecord)` against flattened metadata. AND-of-labels-plus-hostname-glob today; `os:` / `OR` / `NOT` server-side compile is on the v0.x roadmap.
  - `internal/execution`: `Executor` interface + `ManagedExecution` (PENDING / RUNNING / COMPLETED / FAILED / TIMEOUT / CANCELLED / RETRYING with `Callbacks` + `RetryPolicy`), `Pipeline` (sequential stages with stdout-piping), `Shell` (bash / sh / powershell / cmd selectors), `CommandPolicy` (`Validate` / `ValidateNoShell` modes — block shell metachars in direct-exec).
  - `internal/controlplane`: `BatchDispatcher.ExecuteBatch` (semaphore concurrency, 500ms progress ticker, async orchestration detached from request ctx), `ResponseRouter` + `NATSBatchExecutor` (per-CorrelationID waiters fed by an `agent.*.response` subscriber), `GRPCServer` implementing `ExecuteCommand` / `BatchExecuteCommand` (streaming) + `GetBatchJob` / `ListBatchJobs` / `CancelBatchJob` / `ListBatchAgentResults` / `GetBatchAgentResult` + dry-run.
  - `kscorectl exec`: `run` / `async` / `script` / `status` / `list` / `cancel` / `output` subcommands with `--dry-run`; table / json / yaml formatters; `--raw` mode for pipe-friendly single-agent output.
  - In-process integration test exercises a 5-agent fleet with a target matching 3, with sub-second single-agent and sub-2s 5-agent latency.
- **Three binaries** that boot cleanly under SIGTERM/SIGINT: `kscore-server`, `kscore-agent`, `kscorectl`.
- **Make-driven workflow**: `make build`, `test`, `lint`, `proto`, `release-snapshot`. CI invokes Make targets exclusively.
- **Cross-compile matrix**: linux/{amd64,arm64}, darwin/{amd64,arm64}, windows/{amd64,arm64} — pure Go, no CGO.
- **CI**: Forgejo Actions (`.github/workflows/`) and Codeberg Woodpecker (`.woodpecker/`) — same Make targets, two runners.
- **Snapshot release**: `make release-snapshot` produces six tarballs/zips + a SHA-256 checksums file in `dist/`.

State management (epic 08) is in progress; identity/auth (epic 09) and the rest follow. Track progress in [`epics/00-meta-reconstruction-plan.md`](epics/00-meta-reconstruction-plan.md). The ranked v0.x backlog (entries gated on v0.5, v1.0, or further out) lives in [`docs/project/ROADMAP.md`](docs/project/ROADMAP.md).

## What v1.0 commits to

The full success bar — all 19 epics complete, contracts frozen, SemVer stability begins — is in [`docs/project/VERSIONING.md`](docs/project/VERSIONING.md). The headline commitment: a release in which a single binary, in a small lab environment, demonstrates that one system can:

- Manage heterogeneous Linux fleets (VMs, bare metal, mixed distros) via a single agent over NATS.
- Run remote commands with rich targeting, batch concurrency, and streaming output.
- Apply declarative state through 35 universal Linux modules with drift detection and remediation.
- Run highly available out of the box — 3-node cluster, embedded etcd, leader election under 3 seconds, failover under 10.
- Issue real identity from day 1 — embedded SPIFFE-shaped CA, mTLS, join tokens, API keys, JWT.
- Broker secrets via encrypted-file or HashiCorp Vault backends.
- Integrate with GitOps tooling via Argo CD / Flux / GitHub / GitLab webhooks for verification and rollback.
- Be safely extensible through Cosign-signed Starlark modules with capability-based sandboxing.
- Compose higher-level operations via blueprints, runbooks, and a saga coordinator.
- Audit and observe every sensitive action — Prometheus metrics, OpenTelemetry traces, Grafana dashboards.
- Self-manage — bootstrap from seed, back up, restore, rate-limit, distribute files.

Full success criteria: [`docs/project/PROBLEM-STATEMENT.md`](docs/project/PROBLEM-STATEMENT.md).

## What v0.x is *not*

Keystone Core is on the `v0.x` line. SemVer permits breaking changes between v0.x releases; expect them. CLI shapes, state-file params, gRPC contracts, DB migrations — all fair game until v1.0.

To keep the v1.0 timebox honest, the following are explicitly post-v1.0:

- WASM module runtime, Windows agent, macOS agent
- Kubernetes operator embedding
- Full SPIRE integration
- Policy *enforcement* (the v0.x → v1.0 line is audit-mode only)
- Federation, NATS supercluster / leaf nodes
- Web UI, blueprint marketplace, telemetry gateway

If you need any of these now, Keystone Core is not yet your tool. **If you'd like to test the v0.5 milestone** (external-tester-ready cross-Linux release) when it lands, please follow along.

## Reading order

If you're new and want to understand the project:

1. [`docs/project/PROBLEM-STATEMENT.md`](docs/project/PROBLEM-STATEMENT.md) — why this exists.
2. [`docs/project/DESIGN.md`](docs/project/DESIGN.md) — high-level architecture.
3. [`FEATURES.md`](FEATURES.md) — feature inventory with version tags.
4. [`PROJECT-DETAILS.md`](PROJECT-DETAILS.md) — implementation reconstruction guide.
5. [`epics/`](epics/) — the 19 implementation epics, in dependency order.

## Reconstruction progress

Tracked in [`epics/00-meta-reconstruction-plan.md`](epics/00-meta-reconstruction-plan.md), which lists all 19 epics across phases A–N, their dependencies, and current status. Estimated timebox: ~38 epic-weeks; with parallelism, ~10–12 calendar weeks.

## Contributing

Contributions are welcome. During the reboot phase the epics are tightly sequenced, so please open an issue or short RFC to coordinate before opening a code PR — out-of-order work is hard to merge cleanly. Issues, doc improvements, and feedback on the epic structure are always welcome without coordination.

- [`CONTRIBUTING.md`](CONTRIBUTING.md) — how to contribute, DCO sign-off, AI disclosure
- [`AGENTS.md`](AGENTS.md) — operational guidance for AI coding agents
- [`docs/project/AI-CONTRIBUTIONS.md`](docs/project/AI-CONTRIBUTIONS.md) — AI contribution policy
- [`docs/project/RFC.md`](docs/project/RFC.md) — propose larger changes via RFC
- [`SECURITY.md`](SECURITY.md) — security policy and reporting

## Transparency: AI use in this project

Keystone Core was originally bootstrapped with substantial AI assistance, and this reconstruction continues that practice openly. The objective is speed-to-foundation: get to a reviewable baseline quickly, then spend sustained human effort on correctness, scaling, and reliability.

Quality is enforced by process, not by origin. AI-assisted contributions are expected to meet the same standards as human-authored ones: readable design, reproducible builds, tests proportional to risk, security-minded changes, reviewable diffs. Maintainers are responsible for what gets merged. AI involvement must be disclosed in commits per [`docs/project/AI-CONTRIBUTIONS.md`](docs/project/AI-CONTRIBUTIONS.md).

## License

Apache License 2.0 — see [`LICENSE`](LICENSE) and [`NOTICE`](NOTICE).
