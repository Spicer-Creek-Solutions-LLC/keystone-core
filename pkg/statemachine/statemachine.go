// SPDX-License-Identifier: Apache-2.0

package statemachine

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Sentinel errors. Build-time failures come from [Builder.Build];
// the rest are returned by [Machine.Fire] / [Machine.RestoreFrom].
var (
	ErrNoInitialState      = errors.New("statemachine: no initial state defined")
	ErrDuplicateTransition = errors.New("statemachine: duplicate transition for (state,event)")
	ErrUnknownEvent        = errors.New("statemachine: event not defined in any transition")
	ErrNoTransition        = errors.New("statemachine: no transition from current state for event")
	ErrGuardRejected       = errors.New("statemachine: guard rejected transition")
	ErrUnknownState        = errors.New("statemachine: state not known to machine")
	ErrNoCheckpointer      = errors.New("statemachine: no checkpointer configured")
)

// Transition is the value handed to [Guard]s and callbacks describing
// the move under evaluation.
type Transition[S comparable, E comparable] struct {
	From  S
	To    S
	Event E
}

// Guard gates a transition. A non-nil return rejects it; the machine
// state does not change and [Machine.Fire] returns an error wrapping
// [ErrGuardRejected].
type Guard[S comparable, E comparable] func(ctx context.Context, t Transition[S, E]) error

// Callback runs for side effects around an accepted transition. A
// non-nil return is surfaced (joined) by [Machine.Fire] but does not
// roll the transition back — see the package doc.
type Callback[S comparable, E comparable] func(ctx context.Context, t Transition[S, E]) error

// Record is one entry in the transition history.
type Record[S comparable, E comparable] struct {
	From  S
	To    S
	Event E
	At    time.Time
}

// MetricsSnapshot is an immutable copy of a machine's counters.
type MetricsSnapshot struct {
	Transitions     uint64 // accepted transitions that completed
	GuardRejections uint64 // transitions vetoed by a guard
	NoTransition    uint64 // Fire with a known event but no edge from current
	UnknownEvent    uint64 // Fire with an event in no transition at all
}

type transitionKey[S comparable, E comparable] struct {
	from  S
	event E
}

type transitionDef[S comparable, E comparable] struct {
	to           S
	guards       []Guard[S, E]
	onTransition []Callback[S, E]
}

// TransitionOption customises a single transition declared via
// [Builder.Transition].
type TransitionOption[S comparable, E comparable] func(*transitionDef[S, E])

// WithGuard attaches a guard to the transition. Multiple guards are
// evaluated in registration order; the first error vetoes.
func WithGuard[S comparable, E comparable](g Guard[S, E]) TransitionOption[S, E] {
	return func(d *transitionDef[S, E]) { d.guards = append(d.guards, g) }
}

// WithOnTransition attaches a callback fired only for this specific
// transition (after the machine-global OnTransition callbacks).
func WithOnTransition[S comparable, E comparable](cb Callback[S, E]) TransitionOption[S, E] {
	return func(d *transitionDef[S, E]) { d.onTransition = append(d.onTransition, cb) }
}

// Builder accumulates a machine definition. It is not safe for
// concurrent use; build once, then share the resulting [Machine].
type Builder[S comparable, E comparable] struct {
	initial      S
	hasInitial   bool
	states       map[S]struct{}
	events       map[E]struct{}
	transitions  map[transitionKey[S, E]]*transitionDef[S, E]
	onEnter      map[S][]Callback[S, E]
	onExit       map[S][]Callback[S, E]
	onTransition []Callback[S, E]
	clock        func() time.Time
	cp           Checkpointer[S, E]
	err          error
}

// NewBuilder returns an empty [Builder].
func NewBuilder[S comparable, E comparable]() *Builder[S, E] {
	return &Builder[S, E]{
		states:      make(map[S]struct{}),
		events:      make(map[E]struct{}),
		transitions: make(map[transitionKey[S, E]]*transitionDef[S, E]),
		onEnter:     make(map[S][]Callback[S, E]),
		onExit:      make(map[S][]Callback[S, E]),
	}
}

// Initial sets the starting state (and registers it).
func (b *Builder[S, E]) Initial(s S) *Builder[S, E] {
	b.initial = s
	b.hasInitial = true
	b.states[s] = struct{}{}
	return b
}

// State explicitly registers states. Only needed for terminal states
// that never appear as a transition endpoint; transition endpoints and
// the initial state are registered implicitly.
func (b *Builder[S, E]) State(ss ...S) *Builder[S, E] {
	for _, s := range ss {
		b.states[s] = struct{}{}
	}
	return b
}

// Transition declares a `from --event--> to` edge. Declaring the same
// (from, event) pair twice fails [Builder.Build] with
// [ErrDuplicateTransition].
func (b *Builder[S, E]) Transition(from S, event E, to S, opts ...TransitionOption[S, E]) *Builder[S, E] {
	b.states[from] = struct{}{}
	b.states[to] = struct{}{}
	b.events[event] = struct{}{}
	key := transitionKey[S, E]{from: from, event: event}
	if _, dup := b.transitions[key]; dup && b.err == nil {
		b.err = fmt.Errorf("%w: from=%v event=%v", ErrDuplicateTransition, from, event)
		return b
	}
	d := &transitionDef[S, E]{to: to}
	for _, opt := range opts {
		opt(d)
	}
	b.transitions[key] = d
	return b
}

