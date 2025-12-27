---
title: "Observability"
weight: 11
description: >
  Comprehensive monitoring with Prometheus metrics, structured logging, distributed tracing, and Grafana dashboards
---

## Overview

Keystone Core provides comprehensive observability through metrics, logging, and tracing. Monitor system health, track performance, debug issues, and gain insights into your infrastructure operations.

**Three Pillars**:
- **Metrics**: Prometheus-compatible metrics for quantitative measurement
- **Logging**: Structured logs with correlation IDs for debugging
- **Tracing**: Distributed tracing with OpenTelemetry for request flow

**Additional Tools**:
- **Grafana Dashboards**: Pre-built dashboards for visualization
- **TUI Monitor**: Terminal-based real-time monitoring
- **Health Checks**: Kubernetes-compatible health endpoints
- **Performance Profiling**: pprof endpoints for profiling

## Architecture

```
┌─────────────────────────────────────────────┐
│        Keystone Core Components                 │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  │
│  │ Control  │  │  Agents  │  │  Plugins │  │
│  │  Plane   │  │          │  │          │  │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  │
└───────┼─────────────┼─────────────┼─────────┘
        │             │             │
   ┌────┴────┬────────┴────┬────────┴────┐
   │         │             │             │
   ↓         ↓             ↓             ↓
┌────────┐ ┌────────┐  ┌────────┐  ┌────────┐
│Metrics │ │  Logs  │  │ Traces │  │ Health │
│(Prom)  │ │(Struct)│  │ (OTLP) │  │Checks  │
└───┬────┘ └───┬────┘  └───┬────┘  └───┬────┘
    │          │           │           │
    ↓          ↓           ↓           ↓
┌────────┐ ┌────────┐  ┌────────┐  ┌────────┐
│Prometh-│ │  Loki  │  │Jaeger/ │  │  K8s   │
│  eus   │ │        │  │ Tempo  │  │ Probes │
└───┬────┘ └───┬────┘  └───┬────┘  └────────┘
    │          │           │
    └──────┬───┴───────┬───┘
           ↓           ↓
      ┌────────┐  ┌────────┐
      │Grafana │  │  TUI   │
      │Dashbrd │  │Monitor │
      └────────┘  └────────┘
```

## Metrics

Keystone Core exposes 70+ Prometheus-compatible metrics:

### Control Plane Metrics

**API Metrics**:
```
# HTTP requests
kscore_api_requests_total{method,path,status}
kscore_api_request_duration_seconds{method,path,quantile}

# Active connections
kscore_api_active_connections
```

**Agent Metrics**:
```
# Agent status
kscore_agents_connected_total{datacenter,environment,role}
kscore_agents_disconnected_total{reason}

# Heartbeats
kscore_agent_heartbeat_received_total
kscore_agent_heartbeat_missed_total
```

**Command Execution**:
```
# Commands
kscore_commands_executed_total{status,datacenter}
kscore_command_duration_seconds{quantile}

# Batch jobs
kscore_batch_jobs_total{status}
kscore_batch_size{quantile}
```

**State Management**:
```
# State applications
kscore_state_applications_total{status}
kscore_state_application_duration_seconds{quantile}

# Resources
kscore_state_resources_total{module}
kscore_state_changes_total{module}

# Drift
kscore_state_drift_detected_total{severity}
```

**Event System**:
```
# Events
kscore_events_published_total{type}
kscore_events_processed_total{type}
kscore_events_failed_total{type}

# Event processing
kscore_event_processing_duration_seconds{quantile}
kscore_event_lag_seconds
```

**Policy Enforcement**:
```
# Evaluations
kscore_policy_evaluations_total{policy,result}
kscore_policy_evaluation_duration_seconds{policy,quantile}

# Violations
kscore_policy_violations_total{policy,severity}
kscore_policy_compliance_score{policy_set,environment}

# Remediations
kscore_policy_remediations_total{policy,status}
```

