---
title: "State Machine Patterns"
linkTitle: "State Machines"
weight: 30
description: >
  How to implement and use state machines in Keystone Core
---

## Overview

Keystone Core uses explicit state machine patterns to manage complex state transitions throughout the codebase. This document explains when to use state machines, how to implement them, and best practices for testing.

## Why State Machines?

State machines provide several benefits over ad-hoc state management:

| Benefit | Description |
|---------|-------------|
| **Clarity** | Valid states and transitions are documented in code |
| **Correctness** | Invalid transitions fail fast with clear errors |
| **Testability** | State machines can be exhaustively unit tested |
| **Debugging** | Transition history makes issues traceable |
| **Extensibility** | Adding states/transitions is localized |

### Before: Scattered State Logic

```go
// Anti-pattern: State scattered across multiple fields and functions
type ConnectionManager struct {
    state        string
    circuitOpen  bool
    failureCount int
    lastConnected time.Time
}

func (m *ConnectionManager) Connect() error {
    if m.state == "closed" {
        return errors.New("closed")
    }
    if m.circuitOpen && time.Since(m.lastConnected) < 30*time.Second {
        return errors.New("circuit open")
    }
    // More conditional logic...
}
```

### After: Explicit State Machine

```go
// Better: Explicit state machine with validated transitions
type ConnectionState string
const (
    StateDisconnected ConnectionState = "disconnected"
    StateConnecting   ConnectionState = "connecting"
    StateConnected    ConnectionState = "connected"
)

machine := statemachine.New[ConnectionState, ConnectionEvent](StateDisconnected).
    AddTransition(StateDisconnected, EventConnect, StateConnecting).
    AddTransition(StateConnecting, EventConnected, StateConnected).
    AddTransition(StateConnecting, EventFailed, StateDisconnected).
    AddTransition(StateConnected, EventDisconnect, StateDisconnected).
    MustBuild()
```

## When to Use State Machines

Use the `pkg/statemachine` package when your component has:

- **Multiple states** with distinct behaviors (3+ states)
- **Explicit transitions** that need validation
- **Lifecycle management** (init, start, run, stop, cleanup)
- **Recovery/retry logic** with distinct phases
- **Workflow orchestration** with sequential steps

### Good Candidates

- Connection managers (connecting, connected, reconnecting)
- Job execution (pending, running, completed, failed)
- Deployment pipelines (validating, deploying, verifying)
- Circuit breakers (closed, open, half-open)
- Resource lifecycle (created, active, terminating, deleted)

### When NOT to Use

- Simple boolean flags (enabled/disabled)
- Linear sequences without branching
- Short-lived operations without recovery
- External state that you don't control

## The State Machine Library

### Core Types

```go
// State and Event types must be comparable
type State string
type Event string

// Create with builder pattern
machine := statemachine.New[State, Event](initialState).
    AddTransition(from, event, to).
    Build()

// Fire events to trigger transitions
err := machine.Fire(event)

// Check current state
state := machine.State()
```

### Defining States and Events

Always define states and events as named types with constants:

```go
// Define state type
type UpgradePhase string

const (
    PhaseIdle       UpgradePhase = "idle"
    PhasePending    UpgradePhase = "pending"
    PhaseValidating UpgradePhase = "validating"
    PhasePreparing  UpgradePhase = "preparing"
    PhaseUpgrading  UpgradePhase = "upgrading"
    PhaseVerifying  UpgradePhase = "verifying"
    PhaseCompleted  UpgradePhase = "completed"
    PhaseFailed     UpgradePhase = "failed"
)

// Define event type
type UpgradeEvent string

const (
    EventStart    UpgradeEvent = "start"
    EventValidate UpgradeEvent = "validate"
    EventPrepare  UpgradeEvent = "prepare"
    EventUpgrade  UpgradeEvent = "upgrade"
    EventVerify   UpgradeEvent = "verify"
    EventComplete UpgradeEvent = "complete"
    EventFail     UpgradeEvent = "fail"
    EventRollback UpgradeEvent = "rollback"
)
```

### Building a State Machine

Use the fluent builder API:

