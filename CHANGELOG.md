# Changelog

All notable changes to Keystone Core will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed
- Agent state propagation in HA cluster mode - all control plane servers now query shared PostgreSQL database
- HA cluster E2E test failures (TestHACluster_MemberStatus, TestHACluster_MultiMember, TestHACluster_Reconnection)
- TestHACluster_ContinuousOperation context cancellation timing issue
- NATS health check endpoint (changed from /healthz to /varz)

### Added
- NATS restart coordination with recovery actions (restart embedded, reconnect, failover, drain)
- State propagation handlers with version tracking for all 5 state types
- NATSController and StatePropagator interfaces for cluster coordination
- Cluster backup and restore functionality with full shard and config support
- Force light background on Mermaid diagrams in print CSS
- Mermaid diagram support in Pandoc PDF generator
- Cosign signature verification (ECDSA P-256, Ed25519, base64 signatures, bundle parsing)
- 5 new Kubernetes state modules: k8s_configmap, k8s_secret, k8s_service, k8s_ingress, k8s_statefulset
- Loki and Jaeger querying clients for observability
- NATS service discovery for Kubernetes, Consul, and etcd
- BMC/IPMI detection for bare metal servers
- Built-in policy type with 14 policies
- Executor user switching (RunAs) support
- mTLS authenticator for client certificate authentication
- JWT authenticator for API authentication
- macOS user and group management with dscl
- Git previous revision lookup for rollbacks
- ArgoCD revision history lookup
- Memory limit parsing for module loader
- Edge CPU tracking with gopsutil
- Agent CPU/memory/disk metrics with gopsutil
- Helm charts for Kubernetes deployment
- Raw Kubernetes manifests with Kustomize support
- Comprehensive module security documentation
- Capability policy and lock system for module security
- OCI registry client and kscore-registry server

### Changed
- Standardized domain to keystonecore.io
- Converted ASCII diagrams to Mermaid across all documentation
- Updated roadmap to reflect current project state

## [0.15.0] - 2025-01-XX

### Added - Epic 15: Observability Enhancements
- **Phase 1: Stdout-First Logging**
  - Removed file output option from logging configuration
  - Enhanced stdout output with structured JSON format
  - Environment variable configuration (KSCORE_LOG_LEVEL, KSCORE_LOG_FORMAT)
  - Updated logging factory with consistent logger creation

- **Phase 2: Syslog Integration**
  - RFC 5424 compliant syslog output (pkg/logging/syslog.go)
  - Multiple transport options: Unix socket, UDP, TCP, TLS
  - Facility/severity mapping from log levels
  - Cross-platform support (Linux, macOS, Windows Event Log)

- **Phase 3: CLI Audit Logging**
  - AuditLogger interface with structured audit entries (pkg/audit/audit.go)
  - OS-native audit backends (journald, syslog, Windows Event Log)
  - CLI tool integration (kscore-exec, kscore-state)
  - Sensitive data redaction in audit entries

- **Phase 4: NATS Log Transport**
  - NATSLogPublisher for log transport over NATS (pkg/logging/nats.go)
  - Subject hierarchy: kscore.telemetry.logs.{cluster}.{source}.{level}
  - In-memory buffering with configurable overflow policies
  - JetStream persistence support

- **Phase 5: NATS Metrics Push**
  - NATSMetricsPublisher for metrics transport (pkg/metrics/nats.go)
  - Prometheus and OpenMetrics format support
  - Configurable push interval
  - Works alongside /metrics pull endpoint

- **Phase 6: NATS Trace Export**
  - NATSTraceExporter for OTLP traces over NATS (pkg/tracing/nats_exporter.go)
  - Batch span export with configurable flush interval
  - Integration with existing OpenTelemetry tracing

- **Phase 7: TUI Monitor NATS Integration**
  - TelemetrySubscriber for real-time NATS updates (cmd/kscore-monitor/events/)
  - LogMsg, MetricMsg, TraceMsg, AuditMsg Bubble Tea messages
  - LogBuffer and MetricBuffer for data management
  - Real-time updates to Logs and Metrics views

- **Phase 8: Centralized Audit System**
  - NATS-based audit event publishing (pkg/audit/nats.go)
  - Audit event types: authentication, authorization, operations, administration
  - JetStream persistence with configurable retention

- **Phase 9: Documentation**
  - Updated observability concepts documentation
  - NATS telemetry architecture diagrams
  - CLI audit logging configuration guide

