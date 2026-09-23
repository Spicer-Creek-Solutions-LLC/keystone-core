# ADR-0003: Enrollment and identity

- **Status:** Proposed
- **Date:** 2026-09-13
- **Task:** P03, bounded by [`docs/dossiers/P03.md`](../dossiers/P03.md)
- **Builds on:** [`ADR-0002`](0002-nats-native-capabilities.md)
- **Amended by:** [RFC 0005](../rfcs/0005-bootstrap-access-ends-by-refusal-and-expiry.md),
  §§ 1, 2, 6, 7 and 8 — bootstrap access ends by refusal and expiry, not
  revocation, and the token carries no separate secret

## Context

Enrollment is the only key transition Generation 2 specifies.
[`THREAT-MODEL.md`](../project/THREAT-MODEL.md) § 8 says so plainly: everything
after it is undesigned. `ARCH-NATS-004` is the invariant this ADR owns, and it
fixes the shape of the protocol before any design begins — one-use,
token-scoped credentials; a staged, idempotent sequence; fsync and atomic rename
at mode `0600`; proof of a permanent connection; revocation, and **verification**
of that revocation; short bootstrap expiry as the backstop; and a defined
crash-recovery path at every stage. **That was the invariant this ADR was
written against.** RFC 0005 replaced its revocation clauses with refusal and
expiry, after the server's key proved unable to revoke; the sections below are
amended to match.

The charter fixes the operator surface (§ 5.1) and defers three questions here:
whether the token is also accepted on standard input, the file's required mode,
and its lifetime.

`ADR-0002` fixes the ground this stands on: two accounts, decentralized JWT, the
server holding an **account signing key** and never the operator seed, and the
bootstrap identity as a principal whose lifecycle P03 owns.

### The circularity, and what resolves it

`ARCH-COMM-002` forbids assuming the server and agent can reach each other
directly, and `ADR-0002` makes NATS the only transport. **So an un-enrolled
agent can reach the server only over NATS — and needs a NATS credential before
it has an identity.**

`ARCH-NATS-004`'s phrase *"one-use, token-scoped NATS credentials"* is the
resolution, and it decides more of this ADR than any other sentence: **the thing
the operator carries to the host is not a password, it is a credential bundle.**
It contains a bootstrap NATS credential minted by the server, scoped to
token-specific subjects and nothing else, plus the one-use token itself.

That reframes the charter's existing rule. A bundle that must not appear in a
process listing is not a convenience constraint; it is a credential file, and
this ADR treats it as one.

**It is also the agent's trust anchor, and that is a second thing.** An agent
must *verify* a service-signed command and *encrypt* a result to the result
service (`ADR-0005` §§ 4, 5), which needs `AST-7`'s and `AST-8`'s public halves.
Enrollment is the only moment the agent has any channel to the server at all,
and the bundle is the only part of it that arrives **out of band** — carried by
an operator rather than over the broker. So the halves travel in the bundle.

Nothing else could carry them. A half delivered *over* the enrollment exchange
would be one the agent has no way to check, because checking it is what the half
is for.

### On subjects

Subjects are named **by role** and never as literal strings. The canonical
grammar and the permission matrix are P04's. `$JS.*` and `_INBOX` subjects are
NATS system subjects, not Keystone subjects.

## Decision

### 1. Token creation

`keystone enroll create --agent-name <name>` causes the server to, in one
durable step:

| Step | Result |
|---|---|
| Generate a one-use token | A high-entropy random identifier. **Opaque, not a JWT** — a self-validating token cannot be made one-use without server state, so the server keeps the state and the token carries no claims. **It has no separate secret** (RFC 0005 § 2): the bootstrap credential's seed is the secret, the bundle is the only place it exists, and the broker authenticates it before any byte reaches the server |
| Mint a bootstrap NATS credential | A user JWT in the Keystone account, signed with `AST-16`, scoped to this token's subjects only (§ 3), expiring with the token |
| Record a pending enrollment | Token identifier, agent name, expiry, state `pending`, and the bootstrap identity's public key. The agent's own keys do not exist yet |
| Add the service public halves | The **service envelope-signing public half** (`AST-7`) and the **result-service encryption public half** (`AST-8`). Both are public; neither is a secret |
| Emit the bundle **once** | To standard output, never to a file the server chooses |
| Emit an audit record | Issuance is a lifecycle transition (`ARCH-OBS-001`) |

