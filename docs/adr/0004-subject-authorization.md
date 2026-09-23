# ADR-0004: Subject authorization

- **Status:** Proposed
- **Date:** 2026-09-14
- **Task:** P04, bounded by [`docs/dossiers/P04.md`](../dossiers/P04.md)
- **Builds on:** [`ADR-0002`](0002-nats-native-capabilities.md), [`ADR-0003`](0003-enrollment-and-identity.md)

## Context

`ADR-0002` named every principal and the direction and scope it requires, and
said the canonical subject grammar was P04's. `ADR-0003` named the bootstrap
identity's two token-specific subjects the same way. **This is where those
deferrals come due**: every subject either gets an expression here or is found
unrepresentable, which is a finding rather than an omission.

`GLOSSARY.md` § Subject and its § "Not yet defined" table both assign the
grammar and the per-principal permission matrix to P04.

### What this ADR does not run

`REBOOT-EXECUTION-PLAN.md` § P04 is explicit, and it is repeated here because
the opposite assumption is the easy one: **P04 specifies, it does not generate.**
No Go module, build or test runner exists until P11, and P11 follows P00–P10.
C03 generates the configuration, signs the JWTs, and runs the Docker negative
identity matrix against them.

Every case in § 8 and § 9 is therefore a **statement of what must hold**, written
so C03 can implement each one. None is a test.

## Decision

### 1. The canonical subject grammar

Every Keystone subject has **exactly four tokens**:

```
ks.<plane>.<id>.<class>
```

| Token | Values | Meaning |
|---|---|---|
| `ks` | literal | Root. Distinguishes Keystone traffic from anything else sharing a broker |
| `<plane>` | `job`, `out`, `enroll` | Direction and lifecycle stage (§ 2) |
| `<id>` | agent identifier, or enrollment token identifier | Who or what the subject is scoped to |
| `<class>` | `cmd`, `cancel`, `result`, `event`, `presence`, `request`, `reply` | What the message is |

**No deployment token.** `ADR-0002` § 1 isolates deployments by account, and two
deployments sharing a broker occupy two accounts that cannot observe each other.
A deployment prefix inside that boundary would name something the boundary
already separates.

**`<id>` precedes `<class>`, and that ordering is load-bearing.** It is what
makes one filter per agent possible — see § 5.

#### The identifier token

`<id>` is constrained, because it is a subject token and a subject token that
can contain a `.` is a subject *structure* the supplier controls:

| Rule | Reason |
|---|---|
| Lowercase `a`–`z`, digits `0`–`9`, and `-` | NATS treats `.` as a separator and `*` and `>` as wildcards. Any of them in an identifier changes the subject's shape or its match set |
| Non-empty, at most 64 characters | Bounded so a subject cannot be used to consume broker resources by length |
| Unique within the deployment for the life of the identity | Two agents sharing an identifier share every subject scoped to it |
| **Server-assigned, never operator-supplied** | See the finding below |

> **Finding raised against `ADR-0003`, not fixed here.** `ADR-0003` § 1 records an
> **agent name** — the operator's `--agent-name` label, which the charter defines
> as a label and not a claim the product verifies. It does not establish a
> subject-safe **agent identifier**. This ADR's grammar requires one: if the
> operator's label were the `<id>` token, a label containing a `.` would silently
> create a subtree, and two agents given the same label would share every subject
> scoped to it.
>
> P04 decides the *constraint* on the token, which is the grammar's business. It
> cannot decide that `ADR-0003` assigns a conforming identifier, and `ADR-0003`
> does not say it does. `D-P04-5` requires this to be raised and stopped at.
> **Until it is resolved, this ADR's grammar rests on an identifier whose origin
> is unspecified.**

### 2. The three planes

| Plane | Direction | Contains |
|---|---|---|
| `job` | server → agent | `cmd`, `cancel` |
| `out` | agent → server | `result`, `event`, `presence` |
| `enroll` | bootstrap ↔ server | `request`, `reply` |

**`enroll` is a plane rather than a class, and that is the point.**
`ARCH-NATS-005` requires enrollment subjects to be unreachable after enrollment.
With a separate plane, that is **one line — `ks.enroll.>` — in the permanent
agent identity's template**, rather than one per class. Spread across classes it
would be several, and several rules are several chances to miss one.

