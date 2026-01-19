---
title: "NATS Mesh Operations"
weight: 3
description: >
  Monitoring, troubleshooting, and maintaining NATS mesh deployments
---

## Overview

This guide covers day-to-day operations for NATS mesh deployments, including monitoring, troubleshooting, capacity planning, and disaster recovery.

## Monitoring

### Grafana Dashboard

Import the NATS Mesh dashboard from `deploy/grafana/dashboards/nats-mesh.json`:

```bash
# Using Grafana API
curl -X POST http://admin:admin@grafana:3000/api/dashboards/db \
  -H "Content-Type: application/json" \
  -d @deploy/grafana/dashboards/nats-mesh.json
```

The dashboard provides:
- **Overview**: Connection health, message rates, error rates
- **Connections**: Success rates, latency percentiles, circuit breaker states
- **Messages**: Throughput, delivery status, duplicate rates
- **Buffers**: Utilization, overflow rates
- **Topology**: Leaf nodes, gateways, cluster health
- **Reliability**: Circuit breakers, degradation modes

### Key Metrics to Monitor

#### Connection Health

| Metric | Alert Threshold | Description |
|--------|-----------------|-------------|
| `kscore_nats_connections_total{status="success"}` | > 0 | Active connections |
| `kscore_nats_connection_errors_total` | Rate > 0.1/s | Connection failures |
| `kscore_nats_reconnections_total` | Rate > 5/min | Reconnection frequency |
| `kscore_nats_connection_latency_seconds` | P95 > 500ms | Connection latency |

#### Message Delivery

| Metric | Alert Threshold | Description |
|--------|-----------------|-------------|
| `kscore_nats_delivery_acked_total` | Increasing | Successful deliveries |
| `kscore_nats_delivery_failed_total` | Rate > 0.05 | Failed deliveries |
| `kscore_nats_delivery_pending` | > 100 | Pending messages |
| `kscore_nats_duplicates_detected_total` | Rate > 10% | Duplicate messages |

#### Buffer Status

| Metric | Alert Threshold | Description |
|--------|-----------------|-------------|
| `kscore_nats_buffer_size` | > 80% capacity | Buffer utilization |
| `kscore_nats_buffer_overflow_total` | Any increase | Message loss |

#### Topology Health

| Metric | Alert Threshold | Description |
|--------|-----------------|-------------|
| `kscore_nats_leaf_nodes_total` | < expected | Leaf node count |
| `kscore_nats_gateway_connections_total` | < expected | Gateway connections |
| `kscore_nats_gateway_latency_seconds` | P95 > 1s | Cross-cluster latency |

### Prometheus Alert Rules

Deploy alerts from `deploy/grafana/alerts/nats-mesh-alerts.yml`:

```yaml
# Critical alerts
- alert: NATSNoActiveConnections
  expr: sum(kscore_nats_connections_total{status="success"}) == 0
  for: 1m
  labels:
    severity: critical
  annotations:
    summary: "No active NATS connections"

- alert: NATSCircuitBreakerOpen
  expr: kscore_nats_circuit_breaker_state == 2
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "Circuit breaker open for {{ $labels.endpoint }}"

- alert: NATSBufferOverflow
  expr: rate(kscore_nats_buffer_overflow_total[5m]) > 0
  for: 1m
  labels:
    severity: warning
  annotations:
    summary: "Message buffer overflow detected"
```

### Health Checks

#### Control Plane Health

```bash
# Check NATS connectivity
curl http://localhost:8080/health/nats

# Response
{
  "status": "healthy",
  "connections": {
    "active": 3,
    "endpoints": [
      {"url": "nats://nats-1:4222", "status": "connected"},
      {"url": "nats://nats-2:4222", "status": "connected"},
      {"url": "nats://nats-3:4222", "status": "connected"}
    ]
  },
  "latency_ms": 2
}
```

#### Agent Health

```bash
# Check agent NATS status
kscore-agent nats status

# Output
NATS Connection Status
  Endpoint: nats://nats-1:4222
  State: connected
  Uptime: 24h 15m 32s
  Messages Sent: 12,456
  Messages Received: 8,234
  Last Heartbeat: 2s ago
```