**GitOps Integration**:
```
# Webhooks
kscore_gitops_webhooks_received_total{source}

# Verifications
kscore_gitops_verifications_total{status}
kscore_gitops_verification_duration_seconds{quantile}

# Rollbacks
kscore_gitops_rollbacks_total{type,status}
```

### Agent Metrics

**Resource Usage**:
```
# CPU
kscore_agent_cpu_usage_percent{agent_id}

# Memory
kscore_agent_memory_usage_bytes{agent_id}
kscore_agent_memory_total_bytes{agent_id}

# Disk
kscore_agent_disk_usage_bytes{agent_id,mount}
kscore_agent_disk_total_bytes{agent_id,mount}
```

**Operations**:
```
# Commands executed
kscore_agent_commands_executed_total{agent_id,status}

# States applied
kscore_agent_states_applied_total{agent_id,status}

# Events emitted
kscore_agent_events_emitted_total{agent_id,type}
```

**Connection**:
```
# Heartbeat
kscore_agent_heartbeat_sent_total{agent_id}

# Reconnections
kscore_agent_reconnections_total{agent_id}

# Connection status
kscore_agent_connected{agent_id,status}
```

### Metrics Endpoint

```bash
# Scrape metrics
curl http://control-plane:8080/metrics

# Example output
# HELP kscore_agents_connected_total Total connected agents
# TYPE kscore_agents_connected_total gauge
kscore_agents_connected_total{datacenter="us-east-1",environment="production",role="web"} 50
kscore_agents_connected_total{datacenter="us-west-2",environment="staging",role="db"} 10

# HELP kscore_api_request_duration_seconds API request duration
# TYPE kscore_api_request_duration_seconds summary
kscore_api_request_duration_seconds{method="POST",path="/api/v1/exec",quantile="0.5"} 0.025
kscore_api_request_duration_seconds{method="POST",path="/api/v1/exec",quantile="0.95"} 0.150
kscore_api_request_duration_seconds{method="POST",path="/api/v1/exec",quantile="0.99"} 0.500
```

### Prometheus Configuration

```yaml
# prometheus.yml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: 'kscore-control-plane'
    static_configs:
      - targets: ['control-plane-1:8080', 'control-plane-2:8080']

  - job_name: 'kscore-agents'
    # Auto-discover agents
    consul_sd_configs:
      - server: 'consul:8500'
        services: ['kscore-agent']
```

## Logging

Keystone Core uses structured logging with correlation IDs:

### Log Levels

- **Debug**: Detailed debugging information
- **Info**: General informational messages
- **Warn**: Warning messages (non-critical issues)
- **Error**: Error messages (requires attention)

### Log Formats

**JSON** (default, machine-readable):
```json
{
  "timestamp": "2024-01-15T10:30:45.123Z",
  "level": "info",
  "logger": "command-dispatcher",
  "message": "Command executed successfully",
  "correlation_id": "job-abc123",
  "fields": {
    "job_id": "abc123",
    "agent_id": "web-01",
    "command": "systemctl restart nginx",
    "duration_ms": 1234,
    "exit_code": 0
  }
}
```

**Logfmt** (key=value format):
```
timestamp=2024-01-15T10:30:45.123Z level=info logger=command-dispatcher message="Command executed successfully" correlation_id=job-abc123 job_id=abc123 agent_id=web-01 command="systemctl restart nginx" duration_ms=1234 exit_code=0
```

**Text** (human-readable, colored):
```
2024-01-15 10:30:45.123 [INFO] command-dispatcher: Command executed successfully
  correlation_id: job-abc123
  job_id: abc123
  agent_id: web-01
  command: systemctl restart nginx
  duration_ms: 1234
  exit_code: 0
```

### Correlation IDs

Track requests across components:

