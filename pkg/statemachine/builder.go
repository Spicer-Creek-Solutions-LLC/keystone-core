package statemachine

import (
	"context"
	"time"

	"github.com/shawnbutts/keystone-core/internal/logging"
)

// Builder provides a fluent interface for constructing state machines.
type Builder[S, E comparable] struct {
	initialState S
	hasInitial   bool
	transitions  map[transitionKey[S, E]]*transition[S, E]
	ignored      map[transitionKey[S, E]]struct{}
	onEnter      map[S][]StateCallback[S]
	onExit       map[S][]StateCallback[S]
	onTransition []TransitionCallback[S, E]
	historySize  int
	name         string
	logger       Logger
	errorHandler ErrorHandler

	// For chaining guard additions
	lastTransitionKey *transitionKey[S, E]
}

// New creates a new state machine builder with the given initial state.
func New[S, E comparable](initialState S) *Builder[S, E] {
	return &Builder[S, E]{
		initialState: initialState,
		hasInitial:   true,
		transitions:  make(map[transitionKey[S, E]]*transition[S, E]),
		ignored:      make(map[transitionKey[S, E]]struct{}),
		onEnter:      make(map[S][]StateCallback[S]),
		onExit:       make(map[S][]StateCallback[S]),
	}
}

// WithName sets a name for the machine (useful for debugging).
func (b *Builder[S, E]) WithName(name string) *Builder[S, E] {
	b.name = name
	return b
}

// WithHistory enables transition history tracking with the given maximum size.
func (b *Builder[S, E]) WithHistory(maxSize int) *Builder[S, E] {
	b.historySize = maxSize
	return b
}

// WithLogger sets a logger for error and panic reporting.
func (b *Builder[S, E]) WithLogger(logger Logger) *Builder[S, E] {
	b.logger = logger
	return b
}

// WithErrorHandler sets a handler for transition errors and callback panics.
func (b *Builder[S, E]) WithErrorHandler(handler ErrorHandler) *Builder[S, E] {
	b.errorHandler = handler
	return b
}

// AddTransition defines a transition from one state to another via an event.
func (b *Builder[S, E]) AddTransition(from S, event E, to S) *Builder[S, E] {
	key := transitionKey[S, E]{from: from, event: event}
	delete(b.ignored, key)
	b.transitions[key] = &transition[S, E]{
		from:  from,
		event: event,
		to:    to,
	}
	b.lastTransitionKey = &key
	return b
}

// WithGuard adds a guard condition to the most recently defined transition.
// The guard must return true for the transition to be allowed.
func (b *Builder[S, E]) WithGuard(guard Guard[S, E]) *Builder[S, E] {
	if b.lastTransitionKey == nil {
		return b
	}
	if t, ok := b.transitions[*b.lastTransitionKey]; ok {
		t.guards = append(t.guards, guard)
	}
	return b
}

// WithGuardFunc adds a simple guard function that doesn't use the context.
func (b *Builder[S, E]) WithGuardFunc(guard func(from S, event E) bool) *Builder[S, E] {
	return b.WithGuard(func(_ context.Context, from S, event E) bool {
		return guard(from, event)
	})
}

// OnEnter registers a callback to be executed when entering the given state.
func (b *Builder[S, E]) OnEnter(state S, callback StateCallback[S]) *Builder[S, E] {
	b.onEnter[state] = append(b.onEnter[state], callback)
	return b
}

// OnEnterAny registers a callback to be executed when entering any of the given states.
func (b *Builder[S, E]) OnEnterAny(states []S, callback StateCallback[S]) *Builder[S, E] {
	for _, state := range states {
		b.onEnter[state] = append(b.onEnter[state], callback)
	}
	return b
}

// OnExit registers a callback to be executed when exiting the given state.
func (b *Builder[S, E]) OnExit(state S, callback StateCallback[S]) *Builder[S, E] {
	b.onExit[state] = append(b.onExit[state], callback)
	return b
}

// OnExitAny registers a callback to be executed when exiting any of the given states.
func (b *Builder[S, E]) OnExitAny(states []S, callback StateCallback[S]) *Builder[S, E] {
	for _, state := range states {
		b.onExit[state] = append(b.onExit[state], callback)
	}
	return b
}

// OnTransition registers a callback to be executed after any successful transition.
func (b *Builder[S, E]) OnTransition(callback TransitionCallback[S, E]) *Builder[S, E] {
	b.onTransition = append(b.onTransition, callback)
	return b
}

// Permit is an alias for AddTransition, providing Salt/Stateless-style syntax.
func (b *Builder[S, E]) Permit(event E, from S, to S) *Builder[S, E] {
	return b.AddTransition(from, event, to)
}

// PermitIf adds a transition with a guard condition.
func (b *Builder[S, E]) PermitIf(event E, from S, to S, guard Guard[S, E]) *Builder[S, E] {
	return b.AddTransition(from, event, to).WithGuard(guard)
}

