# Task dossiers

A dossier is the artifact that turns a workstream paragraph into implementation
authority. [`REBOOT-EXECUTION-PLAN.md`](../project/REBOOT-EXECUTION-PLAN.md)
§ "Required task dossier" is normative:

> No P/C workstream may receive implementation approval until a checked-in
> dossier defines its `Depends on` task IDs and commit SHAs, allowed paths,
> exact outputs, applicable requirement IDs, acceptance cases and N/A
> rationale, validation commands, documentation changes, security reviewer,
> rollback, and handoff artifacts.

A dossier is not a plan and not an approval. It is the boundary a plan is
judged against. Landing one authorizes nothing by itself: the workstream still
needs its own presented plan and its own explicit approval, and behaviour
workstreams still split into `Cxx-A` and `Cxx-I` under separate approvals.

## Which tasks need one

| Task kind | Dossier |
|---|---|
| `Pxx` and `Cxx` workstreams | Required before any implementation approval |
| `Rxx` repository-transition tasks | Not used; the R stage closed at R10 |
| `Gxx` supporting tasks | Not required; cases and limitations go in the pull request |
| Control-document tasks | Not required; the amendment records the change |

**A dossier is not the approval, and neither is the artifact a task produces.**
Program rule 1 and [`AGENTS.md`](../../AGENTS.md) § 3 require a presented plan
and an explicit maintainer approval for *every* repository task. A task that
needs no dossier still needs that approval; "no dossier" narrows what must be
written down beforehand, never who has to agree to the work.

## Required fields

Every dossier carries these ten, in this order, whether or not each has
content. A field with nothing to say says so and gives the reason.

1. **Depends on** — task IDs *and* commit SHAs.
2. **Allowed paths** — what the task may create, modify and remove. Everything
   else is out of bounds.
3. **Exact outputs** — the artifacts, named.
4. **Requirement IDs** — which `ARCH-*` identifiers apply, and whether the task
   satisfies or merely registers them.
5. **Acceptance cases** — the [`TESTING.md`](../project/TESTING.md) feature
   table, each case marked applicable or `N/A` with a per-case reason; the
   task's own cases; and, for each, what it **cannot** detect. Every case is
   demonstrated failing before it is accepted, recorded in
   `<TASK>-acceptance-evidence.md` alongside the dossier.
6. **Validation commands** — what is run, and what it actually covers.
7. **Documentation changes**.
8. **Security reviewer** — named, or an argued statement that none is required.
9. **Rollback**.
10. **Handoff artifacts** — what later tasks consume from this one.

## Naming

One file per task, `docs/dossiers/<TASK>.md` — `P00.md`, `C05-A.md`. The task
identifier matches the tracker issue's `keystone-core-task` marker, never a
title, per [`ISSUE-TRACKING.md`](../project/ISSUE-TRACKING.md) § Principles.

## Index

| Task | Dossier | State |
|---|---|---|
| P00 — Product charter and canonical journeys | [P00.md](P00.md) | Complete — charter accepted |
| P01 — Threat model | [P01.md](P01.md) | Complete — threat model accepted |
| P02 — NATS-native capability ADR | [P02.md](P02.md) | Complete — `ADR-0002` proposed |
| P03 — Enrollment and identity ADR | [P03.md](P03.md) | Complete — `ADR-0003` proposed |
| P04 — Subject authorization ADR and executable policy | [P04.md](P04.md) | Complete — `ADR-0004` proposed |
| P05 — Versioned encrypted protocol ADR | [P05.md](P05.md) | Complete — `ADR-0005` proposed |
