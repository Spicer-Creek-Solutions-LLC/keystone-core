# Epic 40: Test Coverage Remediation

## Status: COMPLETE ✅

## Overview

Add comprehensive test coverage to 23 packages that currently have no tests but contain substantial business logic. These packages include CLI commands, API handlers, protocol implementations, vendor device adapters, and module runtime components.

**Goal**: Achieve >70% test coverage for all packages containing business logic, ensuring code correctness, preventing regressions, and improving maintainability.

## Success Criteria

- [x] All 23 identified packages have test files
- [x] CLI command packages achieve >40% coverage (per project standards)
- [x] API handler packages achieve >70% coverage
- [x] Protocol implementation packages achieve >70% coverage
- [x] Vendor adapter packages achieve >70% coverage
- [x] Module runtime packages achieve >70% coverage
- [x] No regressions in existing functionality
- [x] Tests follow project patterns (table-driven, t.TempDir(), etc.)

## Completion Summary

All five phases completed:
- **Phase 1**: Protocol implementation tests added
- **Phase 2**: Vendor adapter tests added (cisco, juniper, vyos, arista, opnsense, pfsense)
- **Phase 3**: API handler tests added
- **Phase 4**: Module runtime tests added (builtins, wasm_builtins)
- **Phase 5**: CLI command tests added for all 9 CLI packages

## Package Inventory

### High Priority: Protocol Implementations (5,087 lines)

| Package | Lines | Description |
|---------|-------|-------------|
| `pkg/protocols/rest` | 2,004 | REST client, authentication, response parsing |
| `pkg/protocols/snmp` | 1,702 | SNMPv2c and SNMPv3 implementations |
| `pkg/protocols/winrm` | 1,381 | WinRM shell and file operations |

### High Priority: Vendor Adapters (4,909 lines)

| Package | Lines | Description |
|---------|-------|-------------|
| `pkg/vendors/cisco` | 1,193 | Cisco IOS and NX-OS device adapters |
| `pkg/vendors/vyos` | 1,139 | VyOS device adapter |
| `pkg/vendors/juniper` | 571 | Juniper JunOS device adapter |
| `pkg/vendors/arista` | ~500 | Arista EOS device adapter |
| `pkg/vendors/opnsense` | ~400 | OPNsense firewall adapter |
| `pkg/vendors/pfsense` | ~400 | pfSense firewall adapter |

### Medium Priority: API Handlers (1,272 lines)

| Package | Lines | Description |
|---------|-------|-------------|
| `pkg/api/execution` | 457 | Execution API HTTP handlers |
| `pkg/api/agents` | 326 | Agent API HTTP handlers |
| `pkg/api/events` | 299 | Event API HTTP handlers |
| `pkg/api/webhooks` | 190 | Webhook HTTP handlers |

### Medium Priority: Module Runtime (1,037 lines)

| Package | Lines | Description |
|---------|-------|-------------|
| `pkg/module/runtime` | 1,037 | Starlark and WASM builtin functions |

### Lower Priority: CLI Commands (~6,000 lines)

| Package | Lines | Description |
|---------|-------|-------------|
| `cmd/kscore-backup` | ~900 | Backup management CLI |
| `cmd/kscore-agents` | ~800 | Agent management CLI |
| `cmd/kscore-events` | ~800 | Event management CLI |
| `cmd/kscore-proxy` | ~800 | Proxy agent CLI |
| `cmd/kscore-upgrade` | ~700 | Upgrade management CLI |
| `cmd/kscore-schedule` | ~600 | Schedule management CLI |
| `cmd/kscore-loadtest` | ~600 | Load testing CLI |
| `cmd/kscore-test` | ~400 | Test runner CLI |
| `cmd/kscore-monitor/client` | ~400 | Monitor client |

**Total: ~18,300 lines of untested code**

---

## Phase 1: Protocol Implementations (Weeks 1-3)

### US40.1: REST Protocol Tests
**As a** developer
**I want** comprehensive tests for the REST protocol adapter
**So that** REST-based device communication is reliable

**Acceptance Criteria**:
- Tests for HTTP client creation and configuration
- Tests for authentication methods (Basic, Bearer, API Key, OAuth)
- Tests for response parsing and error handling
- Tests for retry logic and timeout handling
- Mock server for integration tests

**Technical Tasks**:
1. Create `pkg/protocols/rest/adapter_test.go`
2. Create `pkg/protocols/rest/auth_test.go`
3. Create `pkg/protocols/rest/client_test.go`
4. Create `pkg/protocols/rest/response_test.go`
5. Add mock HTTP server helper

