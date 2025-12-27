---
title: "Monitoring Guide"
weight: 2
description: >
  Complete observability setup with Prometheus, Grafana, logging, and alerting
---

## Overview

TitanAnvil provides comprehensive observability through Prometheus metrics, structured logging, distributed tracing, and pre-built Grafana dashboards. This guide covers the complete monitoring stack setup for production deployments.

**Monitoring Stack:**
- **Metrics**: Prometheus + Grafana (70+ metrics exposed)
- **Logging**: Structured JSON logs + Loki/Elasticsearch
- **Tracing**: OpenTelemetry + Jaeger (distributed tracing)
- **Alerting**: Alertmanager + PagerDuty/Slack integration

## Prometheus Integration

TitanAnvil exposes 70+ Prometheus metrics on `/metrics` endpoint.

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
  # TitanAnvil Control Plane
  - job_name: 'titananvil-server'
    static_configs:
      - targets:
          - 'server1:8080'
          - 'server2:8080'
          - 'server3:8080'
    metrics_path: '/metrics'
    scrape_interval: 10s

  # TitanAnvil Agents
  - job_name: 'titananvil-agents'
    relabel_configs:
      - source_labels: [__meta_kubernetes_pod_label_app]
        regex: titananvil-agent
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
rate(titananvil_api_requests_total[5m])

# API latency (P95)
histogram_quantile(0.95, rate(titananvil_api_request_duration_seconds_bucket[5m]))

# Connected agents
titananvil_agents_connected_total

# Command execution rate
rate(titananvil_commands_executed_total[5m])

# State application success rate
rate(titananvil_states_applied_total{result="success"}[5m]) /
rate(titananvil_states_applied_total[5m])
```

**Agent Metrics:**
```promql
# Agent heartbeat failures
titananvil_agent_heartbeat_failures_total

# Agent CPU usage
titananvil_agent_cpu_usage_percent

# Agent memory usage
titananvil_agent_memory_usage_bytes / titananvil_agent_memory_total_bytes

# Agent disk usage
titananvil_agent_disk_usage_bytes / titananvil_agent_disk_total_bytes
```

**Event System Metrics:**
```promql
# Event publish rate
rate(titananvil_events_published_total[5m])

# Reactor execution rate
rate(titananvil_reactor_executions_total[5m])

# Reactor failures
rate(titananvil_reactor_failures_total[5m])
```

**Policy Metrics:**
```promql
# Policy evaluations
rate(titananvil_policy_evaluations_total[5m])

# Policy violations by severity
sum by (severity) (titananvil_policy_violations_total)

# Compliance score
titananvil_policy_compliance_score
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

TitanAnvil provides 6 pre-built Grafana dashboards.

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
wget https://github.com/titananvil/titananvil/raw/main/deploy/grafana/dashboards/*.json \
  -P /etc/grafana/provisioning/dashboards/

# Configure dashboard provisioning
cat > /etc/grafana/provisioning/dashboards/titananvil.yml <<EOF
apiVersion: 1
providers:
  - name: 'TitanAnvil'
    folder: 'TitanAnvil'
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

**1. TitanAnvil Overview**
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
    "expr": "histogram_quantile(0.95, rate(titananvil_api_request_duration_seconds_bucket[5m]))",
    "legendFormat": "{{ method }} {{ path }}"
  }],
  "yaxes": [{
    "format": "s",
    "label": "Latency"
  }]
}
```

## Log Aggregation

TitanAnvil emits structured JSON logs for centralized aggregation.

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
  - job_name: titananvil-server
    static_configs:
      - targets:
          - localhost
        labels:
          job: titananvil-server
          __path__: /var/log/titananvil/server.log
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

  - job_name: titananvil-agents
    static_configs:
      - targets:
          - localhost
        labels:
          job: titananvil-agents
          __path__: /var/log/titananvil/agent-*.log
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
      - /var/log/titananvil/*.log
    json.keys_under_root: true
    json.add_error_key: true

output.elasticsearch:
  hosts: ["localhost:9200"]
  index: "titananvil-%{+yyyy.MM.dd}"

setup.template.name: "titananvil"
setup.template.pattern: "titananvil-*"
```

### Log Queries

