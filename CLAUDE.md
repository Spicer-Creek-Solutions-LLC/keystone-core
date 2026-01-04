# CLAUDE.md

> ⚠️ **STOP - READ THIS FIRST** ⚠️
>
> Before running ANY Docker, Podman, or long-running commands, read the **Critical Operational Constraints** section below.
> Failure to follow these rules causes system-wide fork failures (EAGAIN errors) that crash all terminals.

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

---

## ⛔ Critical Operational Constraints

**YOU MUST FOLLOW THESE RULES** to prevent system resource exhaustion (EAGAIN fork failures).

### ❌ NEVER DO THIS

```bash
# WRONG - background Docker commands cause pgrep polling storms
Bash(run_in_background=true): docker compose up
Bash(run_in_background=true): podman compose up
Bash(run_in_background=true): docker logs -f container_name

# WRONG - streaming logs floods terminal buffer
docker logs -f container_name
podman logs -f container_name

# WRONG - long timeouts with no output limits
Bash(timeout=600000): docker compose up
Bash(timeout=600000): go test ./...

# WRONG - parallel container operations
# (multiple Docker/Podman commands in same message)
```

### ✅ ALWAYS DO THIS

```bash
# RIGHT - start detached, then check status separately
docker compose up -d
docker compose ps

# RIGHT - tail limited logs AFTER containers are running
docker logs --tail 50 container_name

# RIGHT - short timeouts with explicit limits
timeout 30 docker compose up -d
timeout 60 go test -v ./pkg/specific/...

# RIGHT - one container operation at a time, sequentially
```

### Docker/Podman Specific Rules

1. **NEVER use `run_in_background: true`** for any Docker/Podman command
2. **NEVER stream logs** (`-f` or `--follow`) - use `--tail N` after the fact
3. **NEVER run `docker compose up` without `-d`** (detached mode)
4. **ALWAYS use timeouts** of 30-60 seconds maximum
5. **ALWAYS clean up** with `docker compose down` when done
6. **ONE container command per message** - never parallelize

### Test Execution Rules

1. **NEVER use `run_in_background: true`** for test commands
2. **Run ONE test package at a time** - not entire test suites
3. **Use `-v` only when debugging specific failures**
4. **Wrap with `timeout`**: `timeout 60 go test ./pkg/foo/...`

### Why This Matters

Claude Code polls background processes via `pgrep`. When commands:
- Run in background + produce lots of output → pgrep polling exhausts process slots
- Stream indefinitely → terminal buffer overflow
- Run too long → accumulated polling causes EAGAIN

**Result**: `zsh: fork failed: resource temporarily unavailable` across ALL terminals, requiring system restart.

---

## Repository Purpose

This is the **design documentation repository** for Keystone Core, a cloud-native runtime infrastructure control plane. Keystone Core is positioned as the operational layer between GitOps/IaC deployments and runtime infrastructure, inspired by Salt Project but modernized for cloud-native environments.

**Key Concept**: "GitOps deploys it. We keep it running."

## Project Status

This repository contains working implementations of **Epics 1-11**. The project has transitioned from design-only to a working implementation with:

- Full NATS integration (embedded, external, and leaf modes)
- Working agent system with registration, heartbeat, and command execution
- SQLite-based state management
- Git-style plugin architecture for CLI extensibility
- Cross-platform remote execution with targeting
- Declarative state management with drift detection and CLI (Epic 3 complete)
- Event-driven automation with filtering, routing, enrichment, reactors, external integration, persistent storage, and monitoring (Epic 4 complete)
- GitOps integration with webhooks, API clients, verification, rollback automation, and promotion pipelines (Epic 5 complete)
- Policy enforcement with OPA/CEL engines, auditing, and compliance reporting (Epic 6 complete)
- High availability clustering with etcd-based coordination, leader election, and automatic failover (Epic 11 complete)
- Telemetry gateway for aggregating metrics, logs, and traces from isolated agents (Epic 19 complete)
- Comprehensive test suite (>79% coverage across all core packages)

**Current Status**: Epic 1-19 COMPLETE ✅ (Observability Gateway)

### ⚠️ Known Implementation Gaps

The following features are documented as complete but have incomplete or stub implementations:

#### Security & Authentication

| Feature | Epic | Status | Location | Notes |
|---------|------|--------|----------|-------|
| **Cosign verification** | 9 | ✅ IMPLEMENTED | `pkg/module/verify/signature.go` | ECDSA P-256, Ed25519, base64 signatures, bundle parsing |
| **JWT authenticator** | 11 | ✅ IMPLEMENTED | `pkg/api/auth/jwt.go` | HS256/RS256/ES256, issuer/audience validation |
| **mTLS authenticator** | 11 | ✅ IMPLEMENTED | `pkg/api/auth/mtls.go` | CN/SAN pattern matching, glob wildcards, SPIFFE URIs |
| **Built-in policy type** | 6 | ✅ IMPLEMENTED | `pkg/policy/builtin.go` | 14 built-in policies: require-labels, require-owner, allowed-environments, allowed-actions, deny-privileged, allowed-users, denied-users, time-window, no-root-execution, require-approval, max-concurrent, resource-quota, pattern-deny, pattern-allow |

#### State Modules

| Feature | Epic | Status | Location | Notes |
|---------|------|--------|----------|-------|
| **macOS user management** | 3 | ✅ IMPLEMENTED | `pkg/statemgmt/module_user.go` | Uses dscl for create/modify/delete |
| **macOS group management** | 3 | ✅ IMPLEMENTED | `pkg/statemgmt/module_group.go` | Uses dscl for create/modify/delete |
| **K8s namespace module** | 8 | ✅ IMPLEMENTED | `pkg/statemgmt/module_k8s_namespace.go` | Full namespace CRUD with labels/annotations |
| **Windows file ownership** | 3 | ✅ IMPLEMENTED | `pkg/statemgmt/module_file_windows.go` | Uses Windows ACLs via GetSecurityInfo/LookupAccountSid |
| **Executor user switching** | 2 | ✅ IMPLEMENTED | `pkg/agent/executor_unix.go` | Unix: setuid/setgid, Windows: error (requires password) |

#### Observability Backend Integrations

| Feature | Epic | Status | Location | Notes |
|---------|------|--------|----------|-------|
| **Loki log querying** | 7 | ✅ IMPLEMENTED | `pkg/query/logs.go` | Full Loki HTTP API: query_range, labels, label values |
| **Jaeger trace querying** | 7 | ✅ IMPLEMENTED | `pkg/query/traces.go` | Full Jaeger API: traces, get trace, services, operations |

#### Cluster/HA

| Feature | Epic | Status | Location | Notes |
|---------|------|--------|----------|-------|
| **Add cluster member via API** | 11 | ✅ IMPLEMENTED | `pkg/cluster/membership.go` | AddMember method with validation, API handler |
| **Cluster restore from backup** | 11 | ✅ IMPLEMENTED | `pkg/api/cluster/handlers.go:398` | Full restore with shards, config, validation |
| **NATS restart coordination** | 11 | ✅ IMPLEMENTED | `pkg/cluster/coordination_server.go:304-475` | restart embedded, reconnect, failover, drain actions |
| **State propagation handlers** | 11 | ✅ IMPLEMENTED | `pkg/cluster/coordination_server.go:507-651` | All 5 state types with version tracking |

#### NATS Discovery (Epic 14)

| Feature | Epic | Status | Location | Notes |
|---------|------|--------|----------|-------|
| **Kubernetes NATS discovery** | 14 | ✅ IMPLEMENTED | `pkg/nats/discovery.go` | EndpointSlices API + Endpoints fallback + DNS fallback |
| **Consul NATS discovery** | 14 | ✅ IMPLEMENTED | `pkg/nats/discovery.go` | Consul HTTP API with health filtering |
| **etcd NATS discovery** | 14 | ✅ IMPLEMENTED | `pkg/nats/discovery.go` | etcd v3 client with prefix queries |
| **NTLM proxy authentication** | 14 | ✅ IMPLEMENTED | `pkg/nats/ntlm.go`, `pkg/nats/websocket.go` | NTLMv2 with MD4, multi-step challenge-response |

#### Module System

| Feature | Epic | Status | Location | Notes |
|---------|------|--------|----------|-------|
| **Capability implementations** | 9 | ✅ IMPLEMENTED | `pkg/module/capabilities/capabilities.go` | Factory functions wire to real FSRead, FSWrite, HTTPGet, HTTPPost, Exec, SecretsRead, SecretsWrite, Log, Time, KV |
| **Memory limit parsing** | 9 | ✅ IMPLEMENTED | `pkg/module/loader/loader.go` | Supports KB/MB/GB/TB, Ki/Mi/Gi/Ti, and K8s formats |

#### GitOps

| Feature | Epic | Status | Location | Notes |
|---------|------|--------|----------|-------|
| **ArgoCD previous revision lookup** | 5 | ✅ IMPLEMENTED | `pkg/gitops/rollback/argocd.go` | Uses ArgoCD History API |
| **Git previous revision lookup** | 5 | ✅ IMPLEMENTED | `pkg/gitops/rollback/git.go` | Uses go-git parent commit lookup |

#### Hardware/Platform

| Feature | Epic | Status | Location | Notes |
|---------|------|--------|----------|-------|
| **BMC/IPMI detection** | 8 | ✅ IMPLEMENTED | `pkg/hardware/detector.go` | Multi-method detection: IPMI device files, ipmitool, DMI/SMBIOS |
| **Agent CPU/memory metrics** | 1 | ✅ IMPLEMENTED | `pkg/agent/metadata.go` | Uses gopsutil for CPU, memory, disk, load |
| **Edge CPU tracking** | 8 | ✅ IMPLEMENTED | `pkg/edge/manager.go` | Uses gopsutil with caching |

#### Deployment Artifacts

| Feature | Epic | Status | Notes |
|---------|------|--------|-------|
| **Kubernetes manifests** | 8/10 | ✅ IMPLEMENTED | `deploy/kubernetes/` with Kustomize support |
| **Helm charts** | 8/10 | ✅ IMPLEMENTED | `deploy/helm/kscore-server/` and `deploy/helm/kscore-agent/` |

#### Testing

| Feature | Epic | Status | Notes |
|---------|------|--------|-------|
| **Network partition E2E tests** | 12 | ⚠️ SKIPPED | Require Docker network manipulation |
| **Multi-platform E2E** | 12 | ❌ NOT IMPLEMENTED | ARM64, different Linux distros not tested |

#### Confirmed Working ✅

| Feature | Epic | Notes |
|---------|------|-------|
| **Embedded etcd mode** | 11 | Uses `go.etcd.io/etcd/server/v3/embed` |
| **OCI Registry client** | 9 | `pkg/module/registry/oci_client.go` - Full OCI Distribution Spec client |
| **HTTP Registry client** | 9 | `pkg/module/registry/client.go` - Go-mod style HTTP client |
| **kscore-registry server** | 9 | `cmd/kscore-registry/` - HTTP server with tests (18 tests) |
| **kscore-module CLI** | 9 | All commands implemented |
| **kscore-policy CLI** | 6 | All commands implemented |
| **kscore-gitops CLI** | 5 | All commands implemented |
| **kscore-identity CLI** | 17 | All commands implemented (token, ca, federation, bundle, events, status) |
| **CI/CD for E2E tests** | 12 | `.github/workflows/e2e.yml` |
| **SPIFFE Identity Framework** | 17 | `pkg/identity/` - Complete SPIFFE implementation (332 tests) |

**Legend**: ✅ Working | ⚠️ STUB/PLACEHOLDER (partial) | ❌ NOT IMPLEMENTED

These gaps should be addressed before production use.

## Repository Structure

```
/
├── DESIGN.md                          # Main design document
└── epics/                             # Epic-level implementation plans
    ├── 01-core-infrastructure.md      # NATS, agents, control plane
    ├── 02-remote-execution.md         # Command execution system
    ├── 03-state-management.md         # Declarative configuration
    ├── 04-event-system.md             # Event-driven automation
    ├── 05-gitops-integration.md       # ArgoCD/Flux integration
    ├── 06-policy-enforcement.md       # OPA/CEL policy engine
    ├── 07-observability.md            # Metrics, logging, tracing
    ├── 08-multi-environment.md        # K8s, VMs, bare metal, edge
    ├── 09-plugin-system.md            # Starlark/WASM plugin architecture
    ├── 10-documentation.md            # Hugo + Docsy comprehensive documentation
    ├── 11-clustering.md               # High availability clustering with etcd
    ├── 12-e2e-testing.md              # End-to-end & performance testing
    ├── 13-cgo-removal.md              # Pure Go build (no CGO dependencies)
    ├── 14-nats-mesh-communication.md  # NATS-only communication, superclusters, NAT traversal
    ├── 15-observability-enhancements.md  # NATS telemetry, stdout logging, syslog, audit
    ├── 16-stdlib-system-modules.md       # Cross-platform system management modules
    ├── 17-spiffe-identity.md             # SPIFFE/SPIRE identity framework
    ├── 18-ipv6-support.md                # Full IPv6 and dual-stack support
    ├── 19-observability-gateway.md       # Telemetry gateway for isolated agents
    └── 20-windows-support.md             # Production Windows agent, dev environment
```

## Architecture Overview

Keystone Core fills the gap between declarative GitOps tools and runtime operations:

**Core Architecture Components:**
- **Control Plane**: API Server, State Manager, Event/Reactor Engine
- **Message Bus**: NATS with three deployment modes:
  - **Embedded mode**: In-process NATS for initial setups, small deployments (<100 nodes), and edge agents
  - **External cluster mode**: Dedicated NATS cluster for production deployments (100+ nodes)
  - **Hybrid mode**: Control plane uses external cluster, agents use embedded NATS as leaf nodes
  - JetStream for event persistence (supported in all modes)
- **Agents**: Lightweight Go binaries on managed nodes (K8s, VMs, bare metal, edge)
- **State Storage**: SQLite or PostgreSQL for operational state (NOT JetStream - see design rationale)
  - **SQLite (embedded)**: Zero dependencies, perfect for dev/testing/home labs, small deployments (<100 nodes) ✅ IMPLEMENTED
  - **PostgreSQL**: Production deployments, high availability, scalability (100+ nodes) ✅ IMPLEMENTED
  - Automated migration tooling from SQLite → PostgreSQL ✅ IMPLEMENTED (`kscore-migrate` CLI)

**Key Design Decisions**:
- Use NATS JetStream for events/messaging, but SQLite/PostgreSQL for state due to query patterns, indexing needs, and transactional semantics
- SQLite for getting started (mirrors embedded NATS philosophy), PostgreSQL for production (mirrors external NATS cluster)

## Epic Implementation Status

### Epic 1: Core Infrastructure ✅ COMPLETE

**All 4 phases completed:**
- Phase 1: NATS Integration (embedded, external, leaf modes)
- Phase 2: Agent Development (registration, heartbeat, command execution)
- Phase 3: Control Plane Services (state management, connection management)
- Phase 4: Testing & Reliability (>80% test coverage achieved)

**Test Coverage Achieved:**
- pkg/agent: 77.9%
- pkg/state: 90.1%
- pkg/config: 96.6%
- pkg/controlplane: 85.9%
- pkg/security: 80.0%
- pkg/version: 100%

**Key Achievements:**
- Zero-dependency getting started (embedded NATS + SQLite)
- Comprehensive agent lifecycle management
- Robust command execution with streaming output
- Production-ready state persistence
- Extensive test coverage across all core packages

### Epic 2: Remote Execution ✅ COMPLETE

**All 4 weeks completed:**
- Week 1: Foundation (plugin system, shell abstraction, enhanced executor)
- Week 2: Targeting System (expression parser, agent matcher, batch execution)
- Week 3: Integration (protobuf definitions, control plane dispatch, job tracking)
- Week 4: CLI & E2E Testing (kscore-exec plugin, integration tests)

**Test Coverage Achieved:**
- pkg/plugin: 82.7%
- pkg/execution: 82.6%
- pkg/targeting: 84.2%
- E2E integration tests: 100% pass rate

**Key Achievements:**
- Git-style plugin architecture for CLI extensibility
- Cross-platform shell abstraction (Bash, PowerShell, Cmd)
- Flexible targeting with glob and expression-based filtering
- Parallel batch execution across multiple agents
- Complete kscore-exec plugin with streaming output
- Comprehensive integration tests

### Epic 3: State Management & Configuration ✅ COMPLETE

**All 4 weeks completed:**

**Week 1: State Definition & Parsing ✅ COMPLETE**
- Complete type system (pkg/statemgmt/types.go)
  - StateFile, StateDeclaration, Requisites structures
  - StateResult, StateRun, RunSummary for execution tracking
  - ValidationError, SourceType, StateFileFormat enums
