# RFC 0002: Every envelope class is signed

- **Status:** Accepted
- **Decision date:** 2026-09-14
- **Decision owner:** Project maintainer
- **Amends:** [`ARCH-NATS-006`](../project/ARCHITECTURE-INVARIANTS.md) —
  Independent cryptographic layers
- **Ratifies:** [`ADR-0005`](../adr/0005-versioned-encrypted-protocol.md) § 4
  and § 5
- **Closes:** the invariant gap recorded against `RSK-11` in
  [`THREAT-MODEL.md`](../project/THREAT-MODEL.md)

## Summary

`ARCH-NATS-006` requires commands and results to be signed. The product carries
five more envelope classes — enrollment request, enrollment reply, cancellation,
lifecycle event and presence — and the invariant names none of them. This RFC
amends it to name every class, to state which classes must additionally be
encrypted, and to close the enumeration so that a class added later cannot
arrive unnamed the way these five did.

The amendment is **additive**. The three sentences the invariant already
contains are unchanged; they remain exactly true and every document measured
against them stays measured against the same words.

## Motivation

### What the invariant does not require

`ADR-0005` § 4 signs every envelope class. That is permitted, because the
invariant names commands and results and does not say *only* those. Requiring
more than an invariant requires contradicts nothing.

**The consequence is that nothing requires it.** A later ADR could drop
signatures on cancellation, presence, lifecycle events or enrollment and violate
no invariant, pass every architecture gate, and be caught only by whoever
remembered that `ADR-0005` had decided otherwise. `ARCH-TEST-003` makes every
invariant executable; it cannot make an unstated requirement executable.

This is recorded as the open half of `RSK-11`: `ADR-0005` closed the *exposure*
— a holder of the command-publisher credential alone can no longer produce an
accepted cancellation — and stated in its own § 11 that the invariant gap
remained, so that a reader between the two tasks would not be misled.

### Why the gap existed

Not through oversight about cancellation specifically. `ARCH-NATS-006` was
drafted when the product had two described journeys, 5.3 and 5.5, and it named
the two envelopes those journeys carry. Every other class was designed
afterwards. The invariant was accurate about what was known and silent about
what was not, and nothing in its wording distinguished the two.

That is the defect this RFC actually fixes. Naming five more classes corrects
today's list; **stating that the list is closed** is what stops the next class
from arriving the same way.

### Who is affected

No operator and no deployment. There is no Generation 2 product code — it begins
at P11 — and no Generation 1 compatibility obligation survives RFC 0001. The
affected parties are the ADRs that follow: P06 through P09 and the C
workstreams, which are measured against this invariant, and C01, which
implements the envelope.

### If we do nothing

`ADR-0005` § 4 remains a decision without a requirement behind it, and
`RSK-11`'s expiry arrives with its exposure closed and its cause open.

## Design Overview

Three additions to `ARCH-NATS-006`, after its existing text:

1. **Every envelope class the product carries is signed**, with the seven
   classes enumerated, and an envelope that does not verify is refused.
2. **Which classes are additionally encrypted**, and that the remainder being
   signed-and-not-encrypted is this invariant permitting it rather than omitting
   them.
3. **The enumeration is closed** — a new class, or a change to which classes are
   encrypted, amends this invariant first.

## Detailed Design

### The amended requirement

The seven classes and their obligations, which are `ADR-0005` § 4's and § 5's
tables and not a new design:

| Class | Signed | Encrypted to |
|---|---|---|
| Enrollment request | Agent's newly generated signing key | Not encrypted — it carries three public keys and a signature |
| Enrollment reply | Service signing key (`AST-7`) | Not encrypted — a user JWT is not a secret; the seed never left the agent |
| Command | Service signing key (`AST-7`) | Target agent (`AST-5`'s recorded public half) |
| Cancellation | Service signing key (`AST-7`) | Target agent (`AST-5`) |
| Result | Agent (`AST-4`) | Authorized result service (`AST-8`'s public half) |
| Lifecycle event | Agent (`AST-4`) | Not encrypted |
| Presence | Agent (`AST-4`) | Not encrypted |

### Why "refused" and not "rejected"

`ADR-0005` § 9 fixes the set of refusal reasons a receiver may return, and
`signature invalid` is one of them. The invariant uses that word so the
requirement lands on behaviour the protocol already specifies. A requirement to
sign without a requirement to refuse what does not verify is half a requirement.

### Why not-encrypted is stated rather than left out

Silence is what produced this RFC. `ADR-0005` § 5 decided that presence and
lifecycle events are signed and not encrypted for reasons that survive being
written down: encryption would protect what the subject and the timing already
reveal, and it would require recipient keys for the presence consumer and the
monitoring role, **neither of which holds one**, creating key material under
`RSK-12`, which has neither rotation nor revocation. An invariant that named
only the encrypted classes would leave a future reader unable to tell a decision
from an omission — the exact ambiguity being corrected.

### What this does not require

The invariant does not name algorithms, key sizes, canonicalization or wire
format. Those are `ADR-0005`'s and C01's, and an invariant that named them would
have to be amended for a cipher change.

## Compatibility & Upgrade Impact

**Not a breaking change, and not a change to any running system.** There are no
existing configurations, no controller/agent pairs in the field, no schemas, no
stored data, no plugins and no rolling upgrades: the Generation 2 code that
would implement this begins at P11.

No version bump. `VERSIONING.md` governs released artefacts, and this changes a
normative document in a pre-release repository.

**Compatibility with accepted decisions is the material question, and it holds
in both directions.** `ADR-0005` already signs every class, so the amendment
requires nothing the accepted design does not already do — no ADR becomes
non-compliant on merge. And because the amendment is additive, the three
sentences `ADR-0002`, `ADR-0003`, the charter, the glossary and the threat model
cite are unchanged.

## Migration Plan

None required, and this is not a vacuous statement: the ADRs written before this
RFC were checked against it rather than assumed compatible. `ADR-0005` § 4 signs
all seven classes; § 5's encryption set is the one stated here; `ADR-0003` § 4
generates separate keys; `ADR-0002` § 12 registers key separation. No accepted
document needs a change to satisfy the amended invariant. The changes this RFC
makes elsewhere are to documents that described the gap as **open**, which is now
false.

## Alternatives Considered

**Amend to name cancellation only.** Rejected. It is what `RSK-11` literally
describes, and it would repeat the error that produced `RSK-11` — correcting the
list without addressing why the list was short. Presence and lifecycle events
would remain unnamed, and `THR-50` (presence fabricated on the wire) rests on a
signature the invariant would still not require.

**Rewrite the invariant as a single general rule** — "every Keystone envelope is
signed; payloads carrying operator intent or captured output are encrypted to
their recipient" — and drop the enumeration. Rejected for two reasons. It would
delete the three sentences that `P05-acceptance-evidence.md`'s `AC-3` asserts
are present, silently invalidating accepted acceptance evidence. And a general
rule moves the argument to whether a given class "carries operator intent",
which is the kind of judgement an invariant exists to remove.

**Leave the invariant and rely on `ADR-0005`.** Rejected. An ADR is superseded
by a later ADR; that is the mechanism working as designed. An invariant is what
a later ADR is measured against. A requirement that must not be dropped silently
belongs in the second document, not the first.

**Require encryption on every class.** Rejected, and `ADR-0005` § 5 rejected it
first: new recipient keys for the presence consumer and the monitoring role,
under a risk with no rotation, to conceal what the subject and cadence already
disclose.

## Security Considerations

**Authentication and authorization.** The amendment makes the signature on five
classes a requirement rather than an ADR's choice. The threats that depended on
it — `THR-24` (a party cancels a job it does not own) and `THR-50` (presence
fabricated on the wire) — now rest on a normative requirement. `ARCH-NATS-006`'s
row in the threat model's invariant-coverage map gains `THR-24` for that reason.

**Trust model and PKI.** Unchanged. No new key material, no new trust anchor, no
new recipient. `RSK-12` — no rotation or revocation for `AST-7` and `AST-8` — is
untouched, and deliberately so: this RFC ratifies a design, it does not extend
one.

**Confidentiality.** Unchanged. The encryption set is exactly `ADR-0005` § 5's.
Metadata exposure is unchanged and remains `RSK-6`.

**What this does not fix.** A signature proves origin, not correctness. A
compromised agent signs true-looking results with its own key (`RSK-9`), and a
compromised server signs commands with `AST-7` (`RSK-4`). Both remain accepted
and neither is narrowed here.

## Observability & Operations

No new logging, metric or failure mode. `ADR-0005` § 9 already defines
`signature invalid` as a refusal reason and `ARCH-OBS-001` already requires
authorization denials to reach the audit record. An operator sees a refused
envelope exactly as they would have before.

## Rollout & Adoption

No flag, no phased enablement, nothing to opt into. The amendment takes effect
on merge, and the first code measured against it is C01's.

`REQUIREMENTS-TRACEABILITY.md`'s row for `ARCH-NATS-006` planned
`test/e2e/docker/envelope_confidentiality_test.go`, which covers confidentiality
and not signing. The register now names an envelope-signature test alongside it,
because `ARCH-TEST-003` requires every invariant to have automated evidence and
a confidentiality test cannot demonstrate that a presence envelope is signed.

## Open Questions

**None blocking.** One is recorded rather than answered: whether an invariant
should enumerate at all, or whether enumeration is a symptom of the product
being young enough that the list is short. This RFC enumerates because the list
is knowable today and a general rule would be argued about; if the class list
grows past what a reader can hold, that is a reason to revisit the form, not
this decision.

## Prior Art

Kubernetes API conventions treat an unlisted field as unset rather than
unconstrained, and add fields through a documented change rather than by
implication. Go's compatibility promise is explicit about what it covers so that
what it does not cover is also explicit. Both are the same move this RFC makes:
the value of a closed enumeration is that silence stops being ambiguous.

## Decision

- **Outcome:** Accepted.
- **Notes:** Accepted on merge by the project maintainer, together with the
  amendment it authorises. `REQUIREMENTS-TRACEABILITY.md` § Maintenance rules
  compelled an RFC only for *removing or weakening* a requirement, while
  `REBOOT-EXECUTION-PLAN.md` stated that amending a normative invariant requires
  one. This RFC is the first amendment either rule has ever governed, so the
  contradiction is settled here rather than left for the second: the maintenance
  rule now says that changing a normative invariant's text requires an RFC in
  either direction, which is what the plan and `ARCHITECTURE-INVARIANTS.md`'s own
  header already said.

## References

- [RFC 0001 — Generation 2 reboot](0001-generation-2-reboot.md)
- [ADR-0005 — Versioned encrypted protocol](../adr/0005-versioned-encrypted-protocol.md)
- [ARCHITECTURE-INVARIANTS.md](../project/ARCHITECTURE-INVARIANTS.md)
- [REQUIREMENTS-TRACEABILITY.md](../project/REQUIREMENTS-TRACEABILITY.md)
- [THREAT-MODEL.md](../project/THREAT-MODEL.md)