Exit codes are the charter's: `0`, `10` when the operator is not authorized to
create a token, `1` on local or usage error. *Which* operators may create tokens
is P09's.

### 2. Token handling — the charter's three deferrals

| Question | Decision | Why |
|---|---|---|
| Standard input? | **Yes.** `--token-file -` reads the bundle from standard input | `THR-01` is the token being read from shell history or a process listing, and the charter forbids supplying it in a form that exposes it. A pipe satisfies that as well as a file and avoids the bundle touching disk at all, which is strictly better. Neither form ever places the bundle's credential in `argv` |
| Required mode | **`0600`, enforced.** The agent **refuses** a bundle file that is group- or world-readable, and exits `1` | A bundle is a credential. Refusing is a decision, not a warning: a warning that is ignored leaves a NATS credential world-readable on every host in the fleet |
| Lifetime | **The agent removes the bundle after the permanent identity is active.** Server-side the token expires on a short default — minutes, not hours — configurable at creation, enforced by the server | Two independent limits: the file stops existing once it is useless, and the token stops working whether or not the file was removed |

`--token-file` itself is fixed by P00 and is not reopened here.

### 3. The bootstrap credential and its subjects

The bootstrap identity lives in the **Keystone account** (`ADR-0002` § 1), and
holds exactly two permissions:

| Direction | Scope |
|---|---|
| Publish | Its own **token-specific enrollment request subject** |
| Subscribe | Its own **token-specific reply subject** |

Nothing else. No stream, no `$JS.API` subject, no other token's subjects, no
agent subject.

**The reply subject is token-specific and predictable, not a random inbox.**
That is deliberate, and it is what lets `ADR-0002` § 6 keep deferring response
permissions: a dynamic response grant exists to let a service reply into an
inbox it could not have known in advance, and here the server knows the reply
subject because it minted the token that names it.

Enrollment is **core NATS request/reply, not JetStream**. It needs no
durability: an enrollment that is lost is retried by the agent against the same
token, and § 6 makes that safe. So no principal introduced here publishes to a
stream, and none needs a publish-acknowledgement inbox (`ADR-0002` § 8).

### 4. Agent-generated keys

On first enrollment the agent generates, **on the agent**, three separate keys:

| Key | Asset | Algorithm | Public half | Leaves the host? |
|---|---|---|---|---|
| NATS identity NKey | `AST-3`'s material | Ed25519 — **the broker's, not ours** | 32 B | **Public half only** |
| Envelope-signing key | `AST-4` | Ed25519 ‖ ML-DSA-65 (`ADR-0005` § 4) | **1984 B** | **Public half only** |
| Payload-decryption key | `AST-5` | X25519 ‖ ML-KEM-768 (`ADR-0005` § 5) | **1216 B** | **Public half only** |

`ARCH-NATS-006` requires them separate and forbids reusing an NKey seed as a
Keystone signing or encryption key.

**Algorithms and sizes added at `G37`**, when `ADR-0005` named the primitives.
This section had generated three keys without saying what they were, which was
survivable only while nothing produced an envelope.

**The two Keystone keys are hybrid post-quantum; the NKey is not, and cannot
be.** A NATS identity key is an NKey — Ed25519, defined by the broker — so
`ARCH-NATS-006`'s first layer stays classical whatever this project decides.
`ADR-0005` § 5 records why the other two do not: `RSK-12` gives `AST-8` no
rotation and concedes that captured ciphertext stays decryptable for as long as
the key exists.

**The public halves are now kilobytes rather than tens of bytes**, which is what
an enrollment request carries and what § 1's token bundle carries back. The
service halves in the bundle total **3200 B** (`AST-7` 1984 ‖ `AST-8` 1216).

**All three public halves travel in the enrollment request, and the server
records all three.** The NATS public key is what the permanent user JWT is
minted against; the **signing verification key** and the **encryption public
key** are what every later protocol operation needs — P05 encrypts a command *to*
`AST-5`'s public half and verifies a result *against* `AST-4`'s. Enrollment is
the only moment those halves are established, so it is the only place they can
become authoritative. An ADR that generated them on the agent and never recorded
them would leave P05 and C03 to invent a source of truth for them.

