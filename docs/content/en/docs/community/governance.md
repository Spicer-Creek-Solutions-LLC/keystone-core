---
title: "Governance"
weight: 6
description: >
  How Keystone Core is governed and decisions are made
---

Keystone Core uses a BDFL (Benevolent Dictator for Life) + maintainer governance model. This page
summarizes the governance structure; the authoritative documents are in the repository root.

## Governance Documents

The following documents in the repository root define how the project operates:

| Document | Purpose |
|----------|---------|
| [GOVERNANCE.md](https://github.com/shawnbutts/keystone-core/blob/main/GOVERNANCE.md) | Roles, decision making, breaking changes policy |
| [MAINTAINERS.md](https://github.com/shawnbutts/keystone-core/blob/main/MAINTAINERS.md) | Maintainer responsibilities and expectations |
| [RFC.md](https://github.com/shawnbutts/keystone-core/blob/main/RFC.md) | Proposal process for larger changes |
| [COMPATIBILITY.md](https://github.com/shawnbutts/keystone-core/blob/main/COMPATIBILITY.md) | Versioning, release cadence, support windows |
| [CODE_OF_CONDUCT.md](https://github.com/shawnbutts/keystone-core/blob/main/CODE_OF_CONDUCT.md) | Community behavior expectations |

## Summary

### Roles

- **BDFL**: Guides project direction, approves breaking changes and governance updates
- **Maintainers**: Review contributions, enforce compatibility policies, make technical decisions
- **Contributors**: Anyone submitting issues, code, docs, or proposals

### Decision Making

Most changes are handled by maintainers through normal PR review. Larger changes that affect
compatibility, architecture, or user experience require an RFC proposal.

### Breaking Changes

Breaking changes require:
- An RFC proposal (unless trivial)
- Maintainer agreement
- BDFL approval

See [COMPATIBILITY.md](https://github.com/shawnbutts/keystone-core/blob/main/COMPATIBILITY.md) for
versioning guarantees and upgrade paths.

## Questions?

- Open a [GitHub Discussion](https://github.com/shawnbutts/keystone-core/discussions)
- Review [CONTRIBUTING.md](https://github.com/shawnbutts/keystone-core/blob/main/CONTRIBUTING.md)
