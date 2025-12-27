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
COMPLETED                    IN PROGRESS              PLANNED
─────────────────────────────────────────────────────────────────
Epic 1: Core Infrastructure  Epic 10: Documentation   Epic 11: Clustering
Epic 2: Remote Execution     (Phases 1-6 complete)    (HA with etcd)
Epic 3: State Management
Epic 4: Event System
Epic 5: GitOps Integration
Epic 6: Policy Enforcement
Epic 7: Observability
Epic 8: Multi-Environment
Epic 9: Plugin System
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

- **Runtimes**: Starlark (sandboxed Python-like) and WASM (Wasmtime)
- **Capability System**: 10 capability types with fine-grained permissions
- **Cryptographic Verification**: Cosign signatures, SumDB transparency log
- **Dependency Resolution**: SemVer constraints, MVS algorithm
- **Module Registry**: OCI registry with HTTP proxy
- **SDKs**: Starlark, Rust, Go (TinyGo), C++ SDKs
- **Standard Library**: 6 stdlib modules (files, exec, http, strings, json, crypto)
- **Module Loader**: 6-phase loading with caching

**Key Achievements**:
- Secure, sandboxed plugin execution
- Complete SDK suite for multiple languages
- Reproducible builds with lock files
- Content-addressed caching

---

## Current Work

### Epic 10: Documentation 🚧

**Status**: In Progress (Phases 1-6 of 7 complete)

Comprehensive Hugo + Docsy documentation:

**Completed**:
- ✅ Phase 1: Infrastructure (Hugo + Docsy setup)
- ✅ Phase 2: Getting Started (4 pages)
- ✅ Phase 3: Core Concepts (10 pages)
- ✅ Phase 4: Reference Documentation (6 pages)
- ✅ Phase 5: Operations Guide (6 pages)
- ✅ Phase 6: Community Documentation (4 pages)

**Remaining**:
- 🔲 Phase 7: Blog & Release Notes

**Progress**: ~32 documentation pages complete

---

## Planned Development

### Epic 11: High Availability Clustering

**Status**: Planned | **Priority**: High

Implement high-availability clustering for production deployments:

**Planned Features**:
- **etcd Integration**: Distributed consensus for leader election
- **Active-Passive Failover**: Automatic failover between control plane instances
- **Work Distribution**: Load balancing across control plane nodes
- **State Replication**: Consistent state across cluster members
- **Split-Brain Prevention**: Quorum-based decision making

**Design Goals**:
- Zero downtime during planned failovers
- Automatic recovery from node failures
- Horizontal scaling of control plane
- Geographic distribution support

---

### Future Considerations

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
| v0.10.0 | Current | Documentation complete, bug fixes |
| v0.11.0 | Next | Clustering foundation |
| v1.0.0 | Future | Production-ready release |

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
- **Discord**: Join for real-time updates and discussions
- **Twitter**: Follow [@kscore](https://twitter.com/kscore) for announcements
- **Blog**: Subscribe to the blog for detailed release notes and feature announcements
