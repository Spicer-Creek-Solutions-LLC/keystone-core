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

---

>## 🚧 Early Preview / Not Production Ready
>
> Keystone Core is under active development and **not yet suitable for production**.
>
> **We welcome early testers!**  
> Try it in your lab or homelab and please share feedback, open issues, or propose features
---

```
COMPLETED (Epics 1-31)                 PLANNED (Epics 32-34, 42, 100)
───────────────────────────────────────────────────────────────────────
Epic 1: Core Infrastructure            Epic 32: Cross-Platform Testing
                                       Epic 33: Multi-Cloud Real Env Testing
                                       Epic 34: Blueprint Marketplace
                                       Epic 42: Network Protocol Expansion

Epic 2: Remote Execution
Epic 30: CLI UX Restructuring
Epic 31: NIST Design Principles
Epic 3: State Management
                                       

                                       Future Considerations:
Epic 4: Event System                     - Multi-Tenancy & Namespace Isolation
Epic 5: GitOps Integration               - Scheduled Operations & Maintenance Windows
Epic 6: Policy Enforcement               - Web UI Dashboard
Epic 7: Observability                    - Automatic Drift Remediation
Epic 8: Multi-Environment                - Agent Self-Update
Epic 9: Plugin System                    - Compliance Framework Presets
Epic 10: Documentation                   - Network Discovery & Topology
Epic 11: HA Clustering                   - Runbook Automation
Epic 12: E2E Testing                     - Secrets Management (Vault, AWS SM)
Epic 13: CGO Removal                     - Terraform Provider
Epic 14: NATS Mesh Communication         - ServiceNow/ITSM Integration
Epic 15: Observability Enhancements      - Chef/Puppet Migration Tools
Epic 16: Stdlib System Modules           - Mobile Monitoring App
Epic 17: SPIFFE Identity Framework       - Natural Language Interface
Epic 18: IPv6 Support
Epic 19: Observability Gateway
Epic 20: Windows Support
Epic 21: Proxy Agents
Epic 22: File Distribution
Epic 23: Self-Management
Epic 24: Document Review
Epic 25: Blueprints
Epic 26: NEEDSWORK Remediation
Epic 27: Agent Bootstrap Experience
Epic 28: Standard Deployment Blueprints
Epic 29: Bootstrap Testing Infrastructure
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
- Comprehensive state persistence

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
- **Grafana Dashboards**: 10 pre-built dashboards
- **Health Checks**: Kubernetes-compatible probes
- **Profiling**: pprof endpoints for performance analysis
- **Query API**: Unified API for metrics, logs, traces

**Key Achievements**:

- Comprehensive observability stack
- Real-time monitoring without external dependencies
- Complete dashboard coverage

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

High availability clustering:

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

### Epic 15: Observability Enhancements ✅

**Status**: Complete | **Depends on**: Epic 7, 14

Enhanced telemetry transport and logging:

- **NATS Telemetry Transport**: Route metrics, logs, traces over NATS mesh
- **Stdout/Stderr Logging**: Structured logging to stdout for container environments
- **Syslog Integration**: RFC 5424 syslog output (Unix socket, UDP, TCP, TLS)
- **CLI Audit Logging**: Comprehensive audit trail with OS-native backends (journald, syslog, Windows Event Log)
- **TUI Monitor Integration**: Real-time NATS-based log/metrics streaming

**Key Achievements**:

- NATS-native telemetry for air-gapped environments
- Multi-backend audit logging
- Real-time TUI updates via NATS subscriptions

---

### Epic 16: Stdlib System Modules ✅

**Status**: Complete (94 modules) | **Depends on**: Epic 3, 8

Cross-platform system management modules inspired by Salt Project:

- **Core**: user, group (Linux, macOS, Windows)
- **Network**: network, route, firewall, iptables, nftables, firewalld
- **Kubernetes**: 12 modules (namespace, deployment, service, configmap, etc.)
- **Schedule**: cron, systemd_timer, launchd, scheduled_task, at
- **Storage**: mount, swap, lvm_pv, lvm_vg, lvm_lv, disk, filesystem
- **SSH**: authorized_keys, known_hosts, sshd_config
- **Security**: selinux, selinux_boolean, apparmor, apparmor_profile
- **System**: timezone, locale, hostname, hosts, sysctl, kernel_module
- **Container**: docker/podman containers, images, networks, volumes
- **Database**: postgresql_*, mysql_*, redis
- **Web**: nginx_*, apache_*, git, git_config
- **PKI**: x509, ca, acme
- **Config**: pip, npm, gem, ufw, alternatives, logrotate, sudoers, limits, modprobe, syslog, lineinfile, ini_file, archive

**Key Achievements**:

- 94 cross-platform modules
- Comprehensive test coverage
- Example state files for common scenarios

---

### Epic 17: SPIFFE Identity Framework ✅

**Status**: Complete (332 tests) | **Depends on**: Epic 1, 11, 14

Zero-configuration identity and mTLS:

- **Embedded Identity Provider**: Built-in SPIFFE identity with automatic CA management
- **SVID Issuance**: Automatic X.509 and JWT SVID issuance and rotation
- **Attestation**: Pluggable attestors (join_token, aws_iid, gcp_iit, azure_imds, k8s_sat)
- **NATS mTLS**: SVID-based NATS authentication
- **External Providers**: SPIRE Server, AWS IRSA, GCP Workload Identity, Azure MI
- **Service Mesh**: Istio, Consul Connect, Linkerd integration
- **Trust Federation**: Multi-cluster identity federation with policy control
- **Production Hardening**: AES-256-GCM encryption, CA rotation, LRU cache

**Key Achievements**:

- Zero-configuration mTLS out of the box
- Cloud provider attestation
- Trust domain federation
- 332 comprehensive tests

---

### Epic 18: IPv6 Support ✅

**Status**: Complete | **Depends on**: Epic 1, 11, 14

Full IPv6 and dual-stack support:

- **Control Plane**: IPv6 listening on gRPC and REST endpoints
- **NATS Mesh**: IPv6 endpoints, dual-stack discovery
- **Agent Communication**: IPv6 heartbeat and command channels
- **Targeting**: IPv6-aware agent targeting expressions
- **E2E Testing**: IPv6-only test topology

**Key Achievements**:

- Full dual-stack support
- IPv6-only deployment option
- E2E test coverage for IPv6

---

### Epic 19: Observability Gateway ✅

**Status**: Complete | **Depends on**: Epic 7, 14, 15

Telemetry gateway for isolated agents:

- **Metrics Gateway**: Aggregate agent metrics with cardinality control
- **Prometheus Integration**: /metrics and /federate endpoints, remote write client
- **Logs Gateway**: Level/source filtering, Loki pusher with batching
- **Traces Gateway**: Span-to-trace grouping, sampling, OTLP export
- **Deployment**: Docker Compose stack with Prometheus, Loki, Tempo, Grafana

**Key Achievements**:

- Bridge agents to standard observability backends
- Cardinality control for high-scale deployments
- Complete Docker Compose deployment

---

### Epic 20: Windows Support ✅

**Status**: Complete | **Depends on**: Epic 1, 2, 3, 13

Comprehensive Windows agent:

- **Windows Service**: Run kscore-agent as Windows service with recovery
- **Execution**: PowerShell and Cmd.exe with proper encoding
- **State Modules**: Registry, Windows Features (DISM), IIS, Windows Firewall
- **Package Management**: Chocolatey, winget, MSI/MSIX
- **File Operations**: NTFS ACLs, Windows paths, junctions, UNC paths
- **Installer**: WiX-based MSI installer with silent install support

**Key Achievements**:

- Full Windows service lifecycle management
- Windows-native state modules
- Enterprise deployment via MSI

---

### Epic 21: Proxy Agents ✅

**Status**: Complete | **Depends on**: Epic 1, 2, 14, 17

Manage devices that cannot run native agents:

- **Protocol Adapters**: SSH, SNMP v2c/v3, REST/HTTP, WinRM
- **Vendor Adapters**: Cisco IOS/NX-OS, Juniper JUNOS, Arista EOS, VyOS, pfSense, OPNsense
- **Credential Management**: Encrypted file storage, Vault, Kubernetes secrets, environment variables
- **State Modules**: Protocol-specific state modules (ssh_file, snmp_value, network device configs)
- **Discovery**: Network scanning with profile matching, auto-approval workflows
- **Observability**: 30+ metrics, Grafana dashboard, health monitoring

**Key Achievements**:

- Transparent targeting (proxied devices appear as virtual agents)
- Secure credential storage with rotation
- Network device configuration management
- Comprehensive discovery and auto-configuration

---

### Epic 22: File Distribution ✅

**Status**: Complete | **Depends on**: Epic 1, 4, 6, 14, 17, 21

NATS-based file distribution for agents:

- **File Server**: Dedicated file server with REST and NATS APIs
- **Storage Backends**: NATS JetStream Object Store, S3/GCS/Azure, local filesystem, Git
- **Mirror Groups**: Multi-region storage with geographic routing, sync, and failover
- **Transfer Protocol**: Chunked transfers up to 10GB with resume support
- **Security**: Policy-based access control, integrity verification (SHA-256)
- **Caching**: Proxy agent file caching for bandwidth reduction
- **Observability**: 241 tests, Grafana dashboard

**Key Achievements**:

- NATS-native file distribution (no HTTP required)
- Multiple storage backend support
- Geographic routing with failover
- Conflict detection and resolution

---

### Epic 23: Self-Management ✅

**Status**: Complete | **Depends on**: Epic 1, 3, 4, 5, 7, 11, 17, 22

Use Keystone Core to manage itself:

- **Bootstrap**: Deploy fresh clusters from seed configuration
- **Backup/Restore**: Full system backup to portable artifact, disaster recovery
- **Rolling Upgrades**: Zero-downtime upgrades with automatic rollback
- **Validation**: Comprehensive configuration validation
- **State Modules**: kscore_server, kscore_agent, kscore_nats, kscore_database, kscore_backup
- **Runbooks**: 9 operational runbook templates

**Key Achievements**:

- Zero-dependency bootstrap (embedded NATS + SQLite)
- Complete backup/restore with encryption
- Rolling and canary upgrade strategies
- Comprehensive operational runbooks

---

### Epic 24: Document Review ✅

**Status**: Complete | **Depends on**: Epic 10, all completed epics

Documentation validation and quality assurance:

- **Documentation Inventory**: Automated inventory of all docs and code
- **Link Validation**: Broken link detection
- **Example Validation**: Code example syntax validation (YAML, JSON, Go, Bash)
- **Gap Analysis**: Documentation coverage analysis with remediation plan

**Key Achievements**:

- 61 documentation files validated
- 2,103 code examples checked and fixed
- Comprehensive gap analysis with prioritized remediation plan

---

### Epic 25: Blueprints ✅

**Status**: Complete (Design) | **Depends on**: Epic 3, 4, 9, 22

Pre-packaged, reusable state collections (similar to Salt Formulas, Ansible Roles, Helm Charts):

- **Blueprint Structure**: YAML compositions with parameters, dependencies, secrets
- **Parameter System**: JSON Schema-based validation with defaults and types
- **Dependencies**: Soft (concurrent) and hard (sequential) dependency ordering
- **Versioning**: SemVer with breaking change detection
- **CLI**: kscore-blueprint commands (init, validate, build, publish, install, apply, rollback)

**Key Achievements**:

- Complete design specification
- Blueprint manifest format defined
- Parameter and secret handling designed
- Registry and distribution model specified

---

### Epic 27: Agent Bootstrap Experience ✅

**Status**: Complete | **Depends on**: Epic 23, 25

Single-binary bootstrap experience for demo, production, and full-scale deployments:

- **Interactive TUI**: Bubble Tea wizard with mode selection, config screens, and progress
- **CLI/Env/Config**: Full flag + environment variable parity with YAML config support
- **Install Engine**: Package repo setup, version pinning, rollback tracking, service setup
- **Certificates**: Self-signed CA, CSR generation, renewal scaffolding
- **Storage**: SQLite bootstrap, PostgreSQL validation, migration hook
- **NATS**: Embedded, external, cluster, and leaf configuration
- **Blueprints**: Parameterized hooks, verification, rendered state export
- **Verification**: Service, API, NATS/Postgres connectivity and diagnostics capture

**Key Achievements**:

- Single command `kscore-agent bootstrap` for end-to-end setup
- Automated validation and rollback for safer installs
- Diagnostics bundle for troubleshooting failed runs

---

### Epic 26: NEEDSWORK Remediation ✅

**Status**: Complete | **Depends on**: All completed epics

Address all issues identified in the comprehensive project review (NEEDSWORK.md):

- **Security**: Credential exposure fixes, TLS validation gaps, rate limiting
- **API Completeness**: Registry server completeness, cluster operations, backup orchestration
- **Testing**: Integration test coverage, race condition tests, E2E test completion
- **Documentation**: Godoc coverage, internal docs, API reference completion
- **Polish**: Error messages, code cleanup, dependency updates

**Key Achievements**:

- All Critical/High/Medium/Low items resolved or explicitly deferred
- CLI and audit logging standardized with output format parity
- Test flakiness reduced with shared mocks and wait helpers
- Documentation gaps closed across best practices and migrations

---

## Release Epics

### Epic 28: Standard Deployment Blueprints

**Status**: Complete | **Depends on**: Epic 25, 27

Official blueprints for all deployment scenarios (implements Epic 25 blueprint runtime):

- **Core Blueprints**: demo, production-cluster, enterprise-platform
- **Infrastructure Blueprints**: nats-cluster, postgres-ha
- **Observability Blueprints**: monitoring-stack (Prometheus/Loki/Tempo/Grafana), metrics-only
- **Security Blueprints**: security-baseline, identity-federation
- **Integration Blueprints**: gitops-integration, proxy-agents, file-distribution
- **Platform Blueprints**: kubernetes-operator, edge-deployment

14 total blueprints with full documentation and parameter validation.

**Status Update**: Implementation complete; publishing/testing deferred to Epic 100.

---

### Epic 29: Bootstrap Testing Infrastructure

**Status**: Complete | **Depends on**: Epic 27, 28

Comprehensive testing for the bootstrap experience:

- **Docker-Based Tests**: CI/CD integration, all bootstrap scenarios
- **VM-Based Tests**: Deferred to Epic 100 (0.1.0 release readiness)
- **Platform Matrix**: Ubuntu, Debian, RHEL, Rocky, Fedora, Alpine
- **Cluster Tests**: Multi-node formation, join scenarios, failover
- **Blueprint Tests**: All blueprints tested across platforms
- **GitHub Actions**: Automated CI/CD with nightly full test runs

---

### Epic 30: CLI UX Restructuring ✅

**Status**: Complete | **Depends on**: Epic 25, 26

Restructured CLI commands to improve user experience by splitting oversized commands into focused, purpose-specific tools:

- **Deprecation Framework**: Created `pkg/cli/deprecation/` with structured deprecation warnings, migration paths, and configurable suppression
- **Blueprint Split**: `kscore-blueprint` → 3 focused commands
  - `kscore-blueprint` - Core lifecycle (reduced to 8 subcommands)
  - `kscore-blueprint-publish` - Publication workflow (publish, sign, verify, versions, docs)
  - `kscore-blueprint-state` - State management (snapshot, rollback, diff)
- **Federation Extraction**: `kscore-federation` from `kscore-identity` (list, add, show, suspend, activate, remove, refresh, bundle)
- **Backup Extraction**: `kscore-cluster-backup` from `kscore-cluster` (backup, restore, list, verify, schedule)
- **Storage Extraction**: `kscore-files-storage` from `kscore-files` (backend, mirrors management)
- **Audit Extraction**: `kscore-audit` from `kscore-policy` (log, report, export, stats)
- **Webhook Elevation**: `kscore-webhook` from `kscore-gitops` (list, show, test, history, secrets)
- **Backward Compatibility**: All original commands retained with deprecation warnings pointing to new commands

**Key Achievements**:

- Split 6 oversized commands into 12 focused commands
- Created reusable deprecation framework for future migrations
- All commands follow 7±2 subcommand guideline
- Full backward compatibility with clear migration paths
- 21 total CLI binaries now built

---

### Epic 31: NIST 800-53 Design Principles ✅

**Status**: Complete | **Depends on**: Epic 26

Internal project policies, design philosophies, and architectural guardrails inspired by NIST 800-53:

- **Design Principles**: 8 core principles (Least Privilege, Defense in Depth, Fail Secure, Explicit Over Implicit, Auditability, Cryptographic Agility, Reproducible Builds, Trust Boundary Enforcement)
- **Documentation**: SECURITY-DESIGN.md, GLOSSARY.md, contributor security guidelines in CONTRIBUTING.md
- **Cryptographic Standards**: Approved algorithms, key sizes, TLS configurations with deprecation warnings
- **Audit Taxonomy**: Event types, required fields, retention recommendations, sensitive data handling
- **Code Review Checklists**: Security review checklists for each principle

**Key Achievements**:

- Security-conscious design thinking embedded in development process
- Clear contributor expectations for security-relevant decisions
- Comprehensive cryptographic standards with approved/acceptable/deprecated algorithms
- Complete audit event taxonomy with 30+ event types across 8 categories

---

## Future Considerations

These items are under consideration for future development:

### 0.1.0 Release Readiness

Final polish and release readiness for the 0.1.0 project announcement:

- Blueprint signing + registry verification for official catalog
- Version string normalization (reset project references to 0.1.0)
- Full audit of docs + examples for version consistency
- VM-based bootstrap validation on real hosts
- Release checklist and announcement notes

### Multi-Tenancy & Namespace Isolation

- Namespace isolation for multi-team environments
- Tenant-specific policies and resource quotas
- Cross-tenant visibility controls
- Per-tenant audit logging

### Scheduled Operations & Maintenance Windows

- Time-based execution scheduling
- Maintenance windows with automatic blackout periods
- Batch job orchestration
- Priority-based execution queues

### Web UI Dashboard

- React-based web dashboard
- Real-time agent monitoring
- State and drift visualization
- Policy compliance dashboards
- Job history and execution logs

### Automatic Drift Remediation

- Opt-in automatic fix for detected drift
- Approval workflows for critical changes
- Change management integration (ServiceNow, Jira)
- Rollback on remediation failure
- Remediation scheduling (maintenance windows)

### Agent Self-Update

- Secure binary distribution via NATS
- Staged rollouts (canary, percentage-based)
- Automatic rollback on health check failure
- Version pinning per environment
- Offline update packages for air-gapped environments

### Compliance Framework Presets

- CIS Benchmarks policy packs (Linux, Windows, Kubernetes)
- SOC 2 compliance policies
- HIPAA compliance policies
- PCI-DSS compliance policies
- Custom framework builder

### Network Discovery & Topology

- Automatic network scanning
- L2/L3 topology mapping
- Dependency visualization
- Change detection and alerting
- Integration with CMDB

### Runbook Automation

- Multi-step orchestration workflows
- Conditional branching (if/else, switch)
- Approval gates with escalation
- Parallel and sequential step execution
- Human-in-the-loop confirmation

### Disaster Recovery

- Full backup/restore of cluster state
- State export/import for migration
- Cross-region failover automation
- Point-in-time recovery
- Backup verification and testing

### ServiceNow/ITSM Integration

- Change request integration
- Incident creation from policy violations
- CMDB synchronization
- Approval workflow integration

### Secrets Management Integration

- HashiCorp Vault integration (KV, dynamic secrets)
- AWS Secrets Manager support
- Azure Key Vault support
- GCP Secret Manager support

### Terraform Provider

- Terraform provider for Keystone Core resources
- State file management
- Agent provisioning
- Policy-as-code from Terraform

### Chef/Puppet Migration Tools

- Chef cookbook to Keystone Core state converter
- Puppet manifest to Keystone Core state converter
- Migration assessment tools
- Dual-run mode for validation

### Mobile Monitoring App

- iOS and Android apps
- Push notifications for alerts
- Agent status overview
- Quick actions (restart, rerun)

### Natural Language Interface

- ChatGPT/Claude-powered natural language commands
- "Show me all agents with drift"
- "Apply the webserver state to production"
- Context-aware command suggestions

---

## Release Schedule

### Version Strategy

Keystone Core follows [Semantic Versioning](https://semver.org/):

- **Major versions (X.0.0)**: Breaking changes, major features
- **Minor versions (0.X.0)**: New features, backward compatible
- **Patch versions (0.0.X)**: Bug fixes, security patches

For detailed information on support windows, upgrade paths, and compatibility guarantees, see the [Compatibility & Support Policy](/docs/community/compatibility/).

### Planned Releases

| Version | Target | Scope |
|---------|--------|-------|
| v0.25.0 | Current | Epics 1-27 complete (Core through bootstrap experience) |
| v0.26.0 | Complete | NEEDSWORK Remediation (security, API, testing, documentation, polish) |
| v0.27.0 | Complete | Agent Bootstrap Experience (single-binary TUI-guided setup) |
| v0.28.0 | Complete | Standard Deployment Blueprints (14 official blueprints) |
| v0.29.0 | Complete | Bootstrap Testing Infrastructure (Docker-based coverage) |
| v0.30.0 | Complete | CLI UX Restructuring (split 6 oversized commands into 12 focused commands) |
| v0.31.0 | Complete | NIST 800-53 Design Principles (security design documentation) |
| v0.100.0 | Planned | 0.1.0 Release Readiness (signing, version reset, docs audit, VM validation) |
| v1.0.0 | Future | Stable release after comprehensive testing and hardening |

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
