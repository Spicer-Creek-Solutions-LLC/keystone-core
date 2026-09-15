# ADR-0010: The acceptance harness

- **Status:** Proposed
- **Date:** 2026-09-15
- **Task:** P10, bounded by [`docs/dossiers/P10.md`](../dossiers/P10.md)
- **Builds on:** every accepted ADR, [`TESTING.md`](../project/TESTING.md), and [`PRODUCT-CHARTER.md`](../project/PRODUCT-CHARTER.md) § 5

## Context

Every decision in Stage P states properties a document can assert and only a
running system can hold. `ADR-0002` says an agent cannot publish another agent's
results. `ADR-0006` says a crash between the receipt and the start leaves the job
decidably unstarted. `ADR-0007` says the operator's argv never reaches a shell.
`ADR-0009` says authorization is decided before a request byte is parsed. **Not
one of those is true yet.** They are claims about code nobody has written, and
the harness is what would make them checkable.

`ARCH-TEST-003` — *architecture is executable* — is the invariant this task
exists for, and § 14 states plainly why P10 cannot satisfy it.

**This ADR designs the harness and builds nothing.** No Go module, build or test
runner exists until P11, which follows P10. Every entry point named here is a
target for P11 and each C task, not a file that exists.

## Decision

### 1. Two harnesses, because one of them cannot reach half the product

`TESTING.md` § "Docker versus VM coverage" is not a footnote. It requires VM
coverage wherever behaviour depends on any of eleven things, and it says
**container-only emulations of those "may provide PR feedback but do not satisfy
their release gate."**

Measured against the accepted ADRs, that list takes most of the last three:

| `TESTING.md` trigger | Which accepted decision depends on it | Harness |
|---|---|---|
| **users/groups** | `ADR-0007` § 3's identity change — `setgid`, `initgroups`, `setuid`; § 5's `su -l` harvest of a real account's profile; RFC 0003's `nologin` fallback | **VM** |
| **host security policy** | `ADR-0007` § 7's privilege split: an unprivileged agent account and a root executor | **VM** |
| **users/groups** | `ADR-0009` § 1's socket mode and admin group; § 3's denial of a member whose supplementary groups are stale | **VM** |
| **systemd or another init system** | `ADR-0007` § 10's process-group termination under a service manager; C13's packaged units | **VM** |
| **package-manager transactions** | C13's install, upgrade and uninstall | **VM** |
| **reboot** | Restart recovery across a real boot, not a container restart | **VM** |
| **filesystems, mounts** | `ADR-0008` § 4's ownership and modes; § 10's corruption response | **VM** |
| **kernel interfaces** | `ADR-0009` § 3's `SO_PEERCRED` | **VM** |
| **firewall rules, network configuration** | `ARCH-COMM-002`'s no-direct-reachability — the *proof* is a network probe, which Docker networks provide | **Docker** |
| **LVM** | Nothing in `v0.6.0` depends on it | Neither |

**So the split is not Docker-plus-a-few-extras.** Docker carries transport,
identity, subject authorization, lifecycle, durability and the negative-identity
matrix — everything that is a property of the *protocol*. The VM carries
execution and operator authorization — everything that is a property of the
*host*. That is `ADR-0007`'s and `ADR-0009`'s combined subject matter.

**Two consequences, stated because they are easy to discover late:**

- **`ADR-0007` and `ADR-0009` have no Docker acceptance at all.** A container's
  approximation of user accounts is what `TESTING.md` refuses, so a Docker test
  of the harvest or the socket's group would be evidence of nothing. C07 and C04
  get PR feedback from Docker and their **release gate is the VM**.
- **The charter's success metric is already a VM measurement** — *"Time from
  fresh host to first successful command ≤ 10 minutes | Timed fresh-VM run"* —
  so the VM harness is load-bearing for the product's own headline number, not
  only for edge cases.

### 2. The Docker topology

`TESTING.md` § "Required test topology" fixes the minimum. This states how it is
built, not what it contains.

