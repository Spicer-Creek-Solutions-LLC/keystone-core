# C03-I acceptance evidence

This records the implementation half of C03, landed in five stages against the
contract `C03-A` froze. Every one of its forty-five cases is live;
`pending-requirements.json` holds no case, and one deferred gate.

**C03 is not complete.** § 3.2 requires *"broker configuration the topology
consumes"*, and `compose.yaml`'s broker does not consume it — see § Limits. The
deferred gate expires at **C03**, so ticking that task while it stands fails
`pending-contract`.

## The stages, and what each closed

| Stage | Cases | Pull request |
|---|---|---|
| `I1` | `GEN-1`–`GEN-7` — the generator, no broker | #365 |
| `I2` | 16 core `POS` — generated configuration boots a broker | #367 |
| `I3` | 11 core `NEG` | #370 |
| `I4` | 7 `POS` + 3 `NEG` on JetStream subjects | #371 |
| `I5` | `NEG-7` — revocation refused at connection | this one |

`internal/natsauth` generates the operator and account JWTs, the five service
users of `ADR-0002` § 3, the per-agent and bootstrap templates, the account
revocation list, § 9's limits, and the `nats-server.conf` a broker runs.

## What the cases are measured against

**A real broker, on generated configuration.** `ADR-0010` § 3 forbids a
test-only authorization adapter, and there is none: every case connects with a
credential the generator produced, to `nats:2.15.0-alpine` running the
configuration it wrote. No JWT is committed.

**`GEN-3` and `GEN-4` compare generated JWTs against `C03-A`'s frozen matrix in
both directions** — no grant outside it, no grant missing from it — and read the
decoded JWT rather than the generator's own record of what it meant to write.

## Demonstrated failures

Every case was demonstrated against a defect planted in the input it claims to
inspect, and the failure set recorded per mutation, because a case that fires on
every defect is not discriminating. The harnesses, their invocations and their
output are in the pull requests above; each refuses to report a result when an
anchor is not unique or a plant does not compile, and each carries a self-test
that plants a deliberately misaimed defect and requires the harness to reject
it.

Of the forty-five, **every case failed against its own defect, and
forty-four of them failed alone.** `NEG-4` is the single contract case whose
mutation necessarily takes another with it: any grant defeating its subtree
denial also defeats `NEG-3`'s class denial, because `ks.job.*.>` contains
`ks.job.*.cmd`. `POS-3` and `POS-4` share one grant and **did** each fail
alone, because their mutations narrow it rather than remove it.

Two rows in the harnesses are **not contract cases at all** and are declared as
controls: a blanket denial of everything an agent holds, which must fail every
agent denial case — that is the mutation a case asserting only "it was refused"
would pass — and revoking every bootstrap identity, which must fail `NEG-7`
through its control as well as the two cases that use a bootstrap identity.

## Three defects the work found

**An empty NATS allow list is unrestricted, not deny-all.** `ADR-0004` § 4 gave
`Publish: none` to the presence consumer and the monitoring role and reasoned
that an omission already denies. Both could publish a command — `NEG-15`'s
property, false in a generated deployment. `G46` corrected the ADR; the
generator renders a grantless direction as an explicit deny.

**A permissions violation is reported asynchronously**, after `Flush` returns,
so the first fixture read its error record too early and every positive case
passed against a broker that had refused the operation. Replaced by a control
refusal each assertion waits for, which also fails loudly when the mechanism is
not working — and that is what exposed the defect above. `DL-1`.

**The permanent agent key was generated on the server.** `ADR-0003` § 4 keeps it
on the agent, which is what lets a compromised server mint a *new* identity and
not *become* an existing one. `Config.Agents` now carries the agent's public
half and nothing else.

## Feature acceptance table

**Authorization denial** is the row C03 owns, and `NEG-1`–`NEG-15` pay it
against generated production JWTs. Intended target, non-target, server
isolation, payload protection, duplicate delivery, restart, cancellation, audit
and diagnostics are `N/A`: C03 runs no journey.

## Limits

**`ARCH-NATS-007` is partly paid.** § 9's account limits are rendered and
`GEN-5` asserts none is absent or infinite; the stream and consumer limits are
values C06 applies.

**The compose topology does not consume the generated configuration, and that
is an approved § 3.2 output of this task.** The container suite only runs
`docker compose config`, which parses the topology and never brings it up, so
proving the wiring needs a test under `test/e2e/docker/` — a path § 3.3 does not
grant. `DL-9` instance 5.

An earlier version of this file registered it as owed by **C05** and marked C03
complete. **Reassigning an approved output is the maintainer's decision, not
this task's**, and review caught it. The gate now expires at **C03**, so the
epic cannot be ticked while the output is absent.

**No frozen case covers `ADR-0003` § 4's key custody**, and `GEN-7` does not:
its requirement is the *operator* seed and it searches for that one. Package
tests in `internal/natsauth` hold the property for agent seeds.

**Who provisions streams in a deployment is unanswered.** `ADR-0002` § 7 defines
the two streams and not the identity that creates them. The contract's
provisioner is a test fixture minted outside the generated matrix, which is why
`GEN-3` does not see it.

These cases prove the broker enforces what `ADR-0004` § 4 specifies. They do not
prove that § 4 specifies the right permissions.

## Validation

```text
make check
make contract
make pending-contract
go test ./internal/natsauth/...
```
