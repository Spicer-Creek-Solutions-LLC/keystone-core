# Epic 43: Secrets API Implementation

## Overview

**Goal**: Implement the REST and gRPC API layer for secrets management, bridging the existing `internal/secrets/` implementation to the documented API surface in `docs/content/en/docs/reference/secrets-api.md`.

**Current State**: Keystone Core has a complete secrets infrastructure in `internal/secrets/` (broker, lease manager, rotation engine, transit encryption, 5 backend providers, audit, metrics, health) but no API layer exposes it. The `secrets-api.md` reference documents 24+ REST endpoints and gRPC services that do not exist. The `kscore-secrets` CLI generates mock data rather than calling real APIs.

**Target State**: A fully wired secrets API with:
- REST handlers for secrets CRUD, lease management, rotation, transit encryption, compliance, and health
- gRPC service definition and server implementation
- Public `pkg/secrets` client package for programmatic access
- CLI wired to real API instead of mock data
- Server integration in `kscore-server`

## Status

**NOT STARTED**

## Success Criteria

1. All documented REST endpoints in `secrets-api.md` are implemented and return real data from `internal/secrets/`
2. gRPC service proto defined, generated, and registered in `kscore-server`
3. Public `pkg/secrets` client package available for Go consumers
4. `kscore-secrets` CLI calls real API instead of generating mock data
5. >70% test coverage on all new packages
6. API reference docs updated to reflect actual implementation

## Dependencies

- **Epic 36** (Deep Secrets Management) - COMPLETE - provides `internal/secrets/` implementation
- **Epic 1** (Core Infrastructure) - COMPLETE - provides `kscore-server` and API framework

## Implementation Plan

### Phase 1: REST Handlers + Server Wiring (Week 1-2)

Create REST API handlers following the pattern established by `pkg/api/agents/handlers.go`.

**Files to create:**
- `pkg/api/secrets/handlers.go` - Handler struct with dependency injection, all endpoint handlers
- `pkg/api/secrets/handlers_test.go` - Unit tests for all endpoints
- `pkg/api/secrets/types.go` - Request/response types matching documented JSON schema

**Files to modify:**
- `cmd/kscore-server/main.go` - Register secrets handlers on HTTP mux

**Endpoints (Secrets Broker):**
- `GET /api/v1/secrets/{path}` - Read a secret (with version, refresh params)
- `GET /api/v1/secrets/{path}?list=true` - List secrets under a path
- `POST /api/v1/secrets/{path}` - Write a static secret
- `DELETE /api/v1/secrets/{path}` - Delete a secret (soft-delete support)

**Endpoints (Lease Manager):**
- `GET /api/v1/leases/{lease_id}` - Get lease details
- `GET /api/v1/leases` - List leases (filters: path, backend, agent, state; pagination)
- `POST /api/v1/leases/{lease_id}/renew` - Renew a lease with TTL increment
- `POST /api/v1/leases/{lease_id}/revoke` - Revoke a lease
- `POST /api/v1/leases/bulk-revoke` - Bulk revoke by path/backend/agent

**Endpoints (Rotation):**
- `POST /api/v1/rotations` - Start a rotation (blue-green, rolling, canary strategies)
- `GET /api/v1/rotations/{rotation_id}` - Get rotation status
- `POST /api/v1/rotations/{rotation_id}/cancel` - Cancel with optional rollback
- `POST /api/v1/rotations/{rotation_id}/rollback` - Rollback a rotation

**Endpoints (Transit Engine):**
- `POST /api/v1/transit/encrypt/{key_name}` - Encrypt data
- `POST /api/v1/transit/decrypt/{key_name}` - Decrypt data
- `POST /api/v1/transit/batch-encrypt/{key_name}` - Batch encrypt
- `POST /api/v1/transit/batch-decrypt/{key_name}` - Batch decrypt
- `POST /api/v1/transit/datakey/{key_name}` - Generate data key
- `POST /api/v1/transit/sign/{key_name}` - Sign data
- `POST /api/v1/transit/verify/{key_name}` - Verify signature
- `POST /api/v1/transit/hmac/{key_name}` - HMAC operation

