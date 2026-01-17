# Epic 30: CLI UX Restructuring

## Overview

Restructure the Keystone Core CLI commands to improve user experience by splitting oversized commands into focused, purpose-specific tools. This epic addresses cognitive overload from commands with too many subcommands and improves discoverability, RBAC alignment, and workflow clarity.

**Goal**: Reduce command complexity by splitting 6 oversized commands into 12 focused commands, improving UX without breaking existing functionality.

**Status**: Planned

## Rationale

### Current Problems

Analysis of the 17 CLI binaries revealed several UX issues:

| Command | Subcommands | Issue |
|---------|-------------|-------|
| `kscore-blueprint` | 17 | Far too many; mixes publish/consume workflows |
| `kscore-identity` | 7+ with nesting | Federation buried 3 levels deep |
| `kscore-cluster` | 9 | Backup/restore mixed with cluster ops |
| `kscore-files` | 6+ | Confusing `files files` hierarchy |
| `kscore-policy` | 6 | Audit/compliance mixed with policy management |
| `kscore-gitops` | 5 + nested | Webhook setup buried as sub-subcommand |

### Why This Matters

1. **Cognitive Load**: Users scanning `--help` output for 17 subcommands struggle to find what they need
2. **Discoverability**: Important functionality (webhooks, federation, audit) is buried in subcommand hierarchies
3. **RBAC Misalignment**: Different personas (publishers vs consumers, operators vs auditors) share the same command
4. **Workflow Confusion**: Unrelated operations grouped together obscure natural workflows
5. **Automation Friction**: Overly broad commands complicate permission scoping in CI/CD

### Design Principles

1. **Single Responsibility**: Each command should serve one primary purpose
2. **Persona Alignment**: Commands should align with user roles (operator, publisher, auditor)
3. **Flat Over Nested**: Prefer new top-level commands over deeply nested subcommands
4. **7±2 Rule**: Keep subcommand count between 5-9 for optimal usability
5. **Backward Compatibility**: Existing commands continue to work during transition

## Objectives

1. **O1**: Split `kscore-blueprint` (17 subcommands) into 3 focused commands
2. **O2**: Extract `kscore-federation` from `kscore-identity`
3. **O3**: Extract `kscore-cluster-backup` from `kscore-cluster`
4. **O4**: Extract `kscore-files-storage` from `kscore-files`
5. **O5**: Extract `kscore-audit` from `kscore-policy`
6. **O6**: Elevate `kscore-webhook` from `kscore-gitops`
7. **O7**: Maintain backward compatibility with deprecation warnings
8. **O8**: Update all documentation to reflect new command structure

## Command Restructuring Plan

### 1. kscore-blueprint Split (17 → 8 + 5 + 4)

**Current State**: Single command with 17 subcommands mixing lifecycle, publishing, and state management.

**New Structure**:

#### kscore-blueprint (Core Lifecycle - Keep, Reduce)
```
kscore-blueprint
├── init        # Create new blueprint
├── validate    # Validate blueprint syntax
├── lint        # Run linting checks
├── test        # Run blueprint tests
├── search      # Search registries
├── install     # Install blueprint
├── update      # Update installed blueprint
└── remove      # Remove blueprint
```

#### kscore-blueprint-publish (New - Publication Workflow)
```
kscore-blueprint-publish
├── publish     # Publish to registry
├── sign        # Sign blueprint
├── verify      # Verify signature
├── versions    # Manage versions
└── docs        # Generate documentation
```

#### kscore-blueprint-state (New - State/Snapshot Management)
```
kscore-blueprint-state
├── snapshot    # Create state snapshot
├── rollback    # Rollback to previous state
├── list        # List snapshots
└── diff        # Compare snapshots
```

**Rationale**:
- Publishers (sign, publish, versions) are different users than consumers (install, update)
- Snapshots are operational/backup concerns, not blueprint authoring
- `info` functionality absorbed into `search --detailed`

---

### 2. kscore-identity Split (7+ Nested → 5 + 7)