## Troubleshooting

### Connection Issues

#### Symptom: Agent Cannot Connect

```bash
# Check DNS resolution
nslookup nats.example.com

# Test TCP connectivity
nc -zv nats.example.com 4222

# Test NATS protocol
nats server ping nats://nats.example.com:4222
```

**Common causes:**
- Firewall blocking port 4222
- DNS resolution failure
- NATS server not running
- TLS certificate mismatch

**Solutions:**
```bash
# Use WebSocket if TCP blocked
agent:
  nats:
    urls: ["wss://nats.example.com:443/nats"]

# Check certificate
openssl s_client -connect nats.example.com:4222 -servername nats.example.com
```

#### Symptom: Frequent Reconnections

```bash
# Check reconnection events
kscorectl debug nats events --type reconnect --limit 50

# Check connection timeline
kscorectl debug nats timeline --endpoint nats://nats-1:4222
```

**Common causes:**
- Network instability
- NATS server overloaded
- Keepalive timeout too aggressive
- Load balancer idle timeout

**Solutions:**
```yaml
agent:
  nats:
    ping_interval: 30s
    max_reconnects: -1  # Unlimited
    reconnect_wait: 2s
    reconnect_jitter: 1s
```

#### Symptom: High Latency

```bash
# Check latency metrics
curl -s http://localhost:8080/metrics | grep nats_connection_latency

# Test endpoint latency
nats bench test.latency --msgs 1000 --pub 1 --sub 1
```

**Common causes:**
- Wrong region/endpoint
- Network congestion
- NATS server CPU saturation

**Solutions:**
```yaml
agent:
  nats:
    routing:
      strategy: least_latency
    endpoints:
      - url: nats://nats-local:4222
        priority: 1
      - url: nats://nats-remote:4222
        priority: 10
```

### Message Delivery Issues

#### Symptom: Messages Not Delivered

```bash
# Check delivery metrics
curl -s http://localhost:8080/metrics | grep nats_delivery

# Trace specific message
kscorectl debug nats trace --message-id msg-12345
```

**Common causes:**
- Consumer offline
- Subject mismatch
- JetStream stream full
- Dead letter queue full

**Solutions:**
```bash
# Check JetStream stream status
nats stream info KSCORE_COMMANDS

# Check consumer status
nats consumer info KSCORE_COMMANDS agent-consumer

# Replay from stream
nats consumer next KSCORE_COMMANDS agent-consumer --count 10
```

#### Symptom: Duplicate Messages

```bash
# Check duplicate rate
curl -s http://localhost:8080/metrics | grep duplicates_detected
```

**Common causes:**
- Network retries
- JetStream redelivery
- Deduplication window too small

**Solutions:**
```yaml
agent:
  nats:
    dedup:
      window: 5m  # Increase window
      max_entries: 100000
```

### Buffer Issues

#### Symptom: Buffer Overflow

```bash
# Check buffer status
kscore-agent nats buffer status

# Output
Buffer Status
  Mode: leaf
  State: buffering
  Messages: 45,234 / 100,000
  Size: 78 MB / 100 MB
  Oldest Message: 2h ago
  Flush Pending: true
```

**Solutions:**
```yaml
agent:
  nats:
    buffer:
      max_size: 500MB
      max_messages: 500000
      overflow: drop_oldest  # or block
      persistence: true
```

### Leaf Node Issues

#### Symptom: Leaf Not Connecting

```bash
# Check leaf configuration
kscore-agent nats leaf status

# Check hub logs
docker logs nats-hub 2>&1 | grep -i leaf
```

**Common causes:**
- Hub leaf port not open (7422)
- Credentials mismatch
- TLS verification failure

**Solutions:**
```bash
# Test leaf connection directly
nats-server -c /etc/nats/leaf.conf --debug

# Verify credentials
nats server check connection --creds /etc/kscore/leaf.creds
```

### Gateway Issues

#### Symptom: Cross-Cluster Messages Failing

```bash
# Check gateway status
nats server report gateways

# Check gateway metrics
curl -s http://localhost:8080/metrics | grep gateway
```

