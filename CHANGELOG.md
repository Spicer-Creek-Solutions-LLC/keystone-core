# Changelog

All notable changes to Keystone Core will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

All development to date. No releases have been made yet.

### Epic 20: Windows Support

Production-grade Windows agent support.

- **Phase 1: Windows Service**
  - Windows service implementation using golang.org/x/sys/windows/svc
  - Service install, uninstall, start, stop, restart commands
  - Automatic recovery on failure
  - Event log integration

- **Phase 2: PowerShell & Cmd Execution**
  - PowerShell script execution with proper encoding
  - Cmd.exe support for legacy scripts
  - Environment variable handling
  - Working directory management

- **Phase 3: Windows State Modules**
  - Registry module for key/value management
  - Windows Features module (DISM integration)
  - IIS module for web server management
  - Windows Firewall module

- **Phase 4: Package Management**
  - Chocolatey package provider
  - winget package provider
  - MSI/MSIX installation support

- **Phase 5: File Operations**
  - NTFS permission handling (ACLs)
  - Windows path normalization
  - Junction and symlink support
  - UNC path support

- **Phase 6: MSI Installer**
  - WiX-based MSI installer
  - Silent installation support
  - Upgrade and uninstall support
  - Custom actions for configuration

- **Phase 7: Documentation & Testing**
  - Windows development environment guide
  - Windows-specific troubleshooting
  - E2E tests on Windows
  - CI/CD integration for Windows builds

### Epic 19: Observability Gateway

Telemetry gateway for aggregating metrics, logs, and traces from isolated agents.

- **Metrics Gateway**
  - MetricsStore for aggregating agent metrics
  - Agent tracking with labels and metadata
  - Cardinality control (MaxSeries, MaxLabelsPerSeries)
  - Stale agent removal

- **Prometheus Integration**
  - /metrics endpoint for Prometheus scraping
  - /federate endpoint for federation
  - /health and /ready endpoints
  - Remote write client with retry logic

- **Logs Gateway**
  - LogsStore with level and source filtering
  - Query API with time range, search, labels
  - Loki pusher with batch processing
  - Multi-tenant support

- **Traces Gateway**
  - TracesStore grouping spans into traces
  - Sampling with error and slow threshold priority
  - OTLP exporter for Tempo/Jaeger
  - Gzip compression support

- **Deployment**
  - Docker Compose stack with full observability suite
  - Grafana dashboard for gateway monitoring
  - kscore-telemetry-gateway binary

### Epic 18: IPv6 Support

Full IPv6 and dual-stack networking support.

- **Control Plane IPv6**
  - IPv6 listening on gRPC and REST endpoints
  - Dual-stack support (IPv4 + IPv6)
  - IPv6 client connections

- **NATS Mesh IPv6**
  - IPv6 NATS endpoints
  - Dual-stack discovery
  - IPv6 leaf node connections
  - IPv6 gateway connections

- **Agent IPv6**
  - IPv6 heartbeat and command channels
  - IPv6 address in agent metadata
  - IPv6-aware targeting expressions

- **E2E Testing**
  - IPv6-only test topology
  - Dual-stack test topology
  - CI/CD IPv6 testing

### Epic 17: SPIFFE Identity Framework

Zero-configuration SPIFFE identity for secure agent-to-server communication.

- **Phase 1: Embedded Identity Provider Foundation**
  - Core types: SPIFFEID, X509SVID, JWTSVID, TrustBundle, JWTAuthority
  - Identity Provider interfaces and lifecycle management
  - Certificate Authority Manager with two-tier CA hierarchy (root + signing)
  - Support for ECDSA P-256/P-384 and RSA 2048/4096 key types
  - CA persistence to disk with automatic reload
  - Attestation Engine with pluggable attestors
  - Built-in attestors: join_token, aws_iid, gcp_iit, azure_imds, k8s_sat, none
  - Join token store with TTL-based expiration
  - SVID Issuer Service for X.509 and JWT SVIDs
  - Automatic SVID rotation with configurable threshold
  - Agent Identity Client for attestation and SVID management
  - NATS mTLS integration with SVID-based authentication
  - SPIFFE ID-based NATS subject authorization
  - Default agent/server authorization rules
  - Comprehensive user documentation with agent auto-registration guide
  - 54 unit tests covering all components