**No private key the agent uses is ever generated by, transmitted to, or
recoverable from the server.** This is why a compromised server (`RSK-4`,
`THR-44`–`THR-46`, `THR-49`) cannot decrypt a result or forge an agent
signature: it can mint a *new* identity, which is `THR-49`, and it cannot become
an existing one.

A NATS user JWT is not a secret — the seed is — so the permanent credential
returns to the agent in clear over the token-scoped reply subject. Nothing
confidential crosses the enrollment path in either direction.

### 5. Server validation

On an enrollment request the server checks, in order: the token exists; its
state admits enrollment (§ 6); it has not expired; the presenting bootstrap
identity is the one minted for that token; the request carries all three of the
agent's public halves (§ 4); and the request is signed by the envelope-signing
key whose public half it carries — which proves control of the material, not
merely possession of a copy.

**The agent name is bound at creation, not claimed at enrollment.** The
permanent identity is minted for the name recorded in § 1, and the enrolling
party does not supply one. That is `THR-05` — an agent enrolling under another
agent's identity — and the answer is that there is no field in which to assert a
different identity: the token's subjects are its own, and the JWT it yields
names the agent the operator asked for.

Any failure is a **denial**, audited with its reason (`ARCH-OBS-001`), and the
agent exits `10` — the charter's code for a token that is spent, expired or
invalid. The three are not distinguished to the caller: telling an attacker
whether a token exists is an oracle.

### 6. The staged protocol

`ARCH-NATS-004`'s sequence, in its order. Server state is in the left column
because the server's record is what makes the protocol idempotent.

| Stage | Server state | What happens |
|---|---|---|
| S0 | `pending` | Token issued, bootstrap credential minted, nothing claimed |
| S1 | `issued` | Server validates (§ 5); **durably records the agent's three public halves** — NATS identity, signing verification, encryption; mints the permanent user JWT against the recorded NATS public key with `AST-16`; records the JWT; replies on the token-scoped subject |
| S2 | `issued` | **Agent** writes the credential file **and the two service public halves from the bundle** in one operation: write to a temporary file in the same directory, `fsync`, `rename` atomically, `fsync` the directory, mode `0600`. Credential and trust anchor are either both present or neither, by the same atomic rename. It also initialises its **durable job ledger**, so that a command arriving immediately after S4 has somewhere to be recorded before it is acted on (`ARCH-JOB-002`) |
| S3 | `issued` | **Agent** connects with the permanent identity and publishes a proof of connection on its own subject. **It keeps its bootstrap connection open**, because S6 arrives on it (RFC 0005 § 1) |
| S4 | `active` | Server observes the proof and marks the identity active |
| S5 | `active`, token `spent` | In the same durable write as S4, the server records the token **spent**. From here every request on the token is refused and audited, except S6's confirmation (RFC 0005 § 1) |
| S6 | `active`, token `spent` | The agent asks for confirmation on its token-scoped subject, and the server replies that the identity is active and the token spent. The agent then closes the bootstrap connection and removes the bundle. **Bootstrap access ends at the credential's expiry**, which the broker enforces; the server revokes nothing (RFC 0005) |

The agent exits `0` only on S6's confirmation. The charter requires exit `0` to
mean *the permanent identity is active and the server has confirmed the token
spent*, which is a statement about S4 and S6 together. **As first written, S5
revoked the bootstrap user and S6 verified it by re-reading the account.** The
server's key cannot revoke, and a re-read confirmed a revocation the broker was
not enforcing; RFC 0005 records the measurement.

**One-use is a property of the token, not of the message.** The token admits
exactly one agent identity. Re-presenting it from the same bootstrap identity
while the state is `issued` returns the *same* permanent JWT rather than minting
a second, which is what makes S1 idempotent without weakening one-use. A second
identity is never issued for one token.

