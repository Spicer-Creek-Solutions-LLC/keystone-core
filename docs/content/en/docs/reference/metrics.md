---
title: "Metrics Reference"
weight: 6
description: >
  Complete Prometheus metrics catalog with labels, types, and query examples
---

## Overview

Keystone Core exposes Prometheus-compatible metrics for monitoring system health, performance, and operations. Metrics are collected across multiple subsystems, each with its own exporter.

**Metrics Endpoint**: `http://control-plane:8080/metrics` (when `metrics.enabled: true`)

**Metric Categories**:

- [Control Plane Metrics](#control-plane-metrics) - API, agents, commands, state, events, policy
- [Agent Metrics](#agent-metrics) - Per-agent resource and operation metrics
- [State Management Metrics](#state-management-metrics) - Resources, drift, changes
- [GitOps Metrics](#gitops-metrics) - Webhooks, deployments, rollbacks
- [Policy Metrics](#policy-metrics) - Evaluations, violations, compliance, remediations
- [Network Metrics](#network-metrics) - Listeners, connections, IP version
- [Cluster Metrics](#cluster-metrics) - Membership, leadership, rebalancing, etcd
- [Event System Metrics](#event-system-metrics) - Event processing, reactors, actions, storage
- [NATS Mesh Metrics](#nats-mesh-metrics) - Connections, messages, buffers, topology, delivery
- [File Mirror Metrics](#file-mirror-metrics) - Mirror groups, reads, writes, sync
- [File Distribution Metrics](#file-distribution-metrics) - Transfers, cache, backends
- [Proxy Agent Metrics](#proxy-agent-metrics) - Devices, commands, state, drift, discovery

## Metric Types

**Counter**: Cumulative value that only increases (use `rate()` or `increase()` in queries)
**Gauge**: Current value that can go up or down
**Histogram**: Distribution of observations (with configurable buckets)
**Summary**: Distribution with pre-calculated quantiles

## Control Plane Metrics

Source: `internal/metrics/collectors.go`

### API Server

**kscore_api_requests_total**

- Type: Counter
- Description: Total HTTP API requests
- Labels:
  - `method`: HTTP method (GET, POST, PUT, DELETE)
  - `endpoint`: API endpoint path
  - `status`: HTTP status code
- Example:

  ```
  kscore_api_requests_total{method="POST",endpoint="/api/v1/exec",status="200"} 1234
  ```

**kscore_api_request_duration_seconds**

- Type: Histogram
- Description: API request duration in seconds
- Labels:
  - `method`: HTTP method
  - `endpoint`: API endpoint path
- Buckets: default histogram buckets
- Example:

  ```
  kscore_api_request_duration_seconds_bucket{method="POST",endpoint="/api/v1/exec",le="0.1"} 1100
  kscore_api_request_duration_seconds_sum{method="POST",endpoint="/api/v1/exec"} 85.5
  kscore_api_request_duration_seconds_count{method="POST",endpoint="/api/v1/exec"} 1234
  ```

### Agent Connections

**kscore_agents_connected**

- Type: Gauge
- Description: Number of currently connected agents
- Labels:
  - `datacenter`: Agent datacenter
  - `role`: Agent role
- Example:

  ```
  kscore_agents_connected{datacenter="us-east-1",role="web"} 50
  ```

**kscore_agents_disconnected_total**

- Type: Counter
- Description: Total number of agent disconnections
- Example:

  ```
  kscore_agents_disconnected_total 25
  ```

### Command Execution

**kscore_command_executions_total**

- Type: Counter
- Description: Total number of command executions
- Labels:
  - `status`: Execution result (success, failed, timeout)
- Example:

  ```
  kscore_command_executions_total{status="success"} 5000
  ```

**kscore_command_execution_duration_seconds**

- Type: Histogram
- Description: Command execution duration in seconds
- Labels:
  - `status`: Execution result
- Buckets: default histogram buckets
- Example:

  ```
  kscore_command_execution_duration_seconds_bucket{status="success",le="1"} 4500
  kscore_command_execution_duration_seconds_sum{status="success"} 2500.5
  kscore_command_execution_duration_seconds_count{status="success"} 5000
  ```

### State Applications

**kscore_state_applications_total**

- Type: Counter
- Description: Total number of state applications
- Labels:
  - `status`: Application result (success, failed)
- Example:

  ```
  kscore_state_applications_total{status="success"} 1000
  ```

**kscore_state_application_duration_seconds**

- Type: Histogram
- Description: State application duration in seconds
- Labels:
  - `status`: Application result
- Buckets: default histogram buckets
- Example:

  ```
  kscore_state_application_duration_seconds_bucket{status="success",le="10"} 950
  kscore_state_application_duration_seconds_sum{status="success"} 5000.0
  kscore_state_application_duration_seconds_count{status="success"} 1000
  ```

### Policy Evaluations

**kscore_policy_evaluations_total**

- Type: Counter
- Description: Total number of policy evaluations
- Labels:
  - `policy`: Policy identifier
  - `result`: Evaluation result (allowed, denied)
- Example:

  ```
  kscore_policy_evaluations_total{policy="ssh-hardening",result="allowed"} 900
  ```

### Events (Control Plane)

**kscore_events_published_total**

- Type: Counter
- Description: Total number of events published
- Labels:
  - `type`: Event type
- Example:

  ```
  kscore_events_published_total{type="agent.connect"} 500
  ```

**kscore_events_processed_total**

- Type: Counter
- Description: Total number of events processed
- Labels:
  - `type`: Event type
- Example:

  ```
  kscore_events_processed_total{type="job.complete"} 1000
  ```

## Agent Metrics

Source: `internal/metrics/collectors.go`

Per-agent metrics reported by the control plane based on agent data.

**kscore_agent_heartbeat_seconds**

- Type: Gauge
- Description: Unix timestamp of last heartbeat from agent
- Labels:
  - `agent_id`: Agent identifier
- Example:

  ```
  kscore_agent_heartbeat_seconds{agent_id="web-01"} 1707667200
  ```

**kscore_agent_cpu_usage_percent**

- Type: Gauge
- Description: Agent CPU usage percentage
- Labels:
  - `agent_id`: Agent identifier
- Example:

  ```
  kscore_agent_cpu_usage_percent{agent_id="web-01"} 45.2
  ```

**kscore_agent_memory_usage_bytes**

- Type: Gauge
- Description: Agent memory usage in bytes
- Labels:
  - `agent_id`: Agent identifier
- Example:

  ```
  kscore_agent_memory_usage_bytes{agent_id="web-01"} 4294967296
  ```

**kscore_agent_disk_usage_bytes**

- Type: Gauge
- Description: Agent disk usage in bytes
- Labels:
  - `agent_id`: Agent identifier
- Example:

  ```
  kscore_agent_disk_usage_bytes{agent_id="web-01"} 21474836480
  ```

**kscore_agent_commands_executed_total**

- Type: Counter
- Description: Total number of commands executed on agent
- Labels:
  - `agent_id`: Agent identifier
  - `status`: Execution result (success, failed)
- Example:

  ```
  kscore_agent_commands_executed_total{agent_id="web-01",status="success"} 150
  ```

**kscore_agent_states_applied_total**

- Type: Counter
- Description: Total number of states applied on agent
- Labels:
  - `agent_id`: Agent identifier
  - `status`: Application result (success, failed)
- Example:

  ```
  kscore_agent_states_applied_total{agent_id="web-01",status="success"} 80
  ```

## State Management Metrics

Source: `internal/metrics/collectors.go`

### Resources

**kscore_state_resources_total**

- Type: Gauge
- Description: Total number of state resources under management
- Labels:
  - `type`: Resource type (file, package, service, etc.)
  - `status`: Resource status
- Example:

  ```
  kscore_state_resources_total{type="file",status="compliant"} 500
  kscore_state_resources_total{type="package",status="compliant"} 200
  ```

### Drift Detection

**kscore_state_drift_detected_total**

- Type: Counter
- Description: Total number of drift detections
- Labels:
  - `resource`: Resource identifier
- Example:

  ```
  kscore_state_drift_detected_total{resource="/etc/nginx/nginx.conf"} 3
  ```

### State Changes

**kscore_state_changes_applied_total**

- Type: Counter
- Description: Total number of state changes applied
- Labels:
  - `module`: State module name
- Example:

  ```
  kscore_state_changes_applied_total{module="file"} 150
  ```

## GitOps Metrics

Source: `internal/metrics/collectors.go`

**kscore_gitops_webhooks_received_total**

- Type: Counter
- Description: Total number of webhooks received
- Labels:
  - `source`: Webhook source (argocd, flux, github, gitlab)
- Example:

  ```
  kscore_gitops_webhooks_received_total{source="argocd"} 500
  ```

**kscore_gitops_deployments_verified_total**

- Type: Counter
- Description: Total number of deployments verified
- Labels:
  - `status`: Verification result (success, failed)
- Example:

  ```
  kscore_gitops_deployments_verified_total{status="success"} 450
  ```

**kscore_gitops_rollbacks_triggered_total**

- Type: Counter
- Description: Total number of rollbacks triggered
- Example:

  ```
  kscore_gitops_rollbacks_triggered_total 10
  ```

## Policy Metrics

Source: `internal/metrics/collectors.go`

**kscore_policy_violations_total**

- Type: Counter
- Description: Total number of policy violations
- Labels:
  - `policy`: Policy identifier
  - `severity`: Violation severity (low, medium, high, critical)
- Example:

  ```
  kscore_policy_violations_total{policy="ssh-hardening",severity="high"} 15
  ```

**kscore_policy_remediations_total**

- Type: Counter
- Description: Total number of policy remediations
- Labels:
  - `policy`: Policy identifier
  - `status`: Remediation result (success, failed)
- Example:

  ```
  kscore_policy_remediations_total{policy="ssh-hardening",status="success"} 10
  ```

**kscore_compliance_score**

- Type: Gauge
- Description: Compliance score by framework (0-100)
- Labels:
  - `framework`: Compliance framework name
- Example:

  ```
  kscore_compliance_score{framework="cis-benchmark"} 87.5
  ```

## Network Metrics

Source: `internal/metrics/collectors.go`

IPv6 and dual-stack networking metrics.

**kscore_listeners_active**

- Type: Gauge
- Description: Number of active network listeners
- Labels:
  - `protocol`: Network protocol (tcp, udp)
  - `ip_version`: IP version (v4, v6)
  - `port`: Listen port
- Example:

  ```
  kscore_listeners_active{protocol="tcp",ip_version="v6",port="4222"} 1
  ```

**kscore_connections_total**

- Type: Counter
- Description: Total number of connections established
- Labels:
  - `protocol`: Network protocol
  - `ip_version`: IP version
- Example:

  ```
  kscore_connections_total{protocol="tcp",ip_version="v4"} 5000
  ```

**kscore_connections_active**

- Type: Gauge
- Description: Number of currently active connections
- Labels:
  - `protocol`: Network protocol
  - `ip_version`: IP version
- Example:

  ```
  kscore_connections_active{protocol="tcp",ip_version="v4"} 42
  ```

**kscore_agents_by_ip_version**

- Type: Gauge
- Description: Number of agents by IP version
- Labels:
  - `ip_version`: IP version (v4, v6)
- Example:

  ```
  kscore_agents_by_ip_version{ip_version="v4"} 80
  kscore_agents_by_ip_version{ip_version="v6"} 20
  ```

## Cluster Metrics

Source: `internal/metrics/collectors.go`

### Membership

**kscore_cluster_members_total**

- Type: Gauge
- Description: Total number of cluster members
- Example:

  ```
  kscore_cluster_members_total 3
  ```

**kscore_cluster_members_healthy**

- Type: Gauge
- Description: Number of healthy cluster members
- Example:

  ```
  kscore_cluster_members_healthy 3
  ```

**kscore_cluster_member_status**

- Type: Gauge
- Description: Individual member status (1=healthy, 0.5=degraded, 0=unhealthy)
- Labels:
  - `member_id`: Member identifier
- Example:

  ```
  kscore_cluster_member_status{member_id="node-1"} 1
  ```

### Leadership

**kscore_cluster_is_leader**

- Type: Gauge
- Description: Whether this node is the cluster leader (1=leader, 0=follower)
- Example:

  ```
  kscore_cluster_is_leader 1
  ```

**kscore_cluster_has_quorum**

- Type: Gauge
- Description: Whether cluster has quorum (1=yes, 0=no)
- Example:

  ```
  kscore_cluster_has_quorum 1
  ```

**kscore_cluster_leader_changes_total**

- Type: Counter
- Description: Total number of leader changes
- Labels:
  - `reason`: Change reason
- Example:

  ```
  kscore_cluster_leader_changes_total{reason="election"} 5
  ```

**kscore_cluster_leader_election_duration_seconds**

- Type: Histogram
- Description: Leader election duration in seconds
- Buckets: default histogram buckets
- Example:

  ```
  kscore_cluster_leader_election_duration_seconds_bucket{le="0.1"} 4
  kscore_cluster_leader_election_duration_seconds_bucket{le="0.5"} 5
  kscore_cluster_leader_election_duration_seconds_sum 0.85
  kscore_cluster_leader_election_duration_seconds_count 5
  ```

### Rebalancing

**kscore_cluster_rebalance_total**

- Type: Counter
- Description: Total number of rebalance operations
- Labels:
  - `reason`: Rebalance reason
- Example:

  ```
  kscore_cluster_rebalance_total{reason="member_join"} 12
  ```

**kscore_cluster_rebalance_duration_seconds**

- Type: Histogram
- Description: Rebalance operation duration in seconds
- Buckets: default histogram buckets
- Example:

  ```
  kscore_cluster_rebalance_duration_seconds_bucket{le="1"} 10
  kscore_cluster_rebalance_duration_seconds_sum 8.5
  kscore_cluster_rebalance_duration_seconds_count 12
  ```

**kscore_cluster_agents_moved_total**

- Type: Counter
- Description: Total agents moved during rebalancing
- Example:

  ```
  kscore_cluster_agents_moved_total 45
  ```

### Health

**kscore_cluster_heartbeat_latency_seconds**

- Type: Summary
- Description: Inter-member heartbeat latency in seconds
- Labels:
  - `member_id`: Target member identifier
- Quantiles: 0.5, 0.9, 0.99
- Example:

  ```
  kscore_cluster_heartbeat_latency_seconds{member_id="node-2",quantile="0.5"} 0.002
  kscore_cluster_heartbeat_latency_seconds{member_id="node-2",quantile="0.99"} 0.015
  kscore_cluster_heartbeat_latency_seconds_sum{member_id="node-2"} 8.5
  kscore_cluster_heartbeat_latency_seconds_count{member_id="node-2"} 1000
  ```

### etcd Operations

**kscore_cluster_etcd_operations_total**

- Type: Counter
- Description: Total etcd operations
- Labels:
  - `operation`: Operation type (get, put, delete, txn)
  - `status`: Result status (success, failure)
- Example:

  ```
  kscore_cluster_etcd_operations_total{operation="put",status="success"} 5000
  ```

**kscore_cluster_etcd_operation_duration_seconds**

- Type: Histogram
- Description: etcd operation duration in seconds
- Labels:
  - `operation`: Operation type
- Buckets: default histogram buckets
- Example:

  ```
  kscore_cluster_etcd_operation_duration_seconds_bucket{operation="get",le="0.001"} 45000
  kscore_cluster_etcd_operation_duration_seconds_sum{operation="get"} 25.5
  kscore_cluster_etcd_operation_duration_seconds_count{operation="get"} 50000
  ```

## Event System Metrics

Source: `internal/events/prometheus.go`

The event subsystem exports its own metrics via a dedicated Prometheus exporter covering event lifecycle, reactors, actions, and storage.

### Event Lifecycle

**kscore_events_published_total**

- Type: Counter
- Description: Total number of events published by type
- Labels:
  - `type`: Event type
- Example:

  ```
  kscore_events_published_total{type="agent.connect"} 500
  ```

**kscore_events_received_total**

- Type: Counter
- Description: Total number of events received by type
- Labels:
  - `type`: Event type
- Example:

  ```
  kscore_events_received_total{type="agent.connect"} 500
  ```

**kscore_events_processed_total**

- Type: Counter
- Description: Total number of events processed by type
- Labels:
  - `type`: Event type
- Example:

  ```
  kscore_events_processed_total{type="job.complete"} 1000
  ```

**kscore_events_failed_total**

- Type: Counter
- Description: Total number of events that failed processing
- Labels:
  - `type`: Event type
- Example:

  ```
  kscore_events_failed_total{type="state.drift"} 5
  ```

**kscore_events_severity_total**

- Type: Counter
- Description: Total number of events by severity level
- Labels:
  - `severity`: Severity level (debug, info, warning, error, critical)
- Example:

  ```
  kscore_events_severity_total{severity="warning"} 250
  ```

### Publisher and Subscriber

**kscore_publisher_errors_total**

- Type: Counter
- Description: Total number of publisher errors
- Example:

  ```
  kscore_publisher_errors_total 3
  ```

**kscore_subscriber_errors_total**

- Type: Counter
- Description: Total number of subscriber errors
- Example:

  ```
  kscore_subscriber_errors_total 1
  ```

**kscore_active_subscribers**

- Type: Gauge
- Description: Number of active event subscribers
- Example:

  ```
  kscore_active_subscribers 12
  ```

### Reactors

**kscore_reactor_executions_total**

- Type: Counter
- Description: Total number of reactor executions
- Labels:
  - `reactor`: Reactor identifier
- Example:

  ```
  kscore_reactor_executions_total{reactor="auto_remediate_drift"} 50
  ```

**kscore_reactor_failures_total**

- Type: Counter
- Description: Total number of reactor failures
- Labels:
  - `reactor`: Reactor identifier
- Example:

  ```
  kscore_reactor_failures_total{reactor="auto_remediate_drift"} 2
  ```

**kscore_reactor_duration_seconds**

- Type: Summary
- Description: Reactor execution duration in seconds
- Labels:
  - `reactor`: Reactor identifier
- Quantiles: 0.5, 0.95, 0.99
- Example:

  ```
  kscore_reactor_duration_seconds{reactor="auto_remediate_drift",quantile="0.95"} 5.0
  kscore_reactor_duration_seconds_sum{reactor="auto_remediate_drift"} 120.5
  kscore_reactor_duration_seconds_count{reactor="auto_remediate_drift"} 50
  ```

### Actions

**kscore_action_executions_total**

- Type: Counter
- Description: Total number of reactor action executions
- Labels:
  - `type`: Action type (command, webhook, state, etc.)
  - `name`: Action name
- Example:

  ```
  kscore_action_executions_total{type="webhook",name="slack_notify"} 100
  ```

**kscore_action_failures_total**

- Type: Counter
- Description: Total number of reactor action failures
- Labels:
  - `type`: Action type
  - `name`: Action name
- Example:

  ```
  kscore_action_failures_total{type="webhook",name="slack_notify"} 3
  ```

### Storage

**kscore_storage_operations_total**

- Type: Counter
- Description: Total number of event storage operations
- Labels:
  - `operation`: Operation type (store, query, delete)
- Example:

  ```
  kscore_storage_operations_total{operation="store"} 500000
  ```

**kscore_storage_failures_total**

- Type: Counter
- Description: Total number of event storage failures
- Labels:
  - `operation`: Operation type
- Example:

  ```
  kscore_storage_failures_total{operation="store"} 10
  ```

### Processing

**kscore_event_processing_duration_seconds**

- Type: Summary
- Description: Event processing duration in seconds
- Quantiles: 0.5, 0.95, 0.99
- Example:

  ```
  kscore_event_processing_duration_seconds{quantile="0.5"} 0.001
  kscore_event_processing_duration_seconds{quantile="0.95"} 0.010
  kscore_event_processing_duration_seconds_sum 500.0
  kscore_event_processing_duration_seconds_count 100000
  ```

### System

**kscore_uptime_seconds**

- Type: Gauge
- Description: Event system uptime in seconds
- Example:

  ```
  kscore_uptime_seconds 86400.00
  ```

**kscore_event_rate**

- Type: Gauge
- Description: Current events per second
- Example:

  ```
  kscore_event_rate 15.50
  ```

**kscore_last_event_timestamp_seconds**

- Type: Gauge
- Description: Unix timestamp of last event received
- Example:

  ```
  kscore_last_event_timestamp_seconds 1707667200
  ```

## NATS Mesh Metrics

Source: `internal/nats/observability.go`

All NATS mesh metrics use the `kscore_nats` prefix.

### Connection Metrics

**kscore_nats_connections_total**

- Type: Counter
- Description: Total NATS connection attempts
- Labels:
  - `endpoint`: NATS server endpoint
  - `strategy`: Connection strategy (direct, tls, websocket, leaf)
  - `status`: Connection status (success, failed)
- Example:

  ```
  kscore_nats_connections_total{endpoint="nats://server1:4222",strategy="tls",status="success"} 150
  ```

**kscore_nats_connection_errors_total**

- Type: Counter
- Description: Total connection errors
- Labels:
  - `endpoint`: NATS server endpoint
  - `error`: Error type (timeout, auth_failed, tls_error, refused)
- Example:

  ```
  kscore_nats_connection_errors_total{endpoint="nats://server1:4222",error="timeout"} 5
  ```

**kscore_nats_reconnections_total**

- Type: Counter
- Description: Total reconnection events per endpoint
- Labels:
  - `endpoint`: NATS server endpoint
- Example:

  ```
  kscore_nats_reconnections_total{endpoint="nats://server1:4222"} 12
  ```

**kscore_nats_failovers_total**

- Type: Counter
- Description: Total endpoint failover events
- Labels:
  - `from`: Original endpoint
  - `to`: Failover endpoint
- Example:

  ```
  kscore_nats_failovers_total{from="nats://server1:4222",to="nats://server2:4222"} 3
  ```

### Message Metrics

**kscore_nats_messages_total**

- Type: Counter
- Description: Total messages by direction and subject prefix
- Labels:
  - `direction`: Message direction (sent, received)
  - `subject_prefix`: Subject prefix (kscore.agent, kscore.command, etc.)
- Example:

  ```
  kscore_nats_messages_total{direction="sent",subject_prefix="kscore.command"} 50000
  kscore_nats_messages_total{direction="received",subject_prefix="kscore.agent"} 100000
  ```

**kscore_nats_message_bytes_total**

- Type: Counter
- Description: Total message bytes by direction
- Labels:
  - `direction`: Message direction (sent, received)
- Example:

  ```
  kscore_nats_message_bytes_total{direction="sent"} 1073741824
  kscore_nats_message_bytes_total{direction="received"} 2147483648
  ```

### Buffer Metrics

**kscore_nats_buffer_size**

- Type: Gauge
- Description: Current buffer size for message buffering
- Labels:
  - `type`: Buffer type (outbound, inbound, leaf)
- Example:

  ```
  kscore_nats_buffer_size{type="outbound"} 1024
  ```

**kscore_nats_buffer_overflow_total**

- Type: Counter
- Description: Total buffer overflow events (messages dropped)
- Example:

  ```
  kscore_nats_buffer_overflow_total 15
  ```

### Topology Metrics

**kscore_nats_leaf_nodes_total**

- Type: Gauge
- Description: Connected leaf nodes per hub
- Labels:
  - `hub`: Hub server identifier
- Example:

  ```
  kscore_nats_leaf_nodes_total{hub="hub-us-east-1"} 25
  ```

**kscore_nats_gateway_connections_total**

- Type: Gauge
- Description: Active gateway connections between clusters
- Labels:
  - `local_cluster`: Local cluster name
  - `remote_cluster`: Remote cluster name
- Example:

  ```
  kscore_nats_gateway_connections_total{local_cluster="us-east",remote_cluster="us-west"} 3
  ```

### Delivery Metrics

**kscore_nats_delivery_pending**

- Type: Gauge
- Description: Current pending message deliveries
- Example:

  ```
  kscore_nats_delivery_pending 42
  ```

**kscore_nats_delivery_acked_total**

- Type: Counter
- Description: Total acknowledged message deliveries
- Example:

  ```
  kscore_nats_delivery_acked_total 500000
  ```

**kscore_nats_delivery_failed_total**

- Type: Counter
- Description: Total failed message deliveries
- Example:

  ```
  kscore_nats_delivery_failed_total 150
  ```

**kscore_nats_duplicates_detected_total**

- Type: Counter
- Description: Total duplicate messages detected and filtered
- Example:

  ```
  kscore_nats_duplicates_detected_total 25
  ```

### Bootstrap Metrics

**kscore_nats_bootstrap_requests_total**

- Type: Counter
- Description: Agent bootstrap registration requests
- Labels:
  - `status`: Request status (approved, rejected, expired)
- Example:

  ```
  kscore_nats_bootstrap_requests_total{status="approved"} 500
  ```

**kscore_nats_credentials_issued_total**

- Type: Counter
- Description: Total credentials issued to agents
- Labels:
  - `type`: Credential type (nkey, token, jwt)
- Example:

  ```
  kscore_nats_credentials_issued_total{type="nkey"} 450
  ```

### Coordination Metrics

**kscore_nats_coordination_rpcs_total**

- Type: Counter
- Description: Server-to-server coordination RPCs
- Labels:
  - `method`: RPC method name (ClusterHealth, GetLeader, NATSStatus, Heartbeat)
  - `status`: RPC status (success, failed)
- Example:

  ```
  kscore_nats_coordination_rpcs_total{method="ClusterHealth",status="success"} 10000
  ```

### Latency Metrics (via API)

The following latency metrics are tracked internally and available via the NATS metrics API (`GetStats()`). They provide histogram-style statistics (count, sum, min, max, P50, P95, P99).

- **Connection latency** (`connection_latency_seconds`): Per-endpoint connection establishment latency
- **Gateway latency** (`gateway_latency_seconds`): Cross-cluster gateway latency
- **Coordination latency** (`coordination_latency_seconds`): Per-method coordination RPC latency

## File Mirror Metrics

Source: `internal/files/mirror/metrics.go`

All file mirror metrics use the `kscore_mirror` prefix.

### Group Metrics

**kscore_mirror_groups_total**

- Type: Gauge
- Description: Total number of mirror groups configured
- Example:

  ```
  kscore_mirror_groups_total 3
  ```

**kscore_mirror_health**

- Type: Gauge
- Description: Mirror health status (1=healthy, 0=unhealthy)
- Labels:
  - `group`: Mirror group ID
  - `mirror`: Mirror ID
  - `state`: Health state (healthy, unhealthy, degraded, unknown)
- Example:

  ```
  kscore_mirror_health{group="us-east",mirror="mirror-1",state="healthy"} 1
  ```

### Read Metrics

**kscore_mirror_read_operations_total**

- Type: Counter
- Description: Total number of read operations
- Labels:
  - `group`: Mirror group ID

**kscore_mirror_read_bytes_total**

- Type: Counter
- Description: Total bytes read from mirrors
- Labels:
  - `group`: Mirror group ID

**kscore_mirror_read_errors_total**

- Type: Counter
- Description: Total read errors
- Labels:
  - `group`: Mirror group ID

**kscore_mirror_read_latency_seconds**

- Type: Histogram
- Description: Read latency distribution
- Labels:
  - `group`: Mirror group ID
- Buckets: 1ms, 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s

### Write Metrics

**kscore_mirror_write_operations_total**

- Type: Counter
- Description: Total number of write operations
- Labels:
  - `group`: Mirror group ID

**kscore_mirror_write_bytes_total**

- Type: Counter
- Description: Total bytes written to mirrors
- Labels:
  - `group`: Mirror group ID

**kscore_mirror_write_errors_total**

- Type: Counter
- Description: Total write errors
- Labels:
  - `group`: Mirror group ID

**kscore_mirror_write_latency_seconds**

- Type: Histogram
- Description: Write latency distribution
- Labels:
  - `group`: Mirror group ID
- Buckets: 1ms, 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s

### Sync Metrics

**kscore_mirror_sync_operations_total**

- Type: Counter
- Description: Total sync operations initiated
- Labels:
  - `group`: Mirror group ID

**kscore_mirror_sync_operations_active**

- Type: Gauge
- Description: Currently active sync operations
- Labels:
  - `group`: Mirror group ID

**kscore_mirror_sync_operations_succeeded_total**

- Type: Counter
- Description: Total successful sync operations
- Labels:
  - `group`: Mirror group ID

**kscore_mirror_sync_operations_failed_total**

- Type: Counter
- Description: Total failed sync operations
- Labels:
  - `group`: Mirror group ID

**kscore_mirror_sync_bytes_total**

- Type: Counter
- Description: Total bytes transferred during sync
- Labels:
  - `group`: Mirror group ID

**kscore_mirror_sync_files_total**

- Type: Counter
- Description: Total files synchronized
- Labels:
  - `group`: Mirror group ID

**kscore_mirror_sync_conflicts_total**

- Type: Counter
- Description: Total sync conflicts detected
- Labels:
  - `group`: Mirror group ID

**kscore_mirror_sync_latency_seconds**

- Type: Histogram
- Description: Sync operation latency distribution
- Labels:
  - `group`: Mirror group ID
- Buckets: 1ms, 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s

## File Distribution Metrics

Source: `internal/files/ha/metrics.go`

All file distribution metrics use the `kscore_files` prefix and include `instance` and `hostname` labels.

### Transfer Metrics

**kscore_files_transfers_total**

- Type: Counter
- Description: Total number of file transfers

**kscore_files_transfers_active**

- Type: Gauge
- Description: Current number of active transfers

**kscore_files_transfers_failed_total**

- Type: Counter
- Description: Total number of failed transfers

**kscore_files_bytes_transferred_total**

- Type: Counter
- Description: Total bytes transferred

**kscore_files_bytes_uploaded_total**

- Type: Counter
- Description: Total bytes uploaded

**kscore_files_bytes_downloaded_total**

- Type: Counter
- Description: Total bytes downloaded

**kscore_files_transfer_latency_seconds**

- Type: Histogram
- Description: Transfer latency distribution
- Buckets: 10ms, 50ms, 100ms, 500ms, 1s, 5s, 10s

### Cache Metrics

**kscore_files_cache_hits_total**

- Type: Counter
- Description: Total cache hits

**kscore_files_cache_misses_total**

- Type: Counter
- Description: Total cache misses

**kscore_files_cache_size_bytes**

- Type: Gauge
- Description: Current cache size in bytes

**kscore_files_cache_entries**

- Type: Gauge
- Description: Current number of cache entries

**kscore_files_cache_evictions_total**

- Type: Counter
- Description: Total cache evictions

### Backend Metrics

**kscore_files_backend_requests_total**

- Type: Counter
- Description: Total backend requests
- Labels:
  - `backend`: Backend identifier (s3, nats, local, etc.)

**kscore_files_backend_errors_total**

- Type: Counter
- Description: Total backend errors
- Labels:
  - `backend`: Backend identifier

**kscore_files_backend_latency_avg_seconds**

- Type: Gauge
- Description: Average backend latency in seconds
- Labels:
  - `backend`: Backend identifier

### Queue and Rate Limiting

**kscore_files_rate_limited_total**

- Type: Counter
- Description: Total rate-limited requests

**kscore_files_queued_transfers**

- Type: Gauge
- Description: Current number of queued transfers

## Proxy Agent Metrics

Source: `internal/proxy/observability/metrics.go`

All proxy metrics use the `kscore_proxy` prefix.

### Device Metrics

**kscore_proxy_devices_total**

- Type: Gauge
- Description: Total number of proxied devices

**kscore_proxy_devices_healthy**

- Type: Gauge
- Description: Number of healthy devices

**kscore_proxy_devices_degraded**

- Type: Gauge
- Description: Number of degraded devices

**kscore_proxy_devices_unhealthy**

- Type: Gauge
- Description: Number of unhealthy devices

**kscore_proxy_devices_unknown**

- Type: Gauge
- Description: Number of devices with unknown status

**kscore_proxy_devices_by_protocol**

- Type: Gauge
- Description: Devices by protocol
- Labels:
  - `protocol`: Protocol type (ssh, snmp, rest, winrm)

**kscore_proxy_devices_by_vendor**

- Type: Gauge
- Description: Devices by vendor
- Labels:
  - `vendor`: Vendor name

### Connection Metrics

**kscore_proxy_connections_total**

- Type: Counter
- Description: Total connection attempts

**kscore_proxy_connections_active**

- Type: Gauge
- Description: Active connections

**kscore_proxy_connections_failed**

- Type: Counter
- Description: Failed connections

**kscore_proxy_connection_latency_avg_ms**

- Type: Gauge
- Description: Average connection latency in milliseconds

### Command Metrics

**kscore_proxy_commands_total**

- Type: Counter
- Description: Total commands executed

**kscore_proxy_commands_succeeded**

- Type: Counter
- Description: Successful commands

**kscore_proxy_commands_failed**

- Type: Counter
- Description: Failed commands

**kscore_proxy_command_success_rate**

- Type: Gauge
- Description: Command success rate (percentage)

**kscore_proxy_command_latency_avg_ms**

- Type: Gauge
- Description: Average command latency in milliseconds

### Protocol-Specific Metrics

**kscore_proxy_ssh_commands_total**

- Type: Counter
- Description: Total SSH commands executed

**kscore_proxy_snmp_requests_total**

- Type: Counter
- Description: Total SNMP requests

**kscore_proxy_rest_requests_total**

- Type: Counter
- Description: Total REST requests

**kscore_proxy_winrm_commands_total**

- Type: Counter
- Description: Total WinRM commands executed

### State Metrics

**kscore_proxy_states_applied_total**

- Type: Counter
- Description: Total states applied to proxied devices

**kscore_proxy_states_succeeded_total**

- Type: Counter
- Description: Successful state applications

**kscore_proxy_states_failed_total**

- Type: Counter
- Description: Failed state applications

**kscore_proxy_states_changed_total**

- Type: Counter
- Description: States that made changes

**kscore_proxy_state_success_rate**

- Type: Gauge
- Description: State success rate (percentage)

**kscore_proxy_state_latency_avg_ms**

- Type: Gauge
- Description: Average state application latency in milliseconds

### Drift Metrics

**kscore_proxy_drift_checks_total**

- Type: Counter
- Description: Total drift checks performed

**kscore_proxy_drift_detected_total**

- Type: Counter
- Description: Total drift detections

**kscore_proxy_drift_by_severity_total**

- Type: Counter
- Description: Drift detections by severity
- Labels:
  - `severity`: Severity level

### Discovery Metrics

**kscore_proxy_discovery_scans_total**

- Type: Counter
- Description: Total discovery scans

**kscore_proxy_discovered_devices_total**

- Type: Counter
- Description: Total discovered devices

**kscore_proxy_approved_devices_total**

- Type: Counter
- Description: Total approved devices

**kscore_proxy_rejected_devices_total**

- Type: Counter
- Description: Total rejected devices

### Error Metrics

**kscore_proxy_errors_total**

- Type: Counter
- Description: Total errors by type
- Labels:
  - `type`: Error type

## Query Examples

### Agent Monitoring

**Connected agent count**:

```promql
sum(kscore_agents_connected)
```

**High CPU agents**:

```promql
kscore_agent_cpu_usage_percent > 80
```

**Agent heartbeat staleness** (agents not reporting for >60s):

```promql
time() - kscore_agent_heartbeat_seconds > 60
```

### Command Execution

**Command success rate**:

```promql
100 * sum(rate(kscore_command_executions_total{status="success"}[5m])) /
      sum(rate(kscore_command_executions_total[5m]))
```

**P95 command latency**:

```promql
histogram_quantile(0.95, sum(rate(kscore_command_execution_duration_seconds_bucket[5m])) by (le))
```

**Commands per second**:

```promql
sum(rate(kscore_command_executions_total[1m]))
```

### State Management

**State application success rate**:

```promql
100 * sum(rate(kscore_state_applications_total{status="success"}[5m])) /
      sum(rate(kscore_state_applications_total[5m]))
```

**Drift detections per hour**:

```promql
sum(increase(kscore_state_drift_detected_total[1h]))
```

**Resources per type**:

```promql
sum(kscore_state_resources_total) by (type)
```

### Event System

**Event rate**:

```promql
sum(rate(kscore_events_published_total[1m]))
```

**Events by type**:

```promql
sum(increase(kscore_events_published_total[1h])) by (type)
```

**Reactor success rate**:

```promql
100 * sum(rate(kscore_reactor_executions_total[5m])) /
      (sum(rate(kscore_reactor_executions_total[5m])) +
       sum(rate(kscore_reactor_failures_total[5m])))
```

### Policy Compliance

**Overall compliance score**:

```promql
avg(kscore_compliance_score)
```

**Violations by severity**:

```promql
sum(increase(kscore_policy_violations_total[24h])) by (severity)
```

**Top violated policies**:

```promql
topk(10, sum(increase(kscore_policy_violations_total[24h])) by (policy))
```

### GitOps

**Verification success rate**:

```promql
100 * sum(rate(kscore_gitops_deployments_verified_total{status="success"}[5m])) /
      sum(rate(kscore_gitops_deployments_verified_total[5m]))
```

**Rollback frequency**:

```promql
sum(increase(kscore_gitops_rollbacks_triggered_total[24h]))
```

### NATS Mesh

**Message throughput**:

```promql
sum(rate(kscore_nats_messages_total[1m])) by (direction)
```

**Delivery failure rate**:

```promql
rate(kscore_nats_delivery_failed_total[5m]) /
(rate(kscore_nats_delivery_acked_total[5m]) + rate(kscore_nats_delivery_failed_total[5m]))
```

### File Distribution

**Transfer error rate**:

```promql
rate(kscore_files_transfers_failed_total[5m]) /
rate(kscore_files_transfers_total[5m])
```

**Cache hit rate**:

```promql
rate(kscore_files_cache_hits_total[5m]) /
(rate(kscore_files_cache_hits_total[5m]) + rate(kscore_files_cache_misses_total[5m]))
```

### Proxy Agents

**Device health overview**:

```promql
kscore_proxy_devices_healthy / kscore_proxy_devices_total
```

**Proxy command success rate**:

```promql
kscore_proxy_command_success_rate
```

## Alert Examples

### Critical Alerts

**Control plane down**:

```yaml
alert: ControlPlaneDown
expr: up{job="kscore-server"} == 0
for: 1m
severity: critical
```

**High agent churn**:

```yaml
alert: HighAgentChurn
expr: rate(kscore_agents_disconnected_total[5m]) > 0.1
for: 5m
severity: critical
```

**Cluster quorum lost**:

```yaml
alert: ClusterQuorumLost
expr: kscore_cluster_has_quorum == 0
for: 1m
severity: critical
```

### Warning Alerts

**High API latency**:

```yaml
alert: HighAPILatency
expr: histogram_quantile(0.95, sum(rate(kscore_api_request_duration_seconds_bucket[5m])) by (le)) > 1.0
for: 5m
severity: warning
```

**High drift rate**:

```yaml
alert: HighDriftRate
expr: rate(kscore_state_drift_detected_total[5m]) > 0.05
for: 10m
severity: warning
```

**NATS delivery failures**:

```yaml
alert: NATSDeliveryFailures
expr: rate(kscore_nats_delivery_failed_total[5m]) > 0.01
for: 5m
severity: warning
```

**Low file cache hit rate**:

```yaml
alert: LowFileCacheHitRate
expr: >
  rate(kscore_files_cache_hits_total[5m]) /
  (rate(kscore_files_cache_hits_total[5m]) + rate(kscore_files_cache_misses_total[5m])) < 0.5
for: 15m
severity: warning
```

## Prometheus Configuration

### Scrape Configuration

```yaml
scrape_configs:
  - job_name: 'kscore-server'
    static_configs:
      - targets: ['control-plane:8080']
    scrape_interval: 15s
    scrape_timeout: 10s
```

### Recording Rules

```yaml
groups:
  - name: kscore_aggregations
    interval: 1m
    rules:
      - record: kscore:command:success_rate
        expr: |
          100 * sum(rate(kscore_command_executions_total{status="success"}[5m])) /
          sum(rate(kscore_command_executions_total[5m]))

      - record: kscore:events:rate
        expr: sum(rate(kscore_events_published_total[1m]))

      - record: kscore:compliance:avg
        expr: avg(kscore_compliance_score)
```

## See Also

- [Observability Concepts](../../concepts/observability/) - Observability overview
- [API Reference](../api/) - Metrics API endpoints
- [Configuration Reference](../configuration/#metrics) - Metrics configuration
- [Grafana Dashboards](../../operations/monitoring/#grafana-dashboards) - Pre-built dashboards
