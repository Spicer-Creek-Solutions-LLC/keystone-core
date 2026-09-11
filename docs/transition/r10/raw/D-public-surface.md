# R10 audit — slice D: public surface and documented claims

**Summary.** The Git-side preservation evidence holds up under independent check —
archive refs, tag object IDs, the forge-verified tag signature, the offline bundle
digest, every recorded artifact hash, and every counted figure I could recompute.
The *public surface* does not. `docs.keystone-core.io` still serves the complete
Generation 1 documentation site, including install instructions and a CLI reference
for twenty `kscore-*` binaries, with no mention of the reboot; both forges still
advertise Generation 1's capability list and "v0.x pre-stable" in the repository
description; and the live issue-report form asks contributors for
`kscorectl --version` and their `.deb`/`.rpm` install path. Several tracked
documents also state things that are now false — the tracker is "empty by design"
(it holds fourteen issues), the announcement is "pinned" (it is not, and R09's own
apply log records the 403). The transition record itself is mostly honest and in
places unusually careful, but three of its entries have gone stale and one
integrity digest cannot be reproduced without recovering a deleted tool from
history.

Counts: 1 Critical, 8 High, 13 Medium, 7 Low, plus verified-clean evidence and an
explicit list of claims I could not check.

---

## Method

All checks run 2026-09-11 from the development VM, against
`reboot-r10-transition-audit` at `3a378779a` (one commit ahead of `main` at
`7ca552231`). Forge reads used `FORGEJO_TOKEN` against
`https://codeberg.org/api/v1` (non-admin; `branch_protections`, `tag_protections`
and `hooks` all return 403). No mutating request was made. Nothing outside
`docs/transition/r10/raw/` was modified.

What I did:

1. **Live HTTP.** Fetched `docs.keystone-core.io` (root, sitemap, search index and
   six sub-pages), `go.keystone-core.io/keystone-core`, the Codeberg repo and the
   GitHub mirror, and byte-compared the served pages against the repository's
   `deploy/` sources.
2. **Link resolution.** Extracted every `http(s)` URL from all 73 tracked files
   with a Python scanner (96 unique), then resolved each with `curl -L`. Separately
   extracted and resolved every URL in the fourteen R09 issue bodies (8 unique) and
   in both annotated release bodies (8 unique).
3. **Transition record.** Recomputed every SHA-256 the manifest records; walked the
   Git history of each artifact to see whether a recorded digest ever matched;
   recovered the deleted `tools/transition/allowlist.go` from `b874f7223^` to
   reproduce the one digest that did not.
4. **Counts.** Independently recomputed `CAP-*` (703), `ARCH-*` (24), P/C/R stage
   IDs, Generation 1 roadmap entries (163), R08's and R09's file arithmetic, the
   R07 allowlist composition (106 = 103 leaf + 3 tracker), and the tracker census
   (120 closed + 14 open = 134).
5. **Forge state.** Read the repo object, topics, releases, release assets,
   milestones, labels, all fourteen R09 issues, the pinned-issues list, both
   archive refs and all four tags on Codeberg; and the branches, tags, description,
   topics and `pushed_at` on the GitHub mirror.
6. **Doc cross-reading.** Read in full: `README.md`, `SECURITY.md`, `CHANGELOG.md`,
   `CONTRIBUTING.md`, `NOTICE`, `OWNERSHIP.md`, `AGENTS.md`, `Makefile`,
   `.lychee.toml`, all five issue templates, the PR template, the CI workflow,
   `docs/README.md`, `docs/project/{README,ROADMAP,VERSIONING,ISSUE-TRACKING}.md`,
   `docs/rfcs/0001`, `docs/transition/FREEZE.md`, `epics/20`, `deploy/*/README.md`,
   and sampled `SECURITY-GOVERNANCE.md`, `SECURITY-RELEASE.md`, `GOVERNANCE.md`,
   `MAINTAINERS.md`, `FUTURE-CAPABILITIES.md`.
7. **Gate behaviour.** Ran the repository's own link gate to establish what it
   actually checks.

Key raw output is quoted inline with each finding.

---

## Findings

### D-01 — CRITICAL — `docs.keystone-core.io` still serves the entire Generation 1 documentation site

**Claim** (`deploy/docs/README.md`):

> `site/index.html` is the served page. Reboot task R09 rewrote it into the reboot
> announcement: it states that there is nothing to install, why the project
> restarted, where Generation 1 was preserved, and what Generation 2 promises.

**Claim** (`deploy/docs/site/index.html`, the page the repo believes is live):

> **There is nothing to install.** […] There is no rendered site: the Hugo
> toolchain was part of Generation 1 and was removed with it.

**What I found.** The live host serves a 111,515-byte Hugo 0.163.3 page, not the
5,773-byte announcement:

```
live sha256: 712bb8e248220c723821e80f8f34b50c3fdf56447664dadea9d964b0756fc2a8  (111515 bytes)
repo sha256: a45abe81d6a51a0004131e725abc8594aae5c34e567c180d3f39b0bd0884a4c9  (  5773 bytes)
<meta name="generator" content="Hugo 0.163.3">
```

The site is complete and current-tense. Its sitemap lists 100 URLs, none of which
mentions the reboot. Spot checks:

| URL | Status | What it serves |
|---|---|---|
| `https://docs.keystone-core.io/docs/getting-started/` | 200 | "runnable on a fresh Ubuntu host **with Keystone Core installed**" |
| `https://docs.keystone-core.io/docs/reference/cli-reference/` | 200 | twenty `kscore-*` binaries with full `--help` output |
| `https://docs.keystone-core.io/docs/modules/` | 200 | the 35-module state catalogue |
| `https://docs.keystone-core.io/docs/operations/` | 200 | day-2 runbooks (backup/restore, DR, upgrade-cluster) |
| `https://docs.keystone-core.io/docs/reference/roadmap/` | 200 | the Generation 1 ranked backlog, **without** the R02 freeze notice |
| `https://docs.keystone-core.io/docs/reference/project-reboot-review/` | **404** | — |

The build predates even R00: `/docs/reference/project-reboot-review/` and
`/docs/reference/reboot-execution-plan/` both 404, and the served roadmap page
lacks the "Generation 1 archive notice" that R02 added. Grepping the site's own
search index (`/en.search-data.json`, 906 KB):

