# TODO.md

This is a TODO list of work that still needs to be done outside any current epic.

## Short-Term Priority (1-2 Releases)

### Test Coverage Improvements
- [x] Epic 3 (State Management): Add integration tests for module Apply/Test methods
  - Module Apply/Test methods represent 76% of statemgmt code
  - Require integration tests (not unit tests) due to system dependencies
  - **COMPLETED**: Added 16 integration tests in `module_apply_integration_test.go` covering:
    - Service module (full lifecycle, enable/disable)
    - File module (create, directory, symlink, update content)
    - Cmd module (run, creates condition, environment vars)
    - Git module (clone)
    - Docker container module (full lifecycle)
    - X509 module (self-signed certificates)
    - IniFile module (set value, idempotency)
    - Archive module (extract)
    - Sysctl module (set value)
    - Lineinfile module (add, replace lines)
  - Uses fake command pattern for external command dependencies
  - Tests idempotency (apply twice, second should report no changes)
  - Tests full Check→Apply→Test cycle

---

## Notes

- Test coverage targets: >70% for critical packages, >40% for CLI
- Performance benchmarks should be tracked in CI/CD with regression alerting
- All new features should include comprehensive documentation and tests
- Security considerations should be reviewed for all changes
