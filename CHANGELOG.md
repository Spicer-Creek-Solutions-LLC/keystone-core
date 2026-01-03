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
- 5 new Nginx state modules: nginx_upstream (load balancing), nginx_proxy (reverse proxy), nginx_ssl (SSL/TLS), nginx_location (location blocks), nginx_rate_limit (rate limiting)
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

## [0.16.0] - 2025-01-XX (In Progress)

### Added - Epic 16: Standard Library System Modules
- **Phase 1: Cross-Platform User/Group**
  - Windows user management using `net user` command
  - Windows group management using `net localgroup` command
  - PowerShell-based group membership queries
  - Full platform support: Linux (useradd/groupadd), macOS (dscl), Windows (net)
  - User properties: fullname, comment, home directory, active, password
  - Group properties: description, members

- **Phase 2: Network Configuration**
  - Network module (pkg/statemgmt/module_network.go)
    - States: configured, absent, dhcp
    - Parameters: interface, address, gateway, dns, mtu, metric, search_domains
    - Auto-detection of network managers
  - Linux network manager support
    - NetworkManager (nmcli)
    - netplan
    - ifupdown (/etc/network/interfaces)
    - systemd-networkd
  - macOS network support (networksetup command)
  - Windows network support (netsh command)
  - Route module (pkg/statemgmt/module_route.go)
    - States: present, absent
    - Parameters: destination, gateway, interface, metric, table
    - Linux (ip route), macOS (route), Windows (route -p)
    - CIDR notation, host routes, and default route support
  - Comprehensive test coverage (27 tests)
  - User documentation for network and route modules

- **Phase 3: Firewall Management**
  - Cross-platform firewall abstraction (pkg/statemgmt/module_firewall.go)
    - States: present, absent
    - Parameters: protocol, port, port_range, source, destination, interface, action, direction, zone, chain, table, profile, comment
    - Automatic backend detection (firewalld → nftables → iptables on Linux)
    - Platform support: Linux (iptables, nftables, firewalld), macOS (pf), Windows (netsh)
  - Linux iptables module (pkg/statemgmt/module_iptables.go)
    - States: present, absent, flush, policy
    - Full table/chain control (filter, nat, mangle, raw, security)
    - Match extensions (state, multiport)
    - NAT support (SNAT, DNAT, MASQUERADE)
  - Linux nftables module (pkg/statemgmt/module_nftables.go)
    - States: present, absent
    - Address families (ip, ip6, inet, arp, bridge, netdev)
    - Atomic table/chain/rule management
    - Base and regular chain types
  - Linux firewalld module (pkg/statemgmt/module_firewalld.go)
    - States: present, absent
    - Zone-based configuration
    - Service, port, source, interface, rich rule management
    - Masquerading and port forwarding
    - Permanent and runtime modes
  - Comprehensive test coverage (18 tests)
  - User documentation for firewall, iptables, nftables, and firewalld modules

- **Phase 4: Scheduled Tasks**
  - Cron module for Linux (pkg/statemgmt/module_cron.go)
    - States: present, absent
    - 5-field schedule (minute, hour, day, month, weekday)
    - Special schedules (@reboot, @daily, @weekly, @monthly, @yearly, @hourly)
    - Per-user crontab support
    - Disabled entry support (commented out but tracked)
  - Systemd timer module for Linux (pkg/statemgmt/module_systemd_timer.go)
    - States: present, absent
    - Creates both .timer and .service unit files
    - OnCalendar, OnBootSec, OnUnitActiveSec triggers
    - Persistent timers for missed runs
    - Environment variables and working directory
  - Launchd module for macOS (pkg/statemgmt/module_launchd.go)
    - States: present, absent
    - Calendar interval and start interval scheduling
    - WatchPaths and QueueDirectories for event triggers
    - KeepAlive for continuous services
    - plist file generation with launchctl integration
  - Scheduled task module for Windows (pkg/statemgmt/module_scheduled_task.go)
    - States: present, absent
    - Trigger types: once, daily, weekly, monthly, at_logon, at_startup, on_idle
    - Days of week/month scheduling
    - Repeat intervals and run levels (limited, highest)
    - schtasks.exe integration
  - At module for Unix (pkg/statemgmt/module_at.go)
    - States: present, absent
    - Flexible time formats (HH:MM, midnight, noon, now + N hours)
    - Queue priority support (a-z, A-Z)
    - Job tracking via marker comments
  - Comprehensive test coverage (51 tests)
  - User documentation for all 5 scheduled task modules