**It is the agent template, not a deployment-wide rule, and the distinction
matters.** The enrollment service is itself a permanent identity and must reach
the enrollment plane for its whole life; a rule phrased as "every permanent
identity" would deny the one principal enrollment depends on. Only **permanent
agent** identities are excluded from the plane. Bootstrap identities hold exactly
one token's pair and expire (`ADR-0003` § 8).

### 3. Every subject class

| Subject | Carries | Published by | Subscribed by | Stream |
|---|---|---|---|---|
| `ks.job.<agent>.cmd` | Command envelope | Command publisher | That agent | `KS_CMD` |
| `ks.job.<agent>.cancel` | Cancellation envelope | Command publisher | That agent | `KS_CMD` |
| `ks.out.<agent>.result` | Result envelope | That agent | Result consumer, via its durable consumer | `KS_RES` |
| `ks.out.<agent>.event` | Agent lifecycle events | That agent | Monitoring role | None — core NATS |
| `ks.out.<agent>.presence` | Presence | That agent | Presence consumer | None — core NATS (`ADR-0002` § 7) |
| `ks.enroll.<token>.request` | Enrollment request | Bootstrap identity | Enrollment service | None — core NATS (`ADR-0003` § 3) |
| `ks.enroll.<token>.reply` | Permanent credential | Enrollment service | Bootstrap identity | None — core NATS |

`KS_CMD` is configured with subject `ks.job.*.>`; `KS_RES` with
`ks.out.*.result`. Nothing else is stored.

**Every role `ADR-0002` § 4 and `ADR-0003` § 3 named has an expression above.**
§ 10 maps them one by one, because a deferral that closes quietly is
indistinguishable from one that did not close.

### 4. The principal-by-subject permission matrix

Publish and subscribe per principal. **Once an allow list exists for a
direction**, NATS denies every subject not on it, so a principal may do what
appears here and nothing else.

**An EMPTY allow list is not a deny — NATS reads it as unrestricted**, and the
qualifier above is the whole of the difference. Two rows below give `none` in a
direction: the presence consumer and the monitoring role publish nothing. Until
`G46` this section said an omission already denied them, and it does not. The
rendering rule that makes `none` mean none is in § 7, and `C03-I2` found the gap
by measurement rather than by reading:

```
presence-consumer pub allow=[] deny=[]
errors after publishing a COMMAND as the presence consumer: []
```

That is `NEG-15`'s property — *"only the command publisher may publish
commands"* — false in a generated deployment. `ADR-0002` § 8 stated the
mechanism correctly all along; this section had dropped its qualifier.

| Principal | Publish | Subscribe | Scope |
|---|---|---|---|
| **Command publisher** | `ks.job.*.cmd`, `ks.job.*.cancel` | its own publish-acknowledgement inbox | Deployment. **Wildcard across agents, documented** — it must reach any enrolled agent (`ARCH-NATS-003`) |
| **Enrollment service** | `ks.enroll.*.reply` | `ks.enroll.*.request` | Deployment. **Wildcard across tokens, documented** — it must answer any outstanding token |
| **Result consumer** | none | none by subject; consumes `KS_RES` through its durable consumer (§ 6) | Deployment |
| **Presence consumer** | none | `ks.out.*.presence` | Deployment. **Wildcard across agents, documented** — presence is a fleet view |
| **Monitoring role** | none | `ks.out.*.event`, and the account's JetStream advisories (§ 6) | Deployment. **Wildcard across agents, documented** |
| **Agent** | `ks.out.<its own id>.result`, `ks.out.<its own id>.event`, `ks.out.<its own id>.presence` | `ks.job.<its own id>.>` | **Exactly one agent — its own.** No wildcard across identifiers |
| **Bootstrap identity** | `ks.enroll.<its own token>.request` | `ks.enroll.<its own token>.reply` | **Exactly one token.** No wildcard across identifiers |

**Denied to every permanent *agent* identity, and stated rather than left
implicit.** An agent holds a non-empty allow list in both directions, so for an
agent these denials do follow from the grants above — but they are written out
for the reader and as targets for § 9's cases, exactly as `ADR-0002` § 8 does.
**They are not left to follow for a principal whose list is empty**, which is
what § 7's rendering rule exists for.

