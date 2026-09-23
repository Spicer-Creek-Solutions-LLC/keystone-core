# C04-A acceptance evidence

This records the acceptance-contract half of C04. It implements no operator
socket: C04-I owns the listener, the authorization decision, the denial record
and stale-socket recovery, and must remove each pending entry when its case
passes against the production `keystone-server`.

Contract package: `test/contract/operator`

Contract commit: `417a5325377592c0d8da8c8d1421d9835444e376`

Contract amendments:

- `07de877cbdd39e65ae09135616248820c6171487` — `C04`. C04-A's own review round
  on #376, before acceptance. The frozen `ORD-1` trusted a fault point named
  "after the decision" to mark the decision, and the implementation decides
  where it sits: a server that held, then read, then decided passed. `ORD-1`
  now freezes the stricter, platform-neutral rule that a refused connection is
  never read, measured by the kernel (§ Settled decisions, `D-C04-1`). The
  after-decision fault point was removed from the surface, and `ORD-1`'s test
  was renamed to match its requirement. The maintainer approved the change.

  An earlier revision of this file moved `Contract commit:` to this amendment
  instead of declaring it. Review found it: the freeze point does not move, and
  being before merge does not erase that `417a53253` was committed and locked
  as the freeze.

## Settled decisions

Settled at C04-A's approval, per [`C04.md`](C04.md) § 11.

- `D-C04-1` — **the dossier's proposal was replaced, and so was this
  contract's first answer.** The dossier proposed a parser that records when it
  was first entered. The first version of `ORD-1` instead held the server with
  a fault point "after its decision" and watched for request bytes consumed
  before the hold ended. Review of #376 showed both share one flaw: **the
  implementation decides where the marker goes.** A server that holds, then
  reads, then decides passes either one. A record of the server's own order is
  the same claim in a different place.

  `ORD-1` therefore asks the kernel instead. The contract freezes a
  platform-neutral rule: **a connection the in-band check refuses is never
  read, before or after the decision.** A refused peer sends one byte. On
  Linux, a unix stream socket closed with unread data marks its peer
  `ECONNRESET`, and one closed on an empty queue gives a clean end of stream.
  So a denial followed by a reset shows that byte was never consumed, whatever
  the server believes about its own order. No timing, no hold and no server
  marker is involved. The maintainer approved the change and its costs, set
  out under § Limits and § Portability.
- `D-C04-2` — no register row lands at C04, and none cites `ADR-0009`.
  `archlint` checks nothing for C04; the substrate is tracked by this contract
  alone, and C04-I's evidence states the gap again.
- `D-C04-3` — raised, not fixed. One input for C04-I: Go's pure `os/user`
  reads `/etc/group` directly and does not consult NSS, so a host whose groups
  come from a network directory behaves differently from this contract's
  container. C04-I states which lookup it uses and what it does when the lookup
  fails or hangs; the authorization timeout (`LIM-2`) bounds a hang.
- `D-C04-4` — no journey verb. The three CLI exit-code cases and the
  in-flight-request limit are registered deferred to C05.

## The frozen operator interface

The cases cannot be written without an interface, and `C04.md` § "What C04
decides" makes it C04's to decide. It is frozen in `contract_surface.go` beside
the cases, so a later change is an amendment and not an edit:

- **Invocation.** `keystone-server` with no arguments, reading the TOML file
  `KEYSTONE_SERVER_CONFIG` names.
- **Configuration.** `[operator] admin_group` (required, no default), the four
  observable § 9 limits, `[store] path`, and one `[faults]` hold, which makes
  the two pre-authorization limits observable and proves nothing about order.
- **Framing.** A uint32 big-endian length, then a JSON object. Requests carry
  `op`; every server error is an object whose only member is `error`, one of
  `authorization_denied`, `unknown_operation`, `frame_too_large` and
  `connection_limit`.
- **Audit.** A denial is a row in `audit` with action
  `operator.authorization.denied`, NULL `job_id` and `target`, `actor_uid` and
  `actor_username_snapshot`, and no column containing `pid`. An enabled fault
  point is a row whose action is `fault.enabled:` and the key.

**A consequence for C04-I.** `C04.md` § 3.3 lets `go.mod` change for
`golang.org/x/sys` "and nothing else", and no TOML library is a dependency. C04-I
either parses the subset this contract writes by hand or asks for a grant at
its own approval.

## The dossier's cases, mapped

Every row of `C04.md` § 5.2, and where it went. Two cases were added.

