---
title: "Secrets API Reference"
weight: 50
description: "Complete API reference for secrets management, including broker, lease manager, rotation orchestrator, and transit engine."
---

## Overview

This document provides the complete API reference for Keystone Core's secrets management system.

## Secrets Broker API

The secrets broker provides unified access to multiple secret backends.

### Get Secret

Retrieve a secret from the configured backend.

**gRPC:**
```protobuf
rpc GetSecret(GetSecretRequest) returns (GetSecretResponse);

message GetSecretRequest {
  string path = 1;              // Secret path
  string version = 2;           // Optional version
  bool force_refresh = 3;       // Bypass cache
  map<string, string> context = 4; // Additional context
}

message GetSecretResponse {
  bytes data = 1;               // Secret data (JSON)
  SecretMetadata metadata = 2;  // Metadata
  LeaseInfo lease = 3;          // Lease information (if dynamic)
}
```

**REST:**
```http
GET /api/v1/secrets/{path}
Authorization: Bearer <token>

Query Parameters:
  version: string     # Optional version
  refresh: boolean    # Bypass cache

Response:
{
  "data": {
    "username": "app_user",
    "password": "secret_value"
  },
  "metadata": {
    "created_at": "2024-01-15T10:00:00Z",
    "version": 3
  },
  "lease": {
    "lease_id": "database/creds/app/abc123",
    "ttl": 3600,
    "renewable": true
  }
}
```

**Go Client:**
```go
import "github.com/shawnbutts/keystone-core/pkg/secrets"

client := secrets.NewClient(conn)

// Get secret
secret, err := client.GetSecret(ctx, &secrets.GetSecretRequest{
    Path: "database/creds/app",
})
if err != nil {
    return err
}

// Access data
username := secret.Data["username"]
password := secret.Data["password"]

// Check lease
if secret.Lease != nil {
    fmt.Printf("Lease expires in %d seconds\n", secret.Lease.TTL)
}
```

### List Secrets

List secrets at a path (if supported by backend).

**gRPC:**
```protobuf
rpc ListSecrets(ListSecretsRequest) returns (ListSecretsResponse);

message ListSecretsRequest {
  string path = 1;        // Base path
  bool recursive = 2;     // Include subdirectories
}

message ListSecretsResponse {
  repeated string keys = 1;  // Secret paths
}
```

**REST:**
```http
GET /api/v1/secrets/{path}?list=true
Authorization: Bearer <token>

Response:
{
  "keys": [
    "database/creds/app",
    "database/creds/readonly",
    "api/keys/production"
  ]
}
```

### Write Secret

Write or update a secret (static secrets only).

**gRPC:**
```protobuf
rpc WriteSecret(WriteSecretRequest) returns (WriteSecretResponse);

message WriteSecretRequest {
  string path = 1;
  map<string, bytes> data = 2;
  map<string, string> metadata = 3;
}

message WriteSecretResponse {
  int64 version = 1;
  string created_at = 2;
}
```

**REST:**
```http
POST /api/v1/secrets/{path}
Authorization: Bearer <token>
Content-Type: application/json

{
  "data": {
    "username": "new_user",
    "password": "new_password"
  }
}

Response:
{
  "version": 4,
  "created_at": "2024-01-15T10:30:00Z"
}
```

### Delete Secret

Delete a secret.

**gRPC:**
```protobuf
rpc DeleteSecret(DeleteSecretRequest) returns (DeleteSecretResponse);

message DeleteSecretRequest {
  string path = 1;
  bool permanent = 2;  // Permanent delete (if soft-delete enabled)
}
```

**REST:**
```http
DELETE /api/v1/secrets/{path}
Authorization: Bearer <token>

Query Parameters:
  permanent: boolean  # Permanent delete

Response:
{
  "deleted": true,
  "recoverable": true  # If soft-delete enabled
}
```

---

## Lease Manager API

The lease manager tracks and renews dynamic secret leases.

### Get Lease

Get lease information.

