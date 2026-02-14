# Epic 55: CLI Wiring — Secrets & Compliance

## Overview

Replace hardcoded sample data in the secrets, audit, and policy CLIs with real API calls. `kscore-secrets` has ~30 stub subcommands across 9 command groups that use `generateSample*()` functions. `kscore-audit` and `kscore-policy` have 5 additional stub commands.

## Goal

All secrets management commands (backends, audit, rotation, scheduling, policy, cache, templates, key rotation) make real gRPC or REST API calls. Policy and audit commands query real stores instead of creating empty in-memory instances.

## Success Criteria

- [ ] `kscore-secrets`: all ~30 stub commands wired to real SecretsService gRPC or REST API
- [ ] `kscore-audit search`: wired to real audit store query API
- [ ] `kscore-policy audit/report/compliance/violations`: wired to real policy engine via gRPC
- [ ] No `generateSample*` functions remain in these 3 binaries
- [ ] No "stub -- no gRPC RPC" comments remain

## Dependencies

- **Epic 53** (gRPC Service Completion): SecretsServer must be wired with real deps
- **Epic 43** (Secrets API): Secrets gRPC service exists but only covers basic CRUD + leases + transit
- **Epic 46** (gRPC Services): PolicyService already implemented
- **Epic 6** (Policy Enforcement): Policy auditor and compliance reporter exist

## Technical Tasks

### Phase 1: Secrets Backend and Audit Commands (Week 1)

**T1.1: Add Secrets Backend Management RPCs**
- The SecretsService proto has no RPCs for backend management
- Option A: Add `ListBackends`, `GetBackend`, `EnableBackend`, `DisableBackend` RPCs to secrets.proto
- Option B: Add REST endpoints for backend management
- Implement server-side handler backed by `SecretBroker.ListBackends()`

**T1.2: Wire `secrets backends` command**
- Replace `generateSampleBackends()` with real API call

**T1.3: Add Secrets Audit RPCs**
- Add `ListSecretAuditEntries` RPC or REST endpoint
- Backed by `SecretAuditLogger` store

**T1.4: Wire `secrets audit` command**
- Replace `generateSampleAuditEntries()` with real API call

### Phase 2: Secrets Rotation and Scheduling (Week 2-3)

**T2.1: Add Rotation Management RPCs**
- `ListRotations`, `GetRotation`, `StartRotation`, `GetRotationStatus`, `GetRotationHistory`
- `TriggerRotation`, `RollbackRotation`, `PauseRotation`, `ResumeRotation`, `CancelRotation`
- Backed by `internal/credentials/rotation/` engine

**T2.2: Wire all 10 `secrets rotate *` subcommands**
- Replace `generateSampleRotations()`, `generateSampleHistory()`, and print-only commands

**T2.3: Add Rotation Schedule RPCs**
- `ListRotationSchedules`, `GetRotationSchedule`, `CreateRotationSchedule`
- `EnableSchedule`, `DisableSchedule`, `DeleteSchedule`
- Backed by `rotation.PolicyEngine` scheduler

**T2.4: Wire all 6 `secrets schedule *` subcommands**
- Replace `generateSampleSchedules()` and print-only commands

### Phase 3: Secrets Policy, Cache, and Remaining (Week 4)

**T3.1: Add Secrets Policy RPCs**
- `ListSecretPolicies`, `GetSecretPolicy`, `CreateSecretPolicy`, `DeleteSecretPolicy`
- Backed by policy store (may share with `internal/policy/` or be secrets-specific)

**T3.2: Wire `secrets policy` commands**
- Replace `generateSamplePolicies()` and print-only commands

**T3.3: Add Cache Management RPCs**
- `GetCacheStatus`, `ClearCache`, `ListCacheEntries`
- Backed by the real `EncryptedCache` (after Epic 52 fixes it)

**T3.4: Wire `secrets cache`, `secrets rewrap`, `secrets template`, `secrets rotate-keys`**
- `cache`: Replace hardcoded stats with real cache API
- `rewrap`: Use `Transit.Rewrap()` API instead of faking version bump
- `template`: Parse template file and resolve secret references via `SecretsService.GetSecret`
- `rotate-keys`: Use `Transit.RotateKey()` API instead of printing fake progress

### Phase 4: Audit and Policy CLI (Week 5)

**T4.1: Wire `audit search`**
- File: `cmd/kscore-audit/main.go`
- Replace `generateSampleSearchResults()` with query to audit store via REST or gRPC
- The audit query/report/compliance commands already use real (but empty) stores — wire them to the server's store

**T4.2: Wire `policy audit/report/compliance/violations`**
- File: `cmd/kscore-policy/main.go`
- Replace fresh empty in-memory stores with `PolicyService` gRPC calls
- Use `PolicyService.GetAuditLog`, `PolicyService.GetComplianceReport`, `PolicyService.ListViolations`

## Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| Many new RPCs needed for rotation/schedule/policy management | Group related RPCs; reuse existing proto patterns |
| Secrets cache RPCs expose internal state | Gate behind admin role; redact sensitive cache entries |
| Template parsing is complex | Start with simple `{{ secret "path" }}` syntax; expand later |

## Definition of Done

- [ ] All `generateSample*` functions removed from the 3 binaries
- [ ] All new RPCs have server implementations with tests
- [ ] CLI commands handle missing server/disabled features gracefully
- [ ] `make test` and `make lint` pass
- [ ] CLI reference documentation updated
