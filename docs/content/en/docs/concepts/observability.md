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

```mermaid
flowchart TD
    subgraph Components["Keystone Core Components"]
        CP["Control Plane"]
        Agents["Agents"]
        Plugins["Plugins"]
    end

    CP --> Metrics["Metrics (Prom)"]
    CP --> Logs["Logs (Struct)"]
    CP --> Traces["Traces (OTLP)"]
    CP --> Health["Health Checks"]

    Agents --> Metrics
    Agents --> Logs
    Agents --> Traces
    Agents --> Health

    Plugins --> Metrics
    Plugins --> Logs
    Plugins --> Traces
    Plugins --> Health

    Metrics --> Prometheus["Prometheus"]
    Logs --> Loki["Loki"]
    Traces --> Jaeger["Jaeger/Tempo"]
    Health --> K8s["K8s Probes"]

    Prometheus --> Grafana["Grafana Dashboard"]
    Loki --> Grafana
    Jaeger --> Grafana

    Prometheus --> TUI["TUI Monitor"]
    Loki --> TUI
    Jaeger --> TUI
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
kscore_agents_connected{datacenter,environment,role}
kscore_agents_disconnected_total{reason}

# Heartbeats
kscore_agent_heartbeat_received_total
kscore_agent_heartbeat_missed_total
```

**Command Execution**:

```
# Commands
kscore_command_executions_total{status,datacenter}
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
# HELP kscore_agents_connected Total connected agents
# TYPE kscore_agents_connected gauge
kscore_agents_connected{datacenter="us-east-1",environment="production",role="web"} 50
kscore_agents_connected{datacenter="us-west-2",environment="staging",role="db"} 10

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

### Metrics Retention Sizing

Properly sizing metrics retention depends on scrape interval, metric cardinality, and storage capacity. Use these guidelines to calculate storage requirements.

#### Storage Formula

Prometheus storage requirements can be estimated using:

```
Storage (bytes/day) = samples_per_second × bytes_per_sample × seconds_per_day
                    = (metrics × cardinality / scrape_interval) × 2 × 86400
```

Where:

- **metrics**: Number of unique metric names (~70 for Keystone Core)
- **cardinality**: Average label combinations per metric
- **scrape_interval**: Seconds between scrapes (default 15s)
- **bytes_per_sample**: ~2 bytes (Prometheus compressed)

#### Cardinality by Deployment Size

| Deployment Size | Agents | Estimated Cardinality | Daily Storage |
|-----------------|--------|----------------------|---------------|
| Small | 1-50 | ~500 | ~600 MB |
| Medium | 50-500 | ~5,000 | ~6 GB |
| Large | 500-5,000 | ~50,000 | ~60 GB |
| Enterprise | 5,000+ | ~500,000+ | ~600+ GB |

**Cardinality drivers**:

- Agent count (each agent adds label combinations)
- Datacenters/environments (multiplier for many metrics)
- Command diversity (unique commands increase `command` label cardinality)
- Custom labels added to agents

#### Recommended Retention by Use Case

| Use Case | Retention | Typical Storage | Notes |
|----------|-----------|-----------------|-------|
| Real-time monitoring | 2 hours | Minimal | Hot queries only |
| Troubleshooting | 7 days | 1× daily | Recent incident investigation |
| Capacity planning | 30 days | 4× daily | Trend analysis |
| Compliance/audit | 90 days | 12× daily | Regulatory requirements |
| Historical analysis | 1 year | 48× daily | Long-term patterns |

#### Scrape Interval Impact

Shorter scrape intervals provide higher resolution but increase storage:

| Scrape Interval | Samples/Day | Storage Multiplier | Use Case |
|-----------------|-------------|-------------------|----------|
| 5s | 17,280 | 3× | Critical real-time monitoring |
| 15s (default) | 5,760 | 1× | Standard monitoring |
| 30s | 2,880 | 0.5× | Cost-sensitive deployments |
| 60s | 1,440 | 0.25× | Low-priority metrics |

#### Prometheus Storage Configuration

Configure retention based on your requirements:

```yaml
# prometheus.yml
global:
  scrape_interval: 15s

# Command-line flags for storage
# --storage.tsdb.retention.time=30d
# --storage.tsdb.retention.size=100GB
```

**Retention by time vs size**:

```bash
# Retain 30 days of data
prometheus --storage.tsdb.retention.time=30d

# Retain up to 100GB (removes oldest data when exceeded)
prometheus --storage.tsdb.retention.size=100GB

# Both (whichever limit is reached first)
prometheus \
  --storage.tsdb.retention.time=30d \
  --storage.tsdb.retention.size=100GB
```

#### Sizing Examples

**Example 1: Small deployment (50 agents, 7-day retention)**

```
Metrics: 70
Cardinality: 500 (50 agents × 10 avg label combinations)
Scrape interval: 15s
Samples/second: 70 × 500 / 15 = 2,333

Daily storage: 2,333 × 2 × 86,400 = 403 MB/day
7-day storage: 403 × 7 = 2.8 GB

