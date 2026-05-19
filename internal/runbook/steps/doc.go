// Package steps provides the 9 v1.0 runbook step-type executors per
// PROJECT-DETAILS.md §4.17 (Epic 15 task 8): noop, fail, wait,
// command, script, state, api, notification, query.
//
// Each externally-acting step depends on a small port interface
// declared here ([CommandRunner], [StateApplier], [HTTPClient],
// [Notifier], [Querier]) — never a hard import of the heavy
// subsystem. Real adapters (to internal/execution, internal/statemgmt,
// internal/events, the state store, …) are wired at CLI/server
// composition time (Epic 15 task 10+). This keeps the step executors
// unit-testable with fakes and the dependency graph acyclic.
//
// [RegisterAll] binds all 9 types into a [runbook.Registry] from a
// [Deps]. A port left nil means the corresponding step type returns
// [ErrStepNotConfigured] when executed — so a runbook that only uses
// noop/wait/fail/api runs without wiring command/state/query/
// notification. Step Config arrives already templated by the engine.
//
// # query
//
// The `query` step type is underspecified by §4.17. It is modeled as
// a generic read-only lookup against an injected [Querier]; the real
// backend binding is deferred to the wiring task. No functionality is
// deferred (the executor + contract ship here), so there is no
// ROADMAP entry — only the concrete adapter choice is later.
package steps
