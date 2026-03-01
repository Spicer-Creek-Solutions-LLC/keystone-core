# Epic 32: Cross-Platform Testing Infrastructure

## Overview

This epic consolidates and implements comprehensive cross-platform testing infrastructure for Windows and macOS across all Keystone Core components. Currently, cross-platform testing requirements are scattered across 8+ epics (9, 16, 18, 20, 27, 28, 29, 100), leading to inconsistent coverage and duplicate planning.

**Epic Type**: Infrastructure, Testing, CI/CD

**Scope**:
- Windows CI/CD testing matrix (all supported Windows versions)
- macOS CI/CD testing (Intel and Apple Silicon)
- Cross-platform module testing (stdlib modules)
- Windows/macOS bootstrap testing
- VM-based testing infrastructure (consolidating deferred release readiness references)
- Cross-platform E2E test scenarios
- Platform-specific installer testing (MSI, PKG)

**Out of Scope**:
- Linux testing (already well-established)
- Container-based testing (covered by existing E2E infrastructure)
- Cloud provider-specific testing (see Epic 33)
- Performance benchmarking (separate effort)

## Rationale

### Problem Statement

Cross-platform testing appears in multiple TODO items across the project:

| Source | Item |
|--------|------|
| Epic 9 | Starlark debugger testing on Windows |
| Epic 16 | Cross-platform module testing (Windows, macOS) |
| Epic 18 | IPv6 testing in all CI/CD jobs |
| Epic 20 | Comprehensive Windows CI/CD testing matrix |
| Epic 27 | Windows and macOS bootstrap support |
| Epic 28 | Windows and macOS deployment blueprints |
| Epic 29 | Windows and macOS bootstrap testing |
| TODO.md | VM-based testing infrastructure (release readiness) |

This fragmentation causes:
1. **Inconsistent Coverage**: Some components tested on Windows, others not
2. **Duplicate Infrastructure**: Multiple epics planning similar VM testing
3. **Unclear Ownership**: No single epic responsible for cross-platform CI/CD
4. **Blocked Features**: Bootstrap and blueprints waiting on test infrastructure

### Benefits

1. **Unified Infrastructure**: Single, reusable cross-platform testing framework
2. **Consistent Coverage**: All components tested on all supported platforms
3. **Faster Iteration**: Shared infrastructure reduces per-epic setup time
4. **Clear Ownership**: Dedicated epic for cross-platform testing concerns
5. **Unblocks Dependents**: Enables completion of Epics 27, 28, 29 cross-platform work

## Objectives

1. **O1**: Establish Windows CI/CD testing matrix covering Windows Server 2019, 2022, and Windows 10/11
2. **O2**: Establish macOS CI/CD testing for macOS 12+ on both Intel and Apple Silicon
3. **O3**: Create reusable VM provisioning infrastructure for Windows and macOS
4. **O4**: Achieve >80% test pass rate for stdlib modules on Windows and macOS
5. **O5**: Enable Windows and macOS bootstrap testing with full scenario coverage
6. **O6**: Create platform-specific installer testing (MSI validation, PKG signing)
7. **O7**: Integrate IPv6 testing across all platform CI/CD jobs
8. **O8**: Document platform-specific testing patterns and troubleshooting

## Platform Support Matrix

### Windows Targets

| Version | Type | Priority | CI Runner |
|---------|------|----------|-----------|
| Windows Server 2022 | Server | P0 | GitHub Actions windows-2022 |
| Windows Server 2019 | Server | P1 | GitHub Actions windows-2019 |
| Windows 11 | Desktop | P1 | Self-hosted or cloud |
| Windows 10 | Desktop | P2 | Self-hosted or cloud |

### macOS Targets

| Version | Architecture | Priority | CI Runner |
|---------|--------------|----------|-----------|
| macOS 14 (Sonoma) | ARM64 (M1/M2/M3) | P0 | GitHub Actions macos-14 |
| macOS 13 (Ventura) | ARM64 | P1 | GitHub Actions macos-13 |
| macOS 13 (Ventura) | x86_64 (Intel) | P1 | GitHub Actions macos-13-large |
| macOS 12 (Monterey) | x86_64 | P2 | GitHub Actions macos-12 |

### Test Categories

| Category | Windows | macOS | Notes |
|----------|---------|-------|-------|
| Unit Tests | ✓ | ✓ | All packages |
| Integration Tests | ✓ | ✓ | API, NATS, database |
| Stdlib Modules | ✓ | ✓ | Platform-specific modules |
| E2E Scenarios | ✓ | ✓ | Agent lifecycle, state apply |
| Bootstrap | ✓ | ✓ | Full bootstrap wizard |
| Installer | ✓ (MSI) | ✓ (PKG) | Platform installers |
| IPv6 | ✓ | ✓ | Dual-stack testing |

