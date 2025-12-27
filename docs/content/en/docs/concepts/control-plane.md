---
title: "Control Plane"
weight: 1
description: >
  The control plane orchestrates all Keystone Core operations, providing APIs, state management, and agent coordination
---

## Overview

The Keystone Core control plane is the central nervous system of your infrastructure operations. It provides:

- **API Server** - gRPC and REST endpoints for all operations
- **Connection Manager** - Tracks and coordinates agent fleet
- **Command Dispatcher** - Routes execution requests to targeted agents
- **State Manager** - Executes declarative state configurations
- **Event Engine** - Processes and routes infrastructure events
- **Policy Enforcement** - Evaluates and enforces compliance policies

Unlike traditional control planes that only manage containers, Keystone Core orchestrates operations across Kubernetes, VMs, bare metal, edge devices, and cloud resources.

## Architecture

### High-Level Design

```
┌────────────────────────────────────────────────────────┐
│                  Keystone Core Control Plane               │
│                                                         │
│  ┌──────────────────────────────────────────────────┐  │
│  │                 API Layer                         │  │
│  │  ┌────────────┐  ┌────────────┐  ┌────────────┐  │  │
│  │  │   gRPC     │  │    REST    │  │  Webhooks  │  │  │
│  │  │   Server   │  │   (HTTP)   │  │  Receivers │  │  │
│  │  └────────────┘  └────────────┘  └────────────┘  │  │
│  └──────────────────────┬───────────────────────────┘  │
│                         │                              │
│  ┌──────────────────────┴───────────────────────────┐  │
│  │              Core Services                        │  │
│  │                                                   │  │
│  │  ┌─────────────┐  ┌──────────────┐  ┌─────────┐  │  │
│  │  │ Connection  │  │   Command    │  │  State  │  │  │
│  │  │  Manager    │  │  Dispatcher  │  │ Manager │  │  │
│  │  └─────────────┘  └──────────────┘  └─────────┘  │  │
│  │                                                   │  │
│  │  ┌─────────────┐  ┌──────────────┐  ┌─────────┐  │  │
│  │  │   Event     │  │    Policy    │  │  GitOps │  │  │
│  │  │   Engine    │  │  Enforcement │  │ Handler │  │  │
│  │  └─────────────┘  └──────────────┘  └─────────┘  │  │
│  └───────────────────────────────────────────────────┘  │
│                         │                              │
│  ┌──────────────────────┴───────────────────────────┐  │
│  │             Message Bus (NATS)                    │  │
│  │      Embedded or External + JetStream            │  │
│  └────────────────────────────────────────────────────┘  │
│                         │                              │
│  ┌──────────────────────┴───────────────────────────┐  │
│  │      State Storage (SQLite or PostgreSQL)        │  │
│  └────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────┘
```

### Component Breakdown

#### 1. API Server

The API Server provides two interfaces:

**gRPC API** (Primary):
- High-performance binary protocol
- Used by agents for registration, heartbeats, commands
- Bidirectional streaming for real-time updates
- Protobuf schemas ensure type safety

**REST API** (Secondary):
- HTTP/JSON for user tools and webhooks
- Automatically generated from gRPC via gRPC-gateway
- OpenAPI/Swagger documentation
- CORS support for web UIs

**Webhook Receivers**:
- ArgoCD application sync/health events
- Flux Kustomization/HelmRelease events
- GitHub deployment/workflow events
- GitLab deployment/pipeline events

#### 2. Connection Manager

Tracks all connected agents and their metadata.

**Responsibilities**:
- Agent registration (receive metadata on first connect)
- Heartbeat monitoring (30-second interval by default)
- Connection state tracking (online, offline, degraded)
- Metadata storage (datacenter, environment, role, tags, OS, arch)

**Data Model**:
```go
type AgentMetadata struct {
    ID          string            // Unique agent ID
    Datacenter  string            // Physical location
    Environment string            // dev, staging, prod
    Role        string            // web, db, cache, etc.
    Tags        []string          // Custom tags
    OS          string            // linux, windows, darwin
    Arch        string            // amd64, arm64
    Kernel      string            // Kernel version
    LastSeen    time.Time         // Last heartbeat
    CustomData  map[string]string // Extensible metadata
}
```

**Connection Lifecycle**:
1. Agent connects to NATS
2. Agent sends registration message to control plane
3. Control plane stores metadata in database
4. Agent sends heartbeat every 30 seconds
5. If no heartbeat for 90 seconds, mark agent offline

