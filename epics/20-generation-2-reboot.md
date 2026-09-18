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

- [x] P00 — Product charter and canonical journeys. Charter accepted; argv-only
  boundary frozen, and amended twice — by RFC 0003 (`G19`) and RFC 0004 (`G20`).
- [x] P01 — Threat model. Ten actors, 54 threats, all 24 invariants mapped,
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
- [x] P07 — Safe execution ADR. `ADR-0007`: argv reaches `execve` as a vector
  and `argv[0]` resolves against a **fixed** `PATH`, never the harvested one, so
  a writable profile cannot decide which binary a name means. Two components —
  an unprivileged agent holding the NATS connection, the keys and the ledger,
  and a small root executor that only changes identity and owns the process
  group. `RSK-9` renewed, narrowed against memory-safety compromise and not
  against logical compromise. Two findings raised: `ADR-0006` has no state for a
  resource-limit kill, and `ADR-0005`'s key placement stops the executor
  verifying what it runs. A third — whether an operator naming `/bin/sh` or a
  `#!` script as `argv[0]` is inside charter § 6 — `ADR-0007` § 2.1 raised and
  rightly declined to answer; **RFC 0004 (`G20`) settled it** and § 2.1 now
  records the ruling.
- [x] P08 — Persistence and audit ADR. `ADR-0008`: two stores with no
  abstraction over them, receipt and start in **separate transactions** because a
  shared one destroys the distinction `ADR-0006` § 3 pays for, and a retention
  inequality — the job identifier's tombstone outlives `KS_CMD`'s max age — which
  is the one way `ARCH-JOB-003` could have lapsed. **The agent ledger's schema and
  path are published**, so `RSK-4`'s forensic reconstruction no longer routes
  through the server it is meant to check. `RSK-1`, `RSK-4` and `RSK-9` renewed
  to P10. **P08 called P10 the first task that could demonstrate that
  reconstruction; it is not** — P10 designs the harness and nothing runs until
  P11, so P10 specifies the probe and a C-stage task performs it. `G22`
  corrected the claim where P08 wrote it.
- [x] P09 — Local operator API and authorization ADR. `ADR-0009`: a root-owned
  socket restricted to one configured admin group, with authorization evaluated
  on `accept()` from **kernel-supplied peer credentials before any request byte
  is parsed** — so no unauthenticated input reaches the parser and a **revoked
  operator whose shell still carries the cached group membership** is denied and
  recorded. The charter's seven journeys are mapped to authority; every row is
  `admin`, and **the enumeration rather than the values is the decision**, so a
  later RBAC changes cell values instead of inventing a dimension. The audit
  actor is a **uid, authoritative, plus a username marked as a snapshot**.
  `RSK-8` renewed and strengthened — the actor is unforgeable by the caller,
  which is **disclosure, not prevention** — expiring at C04.
- [x] P10 — Acceptance-harness design. `ADR-0010`: **it is two harnesses.**
  `TESTING.md` requires VM coverage wherever behaviour depends on users and
  groups, host security policy or an init system, and refuses container-only
  emulation of them at a release gate — which puts **`ADR-0007`'s privilege
  model and `ADR-0009`'s authorization model entirely on the VM side**, with no
  Docker acceptance at all. Docker carries the protocol; the VM carries the
  host. Fault points are **configuration in the shipped binary**, inert by
  default and recorded in the audit when enabled, because a binary built to be
  testable is not the binary that ships. The invariant map is
  `REQUIREMENTS-TRACEABILITY.md`, updated in place — three rows that named a
  *Test architecture ADR* now resolve — and § 14 states that
  **`ARCH-TEST-003` is not satisfied by a task that runs nothing.** `RSK-1`,
  `RSK-4` and `RSK-9` re-gate to C14, which can attempt what P08 wrongly
  expected here; `RSK-13` and `RSK-14` to C13, correcting a placement made when
  five documents thought P10 did deployment; `RSK-12` keeps a date and loses its
  task gate, because no Generation 2 task resolves it.
