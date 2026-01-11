---
title: "Architecture"
linkTitle: "Architecture"
weight: 4
description: >
  Understanding Keystone Core's architecture and design
---

## High-Level Architecture

Keystone Core follows a **control plane + agent** architecture, similar to Kubernetes but designed for infrastructure operations rather than container orchestration.

```mermaid
flowchart TB
    subgraph External["External Systems"]
        ArgoCD["ArgoCD/Flux"]
        Git["GitHub/GitLab"]
        OPA["OPA Bundles"]
        Prom["Prometheus/Grafana"]
    end

    subgraph CP["Keystone Core Control Plane"]
        subgraph API["API Layer"]
            gRPC["gRPC Server"]
            REST["REST (HTTP)"]
            Webhooks["Webhook Receivers"]
        end

        subgraph Services["Core Services"]
            ConnMgr["Connection Manager"]
            CmdDisp["Command Dispatcher"]
            StateMgr["State Manager"]
            EventEng["Event Engine"]
            PolicyEnf["Policy Enforcement"]
            GitOpsVer["GitOps Verification"]
        end

        NATS[("NATS Message Bus\n(Embedded or External)")]
        Storage[("State Storage\nSQLite / PostgreSQL")]
    end

    subgraph Agents["Agent Fleet"]
        K8s["Kubernetes Pods"]
        VMs["VMs\n(Linux/Windows/macOS)"]
        Cloud["Cloud\n(AWS/GCP/Azure)"]
        Bare["Bare Metal\n(BMC/IPMI)"]
        Edge["Edge Devices\n(Offline)"]
        Containers["Containers\n(Docker/containerd)"]
    end

    ArgoCD -->|webhooks| Webhooks
    Git -->|webhooks| Webhooks
    OPA -->|policies| PolicyEnf
    Prom -->|scrape| CP

    API --> Services
    Services --> NATS
    NATS --> Storage
    NATS <-->|bi-directional| Agents
```

## Core Components

### 1. Control Plane

The control plane orchestrates all operations. It consists of:

#### API Server
- **gRPC** for high-performance agent communication
- **REST** for user/tool interaction
- **Webhooks** for GitOps tool integration

**Responsibilities**:
- Authenticate and authorize requests
- Route commands to agents
- Serve metrics and health checks
- Handle webhook events

#### Connection Manager
Tracks all connected agents and their metadata.

**Key Functions**:
- Agent registration
- Heartbeat monitoring
- Connection state tracking
- Metadata storage

**Data Tracked**:
- Agent ID, datacenter, environment, role
- OS, architecture, kernel version
- Last heartbeat timestamp
- Custom tags

#### Command Dispatcher
Routes command execution requests to targeted agents.

**Features**:
- Expression-based targeting (`role:web and datacenter:us-east-1`)
- Batch execution with concurrency control
- Job tracking and result collection
- Timeout and retry handling

#### State Manager
Executes declarative state configurations.

**Capabilities**:
- Parse and validate state files (YAML)
- Resolve dependencies (requisites)
- Execute state modules (file, package, service, user, group, cmd)
- Detect configuration drift
- Generate state change events

#### Event Engine
Processes and routes infrastructure events.

**Components**:
- **Publisher**: Publishes events to NATS JetStream
- **Subscriber**: Receives events from JetStream
- **Router**: Routes events to reactors based on filters
- **Reactor Engine**: Executes automated responses
- **Storage**: Persists events to database (SQLite/PostgreSQL)

**Event Types**: 15 types across agent, job, state, system, and user categories

#### Policy Enforcement
Enforces policies using OPA (Rego) or CEL.

**Enforcement Points**:
- Pre-execution (before commands/state runs)
- Post-execution (after operations complete)
- On change (when state changes)
- On drift (when drift detected)
- On event (event-triggered)

**Actions**:
- Block (prevent operation)
- Warn (allow but log)
- Audit (log only)
- Remediate (auto-fix)

#### GitOps Integration
Integrates with GitOps tools for deployment lifecycle.

**Webhook Receivers**:
- ArgoCD (application sync/health events)
- Flux (Kustomization/HelmRelease events)
- GitHub (deployment/workflow events)
- GitLab (deployment/pipeline events)

**Verification Framework**:
- HTTP health checks
- Kubernetes resource checks
- Command execution checks
- Custom script checks

**Rollback Automation**:
- ArgoCD rollback executor
- Flux rollback executor
- Git rollback executor
- Approval workflows

### 2. Message Bus (NATS)