```json
{
  "correlation_id": "job-abc123",
  "message": "Command started"
}

{
  "correlation_id": "job-abc123",
  "message": "Command dispatched to agent"
}

{
  "correlation_id": "job-abc123",
  "message": "Command executed successfully"
}
```

**Query logs by correlation ID**:
```bash
kscorectl logs --correlation-id job-abc123
```

### Log Configuration

```yaml
logging:
  level: info              # debug, info, warn, error
  format: json             # json, logfmt, text
  output: stdout           # stdout, file
  file: /var/log/kscore/server.log
  max_size: 100MB
  max_backups: 3
  max_age: 30              # days
  compress: true
```

### Loki Integration

Send logs to Grafana Loki:

```yaml
logging:
  loki:
    enabled: true
    url: "http://loki:3100/loki/api/v1/push"
    labels:
      service: kscore-server
      environment: production
```

### Querying Logs

**LogQL** (Loki query language):
```
# All logs from control plane
{service="kscore-server"}

# Error logs from command dispatcher
{service="kscore-server",logger="command-dispatcher"} |= "level=error"

# Logs for specific job
{service="kscore-server"} |= "correlation_id=job-abc123"

# Failed commands in last hour
{service="kscore-server",logger="command-dispatcher"} |= "exit_code!=0" | json
```

## Tracing

Distributed tracing with OpenTelemetry:

### Trace Context

Every request gets a trace context:

```
TraceID: 1234567890abcdef
SpanID: abc123def456
ParentSpanID: def456abc123
```

### Span Hierarchy

```
Trace: Execute Command (trace_id: 1234567890abcdef)
├─ Span: Dispatch Command (span_id: abc123)
│  ├─ Span: Resolve Targets (span_id: def456)
│  │  └─ Span: Query Agents (span_id: ghi789)
│  └─ Span: Publish to NATS (span_id: jkl012)
├─ Span: Agent Execute (span_id: mno345)
│  ├─ Span: Validate Command (span_id: pqr678)
│  └─ Span: Run Shell Command (span_id: stu901)
└─ Span: Collect Results (span_id: vwx234)
```

### Span Attributes

```json
{
  "span_name": "Execute Command",
  "span_id": "abc123",
  "trace_id": "1234567890abcdef",
  "parent_span_id": null,
  "start_time": "2024-01-15T10:30:45.000Z",
  "end_time": "2024-01-15T10:30:46.234Z",
  "duration_ms": 1234,
  "attributes": {
    "command": "systemctl restart nginx",
    "agent_id": "web-01",
    "datacenter": "us-east-1",
    "exit_code": 0
  }
}
```

### OpenTelemetry Configuration

```yaml
tracing:
  enabled: true
  exporter: otlp           # otlp, jaeger, zipkin
  endpoint: "http://jaeger:4318"

  # Sampling
  sampling:
    strategy: ratio        # always, never, ratio
    ratio: 0.1             # Sample 10% of traces

  # Resource attributes
  resource:
    service.name: kscore-server
    service.version: 1.0.0
    deployment.environment: production
```

### Jaeger UI

View traces in Jaeger:

```bash
# Open Jaeger UI
http://jaeger-ui:16686

# Search for traces
Service: kscore-server
Operation: Execute Command
Tags: agent_id=web-01
Min Duration: 1s
Max Duration: 10s
```

## Grafana Dashboards

Keystone Core provides 6 pre-built Grafana dashboards:

### 1. Keystone Core Overview

**System-wide metrics**:
- Total agents (connected, offline, degraded)
- Command execution rate
- State application rate
- Policy violations
- Event rate
- Recent events timeline

**Variables**:
- Environment (production, staging, dev)
- Datacenter (us-east-1, us-west-2, etc.)

### 2. Control Plane Health

**Metrics**:
- Control plane status and uptime
- CPU and memory usage
- API request rate and latency (p95, p99)
- NATS message throughput
- Database query latency
- Error rates by component

**Alerts**:
- High CPU usage (>80%)
- High memory usage (>90%)
- High API latency (p95 >1s)
- High error rate (>5%)