- [x] P11 — Repository skeleton and CI. **Delivered in two pull requests**, on
  the maintainer's instruction: `P11a` the module and the gates, `P11b` the lint
  tools and the harness shell. That deviates from `AGENTS.md` § 3's *one task,
  one approval, one PR* — recorded rather than silent. The rule exists to stop
  one approval covering several tasks, and this is the opposite: one task, two
  approvals, two reviews. The seam puts every case with a `DL-1` risk in `P11b`.
  - [x] **P11a** — the Go module returns. Three binaries that report their
    version and the configuration they would read, and nothing else: no journey
    verb, no NATS, no socket, no process, enforced against the whole module by
    `internal/cli/boundary_test.go`. A **whitespace gate over every tracked text
    file** closes the gap `markdownlint` leaves, since MD009 does not look inside
    fenced blocks and that is where every generated artifact here lives. CI gains
    the Go gates and **asserts that `make check` runs what it runs**, which
    failed on its first run and found `check` using `test` where CI used
    `test-race`. `AGENTS.md` § 2 stops saying there is no product code.
    **A defect found rather than introduced**: `.gitignore`'s `**/target/`, from
    a Rust section in a repository with no Rust, had silently hidden Generation
    1's `internal/cli/target/` — one file there reached no commit on any branch,
    including the archive. Scoped to `/target/`; the risk was live, because
    Generation 2's journeys are about targeting an agent.
  - [x] **P11b** — `tools/doclint` carries the **standing sweeps only**, each
    with a **retirement condition**, because a sweep guards a correction and
    every task adds one. `tools/archlint` checks the register in both
    directions, requires every design owner to **resolve to a document that
    exists** — the rule that would have caught *Test architecture ADR* — and
    derives liveness from the epic's own ticks via a new **`Lands at`** column,
    so the register's rule becomes enforceable task by task with no dates to
    maintain. The harness lands the topology and the **isolation probe, proved
    in both directions**: separate networks unreachable, *and the same probe
    reporting a deliberately joined pair reachable*, without which it would pass
    on a topology with a route. The suite is **outside `make check`** because
    the runner cannot run Docker in a job yet (`CI-RUNNER.md` R6), and
    `deferred-gates-check` asserts that deferral rather than leaving a gate
    that is in neither set and therefore invisible.

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

- [x] G18 — Close the two charter gaps `ADR-0006` raised. `11` is reworded from
  *agent not present* to **the target agent did not take delivery within the
  deadline**, which is the outcome rather than one of its causes and covers the
  `Undelivered` state. Journey § 5.6's observable effect becomes conditional on
  the cancellation reaching the agent first, and `keystone job cancel` exits `0`
  only when the job reaches terminal cancelled — otherwise the job's own code,
  so "I cancelled it and it ran anyway" is an outcome rather than output.

- [x] G19 — Amend RFC 0001's execution boundary. RFC 0003 admits a
  **caller-selected execution user** with a non-root deployment default, and
  admits a shell **only** to compute that user's login environment under a fixed
  agent-authored command. Operator argv still reaches no shell, which is the
  half of `THR-15`'s mitigation that protects anything. Seven forms unchanged;
  `THR-52` and `THR-53` record what the change costs.

- [x] G20 — Settle what the execution exclusions constrain. RFC 0004: they
  constrain **what Keystone constructs**, not which programs an operator may
  name, so naming `/bin/sh` or a `#!` file as `argv[0]` is inside the boundary.
  Items 1 and 3 reworded, the other six untouched, the count unchanged. It
  unblocks P07's completion and C07, which `ADR-0007` § 2.1 had stopped.
  `THR-54` records what the reading costs: an opaque payload is harder to
  review, controlled by disclosure rather than prevention.

## Epic acceptance