## [0.14.0] - 2025-01-XX

### Added - Epic 14: NATS Mesh Communication
- **Phase 1: NATS-Only Communication**
  - Subject namespace design with cluster-prefixed subjects
  - Message protocol enhancement with envelope, correlation IDs, and trace context
  - Deduplication tracker for at-least-once semantics
  - Server-to-server gRPC coordination channel
  - Secure agent bootstrap registration with time-limited credentials
  - Audit logging for bootstrap events

- **Phase 2: Multi-Endpoint Support**
  - Endpoint configuration with priority, TLS, and auth
  - Connection strategies: Direct, TLS, WebSocket, Leaf Node
  - Pooled connection manager with circuit breaker pattern
  - Health-based routing (Priority, RoundRobin, LeastLatency, Weighted, Random)
  - Per-endpoint connection metrics

- **Phase 3: Agent Embedded NATS**
  - Embedded NATS server in agents (Disabled, Standalone, Leaf modes)
  - Endpoint advertisement with automatic public IP detection
  - Server outbound connection to agent endpoints
  - Hybrid mode with automatic role selection based on network topology

- **Phase 4: Leaf Node Support**
  - Leaf node configuration (Leaf, Hub, Bridge roles)
  - Leaf node chains for multi-hop topologies
  - Local message buffering during outages
  - Automatic flush on reconnection

- **Phase 5: Supercluster Support**
  - Gateway configuration for cross-cluster communication
  - Gateway health monitoring
  - Subject routing across gateways
  - Cross-cluster agent management
  - Supercluster failover orchestration

- **Phase 6: WebSocket Transport**
  - WebSocket client for firewall-friendly connections
  - WebSocket server configuration for NATS
  - Proxy support with HTTP CONNECT tunneling
  - CORS and JWT cookie authentication

- **Phase 7: Discovery & Auto-Configuration**
  - DNS-based discovery with SRV records
  - Kubernetes discovery via EndpointSlices
  - Consul and etcd service registry discovery
  - Auto-configuration based on network topology

- **Phase 8: Reliability & Resilience**
  - Message buffering with size/count/age limits
  - Delivery guarantees: AtMostOnce, AtLeastOnce, ExactlyOnce
  - Advanced circuit breaker with failure rate thresholds
  - Graceful degradation with operation priority filtering

- **Phase 9: Observability**
  - 30+ NATS mesh metrics (connections, messages, buffers, topology)
  - Grafana dashboard for NATS mesh visualization
  - Connection debugging with timeline and message tracing
  - 25 alerting rules covering all failure modes

- **Phase 10: Documentation**
  - NATS mesh architecture documentation
  - Deployment guides for 6 topology patterns
  - Operations guide with troubleshooting and capacity planning
  - Complete API reference

## [0.13.0] - 2024-12-XX

### Added - Epic 13: CGO Removal
- Replaced `github.com/mattn/go-sqlite3` with `modernc.org/sqlite` (pure Go)
- Replaced `github.com/bytecodealliance/wasmtime-go` with `github.com/tetratelabs/wazero` (pure Go)
- Cross-compilation support for linux/amd64, linux/arm64, windows/amd64, darwin/arm64
- `CGO_ENABLED=0` builds for all platforms

### Changed
- SQLite driver name changed from `"sqlite3"` to `"sqlite"`
- WASM runtime now uses context timeouts instead of fuel metering

## [0.12.0] - 2024-12-XX

### Added - Epic 12: End-to-End & Performance Testing
- Test harness with Docker-compose based environment management
- HA cluster harness for multi-server testing
- All-in-one topology (1 server + 3 agents)
- HA cluster topology (3 control planes + 3 NATS + 3 etcd + PostgreSQL + 5 agents)
- Agent lifecycle E2E tests
- Remote execution E2E tests
- State management E2E tests
- Event system E2E tests
- Policy enforcement E2E tests
- GitOps webhook E2E tests
- Performance tests with latency percentiles (P50, P95, P99)
- CI/CD workflow for E2E tests (.github/workflows/e2e.yml)

## [0.11.0] - 2024-11-XX

### Added - Epic 11: High Availability Clustering
- **Phase 1: etcd Integration**
  - etcd v3 client wrapper with connection management
  - Cluster membership management with heartbeat
  - Cluster configuration and validation
  - State storage (StateStore, ClusterConfigStore, ShardStore)
  - Distributed locks and coordination primitives

