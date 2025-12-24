# Epic 7: Observability & Monitoring

## Overview

Implement comprehensive observability features including metrics, logging, tracing, and dashboards to provide complete visibility into TitanAnvil operations and infrastructure state.

**Goal**: Make TitanAnvil fully observable with production-grade monitoring, alerting, and troubleshooting capabilities that integrate seamlessly with existing observability stacks.

## Success Criteria

- [ ] Prometheus metrics for all operations
- [ ] Structured logging with multiple output formats
- [ ] Distributed tracing with OpenTelemetry
- [ ] Pre-built Grafana dashboards
- [ ] Health check endpoints
- [ ] Performance profiling capabilities
- [ ] Integration with popular observability platforms
- [ ] Real-time infrastructure state visualization
- [ ] Query API for metrics and logs
- [ ] Alert rule templates for common scenarios

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│              TitanAnvil Components                       │
│  Control Plane │ Agents │ State Engine │ Event System   │
└────────┬─────────────────────────────────────────────────┘
         │
         │ (instrumentation)
         │
    ┌────┴─────┬──────────┬──────────┐
    │          │          │          │
    ▼          ▼          ▼          ▼
┌─────────┐ ┌────────┐ ┌────────┐ ┌─────────┐
│ Metrics │ │  Logs  │ │ Traces │ │ Events  │
│(Prom)   │ │(JSON)  │ │(OTLP)  │ │(NATS)   │
└────┬────┘ └───┬────┘ └───┬────┘ └────┬────┘
     │          │          │          │
     └──────────┴──────────┴──────────┘
                    │
                    ▼
         ┌──────────────────────┐
         │  Observability Stack │
         │  Prometheus │ Loki   │
         │  Jaeger │ Grafana    │
         └──────────────────────┘
```

## User Stories

### US7.1: Prometheus Metrics
**As an** SRE
**I want to** collect Prometheus metrics from TitanAnvil
**So that** I can monitor system health and performance

**Acceptance Criteria**:
- Expose `/metrics` endpoint on control plane and agents
- RED metrics (Rate, Errors, Duration) for all operations
- USE metrics (Utilization, Saturation, Errors) for resources
- Custom metrics for business logic
- Metrics documented and labeled consistently
- Support for metric filtering and aggregation

**Key Metrics**:
```
# Control Plane Metrics
titan_api_requests_total{method, endpoint, status}
titan_api_request_duration_seconds{method, endpoint}
titan_agents_connected{datacenter, role}
titan_agents_disconnected_total
titan_command_executions_total{status}
titan_command_execution_duration_seconds
titan_state_applications_total{status}
titan_state_application_duration_seconds
titan_policy_evaluations_total{policy, result}
titan_events_published_total{type}
titan_events_processed_total{type}

# Agent Metrics
titan_agent_heartbeat_seconds
titan_agent_cpu_usage_percent
titan_agent_memory_usage_bytes
titan_agent_disk_usage_bytes
titan_agent_commands_executed_total{status}
titan_agent_states_applied_total{status}

# State Management Metrics
titan_state_resources_total{type, status}
titan_state_drift_detected_total{resource}
titan_state_changes_applied_total{module}

# GitOps Metrics
titan_gitops_webhooks_received_total{source}
titan_gitops_deployments_verified_total{status}
titan_gitops_rollbacks_triggered_total

# Policy Metrics
titan_policy_violations_total{policy, severity}
titan_policy_remediations_total{policy, status}
titan_compliance_score{framework}
```

**Example Queries**:
```promql
# Command execution rate
rate(titan_command_executions_total[5m])

# Error rate
sum(rate(titan_command_executions_total{status="error"}[5m])) /
sum(rate(titan_command_executions_total[5m]))

# Command latency p95
histogram_quantile(0.95,
  rate(titan_command_execution_duration_seconds_bucket[5m])
)