**Common causes:**
- Gateway port not reachable
- Interest-only mode filtering
- Clock skew between clusters

**Solutions:**
```yaml
# Use optimistic mode for all subjects
gateway:
  gateways:
    - name: other-cluster
      mode: optimistic  # Not interest_only
```

### Debug Commands

```bash
# Enable debug logging
export KSCORE_NATS_DEBUG=true
kscore-agent restart

# Generate diagnostic report
kscorectl debug nats diagnose > nats-diagnostic.json

# Export connection events
kscorectl debug nats events --export json > events.json

# Trace message flow
kscorectl debug nats trace --subject "kscore.prod.command.*" --duration 5m
```

## Capacity Planning

### Connection Sizing

| Agents | NATS Servers | Memory per Server | JetStream Storage |
|--------|--------------|-------------------|-------------------|
| < 100 | 1 (embedded) | 512MB | 1GB |
| 100-500 | 3 | 2GB | 10GB |
| 500-2000 | 3-5 | 4GB | 50GB |
| 2000-10000 | 5+ | 8GB | 100GB+ |

### Message Throughput

Estimate based on:
- Heartbeats: 1 msg/30s per agent
- Commands: Variable (1-100/min per agent)
- Events: Variable (0-1000/min per agent)
- State: 1-10 msgs per state application

```
Messages/sec = (agents/30) + (cmd_rate * agents) + (event_rate * agents)
```

### Buffer Sizing (Leaf Nodes)

```
Buffer Size = (msg_rate * avg_msg_size * max_offline_duration)

Example:
  10 msg/sec * 1KB * 3600 sec = 36MB minimum
  Add 2x safety margin = 72MB recommended
```

### Network Bandwidth

```
Bandwidth = (messages/sec * avg_message_size * 2) + overhead

Example:
  1000 msg/sec * 1KB * 2 (send+receive) = 2 MB/s
  Add 20% overhead = 2.4 MB/s per NATS server
```

## Performance Tuning

### NATS Server Tuning

```conf
# nats.conf
max_connections: 10000
max_payload: 1MB
max_pending_size: 64MB

jetstream {
  max_memory_store: 4GB
  max_file_store: 100GB
  sync_interval: 1m
}

# For high-throughput
write_deadline: 10s
```

### Agent Tuning

```yaml
agent:
  nats:
    # Connection pool
    pool_size: 3

    # Flush settings
    flush_timeout: 10s
    max_pending: 64MB

    # Reconnection
    reconnect_buf_size: 8MB
    reconnect_jitter: 500ms
```

### OS Tuning

```bash
# Increase file descriptors
echo "* soft nofile 65535" >> /etc/security/limits.conf
echo "* hard nofile 65535" >> /etc/security/limits.conf

# Increase TCP buffers
sysctl -w net.core.rmem_max=16777216
sysctl -w net.core.wmem_max=16777216
sysctl -w net.ipv4.tcp_rmem="4096 87380 16777216"
sysctl -w net.ipv4.tcp_wmem="4096 65536 16777216"
```

## Disaster Recovery

### Backup Strategy

#### JetStream Streams

```bash
# Backup streams
nats stream backup KSCORE_COMMANDS /backup/streams/commands.tar.gz
nats stream backup KSCORE_EVENTS /backup/streams/events.tar.gz

# Backup all streams
for stream in $(nats stream ls -n); do
  nats stream backup $stream /backup/streams/${stream}.tar.gz
done
```

#### Configuration

```bash
# Backup NATS config
cp -r /etc/nats /backup/nats-config/

# Backup agent credentials
cp -r /etc/kscore/creds /backup/kscore-creds/
```

### Recovery Procedures

#### Single NATS Server Failure

1. **Remaining servers continue operating** (quorum maintained)
2. **Replace failed server**:
   ```bash
   # Start new NATS server with same config
   nats-server -c /etc/nats/nats.conf
   ```
3. **Server automatically joins cluster** via routes

#### Complete Cluster Failure

1. **Start first NATS server**:
   ```bash
   nats-server -c /etc/nats/nats.conf
   ```

