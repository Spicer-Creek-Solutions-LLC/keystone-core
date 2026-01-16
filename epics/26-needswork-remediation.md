# Epic 26: NEEDSWORK Remediation (Remaining Work)

## Overview

Epic 26 now focuses on the remaining Medium/Low items in `NEEDSWORK.md` after all Critical and High priority issues were resolved. The work centers on reliability, test infrastructure, CLI consistency, developer experience, documentation quality, and lingering TODO/FIXME items.

**Goal**: Complete all remaining Medium/Low gaps so the project is production-ready and maintainable.

**Reference**: `NEEDSWORK.md` for the full issue catalog and context.

**Status**: Complete

## Success Criteria

- [x] Medium/Low items in `NEEDSWORK.md` resolved (25 Medium, 12 Low)
- [x] Test flakiness reduced (remove time.Sleep in tests; add wait helpers)
- [x] Coverage improved for critical packages (pkg/api, pkg/statemgmt, pkg/cluster, pkg/gateway)
- [x] CLI output and audit logging standardized across plugins
- [x] Developer setup and SDK verification documented and automated
- [x] Documentation gaps closed (best practices, SDK docs, migration guides, glossary)
- [x] High/Medium TODO/FIXME items removed or tracked with issues

## Scope

### Remaining Items by Category (from NEEDSWORK.md)

| Category | Medium | Low | Total |
|----------|--------|-----|-------|
| Security | 2 | 0 | 2 |
| API Completeness | 3 | 0 | 3 |
| Documentation | 4 | 3 | 7 |
| Testing | 6 | 3 | 9 |
| Code Quality | 8 | 5 | 13 |
| Examples | 2 | 1 | 3 |
| **TOTAL** | **25** | **12** | **37** |

## Implementation Plan

**Total Duration**: 6-10 weeks (4 phases + ongoing)

---

## Phase 0: Backlog Validation (Week 1)

### T0.1: Confirm Remaining Work
- [x] Reconcile `NEEDSWORK.md` with codebase (remove items already fixed)
- [x] Convert remaining Medium/Low items into trackable tasks
- [x] Tag TODO/FIXME items with issue IDs or resolve them
- [x] Assign owners and estimates

**Acceptance Criteria**:
- Backlog is accurate, scoped, and owned
- Already-fixed items are removed from the plan

---

## Phase 1: Reliability & Correctness (Weeks 2-4)

### T1.1: Replace time.Sleep in Tests
- [x] Add wait helpers under `pkg/testing/helpers`
- [x] Replace `time.Sleep()` with condition-based waits
- [x] Add timeout controls for slow CI environments

### T1.2: Shared Mock Infrastructure
- [x] Create `pkg/testing/mocks/` for NATS, DB, policy, files
- [x] Migrate key packages to shared mocks

### T1.3: Canary Metrics Real Implementation
- [x] Implement Prometheus queries in `pkg/upgrade/rolling.go`
- [x] Add threshold config and error handling

### T1.4: Version Compatibility Matrix
- [x] Define upgrade compatibility rules in `pkg/upgrade/version.go`
- [x] Implement `IsCompatibleWith()` with tests

### T1.5: mDNS Discovery
- [x] Implement mDNS discovery using `hashicorp/mdns`
- [x] Add unit tests for local discovery

### T1.6: Integration Test Coverage
- [x] Add gRPC+REST integration tests
- [x] Add multi-service coordination tests

**Acceptance Criteria**:
- Flaky tests reduced and time.Sleep removed from targeted packages
- Canary metrics and version compatibility implemented
- mDNS discovery verified
- Integration tests exist for key API flows

---

## Phase 2: CLI Consistency & Auditability (Weeks 4-6)

### T2.1: Output Format Standardization
- [x] Create shared output formatter package
- [x] Add `--output json|yaml|table|text` across plugins

### T2.2: Error Handling Consistency
- [x] Standardize wrapped errors and typed error patterns

### T2.3: Audit Logging Coverage
- [x] Add audit logging to remaining CLI plugins
- [x] Provide helper utilities for consistent implementation

### T2.4: UX Improvements for CLI
- [x] Add `--dry-run` to all write operations
- [x] Add progress indicators for long operations
- [x] Improve error messages with suggestions/doc links

**Acceptance Criteria**:
- Uniform output formats and error patterns across CLI
- Audit logging coverage for all plugins
- Safer write operations via dry-run and progress feedback

---

## Phase 3: Developer Experience & Code Health (Weeks 6-8)

### T3.1: Dev Setup Improvements
- [x] Add `scripts/setup-dev.sh`
- [x] Add `make sdk-verify` target
- [x] Add pre-commit hooks
- [x] Add `.vscode/` settings

### T3.2: Codebase Consistency
- [x] Standardize constructor patterns and lifecycle methods
- [x] Address large functions, deep nesting, magic numbers
- [x] Remove commented-out code

### T3.3: Package Organization
- [x] Split `pkg/statemgmt` into subdirectories as needed
- [x] Consolidate or document example module locations

**Acceptance Criteria**:
- One-command dev setup works
- SDK verification target exists
- Code consistency standards documented and applied

---

## Phase 4: Documentation & UX Polish (Weeks 8-10)

### T4.1: Documentation Gaps
- [x] Best practices guide
- [x] SDK documentation beyond READMEs
- [x] Migration guides (Salt->Keystone, SQLite->PostgreSQL, embedded->external)
- [x] Glossary/terminology page
- [x] Blueprint parameter passing documented

### T4.2: Getting Started UX
- [x] Minimal “hello world” example
- [x] Config validation command (`kscorectl config validate`)
- [x] Shell completion docs

**Acceptance Criteria**:
- Docs cover all gaps listed in `NEEDSWORK.md`
- New users can complete a quick start without ambiguity

---

## Ongoing: TODO/FIXME Triage

- [x] Resolve or track all High/Medium TODOs with issue IDs
- [x] Remove low-value or stale TODOs

---

## Definition of Done

- [x] All Medium/Low items from `NEEDSWORK.md` resolved or explicitly deferred
- [x] Tests added or updated with clear coverage improvements
- [x] Documentation updated and consistent with implementation
- [x] CLI behavior consistent across all plugins
- [x] No untracked TODO/FIXME remains in critical paths

---

## Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Test refactors introduce new flakiness | Medium | Medium | Land in small batches, add helpers first |
| CLI consistency changes break scripts | Medium | Medium | Maintain backward compatibility and document changes |
| Doc updates diverge from code | Low | Medium | Tie doc changes to code PRs |

---

## Dependencies

- `hashicorp/mdns` for mDNS discovery
- Shared test utilities under `pkg/testing`
- Coordination with Epics 27-29 for bootstrap testing scope
