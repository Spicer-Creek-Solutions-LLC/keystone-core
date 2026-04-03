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

- If a `Makefile` target exists, use it instead of raw tool invocations.
  - Example: use `make test`, not `go test ./...`.
- Use `make build` for compiling binaries (outputs to `build/`), not bare `go build`.
- Use available skills from `~/.claude/skills/` when task scope clearly matches.
- Commit and push incrementally as meaningful progress is made.

## 3) Non-Negotiable Workflow: `TODO.md`

Before fixing any `TODO.md` item, you MUST:

1. Review the TODO item and related code/docs.
2. Present an implementation plan.
3. Wait for explicit user approval (`yes` or equivalent).
4. Implement only the approved plan.
5. Commit and push.

Rules:
- Do not batch-fix TODO items without per-item approval.
- This requirement still applies after context resets/resume.

## 4) Commit Attribution Requirements

- Follow DCO/sign-off requirements (see `docs/project/DCO.md`).
- For AI-assisted commits, include required AI disclosure and co-author attribution.
- `Co-Authored-By` must identify the actual agent used for the change.
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
- Follow existing patterns (table-driven tests, `t.TempDir()`, success/error paths).

Coverage targets:
- Critical packages: >70%
- CLI packages: >40%

### Required docs

All code changes must include corresponding documentation updates:

- CLI changes: update `docs/content/en/docs/reference/cli.md` and related quick-reference docs.
- API changes: update relevant API docs.
- Configuration changes: update `docs/content/en/docs/reference/configuration.md`.
- New features: add user-facing docs with usage/examples.

### State machine standard

Use `pkg/statemachine` for components with complex transitions (typically 3+ states, lifecycle/workflow/retry logic).

Required:
- Document state diagrams with Mermaid.
- Test valid and invalid transitions.
- Use guards/callbacks where appropriate.

Reference: `docs/content/en/docs/contributing/state-machines.md`

## 6) Project Context (Compact)

Keystone Core is the runtime operations control plane between deployment tooling (GitOps/IaC) and day-2 operations.

Positioning: "GitOps deploys it. We keep it running."

Do not duplicate volatile inventories (epic counts, binary totals, feature matrices) in this file. Those drift quickly.

## 7) Source-of-Truth Index

Use these files instead of expanding AGENTS with mutable detail:

- Project overview/status: `README.md`
- Design/architecture: `docs/project/DESIGN.md`
- Epic implementation history/plans: `epics/`
- API reference: `docs/content/en/docs/reference/api.md`
- CLI reference: `docs/content/en/docs/reference/cli.md`
- Configuration reference: `docs/content/en/docs/reference/configuration.md`
- State machine guidance: `docs/content/en/docs/contributing/state-machines.md`
- Development workflow: `docs/project/DEVELOPMENT.md`
- Release process and signing ceremony: `RELEASE-PLAYBOOK.md`

## 8) Maintenance Rule for This File

Keep `AGENTS.md` short, normative, and stable:

- Keep only behavior requirements and source-of-truth pointers.
- Remove historical summaries and versioned inventories.
- Prefer links over duplicated long-form context.
