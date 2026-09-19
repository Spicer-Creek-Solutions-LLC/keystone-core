# C01-A acceptance evidence

This records the acceptance-contract half of C01. It does not implement the
protocol; C01-I owns the production behavior and must remove each pending entry
when its corresponding case passes.

Contract commit: `76a29ddcbb0be4d15bbc5397cee302d701c5d279`

## Contract cases

The immutable contract cases are `AC-1` through `AC-13` in
[`test/contract/protocol/contract_test.go`](../../test/contract/protocol/contract_test.go).
The hand-written framing vectors are in
[`framing-vectors.json`](../../test/contract/protocol/framing-vectors.json),
derived from ADR-0005 § 1 with the signature bytes zeroed.

All thirteen cases are pending under `C01-I`. `make pending-contract` runs each
case separately with the pending mode enabled and captures a non-zero exit code;
normal contract execution verifies the manifest before skipping only registered
pending cases. The manifest remains mutable so C01-I can remove entries in the
same pull request that makes their production assertions pass; the accepted
case source and framing tests remain immutable.

The nightly timed fuzz-search deferral is also registered explicitly in the
manifest. It is a gate deferral, not a protocol behavior case, but recording it
here keeps the owed work visible until C01-I or a later approved change removes
the entry.

## Feature acceptance table

All ten `TESTING.md` feature cases are `N/A`: C01-A crosses no NATS transport,
starts no agent, writes no durable state, and produces no audit record. Payload
protection is exercised at the protocol boundary by C01's own cases, but the
broker-observation case remains C08's responsibility.

## Demonstrated failures

| Case | Demonstration | Result |
|---|---|---|
| `AC-1`–`AC-13` | `make pending-contract` | Each registered case fails with its documented C01-I reason |
| Nightly fuzz search | Manifest validation and deferred-gate report from `make pending-contract` | Explicitly pending; no nightly gate is silently absent |

## Limits

The pending contract proves that missing behavior cannot be mistaken for a
passing acceptance suite. It does not prove protocol behavior until C01-I
removes the entries and supplies the implementation. The framing vectors check
field order, uint32 big-endian prefixes, cumulative envelope boundary arithmetic,
length disagreement, and a zeroed signature field; they do not choose the still-
structural value encodings for version, timestamp, or nonce.

## Validation

```text
make pending-contract
make contract
make doclint
make archlint
make gates-agree
```
