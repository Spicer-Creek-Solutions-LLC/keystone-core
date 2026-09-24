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
  the agent, and the staged protocol ends when the server confirms the token
  spent, with bootstrap access ending at the credential's broker-enforced
  expiry. It first ended *"only when revocation is verified"*; RFC 0005 (`G48`)
  replaced that when the server's key proved unable to revoke.
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

- [x] C01 — Protocol types and cryptographic vectors. **The first Stage C task,
  and the first code in this repository that does something rather than reports
  what it would do.** Split into `C01-A`, which Codex wrote and Claude reviewed,
  and `C01-I`, the reverse; `C01-I` landed in four stages — framing, signatures,
  encryption, then the vector binaries and goldens.
  - **Three blockers had to be cleared first, and each was the same shape.**
    `ADR-0005` was structural where it needed to be normative: `G36` defined the
    length prefix, `G37` named the primitives, `G38` closed fields 2 and 5 and
    added the index that fails the build when a field's encoding is silent.
    Each fix left its neighbour, and each neighbour was found by the next task.
  - **All thirteen contract cases are live.** `pending-requirements` is down to
    `nightly-fuzz-search`, which `D-C01-4` leaves owed on purpose.
  - **Two properties no acceptance case could reach**, both found while
    demonstrating a case rather than by review. Removing the transcript from the
    hybrid KDF left `AC-1` green, because a weaker self-consistent construction
    round-trips perfectly — asserted directly instead. And `AC-3`'s first
    version flipped bytes of the signed input and verified against that same
    input, which would have passed had `SignedInput` omitted a region entirely.
  - **The expiry mechanism caught a mis-assigned owner at exactly the right
    moment.** `nightly-fuzz-search` was registered at `C01-A` with owner and
    expiry `C01-I` — a task that was never going to build a nightly schedule.
    Ticking C01 expired the entry and failed `pending-contract`, which is the
    check working rather than an obstacle. Reassigned to **C15**, which owns the
    nightly bucket in `TESTING.md` § Nightly and in the workflow's owed-gate
    list. `TESTING.md` still schedules no fuzzing at all, so `D-C01-4`'s *"a
    schedule that does not exist"* remains literally true.
  - **Review found a real leak.** `SealEnvelope` left field 4 alone, so an
    encrypted command could carry a cleartext correlation identifier — past
    § 7's accepted leakage, and defeating what § 3 says the field is empty for.
    Refused now at three boundaries. The sharper half of that finding was that
    **no case exercised field 4 in either direction**: the gap was in what the
    fixture did not contain, not in what an assertion said.
- [x] C02 — SQLite migrations and durable ledgers. C02-A's frozen contract and
  C02-I's two independent SQLite stores, crash-boundary harness, published
  ledger schema, and thirteen live acceptance cases are landed.
- [x] C03 — NATS operator, account, identity, and permission generation.
  `C03-A`'s forty-five frozen cases are live against a real broker running
  generated configuration, and since `C03-I6` the compose topology's broker runs
  it too. `internal/natsauth` produces the operator and account JWTs, the five
  service users, the per-agent and bootstrap templates, the revocation list and
  `ADR-0002` § 9's limits; every case connects with a credential it made.
  `C03-I` landed in six stages, one pull request each.
  - **Three defects the work found, each corrected where it belonged.** An
    empty NATS allow list is **unrestricted, not deny-all**, so two principals
    `ADR-0004` § 4 grants nothing could publish commands — corrected in the ADR
    at `G46`, not under this task's approval. A permissions violation arrives
    **after `Flush` returns**, so the first fixture could not observe one and
    every positive case passed against a broker that had refused the operation.
    And the permanent agent key was generated **on the server**, which
    `ADR-0003` § 4 forbids because it is what stops a compromised server
    becoming an existing agent rather than minting a new one.
  - **`ARCH-TEST-002` is paid**, all five clauses, `NEG-7` last: a revoked
    bootstrap identity is refused **at connection**, which `ADR-0004` § 9 marks
    as different in kind so it is not implemented as a publish denial.
  - **The tick waited for the output rather than the output being reassigned.**
    § 3.2's *"broker configuration the topology consumes"* was undeliverable
    inside § 3.3 and was registered as a deferred gate **expiring at this task**,
    so this line could not be ticked while it stood. `G47` granted the path once
    the maintainer approved the wiring, and `C03-I6` delivered it.
  - **Still owed and registered, not silently dropped.** No principal this
    generator produces may provision a stream — `ADR-0002` § 7 defines `KS_CMD`
    and `KS_RES` and not the identity that creates them — so the production
    provisioning identity is a deferred gate owed by C06.