| Denied | To whom | Why |
|---|---|---|
| `ks.enroll.>` | Permanent **agent** identities. **Not the enrollment service**, whose two grants above are its entire reason to exist, and not bootstrap identities, which hold one token's pair and expire | `ARCH-NATS-005` — enrollment subjects are unreachable after enrollment. One line in the agent template, because `enroll` is a plane (§ 2) |
| Any `ks.job.*` publish | Agents | `THR-09` — agents hold no publish permission on command subjects |
| Any other agent's `ks.job.<other>.>` or `ks.out.<other>.*` | Agents | `THR-08`, `THR-20` |
| `$SYS.>`, and every administrative `$JS.API` subject | Every Keystone identity | `ARCH-NATS-005` (`ADR-0002` § 8) |

**The four wildcards above are the whole of the documented exception**
`ARCH-NATS-003` allows. Every one is a service role, none is an agent, and each
carries its reason in its own row.

### 5. The consumer filter, and why the grammar is ordered as it is

`ARCH-NATS-009` requires one exact agent `FilterSubject`. `KS_CMD` carries both
`cmd` and `cancel` for every agent.

Had the grammar put the class before the identifier, a per-agent consumer would
need **two** filters — one ending in the command class and one in the
cancellation class, each naming the agent in its final token — and multi-filter
consumers must account for their authorization behaviour explicitly. Those
subjects are written here without backticks because they do not exist: no
subject of this system has that shape. With `<id>` before `<class>`:

```
FilterSubject: ks.job.<agent>.>
```

**One filter, exact as to the agent**, with a trailing wildcard inside that
agent's own prefix — the shape `ADR-0002` § 8 already permits and requires to be
justified. The justification is here: every subject it can match is addressed to
that agent, and a class added to that agent's `job` plane in future is a message
intended for that agent, so capturing it is correct rather than accidental.

**No multi-filter consumer exists in this design.** If one is later required, the
execution plan obliges it to account for its authorization behaviour, and this
section is where that account belongs.

### 6. The data-plane `$JS.API` and reply allowlist

`ADR-0002` § 8 fixed two entry shapes. Here they are in literal form. Each entry
substitutes one exact stream and one exact consumer, or one principal's own
inbox prefix.

| Principal | Shape | Permitted | Justification |
|---|---|---|---|
| Agent | Consumer-scoped | `$JS.API.CONSUMER.MSG.NEXT.KS_CMD.<its own consumer>` | Pull its own next command |
| Agent | Consumer-scoped | `$JS.ACK.KS_CMD.<its own consumer>.>` | Acknowledge. Trailing wildcard: the subject carries per-message tokens that cannot be enumerated. Bounded to one stream and one consumer |
| Agent | Principal-scoped | its own inbox prefix, `.>` | Receive pulled messages and publish-acks. Trailing wildcard: inbox tokens are per request |
| Command publisher | Principal-scoped | its own inbox prefix, `.>` | Receive the `PubAck` for each command published to `KS_CMD` |
| Result consumer | Consumer-scoped | `$JS.API.CONSUMER.MSG.NEXT.KS_RES.<its consumer>` | Pull results |
| Result consumer | Consumer-scoped | `$JS.ACK.KS_RES.<its consumer>.>` | Acknowledge, same justification |
| Result consumer | Principal-scoped | its own inbox prefix, `.>` | Same justification |
| Monitoring role | Subscribe only | `$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.KS_CMD.<consumer>`, `$JS.EVENT.ADVISORY.CONSUMER.MSG_TERMINATED.KS_CMD.<consumer>`, `$JS.EVENT.ADVISORY.API.LIMIT_REACHED` for this account | The terminal delivery signal `ARCH-NATS-010` requires (`ADR-0002` § 10) |

A bare `$JS.ACK.>`, a bare inbox wildcard, and every administrative `$JS.API`
subject are permitted to nobody.

### 7. Configuration and claim shapes

What C03 generates. Shapes, not files.

| Artifact | Shape |
|---|---|
| Account | One Keystone account; one system account, holding no Keystone principal (`ADR-0002` § 1) |
| Account signing key | Held by the server; the operator seed is held outside every Keystone process (`ADR-0002` § 2) |
| Service user JWT | One per service role of `ADR-0002` § 3, carrying that role's publish and subscribe lists from § 4 verbatim, and no others |
| Agent user JWT | Subject lists with `<its own id>` substituted; issued at enrollment against the agent's recorded NATS public key (`ADR-0003` § 6, S1) |
| Bootstrap user JWT | The two `ks.enroll.<token>.*` entries only, expiring with the token (`ADR-0003` § 3, § 8) |
| Revocation | **Not performed at runtime.** Bootstrap access ends by the server's refusal of the spent token and by the credential's expiry (RFC 0005). The account's revocation list is written only offline, with the operator key, by the generator or the broker administrator; the server's account signing key cannot sign it |

