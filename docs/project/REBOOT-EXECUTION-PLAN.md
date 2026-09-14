# Generation 2 Reboot Execution Plan

This plan turns [RFC 0001](../rfcs/0001-generation-2-reboot.md) into program
workstreams. Before approval, each P/C workstream must be decomposed into the
task dossier described below; a workstream paragraph is not itself sufficient
implementation authority. The plan deliberately separates documentation,
reversible local changes, remote forge mutations, and product implementation.
To resume the transition from the development VM, follow the
[reboot development handoff](REBOOT-DEV-VM.md).

## Program rules

1. One repository task, one explicit approval, one branch, one pull request.
2. R-stage tasks consult the relevant Generation 1 sources. P/C-stage tasks
   consult the accepted RFCs, accepted ADRs, the product charter, architecture
   invariants, and their task dossier. Archive sources are research, not active scope.
3. Do not begin implementation until the maintainer approves that task plan.
4. Do not silently absorb discovered scope. Record it and stop for direction.
5. Remote changes require a reviewed dry run and a separate apply approval.
6. Security-sensitive implementation and acceptance review need different
   agents or a named human reviewer.
7. An implementation agent cannot weaken an approved acceptance test.
8. Every commit is DCO-signed and includes the actual AI-agent disclosure.
9. Repository tasks end with tests, documentation, a commit, a push, and a PR.
   Forge-operation and validation gates use the evidence flow below.
10. Future capabilities do not receive implementation code or leaf issues until
    promoted through a phase gate.

### Required task dossier

No P/C workstream may receive implementation approval until a checked-in dossier
defines its `Depends on` task IDs and commit SHAs, allowed paths, exact outputs,
applicable requirement IDs, acceptance cases and N/A rationale, validation
commands, documentation changes, security reviewer, rollback, and handoff
artifacts.

Dossiers live in [`docs/dossiers/`](../dossiers/), one file per task, named for
the task identifier. A dossier is a boundary, not an approval: landing one does
not authorize the work it describes.

**Every applicable acceptance case is demonstrated failing before it is
accepted.** For each case marked applicable, and for each task-specific case,
plant a defect that violates exactly that case, record the check reporting the
failure, restore, and record it passing. A case marked `N/A` is exempt — there
is no defect to plant — and instead requires a per-case rationale that the
independent reviewer checks, since a wrongly waived case is invisible to every
demonstration. Record which cases fired
for each defect: a case that fires on every defect is not discriminating and
must be tightened. The record lands with the work as
`docs/dossiers/<TASK>-acceptance-evidence.md`. A case that cannot fail is not
evidence.

**This is a floor, not sufficiency.** Each dossier states, per case, what that
case cannot detect and what would find it instead. P00's demonstrated-failing
suite still passed three severity-1 semantic defects that independent review
caught, and four of its own checks were found wrong — three too loose, one too
tight. A green acceptance suite does not discharge the independent reviewer
required by rule 6, and a dossier that presents its cases as complete coverage
repeats the defect this rule exists to prevent.

Each behavior workstream is split into at least two repository tasks:

1. `Cxx-A` — an independent agent lands the black-box acceptance contract and
   records its immutable commit SHA. Missing behavior is registered in a
   machine-checked `pending-requirements` manifest with an owner and expiry.
   Normal CI skips only that exact registered case; a separate pending-contract
   target proves it fails for the documented missing behavior.
2. `Cxx-I` — an implementation agent makes the accepted contract pass and may
   add, but not weaken, assertions. It removes the pending registration in the
   same PR. CI rejects edits to the accepted contract by the implementation
   agent unless a separately approved specification PR changed it first.

Planning/ADR tasks may use one PR. Remote forge operations use a dry-run artifact
PR, explicit apply approval, apply, then a postcondition artifact PR. Long-running
pilot/release gates retain a signed evidence report instead of inventing an
implementation commit.

## Naming and retained history

| Name | Meaning |
|---|---|
| Generation 0 | Existing older `archive/v0` lineage |
| Generation 1 | Current `v0.1.0`/`v0.5.0` implementation |
| Generation 2 | Narrow post-reboot implementation leading to `v0.6.0` |
| Archive branch | `archive/2026-09-pre-v0.6-reboot` |
| Archive tag | `archive-2026-09-pre-v0.6-reboot` |
| Development version | `0.0.0-dev+g<commit>` |
| Pilot version | `v0.6.0-alpha.N` |
| First completed reboot release | `v0.6.0` |

