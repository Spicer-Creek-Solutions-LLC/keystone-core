# AGENTS.md

This file provides guidance to Claude Code (claude.ai/code) or other agents when working with code in this repository.

---

# Our working relationship

- I don't like sycophancy
- Avoid flattery that feels like unnecessary praise
- Be anti-sycophantic - don't fold arguments just because I push back a little
- Be straightforward, and clear
- Be concise
- Avoid long-winded explanations
- Challenge my assumptions
- Don't be lazy. Do things the right way, not the easy way
- Be critical
- Fix bugs when you find them
- If a bug affects the work you're doing, fix it now.
- Don't defer fixing discovered bugs and don't create a follow-up task for it
- If a bug takes more than a moderate amount of work to fix, ask what to do
- Take the correct approach, not the easy one
- Don't add technical debt
- Always choose the long-term solution
- Make easily readable and maintainable code
- When there's a tradeoff, present the options with evidence and let the user decide

# Tooling

- Use Skills from ~/.claude/skills/ when tasks match their purpose
- If a Makefile exists, prefer its targets over calling tools directly (e.g. use `make test` instead of `go test ./...`)
- Use `make build` for compiling binaries (outputs to `build/`), not bare `go build`


## ⚠️ CRITICAL: TODO Approval Workflow ⚠️

**STOP** before fixing any TODO.md item. You MUST:
1. Present a plan to the user
2. **WAIT for explicit "yes" or approval**
3. Only then implement the fix

This applies ALWAYS - including after session resumption or context summarization.
**Never batch-fix TODOs without per-item approval.**

---

## Commit Message AI Attribution

- Use the DCO-required AI disclosure in commit messages.
- `Co-Authored-By` must identify the current agent (not Claude or another tool).
  Example: `Co-Authored-By: Codex <noreply@openai.com>` (adjust name/email per agent).

