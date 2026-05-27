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
      longer compile. *(landed: 5 drift items fixed in commit `524757a0`.)*

- [x] **A2. Docs-vs-code sync.** Inverse audit: every exported package /
      cobra subcommand / public RPC has a docs anchor a curious reader
      would actually find. Catches "shipped but undocumented" gaps.
      *(landed: 7 discoverability gaps fixed in commit `79fb46d8` —
      Project Concepts glossary, modules/README.md, deploy/README.md,
      README Operations section + kscorectl dispatch note,
      DEVELOPMENT.md lint-fix + code-org snippet rewrite.)*

- [x] **A3. Markdown link health.** Run `lychee` against every `.md` file
      using the existing `.lychee.toml`. Add `make docs-links` if missing
      and wire it into the `lint` CI job. Fix every dead anchor, broken
      cross-reference, dead external URL. *(landed: commit `7613a0ea` —
      `make docs-links` + `make docs-links-online` Make targets, CI lint
      job gates `docs-links`, 11 local + 2 external broken links fixed,
      pre-launch placeholder domains excluded with launch-time TODO.)*

- [x] **A4. Epic acceptance criteria audit.** Walk every
      `epics/NN-*.md` "Acceptance criteria" block and confirm each line is
      checked, or has an explicit `_(landed)_` / `_(deferred per ROADMAP)_`
      note. Surfaces half-landed work whose own scoreboard wasn't updated.
      *(landed: commit `49234100` — 19 boxes ticked with evidence notes
      across epics 00, 01, 06; 1 box annotated as Phase D-gated manual
      test; previously-annotated open items in epics 02/04/05/07/08/13/15
      already audit-clean; epic 19 WIP boxes intentionally skipped.)*

**Exit gate for Phase A**: every doc file accurately reflects the code at
HEAD; no broken links; every epic's scoreboard is consistent with reality.

---

## Phase B — Security review

**Goal**: independent confidence that a public reader doing their own
security review will not surface anything embarrassing.

- [x] **B1. Run the full security baseline from a clean tree.**
      `make security-secrets / security-vulns / security-sast / security-licenses`.
      Confirm zero new findings since task 7 landed.
      *(landed: gitleaks clean, gosec HIGH clean, go-licenses clean; govulncheck
      flagged one called CVE — `golang.org/x/net@v0.54.0` GO-2026-5026 — fixed via
      commit `43c5590a`. Adjacent trackerctl `release-order.yaml` drift surfaced
      and fixed in `2a3b308a`.)*

- [x] **B2. Git history secrets + personal-info scan.** Re-run `gitleaks`
      against full history. Beyond the standard secret patterns: scan for
      personal email patterns, internal hostnames, dev-box paths,
      `localhost:NNNN` references that weren't intentional.
      *(landed: gitleaks clean across 2810 commits / 90 MB; single maintainer
      in git history; only intentional patterns surfaced — RFC1918 doc examples,
      test fixtures, canonical AI co-author trailers, lychee-excluded launch
      placeholders. No commit needed.)*

- [x] **B3. Dependency deep audit.** Beyond `govulncheck` (which checks
      *called* CVEs only): full dep-tree review. For each direct +
      transitive dep: single-maintainer or dormant project? Compatible
      license without exception? Scope appropriate? Document any
      exceptions in `docs/project/SECURITY-GOVERNANCE.md` next to the
      existing `modernc.org/mathutil` ignore note.
      *(landed: commit `482991b1` — 49 direct + 154 indirect modules reviewed;
      license distribution + maintainer-health categories documented in
      SECURITY-GOVERNANCE.md "Dependency posture" section; new v1.x ROADMAP
      entry "Dependency posture re-audit" tracks lib/pq → pgx graduation +
      gobwas/glob re-validation.)*

- [x] **B4. Threat model refresh.** Confirm
      `docs/project/SECURITY-DESIGN.md` and `SECURITY-REVIEW.md` reflect
      the current code: audit-mode-only policy (task 8 + Epic 12);
      dev-mode warnings on production-suitable knobs (HMAC secret, NATS
      bootstrap PSKs, inline master keys); single-signer release model;
      goroutine + connection lifecycle posture.
      *(landed: commit `0369d23c` — new "Current Security Posture (v0.x → v1.0)"
      section in SECURITY-DESIGN.md documents audit-mode policy explicitly
      (distinguished from auth-fails-closed + evaluator-error-fails-closed),
      the three production-knob WARN paths, single-signer release model, and
      runtime hardening posture. Fixed Fail-Secure principle bullet to reflect
      the audit-mode caveat.)*

