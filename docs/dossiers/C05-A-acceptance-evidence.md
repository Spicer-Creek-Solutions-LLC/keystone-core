# C05-A acceptance evidence

This records the acceptance-contract half of C05. It implements no enrollment
behaviour. C05-I owns the production CLI operation, server and agent state
machines, runtime JWT minting, configuration, persistence and NATS connections,
and must replace each pending body without weakening the surface.

Contract package: `test/contract/enrollment`

Contract commit: `6115c70f35fb0aa03f6642a1de36471976650f05`

Contract amendments:

- `a8405b8d639701217ef4fe0a5577286493af2f04` — `C05`. Review of pull
  request #381 found two blocking omissions in the frozen surface. First, it
  had nowhere to persist the agent's NKey seed, AST-4 signing key or AST-5
  decryption key before S1, so the first three crash cases and permanent
  reconnect could not preserve the public halves S1 bound to the token.
  `agentKeyArtifactV1` now freezes their local representation, atomic mode-0600
  write before S1, and digest binding to the later identity artifact; `KEY-2`
  freezes restart with the same keys. G50 supplied the protocol serialization
  primitive this amendment uses.

  Second, fault-key names were not the crash harness C05 § 3.1 requires and
  let C05-I choose how its own ordering was observed. `crash-harness.json` now
  freezes all six kill boundaries, durable agent and server boundary records,
  SIGKILL, recovery observations, and independent broker/store observations
  for `ORD-1`, `OBS-1` and `S6-1`. Boundary records establish only where to
  kill; they are not accepted as proof of order or recovery. The maintainer
  approved both review fixes. The original contract commit above does not
  move.

- `cf28ca0acfab28ec2a59fac9b2699f2a515450d5` — `C05`. Review round two found
  that `ORD-1` and `S6-1` raced a production consumer against the harness's
  independent copy of the same fanned-out message. A correct server could
  commit after receiving presence but before the harness received its copy; a
  correct agent could exit after receiving S6 but before the harness received
  its copy. The harness now uses the already-frozen pre-proof agent hold and
  post-commit server hold as deterministic negative windows, then requires the
  independent authenticated observation after each hold expires. `OBS-1` is
  unchanged. The original freeze and first amendment do not move.

- `ae80fb12e7d86d40ff1318f431449393e5b7c729` — `C05`. Issue #384 identified
  two interfaces C05-I could not supply without choosing its own acceptance
  surface. First, the server had no configured source for `AST-7`'s signing
  private key or `AST-8`'s result-encryption public half. The amendment freezes
  `[service].signing_private_key_file` and
  `[service].result_encryption_public_key_file`, and tightens `BND-1`: both
  halves in the bundle must equal the fixture-provisioned keys, not merely
  form a self-consistent pair. `SKEY-1` separately requires the server to
  refuse startup when the signing private-key file has a group or other
  permission bit; owner-only modes such as `0400` and `0600` are permitted.
  The fixture provisions the keys; G51 owns production provisioning.

  Second, `LIM-5` named a configured limit that no configuration surface
  defined. The limit is now exactly one request per operator connection: the
  next frame is not read until the current reply is written, and replies stay
  in request order. There is no configuration key or new error. The inert
  `[faults].operator_hold_before_response_ms` keeps the first request in flight
  so the contract can prove that a pipelined second request has not started.
  `FLT-2` requires its `fault.enabled:` audit record before the server accepts
  a connection and requires no such record when the hold is disabled. A
  configurable limit waits for request identifiers; C15 revisits defaults.

## Settled decisions

The maintainer approved these with the C05-A plan.

- `D-C05-1` remains settled by RFC 0005: the server refuses a spent token and
  the broker ends bootstrap access at JWT expiry. Nothing revokes at runtime,
  and nothing treats a claims read-back as proof.
- `D-C05-2`: the server assigns an agent identifier at token creation from 128
  random bits, encoded as 32 lowercase hexadecimal characters and checked by
  `protocol.ValidIdentifier`. The operator's agent name remains a label.