**Idempotency is tied to the recorded key material, not merely to the token.** A
re-presentation carrying *different* public halves is a **denial**, not a
retry — the recorded values are authoritative from S1 onward. Without that rule,
anyone who obtained the bundle after a first request could re-present it with
their own keys and take over a pending enrollment, which is `THR-02` arriving
one stage later than expected.

### 7. Crash recovery, per stage boundary

`ARCH-NATS-004` requires a defined path at every stage. `THR-04` is the threat:
a crash between credential write and activation leaving the two sides
disagreeing about identity state.

| Crash between | Agent on restart | Server |
|---|---|---|
| S0–S1 | Bundle still present; retries the request | Still `pending`; treats the retry as a first request |
| S1–S2 | Bundle still present; retries and receives the **same** JWT (§ 6) | `issued`; returns the recorded JWT |
| S2–S3 | Credential file exists and is complete, by construction — the atomic rename means it is either absent or whole. Agent proceeds to S3 | `issued`, waiting |
| S3–S4 | Permanent connection may already have been proven; agent republishes the proof, which is idempotent | `issued` or `active`; a repeated proof changes nothing |
| S4–S6 | Agent has a working identity and still holds the bundle; it reconnects its bootstrap identity and asks for S6's confirmation again, which is idempotent | `active`, token `spent`, recorded in S4's one durable write (RFC 0005 § 1), so there is no S4–S5 boundary to crash across. **If the bootstrap credential has expired before the agent asks, the confirmation cannot arrive**, though the permanent identity works; what the agent reports then is C05's to decide (RFC 0005 § Open Questions) |

**The partial state that matters is S4 to expiry**: an active identity whose
bootstrap credential is still valid at the broker. That is `THR-03` — bootstrap
access surviving enrollment — and it is **no longer a crash case**. Under RFC
0005 it is how every enrollment ends: the server refuses the spent token, and the
credential lasts until its expiry, able to do only what RFC 0005 § Motivation
lists. `RSK-15` records the acceptance.

### 8. Bootstrap expiry

The bootstrap credential expires on the same short clock as the token, and
**expiry is how bootstrap access ends** — at every enrollment, not only a failed
one (RFC 0005). As first written it was the backstop behind a revocation the
server turned out unable to perform. It still covers every failed enrollment
too: if enrollment never reaches S6, bootstrap access ends anyway.

Expiry is enforced by the broker against the JWT and by the server against its
own record. Neither alone is trusted.

### 9. Reconnect

An enrolled agent that loses its connection **reconnects with its permanent
identity and does not re-enroll**. There is no path from a live permanent
identity back to a bootstrap one; the bundle is gone and the token is spent.

An agent whose credential file is missing or unreadable has no identity and
fails, loudly. It does not silently attempt to acquire a new one.

### 10. Re-enrollment

Re-enrollment issues a **new identity**, not a refreshed one: a new token, new
agent-generated keys, a new user JWT. The operator revokes the old identity
explicitly; the server does not infer that a new enrollment supersedes an old
one, because inferring it would let anyone who can enrol displace an existing
agent.

The agent's job ledger is not carried across. A re-enrolled host is a new agent
to the control plane, and `ARCH-JOB-003`'s at-most-one-attempt guarantee is a
property of a job identifier on an identity.

### 11. Rotation of agent-side material

**Re-enrollment is the rotation mechanism for `AST-3`, `AST-4` and `AST-5`.**
That is not a workaround: the keys are generated on the agent and never escrowed,
so replacing them and replacing the identity are the same operation.

The cost is stated rather than hidden — a rotated agent is a new agent, with a
new identifier and no history. § "Residual risks" records what that resolves and
what it does not.

### 12. Clock assumptions

Expiry and JWT validity are time-dependent, so the assumptions are stated:

| Assumption | If it is wrong |
|---|---|
| Server and broker clocks agree within a small bound — a minute, not an hour | A bootstrap JWT the server considers expired may still be accepted by the broker, extending the window `THR-03` lives in |
| The **agent's** clock is not trusted for any expiry decision | Nothing: the agent's clock is deliberately not load-bearing. Expiry is enforced by the server and the broker |
| Token expiry is short enough that skew is small relative to it | A skew comparable to the lifetime makes expiry meaningless as a backstop |

