# Future: Release Readiness

## Overview

Prepare Keystone Core for the 0.1.0 project announcement by completing final release readiness tasks across the codebase, documentation, and blueprint registry.

**Goal**: Ensure 0.1.0 release quality and consistency before public announcement.

## Success Criteria

1. Blueprint catalog signing and registry verification complete
2. VM-based bootstrap validation completed on real hosts
3. All version references in code, docs, and examples normalized to 0.1.0
4. Repository generation utility produces all distribution artifacts
5. Final release checklist and announcement notes prepared
6. No lingering draft/placeholder content in official documentation

## Scope

### Signing + Registry Verification
- Sign official `kscore/*` blueprint bundles
- Publish signatures and update registry metadata
- Verify signature checks in registry pipeline

### Version Reset to 0.1.0
- Replace all explicit version references across code/docs/examples to 0.1.0
- Audit changelog, README, docs, and templates for version strings
- Ensure tooling defaults target 0.1.0

### Repository Generation Utility
- Build DNF/YUM repository with RPM packages
- Build APT repository with DEB packages
- Build Windows repository (MSI/ZIP packages)
- Build blueprint registry with signed bundles
- Build module registry for Keystone modules
- Generate repository metadata and indices
- Output to `build/repos/` for web server or CDN hosting

### Documentation Audit
- Update release notes and compatibility docs
- Validate examples against 0.1.0 references
- Remove or flag outdated references (0.10.x, 1.x, etc.)

### VM-Based Bootstrap Validation (from Epic 29)
- Run bootstrap scenarios against real VMs (control-plane + agent)
- Validate join flows, blueprint application, and basic health checks
- Record results and document known limitations

### Release Checklist
- Final QA checklist (tests, packaging, registry availability)
- Announcement copy and upgrade guidance

## Implementation Plan

### Phase 1: Registry + Signing
- Generate/sign blueprint bundles
- Update registry metadata with checksums + signatures
- Verify registry acceptance flow

### Phase 2: Version Normalization
- Scan for version strings in repo
- Update references to 0.1.0
- Validate docs + examples compile/parse

### Phase 3: Repository Generation Utility
- Create `cmd/kscore-repo-gen/` utility
- Implement DNF/YUM repository generation (createrepo_c compatible)
- Implement APT repository generation (with GPG signing)
- Implement Windows repository generation
- Implement blueprint registry generation (Go-mod style endpoints)
- Implement module registry generation
- Add Makefile target `make repos` to generate all repositories
- Generate to `build/repos/` with structure for static hosting

### Phase 4: Release Documentation
- Update release notes and compatibility docs
- Draft announcement and checklist

### Phase 5: VM Bootstrap Validation
- Configure VM test inventory
- Run VM scenarios and capture results
- Document VM findings

## Progress

### Phase 1: Registry + Signing (IN PROGRESS)
- ✅ Added official blueprint validation tests (`TestOfficialBlueprintsBuild`, `TestOfficialBlueprintsMetadata`, `TestOfficialBlueprintsStatesExist`)
- ✅ All 14 official kscore/* blueprints validated (demo, edge-deployment, enterprise-platform, file-distribution, gitops-integration, identity-federation, kubernetes-operator, metrics-only, monitoring-stack, nats-cluster, postgres-ha, production-cluster, proxy-agents, security-baseline)
- ✅ Signing/verification infrastructure tested (key-based and keyless)
- ⏳ Actual signing of bundles requires production signing key (to be done at release time)

### Phase 2: Version Normalization (COMPLETE)
- ✅ Blueprint versions normalized to 0.1.0 (monitoring-stack, security-baseline updated from 1.0.0)
- ✅ Registry metadata (catalog.json, individual manifests) updated to 0.1.0
- ✅ Documentation examples updated (support.md, maintenance.md, development.md, upgrade-cluster.md)
- ✅ Upgrade examples use v0.2.0 as target version

### Phase 3: Repository Generation Utility (COMPLETE)
- ✅ Created `cmd/kscore-repo-gen/` utility with subcommands (all, dnf, apt, windows, blueprints, modules)
- ✅ Created `internal/repogen/` package with types, generator, and tests
- ✅ DNF/YUM repository generation (el8, el9 × x86_64, aarch64)
- ✅ APT repository generation (jammy, noble, bookworm, trixie × amd64, arm64)
- ✅ Windows repository generation (x64, arm64) with manifest.json and install.ps1
- ✅ Blueprint registry generation (Go-mod style: /{vendor}/{name}/@v/)
- ✅ Module registry generation placeholder
- ✅ Added `make repos` target and related targets (repos-dnf, repos-apt, repos-windows, repos-blueprints, repos-modules)
- ✅ All 14 official blueprints included in generated registry

### Phase 4: Release Documentation (COMPLETE)
- ✅ CHANGELOG.md updated with 0.1.0 release notes
- ✅ Compatibility documentation updated with correct dates (0.1.x release: 2026-01-28)
- ✅ Release notes page created (docs/content/en/docs/community/release-notes.md)
- ✅ Release checklist created (docs/content/en/docs/community/release-checklist.md)
- ✅ Announcement copy drafted (docs/content/en/docs/community/announcement-0.1.0.md)

### Phase 5: VM Bootstrap Validation (COMPLETE)
- ✅ Single-node demo test config template (`test/bootstrap/vm/single-node-demo.yaml`)
- ✅ Single-node demo test scenarios (`test/bootstrap/vm/scenarios/demo_single_node_test.go`)
  - `testDemoBootstrap`: Installs packages and runs bootstrap
  - `testDemoHealth`: Verifies systemd services and ports
  - `testDemoAPI`: Verifies API endpoints respond
- ✅ Makefile targets: `test-vm`, `test-vm-demo`, `test-vm-smoke`, `repo-server`
- ✅ Support for Ubuntu, Debian, Rocky/RHEL, openSUSE via SSH provider
- ✅ Multi-node cluster test config (`test/bootstrap/vm/multi-node-cluster.yaml`)
- ✅ Multi-node cluster test scenarios (`test/bootstrap/vm/scenarios/multi_node_cluster_test.go`)
  - `testClusterBootstrap`: Bootstraps control-plane and agent nodes
  - `testClusterHealth`: Verifies services on all nodes
  - `testAgentRegistration`: Verifies agent registered with control-plane
- ✅ Bootstrap fixes for multi-node deployment:
  - NATS directory ownership (kscore user)
  - Server config field names (httplisten/grpclisten)
  - Verify phase retry logic for server startup
  - Skip cluster membership check for single-node setups
  - Skip storage/postgres validation for agent-only nodes
  - NATS URL resolution for agent join flow
  - Disable JetStream/auth for external mode agents
- ✅ Tests executed successfully on Ubuntu 24.04 VMs (kscp1 + ksagent1)

## Definition of Done

- [x] Blueprint registry signing infrastructure verified
- [x] Version references normalized to 0.1.0
- [x] Repository generation utility complete
- [x] Documentation audit complete
- [x] VM bootstrap validation complete
- [ ] Release checklist approved
