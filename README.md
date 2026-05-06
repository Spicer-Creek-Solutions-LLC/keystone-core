# Keystone Core

[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Status](https://img.shields.io/badge/Status-Pre--1.0_reconstruction-orange)](epics/00-meta-reconstruction-plan.md)
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

This repository was reset to a v1.0 reconstruction baseline on 2026-05-05. The prior implementation — substantial but unshippable as a coherent v1.0 — is preserved on the `archive/v0` branch (tag `archive/v0-final`).

The reset is deliberate: rather than carry forward technical debt from a sprawling pre-1.0 codebase, v1.0 is being rebuilt against a tight feature scope, a clear MVP definition, and an explicit anti-scope. The full rationale and per-epic plan live in [`epics/00-meta-reconstruction-plan.md`](epics/00-meta-reconstruction-plan.md).

If you were following the previous codebase: it isn't gone, it just isn't `main` anymore. `git fetch && git checkout archive/v0` to read it.

### Current state (epic 01 complete)

The foundations layer is in place. What's in `main` today:

- **Foundations packages**: `pkg/{version,semver,wait,dbutil,api/apierror}`, `internal/{config,logging,cli}`, `pkg/api/v1` (proto codegen pipeline).
- **Three hello-world binaries**: `kscore-server`, `kscore-agent`, `kscorectl`. They parse `--config` (YAML + `KSCORE_`-env via koanf), print a structured JSON startup log with a per-process correlation ID, and exit cleanly on SIGTERM/SIGINT.
- **Make-driven workflow**: `make build`, `test`, `lint`, `smoke`, `proto`, `release-snapshot`, etc. CI invokes Make targets exclusively.
- **Cross-compile matrix**: linux/{amd64,arm64}, darwin/{amd64,arm64}, windows/{amd64,arm64} — pure Go, no CGO.
- **CI**: Forgejo Actions (`.github/workflows/`) and Codeberg Woodpecker (`.woodpecker/`) — same Make targets, two runners.
- **Snapshot release**: `make release-snapshot` produces six tarballs/zips + a SHA-256 checksums file in `dist/`.

This is enough to build, test, and ship a tarball — no business logic yet. Real services start landing in epic 02 (storage layer) and epic 03 (gRPC/REST API surface). Track progress in [`epics/00-meta-reconstruction-plan.md`](epics/00-meta-reconstruction-plan.md).

## What v1.0 commits to

A trial-ready release in which a single binary, in a small lab environment, demonstrates that one system can:

- Manage heterogeneous Linux fleets (VMs, bare metal, mixed distros) via a single agent over NATS.
- Run remote commands with rich targeting, batch concurrency, and streaming output.
- Apply declarative state through ~40 universal Linux modules with drift detection and remediation.
- Run highly available out of the box — 3-node cluster, embedded etcd, leader election under 3 seconds, failover under 10.
- Issue real identity from day 1 — embedded SPIFFE-shaped CA, mTLS, join tokens, API keys, JWT.
- Broker secrets via encrypted-file or HashiCorp Vault backends.
- Integrate with GitOps tooling via Argo CD / Flux / GitHub / GitLab webhooks for verification and rollback.
- Be safely extensible through Cosign-signed Starlark modules with capability-based sandboxing.
- Compose higher-level operations via blueprints, runbooks, and a saga coordinator.
- Audit and observe every sensitive action — Prometheus metrics, OpenTelemetry traces, Grafana dashboards.
- Self-manage — bootstrap from seed, back up, restore, rate-limit, distribute files.

Full success criteria: [`docs/project/PROBLEM-STATEMENT.md`](docs/project/PROBLEM-STATEMENT.md).

## What v1.0 explicitly does NOT include

To keep the timebox honest, the following are deferred to later versions (each tagged in [`PROJECT-DETAILS.md §6`](PROJECT-DETAILS.md)):

- WASM module runtime, Windows agent, macOS agent
- Kubernetes operator embedding
- Full SPIRE integration
- Policy *enforcement* (v1.0 is audit-mode only)
- Federation, NATS supercluster / leaf nodes
- Web UI, blueprint marketplace, telemetry gateway

If you need any of these from a v1.0 release, Keystone Core is not yet your tool. If your interest is in trying a new day-2 control plane in a homelab during the v1.0 trial window, please follow along.

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

Keystone Core was originally bootstrapped with substantial AI assistance, and the v1.0 reconstruction continues that practice openly. The objective is speed-to-foundation: get to a reviewable baseline quickly, then spend sustained human effort on correctness, scaling, and reliability.

Quality is enforced by process, not by origin. AI-assisted contributions are expected to meet the same standards as human-authored ones: readable design, reproducible builds, tests proportional to risk, security-minded changes, reviewable diffs. Maintainers are responsible for what gets merged. AI involvement must be disclosed in commits per [`docs/project/AI-CONTRIBUTIONS.md`](docs/project/AI-CONTRIBUTIONS.md).

## License

Apache License 2.0 — see [`LICENSE`](LICENSE) and [`NOTICE`](NOTICE).