| `C04.md` § 5.2 row | Case |
|---|---|
| The socket's directory or file has the wrong owner, group or mode | `SOCK-1` |
| The server starts with no admin group configured | `SOCK-2` |
| A principal outside the group can `stat` the socket | `SOCK-3` |
| A principal outside the group completes a `connect()` | `SOCK-4` |
| A request byte is parsed before the connection is authorized | `ORD-1` |
| `SO_PEERCRED` is not read before any request byte is read | `ORD-1` |
| A framing byte must be read to decide authorization | `ORD-1` |
| A denial response is written before its record is durable | `ORD-2` |
| A denial is answered when its record could not be made durable | `ORD-3` |
| A stale-group session is not refused by the in-band check | `DENY-1` |
| `root` is authorized without § 4 saying so | `DENY-2` |
| The in-band denial says anything beyond *authorization denied* | `DENY-3` |
| An in-band denial produces no record | `DENY-4` |
| A kernel refusal produces a record | `DENY-5` |
| A denial record's actor is not the `SO_PEERCRED` uid, carries a pid, or does not mark the username | `DENY-6` |
| A request field changes the actor a record names | `DENY-7` |
| A non-socket at the path is removed | `REC-1` |
| A socket with an unexpected owner is removed | `REC-2` |
| A live socket is removed, or a second server starts | `REC-3` |
| A stale `root`-owned socket is not recovered | `REC-4` |
| More unauthorized connections are held than the limit | `LIM-1` |
| An authorization decision outlives its timeout | `LIM-2` |
| More authorized connections are held than the limit | `LIM-3` |
| A frame larger than the maximum is allocated before it is refused | `LIM-4` |
| An operator action reaches an agent without the broker | `ISO-1` |
| The two refusal paths are distinguishable to the CLI's caller | `CLI-1`, deferred to C05 |
| Either refusal path exits other than `10` | `CLI-2`, deferred to C05 |
| `ENOENT` exits other than `1` | `CLI-3`, deferred to C05 |
| A connection holds more requests in flight than the limit | `LIM-5`, deferred to C05 |
| **Added:** a stale socket in a directory a non-root principal can write | `REC-5` — `ADR-0009` § 8's first row makes the directory the precondition of a safe unlink |
| **Added:** an enabled fault point is not recorded | `FLT-1` — `ADR-0010` § 12, owed by the first task with a fault point |

**`ORD-1` carries three rows through a stricter rule.** § 3's rows and § 10 say
nothing may be read or parsed before the decision. `ORD-1` requires that a
refused connection is never read at all. That implies every one of them for a
refused peer, and it is the only form of them the kernel can report without
trusting the server.

**It is stricter than `ADR-0009`**, which permits a server to read and discard a
refused connection's bytes once the decision is made. C04 forbids that too,
because it is what makes the order observable. The throwaway server's
`drain-after-deny` defect below is the cost, stated: it is ADR-compliant and
fails `ORD-1`.

**The authorized path is covered by argument, not observation.** Reading before
the decision for a member is the same code path as reading before the decision
for anyone. The server does not yet know which peer it has, so it cannot read
early for members only. A server that chose to read early from cached knowledge
of a peer would have made a decision from stale state, which is `DENY-1`'s
subject.

**`ISO-1` is a weaker statement than § 11's.** § 11's test is that removing the
broker leaves no path to an agent. At C04 there is no broker and no agent, so the
case asserts what C04 can: the listening server holds no IP socket at all. C06
is where a broker exists to remove.

## Feature acceptance table

Authorization denial is applicable and is `SOCK-4` and `DENY-1`–`DENY-7`, at the
operator boundary. Audit is applicable for C04's own denials: `DENY-4`–`DENY-7`,
`ORD-2`, `ORD-3`. Intended target, non-target, server isolation, payload
protection, duplicate delivery, restart, cancellation and diagnostics are `N/A`:
C04 runs no journey, crosses no broker and starts no agent.

## Where this runs, and what it is worth

Each case starts the production `keystone-server`, built from source as the
e2e image builds it, inside a container with no network. The admin group and
the three principals — a member, a member revoked under a running session, and
an outsider — are real accounts in that container, and `root` is the fourth
principal.

**That is pull-request feedback, not acceptance.** `ADR-0010` § 1 gives
`ADR-0009` no Docker acceptance at all, and § 8 names the socket's mode, its
group and the stale-membership denial as VM cases. **Every case in
`contract_surface.go` is owed to C13's VM harness as its release gate**; none is
claimed as accepted by this contract passing.

## Demonstrated failures

### The registered cases

`make pending-contract` runs every registered test with
`KEYSTONE_PENDING_CONTRACT=1`; each exits non-zero with its case identifier and
C04-I reason. Ordinary `make contract` skips exactly those cases. All of them
fail for the same reason — the server does not listen — so this proves that no
obligation can be mistaken for a pass, and nothing about discrimination.

### The instrument

`sockprobe` is proven on the running kernel by its own tests, which run in every
`make check`. The kernel-semantic ones are in `unread_linux_test.go` and assert
each property `ConsumptionObservable` claims: an unread byte ends in a reset, a
consumed one in a clean end of stream, and the frame written before close is
delivered either way.