Recommended: 5 GB with 20% headroom
```

**Example 2: Medium deployment (500 agents, 30-day retention)**

```
Metrics: 70
Cardinality: 5,000 (500 agents × 10 avg label combinations)
Scrape interval: 15s
Samples/second: 70 × 5,000 / 15 = 23,333

Daily storage: 23,333 × 2 × 86,400 = 4.03 GB/day
30-day storage: 4.03 × 30 = 121 GB

Recommended: 150 GB with 25% headroom
```

**Example 3: Large deployment (5,000 agents, 90-day retention)**

```
Metrics: 70
Cardinality: 50,000 (5,000 agents × 10 avg label combinations)
Scrape interval: 15s
Samples/second: 70 × 50,000 / 15 = 233,333

Daily storage: 233,333 × 2 × 86,400 = 40.3 GB/day
90-day storage: 40.3 × 90 = 3.6 TB

Recommended: 4.5 TB with 25% headroom
Consider: Remote write to long-term storage (Thanos, Cortex, Mimir)
```

#### Reducing Cardinality

High cardinality increases storage costs. Reduce with these strategies:

**1. Aggregate high-cardinality labels**:

```yaml
# prometheus.yml - Use relabeling to drop high-cardinality labels
scrape_configs:
  - job_name: 'kscore-agents'
    metric_relabel_configs:
      # Drop command label for command_duration metric
      - source_labels: [__name__]
        regex: kscore_command_duration_seconds
        target_label: command
        replacement: ''
```

**2. Use recording rules for aggregation**:

```yaml
# recording_rules.yml
groups:
  - name: kscore_aggregations
    rules:
      # Aggregate command metrics by datacenter only
      - record: kscore:commands_executed:by_dc
        expr: sum by (datacenter, status) (kscore_command_executions_total)

      # Aggregate agent count by environment
      - record: kscore:agents_connected:by_env
        expr: sum by (environment) (kscore_agents_connected)
```

**3. Filter unnecessary metrics**:

```yaml
# Scrape only essential metrics
scrape_configs:
  - job_name: 'kscore-control-plane'
    metric_relabel_configs:
      # Keep only specified metrics
      - source_labels: [__name__]
        regex: kscore_(agents_connected|api_requests|command_executions).*
        action: keep
```

#### Long-Term Storage

For retention beyond 30-90 days, use external long-term storage:

**Thanos**:

```yaml
# thanos-sidecar.yml
apiVersion: v1
kind: Pod
spec:
  containers:
    - name: prometheus
      args:
        - --storage.tsdb.retention.time=2h  # Local retention only
        - --storage.tsdb.min-block-duration=2h
        - --storage.tsdb.max-block-duration=2h
    - name: thanos-sidecar
      args:
        - sidecar
        - --tsdb.path=/prometheus
        - --objstore.config-file=/etc/thanos/bucket.yml
```

**Grafana Mimir**:

```yaml
# mimir-config.yml
# Remote write from Prometheus to Mimir
remote_write:
  - url: http://mimir:9009/api/v1/push
    remote_timeout: 30s
    queue_config:
      capacity: 10000
      max_samples_per_send: 5000
```

**Victoria Metrics**:

```yaml
# prometheus.yml - Remote write to VictoriaMetrics
remote_write:
  - url: http://victoriametrics:8428/api/v1/write
    queue_config:
      max_samples_per_send: 10000
      capacity: 20000
```

#### Monitoring Storage Health

Monitor Prometheus storage to prevent issues:

```yaml
# Alert when storage is running low
groups:
  - name: prometheus_storage
    rules:
      - alert: PrometheusStorageNearCapacity
        expr: |
          (prometheus_tsdb_storage_blocks_bytes / prometheus_tsdb_retention_limit_bytes) > 0.8
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Prometheus storage at {{ $value | humanizePercentage }}"

      - alert: PrometheusHighCardinality
        expr: prometheus_tsdb_head_series > 1000000
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "High metric cardinality: {{ $value }} series"

      - alert: PrometheusScrapeErrors
        expr: rate(prometheus_target_scrapes_sample_out_of_order_total[5m]) > 0
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Out-of-order samples detected"
```

#### Quick Reference

| Agents | 7d Retention | 30d Retention | 90d Retention |
|--------|--------------|---------------|---------------|
| 50 | 3 GB | 12 GB | 36 GB |
| 100 | 5 GB | 22 GB | 65 GB |
| 500 | 25 GB | 100 GB | 300 GB |
| 1,000 | 50 GB | 200 GB | 600 GB |
| 5,000 | 250 GB | 1 TB | 3 TB |
| 10,000 | 500 GB | 2 TB | 6 TB |

*Based on 15s scrape interval, default metrics, ~10 label combinations per agent*

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
  file: /var/log/keystone-core/server.log
  max_size: 100MB
  max_backups: 3
  max_age: 30              # days
  compress: true
```

### NATS Log Transport

Send logs to a centralized collector via NATS:

```yaml
logging:
  nats:
    enabled: true
    url: "nats://localhost:4222"
    subject: "kscore.logs"
    subject_per_level: true    # Use kscore.logs.{level}
    subject_per_service: false # Use kscore.logs.{service}
    buffer_size: 10000         # Async buffer size
    flush_interval: 1s         # Flush interval

    # Authentication
    token: ""
    user: ""
    password: ""
    nkey_file: ""
    cred_file: ""
```

**Message Format** (JSON):

```json
{
  "timestamp": "2024-01-15T10:30:45.123Z",
  "level": "info",
  "logger": "command-dispatcher",
  "message": "Command executed successfully",
  "correlation_id": "job-abc123",
  "service": "kscore-server",
  "caller": "dispatcher.go:123",
  "fields": {
    "job_id": "abc123",
    "agent_id": "web-01"
  }
}
```

**Subject Routing**:

- Default: `kscore.logs`
- Per-level: `kscore.logs.info`, `kscore.logs.error`
- Per-service: `kscore.logs.kscore-server`

**Subscribe to logs**:

```bash
# All logs
nats sub "kscore.logs.>"

# Error logs only
nats sub "kscore.logs.error"

# Logs from specific service
nats sub "kscore.logs.kscore-server"
```

### Stdout-First Logging

Keystone Core defaults to stdout logging for container/systemd integration:

```yaml
logging:
  output: stdout     # stdout (default), stderr, file
  format: json       # json (default), logfmt, text
  level: info        # debug, info, warn, error

  # Include caller info
  caller: true       # Include file:line in logs

  # Environment variables override config
  # KSCORE_LOG_LEVEL, KSCORE_LOG_FORMAT, KSCORE_LOG_OUTPUT
```

**Container Integration** (Docker/Kubernetes):

```bash
# View logs via container runtime
docker logs kscore-server
kubectl logs deployment/kscore-server

# Structured logs work with:
# - Docker json-file driver
# - Kubernetes logging agents (Fluentd, Fluent Bit)
# - CloudWatch Container Insights
# - Google Cloud Logging
```

**systemd/journald Integration**:

```bash
# View logs via journalctl
journalctl -u kscore-server -f

# Filter by level (in JSON format)
journalctl -u kscore-server -o json | jq 'select(.level == "error")'
```

### Syslog Integration

Send logs to syslog for traditional infrastructure:

```yaml
logging:
  syslog:
    enabled: true
    transport: unix    # unix, udp, tcp, tcp+tls
    address: "/dev/log"  # or host:port for network
    facility: local0

    # RFC 5424 (modern, default)
    format: rfc5424
    app_name: kscore-server

    # RFC 3164 (BSD syslog)
    # format: bsd

    # TLS for tcp+tls transport
    tls:
      cert_file: /etc/keystone-core/certs/client.crt
      key_file: /etc/keystone-core/certs/client.key
      ca_file: /etc/keystone-core/certs/ca.crt
```

**RFC 5424 Format**:

```
<134>1 2024-01-15T10:30:45.123Z hostname kscore-server 1234 - [kscore@49152 level="info" correlation_id="abc123"] Command executed
```

**BSD Format** (RFC 3164):

```
<134>Jan 15 10:30:45 hostname kscore-server[1234]: Command executed
```

**rsyslog Configuration**:

```
# /etc/rsyslog.d/kscore.conf
local0.* /var/log/keystone-core.log
```

### SIEM Field Mapping

Keystone emits structured log data that maps to common SIEM schemas. This section documents field mappings for popular SIEM platforms.

#### Keystone Log Field Reference

Keystone logs contain these standard fields:

| Keystone Field | Type | Description |
|----------------|------|-------------|
| `timestamp` | ISO 8601 | Event timestamp with microsecond precision |
| `level` | string | Log level: debug, info, warn, error, fatal |
| `logger` | string | Component name (e.g., command-dispatcher) |
| `message` | string | Human-readable event description |
| `correlation_id` | string | Request/job correlation identifier |
| `trace_id` | string | Distributed trace identifier |
| `span_id` | string | Span identifier within trace |
| `agent_id` | string | Agent UUID |
| `agent_name` | string | Agent hostname or display name |
| `command_id` | string | Command execution identifier |
| `job_id` | string | Batch job identifier |
| `user` | string | Authenticated username |
| `source_ip` | string | Client IP address |
| `duration_ms` | number | Operation duration in milliseconds |
| `exit_code` | number | Command exit code |
| `error` | string | Error message if applicable |
| `error_code` | string | Structured error code |

#### Splunk Common Information Model (CIM)

Map Keystone fields to Splunk CIM for consistent searches across data sources:

```
# props.conf - Field extraction
[kscore:json]
SHOULD_LINEMERGE = false
TIME_FORMAT = %Y-%m-%dT%H:%M:%S.%6N%Z
TIME_PREFIX = "timestamp":"
KV_MODE = json
TRUNCATE = 0

# Field aliases for CIM compliance
FIELDALIAS-action = message AS action
FIELDALIAS-app = logger AS app
FIELDALIAS-dest = agent_name AS dest
FIELDALIAS-dest_ip = agent_ip AS dest_ip
FIELDALIAS-duration = duration_ms AS duration
FIELDALIAS-result = exit_code AS result
FIELDALIAS-signature = error_code AS signature
FIELDALIAS-src = source_ip AS src
FIELDALIAS-user = user AS user

# CIM-compliant event categorization
EVAL-vendor = "Anthropic"
EVAL-product = "Keystone"
EVAL-vendor_product = "Anthropic Keystone"
```

