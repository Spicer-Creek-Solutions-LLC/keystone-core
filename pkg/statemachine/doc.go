// Package statemachine provides a generic, type-safe state machine implementation
// NOTE: API/ABI is not finalized and may change without notice.
// for managing complex state transitions in a predictable, testable way.
//
// # Overview
//
// State machines provide explicit control over state transitions, replacing
// scattered conditionals and implicit state tracking with a formal model that
// documents valid states and transitions in code.
//
// # Key Features
//
//   - Generic type parameters for states and events
//   - Validated transitions that prevent invalid state changes
//   - Guard conditions for conditional transitions
//   - OnEnter/OnExit callbacks for state lifecycle
//   - OnTransition callbacks for logging and event emission
//   - Thread-safe operation with proper locking
//   - Transition history for debugging and auditing
//   - Serializable state for persistence
//
// # Basic Usage
//
// Define your state and event types, then build a machine:
//
//	type ConnectionState string
//	const (
//	    StateDisconnected ConnectionState = "disconnected"
//	    StateConnecting   ConnectionState = "connecting"
//	    StateConnected    ConnectionState = "connected"
//	)
//
//	type ConnectionEvent string
//	const (
//	    EventConnect    ConnectionEvent = "connect"
//	    EventConnected  ConnectionEvent = "connected"
//	    EventDisconnect ConnectionEvent = "disconnect"
//	)
//
//	machine := statemachine.New[ConnectionState, ConnectionEvent](StateDisconnected).
//	    AddTransition(StateDisconnected, EventConnect, StateConnecting).
//	    AddTransition(StateConnecting, EventConnected, StateConnected).
//	    AddTransition(StateConnected, EventDisconnect, StateDisconnected).
//	    Build()
//
//	// Trigger a transition
//	if err := machine.Fire(EventConnect); err != nil {
//	    // Handle invalid transition
//	}
//
// # Guards
//
// Guards are conditions that must be true for a transition to occur:
//
//	machine := statemachine.New[State, Event](Initial).
//	    AddTransition(Ready, EventStart, Running).
//	    WithGuard(func(ctx context.Context) bool {
//	        return resourcesAvailable(ctx)
//	    }).
//	    Build()
//
// # Callbacks
//
// Callbacks execute code on state entry, exit, or transition:
//
//	machine := statemachine.New[State, Event](Initial).
//	    OnEnter(Running, func(ctx context.Context, from State) {
//	        log.Info("Entering running state")
//	    }).
//	    OnExit(Running, func(ctx context.Context, to State) {
//	        log.Info("Exiting running state")
//	    }).
//	    OnTransition(func(ctx context.Context, from, to State, event Event) {
//	        metrics.RecordTransition(from, to, event)
//	    }).
//	    Build()
//
// Callback panics are recovered and reported via logs/metrics. Use the builder
// error handler to route errors to your own logging or telemetry.
//
// # History
//
// Enable history tracking for debugging:
//
//	machine := statemachine.New[State, Event](Initial).
//	    WithHistory(100). // Keep last 100 transitions
//	    Build()
//
//	history := machine.History()
//	for _, record := range history {
//	    fmt.Printf("%s: %s -> %s via %s\n",
//	        record.Timestamp, record.From, record.To, record.Event)
//	}
//
// # Thread Safety
//
// All operations on a Machine are thread-safe. The machine uses an internal
// read-write mutex to protect state access and transitions.
//
// Concurrent transitions return ErrConcurrentTransition, allowing callers to
// retry or ignore when needed.
//
// # Best Practices
//
//  1. Define state and event types as named types (not raw strings/ints)
//  2. Use descriptive names that reflect business concepts
//  3. Document the state diagram in comments or diagrams
//  4. Add guards for transitions that require preconditions
//  5. Use callbacks for side effects, not business logic
//  6. Enable history in development for debugging
//  7. Write tests that verify all valid transitions
//  8. Write tests that verify invalid transitions are rejected
package statemachine