- [x] G21 — Say where acceptance evidence lands, and be right about it.
  `docs/dossiers/README.md` told every reader the record sits *alongside the
  dossier*; `REBOOT-EXECUTION-PLAN.md` says it lands **with the work**, and no
  dossier has ever shipped with its evidence. `P00.md` claimed its evidence
  landed in the same pull request as itself — it landed in #286, the dossier in
  #285 — and eight later dossiers copied the paragraph's shape. Review of #326
  read a dossier exactly as the README instructed and declined to approve.
  All eleven dossiers now state where their record landed, and `DL-3` records
  the oldest instance it has ever held — together with the fact that the
  comparison G21 ran is **not** retained in the tree, because no checker in this
  project ever has been.
- [x] G22 — Correct what the documents say P10 is. Six of them described it
  wrongly: `P02.md` called it *deployment and operations*, `ADR-0003` and
  `ADR-0005` gave it rotation and deployment procedures, `TESTING.md` gave its
  artifact-redaction work to P09, and `ADR-0008` and this epic said it would
  demonstrate a reconstruction. **No deployment-and-operations task exists and
  nothing runs until P11.** Three residual risks were gated on that belief. The
  ADRs keep their decisions and gain a correction note; `TESTING.md`, `P02.md`
  and this entry are corrected outright. `DL-3` records it, and a sweep over
  every tracked file asserts no document describes P10 as an operations task or
  as demonstrating anything.
- [x] G23 — Give the container suite a machine, and lock it down. This entry
  said *the forge's hosted runners cannot run containers*, established by two
  probe runs finding no `/var/run/docker.sock`, no `CAP_SYS_ADMIN` and no
  `unshare --net`, and called it structural. **G25 found the attribution was
  never made and G27 found the subject was wrong**: the probe measured the
  container a **job** runs inside, not a machine, and its own row *a job runs
  inside a container* passed. The measurements stand. What holds is that **no job
  on this self-hosted runner can reach Docker** — one runner was probed, and
  whether a hosted one is available here is unknown. That is enough:
  `TESTING.md` makes Docker the integration floor, `ADR-0010` § 2 designs the
  topology on it, and this is the runner claiming this repository's jobs.
  `CI-RUNNER.md` is the machine that fixes it, and the security decision that
  comes with it: this repository is **public** and this forge has no
  first-time-contributor gate, so the container workflow carries **no
  `pull_request` trigger at all** — defence by absence rather than a guarded
  condition — and `pull_request_target` and `issue_comment` are forbidden.
  Reporting is unaffected, which is what makes it free: a run attaches a commit
  status, a pull request's checks are its head commit's statuses, so a branch
  push reports on the pull request. `TESTING.md`'s gate schedule records the
  trigger, and **the trust boundary is stated rather than implied** — write
  access becomes code execution on that host. The runner itself is not
  registered: that needs a token only the owner can mint, and a host.
- [x] G24 — Say what the runner must be, not how to install it. `CI-RUNNER.md`
  restated Forgejo's and the distribution's instructions, and **every finding
  raised against it in review was a defect in that restatement** — `sudo`'s
  environment handling, then `runuser`'s, then a Docker repository path correct
  for one of the two distributions it named. None were facts about this project.
  It now states **seven requirements with how each is checked**, gives
  *illustrative* registration and configuration rather than instructions, and
  names Forgejo's documentation as authoritative. The rule is the one this
  repository already applies to globs and to the invariant map: **do not restate
  a source you do not control**, because nothing can gate a restatement drifting.
  A deviation found while this landed: the runner had advertised **`docker`**,
  the label `reboot-baseline` asked for on `pull_request`. This entry said a pull
  request *could have* been scheduled onto that host; **G25 established that
  every one was**, and that the rename broke the gate rather than closing the
  exposure. Changing the label was sufficient to rename it; **re-registration was
  not required**, and an earlier draft of the runbook wrongly said it was.
  Verification against the live runner then established that **`R3` holds** and
  **`R6` does not** — jobs run in a container with neither a Docker client nor a
  socket — and left **`R1` unproven**. G25 settled it, on the runner list rather
  than on the queued samples: R1 constrains this host's configuration, which an
  owner can read directly and no dispatched job can establish.
