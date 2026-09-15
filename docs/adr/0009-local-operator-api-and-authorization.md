# ADR-0009: Local operator API and authorization

- **Status:** Proposed
- **Date:** 2026-09-15
- **Task:** P09, bounded by [`docs/dossiers/P09.md`](../dossiers/P09.md)
- **Builds on:** [`ADR-0002`](0002-nats-native-capabilities.md), [`ADR-0003`](0003-enrollment-and-identity.md), [`ADR-0004`](0004-subject-authorization.md), [`ADR-0005`](0005-versioned-encrypted-protocol.md), [`ADR-0006`](0006-delivery-and-job-lifecycle.md), [`ADR-0007`](0007-safe-execution.md), [`ADR-0008`](0008-persistence-and-audit.md), [RFC 0003](../rfcs/0003-caller-selected-execution-user.md), [RFC 0004](../rfcs/0004-what-the-execution-exclusions-constrain.md)

## Context

**This is the only place a human authenticates to this product.** Every other
identity in Generation 2 is a key or a NATS credential held by a process;
`ARCH-COMM-001` puts every agent interaction on NATS and leaves exactly one
inbound surface for a person, and this ADR defines it.

Every Stage P ADR deferred its operator-facing half here. `ADR-0003` § 1 said
*which* operators may create tokens is P09's. `ADR-0004` § 13 deferred which
operators may do what. `ADR-0005` § 13 deferred the error an operator sees,
`ADR-0006` § 13 the rendering of every lifecycle state, `ADR-0007` § 5 how a
harvest fallback surfaces, `ADR-0008` § 11 how a retrieved record is presented,
and RFC 0003 § Observability the fallback's visibility.

**Most of those deferrals are not answered here, and that is deliberate.** They
divide into two kinds. *Which operators may do what* is an authorization question
and is § 4. *What an operator sees* is presentation, belongs to C11's diagnostics
UX, and an ADR that invented output formats would be designing a CLI six tasks
before one exists. § 13 records the split so the next reader does not mistake
silence for an omission.

**What makes this ADR hard is that its central decision is nearly a no-op.**
`REBOOT-EXECUTION-PLAN.md` fixes full-admin authorization for the first release,
so "which operators may do what" resolves to *all of it*. The temptation is to
write one sentence and move on. The cost of doing that is paid twice: the audit
record needs an operation identifier now, and a later RBAC would have to invent a
dimension rather than fill one in. § 4 is therefore an enumeration whose every row
is identical, and the enumeration rather than the values is the decision.

## Decision

### 1. The socket, and which domain owns it

| Path | Owner | Group | Mode |
|---|---|---|---|
| `/run/keystone/` | `root` | the configured admin group | `0750` |
| `/run/keystone/operator.sock` | `root` | the configured admin group | `0660` |

The directory is traversable only by the group, so a principal outside it cannot
`stat` the socket, learn whether the server is running, or race its creation. The
socket's own mode is what the kernel enforces on `connect()`.

**The socket belongs to `TD-SRV`, not to `TD-OP`.** The threat model lists
`AST-14` among `TD-OP`'s assets — the operator's workstation, its CLI and its
configuration — while `TB-1` makes the socket the *boundary* between the two
domains. Ownership decides who may widen it, and it must be the server:

- The server creates the file, sets its mode, and is the only party that may
  change either.
- **If the socket were the operator's, a compromised workstation could loosen its
  own authorization** — `RSK-8` would extend from *acting with the operator's
  authority* to *granting that authority to anyone*, which is a strictly larger
  risk than the one accepted.

The `AST-14` row is amended to record the domain. The asset is not moved: it sits
on a boundary, and § 12 states why naming the owner is the useful half.

**One owning group, and that is the whole list.** A filesystem socket has exactly
one owning group, so the set of principals who may connect is *the members of the
configured admin group*, plus `root`, which bypasses mode checks entirely.
Expressing a broader set would require POSIX ACLs; that is rejected in
§ Alternatives, and a deployment needing more principals adds them to the group.