**gRPC:**
```protobuf
rpc GetLease(GetLeaseRequest) returns (LeaseInfo);

message GetLeaseRequest {
  string lease_id = 1;
}

message LeaseInfo {
  string lease_id = 1;
  string path = 2;
  string backend = 3;
  int64 ttl = 4;
  int64 max_ttl = 5;
  bool renewable = 6;
  string state = 7;     // active, expired, revoked
  string created_at = 8;
  string expires_at = 9;
  string agent_id = 10;
}
```

**REST:**
```http
GET /api/v1/leases/{lease_id}
Authorization: Bearer <token>

Response:
{
  "lease_id": "database/creds/app/abc123",
  "path": "database/creds/app",
  "backend": "vault",
  "ttl": 3600,
  "max_ttl": 86400,
  "renewable": true,
  "state": "active",
  "created_at": "2024-01-15T10:00:00Z",
  "expires_at": "2024-01-15T11:00:00Z"
}
```

### List Leases

List leases with filtering.

**gRPC:**
```protobuf
rpc ListLeases(ListLeasesRequest) returns (ListLeasesResponse);

message ListLeasesRequest {
  string path_prefix = 1;  // Filter by path
  string backend = 2;      // Filter by backend
  string agent_id = 3;     // Filter by agent
  string state = 4;        // Filter by state
  int32 limit = 5;
  string cursor = 6;
}

message ListLeasesResponse {
  repeated LeaseInfo leases = 1;
  string next_cursor = 2;
  int64 total = 3;
}
```

**REST:**
```http
GET /api/v1/leases
Authorization: Bearer <token>

Query Parameters:
  path: string      # Path prefix filter
  backend: string   # Backend filter
  agent: string     # Agent filter
  state: string     # State filter (active, expired, revoked)
  limit: int        # Page size
  cursor: string    # Pagination cursor

Response:
{
  "leases": [...],
  "next_cursor": "abc123",
  "total": 150
}
```

### Renew Lease

Renew a lease to extend its TTL.

**gRPC:**
```protobuf
rpc RenewLease(RenewLeaseRequest) returns (RenewLeaseResponse);

message RenewLeaseRequest {
  string lease_id = 1;
  int64 increment = 2;  // Requested TTL extension
}

message RenewLeaseResponse {
  string lease_id = 1;
  int64 ttl = 2;        // New TTL
  string expires_at = 3;
}
```

**REST:**
```http
POST /api/v1/leases/{lease_id}/renew
Authorization: Bearer <token>
Content-Type: application/json

{
  "increment": 3600
}

Response:
{
  "lease_id": "database/creds/app/abc123",
  "ttl": 3600,
  "expires_at": "2024-01-15T12:00:00Z"
}
```

### Revoke Lease

Revoke a lease and invalidate credentials.

**gRPC:**
```protobuf
rpc RevokeLease(RevokeLeaseRequest) returns (RevokeLeaseResponse);

message RevokeLeaseRequest {
  string lease_id = 1;
  bool force = 2;  // Force revocation
}
```

**REST:**
```http
POST /api/v1/leases/{lease_id}/revoke
Authorization: Bearer <token>

Response:
{
  "revoked": true
}
```

### Bulk Revoke

Revoke multiple leases by criteria.

**gRPC:**
```protobuf
rpc BulkRevoke(BulkRevokeRequest) returns (BulkRevokeResponse);

message BulkRevokeRequest {
  string path_prefix = 1;
  string backend = 2;
  string agent_id = 3;
}

message BulkRevokeResponse {
  int64 revoked_count = 1;
  repeated string failed = 2;
}
```

**REST:**
```http
POST /api/v1/leases/bulk-revoke
Authorization: Bearer <token>
Content-Type: application/json

{
  "path_prefix": "database/",
  "agent_id": "agent-123"
}

Response:
{
  "revoked_count": 15,
  "failed": []
}
```

---

## Rotation Orchestrator API

The rotation orchestrator manages secret rotation workflows.

### Start Rotation

Initiate a secret rotation.