**A direction with no grants is rendered as an explicit deny of the whole
subject space.** § 4's table gives `none` to the presence consumer's and the
monitoring role's publish, and an empty allow list does not deny — so `none` is
generated as a deny of `>` rather than as an absent list. This is a rendering
rule, which is why it is here among the shapes rather than in § 4 among the
policy: the permissions are unchanged and what changes is how "nothing" is
written down.

**`C03-A`'s frozen surface covers one instance of this and not the rule.**
§ 9's fifteen negatives include `NEG-15`, a presence consumer publishing a
command; none covers a service principal reaching an administrative `$JS.API`
subject, which the same defect also permitted.
`C03-I2`'s package tests assert the property in both directions for every
principal the generator produces. Adding a sixteenth case would be an amendment
to an accepted contract and is not made here.

**Every JWT's permission lists are generated from § 4 and never hand-edited.**
A permission that exists in a deployed JWT and not in § 4 is a defect in
generation, which is what makes C03's matrix able to find it.

### 8. Positive authorization cases

What must succeed. Each names a principal, a subject, a direction and an
outcome, so C03 can implement each as one test.

| # | Principal | Subject | Direction | Must |
|---|---|---|---|---|
| `POS-1` | Command publisher | `ks.job.<any agent>.cmd` | publish | succeed |
| `POS-2` | Command publisher | `ks.job.<any agent>.cancel` | publish | succeed |
| `POS-3` | Agent | `ks.job.<its own id>.cmd` | subscribe | succeed |
| `POS-4` | Agent | `ks.job.<its own id>.cancel` | subscribe | succeed |
| `POS-5` | Agent | `ks.out.<its own id>.result` | publish | succeed |
| `POS-6` | Agent | `ks.out.<its own id>.presence` | publish | succeed |
| `POS-7` | Agent | `$JS.API.CONSUMER.MSG.NEXT.KS_CMD.<its own consumer>` | publish | succeed |
| `POS-8` | Agent | `$JS.ACK.KS_CMD.<its own consumer>.<tokens>` | publish | succeed |
| `POS-9` | Presence consumer | `ks.out.*.presence` | subscribe | succeed |
| `POS-10` | Result consumer | `$JS.API.CONSUMER.MSG.NEXT.KS_RES.<its consumer>` | publish | succeed |
| `POS-11` | Bootstrap identity | `ks.enroll.<its own token>.request` | publish | succeed |
| `POS-12` | Bootstrap identity | `ks.enroll.<its own token>.reply` | subscribe | succeed |
| `POS-13` | Enrollment service | `ks.enroll.*.request` | subscribe | succeed |
| `POS-14` | Monitoring role | `$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.KS_CMD.<consumer>` | subscribe | succeed |
| `POS-15` | Command publisher | its own inbox prefix, `.>` | subscribe | succeed — without it no `PubAck` is received (`ADR-0002` § 6) |
| `POS-16` | Enrollment service | `ks.enroll.*.reply` | publish | succeed |
| `POS-17` | Agent | `ks.out.<its own id>.event` | publish | succeed |
| `POS-18` | Agent | its own inbox prefix, `.>` | subscribe | succeed |
| `POS-19` | Monitoring role | `ks.out.*.event` | subscribe | succeed |
| `POS-20` | Result consumer | `$JS.ACK.KS_RES.<its consumer>.<tokens>` | publish | succeed |
| `POS-21` | Result consumer | its own inbox prefix, `.>` | subscribe | succeed |
| `POS-22` | Monitoring role | `$JS.EVENT.ADVISORY.CONSUMER.MSG_TERMINATED.KS_CMD.<consumer>` | subscribe | succeed |
| `POS-23` | Monitoring role | `$JS.EVENT.ADVISORY.API.LIMIT_REACHED` for this account | subscribe | succeed |

**§ 8 enumerates every grant in § 4 and § 6, and that completeness is the
point.** A positive list shorter than the permission list lets C03 generate a
JWT missing a granted permission and still pass every case — the authorization
would be too narrow, and nothing here would notice. Each row above is one grant
from § 4 or § 6; **no grant is intentionally left untested.**

**Count them.** § 4 grants fifteen permissions across seven principals and § 6
grants eleven more; § 8 carries twenty-three cases because several § 4 grants are
exercised by one case each and the agent's `ks.job.<its own id>.>` subscribe is
exercised twice, once per class it covers. The monitoring role's three advisory
subjects are three grants and three cases — `POS-14`, `POS-22`, `POS-23` — and
an earlier draft of this section claimed completeness while covering only the
first of them.

