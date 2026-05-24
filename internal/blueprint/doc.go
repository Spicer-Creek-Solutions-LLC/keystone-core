// SPDX-License-Identifier: Apache-2.0

// Package blueprint implements the v1.0 blueprint manifest layer per
// PROJECT-DETAILS.md §4.17 — pre-packaged, Salt-formula-shaped state
// collections.
//
// This package (Epic 15 task 3) owns four concerns:
//
//   - [Manifest] and its sub-types — the parsed shape of a
//     `blueprint.yaml`.
//   - [Load] — read + strict-decode + structurally validate a
//     manifest from disk.
//   - [Manifest.ResolveParams] — coerce string-shaped inputs to each
//     parameter's declared type, apply defaults, and validate the
//     result against a JSON Schema (JSON Schema 2020-12, via
//     github.com/santhosh-tekuri/jsonschema/v6) assembled from the
//     `parameters:` block. Invalid input surfaces a precise error;
//     it is never silently coerced to a zero value (PROJECT-DETAILS
//     §4.17 gotcha).
//   - [Graph] — the inter-blueprint dependency resolver: hard
//     `requires` vs soft `requires_before` edges, cycle detection
//     that reports the offending path, and a dependencies-first
//     topological order.
//
// Epic 15 task 4 adds the pure transforms the executor composes:
//
//   - [EvaluateFeatures] + [FilterStateFile] — `features:` flag
//     evaluation and conditional (per-declaration) state inclusion.
//   - [RenderState] / [RenderContext] — Go-template rendering of
//     state files against the parameter + feature context, reusing
//     statemgmt's renderer (one template dialect product-wide).
//   - [Namespace] + [DetectCollisions] — multi-instance `as:`
//     state-identity namespacing with collision detection.
//
// Epic 15 task 5 adds the [Executor]: it resolves parameters
// (substituting `source: secret` values via a [SecretResolver]),
// evaluates features, runs pre/post hooks as runbooks (via a
// [HookRunner]), renders + filters + namespaces + resolves the
// entrypoint state collection and runs it through a [StateRunner],
// renders [Manifest] outputs, and records an [AppliedRun] so
// [Executor.Rollback] can revert by run ID. All collaborators are
// interfaces; the in-memory [AppliedStore] ships now (a durable
// backend is the gate-v1.0 ROADMAP item "Blueprint applied-runs
// store (durable)").
//
// # Deliberately out of scope for this package/task
//
//   - Wiring the [Executor] into a running kscore-server (the
//     concrete State Runner / SecretBroker / runbook registry are
//     injected at server composition) — Epic 15 task 10+.
//   - The 6-blueprint v1.0 catalog — Epic 15 task 6.
//
// # Sensitive parameters
//
// A parameter with `sensitive: true` (implied by `source: secret`)
// must never be logged. [ResolvedParams.Values] holds resolved
// values including sensitive ones; callers logging params must use
// [ResolvedParams.Redacted].
package blueprint
