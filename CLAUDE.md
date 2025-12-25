# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Purpose

This is the **design documentation repository** for TitanAnvil, a cloud-native runtime infrastructure control plane. TitanAnvil is positioned as the operational layer between GitOps/IaC deployments and runtime infrastructure, inspired by Salt Project but modernized for cloud-native environments.

**Key Concept**: "GitOps deploys it. We keep it running."

## Project Status

This repository contains the **complete implementation of Epic 1: Core Infrastructure**. The project has transitioned from design-only to a working implementation with:

- Full NATS integration (embedded, external, and leaf modes)
- Working agent system with registration, heartbeat, and command execution
- SQLite-based state management
- Comprehensive test suite (>80% coverage across all core packages)

**Current Status**: Epic 1 COMPLETE ✅ | Epic 2 IN PROGRESS (Week 1/4 complete)

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
    └── 09-plugin-system.md            # Starlark/WASM plugin architecture
```

## Architecture Overview

TitanAnvil fills the gap between declarative GitOps tools and runtime operations:

**Core Architecture Components:**
- **Control Plane**: API Server, State Manager, Event/Reactor Engine
- **Message Bus**: NATS with three deployment modes:
  - **Embedded mode**: In-process NATS for initial setups, small deployments (<100 nodes), and edge agents
  - **External cluster mode**: Dedicated NATS cluster for production deployments (100+ nodes)
  - **Hybrid mode**: Control plane uses external cluster, agents use embedded NATS as leaf nodes
  - JetStream for event persistence (supported in all modes)
- **Agents**: Lightweight Go binaries on managed nodes (K8s, VMs, bare metal, edge)
- **State Storage**: SQLite or PostgreSQL for operational state (NOT JetStream - see design rationale)
  - **SQLite (embedded)**: Zero dependencies, perfect for dev/testing/home labs, small deployments (<100 nodes)
  - **PostgreSQL**: Production deployments, high availability, scalability (100+ nodes)
  - Automated migration tooling from SQLite → PostgreSQL

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

### Epic 2: Remote Execution 🚧 IN PROGRESS

**Implementation Plan:** Phases 1-2 (Core Execution + Targeting System) over 4 weeks

**Week 1: Foundation ✅ COMPLETE**
- Plugin discovery system (pkg/plugin/)
  - Automatic discovery of titananvil-* binaries in PATH
  - Plugin execution with streaming I/O
  - Test coverage: 82.7%
- Shell abstraction layer (pkg/execution/shell.go)
  - Cross-platform shell support (Bash, Sh, PowerShell, Cmd)
  - Automatic default shell detection per OS
  - Test coverage: 82.6%
- Enhanced executor (pkg/execution/executor.go)
  - Shell-based command execution
  - Retry logic with configurable attempts and delays
  - Improved cancellation (SIGTERM → SIGKILL with grace period)
  - Test coverage: 84.1%
- titanctl plugin integration
  - Dynamic command discovery and dispatch

**Week 2: Targeting System (PENDING)**
- Target expression parser (pkg/targeting/parser.go)
- Agent matcher with filtering (pkg/targeting/matcher.go)
- Batch execution engine (pkg/targeting/batch.go)
- Dependencies: github.com/expr-lang/expr, github.com/gobwas/glob

**Week 3: Integration (PENDING)**
- Protobuf definitions for multi-agent execution
- Control plane enhancements for parallel dispatch
- State management extensions for job tracking

**Week 4: CLI & E2E (PENDING)**
- titananvil-exec plugin binary
- End-to-end integration tests
- Multi-agent execution scenarios

## Epic Dependencies

Implementation order:
1. **Epic 1** (Core Infrastructure) - ✅ COMPLETE
2. **Epic 2** (Remote Execution) - 🚧 IN PROGRESS (Week 1/4 complete) - Depends on Epic 1
3. **Epic 3** (State Management) - Depends on Epic 1, 2
4. **Epic 4** (Event System) - Depends on Epic 1
5. **Epic 5** (GitOps Integration) - Depends on Epic 2, 3, 4
6. **Epic 6** (Policy Enforcement) - Depends on Epic 2, 3, 4
7. **Epic 7** (Observability) - Instruments all epics
8. **Epic 8** (Multi-Environment) - Depends on Epic 1, 2, 3
9. **Epic 9** (Plugin System) - Depends on Epic 3, 4, 5, 6 (extends all major subsystems)

## Key Architectural Patterns

### Salt Project-Like Features (Modernized)
- Remote execution with flexible targeting
- Declarative state management (idempotent)
- Event-driven reactor system
- Pillar (secure config data) and Grains (agent metadata)

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
- **Modules**: Starlark runtime, WASM (wasmtime), Cosign signatures

### Module System Architecture (Epic 9)

TitanAnvil's module system enables secure extensibility through versioned, dependency-managed packages:

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
6. Downloads ZIPs to content-addressed cache (`~/.titananvil/modules/<hash>/`)
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

**Module CLI (via titanctl):**
- `titanctl module init` - Initialize new module
- `titanctl module resolve` - Resolve dependencies, generate lock file
- `titanctl module install` - Install from lock file
- `titanctl module build` - Build module ZIP
- `titanctl module sign` - Sign with cosign
- `titanctl module publish` - Upload to registry
- `titanctl module update` - Update to latest compatible versions
- `titanctl module tree` - Display dependency tree
- `titanctl module mirror` - Mirror for air-gapped environments

## Working with Design Documents

### Updating DESIGN.md
- Maintain the market positioning section - this defines TitanAnvil's unique value
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

TitanAnvil uses a Git-style plugin architecture for its CLI:

**Main CLI: `titanctl`**
- Lightweight dispatcher that discovers and executes `titananvil-*` binaries
- Similar to how `git` works with `git-*` plugins
- All user-facing commands go through `titanctl`

**Plugin Binaries: `titananvil-*`**
- `titananvil-module` - Module management (init, build, sign, publish, resolve, install)
- `titananvil-state` - State management (apply, check, diff)
- `titananvil-exec` - Remote execution (run commands on agents)
- Custom extensions can add `titananvil-customtool` and it works as `titanctl customtool`

**How it works:**
```bash
# User runs titanctl command
titanctl module install vendor/pkg_apt

