---
title: "Roadmap"
weight: 3
description: >
  Keystone Core project roadmap and future development plans
---

This page outlines the Keystone Core development roadmap, including completed milestones, current work, and future plans. The roadmap is organized around major feature epics and provides visibility into project priorities.

## Project Vision

**"GitOps deploys it. We keep it running."**

Keystone Core aims to be the operational layer between GitOps/IaC deployments and runtime infrastructure. Our goal is to provide a modern, cloud-native runtime infrastructure control plane that works seamlessly across Kubernetes, VMs, bare metal, and edge environments.

## Roadmap Overview

```
COMPLETED (Epics 1-14)                 PLANNED (Epics 15-20)
───────────────────────────────────────────────────────────────────────
Epic 1: Core Infrastructure            Epic 15: Observability Enhancements
Epic 2: Remote Execution               Epic 16: Stdlib System Modules
Epic 3: State Management               Epic 17: SPIFFE Identity Framework
Epic 4: Event System                   Epic 18: IPv6 Support
Epic 5: GitOps Integration             Epic 19: Observability Gateway
Epic 6: Policy Enforcement             Epic 20: Windows Support
Epic 7: Observability
Epic 8: Multi-Environment            Future Considerations:
Epic 9: Plugin System                  - Multi-Tenancy
Epic 10: Documentation                 - Web UI Dashboard
Epic 11: HA Clustering                 - ServiceNow Integration
Epic 12: E2E Testing
Epic 13: CGO Removal
Epic 14: NATS Mesh Communication
```

## Completed Milestones

### Epic 1: Core Infrastructure ✅

**Status**: Complete | **Coverage**: >80%

Foundation for the entire system including:

- **NATS Integration**: Three deployment modes (embedded, external, hybrid)
- **Agent System**: Registration, heartbeat, command execution
- **Control Plane**: API server, connection manager, state manager
- **State Storage**: SQLite for development, PostgreSQL for production
- **Security**: mTLS, API key authentication, credential management

**Key Achievements**:
- Zero-dependency getting started (embedded NATS + SQLite)
- Comprehensive agent lifecycle management
- Robust command execution with streaming output
- Production-ready state persistence

---

### Epic 2: Remote Execution ✅

**Status**: Complete | **Coverage**: >82%

Cross-platform remote command execution:

- **Plugin Architecture**: Git-style CLI plugin system
- **Shell Abstraction**: Bash, PowerShell, Cmd support
- **Targeting System**: Glob patterns and expression-based filtering
- **Batch Execution**: Parallel execution across agent groups
- **Job Tracking**: Comprehensive job status and output streaming

**Key Achievements**:
- Execute commands on 1000+ agents simultaneously
- Flexible targeting with compound expressions
- Real-time streaming output

---

### Epic 3: State Management ✅

**Status**: Complete | **Coverage**: >44%

Declarative configuration management (Salt-like):

- **State Modules**: File, Package, Service, User, Group, Cmd
- **Dependency Resolution**: DAG-based with requisites (require, watch, prereq, onchanges)
- **Templating**: Go templates with vars (config data) and facts (agent metadata)
- **Drift Detection**: Compare desired vs actual state with severity levels
- **CLI Tools**: `kscore-state apply`, `check`, `drift`

**Key Achievements**:
- Idempotent state application
- Cross-platform module support
- Template rendering with rich function library
- Performance: 5,000+ states/sec

---

### Epic 4: Event System ✅

**Status**: Complete | **Coverage**: >75%

Event-driven automation:

- **Event Bus**: NATS JetStream-based pub/sub
- **Event Types**: 15 types across 5 categories (agent, job, state, system, user)
- **Filtering**: Advanced CEL expression-based filtering
- **Routing**: Rule-based event routing with priorities
- **Reactors**: Automated event responses with throttling/debouncing
- **Storage**: SQLite-based event persistence with retention policies
- **Integration**: CloudEvents, Kafka adapters