| Service | Shape |
|---|---|
| NATS | One server, using the account and JWT configuration § 3 generates. Not a test profile |
| Keystone server | One, from the packaged binary |
| Agents | **At least two, with different identities** — the non-target assertion (`ARCH-TEST-001`) is meaningless with one |
| Operator client | One, invoking the production CLI over `ADR-0009`'s socket |

**Networks.** A server network and an agent network with **no route between
them**, and NATS attached to both. No agent publishes a host port.

**The isolation is proved, not assumed.** `TESTING.md` requires a network probe
demonstrating that server-to-agent and agent-to-agent direct connections fail,
and it is a **test that fails when isolation is absent** rather than a statement
in a compose file. A topology that merely omits a route passes vacuously the day
someone adds one.

### 3. Production configuration generation

The harness generates the NATS account, the operator and account JWTs, the
per-role users and the per-agent permissions **by running C03's generator** —
the same code path a deployment runs.

**No test-only authorization adapter exists, at any layer.** `ARCH-TEST-002`
requires negative identity tests to exercise generated production JWTs, and
`ARCH-COMM-003` forbids an in-memory substitute. A harness that could fall back
to a permissive broker configuration when generation is inconvenient is a
harness whose authorization results mean nothing, and the fallback is what would
be reached for under time pressure.

**Fixtures are generated, never committed.** A committed JWT is a JWT that stops
matching the generator, and the first symptom is a test that passes against a
permission set the product no longer has.

### 4. What the harness runs

**Packaged binaries, with production configuration parsing.** Not `go run`, not
a test binary with an exported hook, not a build with test-only symbols.

The Docker images install the same artifacts C13 produces. That is the strongest
form available to a container: it is the production **binary** and the production
**configuration path**, and it is *not* the production **packaging** — an
installed package on a host with an init system is the VM harness's subject.

### 5. Fault controls

`TESTING.md` § "Durability and fault injection" enumerates seven boundaries the
harness must be able to stop at. Each needs a *mechanism*, because a boundary
nobody can cause is a test that silently never runs:

| Boundary | How the harness causes it |
|---|---|
| Before the receipt is persisted | A fault point in the agent, enabled by configuration and **compiled into the production binary** — not a test build |
| After the receipt, before process start | The same mechanism at `ADR-0006` § 3's second durable write, which exists precisely because these are separate transactions |
| While the process runs | Killing the agent's process group from outside |
| After exit, before the result is persisted | A fault point between reaping the process and writing the terminal state, so the exit status exists and is not yet durable |
| After persistence, before publish | A fault point between the durable terminal state and the result envelope, which is the window `ARCH-JOB-005` is written about |
| After publish, before acknowledgement | Dropping the connection at the broker |
| While cancellation is in flight | Killing the agent between `Cancelling` and `Cancelled` (`ADR-0006`) |

**A fault point is configuration, not a build flag.** If faults are compiled out
of the shipped binary, the binary under test is not the shipped one, and
`ARCH-COMM-003` is broken by the mechanism meant to test it. The cost is that a
production binary contains code that can stop it at a boundary; that is
acceptable only because the configuration is inert by default and the audit
record shows it was enabled. **§ 12 makes that a requirement rather than an
intention.**

**The external counter is non-idempotent**, as `TESTING.md` requires: an
accidental duplicate execution must be *visible*, and an idempotent probe hides
exactly the defect `ARCH-JOB-003` exists to prevent.

### 6. External-effect probes

`ARCH-TEST-001` requires proof that an effect occurred on the intended agent and
**did not** occur elsewhere. Both halves are probes, and the second is the one
that gets forgotten:

| Assertion | Probe |
|---|---|
| The effect happened on the target | The non-idempotent counter on that agent advanced by exactly one |
| It did not happen on a non-target | The second agent's counter is unchanged |
| It did not happen on the server | The server's counter is unchanged |
| The job is attributable | The audit record names the actor, target and action (`ADR-0008` § 6) |

**"Unchanged" is asserted, not assumed.** A probe that only checks the target
passes identically whether the command ran on one host or on all of them, which
is the failure `ARCH-TEST-001` is written against.

