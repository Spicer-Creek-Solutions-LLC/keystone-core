# ADR-0007: Safe execution

- **Status:** Proposed
- **Date:** 2026-09-15
- **Task:** P07, bounded by [`docs/dossiers/P07.md`](../dossiers/P07.md)
- **Builds on:** [`ADR-0005`](0005-versioned-encrypted-protocol.md), [`ADR-0006`](0006-delivery-and-job-lifecycle.md), [RFC 0003](../rfcs/0003-caller-selected-execution-user.md)

## Context

Every decision before this one governs a command while it is still data.
`ADR-0002` decided how it travels, `ADR-0004` who may send it, `ADR-0005` what is
inside it, `ADR-0006` what the system believes about it. **This decides what it
can touch once it is a process**, and it is the last decision before argv becomes
a running program.

`ACT-9` — a malicious command author with legitimate authority — is the actor
this ADR exists to bound. Not to exclude: someone who can run commands can run
harmful commands, and no execution design changes that. What it bounds is
everything *beyond* what the operator asked for: a command that escapes its
argv, outlives its deadline, exhausts the host, leaves descendants running, or
reaches authority nobody granted it.

[RFC 0003](../rfcs/0003-caller-selected-execution-user.md) moved the boundary
once, in the same discussion that produced this ADR. A command names the account
it runs as, and a shell may compute that account's login environment. **Nothing
else moved**, and § 2 states the rest as still excluded.

## Decision

### 1. Argv-only execution

A command's argv is a **vector** from the operator's shell to the `execve` call.
It is never a string, never concatenated, never re-split.

**No shell interprets it.** A shell runs elsewhere — § 5's harvest — and it
receives a fixed command the agent authors, never any part of a command's argv.
The property `THR-15` rests on is not that no shell is present on the host, nor
that no shell ever executes; it is that **operator input reaches no parser that
gives characters meaning beyond what the operator wrote**. § 2.1 states the
consequence, which is that a caller may name an interpreter and has escaped
nothing by doing so.

That distinction is the whole of `RFC 0003`'s narrowing of the boundary's first
item, and it is why the amendment does not reopen `THR-15`.

### 2. The excluded forms

| Form | Status | A request for it produces | Closes |
|---|---|---|---|
| A shell interpreting the command's argv | **Excluded** | Keystone constructs no shell invocation and wraps no argv. There is nothing to refuse at runtime because Keystone never builds the form | `THR-15` |
| stdin streaming | **Excluded** | The process receives a closed stdin | `THR-15` |
| Scripts | **Excluded** | Keystone accepts no script body and writes no file to run. It chooses no interpreter | `THR-15` |
| Pipelines | **Excluded** | Refusal; one command is one process tree | `THR-15` |
| A caller-provided environment | **Excluded** | Refusal. The environment comes from the target account (§ 5) | `THR-16` |
| An arbitrary working directory | **Excluded** | Refusal. The directory is fixed by § 6 | `THR-16` |
| Batch fan-out | **Excluded** | Refusal; one command targets one agent (`ARCH-NATS-009`) | — |
| An interactive session | **Excluded** | Refusal; no stdin, no tty, no session | `THR-15` |
| **A caller-selected execution user** | **Admitted** (`RFC 0003`) | The command runs as the named account; absent a name, the deployment's default | `THR-52` |
| **A shell for the login-environment harvest** | **Admitted** (`RFC 0003`) | § 5's harvest, under a fixed agent-authored command | `THR-53` |

### 2.1 What the exclusions constrain, and what they do not

**They constrain what Keystone does. They do not constrain which binaries exist
on the host.** Stating this is not a weakening; it is the difference between the
boundary this ADR can enforce and one it could only pretend to.

Keystone builds no shell invocation, wraps no argv in a string, accepts no
script body, writes no file to execute, and chooses no interpreter. Those are
the forms RFC 0001 removed — Generation 1's `--shell bash` **made Keystone wrap
argv in a shell**, and that is what item 1 took away.

