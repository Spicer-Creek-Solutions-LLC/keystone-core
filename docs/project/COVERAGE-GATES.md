# Coverage gates

Per-package statement-coverage gates enforced by `tools/covgate` and
called from CI via `make coverage-gate`. Targets match
`PROJECT-DETAILS.md` §5.3 and `AGENTS.md` §5:

| Category | Threshold | Packages |
|----------|-----------|----------|
| Critical | ≥ 70% | Business-logic packages: most of `internal/*` and `pkg/*` |
| CLI | ≥ 40% | `cmd/*` binaries and `internal/cli/*` packages |
| Excluded | — | Generated code (`pkg/api/v1`), tooling (`tools/`), test scaffolding (`test/`), example modules (`modules/examples/`) |

## How it works

1. `make test-coverage` runs the unit-test suite with
   `-coverprofile=coverage.out`.
2. `make coverage-gate` runs `go run ./tools/covgate --profile=coverage.out`.
3. covgate parses the profile, aggregates statement-coverage per
   package, classifies each via `tools/covgate/config.go`, and asserts
   the per-category threshold.
4. Exit 0 on all-pass, 1 on any FAIL.

CI runs the gate immediately after `make test-coverage` in the `test`
job — there's no separate workflow.

## Verdicts

| Verdict | Meaning |
|---------|---------|
| `PASS` | Coverage meets or exceeds the category threshold. |
| `FAIL` | Coverage is below the category threshold and the package is **not** on the allowList (or the allowList entry is stale — see below). |
| `ALLOWED` | Coverage is below threshold but the package has an explicit allowList entry. Counts as PASS for exit-code purposes. |
| `SKIP` | Package matched `excludedPrefixes`. No assertion. |
| `WARN` | Package didn't match any classification rule. With `--strict-unmatched` this is promoted to FAIL. |

## The allowList

Some packages are below threshold today and tracked for graduation.
Each lives in `tools/covgate/config.go`'s `allowList` map with the
package's *current measured* coverage. The gate then enforces two
invariants:

1. **Coverage must not regress** below the allowList value — i.e. if
   `pkg/api/cluster` is allow-listed at 68.7%, dropping to 65% fails
   the gate.
2. **Coverage rising above the category threshold removes the
   exception** — once a package crosses 70% (critical) or 40% (CLI),
   the gate fails with a "remove the entry" message so operators
   graduate it into the regular gate.

Every allowList entry MUST carry a comment with one of:

- The PR / commit that will raise coverage.
- A `gate-v1.0` or `v1.x` ROADMAP entry that schedules the work.
- An explicit "ships with epic NN task M" commitment.

Untriaged exceptions are not allowed in the file.

## Adding a new critical package

1. Open `tools/covgate/config.go`.
2. Add the package's relative path to `criticalPackages` (or to
   `criticalPrefixes` if it's a cohesive subtree like
   `internal/statemgmt/stdlib/...`).
3. Run `make test-coverage && make coverage-gate` locally to confirm
   the new package passes the 70% bar.

## Graduating an allowList entry

1. Add tests (or wait for a domain epic to add them).
2. Run `make test-coverage && make coverage-gate` locally. The gate
   will FAIL with "coverage X% now exceeds allowList entry Y% — remove
   the entry."
3. Remove the entry from `allowList`. Re-run the gate; it should
   PASS.
4. PR the removal alongside the test additions.

## Why per-package, not project-wide?

A single project-wide threshold makes regressions in one package
invisible behind another package's high coverage. Per-package keeps
each module honest. The category split (`critical` vs `cli`) reflects
where bugs are *expensive* — control-plane / agent / state code needs
the higher bar; CLI wrappers around tested cores need less.

## Why statement coverage, not branch?

Go's `go test -coverprofile` produces statement coverage only.
Branch coverage requires external tooling (e.g. gocov) and is a v1.x
follow-up tracked under `docs/project/ROADMAP.md` "Benchmark suite
for regression detection."

## Why `mode: set` not `mode: count`?

`-race` enables atomic mode by default for race-safety. The gate
reads `count` and `atomic` profiles identically — any hit > 0 counts
the block as covered. The mode doesn't affect the gate verdict.

## Deferred to v1.x

- **Diff coverage** (per-PR delta) — covgate looks at absolute
  numbers only.
- **Branch coverage** — needs gocov or similar.
- **Codecov / Coveralls upload** — local-CI gate is sufficient for
  v1.0; an external dashboard is post-v1.0.
- **Function-level gates** — package granularity is the agreed
  shape (epics + AGENTS state package-level).