# Agent connectivity
sum(titan_agents_connected) by (datacenter)
```

### US7.2: Structured Logging
**As a** platform engineer
**I want to** have structured, searchable logs
**So that** I can troubleshoot issues effectively

**Acceptance Criteria**:
- JSON log format by default
- Support for multiple formats (JSON, logfmt, text)
- Configurable log levels (debug, info, warn, error)
- Correlation IDs for request tracing
- Context-rich logs with metadata
- Log sampling for high-volume operations
- Integration with log aggregation systems

**Log Structure**:
```json
{
  "timestamp": "2024-01-15T10:30:45.123Z",
  "level": "info",
  "logger": "state-manager",
  "message": "State application completed",
  "correlation_id": "req-abc123",
  "agent_id": "agent-456",
  "state_id": "webserver",
  "duration_ms": 1250,
  "resources_changed": 3,
  "resources_total": 10,
  "target": {
    "datacenter": "us-east-1",
    "role": "web",
    "environment": "production"
  },
  "changes": [
    {"type": "file", "path": "/etc/nginx/nginx.conf"},
    {"type": "service", "name": "nginx", "action": "restarted"}
  ]
}
```

**Log Levels by Component**:
```yaml
# logging.yaml
logging:
  level: info  # Global level

  # Per-component overrides
  components:
    state-manager: debug
    event-bus: warn
    policy-engine: info

  # Output configuration
  outputs:
    - type: stdout
      format: json
    - type: file
      path: /var/log/titan/titan.log
      format: json
      rotation:
        max_size: 100MB
        max_age: 7d
        max_backups: 10
    - type: loki
      url: http://loki:3100
      labels:
        app: titananvil
        environment: production
```

### US7.3: Distributed Tracing
**As an** SRE
**I want to** trace requests through the entire system
**So that** I can identify performance bottlenecks

**Acceptance Criteria**:
- OpenTelemetry instrumentation
- Trace context propagation
- Support for Jaeger, Zipkin, Tempo
- Span attributes with rich metadata
- Trace sampling configuration
- Integration with existing tracing infrastructure

**Trace Example**:
```
Trace ID: abc123
Duration: 2.5s

Span: titan.command.execute
  ├─ Span: titan.target.resolve (50ms)
  │   └─ Tags: target="role:web", matched_agents=10
  │
  ├─ Span: titan.nats.publish (10ms)
  │   └─ Tags: topic="agent.command", agents=10
  │
  ├─ Span: titan.agent.execute (2.3s) [parallel]
  │   ├─ Agent: web-01 (2.1s) ✓
  │   ├─ Agent: web-02 (2.3s) ✓
  │   └─ Agent: web-03 (1.9s) ✓
  │
  └─ Span: titan.results.aggregate (100ms)
      └─ Tags: success=3, failed=0
```

**Configuration**:
```yaml
# tracing.yaml
tracing:
  enabled: true
  exporter: otlp
  endpoint: http://tempo:4317
  sampling:
    rate: 0.1  # Sample 10% of traces
    always_sample:
      - "/api/v1/health"  # Always trace health checks
  attributes:
    service.name: titananvil
    deployment.environment: production
```

### US7.4: Grafana Dashboards
**As an** SRE
**I want to** pre-built Grafana dashboards
**So that** I can visualize TitanAnvil operations

**Acceptance Criteria**:
- Dashboard for control plane health
- Dashboard for agent fleet monitoring
- Dashboard for state management operations
- Dashboard for policy compliance
- Dashboard for GitOps operations
- Dashboards exported as JSON
- Variables for filtering by environment/datacenter

**Dashboards**:

1. **TitanAnvil Overview**
   - Total agents (connected/disconnected)
   - Commands per second
   - State applications per hour
   - Policy violations
   - Recent events timeline

2. **Control Plane Health**
   - API request rate and latency
   - NATS message throughput
   - State backend latency
   - Error rates by component
   - Resource utilization (CPU, memory)

3. **Agent Fleet**
   - Agent distribution by datacenter/role
   - Agent health status
   - Command execution success rate per agent
   - Agent resource utilization
   - Agent version distribution

4. **State Management**
   - State applications over time
   - State changes by module
   - Drift detection events
   - State application duration
   - Failed state applications

5. **Policy Compliance**
   - Compliance score by framework
   - Violations by severity
   - Remediation success rate
   - Top violated policies
   - Compliance trends over time

6. **GitOps Operations**
   - Deployments verified per hour
   - Verification success rate
   - Rollback frequency
   - Deployment verification duration
   - Failed verifications by application

### US7.5: Health Checks
**As a** platform engineer
**I want to** health check endpoints
**So that** I can monitor system availability

**Acceptance Criteria**:
- Liveness endpoint (`/health/live`)
- Readiness endpoint (`/health/ready`)
- Detailed health status (`/health/status`)
- Dependency health checks
- Configurable health check criteria
- Kubernetes-compatible probes

**Health Endpoints**:
```bash
# Liveness - is the process running?
GET /health/live
200 OK
{"status": "ok"}