**CIM Data Model Mappings**:

| CIM Field | Keystone Field | Data Model |
|-----------|----------------|------------|
| `action` | `message` | Change |
| `app` | `logger` | Application State |
| `command` | `command_name` | Endpoint.Processes |
| `dest` | `agent_name` | Common |
| `duration` | `duration_ms` | Common |
| `object` | `resource_type` | Change |
| `object_path` | `resource_id` | Change |
| `result` | `exit_code` | Common |
| `severity` | `level` | Common |
| `signature` | `error_code` | Intrusion Detection |
| `src` | `source_ip` | Common |
| `status` | `status` | Common |
| `user` | `user` | Authentication |
| `vendor_product` | (static) | Common |

**Splunk Search Examples**:

```spl
# All authentication events
index=kscore sourcetype="kscore:json" logger="auth"
| stats count by user, src, action

# Failed commands by agent
index=kscore sourcetype="kscore:json" exit_code!=0
| stats count by dest, command_name
| sort -count

# CIM-compliant change tracking
| datamodel Change search
| search vendor_product="Anthropic Keystone"
| stats count by object, action, user
```

#### Elastic Common Schema (ECS)

Map Keystone fields to ECS for Elastic SIEM and Kibana:

```yaml
# Filebeat module configuration
- module: kscore
  log:
    enabled: true
    var.paths:
      - /var/log/keystone-core/*.log
    var.format: json
```

**ECS Field Mappings**:

| ECS Field | Keystone Field | ECS Category |
|-----------|----------------|--------------|
| `@timestamp` | `timestamp` | Base |
| `event.action` | `message` | Event |
| `event.category` | (derived) | Event |
| `event.dataset` | `logger` | Event |
| `event.duration` | `duration_ms * 1000000` | Event |
| `event.kind` | (derived) | Event |
| `event.module` | `"kscore"` | Event |
| `event.outcome` | (derived from exit_code) | Event |
| `event.severity` | (derived from level) | Event |
| `host.hostname` | `agent_name` | Host |
| `host.id` | `agent_id` | Host |
| `log.level` | `level` | Log |
| `log.logger` | `logger` | Log |
| `message` | `message` | Base |
| `process.exit_code` | `exit_code` | Process |
| `process.name` | `command_name` | Process |
| `source.ip` | `source_ip` | Source |
| `trace.id` | `trace_id` | Tracing |
| `transaction.id` | `correlation_id` | Tracing |
| `user.name` | `user` | User |

**Logstash Pipeline**:

```ruby
# /etc/logstash/conf.d/kscore.conf
input {
  file {
    path => "/var/log/keystone-core/*.log"
    codec => json
    tags => ["kscore"]
  }
}

filter {
  if "kscore" in [tags] {
    # ECS field mapping
    mutate {
      rename => {
        "timestamp" => "@timestamp"
        "logger" => "[log][logger]"
        "level" => "[log][level]"
        "agent_id" => "[host][id]"
        "agent_name" => "[host][hostname]"
        "source_ip" => "[source][ip]"
        "user" => "[user][name]"
        "trace_id" => "[trace][id]"
        "correlation_id" => "[transaction][id]"
        "command_name" => "[process][name]"
        "exit_code" => "[process][exit_code]"
      }
    }

    # Convert duration from ms to nanoseconds (ECS standard)
    if [duration_ms] {
      ruby {
        code => "event.set('[event][duration]', event.get('duration_ms').to_i * 1000000)"
      }
      mutate { remove_field => ["duration_ms"] }
    }

    # Derive event.outcome from exit_code
    if [process][exit_code] {
      if [process][exit_code] == 0 {
        mutate { add_field => { "[event][outcome]" => "success" } }
      } else {
        mutate { add_field => { "[event][outcome]" => "failure" } }
      }
    }

    # Map log level to ECS severity
    translate {
      field => "[log][level]"
      destination => "[event][severity]"
      dictionary => {
        "debug" => "1"
        "info" => "2"
        "warn" => "3"
        "error" => "4"
        "fatal" => "5"
      }
    }

    # Set event categorization
    mutate {
      add_field => {
        "[event][module]" => "kscore"
        "[event][dataset]" => "%{[log][logger]}"
      }
    }

    # Derive event.category from logger
    if [log][logger] == "auth" {
      mutate { add_field => { "[event][category]" => "authentication" } }
    } else if [log][logger] == "command-dispatcher" {
      mutate { add_field => { "[event][category]" => "process" } }
    } else if [log][logger] == "state-manager" {
      mutate { add_field => { "[event][category]" => "configuration" } }
    } else if [log][logger] == "policy-engine" {
      mutate { add_field => { "[event][category]" => "intrusion_detection" } }
    }
  }
}

output {
  if "kscore" in [tags] {
    elasticsearch {
      hosts => ["https://elasticsearch:9200"]
      index => "kscore-%{+YYYY.MM.dd}"
      ssl_certificate_verification => true
    }
  }
}
```

