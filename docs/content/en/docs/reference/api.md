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

**OpenAPI Specification**: A complete OpenAPI 3.0 specification is available at [`api/openapi/openapi-spec.yaml`](https://github.com/shawnbutts/keystone-core/blob/main/api/openapi/openapi-spec.yaml). Use this for:

- Generating client SDKs in any language
- Importing into API tools (Postman, Insomnia, etc.)
- Viewing interactive documentation via Swagger UI or Redoc

## Authentication

Both REST and gRPC endpoints require authentication when `auth.enabled` is `true` in the server configuration. The auth type (`apikey`, `jwt`, `mtls`, or `multi`) determines which methods are accepted.

**Unauthenticated endpoints** (always accessible without credentials):

- `/health/ready`, `/health/status` — health checks for load balancers
- `/api/status` — server status for monitoring tools

### API Key Authentication

Include the API key via the `Authorization` header or the `X-API-Key` header:

```bash
# Using Authorization: Bearer header
curl -H "Authorization: Bearer <api-key>" \
  http://control-plane:8080/api/v1/agents

# Using X-API-Key header (configurable via auth.apikey.header_name)
curl -H "X-API-Key: <api-key>" \
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
kscorectl api-key create --name my-app --expires-in 30d

# Output
API Key created successfully!
Name: my-app
Key: ks_...
Expires: 2024-02-14T10:30:00Z
```

## REST API Endpoints

### Server Status

#### Get Server Status

```http
GET /api/status
```

Returns server status information including version, uptime, agent counts, and runtime metrics. This endpoint is used by the monitor TUI and other tools.

> **Note**: This endpoint does not require authentication and is available at `/api/status` (not `/api/v1/status`).

**Response**:

```json
{
  "version": "0.1.0",
  "git_commit": "abc1234",
  "build_date": "2024-01-15T10:00:00Z",
  "uptime_seconds": 3600,
  "started_at": "2024-01-15T09:00:00Z",
  "agents": {
    "total": 150,
    "online": 145,
    "offline": 5
  },
  "runtime": {
    "goroutines": 50,
    "memory_alloc_mb": 128.5,
    "memory_sys_mb": 256.0,
    "gc_runs": 42
  },
  "health": "healthy"
}
```

**Example**:

```bash
curl http://control-plane:8080/api/status
```

### Agents

#### List Agents

```http
GET /api/v1/agents
```

**Query Parameters**:

- `status` (string): Filter by status (`connected`, `disconnected`, `unknown`)
- `labels` (string): Filter by labels (`key=value,key2=value2` format)
- `sort` (string): Sort field (default: `hostname`)

**Response**:

```json
{
  "agents": [
    {
      "id": "web-01",
      "hostname": "web-01.example.com",
      "os": "linux",
      "arch": "amd64",
      "agent_version": "0.1.0",
      "platform_version": "6.1.0-generic",
      "status": "connected",
      "labels": {"service": "nginx", "tier": "frontend"},
      "ip_addresses": ["10.0.1.100"],
      "ipv4_addresses": ["10.0.1.100"],
      "ipv6_addresses": ["fd00::1"],
      "is_dual_stack": true,
      "registered_at": "2024-01-10T08:00:00Z",
      "last_seen": "2024-01-15T10:30:45Z"
    }
  ],
  "total": 150,
  "online": 148,
  "offline": 2,
  "retrieved_at": "2024-01-15T10:31:00Z"
}
```

**Example**:

```bash
curl -H "Authorization: Bearer $API_KEY" \
  "http://control-plane:8080/api/v1/agents?status=connected&labels=tier=frontend"
```

#### Get Agent

```http
GET /api/v1/agents/{agent_id}
```

**Response**:

```json
{
  "id": "web-01",
  "hostname": "web-01.example.com",
  "os": "linux",
  "arch": "amd64",
  "agent_version": "0.1.0",
  "platform_version": "6.1.0-generic",
  "status": "connected",
  "labels": {"service": "nginx", "tier": "frontend"},
  "ip_addresses": ["10.0.1.100"],
  "ipv4_addresses": ["10.0.1.100"],
  "ipv6_addresses": ["fd00::1"],
  "is_dual_stack": true,
  "registered_at": "2024-01-10T08:00:00Z",
  "last_seen": "2024-01-15T10:30:45Z"
}
```

**Example**:

```bash
curl -H "Authorization: Bearer $API_KEY" \
  http://control-plane:8080/api/v1/agents/web-01
```

#### Update Agent Tags

```http
PATCH /api/v1/agents/{id}/tags
```

**Request Body**:

```json
{
  "tags": {
    "monitored": "true",
    "backup": "enabled",
    "old-tag": ""
  }
}
```

> **Note**: Set a tag value to empty string `""` to delete it.

**Response**:

```json
{
  "agent_id": "web-01",
  "tags": {"service": "nginx", "tier": "frontend", "monitored": "true", "backup": "enabled"},
  "updated": true
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
  "targets": ["web-01", "web-02"],
  "timeout": 30,
  "async": false
}
```

**Parameters**:

- `command` (string, required): Command to execute
- `targets` (array): List of target agent IDs
- `target` (string): Single target agent ID (alternative to `targets`)
- `args` (array): Command arguments
- `env` (object): Environment variables
- `working_dir` (string): Working directory
- `timeout` (int): Execution timeout in seconds (default: 60)
- `async` (bool): Run asynchronously (default: false)
- `user` (string): User to execute as
- `shell` (string): Shell to use for execution

**Synchronous Response**:

```json
{
  "job_id": "job-abc123",
  "status": "completed",
  "targets": ["web-01", "web-02"],
  "results": {
    "web-01": {
      "agent_id": "web-01",
      "status": "success",
      "exit_code": 0,
      "stdout": "Restarted nginx.service\n",
      "stderr": "",
      "completed_at": "2024-01-15T10:30:47Z",
      "duration_ms": 1200
    },
    "web-02": {
      "agent_id": "web-02",
      "status": "success",
      "exit_code": 0,
      "stdout": "Restarted nginx.service\n",
      "stderr": "",
      "completed_at": "2024-01-15T10:30:48Z",
      "duration_ms": 1500
    }
  },
  "created_at": "2024-01-15T10:30:45Z"
}
```

**Asynchronous Response**:

```json
{
  "job_id": "job-abc123",
  "status": "dispatched",
  "targets": ["web-01", "web-02"],
  "results": {
    "web-01": {"agent_id": "web-01", "status": "dispatched"},
    "web-02": {"agent_id": "web-02", "status": "dispatched"}
  },
  "created_at": "2024-01-15T10:30:45Z"
}
```

**Example**:

```bash
curl -X POST -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "command": "uptime",
    "targets": ["web-01"],
    "timeout": 10
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
  "id": "job-abc123",
  "agent_id": "web-01",
  "command": "systemctl restart nginx",
  "args": [],
  "status": "success",
  "exit_code": 0,
  "stdout": "Restarted nginx.service\n",
  "stderr": "",
  "created_at": "2024-01-15T10:30:45Z",
  "started_at": "2024-01-15T10:30:45Z",
  "completed_at": "2024-01-15T10:30:47Z",
  "duration_ms": 2100
}
```

#### List Jobs

```http
GET /api/v1/jobs
```

**Query Parameters**:

- `agent_id` (string): Filter by agent ID
- `status` (string): Filter by status (pending, running, success, failed, timeout, cancelled)
- `sort` (string): Sort field
- `order` (string): Sort order (asc, desc)
- `limit` (int): Max results (default: 50)
- `offset` (int): Pagination offset

**Response**:

```json
{
  "jobs": [
    {
      "id": "job-abc123",
      "agent_id": "web-01",
      "command": "systemctl restart nginx",
      "args": [],
      "status": "success",
      "exit_code": 0,
      "stdout": "Restarted nginx.service\n",
      "stderr": "",
      "created_at": "2024-01-15T10:30:45Z",
      "started_at": "2024-01-15T10:30:45Z",
      "completed_at": "2024-01-15T10:30:47Z",
      "duration_ms": 2100
    }
  ],
  "total": 1,
  "limit": 50,
  "offset": 0,
  "retrieved_at": "2024-01-15T10:35:00Z"
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
  "content": "states:\n  nginx_package:\n    module: package\n    params:\n      name: nginx",
  "vars": {
    "db_host": "postgres.example.com"
  },
  "dry_run": false
}
```

**Parameters**:

- `content` (string): State file YAML content (required if `path` not provided)
- `path` (string): Path to state file on server (required if `content` not provided)
- `vars` (object): Template variables
- `dry_run` (bool): Dry-run mode (default: false)
- `targets` (array): Target specific state IDs

**Response**:

```json
{
  "run_id": "run-xyz789",
  "status": "changed",
  "summary": {
    "total": 3,
    "succeeded": 3,
    "failed": 0,
    "changed": 2,
    "unchanged": 1,
    "skipped": 0
  },
  "results": [
    {
      "id": "nginx_package",
      "module": "package",
      "status": "success",
      "changed": false,
      "comment": "Package nginx already installed",
      "duration": "1.2s"
    },
    {
      "id": "nginx_config",
      "module": "file",
      "status": "success",
      "changed": true,
      "comment": "File /etc/nginx/nginx.conf updated",
      "duration": "0.5s"
    },
    {
      "id": "nginx_service",
      "module": "service",
      "status": "success",
      "changed": true,
      "comment": "Service nginx restarted",
      "duration": "2.1s"
    }
  ],
  "started_at": "2024-01-15T10:30:45Z",
  "duration": "3.8s",
  "dry_run": false
}
```

#### Check State (Validate)

```http
POST /api/v1/state/check
```

Validates state file syntax and structure without applying.

**Request Body**:

```json
{
  "content": "states:\n  nginx_package:\n    module: package\n    params:\n      name: nginx"
}
```

**Response**:

```json
{
  "valid": true,
  "errors": [],
  "warnings": [],
  "states": 3,
  "modules": ["package", "file", "service"]
}
```

**Validation Error Example**:

```json
{
  "valid": false,
  "errors": [
    {
      "state_id": "nginx_config",
      "field": "params.path",
      "message": "required field 'path' is missing",
      "line": 12
    }
  ],
  "warnings": [],
  "states": 3,
  "modules": ["package", "file", "service"]
}
```

#### Detect Drift

```http
POST /api/v1/state/drift
```

**Request Body**:

```json
{
  "content": "states:\n  nginx_service:\n    module: service\n    params:\n      name: nginx\n      state: running"
}
```

**Response**:

```json
{
  "run_id": "drift-abc123",
  "has_drift": true,
  "summary": {
    "total": 3,
    "no_drift": 2,
    "low": 0,
    "medium": 0,
    "high": 1,
    "critical": 0
  },
  "states": [
    {
      "state_id": "nginx_service",
      "module": "service",
      "has_drift": true,
      "severity": "high",
      "differences": [
        {
          "path": "state",
          "expected": "running",
          "actual": "stopped",
          "severity": "high",
          "message": "Service is not running"
        }
      ]
    }
  ],
  "checked_at": "2024-01-15T10:30:45Z",
  "duration": "1.5s"
}
```

### Events

#### List Events

```http
GET /api/v1/events
```

**Query Parameters**:

- `type` (string): Filter by event type (comma-separated for multiple)
- `source` (string): Filter by source (comma-separated for multiple)
- `severity` (string): Filter by severity level (comma-separated for multiple)
- `correlation_id` (string): Filter by correlation ID
- `tags` (string): Filter by tags (format: `key=value,key2=value2`)
- `start` (string): Events from timestamp (RFC3339)
- `end` (string): Events until timestamp (RFC3339)
- `sort` (string): Sort field (default: `time`)
- `order` (string): Sort direction (`asc` or `desc`, default: `desc`)
- `limit` (int): Max results (default: 50)
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
      "tags": {"env": "production", "region": "us-east-1"},
      "data": {
        "agent_id": "web-01",
        "datacenter": "us-east-1",
        "environment": "production"
      }
    }
  ],
  "total": 10000,
  "limit": 50,
  "offset": 0,
  "retrieved_at": "2024-01-15T10:31:00Z"
}
```

#### Get Event

```http
GET /api/v1/events/{id}
```

**Response**:

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
    "datacenter": "us-east-1"
  }
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
  "correlation_id": "alert-123",
  "tags": {"type": "monitoring"},
  "data": {
    "alert": "disk usage high",
    "threshold": 90
  }
}
```