- [x] C04 — Local operator control substrate.
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
  approved. `tools/doclint -approved-documents` therefore uses an explicit
  ledger containing the last sanctioned snapshot commit and the exact approved
  content digest. A document edit must update its ledger entry in the same pull
  request, so the check is a real `make check` gate without needing the future
  merge commit. The regression test replays C01's original approval through
  G26 and G27, catches both verbatim G33 additions, and passes after the G34
  correction is recorded. The current C01 snapshot is `028019924`.
- [x] G36 — Define the envelope's framing in `ADR-0005` § 1. The section said
  *length-prefixed fields* with *explicit lengths* and defined neither width nor
  byte order, so **no implementation could emit a single framing byte from it**
  and no framing vector could be derived. `C01-A` stopped on it, correctly:
  `C01.md` says every wire-visible choice is `ADR-0005`'s, and `docs/adr/` is out
  of C01's paths. **A G task amending an accepted ADR, no RFC** — the precedent is
  `G15` (`59cd56d1f`), which amended `ADR-0003` and `ADR-0005` the same way;
  RFCs 0002–0004 were for architecture invariants and RFC 0001's scope, and this
  is neither.
  - **`uint32` big-endian on every field, the signature included, and the prefix
    is part of the signed input.** That last is a security property, not a
    detail: sign the values alone and `"AB"+"CD"` and `"A"+"BCD"` are the same
    signed bytes, so an attacker re-splits fields and the signature still
    verifies — the ambiguity § 1 rejects canonical JSON to avoid. Demonstrated
    both ways in the pull request.
  - **Representable maximum stated here, enforced maximum referenced.**
    `2³²−1` is a property of the encoding; the 1 MiB cap is `ADR-0002` § 9's
    policy and is pointed at rather than copied. A receiver enforces the smaller,
    and **validates before it allocates** — four bytes can declare 4 GiB.
  - **Review found the same class of gap one paragraph later, and was right.**
    The first draft validated *"a length against the enforced maximum"* while
    that cap is `ADR-0002`'s **account payload limit** — a bound on the whole
    NATS message, not on a field — and never said whether the limit was
    per-field, cumulative, or both. § 10's *"field at its maximum length"* and
    *"field one byte over"* were still unwritable. **There is no per-field
    maximum**: the bound is on the envelope, enforced as a running budget, and
    the order of checks is now normative because § 9's coarse codes leak through
    a difference in ordering — which is what `D-C01-5` tests. Closing a gap and
    leaving its neighbour open is how this task came to exist in the first place.
  - **No new refusal codes.** § 9's *malformed envelope* and *payload too large*
    already cover truncation, disagreement and over-limit lengths, so framing
    adds no wire-visible error surface.
  - **The sentence that caused it is sharpened.** "What this ADR does not decide"
    gave C01 *"the canonical encoder"* unqualified; between that and an unstated
    format, the length prefix belonged to nobody. C01 owns the encoder as **code**.
  - **What this does not fix, said rather than left to be found.** The field
    *value* encodings — the version integer, the timestamp, the nonce — are still
    structural. That is sufficient for the framing vectors `C01-A` owes, which
    take values as inputs, and **not** sufficient to hand-write a complete
    interoperable envelope. Raised, not fixed: widening to cover it is the move
    that produced this task.
  - **`P05`'s acceptance suite passed on an unencodable specification.** Its cases
    reason *about* § 1 — classification, vector dimensions, leakage cells — and
    none asked whether § 1 was *sufficient to produce a byte*. A case that checks
    a section is internally consistent is not one that checks it is sufficient.
    Recorded as a third limit in `P05-acceptance-evidence.md`.
  - **First task to pay G35's gate.** `C01.md` gains a dependency row, so its
    ledger entry moves in the same pull request or the build fails.
  - `doclint` gains `encoder-format-is-c01s`. **Its first draft missed the
    verbatim sentence it was written for** — a list ending in a parenthetical
    owner has no verb to match — and was only found by planting the original
    text. **A `RetireAfter` of `C01-I` would also have been silently permanent**:
    the lifetime list cannot capture a workstream half, and nothing checks that
    a rule's lifetime names a task the list can contain. Raised, not fixed.