**gRPC:**
```protobuf
rpc StartRotation(StartRotationRequest) returns (RotationStatus);

message StartRotationRequest {
  string path = 1;
  RotationStrategy strategy = 2;
  RotationConfig config = 3;
}

message RotationStrategy {
  string type = 1;  // blue_green, rolling, canary
  BlueGreenConfig blue_green = 2;
  RollingConfig rolling = 3;
  CanaryConfig canary = 4;
}

message RollingConfig {
  int32 batch_size = 1;
  int64 batch_delay_ms = 2;
  int32 max_failures = 3;
}

message CanaryConfig {
  int32 percentage = 1;
  int64 observation_window_ms = 2;
  double success_threshold = 3;
}
```

**REST:**
```http
POST /api/v1/rotations
Authorization: Bearer <token>
Content-Type: application/json

{
  "path": "database/creds/app",
  "strategy": {
    "type": "rolling",
    "rolling": {
      "batch_size": 10,
      "batch_delay": "30s",
      "max_failures": 2
    }
  },
  "verification": {
    "type": "health_check",
    "endpoint": "/health",
    "timeout": "30s"
  }
}

Response:
{
  "rotation_id": "rotation-abc123",
  "status": "in_progress",
  "started_at": "2024-01-15T10:00:00Z"
}
```

### Get Rotation Status

Get current rotation status.

**gRPC:**
```protobuf
rpc GetRotationStatus(GetRotationStatusRequest) returns (RotationStatus);

message RotationStatus {
  string rotation_id = 1;
  string path = 2;
  string state = 3;  // pending, in_progress, verifying, completed, failed, rolled_back
  string strategy = 4;
  Progress progress = 5;
  string error = 6;
  string started_at = 7;
  string completed_at = 8;
}

message Progress {
  int32 total_agents = 1;
  int32 completed_agents = 2;
  int32 failed_agents = 3;
  string current_phase = 4;
}
```

**REST:**
```http
GET /api/v1/rotations/{rotation_id}
Authorization: Bearer <token>

Response:
{
  "rotation_id": "rotation-abc123",
  "path": "database/creds/app",
  "state": "in_progress",
  "strategy": "rolling",
  "progress": {
    "total_agents": 100,
    "completed_agents": 45,
    "failed_agents": 0,
    "current_phase": "deploying"
  },
  "started_at": "2024-01-15T10:00:00Z"
}
```

### Cancel Rotation

Cancel an in-progress rotation.

**gRPC:**
```protobuf
rpc CancelRotation(CancelRotationRequest) returns (CancelRotationResponse);

message CancelRotationRequest {
  string rotation_id = 1;
  bool rollback = 2;  // Rollback to previous credentials
}
```

**REST:**
```http
POST /api/v1/rotations/{rotation_id}/cancel
Authorization: Bearer <token>
Content-Type: application/json

{
  "rollback": true
}

Response:
{
  "cancelled": true,
  "rollback_initiated": true
}
```

### Rollback Rotation

Rollback a completed or failed rotation.

**REST:**
```http
POST /api/v1/rotations/{rotation_id}/rollback
Authorization: Bearer <token>

Response:
{
  "rollback_id": "rollback-xyz789",
  "status": "in_progress"
}
```

---

## Transit Engine API

The transit engine provides encryption-as-a-service.

### Encrypt

Encrypt plaintext data.

**gRPC:**
```protobuf
rpc Encrypt(EncryptRequest) returns (EncryptResponse);

message EncryptRequest {
  string key_name = 1;
  bytes plaintext = 2;
  bytes context = 3;     // For convergent encryption
  int32 key_version = 4; // 0 = latest
}

message EncryptResponse {
  bytes ciphertext = 1;
  int32 key_version = 2;
}
```

**REST:**
```http
POST /api/v1/transit/encrypt/{key_name}
Authorization: Bearer <token>
Content-Type: application/json

{
  "plaintext": "base64-encoded-data",
  "context": "optional-context"  // For convergent encryption
}

Response:
{
  "ciphertext": "vault:v1:abc123...",
  "key_version": 3
}
```

