package statemachine

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
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

	// Ignored events that should be treated as no-ops.
	ignored map[transitionKey[S, E]]struct{}

	// Logger for error and panic reporting.
	logger Logger

	// Error handler for transition errors and callback panics.
	errorHandler ErrorHandler

	// Metrics tracking.
	metrics *MachineMetrics

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

// Metrics returns a snapshot of machine metrics.
func (m *Machine[S, E]) Metrics() MachineMetricsSnapshot {
	if m == nil || m.metrics == nil {
		return MachineMetricsSnapshot{}
	}
	return m.metrics.Snapshot()
}

// CanFire returns true if the given event can trigger a transition from the current state.
func (m *Machine[S, E]) CanFire(event E) bool {
	return m.CanFireCtx(context.Background(), event)
}

// CanFireCtx returns true if the given event can trigger a transition from the current state,
// evaluating guard conditions with the provided context.
func (m *Machine[S, E]) CanFireCtx(ctx context.Context, event E) bool {
	var (
		guards []Guard[S, E]
		state  S
	)

	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return false
	}

	state = m.state
	key := transitionKey[S, E]{from: state, event: event}
	if _, ignored := m.ignored[key]; ignored {
		m.mu.RUnlock()
		return true
	}
	t, exists := m.transitions[key]
	if !exists {
		m.mu.RUnlock()
		return false
	}
	guards = append(guards, t.guards...)
	m.mu.RUnlock()

	// Check guards
	for _, guard := range guards {
		if !guard(ctx, state, event) {
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
	return m.fire(ctx, event, nil)
}

// FireWithMetadata triggers a transition and records metadata in history.
func (m *Machine[S, E]) FireWithMetadata(ctx context.Context, event E, metadata map[string]any) error {
	return m.fire(ctx, event, metadata)
}

func (m *Machine[S, E]) fire(ctx context.Context, event E, metadata map[string]any) error {
	var (
		fromState      S
		toState        S
		onExit         []StateCallback[S]
		onEnter        []StateCallback[S]
		onTransition   []TransitionCallback[S, E]
		guards         []Guard[S, E]
		history        *History[S, E]
		stateEnteredAt time.Time
	)

	m.mu.Lock()
	if m.closed {
		currentState := m.state
		m.mu.Unlock()
		err := NewTransitionError(currentState, event, ErrMachineClosed, "")
		m.recordTransitionError(ctx, err)
		return err
	}

	fromState = m.state
	key := transitionKey[S, E]{from: fromState, event: event}
	if _, ignored := m.ignored[key]; ignored {
		m.mu.Unlock()
		return nil
	}

	t, exists := m.transitions[key]
	if !exists {
		m.mu.Unlock()
		err := NewTransitionError(fromState, event, ErrInvalidTransition,
			"no transition defined")
		m.recordTransitionError(ctx, err)
		return err
	}

	toState = t.to
	guards = append(guards, t.guards...)
	onExit = append(onExit, m.onExit[fromState]...)
	onEnter = append(onEnter, m.onEnter[toState]...)
	onTransition = append(onTransition, m.onTransition...)
	history = m.history
	stateEnteredAt = m.stateEnteredAt
	m.mu.Unlock()

	// Check guards
	for _, guard := range guards {
		if !guard(ctx, fromState, event) {
			err := NewTransitionError(fromState, event, ErrGuardFailed,
				"guard condition blocked transition")
			m.recordTransitionError(ctx, err)
			return err
		}
	}

	m.mu.Lock()
	if m.closed {
		currentState := m.state
		m.mu.Unlock()
		err := NewTransitionError(currentState, event, ErrMachineClosed, "")
		m.recordTransitionError(ctx, err)
		return err
	}
	if m.state != fromState {
		currentState := m.state
		m.mu.Unlock()
		err := NewTransitionError(currentState, event, ErrConcurrentTransition,
			"state changed during transition")
		m.recordTransitionError(ctx, err)
		return err
	}

	// Execute transition
	m.state = toState
	m.stateEnteredAt = time.Now()

	if history != nil {
		duration := time.Since(stateEnteredAt)
		history.Record(fromState, toState, event, duration, metadata)
	}
	m.mu.Unlock()

	// Execute onExit callbacks for the previous state
	for _, cb := range onExit {
		m.safeStateCallback(ctx, "exit", fromState, toState, cb)
	}

	// Execute onEnter callbacks for the new state
	for _, cb := range onEnter {
		m.safeStateCallback(ctx, "enter", toState, fromState, cb)
	}

	// Execute onTransition callbacks
	for _, cb := range onTransition {
		m.safeTransitionCallback(ctx, fromState, toState, event, cb)
	}

	return nil
}

func (m *Machine[S, E]) safeStateCallback(ctx context.Context, phase string, state S, other S, cb StateCallback[S]) {
	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("state callback panic: %v", r)
			m.recordCallbackPanic(ctx, err, phase, state, other, nil)
		}
	}()
	cb(ctx, state, other)
}