2. **Restore JetStream data** (if available):
   ```bash
   nats stream restore KSCORE_COMMANDS /backup/streams/commands.tar.gz
   ```

3. **Start remaining servers**:
   ```bash
   # Other servers will sync from first
   nats-server -c /etc/nats/nats.conf
   ```

4. **Verify cluster**:
   ```bash
   nats server report jetstream
   ```

5. **Agents reconnect automatically**

#### Supercluster Recovery

1. **Recover each regional cluster independently**
2. **Verify gateway connectivity**:
   ```bash
   nats server report gateways
   ```
3. **Resume cross-cluster operations**

### Runbook: Connection Storm

When many agents reconnect simultaneously:

1. **Enable rate limiting on NATS**:
   ```yaml
   max_connections: 1000
   max_control_line: 4KB
   ```

2. **Stagger agent reconnections**:
   ```yaml
   agent:
     nats:
       reconnect_jitter: 5s  # Random 0-5s delay
   ```

3. **Monitor connection rate**:
   ```bash
   watch 'nats server report connections | head -20'
   ```

### Runbook: Split Brain (Supercluster)

When gateway partitions occur:

1. **Identify partition**:
   ```bash
   nats server report gateways
   ```

2. **Check agent distribution**:
   ```bash
   kscorectl agent list --group-by cluster
   ```

3. **Wait for automatic recovery** (gateways reconnect)

4. **If manual intervention needed**:
   ```bash
   # Restart gateway on one side
   nats-server --signal reload
   ```

5. **Verify message reconciliation**:
   ```bash
   # Check for duplicate processing
   kscorectl debug nats events --type duplicate --last 1h
   ```

## Maintenance

### Rolling Updates

#### NATS Cluster

```bash
# Update one server at a time
for server in nats-1 nats-2 nats-3; do
  # Drain connections
  nats server request $server drain

  # Wait for drain
  sleep 30

  # Update and restart
  systemctl restart nats-$server

  # Wait for rejoin
  sleep 30

  # Verify cluster health
  nats server report jetstream
done
```

#### Agents

```bash
# Rolling update via Kubernetes
kubectl rollout restart daemonset/kscore-agent

# Or with ansible
ansible-playbook -i inventory rolling-update-agents.yml
```

### Certificate Rotation

```bash
# Generate new certificates
./scripts/generate-certs.sh

# Update NATS servers (one at a time)
for server in nats-1 nats-2 nats-3; do
  scp new-certs/* $server:/etc/nats/certs/
  ssh $server "nats-server --signal reload"
  sleep 30
done

# Update agents
kscorectl agent update-certs --cert new-agent.crt --key new-agent.key
```

### Stream Maintenance

```bash
# Check stream health
nats stream report

# Purge old messages (if needed)
nats stream purge KSCORE_EVENTS --keep 1000000

# Compact storage
nats stream edit KSCORE_EVENTS --max-bytes 10GB
```

## NATS Metrics Dashboards

### Dashboard Overview

The following dashboards provide comprehensive visibility into NATS mesh health and performance.

### Connection Health Dashboard

Monitor connection status across all agents and NATS endpoints.

