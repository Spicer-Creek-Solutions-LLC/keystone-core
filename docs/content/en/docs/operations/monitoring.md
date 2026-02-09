---
title: "Monitoring Guide"
weight: 2
description: >
  Complete observability setup with Prometheus, Grafana, logging, and alerting
---

## Overview

Keystone Core provides comprehensive observability through Prometheus metrics, structured logging, distributed tracing, and pre-built Grafana dashboards. This guide covers the complete monitoring stack setup for production deployments.

**Monitoring Stack:**

- **Metrics**: Prometheus + Grafana (70+ metrics exposed)
- **Logging**: Structured JSON logs + Loki/Elasticsearch
- **Tracing**: OpenTelemetry + Jaeger (distributed tracing)
- **Alerting**: Alertmanager + PagerDuty/Slack integration

## Prometheus Integration

Keystone Core exposes 70+ Prometheus metrics on `/metrics` endpoint.

### Installation

**Install Prometheus:**

```bash
wget https://github.com/prometheus/prometheus/releases/download/v2.45.0/prometheus-2.45.0.linux-amd64.tar.gz
tar xvf prometheus-2.45.0.linux-amd64.tar.gz
sudo mv prometheus-2.45.0.linux-amd64/prometheus /usr/local/bin/
sudo mv prometheus-2.45.0.linux-amd64/promtool /usr/local/bin/
```

**Configuration (prometheus.yml):**

```yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  # Keystone Core Control Plane
  - job_name: 'kscore-server'
    static_configs:
      - targets:
          - 'server1:8080'
          - 'server2:8080'
          - 'server3:8080'
    metrics_path: '/metrics'
    scrape_interval: 10s

  # Keystone Core Agents
  - job_name: 'kscore-agents'
    relabel_configs:
      - source_labels: [__meta_kubernetes_pod_label_app]
        regex: kscore-agent
        action: keep
      - source_labels: [__meta_kubernetes_pod_name]
        target_label: agent_id
    kubernetes_sd_configs:
      - role: pod

  # NATS Server
  - job_name: 'nats'
    static_configs:
      - targets:
          - 'nats1:7777'
          - 'nats2:7777'
          - 'nats3:7777'

  # PostgreSQL
  - job_name: 'postgres'
    static_configs:
      - targets: ['postgres-exporter:9187']
```

**Systemd Service:**

```ini
# /etc/systemd/system/prometheus.service
[Unit]
Description=Prometheus
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/prometheus \
  --config.file=/etc/prometheus/prometheus.yml \
  --storage.tsdb.path=/var/lib/prometheus \
  --storage.tsdb.retention.time=30d \
  --web.enable-lifecycle

Restart=on-failure

[Install]
WantedBy=multi-user.target
```

**Start Prometheus:**

```bash
sudo systemctl enable prometheus
sudo systemctl start prometheus

# Verify targets
curl http://localhost:9090/api/v1/targets
```

### Key Metrics

**Control Plane Metrics:**

```promql
# API request rate
rate(kscore_api_requests_total[5m])

# API latency (P95)
histogram_quantile(0.95, rate(kscore_api_request_duration_seconds_bucket[5m]))

# Connected agents
kscore_agents_connected

# Command execution rate
rate(kscore_command_executions_total[5m])

# State application success rate
rate(kscore_states_applied_total{result="success"}[5m]) /
rate(kscore_states_applied_total[5m])
```

**Agent Metrics:**

```promql
# Agent heartbeat failures
kscore_agent_heartbeat_failures_total

# Agent CPU usage
kscore_agent_cpu_usage_percent

# Agent memory usage
kscore_agent_memory_usage_bytes / kscore_agent_memory_total_bytes

# Agent disk usage
kscore_agent_disk_usage_bytes / kscore_agent_disk_total_bytes
```

**Event System Metrics:**

```promql
# Event publish rate
rate(kscore_events_published_total[5m])

# Reactor execution rate
rate(kscore_reactor_executions_total[5m])

# Reactor failures
rate(kscore_reactor_failures_total[5m])
```

**Policy Metrics:**

```promql
# Policy evaluations
rate(kscore_policy_evaluations_total[5m])

# Policy violations by severity
sum by (severity) (kscore_policy_violations_total)

# Compliance score
kscore_policy_compliance_score
```

### Retention and Storage

**Storage Configuration:**

```yaml
# prometheus.yml
storage:
  tsdb:
    path: /var/lib/prometheus
    retention.time: 30d  # Keep data for 30 days
    retention.size: 100GB  # Or until 100GB limit
```

**Disk Usage Estimate:**

- ~1KB per sample
- 70 metrics × 1,000 agents × 4 samples/min = 280,000 samples/min
- Daily: ~400MB/day
- Monthly (30d): ~12GB/month

## Grafana Dashboards

Keystone Core provides 10 pre-built Grafana dashboards.

### Installation

**Install Grafana:**

```bash
wget -q -O - https://packages.grafana.com/gpg.key | sudo apt-key add -
echo "deb https://packages.grafana.com/oss/deb stable main" | sudo tee /etc/apt/sources.list.d/grafana.list
sudo apt-get update
sudo apt-get install grafana
```

**Configure Datasource:**

```yaml
# /etc/grafana/provisioning/datasources/prometheus.yml
apiVersion: 1

datasources:
  - name: Prometheus
    type: prometheus
    access: proxy
    url: http://localhost:9090
    isDefault: true
    editable: false
```

**Import Dashboards:**

```bash
# Download dashboards
wget https://github.com/shawnbutts/keystone-core/raw/main/deploy/grafana/dashboards/*.json \
  -P /etc/grafana/provisioning/dashboards/

# Configure dashboard provisioning
cat > /etc/grafana/provisioning/dashboards/kscore.yml <<EOF
apiVersion: 1
providers:
  - name: 'Keystone Core'
    folder: 'Keystone Core'
    type: file
    options:
      path: /etc/grafana/provisioning/dashboards
EOF
```

