---
title: "API Reference"
weight: 1
description: >
  Complete REST and gRPC API reference with request/response examples
---

## Overview

Keystone Core provides both REST and gRPC APIs for programmatic access to all functionality. The REST API uses gRPC-gateway for automatic translation from gRPC.

**Base URLs**:
- REST API: `http://control-plane:8080/api/v1`
- gRPC: `control-plane:9090`

**Authentication**: All API requests require authentication (see [Authentication](#authentication))

## Authentication

### API Key Authentication

Include API key in header:

```bash
curl -H "Authorization: Bearer <api-key>" \
  http://control-plane:8080/api/v1/agents
```

### mTLS Authentication

Use client certificates:

```bash
curl --cert client.crt --key client.key --cacert ca.crt \
  https://control-plane:8080/api/v1/agents
```

### Generating API Keys

```bash
# Generate API key
kscorectl auth create-key --name my-app --ttl 30d

# Output
API Key: ta_live_abc123xyz789
Expires: 2024-02-14T10:30:00Z
```

## REST API Endpoints

### Agents

#### List Agents

```http
GET /api/v1/agents
```

**Query Parameters**:
- `datacenter` (string): Filter by datacenter
- `environment` (string): Filter by environment
- `role` (string): Filter by role
- `status` (string): Filter by status (connected, disconnected, degraded)
- `limit` (int): Max results (default: 100)
- `offset` (int): Pagination offset (default: 0)

**Response**:
```json
{
  "agents": [
    {
      "id": "web-01",
      "datacenter": "us-east-1",
      "environment": "production",
      "role": "web",
      "status": "connected",
      "last_heartbeat": "2024-01-15T10:30:45Z",
      "metadata": {
        "hostname": "web-01.example.com",
        "os": "linux",
        "arch": "amd64",
        "ip": "10.0.1.100"
      },
      "tags": ["nginx", "frontend"]
    }
  ],
  "total": 150,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl -H "Authorization: Bearer $API_KEY" \
  "http://control-plane:8080/api/v1/agents?environment=production&role=web"
```

#### Get Agent

```http
GET /api/v1/agents/{agent_id}
```

**Response**:
```json
{
  "id": "web-01",
  "datacenter": "us-east-1",
  "environment": "production",
  "role": "web",
  "status": "connected",
  "last_heartbeat": "2024-01-15T10:30:45Z",
  "connected_at": "2024-01-10T08:00:00Z",
  "version": "1.0.0",
  "metadata": {
    "hostname": "web-01.example.com",
    "os": "linux",
    "arch": "amd64",
    "ip": "10.0.1.100",
    "cpu_count": 4,
    "memory_total": 8589934592
  },
  "tags": ["nginx", "frontend"],
  "resource_usage": {
    "cpu_percent": 45.2,
    "memory_bytes": 4294967296,
    "disk_bytes": 21474836480
  }
}
```

**Example**:
```bash
curl -H "Authorization: Bearer $API_KEY" \
  http://control-plane:8080/api/v1/agents/web-01
```

#### Update Agent Tags

```http
PATCH /api/v1/agents/{agent_id}/tags
```

**Request Body**:
```json
{
  "add_tags": ["monitoring", "backup"],
  "remove_tags": ["old-tag"]
}
```

**Response**:
```json
{
  "id": "web-01",
  "tags": ["nginx", "frontend", "monitoring", "backup"]
}
```

### Remote Execution

#### Execute Command

```http
POST /api/v1/exec
```

**Request Body**:
```json
{
  "command": "systemctl restart nginx",
  "target": "role:web and datacenter:us-east-1",
  "timeout": "30s",
  "async": false
}
```

**Parameters**:
- `command` (string, required): Command to execute
- `target` (string, required): Target expression (glob or CEL)
- `timeout` (string): Execution timeout (default: 5m)
- `async` (bool): Run asynchronously (default: false)
- `batch_size` (int): Batch size for parallel execution
- `batch_delay` (string): Delay between batches

**Synchronous Response**:
```json
{
  "job_id": "job-abc123",
  "status": "completed",
  "started_at": "2024-01-15T10:30:45Z",
  "completed_at": "2024-01-15T10:30:47Z",
  "duration": "2.1s",
  "results": [
    {
      "agent_id": "web-01",
      "status": "success",
      "exit_code": 0,
      "stdout": "Restarted nginx.service\n",
      "stderr": "",
      "duration": "1.2s"
    },
    {
      "agent_id": "web-02",
      "status": "success",
      "exit_code": 0,
      "stdout": "Restarted nginx.service\n",
      "stderr": "",
      "duration": "1.5s"
    }
  ],
  "summary": {
    "total": 2,
    "success": 2,
    "failed": 0,
    "timeout": 0
  }
}
```

**Asynchronous Response**:
```json
{
  "job_id": "job-abc123",
  "status": "running",
  "started_at": "2024-01-15T10:30:45Z",
  "target_count": 50
}
```

**Example**:
```bash
curl -X POST -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "command": "uptime",
    "target": "datacenter:us-east-1",
    "timeout": "10s"
  }' \
  http://control-plane:8080/api/v1/exec
```

#### Get Job Status

```http
GET /api/v1/jobs/{job_id}
```

**Response**:
```json
{
  "job_id": "job-abc123",
  "command": "systemctl restart nginx",
  "target": "role:web",
  "status": "completed",
  "started_at": "2024-01-15T10:30:45Z",
  "completed_at": "2024-01-15T10:30:47Z",
  "duration": "2.1s",
  "results": [...],
  "summary": {
    "total": 50,
    "success": 48,
    "failed": 2,
    "timeout": 0
  }
}
```

#### List Jobs

```http
GET /api/v1/jobs
```

**Query Parameters**:
- `status` (string): Filter by status (running, completed, failed)
- `since` (string): Jobs since timestamp (RFC3339)
- `limit` (int): Max results
- `offset` (int): Pagination offset

**Response**:
```json
{
  "jobs": [
    {
      "job_id": "job-abc123",
      "command": "systemctl restart nginx",
      "status": "completed",
      "started_at": "2024-01-15T10:30:45Z",
      "summary": {
        "total": 50,
        "success": 48,
        "failed": 2
      }
    }
  ],
  "total": 100,
  "limit": 100,
  "offset": 0
}
```

### State Management

#### Apply State

```http
POST /api/v1/state/apply
```

**Request Body**:
```json
{
  "state": "...",  // YAML state file content
  "target": "role:web",
  "check_only": false,
  "vars": {
    "db_host": "postgres.example.com"
  }
}
```

**Parameters**:
- `state` (string, required): State file YAML content
- `target` (string, required): Target expression
- `check_only` (bool): Dry-run mode (default: false)
- `vars` (object): Template variables

**Response**:
```json
{
  "run_id": "run-xyz789",
  "status": "completed",
  "started_at": "2024-01-15T10:30:45Z",
  "completed_at": "2024-01-15T10:31:15Z",
  "duration": "30s",
  "results": [
    {
      "agent_id": "web-01",
      "status": "success",
      "changed": true,
      "states": [
        {
          "id": "nginx_package",
          "module": "package",
          "result": "unchanged",
          "comment": "Package nginx already installed"
        },
        {
          "id": "nginx_config",
          "module": "file",
          "result": "changed",
          "comment": "File /etc/nginx/nginx.conf updated"
        },
        {
          "id": "nginx_service",
          "module": "service",
          "result": "changed",
          "comment": "Service nginx restarted"
        }
      ]
    }
  ],
  "summary": {
    "total_agents": 50,
    "success": 50,
    "failed": 0,
    "total_states": 150,
    "changed": 75,
    "unchanged": 75
  }
}
```

#### Check State (Dry Run)

```http
POST /api/v1/state/check
```

Same as apply but with `check_only: true`. Returns what would change without applying.

#### Detect Drift

```http
POST /api/v1/state/drift
```

**Request Body**:
```json
{
  "state": "...",
  "target": "role:web"
}
```

**Response**:
```json
{
  "drift_detected": true,
  "agents": [
    {
      "agent_id": "web-01",
      "drift_severity": "high",
      "drifted_states": [
        {
          "id": "nginx_service",
          "expected": {"state": "running"},
          "actual": {"state": "stopped"},
          "severity": "high"
        }
      ]
    }
  ],
  "summary": {
    "total_agents": 50,
    "with_drift": 5,
    "severity": {
      "critical": 1,
      "high": 2,
      "medium": 2,
      "low": 0
      }
  }
}
```

### Events

#### List Events

```http
GET /api/v1/events
```

**Query Parameters**:
- `type` (string): Filter by event type
- `source` (string): Filter by source
- `severity` (string): Filter by severity
- `since` (string): Events since timestamp
- `until` (string): Events until timestamp
- `limit` (int): Max results
- `offset` (int): Pagination offset

**Response**:
```json
{
  "events": [
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
        "environment": "production"
      }
    }
  ],
  "total": 10000,
  "limit": 100,
  "offset": 0
}
```

#### Emit Event

```http
POST /api/v1/events
```

**Request Body**:
```json
{
  "type": "user.custom",
  "source": "external-system",
  "severity": "warning",
  "tags": ["monitoring"],
  "data": {
    "alert": "disk usage high",
    "threshold": 90
  }
}
```

**Response**:
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "type": "user.custom",
  "timestamp": "2024-01-15T10:30:45Z"
}
```

### Policies

#### Evaluate Policy

```http
POST /api/v1/policies/evaluate
```

**Request Body**:
```json
{
  "policy_id": "ssh-hardening",
  "input": {
    "resource": {
      "type": "file",
      "path": "/etc/ssh/sshd_config",
      "contents": "Port 2222\nPermitRootLogin no\n"
    }
  }
}
```

**Response**:
```json
{
  "policy_id": "ssh-hardening",
  "allowed": true,
  "violations": [],
  "warnings": [],
  "duration": "5ms"
}
```

#### List Policy Violations

```http
GET /api/v1/policies/violations
```

**Query Parameters**:
- `policy` (string): Filter by policy ID
- `severity` (string): Filter by severity
- `since` (string): Violations since timestamp

**Response**:
```json
{
  "violations": [
    {
      "policy_id": "ssh-hardening",
      "rule_id": "default-port",
      "agent_id": "web-01",
      "severity": "high",
      "message": "SSH must not use default port 22",
      "detected_at": "2024-01-15T10:30:45Z"
    }
  ],
  "total": 50
}
```

#### Get Compliance Report

```http
GET /api/v1/policies/compliance
```

**Query Parameters**:
- `environment` (string): Filter by environment
- `period` (string): Time period (24h, 7d, 30d)

**Response**:
```json
{
  "period": {
    "start": "2024-01-08T10:30:45Z",
    "end": "2024-01-15T10:30:45Z"
  },
  "compliance_rate": 87.5,
  "total_evaluations": 10000,
  "compliant": 8750,
  "violations": 1250,
  "by_severity": {
    "critical": 50,
    "high": 200,
    "medium": 500,
    "low": 500
  },
  "by_policy": [
    {
      "policy_id": "ssh-hardening",
      "compliance_rate": 92.0,
      "violations": 40
    }
  ]
}
```

### GitOps

#### List Verifications

```http
GET /api/v1/gitops/verifications
```

**Response**:
```json
{
  "verifications": [
    {
      "id": "verify-abc123",
      "application": "myapp",
      "environment": "production",
      "status": "success",
      "started_at": "2024-01-15T10:30:45Z",
      "completed_at": "2024-01-15T10:32:00Z",
      "steps": [
        {
          "name": "HTTP health check",
          "status": "success",
          "duration": "500ms"
        }
      ]
    }
  ]
}
```

#### Trigger Rollback

```http
POST /api/v1/gitops/rollback
```

**Request Body**:
```json
{
  "application": "myapp",
  "namespace": "production",
  "strategy": "previous"
}
```

**Response**:
```json
{
  "rollback_id": "rb-xyz789",
  "status": "in_progress",
  "application": "myapp",
  "from_revision": "abc123",
  "to_revision": "def456"
}
```

## gRPC Services

### AgentService

```protobuf
service AgentService {
  rpc ListAgents(ListAgentsRequest) returns (ListAgentsResponse);
  rpc GetAgent(GetAgentRequest) returns (Agent);
  rpc UpdateAgentTags(UpdateAgentTagsRequest) returns (Agent);
}
```

### ExecutionService

```protobuf
service ExecutionService {
  rpc ExecuteCommand(ExecuteCommandRequest) returns (ExecuteCommandResponse);
  rpc GetJob(GetJobRequest) returns (Job);
  rpc ListJobs(ListJobsRequest) returns (ListJobsResponse);
  rpc StreamJobOutput(StreamJobOutputRequest) returns (stream JobOutput);
}
```

### StateService

```protobuf
service StateService {
  rpc ApplyState(ApplyStateRequest) returns (ApplyStateResponse);
  rpc CheckState(CheckStateRequest) returns (CheckStateResponse);
  rpc DetectDrift(DetectDriftRequest) returns (DetectDriftResponse);
}
```

### EventService

```protobuf
service EventService {
  rpc ListEvents(ListEventsRequest) returns (ListEventsResponse);
  rpc EmitEvent(EmitEventRequest) returns (Event);
  rpc SubscribeEvents(SubscribeEventsRequest) returns (stream Event);
}
```

### PolicyService

```protobuf
service PolicyService {
  rpc EvaluatePolicy(EvaluatePolicyRequest) returns (EvaluatePolicyResponse);
  rpc ListViolations(ListViolationsRequest) returns (ListViolationsResponse);
  rpc GetComplianceReport(GetComplianceReportRequest) returns (ComplianceReport);
}
```

## Rate Limiting

All API endpoints are rate-limited:

**Limits**:
- **Default**: 100 requests/minute per API key
- **Burst**: 20 requests/second

**Headers**:
```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1642248645
```

**429 Response**:
```json
{
  "error": "rate_limit_exceeded",
  "message": "Rate limit exceeded. Try again in 30 seconds.",
  "retry_after": 30
}
```

## Error Responses

All errors follow this format:

```json
{
  "error": "error_code",
  "message": "Human-readable error message",
  "details": {
    "field": "Additional context"
  }
}
```

**Common Error Codes**:
- `invalid_request` (400): Invalid request parameters
- `unauthorized` (401): Missing or invalid authentication
- `forbidden` (403): Insufficient permissions
- `not_found` (404): Resource not found
- `conflict` (409): Resource conflict
- `rate_limit_exceeded` (429): Too many requests
- `internal_error` (500): Internal server error
- `service_unavailable` (503): Service temporarily unavailable

## Pagination

List endpoints support cursor-based pagination:

**Request**:
```bash
GET /api/v1/agents?limit=100&offset=0
```

**Response**:
```json
{
  "agents": [...],
  "total": 500,
  "limit": 100,
  "offset": 0,
  "next_offset": 100
}
```

**Next Page**:
```bash
GET /api/v1/agents?limit=100&offset=100
```

## Filtering

Many endpoints support filtering:

**Query Syntax**:
```
?field=value&field2=value2
```

**Examples**:
```bash
# Filter agents by datacenter and role
GET /api/v1/agents?datacenter=us-east-1&role=web

# Filter events by type and severity
GET /api/v1/events?type=agent.disconnect&severity=warning

# Multiple values (OR)
GET /api/v1/agents?role=web,db,cache
```

## Webhooks

Keystone Core can send webhooks for events:

### Configure Webhook

```bash
POST /api/v1/webhooks
```

**Request Body**:
```json
{
  "url": "https://example.com/webhook",
  "events": ["agent.disconnect", "job.fail"],
  "secret": "webhook-secret"
}
```

### Webhook Payload

```json
{
  "webhook_id": "wh-abc123",
  "event": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "type": "agent.disconnect",
    "timestamp": "2024-01-15T10:30:45Z",
    "data": {...}
  },
  "signature": "sha256=abc123..."
}
```

### Verify Signature

```python
import hmac
import hashlib

