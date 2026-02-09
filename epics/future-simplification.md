# Future Epic: Simplification

> **Status**: Future (Not Yet Scheduled)

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

## Work Plan

### Phase 1: Inventory and Target Selection
- Build a map of active vs. unused or duplicated functionality:
  - `rg`-based inventory of binaries, packages, and internal entrypoints.
  - Identify build targets that are unused in 0.1.0 workflows.
- Identify overlapping implementations and competing patterns:
  - Compare adjacent packages for duplicate logic (e.g., retries, config parsing, validation, wait loops).
  - Flag multiple implementations of similar CLIs or registries.
- Define the minimal feature set required for 0.1.0:
  - Enumerate “golden path” workflows (bootstrap → agent registration → state apply → event → audit).
  - Record required binaries, APIs, configs, and docs per workflow.
- Create a deletion and consolidation plan per subsystem:
  - Propose concrete removals/merges with impact notes and replacement path.
  - Tag each proposal with “keep/remove/merge” and required test/doc updates.
- Deliverables:
  - Inventory report with ownership and usage notes.
  - Proposed minimal feature set and golden paths.
  - Prioritized simplification backlog (ranked by impact vs. risk).

### Phase 2: Simplify and Consolidate
- Merge or remove redundant packages and components.
- Collapse multi-layer abstractions into direct implementations.
- Reduce configuration sprawl and eliminate unused options.
- Remove or consolidate CLI binaries and plugin surfaces where feasible.

### Phase 3: Align Tests and Docs
- Delete or update tests for removed functionality.
- Add coverage for simplified paths and edge cases.
- Update user documentation, architecture overviews, and quick references.
- Ensure executive summary and roadmap reflect the simplified model.

### Phase 4: Validation
- Run full test suite and fix regressions.
- Validate end-to-end flows for the minimal supported workflows.
- Perform a documentation accuracy review.

## Dependencies

- Epic 100 (0.1.0 Release Readiness) for final validation scope.

## Risks & Mitigations

- **Risk**: Over-deletion removes needed capabilities.
  - **Mitigation**: Define a minimal feature set and validate it with end-to-end tests.
- **Risk**: Simplification introduces regressions.
  - **Mitigation**: Tighten tests around the simplified paths and remove obsolete tests.
- **Risk**: Documentation drift from rapid changes.
  - **Mitigation**: Make doc updates a required step of every simplification change.

## Testing Strategy

- Regression tests for minimal supported workflows.
- Table-driven tests for simplified APIs.
- Integration tests for core control-plane and agent interactions.

## Definition of Done

- Redundant components removed or merged with no orphaned code paths.
- Configuration and CLI surface materially smaller and documented.
- Tests updated and passing with coverage targets met.
- Documentation updated across docs, README, and executive summary.
