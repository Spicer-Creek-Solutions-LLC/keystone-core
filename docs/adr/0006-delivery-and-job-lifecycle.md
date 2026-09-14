# ADR-0006: Delivery and job lifecycle

- **Status:** Proposed
- **Date:** 2026-09-14
- **Task:** P06, bounded by [`docs/dossiers/P06.md`](../dossiers/P06.md)
- **Builds on:** [`ADR-0002`](0002-nats-native-capabilities.md), [`ADR-0004`](0004-subject-authorization.md), [`ADR-0005`](0005-versioned-encrypted-protocol.md)

## Context

`ADR-0002` decided how Keystone uses the broker, `ADR-0003` how an agent acquires
an identity, `ADR-0004` what each principal may say, `ADR-0005` what is inside
the message. **This decides what the system is entitled to believe, and what it
says when it cannot know.**

It is the first decision in this stage whose subject is time rather than
structure. The four before it can be wrong in a way a reader can see by
inspection. This one is wrong when a crash lands between two writes, and the
document that describes it looks identical either way.

### The one thing this ADR exists to prevent

`THR-36`: *the protocol claims exactly-once execution and the operator
over-trusts a reported outcome*. It is the only threat in the model whose
attacker is the document itself, and `ARCH-JOB-001` is its countermeasure.

Every convenience this ADR could offer — retrying an ambiguous job, treating the
broker's deduplication window as an execution guarantee, reporting a completed
job when what completed was delivery — is a way of writing that threat into the
product. The recurring answer below is to name the ambiguity rather than resolve
it, which is `ARCH-JOB-004`.

## Decision

### 1. The server state machine

The server's states are what an operator sees. The charter fixes the outcomes
(§ 5.3 to § 5.6); this enumerates the states that produce them.

| State | Meaning | Terminal |
|---|---|---|
| `Accepted` | A job identifier is assigned and durably recorded. The charter requires one for **every accepted job** | No |
| `Published` | The command envelope is in `KS_CMD` and the publisher holds its `PubAck` | No |
| `Delivered` | The broker has delivered the command to the agent's consumer at least once | No |
| `Running` | The agent reported a durable start | No |
| `Completed` | A verified terminal result is recorded, **including a non-zero remote exit status** | Yes |
| `Cancelling` | A cancellation has been accepted and the outcome is not yet established | No |
| `Cancelled` | **The command provably did not complete.** Either a verified cancelled result arrived, or the cancellation reached a job whose command was never published | Yes |
| `TimedOut` | The agent's deadline elapsed and the process tree was terminated | Yes |
| `Undelivered` | The command expired in `KS_CMD` having never been delivered. **It did not run** | Yes |
| `Unknown` | The control plane cannot prove whether the command ran | Yes |
| `Refused` | Rejected before publication — no envelope was written | Yes |

**Transitions.** Every row is a permitted transition; nothing else is.

| From | To | Trigger |
|---|---|---|
| `Accepted` | `Published` | `PubAck` received for the command envelope |
| `Accepted` | `Cancelled` | Cancellation of a job whose command was never published. No envelope exists, so nothing can execute |
| `Accepted` | `Refused` | Local validation or authorization rejects the request |
| `Accepted` | `Unknown` | The publish outcome cannot be established (§ 11) |
| `Published` | `Delivered` | A delivery is observed: an agent lifecycle event, or an acknowledgement-progress signal (§ 5) |
| `Published` | `Undelivered` | `KS_CMD` max-age expiry with **no** delivery recorded (§ 11) |
| `Published` | `Cancelling` | A cancellation is accepted (§ 9) |
| `Delivered` | `Running` | The agent's start event arrives |
| `Delivered` | `Unknown` | Redelivery is exhausted (§ 11), or the deadline elapses with no further signal |
| `Delivered` | `Cancelling` | A cancellation is accepted (§ 9) |
| `Running` | `Completed` | A verified terminal result arrives |
| `Running` | `Cancelling` | A cancellation is accepted (§ 9) |
| `Cancelling` | `Cancelled` | A verified cancelled result arrives |
| `Cancelling` | `Completed` | A verified terminal result arrives instead — the agent ran the command before the cancellation reached it (§ 9) |
| `Cancelling` | `TimedOut` | A verified timed-out result arrives instead |
| `Cancelling` | `Undelivered` | The command expires in `KS_CMD` having never been delivered |
| `Cancelling` | `Unknown` | The deadline plus the result grace period elapses with no verified result |
| `Running` | `TimedOut` | A verified timed-out result arrives |
| `Running` | `Unknown` | The deadline plus the result grace period elapses with no terminal result |

