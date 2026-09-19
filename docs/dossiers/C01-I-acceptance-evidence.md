# C01-I acceptance evidence

This records the implementation half of C01. `C01-A` froze the contract; this
made all thirteen of its cases live, in four stages — framing, signatures,
encryption, then the vector binaries and goldens.

## What the contract asserts now

`pending-requirements.json` holds one entry: `nightly-fuzz-search`, which
`D-C01-4` leaves owed on purpose — a fixed `-fuzztime` is a budget, not a
property. Every `AC-` case runs against production code.

| Case | What makes it true |
|---|---|
| `AC-1` | A signed-only and an encrypted class, each through seal, sign, encode, decode, verify, open |
| `AC-2` | `protocol.Frame` reproduces both pinned vectors byte for byte |
| `AC-3` | Every byte of fields 1–8 flipped **through the wire**, decoded, verified |
| `AC-4` | Wrong key, and a spliced hybrid signature |
| `AC-5` | All seven classes sign and verify; nothing outside the set has a signer |
| `AC-6` | Wrong recipient and four truncation depths |
| `AC-7` | Unknown version, and four wrong field sequences |
| `AC-8` | Unknown header tolerated; disagreeing headers refused; bytes inside a field's declared length tolerated, bytes after field 9 refused |
| `AC-9` | Two binaries, separate OS processes, over stdio — genuine vector accepted, tampered vector refused |
| `AC-10` | Eight probes; every refusal inside § 9's closed set and free of input-derived detail |
| `AC-11` | `D-C01-5`'s three pairs, each identical |
| `AC-12` | Nine goldens over framing and signed input, covering all seven classes and both identifier extremes |
| `AC-13` | Twenty seed-corpus entries, each safe for the production parser |

## Limits, stated rather than left to be found

**Goldens cannot cover whole envelopes.** ML-DSA signing is randomized and the
KEM draws fresh randomness, so field 8 and field 9 differ on every production.
Only what the encoder *determines* can be frozen, which is the framing and the
signed input. `AC-12` is about that and cannot be about more.

**A self-consistent weaker construction passes every round-trip case.** Removing
the ephemeral key and the KEM ciphertext from the HKDF input — the difference
between a bound hybrid and the known-weak one — left `AC-1` green, because both
sides derive the same weaker key. `D-C01-1` warned about this shape for the
framing, where hand-written vectors answer it; there is no equivalent here,
because `Seal`'s output cannot be frozen. `TestDeriveBindsTheTranscript` asserts
the property directly instead. **No acceptance case reaches it.**

**`ARCH-COMM-003` is half paid.** `D-C01-3` gives C01 serialization across
processes; production NATS subjects are C08's, so the invariant lands at C05.

**The contract exercises the production API, not released binaries**, except at
`AC-9`. `ARCH-COMM-003`'s remaining clauses are why that matters and why the row
still points at C05.

## What review found

**A cleartext correlation identifier on encrypted classes.** `SealEnvelope`
sealed only the payload and left field 4, so an encrypted command leaked the
identifier that groups an operator's actions — past § 7's accepted leakage, and
defeating exactly what § 3 keeps the field empty for. Refused now at three
boundaries, with a regression test that fails against the pre-fix code.

**The sharper half was that no case exercised field 4 in either direction.** The
gap was in what the fixture did not contain, not in what an assertion said. The
same shape appeared twice more in this task — `AC-3`'s first version could not
see an incomplete signed input, and `AC-1` could not see an unbound transcript.
All three were found by writing the demonstration, and none by reading the test.

## One entry changed owner, and the check is why

`nightly-fuzz-search` was registered at `C01-A` with owner and expiry `C01-I`.
That was never right — a protocol implementation does not build a nightly
schedule — and nothing noticed until C01 was ticked, at which point
`pending-contract` refused the entry as **expired at a completed task**. It is
reassigned to **C15**, which owns the nightly bucket in `TESTING.md` § Nightly
and in the workflow's owed-gate list.

Worth stating plainly: the gate did not catch a typo, it caught a **deferral
pointed at a task that could not discharge it** — which is the failure mode
`D-C01-4` names when it says a pending entry blocked on someone else's work is
how a pending list becomes permanent.

## Reproducing

```text
make check            # every gate, including contract and pending-contract
make contract         # the thirteen cases
make pending-contract # the one registered entry, still failing for its reason
go test -run xxx -fuzz FuzzDecode -fuzztime 30s ./internal/protocol/
```
