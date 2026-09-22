# ADR-0002: NATS-native capability decisions

- **Status:** Proposed
- **Date:** 2026-09-13
- **Task:** P02, bounded by [`docs/dossiers/P02.md`](../dossiers/P02.md)
- **Supersedes:** nothing. First Generation 2 ADR.

## Context

RFC 0001 § "NATS-first architecture" requires that before Keystone adds a
messaging, authorization, durability, flow-control or broker-observability
mechanism, the native NATS capability is evaluated and recorded as `Adopt`,
`Evaluate`, `Defer` or `Reject` with evidence. This is the first messaging ADR,
so it is the first exercise of `ARCH-NATS-008`.

It decides configuration inside constraints already fixed elsewhere, and does
not reopen them:

- RFC 0001 fixes the six cumulative security layers, the required initial
  capability set, and the deferred set.
- [`ARCHITECTURE-INVARIANTS.md`](../project/ARCHITECTURE-INVARIANTS.md) fixes
  account isolation, unique role identities, least-privilege subjects, broker
  API isolation, independent cryptographic layers, bounded resources,
  serialized per-agent consumption and finite redelivery.
- [`THREAT-MODEL.md`](../project/THREAT-MODEL.md) is the specification this ADR
  answers. Every threat naming `ACT-4`, `ACT-5`, `ACT-6` or `ACT-8` is a threat
  about the broker.
- [`PRODUCT-CHARTER.md`](../project/PRODUCT-CHARTER.md) § 5 fixes the seven
  journeys every stream and subject exists to carry.

RFC 0001 closes that section with a constraint this ADR is most likely to
violate, so it is repeated rather than cited:

> P02-P06 decide the safe configuration and protocol details inside these
> constraints; they do not merely ratify an implementation selected in advance.

### On subjects

**This ADR names subjects by role and never as literal strings.**
`GLOSSARY.md` § Subject and its § "Not yet defined" table assign the canonical
subject grammar and the principal-by-subject permission matrix to **P04**. What
P02 fixes is the principal list, the direction and scope each principal
requires, and the consumer shapes P04 writes its matrix against.

`$JS.*` subjects appearing below are **NATS system subjects**, not Keystone
subjects. They are reproduced literally because `ARCH-NATS-005` requires the
data-plane allowlist to be enumerated, and because their shape is fixed by NATS
rather than chosen by this project.

## Decision

### 1. Accounts

Two accounts, as `ARCH-NATS-001` requires.

| Account | Holds | Reachable by |
|---|---|---|
| The **Keystone account** | Every Keystone stream, consumer, service identity and agent identity; the deployment's JetStream storage | Service roles and agents (§ 3) |
| The **system account** (`$SYS`) | Broker administration, server events, and account-level monitoring | The broker operator only. No Keystone principal holds credentials in it |

One deployment occupies one Keystone account. Two deployments sharing a broker
occupy two accounts and cannot observe each other, which is what makes `THR-30`
a provisioning defect rather than an unavoidable exposure.

### 2. Operator and JWT trust

Decentralized operator/account/user JWT, as RFC 0001's required set names.

| Key | Held by | Can |
|---|---|---|
| Operator seed | The broker's administrator, **outside every Keystone process** | Mint accounts, and therefore any identity on the broker |
| Keystone account signing key | The **server** (`TD-SRV`) | Mint user JWTs **inside the Keystone account only** |
| User JWTs | Each service role and each agent | Exactly what § 4 grants them |

**The server holds an account signing key and never the operator seed.** This
is the decision with the widest consequence in this ADR, and it is made for
containment rather than convenience: enrollment (P03) requires the server to
mint a permanent agent identity, so it must hold *something*. Holding the
account signing key means a compromised server can mint identities **within its
own account** and cannot create accounts, reach `$SYS`, or touch another
deployment's account.

> **Finding raised against P01, not fixed here.** `THR-44`–`THR-46` enumerate a
> compromised server issuing commands, decrypting results and fabricating audit.
> They do not enumerate **identity minting**: a compromised `TD-SRV` holding the
> account signing key can mint agent identities at will inside the Keystone
> account, which is a distinct power from publishing on existing ones. It falls
> inside `RSK-4`'s stated scope — "compromise of the server is total within the
> product" — but is not among the threats that scope was derived from. P02's
> allowed paths keep the threat model closed except for three residual-risk
> rows, so this is recorded and raised rather than edited in.

