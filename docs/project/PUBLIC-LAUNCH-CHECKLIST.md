# Public Launch Checklist

A one-time, sequential checklist for flipping the Keystone Core repository
from private to public. Distinct from the per-release [`RELEASE-PLAYBOOK.md`](../../RELEASE-PLAYBOOK.md) — this is about repository readiness, not release ceremony.

This checklist is **strictly sequential**: each phase finishes before the
next begins (`A → B → C → D → E → F → G`). Phase B reads what Phase A
produced; Phase D exercises what Phase C declared green; Phase G builds
on the doc fixes from Phase A; etc. Skipping forward defeats the gating.

When every box below is checked, the repository is ready to flip to public.

---

## Phase A — Internal consistency

**Goal**: every claim the repository makes about itself is true at the
moment of public exposure.

- [x] **A1. Code-vs-docs sync sweep.** Walk every claim in `README.md`,
      `docs/project/*.md`, and the `_(landed)_` notes in `epics/NN-*.md`
      and verify the named code path exists with the described behavior.
      The auto-generated CLI / config / API references are gated by
      `make docs-sync-check`; the **prose** docs are not.
      *Watch for*: renamed functions/files; behavior scoped down "later";
      deferred items still phrased in the present tense; examples that no
      longer compile. _(landed: 5 drift items fixed in commit `524757a0`.)_

- [x] **A2. Docs-vs-code sync.** Inverse audit: every exported package /
      cobra subcommand / public RPC has a docs anchor a curious reader
      would actually find. Catches "shipped but undocumented" gaps.
      _(landed: 7 discoverability gaps fixed in commit `79fb46d8` —
      Project Concepts glossary, modules/README.md, deploy/README.md,
      README Operations section + kscorectl dispatch note,
      DEVELOPMENT.md lint-fix + code-org snippet rewrite.)_

- [x] **A3. Markdown link health.** Run `lychee` against every `.md` file
      using the existing `.lychee.toml`. Add `make docs-links` if missing
      and wire it into the `lint` CI job. Fix every dead anchor, broken
      cross-reference, dead external URL. _(landed: commit `7613a0ea` —
      `make docs-links` + `make docs-links-online` Make targets, CI lint
      job gates `docs-links`, 11 local + 2 external broken links fixed,
      pre-launch placeholder domains excluded with launch-time TODO.)_

- [x] **A4. Epic acceptance criteria audit.** Walk every
      `epics/NN-*.md` "Acceptance criteria" block and confirm each line is
      checked, or has an explicit `_(landed)_` / `_(deferred per ROADMAP)_`
      note. Surfaces half-landed work whose own scoreboard wasn't updated.
      _(landed: commit `49234100` — 19 boxes ticked with evidence notes
      across epics 00, 01, 06; 1 box annotated as Phase D-gated manual
      test; previously-annotated open items in epics 02/04/05/07/08/13/15
      already audit-clean; epic 19 WIP boxes intentionally skipped.)_

**Exit gate for Phase A**: every doc file accurately reflects the code at
HEAD; no broken links; every epic's scoreboard is consistent with reality.

---

## Phase B — Security review

**Goal**: independent confidence that a public reader doing their own
security review will not surface anything embarrassing.

- [ ] **B1. Run the full security baseline from a clean tree.**
      `make security-secrets / security-vulns / security-sast / security-licenses`.
      Confirm zero new findings since task 7 landed.

- [ ] **B2. Git history secrets + personal-info scan.** Re-run `gitleaks`
      against full history. Beyond the standard secret patterns: scan for
      personal email patterns, internal hostnames, dev-box paths,
      `localhost:NNNN` references that weren't intentional.

- [ ] **B3. Dependency deep audit.** Beyond `govulncheck` (which checks
      *called* CVEs only): full dep-tree review. For each direct +
      transitive dep: single-maintainer or dormant project? Compatible
      license without exception? Scope appropriate? Document any
      exceptions in `docs/project/SECURITY-GOVERNANCE.md` next to the
      existing `modernc.org/mathutil` ignore note.

- [ ] **B4. Threat model refresh.** Confirm
      `docs/project/SECURITY-DESIGN.md` and `SECURITY-REVIEW.md` reflect
      the current code: audit-mode-only policy (task 8 + Epic 12);
      dev-mode warnings on production-suitable knobs (HMAC secret, NATS
      bootstrap PSKs, inline master keys); single-signer release model;
      goroutine + connection lifecycle posture.

