# TODO.md

This is a TODO list of work that still needs to be done outside any current epic.

## Short-Term Priority (1-2 Releases)

### Epic 36 State Machine Refactoring

The `internal/secrets/` package has 4 components that should be refactored to use the `pkg/statemachine` library per project guidelines (AGENTS.md). This will improve code clarity, reduce bugs, and align with the state machine patterns already implemented in 15 other components.

- [ ] **CircuitBreaker** (`internal/secrets/failover.go`)
  - States: Closed, Open, HalfOpen
  - Replace switch statements in `AllowRequest()`, `RecordSuccess()`, `RecordFailure()` with state machine
  - Add time-based guards for Open→HalfOpen transition
  - Add counter-based guards for threshold transitions
  - Use callbacks for counter resets on state entry

- [ ] **HealthMonitor** (`internal/secrets/failover.go`)
  - States: Unknown, Healthy, Degraded, Unhealthy
  - Replace conditional state updates in `checkBackend()` with state machine
  - Add threshold-based guards for consecutive success/failure transitions
  - Use callbacks for counter management and timestamp updates

- [ ] **LeaseState Lifecycle** (`internal/secrets/lease_manager.go`)
  - States: Pending, Active, Renewing, Expired, Revoked
  - Replace direct `lease.State = X` assignments with validated state machine transitions
  - Add guards for renewable/expiry conditions
  - Use callbacks for audit logging (`logLeaseEvent`)
  - Apply to both `InMemoryLeaseManager` and `PersistentLeaseManager`

- [ ] **RotationState** (`internal/secrets/types.go`)
  - States: Pending, InProgress, Verifying, Completed, Failed, RolledBack
  - Types defined but no implementation yet
  - Build rotation orchestrator with state machine from the start
  - Add guards for rollback-on-failure config and health check results

Each refactoring should include:
- Mermaid state diagram in code comments
- Tests for all valid transitions
- Tests that invalid transitions are rejected
- Guards for conditional transitions
- Callbacks for side effects (logging, metrics, counters)

---

## Notes

- Test coverage targets: >70% for critical packages, >40% for CLI
- Performance benchmarks should be tracked in CI/CD with regression alerting
- All new features should include comprehensive documentation and tests
- Security considerations should be reviewed for all changes
