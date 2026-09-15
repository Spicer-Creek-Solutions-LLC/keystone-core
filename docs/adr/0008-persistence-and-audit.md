# ADR-0008: Persistence and audit

- **Status:** Proposed
- **Date:** 2026-09-15
- **Task:** P08, bounded by [`docs/dossiers/P08.md`](../dossiers/P08.md)
- **Builds on:** [`ADR-0003`](0003-enrollment-and-identity.md), [`ADR-0005`](0005-versioned-encrypted-protocol.md), [`ADR-0006`](0006-delivery-and-job-lifecycle.md), [`ADR-0007`](0007-safe-execution.md)

## Context

Every decision before this one deferred the record. `ADR-0003` said enrollment's
schema is P08's, `ADR-0005` said the audit record's is, `ADR-0006` stated durable
boundaries as ordering obligations and said what a ledger looks like is P08's,
`ADR-0007` said what execution must never emit and left the record to P08.

`ARCH-OBS-001` is the only invariant in this stage that no earlier task could
satisfy, for exactly that reason.

**What makes this ADR hard is not the schema.** It is that three accepted risks —
`RSK-1`, `RSK-4`, `RSK-9` — all name the record as their compensating control,
and each leans on a record that one of the others controls. § 12 states what this
ADR can and cannot do about that.

## Decision

### 1. Two stores, and no abstraction over them

| Store | Belongs to | Holds |
|---|---|---|
| **Server store** | The server | Agents, enrollment, jobs, results, audit |
| **Agent ledger** | One agent, on one host | That host's receipts, starts, terminal states and results |

They are separate SQLite databases with separate schemas and separate APIs.
**There is no interface over both**, and none may be introduced.

The execution plan says so directly and the reason is worth keeping: the two
stores hold different things for different readers. The server holds fleet state
and the operator's record; the agent holds one host's proof of what it did. The
only property they share is the engine. An interface over both would make that
coincidence look like a design, and every later change would have to be expressed
through it.

### 2. The server store

| Table group | Holds | Retention |
|---|---|---|
| Agents | Identity, recorded public halves (`ADR-0003` § 4), enrollment state | Life of the agent |
| Enrollment | Token identifiers, issuance and denial records | § 7's audit floor |
| Jobs | Job identifier, correlation identifier, target, accepted request, lifecycle state (`ADR-0006` § 1) | § 7 |
| Results | Terminal state, remote exit status, captured output, truncation flag | § 7 |
| Audit | The append-oriented record of § 6 | § 7's audit floor |

The **accepted request** includes argv and the execution user, which is what
journey § 5.7 returns and what `THR-52`'s control depends on.

### 3. The agent ledger, and why it is independently readable

The ledger holds exactly what `ADR-0006` § 3's ordering obligations require:

| Record | Written before |
|---|---|
| Receipt — job identifier, authenticated envelope metadata, receipt state | The process tree is created |
| Start — distinct from the receipt | The process tree is created |
| Terminal state and result | The result envelope is published |
| Result `PubAck` | The acknowledgement is sent |

**The ledger's schema is documented and an operator with host access can read it
without the server.** This is a decision, not an accident, and it changes what
`RSK-4` rests on.

`RSK-4`'s compensating control has been that agent-side ledgers permit a forensic
reconstruction when the server's account is false — *"but the operator reaches
those ledgers only through the server"*. That qualifier existed because nothing
said the ledger was readable any other way. It now is: SQLite with a published
schema, at a documented path, readable by root on the host.

**What that buys and what it does not.** It removes the compromised party from
the path: an operator investigating a suspect server can read the agents directly
rather than through it. It does nothing for `RSK-9` — a compromised *host* owns
that ledger, and reading it directly reads whatever the attacker left. The two
risks remain distinct and § 12 keeps them so.

**The cost is a compatibility surface.** A documented on-disk format is one other
people will read, and C02 inherits the obligation not to break it silently. That
is the price of the control, and it is worth stating rather than discovering.

### 4. Filesystem ownership and modes

