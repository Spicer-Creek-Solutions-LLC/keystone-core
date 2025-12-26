# Epic 7: Observability & Monitoring

## Overview

Implement comprehensive observability features including metrics, logging, tracing, and dashboards to provide complete visibility into TitanAnvil operations and infrastructure state.

**Goal**: Make TitanAnvil fully observable with production-grade monitoring, alerting, and troubleshooting capabilities that integrate seamlessly with existing observability stacks.

## Success Criteria

- [ ] Prometheus metrics for all operations
- [ ] Structured logging with multiple output formats
- [ ] Distributed tracing with OpenTelemetry
- [ ] Terminal-based real-time monitoring tool (TUI)
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

### US7.4: Real-Time TUI Monitor
**As an** operator
**I want to** a terminal-based dashboard for monitoring TitanAnvil
**So that** I can quickly check system status without web UI dependencies

**Acceptance Criteria**:
- Terminal-based interface (TUI) with multiple views
- Real-time updates from event bus and API polling
- Works over SSH connections
- Keyboard navigation and vim-style keybindings
- Filter and search capabilities in all views
- Export current view to file
- Configurable refresh rates
- Color themes (dark/light/custom)
- Zero dependencies (no web server required)

**Views and Features**:

1. **Dashboard Overview** (default view)
   ```
   ┌────────────────────────────────────────────────────────────────┐
   │ TitanAnvil Monitor                    [q] quit [?] help       │
   ├────────────────────────────────────────────────────────────────┤
   │ System                                                          │
   │   Uptime: 3d 12h 45m      API Req/s: 1,234    Events/s: 567   │
   │   Version: 1.0.0          Memory: 2.3GB       Goroutines: 145 │
   │                                                                 │
   │ Agents                                                          │
   │   Connected: 1,245 / 1,250    Disconnected: 5                 │
   │   Recent: ↑ web-05 (2s)  ↓ db-12 (15s)                        │
   │                                                                 │
   │ Jobs (last 5m)                                                 │
   │   Running: 12      Queued: 3       Completed: 456             │
   │   Failed: 2        Avg Duration: 2.3s                          │
   │                                                                 │
   │ State                                                          │
   │   Resources: 15,234        Drift: 12 (🔴 2  🟡 6  🟢 4)       │
   │   Last Check: 30s ago      Changes: 45 (last hour)            │
   │                                                                 │
   │ Policy                                                         │
   │   Violations: 18 (🔴 3  🟡 8  🟢 7)                            │
   │   Compliance: 94.2%        Last Eval: 15s ago                  │
   │                                                                 │
   │ Recent Events                                                  │
   │   10:45:23 ERROR  state.drift  Resource /etc/nginx.conf       │
   │   10:45:18 INFO   agent.connect  Agent web-05 connected       │
   │   10:45:12 WARN   policy.violation  Security policy SVC-001   │
   │   10:45:05 INFO   job.complete  Command finished (2.1s)       │
   └────────────────────────────────────────────────────────────────┘
   ```

2. **Agent List** (press '1')
   ```
   ┌─ Agents (1,250) ─────────────────────── [/] filter [s] sort ─┐
   │ ID          Hostname      OS      Status    Last HB  CPU  Mem │
   ├────────────────────────────────────────────────────────────────┤
   │ web-01      prod-web-01   Linux   ✓ Online  2s       45%  62% │
   │ web-02      prod-web-02   Linux   ✓ Online  1s       52%  58% │
   │ web-03      prod-web-03   Linux   ⚠ Drift   3s       38%  55% │
   │ api-01      prod-api-01   Linux   ✓ Online  2s       67%  71% │
   │ api-02      prod-api-02   Linux   ❌ Offline 45s      --   --  │
   │ db-01       prod-db-01    Linux   ✓ Online  1s       89%  82% │
   │ ...                                                             │
   │                                                                 │
   │ [↑/↓] navigate  [Enter] details  [f] filter  [Esc] back       │
   └────────────────────────────────────────────────────────────────┘
   ```

