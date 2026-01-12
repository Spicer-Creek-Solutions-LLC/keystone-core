# NEEDSWORK.md - Keystone Core Project Review

> **Generated**: January 2026
> **Project Version**: 0.10.0
> **Review Scope**: Full codebase review including code, documentation, deployment, examples, and tests

This document captures all identified issues, gaps, and improvement opportunities across the Keystone Core project. Items are categorized by severity and type.

---

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [Critical Issues (Must Fix)](#critical-issues-must-fix)
3. [High Priority (Should Fix)](#high-priority-should-fix)
4. [Medium Priority (Nice to Have)](#medium-priority-nice-to-have)
5. [Low Priority (Polish)](#low-priority-polish)
6. [Documentation Gaps](#documentation-gaps)
7. [Test Coverage Gaps](#test-coverage-gaps)
8. [Developer Experience Improvements](#developer-experience-improvements)
9. [User Experience Improvements](#user-experience-improvements)
10. [Code Quality & Consistency](#code-quality--consistency)
11. [TODO/FIXME Comments in Code](#todofixme-comments-in-code)

---

## Executive Summary

**Overall Project Health**: 🟢 **Good** (85-90%)

The Keystone Core project is well-architected with 25 completed Epics, comprehensive documentation, and extensive test coverage. However, several gaps exist that should be addressed before production deployment:

| Category | Critical | High | Medium | Low |
|----------|----------|------|--------|-----|
| Security | 3 | 3 | 2 | 0 |
| API Completeness | 2 | 4 | 3 | 0 |
| Documentation | 1 | 5 | 8 | 4 |
| Testing | 0 | 4 | 6 | 3 |
| Code Quality | 0 | 3 | 8 | 5 |
| Examples | 2 | 3 | 2 | 1 |
| **TOTAL** | **8** | **22** | **29** | **13** |

---

## Critical Issues (Must Fix)

### 🔴 CRIT-1: SSH Host Key Verification Disabled

**Location**: `pkg/protocols/ssh/adapter.go:~line 50`
**Impact**: MITM attacks possible on all SSH-based proxy agent operations
**Code**:
```go
HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO: Implement proper host key verification
```
**Fix**: Implement proper host key verification with known_hosts file support or first-use trust model.

---

### 🔴 CRIT-2: Grafana Default Credentials in Docker Compose

**Location**: `deploy/gateway/docker-compose.yml:118`
**Impact**: Anyone with network access can log into Grafana as admin
**Code**:
```yaml
GF_SECURITY_ADMIN_PASSWORD=admin
GF_AUTH_ANONYMOUS_ENABLED=true
```
**Fix**: Change to environment variable reference `${GRAFANA_PASSWORD}` and disable anonymous access.

---

### 🔴 CRIT-3: Blueprint State Files Use Wrong Syntax

**Location**: `examples/blueprints/*/states/*.yaml`
**Impact**: Blueprint examples will NOT work - they use Salt Project syntax instead of Keystone Core format
**Example** (WRONG):
```yaml
apache_package:
  pkg.installed:
    - name: {{ apache_pkg }}
```
**Correct** (Keystone Core format):
```yaml
apache_package:
  module: package
  state: installed
  name: {{ apache_pkg }}
```
**Fix**: Rewrite all blueprint state files to use Keystone Core's native format.

---

### 🔴 CRIT-4: kscore-exec Uses Insecure gRPC Connection

**Location**: `cmd/kscore-exec/main.go:100`
**Impact**: Command execution traffic unencrypted, credentials could be intercepted
**Code**:
```go
// TODO: Add TLS support
conn, err := grpc.Dial(config.ServerAddress, grpc.WithInsecure())
```
**Fix**: Implement mTLS for gRPC client connection.

---

### 🔴 CRIT-5: Cosign Signing is Placeholder in Blueprint Registry

**Location**: `pkg/blueprint/registry/publisher.go`
**Impact**: Blueprints distributed without cryptographic signatures
**Code**:
```go
// TODO: Implement actual signing with cosign
```
**Fix**: Complete the cosign integration for blueprint signing.

---

### 🔴 CRIT-6: Missing gRPC Service Definitions

**Location**: `api/proto/`
**Impact**: Only 3 of 7 documented services have protobuf definitions
**Missing**:
- `StateService` (ApplyState, CheckState, DetectDrift)
- `EventService` (ListEvents, EmitEvent, SubscribeEvents)
- `PolicyService` (EvaluatePolicy, ListViolations, GetComplianceReport)
- `ClusterService` (gRPC version - REST exists)

**Fix**: Create proto definitions for all documented services.

---

### 🔴 CRIT-7: Missing Blueprint State Files

**Location**: `examples/blueprints/security-baseline/states/`
**Impact**: Security baseline blueprint incomplete - 2 of 7 features won't work
**Missing**:
- `fail2ban.yaml` (referenced by `features.fail2ban`)
- `updates.yaml` (referenced by `features.automatic_updates`)

**Fix**: Create the missing state files.

---

### 🔴 CRIT-8: Windows MSI URL Validation Missing

**Location**: `deploy/windows/Service.wxs:94`
**Impact**: SERVERURL property accepts any input without validation
**Fix**: Add custom action to validate NATS URL format.

---

## High Priority (Should Fix)

### 🟠 HIGH-1: No NetworkPolicy Templates in Kubernetes

**Location**: `deploy/kubernetes/`
**Impact**: Pods can communicate without restriction
**Fix**: Add NetworkPolicy templates blocking all ingress by default, allowing only required traffic.

---

### 🟠 HIGH-2: Embedded etcd Lacks TLS Support

**Location**: `pkg/cluster/embedded.go:~line 50`
**Impact**: Cluster communication unencrypted in small deployments
**Code**:
```go
useTLS := false // TODO: Add TLS support for embedded etcd
```
**Fix**: Enable TLS configuration for embedded etcd mode.

---

### 🟠 HIGH-3: pkg/gateway Has No Test Coverage

**Location**: `pkg/gateway/server.go`, `pkg/gateway/integration.go`
**Impact**: Gateway orchestration logic completely untested
**Fix**: Add comprehensive test suite for gateway server lifecycle.

---

### 🟠 HIGH-4: pkg/api Has No Unit Tests

**Location**: `pkg/api/server/controlplane_server.go`
**Impact**: gRPC handlers untested
**Fix**: Add unit tests for ControlPlaneServer and all handlers.

---

### 🟠 HIGH-5: Monitor TUI Uses Mock/Fake Data

**Location**: `cmd/kscore-monitor/client/client.go`
**Impact**: TUI monitor shows placeholder data instead of real metrics
**Code**:
```go
Version:   "0.1.0", // TODO: Get from control plane
Uptime:    0,       // TODO: Get from control plane
EventRate: 0,       // TODO: Get from metrics
```
**Fix**: Implement real API calls to fetch actual data.

---

### 🟠 HIGH-6: Telemetry Gateway Uses Different CLI Framework

**Location**: `cmd/kscore-telemetry-gateway/main.go`
**Impact**: Inconsistent CLI experience - uses `flag` package instead of Cobra
**Fix**: Refactor to use Cobra CLI framework like other binaries.

---

### 🟠 HIGH-7: 47 E2E Tests Skipped

**Location**: `test/e2e/`
**Impact**: Major functionality not tested in E2E scenarios
**Skipped Tests**:
- 6 HA cluster tests (NATS/etcd/PostgreSQL failures)
- 6 GitOps tests (require webhook configuration)
- 5 Policy tests (require policy configuration)
- 20 Self-management tests (require explicit flag)

**Fix**: Enable tests by improving test infrastructure and container configurations.

---

### 🟠 HIGH-8: GitOps Client Packages Have No Tests

**Location**: `pkg/gitops/argocd/`, `pkg/gitops/flux/`, `pkg/gitops/github/`, `pkg/gitops/gitlab/`
**Impact**: All GitOps integrations untested (0% coverage)
**Fix**: Add comprehensive tests for GitOps clients.

---

### 🟠 HIGH-9: Documentation Has 5 Broken Internal Links

**Location**: Various docs files
**Broken Links**:
- `/docs/reference/blueprints/` (Epic 25 not completed)
- `/docs/concepts/kubernetes/` (page doesn't exist)
- `/docs/concepts/cloud-platforms/` (page doesn't exist)
- `/docs/concepts/edge/` (page doesn't exist)
- `/docs/concepts/state-storage/` (page doesn't exist)

**Fix**: Create missing pages or remove/update links.

---

---

## Medium Priority (Nice to Have)

### 🟡 MED-1: Inconsistent CLI Output Formats

**Impact**: Some commands support `--output json/yaml/table`, others don't
**Fix**: Standardize `--output` flag across all CLI plugins.

---

### 🟡 MED-2: Inconsistent Error Handling Patterns

**Impact**: Mix of custom error types, inline fmt.Errorf, and sentinel errors
**Fix**: Standardize on wrapped errors with consistent types.

---

### 🟡 MED-3: Large Interfaces Hard to Implement

**Location**: Various packages
**Impact**: `pkg/state/interface.go` has 30+ method interfaces
**Fix**: Consider breaking into smaller, focused interfaces.

---

### 🟡 MED-4: 194 time.Sleep() Calls in Tests

**Location**: Various test files
**Impact**: Flaky tests on slow systems
**Fix**: Replace with wait helpers and condition checks.

---

### 🟡 MED-5: No Centralized Mock Infrastructure

**Impact**: Each package creates its own mocks (duplication)
**Fix**: Create `pkg/testing/` with shared mocks for NATS, database, policy, etc.

---

### 🟡 MED-6: Missing Audit Logging in Most CLI Plugins

**Impact**: Only kscore-exec and kscore-state have audit integration
**Missing**: kscore-monitor, kscore-policy, kscore-module, kscore-cluster
**Fix**: Add consistent audit logging across all plugins.

---

### 🟡 MED-7: File Cache Eviction Not Implemented

**Location**: `pkg/files/client.go`
**Code**: `// TODO: Evict old entries if over size limit`
**Fix**: Implement LRU eviction when cache exceeds size limit.

---

### 🟡 MED-8: Path Matching Incomplete in File Server

**Location**: `pkg/files/server.go`
**Code**: `// TODO: Implement path matching based on backend config`
**Fix**: Complete path-to-backend routing implementation.

---

### 🟡 MED-9: Canary Metrics Are Placeholders

**Location**: `pkg/upgrade/rolling.go`
**Impact**: Canary deployments can't actually check metrics
**Fix**: Implement real metric checking from Prometheus.

---

### 🟡 MED-10: Version Compatibility Check is Placeholder

**Location**: `pkg/upgrade/version.go`
**Impact**: `IsCompatibleWith()` doesn't actually check compatibility
**Fix**: Implement real version compatibility matrix checking.

---

### 🟡 MED-11: Template Protocol Not Implemented

**Location**: Examples use `source: template://` syntax
**Impact**: Blueprint templates won't work
**Fix**: Either implement template:// protocol or update examples to use file: state.

---

### 🟡 MED-12: mDNS Discovery is Placeholder

**Location**: `pkg/nats/discovery.go`
**Impact**: Local network NATS discovery won't work
**Fix**: Implement using hashicorp/mdns library.

---

---

## Low Priority (Polish)

### 🟢 LOW-1: Inconsistent Constructor Patterns

**Impact**: Some return pointers, others values; parameter styles vary
**Fix**: Standardize on config struct pattern with pointer returns.

---

### 🟢 LOW-2: Inconsistent Lifecycle Methods

**Impact**: Mix of `Close()`, `Stop()`, `Shutdown()` across packages
**Fix**: Standardize on `Close()` for cleanup, `Stop()` for services.

---

### 🟢 LOW-3: Version Output Inconsistency

**Impact**: kscore-monitor uses `version.Version` directly, others use `version.Get().String()`
**Fix**: Use consistent version output pattern.

---

### 🟢 LOW-4: No Plugin Development Documentation

**Location**: `docs/`
**Impact**: Third-party developers can't easily create plugins
**Fix**: Add plugin development guide.

---

### 🟢 LOW-5: Example Module Organization Fragmented

**Impact**: Hello world examples split between `modules/examples/` and `modules/sdk/*/examples/`
**Fix**: Consolidate or clearly document the split.

---

### 🟢 LOW-6: Performance Benchmarks Not Measured

**Impact**: SDK READMEs show approximate times, not actual measurements
**Fix**: Add actual benchmark results.

---

### 🟢 LOW-7: No Glossary in Documentation

**Impact**: Users may not understand domain-specific terms
**Fix**: Add glossary/terminology reference.

---

---

## Documentation Gaps

### Missing Documentation Pages

| Page | Priority | Notes |
|------|----------|-------|
| `/docs/concepts/kubernetes/` | High | Referenced but doesn't exist |
| `/docs/concepts/cloud-platforms/` | High | Referenced but doesn't exist |
| `/docs/concepts/edge/` | Medium | Referenced but doesn't exist |
| `/docs/concepts/state-storage/` | Medium | Referenced but doesn't exist |
| `/docs/reference/blueprints/` | Low | Epic 25 not completed |
| FAQ Section | Medium | No FAQ exists |
| Tutorials Section | Medium | Only quick-start exists |
| Best Practices Guide | Medium | No best practices document |
| SDK Documentation | Medium | 4 SDKs minimally documented outside of README files |
| Migration Guides | Low | Salt→Keystone, embedded→external, SQLite→PostgreSQL |

### Documentation Quality Issues

- Some code examples use outdated patterns
- API reference doesn't match implementation (7 services documented, 3 implemented)
- Blueprint parameter passing format undocumented

---

## Test Coverage Gaps

### Packages with Low/No Coverage

| Package | Coverage | Priority |
|---------|----------|----------|
| `pkg/api/` | 0% | **Critical** |
| `pkg/gitops/argocd/` | 1.4% | **Critical** |
| `pkg/gitops/flux/` | 5.1% | **Critical** |
| `pkg/gitops/github/` | 4.5% | **Critical** |
| `pkg/gitops/gitlab/` | 5.0% | **Critical** |
| `pkg/gateway/` | 23% | High |
| `pkg/statemgmt/` | 35% | Medium |
| `pkg/cluster/` | 48.6% | Medium |
| `pkg/vendors/` | Unknown | Medium |
| `pkg/visualization/` | Unknown | Low |
| `pkg/profiling/` | Unknown | Low |

### Missing Test Types

- No gRPC+REST integration tests
- No multi-service coordination tests
- No network partition E2E tests (documented as SKIPPED)
- No multi-platform E2E tests (documented as NOT IMPLEMENTED)

---

## Developer Experience Improvements

### Build & Development

1. **No Makefile target to verify SDK compilation**
   - Add `make sdk-verify` to compile all SDK examples

2. **No pre-commit hooks**
   - Add hooks for linting, formatting, test running

3. **No development environment setup script**
   - Add `scripts/setup-dev.sh` for one-command setup

4. **IDE configurations incomplete**
   - `.idea/` exists but no `.vscode/` settings

### Code Organization

1. **pkg/statemgmt is too large (102 files)**
   - Consider splitting into `pkg/statemgmt/modules/` subdirectories

2. **Scattered mock implementations**
   - Create `pkg/testing/mocks/` for shared mocks

3. **No code generation for boilerplate**
   - Consider go:generate for repetitive patterns

---

## User Experience Improvements

### CLI Improvements

1. **Add shell completion for all commands**
   - Currently undocumented or incomplete

2. **Add `--dry-run` to all write operations**
   - Currently only some commands support this

3. **Add progress indicators for long operations**
   - Blueprint install, file distribution, state apply

4. **Improve error messages**
   - Add suggestions for common errors
   - Include documentation links

### Getting Started Experience

1. **No "hello world" quick start**
   - Add simplest possible example (install package, ensure file exists)

2. **No interactive setup wizard**
   - Consider `kscorectl init` for guided setup

3. **Configuration validation incomplete**
   - Add `kscorectl config validate` command

---

## Code Quality & Consistency

### Pattern Inconsistencies

| Pattern | Packages Using A | Packages Using B |
|---------|------------------|------------------|
| Error handling | Custom types | inline fmt.Errorf |
| Lifecycle | Close() | Stop() |
| Constructors | Config struct | Individual params |
| Locking | RWMutex | plain Mutex |
| Context | Eager checking | Deferred checking |

### Code Smells

1. **Large functions** (>100 lines in some handlers)
2. **Deep nesting** (>4 levels in some control flow)
3. **Magic numbers** (timeouts, sizes without constants)
4. **Commented-out code** (should be removed)

---

## TODO/FIXME Comments in Code

**Total**: 37 TODO/FIXME comments found

### Critical (Security/Correctness)

| Location | Comment |
|----------|---------|
| `pkg/protocols/ssh/adapter.go` | TODO: Implement proper host key verification |
| `pkg/blueprint/registry/publisher.go` | TODO: Implement actual signing with cosign |
| `pkg/cluster/embedded.go` | TODO: Add TLS support for embedded etcd |
| `cmd/kscore-exec/main.go` | TODO: Add TLS support |

### High (Functionality)

| Location | Comment |
|----------|---------|
| `pkg/module/loader/loader.go` | TODO: Track from capability registry |
| `pkg/gitops/verification/engine.go` | TODO: Pass timeout context to verifier |
| `pkg/files/client.go` | TODO: Evict old entries if over size limit |
| `pkg/files/server.go` | TODO: Implement path matching |
| `pkg/files/mirror/sync.go` | TODO: Implement proper glob matching |
| `cmd/kscore-monitor/client/client.go` | TODO: Get from control plane (3 instances) |
| `cmd/kscore-cluster/client.go` | TODO: Add gRPC support |

### Medium (Improvements)

| Location | Comment |
|----------|---------|
| `pkg/events/kafka.go` | TODO: Log error properly (5 instances) |
| `pkg/events/http.go` | TODO: Log error properly |
| `pkg/statemgmt/module_web.go` | TODO: content comparison for idempotency |
| `pkg/statemgmt/module_lvm.go` | TODO: Check if PVs match, Check size match |
| `pkg/api/server/controlplane_server.go` | TODO: Parse page_token for offset |

### Low (Housekeeping)

| Location | Comment |
|----------|---------|
| `pkg/servicemesh/linkerd.go` | TODO: Extract more metadata |
| `pkg/edge/manager.go` | TODO: separate state vs command counts |
| `cmd/kscore-blueprint/cmd_install.go` | TODO: Check dependencies |
| `cmd/kscore-blueprint/cmd_search.go` | TODO: Read from config file |

---

## Summary Statistics

| Metric | Value |
|--------|-------|
| Go files | 867 |
| YAML files | 210 |
| Markdown files | 846 |
| Total lines of Go code | ~180,000 |
| Test files | 277 |
| TODO/FIXME comments | 37 |
| Critical issues | 8 |
| High priority issues | 22 |
| Medium priority issues | 29 |
| Low priority issues | 13 |
| Packages with <50% coverage | 6+ |
| Broken documentation links | 5 |
| Skipped E2E tests | 47 |

---

## Recommended Action Plan

### Phase 1: Critical Security Fixes (1-2 weeks)
1. Fix SSH host key verification (CRIT-1)
2. Fix Grafana credentials (CRIT-2)
3. Add TLS to kscore-exec (CRIT-4)
4. Fix Windows URL validation (CRIT-8)

### Phase 2: API & Core Completeness (2-4 weeks)
1. Create missing proto definitions (CRIT-6)
2. Complete cosign signing (CRIT-5)
3. Fix blueprint state file syntax (CRIT-3)
4. Add missing blueprint files (CRIT-7)

### Phase 3: Testing & Quality (2-4 weeks)
1. Add pkg/api tests (HIGH-4)
2. Add pkg/gateway tests (HIGH-3)
3. Add GitOps client tests (HIGH-8)
4. Enable skipped E2E tests (HIGH-7)

### Phase 4: Documentation & Polish (2-4 weeks)
1. Fix broken links (HIGH-9)
2. Create missing concept pages
3. Add FAQ and tutorials
4. Standardize CLI patterns (MED-1, MED-6)

---

*This document should be updated as issues are resolved. Mark items with ✅ when completed.*