### 3. Principals

`ARCH-NATS-002` requires distinct identities for each role, co-located or not.

| Principal | Kind | Count |
|---|---|---|
| Command publisher | Service | One |
| Enrollment service | Service | One |
| Result consumer | Service | One |
| Presence consumer | Service | One |
| Monitoring role | Service | One |
| Agent | Per host | One per enrolled agent |
| Bootstrap identity | Per enrollment, short-lived | One per outstanding token; P03 owns its lifecycle |

Co-location is a deployment fact, not an identity fact: a single server process
holding four service credentials is still four principals, and a compromise that
yields one does not yield the others.

### 4. Subject roles — direction and scope

What each principal requires. **P04 turns this into the canonical grammar and
the permission matrix**; the scope column is what forbids a wildcard across
agent identifiers (`ARCH-NATS-003`).

| Principal | Publishes to | Subscribes to | Scope |
|---|---|---|---|
| Command publisher | The command subject of any enrolled agent | Its own publish-acknowledgement inbox | Deployment |
| Enrollment service | Nothing on the agent data plane | The enrollment subject of an outstanding token | Per token |
| Result consumer | Nothing | The result stream, through its own durable consumer | Deployment |
| Presence consumer | Nothing | Agent presence subjects | Deployment |
| Monitoring role | Nothing | JetStream advisories for this account (§ 10) | Deployment |
| Agent | Its own result, event and presence subjects | Its own command and cancellation subjects | **Exactly one agent — its own** |
| Bootstrap identity | Its own token-scoped enrollment subject | Its own token-scoped reply subject | **Exactly one token** |

No principal both publishes and subscribes on the agent data plane except the
agent itself, and the agent's every entry is scoped to itself. An agent holds
no publish permission on any command or cancellation subject, which is what
makes `THR-08`, `THR-09` and `THR-20` broker-enforced rather than conventions.

### 5. What P04 writes its matrix against

P04 receives, and does not need to reopen P02 to obtain: the principal list of
§ 3; the direction and scope of § 4; the stream and consumer names and shapes
of § 7; the allowlist of § 8; and the limits of § 9.

### 6. Response permissions — `Defer`

NATS `allow_responses` lets a service reply to a request without holding a
broad publish permission: the server records the reply subject it handed out
and permits exactly that one reply.

**Keystone's data plane is not request/reply.** Commands and results are
published to JetStream and acknowledged; presence is published. Reply subjects
are genuinely required in exactly two places, and both are handled by explicit
per-principal inbox entries in § 8 rather than by a dynamic response grant:

1. **Pull consumption** — the consumer supplies an inbox for
   `CONSUMER.MSG.NEXT` to deliver into.
2. **Publish acknowledgement** — a JetStream publisher implicitly creates an
   inbox, and the server returns the `PubAck` to it. **A publisher that cannot
   receive its `PubAck` cannot distinguish a stored message from a lost one**,
   which for the command publisher would make the charter's "a job identifier is
   returned for every accepted job" (§ 5.3) unprovable at the moment it is
   claimed.

An explicit entry is **narrower** than `allow_responses` because it is fixed at
provisioning time and testable by P04 against a generated JWT, rather than
granted dynamically by the server at request time.

Deferred rather than rejected: if P09's operator API or a later service acquires
a genuine request/reply surface, `allow_responses` is the native mechanism and
should be reconsidered then, with its `max` and `expires` bounds set.

### 7. Streams and durable consumers

Two streams.

| Stream | Carries | Retention | Published by | Consumed by |
|---|---|---|---|---|
| **`KS_CMD`** | Command and cancellation envelopes, one subject per target agent | `Limits` | Command publisher only | One durable pull consumer per agent |
| **`KS_RES`** | Result envelopes, one subject per originating agent | `Limits` | Agents only, each on its own subject | The result consumer, through one durable pull consumer |

Presence is **not** a stream. It is core NATS publish/subscribe, because the
charter requires presence to be *observed, never inferred* (§ 5.2): a durable
replay of a presence message is a statement about the past, and serving it as
current state is exactly the false positive `THR-07` is about.