**Key Achievements**:
- Real-time event streaming
- Complex filter expressions
- Built-in actions (webhook, command, event emission)
- Prometheus metrics for event system

---

### Epic 5: GitOps Integration ✅

**Status**: Complete | **Coverage**: >50%

Integrate with GitOps tools:

- **Webhook Receivers**: ArgoCD, Flux, GitHub, GitLab
- **API Clients**: Query deployment status, trigger operations
- **Verification Framework**: Post-deployment health checks
- **Rollback Automation**: Automatic and approval-based rollbacks
- **Promotion Pipelines**: Multi-environment deployment workflows
- **Git Sync**: Repository synchronization with state/reactor files

**Key Achievements**:
- Unified webhook handling for major GitOps tools
- Pluggable verification steps (HTTP, K8s, command)
- Progressive delivery (canary, blue/green)

---

### Epic 6: Policy Enforcement ✅

**Status**: Complete | **Coverage**: >79%

Policy-as-code for compliance:

- **Policy Engines**: OPA (Rego) and CEL support
- **Policy Registry**: Centralized policy management
- **Enforcement Modes**: Audit, Warn, Enforce
- **Policy Bindings**: Attach policies to resources/actions
- **Audit Logging**: Complete audit trail of policy decisions
- **Compliance Reporting**: Generate compliance reports

**Key Achievements**:
- Dual policy engine support (OPA + CEL)
- Flexible enforcement modes
- Comprehensive audit logging
- Compliance score tracking

---

### Epic 7: Observability ✅

**Status**: Complete | **Coverage**: >75%

Comprehensive monitoring and observability:

- **Metrics**: Prometheus metrics with 70+ metrics
- **Logging**: Structured logging with JSON/logfmt/text formatters
- **Tracing**: OpenTelemetry integration
- **TUI Monitor**: Real-time terminal dashboard (8 views)
- **Grafana Dashboards**: 6 pre-built dashboards
- **Health Checks**: Kubernetes-compatible probes
- **Profiling**: pprof endpoints for performance analysis
- **Query API**: Unified API for metrics, logs, traces

**Key Achievements**:
- Production-ready observability stack
- Real-time monitoring without external dependencies
- Comprehensive dashboard coverage

---

### Epic 8: Multi-Environment ✅

**Status**: Complete | **Coverage**: >80%

Support for diverse infrastructure:

- **Kubernetes**: Client wrapper, CRDs, operator controllers
- **VMs**: Platform detection, cross-platform modules
- **Bare Metal**: Hardware detection, IPMI/BMC integration
- **Edge**: Offline mode, local caching, connection resilience
- **Cloud Providers**: AWS, GCP, Azure metadata detection
- **Containers**: Docker, containerd runtime detection
- **Service Mesh**: Istio, Linkerd, Consul integration

**Key Achievements**:
- Unified API across all environment types
- Automatic platform detection
- Seamless edge-to-cloud operation

---

### Epic 9: Plugin System ✅

**Status**: Complete | **Coverage**: >70%

Extensible module system:

- **Runtimes**: Starlark (sandboxed Python-like) and WASM (wazero - pure Go)
- **Capability System**: 10 capability types with fine-grained permissions
- **Cryptographic Verification**: Hash and signature verification (RSA, ECDSA, Ed25519), Cosign support
- **Dependency Resolution**: SemVer constraints, MVS algorithm
- **Module CLI**: `kscore-module` with 10 commands (init, validate, build, sign, publish, install, resolve, tree, verify, test)
- **SDKs**: Starlark, Rust, Go (TinyGo), C++ SDKs
- **Standard Library**: 6 stdlib modules (files, exec, http, strings, json, crypto)
- **Module Loader**: 7-phase loading with capability policy and caching
- **Registry**: OCI registry client + HTTP registry server

**Key Achievements**:
- Secure, sandboxed plugin execution
- Complete SDK suite for multiple languages
- Reproducible builds with lock files
- Content-addressed caching
- Full module development CLI (kscore-module)
- Capability policy system for operator control

---