---
## Coding notes
- Do not add superfluous comments
- Commit and push as progress is made (don't wait until the end of a task)
- Keep user documentation updated as tasks are completed:
  - `docs/` - User-facing documentation
  - `AGENTS.md` - Agent instructions and project status
  - `README.md` - Project overview
  - `docs/content/en/docs/executive-summary/_index.md` - Executive summary
  - Other related documentation files as appropriate

### TODO Workflow

**IMPORTANT**: When working on items from `TODO.md`, always follow this workflow:

1. **Review** the TODO item and related code/documentation
2. **Present a plan** describing what changes will be made
3. **Wait for user approval** before making any changes
4. **Implement** the approved changes
5. **Commit and push** the changes

Do NOT proceed with TODO fixes without explicit user approval of the plan. This applies even when resuming from a previous session or continuing through multiple TODOs.

### Documentation Requirements

All code changes **must** include corresponding documentation updates:
- **CLI changes**: Update `docs/content/en/docs/reference/cli.md` and `cli-quick-reference.md`
- **New features**: Add user-facing documentation explaining usage and examples
- **API changes**: Update relevant API documentation
- **Configuration changes**: Update configuration reference docs

### Testing Requirements

All code changes **must** include corresponding tests:
- **New functions/methods**: Add unit tests covering normal operation, edge cases, and error conditions
- **New types**: Add tests for constructors, methods, and interface compliance
- **Bug fixes**: Add regression tests that would have caught the bug
- **Test coverage targets**: >70% for critical packages, >40% for CLI (see TODO.md)

Tests should follow existing patterns in the codebase:
- Use table-driven tests where appropriate
- Use `t.TempDir()` for filesystem isolation
- Test both success and error paths
- Include interface compliance tests (e.g., `var _ Interface = (*Type)(nil)`)

### State Machine Pattern Guidelines

Use the `pkg/statemachine` library for components with complex state transitions. See `docs/content/en/docs/contributing/state-machines.md` for full documentation and examples.

**When to use:** Components with 3+ states, lifecycle management, workflows with sequential steps, retry/recovery logic.

**Required:** Mermaid state diagrams in markdown docs (not code comments), test all valid/invalid transitions, use guards and callbacks.

---

## Repository Purpose

Keystone Core is a cloud-native runtime infrastructure control plane. Positioned as the operational layer between GitOps/IaC deployments and runtime infrastructure, inspired by Salt Project but modernized for cloud-native environments.

**Key Concept**: "GitOps deploys it. We keep it running."

## Project Status

**Current Status**: Epics 1-32, 36-37, 39-49 COMPLETE ✅

> For detailed implementation history of any epic, see `epics/<number>-*.md` and `git log`.

Working implementation with:
- Full NATS integration (embedded, external, leaf modes) with JetStream
- Agent system with registration, heartbeat, command execution
- SQLite/PostgreSQL state management with drift detection
- Git-style plugin CLI architecture (30 binaries)
- Event-driven automation with reactors, GitOps webhooks, policy enforcement (OPA/CEL)
- HA clustering with etcd, leader election, sharding
- Proxy agents for unmanaged devices (SSH, SNMP, REST, WinRM, NETCONF, RESTCONF, gNMI, Telnet — 25 vendor drivers)
- File distribution, mirror groups, multiple storage backends (S3, GCS, Azure, NATS)
- Runbook automation with triggers, approvals, ITSM integration
- Secrets management (REST + gRPC API, client package, CLI)
- Kubernetes operator (CRD watching, reconciliation, drift detection)
- gRPC services: ControlPlane, Secrets, Agent, State, Event, Policy, Cluster
- All 15 REST API handlers wired with real dependencies
- Comprehensive test suite (>79% coverage), 15 state machine components
- `pkg/wait` shared utilities for cancelable timers/polling across all packages
- Default TLS 1.3 minimum with per-component overrides

## Epic Status

### Completed

Epics 1-32, 36-37, 39-49 are all complete. Key packages and where to find details:

| Epic | Area | Key Packages | Details |
|------|------|-------------|---------|
| 1-3 | Core (NATS, execution, state) | `internal/controlplane/`, `internal/statemgmt/` | `epics/01-03*.md` |
| 4-6 | Events, GitOps, Policy | `internal/events/`, `internal/gitops/`, `internal/policy/` | `epics/04-06*.md` |
| 7,15,19 | Observability | `internal/metrics/`, `internal/tracing/`, `internal/gateway/` | `epics/07,15,19*.md` |
| 11,14 | Clustering, NATS mesh | `internal/cluster/`, `internal/nats/` | `epics/11,14*.md` |
| 21,42 | Proxy, protocols, vendors | `internal/proxy/`, `internal/credentials/rotation/` | `epics/21,42*.md` |
| 36,43 | Secrets | `internal/secrets/`, `pkg/secrets/`, `pkg/api/secrets/` | `epics/36,43*.md` |
| 37 | Runbooks | `internal/runbook/` | `epics/37*.md` |
| 41 | DNS | `internal/dns/` | `epics/41*.md` |
| 44 | Cluster join tokens | `internal/cluster/token/`, `pkg/api/cluster/` | `epics/44*.md` |
| 45 | Config wiring | `internal/config/` | `epics/45*.md` |
| 46 | gRPC services | `pkg/api/server/` | `epics/46*.md` |
| 47 | Registry backends | `internal/registry/storage/` | `epics/47*.md` |
| 48 | K8s operator | `internal/k8s/` | `epics/48*.md` |
| 49 | REST handler wiring | `cmd/kscore-server/main.go` | `epics/49*.md` |

### In Progress

- **Epic 43** (Secrets API) — Phase 1-4 complete (REST, gRPC, client, CLI). Phase 5 (real secret store wiring) not started.

### Planned

- **Epic 50** (Outbound Webhooks) - NOT STARTED - Persistent outbound webhook subscriptions, event dispatcher, HMAC signing, retry logic
- **Epic 51** (HA Resilience Testing) - NOT STARTED - NATS/etcd node failure, PostgreSQL failover, network partition, split-brain prevention

### Future (Not Yet Planned)

See `epics/` directory for full list. Key future work:
- **Epic 100**: 0.1.0 Release Readiness (`epics/100-release-readiness-0.1.0.md`)
- **Epic 38**: Air-Gapped Deployments
- **MCP Server**: AI-assisted operations (`epics/future-mcp-server.md`)
- **Web UI**: Management console (`epics/future-web-ui-management-console.md`)
- Multi-Tenancy, Terraform Provider, ITSM Integration, Compliance Presets

## Architecture Overview

**Core Components:**
- **Control Plane**: API Server, State Manager, Event/Reactor Engine
- **Message Bus**: NATS (embedded/external/hybrid modes) with JetStream
- **Agents**: Lightweight Go binaries on managed nodes (K8s, VMs, bare metal, edge)
- **State Storage**: SQLite (embedded) or PostgreSQL (production), with migration tooling

**Key Design Decisions**:
- NATS JetStream for events/messaging; SQLite/PostgreSQL for state (query patterns, indexing, transactions)
- SQLite for getting started, PostgreSQL for production

### Technology Stack
- **Language**: Go 1.25+
- **Message Bus**: NATS 2.10+ with JetStream
- **State Storage**: SQLite 3.x (modernc.org/sqlite, pure Go) or PostgreSQL 14+
- **API**: gRPC + REST
- **Observability**: Prometheus, OpenTelemetry, Grafana
- **Policy**: OPA (Rego), CEL
- **Modules**: Starlark, WASM (wazero), Cosign signatures

### Module System

Capability-based security model with sandboxed execution. Modules identified as `vendor/package` (e.g., `std/files`). Host capabilities: `fs.read/write`, `http.get/post`, `exec`, `secrets.read/write`, `log`, `time`, `kv`. See module manifests (`module.yaml`, `module.lock`).

## CLI Architecture (Plugin Pattern)

Git-style plugin architecture: `kscorectl` dispatches to `kscore-*` binaries in `$PATH`.

**Server Daemons**: `kscore-server`, `kscore-agent`, `kscore-registry`, `kscore-telemetry-gateway`

**CLI Plugins** (25 built-in): `kscore-exec`, `kscore-state`, `kscore-module`, `kscore-monitor`, `kscore-agents`, `kscore-policy`, `kscore-audit`, `kscore-gitops`, `kscore-webhook`, `kscore-cluster`, `kscore-cluster-backup`, `kscore-identity`, `kscore-federation`, `kscore-blueprint`, `kscore-blueprint-publish`, `kscore-blueprint-state`, `kscore-files`, `kscore-files-storage`, `kscore-proxy`, `kscore-backup`, `kscore-events`, `kscore-schedule`, `kscore-upgrade`, `kscore-migrate`, `kscore-bootstrap`

**Dev/Test**: `kscore-loadtest`, `kscore-test`

Third-party: any `kscore-<name>` in `$PATH` works as `kscorectl <name>`

**Total**: 30 binaries (1 CLI + 4 servers + 25 plugins)

## Key Design Principles

1. **Zero Dependencies for Getting Started**: Embedded NATS + SQLite, single binary
2. **Security by Default**: Capability-based access, signed plugins, policy enforcement
3. **Determinism**: Reproducible plugin execution
4. **Minimal Attack Surface**: Sandbox all untrusted code
5. **Auditability**: Comprehensive logging, transparency logs
6. **Performance**: <100ms command execution to 1000 nodes
7. **Hybrid Infrastructure**: Unified K8s, VMs, bare metal, edge
8. **Graceful Scaling**: Embedded → External NATS; SQLite → PostgreSQL

## Documentation Formatting

**Diagrams must use Mermaid format** in markdown files, not ASCII art or code comments.

## Where to Find Details

- **Design document**: `docs/project/DESIGN.md`
- **Epic plans**: `epics/` directory (one file per epic)
- **API reference**: `docs/content/en/docs/reference/api.md`
- **Configuration reference**: `docs/content/en/docs/reference/configuration.md`
- **CLI reference**: `docs/content/en/docs/reference/cli.md`
- **State machines**: `docs/content/en/docs/contributing/state-machines.md`
- **Git history**: `git log --oneline` for implementation details of any completed epic
