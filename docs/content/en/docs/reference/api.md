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

**OpenAPI Specification**: A complete OpenAPI 3.0 specification is available at [`api/openapi/openapi-spec.yaml`](https://github.com/keystone-core/keystone-core/blob/main/api/openapi/openapi-spec.yaml). Use this for:
- Generating client SDKs in any language
- Importing into API tools (Postman, Insomnia, etc.)
- Viewing interactive documentation via Swagger UI or Redoc

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
API Key: <your-api-key>
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

### Cluster (HA Only)

These endpoints are available only when running in high-availability cluster mode.

#### Get Cluster Status

```http
GET /api/v1/cluster/status
```

**Response**:
```json
{
  "cluster_name": "kscore-prod",
  "has_quorum": true,
  "quorum_size": 2,
  "member_count": 3,
  "healthy_count": 3,
  "leader_id": "server-1"
}
```

#### List Cluster Members

```http
GET /api/v1/cluster/members
```

**Response**:
```json
{
  "members": [
    {
      "id": "server-1",
      "address": "192.168.1.10:5000",
      "status": "healthy",
      "is_leader": true,
      "agent_count": 50,
      "last_heartbeat": "2024-01-15T10:30:45Z"
    },
    {
      "id": "server-2",
      "address": "192.168.1.11:5000",
      "status": "healthy",
      "is_leader": false,
      "agent_count": 48,
      "last_heartbeat": "2024-01-15T10:30:44Z"
    },
    {
      "id": "server-3",
      "address": "192.168.1.12:5000",
      "status": "healthy",
      "is_leader": false,
      "agent_count": 52,
      "last_heartbeat": "2024-01-15T10:30:45Z"
    }
  ],
  "total": 3
}
```

#### Add Cluster Member

```http
POST /api/v1/cluster/members
```

Pre-registers a cluster member before it starts. This enables external orchestration tools (Kubernetes operators, Terraform) to prepare the cluster for new members.

**Request Body**:
```json
{
  "id": "server-4",
  "name": "server-4",
  "address": "192.168.1.13:5000",
  "grpc_address": "192.168.1.13:5001",
  "nats_address": "192.168.1.13:4222",
  "metadata": {
    "region": "us-west-2",
    "zone": "a"
  }
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `id` | No | Unique member ID (auto-generated UUID if not provided) |
| `name` | No | Human-readable name (auto-generated from ID if not provided) |
| `address` | **Yes** | Primary address for the member (host:port) |
| `grpc_address` | No | gRPC API address (defaults to `address` if not provided) |
| `nats_address` | No | NATS address for messaging |
| `metadata` | No | Custom key-value metadata |

**Response** (201 Created):
```json
{
  "id": "server-4",
  "name": "server-4",
  "address": "192.168.1.13:5000",
  "grpc_address": "192.168.1.13:5001",
  "nats_address": "192.168.1.13:4222",
  "status": "unknown",
  "metadata": {
    "region": "us-west-2",
    "zone": "a"
  },
  "joined_at": "2024-01-15T10:35:00Z"
}
```

**Error Responses**:
- `400 Bad Request`: Missing required `address` field
- `409 Conflict`: Member ID or address already exists

**Notes**:
- The member starts with `unknown` status until it actually starts and begins heartbeating
- Members added via API are stored persistently (no lease) so they survive control plane restarts
- Once the member starts, it will be detected and its status will update to `healthy`

#### Get Current Leader

```http
GET /api/v1/cluster/leader
```

**Response**:
```json
{
  "leader_id": "server-1",
  "address": "192.168.1.10:5000",
  "elected_at": "2024-01-14T08:00:00Z"
}
```

#### Transfer Leadership

```http
POST /api/v1/cluster/leader/transfer
```

Transfers cluster leadership to a specific member. Useful for planned maintenance or load balancing.

**Request Body**:
```json
{
  "target_id": "server-2"
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `target_id` | **Yes** | ID of the member to transfer leadership to |

**Response** (200 OK):
```json
{
  "message": "Leadership transfer initiated",
  "target_id": "server-2"
}
```

**Error Responses**:
- `400 Bad Request`: Missing or empty `target_id`
- `500 Internal Server Error`: Transfer failed (target unreachable, not eligible, etc.)

**Notes**:
- The target member must be healthy and part of the cluster
- Transfer is asynchronous - leader election may take a few seconds
- Use `GET /api/v1/cluster/leader` to verify the transfer completed

**Example**:
```bash
# Transfer leadership to server-2
curl -X POST \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"target_id": "server-2"}' \
  http://control-plane:8080/api/v1/cluster/leader/transfer
```

#### Create Cluster Backup

```http
GET /api/v1/cluster/backup
```

Creates a complete backup of the cluster state including membership, shard assignments, and configuration.

**Response**:
```json
{
  "version": "1.0",
  "timestamp": "2024-01-15T10:30:45Z",
  "cluster": {
    "name": "kscore-prod",
    "quorum_size": 2,
    "leader_id": "server-1",
    "members": [
      {
        "id": "server-1",
        "address": "192.168.1.10:5000",
        "status": "healthy",
        "is_leader": true
      },
      {
        "id": "server-2",
        "address": "192.168.1.11:5000",
        "status": "healthy",
        "is_leader": false
      }
    ]
  },
  "shards": [
    {
      "agent_id": "web-01",
      "member_id": "server-1",
      "assigned_at": "2024-01-10T08:00:00Z"
    },
    {
      "agent_id": "web-02",
      "member_id": "server-2",
      "assigned_at": "2024-01-10T08:01:00Z"
    }
  ],
  "config": {
    "setting1": "value1",
    "setting2": "value2"
  }
}
```

**Example** (save backup to file):
```bash
curl -H "Authorization: Bearer $API_KEY" \
  http://control-plane:8080/api/v1/cluster/backup > cluster-backup.json
```

#### Restore Cluster from Backup

```http
POST /api/v1/cluster/restore
```

Restores cluster state from a backup. Use with caution in production.

**Query Parameters**:
- `force` (bool): Override safety checks (default: false)
- `restore_shards` (bool): Restore shard assignments (default: true)
- `restore_config` (bool): Restore cluster configuration (default: true)

**Request Body**: The backup JSON from `GET /api/v1/cluster/backup`

**Response**:
```json
{
  "success": true,
  "message": "Cluster restored successfully",
  "shards_restored": 150,
  "config_restored": 5,
  "warnings": [
    "Agent web-05 assigned to unavailable member server-3, reassigned to server-1"
  ]
}
```

**Safety Checks**:
- Backup version must be compatible (1.0 supported)
- Backup must have valid timestamp
- Cluster name must match (prevents restoring wrong backup)
- Cluster should not be healthy (use `?force=true` to override)

**Example**:
```bash
# Restore with default options
curl -X POST \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d @cluster-backup.json \
  http://control-plane:8080/api/v1/cluster/restore

# Force restore on healthy cluster (use with caution)
curl -X POST \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d @cluster-backup.json \
  "http://control-plane:8080/api/v1/cluster/restore?force=true"

# Restore only configuration (not shard assignments)
curl -X POST \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d @cluster-backup.json \
  "http://control-plane:8080/api/v1/cluster/restore?restore_shards=false"
```

#### Trigger Agent Rebalance

```http
POST /api/v1/cluster/rebalance
```

Redistributes agents across cluster members for better load balancing.

**Response**:
```json
{
  "success": true,
  "agents_moved": 15,
  "before": {
    "server-1": 80,
    "server-2": 40,
    "server-3": 30
  },
  "after": {
    "server-1": 50,
    "server-2": 50,
    "server-3": 50
  }
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

### ClusterService

```protobuf
service ClusterService {
  rpc GetClusterStatus(GetClusterStatusRequest) returns (ClusterStatus);
  rpc ListMembers(ListMembersRequest) returns (ListMembersResponse);
  rpc GetLeader(GetLeaderRequest) returns (LeaderInfo);
  rpc CreateBackup(CreateBackupRequest) returns (BackupData);
  rpc RestoreBackup(RestoreBackupRequest) returns (RestoreResult);
  rpc TriggerRebalance(RebalanceRequest) returns (RebalanceResult);
}
```

### CoordinationService (Server-to-Server)

The CoordinationService provides server-to-server coordination when NATS is unavailable. It requires mTLS authentication.

```protobuf
service CoordinationService {
  // ClusterHealth returns cluster health from this server's perspective
  rpc ClusterHealth(ClusterHealthRequest) returns (ClusterHealthResponse);

  // GetLeader returns current cluster leader information
  rpc GetLeader(GetLeaderRequest) returns (GetLeaderResponse);

  // NATSStatus returns NATS connectivity status for this server
  rpc NATSStatus(NATSStatusRequest) returns (NATSStatusResponse);

  // RecoveryCoordinate coordinates NATS recovery actions
  rpc RecoveryCoordinate(RecoveryCoordinateRequest) returns (RecoveryCoordinateResponse);

  // Heartbeat performs lightweight liveness check between servers
  rpc Heartbeat(ServerHeartbeatRequest) returns (ServerHeartbeatResponse);

  // PropagateState propagates state changes when NATS is down
  rpc PropagateState(PropagateStateRequest) returns (PropagateStateResponse);
}
```

**RecoveryAction Types**:
- `RESTART_EMBEDDED` - Restart the embedded NATS server (embedded mode only)
- `RECONNECT` - Force reconnection to NATS servers
- `FAILOVER` - Switch to backup NATS servers (pass `target_urls` in parameters)
- `DRAIN` - Gracefully drain all NATS connections
- `PAUSE` - Pause operations during recovery
- `RESUME` - Resume normal operations

**StateUpdateType Types**:
- `AGENT_REGISTER` - Agent registration propagation
- `AGENT_HEARTBEAT` - Agent heartbeat propagation
- `AGENT_DISCONNECT` - Agent disconnect propagation
- `COMMAND_RESULT` - Command result propagation
- `MEMBERSHIP_CHANGE` - Cluster membership change propagation

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

Use the standard `net/http` package to interact with the REST API:

```go
import (
    "encoding/json"
    "net/http"
)

req, _ := http.NewRequest("GET", "http://control-plane:8080/api/v1/agents?environment=production", nil)
req.Header.Set("Authorization", "Bearer "+apiKey)

resp, err := http.DefaultClient.Do(req)
if err != nil {
    return err
}
defer resp.Body.Close()

var result struct {
    Agents []map[string]interface{} `json:"agents"`
}
json.NewDecoder(resp.Body).Decode(&result)
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
  apiKey: process.env.KSCORE_API_KEY
});

