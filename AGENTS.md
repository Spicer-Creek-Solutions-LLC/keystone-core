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

**Current Status**: Epics 1-32, 36-60 COMPLETE ✅

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

Epics 1-32, 36-59 are all complete. Key packages and where to find details:

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
| 50 | Outbound webhooks | `internal/webhook/outbound/` | `epics/50*.md` |
| 51 | HA resilience testing | `test/e2e/` | `epics/51*.md` |
| 52 | Critical bug fixes | Various | `epics/52*.md` |
| 53 | gRPC service completion | `pkg/api/server/`, `internal/statemgmt/history/` | `epics/53*.md` |

### Recently Completed

- **Epic 58** (Advanced State Orchestration) — COMPLETE — Two reference libraries plus decision matrix: `pkg/saga/` generic saga coordinator with compensating transactions, memory/SQLite logs, resume support; `pkg/statemachine/checkpoint/` persistent checkpoint adapter with ~string type constraint, OnTransition hook, Restore pattern, memory/SQLite stores; integration examples (upgrade saga, bootstrap checkpoint); orchestration patterns doc with decision flowchart, migration guidance, pattern comparison table

- **Epic 57** (Error Handling Hardening) — COMPLETE — Fixed ~16 silently ignored errors across 4 phases: API error handling (restore warnings, webhook marshal, lease parsing, RBAC init, audit logs), WASM error propagation (host function registration, writeResult), channel drop counters (NATS logging, SPIRE, secret audit, dashboard), ServiceMesh AuthorizationPolicy modification detection, wireCapabilities error return, DefaultLogger implementation

- **Epic 53** (gRPC Service Completion) — COMPLETE — Wired ClusterServer with real MembershipManager/LeaderElector/ShardManager; registered CoordinationService with NATS status adapter; implemented GetStateHistory/GetStateStatus with SQLite state history store; wired SecretsServer with BrokerBuilder when secrets.enabled; added SecretsConfig to config system; integration tests for state history

- **Epic 52** (Critical Bug Fixes) — COMPLETE — Deleted no-op EncryptedCache stub, wired real EncryptedSecretCache with AES-GCM in BrokerBuilder; fixed bootstrap handoff empty body (bytes.NewReader); wired gateway NATS TLS (nats.Secure); implemented real zstd/lz4/snappy compression; implemented FileRollback (os.WriteFile/Remove), PackageRollback/ServiceRollback with executor delegation; policy handlers return error instead of always-allow; implemented GetServerStatus with version/uptime/runtime stats

- **Epic 51** (HA Resilience Testing) — COMPLETE — 5 E2E resilience tests: NATS/etcd node failure, PostgreSQL failover, iptables-based network partitions, split-brain prevention

- **Epic 50** (Outbound Webhooks) - COMPLETE - Persistent outbound webhook subscriptions with SQLite store, HMAC-SHA256 signing, event dispatcher via NATS SubscribeQueue, exponential backoff retry, REST API (7 endpoints), CLI commands (6 subcommands)

- **Epic 55** (CLI Wiring: Secrets & Compliance) — COMPLETE — Replaced all generateSample*() stubs with real API calls:
  - Phase 1: `kscore-secrets` backends/audit/cache — Added REST endpoints (backends, cache stats), wired 5 commands
  - Phase 2: `kscore-secrets` rotations — Extended rotation REST endpoints (list, pause, resume, trigger), wired 10 rotate commands
  - Phase 3: `kscore-secrets` schedule/policy — Added rotation policy REST endpoints (7 routes), wired 10 schedule+policy commands
  - Phase 4: `kscore-secrets` remaining — Wired rewrap to transit REST, template resolves secrets client-side, rotate-keys returns not-yet-available
  - Phase 5: `kscore-audit` + `kscore-policy` — Created `pkg/policy/` gRPC client package (4 RPCs); wired 6 audit commands and 5 policy commands to PolicyService gRPC; schedule/remediate/monitor/analyze/watch return not-yet-available
  - Phase 6: Documentation and epic cleanup

