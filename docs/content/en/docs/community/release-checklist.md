---
title: "Release Checklist"
weight: 3
description: >
  Pre-release verification checklist for Keystone Core releases
---

{{% alert title="Release Ceremony Required" color="warning" %}}
This checklist covers pre-release code quality gates. The actual release
process — including air-gapped builds, multi-party signing, quorum votes,
and publication — is defined in
[RELEASE-PLAYBOOK.md](https://github.com/shawnbutts/keystone-core/blob/main/RELEASE-PLAYBOOK.md).
Use Appendix B of the playbook as the ceremony-day checklist.
{{% /alert %}}

This checklist ensures release quality before beginning the release ceremony.

## Version 0.1.0 Release Checklist

### Code Quality

- [x] All tests passing (`make test`)
- [x] Test coverage meets targets (>70% critical packages, >40% CLI)
- [x] No critical or high severity security issues (`make security`)
- [x] Semgrep annotations added for intentional patterns
- [x] TLS minimum version set to 1.2+ across all components
- [x] No CGO dependencies (pure Go build)

### Documentation

- [x] CHANGELOG.md updated with 0.1.0 release notes
- [x] Release notes page created
- [x] Compatibility matrix updated with correct dates
- [x] All version references normalized to 0.1.0
- [x] CLI reference documentation complete
- [x] API documentation complete
- [x] Getting started guide verified

### Blueprints

- [x] All 14 official blueprints validated
- [x] Blueprint versions set to 0.1.0
- [x] Blueprint metadata (catalog.json) updated
- [x] Signing infrastructure verified
- [ ] Production signing keys ready (at release time)

### Repository Generation

- [x] `kscore-repo-gen` utility complete
- [x] DNF/YUM repository generation working
- [x] APT repository generation working
- [x] Windows repository generation working
- [x] Blueprint registry generation working
- [x] Module registry generation working
- [x] `make repos` target functional

### Build Artifacts

- [ ] Linux binaries (amd64, arm64) built
- [ ] Windows binaries (amd64, arm64) built
- [ ] macOS binaries (amd64, arm64) built
- [ ] Docker images built and tagged
- [ ] Helm charts packaged
- [ ] RPM packages built
- [ ] DEB packages built
- [ ] MSI installers built

### Signing & Security (performed during release ceremony)

These items are completed during the offline release ceremony per
`RELEASE-PLAYBOOK.md`. They are listed here for completeness but are
tracked in the release record, not this checklist.

- [ ] Release ceremony convened with quorum (Playbook Phase 1)
- [ ] Source tree hash verified by all participants (Playbook Phase 2)
- [ ] Dependency audit completed and voted on (Playbook Phase 3)
- [ ] Air-gapped build completed, reproducibility verified (Playbook Phase 4)
- [ ] Artifact smoke tests and vulnerability scan passed (Playbook Phase 5)
- [ ] Checksums signed by majority of authorized signers (Playbook Phase 6)
- [ ] SBOMs signed (Playbook Phase 6)
- [ ] Container images built from signed binaries, scanned, and signed with Cosign (Playbook Phase 8)
- [ ] Release record completed and signed by all participants
- [ ] Publication approved per channel (Playbook Phase 9)

### Registry & Distribution

- [ ] Container images pushed to registry
- [ ] Helm charts published to chart repository
- [ ] Package repositories generated and uploaded
- [ ] Blueprint registry populated
- [ ] Module registry populated
- [ ] Download links verified
- [ ] Post-release verification completed within 24 hours (Playbook Phase 10)

### Validation

- [ ] Control plane bootstrap tested on Ubuntu 22.04
- [ ] Control plane bootstrap tested on RHEL 9
- [ ] Agent join tested on Linux
- [ ] Agent join tested on Windows
- [ ] Basic state apply verified
- [ ] Monitoring stack blueprint deployed
- [ ] Security baseline blueprint applied

### Announcement

- [ ] Release announcement drafted
- [ ] Blog post prepared (if applicable)
- [ ] Social media posts prepared
- [ ] Mailing list notification ready
- [ ] GitHub release created

---

## General Release Process

### Pre-Release (T-7 days)

1. Feature freeze - no new features merged
2. Run full test suite including integration tests
3. Run security scans and address findings
4. Update CHANGELOG with all changes
5. Update compatibility documentation
6. Verify all documentation is current

### Release Candidate (T-3 days)

1. Tag release candidate (e.g., v0.1.0-rc1)
2. Build all artifacts from RC tag
3. Run smoke tests on built artifacts
4. Validate on target platforms
5. Address any blocking issues
6. Tag additional RCs if needed

### Release Day (T-0)

1. Final review of CHANGELOG and release notes
2. Tag final release (e.g., v0.1.0)
3. Build and sign all artifacts
4. Generate SBOMs and attestations
5. Upload to distribution channels
6. Create GitHub release with notes
7. Publish announcement

### Post-Release (T+1 day)

1. Verify all download links work
2. Monitor for early bug reports
3. Update website with new version
4. Notify downstream projects
5. Archive release artifacts

---

## Hotfix Release Process

For critical security or stability fixes, see `RELEASE-PLAYBOOK.md`
Phase 14 (Emergency and Patch Releases). Key differences from standard releases:

1. Minimum 2 participants (reduced quorum, requires unanimous approval)
2. Dependency audit may be scoped to changed dependencies only
3. Reproducibility verification still required (2+ independent builds)
4. All signing and verification requirements remain unchanged