3. **Event Stream** (press '2')
   ```
   ┌─ Event Stream (live) ────────────────── [/] filter [p] pause ─┐
   │ Time     Level    Type              Source         Message     │
   ├────────────────────────────────────────────────────────────────┤
   │ 10:45:23 ERROR    state.drift       state-mgr      Resource... │
   │ 10:45:18 INFO     agent.connect     conn-mgr       Agent we... │
   │ 10:45:12 WARN     policy.violation  policy-eng     Security... │
   │ 10:45:05 INFO     job.complete      cmd-dispatch   Command ... │
   │ 10:44:58 INFO     state.apply.done  state-mgr      Applied ... │
   │ 10:44:45 DEBUG    agent.heartbeat   agent-web-05   Heartbe... │
   │ ...                                                             │
   │ ⬇ LIVE (auto-scroll)                                           │
   │                                                                 │
   │ [p] pause  [c] clear  [e] export  [Enter] expand  [Esc] back  │
   └────────────────────────────────────────────────────────────────┘
   ```

4. **State Drift** (press '3')
   ```
   ┌─ State Drift (12 resources) ──────────────── [r] refresh ─────┐
   │ Resource                  Agent      Severity  Last Check      │
   ├────────────────────────────────────────────────────────────────┤
   │ /etc/nginx/nginx.conf     web-03     🔴 High   30s             │
   │ service:nginx             web-03     🟡 Med    30s             │
   │ /var/www/app/config.php   api-05     🔴 High   45s             │
   │ user:appuser              api-05     🟢 Low    45s             │
   │ package:docker            db-02      🟡 Med    1m              │
   │ ...                                                             │
   │                                                                 │
   │ Summary: 2 critical, 6 medium, 4 low                           │
   │                                                                 │
   │ [↑/↓] navigate  [Enter] details  [a] apply  [Esc] back        │
   └────────────────────────────────────────────────────────────────┘
   ```

5. **Policy Violations** (press '4')
   ```
   ┌─ Policy Violations (18) ──────────────────── [g] group by ────┐
   │ Policy              Resource         Severity  Time            │
   ├────────────────────────────────────────────────────────────────┤
   │ SEC-SSH-001         /etc/ssh/config  🔴 Crit   2m              │
   │ SEC-SUDO-002        /etc/sudoers     🔴 Crit   5m              │
   │ CMP-PCI-045         service:mysql    🔴 Crit   8m              │
   │ SEC-FILE-010        /tmp/sensitive   🟡 Med    12m             │
   │ OPS-DISK-001        /var/log         🟡 Med    15m             │
   │ ...                                                             │
   │                                                                 │
   │ Compliance: 94.2% (1,245/1,320 checks passed)                 │
   │                                                                 │
   │ [↑/↓] navigate  [Enter] details  [r] remediate  [Esc] back    │
   └────────────────────────────────────────────────────────────────┘
   ```

6. **Jobs/Execution** (press '5')
   ```
   ┌─ Jobs ───────────────────────────────────── [f] filter ────────┐
   │ Job ID     Target      Command          Status      Duration   │
   ├────────────────────────────────────────────────────────────────┤
   │ job-a3f2   role:web    systemctl...     ✓ Done      2.1s       │
   │ job-b7c1   db-*        apt update       🔄 Running   15s        │
   │ job-c9d4   api-05      cat /etc/...     ✓ Done      0.5s       │
   │ job-d2e8   *           uptime           ❌ Failed    1.2s       │
   │ ...                                                             │
   │                                                                 │
   │ Running: 3   Queued: 1   Completed: 456   Failed: 2            │
   │                                                                 │
   │ [↑/↓] navigate  [Enter] output  [k] kill  [Esc] back          │
   └────────────────────────────────────────────────────────────────┘
   ```

