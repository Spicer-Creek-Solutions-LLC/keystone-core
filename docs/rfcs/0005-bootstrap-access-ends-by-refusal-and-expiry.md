# RFC 0005: Bootstrap access ends by refusal and expiry

- **Status:** Accepted
- **Decision date:** 2026-09-23
- **Decision owner:** Project maintainer
- **Amends:** [`ARCH-NATS-004`](../project/ARCHITECTURE-INVARIANTS.md);
  [`PRODUCT-CHARTER.md`](../project/PRODUCT-CHARTER.md) § 3's enrollment
  measure and § 5.1; [`ADR-0003`](../adr/0003-enrollment-and-identity.md)
  §§ 1, 2, 6, 7, 8 and 10; [`ADR-0004`](../adr/0004-subject-authorization.md)
  §§ 7 and 9
- **Corrects:** [`ADR-0002`](../adr/0002-nats-native-capabilities.md) § 13,
  whose revocation claim is false against the broker this project pins
- **Settles:** [`C05.md`](../dossiers/C05.md)'s `D-C05-1`, `D-C05-9`, and the
  S6 half of `D-C05-3`. **C05 stays blocked** on `D-C05-8`, TLS, which is
  `G49`'s

## Summary

`ARCH-NATS-004` requires enrollment to *revoke bootstrap access and verify the
revocation*, and `ADR-0003` § 6 assigns both to the server. **The server cannot
revoke.** A NATS revocation is a change to the account JWT, only the operator
may sign an account JWT, and `ADR-0002` § 2 keeps the operator seed out of every
Keystone process. This was measured, not inferred, and the failure is worse
than a refusal: the broker reports success, does not enforce it, and a re-read
of the account confirms a revocation that is not in force.

This RFC changes how bootstrap access ends. **The server refuses the spent token
at once, and the broker refuses the bootstrap credential at its expiry**, a
lifetime of minutes that the broker already enforces. Nothing re-reads claims,
because the measurement shows a re-read can lie.

It settles two related questions on the way. **The token carries no separate
secret**: the bootstrap credential is the secret. And it decides now, without
building it, **how a permanent identity will be revoked**: the server refuses it
at once, and permanent credentials become short-lived and server-renewed, so
that revoking an identity is declining to renew it.

## Motivation

### The measurement

Every row below ran against `nats:2.15.0-alpine`, the broker `C03-A` pins, with
this module's `nats-io/jwt/v2` and `nats.go`. The deployment had an operator, a
system account, and a Keystone account carrying one signing key. The update
listed one bootstrap user as revoked and was pushed with
`$SYS.REQ.CLAIMS.UPDATE`.

| Resolver | Account JWT signed by | Broker's reply | Revoked bootstrap user | An unrevoked user, same account |
|---|---|---|---|---|
| `full` | **account signing key**, account loaded | `200 jwt updated`; log: `account validation failed` | **still connects** | connects |
| `full` | **account signing key**, account not yet loaded | `200 jwt updated` | refused | **refused: the account is unusable** |
| `full` | operator key | `200 jwt updated` | refused | connects |
| `MEMORY` — what C03 generates | either | no responder | still connects | connects |

Three findings follow, each worse than the last.

1. **The server's key cannot revoke.** `ADR-0002` § 13 says a revocation is *"a
   broker operation the server can perform with its account signing key"*. It
   is not. An account signing key signs **users**; the revocation list lives in
   the **account** JWT, which only the operator may sign.
2. **The failure is silent.** The broker acknowledges an update it will not
   apply, and `$SYS.REQ.ACCOUNT.<id>.CLAIMS.LOOKUP` then returns the stored JWT
   **listing the revocation**. `ADR-0003` § 6's S6, *"re-read the account state
   and confirm the bootstrap identity is listed"*, therefore reports success for
   a revocation that is not in force. The verification the invariant exists for
   cannot detect this failure.
3. **After a broker restart it takes the account down.** The invalid stored JWT
   is the one the broker loads, and every Keystone identity is refused.

`C03-I`'s `NEG-7` passes, and remains correct: it proves a revoked identity
cannot connect when the **generator** revokes it, with the operator key, at
generation time. It never claimed the server could.

### Why the obvious fixes were not taken

**Give the server an operator signing key.** NATS scopes signing keys only at
the account level, for users. An operator signing key signs **any** account
under the operator, including the system account. A compromised server could
then loosen the Keystone account, create accounts, grant itself broker
administration, and reach another workload's account on a shared broker
(`ADR-0002` § 14, `THR-30`). That is the authority `ADR-0002` § 2 refused the
server when it refused the seed, and a signing key is most of the same power.

**Run a separate revoker that holds the operator key.** Custody stays correct,
and a revoker could validate that it only ever revokes bootstrap-shaped users.
But it is a new component, a new trust boundary, a new dependency for every
enrollment, and a new daemon for an operator the charter wants on their first
command in ten minutes. Its price buys the closing of a window this RFC shows
to be nearly powerless.

### What the bootstrap identity can do after S4