**Start Grafana:**

```bash
sudo systemctl enable grafana-server
sudo systemctl start grafana-server

# Access: http://localhost:3000
# Default: admin/admin
```

### Dashboard Overview

**1. Keystone Core Overview**

- System health status
- Agent counts (total, healthy, degraded, offline)
- Command execution rates
- State application statistics
- Policy compliance score
- Recent events timeline

**2. Control Plane Health**

- Control plane uptime
- API request rate and latency
- NATS message throughput
- Database query latency
- CPU and memory usage
- Error rates by component

**3. Agent Fleet**

- Agent distribution by datacenter/role
- Agent resource utilization (CPU, memory, disk)
- Agent version distribution
- Command execution success rate per agent
- Agent connectivity status

**4. State Management**

- State applications (success/failure rates)
- Drift detection events by severity
- State changes by module
- Application duration percentiles
- Resources under management

**5. Policy Compliance**

- Overall compliance score
- Violations by severity
- Remediation success rate
- Top violated policies
- Compliance trends (7-day average)

**6. GitOps Operations**

- Deployment verification metrics
- Verification success rate
- Rollback frequency and reasons
- Webhook events by source
- Failed verifications by application

### Custom Dashboards

**Create New Dashboard:**

1. Go to Grafana → Dashboards → New Dashboard
2. Add panel
3. Select Prometheus datasource
4. Enter PromQL query
5. Configure visualization
6. Save dashboard

**Example Panel (API Latency):**

```json
{
  "title": "API Request Latency (P95)",
  "targets": [{
    "expr": "histogram_quantile(0.95, rate(kscore_api_request_duration_seconds_bucket[5m]))",
    "legendFormat": "{{ method }} {{ path }}"
  }],
  "yaxes": [{
    "format": "s",
    "label": "Latency"
  }]
}
```

## Log Aggregation

Keystone Core emits structured JSON logs for centralized aggregation.

### Loki Setup (Recommended)

**Install Loki:**

```bash
wget https://github.com/grafana/loki/releases/download/v2.8.0/loki-linux-amd64.zip
unzip loki-linux-amd64.zip
sudo mv loki-linux-amd64 /usr/local/bin/loki
```

**Configuration (loki-config.yml):**

```yaml
auth_enabled: false

server:
  http_listen_port: 3100

ingester:
  lifecycler:
    ring:
      kvstore:
        store: inmemory
      replication_factor: 1
  chunk_idle_period: 5m
  chunk_retain_period: 30s

schema_config:
  configs:
    - from: 2023-01-01
      store: boltdb-shipper
      object_store: filesystem
      schema: v11
      index:
        prefix: index_
        period: 24h

storage_config:
  boltdb_shipper:
    active_index_directory: /var/lib/loki/index
    cache_location: /var/lib/loki/cache
    shared_store: filesystem
  filesystem:
    directory: /var/lib/loki/chunks

limits_config:
  enforce_metric_name: false
  reject_old_samples: true
  reject_old_samples_max_age: 168h

chunk_store_config:
  max_look_back_period: 0s

table_manager:
  retention_deletes_enabled: true
  retention_period: 720h  # 30 days
```

**Start Loki:**

```bash
loki -config.file=loki-config.yml
```

**Install Promtail (Log Shipper):**

```bash
wget https://github.com/grafana/loki/releases/download/v2.8.0/promtail-linux-amd64.zip
unzip promtail-linux-amd64.zip
sudo mv promtail-linux-amd64 /usr/local/bin/promtail
```

**Promtail Configuration:**

```yaml
server:
  http_listen_port: 9080
  grpc_listen_port: 0

positions:
  filename: /tmp/positions.yaml

clients:
  - url: http://localhost:3100/loki/api/v1/push

scrape_configs:
  - job_name: kscore-server
    static_configs:
      - targets:
          - localhost
        labels:
          job: kscore-server
          __path__: /var/log/keystone-core/server.log
    pipeline_stages:
      - json:
          expressions:
            level: level
            logger: logger
            message: message
            correlation_id: correlation_id
      - labels:
          level:
          logger:
      - timestamp:
          source: timestamp
          format: RFC3339

  - job_name: kscore-agents
    static_configs:
      - targets:
          - localhost
        labels:
          job: kscore-agents
          __path__: /var/log/keystone-core/agent-*.log
```

**Add Loki to Grafana:**

```yaml
# /etc/grafana/provisioning/datasources/loki.yml
apiVersion: 1

datasources:
  - name: Loki
    type: loki
    access: proxy
    url: http://localhost:3100
    editable: false
```

### Elasticsearch Setup (Alternative)

**Install Elasticsearch:**

```bash
# Add Elastic repository
wget -qO - https://artifacts.elastic.co/GPG-KEY-elasticsearch | sudo apt-key add -
echo "deb https://artifacts.elastic.co/packages/8.x/apt stable main" | sudo tee /etc/apt/sources.list.d/elastic-8.x.list
sudo apt-get update
sudo apt-get install elasticsearch
```

**Install Filebeat:**

```bash
sudo apt-get install filebeat
```

**Filebeat Configuration:**

```yaml
filebeat.inputs:
  - type: log
    enabled: true
    paths:
      - /var/log/keystone-core/*.log
    json.keys_under_root: true
    json.add_error_key: true

output.elasticsearch:
  hosts: ["localhost:9200"]
  index: "kscore-%{+yyyy.MM.dd}"

setup.template.name: "kscore"
setup.template.pattern: "kscore-*"
```

### Log Queries

**Loki Queries (LogQL):**