- **Phase 2: Leader Election & Work Distribution**
  - etcd concurrency-based leader election
  - Consistent hashing for agent assignment
  - Shard rebalancing on membership changes

- **Phase 3: Failover & Recovery**
  - Automatic failover detection
  - Agent reassignment on member failure
  - State recovery from etcd
  - Split-brain prevention with quorum checks
  - Agent persistence across control plane restarts

- **Phase 4: Data Consistency**
  - Transaction support for atomic operations
  - Consistent reads through etcd

- **Phase 5: Cluster Operations**
  - kscore-cluster CLI plugin (status, members, leader, health, rebalance, remove)
  - Cluster REST API endpoints

- **Phase 6: Observability**
  - 12 cluster metrics (members, quorum, leader, rebalance, etcd operations)
  - Grafana dashboard for cluster health
  - 10 cluster alert rules

- **Phase 7: Testing**
  - Comprehensive unit tests for all cluster components
  - HA cluster E2E tests (formation, failover, quorum loss, rolling updates)

- Embedded etcd mode using `go.etcd.io/etcd/server/v3/embed`
- Automatic embedded server lifecycle management
- kscore-migrate CLI for SQLite to PostgreSQL migration
- PostgreSQL storage backend

## [0.10.0] - 2024-10-XX

### Added - Epic 10: Comprehensive Documentation
- Hugo + Docsy documentation site infrastructure
- **Getting Started** (4 pages): Overview, Installation, Quick Start, Architecture
- **Core Concepts** (10 pages): Control Plane, Agents, Message Bus, State Management, Remote Execution, Events, Reactors, GitOps, Policy, Observability
- **Reference** (6 pages): API, CLI, Configuration, Modules, Events, Metrics
- **Operations** (6 pages): Deployment, Monitoring, Maintenance, Troubleshooting, Security, Registry
- **Community** (4 pages): Contributing, Development, Roadmap, Support
- 40 documentation pages totaling ~20,800 lines
- GoReleaser configuration for release builds

## [0.9.0] - 2024-09-XX

### Added - Epic 9: Plugin System & Extensibility
- **Phase 1: Runtime Foundation**
  - kscorectl plugin dispatcher (Git-style plugin architecture)
  - Starlark runtime with sandboxed execution
  - WASM runtime with wazero (Wasmtime integration)
  - Module manifest parser (module.yaml, module.lock)

- **Phase 2: Capability System**
  - 10 capability types: fs.read, fs.write, http.get, http.post, exec, secrets.read, secrets.write, log, time, kv
  - Path/domain/command scoping for security
  - Rate limiting and resource limits
  - Pluggable backends (SecretsStore, Logger, KVStore)

- **Phase 3: Cryptographic Verification**
  - Hash verification (SHA256, SHA512)
  - Digital signature verification (RSA, ECDSA, Ed25519)
  - SumDB transparency log client
  - Trust policy system with key fingerprinting

- **Phase 4: Dependency Resolution**
  - SemVer 2.0.0 compliant version handling
  - DAG-based dependency graph with cycle detection
  - MVS (Minimum Version Selection) algorithm
  - Content-addressed module cache with eviction policies

- **Phase 5: Policy Integration**
  - Trust-based capability enforcement (6 trust levels)
  - Environment-specific policy restrictions
  - Custom rule system with flexible conditions

- **Phase 6: SDKs & Stdlib**
  - Starlark SDK with testing framework
  - Rust SDK for WASM modules
  - Go SDK for WASM modules (TinyGo)
  - C++ SDK for WASM modules
  - 6 stdlib modules (files, exec, http, strings, json, crypto)
  - Hello world examples in all languages

- **Phase 7: Module Loader**
  - 7-phase module loading workflow
  - Capability wiring to Starlark and WASM runtimes
  - Capability policy and lock system
  - LRU caching with TTL

- kscore-module CLI (init, validate, build, sign, publish, install, resolve, tree, verify, test)
- kscore-policy CLI (check, enforce, audit, report)
- kscore-gitops CLI (verify, rollback, sync, diff)

### Added - Epic 8: Multi-Environment Support
- **Phase 1: Kubernetes Integration**
  - Kubernetes client wrapper with multi-cluster support
  - RemoteExecution and StateConfig CRDs
  - Operator controllers with reconciliation loops
  - k8s_namespace and k8s_deployment state modules

- **Phase 2: VM Support**
  - Platform detection (OS, distribution, package manager, init system)
  - Virtualization and container detection
  - Cross-platform module adapters