**An operator may still name an interpreter.** `python3 /opt/x.py` is argv — a
program and an argument. So is `/bin/sh -c '…'`. So is `/usr/sbin/service`,
which on most distributions **is** a shell script, and `argv[0]` resolving to a
file with a `#!` line means the kernel invokes that interpreter without Keystone
choosing it.

None of that is an escape, and `THR-15` is precise about why: the threat is
*supplied argv **escapes** the bounded surface*. A caller who names `/bin/sh`
has escaped nothing — they ran a command they fully specified, with the
authority `ACT-9`'s own row grants them, *execute on the fleet under legitimate
authority*. What argv-as-a-vector prevents is the other thing: a caller
supplying `; rm -rf /` inside an argument and having it become a second command.
That protection is unaffected by which binary `argv[0]` names.

**The alternative reading is untenable and worth saying so.** Enforcing "no
script ever executes" would mean inspecting every resolved `argv[0]` for a `#!`
line and refusing it — which rejects `service`, `ldconfig` and much of a
distribution's administrative surface, the exact work `PROB-1` exists for — and
then refusing every binary that could interpret something, which is an allowlist
of permitted programs. That is a policy engine, and RFC 0001 discarded it.

**So this ADR does not claim scripts cannot run. It claims Keystone never
arranges for one.** A design that claimed the first would be claiming an
enforcement it does not have, which is the class of error `THR-36` describes.

### 3. Deny-by-default

An execution form this ADR does not name is **refused**, not attempted
(`ARCH-EXEC-001`). A request carrying a field the agent does not understand is
refused rather than ignored, which is the same rule `ADR-0005` § 9 applies to an
envelope and for the same reason: silently discarding an instruction is how a
caller ends up believing something happened that did not.

### 4. Executable resolution

`argv[0]` is resolved **against a `PATH` the agent fixes**, or is used as-is when
it is absolute.

**The harvested `PATH` is passed to the process and is never used for
resolution.** This is the sharpest consequence of admitting the harvest, and it
is worth stating why rather than leaving it as a rule:

`THR-53` records that a writable profile on a target account can influence
commands run as that account, and concludes that this changes no privilege —
true, because the command runs as that account anyway. **Resolution is the case
where it would stop being true.** A profile that sets `PATH=/tmp/evil:$PATH`
would, if resolution used the harvested value, decide which binary an operator's
`systemctl` names — and the operator chose the name believing it meant the
system's. Resolving against a fixed `PATH` keeps the profile's influence inside
the account, which is where `THR-53`'s reasoning needs it.

A command whose `argv[0]` does not resolve is refused before any durable start
is recorded, so it never reaches `Running`.

### 5. The environment

The process receives the **target account's login environment**, harvested as
`RFC 0003` admits.

| Step | Rule |
|---|---|
| Harvest | The agent runs the target's login shell with `su -l` semantics, executing **one fixed command the agent authors** that prints the environment. No part of the command's argv is passed to it |
| Timeout | The harvest carries **its own** timeout, distinct from the command's deadline. Exceeding it falls back |
| Output | Harvest output is read by the agent and **never merged into the command's captured stdout or stderr** |
| Failure | A non-zero exit falls back; it does not fail the command |
| Size | The harvested environment is bounded; exceeding the bound falls back |
| `nologin` | An account with no usable shell falls back without attempting a harvest |
| Fallback | The **derived environment**: `HOME`, `USER`, `LOGNAME`, `SHELL` and `PATH` as the user database and `login.defs` define them for the target |
| Audit | The harvested environment **never reaches an audit record** (§ 11) |

**The process receives that environment unmodified.** The agent adds nothing and
removes nothing. It has no need to: resolution does not consult `PATH` (§ 4), and
a variable the agent injected would be a caller-invisible influence on a command
the operator believes it fully specified.

**A fallback is an operationally significant fact**, not a silent degradation. A
command that ran without the environment it expected fails in ways that look
unrelated, so the result records that a fallback occurred and why. How an
operator sees it is **P09**'s.

### 6. The working directory

The process starts in the **target account's home directory** when it exists and
is accessible, and in `/` otherwise.