### US40.2: SNMP Protocol Tests
**As a** developer
**I want** comprehensive tests for SNMP protocol adapters
**So that** SNMP device polling is reliable

**Acceptance Criteria**:
- Tests for SNMPv2c operations (Get, GetNext, Walk, Set)
- Tests for SNMPv3 authentication and privacy
- Tests for OID parsing and response handling
- Tests for timeout and retry behavior

**Technical Tasks**:
1. Create `pkg/protocols/snmp/adapter_test.go`
2. Create `pkg/protocols/snmp/v2c_test.go`
3. Create `pkg/protocols/snmp/v3_test.go`
4. Add SNMP mock responder

### US40.3: WinRM Protocol Tests
**As a** developer
**I want** comprehensive tests for WinRM protocol adapter
**So that** Windows remote management is reliable

**Acceptance Criteria**:
- Tests for shell creation and command execution
- Tests for file upload and download
- Tests for authentication handling
- Tests for error conditions and timeouts

**Technical Tasks**:
1. Create `pkg/protocols/winrm/adapter_test.go`
2. Create `pkg/protocols/winrm/shell_test.go`
3. Create `pkg/protocols/winrm/file_test.go`

---

## Phase 2: Vendor Adapters (Weeks 4-6)

### US40.4: Cisco Adapter Tests
**As a** developer
**I want** comprehensive tests for Cisco device adapters
**So that** IOS and NX-OS management is reliable

**Acceptance Criteria**:
- Tests for IOS command execution and output parsing
- Tests for NX-OS JSON API handling
- Tests for enable mode and config mode transitions
- Tests for device facts gathering
- Tests for configuration backup and restore

**Technical Tasks**:
1. Create `pkg/vendors/cisco/ios_test.go`
2. Create `pkg/vendors/cisco/nxos_test.go`
3. Add mock SSH shell for testing

### US40.5: Juniper Adapter Tests
**As a** developer
**I want** comprehensive tests for Juniper JunOS adapter
**So that** Juniper device management is reliable

**Acceptance Criteria**:
- Tests for JunOS command execution
- Tests for XML/JSON output parsing
- Tests for configuration operations
- Tests for device facts gathering

**Technical Tasks**:
1. Create `pkg/vendors/juniper/junos_test.go`

### US40.6: VyOS Adapter Tests
**As a** developer
**I want** comprehensive tests for VyOS adapter
**So that** VyOS router management is reliable

**Acceptance Criteria**:
- Tests for VyOS command execution
- Tests for configuration mode operations
- Tests for commit and save operations
- Tests for device facts gathering

**Technical Tasks**:
1. Create `pkg/vendors/vyos/vyos_test.go`

### US40.7: Firewall Adapter Tests
**As a** developer
**I want** comprehensive tests for firewall adapters
**So that** OPNsense and pfSense management is reliable

**Acceptance Criteria**:
- Tests for OPNsense API operations
- Tests for pfSense API operations
- Tests for rule management
- Tests for device facts gathering

**Technical Tasks**:
1. Create `pkg/vendors/opnsense/opnsense_test.go`
2. Create `pkg/vendors/pfsense/pfsense_test.go`

### US40.8: Arista Adapter Tests
**As a** developer
**I want** comprehensive tests for Arista EOS adapter
**So that** Arista switch management is reliable

**Acceptance Criteria**:
- Tests for eAPI operations
- Tests for command execution
- Tests for device facts gathering

**Technical Tasks**:
1. Create `pkg/vendors/arista/eos_test.go`

---

## Phase 3: API Handlers (Weeks 7-8)

### US40.9: Agent API Handler Tests
**As a** developer
**I want** comprehensive tests for agent API handlers
**So that** the REST API is reliable

**Acceptance Criteria**:
- Tests for GET /api/v1/agents (list with filtering)
- Tests for GET /api/v1/agents/{id} (single agent)
- Tests for label filtering and sorting
- Tests for status filtering
- Tests for error responses

**Technical Tasks**:
1. Create `pkg/api/agents/handlers_test.go`
2. Add httptest-based handler tests

### US40.10: Event API Handler Tests
**As a** developer
**I want** comprehensive tests for event API handlers
**So that** event queries are reliable

**Acceptance Criteria**:
- Tests for event listing with filters
- Tests for event type and severity filtering
- Tests for time range queries
- Tests for pagination

**Technical Tasks**:
1. Create `pkg/api/events/handlers_test.go`

### US40.11: Execution API Handler Tests
**As a** developer
**I want** comprehensive tests for execution API handlers
**So that** remote execution via REST is reliable

