# Issue tracking

## Current state

The tracker holds the Generation 2 planning state and nothing else: one
`v0.6.0` milestone, twelve workstream epics for `P00`-`P11`, a release tracker,
and a pinned reboot announcement. All of it was created by reboot task R09, and
the record of that operation — the reviewed plan with every issue body verbatim,
the journal, and the verification report — is in
[`../transition/r09/`](../transition/r09/).

There are no leaf issues. `P00` cannot start until R10 completes, so no
workstream has an approved dossier yet, and an issue is created only when the
work it describes has been accepted.

Generation 1's 106 open issues were closed during reboot task R07 as
**superseded, not completed** — closed because the project changed direction,
not because the work was finished or judged unwanted. Their titles, bodies,
labels, comments and milestone associations are all retained; the issues are
closed and marked, not emptied. The record of that operation, including the
reviewed allowlist and the verification evidence, is in
[`../transition/r07/`](../transition/r07/).

Generation 2 issues are created as work is accepted, not in advance. That is the
main lesson carried forward: the previous generation accumulated a backlog far
larger than the evidence supporting it, and an issue is a commitment that
something will be done.

## Principles

1. **An issue represents accepted work.** Ideas without acceptance belong in
   [`FUTURE-CAPABILITIES.md`](FUTURE-CAPABILITIES.md), which carries no version,
   date or ticket.
2. **Nothing is filed speculatively.** A `Future / Unscheduled` capability gets
   an issue only when it passes the promotion gate in
   [`ROADMAP.md`](ROADMAP.md).
3. **Closing is not deleting.** Issues, comments, labels, milestones and project
   cards are never deleted. Superseded work is closed and labelled.
4. **Identity is explicit.** Every Generation 2 issue carries a stable task
   identifier in its body — for example `R08`, `P02`, `C05-A` — and matching is
   done on that, never on a title. A closed Generation 1 issue must never
   suppress or deduplicate a Generation 2 one.

## Labels

| Label | Meaning |
|---|---|
| `generation/1` | Belongs to the archived Generation 1 line |
| `status/superseded` | Closed because the project changed direction |
| `kind/rfc` | Decision record or request for comment |
| `generation/2` | Belongs to the Generation 2 line, which starts from RFC 0001 |
| `kind/epic` | A workstream tracker, not a unit of work |
| `kind/tracker` | Roll-up issue for a release or milestone |
| `kind/announcement` | A statement of record, not work to be done |
| `kind/feature`, `kind/bug`, `kind/chore`, `kind/docs` | Work type |
| `area/*` | Subsystem |

Existing labels are never rewritten or deleted. A label already in use carries
meaning a later task did not assign and is not entitled to change.

## Milestones

All five Generation 1 milestones — `gate-v0.5`, `gate-v1.0`, `v0.x`, `v1.x`,
`v2.x+` — are closed. They were closed, never deleted, so the grouping every
archived issue referenced still resolves.

Generation 2 uses a single `v0.6.0` milestone, created in R09 and holding the
thirteen workstream and tracker issues. There are no pre-allocated milestones
for later versions: a milestone for work nobody has committed to is a promise
the project cannot keep, which is what the five Generation 1 milestones turned
out to be.

## Automation

There is **no automation that writes to the tracker**, and R09 re-enabled none.

The execution plan reserved R09 for re-enabling issue automation that is
generation-aware and carries a negative test. There turned out to be nothing to
re-enable: the only tracker writer was the nightly dependency-freshness job that
maintained issue #232, R06 disabled it because a nightly writer made reviewed
snapshots decay overnight and would have broken the fail-closed preconditions
R07 depends on, and R08 deleted `tools/depsoutdated` along with the root module.
Re-creating a writer in order to satisfy a requirement about writers would have
been backwards.

The requirement stands for whenever automation does return. Any tracker writer
must be generation-aware, covered by apply and postcondition tests, and must
pass a negative test proving that closing an archived issue cannot cause a
replacement to be created.

R09 ran that negative test against the live tracker rather than against a
proposed writer, which is the strongest form available with no writer to test:
archived issue #232 is still closed, no other issue anywhere on the tracker
carries its title, and `.forgejo/workflows/` contains no step that writes an
issue. The check is in `publish-verify` and its result is recorded in
[`../transition/r09/`](../transition/r09/).

Forge write limits are real and tighter than they look. Comment creation is
limited to roughly 16 per five minutes, and issue creation is tighter still and
tiered. Any bulk tracker operation needs a journal, resumability, and a
rate-limit budget. The tool that did the transition work was removed after R09,
but the operations it performed and the constraints it hit are recorded in
[`../transition/`](../transition/).

## Ticket lifecycle

1. Work is accepted — through an RFC, an epic task, or the roadmap promotion gate.
2. An issue is created with its stable task identifier in the body.
3. It is implemented on one branch, in one pull request, per
   [`AGENTS.md`](../../AGENTS.md) §3.
4. It closes when its acceptance criteria are met and the epic checkbox is
   marked, or it closes as superseded with an explanation.