`Unknown` is reachable from three states and is terminal in all of them. That is
not a modelling convenience: ambiguity can arise before delivery, after delivery
and during execution, and collapsing those into one state is what lets the
operator be told one thing about three different situations. § 12 states what
each one means.

### 2. The agent state machine

| State | Meaning | Terminal |
|---|---|---|
| `Received` | The envelope verified and the receipt is durable (`ARCH-JOB-002`) | No |
| `Refused` | The envelope did not verify; `ADR-0005` § 9's refusal applies | Yes |
| `Started` | The start is durable and the process tree exists | No |
| `Exited` | The process exited; terminal state and result are durable | No |
| `Killed` | The job ended without completing — cancellation or the deadline terminated the process tree, or the deadline had already elapsed when the command was pulled and no tree was created. Any partial result is durable | No |
| `ResultPublished` | The result envelope is in `KS_RES` and the agent holds its `PubAck` | No |
| `Acked` | The JetStream acknowledgement is sent (`ARCH-JOB-005`) | Yes |
| `Indeterminate` | Recovery found a durable start with no durable terminal state | Yes |

| From | To | Trigger |
|---|---|---|
| `Received` | `Refused` | Verification fails |
| `Received` | `Started` | The start record is durable and the process tree launches |
| `Received` | `Killed` | Cancellation arrives before the process starts |
| `Received` | `Killed` | The command's deadline has already elapsed; no process is created (§ 7) |
| `Started` | `Exited` | The process exits and the result is durable |
| `Started` | `Killed` | Cancellation or the deadline terminates the process tree |
| `Started` | `Indeterminate` | Recovery after a crash (§ 10) |
| `Exited` | `ResultPublished` | `PubAck` received for the result envelope |
| `Killed` | `ResultPublished` | `PubAck` received for the result envelope |
| `ResultPublished` | `Acked` | The acknowledgement is sent |
| `Indeterminate` | `ResultPublished` | An `UNKNOWN` result is published for the job |

**Where the two machines are permitted to disagree, and who is authoritative.**

The agent ledger is authoritative for **whether the command ran**. The server is
authoritative for **what the operator is told**. These answer different
questions and there is no contradiction when they differ.

An agent at `Exited` whose result never reaches the server leaves the server at
`Unknown`. Both are correct: the command ran once, and the control plane cannot
prove it. The charter says so directly — *execution having occurred is not itself
proof that a result was retrieved* (§ 5.3).

The disagreement that would be a defect is the reverse: a server at `Completed`
with no agent record of execution. Nothing in § 1 permits it, because every path
into `Completed` requires a verified terminal result, which only the agent can
produce (`ADR-0005` § 4).

### 3. Durable boundaries

Stated as ordering obligations. What a ledger looks like is P08's, and its
schema is C02's.

| Before this happens | This must be durable |
|---|---|
| The command envelope is published | The job identifier and the accepted request |
| The process tree is created | The receipt: job identifier, authenticated envelope metadata, receipt state (`ARCH-JOB-002`) |
| The process tree is created | The start record, distinct from the receipt — see below |
| The result envelope is published | The terminal state and the result |
| The acknowledgement is sent | Everything above, **and** the result's `PubAck` (`ARCH-JOB-005`) |

**The receipt and the start record are separate writes, and that separation is
what makes restart decidable.** With one combined record, an agent recovering
from a crash cannot distinguish *received but never started* from *started and
crashed*, and must treat both as ambiguous — turning every crash before
execution into an `UNKNOWN` the system could have avoided. With two, receipt
without start proves the process was never created, which § 10 turns into the
only case where an agent may begin execution after a restart.