**Current State**: Federation has 7 sub-subcommands buried under `identity federation`.

**New Structure**:

#### kscore-identity (Keep, Simplify)
```
kscore-identity
├── status      # Provider status
├── token       # Token management (create/list/show/revoke)
├── ca          # CA management (info/backup/restore/rotate)
├── bundle      # Trust bundles (show/export)
└── events      # Event monitoring
```

#### kscore-federation (New - Trust Federation)
```
kscore-federation
├── list        # List federated domains
├── add         # Add federation relationship
├── show        # Show federation details
├── suspend     # Suspend federation
├── activate    # Activate federation
├── remove      # Remove federation
└── refresh     # Refresh trust bundles
```

**Rationale**:
- Federation is a distinct trust domain deserving dedicated tooling
- Reduces `identity` command complexity
- Aligns with how multi-party trust is managed separately in enterprise environments

---

### 3. kscore-cluster Split (9 → 7 + 4)

**Current State**: Mixes runtime cluster operations with infrastructure backup/restore.

**New Structure**:

#### kscore-cluster (Keep - Runtime Operations)
```
kscore-cluster
├── status           # Cluster status
├── members          # List members
├── leader           # Leader info
├── add              # Add node
├── remove           # Remove node
├── transfer-leader  # Transfer leadership
└── rebalance        # Rebalance workloads
```

#### kscore-cluster-backup (New - Infrastructure/DR)
```
kscore-cluster-backup
├── create      # Create backup
├── restore     # Restore from backup
├── list        # List backups
└── export      # Export cluster config
```

**Rationale**:
- Backup/restore are disaster recovery operations, not day-to-day cluster management
- Different permission models (infrastructure vs operations)
- Better for automation (backups are scheduled, not interactive)

---

### 4. kscore-files Split (6+ → 3 + 5)

**Current State**: Confusing hierarchy with `files files` subcommand and mixed concerns.

**New Structure**:

#### kscore-files (Keep - Server and File Operations)
```
kscore-files
├── serve       # Start file distribution server
├── list        # List files (was: files list)
├── get         # Download file (was: files get)
├── put         # Upload file (was: files put)
└── delete      # Delete file (was: files delete)
```

#### kscore-files-storage (New - Storage Administration)
```
kscore-files-storage
├── backend     # Configure backends (add/remove/list/info)
├── cache       # Cache operations (clear/stats/configure)
├── namespace   # Namespace management (create/delete/list)
├── mirrors     # Mirror configuration (add/remove/list)
└── status      # Storage system status
```

**Rationale**:
- Eliminates confusing `files files` nesting
- Separates file client operations from storage administration
- Different user roles (file consumers vs storage admins)

---

### 5. kscore-policy Split (6 → 4 + 4)

**Current State**: Policy management mixed with audit/compliance reporting.

**New Structure**:

#### kscore-policy (Keep - Policy Management)
```
kscore-policy
├── list        # List policies
├── show        # Show policy details
├── validate    # Validate policy syntax
└── check       # Evaluate policy against input
```

#### kscore-audit (New - Audit and Compliance)
```
kscore-audit
├── log         # View audit log
├── search      # Search audit events
├── report      # Generate compliance report
└── stats       # Audit statistics
```

**Rationale**:
- Policy operators (manage rules) ≠ compliance auditors (review activity)
- Enables separate RBAC for policy management vs audit access
- Better aligns with compliance tool ecosystems

---

### 6. kscore-webhook Elevation (Sub-subcommand → Top-level)

**Current State**: Webhook management buried under `gitops webhook`.

**New Structure**:

#### kscore-gitops (Keep - Operational Workflows)
```
kscore-gitops
├── verify      # Run verification workflow
├── rollback    # Trigger rollback
├── promote     # Promote between environments
└── status      # Show operation status
```

#### kscore-webhook (New - Integration Setup)
```
kscore-webhook
├── list        # List webhook handlers
├── show        # Show webhook details
├── test        # Test webhook endpoint
├── configure   # Configure webhook
├── events      # View recent webhook events
└── replay      # Replay webhook payload
```

