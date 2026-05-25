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
[DEVELOPMENT.md](docs/project/DEVELOPMENT.md) for details and expectations.

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

- Review [SECURITY-DESIGN.md](docs/project/SECURITY-DESIGN.md) for design principles and cryptographic standards
- Understand trust boundaries your code crosses (see `docs/concepts/threat-model.md`)

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

**Release process:** Official releases follow a formal offline multi-party signing ceremony.
If you are a maintainer involved in releases, see [RELEASE-PLAYBOOK.md](RELEASE-PLAYBOOK.md)
for the complete process, key management, and quorum requirements.

## Getting Started

If you’re new:

1. Read this file
2. Skim [DEVELOPMENT.md](docs/project/DEVELOPMENT.md) for workflow details
3. Check open issues / discussions
4. Ask questions if something is unclear
5. Open a PR or RFC

## Questions?

- Open a GitHub Discussion
- Check `/docs`
- Review the `CLAUDE.md` for project context

Thank you for contributing to Keystone Core!