**The group is configured, never defaulted to a group that already exists.** A
product that defaults to `wheel`, `sudo` or `adm` grants control-plane access to
whoever a distribution decided should have administrative rights, which is a
different and larger population. An unstated group is a configuration error, and
the server refuses to start — the same shape RFC 0003 § "The default user"
requires of the execution user, for the same reason.

### 2. Who may connect

An enumerated principal set, not a permission bit:

| Principal | May connect | Why |
|---|---|---|
| A member of the configured admin group | Yes | The deployment named them |
| `root` | Yes | Bypasses mode checks; no design can prevent it |
| Anyone else | No — refused by the kernel | § 5 states what they see and § 6 what is recorded |

**The socket stays restricted.** It is *not* opened widely in order to make
denials observable. An arbitrary local user who could reach `accept()` could
generate audit records by connecting in a loop, and the population whose denials
would be recorded is everyone on the host — which is not an audit trail, it is
noise with a storage cost.

### 3. Where authorization is evaluated

**Stated as an ordering obligation**, in the shape `ADR-0006` § 3 uses for
durability, because a description of an ordering is not a requirement for one:

| Before this happens | This must have completed |
|---|---|
| Any byte of the request is read | `SO_PEERCRED` has been read for the connection |
| Any byte of the request is read | The peer's authority has been decided from those credentials |
| Any byte of the request is **parsed** | The connection has been authorized |
| A denial response is written | The denial's audit record is durable (`ADR-0008` § 6) |

**The credentials come from the kernel, never from the wire.** `SO_PEERCRED`
reports the uid, gid and pid the kernel recorded when the peer called
`connect()`. Nothing the caller sends can alter it, which is the property that
makes authorizing before parsing possible at all.

**So no unauthenticated input reaches the parser.** That is the same boundary
`ADR-0007` § 7 draws inside the agent — the component that parses untrusted input
holds no privilege — applied to the one surface where the untrusted party is a
local process rather than the network.

**The in-band check is not redundant, and this is the paragraph that explains
why.** Supplementary group membership is resolved at login and **cached in a
process's credentials**. An operator removed from the admin group keeps that
membership in every already-running shell, so the kernel's own check on
`connect()` still admits them. Only a re-derivation against *current* membership
denies them.

**A revoked operator's attempt is precisely the denial an audit record exists
for**, and it is invisible to the socket's mode. Two further cases share the
shape: `root`, which connects regardless of the mode and is authorized only if
§ 4 says so; and a deployment whose socket mode was set wrongly, where the
in-band check fails closed instead of silently granting access.

### 4. What an operator may do

The charter's § 5 journeys, each mapped to the authority it requires. **Every row
is the same value**, and that is this release's policy rather than an oversight:

| Journey | Authority required |
|---|---|
| § 5.1 Enroll | `admin` |
| § 5.2 List and presence | `admin` |
| § 5.3 Run | `admin` |
| § 5.4 Status | `admin` |
| § 5.5 Output | `admin` |
| § 5.6 Cancel | `admin` |
| § 5.7 Audit | `admin` |

**The table is a projection of the charter, not a second enumeration.** P09
defines no list of its own: the journeys are the charter's and the rows follow
them, so there is one source of truth. Two statements of one fact drifting apart
is this project's most-recurring defect, and a second journey list would be one.

**The enumeration is the decision; the values are the policy.** A future release
that wants RBAC changes cell values in a table that already exists. Without this
table it would have to invent the dimension — a per-request authority concept, a
place to record which operation was attempted, and a migration for audit records
written before either existed.

**It also closes deferrals that were open.** `ADR-0003` § 1's *which operators may
create tokens* is § 5.1's row. `THR-24`'s *which operators may cancel which jobs*
is § 5.6's: any admin may cancel any job, and per-job ownership does not exist in
this release. `ADR-0004` § 13's deferral is the table entire.

**A table whose every row is identical cannot disagree with itself**, so it proves
nothing on its own. The acceptance case reads the journeys **out of the charter**
and asserts the mapping is total; that is a property that can fail, and the cell
values are not.

