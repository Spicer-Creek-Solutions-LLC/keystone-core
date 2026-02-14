# Epic 52: Critical Bug Fixes

## Overview

Address broken production code paths discovered during a full codebase audit. These are active bugs where stub/placeholder implementations are wired into production code paths, causing silent failures or incorrect behavior at runtime.

## Goal

Ensure all production code paths that are wired and reachable do what they claim to do. No silent no-ops, no placeholder fallbacks, no missing data in requests.

## Success Criteria

- [ ] `EncryptedCache` in `internal/secrets/factory.go` has a real implementation (or is replaced by `cache.go` implementation)
- [ ] `applyStateFile` in `internal/bootstrap/handoff.go` sends file data in the request body
- [ ] Gateway NATS connections use TLS when `config.NATS.TLS.Enabled` is true (`internal/gateway/server.go`)
- [ ] Zstd, LZ4, and Snappy compression use real algorithms instead of falling back to gzip (`internal/files/compression/compression.go`)
- [ ] `FileRollback` and `PackageRollback` in `internal/statemgmt/transaction.go` perform actual rollback operations
- [ ] `OPAHandler.Evaluate`, `CELHandler.Evaluate`, `BuiltinHandler.Evaluate` in `internal/policy/type_registry.go` either delegate to the engine or are unreachable
- [ ] `GetServerStatus` RPC is implemented in `ControlPlaneServer` instead of falling through to embedded unimplemented stub

## Dependencies

None. These are all independent fixes in existing code.

## Technical Tasks

### Phase 1: Secrets Cache and Bootstrap (Week 1)

**T1.1: Fix EncryptedCache Stub**
- File: `internal/secrets/factory.go` lines 540-587
- Problem: `EncryptedCache` type has all no-op methods (`Get` returns `nil, false`; `Put`/`Delete`/`Clear` return nil). Wired into production via broker builder when caching is enabled.
- Fix: Either wire the real `cache.go` implementation into the factory, or remove the dead `EncryptedCache` type and use the real one directly.
- Verify: Unit test that cache hits/misses work when `CacheConfig.Enabled = true`.

**T1.2: Fix Bootstrap Handoff Empty Body**
- File: `internal/bootstrap/handoff.go` line 281
- Problem: `_ = data` discards the state file content, then sends `http.NoBody`. The server receives an empty POST.
- Fix: Pass `bytes.NewReader(data)` as the request body.
- Verify: Unit test that the request body contains the file content.

### Phase 2: Gateway TLS and Compression (Week 2)

**T2.1: Wire Gateway NATS TLS**
- File: `internal/gateway/server.go` line 168
- Problem: TODO comment — TLS config fields exist in the struct but are never wired into the NATS connection options.
- Fix: Add `nats.Secure(tlsConfig)` when `s.config.NATS.TLS.Enabled` is true, following the same pattern used elsewhere in the codebase (e.g., `internal/nats/options.go`).
- Verify: Unit test with TLS config set.

**T2.2: Implement Real Compression Algorithms**
- File: `internal/files/compression/compression.go` lines 391-422
- Problem: `compressZstd`, `decompressZstd`, `compressLZ4`, `decompressLZ4`, `compressSnappy`, `decompressSnappy` all silently fall back to gzip. Data labeled as zstd/lz4/snappy is actually gzip-compressed.
- Fix: Use `github.com/klauspost/compress/zstd`, `github.com/pierrec/lz4/v4`, and `github.com/golang/snappy` (or equivalent pure-Go libraries).
- Verify: Round-trip test for each algorithm. Cross-verify that output is decompressible by the canonical tool.

### Phase 3: Transaction Rollback and Policy (Week 3)

**T3.1: Implement FileRollback and PackageRollback**
- File: `internal/statemgmt/transaction.go` lines 596-618
- Problem: Both functions return nil without performing any operations. File content is not restored, packages are not reinstalled.
- Fix: `FileRollback` should use `os.WriteFile`/`os.Remove` to restore/delete files. `PackageRollback` should invoke the package module to reinstall the previous version or remove the newly installed package.
- Verify: Unit tests for restore-existing, delete-new-file, reinstall-package, remove-new-package scenarios.

**T3.2: Fix Policy Type Handler Always-Allow**
- File: `internal/policy/type_registry.go` lines 280, 317, 352
- Problem: `OPAHandler.Evaluate()`, `CELHandler.Evaluate()`, `BuiltinHandler.Evaluate()` always return `Allowed: true`.
- Fix: Either (a) make these handlers delegate to the real engine, (b) make them unreachable by removing them from the `TypeHandler` interface if the engine is the only evaluation path, or (c) add a clear `panic("must use engine.Evaluate")` to prevent accidental direct calls.
- Verify: Test that calling a handler directly does NOT return `Allowed: true` for a policy that should deny.

**T3.3: Implement GetServerStatus**
- File: `pkg/api/server/controlplane_server.go`
- Problem: The `GetServerStatus` RPC defined in `controlplane.proto` is never overridden — falls through to the embedded `UnimplementedControlPlaneServiceServer` and returns `codes.Unimplemented`.
- Fix: Implement `GetServerStatus` returning server version, uptime, connected agent count, and health status.
- Verify: Unit test covering the new method.

## Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| Compression library additions increase binary size | Use `klauspost/compress` which is widely used and well-maintained |
| PackageRollback needs module system access | Start with file rollback only; package rollback can delegate to the existing module executor |
| Policy handler change could break evaluation flow | Trace all call sites before modifying; add integration test |

## Definition of Done

- [ ] All 7 bugs fixed with unit tests
- [ ] No `generateSample*` or placeholder logic in the affected files
- [ ] `make test` passes with race detector
- [ ] `make lint` passes with 0 issues