- [x] G37 — Name the protocol's primitives in `ADR-0005`. §§ 4 and 5 named key
  *roles* and no algorithms, so **no envelope could be produced**: `C01-I` stopped
  the way `C01-A` had, on the clause next to the one `G36` fixed. A G task
  amending accepted ADRs, no RFC — precedent `G15` and `G36`; no invariant moves
  and no refusal code is added.
  - **Hybrid post-quantum, both pairs.** Field 9 is Ed25519 ‖ ML-DSA-65, exactly
    **3373 bytes**, and **both must verify** — hybrid is conjunction, so a forgery
    breaks both or neither. Field 8 is an X25519 ‖ ML-KEM-768 KEM, **plaintext +
    1136 bytes**, with the transcript bound into the HKDF input because
    concatenating shared secrets alone is the known-weak hybrid.
  - **The justification is `RSK-12`, not fashion.** That row already concedes
    `AST-8` has no rotation and that *"captured ciphertext stays decryptable
    offline for as long as the key exists"* — harvest-now-decrypt-later, accepted
    and unmitigated. `AST-7` has no rotation either, so "migrate before a quantum
    computer exists" had no mechanism to migrate with. The risk row is narrowed
    **cryptographically only**; no rotation path was created and the date stands.
  - **The NATS layer stays classical and the ADR says so.** `AST-3`, `AST-6` and
    `AST-16` are NKeys — Ed25519, the broker's. `ARCH-NATS-006`'s first layer is
    out of reach whatever this project decides.
  - **Presence costs 21×** — a signed-only envelope goes from ~161 to ~3470
    bytes, and presence is the one class sent on a timer. `ADR-0005` already
    listed signing presence as a negative; that line now carries the figure.
  - **Envelope bytes are not reproducible**, verified: ML-DSA signing is
    randomized, so the same input signed twice with the same key differs. The
    ciphertext already was. **Goldens therefore cover the framing and the signed
    input, not whole envelopes** — which `AC-12`'s frozen wording permits.
  - **Zero dependencies.** Every primitive is standard library at the pinned Go
    version. NaCl box and ChaCha20-Poly1305 were rejected on that alone; pure PQ
    was rejected for giving up the security-if-either-holds property to save 96
    bytes.
  - **`ADR-0003` § 4 gains algorithms and sizes** — public halves are 1984 B
    signing and 1216 B decryption, and the token bundle's service halves are
    3200 B where they were 64.
  - **The same sentence was sharpened twice, and that is the finding.** `G36`
    fixed *the canonical encoder* and left *the signature and encryption
    operations* beside it — the identical defect, one clause to the right, found
    by the next task rather than by the fix. `doclint` gains
    `primitives-are-c01s` as the sibling of `encoder-format-is-c01s`.
  - **Both sweeps fired on this task's own drafts.** `encoder-format-is-c01s`
    caught a reworded deferral that read as the old ownership claim, and
    `primitives-are-c01s`' first draft had a **subject list narrower than its
    stale list**, so planted claims about a *cipher*, a *curve* and a *signature
    scheme* were invisible — the rule read as though it swept them and did not.
  - **Review found the mirror of that defect, and the reason it survived.**
    Widening `Pattern` to fix the first drift left `Stale` naming only the
    generic words, so a claim naming a **concrete** primitive — *"C01 chooses
    ML-KEM-768"*, *"Ed25519 is C01's choice"*, the most direct form of the claim
    the rule exists to catch — passed. **Two lists that must agree are now one
    list**, read by both halves. The first drift was caught by planting in a
    shell and **not checked in**, which is exactly why the second reached review:
    `TestPrimitiveOwnershipRuleCatchesConcreteNames` is the demonstration in the
    tree, and it fails against the pre-fix rule on precisely those cases.
