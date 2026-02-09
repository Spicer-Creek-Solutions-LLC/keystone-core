---
title: "Metrics Reference"
weight: 6
description: >
  Complete Prometheus metrics catalog with labels, types, and query examples
---

## Overview

Keystone Core exposes 70+ Prometheus-compatible metrics for monitoring system health, performance, and operations.

**Metrics Endpoint**: `http://control-plane:8080/metrics`

**Metric Categories**:

- [Control Plane Metrics](#control-plane-metrics)
- [Cluster Metrics](#cluster-metrics)
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

**kscore_api_requests_total**

- Type: Counter
- Description: Total HTTP API requests
- Labels:
  - `method`: HTTP method (GET, POST, etc.)
  - `path`: API path
  - `status`: HTTP status code
- Example:

  ```
  kscore_api_requests_total{method="POST",path="/api/v1/exec",status="200"} 1234
  ```

**kscore_api_request_duration_seconds**

- Type: Summary
- Description: API request duration
- Labels:
  - `method`: HTTP method
  - `path`: API path
  - `quantile`: 0.5, 0.95, 0.99
- Example:

  ```
  kscore_api_request_duration_seconds{method="POST",path="/api/v1/exec",quantile="0.95"} 0.150
  ```

**kscore_api_active_connections**

- Type: Gauge
- Description: Current active connections
- Example:

  ```
  kscore_api_active_connections 42
  ```

### NATS Message Bus

**kscore_nats_messages_in_total**

- Type: Counter
- Description: Messages received from NATS
- Example:

  ```
  kscore_nats_messages_in_total 1000000
  ```

**kscore_nats_messages_out_total**

- Type: Counter
- Description: Messages sent to NATS
- Example:

  ```
  kscore_nats_messages_out_total 950000
  ```

**kscore_nats_bytes_in_total**

- Type: Counter
- Description: Bytes received from NATS
- Example:

  ```
  kscore_nats_bytes_in_total 10737418240
  ```

**kscore_nats_bytes_out_total**

- Type: Counter
- Description: Bytes sent to NATS
- Example:

  ```
  kscore_nats_bytes_out_total 9663676416
  ```

**kscore_nats_reconnections_total**

- Type: Counter
- Description: NATS reconnection count
- Example:

  ```
  kscore_nats_reconnections_total 5
  ```

### NATS Mesh (Epic 14)

These metrics are collected by the NATS mesh communication layer for monitoring connections, message delivery, topology, and reliability across the distributed NATS infrastructure.

#### Connection Metrics

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

**kscore_nats_connection_latency_seconds**

- Type: Histogram
- Description: Connection establishment latency
- Labels:
  - `endpoint`: NATS server endpoint
- Buckets: 1ms, 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s
- Example:

  ```
  kscore_nats_connection_latency_seconds_bucket{endpoint="nats://server1:4222",le="0.1"} 145
  kscore_nats_connection_latency_seconds_sum{endpoint="nats://server1:4222"} 12.5
  kscore_nats_connection_latency_seconds_count{endpoint="nats://server1:4222"} 150
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

#### Message Metrics

**kscore_nats_messages_total**

- Type: Counter
- Description: Total messages by direction and subject
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

#### Buffer Metrics

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

#### Topology Metrics

**kscore_nats_leaf_nodes_total**

- Type: Gauge
- Description: Connected leaf nodes per hub
- Labels:
  - `hub`: Hub server identifier
- Example:

  ```
  kscore_nats_leaf_nodes_total{hub="hub-us-east-1"} 25
  kscore_nats_leaf_nodes_total{hub="hub-us-west-2"} 18
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

**kscore_nats_gateway_latency_seconds**

- Type: Histogram
- Description: Cross-cluster gateway latency
- Labels:
  - `local_cluster`: Local cluster name
  - `remote_cluster`: Remote cluster name
- Buckets: 1ms, 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s
- Example:

  ```
  kscore_nats_gateway_latency_seconds_bucket{local_cluster="us-east",remote_cluster="us-west",le="0.1"} 950
  kscore_nats_gateway_latency_seconds_sum{local_cluster="us-east",remote_cluster="us-west"} 45.5
  kscore_nats_gateway_latency_seconds_count{local_cluster="us-east",remote_cluster="us-west"} 1000
  ```

#### Delivery Metrics

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

#### Bootstrap Metrics

**kscore_nats_bootstrap_requests_total**

- Type: Counter
- Description: Agent bootstrap registration requests
- Labels:
  - `status`: Request status (approved, rejected, expired)
- Example:

  ```
  kscore_nats_bootstrap_requests_total{status="approved"} 500
  kscore_nats_bootstrap_requests_total{status="rejected"} 15
  kscore_nats_bootstrap_requests_total{status="expired"} 8
  ```

**kscore_nats_credentials_issued_total**

- Type: Counter
- Description: Total credentials issued to agents
- Labels:
  - `type`: Credential type (nkey, token, jwt)
- Example:

  ```
  kscore_nats_credentials_issued_total{type="nkey"} 450
  kscore_nats_credentials_issued_total{type="token"} 50
  ```

#### Coordination Metrics

**kscore_nats_coordination_rpcs_total**

- Type: Counter
- Description: Server-to-server coordination RPCs
- Labels:
  - `method`: RPC method name (ClusterHealth, GetLeader, NATSStatus, Heartbeat)
  - `status`: RPC status (success, failed)
- Example:

  ```
  kscore_nats_coordination_rpcs_total{method="ClusterHealth",status="success"} 10000
  kscore_nats_coordination_rpcs_total{method="Heartbeat",status="success"} 50000
  ```

**kscore_nats_coordination_latency_seconds**

- Type: Histogram
- Description: Coordination RPC latency
- Labels:
  - `method`: RPC method name
- Buckets: 1ms, 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s
- Example:

  ```
  kscore_nats_coordination_latency_seconds_bucket{method="Heartbeat",le="0.01"} 49500
  kscore_nats_coordination_latency_seconds_sum{method="Heartbeat"} 250.5
  kscore_nats_coordination_latency_seconds_count{method="Heartbeat"} 50000
  ```

### File Mirror (Epic 22)

Metrics for the file distribution mirror system, including mirror health, read/write operations, synchronization, and conflicts.

#### Group Metrics

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
  kscore_mirror_health{group="us-east",mirror="mirror-2",state="degraded"} 0
  ```

#### Read Metrics

**kscore_mirror_read_operations_total**

- Type: Counter
- Description: Total number of read operations
- Labels:
  - `group`: Mirror group ID
- Example:

  ```
  kscore_mirror_read_operations_total{group="us-east"} 150000
  ```

**kscore_mirror_read_bytes_total**

- Type: Counter
- Description: Total bytes read from mirrors
- Labels:
  - `group`: Mirror group ID
- Example:

  ```
  kscore_mirror_read_bytes_total{group="us-east"} 1073741824
  ```

**kscore_mirror_read_errors_total**

- Type: Counter
- Description: Total read errors
- Labels:
  - `group`: Mirror group ID
- Example:

  ```
  kscore_mirror_read_errors_total{group="us-east"} 15
  ```

**kscore_mirror_read_latency_seconds**

- Type: Histogram
- Description: Read latency distribution
- Labels:
  - `group`: Mirror group ID
- Buckets: 1ms, 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s
- Example:

  ```
  kscore_mirror_read_latency_seconds_bucket{group="us-east",le="0.01"} 140000
  kscore_mirror_read_latency_seconds_bucket{group="us-east",le="0.1"} 149000
  kscore_mirror_read_latency_seconds_bucket{group="us-east",le="+Inf"} 150000
  ```

#### Write Metrics

**kscore_mirror_write_operations_total**

- Type: Counter
- Description: Total number of write operations
- Labels:
  - `group`: Mirror group ID
- Example:

  ```
  kscore_mirror_write_operations_total{group="us-east"} 50000
  ```

**kscore_mirror_write_bytes_total**

- Type: Counter
- Description: Total bytes written to mirrors
- Labels:
  - `group`: Mirror group ID
- Example:

  ```
  kscore_mirror_write_bytes_total{group="us-east"} 536870912
  ```

**kscore_mirror_write_errors_total**

- Type: Counter
- Description: Total write errors
- Labels:
  - `group`: Mirror group ID
- Example:

  ```
  kscore_mirror_write_errors_total{group="us-east"} 5
  ```

**kscore_mirror_write_latency_seconds**

- Type: Histogram
- Description: Write latency distribution
- Labels:
  - `group`: Mirror group ID
- Buckets: 1ms, 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s
- Example:

  ```
  kscore_mirror_write_latency_seconds_bucket{group="us-east",le="0.1"} 45000
  kscore_mirror_write_latency_seconds_bucket{group="us-east",le="1.0"} 49500
  kscore_mirror_write_latency_seconds_bucket{group="us-east",le="+Inf"} 50000
  ```

#### Sync Metrics

**kscore_mirror_sync_operations_total**

- Type: Counter
- Description: Total number of sync operations initiated
- Labels:
  - `group`: Mirror group ID
- Example:

  ```
  kscore_mirror_sync_operations_total{group="us-east"} 1000
  ```

**kscore_mirror_sync_operations_active**

- Type: Gauge
- Description: Currently active sync operations
- Labels:
  - `group`: Mirror group ID
- Example:

  ```
  kscore_mirror_sync_operations_active{group="us-east"} 2
  ```

**kscore_mirror_sync_operations_succeeded_total**

- Type: Counter
- Description: Total successful sync operations
- Labels:
  - `group`: Mirror group ID
- Example:

  ```
  kscore_mirror_sync_operations_succeeded_total{group="us-east"} 995
  ```

**kscore_mirror_sync_operations_failed_total**

- Type: Counter
- Description: Total failed sync operations
- Labels:
  - `group`: Mirror group ID
- Example:

  ```
  kscore_mirror_sync_operations_failed_total{group="us-east"} 5
  ```

**kscore_mirror_sync_bytes_total**

- Type: Counter
- Description: Total bytes transferred during sync
- Labels:
  - `group`: Mirror group ID
- Example:

  ```
  kscore_mirror_sync_bytes_total{group="us-east"} 10737418240
  ```

**kscore_mirror_sync_files_total**

- Type: Counter
- Description: Total files synchronized
- Labels:
  - `group`: Mirror group ID
- Example:

  ```
  kscore_mirror_sync_files_total{group="us-east"} 25000
  ```

**kscore_mirror_sync_conflicts_total**

- Type: Counter
- Description: Total sync conflicts detected
- Labels:
  - `group`: Mirror group ID
- Example:

  ```
  kscore_mirror_sync_conflicts_total{group="us-east"} 12
  ```

**kscore_mirror_sync_latency_seconds**

- Type: Histogram
- Description: Sync operation latency distribution
- Labels:
  - `group`: Mirror group ID
- Buckets: 1ms, 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s
- Example:

  ```
  kscore_mirror_sync_latency_seconds_bucket{group="us-east",le="1.0"} 800
  kscore_mirror_sync_latency_seconds_bucket{group="us-east",le="10.0"} 990
  kscore_mirror_sync_latency_seconds_bucket{group="us-east",le="+Inf"} 1000
  ```

### Database

**kscore_db_connections_active**

- Type: Gauge
- Description: Active database connections
- Example:

  ```
  kscore_db_connections_active 15
  ```

**kscore_db_connections_idle**

- Type: Gauge
- Description: Idle database connections
- Example:

  ```
  kscore_db_connections_idle 5
  ```

**kscore_db_query_duration_seconds**

- Type: Summary
- Description: Database query duration
- Labels:
  - `operation`: Query type (select, insert, update, delete)
  - `quantile`: 0.5, 0.95, 0.99
- Example:

  ```
  kscore_db_query_duration_seconds{operation="select",quantile="0.95"} 0.005
  ```

## Cluster Metrics

### Membership

**kscore_cluster_members_total**

- Type: Gauge
- Description: Total cluster members
- Example:

  ```
  kscore_cluster_members_total 3
  ```

**kscore_cluster_members_healthy**

- Type: Gauge
- Description: Healthy cluster members
- Example:

  ```
  kscore_cluster_members_healthy 3
  ```

**kscore_cluster_member_status**

- Type: Gauge
- Description: Individual member status (1=healthy, 0=unhealthy)
- Labels:
  - `member_id`: Member identifier
  - `address`: Member address
- Example:

  ```
  kscore_cluster_member_status{member_id="node-1",address="10.0.0.1:7000"} 1
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
- Description: Total leader elections
- Example:

  ```
  kscore_cluster_leader_changes_total 5
  ```

**kscore_cluster_leader_election_duration_seconds**

- Type: Histogram
- Description: Leader election duration
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
- Description: Total rebalance operations
- Example:

  ```
  kscore_cluster_rebalance_total 12
  ```

**kscore_cluster_rebalance_duration_seconds**

- Type: Histogram
- Description: Rebalance operation duration
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

- Type: Histogram
- Description: Inter-member heartbeat latency
- Labels:
  - `target`: Target member
- Example:

  ```
  kscore_cluster_heartbeat_latency_seconds_bucket{target="node-2",le="0.01"} 950
  kscore_cluster_heartbeat_latency_seconds_bucket{target="node-2",le="0.05"} 1000
  kscore_cluster_heartbeat_latency_seconds_sum{target="node-2"} 8.5
  kscore_cluster_heartbeat_latency_seconds_count{target="node-2"} 1000
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
  kscore_cluster_etcd_operations_total{operation="get",status="success"} 50000
  ```

**kscore_cluster_etcd_operation_duration_seconds**

- Type: Histogram
- Description: etcd operation duration
- Labels:
  - `operation`: Operation type (get, put, delete, txn)
- Example:

  ```
  kscore_cluster_etcd_operation_duration_seconds_bucket{operation="get",le="0.001"} 45000
  kscore_cluster_etcd_operation_duration_seconds_bucket{operation="get",le="0.01"} 50000
  kscore_cluster_etcd_operation_duration_seconds_sum{operation="get"} 25.5
  kscore_cluster_etcd_operation_duration_seconds_count{operation="get"} 50000
  ```

## Agent Metrics

### Agent Status

**kscore_agents_connected**

- Type: Gauge
- Description: Connected agents
- Labels:
  - `datacenter`: Agent datacenter
  - `environment`: Agent environment
  - `role`: Agent role
- Example:

  ```
  kscore_agents_connected{datacenter="us-east-1",environment="production",role="web"} 50
  ```

**kscore_agents_disconnected_total**

- Type: Counter
- Description: Total agent disconnections
- Labels:
  - `reason`: Disconnect reason (timeout, graceful, error)
- Example:

  ```
  kscore_agents_disconnected_total{reason="timeout"} 25
  ```

**kscore_agent_heartbeat_received_total**

- Type: Counter
- Description: Heartbeats received
- Example:

  ```
  kscore_agent_heartbeat_received_total 1000000
  ```

**kscore_agent_heartbeat_missed_total**

- Type: Counter
- Description: Missed heartbeats
- Example:

  ```
  kscore_agent_heartbeat_missed_total 150
  ```

### Agent Resources

**kscore_agent_cpu_usage_percent**

- Type: Gauge
- Description: Agent CPU usage
- Labels:
  - `agent_id`: Agent identifier
- Example:

  ```
  kscore_agent_cpu_usage_percent{agent_id="web-01"} 45.2
  ```

**kscore_agent_memory_usage_bytes**

- Type: Gauge
- Description: Agent memory usage
- Labels:
  - `agent_id`: Agent identifier
- Example:

  ```
  kscore_agent_memory_usage_bytes{agent_id="web-01"} 4294967296
  ```

**kscore_agent_memory_total_bytes**

- Type: Gauge
- Description: Agent total memory
- Labels:
  - `agent_id`: Agent identifier
- Example:

  ```
  kscore_agent_memory_total_bytes{agent_id="web-01"} 8589934592
  ```

**kscore_agent_disk_usage_bytes**

- Type: Gauge
- Description: Agent disk usage
- Labels:
  - `agent_id`: Agent identifier
  - `mount`: Mount point
- Example:

  ```
  kscore_agent_disk_usage_bytes{agent_id="web-01",mount="/"} 21474836480
  ```

**kscore_agent_disk_total_bytes**

- Type: Gauge
- Description: Agent total disk
- Labels:
  - `agent_id`: Agent identifier
  - `mount`: Mount point
- Example:

  ```
  kscore_agent_disk_total_bytes{agent_id="web-01",mount="/"} 107374182400
  ```

## Execution Metrics

### Commands

**kscore_command_executions_total**

- Type: Counter
- Description: Commands executed
- Labels:
  - `status`: success, failed, timeout
  - `datacenter`: Target datacenter
- Example:

  ```
  kscore_command_executions_total{status="success",datacenter="us-east-1"} 5000
  ```

**kscore_command_duration_seconds**

- Type: Summary
- Description: Command execution duration
- Labels:
  - `quantile`: 0.5, 0.95, 0.99
- Example:

  ```
  kscore_command_duration_seconds{quantile="0.95"} 2.5
  ```

**kscore_command_target_count**

- Type: Histogram
- Description: Number of targeted agents
- Labels:
  - `le`: Bucket upper bound
- Example:

  ```
  kscore_command_target_count_bucket{le="10"} 500
  kscore_command_target_count_bucket{le="50"} 800
  ```

### Batch Jobs

**kscore_batch_jobs_total**

- Type: Counter
- Description: Batch jobs executed
- Labels:
  - `status`: completed, failed
- Example:

  ```
  kscore_batch_jobs_total{status="completed"} 250
  ```

**kscore_batch_size**

- Type: Summary
- Description: Batch size distribution
- Labels:
  - `quantile`: 0.5, 0.95, 0.99
- Example:

  ```
  kscore_batch_size{quantile="0.95"} 50
  ```

## State Management Metrics

### State Applications

**kscore_state_applications_total**

- Type: Counter
- Description: State applications
- Labels:
  - `status`: success, failed
- Example:

  ```
  kscore_state_applications_total{status="success"} 1000
  ```

**kscore_state_application_duration_seconds**

- Type: Summary
- Description: State application duration
- Labels:
  - `quantile`: 0.5, 0.95, 0.99
- Example:

  ```
  kscore_state_application_duration_seconds{quantile="0.95"} 30
  ```

### Resources

**kscore_state_resources_total**

- Type: Gauge
- Description: Resources under management
- Labels:
  - `module`: State module (file, package, service, etc.)
- Example:

  ```
  kscore_state_resources_total{module="file"} 500
  kscore_state_resources_total{module="package"} 200
  ```

**kscore_state_changes_total**

- Type: Counter
- Description: State changes
- Labels:
  - `module`: State module
- Example:

  ```
  kscore_state_changes_total{module="file"} 150
  ```

### Drift Detection

**kscore_state_drift_detected_total**

- Type: Counter
- Description: Drift detections
- Labels:
  - `severity`: low, medium, high, critical
- Example:

  ```
  kscore_state_drift_detected_total{severity="high"} 25
  ```

## Event System Metrics

### Events

**kscore_events_published_total**

- Type: Counter
- Description: Events published
- Labels:
  - `type`: Event type
- Example:

  ```
  kscore_events_published_total{type="agent.connect"} 500
  ```

**kscore_events_processed_total**

- Type: Counter
- Description: Events processed
- Labels:
  - `type`: Event type
- Example:

  ```
  kscore_events_processed_total{type="job.complete"} 1000
  ```

**kscore_events_failed_total**

- Type: Counter
- Description: Event processing failures
- Labels:
  - `type`: Event type
- Example:

  ```
  kscore_events_failed_total{type="state.drift"} 5
  ```

**kscore_events_severity_total**

- Type: Counter
- Description: Events by severity
- Labels:
  - `severity`: debug, info, warning, error, critical
- Example:

  ```
  kscore_events_severity_total{severity="warning"} 250
  ```

### Event Processing

**kscore_event_processing_duration_seconds**

- Type: Summary
- Description: Event processing duration
- Labels:
  - `quantile`: 0.5, 0.95, 0.99
- Example:

  ```
  kscore_event_processing_duration_seconds{quantile="0.95"} 0.010
  ```

**kscore_event_lag_seconds**

- Type: Gauge
- Description: Event processing lag
- Example:

  ```
  kscore_event_lag_seconds 0.5
  ```

### Storage

**kscore_events_stored_total**

- Type: Counter
- Description: Events stored to database
- Example:

  ```
  kscore_events_stored_total 500000
  ```

**kscore_events_storage_errors_total**

- Type: Counter
- Description: Event storage errors
- Example:

  ```
  kscore_events_storage_errors_total 10
  ```

**kscore_events_count**

- Type: Gauge
- Description: Current event count
- Labels:
  - `type`: Event type
- Example:

  ```
  kscore_events_count{type="agent.connect"} 10000
  ```

### Reactors

**kscore_reactor_executions_total**

- Type: Counter
- Description: Reactor executions
- Labels:
  - `reactor`: Reactor name
- Example:

  ```
  kscore_reactor_executions_total{reactor="auto_remediate_drift"} 50
  ```

**kscore_reactor_failures_total**

- Type: Counter
- Description: Reactor failures
- Labels:
  - `reactor`: Reactor name
- Example:

  ```
  kscore_reactor_failures_total{reactor="auto_remediate_drift"} 2
  ```

**kscore_reactor_duration_seconds**

- Type: Summary
- Description: Reactor execution duration
- Labels:
  - `reactor`: Reactor name
  - `quantile`: 0.5, 0.95, 0.99
- Example:

  ```
  kscore_reactor_duration_seconds{reactor="auto_remediate_drift",quantile="0.95"} 5.0
  ```

**kscore_action_executions_total**

- Type: Counter
- Description: Reactor action executions
- Labels:
  - `type`: Action type (command, webhook, etc.)
  - `name`: Action name
- Example:

  ```
  kscore_action_executions_total{type="webhook",name="slack_notify"} 100
  ```

## Policy Metrics

### Evaluations

**kscore_policy_evaluations_total**

- Type: Counter
- Description: Policy evaluations
- Labels:
  - `policy`: Policy ID
  - `result`: allowed, denied
- Example:

  ```
  kscore_policy_evaluations_total{policy="ssh-hardening",result="allowed"} 900
  ```

**kscore_policy_evaluation_duration_seconds**

- Type: Summary
- Description: Policy evaluation duration
- Labels:
  - `policy`: Policy ID
  - `quantile`: 0.5, 0.95, 0.99
- Example:

  ```
  kscore_policy_evaluation_duration_seconds{policy="ssh-hardening",quantile="0.95"} 0.005
  ```

### Violations

**kscore_policy_violations_total**

- Type: Counter
- Description: Policy violations
- Labels:
  - `policy`: Policy ID
  - `severity`: low, medium, high, critical
- Example:

  ```
  kscore_policy_violations_total{policy="ssh-hardening",severity="high"} 15
  ```

**kscore_policy_violations_by_agent**

- Type: Gauge
- Description: Violations per agent
- Labels:
  - `agent`: Agent ID
  - `policy`: Policy ID
- Example:

  ```
  kscore_policy_violations_by_agent{agent="web-01",policy="ssh-hardening"} 2
  ```

### Compliance

**kscore_policy_compliance_score**

- Type: Gauge
- Description: Compliance score (0-100)
- Labels:
  - `policy_set`: Policy set ID
  - `environment`: Environment
- Example:

  ```
  kscore_policy_compliance_score{policy_set="security-baseline",environment="production"} 87.5
  ```

**kscore_policy_compliant_agents**

- Type: Gauge
- Description: Compliant agent count
- Labels:
  - `environment`: Environment
- Example:

  ```
  kscore_policy_compliant_agents{environment="production"} 45
  ```

### Remediations

**kscore_policy_remediations_total**

- Type: Counter
- Description: Policy remediations
- Labels:
  - `policy`: Policy ID
  - `status`: success, failed
- Example:

  ```
  kscore_policy_remediations_total{policy="ssh-hardening",status="success"} 10
  ```

## GitOps Metrics

### Webhooks

**kscore_gitops_webhooks_received_total**

- Type: Counter
- Description: Webhooks received
- Labels:
  - `source`: argocd, flux, github, gitlab
- Example:

  ```
  kscore_gitops_webhooks_received_total{source="argocd"} 500
  ```

**kscore_gitops_webhooks_failed_total**

- Type: Counter
- Description: Webhook processing failures
- Labels:
  - `source`: Webhook source
- Example:

  ```
  kscore_gitops_webhooks_failed_total{source="argocd"} 5
  ```

### Verifications

**kscore_gitops_verifications_total**

- Type: Counter
- Description: Deployment verifications
- Labels:
  - `status`: success, failed
- Example:

  ```
  kscore_gitops_verifications_total{status="success"} 450
  ```

**kscore_gitops_verification_duration_seconds**

- Type: Summary
- Description: Verification duration
- Labels:
  - `quantile`: 0.5, 0.95, 0.99
- Example:

  ```
  kscore_gitops_verification_duration_seconds{quantile="0.95"} 60
  ```

### Rollbacks

**kscore_gitops_rollbacks_total**

- Type: Counter
- Description: Rollbacks triggered
- Labels:
  - `type`: argocd, flux, git, manual
  - `status`: success, failed
- Example:

  ```
  kscore_gitops_rollbacks_total{type="argocd",status="success"} 10
  ```

**kscore_gitops_rollback_duration_seconds**

- Type: Summary
- Description: Rollback duration
- Labels:
  - `quantile`: 0.5, 0.95, 0.99
- Example:

  ```
  kscore_gitops_rollback_duration_seconds{quantile="0.95"} 30
  ```

### Promotions

**kscore_gitops_promotions_total**

- Type: Counter
- Description: Environment promotions
- Labels:
  - `pipeline`: Pipeline name
  - `status`: success, failed
- Example:

  ```
  kscore_gitops_promotions_total{pipeline="myapp",status="success"} 25
  ```

### Git Sync

**kscore_gitops_sync_total**

- Type: Counter
- Description: Git repository syncs
- Labels:
  - `repository`: Repository name
  - `status`: success, failed
- Example:

  ```
  kscore_gitops_sync_total{repository="infrastructure-config",status="success"} 1000
  ```

**kscore_gitops_sync_duration_seconds**

- Type: Summary
- Description: Git sync duration
- Labels:
  - `quantile`: 0.5, 0.95, 0.99
- Example:

  ```
  kscore_gitops_sync_duration_seconds{quantile="0.95"} 2.0
  ```

## Query Examples

### Agent Monitoring

**Agent availability**:

```promql
100 * sum(kscore_agents_connected) /
      sum(kscore_agents_connected + kscore_agents_disconnected_total)
```

**High CPU agents**:

```promql
kscore_agent_cpu_usage_percent > 80
```

**Low memory agents**:

```promql
kscore_agent_memory_usage_bytes /
kscore_agent_memory_total_bytes > 0.9
```

### Command Execution

**Command success rate**:

```promql
100 * sum(rate(kscore_command_executions_total{status="success"}[5m])) /
      sum(rate(kscore_command_executions_total[5m]))
```

**P95 command latency**:

```promql
kscore_command_duration_seconds{quantile="0.95"}
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

**Drift by severity**:

```promql
sum(increase(kscore_state_drift_detected_total[1h])) by (severity)
```

**Resources per module**:

```promql
sum(kscore_state_resources_total) by (module)
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

**Event lag**:

```promql
kscore_event_lag_seconds
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
avg(kscore_policy_compliance_score)
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

**Webhook failure rate**:

```promql
100 * sum(rate(kscore_gitops_webhooks_failed_total[5m])) /
      sum(rate(kscore_gitops_webhooks_received_total[5m]))
```

**Verification success rate**:

```promql
100 * sum(rate(kscore_gitops_verifications_total{status="success"}[5m])) /
      sum(rate(kscore_gitops_verifications_total[5m]))
```

**Rollback frequency**:

```promql
sum(increase(kscore_gitops_rollbacks_total[24h]))
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

**Event processing lag**:

```yaml
alert: HighEventLag
expr: kscore_event_lag_seconds > 10
for: 2m
severity: critical
```

### Warning Alerts

**High API latency**:

```yaml
alert: HighAPILatency
expr: kscore_api_request_duration_seconds{quantile="0.95"} > 1.0
for: 5m
severity: warning
```

**Low agent availability**:

```yaml
alert: LowAgentAvailability
expr: |
  100 * sum(kscore_agents_connected) /
  (sum(kscore_agents_connected) + sum(kscore_agents_disconnected_total)) < 80
for: 10m
severity: warning
```

**High drift detection**:

```yaml
alert: HighDriftRate
expr: rate(kscore_state_drift_detected_total{severity=~"high|critical"}[5m]) > 0.05
for: 10m
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
      - record: kscore:agent:availability
        expr: |
          100 * sum(kscore_agents_connected) /
          (sum(kscore_agents_connected) + sum(kscore_agents_disconnected_total))

      - record: kscore:command:success_rate
        expr: |
          100 * sum(rate(kscore_command_executions_total{status="success"}[5m])) /
          sum(rate(kscore_command_executions_total[5m]))

      - record: kscore:events:rate
        expr: sum(rate(kscore_events_published_total[1m]))
```

## See Also

- [Observability Concepts](../../concepts/observability/) - Observability overview
- [API Reference](../api/) - Metrics API endpoints
- [Configuration Reference](../configuration/#metrics) - Metrics configuration
- [Grafana Dashboards](../../operations/monitoring/#grafana-dashboards) - Pre-built dashboards