**Acceptance Criteria**:
- Tests for command submission
- Tests for execution status queries
- Tests for result retrieval
- Tests for targeting validation

**Technical Tasks**:
1. Create `pkg/api/execution/handlers_test.go`

### US40.12: Webhook API Handler Tests
**As a** developer
**I want** comprehensive tests for webhook handlers
**So that** webhook processing is reliable

**Acceptance Criteria**:
- Tests for webhook registration
- Tests for webhook delivery
- Tests for signature validation

**Technical Tasks**:
1. Create `pkg/api/webhooks/handlers_test.go`

---

## Phase 4: Module Runtime (Week 9)

### US40.13: Module Runtime Tests
**As a** developer
**I want** comprehensive tests for module runtime
**So that** Starlark and WASM execution is reliable

**Acceptance Criteria**:
- Tests for Starlark builtin functions
- Tests for WASM builtin functions
- Tests for capability enforcement
- Tests for resource limits

**Technical Tasks**:
1. Create `pkg/module/runtime/builtins_test.go`
2. Create `pkg/module/runtime/wasm_builtins_test.go`
3. Create `pkg/module/runtime/types_test.go`

---

## Phase 5: CLI Commands (Weeks 10-12)

### US40.14: Backup CLI Tests
**As a** developer
**I want** tests for backup CLI command logic
**So that** backup operations are reliable

**Acceptance Criteria**:
- Tests for backup type determination
- Tests for component selection
- Tests for compression options
- Tests for filter parsing

**Technical Tasks**:
1. Create `cmd/kscore-backup/main_test.go` or extract logic to testable functions

### US40.15: Agent CLI Tests
**As a** developer
**I want** tests for agent CLI command logic
**So that** agent management is reliable

**Acceptance Criteria**:
- Tests for label filter parsing
- Tests for status filtering
- Tests for output formatting

**Technical Tasks**:
1. Create `cmd/kscore-agents/main_test.go`

### US40.16: Event CLI Tests
**As a** developer
**I want** tests for event CLI command logic
**So that** event queries are reliable

**Acceptance Criteria**:
- Tests for event filtering
- Tests for severity validation
- Tests for output formatting

**Technical Tasks**:
1. Create `cmd/kscore-events/main_test.go`

### US40.17: Remaining CLI Tests
**As a** developer
**I want** tests for remaining CLI commands
**So that** all CLI operations are reliable

**Acceptance Criteria**:
- Tests for kscore-proxy device filtering
- Tests for kscore-schedule cron parsing
- Tests for kscore-upgrade version checking
- Tests for kscore-loadtest metrics
- Tests for kscore-test filtering
- Tests for kscore-monitor client

**Technical Tasks**:
1. Create test files for each remaining CLI package
2. Extract business logic to testable functions where needed

---

## Dependencies

### Required Libraries
- `net/http/httptest` - HTTP handler testing
- `github.com/stretchr/testify` - Assertions (if used in project)

### Epic Dependencies
- None (independent testing epic)

### Internal Dependencies
- Phase 1 should complete before Phase 2 (protocol tests needed for vendor adapter tests)
- Phases 2-5 can be parallelized

---

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| External dependencies hard to mock | High | Create mock implementations for SSH, SNMP, WinRM |
| Tests require real devices | High | Use mock responses based on real device output |
| CLI tests require refactoring | Medium | Extract business logic to testable functions |
| Large scope | Medium | Prioritize by code complexity and risk |

---

## Testing Strategy

### Unit Tests
- Table-driven tests for parsing and filtering logic
- Mock dependencies for isolation
- Test both success and error paths
- Test edge cases and boundary conditions

### Integration Tests
- HTTP handler tests using httptest
- Mock server tests for protocol implementations

### Test Patterns
- Use `t.TempDir()` for filesystem isolation
- Use table-driven tests where appropriate
- Include interface compliance tests
- Follow existing codebase patterns

---

## Definition of Done

### Per User Story
- [ ] Test file created with comprehensive coverage
- [ ] Tests cover normal operation, edge cases, and error conditions
- [ ] Tests pass locally and in CI
- [ ] Coverage meets target (>70% for core, >40% for CLI)
- [ ] Tests follow project patterns

### Per Phase
- [ ] All user stories complete
- [ ] No test failures
- [ ] Coverage targets met
- [ ] Code reviewed

### Epic Complete
- [ ] All phases complete
- [ ] All 23 packages have test coverage
- [ ] Overall project test coverage improved
- [ ] CI passes with new tests