```json
{
  "title": "NATS Connection Health",
  "panels": [
    {
      "title": "Active Connections",
      "type": "stat",
      "targets": [{
        "expr": "sum(kscore_nats_connections_total{status=\"success\"})",
        "legendFormat": "Active"
      }],
      "fieldConfig": {
        "defaults": {
          "thresholds": {
            "mode": "absolute",
            "steps": [
              {"color": "red", "value": 0},
              {"color": "yellow", "value": 10},
              {"color": "green", "value": 50}
            ]
          }
        }
      }
    },
    {
      "title": "Connection Success Rate",
      "type": "gauge",
      "targets": [{
        "expr": "sum(rate(kscore_nats_connections_total{status=\"success\"}[5m])) / sum(rate(kscore_nats_connections_total[5m])) * 100"
      }],
      "fieldConfig": {
        "defaults": {
          "unit": "percent",
          "min": 0,
          "max": 100,
          "thresholds": {
            "steps": [
              {"color": "red", "value": 0},
              {"color": "yellow", "value": 95},
              {"color": "green", "value": 99}
            ]
          }
        }
      }
    },
    {
      "title": "Connection Latency by Endpoint",
      "type": "timeseries",
      "targets": [{
        "expr": "histogram_quantile(0.95, rate(kscore_nats_connection_latency_seconds_bucket[5m])) * 1000",
        "legendFormat": "P95 {{endpoint}}"
      }, {
        "expr": "histogram_quantile(0.50, rate(kscore_nats_connection_latency_seconds_bucket[5m])) * 1000",
        "legendFormat": "P50 {{endpoint}}"
      }],
      "fieldConfig": {"defaults": {"unit": "ms"}}
    },
    {
      "title": "Reconnections Rate",
      "type": "timeseries",
      "targets": [{
        "expr": "sum(rate(kscore_nats_reconnections_total[5m])) by (endpoint)",
        "legendFormat": "{{endpoint}}"
      }],
      "fieldConfig": {"defaults": {"unit": "ops"}}
    },
    {
      "title": "Connection Errors",
      "type": "timeseries",
      "targets": [{
        "expr": "sum(rate(kscore_nats_connection_errors_total[5m])) by (error_type)",
        "legendFormat": "{{error_type}}"
      }]
    },
    {
      "title": "Connections by Status",
      "type": "piechart",
      "targets": [{
        "expr": "sum(kscore_nats_connections_total) by (status)",
        "legendFormat": "{{status}}"
      }]
    }
  ]
}
```

### Message Throughput Dashboard

Track message rates, delivery status, and processing latency.

```json
{
  "title": "NATS Message Throughput",
  "panels": [
    {
      "title": "Messages Published/sec",
      "type": "stat",
      "targets": [{
        "expr": "sum(rate(kscore_nats_messages_published_total[5m]))"
      }],
      "fieldConfig": {"defaults": {"unit": "msg/s"}}
    },
    {
      "title": "Messages Received/sec",
      "type": "stat",
      "targets": [{
        "expr": "sum(rate(kscore_nats_messages_received_total[5m]))"
      }],
      "fieldConfig": {"defaults": {"unit": "msg/s"}}
    },
    {
      "title": "Message Rate by Subject",
      "type": "timeseries",
      "targets": [{
        "expr": "topk(10, sum(rate(kscore_nats_messages_published_total[5m])) by (subject))",
        "legendFormat": "{{subject}}"
      }],
      "fieldConfig": {"defaults": {"unit": "msg/s"}}
    },
    {
      "title": "Delivery Status",
      "type": "piechart",
      "targets": [{
        "expr": "sum(increase(kscore_nats_delivery_acked_total[1h]))",
        "legendFormat": "Acked"
      }, {
        "expr": "sum(increase(kscore_nats_delivery_failed_total[1h]))",
        "legendFormat": "Failed"
      }, {
        "expr": "sum(kscore_nats_delivery_pending)",
        "legendFormat": "Pending"
      }]
    },
    {
      "title": "Message Latency Heatmap",
      "type": "heatmap",
      "targets": [{
        "expr": "sum(rate(kscore_nats_message_latency_seconds_bucket[5m])) by (le)",
        "format": "heatmap"
      }],
      "options": {
        "yAxis": {"unit": "s"}
      }
    },
    {
      "title": "Duplicate Detection Rate",
      "type": "timeseries",
      "targets": [{
        "expr": "sum(rate(kscore_nats_duplicates_detected_total[5m])) by (subject)",
        "legendFormat": "{{subject}}"
      }]
    },
    {
      "title": "Message Size Distribution",
      "type": "histogram",
      "targets": [{
        "expr": "sum(rate(kscore_nats_message_size_bytes_bucket[5m])) by (le)",
        "format": "heatmap"
      }],
      "options": {
        "xAxis": {"unit": "bytes"}
      }
    }
  ]
}
```

### JetStream Dashboard

Monitor JetStream streams, consumers, and storage.

