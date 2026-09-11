# Versioning

## Current state

**Nothing is released and nothing is supported.** The next release is `v0.6.0`.

| | |
|---|---|
| Development builds | `0.0.0-dev+g<commit>` |
| Public `v0.0.0` tag | never created |
| External pilot builds | `v0.6.0-alpha.N` |
| First completed Generation 2 release | `v0.6.0` |

Historical tags are unchanged and remain valid. `v0.1.0` and `v0.5.0` are
published, archived and unsupported. Generation 2 development versions must not
be derived from them — a build reporting `0.5.1-dev` would imply a continuity
that does not exist.

The jump from `v0.5.0` to `v0.6.0`, skipping `v0.0.0`, preserves SemVer ordering
and avoids tooling interpreting the reboot as a downgrade.

## Compatibility

Generation 2 intentionally breaks every Generation 1 API, CLI, configuration
format, storage layout, wire protocol, package and deployment assumption.

There is no migration path and no user-data migration, because there are no
users. Archived releases stay available for historical reference.

## The `v0.6.0` gate

`v0.6.0` ships when the product promise in
[RFC 0001](../rfcs/0001-generation-2-reboot.md) is demonstrably true, not when a
feature count is reached:

> An operator can securely enroll a Linux agent, target it, execute a bounded
> command over NATS, observe its durable lifecycle, cancel it, and retrieve an
> auditable result.

Required before release:

- every architecture invariant in
  [`ARCHITECTURE-INVARIANTS.md`](ARCHITECTURE-INVARIANTS.md) has executable
  evidence registered in
  [`REQUIREMENTS-TRACEABILITY.md`](REQUIREMENTS-TRACEABILITY.md);
- black-box acceptance against production binaries, per
  [`TESTING.md`](TESTING.md) — Docker minimum, VM where the change touches init
  systems, reboot, networking, firewalls, mounts, LVM, packages or kernel
  behaviour;
- the adversarial and fault matrix closed, with no unresolved Critical or High
  finding unless the maintainer records a time-bounded risk acceptance with an
  owner and expiry;
- scale and soak passed; and
- an independent security review before any `v0.6.0-alpha.N` reaches an external
  partner.

`v0.6.0` itself additionally requires the business gate: at least three external
design partners completing the core journey without maintainer assistance,
sustained real-fleet use, and evidence of willingness to pay. If that evidence
does not appear, the project narrows or stops rather than expanding
speculatively.

## Signing

Generation 1 shipped unsigned under a documented carve-out. That carve-out does
not carry forward: it covered a specific release line that no longer exists.
Release signing for Generation 2 is decided in Stage C packaging (C13), which
requires a documented human ceremony with private material never reaching CI or
an implementation agent.

The archive tag `archive-2026-09-pre-v0.6-reboot` is signed.