- [x] G38 — Complete the envelope's field encodings, and make silence a build
  failure. `C01-I` stopped a second time: field 2, the message class, had **no
  wire encoding at all**, and field 5 had one only for agent senders. `G37` had
  specified §§ 2, 4, 5 and 6 and left both — the third task running to close one
  field's gap and leave its neighbour, each found by the next task rather than by
  the fix.
  - **Field 2 is the class's canonical ASCII token** — `command`, `cancellation`,
    `lifecycle-event` and the rest — **the same bytes § 8's header carries**. A
    token rather than an integer because the header already names the class, and
    two vocabularies for one concept is a mapping table that drifts; a wrong
    integer is a silent interop bug where a wrong token is a visible one.
  - **The header and field 2 must agree, and disagreement is `malformed
    envelope`.** Stated with its limit: field 2 is signed and the header is not,
    so this is **not** an authenticity control — the signature already decides the
    class. It exists so a broker-side router and a receiver cannot act on
    different classes for the same message. Field 2 is authoritative.
  - **Field 5's service principals get reserved tokens, and the reservation is
    the finding.** Every one of them — `command-publisher`, `enrollment-service`
    and the rest — is a **valid agent identifier** under `ADR-0004`'s grammar, so
    without a reservation an agent could be assigned `command-publisher` and sign
    as one. Nothing in the grammar prevented it and nothing would have noticed. A
    structural separation would be better and the charset has nowhere to put one.
  - **`ADR-0005` § 1 now indexes all nine fields against their encodings, and
    `archlint` fails the build if a row is blank, deferred, missing or the
    heading is gone.** The `doclint` sweeps added at `G36` and `G37` catch a
    document naming the **wrong owner**; nothing caught a document **saying
    nothing**, which is what actually blocked `C01-A` once and `C01-I` twice.
  - **The check's first draft read the wrong table**, and only planting told
    them apart. Section 1 already held a three-column table with a leading digit,
    so the regex matched it first: blanking a cell in the **encoding index** left
    `archlint` green while blanking one in the **older table** failed it. A check
    that names one table and reads another is the exact defect it exists to
    prevent. It is now anchored to its own heading, and removing that heading is
    itself a failure so the check cannot be quietly switched off.
  - **The demonstration is checked in, which `G37`'s review is why.** `archlint`
    had no test file at all; it has one now, carrying the decoy table that fooled
    the first draft and six mutations that must each be caught. It **fails
    against the unanchored check** on the decoy case. A demonstration run in a
    shell proves a check worked once and guards nothing afterwards.
- [x] G39 — Generalise `pending-contract` to more than one contract package.
  `C01-A` built the manifest, the expiry check and the immutability guard as
  general machinery, and the runner underneath was coupled to a single package
  in **three** places: a hardcoded case-to-test map, a hardcoded manifest path,
  and `./test/contract/protocol` passed to `go test`. `C02-A` could not have
  registered a case without changing it.
  - **`C02.md`'s `D-C02-4` named the collision and § 3.3 did not grant the path
    to fix it**, one section away. That is the same defect `G36` and `G37`
    produced — close a gap, leave its neighbour — and this one was mine, in a
    dossier written after the pattern had been recorded twice.
  - **A better fix than the decision proposed.** `D-C02-4` asked for
    package-qualified identifiers (`C02/AC-1`). Enumerating
    `test/contract/*/pending-requirements.json` and scoping each manifest's
    identifiers to its own package removes the need: **the directory already
    qualifies them**. The enumeration comes from the filesystem for the reason
    `docs-links` enumerates from git — a list beside the thing it lists is a
    second copy, and the copy goes stale.
  - **The case-to-test map became data.** The tool cannot import a contract
    package, so a hardcoded map was the only way to know the mapping — and a
    hardcoded map is single-package by construction. Manifest entries now carry
    a `test` field. A wrong name **self-reports**: `go test -run ^Nonexistent$`
    exits zero, and a case that passes is already fatal, so a typo surfaces as
    *"passed; pending-contract must fail"* rather than a case checking nothing.
    Demonstrated.
  - **The tool had no test file**, exactly as `archlint` had none when `G38`
    added a check to it. It has one now — and the first draft of its central
    case **passed against a single-package glob**, because the test did the
    enumerating itself and asserted only that `seen` is scoped per manifest.
    True, and not the property its name claimed. It now runs the real entry
    point and fails when a package is missed.
  - **Review found that the first draft inferred deferral from absence.**
    Dropping the hardcoded case-to-test map took an allowlist with it — the old
    tool named which entries were *allowed* to have no test and fataled on any
    other — and the replacement treated an empty `test` field as a declaration
    of intent. So a manifest entry that merely **omitted** the field was
    reported as a deferred gate and its case vanished from the runner: a typo
    could retire a real acceptance case silently, which is the one thing
    `pending-contract` exists to prevent. **It also contradicted this task's own
    claim that a bad registration self-reports** — true of a *wrong* name, not
    of a missing one. Deferral is **declared** now, with every contradiction
    fatal, and all four combinations are regression-tested.
  - **`stray-binary-check` was wrong about the name, and this task is how that
    surfaced — by committing a 4.7 MB binary with the gate green.** It predicted
    `tools/<dir>/<dir>`, the binary named after its **directory**. Go names it
    after the module path's last element, and `tools/pendingcontract/` declares
    module `…/pending-contract`, so the two differ by one hyphen. The gate now
    **enumerates** any executable regular file under `tools/` instead of
    guessing: a check that has to predict a name is wrong whenever the name is.
    Demonstrated both ways — the old form returns **0** on the binary that was
    committed; the new one catches it and three other spellings.