# titanctl looks for titananvil-module in $PATH
# Executes: titananvil-module install vendor/pkg_apt

# Third-party plugins also work
titanctl custom-backup run  # Executes titananvil-custom-backup run
```

**Benefits:**
- Clear namespace separation (all binaries prefixed with `titananvil-`)
- Extensibility - third parties can add plugins
- Clean documentation (everything is `titanctl <subcommand>`)
- Code separation - each subsystem can be developed independently
- Optional components - users only install what they need

**Server Binaries (not plugins):**
- `titananvil-server` - Control plane daemon
- `titananvil-agent` - Agent daemon on managed nodes
- `titananvil-registry` - Module registry server

These are long-running services, not invoked via `titanctl`.

## Binary Summary

TitanAnvil will have the following executables:

### 1. **User-Facing CLI**
- **`titanctl`** - Main CLI tool (plugin dispatcher)
  - Lightweight binary that discovers and executes `titananvil-*` plugins
  - All user commands go through this: `titanctl module install`, `titanctl state apply`, etc.
  - Follows Git/kubectl plugin pattern

### 2. **Server Daemons** (long-running services)
- **`titananvil-server`** - Control plane daemon
  - API Server (gRPC + REST)
  - State Manager
  - Connection Manager
  - Event/Reactor Engine
  - Can run with embedded NATS or connect to external cluster

- **`titananvil-agent`** - Agent daemon on managed nodes
  - Runs on K8s, VMs, bare metal, edge devices
  - Executes commands locally
  - Reports heartbeat and system metadata
  - Can run with embedded NATS (leaf node mode)

- **`titananvil-registry`** - Module registry server
  - OCI registry integration
  - HTTP proxy (Go-mod-style endpoints)
  - SumDB transparency log
  - Signature verification service

### 3. **CLI Plugins** (invoked via titanctl)
- **`titananvil-module`** - Module management
  - Development: `init`, `build`, `sign`, `publish`, `validate`, `test`
  - Installation: `install`, `resolve`, `update`, `tree`, `verify`, `clean`, `mirror`
  - Implements dependency resolution, SumDB verification, etc.

- **`titananvil-state`** - State management (Epic 3)
  - `apply`, `check`, `diff`, `show`, `list`
  - Declarative configuration management

- **`titananvil-exec`** - Remote execution (Epic 2)
  - `run`, `async`, `status`, `output`
  - Execute commands across infrastructure

- **`titananvil-policy`** (optional, Epic 6) - Policy enforcement
  - `check`, `enforce`, `audit`, `report`
  - OPA/CEL policy evaluation

- **`titananvil-gitops`** (optional, Epic 5) - GitOps integration
  - `verify`, `rollback`, `sync`, `diff`
  - ArgoCD/Flux integration

### 4. **Third-Party Plugins** (optional)
- Any binary named `titananvil-<name>` in $PATH automatically works as `titanctl <name>`
- Examples:
  - `titananvil-backup` → `titanctl backup`
  - `titananvil-migrate` → `titanctl migrate`
  - Community extensions without forking core

**Total Core Binaries**: 7 (1 CLI + 3 servers + 3 built-in plugins)
**Extensible**: Unlimited via third-party `titananvil-*` plugins

## Future Implementation Repository

When code implementation begins, it will likely be structured as:
```
titan-anvil/
├── cmd/
│   ├── titanctl/              # Main CLI (plugin-style dispatcher)
│   ├── titananvil-server/     # Control plane
│   ├── titananvil-agent/      # Agent binary
│   ├── titananvil-module/     # Module commands (invoked via titanctl)
│   ├── titananvil-state/      # State commands (invoked via titanctl)
│   ├── titananvil-exec/       # Execution commands (invoked via titanctl)
│   └── titananvil-registry/   # Registry server (OCI + HTTP proxy)
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
│       └── rust/          # Rust SDK for WASM modules (titan-module-sdk)
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

When implementing TitanAnvil, follow these principles from the design documents:

1. **Zero Dependencies for Getting Started**:
   - Embedded NATS mode (no external message broker required)
   - Embedded SQLite storage (no external database required)
   - Single binary deployment (`titananvil-server`) runs everything
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
