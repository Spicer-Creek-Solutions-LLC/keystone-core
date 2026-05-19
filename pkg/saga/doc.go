// Package saga implements the v0.1 saga coordinator — multi-step
// orchestration with compensating transactions on failure, per
// PROJECT-DETAILS.md §4.17.
//
// A saga is a sequence of [Step]s, each pairing a forward [Step.Action]
// with an inverse [Step.Compensate]. The [Coordinator] runs the steps
// in order, threading the data each Action returns into the next
// Action's input. On the first error, it walks the *completed* steps
// in **reverse** invoking each step's Compensate to undo whatever
// Action did. The Compensate of a step that itself returns an error
// is logged onto the [StepResult] but does NOT abort the walk —
// PROJECT-DETAILS.md §4.17 line 1223 explicitly mandates
// "aggregate-and-continue (don't abort)" semantics so cleanup work
// goes as far as it can.
//
// # Status taxonomy
//
//	Completed  — every Action succeeded; no compensation ran.
//	Failed     — an Action failed, every Compensate that did run
//	             succeeded; the saga unwound cleanly.
//	Aborted    — an Action failed AND at least one Compensate also
//	             failed; the saga unwound as far as possible but
//	             left some side effects behind.
//
// # What ships in v0.1 (this package)
//
//   - [Step], [Execution], [StepResult] types matching the spec.
//   - [Coordinator.Run] — forward-execute + reverse-compensate.
//   - [Log] interface + [NewInMemoryLog] for in-process tests and
//     the default coordinator hookup.
//   - [NewSQLiteLog] — durable SQLite-backed [Log] (audit trail /
//     list-executions). NOTE its round-trip is lossy: `Data` decodes
//     to a generic value and errors rehydrate as flat [errors.New]
//     (wrap chain lost). See that constructor's doc.
//   - [Coordinator.Clock] injection for deterministic tests.
//
// # What's deferred to v0.x → v1.x (see docs/project/ROADMAP.md)
//
//   - Checkpoint-resume — a saga that crashes mid-forward can be
//     re-loaded and continue from the last completed step, with the
//     pre-step data restored from the log.
//   - Cross-state compensation graphs — compensate by a dependency
//     graph instead of strict reverse-linear order.
//   - Compensation-aggregation reporting — richer error data
//     structures for multi-failure unwinds.
//   - gRPC `StateService.RollbackStateSaga` — saga-driven rollback
//     distinct from today's whole-run rollback CLI.
//
// # State management integration
//
// `internal/statemgmt.Runner.RunSaga` wraps a state run in a saga
// where each declaration is a step whose Compensate re-applies the
// most recent successful prior state from `state.StateHistoryStore`.
// See that method's doc for the integration contract.
package saga
