---
title: "Event Reference"
weight: 5
description: >
  Complete event schema reference with filtering expressions and examples
---

## Overview

Keystone Core emits 29 standard event types across 8 categories, plus dynamic GitOps webhook events. All events follow a consistent schema and support CEL-based filtering.

**Event Categories**:

- [Agent Events](#agent-events) (5 types)
- [Job Events](#job-events) (4 types)
- [State Events](#state-events) (5 types)
- [GitOps Webhook Events](#gitops-webhook-events) (dynamic)
- [Bootstrap Events](#bootstrap-events) (7 types)
- [System Events](#system-events) (3 types)
- [User Events](#user-events) (3 types)
- [Policy Events](#policy-events) (2 types)

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
  "tags": {"env": "production", "region": "us-east-1"},
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

- Event type (see [Agent Events](#agent-events), [Job Events](#job-events), [State Events](#state-events), etc.)
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

**tags** (object, map of string to string)

- Custom key-value tags
- Example: `{"env": "production", "region": "us-east-1"}`

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
  "tags": {"env": "production", "region": "us-east-1"},
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

### agent.heartbeat

Emitted when agent sends a heartbeat.

**Severity**: `debug`

**Data Fields**:

```json
{
  "agent_id": "web-01",
  "uptime": "5d2h30m",
  "load_avg": 0.75,
  "memory_used_percent": 62.5
}
```

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

### agent.error

Emitted when an agent encounters an error.

**Severity**: `error`

**Data Fields**:

```json
{
  "agent_id": "web-01",
  "error": "connection timeout",
  "error_code": "CONN_TIMEOUT",
  "component": "nats",
  "recoverable": true
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

### job.output

Emitted when streaming output from a job execution.

**Severity**: `info`

**Data Fields**:

```json
{
  "job_id": "job-abc123",
  "agent_id": "web-01",
  "stream": "stdout",
  "output": "nginx: the configuration file /etc/nginx/nginx.conf syntax is ok",
  "sequence": 1
}
```

**Stream Values**:

- `stdout` - Standard output
- `stderr` - Standard error

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

## GitOps Webhook Events

GitOps webhook events are dynamic and follow a consistent naming pattern based on the webhook source. These events are emitted when webhook payloads are parsed in `internal/gitops/webhook/`.

**Event Type Patterns**:

- `gitops.argocd.<event_type>`: Event type from ArgoCD payload (`type`) or operation phase
- `gitops.flux.<event_type>`: Event type from `X-Flux-Event` header or payload `reason`
- `gitops.github.<event_type>`: Event type from `X-GitHub-Event` header
- `gitops.gitlab.<event_type>`: Event type from `X-Gitlab-Event` header or payload `object_kind`/`event_name`
- `gitops.webhook`: Fallback when a specific type cannot be determined

**Common Examples**:

- `gitops.argocd.sync`, `gitops.argocd.health`, `gitops.argocd.Succeeded`
- `gitops.flux.ReconciliationSucceeded`, `gitops.flux.ReconciliationFailed`
- `gitops.github.deployment`, `gitops.github.deployment_status`, `gitops.github.workflow_run`, `gitops.github.push`
- `gitops.gitlab.deployment`, `gitops.gitlab.pipeline`, `gitops.gitlab.push`, `gitops.gitlab.merge_request`

**Data Fields** (superset; varies by source):

```json
{
  "webhook_id": "uuid",
  "webhook_type": "argocd|flux|github|gitlab",
  "application": "app-name",
  "namespace": "namespace-or-env",
  "revision": "git-sha",
  "status": "status-or-severity",
  "repo_url": "https://example.com/repo.git",
  "target_revision": "main",
  "message": "source-specific message",
  "reason": "source-specific reason"
}
```

**Example (ArgoCD Sync Event)**:

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "type": "gitops.argocd.sync",
  "source": "webhook/argocd",
  "timestamp": "2024-01-15T10:30:45Z",
  "severity": "info",
  "correlation_id": "webhook-2f734e9b-1f3f-4b63-8cb2-61a0f1b6b4c8",
  "data": {
    "webhook_id": "2f734e9b-1f3f-4b63-8cb2-61a0f1b6b4c8",
    "webhook_type": "argocd",
    "application": "test-app",
    "namespace": "argocd",
    "revision": "abc123",
    "status": "Synced",
    "repo_url": "https://github.com/org/repo.git",
    "target_revision": "main"
  }
}
```

## Bootstrap Events

Events related to agent bootstrap registration flow.

### bootstrap.generate

Emitted when a bootstrap credential is generated.

**Severity**: `info`

**Data Fields**:

```json
{
  "credential_id": "boot-abc123",
  "cluster": "production",
  "ttl": 300,
  "max_uses": 1,
  "credential_type": "nkey"
}
```

### bootstrap.validate

Emitted when a bootstrap credential is validated.

**Severity**: `info` (success) / `warning` (failure)

**Data Fields**:

```json
{
  "credential_id": "boot-abc123",
  "cluster": "production",
  "success": true,
  "error": ""
}
```

### bootstrap.use

Emitted when a bootstrap credential is used for registration.

**Severity**: `info`

**Data Fields**:

```json
{
  "credential_id": "boot-abc123",
  "agent_id": "web-01",
  "cluster": "production",
  "source_ip": "10.0.1.100"
}
```

### bootstrap.register

Emitted when an agent successfully registers via bootstrap.

**Severity**: `info`

**Data Fields**:

```json
{
  "credential_id": "boot-abc123",
  "agent_id": "web-01",
  "cluster": "production",
  "success": true,
  "source_ip": "10.0.1.100"
}
```

### bootstrap.revoke

Emitted when a bootstrap credential is revoked.

**Severity**: `info`

**Data Fields**:

```json
{
  "credential_id": "boot-abc123",
  "cluster": "production",
  "reason": "security concern"
}
```

### bootstrap.expire

Emitted when a bootstrap credential expires.

**Severity**: `info`

**Data Fields**:

```json
{
  "credential_id": "boot-abc123",
  "cluster": "production",
  "expired_at": "2024-01-15T10:35:45Z"
}
```

### bootstrap.cleanup

Emitted when expired credentials are cleaned up.

**Severity**: `debug`

**Data Fields**:

```json
{
  "cluster": "production",
  "cleaned_count": 5
}
```

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

### system.error

Emitted when a system-level error occurs.

**Severity**: `error`

**Data Fields**:

```json
{
  "error": "database connection lost",
  "error_code": "DB_CONN_LOST",
  "component": "state-manager",
  "recoverable": true,
  "retry_count": 3
}
```

## User Events

Events related to user actions and authentication.

### user.login

Emitted when a user authenticates.

**Severity**: `info`

**Data Fields**:

```json
{
  "user": "admin@example.com",
  "method": "api_key",
  "source_ip": "192.168.1.100",
  "user_agent": "kscorectl/1.0.0"
}
```

**Method Values**:

- `api_key` - API key authentication
- `mtls` - mTLS certificate authentication
- `jwt` - JWT token authentication

### user.command

Emitted when a user executes a command via CLI or API.

**Severity**: `info`

**Data Fields**:

```json
{
  "user": "admin@example.com",
  "command": "exec run",
  "args": ["systemctl restart nginx", "--target", "role:web"],
  "source_ip": "192.168.1.100"
}
```

### user.error

Emitted when a user action fails.

**Severity**: `error`

**Data Fields**:

```json
{
  "user": "admin@example.com",
  "action": "state apply",
  "error": "permission denied",
  "error_code": "AUTHZ_DENIED",
  "resource": "states/production.yaml"
}
```

## Policy Events

Events related to policy evaluation and enforcement.

### policy.pass

Emitted when a policy evaluation passes.

**Severity**: `info`

**Data Fields**:

```json
{
  "policy_id": "require-labels",
  "policy_name": "Require Labels",
  "resource_type": "state",
  "resource_id": "nginx_config",
  "mode": "enforce",
  "duration_ms": 5
}
```

### policy.violation

Emitted when a policy violation is detected.

**Severity**: `warning` (audit mode) / `error` (enforce mode)

**Data Fields**:

```json
{
  "policy_id": "require-labels",
  "policy_name": "Require Labels",
  "resource_type": "state",
  "resource_id": "nginx_config",
  "mode": "enforce",
  "violations": [
    {
      "rule": "labels-required",
      "message": "Resource missing required label: owner",
      "severity": "high",
      "path": "metadata.labels"
    }
  ],
  "duration_ms": 8
}
```

**Mode Values**:

- `enforce` - Violations are blocked
- `audit` - Violations are logged only
- `warn` - Violations generate warnings but are not blocked

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
kscorectl events list --type agent.connect

# Query with expression
kscorectl events query "type == 'job.fail' and severity == 'error'"

# Time range
kscorectl events list --since 1h --before 30m

# Severity filter (minimum severity - shows warning and above)
kscorectl events list --severity warning
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
event := events.NewEvent(events.EventTypeJobComplete).
    Source("control-plane").
    Severity(events.SeverityInfo).
    CorrelationID("job-abc123").
    Tag("env", "production").
    DataMap(map[string]interface{}{
        "job_id": "abc123",
        "status": "success",
    }).
    Build()

publisher.Publish(event)
```

### From CLI

```bash
kscorectl events emit \
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