- [x] G25 — Give the baseline gate a runner, without giving it untrusted code.
  `reboot-baseline` asked for the label `docker`, which no runner claimed once
  G24's rename landed, so **every gate had been dark since**. Pointing it at
  `keystone-docker` alone would have put every pull request back on the private
  host, so the label and the `pull_request` trigger moved together — which is
  what actually closes G24's deviation. Removing that trigger would have left the
  DCO step's `if: github.event_name == 'pull_request'` permanently false: **a gate
  still present, still green, and never running again**. Its range is derived
  from the merge base with `main` instead, and `dco-exempt-check` now asserts
  that no step waits on an event the workflow does not trigger on — presence was
  never the property it claimed to check. `CI-RUNNER.md`'s reasoning rested
  throughout on a hosted pool holding the `docker` label; that premise is
  unestablished, and with it § Verification step 2's discriminator, which
  compared Docker on two runners that both fail it. Review found two more: the
  first draft read queued jobs as proof that **nothing** serves `docker`, which
  queue state cannot show — a runner may advertise a label while offline — and it
  claimed both workflows trigger on `push` *and nothing else* while the same file
  declares `workflow_dispatch`. The queue inference then survived its own removal
  in two more places, because the first sweep matched the phrasing *"nothing
  serves"* rather than the claim — **DL-8 again, and the sweep that found the
  survivors matched any sentence joining an absence to a label**. `tools/doclint`
  lands in P11b and owes a standing rule for it. The required external change is
  **applied**: `main` now requires the `(push)` context. Status checks were
  required throughout, so between G24's rename and that change `main` was
  **unmergeable** rather than merely unverified — #334 was blocked, not just
  missing a result.
- [x] G26 — Say which unit a dossier is named for. `REBOOT-EXECUTION-PLAN.md`
  and `docs/dossiers/README.md` both read **one file per task** while a
  behaviour workstream's two tasks share one dossier, and the index's
  `C05-A.md` example implied the opposite. The unit is the **workstream**. C01
  raised it and could not fix it: both documents are outside a task's paths, and
  amending a source of truth under a task approval is what its own § 1 refuses on
  `ADR-0005`'s behalf. **The first attempt made it worse** — a clarification
  added to the index while *one file per task* stood four lines above it, and the
  execution plan untouched. A convention recorded in one place and contradicted
  in two looks settled, which is worse than one that is merely ambiguous.
  `tools/doclint` carries the sweep.
- [x] G27 — Say what the probe measured. Four documents read *the runner cannot
  run containers*; the probe measured **a job's container**, and one of its own
  rows — *a job runs inside a container* — passed, so the host starts containers.
  The maintainer states this runner has run Docker since before the reboot. **The
  worst instance was introduced by G25** while correcting the attribution in the
  same sentence: it replaced a claim about a hosted pool with a claim about every
  runner, and neither was measured. **Review caught a third turn of the same
  screw in this task's own first draft** — *no job on a runner available to this
  repository* still quantified over runners nobody probed. One runner was
  measured; that is the claim. `R6` is unchanged and still unmet — a job reaches
  Docker when its image carries a client and a daemon comes with it, both
  configuration. **The sweep this task owed is deferred to `G28` and the reason is
  a defect in `doclint` itself**: `emphasised()` disposes of any hit with an
  asterisk somewhere before and after it, so a claim written in `**bold**` — which
  is how this repository writes its load-bearing claims — can never fire a rule.
  Every superseded wording here is bold. Fixing it surfaces three previously
  masked hits that need adjudicating, and shipping a sweep that cannot catch its
  own motivating instance is the `DL-1` shape this ledger exists for. **`G28`
  carries it**, and this entry was held unticked until `G28` landed the sweep —
  every other correction in this series shipped with its guard, and `G25` was
  held the same way while its branch-protection change was outstanding.
