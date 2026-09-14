# ADR-0003: Enrollment and identity

- **Status:** Proposed
- **Date:** 2026-09-13
- **Task:** P03, bounded by [`docs/dossiers/P03.md`](../dossiers/P03.md)
- **Builds on:** [`ADR-0002`](0002-nats-native-capabilities.md)

## Context

Enrollment is the only key transition Generation 2 specifies.
[`THREAT-MODEL.md`](../project/THREAT-MODEL.md) § 8 says so plainly: everything
after it is undesigned. `ARCH-NATS-004` is the invariant this ADR owns, and it
fixes the shape of the protocol before any design begins — one-use,
token-scoped credentials; a staged, idempotent sequence; fsync and atomic rename
at mode `0600`; proof of a permanent connection; revocation, and **verification**
of that revocation; short bootstrap expiry as the backstop; and a defined
crash-recovery path at every stage.

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
| Generate a one-use token | High-entropy random identifier and secret. **Opaque, not a JWT** — a self-validating token cannot be made one-use without server state, so the server keeps the state and the token carries no claims |
| Mint a bootstrap NATS credential | A user JWT in the Keystone account, signed with `AST-16`, scoped to this token's subjects only (§ 3), expiring with the token |
| Record a pending enrollment | Token identifier, agent name, expiry, state `pending`, and the bootstrap identity's public key. The agent's own keys do not exist yet |
| Emit the bundle **once** | To standard output, never to a file the server chooses |
| Emit an audit record | Issuance is a lifecycle transition (`ARCH-OBS-001`) |

Exit codes are the charter's: `0`, `10` when the operator is not authorized to
create a token, `1` on local or usage error. *Which* operators may create tokens
is P09's.

### 2. Token handling — the charter's three deferrals

| Question | Decision | Why |
|---|---|---|
| Standard input? | **Yes.** `--token-file -` reads the bundle from standard input | `THR-01` is the token being read from shell history or a process listing, and the charter forbids supplying it in a form that exposes it. A pipe satisfies that as well as a file and avoids the bundle touching disk at all, which is strictly better. Neither form ever places the secret in `argv` |
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

| Key | Asset | Leaves the host? |
|---|---|---|
| NATS identity NKey | `AST-3`'s material | **Public half only** |
| Envelope-signing key | `AST-4` | **Public half only** |
| Payload-decryption key | `AST-5` | **Public half only** |

`ARCH-NATS-006` requires them separate and forbids reusing an NKey seed as a
Keystone signing or encryption key.

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
| S2 | `issued` | **Agent** writes the credential file: write to a temporary file in the same directory, `fsync`, `rename` atomically, `fsync` the directory, mode `0600`. It also initialises its **durable job ledger**, so that a command arriving immediately after S4 has somewhere to be recorded before it is acted on (`ARCH-JOB-002`) |
| S3 | `issued` | **Agent** disconnects the bootstrap identity, connects with the permanent identity, and publishes a proof of connection on its own subject |
| S4 | `active` | Server observes the proof and marks the identity active |
| S5 | `active` | Server revokes bootstrap access — the bootstrap user is added to the account's revocation list |
| S6 | `active`, bootstrap `revoked` | **Server verifies the revocation** by re-reading the account state and confirming the bootstrap identity is listed. Enrollment is complete only here |

The agent exits `0` only after S6 is confirmed to it — the charter requires exit
`0` to mean *the permanent identity is active and bootstrap access is revoked*,
which is a statement about S4 and S6 together.

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
| S4–S5 | Agent has a working identity; it waits for completion and retries the proof | `active`, bootstrap **not yet revoked**. The backstop is bootstrap expiry (§ 8) |
| S5–S6 | As above | `active`, revocation performed but unverified. The server re-runs S6 on recovery; an unverified revocation is not treated as a revocation |

**The partial state that matters is S4–S6**: an active identity whose bootstrap
access is still live. That is `THR-03` — bootstrap access surviving enrollment —
and the design's answer is that it is bounded by expiry even if the server never
recovers.

### 8. Bootstrap expiry

The bootstrap credential expires on the same short clock as the token, and the
expiry is the **failure backstop for every stage above**. If enrollment never
reaches S6, bootstrap access ends anyway; if the server never recovers to revoke,
bootstrap access ends anyway.

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

## Invariant coverage

| Invariant | Where |
|---|---|
| `ARCH-NATS-004` | §§ 1–8 — **satisfied**; every clause of the invariant is a section here |
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