const agents = await client.agents.list({
  environment: 'production'
});
```

## Examples

### Execute Command and Wait

```bash
#!/bin/bash
API_KEY="<your-api-key>"
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

## API Versioning and Compatibility Policy

This section defines how Keystone Core versions its APIs and maintains backward compatibility.

### Version Scheme

#### REST API

The REST API uses URL path versioning:

```
/api/v1/agents
/api/v2/agents
```

**Current Versions**:
- `v1`: Stable, recommended for most use cases
- `v2`: Latest features, may have breaking changes in minor releases until GA

**Version Header**:
You can also specify the version via header:

```bash
curl -H "Accept: application/vnd.kscore.v2+json" \
  http://control-plane:8080/api/agents
```

#### gRPC API

gRPC uses package versioning:

```protobuf
// v1 API
package kscore.api.v1;

// v2 API
package kscore.api.v2;
```

### Compatibility Guarantees

#### Stable (v1) API

For stable API versions, we guarantee:

| Change Type | Allowed | Notes |
|-------------|---------|-------|
| Add new endpoint | ✅ Yes | Non-breaking |
| Add optional field | ✅ Yes | Non-breaking |
| Add new enum value | ✅ Yes | Clients must handle unknown values |
| Add new error code | ✅ Yes | Non-breaking |
| Remove endpoint | ❌ No | Breaking change |
| Remove field | ❌ No | Breaking change |
| Change field type | ❌ No | Breaking change |
| Change field semantics | ❌ No | Breaking change |
| Rename endpoint | ❌ No | Breaking change |
| Remove enum value | ❌ No | Breaking change |