**Per-agent command consumers.** Each agent's consumer has one exact
`FilterSubject` — that agent's own command subject — and `MaxAckPending=1`, as
`ARCH-NATS-009` requires. Serialization is per agent by construction rather than
by convention, and no multi-filter consumer is used, so the authorization
behaviour of multi-filter consumers does not arise.

**Redelivery is finite.** Each command consumer sets a finite `MaxDeliver` and
an explicit `BackOff` schedule (`ARCH-NATS-010`), with `AckPolicy` explicit.
When `MaxDeliver` is exhausted the broker emits a terminal advisory (§ 10).
Redelivery repairs transport failure; it never authorizes a second logical
execution attempt, which stays the agent ledger's decision (`ARCH-JOB-003`).

**Only the server publishes commands, and only agents publish results.** That
asymmetry decides § 9's limits and is why the two streams are bounded
differently.

### 8. Data-plane `$JS.API` and reply allowlist

`ARCH-NATS-005` names P02 and P04 jointly: P02 enumerates, P04 tests the
enumeration against generated production JWTs. NATS allow-lists are exclusive —
once an allow list exists, every subject not on it is denied — so this list is
the whole of what a Keystone identity may reach in the JetStream API.

Entries come in **two shapes**, and the difference is not cosmetic:

- **Consumer-scoped** — consumer-next and acknowledgement. Each substitutes one
  exact stream and one exact consumer.
- **Principal-scoped** — a publisher's publish-acknowledgement inbox. It names
  that principal's own inbox prefix and **no stream or consumer**, because a
  `PubAck` is returned to the publisher and belongs to no consumer (§ 6).

Nothing in either shape wildcards a stream, consumer, agent or principal
identifier.

| Principal | Shape | Permitted | Why |
|---|---|---|---|
| Agent | Consumer-scoped | `$JS.API.CONSUMER.MSG.NEXT.KS_CMD.<its own consumer>` | Pull its own next command |
| Agent | Consumer-scoped | `$JS.ACK.KS_CMD.<its own consumer>.>` | Acknowledge. **Trailing wildcard justified:** the subject carries per-message tokens — delivery count, stream sequence, consumer sequence, timestamp — that cannot be enumerated ahead of time. Bounded to one stream and one consumer |
| Agent | Principal-scoped | Its own reply inbox prefix, `.>` | Receive pulled messages and publish-acks. **Trailing wildcard justified:** inbox tokens are generated per request. Bounded to that agent's own prefix |
| Command publisher | Principal-scoped | Its own reply inbox prefix, `.>` | Receive the `PubAck` for each command it publishes to `KS_CMD`. **Trailing wildcard justified:** inbox tokens are generated per request. Bounded to the command publisher's own prefix |
| Result consumer | Consumer-scoped | `$JS.API.CONSUMER.MSG.NEXT.KS_RES.<its consumer>` | Pull results |
| Result consumer | Consumer-scoped | `$JS.ACK.KS_RES.<its consumer>.>` | Acknowledge, same justification |
| Result consumer | Principal-scoped | Its own reply inbox prefix, `.>` | Same justification |

**Denied to every Keystone identity**, and stated explicitly. For a principal
holding a non-empty allow list in that direction the exclusive list already
denies them, and the repetition is for the reader and for P04's negative tests
rather than for the broker. **For a principal with no list in that direction it
is not repetition but the rule itself**: an empty allow list is read as
unrestricted, so the deny has to be rendered — `ADR-0004` § 7, corrected at
`G46` after `C03-I2` measured a presence consumer publishing a command. **The
principals this paragraph covers least well are the ones with no PUBLISH grants
at all** — `ADR-0004` § 4 gives the presence consumer and the monitoring role
`none` there — because it is an empty publish list, not an absent `$JS.API`
entry, that fails to exclude the administrative subjects below. The enrollment
service and bootstrap identities reach no `$JS.API` subject either and are
unaffected: both publish elsewhere, so their lists exist and exclude. Hence:

- all of `$SYS`;
- every administrative `$JS.API` subject — stream and consumer create, update,
  delete, list, purge, and account information;
- any `$JS.API` or inbox subject belonging to another agent or another
  deployment;
- enrollment subjects, after enrollment completes (`ARCH-NATS-004`).

A bare `$JS.ACK.>` or a bare inbox wildcard is **not** permitted to anyone. The
difference between that and the entries above is the difference between "any
message on this broker" and "any message for this one consumer".