### 9. Negative authorization cases

What must be refused, **at the broker**, by the generated permissions and not by
application logic. `ARCH-TEST-002`'s five clauses are covered by name, and each
is marked.

| # | Principal | Subject | Direction | Must | `ARCH-TEST-002` |
|---|---|---|---|---|---|
| `NEG-1` | Agent | `ks.job.<any agent>.cmd` | publish | be denied | **an agent cannot publish commands** |
| `NEG-2` | Agent | `ks.job.<any agent>.cancel` | publish | be denied | an agent cannot publish commands |
| `NEG-3` | Agent | `ks.job.<another agent>.cmd` | subscribe | be denied | **cannot consume another agent's commands** |
| `NEG-4` | Agent | `ks.job.<another agent>.>` | subscribe | be denied | cannot consume another agent's commands |
| `NEG-5` | Agent | `ks.out.<another agent>.result` | publish | be denied | **cannot publish another agent's results** |
| `NEG-6` | Agent | `ks.out.<another agent>.result` | subscribe | be denied | cannot consume another agent's results |
| `NEG-7` | Revoked bootstrap identity | `ks.enroll.<its own token>.request` | publish | be denied — the connection itself must fail | **cannot use revoked bootstrap credentials** |
| `NEG-8` | Agent (permanent) | `ks.enroll.<any token>.request` | publish | be denied | enrollment subjects unreachable after enrollment (`ARCH-NATS-005`) |
| `NEG-9` | Agent (permanent) | `ks.enroll.>` | subscribe | be denied | as above; one deny covers the plane |
| `NEG-10` | Agent | `$SYS.>` | subscribe | be denied | **cannot access broker administration subjects** |
| `NEG-11` | Agent | `$JS.API.STREAM.DELETE.KS_CMD` | publish | be denied | administrative `$JS.API` is denied to every Keystone identity |
| `NEG-12` | Agent | `$JS.API.CONSUMER.MSG.NEXT.KS_CMD.<another agent's consumer>` | publish | be denied | consumer-scoped entries are bound to one consumer |
| `NEG-13` | Agent | `$JS.ACK.>` | publish | be denied | a bare acknowledgement wildcard is permitted to nobody |
| `NEG-14` | Command publisher | `ks.out.*.result` | subscribe | be denied | the command publisher has no result authority |
| `NEG-15` | Presence consumer | `ks.job.*.cmd` | publish | be denied | only the command publisher may publish commands |

**`NEG-7` is different in kind from the rest and is marked so C03 does not
implement it as a publish denial.** A revoked bootstrap identity fails at
connection, not at publish, because revocation removes the user from the
account. A test that connects successfully and is refused on publish has proved
something weaker than revocation.

**After RFC 0005, `NEG-7` proves the offline path.** The product never revokes a
bootstrap identity at runtime; one that is revoked was revoked with the operator
key, by the generator or the broker administrator. The case still holds, and
still matters: that is the emergency path for permanent identities.

### 10. Every deferral, closed

`ADR-0002` § 4 and `ADR-0003` § 3 named subjects by role. Each row is that role
and its expression, so a deferral that did not close is visible rather than
absent.

| Deferred by | Role named | Expression |
|---|---|---|
| `ADR-0002` § 4 | Command publisher publishes to "the command subject of any enrolled agent" | `ks.job.*.cmd`, and `ks.job.*.cancel` for cancellation |
| `ADR-0002` § 4 | Command publisher subscribes to "its own publish-acknowledgement inbox" | its own inbox prefix, § 6 |
| `ADR-0002` § 4 | Enrollment service subscribes to "the enrollment subject of an outstanding token" | `ks.enroll.*.request` |
| `ADR-0002` § 4 | Result consumer subscribes to "the result stream, through its own durable consumer" | `KS_RES`, subject `ks.out.*.result`, consumed per § 6 |
| `ADR-0002` § 4 | Presence consumer subscribes to "agent presence subjects" | `ks.out.*.presence` |
| `ADR-0002` § 4 | Monitoring role subscribes to "JetStream advisories for this account" | the three advisory subjects in § 6, plus `ks.out.*.event` |
| `ADR-0002` § 4 | Agent publishes "its own result, event and presence subjects" | `ks.out.<its own id>.result`, `.event`, `.presence` |
| `ADR-0002` § 4 | Agent subscribes to "its own command and cancellation subjects" | `ks.job.<its own id>.>` |
| `ADR-0002` § 4 | Bootstrap identity publishes "its own token-scoped enrollment subject" | `ks.enroll.<its own token>.request` |
| `ADR-0002` § 4 | Bootstrap identity subscribes to "its own token-scoped reply subject" | `ks.enroll.<its own token>.reply` |
| `ADR-0003` § 3 | Bootstrap publishes "its own token-specific enrollment request subject" | same as above |
| `ADR-0003` § 3 | Bootstrap subscribes to "its own token-specific reply subject" | same as above |

