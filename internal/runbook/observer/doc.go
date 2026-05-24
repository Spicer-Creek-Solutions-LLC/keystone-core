// SPDX-License-Identifier: Apache-2.0

// Package observer adapts the runbook engine's transition stream
// ([runbook.Observer]) onto the Epic 11 event bus and the Epic 12
// audit log (Epic 15 task 9).
//
//   - [EventObserver] publishes one event per transition under the
//     `runbook` event category: runbook.execute.start/done/fail and
//     runbook.step.start/done/fail/skip.
//   - [AuditObserver] emits one audit entry per terminal transition
//     (execution succeeded/failed, step succeeded/failed). Skipped
//     and in-flight transitions do not audit — a skipped step never
//     ran, mirroring internal/audit.StateApplyObserver.
//
// Both are best-effort: a telemetry backend failure must never block
// or fail a runbook, so publish/emit errors are dropped (the event
// bus has its own async-error path; audit has its own sinks). Compose
// them with [runbook.MultiObserver] and set the result on
// [runbook.Executor.Observer] at server wiring (a later task).
package observer