**Authority is not a claim in the request.** It is derived in § 3 from the peer's
credentials. There is no field an operator can set, which is § 7.

### 5. Exit codes, and the two refusal paths

The codes are the charter's; none is invented here.

| Situation | Code | Where decided |
|---|---|---|
| The operation succeeded | `0` | The server |
| Bad arguments, unreadable configuration | `1` | The CLI, before connecting |
| **The connection is refused by the kernel** | `10` | The kernel; mapped by the CLI |
| **The peer connects and is not authorized** | `10` | § 3's in-band check |
| The socket does not exist, or nothing is listening | `1` | The CLI |

**`EACCES` on `connect()` maps to `10`.** The charter delegates the placement —
*"`10` applies to every command whose invocation is subject to operator
authorization; P09 defines where that is evaluated"* — and a connection the kernel
refused on the socket's permissions **is** authorization denied. Mapping it to `1`
would be wrong on the charter's own words: `1` is *"local or usage error: bad
arguments, unreadable configuration"*, and an `EACCES` is none of those.

**The two paths are deliberately indistinguishable to the caller.** Both exit
`10` and both report that authorization was denied. A message that told an
unauthorized caller *which* check refused them would report whether they are
outside the group or merely stale within it, which is information about the
deployment's configuration offered to the one party who should not have it.

**They are distinguishable to the operator of the server**, because only one of
them produces a record — which is § 6.

`ENOENT` is `1` rather than `10`: nothing refused the caller, and telling someone
the service is down is not an authorization decision.

### 6. What a denial records, and what it cannot

`ARCH-OBS-001` names *authorization denial* among the transitions that must emit
a record, and `ADR-0008` § 6 stores it. This ADR decides which denials exist to be
stored.

| Denial | Recorded | Why |
|---|---|---|
| A connected peer fails the in-band check | **Yes** — a full record: the uid, the resolved name, the attempted operation where one is known | The server observed it |
| A member's session whose group membership is stale | **Yes**, by the row above — this is its most common cause | The connection succeeded; the authority did not |
| `root` connecting without authority under § 4 | **Yes** | Same |
| **The kernel refuses `connect()`** | **No** | § 12 |

**The unrecorded set is named rather than left as an absence.** A principal the
kernel refuses never reaches the server, so Keystone cannot record what it did not
observe. **That set is exactly "principals the deployment never granted access
to"** — not operators who were revoked, whose stale sessions still connect and are
recorded by row two.

Whether this satisfies `ARCH-OBS-001` rests on an event Keystone never sees not
being a transition in Keystone's lifecycle. **That reading is stated here so it
can be disputed**, because a compliance claim that depends on an unstated
interpretation is the shape `DL-1` exists to catch. § 12 records the cost.

### 7. The audit identity, and impersonation

`ADR-0008` § 6's **Actor** field says only *"Who caused it"*. For an operator it
is two parts:

| Part | Source | Status |
|---|---|---|
| **uid** | `SO_PEERCRED` | **Authoritative.** It is what the kernel reported and it never changes meaning |
| **username** | Resolved from the uid at the time of the request | **A snapshot.** Recorded for legibility, and it is not evidence |

**A username is a claim about a moment stored as a fact.** Delete the account,
reuse the uid, and a record written last March now names a person who never ran
it — with nothing in the record to reveal that it happened. Recording the uid
keeps the identity stable; recording the name keeps the record readable at three
in the morning; marking which is which keeps a reader from trusting the wrong
half.

**The schema states that this is the identity exercised and not the person.**
`RSK-8` already concedes it, and a concession that lives only in the risk register
is invisible to whoever is actually reading an audit row.

**The pid is not recorded.** `SO_PEERCRED` reports it, it is useful for minutes
while the process still exists, and it is aggressively reused thereafter. Storing
it would invite exactly the over-reading this field is trying to prevent.

**No request field may set the acting identity.** There is no `--as`, no actor
field in the request, and no configuration key that changes who a request is
attributed to. **What this forecloses is real**: an automation account cannot run
a job "on behalf of" a named human, and a shared service account produces records
naming the service account rather than whoever invoked it. That is the correct
trade for a release with one authority level — an impersonation field is an
authorization mechanism, and this release has no authorization model to bound it.