| Planted defect | Fails |
|---|---|
| A reset is reported as a clean end of stream | the three tests that expect a reset |
| The Linux build reports no instrument | `TestConsumptionIsObservableHere` only |

Two of those tests assert limits rather than capabilities, so that neither can
silently stop being true. A partial read of a longer payload still ends in a
reset, which is why `ORD-1` sends one byte. A `MSG_PEEK` ends in a reset too.

### The fixture

`TestFixtureInstrumentsTheHost` is not an acceptance case and is not pending. It
runs on every `make contract` and proves what does not need C04-I's server: the
image, the accounts, the instrument inside the container, and the premise every
stale-member case rests on — a session started before revocation still passes
the kernel. C03-A had to record its fixture as reviewed but not run; this one
runs.

| Planted defect | Fails |
|---|---|
| The outsider is made a member of the admin group | `the kernel refuses an outsider` only |
| The closing stand-in listener reads before it closes | `a peer that closes without reading is seen not to have read` only |
| Revocation does nothing | `a revoked member's running session still passes the kernel` only |

### The cases' discrimination, against a throwaway server

The pending cases cannot run against today's server, so their bodies were run
against a **throwaway reference server** written only for this measurement and
not committed. It exists to answer two questions the pending run cannot: whether
the contract can be satisfied at all, and whether each case fails on the defect
it is for. It says nothing about C04-I's server, and **C04-I still owes a
planted-defect demonstration per case against production code**, per `C04.md`
§ 5.3.

Unmodified, it passed every case. Its first run found two defects in this
contract's own bodies before they reached C04-I: an unanchored `pkill` pattern
that matched its own shell, and an inode comparison defeated by tmpfs reusing an
unlinked inode's number (see § Limits).

**The first version of this matrix missed the defect review found.** Its only
ordering defect read the request immediately, which the first `ORD-1` caught
by timing; nothing planted a server that held first, then read, then decided.
Both ordering defects below are now **faithful**: whatever they read before
deciding is replayed after authorization, so each is only an ordering defect and
the authorized path still works. An earlier draft of them swallowed the
authorized control's request, failed nearly every case, and so said nothing
about `ORD-1` in particular.

Each row below plants one defect and records every case that failed.

| Planted defect | Cases that failed |
|---|---|
| None | none |
| Parses the first request before deciding, then behaves correctly | `ORD-1`, `LIM-4` |
| **Holds, then reads what the peer sent, then decides** — the defect review found | `ORD-1`, `LIM-2` |
| Decides first, then reads and discards a refused connection before closing | `ORD-1` — ADR-compliant; the stricter rule's cost |
| Writes the denial, then the record 20 ms later | `ORD-2`, `ORD-3` |
| Answers the denial when the record's write failed | `ORD-3` |
| Never writes a denial record | `ORD-2`, `ORD-3`, `DENY-4`, `DENY-6`, `DENY-7` |
| Authorizes every peer the kernel admitted | `ORD-1`, `ORD-2`, `ORD-3`, `DENY-1`–`DENY-4`, `DENY-6`, `DENY-7` |
| Authorizes `root` | `ORD-1`, `ORD-3`, `DENY-2`, `DENY-3`, `DENY-4` |
| Adds a `reason` member to the denial | `ORD-1`, `ORD-2`, `ORD-3`, `DENY-1`–`DENY-4`, `DENY-6`, `DENY-7` |
| Adds a `peer_pid` column | `DENY-6` |
| Takes the recorded actor from a request field | `ORD-1`, `DENY-7` |
| Unlinks whatever is at the socket path | `REC-1`, `REC-2`, `REC-3` |
| Trusts a directory the admin group can write | `REC-5` |
| Never recovers a stale socket | `REC-4`, `REC-5` |
| No limit on connections awaiting authorization | `LIM-1` |
| No authorization timeout | `LIM-2` |
| No limit on authorized connections | `LIM-3` |
| Allocates the length prefix before checking it | `LIM-4` |
| Opens a TCP listener | `ISO-1` |
| Does not audit an enabled fault point | `FLT-1` |
| Socket mode `0666` | `SOCK-1` |
| Directory mode `0755` | `SOCK-1`, `SOCK-3` |
| Defaults the admin group when none is configured | `SOCK-2` |

**Where one defect fires many cases, the breadth is shared machinery, not a
blunt case.**

- **A leaked reason.** Every case that observes a denial checks it through one
  helper that requires the exact frame, so a leaked `reason` fails all of them.
  `DENY-3` is the case that owns the property.
- **No in-band check.** Authorizing every peer the kernel admitted removes the
  denial each of those cases starts from.