- [x] G40 — Generalise the remaining contract gates to more than one package.
  `G39` fixed `pending-contract` and **left its two siblings in the same
  Makefile**: `contract` ran `./test/contract/protocol` and
  `contract-immutability-check` hardcoded both the surface paths and
  `C01-A-acceptance-evidence.md`. `C02-A` could not land a second package
  without changing them. **Close a gap, leave its neighbour — the fourth time,
  and this one was one file away from the fix.**
  - **`contract` enumerates** `test/contract/*/` and fails when the directory
    holds none, so an empty tree is a failure rather than a silent pass.
  - **The guard needed a decision, not a glob.** A directory name and a task id
    are unrelated, so nothing links `test/contract/persistence` to
    `C02-A-acceptance-evidence.md`. Each evidence file now **declares the
    package it governs**, beside the commit it already records — two facts that
    only make sense together, kept together.
  - **Matched both ways, as `archlint` checks the register.** A surface with no
    declaration **fails rather than being skipped**, and a declaration naming a
    package that does not exist fails too. Skipping an undeclared surface would
    be inference from absence, which is exactly what `G39` had to undo one task
    earlier.
  - **Putting the commit inside the frozen surface was considered and
    rejected**: a file cannot contain the hash of the commit that introduces it,
    which is the chicken-and-egg `C01-A` already solved with a second commit.
  - Demonstrated four ways — an undeclared second surface fails; a declaration
    naming a missing package fails; a declared package with an unreachable
    commit fails; and `make contract` runs both packages where the pre-G40 form
    ran one. **A Makefile recipe is not something a test can be committed for**,
    so these are shell-verified and reported as such, the same caveat `C01-I1`
    made about implementation mutations.
- [x] G41 — Let `C02.md` § 3.3 permit the binary `C02-A` specified. The crash
  harness is named at `cmd/keystone-ledger-crash` in
  `test/contract/persistence/crash_harness.go`, and § 3.3 — **two sections
  away** — listed no path under `cmd/`. `C02-I` built it where the specification
  said and was out of scope by the dossier's own rule, which § 7 makes
  authoritative. **The specification was right and the list was wrong.**
  - **The fifth instance of close-a-gap-leave-its-neighbour, and the shortest
    distance yet.** `G36` left a paragraph, `G37` left a clause, `G39` left two
    targets in one Makefile, and this left a path list in the same file as the
    decision that needed it. The dossier was written *after* the pattern had
    been recorded twice.
  - **Every path `C02-I` touched was swept against § 3.3**, not just the one
    review noticed. `cmd/keystone-ledger-crash/` is the only gap; the other
    twelve files are all permitted. Fixing what was found and not looking for
    its neighbours is how this list got here.
  - **Named specifically rather than opening `cmd/`.** C02 needs one binary, and
    a path list admitting any binary is not a boundary.
- [x] G42 — Let `AC-11` require what C02 can hold, and make amending a frozen
  surface checkable. `C02-A` froze `AC-11` as *"each store **and its directory**
  must carry the owner and mode `ADR-0008` fixes"* while `C02.md` § 12 puts
  creating accounts and directories at C13. **The case demanded a property of a
  thing C02 is forbidden to create**, so no implementation could satisfy it; the
  requirement now names the store file modes, and § 5.2's row agrees. The
  directory modes and the owning accounts are owed by C13 and belong in the
  manifest as a deferred gate, which `C02-I` registers.
  - **`pending-contract` rejects a deferred entry that names a registered
    case.** `C02-I` marked `AC-11` deferred, and the tool accepted it: the case
    ran green under `make contract` while the manifest reported it as work
    nobody had done. G39 made the classification **declared** rather than
    inferred from an empty field, and left the declaration unverified. The
    surface is read from source, because `tools/` cannot import a contract
    package; an unreadable one is fatal rather than read as no cases.
  - **`contract-immutability-check` no longer re-baselines what it claims to
    reject.** It ran `git diff --quiet` against `Contract commit:`, so a pull
    request that changed a surface and moved that commit passed every gate. The
    freeze commit now never moves: every commit touching a frozen file after it
    is enumerated from git and must be declared under `Contract amendments:`
    with an approving task that exists in the epic, matched both ways.
  - **G42 is the first amendment to a frozen surface**, which is exactly why it
    could not be the change that walked through that hole. The branch
    demonstrates the gate against itself: the commit that amends the surface
    fails `contract-immutability-check` until the commit that records it lands.