### 8. Stale-socket recovery

A crashed server leaves a socket file behind. Binding without removing it fails
with `EADDRINUSE`; **removing it unconditionally is a hijack primitive.**

The obligation, in order:

| Before this happens | This must have completed |
|---|---|
| The path is unlinked | The file has been confirmed to be a socket, owned by `root`, in a directory only `root` may write |
| The path is unlinked | A `connect()` to it has been confirmed to fail with `ECONNREFUSED` |
| The listener is bound | The stale path has been unlinked |

**The directory's ownership is what closes the race.** Checking a path and then
unlinking it is a time-of-check-to-time-of-use window in general: an attacker who
can replace the path between the two steps redirects the unlink. Here they cannot,
because `/run/keystone/` is `root`-owned and mode `0750` — **only `root` may
create, rename or remove entries in it**, so no unprivileged party can substitute
the path at any point in the sequence. The check is safe because of the
directory, not because of the check.

A path that exists and is **not** a socket, or is a socket with an unexpected
owner, is **not** removed. The server refuses to start and says so. That is a
deployment that has been tampered with or misconfigured, and silently deleting the
evidence is the wrong response to both.

`ECONNREFUSED` distinguishes a stale socket from a live one. A socket that accepts
a connection belongs to a running server, and starting a second one would give the
deployment two servers racing on one store — which `ADR-0008` § 1 has no design
for.

### 9. Request limits

Stated per stage, because the stage before authorization is the one an attacker
reaches first.

