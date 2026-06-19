# AGENTS.md

Operational guidance for AI coding agents in this repository. Keep this file policy-focused and low-drift.

## 1) Collaboration Contract

- Be direct, concise, and technical.
- Avoid flattery and unnecessary praise.
- Challenge weak assumptions with evidence.
- Prefer correct long-term solutions over quick patches.
- Do not add technical debt.
- Keep code readable and maintainable.
- If a tradeoff exists, present options with evidence and ask for a decision.

## 2) Execution Rules

- Use available skills from `~/.claude/skills/` when task scope clearly matches.
- Once a `Makefile` exists, prefer its targets over raw tool invocations (e.g., `make test` over `go test ./...`). The reboot starts without one — Epic 01 reintroduces it.
- Commit and push incrementally as meaningful progress is made.

## 3) Non-Negotiable Workflow: Epic Tasks

This repo is being reconstructed from scratch toward an eventual v1.0 SemVer-stable release; the current line is `v0.x` (see `docs/project/VERSIONING.md`). All work flows through the epics in `epics/` (start at `epics/00-meta-reconstruction-plan.md`).

Before starting any epic task, you MUST:

1. Read the epic and the task within it (`epics/NN-*.md`).
2. Cross-reference relevant sections in `PROJECT-DETAILS.md` and `FEATURES.md`.
3. Present an implementation plan.
4. Wait for explicit user approval (`yes` or equivalent).
5. Implement only the approved plan.
6. Commit and push.

Rules:

- Do not batch tasks across approvals — one task, one approval, one PR.
- This requirement still applies after context resets/resume.
- If a task uncovers scope outside the epic, stop and ask before expanding.

## 4) Commit Attribution Requirements

- Follow DCO/sign-off requirements (see `docs/project/DCO.md`).
- For AI-assisted commits, include required AI disclosure and co-author attribution.
- `Co-Authored-By` must identify the actual agent used for the change.
  - Example for Claude Code: `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>`
  - Example for Codex: `Co-Authored-By: Codex <noreply@openai.com>`

## 5) Engineering Quality Gates

- Fix bugs encountered in the touched scope immediately.
- If a discovered bug is large/non-trivial, stop and ask for direction.
- Do not add superfluous comments.

### Required tests

All code changes must include tests:

- New functions/methods: normal path, edge cases, error paths.
- New types: constructor/method/interface compliance tests as applicable.
- Bug fixes: add regression tests.
- Follow existing patterns once they emerge (table-driven, `t.TempDir()`, success/error paths).

Coverage targets (per `epics/00-meta-reconstruction-plan.md`):

- Critical packages: >70%
- CLI packages: >40%

### Required docs

Every code change must update the documentation it affects. Until the Hugo site lands (planned for v0.5; see [`docs/project/VERSIONING.md`](docs/project/VERSIONING.md) § v0.5 gate § Documentation), the canonical doc surfaces are:

- `README.md` — project overview / status / quickstart
- `epics/NN-*.md` — mark task acceptance criteria as met when work lands
- `docs/project/*.md` — design, governance, security, glossary

When epic tasks introduce new doc surfaces (CLI reference, API reference, configuration reference), record their location in §7 below.

## 6) Project Context (Compact)

Keystone Core is the runtime operations control plane between deployment tooling (GitOps/IaC) and day-2 operations.

Positioning: "GitOps deploys it. We keep it running."

This repository was reset to a clean reconstruction baseline on 2026-05-05. The prior implementation was substantial but unshippable as a coherent first release; it is not preserved in this repository. The current line starts fresh from the reconstruction baseline — there is no v0 history to import from.

Do not duplicate volatile inventories (epic counts, feature matrices, binary lists) in this file. Those drift quickly.

## 7) Source-of-Truth Index

Use these files instead of expanding AGENTS with mutable detail:

- Project overview/status: `README.md`
- Versioning scheme + v0.5 + v1.0 gates: `docs/project/VERSIONING.md`
- Why this project exists: `docs/project/PROBLEM-STATEMENT.md`
- Feature inventory + version tags: `FEATURES.md`
- State module support matrix (per-param stable/experimental status — the v0.5 "what works / what doesn't" doc): `docs/project/STATE-SUPPORT-MATRIX.md`
- Ranked v0.x backlog (implementation-time deferrals): `docs/project/ROADMAP.md` (update whenever a task narrows scope mid-implementation)
- Implementation reconstruction guide: `PROJECT-DETAILS.md`
- Epic plans: `epics/` (start at `00-meta-reconstruction-plan.md`)
- Issue tracker conventions (labels, milestones, tracker issues, ticket lifecycle): `docs/project/ISSUE-TRACKING.md`
- High-level design: `docs/project/DESIGN.md`
- Policy & audit operator guide (audit-mode-only + enabling-enforcement migration): `docs/project/POLICY-AUDIT.md`
- Governance / DCO / AI policy: `docs/project/{GOVERNANCE,DCO,AI-CONTRIBUTIONS,MAINTAINERS,RFC}.md`
- Security policy: `SECURITY.md`, `docs/project/SECURITY-*.md`
- Release process and signing ceremony: `RELEASE-PLAYBOOK.md`
- Release-incident response (yank / fast follow-up / public comms): `docs/project/RELEASE-INCIDENT.md`
- Operational runbooks (carried from v0; rework in-flight): `docs/runbooks/`
- Go vanity-import static-site source (`go.keystone-core.io` meta tag): `deploy/vanity/` (`vangen.json` + generated `site/`; regen via `make vanity-regen`)
- Documentation-site source (`docs.keystone-core.io`): `deploy/docs/` (v0.1.x: placeholder pointing at the in-repo Markdown docs; full Hugo site lands at gate-v0.5 per ROADMAP)

## 8) Maintenance Rule for This File

Keep `AGENTS.md` short, normative, and stable:

- Keep only behavior requirements and source-of-truth pointers.
- Remove historical summaries and versioned inventories.
- Prefer links over duplicated long-form context.