- **Phase 3: Bare Metal Support**
  - Hardware detection (CPU, memory, disk, network)
  - BMC/IPMI detection
  - Extended agent metadata

- **Phase 4: Edge Support**
  - Offline mode with local buffering
  - Connection resilience with exponential backoff
  - Resource constraint handling

- **Phase 5: Cloud Integration**
  - AWS integration (EC2, ECS, Lambda with IMDSv2)
  - GCP integration (Compute Engine, GKE, Cloud Functions)
  - Azure integration (VMs, AKS, Azure Functions)
  - Multi-cloud detector with caching

- **Phase 6: Container & Service Mesh**
  - Container runtime detection (Docker, containerd)
  - Service mesh integration (Istio, Linkerd, Consul)
  - SPIFFE ID extraction

## [0.8.0] - 2024-08-XX

### Added - Epic 7: Observability & Monitoring
- **Phase 1: Metrics**
  - Prometheus metrics infrastructure
  - 28 standard Keystone Core metrics
  - Specialized collectors (ControlPlane, Agent, State, GitOps, Policy)

- **Phase 2: Logging**
  - Structured logging with Logger interface
  - JSON, Logfmt, and Text formatters
  - Correlation ID management
  - Log level filtering and sampling

- **Phase 3: Tracing**
  - OpenTelemetry integration with OTLP exporter
  - Distributed trace context propagation
  - Instrumentation for control plane, state, events, and policy

- **Phase 4: TUI Monitor (kscore-monitor)**
  - 8 interactive views: Dashboard, Agents, Events, State Drift, Policy, Jobs, Logs, Metrics
  - Built with Bubble Tea framework
  - Real-time updates via NATS JetStream and gRPC

- **Phase 5: Dashboards**
  - 6 Grafana dashboards (Overview, Control Plane, Agent Fleet, State, Policy, GitOps)
  - 25+ Prometheus alert rules

- **Phase 6: Health & Status**
  - Health check manager with pluggable checkers
  - Circuit breaker pattern for fault tolerance
  - HTTP endpoints (/health/live, /health/ready, /health/status)

- **Phase 7: Advanced Features**
  - Performance profiling with pprof endpoints
  - Query API for metrics, logs, and traces
  - Infrastructure visualization with topology and graph APIs

## [0.7.0] - 2024-07-XX

### Added - Epic 6: Policy Enforcement
- Policy types and enums (OPA, CEL, Builtin)
- Policy categories (Security, Compliance, Operational, Cost, Custom)
- Policy registry with sets and bindings
- OPA evaluator for Rego policies
- CEL evaluator for Common Expression Language
- Policy engine orchestrating evaluation
- Policy enforcement layer with enforcement points
- Integration with state management and event system
- Policy auditing with ring buffer
- Compliance reporting with period-based analysis

## [0.6.0] - 2024-06-XX

### Added - Epic 5: GitOps Integration
- **Phase 1: Webhook Infrastructure**
  - Webhook receiver HTTP server
  - Authentication (None, HMAC, Bearer token)
  - Handlers for ArgoCD, Flux, GitHub, GitLab

- **Phase 2: GitOps Tool Integration**
  - ArgoCD API client (status, sync, rollback)
  - Flux client (Kustomization, HelmRelease)
  - GitHub client (PRs, commit statuses)
  - GitLab client (MRs, commit statuses)

- **Phase 3: Verification Framework**
  - Verification engine with sequential/parallel execution
  - HTTP health check verifier
  - Kubernetes resource verifier
  - Command and script verifiers

- **Phase 4: Git Sync**
  - Git repository client (clone, sync, commit, push)
  - Repository manager for multiple repos
  - HTTPS and SSH authentication

- **Phase 5: Rollback Automation**
  - Rollback engine with approval workflows
  - ArgoCD and Git executors
  - Rollback strategies (Previous, Specific, LastKnownGood)

- **Phase 6: Promotion Pipelines**
  - Promotion engine with multi-environment support
  - Strategies: BlueGreen, Canary, Rolling, Immediate
  - Progressive delivery with canary steps

## [0.5.0] - 2024-05-XX

### Added - Epic 4: Event-Driven Automation System
- **Week 1: Event Bus Foundation**
  - Event schema with 15 event types
  - JetStream publisher and subscriber
  - Event manager for simplified API

- **Week 2: Event Emission**
  - State management event emission
  - Control plane agent events
  - Job execution events
  - Correlation ID support