```logql
# All error logs
{job="kscore-server"} |= "error"

# Logs with correlation ID
{job="kscore-server"} | json | correlation_id="abc-123"

# API request logs (HTTP 500)
{job="kscore-server"} | json | status_code="500"

# Agent connection failures
{job="kscore-server"} | json | message=~"agent.*connection.*failed"
```

**Elasticsearch Queries:**

```json
{
  "query": {
    "bool": {
      "must": [
        { "match": { "level": "error" }},
        { "range": { "@timestamp": { "gte": "now-1h" }}}
      ]
    }
  }
}
```

## Alerting

Configure Prometheus Alertmanager for production alerting.

### Alertmanager Installation

**Install:**

```bash
wget https://github.com/prometheus/alertmanager/releases/download/v0.26.0/alertmanager-0.26.0.linux-amd64.tar.gz
tar xvf alertmanager-0.26.0.linux-amd64.tar.gz
sudo mv alertmanager-0.26.0.linux-amd64/alertmanager /usr/local/bin/
```

**Configuration (alertmanager.yml):**

```yaml
global:
  resolve_timeout: 5m

route:
  group_by: ['alertname', 'cluster', 'service']
  group_wait: 10s
  group_interval: 10s
  repeat_interval: 12h
  receiver: 'default'
  routes:
    - match:
        severity: critical
      receiver: pagerduty
      continue: true
    - match:
        severity: warning
      receiver: slack

receivers:
  - name: 'default'
    email_configs:
      - to: 'ops@example.com'
        from: 'alertmanager@example.com'
        smarthost: 'smtp.example.com:587'
        auth_username: 'alertmanager'
        auth_password: '$SMTP_PASSWORD'

  - name: 'pagerduty'
    pagerduty_configs:
      - service_key: '$PAGERDUTY_KEY'
        description: '{{ .GroupLabels.alertname }}'

  - name: 'slack'
    slack_configs:
      - api_url: '$SLACK_WEBHOOK_URL'
        channel: '#alerts'
        title: 'Keystone Core Alert'
        text: '{{ range .Alerts }}{{ .Annotations.description }}{{ end }}'
```

### Alert Rules

**Create Alert Rules (alerts.yml):**

