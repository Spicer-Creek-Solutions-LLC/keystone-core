# AGENTS.md

Operational guidance for AI coding agents in this repository. Keep this file
policy-focused and low-drift.

## 1) Collaboration Contract

- Be direct, concise, and technical.
- Avoid flattery and unnecessary praise.
- Challenge weak assumptions with evidence.
- Prefer correct long-term solutions over quick patches.
- Do not add technical debt.
- Keep code readable and maintainable.
- If a tradeoff exists, present options with evidence and ask for a decision.

## 2) What this repository currently is

Planning, governance and transition evidence. **There is no product code here.**

The Generation 1 implementation was archived at
`archive/2026-09-pre-v0.6-reboot` and removed from the tip by reboot task R08.
Generation 2 code begins at P11, which reintroduces a Go module, a build, and
the gates that go with it.

One developer tool survives, in its own module so it does not depend on a root
module that no longer exists:

- `tools/capcheck` — validates the archive capability catalog. Permanent.

`tools/transition`, which retired the Generation 1 tracker and published the
Generation 2 planning state, was removed at R09. It existed to mutate a tracker
during a one-time transition; what it did is recorded in `docs/transition/`.

Use `make` targets rather than raw tool invocations; `make check` runs every
gate. Commit and push incrementally as meaningful progress is made.

## 3) Non-Negotiable Workflow: Epic Tasks

All work flows through [`epics/20-generation-2-reboot.md`](epics/20-generation-2-reboot.md).

Before starting any epic task, you MUST:

1. Read the epic and the task within it.
2. Consult RFC 0001, accepted ADRs, the product charter, the architecture
   invariants, and the task dossier. Generation 1 sources are research, and live
   in the archive — see §7.
3. Present an implementation plan.
4. Wait for explicit user approval (`yes` or equivalent).
5. Implement only the approved plan.
6. Commit and push.

Rules:

- Do not batch tasks across approvals — one task, one approval, one PR.
- This requirement still applies after context resets/resume.
- If a task uncovers scope outside the epic, stop and ask before expanding.
- Remote forge changes need a reviewed dry run and a **separate** apply
  approval. Approving a task plan never authorises the apply.
- P and C workstreams cannot be approved directly. Each needs a checked-in
  dossier first, and splits into an acceptance-contract task and an
  implementation task with different agents.

## 4) Commit Attribution Requirements

- Follow DCO/sign-off requirements (see [`docs/project/DCO.md`](docs/project/DCO.md)).
- For AI-assisted commits, include required AI disclosure and co-author attribution.
- `Co-Authored-By` must identify the actual agent used for the change.
  - Example for Claude Code: `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>`
  - Example for Codex: `Co-Authored-By: Codex <noreply@openai.com>`

### Commit signing

Author commits are GPG-signed (`git commit -S`). Merge commits created by the
forge are not, and are not expected to be — the forge signs with nothing and the
maintainer merges through the web interface.

This was an unwritten habit until R10. Tasks R02 to R08 were signed, R09 was
not, and nothing detected the lapse because no rule existed to check against.
R09's commits are on `main` and stay unsigned: RFC 0001 commits to history
remaining continuous, which rules out rewriting them.

A convention strong enough that breaking it is a finding is strong enough to
write down.

## 5) Engineering Quality Gates

- Fix bugs encountered in the touched scope immediately.
- If a discovered bug is large/non-trivial, stop and ask for direction.
- Do not add superfluous comments.

### Required tests

Code changes require tests. Generation 2's standard is stricter than Generation
1's and is normative in [`TESTING.md`](docs/project/TESTING.md): a feature is not
complete until a black-box test drives the production CLI or API, crosses the
production transport, executes in the intended agent process, and verifies the
externally observable effect. Package tests support development; they do not
satisfy acceptance.

Generation 1's coverage thresholds do not carry over. P11 sets the gates for
Generation 2 code.

### Required docs

Every change updates the documentation it affects, and every epic task marks its
acceptance criteria when the work lands.

## 6) Project Context (Compact)

Keystone Core is the runtime operations control plane between deployment tooling
(GitOps/IaC) and day-2 operations. Positioning: "GitOps deploys it. We keep it
running."

Generation 2 starts from one narrow promise — secure enrollment, targeted
bounded command execution over NATS, durable lifecycle, cancellation, auditable
result — and adds nothing until that is demonstrably true. Everything Generation
1 had is catalogued as a Future candidate, not a commitment.

Do not duplicate volatile inventories in this file. Those drift quickly.

## 7) Source-of-Truth Index

- Project overview/status: `README.md`
- Accepted reboot decision: `docs/rfcs/0001-generation-2-reboot.md`
- Task sequencing and controls: `docs/project/REBOOT-EXECUTION-PLAN.md`
- Active epic: `epics/20-generation-2-reboot.md`
- Architecture invariants (normative, stable `ARCH-*` ids): `docs/project/ARCHITECTURE-INVARIANTS.md`
- Testing requirements: `docs/project/TESTING.md`
- Requirement-to-evidence register: `docs/project/REQUIREMENTS-TRACEABILITY.md`
- Archive capability catalog (`CAP-*` ids): `docs/project/FUTURE-CAPABILITIES.md`
- Transition evidence — archive manifest, freeze record, tracker retirement: `docs/transition/`
- Reboot assessment evidence: `docs/project/PROJECT-REBOOT-REVIEW.md`
- Why this project exists: `docs/project/PROBLEM-STATEMENT.md`
- Version policy: `docs/project/VERSIONING.md`
- Roadmap (`Now` / `Next` / `Future / Unscheduled` / `Not Planned`): `docs/project/ROADMAP.md`
- Issue tracker conventions: `docs/project/ISSUE-TRACKING.md`
- Governance / DCO / AI policy: `docs/project/{GOVERNANCE,DCO,AI-CONTRIBUTIONS,MAINTAINERS,RFC}.md`
- Security policy and process: `SECURITY.md`, `docs/project/SECURITY-RELEASE.md`
- ADR templates: `docs/adr/`
- Terminology: `docs/project/GLOSSARY.md`
- Development-VM handoff: `docs/project/REBOOT-DEV-VM.md`

### Generation 1 research

Generation 1 sources are not in the working tree. Read them from the archive,
which is pinned and protected:

```bash
git show archive-2026-09-pre-v0.6-reboot:FEATURES.md
git show archive-2026-09-pre-v0.6-reboot:PROJECT-DETAILS.md
git show archive-2026-09-pre-v0.6-reboot:docs/project/ROADMAP.md
git show archive-2026-09-pre-v0.6-reboot:docs/runbooks/README.md
```

They are research, never authority. RFC 0001 and this epic control.

## 8) Maintenance Rule for This File

Keep `AGENTS.md` short, normative, and stable:

- Keep only behavior requirements and source-of-truth pointers.
- Remove historical summaries and versioned inventories.
- Prefer links over duplicated long-form context.