**Rationale**:
- Webhooks are infrastructure setup (done once), separate from daily GitOps
- ArgoCD, Flux, GitHub, GitLab integrations deserve dedicated tooling
- Improves discoverability for integration setup

---

## Summary of Changes

| Original Command | New Commands | Subcommand Reduction |
|------------------|--------------|----------------------|
| `kscore-blueprint` (17) | `kscore-blueprint` (8) + `kscore-blueprint-publish` (5) + `kscore-blueprint-state` (4) | 17 → 8 primary |
| `kscore-identity` (7+) | `kscore-identity` (5) + `kscore-federation` (7) | 7+ nested → 5 flat |
| `kscore-cluster` (9) | `kscore-cluster` (7) + `kscore-cluster-backup` (4) | 9 → 7 |
| `kscore-files` (6+) | `kscore-files` (5) + `kscore-files-storage` (5) | Eliminates nesting |
| `kscore-policy` (6) | `kscore-policy` (4) + `kscore-audit` (4) | 6 → 4 |
| `kscore-gitops` (5+) | `kscore-gitops` (4) + `kscore-webhook` (6) | Elevates buried feature |

**Net Result**: 6 commands become 12 commands, but each is more focused and usable.

## Backward Compatibility

### Deprecation Strategy

1. **Phase 1**: New commands available alongside old structure
2. **Phase 2**: Old subcommands emit deprecation warnings pointing to new commands
3. **Phase 3**: Old subcommands removed (major version bump)

### Compatibility Shims

```go
// Example: kscore-blueprint publish → kscore-blueprint-publish publish
if subcommand == "publish" {
    fmt.Fprintf(os.Stderr, "DEPRECATED: 'kscore-blueprint publish' is deprecated.\n")
    fmt.Fprintf(os.Stderr, "Use 'kscore-blueprint-publish publish' instead.\n")
    // Execute with deprecation warning
}
```

### Timeline

| Phase | Duration | Action |
|-------|----------|--------|
| Phase 1 | v0.2.0 | New commands available, no warnings |
| Phase 2 | v0.3.0 | Deprecation warnings on old paths |
| Phase 3 | v1.0.0 | Remove deprecated subcommands |

## Deliverables

### D1: New CLI Binaries (6 new commands)
- `cmd/kscore-blueprint-publish/`
- `cmd/kscore-blueprint-state/`
- `cmd/kscore-federation/`
- `cmd/kscore-cluster-backup/`
- `cmd/kscore-files-storage/`
- `cmd/kscore-audit/`
- `cmd/kscore-webhook/`

### D2: Refactored Existing Commands
- Updated `cmd/kscore-blueprint/`
- Updated `cmd/kscore-identity/`
- Updated `cmd/kscore-cluster/`
- Updated `cmd/kscore-files/`
- Updated `cmd/kscore-policy/`
- Updated `cmd/kscore-gitops/`

### D3: Shared Package Extraction
- Extract shared logic to `pkg/` for code reuse
- Avoid duplication between old and new commands

### D4: kscorectl Plugin Registration
- Register new commands in kscorectl plugin system
- Update command discovery

### D5: Documentation Updates
- Update CLI reference documentation
- Add migration guide for users
- Update examples and tutorials

### D6: Deprecation Infrastructure
- Deprecation warning system
- Migration path documentation

## Acceptance Criteria

### AC1: New Commands Functional
- [ ] All 7 new commands build and pass tests
- [ ] Each new command has `--help` with clear descriptions
- [ ] Commands registered in kscorectl

### AC2: Existing Commands Updated
- [ ] Subcommands moved to new locations
- [ ] No functionality lost
- [ ] Tests updated and passing

### AC3: Backward Compatibility
- [ ] Old subcommand paths still work
- [ ] Deprecation warnings displayed
- [ ] Migration path documented

### AC4: Documentation Complete
- [ ] CLI reference updated for all commands
- [ ] Migration guide published
- [ ] Examples updated