7. **Logs Tail** (press '6')
   ```
   ┌─ Logs (live) ─────────────────────────── [/] filter [l] level ─┐
   │ 10:45:23 state-mgr    ERROR  Drift detected: /etc/nginx.conf   │
   │ 10:45:18 conn-mgr     INFO   Agent web-05 registered           │
   │ 10:45:12 policy-eng   WARN   Policy violation: SEC-SSH-001     │
   │ 10:45:05 cmd-dispatch INFO   Command job-a3f2 completed        │
   │ 10:44:58 state-mgr    INFO   Applied 12 state changes          │
   │ 10:44:45 agent-web-05 DEBUG  Heartbeat sent                    │
   │ ...                                                             │
   │ ⬇ LIVE (auto-scroll)                                           │
   │                                                                 │
   │ [l] level  [c] clear  [e] export  [/] filter  [Esc] back      │
   └────────────────────────────────────────────────────────────────┘
   ```

8. **Metrics** (press '7')
   ```
   ┌─ Metrics ──────────────────────────────────── [r] refresh ─────┐
   │                                                                 │
   │ API Requests/sec          ▂▃▅▇█▇▅▃▂ 1,234 req/s               │
   │ Command Execution Rate    ▂▂▃▄▅▄▃▂▂   145 cmd/s               │
   │ Event Processing Rate     ▃▄▆▇█▇▆▄▃   567 evt/s               │
   │                                                                 │
   │ CPU Usage                 ██████▒▒▒▒ 65%                       │
   │ Memory Usage              ████████▒▒ 82%                       │
   │ Goroutines                ████▒▒▒▒▒▒ 145 / 500                 │
   │                                                                 │
   │ Latency (p95)                                                  │
   │   Command Exec:  2.3s  ████████▒▒                             │
   │   State Apply:   1.8s  ██████▒▒▒▒                             │
   │   API Request:   45ms  █▒▒▒▒▒▒▒▒▒                             │
   │                                                                 │
   │ [↑/↓] navigate  [e] export  [Esc] back                        │
   └────────────────────────────────────────────────────────────────┘
   ```

**Keyboard Navigation**:
- `1-8` - Switch to specific view
- `Tab` / `Shift+Tab` - Next/previous view
- `j/k` or `↑/↓` - Navigate lists
- `gg` / `G` - Top/bottom of list
- `Ctrl+d` / `Ctrl+u` - Page down/up
- `/` - Search/filter
- `n` / `N` - Next/previous search result
- `r` - Refresh current view
- `p` - Pause/resume live updates
- `e` - Export current view
- `c` - Clear (logs/events)
- `Enter` - View details
- `Esc` - Back/cancel
- `?` - Help
- `q` - Quit

**Configuration**:
```yaml
# ~/.titananvil/monitor.yaml
monitor:
  # Connection
  control_plane: localhost:8080
  tls:
    enabled: true
    ca_cert: /path/to/ca.pem
    client_cert: /path/to/client.pem
    client_key: /path/to/client-key.pem

  # Refresh rates
  refresh:
    dashboard: 2s
    agents: 5s
    events: realtime
    state: 10s
    policy: 10s
    jobs: 3s
    logs: realtime
    metrics: 5s

  # Display
  theme: dark  # dark, light, solarized, monokai
  date_format: "15:04:05"
  timezone: "America/New_York"

  # Filters (persist across sessions)
  filters:
    log_level: info  # minimum level
    event_types: []  # empty = all
    agent_status: []  # empty = all

  # Limits
  max_rows:
    events: 1000
    logs: 1000
    agents: 10000

  # Export
  export_dir: ~/titananvil-exports
```

**CLI Usage**:
```bash
# Basic usage (connects to default control plane)
titanctl monitor

# Connect to specific control plane
titanctl monitor --endpoint prod-titan.example.com:8080

# Start with specific view
titanctl monitor --view agents

# Custom config
titanctl monitor --config /path/to/monitor.yaml

# Export mode (capture and exit)
titanctl monitor --export --duration 30s --output snapshot.json

# Filter from command line
titanctl monitor --filter "level>=warn"
titanctl monitor --view agents --filter "status=offline"
```

### US7.5: Grafana Dashboards
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

### US7.6: Health Checks
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

### US7.7: Performance Profiling
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

### US7.8: Infrastructure State Visualization
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

### Phase 4: TUI Monitor (Week 5-6)

**Week 1: Core Framework & Basic Views**

