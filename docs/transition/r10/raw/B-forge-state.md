# R10 independent audit — slice B: forge state

**Summary: preservation held and the reset is real — 106/106 Generation 1 issues
retired non-destructively with nothing deleted, all 5 milestones closed and
intact, both historical releases published with all 40 assets and unmoved tags,
and `main` requires exactly one status context in its mergeable `(pull_request)`
form. One HIGH finding: the transition record's statement that the GitHub mirror
is dead and does not resolve the archive refs is false — the mirror synced on
2026-09-11 and carries both archive refs. Two MEDIUM public-presentation gaps
(repository description/topics/site link and `docs.keystone-core.io` still
present Generation 1 as current) and one MEDIUM verification limit (tag
protection is not observable with a non-admin token).**

Auditor: agent that did not perform R05–R09. Read-only. No POST/PATCH/DELETE was
issued against any forge. Every number below comes from a live API call made on
2026-09-11, not from `docs/transition/`.

---

## Method

Forge: `https://codeberg.org/api/v1`, repo `Spicer-Creek-Solutions-LLC/keystone-core`
(id `1749683`), authenticated as `keystone-bot` (`is_admin=false`,
`permissions={admin:false, push:true, pull:true}`).

Calls made (all GET):

| Endpoint | Purpose |
|---|---|
| `/repos/{o}/{r}` | repo metadata, description, topics, website, `archived` |
| `/repos/{o}/{r}/branches`, `/branches/{b}` | protection flags, required contexts |
| `/repos/{o}/{r}/branch_protections`, `/tag_protections`, `/push_mirrors`, `/hooks` | **403** — recorded as limits |
| `/repos/{o}/{r}/tags?limit=100` | tag objects and targets |
| `/repos/{o}/{r}/releases?limit=100`, `/releases/tags/{t}` | release bodies + assets |
| `/repos/{o}/{r}/issues?state=all&type=issues` (3 pages) | full issue census, 134 objects |
| `/repos/{o}/{r}/issues?state=all&type=pulls` (3 pages) | full PR census, 148 objects |
| `/repos/{o}/{r}/issues/{n}/timeline` | **all 106** retired issues, event-level evidence |
| `/repos/{o}/{r}/issues/pinned`, `/issues/268` | pin state |
| `/repos/{o}/{r}/milestones?state=all` | milestone state and counts |
| `/repos/{o}/{r}/labels?limit=100` | label survival |
| `/repos/{o}/{r}/commits/{sha}/statuses` | which status contexts actually report |
| `/repos/{o}/{r}/pulls/279` | identity of the number missing from the Gen 2 range |
| `api.github.com/repos/...` (branches, tags, releases, commits) | mirror state |
| `git ls-remote origin` / `git ls-remote https://github.com/...` | ref objects, independent of both APIs |
| `curl https://docs.keystone-core.io/`, `https://keystone-core.io/` | public link state |

Key raw output:

```
GET /repos/.../branches/main
  "protected": true,
  "enable_status_check": true,
  "status_check_contexts": ["reboot-baseline / verify (pull_request)"],
  "required_approvals": 0, "user_can_push": false, "user_can_merge": false

GET /repos/.../branches/archive%2F2026-09-pre-v0.6-reboot
  "protected": true, "enable_status_check": false, "status_check_contexts": null,
  "user_can_push": false, "user_can_merge": false
  commit 93eb147f7fcc559d31f2cce77d81e791f01673f8

GET /repos/.../branch_protections  -> HTTP 403
GET /repos/.../tag_protections     -> HTTP 403
GET /repos/.../push_mirrors        -> HTTP 403
GET /repos/.../hooks               -> HTTP 403
```

---

## Findings

### B-01 — `main` requires exactly one status context, and it is the mergeable one — PASS (INFO)

**Checked:** the documented trap that Forgejo status contexts carry a trigger
suffix, so requiring both `(pull_request)` and `(push)` makes a branch
permanently unmergeable.

**Found:** `main` reports `enable_status_check: true` with exactly one entry:

```
status_check_contexts: ["reboot-baseline / verify (pull_request)"]
```

That context is really produced, and really goes green, on PR heads:

```
GET /repos/.../commits/b874f72233ec.../statuses      # head of PR #279
success | 'reboot-baseline / verify (pull_request)' | 2026-09-11T16:27:45+02:00
```

The `(push)` variant exists but is **not** required:

```
GET /repos/.../commits/7ca55223125...(main HEAD)/statuses
success | 'reboot-baseline / verify (push)' | 2026-09-11T16:39:45+02:00
```

The workflow backing the context is present at the tip
(`.forgejo/workflows/reboot-baseline.yml`, `name: reboot-baseline`, job `verify`,
triggers `push:[main]`, `pull_request`, `workflow_dispatch`). The other half of
the trap — a required context whose workflow no longer exists — therefore does
not apply either.

**Why it matters:** the repository is mergeable and gated. Both failure modes the
plan names are absent.

### B-02 — Archive branch is protected and at the recorded commit — PASS (INFO)

`archive/2026-09-pre-v0.6-reboot` reports `protected: true`, `user_can_push:
false`, `user_can_merge: false`. `git ls-remote origin` confirms the ref object:

```
93eb147f7fcc559d31f2cce77d81e791f01673f8  refs/heads/archive/2026-09-pre-v0.6-reboot
```

which is the same commit the archive tag dereferences to. The control comparison
holds: `main` reports `protected:true` on the same endpoint and there is no
branch in the repo reporting `protected:false`, so the reading is a positive
signal but not a discriminating one on its own (see B-03).

### B-03 — Tag protection, and the specific force-push/delete flags, cannot be observed with this credential — MEDIUM (verification limit)

**Checked:** the claim that the branch *and tag* are protected against update,
force-update and deletion.

**Found:** `GET /tag_protections` returns HTTP 403
(`"user should be an owner or a collaborator with admin write of a repository"`),
and the tag read (`GET /repos/.../tags`) exposes no protection field at all. So
**tag protection is not independently verifiable by me in any form.** Likewise
`GET /branch_protections` is 403, so for the archive branch I can see *that* a
protection rule exists (`protected:true`, `user_can_push:false`) but **not**
whether `enable_force_push` is off or whether a push/force-push allowlist grants
someone an exception. Whether any protection rule covers a *pattern* (e.g.
`archive/*`) rather than the two existing branches is also invisible.

I deliberately did not probe by attempting a mutation.

**Why it matters:** the strongest preservation claim in the transition record —
that the signed archive tag cannot be moved or deleted — rests, for an outside
reader, on a maintainer's UI reading. The manifest is honest about this and says
so in the same words; I am confirming the limit still stands and that R10 cannot
close it without an admin credential. The mitigating evidence is B-04: the tag
object id is unchanged everywhere I can observe it, including on a second forge.

### B-04 — Historical tags were not moved, and the archive tag's signature verifies against the recorded key — PASS (INFO)

Codeberg API, `git ls-remote origin`, and the local object store agree exactly on
all four tag objects:

| Tag | Tag object | Dereferenced commit |
|---|---|---|
| `archive-2026-09-pre-v0.6-reboot` | `d99de5715cc817fbcdb1654860cf92a33b1934d3` | `93eb147f7fcc…` |
| `v0.1.0` | `318c19f2c806580f4041911e644d4efc6143a7f2` | `8a48da100701…` |
| `v0.5.0` | `7003d8ddda6dfb12fc3d4cc6eff439ed8c919df2` | `0f4810cd1d3f…` |
| `archive/v0-final` | `2f3c81f6cb4592e7e909a14d8fb9695a1f33aae4` | `7d21b848a6d9…` |

All four are annotated tag objects, so moving one would necessarily change the
object id; none changed. The same four objects resolve identically on the GitHub
mirror (B-11), which is an independent copy.

`git verify-tag archive-2026-09-pre-v0.6-reboot`:

```
gpg: Signature made Thu 10 Sep 2026 11:20:46 AM UTC
gpg:                using EDDSA key D1D65D8F57A31599F634E40023583C11192A9DF4
gpg: Good signature from "Keystone-core Bot <keystone-bot@keystone-core.io>"
```