- [ ] **B5. Independent security review.** Invoke `/security-review`
      against the full repo (or scope to security-sensitive areas: auth,
      secrets, audit, identity, gitops). Triage findings: fix pre-launch
      vs file as ROADMAP entries.

- [ ] **B6. SECURITY.md disclosure flow end-to-end test.** Send a dummy
      report through the documented channel; confirm it reaches the
      maintainer; confirm GPG key (if listed) decrypts. Catches dead
      addresses and broken Codeberg security-advisory wiring.

**Exit gate for Phase B**: zero open security findings rated
medium-or-higher; threat model docs accurate; disclosure flow proven.

---

## Phase C — Quality gates from a clean tree

**Goal**: every gate the repo declares it has actually passes from a
zero-state machine.

- [ ] **C1. Clean-tree full gauntlet.** `make clean-all` followed by the
      full validation chain: `install-tools, lint, race-policy,
      goleak-policy, clean-check, docs-sync-check, docs-lint, test,
      test-coverage, coverage-gate, test-integration, slo, smoke,
      security-secrets, security-vulns, security-sast, security-licenses,
      release-dry-run`. Every target green from zero state.

- [ ] **C2. Release dry-run.**
      `RELEASE_SMOKE_CONTAINERS=1 make release-dry-run` against latest
      main. Validates the full task 13 pipeline: config check + snapshot
      build + smoke + container install.

- [ ] **C3. Forgejo Actions CI green on main.** Confirm every job in
      `.github/workflows/ci.yml` is green on the forge (not just locally):
      `lint`, `test`, `slo`, `integration`, `smoke`, `release-dry-run`,
      `security`.

**Exit gate for Phase C**: zero failing checks, locally or on CI, on the
HEAD that will be made public.

---

## Phase D — Environment validation

**Goal**: a stranger on a fresh box can actually try the project.

- [ ] **D1. Fresh Ubuntu 22.04 VM end-to-end.** Per epic 19 acceptance
      line 116: provision a stock Ubuntu 22.04 VM with no Keystone Core
      artifacts. Clone, build, follow
      `docs/project/GETTING-STARTED.md` to completion in <30 minutes.
      Document every friction point as either a getting-started.md fix
      or a ROADMAP item.

- [ ] **D2. Fresh Debian 12 + Rocky 9 install smoke.** Provision a fresh
      Debian 12 VM, install `kscore-server_*_linux_amd64.deb` via
      `dpkg -i`, start the systemd unit, hit `/health/ready`, stop the
      unit, uninstall. Same on Rocky 9 with the rpm. Task 13's smoke does
      content + install assertions in throwaway containers; this VM pass
      catches what containers miss (real systemd, reboot persistence,
      package-removal cleanup).

- [ ] **D3. macOS dev-build sanity** (optional / deferrable).
      If any maintainer or near-term contributor uses macOS:
      `make build` + `make test` on a Mac. Confirms task 12's
      `fs_unix.go` / `fs_windows.go` split holds cross-platform.

**Exit gate for Phase D**: getting-started works on stock Ubuntu;
.deb/.rpm install + uninstall clean on real systemd.

---

## Phase E — Repository hygiene + Codeberg-side configuration

**Goal**: the Codeberg project page reads like a project, not a sandbox.

- [ ] **E1. Codeberg repo settings.** Branch protection on `main`
      (signed-commit requirement; DCO check; status-check requirement).
      Default branch confirmed as `main`. Topics, description, website
      fields set. Disable any unused features (wiki, packages) if not
      planned for v0.1.x.

- [ ] **E2. Issue + PR templates.** `.gitea/ISSUE_TEMPLATE/`: bug,
      feature, security. `.gitea/PULL_REQUEST_TEMPLATE.md` enforcing DCO
      sign-off + AI disclosure (per `docs/project/AI-CONTRIBUTIONS.md`)
      + tests required.

- [ ] **E3. License headers audit decision.** Decide whether to add SPDX
      headers (`// SPDX-License-Identifier: Apache-2.0`) to every source
      file. If yes: one bulk commit via script. If no: document the
      decision in `LICENSE` or `CONTRIBUTING.md`.

- [ ] **E4. NOTICE accuracy.** Run `go-licenses report ./...` and
      cross-check against the current `NOTICE` file. Every third-party
      component requiring attribution is listed; nothing listed that
      isn't actually a dep.