### Epic 10: Documentation ✅

**Status**: Complete | **Coverage**: 45 pages

Comprehensive Hugo + Docsy documentation:

- **Phase 1**: Infrastructure (Hugo + Docsy setup)
- **Phase 2**: Getting Started (4 pages)
- **Phase 3**: Core Concepts (10 pages)
- **Phase 4**: Reference Documentation (6 pages)
- **Phase 5**: Operations Guide (6 pages)
- **Phase 6**: Community Documentation (4 pages)
- **Phase 7**: Blog & Release Notes (5 pages)

**Key Achievements**:
- ~21,500 lines of documentation
- Complete coverage of all features
- Searchable documentation site
- PDF generation support

---

### Epic 11: High Availability Clustering ✅

**Status**: Complete | **Coverage**: >48%

Production-ready high availability:

- **etcd Integration**: Distributed consensus with embedded and external modes
- **Embedded etcd Mode**: In-process etcd for simpler deployments (3-5 nodes)
- **Leader Election**: etcd-based election with automatic failover
- **Work Distribution**: Consistent hashing for agent assignment
- **Cluster Membership**: Health monitoring and quorum management
- **Agent Handoff**: Seamless agent persistence across control plane failovers
- **Observability**: 12 cluster metrics, Grafana dashboard, 10 alert rules
- **CLI Tools**: `kscore-cluster status`, `members`, `leader`, `health`, `rebalance`

**Key Achievements**:
- Zero-downtime failovers
- Automatic agent reassignment on node failure
- Split-brain prevention with quorum checks
- Embedded etcd for simpler HA deployments
- Comprehensive cluster metrics and alerting

---

### Epic 12: E2E Testing ✅

**Status**: Complete

Comprehensive end-to-end testing framework:

- **Test Harness**: Docker-compose based environment management
- **HA Cluster Harness**: Multi-server environment with lifecycle control
- **Topologies**: All-in-one (dev) and HA Cluster (3 control planes + 5 agents)
- **Scenario Tests**: Agent lifecycle, remote execution, state management, events, policy, GitOps
- **Performance Tests**: Scale testing (100+ agents), throughput, latency percentiles
- **CI/CD**: GitHub Actions workflow for automated E2E tests

**Test Organization**:
```
test/e2e/
├── harness/          # Test environment management
├── containers/       # Docker-compose for all-in-one
├── topologies/       # HA cluster docker-compose
├── topology/         # Topology-specific tests
├── scenarios/        # Feature scenario tests
└── performance/      # Performance and scale tests
```

**Key Achievements**:
- Complete E2E test infrastructure
- HA cluster topology with NATS cluster + etcd + PostgreSQL
- Performance tests with JSON reports
- 6 comprehensive scenario test files
- CI/CD integration via GitHub Actions

---

### Epic 13: CGO Removal ✅

**Status**: Complete

Pure Go build for simplified cross-compilation:

- **SQLite**: Replaced `github.com/mattn/go-sqlite3` with `modernc.org/sqlite`
- **WASM Runtime**: Using `github.com/tetratelabs/wazero` (pure Go WebAssembly)

**Key Achievements**:
- `CGO_ENABLED=0 go build ./...` works
- Cross-compilation without toolchains
- Alpine/scratch Docker images without libc
- Simpler CI/CD (no gcc/clang required)

---

### Epic 14: NATS Mesh Communication ✅

**Status**: Complete | **Coverage**: >70%

NATS-only communication with advanced networking:

- **NATS-Only Communication**: All agent-server communication via NATS (gRPC retained for client API only)
- **Subject Namespace**: Cluster-prefixed subjects with category-based organization
- **Multi-Endpoint Support**: Priority-based failover, circuit breakers, health-based routing
- **Agent Embedded NATS**: Reverse connection mode for NAT traversal
- **Leaf Node Support**: Hub-spoke topology with local persistence during outages
- **Supercluster**: Multi-region gateway architecture with cross-cluster agent management
- **WebSocket Transport**: Firewall-friendly connections with HTTP proxy support
- **Discovery**: DNS, mDNS, Kubernetes, Consul, etcd-based endpoint discovery
- **Reliability**: Message buffering, delivery guarantees (at-most-once, at-least-once, exactly-once)
- **Observability**: 30+ NATS mesh metrics, Grafana dashboard, 25 alert rules