def verify_webhook(payload, signature, secret):
    expected = hmac.new(
        secret.encode(),
        payload.encode(),
        hashlib.sha256
    ).hexdigest()
    return hmac.compare_digest(f"sha256={expected}", signature)
```

## Client Libraries

Official client libraries:

### Go

```go
import "github.com/kscore/keystone-core/pkg/client"

client := client.New("http://control-plane:8080", apiKey)
agents, err := client.Agents().List(ctx, &client.ListAgentsOptions{
    Environment: "production",
})
```

### Python

```python
from kscore import Client

client = Client("http://control-plane:8080", api_key)
agents = client.agents.list(environment="production")
```

### JavaScript/TypeScript

```typescript
import { Keystone CoreClient } from '@kscore/client';

const client = new Keystone CoreClient({
  baseURL: 'http://control-plane:8080',
  apiKey: process.env.TITAN_API_KEY
});

const agents = await client.agents.list({
  environment: 'production'
});
```

## Examples

### Execute Command and Wait

```bash
#!/bin/bash
API_KEY="ta_live_abc123"
BASE_URL="http://control-plane:8080/api/v1"

# Execute command
JOB_ID=$(curl -s -X POST \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "command": "systemctl restart nginx",
    "target": "role:web",
    "async": true
  }' \
  "$BASE_URL/exec" | jq -r '.job_id')

