# TitanAnvil Design Document

## Executive Summary

TitanAnvil is a cloud-native runtime infrastructure control plane that provides real-time execution, continuous compliance, and operational automation across hybrid environments. It complements GitOps and Infrastructure-as-Code tools by managing what happens between deployments.

**Tagline**: *GitOps deploys it. We keep it running.*

## Market Position & Why This is Needed

### The Gap in Modern Infrastructure Management

Modern infrastructure teams use a combination of tools:
- **GitOps** (ArgoCD, Flux): Declarative application deployment
- **Infrastructure-as-Code** (Terraform, Pulumi): Declarative infrastructure provisioning
- **Configuration Management** (Ansible): Ad-hoc configuration and deployment

However, a critical gap exists in the operational layer:

#### Problems TitanAnvil Solves

**1. The GitOps Deployment Gap**
- GitOps handles "what should be deployed" but not "what happens after"
- No real-time verification that deployed state matches desired state
- Manual drift detection and remediation
- Slow response to runtime issues (commit → PR → merge → deploy cycle)

**2. Real-time Operations at Scale**
- Ansible doesn't scale well beyond 1000+ nodes
- No good solution for "execute this NOW across 5000 servers"
- Incident response requires ad-hoc scripts or manual intervention
- Coordinated operations across hybrid infrastructure are complex

**3. Continuous Compliance & Drift**
- Runtime drift happens between deployments (manual changes, failed updates, security violations)
- Compliance checking only at deployment time, not continuously
- No automated remediation of policy violations
- Audit requirements need real-time enforcement, not eventual consistency

**4. Hybrid Infrastructure Complexity**
- Teams manage Kubernetes clusters, VMs, bare metal, edge devices
- Different tools for different environments
- No unified operational control plane
- Inconsistent policy enforcement across infrastructure types

### How TitanAnvil Augments Existing Tools

TitanAnvil is **not a replacement** for GitOps or IaC - it's the **operational layer** that makes them production-ready.

```
┌─────────────────────────────────────────────────────────┐
│                    GitOps / IaC Layer                   │
│              (Desired State Definition)                 │
│         ArgoCD, Flux, Terraform, Pulumi                 │
└─────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────┐
│                     TitanAnvil                          │
│              (Runtime Control Plane)                    │
│  • Real-time execution    • Continuous compliance       │
│  • Drift detection        • Operational automation      │
│  • Event-driven response  • Unified hybrid management   │
└─────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────┐
│                  Infrastructure                         │
│     Kubernetes • VMs • Bare Metal • Edge Devices        │
└─────────────────────────────────────────────────────────┘
```

### Comparison Matrix

| Capability | GitOps/IaC | Ansible | TitanAnvil |
|------------|------------|---------|------------|
| Deployment speed | Minutes | Minutes | Seconds |
| Scale (nodes) | Unlimited | ~500 | 10,000+ |
| Real-time execution | ❌ | ❌ | ✅ |
| Continuous compliance | ❌ | ❌ | ✅ |
| Event-driven | Limited | ❌ | ✅ |
| Hybrid infra | Separate tools | ✅ | ✅ |
| GitOps integration | N/A | Manual | Native |
| Declarative state | ✅ | ✅ | ✅ |
| Imperative actions | ❌ | ✅ | ✅ |

## High-Level Architecture

### Core Components

```
┌──────────────────────────────────────────────────────────────┐
│                      Control Plane                           │
│  ┌────────────┐  ┌─────────────┐  ┌──────────────────┐     │
│  │  API       │  │  State      │  │  Event/Reactor   │     │
│  │  Server    │  │  Manager    │  │  Engine          │     │
│  └────────────┘  └─────────────┘  └──────────────────┘     │
│         │               │                    │               │
│         └───────────────┴────────────────────┘               │
│                         │                                    │
└─────────────────────────┼────────────────────────────────────┘
                          │
                   ┌──────▼──────┐
                   │    NATS     │
                   │  Message    │
                   │    Bus      │
                   └──────┬──────┘
                          │
        ┌─────────────────┼─────────────────┐
        │                 │                 │
   ┌────▼────┐      ┌────▼────┐      ┌────▼────┐
   │ Agent   │      │ Agent   │      │ Agent   │
   │ (K8s)   │      │ (VM)    │      │ (Edge)  │
   └─────────┘      └─────────┘      └─────────┘
```

### Key Architectural Decisions