- **Epic 54** (CLI Wiring: Core Operations) — COMPLETE — Replaced all generateSample*() stubs with real API calls:
  - Phase 1: `kscore-events` — 8 commands wired to EventService gRPC (list, query, emit, watch, subscribe, export, analyze, storage-stats); 11 not-yet-available commands return clear errors
  - Phase 2: `kscore-schedule` — Created Schedule REST API (19 routes: 10 schedules + 9 maintenance windows) backed by `schedule.Manager`/`MaintenanceWindowManager`; wired all 21 CLI commands via REST client; registered in `kscore-server` when cluster mode enabled
  - Phase 3: `kscore-runbook` — Extended runbook REST API with 5 new routes (list runbooks, list/get executions, execute, audit query); wired 8 stub commands (list, execute, status, list-executions, audit show/list/report, test) via REST client
  - Phase 4: `kscore-exec` — Replaced `output --follow` stub with polling-based approach using `GetBatchJobStatus`; tracks seen agents to avoid duplicate output
  - 15 handler tests (runbook), 5 exec tests, httptest-based CLI tests; all `generateSample*` functions removed

- **Epic 56** (CLI Wiring: GitOps & Infrastructure) — COMPLETE — Wired 7 CLI binaries:
  - Phase 1: `kscore-gitops` — Wired rollback to REST API; 9 remaining commands return "not yet available"; removed 8 types + 8 generators
  - Phase 2: `kscore-webhook` — Inbound stubs return "not yet available"; test wired to POST real payload; removed 3 types + 6 generators
  - Phase 2: `kscore-agents` — Removed sample data fallbacks; wired re-enroll/token create to POST /api/v1/cluster/tokens; removed broken generateRandomToken()
  - Phase 3: `kscore-module` — Replaced mockRegistryClient with registry.HTTPClient; added --registry flag; --coverage fails early; SumDB not yet available
  - Phase 3: `kscorectl` — Removed 5 maintenance mock data fallbacks (enable, disable, status, queue, cleanup)
  - Phase 4: `kscore-files` — --wait prints stderr warning; failover returns "not yet available"
  - `kscore-monitor` — No changes needed (hardcoded 0 metrics correct until aggregation endpoints exist)

### Recently Completed (cont.)

- **Epic 59** (Simplification) — COMPLETE — Inventory-driven codebase simplification:
  - Phase 1: 5 parallel inventory agents (binaries, duplication, dead code, dependencies, golden paths)
  - Phase 2: Makefile fix (11 missing binaries); removed 6 dead packages (~10,210 lines: federation, baremetal, dr, edge, visualization, transfer/throttle); removed dead `pkg/retry` (~987 lines)
  - Phase 3: Fixed 6 SQLite stores missing `SetMaxOpenConns(1)`; created `pkg/dbutil` with `OpenSQLite()` factory; fixed 2 `InsecureSkipVerify` security gaps (blueprint registry, file HTTP backend)
  - Phase 4: Investigated HTTP client factory, error standardization, config consolidation, build-tag gating — all deferred after analysis showed low ROI
  - Total: ~11,197 lines removed, 8 bugs fixed

