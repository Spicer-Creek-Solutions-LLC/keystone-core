# Generation 2 Reboot Development Handoff

This runbook resumes the Generation 2 reboot from an already-configured
development VM. It does not cover VM provisioning, tool installation, or
authentication setup.

The active repository task is R01 on `reboot-r01-control-docs`. R01 is stacked
on R00, whose branch is `project-reboot-review`. Do not start R02 until R00 and
R01 have both merged to `main` in the required order.

## Update the R01 working tree

From the existing clone, first confirm that no local work would be overwritten:

```bash
cd /path/to/keystone-core
git status --short --branch
```

If the status shows modified or untracked files, stop and reconcile them before
continuing. Then fetch the remote state and switch to R01:

```bash
git fetch --prune origin
git switch reboot-r01-control-docs
git pull --ff-only origin reboot-r01-control-docs
```

If the local branch does not exist yet, create it from the remote branch instead:

```bash
git switch --track origin/reboot-r01-control-docs
```

Verify that the working tree is clean, the local branch tracks the intended
remote branch, and both resolve to the same commit:

```bash
git status --short --branch
git branch -vv --list reboot-r01-control-docs
git rev-parse HEAD
git rev-parse origin/reboot-r01-control-docs
```

Do not continue if the two commit IDs differ after the fast-forward pull.

## Resume R01 with Codex

Start Codex from the repository root so it discovers the repository-level
`AGENTS.md` instructions:

```bash
codex
```

Use this initial request:

```text
Read AGENTS.md, epics/20-generation-2-reboot.md, the R01 section of
docs/project/REBOOT-EXECUTION-PLAN.md, docs/rfcs/0001-generation-2-reboot.md,
PROJECT-DETAILS.md, and FEATURES.md. We are resuming R01 only on
reboot-r01-control-docs. Inspect the branch and remote state, then present a
plan for any remaining R01 work and wait for my explicit approval before
editing. Do not start R02 or perform remote forge mutations.
```

Approval does not carry across a new agent session. Review the plan and answer
`yes` only if it is limited to R01. One task must still produce one reviewed
branch and one pull request.

## Land R00 and R01

R01 is a stacked change. Preserve this order:

1. Open and review the R00 pull request from `project-reboot-review` to `main`.
2. Open the R01 pull request from `reboot-r01-control-docs` to
   `project-reboot-review` while R00 is still under review.
3. Merge R00 to `main` first.
4. Retarget R01 to the updated `main`, rebasing only if needed.
5. Verify that the retargeted R01 diff contains only R01 changes.
6. Merge R01 to `main` only after its checks and review pass.

Never merge R01 into the R00 branch. Opening or changing pull requests and
other forge state is an external mutation; obtain the approval required by the
execution plan before doing it.

## Start R02 after both predecessors merge

Once Codeberg shows both R00 and R01 merged to `main`, update the VM clone and
create the single-task R02 branch:

```bash
git fetch --prune origin
git switch main
git merge --ff-only origin/main
git switch -c reboot-r02-freeze-reconcile
```

Before any R02 edit or forge mutation, start a fresh Codex task with:

```text
Read AGENTS.md, epics/20-generation-2-reboot.md, the R02 section of
docs/project/REBOOT-EXECUTION-PLAN.md, PROJECT-DETAILS.md, and FEATURES.md.
Confirm that R00 and R01 are present on main. Plan R02 only, separating
read-only inventory, repository changes, and remote forge mutations. Wait for
my explicit approval before implementation. Do not begin R03.
```

R02 includes remote freeze controls. A task-plan approval does not authorize
their immediate application: follow the execution plan's reviewed dry-run and
separate apply-approval requirements.

## Validate and hand off each repository task

Use the repository targets when they exist. For documentation-only R01 changes,
the minimum validation is:

```bash
make docs-lint
make docs-links
git diff --check
```

Review the staged diff before committing. Every commit must include the human
DCO sign-off plus the actual AI disclosure and co-author attribution required
by [`DCO.md`](DCO.md) and [`AI-CONTRIBUTIONS.md`](AI-CONTRIBUTIONS.md). Push the
task branch, open its single pull request, and record any required evidence in
the repository before moving to the next task.