`D1D65D8F…` is the `[S]` subkey of primary `18913D0E53BA40B100C3086A7796A98BEA4F4647`,
which is the fingerprint the manifest records — the manifest is correct, not
inconsistent. `v0.1.0` and `v0.5.0` are annotated but unsigned, which matches
what their own release bodies say.

Both release commits remain reachable: `v0.1.0` and `v0.5.0` are ancestors of the
archive head, which is itself an ancestor of current `main`.

### B-05 — All 106 retirements are real, uniform, and non-destructive — PASS (INFO)

I did not sample. I pulled the **full event timeline for every one of the 106**
issues carrying both labels (106/106 HTTP 200) and asserted per issue.

Census from `GET /issues?state=all&type=issues` (134 objects, 3 pages):

```
issues with BOTH status/superseded + generation/1 : 106   (106 closed, 0 open)
generation/1 total: 106      status/superseded total: 106   (no stragglers either way)
issue number range of the retired set: #17 … #246
```

Per-issue timeline assertions, **zero exceptions across 106**:

- exactly **1** comment whose body starts `**Superseded, not completed.**`
- that comment body is **byte-identical across all 106** (single SHA-256 prefix
  `c2f97da93e57`, count 106)
- exactly **2** label-*add* events, `generation/1` and `status/superseded`; **0**
  label-*remove* events for either
- exactly **1** `close` event; **0** `reopen` events
- **0** `change_title` events, ever, on any of the 106 (the whole event-type
  census is `milestone:136, label:760, issue_ref:101, comment:106, close:106,
  pull_ref:20, commit_ref:18, comment_ref:1` — no title-change type appears)
- **0** milestone-change events dated on or after 2026-09-09, i.e. no milestone
  association was altered during the reboot window
- comment strictly precedes close in every case

Applied order is confirmed by timestamps rather than asserted:

```
max leaf closed_at      2026-09-11T02:19:39+02:00
min tracker closed_at   2026-09-11T02:19:48+02:00   (#34, #85, #132)
max retired closed_at   2026-09-11T02:20:17+02:00
milestones closed       2026-09-11T02:20:19 … 02:20:28+02:00
```

Leaves, then trackers, then milestones — as documented.

Comment-count consistency: every one of the 106 reports `comments == 1` on the
issue object, and the timeline shows exactly one comment. That is internally
consistent with R07's own snapshot note `comment_bodies_captured: 0` — these
issues carried no discussion before retirement — and I reached it from the
timeline, independently of that file.