It is not caller-selectable — an arbitrary working directory remains excluded
(§ 2). Home is the choice consistent with `su -l` semantics and with the
motivating case for `RFC 0003`: a command that creates files should create them
where that account's files belong.

### 7. Service identity and privilege

**Two components.** This is the decision with the largest consequence in the
ADR, and `RSK-9` is what it is measured against.

| Component | Runs as | Holds | Does |
|---|---|---|---|
| **Agent** | A dedicated unprivileged account | The NATS connection, `AST-3`/`AST-4`/`AST-5`, the ledger, all protocol parsing and crypto | Everything except creating a process |
| **Executor** | `root` | Nothing but the ability to change identity | `setgid`, `initgroups`, `setuid`, the harvest, `execve`, and ownership of the process group |

The interface between them is local, narrow, and carries one operation: *run
this argv, as this account, with these limits*.

**What this bounds.** The component with the network attack surface — envelope
parsing, decryption, signature verification, JetStream — is not root. A
memory-safety defect there yields the agent account, not the host.

**What it does not bound, stated because the reverse is easy to assume.** An
attacker who controls the agent *logically* can ask the executor to run a command
as root, and the executor will. The split moves the boundary; it does not close
it. `RSK-9` is narrowed in one dimension and unchanged in the other.

**What would close it, and why this ADR does not.** An executor that
**independently verified the service signature** before executing would mean a
compromised agent could lie about results and still not obtain arbitrary root —
only commands the server actually signed would run. The public half it needs is
already on the host (`ADR-0003` § 1, since `G15`).

It does not work against the protocol as accepted, and the reason is a finding
rather than a design choice. See § 13.

### 8. Limits

`ARCH-EXEC-001` names six categories. Each has a value or a stated owner; none is
absent or infinite.

| Category | Decision |
|---|---|
| **Duration** | Per command, from the operator's `--timeout`, bounded above by a deployment maximum. A command naming none gets the deployment default. Expiry is `ADR-0006`'s `TimedOut` |
| **Output** | Bounded per command, **well below** `ADR-0002` § 9's 1 MiB payload limit, because a result envelope carries more than output. Exceeding it truncates (§ 9) |
| **Environment** | The harvest's size bound (§ 5) |
| **Working directory** | Fixed, not selectable (§ 6) |
| **Concurrency** | **One command per agent at a time**, which is not a new limit: `ADR-0002` § 7's `MaxAckPending=1` already makes it structural, and `ADR-0006` § 4 records that this is what makes `Running` a single job |
| **Host resources** | Memory, CPU and process count, applied by the platform's own mechanism to the process group. The **values are the deployment's**; the requirement is that they exist and are finite |

**Values are deployment-settable and their absence is not.** A deployment that
sets none gets the defaults; a deployment that sets one to unbounded is rejected
at configuration time rather than at execution time.

### 9. Output handling

Both streams are captured, bounded, and returned in the result envelope.

**Truncation is reported explicitly, never silently** (`THR-22`). The result
carries that truncation occurred, which stream, and how much was discarded.

**A truncated command is not a failed command.** It ran, it exited, and its
status is what it is. Truncation is a fact about the record, not about the
outcome, and conflating them would report a successful command as failed for a
reason the operator did not choose.

Output arrives after the bound is reached only to be discarded; the process is
not killed for producing it. That is deliberate — killing a process for being
verbose turns a reporting limit into an execution outcome, and `THR-22`'s concern
is memory exhaustion, which discarding already prevents.

### 10. Termination

One mechanism serves cancellation (`ADR-0006` § 9) and the deadline.

1. **`SIGTERM` to the process group**, not the immediate child.
2. **A grace period**, deployment-settable, during which the process may exit on
   its own terms.
3. **`SIGKILL` to the process group.**

`ARCH-EXEC-002` requires the complete process group or platform-equivalent job
object, and the process group is created at execution time precisely so it can be
signalled as a unit. `THR-23` is the defect this closes — *cancellation kills the
immediate child and leaves descendants running* — and it is `PROB-5`'s technical
half, which is why the charter measures it rather than trusting it. A descendant that escapes its process group — by creating
its own session — is not reached by this, and the acceptance harness's
descendant-process test is what would find it (`P10`, and the charter's own
success metric).