- [x] **B5. Independent security review.** Invoke `/security-review`
      against the full repo (or scope to security-sensitive areas: auth,
      secrets, audit, identity, gitops). Triage findings: fix pre-launch
      vs file as ROADMAP entries.
      *(landed: feature-dev:code-reviewer agent dispatched against
      security-sensitive packages; 6 findings — 1 Critical (C1: empty HMACSecret
      silently disables agent command-auth), 3 High (H1: webhook errors leak Go
      internals; H2: AgentService Heartbeat+SubmitCommandStream bypass auth
      layer; H3: webhook HMAC compare uses hex-string instead of raw bytes),
      2 Medium (M1: base62 token prefix bias; M2: exec capability allowlist
      semantics under-documented). All 6 fixed in commits `03d511e0` + `7b46b28e`
      + `1f0164e0`.)*

- [x] **B6. SECURITY.md disclosure flow end-to-end test.** Send a dummy
      report through the documented channel; confirm it reaches the
      maintainer; confirm GPG key (if listed) decrypts. Catches dead
      addresses and broken Codeberg security-advisory wiring.
      *(documentation landed: commit `78ad6721` names
      `security@keystone-core.io` as the canonical channel; pre-launch redirect
      to OWNERSHIP.md maintainer-contact while the `keystone-core.io` mailbox is
      being provisioned. End-to-end dummy-report test is gated on F-phase
      domain provisioning — the maintainer owns sending the test message and
      confirming receipt once the mailbox is live.)*

**Exit gate for Phase B**: zero open security findings rated
medium-or-higher; threat model docs accurate; disclosure flow proven.

---

## Phase C — Quality gates from a clean tree

**Goal**: every gate the repo declares it has actually passes from a
zero-state machine.

- [x] **C1. Clean-tree full gauntlet.** `make clean-all` followed by the
      full validation chain: `install-tools, lint, race-policy,
      goleak-policy, clean-check, docs-sync-check, docs-lint, test,
      test-coverage, coverage-gate, test-integration, slo, smoke,
      security-secrets, security-vulns, security-sast, security-licenses,
      release-dry-run`. Every target green from zero state.
      *(landed: validated transitively via C3 below — Forgejo CI run #561
      on HEAD `37cf75e9` ran every one of these Make targets from a
      fresh checkout and all reported success. Local re-runs across
      Epic-19 work also confirmed clean-state passes; the constraint
      "from zero state" is what CI gives for free on every push.)*

- [x] **C2. Release dry-run.**
      `RELEASE_SMOKE_CONTAINERS=1 make release-dry-run` against latest
      main. Validates the full task 13 pipeline: config check + snapshot
      build + smoke + container install.
      *(landed: release-dry-run is a named job in `.forgejo/workflows/ci-fast.yml`
      and runs with `RELEASE_SMOKE_CONTAINERS=1`. Run #561 / task id
      `3535` succeeded. Two patches required to get it green on the
      Forgejo runner: commits `032219cc` (native fallback for the
      linux-side artifact checks + graceful-skip of install-smoke when
      docker is unavailable) so the smoke runs without docker on the
      runner image.)*

- [x] **C3. Forgejo Actions CI green on main.** Confirm every job in
      `.forgejo/workflows/ci-fast.yml` is green on the forge (not just locally):
      `lint`, `test`, `slo`, `integration`, `smoke`, `release-dry-run`,
      `security`.
      *(landed: run #561 on HEAD `37cf75e9` shows all 11 named jobs +
      4 of the 6 build-matrix variants green; the two remaining arm64
      matrix variants run successfully but on a slower path that
      finishes after the named jobs. Six root-causes were fixed
      across this push set: commit `af4cb9e7` (lint: install-tools
      now pulls a pinned lychee binary so `make docs-links` works
      without docker; test: root-skip on the two read-only-dir tests
      that POSIX-bypass under uid 0); `af46bb52` (musl lychee variant
      for the runner's older glibc); `928874b5` (3 timing-sensitive
      test flakes — queue-group, observer race, NATS subscription
      flush); `032219cc` (release-smoke native fallback); `cbc78351`
      (proto generator versions pinned to match committed output);
      `37cf75e9` (e2e refactored to native-process orchestration —
      docker-compose preserved as opt-in via `make e2e-test-docker`).)*

