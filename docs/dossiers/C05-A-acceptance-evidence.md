# C05-A acceptance evidence

This records the acceptance-contract half of C05. It implements no enrollment
behaviour. C05-I owns the production CLI operation, server and agent state
machines, runtime JWT minting, configuration, persistence and NATS connections,
and must replace each pending body without weakening the surface.

Contract package: `test/contract/enrollment`

Contract commit: `6115c70f35fb0aa03f6642a1de36471976650f05`

Contract amendments:

None.

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
  receives one explicit path for the enrollment credential, presence credential
  and account signing seed; it never receives a credential directory. The agent
  receives one identity-artifact path and one ledger path.
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
- one version-1, mode-`0600` identity artifact containing the permanent NATS
  credential and both service trust halves, so one rename exposes all or none;
- the exact server and agent configuration keys; and
- five agent holds and one server hold covering the amended crash matrix.

The broker address and TLS CA are deliberately absent from the bundle. They are
deployment configuration, while the bundle is the out-of-band trust path for
the service signing and result-encryption public halves.

## Dossier cases mapped

| Dossier obligation | Frozen case |
|---|---|
| issuance audit and exactly-once stdout | `ISS-1` |
| indistinguishable operator refusals and exit mapping | `CLI-1`–`CLI-3` |
| per-connection in-flight limit | `LIM-5` |
| exact, expiring bootstrap JWT | `JWT-1` |
| both service public halves in the bundle | `BND-1` |
| no bundle in argv | `ARG-1` |
| bundle mode, stdin and removal | `FILE-1`, `STDIN-1`, `FILE-2` |
| expired, spent and invalid coarse denials plus audit | `DENY-1` |
| bootstrap identity and request-signature binding | `AUTH-1` |
| different-key denial and same-key idempotency | `IDEM-1`, `IDEM-2` |
| server-assigned identifier | `ID-1` |
| no agent private key leaves the agent | `KEY-1` |
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

Exit `0`: all 36 C05 cases exited non-zero with their identifier and documented
reason. `make contract` skips exactly those cases under ordinary execution.

### Throwaway reference state machine

A throwaway Go state machine, not retained in the repository, exercised the
frozen state transitions and assertion accounting before the freeze. Its clean
run was:

```text
go run /tmp/c05-reference.go
reference: PASS
```

Exit `0`.

Each mutation was then run independently as:

```text
go run /tmp/c05-reference.go <case-id>
```

Each exited `1` and reported only its intended case. The mutations were:

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

This model demonstrates that the state requirements are mutually
discriminating and satisfiable. It does **not** substitute for the required
C05-I mutation run against the real processes, broker, filesystem and kernel.
C05-I must show each implemented body fail against the corresponding planted
production defect before removing its pending entry.

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