Its grants are two subjects wide (`ADR-0003` § 3, `ADR-0004` § 4): publish to
its own `ks.enroll.<token>.request`, and subscribe to its own
`ks.enroll.<token>.reply`. Once the server refuses the spent token, a holder of
the credential can:

- publish into a subject whose only reader refuses it and records the attempt;
- subscribe to a subject that carries nothing confidential (`ADR-0003` § 4);
- hold a connection, bounded by the account's connection limit
  (`ARCH-NATS-007`), until the credential expires.

It reaches no agent, no job, no stream and no other token. **Broker revocation
would close a window in which the identity can do nearly nothing.** Closing it
is worth a key custody change or a new component only if it is worth more than
that, and it is not.

### Who is affected

No operator and no deployment. C05, the first task that runs enrollment, has
not started.

## Design Overview

Three decisions.

1. **Bootstrap access ends by the server's refusal and the credential's
   expiry**, not by broker revocation. The agent's exit `0` rests on the
   server's confirmation, over a channel that now survives to deliver it.
2. **The token is an identifier, and the bootstrap credential is the secret.**
3. **A permanent identity will be revoked by application refusal and
   non-renewal**, with the broker administrator's offline revocation as an
   emergency path. It is decided here and built when a decommission journey is
   promoted.

## Detailed Design

### 1. How bootstrap access ends

`ADR-0003` § 6's stages S0, S1, S2 and S4 are unchanged. **S3 changes in one
respect**: the agent keeps its bootstrap connection open while it connects and
proves the permanent one, instead of closing it first, because S6 needs it. S5
and S6 become:

| Stage | Server state | What happens |
|---|---|---|
| S5 | `active`, token `spent` | In the same durable write as S4, the server records the token **spent**. From here, every request on the token from any identity is refused, except the confirmation below, and any refusal is an audited denial |
| S6 | `active`, token `spent` | The agent, still holding its bootstrap connection, asks for confirmation on its token-scoped subject. The server replies that the identity is active and the token spent. **The agent exits `0` only on that reply**, then closes the bootstrap connection and removes the bundle |

Bootstrap access then ends at the credential's **expiry**, which the broker
enforces: it disconnects a live connection at the moment its JWT expires and
refuses new ones (measured). The server enforces the same expiry against its
own record, as `ADR-0003` § 8 already required. **Neither is trusted alone.**

**This also settles `D-C05-3`'s S6 half.** Under the old design the agent had no channel
on which to hear S6: revocation ended the bootstrap connection, and nothing in
`ADR-0004` § 4's matrix reaches a permanent identity outside JetStream. With
nothing revoking the bootstrap identity, its own reply subject stays open until
the agent closes it, and that is the channel. **No grant is added.** The
confirmation carries nothing confidential: the identity is active, and the
permanent JWT is not a secret.

**What "verified" now means.** The server verifies nothing by re-reading the
broker; the measurement shows a re-read can report a revocation that is not
there. The two facts are proved where they are true:

- **refusal** by the server's durable `spent` state, confirmed to the agent at
  S6;
- **expiry** by the broker itself. Acceptance asserts it by connecting with the
  expired credential and being refused, never by reading claims.

### 2. The token has no separate secret

`ADR-0003` § 1 generates *"a high-entropy random identifier and secret"*, but
§ 5's six checks never test a secret. Presenting one would contradict § 4's
*"nothing confidential crosses the enrollment path"*: the enrollment request is
signed and not encrypted (`ADR-0005` § 7), and no key exists that the
enrollment service could decrypt with.

**The bootstrap credential is the secret.** The bundle holds its seed, and
nothing else does. § 5's fourth check, that the presenting identity is the one
minted for this token, is performed by the broker, which authenticates that
seed before a single byte reaches the server. The token becomes an identifier:
high-entropy, unguessable, and not confidential on its own. § 1 and § 2 drop
*"and secret"*; everything that protects the bundle still applies, because the
bundle still holds a credential.

### 3. How a permanent identity will be revoked

`ADR-0003` § 10 said *"the operator revokes the old identity explicitly"*, and
the same custody problem applies, so this RFC amends § 10 to the design
below. The operator still acts explicitly; the server still never infers that
a new enrollment replaces an old one. No charter journey revokes an agent yet, so
this is **decided and not built**:

| Layer | When | By whom | Mechanism |
|---|---|---|---|
| Application refusal | Immediately | The server | The agent is recorded revoked: no commands are sent to it, and its results and presence are rejected. Its keys stop being trusted |
| Broker cut-off | Within the credential's lifetime | Automatic | **Permanent JWTs are short-lived and renewed by the server**, which signs them with the account signing key it already holds: that key signs users, which is exactly what this is. Revoking is declining to renew. The broker disconnects the identity when its JWT expires |
| Emergency cut-off | When the administrator acts | The broker administrator, offline | The operator key, which never enters Keystone, re-signs the account with the identity revoked. A configuration reload applies it. Measured with the `MEMORY` resolver: the revoked identity's live connection is closed at once, and another identity in the account is unaffected |

**Constraints on whichever task builds it:**

- **Lifetimes and renewal ship together.** A permanent JWT with an expiry and no
  way to renew it is a fleet that expires.