- `D-C05-3`: the agent's first correctly signed presence envelope is S3's proof.
  It keeps the bootstrap connection through S6 and receives confirmation on the
  token reply subject. Expiry between S4 and S6 produces exit `10`, never `0`,
  while leaving the working permanent identity intact.
- `D-C05-4`: broker address, CA and verified name are configuration. The server
  receives one explicit path for the enrollment credential, presence credential,
  account signing seed, service signing private key and result-encryption public
  key; it never receives a credential or key directory. The service signing
  private-key file has no group or other permission bit; both `0400` and `0600`
  are permitted. The result-encryption half is public. The agent receives
  explicit paths for its pre-S1 private-key artifact, later identity artifact,
  durable fault journal and ledger.
- `D-C05-5`: the contract and both C05 traceability paths live here. Crash
  boundaries use audited holds configured in the shipped binaries; the harness
  observes the boundary record before killing the process.
- `D-C05-6`: C05 records and proves the identity `active`; C10 owns listing it.
- `D-C05-7`: C05-I may remove only `enroll` from the no-journey-verb boundary.
- `D-C05-8` remains implemented by G49. Every C05 connection must use TLS-first
  TLS 1.3 and `natsauth.ClientTLSConfig` with the configured CA and name.
- `D-C05-9` remains settled by RFC 0005: the bootstrap credential is the secret.
  The token is a 256-bit random identifier encoded as 64 lowercase hexadecimal
  characters. Its default lifetime is 15 minutes; an optional creation TTL is
  bounded to 1 through 59 minutes.

## Frozen interface

`contract_surface.go` freezes the complete interface C05 owns:

- operator operation `enroll.create`, carrying `agent_name` and optional
  `ttl_seconds`;
- a version-1 JSON bundle carrying the token and assigned agent identifiers,
  bootstrap NATS credentials and both service public halves;
- version-1 request and reply payloads inside ADR-0005 envelopes;
- one version-1, mode-`0600` agent-key artifact containing the NKey seed and
  AST-4 and AST-5 private halves, atomically renamed and fsynced before S1;
- one later version-1, mode-`0600` identity artifact containing the permanent
  NATS credential, both service trust halves and the SHA-256 digest of the
  exact key-artifact bytes, so a restart cannot pair different credentials and
  keys;
- the exact server and agent configuration keys, including the two service-key
  paths and the signing private-key file's owner-only permission rule;
- one operator hold, five agent holds, one server hold, and the version-1
  crash-harness surface that fixes the enrollment holds' durable arrival
  records and independent observations; and
- exactly one request at a time on each operator connection, with the next
  frame unread until the current reply is written and replies in request order.

The broker address and TLS CA are deliberately absent from the bundle. They are
deployment configuration, while the bundle is the out-of-band trust path for
the service signing and result-encryption public halves.

## Dossier cases mapped

