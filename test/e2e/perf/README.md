# Performance SLO suite

Epic 19 task 3. Three throughput + latency SLOs that complement the
HA / cluster-formation SLOs under `test/e2e/ha/`.

## SLOs

| Bound | Test | What it measures |
|-------|------|------------------|
| Single-agent command latency `<100 ms` | `TestSLO_CommandLatency_LocalNATS` | Median wall-clock from `CommandDispatcher.Dispatch` to terminal `CommandResponse` over 100 measured samples (20-sample warm-up first). Production `NATSBatchExecutor` is the runner — same path the server's `ExecuteCommand` RPC drives. |
| Event-emission throughput `>10k events/s` | `TestSLO_EventThroughput` | 1000 events through `JetStreamPublisher.Publish` complete in `<100 ms`. |
| 100-batch command exec across 10 agents `<2 s` | `TestSLO_BatchExec_10Agents` | Wall-clock from first `Dispatch` to last terminal response when fanning out one command across 10 in-process agents. |

The HA SLOs (cluster forms `<10s`, first leader `<3s`, failover
detection `<5s`, agent reassign `<10s`, minority blocks writes `<1s`,
recovery `<15s`) live in `test/e2e/ha/slo_test.go`.

## Running

```bash
make slo
```

The single target runs both `test/e2e/ha/...` and `test/e2e/perf/...`
under the `slo` build tag. CI's `slo` job picks up both suites — no
separate gate.

## Harness shape

In-process: embedded NATS server + in-process control-plane
dispatcher / response router / `NATSBatchExecutor` + in-process
agent runtime(s). Docker-compose is deliberately avoided — bridge
NAT adds 1–5 ms per RPC, which is 5% of the 100 ms command-latency
SLO. The cost we measure is the mechanism cost, not the container-
orchestration cost.

`make slo` runs without `-race` (race instrumentation inflates
wall-clock 2–10×, making the asserted numbers meaningless). The
functional in-`-race` smoke lives in the per-domain integration
tests (`make test-integration`).

## Why these numbers

The three v1.0 baseline SLOs are documented in epic 19 §Scope and
match what the operator-facing release commits to. Targets are
generous enough to survive CI scheduling jitter and tight enough to
catch genuine regressions. If a future change pushes any of them
above the bound, the SLO test fails before the change ships — at
which point we either reduce the regression or revisit the bound
with evidence.

Current measured numbers on a developer workstation (Linux amd64):

| SLO | Bound | Measured |
|-----|-------|----------|
| Command latency p50 | <100 ms | ~400 µs |
| Event throughput | >10k/s | ~75k/s |
| 10-agent batch | <2 s | ~12 ms |

CI runners are slower, but the bounds have ~100× headroom.

## Open questions / known deferrals

- **No tail-latency SLO** (p95/p99) — p50 is the right summary for
  steady-state mechanism cost; tail latency includes GC pauses + OS
  scheduler noise and belongs in a separate benchmark-style suite.
  v1.x ROADMAP item: "Benchmark suite for regression detection".
- **No multi-process / docker-compose SLO** — the v1.0 baseline is
  the mechanism-cost SLO. Multi-process measurement adds bridge-
  network latency that isn't what these bounds promise.
- **No load-generation tooling** (vegeta / k6) — the three tests
  are fixed-shape, not load-curve sweeps.
