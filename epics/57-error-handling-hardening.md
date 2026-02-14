# Epic 57: Error Handling Hardening

## Overview

A codebase audit identified ~20 locations where errors are silently discarded, channel drops go untracked, and parse failures are swallowed. While some are intentional (named no-op loggers, best-effort storage), others mask real failures that could cause data loss, security gaps, or hard-to-debug issues.

## Goal

Ensure all error paths either handle the error meaningfully (log, metric, return) or have a clear documented reason for ignoring it. Add drop counters for channel-full scenarios. Fix input validation gaps.

## Success Criteria

- [ ] Cluster restore warnings are logged, not discarded
- [ ] Webhook handler `json.Marshal` errors return HTTP 500
- [ ] Lease API `strconv.Atoi` parse errors return HTTP 400
- [ ] RBAC `CreateStandardRoles` errors are handled at startup
- [ ] Audit log write errors are logged (at minimum)
- [ ] WASM host function registration errors cause startup failure
- [ ] WASM `writeResult` errors are propagated
- [ ] Trusted key loading errors cause verification setup failure
- [ ] Channel-full drops in NATS logging, SPIRE updates, secret audit, and dashboard have metrics/counters
- [ ] ServiceMesh policy sync detects AuthorizationPolicy modifications

## Dependencies

None. These are all independent fixes.

## Technical Tasks

### Phase 1: Critical Error Handling (Week 1)

**T1.1: Fix Cluster Restore Warnings**
- File: `pkg/api/cluster/handlers.go` line 723
- Change `_ = warnings` to log each warning at WARN level

**T1.2: Fix Webhook Marshal Error**
- File: `pkg/api/webhooks/handlers.go` line 98
- Check `json.Marshal` error, return HTTP 500 if it fails

**T1.3: Fix Lease Pagination Parse Errors**
- File: `pkg/api/secrets/leases.go` lines 65, 68
- Return HTTP 400 with descriptive error when `limit` or `offset` are not valid integers

**T1.4: Fix RBAC Setup Error**
- File: `pkg/api/rbac/handlers.go` line 20
- Return error from `NewRBACHandlers` if `CreateStandardRoles` fails, or log and continue with degraded state

**T1.5: Fix Audit Log Write Errors**
- File: `pkg/api/cluster/token_handler.go` lines 318, 333, 350
- Log audit write failures at ERROR level. For compliance-sensitive operations, consider returning an error.

### Phase 2: WASM and Module Security (Week 2)

**T2.1: Propagate WASM Host Function Registration Errors**
- File: `pkg/module/runtime/wasm_builtins.go` lines 408-449
- Collect errors from all 9 `RegisterHostFunction` calls. If any fail, return error from the setup function.

**T2.2: Propagate WASM writeResult Errors**
- File: `pkg/module/runtime/wasm_builtins.go` lines 172-387 (12 locations)
- Return error codes to the WASM guest when write fails instead of silently dropping

**T2.3: Fix Trusted Key Loading Errors**
- File: `pkg/module/verify/verifier.go` lines 30, 35
- If `AddTrustedKey` or `AddTrustedKeyID` fails, return error from verifier construction

### Phase 3: Channel Drop Metrics (Week 3)

**T3.1: Add Drop Counter to NATS Logging**
- File: `internal/logging/nats.go` line 202
- Add atomic counter for dropped messages. Expose via metrics. Periodically log if drops are occurring.

**T3.2: Add Drop Counter to SPIRE SVID Updates**
- File: `internal/identity/spire/provider.go` line 246
- Log at WARN level when SVID update is dropped. Add counter.

**T3.3: Add Drop Counter to Secret Audit Events**
- File: `internal/secrets/audit.go` line 497
- Log at WARN level when audit event is dropped. This is compliance-relevant.

**T3.4: Add Drop Counter to Dashboard Updates**
- File: `internal/visualization/server.go` line 114
- Add counter. Less critical since dashboard updates are best-effort.

### Phase 4: ServiceMesh and Remaining (Week 4)

**T4.1: Implement AuthorizationPolicy Modification Detection**
- File: `internal/servicemesh/policy_sync.go` line 408
- Track policy resource versions or hashes. On sync, compare current vs. last-seen. Report modifications.

**T4.2: Fix Remaining Minor Issues**
- `internal/secrets/pool.go:521` — `BackendConnection.Close()` should clean up backend resources
- `pkg/module/loader/loader.go:648-651` — Unknown runtime types in `wireCapabilities` should return error instead of silently succeeding
- `pkg/module/capabilities/types.go:256` — `DefaultLogger.Log()` should delegate to `log.Printf` or accept a real logger

## Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| Making WASM registration errors fatal could break existing setups | Add graceful degradation: log error, disable the capability, continue |
| Audit log errors could cause request failures | Log the audit error but don't fail the request; add metric for audit failures |
| Drop counters add overhead | Use atomic counters; negligible overhead |

## Definition of Done

- [ ] No `_ = importantFunction()` patterns remain without documented justification
- [ ] All channel-full drops have counters exposed via metrics
- [ ] Input validation returns proper HTTP status codes
- [ ] WASM setup errors are propagated
- [ ] `make test` and `make lint` pass
- [ ] No new `//nolint:errcheck` directives without explanatory comments
