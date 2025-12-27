# Keystone Core Grafana Dashboards

This directory contains pre-built Grafana dashboards and Prometheus alert rules for monitoring Keystone Core infrastructure.

## Contents

```
deploy/grafana/
├── dashboards/              # Grafana dashboard JSON files
│   ├── kscore-overview.json          # System overview
│   ├── control-plane-health.json         # Control plane metrics
│   ├── agent-fleet.json                  # Agent fleet monitoring
│   ├── state-management.json             # State operations
│   ├── policy-compliance.json            # Policy & compliance
│   └── gitops-operations.json            # GitOps deployments
├── alerts/                  # Prometheus alert rules
│   └── kscore-alerts.yml             # Alert definitions
├── provisioning/            # Grafana provisioning configs
│   ├── datasources/         # Datasource configurations
│   ├── dashboards/          # Dashboard provisioning
│   └── alerting/            # Alert provisioning
├── docker-compose.yml       # Docker Compose setup
└── README.md               # This file
```

## Dashboards

### 1. Keystone Core Overview
**UID**: `kscore-overview`

High-level system overview providing at-a-glance visibility into Keystone Core operations.

**Panels**:
- Total Agents (stat)
- Connected Agents (stat)
- Disconnected Agents (stat)
- Policy Violations (stat)
- Commands Per Second (graph)
- State Applications Per Hour (graph)
- Agent Status Distribution (pie chart)
- Command Success Rate (gauge)
- State Application Success Rate (gauge)
- Recent Events Timeline (table)

**Variables**:
- `$environment` - Filter by environment
- `$datacenter` - Filter by datacenter

**Use Cases**:
- Daily operations monitoring
- Executive dashboards
- Quick health checks
- Incident overview

### 2. Control Plane Health
**UID**: `kscore-control-plane`

Detailed monitoring of Keystone Core control plane performance and resource utilization.

**Panels**:
- Control Plane Status (stat)
- Uptime (stat)
- Memory Usage (gauge)
- Goroutines (gauge)
- API Request Rate (graph)
- API Request Latency p95/p99 (graph)
- NATS Message Throughput (graph)
- NATS Bandwidth (graph)
- State Backend Query Latency (graph)
- Error Rates by Component (stacked graph)
- CPU Usage (graph)
- Memory Usage Over Time (graph)

**Variables**:
- `$instance` - Filter by control plane instance

**Use Cases**:
- Performance troubleshooting
- Capacity planning
- SLA monitoring
- Incident investigation

### 3. Agent Fleet
**UID**: `kscore-agent-fleet`

Comprehensive agent fleet monitoring including health, performance, and resource utilization.

**Panels**:
- Total Agents (stat)
- Healthy Agents (stat)
- Degraded Agents (stat)
- Offline Agents (stat)
- Agent Distribution by Datacenter (pie chart)
- Agent Distribution by Role (pie chart)
- Agent Health Status Over Time (stacked graph)
- Command Execution Success Rate by Agent (graph)
- Agent Version Distribution (bar graph)
- Agent CPU Usage (graph)
- Agent Memory Usage (graph)
- Agent Disk Usage (graph)

**Variables**:
- `$datacenter` - Filter by datacenter
- `$role` - Filter by agent role
- `$agent_id` - Filter by specific agent

**Use Cases**:
- Fleet health monitoring
- Agent deployment verification
- Resource utilization tracking
- Agent upgrade planning

### 4. State Management
**UID**: `kscore-state-management`

Monitors state application operations, drift detection, and configuration management.

**Panels**:
- State Applications Total (stat)
- Successful Applications (stat)
- Failed Applications (stat)
- Drift Events (stat)
- State Applications Over Time (graph)
- State Success Rate (graph)
- State Changes by Module (stacked graph)
- Drift Detection Events by Severity (stacked graph)
- State Application Duration p95/p99 (graph)
- Failed State Applications by Reason (stacked graph)
- Resources Under Management (stacked graph)
- Drift by Severity (pie chart)

**Variables**:
- `$environment` - Filter by environment
- `$module` - Filter by state module

**Use Cases**:
- Configuration drift monitoring
- State application troubleshooting
- Compliance verification
- Capacity planning

### 5. Policy Compliance
**UID**: `kscore-policy-compliance`

Tracks policy violations, compliance scores, and remediation effectiveness.

**Panels**:
- Overall Compliance Score (gauge)
- Total Violations (stat)
- Remediation Success Rate (gauge)
- Critical Violations (stat)
- High Severity Violations (stat)
- Violations by Severity (stacked graph)
- Compliance Score by Framework (graph)
- Top Violated Policies (bar graph)
- Remediation Success Over Time (graph)
- Policy Evaluation Rate (graph)
- Policy Evaluation Duration p95/p99 (graph)
- Compliance Trend 7-day (graph)