```go
machine := statemachine.New[UpgradePhase, UpgradeEvent](PhaseIdle).
    // Define transitions
    AddTransition(PhaseIdle, EventStart, PhasePending).
    AddTransition(PhasePending, EventValidate, PhaseValidating).
    AddTransition(PhaseValidating, EventPrepare, PhasePreparing).
    AddTransition(PhasePreparing, EventUpgrade, PhaseUpgrading).
    AddTransition(PhaseUpgrading, EventVerify, PhaseVerifying).
    AddTransition(PhaseVerifying, EventComplete, PhaseCompleted).

    // Failure can happen from multiple states
    AddTransition(PhaseValidating, EventFail, PhaseFailed).
    AddTransition(PhasePreparing, EventFail, PhaseFailed).
    AddTransition(PhaseUpgrading, EventFail, PhaseFailed).
    AddTransition(PhaseVerifying, EventFail, PhaseFailed).

    // Enable history tracking for debugging
    WithHistory(100).

    // Build the machine
    MustBuild()
```

### Guard Conditions

Guards allow conditional transitions based on runtime state:

```go
machine := statemachine.New[State, Event](StateIdle).
    AddTransition(StateIdle, EventStart, StateRunning).
    WithGuard(func(ctx context.Context, from State, event Event) bool {
        // Only allow start if resources are available
        return resourceManager.HasAvailableCapacity()
    }).
    MustBuild()
```

Guards receive the context, so you can access request-scoped values:

```go
WithGuard(func(ctx context.Context, from State, event Event) bool {
    user := auth.UserFromContext(ctx)
    return user.HasPermission("upgrade:execute")
})
```

### Callbacks

Execute code on state entry, exit, or transitions:

```go
machine := statemachine.New[State, Event](StateIdle).
    // Called when entering a state
    OnEnter(StateRunning, func(ctx context.Context, state, fromState State) {
        metrics.Increment("jobs_running")
        log.Info("Job started", "from", fromState)
    }).

    // Called when exiting a state
    OnExit(StateRunning, func(ctx context.Context, state, toState State) {
        metrics.Decrement("jobs_running")
    }).

    // Called on any transition
    OnTransition(func(ctx context.Context, from, to State, event Event) {
        events.Emit(StateChangeEvent{
            From:  from,
            To:    to,
            Event: event,
        })
    }).
    MustBuild()
```

Callbacks execute outside the machine lock. Use the callback parameters (`from`, `to`, `event`) instead of reading the machine state to avoid ordering assumptions.
Callback panics are recovered and reported via logs/metrics. For custom handling, configure an error handler on the builder.

### State Configuration Style

For complex state machines, use the configuration style:

```go
machine := statemachine.New[State, Event](StateIdle).
    Configure(StateIdle, func(cfg *statemachine.StateConfiguration[State, Event]) {
        cfg.Permit(EventStart, StateRunning).
            OnEnter(func(ctx context.Context, state, from State) {
                // Entry logic
            }).
            OnExit(func(ctx context.Context, state, to State) {
                // Exit logic
            })
    }).
    Configure(StateRunning, func(cfg *statemachine.StateConfiguration[State, Event]) {
        cfg.Permit(EventComplete, StateCompleted).
            Permit(EventFail, StateFailed).
            PermitReentry(EventRetry)  // Allows state to transition to itself
    }).
    MustBuild()
```

### Ignoring Events

Ignored events are treated as no-ops: no state change, callbacks, or history records.

```go
machine := statemachine.New[State, Event](StateIdle).
    Ignore(StateIdle, EventRetry).
    MustBuild()
```

### Checking State

```go
// Current state
state := machine.State()

// Check specific state
if machine.IsInState(StateRunning) {
    // ...
}

// Check multiple states
if machine.IsInAnyState(StateCompleted, StateFailed) {
    // Job is done
}

// Check if transition is possible
if machine.CanFire(EventStart) {
    err := machine.Fire(EventStart)
}

// Available events from current state
events := machine.AvailableEvents()
```

### Error Handling and Metrics

Use the error handler to capture transition errors and callback panics, and the metrics snapshot for visibility:

```go
machine := statemachine.New[State, Event](StateIdle).
    WithErrorHandler(func(ctx context.Context, err error) {
        // custom logging/metrics hooks
    }).
    MustBuild()

metrics := machine.Metrics()
```

Concurrent transitions return `ErrConcurrentTransition` so callers can decide whether to retry.

### History Tracking

Enable history for debugging and auditing:

```go
machine := statemachine.New[State, Event](StateIdle).
    WithHistory(100).  // Keep last 100 transitions
    // ... transitions ...
    MustBuild()

// Later: retrieve history
history := machine.History()

// Get all records
for _, record := range history.All() {
    fmt.Printf("%s: %s -> %s via %s (duration: %v)\n",
        record.Timestamp,
        record.From,
        record.To,
        record.Event,
        record.Duration)
}

// Filter by state or event
fromIdle := history.TransitionsFrom(StateIdle)
toFailed := history.TransitionsTo(StateFailed)
byRetry := history.TransitionsByEvent(EventRetry)

// Get latest
latest := history.Latest()
```

### Thread Safety

The state machine is fully thread-safe. All operations use proper locking:

```go
// Safe to call from multiple goroutines
go func() {
    machine.Fire(EventStart)
}()
go func() {
    state := machine.State()
    // ...
}()
```

## State Machine Composition

For complex systems, compose multiple state machines:

```go
type ConnectionManager struct {
    connection    *statemachine.Machine[ConnState, ConnEvent]
    circuitBreaker *statemachine.Machine[CBState, CBEvent]
}

func (m *ConnectionManager) Connect() error {
    // Check circuit breaker first
    if m.circuitBreaker.IsInState(CBStateOpen) {
        return ErrCircuitOpen
    }

    // Proceed with connection
    return m.connection.Fire(ConnEventConnect)
}
```

## Testing State Machines

### Test All Transitions

```go
func TestUpgradeMachine_Transitions(t *testing.T) {
    tests := []struct {
        name          string
        initialState  UpgradePhase
        event         UpgradeEvent
        expectedState UpgradePhase
        shouldFail    bool
    }{
        {"idle to pending", PhaseIdle, EventStart, PhasePending, false},
        {"pending to validating", PhasePending, EventValidate, PhaseValidating, false},
        {"invalid: idle to completed", PhaseIdle, EventComplete, PhaseIdle, true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            machine := buildUpgradeMachine(tt.initialState)

            err := machine.Fire(tt.event)

            if tt.shouldFail && err == nil {
                t.Error("expected error")
            }
            if !tt.shouldFail && err != nil {
                t.Errorf("unexpected error: %v", err)
            }
            if machine.State() != tt.expectedState {
                t.Errorf("got state %v, want %v", machine.State(), tt.expectedState)
            }
        })
    }
}
```

### Test Guards

```go
func TestUpgradeMachine_Guard(t *testing.T) {
    hasCapacity := true

    machine := statemachine.New[State, Event](StateIdle).
        AddTransition(StateIdle, EventStart, StateRunning).
        WithGuard(func(ctx context.Context, from State, event Event) bool {
            return hasCapacity
        }).
        MustBuild()

    // Should succeed with capacity
    if err := machine.Fire(EventStart); err != nil {
        t.Errorf("unexpected error: %v", err)
    }

    machine.Reset()
    hasCapacity = false

    // Should fail without capacity
    err := machine.Fire(EventStart)
    if !errors.Is(err, statemachine.ErrGuardFailed) {
        t.Error("expected ErrGuardFailed")
    }
}
```

### Test Callbacks

```go
func TestUpgradeMachine_Callbacks(t *testing.T) {
    var events []string

    machine := statemachine.New[State, Event](StateIdle).
        AddTransition(StateIdle, EventStart, StateRunning).
        OnExit(StateIdle, func(ctx context.Context, state, to State) {
            events = append(events, "exit:idle")
        }).
        OnEnter(StateRunning, func(ctx context.Context, state, from State) {
            events = append(events, "enter:running")
        }).
        OnTransition(func(ctx context.Context, from, to State, event Event) {
            events = append(events, fmt.Sprintf("transition:%s->%s", from, to))
        }).
        MustBuild()

    machine.Fire(EventStart)

    expected := []string{"exit:idle", "enter:running", "transition:idle->running"}
    if !reflect.DeepEqual(events, expected) {
        t.Errorf("got events %v, want %v", events, expected)
    }
}
```

