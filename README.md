# Keystone Core

[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Status](https://img.shields.io/badge/Status-v0.x_pre--release-orange)](docs/project/VERSIONING.md)
[![AI Contributions Welcome](https://img.shields.io/badge/AI_Contributions-Welcome-brightgreen)](docs/project/AI-CONTRIBUTIONS.md)

**GitOps deploys it. We keep it running.**

Keystone Core is the runtime operations control plane between deployment tooling (GitOps/IaC) and day-2 operations. Where Argo CD, Flux, and Terraform answer *what should be deployed*, Keystone Core answers *what is happening right now, is it drifting, and what should we do about it?*

Inspired by Salt Project's UX, built on a modern Go stack with cloud-native primitives — clusterable from day 1, security-real from day 1.

## Topology

```
                   ┌────────────────────┐
   Operators ─────▶│   kscore-server    │◀───── Postgres (state, audit, events)
   (kscorectl /    │  gRPC :9090        │
    REST /         │  REST :8080        │
    kscore-*)      │  Webhooks :8081    │
                   └─────────┬──────────┘
                             │ NATS (commands, responses, events)
            ┌────────────────┼────────────────┐
            ▼                ▼                ▼
      ┌──────────┐     ┌──────────┐     ┌──────────┐
      │  kscore- │     │  kscore- │     │  kscore- │
      │  agent   │     │  agent   │     │  agent   │
      └──────────┘     └──────────┘     └──────────┘
       (linux host)     (linux host)     (linux host)
```

One control plane, N agents, NATS as the transport, Postgres for durable state. Bootstrap PSK or join-token + SVID; commands are HMAC-signed; events fan out via JetStream; audit + policy go through the same bus.

## Quickstart

The fast path is `make e2e-up` — brings up server + 2 agents + Postgres + NATS via docker-compose:

```bash
git clone https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core.git
cd keystone-core
make e2e-up
```

Topology is reachable at `http://127.0.0.1:8080` (REST + health) and `127.0.0.1:9090` (gRPC). Both agents bootstrap automatically and reach `connected` within a few seconds.

Tear down: `make e2e-down`.

For a complete 30-minute walkthrough from a fresh Ubuntu VM (install Go + Docker, generate a PSK, run a command, apply state), see [`docs/project/GETTING-STARTED.md`](docs/project/GETTING-STARTED.md).

## Documentation

### Reference

- [`docs/project/CLI-REFERENCE.md`](docs/project/CLI-REFERENCE.md) — every `kscore-*` binary + every subcommand (auto-generated via `make docs-sync`).
- [`docs/project/CONFIGURATION-REFERENCE.md`](docs/project/CONFIGURATION-REFERENCE.md) — every config key with type + description (auto-generated).
- [`docs/project/API-REFERENCE.md`](docs/project/API-REFERENCE.md) — every gRPC RPC + REST endpoint (auto-generated, links to canonical proto/openapi sources).
- [`docs/project/GETTING-STARTED.md`](docs/project/GETTING-STARTED.md) — the 30-minute fresh-VM walkthrough.

### Project

- [`FEATURES.md`](FEATURES.md) — feature inventory with version tags.
- [`PROJECT-DETAILS.md`](PROJECT-DETAILS.md) — implementation reconstruction guide.
- [`docs/project/DESIGN.md`](docs/project/DESIGN.md) — high-level architecture.
- [`docs/project/PROBLEM-STATEMENT.md`](docs/project/PROBLEM-STATEMENT.md) — why this exists.
- [`docs/project/VERSIONING.md`](docs/project/VERSIONING.md) — the v0.x → v0.5 → v1.0 release ladder.
- [`docs/project/ROADMAP.md`](docs/project/ROADMAP.md) — ranked backlog (gate-v0.5 / gate-v1.0 / v1.x / v2.x+).
- [`epics/`](epics/) — the 19 reconstruction epics, in dependency order.

### Releases

- [`CHANGELOG.md`](CHANGELOG.md) — release notes, breaking changes, feature-by-epic mapping. Current line: `v0.x`.
- [`RELEASE-PLAYBOOK.md`](RELEASE-PLAYBOOK.md) — end-to-end build / sign / publish ceremony; single-signer for v0.x / v1.0 / v1.1; multi-party from v1.2.
- [`docs/project/VERSIONING.md`](docs/project/VERSIONING.md) — the v0.1 → v0.5 → v0.9 → v1.0 ladder + gate checklists.

### Governance + security

- [`docs/project/GOVERNANCE.md`](docs/project/GOVERNANCE.md) — BDFL + maintainer model + RFC process.
- [`docs/project/MAINTAINERS.md`](docs/project/MAINTAINERS.md) — current maintainers.
- [`docs/project/SECURITY-GOVERNANCE.md`](docs/project/SECURITY-GOVERNANCE.md) — security policy, response, and the four-scan baseline pipeline.
- [`docs/project/COVERAGE-GATES.md`](docs/project/COVERAGE-GATES.md) — per-package coverage gates.
- [`docs/project/TEST-POLICY.md`](docs/project/TEST-POLICY.md) — race detector + goleak policy.
- [`docs/project/HARDENING-BASELINE.md`](docs/project/HARDENING-BASELINE.md) — v1.0 hardening audit.
- [`docs/project/PROFILING-BASELINE.md`](docs/project/PROFILING-BASELINE.md) — measured CPU/alloc baseline.
- [`SECURITY.md`](SECURITY.md) — vulnerability reporting.

## Public hosting

- **Primary**: [`codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core`](https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core)
- **Mirror (code-only)**: [`github.com/Spicer-Creek-Solutions-LLC/keystone-core`](https://github.com/Spicer-Creek-Solutions-LLC/keystone-core)

File issues, discussions, and contributions on Codeberg. The GitHub mirror exists for discoverability and is not where development happens.

Project sponsor: **Spicer Creek Solutions LLC** ([`OWNERSHIP.md`](OWNERSHIP.md)).

## Project status

> **Pre-1.0. Reconstruction approaching v1.0.**

This repository was reset to a clean reconstruction baseline on 2026-05-05. The prior implementation — substantial but unshippable as a coherent first release — is preserved on the `archive/v0` branch (tag `archive/v0-final`).

The 19 reconstruction epics in [`epics/`](epics/) sequence dependency-ordered v1.0 work. Track current progress in [`epics/00-meta-reconstruction-plan.md`](epics/00-meta-reconstruction-plan.md). The versioning scheme — three milestone tiers `v0.1` → `v0.5` (external-tester ready, all Linux) → `v1.0` (all 19 epics + SemVer stability) — is in [`docs/project/VERSIONING.md`](docs/project/VERSIONING.md).

## What v1.0 commits to

The full success bar — all 19 epics complete, contracts frozen, SemVer stability begins — is in [`docs/project/VERSIONING.md`](docs/project/VERSIONING.md). The headline commitment: a single-binary release that demonstrates one system can:

- Manage heterogeneous Linux fleets via a single agent over NATS.
- Run remote commands with rich targeting, batch concurrency, and streaming output.
- Apply declarative state through 35 universal Linux modules with drift detection and remediation.
- Run highly available out of the box — 3-node cluster, embedded etcd, leader election <3 s, failover <10 s.
- Issue real identity from day 1 — embedded SPIFFE-shaped CA, mTLS, join tokens, API keys, JWT.
- Broker secrets via encrypted-file or HashiCorp Vault backends.
- Integrate with GitOps tooling — Argo CD / Flux / GitHub / GitLab webhooks for verification and rollback.
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
- Policy *enforcement* — the `v0.x → v1.0` line is **audit-mode only**: policies evaluate + audit but never block (see [`docs/project/POLICY-AUDIT.md`](docs/project/POLICY-AUDIT.md))
- Federation, NATS supercluster / leaf nodes
- Web UI, blueprint marketplace, telemetry gateway
- Hugo docs site — pre-Hugo, the reference docs live as Markdown in `docs/project/`.

If you need any of these now, Keystone Core is not yet your tool.

## Building from source

Toolchain: Go 1.26.3+ (pinned in `go.mod`). The Makefile is the orchestrator — CI never runs raw `go test` / `go build`.

```bash
# Install dev tools (golangci-lint, gosec, govulncheck, gitleaks, go-licenses, …)
make install-tools

# Cross-platform builds (linux/amd64,arm64; darwin/amd64,arm64; windows/amd64,arm64)
make build-all-platforms

# Snapshot multi-arch tarballs
make release-snapshot
```

## Test surface

- `make test` — unit tests with `-race`.
- `make test-coverage` — same with `-coverprofile`; gated by `make coverage-gate` (critical ≥70%, CLI ≥40%).
- `make test-integration` — `-tags=integration`, against Postgres + embedded NATS. goleak active.
- `make slo` — wall-clock SLO assertions (HA: cluster forms <10 s, leader <3 s; perf: command latency <100 ms, event throughput >10k/s, batch-10 <2 s).
- `make e2e-test` — single-topology docker-compose: 11 scenarios covering infrastructure / registration / command / state / blueprint / module / secrets / audit / outbound webhook / GitOps webhook / rollback.
- `make profile` — pprof against the perf SLO workload.
- `make security-{secrets,vulns,sast,licenses}` — gitleaks / govulncheck / gosec / go-licenses.

Each gate is enforceable via Make + runs in CI. The full policy lives in [`docs/project/TEST-POLICY.md`](docs/project/TEST-POLICY.md) and [`docs/project/COVERAGE-GATES.md`](docs/project/COVERAGE-GATES.md).

## Contributing

Contributions are welcome. During the v1.0 sprint the epics are tightly sequenced — please open an issue or short RFC to coordinate before opening a code PR. Issues, doc improvements, and feedback on the epic structure are always welcome without coordination.

- [`CONTRIBUTING.md`](CONTRIBUTING.md) — how to contribute, DCO sign-off, AI disclosure.
- [`AGENTS.md`](AGENTS.md) — operational guidance for AI coding agents.
- [`docs/project/AI-CONTRIBUTIONS.md`](docs/project/AI-CONTRIBUTIONS.md) — AI contribution policy.
- [`docs/project/RFC.md`](docs/project/RFC.md) — propose larger changes via RFC.
- [`SECURITY.md`](SECURITY.md) — security policy and reporting.

### Transparency: AI use in this project

Keystone Core was originally bootstrapped with substantial AI assistance, and this reconstruction continues that practice openly. The objective is speed-to-foundation: get to a reviewable baseline quickly, then spend sustained human effort on correctness, scaling, and reliability.

Quality is enforced by process, not by origin. AI-assisted contributions are expected to meet the same standards as human-authored ones: readable design, reproducible builds, tests proportional to risk, security-minded changes, reviewable diffs. Maintainers are responsible for what gets merged. AI involvement must be disclosed in commits per [`docs/project/AI-CONTRIBUTIONS.md`](docs/project/AI-CONTRIBUTIONS.md).

## License

Apache License 2.0 — see [`LICENSE`](LICENSE) and [`NOTICE`](NOTICE).