NATS provides the communication backbone.

#### Deployment Modes

**Embedded Mode** (development, small deployments):
- NATS runs in-process with control plane
- Zero external dependencies
- Suitable for <100 nodes
- Data stored in-memory (events in JetStream)

**External Cluster Mode** (production):
- Dedicated NATS cluster (3+ nodes)
- High availability
- Suitable for 100+ nodes
- Persistent JetStream storage

**Hybrid Mode** (advanced):
- Control plane connects to external NATS cluster
- Agents run embedded NATS as leaf nodes
- Best of both worlds

#### JetStream

Used for event persistence:
- Durable event storage
- Event replay
- Stream-based consumption
- At-least-once delivery

**Not used for state storage** (SQLite/PostgreSQL preferred for query patterns).

### 3. State Storage

Persistent storage for operational state.

#### SQLite (Embedded)
**Best for**:
- Development and testing
- Small deployments (<100 nodes)
- Home labs and edge locations
- Single-node setups

**Advantages**:
- Zero dependencies
- Simple deployment
- Excellent for getting started

**Limitations**:
- Single-node only
- Limited concurrency
- Not suitable for HA

#### PostgreSQL (Production)
**Best for**:
- Production deployments
- Large scale (100+ nodes)
- High availability requirements
- Multiple control plane nodes (clustering)

**Advantages**:
- ACID transactions
- High concurrency
- HA with replication
- Advanced indexing and querying

**Migration Path**: Automated tooling to migrate from SQLite → PostgreSQL

### 4. Agents

Lightweight Go binaries running on managed nodes.

#### Agent Lifecycle

```mermaid
flowchart TD
    Start([Start]) --> Connect
    Connect[Connect to NATS] --> Register
    Register[Register with Control Plane] --> Heartbeat
    Heartbeat[Send Heartbeat] --> Listen
    Listen[Subscribe to Commands] --> Execute
    Execute[Execute Commands / Apply State] --> Report
    Report[Report Results] --> Heartbeat

    Connect -.- note1[Connect to control plane via NATS]
    Register -.- note2[Send metadata to control plane]
    Heartbeat -.- note3[Send periodic heartbeats - 30s]
```

#### Agent Capabilities
- Command execution (shell, PowerShell, cmd)
- State module execution
- Metadata collection (OS, hardware, cloud provider)
- Event emission
- Local caching (for offline/edge mode)

#### Cross-Platform Support
- **Linux**: All major distributions
- **Windows**: PowerShell and cmd support
- **macOS**: Full support
- **ARM**: ARM64 and ARMv7 support

## Data Flow

### 1. Command Execution Flow

```mermaid
sequenceDiagram
    participant User
    participant CLI as kscorectl
    participant API as API Server
    participant Dispatch as Command Dispatcher
    participant NATS
    participant Agent

    User->>CLI: Execute command
    CLI->>API: gRPC/REST request
    API->>Dispatch: Dispatch command
    Dispatch->>NATS: Publish to agent.{id}.command
    NATS->>Agent: Deliver command
    Agent->>Agent: Execute command
    Agent->>NATS: Publish result
    NATS->>Dispatch: Deliver result
    Dispatch->>Dispatch: Store result
    Dispatch->>API: Return result
    API->>CLI: Response
    CLI->>User: Display output
```

### 2. State Application Flow

```mermaid
sequenceDiagram
    participant User
    participant CLI as kscorectl state apply
    participant API as API Server
    participant State as State Manager
    participant NATS
    participant Agent

    User->>CLI: Apply state file
    CLI->>API: Submit state
    API->>State: Process state
    State->>State: Parse & validate YAML
    State->>State: Build dependency graph (DAG)
    State->>State: Topological sort
    State->>NATS: Send to agents
    NATS->>Agent: Deliver modules
    Agent->>Agent: Execute modules (idempotent)
    Agent->>NATS: Return results
    NATS->>State: Collect results
    State->>State: Detect drift
    State->>NATS: Emit state.* events
    State->>API: Return summary
    API->>CLI: Response
    CLI->>User: Display summary
```

### 3. Event-Driven Automation Flow

```mermaid
flowchart TD
    Source[Agent/System] --> Event[Event]
    Event --> JS[NATS JetStream]
    JS --> Router[Event Router]
    Router --> Filter[Apply Filter Expression]
    Filter --> Match[Match Reactors by Priority]
    Match --> Execute[Execute Reactor Actions]

    Execute --> Cmd[Command]
    Execute --> Hook[Webhook]
    Execute --> State[State Apply]
    Execute --> Custom[Custom Function]

    Execute --> Store[Store Event]
    Store --> Metrics[Update Metrics]
```