### 7. Negative identities

`TESTING.md` § "NATS security suite" fixes the principals and the verifications.
The harness provisions every one of them from § 3's generator, and each negative
case is a **test that fails if the operation succeeds** — never a test that
merely records an error.

**Two additions this ADR makes**, because they are `ADR-0009`'s and did not exist
when `TESTING.md` was written:

- **A principal outside the operator admin group**, whose `connect()` the kernel
  refuses. Its assertion is the exit code — `10` per `ADR-0009` § 5 — and the
  absence of any audit record, which § 6 of that ADR states is expected.
- **A principal inside the group whose membership was revoked after login**, so
  its cached supplementary groups still pass the socket's mode check. It must be
  denied in-band **and produce an audit record**. This is the case `ADR-0009`
  § 3 says only the in-band check catches, and without it that argument is
  untested.

Both are VM cases, per § 1.

### 8. The VM harness

**What it exists for**, from § 1's table: users and groups, host security policy,
init systems, package transactions, reboot, filesystems and kernel interfaces.

| Case | Why a container cannot do it |
|---|---|
| Execution as a named service account, with its login environment | Requires real accounts, real profile scripts, a real `nologin` shell |
| The privilege split | Requires a host account boundary the container does not enforce |
| The operator socket's mode, group and stale-membership denial | Requires real supplementary groups and a real login session |
| Process-group termination under a service manager | Requires an init system |
| Install, upgrade, uninstall | Requires package-manager transactions |
| Restart recovery across a reboot | A container restart is not a boot |
| The charter's fresh-host metric | Its own definition says fresh-VM |

**The VM set is the declared Linux support set**, which is C13's to declare. P10
does not name distributions: a list written now would be stale before a harness
exists to run it, and `VERSIONING.md` governs what support means.

### 9. Artifacts: retention, classification and redaction

This is P10's by the plan, and `G22` corrected `TESTING.md` to say so.

**Three classes, and the rule is what may be uploaded rather than what may be
kept:**

| Class | Examples | May reach CI |
|---|---|---|
| **Public** | Test assertions, timings, exit codes, subject names, state transitions | Yes |
| **Sanitised** | Broker configuration, server and agent logs, audit records, job ledgers | Yes, **after redaction** |
| **Never** | Private seeds, NKeys, JWTs with a signing capability, the harvested environment (RFC 0003), command argv beyond the audit record's own copy | **No** |

**Canaries are seeded before the run, not searched for after it.** Each class
gets a planted value with a distinctive marker — a private-key canary, a token
canary, a command canary and an output canary — and **the upload fails if any
canary or private seed appears in any artifact.**

**What this proves and what it does not.** It proves the scan runs and that the
four seeded shapes are caught. **It does not prove redaction is complete**: a
secret the harness never planted is a secret the scan has no pattern for, and
the harvested environment is the case RFC 0003 says will actually carry
credentials. § 14 records this as the deferral it is.

**Retention.** Artifacts are kept on failure. A passing run keeps assertions and
timings only — keeping ledgers and logs for every green run buys nothing and
grows the window in which an unredacted artifact could sit.

### 10. The scale profile

`TESTING.md` § Nightly requires at least 100 agents for one hour. What that
measures, so a failure means something:

| Measured | Failure means |
|---|---|
| Delivery lag at the 99th percentile | The consumer configuration (`ADR-0002` § 7) does not hold at fleet size |
| Redelivery count | `MaxDeliver` and `BackOff` are mistuned, or agents are not acknowledging |
| File descriptors, goroutines, connections, storage, consumers | A leak — the most likely defect a soak finds and the least likely a journey test finds |
| Audit and ledger growth against § 7's retention inequality | Retention does not hold at rate, which is how `ARCH-JOB-003` lapses |

**One hundred agents is a floor chosen by `TESTING.md`, not a target derived from
a deployment.** No deployment exists. C15 sets the real profile, and this section
exists so C15 inherits a shape rather than a number.