**1. NATS as Message Bus**
- Proven scalability (millions of messages/sec)
- **Embedded mode** for initial setups, small deployments, and agents (zero external dependencies)
- External cluster mode for production scale and high availability
- Leaf nodes for edge/disconnected scenarios
- JetStream for persistence and event sourcing
- Seamless migration path from embedded to external cluster as deployments grow

**2. Written in Go**
- Single static binary (no runtime dependencies)
- Excellent performance and low resource usage
- Cross-platform support
- Strong concurrency primitives
- Large ecosystem for cloud-native integration

**3. Agent-Based Architecture**
- Lightweight agents on managed nodes
- Pull and push modes supported
- Secure by default (mutual TLS)
- Self-healing and auto-reconnection
- Local execution prevents network dependency

**4. API-First Design**
- Everything accessible via API
- CLI wraps API calls
- Easy integration with existing tools
- OpenAPI/Swagger documentation
- Event streaming via WebSockets

### NATS Deployment Modes

TitanAnvil supports flexible NATS deployment strategies to match operational requirements:

**Embedded Mode** (Recommended for getting started)
- NATS server runs in-process with control plane or agent
- Zero external dependencies - single binary deployment
- Ideal for:
  - Development and testing environments
  - Small deployments (<100 nodes)
  - Edge agents in disconnected scenarios
  - Quick proof-of-concept installations
- Automatic upgrade path to external NATS cluster
- Full JetStream support for local persistence

**External Cluster Mode** (Recommended for production)
- Dedicated NATS cluster infrastructure
- High availability with automatic failover
- Ideal for:
  - Production deployments (100+ nodes)
  - Multi-region infrastructures
  - Environments requiring guaranteed uptime
  - Shared NATS infrastructure across multiple TitanAnvil instances
- Supports NATS clustering and JetStream replication

**Hybrid Mode**
- Control plane uses external NATS cluster
- Agents use embedded NATS with leaf node connections
- Ideal for:
  - Edge computing with intermittent connectivity
  - Multi-cloud deployments
  - Geographically distributed infrastructure
- Agents maintain local queue during network partitions

### State Storage Deployment Modes

TitanAnvil provides flexible storage options following the same zero-dependencies philosophy:

**SQLite Mode** (Recommended for getting started)
- Embedded database - zero external dependencies
- Single-file storage with ACID guarantees
- Ideal for:
  - Development and testing environments
  - Small deployments (<100 nodes)
  - Home labs and proof-of-concept installations
  - Quick setup without infrastructure overhead
- Full-featured with proper indexing and query support
- Automated migration tooling to PostgreSQL when scaling up

**PostgreSQL Mode** (Recommended for production)
- External database for high availability
- Advanced features: replication, point-in-time recovery
- Ideal for:
  - Production deployments (100+ nodes)
  - High availability requirements
  - Multi-instance TitanAnvil deployments
  - Enterprise environments
- Seamless migration from SQLite with zero downtime

**Design Rationale: Why Not JetStream for State?**
- JetStream is perfect for events and messaging (temporal, ordered data)
- State requires: complex queries, secondary indexes, transactional semantics, relational joins
- SQLite/PostgreSQL provide mature query optimization and ACID guarantees
- This separation of concerns follows best practices: events in NATS, operational state in relational DB

## Feature Categories

TitanAnvil combines proven SaltStack-like capabilities with modern cloud-native features:

### Core SaltStack-Like Features

1. **Remote Execution** - Run commands across infrastructure instantly
2. **State Management** - Declarative configuration with idempotent execution
3. **Event System** - Pub/sub event bus for system events
4. **Targeting** - Flexible node selection (grains, roles, attributes)
5. **Pillar System** - Secure configuration data distribution

### Modern Cloud-Native Features

6. **GitOps Integration** - Native integration with ArgoCD, Flux, Git webhooks
7. **Policy Enforcement** - Continuous compliance with OPA/CEL
8. **Kubernetes Native** - Operator mode, CRDs, pod exec, service mesh integration
9. **Observability** - Prometheus metrics, OpenTelemetry, structured logging
10. **Multi-Environment** - Unified management of K8s, VMs, bare metal, edge

### Operational Excellence Features

11. **Workflow Orchestration** - Complex multi-step operations with dependencies
12. **Drift Detection** - Continuous monitoring and auto-remediation
13. **Rollback Capabilities** - Automated rollback on failures
14. **Break-Glass Operations** - Emergency access with full audit trail
15. **Secret Management** - Integration with Vault, K8s secrets, cloud KMS

### Extensibility & Security Features