# Poll for completion
while true; do
  STATUS=$(curl -s -H "Authorization: Bearer $API_KEY" \
    "$BASE_URL/jobs/$JOB_ID" | jq -r '.status')

  if [ "$STATUS" = "completed" ] || [ "$STATUS" = "failed" ]; then
    break
  fi

  sleep 2
done

# Get results
curl -s -H "Authorization: Bearer $API_KEY" \
  "$BASE_URL/jobs/$JOB_ID" | jq
```

### Apply State with Variables

```bash
STATE=$(cat <<'EOF'
nginx_package:
  module: package
  state: installed
  name: nginx

nginx_config:
  module: file
  state: present
  path: /etc/nginx/nginx.conf
  contents: |
    worker_processes {{ .vars.worker_processes }};
EOF
)

curl -X POST \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d "{
    \"state\": $(echo "$STATE" | jq -Rs .),
    \"target\": \"role:web\",
    \"vars\": {
      \"worker_processes\": 4
    }
  }" \
  "$BASE_URL/state/apply"
```

### Subscribe to Events (gRPC)

```python
import grpc
from kscore.proto import event_service_pb2, event_service_pb2_grpc

channel = grpc.insecure_channel('control-plane:9090')
stub = event_service_pb2_grpc.EventServiceStub(channel)

request = event_service_pb2.SubscribeEventsRequest(
    filter="type =~ 'agent.*'"
)

for event in stub.SubscribeEvents(request):
    print(f"Event: {event.type} from {event.source}")
```

## See Also

- [CLI Reference](../cli/) - Command-line interface
- [Configuration Reference](../configuration/) - Configuration options
- [Event Reference](../events/) - Event schemas
- [Getting Started](../../getting-started/) - Quick start guide