```json
{
  "title": "NATS JetStream",
  "panels": [
    {
      "title": "Stream Overview",
      "type": "table",
      "targets": [{
        "expr": "kscore_nats_stream_messages",
        "legendFormat": "",
        "instant": true
      }],
      "transformations": [{
        "id": "organize",
        "options": {
          "includeByName": {
            "stream": true,
            "Value": true
          },
          "renameByName": {"Value": "Messages"}
        }
      }]
    },
    {
      "title": "Stream Messages Count",
      "type": "timeseries",
      "targets": [{
        "expr": "kscore_nats_stream_messages",
        "legendFormat": "{{stream}}"
      }]
    },
    {
      "title": "Stream Storage Used",
      "type": "timeseries",
      "targets": [{
        "expr": "kscore_nats_stream_bytes",
        "legendFormat": "{{stream}}"
      }],
      "fieldConfig": {"defaults": {"unit": "bytes"}}
    },
    {
      "title": "Consumer Pending Messages",
      "type": "timeseries",
      "targets": [{
        "expr": "kscore_nats_consumer_pending",
        "legendFormat": "{{stream}}/{{consumer}}"
      }]
    },
    {
      "title": "Consumer Ack Pending",
      "type": "timeseries",
      "targets": [{
        "expr": "kscore_nats_consumer_ack_pending",
        "legendFormat": "{{stream}}/{{consumer}}"
      }]
    },
    {
      "title": "Redelivery Rate",
      "type": "timeseries",
      "targets": [{
        "expr": "sum(rate(kscore_nats_consumer_redelivered_total[5m])) by (consumer)",
        "legendFormat": "{{consumer}}"
      }]
    },
    {
      "title": "Stream Storage Capacity",
      "type": "gauge",
      "targets": [{
        "expr": "kscore_nats_stream_bytes / kscore_nats_stream_max_bytes * 100",
        "legendFormat": "{{stream}}"
      }],
      "fieldConfig": {
        "defaults": {
          "unit": "percent",
          "thresholds": {
            "steps": [
              {"color": "green", "value": 0},
              {"color": "yellow", "value": 70},
              {"color": "red", "value": 90}
            ]
          }
        }
      }
    }
  ]
}
```

### Cluster Topology Dashboard

Visualize NATS cluster structure and health.

```json
{
  "title": "NATS Cluster Topology",
  "panels": [
    {
      "title": "Cluster Servers",
      "type": "stat",
      "targets": [{
        "expr": "count(kscore_nats_server_up == 1)"
      }]
    },
    {
      "title": "Leaf Nodes Connected",
      "type": "stat",
      "targets": [{
        "expr": "sum(kscore_nats_leaf_nodes_total)"
      }]
    },
    {
      "title": "Gateway Connections",
      "type": "stat",
      "targets": [{
        "expr": "sum(kscore_nats_gateway_connections_total)"
      }]
    },
    {
      "title": "Server Status",
      "type": "table",
      "targets": [{
        "expr": "kscore_nats_server_up",
        "instant": true
      }, {
        "expr": "kscore_nats_server_connections",
        "instant": true
      }, {
        "expr": "kscore_nats_server_subscriptions",
        "instant": true
      }],
      "transformations": [{
        "id": "merge"
      }]
    },
    {
      "title": "Route Connections",
      "type": "timeseries",
      "targets": [{
        "expr": "kscore_nats_route_connections by (server)",
        "legendFormat": "{{server}}"
      }]
    },
    {
      "title": "Leaf Node Distribution",
      "type": "piechart",
      "targets": [{
        "expr": "sum(kscore_nats_leaf_nodes_total) by (hub_server)",
        "legendFormat": "{{hub_server}}"
      }]
    },
    {
      "title": "Gateway Latency",
      "type": "timeseries",
      "targets": [{
        "expr": "histogram_quantile(0.95, rate(kscore_nats_gateway_latency_seconds_bucket[5m])) * 1000",
        "legendFormat": "{{gateway}} P95"
      }],
      "fieldConfig": {"defaults": {"unit": "ms"}}
    },
    {
      "title": "Cross-Cluster Messages",
      "type": "timeseries",
      "targets": [{
        "expr": "sum(rate(kscore_nats_gateway_messages_total[5m])) by (gateway)",
        "legendFormat": "{{gateway}}"
      }],
      "fieldConfig": {"defaults": {"unit": "msg/s"}}
    }
  ]
}
```