**Kibana Query Examples**:

```
# All failed commands
event.outcome: "failure" AND event.module: "kscore"

# Authentication events by user
event.category: "authentication" AND user.name: *

# Configuration changes in last 24 hours
event.category: "configuration" AND @timestamp >= now-24h
```

#### Common Event Format (CEF)

CEF mapping for ArcSight, QRadar, and other SIEM platforms:

**CEF Header Mapping**:

```
CEF:0|Anthropic|Keystone|1.0|<signature_id>|<name>|<severity>|<extension>
```

| CEF Field | Keystone Source | Notes |
|-----------|-----------------|-------|
| `Version` | `0` | CEF version |
| `Device Vendor` | `Anthropic` | Static |
| `Device Product` | `Keystone` | Static |
| `Device Version` | `version` | Keystone version |
| `Signature ID` | `error_code` or `event_type` | Event identifier |
| `Name` | `message` | Event description |
| `Severity` | `level` | 0-10 scale mapping |

**Severity Mapping**:

| Keystone Level | CEF Severity |
|----------------|--------------|
| `debug` | `0` |
| `info` | `3` |
| `warn` | `5` |
| `error` | `7` |
| `fatal` | `10` |

**CEF Extension Fields**:

| CEF Extension | Keystone Field | Key |
|---------------|----------------|-----|
| `sourceAddress` | `source_ip` | `src` |
| `destinationHostName` | `agent_name` | `dhost` |
| `destinationProcessId` | `agent_id` | `dpid` |
| `sourceUserName` | `user` | `suser` |
| `deviceCustomString1` | `correlation_id` | `cs1` |
| `deviceCustomString1Label` | `Correlation ID` | `cs1Label` |
| `deviceCustomString2` | `trace_id` | `cs2` |
| `deviceCustomString2Label` | `Trace ID` | `cs2Label` |
| `deviceCustomString3` | `command_name` | `cs3` |
| `deviceCustomString3Label` | `Command` | `cs3Label` |
| `deviceCustomNumber1` | `duration_ms` | `cn1` |
| `deviceCustomNumber1Label` | `Duration (ms)` | `cn1Label` |
| `deviceCustomNumber2` | `exit_code` | `cn2` |
| `deviceCustomNumber2Label` | `Exit Code` | `cn2Label` |
| `message` | `error` | `msg` |

**CEF Output Configuration**:

```yaml
logging:
  syslog:
    enabled: true
    transport: tcp+tls
    address: "siem.example.com:6514"
    format: cef
    cef:
      device_vendor: "Anthropic"
      device_product: "Keystone"
```

**Sample CEF Output**:

```
CEF:0|Anthropic|Keystone|1.0|CMD_EXEC|Command Executed|3|src=10.0.0.1 dhost=agent-01 suser=admin cs1=job-abc123 cs1Label=Correlation ID cs3=apt-get cs3Label=Command cn1=1234 cn1Label=Duration (ms) cn2=0 cn2Label=Exit Code
```

#### Log Event Extended Format (LEEF)

LEEF mapping for IBM QRadar:

**LEEF Header**:

```
LEEF:2.0|Anthropic|Keystone|1.0|<event_id>|
```

**LEEF Field Mappings**:

| LEEF Field | Keystone Field | Notes |
|------------|----------------|-------|
| `devTime` | `timestamp` | ISO 8601 format |
| `devTimeFormat` | `yyyy-MM-dd'T'HH:mm:ss.SSSZ` | Time format |
| `src` | `source_ip` | Source IP |
| `dst` | `agent_ip` | Destination IP |
| `dstHost` | `agent_name` | Agent hostname |
| `usrName` | `user` | Username |
| `cat` | `logger` | Event category |
| `sev` | `level` | Severity (1-10) |
| `identSrc` | `agent_id` | Agent UUID |
| `msg` | `message` | Event message |
| `resource` | `resource_type` | Resource type |
| `action` | `command_name` | Action performed |
| `responseTime` | `duration_ms` | Response time |
| `outcome` | `exit_code` | Result code |

**Sample LEEF Output**:

```
LEEF:2.0|Anthropic|Keystone|1.0|CMD_EXEC|devTime=2024-01-15T10:30:45.123Z cat=command-dispatcher sev=3 src=10.0.0.1 dstHost=agent-01 usrName=admin action=apt-get responseTime=1234 outcome=0 msg=Command executed successfully
```

#### Microsoft Sentinel (Azure)

Data connector configuration for Microsoft Sentinel:

**Log Analytics Workspace Schema**:

```kusto
// Custom log table: Keystone_CL
Keystone_CL
| project
    TimeGenerated,
    Level_s as Level,
    Logger_s as Logger,
    Message_s as Message,
    CorrelationId_g as CorrelationId,
    TraceId_s as TraceId,
    AgentId_g as AgentId,
    AgentName_s as AgentName,
    SourceIP_s as SourceIP,
    User_s as User,
    CommandName_s as CommandName,
    DurationMs_d as DurationMs,
    ExitCode_d as ExitCode
```