**T4.1: TUI Framework Setup**
- Add Bubble Tea, Lip Gloss, Bubbles dependencies
- Create basic TUI application structure
- Implement view switching (tabs)
- Set up keyboard navigation (vim-style)
- Implement help system (? key)
- Add quit handler (q key)
- Create base model/update/view architecture
- Terminal detection and fallback handling

**T4.2: Configuration System**
- Define configuration schema (monitor.yaml)
- Implement config file loading (~/.titananvil/monitor.yaml)
- Command-line flag parsing
  - `--endpoint` - control plane address
  - `--view` - starting view
  - `--config` - config file path
  - `--filter` - initial filter
  - `--theme` - color theme
- Environment variable support
- Config validation
- Default values for all settings

**T4.3: Control Plane Client**
- gRPC client for TitanAnvil API
- Authentication (TLS client certs)
- Connection management (reconnect logic)
- API methods:
  - GetSystemStatus() - uptime, version, metrics
  - ListAgents() - agent list with status
  - GetAgentDetails(id) - detailed agent info
  - ListJobs() - active and recent jobs
  - GetJobOutput(id) - job execution output
  - GetStateStatus() - drift summary, resource counts
  - GetPolicyStatus() - violations, compliance score
  - StreamMetrics() - real-time metrics stream
- Error handling and retries
- Connection status indicator

**T4.4: Event Bus Subscriber**
- NATS JetStream subscriber
- Subscribe to all event types
- Event filtering on client side
- Ring buffer for recent events (configurable size)
- Event parsing and formatting
- Correlation ID extraction
- Real-time event updates to UI
- Backpressure handling

**T4.5: Dashboard Overview View**
- Main dashboard layout (default view)
- System section:
  - Uptime display
  - Version information
  - API request rate
  - Event processing rate
  - Memory usage
  - Goroutine count
- Agent summary (connected/total, recent changes)
- Job summary (running/queued/completed/failed)
- State summary (resources, drift count by severity)
- Policy summary (violations by severity, compliance %)
- Recent events (last 5-10 events with severity colors)
- Auto-refresh every N seconds (configurable)
- Status indicators with emoji/colors (✓/⚠/❌)

**Week 2: Detailed Views & Advanced Features**

**T4.6: Agent List View**
- Sortable table component
  - Columns: ID, Hostname, OS, Status, Last Heartbeat, CPU, Memory
  - Sort by any column (click column header or hotkey)
  - Color-coded status (green=online, yellow=drift, red=offline)
- Pagination (for large fleets)
- Filtering:
  - By status (online/offline/drift)
  - By hostname/ID (fuzzy search)
  - By resource usage thresholds
- Agent detail modal (press Enter)
  - Full metadata
  - Recent commands
  - State application history
  - Policy violations
- Keyboard navigation (j/k, gg, G)
- Sparkline charts for CPU/Memory trends

**T4.7: Event Stream View**
- Live-scrolling event feed
- Syntax highlighting by severity
- Auto-scroll toggle (pause/resume with 'p')
- Filtering:
  - By event type
  - By severity level
  - By source component
  - By correlation ID
  - Search in event data
- Event expansion (press Enter)
  - Full JSON view
  - Pretty-printed with syntax highlighting
  - Copy to clipboard option
- Clear buffer ('c' key)
- Export to file ('e' key)
- Tail mode (follow last N events)
- Max buffer size (prevent memory exhaustion)

**T4.8: State Drift View**
- Table of drifted resources
  - Columns: Resource, Agent, Severity, Last Check
  - Severity color coding (red=high, yellow=medium, green=low)
- Drift summary statistics
- Group by:
  - Agent
  - Resource type
  - Severity
- Drill down to details (press Enter)
  - Expected vs actual state
  - Diff visualization
  - Remediation suggestions
- Quick apply action ('a' key)
  - Confirm dialog
  - Apply state to fix drift
  - Show progress
- Filter by severity
- Sort by last check time

**T4.9: Policy Violations View**
- Table of violations
  - Columns: Policy, Resource, Severity, Time
  - Severity indicators
- Compliance score display (% and bar chart)
- Group by:
  - Policy ID
  - Resource type
  - Severity
- Violation details (press Enter)
  - Full violation message
  - Policy code snippet
  - Remediation steps
  - Related resources