**No role named by either ADR is unrepresentable in this grammar.**

## Invariant coverage

| Invariant | Where |
|---|---|
| `ARCH-NATS-003` | §§ 1, 4, 5 — **satisfied** as design. The automated evidence lands with the implementation, per `REQUIREMENTS-TRACEABILITY.md` |
| `ARCH-NATS-005` | § 4, § 6 — **satisfied jointly with P02** as design owner; C03 carries the evidence. See the plan's recorded, open question on that invariant's wording |
| `ARCH-TEST-002` | § 9 — **satisfied as design owner**; all five clauses are named cases. C03 executes them against generated production JWTs |
| `ARCH-NATS-002` | § 4 — **registered**; the principal list is `ADR-0002` § 3 |
| `ARCH-NATS-004` | § 3, § 4 — **registered**; enrollment semantics are `ADR-0003`'s and are unchanged here |
| `ARCH-NATS-009` | § 5 — **registered**; the consumer shape is `ADR-0002` § 7, and this ADR states its authorization consequence |

## Alternatives considered

**`ks.<plane>.<class>.<id>`, class before identifier.** Rejected. It forces a
two-filter consumer per agent, against `ARCH-NATS-009`'s one exact filter, and
turns a documented wildcard exception into a structural requirement.

**A deployment token in the subject.** Rejected. `ADR-0002` § 1 isolates
deployments by account; a prefix would name what the account boundary already
separates, and would invite treating the subject as the isolation mechanism.

**Enrollment as a class on the agent plane.** Rejected. `ARCH-NATS-005` requires
enrollment subjects unreachable after enrollment; as a separate plane that is
one deny (`ks.enroll.>`), and as a class it is several — several rules being
several chances to miss one.

**Operator-supplied `--agent-name` as the `<id>` token.** Rejected, and the
reason is a finding rather than a preference: a label containing a `.` would
create a subtree, and a reused label would merge two agents' subjects. See § 1.

**Wildcards for agents.** Rejected outright. Every wildcard here belongs to a
service role, which is the whole of `ARCH-NATS-003`'s documented exception.

## Consequences

**Positive.** One filter per agent. One deny closes the enrollment plane. Every
wildcard is a service role with its reason in its own row. Every deferral from
two prior ADRs is closed in § 10, visibly.

**Negative.** The grammar depends on an agent identifier whose origin
`ADR-0003` does not specify — recorded as a finding in § 1 and unresolved.
Fixed four-token arity means a future subject class must fit the shape or
require an amendment.

**Neutral.** No deployment token means a deployment's subjects look identical
across deployments, which is harmless while accounts isolate them and would
matter if that ever stopped being true.

## What this ADR does not decide

Envelope format, signing and encryption, and `RSK-12` (**P05**); job lifecycle
and `UNKNOWN` semantics (**P06**); audit record schema (**P08**); which
operators may do what (**P09**); **generated configuration, signed JWTs, and the
executed negative identity matrix (C03)**; and whether `ADR-0003` should assign
a subject-safe agent identifier — raised in § 1, and its own task.

## Validation

`make check`. Acceptance cases and their demonstrations are in
[`P04-acceptance-evidence.md`](../dossiers/P04-acceptance-evidence.md).

## References

- [NATS authorization](https://docs.nats.io/running-a-nats-service/configuration/securing_nats/authorization)
- [NATS decentralized JWT authentication](https://docs.nats.io/running-a-nats-service/configuration/securing_nats/auth_intro/jwt)
- [JetStream consumers](https://docs.nats.io/nats-concepts/jetstream/consumers)
- [ADR-0002 — NATS-native capability decisions](0002-nats-native-capabilities.md)
- [ADR-0003 — Enrollment and identity](0003-enrollment-and-identity.md)