**Variables**:
- `$framework` - Filter by compliance framework
- `$environment` - Filter by environment

**Use Cases**:
- Compliance reporting
- Security posture monitoring
- Audit preparation
- Policy effectiveness analysis

### 6. GitOps Operations
**UID**: `kscore-gitops-operations`

Monitors GitOps deployments, verification workflows, and rollback operations.

**Panels**:
- Total Deployments (stat)
- Deployments Verified (stat)
- Failed Verifications (stat)
- Rollbacks Triggered (stat)
- Deployments Per Hour (bar graph)
- Verification Success Rate (graph)
- Deployment Verification Duration p95/p99 (graph)
- Rollback Frequency by Reason (stacked graph)
- Failed Verifications by Application (bar graph)
- Deployments by Environment (pie chart)
- Webhook Events Received by Source (stacked graph)
- Rollback Duration p95/p99 (graph)

**Variables**:
- `$application` - Filter by application
- `$environment` - Filter by environment

**Use Cases**:
- Deployment monitoring
- DORA metrics tracking
- Incident analysis
- Change management

## Prometheus Alerts

Alert rules are defined in `alerts/kscore-alerts.yml` and organized into groups:

### Control Plane Alerts
- **ControlPlaneDown** (critical): Control plane instance is down for >1 minute
- **ControlPlaneHighMemoryUsage** (warning): Memory usage >1536MB for >5 minutes
- **ControlPlaneHighGoroutineCount** (warning): Goroutines >800 for >5 minutes
- **ControlPlaneHighAPILatency** (warning): p95 API latency >1.0s for >5 minutes
- **ControlPlaneHighErrorRate** (warning): Error rate >1.0/sec for >5 minutes

### Agent Fleet Alerts
- **AgentFleetLowAvailability** (warning): <80% agents connected for >5 minutes
- **AgentFleetCriticalAvailability** (critical): <50% agents connected for >2 minutes
- **MultipleAgentsOffline** (warning): >10 agents offline for >5 minutes
- **AgentHighCPUUsage** (warning): Agent CPU >90% for >10 minutes
- **AgentHighMemoryUsage** (warning): Agent memory >90% for >10 minutes
- **AgentHighDiskUsage** (warning): Agent disk >85% for >10 minutes
- **AgentCommandFailureRate** (warning): Command failure rate >10% for >10 minutes

### State Management Alerts
- **StateApplicationHighFailureRate** (warning): Failure rate >5% for >10 minutes
- **StateApplicationCriticalFailureRate** (critical): Failure rate >20% for >5 minutes
- **HighDriftDetectionRate** (warning): >0.5 high/critical drift events/sec for >10 minutes
- **StateApplicationSlowPerformance** (warning): p95 duration >300s for >10 minutes

### Policy Alerts
- **CriticalPolicyViolations** (critical): Any critical violations detected
- **HighPolicyViolations** (warning): >10 high-severity violations for >5 minutes
- **ComplianceScoreBelowThreshold** (warning): Compliance score <85% for >10 minutes
- **ComplianceScoreCritical** (critical): Compliance score <70% for >5 minutes
- **PolicyRemediationFailureRate** (warning): Remediation failure rate >30% for >10 minutes

### GitOps Alerts
- **GitOpsVerificationFailures** (warning): >0.1 verification failures/sec for >5 minutes
- **GitOpsHighRollbackRate** (warning): >0.05 rollbacks/sec for >10 minutes
- **GitOpsVerificationSlowPerformance** (warning): p95 duration >600s for >10 minutes
- **GitOpsWebhookProcessingErrors** (warning): >0.1 webhook errors/sec for >5 minutes

### NATS Alerts
- **NATSHighMemoryUsage** (warning): Memory >1024MB for >5 minutes
- **NATSSlowConsumers** (warning): Slow consumers detected for >2 minutes
- **NATSConnectionsHigh** (warning): >1000 connections for >5 minutes
- **NATSJetStreamStorageUsage** (warning): Storage >85% full for >5 minutes

## Quick Start

### Using Docker Compose

The easiest way to run Grafana with Keystone Core dashboards:

```bash
# Navigate to the grafana directory
cd deploy/grafana

# Set alert notification environment variables (optional)
export SLACK_WEBHOOK_URL="https://hooks.slack.com/services/YOUR/WEBHOOK/URL"
export ALERT_EMAIL_ADDRESSES="ops@example.com,sre@example.com"

# Start Prometheus and Grafana
docker-compose up -d

# Access Grafana
open http://localhost:3000
# Default credentials: admin/admin
```