### 11. Entry points and the gate schedule

One local command runs the same suites CI runs. Anything reachable only in CI is
a suite nobody debugs.

| Gate (`TESTING.md`) | What runs |
|---|---|
| Every pull request | Lint, unit and race, vectors, invariant and traceability lint, the Docker topology for the touched journey, the NATS permission matrix, regression tests |
| Every merge to `main` | The complete Docker journey suite, the fault matrix, the adversarial suite, package smoke tests, documentation and coverage checks |
| Nightly | § 10's profile, chaos, leak checks, the NATS version matrix |
| Release candidate | **The VM harness in full** — fresh install and uninstall per distribution, upgrade, reboot, and the charter's timed fresh-host run |

**The VM harness is a release-candidate gate**, not a per-pull-request one,
because it is slow and because `TESTING.md` already places it there. **That is a
real cost and § 13 states it**: `ADR-0007`'s and `ADR-0009`'s designs get
merge-time feedback from a container that cannot prove them, and their proof
arrives at the release candidate.

### 12. What the harness must never do

Each of these is a way a green suite could mean nothing:

- **Substitute a test-only authorization adapter** for generated JWTs
  (`ARCH-TEST-002`).
- **Substitute an in-memory bus or an in-process agent** for the broker and a
  separate agent process (`ARCH-COMM-003`).
- **Build with test-only symbols or a test-only binary.** Fault points are
  configuration in the shipped binary (§ 5), inert by default, and **enabling one
  is recorded in the audit record** so a run that used them cannot be mistaken
  for one that did not.
- **Assert only the positive half** of an external effect (§ 6).
- **Emulate users, groups or an init system in a container** and present it as
  satisfying a release gate (§ 1).
- **Commit a generated credential or JWT** as a fixture (§ 3).

### 13. The invariant map lives in the register

`REQUIREMENTS-TRACEABILITY.md` already maps all 24 invariants to planned
evidence, with a design owner and a gate per row. **P10 updates those rows; it
does not copy them.**

Three rows name a *"Test architecture ADR"* as their design owner and have had no
such document to point at. This is it, and they now resolve.

**There is exactly one map**, and this section is the whole of what this ADR says
about it. A second table here would be a second thing to drift, and drift between
two statements of one fact is this project's most-recurring defect.

**The same rule settles a staleness this ADR creates.** `ADR-0003` § 12 records
`RSK-13` as *"Carried to P10"* and `ADR-0008` § Residual risks records three
rows expiring *"or P10"*. **Those gates moved above**, and both documents are out
of this task's bounds.

They are not corrected and do not need to be: **`THREAT-MODEL.md` § 9 is
authoritative for a risk's current owner, rationale, control and expiry**, and an
ADR's own residual-risk table records *that ADR's disposition at its own time* —
the same status `REBOOT-EXECUTION-PLAN.md` gives acceptance evidence, and the
reason RFC 0003 left `P00-acceptance-evidence.md` unedited.

**That an ADR's table can say something the threat model contradicts is a real
cost**, and it is § "What this ADR does not decide"'s third finding rather than
something this ADR quietly relies on.

## Residual risks

| ID | Disposition |
|---|---|
| `RSK-1` | **Renewed.** 2027-09-14, or **C14** |
| `RSK-4` | **Renewed.** 2027-03-13, or **C14** |
| `RSK-9` | **Renewed.** 2027-09-14, or **C14** |
| `RSK-12` | **Renewed, with no task gate.** 2027-09-14 |
| `RSK-13` | **Renewed, re-gated.** 2027-09-13, or **C13** |
| `RSK-14` | **Renewed, re-gated.** 2027-09-14, or **C13** |

**`RSK-1`, `RSK-4` and `RSK-9` move to C14, and the reason is a correction.**
`ADR-0008` sent them here as *"the first task that could demonstrate the
reconstruction of § 3 actually working, rather than asserting it."* **P10 cannot
demonstrate anything** — it designs a harness and nothing runs until P11, which
follows P10. `G22` added the correction note to `ADR-0008`; this is where the
gate moves to a task that can. C14 runs the adversarial suite, and § 7's revoked
principal and § 5's fault matrix are where a reconstruction from agent ledgers
would actually be attempted.