### 9. Limits

`ARCH-NATS-007` prohibits unbounded streams and consumers. Values are starting
points a deployment may tighten and C03 must render into configuration; the
requirement is that none is absent or infinite.

| Scope | Limit | Value | Reason |
|---|---|---|---|
| Keystone account | Max connections | Fleet size + service roles + headroom | One connection per agent, four per server role |
| Keystone account | Max subscriptions per connection | Small fixed bound | An agent needs its command subject, its cancellation subject and its inbox; each publishing service role needs its publish-acknowledgement inbox |
| Keystone account | Max payload | 1 MiB | The charter bounds captured output (`ARCH-EXEC-001`); a payload larger than this is a defect, not a workload |
| Keystone account | Max JetStream storage | Explicit byte cap | `ARCH-NATS-007` |
| `KS_CMD` | Max age | Short — hours | A command older than its deadline is not worth delivering |
| `KS_CMD` | Max msgs, max bytes | Explicit | — |
| `KS_RES` | Max age, max msgs, max bytes | Explicit | — |
| `KS_RES` | **Max messages per subject** | Explicit, small | **This is what bounds one agent's contribution to shared storage.** One subject per agent means the per-subject limit is a per-agent limit, natively |
| Every consumer | `MaxAckPending` | `1` on command consumers | `ARCH-NATS-009` |
| Every consumer | `MaxDeliver`, `BackOff` | Finite, explicit | `ARCH-NATS-010` |

### 10. Advisories

The monitoring role subscribes, inside the Keystone account, to:

| Advisory | Used for |
|---|---|
| `$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.KS_CMD.<consumer>` | The terminal delivery signal `ARCH-NATS-010` requires. A command that exhausted redelivery is an outcome the control plane must record, not a silence |
| `$JS.EVENT.ADVISORY.CONSUMER.MSG_TERMINATED.KS_CMD.<consumer>` | Explicit termination, distinguished from exhaustion |
| `$JS.EVENT.ADVISORY.API.LIMIT_REACHED` for this account | Limits being hit is an operational fact the operator needs before it becomes a denial |

These are **JetStream advisories inside the Keystone account**, not `$SYS`
events. `ARCH-NATS-001` places *broker administration* in the system account;
it does not require a Keystone deployment to give up visibility of its own
streams, and no Keystone principal holds a `$SYS` credential (§ 1).

### 11. Headers

`Nats-Msg-Id` is set on every published envelope and enables JetStream's
publish-side duplicate detection within the stream's duplicate window, whose
default is two minutes.

**The window is a transport optimisation and is never the authority.** RFC 0001
is explicit that the application ledger remains authoritative after the window
expires, and `ARCH-JOB-003` makes the agent's durable ledger the thing that
permits at most one automatic attempt. A design that relied on the dedup window
for execution-once would be claiming exactly-once execution, which
`ARCH-JOB-001` forbids.

No Keystone secret, token or payload plaintext appears in any header
(`ARCH-OBS-001`, `THR-27`).

### 12. TLS

TLS on every connection, terminating **at the broker**.

That is a connection control and not payload secrecy. The broker, and anyone
administering it, sees every byte TLS protects in transit. Payload
confidentiality against `ACT-5` comes from Keystone's own encryption to the
intended recipient (`ARCH-NATS-006`, `THR-10`, `THR-21`), which this ADR does
not design — P05 does. Stating it here because the opposite assumption is the
single most likely misreading of a TLS-everywhere decision.

### 13. Credential rotation, and what it cannot reach

Broker-side, a user JWT is revoked by the account it belongs to, and the
revocation is a broker operation the server can perform with its account
signing key (§ 2).

**It reaches the NATS identity and nothing else.** P01 § 8 records that
revoking an agent's NATS identity does not revoke that agent's Keystone
signing key (`AST-4`) or payload-decryption key (`AST-5`), and that no
revocation mechanism exists for the service signing key (`AST-7`) or the
result-decryption key (`AST-8`) at all. Removing the NATS identity removes the
*publish path*, which contains a stolen Keystone key without invalidating it —
`RSK-1` and `RSK-7`. Designing that mechanism is P03's and P05's.

### 14. Deployment modes