- **Epic 38** (Air-Gapped Deployments) — COMPLETE:
  - Phase 1: Bootstrap packages with manifest types, signing/verification, binary collection, archive builder, content bundling (modules, blueprints, policies, docs), config templates (server/agent YAML, install script), installer, CLI (`kscore-bootstrap package create|verify|install|inspect`), Makefile target
  - Phase 2: Offline registry (`internal/airgap/registry/`) — filesystem-backed module registry wrapping `FilesystemBackend`; `LocalClient` implementing `resolver.RegistryClient`; JSON index with search; import from bootstrap packages or directories; export from online registries; garbage collection with version retention and max-age; Ed25519 trust store with signature verification; CLI wiring (`kscore-registry offline init|list|search|import|verify|gc|reindex|trust`; `module mirror` export/import)
  - Phase 3: Upgrade packages (`internal/airgap/upgrade/`) — manifest with from/to versions, migrations, scripts, config changes; builder creates signed tar.gz archives with binaries/modules/migrations/scripts; scanner compares directories to detect changes; package verifier with signature/checksum/compatibility checks; installer with extract→verify→backup→replace→verify workflow; rollback from backup; CLI wiring (`kscore-upgrade package create|verify|inspect|apply|rollback`)
  - Phase 4: Export/import for air-gapped data transfer (`internal/airgap/transfer/`) — ExportManifest with validation, DataCollector interface (EventCollector, AuditCollector, StateCollector) with paginated JSONL output; Exporter builds signed/encrypted tar.gz archives from collectors; Importer verifies signatures/checksums, decrypts .age packages, extracts with selective dataset filtering; CLI wiring (`kscore-transfer export|import|verify`)
  - Phase 5: Advanced features — sync window scheduling (`internal/airgap/sync/`) with state machine, cron-based scheduling, bandwidth limiter, priority operation queue; UDP data diode support (`internal/airgap/diode/`) with binary wire protocol, XOR parity FEC, sender/receiver; air-gap compliance validation (`internal/airgap/validate/`) scanning binaries, configs, modules, and network connections; CLI wiring (`kscore-transfer sync|diode`, `kscore-bootstrap airgap-validate`)

- **Epic 60** (MCP Server for AI-Assisted Operations) — COMPLETE — MCP server binary (`kscore-mcp`) exposing Keystone Core operations to AI clients (Claude Desktop, Claude Code, Cursor):
  - Phase 1: Foundation — `cmd/kscore-mcp` Cobra CLI, `internal/mcp/` config/server/gRPC client/profiles/metadata; server-side MCP metadata capture in auth interceptors; audit callback with MCP attribution; Makefile target
  - Phase 2: MVP Tools — 7 tools (agent_list/show/health, exec_run/status/history, cluster_status) with mock-based tests via MCP SDK InMemoryTransport
  - Phase 3: Extended Tools — 9 tools (state_check/drift/history/apply, event_query/stats, runbook_list/execute/status); 3 resources (keystone://agents, keystone://cluster/status, keystone://events/recent); httptest-based runbook tests
  - Phase 4: Documentation — MCP setup guide, security guide, CLI reference entry
  - Credential pass-through (operator's own creds), capability profiles (read_only/ops_safe/ops_admin), audit attribution via gRPC metadata headers, 116 tests total

### Future (Unnumbered)

- Web UI / Management Console (`epics/future-web-ui-management-console.md`)
- Release & Distribution (`epics/future-release-distribution.md`)
- Blueprint Marketplace (`epics/future-blueprint-marketplace.md`)
- Cross-Platform Testing (`epics/future-cross-platform-testing.md`)
- Multi-Cloud Testing (`epics/future-multi-cloud-testing.md`)

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

**Companion Services**: `kscore-mcp` (MCP server for AI clients)

**CLI Plugins** (26 built-in): `kscore-exec`, `kscore-state`, `kscore-module`, `kscore-monitor`, `kscore-agents`, `kscore-policy`, `kscore-audit`, `kscore-gitops`, `kscore-webhook`, `kscore-cluster`, `kscore-cluster-backup`, `kscore-identity`, `kscore-federation`, `kscore-blueprint`, `kscore-blueprint-publish`, `kscore-blueprint-state`, `kscore-files`, `kscore-files-storage`, `kscore-proxy`, `kscore-backup`, `kscore-events`, `kscore-schedule`, `kscore-upgrade`, `kscore-migrate`, `kscore-bootstrap`, `kscore-transfer`

**Dev/Test**: `kscore-loadtest`, `kscore-test`

Third-party: any `kscore-<name>` in `$PATH` works as `kscorectl <name>`

**Total**: 32 binaries (1 CLI + 4 servers + 1 companion + 26 plugins)

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