**Key Achievements**:
- Flexible deployment across NAT, firewalls, and complex networks
- Automatic endpoint discovery and failover
- Local message buffering during network outages
- Multi-region supercluster support
- Comprehensive observability and debugging tools

---

## Planned Epics

### Epic 15: Observability Enhancements

**Status**: Planned | **Depends on**: Epic 7, 14

Enhanced telemetry transport and logging:

- **NATS Telemetry Transport**: Route metrics, logs, traces over NATS mesh
- **Stdout/Stderr Logging**: Structured logging to stdout for container environments
- **Syslog Integration**: RFC 5424 syslog output for enterprise environments
- **CLI Audit Logging**: Comprehensive audit trail for all CLI operations
- **Telemetry Aggregation**: Aggregate telemetry from leaf nodes at hub

---

### Epic 16: Stdlib System Modules

**Status**: Planned | **Depends on**: Epic 3, 8

40+ cross-platform system management modules inspired by Salt Project:

- **Core**: Cross-platform user/group management (Linux, macOS, Windows)
- **Network**: Interface config, routes, firewall (iptables, nftables, pf, netsh)
- **Schedule**: cron, systemd timers, launchd, Windows Task Scheduler
- **Storage**: mount, swap, LVM, disk management
- **SSH**: authorized_keys, known_hosts, sshd_config
- **Security**: SELinux, AppArmor policy management
- **System**: timezone, locale, hostname, hosts, sysctl, kernel_module
- **Container**: Docker/Podman containers, images, networks, volumes
- **Database**: PostgreSQL, MySQL, Redis primitives
- **Web**: Nginx and Apache site/module management
- **VCS**: Git repository and config management
- **PKI**: X509 certificates, CA management, ACME/Let's Encrypt

---

### Epic 17: SPIFFE Identity Framework

**Status**: Planned | **Depends on**: Epic 1, 11, 14

Zero-configuration identity and mTLS:

- **Embedded Identity Provider**: Built-in SPIFFE identity for zero-configuration mTLS
- **Automatic CA Management**: Generation, rotation, and persistence of root CA
- **SVID Issuance**: Automatic SVID issuance and rotation for agents
- **NATS mTLS**: Use SVIDs for NATS authentication
- **External Provider Integration**: SPIRE Server, AWS IRSA, GCP Workload Identity, Azure MI
- **Service Mesh Integration**: Istio, Consul Connect, Linkerd trust federation
- **Trust Domain Federation**: Multi-cluster identity federation

---

### Epic 18: IPv6 Support

**Status**: Planned | **Depends on**: Epic 1, 11, 14

Full IPv6 and dual-stack support:

- **Control Plane**: IPv6 listening and client connections
- **NATS Mesh**: IPv6 endpoints, dual-stack discovery
- **Agent Communication**: IPv6 heartbeat and command channels
- **Targeting**: IPv6-aware agent targeting expressions
- **Health Checks**: IPv6 health check endpoints
- **Documentation**: IPv6 deployment guides

---

### Epic 19: Observability Gateway

**Status**: Planned | **Depends on**: Epic 7, 14, 15

Telemetry gateway for isolated agents:

- **Gateway Architecture**: Centralized telemetry collection point
- **Prometheus Bridge**: Expose agent metrics as Prometheus remote write
- **Loki Bridge**: Forward logs to Loki via the gateway
- **Tempo Bridge**: Forward traces to Tempo via the gateway
- **Buffering**: Local telemetry buffering during gateway outages
- **Aggregation**: Pre-aggregate metrics at the gateway
- **Security**: mTLS authentication for telemetry streams

---

### Epic 20: Windows Support