**What P10 adds to all three is a specification and not evidence.** § 6's
non-target probe is `RSK-1`'s separation made checkable; § 9's canary scan is
what would catch a key in an artifact; § 1 states that `RSK-9`'s host boundary is
a VM case. None of that is a demonstration, and saying so is the point of the
disposition.

**`RSK-13` and `RSK-14` re-gate to C13**, which is a correction to where they
were placed rather than a change to the risks. `ADR-0003` carried `RSK-13` to P10
as *"an operational runbook"* and `ADR-0005` assigned `RSK-14`'s
channel-separated deployment mode to P10 — both on the belief that P10 was a
deployment and operations task. **It is not, and no such task exists**, which
`G22` corrected in six documents. C13 builds packages, service definitions and
least-privilege users, so a rotation runbook and an anchor **delivered with the
installation** are its work. P10's contribution is that § 8 can *test* a
pre-provisioned anchor once C13 delivers one.

**`RSK-12` keeps a date and loses its task gate.** Its own compensating control
says the candidate mitigation is a certificate chain rooted outside the
deployment — `CAP-IDENT-004`, `CAP-IDENT-021` — and that **reaching it is a
phase-gate promotion**. No Generation 2 task resolves it, so a task gate would be
theatre: it would expire at a task that renews it unchanged, which is what
happened here. **A date alone is the honest form**, and the cost is stated: it
surfaces at review with no task forcing the question, which is exactly what the
task-gate mechanism exists to prevent. That cost is accepted rather than hidden.

## Invariant coverage

| Invariant | How |
|---|---|
| `ARCH-TEST-003` | § 13 — every invariant has planned evidence in the register. **See § 14** |
| `ARCH-TEST-001` | § 6 — both halves of the external effect |
| `ARCH-TEST-002` | § 7 and § 3 — generated production JWTs, no test-only adapter |
| `ARCH-COMM-002` | § 2 — isolated networks, proved by a probe |
| `ARCH-COMM-003` | § 4 and § 12 — packaged binaries, production configuration, no substitutes |
| Every other `ARCH-*` | § 13's register rows |

## Alternatives considered

**One harness, with containers emulating users and init.** Rejected, and
`TESTING.md` rejects it first: container-only emulation of those capabilities
does not satisfy a release gate. It is also the tempting option, because a single
fast suite is worth a great deal — and it would make `ADR-0007`'s and
`ADR-0009`'s green results meaningless.

**Fault injection as a build flag.** Rejected — § 5. A binary built to be
testable is not the binary that ships, and `ARCH-COMM-003` exists to stop exactly
that substitution.

**Committed JWT fixtures.** Rejected — § 3. They decouple from the generator
silently, and the first symptom is a passing test against permissions the product
no longer has.

**A second invariant map inside this ADR.** Rejected — § 13.

**Naming the supported distributions here.** Rejected — § 8. C13 declares the
support set; a list written before a harness exists is stale on arrival.

**Running the VM harness on every pull request.** Rejected on cost, and the cost
of rejecting it is stated in § 11 and § 13 rather than assumed away.

## Consequences

**C14 becomes the task that discharges three residual risks**, which is more
weight than "adversarial and fault-matrix closure" suggests.

**C04 and C07 get their release gate from the VM harness**, not from Docker. Both
should expect merge-time green to be weaker evidence than usual.

**P11 inherits the entry points** and must make one local command run what CI
runs. It also inherits the first real test of § 13: a register row naming a file
is a claim, and P11 is where a named file exists or does not.

**C03 inherits § 3's generator as the only path to a JWT** the harness will ever
see.

**The audit record gains a fault-point field** (§ 5, § 12), which is a change to
what `ADR-0008` § 6 enumerates. It is **additive** — a record that says no fault
point was enabled is the normal case — and it is named here because a reader of
`ADR-0008` would not otherwise expect it.

