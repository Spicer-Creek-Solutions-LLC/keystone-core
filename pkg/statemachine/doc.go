// SPDX-License-Identifier: Apache-2.0

// Package statemachine implements the v0.1 generic finite-state-machine
// library, per PROJECT-DETAILS.md §4.17.
//
// A [Machine] is parameterised over a state type S and an event type E
// (both `comparable`). It is constructed via a [Builder]: declare an
// initial state, the legal transitions (each a `from --event--> to`
// triple), optional per-transition [Guard]s, and lifecycle callbacks.
// [Builder.Build] validates the wiring and returns an immutable, ready
// machine.
//
// At runtime [Machine.Fire] drives the machine: it resolves the
// transition for (current state, event), evaluates the transition's
// guards, and — if accepted — runs the OnExit / OnEnter / OnTransition
// callbacks, advances the current state, and appends a [Record] to the
// history.
//
// # Veto vs. observe semantics
//
//	Guards         — vetoing. A non-nil guard error rejects the
//	                 transition; the state does NOT change.
//	OnExit/OnEnter/ — observing. They run for side effects only. A
//	OnTransition      callback error is surfaced (joined) from Fire
//	                  but the transition has already taken effect and
//	                  is recorded; it does not roll back.
//
// This split keeps the state deterministic: a fired event either is
// rejected up front (by a guard) or completes fully. Callbacks model
// effects, not gates — gate with a [Guard].
//
// # Concurrency
//
// Every public method is mutex-guarded; a [Machine] is safe for
// concurrent use (Epic 13 cluster lifecycle drives one from multiple
// goroutines). The lock is held across guard and callback execution,
// so a guard or callback MUST NOT call back into the same machine
// (re-entrant Fire would deadlock).
//
// # Auto-registration
//
// States named by [Builder.Initial], [Builder.Transition],
// [Builder.OnEnter] and [Builder.OnExit] are registered implicitly;
// [Builder.State] only matters for declaring otherwise-unreferenced
// terminal states. There is therefore no "transition references an
// undeclared state" build error — the only structural build failures
// are [ErrNoInitialState] and [ErrDuplicateTransition].
//
// # What ships in v0.1 (this package)
//
//   - [Builder] with [Guard]s, OnEnter/OnExit/OnTransition callbacks,
//     transition history, and a [MetricsSnapshot].
//   - [Machine.Fire] with veto/observe semantics above.
//   - Optional [Checkpointer] seam + [NewMemoryCheckpointer] for
//     snapshot/restore of (state, history).
//   - Injectable [Builder.Clock] for deterministic history timestamps.
//
// # What's deferred to v0.x → v1.x (see docs/project/ROADMAP.md)
//
//   - Hierarchical / parallel (orthogonal) states.
//   - Persistent [Checkpointer] backends (SQLite/etcd) — the
//     interface ships now; only the in-memory impl does.
//   - Metrics export — [MetricsSnapshot] is a value snapshot; wiring
//     it to a metrics sink lands with Epic 17 observability.
//
// # Internal consumers
//
// Per the spec this library is the shared FSM substrate for the
// runbook executor (Epic 15), the rollback and promotion engines
// (Epic 16), and the cluster lifecycle (Epic 13). It deliberately
// carries no dependency on any internal package so those callers can
// adopt it without a cycle.
package statemachine