### Buffer and Reliability Dashboard

Monitor message buffers, circuit breakers, and reliability features.

```json
{
  "title": "NATS Reliability",
  "panels": [
    {
      "title": "Buffer Utilization",
      "type": "gauge",
      "targets": [{
        "expr": "kscore_nats_buffer_size / kscore_nats_buffer_capacity * 100",
        "legendFormat": "{{agent}}"
      }],
      "fieldConfig": {
        "defaults": {
          "unit": "percent",
          "thresholds": {
            "steps": [
              {"color": "green", "value": 0},
              {"color": "yellow", "value": 60},
              {"color": "red", "value": 80}
            ]
          }
        }
      }
    },
    {
      "title": "Buffer Overflow Events",
      "type": "timeseries",
      "targets": [{
        "expr": "rate(kscore_nats_buffer_overflow_total[5m])",
        "legendFormat": "Overflows/sec"
      }]
    },
    {
      "title": "Circuit Breaker States",
      "type": "stat",
      "targets": [{
        "expr": "count(kscore_nats_circuit_breaker_state == 0)",
        "legendFormat": "Closed"
      }, {
        "expr": "count(kscore_nats_circuit_breaker_state == 1)",
        "legendFormat": "Half-Open"
      }, {
        "expr": "count(kscore_nats_circuit_breaker_state == 2)",
        "legendFormat": "Open"
      }],
      "fieldConfig": {
        "defaults": {
          "mappings": [{
            "type": "value",
            "options": {
              "0": {"text": "Closed", "color": "green"},
              "1": {"text": "Half-Open", "color": "yellow"},
              "2": {"text": "Open", "color": "red"}
            }
          }]
        }
      }
    },
    {
      "title": "Circuit Breaker Trips",
      "type": "timeseries",
      "targets": [{
        "expr": "sum(increase(kscore_nats_circuit_breaker_trips_total[1h])) by (endpoint)",
        "legendFormat": "{{endpoint}}"
      }]
    },
    {
      "title": "Message Retry Rate",
      "type": "timeseries",
      "targets": [{
        "expr": "sum(rate(kscore_nats_message_retries_total[5m])) by (reason)",
        "legendFormat": "{{reason}}"
      }]
    },
    {
      "title": "Dead Letter Queue Size",
      "type": "timeseries",
      "targets": [{
        "expr": "kscore_nats_dlq_messages_total",
        "legendFormat": "DLQ Messages"
      }]
    },
    {
      "title": "Degradation Mode Status",
      "type": "stat",
      "targets": [{
        "expr": "kscore_nats_degradation_mode",
        "legendFormat": "{{agent}}"
      }],
      "fieldConfig": {
        "defaults": {
          "mappings": [{
            "type": "value",
            "options": {
              "0": {"text": "Normal", "color": "green"},
              "1": {"text": "Degraded", "color": "yellow"},
              "2": {"text": "Offline", "color": "red"}
            }
          }]
        }
      }
    }
  ]
}
```

### Agent NATS Dashboard

Per-agent NATS metrics for detailed troubleshooting.