**Deprecation Notice**: Deprecated endpoints remain available for at least 12 months before removal.

#### Preview (Beta) API

For preview API versions (e.g., `v2-beta`):

| Change Type | Allowed | Notes |
|-------------|---------|-------|
| All additive changes | ✅ Yes | |
| Breaking changes | ⚠️ Yes | With 30-day notice |
| Endpoint removal | ⚠️ Yes | With 30-day notice |

### API Lifecycle

```
Alpha → Beta → Stable → Deprecated → Removed
  ↓       ↓       ↓          ↓           ↓
 Dev   Preview   GA      6 months    12 months
                        notice       after dep.
```

| Stage | Stability | SLA | Support |
|-------|-----------|-----|---------|
| Alpha | Unstable | None | None |
| Beta | Semi-stable | None | Best effort |
| Stable | Stable | 99.9% | Full support |
| Deprecated | Stable | 99.9% | Security only |
| Removed | N/A | N/A | N/A |

### Version Discovery

```bash
# List supported API versions
curl http://control-plane:8080/api/versions

# Response
{
  "versions": [
    {
      "version": "v1",
      "status": "stable",
      "default": true
    },
    {
      "version": "v2",
      "status": "beta",
      "default": false,
      "sunset_date": null
    }
  ],
  "deprecated_versions": [
    {
      "version": "v1beta1",
      "status": "deprecated",
      "sunset_date": "2025-06-01",
      "migration_guide": "https://docs.kscore.io/api/migrate-v1beta1-to-v1"
    }
  ]
}
```