16. **Plugin System** - Secure, sandboxed plugins (Starlark/WASM) with capability-based security
17. **Cryptographic Verification** - Cosign signatures and SumDB-style transparency for plugins
18. **Deterministic Execution** - Pure, side-effect-free plugin logic with auditable behavior

## Use Cases

### 1. Deployment Verification
```
ArgoCD deploys new version → TitanAnvil verifies health across fleet
→ Detects errors → Triggers automated rollback via GitOps
```

### 2. Incident Response
```
Alert: Memory leak detected → TitanAnvil gathers diagnostics from 500 pods
→ Restarts affected services → Coordinates traffic shift → All in <60s
```

### 3. Continuous Compliance
```
Security policy: No containers run as root → TitanAnvil continuously monitors
→ Kills violating pods → Alerts security team → Blocks re-deployment
```

### 4. Coordinated Maintenance
```
Maintenance window → TitanAnvil drains nodes by zone → Applies updates
→ Verifies health → Returns to service → Moves to next zone
```

### 5. Hybrid Infrastructure Management
```
Single command → Execute across K8s pods, VMs, bare metal, edge devices
→ Unified reporting → Consistent policy enforcement
```

## Technology Stack

### Core Technologies
- **Language**: Go 1.21+
- **Message Bus**: NATS 2.10+ with JetStream (embedded or external cluster)
- **Storage**: SQLite (embedded) or PostgreSQL for state
- **API**: gRPC + REST (gRPC-gateway)
- **Security**: mTLS, RBAC, audit logging

### Integration Technologies
- **Kubernetes**: client-go, controller-runtime
- **GitOps**: ArgoCD API, Flux controllers
- **Policy**: Open Policy Agent (OPA), CEL
- **Observability**: Prometheus, OpenTelemetry, Grafana
- **Secrets**: HashiCorp Vault, cloud KMS providers

## Success Metrics

### Adoption Metrics
- Time to first successful remote execution: <5 minutes
- Number of nodes managed per installation
- Multi-environment adoption rate (K8s + VMs)

### Performance Metrics
- Command execution latency: <100ms to 1000 nodes
- Event processing throughput: >100k events/sec
- State application time: <30s for 5000 nodes

### Operational Metrics
- Drift detection time: <60s from occurrence
- Incident response time reduction: >70%
- Compliance violation detection rate: 100%

## Project Phases

### Phase 1: Core Foundation (MVP)
- NATS integration and agent communication
- Remote execution engine
- Basic state management
- CLI and API server

### Phase 2: GitOps Integration
- Event receivers from ArgoCD/Flux
- Deployment verification workflows
- Rollback capabilities
- Webhook support

### Phase 3: Policy & Compliance
- OPA integration
- Continuous compliance monitoring
- Drift detection and remediation
- Audit logging

### Phase 4: Multi-Environment
- Kubernetes operator mode
- VM/bare metal support
- Edge computing scenarios
- Unified targeting

### Phase 5: Enterprise Features
- RBAC and multi-tenancy
- High availability and disaster recovery
- Advanced workflow orchestration
- Enterprise integrations

## Competitive Differentiation

**vs. Ansible**
- 10x faster execution at scale
- Real-time vs. sequential execution
- Event-driven automation
- Better GitOps integration

**vs. SaltStack**
- Modern Go implementation (single binary)
- Cloud-native first (K8s, service mesh)
- Active development and community
- Clear open-source licensing

**vs. Kubernetes Operators**
- Works across all infrastructure types
- Not limited to K8s resources
- Operational commands, not just state management
- Unified interface for hybrid environments

## Open Source Strategy

- **License**: Apache 2.0
- **Repository**: GitHub with public roadmap
- **Community**: Discord, monthly community calls
- **Documentation**: Comprehensive docs site with tutorials
- **Extensions**: Plugin system for community contributions

## Next Steps

See individual epic documents in `/epics` for detailed implementation plans:

1. Core Infrastructure (`epics/01-core-infrastructure.md`)
2. Remote Execution (`epics/02-remote-execution.md`)
3. State Management (`epics/03-state-management.md`)
4. Event System (`epics/04-event-system.md`)
5. GitOps Integration (`epics/05-gitops-integration.md`)
6. Policy Enforcement (`epics/06-policy-enforcement.md`)
7. Observability (`epics/07-observability.md`)
8. Multi-Environment Support (`epics/08-multi-environment.md`)
9. Plugin System & Extensibility (`epics/09-plugin-system.md`)
