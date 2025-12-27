---
title: "Metrics Reference"
weight: 6
description: >
  Complete Prometheus metrics catalog with labels, types, and query examples
---

## Overview

TitanAnvil exposes 70+ Prometheus-compatible metrics for monitoring system health, performance, and operations.

**Metrics Endpoint**: `http://control-plane:8080/metrics`

**Metric Categories**:
- [Control Plane Metrics](#control-plane-metrics)
- [Agent Metrics](#agent-metrics)
- [Execution Metrics](#execution-metrics)
- [State Management Metrics](#state-management-metrics)
- [Event System Metrics](#event-system-metrics)
- [Policy Metrics](#policy-metrics)
- [GitOps Metrics](#gitops-metrics)

## Metric Types

**Counter**: Cumulative value that only increases
**Gauge**: Current value that can go up or down
**Histogram**: Distribution of observations (with buckets)
**Summary**: Distribution with calculated quantiles

## Control Plane Metrics

### API Server

**titananvil_api_requests_total**
- Type: Counter
- Description: Total HTTP API requests
- Labels:
  - `method`: HTTP method (GET, POST, etc.)
  - `path`: API path
  - `status`: HTTP status code
- Example:
  ```
  titananvil_api_requests_total{method="POST",path="/api/v1/exec",status="200"} 1234
  ```

**titananvil_api_request_duration_seconds**
- Type: Summary
- Description: API request duration
- Labels:
  - `method`: HTTP method
  - `path`: API path
  - `quantile`: 0.5, 0.95, 0.99
- Example:
  ```
  titananvil_api_request_duration_seconds{method="POST",path="/api/v1/exec",quantile="0.95"} 0.150
  ```

**titananvil_api_active_connections**
- Type: Gauge
- Description: Current active connections
- Example:
  ```
  titananvil_api_active_connections 42
  ```

### NATS Message Bus

**titananvil_nats_messages_in_total**
- Type: Counter
- Description: Messages received from NATS
- Example:
  ```
  titananvil_nats_messages_in_total 1000000
  ```

**titananvil_nats_messages_out_total**
- Type: Counter
- Description: Messages sent to NATS
- Example:
  ```
  titananvil_nats_messages_out_total 950000
  ```

**titananvil_nats_bytes_in_total**
- Type: Counter
- Description: Bytes received from NATS
- Example:
  ```
  titananvil_nats_bytes_in_total 10737418240
  ```

**titananvil_nats_bytes_out_total**
- Type: Counter
- Description: Bytes sent to NATS
- Example:
  ```
  titananvil_nats_bytes_out_total 9663676416
  ```

**titananvil_nats_reconnections_total**
- Type: Counter
- Description: NATS reconnection count
- Example:
  ```
  titananvil_nats_reconnections_total 5
  ```

### Database

**titananvil_db_connections_active**
- Type: Gauge
- Description: Active database connections
- Example:
  ```
  titananvil_db_connections_active 15
  ```

**titananvil_db_connections_idle**
- Type: Gauge
- Description: Idle database connections
- Example:
  ```
  titananvil_db_connections_idle 5
  ```

**titananvil_db_query_duration_seconds**
- Type: Summary
- Description: Database query duration
- Labels:
  - `operation`: Query type (select, insert, update, delete)
  - `quantile`: 0.5, 0.95, 0.99
- Example:
  ```
  titananvil_db_query_duration_seconds{operation="select",quantile="0.95"} 0.005
  ```

## Agent Metrics

### Agent Status

**titananvil_agents_connected_total**
- Type: Gauge
- Description: Connected agents
- Labels:
  - `datacenter`: Agent datacenter
  - `environment`: Agent environment
  - `role`: Agent role
- Example:
  ```
  titananvil_agents_connected_total{datacenter="us-east-1",environment="production",role="web"} 50
  ```

**titananvil_agents_disconnected_total**
- Type: Counter
- Description: Total agent disconnections
- Labels:
  - `reason`: Disconnect reason (timeout, graceful, error)
- Example:
  ```
  titananvil_agents_disconnected_total{reason="timeout"} 25
  ```

**titananvil_agent_heartbeat_received_total**
- Type: Counter
- Description: Heartbeats received
- Example:
  ```
  titananvil_agent_heartbeat_received_total 1000000
  ```

**titananvil_agent_heartbeat_missed_total**
- Type: Counter
- Description: Missed heartbeats
- Example:
  ```
  titananvil_agent_heartbeat_missed_total 150
  ```

### Agent Resources

**titananvil_agent_cpu_usage_percent**
- Type: Gauge
- Description: Agent CPU usage
- Labels:
  - `agent_id`: Agent identifier
- Example:
  ```
  titananvil_agent_cpu_usage_percent{agent_id="web-01"} 45.2
  ```

**titananvil_agent_memory_usage_bytes**
- Type: Gauge
- Description: Agent memory usage
- Labels:
  - `agent_id`: Agent identifier
- Example:
  ```
  titananvil_agent_memory_usage_bytes{agent_id="web-01"} 4294967296
  ```

**titananvil_agent_memory_total_bytes**
- Type: Gauge
- Description: Agent total memory
- Labels:
  - `agent_id`: Agent identifier
- Example:
  ```
  titananvil_agent_memory_total_bytes{agent_id="web-01"} 8589934592
  ```

**titananvil_agent_disk_usage_bytes**
- Type: Gauge
- Description: Agent disk usage
- Labels:
  - `agent_id`: Agent identifier
  - `mount`: Mount point
- Example:
  ```
  titananvil_agent_disk_usage_bytes{agent_id="web-01",mount="/"} 21474836480
  ```

**titananvil_agent_disk_total_bytes**
- Type: Gauge
- Description: Agent total disk
- Labels:
  - `agent_id`: Agent identifier
  - `mount`: Mount point
- Example:
  ```
  titananvil_agent_disk_total_bytes{agent_id="web-01",mount="/"} 107374182400
  ```

## Execution Metrics

### Commands

**titananvil_commands_executed_total**
- Type: Counter
- Description: Commands executed
- Labels:
  - `status`: success, failed, timeout
  - `datacenter`: Target datacenter
- Example:
  ```
  titananvil_commands_executed_total{status="success",datacenter="us-east-1"} 5000
  ```

**titananvil_command_duration_seconds**
- Type: Summary
- Description: Command execution duration
- Labels:
  - `quantile`: 0.5, 0.95, 0.99
- Example:
  ```
  titananvil_command_duration_seconds{quantile="0.95"} 2.5
  ```

**titananvil_command_target_count**
- Type: Histogram
- Description: Number of targeted agents
- Labels:
  - `le`: Bucket upper bound
- Example:
  ```
  titananvil_command_target_count_bucket{le="10"} 500
  titananvil_command_target_count_bucket{le="50"} 800
  ```

### Batch Jobs

**titananvil_batch_jobs_total**
- Type: Counter
- Description: Batch jobs executed
- Labels:
  - `status`: completed, failed
- Example:
  ```
  titananvil_batch_jobs_total{status="completed"} 250
  ```

**titananvil_batch_size**
- Type: Summary
- Description: Batch size distribution
- Labels:
  - `quantile`: 0.5, 0.95, 0.99
- Example:
  ```
  titananvil_batch_size{quantile="0.95"} 50
  ```

## State Management Metrics

### State Applications

**titananvil_state_applications_total**
- Type: Counter
- Description: State applications
- Labels:
  - `status`: success, failed
- Example:
  ```
  titananvil_state_applications_total{status="success"} 1000
  ```

**titananvil_state_application_duration_seconds**
- Type: Summary
- Description: State application duration
- Labels:
  - `quantile`: 0.5, 0.95, 0.99
- Example:
  ```
  titananvil_state_application_duration_seconds{quantile="0.95"} 30
  ```

### Resources

**titananvil_state_resources_total**
- Type: Gauge
- Description: Resources under management
- Labels:
  - `module`: State module (file, package, service, etc.)
- Example:
  ```
  titananvil_state_resources_total{module="file"} 500
  titananvil_state_resources_total{module="package"} 200
  ```

**titananvil_state_changes_total**
- Type: Counter
- Description: State changes
- Labels:
  - `module`: State module
- Example:
  ```
  titananvil_state_changes_total{module="file"} 150
  ```

### Drift Detection

**titananvil_state_drift_detected_total**
- Type: Counter
- Description: Drift detections
- Labels:
  - `severity`: low, medium, high, critical
- Example:
  ```
  titananvil_state_drift_detected_total{severity="high"} 25
  ```

## Event System Metrics

### Events

**titananvil_events_published_total**
- Type: Counter
- Description: Events published
- Labels:
  - `type`: Event type
- Example:
  ```
  titananvil_events_published_total{type="agent.connect"} 500
  ```

**titananvil_events_processed_total**
- Type: Counter
- Description: Events processed
- Labels:
  - `type`: Event type
- Example:
  ```
  titananvil_events_processed_total{type="job.complete"} 1000
  ```

**titananvil_events_failed_total**
- Type: Counter
- Description: Event processing failures
- Labels:
  - `type`: Event type
- Example:
  ```
  titananvil_events_failed_total{type="state.drift"} 5
  ```

**titananvil_events_severity_total**
- Type: Counter
- Description: Events by severity
- Labels:
  - `severity`: debug, info, warning, error, critical
- Example:
  ```
  titananvil_events_severity_total{severity="warning"} 250
  ```

### Event Processing

**titananvil_event_processing_duration_seconds**
- Type: Summary
- Description: Event processing duration
- Labels:
  - `quantile`: 0.5, 0.95, 0.99
- Example:
  ```
  titananvil_event_processing_duration_seconds{quantile="0.95"} 0.010
  ```

**titananvil_event_lag_seconds**
- Type: Gauge
- Description: Event processing lag
- Example:
  ```
  titananvil_event_lag_seconds 0.5
  ```

### Storage

**titananvil_events_stored_total**
- Type: Counter
- Description: Events stored to database
- Example:
  ```
  titananvil_events_stored_total 500000
  ```

**titananvil_events_storage_errors_total**
- Type: Counter
- Description: Event storage errors
- Example:
  ```
  titananvil_events_storage_errors_total 10
  ```

**titananvil_events_count**
- Type: Gauge
- Description: Current event count
- Labels:
  - `type`: Event type
- Example:
  ```
  titananvil_events_count{type="agent.connect"} 10000
  ```

### Reactors

**titananvil_reactor_executions_total**
- Type: Counter
- Description: Reactor executions
- Labels:
  - `reactor`: Reactor name
- Example:
  ```
  titananvil_reactor_executions_total{reactor="auto_remediate_drift"} 50
  ```

**titananvil_reactor_failures_total**
- Type: Counter
- Description: Reactor failures
- Labels:
  - `reactor`: Reactor name
- Example:
  ```
  titananvil_reactor_failures_total{reactor="auto_remediate_drift"} 2
  ```

**titananvil_reactor_duration_seconds**
- Type: Summary
- Description: Reactor execution duration
- Labels:
  - `reactor`: Reactor name
  - `quantile`: 0.5, 0.95, 0.99
- Example:
  ```
  titananvil_reactor_duration_seconds{reactor="auto_remediate_drift",quantile="0.95"} 5.0
  ```

**titananvil_action_executions_total**
- Type: Counter
- Description: Reactor action executions
- Labels:
  - `type`: Action type (command, webhook, etc.)
  - `name`: Action name
- Example:
  ```
  titananvil_action_executions_total{type="webhook",name="slack_notify"} 100
  ```

## Policy Metrics

### Evaluations

**titananvil_policy_evaluations_total**
- Type: Counter
- Description: Policy evaluations
- Labels:
  - `policy`: Policy ID
  - `result`: allowed, denied
- Example:
  ```
  titananvil_policy_evaluations_total{policy="ssh-hardening",result="allowed"} 900
  ```

**titananvil_policy_evaluation_duration_seconds**
- Type: Summary
- Description: Policy evaluation duration
- Labels:
  - `policy`: Policy ID
  - `quantile`: 0.5, 0.95, 0.99
- Example:
  ```
  titananvil_policy_evaluation_duration_seconds{policy="ssh-hardening",quantile="0.95"} 0.005
  ```

### Violations

**titananvil_policy_violations_total**
- Type: Counter
- Description: Policy violations
- Labels:
  - `policy`: Policy ID
  - `severity`: low, medium, high, critical
- Example:
  ```
  titananvil_policy_violations_total{policy="ssh-hardening",severity="high"} 15
  ```

**titananvil_policy_violations_by_agent**
- Type: Gauge
- Description: Violations per agent
- Labels:
  - `agent`: Agent ID
  - `policy`: Policy ID
- Example:
  ```
  titananvil_policy_violations_by_agent{agent="web-01",policy="ssh-hardening"} 2
  ```

### Compliance

**titananvil_policy_compliance_score**
- Type: Gauge
- Description: Compliance score (0-100)
- Labels:
  - `policy_set`: Policy set ID
  - `environment`: Environment
- Example:
  ```
  titananvil_policy_compliance_score{policy_set="security-baseline",environment="production"} 87.5
  ```

**titananvil_policy_compliant_agents**
- Type: Gauge
- Description: Compliant agent count
- Labels:
  - `environment`: Environment
- Example:
  ```
  titananvil_policy_compliant_agents{environment="production"} 45
  ```

### Remediations

**titananvil_policy_remediations_total**
- Type: Counter
- Description: Policy remediations
- Labels:
  - `policy`: Policy ID
  - `status`: success, failed
- Example:
  ```
  titananvil_policy_remediations_total{policy="ssh-hardening",status="success"} 10
  ```

## GitOps Metrics

### Webhooks

**titananvil_gitops_webhooks_received_total**
- Type: Counter
- Description: Webhooks received
- Labels:
  - `source`: argocd, flux, github, gitlab
- Example:
  ```
  titananvil_gitops_webhooks_received_total{source="argocd"} 500
  ```

**titananvil_gitops_webhooks_failed_total**
- Type: Counter
- Description: Webhook processing failures
- Labels:
  - `source`: Webhook source
- Example:
  ```
  titananvil_gitops_webhooks_failed_total{source="argocd"} 5
  ```

### Verifications

**titananvil_gitops_verifications_total**
- Type: Counter
- Description: Deployment verifications
- Labels:
  - `status`: success, failed
- Example:
  ```
  titananvil_gitops_verifications_total{status="success"} 450
  ```

**titananvil_gitops_verification_duration_seconds**
- Type: Summary
- Description: Verification duration
- Labels:
  - `quantile`: 0.5, 0.95, 0.99
- Example:
  ```
  titananvil_gitops_verification_duration_seconds{quantile="0.95"} 60
  ```

### Rollbacks

**titananvil_gitops_rollbacks_total**
- Type: Counter
- Description: Rollbacks triggered
- Labels:
  - `type`: argocd, flux, git, manual
  - `status`: success, failed
- Example:
  ```
  titananvil_gitops_rollbacks_total{type="argocd",status="success"} 10
  ```

**titananvil_gitops_rollback_duration_seconds**
- Type: Summary
- Description: Rollback duration
- Labels:
  - `quantile`: 0.5, 0.95, 0.99
- Example:
  ```
  titananvil_gitops_rollback_duration_seconds{quantile="0.95"} 30
  ```

### Promotions

**titananvil_gitops_promotions_total**
- Type: Counter
- Description: Environment promotions
- Labels:
  - `pipeline`: Pipeline name
  - `status`: success, failed
- Example:
  ```
  titananvil_gitops_promotions_total{pipeline="myapp",status="success"} 25
  ```

### Git Sync

**titananvil_gitops_sync_total**
- Type: Counter
- Description: Git repository syncs
- Labels:
  - `repository`: Repository name
  - `status`: success, failed
- Example:
  ```
  titananvil_gitops_sync_total{repository="infrastructure-config",status="success"} 1000
  ```

**titananvil_gitops_sync_duration_seconds**
- Type: Summary
- Description: Git sync duration
- Labels:
  - `quantile`: 0.5, 0.95, 0.99
- Example:
  ```
  titananvil_gitops_sync_duration_seconds{quantile="0.95"} 2.0
  ```

## Query Examples

### Agent Monitoring

**Agent availability**:
```promql
100 * sum(titananvil_agents_connected_total) /
      sum(titananvil_agents_connected_total + titananvil_agents_disconnected_total)
```

**High CPU agents**:
```promql
titananvil_agent_cpu_usage_percent > 80
```

**Low memory agents**:
```promql
titananvil_agent_memory_usage_bytes /
titananvil_agent_memory_total_bytes > 0.9
```

### Command Execution

**Command success rate**:
```promql
100 * sum(rate(titananvil_commands_executed_total{status="success"}[5m])) /
      sum(rate(titananvil_commands_executed_total[5m]))
```

**P95 command latency**:
```promql
titananvil_command_duration_seconds{quantile="0.95"}
```

**Commands per second**:
```promql
sum(rate(titananvil_commands_executed_total[1m]))
```

### State Management

**State application success rate**:
```promql
100 * sum(rate(titananvil_state_applications_total{status="success"}[5m])) /
      sum(rate(titananvil_state_applications_total[5m]))
```

**Drift by severity**:
```promql
sum(increase(titananvil_state_drift_detected_total[1h])) by (severity)
```

**Resources per module**:
```promql
sum(titananvil_state_resources_total) by (module)
```

### Event System

**Event rate**:
```promql
sum(rate(titananvil_events_published_total[1m]))
```

**Events by type**:
```promql
sum(increase(titananvil_events_published_total[1h])) by (type)
```

**Event lag**:
```promql
titananvil_event_lag_seconds
```

**Reactor success rate**:
```promql
100 * sum(rate(titananvil_reactor_executions_total[5m])) /
      (sum(rate(titananvil_reactor_executions_total[5m])) +
       sum(rate(titananvil_reactor_failures_total[5m])))
```

### Policy Compliance

**Overall compliance score**:
```promql
avg(titananvil_policy_compliance_score)
```

**Violations by severity**:
```promql
sum(increase(titananvil_policy_violations_total[24h])) by (severity)
```

**Top violated policies**:
```promql
topk(10, sum(increase(titananvil_policy_violations_total[24h])) by (policy))
```

### GitOps

**Webhook failure rate**:
```promql
100 * sum(rate(titananvil_gitops_webhooks_failed_total[5m])) /
      sum(rate(titananvil_gitops_webhooks_received_total[5m]))
```

**Verification success rate**:
```promql
100 * sum(rate(titananvil_gitops_verifications_total{status="success"}[5m])) /
      sum(rate(titananvil_gitops_verifications_total[5m]))
```

**Rollback frequency**:
```promql
sum(increase(titananvil_gitops_rollbacks_total[24h]))
```

## Alert Examples

### Critical Alerts

**Control plane down**:
```yaml
alert: ControlPlaneDown
expr: up{job="titananvil-server"} == 0
for: 1m
severity: critical
```

**High agent churn**:
```yaml
alert: HighAgentChurn
expr: rate(titananvil_agents_disconnected_total[5m]) > 0.1
for: 5m
severity: critical
```

**Event processing lag**:
```yaml
alert: HighEventLag
expr: titananvil_event_lag_seconds > 10
for: 2m
severity: critical
```

### Warning Alerts

**High API latency**:
```yaml
alert: HighAPILatency
expr: titananvil_api_request_duration_seconds{quantile="0.95"} > 1.0
for: 5m
severity: warning
```

**Low agent availability**:
```yaml
alert: LowAgentAvailability
expr: |
  100 * sum(titananvil_agents_connected_total) /
  (sum(titananvil_agents_connected_total) + sum(titananvil_agents_disconnected_total)) < 80
for: 10m
severity: warning
```

**High drift detection**:
```yaml
alert: HighDriftRate
expr: rate(titananvil_state_drift_detected_total{severity=~"high|critical"}[5m]) > 0.05
for: 10m
severity: warning
```

## Prometheus Configuration

### Scrape Configuration

```yaml
scrape_configs:
  - job_name: 'titananvil-server'
    static_configs:
      - targets: ['control-plane:8080']
    scrape_interval: 15s
    scrape_timeout: 10s
```

### Recording Rules

```yaml
groups:
  - name: titananvil_aggregations
    interval: 1m
    rules:
      - record: titananvil:agent:availability
        expr: |
          100 * sum(titananvil_agents_connected_total) /
          (sum(titananvil_agents_connected_total) + sum(titananvil_agents_disconnected_total))

      - record: titananvil:command:success_rate
        expr: |
          100 * sum(rate(titananvil_commands_executed_total{status="success"}[5m])) /
          sum(rate(titananvil_commands_executed_total[5m]))

      - record: titananvil:events:rate
        expr: sum(rate(titananvil_events_published_total[1m]))
```

## See Also

- [Observability Concepts](../../concepts/observability/) - Observability overview
- [API Reference](../api/) - Metrics API endpoints
- [Configuration Reference](../configuration/#metrics) - Metrics configuration
- [Grafana Dashboards](../../operations/monitoring/#grafana-dashboards) - Pre-built dashboards