| Mode | Shape | Constraint it satisfies |
|---|---|---|
| Single broker, single account | One NATS server, one Keystone account, one server, N agents | The baseline |
| Isolated networks | Server and agents on networks with **no direct reachability**, sharing only the broker | `ARCH-COMM-002`; acceptance places them this way |
| Shared broker, separate accounts | Another workload on the same broker, in a different account | `ARCH-NATS-001`; `THR-30` |

No clustering, no superclusters, no leaf nodes, and **no high-availability
commitment** — `ROADMAP.md` is explicit, and `RSK-2` records the consequence.

### 15. Capability matrix

`ARCH-NATS-008`. One row per relevant capability, each with evidence rather
than a restatement of the verdict.

#### RFC 0001's required initial set

| Capability | Verdict | Evidence |
|---|---|---|
| Accounts | `Adopt` | Isolation between deployments is a broker-enforced boundary; § 1, and `THR-30`'s mitigation depends on it |
| NKeys / JWT | `Adopt` | `ARCH-NATS-002` needs a distinct identity per agent; decentralized JWT is how NATS issues identities at fleet scale without a shared secret |
| Exact subject permissions | `Adopt` | Allow-lists are exclusive and deny beats allow, so an enumerated allow list is a closed boundary rather than a filter; § 4, § 8 |
| TLS | `Adopt` | Connection control. Explicitly *not* payload secrecy — § 12 |
| JetStream streams | `Adopt` | Durability between publisher and a disconnected agent is the product's core requirement; § 7 |
| Durable consumers | `Adopt` | An agent that reconnects must resume where it stopped, not replay from zero; § 7 |
| Explicit acknowledgements | `Adopt` | `ARCH-JOB-005` acknowledges only after receipt, terminal state and durable result; only explicit acks express that |
| `Nats-Msg-Id` deduplication | `Adopt` | Publish-side duplicate suppression within the window; the ledger remains authoritative after it — § 11 |
| Bounded stream and account limits | `Adopt` | `ARCH-NATS-007`; § 9, where the per-subject limit is what makes one agent's storage bounded |
| System account | `Adopt` | Separating administration from the data plane; § 1 |
| Advisories | `Adopt` | `ARCH-NATS-010`'s terminal delivery signal has no application-level equivalent; § 10 |

#### RFC 0001's deferred set

| Capability | Verdict | Evidence |
|---|---|---|
| Response permissions | `Defer` | The data plane is publish/subscribe with acknowledgement, not request/reply; both reply surfaces — pull consumption and publish acknowledgement — are handled by narrower per-principal inbox entries fixed at provisioning time — § 6 |
| Key/Value (KV) | `Defer` | Presence is observed rather than stored (§ 7) and job state is the server's durable store (P08). No requirement is currently unserved |
| Subject mappings | `Defer` | No requirement to rewrite subjects. Mapping would also move authorization-relevant routing out of the permission matrix P04 tests, which is a reason to be slow rather than fast here |
| Object Store | `Defer` | Output is bounded by `ARCH-EXEC-001` and fits a payload; large-artifact transfer is not a v0.6.0 journey |
| Queue groups | `Reject` | They distribute work across a group. `ARCH-NATS-009` requires the opposite — one exact `FilterSubject` per agent — and a command must reach one named host, not any available worker |
| Leaf nodes | `Defer` | Isolated deployment is served by § 14 without topology extension. Leaf nodes would add a trust boundary the threat model does not contain |
| Superclusters | `Defer` | No HA or geographic commitment exists to require one; `ROADMAP.md` |

## Residual risks

`RSK-2`, `RSK-3` and `RSK-10` name P02 as their expiry.

| Risk | Outcome | Reason |
|---|---|---|
| `RSK-2` — the broker administrator can withhold or delay any message | **Renewed** | Nothing in this ADR changes it. Keystone does not own the broker, and a mediated design cannot exclude its operator. Withholding still produces `UNKNOWN` rather than false success (`ARCH-JOB-004`) |
| `RSK-3` — the NATS operator can mint any Keystone identity | **Renewed, narrowed** | Still true of the operator seed, which is above the product by construction. Narrowed by § 2: the seed is held outside every Keystone process, so this is the broker administrator's authority rather than the server's |
| `RSK-10` — one agent may consume broker limits shared with the fleet | **Resolved** | § 7 and § 9. Agents cannot publish to `KS_CMD` at all, so the command path is not floodable by an agent. On `KS_RES`, one subject per agent plus an explicit **maximum messages per subject** bounds each agent's stored messages independently on shared storage — a native limit rather than an invented one |