### 4. GitOps Verification Flow

```mermaid
flowchart TD
    GitOps[ArgoCD/Flux] --> Webhook[Webhook]
    Webhook --> API[API Server]
    API --> Handler[GitOps Handler]
    Handler --> Parse[Parse Webhook Payload]
    Parse --> Status[Determine Deployment Status]
    Status --> Verify[Trigger Verification Steps]

    Verify --> HTTP[HTTP Check]
    Verify --> K8s[K8s Check]
    Verify --> Cmd[Command]
    Verify --> Script[Script]

    HTTP --> Aggregate[Aggregate Results]
    K8s --> Aggregate
    Cmd --> Aggregate
    Script --> Aggregate

    Aggregate --> Decision{Pass?}
    Decision -->|Yes| Success[Emit Success Event]
    Decision -->|No| Rollback[Trigger Rollback]
```

## Scaling Architecture

### Small Deployment (<100 nodes)

```mermaid
flowchart TD
    subgraph CP["Control Plane"]
        NATS["Embedded NATS"]
        SQLite["SQLite Storage"]
        Server["Single Instance"]
    end

    CP --> A1["Agent"]
    CP --> A2["Agent"]
    CP --> A3["Agent (x100)"]
```

### Medium Deployment (100-1000 nodes)

```mermaid
flowchart TD
    subgraph NATS["External NATS Cluster"]
        N1["Node 1"]
        N2["Node 2"]
        N3["Node 3"]
    end

    subgraph CP["Control Plane"]
        PG[("PostgreSQL")]
        Server["Single Instance"]
    end

    NATS --> CP
    CP --> A1["Agent"]
    CP --> A2["Agent"]
    CP --> A3["... Agent (x1000)"]
```

### Large Deployment (1000+ nodes)

```mermaid
flowchart TD
    subgraph NATS["External NATS Cluster (5+ nodes)"]
        N1["Node 1"]
        N2["Node 2"]
        N3["Node 3"]
        N4["Node 4"]
        N5["Node 5"]
    end

    subgraph HA["Control Plane (HA)"]
        CP1["Control Plane 1"]
        CP2["Control Plane 2"]
    end

    PG[("PostgreSQL\n(with HA)")]

    NATS --> HA
    HA --> PG
    HA --> A1["Agent (x10000)"]
    HA --> A2["Agent"]
    HA --> A3["Agent"]
```

## Security Model

### Transport Security
- **TLS**: All NATS connections can use TLS
- **Mutual TLS**: Agent authentication via client certs
- **API TLS**: HTTPS for REST API

### Authentication
- **NATS Credentials**: JWT-based authentication
- **API Tokens**: Bearer tokens for API access
- **Webhook Signatures**: HMAC verification

### Authorization
- **RBAC**: Role-based access control
- **Policy Enforcement**: OPA/CEL policies gate operations
- **Audit Logging**: All operations logged

### Plugin Security
- **Sandboxing**: Starlark and WASM sandboxed execution
- **Capability-based**: Explicit permission grants only
- **Cryptographic Verification**: Cosign signatures + SumDB
- **Trust Policies**: Control which modules can load

## Design Decisions

### Why NATS Instead of Kafka?
- Simpler operations (embedded mode)
- Lower latency (<1ms vs ~10ms)
- Built-in request-reply patterns
- Lightweight footprint

### Why SQLite AND PostgreSQL?
- **SQLite**: Zero dependencies for getting started
- **PostgreSQL**: Production scale and HA
- **Migration path**: Grow from prototype to production

### Why Separate Event Storage?
- JetStream is great for streaming events
- Relational DB better for complex queries
- Indexes and joins needed for analytics
- Retention policies easier in SQL

### Why Go?
- Cross-platform compilation
- Excellent concurrency (goroutines)
- Small binary size
- Strong ecosystem (gRPC, NATS clients)

## Next Steps

Now that you understand the architecture:

- **[Concepts](../../concepts/)** - Deep dive into each subsystem
- **[Operations](../../operations/)** - Production deployment and operations guides
- **[Reference](../../reference/)** - Complete API/CLI documentation

Or explore specific architectural topics:
- [NATS Integration](../../concepts/message-bus/)
- [State Storage Design](../../concepts/state-storage/)
- [Event System Architecture](../../concepts/events/)
- [Plugin System Design](../../concepts/plugin-system/)