#### 3. Command Dispatcher

Routes command execution requests to targeted agents.

**Targeting System**:
- **Glob patterns**: `web-*`, `db-prod-*`
- **Expression-based**: `role:web and datacenter:us-east-1`
- **Agent ID**: Direct targeting by ID
- **Compound**: `environment:prod and (role:web or role:api)`

**Execution Modes**:
- **Synchronous**: Wait for all agents to complete
- **Asynchronous**: Fire and forget, poll for results later
- **Batch**: Execute in batches with concurrency control

**Job Tracking**:
```go
type Job struct {
    ID          string
    Command     string
    Targets     []string    // Resolved agent IDs
    Status      JobStatus   // pending, running, completed, failed
    Results     map[string]*JobResult
    StartTime   time.Time
    EndTime     time.Time
    Timeout     time.Duration
}

type JobResult struct {
    AgentID    string
    ExitCode   int
    Stdout     string
    Stderr     string
    StartTime  time.Time
    Duration   time.Duration
    Error      string
}
```

**Features**:
- Timeout enforcement (per-job and per-agent)
- Retry logic with exponential backoff
- Partial failure handling
- Progress tracking and streaming output

#### 4. State Manager

Executes declarative state configurations.

**Workflow**:
1. Parse state file (YAML) and validate schema
2. Render templates with vars and facts
3. Build dependency graph (DAG)
4. Topologically sort modules
5. Send execution plan to agents
6. Agents execute modules idempotently
7. Collect results and detect drift
8. Emit state change events

**Supported Modules**:
- `file` - File/directory management
- `package` - Package installation/removal
- `service` - Service management (systemd, upstart, etc.)
- `user` - User account management
- `group` - Group management
- `cmd` - Command execution

**Drift Detection**:
- Compare desired state vs actual state
- Calculate drift severity (none, low, medium, high, critical)
- Emit `state.drift` events with details
- Generate drift reports

#### 5. Event Engine

Processes and routes infrastructure events.

**Event Flow**:
```
1. Event Source (agent, state, job, webhook)
   ↓
2. Event Publisher (publishes to NATS JetStream)
   ↓
3. Event Router (filters and routes to reactors)
   ↓
4. Event Storage (persists to SQLite/PostgreSQL)
   ↓
5. Reactor Engine (executes automated responses)
```

**Event Types** (15 total):
- Agent: connect, disconnect, heartbeat_failed, metadata_changed
- Job: start, complete, fail
- State: apply.start, apply.done, apply.fail, change, drift
- System: startup, shutdown
- User: custom events

**Event Schema**:
```go
type Event struct {
    ID            string
    Type          EventType
    Source        string          // Agent ID or system component
    Timestamp     time.Time
    Severity      Severity        // debug, info, warning, error, critical
    CorrelationID string          // For tracking related events
    Tags          []string
    Data          map[string]interface{}
}
```

**Reactor System**:
- Filter events with CEL expressions
- Priority-based reactor ordering
- Throttling and debouncing
- Actions: command, webhook, state apply, custom functions

#### 6. Policy Enforcement

Evaluates and enforces compliance policies.

**Policy Types**:
- **OPA (Rego)**: Powerful policy language from Open Policy Agent
- **CEL**: Common Expression Language for simple policies

**Enforcement Points**:
- Pre-execution (before commands/state runs)
- Post-execution (after operations complete)
- On change (when state changes)
- On drift (when drift detected)
- On event (event-triggered policies)

**Enforcement Actions**:
- **Block**: Prevent operation from executing
- **Warn**: Allow but log warning
- **Audit**: Log for compliance reporting
- **Remediate**: Automatically fix violation

**Policy Workflow**:
1. Operation triggers enforcement point
2. Load policies bound to resource type
3. Evaluate policies (OPA or CEL)
4. Collect violations
5. Apply enforcement action
6. Emit policy events (pass or violation)
7. Record audit log

## Deployment Modes

### Embedded Mode (Development/Small Deployments)

Best for:
- Development and testing
- Small deployments (<100 nodes)
- Home labs and edge locations
- Quick prototyping

Configuration:
```yaml
nats:
  mode: embedded
  listen: "0.0.0.0:4222"

storage:
  type: sqlite
  path: /var/lib/kscore/state.db

api:
  listen: "0.0.0.0:8080"
```

