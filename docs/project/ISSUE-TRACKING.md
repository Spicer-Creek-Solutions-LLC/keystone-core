# Issue tracking

## Current state

The tracker is empty by design.

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
| `kind/epic` | A workstream tracker, not a unit of work |
| `kind/feature`, `kind/bug`, `kind/chore`, `kind/docs` | Work type |
| `area/*` | Subsystem |

Existing labels are never rewritten or deleted. A label already in use carries
meaning a later task did not assign and is not entitled to change.

## Milestones

All five Generation 1 milestones — `gate-v0.5`, `gate-v1.0`, `v0.x`, `v1.x`,
`v2.x+` — are closed. They were closed, never deleted, so the grouping every
archived issue referenced still resolves.

Generation 2 uses a single `v0.6.0` milestone, created in R09. There are no
pre-allocated milestones for later versions: a milestone for work nobody has
committed to is a promise the project cannot keep.

## Automation

There is currently **no automation that writes to the tracker.** The nightly
dependency-freshness job that maintained a tracking issue was removed in R06,
because a nightly writer made reviewed snapshots decay overnight and would have
broken the fail-closed preconditions R07 depends on.

Issue automation returns in R09, and only if it is generation-aware and carries
a negative test proving that closing an archived issue cannot cause a
replacement to be created.

Forge write limits are real and tighter than they look. Comment creation is
limited to roughly 16 per five minutes, and issue creation is tighter still.
Any bulk tracker operation needs a journal, resumability, and a rate-limit
budget — see [`../../tools/transition/README.md`](../../tools/transition/README.md)
for a worked example.

## Ticket lifecycle

1. Work is accepted — through an RFC, an epic task, or the roadmap promotion gate.
2. An issue is created with its stable task identifier in the body.
3. It is implemented on one branch, in one pull request, per
   [`AGENTS.md`](../../AGENTS.md) §3.
4. It closes when its acceptance criteria are met and the epic checkbox is
   marked, or it closes as superseded with an explanation.