```yaml
groups:
  - name: kscore-control-plane
    interval: 30s
    rules:
      - alert: ControlPlaneDown
        expr: up{job="kscore-server"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Control plane {{ $labels.instance }} is down"
          description: "Control plane has been down for more than 1 minute"

      - alert: HighAPILatency
        expr: histogram_quantile(0.95, rate(kscore_api_request_duration_seconds_bucket[5m])) > 1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High API latency on {{ $labels.instance }}"
          description: "P95 latency is {{ $value }}s (threshold: 1s)"

      - alert: HighErrorRate
        expr: rate(kscore_api_requests_total{status=~"5.."}[5m]) > 0.05
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High error rate on {{ $labels.instance }}"
          description: "Error rate is {{ $value | humanizePercentage }}"

  - name: kscore-agents
    interval: 30s
    rules:
      - alert: AgentFleetLowAvailability
        expr: (kscore_agents_connected / kscore_agents_total) < 0.9
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Agent fleet availability is low"
          description: "Only {{ $value | humanizePercentage }} agents connected"

      - alert: AgentFleetCriticalAvailability
        expr: (kscore_agents_connected / kscore_agents_total) < 0.7
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Agent fleet availability is critical"
          description: "Only {{ $value | humanizePercentage }} agents connected"

      - alert: AgentHighCPU
        expr: kscore_agent_cpu_usage_percent > 90
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Agent {{ $labels.agent_id }} high CPU usage"
          description: "CPU usage is {{ $value }}%"

  - name: kscore-state
    interval: 30s
    rules:
      - alert: StateApplicationHighFailureRate
        expr: rate(kscore_states_applied_total{result="failed"}[5m]) / rate(kscore_states_applied_total[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High state application failure rate"
          description: "{{ $value | humanizePercentage }} of state applications are failing"

      - alert: DriftDetectionCritical
        expr: sum(kscore_drift_detected_total{severity="critical"}) > 10
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Critical configuration drift detected"
          description: "{{ $value }} critical drift events detected"

  - name: kscore-policy
    interval: 30s
    rules:
      - alert: PolicyViolationsCritical
        expr: sum(rate(kscore_policy_violations_total{severity="critical"}[5m])) > 1
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Critical policy violations detected"
          description: "{{ $value }} critical policy violations per second"

      - alert: ComplianceScoreLow
        expr: kscore_policy_compliance_score < 80
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Compliance score is low"
          description: "Compliance score is {{ $value }}% (threshold: 80%)"

  - name: kscore-infrastructure
    interval: 30s
    rules:
      # Resource utilization alerts
      - alert: ControlPlaneHighCPU
        expr: rate(process_cpu_seconds_total{job="kscore-server"}[5m]) * 100 > 80
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Control plane {{ $labels.instance }} high CPU usage"
          description: "CPU usage is {{ $value | printf \"%.1f\" }}% for over 10 minutes"
          runbook_url: "https://docs.example.com/runbooks/high-cpu"

      - alert: ControlPlaneHighMemory
        expr: process_resident_memory_bytes{job="kscore-server"} / node_memory_MemTotal_bytes > 0.85
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Control plane {{ $labels.instance }} high memory usage"
          description: "Memory usage is {{ $value | humanizePercentage }}"
          runbook_url: "https://docs.example.com/runbooks/high-memory"

      - alert: ControlPlaneDiskSpaceLow
        expr: (node_filesystem_avail_bytes{mountpoint="/var/lib/keystone-core"} / node_filesystem_size_bytes{mountpoint="/var/lib/keystone-core"}) < 0.15
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Control plane disk space low on {{ $labels.instance }}"
          description: "Only {{ $value | humanizePercentage }} disk space remaining"

      - alert: ControlPlaneDiskSpaceCritical
        expr: (node_filesystem_avail_bytes{mountpoint="/var/lib/keystone-core"} / node_filesystem_size_bytes{mountpoint="/var/lib/keystone-core"}) < 0.05
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Control plane disk space critical on {{ $labels.instance }}"
          description: "Only {{ $value | humanizePercentage }} disk space remaining"

  - name: kscore-nats
    interval: 30s
    rules:
      - alert: NATSDown
        expr: up{job="nats"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "NATS server {{ $labels.instance }} is down"
          description: "NATS has been unreachable for more than 1 minute"

      - alert: NATSNoLeader
        expr: nats_jetstream_meta_leader == 0
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "NATS JetStream has no leader"
          description: "JetStream cluster has no elected leader"

      - alert: NATSHighPendingMessages
        expr: nats_jetstream_consumer_pending > 10000
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High pending messages in NATS consumer {{ $labels.consumer }}"
          description: "{{ $value }} messages pending (threshold: 10000)"

      - alert: NATSSlowConsumer
        expr: rate(nats_server_slow_consumers_total[5m]) > 0
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "NATS slow consumers detected on {{ $labels.instance }}"
          description: "Slow consumer events occurring at {{ $value }}/sec"

      - alert: NATSConnectionsHigh
        expr: nats_server_connections > 5000
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High NATS connection count on {{ $labels.instance }}"
          description: "{{ $value }} connections (threshold: 5000)"

  - name: kscore-database
    interval: 30s
    rules:
      - alert: DatabaseDown
        expr: up{job="postgres"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Database is down"
          description: "PostgreSQL has been unreachable for more than 1 minute"

      - alert: DatabaseHighConnections
        expr: pg_stat_activity_count / pg_settings_max_connections > 0.8
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Database connection pool nearly exhausted"
          description: "{{ $value | humanizePercentage }} of max connections in use"

      - alert: DatabaseSlowQueries
        expr: rate(pg_stat_statements_seconds_total[5m]) / rate(pg_stat_statements_calls_total[5m]) > 1
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Database experiencing slow queries"
          description: "Average query time is {{ $value | humanizeDuration }}"

      - alert: DatabaseReplicationLag
        expr: pg_replication_lag > 30
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Database replication lag high"
          description: "Replication lag is {{ $value }}s (threshold: 30s)"

  - name: kscore-gitops
    interval: 30s
    rules:
      - alert: GitOpsVerificationFailed
        expr: rate(kscore_gitops_verification_failures_total[10m]) > 0
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "GitOps verification failures detected"
          description: "{{ $value }} verification failures per second for application {{ $labels.application }}"

      - alert: GitOpsRollbackTriggered
        expr: increase(kscore_gitops_rollbacks_total[1h]) > 2
        for: 1m
        labels:
          severity: warning
        annotations:
          summary: "Multiple GitOps rollbacks in last hour"
          description: "{{ $value }} rollbacks triggered for {{ $labels.application }}"

      - alert: GitOpsSyncFailed
        expr: kscore_gitops_sync_status == 0
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "GitOps sync failed for {{ $labels.application }}"
          description: "Application has been out of sync for over 10 minutes"

  - name: kscore-events
    interval: 30s
    rules:
      - alert: EventQueueBacklog
        expr: kscore_event_queue_depth > 1000
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Event queue backlog building up"
          description: "{{ $value }} events in queue (threshold: 1000)"

      - alert: EventProcessingErrors
        expr: rate(kscore_event_processing_errors_total[5m]) > 1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Event processing errors detected"
          description: "{{ $value }} errors per second"

      - alert: ReactorExecutionFailures
        expr: rate(kscore_reactor_failures_total[5m]) / rate(kscore_reactor_executions_total[5m]) > 0.1
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "High reactor failure rate"
          description: "{{ $value | humanizePercentage }} of reactor executions failing"

  - name: kscore-certificates
    interval: 1h
    rules:
      - alert: CertificateExpiringSoon
        expr: (kscore_certificate_expiry_timestamp - time()) / 86400 < 30
        for: 1h
        labels:
          severity: warning
        annotations:
          summary: "Certificate expiring soon: {{ $labels.certificate }}"
          description: "Certificate expires in {{ $value | printf \"%.0f\" }} days"

      - alert: CertificateExpiryCritical
        expr: (kscore_certificate_expiry_timestamp - time()) / 86400 < 7
        for: 1h
        labels:
          severity: critical
        annotations:
          summary: "Certificate expiring very soon: {{ $labels.certificate }}"
          description: "Certificate expires in {{ $value | printf \"%.0f\" }} days"

      - alert: CertificateExpired
        expr: kscore_certificate_expiry_timestamp < time()
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Certificate expired: {{ $labels.certificate }}"
          description: "Certificate has already expired"

  - name: kscore-command-execution
    interval: 30s
    rules:
      - alert: CommandTimeoutRateHigh
        expr: rate(kscore_command_timeouts_total[5m]) / rate(kscore_command_executions_total[5m]) > 0.05
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "High command timeout rate"
          description: "{{ $value | humanizePercentage }} of commands timing out"

      - alert: CommandExecutionQueueHigh
        expr: kscore_command_queue_depth > 100
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Command execution queue building up"
          description: "{{ $value }} commands queued for execution"

      - alert: BatchJobStuck
        expr: (time() - kscore_batch_job_start_timestamp) > 1800 and kscore_batch_job_status == 1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Batch job {{ $labels.job_id }} appears stuck"
          description: "Job has been running for over 30 minutes"
```

**Load Alert Rules in Prometheus:**

```yaml
# prometheus.yml
rule_files:
  - 'alerts.yml'

alerting:
  alertmanagers:
    - static_configs:
        - targets: ['localhost:9093']
```

### Testing Alerts

**Test Alert Rule:**

```bash
# Check alert rule syntax
promtool check rules alerts.yml

# Query pending/firing alerts
curl http://localhost:9090/api/v1/alerts
```