Characteristics:
- Single binary runs everything
- Zero external dependencies
- SQLite for state storage
- NATS runs in-process
- Perfect for getting started

### Production Mode (Large Deployments)

Best for:
- Production deployments
- Large scale (100+ nodes)
- High availability requirements
- Multi-region deployments

Configuration:
```yaml
nats:
  mode: external
  urls:
    - nats://nats1.example.com:4222
    - nats://nats2.example.com:4222
    - nats://nats3.example.com:4222
  credentials: /etc/kscore/nats.creds

storage:
  type: postgresql
  connection_string: "postgres://user:pass@postgres.example.com:5432/kscore?sslmode=require"

api:
  listen: "0.0.0.0:8080"
  tls:
    enabled: true
    cert_file: /etc/kscore/tls/server.crt
    key_file: /etc/kscore/tls/server.key
```

Characteristics:
- External NATS cluster (3+ nodes)
- PostgreSQL with replication
- TLS encryption everywhere
- Multiple control plane instances (HA)
- Horizontal scalability

## High Availability

For production deployments, run multiple control plane instances:

### Architecture

```
┌────────────┐     ┌────────────┐     ┌────────────┐
│  Control   │     │  Control   │     │  Control   │
│  Plane 1   │     │  Plane 2   │     │  Plane 3   │
└─────┬──────┘     └─────┬──────┘     └─────┬──────┘
      │                  │                  │
      └──────────────────┴──────────────────┘
                         │
         ┌───────────────┴───────────────┐
         │                               │
    ┌────┴────┐                    ┌─────┴──────┐
    │  NATS   │                    │ PostgreSQL │
    │ Cluster │                    │  (with HA) │
    └─────────┘                    └────────────┘
```

### Load Distribution

- **NATS**: Built-in load balancing (queue subscribers)
- **Database**: Single writer, multiple readers (PostgreSQL streaming replication)
- **API**: Load balancer (HAProxy, Nginx, or cloud LB)

### Failure Scenarios

**Control Plane Failure**:
- Agents automatically reconnect to surviving instances
- NATS queue ensures no message loss
- State operations are idempotent (safe to retry)

**NATS Cluster Failure**:
- Agents buffer messages locally
- Control plane retries until NATS recovers
- JetStream persists events

**Database Failure**:
- Read replica promoted to primary
- Control plane reconnects automatically
- Minimal downtime (<30 seconds typical)

## Performance Characteristics

### Throughput

- **Agent Registration**: 1,000 agents/second
- **Heartbeat Processing**: 10,000 heartbeats/second
- **Command Execution**: 100,000 commands/second (distributed)
- **State Applications**: 1,000 state runs/second
- **Event Processing**: 50,000 events/second

### Latency

- **API Response Time**: <10ms (p95)
- **Command Dispatch**: <50ms (p95)
- **Event Publication**: <5ms (p95)
- **State Execution**: Depends on modules (typically seconds)

### Resource Usage

**Small Deployment** (<100 agents):
- CPU: 0.5 cores
- Memory: 512MB
- Disk: 1GB

**Medium Deployment** (100-1,000 agents):
- CPU: 2 cores
- Memory: 2GB
- Disk: 10GB

**Large Deployment** (1,000+ agents):
- CPU: 4-8 cores
- Memory: 4-8GB
- Disk: 50GB+

## Security

### Authentication

- **NATS Credentials**: JWT-based authentication for agents
- **API Tokens**: Bearer tokens for user/tool access
- **Webhook Signatures**: HMAC verification for webhooks
- **Mutual TLS**: Client certificate verification

### Authorization

- **RBAC**: Role-based access control for API
- **Policy Enforcement**: OPA/CEL policies gate operations
- **Resource Scoping**: Users can only access allowed resources

### Encryption

- **In Transit**: TLS for all connections (NATS, API, database)
- **At Rest**: Database encryption (PostgreSQL native encryption)
- **Secrets**: Integration with external secret stores

### Audit Logging

All operations are logged with:
- **Who**: User/service account
- **What**: Operation performed
- **When**: Timestamp
- **Where**: Source IP/agent
- **Why**: Correlation ID linking related operations

## Monitoring

### Health Endpoints

- `GET /health/live`: Liveness probe (always 200 if running)
- `GET /health/ready`: Readiness probe (503 if dependencies unavailable)
- `GET /health/status`: Detailed component status

### Metrics (Prometheus)