```
apt-get install -> 2      dpkg -i -> 4      rpm -i -> 3
kscorectl -> 178          kscore-server -> 288
"Generation 2" -> 0
```

The fallback rule `deploy/docs/README.md` specifies is also absent:
`https://docs.keystone-core.io/some/deep/path` returns **404**, not the
announcement.

This host is the `website` field on **both** forge repositories, so it is the
first-party destination a visitor reaches from either project page.

**Why it matters.** This is the single place where the reboot is most visible to
an outsider, and it currently tells them Keystone Core is an installable, supported
Linux fleet product with twenty binaries and a runbook library. `SECURITY.md` says
"There is no supported release of Keystone Core" and "Do not deploy them"; the
site contradicts that at 100 URLs. The manifest records the deployment as deferred
(see D-08 for the wording), so this is a known gap rather than a concealed one —
but it is the gap, and R10's acceptance covers "public links".

---

### D-02 — HIGH — Both forge repository descriptions still advertise Generation 1 capabilities and "v0.x pre-stable"

**Claim** (`docs/transition/manifest.json`,
`planning_state_publication.deferred.repository_metadata`):

> "PATCH /repos/... returns 403 for this token. Description, topics and website are
> maintainer actions; the maintainer elected to keep them, dropping only the 'v0.x
> pre-stable.' clause."

**What I found.** The clause is still there, on both forges, verbatim:

```
Codeberg GET /repos/Spicer-Creek-Solutions-LLC/keystone-core
  description = 'GitOps deploys it. We keep it running. A runtime operations control plane
                 for Linux fleets — state, audit, events, secrets. v0.x pre-stable.'
  website     = 'https://docs.keystone-core.io'
  topics      = ['configuration-management','control-plane','day2-operations',
                 'fleet-management','gitops','golang','linux']

GitHub GET /repos/Spicer-Creek-Solutions-LLC/keystone-core
  description = (identical string)
  homepage    = 'https://docs.keystone-core.io'
  topics      = [...,'infrastructure-as-code','nats','state-management',...]
```

**Why it matters.** Two problems, not one. First, the recorded decision has not
been carried out, so the manifest states an intention as though it were settled
and the reader has no way to tell. Second, the description names four capability
areas — state, audit, events, secrets — that RFC 0001 explicitly demotes to Future
candidates, and "v0.x pre-stable" asserts a shipping pre-release line. This is the
one-line summary shown in every search result, every fork listing and every social
card for the project.

---

### D-03 — HIGH — The live public bug-report form presents Generation 1 as an installed, supported product

**Location.** `.forgejo/ISSUE_TEMPLATE/bug_report.md`, rendered at
`https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/issues/new/choose`
(200; `has_issues = true`).

```markdown
## Environment

- **Keystone Core version**: <!-- `kscorectl --version` -->
- **OS**: <!-- e.g., Ubuntu 24.04, Debian 12, Rocky 9 -->
- **Install path**: <!-- .deb / .rpm package, built from source, container -->
- **Deployment mode**: <!-- single-node trial, multi-host, embedded NATS, external NATS -->

## Logs / Output
  sudo journalctl -u kscore-server -n 100 --no-pager
  sudo journalctl -u kscore-agent -n 100 --no-pager
```

`.forgejo/ISSUE_TEMPLATE/config.yml` compounds it:

> "Many 'how do I…' questions are answered in docs/project/GETTING-STARTED.md, the
> CLI / Configuration / API references, and the runbooks under docs/runbooks/.
> Check there before filing."

All four of those paths were deleted by R08.

**Why it matters.** R08's acceptance criterion is "no installation instructions in
README, SECURITY.md marks every version unsupported". The check was scoped to prose
documents and missed the interactive surface, which is arguably the stronger claim:
a form that asks which package format you installed from is an assertion that
packages exist. `clean_baseline.retained` lists "issue templates" among what was
deliberately kept, so these were considered and kept as-is.

---

### D-04 — HIGH — `ISSUE-TRACKING.md` says the announcement is pinned; it is not, and R09's own log says so

**Claim** (`docs/project/ISSUE-TRACKING.md`, first paragraph):

> "The tracker holds the Generation 2 planning state and nothing else: one `v0.6.0`
> milestone, twelve workstream epics for `P00`-`P11`, a release tracker, and a
> **pinned** reboot announcement."

**What I found.**

```
GET /repos/.../issues/268   -> is_pinned: None, pin_order: 0
GET /repos/.../issues/pinned -> []
```

And `docs/transition/r09/apply.txt`, written in the same task:

```
pins:
  ANNOUNCE-reboot  PIN FAILED on #268: POST /repos/.../issues/268/pin:
    403 Forbidden: {"message":"user should be an owner or a collaborator with admin write of a repository"}
```

and `manifest.json` → `planning_state_publication.issues[0]`:
`"pinned_requested": true, "pinned": false`.

**Why it matters.** The evidence file, the manifest and the prose document were
produced by the same task and disagree. The machine-readable side is correct and
candid; the human-readable side — the one a reader is most likely to consult — is
not. That is the exact failure mode R10 exists to catch.

---

### D-05 — HIGH — `README.md` says the issue tracker is empty; it holds fourteen open issues

**Claim** (`README.md`, Contributing):

> "The issue tracker is empty by design: its 106 Generation 1 issues were closed as
> superseded, not completed, during the transition."

**What I found.** `open_issues_count = 14`; the listing returns #268–#278 and
#280–#282 (#279 is the R09 pull request). All fourteen are open, labelled
`generation/2`, and were created by R09 on 2026-09-11.

The manifest anticipated this — `post_snapshot_updates[3]` records "The tracker is
no longer empty" — but `README.md` was not updated. The statement was true between
R07 and R09 and has been false since.

**Why it matters.** `README.md` is the repository's front page on both forges and
the one document every visitor reads. A reader who takes it at face value concludes
there is no Generation 2 work tracked anywhere.

---

### D-06 — HIGH — The public vulnerability-disclosure route is a dead end and contradicts itself across three documents

**Claim** (`SECURITY.md`):