No clock is trusted to be monotonic across a restart, which is why every
recovery path in § 7 is driven by recorded state rather than elapsed time.

### 13. Disaster recovery

If the **server's store** is lost, enrolled agents keep working at the broker —
their identities are JWTs the broker validates, not rows in the server's
database — but the control plane can no longer correlate them, and no new
enrollment or revocation is possible until it is restored.

Recovery is restoring the store. Failing that, the fleet is re-enrolled, which
is `RSK-13`'s cost made concrete.

If an **agent's** credential file is lost, that agent is re-enrolled (§ 10).

### 14. Attestation, and what it does not attempt

**What Generation 2 accepts as evidence** that an agent is the identity it
claims to be:

1. possession of a valid, unspent, unexpired token bundle;
2. a request signed by the envelope-signing key whose public half it presents,
   proving control of freshly generated key material;
3. a successful permanent connection with the issued identity (S3), proving
   control of the NKey seed it registered.

**And what the agent accepts about the server**, which this section previously
did not mention at all: the service public halves in the bundle, on the strength
of the operator having carried it. The agent performs no check on them — it
cannot, since they are what checking is done *with*. **The bundle's integrity is
the whole of the agent's trust in the server**, which is `THR-51`.

**What it does not attempt**, stated because an attestation section that lists
only what it accepts reads as a stronger guarantee than this product makes:

- **No hardware root of trust.** No TPM, no secure element, no measured boot.
- **No host identity.** Nothing binds the enrollment to a particular machine —
  not a serial number, not a MAC address, not a fingerprint. The `--agent-name`
  is an operator's label, not a claim the product verifies.
- **No proof the bundle was not copied.** Anyone holding it before it expires can
  enrol as that agent, which is `THR-02`. One-use and short expiry bound the
  window; they do not close it.
- **No proof of the operator's intent.** The product cannot distinguish the host
  the operator meant from another host that received the bundle first.
- **No verification of the trust anchor.** A bundle altered in transit can
  substitute the service public halves, and the agent will trust whoever
  supplied them — `THR-51`, accepted as `RSK-14`.

Enrollment therefore attests **possession and freshness, not provenance**.
Strengthening it would require a capability Generation 2 does not have, and is
catalogued as a Future candidate rather than designed here.

## Residual risks

`RSK-7` named P03 as its expiry while covering seven assets across three owners.
It is **split**, and every asset that cited it lands in exactly one part.

| Risk | Assets | Outcome |
|---|---|---|
| `RSK-7` | `AST-3`, `AST-4`, `AST-5` | **Resolved.** Re-enrollment (§ 10, § 11) is a designed rotation mechanism for agent-side material. Its cost — a new identity and no history — is stated, not hidden |
| `RSK-12` | `AST-7`, `AST-8` | **Carried to P05.** The service envelope-signing and result-decryption keys have neither rotation nor revocation, and the envelope that uses them is P05's |
| `RSK-13` | `AST-6`, `AST-16` | **Carried to P10.** Broker-side revocation exists for both; no rotation *procedure* does, and rotating `AST-16` re-issues every identity it signed — a fleet-wide re-enrollment, which is an operational runbook rather than a protocol decision |

`AST-16` is placed in `RSK-13` rather than with the agent material, despite
re-enrollment being P03's design, because the gap is not the mechanism — § 10
supplies it — but the *procedure* for running it across a fleet without an
outage. That is P10's.

**Corrected at `G22`:** P10 is the *acceptance-harness design*, not a deployment or operations task — no such task exists in `REBOOT-EXECUTION-PLAN.md`. The work below is real and needs an owner; where it goes is settled when P10 disposes of the risk. The decision this ADR made is unchanged.

## Invariant coverage