**Parameters**:

- `type` (string, required): Event type
- `source` (string, required): Event source
- `severity` (string): Severity level (default: `info`)
- `correlation_id` (string): Correlation ID for grouping related events
- `tags` (object): Key-value tags
- `data` (object): Event payload data

**Response** (201 Created):

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "type": "user.custom",
  "source": "external-system",
  "timestamp": "2024-01-15T10:30:45Z",
  "severity": "warning",
  "correlation_id": "alert-123",
  "tags": {"type": "monitoring"},
  "data": {
    "alert": "disk usage high",
    "threshold": 90
  }
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
  "resource": {
    "type": "file",
    "path": "/etc/ssh/sshd_config",
    "contents": "Port 2222\nPermitRootLogin no\n"
  },
  "action": "apply",
  "user": "admin",
  "context": {}
}
```

**Parameters**:

- `policy_id` (string): Evaluate a single policy
- `policy_set_id` (string): Evaluate a policy set (alternative to `policy_id`)
- `resource_type` (string): Evaluate all policies bound to resource type (alternative)
- `resource` (object): The resource being evaluated
- `action` (string, required): Action being performed
- `user` (string): User performing the action
- `context` (object): Additional evaluation context

> **Note**: One of `policy_id`, `policy_set_id`, or `resource_type` is required.

**Response**:

```json
{
  "allowed": true,
  "results": [
    {
      "policy_id": "ssh-hardening",
      "policy_name": "SSH Hardening Policy",
      "allowed": true,
      "violations": [],
      "warnings": [],
      "message": "All checks passed",
      "duration": "5ms",
      "evaluated_at": "2024-01-15T10:30:45Z"
    }
  ],
  "summary": {
    "total": 1,
    "allowed": 1,
    "denied": 0
  },
  "total_duration": "5ms",
  "evaluated_at": "2024-01-15T10:30:45Z"
}
```

#### List Policy Violations

```http
GET /api/v1/policies/violations
```

**Query Parameters**:

- `policy_id` (string): Filter by policy ID
- `resource_type` (string): Filter by resource type
- `user` (string): Filter by user
- `action` (string): Filter by action
- `start` (string): Violations from timestamp (RFC3339)
- `end` (string): Violations until timestamp (RFC3339)
- `limit` (int): Max results (default: 100)

**Response**:

```json
{
  "violations": [
    {
      "id": "viol-abc123",
      "timestamp": "2024-01-15T10:30:45Z",
      "policy_id": "ssh-hardening",
      "policy_name": "SSH Hardening Policy",
      "resource_type": "file",
      "user": "deploy-bot",
      "action": "apply",
      "violations": [
        {
          "rule": "default-port",
          "message": "SSH must not use default port 22",
          "severity": "high"
        }
      ],
      "enforcement_mode": "enforce"
    }
  ],
  "summary": {
    "total_evaluations": 100,
    "violations": 5,
    "by_severity": {"high": 3, "medium": 2}
  },
  "total": 5,
  "limit": 100,
  "retrieved_at": "2024-01-15T10:35:00Z"
}
```

#### Get Compliance Report

```http
GET /api/v1/policies/compliance
```

**Query Parameters**:

- `start` (string): Period start timestamp (RFC3339, default: 24 hours ago)
- `end` (string): Period end timestamp (RFC3339, default: now)

**Response**:

```json
{
  "report": {
    "total_policies": 10,
    "compliant_policies": 8,
    "compliance_rate": 80.0,
    "top_violations": [
      {
        "policy_id": "ssh-hardening",
        "policy_name": "SSH Hardening Policy",
        "count": 15,
        "severity": "high"
      }
    ],
    "severity_breakdown": {
      "critical": 2,
      "high": 15,
      "medium": 30,
      "low": 10
    }
  },
  "period": {
    "start": "2024-01-14T10:30:45Z",
    "end": "2024-01-15T10:30:45Z"
  },
  "generated_at": "2024-01-15T10:30:45Z"
}
```

### GitOps

#### List Verifications

```http
GET /api/v1/gitops/verifications
```

**Query Parameters**:

- `workflow` (string): Filter by workflow name
- `success` (bool): Filter by success status (`true` or `false`)
- `limit` (int): Max results (default: 50)
- `offset` (int): Pagination offset

**Response**:

```json
{
  "verifications": [
    {
      "id": "verify-abc123",
      "workflow_name": "post-deploy-verification",
      "success": true,
      "steps": [
        {
          "step_name": "HTTP health check",
          "success": true,
          "message": "Endpoint returned 200 OK",
          "duration": "500ms",
          "timestamp": "2024-01-15T10:31:00Z"
        }
      ],
      "total_steps": 2,
      "passed_steps": 2,
      "failed_steps": 0,
      "skipped_steps": 0,
      "duration": "1.2s",
      "start_time": "2024-01-15T10:30:45Z",
      "end_time": "2024-01-15T10:32:00Z"
    }
  ],
  "total": 1,
  "limit": 50,
  "offset": 0,
  "retrieved_at": "2024-01-15T11:00:00Z"
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
  "type": "argocd",
  "strategy": "previous",
  "reason": "Deployment caused increased error rate",
  "requested_by": "ops-team",
  "require_approval": true
}
```

**Parameters**:

- `application` (string, required): Application name
- `type` (string, required): Rollback type (`argocd`, `flux`, `git`, `manual`)
- `reason` (string, required): Reason for rollback
- `namespace` (string): Kubernetes namespace
- `strategy` (string): Rollback strategy (`previous`, `specific`, `last_known_good`)
- `revision` (string): Target revision (for `specific` strategy)
- `requested_by` (string): User requesting the rollback
- `skip_verification` (bool): Skip post-rollback verification
- `require_approval` (bool): Force approval requirement

**Response** (202 Accepted):

```json
{
  "id": "rb-xyz789",
  "application": "myapp",
  "namespace": "production",
  "type": "argocd",
  "strategy": "previous",
  "status": "pending_approval",
  "previous_revision": "abc123",
  "start_time": "2024-01-15T10:30:00Z",
  "approval_info": {
    "required": true,
    "status": "pending"
  }
}
```

#### Get Verification

```http
GET /api/v1/gitops/verifications/{id}
```

**Response**:

```json
{
  "id": "verify-abc123",
  "workflow_name": "post-deploy-verification",
  "success": true,
  "steps": [
    {
      "step_name": "HTTP health check",
      "success": true,
      "message": "Endpoint returned 200 OK",
      "duration": "500ms",
      "timestamp": "2024-01-15T10:31:00Z"
    },
    {
      "step_name": "Database connectivity",
      "success": true,
      "message": "Connection established",
      "duration": "120ms",
      "timestamp": "2024-01-15T10:31:01Z"
    }
  ],
  "total_steps": 2,
  "passed_steps": 2,
  "failed_steps": 0,
  "skipped_steps": 0,
  "duration": "1.2s",
  "start_time": "2024-01-15T10:30:45Z",
  "end_time": "2024-01-15T10:32:00Z"
}
```

#### List Rollbacks

```http
GET /api/v1/gitops/rollbacks
```

**Query Parameters**:

- `application` (string): Filter by application name
- `status` (string): Filter by status (`pending`, `pending_approval`, `in_progress`, `completed`, `failed`)
- `limit` (int): Maximum results (default: 50)
- `offset` (int): Pagination offset

**Response**:

```json
{
  "rollbacks": [
    {
      "id": "rb-xyz789",
      "application": "myapp",
      "namespace": "production",
      "type": "argocd",
      "strategy": "previous",
      "status": "completed",
      "previous_revision": "abc123",
      "current_revision": "def456",
      "message": "Rollback completed successfully",
      "duration": "45s",
      "start_time": "2024-01-15T10:30:00Z",
      "end_time": "2024-01-15T10:30:45Z"
    }
  ],
  "total": 1,
  "limit": 50,
  "offset": 0,
  "retrieved_at": "2024-01-15T11:00:00Z"
}
```

#### Get Rollback

```http
GET /api/v1/gitops/rollbacks/{id}
```

**Response**:

```json
{
  "id": "rb-xyz789",
  "application": "myapp",
  "namespace": "production",
  "type": "argocd",
  "strategy": "previous",
  "status": "completed",
  "previous_revision": "abc123",
  "current_revision": "def456",
  "message": "Rollback completed successfully",
  "duration": "45s",
  "start_time": "2024-01-15T10:30:00Z",
  "end_time": "2024-01-15T10:30:45Z",
  "approval_info": {
    "required": true,
    "status": "approved",
    "approved_by": "ops-lead",
    "approved_at": "2024-01-15T10:29:30Z",
    "reason": "Emergency rollback approved"
  }
}
```

#### Approve/Reject Rollback

```http
POST /api/v1/gitops/rollbacks/{id}/approve
```

**Request Body**:

```json
{
  "approved": true,
  "approved_by": "ops-lead",
  "reason": "Emergency rollback approved"
}
```

**Parameters**:

- `approved` (bool): Whether to approve or reject
- `approved_by` (string, required): User approving/rejecting
- `reason` (string): Reason for approval/rejection

**Response**:
Returns the updated rollback object with approval info.

### Cluster (HA Only)

These endpoints are available only when running in high-availability cluster mode.

#### Get Cluster Status

```http
GET /api/v1/cluster/status
```

**Response**:

```json
{
  "healthy": true,
  "member_count": 3,
  "quorum_size": 2,
  "has_quorum": true,
  "leader_id": "server-1",
  "members": [
    {
      "id": "server-1",
      "address": "192.168.1.10:5000",
      "status": "healthy",
      "is_leader": true,
      "version": "0.1.0",
      "started_at": "2024-01-14T08:00:00Z",
      "last_seen": "2024-01-15T10:30:45Z",
      "agent_count": 50,
      "job_count": 120
    }
  ],
  "updated_at": "2024-01-15T10:30:45Z"
}
```

#### List Cluster Members

```http
GET /api/v1/cluster/members
```

**Response**:

```json
[
  {
    "id": "server-1",
    "address": "192.168.1.10:5000",
    "status": "healthy",
    "is_leader": true,
    "version": "0.1.0",
    "started_at": "2024-01-14T08:00:00Z",
    "last_seen": "2024-01-15T10:30:45Z",
    "agent_count": 50,
    "job_count": 120
  },
  {
    "id": "server-2",
    "address": "192.168.1.11:5000",
    "status": "healthy",
    "is_leader": false,
    "version": "0.1.0",
    "started_at": "2024-01-14T08:00:00Z",
    "last_seen": "2024-01-15T10:30:44Z",
    "agent_count": 48,
    "job_count": 95
  },
  {
    "id": "server-3",
    "address": "192.168.1.12:5000",
    "status": "healthy",
    "is_leader": false,
    "version": "0.1.0",
    "started_at": "2024-01-14T08:00:00Z",
    "last_seen": "2024-01-15T10:30:45Z",
    "agent_count": 52,
    "job_count": 110
  }
]
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
  "id": "server-1",
  "address": "192.168.1.10:5000",
  "status": "healthy",
  "is_leader": true,
  "version": "0.1.0",
  "started_at": "2024-01-14T08:00:00Z",
  "last_seen": "2024-01-15T10:30:45Z",
  "agent_count": 50,
  "job_count": 120
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

