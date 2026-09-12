# Epic 20: Generation 2 Reboot

## Goal

Preserve the Generation 1 implementation and restart Keystone Core with a
narrow, NATS-first, acceptance-tested command-and-control foundation leading to
`v0.6.0`.

This epic supersedes the implementation sequencing in Epics 01–19. Those epics
remain Generation 1 evidence until the archive is created. They are not an
instruction to carry their scope into Generation 2.

## Governing documents

- [`docs/rfcs/0001-generation-2-reboot.md`](../docs/rfcs/0001-generation-2-reboot.md)
- [`docs/project/REBOOT-EXECUTION-PLAN.md`](../docs/project/REBOOT-EXECUTION-PLAN.md)
- [`docs/project/ARCHITECTURE-INVARIANTS.md`](../docs/project/ARCHITECTURE-INVARIANTS.md)
- [`docs/project/TESTING.md`](../docs/project/TESTING.md)
- [`docs/project/REQUIREMENTS-TRACEABILITY.md`](../docs/project/REQUIREMENTS-TRACEABILITY.md)
- [`docs/project/FUTURE-CAPABILITIES.md`](../docs/project/FUTURE-CAPABILITIES.md)

If this epic conflicts with an older epic, RFC 0001 and this epic control.

## Scope

- make the reboot decision and constraints explicit;
- freeze, inventory, and immutably archive Generation 1;
- retire Generation 1 tracker state as superseded;
- land a clean Generation 2 repository baseline without rewriting history;
- complete the product, threat-model, NATS, protocol, execution, persistence,
  and test ADRs;
- build a production-process acceptance harness before feature implementation;
- deliver secure, durable command and control over NATS;
- validate installation, operations, scale, security, usefulness, and initial
  willingness to pay with external design partners; and
- release `v0.6.0` only after all stated gates pass.

## Non-goals

- importing Generation 1 implementation code by default;
- reproducing the Generation 1 feature catalog before user evidence;
- preserving Generation 1 API, configuration, storage, or wire compatibility;
- tagging `v0.0.0`;
- calling mocks, in-process agents, or package tests end-to-end acceptance; or
- implementing Future capabilities while command-and-control gates are open.

## Tasks

Every R checkbox is a separate repository task. P/C checkboxes are program
workstreams and cannot be approved directly: their checked-in dossier splits
acceptance-contract and implementation tasks into separate approvals, branches,
and pull requests. G checkboxes are supporting repository tasks — not
workstreams, so no dossier — that correct project documentation a later
workstream depends on. Detailed controls are normative in the execution plan.

### Repository transition

- [x] R00 — Reboot assessment.
- [x] R01 — Decision and control documents.
- [x] R02 — Freeze and reconcile Generation 1.
- [x] R03 — Complete archive capability catalog.
- [x] R04 — Build tracker-retirement tooling.
- [x] R05 — Create immutable archive evidence.
- [x] R06 — Prepare CI and branch protection.
- [x] R07 — Retire Generation 1 tracker state.
- [x] R08 — Land the clean Generation 2 baseline.
- [x] R09 — Publish Generation 2 planning state.
- [x] R10 — Independent transition audit. Gate clear; `D-01` risk-accepted to 2026-12-10.

### Product and architecture foundation

- [x] P00 — Product charter and canonical journeys. Charter accepted; argv-only boundary frozen.
- [ ] P01 — Threat model.
- [ ] P02 — NATS-native capability ADR.
- [ ] P03 — Enrollment and identity ADR.
- [ ] P04 — Subject authorization ADR and executable policy.
- [ ] P05 — Versioned encrypted protocol ADR.
- [ ] P06 — Delivery and job lifecycle ADR.
- [ ] P07 — Safe execution ADR.
- [ ] P08 — Persistence and audit ADR.
- [ ] P09 — Local operator API and authorization ADR.
- [ ] P10 — Acceptance-harness design.
- [ ] P11 — Repository skeleton and CI.

### First command-and-control release

- [ ] C01 — Protocol types and cryptographic vectors.
- [ ] C02 — SQLite migrations and durable ledgers.
- [ ] C03 — NATS operator, account, identity, and permission generation.
- [ ] C04 — Local operator control substrate.
- [ ] C05 — One-use enrollment vertical slice.
- [ ] C06 — JetStream transport adapter.
- [ ] C07 — Bounded executor.
- [ ] C08 — Single-agent command vertical slice.
- [ ] C09 — Cancellation vertical slice.
- [ ] C10 — Presence and minimal inventory vertical slice.
- [ ] C11 — Operator history and diagnostics UX.
- [ ] C12 — Audit, metrics, advisories, and diagnostics.
- [ ] C13 — Packaging, install, upgrade, and uninstall.
- [ ] C14 — Adversarial and fault-matrix closure.
- [ ] C15 — Scale and soak.
- [ ] C16 — External pilot and `v0.6.0`.

### Supporting tasks

Not workstreams. Each is a single repository task correcting documentation that
a workstream depends on, and each names the workstream it unblocks.

- [x] G01 — Terminology baseline. Rebuilds `GLOSSARY.md` for Generation 2.
  Unblocks P01, which depends on terms the Generation 1 glossary defined
  wrongly or not at all.

## Epic acceptance

- [ ] Generation 1 is recoverable from protected refs and a verified bundle.
- [ ] Generation 1 issues and milestones remain readable and are accurately
  marked superseded rather than completed.
- [ ] The clean baseline preserves governance and public historical links.
- [ ] Every Generation 2 architecture invariant has executable evidence.
- [ ] NATS enforces least privilege in addition to application signatures and
  end-to-end payload encryption.
- [ ] Production binaries pass isolated Docker black-box journeys and required
  VM tests.
- [ ] Duplicate delivery never causes automatic duplicate execution.
- [ ] Cancellation terminates the complete process tree.
- [ ] At least three external design partners complete the core journey without
  maintainer control of the keyboard.
- [ ] The continuation gate records real repeated use and a concrete willingness
  to pay, or the project narrows/stops rather than expanding speculatively.