- **Phase 5: Mount and Storage**
  - Mount module for cross-platform mount management (pkg/statemgmt/module_mount.go)
    - States: mounted, unmounted, present, absent
    - Linux: /proc/mounts parsing, /etc/fstab management
    - macOS: diskutil and mount command integration
    - Windows: net use for network shares (limited)
    - Persistent mount configuration via fstab
  - Swap module for Linux (pkg/statemgmt/module_swap.go)
    - States: enabled, disabled, present, absent
    - Swap file creation via fallocate or dd
    - Size parsing (G/GB, M/MB, K/KB suffixes)
    - mkswap, swapon, swapoff integration
    - Persistent swap via /etc/fstab
  - LVM modules for Linux (pkg/statemgmt/module_lvm.go)
    - lvm_pv: Physical volume management (pvcreate, pvremove)
    - lvm_vg: Volume group management (vgcreate, vgextend, vgremove)
    - lvm_lv: Logical volume management (lvcreate, lvextend, lvremove)
    - Size formats: absolute (10G) and percentage (100%FREE)
    - Optional filesystem creation on new LV
  - Disk module for Linux (pkg/statemgmt/module_disk.go)
    - States: present, absent, formatted
    - parted command integration for partition operations
    - GPT and MSDOS partition table support
    - Partition flags: boot, lvm, raid, esp
  - Filesystem module for Linux (pkg/statemgmt/module_disk.go)
    - States: present, absent
    - mkfs support: ext4, ext3, xfs, btrfs, vfat, ntfs
    - wipefs for filesystem removal
    - blkid for filesystem detection
  - Comprehensive test coverage (51 tests)
  - User documentation for all 7 storage modules

- **Phase 6: SSH and Security**
  - SSH Authorized Keys module (pkg/statemgmt/module_ssh.go)
    - States: present, absent
    - Parameters: user, key, key_type, comment, options
    - Linux and macOS support via ~/.ssh/authorized_keys
    - SSH key options support (no-port-forwarding, command, etc.)
  - SSH Known Hosts module
    - States: present, absent
    - Parameters: host, key, key_type, user, path, hash_known_hosts
    - Host key scanning via ssh-keyscan
    - System-wide and per-user known_hosts support
  - SSHD Config module
    - States: present, absent
    - Parameters: name, value, path, backup
    - Case-insensitive directive matching
    - Backup before modification
  - SELinux module (pkg/statemgmt/module_security.go)
    - States: enforcing, permissive, disabled
    - getenforce/setenforce command integration
    - Persistent mode via /etc/selinux/config
  - SELinux Boolean module
    - States: on, off
    - getsebool/setsebool command integration
    - Persistent flag support
  - AppArmor module
    - States: enforce, complain, disabled
    - aa-enforce, aa-complain, aa-disable command integration
    - Profile status parsing from /sys/kernel/security/apparmor/profiles
  - AppArmor Profile module
    - States: present, absent
    - Parameters: name, source, content, mode
    - Profile installation to /etc/apparmor.d/
    - apparmor_parser for loading/unloading
  - Comprehensive test coverage (22 tests)
  - User documentation for all 7 SSH and security modules

- **Phase 7: System Configuration**
  - Timezone module (pkg/statemgmt/module_system.go)
    - States: present
    - Parameters: name (IANA timezone)
    - Linux: timedatectl/etc/timezone, macOS: systemsetup, Windows: tzutil
  - Locale module
    - States: present
    - Parameters: name (e.g., en_US.UTF-8)
    - Linux: localectl, macOS: partial, Windows: not supported
  - Hostname module
    - States: present
    - Parameters: name, fqdn
    - Linux: hostnamectl, macOS: scutil, Windows: wmic
  - Hosts module
    - States: present, absent
    - Parameters: ip, name/names
    - Cross-platform: /etc/hosts (Linux/macOS), drivers\etc\hosts (Windows)
    - Multi-hostname entry support with order-independent comparison
  - Sysctl module (Linux only)
    - States: present, absent
    - Parameters: name, value, persist
    - /proc/sys and sysctl command integration
    - Persistent config via /etc/sysctl.d/
  - Kernel Module module (Linux only)
    - States: loaded, unloaded, blacklisted
    - Parameters: name, params, persist
    - modprobe/rmmod commands, /proc/modules parsing
    - Persistent: /etc/modules-load.d/, Blacklist: /etc/modprobe.d/
  - Comprehensive test coverage (26 tests)
  - User documentation for all 6 system configuration modules

