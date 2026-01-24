package statemachine

import (
	"context"
	"sync"
	"time"
)

// Guard is a function that determines whether a transition should be allowed.
// It receives the context and returns true if the transition should proceed.
type Guard[S, E comparable] func(ctx context.Context, from S, event E) bool

// StateCallback is called when entering or exiting a state.
type StateCallback[S comparable] func(ctx context.Context, state S, other S)

// TransitionCallback is called after a successful transition.
type TransitionCallback[S, E comparable] func(ctx context.Context, from, to S, event E)

// transition represents a single state transition rule.
type transition[S, E comparable] struct {
	from   S
	event  E
	to     S
	guards []Guard[S, E]
}

// transitionKey uniquely identifies a transition by its source state and event.
type transitionKey[S, E comparable] struct {
	from  S
	event E
}

// Machine is a generic, thread-safe state machine.
type Machine[S, E comparable] struct {
	mu sync.RWMutex

	// Current state of the machine.
	state S

	// Time when the machine entered the current state.
	stateEnteredAt time.Time

	// Initial state (for reset).
	initialState S

	// Transition rules indexed by (from state, event).
	transitions map[transitionKey[S, E]]*transition[S, E]

	// Callbacks for state entry.
	onEnter map[S][]StateCallback[S]

	// Callbacks for state exit.
	onExit map[S][]StateCallback[S]

	// Callbacks for any transition.
	onTransition []TransitionCallback[S, E]

	// History tracking.
	history *History[S, E]

	// Whether the machine is closed.
	closed bool

	// Optional name for debugging.
	name string
}

// State returns the current state of the machine.
func (m *Machine[S, E]) State() S {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

// StateEnteredAt returns when the machine entered its current state.
func (m *Machine[S, E]) StateEnteredAt() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.stateEnteredAt
}

// StateDuration returns how long the machine has been in its current state.
func (m *Machine[S, E]) StateDuration() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return time.Since(m.stateEnteredAt)
}

// Name returns the machine's name.
func (m *Machine[S, E]) Name() string {
	return m.name
}

// CanFire returns true if the given event can trigger a transition from the current state.
func (m *Machine[S, E]) CanFire(event E) bool {
	return m.CanFireCtx(context.Background(), event)
}

// CanFireCtx returns true if the given event can trigger a transition from the current state,
// evaluating guard conditions with the provided context.
func (m *Machine[S, E]) CanFireCtx(ctx context.Context, event E) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return false
	}

	key := transitionKey[S, E]{from: m.state, event: event}
	t, exists := m.transitions[key]
	if !exists {
		return false
	}

	// Check guards
	for _, guard := range t.guards {
		if !guard(ctx, m.state, event) {
			return false
		}
	}

	return true
}

// Fire triggers a state transition using the given event.
// Returns an error if the transition is not allowed.
func (m *Machine[S, E]) Fire(event E) error {
	return m.FireCtx(context.Background(), event)
}

// FireCtx triggers a state transition using the given event and context.
// The context is passed to guard conditions and callbacks.
// Returns an error if the transition is not allowed.
func (m *Machine[S, E]) FireCtx(ctx context.Context, event E) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return NewTransitionError(m.state, event, ErrMachineClosed, "")
	}

	key := transitionKey[S, E]{from: m.state, event: event}
	t, exists := m.transitions[key]
	if !exists {
		return NewTransitionError(m.state, event, ErrInvalidTransition,
			"no transition defined")
	}

	// Check guards
	for _, guard := range t.guards {
		if !guard(ctx, m.state, event) {
			return NewTransitionError(m.state, event, ErrGuardFailed,
				"guard condition blocked transition")
		}
	}

	// Execute transition
	fromState := m.state
	toState := t.to
	duration := time.Since(m.stateEnteredAt)

	// Execute onExit callbacks for the current state
	if callbacks, ok := m.onExit[fromState]; ok {
		for _, cb := range callbacks {
			cb(ctx, fromState, toState)
		}
	}

	// Update state
	m.state = toState
	m.stateEnteredAt = time.Now()

	// Execute onEnter callbacks for the new state
	if callbacks, ok := m.onEnter[toState]; ok {
		for _, cb := range callbacks {
			cb(ctx, toState, fromState)
		}
	}

	// Execute onTransition callbacks
	for _, cb := range m.onTransition {
		cb(ctx, fromState, toState, event)
	}

	// Record in history
	if m.history != nil {
		m.history.Record(fromState, toState, event, duration, nil)
	}

	return nil
}

