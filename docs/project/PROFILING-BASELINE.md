# Profiling baseline

Epic 19 task 8 — the v1.0 performance-profiling pass. Captured
against the perf SLO workload (`test/e2e/perf/`); rerun via
`make profile`.

## Acceptance bar (epic 19 §Hardening)

- CPU > 5% in any single function not in a hot path documented as
  inherent (e.g., JSON marshaling, HMAC computation).
- Allocation > 50 MB cumulative in any single test run.

**Status**: no project-code findings above either bar.

## How to rerun

```bash
make profile
```

Outputs:

- `/tmp/kscore-profile.cpu` — CPU sample profile.
- `/tmp/kscore-profile.mem` — heap allocation profile.
- Top 20 cumulative CPU consumers + top 20 allocation sites printed
  to stdout.

Interactive analysis: `go tool pprof /tmp/kscore-profile.{cpu,mem}`.

## Baseline (v1.0)

### Workload

Three perf SLO tests under `make profile`:

- `TestSLO_CommandLatency_LocalNATS` — 100 sequential ExecuteCommand
  calls + warm-up.
- `TestSLO_EventThroughput` — 1000 events through JetStreamPublisher
  in <100 ms.
- `TestSLO_BatchExec_10Agents` — one BatchExecuteCommand fanned out
  across 10 in-process agents.

Total runtime: ~250 ms on dev workstation.

### CPU baseline

Top cumulative consumers (sample-weighted, dev workstation):

| Cum % | Site |
|-------|------|
| ~29 % | `nats-server/v2.(*client).flushOutbound` / `writeLoop` (embedded broker I/O) |
| ~14 % | `nats.go.(*Conn).RequestMsgWithContext` (publish path) |
| ~14 % | `internal/agent.(*Executor).waitWithKillProtocol` (child-process wait — `/bin/echo`, `/bin/true`) |
| ~14 % | `internal/events.(*JetStreamPublisher).Publish` → `publishOne` |
| < 5 % | rest (none individually > 5 %) |

**No project-code hot spot above 5 %.** The top three are NATS
broker I/O + agent's child-process exec — both inherent to the
workload (the perf SLOs measure end-to-end roundtrips that include
these costs).

### Allocation baseline

Total allocations across the SLO suite: **~33 MB** — well under the
50 MB cumulative bar.

Top sites:

| % | Bytes | Site |
|---|-------|------|
| 19 % | 6.3 MB | `io.copyBuffer` (test-fixture scaffolding) |
| 6 %  | 2.0 MB | `runtime.mallocgc` (general allocator) |
| 3.5 %| 1.2 MB | `nats-server.init` (broker init) |
| 3.5 %| 1.2 MB | `runtime/pprof.StartCPUProfile` (the profiler itself) |
| 3 %  | 1.0 MB | `compress/flate.(*dictDecoder).init` |
| 3 %  | 1.0 MB | `regexp/syntax.(*compiler).inst` |
| < 3 %| each | rest (none individually large) |

**No allocation hot spot above 50 MB.** The largest single site is
test scaffolding; the rest are library inits.

## Remediations applied in this pass

None — the baseline is below both thresholds. Documenting the
numbers so future regressions are catchable.

## What this baseline does NOT measure

- **Steady-state production load**. The SLO workload is short
  (~250 ms total). Long-running profiling against a production-
  shaped workload (sustained agent fleet, real Postgres writes,
  long-running batches) is a separate v1.x ROADMAP item:
  *"Sustained-load profiling baseline."*
- **Per-domain profile detail**. Each SLO test exercises one path.
  Profiling individual domains (state engine apply, blueprint
  apply, secrets read, audit emit) is deferred to v1.x as part of
  the sustained-load work.
- **GC pressure / GOMAXPROCS sensitivity**. The dev-workstation
  numbers are at GOMAXPROCS=N (N = host cores). Production tuning
  may differ.

## When to revisit

- After any change that touches the publish / dispatch / state-runner
  hot paths.
- Pre-release: rerun `make profile`, diff top-20 lists against this
  baseline.
- Whenever a perf SLO test gets a flake — pprof reveals whether
  the regression is in project code or library overhead.
