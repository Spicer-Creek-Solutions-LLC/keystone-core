# Epic 54: CLI Wiring — Core Operations

## Overview

Replace hardcoded sample data in the events, scheduling, runbook, and execution CLIs with real API calls. These CLIs currently use `generateSample*()` functions that return fake data, giving users the impression of a working system while actually showing static fixtures.

## Goal

All list/show/query commands in `kscore-events`, `kscore-schedule`, `kscore-runbook`, and `kscore-exec` make real gRPC or REST API calls. Mutating commands (create, trigger, pause, etc.) perform real operations on the server.

## Success Criteria

- [ ] `kscore-events`: all 8 commands wired to real EventService gRPC or REST API
- [ ] `kscore-schedule`: all 11 commands wired to real scheduling API
- [ ] `kscore-runbook`: 5 remaining stub commands wired to real runbook API
- [ ] `kscore-exec`: batch `--follow` streaming implemented via gRPC server-side streaming
- [ ] No `generateSample*` functions remain in these 4 binaries
- [ ] All commands handle connection errors gracefully (clear error message, not a stack trace)

## Dependencies

- **Epic 46** (gRPC Services): EventService already implemented
- **Epic 37** (Runbooks): Runbook execution engine exists; need query API
- REST API handlers from **Epic 49** for scheduling and runbook queries

## Technical Tasks

### Phase 1: Events CLI (Week 1)

**T1.1: Wire `events list` and `events query`**
- Replace `generateSampleEvents()` with `EventService.ListEvents` gRPC call
- Map CLI flags (--type, --severity, --source, --since, --until) to proto filter fields
- Pagination via `--limit` and `--cursor` flags

**T1.2: Wire `events emit`**
- Replace print statement with `EventService.EmitEvent` gRPC call
- Map CLI flags (--type, --source, --severity, --data) to proto fields

**T1.3: Wire `events replay`, `events analyze`, `events prune`, `events archive`**
- `replay`: query events via `ListEvents` with time range, re-emit via `EmitEvent`
- `analyze`: query events and compute stats client-side (or use `GetEventStats` RPC)
- `prune`/`archive`: need server-side support — may require new RPCs or REST endpoints

**T1.4: Wire `events dlq list`, `events storage stats`**
- DLQ: needs REST or gRPC endpoint for dead-letter queue access
- Storage stats: needs server-side JetStream stats endpoint

### Phase 2: Schedule CLI (Week 2-3)

**T2.1: Define Schedule REST/gRPC API**
- The schedule subsystem exists (`internal/schedule/`) but has no REST or gRPC API
- Add REST handlers: `GET/POST /api/v1/schedules`, `GET/PUT/DELETE /api/v1/schedules/{id}`
- Add REST handlers: `POST /api/v1/schedules/{id}/trigger`, `POST /api/v1/schedules/{id}/pause`
- Add maintenance window endpoints

**T2.2: Wire `schedule list`, `show`, `create`, `update`, `delete`**
- Replace `generateSampleSchedules()` with REST API calls
- Map CLI flags to request parameters

**T2.3: Wire `schedule trigger`, `pause`, `resume`, `enable`, `disable`, `history`**
- Replace print-only commands with real REST API calls
- `history` queries execution history from the schedule store

**T2.4: Wire `maintenance list`, `create`, `delete`, `start`, `end`, `active`, `upcoming`**
- Replace `generateSampleWindows()` with REST API calls

### Phase 3: Runbook CLI (Week 4)

**T3.1: Add Runbook Query REST API**
- `GET /api/v1/runbooks` (list), `GET /api/v1/runbooks/{name}` (show)
- `GET /api/v1/runbooks/executions` (list), `GET /api/v1/runbooks/executions/{id}` (show)
- `GET /api/v1/runbooks/{name}/audit` (audit log)

**T3.2: Wire remaining stub commands**
- Replace `generateSampleRunbooks()`, `generateSampleExecutions()`, `generateSampleAuditEntries()` with real API calls
- `runbook run`, `approve`, `test` are already wired — leave as-is

### Phase 4: Exec Streaming (Week 5)

**T4.1: Implement batch `--follow` streaming**
- File: `cmd/kscore-exec/main.go` line 1288
- Use gRPC server-side streaming to follow batch execution output in real time
- Display results as they arrive, similar to `kubectl logs -f`

## Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| Schedule and runbook query APIs don't exist yet | Phase 2-3 includes creating these APIs |
| DLQ and storage stats may need JetStream admin access | Use NATS management API; document required permissions |
| Streaming output adds complexity | Follow existing `SubscribeEvents` streaming pattern |

## Definition of Done

- [ ] All `generateSample*` functions removed from the 4 binaries
- [ ] Each command has at least a basic integration test (mock server or test fixture)
- [ ] Connection error handling: commands print actionable error messages
- [ ] `make test` and `make lint` pass
- [ ] CLI reference documentation updated for any new flags/behavior changes