> **Note**: This endpoint returns a file download (`Content-Type: application/octet-stream`) with the backup JSON content.

**Response** (file download):

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

**Query Parameters**:

- `reason` (string): Reason for rebalance (default: "API request")

**Response**:

```json
{
  "success": true,
  "reason": "API request",
  "moved_agents": 15,
  "trigger_member_id": "server-1",
  "start_time": "2024-01-15T10:30:00Z",
  "end_time": "2024-01-15T10:30:05Z",
  "duration": "5s"
}
```

### Runbooks

Runbook endpoints manage approval requests and human-in-the-loop interventions during runbook execution.

#### List Approval Requests

```http
GET /api/v1/runbook/approvals
```

**Query Parameters**:

- `state` (string): Filter by state (`pending`, `approved`, `rejected`, `expired`)
- `execution_id` (string): Filter by execution ID
- `limit` (int): Maximum results (default: 50)
- `offset` (int): Pagination offset

**Response**:

```json
{
  "approvals": [
    {
      "id": "apr-123",
      "execution_id": "exec-456",
      "step_name": "deploy-production",
      "state": "pending",
      "title": "Approve Production Deployment",
      "description": "Review and approve deployment to production",
      "approvers": ["ops-team", "lead-engineer"],
      "mode": "any",
      "required_count": 1,
      "responses": [],
      "expires_at": "2024-01-15T12:00:00Z",
      "created_at": "2024-01-15T10:00:00Z",
      "updated_at": "2024-01-15T10:00:00Z"
    }
  ],
  "total": 1,
  "limit": 50,
  "offset": 0,
  "retrieved_at": "2024-01-15T10:30:00Z"
}
```