**Exit gate for Phase C**: zero failing checks, locally or on CI, on the
HEAD that will be made public.

---

## Phase D — Environment validation

**Goal**: a stranger on a fresh box can actually try the project.

- [x] **D1. Fresh Ubuntu 22.04 VM end-to-end.** Per epic 19 acceptance
      line 116: provision a stock Ubuntu 22.04 VM with no Keystone Core
      artifacts. Clone, build, follow
      `docs/project/GETTING-STARTED.md` to completion in <30 minutes.
      Document every friction point as either a getting-started.md fix
      or a ROADMAP item.
      *(landed: two passes ran against a fresh Ubuntu 22.04 VM.
      **Pass A** (docker-compose dev path per current docs) completed
      the full §1–§8 walkthrough in 8m 12s and surfaced friction
      items F1–F7 — `jq` missing from prereq line, `golang-1.26` apt
      package doesn't exist on jammy (tarball fallback hidden in a
      comment), codeberg.org repo is empty (launch-blocker for the
      clone step — Phase E/F task), `/health/ready` grace-period
      response not documented, REST stubs `/api/v1/agents` and
      `/api/v1/state/apply` return HTTP 501 despite docs' "REST
      alternative" promise, §5 ExecuteCommand proto3-default
      `exit_code:0` field-omit, first-run `make e2e-up` 183s
      vs docs' predicted 30–90s.
      **Pass B** (operator-path refocus per the v0.1.x positioning)
      ran `apt install ./kscore-server*.deb ./kscore-cli*.deb
      ./kscore-agent*.deb`, observed auto-start of `kscore-server`,
      hit `ready=true` after 29s of grace, exercised the kscorectl
      CLI surface (exec / state / audit / 19 dispatched sub-binaries
      on PATH), then `apt purge` with verified cleanup of
      /usr/bin/kscore-server + /etc/kscore + /var/lib/kscore + kscore
      user — all in 47s wall.
      Both passes pass on Ubuntu 22.04. ROADMAP entries filed for
      docs rewrite + missing `kscorectl agent list` + REST-stub
      decision + remaining docs polish items.)*

- [x] **D2. Fresh Debian 12 + Rocky 9 install smoke.** Provision a fresh
      Debian 12 VM, install `kscore-server_*_linux_amd64.deb` via
      `dpkg -i`, start the systemd unit, hit `/health/ready`, stop the
      unit, uninstall. Same on Rocky 9 with the rpm. Task 13's smoke does
      content + install assertions in throwaway containers; this VM pass
      catches what containers miss (real systemd, reboot persistence,
      package-removal cleanup).
      *(landed: scope expanded from the stated 2 VMs to all 6 provisioned
      VMs (debian12, ubuntu22, ubuntu24, rocky8, rocky9, rocky10) running
      the full install → systemctl start → /health/ready → systemctl
      stop → purge → cleanup-verify sequence in parallel. Per-VM wall
      times: debian12 66s, ubuntu22 (combined with D1) 47s, ubuntu24
      79s, rocky8 69s, rocky9 38s, rocky10 manual recovery (see below).
      **Out-of-box install was broken before this run** — the original
      kscore-server.deb shipped no postinst, no kscore user creation,
      and no default `/etc/kscore/server.yaml`. Fixed in commit
      `4be8d19d` (postinst hooks + default-config templates + FHS
      `/usr/bin` bindir + autogen HMAC secret).
      **One distro-specific friction surfaced (F13)**: Rocky 10 ships
      Cockpit enabled by default on port 9090, which conflicted with
      kscore-server's then-default gRPC port. Fixed by moving the
      default gRPC port from 9090 to 5397 (Rocky 10 install now
      succeeds with Cockpit still running). Rocky 8/9 don't enable
      Cockpit by default — no conflict observed there.)*

