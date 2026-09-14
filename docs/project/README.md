# Project documentation

Planning, governance and transition records for Keystone Core. There is no
product documentation here: the Generation 1 implementation was archived in
September 2026 and its references, guides and runbooks went with it.

## The reboot

- [`../rfcs/0001-generation-2-reboot.md`](../rfcs/0001-generation-2-reboot.md) —
  the accepted decision, its reasoning, and the alternatives rejected
- [`REBOOT-EXECUTION-PLAN.md`](REBOOT-EXECUTION-PLAN.md) — every transition and
  foundation task, with its controls and acceptance criteria
- [`PROJECT-REBOOT-REVIEW.md`](PROJECT-REBOOT-REVIEW.md) — the assessment the
  decision rests on
- [`REBOOT-DEV-VM.md`](REBOOT-DEV-VM.md) — resuming the transition on the
  development VM
- [`../transition/`](../transition/) — archive manifest, freeze record, and the
  tracker-retirement evidence

## Generation 2 foundations

- [`ARCHITECTURE-INVARIANTS.md`](ARCHITECTURE-INVARIANTS.md) — 24 normative rules
  with stable `ARCH-*` identifiers
- [`TESTING.md`](TESTING.md) — what "shipped" means; black-box acceptance against
  production binaries
- [`REQUIREMENTS-TRACEABILITY.md`](REQUIREMENTS-TRACEABILITY.md) — each invariant
  mapped to its planned evidence
- [`FUTURE-CAPABILITIES.md`](FUTURE-CAPABILITIES.md) — 703 catalogued Generation 1
  capabilities, none of them a commitment
- [`../adr/`](../adr/) — ADR templates for the Stage P decisions

## Project

- [`PRODUCT-CHARTER.md`](PRODUCT-CHARTER.md) — why this exists, the seven canonical journeys, and the `v0.6.0` business gate
- [`THREAT-MODEL.md`](THREAT-MODEL.md) — actors, threats, invariant coverage, and accepted residual risk
- [`ROADMAP.md`](ROADMAP.md) — Now, Next, Future / Unscheduled, Not Planned
- [`VERSIONING.md`](VERSIONING.md) — version policy and the `v0.6.0` gate
- [`ISSUE-TRACKING.md`](ISSUE-TRACKING.md) — tracker conventions
- [`GLOSSARY.md`](GLOSSARY.md) — terminology already decided, and what is not yet defined
- [`DEFECT-LEDGER.md`](DEFECT-LEDGER.md) — recurring agent defects, their countermeasures, and whether each held
- [`../adr/0002-nats-native-capabilities.md`](../adr/0002-nats-native-capabilities.md) — how Keystone uses NATS: accounts, identities, streams, limits
- [`../adr/0003-enrollment-and-identity.md`](../adr/0003-enrollment-and-identity.md) — how an agent acquires an identity, and what enrollment attests
- [`../adr/0004-subject-authorization.md`](../adr/0004-subject-authorization.md) — the subject grammar, the permission matrix, and what must be refused

## Governance and policy

- [`GOVERNANCE.md`](GOVERNANCE.md), [`MAINTAINERS.md`](MAINTAINERS.md),
  [`RFC.md`](RFC.md)
- [`DCO.md`](DCO.md), [`AI-CONTRIBUTIONS.md`](AI-CONTRIBUTIONS.md)
- [`SECURITY-RELEASE.md`](SECURITY-RELEASE.md) and
  [`../../SECURITY.md`](../../SECURITY.md)

## Generation 1 sources

Not in the working tree. Read them from the protected archive:

```bash
git show archive-2026-09-pre-v0.6-reboot:docs/project/DESIGN.md
git show archive-2026-09-pre-v0.6-reboot:docs/project/GETTING-STARTED.md
git show archive-2026-09-pre-v0.6-reboot:docs/runbooks/README.md
```

They are research. RFC 0001 controls.