#### Get Approval Request

```http
GET /api/v1/runbook/approvals/{id}
```

**Response**: Single `ApprovalResponse` object (see list response schema).

#### Approve Request

```http
POST /api/v1/runbook/approvals/{id}/approve
```

**Request Body**:

```json
{
  "approver": "ops-team",
  "comment": "Reviewed and approved"
}
```

**Response**:

```json
{
  "id": "apr-123",
  "state": "approved",
  "responses": [
    {
      "approver": "ops-team",
      "decision": "approved",
      "comment": "Reviewed and approved",
      "responded_at": "2024-01-15T10:35:00Z"
    }
  ],
  "completed_at": "2024-01-15T10:35:00Z"
}
```

#### Reject Request

```http
POST /api/v1/runbook/approvals/{id}/reject
```

**Request Body**:

```json
{
  "approver": "ops-team",
  "comment": "Rejecting due to incomplete testing"
}
```

#### Delegate Approval

```http
POST /api/v1/runbook/approvals/{id}/delegate
```

**Request Body**:

```json
{
  "from": "ops-team",
  "to": "senior-ops"
}
```

#### List Intervention Requests

```http
GET /api/v1/runbook/interventions
```

**Query Parameters**:

- `state` (string): Filter by state (`pending`, `responded`, `cancelled`, `expired`)
- `execution_id` (string): Filter by execution ID
- `type` (string): Filter by type (`confirmation`, `input`, `choice`)
- `limit` (int): Maximum results (default: 50)
- `offset` (int): Pagination offset