| Limit | Applies | Why |
|---|---|---|
| Maximum concurrent **unauthorized** connections | Before authorization | A caller inside the group who is no longer authorized can otherwise hold connections open |
| Authorization timeout | Before authorization | A peer that connects and sends nothing must not hold a slot |
| Maximum concurrent authorized connections | After | Bounded resources (`ARCH-NATS-007`'s principle, applied locally) |
| Maximum request size | After | A frame larger than this is refused before allocation |
| Maximum in-flight requests per connection | After | Prevents one connection monopolising the server |

**The first two exist because § 3 authorizes before parsing.** The connection is
accepted before authority is known, so the window between `accept()` and the
decision is a resource an unauthorized peer can consume. The set is small — only
group members and `root` reach it — but "small" is not "bounded", and a limit is
what makes it bounded.

Numbers are **not** fixed here. They are C04's and C15's, and a number invented in
an ADR without a load profile behind it is a number that gets copied forward
unexamined.

### 10. The framing, stated only where authorization depends on it

This ADR does not design a wire protocol; that is C04's. It fixes the one property
§ 3 depends on:

**The request framing must not require reading request bytes to determine whether
the caller is authorized.** Authorization is a function of the connection, not of
its content, so no negotiation, no capability exchange and no header may precede
the decision.

This is stated because it is the way § 3 could be quietly defeated: a framing that
began with a version handshake would have the server parsing attacker-supplied
bytes before authorizing, which is § 3's rejected alternative arriving by
accident. **Version negotiation, if C04 needs one, happens after authorization.**

### 11. What the operator API must never become

`ARCH-COMM-001` permits a local socket and adds: *"it must not become a hidden
agent transport."* An intention will not hold that. The property:

**No request on this socket may cause the server to accept, originate or relay a
message on behalf of an agent, and no agent may reach this socket.** Concretely:

- The socket is local and bound to a filesystem path. It has no network address,
  and `ARCH-COMM-002`'s isolated-network acceptance means an agent has no route to
  the server's filesystem in the first place.
- Every operator action in § 4 that reaches an agent does so **by publishing to
  NATS** on the subjects `ADR-0004` fixed. There is no path from this socket to an
  agent that does not traverse the broker.
- **The test that distinguishes a legitimate feature from a violation**: if
  removing the broker would leave an operator action still able to reach an agent,
  the action is a second transport. That is checkable, and it is what P11's
  architecture lint would enforce.

**The erosion this guards against is incremental.** No one adds an agent transport
deliberately; someone adds a diagnostic that reads an agent's state directly
because the broker round-trip was inconvenient. The property above is written so
that such a change fails a stated test rather than a reviewer's memory.

`ADR-0002` § 6 deferred `allow_responses` against this API acquiring a
request/reply surface. **It has not acquired one**: the operator API is
operator-to-server, and server-to-agent traffic is unchanged. That deferral stays
open on its original terms.

## Residual risks

| ID | Disposition |
|---|---|
| `RSK-8` | **Renewed, with a stronger control and a sharper exposure.** Expires 2027-03-13, or **C04** |

**`RSK-8` cannot resolve.** Distinguishing the operator from code running in the
operator's session requires attestation or a second factor. Neither is in
Generation 2's scope, neither is proposed, and claiming otherwise would be a
statement with nothing behind it.

**What P09 adds is that the actor is kernel-derived.** § 3 takes the identity from
`SO_PEERCRED` and § 7 forbids any request field that sets it, so **a caller cannot
forge an attribution to another operator**. The current control says only that a
record exists; it does not say the record's actor is trustworthy, and until now
nothing guaranteed it.

**This is disclosure, not prevention.** Malware in the operator's own session is
attributed to the operator, correctly and uselessly — it *is* acting with that
identity. Nothing here reduces what such an attacker can do. The control got
harder to lie to; the risk did not get smaller, and "strengthened" must not be
read as "smaller".

**The exposure is also more concrete than when `RSK-8` was written.** After RFC
0003 the operator's full authority includes running argv **as root on every
enrolled host**, and § 4 puts token creation, execution, cancellation and audit
read behind one group membership. `THR-35` predicted this — *"P09's authorization
model will share whatever authority the compromise holds"* — so it is a forecast
confirmed, not a widening.

**Expires at C04**, the first task that builds the socket and could demonstrate
peer-credential attribution working rather than assert it. The calendar date is
unchanged at 2027-03-13: a renewal that also bought twelve months would be two
decisions wearing one word.

## Invariant coverage

| Invariant | How |
|---|---|
| `ARCH-COMM-001` | § 1 defines the permitted local socket; § 11 states the property that keeps the hidden-transport clause true, with a test |
| `ARCH-OBS-001` | § 6 defines which authorization denials emit a record, and names the set that cannot |
| `ARCH-COMM-002` | § 11 — the API is local and creates no reason to dial an agent |
| `ARCH-COMM-003` | The socket is a production path; § 10 forbids a framing that would need a test-only shortcut |
| `ARCH-NATS-003` | § 4's journeys map to publish permissions `ADR-0004` already fixed; none is widened |
| `ARCH-TEST-002` | § 2's *anyone else* is a negative identity C14 must exercise |

## Alternatives considered

**Authorization by socket mode alone, with no in-band check.** Rejected. It is
simpler and it cannot deny a revoked operator whose shell still carries the cached
group — the denial that matters most — and it produces no `ARCH-OBS-001` record
for any denial at all.

**Accept connections widely and authorize from the request.** Rejected. It makes
every denial observable at the cost of letting unauthenticated peers reach the
parser, which is the surface `ADR-0007` § 7 split the agent apart to avoid. Buying
an audit line with a parse surface is the wrong trade in a product whose one
promise is bounded execution.

**Opening the socket widely so that every denial is recorded.** Rejected, and it
was the position this design started from. It makes an audit-flood available to
any local user and records denials from a population that was never a candidate
for access. Restricting the socket costs the records in § 6's last row and keeps
everything that matters, because a *revoked* operator still connects.

**POSIX ACLs, to name several groups and users on the socket.** Rejected. It would
express "a small list of well-known principals" directly rather than through one
group's membership, and it adds a permission model with different semantics on
every filesystem, a second place for authorization to live, and a state that no
`ls -l` reveals. One group whose membership *is* the list has the same
expressiveness and one place to look.

**A default admin group of `wheel`, `sudo` or `adm`.** Rejected. It would make the
product work out of the box by granting control-plane access to whoever the
distribution decided should hold administrative rights. Refusing to start without
a configured group is RFC 0003 § "The default user"'s shape, for the same reason.

**A second authority level — admin and read-only.** Rejected here, not on its
merits. Read-only is the distinction with the clearest operational value and it
would genuinely narrow `RSK-8` for most humans. It contradicts
`REBOOT-EXECUTION-PLAN.md`, which fixes full admin for the first release and can
be amended only by a `G` task. § 4's table is what makes adding it later a change
of values rather than a change of shape.

**Recording the pid alongside the uid.** Rejected — § 7.

**An impersonation field, so automation can act for a named human.** Rejected —
§ 7. It is an authorization mechanism, and this release has no authorization model
to bound it.

## Consequences

**C04 is the task that makes this ADR true**, and it inherits an ordering
obligation rather than a suggestion: § 3's table and § 8's are both written the way
`ADR-0006` § 3 writes durability, because both are properties that a plausible
implementation can satisfy in the wrong order.

**C13 must create the admin group and the directory**, and the server refuses to
start without them. That is a packaging obligation, not a runtime fallback.

**C11 inherits every presentation deferral** this ADR did not answer, listed in
§ 13.

**C14 gets three concrete adversaries**: a caller outside the group, a revoked
operator whose session still holds the cached membership, and a stale-socket
substitution attempt.

**An operator's uid is now load-bearing.** Rebuilding a workstation and getting a
different uid makes old audit records harder to attribute, and reusing a uid makes
them wrong. That is inherent to attributing to the kernel's identity, and § 7's
annotation is what makes it survivable.

## What this ADR does not decide

**Presentation.** `ADR-0005`'s error text, `ADR-0006`'s rendering of each
lifecycle state, `ADR-0007`'s refusal and truncation messages, `ADR-0008`'s record
display, and RFC 0003's fallback notice are all **C11**'s. This ADR decides *who
may ask*; what the answer looks like is a CLI that does not exist yet, and
inventing formats for it here would be designing six tasks ahead.

**The wire protocol**, beyond § 10's one property — C04's.

**The numbers** behind § 9's limits — C04's and C15's.

### Deferred decision points, each with a trigger

| Deferred | Trigger |
|---|---|
| **RBAC, and more than one authority level** | The first deployment that needs a principal who may read status and audit but not run commands. § 4's table is where it lands, as cell values |
| **Remote operator access** | Out of scope by RFC 0001. It returns only through an RFC that amends the reboot's surface, and it would need an authentication mechanism this ADR deliberately does not have |
| **Whether `AST-14` should move domains in the threat model** | § 1 names the owner, which is the half that changes behaviour. Moving the asset is P01's model to change, and the trigger is a task that needs the domains to be disjoint |
| **`allow_responses`** (`ADR-0002` § 6) | This API acquiring a genuine request/reply surface over NATS. § 11 says it has not |
| **An impersonation mechanism** | An authorization model that can bound it — that is, RBAC first |

## Validation

`make check`. The acceptance cases and their demonstrated failures are recorded
in [`docs/dossiers/P09-acceptance-evidence.md`](../dossiers/P09-acceptance-evidence.md).

## References

- [P09 dossier](../dossiers/P09.md)
- [PRODUCT-CHARTER.md](../project/PRODUCT-CHARTER.md) — § 5's journeys and § Exit codes
- [THREAT-MODEL.md](../project/THREAT-MODEL.md) — `RSK-8`, `THR-24`, `THR-35`, `THR-37`, `AST-14`, `TD-OP`, `TB-1`
- [ARCHITECTURE-INVARIANTS.md](../project/ARCHITECTURE-INVARIANTS.md) — `ARCH-COMM-001`, `ARCH-OBS-001`
- [ADR-0008 — Persistence and audit](0008-persistence-and-audit.md) — § 6's **Actor** field
- [RFC 0003 — A caller-selected execution user](../rfcs/0003-caller-selected-execution-user.md)