// OnEnter registers a callback run after the machine enters state s.
func (b *Builder[S, E]) OnEnter(s S, cb Callback[S, E]) *Builder[S, E] {
	b.states[s] = struct{}{}
	b.onEnter[s] = append(b.onEnter[s], cb)
	return b
}

// OnExit registers a callback run before the machine leaves state s.
func (b *Builder[S, E]) OnExit(s S, cb Callback[S, E]) *Builder[S, E] {
	b.states[s] = struct{}{}
	b.onExit[s] = append(b.onExit[s], cb)
	return b
}

// OnTransition registers a callback run on every accepted transition,
// before any per-transition [WithOnTransition] callback.
func (b *Builder[S, E]) OnTransition(cb Callback[S, E]) *Builder[S, E] {
	b.onTransition = append(b.onTransition, cb)
	return b
}

// Clock injects the time source used for [Record.At]. nil (the
// default) uses [time.Now]; tests inject a deterministic clock.
func (b *Builder[S, E]) Clock(fn func() time.Time) *Builder[S, E] {
	b.clock = fn
	return b
}

// Checkpointer attaches an optional snapshot/restore seam.
func (b *Builder[S, E]) Checkpointer(cp Checkpointer[S, E]) *Builder[S, E] {
	b.cp = cp
	return b
}

// Build validates the definition and returns a ready [Machine].
func (b *Builder[S, E]) Build() (*Machine[S, E], error) {
	if b.err != nil {
		return nil, b.err
	}
	if !b.hasInitial {
		return nil, ErrNoInitialState
	}
	return &Machine[S, E]{
		current:      b.initial,
		states:       b.states,
		events:       b.events,
		transitions:  b.transitions,
		onEnter:      b.onEnter,
		onExit:       b.onExit,
		onTransition: b.onTransition,
		clock:        b.clock,
		cp:           b.cp,
	}, nil
}

// MustBuild is [Builder.Build] that panics on error — for static
// machine definitions where a build failure is a programming bug.
func (b *Builder[S, E]) MustBuild() *Machine[S, E] {
	m, err := b.Build()
	if err != nil {
		panic(err)
	}
	return m
}

// Machine is a constructed, runnable state machine. Safe for
// concurrent use.
type Machine[S comparable, E comparable] struct {
	mu           sync.Mutex
	current      S
	states       map[S]struct{}
	events       map[E]struct{}
	transitions  map[transitionKey[S, E]]*transitionDef[S, E]
	onEnter      map[S][]Callback[S, E]
	onExit       map[S][]Callback[S, E]
	onTransition []Callback[S, E]
	history      []Record[S, E]
	metrics      MetricsSnapshot
	clock        func() time.Time
	cp           Checkpointer[S, E]
}

func (m *Machine[S, E]) now() time.Time {
	if m.clock != nil {
		return m.clock()
	}
	return time.Now()
}

// Current returns the present state.
func (m *Machine[S, E]) Current() S {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.current
}

// Can reports whether an edge for (current state, event) exists. It
// does NOT evaluate guards — guards run only on [Machine.Fire].
func (m *Machine[S, E]) Can(event E) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.transitions[transitionKey[S, E]{from: m.current, event: event}]
	return ok
}

// Fire drives the machine with an event. See the package doc for the
// guard-veto / callback-observe contract. The returned error is:
// [ErrUnknownEvent], [ErrNoTransition], a wrap of [ErrGuardRejected],
// or the joined non-nil callback errors (transition still took effect).
func (m *Machine[S, E]) Fire(ctx context.Context, event E) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	from := m.current
	def, ok := m.transitions[transitionKey[S, E]{from: from, event: event}]
	if !ok {
		if _, known := m.events[event]; !known {
			m.metrics.UnknownEvent++
			return fmt.Errorf("%w: event=%v", ErrUnknownEvent, event)
		}
		m.metrics.NoTransition++
		return fmt.Errorf("%w: state=%v event=%v", ErrNoTransition, from, event)
	}

	tr := Transition[S, E]{From: from, To: def.to, Event: event}

	for _, g := range def.guards {
		if err := g(ctx, tr); err != nil {
			m.metrics.GuardRejections++
			return fmt.Errorf("%w: state=%v event=%v: %w", ErrGuardRejected, from, event, err)
		}
	}

	var cbErrs []error
	collect := func(cbs []Callback[S, E]) {
		for _, cb := range cbs {
			if err := cb(ctx, tr); err != nil {
				cbErrs = append(cbErrs, err)
			}
		}
	}

	collect(m.onExit[from])
	m.current = def.to
	collect(m.onEnter[def.to])
	collect(m.onTransition)
	collect(def.onTransition)

	m.history = append(m.history, Record[S, E]{From: from, To: def.to, Event: event, At: m.now()})
	m.metrics.Transitions++

	return errors.Join(cbErrs...)
}

// History returns a copy of the transition records in order.
func (m *Machine[S, E]) History() []Record[S, E] {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Record[S, E], len(m.history))
	copy(out, m.history)
	return out
}

// Metrics returns a snapshot of the machine's counters.
func (m *Machine[S, E]) Metrics() MetricsSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.metrics
}

// States returns the set of states known to the machine (unordered).
func (m *Machine[S, E]) States() []S {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]S, 0, len(m.states))
	for s := range m.states {
		out = append(out, s)
	}
	return out
}
