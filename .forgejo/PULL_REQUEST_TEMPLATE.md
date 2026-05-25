<!-- markdownlint-disable MD041 -->
<!--
Thanks for the contribution! Before opening this PR, please confirm
the items in the checklist below. Maintainers won't merge until the
items that apply are checked off — they're not bureaucracy, they're
the bar AGENTS.md sets for all code changes.

(PR templates conventionally have no top-level H1 — the PR title is
the title — so MD041 is disabled at the top of this file.)
-->

## Description

<!--
What does this PR do? Lead with the operator-visible change, then the
mechanism. One short paragraph is usually enough; longer commits
reference the rationale + tradeoffs.
-->

## Related Issue

<!-- Link the issue this PR addresses. "Fixes #N" auto-closes on merge. -->

Fixes #

## Type of Change

<!-- Mark the relevant option(s) with [x] -->

- [ ] Bug fix (non-breaking)
- [ ] New feature (non-breaking)
- [ ] Breaking change (anything operators must change to upgrade)
- [ ] Documentation only
- [ ] Refactor (no functional change)
- [ ] Test only

## Required: tests

Per [`AGENTS.md`](../AGENTS.md) § 5 (Engineering Quality Gates),
**all code changes must include tests**. Mark the cases your PR covers:

- [ ] New functions/methods: normal path, edge cases, error paths
- [ ] New types: constructor / method / interface-compliance tests
- [ ] Bug fix: regression test that fails on `main` and passes here
- [ ] Documentation-only or refactor-only — no new tests needed (justify
      in the description)

Local verification (use Make targets per AGENTS.md § 2):

- [ ] `make test` passes
- [ ] `make test-integration` passes (if your change touches the integration surface)
- [ ] `make lint` passes
- [ ] `make docs-lint` passes (if your change touches Markdown under `docs/`)
- [ ] Coverage targets per [`docs/project/COVERAGE-GATES.md`](../docs/project/COVERAGE-GATES.md)
      met for any new package (critical >70%, CLI >40%)

## Required: docs

Per AGENTS.md § 5, **every code change must update the documentation
it affects**. Confirm the surfaces touched:

- [ ] `README.md` — if operator-visible quickstart / topology changed
- [ ] `epics/NN-*.md` — acceptance criteria marked as met when work lands (if epic-task)
- [ ] `docs/project/*.md` — design, governance, security, glossary, reference
- [ ] `docs/project/{CLI,CONFIGURATION,API}-REFERENCE.md` — regenerated via `make docs-sync`
      if CLI flags / config keys / proto-defined RPCs / OpenAPI endpoints changed
- [ ] `docs/runbooks/*.md` — if operational procedures shifted
- [ ] `CHANGELOG.md` — `[Unreleased]` section updated
- [ ] No doc changes needed (justify in the description)

## Required: SPDX header on new Go / shell files

The `goheader` linter enforces `// SPDX-License-Identifier: Apache-2.0`
on the first line of every new `.go` file (and `# SPDX-License-Identifier:
Apache-2.0` after the shebang on `.sh`).

- [ ] New `.go` / `.sh` files carry the SPDX header (or this PR adds no new files)

## Required: AI assistance disclosure

Per [`docs/project/AI-CONTRIBUTIONS.md`](../docs/project/AI-CONTRIBUTIONS.md),
AI-assisted contributions are welcome and disclosure is required:

- [ ] This PR contains NO AI-generated or AI-assisted code
- [ ] This PR contains AI-generated or AI-assisted code

**If AI was used**:

- [ ] AI tool(s) used (e.g., Claude, Codex, Copilot, GPT-4):
- [ ] Commit messages carry `Co-Authored-By:` identifying the actual agent
      (per AGENTS.md § 4 — the agent identity is part of the audit trail)
- [ ] I have reviewed, tested, and understand all AI-generated code in this PR
- [ ] I take responsibility for this contribution as if I authored it myself

## Required: DCO sign-off

Per [`docs/project/DCO.md`](../docs/project/DCO.md), all commits in this
PR must carry a `Signed-off-by:` trailer (added by `git commit -s`).

- [ ] Every commit has a DCO sign-off trailer

## Additional Notes

<!--
Anything reviewers should know: deferred follow-ups (filed where?),
behavior changes worth highlighting, known limitations, things you
considered and decided against.
-->
