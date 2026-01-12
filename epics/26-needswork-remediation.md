# Epic 26: NEEDSWORK Remediation

## Overview

Address all issues, gaps, and improvements identified in the comprehensive project review (NEEDSWORK.md). This epic covers security fixes, API completeness, testing improvements, documentation updates, and code quality enhancements needed before production release.

**Goal**: Resolve all critical and high-priority issues identified in the project review, bringing the codebase to production-ready quality.

**Reference**: See `NEEDSWORK.md` for complete issue details and context.

## Success Criteria

- [ ] All 8 critical issues resolved
- [ ] All 22 high-priority issues resolved
- [ ] Test coverage increased to 70%+ for critical packages
- [ ] All broken documentation links fixed
- [ ] All TODO/FIXME comments in critical paths addressed
- [ ] CLI consistency achieved across all plugins
- [ ] API services complete with proto definitions
- [ ] Blueprint examples working with correct syntax
- [ ] Security audit passes for SSH, TLS, and authentication

## Scope

### Issues by Category

| Category | Critical | High | Medium | Low | Total |
|----------|----------|------|--------|-----|-------|
| Security | 3 | 3 | 2 | 0 | 8 |
| API Completeness | 2 | 4 | 3 | 0 | 9 |
| Documentation | 1 | 5 | 8 | 4 | 18 |
| Testing | 0 | 4 | 6 | 3 | 13 |
| Code Quality | 0 | 3 | 8 | 5 | 16 |
| Examples | 2 | 3 | 2 | 1 | 8 |
| **TOTAL** | **8** | **22** | **29** | **13** | **72** |

## Implementation Plan

**Total Duration**: 12-16 weeks (4 phases)

---

## Phase 1: Critical Security Fixes (Weeks 1-2)

### Week 1: Authentication & Transport Security

#### T1.1: SSH Host Key Verification
**Priority**: 🔴 CRITICAL
**Location**: `pkg/protocols/ssh/adapter.go`
**Issue**: `InsecureIgnoreHostKey()` allows MITM attacks

**Tasks**:
- [ ] Create `KnownHostsVerifier` struct implementing `ssh.HostKeyCallback`
- [ ] Support loading known_hosts from file (`~/.ssh/known_hosts`, `/etc/ssh/ssh_known_hosts`)
- [ ] Implement "trust on first use" (TOFU) mode with optional strict mode
- [ ] Add `StrictHostKeyChecking` config option (yes, no, ask, accept-new)
- [ ] Store learned host keys in agent-local database
- [ ] Add host key fingerprint logging for audit
- [ ] Update SSHAdapter to use new verifier by default
- [ ] Add tests for all verification modes

**Acceptance Criteria**:
- SSH connections verify host keys by default
- TOFU mode stores keys on first connection
- Strict mode rejects unknown hosts
- Host key mismatches logged and rejected

#### T1.2: kscore-exec TLS Support
**Priority**: 🔴 CRITICAL
**Location**: `cmd/kscore-exec/main.go`
**Issue**: gRPC connection uses `grpc.WithInsecure()`

