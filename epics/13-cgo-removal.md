# Epic 13: CGO Removal - Pure Go Build

## Overview

Remove all CGO dependencies from Keystone Core to enable:
- Cross-compilation without toolchain complexity
- Simpler CI/CD pipelines (no gcc/clang required)
- Smaller, static binaries
- Alpine/scratch Docker images without libc
- ARM64 and other architecture support without cross-compilers

## Current CGO Dependencies

| Package | Usage | Replacement |
|---------|-------|-------------|
| `github.com/mattn/go-sqlite3` | State storage, event storage | `modernc.org/sqlite` |
| `github.com/bytecodealliance/wasmtime-go/v25` | WASM module runtime | `github.com/tetratelabs/wazero` |

## Success Criteria

1. `CGO_ENABLED=0 go build ./...` succeeds
2. All existing tests pass
3. Cross-compilation works: `GOOS=linux GOARCH=arm64 go build ./...`
4. No performance regression >20% for SQLite operations
5. WASM module execution maintains feature parity

## Phase 1: SQLite Migration (Week 1)

### T1.1: Replace SQLite Driver

**Files to modify:**
- `pkg/state/sqlite.go`
- `pkg/events/storage_sqlite.go`

**Changes:**
```go
// Before
import _ "github.com/mattn/go-sqlite3"
sql.Open("sqlite3", path)

// After
import _ "modernc.org/sqlite"
sql.Open("sqlite", path)  // Note: driver name is "sqlite" not "sqlite3"
```

**Considerations:**
- modernc.org/sqlite uses `sqlite` as driver name (not `sqlite3`)
- Connection string format is compatible
- All SQL syntax is compatible (it's the same SQLite, just transpiled to Go)

### T1.2: Update go.mod

```bash
go get modernc.org/sqlite
go mod tidy
```

### T1.3: Verify Tests Pass

```bash
CGO_ENABLED=0 go test ./pkg/state/... ./pkg/events/...
```

### T1.4: Benchmark Comparison

Run benchmarks before/after to measure any performance impact:
- State store operations (CRUD)
- Event storage operations
- Bulk insert performance

## Phase 2: WASM Runtime Migration (Week 2-3)

### T2.1: Implement wazero Runtime

Replace wasmtime with wazero in `pkg/module/runtime/wasm/`:

**Key API differences:**

| wasmtime | wazero |
|----------|--------|
| `wasmtime.NewEngine()` | `wazero.NewRuntime(ctx)` |
| `wasmtime.NewStore(engine)` | Runtime has embedded store |
| `wasmtime.NewModule(engine, wasm)` | `runtime.CompileModule(ctx, wasm)` |
| `wasmtime.NewLinker(engine)` | `runtime.NewHostModuleBuilder()` |
| `instance.GetExport()` | `module.ExportedFunction()` |
| Fuel metering | `wazero.RuntimeConfig().WithCompilationCache()` |

**wazero advantages:**
- Cleaner, more Go-idiomatic API
- Built-in WASI support (`wazero.NewModuleConfig().WithWASI()`)
- Context-based cancellation (native Go patterns)
- No external dependencies

### T2.2: Update Runtime Interface

Current interface in `pkg/module/runtime/types.go`:
```go
type WasmRuntime interface {
    Runtime
    ExecuteFunction(ctx context.Context, name string, args ...interface{}) (interface{}, error)
}
```

This interface should remain compatible - only implementation changes.

### T2.3: Implement Host Functions

Reimplement host function bindings for capabilities:
- `fs_read`, `fs_write`
- `http_get`, `http_post`
- `exec_run`
- `log_write`
- `kv_get`, `kv_set`

wazero host function pattern:
```go
builder := runtime.NewHostModuleBuilder("env")
builder.NewFunctionBuilder().
    WithFunc(func(ctx context.Context, ptr, len uint32) uint32 {
        // Implementation
    }).
    Export("fs_read")
```

### T2.4: Memory Management

wazero memory access:
```go
mem := mod.Memory()
data, ok := mem.Read(offset, size)
mem.Write(offset, data)
```

### T2.5: WASI Support

wazero has excellent WASI support:
```go
wasi_snapshot_preview1.MustInstantiate(ctx, runtime)
config := wazero.NewModuleConfig().
    WithStdout(os.Stdout).
    WithStderr(os.Stderr).
    WithFS(fs)
```

### T2.6: Update Tests

Update `pkg/module/runtime/wasm/runtime_test.go` to use wazero API.

## Phase 3: Validation & Cleanup (Week 4)

### T3.1: Full Test Suite

```bash
CGO_ENABLED=0 go test ./...
```

### T3.2: Cross-Compilation Verification

```bash
# Linux AMD64
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/kscore-server-linux-amd64 ./cmd/kscore-server

# Linux ARM64
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o bin/kscore-server-linux-arm64 ./cmd/kscore-server

# Windows AMD64
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o bin/kscore-server-windows-amd64.exe ./cmd/kscore-server

# macOS ARM64 (Apple Silicon)
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o bin/kscore-server-darwin-arm64 ./cmd/kscore-server
```

### T3.3: Remove Old Dependencies

```bash
go mod tidy
```

Verify `go.mod` no longer contains:
- `github.com/mattn/go-sqlite3`
- `github.com/bytecodealliance/wasmtime-go`

### T3.4: Update CI/CD

Update GitHub Actions / CI to use `CGO_ENABLED=0` and remove C compiler setup.

### T3.5: Update Documentation

- Update CLAUDE.md technology stack section
- Update installation docs (no C compiler requirement)
- Update build instructions

## Dependencies

- None (can be done independently)

## Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| SQLite performance regression | Low | Medium | Benchmark before/after; modernc performance is within 10-20% |
| WASM feature gap | Low | Medium | wazero supports WASI preview1; verify all needed features |
| API compatibility issues | Low | Low | Both replacements are well-documented with migration guides |

## Testing Strategy

1. **Unit Tests**: All existing tests must pass with CGO_ENABLED=0
2. **Integration Tests**: SQLite and WASM integration tests
3. **Benchmarks**: Compare performance before/after
4. **Cross-compile**: Verify builds for all target platforms

## Definition of Done

- [ ] All code compiles with `CGO_ENABLED=0`
- [ ] All tests pass
- [ ] Cross-compilation works for linux/amd64, linux/arm64, darwin/arm64, windows/amd64
- [ ] No CGO dependencies in `go.mod`
- [ ] Performance benchmarks documented
- [ ] CI/CD updated to use pure Go builds
- [ ] Documentation updated