Against `ADR-0007` § 7's two components.

| Path | Owner | Mode | Why |
|---|---|---|---|
| Server store | The server account | `0600` | Nothing else on the host needs it |
| Server store directory | The server account | `0700` | — |
| Agent ledger | The **agent** account, not the executor | `0600` | The agent writes it; the executor never touches it |
| Agent ledger directory | The agent account | `0700` | — |

**The executor holds root and does not hold the ledger.** It creates processes
and owns process groups; it has no reason to read or write durable state, and
giving it one would widen the component whose whole purpose is to be small.

Root can read any of these, which is what makes § 3's independent reading
possible and is not a weakening: root on the host is already inside `RSK-9`.

### 5. Migrations and transaction boundaries

**Migrations are ordered, forward-only, and recorded in the store they change.**
Each store migrates independently — a consequence of § 1 that is easy to lose:
there is no combined version, and no migration spans both.

A partially applied migration leaves the store at its last complete step. The
next start resumes from there; it does not roll forward past a step that failed.

**Transaction boundaries, stated against `ADR-0006` § 3's orderings:**

| Ordering | One transaction? |
|---|---|
| Job identifier and accepted request, before publication | Yes |
| **Receipt, and start** | **No — two transactions, deliberately** |
| Terminal state and result | Yes |
| Result `PubAck` | Its own write |

**The receipt and the start must not share a transaction**, and this is the
schema consequence of `ADR-0006` § 3's separate writes. If they commit together,
a crash between them is indistinguishable from a crash before either, and the
recovery rule that lets an agent start a command it never started collapses into
`Indeterminate`. The separation only exists if the commits are separate.

### 6. The audit record

One append-oriented table. A record is written once and never updated.

