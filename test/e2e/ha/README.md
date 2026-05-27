# HA end-to-end suite (Epic 13 task 17)

In-process, component-level HA end-to-end tests for the Epic 13
clustering control plane.

## Scope — honest

A literal multi-process `3×kscore-server` deployment with real
`iptables` partitions is **blocked** by the `gate-v1.0` boot-wiring
ROADMAP entries: `ClusterService`/`CoordinationService` are not yet
registered at `kscore-server` boot, `FencingManager.Guard` is not
wired around the write handlers, and `SingletonTaskManager` is not
constructed at boot.

This suite is the **harness those entries graduate against**. It
proves the HA *mechanisms* now, against the real implementation:

- real `internal/cluster` managers (membership / election / shard /
  failover / fencing / recovery) on a **real embedded etcd**;
- the real `CoordinationGRPCServer` over a **real mTLS TCP
  listener** (in-test CA + client/server certs);
- a **real Postgres** for the database-failover scenario
  (`KSCORE_TEST_POSTGRES_DSN`-gated).

The "3-node cluster" is the §4.15 model: N membership / election /
shard / failover instances sharing one embedded `EtcdClient`
(embedded etcd ≤3 members; Keystone members are keyed by ID on
shared etcd).

What is *deterministically seamed* (and why):

- **Quorum signal** (`fakeQuorum`) — `HealthMonitor`'s quorum
  *detection* is unit-tested in `internal/cluster`; driving the
  signal directly makes the 1s minority-block bound measurable
  without racing a real partition.
- **Membership delivery** for the rebalance-math test
  (`fakeMembers`) — short, near-identical agent ids FNV-cluster
  into a narrow ring arc and a live `MembershipManager` watch adds
  cross-node latency; the internal `shardmanager_test` avoids both
  the same way. The ring + `ShardStore` + etcd + consistent-hash
  math under test are all real.
- **NATS up/down** (`toggleNATS`) — `Connected()=false` is exactly
  what a real NATS `Manager` surfaces to `CoordinationService`.

## Running

Build-tagged `integration`, so it is **excluded from
`make test`**. CI runs it on every PR via `make test-integration`
(the `integration:` job in `.forgejo/workflows/ci-fast.yml`, which
provides the sidecar Postgres).

```sh
make test-integration                 # whole integration suite
go test -tags=integration ./test/e2e/ha/...   # this suite only
```

`DatabaseFailover` skips unless `KSCORE_TEST_POSTGRES_DSN` points at
a writable Postgres (set in CI).

## Coverage note

The `internal/cluster` >80% coverage gate is met by unit tests and
measured by `make test-coverage` (no `-tags=integration`). This
suite is additive behavioural verification — it is not
coverage-counted.

## SLO numbers (Task 18)

The functional scenarios above (`//go:build integration`) assert
correctness with CI-safe budgets. The tight §4.15 SLO numbers are a
separate gate: `slo_test.go` (`//go:build slo`), run by **`make
slo`** WITHOUT `-race` (race instrumentation inflates wall-clock
2–10×, which would make the asserted numbers meaningless).

```sh
make slo                                  # the SLO gate (no -race)
go test -tags=slo ./test/e2e/ha/...       # same, directly
```

`make slo` is its own required CI job (the `slo:` step in
`.forgejo/workflows/ci-fast.yml`) — no Postgres sidecar, runs on every
PR, failures block merge. Bounds asserted:
first leader `<3s`, cluster forms `<10s`, failover detection `<5s` /
completion `<10s`, agent reassignment `<10s`, minority blocks writes
`<1s`, recovery (restart) `<15s`, zero failover duplication. Each
sub-test logs its measured value even when green, so a regression
that still passes the bound is visible in CI output.

The shared harness compiles under `integration || slo`; the
functional-only helpers live in `harness_integration_test.go`
(`integration` only) so the `slo` build has no unused symbols.

These verify the SLOs of the real in-process mechanisms. The
server-integrated numbers (agent-reconnect latency through a running
multi-process `kscore-server`, graceful-shutdown zero-disconnect
timing) ride with the `gate-v1.0` "HA E2E multi-process /
iptables-partition form" ROADMAP entry.