### 3. Agent Fleet

**Metrics**:
- Fleet health status (healthy, degraded, offline)
- Agent distribution by datacenter and role
- Command execution success rate per agent
- Agent resource utilization (CPU, memory, disk)
- Agent version distribution

**Variables**:
- Datacenter
- Role
- Agent ID

### 4. State Management

**Metrics**:
- State applications (success, failure)
- Drift detection events by severity
- State changes by module
- Application duration percentiles
- Resources under management

**Variables**:
- Environment
- Module (file, package, service, etc.)

### 5. Policy Compliance

**Metrics**:
- Overall compliance score (gauge)
- Violations by severity (critical, high, medium, low)
- Remediation success rate
- Top violated policies
- Compliance trends (7-day average)

**Variables**:
- Policy framework (OPA, CEL)
- Environment

### 6. GitOps Operations

**Metrics**:
- Deployment verification metrics
- Verification success rate
- Rollback frequency and reasons
- Deployment duration percentiles
- Failed verifications by application

**Variables**:
- Application
- Environment

### Dashboard Access

```bash
# Import dashboards
http://grafana:3000

# Dashboards location
Dashboards → Keystone Core → [Dashboard Name]
```

## TUI Monitor

Terminal-based real-time monitoring (`kscore-monitor`):

### Views

**1. Dashboard** (default view):
- System overview
- Agent counts
- Recent events
- Job statistics

**2. Agents**:
- Interactive table with agents
- Status, metadata, heartbeat
- Filter by datacenter/environment/role

**3. Events**:
- Real-time event stream
- Filter by type, severity
- Search events
- Pause/resume stream

**4. State Drift**:
- Configuration drift monitoring
- Drift by severity
- Affected resources

**5. Policy Violations**:
- Policy compliance tracking
- Violations by policy
- Remediation status

**6. Jobs**:
- Command execution history
- Job status
- Output logs

**7. Logs**:
- Structured log streaming
- Filter by level, logger
- Correlation ID search

**8. Metrics**:
- Performance metrics
- Resource utilization
- Real-time graphs

### Navigation

```
Keys:
  1-8     Switch views
  ↑/↓     Scroll
  /       Search
  f       Filter
  r       Refresh
  p       Pause/Resume
  q       Quit
```

### Running the Monitor

```bash
# Start monitor
kscorectl monitor

# Connect to specific control plane
kscorectl monitor --server control-plane.example.com:8080

# Start with specific view
kscorectl monitor --view events
```

## Health Checks

Kubernetes-compatible health endpoints:

### Liveness Probe

**Endpoint**: `GET /health/live`

**Response**:
```json
{
  "status": "healthy",
  "timestamp": "2024-01-15T10:30:45Z"
}
```

**Status Codes**:
- `200 OK`: Process is alive
- `503 Service Unavailable`: Process is not responding

### Readiness Probe

**Endpoint**: `GET /health/ready`

**Response**:
```json
{
  "status": "ready",
  "timestamp": "2024-01-15T10:30:45Z",
  "checks": {
    "nats": "healthy",
    "database": "healthy",
    "agents": "healthy"
  }
}
```

**Status Codes**:
- `200 OK`: Ready to receive traffic
- `503 Service Unavailable`: Not ready (dependencies unavailable)

### Detailed Status

**Endpoint**: `GET /health/status`

**Response**:
```json
{
  "status": "healthy",
  "timestamp": "2024-01-15T10:30:45Z",
  "uptime": "72h30m15s",
  "version": "1.0.0",
  "components": {
    "api_server": {
      "status": "healthy",
      "uptime": "72h30m15s"
    },
    "nats": {
      "status": "healthy",
      "connections": 150,
      "messages_per_sec": 1234
    },
    "database": {
      "status": "healthy",
      "connections": 10,
      "query_latency_ms": 5
    },
    "agent_pool": {
      "status": "healthy",
      "connected_agents": 150,
      "disconnected_agents": 2
    }
  }
}
```

