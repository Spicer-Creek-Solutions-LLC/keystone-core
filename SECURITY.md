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

Generation 2 has not been built yet. Its threat model is a deliberate,
early-sequenced deliverable (P01) rather than something retrofitted, and the
layered security design it must satisfy — NATS account isolation, per-agent
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