### AC5: Integration Verified
- [ ] E2E tests cover new command structure
- [ ] CI/CD examples updated
- [ ] Shell completions regenerated

## Sub-Issues / Tasks

### Phase 1: Infrastructure and Planning (Week 1)

#### T1.1: Create Deprecation Framework
- Design deprecation warning system
- Implement reusable deprecation helpers
- Add deprecation tracking

**Deliverable**: `pkg/cli/deprecation/` package

#### T1.2: Audit Shared Logic
- Identify code to extract to shared packages
- Document dependencies between commands
- Plan extraction sequence

**Deliverable**: Shared code extraction plan

#### T1.3: Update Build System
- Add new binaries to Makefile
- Update goreleaser configuration
- Ensure cross-platform builds

**Deliverable**: Build system updated

### Phase 2: Blueprint Command Split (Weeks 2-3)

#### T2.1: Create kscore-blueprint-publish
- Create `cmd/kscore-blueprint-publish/`
- Move publish, sign, verify, versions, docs subcommands
- Add deprecation shims to original command

**Deliverable**: Working `kscore-blueprint-publish` binary

#### T2.2: Create kscore-blueprint-state
- Create `cmd/kscore-blueprint-state/`
- Move snapshot, rollback subcommands
- Add list and diff functionality

**Deliverable**: Working `kscore-blueprint-state` binary

#### T2.3: Refactor kscore-blueprint
- Remove migrated subcommands
- Add deprecation warnings for old paths
- Consolidate `info` into `search --detailed`

**Deliverable**: Streamlined `kscore-blueprint` with 8 subcommands

#### T2.4: Blueprint Tests and Docs
- Update unit tests for all three commands
- Update CLI documentation
- Add migration examples

**Deliverable**: Tests passing, docs updated

### Phase 3: Identity and Federation Split (Weeks 4-5)

#### T3.1: Create kscore-federation
- Create `cmd/kscore-federation/`
- Move federation subcommands from identity
- Implement all 7 federation operations

**Deliverable**: Working `kscore-federation` binary

#### T3.2: Refactor kscore-identity
- Remove federation subcommand
- Add deprecation warning for `identity federation`
- Simplify remaining command structure

**Deliverable**: Streamlined `kscore-identity` with 5 subcommands

#### T3.3: Federation Tests and Docs
- Update integration tests
- Update identity/federation documentation
- Add RBAC examples showing separation

**Deliverable**: Tests passing, docs updated

### Phase 4: Cluster and Backup Split (Week 6)

#### T4.1: Create kscore-cluster-backup
- Create `cmd/kscore-cluster-backup/`
- Move backup, restore subcommands
- Add list and export functionality

**Deliverable**: Working `kscore-cluster-backup` binary

#### T4.2: Refactor kscore-cluster
- Remove backup/restore subcommands
- Add deprecation warnings
- Keep version subcommand

**Deliverable**: Streamlined `kscore-cluster` with 7 subcommands

#### T4.3: Cluster Tests and Docs
- Update cluster tests
- Update backup/restore documentation
- Add DR workflow examples

**Deliverable**: Tests passing, docs updated

### Phase 5: Files and Storage Split (Week 7)

#### T5.1: Create kscore-files-storage
- Create `cmd/kscore-files-storage/`
- Move backend, cache, namespace, mirrors subcommands
- Add status subcommand

**Deliverable**: Working `kscore-files-storage` binary

#### T5.2: Refactor kscore-files
- Flatten `files` subcommand to top-level (list, get, put, delete)
- Remove storage admin subcommands
- Add deprecation warnings

**Deliverable**: Streamlined `kscore-files` with 5 subcommands

#### T5.3: Files Tests and Docs
- Update file distribution tests
- Update storage administration docs
- Fix all `files files` references

**Deliverable**: Tests passing, docs updated

### Phase 6: Policy and Audit Split (Week 8)

#### T6.1: Create kscore-audit
- Create `cmd/kscore-audit/`
- Move audit, report subcommands
- Add log, search, stats functionality