**Go Client:**
```go
import "github.com/shawnbutts/keystone-core/pkg/secrets/transit"

engine := transit.NewEngine(kmsProvider)

// Simple encryption
ciphertext, err := engine.Encrypt(ctx, "my-key", plaintext)

// With context (convergent encryption)
ciphertext, err := engine.EncryptWithContext(ctx, "my-key", plaintext, context)

// Specify key version
ciphertext, err := engine.EncryptWithVersion(ctx, "my-key", plaintext, 2)
```

### Decrypt

Decrypt ciphertext data.

**gRPC:**
```protobuf
rpc Decrypt(DecryptRequest) returns (DecryptResponse);

message DecryptRequest {
  string key_name = 1;
  bytes ciphertext = 2;
  bytes context = 3;  // Must match encryption context
}

message DecryptResponse {
  bytes plaintext = 1;
}
```

**REST:**
```http
POST /api/v1/transit/decrypt/{key_name}
Authorization: Bearer <token>
Content-Type: application/json

{
  "ciphertext": "vault:v1:abc123..."
}

Response:
{
  "plaintext": "base64-encoded-data"
}
```

### Batch Encrypt/Decrypt

Process multiple items efficiently.

**gRPC:**
```protobuf
rpc BatchEncrypt(BatchEncryptRequest) returns (BatchEncryptResponse);

message BatchEncryptRequest {
  string key_name = 1;
  repeated BatchItem items = 2;
}

message BatchItem {
  bytes plaintext = 1;
  bytes context = 2;
}

message BatchEncryptResponse {
  repeated BatchResult results = 1;
}

message BatchResult {
  bytes ciphertext = 1;
  string error = 2;
}
```

**REST:**
```http
POST /api/v1/transit/batch-encrypt/{key_name}
Authorization: Bearer <token>
Content-Type: application/json

{
  "items": [
    {"plaintext": "data1"},
    {"plaintext": "data2"},
    {"plaintext": "data3"}
  ]
}

Response:
{
  "results": [
    {"ciphertext": "vault:v1:aaa..."},
    {"ciphertext": "vault:v1:bbb..."},
    {"ciphertext": "vault:v1:ccc..."}
  ]
}
```

### Generate Data Key

Generate a data encryption key (envelope encryption).

**gRPC:**
```protobuf
rpc GenerateDataKey(GenerateDataKeyRequest) returns (DataKey);

message GenerateDataKeyRequest {
  string key_name = 1;
  int32 bits = 2;    // 128, 256, 512
  bytes context = 3;
}

message DataKey {
  bytes plaintext = 1;    // Use for encryption, then discard
  bytes ciphertext = 2;   // Store alongside encrypted data
}
```

**REST:**
```http
POST /api/v1/transit/datakey/{key_name}
Authorization: Bearer <token>
Content-Type: application/json

{
  "bits": 256
}

Response:
{
  "plaintext": "base64-plaintext-key",
  "ciphertext": "vault:v1:wrapped-key..."
}
```

### Sign

Create a digital signature.

**gRPC:**
```protobuf
rpc Sign(SignRequest) returns (SignResponse);

message SignRequest {
  string key_name = 1;
  bytes input = 2;
  string hash_algorithm = 3;  // sha256, sha384, sha512
  bool prehashed = 4;         // Input is already hashed
}

message SignResponse {
  bytes signature = 1;
  int32 key_version = 2;
}
```

**REST:**
```http
POST /api/v1/transit/sign/{key_name}
Authorization: Bearer <token>
Content-Type: application/json

{
  "input": "base64-data",
  "hash_algorithm": "sha256"
}

Response:
{
  "signature": "vault:v1:signature...",
  "key_version": 1
}
```

### Verify

Verify a digital signature.

**REST:**
```http
POST /api/v1/transit/verify/{key_name}
Authorization: Bearer <token>
Content-Type: application/json

{
  "input": "base64-data",
  "signature": "vault:v1:signature...",
  "hash_algorithm": "sha256"
}

Response:
{
  "valid": true
}
```

### HMAC

Generate HMAC.

**REST:**
```http
POST /api/v1/transit/hmac/{key_name}
Authorization: Bearer <token>
Content-Type: application/json

{
  "input": "base64-data",
  "algorithm": "sha256"  // sha256, sha384, sha512
}

Response:
{
  "hmac": "vault:v1:hmac-value..."
}
```

