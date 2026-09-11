# Generation 2 Product Charter

This charter is normative for Generation 2. It defines who the product is for,
what it promises, what it refuses, and the exact operator journeys that must be
demonstrably true before `v0.6.0` ships.

It answers "why does this project exist" for Generation 2 and supersedes the
Generation 1 problem statement, which described a scope the reboot discarded.
[RFC 0001](../rfcs/0001-generation-2-reboot.md) governs; where this charter and
RFC 0001 disagree, RFC 0001 wins and this document is wrong.

## What this charter fixes, and what it leaves open

P00 fixes the **operator-visible surface**: command names, required arguments,
exit codes, and externally observable effects. It does not fix transport,
credential format, subject grammar, storage, or protocol encoding — those are
decided in P02–P09 and may not be inferred from the journeys below.

Where an argument exists but its form depends on a later decision, it is marked
**‡** and the owning ADR is named. A ‡ argument's *presence* is fixed here; its
*shape* is not.

## 1. Target operator and fleet profile

**The operator** is the person accountable for keeping a fleet of Linux hosts
running after something else deployed to it. A solo systems administrator, or a
small platform or SRE team. They already have deployment tooling they are not
looking to replace, and they reach for SSH or a heavyweight configuration
management install when they need to act on a host right now.

**The fleet** is 10 to 500 Linux hosts, mixed distributions, mixed placement
across virtual machines, bare metal and cloud instances. Hosts are not assumed
to be directly reachable from the control plane, and the control plane is not
assumed to be reachable from the hosts; only the broker is shared
(`ARCH-COMM-002`).

The operator is not assumed to run Kubernetes, and Keystone Core does not
require it.

## 2. Problems

Exactly the problems `v0.6.0` addresses. Anything else is § 4.

| ID | Problem |
|---|---|
| `PROB-1` | Running one command on one known host requires either standing interactive access or installing a configuration-management system far larger than the task |
| `PROB-2` | When a remote command is interrupted — broker, network, agent or operator — the operator cannot tell whether it ran |
| `PROB-3` | Ad-hoc remote execution leaves no durable record of who ran what, where, and what resulted |
| `PROB-4` | Onboarding a host means distributing a credential that outlives the onboarding |
| `PROB-5` | A long-running remote command cannot be reliably stopped, and stopping the parent process leaves its descendants running |

## 3. Measurable success

Every measure has a number, a method, and an owner. A measure without all three
is not a measure.

| Measure | Target | Method | Owner |
|---|---|---|---|
| Time from fresh host to first successful command | ≤ 10 minutes | Timed fresh-VM run at release-candidate gate, per `TESTING.md` § Gate schedule | Project maintainer |
| Enrollment uses no long-lived credential | 100% of enrollments | Enrollment acceptance test asserts bootstrap credentials are revoked and revocation is verified (`ARCH-NATS-004`) | Project maintainer |
| Interruption never reports a false outcome | 100% of injected fault boundaries | Restart and fault-boundary matrix; every boundary yields a correct terminal state or explicit `UNKNOWN`, never a false success (`ARCH-JOB-004`) | Project maintainer |
| Cancellation terminates the whole process tree | 100% of cancellations | Descendant-process cancellation test, VM-gated (`ARCH-EXEC-002`) | Project maintainer |
| Every lifecycle transition is auditable | 100% of transitions | Correlated lifecycle audit test (`ARCH-OBS-001`) | Project maintainer |
| Duplicate delivery never re-executes | 0 duplicate executions | Non-idempotent external counter under duplicate delivery (`ARCH-JOB-003`) | Project maintainer |
| External operators complete the journey unaided | ≥ 3 partners | § 7 validation plan | Project maintainer |

## 4. Explicit non-goals

Not in `v0.6.0`. Each resolves to a catalogued `Future / Unscheduled`
capability, reachable only through the promotion gate in
[`ROADMAP.md`](ROADMAP.md), or to a direction that needs a new product-level
RFC.