| Dossier obligation | Frozen case |
|---|---|
| issuance audit and exactly-once stdout | `ISS-1` |
| indistinguishable operator refusals and exit mapping | `CLI-1`–`CLI-3` |
| one request at a time per operator connection | `LIM-5` |
| operator response hold enablement is audited before serving | `FLT-2` |
| exact, expiring bootstrap JWT | `JWT-1` |
| fixture-provisioned service public halves in the bundle | `BND-1` |
| owner-only service signing private-key file | `SKEY-1` |
| no bundle in argv | `ARG-1` |
| bundle mode, stdin and removal | `FILE-1`, `STDIN-1`, `FILE-2` |
| expired, spent and invalid coarse denials plus audit | `DENY-1` |
| bootstrap identity and request-signature binding | `AUTH-1` |
| different-key denial and same-key idempotency | `IDEM-1`, `IDEM-2` |
| server-assigned identifier | `ID-1` |
| no agent private key leaves the agent | `KEY-1` |
| private keys persist atomically before S1 and survive restart | `KEY-2` |
| atomic mode-0600 identity artifact | `CRED-1` |
| ledger before activation | `LEDGER-1` |
| permanent proof before the active-and-spent write | `ORD-1` |
| spent-token refusal with the sole S6 exception | `SPENT-1` |
| bootstrap proven usable before broker-enforced expiry | `EXP-1` |
| refusal and expiry observed where enforced, never read back | `OBS-1` |
| authenticated S6 required for exit 0 | `S6-1` |
| agent kills at S0-S1, S1-S2, S2-S3, S3-S4 and S4-S6 | `CRASH-1`–`CRASH-5` |
| server kill after S4's write and before S6 | `CRASH-6` |
| permanent reconnect and missing-identity behavior | `RECON-1`, `RECON-2` |
| second agent leaves the first unchanged | `NON-1` |
| verified TLS on every connection | `TLS-1` |
| exact credential slots and wrong-role refusal | `ROLE-1` |
| isolated topology and broker-only journey | `ISO-1` |

Each crash boundary is a separate case. A harness cannot satisfy one boundary
by reaching another, and each case requires the fault-point audit record before
it evaluates recovery.

## Frozen crash harness

`crash-harness.json` is a frozen input to C05-I's process driver, not a set of
implementation log messages. It fixes `SIGKILL`, one run per boundary, the
arrival precondition, and the recovery observation for every `CRASH-*` case.
The agent appends version-1 JSON lines to configured `faults.record_path` and
fsyncs each `enabled` and `reached` record. The server uses durable audit rows
with `fault.enabled:` and `fault.reached:` action prefixes. The harness waits
for the matching `reached` record before killing the named production process.

Those records answer only *where the kill landed*. They cannot establish the
orders whose correctness is under test:

- `ORD-1` enables the after-identity-write hold and establishes its broker
  subscription before the agent starts. While the agent is durably recorded at
  that pre-proof hold, neither presence nor active-and-spent may be observable.
  After the hold expires, the harness requires an AST-4-verified presence and
  then active-and-spent from a separate read-only store connection.
- `OBS-1` proves the bootstrap credential usable before expiry. While it is
  still live but spent, a fresh correctly signed request must receive the
  application denial and create its denial record, while broker protocol trace
  contains no `$SYS.REQ.CLAIMS.*` publish. After expiry, a new connection with
  the same credential must fail with the broker's authorization violation. The
  broker runs with protocol tracing enabled by a harness-side command-line flag
  such as `-DV`; this is not a generated-configuration change.
- `S6-1` enables the server's after-activation-commit hold and subscribes to the
  token reply subject before enrollment. While the server is durably recorded
  at that hold, the harness must receive no S6 and the agent must remain
  running. After the hold expires, the harness requires an authenticated S6
  envelope and reads active-and-spent directly from the server store.

`crash_harness_test.go` rejects a missing or duplicate boundary, a mismatched
fault key, an ordering case without its observation, or an unspecified record
sink. C05-I supplies only the production-process driver and assertions that
consume this surface.

## Feature acceptance table

| TESTING.md case | C05 evidence |
|---|---|
| Intended target | `IDEM-2`, `ID-1`, `ORD-1`, `SPENT-1` |
| Non-target | `NON-1` |
| Server isolation | `KEY-1`, `ISO-1` |
| Authorization denial | `CLI-1`, `CLI-2`, `DENY-1`, `AUTH-1`, `ROLE-1` |
| Payload protection | `KEY-1`; enrollment is signed and deliberately not encrypted |
| Duplicate delivery | `IDEM-1`, `IDEM-2` |
| Restart | `CRASH-1`–`CRASH-6`, `RECON-1`, `RECON-2` |
| Cancellation/timeout | `N/A`: enrollment creates no job; token expiry is covered by `DENY-1`, `EXP-1` and `S6-1` |
| Audit | `ISS-1`, `DENY-1`, `SPENT-1`, plus the crash-boundary audit precondition |
| Diagnostics | `N/A`: C10/C11 own listing and presentation; C05 freezes only the charter's enrollment exit codes |

