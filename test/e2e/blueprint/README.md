# Blueprint/Runbook end-to-end suite (Epic 15 task 13)

Build-tagged (`//go:build integration`) in-process integration suite
that wires the real Epic-15 engines exactly as a server composes them
and drives them against the real shipped catalog.

## Run

```sh
make test-integration            # whole integration suite
go test -tags=integration ./test/e2e/blueprint/
```

Excluded from the default `go test ./...` by the build tag.

## What it proves

- `production-cluster` applied end to end: `secret://` password
  resolved + substituted into rendered state, parameters/features
  resolved, the **preflight hook actually runs as a runbook**, the
  entrypoint is rendered → parsed → feature-filtered → resolved, every
  declaration reaches the State Runner, the `AppliedRun` is recorded,
  and `rollback` reverts via `entrypoints.rollback`.
- `demo` applies and renders its output (acceptance #1).
- Runbook: step 2 reads step 1's output via templating; a step
  failure triggers the `onFailure` chain; an audit trail is recorded.
- Saga: steps 1–3 succeed, step 4 fails, 3→2→1 compensate in reverse.

## Scope

A **recording State Runner** stands in for host convergence —
applying `production-cluster`'s package/service/sysctl declarations
for real is not CI-safe with the v1.0 stdlib and is not what proves
the integration (the runner records that every resolved declaration
was dispatched). The literal multi-container docker-compose
convergence form is a `gate-v1.0` ROADMAP item ("Blueprint e2e real
docker-compose convergence form").