**Status**: Planned | **Depends on**: Epic 1, 2, 3, 13

Production-grade Windows agent:

- **Windows Service**: Run kscore-agent as Windows service
- **State Modules**: Windows-specific modules (registry, IIS, Windows features)
- **Package Management**: Chocolatey, winget, MSI integration
- **Service Management**: Windows service state module
- **User/Group Management**: Windows user and group modules via net commands
- **Firewall**: Windows Firewall state module
- **MSI Installer**: Windows installer package
- **Development Environment**: Windows dev setup documentation

---

## Future Considerations

These items are under consideration for future development:

#### Multi-Tenancy

- Namespace isolation
- Tenant-specific policies
- Resource quotas per tenant
- Cross-tenant visibility controls

#### Advanced Scheduling

- Time-based execution scheduling
- Maintenance windows
- Batch job orchestration
- Priority-based execution queues

#### Enhanced Security

- Hardware security module (HSM) integration
- Secrets management integration (Vault, AWS Secrets Manager)
- Zero-trust networking
- Advanced RBAC with attribute-based access control (ABAC)

#### User Experience

- Web UI dashboard
- Mobile monitoring app
- Natural language command interface
- Visual workflow builder

#### Integrations

- ServiceNow integration
- PagerDuty/Opsgenie direct integration
- Terraform provider
- Ansible collection
- Chef/Puppet migration tools

---

## Release Schedule

### Version Strategy

Keystone Core follows [Semantic Versioning](https://semver.org/):

- **Major versions (X.0.0)**: Breaking changes, major features
- **Minor versions (0.X.0)**: New features, backward compatible
- **Patch versions (0.0.X)**: Bug fixes, security patches

### Planned Releases

| Version | Target | Scope |
|---------|--------|-------|
| v0.14.0 | Current | NATS Mesh Communication complete |
| v0.15.0 | Next | Observability enhancements |
| v0.16.0 | Planned | Stdlib system modules |
| v1.0.0 | Future | Production-ready release (all core features complete) |

### Release Cadence

- **Minor releases**: Every 6-8 weeks
- **Patch releases**: As needed for critical fixes
- **Security releases**: Within 48 hours of disclosure

---

## Contributing to the Roadmap

### Suggest Features

Have an idea? We'd love to hear it:

1. **Check existing issues**: Search [GitHub Issues](https://github.com/shawnbutts/keystone-core/issues) for similar requests
2. **Open a feature request**: Use the feature request template
3. **Discuss on Discord**: Join the `#feature-requests` channel
4. **Submit an RFC**: For major features, write a design document

### Vote on Features

Help prioritize development:

- 👍 React to issues you want to see implemented
- 💬 Comment with your use case
- 📝 Provide detailed requirements

### Implement Features

Ready to contribute?

1. Check [good first issues](https://github.com/shawnbutts/keystone-core/labels/good%20first%20issue) for starter tasks
2. Claim an issue by commenting
3. Follow the [Contributing Guide](../contributing/)
4. Submit a pull request

---

## FAQ

### When will feature X be available?

We don't provide specific dates. The roadmap shows relative priorities and planned order. Join our Discord for the latest updates.

### How can I sponsor development of a specific feature?

Contact the maintainers via Discord or email to discuss sponsorship options for accelerating specific features.

### Why isn't feature X on the roadmap?

We may not have considered it! Please open a feature request issue. Features are prioritized based on community demand, strategic alignment, and implementation complexity.

### How often is the roadmap updated?

The roadmap is reviewed and updated monthly. Major changes are announced on Discord and in release notes.

---

## Stay Updated

- **GitHub Releases**: Watch the [repository](https://github.com/shawnbutts/keystone-core) for release notifications
- **Changelog**: See [CHANGELOG.md](https://github.com/shawnbutts/keystone-core/blob/main/CHANGELOG.md) for detailed release notes
- **Discord**: Join for real-time updates and discussions
- **Twitter**: Follow [@kscore](https://twitter.com/kscore) for announcements