# Readiness - ready to accept traffic?
GET /health/ready
200 OK
{
  "status": "ready",
  "checks": {
    "nats": "ok",
    "state_backend": "ok",
    "agents": "ok"
  }
}

# Detailed status
GET /health/status
200 OK
{
  "status": "healthy",
  "version": "1.0.0",
  "uptime": "72h15m30s",
  "components": {
    "api_server": {
      "status": "healthy",
      "requests_per_sec": 150
    },
    "nats": {
      "status": "healthy",
      "connected": true,
      "servers": ["nats://nats-1:4222", "nats://nats-2:4222"]
    },
    "state_backend": {
      "status": "healthy",
      "type": "postgresql",
      "latency_ms": 5
    },
    "agents": {
      "status": "healthy",
      "connected": 1250,
      "disconnected": 5
    }
  }
}
```

### US7.6: Performance Profiling
**As a** developer
**I want to** profile TitanAnvil performance
**So that** I can identify and fix bottlenecks

**Acceptance Criteria**:
- CPU profiling via pprof
- Memory profiling
- Goroutine profiling
- Mutex contention profiling
- Trace profiling
- On-demand profiling via API
- Continuous profiling integration (Pyroscope)

**Profiling Endpoints**:
```bash
# CPU profile (30 seconds)
curl http://titan:6060/debug/pprof/profile?seconds=30 > cpu.prof
go tool pprof cpu.prof

# Heap profile
curl http://titan:6060/debug/pprof/heap > heap.prof
go tool pprof heap.prof

# Goroutine profile
curl http://titan:6060/debug/pprof/goroutine > goroutine.prof

# Start CPU profiling via API
titanctl debug profile --type cpu --duration 30s --output cpu.prof
```

### US7.7: Infrastructure State Visualization
**As a** platform engineer
**I want to** visualize infrastructure state in real-time
**So that** I can understand system topology

**Acceptance Criteria**:
- Web UI showing agent topology
- Agent grouping by datacenter/role
- Real-time agent status updates
- Visual indication of drift/violations
- Clickable agents to view details
- Export topology as graph

**Topology View**:
```
UI Dashboard: Infrastructure Topology

Datacenter: us-east-1 (500 agents)
├─ Role: web (200 agents)
│  ├─ ✓ web-01 (healthy)
│  ├─ ✓ web-02 (healthy)
│  ├─ ⚠ web-03 (drift detected)
│  └─ ...
├─ Role: api (150 agents)
│  ├─ ✓ api-01 (healthy)
│  ├─ ❌ api-02 (offline)
│  └─ ...
└─ Role: db (150 agents)

Datacenter: us-west-2 (300 agents)
├─ Role: web (120 agents)
└─ ...