- [x] G28 — Stop `doclint` disposing of claims because they are bold, and give
  G27 its sweep. `emphasised()` returns true when a unit holds any asterisk
  before the hit and any after it — unpaired, and not distinguishing `*` from
  `**`. This repository asserts in bold, so **the claims most worth sweeping are
  the ones no rule can fire on**, and that has been true of all nine rules since
  P11b. Three parts, and the second is why this is not a one-line fix:
  - Require a genuine single-asterisk span. Bold is assertion here, not
    quotation.
  - **Adjudicate the three hits the fix surfaces.** All three are false positives
    of `runner-label-absence`, whose pattern joins `nothing`, a label word and a
    serving verb across clause boundaries — one of them is a sentence whose whole
    point is that queue state does *not* establish label absence. That rule needs
    tightening, not exempting.
  - Land `runner-cannot-run-containers`, which G27 wrote and withdrew, and
    **demonstrate it against the verbatim text of all three superseded versions
    read out of git history** — every one of them is bold, and every one passed
    against the old classifier. All three fail now, read out of `0cb125a0c`,
    `bd156c3ed` and `88ae5d1bb`.

  **Two defects were found that this entry did not anticipate, and the second is
  the worse one.**

  - **`runner-label-absence`'s spans joined clauses.** `[\s\S]{0,60}` let
    `nothing` + a label word + a serving verb match across a semicolon, flagging
    the sentence whose whole point is the correction. The spans forbid `.` and
    `;` now: an absence claim about a label is one clause.
  - **`describes` carried two literal backspace bytes**, `(?i)\x08it is not\x08`,
    so its first alternative could never match — since P11b. It survived because
    `gofmt`, `vet` and every editor render the line as correct, and because the
    broken `emphasised()` disposed of the hits it would have caught anyway. **Two
    defects hid each other**, and fixing one is what exposed the other: a hit
    stayed `LIVE` whose unit plainly contained *it is not*.
    `make whitespace-check` now rejects control characters in any tracked text
    file, with the backspace re-planted as its demonstration.

  **And two more from review**, both the same shape as the backspace: a pattern
  that reads correctly and matches the wrong thing.

  - **`describes` and `negated` lost their word boundaries.** `it is not` matched
    *it is notable*, and `not\s*$` matched a unit ending in *cannot* — so a stale
    hit in either unit was reported as handled. Restored on every alternative.
    **The backspaces were almost certainly `\b` to begin with**: these files are
    written by generator scripts, and `\b` in a non-raw Python string is a
    backspace byte. The defect and its repair have the same origin.
  - **No gate ran any tools-module test.** Each tool is its own module, so the
    root `go test ./...` never reached them: `tools/capcheck` has had a
    `main_test.go` since R08 that **has never executed**. Found while adding
    `doclint`'s. `make tools-test` runs them all and CI runs it.
- [x] G29 — Stop `CI-RUNNER.md` asking for something already done. It read
  *"Those queued samples will never run and should be cancelled"*; the maintainer
  cancelled the four stalled runs on 2026-09-17, carrying seven queued jobs
  between them. **Review caught the first draft stating two different totals** —
  the document double-counted three runs and this entry said six, a count that
  came from including three runs `cancel-in-progress` had already superseded. A
  task correcting an operational record cannot leave contradictory totals in it,
  and a second round found the corrected paragraph still ending in the request
  itself — quoted, but quoted is not the same as evidently finished.

  **A document that records an open action and is not updated when the action is
  taken reads as an obligation forever**, and this one survived G25, G27 and G28
  — each of which edited the paragraph above it. The observation is in the past
  tense now, and the paragraph says outright that it asks for nothing.

  **No sweep, deliberately.** `tools/doclint` guards superseded *conclusions*,
  and this was a stale *fact* — the claim was true when written. A rule keyed to
  "should be" would fire on every legitimate recommendation in the tree. What
  would catch this class is a check that an action a document asks for has an
  owner and an end, which nothing here has and which is a larger idea than this
  task.