```json
{
  "title": "Agent NATS Details",
  "variables": [
    {
      "name": "agent",
      "type": "query",
      "query": "label_values(kscore_nats_connections_total, agent)"
    }
  ],
  "panels": [
    {
      "title": "Connection Status",
      "type": "stat",
      "targets": [{
        "expr": "kscore_nats_connection_status{agent=\"$agent\"}"
      }],
      "fieldConfig": {
        "defaults": {
          "mappings": [{
            "type": "value",
            "options": {
              "0": {"text": "Disconnected", "color": "red"},
              "1": {"text": "Connected", "color": "green"},
              "2": {"text": "Reconnecting", "color": "yellow"}
            }
          }]
        }
      }
    },
    {
      "title": "Connected Endpoint",
      "type": "stat",
      "targets": [{
        "expr": "kscore_nats_current_endpoint{agent=\"$agent\"}",
        "legendFormat": "{{endpoint}}"
      }]
    },
    {
      "title": "Messages Published",
      "type": "timeseries",
      "targets": [{
        "expr": "rate(kscore_nats_messages_published_total{agent=\"$agent\"}[5m])",
        "legendFormat": "{{subject}}"
      }],
      "fieldConfig": {"defaults": {"unit": "msg/s"}}
    },
    {
      "title": "Messages Received",
      "type": "timeseries",
      "targets": [{
        "expr": "rate(kscore_nats_messages_received_total{agent=\"$agent\"}[5m])",
        "legendFormat": "{{subject}}"
      }],
      "fieldConfig": {"defaults": {"unit": "msg/s"}}
    },
    {
      "title": "Buffer Status",
      "type": "timeseries",
      "targets": [{
        "expr": "kscore_nats_buffer_size{agent=\"$agent\"}",
        "legendFormat": "Current Size"
      }, {
        "expr": "kscore_nats_buffer_capacity{agent=\"$agent\"}",
        "legendFormat": "Capacity"
      }],
      "fieldConfig": {"defaults": {"unit": "bytes"}}
    },
    {
      "title": "Subscriptions",
      "type": "table",
      "targets": [{
        "expr": "kscore_nats_subscription_messages_total{agent=\"$agent\"}",
        "instant": true
      }]
    },
    {
      "title": "Connection Events",
      "type": "logs",
      "targets": [{
        "expr": "{agent=\"$agent\", component=\"nats\"}",
        "datasource": "Loki"
      }]
    }
  ]
}
```

### Dashboard Installation

```bash
# Import all NATS dashboards
for dashboard in connection-health message-throughput jetstream cluster-topology reliability agent-details; do
  curl -X POST http://admin:admin@grafana:3000/api/dashboards/db \
    -H "Content-Type: application/json" \
    -d @deploy/grafana/dashboards/nats-${dashboard}.json
done

# Or use Grafana provisioning
cp deploy/grafana/dashboards/nats-*.json /etc/grafana/provisioning/dashboards/
```

### Dashboard Variables

All dashboards support these common variables:

| Variable | Query | Description |
|----------|-------|-------------|
| `$cluster` | `label_values(kscore_nats_server_up, cluster)` | NATS cluster |
| `$endpoint` | `label_values(kscore_nats_connections_total, endpoint)` | NATS endpoint |
| `$agent` | `label_values(kscore_nats_connections_total, agent)` | Agent ID |
| `$stream` | `label_values(kscore_nats_stream_messages, stream)` | JetStream stream |
| `$interval` | `$__auto_interval_interval` | Auto time interval |

### Recording Rules for Dashboards

Add these recording rules for dashboard performance:

```yaml
groups:
  - name: nats-recording-rules
    rules:
      # Connection success rate (pre-computed)
      - record: kscore:nats_connection_success_rate
        expr: |
          sum(rate(kscore_nats_connections_total{status="success"}[5m])) /
          sum(rate(kscore_nats_connections_total[5m]))

      # Message throughput aggregates
      - record: kscore:nats_messages_per_second
        expr: sum(rate(kscore_nats_messages_published_total[5m]))

      # P95 latency by endpoint
      - record: kscore:nats_latency_p95
        expr: histogram_quantile(0.95, rate(kscore_nats_connection_latency_seconds_bucket[5m]))

      # Buffer utilization
      - record: kscore:nats_buffer_utilization
        expr: kscore_nats_buffer_size / kscore_nats_buffer_capacity

      # JetStream storage utilization
      - record: kscore:nats_stream_utilization
        expr: kscore_nats_stream_bytes / kscore_nats_stream_max_bytes
```

## See Also

- [NATS Mesh Concepts]({{< ref "../concepts/nats-mesh" >}}) - Architecture overview
- [NATS Mesh Deployment]({{< ref "nats-mesh-deployment" >}}) - Deployment guides
- [NATS Mesh Reference]({{< ref "../reference/nats-mesh" >}}) - Configuration reference