**Azure Monitor Agent Configuration**:

```json
{
  "logs": [
    {
      "streams": ["Custom-Keystone_CL"],
      "filePaths": ["/var/log/keystone-core/*.log"],
      "format": "json",
      "settings": {
        "text": {
          "recordStartTimestampFormat": "ISO 8601"
        }
      }
    }
  ]
}
```

**KQL Queries for Threat Hunting**:

```kusto
// Failed authentication attempts
Keystone_CL
| where Logger_s == "auth" and Level_s == "error"
| summarize FailedAttempts=count() by SourceIP_s, bin(TimeGenerated, 1h)
| where FailedAttempts > 10

// Unusual command execution patterns
Keystone_CL
| where Logger_s == "command-dispatcher"
| summarize CommandCount=count() by AgentName_s, CommandName_s, bin(TimeGenerated, 1h)
| join kind=inner (
    Keystone_CL
    | where Logger_s == "command-dispatcher"
    | summarize AvgCount=avg(CommandCount) by AgentName_s, CommandName_s
) on AgentName_s, CommandName_s
| where CommandCount > AvgCount * 3

// Policy violations
Keystone_CL
| where Logger_s == "policy-engine" and Message_s contains "denied"
| project TimeGenerated, AgentName_s, User_s, Message_s
| order by TimeGenerated desc
```

#### Field Normalization Reference

Quick reference for field normalization across all SIEM formats:

| Keystone | Splunk CIM | ECS | CEF | LEEF | Sentinel |
|----------|------------|-----|-----|------|----------|
| `timestamp` | `_time` | `@timestamp` | (header) | `devTime` | `TimeGenerated` |
| `level` | `severity` | `log.level` | Severity | `sev` | `Level_s` |
| `logger` | `app` | `event.dataset` | (extension) | `cat` | `Logger_s` |
| `message` | `action` | `message` | Name | `msg` | `Message_s` |
| `source_ip` | `src` | `source.ip` | `src` | `src` | `SourceIP_s` |
| `agent_name` | `dest` | `host.hostname` | `dhost` | `dstHost` | `AgentName_s` |
| `agent_id` | `dest_id` | `host.id` | `dpid` | `identSrc` | `AgentId_g` |
| `user` | `user` | `user.name` | `suser` | `usrName` | `User_s` |
| `command_name` | `command` | `process.name` | `cs3` | `action` | `CommandName_s` |
| `duration_ms` | `duration` | `event.duration` | `cn1` | `responseTime` | `DurationMs_d` |
| `exit_code` | `result` | `process.exit_code` | `cn2` | `outcome` | `ExitCode_d` |
| `correlation_id` | `correlation_id` | `transaction.id` | `cs1` | (custom) | `CorrelationId_g` |
| `trace_id` | `trace_id` | `trace.id` | `cs2` | (custom) | `TraceId_s` |

#### Parser Configuration Templates

**Splunk Add-on**:

```
# inputs.conf
[monitor:///var/log/keystone-core]
sourcetype = kscore:json
index = main

# transforms.conf
[kscore_severity]
REGEX = "level":"(\w+)"
FORMAT = severity::$1
WRITE_META = true
```

**Elastic Ingest Pipeline**:

```json
{
  "description": "Keystone log parser",
  "processors": [
    {
      "json": {
        "field": "message",
        "target_field": "kscore"
      }
    },
    {
      "rename": {
        "field": "kscore.timestamp",
        "target_field": "@timestamp"
      }
    },
    {
      "rename": {
        "field": "kscore.level",
        "target_field": "log.level"
      }
    },
    {
      "set": {
        "field": "event.module",
        "value": "kscore"
      }
    }
  ]
}
```

**Fluentd Configuration**:

```
<source>
  @type tail
  path /var/log/keystone-core/*.log
  pos_file /var/log/td-agent/kscore.pos
  tag kscore.logs
  <parse>
    @type json
    time_key timestamp
    time_format %Y-%m-%dT%H:%M:%S.%N%z
  </parse>
</source>

<filter kscore.logs>
  @type record_transformer
  enable_ruby true
  <record>
    # ECS field mapping
    event.module kscore
    event.dataset ${record["logger"]}
    host.hostname ${record["agent_name"]}
    user.name ${record["user"]}
    source.ip ${record["source_ip"]}
  </record>
</filter>
```

**Vector Configuration**:

```toml
[sources.kscore_logs]
type = "file"
include = ["/var/log/keystone-core/*.log"]

[transforms.kscore_parse]
type = "remap"
inputs = ["kscore_logs"]
source = '''
. = parse_json!(.message)
.event.module = "kscore"
.event.dataset = .logger
.host.hostname = .agent_name
.user.name = .user
.source.ip = .source_ip
del(.logger)
del(.agent_name)
del(.user)
del(.source_ip)
'''

[sinks.elasticsearch]
type = "elasticsearch"
inputs = ["kscore_parse"]
endpoints = ["https://elasticsearch:9200"]
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

### Programmatic Querying

Keystone Core provides Go clients for querying Loki and Jaeger programmatically:

**Loki Client**:

```go
import "github.com/shawnbutts/keystone-core/pkg/query"