**Deliverable**: Working `kscore-audit` binary

#### T6.2: Refactor kscore-policy
- Remove audit/report subcommands
- Add deprecation warnings
- Keep core policy operations

**Deliverable**: Streamlined `kscore-policy` with 4 subcommands

#### T6.3: Audit Tests and Docs
- Update policy tests
- Create audit command documentation
- Add compliance workflow examples

**Deliverable**: Tests passing, docs updated

### Phase 7: Webhook Elevation (Week 9)

#### T7.1: Create kscore-webhook
- Create `cmd/kscore-webhook/`
- Move webhook subcommands from gitops
- Add events and replay functionality

**Deliverable**: Working `kscore-webhook` binary

#### T7.2: Refactor kscore-gitops
- Remove webhook subcommand
- Add deprecation warning for `gitops webhook`
- Keep operational workflow commands

**Deliverable**: Streamlined `kscore-gitops` with 4 subcommands

#### T7.3: Webhook Tests and Docs
- Update GitOps integration tests
- Create webhook command documentation
- Add integration setup examples

**Deliverable**: Tests passing, docs updated

### Phase 8: Integration and Polish (Weeks 10-11)

#### T8.1: kscorectl Integration
- Register all new commands as plugins
- Update plugin discovery
- Test plugin dispatch

**Deliverable**: All commands accessible via kscorectl

#### T8.2: Shell Completion Updates
- Regenerate bash completions
- Regenerate zsh completions
- Regenerate fish completions

**Deliverable**: Updated shell completions

#### T8.3: E2E Test Updates
- Update E2E tests for new command structure
- Add deprecation path tests
- Verify backward compatibility

**Deliverable**: E2E tests passing

#### T8.4: Documentation Review
- Review all CLI reference docs
- Publish migration guide
- Update tutorials and examples

**Deliverable**: Documentation complete

#### T8.5: Update CLAUDE.md
- Document new command structure
- Update command inventory
- Note deprecation timeline

**Deliverable**: CLAUDE.md updated

## Dependencies

- **Epic 25** (Blueprints): Blueprint command structure
- **Epic 17** (SPIFFE Identity): Identity command structure
- **Epic 22** (File Distribution): Files command structure
- **Epic 6** (Policy Enforcement): Policy command structure
- **Epic 5** (GitOps Integration): GitOps command structure

## Risks and Mitigations

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Breaking existing scripts | Medium | High | Deprecation period with warnings; compatibility shims |
| Code duplication | Medium | Medium | Extract shared logic to pkg/ first |
| Incomplete migration | Low | High | Comprehensive test coverage; phase gates |
| User confusion during transition | Medium | Medium | Clear migration guide; helpful deprecation messages |
| Documentation drift | Medium | Medium | Update docs in same PR as code changes |

## Testing Strategy

- **Unit Tests**: Each new command has comprehensive unit tests
- **Integration Tests**: Commands work correctly with control plane
- **E2E Tests**: Full workflows work with new command structure
- **Deprecation Tests**: Old paths work and show warnings
- **Completion Tests**: Shell completions work correctly

## Definition of Done

- [ ] All 7 new commands implemented and tested
- [ ] All 6 existing commands refactored
- [ ] Backward compatibility verified
- [ ] Deprecation warnings implemented
- [ ] Shell completions updated
- [ ] CLI reference documentation updated
- [ ] Migration guide published
- [ ] E2E tests passing
- [ ] CLAUDE.md updated with Epic 31 status
- [ ] Code review approved

## Success Metrics

| Metric | Target |
|--------|--------|
| New commands created | 7 |
| Max subcommands per command | ≤8 |
| Nesting depth | ≤2 levels |
| Test coverage on new code | >80% |
| Documentation pages updated | All CLI reference |

## Future Considerations

- Additional command splits may be identified as usage patterns emerge
- Shell completion improvements (context-aware suggestions)
- Command aliasing system for user customization
- Interactive command builder TUI