## Architecture

### VM Provisioning Infrastructure

```
┌─────────────────────────────────────────────────────────────────┐
│                    Cross-Platform Test Infrastructure            │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │   GitHub     │  │   Self-      │  │   Cloud      │          │
│  │   Actions    │  │   Hosted     │  │   VMs        │          │
│  │   Runners    │  │   Runners    │  │   (on-demand)│          │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘          │
│         │                 │                  │                   │
│         └────────────────┼──────────────────┘                   │
│                          │                                       │
│                          ▼                                       │
│         ┌────────────────────────────────┐                      │
│         │     Test Orchestration Layer    │                      │
│         │  - Platform detection           │                      │
│         │  - Test selection               │                      │
│         │  - Result aggregation           │                      │
│         └────────────────┬───────────────┘                      │
│                          │                                       │
│         ┌────────────────┼────────────────┐                     │
│         ▼                ▼                ▼                      │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐            │
│  │   Windows    │ │    macOS     │ │    Linux     │            │
│  │   Test       │ │    Test      │ │    Test      │            │
│  │   Suite      │ │    Suite     │ │    Suite     │            │
│  └──────────────┘ └──────────────┘ └──────────────┘            │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### Test Infrastructure Components

```
test/
├── platforms/
│   ├── windows/
│   │   ├── setup.ps1           # Windows environment setup
│   │   ├── teardown.ps1        # Cleanup script
│   │   ├── skip_conditions.go  # Windows-specific skip logic
│   │   └── helpers_windows.go  # Windows test helpers
│   ├── darwin/
│   │   ├── setup.sh            # macOS environment setup
│   │   ├── teardown.sh         # Cleanup script
│   │   ├── skip_conditions.go  # macOS-specific skip logic
│   │   └── helpers_darwin.go   # macOS test helpers
│   └── shared/
│       ├── vm_provisioner.go   # VM lifecycle management
│       ├── platform_matrix.go  # Platform/version matrix
│       └── result_reporter.go  # Cross-platform result aggregation
├── cross-platform/
│   ├── stdlib_test.go          # Stdlib module cross-platform tests
│   ├── bootstrap_test.go       # Bootstrap cross-platform tests
│   ├── ipv6_test.go            # IPv6 cross-platform tests
│   └── installer_test.go       # Installer validation tests
└── ci/
    ├── windows-matrix.yaml     # Windows CI configuration
    ├── macos-matrix.yaml       # macOS CI configuration
    └── cross-platform.yaml     # Combined workflow