- [x] G43 — Let `AC-11`'s test name claim what its requirement says. `G42`
  narrowed the requirement to *"each store file must carry the mode `ADR-0008`
  fixes"* and left the test called `TestAC11OwnershipAndModes`, which asserts no
  ownership at all. It is `TestAC11StoreFileModes` now. **The amendment that
  narrowed the requirement is the one that should have renamed it** — close a
  gap, leave its neighbour, this time inside the file the gap was in.
  - **Both frozen surfaces were swept, not just the case review named.** Of
    twenty-six case names, twenty-five are at least as narrow as their
    requirement and one claimed more. **Direction is the whole finding.** A name
    narrower than its requirement is harmless because the requirement governs —
    `AC-9` omits *"at its documented path"*, `AC-6` omits *"before publication"*
    — while a name broader than its requirement is taken for the specification
    by every reader who does not open the surface.
  - **No gate can hold this, and none is claimed.** A test name is not
    mechanically comparable to a natural-language requirement. What is enforced
    is that every surface entry has a function of that name, so a half-landed
    rename does not compile; the sweep is a reading, and
    `C02-A-acceptance-evidence.md` records it as one rather than implying a
    check.
  - **The second amendment through `G42`'s route, and the first that is not
    `G42` itself.** The commit that renames fails `contract-immutability-check`
    until the commit that records it lands.
- [x] G44 — Let `C03.md` § 3.3 permit the client `C03-I` needs. `D-C03-1`
  decided how a NATS JWT gets **signed** and weighed `jwt/v2` and `nkeys`
  against `nsc` and a hand-rolled implementation. It never weighed how a
  principal **connects**, because signing and connecting are different problems
  — and every `POS` and `NEG` case connects, so `C03-I2` could not start against
  a list that permitted only `D-C03-1`'s dependencies. § 3.3 now names
  `github.com/nats-io/nats.go`.
  - **The third instance of a dossier under-specifying a neighbour**, after
    `G39` (`D-C02-4` named a tool § 3.3 did not grant) and `G41` (`C02-A`
    specified a binary at a path § 3.3 did not permit). Each time the decision
    was right and the path list was wrong, and each time the two were sections
    apart in one file. **Writing the decision is not writing the permission to
    carry it out.** Recorded as **`DL-9`** in `DEFECT-LEDGER.md`, in this pull
    request rather than a later one, which is that document's own rule. It is
    **not** `DL-8`: nothing was reported and nothing was repaired too narrowly —
    both halves were written by one author in one pull request and merged
    inconsistent.
  - **`D-C03-1`'s approved text was not rewritten.** It weighed a signing
    library, so its counts are the record of what was decided rather than a
    running total; a paragraph records the third dependency it never considered
    and states the total after `C03-I2` — four. `G43` had to repair a number in
    one document that stopped matching a table in another, and *"two direct
    dependencies"* beside a `go.mod` holding four invites the wrong correction.
  - **§ 12 was read and deliberately left alone.** It already says the cases
    *"assert authorization outcomes, not message delivery"*, which is what a
    permission probe is. Editing a neighbour that already agrees is how a
    document acquires two statements of one rule.
  - **The boundary holds.** `internal/cli/boundary_test.go` forbids the client
    outside `_test.go`, so the contract connects and the product still cannot.
