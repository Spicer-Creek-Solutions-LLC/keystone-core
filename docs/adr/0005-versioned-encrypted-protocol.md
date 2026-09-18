# ADR-0005: Versioned encrypted protocol

- **Status:** Proposed
- **Date:** 2026-09-14
- **Task:** P05, bounded by [`docs/dossiers/P05.md`](../dossiers/P05.md)
- **Builds on:** [`ADR-0002`](0002-nats-native-capabilities.md), [`ADR-0003`](0003-enrollment-and-identity.md), [`ADR-0004`](0004-subject-authorization.md)

## Context

`ADR-0002` decided how Keystone uses the broker, `ADR-0003` how an agent
acquires an identity, `ADR-0004` what each principal may say. **This decides
what is inside the message** — the fourth and fifth of RFC 0001's six cumulative
security layers, and the first that protects content from a party who is
*authorized to carry it*.

A defect here is invisible to every broker-side control the previous three
provide. `ACT-5` holds every byte in flight and every permission check passes.

### Where recipients come from

`ADR-0003` § 4 and § 6 record the agent's **signing verification key** and
**encryption public key** at enrollment, durably, against the issued identity.
Those are this protocol's only source of truth for an agent recipient, and they
exist because P04's review found they were being established and dropped.

The record once ran one way: an agent must **verify** a service-signed command,
which needs `AST-7`'s public half, and must **encrypt** a result to the result
service, which needs `AST-8`'s public half, and `ADR-0003` delivered neither.
That was raised here as a finding and has since been **closed**: `ADR-0003` § 1
puts both halves in the token bundle and its § 6 persists them with the
credential file in one atomic operation.

The bundle is the only pre-trust channel in the system, which is also its
weakness — a bundle altered in transit substitutes the anchor, and the agent
cannot check what checking is done with. That is `THR-51`, accepted as `RSK-14`,
whose real mitigation is channel separation and a deployment mode P10 would own.

**Corrected at `G22`:** P10 is the *acceptance-harness design*, not a deployment or operations task — no such task exists in `REBOOT-EXECUTION-PLAN.md`. The work below is real and needs an owner; where it goes is settled when P10 disposes of the risk. The decision this ADR made is unchanged.

## Decision

### 1. Envelope canonicalization

An envelope is a **fixed sequence of length-prefixed fields**, in the order
below. The signature is computed over the concatenation of every field before
it.

| # | Field | Notes |
|---|---|---|
| 1 | Protocol version | Cleartext, and **covered by the signature** (§ 2) |
| 2 | Message class | One of the seven in § 7 |
| 3 | Job identifier | Empty for classes that carry none (§ 3) |
| 4 | Correlation identifier | **Empty on encrypted classes**, where it travels inside the payload instead (§ 3) |
| 5 | Sender identifier | The principal, per `ADR-0004` |
| 6 | Timestamp | § 6 |
| 7 | Nonce | § 6 |
| 8 | Ciphertext, or cleartext payload | § 5 decides which per class |
| 9 | Signature | Over fields 1–8 |

**Not canonical JSON, and the reason is not taste.** Canonical JSON requires
agreement on key ordering, number formatting, string escaping and Unicode
normalization; each is a place two implementations can disagree and produce
different bytes for the same message, which makes a signature fail for no
security reason — or worse, makes two different messages produce the same bytes.
A fixed field order with explicit lengths is **deterministic by construction**:
there is nothing to canonicalize.

#### The length prefix

**Added at `G36`.** This section said *length-prefixed* and *explicit lengths*
and defined neither the width nor the encoding, so no implementation could emit
a single framing byte from it and no framing vector could be derived. `C01-A`
stopped on exactly that, correctly: § "What this ADR does not decide" gives C01
the encoder's implementation, and every wire-visible choice is this ADR's.

**Every field carries a `uint32` big-endian length prefix**, immediately before
its bytes. All nine, the signature included — one rule, no exceptions, because a
per-field rule is nine places two implementations can disagree.

**The prefix is part of the signed input.** Field 9 is computed over the framed
bytes of fields 1–8: each field's prefix followed by its value, concatenated in
order. This is not a detail. Sign the values alone and two different field
splits produce a byte-identical signed input — an attacker moves a byte from the
end of one field to the start of the next and the signature still verifies. That
is precisely the ambiguity this section rejects canonical JSON to avoid, so
excluding the prefix would defeat the reason the encoding was chosen.