**Tasks**:
- [ ] Add `--tls` flag to enable TLS
- [ ] Add `--tls-ca-cert` flag for CA certificate
- [ ] Add `--tls-cert` and `--tls-key` for mTLS
- [ ] Add `--tls-skip-verify` for development (with warning)
- [ ] Default to TLS enabled when server has TLS configured
- [ ] Auto-detect TLS from server URL scheme (grpcs://)
- [ ] Add certificate loading and validation
- [ ] Update documentation with TLS examples

**Acceptance Criteria**:
- TLS connections work with CA certificate
- mTLS connections work with client certificates
- Insecure connections require explicit flag
- Clear error messages for certificate issues

#### T1.3: Grafana Security in Docker Compose
**Priority**: 🔴 CRITICAL
**Location**: `deploy/gateway/docker-compose.yml`
**Issue**: Hardcoded admin password, anonymous access enabled

**Tasks**:
- [ ] Change `GF_SECURITY_ADMIN_PASSWORD` to `${GRAFANA_ADMIN_PASSWORD:?required}`
- [ ] Set `GF_AUTH_ANONYMOUS_ENABLED=false`
- [ ] Add `.env.example` with required variables
- [ ] Update `deploy/gateway/README.md` with security setup
- [ ] Add docker-compose validation in CI

**Acceptance Criteria**:
- Docker compose fails without password set
- Anonymous access disabled by default
- Documentation explains secure setup

### Week 2: Cryptographic Security

#### T1.4: Windows MSI URL Validation
**Priority**: 🔴 CRITICAL
**Location**: `deploy/windows/Service.wxs`
**Issue**: SERVERURL accepts any input without validation

**Tasks**:
- [ ] Create custom action to validate NATS URL format
- [ ] Validate scheme (nats://, tls://, wss://)
- [ ] Validate hostname and port format
- [ ] Show clear error for invalid URLs
- [ ] Add unit tests for URL validation logic

**Acceptance Criteria**:
- Invalid URLs rejected during installation
- Clear error messages shown to user
- Valid NATS URL formats accepted

#### T1.5: Cosign Signing Implementation
**Priority**: 🔴 CRITICAL
**Location**: `pkg/blueprint/registry/publisher.go`
**Issue**: Signing is placeholder

**Tasks**:
- [ ] Integrate sigstore/cosign library
- [ ] Implement `SignBlueprint(path, keyPath)` function
- [ ] Support keyless signing with OIDC (Fulcio)
- [ ] Support key-based signing (local keys)
- [ ] Add signature verification in blueprint install
- [ ] Add `--sign` flag to `kscore-blueprint publish`
- [ ] Add `--verify` flag to `kscore-blueprint install`
- [ ] Document signing workflow

**Acceptance Criteria**:
- Blueprints can be signed with cosign
- Signatures verified on install
- Keyless signing works with OIDC providers

#### T1.6: Embedded etcd TLS Support
**Priority**: 🟠 HIGH
**Location**: `pkg/cluster/embedded.go`
**Issue**: `useTLS := false` hardcoded

**Tasks**:
- [ ] Add TLS configuration to embedded etcd config
- [ ] Generate self-signed certificates if not provided
- [ ] Support provided certificates
- [ ] Enable peer TLS for cluster communication
- [ ] Enable client TLS for API access
- [ ] Add certificate rotation support
- [ ] Update cluster documentation

**Acceptance Criteria**:
- Embedded etcd uses TLS by default
- Self-signed certs generated automatically
- Custom certificates can be provided

---

## Phase 2: API & Core Completeness (Weeks 3-6)

### Week 3-4: gRPC Service Definitions

#### T2.1: StateService Proto Definition
**Priority**: 🔴 CRITICAL
**Location**: `api/proto/state.proto` (new file)

**Tasks**:
- [ ] Create `state.proto` with StateService definition
- [ ] Define `ApplyStateRequest`/`ApplyStateResponse`
- [ ] Define `CheckStateRequest`/`CheckStateResponse`
- [ ] Define `DetectDriftRequest`/`DetectDriftResponse`
- [ ] Define `StateResult`, `DriftReport` messages
- [ ] Generate Go code with buf
- [ ] Implement StateServiceServer in `pkg/api/server/`
- [ ] Add REST gateway annotations
- [ ] Add tests for all RPCs

**Proto Definition**:
```protobuf
service StateService {
  rpc ApplyState(ApplyStateRequest) returns (ApplyStateResponse);
  rpc CheckState(CheckStateRequest) returns (CheckStateResponse);
  rpc DetectDrift(DetectDriftRequest) returns (DetectDriftResponse);
  rpc GetStateHistory(GetStateHistoryRequest) returns (GetStateHistoryResponse);
}
```

#### T2.2: EventService Proto Definition
**Priority**: 🔴 CRITICAL
**Location**: `api/proto/event.proto` (new file)

**Tasks**:
- [ ] Create `event.proto` with EventService definition
- [ ] Define `ListEventsRequest`/`ListEventsResponse`
- [ ] Define `EmitEventRequest`/`EmitEventResponse`
- [ ] Define `SubscribeEventsRequest` with streaming response
- [ ] Define `Event`, `EventFilter` messages
- [ ] Generate Go code with buf
- [ ] Implement EventServiceServer
- [ ] Add REST gateway annotations
- [ ] Add tests for all RPCs

#### T2.3: PolicyService Proto Definition
**Priority**: 🔴 CRITICAL
**Location**: `api/proto/policy.proto` (new file)

**Tasks**:
- [ ] Create `policy.proto` with PolicyService definition
- [ ] Define `EvaluatePolicyRequest`/`EvaluatePolicyResponse`
- [ ] Define `ListViolationsRequest`/`ListViolationsResponse`
- [ ] Define `GetComplianceReportRequest`/`GetComplianceReportResponse`
- [ ] Define `PolicyViolation`, `ComplianceReport` messages
- [ ] Generate Go code with buf
- [ ] Implement PolicyServiceServer
- [ ] Add REST gateway annotations
- [ ] Add tests for all RPCs

#### T2.4: ClusterService gRPC Definition
**Priority**: 🟠 HIGH
**Location**: `api/proto/cluster.proto` (new file)

**Tasks**:
- [ ] Create `cluster.proto` mirroring existing REST API
- [ ] Define cluster status, member management RPCs
- [ ] Define backup/restore RPCs
- [ ] Generate Go code
- [ ] Implement ClusterServiceServer (wrap existing handlers)
- [ ] Add tests

### Week 5-6: Blueprint & Example Fixes

#### T2.5: Fix Blueprint State File Syntax
**Priority**: 🔴 CRITICAL
**Location**: `examples/blueprints/*/states/*.yaml`
**Issue**: Files use Salt syntax instead of Keystone Core format

**Tasks**:
- [ ] Audit all blueprint state files for Salt syntax
- [ ] Convert `pkg.installed:` to `module: package` format
- [ ] Convert `file.managed:` to `module: file` format
- [ ] Convert `service.running:` to `module: service` format
- [ ] Update Jinja2 templates to Go text/template syntax
- [ ] Validate all converted files parse correctly
- [ ] Test blueprint execution end-to-end

**Files to Convert**:
- `lamp-stack/states/main.yaml`
- `lamp-stack/states/apache.yaml`
- `lamp-stack/states/mysql.yaml`
- `lamp-stack/states/php.yaml`
- `monitoring-stack/states/*.yaml`
- `security-baseline/states/*.yaml`

#### T2.6: Create Missing Blueprint State Files
**Priority**: 🔴 CRITICAL
**Location**: `examples/blueprints/security-baseline/states/`

**Tasks**:
- [ ] Create `fail2ban.yaml` state file
  - Install fail2ban package
  - Configure jail.local
  - Enable and start service
- [ ] Create `updates.yaml` state file
  - Configure unattended-upgrades (Debian/Ubuntu)
  - Configure dnf-automatic (RHEL/Fedora)
  - Set update schedule
- [ ] Add tests for new state files
- [ ] Update blueprint documentation

#### T2.7: Fix Template Protocol
**Priority**: 🟡 MEDIUM
**Location**: Blueprint examples use `source: template://`

**Tasks**:
- [ ] Option A: Implement `template://` protocol in file server
- [ ] Option B: Convert examples to use inline `content:` with templating
- [ ] Update documentation to clarify supported source protocols
- [ ] Add examples of correct template usage

---

## Phase 3: Testing & Quality (Weeks 7-10)

### Week 7-8: Critical Test Coverage

#### T3.1: Add pkg/api Tests
**Priority**: 🟠 HIGH
**Location**: `pkg/api/server/`
**Current Coverage**: 0%

**Tasks**:
- [ ] Create `controlplane_server_test.go`
- [ ] Test all 8 ControlPlaneService RPCs
- [ ] Test authentication interceptors
- [ ] Test rate limiting
- [ ] Test error handling
- [ ] Add integration tests with mock NATS
- [ ] Target: 70% coverage

**Test Cases**:
- RegisterAgent (valid, duplicate, invalid metadata)
- Heartbeat (valid, unknown agent, stale)
- ExecuteCommand (success, timeout, error)
- BatchExecuteCommand (partial success, all fail)
- GetAgentInfo (exists, not found)
- ListAgents (pagination, filtering)
- GetCommandStatus (exists, not found)
- StreamCommandOutput (streaming, cancellation)

#### T3.2: Add pkg/gateway Tests
**Priority**: 🟠 HIGH
**Location**: `pkg/gateway/`
**Current Coverage**: 23%

**Tasks**:
- [ ] Create `server_test.go` for gateway orchestration
- [ ] Test component initialization order
- [ ] Test NATS connection failure handling
- [ ] Test HTTP handler registration
- [ ] Test graceful shutdown
- [ ] Test stale data cleanup
- [ ] Target: 70% coverage

#### T3.3: Add GitOps Client Tests
**Priority**: 🟠 HIGH
**Location**: `pkg/gitops/argocd/`, `pkg/gitops/flux/`, etc.
**Current Coverage**: 1-5%

**Tasks**:
- [ ] Create mock HTTP servers for each provider
- [ ] Test ArgoCD client operations
- [ ] Test Flux client operations
- [ ] Test GitHub client operations
- [ ] Test GitLab client operations
- [ ] Test error handling and retries
- [ ] Target: 60% coverage per package

#### T3.4: Enable Skipped E2E Tests
**Priority**: 🟠 HIGH
**Location**: `test/e2e/`
**Skipped**: 47 tests

**Tasks**:
- [ ] Fix HA cluster test infrastructure
  - Add proper NATS/etcd/PostgreSQL startup checks
  - Implement retry logic for container startup
- [ ] Fix GitOps webhook tests
  - Add webhook server to test containers
  - Configure webhook endpoints
- [ ] Fix Policy tests
  - Add policy configuration to test environment
- [ ] Enable self-management tests by default
- [ ] Add dynamic port allocation to prevent conflicts
- [ ] Replace `time.Sleep()` with condition-based waits

### Week 9-10: Test Infrastructure & Quality

#### T3.5: Create Shared Mock Infrastructure
**Priority**: 🟡 MEDIUM
**Location**: `pkg/testing/` (new package)

**Tasks**:
- [ ] Create `pkg/testing/mocks/nats.go` - Mock NATS connection
- [ ] Create `pkg/testing/mocks/database.go` - Mock database
- [ ] Create `pkg/testing/mocks/policy.go` - Mock policy engine
- [ ] Create `pkg/testing/mocks/files.go` - Mock file server
- [ ] Create `pkg/testing/helpers/wait.go` - Wait helpers
- [ ] Create `pkg/testing/helpers/assert.go` - Custom assertions
- [ ] Migrate existing tests to use shared mocks
- [ ] Document mock usage patterns

#### T3.6: Replace time.Sleep() in Tests
**Priority**: 🟡 MEDIUM
**Issue**: 194 `time.Sleep()` calls cause flaky tests

**Tasks**:
- [ ] Create `WaitFor(condition, timeout)` helper
- [ ] Create `WaitForCondition(fn, interval, timeout)` helper
- [ ] Identify all test files with `time.Sleep()`
- [ ] Replace with condition-based waits
- [ ] Add timeout configuration for slow CI environments

#### T3.7: Add Missing Vendor Tests
**Priority**: 🟡 MEDIUM
**Location**: `pkg/vendors/`

**Tasks**:
- [ ] Add tests for Cisco IOS adapter
- [ ] Add tests for Cisco NX-OS adapter
- [ ] Add tests for Juniper JUNOS adapter
- [ ] Add tests for Arista EOS adapter
- [ ] Add tests for pfSense adapter
- [ ] Add tests for VyOS adapter
- [ ] Use mock SSH/REST responses

---

## Phase 4: Documentation & Polish (Weeks 11-14)

### Week 11-12: Documentation Fixes

#### T4.1: Fix Broken Documentation Links
**Priority**: 🟠 HIGH
**Issue**: 5 broken internal links

**Tasks**:
- [ ] Create `docs/content/en/docs/concepts/kubernetes.md`
- [ ] Create `docs/content/en/docs/concepts/cloud-platforms.md`
- [ ] Create `docs/content/en/docs/concepts/edge.md`
- [ ] Create `docs/content/en/docs/concepts/state-storage.md`
- [ ] Update or remove `/docs/reference/blueprints/` link (Epic 25 incomplete)
- [ ] Run link checker and fix any remaining broken links

#### T4.2: Create Missing Concept Pages
**Priority**: 🟠 HIGH

**Kubernetes Concepts** (`kubernetes.md`):
- [ ] Kubernetes integration overview
- [ ] CRD definitions (RemoteExecution, StateConfig)
- [ ] Operator controllers
- [ ] State modules (namespace, deployment)
- [ ] Service account configuration

**Cloud Platforms** (`cloud-platforms.md`):
- [ ] Multi-cloud architecture
- [ ] AWS integration (EC2, ECS, Lambda detection)
- [ ] GCP integration (GCE, GKE, Cloud Functions)
- [ ] Azure integration (VMs, AKS, Functions)
- [ ] Cloud metadata and auto-detection

**Edge** (`edge.md`):
- [ ] Edge deployment patterns
- [ ] Offline mode operation
- [ ] Local state caching
- [ ] Connection resilience
- [ ] Resource constraints

**State Storage** (`state-storage.md`):
- [ ] SQLite vs PostgreSQL
- [ ] Migration between backends
- [ ] Backup and restore
- [ ] Performance considerations

#### T4.3: Add FAQ Section
**Priority**: 🟡 MEDIUM
**Location**: `docs/content/en/docs/faq/`

**Tasks**:
- [ ] Create `_index.md` with FAQ overview
- [ ] Create `general.md` - General questions
- [ ] Create `installation.md` - Installation questions
- [ ] Create `troubleshooting.md` - Common issues
- [ ] Create `migration.md` - Migration from Salt/Ansible/Puppet
- [ ] Add 20+ common questions with answers

#### T4.4: Add Tutorials Section
**Priority**: 🟡 MEDIUM
**Location**: `docs/content/en/docs/tutorials/`

**Tasks**:
- [ ] Create `_index.md` with tutorials overview
- [ ] Create `multi-node-setup.md` - HA cluster setup
- [ ] Create `monitoring-setup.md` - Prometheus/Grafana setup
- [ ] Create `first-policy.md` - Creating your first policy
- [ ] Create `gitops-integration.md` - ArgoCD/Flux integration
- [ ] Create `custom-module.md` - Writing a custom module

### Week 13-14: CLI & Code Consistency

#### T4.5: Standardize CLI Output Formats
**Priority**: 🟡 MEDIUM
**Issue**: Inconsistent `--output` flag support

**Tasks**:
- [ ] Create `pkg/cli/output/` package with formatters
- [ ] Implement JSON, YAML, table, plain text formatters
- [ ] Add `--output` flag to all list/get commands
- [ ] Update kscore-exec to support output formats
- [ ] Update kscore-state to support output formats
- [ ] Update kscore-cluster to support output formats
- [ ] Document output format options

#### T4.6: Standardize Audit Logging
**Priority**: 🟡 MEDIUM
**Issue**: Only kscore-exec and kscore-state have audit logging

**Tasks**:
- [ ] Add audit logging to kscore-monitor
- [ ] Add audit logging to kscore-policy
- [ ] Add audit logging to kscore-module
- [ ] Add audit logging to kscore-cluster
- [ ] Add audit logging to kscore-gitops
- [ ] Add audit logging to kscore-identity
- [ ] Create audit logging helper for consistent implementation

#### T4.7: Refactor Telemetry Gateway CLI
**Priority**: 🟠 HIGH
**Issue**: Uses `flag` package instead of Cobra

**Tasks**:
- [ ] Refactor to use Cobra CLI framework
- [ ] Add `serve` subcommand (default)
- [ ] Add `version` subcommand
- [ ] Add `config validate` subcommand
- [ ] Match CLI patterns from other binaries
- [ ] Update documentation

#### T4.8: Implement Monitor Real Data
**Priority**: 🟠 HIGH
**Location**: `cmd/kscore-monitor/client/client.go`
**Issue**: Shows placeholder/mock data

**Tasks**:
- [ ] Implement gRPC client for control plane API
- [ ] Fetch real agent list and status
- [ ] Fetch real event stream
- [ ] Fetch real metrics from Prometheus
- [ ] Fetch real job history
- [ ] Add connection status indicator
- [ ] Handle API errors gracefully

---

## Phase 5: Medium & Low Priority (Weeks 15-16, Optional)

### Medium Priority Items

#### T5.1: File Cache Eviction
**Location**: `pkg/files/client.go`
```go
// TODO: Evict old entries if over size limit
```
- [ ] Implement LRU eviction when cache exceeds configured size
- [ ] Add cache size monitoring
- [ ] Add cache cleanup command

#### T5.2: Path Matching in File Server
**Location**: `pkg/files/server.go`
```go
// TODO: Implement path matching based on backend config
```
- [ ] Implement glob pattern matching for backend routing
- [ ] Support prefix-based backend selection
- [ ] Add configuration documentation

#### T5.3: Canary Metrics Implementation
**Location**: `pkg/upgrade/rolling.go`
- [ ] Connect to Prometheus for metric queries
- [ ] Implement error rate checking
- [ ] Implement latency percentile checking
- [ ] Add configurable metric thresholds

#### T5.4: Version Compatibility Matrix
**Location**: `pkg/upgrade/version.go`
- [ ] Define compatibility rules
- [ ] Implement `IsCompatibleWith()` logic
- [ ] Add upgrade path validation
- [ ] Document version compatibility

#### T5.5: mDNS Discovery Implementation
**Location**: `pkg/nats/discovery.go`
- [ ] Add hashicorp/mdns dependency
- [ ] Implement mDNS browser
- [ ] Add service registration
- [ ] Test local network discovery

### Low Priority Items

#### T5.6: Consolidate Example Modules
- [ ] Move all hello-world examples to `modules/examples/`
- [ ] Update documentation to reflect new locations
- [ ] Add symlinks for backwards compatibility

#### T5.7: Add Plugin Development Guide
- [ ] Create `docs/content/en/docs/community/plugin-development.md`
- [ ] Document plugin discovery mechanism
- [ ] Provide plugin template
- [ ] Add plugin testing guide

#### T5.8: Standardize Constructor Patterns
- [ ] Audit all `New*` functions
- [ ] Standardize on config struct pattern
- [ ] Update to return pointers consistently
- [ ] Document constructor conventions

#### T5.9: Standardize Lifecycle Methods
- [ ] Audit `Close()`, `Stop()`, `Shutdown()` usage
- [ ] Standardize on `Close()` for cleanup
- [ ] Standardize on `Stop()` for services
- [ ] Update interface documentation

---

## Definition of Done

### For Each Task
- [ ] Code implemented and compiles
- [ ] Unit tests written and passing
- [ ] Integration tests updated if applicable
- [ ] Documentation updated
- [ ] NEEDSWORK.md item marked complete
- [ ] Code reviewed and approved
- [ ] No new linting errors

### For Each Phase
- [ ] All critical items in phase completed
- [ ] All high-priority items in phase completed
- [ ] Test coverage targets met
- [ ] CI/CD pipeline passing
- [ ] Documentation reviewed

### For Epic Completion
- [ ] All critical issues resolved (8/8)
- [ ] All high-priority issues resolved (22/22)
- [ ] NEEDSWORK.md fully addressed
- [ ] Security audit passed
- [ ] Performance benchmarks acceptable
- [ ] Release notes updated

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| SSH host key changes break existing deployments | Medium | High | Add migration guide, support TOFU mode |
| Proto changes break API compatibility | Low | High | Version APIs, maintain backwards compat |
| Blueprint syntax changes break examples | High | Medium | Provide migration script |
| Test infrastructure changes cause CI failures | Medium | Medium | Run tests in parallel branch first |
| TLS requirements break dev workflows | Medium | Low | Keep `--insecure` flag for development |

---

## Dependencies

### External Dependencies
- sigstore/cosign library (for blueprint signing)
- hashicorp/mdns (for mDNS discovery)
- Updated buf configuration (for new protos)

### Internal Dependencies
- Phase 2 depends on Phase 1 (security must be fixed first)
- Phase 3 depends on Phase 2 (need working APIs to test)
- Phase 4 can run in parallel with Phase 3

---

## Estimated Effort

| Phase | Duration | Effort |
|-------|----------|--------|
| Phase 1: Security | 2 weeks | 80 hours |
| Phase 2: API | 4 weeks | 160 hours |
| Phase 3: Testing | 4 weeks | 160 hours |
| Phase 4: Documentation | 4 weeks | 120 hours |
| Phase 5: Polish | 2 weeks | 60 hours |
| **Total** | **16 weeks** | **580 hours** |

---

## Tracking

Progress will be tracked in:
1. This epic document (checkboxes)
2. NEEDSWORK.md (mark items ✅ when complete)
3. CLAUDE.md (update implementation status)
4. GitHub issues/PRs (for code changes)