This is the one place where the design pays for precision with an extra write.
It is worth it because the alternative manufactures ambiguity, and `ARCH-JOB-004`
means manufactured ambiguity reaches the operator.

### 4. Stream and consumer configuration

`ADR-0002` § 7 and § 9 already fix the shapes. P06 does not re-decide them; it
states what this lifecycle requires of the values, and one of those requirements
is new.

| Setting | Fixed by | What this lifecycle requires |
|---|---|---|
| `KS_CMD` `FilterSubject` | `ADR-0002` § 7 — one exact agent subject | Unchanged |
| `MaxAckPending` | `ADR-0002` § 7 — `1` | Unchanged. One outstanding command per agent is what makes § 1's `Running` a single job |
| `MaxDeliver`, `BackOff` | `ADR-0002` § 7 — finite, explicit | Unchanged, and § 8 states what a redelivery may cause |
| `AckPolicy` | `ADR-0002` § 7 — explicit | Unchanged |
| `KS_CMD` max age | `ADR-0002` § 9 — short, hours | **It must exceed the longest accepted command deadline plus the result grace period**, or a running job's command can expire beneath it |
| `AckWait` | Not previously stated | **It must be shorter than the max age and longer than the progress interval of § 5** |

The max-age requirement is a constraint P06 discovers rather than invents:
`ADR-0002` § 9 justified a short max age as "a command older than its deadline is
not worth delivering", which is true of an *undelivered* command and not of a
delivered one still running. Both settings remain `ADR-0002`'s; this records the
relationship between them that only a lifecycle can see.

### 5. Acknowledgement and progress

**The acknowledgement is the last thing that happens**, after receipt, terminal
state, result durability and the result's `PubAck` (`ARCH-JOB-005`). Nothing
below weakens that.

A command that takes longer than `AckWait` would be redelivered while it is
still running. The agent therefore emits **acknowledgement progress** on the
same `$JS.ACK` subject its allowlist already permits (`ADR-0002` § 8), at an
interval shorter than `AckWait`.

**Progress is not a partial acknowledgement.** It resets the redelivery timer
and asserts nothing about the job: an agent that crashes after sending progress
has acknowledged nothing, and the command is redelivered exactly as if it had
sent none. `ARCH-JOB-005` permits this explicitly — *long operations emit
acknowledgement progress without changing this rule*.

**Progress does not extend `MaxDeliver`.** It prevents a redelivery from being
triggered; it does not increase how many may occur. An agent that is alive and
working therefore does not consume its redelivery budget, and an agent that is
failing does.

### 6. Publication deduplication

`Nats-Msg-Id` is set on every stored envelope (`ADR-0002` § 11, `ADR-0005` § 8),
derived from the job identifier and class.

**What it guarantees:** within the stream's duplicate window — two minutes by
default — a republication of the same envelope does not create a second stream
message. That makes a publisher's retry after an uncertain `PubAck` safe, which
is its entire purpose.

**What it does not guarantee:** anything after the window closes, and anything
about execution at any time. A command republished three minutes later produces
a second stream message and a second delivery, and the agent ledger is what
prevents a second execution (§ 7).

Treating the window as the execution guarantee would be claiming exactly-once
execution — `THR-36`, forbidden by `ARCH-JOB-001`. The window is a transport
optimisation whose failure mode is a duplicate the next layer must already
handle.

### 7. Application-ledger deduplication

**The agent's durable ledger is the authority, keyed on the job identifier, for
the life of the deployment** (`ARCH-JOB-003`).

On receiving a command the agent consults the ledger before anything else:

| Ledger says | The agent does |
|---|---|
| Nothing, and the command's deadline has elapsed | Record receipt, then `Killed` without creating a process. Publishes a deadline-exceeded result |
| Nothing | Record receipt, then start (§ 3) |
| `Received`, no start | Record the start and begin. This is the first attempt, not a second |
| `Started`, no terminal | **Does not execute.** Republishes an `UNKNOWN` result for the job (§ 12) |
| Terminal state recorded | **Does not execute.** Republishes the recorded result |
| `Refused` | Republishes the refusal |