| Property | Value |
|---|---|
| Prefix width | 4 bytes, `uint32`, unsigned |
| Byte order | Big-endian (network order) |
| Representable field length | `0` to `2³²−1` |
| Enforced maximum | **Not stated here.** `ADR-0002` § 9 caps the payload, and that value is its to change |
| Zero-length field | Prefix `00 00 00 00` and no bytes. Representable, and **distinct from absent** — fields 3 and 4 require it |
| Envelope length | The sum of its framed fields. Nothing follows field 9 |

**The representable maximum and the enforced maximum are different numbers, and
a receiver enforces the smaller.** The first is a property of this encoding; the
second is policy `ADR-0002` § 9 owns. Restating that policy here would create a
second copy to drift, which is why the row above points instead of repeating.

**A receiver validates a length against the enforced maximum before it
allocates.** A malformed envelope can declare nearly 4 GiB in four bytes, and a
parser that trusts the prefix first is a denial of service reachable by anyone
who can publish. This is a requirement on the receiver, not a quality-of-
implementation note for C01 to weigh.

**Refusals reuse § 9's existing set**; framing adds no code. A truncated prefix,
a field shorter than its prefix declares, a prefix that disagrees with its field,
or bytes after field 9 are all **malformed envelope**. A length over the enforced
maximum is **payload too large**.

**What this fixes and what it does not, stated so the boundary is not discovered
the way the last one was.** The framing is now fully determined: given nine field
*values*, the bytes on the wire follow from this section and nothing else. The
**value encodings do not follow from it** — this ADR is structural about the
version integer (§ 2), the timestamp and the nonce (§ 6), and it remains so.

That is sufficient for the framing vectors `C01-A` owes, because a framing vector
takes field values as inputs and asserts the bytes they produce. It is **not**
sufficient to write a complete interoperable envelope by hand, and two
implementations could still encode the same version integer differently. **G36
raised that and did not fix it**: it is a distinct wire-visible gap, and widening
a task to cover it is the move that produced this one.

### 2. Version negotiation, and why the version is in two places

The protocol version is a single integer, **readable in cleartext as field 1**
and **covered by the signature**.

Both are necessary and for opposite reasons:

- **Cleartext**, because a receiver must know how to parse an envelope before it
  can verify one. A version inside the ciphertext could only be read by
  decrypting, and decrypting requires knowing the format.
- **Signed**, because a cleartext version an attacker can edit is a downgrade
  switch. `ACT-5` holds every byte in flight; if flipping field 1 selected an
  older format, the protocol's floor would be its weakest historical version
  forever. Covering it means a flip fails verification.

**An unknown version is refused**, with the typed error of § 9. It is not
best-effort parsed. A receiver that tries to make sense of a version it does not
know is a receiver an attacker can teach, and "tolerant of the unknown" is the
property every downgrade attack needs.

### 3. Identifiers

| Identifier | Assigned by | Scope | Carried in |
|---|---|---|---|
| Job identifier | The server, when it accepts a job | Unique for the life of the deployment | Command, cancellation, result, and the events of that job |
| Correlation identifier | The server, per operator action | Groups the envelopes of one action | Every envelope of that action |

The charter requires a job identifier for **every accepted job** (§ 5.3), so it
is assigned at acceptance and not at execution: a job that never reaches its
agent still has one, which is what lets `UNKNOWN` name a specific job.

**Which of the two is visible to the broker, and why they differ.** The **job
identifier** is cleartext, and deliberately: § 8 puts it in a header because an
operator-facing audit record correlates on it, so hiding it in the envelope
would achieve nothing while the header carries it. The **correlation
identifier** has no such requirement — nothing outside the two endpoints needs
it — so on every encrypted class it travels **inside the payload**, and envelope
field 4 is empty. On the classes that are not encrypted it is cleartext like
everything else about them.

That asymmetry is the whole of the difference: an observer can link a command to
its result by job identifier, which § 6 already concedes as correlation, and
cannot link an operator's several actions to each other.

Both are opaque, high-entropy, and constrained to the same character set as
`ADR-0004`'s subject tokens — not because they appear in subjects, but because
they appear in audit records (`ARCH-OBS-001`) and in operator output, and an
identifier that can carry a delimiter is an identifier that can be mistaken for
structure.

### 4. Signatures

**Three key roles, and they are never the same key.** `ARCH-NATS-006`'s first
clause requires NATS transport identity, Keystone envelope signing and Keystone
payload encryption to **use separate keys**, and forbids reusing an NKey seed as
either. This protocol never mixes them: a signature is made with a signing key
(`AST-4`, `AST-7`) and never with a NATS identity key, and encryption uses a
payload key (`AST-5`, `AST-8`) and never a signing key. `ADR-0003` § 4 generates
the agent's three separately for the same reason.