**Test Alertmanager:**

```bash
# Send test alert
amtool alert add test_alert severity=critical --alertmanager.url=http://localhost:9093

# Check alert status
amtool alert --alertmanager.url=http://localhost:9093
```

## Health Checks

Keystone Core exposes health check endpoints for load balancers and orchestrators.

### Endpoints

**Liveness Probe:**

```bash
GET /health/live

# Returns 200 if process is running
# Use for Kubernetes liveness probe
```

**Readiness Probe:**

```bash
GET /health/ready

# Returns 200 if ready to serve traffic
# Returns 503 if dependencies unavailable (NATS, database)
# Use for Kubernetes readiness probe and load balancer health checks
```

**Detailed Status:**

```bash
GET /health/status

# Returns detailed component health
{
  "status": "healthy",
  "components": {
    "nats": {"status": "healthy", "latency_ms": 2},
    "database": {"status": "healthy", "latency_ms": 5},
    "agent_pool": {"status": "healthy", "agents": 150}
  },
  "uptime_seconds": 86400
}
```

### Kubernetes Health Checks

```yaml
livenessProbe:
  httpGet:
    path: /health/live
    port: 8080
  initialDelaySeconds: 30
  periodSeconds: 10
  timeoutSeconds: 5
  failureThreshold: 3

readinessProbe:
  httpGet:
    path: /health/ready
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 5
  timeoutSeconds: 3
  failureThreshold: 3
```

### Load Balancer Health Checks

**HAProxy:**

```
option httpchk GET /health/ready
http-check expect status 200
```

**Nginx:**

```nginx
upstream kscore {
    server server1:8080 max_fails=3 fail_timeout=30s;
    server server2:8080 max_fails=3 fail_timeout=30s;
}

server {
    location /health/ready {
        proxy_pass http://kscore;
    }
}
```

## Performance Monitoring

### Performance Baselines

This section documents expected performance characteristics for Keystone Core components. Use these baselines for capacity planning, performance testing, and identifying regressions.

#### Agent Registration Throughput

Agent registration throughput depends on control plane resources and network conditions.

**Baseline Measurements** (3-node cluster, 8 CPU, 16GB RAM per node):

| Metric | Small (≤100) | Medium (100-1K) | Large (1K-10K) | Enterprise (10K+) |
|--------|--------------|-----------------|----------------|-------------------|
| Peak registrations/sec | 500 | 200 | 100 | 50 |
| Sustained registrations/sec | 200 | 100 | 50 | 25 |
| P50 registration latency | 5ms | 10ms | 25ms | 50ms |
| P95 registration latency | 15ms | 30ms | 75ms | 150ms |
| P99 registration latency | 50ms | 100ms | 200ms | 400ms |

**Key Metrics**:

```promql
# Registration throughput
rate(kscore_agent_registrations_total[5m])

# Registration latency percentiles
histogram_quantile(0.50, rate(kscore_agent_registration_duration_seconds_bucket[5m]))
histogram_quantile(0.95, rate(kscore_agent_registration_duration_seconds_bucket[5m]))
histogram_quantile(0.99, rate(kscore_agent_registration_duration_seconds_bucket[5m]))

# Failed registrations
rate(kscore_agent_registration_failures_total[5m])
```

**Factors Affecting Performance**:

- Database backend (PostgreSQL faster than SQLite at scale)
- Network latency to control plane
- TLS handshake overhead
- Agent metadata size
- Concurrent registration bursts

**Optimization Tips**:

- Use PostgreSQL for >500 agents
- Enable connection pooling (pgbouncer)
- Configure agent staggered startup (avoid thundering herd)
- Pre-warm control plane before mass registration

#### Command Execution Latency

Command execution latency measures the time from command dispatch to result receipt.

**Baseline Measurements** (excluding command runtime):

| Phase | P50 | P95 | P99 | Description |
|-------|-----|-----|-----|-------------|
| Dispatch | 2ms | 10ms | 25ms | Control plane to NATS |
| Delivery | 5ms | 25ms | 50ms | NATS to agent |
| Execution overhead | 3ms | 15ms | 30ms | Agent process spawn |
| Result return | 5ms | 25ms | 50ms | Agent to control plane |
| **Total overhead** | **15ms** | **75ms** | **155ms** | Excluding command runtime |

**By Command Type**:

| Command Type | P50 Total | P95 Total | P99 Total | Notes |
|--------------|-----------|-----------|-----------|-------|
| Shell (echo) | 20ms | 100ms | 200ms | Minimal command |
| File read | 25ms | 125ms | 250ms | Small file |
| Package query | 50ms | 250ms | 500ms | Varies by OS |
| Service status | 30ms | 150ms | 300ms | systemctl/sc |
| PowerShell | 100ms | 400ms | 800ms | Includes startup |

**Key Metrics**:

```promql
# Command dispatch latency
histogram_quantile(0.95, rate(kscore_command_dispatch_duration_seconds_bucket[5m]))

# End-to-end command latency (excluding runtime)
histogram_quantile(0.95, rate(kscore_command_overhead_duration_seconds_bucket[5m]))

# Command throughput
rate(kscore_command_executions_total[5m])

# By exit code
rate(kscore_command_executions_total{exit_code="0"}[5m])  # Success
rate(kscore_command_executions_total{exit_code!="0"}[5m]) # Failure
```

**Batch Execution Scaling**:

| Batch Size | Sequential Time | Parallel Time (100 workers) | Efficiency |
|------------|-----------------|----------------------------|------------|
| 10 | 200ms | 50ms | 4x |
| 100 | 2s | 100ms | 20x |
| 1,000 | 20s | 500ms | 40x |
| 10,000 | 200s | 5s | 40x |

**Optimization Tips**:

- Use batch execution for multiple targets
- Configure appropriate timeouts (don't use excessive timeouts)
- Prefer PowerShell Core over Windows PowerShell
- Use async execution for long-running commands

#### State Application Throughput

State application performance depends on state complexity and target count.

**Baseline Measurements**:

| State Size | Resources | P50 Apply Time | P95 Apply Time | Memory (agent) |
|------------|-----------|----------------|----------------|----------------|
| Minimal | 1-10 | 50ms | 150ms | 10MB |
| Small | 10-50 | 200ms | 500ms | 25MB |
| Medium | 50-200 | 1s | 2.5s | 50MB |
| Large | 200-500 | 3s | 7s | 100MB |
| Very Large | 500-1000 | 8s | 18s | 200MB |
| Massive | 1000+ | 20s+ | 45s+ | 400MB+ |

**By Module Type**:

| Module | Avg Apply Time | Check Time | Notes |
|--------|---------------|------------|-------|
| `file` (small) | 5ms | 2ms | <1KB content |
| `file` (large) | 50ms | 10ms | 1MB+ content |
| `package` (installed) | 10ms | 5ms | No-op check |
| `package` (install) | 5-60s | 5ms | Depends on package |
| `service` | 20ms | 5ms | Start/stop |
| `user` | 30ms | 10ms | Create/modify |
| `registry` (Windows) | 15ms | 5ms | Per value |
| `command` | Variable | N/A | Depends on command |

**Key Metrics**:

```promql
# State application throughput (resources/sec)
rate(kscore_state_resources_applied_total[5m])

# State application duration
histogram_quantile(0.95, rate(kscore_state_apply_duration_seconds_bucket[5m]))

# By result
rate(kscore_state_resources_applied_total{result="changed"}[5m])
rate(kscore_state_resources_applied_total{result="unchanged"}[5m])
rate(kscore_state_resources_applied_total{result="failed"}[5m])

# DAG processing time
histogram_quantile(0.95, rate(kscore_state_dag_duration_seconds_bucket[5m]))
```

**Parallel Apply Performance** (100 agents):

| State Size | Sequential | Parallel (10) | Parallel (50) | Parallel (100) |
|------------|------------|---------------|---------------|----------------|
| 50 resources | 50s | 5s | 1.5s | 1s |
| 200 resources | 200s | 20s | 5s | 3s |
| 500 resources | 500s | 50s | 12s | 8s |

**Optimization Tips**:

- Minimize state file size (split into logical groups)
- Use state compilation for complex templates
- Enable parallel module execution where safe
- Use `check` mode to preview changes

#### Event Processing Latency

Event system latency from publish to reactor execution.

**Baseline Measurements**:

| Phase | P50 | P95 | P99 | Description |
|-------|-----|-----|-----|-------------|
| Publish to NATS | 1ms | 5ms | 15ms | Event ingestion |
| NATS to subscriber | 2ms | 10ms | 25ms | Message delivery |
| Event matching | 1ms | 3ms | 10ms | Pattern evaluation |
| Reactor dispatch | 2ms | 8ms | 20ms | Command creation |
| **Total (no reactor)** | **6ms** | **26ms** | **70ms** | Event routing only |

**By Event Volume**:

| Events/sec | P50 Latency | P95 Latency | CPU (control plane) |
|------------|-------------|-------------|---------------------|
| 100 | 5ms | 20ms | 5% |
| 1,000 | 8ms | 35ms | 15% |
| 10,000 | 15ms | 75ms | 40% |
| 50,000 | 50ms | 200ms | 80% |
| 100,000+ | 100ms+ | 500ms+ | Saturation |

**Key Metrics**:

```promql
# Event publish rate
rate(kscore_events_published_total[5m])

# Event processing latency
histogram_quantile(0.95, rate(kscore_event_processing_duration_seconds_bucket[5m]))

# Reactor execution rate
rate(kscore_reactor_executions_total[5m])

# Event queue depth
kscore_event_queue_depth

# Dropped events (at capacity)
rate(kscore_events_dropped_total[5m])
```

**JetStream Performance** (persistent events):

| Stream | Publish Rate | Consume Rate | Storage Overhead |
|--------|--------------|--------------|------------------|
| Memory | 500K msg/s | 500K msg/s | RAM only |
| File | 100K msg/s | 200K msg/s | ~100 bytes/msg |
| Replicated (R=3) | 30K msg/s | 100K msg/s | 3x storage |

**Optimization Tips**:

- Use memory streams for high-volume, non-critical events
- Configure appropriate retention limits
- Use consumer groups for parallel processing
- Batch event acknowledgments

#### Policy Evaluation Latency

Policy evaluation performance for OPA/Rego policies.

**Baseline Measurements**:

| Policy Complexity | P50 | P95 | P99 | Memory |
|-------------------|-----|-----|-----|--------|
| Simple (5 rules) | 0.5ms | 2ms | 5ms | 1MB |
| Moderate (20 rules) | 2ms | 8ms | 20ms | 5MB |
| Complex (50 rules) | 5ms | 20ms | 50ms | 15MB |
| Very Complex (100+ rules) | 15ms | 60ms | 150ms | 50MB |

**By Input Size**:

| Input Size | P50 | P95 | P99 |
|------------|-----|-----|-----|
| Small (<1KB) | 1ms | 5ms | 15ms |
| Medium (1-10KB) | 3ms | 15ms | 40ms |
| Large (10-100KB) | 10ms | 50ms | 125ms |
| Very Large (>100KB) | 50ms | 200ms | 500ms |

**Key Metrics**:

```promql
# Policy evaluation rate
rate(kscore_policy_evaluations_total[5m])

# Policy evaluation latency
histogram_quantile(0.95, rate(kscore_policy_evaluation_duration_seconds_bucket[5m]))

# By policy
histogram_quantile(0.95, rate(kscore_policy_evaluation_duration_seconds_bucket{policy="ssh-hardening"}[5m]))

# Violations detected
rate(kscore_policy_violations_total[5m])

# Cache hit rate
rate(kscore_policy_cache_hits_total[5m]) /
(rate(kscore_policy_cache_hits_total[5m]) + rate(kscore_policy_cache_misses_total[5m]))
```

**Bulk Evaluation Performance**:

| Agents | Policies | Total Evaluations | Duration (cached) | Duration (uncached) |
|--------|----------|-------------------|-------------------|---------------------|
| 100 | 10 | 1,000 | 100ms | 2s |
| 1,000 | 10 | 10,000 | 500ms | 20s |
| 1,000 | 50 | 50,000 | 2s | 100s |
| 10,000 | 10 | 100,000 | 5s | 200s |

**Optimization Tips**:

- Enable policy result caching
- Use partial evaluation for complex policies
- Pre-compile policies at startup
- Minimize input data size
- Use policy bundles for large policy sets

#### Performance Testing Methodology

**Load Testing Tools**:

```bash
# Use kscorectl built-in benchmarks
kscorectl benchmark agent-registration --count 1000 --parallel 50
kscorectl benchmark command-execution --count 10000 --parallel 100
kscorectl benchmark state-apply --state test.yaml --targets 100

# Export results
kscorectl benchmark --output json > benchmark-results.json
```

**Continuous Benchmarking**:

```yaml
# .github/workflows/benchmark.yml
name: Performance Benchmarks
on:
  push:
    branches: [main]

jobs:
  benchmark:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Run benchmarks
        run: |
          kscorectl benchmark all --output json > results.json
      - name: Compare with baseline
        run: |
          kscorectl benchmark compare baseline.json results.json --threshold 10%
      - name: Upload results
        uses: actions/upload-artifact@v4
        with:
          name: benchmark-results
          path: results.json
```

**Recording Rules for Baselines**:

```yaml
# recording_rules.yml
groups:
  - name: kscore_performance_baselines
    rules:
      # 7-day rolling P95 latencies
      - record: kscore:api_latency_p95:7d
        expr: histogram_quantile(0.95, avg_over_time(rate(kscore_api_request_duration_seconds_bucket[5m])[7d:1h]))

      - record: kscore:command_latency_p95:7d
        expr: histogram_quantile(0.95, avg_over_time(rate(kscore_command_overhead_duration_seconds_bucket[5m])[7d:1h]))

      - record: kscore:state_apply_p95:7d
        expr: histogram_quantile(0.95, avg_over_time(rate(kscore_state_apply_duration_seconds_bucket[5m])[7d:1h]))

      - record: kscore:event_latency_p95:7d
        expr: histogram_quantile(0.95, avg_over_time(rate(kscore_event_processing_duration_seconds_bucket[5m])[7d:1h]))

      - record: kscore:policy_eval_p95:7d
        expr: histogram_quantile(0.95, avg_over_time(rate(kscore_policy_evaluation_duration_seconds_bucket[5m])[7d:1h]))
```

### Service Level Objectives (SLOs)

**Control Plane SLOs:**

- **Availability**: 99.9% uptime (43 minutes downtime/month)
- **Latency**: P95 API latency <500ms, P99 <1s
- **Throughput**: 1000+ API requests/sec
- **Error Rate**: <0.1% (5xx errors)

**Agent SLOs:**

- **Availability**: 95% agents connected at all times
- **Command Execution**: P95 latency <100ms, P99 <500ms
- **Heartbeat**: 100% heartbeats within 30s interval

**State Management SLOs:**

- **Application Success**: >99% state applications succeed
- **Drift Detection**: Drift detected within 5 minutes
- **Idempotency**: 100% idempotent operations

### Monitoring SLOs

**SLO Dashboard Queries:**

```promql
# Availability (control plane)
avg_over_time(up{job="kscore-server"}[30d]) * 100

# API Latency P95
histogram_quantile(0.95, rate(kscore_api_request_duration_seconds_bucket[5m]))

# Error Rate
rate(kscore_api_requests_total{status=~"5.."}[5m]) /
rate(kscore_api_requests_total[5m])

# Agent Availability
(kscore_agents_connected / kscore_agents_total) * 100
```

**Error Budget Alerts:**

```yaml
- alert: SLOErrorBudgetExhausted
  expr: (1 - avg_over_time(up{job="kscore-server"}[30d])) > 0.001
  labels:
    severity: critical
  annotations:
    summary: "SLO error budget exhausted"
    description: "99.9% availability SLO violated"
```

## Profiling

Keystone Core includes built-in pprof profiling endpoints for debugging performance issues.

### Enabling Profiling

**Server Configuration:**

```yaml
# server.yaml
profiling:
  enabled: true
  listen: "127.0.0.1:6060"  # Restrict to localhost for security
  block_rate: 0             # Block profiling rate (0 = disabled)
  mutex_rate: 0             # Mutex profiling rate (0 = disabled)
```

**Agent Configuration:**

```yaml
# agent.yaml
profiling:
  enabled: true
  listen: "127.0.0.1:6061"
```

### Available Profiles

| Profile | Description | URL |
|---------|-------------|-----|
| CPU | CPU usage over duration | `/debug/pprof/profile?seconds=30` |
| Heap | Memory allocation snapshot | `/debug/pprof/heap` |
| Goroutine | All goroutine stack traces | `/debug/pprof/goroutine` |
| Mutex | Mutex contention | `/debug/pprof/mutex` |
| Block | Blocking operations | `/debug/pprof/block` |
| Allocs | Memory allocations | `/debug/pprof/allocs` |
| Trace | Execution trace | `/debug/pprof/trace?seconds=5` |

### Capturing Profiles

**Using curl:**

```bash
# CPU profile (30 seconds)
curl -o cpu.prof http://localhost:6060/debug/pprof/profile?seconds=30

# Heap profile
curl -o heap.prof http://localhost:6060/debug/pprof/heap

# Goroutine dump
curl -o goroutine.txt http://localhost:6060/debug/pprof/goroutine?debug=2

# Execution trace (5 seconds)
curl -o trace.out http://localhost:6060/debug/pprof/trace?seconds=5
```

**Using go tool pprof:**

```bash
# Interactive CPU analysis
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# Interactive heap analysis
go tool pprof http://localhost:6060/debug/pprof/heap

# Web UI (opens browser)
go tool pprof -http=:8080 http://localhost:6060/debug/pprof/profile?seconds=30
```

**Using go tool trace:**

```bash
# Capture trace
curl -o trace.out http://localhost:6060/debug/pprof/trace?seconds=5

# Analyze trace (opens browser)
go tool trace trace.out
```

### Common Profiling Scenarios

**High CPU Usage:**

```bash
# 1. Capture CPU profile during high load
go tool pprof -http=:8080 http://server:6060/debug/pprof/profile?seconds=60

# 2. Look for hot functions in the flame graph
# 3. Check the "top" view for functions consuming most CPU
```

**Memory Leaks:**

```bash
# 1. Capture heap profile at baseline
curl -o heap1.prof http://server:6060/debug/pprof/heap

# 2. Wait for suspected leak period

# 3. Capture another heap profile
curl -o heap2.prof http://server:6060/debug/pprof/heap

# 4. Compare profiles
go tool pprof -base=heap1.prof heap2.prof
```

**Goroutine Leaks:**

```bash
# Check goroutine count
curl -s http://server:6060/debug/pprof/goroutine?debug=1 | head -1

# Full stack traces
curl -s http://server:6060/debug/pprof/goroutine?debug=2 > goroutines.txt
```

**Mutex Contention:**

```yaml
# Enable mutex profiling in config
profiling:
  enabled: true
  mutex_rate: 1  # Profile all mutex operations
```

```bash
# Capture mutex profile
go tool pprof http://server:6060/debug/pprof/mutex
```

### Security Considerations

**Warning**: Profiling endpoints expose internal application state. Follow these practices:

1. **Bind to localhost**: Only expose profiling on `127.0.0.1`
2. **Use SSH tunneling** for remote access:

   ```bash
   ssh -L 6060:127.0.0.1:6060 user@server
   go tool pprof http://localhost:6060/debug/pprof/profile
   ```

3. **Disable in production** unless actively debugging
4. **Use firewall rules** to restrict access
5. **Enable authentication** if exposing externally

### Runtime Statistics

The profiling endpoint also provides runtime statistics:

```bash
# Get runtime stats
curl http://localhost:6060/debug/pprof/cmdline  # Command line
curl http://localhost:6060/debug/pprof/symbol   # Symbol table
```

**Programmatic Access:**

```go
// Get runtime stats via API
stats := profiling.GetStats()
fmt.Printf("Goroutines: %d\n", stats.NumGoroutine)
fmt.Printf("Heap Alloc: %d bytes\n", stats.HeapAlloc)
fmt.Printf("Heap Objects: %d\n", stats.HeapObjects)
fmt.Printf("GC Cycles: %d\n", stats.NumGC)
```

## Best Practices

### Metrics Collection

- **Scrape interval**: 10-15s for control plane, 30s for agents
- **Retention**: 30 days minimum, 90 days for compliance
- **Cardinality**: Limit high-cardinality labels (e.g., don't use correlation_id as label)
- **Aggregation**: Use recording rules for expensive queries

### Logging

- **Structured logging**: Always use JSON format
- **Log levels**: ERROR for actionable issues, WARN for degraded state, INFO for significant events
- **Correlation IDs**: Always include for request tracing
- **Sampling**: Sample DEBUG logs in production (1% sampling typical)

### Alerting

- **Alert on symptoms, not causes** - Alert on user impact (high latency) not causes (high CPU)
- **Group alerts** - Group by cluster/datacenter to reduce noise
- **Silence during maintenance** - Use Alertmanager silences for planned downtime
- **Runbooks**: Every alert should link to a runbook

### Dashboards

- **Role-based dashboards** - Separate dashboards for SRE, Dev, Ops
- **Drill-down capability** - Link from high-level to detailed views
- **Time range selector** - Make time ranges easily adjustable
- **Annotations**: Mark deployments, incidents, maintenance windows

## Troubleshooting Monitoring

### Prometheus Issues

**High Memory Usage:**

```bash
# Check TSDB stats
curl http://localhost:9090/api/v1/status/tsdb

# Reduce retention or increase resources
```

**Missing Targets:**

```bash
# Check target status
curl http://localhost:9090/api/v1/targets

# Verify network connectivity and firewall rules
```

### Grafana Issues

**Dashboard Not Loading:**

```bash
# Check Grafana logs
sudo journalctl -u grafana-server -f

# Verify datasource connectivity
# Grafana → Configuration → Data Sources → Test
```

**No Data in Panels:**

- Verify Prometheus datasource configured correctly
- Check time range selector
- Test PromQL query in Prometheus UI first
- Check for label mismatches

### Loki Issues

**Logs Not Appearing:**

```bash
# Check Promtail status
sudo systemctl status promtail

# Verify Promtail config
promtool check config promtail.yml

# Check Loki ingestion rate
curl http://localhost:3100/metrics | grep loki_ingester
```

## See Also

- [Deployment Guide](/docs/operations/deployment/) - Deploy monitoring stack
- [Troubleshooting Guide](/docs/operations/troubleshooting/) - Debug monitoring issues
- [Metrics Reference](/docs/reference/metrics/) - Complete metrics catalog
- [Grafana Dashboards](https://github.com/shawnbutts/grafana-dashboards) - Dashboard repository
