# Project Ownership

This document describes the ownership and governance structure of Keystone
Core. It is informational, not a legal contract.

## Project Sponsor

**Keystone Core is a project of Spicer Creek Solutions LLC** ("SCS").

SCS is the entity behind the project and holds the operational responsibilities
that come with running an open-source project: maintaining the public
hosting accounts, the project domain, the release infrastructure, and the
governance authority that selects project leadership.

## What SCS owns and operates

| Asset | Held by | Notes |
|---|---|---|
| Project domain | SCS | `keystone-core.io` (and the vanity Go import path `go.keystone-core.io/keystone-core`) |
| Code-hosting accounts | SCS | `codeberg.org/Spicer-Creek-Solutions-LLC` (primary), `github.com/Spicer-Creek-Solutions-LLC` (code-only mirror) |
| Release infrastructure | SCS | Build/sign infrastructure, release-signing keys (when the v1.2 multi-party signing ceremony lands per `RELEASE-PLAYBOOK.md`) |
| Governance authority | SCS | SCS appoints the BDFL; see `docs/project/GOVERNANCE.md` |

SCS does **not** currently hold a registered trademark on the name "Keystone
Core". If a trademark is registered or claimed in the future, this document
and `NOTICE` will be updated accordingly.

## Code copyright

Code copyright is **not** assigned to SCS. The project follows the standard
Apache 2.0 + DCO ("Developer Certificate of Origin") model:

- Each contributor retains copyright on their own contributions.
- Each contribution is licensed to the project (and to downstream users)
  under the Apache License 2.0, the project's chosen license.
- Each commit must be DCO-signed-off (`git commit -s`); see
  `docs/project/DCO.md`.

The collective copyright line `Copyright 2026 The Keystone Core Authors`
in `NOTICE` and source files refers to the **collective of all
DCO-signed-off contributors** to the repository.

This is the same model used by the Linux kernel, Kubernetes, PostgreSQL,
and most modern open-source projects. SCS holds operational and brand
authority; the code itself is owned by its contributors and licensed under
Apache 2.0.

### What this means in practice

- SCS cannot unilaterally relicense the codebase under a different license.
  A relicensing would require the consent of contributors whose code remains
  in the repository (or the rewriting of those contributions).
- Contributors do not need to sign a Contributor License Agreement (CLA).
  A DCO sign-off is sufficient.
- Forks of the repository carry the same Apache 2.0 license. Forks may not
  use the "Keystone Core" name as a project identifier without permission
  from SCS, but they retain full freedom to use the code.

## Governance

Day-to-day technical direction is set by the project's **BDFL** (Benevolent
Dictator for Life) and its **maintainers**, per `docs/project/GOVERNANCE.md`.
SCS holds the authority to appoint the BDFL but does not direct technical
decisions. In practice, during the v1.0 reconstruction, the BDFL role is
held by an officer of SCS.

Maintainers are nominated and added per the process described in
`docs/project/GOVERNANCE.md` and `docs/project/MAINTAINERS.md`.

## Contact

- For project / technical questions: open an issue at the Codeberg repository.
- For ownership, governance, or legal questions: open an issue tagged
  `governance`, or contact a maintainer directly.

## Future evolution

This document will be updated if:

- SCS registers a trademark.
- The governance model changes (e.g., transfer to a foundation).
- The licensing or contribution model changes (e.g., introduction of a CLA
  for a future commercial edition under the open-core model).

Material changes are made via the project's RFC process per
`docs/project/RFC.md`.