## Stage R — Repository transition

### R00 — Reboot assessment

**Status:** complete in commit `5ad5895d`.

**Output:** `docs/project/PROJECT-REBOOT-REVIEW.md` with product, market, and
repository evidence.

### R01 — Decision and control documents

**Status:** complete on `reboot-r01-control-docs`.

**Goal:** make the reboot, NATS-first boundary, test policy, task ordering, and
future-capability policy reviewable before changing the repository.

**Outputs:** RFC 0001, this plan, architecture invariants, Generation 2 testing
requirements, requirements traceability, and the future-capability catalog.

**Acceptance:** links pass; architecture rules have stable identifiers and all
program work has task IDs; no code deletion, archive-ref creation, tracker
mutation, or forge-setting mutation occurs. R01 is developed on
`reboot-r01-control-docs` and initially targets the R00 review branch so the two
repository tasks remain distinct.

The stacked PR is review-only until R00 lands: never merge R01 into the R00
branch. Merge R00 to `main` first, retain and retarget R01 to updated `main`
(rebasing if needed), verify the PR diff contains only R01, then merge R01.

### R02 — Freeze and reconcile Generation 1

**Status:** complete in commit `93eb147f7` (pull request #259).

**Goal:** establish a known final Generation 1 commit.

**Work:**

- announce a code freeze and disable merge automation except the freeze PR;
- reconcile README/changelog with observed release state and known red tests
  without adding product functionality; inventory and verify historical tag
  object IDs but never move, recreate, or delete those tags;
- record branch heads, tags, releases, open PRs, issue/milestone counts, CI
  contexts, repository settings, and mirror state;
- inventory issue-writing automation, especially dependency-freshness jobs;
- generate a manifest of retained paths and generated artifacts; and
- run the strongest available test and security gates, recording failures
  honestly rather than blocking preservation on unrelated cleanup.

**Acceptance:** one signed final commit; machine-readable inventory records
`generation_1_final_sha` as the exact archive target and the observed `v0.1.0`
and `v0.5.0` tag object IDs; tag-signing key and signer identity preflight
passes; no feature work occurs after that SHA without a new decision.

### R03 — Complete the archive capability catalog

**Status:** complete on `reboot-r03-capability-catalog`.

**Goal:** preserve every Generation 1 capability and limitation as discoverable
future context.

**Work:** compare `FEATURES.md`, `PROJECT-DETAILS.md`, `ROADMAP.md`, all epics,
unreleased changelog fragments, support matrices, operator references, and
runbooks. For every item record its archive source, status (`implemented`,
`partial`, `planned`, or `unknown`), known integration gaps, prerequisites, and
Generation 2 disposition.

**Acceptance:** every heading and checklist item from the source documents maps
to one catalog entry or an explicit duplicate; a second agent verifies coverage;
all entries remain `Future / Unscheduled` unless RFC 0001 includes them.

### R04 — Build tracker-retirement tooling

**Status:** complete on `reboot-r04-tracker-tooling`. The tool was `tools/transition`, removed at R09; its artifacts are in `docs/transition/`.

**Goal:** make issue retirement safe, repeatable, and auditable.

**Work:** extend `trackerctl` with dry-run default, explicit `--apply`, an exact
issue allowlist, a comment template, resumability, rate-limit handling, and
postcondition verification. Add generation-aware matching so closed Generation
1 titles cannot suppress Generation 2 issues. Issue identity uses a stable
generation/task ID in checked-in metadata and the issue body, never title or
labels alone.

Give the transition tool its own minimal Go module and locked dependencies so it
remains buildable after R08 removes the Generation 1 root module. Record the
reviewed binary checksum used for every apply.

The tool applies operations in this order: comment, labels, close. It never
deletes issues, comments, labels, milestones, or project cards.

Apply fails closed unless the reviewed manifest matches the exact forge host,
repository owner/name and repository ID, snapshot hash and cutoff, milestone and
project IDs/before-state, and each issue's number, state, identity, and
`updated_at`. Preconditions are checked before the first mutation and again
before each close. Any drift requires a new dry run and apply approval.

**Acceptance:** unit tests cover interrupted and repeated runs; fixture tests
prove issues outside the allowlist are untouched; dry-run output includes the
target count and SHA-256 of the allowlist.

### R05 — Create immutable archive evidence

**Status:** complete. Refs and bundle created; branch protection probe-verified, tag protection maintainer-applied and not independently verifiable by a non-admin token. GitHub mirror out of scope by maintainer decision.

**Goal:** preserve the final Generation 1 state before any destructive-looking
cleanup.

**Dry run:** print `generation_1_final_sha`, proposed branch/tag refs, remotes,
bundle path, checksum path, and protection changes.

**Apply after approval:**

- create and push `archive/2026-09-pre-v0.6-reboot` at exactly
  `generation_1_final_sha`;
- create and push a signed annotated
  `archive-2026-09-pre-v0.6-reboot` tag;
- create an all-refs Git bundle and SHA-256 checksum outside the working tree;
- verify the bundle by cloning it into a temporary directory;
- apply exact-name protection on both forges denying direct update, force
  update, deletion, and archive-branch merges, and denying tag update/deletion;
  perform safe negative update/delete probes where provider APIs permit; and
- record immutable commit links and bundle custody in the transition manifest.

**Acceptance:** both forges resolve the refs; recorded settings and negative
probes demonstrate the intended read-only behavior; a clean clone from the
bundle reaches the final commit; the `v0.1.0` and `v0.5.0` tag object IDs exactly
match the R02 manifest.

### R06 — Prepare CI and branch protection

**Status:** complete. `reboot-baseline` added and observed; issue-writing automation removed; required-context switch performed by the maintainer and verified through the branches API. GitHub mirror out of scope by maintainer decision.

**Goal:** prevent removal of old workflows from making `main` unmergeable.

**Work:** add and observe a minimal `reboot-baseline` workflow on a PR, then
switch required checks to that observed context before removing legacy workflow
files. If old required checks cannot pass, use a separately approved, time-boxed
protection-maintenance window: capture before-state, retain PR-only/no-force/
no-delete rules, switch only required contexts, merge, verify, and immediately
restore the intended protection. Disable every automation path that writes
issues or releases, explicitly including `.forgejo/workflows/ci-full.yml`'s
`make deps-outdated-issue`. Record settings before and after on both forges.

**Acceptance:** a test PR can merge using only the new required context; legacy
contexts no longer block; `main` requires a PR and passing CI and retains
push/force/delete restrictions. Required approval count remains explicit and
compatible with the available reviewer set.

### R07 — Retire Generation 1 tracker state

**Status:** complete. All 106 open issues retired as superseded, five milestones closed, three labels created, zero open issues remaining. Evidence in `docs/transition/r07/`.

**Goal:** close old planning state as superseded without erasing history.

The 2026-09-08 inventory found 106 open issues and zero open pull requests. R02
must refresh these counts. R07 uses an allowlist generated from the reviewed R02
snapshot, never a live “all open” selector.

The snapshot queries Codeberg with `type=issues`, paginates until empty, fetches
comments separately, and records repository ID, UTC cutoff, exact issue,
milestone, and project IDs, normalized response hashes, and the disposition of
issues opened after cutoff. Full raw responses stay with the offline archive
unless reviewed field-by-field for safe permanent publication.

**Apply sequence:**

1. disable issue-writing automation;
2. save issue metadata and comments;
3. review allowlist count and checksum;
4. create `status/superseded`, `generation/1`, and `kind/rfc` labels;
5. comment that each issue is superseded, not completed, with commit-pinned RFC,
   archive, and R03 catalog links; the branch link is convenience only and no
   not-yet-published roadmap link is promised;
6. retain existing labels and add the transition labels;
7. close allowlisted leaf issues, then tracker issues;
8. close, but do not delete, old milestones;
9. archive/rename the old roadmap project without clearing cards; and
10. verify every target and prove that no non-target changed.

**Acceptance:** a normalized verification report and manifest are committed;
interrupted apply is safe to resume; no issue outside the reviewed allowlist
changed; issues after cutoff have an explicit disposition; no new Generation 2
issue exists until verification is green.

### R08 — Land the clean Generation 2 baseline

**Status:** complete. 1,911 files removed, 79 remain. No root Go module, no build target, no installable claim. Two tool modules survived: `tools/capcheck` permanently, `tools/transition` until R09.

**Goal:** make `main` an intentionally small planning and governance repository.

Use a normal PR and deletion commit. Retain license, ownership, governance,
security reporting, contribution/DCO/AI policy, README, RFC 0001, the reboot
plan, future catalog, transition manifest, roadmap, versioning policy, minimal
CI, templates, and essential brand assets.

Remove active Generation 1 code, Go modules, generated API/config references,
runtime deployments, old implementation epics, examples, runbooks, promo and
release automation, and legacy tracker generators. These remain in the archive.
Retain the audited R04 transition tool until R09 verification completes, then
remove it or explicitly adopt it as Generation 2 maintenance tooling. Reset the
changelog with links to the archive and historical releases.

Atomically rewrite `AGENTS.md`, `ISSUE-TRACKING.md`, `VERSIONING.md`, and the
source-of-truth index for the new baseline. Remove requirements to read deleted
Generation 1 files; retain commit-pinned archive research links. Replace the
public roadmap with `Now`, `Next`, `Future / Unscheduled`, and `Not Planned`.

Update checked-in public surfaces so Generation 1 is not advertised as current:
rewrite supported versions in `SECURITY.md`; replace the documentation-site
content with the reboot notice; decide and document vanity-import retention for
P11; and update package-repository status.

**Acceptance:** baseline contains no buildable product claim; all archive links
resolve; docs lint and minimal CI pass; `git diff --find-renames` receives human
review for accidental retention/deletion.

### R09 — Publish Generation 2 planning state

**Goal:** create only the work that has passed the reboot gate.

Create a single `v0.6.0` milestone and one release tracker. Initially create one
epic issue for each P00-P11 workstream and leaf issues only for the next approved
P-stage dossier; create no C-stage leaves yet. Publish a reboot announcement
linking the RFC, archive, future catalog, and version policy. Do not create
tickets for the unversioned catalog. Re-enable only issue automation that is
generation-aware, covered by apply/postcondition tests, and passes a negative
test proving closure of old dependency issue `#232` cannot create a replacement.

As reviewed remote mutations, annotate Codeberg and GitHub `v0.1`/`v0.5`
release pages as archived/unsupported without deleting artifacts; update the
repository description, topics, and site link; publish the reboot documentation
site; and verify mirrors and package-repository landing pages no longer present
Generation 1 as current.

**Acceptance:** every issue maps to a task below; no issue title is silently
deduplicated against a closed Generation 1 issue; automation is generation-aware.

**Status:** complete on `reboot-r09-planning-state`. Milestone `v0.6.0` and
fourteen issues — a reboot announcement (#268), the release tracker
(#269), and twelve `P00`-`P11` epics (#270-#278, #280-#282). No leaf issues:
`P00` is gated behind R10. Both archived releases carry an unsupported notice,
bodies only, assets untouched. No automation was re-enabled because none
survived R08; the negative test ran against the live tracker instead. Evidence
in `docs/transition/r09/`. Outstanding maintainer actions, recorded in the
manifest: pinning #268 and deploying the docs page. The line that also listed
"repairing the GitHub mirror" was wrong — R10 found the mirror live and carrying
both archive refs.

### R10 — Independent transition audit

**Goal:** prove that preservation and reset both succeeded.

An agent that did not perform R05-R09 verifies archive refs and bundle, forge
protections, historical tags/releases, issue postconditions, milestone state,
baseline contents, CI enforcement, public links, and rollback instructions.

**Acceptance:** signed audit report has no unresolved Critical or High finding
unless the maintainer records a time-bounded risk acceptance with owner and
expiry. This is the gate to implementation.

**Status:** complete on `reboot-r10-transition-audit`. Four independent agents,
none of which performed R05-R09, produced 80 findings: 1 Critical, 11 High, 18
Medium, 11 Low, 39 pass or informational. All eleven High are resolved. The
Critical — `docs.keystone-core.io` still serving the Generation 1 site — is
risk-accepted to 2026-12-10, owner the project maintainer. Evidence, the four
raw reports and the ten maintainer decisions are in `docs/transition/r10/`.
Medium and Low findings remain open and listed. **The gate is clear; Stage P may
begin.**

## Stage P — Product and architecture foundation

No production implementation starts until P00-P11 are accepted. Each item is a
separate approval and PR.

### P00 — Product charter and canonical journeys

Define target operator, fleet profile, problems, measurable success, explicit
non-goals, and exact CLI journeys for enroll, list/presence, run, status, output,
cancel, and audit. Record the 60–90 day external validation plan and business
evidence gate. Freeze the initial argv-only execution boundary from RFC 0001,
since amended once by RFC 0003.

### P01 — Threat model

Model operator, server service, agent, NATS operator, broker administrator,
network attacker, compromised agent, compromised service credential, malicious
command author, and supply-chain attacker. Document assets, trust boundaries,
metadata leakage, denial/delay powers, key rotation, revocation, recovery, and
accepted residual risk.

### P02 — NATS-native capability ADR

Specify accounts, system account, operator/JWT trust, service/agent principals,
the direction and scope each principal requires, response permissions decision,
JetStream streams and consumers, limits, advisories, headers, TLS, credential
rotation, and deployment modes. The canonical subject grammar and the
principal-by-subject permission matrix are P04's, per `GLOSSARY.md` § Subject;
P02 names subjects by role and fixes what P04 writes its matrix against. Include an `Adopt/Evaluate/Defer/Reject` matrix
for every relevant NATS feature and the exact data-plane `$JS.API`/reply
allowlist required by pull consumption and acknowledgements.

### P03 — Enrollment and identity ADR

Specify one-use token creation, bootstrap NATS credentials, agent-generated NATS
and application keys, token subjects, server validation, permanent scoped
credentials, staged activation, fsync/rename, proof of permanent connection,
revocation, bootstrap expiry, reconnect, re-enrollment, rotation, clock
assumptions, crash recovery at every edge, and disaster recovery.

### P04 — Subject authorization ADR and executable policy

Define the canonical subject grammar and a principal-by-subject permission
matrix. Specify the NATS configuration and JWT claim shapes generated from them,
and the positive and negative authorization cases they must satisfy — what each
principal may do, and what it must be denied. Prefer one exact `FilterSubject`
per consumer; any multi-filter use must account for its authorization behavior
explicitly.

**"Executable policy" names what P04 specifies, not what it ships.** No Go
module, build or test runner exists until P11, and P11 follows P00–P10, so P04
produces no generator, no signed JWT and no runnable case. C03 implements the
generation and runs the Docker negative identity matrix against it. A fixture
with no generator and no runner is a claim nobody can check.

**An unresolved question this raises, recorded rather than decided.**
`ARCH-NATS-005` says "P02/P04 must document and test that system-subject
allowlist", and `ARCH-TEST-002` requires negative identity tests against
generated production JWTs. Neither can be executed at P02 or P04. The available
reading is that those clauses name **design ownership**, which is how
[`REQUIREMENTS-TRACEABILITY.md`](REQUIREMENTS-TRACEABILITY.md) already records
them — its columns separate *design owner* from *planned automated evidence*,
and its header states the test paths "become mandatory as the corresponding
implementation lands". Under that reading P04 owns the requirement and C03
carries the evidence, and nothing is in conflict.

**Whether that reading is sufficient, or `ARCH-NATS-005` should be amended to
say so, is not settled here.** Amending a normative invariant requires an RFC
(`REQUIREMENTS-TRACEABILITY.md` § Maintenance rules). Until one is accepted or
the reading is ratified, a P04 agent should take the register's separation as
authoritative and raise the point rather than attempt an executable test before
a build exists.

### P05 — Versioned encrypted protocol ADR

Define envelope canonicalization, version negotiation, job and correlation IDs,
signatures, recipient encryption, replay bounds, timestamps/nonces, headers,
payload and output limits, forward compatibility, **the coverage a test-vector
set must have** — which inputs, fields and boundary cases it must exercise — and
error codes.
Headers contain only routing-safe metadata—never secrets or plaintext commands.
Classify enrollment, command, cancellation, result, lifecycle event, and presence
messages by signer, verifier, encryption recipient, replay key/window, maximum
size, durability, retention, headers, and accepted metadata leakage.

**P05 specifies the vectors' coverage; C01 computes them.** A cryptographic test
vector is a concrete byte string produced by running a canonical encoding and a
signature or encryption operation over a known input. No Go module, build or
test runner exists until P11, and P11 follows P00–P10, so P05 has nothing to
compute one with. C01 implements the encoding and the operations and produces
the cross-process vectors from P05's coverage specification. **A vector nobody
can recompute is a number, not evidence** — and one written by hand before an
implementation exists is a number the implementation will be tuned to match.

### P06 — Delivery and job lifecycle ADR

Define state machines for server and agent, durable boundaries, JetStream stream
and pull-consumer configuration, ack and progress rules, publication deduplication,
redelivery, application-ledger deduplication, cancellation races, server/agent
restart, retention, delivery failure, expiry, and `UNKNOWN` reconciliation when
a verified late result arrives.

### P07 — Safe execution ADR

Define safe argv-only execution, the caller-selected execution user and its
login-environment harvest ([RFC 0003](../rfcs/0003-caller-selected-execution-user.md)),
fixed working directory,
service identity/privilege, duration/output/resource limits, process-group
cancellation, signal escalation, executable resolution, audit redaction, and
unsupported forms. Shell, stdin, scripts, pipelines, user/environment/directory
selection, batches, and interactive sessions remain out of scope.

### P08 — Persistence and audit ADR

Define separate server and agent SQLite schemas/APIs, filesystem ownership and
modes, migration ordering, transaction boundaries, ledger retention, result
storage, output truncation, append-oriented audit, sensitive-data handling,
backup, corruption response, and future Postgres/tamper-evidence decision points.
Do not introduce a shared mega-store interface.

### P09 — Local operator API and authorization ADR

Define a root-owned Unix-domain socket, mode and admin-group semantics,
peer-credential attribution, full-admin authorization for the narrow first
release, impersonation boundaries, stale-socket recovery, request limits, and
audit identity. RBAC and remote operator access remain Future.

### P10 — Acceptance-harness design

Specify the isolated Docker topology, production NATS config generation, binary
packaging, fault controls, external effect probes, negative identities, artifact
retention, VM boundary, scale profile, and local/CI entry points. Map every
architecture invariant to planned evidence.

Define artifact classification and redaction. Seed private-key, token, command,
and output canaries and scan artifacts before CI upload.

### P11 — Repository skeleton and CI

Introduce the Go module, Makefile, formatter/linter/test targets, minimal command
packages, configuration conventions, ADR tooling, architecture/traceability
lint, Docker harness shell, dependency updates, supply-chain scanning, and
`0.0.0-dev+g<commit>` version logic. No command-execution behavior lands here.

### P-stage dependency gates

| Workstream | Must follow |
|---|---|
| P00 | R10 |
| P01 | P00 |
| P02 | P01 |
| P03 | P01, P02 |
| P04 | P02, P03 |
| P05 | P01–P04 |
| P06 | P02, P04, P05 |
| P07 | P00, P01 |
| P08 | P01, P06 |
| P09 | P00, P01 |
| P10 | P02–P09 |
| P11 | P00–P10 |

## Stage C — First command-and-control release

The following sequence is intentionally serial where trust or protocol
decisions constrain later code. Parallel work is allowed only when task inputs
are already accepted and agents touch independent paths.

### C01 — Protocol types and cryptographic vectors

Implement only accepted P05 envelopes, state enums, canonical encoding,
signature/encryption operations, compatibility errors, fuzzing, and cross-process
test vectors.

### C02 — SQLite migrations and durable ledgers

Implement separate P08 server schemas/APIs for agents, enrollment, jobs, results,
and audit and agent schemas/APIs for receipts, execution, and results. Test file
ownership/modes, migration ordering, corruption, rollback, concurrency, and
crash boundaries without a shared mega-store interface.

### C03 — NATS operator, account, identity, and permission generation

Implement P02/P04 configuration generation and validation. The Docker negative
identity matrix is acceptance, not optional follow-up work.

### C04 — Local operator control substrate

Implement the P09 Unix-domain socket authentication, request framing, peer
attribution, CLI connection/configuration, and minimal token/agent/job endpoints
needed by later black-box contracts. Unsupported operations return stable errors;
no command transport lands here.

### C05 — One-use enrollment vertical slice

Implement token creation through the public CLI and the complete staged
bootstrap-to-permanent-identity journey over NATS. Acceptance restarts the real
agent at every transition, proves permanent-connect before revocation, and proves
expired/revoked bootstrap credentials fail.

### C06 — JetStream transport adapter

Provision bounded streams and one exact-filter durable pull consumer per agent;
implement only transport binding, pull, ack/progress, publication ack,
redelivery/advisory exposure, and broker deduplication contracts. Agent receipt
and execution lifecycle belongs to C08; server logical dispatch/result state
belongs to C08. Do not add a parallel Keystone retry scheduler.

### C07 — Bounded executor

Implement argv-only execution with the caller-selected user, its environment
harvest and fixed directory,
output/time/resource bounds, full process-tree cancellation, result capture, and
policy. The RFC non-goals cannot be expanded by this task.

### C08 — Single-agent command vertical slice

Join C01-C07 in separately running server and agent processes. Implement exact
agent targeting, durable server dispatch, encrypted publication, durable agent
receipt, at-most-one execution attempt, ack/progress, durable encrypted result,
verification/decryption, late-result reconciliation, and public CLI
`run`/`status`/`output`. Prove the entire journey and every crash boundary with
a non-idempotent external counter.

### C09 — Cancellation vertical slice

Implement durable cancellation intent, NATS cancellation subject, race handling,
full process-tree termination, idempotent repeated cancellation, and audit.

### C10 — Presence and minimal inventory vertical slice

Implement authenticated presence with agent ID, hostname, OS, architecture,
agent version, connection epoch, and last-seen status. Do not build an open-ended
facts system.

### C11 — Operator history and diagnostics UX

Complete the P00 list/show/history/audit journeys and user-facing distinctions
among denied, unavailable, delivery-failed, expired, timeout, failed, cancelled,
unknown, and superseded-by-result. Acceptance uses only the public CLI.

### C12 — Audit, metrics, advisories, and diagnostics

Connect lifecycle audit, NATS advisories, consumer lag/redelivery, authorization
violations, resource limits, health, and operator-facing distinctions among
offline, denied, timeout, failed, cancelled, and unknown.

### C13 — Packaging, install, upgrade, and uninstall

Build unsigned CI packages, checksums, and service definitions, least-privilege users,
configuration/data ownership, upgrade/rollback behavior, clean uninstall, and
fresh-VM tests on the declared Linux support set.

Release signing uses a documented human ceremony. Implementation agents and CI
never receive private release-signing material.

### C14 — Adversarial and fault-matrix closure

Run the complete identity, malformed envelope, replay, duplicate, restart,
partition, cancellation, storage-full, broker-limit, and artifact-canary suite.
Resolve every Critical and High finding before pilot builds unless the
maintainer records a time-bounded risk acceptance with owner and expiry.

Severity uses CVSS as input plus Keystone-specific impact. A finding that can
cross an identity boundary, expose command/result plaintext, execute without
authorization, repeat a command, or conceal execution ambiguity is at least
High unless the independent reviewer documents why not.

### C15 — Scale and soak

Run at least 100 agents for one hour. Gate on stable resources, bounded consumer
lag and storage, no duplicate execution, usable diagnostics, and documented
capacity assumptions.

### C16 — External pilot and v0.6.0

Publish `v0.6.0-alpha.N` only after an independent security review. Require at
least three external design partners, installation without maintainer control
of the keyboard, sustained real-fleet use, and evidence of willingness to pay.
Release `v0.6.0` only when the product and business gates in the charter pass.

### C-stage dependency gates

Each row means the accepted implementation (`-I`) of the prerequisite, not only
its acceptance contract (`-A`). Dossiers may narrow paths further but cannot
weaken these gates.

| Workstream | Must follow |
|---|---|
| C01 | P05, P11 |
| C02 | P08, P11, C01 |
| C03 | P02, P04, P11 |
| C04 | P09, P11, C02 |
| C05 | C01–C04 |
| C06 | C01–C05 |
| C07 | P07, P11 |
| C08 | C01–C07 |
| C09 | C08 |
| C10 | C03–C05 |
| C11 | C08–C10 |
| C12 | C08–C11 |
| C13 | C12 |
| C14 | C13 |
| C15 | C14 |
| C16 | C15 |

## Promotion gate for future capabilities

A capability may move from `Future / Unscheduled` to `Next` only when:

- at least two external operators connect it to a repeated problem;
- the current command-and-control SLOs and security gates remain green;
- prerequisites and operational cost are understood;
- an RFC defines its minimal slice and explicit non-goals;
- its production-process Docker tests are designed before implementation;
- required VM tests are identified; and
- the maintainer accepts its maintenance and commercial rationale.