func (m *Machine[S, E]) safeTransitionCallback(ctx context.Context, from S, to S, event E, cb TransitionCallback[S, E]) {
	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("transition callback panic: %v", r)
			m.recordCallbackPanic(ctx, err, "transition", from, to, &event)
		}
	}()
	cb(ctx, from, to, event)
}

func (m *Machine[S, E]) recordTransitionError(ctx context.Context, err error) {
	if m.metrics != nil {
		switch {
		case errors.Is(err, ErrGuardFailed):
			m.metrics.recordGuardFailure()
		case errors.Is(err, ErrMachineClosed):
			m.metrics.recordMachineClosed()
		case errors.Is(err, ErrConcurrentTransition):
			m.metrics.recordConcurrentTransition()
		case errors.Is(err, ErrInvalidTransition):
			m.metrics.recordInvalidTransition()
		default:
			m.metrics.transitionErrors.Add(1)
		}
	}
	if m.logger != nil {
		fields := []Field{
			{Key: "machine", Value: m.name},
			{Key: "error", Value: err},
		}
		if transErr, ok := err.(*TransitionError); ok {
			fields = append(fields,
				Field{Key: "from", Value: transErr.From},
				Field{Key: "event", Value: transErr.Event},
				Field{Key: "reason", Value: transErr.Reason},
			)
		}
		m.logger.WithContext(ctx).Warn("state machine transition error", fields...)
	}
	if m.errorHandler != nil {
		m.errorHandler(ctx, err)
	}
}

func (m *Machine[S, E]) recordCallbackPanic(ctx context.Context, err error, phase string, state S, other S, event *E) {
	if m.metrics != nil {
		m.metrics.recordCallbackPanic()
	}
	if m.logger != nil {
		fields := []Field{
			{Key: "machine", Value: m.name},
			{Key: "phase", Value: phase},
			{Key: "state", Value: state},
			{Key: "other_state", Value: other},
			{Key: "error", Value: err},
			{Key: "stack", Value: string(debug.Stack())},
		}
		if event != nil {
			fields = append(fields, Field{Key: "event", Value: *event})
		}
		m.logger.WithContext(ctx).Error("state machine callback panic", fields...)
	}
	if m.errorHandler != nil {
		m.errorHandler(ctx, err)
	}
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
	for key := range m.ignored {
		if key.from == m.state {
			events = append(events, key.event)
		}
	}
	return events
}

// AvailableEventsWithGuards returns events that can be fired from the current state,
// evaluating guard conditions with the provided context.
func (m *Machine[S, E]) AvailableEventsWithGuards(ctx context.Context) []E {
	var (
		state       S
		transitions []transition[S, E]
		events      []E
	)

	m.mu.RLock()
	state = m.state
	for key, t := range m.transitions {
		if key.from != state {
			continue
		}
		transitions = append(transitions, *t)
	}
	for key := range m.ignored {
		if key.from == state {
			events = append(events, key.event)
		}
	}
	m.mu.RUnlock()

	for _, t := range transitions {
		allowed := true
		for _, guard := range t.guards {
			if !guard(ctx, state, t.event) {
				allowed = false
				break
			}
		}

		if allowed {
			events = append(events, t.event)
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
	for key := range m.ignored {
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
	for key := range m.ignored {
		if key.from == state {
			result[key.event] = state
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