### Manual Installation

#### 1. Import Dashboards

In Grafana UI:
1. Navigate to **Dashboards** → **Import**
2. Upload JSON files from `dashboards/` directory
3. Select **Prometheus** as the datasource

Via API:
```bash
# Import all dashboards
for dashboard in dashboards/*.json; do
  curl -X POST \
    -H "Content-Type: application/json" \
    -d @"$dashboard" \
    http://admin:admin@localhost:3000/api/dashboards/import
done
```

#### 2. Configure Prometheus Datasource

In Grafana UI:
1. Navigate to **Configuration** → **Data Sources**
2. Add **Prometheus**
3. Set URL to your Prometheus instance (e.g., `http://prometheus:9090`)
4. Click **Save & Test**

Via provisioning:
```bash
# Copy datasource config
cp provisioning/datasources/prometheus.yml /etc/grafana/provisioning/datasources/

# Restart Grafana
systemctl restart grafana-server
```

#### 3. Configure Prometheus Alert Rules

Add to your Prometheus configuration:

```yaml
# prometheus.yml
rule_files:
  - /path/to/kscore-alerts.yml

alerting:
  alertmanagers:
    - static_configs:
        - targets:
            - alertmanager:9093
```

Reload Prometheus:
```bash
curl -X POST http://localhost:9090/-/reload
```

## Metrics Reference

### Control Plane Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `kscore_api_requests_total` | Counter | endpoint, method, status | Total API requests |
| `kscore_api_request_duration_seconds` | Histogram | endpoint | API request duration |
| `kscore_errors_total` | Counter | component, type | Total errors by component |
| `kscore_state_query_duration_seconds` | Histogram | operation | State backend query duration |

### Agent Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `kscore_agents_total` | Gauge | datacenter, role, environment | Total agents |
| `kscore_agents_connected` | Gauge | datacenter, role | Connected agents |
| `kscore_agents_disconnected` | Gauge | datacenter, role | Disconnected agents |
| `kscore_agents_by_status` | Gauge | status, datacenter, role | Agents grouped by status |
| `kscore_agent_heartbeat` | Gauge | agent_id, datacenter, role | Agent heartbeat timestamp |
| `kscore_agent_cpu_usage` | Gauge | agent_id | Agent CPU usage percent |
| `kscore_agent_memory_usage` | Gauge | agent_id | Agent memory usage percent |
| `kscore_agent_disk_usage` | Gauge | agent_id | Agent disk usage percent |
| `kscore_agent_commands_total` | Counter | agent_id | Total commands executed |
| `kscore_agent_commands_successful` | Counter | agent_id | Successful commands |
| `kscore_agent_commands_failed` | Counter | agent_id | Failed commands |

### State Management Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `kscore_state_applications_total` | Counter | environment | Total state applications |
| `kscore_state_successful_total` | Counter | environment | Successful state applications |
| `kscore_state_failed_total` | Counter | environment, reason | Failed state applications |
| `kscore_state_changes_total` | Counter | module, environment | State changes by module |
| `kscore_drift_detected_total` | Counter | severity, environment | Drift detection events |
| `kscore_state_duration_seconds` | Histogram | environment | State application duration |
| `kscore_state_resources_total` | Gauge | type, environment | Resources under management |

### Policy Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `kscore_policy_violations_total` | Counter | policy, severity, framework, environment | Policy violations |
| `kscore_policy_compliance_score` | Gauge | framework, environment | Compliance score percent |
| `kscore_policy_evaluations_total` | Counter | framework, environment | Total policy evaluations |
| `kscore_policy_evaluation_duration_seconds` | Histogram | framework, environment | Policy evaluation duration |
| `kscore_policy_remediation_attempted_total` | Counter | framework, environment | Remediation attempts |
| `kscore_policy_remediation_successful_total` | Counter | framework, environment | Successful remediations |
| `kscore_policy_remediation_failed_total` | Counter | framework, environment | Failed remediations |

### GitOps Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `kscore_gitops_deployments_total` | Counter | application, environment | Total deployments |
| `kscore_gitops_verifications_total` | Counter | application, environment | Total verifications |
| `kscore_gitops_verifications_successful_total` | Counter | application, environment | Successful verifications |
| `kscore_gitops_verifications_failed_total` | Counter | application, environment | Failed verifications |
| `kscore_gitops_verification_duration_seconds` | Histogram | application, environment | Verification duration |
| `kscore_gitops_rollbacks_total` | Counter | application, environment, reason | Total rollbacks |
| `kscore_gitops_rollback_duration_seconds` | Histogram | application, environment | Rollback duration |
| `kscore_gitops_webhooks_total` | Counter | source, application, environment | Webhook events received |
| `kscore_gitops_webhook_errors_total` | Counter | source, error_type | Webhook processing errors |