### Test Concurrency

```go
func TestUpgradeMachine_Concurrency(t *testing.T) {
    machine := buildUpgradeMachine(StateIdle)

    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            // Try various transitions
            machine.Fire(EventStart)
            machine.Fire(EventComplete)
            machine.Fire(EventFail)
        }()
    }
    wg.Wait()

    // Machine should be in a valid final state
    state := machine.State()
    validFinalStates := map[State]bool{
        StateCompleted: true,
        StateFailed:    true,
        StateRunning:   true,
    }
    if !validFinalStates[state] {
        t.Errorf("machine in invalid state: %v", state)
    }
}
```

## Best Practices

### DO

1. **Define states and events as typed constants** - Prevents typos, enables IDE support
2. **Document the state diagram** - Use Mermaid diagrams in comments
3. **Use meaningful state names** - Reflect business concepts, not implementation
4. **Add guards for preconditions** - Validate before transitioning
5. **Use callbacks for side effects** - Logging, metrics, events
6. **Enable history in development** - Invaluable for debugging
7. **Test invalid transitions** - Verify they're rejected
8. **Keep state machines focused** - One machine per concern

### DON'T

1. **Don't put business logic in callbacks** - Keep them for side effects only
2. **Don't create states for every possible variation** - Keep it simple
3. **Don't share machines between unrelated concerns** - Compose instead
4. **Don't ignore transition errors** - Handle them explicitly
5. **Don't modify external state in guards** - Guards should be pure

## State Diagram Documentation

Document your state machine with a Mermaid diagram:

```go
// UpgradeMachine manages the lifecycle of an upgrade operation.
//
// State diagram:
//
// ```mermaid
// stateDiagram-v2
//     [*] --> Idle
//     Idle --> Pending: Start
//     Pending --> Validating: Validate
//     Validating --> Preparing: Prepare
//     Preparing --> Upgrading: Upgrade
//     Upgrading --> Verifying: Verify
//     Verifying --> Completed: Complete
//
//     Validating --> Failed: Fail
//     Preparing --> Failed: Fail
//     Upgrading --> Failed: Fail
//     Verifying --> Failed: Fail
//
//     Failed --> [*]
//     Completed --> [*]
// ```
func NewUpgradeMachine() *statemachine.Machine[UpgradePhase, UpgradeEvent] {
    // ...
}
```

## Migration Guide

When refactoring existing code to use state machines:

1. **Identify current states** - List all values of status/phase fields
2. **Map transitions** - Document what causes each state change
3. **Find guards** - Identify conditions that must be true for transitions
4. **Locate callbacks** - Find code that runs on state changes
5. **Build incrementally** - Start with core states, add edge cases
6. **Maintain compatibility** - Keep existing API, change internals
7. **Add tests first** - Capture current behavior before refactoring

## Reference

### Package: `pkg/statemachine`

| Type | Description |
|------|-------------|
| `Machine[S, E]` | Thread-safe state machine |
| `Builder[S, E]` | Fluent builder for machine construction |
| `History[S, E]` | Transition history tracker |
| `Guard[S, E]` | Condition function type |
| `StateCallback[S]` | Entry/exit callback type |
| `TransitionCallback[S, E]` | Transition callback type |

### Errors

| Error | Description |
|-------|-------------|
| `ErrInvalidTransition` | No transition defined for state+event |
| `ErrGuardFailed` | Guard condition returned false |
| `ErrMachineClosed` | Machine was closed |
| `ErrNoInitialState` | Builder has no initial state |

## Further Reading

- [Epic 39: State Machine Refactoring](/docs/community/roadmap/#epic-39-state-machine-refactoring)
- [pkg/statemachine GoDoc](https://pkg.go.dev/github.com/shawnbutts/keystone-core/pkg/statemachine)