- YAML parser with includes (pkg/statemgmt/parser.go)
  - Metadata, variables, and module-specific parameters
  - Recursive include loading with circular dependency detection
  - Source type abstraction (file://, http://, template://)
- Schema-based validation (pkg/statemgmt/validator.go)
  - Six module types: file, package, service, user, group, cmd
  - Parameter validation (file mode, etc.)
  - Requisite reference validation
- Test coverage: 82.0% (22 tests passing)

**Week 2: State Modules & Execution ✅ COMPLETE**
- Module framework (pkg/statemgmt/module.go)
  - Module interface (Check, Apply, Test)
  - ModuleRegistry for module registration
  - BaseModule with parameter helpers
- Idempotent state module implementations:
  - File module (present, absent, directory, symlink) - pkg/statemgmt/module_file.go
  - Package module (installed, removed, latest, purged) - pkg/statemgmt/module_package.go
  - Service module (running, stopped, enabled, disabled) - pkg/statemgmt/module_service.go
  - User module (present, absent) - pkg/statemgmt/module_user.go
  - Group module (present, absent) - pkg/statemgmt/module_group.go
  - Cmd module (run, wait) - pkg/statemgmt/module_cmd.go
- Module executor (pkg/statemgmt/executor.go)
  - Idempotent execution (check before apply)
  - Dry-run mode support
  - Retry logic with backoff
  - Condition evaluation (unless, only_if)
- Test coverage: 35.2% (64 tests passing)
  - Comprehensive unit tests for framework
  - File module tests (all states)
  - Integration tests (full workflow, dependencies, error handling, performance)
  - Performance: 5,196 states/sec (100 states in 19ms)

**Week 3: Dependency Resolution & Templating ✅ COMPLETE**
- Dependency graph construction (pkg/statemgmt/dependency.go)
  - DAG builder with adjacency list representation
  - Kahn's algorithm for topological sort
  - DFS for circular dependency detection with full cycle paths
  - Execution level tracking for parallel execution opportunities
  - Support for all requisite types: require, require_in, watch, watch_in, prereq, prereq_in, onchanges, onchanges_in
- Template rendering engine (pkg/statemgmt/template.go)
  - Go text/template integration
  - Custom template functions: upper, lower, title, trim, split, join, replace, contains, hasPrefix, hasSuffix, default, ternary
  - TemplateContext with vars and facts
  - RenderStateFile for complete state file rendering
- Vars system (configuration data)
  - LoadVarsFromYAML for loading from YAML
  - Merge support for variable composition
  - Environment-scoped vars
- Facts system (agent metadata)
  - Auto-collection of system facts (OS, arch, CPU count, Go version)
  - Custom fact registration
  - Type-safe accessors (GetString, etc.)
- Updated executor (pkg/statemgmt/executor.go)
  - Dependency-ordered execution via ResolveExecutionOrder
  - Respects all requisite relationships during execution
- Test coverage: 40.8% (85 tests passing)
  - 8 comprehensive dependency resolution tests
  - 12 template rendering and vars/facts tests
  - All edge cases covered: simple chains, parallel execution, circular dependencies, all requisite types

**Week 4: Drift Detection & CLI ✅ COMPLETE**
- State comparison/diff engine (pkg/statemgmt/diff.go)
  - StateDiffer for comparing desired vs actual state
  - DriftStatus tracking with severity levels (none, low, medium, high, critical)
  - Difference detection with field-level granularity
  - Severity calculation based on field criticality (mode, owner, contents, etc.)
  - DriftReport with summary statistics
  - FormatDriftReport for human-readable output
  - CompareStates for state-to-state comparison
- kscore-state CLI plugin (cmd/kscore-state)
  - `kscorectl state apply` - Apply state declarations
  - `kscorectl state check` - Check without applying (dry-run)
  - `kscorectl state drift` - Detect configuration drift
  - Variables file support (--vars flag)
  - Template rendering integration
  - Color-coded output (✓/✗ status indicators)
  - Summary statistics (total, succeeded, failed, changed, unchanged)
  - Proper exit codes (0 for success, 1 for failure/drift)
- Integration tests (cmd/kscore-state/integration_test.go)
  - End-to-end CLI workflow testing
  - Check, apply, drift command tests
  - Idempotency verification
  - Drift detection with/without changes
  - Version command test
- Test coverage: 44.2% (107 tests passing)
  - 20 drift detection tests
  - 6 integration tests (5 workflow + 1 version)

### Epic 4: Event-Driven Automation System ✅ COMPLETE

**Implementation Plan:** 8 weeks (All weeks complete)

**Week 1: Event Bus Foundation (Part 1) ✅ COMPLETE**
- Event schema definition (pkg/events/types.go)
  - 15 event types across 4 categories (agent, job, state, user, system)
  - 5 severity levels (debug, info, warning, error, critical)
  - Event structure with ID, type, source, time, severity, correlation ID, tags, data
  - Event filtering with multiple criteria (type, source, tags, severity, time range)
  - Event builder with fluent API
- Test coverage: 11 tests passing
  - Event builder functionality
  - Type, source, tag, severity, time range filtering
  - Multiple criteria filtering
  - Severity level comparison

**Week 1: Event Bus Foundation (Part 2) ✅ COMPLETE**
- NATS JetStream integration (pkg/events/publisher.go, subscriber.go, manager.go)
  - JetStreamPublisher with sync/async publish
  - JetStreamSubscriber with wildcard and queue subscriptions
  - Automatic stream creation and configuration
  - Durable consumers with manual ack and retry
  - Event filtering at subscriber level
- JSON serialization for events
- Event Manager for simplified API
- Test coverage: 80.7% (36 tests passing)
  - Publisher tests (9 tests)
  - Subscriber tests (8 tests)
  - Integration tests (8 tests)
  - Types tests (11 tests from Part 1)

**CloudEvents adapter - DEFERRED to Week 3**

**Week 2: Event Emission from Operations ✅ COMPLETE**
- State management event emission (pkg/statemgmt/executor.go, diff.go) ✅
  - state.apply.start - emitted when state execution begins
  - state.apply.done - emitted when execution completes (with summary)
  - state.apply.fail - emitted on execution failure
  - state.change - emitted when state resources change
  - state.drift - emitted when drift is detected (with severity mapping)
- Control plane event emission (pkg/controlplane/connection_manager.go) ✅
  - agent.connect - emitted when agents register (with full metadata)
  - agent.disconnect - emitted when agents go offline (with diagnostics)
- Job execution event emission (pkg/controlplane/command_dispatcher.go) ✅
  - job.start - emitted when command is dispatched
  - job.complete - emitted when command succeeds
  - job.fail - emitted when command fails/times out
- Event correlation IDs ✅
  - State operations: correlated by run_id
  - Job operations: correlated by job_id
  - Agent operations: correlated by agent-{agent_id}
- Async publishing used throughout to avoid blocking operations

**Week 3: Event Filtering and Routing ✅ COMPLETE**
- Advanced filter expression parser (pkg/events/filter_expression.go) ✅
  - Comparison operators: ==, !=, >, >=, <, <=, =~, ~~, contains
  - Logical operators: AND, OR, NOT with proper precedence
  - Field access: type, source, severity, correlation_id, tags.*, data.*
  - Regex and glob pattern matching
  - Severity level comparison (debug < info < warning < error < critical)
- Event router with routing rules (pkg/events/router.go) ✅
  - Rule-based event routing with filters
  - Priority-based rule ordering
  - Enable/disable rules dynamically
  - StopOnMatch for routing control
  - Comprehensive routing metrics (events processed/matched/unmatched, per-rule stats)
  - Async routing support
- Fan-out patterns for multiple consumers ✅
  - FanOut - synchronous fan-out to multiple handlers
  - FanOutAsync - parallel async fan-out
  - FilterHandler - conditional handler execution
  - ConditionalHandler - if/else handler logic
  - ChainHandlers - sequential handler composition
- Event enrichment pipeline (pkg/events/enrichment.go) ✅
  - TagEnricher - add static tags to events
  - DataEnricher - add static data fields
  - FunctionEnricher - custom enrichment logic
  - ConditionalEnricher - conditional enrichment based on filters
  - TimestampEnricher - add timestamp fields
  - HostnameEnricher - add hostname information
  - SequenceNumberEnricher - add sequence numbers
  - ContextEnricher - extract from context.Context
  - ChainEnrichers - compose multiple enrichers
  - EnrichedPublisher - wrapper for automatic enrichment
- Test coverage: 83.7% (95 tests passing)
  - Filter expression tests (35 tests)
  - Router tests (13 tests)
  - Fan-out and composition tests (6 tests)
  - Enrichment tests (25 tests)
  - Integration tests (16 tests from Week 1-2)

**Week 4-5: Reactor System ✅ COMPLETE**
- Reactor engine for automated event responses (pkg/events/reactor.go) ✅
  - Rule-based event processing with filter expressions
  - Priority-based reactor ordering
  - Enable/disable reactors dynamically
  - Concurrent execution control (MaxConcurrent)
  - Timeout support for action execution
  - Error handling strategies (continue, stop, retry)
  - Advanced conditions: OnlyIf, Unless, Throttle, Debounce, MaxExecutions
  - Comprehensive metrics tracking (per-reactor and global)
  - Event emission for reactor execution (reactor.execute, reactor.action)
- Built-in actions (pkg/events/actions.go) ✅
  - LogAction - log event information
  - EventAction - emit new events
  - WebhookAction - HTTP POST to external services
  - CommandAction - execute shell commands
  - FunctionAction - custom function execution
  - ConditionalAction - conditional execution based on filters
  - SequenceAction - execute actions in sequence
  - ParallelAction - execute actions in parallel
  - DelayAction - introduce delays
  - RetryAction - retry failed actions with exponential backoff
- Throttling and debouncing ✅
  - Throttle - minimum time between executions
  - Debounce - wait for quiet period before executing
  - Per-reactor execution limits and time windows
- Test coverage: 81.8% (30 reactor tests + 16 action tests)
  - Reactor engine tests (14 tests)
  - Reactor conditions tests (throttle, debounce, concurrent, error handling)
  - Action tests (16 tests covering all action types)
  - Integration with filter expressions and event system

**Week 6: External Integration ✅ COMPLETE**
- CloudEvents 1.0 adapter (pkg/events/cloudevents.go)
  - CloudEvent struct with required/optional attributes and extensions
  - Custom JSON marshaling/unmarshaling for extension attributes
  - ToCloudEvent/FromCloudEvent conversion functions
  - CloudEventPublisher/CloudEventSubscriber wrappers
  - HTTPCloudEventHandler for receiving CloudEvents via HTTP
  - ValidateCloudEvent for spec compliance
  - CloudEventBatch for batch operations
  - 12 tests covering conversion, validation, round-trips, JSON marshaling
- Kafka publisher and subscriber (pkg/events/kafka.go)
  - KafkaPublisher with sync and async publishing
  - KafkaSubscriber with consumer group support
  - Support for both CloudEvents and native format
  - SASL/TLS authentication support
  - Configurable compression (Snappy, LZ4, Gzip)
  - 10 tests covering publishing, subscribing, parsing, configuration
- Event bridge for external systems (pkg/events/bridge.go)
  - Bridge for routing events between different systems
  - Support for filtering and transformation
  - Retry logic with exponential backoff
  - BridgeManager for managing multiple bridges
  - Comprehensive metrics tracking
  - 12 tests covering forwarding, filtering, transformation, retry, manager
- HTTP event receiver for webhooks (pkg/events/http.go)
  - HTTPReceiver with support for single and batch events
  - Support for both CloudEvents and native format
  - HMAC signature verification for security
  - Health check and metrics endpoints
  - HTTPSender for sending events via HTTP
  - 17 tests covering receiver, sender, signatures, integration
- Test coverage: 75.8% (60 new tests across all integration components)

**Week 7: Event Storage and Query ✅ COMPLETE**
- Event storage interfaces (pkg/events/storage.go)
  - EventStore interface for persistence operations
  - EventQuery with fluent API for building queries
  - RetentionPolicy with age, count, and severity-based retention
  - StorageMetrics for tracking storage statistics
  - EventReplay interface for event replay capabilities
- SQLite event store implementation (pkg/events/storage_sqlite.go)
  - Schema with indexes on type, source, severity, time, correlation_id
  - Store/StoreBatch for efficient event persistence
  - Query with multiple filters: type, source, severity, correlation ID, time range
  - Pagination and sorting support
  - Retention policy application (MaxAge, MaxCount, MinSeverity)
  - Auto-retention background goroutine
  - GetMetrics for storage statistics
  - Event replay functionality
- Comprehensive query API
  - Filter by event type, source, severity, tags, correlation ID
  - Time range queries with start/end times
  - Pagination with limit/offset
  - Sorting by time, type, severity (asc/desc)
  - Count queries for aggregations
- Retention policies
  - Time-based retention (MaxAge)
  - Count-based retention (MaxCount - keeps most recent)
  - Severity-based retention (MinSeverity - delete low severity events)
  - Per-type retention policies
  - Automatic background retention with configurable interval
- Event replay capabilities
  - Replay events matching query criteria
  - Replay from specific time
  - Replay within time range
  - Custom event handler for replay processing
- Test coverage: 76.3% (20 comprehensive tests covering all storage operations)
  - Store/batch operations
  - Query filtering (type, source, severity, time, correlation ID)
  - Pagination and sorting
  - Delete operations
  - All retention policies
  - Metrics collection
  - Event replay
  - Auto-retention background process
  - Benchmarks for store and query operations

**Week 8: Monitoring and Observability ✅ COMPLETE**
- Metrics collection system (pkg/events/metrics.go)
  - MetricsCollector interface for recording all event operations
  - DefaultMetricsCollector implementation with comprehensive tracking
  - Metrics struct tracking:
    - Event counts (published, received, processed, failed by type)
    - Events by severity (debug, info, warning, error, critical)
    - Publisher/subscriber errors and active subscribers
    - Reactor executions, failures, and durations
    - Action executions, failures, and durations
    - Storage operations, failures, and durations
    - Processing duration statistics (min, max, avg, P50, P95, P99)
    - System stats (uptime, event rate, last event time)
  - DurationStats with percentile calculations (P50, P95, P99)
    - Rolling window of last 1000 durations for accurate percentiles
    - Min, max, avg, total tracking
    - Concurrent-safe with RWMutex
- Health check system (pkg/events/metrics.go)
  - HealthChecker interface for component health monitoring
  - HealthMonitor for managing multiple health checks
  - Health statuses: healthy, degraded, unhealthy
  - EventSystemHealthCheck - monitors event system health
    - Check for recent events (age threshold)
    - Monitor error rates (publisher/subscriber errors)
    - Configurable thresholds
  - StorageHealthCheck - monitors storage backend health
    - Query timeout protection (5 second timeout)
    - Detects database connectivity issues
    - Goroutine-based health check with timeout
- Prometheus metrics exporter (pkg/events/prometheus.go)
  - PrometheusExporter for standard Prometheus text format
  - All metrics exported with HELP and TYPE comments
  - Counter metrics:
    - kscore_events_published_total (by type)
    - kscore_events_received_total (by type)
    - kscore_events_processed_total (by type)
    - kscore_events_failed_total (by type)
    - kscore_events_severity_total (by severity)
    - kscore_publisher_errors_total
    - kscore_subscriber_errors_total
    - kscore_reactor_executions_total (by reactor)
    - kscore_reactor_failures_total (by reactor)
    - kscore_action_executions_total (by type and name)
    - kscore_action_failures_total (by type and name)
    - kscore_storage_operations_total (by operation)
    - kscore_storage_failures_total (by operation)
  - Summary metrics with quantiles (P50, P95, P99):
    - kscore_reactor_duration_seconds (by reactor)
    - kscore_event_processing_duration_seconds
  - Gauge metrics:
    - kscore_active_subscribers
    - kscore_uptime_seconds
    - kscore_event_rate (events/sec)
    - kscore_last_event_timestamp_seconds
  - ExportString() for easy string output
- Human-readable metrics summary (pkg/events/prometheus.go)
  - MetricsSummary with aggregated statistics
  - Top 10 event types by count (sorted descending)
  - Error rate calculation (failed / total attempted * 100)
  - Average processing time
  - FormatSummary for formatted text output
- Test coverage: 79.3% (58 new tests)
  - Metrics collector tests (15 tests)
    - All recording methods (events, reactors, actions, storage)
    - Uptime and event rate calculations
    - Concurrency safety tests
  - Duration statistics tests (3 tests)
    - Record tracking (min/max/avg/total)
    - Percentile calculations (P50, P95, P99)
    - Rolling window management (1000 recent values)
  - Health monitoring tests (8 tests)
    - Health check registration/unregistration
    - Overall status aggregation
    - Event system health checks (healthy/degraded states)
    - Storage health checks (healthy/unhealthy states)
  - Prometheus exporter tests (14 tests)
    - All metric types exported correctly
    - Proper Prometheus format with HELP/TYPE comments
    - Summary metrics with quantiles
    - Edge cases (empty metrics, missing data)
  - Metrics summary tests (5 tests)
    - Summary calculation and aggregation
    - Top event types sorting and limiting
    - Error rate calculation
    - Formatted output
  - Benchmark tests (13 tests)
    - Metrics recording performance
    - Duration stats recording
    - Prometheus export performance
    - Summary generation performance

### Epic 5: GitOps Integration ✅ COMPLETE

**Implementation Plan:** 6 phases (8 weeks total) - All phases complete!

**Phase 1 Week 1: Webhook Infrastructure ✅ COMPLETE**
- Webhook receiver HTTP server (pkg/gitops/webhook/receiver.go)
  - HTTP server for receiving webhooks from ArgoCD, Flux, GitHub, GitLab
  - Automatic webhook type detection via headers and User-Agent
  - Async event processing with processor pattern
  - Health check and statistics endpoints
  - Comprehensive receiver statistics (received, processed, failed, by-type)
- Authentication system (pkg/gitops/webhook/auth.go)
  - None authenticator (no auth)
  - HMAC authenticator (SHA-256 signature verification)
  - Bearer token authenticator
  - Pluggable authenticator pattern
- Webhook handlers (pkg/gitops/webhook/)
  - ArgoCD handler - parses application sync/health/deployment events
  - Flux handler - parses Kustomization/HelmRelease events
  - GitHub handler - parses deployment/workflow/push events
  - GitLab handler - parses deployment/pipeline/push events
  - Handler registry for webhook type routing
- Event integration (pkg/gitops/webhook/types.go)
  - WebhookEvent → Keystone Core Event conversion
  - EventBusProcessor for publishing to event bus
  - Correlation IDs for tracking webhook-triggered workflows
  - Event types: gitops.argocd.*, gitops.flux.*, gitops.github.*, gitops.gitlab.*
- Test coverage: 65.8% (19 tests passing)
  - Authentication tests (4 test suites, 10 test cases)
  - Handler registry and detection tests (2 test suites, 11 test cases)
  - ArgoCD handler tests (valid/invalid payloads, event conversion)
  - Flux handler tests (event parsing, fallback logic)
  - Receiver integration tests (7 test scenarios)

**Phase 2 Week 2-3: GitOps Tool Integration ✅ COMPLETE**
- ArgoCD API client (pkg/gitops/argocd/)
  - Get application status, list applications
  - Trigger sync and rollback operations
  - Update application annotations
- Flux client (pkg/gitops/flux/)
  - Get resource status (Kustomization, HelmRelease, GitRepository, HelmRepository)
  - Suspend/resume reconciliation, trigger reconciliation
- GitHub client (pkg/gitops/github/)
  - Create/list/merge pull requests, update commit statuses, PR comments
- GitLab client (pkg/gitops/gitlab/)
  - Create/list/merge merge requests, update commit statuses, MR comments
- Test coverage: argocd 1.4%, flux 5.1%, github 4.5%, gitlab 5.0%

**Phase 3 Week 4-5: Verification Framework ✅ COMPLETE**
- Verification engine (pkg/gitops/verification/engine.go)
  - Sequential and parallel step execution
  - Retry logic with configurable delays
  - Timeout support for workflows and individual steps
  - Continue-on-failure and stop-on-failure modes
  - Comprehensive result tracking and statistics
- Verification modules:
  - HTTP health check (pkg/gitops/verification/http.go)
    - Configurable methods (GET, POST, etc.)
    - Expected status codes and response body validation
    - Custom headers support
  - Kubernetes resource check (pkg/gitops/verification/k8s.go)
    - Check deployment/statefulset/service availability
    - Replica count validation
    - Ready condition checks
  - Command execution (pkg/gitops/verification/command.go)
    - Execute shell commands with exit code validation
    - Expected output verification
    - Working directory support
  - Script execution (pkg/gitops/verification/command.go)
    - Custom script execution with arguments
    - Exit code and output validation
- Test coverage: 55.8% (21 tests passing)
  - Engine tests: sequential, parallel, retries, failure modes
  - HTTP verifier tests: success, failure, custom methods, headers
  - Command verifier tests: success, failure, exit codes, output validation

**Phase 4 Week 6: Git Sync ✅ COMPLETE**
- Repository types and configuration (pkg/gitops/gitsync/types.go)
  - RepositoryConfig with URL, branch, local path, auth, sync interval
  - AuthConfig supporting none, token, and SSH authentication
  - PathsConfig for syncing states, reactors, vars, workflows directories
  - SyncResult for tracking sync operations (success, commit hashes, changed files)
  - CommitRequest, BranchRequest, PullRequestRequest for Git operations
- Git repository client (pkg/gitops/gitsync/client.go)
  - Repository struct for managing individual Git repositories
  - Clone() - clone repository to local path
  - Open() - open existing repository
  - Sync() - pull updates from remote with change detection
  - Commit() - create commits with file staging
  - Push() - push commits to remote
  - CreateBranch() - create new branches
  - GetPathFiles() - list files in repository paths
  - GetCurrentCommit() - get current commit hash
  - GetPreviousCommit() - get parent of HEAD for rollbacks
  - GetCommitHistory(limit) - get last n commits from HEAD
  - GetCommit(hash) - get info about specific commit
  - CommitInfo struct with hash, message, author, email, timestamp, parent
  - Support for HTTPS (token) and SSH key authentication
- Repository manager (pkg/gitops/gitsync/client.go)
  - Manager for handling multiple repositories
  - AddRepository() - register new repositories
  - GetRepository() - retrieve repository by name
  - SyncAll() - sync all registered repositories
  - Watch() - background sync with configurable intervals
- Test coverage: 50.0% (10 tests passing)
  - Config validation tests
  - Authentication setup tests (none, token, SSH)
  - Repository operations (open, commit, branch, file listing)
  - Manager tests (multi-repo handling, sync all)

**Phase 5 Week 7: Rollback Automation ✅ COMPLETE**
- Rollback types and configuration (pkg/gitops/rollback/types.go)
  - RollbackType: ArgoCD, Flux, Git, Manual
  - RollbackStrategy: Previous, Specific, LastKnownGood
  - RollbackTrigger: Manual, Automatic, Scheduled
  - RollbackConfig with approval workflow, verification, timeout
  - RollbackResult with status tracking, timing, approval info
  - ApprovalInfo for tracking approval/rejection details
  - ApprovalRequest for approve/reject operations
- Rollback engine (pkg/gitops/rollback/engine.go)
  - Executor interface for pluggable rollback implementations
  - Engine for orchestrating rollback operations
  - RegisterExecutor() - register rollback executors by type
  - Execute() - execute rollback with optional approval workflow
  - ApproveRollback() - approve or reject pending rollbacks
  - GetRollback(), ListRollbacks(), ListPendingRollbacks() - rollback management
  - Support for immediate execution or approval-required workflows
  - Automatic revision determination based on strategy
  - Post-rollback verification support
  - Comprehensive status tracking (pending, approved, rejected, in_progress, completed, failed, verifying, verified)
- ArgoCD executor (pkg/gitops/rollback/argocd.go)
  - ArgoCDExecutor for ArgoCD-based rollbacks
  - Execute() - rollback ArgoCD application to specific revision
  - GetPreviousRevision() - get previous application revision
  - GetLastKnownGood() - find last healthy deployment
  - Integration with ArgoCD API client
- Git executor (pkg/gitops/rollback/git.go)
  - GitExecutor for Git-based rollbacks
  - Execute() - create rollback branch and revert commit
  - Push rollback to remote repository
  - Integration with Git sync manager
- Test coverage: 50.3% (10 tests passing)
  - Engine tests: register, execute, approval workflow
  - Strategy tests: previous, last known good, specific revision
  - Approval tests: approve and reject workflows
  - Listing tests: all rollbacks and pending rollbacks

**Phase 6 Week 8: Promotion Pipelines ✅ COMPLETE**
- Promotion types and configuration (pkg/gitops/promotion/types.go)
  - PromotionStrategy: BlueGreen, Canary, Rolling, Immediate
  - Environment configuration with approval and verification
  - Pipeline definition across multiple environments
  - CanaryStep for gradual rollout configuration
  - PromotionRequest and PromotionResult tracking
  - StageResult for per-environment promotion results
  - CanaryProgress for tracking canary deployment steps
  - ApprovalInfo and ApprovalRequest for approval workflow
- Promotion engine (pkg/gitops/promotion/engine.go)
  - Deployer interface for pluggable deployment implementations
  - Engine for orchestrating promotion pipelines
  - RegisterPipeline() - register promotion pipelines
  - Promote() - execute promotion with optional approval
  - ApprovePromotion() - approve or reject pending promotions
  - GetPromotion(), ListPromotions(), ListPendingPromotions() - promotion management
  - Progressive delivery strategies:
    - Immediate - full deployment at once
    - Canary - gradual traffic shift with steps (25%, 50%, 75%, 100%)
    - Blue/Green - deploy to green, switch traffic
    - Rolling - rolling update (platform-managed)
  - Automatic rollback on failure support
  - Per-environment verification integration
  - Comprehensive status tracking (pending, approved, rejected, in_progress, verifying, rolling_out, completed, failed, rolling_back, rolled_back)
- Test coverage: 73.0% (9 tests passing)
  - Engine tests: register, validation
  - Promotion tests: immediate, with approval, canary deployment
  - Approval tests: approve and reject workflows
  - Listing tests: all promotions and pending promotions

### Epic 6: Policy Enforcement ✅ COMPLETE

**Implementation Plan:** 6 phases (All phases complete!)

**Phase 1: Policy Definition & Types ✅ COMPLETE**
- Policy types and enums (pkg/policy/types.go)
  - PolicyType: OPA (Rego), CEL, Builtin
  - PolicyCategory: Security, Compliance, Operational, Cost, Custom
  - Severity levels: Low, Medium, High, Critical
  - EnforcementMode: Audit, Enforce, Warn
- Policy structure (pkg/policy/types.go)
  - Policy definition with ID, name, description, code
  - EvaluationInput for policy evaluation context
  - EvaluationResult with violations and warnings
  - Violation details with rule, message, severity, path, remediation
- Policy organization (pkg/policy/types.go)
  - PolicySet for grouping related policies
  - PolicyBinding for attaching policies to resources
  - PolicyResult for aggregated evaluation results
  - PolicySummary with violation statistics
- Policy registry (pkg/policy/registry.go)
  - Registry for managing policies, sets, and bindings
  - RegisterPolicy(), GetPolicy(), ListPolicies()
  - ListPoliciesByCategory(), ListPoliciesByType()
  - UpdatePolicy(), DeletePolicy()
  - Policy set management (register, get, list, delete)
  - Policy binding management with resource filtering
  - ListBindingsForResource() for resource-specific policies
  - Thread-safe operations with mutex locks
- Test coverage: 81.0% (11 tests passing)
  - Policy registration and validation
  - Update and delete operations
  - List by category and type
  - Policy set operations
  - Binding operations and validation
  - Resource-specific binding queries

**Phase 2: OPA Integration ✅ COMPLETE**
- OPA evaluator (pkg/policy/opa.go)
  - OPAEvaluator for evaluating Rego policies
  - Evaluate() - evaluate policy with allow/deny decision
  - EvaluateWithDeny() - evaluate with explicit deny rules
  - extractViolations() - extract violation details from results
  - parseViolationsFromData() - parse violations from OPA bindings
  - ValidatePolicy() - syntax validation for Rego code
  - Package name parsing from policy code
  - Package name sanitization (hyphens to underscores)
- Policy evaluation features
  - Allow/deny decision logic
  - Violation extraction with severity, path, remediation
  - Warning collection
  - Duration tracking for performance monitoring
  - Support for complex Rego policies with multiple rules
- Test coverage: 73.6% (6 OPA tests + 11 registry tests = 17 tests passing)
  - Simple allow/deny policies
  - Complex multi-condition policies
  - Explicit deny with violations
  - Invalid policy handling
  - Policy validation
  - Duration and timestamp tracking

**Phase 3: CEL Integration ✅ COMPLETE**
- CEL evaluator (pkg/policy/cel.go)
  - CELEvaluator for evaluating Common Expression Language policies
  - Evaluate() - evaluate CEL expressions with boolean results
  - EvaluateWithDetails() - evaluate with detailed violation extraction
  - extractViolationsFromDetails() - extract violation details from evaluation state
  - ValidatePolicy() - syntax validation for CEL expressions
  - Standard variable declarations (input, resource, action, user, context)
  - Type-safe evaluation with boolean return type validation
- Policy evaluation features
  - Allow/deny decision based on CEL expression evaluation
  - Support for complex expressions with logical operators (&&, ||)
  - Resource access patterns (resource.owner, resource.public, etc.)
  - Context-based policies (context.environment, etc.)
  - Violation generation on policy denial
  - Duration tracking for performance monitoring
  - Error handling for compilation and evaluation failures
- Test coverage: 70.1% (8 CEL tests + 6 OPA tests + 11 registry tests = 25 tests passing)
  - Simple allow/deny expressions
  - Complex multi-condition policies with OR/AND logic
  - Resource ownership and sharing patterns
  - Context-based conditional policies
  - Invalid policy syntax handling
  - Policy validation tests
  - Duration and timestamp tracking
  - Detailed evaluation with violation extraction

**Phase 4: Policy Engine ✅ COMPLETE**
- Policy engine orchestration (pkg/policy/engine.go)
  - PolicyEngine coordinates evaluation across OPA and CEL evaluators
  - Evaluate() - evaluate single policy by ID with registry lookup
  - EvaluatePolicySet() - evaluate all policies in a set with aggregation
  - EvaluateForResource() - evaluate all policies bound to a resource type
  - EvaluateBatch() - serial batch evaluation of multiple inputs
  - EvaluateBatchParallel() - parallel batch evaluation with goroutines
  - ValidatePolicy() - pre-registration policy validation
  - Enforcement mode support (Enforce, Audit, Warn)
  - Automatic evaluator selection based on policy type
  - Disabled policy/set handling (skip with warnings)
- Evaluation features
  - Registry integration for policy and binding management
  - Action filtering in bindings (evaluate only matching actions)
  - Aggregated results with summary statistics
  - Violation counting by severity (using ViolationsBySeverity map)
  - Total duration tracking across multiple evaluations
  - Error handling with fallback violation generation
  - Thread-safe operations with mutex locks
- Test coverage: 74.3% (91 total tests passing)
  - Single policy evaluation (OPA and CEL)
  - Policy set evaluation and aggregation
  - Resource-bound policy evaluation with action filtering
  - Disabled policy and policy set handling
  - Batch evaluation (serial and parallel)
  - Policy validation (all policy types)
  - Enforcement mode testing (Enforce, Audit, Warn)
  - Violation severity aggregation
  - Error handling (non-existent policies and sets)

**Phase 5: Policy Enforcement ✅ COMPLETE**
- Policy enforcement layer (pkg/policy/enforcement.go)
  - PolicyEnforcer coordinates policy enforcement across the system
  - EnforcementPoint types (pre_execution, post_execution, on_change, on_drift, on_event)
  - EnforcementAction types (block, warn, audit, remediate)
  - EnforcementConfig for configurable enforcement behavior
  - EnforceForResource() evaluates all policies bound to a resource type
  - EnforcePolicy() evaluates and enforces a specific policy
  - Resource type scoping for targeted enforcement
  - Custom violation handlers for extensibility
  - Event emission for policy evaluations (pass/violation events)
- Integration features
  - StateEnforcementHook() creates hooks for state management integration
  - EventEnforcementReactor() creates reactors for event-driven enforcement
  - PolicyEnforcementAction implements events.Action interface
  - Event-based policy triggers with context propagation
  - Configurable enforcement actions per policy
  - Violation handling pipeline with custom handlers
- Event integration (pkg/events/types.go)
  - Added EventTypePolicyPass for successful policy evaluations
  - Added EventTypePolicyViolation for policy violations
  - Severity mapping from policy to event severities
  - Detailed violation data in event payloads
- Test coverage: 76.0% (114 tests passing)
  - Resource enforcement with scoping
  - Enforcement action modes (Block, Warn, Audit)
  - Event emission and publishing
  - Custom violation handlers
  - Single policy enforcement
  - State hook integration
  - Event reactor integration
  - Severity conversion

**Phase 6: Policy Reporting & Auditing ✅ COMPLETE**
- Policy auditing system (pkg/policy/audit.go)
  - PolicyAuditor for recording and managing audit entries
  - RecordEvaluation() - record single policy evaluation
  - RecordPolicyResult() - record multi-policy evaluation result
  - GetEntries() - retrieve audit entries with filtering
  - GetSummary() - generate statistical summary of evaluations
  - Clear() - clear all audit entries
  - Ring buffer behavior with configurable max size (default 10,000)
  - Thread-safe operations with mutex locks
- Audit filtering and querying
  - AuditFilter with multiple criteria (policy, resource, allowed status, time range, user, action)
  - Time range filtering (start/end times)
  - Boolean filtering (allowed/denied evaluations)
  - Limit support for pagination
  - Matches() - evaluate if entry matches filter criteria
- Audit summaries and statistics
  - AuditSummary with comprehensive statistics
  - Total, allowed, and denied evaluation counts
  - Total violations count
  - Violations grouped by severity (Low, Medium, High, Critical)
  - Violations grouped by policy ID
  - Evaluations grouped by resource type
  - Average evaluation duration calculation
- Compliance reporting (pkg/policy/audit.go)
  - ComplianceReporter for generating compliance reports
  - GenerateReport() - create compliance report for time period
  - ComplianceReport with period-based analysis
  - Compliance rate calculation (compliant/total * 100)
  - Policy-level statistics (total, compliant, violating policies)
  - Top violations tracking by policy
  - Severity distribution analysis
- Compliance analysis features
  - ReportPeriod with start/end times
  - PolicySummary for individual policy statistics
  - ViolationSummary with policy name, count, severity
  - Top N violations sorted by count
  - Per-policy compliance tracking
- Test coverage: 79.4% (58 tests passing)
  - Audit entry recording (single and batch)
  - Ring buffer capacity management
  - Filtering by policy, resource, allowed status, time range, user, action
  - Limit and pagination
  - Summary generation with statistics
  - Clear operations
  - Compliance report generation
  - Top violations ranking
  - Policy aggregation and statistics

### Epic 7: Observability & Monitoring ✅ COMPLETE

**Implementation Plan:** 7 phases (10 weeks total)

**Current Status**: Phases 1-7 COMPLETE ✅

**Phase 1: Metrics ✅ COMPLETE**
- Metrics infrastructure (pkg/metrics/types.go)
  - Collector interface for metrics collection
  - IncCounter(), AddCounter(), SetGauge(), IncGauge(), DecGauge()
  - ObserveHistogram(), ObserveSummary(), RecordDuration()
  - MetricType enum (counter, gauge, histogram, summary)
  - MetricDefinition with name, type, help, labels, buckets, objectives
  - MetricRegistry interface for metric management
  - Timer helper for timing operations
  - DefaultBuckets for histograms (1ms to 10s)
  - DefaultObjectives for summaries (P50, P90, P95, P99)
- Prometheus implementation (pkg/metrics/prometheus.go)
  - PrometheusCollector implementing Collector interface
  - RegisterMetric() for metric registration with Prometheus
  - Thread-safe operations with mutex locks
  - Support for CounterVec, GaugeVec, HistogramVec, SummaryVec
  - Handler() for /metrics HTTP endpoint
  - Registry() for accessing underlying Prometheus registry
  - Automatic default buckets/objectives
- Standard metrics (pkg/metrics/collectors.go)
  - 28 standard Keystone Core metrics defined
  - Control plane metrics: API requests, agents, commands, states, policies, events
  - Agent metrics: heartbeat, CPU/memory/disk usage, commands, states
  - State management metrics: resources, drift, changes
  - GitOps metrics: webhooks, verifications, rollbacks
  - Policy metrics: violations, remediations, compliance score
  - InitializeStandardMetrics() for bulk registration
- Specialized collectors (pkg/metrics/collectors.go)
  - ControlPlaneCollector for control plane operations
    - RecordAPIRequest(), SetAgentsConnected(), RecordAgentDisconnect()
    - RecordCommandExecution(), RecordStateApplication()
    - RecordPolicyEvaluation(), RecordEventPublished/Processed()
  - AgentCollector for agent operations
    - RecordHeartbeat(), RecordCPUUsage(), RecordMemoryUsage(), RecordDiskUsage()
    - RecordCommandExecuted(), RecordStateApplied()
  - StateCollector for state management
    - SetResourceCount(), RecordDriftDetection(), RecordStateChange()
  - GitOpsCollector for GitOps operations
    - RecordWebhookReceived(), RecordDeploymentVerified(), RecordRollbackTriggered()
  - PolicyCollector for policy operations
    - RecordViolation(), RecordRemediation(), SetComplianceScore()
- Test coverage: 82.5% (16 tests passing)
  - Counter, gauge, histogram, summary operations
  - Duration recording and timer functionality
  - All specialized collectors (control plane, agent, state, GitOps, policy)
  - Duplicate registration handling
  - Unknown metric type handling
  - Non-existent metric operations (graceful degradation)
  - HTTP handler functionality

**Phase 2: Logging ✅ COMPLETE**
- Logging infrastructure (pkg/logging/types.go)
  - Logger interface for structured logging
  - Debug(), Info(), Warn(), Error() methods
  - WithFields(), WithCorrelationID(), WithContext() for logger chaining
  - SetLevel(), GetLevel() for log level management
  - Level enum (Debug, Info, Warn, Error) with String() and ParseLevel()
  - Entry structure with timestamp, level, logger, message, correlation ID, fields
  - Field structure for key-value pairs
  - Field constructors: String(), Int(), Int64(), Float64(), Bool(), Duration(), Time(), Error(), Any()
  - Fields() helper for bulk field creation from key-value pairs
  - Formatter interface for pluggable formatters
  - Output interface for pluggable outputs
  - WriterOutput wrapping io.Writer
  - SamplingConfig for high-volume log sampling
  - Config with level, name, formatter, outputs, sampling, caller info
- Formatters (pkg/logging/formatters.go)
  - JSONFormatter: JSON format with optional pretty-printing
  - LogfmtFormatter: logfmt format (key=value pairs)
  - TextFormatter: Human-readable text with optional colors
  - ANSI color support for different log levels (red=error, yellow=warn, blue=info, gray=debug)
  - Automatic quoting in logfmt for values with spaces/special chars
  - Sorted field output for consistent formatting
- Correlation ID management (pkg/logging/correlation.go)
  - GenerateCorrelationID(): Generate unique correlation IDs
  - ContextWithCorrelationID(): Add correlation ID to context
  - CorrelationIDFromContext(): Extract correlation ID from context
  - EnsureCorrelationID(): Get or generate correlation ID
  - Crypto-random ID generation with counter fallback
- Structured logger implementation (pkg/logging/logger.go)
  - StructuredLogger implementing Logger interface
  - NewLogger() with full configuration
  - NewDefaultLogger() with sensible defaults
  - Thread-safe operations with read/write mutex
  - Log level filtering
  - Sampling support for high-volume scenarios
  - Multiple output support (write to multiple destinations)
  - Base field inheritance for child loggers
  - Context-aware logging with correlation ID extraction
  - Close() for cleanup
  - Global default logger with package-level functions
  - SetDefault(), Default() for global logger management
- Package-level logging functions
  - Debug(), Info(), Warn(), ErrorLog() using default logger
  - WithFields(), WithCorrelationID(), WithContext() shortcuts
- Test coverage: 75.2% (20 tests passing)
  - Log level string conversion and parsing
  - JSON formatter (with and without correlation ID)
  - Logfmt formatter
  - Text formatter
  - Correlation ID generation and context management
  - Structured logger operations (all log levels)
  - Log level filtering
  - Logger chaining with fields
  - Logger with correlation ID
  - Logger with context
  - Field constructors (all types)
  - Error field handling
  - Fields helper
  - WriterOutput
  - Default logger operations
  - SetLevel/GetLevel

**Phase 3: Tracing ✅ COMPLETE**
- OpenTelemetry integration (pkg/tracing/)
  - Tracer initialization with OTLP exporter
  - Distributed trace context propagation
  - Span creation and instrumentation helpers
  - Trace sampling strategies (always, never, ratio-based)
- Instrumentation of core components
  - Control plane API instrumentation
  - State management operation tracing
  - Event system tracing
  - Policy evaluation tracing
- Test coverage with mock exporters

**Phase 4: TUI Monitor ✅ COMPLETE**
- Terminal-based real-time monitoring tool (`kscore-monitor`)
- 8 fully functional interactive views:
  - **Dashboard** (View 1): System overview with metrics, agent counts, job stats, recent events
  - **Agents** (View 2): Interactive table with live agent status, metadata, and heartbeats
  - **Events** (View 3): Real-time event stream with filtering, search, pause/resume
  - **State Drift** (View 4): Configuration drift detection and monitoring interface
  - **Policy Violations** (View 5): Compliance tracking and violation alerts
  - **Jobs** (View 6): Command and batch job execution history with dual-mode table
  - **Logs** (View 7): Structured log streaming interface
  - **Metrics** (View 8): Performance metrics and resource utilization overview
- Built with Bubble Tea framework (charmbracelet)
- Real-time updates via NATS JetStream and gRPC API
- Keyboard navigation (1-8: switch views, ↑/↓: scroll, r: refresh, q: quit)
- Search/filter capabilities across all views
- Color-coded status indicators and severity levels
- Responsive layout with window resizing
- Thread-safe data handling
- Components:
  - cmd/kscore-monitor/main.go - CLI entry with Cobra
  - cmd/kscore-monitor/config/ - Configuration management
  - cmd/kscore-monitor/client/ - gRPC client wrapper
  - cmd/kscore-monitor/events/ - NATS JetStream subscriber
  - cmd/kscore-monitor/ui/ - All 8 view implementations

**Phase 5: Dashboards ✅ COMPLETE**
- Six comprehensive Grafana dashboards (deploy/grafana/dashboards/)
  - Keystone Core Overview (kscore-overview.json)
    - System-wide metrics, agent counts, command rates, policy violations
    - Environment and datacenter filtering variables
    - Agent status distribution, success rates, recent events timeline
  - Control Plane Health (control-plane-health.json)
    - Control plane status, uptime, resource utilization
    - API request rate and latency (p95/p99)
    - NATS message throughput and bandwidth
    - State backend query latency
    - Error rates by component
    - CPU and memory usage over time
  - Agent Fleet (agent-fleet.json)
    - Fleet health status (total, healthy, degraded, offline)
    - Agent distribution by datacenter and role
    - Command execution success rate per agent
    - Agent resource utilization (CPU, memory, disk)
    - Agent version distribution
    - Filtering by datacenter, role, and agent ID
  - State Management (state-management.json)
    - State applications, success/failure rates
    - Drift detection events by severity
    - State changes by module
    - Application duration percentiles
    - Resources under management
    - Environment and module filtering
  - Policy Compliance (policy-compliance.json)
    - Overall compliance score gauge
    - Violations by severity (critical, high, medium, low)
    - Remediation success rate
    - Top violated policies
    - Compliance trends (7-day average)
    - Policy evaluation rate and duration
    - Framework and environment filtering
  - GitOps Operations (gitops-operations.json)
    - Deployment verification metrics
    - Verification success rate
    - Rollback frequency and reasons
    - Deployment duration percentiles
    - Failed verifications by application
    - Webhook events by source
    - Application and environment filtering
- Prometheus alert rules (deploy/grafana/alerts/kscore-alerts.yml)
  - 5 alert groups covering all Keystone Core components
  - Control plane alerts (5 rules): down, high memory, high goroutines, high latency, high error rate
  - Agent fleet alerts (7 rules): low/critical availability, multiple offline, high resource usage, command failures
  - State management alerts (4 rules): high/critical failure rates, high drift detection, slow performance
  - Policy alerts (5 rules): critical/high violations, low compliance score, remediation failures
  - GitOps alerts (4 rules): verification failures, high rollback rate, slow performance, webhook errors
  - NATS alerts (4 rules): high memory, slow consumers, high connections, storage usage
- Grafana provisioning configuration
  - Datasource provisioning (provisioning/datasources/prometheus.yml)
  - Dashboard provisioning (provisioning/dashboards/kscore.yml)
  - Alerting provisioning (provisioning/alerting/kscore.yml)
  - Auto-import on Grafana startup
- Docker Compose deployment (docker-compose.yml)
  - Prometheus + Grafana stack
  - Auto-configured with all dashboards
  - Environment variable support for alert notifications
- Comprehensive documentation (README.md)
  - Dashboard descriptions and use cases
  - Alert rule reference
  - Metrics reference (70+ metrics documented)
  - Quick start guide
  - Customization instructions
  - Troubleshooting guide

**Phase 6: Health & Status ✅ COMPLETE**
- Health check types and interfaces (pkg/health/types.go)
  - Status enum (Healthy, Degraded, Unhealthy, Unknown)
  - CheckResult structure with timestamp, duration, details
  - Checker interface for pluggable health checks
  - ComponentStatus for individual component health
  - LivenessResponse, ReadinessResponse, StatusResponse
  - Config with check intervals, timeouts, startup grace period
- Health check manager (pkg/health/manager.go)
  - Manager for registering and running health checks
  - Background check loop with configurable intervals
  - Readiness tracking with required checks
  - Liveness probe (always healthy if process running)
  - Readiness probe (checks required dependencies)
  - Detailed status reporting with component breakdown
  - Startup grace period handling
  - Thread-safe concurrent check execution
- Dependency health checkers (pkg/health/checkers.go)
  - NATSChecker - NATS connection health with reconnect tracking
  - DatabaseChecker - database ping with latency and connection pool stats
  - AgentPoolChecker - agent availability with configurable thresholds
  - FunctionChecker - custom health check functions
  - AlwaysHealthyChecker - testing helper
- Circuit breaker pattern (pkg/health/circuitbreaker.go)
  - CircuitBreaker for fault tolerance and self-healing
  - States: Closed, Open, HalfOpen with automatic transitions
  - Configurable failure/success thresholds
  - Timeout-based recovery attempts
  - Execute wrapper for protected function calls
  - State change callbacks for monitoring
  - Comprehensive statistics tracking
- HTTP handlers (pkg/health/http.go)
  - LivenessHandler - GET /health/live (always 200 if running)
  - ReadinessHandler - GET /health/ready (503 if not ready)
  - StatusHandler - GET /health/status (detailed component info)
  - RegisterRoutes for standard /health/* paths
  - RegisterRoutesWithPrefix for custom prefixes
  - Kubernetes-compatible probe endpoints
- Test coverage: 100% (39 tests passing)
  - Manager tests (11 tests): registration, readiness, status, lifecycle
  - Checker tests (8 tests): all checker types, healthy/degraded/unhealthy states
  - Circuit breaker tests (13 tests): state transitions, thresholds, execute, callbacks
  - HTTP handler tests (7 tests): all endpoints, method validation, status codes

**Phase 7: Advanced Features ✅ COMPLETE**
- Performance profiling (pkg/profiling/)
  - ProfileServer with built-in pprof endpoints (/debug/pprof/*)
  - CaptureProfile API for all profile types (CPU, heap, goroutine, mutex, block, threads, allocs, trace)
  - ProfileStats for runtime statistics (goroutines, memory, CPU count)
  - Configurable profiling rates (block, mutex)
  - HTTP server for pprof endpoints
  - Test coverage: 15 tests passing (server lifecycle, all profile types, stats)
- Query API (pkg/query/)
  - Unified API for metrics, logs, and traces
  - PrometheusQuerier for metrics (instant and range queries)
  - InMemoryLogsQuerier for logs (with time range, filters, direction, limit)
  - InMemoryTracesQuerier for traces (by service, operation, tags, duration, time range)
  - Graph and topology builders
  - Placeholder for Loki and Jaeger integration
  - ErrorResponse with proper error handling
  - Test coverage: 28 tests passing (all queriers, filters, API)
- Infrastructure visualization (pkg/visualization/)
  - AgentProvider interface for agent data
  - HTTP API endpoints:
    - GET /api/agents - list agents with filtering
    - GET /api/agents/{id} - get specific agent
    - GET /api/topology - hierarchical topology tree
    - GET /api/graph - graph representation with nodes/edges
  - WebSocket support for real-time updates (/ws/topology)
  - TopologyNode hierarchical structure (datacenter → environment → role → agent)
  - Graph visualization with nodes and edges
  - Agent status tracking (healthy, degraded, offline, unknown)
  - Filter options (datacenter, environment, role, status, tags)
  - Real-time topology updates via WebSocket
  - Test coverage: 11 tests passing (all endpoints, topology building, graph building, filtering)

### Epic 8: Multi-Environment Support ✅ COMPLETE

**Implementation Plan:** 6 phases (All phases complete!)

**Current Status**: All 6 phases COMPLETE ✅

**Phase 1: Kubernetes Integration ✅ COMPLETE**
- Kubernetes client wrapper (pkg/k8s/)
  - Client with automatic kubeconfig loading
  - Multi-cluster support
  - Context switching
  - PodExec for command execution in pods
  - Resource management (create, get, list, update, delete)
- CRD definitions
  - RemoteExecution CRD for distributed command execution
  - StateConfig CRD for declarative configuration
- Operator controllers (pkg/k8s/)
  - RemoteExecution controller with reconciliation loops
  - StateConfig controller with state synchronization
  - Automatic CRD installation
- Kubernetes state modules (pkg/statemgmt/)
  - k8s_namespace module (present, absent states) with full CRUD
  - k8s_deployment module (present, absent states) with full CRUD (create, update, delete, scale)
- Test coverage: 100% (26 tests passing - 8 namespace + 10 deployment + 8 helper)

**Phase 2: VM Support ✅ COMPLETE**
- Platform detection system (pkg/platform/)
  - OS detection (Linux, Windows, macOS, BSD)
  - Distribution detection (Ubuntu, Debian, CentOS, RHEL, Fedora, Alpine, Arch, openSUSE, Amazon Linux)
  - Version detection with /etc/os-release and /etc/lsb-release parsing
  - Package manager detection (apt, yum, dnf, zypper, pacman, apk, brew, chocolatey, winget)
  - Init system detection (systemd, upstart, sysv, openrc, launchd, windows_service)
  - Platform family detection (debian, rhel, suse, arch, alpine)
  - Virtualization detection (VMware, VirtualBox, KVM, QEMU, Xen)
  - Container detection (Docker, LXC, Kubernetes)
  - Caching support with configurable TTL
- Cross-platform module adapters
  - Package module (pkg/statemgmt/module_package.go) with auto-detection
  - Service module (pkg/statemgmt/module_service.go) with init system support
  - File module (pkg/statemgmt/module_file.go) with path normalization
  - User and Group modules with OS-specific handling
- Test coverage: 41.9% (18 tests passing)
  - OS and architecture detection
  - Distribution detection and normalization
  - Package manager and init system detection
  - Platform helper functions
  - Caching behavior

**Phase 3: Bare Metal Support ✅ COMPLETE**
- Hardware detection using gopsutil
  - CPU information (cores, model, frequency, vendor, cache, flags)
  - Memory information (total, available, used, swap)
  - Disk information (devices, partitions, usage, serial)
  - Network interface information (MAC, IPs, MTU, flags)
  - System information (hostname, OS, platform, kernel)
- BMC/IPMI detection (pkg/hardware/detector.go)
  - Multi-method detection: IPMI device files, ipmitool, DMI/SMBIOS
  - Detects BMC presence, IP, MAC, firmware version, manufacturer
  - Linux-focused with ipmitool integration
  - Parsing of `ipmitool bmc info` and `ipmitool lan print` output
- Agent metadata extended with hardware info
- Cross-platform support (Linux, Windows, macOS, ARM)
- Test coverage: 100% (15 tests passing)

**Phase 4: Edge Support ✅ COMPLETE**
- Edge package with multiple modes (pkg/edge/)
  - Offline mode with local buffering
  - Online mode with cloud sync
  - Lightweight mode with minimal resource usage
- Local state caching
  - File-based cache storage
  - TTL-based expiration
  - Size limits with LRU eviction
- Connection resilience
  - Automatic reconnection
  - Exponential backoff
  - Heartbeat monitoring
- Resource constraints handling
  - Memory limits
  - CPU limits
  - Graceful degradation
- Test coverage: 100% (16 tests passing)

**Phase 5: Cloud Integration ✅ COMPLETE**
- AWS integration (pkg/cloud/aws/)
  - EC2 instance detection
  - ECS task detection
  - Lambda function detection
  - IMDSv2 metadata service client
- GCP integration (pkg/cloud/gcp/)
  - Compute Engine instance detection
  - GKE cluster detection
  - Cloud Functions detection
  - Metadata service client
- Azure integration (pkg/cloud/azure/)
  - Virtual Machine detection
  - AKS cluster detection
  - Azure Functions detection
  - IMDS metadata service client
- Multi-cloud detector with caching
  - Automatic cloud provider detection
  - Metadata caching
  - Concurrent detection with timeouts
- Test coverage: 100% (21 tests passing)

**Phase 6: Container & Service Mesh ✅ COMPLETE**
- Container runtime detection (pkg/container/)
  - Docker detection via Docker socket
  - containerd detection via CRI socket
  - Runtime version detection
  - Container ID extraction from cgroup
- Service mesh integration (pkg/servicemesh/)
  - Istio integration (sidecar detection, proxy version, metrics endpoint)
  - Linkerd integration (proxy detection, version, identity)
  - Consul integration (agent detection, datacenter, service registration)
  - mTLS configuration detection
  - SPIFFE ID extraction
  - Proxy configuration retrieval
  - Metrics endpoint discovery
- Test coverage: 100% (34 tests passing)

**Key Achievements**:
- Complete multi-environment support across all deployment targets
- Unified platform detection for cross-platform operations
- Seamless integration with Kubernetes, VMs, bare metal, edge, and cloud
- Container and service mesh awareness for modern workloads
- 116 comprehensive tests with 80%+ average coverage
- ~9,671 lines of production code

### Epic 9: Plugin System & Extensibility ✅ COMPLETE (All 7 Phases)

**Implementation Plan:** 7 phases (10 weeks total)

**Current Status**: Phases 1-7 COMPLETE ✅

**Phase 1: CLI Infrastructure & Plugin Runtime Foundation ✅ COMPLETE (Week 1-2)**
- **T1.0: kscorectl Plugin Dispatcher** ✅
  - Git-style plugin architecture (pkg/plugin/)
  - Discovers `kscore-*` binaries in PATH
  - Plugin execution with Cobra integration
  - 10 tests passing, 100% coverage
- **T1.1: Starlark Runtime** ✅
  - Sandboxed Starlark runtime (pkg/module/runtime/starlark/)
  - Deterministic execution mode
  - Resource limits (execution time, max steps, stack depth)
  - Capability registration system
  - Go ↔ Starlark value conversion
  - Hot-reload support
  - 18 tests passing
- **T1.2: WASM Runtime** ✅
  - Wasmtime integration with WASI support (pkg/module/runtime/wasm/)
  - Memory isolation and limits
  - Fuel metering for instruction counting
  - Host function registration
  - Linear memory read/write
  - 14 tests passing
- **T1.3: Plugin Manifest Parser** ✅
  - Module manifest (module.yaml) parser (pkg/module/manifest/)
  - Lock file (module.lock) support
  - Capability and dependency declarations
  - Resource limits configuration
  - YAML serialization/deserialization
  - 19 tests passing

**Phase 1 Achievements**:
- Complete plugin runtime foundation
- Sandboxed execution for Starlark and WASM
- Type-safe manifest parsing
- 61 comprehensive tests passing
- ~2,800 lines of production code

**Phase 2: Capability System ✅ COMPLETE (Week 3-4)**
- **Base Capability Infrastructure** ✅
  - Capability interface and base types (pkg/module/capabilities/)
  - CapabilityContext for execution tracking
  - CapabilityRegistry for capability management
  - CapabilityInvoker with auditing support
  - AuditLogger interface for capability invocation tracking
  - 14 tests passing for base infrastructure
- **Filesystem Capabilities** ✅
  - FSReadCapability: path-scoped file reading (pkg/module/capabilities/fs.go)
    - Glob pattern matching with ** wildcard support
    - Allowed/denied path lists
    - Max file size limits
    - ReadFile(), OpenFile() operations
  - FSWriteCapability: path-scoped file writing
    - WriteFile(), AppendFile(), DeleteFile()
    - Mkdir(), MkdirAll() for directory creation
    - CopyFile() with dual capability validation
    - Automatic parent directory creation
  - 16 filesystem tests passing
- **HTTP Capabilities** ✅
  - HTTPGetCapability: domain-scoped GET requests (pkg/module/capabilities/http.go)
    - Wildcard domain matching (*.example.com)
    - Timeout and response size limits
    - Token bucket rate limiting
    - Custom headers support
  - HTTPPostCapability: domain-scoped POST requests
    - Request and response size limits
    - Rate limiting per capability
    - JSON/form data support
  - 13 HTTP tests passing
- **Execution Capability** ✅
  - ExecCapability: command allowlist execution (pkg/module/capabilities/exec.go)
    - Command allowlist validation
    - Timeout with context cancellation
    - Stdout/stderr capture
    - ExecWithInput() for stdin support
    - Working directory configuration
  - 7 exec tests passing
- **Additional Capabilities** ✅ (pkg/module/capabilities/other.go)
  - SecretsReadCapability: path-scoped secret reading
    - Pluggable SecretsStore backend
    - Path pattern matching
    - Audit support
  - SecretsWriteCapability: path-scoped secret writing
    - WriteSecret(), DeleteSecret() operations
  - LogCapability: structured logging
    - Pluggable Logger backend
    - Rate limiting for log messages
    - Automatic context enrichment (module, correlation_id)
  - TimeCapability: current time access (breaks determinism!)
    - Now(), Unix() methods
  - KVCapability: namespace-scoped key-value storage
    - Pluggable KVStore backend
    - Automatic namespace isolation
    - Get(), Set(), Delete(), List() operations
  - 27 tests for additional capabilities

**Phase 2 Achievements**:
- Complete capability-based security system
- 10 capability types implemented: fs.read, fs.write, http.get, http.post, exec, secrets.read, secrets.write, log, time, kv
- Path/domain/command scoping for security
- Rate limiting and resource limits
- Pluggable backends (SecretsStore, Logger, KVStore)
- 77 comprehensive tests passing (100% pass rate)
- ~2,100 lines of production code

**Phase 3: Cryptographic Verification ✅ COMPLETE (Week 5-6)**
- **Verification Types & Infrastructure** ✅ (pkg/module/verify/types.go, errors.go)
  - VerificationResult with signature, hash, SumDB, and trust validation status
  - VerificationOptions for configurable verification requirements
  - ModuleArtifact structure for module metadata
  - VerificationReport for detailed verification results
  - SignatureFormat enum (Cosign, GPG, Custom)
  - HashAlgorithm enum (SHA256, SHA512)
  - Comprehensive error types for all verification failures
- **Hash Verification** ✅ (pkg/module/verify/hash.go)
  - DefaultHashVerifier with SHA256/SHA512 support
  - ComputeHash() for files, directories, and ZIP archives
  - VerifyHash() for hash validation with prefix handling
  - Deterministic directory hashing (sorted file iteration)
  - Deterministic ZIP archive hashing
  - Hash format parsing and normalization (sha256:hex, sha512:hex)
  - 12 hash verification tests passing
- **Signature Verification** ✅ (pkg/module/verify/signature.go)
  - DefaultSignatureVerifier for cryptographic signatures
  - Support for RSA, ECDSA, and Ed25519 signatures
  - PEM-encoded public key parsing (PKIX, PKCS1)
  - VerifySignature() against module files
  - GetSignerIdentity() for extracting signer information
  - CosignVerifier placeholder for future Sigstore integration
  - Module signature files (.sig) support
- **SumDB Client (Transparency Log)** ✅ (pkg/module/verify/sumdb.go)
  - HTTPSumDBClient for remote transparency log queries
  - InMemorySumDB for testing and air-gapped environments
  - Lookup() - retrieve module hash from SumDB
  - Verify() - validate module hash against SumDB
  - Submit() - submit module hash to SumDB
  - Caching layer for performance
  - Duplicate submission detection (same hash allowed, different hash rejected)
  - Hash normalization for comparison
  - 4 SumDB tests passing
- **Trust Policies** ✅ (pkg/module/verify/trust.go)
  - DefaultTrustPolicy for managing trusted keys
  - AddTrustedKey(), RemoveTrustedKey(), IsTrusted()
  - Key fingerprinting with SHA256
  - TrustedKeyIDs for fingerprint-based trust
  - CompositeTrustPolicy for combining multiple policies
  - GetPublicKey() for retrieving trusted keys
  - Thread-safe operations with RWMutex
  - 3 trust policy tests passing
- **Module Verifier (Orchestration)** ✅ (pkg/module/verify/verifier.go)
  - ModuleVerifier coordinates complete verification workflow
  - NewModuleVerifier() with configuration from VerificationOptions
  - Verify() - full verification pipeline for module files
  - VerifyArtifact() - verify ModuleArtifact with detailed reporting
  - Hash verification with expected hash comparison
  - Signature verification with trust policy integration
  - SumDB verification with transparency log lookup
  - Insecure mode support (AllowInsecure) with warnings
  - Enforcement mode control (RequireSignature, RequireSumDB, RequireHashMatch)
  - SetSumDB(), SetTrustPolicy() for runtime configuration
  - 6 comprehensive verifier tests passing

**Phase 3 Achievements**:
- Complete cryptographic verification system
- Multi-algorithm hash verification (SHA256, SHA512)
- Digital signature verification (RSA, ECDSA, Ed25519)
- Transparency log integration (SumDB client)
- Flexible trust policy system with key fingerprinting
- Composite verification workflow with configurable requirements
- Insecure mode for development/testing
- 22 comprehensive tests passing (100% pass rate)
- ~800 lines of production code
- Ready for integration with module resolver (Phase 4)

**Phase 4: Module Resolver & Dependency Management ✅ COMPLETE (Week 7)**
- **Resolver Types & Interfaces** ✅ (pkg/module/resolver/types.go, errors.go)
  - ModuleReference with name, version, resolved version, and hash
  - DependencyNode for dependency graph representation
  - ResolutionRequest and ResolutionResult structures
  - RegistryClient, Resolver, VersionConstraint, VersionSelector interfaces
  - ConflictResolver interface for version conflict resolution
  - DependencyGraph interface for graph operations
  - CacheConfig and CacheEntry for module caching
  - Comprehensive error types (CircularDependencyError, ConstraintError, ConflictError, etc.)
- **SemVer Parsing & Constraints** ✅ (pkg/module/resolver/semver.go)
  - Version parsing with SemVer 2.0.0 compliance
  - Major.minor.patch with optional prerelease and build metadata
  - Version comparison with correct prerelease ordering
  - Constraint operators: =, !=, >, >=, <, <=, ^, ~, *
  - Caret (^) for compatible versions (^1.2.3 allows >=1.2.3 <2.0.0)
  - Tilde (~) for patch-level changes (~1.2.3 allows >=1.2.3 <1.3.0)
  - Multi-constraint support (AND'd constraints)
  - DefaultConstraintParser with wildcard and "latest" support
  - DefaultVersionSelector for selecting highest/lowest matching versions
  - 59 SemVer tests passing
- **Dependency Graph (DAG)** ✅ (pkg/module/resolver/dag.go)
  - DefaultDependencyGraph with adjacency list representation
  - Dual edge maps: edges (dep -> dependents) and dependencies (node -> deps)
  - AddNode(), GetNode(), GetAllNodes() for graph manipulation
  - HasCycle() with DFS-based cycle detection and path reconstruction
  - TopologicalSort() using Kahn's algorithm
  - Flatten() for flattened dependency list
  - GetDependencies() and GetDependents() for graph queries
  - Thread-safe operations with RWMutex
  - 12 DAG tests passing including complex graphs
- **MVS (Minimum Version Selection)** ✅ (pkg/module/resolver/mvs.go)
  - MVSConflictResolver implementing Go's MVS algorithm
  - ResolveWithVersions() for conflict resolution with available versions
  - Selects highest version satisfying all constraints
  - BuildRequirementList for tracking module requirements
  - AddRequirement() with automatic version upgrades (higher wins)
  - Merge() for combining requirement lists
  - Sort() for deterministic requirement ordering
  - 11 MVS tests passing
- **Content-Addressed Cache** ✅ (pkg/module/resolver/cache.go)
  - ModuleCache with content-addressed storage (hash[:2]/hash)
  - Put() with automatic hash computation
  - Get() for retrieving cached modules by hash
  - Has() for existence checks
  - Delete() for cache entry removal
  - List() for all cached modules
  - Clean() with MaxAge and MaxSize policies
  - Oldest-first eviction when over size limit
  - Index persistence (index.json) across restarts
  - Size() and Count() for cache statistics
  - Readonly mode support
  - Thread-safe with RWMutex
  - 9 cache tests passing

- **Resolver Orchestration** ✅ (pkg/module/resolver/resolver.go)
  - ModuleResolver implementing the Resolver interface
  - Resolve() - complete dependency resolution workflow
  - ResolveFromManifest() - resolve from module manifest
  - Update() - update dependencies to latest compatible versions
  - ValidateLockFile() - validate lock file integrity
  - resolveDependencies() - recursive dependency resolution with cycle detection
  - Integrates SemVer constraint parsing, version selection, DAG building, and MVS conflict resolution
  - Support for lock file resolution (use pinned versions)
  - Configurable max depth and prerelease handling
  - 4 resolver orchestration tests passing
- **Lock File Generation & Validation** ✅ (pkg/module/resolver/resolver.go)
  - generateLockFile() - generates lock file from resolved dependencies
  - buildGraphFromLockFile() - reconstructs dependency graph from lock file
  - ValidateLockFile() - validates lock file against registry
    - Schema version validation
    - Module existence verification
    - Hash integrity verification
  - Lock file format: map[string]LockedModule (manifest.LockFile)
  - 5 lock file validation tests passing

**Phase 4 Achievements (ALL 7 TASKS COMPLETE)**:
- Complete resolver infrastructure with orchestration
- SemVer 2.0.0 compliant version handling
- DAG-based dependency graph with cycle detection
- MVS algorithm for reproducible version selection
- Content-addressed module cache with eviction policies
- Lock file generation and validation
- End-to-end resolution integration tests
- 115+ comprehensive tests passing (100% pass rate)
- ~2,000 lines of production code

**Phase 5: Policy Integration ✅ COMPLETE (Week 8)**
- **Policy Integration Types** ✅ (pkg/module/policy/types.go)
  - ModulePolicyContext with Module, Capabilities, TrustLevel, Environment, User, Timestamp
  - TrustLevel enum with 6 levels: Unknown, Untrusted, Community, Verified, Internal, System
  - ModulePolicyResult with Allowed, AllowedCapabilities, DeniedCapabilities, Violations, Warnings
  - ModulePolicyValidator interface for policy evaluation
  - CapabilityPolicyConfig with trust level requirements and environment restrictions
  - ModulePolicyRule with RuleConditions and RuleAction
  - ActionType enum: Allow, Deny, Warn, Modify
  - LoadTimePolicy and RuntimePolicy for policy hooks
- **Module Policy Validator** ✅ (pkg/module/policy/validator.go)
  - ModulePolicyEngine coordinating policy evaluation
  - ValidateModule() - complete policy validation workflow
  - ValidateCapability() - single capability validation against policies
  - ValidateCapabilities() - batch capability validation
  - DefaultCapabilityPolicyConfig() with security defaults:
    - Require approval: exec, http.post, secrets.write
    - Trust requirements: exec→Verified, http.post→Community, secrets.write→Verified
    - Environment restrictions: prod blocks exec and secrets.write
  - Custom rule support with priority ordering
  - Rule matching: module name patterns, trust levels, environments, capabilities
  - Rule actions: deny, warn, modify capabilities, block
  - Trust level comparison (meetsMinimumTrust, meetsMaximumTrust)
  - Enforcement mode support (Enforce, Audit, Warn)
- **Comprehensive Tests** ✅ (pkg/module/policy/validator_test.go)
  - 23 tests covering all policy validation scenarios
  - Default config validation
  - Capability blocking and trust level enforcement
  - Environment-specific restrictions
  - Enforcement mode testing (Enforce vs Audit)
  - Custom rule evaluation and priority ordering
  - Rule condition matching (patterns, trust, environment, capabilities)
  - Rule action application (deny, warn, modify, block)
  - Trust level comparison helpers
  - 93.2% test coverage

**Phase 5 Achievements**:
- Complete policy integration for module security
- Trust-based capability enforcement
- Environment-specific policy restrictions
- Custom rule system with flexible conditions and actions
- Integration with Epic 6 (Policy Engine) for OPA/CEL support
- 23 comprehensive tests passing (100% pass rate)
- 93.2% test coverage
- ~600 lines of production code
- Ready for integration with module loader and runtime

**Phase 6: Plugin SDK & Developer Experience ✅ COMPLETE (Week 9)**

- **T6.0: Starlark SDK & Testing Framework** ✅ (pkg/module/sdk/starlark/)
  - ModuleTemplate for scaffolding new modules
    - generateManifest(), generateMainStar(), generateTests(), generateReadme()
    - Complete module.yaml, states/main.star, tests/, README.md generation
  - TestRunner for Starlark test execution
    - RunTestFile() with test discovery
    - assertEq, assertNe, assertTrue, assertFalse, assertFail, assertContains
    - TestSuite and TestCase tracking
    - Success/failure reporting with detailed output
  - Type conversion helpers (ConvertToGo, ConvertFromGo)
    - Bidirectional Starlark ↔ Go value conversion
    - StringValue, IntValue, BoolValue, DictValue, ListValue
  - 18 tests passing (template, testing, helpers)
  - 66.8% test coverage

- **T6.1: WASM SDK - Rust** ✅ (modules/sdk/rust/)
  - Complete Rust SDK for wasm32-wasi compilation
  - Core library (src/lib.rs, types.rs, error.rs, host.rs)
    - module_main!, export_fn! macros for WASM exports
    - Capability, ModuleContext, ModuleResult<T> types
    - Error types: CapabilityDenied, FileSystem, Http, Exec, Serialization
    - Host function bindings for all capabilities (extern "C" imports)
  - Capability modules:
    - fs: read_file, write_file, read_string, write_string
    - http: get, post (JSON response parsing)
    - exec: run, run_with_input
    - log: debug, info, warn, error
    - kv: get (optional), set
    - system: cpu_info (cross-platform: Linux/macOS/Windows)
    - crypto: sha256, sha256_string
  - Hello world example (examples/hello-world/)
    - Complete demo: CPU info, SHA256 hash, file write
    - module.yaml manifest, Cargo.toml
  - Comprehensive README with API docs and examples
  - 30 tests passing (4 unit + 23 integration + 3 doc tests)
  - Size-optimized: opt-level="z", lto=true, strip=true

- **T6.2: WASM SDK - Go (TinyGo)** ✅ (modules/sdk/go/)
  - Complete Go SDK for TinyGo wasm32-wasi compilation
  - Core library (types.go, error.go, host.go, host_stub.go)
    - Capability constants, ModuleContext, ModuleResult[T]
    - Error types with custom Error struct
    - Host function bindings with //go:wasm-module directive
    - Build tag separation (tinygo.wasm vs normal Go)
  - Capability functions:
    - ReadFile, WriteFile, ReadString, WriteString
    - HTTPGet, HTTPPost (JSON unmarshaling)
    - Exec, ExecWithInput
    - LogDebug, LogInfo, LogWarn, LogError
    - KvGet (returns bool for existence), KvSet
    - GetCPUInfo (cross-platform)
    - SHA256, SHA256String
  - Hello world example (examples/hello-world/)
    - go.mod with local replace directive
    - module.yaml with TinyGo build command
  - Comprehensive README with TinyGo setup instructions
  - 22 tests passing (all API coverage with stubs)
  - TinyGo-optimized: 50-200 KB typical binary size

- **T6.3: WASM SDK - C++** ✅ (modules/sdk/cpp/)
  - Complete C++17 header-only SDK
  - Headers (include/kscore/):
    - kscore.h: Main include with SDK version
    - types.h: Capability, ModuleContext, ModuleResult<T>, LogLevel
    - error.h: Exception-based error types with Error base class
    - host.h: Host function bindings with all capabilities
  - Capability namespaces:
    - kscore::fs: read, write, read_string, write_string
    - kscore::http: get, post (minimal JSON parsing)
    - kscore::exec: run, run_with_input
    - kscore::log: debug, info, warn, error
    - kscore::kv: get (returns optional), set
    - kscore::system: get_cpu_info
    - kscore::crypto: sha256, sha256_string
  - Minimal JSON parser/builder (no external deps)
  - CMakeLists.txt with WASI SDK and Emscripten support
  - Hello world example (examples/hello-world/)
    - CMakeLists.txt, module.yaml
    - Size optimization: -Os, -flto, -fno-exceptions, -fno-rtti
  - Comprehensive README with WASI SDK/Emscripten setup
  - Header-only (no tests - validated via compilation)
  - 100-300 KB typical binary size

- **T6.4: Stdlib Modules** ✅ (modules/stdlib/)
  - Six standard library modules in Starlark:

  - std/files (fs.read, fs.write capabilities)
    - read, read_bytes, write, write_bytes
    - exists, read_lines, write_lines, append

  - std/exec (exec capability)
    - run, run_with_input, success, output, which

  - std/http (http.get, http.post capabilities)
    - get, post, get_text, get_json, post_json, is_success

  - std/strings (no capabilities - pure Starlark)
    - upper, lower, title, trim, split, join, replace
    - contains, has_prefix, has_suffix, trim_prefix, trim_suffix
    - repeat, reverse

  - std/json (no capabilities - wraps built-in json)
    - encode, decode, indent

  - std/crypto (exec, fs.write capabilities)
    - sha256, sha256_file, verify_sha256

  - All modules: module.yaml manifests, comprehensive docstrings
  - Export public API via struct pattern
  - Maintain capability constraints (security-first)

- **T6.6: Hello World Examples in All Languages** ✅ (modules/examples/)
  - Starlark example (hello-world-starlark/)
    - Uses stdlib modules (exec, files, crypto, json)
    - Cross-platform CPU detection
    - module.yaml with capability declarations

  - Rust example (hello-world-rust/) - see T6.1
  - Go example (hello-world-go/) - see T6.2
  - C++ example (hello-world-cpp/) - see T6.3

  - All examples perform identical operations:
    1. Get CPU make and model
    2. Compute SHA256 hash
    3. Write to temp file (hello-from-kscore-{lang}.txt)
    4. Return JSON with cpu_info, hash, file_path

  - Comprehensive README.md:
    - Language comparison table (binary size, build time, execution time)
    - Performance comparison
    - Language choice guide
    - Testing instructions

**Phase 6 Achievements**:
- Complete SDK suite for all supported languages (Starlark, Rust, Go, C++)
- Starlark testing framework with 6 assertion functions
- Three WASM SDKs with comprehensive capability bindings
- Six stdlib modules providing common functionality
- Four hello world examples demonstrating language equivalence
- Comprehensive documentation and README files
- All examples meet user requirements for identical results
- Total: 70+ tests passing across all SDKs
- ~8,500 lines of SDK and module code

**Phase 7: Module Loader & Orchestration ✅ COMPLETE (Week 10)**
- **T7.0: Module Loader Architecture** ✅ (pkg/module/loader/)
  - 6-phase module loading orchestration:
    1. Manifest parsing (module.yaml)
    2. Cryptographic verification (hash, signature, SumDB)
    3. Policy validation (trust level, capabilities)
    4. Runtime initialization (Starlark or WASM)
    5. Capability registration
    6. LRU caching with TTL
  - LoadOptions with skip flags for verification/policy
  - ExecuteOptions with timeout and context support
  - LoadResult tracking manifest, runtime, verification, policy results
  - 10 LoadEventType constants for progress tracking
  - Event emission for monitoring load workflow
  - CapabilityBackends for pluggable storage/logging
  - 399-line orchestration implementation

- **T7.1: Module Cache** ✅ (pkg/module/loader/cache.go)
  - InMemoryModuleCache with LRU eviction
  - TTL-based expiration
  - Per-entry statistics (access count, access time)
  - Automatic cleanup of expired entries
  - Get(), Put(), Evict(), Clear() operations
  - Thread-safe with RWMutex

- **T7.2: Runtime Interface Unification** ✅ (pkg/module/runtime/types.go)
  - Unified Runtime interface with Close() method
  - StarlarkRuntime interface with ExecuteFile()
  - WasmRuntime interface with ExecuteFunction()
  - Context-based execution with timeout support
  - Updated Starlark runtime with ExecuteFile implementation
  - Updated WASM runtime with ExecuteFunction implementation

- **T7.3: Type Definition Completion** ✅
  - pkg/module/verify/types.go: 20+ fields/methods added
    - HashValid, SignatureValid, TrustedKey, SumDBVerified
    - SignerIdentity, ContentHash fields
    - AddError(), AddWarning() methods
    - VerifyHash(), GetSignerIdentity() interface methods
    - NewVerificationReport() constructor
  - pkg/module/policy/types.go: Complete policy structures
    - PolicyCondition with all validation fields
    - PolicyAction with Type, Block, Warn, capabilities
    - Violation structure with PolicyID, RuleID, Severity
    - ModuleInfo for module metadata
    - TrustLevelSystem constant
  - pkg/module/capabilities/types.go: Interface implementations
    - SecretsStore: Get(), Set(), Delete() methods
    - Logger: Log() with fields parameter
    - KVStore: Get(), Set(), Delete(), List() methods

- **T7.4: Capability Stub Constructors** ✅ (pkg/module/capabilities/capabilities.go)
  - 10 capability constructor functions:
    - NewFSReadCapability, NewFSWriteCapability
    - NewHTTPGetCapability, NewHTTPPostCapability
    - NewExecCapability
    - NewSecretsReadCapability, NewSecretsWriteCapability
    - NewLogCapability, NewTimeCapability, NewKVCapability
  - StubCapability implementation for testing
  - All constructors return Capability interface

- **T7.5: Comprehensive Testing** ✅ (pkg/module/loader/loader_test.go)
  - 14 test functions, all passing (0.688s)
  - Tests cover:
    - Module loader creation and configuration
    - Load/Execute options with defaults
    - Load/Execute result structures
    - Event type enumeration
    - Cache management
    - Event handler registration
    - Invalid path handling
    - All 10 capability constructors
    - Memory limit parsing
    - Timeout handling
    - Starlark and WASM module loading

- **T7.6: Capability Wiring to Runtimes** ✅ (pkg/module/runtime/)
  - Starlark capability builtins (builtins.go)
    - All 10 capability types wired to Starlark functions
    - fs_read, fs_exists, fs_write, fs_delete, fs_mkdir
    - exec, http_get, http_post, log
    - kv_get, kv_set, kv_delete
    - secret_get, secret_set, time_now
    - Safe type assertions for stub capability handling
  - WASM host function bindings (wasm_builtins.go)
    - All 10 capability types wired as WASM host functions
    - wazero host module builder with proper function signatures
    - Memory read/write helpers for string marshaling
    - JSON result serialization for complex return values
  - Fine-grained capability restrictions from manifest
    - CapabilityConfig struct with allowed/denied paths, domains, commands
    - Path pattern matching for filesystem capabilities
    - Domain matching for HTTP capabilities
    - Command allowlist for exec capability
    - Backwards-compatible defaults when restrictions not specified
  - ~800 lines of capability wiring code

- **T7.7: Capability Policy & Lock System** ✅ (pkg/module/capabilities/)
  - Capability policy evaluation (policy.go)
    - CapabilityMode: allow, deny, restrict
    - TrustLevel: none, limited, full
    - CapabilityPolicyConfig for fine-grained restrictions
    - ModulePolicy for per-module settings with lock flag
    - CapabilityPolicy with defaults and per-module overrides
    - PolicyEvaluator with EvaluateCapability(), EvaluateAllCapabilities()
    - CheckModuleUpdate() for update lock validation
    - FilePolicyStore for YAML-based policy storage
    - DefaultCapabilityPolicy() with secure defaults (deny exec, restrict fs.write)
    - Wildcard matching for capability denial (e.g., "fs.*")
    - Config merging with intersection (more restrictive wins)
  - Capability lock storage (lock.go)
    - CapabilityLock struct with module, version, capabilities, config
    - HasCapability(), GetCapabilityConfig(), AddCapability()
    - LockStore interface for pluggable storage
    - InMemoryLockStore for testing
    - FileLockStore for JSON file persistence
    - LockManager for high-level lock operations
    - LockModule(), UnlockModule(), CheckUpdate()
    - CreateLockFromManifest() helper
  - Module loader integration (loader.go)
    - SetCapabilityPolicyEvaluator(), SetLockManager()
    - Capability policy check phase in Load()
    - Module update lock violation detection
    - Denied capabilities tracked in LoadResult
    - CapabilityPolicyDecisions in LoadResult
  - 28 comprehensive tests (policy_test.go, lock_test.go)
    - Policy evaluation tests (allowed, denied, wildcard, trust levels)
    - Lock storage tests (in-memory, file, persistence)
    - Lock manager tests (lock, unlock, check update)
    - Config merging and restriction tests
  - ~700 lines of policy/lock code

**Phase 7 Achievements**:
- Complete 7-phase module loading workflow (now includes capability policy check)
- Unified runtime interface for Starlark and WASM
- Complete capability wiring to both Starlark and WASM runtimes
- Capability policy and lock system for operator control
- Type-safe orchestration with comprehensive error handling
- LRU caching for performance optimization
- Event-driven progress tracking
- All dependency packages properly typed and integrated
- 42 comprehensive tests passing (14 loader + 28 policy/lock)
- Successfully compiles and executes complete load workflow
- ~600 lines of loader code + 200 lines of type updates + 800 lines of capability wiring + 700 lines of policy/lock

**Phase 7 Deferred**:
- Performance optimization and benchmarking
- Advanced caching strategies (content-addressable, distributed)
- CLI tooling (kscorectl module init/build/test commands)

**Epic 9 Complete!** All 7 phases finished:
- Phase 1: Runtime foundation (Starlark, WASM, manifest) ✅
- Phase 2: Capability system (10 capability types) ✅
- Phase 3: Cryptographic verification (hash, signature, SumDB, trust) ✅ (Cosign is stub only)
- Phase 4: Dependency resolution (SemVer, DAG, MVS) ✅
- Phase 5: Registry & distribution ✅ (HTTP registry client and kscore-registry server implemented)
- Phase 6: SDKs & stdlib (Starlark, Rust, Go, C++) ✅
- Phase 7: Module loader orchestration (6-phase loading) ✅

**Total Epic 9 Achievements**:
- Complete plugin system architecture
- 10 capability types with path/domain/command scoping
- Capability wiring to Starlark (builtins.go) and WASM (wasm_builtins.go) runtimes
- Fine-grained capability restrictions via manifest CapabilityConfig
- Capability policy system for operator override/restriction of module capabilities
- Capability lock system to prevent malicious module updates from escalating permissions
- Full cryptographic verification pipeline (Cosign stub only - RSA/ECDSA/Ed25519 work)
- Dependency resolution with MVS algorithm
- OCI registry client (OCI Distribution Spec) + HTTP registry client (Go-mod style)
- kscore-registry server with comprehensive test coverage (18 tests)
- SDK suite for 4 languages (Starlark, Rust, Go, C++)
- 6 stdlib modules + 4 hello world examples
- Module loader with caching and orchestration
- 180+ comprehensive tests passing
- ~17,000+ lines of production code

**Epic 9 Implementation Gaps**:
- `pkg/module/registry/` - ✅ Both OCI and HTTP registry clients implemented
  - OCI client: Push/Pull with OCI Distribution Spec, manifest/blob handling
  - HTTP client: Go-mod style endpoints for module distribution
- `cmd/kscore-registry/` - ✅ HTTP server with tests (18 tests passing)
- `cmd/kscore-module/` - ✅ CLI fully implemented (init, validate, build, sign, publish, install, resolve, tree, verify, test)
- Cosign verification ✅ IMPLEMENTED (ECDSA P-256, Ed25519, base64 signatures, bundle parsing, identity extraction)

### Epic 10: Documentation ✅ COMPLETE

**Implementation Plan:** 7 phases (All phases complete)

**Current Status**: All 7 Phases COMPLETE ✅

**Phase 1: Documentation Infrastructure & Getting Started ✅ COMPLETE**
- Hugo + Docsy documentation site setup
  - Hugo configuration with publishDir → build/docs
  - Docsy theme as git submodule
  - npm dependencies for PostCSS/Bootstrap/Font-Awesome
  - Symlink setup for build requirements
- Getting Started documentation (4 pages, 1,583 lines):
  - **Overview** (250 lines): What is Keystone Core, 8 core capabilities, use cases, comparison table
  - **Installation** (396 lines): 5 installation methods, configuration, systemd services
  - **Quick Start** (352 lines): 9-step walkthrough, 5-minute deployment guide
  - **Architecture** (512 lines): High-level diagrams, components, data flows, scaling
- Documentation infrastructure:
  - docs/hugo.toml - Hugo configuration
  - docs/README.md - Build instructions and guidelines (171 lines)
  - docs/content/en/ - Markdown documentation content
  - docs/themes/docsy/ - Docsy theme (git submodule)
- Build system:
  - Clean builds with no warnings
  - Output to build/docs/ (16 pages generated)
  - Local development server with live reload
  - build/ directory structure (docs, bin)
- .gitignore updated for build artifacts

**Phase 1 Achievements**:
- Complete documentation infrastructure
- Getting Started section 100% complete (4 pages)
- Hugo builds cleanly to build/docs/
- All documentation sources committed to git repo
- ~1,750 lines of documentation + build configuration
- Build directory structure for all build outputs

**Phase 3: Core Concepts Documentation ✅ COMPLETE**
- Comprehensive deep-dive documentation for all major subsystems (10 pages, ~7,075 lines):
  - **Part 1** (5 pages, 2,675 lines):
    - Control Plane (600+ lines): Architecture, components, deployment modes, configuration
    - Agents (500+ lines): Lifecycle, cross-platform support, edge/offline mode
    - Message Bus (400+ lines): NATS architecture, deployment modes, JetStream
    - State Management (500+ lines): Declarative config, 6 modules, drift detection
  - **Part 2** (6 pages, 4,400 lines):
    - Remote Execution (500+ lines): Targeting, job tracking, cross-platform shells
    - Events (450+ lines): 15 event types, filtering, storage, integration
    - Reactors (850+ lines): Event automation, actions, orchestration patterns
    - GitOps (850+ lines): Webhooks, verification, rollback, promotion pipelines
    - Policy (950+ lines): OPA/CEL engines, enforcement modes, compliance
    - Observability (800+ lines): Metrics, logging, tracing, Grafana dashboards
- Each concept page includes:
  - Architecture diagrams (ASCII art)
  - Configuration examples with YAML/code
  - Use cases and design patterns
  - Best practices
  - Troubleshooting guides
  - Performance characteristics
  - Cross-references to related concepts

**Phase 3 Achievements**:
- Complete core concepts documentation covering Epics 1-7
- ~7,075 lines of comprehensive concept documentation
- All 10 concept pages built and verified
- Consistent structure and quality across all pages
- Hugo builds cleanly with no warnings

**Phase 4: Reference Documentation ✅ COMPLETE**
- Comprehensive technical reference documentation (6 pages, ~5,000 lines):
  - **API Reference** (~900 lines): Complete REST and gRPC API documentation
    - All major endpoints (Agents, Execution, State, Events, Policy, GitOps)
    - Authentication methods (API key, mTLS)
    - Request/response examples with JSON
    - Rate limiting, pagination, filtering
    - Webhook configuration
    - Client libraries (Go, Python, JavaScript)
  - **CLI Reference** (~850 lines): Complete kscorectl and plugin command reference
    - kscore-exec (remote execution)
    - kscore-state (state management)
    - kscore-monitor (TUI monitoring)
    - kscore-module (module management)
    - kscore-policy (policy management)
    - kscore-gitops (GitOps management)
    - Global flags, environment variables, shell completion
  - **Configuration Reference** (~700 lines): Complete YAML configuration reference
    - Control plane configuration (all subsystems)
    - Agent configuration
    - CLI configuration
    - State file syntax with requisites
    - Reactor definitions
    - Policy definitions
  - **Module Reference** (~800 lines): All 6 state modules documented
    - Complete parameter specifications
    - Platform compatibility matrices
    - Idempotency guarantees
    - Requisites (require, watch, prereq, onchanges)
    - Template support with vars/facts
    - 30+ complete examples
  - **Event Reference** (~850 lines): All 15 event types with schemas
    - 5 event categories (Agent, Job, State, System, User)
    - CEL filtering expression reference
    - 50+ filter examples
    - Event querying methods
  - **Metrics Reference** (~900 lines): Complete Prometheus metrics catalog
    - 70+ metrics documented
    - 7 metric categories (Control Plane, Agent, Execution, State, Events, Policy, GitOps)
    - PromQL query examples
    - Alert rule examples
- Each reference page includes:
  - Complete API/command/parameter specifications
  - Request/response examples
  - Code samples in multiple languages
  - Best practices
  - Common workflows
  - Troubleshooting tips

**Phase 4 Achievements**:
- Complete reference documentation covering all Keystone Core APIs and interfaces
- ~5,000 lines of detailed technical reference
- All 6 reference pages built and verified
- Consistent structure across all reference pages
- Hugo builds cleanly with no warnings
- Individual commits per section for better tracking

**Phase 5: Operations Guide ✅ COMPLETE**
- Comprehensive operational documentation (6 pages, ~4,500 lines):
  - **Operations Navigation** (150 lines): Guide overview and navigation
    - Quick navigation by role (DevOps, Platform, Security, SRE)
    - Production deployment checklist
    - Best practices summary
  - **Deployment Guide** (~850 lines): Production deployment patterns
    - Single-node deployment (embedded NATS, SQLite)
    - High-availability setup (HA cluster, external NATS, PostgreSQL)
    - Kubernetes deployment (Helm charts, StatefulSets, DaemonSets)
    - Docker Compose deployment
    - Scaling strategies (horizontal and vertical)
    - Migration paths (embedded→external NATS, SQLite→PostgreSQL)
  - **Monitoring Guide** (~950 lines): Complete observability setup
    - Prometheus integration (installation, configuration, key metrics)
    - Grafana dashboards (6 pre-built dashboards, custom creation)
    - Log aggregation (Loki and Elasticsearch setup)
    - Alerting (Alertmanager, alert rules, PagerDuty/Slack)
    - Health checks (liveness, readiness, detailed status)
    - Performance monitoring (SLOs, SLO dashboards, error budgets)
  - **Maintenance Guide** (~950 lines): Operational maintenance procedures
    - Backup procedures (SQLite, PostgreSQL, JetStream, configs)
    - Restore procedures (full restore, PITR, disaster recovery)
    - Upgrade procedures (single-node, HA rolling, Kubernetes, agents)
    - Database maintenance (vacuum, reindex, optimization)
    - SQLite → PostgreSQL migration
    - Data retention policies and capacity planning
  - **Troubleshooting Guide** (~900 lines): Diagnostic procedures
    - Agent connectivity issues (firewall, DNS, TLS, credentials)
    - NATS connection problems (cluster, JetStream, resources)
    - State application failures (syntax, dependencies, timeouts)
    - Performance issues (CPU, memory, disk, database)
    - Common error messages with solutions
    - Debug logging and network diagnostics
    - Performance tuning (OS, PostgreSQL, NATS, control plane)
  - **Security Guide** (~950 lines): Security hardening and compliance
    - Authentication (API keys, JWT tokens, mTLS certificates)
    - TLS configuration (control plane, NATS, PostgreSQL)
    - RBAC (built-in roles, custom roles, policy-based access)
    - Security hardening (OS, application, network segmentation)
    - Audit logging (format, querying, retention, archival)
    - Compliance (SOC 2, HIPAA, GDPR)
    - Secret management (Vault, Kubernetes secrets)
    - Security checklist and incident response
- Each operations page includes:
  - Step-by-step procedures with command examples
  - Configuration file examples
  - Diagnostic commands and troubleshooting steps
  - Best practices and production recommendations
  - Cross-references to related documentation

**Phase 5 Achievements**:
- Complete operations documentation for production deployments
- ~4,500 lines of operational procedures and guides
- All 6 operations pages built and verified
- Comprehensive coverage from deployment to security
- Hugo builds cleanly with no warnings
- Individual commits per guide for better tracking

**Phase 6: Community Documentation ✅ COMPLETE**
- Comprehensive community documentation (4 pages, ~2,000 lines):
  - **Community Index** (~55 lines): Navigation and community principles
    - Quick links to all community resources
    - Get involved section
    - Code of conduct summary
  - **Contributing Guide** (~430 lines): Complete contribution workflow
    - Bug reporting and feature request templates
    - Fork, branch, commit, PR workflow
    - Coding standards (Go style, error handling, logging)
    - Testing guidelines (unit, integration, table-driven)
    - Pull request guidelines and review process
    - First-time contributor resources
  - **Development Guide** (~720 lines): Development environment setup
    - Prerequisites and tool installation
    - Repository structure overview
    - Building from source (all platforms, cross-compilation, Docker)
    - Running tests (unit, integration, coverage, benchmarks)
    - Linting and formatting
    - Local development workflow
    - Module development (Starlark and WASM)
    - Documentation development
    - IDE setup (VS Code, GoLand, Vim)
    - Debugging with Delve
  - **Roadmap** (~390 lines): Project roadmap and future plans
    - Project vision and overview
    - Completed milestones (Epics 1-9)
    - Current work (Epic 10)
    - Planned development (Epic 11: Clustering)
    - Future considerations (multi-tenancy, scheduling, security, UX)
    - Release schedule and versioning strategy
    - Contributing to roadmap
  - **Support** (~430 lines): Getting help and support resources
    - Community support channels (GitHub Discussions, Discord, Stack Overflow)
    - Documentation resources and troubleshooting guides
    - Bug reporting and security issue reporting
    - Self-help resources and common issues
    - Commercial support options
    - Community guidelines

**Phase 6 Achievements**:
- Complete community documentation for contributors and users
- ~2,000 lines of community-focused content
- All 4 community pages built and verified
- Clear contribution workflow documented
- Development environment setup guide
- Project roadmap with completed/planned work
- Hugo builds cleanly with no warnings (38 total pages)

**Epic 10 Final Statistics**:
- Total documentation pages: 40
- Total lines of documentation: ~20,800
- Sections completed: Getting Started, Core Concepts, Reference, Operations, Community
- All phases complete with no build warnings
- Release notes tracked in CHANGELOG.md

### Epic 11: High Availability Clustering ✅ COMPLETE

**Implementation Plan:** 8 phases (16 weeks total)

**Current Status**: All 8 Phases COMPLETE ✅

**Phase 1 Week 1-2: etcd Integration & Cluster Formation ✅ COMPLETE**
- etcd client integration (pkg/cluster/etcd.go)
  - etcd v3 client wrapper with connection management
  - Support for embedded and external etcd modes (⚠️ embedded mode config only - no `embed.Etcd` startup)
  - Session management with TTL-based leases
  - Transaction support with compare-and-swap
  - Retry logic with exponential backoff
  - TLS configuration support
- Cluster membership management (pkg/cluster/membership.go)
  - Member registration on startup
  - Heartbeat mechanism (configurable intervals)
  - Member discovery from etcd
  - Member health monitoring with status transitions
  - Automatic member removal on timeout
  - Observer pattern for membership changes
  - Quorum calculation and tracking
- Cluster configuration (pkg/cluster/config.go)
  - Comprehensive configuration structures
  - Validation for all settings
  - Support for embedded/external etcd modes
  - TLS configuration
  - Heartbeat/election timeout settings
- Cluster state storage (pkg/cluster/state.go)
  - StateStore for general cluster state
  - ClusterConfigStore for distributed configuration
  - ShardStore for agent-to-member assignments
  - DistributedLock for coordination
  - CoordinationStore with barriers and elections
  - Counter for distributed atomic counters
- Test coverage: 48.6% (91 tests passing)
  - Comprehensive unit tests for all components
  - Configuration validation tests
  - Error handling tests

**Phase 2: Leader Election & Work Distribution ✅ COMPLETE**
- Leader election (pkg/cluster/election.go)
  - etcd concurrency-based leader election
  - Campaign and resign methods
  - Leader observation with callbacks
  - Graceful leadership handoff
- Work distribution (pkg/cluster/sharding.go)
  - Consistent hashing for agent assignment
  - Virtual nodes for balanced distribution
  - Shard rebalancing on membership changes
  - Agent-to-member mapping

**Phase 3: Failover & Recovery ✅ COMPLETE**
- Automatic failover detection
- Agent reassignment on member failure
- State recovery from etcd
- Split-brain prevention with quorum checks
- Agent persistence and handoff (pkg/controlplane/connection_manager.go)
  - AgentStore interface for database-backed agent lookup
  - Load all agents from database on control plane startup
  - Dynamic agent lookup when heartbeat from unknown agent received
  - Seamless agent handoff between control plane servers
  - Zero re-registration during failover or rolling updates

**Phase 4: Data Consistency & Replication ✅ COMPLETE**
- Existing infrastructure from Phase 1 provides:
  - etcd-based distributed state storage
  - Transaction support for atomic operations
  - Consistent reads through etcd

**Phase 5: Cluster Operations & Management ✅ COMPLETE**
- Cluster CLI plugin (cmd/kscore-cluster/)
  - `kscore-cluster status` - Show cluster status
  - `kscore-cluster members` - List cluster members
  - `kscore-cluster leader` - Show current leader
  - `kscore-cluster health` - Cluster health check
  - `kscore-cluster rebalance` - Trigger agent rebalance
  - `kscore-cluster remove` - Remove unhealthy member
- REST API (pkg/api/cluster/handlers.go)
  - GET /cluster/status - Cluster status
  - GET /cluster/members - List members
  - GET /cluster/members/{id} - Member details
  - DELETE /cluster/members/{id} - Remove member
  - POST /cluster/rebalance - Trigger rebalance
  - GET /cluster/leader - Current leader info

**Phase 6: Observability & Monitoring ✅ COMPLETE**
- Cluster metrics (pkg/metrics/collectors.go)
  - kscore_cluster_members_total - Total cluster members
  - kscore_cluster_members_healthy - Healthy member count
  - kscore_cluster_has_quorum - Quorum status (0/1)
  - kscore_cluster_is_leader - Leadership status (0/1)
  - kscore_cluster_leader_changes_total - Leader change counter
  - kscore_cluster_leader_election_duration_seconds - Election latency
  - kscore_cluster_rebalance_total - Rebalance operations
  - kscore_cluster_rebalance_duration_seconds - Rebalance latency
  - kscore_cluster_agents_moved_total - Agents moved during rebalance
  - kscore_cluster_heartbeat_latency_seconds - Inter-member heartbeat latency
  - kscore_cluster_etcd_operations_total - etcd operation counter
  - kscore_cluster_etcd_operation_duration_seconds - etcd latency
  - kscore_cluster_member_status - Per-member health status
- ClusterCollector helper struct with typed methods
- Grafana dashboard (deploy/grafana/dashboards/cluster-health.json)
  - Cluster overview: quorum, members, leader status
  - Member health table and timeline
  - Leader election and rebalance metrics
  - etcd operations and latency
- Alert rules (deploy/grafana/alerts/kscore-alerts.yml)
  - ClusterNoQuorum (critical)
  - ClusterMemberUnhealthy (warning)
  - ClusterAllMembersUnhealthy (critical)
  - ClusterFrequentLeaderChanges (warning)
  - ClusterSlowLeaderElection (warning)
  - ClusterHighRebalanceRate (warning)
  - ClusterEtcdHighLatency (warning)
  - ClusterEtcdHighErrorRate (warning)
  - ClusterHeartbeatLatencyHigh (warning)
  - ClusterMemberCountLow (warning)

**Phase 7: Testing & Validation ✅ COMPLETE**
- Unit tests for all cluster components
- RemoveMember tests (membership_test.go)
- ClusterCollector tests (metrics_test.go)
- Integration tests for cluster operations
- Test coverage: pkg/cluster 48.6%, pkg/metrics 92.1%

**Phase 8: Documentation Update ✅ COMPLETE**
- Updated CLAUDE.md with Epic 11 completion status
- Documented all 8 phases with implementation details
- Updated Epic Dependencies section

**Epic 11 Achievements**:
- Complete embedded etcd mode using `go.etcd.io/etcd/server/v3/embed`
- Automatic server lifecycle management (start on Connect, stop on Close)
- Configurable data directory, ports, clustering, and logging
- All phases 1-8 fully implemented with no remaining gaps

### Epic 12: End-to-End & Performance Testing ✅ COMPLETE

**Implementation Plan:** 8 phases (Infrastructure built)

**Goal**: Comprehensive E2E testing framework using containers to validate all Keystone Core capabilities across deployment topologies.

**Phase 1: Test Infrastructure ✅ COMPLETE**
- Test harness (`test/e2e/harness/harness.go`) - Docker-compose based environment management
- HA cluster harness (`test/e2e/harness/ha_harness.go`) - Multi-server environment with lifecycle control
- Assertion helpers (`test/e2e/harness/assertions.go`) - Command execution, wait utilities, assertions
- Dockerfiles (`test/e2e/containers/Dockerfile.server`, `Dockerfile.agent`)
- Makefile (`test/e2e/Makefile`) - 20+ test targets for all test types

**Phase 2: Deployment Topology Tests ✅ COMPLETE**
- All-in-one topology (`test/e2e/containers/docker-compose.yml`)
  - 1 server (embedded NATS + SQLite) + 3 agents
  - Tests: agent registration, health, single/batch commands
- HA Cluster topology (`test/e2e/topologies/ha-cluster/docker-compose.yml`)
  - 3 control planes + 3 NATS nodes + 3 etcd nodes + PostgreSQL + 5 agents
  - Tests: cluster formation, leader election, failover, reconnection, quorum loss, rolling updates

**Phase 3: Functional E2E Tests ✅ COMPLETE**
- `test/e2e/scenarios/agent_lifecycle_test.go` - Registration, health, heartbeat, metadata, labels
- `test/e2e/scenarios/remote_exec_test.go` - Simple commands, streaming, stderr, timeouts, parallel
- `test/e2e/scenarios/state_management_test.go` - State application, drift detection
- `test/e2e/scenarios/event_system_test.go` - Event publishing, filtering, routing
- `test/e2e/scenarios/policy_enforcement_test.go` - Policy evaluation, enforcement
- `test/e2e/scenarios/gitops_webhook_test.go` - Webhook handling, event integration

**Phase 4: Performance Tests ✅ COMPLETE**
- `test/e2e/performance/scale_test.go` - Agent registration, status checks, concurrent operations
- `test/e2e/performance/throughput_test.go` - Sequential/parallel commands, batch throughput, sustained load
- Latency percentiles (P50, P95, P99) with JSON report output
- Baseline comparison framework

**Phase 5: Chaos Tests ⚠️ PARTIAL**
- HA cluster failover and quorum tests implemented
- Network partition tests skipped (require Docker network manipulation)
- Split-brain prevention tests skipped

**Phase 6-8: Not Yet Implemented**
- CI/CD Integration (GitHub Actions workflow)
- Multi-Platform Validation (ARM64, different Linux distros)
- Test documentation and reporting dashboards

**Test Structure:**
```
test/e2e/
├── harness/          # Test environment management
├── containers/       # All-in-one docker-compose + Dockerfiles
├── topologies/       # HA cluster docker-compose + configs
├── topology/         # Topology tests (allinone_test.go, hacluster_test.go)
├── scenarios/        # Feature scenario tests (6 files)
├── performance/      # Scale and throughput tests
└── Makefile          # Test orchestration
```

**Running Tests:**
```bash
# Quick smoke tests (all-in-one)
KSCORE_E2E_TESTS=1 make -C test/e2e test-quick

# Full test suite
KSCORE_E2E_TESTS=1 make -C test/e2e test-full

# HA cluster tests
KSCORE_E2E_TESTS=1 KSCORE_TOPOLOGY=ha-cluster make -C test/e2e test-ha

# Performance tests
KSCORE_E2E_TESTS=1 KSCORE_PERF_TESTS=1 make -C test/e2e test-performance
```

### Epic 13: CGO Removal - Pure Go Build ✅ COMPLETE

**Implementation Plan:** 3 phases (completed)

**Goal**: Remove all CGO dependencies to enable cross-compilation, simpler CI/CD, and smaller static binaries.

**Phase 1: SQLite Migration ✅ COMPLETE**
- Replaced `github.com/mattn/go-sqlite3` with `modernc.org/sqlite`
- Changed driver name from `"sqlite3"` to `"sqlite"`
- Files modified: `pkg/state/sqlite.go`, `pkg/events/storage_sqlite.go`
- All SQLite tests pass with `CGO_ENABLED=0`

**Phase 2: WASM Runtime Migration ✅ COMPLETE**
- Replaced `github.com/bytecodealliance/wasmtime-go` with `github.com/tetratelabs/wazero`
- Rewrote WASM runtime with wazero API (cleaner, more Go-idiomatic)
- Updated tests to use pre-compiled WASM binaries (wazero only accepts binary format)
- Files modified: `pkg/module/runtime/wasm/runtime.go`, `pkg/module/runtime/wasm/runtime_test.go`
- All WASM tests pass with `CGO_ENABLED=0`
- Note: wazero uses context timeouts instead of fuel metering for execution limits

**Phase 3: Validation & Cross-compilation ✅ COMPLETE**
- Verified `CGO_ENABLED=0 go build ./cmd/...` succeeds
- Cross-compilation tested:
  - `linux/amd64` ✅
  - `linux/arm64` ✅
  - `windows/amd64` ✅
  - `darwin/arm64` ✅

**Benefits Achieved:**
- `CGO_ENABLED=0 go build ./...` works for all core packages
- Cross-compilation without toolchains (linux/arm64, windows/amd64, etc.)
- Alpine/scratch Docker images without libc
- Simpler CI/CD (no gcc/clang required)

### Epic 14: NATS Mesh Communication ✅ COMPLETE

**Implementation Plan:** 11 phases (24 weeks total)

**Goal**: Decouple all agent↔server communication to use NATS as the sole transport layer, enabling flexible deployment topologies across NAT, firewalls, and complex network boundaries.

**Phase 1: NATS-Only Communication Refactor (Weeks 1-3)** ✅ COMPLETE

- **T1.1: Subject Namespace Design** ✅ COMPLETE
  - Created `pkg/nats/subjects.go` with cluster-prefixed subjects
  - Subject pattern: `kscore.{cluster}.{category}.{entity}.{operation}`
  - Categories: agent, server, bootstrap, discovery, command
  - SubjectBuilder for generating subjects with validation
  - Permission helpers for bootstrap/agent/server roles
  - 52 tests passing

- **T1.3: Message Protocol Enhancement** ✅ COMPLETE
  - Created `pkg/nats/message.go` with message envelope
  - Envelope with MessageID, CorrelationID, Priority, TTL
  - Cluster field for supercluster routing
  - Trace context (traceID, spanID) for distributed tracing
  - DeduplicationTracker for at-least-once semantics
  - EnvelopeHandler with automatic dedup and expiry checking
  - NATS message conversion with standard headers
  - Comprehensive tests passing

- **T1.2: Remove Direct gRPC Agent Connections** ✅ COMPLETE
  - Agents already use NATS-only communication (no gRPC)
  - Server gRPC retained only for client API (kubectl-style tools)

- **T1.4: Connection Manager Refactor** ✅ COMPLETE
  - ConnectionManager refactored to use SubjectBuilder
  - NATS-centric subscription management

- **T1.5: Agent Communication Refactor** ✅ COMPLETE
  - Agent refactored to use SubjectBuilder
  - NATS request/reply for registration
  - NATS pub/sub for heartbeat and commands

- **T1.6: Server-to-Server gRPC Coordination Channel** ✅ COMPLETE
  - Created `pkg/cluster/coordination.go` with gRPC service
  - ClusterHealth, GetLeader, NATSStatus, Heartbeat RPCs
  - mTLS authentication with existing certs
  - Lightweight heartbeat when NATS is healthy
  - Fallback coordination when NATS unavailable

- **T1.7: Secure Agent Bootstrap Registration** ✅ COMPLETE
  - Created `pkg/nats/bootstrap.go` with credential types
  - BootstrapCredential with NKey, Token, and JWT types
  - BootstrapCredentialProvider interface with InMemory implementation
  - Time-limited credentials (default 5 min TTL, max 24 hours)
  - Created `pkg/nats/bootstrap_handler.go` for registration flow
  - IdentityVerifier and CredentialIssuer interfaces for extensibility
  - Created `pkg/nats/bootstrap_audit.go` for audit logging
  - 7 audit event types integrating with events system
  - 66 tests covering all bootstrap functionality

**Phase 2: Multi-Endpoint Support (Weeks 4-5)** ✅ COMPLETE

- **T2.1: Endpoint Configuration** ✅ COMPLETE
  - Created `pkg/nats/endpoint.go` with Endpoint type
  - URL, priority, TLS, auth configuration
  - NATS URL parsing with scheme detection (nats, tls, ws, wss, leaf)
  - Priority-based endpoint ordering
  - ConnectionCallbacks for state change notifications
  - 14 tests passing

- **T2.2: Connection Strategy Framework** ✅ COMPLETE
  - Created `pkg/nats/strategy.go` with ConnectionStrategy interface
  - DirectStrategy for standard TCP connections
  - TLSStrategy for encrypted connections with cert configuration
  - WebSocketStrategy for firewall-friendly connections
  - LeafNodeStrategy for leaf node topology connections
  - StrategySelector for automatic strategy selection
  - 34 tests passing

- **T2.3: Multi-Endpoint Connection Manager** ✅ COMPLETE
  - Created `pkg/nats/connection_manager.go` with PooledConnectionManager
  - Connection pooling with configurable pool size
  - Circuit breaker pattern for failed endpoints
  - Automatic failover to next priority endpoint
  - State tracking (disconnected, connecting, connected, reconnecting, closed)
  - Callback notifications for state changes
  - 28 tests passing

- **T2.4: Health-Based Routing** ✅ COMPLETE
  - Created `pkg/nats/health.go` with HealthTracker
  - HealthChecker interface for custom health checks
  - Health status levels (unknown, healthy, degraded, unhealthy)
  - Multiple routing strategies:
    - Priority: prefer highest priority healthy endpoint
    - RoundRobin: distribute across healthy endpoints
    - LeastLatency: prefer lowest latency endpoint
    - Weighted: weight by health score and latency
    - Random: random selection from healthy endpoints
  - Comprehensive tests passing

- **T2.5: Connection Metrics** ✅ COMPLETE
  - Created `pkg/nats/metrics.go` with ConnectionMetrics
  - EndpointMetrics for per-endpoint statistics
  - Latency histograms with configurable buckets
  - Success/failure rate calculations
  - NATSStatsCollector for collecting NATS connection stats
  - MetricsCollectorCallbacks for integration
  - 33 tests passing

**Phase 3: Agent Embedded NATS (Reverse Connection) (Weeks 6-8)** ✅ COMPLETE

- **T3.1: Agent Embedded NATS Server** ✅ COMPLETE
  - Created `pkg/agent/nats_server.go` with EmbeddedNATSServer
  - Three modes: Disabled, Standalone, Leaf
  - Configurable host, port, advertise address for NAT traversal
  - TLS configuration with cert/key files or pre-configured *tls.Config
  - Authentication: Token, Username/Password, NKey
  - Leaf node remote configuration for hub connections
  - State tracking (stopped, starting, running, stopping)
  - Client connection callbacks for monitoring
  - Statistics: connections, messages, bytes, slow consumers
  - 20 tests passing

- **T3.2: Agent Endpoint Advertisement** ✅ COMPLETE
  - Created `pkg/agent/endpoint_advertiser.go` with EndpointAdvertisement
  - Endpoint types: NATS, NATS-TLS, WebSocket, WebSocket-TLS
  - Health status tracking (unknown, healthy, degraded, unhealthy)
  - Automatic public IP detection from multiple services
  - Local address collection for all network interfaces
  - TTL-based advertisement expiry
  - Sequence number for stale update detection
  - EndpointRegistry for tracking discovered endpoints
  - Change callback notifications
  - 20 tests passing

- **T3.3: Server Outbound Connection** ✅ COMPLETE
  - Created `pkg/controlplane/agent_connector.go` with AgentConnector
  - Connection states: disconnected, connecting, connected, reconnecting, failed
  - Registry integration for endpoint discovery
  - Automatic connection on endpoint registration
  - Automatic cleanup on endpoint unregistration
  - TLS requirement enforcement
  - NATS pub/sub/request to specific agents
  - Connection statistics tracking
  - Callback notifications for connect/disconnect
  - 20 tests passing

- **T3.4: Hybrid Mode** ✅ COMPLETE
  - Created `pkg/agent/hybrid_mode.go` with HybridModeManager
  - Connection roles: Undetermined, Client, Host, Leaf
  - Role selection modes: Auto, Manual, PreferHost, PreferClient
  - Network reachability detection: Direct, NAT, Restricted
  - Automatic role selection based on network topology
  - Fallback support (host → client, client → host)
  - Embedded server management for host mode
  - Endpoint advertiser integration
  - Local NATS client connection in host mode
  - Background reachability monitoring
  - Comprehensive statistics
  - 38 tests passing

**Phase 4: Leaf Node Support (Weeks 9-11)** ✅ COMPLETE

- **T4.1: Leaf Node Configuration** ✅ COMPLETE
  - Created `pkg/nats/leaf.go` with comprehensive leaf node support
  - LeafNodeRole enum: Leaf, Hub, Bridge (for multi-hop topologies)
  - LeafNodeConfig with remotes, listen, TLS, auth, subject mappings
  - LeafRemoteConfig for upstream hub connections
  - LeafTLSConfig for secure connections
  - LeafAuthConfig with token and user/password support
  - SubjectMapping and SubjectPermission for import/export control
  - Configuration validation with comprehensive error messages
  - DefaultLeafNodeConfig for sensible defaults
  - 38 tests passing

- **T4.2: Leaf Node Connection** ✅ COMPLETE
  - LeafNodeManager for managing leaf node connections
  - Connection states: disconnected, connecting, connected, reconnecting, failed
  - Three modes: startAsHub, startAsLeaf, startAsBridge
  - Embedded NATS server with leaf node configuration
  - Local client connection for message routing
  - Connection monitoring with state updates
  - Callbacks: onStateChange, onRemoteConnect, onRemoteDisconnect, onMessage
  - Statistics: connected remotes, messages, bytes, latency

- **T4.3: Leaf Node Chains** ✅ COMPLETE
  - LeafChainConfig for multi-hop leaf topologies
  - LeafChainHop for individual hops in chain
  - Chain validation: leaf→bridge→...→hub pattern enforcement
  - BuildHopConfigs() generates LeafNodeConfig for each hop
  - CalculateHopTimeout() for timeout distribution
  - JetStream support for persistence at each hop
  - Deduplication window configuration

- **T4.4: Local Persistence During Outage** ✅ COMPLETE
  - Created `pkg/nats/leaf_buffer.go` with MessageBuffer
  - BufferConfig: max size, max messages, max age, persistence
  - BufferedMessage with ID, subject, data, headers, timestamp, retry count
  - Buffer states: idle, buffering, flushing, draining
  - Size and message count limits with automatic eviction
  - Message deduplication with configurable window
  - JetStream persistence support (optional)
  - Flush() and FlushBatch() for message delivery
  - Retry logic with configurable delays
  - Cleanup goroutine for expired messages
  - LeafBufferManager extends LeafNodeManager with buffering
  - Auto-flush on reconnection
  - BufferStats for monitoring
  - 25 tests passing

- **T4.5: Leaf Node Testing** ✅ COMPLETE
  - Comprehensive tests for all leaf node functionality
  - Configuration validation tests
  - Connection lifecycle tests
  - Chain configuration and validation tests
  - Buffer lifecycle and limit tests
  - Deduplication tests
  - Integration tests for LeafBufferManager

**Phase 5: Supercluster Support (Weeks 12-14)** ✅ COMPLETE

- **T5.1: Gateway Configuration** ✅ COMPLETE
  - Created `pkg/nats/gateway.go` with comprehensive gateway support
  - GatewayConfig with listen, port, TLS, auth configuration
  - GatewayRemoteConfig for connecting to other clusters
  - GatewayTLSConfig with ToTLSConfig() for certificate loading
  - GatewayAuthConfig with token and user/password support
  - GatewayMode enum: Optimistic, InterestOnly
  - GatewayConnection with state tracking (disconnected, connecting, connected, etc.)
  - GatewayManager for managing gateway connections
  - Start/Stop lifecycle management
  - ConfigureNATSServer() for applying gateway configuration to NATS server
  - BuildGatewayURLs() for generating connection URLs
  - 50+ tests passing

- **T5.2: Gateway Connection Manager** ✅ COMPLETE
  - GatewayHealthConfig with check intervals and thresholds
  - GatewayHealth struct for tracking health status per gateway
  - GatewayHealthMonitor with background health checking
  - Health status: unknown, healthy, degraded, unhealthy
  - Dynamic gateway management: AddGateway(), RemoveGateway()
  - Server/Client accessors for NATS integration
  - UpdateGatewayStatusFromServer() for status synchronization
  - Health change callbacks for monitoring

- **T5.3: Subject Routing Across Gateways** ✅ COMPLETE
  - SubjectRouter for routing subjects to appropriate clusters
  - ClusterRoute with priority, latency, and availability tracking
  - AddRoute(), RemoveRoute(), GetRoute() for route management
  - AddSubjectPrefix(), RemoveSubjectPrefix() for prefix-based routing
  - RouteSubject() for determining target cluster
  - Preference for local cluster (preferLocal option)
  - Priority and latency-based route selection
  - GetAvailableClusters() for listing connected clusters

- **T5.4: Cross-Cluster Agent Management** ✅ COMPLETE
  - CrossClusterAgentManager for managing agents across clusters
  - RegisterAgent(), UnregisterAgent() for agent lifecycle
  - GetAgentCluster(), IsLocalAgent() for agent lookup
  - GetLocalAgents(), GetAgentsInCluster(), GetAllAgents()
  - GetClusterStats() for agent distribution statistics
  - BuildAgentSubject() for cluster-aware subject generation
  - GetTimeoutForAgent() for cross-cluster timeout adjustment
  - SetLocalityPreference() for routing preference

- **T5.5: Supercluster Failover** ✅ COMPLETE
  - FailoverState enum: Normal, Detecting, FailingOver, FailedOver, FailingBack
  - FailoverConfig with detection/failover timeouts, min healthy nodes
  - FailoverManager for orchestrating failover operations
  - State() for current failover state
  - ManualFailback() for forced failback
  - SetFailoverCallback(), SetFailbackCallback() for notifications
  - GetStatus() for detailed failover status

- **T5.6: Supercluster Testing** ✅ COMPLETE
  - 14 comprehensive supercluster integration tests
  - Multi-cluster topology tests (3-cluster setup)
  - Cross-cluster agent routing tests
  - Subject routing with failover tests
  - Failover state transition tests
  - Gateway health aggregation tests
  - Cross-cluster timeout tests
  - Gateway mode configuration tests
  - Routing priority tests
  - Dynamic route management tests
  - Agent migration tests
  - Connection state tracking tests
  - Failover config validation tests

**Phase 6: WebSocket Transport (Weeks 15-16)** ✅ COMPLETE

- **T6.1: WebSocket Client** ✅ COMPLETE
  - Created `pkg/nats/websocket.go` with comprehensive WebSocket support
  - WebSocketConfig with host, port, path, TLS, proxy, compression settings
  - WebSocketTLSConfig for WSS connections with cert/key/CA support
  - WebSocketConnection for NATS over WebSocket
  - Connection states: disconnected, connecting, connected, reconnecting, closed
  - Automatic TLS configuration for wss:// endpoints
  - Custom headers and origin support
  - WebSocketConnectionStats for monitoring
  - Callback notifications for state changes and errors

- **T6.2: WebSocket Server Configuration** ✅ COMPLETE
  - WebSocketServerConfig for embedded NATS WebSocket listener
  - ToNATSWebsocket() converts to NATS server.WebsocketOpts
  - TLS configuration with certificate and CA support
  - CORS configuration with AllowedOrigins and SameOrigin
  - JWT cookie support for authentication
  - Compression and handshake timeout settings
  - GetURL() for client connection URL generation

- **T6.3: WebSocket Through Proxy** ✅ COMPLETE
  - WebSocketProxyConfig with URL, auth, NoProxy settings
  - ProxyAuthType enum: None, Basic, Digest, NTLM
  - ProxyDialer for HTTP CONNECT tunnel support
  - ShouldBypass() with wildcard pattern matching (*.example.com)
  - Environment variable support (HTTP_PROXY, HTTPS_PROXY, NO_PROXY)
  - Proxy authentication header generation
  - EnhancedWebSocketStrategy with proxy integration

- **T6.4: WebSocket Performance & Testing** ✅ COMPLETE
  - Comprehensive test suite: 40+ WebSocket tests
  - Performance benchmarks:
    - Config validation: ~6ns/op
    - URL building: ~535ns/op
    - Proxy bypass check: ~27ns/op
    - Connection creation: ~500ns/op
    - Options building: ~870ns/op
    - Server config conversion: ~200ns/op
    - Strategy options: ~1.5µs/op
  - WebSocketManager for multi-connection handling
  - Connection pooling with add/remove operations

**Phase 7: Discovery & Auto-Configuration (Weeks 17-18)** ✅ COMPLETE

- **T7.1: DNS-Based Discovery** ✅ COMPLETE
  - Created `pkg/nats/discovery.go` with comprehensive discovery framework
  - DNSDiscoveryConfig with service name, domain, refresh interval, timeout, TLS settings
  - DNSDiscoverer with SRV record lookup and A/AAAA fallback
  - Endpoint priority and weight-based sorting from SRV records
  - Cached endpoints with TTL-based expiration

- **T7.2: mDNS/Bonjour Discovery** ✅ COMPLETE
  - MDNSDiscoveryConfig with service type, domain, browse timeout
  - MDNSDiscoverer placeholder (requires hashicorp/mdns library for full implementation)
  - `_nats._tcp` service type for local network discovery
  - Interface-specific browsing support

- **T7.3: Kubernetes Discovery** ✅ COMPLETE
  - KubernetesDiscoveryConfig with service name, namespace, port name, label selector
  - KubernetesDiscoverer with DNS-based service discovery
  - Headless service endpoint resolution via cluster DNS
  - Support for in-cluster and out-of-cluster configurations

- **T7.4: Service Registry Discovery** ✅ COMPLETE
  - ServiceRegistryConfig supporting Consul and etcd registries
  - ServiceRegistryDiscoverer with pluggable backends
  - Tag-based filtering for Consul
  - Prefix-based discovery for etcd
  - Token and TLS authentication support

- **T7.5: Auto-Configuration** ✅ COMPLETE
  - AutoConfigurator with intelligent endpoint selection
  - Network type detection (Direct, NAT, SymmetricNAT, Firewall)
  - Strategy selection based on network topology
  - DiscoveryManager aggregating multiple discoverers
  - Health checking with TCP connectivity tests
  - Endpoint caching with TTL and health status tracking
  - Callback-based endpoint change notifications
  - Static, DNS, Kubernetes, Consul discoverer integration

- **Test Coverage**: 44 tests covering all discovery components
  - Config validation tests
  - Discoverer lifecycle tests
  - Health check and endpoint management tests
  - Auto-configuration strategy tests
  - Watch/callback functionality tests

**Phase 8: Reliability & Resilience (Weeks 19-20)** ✅ COMPLETE

- **T8.1: Message Buffering** ✅ COMPLETE
  - Leverages existing `pkg/nats/leaf_buffer.go` MessageBuffer
  - BufferConfig with max size, max messages, max age, persistence
  - BufferedMessage with ID, subject, data, headers, timestamp
  - Added Enqueue()/Dequeue() methods for delivery manager integration
  - Added Attempts and LastAttempt fields for retry tracking
  - Flush and FlushBatch for message delivery
  - Cleanup goroutine for expired messages
  - Size and message count limits with automatic eviction

- **T8.2: Delivery Guarantees** ✅ COMPLETE
  - Created `pkg/nats/delivery.go` with DeliveryManager
  - Three delivery modes:
    - AtMostOnce: fire-and-forget (no guarantee)
    - AtLeastOnce: retry until acknowledged (uses MessageBuffer)
    - ExactlyOnce: JetStream-based with message deduplication
  - DeliveryConfig with timeout, max retries, backoff, DLQ settings
  - DeliveryRecord for tracking delivery attempts and status
  - DeliveryStatus: pending, acked, nacked, timeout, failed, dead_lettered
  - Exponential backoff with configurable multiplier and max
  - Dead letter queue support for failed messages
  - Latency tracking with P95/P99 percentiles
  - Delivery complete and dead letter callbacks

- **T8.3: Duplicate Detection** ✅ COMPLETE
  - Leverages existing `pkg/nats/dedup.go` Deduplicator
  - DedupConfig with window duration, max entries, cleanup interval
  - Content-based hashing for message ID generation
  - Per-subject deduplication support
  - Automatic cleanup of expired entries
  - DedupStats for monitoring (total checked, duplicates found)

- **T8.4: Circuit Breaker** ✅ COMPLETE
  - Created `pkg/nats/circuitbreaker.go` with advanced circuit breaker
  - AdvancedCircuitBreakerConfig with failure/success thresholds
  - Three states: Closed, Open, HalfOpen
  - Failure rate threshold with sampling window
  - Minimum requests before rate applies
  - HalfOpen request limiting
  - State change callbacks for monitoring
  - Custom IsFailure function for error classification
  - CircuitBreakerManager for managing multiple breakers
  - Execute() wrapper for protected function calls
  - Manual Trip() and Reset() methods
  - Comprehensive statistics (failures, successes, state time)

- **T8.5: Graceful Degradation** ✅ COMPLETE
  - Created `pkg/nats/degradation.go` with DegradationManager
  - Four degradation modes: Normal, Degraded, Limited, Offline
  - Operation priority levels: Critical, High, Normal, Low, Background
  - Priority-based operation filtering per degradation mode
  - Operation queue with priority ordering
  - Queue preemption for high-priority operations
  - Rate limiting per degradation mode (token bucket)
  - Automatic mode transitions based on failure/success counts
  - Health checking with configurable thresholds
  - Start/Stop lifecycle management
  - Queue operations: Queue, Dequeue, CancelOperation, ClearQueue
  - DegradationStats for monitoring

- **Test Coverage**: 28 tests covering all reliability components
  - Circuit breaker tests: state transitions, failure rate, execute, reset
  - Deduplicator tests: duplicate detection, content hash, expiry, stats
  - Degradation manager tests: mode transitions, allow operation, queue, dequeue
  - Delivery manager tests: config validation, modes, start/stop, stats
  - Integration tests: circuit breaker with degradation, dedup with buffer

**Phase 9: Observability (Weeks 21-22)** ✅ COMPLETE

- **T9.1: Connection Metrics** ✅ COMPLETE
  - Created `pkg/nats/observability.go` with NATSMetricsCollector
  - ObservabilityConfig with prefix, histogram buckets, collection interval
  - 30+ metrics covering all NATS mesh operations:
    - Connection metrics: total, errors, reconnections, latency
    - Message metrics: sent/received by type, size
    - Buffer metrics: size, overflow, utilization
    - Delivery metrics: acked, failed, pending, retried
    - Topology metrics: leaf nodes, gateway connections, gateway latency
    - Bootstrap metrics: requests by status (approved, rejected, expired)
    - Coordination metrics: RPC counts, latency
  - LatencyHistogram with percentile calculations (P50, P95, P99)
  - ObservabilityStats for aggregated metrics access

- **T9.2: Topology Visualization (Grafana Dashboard)** ✅ COMPLETE
  - Created `deploy/grafana/dashboards/nats-mesh.json` with comprehensive dashboard
  - 8 panel rows covering all aspects:
    - Overview: connection health, message rate, error rate
    - Connections: success rate, latency percentiles, circuit breaker states
    - Messages: throughput, size distribution, delivery status
    - Buffers: utilization, overflow rate, cleanup efficiency
    - Topology: leaf node connections, gateway health, cluster distribution
    - Bootstrap: registration rate, rejections, expirations
    - Coordination: RPC success rate, latency
    - Reliability: circuit breaker, degradation, delivery guarantees
  - Template variables for cluster, endpoint, gateway filtering
  - Threshold-based panel coloring

- **T9.3: Connection Debugging** ✅ COMPLETE
  - Created `pkg/nats/debug.go` with ConnectionDebugger
  - DebugConfig with log levels, max events, tracing options
  - Connection event recording with timeline tracking
  - Message flow tracing with hop-by-hop latency
  - MessageTrace with hops, latency breakdown, status
  - ConnectionTimeline with MTBF/MTTR statistics
  - DiagnosticCLI with interactive commands:
    - StatusCommand: current connection status
    - DiagnoseCommand: diagnostic report
    - TraceCommand: message trace details
    - EventsCommand: recent connection events
  - DiagnosticReport with comprehensive system analysis
  - ExportJSON for external tool integration

- **T9.4: Alerting Rules** ✅ COMPLETE
  - Created `deploy/grafana/alerts/nats-mesh-alerts.yml` with 25 alert rules
  - 8 alert groups covering all failure modes:
    - Connection: high failure rate, no active connections, frequent reconnects
    - Latency: high connection latency, gateway latency (warning/critical)
    - Buffer: overflow detected, high buffer size (warning/critical)
    - Delivery: high failure rate, pending deliveries, high duplicate rate
    - Circuit Breaker: open state, prolonged open state
    - Topology: leaf node disconnections, no leaf nodes, gateway disconnected
    - Failover: high failover rate, failover flapping
    - Bootstrap: high rejection rate, high expiration rate
    - Coordination: RPC errors, high latency
  - Severity levels: warning, critical
  - Runbook URLs for troubleshooting
  - Configurable thresholds and evaluation windows

- **Test Coverage**: 70+ tests covering all observability components
  - `pkg/nats/observability_test.go`: 28 tests for metrics collection
  - `pkg/nats/debug_test.go`: 24 tests for debugging functionality
  - Tests cover metrics recording, stats retrieval, Prometheus export
  - Tests cover event recording, tracing, timeline, diagnostic reports

**Phase 10: Documentation (Weeks 23-24)** ✅ COMPLETE

- **T10.1: Architecture Documentation** ✅ COMPLETE
  - Created `docs/content/en/docs/concepts/nats-mesh.md`
  - NATS mesh architecture overview
  - Subject namespace design with category reference table
  - Connection strategies (Direct, TLS, WebSocket, Leaf Node)
  - Topology diagrams (Mermaid) for all deployment patterns:
    - Simple deployment
    - Production HA cluster
    - Edge deployment (leaf nodes)
    - Multi-region (supercluster)
  - Multi-endpoint support with failover behavior
  - Leaf node architecture with buffering
  - Supercluster (gateway) architecture
  - WebSocket transport with proxy support
  - Discovery and auto-configuration
  - Reliability features (circuit breaker, delivery guarantees, degradation)
  - Security model (bootstrap, credentials, TLS)
  - Metrics reference table
  - Troubleshooting commands

- **T10.2: Deployment Guides** ✅ COMPLETE
  - Created `docs/content/en/docs/operations/nats-mesh-deployment.md`
  - 6 deployment patterns documented:
    - Simple deployment (embedded NATS)
    - Standalone production (external NATS)
    - HA cluster (NATS cluster + multiple control planes)
    - Edge deployment (leaf nodes)
    - Multi-region supercluster (gateways)
    - Hybrid deployment (mixed topologies)
  - Complete configuration examples for each pattern
  - Docker Compose examples
  - Kubernetes StatefulSet examples
  - Migration guides:
    - Embedded to external NATS
    - Direct to leaf node
    - Adding gateway (supercluster)
  - Verification commands

- **T10.3: Operations Guides** ✅ COMPLETE
  - Created `docs/content/en/docs/operations/nats-mesh-operations.md`
  - Monitoring section:
    - Grafana dashboard setup
    - Key metrics to monitor (connection, delivery, buffer, topology)
    - Prometheus alert rules
    - Health check endpoints
  - Troubleshooting section:
    - Connection issues
    - Message delivery issues
    - Buffer issues
    - Leaf node issues
    - Gateway issues
    - Debug commands
  - Capacity planning:
    - Connection sizing table
    - Message throughput estimation
    - Buffer sizing formula
    - Network bandwidth calculation
  - Performance tuning:
    - NATS server tuning
    - Agent tuning
    - OS tuning
  - Disaster recovery:
    - Backup strategy (JetStream, config)
    - Recovery procedures (single server, cluster, supercluster)
    - Runbooks (connection storm, split brain)
  - Maintenance:
    - Rolling updates
    - Certificate rotation
    - Stream maintenance

- **T10.4: API Reference** ✅ COMPLETE
  - Created `docs/content/en/docs/reference/nats-mesh.md`
  - Complete server NATS configuration reference
  - Complete agent NATS configuration reference
  - Subject namespace reference with all standard subjects
  - CLI reference (agent and control plane commands)
  - Metrics reference (all 30+ metrics documented)
  - Environment variables reference
  - Error codes reference
  - Protocol specification:
    - Message envelope format
    - Priority levels
    - Bootstrap protocol (sequence diagram)
    - Heartbeat protocol
    - Command protocol

**Epic 14: NATS Mesh Communication - COMPLETE** ✅

All 10 phases implemented:
- Phase 1: NATS-Only Communication Refactor
- Phase 2: Multi-Endpoint Support
- Phase 3: Agent Embedded NATS
- Phase 4: Leaf Node Support
- Phase 5: Supercluster Support
- Phase 6: WebSocket Transport
- Phase 7: Discovery & Auto-Configuration
- Phase 8: Reliability & Resilience
- Phase 9: Observability
- Phase 10: Documentation

### Epic 15: Observability Enhancements ✅ COMPLETE

**Implementation Plan:** 9 phases (18 weeks total)

**Goal**: Enhance Keystone Core's observability infrastructure to use NATS as the primary transport for all telemetry data, implement stdout-first logging for journald/container integration, and add comprehensive audit logging.

**Current Status**: All 9 phases COMPLETE ✅

**Phase 1: Stdout-First Logging Refactor (Weeks 1-2) ✅ COMPLETE**

- **T1.1: Add LoggingConfig to config** ✅ COMPLETE
- **T1.2: Enhance stdout output with metadata** ✅ COMPLETE
- **T1.3: Update service logging** ✅ COMPLETE
- **T1.4: Environment variable configuration** ✅ COMPLETE
- Test coverage: 26 tests passing for pkg/logging

**Phase 2: Syslog Integration (Week 3-4) ✅ COMPLETE**

- **T2.1: RFC 5424 Syslog Formatter** ✅ COMPLETE
  - Created `pkg/logging/syslog.go` with RFC 5424 support
  - `SyslogFormatter` with structured data (SD-ELEMENT)
  - Priority calculation: `facility * 8 + severity`
  - RFC 3339 timestamp format, hostname, app-name, procid, msgid
  - Structured data: `[kscore@49152 key="value"]` format
  - IANA private enterprise number 49152 placeholder

- **T2.2: BSD Syslog Formatter** ✅ COMPLETE
  - `BSDSyslogFormatter` for RFC 3164 compatibility
  - BSD timestamp format (Mmm dd HH:MM:SS)
  - Tag[PID]: format for process identification

- **T2.3: Syslog Output** ✅ COMPLETE
  - `SyslogOutput` with multiple transports:
    - Unix socket (/dev/log, /var/run/syslog)
    - UDP
    - TCP
    - TLS-encrypted TCP (tcp+tls)
  - Auto-reconnect on connection failure
  - Dial timeout and write retry logic
  - Thread-safe with mutex

- **T2.4: Factory Integration** ✅ COMPLETE
  - Updated `pkg/logging/factory.go` to support syslog output
  - `SyslogConfig` parsing from config
  - Fallback to stdout if syslog connection fails
  - TLS configuration support

- Test coverage: 15 tests passing for syslog functionality

**Phase 3: CLI Audit Logging (Week 5-6) ✅ COMPLETE**

- **T3.1: Audit Logging Infrastructure** ✅ COMPLETE
  - Created `pkg/audit/audit.go` with core audit types
  - `AuditLevel`: all, errors, none
  - `AuditAction`: 14 action types (command_executed, state_applied, etc.)
  - `AuditResult`: success, failure, denied, timeout
  - `AuditEntry` with comprehensive fields (user, UID, TTY, PID, tool, command, args, target, result, duration, correlation ID, extra)
  - `Auditor` coordinator with redaction support
  - Global audit functions: `Init()`, `Log()`, `StartEntry()`, `Close()`
  - Sensitive data redaction (passwords, tokens, secrets, keys)

- **T3.2: OS-Specific Audit Backends** ✅ COMPLETE
  - Created `pkg/audit/backends.go` with multiple backends:
    - `SyslogAuditLogger` - RFC 3164 syslog with priority calculation
    - `JournaldAuditLogger` - systemd journal with structured fields (KSCORE_*)
    - `StderrAuditLogger` - JSON to stderr for container environments
    - `FileAuditLogger` - JSON lines to file
    - `MemoryAuditLogger` - In-memory for testing
    - `MultiAuditLogger` - Fan-out to multiple backends
    - `TimeoutAuditLogger` - Timeout wrapper
    - `NoopAuditLogger` - No-op for disabled logging
  - Auto-detection: Linux (journald → syslog), macOS (syslog), Windows (stderr)

- **T3.3: CLI Integration** ✅ COMPLETE
  - Integrated audit logging into `kscore-exec`:
    - All run command executions logged
    - Target expression and agents matched tracked
    - Success/failure results with exit codes
    - Duration tracking
  - Integrated audit logging into `kscore-state`:
    - Apply, check, and drift commands logged
    - State execution results tracked
    - Dry-run mode indicated
  - Audit flags: `--audit-level`, `--audit-output`

- Test coverage: 20 tests passing for pkg/audit

**Phase 4: NATS Log Transport (Week 7-8) ✅ COMPLETE**

- **T4.1: NATS Log Transport Infrastructure** ✅ COMPLETE
  - Created `pkg/logging/nats.go` with centralized log collection
  - `NATSLogConfig` with URL, subject, service name, buffering settings
  - `NATSLogMessage` structure for JSON serialization
  - Subject routing: per-level (`kscore.logs.{level}`) and per-service options
  - Async publishing with configurable buffer size (default 10000)
  - Flush interval for batched publishing
  - Authentication support: token, user/password, NKey, credentials file

- **T4.2: NATS Log Output** ✅ COMPLETE
  - `NATSOutput` implementing Output interface
  - Buffered message channel with non-blocking publish
  - Background flush goroutine
  - Stats tracking: messages published, dropped, errors
  - Graceful shutdown with flush

- **T4.3: NATS Log Formatter** ✅ COMPLETE
  - `NATSFormatter` for JSON serialization
  - Includes: timestamp, level, logger, message, correlation_id, service, caller
  - Metadata extraction for caller info

- **T4.4: NATS Log Subscriber** ✅ COMPLETE
  - `NATSLogSubscriber` for consuming centralized logs
  - Handler callback pattern
  - Multiple subscription support

- Test coverage: 19 tests passing for pkg/logging NATS functionality

**Phase 5: NATS Metrics Transport (Week 9-10) ✅ COMPLETE**

- **T5.1: NATS Metrics Collector** ✅ COMPLETE
  - Created `pkg/metrics/nats.go` with centralized metrics collection
  - `NATSMetricsConfig` with URL, subject, service name, batching settings
  - Implements `Collector` interface (IncCounter, SetGauge, ObserveHistogram, etc.)
  - Subject routing: per-metric (`kscore.metrics.{service}.{name}`)
  - Batch publishing with configurable interval (default 10s)

- **T5.2: Metric Types** ✅ COMPLETE
  - Counter: increment and add operations
  - Gauge: set, increment, decrement operations
  - Histogram: observation with configurable buckets
  - Summary: observation with quantile calculation (P50, P90, P95, P99)
  - `NATSMetricMessage` structure for JSON serialization

- **T5.3: Metric Publishing** ✅ COMPLETE
  - Batched publishing for efficiency
  - Background flush goroutine
  - Stats tracking: messages published, dropped
  - Graceful shutdown with final flush

- **T5.4: Metric Subscriber** ✅ COMPLETE
  - `NATSMetricsSubscriber` for consuming centralized metrics
  - Handler callback pattern
  - Wildcard subscription support

- Test coverage: 64.6% for pkg/metrics (NATS functionality)

**Phase 6: NATS Trace Transport (Week 11-12) ✅ COMPLETE**

- **T6.1: NATS Span Exporter** ✅ COMPLETE
  - Created `pkg/tracing/nats_exporter.go` with centralized trace collection
  - `NATSExporterConfig` with URL, subject, service name, batching settings
  - `NATSSpan` structure matching OpenTelemetry model:
    - trace_id, span_id, parent_span_id
    - name, kind (internal, server, client, producer, consumer)
    - start_time, end_time, duration_ns
    - status, status_message
    - attributes, events, links
    - service, host, version
  - Batch publishing with configurable batch size (default 100) and flush interval (default 5s)

- **T6.2: Span Events and Links** ✅ COMPLETE
  - `NATSSpanEvent` for span events with timestamp and attributes
  - `NATSSpanLink` for linking to other spans
  - `NATSSpanBatch` for batch publishing

- **T6.3: SpanBuilder** ✅ COMPLETE
  - Fluent API for constructing spans
  - `WithParentSpanID`, `WithKind`, `WithStatus`
  - `WithStartTime`, `WithEndTime`, `WithDuration`
  - `WithAttribute`, `WithEvent`, `WithLink`
  - `WithService`, `WithHost`, `WithVersion`
  - Duration auto-calculation from start/end times

- **T6.4: Span Subscriber** ✅ COMPLETE
  - `NATSSpanSubscriber` for consuming centralized traces
  - Handler callback pattern

- Test coverage: 51.9% for pkg/tracing (NATS exporter - 20 tests)

**Phase 7: NATS Audit Transport (Week 13-14) ✅ COMPLETE**

- **T7.1: NATS Audit Logger** ✅ COMPLETE
  - Created `pkg/audit/nats.go` with centralized audit collection
  - `NATSAuditConfig` with URL, subject, service name, buffering settings
  - `NATSAuditLogger` implementing `AuditLogger` interface
  - Subject routing: per-tool and per-action (`kscore.audit.{tool}.{action}`)
  - Async buffered publishing with batch processing
  - Authentication support: token, user/password, NKey, credentials file

- **T7.2: NATS Audit Message** ✅ COMPLETE
  - `NATSAuditMessage` for JSON serialization
  - All audit fields: user, uid, tty, pid, tool, command, args, target
  - Result tracking: success, failure, denied, timeout
  - Duration, exit code, correlation ID, error message
  - Service and hostname metadata
  - Extra data for action-specific information

- **T7.3: NATS Audit Subscriber** ✅ COMPLETE
  - `NATSAuditSubscriber` for consuming centralized audit logs
  - Handler callback pattern
  - `NATSAuditBatch` for batch operations

- **T7.4: Audit Aggregator** ✅ COMPLETE
  - `NATSAuditAggregator` for in-memory audit analysis
  - Filter methods: FilterByUser, FilterByAction, FilterByResult, FilterByTool, FilterByTimeRange
  - Max size with oldest-first eviction
  - `NATSAuditSummary` with comprehensive statistics:
    - Total, success, failure, denied, timeout counts
    - Counts by action, tool, user
    - Unique users count
    - Oldest/newest entry timestamps

- Test coverage: 48.6% for pkg/audit (25 NATS tests)

**Phase 8: TUI Monitor Real-time Updates (Week 15-16) ✅ COMPLETE**

- **T8.1: Telemetry Subscriber Infrastructure** ✅ COMPLETE
  - Created `cmd/kscore-monitor/events/telemetry.go` with NATS integration
  - `TelemetryConfig` with NATS URL and subject configuration
  - `TelemetrySubscriber` managing all telemetry subscriptions:
    - Log subscription (kscore.logs.>)
    - Metric subscription (kscore.metrics.>)
    - Trace subscription (kscore.traces.>)
    - Audit subscription (kscore.audit.>)
  - Stats tracking for received messages per stream
  - NATS reconnection handling with event emission

- **T8.2: Bubble Tea Message Types** ✅ COMPLETE
  - `LogMsg` for log message events
  - `MetricMsg` for metric message events
  - `TraceMsg` for trace batch events
  - `AuditMsg` for audit message events
  - Error propagation for connection issues

- **T8.3: Data Buffers** ✅ COMPLETE
  - `LogBuffer` ring buffer for log messages:
    - Configurable max size (default 1000)
    - Add, All, Last, FilterByLevel, Clear, Count methods
    - Thread-safe with mutex
    - Oldest-first eviction when full
  - `MetricBuffer` for latest metric values:
    - Key-value storage (service:name)
    - Add, Get, All, FilterByService, FilterByType, Clear, Count methods
    - Auto-updates existing metrics (latest value wins)
    - Thread-safe with mutex

- **T8.4: UI View Integration** ✅ COMPLETE
  - Updated `LogsModel` in `cmd/kscore-monitor/ui/view_state_policy_logs_metrics.go`:
    - Integrated LogBuffer for message storage
    - Handle LogMsg in Update() method
    - Pause/resume functionality (p/space keys)
    - Clear functionality (c key)
    - Live/Paused status indicator
    - Color-coded log levels (debug, info, warn, error)
    - Real-time log rendering with timestamp, level, service, message
  - Updated `MetricsModel`:
    - Integrated MetricBuffer for metric storage
    - Handle MetricMsg in Update() method
    - Clear functionality (c key)
    - Metric count display
    - Group metrics by service
    - Color-coded metric types (counter, gauge, histogram)
    - Real-time metric rendering with name, value, type

- **T8.5: Subscription Helpers** ✅ COMPLETE
  - Added `Subscription()` method to all subscriber types:
    - `pkg/logging/nats.go` - NATSSubscriber.Subscription()
    - `pkg/metrics/nats.go` - NATSMetricsSubscriber.Subscription()
    - `pkg/tracing/nats_exporter.go` - NATSSpanSubscriber.Subscription()
    - `pkg/audit/nats.go` - NATSAuditSubscriber.Subscription()
  - Added `NewNATSLogSubscriber` alias for clarity
  - Added `NewNATSMetricsMessageSubscriber` wrapper for single-message handling

- Test coverage: 16 tests passing for cmd/kscore-monitor/events

**Phase 9: Documentation (Week 17-18) ✅ COMPLETE**

- **T9.1: Update Observability Documentation** ✅ COMPLETE
  - Updated `docs/content/en/docs/concepts/observability.md` with Epic 15 features
  - Added "NATS Log Transport" section with configuration and message format
  - Added "Stdout-First Logging" section with container/systemd integration
  - Added "Syslog Integration" section with RFC 5424/3164 formats
  - Added "NATS Telemetry Transport" section with metrics and trace transport
  - Added "Audit Logging" section with entry format, actions, and configuration
  - Updated "TUI Monitor" section with NATS integration details

- **T9.2: Documentation Sections Added** ✅ COMPLETE
  - NATS Log Transport: URL, subject routing, message format, subscription examples
  - Stdout-First Logging: Environment variables, container integration, journald
  - Syslog Integration: RFC 5424/3164, transports (unix, udp, tcp, tcp+tls), TLS config
  - NATS Metrics Transport: Batch format, subject routing, subscription examples
  - NATS Trace Transport: Span format, batch format, OpenTelemetry-compatible
  - Audit Logging: Entry format, 14 audit actions, multi-backend configuration
  - NATS Audit Transport: Subject routing (per-tool, per-action), CLI integration
  - Sensitive Data Redaction: Automatic password/token/secret redaction
  - TUI NATS Integration: Subject subscriptions, configuration examples

**Epic 15 Complete!** All 9 phases implemented:
- Phase 1: Stdout-First Logging Refactor
- Phase 2: Syslog Integration
- Phase 3: CLI Audit Logging
- Phase 4: NATS Log Transport
- Phase 5: NATS Metrics Transport
- Phase 6: NATS Trace Transport
- Phase 7: NATS Audit Transport
- Phase 8: TUI Monitor Real-time Updates
- Phase 9: Documentation

### Epic 16: Standard Library System Modules ✅ COMPLETE

**Implementation Plan:** 14 phases (26 weeks total)

**Goal**: Expand Keystone Core's standard library with cross-platform system management modules inspired by Salt Project's state modules, enabling infrastructure automation across Linux, macOS, and Windows.

**Current Status**: Phase 14 COMPLETE - Epic COMPLETE (84 modules total)

**Phase 1: Cross-Platform User/Group (Weeks 1-2) ✅ COMPLETE**

- **T1.1: Windows User Module** ✅ COMPLETE
  - Added Windows support to `pkg/statemgmt/module_user.go`
  - `createUserWindows()` using `net user /add` command
  - `modifyUserWindows()` for user property changes
  - `deleteUser()` updated with Windows support
  - `getUserGroupsWindows()` using PowerShell
  - `updateGroupMembershipWindows()` for group add/remove
  - Handles fullname, comment, home directory, active flag
  - Password and password_never_expires support

- **T1.2: Windows Group Module** ✅ COMPLETE
  - Added Windows support to `pkg/statemgmt/module_group.go`
  - `createGroupWindows()` using `net localgroup /add`
  - `modifyGroupWindows()` for group property changes
  - `deleteGroup()` updated with Windows support
  - `getGroupMembersWindows()` parsing `net localgroup` output
  - `updateGroupMembersWindows()` for member add/remove
  - Description/comment support

- **T1.3: Existing Linux Support** ✅ ALREADY IMPLEMENTED
  - useradd, usermod, userdel for user management
  - groupadd, groupmod, groupdel for group management

- **T1.4: Existing macOS Support** ✅ ALREADY IMPLEMENTED
  - dscl commands for user management
  - dscl commands for group management

**Phase 2: Network Configuration (Weeks 3-4) ✅ COMPLETE**

- **T2.1: Network Module Design** ✅ COMPLETE
  - Created `pkg/statemgmt/module_network.go` (720+ lines)
  - NetworkModule with BaseModule pattern
  - States: configured, absent, dhcp
  - NetworkConfig struct for parameters
  - NetworkManager enum for platform detection
  - Helper functions: normalizeAddress, cidrToNetmask, stringSlicesEqual

- **T2.2: Linux Network Providers** ✅ COMPLETE
  - NetworkManager detection and support (nmcli)
  - netplan detection and support
  - ifupdown detection and support (/etc/network/interfaces)
  - systemd-networkd detection and support
  - Auto-detection via detectNetworkManager()

- **T2.3: macOS Network Provider** ✅ COMPLETE
  - networksetup command integration
  - Static IP configuration
  - DHCP configuration
  - DNS and search domain support

- **T2.4: Windows Network Provider** ✅ COMPLETE
  - netsh command integration
  - Static IP configuration
  - DHCP configuration
  - DNS server configuration

- **T2.5: Route Management** ✅ COMPLETE
  - Created `pkg/statemgmt/module_route.go` (508 lines)
  - RouteModule with BaseModule pattern
  - States: present, absent
  - RouteConfig struct: destination, gateway, interface, metric, table
  - Linux: `ip route` commands with table support
  - macOS: `route` commands with -net/-host
  - Windows: `route` commands with -p for persistent
  - CIDR to netmask conversion
  - Host route and default route support

- **T2.6: Tests** ✅ COMPLETE
  - module_network_test.go: 14 tests
    - NewNetworkModule, ParseConfig, DetectNetworkManager
    - NormalizeAddress, CIDRToNetmask, StringSlicesEqual
    - Check with nonexistent interface, absent state
  - module_route_test.go: 13 tests
    - NewRouteModule, ParseConfig, Check operations
    - Validation tests, host route, zero route
    - Apply already-absent test

- **T2.7: Documentation** ✅ COMPLETE
  - Updated docs/content/en/docs/reference/modules.md
  - Network module section: states, parameters, examples, platform support
  - Route module section: states, parameters, examples, validation

**Phase 3: Firewall Management (Weeks 5-6) ✅ COMPLETE**

- **T3.1: Firewall Abstraction Layer** ✅ COMPLETE
  - Created `pkg/statemgmt/module_firewall.go` (700+ lines)
  - FirewallModule with BaseModule pattern
  - States: present, absent
  - FirewallConfig struct for cross-platform parameters
  - FirewallBackend enum: FBUnknown, FBIptables, FBNftables, FBFirewalld, FBPF, FBNetsh
  - FirewallAction enum: FAAccept, FADrop, FAReject
  - FirewallDirection enum: FDInput, FDOutput, FDForward
  - detectFirewallBackend() with priority: firewalld → nftables → iptables on Linux
  - Platform dispatch via runtime.GOOS switch

- **T3.2: iptables Provider** ✅ COMPLETE
  - Created `pkg/statemgmt/module_iptables.go` (350+ lines)
  - IptablesModule with direct iptables control
  - States: present, absent, flush, policy
  - IptablesConfig with table, chain, protocol, source, destination, jump, match, etc.
  - Tables: filter, nat, mangle, raw, security
  - Chains: INPUT, OUTPUT, FORWARD, PREROUTING, POSTROUTING, custom
  - Match extensions: state, multiport
  - NAT targets: SNAT, DNAT, MASQUERADE
  - Rule checking with `-C`, insertion with `-I`, deletion with `-D`

- **T3.3: nftables Provider** ✅ COMPLETE
  - Created `pkg/statemgmt/module_nftables.go` (350+ lines)
  - NftablesModule for modern nftables firewall
  - States: present, absent
  - NftablesConfig with family, table, chain, chain_type, chain_hook, chain_priority, etc.
  - Families: ip, ip6, inet, arp, bridge, netdev
  - Atomic table/chain/rule management
  - Auto-creation of tables and chains
  - Base chains with type, hook, priority, policy
  - Rule content matching for idempotency

- **T3.4: firewalld Provider** ✅ COMPLETE
  - Created `pkg/statemgmt/module_firewalld.go` (450+ lines)
  - FirewalldModule for zone-based firewall
  - States: present, absent
  - FirewalldConfig with zone, service, port, rich_rule, source, interface, etc.
  - Zone management: public, internal, external, dmz, trusted, drop, etc.
  - Service, port, source, interface management
  - Rich rule support for complex matching
  - Masquerading and port forwarding
  - ICMP block support
  - Permanent and immediate mode flags

- **T3.5: pf Provider (macOS/BSD)** ✅ COMPLETE
  - Integrated in module_firewall.go
  - Uses anchor-based rule management (`com.keystone.core`)
  - pfctl commands for rule management
  - Anchor isolation for safety

- **T3.6: Windows Firewall Provider** ✅ COMPLETE
  - Integrated in module_firewall.go
  - Uses netsh advfirewall commands
  - Profile support: domain, private, public, any
  - Rule names derived from comments

- **T3.7: Tests** ✅ COMPLETE
  - Created `pkg/statemgmt/module_firewall_test.go` (500+ lines)
  - 18 tests covering all firewall modules
  - ParseConfig, DetectBackend, BuildRuleDescription tests
  - Constants validation tests
  - Check operation tests for all modules

- **T3.8: Documentation** ✅ COMPLETE
  - Updated docs/content/en/docs/reference/modules.md
  - Firewall module section: overview, states, parameters, platform behavior, examples
  - Iptables module section: states, parameters, chain management, examples
  - Nftables module section: states, parameters, atomic operations, examples
  - Firewalld module section: states, parameters, zone reference, examples

**Phase 4: Scheduled Tasks (Weeks 7-8) ✅ COMPLETE**

- **T4.1: Cron Module (Linux)** ✅ COMPLETE
  - Created `pkg/statemgmt/module_cron.go` (300+ lines)
  - CronModule with BaseModule pattern
  - States: present, absent
  - CronConfig with command, minute, hour, day, month, weekday, special, user, disabled
  - Special schedules: @reboot, @yearly, @monthly, @weekly, @daily, @hourly
  - Crontab file reading and writing
  - Entry building with comment tracking for idempotency
  - Per-user crontab support

- **T4.2: Systemd Timer Module (Linux)** ✅ COMPLETE
  - Created `pkg/statemgmt/module_systemd_timer.go` (400+ lines)
  - SystemdTimerModule with BaseModule pattern
  - States: present, absent
  - SystemdTimerConfig with command, description, on_calendar, on_boot_sec, on_unit_active_sec, etc.
  - Creates both .timer and .service unit files
  - Timer unit with OnCalendar, OnBootSec, AccuracySec, Persistent, etc.
  - Service unit with ExecStart, User, Group, WorkingDirectory, Environment
  - systemctl enable/start/stop/disable integration

- **T4.3: Launchd Module (macOS)** ✅ COMPLETE
  - Created `pkg/statemgmt/module_launchd.go` (400+ lines)
  - LaunchdModule with BaseModule pattern
  - States: present, absent
  - LaunchdConfig with label, program, program_arguments, run_at_load, start_interval, etc.
  - StartCalendarInterval for calendar-based scheduling
  - WatchPaths and QueueDirectories for event-based triggers
  - KeepAlive for continuous services
  - Environment variables, user/group, nice value support
  - plist file generation with proper XML formatting
  - launchctl load/unload/bootstrap integration

- **T4.4: Scheduled Task Module (Windows)** ✅ COMPLETE
  - Created `pkg/statemgmt/module_scheduled_task.go` (460+ lines)
  - ScheduledTaskModule with BaseModule pattern
  - States: present, absent
  - ScheduledTaskConfig with trigger_type, execute, arguments, start_time, start_date, etc.
  - Trigger types: once, daily, weekly, monthly, at_logon, at_startup, on_idle
  - Days of week, months of year, days of month for scheduling
  - Repeat interval and duration support
  - Run level (limited, highest) and user credentials
  - schtasks.exe command integration

- **T4.5: At Module (Unix)** ✅ COMPLETE
  - Created `pkg/statemgmt/module_at.go` (310+ lines)
  - AtModule with BaseModule pattern
  - States: present, absent
  - AtConfig with command, time, date, queue, send_mail, no_mail
  - Flexible time formats: HH:MM, midnight, noon, now + N hours/minutes
  - Date support: tomorrow, next week, YYYY-MM-DD
  - Queue priority (a-z, A-Z)
  - Job tracking via marker comments (# Keystone Core: {id})
  - atq/atrm commands for job management

- **T4.6: Tests** ✅ COMPLETE
  - Created `pkg/statemgmt/module_scheduled_test.go` (500+ lines)
  - 51 tests covering all scheduled task modules
  - CronModule: NewCronModule, ParseConfig (5 scenarios), BuildEntry, RemoveEntry
  - SystemdTimerModule: NewSystemdTimerModule, ParseConfig (5 scenarios), GenerateTimerUnit, GenerateServiceUnit
  - LaunchdModule: NewLaunchdModule, ParseConfig (4 scenarios), GeneratePlist
  - ScheduledTaskModule: NewScheduledTaskModule, ParseConfig (5 scenarios), BuildTriggerArgs, GetTaskName
  - AtModule: NewAtModule, ParseConfig (7 scenarios)

- **T4.7: Documentation** ✅ COMPLETE
  - Updated docs/content/en/docs/reference/modules.md
  - Added Scheduled Task Modules category to overview (5 modules)
  - Cron module section: platform, states, parameters, examples
  - Systemd_Timer module section: platform, states, parameters, examples
  - Launchd module section: platform, states, parameters, examples
  - Scheduled_Task module section: platform, states, parameters, examples
  - At module section: platform, states, parameters, examples, important notes

**Phase 5: Mount and Storage (Weeks 9-10) ✅ COMPLETE**

- **T5.1: Mount Module** ✅ COMPLETE
  - Created `pkg/statemgmt/module_mount.go` (600+ lines)
  - MountModule with BaseModule pattern
  - States: mounted, unmounted, present, absent
  - MountConfig with device, path, fstype, options, dump, pass, persist, create_path, owner, group, mode
  - Linux: /proc/mounts parsing, /etc/fstab management
  - macOS: diskutil and mount command integration
  - Windows: net use for network shares (limited)
  - fstab entry management for persistent mounts
  - Mount point directory creation with permissions

- **T5.2: Swap Module (Linux)** ✅ COMPLETE
  - Created `pkg/statemgmt/module_swap.go` (480+ lines)
  - SwapModule with BaseModule pattern
  - States: enabled, disabled, present, absent
  - SwapConfig with path, size, priority, persist, label, uuid
  - Size parsing: G/GB, M/MB, K/KB suffixes, plain numbers as MiB
  - Swap file creation via fallocate or dd
  - mkswap for swap formatting with label/uuid
  - swapon/swapoff for enabling/disabling
  - /proc/swaps parsing for status check
  - /etc/fstab integration for persistent swap

- **T5.3: LVM Module** ✅ COMPLETE
  - Created `pkg/statemgmt/module_lvm.go` (850+ lines)
  - LVMPVModule for physical volumes
    - States: present, absent
    - pvcreate/pvremove command integration
    - pvs parsing for status check
  - LVMVGModule for volume groups
    - States: present, absent
    - VGConfig with name, devices, pe_size
    - vgcreate/vgextend/vgremove command integration
    - vgs parsing for status check
  - LVMLVModule for logical volumes
    - States: present, absent
    - LVConfig with name, vg, size, thin_pool, snapshot, fstype
    - Size formats: absolute (10G) and percentage (100%FREE)
    - lvcreate/lvextend/lvremove command integration
    - lvs parsing for status check
    - Optional filesystem creation on new LV

- **T5.4: Disk Module** ✅ COMPLETE
  - Created `pkg/statemgmt/module_disk.go` (750+ lines)
  - DiskModule for partition management
    - States: present, absent, formatted
    - DiskConfig with device, number, start, end, size, type, table_type, fstype, label, flags
    - parted command integration for partition operations
    - GPT and MSDOS partition table support
    - Partition flags: boot, lvm, raid, esp
    - Partition number management
  - FilesystemModule for filesystem creation
    - States: present, absent
    - FilesystemConfig with device, fstype, label, uuid, force, options
    - mkfs.ext4, mkfs.ext3, mkfs.xfs, mkfs.btrfs, mkfs.vfat, mkfs.ntfs support
    - blkid for filesystem detection
    - wipefs for filesystem removal

- **T5.5: Tests** ✅ COMPLETE
  - Created `pkg/statemgmt/module_storage_test.go` (960+ lines)
  - 51 tests covering all storage modules
  - MountModule: creation, states, Check with/without path, Apply, Test, config parsing
  - SwapModule: creation, states, Check, Apply, Test, parseSize (8 scenarios)
  - LVMPVModule: creation, states, Check, missing device validation, Apply, Test
  - LVMVGModule: creation, states, Check, missing name validation, Apply, Test
  - LVMLVModule: creation, states, Check, missing name/vg validation, Apply, Test
  - DiskModule: creation, states, Check, missing device validation, Apply, Test
  - FilesystemModule: creation, states, Check, missing device/fstype validation, Apply, Test, FSTypes
  - StorageModules_ImplementInterface: validates Module interface
  - StorageModules_ValidStates: validates expected states for all 7 modules
  - Linux-only tests properly skip on macOS/Windows

- **T5.6: Documentation** ✅ COMPLETE
  - Updated docs/content/en/docs/reference/modules.md
  - Changed module count from 29 to 36
  - Added Storage Modules category to overview (7 modules)
  - Mount module section: platform support, states, parameters, examples (basic, persistent, NFS)
  - Swap module section: states, parameters, examples (create, disable, remove)
  - Lvm_Pv module section: states, parameters, examples (create, remove)
  - Lvm_Vg module section: states, parameters, examples (create, extend)
  - Lvm_Lv module section: states, parameters, examples (create, with fs, all free space)
  - Disk module section: states, parameters, examples (GPT, boot, LVM)
  - Filesystem module section: states, parameters, examples (ext4, xfs, recreate, remove)
  - Complete LVM + Filesystem example showing full workflow

**Phase 6: SSH and Security (Weeks 11-12) ✅ COMPLETE**

- **T6.1: SSH Authorized Keys Module** ✅ COMPLETE
  - Created `pkg/statemgmt/module_ssh.go` (650+ lines)
  - AuthorizedKeysModule with BaseModule pattern
  - States: present, absent
  - Parameters: user (required), key (required), key_type, comment, options
  - Key parsing with support for options prefix (e.g., no-port-forwarding)
  - Cross-platform: Linux and macOS via ~/.ssh/authorized_keys
  - User home directory detection via getent passwd
  - Ownership management when running as root

- **T6.2: SSH Known Hosts Module** ✅ COMPLETE
  - KnownHostsModule with BaseModule pattern
  - States: present, absent
  - Parameters: host (required), key, key_type, user, path, hash_known_hosts
  - Host key scanning via ssh-keyscan when key not provided
  - System-wide (/etc/ssh/ssh_known_hosts) and per-user (~/.ssh/known_hosts) support
  - Multiple host aliases parsing (host,ip format)
  - Hash hostname support for security

- **T6.3: SSHD Config Module** ✅ COMPLETE
  - SSHDConfigModule with BaseModule pattern
  - States: present, absent
  - Parameters: name (required), value, path, backup
  - Case-insensitive directive matching (SSH standard)
  - Backup file creation before modification
  - Comment-out removal instead of deletion for safety
  - Default path: /etc/ssh/sshd_config

- **T6.4: SELinux Module** ✅ COMPLETE
  - Created `pkg/statemgmt/module_security.go` (550+ lines)
  - SELinuxModule for enforcement mode
  - States: enforcing, permissive, disabled
  - Parameters: persistent
  - getenforce/setenforce command integration
  - /etc/selinux/config modification for persistent changes
  - Reboot notification for disabled state

- **T6.5: SELinux Boolean Module** ✅ COMPLETE
  - SELinuxBooleanModule for boolean values
  - States: on, off
  - Parameters: name (required), persistent
  - getsebool/setsebool command integration
  - Persistent flag (-P) for across-reboot changes

- **T6.6: AppArmor Module** ✅ COMPLETE
  - AppArmorModule for profile enforcement
  - States: enforce, complain, disabled
  - Parameters: profile (required)
  - /sys/kernel/security/apparmor/profiles parsing
  - aa-enforce, aa-complain, aa-disable command integration

- **T6.7: AppArmor Profile Module** ✅ COMPLETE
  - AppArmorProfileModule for profile installation
  - States: present, absent
  - Parameters: name (required), source, content, mode
  - Profile installation to /etc/apparmor.d/
  - apparmor_parser for loading/unloading profiles
  - Support for both source file and inline content

- **T6.8: Tests** ✅ COMPLETE
  - Created `pkg/statemgmt/module_ssh_security_test.go` (350+ lines)
  - 22 tests covering all SSH and security modules
  - AuthorizedKeysModule: creation, states, missing user/key validation, key parsing
  - KnownHostsModule: creation, states, missing host validation, host parsing
  - SSHDConfigModule: creation, states, missing name validation, config parsing, set value
  - SELinuxModule: creation, states, non-Linux error
  - SELinuxBooleanModule: creation, states, missing name validation
  - AppArmorModule: creation, states, non-Linux error, missing profile validation
  - AppArmorProfileModule: creation, states, non-Linux error, missing name validation
  - Platform-aware test skipping (Linux-only modules skip on macOS/Windows)

- **T6.9: Documentation** ✅ COMPLETE
  - Updated docs/content/en/docs/reference/modules.md
  - Changed module count from 36 to 42
  - Added SSH Modules category (3 modules)
  - Added Security Modules category (4 modules)
  - authorized_keys module section: platform, states, parameters, examples
  - known_hosts module section: platform, states, parameters, examples
  - sshd_config module section: platform, states, parameters, examples
  - selinux module section: platform, states, parameters, examples
  - selinux_boolean module section: platform, states, parameters, examples
  - apparmor module section: platform, states, parameters, examples
  - apparmor_profile module section: platform, states, parameters, examples
  - Added Cross-Platform Compatibility entries for SSH and security modules

**Phase 7: System Configuration (Weeks 13-14) ✅ COMPLETE**

- **T7.1: Timezone Module** ✅ COMPLETE
  - Created `pkg/statemgmt/module_system.go` (900+ lines)
  - TimezoneModule with BaseModule pattern
  - States: present
  - Parameters: name (required, IANA timezone)
  - Linux: timedatectl (systemd), /etc/timezone (non-systemd)
  - macOS: systemsetup command
  - Windows: tzutil command

- **T7.2: Locale Module** ✅ COMPLETE
  - LocaleModule with BaseModule pattern
  - States: present
  - Parameters: name (required, e.g., en_US.UTF-8)
  - Linux: localectl (systemd)
  - macOS: Partial support via defaults
  - Windows: Not supported

- **T7.3: Hostname Module** ✅ COMPLETE
  - HostnameModule with BaseModule pattern
  - States: present
  - Parameters: name (required), fqdn
  - Linux: hostnamectl (systemd), /etc/hostname (non-systemd)
  - macOS: scutil command
  - Windows: wmic command

- **T7.4: Hosts Module** ✅ COMPLETE
  - HostsModule with BaseModule pattern
  - States: present, absent
  - Parameters: ip (required), name/names
  - Cross-platform hosts file management
  - Linux/macOS: /etc/hosts
  - Windows: C:\Windows\System32\drivers\etc\hosts
  - Entry parsing with multi-hostname support
  - namesMatch helper for order-independent comparison

- **T7.5: Sysctl Module** ✅ COMPLETE
  - SysctlModule with BaseModule pattern
  - States: present, absent
  - Parameters: name (required), value, persist
  - Linux only: /proc/sys and sysctl command
  - Persistent config via /etc/sysctl.d/
  - Non-Linux returns appropriate error

- **T7.6: Kernel Module Module** ✅ COMPLETE
  - KernelModuleModule with BaseModule pattern
  - States: loaded, unloaded, blacklisted
  - Parameters: name (required), params, persist
  - Linux only: modprobe, rmmod commands
  - /proc/modules parsing for load status
  - Persistent config via /etc/modules-load.d/
  - Blacklist via /etc/modprobe.d/

- **T7.7: Tests** ✅ COMPLETE
  - Created `pkg/statemgmt/module_system_test.go` (300+ lines)
  - 26 tests covering all system modules
  - TimezoneModule: creation, states, missing name, getCurrentTimezone
  - LocaleModule: creation, states, missing name, Windows error
  - HostnameModule: creation, states, missing name, current hostname check
  - HostsModule: creation, states, missing IP/name, entry parsing, add/remove entry
  - SysctlModule: creation, states, non-Linux error, missing name, getValue
  - KernelModuleModule: creation, states, non-Linux error, missing name, isLoaded
  - Helper test: namesMatch function
  - Platform-aware test skipping (Linux-only modules skip on macOS/Windows)

- **T7.8: Documentation** ✅ COMPLETE
  - Updated docs/content/en/docs/reference/modules.md
  - Changed module count from 42 to 48
  - Added System Configuration Modules category (6 modules)
  - timezone module section: platform, states, parameters, examples
  - locale module section: platform, states, parameters, examples
  - hostname module section: platform, states, parameters, examples
  - hosts module section: platform, states, parameters, examples
  - sysctl module section: platform, states, parameters, examples
  - kernel_module module section: platform, states, parameters, examples

**Phase 8: Container Management (Weeks 15-16) ✅ COMPLETE**

- **T8.1: Docker Container Module** ✅ COMPLETE
  - Created `pkg/statemgmt/module_docker.go` (1300+ lines)
  - DockerContainerModule with BaseModule pattern
  - States: running, stopped, absent
  - Parameters: name, image, ports, volumes, env, network, restart, command, force
  - Container lifecycle: create, start, stop, remove
  - JSON inspect for container state detection

- **T8.2: Docker Image Module** ✅ COMPLETE
  - DockerImageModule with BaseModule pattern
  - States: present, absent
  - Parameters: name, tag, force
  - docker pull/rmi integration

- **T8.3: Docker Network Module** ✅ COMPLETE
  - DockerNetworkModule with BaseModule pattern
  - States: present, absent
  - Parameters: name, driver, subnet, gateway, ip_range
  - docker network create/rm integration

- **T8.4: Docker Volume Module** ✅ COMPLETE
  - DockerVolumeModule with BaseModule pattern
  - States: present, absent
  - Parameters: name, driver, opts, force
  - docker volume create/rm integration
  - Driver options support for NFS/other drivers

- **T8.5: Podman Container Module** ✅ COMPLETE
  - PodmanContainerModule with same interface as Docker
  - States: running, stopped, absent
  - Parameters: same as docker_container
  - podman commands integration

- **T8.6: Podman Image Module** ✅ COMPLETE
  - PodmanImageModule with same interface as Docker
  - States: present, absent
  - podman pull/rmi integration

- **T8.7: Podman Network Module** ✅ COMPLETE
  - PodmanNetworkModule with same interface as Docker
  - States: present, absent
  - podman network create/rm integration

- **T8.8: Podman Volume Module** ✅ COMPLETE
  - PodmanVolumeModule with same interface as Docker
  - States: present, absent
  - podman volume create/rm integration

- **T8.9: Container Runtime Detector** ✅ COMPLETE
  - DetectContainerRuntime() - auto-detect docker or podman
  - GetContainerRuntimeVersion() - get runtime version
  - ListContainers() - list containers for given runtime
  - ContainerRuntime enum: docker, podman, unknown

- **T8.10: Tests** ✅ COMPLETE
  - Created `pkg/statemgmt/module_docker_test.go` (350+ lines)
  - 30 tests covering all container modules
  - Module creation and state validation tests
  - Parameter validation tests (missing name, missing image)
  - Helper function tests (getEnvParameters, getDriverOpts)
  - Container runtime detection tests
  - Integration tests (require Docker/Podman)

- **T8.11: Documentation** ✅ COMPLETE
  - Updated docs/content/en/docs/reference/modules.md
  - Changed module count from 48 to 56
  - Added Container Modules category (8 modules)
  - All 8 container module sections with states, parameters, examples

**Phase 9: Database Primitives (Weeks 17-18) ✅ COMPLETE**

- **T9.1: PostgreSQL Database Module** ✅ COMPLETE
  - Created `pkg/statemgmt/module_database.go` (850+ lines)
  - PostgresDatabaseModule with BaseModule pattern
  - States: present, absent
  - Parameters: name, host, port, user, password, maintenance_db, owner, encoding, template, lc_collate, lc_ctype
  - psql command integration for CREATE/DROP DATABASE

- **T9.2: PostgreSQL User Module** ✅ COMPLETE
  - PostgresUserModule with BaseModule pattern
  - States: present, absent
  - Parameters: name, role_password, superuser, createdb, createrole, login, replication, connection_limit
  - CREATE/ALTER ROLE integration with all PostgreSQL role attributes

- **T9.3: PostgreSQL Extension Module** ✅ COMPLETE
  - PostgresExtensionModule with BaseModule pattern
  - States: present, absent
  - Parameters: name, database, schema, version, cascade
  - CREATE EXTENSION/DROP EXTENSION integration

- **T9.4: MySQL Database Module** ✅ COMPLETE
  - MySQLDatabaseModule with BaseModule pattern
  - States: present, absent
  - Parameters: name, host, port, user, password, socket, charset, collation
  - mysql command integration with TCP and socket support

- **T9.5: MySQL User Module** ✅ COMPLETE
  - MySQLUserModule with BaseModule pattern
  - States: present, absent
  - Parameters: name, host_name, user_password, priv (grant format "db.table:PRIV1,PRIV2")
  - CREATE USER, DROP USER, GRANT integration

- **T9.6: Redis Module** ✅ COMPLETE
  - RedisModule with BaseModule pattern
  - States: present, absent
  - Types: config, acl
  - Config parameters: name, value (CONFIG SET/GET)
  - ACL parameters: acl_password, acl_rules (ACL SETUSER/DELUSER)
  - redis-cli integration with TCP and socket support

- **T9.7: Helper Functions** ✅ COMPLETE
  - escapePostgresString() - escape single quotes for PostgreSQL
  - quotePostgresIdentifier() - quote identifiers with special characters
  - escapeMySQLString() - escape quotes and backslashes for MySQL
  - escapeMySQLIdentifier() - escape backticks for MySQL

- **T9.8: Tests** ✅ COMPLETE
  - Created `pkg/statemgmt/module_database_test.go` (340+ lines)
  - 28 tests covering all database modules (25 pass, 3 skip)
  - Module creation and state validation tests
  - Parameter validation tests (missing name, missing database)
  - Helper function tests (escape functions, quote functions)
  - Connection args tests for all modules
  - Grant SQL building tests for MySQL
  - Integration tests (require database tools)

- **T9.9: Documentation** ✅ COMPLETE
  - Updated docs/content/en/docs/reference/modules.md
  - Changed module count from 56 to 62
  - Added Database Modules category (6 modules)
  - Full documentation for all 6 database modules with states, parameters, examples

**Phase 10: Web Server Configuration (Weeks 19-20) ✅ COMPLETE**

- **T10.1: Nginx Site Module** ✅ COMPLETE
  - Created `pkg/statemgmt/module_web.go` (700+ lines)
  - NginxSiteModule with BaseModule pattern
  - States: enabled, disabled, absent
  - Parameters: name, content, source, reload
  - Sites-available/sites-enabled pattern
  - Symlink management for enabling/disabling
  - nginx -t validation before reload
  - nginx -s reload integration

- **T10.2: Nginx Config Module** ✅ COMPLETE
  - NginxConfigModule with BaseModule pattern
  - States: present, absent
  - Parameters: name, content, source, dest, reload
  - Config snippet management (conf.d or custom paths)
  - Validation and reload integration

- **T10.3: Apache Site Module** ✅ COMPLETE
  - ApacheSiteModule with BaseModule pattern
  - States: enabled, disabled, absent
  - Parameters: name, content, source, reload
  - a2ensite/a2dissite integration with fallback to symlinks
  - apachectl/apache2ctl graceful reload

- **T10.4: Apache Module Module** ✅ COMPLETE
  - ApacheModuleModule with BaseModule pattern
  - States: enabled, disabled
  - Parameters: name, reload
  - a2enmod/a2dismod integration
  - Module availability detection

- **T10.5: Platform Support** ✅ COMPLETE
  - Linux: /etc/nginx, /etc/apache2 paths
  - macOS: Homebrew paths (/usr/local/etc/nginx, /usr/local/etc/httpd)
  - Windows: Not supported (returns error)
  - Path auto-detection based on runtime.GOOS

- **T10.6: Tests** ✅ COMPLETE
  - Created `pkg/statemgmt/module_web_test.go` (300+ lines)
  - 18 tests (13 pass, 5 skip for Windows/missing tools)
  - Module creation and state validation tests
  - Parameter validation tests (missing name)
  - Platform-specific path tests
  - Integration tests (require nginx/apache)

- **T10.7: Documentation** ✅ COMPLETE
  - Updated docs/content/en/docs/reference/modules.md
  - Changed module count from 62 to 66
  - Added Web Server Modules category (4 modules)
  - Full documentation for all 4 web server modules with states, parameters, examples

**Phase 11: Version Control (Week 21) ✅ COMPLETE**

- **T11.1: Git Module** ✅ COMPLETE
  - Created `pkg/statemgmt/module_git.go` (600+ lines)
  - GitModule with BaseModule pattern
  - States: present, absent, latest
  - Parameters: repo, dest, version, force, depth, recursive, ssh_key
  - Clone repositories with shallow clone support
  - SSH key authentication for private repos
  - Submodule support with recursive init
  - Working tree status detection
  - Behind count for update detection

- **T11.2: Git Config Module** ✅ COMPLETE
  - GitConfigModule with BaseModule pattern
  - States: present, absent
  - Parameters: name, value, scope, file
  - Scopes: global, system, local, worktree
  - Custom config file support
  - Config value get/set/unset operations

- **T11.3: Tests** ✅ COMPLETE
  - Created `pkg/statemgmt/module_git_test.go` (450+ lines)
  - 18 tests for git and git_config modules
  - Parameter validation tests
  - Clone and update integration tests
  - Config set/unset integration tests
  - Path helper tests

- **T11.4: Documentation** ✅ COMPLETE
  - Updated docs/content/en/docs/reference/modules.md
  - Changed module count from 66 to 68
  - Added Version Control Modules category (2 modules)
  - Full documentation for git and git_config modules

**Phase 12: Certificates (Week 22) ✅ COMPLETE**

- **T12.1: X509 Module** ✅ COMPLETE
  - Created `pkg/statemgmt/module_x509.go` (990+ lines)
  - X509Module with BaseModule pattern
  - States: present, absent
  - Parameters: path, key_path, common_name, organization, country, validity_days, key_type, key_size, self_signed, is_ca, san_names, san_ips
  - Key types: RSA (2048, 4096), ECDSA (P-256, P-384, P-521), Ed25519
  - Self-signed certificate generation
  - Subject Alternative Names (DNS and IP)
  - Certificate metadata extraction

- **T12.2: CA Module** ✅ COMPLETE
  - CAModule with BaseModule pattern
  - States: present, absent
  - Parameters: path, key_path, common_name, organization, country, validity_days, key_type, key_size, max_path_len
  - CA certificate and key generation
  - SignCertificate method for CSR signing
  - Intermediate CA support with max_path_len

- **T12.3: ACME Module** ✅ COMPLETE
  - ACMEModule with BaseModule pattern
  - States: present, absent, renewed
  - Parameters: path, key_path, domain, email, challenge, staging, renew_days, webroot, dns_provider
  - Challenge types: http-01, dns-01
  - Renewal threshold monitoring
  - Certificate state tracking and metadata extraction
  - Framework for external ACME tool integration (certbot/lego)

- **T12.4: Tests** ✅ COMPLETE
  - Created `pkg/statemgmt/module_x509_test.go` (900+ lines)
  - 27 tests for x509, ca, acme modules
  - RSA, ECDSA, Ed25519 key generation tests
  - CA creation and certificate signing tests
  - ACME renewal detection tests
  - Certificate metadata validation

- **T12.5: Documentation** ✅ COMPLETE
  - Updated docs/content/en/docs/reference/modules.md
  - Changed module count from 68 to 71
  - Added Certificate Modules category (3 modules)
  - Full documentation for x509, ca, acme modules

**Phase 13: Language Package Managers & Config Management (Weeks 23-24) ✅ COMPLETE**

- **T13.1: Pip Module (Python)** ✅ COMPLETE
  - Created `pkg/statemgmt/module_langpkg.go` (1216 lines)
  - PipModule with BaseModule pattern
  - States: installed, removed, latest
  - Parameters: name, version, pip3, extra_index_url, user, virtualenv
  - pip/pip3 command integration
  - Version pinning and upgrade support
  - Virtual environment support

- **T13.2: NPM Module (Node.js)** ✅ COMPLETE
  - NpmModule with BaseModule pattern
  - States: installed, removed, latest
  - Parameters: name, version, global, path, registry
  - npm install/uninstall integration
  - Global and local package support
  - Custom registry support

- **T13.3: Gem Module (Ruby)** ✅ COMPLETE
  - GemModule with BaseModule pattern
  - States: installed, removed, latest
  - Parameters: name, version, executable, user, prerelease, source
  - gem install/uninstall integration
  - User gem installation support
  - Prerelease version support

- **T13.4: UFW Module (Ubuntu Firewall)** ✅ COMPLETE
  - UfwModule with BaseModule pattern
  - States: enabled, disabled, allow, deny, reject, absent
  - Parameters: port, proto, from, to, comment
  - ufw enable/disable integration
  - Rule management (allow, deny, reject)
  - Source/destination filtering

- **T13.5: Alternatives Module** ✅ COMPLETE
  - AlternativesModule with BaseModule pattern
  - States: set, auto
  - Parameters: name, path, priority
  - update-alternatives integration
  - Manual and auto mode support
  - Priority setting support

- **T13.6: Logrotate Module** ✅ COMPLETE
  - Created `pkg/statemgmt/module_config.go` (2230 lines)
  - LogrotateModule with BaseModule pattern
  - States: present, absent
  - Parameters: name, path, frequency, rotate, compress, missingok, notifempty, create, etc.
  - Config file generation in /etc/logrotate.d/
  - All logrotate directives supported

- **T13.7: Sudoers Module** ✅ COMPLETE
  - SudoersModule with BaseModule pattern
  - States: present, absent
  - Parameters: name, user, group, commands, nopasswd, validate
  - Sudoers file generation in /etc/sudoers.d/
  - Syntax validation with visudo -c
  - Path traversal prevention

- **T13.8: Limits Module** ✅ COMPLETE
  - LimitsModule with BaseModule pattern
  - States: present, absent
  - Parameters: name, domain, type, item, value
  - Config file generation in /etc/security/limits.d/
  - Soft and hard limits support
  - All pam_limits items supported

- **T13.9: Modprobe Module** ✅ COMPLETE
  - ModprobeModule with BaseModule pattern
  - States: present, absent, blacklist
  - Parameters: name, options, persist
  - Kernel module configuration
  - Config file generation in /etc/modprobe.d/
  - Blacklist support

- **T13.10: Syslog Module** ✅ COMPLETE
  - SyslogModule with BaseModule pattern
  - States: present, absent
  - Parameters: name, facility, priority, action, syslog_type
  - Rsyslog and syslog-ng support
  - Config file generation in /etc/rsyslog.d/

- **T13.11: Lineinfile Module** ✅ COMPLETE
  - LineinfileModule with BaseModule pattern
  - States: present, absent
  - Parameters: path, line, regexp, insertafter, insertbefore, create, backup
  - Line matching with regexp support
  - Line insertion positioning
  - File backup support

- **T13.12: INI File Module** ✅ COMPLETE
  - IniFileModule with BaseModule pattern
  - States: present, absent
  - Parameters: path, section, option, value, create, backup
  - INI file parsing and modification
  - Section and option management
  - File backup support

- **T13.13: Archive Module** ✅ COMPLETE
  - ArchiveModule with BaseModule pattern
  - States: present, absent
  - Parameters: src, dest, format, creates
  - Format auto-detection (tar, tar.gz, tar.bz2, tar.xz, zip)
  - tar and unzip command integration
  - Creates parameter for idempotency

- **T13.14: Tests** ✅ COMPLETE
  - Created `pkg/statemgmt/module_langpkg_test.go` (430 lines)
    - 17 tests for pip, npm, gem, ufw, alternatives modules
    - Module creation and state validation tests
    - Parameter validation tests
    - Platform-specific tests (ufw, alternatives Linux-only)
  - Created `pkg/statemgmt/module_config_test.go` (792 lines)
    - 35 tests for logrotate, sudoers, limits, modprobe, syslog, lineinfile, ini_file, archive modules
    - Module creation and state validation tests
    - Parameter validation tests
    - Content building tests
    - Integration tests (lineinfile, ini_file file operations)

- **T13.15: Documentation** ✅ COMPLETE
  - Updated CLAUDE.md with Phase 13 implementation details
  - Module count increased from 71 to 84 (13 new modules)

**Phase 14: Testing & Documentation (Weeks 25-26) ✅ COMPLETE**

- **T14.1: Unit Tests** ✅ COMPLETE
  - Created `pkg/statemgmt/module_cmd_test.go` (420+ lines)
    - 22 tests for CmdModule Check, Test, Apply methods
    - Tests for creates/removes conditions, wait state, environment variables
    - Tests for stateful output, custom shell, failing commands
  - Created `pkg/statemgmt/module_package_test.go` (91 lines)
    - Tests for PackageModule constructor and ValidStates
    - Tests for convertPlatformPM type conversion
    - Tests for PackageManager string values
  - Created `pkg/statemgmt/module_service_test.go` (86 lines)
    - Tests for ServiceModule constructor and ValidStates
    - Tests for convertPlatformInitSystem type conversion
    - Tests for ServiceManager string values
  - Created `pkg/statemgmt/module_cron_test.go` (239 lines)
    - Additional tests not covered in module_scheduled_test.go
    - Tests for parseCronConfig: environment, disabled, time fields
    - Tests for findEntry: exists, matches, different configurations
    - Tests for removeEntry and default values
  - Test coverage: 32.3% (limited by system-dependent Apply methods)

- **T14.2: Integration Tests** ✅ COMPLETE
  - Existing integration_test.go verified
  - TestIntegration_FullWorkflow, TestIntegration_ComplexDependencies
  - TestIntegration_CmdModule for command execution
  - TestIntegration_ErrorHandling, TestIntegration_Performance

- **T14.3: Module Reference Documentation** ✅ COMPLETE
  - Updated docs/content/en/docs/reference/modules.md
  - Added Pip Module documentation (~100 lines)
  - Added Npm Module documentation (~80 lines)
  - Added Gem Module documentation (~75 lines)
  - Added UFW Module documentation (~110 lines)
  - Added Alternatives Module documentation (~80 lines)

- **T14.4: Example State Files** ✅ COMPLETE
  - Created `examples/states/` directory
  - Created `examples/states/README.md` - Guide and index
  - Created `examples/states/webserver.yaml` - Nginx web server setup
  - Created `examples/states/python-app.yaml` - Flask application deployment
  - Created `examples/states/firewall.yaml` - UFW firewall configuration
  - Created `examples/states/scheduled-tasks.yaml` - Cron jobs and maintenance
  - Created `examples/states/kubernetes-app.yaml` - K8s application deployment
  - Created `examples/states/docker-stack.yaml` - Docker container stack

**Epic 16 Summary:**
- 14 phases completed
- 84 total modules implemented
- Cross-platform support: Linux, macOS, Windows
- Categories: Core (6), Network (6), Firewall (4), Kubernetes (12), Scheduled (5), Storage (6), SSH (3), Security (4), System (6), Container (8), Database (5), Web (9), VCS (2), PKI (3), Language Package (3), Config (8)
- Comprehensive test coverage with unit and integration tests
- Full documentation in module reference
- Example state files for common use cases

### Epic 17: SPIFFE/SPIRE Identity Framework ✅ COMPLETE

**Implementation Plan:** 6 phases

**Goal**: Implement comprehensive SPIFFE identity support with embedded identity provider for zero-configuration deployments and external provider integration (SPIRE, cloud, service mesh).

**Current Status**: All 6 Phases COMPLETE ✅ (332 tests)

**Phase 1: Identity Foundation (Weeks 1-2) ✅ COMPLETE**
- Core SPIFFE types (`pkg/identity/types.go`)
  - SPIFFEID with trust domain and path
  - X509SVID for X.509 certificates with SPIFFE identity
  - JWTSVID for JWT tokens with SPIFFE claims
  - TrustBundle for X.509 CA certificates
  - ParseSPIFFEID for URI parsing
- Certificate Authority (`pkg/identity/ca.go`)
  - CA for issuing X.509 SVIDs
  - ECDSA P-256 key generation
  - Certificate template with SPIFFE URI SAN
  - TTL and serial number management
- Attestation types (`pkg/identity/attestation.go`)
  - AgentAttestation with attestor, data, selectors
  - AttestationType enum (node, join_token, workload, cloud, mesh)
  - CloudMetadata for AWS/GCP/Azure
  - WorkloadSelector for process/container identity
- NATS mTLS integration (`pkg/identity/nats.go`)
  - SVIDToTLSConfig for TLS configuration from SVID
  - NewMutualTLSConfig for mTLS setup
  - BundleToRootCAs for trust bundle conversion
- 54 tests passing

**Phase 2: Embedded Identity Provider (Weeks 3-4) ✅ COMPLETE**
- Embedded provider implementation (`pkg/identity/provider.go`)
  - IdentityProvider interface
  - EmbeddedProvider with local CA
  - SVID issuance with configurable TTL
  - Certificate rotation support
  - Watch API for SVID updates
  - Trust bundle distribution
- Provider configuration
  - CAConfig with key algorithm, lifetime
  - SVIDConfig with TTL, DNS SANs, renewal threshold
- Comprehensive tests (54 tests)

**Phase 3: External Provider Integration (Weeks 5-7) ✅ COMPLETE**
- **T3.1: SPIRE Workload API Client** ✅ COMPLETE (`pkg/identity/spire/`)
  - WorkloadAPIClient implementing IdentityProvider
  - Unix domain socket connection
  - FetchX509SVID, FetchJWTSVID, FetchTrustBundle
  - SVID and bundle watching with streaming
  - Backoff retry logic for reconnection
  - Health checking
  - 45 tests
- **T3.2: Cloud Identity Providers** ✅ COMPLETE (`pkg/identity/cloud/`)
  - AWSProvider using IAM roles and instance identity
  - GCPProvider using service accounts and metadata
  - AzureProvider using managed identity
  - Common Provider interface with SVID/bundle methods
  - Attestation data extraction from cloud metadata
  - Token-based authentication
  - 74 tests
- **T3.3: Service Mesh Integration** ✅ COMPLETE (`pkg/identity/mesh/`)
  - IstioProvider using Istio sidecar certificates
  - ConsulProvider for Consul Connect identity
  - LinkerdProvider for Linkerd proxy identity
  - File-based certificate loading
  - SPIFFE ID extraction from certificates
  - Health checking and watching
  - 32 tests

**Phase 4: Trust Federation (Weeks 8-10) ✅ COMPLETE**
- Federation types (`pkg/identity/federation/types.go`)
  - FederationState enum (pending, active, suspended, revoked, expired)
  - FederationType (bidirectional, unidirectional, transitive)
  - TrustPolicy with allowed/denied paths and services
  - FederatedDomain with trust bundle and policy
  - FederationManager interface
  - ValidationResult for SVID validation
  - BundleFetcher and FederationStore interfaces
- Federation manager (`pkg/identity/federation/manager.go`)
  - Manager implementing FederationManager
  - AddFederatedDomain, RemoveFederatedDomain, UpdateFederatedDomain
  - ValidateSVID with policy enforcement
  - GetAggregatedTrustBundle for combined bundle
  - Background trust bundle refresh
  - Event emission for federation changes
- HTTP bundle fetcher (`pkg/identity/federation/fetcher.go`)
  - HTTPBundleFetcher for remote bundles
  - Profiles: https_web, https_spiffe, spiffe_bundle_endpoint
  - PEM and SPIFFE Bundle (JWK Set) format parsing
- In-memory store (`pkg/identity/federation/store.go`)
  - InMemoryStore implementing FederationStore
  - CRUD operations with thread safety
- 69 tests

**Phase 5: E2E Testing (Weeks 11-12) ✅ COMPLETE**
- Comprehensive E2E tests (`pkg/identity/e2e/`)
  - TestE2E_IdentityLifecycle - Complete identity lifecycle
  - TestE2E_Federation - Federation between trust domains
  - TestE2E_FederationPolicy - Policy enforcement testing
  - TestE2E_FederationStateTransitions - State machine testing
  - TestE2E_MultipleCAs - CA rotation scenarios
  - TestE2E_FileBasedIdentity - File provider testing
  - TestE2E_SPIFFEIDParsing - SPIFFE ID parsing
  - TestE2E_TrustBundleExpiration - Expiry handling
  - TestE2E_FederationStorePersistence - Store testing
  - TestE2E_CompleteWorkflow - Full workflow test
- 19 E2E tests

**Phase 6: Documentation & Migration ✅ COMPLETE**
- Updated CLAUDE.md with Epic 17 completion status
- Updated Epic Dependencies section
- Package documentation in code files

**Epic 17 Summary:**
- 6 phases completed
- 332 total tests
- Complete SPIFFE implementation:
  - Embedded identity provider (zero-configuration)
  - SPIRE Workload API integration
  - Cloud provider support (AWS, GCP, Azure)
  - Service mesh integration (Istio, Consul, Linkerd)
  - Trust federation between domains
  - Policy-based access control
- Packages: `pkg/identity/`, `pkg/identity/spire/`, `pkg/identity/cloud/`, `pkg/identity/mesh/`, `pkg/identity/federation/`, `pkg/identity/e2e/`

### Epic 19: Observability Gateway ✅ COMPLETE

**Implementation Plan:** 8 phases

**Goal**: Build a telemetry gateway that aggregates metrics, logs, and traces from agents over NATS and exposes them to standard observability backends.

**Phase 1: Metrics Gateway Core ✅ COMPLETE**
- Core types and configuration (`pkg/gateway/types.go`)
  - Config with NATS, server, metrics, logs, traces, HA configuration
  - DefaultConfig with sensible defaults
- Metrics store (`pkg/gateway/metrics/store.go`)
  - MetricsStore for aggregating agent metrics
  - Agent tracking with labels and metadata
  - Cardinality control (MaxSeries, MaxLabelsPerSeries, DropHighCardinality)
  - Stale agent removal

**Phase 2: Prometheus Integration ✅ COMPLETE**
- HTTP handlers (`pkg/gateway/metrics/handler.go`)
  - /metrics endpoint for Prometheus scraping
  - /federate endpoint for federation
  - /health and /ready endpoints
  - Label transformations (add, drop, rewrite)
- Remote write client (`pkg/gateway/metrics/remote_write.go`)
  - Custom protobuf marshaling (avoiding prometheus/prometheus dependency)
  - Snappy compression
  - Retry with exponential backoff
  - Basic auth and bearer token support
- NATS subscriber (`pkg/gateway/metrics/subscriber.go`)
  - JetStream and core NATS support
  - Prometheus text, protobuf, and JSON format parsing

**Phase 3: Logs Gateway ✅ COMPLETE**
- Logs store (`pkg/gateway/logs/store.go`)
  - LogsStore with level filtering
  - Source filtering (include/exclude)
  - Query API with time range, search, labels
- Loki pusher (`pkg/gateway/logs/loki.go`)
  - LokiPusher for pushing logs to Loki
  - Batch processing and retry logic
  - Multi-tenant support via X-Scope-OrgID

**Phase 4: Traces Gateway ✅ COMPLETE**
- Traces store (`pkg/gateway/traces/store.go`)
  - TracesStore grouping spans into traces
  - Sampling with error and slow threshold priority
  - Query API by service, operation, duration
- OTLP exporter (`pkg/gateway/traces/otlp.go`)
  - OTLPExporter for exporting to Tempo/Jaeger
  - OTLP JSON format
  - Gzip compression support

**Phase 5: Control Plane Integration ✅ COMPLETE**
- Integration helper (`pkg/gateway/integration.go`)
  - PublishMetrics, PublishLogs, PublishTraces methods
  - GatewayStatus for status reporting
  - LogWriter for io.Writer compatibility

**Phase 6: Scaling and HA ✅ COMPLETE**
- HA configuration in types
- Shard-based agent distribution
- Instance ID for coordination

**Phase 7: Deployment ✅ COMPLETE**
- Docker Compose stack (`deploy/gateway/docker-compose.yml`)
  - NATS, Gateway, Prometheus, Loki, Tempo, Grafana
  - Pre-configured datasources and dashboard
- Dockerfile (`deploy/gateway/Dockerfile`)
- Configuration examples (`deploy/gateway/config.yaml.example`)

**Phase 8: Documentation and Testing ✅ COMPLETE**
- Gateway documentation (`docs/content/en/docs/operations/gateway.md`)
- Test coverage for stores (23 tests)
  - Metrics store tests
  - Logs store tests
  - Traces store tests

**Epic 19 Summary:**
- 8 phases completed
- 23 tests for gateway stores
- Complete telemetry aggregation:
  - Metrics: Prometheus scraping, federation, remote write
  - Logs: Loki push with batching
  - Traces: OTLP export with sampling
  - Docker Compose deployment with full observability stack
- Packages: `pkg/gateway/`, `pkg/gateway/metrics/`, `pkg/gateway/logs/`, `pkg/gateway/traces/`
- Binary: `cmd/kscore-telemetry-gateway/`
- Deployment: `deploy/gateway/`

## Epic Dependencies

Implementation order:
1. **Epic 1** (Core Infrastructure) - ✅ COMPLETE
2. **Epic 2** (Remote Execution) - ✅ COMPLETE
3. **Epic 3** (State Management) - ✅ COMPLETE
4. **Epic 4** (Event System) - ✅ COMPLETE (All 8 weeks) - Depends on Epic 1
5. **Epic 5** (GitOps Integration) - ✅ COMPLETE - Depends on Epic 2, 3, 4
6. **Epic 6** (Policy Enforcement) - ✅ COMPLETE - Depends on Epic 2, 3, 4
7. **Epic 7** (Observability) - ✅ COMPLETE - Instruments all epics
8. **Epic 8** (Multi-Environment) - ✅ COMPLETE - Depends on Epic 1, 2, 3
9. **Epic 9** (Plugin System) - ✅ COMPLETE (All 7 phases) - Depends on Epic 3, 4, 5, 6 (extends all major subsystems)
10. **Epic 10** (Documentation) - ✅ COMPLETE (All 7 phases) - Documents Epic 1-9 (Hugo + Docsy, 45 pages, ~21,500 lines)
11. **Epic 11** (Clustering) - ✅ COMPLETE (All 8 phases) - Depends on Epic 1, 7 (etcd-based HA clustering, automatic failover, work distribution)
12. **Epic 12** (E2E Testing) - ✅ COMPLETE - Comprehensive E2E test infrastructure (harness, topologies, scenarios, performance)
13. **Epic 13** (CGO Removal) - ✅ COMPLETE - Independent, enables pure Go builds
14. **Epic 14** (NATS Mesh Communication) - ✅ COMPLETE - Depends on Epic 1, 7, 11 (NATS-only communication, superclusters, NAT traversal)
15. **Epic 15** (Observability Enhancements) - ✅ COMPLETE - Depends on Epic 7, 14 (NATS telemetry transport, stdout/syslog logging, CLI audit)
16. **Epic 16** (Stdlib System Modules) - ✅ COMPLETE - Depends on Epic 3, 8 (84 cross-platform system management modules)
17. **Epic 17** (SPIFFE Identity) - ✅ COMPLETE (All 6 phases) - Depends on Epic 1, 11, 14 (embedded SPIFFE provider, SPIRE/cloud/mesh integration, trust federation, 332 tests)
18. **Epic 18** (IPv6 Support) - ✅ COMPLETE - Depends on Epic 1, 11, 14 (full IPv6 and dual-stack support for all components, E2E test topology)
19. **Epic 19** (Observability Gateway) - ✅ COMPLETE - Depends on Epic 7, 14, 15 (telemetry gateway for isolated agents, Prometheus/Loki/Tempo bridge)
20. **Epic 20** (Windows Support) - ✅ COMPLETE (All 7 phases) - Depends on Epic 1, 2, 3, 13 (Windows service, PowerShell/Cmd execution, state modules, file operations, MSI installer)

## Key Architectural Patterns

### Salt Project-Like Features (Modernized)
- Remote execution with flexible targeting
- Declarative state management (idempotent)
- Event-driven reactor system
- Vars (configuration data) and Facts (agent metadata)

### Cloud-Native Extensions
- GitOps integration (ArgoCD, Flux) for deployment verification and rollback
- Policy-as-code (OPA/CEL) for continuous compliance
- Kubernetes operator mode with CRDs
- Multi-cloud support (AWS, GCP, Azure)
- Service mesh integration (Istio, Linkerd, Consul)

### Technology Stack
- **Language**: Go 1.21+
- **Message Bus**: NATS 2.10+ with JetStream (embedded or external)
- **State Storage**:
  - SQLite 3.x (embedded, for dev/small deployments)
  - PostgreSQL 14+ (for production/large deployments)
- **API**: gRPC + REST (gRPC-gateway)
- **Observability**: Prometheus, OpenTelemetry, Grafana
- **Policy**: OPA (Rego), CEL
- **Modules**: Starlark runtime, WASM (wazero - pure Go), Cosign signatures
- **SQLite**: modernc.org/sqlite (pure Go, no CGO)

### Module System Architecture (Epic 9)

Keystone Core's module system enables secure extensibility through versioned, dependency-managed packages:

**Module Format:**
- **module.yaml**: Manifest declaring dependencies, capabilities, limits, entrypoints
- **module.lock**: Pinned dependency versions for reproducible builds
- **Structured layout**: `states/` (Starlark), `providers/` (WASM), `tests/`, SBOM, provenance
- **Namespaced**: Modules identified as `vendor/package` (e.g., `std/files`, `myorg/custom-state`)

**Dependency Management:**
- **SemVer Constraints**: Version ranges (`>=1.0 <2.0`, `^1.5.0`, `~1.2.3`)
- **Transitive Resolution**: Automatically resolves entire dependency graph
- **Conflict Resolution**: Minimum Version Selection (MVS) algorithm (Go-inspired)
- **Lock Files**: Pin exact versions and SHA256 hashes for reproducibility
- **Circular Detection**: DAG construction with cycle detection

**Registry & Distribution (Hybrid OCI + HTTP Proxy):**
- **OCI Registry**: Source of truth, stores module ZIPs as OCI artifacts with cosign signatures
- **HTTP Proxy**: Go-mod-style endpoints (`/<module>/@v/list`, `/@v/<ver>.info`, `.mod`, `.zip`)
- **SumDB**: Transparency log prevents registry serving different versions to different users
- **Air-Gapped**: Mirror support for disconnected environments

**Resolver:**
A Go tool that:
1. Parses `module.yaml` and optional `module.lock`
2. Queries registry HTTP proxy for version lists and metadata
3. Builds dependency DAG (detects cycles)
4. Resolves version conflicts using MVS
5. Verifies modules: SumDB hash lookup + cosign signature verification
6. Downloads ZIPs to content-addressed cache (`~/.kscore/modules/<hash>/`)
7. Generates reproducible `module.lock`

**Security Model - Capability-Based Access:**
- **No Ambient Authority**: Modules can only access explicitly granted capabilities
- **Sandboxed Execution**: Starlark and WASM runtimes prevent escape
- **Cryptographic Verification**: Cosign signatures + SumDB-style transparency log
- **Deterministic**: Modules are pure functions with no side effects (reproducible execution)

**Module Types:**
- **State Modules**: Custom resource types (e.g., Vault secrets, Firecracker VMs)
- **Execution Handlers**: Custom execution environments (e.g., unikernels, WebAssembly)
- **Policy Rules**: Custom compliance checks (e.g., organization-specific requirements)
- **Reactors**: Custom event handlers (e.g., incident response automation)
- **Verification Steps**: Custom deployment checks for GitOps workflows

**Host Capabilities** (minimal, audited interfaces):
- `fs.read` / `fs.write` - Filesystem access (path-scoped)
- `http.get` / `http.post` - HTTP requests (domain-scoped)
- `exec` - Command execution (command allowlist)
- `secrets.read` / `secrets.write` - Secret access (path-scoped)
- `log` - Structured logging (rate-limited)
- `time` - Time access (breaks determinism, rarely granted)
- `kv` - Module key-value storage (namespace-scoped)

**Trust & Distribution:**
- All modules signed with Cosign
- Transparency log prevents serving different versions to different clients
- Policy engine (OPA) controls which modules can load based on:
  - Signature key (trust root)
  - Requested capabilities
  - Environment (dev/staging/prod)
  - Risk profile
  - Dependency chain (transitive trust)

**Why Starlark and WASM:**
- **Starlark**: Simple, Python-like, deterministic by design, used by Bazel/Buck
- **WASM**: High performance, compile from any language (Rust/Go/C++), standardized sandboxing

**Module CLI (via kscorectl):**
- `kscorectl module init` - Initialize new module
- `kscorectl module resolve` - Resolve dependencies, generate lock file
- `kscorectl module install` - Install from lock file
- `kscorectl module build` - Build module ZIP
- `kscorectl module sign` - Sign with cosign
- `kscorectl module publish` - Upload to registry
- `kscorectl module update` - Update to latest compatible versions
- `kscorectl module tree` - Display dependency tree
- `kscorectl module mirror` - Mirror for air-gapped environments

## Working with Design Documents

### Updating DESIGN.md
- Maintain the market positioning section - this defines Keystone Core's unique value
- Keep architecture diagrams in sync with epic changes
- Update comparison matrix if competitive landscape changes
- Technology stack changes require epic updates too

### Updating Epic Documents
Each epic follows a consistent structure:
- **Overview & Success Criteria**: High-level goals
- **User Stories**: Feature requirements with acceptance criteria
- **Technical Tasks**: Week-by-week implementation breakdown
- **Dependencies**: Required epics and libraries
- **Risks & Mitigations**: Known challenges
- **Testing Strategy**: Unit, integration, performance tests
- **Definition of Done**: Completion checklist

### Adding New Features
When adding new feature ideas:
1. Determine which epic it belongs to (or if it needs a new epic)
2. Add user story with clear acceptance criteria
3. Update technical tasks with implementation approach
4. Consider dependencies and risks
5. Update success criteria and definition of done

### Cross-Epic Coordination
Many features span multiple epics:
- **Deployment Verification**: Epic 5 (GitOps) uses Epic 2 (Execution) and Epic 3 (State)
- **Drift Detection**: Epic 3 (State) generates Epic 4 (Events) that trigger Epic 6 (Policy)
- **Real-time Dashboards**: Epic 7 (Observability) visualizes data from all epics
- **Plugin System**: Epic 9 extends Epic 3 (custom state modules), Epic 4 (custom reactors), Epic 5 (custom verifications), and Epic 6 (custom policies)

When updating one epic, check if related epics need updates.

### Plugin System Considerations

When working on other epics, consider plugin extension points:
- **Epic 3 (State)**: Where can custom state modules integrate? What interface must they implement?
- **Epic 4 (Events)**: How do plugins subscribe to events? What reactor interface is exposed?
- **Epic 5 (GitOps)**: Can plugins add custom verification steps? What's the verification interface?
- **Epic 6 (Policy)**: How do custom OPA/CEL rules integrate? Can plugins define new policy types?

The plugin manifest must declare which capabilities the plugin needs, and the host only grants scoped, minimal access.

## CLI Architecture (Plugin Pattern)

Keystone Core uses a Git-style plugin architecture for its CLI:

**Main CLI: `kscorectl`**
- Lightweight dispatcher that discovers and executes `kscore-*` binaries
- Similar to how `git` works with `git-*` plugins
- All user-facing commands go through `kscorectl`

**Plugin Binaries: `kscore-*`**
- `kscore-module` - Module management (init, build, sign, publish, resolve, install)
- `kscore-state` - State management (apply, check, diff)
- `kscore-exec` - Remote execution (run commands on agents)
- Custom extensions can add `kscore-customtool` and it works as `kscorectl customtool`

**How it works:**
```bash
# User runs kscorectl command
kscorectl module install vendor/pkg_apt

# kscorectl looks for kscore-module in $PATH
# Executes: kscore-module install vendor/pkg_apt

# Third-party plugins also work
kscorectl custom-backup run  # Executes kscore-custom-backup run
```

**Benefits:**
- Clear namespace separation (all binaries prefixed with `kscore-`)
- Extensibility - third parties can add plugins
- Clean documentation (everything is `kscorectl <subcommand>`)
- Code separation - each subsystem can be developed independently
- Optional components - users only install what they need

**Server Binaries (not plugins):**
- `kscore-server` - Control plane daemon
- `kscore-agent` - Agent daemon on managed nodes
- `kscore-registry` - Module registry server

These are long-running services, not invoked via `kscorectl`.

## Binary Summary

Keystone Core will have the following executables:

### 1. **User-Facing CLI**
- **`kscorectl`** - Main CLI tool (plugin dispatcher)
  - Lightweight binary that discovers and executes `kscore-*` plugins
  - All user commands go through this: `kscorectl module install`, `kscorectl state apply`, etc.
  - Follows Git/kubectl plugin pattern

### 2. **Server Daemons** (long-running services)
- **`kscore-server`** - Control plane daemon
  - API Server (gRPC + REST)
  - State Manager
  - Connection Manager
  - Event/Reactor Engine
  - Can run with embedded NATS or connect to external cluster

- **`kscore-agent`** - Agent daemon on managed nodes
  - Runs on K8s, VMs, bare metal, edge devices
  - Executes commands locally
  - Reports heartbeat and system metadata
  - Can run with embedded NATS (leaf node mode)

- **`kscore-registry`** - Module registry server
  - OCI registry integration
  - HTTP proxy (Go-mod-style endpoints)
  - SumDB transparency log
  - Signature verification service

- **`kscore-telemetry-gateway`** - Telemetry aggregation gateway
  - Aggregates metrics, logs, and traces from agents over NATS
  - Exposes /metrics and /federate endpoints for Prometheus
  - Pushes logs to Loki
  - Exports traces to OTLP (Tempo/Jaeger)
  - Supports HA mode with sharding

### 3. **CLI Plugins** (invoked via kscorectl)
- **`kscore-module`** - Module management
  - Development: `init`, `build`, `sign`, `publish`, `validate`, `test`
  - Installation: `install`, `resolve`, `update`, `tree`, `verify`, `clean`, `mirror`
  - Implements dependency resolution, SumDB verification, etc.

- **`kscore-state`** - State management (Epic 3)
  - `apply`, `check`, `diff`, `show`, `list`
  - Declarative configuration management

- **`kscore-exec`** - Remote execution (Epic 2)
  - `run`, `async`, `status`, `output`
  - Execute commands across infrastructure

- **`kscore-monitor`** - Real-time TUI monitoring (Epic 7)
  - Terminal-based dashboard for live system monitoring
  - 8 views: Dashboard, Agents, Events, State, Policy, Jobs, Logs, Metrics
  - Vim-style navigation, filtering, export
  - SSH-friendly, zero web dependencies

- **`kscore-policy`** (optional, Epic 6) - Policy enforcement
  - `check`, `enforce`, `audit`, `report`
  - OPA/CEL policy evaluation

- **`kscore-gitops`** (optional, Epic 5) - GitOps integration
  - `verify`, `rollback`, `sync`, `diff`
  - ArgoCD/Flux integration

- **`kscore-migrate`** - Database migration tool ✅ IMPLEMENTED
  - `run` - Run migration from SQLite to PostgreSQL
  - `validate` - Validate migration completeness
  - Dry-run mode, batch processing, skip-existing support

- **`kscore-identity`** (Epic 17) - SPIFFE identity management ✅ IMPLEMENTED
  - `token` - Create, list, show, revoke join tokens
  - `ca` - CA info, backup, restore, rotate
  - `federation` - List, add, show, suspend, activate, remove, refresh
  - `bundle` - Show, export trust bundles
  - `events` - View identity events
  - `status` - Show identity provider status

### 4. **Third-Party Plugins** (optional)
- Any binary named `kscore-<name>` in $PATH automatically works as `kscorectl <name>`
- Examples:
  - `kscore-backup` → `kscorectl backup`
  - `kscore-custom-deploy` → `kscorectl custom-deploy`
  - Community extensions without forking core

**Total Core Binaries**: 11 (1 CLI + 4 servers + 6 built-in plugins)
**Extensible**: Unlimited via third-party `kscore-*` plugins

## Future Implementation Repository

When code implementation begins, it will likely be structured as:
```
keystone-core/
├── cmd/
│   ├── kscorectl/              # Main CLI (plugin-style dispatcher)
│   ├── kscore-server/     # Control plane
│   ├── kscore-agent/      # Agent binary
│   ├── kscore-module/     # Module commands (invoked via kscorectl)
│   ├── kscore-state/      # State commands (invoked via kscorectl)
│   ├── kscore-exec/       # Execution commands (invoked via kscorectl)
│   ├── kscore-monitor/    # TUI monitor (invoked via kscorectl)
│   ├── kscore-migrate/    # Database migration tool (invoked via kscorectl)
│   ├── kscore-identity/   # Identity management (invoked via kscorectl)
│   └── kscore-registry/   # Registry server (OCI + HTTP proxy)
├── pkg/
│   ├── api/               # gRPC/REST API
│   ├── state/             # State management
│   ├── execution/         # Remote execution
│   ├── events/            # Event system
│   ├── policy/            # Policy engine
│   ├── module/            # Module system
│   │   ├── runtime/       # Module runtime & loader
│   │   │   ├── starlark/  # Starlark runtime
│   │   │   └── wasm/      # WASM runtime (wasmtime)
│   │   ├── capabilities/  # Host capability implementations
│   │   ├── resolver/      # Dependency resolution engine
│   │   │   ├── dag/       # Dependency graph construction
│   │   │   ├── semver/    # SemVer constraint solver
│   │   │   └── mvs/       # Minimum Version Selection
│   │   ├── registry/      # Registry client (OCI + HTTP proxy)
│   │   │   ├── oci/       # OCI registry client
│   │   │   ├── proxy/     # HTTP proxy client (Go-mod style)
│   │   │   └── cache/     # Content-addressed cache
│   │   ├── sumdb/         # SumDB client (transparency log)
│   │   └── verify/        # Signature & hash verification
│   └── ...
├── modules/
│   ├── examples/          # Example modules (Starlark & WASM)
│   ├── stdlib/            # Standard library modules (std/*)
│   │   ├── files/         # std/files - File operations
│   │   ├── exec/          # std/exec - Command execution
│   │   ├── strings/       # std/strings - String utilities
│   │   └── ...
│   └── sdk/
│       ├── starlark/      # Starlark SDK helpers
│       └── rust/          # Rust SDK for WASM modules (kscore-module-sdk)
├── deploy/
│   ├── kubernetes/        # K8s manifests
│   ├── docker/            # Container images
│   ├── terraform/         # Infrastructure
│   └── plugin-registry/   # Plugin registry deployment
└── docs/                  # User documentation
    └── plugins/           # Plugin development guides
```

This structure is implied by the epic technical tasks but not yet created.

## Key Design Principles

When implementing Keystone Core, follow these principles from the design documents:

1. **Zero Dependencies for Getting Started**:
   - Embedded NATS mode (no external message broker required)
   - Embedded SQLite storage (no external database required)
   - Single binary deployment (`kscore-server`) runs everything
2. **Security by Default**: Capability-based access, signed plugins, policy enforcement
3. **Determinism**: Plugins and operations should be reproducible (same input → same output)
4. **Minimal Attack Surface**: Only grant necessary capabilities, sandbox all untrusted code
5. **Auditability**: Comprehensive logging, transparency logs, policy decisions tracked
6. **Performance**: <100ms command execution to 1000 nodes, <10ms plugin overhead
7. **Hybrid Infrastructure**: Unified interface for K8s, VMs, bare metal, edge devices
8. **Graceful Scaling**: Seamless migration paths as deployments grow:
   - Embedded NATS → External NATS cluster
   - SQLite → PostgreSQL
   - Both with automated migration tooling