- **Phase 2: Production Hardening**
  - CA Security Hardening
    - AES-256-GCM encrypted key storage at rest
    - Key Encryption Key (KEK) from env var, file, or direct config
    - HSM support via PKCS#11 interface (stub for integration)
    - Automatic CA rotation with configurable threshold and overlap duration
    - Dual-signing support during CA rotation transitions
    - Encrypted CA backup and restore with SHA-256 integrity verification
  - SVID Rotation Robustness
    - Retry strategies: exponential backoff, linear, constant
    - Configurable jitter to prevent thundering herd
    - Grace period for continuing with old SVID during rotation
    - Connection draining before SVID switchover
    - Comprehensive rotation metrics (total, success, failure, retries)
    - State machine: idle, rotating, draining, retrying, failed, grace_period
  - High Availability Identity Provider
    - Multi-server identity coordination
    - Leader election for SVID issuance
    - State replication modes: sync, async, semi-sync
    - Trust bundle synchronization across cluster
    - Peer health monitoring
  - Performance Optimization
    - LRU SVID cache with configurable size and TTL
    - Pre-rotation buffer eviction
    - Cache metrics (hits, misses, evictions, hit rate)
    - Batch SVID issuance for efficiency
    - Connection pooling with health checks
  - Documentation updated with Production Hardening section
  - 36 unit tests covering all Phase 2 components

### Epic 16: Standard Library System Modules (84 modules)

Cross-platform system management modules inspired by Salt Project.

- **Phase 1: Cross-Platform User/Group**
  - Windows user management using `net user` command
  - Windows group management using `net localgroup` command
  - Full platform support: Linux (useradd/groupadd), macOS (dscl), Windows (net)

- **Phase 2: Network Configuration**
  - Network module with auto-detection of network managers
  - Linux: NetworkManager, netplan, ifupdown, systemd-networkd
  - macOS: networksetup, Windows: netsh
  - Route module for static route management

- **Phase 3: Firewall Management**
  - Cross-platform firewall abstraction
  - Linux: iptables, nftables, firewalld modules
  - macOS: pf, Windows: netsh advfirewall

- **Phase 4: Kubernetes Resources**
  - k8s_namespace, k8s_deployment, k8s_service, k8s_configmap, k8s_secret
  - k8s_ingress, k8s_statefulset, k8s_daemonset, k8s_job, k8s_cronjob
  - k8s_pvc, k8s_hpa (12 modules total)

- **Phase 5: Scheduled Tasks**
  - cron, systemd_timer, launchd, scheduled_task (Windows), at modules

- **Phase 6: Storage Management**
  - mount, swap, lvm_pv, lvm_vg, lvm_lv, disk, filesystem modules

- **Phase 7: SSH Configuration**
  - authorized_keys, known_hosts, sshd_config modules

- **Phase 8: Security Modules**
  - selinux, selinux_boolean, apparmor, apparmor_profile modules

- **Phase 9: System Configuration**
  - timezone, locale, hostname, hosts, sysctl, kernel_module modules

- **Phase 10: Container Management**
  - docker_container, docker_image, docker_network, docker_volume
  - podman_container, podman_image, podman_network, podman_volume

- **Phase 11: Database Primitives**
  - postgresql_database, postgresql_user, postgresql_extension
  - mysql_database, mysql_user, redis modules

- **Phase 12: Web Server & VCS**
  - nginx_site, nginx_config, nginx_upstream, nginx_proxy, nginx_ssl, nginx_location, nginx_rate_limit
  - apache_site, apache_module
  - git, git_config modules
  - x509, ca, acme certificate modules

- **Phase 13: Language Package Managers & Config**
  - pip, npm, gem modules
  - ufw, alternatives modules
  - logrotate, sudoers, limits, modprobe, syslog modules
  - lineinfile, ini_file, archive modules

- **Phase 14: Testing & Documentation**
  - Unit tests for cmd, package, service, cron modules
  - Module reference documentation for all 84 modules
  - Example state files (webserver, python-app, firewall, kubernetes, docker)

### Epic 15: Observability Enhancements

- Stdout-first logging with structured JSON format
- RFC 5424 compliant syslog output (Unix socket, UDP, TCP, TLS)
- CLI audit logging with OS-native backends (journald, syslog, Windows Event Log)
- NATS log transport with JetStream persistence
- NATS metrics push alongside /metrics pull endpoint
- NATS trace export for OTLP traces
- TUI monitor NATS integration for real-time updates

### Epic 14: NATS Mesh Communication