**Response**:

```json
{
  "interventions": [
    {
      "id": "int-789",
      "execution_id": "exec-456",
      "step_name": "configure-database",
      "type": "input",
      "state": "pending",
      "title": "Database Configuration Required",
      "description": "Provide database connection parameters",
      "prompts": [
        {
          "name": "db_host",
          "label": "Database Host",
          "type": "string",
          "required": true
        },
        {
          "name": "db_port",
          "label": "Database Port",
          "type": "number",
          "required": true,
          "default": 5432,
          "validation": {
            "min": 1,
            "max": 65535
          }
        }
      ],
      "created_at": "2024-01-15T10:00:00Z",
      "updated_at": "2024-01-15T10:00:00Z"
    }
  ],
  "total": 1,
  "limit": 50,
  "offset": 0,
  "retrieved_at": "2024-01-15T10:30:00Z"
}
```

#### Get Intervention Request

```http
GET /api/v1/runbook/interventions/{id}
```

**Response**: Single `InterventionResponse` object (see list response schema).

#### Respond to Intervention

```http
POST /api/v1/runbook/interventions/{id}/respond
```

**Request Body**:

```json
{
  "operator": "db-admin",
  "confirmed": true,
  "values": {
    "db_host": "db.example.com",
    "db_port": 5432
  },
  "comment": "Using production database"
}
```

**Response**:

```json
{
  "id": "int-789",
  "state": "responded",
  "response": {
    "operator": "db-admin",
    "confirmed": true,
    "values": {
      "db_host": "db.example.com",
      "db_port": 5432
    },
    "comment": "Using production database",
    "responded_at": "2024-01-15T10:40:00Z"
  },
  "completed_at": "2024-01-15T10:40:00Z"
}
```

#### Cancel Intervention

```http
POST /api/v1/runbook/interventions/{id}/cancel
```

**Request Body**:

```json
{
  "operator": "db-admin",
  "comment": "Cancelling - will use different approach"
}
```

## gRPC Services

### AgentService

> **Status:** Generated stubs exist but no server implementation. Not registered in kscore-server. See [Epic 46](../../../epics/46-grpc-service-implementation.md).

The AgentService defines the agent-to-control-plane communication protocol. This service is used by agents to register, send heartbeats, execute commands, and retrieve agent information.

```protobuf
service AgentService {
  // Register registers an agent with the control plane
  rpc Register(RegisterRequest) returns (RegisterResponse);

  // Heartbeat sends periodic health status
  rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse);

  // ExecuteCommand executes a command on the agent
  rpc ExecuteCommand(ExecuteCommandRequest) returns (stream ExecuteCommandResponse);

  // GetAgentInfo retrieves agent information
  rpc GetAgentInfo(GetAgentInfoRequest) returns (GetAgentInfoResponse);
}
```

### ControlPlaneService

The ControlPlaneService is the primary client-facing API for managing agents, executing commands, and viewing execution history.

```protobuf
service ControlPlaneService {
  // GetServerStatus retrieves the server status and runtime information
  rpc GetServerStatus(GetServerStatusRequest) returns (GetServerStatusResponse);

  // ListAgents lists all registered agents
  rpc ListAgents(ListAgentsRequest) returns (ListAgentsResponse);

  // GetAgent retrieves information about a specific agent
  rpc GetAgent(GetAgentRequest) returns (GetAgentResponse);

  // ExecuteCommand executes a command on one or more agents
  rpc ExecuteCommand(ExecuteCommandRequest) returns (stream ExecuteCommandResponse);

  // GetCommandStatus retrieves the status of a command execution
  rpc GetCommandStatus(GetCommandStatusRequest) returns (GetCommandStatusResponse);

  // ListCommands lists command execution history
  rpc ListCommands(ListCommandsRequest) returns (ListCommandsResponse);

  // BatchExecuteCommand executes a command across multiple agents using a target expression
  rpc BatchExecuteCommand(BatchExecuteCommandRequest) returns (stream BatchExecuteCommandResponse);

  // GetBatchJobStatus retrieves the status of a batch job
  rpc GetBatchJobStatus(GetBatchJobStatusRequest) returns (GetBatchJobStatusResponse);

  // ListBatchJobs lists batch job execution history
  rpc ListBatchJobs(ListBatchJobsRequest) returns (ListBatchJobsResponse);
}
```

### StateService

> **Status:** Proto defined but code not generated. No server implementation. See [Epic 46](../../../epics/46-grpc-service-implementation.md).

```protobuf
service StateService {
  // ApplyState applies state declarations to one or more agents
  rpc ApplyState(ApplyStateRequest) returns (stream ApplyStateResponse);

  // CheckState checks state without applying (dry-run mode)
  rpc CheckState(CheckStateRequest) returns (CheckStateResponse);

  // DetectDrift detects configuration drift from desired state
  rpc DetectDrift(DetectDriftRequest) returns (DetectDriftResponse);

  // GetStateHistory retrieves state application history
  rpc GetStateHistory(GetStateHistoryRequest) returns (GetStateHistoryResponse);

  // GetStateStatus retrieves current state status for an agent
  rpc GetStateStatus(GetStateStatusRequest) returns (GetStateStatusResponse);
}
```

### EventService

> **Status:** Proto defined but code not generated. No server implementation. See [Epic 46](../../../epics/46-grpc-service-implementation.md).

