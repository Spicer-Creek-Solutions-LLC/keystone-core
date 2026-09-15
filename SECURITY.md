# Security Policy

## Supported versions

**None.** There is no supported release of Keystone Core.

The `v0.1.0` and `v0.5.0` releases remain published for historical reference and
are **unsupported**: they receive no fixes, including security fixes. They were
built from the Generation 1 implementation, which was archived in September 2026
and is no longer developed. Do not deploy them.

| Version | Supported | Notes |
|---|---|---|
| `v0.5.0` | No | Archived Generation 1. Unsupported. |
| `v0.1.0` | No | Archived Generation 1. Unsupported. |
| `v0.6.0` | Not yet released | First Generation 2 release |

## Reporting a vulnerability

Email <security@keystone-core.io>. Do not open a public issue.

That address is the only reporting channel. Include what you observed, how to
reproduce it, and which commit or archived release it applies to.

**What to expect.** The project has one maintainer and no on-call rotation, so
there is no response-time commitment — a report is acknowledged when the
maintainer reads it. Because nothing is supported, a report against an archived
release will be acknowledged and recorded, but will not produce a patch. Reports
against the current repository contents — `tools/capcheck` or the published
documentation — are acted on.

The handling process, severity model and disclosure expectations are in
[`docs/project/SECURITY-RELEASE.md`](docs/project/SECURITY-RELEASE.md).

## Security posture of Generation 2

Generation 2 has not been built yet. Its threat model was written before any of
it — [`THREAT-MODEL.md`](docs/project/THREAT-MODEL.md), accepted as task P01 —
and it is the input the architecture decisions are designed against rather than
something retrofitted to them.

It models ten actors and 54 threats against the seven canonical operator
journeys, maps all 24 architecture invariants to threats, and records fourteen
residual risks with owners and expiry dates, eleven of which remain accepted.
Read § 10 first if you are evaluating it: it says what the model cannot tell
you.

The layered security design it must satisfy — NATS account isolation, per-agent
identity, exact subject authorisation, signed and end-to-end encrypted
envelopes, and local execution policy — is set out in
[RFC 0001](docs/rfcs/0001-generation-2-reboot.md) and
[`ARCHITECTURE-INVARIANTS.md`](docs/project/ARCHITECTURE-INVARIANTS.md).

Generation 1's security design documents were archived with the code they
described, so that superseded cryptographic decisions are not mistaken for
current ones. They remain readable:

```bash
git show archive-2026-09-pre-v0.6-reboot:docs/project/SECURITY-DESIGN.md
```
