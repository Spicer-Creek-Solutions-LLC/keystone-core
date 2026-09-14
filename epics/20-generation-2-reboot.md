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
workstreams and cannot be approved directly: each needs a checked-in dossier
first. **Behaviour** workstreams additionally split acceptance-contract and
implementation tasks into separate approvals, branches and pull requests;
planning and ADR tasks may use one. G checkboxes are supporting repository
tasks — not
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
- [x] P01 — Threat model. Ten actors, 51 threats, all 24 invariants mapped,
  fourteen residual risks, eleven still accepted.
- [x] P02 — NATS-native capability ADR. `ADR-0002`: two accounts, decentralized
  JWT with the operator seed outside every Keystone process, six service
  principals plus one identity per agent, two streams with per-agent command
  consumers, and an enumerated data-plane allowlist. `RSK-10` resolved;
  `RSK-2` and `RSK-3` renewed.
- [x] P03 — Enrollment and identity ADR. `ADR-0003`: the token file carries a
  token-scoped bootstrap NATS credential, all three agent keys are generated on
  the agent, and the staged protocol ends only when revocation is verified.
  `RSK-7` resolved for agent material; `RSK-12` to P05, `RSK-13` to P10.
- [x] P04 — Subject authorization ADR and executable policy. `ADR-0004`: a
  four-token grammar `ks.<plane>.<id>.<class>`, with the identifier before the
  class so one exact `FilterSubject` per agent is possible, and `enroll` as its
  own plane so one deny closes it. Fifteen negative cases cover all five
  `ARCH-TEST-002` clauses.
- [x] P05 — Versioned encrypted protocol ADR. `ADR-0005`: every envelope class
  signed, fixed-order length-prefixed canonicalization, the version readable in
  cleartext and covered by the signature so it cannot be downgraded, and
  presence and events signed but not encrypted. `RSK-11` resolved; `RSK-1`,
  `RSK-6` and `RSK-12` renewed.
- [x] P06 — Delivery and job lifecycle ADR. `ADR-0006`: eleven server states and
  eight agent states, receipt and start as separate durable writes, the agent
  ledger authoritative over the deduplication window, and **delivery failure
  split into two outcomes with different truth values** — expiry proves the
  command did not run, exhaustion proves only that it was received. Cancellation
  separates the accepted request (`Cancelling`) from the evidence-backed outcome
  (`Cancelled`). `RSK-9` renewed to P08.
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

Not workstreams. Each is a single repository task establishing or correcting
project documentation that later work depends on. Each still needs its own
presented plan and explicit approval; no dossier is required, and cases and
limitations go in the pull request.

- [x] G01 — Terminology baseline. Rebuilds `GLOSSARY.md` for Generation 2.
  Unblocks P01, which depends on terms the Generation 1 glossary defined
  wrongly or not at all.
- [x] G02 — Defect ledger. Records recurring agent defects with root causes and
  countermeasures, and whether each countermeasure held.
- [x] G03 — Defect ledger entry `DL-8`. Records the class P01's three review
  rounds demonstrated twice: a finding repaired only where it was pointed out.
- [x] G04 — Correct `AGENTS.md` § 3's workstream-split rule, which claimed every
  P and C task splits into two agents where the execution plan splits only
  behaviour workstreams and allows planning and ADR tasks one PR.
- [x] G05 — Record `DL-8` as `Failed` and revise its countermeasure. It recurred
  in G04 while being applied, because deriving the shape of a finding was a
  judgement with no test of whether the shape was wide enough.
- [x] G06 — Align P02's scope with `GLOSSARY.md`. The P02 dossier and the
  execution plan both claimed the subject grammar and permission matrix that
  the glossary assigns to P04.
- [x] G07 — Revise `DL-8` again. It recurred in G06 under the revised
  countermeasure, this time because of how the search was written and how its
  output was trimmed rather than where it ran.
- [x] G08 — Correct P02's `AC-4`, which forbade every wildcard and so could not
  be satisfied by any ADR that permits JetStream acknowledgement.
- [x] G09 — Make P02's `AC-4` shape-aware. It required every allowlist entry to
  name one exact consumer, which a publisher's publish-acknowledgement inbox
  cannot have.
- [x] G10 — Threat-model follow-ups from P02: model the account signing key the
  ADR gives the server (`AST-16`, `THR-49`), and carry `RSK-10`'s resolution
  into the two places that still called it undecided.
- [x] G11 — Add attestation to P03's dossier. `GLOSSARY.md` assigns it to P03
  by name and the dossier's exact outputs did not carry it.
- [x] G12 — Correct P04's workstream paragraph, which asked for generated
  fixtures and acceptance cases before any build exists to produce or run them,
  and the dossier handoffs that repeated it. The related question — whether
  `ARCH-NATS-005`'s "P02/P04 must document and test" needs amending, or is
  answered by the traceability register's design-owner/evidence split — is
  **recorded in the execution plan and left open**, not closed by this task.
- [x] G13 — Correct P05's workstream paragraph, which asked for test vectors
  before any implementation exists to compute them. Same class as G12, one
  workstream over.
- [x] G14 — Model presence fabricated **on the wire** (`THR-50`). Broker
  permissions constrain agents and not the broker or the NATS operator; the
  answer is a signed presence envelope, which is P05's to shape. It does **not**
  reach a compromised server, which verifies presence and reports it and so
  needs no forgery — that path stays `THR-46` and `RSK-4`.
- [x] G15 — Close the trust-anchor gap `ADR-0005` raised: the token bundle now
  carries the service signing and result-encryption public halves, without which
  an agent cannot verify a command or encrypt a result. `THR-51` and `RSK-14`
  record what that makes the bundle worth tampering with.
- [x] G16 — Amend `ARCH-NATS-006` to name every envelope class the product
  carries. Generation 2's first RFC: `ADR-0005` signs all seven classes, which
  the invariant permitted rather than required, so nothing stopped a later ADR
  dropping the five it never named. RFC 0002 names them, states which are
  additionally encrypted, and closes the enumeration. `RSK-11`'s invariant gap
  is closed.
- [x] G17 — Bring the defect ledger's recurrence record current. `DL-8` said
  four instances while the class had recurred nine more times since its last
  revision, each recorded only in a commit message. `DL-1` and `DL-7` left
  `Proposed` for `Failed` on evidence that already existed. A recurrence is now
  recorded in the pull request that finds it, and `DL-8` gains a third tier for
  conclusions, which have no identifier to sweep on.

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
