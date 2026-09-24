# Issue tracking

## Current state

The tracker holds what R09 created — one `v0.6.0` milestone, twelve workstream
epics for `P00`-`P11`, a release tracker, and a reboot announcement (#268) — and
the Generation 2 issues filed since. The record of R09's operation — the
reviewed plan with every issue body verbatim, the journal, and the verification
report — is in [`../transition/r09/`](../transition/r09/).

**The `P00`-`P11` epics are complete and their issues are still open.** The
epic's checkboxes were ticked as each workstream merged, and no task closed the
issues: closing one is a forge operation, and none was approved. Closing them
spans several issues, so it takes the full flow in
[Forge operations on one issue](#forge-operations-on-one-issue) and is its own
operation.

No C-stage work was tracked here until `G52`. From `G52` on, a request one agent
makes of another is an issue — [Requests between agents](#requests-between-agents).

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

## Requests between agents

Behaviour workstreams alternate agents: one writes the acceptance contract, the
other the implementation, and whichever writes the code, the other reviews
([`AGENTS.md`](../../AGENTS.md) § 3). When one agent needs work from the other —
a contract amendment the implementation cannot write for itself, a review — the
request is an issue, not a message pasted between sessions.

The issue:

- carries the task identifier of the approved work it belongs to (principle 4);
- names the requesting agent and the agent asked to do the work;
- **points at the text that governs the work** — a dossier section, a contract's
  evidence file, an ADR — and does not restate it. A specification copied into
  an issue is a second specification, and the gates check only the one in the
  repository;
- records every maintainer decision it depends on that no repository document
  yet holds, with its date, so the decision outlives the conversation it was
  made in;
- is referenced by the pull request that does the work, and closes under the
  ticket lifecycle below.

It uses existing labels only: `generation/2` and one `kind/*`.

## Forge operations on one issue

Program rule 5 of
[`REBOOT-EXECUTION-PLAN.md`](REBOOT-EXECUTION-PLAN.md#program-rules) requires a
reviewed dry run and a separate apply approval for every remote change. RFC 0001
requires the same, and its § Rollback adds that **every forge mutation records
its before-state**. How much ceremony that takes depends on the operation.

**One issue, for one approved task** — creating it, commenting on it, adding or
removing a label that already exists, or closing it:

1. **Before-state, read from the live tracker.** Not from memory and not from
   an earlier reading (`DL-5`):

   | Operation | Before-state |
   |---|---|
   | Create | Every issue, open or closed, whose body carries the task identifier — or that there is none |
   | Comment | The issue's number and state. A comment adds and changes nothing, and is never edited or deleted afterwards |
   | Label | The issue's labels before the change |
   | Close | The issue's state and labels before closing |

2. **Dry run.** The agent shows the maintainer the before-state and the exact
   operation: for a new issue, its title, body and labels verbatim; otherwise
   the issue number and the text or change.
3. **Apply approval.** The agent applies only on the maintainer's separate,
   explicit approval of that dry run. Approving the task plan does not approve
   it, and neither does approving a different dry run. A before-state that has
   changed by the time of the apply is a new dry run.
4. **Record, on the issue.** The before-state goes where the mutation lands, so
   the record and the change cannot be separated: a new issue's body ends with
   its before-state and the time it was read; a label change or closure is
   accompanied by a comment stating the before-state and the change. The forge's
   own timeline records the label and state events as well; the comment is what
   states them in words and is not a reading of forge internals. The task's pull
   request names the issue's URL.

**Anything else takes the full flow**: a dry-run artifact pull request, explicit
apply approval, the apply, then a postcondition artifact pull request, with the
before-state in a manifest as R07's and R09's were. That covers any operation
spanning more than one issue, and any change to label definitions, milestones,
repository settings or branch protections — where a mistake reaches many
records at once.

**The lighter path trades one thing, and the trade is stated.** Its dry run is
in the conversation, not in the repository. What it keeps is the two approvals,
a before-state recorded durably on the forge beside the change it describes,
and a public result anyone can inspect. Principle 3 still holds: a mistaken
issue is closed and explained, never deleted, so reversal is a new one-issue
operation taken against the recorded before-state, and the apply approval is the
only point at which a mistake is free.

## Ticket lifecycle

1. Work is accepted — through an RFC, an epic task, or the roadmap promotion gate.
2. An issue is created with its stable task identifier in the body.
3. It is implemented on one branch, in one pull request, per
   [`AGENTS.md`](../../AGENTS.md) §3.
4. It closes when its acceptance criteria are met and the epic checkbox is
   marked, or it closes as superseded with an explanation.