**Every envelope class is signed.** No class travels unsigned.

| Class | Signed by | Verified by |
|---|---|---|
| Command | Service signing key (`AST-7`) | Agent, with `AST-7`'s public half |
| Cancellation | Service signing key (`AST-7`) | Agent, with `AST-7`'s public half |
| Result | Agent (`AST-4`) | Result service, with the half recorded at enrollment |
| Lifecycle event | Agent (`AST-4`) | Monitoring role, same half |
| Presence | Agent (`AST-4`) | Presence consumer, same half |
| Enrollment request | The agent's newly generated signing key | Enrollment service, with the half in the request (`ADR-0003` § 5) |
| Enrollment reply | Service signing key (`AST-7`) | Agent, with the half from the token bundle |

`ARCH-NATS-006` named commands and results when this ADR was written. It did not
say *only* those, so requiring signatures on the rest added an obligation
without contradicting it; [RFC 0002](../rfcs/0002-signed-envelope-classes.md)
has since made the invariant name all seven. See § 11.

**Signing presence and events is not decoration.** `THR-50` records presence
fabricated on the wire: `ADR-0004`'s permission matrix constrains *agents*, and
not `ACT-5` who can inject, nor `ACT-4` who mints identities. `AST-4` never
leaves the host (`ADR-0003` § 4), so a minted NATS identity can connect and
publish and **cannot produce the signature**.

### 5. Recipient encryption, and what is deliberately not encrypted

