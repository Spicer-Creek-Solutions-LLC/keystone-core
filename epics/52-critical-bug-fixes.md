# Epic 52: Critical Bug Fixes

## Overview

Address broken production code paths discovered during a full codebase audit. These are active bugs where stub/placeholder implementations are wired into production code paths, causing silent failures or incorrect behavior at runtime.

## Goal

Ensure all production code paths that are wired and reachable do what they claim to do. No silent no-ops, no placeholder fallbacks, no missing data in requests.

## Success Criteria

- [x] `EncryptedCache` in `internal/secrets/factory.go` has a real implementation (or is replaced by `cache.go` implementation)
- [x] `applyStateFile` in `internal/bootstrap/handoff.go` sends file data in the request body
- [x] Gateway NATS connections use TLS when `config.NATS.TLS.Enabled` is true (`internal/gateway/server.go`)
- [x] Zstd, LZ4, and Snappy compression use real algorithms instead of falling back to gzip (`internal/files/compression/compression.go`)
- [x] `FileRollback` and `PackageRollback` in `internal/statemgmt/transaction.go` perform actual rollback operations
- [x] `OPAHandler.Evaluate`, `CELHandler.Evaluate`, `BuiltinHandler.Evaluate` in `internal/policy/type_registry.go` either delegate to the engine or are unreachable
- [x] `GetServerStatus` RPC is implemented in `ControlPlaneServer` instead of falling through to embedded unimplemented stub

## Dependencies

None. These are all independent fixes in existing code.

## Technical Tasks

### Phase 1: Secrets Cache and Bootstrap (Week 1) - COMPLETE

**T1.1: Fix EncryptedCache Stub**
- File: `internal/secrets/factory.go`
- Deleted the entire no-op `EncryptedCache` stub type (was lines 540-587)
- Added `encryptionKey` field and `WithEncryptionKey()` setter to `BrokerBuilder`
- Updated `Build()` to generate random 32-byte AES key and call `NewEncryptedSecretCache(config.Cache, key)`
- Tests verify real `*EncryptedSecretCache` is wired when caching enabled

**T1.2: Fix Bootstrap Handoff Empty Body**
- File: `internal/bootstrap/handoff.go`
- Replaced `http.NoBody` with `bytes.NewReader(data)`, removed `_ = data`

### Phase 2: Gateway TLS and Compression (Week 2) - COMPLETE

**T2.1: Wire Gateway NATS TLS**
- File: `internal/gateway/server.go`
- Added TLS config block using `nats.Secure(tlsConfig)` with cert/key pair, CA cert, min version, insecure mode

**T2.2: Implement Real Compression Algorithms**
- File: `internal/files/compression/compression.go`
- Zstd: `github.com/klauspost/compress/zstd` (new dependency)
- LZ4: `github.com/pierrec/lz4/v4` (promoted from indirect)
- Snappy: `github.com/golang/snappy` (already available)
- Round-trip tests verify all 3 algorithms compress and decompress correctly

### Phase 3: Transaction Rollback and Policy (Week 3) - COMPLETE

**T3.1: Implement FileRollback, PackageRollback, ServiceRollback**
- File: `internal/statemgmt/transaction.go`
- `FileRollback`: uses `os.WriteFile`/`os.Remove` with 0600 permissions
- `PackageRollback`/`ServiceRollback`: delegate to executor function fields on `RollbackBuilder`
- Returns clear error when executor is nil (not configured)

**T3.2: Fix Policy Type Handler Always-Allow**
- File: `internal/policy/type_registry.go`
- All 3 handlers (OPA, CEL, Builtin) now return error: "must go through Engine.Evaluate()"
- No production code calls these directly — engine uses its own evaluators

**T3.3: Implement GetServerStatus**
- File: `pkg/api/server/controlplane_server.go`
- Added `startTime` field, set in constructor
- Returns version, uptime, runtime stats (goroutines, memory), connected agent count, health status

### Phase 4: Tests and Verification - COMPLETE

- Cache wiring tests: `TestBrokerBuilder_CacheWiring`, `TestBrokerBuilder_CacheWiring_WithKey`, `TestBrokerBuilder_NoCacheWhenDisabled`
- Compression round-trip tests: `TestCompressor_Zstd_RoundTrip`, `TestCompressor_LZ4_RoundTrip`, `TestCompressor_Snappy_RoundTrip`
- Policy handler error tests added to `TestOPAHandler`, `TestCELHandler`, `TestBuiltinHandler`
- Rollback tests: `TestRollbackBuilder_FileRollback` (restore/delete/idempotent), `TestRollbackBuilder_PackageRollback` (nil executor/reinstall/remove), `TestRollbackBuilder_ServiceRollback` (nil executor/restore running/restore stopped)
- GetServerStatus test: `TestGetServerStatus` verifying version, uptime, goroutines, memory
- `make lint` passes with 0 issues
- `make test` passes with race detector

## Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| Compression library additions increase binary size | Use `klauspost/compress` which is widely used and well-maintained |
| PackageRollback needs module system access | Delegated to configurable executor function, returns error when not configured |
| Policy handler change could break evaluation flow | Traced all call sites — no production code calls handler.Evaluate() directly |

## Definition of Done

- [x] All 7 bugs fixed with unit tests
- [x] No `generateSample*` or placeholder logic in the affected files
- [x] `make test` passes with race detector
- [x] `make lint` passes with 0 issues
