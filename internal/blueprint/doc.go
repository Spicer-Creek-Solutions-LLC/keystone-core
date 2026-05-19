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
// # Deliberately out of scope for this package/task
//
//   - Feature-flag evaluation + template rendering — Epic 15 task 4.
//   - The blueprint executor + Epic 10 SecretBroker credential
//     lookup for `source: secret` params — Epic 15 task 5.
//     [Manifest.ResolveParams] *identifies* secret-sourced
//     parameters ([ResolvedParams.Secret]) but does NOT fetch them.
//   - The 6-blueprint v1.0 catalog — Epic 15 task 6.
//
// # Sensitive parameters
//
// A parameter with `sensitive: true` (implied by `source: secret`)
// must never be logged. [ResolvedParams.Values] holds resolved
// values including sensitive ones; callers logging params must use
// [ResolvedParams.Redacted].
package blueprint