- [x] G30 — Stop the tools reading whatever repository `GIT_DIR` names.
  `capcheck` and `doclint` both set `cmd.Dir = root` and inherited the ambient
  environment, and **`GIT_DIR` overrides discovery entirely** — so each read the
  repository the environment named rather than the root it was given. **Git hooks
  set `GIT_DIR`**, which makes `make check` from a pre-commit hook enough to
  reach it.
  - **`doclint` is the dangerous one.** It enumerates what it sweeps with
    `git ls-files`; pointed at another tree it would sweep nothing relevant and
    report `0 live`, which reads exactly like success.
  - **The Makefile had it too**, in every recipe and in
    `COMMIT := $(shell git rev-parse HEAD)` — a build from a hook stamps the
    wrong commit into the binaries, or an empty one. Demonstrated: under
    `GIT_DIR=/tmp`, `whitespace-check` matched no files and `commit=` was blank.
  - Found by review of `G29`, which noticed `capcheck`'s
    `TestEnumerateSourcesUnknownPin` failing under an injected `GIT_DIR`. **That
    test had been passing for a reason it did not control** — it expected failure
    on a temp directory, and only got it when no `GIT_DIR` was set. It sets one
    now, so it exercises the hazard rather than depending on its absence.
  - Only visible at all because `G28` made the tools-module tests run for the
    first time since R08.
- [x] G31 — Ask the runner the right questions. `R6`'s diagnosis rested on two
  readings and **neither holds**. The probe tested `/var/run/docker.sock` when
  this host's socket is **`/run/docker.sock`** — `/var/run` is usually a symlink
  to `/run` but a minimal image need not carry one, so the negative established
  nothing, and `CI-RUNNER.md` called that row one of *"the decisive two"*. And
  `w3` inferred *jobs run inside a container* from `/.dockerenv`, which is
  **equally true if the runner itself is containerised** and therefore cannot
  distinguish the two arrangements — the difference is what decides what `R6`
  needs.
  - The row is **withdrawn rather than reinterpreted**. `R6` stays unmet, because
    a job did not reach Docker; **its cause is now recorded as open.**
  - `.forgejo/workflows/runner-probe.yml` replaces nine pass/fail jobs with one
    that **prints**. The original shape existed because job logs are not readable
    through this forge's API — but the maintainer reads them in the web
    interface, so reporting values beats reporting a bit each. No step can fail
    the run: a diagnostic that stops at the first missing thing hides the rest.
  - `workflow_dispatch` only. It is not a gate and must never run on a push.
  - Found by the maintainer saying plainly that the runner has Docker and the
    `docker` command. **That was the third correction of this kind in a day** —
    the first two were about a hosted pool that was never measured and a claim
    quantified over runners nobody probed. The pattern is the same each time: a
    measurement of a job read as a fact about a machine.