// Simple client
client := query.NewLokiQuerier("http://loki:3100")

// With full configuration
client := query.NewLokiQuerierWithConfig(&query.LokiConfig{
    Address:  "http://loki:3100",
    Username: "admin",          // Optional: basic auth
    Password: "secret",
    TenantID: "my-org",         // Optional: multi-tenant
    Timeout:  30 * time.Second,
})

// Query logs
result, err := client.Query(ctx, &query.LogsQuery{
    Query: `{service="kscore-server"} |= "error"`,
    Range: query.TimeRange{
        Start: time.Now().Add(-1 * time.Hour),
        End:   time.Now(),
    },
    Limit:     100,
    Direction: "backward",  // Most recent first
})

// Get available labels
labels, err := client.Labels(ctx, start, end)

// Get values for a label
values, err := client.LabelValues(ctx, "service", start, end)
```

**Jaeger Client**:

```go
import "github.com/shawnbutts/keystone-core/pkg/query"

// Simple client
client := query.NewJaegerQuerier("http://jaeger-query:16686")

// With full configuration
client := query.NewJaegerQuerierWithConfig(&query.JaegerConfig{
    Address:  "http://jaeger-query:16686",
    Username: "admin",          // Optional: basic auth
    Password: "secret",
    Timeout:  30 * time.Second,
})

// Query traces
result, err := client.Query(ctx, &query.TracesQuery{
    Service:   "kscore-server",
    Operation: "Execute Command",
    Tags: map[string]string{
        "agent_id": "web-01",
    },
    Range: &query.TimeRange{
        Start: time.Now().Add(-1 * time.Hour),
        End:   time.Now(),
    },
    MinDuration: 100 * time.Millisecond,
    Limit:       20,
})

// Get a specific trace
trace, err := client.GetTrace(ctx, "abc123def456")

// Get available services
services, err := client.GetServices(ctx)

// Get operations for a service
operations, err := client.GetOperations(ctx, "kscore-server")
```

**Query Results**:

```go
// Loki results
for _, entry := range result.Entries {
    fmt.Printf("[%s] %s\n", entry.Timestamp, entry.Line)
    for k, v := range entry.Labels {
        fmt.Printf("  %s=%s\n", k, v)
    }
}

// Jaeger results
for _, trace := range result.Traces {
    fmt.Printf("Trace: %s (%d spans)\n", trace.TraceID, len(trace.Spans))
    for _, span := range trace.Spans {
        fmt.Printf("  %s: %s (%v)\n", span.SpanID, span.OperationName, span.Duration)
    }
}
```

## Grafana Dashboards

Keystone Core provides 10 pre-built Grafana dashboards:

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

- Real-time log streaming via NATS
- Color-coded by level (debug, info, warn, error)
- Pause/resume with `p` or `space`
- Clear logs with `c`
- Live/Paused status indicator

**8. Metrics**:

- Real-time metrics via NATS
- Grouped by service
- Color-coded by type (counter, gauge, histogram)
- Clear metrics with `c`

### NATS Telemetry Integration

The TUI monitor connects to NATS for real-time telemetry streaming:

```bash
# Start monitor with NATS URL
kscorectl monitor --nats-url nats://localhost:4222

# Or configure in config file
monitor:
  nats_url: "nats://localhost:4222"
  log_subject: "kscore.logs.>"
  metric_subject: "kscore.metrics.>"
  trace_subject: "kscore.traces.>"
  audit_subject: "kscore.audit.>"
```

**Subscribed subjects**:

- `kscore.logs.>` - Log messages
- `kscore.metrics.>` - Metric updates
- `kscore.traces.>` - Trace spans
- `kscore.audit.>` - Audit entries

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

## NATS Telemetry Transport

Keystone Core supports NATS as a unified transport for all telemetry data (logs, metrics, traces, audit). This enables centralized collection without requiring separate backends for each telemetry type.

### NATS Metrics Transport

Send metrics to a centralized collector via NATS:

```yaml
metrics:
  nats:
    enabled: true
    url: "nats://localhost:4222"
    subject: "kscore.metrics"
    service_name: "kscore-server"
    batch_size: 100        # Metrics per batch
    flush_interval: 10s    # Batch flush interval

    # Authentication
    token: ""
    user: ""
    password: ""
    nkey_file: ""
    cred_file: ""
```

**Message Format** (JSON batch):

```json
{
  "timestamp": "2024-01-15T10:30:45.123Z",
  "service": "kscore-server",
  "host": "server-01",
  "metrics": [
    {
      "name": "api_requests_total",
      "type": "counter",
      "value": 12345,
      "labels": {"method": "POST", "path": "/api/exec"}
    },
    {
      "name": "api_request_duration_seconds",
      "type": "histogram",
      "value": 0.025,
      "labels": {"method": "POST"}
    }
  ]
}
```

**Subscribe to metrics**:

```bash
# All metrics
nats sub "kscore.metrics.>"