**The deadline check is first, and it is what bounds § 9's offline
cancellation case.** A command that has outlived the deadline its operator gave
it is not worth running, and an agent that reconnects hours later would otherwise
execute a job whose outcome nobody is still waiting for. `ADR-0005` § 6 already
makes the timestamp a staleness bound; this is the lifecycle acting on it.

The fourth and fifth rows are the substance of `ARCH-JOB-003`: *redelivery of a
known command identifier may resume result delivery but may not automatically
execute the command again*. Resuming result delivery is exactly what the last
three rows do, and it is why redelivery is useful rather than merely survivable.

### 8. Redelivery

Finite `MaxDeliver` with explicit `BackOff` (`ARCH-NATS-010`, `ADR-0002` § 7).

**What a redelivery may cause:** a ledger lookup, and the republication of a
result the agent already holds.

**What it may not cause:** a second execution of a job the ledger knows about.
Every row of § 7 but the first refuses to execute, and the first row is reached
only when the ledger holds nothing for that job.

**The limit of that guarantee, stated because it is easy to overclaim.** An agent
whose ledger has *lost* an entry for a job it did execute is indistinguishable,
to itself, from an agent seeing that job for the first time — and it will
execute. `ARCH-JOB-003` makes the ledger authoritative; it cannot make the ledger
correct. So at-most-one-automatic-attempt holds exactly as far as the ledger's
durability holds, which is a property of § 3's writes and of the storage beneath
them (**C02**), and not of this protocol.

A ledger that is lost deliberately rather than by fault is `THR-48`, inside
`RSK-9`: a host that can erase its own ledger can also run the command as many
times as it likes without being asked, so this adds nothing to that attacker.

When `MaxDeliver` is exhausted the broker emits the terminal advisory
`$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.KS_CMD.<consumer>`, which the
monitoring role already subscribes to (`ADR-0002` § 10). § 11 states what the
server does with it.

### 9. Cancellation races

Cancellation is its own envelope class on its own subject (`ADR-0004`), signed
(`ADR-0005` § 4, RFC 0002), and idempotent — cancelling a cancelled job is a
no-op (`ADR-0005` § 6).

**Which path delivers a cancellation, which no prior ADR states.** The accepted
decisions provide two, and the difference between them is the whole of this
section:

| Path | Mechanism | Reaches | Survives an offline agent |
|---|---|---|---|
| **Live** | The agent's core-NATS subscription to `ks.job.<id>.cancel` (`ADR-0002` § 4; `ADR-0004` `POS-4`) | A connected agent, immediately | No. Core NATS is not retained |
| **Durable** | The same envelope stored in `KS_CMD` and pulled by the agent's consumer (`ADR-0002` § 7) | Any agent, eventually | Yes, but **behind the command in FIFO order** |

The live path is why cancellation during execution works at all. The agent's
consumer has `MaxAckPending=1` and `ARCH-JOB-005` withholds the acknowledgement
until the result is published, so a cancellation waiting in `KS_CMD` **cannot be
delivered while the command it cancels is running**. Only the core subscription
reaches a busy agent.

`ADR-0002` § 4 grants the subscription and `ADR-0004` tests it; neither says what
it is for. It is for this.