| Non-goal | Resolves to |
|---|---|
| Declarative state management and drift remediation | `CAP-STATE-006` |
| Blueprints | `CAP-BLUE-001`, `CAP-BLUE-002` |
| Runbooks | `CAP-API-018` |
| Secret brokering | `CAP-SECRET-001`, `CAP-SECRET-002` |
| GitOps webhooks and verification workflows | `CAP-API-020`, `CAP-EVENT-019` |
| Clustering and high availability | `CAP-API-007`, `CAP-NATS-002` |
| Policy enforcement | `CAP-EVENT-022` |
| Plugins and extension modules | `CAP-AGENT-015` |
| Windows and macOS agents | `CAP-AGENT-019`, `CAP-AGENT-020` |
| Multi-region broker topologies | `CAP-NATS-014` |
| General-purpose IaC engine, Kubernetes replacement, remote-desktop product, endpoint-detection product, unbounded shell gateway | `ROADMAP.md` § Not Planned — requires a product-level RFC |

Catalogue presence means the idea is retained, not that it is planned. Nothing
above has a version, a date, or an issue.

## 5. Canonical journeys

Seven journeys. Each is an acceptance surface: P10 designs the harness that
drives them, and `ARCH-TEST-001` requires every one to prove its effect
occurred on the intended agent and nowhere else.

Binaries: `keystone` is the operator CLI, `keystone-server` the server,
`keystone-agent` the agent. Packaging additionally installs `ks` as an alias
for `keystone`.

### Exit codes

`keystone` exit codes describe the **control-plane outcome**. They never encode
the remote command's own exit status.

| Code | Meaning |
|---|---|
| `0` | The operation succeeded and its outcome is known |
| `1` | Local or usage error: bad arguments, unreadable configuration |
| `10` | Authorization denied |
| `11` | Target agent is not present within the deadline |
| `12` | The job exceeded its deadline |
| `13` | `UNKNOWN` — the control plane cannot prove whether execution occurred |
| `14` | The job was cancelled |

Codes `10` to `14` exist so the operator can distinguish denial, offline,
timeout, unknown and cancellation without parsing output, which
`TESTING.md` § Feature acceptance contract requires of the Diagnostics case.

A remote command that exits non-zero is a **successful** `keystone run`: the job
completed and its result was retrieved. The remote status appears in the result.
`--exit-remote` makes `keystone run` exit with the remote status instead — in
that mode a remote command exiting `13` is indistinguishable from a control-plane
`UNKNOWN`, which is why it is opt-in and why the default separates them.
Conflating the two is exactly the ambiguity `ARCH-JOB-004` forbids.

### 5.1 Enroll

Two actors, so two invocations.

```
keystone enroll create --agent-name <name>
keystone-agent enroll --token <token> ‡
```

**Exit:** `0` on success. `10` if the operator is not authorized to create a
token. `keystone-agent enroll` exits `0` once the permanent identity is active,
non-zero if the token is spent, expired or invalid.

**Observable effect:** a one-use token is issued and printed once; the agent
holds a permanent scoped credential at mode `0600`; bootstrap access is revoked
and the revocation is verified; the agent appears in § 5.2.

**Invariants:** `ARCH-NATS-004`, `ARCH-NATS-002`, `ARCH-COMM-001`.

‡ Connection arguments are fixed by P03 (enrollment and identity).

### 5.2 List and presence

```
keystone agents list
```

**Exit:** `0` whether or not any agent is present; an empty fleet is not an
error.

**Observable effect:** each enrolled agent is listed with its identifier and
current presence state. Presence is reported as observed, never inferred from
enrollment.

**Invariants:** `ARCH-COMM-001`, `ARCH-OBS-001`.

### 5.3 Run

```
keystone run --agent <agent-id> [--timeout <duration>] -- <argv>...
```

Everything after `--` is the command's argv, passed as a vector. There is no
shell anywhere on this path (§ 6).

**Exit:** `0` when the job reached a terminal state and its result was
retrieved. `10` denied, `11` agent not present, `12` deadline exceeded, `13`
`UNKNOWN`.

**Observable effect:** the command executes exactly once on the named agent and
on no other host; the server performs no equivalent action; a job identifier,
the remote exit status, and captured output are returned; the lifecycle is
durably recorded before execution begins.

**Invariants:** `ARCH-JOB-002`, `ARCH-JOB-003`, `ARCH-EXEC-001`,
`ARCH-TEST-001`, `ARCH-NATS-009`.