### Deprecation Policy

#### Marking Deprecation

Deprecated endpoints include warning headers:

```http
HTTP/1.1 200 OK
Deprecation: true
Sunset: Sat, 01 Jun 2025 00:00:00 GMT
Link: <https://docs.kscore.io/api/migrate-v1-events>; rel="deprecation"
Warning: 299 - "Deprecated API endpoint, migrate to /api/v2/events"
```

#### Deprecation Announcements

- **API changelog**: All deprecations documented
- **Release notes**: Major deprecations highlighted
- **Email notification**: For registered API consumers
- **In-API warnings**: As shown above

#### Migration Timeline

| Milestone | Timeline |
|-----------|----------|
| Deprecation announced | T+0 |
| Warning headers added | T+0 |
| Deprecation notice in docs | T+0 |
| Migration guide published | T+0 |
| Email notification sent | T+0 |
| API marked deprecated in discovery | T+0 |
| Last day of support | T+12 months |
| API removed | T+12 months |

### Backward Compatibility Guidelines

#### Client Implementation Recommendations

```python
# Good: Handle unknown fields gracefully
def process_agent(agent_data):
    # Only access fields you need
    agent_id = agent_data.get('id')
    status = agent_data.get('status', 'unknown')
    # Ignore unknown fields

# Good: Handle unknown enum values
def handle_status(status):
    if status in ('connected', 'disconnected', 'degraded'):
        return status
    return 'unknown'  # Handle future enum values

# Good: Check API version
def check_version():
    versions = client.get_versions()
    if 'v2' in [v['version'] for v in versions['versions']]:
        return 'v2'
    return 'v1'
```

#### Handling API Changes

```python
# Check for deprecated endpoints
import warnings

response = client.get('/api/v1/old-endpoint')
if 'Deprecation' in response.headers:
    sunset = response.headers.get('Sunset')
    warnings.warn(
        f"API endpoint deprecated, will be removed: {sunset}",
        DeprecationWarning
    )
```

### Version Migration

#### v1beta1 → v1 Migration

```bash
# Before (v1beta1)
curl /api/v1beta1/agents?filter=role:web

# After (v1)
curl /api/v1/agents?role=web
```

| v1beta1 | v1 | Notes |
|---------|-----|-------|
| `filter=field:value` | `field=value` | Query param syntax change |
| `agent.metadata.os` | `agent.os` | Field moved |
| `job.state` | `job.status` | Field renamed |

#### v1 → v2 Migration (Preview)