## Demonstrated failures

### Missing production behaviour

The retained pending runner executed every registered case separately:

```text
GOCACHE=$PWD/.c05-go-cache GOTMPDIR=$PWD/.c05-go-tmp make pending-contract
```

Exit `0`: all 37 C05 cases exited non-zero with their identifier and documented
reason. `make contract` skips exactly those cases under ordinary execution.

### Throwaway requirement model — not contract discrimination

A throwaway Go state machine, not retained in the repository, exercised the
frozen state transitions and assertion accounting before the freeze. Its clean
run was:

```text
go run /tmp/c05-reference.go
reference: PASS
```

Exit `0`.

Each modeled mutation was then run independently as:

```text
go run /tmp/c05-reference.go <case-id>
```

Each exited `1` and reported only its intended model assertion. The mutations
were:

| Mutation | Failed case |
|---|---|
| emit twice or omit the durable issuance record | `ISS-1` |
| expose which operator authorization layer refused | `CLI-1` |
| map authorization refusal away from 10 | `CLI-2` |
| map a missing socket away from 1 | `CLI-3` |
| admit one request beyond the in-flight limit | `LIM-5` |
| widen or outlive the bootstrap JWT | `JWT-1` |
| omit either service public half | `BND-1` |
| place the bundle in argv | `ARG-1` |
| accept a group-readable bundle | `FILE-1` |
| reject standard input | `STDIN-1` |
| retain the bundle after exit 0 | `FILE-2` |
| distinguish or omit audit for expired, spent or invalid | `DENY-1` |
| accept the wrong bootstrap identity or signing key | `AUTH-1` |
| treat changed public halves as a retry | `IDEM-1` |
| mint again for identical public halves | `IDEM-2` |
| derive the identifier from the operator label | `ID-1` |
| copy an agent private key across the boundary | `KEY-1` |
| expose a partial or permissive identity artifact | `CRED-1` |
| activate before ledger creation | `LEDGER-1` |
| commit active/spent before the broker observes permanent proof | `ORD-1` |
| answer a spent request other than S6, or omit its denial record | `SPENT-1` |
| test only an identity that never connected before expiry | `EXP-1` |
| trust a claims read-back—the exact RFC 0005 defect | `OBS-1` |
| confirm S6 before S4's durable write, or exit 0 without it | `S6-1` |
| omit each named agent recovery edge in turn | `CRASH-1`–`CRASH-5` |
| repeat rather than resume after the server S4-S6 kill | `CRASH-6` |
| re-enroll an already enrolled agent | `RECON-1` |
| start without a readable identity artifact | `RECON-2` |
| alter agent 1 while enrolling agent 2 | `NON-1` |
| disable TLS-first or verification on one connection | `TLS-1` |
| accept a credential in the wrong server slot | `ROLE-1` |
| add a direct server-agent route | `ISO-1` |

This demonstrates only that the modeled requirement set is satisfiable and
that the model's own assertions notice its own mutations. **It is not evidence
that any frozen pending test discriminates: none of those bodies has run, and
the model was neither retained nor called by this package.** C05-I must show
each implemented body fail against the corresponding planted production defect
in the real processes, broker, filesystem and kernel before removing its
pending entry.

### Issue #384 amendment demonstration

A second throwaway model, also not retained in the repository, exercised only
the issue #384 requirements and their review amendments. Its clean run was:

```text
go run /tmp/c05a-384-reference.go
reference: PASS
```

Each amendment mutation was then run independently. Each exited `1` and named
only the intended case:

| Mutation | Failed case |
|---|---|
| substitute a self-consistent signing public half for the fixture-provisioned half | `BND-1` |
| substitute a self-consistent result-encryption public half for the fixture-provisioned half | `BND-1` |
| make the service signing private-key file group-readable | `SKEY-1` |
| refuse an owner-read-only service signing private-key file | `SKEY-1` |
| read the pipelined second frame before writing the first reply | `LIM-5` |
| write the second reply before the first | `LIM-5` |
| enable the operator response hold without its pre-serve audit record | `FLT-2` |
| record the operator response hold when it is disabled | `FLT-2` |

The model demonstrates that the tightened requirements are satisfiable and
distinguish the omissions issue #384 identified. It does not replace C05-I's
required production-process mutation evidence.

At the amendment head, the enrollment package test, `make pending-contract`
and `make contract-immutability-check` passed; the pending runner reported the
39 absent C05-I behaviors and the immutability gate named
`ae80fb12e7d86d40ff1318f431449393e5b7c729` as the third declared amendment.
A clean-worktree `make check` then passed formatting, vet, race tests, nested
tool tests, whitespace, build, vulnerability scanning, Markdown lint, link
checking, the capability catalog, doclint and approved-document checking. It
stopped when the authorization contract tried to execute `docker`, which this
host does not provide. The branch-push runner must supply the authoritative
container result.

### Review-amendment checks

The amendment's retained self-test and pending registration passed:

```text
go test -tags contract ./test/contract/enrollment
make pending-contract
make contract-immutability-check
```

Exit `0`; `pending-contract` reported all 37 C05 cases failing for their
documented absent production behavior, and the immutability gate retained
`6115c70f35fb0aa03f6642a1de36471976650f05` as the freeze with the declared
amendments.

The harness surface was then changed temporarily so `CRASH-6` named
`enrollment_server_hold_after_activation_commit_ms_defect` instead of its
frozen server fault. This command:

```text
go test -tags contract ./test/contract/enrollment \
  -run TestCrashHarnessSurfaceIsComplete
```

exited `1` and reported `bad crash boundary` for `CRASH-6`. The defect was
removed before commit. This proves the retained harness self-test detects a
boundary-key drift; it does not claim that any pending production case has run.

## Limits and VM debt

- Docker provides pull-request feedback for the complete transport journey.
  Mode `0600`, atomic rename, `fsync`, account ownership and power-loss
  durability remain C13 VM evidence, exactly as the dossier requires.
- A container shares the broker host's clock. `EXP-1` can prove broker behavior
  before and after expiry, not tolerance of independent clock skew.
- The bundle is trusted because it arrived out of band. This contract cannot
  prove its provenance (`ADR-0003` § 14).
- A process kill demonstrates recovery from committed state, not persistence
  through power loss.
- `KEY-1` observes server storage, logs and broker bytes. Independent source
  review remains necessary to find a private key copied into a transient buffer
  and erased before any observation.
- The harness self-test enforces the case set, fault-key mapping and presence of
  each prose mechanism; it does not interpret the prose. Review and the
  immutability gate guard the mechanism's meaning, as they do for C03-A's
  matrix transcription.

## Validation at freeze

Passed:

```text
go test -tags contract ./test/contract/enrollment
make pending-contract
git diff --check
```

From a clean detached worktree, the following also passed: build,
`govulncheck`, Markdown lint, link checking, capability-catalog check, doclint,
approved-document checking, contract immutability, archlint, DCO exemption
checking and gate agreement.

The host has no `docker` executable. Consequently `make contract` reaches the
existing authorization package and fails when it tries to start its broker;
`make container-suite` and therefore a full `make check` cannot run here. This
is an environment limitation, not reported as green. A clean-worktree
`make check` also exposed an existing host-sensitive race-suite failure in
`test/contract/operator/sockprobe`: `TestAPartialReadOfALongerPayloadStillEndsInReset`
returned `EINVAL` in the full `./...` run, while the same package passed when
run independently. C05-A does not touch that package. The branch-push CI runner
must supply the authoritative full result.