Key metrics exposed at `/metrics`:

```
# Control Plane
kscore_control_plane_uptime_seconds
kscore_agents_connected
kscore_agents_registered_total

# API Server
kscore_api_requests_total{method, path, status}
kscore_api_request_duration_seconds{method, path}

# Command Dispatcher
kscore_commands_dispatched_total
kscore_commands_completed_total
kscore_commands_failed_total

# State Manager
kscore_state_applications_total
kscore_state_drift_detected_total

# Event Engine
kscore_events_published_total{type}
kscore_events_processed_total{type}

# Policy Enforcement
kscore_policy_evaluations_total
kscore_policy_violations_total{severity}
```

## Configuration Reference

Complete configuration options:

```yaml
# API Server
api:
  listen: "0.0.0.0:8080"
  tls:
    enabled: false
    cert_file: ""
    key_file: ""
  cors:
    enabled: false
    allowed_origins: ["*"]
  rate_limit:
    enabled: false
    requests_per_second: 100

# NATS Configuration
nats:
  mode: embedded              # embedded, external, leaf
  listen: "0.0.0.0:4222"
  urls: []                    # For external mode
  credentials: ""             # For authentication
  tls:
    enabled: false
    cert_file: ""
    key_file: ""
    ca_file: ""

# Storage Configuration
storage:
  type: sqlite                # sqlite, postgresql
  path: ""                    # For SQLite
  connection_string: ""       # For PostgreSQL
  max_connections: 25
  max_idle_connections: 5
  connection_max_lifetime: "1h"

# Connection Manager
connection_manager:
  heartbeat_interval: "30s"
  heartbeat_timeout: "90s"
  max_agents: 10000

# Command Dispatcher
command_dispatcher:
  default_timeout: "5m"
  max_concurrent_jobs: 1000
  retry_attempts: 3
  retry_backoff: "exponential"

# Event Engine
event_engine:
  buffer_size: 10000
  worker_count: 10
  retention_days: 30

# Policy Enforcement
policy:
  enabled: true
  default_action: "audit"     # block, warn, audit
  cache_ttl: "5m"

# Logging
logging:
  level: "info"               # debug, info, warn, error
  format: "json"              # json, logfmt, text
  output: "stdout"            # stdout, file

# Observability
observability:
  metrics:
    enabled: true
    path: "/metrics"
  tracing:
    enabled: false
    endpoint: ""
```

## Best Practices

### Sizing

1. **Start Small**: Begin with embedded mode, migrate to production mode as you grow
2. **Monitor Resources**: Track CPU, memory, disk I/O
3. **Plan for Growth**: Size database and NATS cluster appropriately
4. **Test Limits**: Load test with expected agent count + 50% headroom

### Reliability

1. **Deploy HA**: Multiple control plane instances in production
2. **Monitor Health**: Set up alerts for health endpoint failures
3. **Backup Database**: Regular backups of state database
4. **Use TLS**: Encrypt all connections in production
5. **Rate Limit**: Protect against abuse with rate limiting

### Operations

1. **Rolling Updates**: Update one control plane instance at a time
2. **Gradual Rollouts**: Test changes on dev/staging first
3. **Feature Flags**: Use policy enforcement to gate new features
4. **Audit Everything**: Enable comprehensive audit logging

## Troubleshooting

### Common Issues

**Problem**: Control plane won't start
- Check: Port 4222 (NATS) and 8080 (API) not in use
- Check: Database connection string correct
- Check: Config file syntax valid

**Problem**: Agents not connecting
- Check: NATS URL reachable from agents
- Check: Firewall allows port 4222
- Check: NATS credentials valid (if using auth)

**Problem**: High memory usage
- Check: Number of connected agents (may need to scale horizontally)
- Check: Event retention period (reduce if too long)
- Check: Database query performance

**Problem**: Slow API responses
- Check: Database indexes created
- Check: NATS cluster healthy
- Check: CPU utilization (may need more cores)

### Debug Logging

Enable debug logging for troubleshooting:

```yaml
logging:
  level: "debug"
```

Or at runtime via API:
```bash
curl -X POST http://localhost:8080/admin/log-level -d '{"level":"debug"}'
```

## Next Steps

- Learn about [Agents](agents/) that connect to the control plane
- Understand the [Message Bus](message-bus/) architecture
- Explore [State Storage](state-storage/) design decisions
- See [Remote Execution](remote-execution/) for command dispatching details