## What this ADR does not decide

**Whether the planned evidence is adequate.** § 13 says every invariant has a
row; it cannot say the row's test would establish the invariant.

**The numbers** — timeouts, limits, the real scale profile. C15's.

**The distribution support set.** C13's.

**Two findings are raised and not repaired**, because both live in documents out
of this task's bounds:

- **`TESTING.md` says "the production Docker topology" twice and `ROADMAP.md`
  says "production-process Docker tests".** All three mean *a Docker topology
  running production processes*; read plainly they say Docker is production.
  Docker is the integration floor and **production is packages on hosts**, which
  is why § 1 and § 8 exist at all. Same class as the six documents `G22`
  corrected.
- **A prior ADR's residual-risk table can contradict the threat model.**
  `ADR-0003` and `ADR-0008` still name P10 as a gate this ADR moved, and both are
  out of bounds. § 13 states that the threat model is authoritative and an ADR's
  table records its own disposition — but **nothing enforces that reading**, and a
  reader who finds the gate in an ADR has no signal that it is stale. Whether an
  ADR's risk table should carry dates and gates at all, or only a reference to
  the threat model, is worth deciding once rather than discovering again.
- **No document states the production deployment shape.** `ADR-0002` § 14 is
  titled *Deployment modes* and covers only broker and account topology. That
  agents install as packages on hosts running an init system is inferable from
  the charter's fresh-VM metric, C13's service definitions, `ADR-0007`'s login
  shells and `ADR-0009`'s `/run` socket — and **stated nowhere**. This is not
  pedantry: it is how `RSK-13` and `RSK-14` came to be gated on a task that did
  no deployment, and `ADR-0002` § 14 is where it belongs.

### 14. `ARCH-TEST-003` is not satisfied by this ADR

*Architecture is executable* requires every invariant to be enforced by an
automated test, a static rule, or an equivalent. § 13 gives every invariant a
row naming planned evidence.

**A row naming a file that does not exist is not enforcement.** The register says
so itself — *"The test paths are targets for the clean baseline; they become
mandatory as the corresponding implementation lands"* — and P10 runs nothing, so
it cannot move a single row from target to fact.

**P11 is where a named file exists or does not**, and each C task is where its
rows become true. This section exists because a 24-row table is the most
convincing-looking artifact this ADR produces and the least load-bearing, and a
reader who takes it for coverage will stop looking.

### Deferred decision points, each with a trigger

| Deferred | Trigger |
|---|---|
| **Whether canary scanning is sufficient redaction** | The first artifact found to contain a secret the scan had no pattern for. § 9 states it proves the scan runs, not that redaction is complete |
| **The real scale profile** | C15, with a deployment or a pilot to derive it from |
| **The distribution support set** | C13 declaring it, which it must before a release-candidate gate can name what it ran on |
| **Whether the VM harness can run per-merge** | VM run time falling far enough that § 11's cost stops being worth paying |
| **A production deployment-shape document** | The finding above being owned; `ADR-0002` § 14 is where it lands |

## Validation

`make check`. The acceptance cases and their demonstrated failures are recorded
in [`docs/dossiers/P10-acceptance-evidence.md`](../dossiers/P10-acceptance-evidence.md).

## References

- [P10 dossier](../dossiers/P10.md)
- [TESTING.md](../project/TESTING.md) — the governing rule, topology, suites and gate schedule
- [REQUIREMENTS-TRACEABILITY.md](../project/REQUIREMENTS-TRACEABILITY.md) — the invariant map
- [ARCHITECTURE-INVARIANTS.md](../project/ARCHITECTURE-INVARIANTS.md) — `ARCH-TEST-001`, `ARCH-TEST-002`, `ARCH-TEST-003`
- [THREAT-MODEL.md](../project/THREAT-MODEL.md) — the six risks disposed above
- [ADR-0007 — Safe execution](0007-safe-execution.md) and [ADR-0009 — Local operator API and authorization](0009-local-operator-api-and-authorization.md) — the designs § 1 places on the VM side