- **Phase 1: NATS-Only Communication**
  - Subject namespace design with cluster prefixes
  - Message envelope with correlation IDs and trace context
  - Server-to-server gRPC coordination channel
  - Secure agent bootstrap registration with time-limited credentials

- **Phase 2: Multi-Endpoint Support**
  - Connection strategies: Direct, TLS, WebSocket, LeafNode
  - Health-based routing (Priority, RoundRobin, LeastLatency, Weighted)
  - Connection metrics and circuit breaker

- **Phase 3: Agent Embedded NATS**
  - Embedded NATS server modes: Disabled, Standalone, Leaf
  - Endpoint advertisement with automatic IP detection
  - Hybrid mode manager for automatic role selection

- **Phase 4: Leaf Node Support**
  - Leaf node configuration for hub-spoke topologies
  - Message buffering during outages with JetStream persistence
  - Multi-hop leaf chains

- **Phase 5: Supercluster Support**
  - Gateway configuration for cross-cluster communication
  - Cross-cluster agent management
  - Supercluster failover

- **Phase 6: WebSocket Transport**
  - WebSocket client with TLS and compression
  - Proxy support with NTLM authentication

- **Phase 7: Discovery & Auto-Configuration**
  - DNS, mDNS, Kubernetes, Consul, etcd discovery
  - Auto-configuration based on network topology

- **Phase 8: Reliability & Resilience**
  - Delivery guarantees: AtMostOnce, AtLeastOnce, ExactlyOnce
  - Circuit breaker and graceful degradation

- **Phase 9-10: Observability & Documentation**
  - 30+ NATS mesh metrics
  - Grafana dashboard and 25 alert rules
  - Comprehensive deployment and operations guides

### Epic 13: CGO Removal

- Replaced `github.com/mattn/go-sqlite3` with `modernc.org/sqlite` (pure Go)
- Replaced `github.com/bytecodealliance/wasmtime-go` with `github.com/tetratelabs/wazero` (pure Go)
- Cross-compilation support for linux/amd64, linux/arm64, windows/amd64, darwin/arm64
- `CGO_ENABLED=0` builds for all platforms

### Epic 12: End-to-End & Performance Testing

- Test harness with Docker-compose based environment management
- HA cluster harness for multi-server testing
- All-in-one topology (1 server + 3 agents)
- HA cluster topology (3 control planes + 3 NATS + 3 etcd + PostgreSQL + 5 agents)
- E2E tests: agent lifecycle, remote execution, state management, events, policy, GitOps
- Performance tests with latency percentiles (P50, P95, P99)
- CI/CD workflow for E2E tests

### Epic 11: High Availability Clustering

- etcd v3 client wrapper with connection management
- Cluster membership management with heartbeat
- Leader election with etcd concurrency primitives
- Consistent hashing for agent-to-server assignment
- Automatic failover and agent reassignment
- Split-brain prevention with quorum checks
- kscore-cluster CLI plugin
- Cluster REST API endpoints
- 12 cluster metrics with Grafana dashboard
- Embedded etcd mode
- kscore-migrate CLI for SQLite to PostgreSQL migration
- PostgreSQL storage backend

### Epic 10: Comprehensive Documentation

- Hugo + Docsy documentation site
- Getting Started (4 pages): Overview, Installation, Quick Start, Architecture
- Core Concepts (10 pages): Control Plane, Agents, Message Bus, State, Execution, Events, Reactors, GitOps, Policy, Observability
- Reference (6 pages): API, CLI, Configuration, Modules, Events, Metrics
- Operations (6 pages): Deployment, Monitoring, Maintenance, Troubleshooting, Security, Registry
- Community (4 pages): Contributing, Development, Roadmap, Support
- 40+ documentation pages totaling ~20,800 lines

### Epic 9: Plugin System & Extensibility

- kscorectl plugin dispatcher (Git-style plugin architecture)
- Starlark runtime with sandboxed execution
- WASM runtime with wazero
- Module manifest parser (module.yaml, module.lock)
- 10 capability types: fs.read, fs.write, http.get, http.post, exec, secrets.read, secrets.write, log, time, kv
- Cryptographic verification (hash, signature, SumDB)
- SemVer constraint solver with MVS algorithm
- Content-addressed module cache
- OCI registry client and HTTP proxy client
- kscore-registry server
- kscore-module CLI (init, build, sign, publish, install, resolve, verify, test)
- SDKs: Starlark, Rust, Go (TinyGo), C++
- 6 stdlib modules: files, exec, http, strings, json, crypto