```

## Deliverables

### D1: Windows CI/CD Matrix

GitHub Actions workflow with full Windows testing matrix.

**Features**:
- Windows Server 2019 and 2022 runners
- PowerShell 5.1 and 7.x testing
- Windows service installation testing
- Registry and ACL module testing
- MSI installer validation

### D2: macOS CI/CD Matrix

GitHub Actions workflow with macOS testing coverage.

**Features**:
- macOS 12, 13, 14 runners
- Intel and Apple Silicon (ARM64) testing
- launchd service testing
- PKG installer validation
- Code signing verification

### D3: VM Provisioning Framework

Reusable Go library for VM lifecycle management.

**Features**:
- Packer templates for Windows and macOS images
- Terraform modules for cloud VM provisioning
- SSH/WinRM connection management
- Artifact upload/download
- Parallel VM orchestration

### D4: Cross-Platform Test Helpers

Go test helpers for platform-aware testing.

**Features**:
- Platform detection and skip conditions
- Path normalization utilities
- Service management wrappers
- Privilege escalation helpers
- Temp directory management

### D5: Stdlib Module Test Matrix

Comprehensive test coverage for all 94 stdlib modules.

**Coverage**:
- Linux: 100% (baseline)
- Windows: >80% (platform-applicable modules)
- macOS: >80% (platform-applicable modules)

### D6: Bootstrap Test Suite

Full bootstrap testing on Windows and macOS.

**Scenarios**:
- Fresh install (demo mode)
- Production cluster join
- Upgrade from previous version
- Rollback on failure
- Blueprint application

### D7: Installer Validation Suite

Platform-specific installer testing.

**Windows (MSI)**:
- Silent install/uninstall
- Upgrade scenarios
- Custom install paths
- Service registration
- Firewall rules

**macOS (PKG)**:
- Notarization verification
- Code signature validation
- launchd plist installation
- Uninstall via pkgutil

### D8: IPv6 Test Integration

IPv6 testing integrated into all platform CI jobs.

**Coverage**:
- Dual-stack agent communication
- IPv6-only mode
- DNS resolution
- Firewall configuration

### D9: Documentation

Cross-platform testing documentation for contributors.

**Contents**:
- Adding new platform tests
- Skip conditions and platform detection
- Local testing on Windows/macOS
- Troubleshooting platform-specific failures
- VM provisioning guide

## Acceptance Criteria

### AC1: Windows CI/CD Operational
- [ ] Windows Server 2022 tests pass in CI
- [ ] Windows Server 2019 tests pass in CI
- [ ] PowerShell module tests functional
- [ ] MSI installer tests automated

### AC2: macOS CI/CD Operational
- [ ] macOS 14 ARM64 tests pass in CI
- [ ] macOS 13 Intel tests pass in CI
- [ ] launchd integration tested
- [ ] PKG installer tests automated

### AC3: Stdlib Module Coverage
- [ ] >80% stdlib modules tested on Windows
- [ ] >80% stdlib modules tested on macOS
- [ ] Platform-specific modules tested on target platform
- [ ] Skip conditions documented for inapplicable modules

### AC4: Bootstrap Testing Complete
- [ ] Windows bootstrap scenarios tested
- [ ] macOS bootstrap scenarios tested
- [ ] Failure recovery tested on both platforms
- [ ] Upgrade scenarios tested on both platforms

### AC5: IPv6 Testing Integrated
- [ ] IPv6 tests run on Windows CI
- [ ] IPv6 tests run on macOS CI
- [ ] Dual-stack scenarios covered
- [ ] IPv6-only scenarios covered

### AC6: Documentation Complete
- [ ] Platform testing guide published
- [ ] Skip condition reference documented
- [ ] Local development setup documented
- [ ] Troubleshooting guide published

## Sub-Issues / Tasks

### Phase 1: Foundation (Weeks 1-3)

#### T1.1: Create Platform Test Infrastructure
Set up the base test infrastructure for cross-platform testing.

**Deliverables**:
- `test/platforms/` directory structure
- Platform detection utilities
- Skip condition framework

#### T1.2: Windows CI Workflow (Basic)
Create initial Windows CI workflow with basic test matrix.

**Deliverables**:
- `.github/workflows/windows.yaml`
- Windows Server 2022 job
- Unit test execution

#### T1.3: macOS CI Workflow (Basic)
Create initial macOS CI workflow with basic test matrix.

**Deliverables**:
- `.github/workflows/macos.yaml`
- macOS 14 ARM64 job
- Unit test execution

### Phase 2: Test Helpers (Weeks 4-6)

#### T2.1: Windows Test Helpers
Create Windows-specific test helpers and utilities.

**Deliverables**:
- `test/platforms/windows/helpers_windows.go`
- PowerShell execution wrapper
- Registry test utilities
- Service management helpers

#### T2.2: macOS Test Helpers
Create macOS-specific test helpers and utilities.

**Deliverables**:
- `test/platforms/darwin/helpers_darwin.go`
- launchctl wrapper
- plist management utilities
- Code signing verification

#### T2.3: Cross-Platform Path Utilities
Create path normalization and temp directory utilities.

**Deliverables**:
- Path separator handling
- Temp directory creation/cleanup
- Home directory resolution

### Phase 3: Stdlib Module Testing (Weeks 7-10)

#### T3.1: Audit Stdlib Modules for Platform Applicability
Review all 94 stdlib modules and categorize by platform.

**Deliverables**:
- Platform applicability matrix
- Skip condition annotations
- Test gap analysis

#### T3.2: Windows Stdlib Module Tests
Implement tests for Windows-applicable stdlib modules.

**Deliverables**:
- Registry module tests
- Windows service module tests
- Windows firewall module tests
- NTFS ACL module tests

#### T3.3: macOS Stdlib Module Tests
Implement tests for macOS-applicable stdlib modules.

**Deliverables**:
- launchd module tests
- plist module tests
- macOS user/group module tests
- Homebrew module tests (if applicable)

#### T3.4: Cross-Platform Module Tests
Test modules that work on both platforms.

**Deliverables**:
- File module cross-platform tests
- Package manager abstraction tests
- Network module tests
- Process module tests

### Phase 4: Bootstrap Testing (Weeks 11-14)

#### T4.1: Windows Bootstrap Test Suite
Implement bootstrap testing on Windows.

**Deliverables**:
- Demo mode bootstrap test
- Cluster join test
- Service installation verification
- Upgrade/rollback tests

#### T4.2: macOS Bootstrap Test Suite
Implement bootstrap testing on macOS.

**Deliverables**:
- Demo mode bootstrap test
- Cluster join test
- launchd agent installation
- Upgrade/rollback tests

#### T4.3: Blueprint Application Tests
Test blueprint application on Windows and macOS.

**Deliverables**:
- Security baseline blueprint test
- Monitoring stack blueprint test
- Platform-specific blueprint tests

### Phase 5: Installer Testing (Weeks 15-17)

#### T5.1: MSI Installer Test Suite
Automated testing for Windows MSI installer.

**Deliverables**:
- Silent install test
- Upgrade test
- Uninstall test
- Custom path test
- Service registration verification

#### T5.2: PKG Installer Test Suite
Automated testing for macOS PKG installer.

**Deliverables**:
- Notarization check
- Code signature verification
- launchd plist installation
- Uninstall verification

### Phase 6: IPv6 Integration (Weeks 18-19)

#### T6.1: IPv6 Test Suite
Comprehensive IPv6 testing on all platforms.

**Deliverables**:
- Dual-stack communication tests
- IPv6-only mode tests
- DNS AAAA record tests
- Platform firewall IPv6 tests

#### T6.2: CI Integration
Integrate IPv6 tests into all platform CI jobs.

**Deliverables**:
- IPv6-enabled CI runners (or skip conditions)
- IPv6 test job in matrix
- Result reporting

### Phase 7: VM Infrastructure (Weeks 20-22)

#### T7.1: Packer Templates
Create Packer templates for Windows and macOS images.

**Deliverables**:
- Windows Server 2022 template
- Windows Server 2019 template
- macOS Ventura template
- macOS Sonoma template

#### T7.2: Terraform Provisioning
Create Terraform modules for on-demand VM provisioning.

**Deliverables**:
- AWS Windows VM module
- AWS macOS VM module (if available)
- Azure Windows VM module
- GCP Windows VM module

#### T7.3: VM Orchestration Library
Go library for VM lifecycle management in tests.

**Deliverables**:
- `test/platforms/shared/vm_provisioner.go`
- Parallel VM creation
- Artifact deployment
- Result collection

### Phase 8: Documentation and Polish (Weeks 23-24)

#### T8.1: Platform Testing Documentation
Comprehensive documentation for contributors.

**Deliverables**:
- Adding platform tests guide
- Local testing setup
- Troubleshooting guide
- Platform matrix reference

#### T8.2: CI Optimization
Optimize CI for speed and reliability.

**Deliverables**:
- Test parallelization
- Caching optimization
- Flaky test mitigation
- Matrix optimization

#### T8.3: Nightly Full Matrix
Set up nightly runs for full platform matrix.

**Deliverables**:
- Nightly workflow
- Full matrix (all versions)
- Performance tracking
- Regression alerting

## Dependencies

- **Epic 12** (E2E Testing): Provides base E2E infrastructure patterns
- **Epic 20** (Windows Support): Provides Windows-specific modules and service code
- **Epic 27** (Bootstrap): Provides bootstrap code to test
- **Epic 28** (Standard Blueprints): Provides blueprints to test
- **GitHub Actions**: Windows and macOS runner availability

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| macOS runner costs | High CI costs | Use GitHub-hosted runners; optimize test selection |
| Flaky platform tests | CI unreliability | Implement retry logic; quarantine flaky tests |
| Platform API changes | Test breakage | Pin OS versions; abstract platform APIs |
| IPv6 unavailability | Incomplete testing | Skip IPv6 tests gracefully; document gaps |
| VM provisioning complexity | Slow CI | Use pre-built images; parallel provisioning |

## Success Metrics

| Metric | Target |
|--------|--------|
| Windows CI jobs | 2+ (Server 2019, 2022) |
| macOS CI jobs | 2+ (ARM64, Intel) |
| Stdlib module coverage (Windows) | >80% |
| Stdlib module coverage (macOS) | >80% |
| Bootstrap scenarios tested | 5+ per platform |
| Installer tests automated | MSI + PKG |
| IPv6 test coverage | All platforms |
| CI pass rate | >95% |

## Definition of Done

- [ ] All deliverables (D1-D9) implemented and operational
- [ ] All acceptance criteria (AC1-AC6) met
- [ ] Windows and macOS CI passing in GitHub Actions
- [ ] Documentation published to docs site
- [ ] Nightly full-matrix runs operational
- [ ] TODO.md cross-platform items marked complete
- [ ] Dependent epics (27, 28, 29) unblocked

## Future Considerations

- ARM64 Windows testing (when available)
- Additional macOS versions as released
- Integration with third-party CI platforms (Azure DevOps, GitLab)
- Automated performance regression testing per platform
- Browser-based testing for future Web UI