- **Renewal needs a channel**, and `ADR-0004` § 4 has none from a service to a
  permanent identity outside JetStream. That task amends the matrix.
- **An agent offline longer than the lifetime re-enrolls**, because an expired
  credential cannot connect to ask for renewal. The lifetime is configuration,
  so a deployment trades revocation speed against offline tolerance.
- **Until then, C05 mints permanent JWTs without an expiry**, so that no agent
  can expire before renewal exists.

## Compatibility & Upgrade Impact

None. No deployment exists and no product code enrolls anything. This is not a
breaking change and needs no version bump.

**C03's output is unaffected.** The generated configuration keeps
`resolver: MEMORY`: nothing now pushes an account update at runtime, and the
emergency path uses a configuration reload, which that resolver supports.
`NEG-7` stays a correct case, and now describes the offline path the
administrator can use.

## Migration Plan

No migration is required.

## Alternatives Considered

**The server holds an operator signing key.** Rejected: see § Motivation. It
restores `ADR-0003` § 6 exactly, and gives a compromised server authority over
every account on the broker, the system account included.

**A separate revoker holding the operator key.** Rejected for the bootstrap
case: a new component to close a nearly powerless window. The **offline**
version of the same custody, the administrator with the operator key, is kept
as the emergency path for permanent identities.

**Verifying revocation by re-reading the account.** Rejected, and it is the
alternative with evidence against it: the broker returned the stored JWT
listing a revocation it was not enforcing.

**NATS auth callout**, with the server deciding at connect time. Not measured.
It would give instant refusal at connection without an operator key, but it is
configured in the account JWT, likely needs bootstrap users in an account of
their own, and still needs a system-account credential to end an existing
connection. It reshapes `ADR-0002`'s two-account design to improve on a window
this RFC accepts. It stays available if that window ever matters.

## Security Considerations

**`THR-03` is bounded, not closed.** A copied bundle can connect until the
token expires, and can do only what § Motivation lists. The token's lifetime is
short by `ADR-0003` § 2, minutes and configurable at creation, and it is the
whole exposure. `THREAT-MODEL.md` records it as `RSK-15`.

**Key custody is unchanged.** The server holds the account signing key and
nothing above it, as `ADR-0002` § 2 requires. The operator key stays with the
broker administrator.

**Expiry depends on the broker's clock.** `ADR-0003` § 12 already states that
assumption, and its bound still holds: a token lifetime of minutes against a
skew bound of a minute.

**`THR-02` is unaffected.** Whoever presents the bundle first still enrolls;
revocation never protected against that, and one-use still does.

## Observability & Operations

Every refusal of a spent token is an audited denial (`ARCH-OBS-001`). Expiry
emits nothing, because the server does not observe the broker's enforcement,
and the acceptance suite is where it is proved.

The emergency path is an operator procedure, not a product command, until a
decommission journey exists. It needs no product change, because the broker
configuration is the administrator's.

## Rollout & Adoption

The amendments below land with this RFC. C05 implements § 1 and § 2. § 3 is
built by the task that promotes agent decommission.

## Open Questions

- **The token's default lifetime.** `ADR-0003` § 2 says *minutes, not hours*.
  C05 chooses the number, and it is the whole of `RSK-15`'s exposure.
- **The permanent credential's default lifetime**, when § 3 is built. It trades
  revocation speed against how long an agent may be offline.
- **An agent that restarts after its bootstrap credential expired, between S4
  and S6.** Its permanent identity works and the server holds it `active`, but
  the confirmation can no longer arrive. C05's contract decides what the agent
  reports, and must not report exit `0` without the confirmation the charter
  requires.

## Prior Art

- **Short-lived, renewed credentials** are how SPIFFE and short-TTL X.509
  deployments avoid depending on revocation lists at all.
- **Expiry as the end of access** is the NATS user JWT's own `exp`, which the
  broker enforces on live connections.

## Decision

- **Outcome:** Accepted.
- **Notes:** The maintainer compared the three options in turn: an operator
  signing key on the server, a separate revoker, and expiry with refusal.
  They chose the third, and then asked for a revocation path for permanent
  identities that needs no administrator step, which is § 3's renewal layer.
  **TLS on every connection (`ADR-0002` § 12) is out of scope here** and is
  assigned to `G49`; C05 stays blocked on it.

## References

- [`ARCHITECTURE-INVARIANTS.md`](../project/ARCHITECTURE-INVARIANTS.md) — `ARCH-NATS-004`
- [`ADR-0002`](../adr/0002-nats-native-capabilities.md) §§ 2, 13, 14
- [`ADR-0003`](../adr/0003-enrollment-and-identity.md) §§ 1–8, 10, 12
- [`ADR-0004`](../adr/0004-subject-authorization.md) §§ 4, 7, 9
- [`C05.md`](../dossiers/C05.md) § 11
- [`THREAT-MODEL.md`](../project/THREAT-MODEL.md) — `THR-02`, `THR-03`, `THR-30`, `RSK-15`
- [NATS decentralized JWT authentication](https://docs.nats.io/running-a-nats-service/configuration/securing_nats/auth_intro/jwt)