- [ ] **E5. Repo-root inventory.** Every top-level file is one a public
      reader would expect: `README`, `LICENSE`, `NOTICE`, `CONTRIBUTING`,
      `CODE_OF_CONDUCT`, `SECURITY`, `CHANGELOG`, `OWNERSHIP`, `AGENTS`,
      `CLAUDE`, `Makefile`, `go.{mod,sum}`, plus the standard dotfiles.
      Any dev-only / personal artifact at root gets moved or removed.

**Exit gate for Phase E**: forge configuration matches the project's
intended posture; first-impression scan of the root file list is clean.

---

## Phase F — Launch logistics

**Goal**: decisions and artifacts in place for the public flip itself.

- [ ] **F1. Soft-launch vs hard-launch decision.** Quietly flip-to-public
      and let discovery happen organically, or actively announce?
      VERSIONING.md's v0.1.x framing ("explicitly invited to install")
      implies invitation; level of invitation is the open question.

- [ ] **F2. Announcement draft** (if hard-launch). Short post covering:
      what is it, what works, what doesn't, how to try, where to file
      feedback, what we are explicitly *not* committing to yet. Location
      decided per F1.

- [ ] **F3. Triage SLO commitment.** Document expected response time for
      issues, PRs, and security reports. Even "best effort, no SLO" is
      fine — but it should be stated explicitly. Lands in
      `docs/project/GOVERNANCE.md` or `MAINTAINERS.md`.

- [ ] **F4. Rollback / yank plan.** Procedure if the first hour of public
      exposure reveals a critical bug. Cover: how to pull the latest tag;
      how to push a fast v0.1.0-rc2; how to communicate the incident.
      Lands in `docs/project/INCIDENT-RESPONSE.md` or alongside the
      RELEASE-PLAYBOOK.

**Exit gate for Phase F**: every launch-day question has a documented
answer. Repo is ready to flip.

---

## Phase G — Documentation site

**Goal**: a slick, browseable documentation site that a curious operator
or contributor can land on and immediately get oriented. Markdown in
`docs/project/` is enough for GitHub-style rendering but not enough for
the kind of project landing-page a serious project needs.

Details are deferred until this phase starts — pick the right tool then,
not now. Worth knowing going in:

- The `archive/v0` branch has a prior Hugo-based doc-site attempt; review
  it for content structure, theme choices, what worked, what didn't.
- Hugo is the obvious default (referenced throughout `docs/project/`
  ROADMAP entries as the post-v1.0 docs target), but if a better option
  emerged since then (MkDocs Material, Docusaurus, mdBook, …), pick that.
  The deciding factor is "what produces the slickest output for the time
  invested," not loyalty to any one tool.
- Publishing target is also TBD: Codeberg Pages is the natural fit
  alongside the source repo; Cloudflare Pages / Netlify / self-hosted all
  remain options.
- Content scope: every doc currently under `docs/project/` should land
  on the site, plus the auto-generated CLI / configuration / API
  references and at least one curated landing page.

- [ ] **G1. Survey + tool choice.** Read the `archive/v0` doc-site
      attempt. Decide tool (Hugo / MkDocs / Docusaurus / mdBook / other).
      Record the decision + rationale.

- [ ] **G2. Content structure.** Lay out the site information
      architecture: landing page, getting-started, architecture, CLI
      reference, configuration reference, API reference, operations,
      contributing, security, release notes.

- [ ] **G3. Publishing pipeline.** Set up the build + deploy flow. Local
      `make docs-site` to preview; CI job builds on PR; merge to `main`
      deploys to production URL.

- [ ] **G4. Live + linked.** Site is live at its production URL; README
      and `docs/project/` README-equivalents link to it; auto-generated
      references regenerate as part of the deploy (no drift from
      `make docs-sync-check`).

**Exit gate for Phase G**: a public reader landing on the project
discovers a polished site, not a wall of GitHub-rendered markdown.

---

## After all phases

When every box above is checked:

1. Final `git log` review on `main` — last commit's content is the public-
   facing HEAD. No surprises.
2. Codeberg setting: repository visibility → **public**.
3. Begin task 14 (v0.1.0-rc1) per `epics/19-test-harden-release.md`.

This checklist persists in-repo so the next pre-launch event (e.g., if a
major rewrite ever forces a re-launch posture) can re-run it.