**Endpoints (Compliance & Health):**
- `POST /api/v1/compliance/reports` - Generate compliance report (SOC2, PCI-DSS, HIPAA)
- `GET /api/v1/audit/logs` - Query audit logs with filters and pagination
- `GET /api/v1/health/secrets` - Secrets subsystem health with backend status and cache metrics

### Phase 2: gRPC Service Definition (Week 3)

**Files to create:**
- `api/proto/secrets.proto` - Service and message definitions
- `pkg/api/v1/secrets_grpc.pb.go` - Generated gRPC stubs
- `pkg/api/v1/secrets.pb.go` - Generated protobuf types
- `pkg/api/v1/secrets_service.go` - gRPC service implementation

**Files to modify:**
- `cmd/kscore-server/main.go` - Register SecretsService on gRPC server

**gRPC Service:**
```protobuf
service SecretsService {
  rpc GetSecret(GetSecretRequest) returns (GetSecretResponse);
  rpc ListSecrets(ListSecretsRequest) returns (ListSecretsResponse);
  rpc WriteSecret(WriteSecretRequest) returns (WriteSecretResponse);
  rpc DeleteSecret(DeleteSecretRequest) returns (DeleteSecretResponse);
  rpc GetLease(GetLeaseRequest) returns (GetLeaseResponse);
  rpc ListLeases(ListLeasesRequest) returns (ListLeasesResponse);
  rpc RenewLease(RenewLeaseRequest) returns (RenewLeaseResponse);
  rpc RevokeLease(RevokeLeaseRequest) returns (RevokeLeaseResponse);
  rpc Encrypt(TransitEncryptRequest) returns (TransitEncryptResponse);
  rpc Decrypt(TransitDecryptRequest) returns (TransitDecryptResponse);
  rpc Sign(TransitSignRequest) returns (TransitSignResponse);
  rpc Verify(TransitVerifyRequest) returns (TransitVerifyResponse);
}
```

### Phase 3: Public Client Package (Week 4)

**Files to create:**
- `pkg/secrets/client.go` - Client struct wrapping gRPC/REST with connection management
- `pkg/secrets/types.go` - Public types mirroring `internal/secrets/types.go`
- `pkg/secrets/transit/client.go` - Transit engine client
- `pkg/secrets/client_test.go` - Client tests

### Phase 4: CLI Wiring (Week 5)

**Files to modify:**
- `cmd/kscore-secrets/main.go` - Replace mock data generation with real `pkg/secrets.Client` calls

**Changes per command:**
- `get` - Call `client.GetSecret()` instead of generating sample output
- `list` - Call `client.ListSecrets()` instead of sample listing
- `dynamic list/get/revoke` - Call broker dynamic secret methods
- `leases list/revoke/renew` - Call lease manager methods
- `encrypt/decrypt/rewrap` - Call transit engine methods
- `cache status/clear/list` - Call cache management methods
- `audit` - Call audit log query endpoint

### Phase 5: Documentation Update (Week 5-6)

**Files to modify:**
- `docs/content/en/docs/reference/secrets-api.md` - Align with actual implementation, reference real proto files, fix import paths
- `docs/content/en/docs/reference/cli.md` - Update secrets CLI reference
- `docs/content/en/docs/reference/cli-quick-reference.md` - Update secrets commands

## Testing Strategy

- **Unit tests**: Each REST handler tested for success, error, and edge cases (~75 tests)
- **gRPC tests**: Service implementation tests with mock broker/lease/transit dependencies
- **Integration tests**: End-to-end tests with in-memory secrets broker
- **Coverage target**: >70% for `pkg/api/secrets/`, >70% for `pkg/secrets/`

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| `internal/secrets` API changes during implementation | Pin to current interface; adapter pattern if needed |
| Server dependency graph complexity (broker needs backends, cache, audit) | Use interface-based DI; provide factory function with sensible defaults |
| Proto generation toolchain not set up | Follow existing `api/proto/` patterns; Makefile targets exist |

## Definition of Done

- [ ] All 24+ REST endpoints implemented and tested
- [ ] gRPC proto defined, generated, and service registered
- [ ] Public `pkg/secrets` client package with tests
- [ ] CLI wired to real API
- [ ] >70% test coverage across all new packages
- [ ] API reference docs updated
- [ ] All tests pass with race detector
- [ ] TODO.md item marked complete