### Epic 8: Multi-Environment Support

- Kubernetes integration with CRDs (RemoteExecution, StateConfig)
- Platform detection (OS, distribution, package manager, init system)
- Hardware detection with gopsutil (CPU, memory, disk, network)
- BMC/IPMI detection for bare metal
- Edge support with offline mode and local caching
- Cloud integration: AWS, GCP, Azure metadata detection
- Container runtime detection (Docker, containerd)
- Service mesh integration (Istio, Linkerd, Consul)

### Epic 7: Observability & Monitoring

- Prometheus metrics with 70+ standard metrics
- Structured logging with JSON, logfmt, text formatters
- OpenTelemetry tracing integration
- kscore-monitor TUI with 8 views (Dashboard, Agents, Events, State, Policy, Jobs, Logs, Metrics)
- 6 Grafana dashboards (Overview, Control Plane, Agent Fleet, State, Policy, GitOps)
- Health check system with liveness/readiness probes
- Circuit breaker pattern for fault tolerance
- Performance profiling with pprof
- Query API for metrics, logs, traces

### Epic 6: Policy Enforcement

- OPA (Rego) policy evaluator
- CEL policy evaluator
- Policy engine with registry and bindings
- Enforcement modes: Enforce, Audit, Warn
- Policy auditing with ring buffer storage
- Compliance reporting with severity analysis
- 14 built-in policies
- kscore-policy CLI

### Epic 5: GitOps Integration

- Webhook receiver for ArgoCD, Flux, GitHub, GitLab
- ArgoCD, Flux, GitHub, GitLab API clients
- Verification engine with HTTP, K8s, command checks
- Git repository sync with change detection
- Rollback engine with approval workflows
- Promotion pipelines (Immediate, Canary, BlueGreen, Rolling)
- kscore-gitops CLI

### Epic 4: Event-Driven Automation

- NATS JetStream event bus
- 15 event types across 5 categories
- Filter expression parser with comparison and logical operators
- Event router with priority-based rules
- Event enrichment pipeline
- Reactor engine with throttling and debouncing
- Built-in actions: Log, Event, Webhook, Command, Function
- CloudEvents 1.0 adapter
- Kafka publisher and subscriber
- SQLite event storage with retention policies
- Prometheus metrics exporter

### Epic 3: State Management

- State file parser with YAML and includes
- Schema-based validation
- 6 core modules: file, package, service, user, group, cmd
- Dependency graph with topological sort
- Template rendering with Go text/template
- Vars and facts system
- Drift detection with severity levels
- kscore-state CLI (apply, check, drift)

### Epic 2: Remote Execution

- Git-style plugin architecture
- Cross-platform shell abstraction (Bash, PowerShell, Cmd)
- Targeting with glob and expression-based filtering
- Batch execution across multiple agents
- kscore-exec CLI with streaming output

### Epic 1: Core Infrastructure

- NATS integration (embedded, external, leaf modes)
- Agent registration, heartbeat, command execution
- Control plane services (state management, connection management)
- SQLite storage backend
- >80% test coverage across core packages

### Security Fixes

- **SQL Injection Prevention (P0)**: Added allowlist validation for ORDER BY columns in SQLite and PostgreSQL storage backends to prevent SQL injection via sort parameters
- **TLS InsecureSkipVerify Protection (P0)**: All TLS configurations now block `InsecureSkipVerify: true` unless `KSCORE_ALLOW_INSECURE_TLS=1` environment variable is explicitly set (development/testing only). Affected components: NATS (connection strategies, WebSocket, gateway), module registry clients, syslog transport
- **Rate Limiting for Authentication (P2)**: Added rate limiting for failed authentication attempts to prevent brute-force attacks. Default: 5 failures triggers 15-minute lockout. Configurable via `RateLimitConfig`. Returns gRPC `ResourceExhausted` status when rate limited
- **ExecutionModePermissive Deprecation (P1)**: Added deprecation warning (logged once via sync.Once) when using `ExecutionModePermissive` in command policy. Permissive mode provides minimal security and will be removed in a future release

### Recent Fixes

- Agent state propagation in HA cluster mode
- HA cluster E2E test failures
- NATS health check endpoint (changed from /healthz to /varz)

### Recent Changes

- Standardized domain to keystonecore.io
- Converted ASCII diagrams to Mermaid across documentation