- Remediation action ('r' key)
  - Trigger automated remediation
  - Show progress
- Filter by severity/policy
- Sort by time/severity

**T4.10: Jobs/Execution View**
- Table of jobs
  - Columns: Job ID, Target, Command, Status, Duration
  - Status indicators (✓/🔄/❌)
- Job filtering:
  - By status (running/completed/failed)
  - By target expression
  - Time range
- Job output viewer (press Enter)
  - Real-time streaming output for running jobs
  - Full output for completed jobs
  - ANSI color support
  - Scrollable output
- Job actions:
  - Kill running job ('k' key)
  - Retry failed job ('r' key)
- Summary statistics (count by status, avg duration)

**T4.11: Logs Tail View**
- Live log stream from control plane
- Log level filtering (debug/info/warn/error)
- Source filtering (by logger name)
- Correlation ID filtering
- Search/highlight
- ANSI color support
- Auto-scroll toggle
- Clear buffer
- Export to file
- Adjustable buffer size

**T4.12: Metrics View**
- ASCII sparkline charts for time-series metrics
- Gauge visualizations (progress bars)
- Key metrics:
  - API requests/sec (sparkline)
  - Command execution rate (sparkline)
  - Event processing rate (sparkline)
  - CPU usage (gauge)
  - Memory usage (gauge)
  - Goroutine count (gauge)
  - Latency percentiles (horizontal bars)
- Auto-refresh
- Export current metrics snapshot

**Week 3: Polish & Advanced Features**

**T4.13: Theme System**
- Define color schemes:
  - Dark (default)
  - Light
  - Solarized Dark/Light
  - Monokai
  - Custom (user-defined in config)
- Apply theme to all views
- Runtime theme switching ('t' key)
- Respect terminal capabilities (256 color, true color)
- Fallback to basic colors if needed

**T4.14: Search & Filter Engine**
- Unified filter syntax across all views
- Filter expression parser:
  - Comparison operators: =, !=, >, <, >=, <=
  - Logical operators: AND, OR, NOT
  - Field access: status=online, cpu>80, severity=error
  - Regex support: name~"web-.*"
- Filter UI component (/ key to activate)
- Filter history (up/down arrows)
- Persistent filters (saved in config)
- Clear filter (Esc)
- Visual filter indicator (show active filters)

**T4.15: Export & Snapshot**
- Export current view to file
- Export formats:
  - JSON (structured data)
  - CSV (table views)
  - Text (formatted output)
  - Markdown (for documentation)
- Export to configurable directory
- Filename includes timestamp
- Snapshot mode (--export flag)
  - Capture current state
  - Exit immediately
  - Useful for scripts/automation
- Compression for large exports (gzip)