### 5.4 Status

```
keystone job status <job-id>
```

**Exit:** `0` when the state is known, including a known terminal failure. `13`
when the state is `UNKNOWN`.

**Observable effect:** the job's current lifecycle state, its target agent, and
its timestamps are reported. `UNKNOWN` is reported as `UNKNOWN` and never
rendered as a failure or a success.

**Invariants:** `ARCH-JOB-004`, `ARCH-OBS-001`.

### 5.5 Output

```
keystone job output <job-id>
```

**Exit:** `0` when the result is durable and retrievable. `13` when the job's
outcome is `UNKNOWN`.

**Observable effect:** the captured stdout and stderr of the remote command are
returned, subject to the output limits of `ARCH-EXEC-001`, together with the
remote exit status. Truncation is reported explicitly rather than silently.

**Invariants:** `ARCH-EXEC-001`, `ARCH-JOB-005`.

### 5.6 Cancel

```
keystone job cancel <job-id>
```

**Exit:** `0` once the job is terminal. `11` if the agent is not present. `13`
if the control plane cannot establish the outcome.

**Observable effect:** the complete process tree on the agent exits, not only
the immediate child; the job reaches a terminal cancelled state; a later result
never contradicts it silently.

**Invariants:** `ARCH-EXEC-002`, `ARCH-JOB-004`, `ARCH-OBS-001`.

### 5.7 Audit

```
keystone audit --job <job-id>
```

**Exit:** `0` when the correlated records are returned.

**Observable effect:** the correlated lifecycle records for the job — actor,
target, action, result and correlation identifier — are returned in order.
Records contain no command secrets and no payload plaintext.

**Invariants:** `ARCH-OBS-001`, `ARCH-NATS-006`.

## 6. The argv-only execution boundary

Frozen from RFC 0001 § Implementation governance. The first release's execution
surface is argv-only and non-interactive. It has none of:

1. a shell;
2. stdin streaming;
3. scripts;
4. pipelines;
5. a caller-selected user;
6. a caller-provided environment;
7. an arbitrary working directory;
8. batch fan-out; and
9. an interactive session.

Execution policy is deny-by-default for unsupported forms (`ARCH-EXEC-001`).

**Widening this boundary requires an amendment to RFC 0001.** It is not a
roadmap entry, not an ADR decision, and not an implementation detail. P07
inherits this boundary and may narrow it; it may not widen it.

## 7. External validation plan

Ninety days from the first `v0.6.0-alpha.N` reaching an external partner, with a
review at day 60.

- **Partners:** at least three external operators with real, non-demo fleets.
  Maintainer-operated fleets do not count.
- **Installation:** each partner installs and enrolls without the maintainer
  controlling the keyboard. Written documentation is the only permitted
  assistance; a session where the maintainer drives is a failed installation,
  recorded as such.
- **Journey completion:** each partner completes all seven journeys in § 5 on
  their own fleet.
- **Sustained real-fleet use:** a partner is sustained if they run commands on
  their own fleet in at least six distinct weeks of the window, unprompted.
- **Evidence retained:** per-partner install outcome, journeys completed,
  weeks active, defects reported, and verbatim statements about willingness to
  pay.

An independent security review precedes any alpha reaching a partner, per
`TESTING.md` § Gate schedule.

## 8. Business evidence gate

`v0.6.0` releases only if, at the end of § 7:

1. at least three external design partners completed the core journey without
   maintainer assistance;
2. sustained real-fleet use is demonstrated as defined in § 7; and
3. there is concrete evidence of willingness to pay — a signed agreement, an
   invoice paid, or a written commitment naming a budget. An expression of
   interest is not evidence.

If that evidence does not appear, **the project narrows or stops rather than
expanding speculatively.** Narrowing means cutting scope to what the evidence
supports. It does not mean adding capabilities from
[`FUTURE-CAPABILITIES.md`](FUTURE-CAPABILITIES.md) in search of a use case;
that is the failure mode the reboot exists to correct.

This gate is a product decision, not a quality gate. Passing every test in
`TESTING.md` does not satisfy it, and failing it is not a defect.