// FireWithMetadata triggers a transition and records metadata in history.
func (m *Machine[S, E]) FireWithMetadata(ctx context.Context, event E, metadata map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return NewTransitionError(m.state, event, ErrMachineClosed, "")
	}

	key := transitionKey[S, E]{from: m.state, event: event}
	t, exists := m.transitions[key]
	if !exists {
		return NewTransitionError(m.state, event, ErrInvalidTransition,
			"no transition defined")
	}

	// Check guards
	for _, guard := range t.guards {
		if !guard(ctx, m.state, event) {
			return NewTransitionError(m.state, event, ErrGuardFailed,
				"guard condition blocked transition")
		}
	}

	// Execute transition
	fromState := m.state
	toState := t.to
	duration := time.Since(m.stateEnteredAt)

	// Execute onExit callbacks
	if callbacks, ok := m.onExit[fromState]; ok {
		for _, cb := range callbacks {
			cb(ctx, fromState, toState)
		}
	}

	// Update state
	m.state = toState
	m.stateEnteredAt = time.Now()

	// Execute onEnter callbacks
	if callbacks, ok := m.onEnter[toState]; ok {
		for _, cb := range callbacks {
			cb(ctx, toState, fromState)
		}
	}

	// Execute onTransition callbacks
	for _, cb := range m.onTransition {
		cb(ctx, fromState, toState, event)
	}

	// Record in history with metadata
	if m.history != nil {
		m.history.Record(fromState, toState, event, duration, metadata)
	}

	return nil
}

// Reset returns the machine to its initial state without triggering callbacks.
func (m *Machine[S, E]) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.state = m.initialState
	m.stateEnteredAt = time.Now()
}

// ResetWithCallbacks returns the machine to its initial state, triggering exit
// and entry callbacks as if it were a normal transition.
func (m *Machine[S, E]) ResetWithCallbacks(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.state == m.initialState {
		return
	}

	fromState := m.state
	toState := m.initialState

	// Execute onExit callbacks
	if callbacks, ok := m.onExit[fromState]; ok {
		for _, cb := range callbacks {
			cb(ctx, fromState, toState)
		}
	}

	m.state = toState
	m.stateEnteredAt = time.Now()

	// Execute onEnter callbacks
	if callbacks, ok := m.onEnter[toState]; ok {
		for _, cb := range callbacks {
			cb(ctx, toState, fromState)
		}
	}
}

// Close shuts down the machine, preventing further transitions.
func (m *Machine[S, E]) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
}

// IsClosed returns true if the machine has been closed.
func (m *Machine[S, E]) IsClosed() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.closed
}

// History returns the transition history, or nil if history is disabled.
func (m *Machine[S, E]) History() *History[S, E] {
	return m.history
}

// AvailableEvents returns the events that can be fired from the current state.
// This does not evaluate guard conditions.
func (m *Machine[S, E]) AvailableEvents() []E {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var events []E
	for key := range m.transitions {
		if key.from == m.state {
			events = append(events, key.event)
		}
	}
	return events
}

// AvailableEventsWithGuards returns events that can be fired from the current state,
// evaluating guard conditions with the provided context.
func (m *Machine[S, E]) AvailableEventsWithGuards(ctx context.Context) []E {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var events []E
	for key, t := range m.transitions {
		if key.from != m.state {
			continue
		}

		// Check guards
		allowed := true
		for _, guard := range t.guards {
			if !guard(ctx, m.state, key.event) {
				allowed = false
				break
			}
		}

		if allowed {
			events = append(events, key.event)
		}
	}
	return events
}

// AllStates returns all states that have transitions defined.
func (m *Machine[S, E]) AllStates() []S {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stateSet := make(map[S]struct{})
	stateSet[m.initialState] = struct{}{}

	for key, t := range m.transitions {
		stateSet[key.from] = struct{}{}
		stateSet[t.to] = struct{}{}
	}

	states := make([]S, 0, len(stateSet))
	for s := range stateSet {
		states = append(states, s)
	}
	return states
}

// AllEvents returns all events that have transitions defined.
func (m *Machine[S, E]) AllEvents() []E {
	m.mu.RLock()
	defer m.mu.RUnlock()

	eventSet := make(map[E]struct{})
	for key := range m.transitions {
		eventSet[key.event] = struct{}{}
	}

	events := make([]E, 0, len(eventSet))
	for e := range eventSet {
		events = append(events, e)
	}
	return events
}

// TransitionsFrom returns the possible transitions from the given state.
func (m *Machine[S, E]) TransitionsFrom(state S) map[E]S {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[E]S)
	for key, t := range m.transitions {
		if key.from == state {
			result[key.event] = t.to
		}
	}
	return result
}

// IsInState returns true if the machine is currently in the given state.
func (m *Machine[S, E]) IsInState(state S) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state == state
}

// IsInAnyState returns true if the machine is in any of the given states.
func (m *Machine[S, E]) IsInAnyState(states ...S) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, s := range states {
		if m.state == s {
			return true
		}
	}
	return false
}