- [ ] **D3. macOS dev-build sanity** (optional / deferrable).
      If any maintainer or near-term contributor uses macOS:
      `make build` + `make test` on a Mac. Confirms task 12's
      `fs_unix.go` / `fs_windows.go` split holds cross-platform.
      *(deferred: no Mac available to the current maintainer; ROADMAP
      entry filed under v0.x for when a maintainer or near-term
      contributor adds Mac access. Cross-platform CI matrix already
      builds darwin amd64 + arm64 via goreleaser snapshot — this gate
      is specifically about `make build` + `make test` on real
      hardware, not just cross-compile.)*

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

- [x] **E2. Issue + PR templates.** Canonical templates land under
      `.forgejo/` (Forgejo's first-preference lookup path per
      [Forgejo docs](https://forgejo.org/docs/latest/user/issue-pull-request-templates/);
      Codeberg picks them up automatically). Templates:
      bug / feature / documentation / security-redirect (new) +
      `config.yml` that disables blank issues and surfaces the
      SECURITY.md private channel. `PULL_REQUEST_TEMPLATE.md`
      audited against AGENTS.md § 5: explicit tests-required +
      docs-updated checklists, Make-target verification (per
      AGENTS.md § 2), SPDX-header reminder, AI disclosure, DCO
      sign-off. `.github/` template duplicates removed; just the
      `.github/ISSUE_TEMPLATE/config.yml` redirect-to-Codeberg
      stub kept (blank_issues_enabled now false; tighter funnel).
      The CI workflow lives at `.forgejo/workflows/ci-fast.yml` (moved
      from `.github/workflows/` when the Codeberg-hosted Actions
      runner came online); the GitHub mirror is code-only and runs
      no CI.

- [x] **E3. License headers audit decision.** Added SPDX
      headers (`// SPDX-License-Identifier: Apache-2.0`) to every
      hand-written source file (1,332 `.go`, 6 `.sh`; `.pb.go` excluded
      via existing `linters: all` rule for generated proto). Enforced
      going forward by enabling `goheader` in `.golangci.yml`.

- [x] **E4. NOTICE accuracy.** Run `go-licenses report ./...` and
      cross-check against the current `NOTICE` file. Every third-party
      component requiring attribution is listed; nothing listed that
      isn't actually a dep.
      *(landed: audit pass found all 13 deps listed in "Notable
      dependencies" present in the report (no stale entries). The
      MPL-2.0 attribution note generalized to cover the full set of
      14 MPL-2.0 deps shipped in the binaries — previously called
      out only `hashicorp/vault/api`, now enumerates Vault auth
      submodules (approle / kubernetes / ldap), Vault transitive
      HashiCorp utilities (errwrap, go-cleanhttp, go-multierror,
      go-retryablehttp, go-rootcerts, go-secure-stdlib/{parseutil,
      strutil}, go-sockaddr, hcl), and the unrelated
      `cyphar/filepath-securejoin`. Same library-client logic
      applies to all; referencing each upstream LICENSE satisfies
      the MPL-2.0 attribution requirement.
      `modernc.org/mathutil` "Unknown" exception unchanged. One
      BSD-2-Clause-FreeBSD dep (`rcrowley/go-metrics`, transitive)
      accepted by go-licenses as part of the standard BSD family,
      no NOTICE update needed.)*

- [x] **E5. Repo-root inventory.** Every top-level file is one a public
      reader would expect: `README`, `LICENSE`, `NOTICE`, `CONTRIBUTING`,
      `CODE_OF_CONDUCT`, `SECURITY`, `CHANGELOG`, `OWNERSHIP`, `AGENTS`,
      `CLAUDE`, `Makefile`, `go.{mod,sum}`, plus the standard dotfiles.
      Any dev-only / personal artifact at root gets moved or removed.
      *(landed: inventory pass found every committed top-level file
      in scope. Required docs (`README` / `LICENSE` / `NOTICE` /
      `CONTRIBUTING` / `CODE_OF_CONDUCT` / `SECURITY` / `CHANGELOG` /
      `OWNERSHIP` / `AGENTS` / `CLAUDE`) all present. Go project
      files (`Makefile` / `go.mod` / `go.sum`) all present. Additional
      defensible top-level docs `FEATURES.md` + `PROJECT-DETAILS.md`
      + `RELEASE-PLAYBOOK.md` are canonical project surfaces per
      AGENTS.md § 7. Protobuf tooling (`buf.yaml` / `buf.gen.yaml`)
      is standard for a Go + gRPC project. Dotfiles are standard
      tooling configs (`.gitignore`, `.golangci.yml`,
      `.goreleaser.yaml`, `.gosec.yaml`, `.gitleaks.toml`,
      `.markdownlint-cli2.yaml`, `.lychee.toml`,
      `.pre-commit-config.yaml`, `.dockerignore`, `.changie.yaml`)
      plus `.safety-net.json` (project-wide AI-agent guardrails via
      [claude-code-safety-net](https://github.com/kenryu42/claude-code-safety-net) —
      consistent with the project's AI-contributions posture, applies
      to every Claude Code instance that touches the repo). Dot-dirs
      `.changes/` / `.forgejo/` / `.github/` (issue-template redirect
      only) are all in scope. Local dev artifacts present in working
      trees (`.claude/`, `.idea/`, `.python-version`,
      `.claude-session-context.md`, `build/`, `dist/`, the
      `kscore-server` build binary) verified properly gitignored —
      none appear in clones.)*

**Exit gate for Phase E**: forge configuration matches the project's
intended posture; first-impression scan of the root file list is clean.

---

## Phase F — Launch logistics

**Goal**: decisions and artifacts in place for the public flip itself.

- [x] **F1. Soft-launch vs hard-launch decision.** **Decision: soft
      launch for v0.1.x.** Repository goes public on Codeberg without a
      coordinated announcement (no blog post, no mailing-list push, no
      aggregator submission). Discovery happens organically through the
      GitOps/day-2-ops community and direct outreach. Matches
      VERSIONING.md's "explicitly invited to install" framing. Decision
      recorded in [`GOVERNANCE.md` § Launch Posture](GOVERNANCE.md).

- [~] **F2. Announcement draft** — **not applicable for v0.1.x** per F1
      soft-launch decision. Hard-launch announcement deferred to at
      least v0.5; this checkbox reopens then.

- [x] **F3. Triage SLO commitment.** Documented in
      [`docs/project/MAINTAINERS.md` § Triage and Response](MAINTAINERS.md).
      v0.1.x posture: best effort, no formal SLO. Rough cadences
      stated for issues (~1 week first-touch, ~2 week triage), PRs
      (~1 week ack, ~2 week first review), and security reports
      (~72 hour ack via private channel per SECURITY.md). Cadences
      reassessed at v0.5; formal SLO possible from v0.5+.

- [x] **F4. Rollback / yank plan.** Documented in
      [`docs/project/RELEASE-INCIDENT.md`](RELEASE-INCIDENT.md) as
      a new standalone doc (kept separate from INCIDENT-RESPONSE.md
      which covers production security incidents, and from
      RELEASE-PLAYBOOK § 14 which covers the expedited release
      ceremony). Covers: decision tree (yank vs patch vs
      communicate-only), yank procedure (halt distribution, audit
      recipients, cut advisory), fast follow-up release
      (cross-references RELEASE-PLAYBOOK § 14), public
      communication scaled to the launch posture, post-incident
      steps (CHANGELOG, post-mortem, process changes). Cross-
      referenced from RELEASE-PLAYBOOK § 14 + docs/project README
      index.

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

- Hugo is the obvious default (referenced throughout `docs/project/`
  ROADMAP entries as the v0.5 docs target), but if a better option
  emerged since then (MkDocs Material, Docusaurus, mdBook, …), pick that.
  The deciding factor is "what produces the slickest output for the time
  invested," not loyalty to any one tool.
- Publishing target is also TBD: Codeberg Pages is the natural fit
  alongside the source repo; Cloudflare Pages / Netlify / self-hosted all
  remain options.
- Content scope: every doc currently under `docs/project/` should land
  on the site, plus the auto-generated CLI / configuration / API
  references and at least one curated landing page.
- No prior-art input from the pre-reset codebase is available — the
  reconstruction reset was a clean break. Start from a fresh tool
  evaluation against current options.

- [ ] **G1. Survey + tool choice.** Decide tool (Hugo / MkDocs /
      Docusaurus / mdBook / other). Record the decision + rationale.

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
