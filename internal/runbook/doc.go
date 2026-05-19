// Package runbook implements the v1.0 runbook engine per
// PROJECT-DETAILS.md §4.17 — trigger-based workflow automation.
//
// A [Runbook] is a set of [Step]s wired by `DependsOn` edges. The
// [Executor] resolves the dependency DAG (cycle-detected, with the
// offending path reported), then walks it in topological order:
// each step's `Condition` is rendered and, if falsey, the step is
// skipped (conditional pre-execution); otherwise its `Config` is
// rendered against the run context and the step is dispatched to the
// [StepExecutor] registered for its `Type`, with per-step retry and
// exponential backoff. Outputs thread into the context so later
// steps reference them via `{{ .steps.<name>.outputs.<field> }}`.
//
// The overall run lifecycle (pending → running → succeeded/failed)
// is driven by a pkg/statemachine machine; every step transition is
// recorded on the in-memory [Execution] audit trail.
//
// # Variable scope (gotcha, §4.17)
//
// Cross-step data flows ONLY through explicit template references.
// Rendering uses statemgmt's renderer with missingkey=error, so a
// typo'd `{{ .steps.x.outputs.y }}` fails the step loudly rather
// than silently substituting an empty value.
//
// # What ships in this package (Epic 15 task 7)
//
//   - [Runbook]/[Step]/[Spec] types + strict YAML [Load].
//   - DAG build + cycle detection + conditional gating.
//   - Inter-step variable templating.
//   - Per-step retry with exponential backoff.
//   - [Executor] + the [StepExecutor]/[Registry] seam.
//   - In-memory per-execution audit trail.
//
// # Deliberately out of scope (later tasks)
//
//   - The 9 concrete v1.0 step types (command/api/state/…) —
//     Epic 15 task 8. This package ships only the [StepExecutor]
//     contract + [Registry]; tests use fakes.
//   - Event-system + audit-log emission (Epic 11/12) — task 9.
//     This package keeps an in-memory trail only.
//   - CLI / REST / gRPC — tasks 10/11/12.
//   - post-v1.0 step kinds (if/switch/loop/parallel/sub-runbook).
package runbook