// PermitReentry allows a transition from a state back to itself.
func (b *Builder[S, E]) PermitReentry(state S, event E) *Builder[S, E] {
	return b.AddTransition(state, event, state)
}

// Ignore defines that an event should be ignored (no-op) in a given state.
// This is different from not having a transition: ignored events don't return errors.
func (b *Builder[S, E]) Ignore(state S, event E) *Builder[S, E] {
	key := transitionKey[S, E]{from: state, event: event}
	delete(b.transitions, key)
	b.ignored[key] = struct{}{}
	b.lastTransitionKey = nil
	return b
}

// Build creates the state machine from the builder configuration.
// Returns an error if the configuration is invalid.
func (b *Builder[S, E]) Build() (*Machine[S, E], error) {
	if !b.hasInitial {
		return nil, NewBuildError(ErrNoInitialState, "")
	}

	machine := &Machine[S, E]{
		state:          b.initialState,
		initialState:   b.initialState,
		stateEnteredAt: time.Now(),
		transitions:    make(map[transitionKey[S, E]]*transition[S, E]),
		onEnter:        make(map[S][]StateCallback[S]),
		onExit:         make(map[S][]StateCallback[S]),
		ignored:        make(map[transitionKey[S, E]]struct{}),
		name:           b.name,
		logger:         b.logger,
		errorHandler:   b.errorHandler,
		metrics:        &MachineMetrics{},
	}
	if machine.logger == nil {
		fields := []logging.Field{
			{Key: "component", Value: "statemachine"},
		}
		if machine.name != "" {
			fields = append(fields, logging.Field{Key: "machine", Value: machine.name})
		}
		machine.logger = logging.WithFields(fields...)
	}

	// Copy transitions
	for k, v := range b.transitions {
		// Deep copy the transition
		t := &transition[S, E]{
			from:   v.from,
			event:  v.event,
			to:     v.to,
			guards: make([]Guard[S, E], len(v.guards)),
		}
		copy(t.guards, v.guards)
		machine.transitions[k] = t
	}
	for k := range b.ignored {
		machine.ignored[k] = struct{}{}
	}

	// Copy callbacks
	for state, callbacks := range b.onEnter {
		machine.onEnter[state] = make([]StateCallback[S], len(callbacks))
		copy(machine.onEnter[state], callbacks)
	}
	for state, callbacks := range b.onExit {
		machine.onExit[state] = make([]StateCallback[S], len(callbacks))
		copy(machine.onExit[state], callbacks)
	}
	machine.onTransition = make([]TransitionCallback[S, E], len(b.onTransition))
	copy(machine.onTransition, b.onTransition)

	// Create history if requested
	if b.historySize > 0 {
		machine.history = NewHistory[S, E](b.historySize)
	}

	return machine, nil
}

// MustBuild creates the state machine or panics if configuration is invalid.
// Use this for compile-time defined state machines where errors indicate bugs.
func (b *Builder[S, E]) MustBuild() *Machine[S, E] {
	m, err := b.Build()
	if err != nil {
		panic(err)
	}
	return m
}

// Configure provides a callback-based approach to configuration.
func (b *Builder[S, E]) Configure(state S, configurator func(*StateConfiguration[S, E])) *Builder[S, E] {
	cfg := &StateConfiguration[S, E]{
		builder: b,
		state:   state,
	}
	configurator(cfg)
	return b
}

// StateConfiguration allows configuring a specific state.
type StateConfiguration[S, E comparable] struct {
	builder *Builder[S, E]
	state   S
}

// Permit adds a transition from this state.
func (c *StateConfiguration[S, E]) Permit(event E, to S) *StateConfiguration[S, E] {
	c.builder.AddTransition(c.state, event, to)
	return c
}

// PermitIf adds a guarded transition from this state.
func (c *StateConfiguration[S, E]) PermitIf(event E, to S, guard Guard[S, E]) *StateConfiguration[S, E] {
	c.builder.AddTransition(c.state, event, to).WithGuard(guard)
	return c
}

// PermitReentry allows this state to transition to itself.
func (c *StateConfiguration[S, E]) PermitReentry(event E) *StateConfiguration[S, E] {
	c.builder.AddTransition(c.state, event, c.state)
	return c
}

// Ignore specifies that an event should be ignored in this state.
func (c *StateConfiguration[S, E]) Ignore(event E) *StateConfiguration[S, E] {
	c.builder.Ignore(c.state, event)
	return c
}

// OnEnter registers an entry callback for this state.
func (c *StateConfiguration[S, E]) OnEnter(callback StateCallback[S]) *StateConfiguration[S, E] {
	c.builder.OnEnter(c.state, callback)
	return c
}

// OnExit registers an exit callback for this state.
func (c *StateConfiguration[S, E]) OnExit(callback StateCallback[S]) *StateConfiguration[S, E] {
	c.builder.OnExit(c.state, callback)
	return c
}