**T4.16: Performance Optimization**
- Efficient rendering (only redraw changed areas)
- Debounce rapid updates
- Limit refresh rates per view
- Background data fetching (don't block UI)
- Memory-efficient ring buffers
- Pagination for large datasets
- Lazy loading for details views
- Connection pooling for API calls
- Goroutine management (prevent leaks)

**Week 4: Testing, Documentation & Integration**

**T4.17: Unit Tests**
- Config loading and validation tests
- Filter expression parser tests
- Data model tests
- API client tests (with mock server)
- Event subscriber tests (with mock NATS)
- View rendering tests (basic)
- Keyboard handler tests
- Theme system tests
- Export functionality tests
- Target coverage: >75%

**T4.18: Integration Tests**
- End-to-end TUI tests
- Test with real control plane (Docker Compose)
- View switching and navigation
- Live event streaming
- API polling
- Filter and search
- Export functionality
- Theme switching
- Configuration loading
- Error handling (network failures, etc.)

**T4.19: Documentation**
- User guide:
  - Installation
  - Quick start
  - View descriptions
  - Keyboard shortcuts reference
  - Configuration guide
  - Filtering syntax
  - Export usage
  - Troubleshooting
- Developer guide:
  - Architecture overview
  - Adding new views
  - Extending filters
  - Custom themes
- Man page (titanctl-monitor.1)
- Built-in help system (? key)
- Example configurations

**T4.20: CLI Plugin Integration**
- Create `titananvil-monitor` binary
- Integrate with titanctl plugin system
- Command-line argument parsing
- Proper exit codes
- Signal handling (Ctrl+C, SIGTERM)
- Version command (--version)
- Help text (--help)
- Package for distribution

**T4.21: Polish & User Experience**
- Loading indicators for slow operations
- Error messages (user-friendly, actionable)
- Confirmation dialogs for destructive actions
- Responsive layout (adapt to terminal size)
- Graceful degradation (small terminals)
- Status bar with hints (context-sensitive)
- Smooth transitions between views
- Visual feedback for actions
- Accessibility considerations (screen readers)
- Demo mode (generate fake data for screenshots)

### Phase 5: Dashboards (Week 7-8)

**T5.1: Grafana Dashboard Development**
- Create dashboard templates
- Define panels and visualizations
- Add variables for filtering
- Export dashboards as JSON
- Documentation for dashboards

**T5.2: Alert Rules**
- Prometheus alert rules
- Alert rule templates
- Integration with Alertmanager
- Alert documentation

**T5.3: Dashboard Automation**
- Auto-import dashboards on deployment
- Dashboard versioning
- Custom dashboard support

### Phase 6: Health & Status (Week 9)

**T6.1: Health Check Endpoints**
- Implement liveness probe
- Implement readiness probe
- Detailed status endpoint
- Dependency health checks

**T6.2: Status API**
- Component status reporting
- Version information
- Uptime tracking
- System diagnostics

**T6.3: Self-Healing**
- Automatic component restart on failure
- Circuit breakers
- Graceful degradation
- Recovery metrics

### Phase 7: Advanced Features (Week 10)

**T7.1: Performance Profiling**
- Enable pprof endpoints
- Continuous profiling support
- Profile storage and analysis
- Profile visualization

**T7.2: Infrastructure Visualization**
- Web UI for topology
- Real-time agent status
- Interactive graph visualization
- Export capabilities

**T7.3: Query API**
- Metrics query API
- Logs query API
- Trace query API
- Unified query interface

## Dependencies

- **All Epics**: Instrumentation touches all components
- **Go Libraries**:
  - `github.com/prometheus/client_golang` - Prometheus client
  - `go.opentelemetry.io/otel` - OpenTelemetry
  - Structured logging (implemented in pkg/logging)
  - `github.com/charmbracelet/bubbletea` - TUI framework
  - `github.com/charmbracelet/lipgloss` - TUI styling
  - `github.com/charmbracelet/bubbles` - TUI components
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
- TUI component logic (views, filters, themes)
- TUI configuration parsing
- Health check logic

### Integration Tests
- End-to-end metric collection
- Log forwarding to aggregators
- Trace propagation across services
- TUI end-to-end testing with real control plane
- TUI event streaming and API polling
- Dashboard functionality

### Load Tests
- High-volume metric collection
- High-volume log generation
- Trace sampling under load
- TUI performance with large datasets (10k+ agents)
- Health check under load

### Manual Tests
- TUI usability testing across different terminals
- TUI responsiveness with various screen sizes
- Keyboard navigation and shortcuts
- Theme rendering in different terminal emulators

## Documentation Requirements

- [ ] Metrics reference guide
- [ ] Logging configuration guide
- [ ] Tracing setup guide
- [ ] TUI monitor user guide
  - [ ] Installation and quick start
  - [ ] Keyboard shortcuts reference
  - [ ] Configuration guide
  - [ ] Filtering syntax
  - [ ] Export functionality
  - [ ] Troubleshooting
- [ ] TUI monitor developer guide
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
- [ ] TUI monitor fully functional
  - [ ] All 8 views implemented
  - [ ] Real-time updates working
  - [ ] Filtering and search operational
  - [ ] Export functionality working
  - [ ] Multiple themes supported
  - [ ] Works across major terminal emulators
- [ ] Grafana dashboards available
- [ ] Health checks implemented
- [ ] Profiling enabled
- [ ] Documentation complete
- [ ] Integration tested with observability stack
- [ ] Production-ready
