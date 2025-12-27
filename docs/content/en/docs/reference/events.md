---
title: "Event Reference"
weight: 5
description: >
  Complete event schema reference with filtering expressions and examples
---

## Overview

TitanAnvil emits 15 standard event types across 5 categories. All events follow a consistent schema and support CEL-based filtering.

**Event Categories**:
- [Agent Events](#agent-events) (4 types)
- [Job Events](#job-events) (3 types)
- [State Events](#state-events) (5 types)
- [System Events](#system-events) (2 types)
- [User Events](#user-events) (1 type)

## Event Schema

All events follow this structure:

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "type": "event.type",
  "source": "source-identifier",
  "timestamp": "2024-01-15T10:30:45Z",
  "severity": "info",
  "correlation_id": "correlation-id",
  "tags": ["tag1", "tag2"],
  "data": {
    "event-specific": "fields"
  }
}
```

### Fields

**id** (string, UUID)
- Unique event identifier
- Auto-generated

**type** (string)
- Event type (see [Event Types](#event-types))
- Example: `agent.connect`, `job.complete`

**source** (string)
- Event source identifier
- Usually agent ID or system component
- Example: `web-01`, `control-plane`

**timestamp** (string, RFC3339)
- When event occurred
- Example: `2024-01-15T10:30:45Z`

**severity** (string)
- Event severity level
- Values: `debug`, `info`, `warning`, `error`, `critical`

**correlation_id** (string)
- For tracking related events
- Example: `job-abc123`, `agent-web-01`

**tags** (array of strings)
- Custom tags
- Example: `["production", "us-east-1"]`

**data** (object)
- Event-specific data
- Schema varies by event type

## Agent Events

Events related to agent lifecycle.

### agent.connect

Emitted when agent registers with control plane.

**Severity**: `info`

**Data Fields**:
```json
{
  "agent_id": "web-01",
  "datacenter": "us-east-1",
  "environment": "production",
  "role": "web",
  "os": "linux",
  "arch": "amd64",
  "version": "1.0.0",
  "metadata": {
    "hostname": "web-01.example.com",
    "ip": "10.0.1.100",
    "cpu_count": 4,
    "memory_total": 8589934592
  }
}
```

**Example**:
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "type": "agent.connect",
  "source": "web-01",
  "timestamp": "2024-01-15T10:30:45Z",
  "severity": "info",
  "correlation_id": "agent-web-01",
  "tags": ["production", "us-east-1"],
  "data": {
    "agent_id": "web-01",
    "datacenter": "us-east-1",
    "environment": "production",
    "role": "web"
  }
}
```

### agent.disconnect

Emitted when agent disconnects.

**Severity**: `warning`

**Data Fields**:
```json
{
  "agent_id": "web-01",
  "reason": "timeout",
  "last_heartbeat": "2024-01-15T10:28:00Z",
  "connected_duration": "5d2h30m"
}
```

**Reason Values**:
- `timeout` - Heartbeat timeout
- `graceful` - Graceful shutdown
- `error` - Connection error

### agent.heartbeat_failed

Emitted when agent misses heartbeats.

**Severity**: `warning`

**Data Fields**:
```json
{
  "agent_id": "web-01",
  "missed_count": 3,
  "last_successful": "2024-01-15T10:25:00Z"
}
```

### agent.metadata_changed

Emitted when agent metadata updates.

**Severity**: `info`

**Data Fields**:
```json
{
  "agent_id": "web-01",
  "changed_fields": ["tags", "role"],
  "old_metadata": {
    "role": "web",
    "tags": ["nginx"]
  },
  "new_metadata": {
    "role": "web",
    "tags": ["nginx", "monitoring"]
  }
}
```

## Job Events

Events related to command execution.

### job.start

Emitted when command execution begins.

**Severity**: `info`

**Data Fields**:
```json
{
  "job_id": "job-abc123",
  "command": "systemctl restart nginx",
  "target": "role:web",
  "target_count": 50,
  "timeout": "5m",
  "user": "ops-user"
}
```

### job.complete

Emitted when command execution succeeds.

**Severity**: `info`

**Data Fields**:
```json
{
  "job_id": "job-abc123",
  "command": "systemctl restart nginx",
  "duration": "2.5s",
  "results": {
    "total": 50,
    "success": 48,
    "failed": 2,
    "timeout": 0
  }
}
```

### job.fail

Emitted when command execution fails.

**Severity**: `error`

**Data Fields**:
```json
{
  "job_id": "job-abc123",
  "command": "systemctl restart nginx",
  "error": "timeout exceeded",
  "duration": "5m",
  "failed_agents": ["web-03", "web-07"],
  "results": {
    "total": 50,
    "success": 20,
    "failed": 25,
    "timeout": 5
  }
}
```

## State Events

Events related to state management.

### state.apply.start

Emitted when state application begins.

**Severity**: `info`

**Data Fields**:
```json
{
  "run_id": "run-xyz789",
  "state_file": "web-server.yaml",
  "target": "role:web",
  "target_count": 50,
  "module_count": 10,
  "check_only": false
}
```

### state.apply.done

Emitted when state application completes successfully.

**Severity**: `info`

**Data Fields**:
```json
{
  "run_id": "run-xyz789",
  "state_file": "web-server.yaml",
  "duration": "30s",
  "results": {
    "total_agents": 50,
    "success": 50,
    "failed": 0,
    "total_states": 500,
    "changed": 150,
    "unchanged": 350
  }
}
```

### state.apply.fail

Emitted when state application fails.

**Severity**: `error`

**Data Fields**:
```json
{
  "run_id": "run-xyz789",
  "state_file": "web-server.yaml",
  "error": "module execution failed",
  "duration": "15s",
  "failed_modules": ["nginx_service"],
  "failed_agents": ["web-05"],
  "results": {
    "total_agents": 50,
    "success": 49,
    "failed": 1
  }
}
```

### state.change

Emitted when state resources change.

**Severity**: `info`

**Data Fields**:
```json
{
  "agent_id": "web-01",
  "state_id": "nginx_config",
  "module": "file",
  "old_state": {
    "contents_hash": "abc123",
    "mode": "0644"
  },
  "new_state": {
    "contents_hash": "def456",
    "mode": "0644"
  }
}
```

### state.drift

Emitted when configuration drift detected.

**Severity**: Varies by drift severity

**Data Fields**:
```json
{
  "agent_id": "web-01",
  "state_file": "web-server.yaml",
  "drift_severity": "high",
  "drifted_states": [
    {
      "state_id": "nginx_service",
      "module": "service",
      "expected": {"state": "running"},
      "actual": {"state": "stopped"},
      "severity": "high"
    }
  ],
  "summary": {
    "total_states": 10,
    "compliant": 9,
    "drifted": 1
  }
}
```

**Drift Severity Mapping**:
- `low` → event severity: `info`
- `medium` → event severity: `warning`
- `high` → event severity: `error`
- `critical` → event severity: `critical`

## System Events

Events related to system lifecycle.

### system.startup

Emitted when control plane starts.

**Severity**: `info`

**Data Fields**:
```json
{
  "version": "1.0.0",
  "mode": "production",
  "config_summary": {
    "nats_mode": "external",
    "storage_type": "postgresql"
  }
}
```

### system.shutdown

Emitted when control plane shuts down gracefully.

**Severity**: `warning`

**Data Fields**:
```json
{
  "reason": "graceful shutdown",
  "uptime": "72h30m15s",
  "connected_agents": 150
}
```

## User Events

Custom events emitted by users or scripts.

### user.custom

Custom events with user-defined payloads.

**Severity**: User-defined (default: `info`)

**Data Fields**:
User-defined (arbitrary JSON)

**Example**:
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "type": "user.custom",
  "source": "backup-script",
  "timestamp": "2024-01-15T10:30:45Z",
  "severity": "info",
  "tags": ["backup", "database"],
  "data": {
    "action": "backup",
    "database": "mydb",
    "size_bytes": 1073741824,
    "status": "success",
    "duration": "5m30s"
  }
}
```

## Filtering Expressions

Filter events using CEL (Common Expression Language).

### Field Access

**Top-level fields**:
```javascript
type == 'agent.connect'
source == 'web-01'
severity == 'error'
```

**Data fields**:
```javascript
data.agent_id == 'web-01'
data.job_id == 'job-abc123'
data.environment == 'production'
```

**Nested data**:
```javascript
data.metadata.hostname == 'web-01.example.com'
data.results.success > 0
```

**Arrays**:
```javascript
'production' in tags
tags.contains('us-east-1')
```

### Comparison Operators

**Equality**:
```javascript
type == 'agent.connect'
type != 'user.custom'
```

**Comparison**:
```javascript
severity >= 'warning'
data.exit_code != 0
data.duration > 60
```

**Regex**:
```javascript
source =~ 'web-.*'
type =~ '^agent\\..*'
```

**Glob**:
```javascript
source ~~ 'web-*'
type ~~ 'state.*'
```

### Logical Operators

**AND**:
```javascript
type == 'job.fail' && severity == 'critical'
type == 'agent.disconnect' and data.reason == 'timeout'
```

**OR**:
```javascript
type == 'agent.connect' || type == 'agent.disconnect'
type == 'job.fail' or type == 'state.apply.fail'
```

**NOT**:
```javascript
!(type == 'user.custom')
not (severity == 'debug')
```

### Container Operations

**Contains**:
```javascript
tags.contains('production')
type.contains('agent')
```

**In**:
```javascript
'production' in tags
'us-east-1' in tags
```

**StartsWith/EndsWith**:
```javascript
type.startsWith('agent.')
source.endsWith('-01')
```

### Complex Filters

**Multiple conditions**:
```javascript
type == 'state.drift' &&
data.drift_severity >= 'high' &&
data.agent_id in ['web-01', 'web-02'] &&
'production' in tags
```

**Nested conditions**:
```javascript
(type == 'job.fail' || type == 'state.apply.fail') &&
severity >= 'error' &&
data.environment == 'production'
```

**Data validation**:
```javascript
type == 'agent.connect' &&
data.os == 'linux' &&
data.cpu_count >= 4 &&
data.memory_total >= 8589934592
```

## Filter Examples

### Common Filters

**All agent events**:
```javascript
type.startsWith('agent.')
```

**Production errors**:
```javascript
'production' in tags && severity >= 'error'
```

**Failed operations**:
```javascript
type == 'job.fail' || type == 'state.apply.fail'
```

**High-severity drift**:
```javascript
type == 'state.drift' && data.drift_severity >= 'high'
```

**Specific agent**:
```javascript
source == 'web-01' || data.agent_id == 'web-01'
```

**Time-based** (requires event.timestamp):
```javascript
type == 'agent.disconnect' &&
timestamp > timestamp('2024-01-15T00:00:00Z')
```

### Use Case Filters

**Auto-remediation trigger**:
```javascript
type == 'state.drift' &&
data.drift_severity == 'critical' &&
data.environment == 'production'
```

**Security alerts**:
```javascript
type == 'policy.violation' &&
data.category == 'security' &&
data.severity >= 'high'
```

**Performance monitoring**:
```javascript
type == 'job.complete' &&
data.duration > duration('5m')
```

**Capacity alerts**:
```javascript
type == 'agent.connect' &&
data.metadata.cpu_count < 4
```

## Querying Events

### CLI

```bash
# List events with filter
titanctl event list --type agent.connect

# Query with expression
titanctl event query "type == 'job.fail' and severity == 'error'"

# Time range
titanctl event list --since 1h --until now

# Severity filter
titanctl event list --severity warning,error,critical
```

### API

```bash
# GET /api/v1/events
curl "http://control-plane:8080/api/v1/events?type=agent.connect&limit=100"
```

### gRPC

```protobuf
message ListEventsRequest {
  string type = 1;
  string source = 2;
  string severity = 3;
  string since = 4;
  string until = 5;
  int32 limit = 6;
  int32 offset = 7;
}
```

## Event Emission

### From Code

```go
event := events.NewEvent().
    WithType(events.EventTypeJobComplete).
    WithSource("control-plane").
    WithSeverity(events.SeverityInfo).
    WithCorrelationID("job-abc123").
    WithTag("production").
    WithData(map[string]interface{}{
        "job_id": "abc123",
        "status": "success",
    }).
    Build()

publisher.Publish(ctx, event)
```

### From CLI

```bash
titanctl event emit \
  --type user.custom \
  --source "maintenance-script" \
  --severity info \
  --data '{"action":"backup","status":"success"}'
```

### From HTTP API

```bash
curl -X POST http://control-plane:8080/api/v1/events \
  -H "Content-Type: application/json" \
  -d '{
    "type": "user.custom",
    "source": "external-system",
    "severity": "warning",
    "data": {"alert": "disk usage high"}
  }'
```

## Event Retention

Events are stored with configurable retention:

```yaml
events:
  storage:
    retention:
      max_age: "30d"
      max_count: 1000000
      min_severity: "info"
    type_retention:
      "agent.heartbeat": "1d"
      "state.drift": "90d"
      "user.custom": "365d"
```

## Best Practices

1. **Use Correlation IDs**: Always set correlation IDs for related events
2. **Appropriate Severity**: Match severity to actual impact
3. **Meaningful Tags**: Use tags for flexible filtering
4. **Structured Data**: Keep data fields consistent per event type
5. **Filter Specificity**: Write narrow filters to reduce processing
6. **Retention Policies**: Configure appropriate retention per type

## See Also

- [Event System Concepts](../../concepts/events/) - Event system overview
- [Reactors Concepts](../../concepts/reactors/) - Event automation
- [API Reference](../api/#events) - Event API endpoints
- [Metrics Reference](../metrics/) - Event-related metrics