> "Report privately rather than through a public issue.
> - Use the repository's private security-report route, or
> - email the maintainer listed in [`OWNERSHIP.md`](OWNERSHIP.md)."

**What I found.**

1. `OWNERSHIP.md` contains **no email address**. Its entire Contact section reads:

   > "- For project / technical questions: open an issue at the Codeberg repository.
   > - For ownership, governance, or legal questions: open an issue tagged
   > `governance`, or contact a maintainer directly."

   So the only route `SECURITY.md` offers by name resolves to "open a public issue",
   which `SECURITY.md` forbids. (`governance` is also not among the labels
   `ISSUE-TRACKING.md` documents.)
2. The "private security-report route" is doubtful. `tag_protections`,
   `branch_protections` and `hooks` all 403 for this token so I cannot enumerate
   repository settings, and the project's own
   `.forgejo/ISSUE_TEMPLATE/security.md` says: *"GitHub-style 'private
   vulnerability reporting' exists on some forges but not all."*
3. `docs/project/SECURITY-RELEASE.md` — which `SECURITY.md` names as the normative
   handling process — gives a *different* channel:

   | `<security@keystone-core.io>` | General security reports | 24 hours |
   | HackerOne (if applicable) | Bug bounty reports | 24 hours |

   `docs/project/SECURITY-GOVERNANCE.md` gives two more:
   `<security-wg@keystone-core.io>` and `<security@keystone-core.io>`, plus
   "**PGP Key Fingerprint**: (Published on website)" — the website being the
   Generation 1 site in D-01, which publishes no such key.

`keystone-core.io` has live MX records (`in1-smtp.messagingengine.com`), so the
addresses may well be deliverable; I have no way to test whether they are monitored.

**Why it matters.** `SECURITY.md` is the document Codeberg surfaces from its own
security tab and that both issue templates redirect to. A reporter following it
lands nowhere, and the three documents that describe the process name four
different destinations between them.

---

### D-07 — HIGH — The pull-request template requires build gates that no longer exist

**Location.** `.forgejo/PULL_REQUEST_TEMPLATE.md`, rendered on every PR.

> "Local verification (use Make targets per AGENTS.md § 2):
>
> - [ ] `make test` passes
> - [ ] `make test-integration` passes (if your change touches the integration surface)
> - [ ] `make lint` passes
> […]
> - [ ] `docs/project/{CLI,CONFIGURATION,API}-REFERENCE.md` — regenerated via `make docs-sync`
> - [ ] `docs/runbooks/*.md` — if operational procedures shifted
> - [ ] Changelog fragment added under `.changes/unreleased/` (via `make changelog-new` …)
> - [ ] The `goheader` linter enforces `// SPDX-License-Identifier: Apache-2.0` …"

The Makefile now defines exactly five targets: `help`, `docs-lint`, `docs-lint-fix`,
`docs-links`, `stray-binary-check`, `capability-catalog-check`, `check`. There is no
`make test`, `lint`, `test-integration`, `docs-sync` or `changelog-new`; no
`.changes/`, no `docs/runbooks/`, no generated references, no `goheader` (no Go
linter configuration at all). `CONTRIBUTING.md` itself says "Changelog entries:
There are none."

`AGENTS.md § 2` — which the template cites for these targets — was rewritten and
now lists none of them.

**Why it matters.** Every contributor is handed a checklist of impossible steps as
the merge bar, and R08's `rewritten` list does not include this file, so it was
never reviewed against the baseline it now sits on.

---

### D-08 — HIGH — The manifest's GitHub-mirror findings are false as of today; the mirror is alive, synced, and carries both protected archive refs

**Claim A** (`manifest.json` → `archive.mirror`):

> "observed": "The GitHub mirror was last updated 2026-09-06 at 0a476caaa, which
> predates every reboot commit. It is not syncing, so the archive refs do not
> resolve there."
> "acceptance_impact": "R05's 'both forges resolve the refs' is met on Codeberg only."

**Claim B** (`manifest.json` →
`planning_state_publication.deferred.github_mirror`):

> "Dead at 0a476caaa (2026-09-06), before any reboot commit, so it **still presents
> Generation 1 as current**. The maintainer is repairing the sync after the reboot
> completes."

**What I found.**

```
GET https://api.github.com/repos/Spicer-Creek-Solutions-LLC/keystone-core
  pushed_at = '2026-09-11T12:52:01Z'

branches: archive/2026-09-pre-v0.6-reboot  93eb147f7fcc  protected True
          main                            47ae589b817f  protected True
tags:     archive-2026-09-pre-v0.6-reboot  -> 93eb147f7fcc
          archive/v0-final                 -> 7d21b848a6d9
          v0.5.0 -> 0f4810cd1d3f     v0.1.0 -> 8a48da100701
```

The mirror is at `47ae589b8` (the R08 merge), pushed today, and its rendered README
contains "There is nothing to install". Every tag resolves to the same commit as
Codeberg. So:

- Claim B is false in both halves: the mirror is not dead at `0a476caaa`, and it
  does not present Generation 1 as current (it is one commit behind `main`, missing
  only R09).
- Claim A is false: both archive refs resolve on GitHub and both report
  `protected: true`. R05's acceptance criterion "both forges resolve the refs",
  recorded as met on Codeberg only, is in fact met on both — and RFC 0001's
  "The branch and tag will be protected on the primary Codeberg repository **and the
  GitHub mirror**" is satisfied rather than waived.

**Why it matters.** The direction of the error is benign — the record is more
pessimistic than reality — but a transition record whose stated observations are
wrong is not evidence. R05's acceptance was recorded as partially unmet on the
strength of an observation that no longer holds, and nobody re-checked. Note the
mirror's description and topics are still Generation 1's (D-02), and
`has_issues = false` there, so `.github/ISSUE_TEMPLATE/config.yml` never renders.

---

### D-09 — HIGH — `SECURITY-GOVERNANCE.md` was retained as "project process" but is substantially a Generation 1 product document, and contradicts `FREEZE.md`

**Claim** (`manifest.json` → `clean_baseline.decisions.security_docs`):

> "SECURITY-DESIGN and SECURITY-REVIEW removed as Generation 1 product
> documentation; SECURITY-GOVERNANCE and SECURITY-RELEASE **retained as project
> process**."

