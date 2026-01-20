# TODO.md

This is a TODO list of work that still needs to be done outside any current epic.

## Short-Term Priority (1-2 Releases)

### Test Coverage Improvements
- [ ] Epic 3 (State Management): Increase coverage from 44% to >80%
- [x] Add integration tests between all major epics
- [ ] Implement load test scenarios with configurable agent counts

### Epic 8 (Multi-Environment) Gaps
- [x] Implement automated bare metal discovery with profile matching
- [x] Complete service mesh integration with mTLS policy verification
- [x] Implement K8s NetworkPolicy integration for network enforcement
- [x] Add comprehensive container registry authentication support

---

## Medium-Term Priority (2-3 Releases)

### Epic 16 (Stdlib Modules) Gaps
- [x] Increase unit test coverage to >80% for all modules (87.7% achieved)

### Epic 17 (SPIFFE Identity) Gaps
- [x] Simplify trust federation setup with interactive wizard
- [x] Add automatic fallback between attestation methods

### Epic 23 (Self-Management) Gaps
- [x] Test automatic rollback on upgrade failure extensively (11 test scenarios)

### Epic 27 (Bootstrap) Gaps
- [x] Implement comprehensive error recovery with detailed diagnostics
- [x] Implement atomic bootstrap with automatic rollback (checkpoint/transaction system)

### Epic 29 (Bootstrap Testing) Gaps
- [x] Comprehensive recovery and rollback testing

---

## Long-Term Priority (Future Versions)

### Major Features (from Roadmap)
- [ ] Scheduled operations & maintenance windows
- [ ] Multi-tenancy and namespace isolation
- [ ] Agent self-update with staged rollouts

### Infrastructure
- [ ] Create automated translation pipeline for additional languages

> **Note**: Web UI dashboard, Mobile monitoring app, and Natural language interface
> have been moved to Future Epics in AGENTS.md for separate epic planning.

---

## Notes

- Test coverage targets: >70% for critical packages, >40% for CLI
- Performance benchmarks should be tracked in CI/CD with regression alerting
- All new features should include comprehensive documentation and tests
- Security considerations should be reviewed for all changes
