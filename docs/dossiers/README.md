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
   demonstrated failing before it is accepted, and **the record lands with the
   work** as `<TASK>-acceptance-evidence.md` — in the pull request that performs
   the task, never in the one that lands the dossier
   ([`REBOOT-EXECUTION-PLAN.md`](../project/REBOOT-EXECUTION-PLAN.md)
   § "Required task dossier", which is the authority). A dossier is a boundary
   and ships alone: its cases are about a document the task has not written yet,
   so there is nothing to demonstrate when it lands.
6. **Validation commands** — what is run, and what it actually covers.
7. **Documentation changes**.
8. **Security reviewer** — named, or an argued statement that none is required.
9. **Rollback**.
10. **Handoff artifacts** — what later tasks consume from this one.

## Naming

One file per **workstream**, `docs/dossiers/<WORKSTREAM>.md` — `P00.md`,
`C01.md`. The identifier matches the tracker issue's `keystone-core-task`
marker, never a title, per
[`ISSUE-TRACKING.md`](../project/ISSUE-TRACKING.md) § Principles.

**A split behaviour workstream shares one dossier.** `Cxx-A` and `Cxx-I` are two
tasks — separate agents, separate pull requests, separate
`<TASK>-acceptance-evidence.md` — and one dossier, `Cxx.md`, whose § 3 states
each half's outputs separately.

This said *one file per task* and gave `C05-A.md` as the filename example, which
[`REBOOT-EXECUTION-PLAN.md`](../project/REBOOT-EXECUTION-PLAN.md) § "Required
task dossier" repeated independently. C01 is where the ambiguity surfaced and
G26 is where both were corrected. **`C05-A` is still a task identifier** and
`C05-A-acceptance-evidence.md` is still its evidence file; neither is a dossier
name.

## Index

| Task | Dossier | State |
|---|---|---|
| P00 — Product charter and canonical journeys | [P00.md](P00.md) | Complete — charter accepted |
| P01 — Threat model | [P01.md](P01.md) | Complete — threat model accepted |
| P02 — NATS-native capability ADR | [P02.md](P02.md) | Complete — `ADR-0002` proposed |
| P03 — Enrollment and identity ADR | [P03.md](P03.md) | Complete — `ADR-0003` proposed |
| P04 — Subject authorization ADR and executable policy | [P04.md](P04.md) | Complete — `ADR-0004` proposed |
| P05 — Versioned encrypted protocol ADR | [P05.md](P05.md) | Complete — `ADR-0005` proposed |
| P06 — Delivery and job lifecycle ADR | [P06.md](P06.md) | Complete — `ADR-0006` proposed |
| P07 — Safe execution ADR | [P07.md](P07.md) | Complete — `ADR-0007` proposed |
| P08 — Persistence and audit ADR | [P08.md](P08.md) | Complete — `ADR-0008` proposed |
| P09 — Local operator API and authorization ADR | [P09.md](P09.md) | Complete — `ADR-0009` proposed |
| P10 — Acceptance-harness design | [P10.md](P10.md) | Complete — `ADR-0010` proposed |
| P11 — Repository skeleton and CI | [P11.md](P11.md) | Complete — P11a and P11b merged |
| C01 — Protocol types and cryptographic vectors | [C01.md](C01.md) | **Complete** — C01-A's contract and C01-I's implementation both merged |
| C02 — SQLite migrations and durable ledgers | [C02.md](C02.md) | **Complete** — C02-A and C02-I landed |
| C03 — NATS operator, account, identity and permission generation | [C03.md](C03.md) | **Complete** — C03-A's forty-five cases and C03-I's six stages landed |
| C04 — Local operator control substrate | [C04.md](C04.md) | **Complete** — C04-A contract and C04-I implementation landed |
| C05 — One-use enrollment vertical slice | [C05.md](C05.md) | Dossier landed — `D-C05-1` and `D-C05-9` settled by RFC 0005 (`G48`); **blocked on `D-C05-8`, TLS (`G49`)**; C05-A and C05-I not started |