```protobuf
service EventService {
  // ListEvents lists events with filtering
  rpc ListEvents(ListEventsRequest) returns (ListEventsResponse);

  // GetEvent retrieves a specific event
  rpc GetEvent(GetEventRequest) returns (GetEventResponse);

  // EmitEvent emits a custom event
  rpc EmitEvent(EmitEventRequest) returns (EmitEventResponse);

  // SubscribeEvents subscribes to events in real-time
  rpc SubscribeEvents(SubscribeEventsRequest) returns (stream Event);

  // GetEventTypes returns available event types
  rpc GetEventTypes(GetEventTypesRequest) returns (GetEventTypesResponse);

  // GetEventStats returns event statistics
  rpc GetEventStats(GetEventStatsRequest) returns (GetEventStatsResponse);
}
```

### PolicyService

> **Status:** Proto defined but code not generated. No server implementation. See [Epic 46](../../../epics/46-grpc-service-implementation.md).

```protobuf
service PolicyService {
  // EvaluatePolicy evaluates a policy against input data
  rpc EvaluatePolicy(EvaluatePolicyRequest) returns (EvaluatePolicyResponse);

  // EvaluatePolicySet evaluates all policies in a policy set
  rpc EvaluatePolicySet(EvaluatePolicySetRequest) returns (EvaluatePolicySetResponse);

  // ListPolicies lists all policies
  rpc ListPolicies(ListPoliciesRequest) returns (ListPoliciesResponse);

  // GetPolicy retrieves a specific policy
  rpc GetPolicy(GetPolicyRequest) returns (GetPolicyResponse);

  // CreatePolicy creates a new policy
  rpc CreatePolicy(CreatePolicyRequest) returns (CreatePolicyResponse);

  // UpdatePolicy updates an existing policy
  rpc UpdatePolicy(UpdatePolicyRequest) returns (UpdatePolicyResponse);

  // DeletePolicy deletes a policy
  rpc DeletePolicy(DeletePolicyRequest) returns (DeletePolicyResponse);

  // ListViolations lists policy violations
  rpc ListViolations(ListViolationsRequest) returns (ListViolationsResponse);

  // GetComplianceReport generates a compliance report
  rpc GetComplianceReport(GetComplianceReportRequest) returns (GetComplianceReportResponse);

  // GetAuditLog retrieves policy evaluation audit log
  rpc GetAuditLog(GetAuditLogRequest) returns (GetAuditLogResponse);

  // ListPolicySets lists policy sets
  rpc ListPolicySets(ListPolicySetsRequest) returns (ListPolicySetsResponse);

  // GetPolicySet retrieves a policy set
  rpc GetPolicySet(GetPolicySetRequest) returns (GetPolicySetResponse);
}
```

### ClusterService

> **Status:** Proto defined but code not generated. No server implementation. See [Epic 46](../../../epics/46-grpc-service-implementation.md).

```protobuf
service ClusterService {
  // GetClusterStatus returns the overall cluster status
  rpc GetClusterStatus(GetClusterStatusRequest) returns (GetClusterStatusResponse);

  // ListMembers lists all cluster members
  rpc ListMembers(ListMembersRequest) returns (ListMembersResponse);

  // GetMember retrieves a specific cluster member
  rpc GetMember(GetMemberRequest) returns (GetMemberResponse);

  // AddMember adds a new member to the cluster
  rpc AddMember(AddMemberRequest) returns (AddMemberResponse);

  // RemoveMember removes a member from the cluster
  rpc RemoveMember(RemoveMemberRequest) returns (RemoveMemberResponse);

  // GetLeader returns the current cluster leader
  rpc GetLeader(GetLeaderRequest) returns (GetLeaderResponse);

  // TransferLeader transfers leadership to another member
  rpc TransferLeader(TransferLeaderRequest) returns (TransferLeaderResponse);

  // Rebalance triggers agent rebalancing across cluster members
  rpc Rebalance(RebalanceRequest) returns (RebalanceResponse);

  // CreateBackup creates a cluster state backup
  rpc CreateBackup(CreateBackupRequest) returns (CreateBackupResponse);

  // RestoreBackup restores cluster state from a backup
  rpc RestoreBackup(RestoreBackupRequest) returns (RestoreBackupResponse);

  // WatchMembership watches for membership changes
  rpc WatchMembership(WatchMembershipRequest) returns (stream MembershipEvent);

  // WatchLeadership watches for leadership changes
  rpc WatchLeadership(WatchLeadershipRequest) returns (stream LeadershipEvent);
}
```

### CoordinationService (Server-to-Server)

> **Status:** Generated stubs and server implementation exist but not registered in kscore-server. See [Epic 46](../../../epics/46-grpc-service-implementation.md).

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

#### ClusterHealth

Returns cluster health from the responding server's perspective.

| Request Field | Type | Description |
|---------------|------|-------------|
| `request_id` | string | Correlation ID |
| `include_members` | bool | Include per-member status details |
| `include_nats` | bool | Include NATS connectivity status |

| Response Field | Type | Description |
|----------------|------|-------------|
| `status` | ClusterHealthStatus | `HEALTHY`, `DEGRADED`, `UNHEALTHY`, `UNKNOWN` |
| `healthy_members` | int32 | Number of healthy members |
| `total_members` | int32 | Total member count |
| `has_quorum` | bool | Whether the cluster has quorum |
| `leader_id` | string | Current leader ID |
| `members` | repeated MemberStatus | Per-member details (if requested) |
| `nats_status` | NATSClusterStatus | NATS cluster details (if requested) |

#### GetLeader

Returns information about the current cluster leader.

| Response Field | Type | Description |
|----------------|------|-------------|
| `has_leader` | bool | Whether a leader exists |
| `leader_id` | string | Leader member ID |
| `leader_address` | string | Leader address (host:port) |
| `leader_term` | int64 | Leadership term/epoch |
| `leader_since` | Timestamp | When leadership was acquired |

#### NATSStatus

Returns NATS connectivity status for the responding server.

| Request Field | Type | Description |
|---------------|------|-------------|
| `requester_id` | string | Server ID making the request |

| Response Field | Type | Description |
|----------------|------|-------------|
| `server_id` | string | Responding server ID |
| `connection_status` | NATSConnectionStatus | `CONNECTED`, `CONNECTING`, `RECONNECTING`, `DISCONNECTED`, `CLOSED` |
| `connected_urls` | repeated string | NATS server URLs currently connected to |
| `jetstream_status` | JetStreamStatus | JetStream availability, domain, stream/consumer counts |
| `last_publish` | Timestamp | Last successful publish |
| `last_subscribe` | Timestamp | Last successful subscribe |
| `error` | string | Error details if unhealthy |