- [x] G32 — Write down what the runner actually is. § What is deployed had been
  `*(unset)*` since G23; run `876`'s attempts 1 and 3, either side of a
  configuration change on 2026-09-18, filled it with measurements. **Review found
  the document conflating three separate probe events** — the 2026-09-15 probe,
  the 2026-09-16 verification runs and this one — with dates that could not all
  be true. `CI-RUNNER.md` now opens with one timeline naming each, and every
  finding cites which produced it.
  - **The job image is `node:22-bookworm`** and carries no `docker` binary. That
    is the whole of `R6`'s cause, and it is not a deployment mistake: `docs-lint`
    needs `npx`, and `actions/checkout` and `setup-go` are JavaScript actions
    needing Node **in the container**. No stock image carries Node and the Docker
    client both — verified against `docker:cli` and `node:22-bookworm`. **The
    client is therefore the repository's to supply**, as a pinned tarball, the way
    lychee already is.
  - **The host's socket is now mounted** — `/run/docker.sock`,
    `srw-rw---- root:983`, with the job running as `uid=0`. Measured absent
    before the change and present after.
  - **`R3` corroborated independently**: the runner `docker create`s a container
    per job, and the two cited attempts reported different hostnames.
  - **The design changed, and the reason is `R5`.** G23 chose *a daemon of the
    job's own, not the host's socket*. `R5` already conceded that the runner holds
    the host's socket, that this is root on the host, and that **"sandboxing
    inside a job is decoration; the host is the boundary"** — so once a job can
    reach Docker by any route, the container around it is not a security
    boundary. The per-job daemon was buying hygiene and paying the entire image
    cache for it. **G23 weighed it as though it were buying safety.** The
    maintainer's question — *why run Docker commands in a container instead of on
    the host?* — is what exposed that.
  - Host execution was **considered and declined**: it gives up `R3`, which
    verification has established twice.
  - **Two corrections in opposite directions.** G31 withdrew the socket row for
    testing a path that need not exist — right as method, and the withdrawn
    answer turns out to have been correct, since `/var/run` *is* a symlink here.
    And G31 doubted `w3`'s per-job-container finding; **`w3` was right.** Doubting
    a correct result is as much an error as trusting a wrong one, and this
    document has now done both about the same probe.