- [x] G45 — Let every task record a recurrence. `DEFECT-LEDGER.md` requires the
  task that finds a recurrence to record it **in the same pull request**, and **no behaviour dossier grants the path to
  that file**: `C01.md`, `C02.md` and `C03.md` each name it in § 1 as an input
  and none lists it in § 3.3. Every `Cxx` task was required to record a
  recurrence there and forbidden to, and every commit to it up to this one is a
  G or P task. The grant is now a standing statement in
  `REBOOT-EXECUTION-PLAN.md` § "Required task dossier" — **one statement rather
  than a line in each dossier**, because three exist and every future one would
  carry the copy.
  - **`DL-1` records why its count is low**, and does not reconstruct it.
    `C01-I` and `C02-I` both produced instances of the class their own dossiers
    name and neither could record one here. Inventing figures for tasks whose
    evidence would have to be re-derived is the drift that document exists
    against. **`C03-I2`'s own instance is `C03-I2`'s to record**, in the pull
    request that found it, which is the rule this task exists to make possible.
  - **`DL-9` gains an instance and a wider class.** It was *"a document that
    decides work its **own** permission list forbids"*; the fourth is this file
    against every dossier — an obligation imposed by a **different** document
    than the one holding the permissions. Same root cause, wider reach, so the
    entry broadened rather than a `DL-10` being invented for a variant.
  - **`DL-9` moves `Proposed` → `Failed`.** The countermeasure was written at
    `G44`, was applied at `C03-I2`, and did not prevent this: **it checks the
    paths a plan intends to touch, and the duty to record a recurrence is
    incurred by *finding* one**, which no plan enumerates in advance. The entry
    gains a third step for obligations that arise during a task.
  - **`ADR-0004` § 4 is untouched.** Review of #367 raised its unsafe mechanism
    claim in the same breath as this; it is an ADR correction and a task of its
    own.
- [x] G46 — Say what an empty allow list actually does. `ADR-0004` § 4 read
  *"NATS allow-lists are exclusive: a principal may do what appears here and
  nothing else"*, and its Denied table rested on *"an omission already
  denies"*. **Both are true of a list that exists and false of one that does
  not**: NATS reads an empty allow list as **unrestricted**. The same table
  gives `none` to the presence consumer's and the monitoring role's publish, so
  both could publish a command — `NEG-15`'s property, false in a generated
  deployment, measured at `C03-I2` rather than read.
  - **`ADR-0002` § 8 had it right all along** — *"once an allow list exists,
    every subject not on it is denied"* — and `ADR-0004` dropped the qualifier.
    The correction restores it rather than inventing a new statement.
  - **The rendering rule lands in `ADR-0004` § 7**, among the shapes rather than
    in § 4 among the policy: no permission changes, and what changes is how
    "nothing" is written down.
  - **`ADR-0002` § 8's own denial paragraph was corrected too.** It said the
    repetition was for the reader; for a principal with no list in that
    direction it is not repetition but the rule. **The distinguishing property
    is no PUBLISH grants at all**, not an absent `$JS.API` entry: the enrollment
    service and bootstrap identities reach no `$JS.API` subject either and are
    unaffected, because they publish elsewhere and so their lists exist and
    exclude.
  - **§ 15's capability row was read and left alone.** It says *"an **enumerated**
    allow list is a closed boundary"*, which speaks of a list that exists and is
    already correct. Editing a neighbour that agrees is how a document acquires
    two statements of one rule.
  - **No acceptance case was added and the frozen surface is untouched.**
    `frozenPermissionMatrix` transcribes § 4's grants, and a rendering rule adds
    none; § 9's fifteen negatives cover one instance of the defect and not the
    administrative `$JS.API` reach it also permitted. The ADR says so rather
    than implying the contract holds it.
- [x] G47 — Let `C03.md` § 3.3 permit the test that proves § 3.2's output.
  § 3.2 requires *"broker configuration the topology consumes"*; § 3.3 grants
  `compose.yaml` and `Dockerfile` and no test file beside them, and the
  container suite only runs `docker compose config`, which parses the topology
  and never brings it up. **The output was therefore unprovable inside its own
  boundary**, which is `DL-9`'s instance 5.
  - **`C03-I5` did not take the path, and did not shed the output either.** It
    registered a deferred gate expiring at **C03**, so `pending-contract` fails
    if that line is ticked while the output is absent. An earlier version of
    that task reassigned the output to C05 and ticked C03; review refused it,
    and rightly — **narrowing an approved deliverable is the same overreach as
    widening a path list**, taken in the other direction.
  - **One named file, not the directory.** `test/e2e/docker/` is P11b's, and a
    path list that admits any test beside the topology is not a boundary. The
    fourth instance of this repair after `G39`, `G41` and `G44`, and the first
    where the decision reached the maintainer before the task ran rather than
    being discovered inside one.
  - **No code.** `C03-I6` does the wiring and removes the gate; this grants the
    path, as `G41` and `G44` did before their implementations.