```bash
# Before (v1)
curl /api/v1/agents/web-01/execute \
  -d '{"command": "hostname"}'

# After (v2)
curl /api/v2/agents/web-01/commands \
  -d '{"command": "hostname", "options": {"timeout": "30s"}}'
```

| v1 | v2 | Notes |
|-----|-----|-------|
| `/agents/{id}/execute` | `/agents/{id}/commands` | Endpoint renamed |
| `command` (string) | `command` (object) | Richer command spec |
| Sync response | Async job | Always returns job ID |

### gRPC Service Evolution

#### Adding New RPCs

```protobuf
// v1 - original
service AgentService {
  rpc ListAgents(ListAgentsRequest) returns (ListAgentsResponse);
  rpc GetAgent(GetAgentRequest) returns (Agent);
}

// v1 - with new RPC (non-breaking)
service AgentService {
  rpc ListAgents(ListAgentsRequest) returns (ListAgentsResponse);
  rpc GetAgent(GetAgentRequest) returns (Agent);
  rpc StreamAgentEvents(StreamAgentEventsRequest) returns (stream AgentEvent);  // New!
}
```

#### Adding New Fields

```protobuf
// Original
message Agent {
  string id = 1;
  string status = 2;
}

// Updated (non-breaking)
message Agent {
  string id = 1;
  string status = 2;
  AgentHealth health = 3;  // New optional field
  repeated string tags = 4;  // New repeated field
}
```

### Error Handling

#### Standard Error Response

```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "Invalid agent ID format",
    "details": [
      {
        "type": "FieldViolation",
        "field": "agent_id",
        "description": "Must be alphanumeric, 3-64 characters"
      }
    ],
    "request_id": "req-12345"
  }
}
```

#### Error Code Stability

| Code | Meaning | Stability |
|------|---------|-----------|
| `OK` | Success | Stable |
| `INVALID_ARGUMENT` | Bad request | Stable |
| `NOT_FOUND` | Resource not found | Stable |
| `ALREADY_EXISTS` | Duplicate resource | Stable |
| `PERMISSION_DENIED` | Authorization failed | Stable |
| `UNAUTHENTICATED` | Authentication failed | Stable |
| `INTERNAL` | Server error | Stable |
| `UNAVAILABLE` | Service unavailable | Stable |

New error codes may be added; clients should handle unknown codes gracefully.

### Rate Limiting

Rate limits are communicated via headers:

```http
HTTP/1.1 429 Too Many Requests
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1705312345
Retry-After: 60
```

Rate limits are per API version and may differ between versions.

### Changelog

Major API changes are documented in the changelog:

```yaml
# api-changelog.yaml
- version: "v1.5.0"
  date: "2025-01-15"
  changes:
    - type: added
      path: "/api/v1/agents/{id}/metrics"
      description: "New endpoint for agent metrics"
    - type: deprecated
      path: "/api/v1/status"
      description: "Use /api/v1/health instead"
      sunset: "2026-01-15"

- version: "v1.4.0"
  date: "2024-10-15"
  changes:
    - type: added
      field: "Agent.health"
      description: "Health status object added to Agent"
```

### SDK Compatibility

| SDK | API v1 Support | API v2 Support | Auto-upgrade |
|-----|----------------|----------------|--------------|
| Go SDK 1.x | ✅ | ❌ | No |
| Go SDK 2.x | ✅ | ✅ | Yes |
| Python SDK 1.x | ✅ | ❌ | No |
| Python SDK 2.x | ✅ | ✅ | Yes |

**Auto-upgrade**: SDK automatically uses highest available API version.

### Testing Against API Versions

```bash
# Run tests against v1 API
KSCORE_API_VERSION=v1 go test ./...

# Run tests against v2 API
KSCORE_API_VERSION=v2 go test ./...

# Run compatibility tests
make test-api-compat
```

## See Also

- [CLI Reference](../cli/) - Command-line interface
- [Configuration Reference](../configuration/) - Configuration options
- [Event Reference](../events/) - Event schemas
- [Getting Started](../../getting-started/) - Quick start guide