- [x] G33 — Give a job the Docker client, and land the container suite in both
  gate sets. `R6` was unmet for one measured reason — `node:22-bookworm` carries
  no `docker` binary — and G32 established that the client is therefore the
  repository's to supply. The workflow installs it pinned, runs `docker compose
  version` against the socket mounted in at P3, and `container-suite` moves into
  `check` and `reboot-baseline` together so `gates-agree` never sees a set it
  cannot reconcile. **`R6` was met at P4** — run `881`, `reboot-baseline` itself
  rather than a dispatched diagnostic, which is what § Verification asks for: a
  job that would fail on a misconfigured host, passing.
  - **The suite could not have failed in CI as written.** `requireDocker` called
    `t.Skip` when the client or the daemon was absent, so a job whose client
    install had silently failed would have reported a **green** gate. That is
    `DL-1` — a check that cannot fail, mistaken for evidence — recorded as a
    recurrence. The gate path now fails and names which of the two is missing;
    a developer running `go test` directly still skips, which is the only path
    that should. `make check` therefore requires Docker, and that cost was
    weighed against the skip and accepted.
  - **Debian packages rather than the static tarball, on provenance.**
    `download.docker.com` publishes **no checksum** for `docker-<version>.tgz` —
    `.sha256`, `.sha256sum` and `.asc` are each a `404` — so a pinned hash there
    asserts bytes someone fetched once. The two `.deb` hashes come from a
    `Packages` index covered by a PGP-signed `InRelease`. A binary that runs as
    root against the host's socket is worth the stronger chain. Size agreed
    rather than decided: **24 MiB against 118 MB**, the tarball's `docker/docker`
    being its second-to-last member so streaming less is not available. G32 said
    *a pinned tarball*; that settled **who supplies the client**, and the archive
    format moved on evidence.
  - **The deferral ends rather than empties.** `deferred-gates-check` existed
    because a gate in neither set is invisible to `gates-agree`. With nothing
    deferred, asserting the deferred list is empty passes by checking nothing —
    the shape it was written to prevent — so it is removed from `check`, the
    workflow and the Makefile together.
  - **Teardown is by label, on entry as well as exit.** `t.Cleanup` covers a
    failing test; it does not cover a timeout panic or a run this forge cancels
    on push, and containers created here are **siblings on the host daemon** that
    outlive the job. That is the cost `CI-RUNNER.md` states for mounting the host
    socket, and this is where it is paid.
  - **Compose is load-bearing or it is not installed.** `docker compose config`
    validates the tracked `compose.yaml` — which no gate read, though it has been
    in the tree since P11b — with a negative case proving the validator rejects a
    topology it should.
  - **Two stale claims in a merged dossier, found by the new sweep rather than
    by reading.** `C01.md` told C01 to assert its owed fuzz schedule with
    `deferred-gates-check`, a target this task deletes; and it rejected writing
    Docker tests at C01 partly because *`R6` is unmet with no owner and no
    diagnosis*. Both corrected as facts; **`D-C01-3` and `D-C01-4`'s decisions
    are untouched**, each standing on a reason that never depended on the runner.
  - **`doclint`'s `correction` classifier matched `G2\d corrected`** and would
    have stopped matching silently at G30 — a classifier narrower than its name,
    disposing of nothing while reading as though it disposed of corrections.
    Widened in touched scope.
  - **The workflow's owed-gate list named `archlint` as owed after it was paid.**
    `make archlint` had been a step above it since P11b. An obligation listed as
    owed after it is met makes the rest of the list read as less than binding.
  - `ARCH-COMM-002`'s register row has claimed gate `PR` with evidence at a path
    since P11, and **no pull request ran it**. The claim is now true. **Nothing
    in `archlint` checks that a `PR`-gated row's evidence actually runs on a pull
    request** — raised, not fixed.
- [x] G34 — Return C01's deferral mechanism to C01. G33 corrected `D-C01-4`,
  which had told C01 to assert its owed nightly fuzz search with
  `deferred-gates-check` — a target G33 deleted. The correction went further than
  the fact required: it also assigned C01 new work (*"C01 therefore **builds** the
  assertion"*, where the approved text had it **reuse** one) and asserted, in the
  dossier's own voice, that *"which target asserts it was never this dossier's to
  fix."* Neither was approved at C01, and **a G task may correct a fact in a
  merged dossier without deciding scope for the task that owns it.**
  - **The decision was never in doubt and is unchanged**: the deferral is
    asserted rather than left as an absence, because a gate in neither `check`
    nor CI is invisible to `gates-agree`. The removal of `deferred-gates-check`
    and why still stand. What returns to C01 is only **how** the deferral is
    asserted, settled at C01's own approval.
  - Whether it folds into the `pending-requirements` manifest `D-C01-2` already
    has `C01-A` building is **open, and deliberately left so** — that manifest is
    for contract cases with missing behaviour, and an owed nightly *gate* is not
    obviously one. It is a design question for `C01-A` and its reviewer.
  - **Found by re-reading the approved text against the current one**, not by a
    gate. `doclint` compares a document against conclusions it must not restate;
    nothing compares a merged document against what was approved. Raised, not
    fixed — a diff against the approving commit is a check that could exist.
- [x] G35 — Check a merged document against what approved it. Git history records
  which commits changed a document, not which paragraphs a task actually
  approved. `tools/doclint -approved-documents` therefore uses an explicit,
  post-merge approval ledger and reports the complete diff from each document's
  recorded snapshot. It is a review obligation rather than a `make check` gate:
  a pre-merge gate cannot know the eventual merge commit, and moving its own
  baseline would make it meaningless. The regression test reproduces C01 after
  G26 and G27, catches both verbatim G33 additions, and passes after the G34
  correction is recorded. The current C01 snapshot is `028019924`.
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
- [ ] Cancellation that reaches a running command terminates the complete
  process tree; one that does not is reported as the outcome the agent proves,
  never as a cancellation that took effect.
- [ ] At least three external design partners complete the core journey without
  maintainer control of the keyboard.
- [ ] The continuation gate records real repeated use and a concrete willingness
  to pay, or the project narrows/stops rather than expanding speculatively.
