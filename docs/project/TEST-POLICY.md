# Test policy

Cross-cutting rules for how this project runs its tests. Adjacent to
[COVERAGE-GATES.md](COVERAGE-GATES.md) (which covers per-package
statement coverage) — this file covers the *runtime* test-execution
posture.

## Race detector

**Default**: every `go test` invocation under tracked source
(`Makefile`, `scripts/*.sh`) runs with `-race`.

**Enforced by**: `make race-policy` (CI's `lint` job runs it). The
linter is `tools/racegate/main.go` — scans the Makefile + smoke
script line-by-line, flags any `go test` without `-race` unless its
enclosing Make target is on an explicit allowList.

### Documented exceptions

| Target | Reason |
|--------|--------|
| `make slo` | Wall-clock SLO tests in `test/e2e/{ha,perf}/`. Race instrumentation inflates wall-clock 2-10x, which would make the asserted SLO bounds meaningless. The functional in-`-race` smoke for these mechanisms lives in the per-domain integration tests under `make test-integration`. |

### Adding a new exception

If a new Make target genuinely needs to opt out of `-race`:

1. Add the target name + a one-line reason to `allowList` in
   `tools/racegate/main.go`.
2. Add a row to the table above with the same reason.
3. Run `make race-policy` locally — should pass.

If the justification is anything other than "wall-clock measurement
where race overhead invalidates the result," push back. Race-disabled
test runs are a footgun.

## Test categories

| Make target | Tag | Race | Scope |
|-------------|-----|------|-------|
| `make test` | (none) | yes | Default unit tests (every package). |
| `make test-verbose` | (none) | yes | Same as `test`, with `-v`. |
| `make test-coverage` | (none) | yes | Same as `test`, with `-coverprofile`. CI's primary test job. |
| `make test-integration` | `integration` | yes | Integration tests requiring side-channels (Postgres, embedded NATS). `-p=1` because they share `KSCORE_TEST_POSTGRES_DSN`. |
| `make slo` | `slo` | no | Wall-clock SLO assertions — `test/e2e/{ha,perf}/...`. |
| `make e2e-test` | `e2e` | yes | Single-topology docker-compose E2E (`test/e2e/single/`). The container-side processes run un-instrumented (race doesn't cross process boundaries); the Go-side test code is race-instrumented. |
| `make smoke` | (none) | yes | Fast compile + sqlite-pragma gate. Pre-commit. |
| `make test-cross-distro` | n/a | n/a | Docker-compose cross-distro state-stdlib smoke. Not Go tests. |

## What the gate catches

`make race-policy` (and CI's `lint` job) catches:

- A new Make target whose recipe runs `go test` without `-race`.
- A recipe that loses `-race` via an accidental edit.
- An invocation in `scripts/smoke-test.sh` that drops `-race`.

It does **not** catch:

- Tests that aren't go-test invocations (e.g., `bash test/e2e/state/run.sh`).
- Race opportunities introduced by code itself (that's what `-race`
  at runtime catches).
- Race-instrumented binaries in container images (the kscore-server
  / kscore-agent docker images don't build with `-race`; see "Deferred"
  below).

## Deferred

- **Race-instrumented container images** for `make e2e-test`. Building
  the kscore-server / kscore-agent images with `-race` would catch
  in-server races during scenario runs. Race-instrumented Go binaries
  are 2x memory + 2x CPU; for a v1.0 baseline E2E that's overkill.
  Tracked as a v1.x ROADMAP item: "Race-instrumented e2e images for
  race-sensitive scenarios."
- **goleak in unit tests**: leaked goroutines aren't a race issue but
  are a sibling correctness concern. Tracked as epic 19 task 6.