**Loki Queries (LogQL):**
```logql
# All error logs
{job="titananvil-server"} |= "error"

# Logs with correlation ID
{job="titananvil-server"} | json | correlation_id="abc-123"

# API request logs (HTTP 500)
{job="titananvil-server"} | json | status_code="500"

# Agent connection failures
{job="titananvil-server"} | json | message=~"agent.*connection.*failed"
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
        title: 'TitanAnvil Alert'
        text: '{{ range .Alerts }}{{ .Annotations.description }}{{ end }}'
```

### Alert Rules

**Create Alert Rules (alerts.yml):**
```yaml
groups:
  - name: titananvil-control-plane
    interval: 30s
    rules:
      - alert: ControlPlaneDown
        expr: up{job="titananvil-server"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Control plane {{ $labels.instance }} is down"
          description: "Control plane has been down for more than 1 minute"

      - alert: HighAPILatency
        expr: histogram_quantile(0.95, rate(titananvil_api_request_duration_seconds_bucket[5m])) > 1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High API latency on {{ $labels.instance }}"
          description: "P95 latency is {{ $value }}s (threshold: 1s)"

      - alert: HighErrorRate
        expr: rate(titananvil_api_requests_total{status=~"5.."}[5m]) > 0.05
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High error rate on {{ $labels.instance }}"
          description: "Error rate is {{ $value | humanizePercentage }}"

  - name: titananvil-agents
    interval: 30s
    rules:
      - alert: AgentFleetLowAvailability
        expr: (titananvil_agents_connected_total / titananvil_agents_total) < 0.9
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Agent fleet availability is low"
          description: "Only {{ $value | humanizePercentage }} agents connected"

      - alert: AgentFleetCriticalAvailability
        expr: (titananvil_agents_connected_total / titananvil_agents_total) < 0.7
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Agent fleet availability is critical"
          description: "Only {{ $value | humanizePercentage }} agents connected"

      - alert: AgentHighCPU
        expr: titananvil_agent_cpu_usage_percent > 90
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Agent {{ $labels.agent_id }} high CPU usage"
          description: "CPU usage is {{ $value }}%"

  - name: titananvil-state
    interval: 30s
    rules:
      - alert: StateApplicationHighFailureRate
        expr: rate(titananvil_states_applied_total{result="failed"}[5m]) / rate(titananvil_states_applied_total[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High state application failure rate"
          description: "{{ $value | humanizePercentage }} of state applications are failing"

      - alert: DriftDetectionCritical
        expr: sum(titananvil_drift_detected_total{severity="critical"}) > 10
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Critical configuration drift detected"
          description: "{{ $value }} critical drift events detected"

  - name: titananvil-policy
    interval: 30s
    rules:
      - alert: PolicyViolationsCritical
        expr: sum(rate(titananvil_policy_violations_total{severity="critical"}[5m])) > 1
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Critical policy violations detected"
          description: "{{ $value }} critical policy violations per second"

      - alert: ComplianceScoreLow
        expr: titananvil_policy_compliance_score < 80
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Compliance score is low"
          description: "Compliance score is {{ $value }}% (threshold: 80%)"
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

TitanAnvil exposes health check endpoints for load balancers and orchestrators.

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
upstream titananvil {
    server server1:8080 max_fails=3 fail_timeout=30s;
    server server2:8080 max_fails=3 fail_timeout=30s;
}

server {
    location /health/ready {
        proxy_pass http://titananvil;
    }
}
```

## Performance Monitoring

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
avg_over_time(up{job="titananvil-server"}[30d]) * 100

# API Latency P95
histogram_quantile(0.95, rate(titananvil_api_request_duration_seconds_bucket[5m]))

# Error Rate
rate(titananvil_api_requests_total{status=~"5.."}[5m]) /
rate(titananvil_api_requests_total[5m])

# Agent Availability
(titananvil_agents_connected_total / titananvil_agents_total) * 100
```

**Error Budget Alerts:**
```yaml
- alert: SLOErrorBudgetExhausted
  expr: (1 - avg_over_time(up{job="titananvil-server"}[30d])) > 0.001
  labels:
    severity: critical
  annotations:
    summary: "SLO error budget exhausted"
    description: "99.9% availability SLO violated"
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

- [Deployment Guide](deployment/) - Deploy monitoring stack
- [Troubleshooting Guide](troubleshooting/) - Debug monitoring issues
- [Metrics Reference](/docs/reference/metrics/) - Complete metrics catalog
- [Grafana Dashboards](https://github.com/titananvil/grafana-dashboards) - Dashboard repository