## Invariant coverage

| Invariant | Where |
|---|---|
| `ARCH-NATS-001` | § 1 — satisfied |
| `ARCH-NATS-002` | § 3 — satisfied |
| `ARCH-NATS-003` | § 4 — **registered**; the grammar, the matrix and the executable policy are P04's |
| `ARCH-NATS-004` | § 3, § 8 — **registered**; enrollment staging is P03's |
| `ARCH-NATS-005` | § 8 — satisfied jointly with P04, which tests the enumeration |
| `ARCH-NATS-006` | § 12 — **registered**; key separation constrains § 2, the envelope is P05's |
| `ARCH-NATS-007` | § 9 — satisfied |
| `ARCH-NATS-008` | § 15 — satisfied |
| `ARCH-NATS-009` | § 7 — satisfied |
| `ARCH-NATS-010` | § 7, § 10 — satisfied |
| `ARCH-COMM-002` | § 14 — **registered**; acceptance that proves it is C-stage |

## Alternatives considered

**Per-agent command streams instead of per-agent consumers on one stream.**
Rejected. It would isolate storage per agent, but agents cannot publish to the
command stream at all, so the isolation buys nothing against the threat that
motivated it — and it makes enrollment a stream-provisioning operation, putting
administrative JetStream API access on the enrollment path that § 8 exists to
deny.

**One stream for both commands and results.** Rejected. The two have opposite
publishers, opposite consumers, and opposite limit pressures; a shared stream
would need a permission model that lets agents publish into the same stream
they consume from.

**JetStream for presence.** Rejected. Durability makes a stale presence message
replayable as current state, which is the false positive `THR-07` and the
charter's "observed, never inferred" both forbid.

**The server holding the operator seed.** Rejected. It would make server
compromise equal to broker compromise, collapsing `RSK-3` and `RSK-4` into one
another and removing the account boundary that limits `THR-30`.

## Consequences

**Positive.** Every authorization boundary in this design is broker-enforced
rather than conventional. `RSK-10` closes on a native limit. The blast radius of
a compromised server is bounded by an account rather than by the broker.

**Negative.** The server holds an account signing key, which is a real power and
a new one to state (§ 2). Per-agent consumers mean per-agent provisioning, which
P04 and C03 must generate rather than hand-write. `MaxAckPending=1` bounds
per-agent throughput by design.

**Neutral.** Deferring KV, Object Store, subject mappings and leaf nodes costs
nothing now and leaves each reconsiderable with a stated trigger.

## What this ADR does not decide

The canonical subject grammar and the principal-by-subject permission matrix
(**P04**); enrollment staging, bootstrap credentials and the permanent identity
handover (**P03**) — noting that **any principal P03 introduces which publishes
to a stream needs its own publish-acknowledgement inbox entry**, for the reason
in § 6; envelope format, signing and encryption, including the
cancellation-envelope gap `RSK-11` records (**P05**); job lifecycle and
`UNKNOWN` semantics (**P06**); the subject grammar, the permission matrix and
the authorization cases they must satisfy (**P04**); and **generated**
configuration, signed JWT fixtures and the executed negative identity matrix
(**C03**, from P04's specification — no build exists before P11).

## Validation

`make check`. The acceptance cases and their demonstrations are in
[`P02-acceptance-evidence.md`](../dossiers/P02-acceptance-evidence.md).

## References

- [NATS authorization](https://docs.nats.io/running-a-nats-service/configuration/securing_nats/authorization)
- [NATS decentralized JWT authentication](https://docs.nats.io/running-a-nats-service/configuration/securing_nats/auth_intro/jwt)
- [JetStream streams](https://docs.nats.io/nats-concepts/jetstream/streams)
- [JetStream consumers](https://docs.nats.io/nats-concepts/jetstream/consumers)
- [JetStream pull request subject](https://docs.nats.io/reference/jetstream/api/consumer/get-next)
- [JetStream advisories](https://docs.nats.io/reference/jetstream/advisory)
- [JetStream publishing and `PubAck`](https://docs.nats.io/learn/jetstream/publishing)