Original content is intact on inspection. Twelve spread across the range
(#17, #26, #41, #50, #60, #69, #80, #94, #105, #114, #125, #246) all retain
Generation 1 bodies of 673–2651 bytes in the original `- **Priority**: …` form,
original `created_at` (May–Sep 2026), and their original milestone:

```
#17  gate-v0.5  `service` stdlib module — OpenRC / sysvinit / launchd backends
#41  gate-v1.0  Blueprint applied-runs store (durable)
#94  v0.x       Native package repositories — APT, DNF/YUM
#246 (none)     ci(test): flaky test job — 4 failures in 12 runs …
```

Pre-existing labels survived alongside the two new ones — the 106 still carry
`kind/feature`×100, `roadmap-backlog`×100, `source/v1x-backlog`×100,
`area/statemgmt`×33, `area/server`×21, `area/security`×16,
`kind/release-tracker`×3 and eleven more — and the repo still has all 30 labels.
No label was deleted.

Two of the 106 (#232, #246) have no milestone. Their timelines contain **no
milestone event at all**, so they never had one; nothing was removed.

### B-06 — Nothing was deleted from the tracker — PASS (INFO)

148 pull requests + 134 issues = 282, and the highest number in the repo is 282:

```
UNACCOUNTED numbers in range 1..282 (possible deletions): []
overlap between the issue set and the PR set: 0
```

Every integer from #1 to #282 resolves to either an issue or a pull request.
There is no hole, so no issue or PR was deleted.

### B-07 — No Generation 1 issue was left open — PASS (INFO)

The repository has exactly **14 open issues**, and all 14 carry `generation/2`.
`open_issues_count: 14` on the repo object agrees. There is no open issue
carrying `generation/1`, and no open issue that predates the reboot.

The 14 closed issues that do *not* carry `generation/1` are all closures that
predate the retirement run, dated 2026-05-28 → 2026-09-03: issues 6, 30–33, 79,
88, 89, 90, 96, 117, 160, 231 and 234. None carries a transition label. This is
the independent version of R07's blast-radius check, and it reconciles exactly
against milestone counters:

| Milestone | closed issues (API) | of which retired | closed earlier |
|---|---|---|---|
| gate-v0.5 | 19 | 15 | 4 |
| gate-v1.0 | 50 | 49 | 1 |
| v0.x | 47 | 40 | 7 |
| (none) | 4 | 2 | 2 |
| **total** | **120** | **106** | **14** |

### B-08 — All five Generation 1 milestones are closed and still exist; `v0.6.0` is correct — PASS (INFO)

`GET /milestones?state=all` returns **6**:

```
89201  'gate-v0.5' closed  open 0  closed 19  closed_at 2026-09-11T02:20:19+02:00
89204  'gate-v1.0' closed  open 0  closed 50  closed_at 2026-09-11T02:20:21+02:00
89207  'v0.x'      closed  open 0  closed 47  closed_at 2026-09-11T02:20:23+02:00
89210  'v1.x'      closed  open 0  closed  0  closed_at 2026-09-11T02:20:26+02:00
89213  'v2.x+'     closed  open 0  closed  0  closed_at 2026-09-11T02:20:28+02:00
142396 'v0.6.0'    open    open 13 closed  0
```

None deleted, all five closed, zero open issues on any. `v1.x` and `v2.x+` show 0
issues; that is not loss — they were roadmap placeholders that were never
populated, and issue #17's timeline still shows a 2026-06-19 move *through* `v1.x`
and back, which is history that a deletion would have destroyed.

`v0.6.0` holds 13 open / 0 closed, with a description scoped to the Generation 2
promise ("secure enrollment, targeted bounded command execution over NATS,
durable lifecycle, cancellation, and an auditable result").

### B-09 — All 14 Generation 2 issues exist as described; #279 is a pull request — PASS (INFO)

All 14 are **open**, all carry `generation/2`, all have 0 comments:

```
#268 kind/announcement  milestone: none   Keystone Core has restarted: Generation 1 is archived…
#269 kind/tracker       milestone v0.6.0  v0.6.0 — release tracker
#270-#278 kind/epic     milestone v0.6.0  P00 … P08
#280-#282 kind/epic     milestone v0.6.0  P09, P10, P11
```

That is 1 announcement + 1 tracker + 12 epics (P00–P11, no gaps), 13 of them on
`v0.6.0` and only the announcement off-milestone — exactly the shape claimed, and
it reconciles with the milestone's `open_issues: 13`.

**#279 is absent from the set because it is a pull request, not a skipped issue:**

```
GET /pulls/279 -> "docs(reboot): publish the Generation 2 planning state (R09)",
   state closed, merged true, head b874f72233ec, base main, merged_by shawnbutts
```

The R09 publication PR took number 279 in the shared numbering sequence between
the creation of #278 (15:52) and #280 (16:22). Benign.

Bodies carry machine-readable identity (`<!-- keystone-core-task: ANNOUNCE-reboot -->`),
which is the stable-ID scheme the plan requires rather than title matching.

### B-10 — Both historical releases are published, complete, and annotated exactly once — PASS (INFO)

```
GET /repos/.../releases?limit=100  -> 2
  id 9667643  v0.1.0  draft false  prerelease false  assets 20  body 3653 bytes
  id 10425107 v0.5.0  draft false  prerelease false  assets 20  body 2276 bytes
```

(A first query with `draft=true` returned 0; that is the Forgejo filter selecting
*only* drafts, not a missing release. Recorded so the number is not misread.)

**Assets: 20 each, 40 total, none missing.** Each set is 5 archives
(linux/darwin × amd64/arm64 + windows_amd64.zip) + 12 native packages
(`kscore-{agent,cli,server}` × {deb,rpm} × {amd64,arm64}) + `checksums.txt` +
2 SBOMs — which is precisely the inventory each release body itself describes.
Every asset's `created_at` is its **original** upload timestamp
(2026-05-28T02:03–02:32 for v0.1.0; 2026-06-27T21:11–21:20 for v0.5.0), and
`created_at`/`published_at` on the releases are unchanged from the original
publication. Nothing was re-uploaded. Assets are live: a real `-L` fetch of
`releases/download/v0.1.0/checksums.txt` and the v0.5.0 equivalent both return
200. Download counters retain historical values (e.g. `kscore-cli_0.1.0_linux_amd64.rpm` = 7).

**The archived notice appears exactly once in each body** (`body.count("Archived
and unsupported") == 1`), as a leading blockquote, followed by `---` and then the
**complete original release notes**, unaltered: v0.1.0 still carries its
"⚠ Unsigned release", Verification, Install and Getting started sections;
v0.5.0 still carries Highlights and Verification. Every link in both bodies
resolves 200 (RFC 0001 on `main`; `VERSIONING.md` and `ROADMAP.md` on `main`;
`CHANGELOG.md`, `GETTING-STARTED.md`, `RELEASE-PLAYBOOK.md` at the respective
tags).

Tags were not moved (B-04), so the releases still point at the commits they were
built from.

### B-11 — The record's account of the GitHub mirror is false: the mirror is live and does resolve the archive refs — HIGH

**Checked:** the transition record states, in two places, that the second forge is
dead:

- `manifest.json → archive.mirror.observed`: *"The GitHub mirror was last updated
  2026-09-06 at 0a476caaa, which predates every reboot commit. It is not syncing,
  so the archive refs do not resolve there."*
- `manifest.json → planning_state_publication.deferred.github_mirror`: *"Dead at
  0a476caaa (2026-09-06), before any reboot commit, so it still presents
  Generation 1 as current."*
- `REBOOT-EXECUTION-PLAN.md` R05/R06/R09 statuses repeat it and use it as the
  rationale for scoping the mirror out ("there is no second forge state to
  record").

**Found (live, unauthenticated `api.github.com` + `git ls-remote`):**

```
GET /repos/Spicer-Creek-Solutions-LLC/keystone-core
  pushed_at 2026-09-11T12:52:01Z      archived false     default_branch main

branches: archive/2026-09-pre-v0.6-reboot 93eb147f7fcc protected=True
          main                           47ae589b817f  protected=True
tags:     v0.5.0 0f4810cd1d3f | v0.1.0 8a48da100701
          archive-2026-09-pre-v0.6-reboot 93eb147f7fcc | archive/v0-final 7d21b848a6d9
git ls-remote: refs/tags/archive-2026-09-pre-v0.6-reboot -> d99de5715cc8 (^{} 93eb147f7fcc)
releases: 0
main@47ae589b = "Merge pull request 'feat(baseline): land the clean Generation 2 baseline (R08)' (#267)"
             committed 2026-09-11T11:26:12Z
```

The mirror pushed at **12:52Z on 2026-09-11**, which is *before* R09's own
verification run (`docs/transition/r09/verification.txt`, `2026-09-11T14:24:45Z`) —
so the statement was already false when it was written. The mirror is not at
`0a476caaa`; it is at the R08 clean baseline, one merge behind Codeberg `main`
(`7ca5522312`, the R09 merge). The archive branch **and** the annotated archive
tag object **do** resolve there, both branches report `protected: true`, and the
mirror presents the Generation 2 baseline tree, not Generation 1 code.

**Why it matters:**

1. A documented, load-bearing factual claim in the transition manifest is wrong,
   and it is the claim used to justify scoping a public forge out of R05, R06 and
   R09. That rationale is void: there *is* a second forge state, it is being
   written to by an automation nobody is currently tracking, and it was written to
   during the transition window.
2. The error is in the **safe** direction for preservation — the archive is more
   widely replicated than recorded, not less — so this is not a preservation
   failure. It is an accuracy failure in the audit trail, and it needs a manifest
   correction plus a decision about whether GitHub is in scope after all.
3. Consequences that follow from the mirror being alive (none of them fatal, all
   currently unmanaged): GitHub carries **0 releases**, so the plan's requirement
   to "annotate Codeberg **and GitHub** `v0.1`/`v0.5` release pages as
   archived/unsupported" has no GitHub target — but GitHub *does* publish the
   `v0.1.0`/`v0.5.0` tags with auto-generated source archives and no archived
   notice anywhere; and GitHub's own repo metadata still reads
   `"GitOps deploys it… state, audit, events, secrets. v0.x pre-stable."` with
   topics `nats, state-management, infrastructure-as-code, golang`. See B-12.
4. GitHub branch-protection *details* are not readable unauthenticated, so I can
   confirm `protected: true` on both branches there but not the specific rules.

**Recommended disposition:** correct `archive.mirror` and
`planning_state_publication.deferred.github_mirror` in the manifest to the
observed state, and either bring the mirror into scope (metadata + an archived
notice) or record an explicit, dated risk acceptance for a live public forge that
presents Generation 1 framing.

### B-12 — The repository's own description, topics and site link still present Generation 1 as current — MEDIUM

**Checked:** R09's task text requires, as a reviewed remote mutation, "update the
repository description, topics, and site link".

**Found (live, both forges):**

```
Codeberg description: "GitOps deploys it. We keep it running. A runtime operations
  control plane for Linux fleets — state, audit, events, secrets. v0.x pre-stable."
Codeberg topics: configuration-management, control-plane, day2-operations,
  fleet-management, gitops, golang, linux
Codeberg website: https://docs.keystone-core.io
GitHub description: identical string; homepage identical;
  topics: codeberg-mirror, control-plane, day-2-operations, fleet-management,
          gitops, golang, infrastructure-as-code, nats, state-management
```

Nothing was changed. This directly contradicts the repository's own announcement
issue #268, which says **"There is nothing to install"** — while the description
one line above it advertises `state, audit, events, secrets` and `v0.x
pre-stable`, i.e. a shipped pre-stable product. `golang` as a topic is also now
false at the tip (no Go module outside `tools/`).

The manifest **does** disclose this as deferred
(`planning_state_publication.deferred.repository_metadata`: *"PATCH /repos/…
returns 403 for this token. Description, topics and website are maintainer
actions; the maintainer elected to keep them, dropping only the 'v0.x
pre-stable.' clause."*). I confirmed the 403: the bot cannot PATCH the repo. But
note that **even the one edit the maintainer elected to make has not happened** —
`v0.x pre-stable.` is still in the live description on both forges.

It is *not* disclosed in `REBOOT-EXECUTION-PLAN.md`, whose R09 status lists only
three outstanding maintainer actions: "pinning #268, deploying the docs page, and
repairing the GitHub mirror". Repository metadata is missing from that list, so a
reader of the plan alone would conclude R09 shipped it.

**Why it matters:** this is the single most-read piece of forge state — it is the
one-line summary on the repo page, in search results, and on the mirror. It is
the reset half of "prove that preservation and reset both succeeded", and it has
not landed. Severity is MEDIUM rather than HIGH because the manifest records it
honestly as a deferred maintainer action and the token genuinely cannot perform
it; the fix is one PATCH by an owner plus a one-line correction to the plan's
outstanding list.

### B-13 — `docs.keystone-core.io`, the repository's linked site, still serves the Generation 1 documentation with no reboot notice — MEDIUM

```
GET https://docs.keystone-core.io/    -> HTTP 200, 111,515 bytes, <title>Keystone Core</title>
  nav: Getting Started · Using State Management · Authoring a Blueprint ·
       Authoring & Publishing a Module · Managing Secrets · Querying the Audit Log
       & Policies · GitOps Integration · HA Cluster Topology · CLI Reference ·
       Configuration Reference · API Reference · Coverage Gates · E2E VM Testing …
  contains "reboot": False   contains "Generation 2": False
  contains "unsupported": False   contains "superseded": False
GET https://keystone-core.io/         -> HTTP 200, a meta-refresh to the Codeberg repo (fine)
GET https://packages.keystone-core.io/ -> DNS does not resolve (the Generation 1
                                          package-repo landing page is gone, which
                                          satisfies that half of R09's requirement)
```

The site is the `website` field of both forge repos and is linked from the v0.5.0
release body. It presents the full Generation 1 feature documentation as current.
R09 requires "publish the reboot documentation site"; the replacement page exists
in the repository at `deploy/docs/site/index.html` (5,773 bytes) but has not been
deployed. The manifest discloses this
(`planning_state_publication.deferred.docs_site_deployment`), so it is a known
gap, not a hidden one — but it is live and public, and it is the most detailed
public artifact still presenting Generation 1 as the current product.

### B-14 — The plan asserts the announcement is pinned; it is not — LOW

```
GET /repos/.../issues/pinned -> []
GET /repos/.../issues/268    -> "pin_order": 0, "is_locked": false
```

`REBOOT-EXECUTION-PLAN.md` R09 status reads "fourteen issues — **a pinned reboot
announcement (#268)**…" and then, three sentences later, lists "pinning #268" as
an outstanding maintainer action. The second statement is correct; the first is
not. The manifest is consistent and correct
(`deferred.announcement_pin`: `POST /issues/268/pin returned 403`). Cosmetic
wording defect in one document, fully disclosed elsewhere. Effect in practice:
the announcement does not appear at the top of the issue list, so a visitor's
first impression of the tracker is twelve `P0x` epics with no context.

### B-15 — `kind/rfc` label was created but is used by nothing — LOW

R07 created three labels (`status/superseded`, `generation/1`, `kind/rfc`).
Usage census over all 134 issues: `status/superseded` 106, `generation/1` 106,
**`kind/rfc` 0**. Harmless, but it is an unused artifact of the retirement run;
either apply it or drop it when the tracker conventions are next revisited.

### B-16 — Pre-existing inaccuracies inside the preserved release notes were correctly left alone — INFO

The v0.1.0 body describes archives as `keystone-core_v0.1.0_{…}` while the actual
assets are `keystone-core_0.1.0_{…}` (no `v`), and it says "Signed releases begin
v0.2.0" while the v0.5.0 body says signing begins v0.8. Both predate the reboot.
Leaving them untouched is the **right** call — the notice says "nothing about this
release has been deleted or rewritten" and the edit was confined to prepending
the notice. Recorded so a later reader does not mistake them for reboot damage.

### B-17 — Assorted forge hygiene, all clean — INFO

- **0 open pull requests**; no leftover `reboot-r0*` branches on the forge —
  `git ls-remote --heads origin` returns only `main` and the archive branch.
- No issue-writing automation survives: `.forgejo/` contains only
  `workflows/reboot-baseline.yml` and issue templates; `grep` finds no
  `deps-outdated-issue` or issue-creating step. This is consistent with the two
  dependency-freshness issues (#160, #231, #232) having no successor — I confirmed
  independently that no issue with that title exists after #232's closure.
- The retirement comment's three commit-pinned links (RFC 0001,
  REBOOT-EXECUTION-PLAN.md, FUTURE-CAPABILITIES.md at `93eb147f7fcc`) all exist in
  the tree at that commit and all return HTTP 200 live. All 106 comments are
  therefore not silently broken.
- `archived: false` on the Codeberg repo is correct — Generation 2 continues there.
- The archive branch head still carries its historical `ci-full / …` statuses
  (53 of them). Harmless; the removed workflows cannot re-run.
- `main` has `required_approvals: 0` and `keystone-bot` has
  `user_can_push:false`/`user_can_merge:false`, so bot self-merge is impossible and
  merges are performed by the maintainer (`merged_by: shawnbutts` on #279). Not
  claimed either way in the record; noting it as observed protection shape.
- The archive tag's **signing subkey expires 2029-09-08** (primary 2036-05-29).
  Signatures made before expiry still verify afterwards, with a warning. For a ref
  intended to be permanent, worth knowing before someone reads a future warning as
  tampering.

---

## Claims I could NOT independently verify

These are as important as the findings above. Each is a claim in the transition
record that my access cannot confirm or refute. None of them is reported as a
pass.

1. **Tag protection on `archive-2026-09-pre-v0.6-reboot`.** `GET /tag_protections`
   is 403 and the tag read exposes no protection field. There is **no read-only
   API surface** through which a non-admin can observe tag protection on Forgejo.
   The manifest's own evidence is a maintainer's UI reading at
   2026-09-10T13:04:22Z. My substitute evidence is weaker but real: the tag object
   id is unchanged on Codeberg, on GitHub, and locally (B-04). **Unverified.**
2. **The specific protection *rules* on the archive branch and on `main`** —
   whether force-push is disabled, whether deletion is blocked, whether any
   user/team is whitelisted to bypass, and whether the rule is name-exact or a
   pattern. `GET /branch_protections` is 403. I can only observe the derived
   `protected: true` / `user_can_push: false` for *this* credential. The manifest
   records two negative probes (a rejected `git push --force` and a `403 branch
   protected` on DELETE) performed at R05; those are good evidence but they are
   the actor's own record, not my observation, and I did not repeat them because
   R10 is read-only. **Partially verified; the rule contents are unverified.**
3. **That the release bodies were edited and nothing else was.** Forgejo's release
   object exposes no `updated_at` or edit history. I can prove the assets are
   original (untouched upload timestamps, original download counts), that
   `created_at`/`published_at` are original, and that the notice appears exactly
   once above intact original notes — which is strong circumstantial evidence for
   "body only". I cannot prove *when* the body was edited or that no other body
   text was altered in the same edit. **Circumstantially verified; not provable.**
4. **That no issue body was silently edited during retirement.** Forgejo emits a
   timeline event for a title change but **not** for a body edit, and exposes no
   body edit history via the API. I verified 0 title changes across all 106 and
   that the bodies are substantial, era-appropriate Generation 1 content with
   original `created_at`. A body rewrite would leave no trace I can read.
   **Unverifiable by API.**
5. **That no comment was deleted before or during retirement.** Deleted comments
   leave no timeline residue. All 106 issues show exactly one comment (the
   retirement comment), which is internally consistent — but "these issues never
   had comments" and "their comments were removed" are indistinguishable from the
   outside. R07's own snapshot claims `comment_bodies_captured: 0`, which is a
   claim, not evidence. **Unverifiable by API.**
6. **The offline archive bundle** (`keystone-core-generation-1-93eb147f7.bundle`,
   SHA-256 `34989c1f…`, 84,189,066 bytes, 2,185 refs, 3,072 commits). It is held
   in maintainer custody and is outside my slice and my reach. **Not examined.**
7. **Push-mirror configuration and webhooks on Codeberg.** `GET /push_mirrors` and
   `GET /hooks` are both 403, so I cannot show *how* GitHub received the
   2026-09-11 push (B-11) — only that it did. Whether a Codeberg push mirror, a
   GitHub-side pull mirror, or a manual push is responsible is **unknown**, and
   that matters for whether the mirror will continue to track `main`.
8. **Whether GitHub's branch protections are equivalent to Codeberg's.**
   Unauthenticated GitHub returns `protected: true` on both branches but refuses
   the protection detail endpoints. **Partially verified.**
9. **Anything about a third forge or package repository.** `packages.keystone-core.io`
   does not resolve, which is consistent with retirement, but I cannot prove no
   other Generation 1 distribution endpoint remains published somewhere.

---

## Disposition against the R10 gate

The R10 acceptance bar is "no unresolved Critical or High finding unless the
maintainer records a time-bounded risk acceptance with owner and expiry".

- **CRITICAL: none.** No preservation failure, no data destruction, no
  unmergeable branch. Every ref, release, asset, issue, comment and milestone
  claimed to be preserved is present and reachable, verified against live APIs
  and a second forge.
- **HIGH: one — B-11.** It is an accuracy defect in the audit trail, not a
  preservation failure, and the error runs in the safe direction. It is
  resolvable with a manifest correction plus a scope decision on the GitHub
  mirror.
- **MEDIUM: three** — B-03 (tag protection unverifiable without an admin
  credential), B-12 (repository metadata still Generation 1 on both forges),
  B-13 (documentation site still Generation 1).
- **LOW: two** — B-14 (plan says "pinned"; #268 is not), B-15 (unused `kind/rfc`).

Slice B's verdict: **preservation succeeded and is independently demonstrable.
The reset succeeded inside the tracker and is independently demonstrable. The
reset has not yet landed on the project's public-facing surfaces** — repository
description and topics on two forges, the linked documentation site, and the
unpinned announcement — and the record's description of the second forge is out
of date.