Legend:
✓ Healthy
⚠ Drift/Warning
❌ Offline/Error
```

## Technical Tasks

### Phase 1: Metrics (Week 1-2)

**T1.1: Prometheus Instrumentation**
- Add Prometheus client library
- Instrument all operations with metrics
- Create metric collectors
- Expose `/metrics` endpoint
- Add metric documentation

**T1.2: Custom Metrics**
- Business metrics (agents, commands, states)
- Performance metrics (latency, throughput)
- Error metrics by component
- Resource utilization metrics
- Cardinality management

**T1.3: Metric Aggregation**
- Agent metric collection
- Centralized metric aggregation
- Remote write to Prometheus
- Metric retention policies

### Phase 2: Logging (Week 3)

**T2.1: Structured Logging**
- Implement structured logger
- JSON/logfmt/text formatters
- Log level management
- Context propagation
- Sampling for high-volume logs

**T2.2: Log Outputs**
- Stdout/stderr output
- File output with rotation
- Syslog output
- Loki integration
- Elasticsearch integration

**T2.3: Correlation**
- Generate correlation IDs
- Propagate IDs across components
- Include IDs in all logs
- Trace ID integration

### Phase 3: Tracing (Week 4)

**T3.1: OpenTelemetry Integration**
- Add OTEL SDK
- Instrument critical paths
- Create spans for operations
- Add span attributes
- Context propagation

**T3.2: Trace Exporters**
- OTLP exporter (Tempo, Jaeger)
- Zipkin exporter
- Trace sampling configuration
- Batch span processing

**T3.3: Distributed Tracing**
- Trace across control plane and agents
- NATS message tracing
- Cross-service correlation
- Performance overhead optimization

### Phase 4: Dashboards (Week 5-6)

**T4.1: Grafana Dashboard Development**
- Create dashboard templates
- Define panels and visualizations
- Add variables for filtering
- Export dashboards as JSON
- Documentation for dashboards

**T4.2: Alert Rules**
- Prometheus alert rules
- Alert rule templates
- Integration with Alertmanager
- Alert documentation

**T4.3: Dashboard Automation**
- Auto-import dashboards on deployment
- Dashboard versioning
- Custom dashboard support

### Phase 5: Health & Status (Week 7)

**T5.1: Health Check Endpoints**
- Implement liveness probe
- Implement readiness probe
- Detailed status endpoint
- Dependency health checks

**T5.2: Status API**
- Component status reporting
- Version information
- Uptime tracking
- System diagnostics

**T5.3: Self-Healing**
- Automatic component restart on failure
- Circuit breakers
- Graceful degradation
- Recovery metrics

### Phase 6: Advanced Features (Week 8)

**T6.1: Performance Profiling**
- Enable pprof endpoints
- Continuous profiling support
- Profile storage and analysis
- Profile visualization

**T6.2: Infrastructure Visualization**
- Web UI for topology
- Real-time agent status
- Interactive graph visualization
- Export capabilities

**T6.3: Query API**
- Metrics query API
- Logs query API
- Trace query API
- Unified query interface

## Dependencies

- **All Epics**: Instrumentation touches all components
- **Go Libraries**:
  - `github.com/prometheus/client_golang` - Prometheus client
  - `go.opentelemetry.io/otel` - OpenTelemetry
  - `go.uber.org/zap` - Structured logging
  - `github.com/grafana/grafana-api-golang-client` - Grafana API
  - `net/http/pprof` - Performance profiling

## Risks & Mitigations

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| High cardinality metrics | High | Medium | Label validation, cardinality limits |
| Logging volume overwhelming storage | Medium | High | Sampling, retention policies, compression |
| Tracing overhead impacts performance | Medium | Medium | Sampling, async processing, optimization |
| Dashboard maintenance burden | Low | High | Automation, versioning, documentation |

## Metrics & Monitoring

### Key Metrics (Meta)
- Metric collection latency
- Log ingestion rate
- Trace sampling rate
- Dashboard load time
- Health check response time

### Alerts
- Metrics scrape failures
- Log ingestion errors
- Trace export failures
- Dashboard errors
- Health check failures

## Testing Strategy

### Unit Tests
- Metric registration and collection
- Log formatting
- Span creation and attributes
- Health check logic

### Integration Tests
- End-to-end metric collection
- Log forwarding to aggregators
- Trace propagation across services
- Dashboard functionality

### Load Tests
- High-volume metric collection
- High-volume log generation
- Trace sampling under load
- Health check under load

## Documentation Requirements

- [ ] Metrics reference guide
- [ ] Logging configuration guide
- [ ] Tracing setup guide
- [ ] Dashboard user guide
- [ ] Alert rule reference
- [ ] Troubleshooting with observability
- [ ] Performance profiling guide
- [ ] Integration guides (Prometheus, Loki, Grafana, Jaeger)

## Definition of Done

- [ ] All user stories implemented
- [ ] Metrics exposed and documented
- [ ] Structured logging operational
- [ ] Distributed tracing working
- [ ] Grafana dashboards available
- [ ] Health checks implemented
- [ ] Profiling enabled
- [ ] Documentation complete
- [ ] Integration tested with observability stack
- [ ] Production-ready