| Invariant | Where |
|---|---|
| `ARCH-NATS-004` | §§ 1–8 — **satisfied**; every clause of the invariant, as RFC 0005 amended it, is a section here |
| `ARCH-NATS-002` | § 4, § 6 — **registered**; the principal list is `ADR-0002` § 3 |
| `ARCH-NATS-003` | § 3 — **registered**; the grammar and matrix are P04's |
| `ARCH-NATS-005` | § 3 — **registered**; the bootstrap identity reaches no `$JS.API` subject, and the allowlist is `ADR-0002` § 8, tested by P04 |
| `ARCH-NATS-006` | § 4 — **satisfied in part**; key generation and separation are decided here, the envelope is P05's |
| `ARCH-JOB-002` | § 6 — **registered**; the agent's durable ledger exists before enrollment completes, so a command arriving at activation has a receipt to be written before execution. Its schema is P08's |
| `ARCH-JOB-003` | § 10 — **registered**; at-most-one-attempt is a property of a job identifier on an identity, which is why § 10 does not carry a ledger across re-enrollment |
| `ARCH-OBS-001` | § 1, § 5 — **registered**; issuance and denial emit records, the schema is P08's |

P03 creates no new `ARCH-*` identifier.

## Alternatives considered

**A self-validating token — a signed JWT the server need not remember.**
Rejected. One-use cannot be enforced without server state, so the state exists
either way; a token carrying claims only adds a second thing that can disagree
with it.

**Server-generated agent keys, delivered in the bundle.** Rejected, and it is
the alternative that would have been easiest. It would put `AST-4` and `AST-5`
inside `TD-SRV`, making a compromised server able to forge any agent's signature
and decrypt any result — collapsing the separation `THR-45` and `RSK-4` depend on.

**A random reply inbox for enrollment.** Rejected. It would require response
permissions, which `ADR-0002` § 6 defers; a token-scoped reply subject is known
in advance and needs no dynamic grant.

**Treating a second presentation of a spent token as a new enrollment.**
Rejected. It makes the token multi-use in everything but name. § 6 returns the
same JWT instead, which is idempotent without being reusable.

**Binding enrollment to a host fingerprint.** Rejected for v0.6.0, and § 14 says
so plainly rather than implying the product does it.

## Consequences

**Positive.** No private key an agent uses ever exists outside that agent. The
bootstrap identity's authority is two subjects wide and expires on its own.
Every stage boundary has a recovery path driven by recorded state.

**Negative.** Rotation means a new identity and a lost history. The token bundle
is a credential in transit, and the product cannot tell whether it was copied.
The server holds `AST-16`, which is `THR-49`.

**Neutral.** Enrollment stays outside JetStream, so it adds no stream, no
consumer and no `$JS.API` permission.

## What this ADR does not decide

**Whether an agent may require its trust anchor to arrive by a second channel.**
The mitigation for `THR-51` is *channel separation* — a service public half baked
into a signed package or a machine image, so an attacker must compromise both the
install path and the bundle. That is a **deployment mode, not an option**: an
agent that merely *prefers* a pre-provisioned anchor falls back to the bundle
when the file is absent, and an attacker who can alter the bundle can delete a
file. It would belong in `ADR-0002` § 14's deployment modes with refusal as the
behaviour, its operational procedure is P10's, and `CAP-IDENT-021` is the
catalogued capability. **Not designed here**; `RSK-14` records the acceptance.

**Corrected at `G22`:** P10 is the *acceptance-harness design*, not a deployment or operations task — no such task exists in `REBOOT-EXECUTION-PLAN.md`. The work below is real and needs an owner; where it goes is settled when P10 disposes of the risk. The decision this ADR made is unchanged.

The canonical subject grammar and the permission matrix (**P04**); envelope
format, signing and encryption operations, and `RSK-12` (**P05**) — which
consumes the signing and encryption public halves S1 records, rather than
establishing them; job lifecycle and ledger semantics (**P06**); audit record schema and retention (**P08**);
which operators may create tokens (**P09**); fleet rotation procedure and
`RSK-13` (**P10**); generated configuration and JWT fixtures (**C03**); the
crash matrix as executed rather than described (**C05**).

## Validation

`make check`. Acceptance cases and their demonstrations are in
[`P03-acceptance-evidence.md`](../dossiers/P03-acceptance-evidence.md).

## References

- [NATS decentralized JWT authentication](https://docs.nats.io/running-a-nats-service/configuration/securing_nats/auth_intro/jwt)
- [NATS authorization](https://docs.nats.io/running-a-nats-service/configuration/securing_nats/authorization)
- [ADR-0002 — NATS-native capability decisions](0002-nats-native-capabilities.md)
