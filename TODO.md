# TODO.md

This is a TODO list of work that still needs to be done outside any current epic.

## Short-Term Priority (1-2 Releases)

### Epic 36 State Machine Refactoring - COMPLETE

The `internal/secrets/` package has been refactored to use the `pkg/statemachine` library per project guidelines (AGENTS.md). All major components now have explicit state machines with Mermaid diagrams, guards, callbacks, and comprehensive tests.

- [x] **CircuitBreaker** (`internal/secrets/failover.go`) - COMPLETED
  - States: Closed, Open, HalfOpen
  - Added CircuitEvent type with failure, success, timeout, reset events
  - Added guards for threshold-based transitions
  - Added callbacks for counter resets on state entry
  - Added History() and CanTransition() methods
  - Includes Mermaid state diagram in code comments
  - Comprehensive test coverage added

- [x] **HealthMonitor** (`internal/secrets/failover.go`) - COMPLETED
  - States: Unknown, Healthy, Degraded, Unhealthy
  - Added HealthEvent type with check_success, check_failure events
  - Added per-backend state machines in BackendHealth
  - Added threshold-based guards for transitions
  - Added callbacks for healthy/degraded/unhealthy notifications
  - Includes Mermaid state diagram in code comments
  - Comprehensive test coverage added

- [x] **LeaseState Lifecycle** (`internal/secrets/lease_manager.go`) - COMPLETED
  - States: Pending, Active, Renewing, Expired, Revoked
  - Added LeaseTransitionEvent type for state machine events
  - Added validation state machine for transition checking
  - Added CanTransitionLease() and NextLeaseState() helpers
  - Updated Track, Renew, Revoke methods to use state machine
  - Includes Mermaid state diagram in code comments
  - Comprehensive test coverage added

- [x] **RotationState** (`internal/secrets/rotation.go`) - ALREADY COMPLETE
  - States: Pending, InProgress, Verifying, Completed, Failed, RolledBack, Cancelled
  - ManagedRotation already uses pkg/statemachine
  - Full state diagram with Mermaid documentation
  - Event-driven architecture with RotationEvent type
  - Callbacks via RotationCallbacks struct

- [x] **ClientState** (`internal/secrets/agent/client.go`) - COMPLETED
  - States: Disconnected, Connecting, Connected, Closed
  - Added ClientEvent type with connect, connected, connect_failed, disconnect, close events
  - Added state machine with guards and callbacks
  - Updated Connect(), Close(), State() to use state machine
  - Includes Mermaid state diagram in code comments
  - Comprehensive test coverage added

- [x] **HSMSessionState** (`internal/secrets/kms/hsm_session.go`) - COMPLETED
  - States: Idle, Active, Invalid, Closed
  - Added HSMSessionEvent type with acquire, release, error, close events
  - Added per-session state machines via buildSessionStateMachine()
  - Updated pool acquire/release methods to use state machine
  - Includes Mermaid state diagram in code comments
  - Comprehensive test coverage added

- [x] **HSMNodeState** (`internal/secrets/kms/hsm_failover.go`) - COMPLETED
  - States: Healthy, Degraded, Unhealthy, CircuitOpen
  - Added HSMNodeEvent type with success, degrade, circuit_trip, recovery_attempt events
  - Updated RecordSuccess(), RecordFailure(), TryRecovery() to use state machine
  - Added threshold-based event selection in RecordFailure
  - Added time-based guards for circuit breaker recovery
  - Includes Mermaid state diagram in code comments
  - Comprehensive test coverage added

- [x] **TargetStatus** (`internal/secrets/rotation.go`) - COMPLETED
  - States: Pending, Updating, Updated, Verified, Failed, RolledBack
  - Added TargetEvent type with start_update, update_success, update_failed, etc.
  - Added NextTargetStatus(), CanTransitionTarget(), TransitionTarget() helpers
  - Updated BlueGreen, Rolling, Canary strategies to use state machine
  - Includes Mermaid state diagram in code comments
  - Comprehensive test coverage added

---

## Notes

- Test coverage targets: >70% for critical packages, >40% for CLI
- Performance benchmarks should be tracked in CI/CD with regression alerting
- All new features should include comprehensive documentation and tests
- Security considerations should be reviewed for all changes