# Metrics from specific service
nats sub "kscore.metrics.kscore-server"
```

### NATS Trace Transport

Send trace spans to a centralized collector via NATS:

```yaml
tracing:
  nats:
    enabled: true
    url: "nats://localhost:4222"
    subject: "kscore.traces"
    service_name: "kscore-server"
    batch_size: 100        # Spans per batch
    flush_interval: 5s     # Batch flush interval
    buffer_size: 10000     # Span buffer size
```

**Span Format** (JSON):

```json
{
  "trace_id": "1234567890abcdef",
  "span_id": "abc123def456",
  "parent_span_id": "def456abc123",
  "name": "Execute Command",
  "kind": "server",
  "start_time": "2024-01-15T10:30:45.000Z",
  "end_time": "2024-01-15T10:30:46.234Z",
  "duration_ns": 1234000000,
  "status": "ok",
  "attributes": {
    "agent_id": "web-01",
    "command": "systemctl restart nginx"
  },
  "events": [
    {"name": "command.started", "timestamp": "..."}
  ]
}
```

**Batch Format**:

```json
{
  "timestamp": "2024-01-15T10:30:45Z",
  "service": "kscore-server",
  "host": "server-01",
  "spans": [/* array of spans */]
}
```

## Audit Logging

Keystone Core provides comprehensive audit logging for security and compliance:

### Audit Entry Format

```json
{
  "timestamp": "2024-01-15T10:30:45.123Z",
  "user": "admin",
  "uid": 1000,
  "tty": "pts/0",
  "pid": 12345,
  "tool": "kscore-exec",
  "command": "run",
  "args": ["--target", "role:web", "systemctl restart nginx"],
  "target": "role:web",
  "action": "command_executed",
  "result": "success",
  "exit_code": 0,
  "duration_ms": 1234,
  "correlation_id": "job-abc123",
  "extra": {
    "agents_matched": 50,
    "agents_succeeded": 50
  }
}
```

### Audit Actions

- `command_executed`: Remote command execution
- `state_applied`: State file application
- `state_checked`: State drift check
- `policy_evaluated`: Policy evaluation
- `agent_connected`: Agent registration
- `agent_disconnected`: Agent disconnection
- `config_changed`: Configuration modification
- `secret_accessed`: Secret retrieval
- `job_created`: Batch job creation
- `job_completed`: Batch job completion
- `webhook_received`: GitOps webhook received
- `rollback_triggered`: Rollback initiated
- `promotion_requested`: Environment promotion
- `plugin_loaded`: Module loaded

### Audit Configuration

```yaml
audit:
  level: all             # all, errors, none
  output: auto           # auto, syslog, journald, stderr, file, nats

  # Syslog backend (Linux default)
  syslog:
    facility: auth       # auth, local0-7

  # Journald backend (systemd)
  journald:
    identifier: kscore-audit

  # File backend
  file:
    path: /var/log/keystone-core/audit.log
    max_size: 100MB
    max_backups: 10

  # NATS backend
  nats:
    url: "nats://localhost:4222"
    subject: "kscore.audit"
    buffer_size: 10000
```

### NATS Audit Transport

Send audit logs to a centralized collector via NATS:

```yaml
audit:
  nats:
    enabled: true
    url: "nats://localhost:4222"
    subject: "kscore.audit"
    subject_per_tool: true   # Use kscore.audit.{tool}
    buffer_size: 10000
    flush_interval: 1s
```

**Subject Routing**:

- Default: `kscore.audit`
- Per-tool: `kscore.audit.kscore-exec`
- Per-action: `kscore.audit.{tool}.{action}`

**Subscribe to audit logs**:

```bash
# All audit logs
nats sub "kscore.audit.>"

# Audit logs from kscore-exec
nats sub "kscore.audit.kscore-exec.>"

# Failed operations
nats sub "kscore.audit.>" | jq 'select(.result == "failure")'
```

### CLI Audit Integration

Audit logging is integrated into all CLI tools:

```bash
# kscore-exec logs command executions
kscorectl exec run --target "role:web" -- systemctl restart nginx
# → Audit: command_executed, result=success, agents=50

# kscore-state logs state applications
kscorectl state apply webserver.yaml
# → Audit: state_applied, result=success, changes=3

# kscore-state logs drift checks
kscorectl state drift webserver.yaml
# → Audit: state_checked, result=success, drift=2
```

**Override audit settings**:

```bash
# Disable audit logging
kscorectl exec --audit-level none run "role:web" -- echo test

# Log to stderr
kscorectl exec --audit-output stderr run "role:web" -- echo test
```

### Sensitive Data Redaction

Audit logs automatically redact sensitive data:

```bash
# Input
kscorectl exec run "role:db" -- mysql -p secret123 -e 'SELECT *'

# Audit entry (redacted)
{
  "args": ["mysql", "-p", "[REDACTED]", "-e", "SELECT *"],
  ...
}
```

**Redacted patterns**:

- Passwords (`-p`, `--password`)
- Tokens (`--token`, `-t`)
- Secrets (`--secret`)
- Keys (`--key`, `--api-key`)
- Credentials (`--creds`, `--credentials`)

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