- **Phase 8: Container Management**
  - Docker Container module (pkg/statemgmt/module_docker.go)
    - States: running, stopped, absent
    - Parameters: name, image, ports, volumes, env, network, restart, command, force
    - Container lifecycle management (create, start, stop, remove)
  - Docker Image module
    - States: present, absent
    - Parameters: name, tag, force
    - docker pull/rmi integration
  - Docker Network module
    - States: present, absent
    - Parameters: name, driver, subnet, gateway, ip_range
    - docker network create/rm integration
  - Docker Volume module
    - States: present, absent
    - Parameters: name, driver, opts, force
    - Driver options support for NFS/other drivers
  - Podman Container module (same interface as Docker)
    - States: running, stopped, absent
  - Podman Image module
    - States: present, absent
  - Podman Network module
    - States: present, absent
  - Podman Volume module
    - States: present, absent
  - Container runtime detector (auto-detect Docker or Podman)
  - Comprehensive test coverage (30 tests)
  - User documentation for all 8 container modules

- **Phase 9: Database Primitives**
  - PostgreSQL Database module (pkg/statemgmt/module_database.go)
    - States: present, absent
    - Parameters: name, owner, encoding, lc_collate, lc_ctype, template, host, port, user
    - psql command integration with PGPASSWORD env var
  - PostgreSQL User module
    - States: present, absent
    - Parameters: name, password, superuser, createdb, createrole, login, host, port, user
    - Role attribute management (SUPERUSER, CREATEDB, CREATEROLE, LOGIN)
  - PostgreSQL Extension module
    - States: present, absent
    - Parameters: name, database, schema, version, host, port, user
    - CREATE EXTENSION / DROP EXTENSION integration
  - MySQL Database module
    - States: present, absent
    - Parameters: name, character_set, collation, host, port, socket, user, password
    - mysql command integration with TCP and Unix socket support
  - MySQL User module
    - States: present, absent
    - Parameters: name, password, host, privileges, host, port, socket, user
    - GRANT/REVOKE privilege management
  - Redis module
    - States: present, absent
    - Types: config (redis configuration), acl (ACL user management)
    - Parameters: key, value, user, password, rules, host, port
    - CONFIG SET/GET for configuration
    - ACL SETUSER/DELUSER for user management
  - Helper functions: escapePostgresString, quotePostgresIdentifier, escapeMySQLString, escapeMySQLIdentifier
  - Comprehensive test coverage (28 tests)
  - User documentation for all 6 database modules

- **Phase 10: Web Server Configuration**
  - Nginx Site module (pkg/statemgmt/module_web.go)
    - States: enabled, disabled, absent
    - Parameters: name, content, source, reload
    - sites-available/sites-enabled pattern
    - nginx -t validation before reload
  - Nginx Config module
    - States: present, absent
    - Parameters: name, content, source, dest, reload
    - Config snippet management (conf.d or custom paths)
  - Apache Site module
    - States: enabled, disabled, absent
    - Parameters: name, content, source, reload
    - a2ensite/a2dissite integration with fallback to symlinks
  - Apache Module module
    - States: enabled, disabled
    - Parameters: name, reload
    - a2enmod/a2dismod integration
  - Platform support: Linux (system paths), macOS (Homebrew paths)
  - Comprehensive test coverage (18 tests)
  - User documentation for all 4 web server modules

- **Phase 11: Version Control**
  - Git module (pkg/statemgmt/module_git.go)
    - States: present, absent, latest
    - Parameters: repo, dest, version, force, depth, recursive, ssh_key
    - Clone repositories with shallow clone and submodule support
    - SSH key authentication for private repos
    - Behind count detection for update checks
  - Git Config module
    - States: present, absent
    - Parameters: name, value, scope, file
    - Scopes: global, system, local, worktree
    - Custom config file support
  - Platform support: Linux, macOS, Windows (requires git in PATH)
  - Comprehensive test coverage (18 tests)
  - User documentation for git and git_config modules

- **Phase 12: Certificates**
  - X509 module (pkg/statemgmt/module_x509.go)
    - States: present, absent
    - Parameters: path, key_path, common_name, organization, country, validity_days, key_type, key_size, self_signed, is_ca, san_names, san_ips
    - Key types: RSA (2048, 4096), ECDSA (P-256, P-384, P-521), Ed25519
    - Self-signed certificate generation with SAN support
  - CA module
    - States: present, absent
    - Parameters: path, key_path, common_name, organization, country, validity_days, key_type, key_size, max_path_len
    - CA certificate and key generation
    - SignCertificate method for CSR signing
  - ACME module
    - States: present, absent, renewed
    - Parameters: path, key_path, domain, email, challenge, staging, renew_days, webroot, dns_provider
    - Challenge types: http-01, dns-01
    - Renewal threshold monitoring
    - Framework for external ACME tool integration
  - Platform support: Linux, macOS, Windows
  - Comprehensive test coverage (27 tests)
  - User documentation for x509, ca, acme modules

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

[Unreleased]: https://github.com/keystone-core/keystone-core/compare/v0.16.0...HEAD
[0.16.0]: https://github.com/keystone-core/keystone-core/compare/v0.15.0...v0.16.0
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