| Field | From |
|---|---|
| Timestamp | The server's clock |
| Job identifier, correlation identifier | `ADR-0005` § 3 |
| Actor | Who caused it — operator, agent, broker, or the deadline |
| Target | Which agent |
| Action | The transition (`ARCH-OBS-001`'s list) |
| Result | The outcome, including `UNKNOWN` |

**Every transition `ARCH-OBS-001` names has a record**: enrollment,
authorization denial, receipt, start, cancellation, timeout, completion, unknown
outcome, and result retrieval.

**What append-oriented storage proves, and what it does not.** It proves that
Keystone's own code paths do not update a record after writing it, which makes an
accidental overwrite a bug rather than a routine operation. **It does not prove
that the record was not changed.** The server owns the file and can rewrite it;
anything holding the server's account can do the same. Append-only is a
discipline, not evidence.

That distinction is the whole of § 13's deferral, and it is stated here so that
nobody later reads "append-only" as the tamper-evidence it is not.

### 7. Retention, and the one way `ARCH-JOB-003` can lapse

`ARCH-JOB-003` makes the agent ledger authoritative for at-most-one automatic
execution. **A ledger entry that has expired is a job the agent would execute
again**, so retention is the one mechanism that can break the invariant.

The answer is an inequality, not a duration:

```
ledger identifier retention  >  KS_CMD max age  >  longest deadline + result grace
```

The right-hand inequality is `ADR-0006` § 4's. The left is this ADR's: **a job
identifier's tombstone outlives the longest time its command can survive in the
stream**, so a redelivery can never arrive for a job the ledger has forgotten.

That separates two retentions:

| What | Retained for | Bounded by |
|---|---|---|
| **Job identifier tombstone** | Long | Must exceed `KS_CMD` max age |
| Receipt, start, result, output | Shorter, deployment-set | Storage |
| Audit records | Deployment-set, with a floor | § 12's reconstruction |

**Both observations of a reconciled `UNKNOWN` share the audit floor**
(`ADR-0006` § 12). Retention may not discard either, because the pair *is* the
record — an `UNKNOWN` reported and a later result that superseded it are two
facts, and keeping only the second makes the audit disagree with what the
operator was told.

### 8. Results and truncation

Captured output is stored with the result, bounded by `ADR-0007` § 8's limit,
which is itself bounded by `ADR-0002` § 9's payload limit.

**Truncation is stored as a fact, not inferred from a length.** The result
carries that truncation occurred, which stream, and how much was discarded
(`ADR-0007` § 9). A reader must never have to compare a length against a limit to
find out.

### 9. Sensitive data

| Item | Stored? |
|---|---|
| argv, and the execution user | **Yes** — journey § 5.7 returns the action, and `THR-52`'s control is that the choice is recorded |
| The harvested login environment | **Never** (`RFC 0003`, `ADR-0007` § 5) |
| Payload plaintext beyond the result | **Never** (`ADR-0005` § 8) |
| Any NATS credential, seed, or private key | **Never** |
| Envelope headers | Only the routing-safe set `ADR-0005` § 8 fixes |

**The argv limit, restated as a storage consequence.** `ADR-0007` § 11 records
that a secret placed in argv is a secret placed in the audit record, because argv
is operator-supplied and structurally opaque. Storing it durably is what makes
that permanent: the record is the thing that outlives the job. Retention bounds
how long, and nothing bounds whether.

### 10. Backup and corruption

**Backup is the deployment's**, and the ADR states only what a backup must
capture to be useful: the server store whole, since a partial copy of an audit is
worse than none.

**Corruption response differs by store, and the difference follows from what each
store is for:**

| Store | On detecting corruption |
|---|---|
| **Agent ledger** | **Fail closed.** The agent stops accepting commands. It cannot prove at-most-one without the ledger, and executing anyway is the one thing `ARCH-JOB-003` forbids |
| **Server store** | Refuse new jobs; keep serving reads that succeed. In-flight jobs become `Unknown` when their deadline passes, which is the honest state |

An operator is told which store, that it is corrupt, and what was refused as a
result. **A corrupt store is never repaired silently**, because a repair that
guessed would be manufacturing exactly the record this ADR exists to make
trustworthy.

### 11. What an operator can retrieve

Journey § 5.7's correlated records for a job: actor, target, action, result and
correlation identifier, **in order**. The store returns them; how they are
presented is **P09**'s.

Ordering is by the server's clock and, within a clock tick, by insertion. This is
an audit trail rather than a distributed ordering: it records what the server
learned and when, not a global truth about what happened first.

## Residual risks

| Risk | Outcome |
|---|---|
| `RSK-1` | **Renewed.** Joint possession of the publisher credential and `AST-7` still yields accepted commands, and § 6's append discipline does not change what such a party can do to the record afterwards — it owns neither key nor store, but anything holding the server's account can rewrite the audit. P08 makes accidental loss a bug; it does nothing about deliberate rewriting. **2027-09-14, or P10** |
| `RSK-4` | **Renewed, with a stronger compensating control.** § 3 publishes the agent ledger's schema and path, so an operator investigating a false server reads the agents **without the server in the path** — the qualifier this risk carried since P01 is gone. It is still a renewal: a compromised server can still lie to the operator who has not thought to go and look, and the reconstruction still fails where the agent is also compromised. **2027-03-13, or P10** |
| `RSK-9` | **Renewed, unchanged by this ADR.** § 3's independent readability reads whatever a compromised host left; § 5's separate transactions and § 7's tombstones strengthen what a correct agent proves. Both are mechanisms the attacker owns on a compromised host. **2027-09-14, or P10** |

**All three now expire at P10**, which owns the acceptance harness — the first
task that could demonstrate the reconstruction of § 3 actually working, rather
than asserting it.

**Corrected at `G22`: the second half of that sentence is wrong.** P10 designs
the harness and nothing runs until P11, which follows P10 — so P10 can specify
the probe and cannot perform it. The expiry gate stands and P10 still disposes
of all three; what it cannot do is produce the evidence this paragraph promised.
The task that can is a C-stage one, named when P10 renews them. The decision
this ADR made is unchanged.

## Invariant coverage

| Invariant | Where |
|---|---|
| `ARCH-OBS-001` | § 6 — **satisfied**; every named transition has a record, and § 9 keeps secrets out of it |
| `ARCH-JOB-002` | §§ 3, 5 — **satisfied in part**; the ordering is `ADR-0006` § 3's, the store that makes it good is here, and § 5 states the transaction boundary that preserves it |
| `ARCH-JOB-003` | § 7 — **satisfied in part**; the ledger is the authority and the tombstone inequality is what stops retention lapsing it |
| `ARCH-JOB-004` | § 7 — **registered**; both observations survive retention, and § 10 makes an unprovable in-flight job `Unknown` rather than guessing |
| `ARCH-JOB-005` | § 5 — **registered**; the acknowledgement follows the result's `PubAck`, which is its own durable write |
| `ARCH-NATS-007` | § 8 — **registered**; stored output fits inside `ADR-0002` § 9's bound |
| `ARCH-COMM-003` | Throughout — **registered**; no in-memory substitute for a store |

P08 creates no new `ARCH-*` identifier.

## Alternatives considered

**One store with a shared interface.** Rejected — § 1, and the execution plan
forbids it. The two stores share an engine and nothing else.

**Receipt and start in one transaction.** Rejected — § 5. It is one fewer write
and it destroys the distinction `ADR-0006` § 3 pays for, turning every
pre-execution crash into `Indeterminate`.

**A single retention period.** Rejected — § 7. Output is large and uninteresting
after a while; a job identifier is small and must outlive the stream. One period
either keeps output too long or forgets identifiers too soon, and the second
breaks `ARCH-JOB-003`.

**Repairing a corrupt ledger automatically.** Rejected — § 10. A repair that
guessed would manufacture the record this ADR exists to make trustworthy.

**Hash-chaining the audit now.** Rejected — § 13, and it is the tempting one. A
chain the server holds is a chain the server can rebuild, so it would add
ceremony without adding evidence.

**Keeping the agent ledger's format private.** Rejected — § 3. It would leave
`RSK-4`'s control depending on the party that risk is about.

## Consequences

**Positive.** `RSK-4`'s compensating control no longer routes through the
compromised party. Retention cannot silently lapse `ARCH-JOB-003`. Truncation and
corruption are recorded facts rather than inferences.

**Negative.** A documented on-disk format is a compatibility surface C02 must not
break. Two stores mean two migration paths. The agent fails closed on ledger
corruption, which converts a storage fault into an outage — deliberately, because
the alternative is executing without proof.

**Neutral.** Append-oriented storage costs nothing and proves less than its name
suggests, which is why § 6 says so twice.

## What this ADR does not decide

How records are presented to an operator, including ordering shown and error
text (**P09**); the acceptance harness that would demonstrate § 3's
reconstruction (**P10**); whether a lint can enforce § 1's separation past C02
(**P11**); migrations, SQL and the implementation (**C02**); emitting the records
(**C12**); creating accounts and directories (**C13**).

### Deferred decision points, each with a trigger

| Deferred | Taken up when |
|---|---|
| **Tamper-evidence** anchored outside the deployment | An external operator asks whether the audit can be trusted against the server that produced it, **or** `RSK-1` or `RSK-4` reaches an expiry with no other mitigation available |
| **Postgres** in place of SQLite on the server | Concurrent operator access or fleet size exceeds what one SQLite writer serves, measured at C15 |

Neither is a roadmap entry. Both are recorded here so that the absence is a
decision with a condition attached rather than an omission.

## Validation

`make check`. Acceptance cases and their demonstrations are in
[`P08-acceptance-evidence.md`](../dossiers/P08-acceptance-evidence.md).

## References

- [ADR-0006 — Delivery and job lifecycle](0006-delivery-and-job-lifecycle.md)
- [ADR-0007 — Safe execution](0007-safe-execution.md)
- [PRODUCT-CHARTER.md](../project/PRODUCT-CHARTER.md)
- [THREAT-MODEL.md](../project/THREAT-MODEL.md)
- [ARCHITECTURE-INVARIANTS.md](../project/ARCHITECTURE-INVARIANTS.md)
