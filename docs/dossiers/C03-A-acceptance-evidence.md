# C03-A acceptance evidence

This records the acceptance-contract half of C03. It implements no NATS
authorization generator: C03-I owns the production JWTs and broker
configuration and must remove each pending entry when its corresponding case
passes against those production outputs.

Contract package: `test/contract/authorization`

Contract commit: `f7bee60a2857c78beb81b2c6ea70a1b81d3cd575`

Contract amendments:

None.

## Settled decisions

- `D-C03-1`: C03-I adds `github.com/nats-io/jwt/v2`,
  `github.com/nats-io/nkeys`, and the required `golang.org/x/crypto v0.56.0`.
  C03-A adds none of them.
- `D-C03-2`: the frozen surface and broker fixture live under
  `test/contract/authorization/`; the four C03 evidence paths in the
  traceability register move there.
- `D-C03-3`: `POS-1`–`POS-23`, `NEG-1`–`NEG-15`, `GEN-1`–`GEN-7`, and the
  permission matrix are frozen. Generated JWTs are never committed and never
  used to derive their own expectations.
- `D-C03-4`: C03 renders account-scoped limits and the stream and consumer
  limit values; C06 applies the latter.
- `D-C03-5`: authorization is measured against exact image
  `nats:2.15.0-alpine`, the stable NATS Server v2.15.0 release published on
  2026-09-17.

No separate `NATS-AUTHORIZATION.md` is owed. The operator-readable permission
set would duplicate the accepted ADR while providing no stronger protection:
the structured matrix in `contract_surface.go` is the independent source C03-I
must compare generated JWTs against, and the existing immutability guard freezes
it with the case meanings. C03-I therefore cannot retune the generator and its
expected permissions together.

The freeze-time review reconciled every enumerated grant in `ADR-0004` §§ 4 and
6 with the matrix: it contains twenty-two distinct grants, with none missing or
extra. The ADR's summary arithmetic says those sections contain fifteen and
eleven grants, but their enumerated lists contain thirteen and ten; the command
publisher's inbox grant appears in both sections and is represented once in the
matrix.

## Contract cases

The surface carries forty-five cases: all thirty-eight broker outcomes from
`ADR-0004` §§ 8–9 and seven generator properties from the C03 dossier. Each has
one named Go test and one `pending-requirements.json` entry owned by and expiring
at C03-I. No entry is a deferred gate.

The broker fixture accepts only a directory produced by the production
generator, requires `nats-server.conf`, mounts the directory read-only, and
starts `nats:2.15.0-alpine`. It contains no JWT and no permissive fallback.
No case can execute the fixture until C03-I supplies that generated directory,
so C03-A has reviewed but not run its port parsing, readiness loop, or cleanup.
The fixture is not part of the frozen surface and C03-I may correct it without a
contract amendment.

The frozen requirements preserve the three distinctions most likely to produce
false evidence:

- JetStream API and advisory cases must distinguish a permissions violation
  from an absent stream, consumer, or advisory source.
- `NEG-7` fails while establishing the revoked identity's connection; a
  post-connect publish denial does not satisfy it.
- `GEN-3` and `GEN-4` compare generated JWTs in both directions with the frozen
  matrix, never with a list derived from generator code.

## Feature acceptance table

Authorization denial is applicable and represented by `NEG-1`–`NEG-15`; it
becomes evidence only when C03-I exercises generated credentials against the
broker. Intended target, non-target, server isolation, payload protection,
duplicate delivery, restart, cancellation, audit, and diagnostics are `N/A`:
C03 has no command journey, agent execution, durable lifecycle, or audit output.

## Demonstrated failures

`make pending-contract` runs every registered test separately with
`KEYSTONE_PENDING_CONTRACT=1`. Each exits non-zero with its case identifier and
C03-I reason; the command exits zero only after confirming all forty-five
expected failures. Ordinary `make contract` skips exactly those registered
cases and passes.

This demonstrates the planted missing-implementation defect: none of C03-I's
forty-five obligations can be mistaken for a passing contract. It does not yet
demonstrate broker behavior. Before C03-I removes an entry, that task must show
the implemented case failing against a defect planted in the specific input it
claims to inspect. In particular, its JetStream mutations must preserve an
existing target, and its `NEG-7` mutation must preserve a successful unrevoked
connection control.

## Limits

The frozen matrix proves what C03-I must compare. It cannot prove that
`ADR-0004` chose the right permissions, that a future NATS version enforces them
identically, or that an unimplemented pending body would detect a behavioral
mutation. Its transcription is guarded by independent review at freeze time and
by immutability thereafter; any transcription error remains part of the
contract until an approved amendment removes it. The exact broker pin,
two-direction matrix cases, per-case mutation requirement, and independent
security review hold those boundaries explicitly.

## Validation

```text
make pending-contract
make contract
make contract-immutability-check
make archlint
make gates-agree
make check
```