**A process that ignores `SIGKILL`** is in uninterruptible sleep and no signal
will reach it. The agent does not wait indefinitely: it stops waiting, and the
job's outcome is `ADR-0006`'s `Unknown` — the control plane cannot prove whether
the command completed. Reporting it as cancelled would be a false terminal state,
which is `THR-25`.

### 11. Audit redaction, and what it cannot do

| Item | In the record? |
|---|---|
| argv | **Yes.** It is what the operator asked for; the charter's § 5.7 journey returns the action |
| The execution user | **Yes.** It is part of what was asked, and `THR-52`'s only control |
| The harvested environment | **Never.** It is where a deployment keeps credentials (`RFC 0003`) |
| Captured output | Per `ARCH-OBS-001` and **P08**, which owns the record |

**What redaction cannot do.** argv is operator-supplied and structurally opaque:
`mysql --password=hunter2` is a secret, and `mysql --database=hunter2` is not,
and nothing in the envelope distinguishes them. Keystone records what it was
told to run, so **a secret placed in argv is a secret placed in the audit
record.**

This is a limit, not a gap, and stating it plainly is the mitigation available:
an operator who knows it will not pass credentials in argv. A design that
claimed to strip secrets from argv would be more dangerous than this one, because
it would be believed.

### 12. What a limit breach becomes

Expressed as `ADR-0006` § 1's states. P07 invents none.

| Breach | Outcome |
|---|---|
| Duration | `TimedOut` — the process group is terminated by § 10 |
| Output | **No lifecycle effect.** The command runs to completion and § 9 truncates |
| Environment or harvest | No lifecycle effect; § 5's fallback applies |
| `argv[0]` does not resolve | `Refused`, before any durable start (§ 4) |
| An excluded form is requested | `Refused` (§ 3) |
| **Host resources** | **See § 13.** The process group is terminated and `ADR-0006` has no state for it |

### 13. Two findings this ADR raises and does not fix

**`ADR-0006` has no lifecycle state for a resource-limit termination.** Its § 1
offers `TimedOut` for a deadline, `Cancelled` for a cancellation, and `Completed`
for a process that ran to its own conclusion. A process killed for exceeding
memory or a process-count limit is none of the three: it did not complete, no
deadline elapsed, and nobody cancelled it. Reporting it as `Completed` with a
signal status would be the dishonesty `ARCH-JOB-001` and `THR-36` exist against,
and reporting it as `TimedOut` would name a cause that did not occur.

The candidates are widening `TimedOut` to *terminated by a limit*, or adding a
state. Both change `ADR-0006` § 1 and its lifecycle table, which is out of this
task's boundary. **Raised against `ADR-0006`.**

**`ADR-0005`'s key placement blocks the executor from verifying what it runs.**
§ 7 records that an executor verifying the service signature would narrow
`RSK-9` materially. It cannot, as the protocol stands: `ADR-0005` § 5 encrypts
the command payload **to the agent** (`AST-5`) and § 4 signs over the envelope.
The executor can verify the envelope is genuine and cannot read argv; if the
agent decrypts and passes argv across the interface, a compromised agent can
present a genuine envelope alongside substituted argv and the signature proves
nothing about what will run.

Closing it means the **executor** holds the payload-decryption key rather than
the agent — which is `ADR-0003` § 4's placement, decided when there was one
component. **Raised against `ADR-0003` and `ADR-0005` jointly**, because neither
is wrong alone: the placement was right for a single-process agent, and the
signature covers what a single-process agent needed it to cover.

## Residual risks

| Risk | Outcome |
|---|---|
| `RSK-9` | **Renewed, and narrowed in one dimension.** § 7 moves the NATS connection, the protocol parsing and the key material out of root, so a memory-safety defect in the network-facing component no longer yields the host. It does not narrow logical compromise: an attacker controlling the agent can ask the executor for root. The mechanism that would close that is § 13's second finding. Expiry unchanged: **2027-09-14, or P08** |

