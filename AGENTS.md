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
- Once a `Makefile` exists, prefer its targets over raw tool invocations (e.g., `make test` over `go test ./...`). The clean baseline starts without one — task P11 reintroduces it.
- Commit and push incrementally as meaningful progress is made.

## 3) Non-Negotiable Workflow: Epic Tasks

This repo is transitioning from the Generation 1 implementation to the accepted
Generation 2 reboot. All new work flows through
`epics/20-generation-2-reboot.md`; Epics 01–19 are Generation 1 evidence and do
not authorize implementation. See `docs/rfcs/0001-generation-2-reboot.md` and
`docs/project/REBOOT-EXECUTION-PLAN.md`.

Before starting any epic task, you MUST:

1. Read the epic and the task within it (`epics/NN-*.md`).
2. For R-stage work, cross-reference relevant Generation 1 sections in
   `PROJECT-DETAILS.md` and `FEATURES.md`. For P/C-stage work, use RFC 0001,
   accepted ADRs, the product charter, architecture invariants, and the task
   dossier. Consult commit-pinned archive sources only when historical research
   is relevant.
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

The Generation 1 implementation is still present until task R08 lands the clean
baseline. Do not treat its implemented breadth as Generation 2 scope. Tasks
R02–R10 preserve the old line, transition the tracker and forge safely, and
establish the clean baseline without rewriting history.

Do not duplicate volatile inventories (epic counts, feature matrices, binary lists) in this file. Those drift quickly.

## 7) Source-of-Truth Index

Use these files instead of expanding AGENTS with mutable detail:

- Project overview/status: `README.md`
- Reboot assessment evidence: `docs/project/PROJECT-REBOOT-REVIEW.md`
- Accepted reboot decision: `docs/rfcs/0001-generation-2-reboot.md`
- Reboot tasks and sequencing: `docs/project/REBOOT-EXECUTION-PLAN.md`
- Reboot development-VM handoff: `docs/project/REBOOT-DEV-VM.md`
- Generation 2 invariants/tests: `docs/project/{ARCHITECTURE-INVARIANTS,TESTING,REQUIREMENTS-TRACEABILITY}.md`
- Uncommitted archive capability catalog: `docs/project/FUTURE-CAPABILITIES.md`
- Active version policy plus retained Generation 1 gates: `docs/project/VERSIONING.md`
- Why this project exists: `docs/project/PROBLEM-STATEMENT.md`
- Generation 1 feature inventory pending archive: `FEATURES.md`
- Generation 1 state support evidence pending archive: `docs/project/STATE-SUPPORT-MATRIX.md`
- Frozen Generation 1 backlog and transition notice: `docs/project/ROADMAP.md`
- Generation 1 implementation evidence pending archive: `PROJECT-DETAILS.md`
- Active epic: `epics/20-generation-2-reboot.md`; Epics 00–19 are Generation 1
  evidence pending archive.
- Issue tracker conventions (labels, milestones, tracker issues, ticket lifecycle): `docs/project/ISSUE-TRACKING.md`
- Generation 1 tracker retirement tool (R04/R07, its own Go module): `tools/transition/README.md`
- High-level design: `docs/project/DESIGN.md`
- Policy & audit operator guide (audit-mode-only + enabling-enforcement migration): `docs/project/POLICY-AUDIT.md`
- Governance / DCO / AI policy: `docs/project/{GOVERNANCE,DCO,AI-CONTRIBUTIONS,MAINTAINERS,RFC}.md`
- Security policy: `SECURITY.md`, `docs/project/SECURITY-*.md`
- Release process and signing ceremony: `RELEASE-PLAYBOOK.md`
- Release-incident response (yank / fast follow-up / public comms): `docs/project/RELEASE-INCIDENT.md`
- Operational runbooks (carried from v0; rework in-flight): `docs/runbooks/`
- Go vanity-import static-site source (`go.keystone-core.io` meta tag): `deploy/vanity/` (`vangen.json` + generated `site/`; regen via `make vanity-regen`)
- Promo-video pipeline (script + shot list + generated cards): `assets/promo/` (`make update-promo` regenerates from the branch, `make promo-check` gates it in CI, `make promo` renders; tag a changelog fragment with `Demo:` to flag it for a shot)
- Documentation-site source (`docs.keystone-core.io`): `deploy/docs/` (v0.1.x: placeholder pointing at the in-repo Markdown docs; full Hugo site lands at gate-v0.5 per ROADMAP)

## 8) Maintenance Rule for This File

Keep `AGENTS.md` short, normative, and stable:

- Keep only behavior requirements and source-of-truth pointers.
- Remove historical summaries and versioned inventories.
- Prefer links over duplicated long-form context.