#### RecoveryCoordinate

Coordinates NATS recovery actions across cluster servers.

| Request Field | Type | Description |
|---------------|------|-------------|
| `initiator_id` | string | Server initiating recovery |
| `action` | RecoveryAction | Recovery action to take |
| `target_server` | string | Target NATS server (if applicable) |
| `parameters` | map\<string,string\> | Action-specific parameters |

| Response Field | Type | Description |
|----------------|------|-------------|
| `accepted` | bool | Whether the action was accepted |
| `state` | RecoveryState | `IDLE`, `IN_PROGRESS`, `COMPLETED`, `FAILED` |
| `error` | string | Error message if not accepted |

**RecoveryAction values:**

- `RESTART_EMBEDDED` - Restart the embedded NATS server (embedded mode only)
- `RECONNECT` - Force reconnection to NATS servers
- `FAILOVER` - Switch to backup NATS servers (pass `target_urls` in parameters)
- `DRAIN` - Gracefully drain all NATS connections
- `PAUSE` - Pause operations during recovery
- `RESUME` - Resume normal operations

#### Heartbeat

Lightweight liveness check between servers.

| Request Field | Type | Description |
|---------------|------|-------------|
| `sender_id` | string | Sending server ID |
| `timestamp` | Timestamp | Sender's current time |
| `sequence` | int64 | Sequence number for ordering |

| Response Field | Type | Description |
|----------------|------|-------------|
| `responder_id` | string | Responding server ID |
| `sequence` | int64 | Echoed sequence number |
| `latency` | Duration | Round-trip latency measured by responder |

#### PropagateState

Propagates cluster state changes when NATS is unavailable.

| Request Field | Type | Description |
|---------------|------|-------------|
| `sender_id` | string | Server sending the update |
| `update_type` | StateUpdateType | Type of state update |
| `state_data` | bytes | Serialized state payload |
| `version` | int64 | State version/sequence |
| `state_timestamp` | Timestamp | Original state change time |

| Response Field | Type | Description |
|----------------|------|-------------|
| `applied` | bool | Whether the update was applied |
| `current_version` | int64 | Current version on responding server |
| `error` | string | Error message if not applied |

**StateUpdateType values:**

- `AGENT_REGISTER` - Agent registration propagation
- `AGENT_HEARTBEAT` - Agent heartbeat propagation
- `AGENT_DISCONNECT` - Agent disconnect propagation
- `COMMAND_RESULT` - Command result propagation
- `MEMBERSHIP_CHANGE` - Cluster membership change propagation

### Streaming gRPC Methods

Three gRPC methods use server-side streaming to push real-time updates to clients. All streaming RPCs return a long-lived stream that the server writes to as events occur. Clients should implement reconnection logic with backoff when the stream ends or errors.

#### SubscribeEvents

Subscribes to the event bus in real time. Supports filtering by type, source, severity, tags, and CEL expressions.

**Request fields:**

| Field | Type | Description |
|-------|------|-------------|
| `types` | repeated string | Event type patterns (empty = all) |
| `sources` | repeated string | Filter by event sources (empty = all) |
| `min_severity` | EventSeverity | Minimum severity level |
| `tags` | map\<string,string\> | All tags must match |
| `filter` | string | CEL filter expression |
| `replay_seconds` | int32 | Include historical events from the last N seconds (0 = real-time only) |
| `queue_group` | string | Queue group name for load-balanced consumption |

**Response:** stream of `Event` messages, each containing `id`, `type`, `source`, `severity`, `timestamp`, `data`, and `tags`.

```python
import grpc
from kscore.proto import event_pb2, event_pb2_grpc

channel = grpc.insecure_channel('control-plane:9090')
stub = event_pb2_grpc.EventServiceStub(channel)

request = event_pb2.SubscribeEventsRequest(
    types=["agent.*", "exec.*"],
    replay_seconds=60,
)

for event in stub.SubscribeEvents(request):
    print(f"{event.timestamp} [{event.type}] {event.source}: {event.data}")
```

#### WatchMembership

Watches for cluster membership changes (nodes joining, leaving, failing).

**Request fields:**

| Field | Type | Description |
|-------|------|-------------|
| `types` | repeated MembershipEventType | Filter by event types (empty = all) |
| `member_ids` | repeated string | Filter by member IDs (empty = all) |

**MembershipEventType values:** `JOINED`, `LEFT`, `FAILED`, `RECOVERED`, `UPDATED`

**Response:** stream of `MembershipEvent` messages, each containing `type`, `member` (with `id`, `name`, address, role, status), `timestamp`, and `reason`.

```go
stream, err := clusterClient.WatchMembership(ctx, &clusterpb.WatchMembershipRequest{
    Types: []clusterpb.MembershipEventType{
        clusterpb.MEMBERSHIP_EVENT_TYPE_JOINED,
        clusterpb.MEMBERSHIP_EVENT_TYPE_FAILED,
    },
})
for {
    event, err := stream.Recv()
    if err != nil {
        break
    }
    log.Printf("Member %s %s: %s", event.Member.Id, event.Type, event.Reason)
}
```

#### WatchLeadership

Watches for cluster leadership changes (elections, resignations, failovers).

**Request fields:**

| Field | Type | Description |
|-------|------|-------------|
| `types` | repeated LeadershipEventType | Filter by event types (empty = all) |

**LeadershipEventType values:** `ELECTED`, `RESIGNED`, `LOST`, `TRANSFERRED`

**Response:** stream of `LeadershipEvent` messages, each containing `type`, `leader_id`, `previous_leader_id`, `timestamp`, and `reason`.

```go
stream, err := clusterClient.WatchLeadership(ctx, &clusterpb.WatchLeadershipRequest{})
for {
    event, err := stream.Recv()
    if err != nil {
        break
    }
    log.Printf("Leadership %s: %s -> %s (%s)",
        event.Type, event.PreviousLeaderId, event.LeaderId, event.Reason)
}
```

## Rate Limiting

REST API endpoints are rate-limited when `rate_limit.enabled` is `true` in the server configuration. Limits are configurable via `rate_limit.requests_per_minute` and `rate_limit.burst`.

**Client Identification**:

The `rate_limit.key_extractor` setting controls how clients are identified:

- `ip` (default) — by client IP address (respects `X-Forwarded-For` and `X-Real-IP`)
- `apikey` — by `X-API-Key` header value
- `header` — by a custom header specified in `rate_limit.header_name`

**Response Headers** (included on every response when rate limiting is enabled):