**What I found.** `SECURITY-GOVERNANCE.md` is 704 lines and describes, in the
present tense, a Generation 1 product and organisation:

- *"Security Baseline Pipeline (Epic 19 task 7) — Four scans gate every PR via CI's
  `security` job. The same scans are runnable locally as `make security-secrets`,
  `make security-vulns`, `make security-sast`, `make security-licenses`."* None of
  those targets exist; the only CI job is `reboot-baseline / verify`.
- *"Go toolchain pinned to `go 1.27.1` in `go.mod`"* — there is no root `go.mod`.
- *"Release Dry-Run Smoke (Epic 19 task 13): `make release-dry-run` builds the full
  goreleaser snapshot (5 archives, 12 nfpm packages, `checksums.txt`) … CI's
  `release-dry-run` job runs the full smoke (including container install) on every
  PR"*, with a table asserting `kscore-server.rpm` installs cleanly via `rpm -i`.
- *"**Dependabot alerts enabled for all repositories**"* — flatly contradicted by
  `docs/transition/FREEZE.md`: *"the repository has no auto-merge, no merge queue,
  **no Dependabot, and no Renovate**."*
- A Security Working Group, Security Maintainers, Security Champions, a Security
  Response Team, required training, KPIs and a security dashboard — for a project
  whose `MAINTAINERS.md` describes a BDFL and "maintainers respond as bandwidth
  allows".
- `## Document History | 1.0 | 2025-01 | Security Team | Initial governance framework` —
  never revised.

Only the outbound links were touched at R08 (repointed at the archive tag); the
body was not reviewed.

**Why it matters.** `SECURITY.md` — the public front door — sends readers here for
"the handling process, severity model and disclosure expectations". What they find
describes a scanning pipeline, a release process, a package matrix and a security
organisation that do not exist. The manifest's one-line justification ("retained as
project process") is true of the document's *title* and not of its *contents*, which
is the definition of a true-but-misleading claim.

---

### D-10 — MEDIUM — The amended Rollback section is not public; `main` still carries the unbacked claim

**What I found.** The R10 amendment (`3a378779a`) exists only on
`reboot-r10-transition-audit`. Fetching what the public actually gets:

```
$ curl https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/raw/branch/main/docs/rfcs/0001-generation-2-reboot.md

## Rollback

Before the clean-baseline PR merges, rollback means abandoning the transition
branch. After merge, Git rollback means reverting the baseline commit or
branching from the immutable Generation 1 tag. Forge mutations have their own
before-state manifest and compensating procedure. …
```

Every public pointer to RFC 0001 — issue #268, the `deploy/docs` announcement page,
both annotated release bodies, `README.md` on the GitHub mirror — uses the
`src/branch/main/` form and therefore serves this text.

**On the amendment itself: it is honest.** I diffed `3a378779a` and its description
of what changed is exact. The removed sentence is quoted verbatim and correctly; the
replacement names three real manifest keys (`ci_and_protection.before`,
`tracker_retirement`, `planning_state_publication`), all of which exist; the commit
message states plainly that "no compensating procedure was ever written … so an
accepted decision record asserted an artifact that does not exist" and that it is
"Corrected rather than satisfied"; and it explains why an in-place dated amendment
rather than a supersession (`docs/project/RFC.md` defines "Superseded by #NN" for
replacement and nothing for a factual correction). It also states, accurately, that
it was written *before* the audit so the auditors evaluate what is there. I have no
objection to the amendment. This finding is only that it has not reached the public
record yet, and will not until the R10 PR merges.

---

### D-11 — MEDIUM — "Every forge mutation records its before-state" is not literally true for two of them

**Claim** (RFC 0001, amended Rollback section):

> "Every forge mutation records its before-state. […] The before-states are in
> `docs/transition/manifest.json` — `ci_and_protection.before` […],
> `tracker_retirement` with the reviewed snapshot in `docs/transition/r07/` […],
> and `planning_state_publication` for the Generation 2 planning state."

**What I found.** Two of the three hold cleanly. `ci_and_protection.before` records
the twelve prior required contexts and the protection settings. `tracker_retirement`
plus `r07/snapshot.json` records all 106 issues in their pre-mutation state, and I
verified `snapshot.json`'s digest matches.

`planning_state_publication` does **not** record a before-state — it records the
after-state: what was created (milestone, three labels, fourteen issues) and what
was left alone. The nearest thing to a before-state is one line in
`docs/transition/r09/apply.txt` — *"tracker holds 120 issues; 0 carry a Generation 2
task marker"* — which the RFC does not name.

More materially, **the two release-body annotations have no recorded before-state
anywhere.** `planning_state_publication.release_annotations` says only
`"change": "body only; the notice prepended above the original notes"`. I confirmed
by reading the live v0.5.0 body that the original text survives below a `---`
separator, so the mutation is in practice reversible by hand — but that is an
inference from the current state, not a record. If someone edited those bodies
again, nothing in the transition record would establish what they used to say.

**Why it matters.** The sentence is the load-bearing one in the amended Rollback
section: it is what replaced the deleted "compensating procedure" claim. It should
hold for every mutation the transition performed, and for release annotations it
does not.

---

### D-12 — MEDIUM — `tracker_retirement.allowlist_sha256` is not the file's digest, and the only code that could reproduce it was deleted the next task

**Claim** (`manifest.json` → `tracker_retirement`):

```json
"artifacts": ["docs/transition/r07/snapshot.json",
              "docs/transition/r07/allowlist.json", …],
"allowlist_sha256": "93bb3f86842117cae4e6f05be19c4e3f740102e9695bc0118b76f5cb07fdc762",
"snapshot_sha256":  "9ace87373af1f3cf1dbc161563ca07572e612cd74769cdc7095d37c767b33c30"
```

**What I found.** `snapshot_sha256` is the file digest and matches exactly.
`allowlist_sha256` does not, and never has:

```
$ sha256sum docs/transition/r07/allowlist.json
e6663146d0e70c6b302a18442b05f2121866d896794bf52fe5ab9bce169042d1
```

