---
title: "Release Notes"
weight: 2
description: >
  Release notes for Keystone Core versions
---

# Release Notes

This page contains release notes for Keystone Core. For detailed changelog entries, see the [CHANGELOG](https://github.com/keystone-core/keystone-core/blob/main/CHANGELOG.md).

## Version 0.1.0 (2026-01-28)

**Initial Release** - First public release of Keystone Core.

This release represents the culmination of 41 completed epics spanning core infrastructure, remote execution, state management, event-driven automation, GitOps integration, policy enforcement, observability, and comprehensive documentation.

### Highlights

- **Cloud-Native Runtime Control Plane**: Complete infrastructure management solution bridging GitOps deployments and runtime operations
- **NATS-Based Messaging**: Three deployment modes (embedded, external cluster, hybrid) with JetStream for event persistence
- **Declarative State Management**: Idempotent configuration with drift detection and remediation
- **Event-Driven Automation**: Reactor system with filtering, routing, enrichment, and dead-letter handling
- **Policy Enforcement**: OPA and CEL policy engines with continuous compliance monitoring
- **High Availability**: etcd-based clustering with leader election and automatic failover
- **Multi-Environment Support**: Unified management across Kubernetes, VMs, bare metal, and edge devices

### Core Features

#### Infrastructure

- Embedded and external NATS support with automatic mode detection
- SQLite (embedded) and PostgreSQL (production) storage backends
- etcd-based clustering for high availability
- SPIFFE/SPIRE identity management
- IPv6 and dual-stack networking support

#### Remote Execution

- Cross-platform command execution (Linux, Windows, macOS)
- Flexible targeting with compound filters
- Pipeline execution with stages and error handling
- Execution timeouts and cancellation

#### State Management

- Declarative state definitions with YAML/JSON
- 15+ built-in state modules (files, packages, services, users, etc.)
- Drift detection with configurable remediation
- Starlark and WASM module extensibility

#### Event System

- Event-driven reactor with pattern matching
- Dead-letter queue for failed events
- Event enrichment and transformation
- HTTP webhook receiver

#### GitOps Integration

- ArgoCD and Flux webhook handlers
- Deployment verification and health checks
- Automatic rollback on failure
- Promotion pipelines across environments

#### Policy & Compliance

- OPA (Rego) and CEL policy engines
- Policy bundles with versioning
- Compliance reporting and audit trails
- Real-time policy evaluation

#### Observability

- Prometheus metrics exposition
- OpenTelemetry tracing integration
- Structured logging with syslog support
- Telemetry gateway for aggregation

#### File Distribution

- NATS-based file transfer with chunking
- Multiple storage backends (local, S3, GCS, Azure Blob)
- Mirror groups with bandwidth management
- Content-addressable deduplication

#### Self-Management

- Single-binary bootstrap experience
- Automated backup and restore
- Rolling upgrade orchestration
- Health monitoring and recovery

#### Blueprints

- Reusable state collections
- 14 official blueprints (monitoring-stack, security-baseline, nats-cluster, etc.)
- Blueprint registry with signing and verification
- Parameter templating and customization

#### Secrets Management

- Multi-backend support (Vault, AWS, Azure, GCP)
- Automated secret rotation
- Transit encryption (encryption-as-a-service)
- KMS and HSM integration

#### DNS Management

- Declarative DNS record management
- Multi-provider support via libdns
- Supports A, AAAA, CNAME, TXT, MX, SRV, CAA, NS, ALIAS, PTR records

#### Advanced Networking

- WiFi configuration management
- 802.1X authentication support
- Link settings and promiscuous mode
- Network interface management

#### Runbook Automation

- YAML-based runbook definitions
- Conditional logic (if/switch/loop/parallel)
- Human-in-the-loop approvals
- ITSM integration (PagerDuty, Opsgenie, ServiceNow)

### Platform Support

#### Control Plane

| Platform | Status |
|----------|--------|
| Ubuntu 22.04 LTS | Fully Supported |
| Ubuntu 24.04 LTS | Supported |
| RHEL 8 | Fully Supported |
| RHEL 9 | Fully Supported |
| Debian 11 | Fully Supported |
| Debian 12 | Supported |

#### Agent

| Platform | Status |
|----------|--------|
| Linux (amd64, arm64) | Fully Supported |
| Windows Server 2019+ | Supported |
| macOS 12+ | Supported |

### Dependencies

| Component | Minimum Version | Recommended |
|-----------|-----------------|-------------|
| Go | 1.25 | 1.25+ |
| NATS Server | 2.9 | 2.10+ |
| PostgreSQL | 14 | 16 |
| etcd | 3.4 | 3.5 |
| Kubernetes | 1.26 | 1.29+ |

### Known Limitations

- Windows agent features are functional but less tested than Linux
- macOS support is primarily for development environments
- PostgreSQL 16 recommended for deployments > 500 agents
- NATS 2.11 recommended for new deployments

### Upgrade Notes

This is the initial release. For future upgrades:
- Follow the sequential upgrade path (0.1.x → 0.2.x → 0.3.x)
- Review compatibility matrix before upgrading
- Back up state database before major upgrades
- Test upgrades in non-production environments first

### Getting Started

```bash
# Install the CLI
curl -fsSL https://get.keystone-core.io | sh

# Bootstrap a control plane
kscorectl bootstrap init --mode embedded

# Join an agent
kscore-agent join --server https://control-plane:8443

# Apply your first state
kscorectl state apply -f my-state.yaml
```

See the [Quick Start Guide](/docs/getting-started/quickstart/) for detailed instructions.

### Documentation

- [Concepts](/docs/concepts/) - Architecture and design
- [Tutorials](/docs/tutorials/) - Step-by-step guides
- [Reference](/docs/reference/) - CLI and API reference
- [Operations](/docs/operations/) - Production deployment guides

### Acknowledgments

Thank you to all contributors who made this release possible.