P07 accepts no new risk. `THR-52` and `THR-53` were recorded by `RFC 0003` and
their mitigations are §§ 4, 5 and 11 of this ADR.

## Invariant coverage

| Invariant | Where |
|---|---|
| `ARCH-EXEC-001` | §§ 2, 3, 8, 9 — **satisfied**; six limit categories, each with a value or a named owner, and deny-by-default for unsupported forms |
| `ARCH-EXEC-002` | § 10 — **satisfied**; `SIGTERM`, grace, `SIGKILL`, to the process group and not the immediate child |
| `ARCH-JOB-002` | §§ 4, 12 — **registered**; a refusal precedes any durable start, and the ordering is `ADR-0006` § 3's |
| `ARCH-JOB-004` | §§ 10, 13 — **registered**; an unkillable process is `Unknown` rather than a false terminal state |
| `ARCH-OBS-001` | § 11 — **registered in part**; what execution must never emit is decided here, the record's schema is **P08**'s |
| `ARCH-NATS-007` | § 8 — **registered**; the output limit fits inside `ADR-0002` § 9's payload bound |
| `ARCH-NATS-009` | § 8 — **registered**; concurrency of one is `ADR-0002` § 7's `MaxAckPending=1`, not a new limit |
| `ARCH-COMM-003` | Throughout — **registered**; no in-process substitute for execution is described |

P07 creates no new `ARCH-*` identifier.

## Alternatives considered

**One process running as root.** Rejected — § 7. It is simpler and it makes the
component parsing untrusted network input the component holding the host.

**An unprivileged agent with no privileged component.** Rejected. It cannot
become another account, so it cannot satisfy `RFC 0003`, and the deployments that
need a service account would fall back to sudoers entries — moving the privilege
decision outside the product where nothing records it.

**Resolving `argv[0]` against the harvested `PATH`.** Rejected — § 4. It is the
one case where a writable profile would decide which binary an operator's command
names, and it would give `THR-53` a reach its own mitigation says it does not
have.

**Killing a process that exceeds its output limit.** Rejected — § 9. It converts
a reporting limit into an execution outcome, and discarding already prevents the
exhaustion `THR-22` is about.

**Stripping secrets from argv before auditing.** Rejected — § 11. It cannot be
done correctly and would be believed.

**Waiting indefinitely for a process that ignores `SIGKILL`.** Rejected — § 10.
A job that never reaches a terminal state is worse than one honestly reported as
`Unknown`.

## Consequences

**Positive.** The network-facing component is unprivileged. A command runs as the
account it should, with that account's environment, and creates files that
account owns. Every limit has a value or a named owner. Truncation, fallback and
refusal are all reported rather than inferred.

**Negative.** Two components where there was one, which C07 and C13 both carry —
a local interface to define and a service account to create. The executor is
root and the split does not bound a logical compromise of the agent. A secret in
argv reaches the audit record.

**Neutral.** The harvest costs a process per command on accounts that have a
shell. Whether that matters is a C15 question, not a design one.

## What this ADR does not decide

The audit record's schema and retention (**P08**); how a refusal, a truncation
or a fallback is presented to an operator (**P09**); the acceptance harness and
its descendant-process test (**P10**); the executor, the local interface and the
signal handling in code (**C07**); the service account and the packaged units
(**C13**).

And it does not decide either finding in § 13: the lifecycle state for a
resource-limit termination belongs to `ADR-0006`, and the key placement that
would let the executor verify what it runs belongs to `ADR-0003` and `ADR-0005`.

## Validation

`make check`. Acceptance cases and their demonstrations are in
[`P07-acceptance-evidence.md`](../dossiers/P07-acceptance-evidence.md).

## References

- [ADR-0005 — Versioned encrypted protocol](0005-versioned-encrypted-protocol.md)
- [ADR-0006 — Delivery and job lifecycle](0006-delivery-and-job-lifecycle.md)
- [RFC 0003 — A caller-selected execution user](../rfcs/0003-caller-selected-execution-user.md)
- [PRODUCT-CHARTER.md](../project/PRODUCT-CHARTER.md)
- [THREAT-MODEL.md](../project/THREAT-MODEL.md)