### Kubernetes Probes

```yaml
# Deployment manifest
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kscore-server
spec:
  template:
    spec:
      containers:
      - name: server
        image: kscore/server:latest
        livenessProbe:
          httpGet:
            path: /health/live
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 10

        readinessProbe:
          httpGet:
            path: /health/ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
```

## Performance Profiling

pprof endpoints for profiling:

### Available Profiles

```bash
# CPU profile
curl http://control-plane:8080/debug/pprof/profile?seconds=30 > cpu.prof

# Heap profile
curl http://control-plane:8080/debug/pprof/heap > heap.prof

# Goroutine profile
curl http://control-plane:8080/debug/pprof/goroutine > goroutine.prof

# Mutex profile
curl http://control-plane:8080/debug/pprof/mutex > mutex.prof

# Block profile
curl http://control-plane:8080/debug/pprof/block > block.prof

# Trace
curl http://control-plane:8080/debug/pprof/trace?seconds=5 > trace.out
```

### Analyzing Profiles

```bash
# CPU profile
go tool pprof -http=:8081 cpu.prof

# Heap profile
go tool pprof -http=:8081 heap.prof

# Compare profiles
go tool pprof -http=:8081 -base heap1.prof heap2.prof

# Trace
go tool trace trace.out
```

## Best Practices

### Metrics

1. **Consistent Labels**: Use consistent label names across metrics
2. **Cardinality**: Avoid high-cardinality labels (user IDs, request IDs)
3. **Naming**: Follow Prometheus naming conventions
4. **Aggregation**: Use histograms/summaries for latency metrics

### Logging

1. **Structured Logs**: Always use structured logging (JSON/logfmt)
2. **Correlation IDs**: Include correlation IDs in all logs
3. **Log Levels**: Use appropriate log levels (don't log everything as info)
4. **Sensitive Data**: Never log secrets, passwords, tokens

### Tracing

1. **Sampling**: Use sampling in production to reduce overhead
2. **Span Attributes**: Add relevant attributes to spans
3. **Errors**: Mark spans as error when operations fail
4. **Context Propagation**: Always propagate trace context

### Dashboards

1. **Focus**: Each dashboard should have a specific purpose
2. **Variables**: Use variables for filtering (environment, datacenter)
3. **Alerts**: Link dashboards to relevant alerts
4. **Documentation**: Add panel descriptions

## Troubleshooting

### High Metrics Cardinality

**Problem**: Prometheus running out of memory

**Cause**: Too many unique label combinations

Fix:
```yaml
# Remove high-cardinality labels
# BEFORE (bad):
kscore_requests_total{user_id="12345",request_id="abc"}

# AFTER (good):
kscore_requests_total{endpoint="/api/exec"}
```

### Missing Logs

**Problem**: Logs not appearing in Loki

Check:
```bash
# Check log output
kscorectl logs --tail 100

# Check Loki connectivity
curl http://loki:3100/ready

# Check Loki ingestion
curl http://loki:3100/metrics | grep loki_ingester
```

### Incomplete Traces

**Problem**: Traces missing spans

Check:
```bash
# Verify trace context propagation
kscorectl logs | grep trace_id

# Check sampling rate
curl http://control-plane:8080/health/status | jq '.tracing'

# Increase sampling temporarily
# tracing.sampling.ratio: 1.0
```

## Next Steps

- Set up [Prometheus](https://prometheus.io/) for metrics
- Deploy [Grafana](https://grafana.com/) with Keystone Core dashboards
- Configure [Loki](https://grafana.com/oss/loki/) for log aggregation
- Set up [Jaeger](https://www.jaegertracing.io/) or [Tempo](https://grafana.com/oss/tempo/) for tracing
- Review alert rules in `deploy/grafana/alerts/`