- [x] G48 — End bootstrap access by refusal and expiry. RFC 0005: the server
  **cannot** revoke a NATS user with the account signing key it holds. Measured
  against the pinned broker, an update it signs is acknowledged, not enforced,
  and read back as though it were; with the account not yet loaded, it makes
  the account unusable. `ADR-0002` § 13 said otherwise and is corrected.
  - **Bootstrap access now ends by the server's refusal of the spent token and
    the credential's expiry.** `ARCH-NATS-004`, charter § 5.1 and § 3's
    enrollment measure are amended, and `RSK-15` accepts the minutes-long
    window in which the credential can do almost nothing.
  - **The token has no separate secret**: the bootstrap credential is the
    secret, which `ADR-0003` § 5 never checked and § 4 forbade sending.
  - **Permanent-identity revocation is decided, not built**: immediate refusal,
    short-lived server-renewed credentials, and the operator key offline as the
    emergency path. Both broker behaviours it relies on were measured.
  - **Settles `C05.md`'s `D-C05-1`, `D-C05-9` and `D-C05-3`'s S6 half.** C05
    stays blocked on TLS, which is `G49`'s.
- [x] G49 — Implement `ADR-0002` § 12's TLS on every broker connection. The
  generator issues a CA and a broker certificate and writes a `tls` block; the
  broker is **TLS-first at TLS 1.3**, so it sends no plaintext byte, and every
  client verifies it against a configured name with
  `natsauth.ClientTLSConfig`, which cannot turn verification off.
  - **The CA's private key is never written to the deployment.** It is returned
    to the caller, like the operator seed, for an HSM, KMS, vault or offline
    media.
  - **The threat model owns both new keys**: `AST-17` (the CA key) and `AST-18`
    (the broker's), their lifecycle rows, `THR-55` broker impersonation, and
    `RSK-16` accepting that nothing revokes a broker certificate until C13's
    rotation procedure.
  - **Every choice was measured against the pinned broker first.** One changed
    the design: the broker resolves certificate paths against its working
    directory, so the configuration names them absolutely.
  - **C03's contract passes with every case over TLS**, its frozen surface
    untouched, and the topology's plaintext monitoring port is gone.
  - **Unblocks C05** (`D-C05-8`). Regulated-market posture (FIPS, FedRAMP,
    STIGs) is recorded in `ROADMAP.md` as an open question.
- [x] G50 — Supply the private signing-key persistence primitive C05 requires.
  Review of pull request #381 found that C05's durable agent identity must
  contain AST-4, but `SigningKey` exposes no representation and C05 § 3.3
  explicitly forbids changing `internal/protocol/`.
  - **The format is fixed and local:** the 32-byte Ed25519 seed followed by the
    32-byte ML-DSA-65 seed. Parsing requires exactly 64 bytes and discloses only
    the existing coarse signature refusal.
  - **Private material never crosses the agent boundary.** The API exists for
    enrolled-agent persistence; enrollment still sends only the public half.
  - **No C05 behavior is implemented here.** C05 consumes this primitive after
    G50 lands, preserving the acceptance/implementation split and its path
    boundary.
  - **Records `DL-9` instance 7.** The requirement and permission list were
    inconsistent and review caught the gap before C05 implementation began.
- [x] G52 — Make the issue tracker the channel between agents.
  `ISSUE-TRACKING.md` said `P00` could not start; the tracker had not been
  touched since R09, and requests between Codex and Claude were pasted prompts
  whose decisions lived only in conversation.
  - **A request one agent makes of another is an issue.** It carries the task
    identifier, names both agents, points at the governing text rather than
    restating it, and records the maintainer decisions no document yet holds.
  - **Program rule 5 is scoped, not relaxed.** An operation on one issue for
    one approved task shows its dry run to the maintainer verbatim and needs a
    separate apply approval; its postcondition is the issue. Anything wider
    keeps the artifact-PR flow. RFC 0001's apply-after-dry-run holds either way.
  - **No forge write.** The finished `P00`-`P11` epic issues are still open;
    closing them spans several issues and is its own full-flow operation.
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