The file has exactly one revision in history (`1901975a5`), hashing to the same
`e6663146…`, so the recorded value never matched any committed state. It is in fact
the allowlist's internal `entries_sha256` — a digest of the *entry list*, not the
file — computed by a bespoke algorithm. I reproduced it by recovering the deleted
`tools/transition/allowlist.go` from `b874f7223^`:

```go
fmt.Fprintf(h, "%d\x00%s\x00%s\x00%s\x00%s\n", e.Number, e.Identity, e.State, e.UpdatedAt, e.Kind)
```

Re-implementing that over the 106 sorted entries yields
`93bb3f86842117cae4e6f05be19c4e3f740102e9695bc0118b76f5cb07fdc762` — **match**. The
record is therefore *correct*, and the allowlist is intact.

**Why it matters.** Two adjacent fields in the same object use two different and
undocumented hashing conventions, one of which requires a tool that R09 deleted. An
auditor who does the obvious thing — `sha256sum` both files — gets one match and one
mismatch on an integrity record, with nothing in the manifest to explain it. It cost
me four steps and a `git show` into deleted history to clear. Recording the file
digest alongside it, or renaming the field to `allowlist_entries_sha256`, would
remove the trap.

---

### D-13 — MEDIUM — `planning_state_publication.source_sha256` does not match the source document in the commit that records it

**Claim.**

```json
"source_document": "docs/project/REBOOT-EXECUTION-PLAN.md",
"source_sha256":   "a745b20358342f80dd62eef90610ebd6486c31ba584d2df4f2bf36ca3880e704"
```

**What I found.** The current file hashes to
`5d5d7c9816d676866970fba704ce5240373eeaf5a00257e404e13a9c67301525`. Walking the
file's history, `a745b20…` is its state at `242830bfb` (R08). Commit `b874f7223`
edited `REBOOT-EXECUTION-PLAN.md` **and** added this manifest block in the same
commit — so the digest was stale the moment it was written.

The publication itself is fine: the plan was at `a745b20…` when the apply ran at
14:23Z, and `plan_sha256` (`daf30adc…`) matches `dry-run.txt`, `journal.json` and
`publication-plan.json` consistently. This is a provenance-record defect, not a
publication defect.

**Why it matters.** The field exists so a reader can confirm which text the fourteen
issue bodies were derived from. As recorded, running `sha256sum` on the named
document in the commit that names it produces a mismatch with no explanation, which
looks exactly like tampering.

---

### D-14 — MEDIUM — Every R09 and R10 commit is unsigned, breaking the transition record's own signing chain

**What I found.**

```
3a378779a N docs(rfc): correct the Rollback section's unbacked claim (R10)
b874f7223 N chore(baseline): remove tools/transition and record the publication (R09)
6e755808e N fix(transition): judge a replacement by when it was created…
291690baf N docs(r09): record the publication dry run
d5c33e2ef N docs(r09): compose the publication plan…
96a14f255 N docs(deploy): publish the reboot announcement…
db5866109 N feat(transition): add the Generation 2 publication commands (R09)
242830bfb G feat(baseline): land the clean Generation 2 baseline          (R08)
1901975a5 G feat(transition): retire the Generation 1 tracker             (R07)
238953ac0 G docs(transition): record the verified branch-protection after-state
0a08e1913 G ci(reboot): add the baseline gate…
```

Every substantive transition commit from R02 through R08 is GPG-signed and
forge-verified. Every R09 commit, and the R10 amendment, is not.

**Why it matters.** `manifest.json` devotes a whole `signing_preflight` block and two
`post_snapshot_updates` entries to establishing that this key signs and that Codeberg
verifies it, because RFC 0001 pauses the transition rather than accept weaker
evidence. R09's commits are the ones that carry the record of the last forge
mutation and the publication digests — precisely the material the signing chain was
established to protect — and they carry no signature. Nothing in the record notes the
lapse.

---

### D-15 — MEDIUM — `CONTRIBUTING.md` still gives Generation 1 coding instructions and points at GitHub Discussions

`clean_baseline.rewritten` lists `CONTRIBUTING.md`. It was partly rewritten — the
threat-model, release-process and changelog sections are correct and current — but:

> "**During development:**
> - Validate all external input using `pkg/security.Validate*` helpers
> - Use parameterized queries — never concatenate SQL strings
> - Avoid shell injection — never pass user input to `exec.Command` without validation
> - Use structured logging with automatic redaction for sensitive data"

`pkg/` was deleted at R08. And:

> "## Questions?
> - Open a GitHub Discussion"

The GitHub repository is a code-only mirror with `has_issues = false`; Codeberg is
the canonical tracker, and `.github/ISSUE_TEMPLATE/config.yml` says so explicitly.

---

### D-16 — MEDIUM — `GOVERNANCE.md` and `MAINTAINERS.md` still describe the v0.1.x invited-installer posture as current

`docs/project/GOVERNANCE.md` § Launch Posture, present tense:

> "**v0.1.x — soft launch.** The repository is made public on Codeberg without a
> coordinated announcement […] The intended v0.1.x audience is operators who have
> been **explicitly invited to install** (per [`VERSIONING.md`](VERSIONING.md)) […]
> A hard-launch announcement […] is **not planned until at least v0.5**, when the
> v0.5 gates in [`VERSIONING.md`](VERSIONING.md) have been met."

`docs/project/MAINTAINERS.md` L124 likewise: *"beyond the v0.1.x invited-installer
base."*