```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1642248645
Retry-After: 30
```

| Header | Description |
|--------|-------------|
| `X-RateLimit-Limit` | Maximum requests allowed per window |
| `X-RateLimit-Remaining` | Requests remaining in current window |
| `X-RateLimit-Reset` | Unix timestamp when the window resets |
| `Retry-After` | Seconds until next request is allowed (only on 429) |

**429 Response**:

```json
{
  "error": "rate_limit_exceeded",
  "message": "Rate limit exceeded"
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

### REST API

REST list endpoints support offset-based pagination with `limit` and `offset` query parameters:

**Request**:

```bash
GET /api/v1/jobs?limit=50&offset=0
```

**Response**:

```json
{
  "jobs": [...],
  "total": 230,
  "limit": 50,
  "offset": 0,
  "retrieved_at": "2025-01-15T10:30:00Z"
}
```

**Next Page** — increment `offset` by `limit`:

```bash
GET /api/v1/jobs?limit=50&offset=50
```

Default `limit` is 50 for most endpoints. If `total` is less than or equal to `offset + limit`, there are no more pages.

### gRPC API

gRPC list methods use cursor-based pagination with `page_size` and `page_token`:

```protobuf
message ListAgentsRequest {
  int32 page_size = 1;
  string page_token = 2;
}
```

The response includes a `next_page_token`. Pass it as `page_token` in the next request to fetch the next page. When `next_page_token` is empty, there are no more results.

## Filtering

Many endpoints support filtering:

**Query Syntax**:

```
?field=value&field2=value2
```

**Examples**:

```bash
# Filter agents by status
GET /api/v1/agents?status=connected

# Filter agents by labels
GET /api/v1/agents?labels=tier=frontend,env=production

# Filter events by type and severity
GET /api/v1/events?type=agent.disconnect&severity=warning
```

## Webhooks

Keystone Core receives inbound webhooks from GitOps tools (ArgoCD, Flux, GitHub, GitLab).
The webhook source is auto-detected from request headers, or can be specified in the request body.

> **Note**: These endpoints are not yet wired into `kscore-server`. The handler exists at `pkg/api/webhooks/` but requires the webhook receiver dependency to be instantiated first.

### Receive Webhook

```http
POST /api/v1/webhooks
```

The source type is detected from headers (`X-GitHub-Event`, `X-GitLab-Event`, `X-Argo-Event`, `X-Flux-Event`).
Alternatively, provide a JSON body with an explicit type:

**Request Body** (optional, for explicit type):

```json
{
  "type": "github",
  "payload": { ... }
}
```

**Response** (202 Accepted):

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "accepted",
  "type": "github",
  "event_type": "push",
  "timestamp": "2024-01-15T10:30:45Z"
}
```

### Webhook Statistics

```http
GET /api/v1/webhooks/stats
```

**Response**:

```json
{
  "total_received": 1500,
  "total_processed": 1495,
  "total_failed": 5,
  "by_type": {
    "github": 1000,
    "argocd": 300,
    "flux": 150,
    "gitlab": 50
  },
  "last_received_time": "2024-01-15T10:30:45Z",
  "last_processed_time": "2024-01-15T10:30:45Z",
  "retrieved_at": "2024-01-15T10:31:00Z"
}
```

### Webhook Configuration

```http
GET /api/v1/webhooks/config
```

**Response**:

```json
{
  "enabled": true,
  "addr": ":9095",
  "path": "/webhooks",
  "auth_type": "hmac",
  "handlers": ["argocd", "flux", "github", "gitlab"],
  "webhook_url": "https://control-plane.example.com:9095/webhooks"
}
```

### Verify Inbound Webhook Signature

When configuring a webhook secret in your GitOps tool, Keystone Core validates the signature on incoming requests. To verify signatures in your own integration tests:

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

req, _ := http.NewRequest("GET", "http://control-plane:8080/api/v1/agents?status=connected", nil)
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
agents = client.agents.list(status="connected")
```

### JavaScript/TypeScript

```typescript
import { Keystone CoreClient } from '@kscore/client';

const client = new Keystone CoreClient({
  baseURL: 'http://control-plane:8080',
  apiKey: process.env.KSCORE_API_KEY
});

const agents = await client.agents.list({
  status: 'connected'
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
from kscore.proto import controlplane_pb2, controlplane_pb2_grpc

channel = grpc.insecure_channel('control-plane:9090')
stub = controlplane_pb2_grpc.ControlPlaneServiceStub(channel)

request = controlplane_pb2.SubscribeEventsRequest(
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
```

**Current Versions**:

- `v1`: Stable, current version, recommended for all use cases

> **Note**: Only API v1 is currently implemented. Future versions will be documented here when available.

**Version Header**:
You can also specify the version via header:

```bash
curl -H "Accept: application/vnd.kscore.v1+json" \
  http://control-plane:8080/api/agents
```

#### gRPC API

gRPC uses package versioning:

```protobuf
// Current v1 API
package keystone.core.v1;
```

> **Note**: Only gRPC v1 services are currently implemented.

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
      "migration_guide": "https://docs.keystone-core.io/api/migrate-v1beta1-to-v1"
    }
  ]
}
```

### Deprecation Policy

#### Marking Deprecation

When endpoints are deprecated, they will include warning headers:

```http
HTTP/1.1 200 OK
Deprecation: true
Sunset: Sat, 01 Jun 2026 00:00:00 GMT
Link: <https://docs.keystone-core.io/api/migration-guide>; rel="deprecation"
Warning: 299 - "Deprecated API endpoint, see migration guide"
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
curl /api/v1beta1/agents?filter=status:connected

# After (v1)
curl /api/v1/agents?status=connected
```

| v1beta1 | v1 | Notes |
|---------|-----|-------|
| `filter=field:value` | `field=value` | Query param syntax change |
| `agent.metadata.os` | `agent.os` | Field moved |
| `job.state` | `job.status` | Field renamed |

#### v1 → v2 Migration (Preview)

```bash
# Before (v1)
curl /api/v1/exec \
  -d '{"target": "web-01", "command": "hostname"}'

# Hypothetical future version
curl /api/v2/agents/web-01/commands \
  -d '{"command": "hostname", "options": {"timeout": "30s"}}'
```

> **Note**: The above v2 example is hypothetical. Only v1 is currently implemented.

| Current (v1) | Future (hypothetical) | Notes |
|-----|-----|-------|
| `/exec` | `/agents/{id}/commands` | Possible endpoint rename |
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
      description: "Use /api/status instead"
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