### NATS Metrics

Keystone Core leverages standard NATS server metrics:

| Metric | Type | Description |
|--------|------|-------------|
| `nats_varz_mem` | Gauge | Memory usage |
| `nats_varz_in_msgs` | Counter | Inbound messages |
| `nats_varz_out_msgs` | Counter | Outbound messages |
| `nats_varz_in_bytes` | Counter | Inbound bytes |
| `nats_varz_out_bytes` | Counter | Outbound bytes |
| `nats_varz_connections` | Gauge | Active connections |
| `nats_varz_slow_consumers` | Gauge | Slow consumer count |
| `nats_jetstream_storage_used` | Gauge | JetStream storage used |
| `nats_jetstream_storage_total` | Gauge | JetStream storage total |

## Customization

### Adding Custom Variables

Edit dashboard JSON and add to `templating.list`:

```json
{
  "name": "my_variable",
  "type": "query",
  "datasource": {"type": "prometheus", "uid": "${DS_PROMETHEUS}"},
  "query": "label_values(my_metric, my_label)",
  "refresh": 1,
  "multi": true,
  "includeAll": true,
  "allValue": ".*"
}
```

### Adding Custom Panels

1. Open dashboard in Grafana
2. Click **Add panel**
3. Configure visualization and query
4. Save dashboard
5. Export JSON via **Dashboard settings** → **JSON Model**
6. Replace file in `dashboards/` directory

### Modifying Alert Thresholds

Edit `alerts/kscore-alerts.yml` and adjust `expr` or `for` values:

```yaml
- alert: MyCustomAlert
  expr: my_metric > 100  # Change threshold
  for: 10m               # Change duration
  labels:
    severity: warning    # Change severity
```

## Troubleshooting

### Dashboards Not Appearing

1. Check Grafana logs: `docker-compose logs grafana`
2. Verify provisioning directory is mounted: `docker-compose config`
3. Check dashboard JSON validity: Use [JSONLint](https://jsonlint.com/)
4. Restart Grafana: `docker-compose restart grafana`

### No Data in Panels

1. Verify Prometheus is scraping Keystone Core metrics:
   ```bash
   curl http://localhost:9090/api/v1/targets
   ```
2. Check Keystone Core `/metrics` endpoint:
   ```bash
   curl http://kscore-server:8080/metrics
   ```
3. Test Prometheus query manually:
   - Open Prometheus UI at `http://localhost:9090`
   - Run query from dashboard panel

### Alerts Not Firing

1. Verify alert rules loaded in Prometheus:
   ```bash
   curl http://localhost:9090/api/v1/rules
   ```
2. Check alert evaluation in Prometheus UI
3. Verify Alertmanager is configured and reachable
4. Check Alertmanager logs for delivery issues

## Best Practices

### Dashboard Organization

- **Overview first**: Start with the Keystone Core Overview dashboard for daily monitoring
- **Drill down**: Use other dashboards for deep-dive troubleshooting
- **Custom folders**: Organize dashboards in Grafana folders by team or function
- **Favorites**: Star frequently used dashboards for quick access

### Alert Configuration

- **Tune thresholds**: Adjust alert thresholds based on your environment
- **Avoid alert fatigue**: Set appropriate `for` durations to avoid flapping
- **Use labels**: Tag alerts with `component`, `severity`, and `environment`
- **Test alerts**: Use Prometheus `/alerts` endpoint to verify before deploying

### Performance Optimization

- **Limit time range**: Use appropriate dashboard time ranges (1h for real-time, 6h for trends)
- **Use recording rules**: Pre-compute expensive queries with Prometheus recording rules
- **Dashboard variables**: Use variables to reduce number of queries
- **Auto-refresh**: Set reasonable refresh intervals (30s-1m) to reduce load

## Contributing

To contribute improvements to Keystone Core dashboards:

1. Make changes to dashboard JSON files
2. Test thoroughly in your environment
3. Document any new metrics or panels
4. Submit pull request with clear description
5. Update this README if adding new dashboards

## Support

For issues or questions:
- **GitHub Issues**: [Keystone Core Repository](https://github.com/kscore/keystone-core/issues)
- **Documentation**: See main Keystone Core documentation
- **Metrics Reference**: Epic 7 (Observability) in project epics

## License

Keystone Core dashboards are part of the Keystone Core project and share the same license.