`VERSIONING.md` was rewritten at R08 and now contains no v0.1.x audience policy and
no v0.5 gates at all, so both cross-references point at a document that no longer
says what is attributed to it. Meanwhile R09 *did* publish a coordinated
announcement (issue #268), and v0.5 shipped and was then archived. Every clause here
is false.

---

### D-17 — MEDIUM — `deploy/docs/README.md`'s own verification commands fail

> ```bash
> curl -fsSL https://docs.keystone-core.io/ | grep -E 'nothing to install'
> curl -fsSL https://docs.keystone-core.io/some/deep/path | grep -E 'Keystone Core'
> ```
> "The second checks the fallback rule: a deep path that no longer exists must still
> return the announcement."

Run today:

```
$ curl -fsSL https://docs.keystone-core.io/ | grep -E 'nothing to install'
(no output, exit 1)
$ curl -fsSL https://docs.keystone-core.io/some/deep/path
curl: (22) The requested URL returned error: 404
```

Both fail — correctly, because of D-01. Recorded separately because the document
supplies a working detector for the defect and nobody ran it. To the document's
credit, its "A caveat worth knowing" section is the most honest paragraph in the
`deploy/` tree: it says outright that nothing in CI checks this page, that
`make docs-links` "parses no HTML and resolves no external URL", and that this is
"how the previous placeholder came to announce a v0.5 documentation site and link to
three files that had been deleted". That warning is accurate (see D-22) and the same
failure has recurred one layer up.

---

### D-18 — MEDIUM — `FUTURE-CAPABILITIES.md` still promises an R05 deliverable in the future tense that was never performed

> "After archive task R05, canonical historical links **will resolve** under these
> locations:
>
> - `archive/2026-09-pre-v0.6-reboot:FEATURES.md`
> - `archive/2026-09-pre-v0.6-reboot:PROJECT-DETAILS.md`
> - `archive/2026-09-pre-v0.6-reboot:docs/project/ROADMAP.md`
> - `archive/2026-09-pre-v0.6-reboot:epics/`
>
> They are code-form placeholders, not broken forward links. […] **R05 turns the
> convenience archive locations into links.**"

R05 completed on 2026-09-10. All four are still code-form placeholders at
`docs/project/FUTURE-CAPABILITIES.md:10-13`. The refs themselves do resolve, so
nothing is broken — but a stated task deliverable was silently not done, and the
document's tense tells a reader R05 has not yet run.

---

### D-19 — MEDIUM — `NOTICE` describes the dependency set of a product that no longer exists, and one attribution link 404s

`NOTICE` is a licence-adjacent public document. Its THIRD-PARTY NOTICES section:

> "The complete list of direct and indirect dependencies is recorded in **go.mod /
> go.sum**, and a machine-readable license report can be regenerated at any time with
> `make security-licenses` (wrapping `go-licenses report ./...`).
>
> The exhaustive list is in go.mod; the components below are called out because they
> are significant to the runtime architecture…"

There is no `go.mod`, no `go.sum` and no `make security-licenses`. It then attributes
NATS, etcd, gRPC, OpenTelemetry, Prometheus, SPIFFE, HashiCorp Vault (with a
paragraph of MPL-2.0 reasoning about "the binaries" this project ships), OPA, CEL,
Starlark, go-git and minio-go — none of which the repository depends on; the single
surviving module `tools/capcheck` is stdlib-only.

One of the attribution links is dead:

```
https://github.com/google/cel-go -> 404   (api.github.com/repos/google/cel-go -> 404)
```

Only reachable because `make docs-links` does not check `NOTICE` (not `*.md`) and
resolves no external URL anyway.

---

### D-20 — MEDIUM — `.lychee.toml` configures the link gate for a tree that no longer exists

```toml
  # Pre-launch placeholders. … per docs/project/PUBLIC-LAUNCH-CHECKLIST.md Phase B6 + F-phase.
  "@keystone-core.io", "@keystone.io", "docs.keystone.io", "keystone-community.slack.com",
exclude_path = [
  "docs/themes", "docs/node_modules", "docs/resources", "docs/public",
  # … Those are link-checked by `make docs-links-site` over docs/public/ instead
  "docs/content"
]
```

`docs/project/PUBLIC-LAUNCH-CHECKLIST.md`, `docs/themes/`, `docs/resources/` and
`docs/content/` were all deleted at R08; `make docs-links-site` does not exist.
`.gitleaks.toml` similarly still allowlists `docs/content/.*\.md$`. None of this
breaks the gate — it just no longer describes anything.

---

### D-21 — MEDIUM — `SECURITY.md` offers to act on reports against tooling that was deleted

> "Reports against the current repository contents — **the transition tooling in
> `tools/`**, or the published documentation — are acted on."

`tools/` now contains only `tools/capcheck`. The transition tooling was removed at
R09, in the same task that published this wording's context. Minor in isolation,
but it is in the security policy, which is the document that most needs to describe
the current attack surface accurately.

---

### D-22 — MEDIUM — No external link in any tracked document has ever been machine-checked; I checked them, and found one genuine 404

The gate is `lychee --offline`, which skips every `http`/`https` URL:

```
$ lychee --offline --config .lychee.toml --root-dir "$(pwd)" "**/*.md"
🔍 208 Total  🔗 98 Unique  ✅ 149 OK  🚫 0 Errors  👻 59 Excluded
```

All 59 exclusions are external URLs. The glob is also `**/*.md` only, so
`deploy/docs/site/index.html`, `deploy/vanity/site/keystone-core/index.html`,
`NOTICE` and the YAML templates are never parsed at all.

I resolved all 96 unique URLs across all 73 tracked files by hand. Results:

- **All 34 Codeberg URLs return 200**, including every `src/tag/archive-2026-09-pre-v0.6-reboot/...`
  deep link (16 of them: `FEATURES.md`, `RELEASE-PLAYBOOK.md`, `deploy/systemd`,
  `deploy/grafana`, `COVERAGE-GATES.md`, `DESIGN.md`, `INCIDENT-RESPONSE.md`,
  `POLICY-AUDIT.md`, `PUBLIC-LAUNCH-CHECKLIST.md`, `SECURITY-DESIGN.md`,
  `SECURITY-REVIEW.md`, `STATE-SUPPORT-MATRIX.md`, `TEST-POLICY.md`,
  `epics/00-meta-reconstruction-plan.md`, `.changes/unreleased`, `docs`) and all eight
  `src/branch/main/...` links in the announcement page.
- **All 8 URLs in the fourteen R09 issue bodies return 200.**
- **All 8 URLs in the two annotated release bodies return 200.**
- One genuine external 404: `https://github.com/google/cel-go` in `NOTICE` (D-19).
- One expected failure: `https://docs.keystone-core.io/some/deep/path` (D-17).
- One transient `429` (hashicorp.com PDF in `PROJECT-REBOOT-REVIEW.md`,
  rate-limited, not a defect).

So R08's `archive_links_resolve` acceptance ("18 distinct archived paths … all
resolve at the archive tag") is substantively confirmed. Recorded as MEDIUM because
the *gate* provides no assurance here and the project has no mechanism that would
catch the next dead link; the current state is clean by luck of one person checking.

---

### D-23 — LOW — `deploy/vanity/README.md` is stale in three places

- *"The lychee exclusion that lets CI link-checking pass **before the domain is
  live**"* — the domain has been live throughout this audit. The exclusion it points
  at is the string `@keystone-core.io`, which matches `mailto:` addresses, not
  `docs.keystone-core.io` / `go.keystone-core.io`; and `--offline` skips those
  regardless. The stated mechanism is not the actual mechanism.
- Its "Verifying" step tells the reader
  `GOPROXY=direct go list -m -versions go.keystone-core.io/keystone-core` "should
  list the project's tags (e.g. v0.1.0, v0.1.0-rc1, …)". There is no root module at
  the tip any more; the guidance was written for Generation 1.
- Links to `PUBLIC-LAUNCH-CHECKLIST.md` for provisioning context (correctly
  archive-pinned, resolves 200 — noted only for completeness).

The generated page and `vangen.json` are byte-identical to what the host serves,
which I confirmed; the vanity deployment itself is correct.

---

### D-24 — LOW — The live vanity page advertises `go get` for a module path that no longer has a `go.mod`

`https://go.keystone-core.io/keystone-core` serves, correctly and matching the repo:

```html
<code>go get go.keystone-core.io/keystone-core</code>
<code>import "go.keystone-core.io/keystone-core"</code>
```

The `go-import` meta tag is right and the namespace is genuinely in use — but only
by `go.keystone-core.io/keystone-core/tools/capcheck`. The root path has no `go.mod`
at the tip, so the two commands the page displays as the headline instruction cannot
succeed against `main`. Consistent with `clean_baseline.decisions.vanity_imports`
("the two surviving tool modules declare paths under go.keystone-core.io") — which
is itself now off by one, since `tools/transition` was removed at R09.

---

### D-25 — LOW — `README.md` and issue #268 assert tag protection more strongly than the manifest supports

`README.md`: *"Both refs are protected against update and deletion."*
Issue #268 (public, and the announcement of record): the identical sentence.

The manifest is more careful, and more honest:

> `archive.protection.tag.verification`: "Confirmed by the maintainer in the Codeberg
> UI on 2026-09-10T13:04:22Z. **Still not independently observed by this
> credential**: GET /tag_protections returns 403 for a non-admin token and the tag
> read exposes no protection flag. The distinction is kept because the branch
> protection above rests on two observed negative probes, and this rests on a human
> reading a settings page."

I reproduced the limitation: `GET /repos/.../tag_protections` → **403**. The branch
half of the claim I did verify (`protected: true`, `user_can_push: false`). So the
public sentence flattens a distinction the record deliberately preserved. Low
severity because the underlying protection is probably real and the tag's integrity
rests on its signature rather than on forge settings — but the manifest's own
reasoning for keeping the distinction applies equally to the README.

---

### D-26 — LOW — `OWNERSHIP.md` release-infrastructure row still references the Generation 1 signing plan

> "Release infrastructure | SCS | Build/sign infrastructure, release-signing keys
> (when the **v1.2 multi-party signing ceremony** lands per `RELEASE-PLAYBOOK.md`)"

`VERSIONING.md` now states signing is decided in Stage C (C13) for `v0.6.0`;
`RELEASE-PLAYBOOK.md` exists only in the archive and is referenced here without the
archive-tag prefix used elsewhere.

---

### D-27 — LOW — RFC 0001 says every archived capability starts in `Future / Unscheduled`; 49 are scoped `v0.6`

RFC 0001: *"Every archived capability starts in `Future / Unscheduled`, without a
version promise or leaf issue."*
`FUTURE-CAPABILITIES.md` Scope column: 651 `Future`, **49 `v0.6`**.

The catalogue documents this as the deliberate "scope subtraction rule" — the 49 are
the narrow slice RFC 0001 itself accepts, "excluded from `Future / Unscheduled`" —
and `capability-coverage.json` confirms 703 entries over 1,064 source items, matching
every count in `README.md`, `ROADMAP.md` and `docs/project/README.md`. So this is a
wording tension, not a contradiction. Noted because the RFC's absolute "every" reads
as a stronger commitment than the catalogue implements.

---

### D-28 — LOW — `.forgejo/ISSUE_TEMPLATE/security.md` and `CHANGELOG.md` reference `tools/transition`-era context

Minor residue only: issue #268 and every epic issue end with
*"Created by `tools/transition publish` (reboot task R09)."* — a tool that the same
task deleted. Historically accurate and arguably correct provenance, but a reader who
looks for it in the repository will not find it, and nothing in the issue says so.

---

## Verified clean — with evidence

These are the claims I tried hardest to break and could not.

| Claim | Source | Verification |
|---|---|---|
| Archive branch at `93eb147f7fcc…` | `archive.branch` | Codeberg `GET /branches/archive%2F...` → `93eb147f7fcc…`, `protected: true`, `user_can_push: false` |
| Archive tag object `d99de5715cc8…` → `93eb147f7fcc…` | `archive.tag` | Codeberg `GET /git/tags/d99de57…` exact match |
| Tag is signed **and forge-verified** | `archive.tag.signed`, `post_snapshot_updates[1]` | Codeberg returns `verification.verified: true`, `reason: "keystone-bot / 23583C11192A9DF4"`; `git tag -v` good locally |
| `v0.1.0` / `v0.5.0` / `archive/v0-final` tag object IDs unchanged | `archive.historical_tags_reverified` | All three match Codeberg, GitHub and local git exactly |
| Bundle SHA-256 `34989c1fb33a…` | `archive.bundle.sha256` | `sha256sum ~/keystone-archive/keystone-core-generation-1-93eb147f7.bundle` → **exact match**, 84,189,066 bytes |
| All five R09 artifact digests | `planning_state_publication.artifacts` | All five `sha256sum` → **exact match** |
| R07 snapshot digest | `tracker_retirement.snapshot_sha256` | `sha256sum` → **exact match** |
| R07 allowlist integrity | `tracker_retirement.allowlist_sha256` | Reproduced via recovered `allowlist.go` → **match** (see D-12 for the caveat) |
| 106 issues retired, 103 leaf + 3 tracker | `r07/verification-report.json` | Recomputed from `allowlist.json`: 106 / 103 / 3 |
| 120 closed + 14 open = 134 tracker issues | `r09/verification.txt` | Codeberg: `open_issues_count = 14`; report records 120 closed |
| All 14 R09 issues exist, once each, correctly labelled and milestoned | `planning_state_publication.issues` | Fetched all 14; numbers, titles, labels, milestone all match; #279 is the PR, explaining the gap |
| Both releases annotated, assets untouched | `release_annotations` | Both bodies carry the notice above a `---`; 20 assets each; `checksums.txt` downloads (HTTP 206) |
| Branch-protection after-state | `ci_and_protection.after` | Codeberg `GET /branches/main`: `protected: true`, `required_approvals: 0`, `status_check_contexts: ['reboot-baseline / verify (pull_request)']` — exactly one context, matching the `push_context_trap` resolution |
| 703 capabilities / 24 invariants / 163 archived roadmap entries | `README.md`, `ROADMAP.md`, `docs/project/README.md` | Independently counted: 703 `CAP-*`, 24 `ARCH-*`, 163 `####` entries in the archived roadmap |
| R08 file arithmetic (1,989 → 79, 1,911 deleted) | `clean_baseline` | `git diff --name-status 242830bfb^ 242830bfb`: 1911 D, 1 A, 28 M → 1989 − 1911 + 1 = **79** ✓; tree at `242830bfb` is 79 files |
| R09 file arithmetic | (implied) | 79 − 11 + 5 = **73**; `git ls-files \| wc -l` = 73 ✓ |
| "no tracker writers in `.forgejo/workflows/`" | `ISSUE-TRACKING.md`, `r09/verification.txt` | Read `reboot-baseline.yml` in full — markdown lint, lychee, capcheck, DCO. No issue writes |
| The R10 RFC amendment describes its own change accurately | `docs/rfcs/0001` § Amendments | Diffed `3a378779a`; quoted sentence and characterisation are exact |
| R09's apply log is candid about its failure | `r09/apply.txt` | Records the pin 403 verbatim rather than omitting it |
| P/C dependency tables in issue #269 | `REBOOT-EXECUTION-PLAN.md` | All twelve rows match the plan's table row for row |
| Epic 20 checkbox state | `epics/20-...md` | R00–R09 `[x]`, R10 `[ ]` — correct |

I also want to record two places where the record is *better* than it needed to be:
`archive.unique_history.correction` withdraws an earlier overstatement about ~2,000
commits of "otherwise-lost history" in the bundle, and
`r07/verification-report.json`'s `verification_caveat` volunteers that the tool's own
"nothing else moved" check "was vacuous here". Both are self-incriminating and
neither had to be written down.

---

## Claims I could NOT independently verify

1. **Tag protection on Codeberg.** `GET /tag_protections` → 403 (non-admin).
   `archive.protection.tag.maintainer_confirmed` rests on a human reading a settings
   page. The manifest says so plainly; I can neither confirm nor refute it. A delete
   probe is correctly ruled out as destructive.
2. **Branch-protection rule contents on Codeberg.** `GET /branch_protections` → 403.
   I could read `protected`, `required_approvals` and `status_check_contexts` off
   `GET /branches/main`, which matched; the underlying rule objects I could not see.
3. **GitHub mirror protection enforcement.** The API reports `protected: true` for
   `main` and the archive branch, but I have no credential there and performed no
   probe. Whether the tag is protected on GitHub is unknown.
4. **Who restored the GitHub mirror sync, and when.** `pushed_at` is
   `2026-09-11T12:52:01Z` and the mirror holds every reboot commit through R08. The
   transition record says the opposite (D-08) and records no repair. I cannot
   establish the cause from outside.
5. **Bundle copies on the maintainer NAS and workstation.** I verified the dev-VM
   copy byte-for-byte against the recorded digest. The other two custody locations in
   `archive.bundle.custody` are outside my reach.
6. **Package-registry landing pages.** `not_verified.package_registry_landing_pages`
   records these as unchecked for lack of `read:package`. My token has the same
   limitation. **This is honestly characterised** — it says "Recorded as unverified
   rather than claimed clean" — and I confirm the gap is real, not that the pages are
   clean.
7. **Whether `security@keystone-core.io`, `security-wg@keystone-core.io` and the
   other role addresses are monitored.** `keystone-core.io` has live MX records
   (`in1-smtp.messagingengine.com`, `in2-smtp.messagingengine.com`), so delivery is
   plausible. Monitoring is untestable read-only.
8. **Whether Codeberg offers the "private security-report route"** that `SECURITY.md`
   names first. Requires repository settings access (403).
9. **Bundle ref/commit counts** (`refs: 2185`, `commits: 3072`). I verified the
   digest, which binds the content; I did not clone the 84 MB bundle to recount. That
   sits in another auditor's slice.
10. **`known_red_gates` (`security-licenses`).** Recorded as RED on `main` at R02
    with a clear disposition ("a licence-scanning tooling failure, not a licence
    violation: no forbidden, restricted, or unknown licence was reported, because the
    scan aborted before classification"). It is now unfalsifiable in either direction:
    R08 deleted the root module, the Makefile target and the Go code the scan ran
    over. The entry remains open in the manifest with `action_required: "Maintainer
    decision"`, describing a gate that can no longer be run. That characterisation is
    honest for R02's snapshot but is now permanently unresolvable; it should
    arguably be closed as moot rather than left as an open red gate.

---

## What an auditor would want fixed before this gate closes

In rough order of public impact: deploy the announcement page (D-01); correct both
repository descriptions (D-02); rewrite the bug-report and PR templates (D-03, D-07);
fix the three false sentences in `README.md` and `ISSUE-TRACKING.md` (D-04, D-05);
repair the vulnerability-disclosure route (D-06); and re-observe the GitHub mirror
and correct the two manifest entries about it (D-08). `SECURITY-GOVERNANCE.md` (D-09)
is the largest single body of Generation 1 text still presented as current policy and
needs either a real review pass or an explicit "archived, retained for reference"
banner.