| Class | Encrypted to | Why |
|---|---|---|
| Command | Agent (`AST-5`'s recorded public half) | Argv is the operator's intent; `THR-10` |
| Cancellation | Agent (`AST-5`) | Carries the job identifier and nothing else of value, but travels the same path as a command and is encrypted for the same reason |
| Result | Result service (`AST-8`'s public half) | Captured output; `THR-21` |
| Enrollment request | **Not encrypted** | It carries three public keys and a signature. Public by construction |
| Enrollment reply | **Not encrypted** | A NATS user JWT is not a secret; the seed is, and the agent generated it and never sent it |
| Lifecycle event | **Not encrypted** | See below |
| Presence | **Not encrypted** | See below |

**Presence and lifecycle events are signed and not encrypted, deliberately.**
Encryption would protect a payload that adds nothing the subject and the timing
do not already reveal — `ADR-0004` puts the agent identifier in the subject, and
`THREAT-MODEL.md` § 6 already concedes presence cadence as `RSK-6`. It would
also require a recipient key for the presence consumer and the monitoring role,
**neither of which holds one**, so it would mean new key material under a risk
(`RSK-12`) that has no rotation or revocation.

`ARCH-NATS-006` names the layers as independent. Using authenticity without
confidentiality is that independence working, not a shortcut.

**Lifecycle events are therefore constrained to carry nothing job-specific
beyond the identifiers of § 3** — a coarse transition and the job it belongs to,
which the subject and timing already imply. An event that needed to carry
*content* would need a recipient key, and that decision would reopen this one
rather than stretch it.

### 6. Replay bounds, timestamps and nonces

| Class | Replay key | Window |
|---|---|---|
| Command | Job identifier | **The agent's durable ledger, not a window.** `ARCH-JOB-003` permits at most one automatic attempt per job identifier, and the ledger outlives any broker deduplication |
| Cancellation | Job identifier and class | Idempotent by construction: cancelling a cancelled job is a no-op |
| Result | Job identifier | At most one terminal result per job; a later verified result may supersede `UNKNOWN` (`ARCH-JOB-004`) |
| Enrollment request | Token identifier | Until the token expires. Re-presentation returns the same identity (`ADR-0003` § 6) |
| Lifecycle event | Event identifier | Bounded age; duplicates are discarded |
| Presence | Timestamp | A presence older than the staleness bound is discarded rather than treated as current |

**`Nats-Msg-Id` is not the replay control.** `ADR-0002` § 11 sets it and its
window defaults to two minutes; RFC 0001 is explicit that the application ledger
remains authoritative after the window expires. Treating the deduplication
window as the replay bound would be claiming exactly-once execution, which
`ARCH-JOB-001` forbids.

**Timestamps bound staleness; nonces bound repetition.** The timestamp rests on
the clock assumption `ADR-0003` § 12 already states — server and broker agree
within a small bound, the agent's clock is not trusted for expiry — and the same
assumption is relied on here rather than a new one invented.

### 7. Message classification

The table the whole ADR exists to produce.

| | Enrollment request | Enrollment reply | Command | Cancellation | Result | Lifecycle event | Presence |
|---|---|---|---|---|---|---|---|
| **Signer** | Agent's new signing key | Service (`AST-7`) | Service (`AST-7`) | Service (`AST-7`) | Agent (`AST-4`) | Agent (`AST-4`) | Agent (`AST-4`) |
| **Verifier** | Enrollment service | Agent | Agent | Agent | Result service | Monitoring role | Presence consumer |
| **Encryption recipient** | None | None | Agent (`AST-5`) | Agent (`AST-5`) | Result service (`AST-8`) | None | None |
| **Replay key / window** | Token id / token lifetime | Token id / token lifetime | Job id / ledger | Job id + class / idempotent | Job id / at-most-one terminal | Event id / bounded age | Timestamp / staleness bound |
| **Maximum size** | Small fixed bound | Small fixed bound | `ADR-0002` § 9's payload limit | Small fixed bound | Output limit, truncation reported (`ARCH-EXEC-001`) | Small fixed bound | Small fixed bound |
| **Durability** | None — core NATS | None — core NATS | `KS_CMD` | `KS_CMD` | `KS_RES` | None — core NATS | None — core NATS |
| **Retention** | None | None | `KS_CMD` max age | `KS_CMD` max age | `KS_RES` age and per-subject cap | None | None |
| **Headers** | Version, class | Version, class | `Nats-Msg-Id`, version, class, job id | `Nats-Msg-Id`, version, class, job id | `Nats-Msg-Id`, version, class, job id | Version, class | Version, class |
| **Accepted metadata leakage** | That a token is being redeemed, and when; the token identifier | That one was redeemed; the token identifier | Which agent is commanded, when, roughly how long the argv is, **and the job identifier in cleartext** | That a job was cancelled, when, **and which job** | Job duration, roughly how much output, **and the job identifier** | That an agent changed state, **which job it concerns, and the coarse transition** | Which agents are live, and when one stops |

**Most of this is already conceded by `THREAT-MODEL.md` § 6 and recorded as
`RSK-6`. One part is not, and saying otherwise would be the kind of claim this
project keeps having to retract.**

The cleartext **job identifier** makes correlation *exact* where § 6 concedes it
as *inferred*: that table says an observer can build "a usable operational
picture… who is working on what, when, and for how long" by correlating subject,
timing and size. A literal identifier linking a command to its result, its
cancellation and its events removes the inference step. The difference is degree
rather than kind, and it is a widening of `RSK-6` rather than a restatement of
it.

A **lifecycle event** additionally reveals the job it concerns and a coarse
transition — which § 6's table does not list at all, because events did not have
a defined payload before this ADR.

> **Finding raised, not fixed.** `THREAT-MODEL.md` § 6's observable table should
> gain a row for cleartext envelope identifiers. P05's dossier permits editing
> § 6's closing sentence and `RSK-6`'s row and **not that table**, so the
> widening is recorded in both places P05 may write and the table's row is left
> to its own task. Widening a risk in the row while leaving the table that
> derives it unchanged is half a fix, and this is the half P05 is allowed.

### 8. Headers

The complete set. No other header is set by Keystone.

| Header | On | Carries |
|---|---|---|
| `Nats-Msg-Id` | Classes stored in a stream | A deduplication identifier derived from the job identifier and class |
| Protocol version | Every class | The integer of § 2, duplicating field 1 so a broker-side consumer can route without parsing |
| Message class | Every class | The class name of § 7 |
| Job identifier | Command, cancellation, result | The identifier of § 3 |

**No header carries a secret, a token, or payload plaintext** (`ARCH-OBS-001`,
`THR-27`). The job identifier is in a header because an operator-facing audit
record correlates on it; it is an opaque identifier and not content.

A header is readable by `ACT-5` by definition. The test for whether a field
belongs here is not "is it sensitive" but **"is the broker already entitled to
it"** — and only routing metadata is.

### 9. Error codes and forward compatibility

A receiver refusing an envelope returns one of a fixed set: unknown version,
malformed envelope, signature invalid, decryption failed, replay rejected,
payload too large, unknown class.

**The set is deliberately coarse.** A sender learns that its envelope was
refused and roughly why. It does not learn which key failed, whether an
identifier exists, or where in the structure parsing stopped — distinctions that
help an attacker enumerate more than they help an operator debug. Diagnosis
happens in the audit record (`ARCH-OBS-001`), which the operator can reach and
an attacker cannot.

**Forward compatibility, stated as what a receiver must and must not do:**

| Must tolerate | Must refuse |
|---|---|
| An unknown **header** it does not use | An unknown **version** |
| Additional trailing bytes in a field it does not interpret, **if the signature still verifies** — that is, bytes **within that field's declared length** (§ 1) | A **known** version whose envelope does not match that version's field sequence |
| — | An unknown **class** |

Tolerance stops where verification does. A receiver never accepts something it
cannot verify because the alternative would be worse for availability — that
trade is exactly the downgrade path § 2 exists to close.

### 10. What a test-vector set must cover

**No vector value appears in this ADR.** A cryptographic test vector is a byte
string produced by running the encoding and the operations, and no build exists
before P11 (`REBOOT-EXECUTION-PLAN.md` § P05). C01 computes them from this
coverage. **A vector written by hand before an implementation exists is a number
the implementation will be tuned to match.**

The set must exercise:

| Dimension | Cases |
|---|---|
| Class | All seven of § 7 |
| Field boundaries | Empty field, single-byte field, field at its maximum length, field one byte over |
| Identifiers | Absent job identifier where the class permits it; maximum-length identifier |
| Version | Current version; a version below the minimum; a version above the maximum known |
| Signature | Valid; one bit flipped in each of fields 1 through 8; signature from the wrong key |
| Encryption | Valid; ciphertext truncated; ciphertext from the wrong recipient key |
| Ordering | Fields transposed; a length prefix that disagrees with its field |
| Cross-process | Every vector produced by one process and verified by another, which is the point of `ARCH-COMM-003` |

The **one bit flipped in each of fields 1 through 8** row is the one that matters
most: it is what demonstrates the version is genuinely covered by the signature
(§ 2), and a vector set that omits field 1 would leave the downgrade defence
untested.

## Residual risks

| Risk | Outcome |
|---|---|
| `RSK-11` | **Resolved.** § 4 signs every envelope class, cancellation included, so a holder of the command publisher credential alone cannot produce an accepted cancellation. **At P05 this closed the exposure and not the invariant gap**; [RFC 0002](../rfcs/0002-signed-envelope-classes.md) has since closed the gap by making the invariant name every class — see § 11 |
| `RSK-1` | **Renewed.** Joint possession of the publisher credential and `AST-7` still yields accepted commands. P05 designs the envelope, not key custody; nothing here changes what two thefts together can do |
| `RSK-6` | **Renewed and widened.** § 8 fixes the header set and § 7 enumerates the accepted leakage per class, so what the broker learns is stated rather than inferred — but stating it is not reducing it, and § 7 adds two things `THREAT-MODEL.md` § 6 does not list: a **cleartext job identifier**, which makes the correlation § 6 concedes *exact* rather than inferred, and a lifecycle event's job linkage and coarse transition. The correlation identifier is not among them — § 3 moves it inside the payload on every encrypted class. § 6's observable table should gain a row for the first two, which is outside this task's boundary and recorded in § 7 |
| `RSK-12` | **Renewed.** `AST-7` and `AST-8` still have neither rotation nor revocation. **The candidate mitigation is recorded rather than left to be rediscovered**: a certificate chain rooted outside the deployment — `CAP-IDENT-004` and `CAP-IDENT-021`, Generation 1 capabilities catalogued as Future — would allow leaf rotation, and reaching it is a phase-gate promotion (program rule 10). A *server-held* root is rejected: it would let a compromised server certify itself as any agent and forge that agent's signatures, destroying what `ADR-0003` § 4 exists to create and widening `RSK-4` |

## The invariant gap `RSK-11` left behind, and its closure

`ARCH-NATS-006` required **commands and results** to be signed. This ADR
requires every class to be signed, which was permitted because the invariant did
not say *only* those — and which meant nothing required it. A later ADR could
have dropped signatures on cancellation, presence, events or enrollment and
violated nothing. `RSK-11`'s exposure was closed here; the reason it existed was
not.

**It is closed now.** [RFC 0002](../rfcs/0002-signed-envelope-classes.md) amends
`ARCH-NATS-006` to name every envelope class the product carries, citing this
ADR as the design it ratifies. The omission was never specific to cancellation —
the invariant was written around the two journeys that existed when it was
drafted, and this ADR is what made the full list knowable.

## Invariant coverage

| Invariant | Where |
|---|---|
| `ARCH-NATS-006` | §§ 4, 5 — **satisfied**. What these sections exceeded in the invariant as drafted, [RFC 0002](../rfcs/0002-signed-envelope-classes.md) has since made the invariant require |
| `ARCH-OBS-001` | § 8 — **satisfied in part**; no header carries a secret or plaintext. The audit record's schema is P08's |
| `ARCH-JOB-001` | § 6 — **registered**; replay bounds are never described as exactly-once execution |
| `ARCH-JOB-003` | § 6 — **registered**; the ledger is authoritative beyond any window. The ledger is P06's and P08's |
| `ARCH-NATS-007` | § 7 — **registered**; sizes fit inside `ADR-0002` § 9's limits |
| `ARCH-NATS-010` | § 6 — **registered**; a redelivered envelope is byte-identical and resolves on the replay key |
| `ARCH-COMM-003` | § 10 — **registered**; cross-process vectors are required of C01, which is where they can exist |

P05 creates no new `ARCH-*` identifier.

## Alternatives considered

**Canonical JSON.** Rejected — see § 1. Every canonicalization rule is a place
two implementations disagree, and the failure is silent.

**Version inside the ciphertext.** Rejected. A receiver must parse before it can
decrypt; a version it cannot read until after decryption is not a version.

**Version in cleartext only, unsigned.** Rejected, and it is the tempting one:
it parses fine and costs nothing. It also hands `ACT-5` a downgrade switch, and
the protocol's floor becomes its weakest historical version permanently.

**Encrypting presence and events.** Rejected. It protects what the subject
already reveals, and would require new recipient keys under `RSK-12`, which has
no rotation or revocation. Signing gives the property actually missing —
`THR-50` — at no new key material.

**Fine-grained error codes.** Rejected. "Signature invalid at field 6" helps an
attacker enumerate structure more than it helps an operator, who has the audit
record.

**Best-effort parsing of unknown versions.** Rejected. Availability bought by
accepting the unverifiable is the transaction every downgrade attack needs.

## Consequences

**Positive.** No class travels unsigned. The version cannot be downgraded
without failing verification. The accepted metadata leakage is now enumerated
per class rather than described in aggregate.

**Negative.** Signing presence costs a signature per heartbeat per agent.
Every recipient here is trusted because the token bundle said so, which makes
the bundle's integrity a protocol dependency and not only an enrollment one —
`THR-51`, accepted as `RSK-14`. `RSK-12` is unchanged, and the protocol's keys
still cannot be rotated.

**Neutral.** Fixed field order means a new field is a version change, not an
extension. That is a cost with versions and a benefit without ambiguity.

## What this ADR does not decide

Job lifecycle, delivery semantics and `UNKNOWN` (**P06**); execution limits and
argv handling (**P07**); the audit record's schema and retention (**P08**);
operator-facing error presentation (**P09**); the canonical encoder's
**implementation**, the signature and encryption operations, fuzzing, and the
**computed** vectors (**C01**).

**Sharpened at `G36`, because the earlier wording is what went wrong.** This read
*the canonical encoder* without qualification, which can be taken as the encoder's
**format** — and § 1 did not state the format either, so the two documents
together left the length prefix belonging to nobody. C01 owns the encoder as
code. **The bytes it emits are this ADR's**, as § 1 now says outright. How the agent obtains the service public halves is no longer open
here — `ADR-0003` § 1 and § 6 decide it — and the residue is `RSK-14`, whose
channel-separated deployment mode belongs to **P10**.

**Corrected at `G22`:** P10 is the *acceptance-harness design*, not a deployment or operations task — no such task exists in `REBOOT-EXECUTION-PLAN.md`. The work below is real and needs an owner; where it goes is settled when P10 disposes of the risk. The decision this ADR made is unchanged.

## Validation

`make check`. Acceptance cases and their demonstrations are in
[`P05-acceptance-evidence.md`](../dossiers/P05-acceptance-evidence.md).

## References

- [ADR-0002 — NATS-native capability decisions](0002-nats-native-capabilities.md)
- [ADR-0003 — Enrollment and identity](0003-enrollment-and-identity.md)
- [ADR-0004 — Subject authorization](0004-subject-authorization.md)
- [NATS authorization](https://docs.nats.io/running-a-nats-service/configuration/securing_nats/authorization)
- [JetStream streams](https://docs.nats.io/nats-concepts/jetstream/streams)