- **Week 3: Filtering and Routing**
  - Advanced filter expression parser
  - Event router with routing rules
  - Fan-out patterns for multiple consumers
  - Event enrichment pipeline

- **Week 4-5: Reactor System**
  - Reactor engine for automated responses
  - Built-in actions (Log, Event, Webhook, Command, Function)
  - Throttling and debouncing
  - Error handling strategies

- **Week 6: External Integration**
  - CloudEvents 1.0 adapter
  - Kafka publisher and subscriber
  - Event bridge for external systems
  - HTTP event receiver for webhooks

- **Week 7: Event Storage**
  - SQLite event store with indexes
  - Query API with filtering and pagination
  - Retention policies (age, count, severity)
  - Event replay capabilities

- **Week 8: Monitoring**
  - Metrics collector for event operations
  - Health check system
  - Prometheus exporter
  - Human-readable metrics summary

## [0.4.0] - 2024-04-XX

### Added - Epic 3: State Management & Configuration
- **Week 1: State Definition & Parsing**
  - State file types (StateFile, StateDeclaration, Requisites)
  - YAML parser with includes
  - Schema-based validation
  - Six module types: file, package, service, user, group, cmd

- **Week 2: State Modules & Execution**
  - Module interface (Check, Apply, Test)
  - Module registry
  - Idempotent execution with dry-run support
  - Retry logic with backoff

- **Week 3: Dependency Resolution & Templating**
  - DAG construction with Kahn's algorithm
  - Circular dependency detection
  - Go text/template integration
  - Vars and facts systems

- **Week 4: Drift Detection & CLI**
  - State comparison/diff engine
  - Drift severity levels
  - kscore-state CLI (apply, check, drift)

## [0.3.0] - 2024-03-XX

### Added - Epic 2: Remote Execution
- **Week 1: Foundation**
  - Git-style plugin system
  - Cross-platform shell abstraction (Bash, PowerShell, Cmd)
  - Enhanced executor with streaming output

- **Week 2: Targeting System**
  - Expression parser for targeting
  - Agent matcher with glob and expression filters
  - Batch execution framework

- **Week 3: Integration**
  - Protobuf definitions for batch operations
  - Control plane dispatch
  - Batch job tracking and state management

- **Week 4: CLI & Testing**
  - kscore-exec CLI plugin
  - Integration tests for batch execution

## [0.2.0] - 2024-02-XX

### Added - Epic 1: Core Infrastructure
- **Phase 1: NATS Integration**
  - Embedded NATS mode for zero-dependency deployment
  - External NATS cluster support
  - Leaf node mode for hybrid deployments
  - JetStream for event persistence

- **Phase 2: Agent Development**
  - Agent registration and heartbeat
  - Command execution with streaming output
  - System metadata collection

- **Phase 3: Control Plane Services**
  - Connection manager for agent lifecycle
  - SQLite-based state storage
  - gRPC API server

- **Phase 4: Testing & Reliability**
  - >80% test coverage across core packages
  - Comprehensive error handling

## [0.1.0] - 2024-01-XX

### Added
- Initial project structure
- Design documents (DESIGN.md)
- Epic planning documents (epics/)
- SPIFFE/SPIRE security architecture

---

[Unreleased]: https://github.com/keystone-core/keystone-core/compare/v0.15.0...HEAD
[0.15.0]: https://github.com/keystone-core/keystone-core/compare/v0.14.0...v0.15.0
[0.14.0]: https://github.com/keystone-core/keystone-core/compare/v0.13.0...v0.14.0
[0.13.0]: https://github.com/keystone-core/keystone-core/compare/v0.12.0...v0.13.0
[0.12.0]: https://github.com/keystone-core/keystone-core/compare/v0.11.0...v0.12.0
[0.11.0]: https://github.com/keystone-core/keystone-core/compare/v0.10.0...v0.11.0
[0.10.0]: https://github.com/keystone-core/keystone-core/compare/v0.9.0...v0.10.0
[0.9.0]: https://github.com/keystone-core/keystone-core/compare/v0.8.0...v0.9.0
[0.8.0]: https://github.com/keystone-core/keystone-core/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/keystone-core/keystone-core/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/keystone-core/keystone-core/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/keystone-core/keystone-core/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/keystone-core/keystone-core/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/keystone-core/keystone-core/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/keystone-core/keystone-core/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/keystone-core/keystone-core/releases/tag/v0.1.0
