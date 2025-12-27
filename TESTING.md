# Keystone Core Testing Documentation

## Test Coverage Summary

### Overall Status: ✅ Core Components Tested

**Total Coverage:** 12.8% (across all packages)
**Core Package Coverage:**
- `pkg/state`: 71.4% ✅
- `pkg/agent`: 38.4% ✅

## Test Suites

### 1. Agent Package Tests (`pkg/agent/`)

**Files:**
- `metadata_test.go` - System metadata collection tests
- `executor_test.go` - Command execution engine tests

**Tests:**
- ✅ `TestCollectMetadata` - Validates system information gathering
- ✅ `TestCollectMetrics` - Validates metrics collection
- ✅ `TestGetIPAddresses` - Network interface discovery
- ✅ `TestExecutor_Execute_Success` - Basic command execution
- ✅ `TestExecutor_Execute_WithTimeout` - Timeout handling
- ✅ `TestExecutor_Execute_WithOutputHandler` - Streaming output
- ✅ `TestExecutor_Execute_InvalidCommand` - Error handling
- ✅ `TestExecutor_CancelCommand` - Command cancellation
- ✅ `TestExecutor_GetRunningCommands` - Process tracking
- ✅ `TestNewExecutor` - Constructor validation

**Coverage:** 38.4% of statements

**Key Areas Tested:**
- System metadata collection (OS, arch, hostname, IPs)
- Metrics gathering (CPU, memory, goroutines)
- Command execution with various scenarios
- Output streaming
- Timeout handling
- Error conditions

### 2. State Storage Tests (`pkg/state/`)

**Files:**
- `sqlite_test.go` - SQLite backend tests

**Tests:**
- ✅ `TestSQLiteStore_AgentOperations` - Complete agent CRUD
- ✅ `TestSQLiteStore_CommandOperations` - Command history tracking
- ✅ `TestSQLiteStore_ListAgents_WithFilters` - Query filtering
- ✅ `TestSQLiteStore_Ping` - Database connectivity

**Coverage:** 71.4% of statements

**Key Areas Tested:**
- Agent registration and persistence
- Agent status updates
- Metrics storage and retrieval
- Command recording
- Command status updates
- Result persistence
- Filtering and pagination
- Database health checks

**Well-Tested Functions (100% coverage):**
- `initSchema` - Database schema creation
- `SaveAgent` - Agent persistence
- `UpdateAgentStatus` - Status updates
- `UpdateAgentMetrics` - Metrics updates
- `DeleteAgent` - Agent removal
- `SaveCommand` - Command recording
- `UpdateCommandStatus` - Status updates
- `UpdateCommandResult` - Result storage
- `Ping` - Health check
- `Close` - Connection cleanup

## Running Tests

### Run All Tests
```bash
make test
```

### Run Specific Package Tests
```bash
go test -v ./pkg/agent/...
go test -v ./pkg/state/...
```

### Generate Coverage Report
```bash
go test -coverprofile=coverage.out ./pkg/...
go tool cover -html=coverage.out
```

### Run Tests with Race Detection
```bash
go test -race ./pkg/...
```

## Test Infrastructure

### Temporary Test Databases

All state storage tests use temporary SQLite databases that are automatically cleaned up:
```go
tmpFile := "/tmp/test-keystone-core-" + time.Now().Format("20060102150405") + ".db"
defer os.Remove(tmpFile)
```

### Cross-Platform Command Testing

Executor tests adapt commands for the platform:
```go
if runtime.GOOS == "windows" {
    command = "cmd"
    args = []string{"/c", "echo", "hello"}
} else {
    command = "echo"
    args = []string{"hello"}
}
```

## Untested Packages (Future Work)

The following packages have 0% coverage and need test implementation:

- `pkg/api/server` - gRPC API server
- `pkg/controlplane` - Connection manager and command dispatcher
- `pkg/nats` - NATS connection manager
- `pkg/security` - Certificate generation
- `pkg/config` - Configuration management
- `pkg/version` - Version information

## Integration Tests (Future)

Planned integration tests:
- [ ] Full agent registration flow
- [ ] End-to-end command execution
- [ ] Multi-agent scenarios
- [ ] Network failure recovery
- [ ] State persistence across restarts

## Performance Tests (Future)

Planned performance benchmarks:
- [ ] Command execution latency (<100ms to 1000 nodes)
- [ ] Message throughput (>100k msgs/sec)
- [ ] State operation performance
- [ ] Memory usage profiling
- [ ] Concurrent agent handling (1000+ agents)

## Chaos Tests (Future)

Planned chaos scenarios:
- [ ] NATS server failures
- [ ] Network partitions
- [ ] Agent crashes and recovery
- [ ] Control plane restarts
- [ ] Database corruption scenarios

## Test Quality Metrics

### Current Status
- ✅ All critical paths in state storage tested
- ✅ Command execution core functionality tested
- ✅ Metadata collection validated
- ✅ Cross-platform compatibility tested
- ✅ Error handling covered
- ⚠️  Integration tests needed
- ⚠️  API server tests needed
- ⚠️  NATS manager tests needed

### Goal for Phase 4 Completion
- Target: >80% coverage for core packages
- Current: 71.4% (state), 38.4% (agent)
- Status: Good foundation, more tests needed

## CI/CD Integration

Tests can be run in CI/CD pipelines:

```yaml
# Example GitHub Actions
- name: Run tests
  run: make test

- name: Upload coverage
  run: |
    go test -coverprofile=coverage.out ./pkg/...
    go tool cover -html=coverage.out -o coverage.html
```

## Notes

- Tests are designed to be fast and independent
- No external dependencies required for testing
- SQLite tests use temporary databases
- Command execution tests are cross-platform
- All tests pass on macOS, Linux, and Windows
