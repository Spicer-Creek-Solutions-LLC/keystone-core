# Epic 59: Simplification

> **Status**: COMPLETE ✅

## Overview & Success Criteria

This epic focuses on refactoring Keystone Core to the minimum amount of code needed to do the job. There are no external users yet, so API/ABI stability is not a constraint. The goal is to aggressively reduce complexity, eliminate duplication, and collapse unnecessary layers while preserving required functionality.

**Success Criteria**
- Remove or merge redundant subsystems and eliminate dead code paths.
- Reduce cognitive overhead through fewer abstractions and tighter package boundaries.
- Shrink dependency surface area and build targets without loss of required capability.
- Update documentation to reflect the simplified architecture and workflows.
- Maintain or improve test coverage for retained functionality.

## Rationale

The repository has accumulated a wide set of features and patterns that were valuable for exploration and completeness. With a 0.1.0 release on the horizon, the priority shifts to simplicity, maintainability, and minimalism. Because there are no external users yet, we can aggressively refactor and consolidate without compatibility constraints.

Key drivers:
- Minimize operational and maintenance burden.
- Improve developer onboarding speed.
- Reduce surface area for bugs and security issues.
- Improve clarity of the core "golden paths."

## Scope

- Consolidate overlapping functionality across packages and CLIs.
- Remove features or code paths that are not required for initial release goals.
- Simplify configuration structures and defaults.
- Reduce or eliminate unnecessary indirection and abstractions.
- Reduce dependency graph and build targets where possible.
- Align tests and docs with simplified behavior and interfaces.

## Out of Scope

- Expanding feature coverage.
- Performance optimization beyond what is necessary for correctness.
- Large-scale feature redesigns that are not directly tied to simplification.

## Guiding Principles

- Prefer deletion over refactoring when possible.
- One canonical path per workflow.
- Favor small, explicit interfaces over generic abstractions.
- Fewer packages and binaries, unless separation is required for security or deployment.
- Documentation must always describe the actual simplified behavior.

---

## Work Plan

### Phase 1: Inventory and Target Selection — COMPLETE ✅

Ran 5 parallel inventory agents (binary inventory, package duplication, dead code detection, dependency analysis, golden path mapping). Produced prioritized simplification backlog.

**Deliverables:**
- Inventory report with 8 prioritized recommendations
- Golden path mapping (bootstrap → agent registration → state apply → event → audit)

### Phase 2: Dead Code and Build System Cleanup — COMPLETE ✅

**Makefile fix** (`5d729229`):
- Added 11 missing binaries to BINARIES list and individual build targets
- All 36 binaries now have both individual and `make build` targets

**Dead package removal** (`ff546248`):
- `internal/federation/` — multi-cluster federation (946 lines, superseded by `internal/cluster/`)
- `internal/baremetal/` — bare metal discovery (893 lines, superseded by proxy agent protocols)
- `internal/dr/` — DR drill scheduler (687 lines, duplicates runbook + schedule systems)
- `internal/edge/` — edge device manager (760 lines, not integrated with agent/statemgmt)
- `internal/visualization/` — websocket topology server (716 lines, prototype for future Web UI)
- `internal/transfer/throttle.go` — bandwidth throttler (619 lines, superseded by `internal/airgap/sync/`)
- Total: 17 files, ~10,210 lines removed

**Dead shared library removal** (`9656cdd6`):
- `pkg/retry/` — shared retry library with zero importers (987 lines)
- Internal retry implementations (secrets, webhook) have domain-specific features that don't benefit from a generic wrapper

### Phase 3: Bug Fixes and Shared Utilities — COMPLETE ✅

**SQLite MaxOpenConns fix** (`bd9e7200`):
- 6 SQLite stores were missing `SetMaxOpenConns(1)`, risking "database is locked" errors
- Fixed: `statemgmt/history`, `runbook/storage`, `cluster/token`, `identity/token_store`, `kscore-server` (outbound webhook DB, runbook DB)
- Created `pkg/dbutil` with `OpenSQLite()` factory for future stores (WAL + MaxOpenConns=1 + optional PRAGMAs)

**InsecureSkipVerify security fix** (`a1f46a7d`):
- Blueprint registry and file HTTP backend allowed `InsecureSkipVerify` without requiring `KSCORE_ALLOW_INSECURE_TLS=1` env var
- Now consistent with module registry clients that properly enforce this gate

### Phase 4: Remaining Consolidation — COMPLETE ✅ (investigated, deferred)

Each item was investigated in depth. All deferred after analysis showed low ROI:

| Item | Assessment | Decision |
|------|-----------|----------|
| HTTP client factory (`pkg/httputil`) | 17 creation sites, all domain-specific with legitimate differences | **Skip** — no meaningful consolidation possible |
| Error type standardization (`pkg/errors`) | 62 sentinels already well-organized per package; inline errors carry domain context | **Skip** — existing patterns are appropriate |
| Config struct consolidation | High effort, high risk for config-heavy code | **Deferred** — revisit if config sprawl becomes a pain point |
| Build-tag gating heavy deps | All remaining deps actively used; adds CI complexity, fragments "zero-dep" UX | **Skip** — premature optimization |

### Phase 5: Documentation and Epic Completion — COMPLETE ✅

- Updated `epics/59-simplification.md` — marked COMPLETE
- Updated `AGENTS.md` — moved Epic 59 to completed

---

## Summary of Changes

| Commit | Description | Impact |
|--------|-------------|--------|
| `5d729229` | Add 11 missing binaries to Makefile | All 36 binaries buildable |
| `ff546248` | Remove 6 dead internal packages | -10,210 lines |
| `9656cdd6` | Remove unused `pkg/retry` | -987 lines |
| `bd9e7200` | Fix 6 SQLite stores + add `pkg/dbutil` | Bug fix + shared utility |
| `a1f46a7d` | Gate InsecureSkipVerify in 2 HTTP clients | Security fix |

**Total lines removed**: ~11,197
**Bugs fixed**: 6 SQLite MaxOpenConns, 2 InsecureSkipVerify gates

## Dependencies

- Future release readiness epic for final validation scope.

## Risks & Mitigations

- **Risk**: Over-deletion removes needed capabilities.
  - **Mitigation**: Verified zero importers before every deletion; full build/lint/test after each change.
- **Risk**: Simplification introduces regressions.
  - **Mitigation**: `make build` (36 binaries), `make lint` (0 issues), `make test` (0 failures) after every commit.

## Testing Strategy

- Regression tests for minimal supported workflows.
- Table-driven tests for simplified APIs.
- Integration tests for core control-plane and agent interactions.

## Definition of Done

- Redundant components removed or merged with no orphaned code paths.
- Configuration and CLI surface materially smaller and documented.
- Tests updated and passing with coverage targets met.
- Documentation updated across docs, README, and executive summary.
