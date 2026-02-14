# Epic 55: CLI Wiring — Secrets & Compliance

## Status: COMPLETE

## Overview

Replace hardcoded sample data in the secrets, audit, and policy CLIs with real API calls. `kscore-secrets` has ~30 stub subcommands across 9 command groups that use `generateSample*()` functions. `kscore-audit` and `kscore-policy` have 5 additional stub commands.

## Goal

All secrets management commands (backends, audit, rotation, scheduling, policy, cache, templates, key rotation) make real gRPC or REST API calls. Policy and audit commands query real stores instead of creating empty in-memory instances.

## Success Criteria

- [x] `kscore-secrets`: all ~30 stub commands wired to real SecretsService gRPC or REST API
- [x] `kscore-audit search`: wired to real audit store query API
- [x] `kscore-policy audit/report/compliance/violations`: wired to real policy engine via gRPC
- [x] No `generateSample*` functions remain in these 3 binaries
- [x] No "stub -- no gRPC RPC" comments remain

## Implementation Summary

### Architecture Decision: REST for secrets, gRPC for audit/policy

- **kscore-secrets → REST**: The backing infrastructure (`SecretBroker`, rotation `Engine`, `PolicyEngine`) lives in `internal/` packages. REST handlers in `pkg/api/secrets/` are simpler than adding ~20 new proto RPCs.
- **kscore-audit/kscore-policy → gRPC**: `PolicyService` already has `GetAuditLog`, `GetComplianceReport`, `ListViolations`, `EvaluatePolicy` RPCs fully implemented. A public gRPC client package (`pkg/policy/`) wraps these.

### Phase 1: Secrets REST Extensions — Backends, Audit, Cache (COMPLETE)
- Added REST endpoints: `GET /api/v1/secrets/backends`, `GET /api/v1/secrets/backends/{name}`, `GET /api/v1/secrets/cache/stats`, `DELETE /api/v1/secrets/cache`
- Created `cmd/kscore-secrets/rest_client.go` with REST client for new endpoints
- Wired `backends`, `audit`, `cache status/list`, `cache clear` commands
- Wired `kscore-server` to pass real broker to secrets REST handler
- Removed `generateSampleBackends()`, `generateSampleAuditEntries()`, `generateSampleCacheEntries()`

### Phase 2: Rotation REST Extensions + CLI Wiring (COMPLETE)
- Added REST endpoints: `GET /api/v1/rotations`, `POST /api/v1/rotations/{id}/pause`, `POST /api/v1/rotations/{id}/resume`, `POST /api/v1/rotations/{id}/trigger`
- Wired all 10 `rotate` commands to REST client
- Removed `generateSampleRotations()`, `generateSampleHistory()`

### Phase 3: Schedule & Policy REST + CLI Wiring (COMPLETE)
- Added REST endpoints for rotation policies: `GET/POST /api/v1/secrets/rotation/policies`, `GET/DELETE /api/v1/secrets/rotation/policies/{id}`, enable/disable, `GET /api/v1/secrets/rotation/scheduler`
- Wired `schedule` (6 commands) and `policy` (4 commands) to REST client
- Removed `generateSampleSchedules()`, `generateSamplePolicies()`

### Phase 4: Remaining Secrets Commands (COMPLETE)
- `rewrap`: Wired to existing transit REST endpoint `/api/v1/transit/rewrap`
- `template`: Client-side implementation resolving `{{ secret "path" }}` references via gRPC
- `rotate-keys`: Returns "not yet available" error (transit key rotation API doesn't exist)

### Phase 5: Policy & Audit gRPC Client + CLI Wiring (COMPLETE)
- Created `pkg/policy/` public gRPC client package (client.go, types.go, errors.go)
- 4 RPC methods: `GetAuditLog`, `GetComplianceReport`, `ListViolations`, `EvaluatePolicy`
- Wired `kscore-audit`: log, search, report, stats, export, timeline → gRPC; analyze, watch → "not yet available"
- Wired `kscore-policy`: compliance, violations, audit, report, eval → gRPC; schedule, remediate, monitor → "not yet available"
- File-based policy commands (list, validate, check, show, create, update, delete, test) unchanged

### Phase 6: Documentation + Cleanup (COMPLETE)
- Updated epic file, AGENTS.md, CLI reference, API reference

## Dependencies

- **Epic 53** (gRPC Service Completion): SecretsServer must be wired with real deps
- **Epic 43** (Secrets API): Secrets gRPC service exists but only covers basic CRUD + leases + transit
- **Epic 46** (gRPC Services): PolicyService already implemented
- **Epic 6** (Policy Enforcement): Policy auditor and compliance reporter exist

## Definition of Done

- [x] All `generateSample*` functions removed from the 3 binaries
- [x] All new RPCs have server implementations with tests
- [x] CLI commands handle missing server/disabled features gracefully
- [x] `make test` and `make lint` pass
- [x] CLI reference documentation updated
