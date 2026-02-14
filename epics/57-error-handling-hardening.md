# Epic 57: Error Handling Hardening

## Status: COMPLETE ✅

## Overview

A codebase audit identified ~20 locations where errors are silently discarded, channel drops go untracked, and parse failures are swallowed. While some are intentional (named no-op loggers, best-effort storage), others mask real failures that could cause data loss, security gaps, or hard-to-debug issues.

## Goal

Ensure all error paths either handle the error meaningfully (log, metric, return) or have a clear documented reason for ignoring it. Add drop counters for channel-full scenarios. Fix input validation gaps.

## Success Criteria

- [x] Cluster restore warnings are logged, not discarded
- [x] Webhook handler `json.Marshal` errors return HTTP 500
- [x] Lease API `strconv.Atoi` parse errors return HTTP 400
- [x] RBAC `CreateStandardRoles` errors are handled at startup
- [x] Audit log write errors are logged (at minimum)
- [x] WASM host function registration errors cause startup failure
- [x] WASM `writeResult` errors are propagated
- [x] Trusted key loading errors cause verification setup failure
- [x] Channel-full drops in NATS logging, SPIRE updates, secret audit, and dashboard have metrics/counters
- [x] ServiceMesh policy sync detects AuthorizationPolicy modifications

## Dependencies

None. These are all independent fixes.

## Implementation Summary

### Phase 1: Critical Error Handling (API layer)

**T1.1: Fix Cluster Restore Warnings** ✅
- `pkg/api/cluster/handlers.go`: Changed `performRestore` to return `([]string, error)`, log warnings, include in response JSON

**T1.2: Fix Webhook Marshal Error** ✅
- `pkg/api/webhooks/handlers.go`: Return HTTP 500 on `json.Marshal` failure

**T1.3: Fix Lease Pagination Parse Errors** ✅
- `pkg/api/secrets/leases.go`: Return HTTP 400 on invalid `limit`/`offset` parameters

**T1.4: Fix RBAC Setup Error** ✅
- `pkg/api/rbac/handlers.go`: Changed `NewHandler()` to return `(*Handler, error)`
- `cmd/kscore-server/main.go`: Updated caller to check error and exit

**T1.5: Fix Audit Log Write Errors** ✅
- `pkg/api/cluster/token_handler.go`: Log audit write failures at ERROR level

### Phase 2: WASM and Module Security

**T2.1: Propagate WASM Host Function Registration Errors** ✅
- `pkg/module/runtime/wasm_builtins.go`: Collect errors via `errors.Join`, return from `RegisterWithWasmRuntime`

**T2.2: Propagate WASM writeResult Errors** ✅
- `pkg/module/runtime/wasm_builtins.go`: Return error code 1 when `writeResult` fails (12 locations)

**T2.3: Fix Trusted Key Loading Errors** ✅
- `pkg/module/verify/verifier.go`: Changed `NewModuleVerifier` to return `(*ModuleVerifier, error)`

### Phase 3: Channel Drop Metrics

**T3.1: Add Drop Counter to NATS Logging** ✅
- `internal/logging/nats.go`: Switched to `atomic.Int64` counters, rate-limited log on drop

**T3.2: Add Drop Counter to SPIRE Trust Bundle Updates** ✅
- `internal/identity/spire/provider.go`: Added `trustBundleDropped` atomic counter with WARN logging

**T3.3: Add Drop Counter to Secret Audit Events** ✅
- `internal/secrets/audit.go`: Return error on channel-full drop, added `EventsDropped()` method

**T3.4: Add Drop Counter to Dashboard Updates** ✅
- `internal/visualization/server.go`: Added `updatesDropped` atomic counter with `UpdatesDropped()` method

### Phase 4: ServiceMesh and Remaining

**T4.1: Implement AuthorizationPolicy Modification Detection** ✅
- `internal/servicemesh/policy_sync.go`: Added `authPoliciesEqual()`, `OldAuthPolicy` field, modification detection

**T4.2: Fix wireCapabilities Unknown Runtime** ✅
- `pkg/module/loader/loader.go`: Return error for unknown runtime type

**T4.3: Fix DefaultLogger** ✅
- `pkg/module/capabilities/types.go`: Delegate to `log.Printf`

**T4.4: BackendConnection.Close()** ✅
- `internal/secrets/pool.go`: Documented as intentional no-op (pool manages lifecycle)

## Definition of Done

- [x] No `_ = importantFunction()` patterns remain without documented justification
- [x] All channel-full drops have counters exposed via metrics
- [x] Input validation returns proper HTTP status codes
- [x] WASM setup errors are propagated
- [x] `make test` and `make lint` pass
- [x] No new `//nolint:errcheck` directives without explanatory comments
