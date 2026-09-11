# Contributing to Keystone Core

Thanks for your interest in contributing! We’re friendly, pragmatic, and focused on building a solid
infrastructure system with good upgrade and compatibility behavior.

You don’t need permission to open issues or PRs. If you’re unsure about an idea, ask — it’s cheaper
to discuss early than rewrite late.

## Ways to Contribute

- Bug reports
- Feature requests
- Documentation improvements
- Code contributions
- RFCs for larger changes (see [RFC.md](docs/project/RFC.md))

## AI‑Assisted Contributions

AI‑assisted contributions are welcome. Use whatever tools help you think and iterate. See
[AGENTS.md](AGENTS.md) for details and expectations.

## Compatibility Awareness

Keystone Core cares about upgrade safety and compatibility. Minor changes are easy to merge; big
changes require proposals ([RFC.md](docs/project/RFC.md)) so operators aren't surprised.

## Governance

Keystone Core uses a BDFL + maintainer model (see [GOVERNANCE.md](docs/project/GOVERNANCE.md)). Decisions are technical and
consensus‑driven, with a project lead who resolves edge cases.

## Code of Conduct

Be respectful and constructive. See `CODE_OF_CONDUCT.md` for details.

## Security Guidelines

Keystone Core handles sensitive infrastructure operations. Security-conscious development is required.

**Before writing code:**

- Generation 2's threat model is a Stage P deliverable (P01) and does not exist yet.
  Until it does, raise security-relevant design questions in an RFC rather than
  assuming a boundary. Generation 1's security design is archived at
  [`SECURITY-DESIGN.md`](https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/src/tag/archive-2026-09-pre-v0.6-reboot/docs/project/SECURITY-DESIGN.md) and describes a
  system that no longer exists.

**During development:**

- Validate all external input using `pkg/security.Validate*` helpers
- Use parameterized queries — never concatenate SQL strings
- Avoid shell injection — never pass user input to `exec.Command` without validation
- Handle errors securely — don't expose internal details to users
- Use structured logging with automatic redaction for sensitive data

**PRs requiring security review:**

- Authentication or authorization logic
- Cryptographic operations
- Database queries with user input
- File operations with user-supplied paths
- Credential or secret management
- Audit logging changes

**Reporting vulnerabilities:** See `SECURITY.md` for responsible disclosure procedures.

**Release process:** there is nothing to release. The first Generation 2 release is
`v0.6.0`, and its packaging and signing ceremony are decided in Stage C (C13). Generation
1's release playbook is archived at [RELEASE-PLAYBOOK.md](https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/src/tag/archive-2026-09-pre-v0.6-reboot/RELEASE-PLAYBOOK.md).

## Getting Started

If you’re new:

1. Read this file
2. Skim [AGENTS.md](AGENTS.md) for workflow details
3. Check open issues / discussions
4. Ask questions if something is unclear
5. Open a PR or RFC

## Changelog entries

There are none. Per-pull-request changelog fragments were removed with the
Generation 1 code they described, and there is nothing released to describe.
The transition record lives in [`docs/transition/`](docs/transition/) and the
task list in [epic 20](epics/20-generation-2-reboot.md).

## Questions?

- Open a GitHub Discussion
- Check `/docs`
- Review the `CLAUDE.md` for project context

Thank you for contributing to Keystone Core!