| When cancellation arrives | Outcome |
|---|---|
| **Before the command is delivered, agent connected** | `Cancelling`. The live path reaches the agent before it pulls; the ledger records the cancellation, § 7's check refuses the command when it is pulled, and the cancelled result moves the job to `Cancelled`. No execution |
| **Before the command is delivered, agent offline** | The job enters `Cancelling`. **The cancellation does not prevent execution**, and the job's terminal state is whatever the agent eventually reports. See below |
| **After receipt, before start** | `Cancelling`. No process exists. The agent moves `Received` → `Killed`, publishes a cancelled result, and never creates a process tree; the job reaches `Cancelled` |
| **During execution** | `Cancelling`. The complete process tree is terminated (`ARCH-EXEC-002`, P07's mechanics). The agent moves `Started` → `Killed` and publishes a cancelled result **carrying whatever partial output is durable**; the job reaches `Cancelled` |
| **After a terminal state** | A no-op for the job's state. The cancellation is recorded as an audit observation and the job stays in its terminal state |

**The offline case, stated as the limitation it is.** An agent that was not
connected when the cancellation was published missed the live path, and the
durable copy sits behind the command in one FIFO consumer. On reconnecting the
agent pulls the command first, its ledger holds no cancellation for that job, and
§ 7's first row starts it. The cancellation is delivered only after the command
is acknowledged, by which time the job is terminal and § 9's fourth row makes it
a recorded no-op.

**This is why `Cancelling` exists and `Cancelled` is not entered on request.**
The server records `Cancelling` when it accepts the cancellation, and the job's
terminal state is whatever the agent eventually proves: `Cancelled` if the
cancellation won, `Completed` or `TimedOut` if the command ran first. An operator
who cancels a job queued for an offline agent has changed what the control plane
is *trying* to do and not what the host will do, and the state name says so.

`Cancelled` therefore always means the same thing — **the command provably did
not complete** — with two proofs: a verified cancelled result, or a job whose
command was never published, where no envelope exists to execute. Both are
evidence. Neither is a request.

**What bounds it.** The agent refuses a command whose deadline has already
elapsed (§ 7), so the exposure is the command's own deadline rather than
`KS_CMD`'s max age — which § 4 requires to be the longer of the two. A job
cancelled while its agent is offline executes only if that agent reconnects
before the deadline it was given.

**How an operator sees this, settled after P06 by `G18`.** Journey § 5.6 stated
the observable effect of a cancellation as *the job reaches a terminal cancelled
state*, which under `Cancelling` holds when the cancellation wins and not when
the command ran first. The charter now makes that conditional, and gives the
operator the answer in the exit status: `keystone job cancel` exits `0` only when
the job reaches terminal **cancelled**, and otherwise exits with the job's own
outcome code. "I cancelled it and it ran anyway" is reported as an outcome rather
than left in the output.

**What would close it**, and why none of it is decided here. Making pre-delivery
cancellation authoritative needs one of: a **separate cancellation consumer**, so
cancellation is not ordered behind commands; **removal of the queued command**,
which needs a stream-message-delete permission the command publisher does not
hold (`ADR-0002` § 8); or a **pre-start liveness check**, which needs a
request/reply class `ADR-0004`'s grammar does not define. Each changes an
accepted ADR, so each is raised here and fixed by its owner — `ADR-0002` § 7 for
the first two, `ADR-0004` for the third.

**The crash boundary.** An agent that crashes between terminating the process
tree and making the cancelled result durable recovers to `Started` with no
terminal record, which § 10 resolves as `Indeterminate` and § 12 reports as
`Unknown`. It is not reported as `Cancelled`: the process tree's termination was
not durably observed, and `THR-25` is exactly the defect of reporting a terminal
state that a crash left indeterminate.

**A late result never contradicts a cancellation silently** (charter § 5.6). A
verified terminal result arriving after `Cancelled` does not change the job's
state. It is recorded, both observations are retained, and the operator can see
both — the same rule § 12 applies to `Unknown`, with the difference that
`Cancelled` is not superseded and `Unknown` is.

### 10. Restart

**Server restart.** In-flight jobs are read from the server's durable record.
For a job at `Accepted` or `Published`, the server may republish the command
envelope. That is safe and is worth stating because it looks unsafe: the
republication is deduplicated inside the window by `Nats-Msg-Id` (§ 6) and
outside it by the agent's ledger (§ 7), and the second mechanism is the one that
matters. A server that refused to republish would strand jobs whose `PubAck` it
never saw.

**Agent restart.** The ledger decides, by the same table as § 7:

| Recovery finds | The agent does |
|---|---|
| Receipt, no start record | Starts. The process was never created, so this is the first attempt |
| Start record, no terminal state | `Indeterminate`. Publishes an `UNKNOWN` result. **Never re-executes** |
| Terminal state, no `PubAck` | Republishes the result |
| Terminal state and `PubAck`, no acknowledgement | Acknowledges |

The second row is where § 3's separate writes are spent. Without them every
crash before execution would land here.

**What cannot be recovered on either side:** whether a process that was running
at the moment of the crash completed its work. Nothing in `v0.6.0` observes the
host independently of the agent, which is `RSK-9`.

### 11. Delivery failure and expiry

These are two outcomes, not one, and **they have different truth values.** The
broker mechanism that ends the job is what distinguishes them.

| | Expiry | Redelivery exhaustion |
|---|---|---|
| **What happened** | The command aged out of `KS_CMD` with no delivery recorded | The command was delivered `MaxDeliver` times and never acknowledged |
| **Broker signal** | Max-age expiry; no delivery advisory was ever emitted | `MAX_DELIVERIES` advisory (`ADR-0002` § 10) |
| **Did it run?** | **No.** A JetStream delivery count increments only when a consumer pulls. Nothing pulled, so nothing received it | **Unprovable.** The agent received it at least once and may have executed and crashed before acknowledging |
| **Server state** | `Undelivered` | `Unknown` |

**`Undelivered` is a distinct state because claiming ambiguity we do not have is
the same defect as claiming certainty we do not have.** `ARCH-JOB-004` requires
the control plane to report `UNKNOWN` when it cannot prove whether a command
ran. Here it can: a command that was never delivered was never executed, and
reporting `UNKNOWN` would understate what the system knows and send an operator
to investigate a host that never heard of the job.

**The asymmetry rests on one broker property**, stated here so a reviewer can
check it rather than take it: a pull consumer's delivery count increments on
delivery to a consumer, and an agent that is not pulling receives no deliveries.
An offline agent therefore exhausts max age, never `MaxDeliver`. If that property
did not hold, `Undelivered` would be unsound and both cases would be `Unknown`.

**How an operator sees this, settled after P06 by `G18`.** `Undelivered` reports
as **`11`**, which the charter defines as *the target agent did not take
delivery within the deadline*.

This ADR originally raised it as an unmapped state, on the reasoning that `11`
was "a pre-flight presence check before a job exists". **The charter never said
that** — it said *not present within the deadline*, and where the check happens
was this document's inference rather than the charter's text. `G18` corrected
the wording to describe the outcome rather than one of its causes, at which point
`Undelivered` was already covered: a command that expired undelivered is work
that never reached the host, which is what `11` now says.

### 12. `UNKNOWN`

**When it is reported.** The three paths of § 1, which are three different
situations:

| Path | What is unknown |
|---|---|
| `Accepted` → `Unknown` | Whether the command was published at all |
| `Delivered` → `Unknown` | Whether an agent that received it executed it |
| `Running` → `Unknown` | Whether an agent that started it finished |

**What it may never be rendered as.** Not success, not failure, not a retry
prompt. `ARCH-JOB-004` forbids hiding ambiguity by retrying arbitrary commands,
and the charter gives `UNKNOWN` its own exit code (`13`) precisely so it cannot
be folded into either neighbour.

**Late-result reconciliation.** A verified terminal result for a job in
`Unknown` supersedes it: the job's current state becomes the result's terminal
state, because the ambiguity is genuinely resolved and continuing to report
`UNKNOWN` would now be the inaccurate answer.

**Both observations are retained.** `ARCH-JOB-004` requires it — *a verified
late result may supersede that knowledge state; both observations remain in the
audit log*. The `UNKNOWN` was true when it was recorded, and an operator who
acted on it needs to see that it was reported. A reconciliation that overwrote
the earlier observation would make the audit record disagree with what the
operator was told, which is `THR-26`'s class.

**Supersession is one-way and requires verification.** Only a result that
verifies (`ADR-0005` § 4) supersedes, only `Unknown` is superseded, and nothing
supersedes a terminal state that was proven — a late result arriving for a
`Completed` job is a duplicate, and for a `Cancelled` job is § 9's recorded
observation.

### 13. The lifecycle table

Every server state by durability point, causer, operator-visible form and
terminality. **The codes are the charter's** (§ Exit codes), cited so that every
terminal state has a distinguishable outcome and none has to be inferred; P06
fixes no code and P09 decides how each is rendered.

| State | Durable when | Caused by | Operator sees | Terminal |
|---|---|---|---|---|
| `Accepted` | Before publication | The operator's request | The job identifier | No |
| `Published` | On `PubAck` | The command publisher | In flight | No |
| `Delivered` | On the first delivery signal | The broker and the agent | Delivered | No |
| `Running` | On the agent's start event | The agent | Running | No |
| `Completed` | On the verified result | The agent | The remote exit status and output; `0` | Yes |
| `Cancelling` | On the cancellation being accepted | An operator | Cancelling, with the outcome not yet established | No |
| `Cancelled` | On the verified cancelled result, or on cancelling an unpublished command | An operator, confirmed by the agent or by the absence of any envelope | Cancelled, the command did not complete; `14` | Yes |
| `TimedOut` | On the verified timed-out result | The deadline, via the agent | Deadline exceeded; `12` | Yes |
| `Undelivered` | On max-age expiry with no delivery | The broker | Never delivered, it did not run; `11` | Yes |
| `Unknown` | On the ambiguity being established | Absence of evidence | `UNKNOWN`, with which of § 12's three; `13` | Yes |
| `Refused` | Before publication | The server | Rejected, with a reason; `1` or `10` | Yes |

## Residual risks

| Risk | Outcome |
|---|---|
| `RSK-9` | **Renewed, and the reason is sharper than before.** § 3's durable boundaries and § 7's ledger strengthen what a **correct** agent can prove. `RSK-9` is about an agent that is not correct: a compromised host owns the ledger § 7 makes authoritative and signs results with the key § 2 trusts, so every mechanism added here is one the attacker controls. The server gains no independent witness. Attestation would be one and is outside Generation 2's promise (`CAP-IDENT-*`). **P06 makes this risk more precisely stateable without reducing it**, and the honest record of that is a renewal with an unchanged compensating control — bounded to one host by construction. Expiry moves to **P08**, which owns the audit record and is the next place an independent witness could plausibly appear |

## Invariant coverage

| Invariant | Where |
|---|---|
| `ARCH-JOB-001` | §§ 6, 8, 11 — **satisfied**; at-least-once delivery is documented and exactly-once execution is never claimed. `THR-36` is named in Context as the threat the ADR is written against |
| `ARCH-JOB-002` | § 3 — **satisfied**; receipt is durable before the process tree exists, and the start record is a separate write |
| `ARCH-JOB-003` | § 7 — **satisfied**; the ledger is the authority, and § 8 states that no redelivery may cause a second execution |
| `ARCH-JOB-004` | § 12 — **satisfied**; `UNKNOWN` is its own state with three distinguishable causes, and both observations survive reconciliation |
| `ARCH-JOB-005` | § 5 — **satisfied**; the acknowledgement follows the result's `PubAck`, and progress asserts nothing |
| `ARCH-NATS-009` | § 4 — **satisfied in part**; the exact-filter serialized consumer is `ADR-0002` § 7's. P06 states that `MaxAckPending=1` is what makes `Running` a single job |
| `ARCH-NATS-010` | § 8 — **satisfied**; redelivery is finite and its terminal advisory is § 11's input |
| `ARCH-EXEC-002` | § 9 — **registered**; the lifecycle states cancellation moves a job through are decided here, the process-tree mechanics are **P07**'s |
| `ARCH-EXEC-001` | § 9 — **registered**; a deadline's lifecycle outcome is `TimedOut`, the limits themselves are **P07**'s |
| `ARCH-OBS-001` | §§ 9, 12 — **registered**; every transition and both retained observations must reach the audit record, whose schema is **P08**'s |
| `ARCH-NATS-006` | § 2 — **registered**; every class sequenced here is signed (RFC 0002) and P06 adds no cryptographic requirement |

P06 creates no new `ARCH-*` identifier.

## Alternatives considered

**One `UNKNOWN` for both delivery failures.** Rejected — § 11. It is simpler and
it discards a fact the system holds. An operator told `UNKNOWN` for a command
that was never delivered will look for evidence on a host that never received
it.

**Treating `MaxDeliver` exhaustion as proof of non-execution.** Rejected, and it
is the tempting mirror of the above. Exhaustion proves the command was delivered
and not acknowledged, which is compatible with an agent that executed it and
crashed. This is the case `ARCH-JOB-004` exists for.

**A single combined receipt-and-start record.** Rejected — § 3. One write is
cheaper and makes every pre-execution crash ambiguous, manufacturing `UNKNOWN`
results the system could have avoided.

**Automatic retry of an ambiguous job.** Rejected. It is what `ARCH-JOB-004`
forbids in as many words, and it converts a reporting problem into a
double-execution problem for exactly the commands whose idempotence is unknown.

**Relying on the `Nats-Msg-Id` window for execution-once.** Rejected — § 6. It
is a two-minute transport optimisation being asked to carry a safety property,
and it is how a protocol ends up claiming exactly-once execution without anyone
deciding to.

**Acknowledging on receipt and publishing the result independently.** Rejected.
It simplifies `AckWait` handling and violates `ARCH-JOB-005`: the command would
be acknowledged before the result was durable, so a crash between the two loses
the result with the broker's copy already released.

## Consequences

**Positive.** Every terminal state is reached by evidence rather than by
timeout, except the two ambiguity states, which say so. The restart rules are
derivable from the durable boundaries rather than being a separate policy. An
operator who is told `UNKNOWN` can be told which of three situations produced it.

**Negative.** The agent writes twice before executing. Ambiguity is visible to
operators rather than smoothed over, so `v0.6.0` will report `UNKNOWN` in
situations a less careful system would report as failure. `RSK-9` is unchanged,
and the ledger this ADR makes authoritative is owned by the host whose
compromise that risk describes.

**Neutral.** `Undelivered` is a tenth server state that exists to carry one
distinction. It earns its place only if `ADR-0002` § 9's max age stays short
enough for expiry to be common.

## What this ADR does not decide

Execution limits, argv handling, the working directory and environment, and the
process-tree termination mechanics (**P07**); the audit record's schema and
retention, and the ledger's storage design (**P08**); the operator-facing
presentation of every state above, **including which exit code `Undelivered`
maps to** (**P09**); the fault matrix as an executable harness (**P10**);
migrations and the ledger implementation (**C02**); the transport adapter
(**C06**).

The `KS_CMD` max-age and `AckWait` relationships in § 4 are stated as
constraints on `ADR-0002` § 9's values, not as new values. C03 renders them.

**And it does not decide how to make a pre-delivery cancellation authoritative
for an offline agent** (§ 9). How both cases are *presented* was settled after
this ADR by `G18`, which amended the charter; how the underlying limitation is
*removed* still belongs to `ADR-0002` § 7 and § 8 or to `ADR-0004`. The three candidate mechanisms each change an
accepted ADR — a separate cancellation consumer or the removal of a queued
command belong to `ADR-0002` § 7 and § 8, a pre-start liveness check to
`ADR-0004`'s grammar. P06 states the limitation, bounds it with the deadline
check of § 7, and raises the rest.

## Validation

`make check`. Acceptance cases and their demonstrations are in
[`P06-acceptance-evidence.md`](../dossiers/P06-acceptance-evidence.md).

## References

- [ADR-0002 — NATS-native capabilities](0002-nats-native-capabilities.md)
- [ADR-0004 — Subject authorization](0004-subject-authorization.md)
- [ADR-0005 — Versioned encrypted protocol](0005-versioned-encrypted-protocol.md)
- [RFC 0002 — Every envelope class is signed](../rfcs/0002-signed-envelope-classes.md)
- [ARCHITECTURE-INVARIANTS.md](../project/ARCHITECTURE-INVARIANTS.md)
- [THREAT-MODEL.md](../project/THREAT-MODEL.md)
- [JetStream consumers](https://docs.nats.io/nats-concepts/jetstream/consumers)