- **Parsing before deciding.** The defect fires `LIM-4` as well as `ORD-1`
  legitimately: parsing the first request before the decision includes
  allocating its body, and `LIM-4`'s request is a hostile length prefix. Against
  `ORD-1`'s one byte it blocks for the rest of a frame, so the refused peer gets
  no denial at all.
- **Holding, reading, deciding.** `ORD-1` fails because the refused
  connection ends in a clean end of stream after its denial — the byte was
  consumed. `LIM-2` fails too, because the defect's hold sits inside the
  authorization window that case times.
- **Reading a refused peer's request.** The defect that takes the recorded actor
  from the request fires `ORD-1` as well as `DENY-7`, because reading that
  request at all is what `ORD-1` forbids.
- **`ORD-3` fires on several.** Its last step expects one record and one answer
  after the disk is freed.

**No defect fires `SOCK-4` alone, and none can while `SOCK-1` holds.** The
directory's `0750` refuses an outsider before the socket's mode is consulted, so
a socket loosened to `0666` is still unreachable. `SOCK-4` asserts the outcome
the operator sees; `SOCK-1` asserts each mode.

## Limits

What these cases cannot detect, and what would.

- **A peek.** A server that inspects a refused peer's bytes with `MSG_PEEK`
  consumes nothing, and `ORD-1` cannot see it. Review of C04-I's source is what
  finds it.
- **The authorized path.** `ORD-1` observes refused peers only; the argument
  for members is in § "The dossier's cases, mapped".
- **Timing.** `LIM-1` and `LIM-2` rely on a pre-authorization hold of seconds
  against margins of hundreds of milliseconds, in both directions: a served
  connection must answer after the hold, a shed one must close well before it.
  A runner stalled for longer than a margin can fail a correct server. What
  these cases catch is what the defect matrix shows, and no more general claim
  is made. `ORD-1` has no timing dependence.
- **`ORD-2` is probabilistic.** A server that answers before its record is
  durable loses the record only if the kill lands in the gap. The kill is
  microseconds behind the response and the case repeats five times.
- **Durability against power loss.** A SIGKILLed process's writes survive in the
  host's page cache, so `ORD-2` proves the record was written before the
  response and not that it was synced. A VM that can be reset is where that is
  tested.
- **`ORD-3` depends on the journal.** It fills the store's filesystem, so the
  record's write fails for a real reason. SQLite's rollback journal needs new
  space for every write; a write-ahead log whose file had already grown could
  reuse it, and the case would then observe a successful write.
- **Group resolution is `/etc/group`.** The container has no NSS source beyond
  files. How C04-I behaves against a network directory is `D-C04-3`'s, and a VM
  joined to one is where it can be measured.
- **`LIM-4` reads peak virtual size.** Go backs a large slice lazily, so
  allocating a hostile prefix survives any memory cap; its address-space
  reservation does not. An allocator that reserved nothing until written would
  pass, and would also not have allocated.
- **Inodes are reused.** tmpfs reissued an unlinked socket's inode number when
  `REC-4` was first run against the throwaway server, so no case uses an inode
  to prove a replacement — only that an unchanged object was not touched.

## Portability

Generation 2 is Linux-only, and the maintainer's direction is that later
Windows, macOS and BSD support must stay possible. The contract separates what
is required from how it is measured, so that a port replaces the measurement and
not the requirement.

- **`ORD-1`'s requirement is platform-neutral.** Its rule — a refused
  connection is never read — is a statement about the server and names no
  kernel interface. It is the one requirement this contract chose the form of.
- **Other requirements name what `ADR-0009` names.** `SOCK-4`, `DENY-5`,
  `DENY-6` and `REC-4` mention `connect()`, `EACCES`, `SO_PEERCRED` or
  `ECONNREFUSED` because the ADR decides in those terms. A port amends them
  when it amends the ADR, not before.
- **The measurement is behind one seam.** `sockprobe.ConsumptionObservable` is
  defined per platform: `unread_linux.go` returns true and carries the kernel
  argument, and `unread_other.go` returns false on every other platform. A false
  result **fails** `ORD-1` as unmeasurable rather than passing it. A port adds
  its own file and kernel test, or records that the platform cannot observe
  consumption and decides what replaces it.
- **The larger coupling is `ADR-0009`'s, not this contract's.** `SO_PEERCRED`
  is Linux's; the BSDs and macOS have `getpeereid` and `LOCAL_PEERCRED`, and
  Windows has neither on unix sockets and would likely need named pipes and a
  different identity model. `/run/keystone`, POSIX groups and `root` do not map
  to Windows at all. **For C04-I, and for its reviewer:** peer-credential
  retrieval and group-membership lookup belong behind a small interface with
  Linux as its only implementation, so that a port adds a file rather than
  rewriting the listener. Requiring that is C04-I's approval to make; it is
  recorded here so it is not rediscovered.

## Validation

```text
make pending-contract
make contract
make contract-immutability-check
make check
```