---

## Compliance API

The compliance API provides audit and reporting functionality.

### Generate Compliance Report

Generate a compliance report for a framework.

**gRPC:**
```protobuf
rpc GenerateComplianceReport(ComplianceReportRequest) returns (ComplianceReport);

message ComplianceReportRequest {
  string framework = 1;  // soc2, pci_dss, hipaa, gdpr, fedramp, nist
  string start_time = 2;
  string end_time = 3;
}

message ComplianceReport {
  string id = 1;
  string framework = 2;
  ComplianceSummary summary = 3;
  repeated ComplianceCheckResult results = 4;
  repeated KeyInventoryItem key_inventory = 5;
  AccessAuditSummary access_audit = 6;
  RotationSummary rotation_summary = 7;
}
```

**REST:**
```http
POST /api/v1/compliance/reports
Authorization: Bearer <token>
Content-Type: application/json

{
  "framework": "soc2",
  "period": {
    "start": "2024-01-01T00:00:00Z",
    "end": "2024-01-31T23:59:59Z"
  }
}

Response:
{
  "id": "compliance-abc123",
  "framework": "soc2",
  "summary": {
    "total_requirements": 6,
    "compliant": 5,
    "non_compliant": 1,
    "compliance_percentage": 83.3,
    "risk_level": "medium"
  },
  "results": [...],
  "key_inventory": [...],
  "access_audit": {...},
  "rotation_summary": {...}
}
```

### Get Audit Logs

Query audit logs.

**REST:**
```http
GET /api/v1/audit/logs
Authorization: Bearer <token>

Query Parameters:
  path: string        # Filter by secret path
  principal: string   # Filter by principal
  operation: string   # Filter by operation
  start: timestamp    # Start time
  end: timestamp      # End time
  limit: int          # Page size
  cursor: string      # Pagination

Response:
{
  "logs": [
    {
      "timestamp": "2024-01-15T10:30:00Z",
      "event_id": "evt_abc123",
      "operation": "read",
      "path": "database/creds/app",
      "principal": "agent/web-1",
      "source_ip": "10.0.1.50",
      "success": true,
      "duration_ms": 45
    }
  ],
  "next_cursor": "...",
  "total": 1250
}
```

---

## Health API

### Secrets Health

**REST:**
```http
GET /api/v1/health/secrets
Authorization: Bearer <token>

Response:
{
  "status": "healthy",
  "backends": {
    "vault": {"status": "healthy", "latency_ms": 12},
    "aws": {"status": "healthy", "latency_ms": 45}
  },
  "lease_manager": {"status": "healthy", "active_leases": 150},
  "cache": {"status": "healthy", "hit_rate": 0.85}
}
```

---

## Error Codes

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `NOT_FOUND` | 404 | Secret or lease not found |
| `PERMISSION_DENIED` | 403 | Insufficient permissions |
| `UNAUTHENTICATED` | 401 | Invalid or missing credentials |
| `INVALID_ARGUMENT` | 400 | Invalid request parameters |
| `ALREADY_EXISTS` | 409 | Resource already exists |
| `FAILED_PRECONDITION` | 412 | Operation prerequisites not met |
| `RESOURCE_EXHAUSTED` | 429 | Rate limit exceeded |
| `INTERNAL` | 500 | Internal server error |
| `UNAVAILABLE` | 503 | Backend unavailable |
| `DEADLINE_EXCEEDED` | 504 | Request timeout |

## Rate Limits

Default rate limits:

| Operation | Limit |
|-----------|-------|
| Get Secret | 1000/min |
| Write Secret | 100/min |
| List Secrets | 100/min |
| Lease Operations | 500/min |
| Transit Operations | 5000/min |
| Rotation Operations | 10/min |

## Next Steps

- [Concepts Guide](/docs/concepts/secrets-management/) - Understanding secrets management
- [Backend Setup](/docs/operations/secrets-backends/) - Configure backends
- [Security Guide](/docs/operations/secrets-security/) - Security best practices
