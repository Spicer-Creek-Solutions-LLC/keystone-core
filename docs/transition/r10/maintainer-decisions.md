# R10 — maintainer decisions on audit findings

Each decision below was taken by the project maintainer against a finding in
[`audit-report.md`](audit-report.md). Recorded as taken, including the reasoning
given, so a later reader can tell a deliberate choice from an oversight.

## 1. `D-01` (Critical) — `docs.keystone-core.io` serves the Generation 1 site

**Decision: deploy the R09 page.** The web host's document root moves to
`deploy/docs/site/`, with the `try_files … /index.html` fallback already
described in [`../../../deploy/docs/README.md`](../../../deploy/docs/README.md)
so that all 100 indexed Generation 1 URLs resolve to the reboot announcement
rather than returning 404.

Rejected: taking the domain down, which kills 100 URLs without explaining
anything; and retaining the Generation 1 documentation behind an archived
banner, which would require restoring the Hugo toolchain R08 deleted in order
to regenerate the site.

**Maintainer context, recorded because it changes what the finding means.**
Nobody is running `v0.5.0` or any other release, and the site has no users. The
severity was assigned on the assumption of readers being misled. With no
readers, this is a defect in the accuracy of the public record rather than a
live hazard to an operator. The severity is left at Critical as the auditor
assigned it — the assembling agent does not reduce severities on the strength
of its own judgement — and the action is unchanged either way.

**Owner:** project maintainer. **Action is outside the repository:** the bot has
no access to the web host.

## 2. `A-06` (High) — the four `filter-repo` commits are not in the bundle

**Decision: correct the record; do not preserve the commits.**

The four commits `git filter-repo` removed on 2026-02-20 — `d3bd7bf16`,
`4235b9012`, `27666ad35`, `d1198adc3` — contain `CHECKPOINT.md`,
`TUI_MONITOR_TEST_RESULTS.md` and `SESSION-SUMMARY.md`, development checkpoints
written in December 2025 under the project's former name, TitanAnvil. They hold
no credentials or key material.

The maintainer confirms the removal was deliberate. The defect is therefore not
that the commits were lost but that `manifest.json` claimed the bundle preserved
them, which reversed the maintainer's intent on paper while doing nothing about
it in fact. The manifest is corrected to say they were deliberately removed and
are not preserved anywhere.

The `refs/preserve/gen1-filtered-scratch/*` refs created during the audit to
stop `git gc` collecting the objects have been deleted. They were a holding
action taken before the intent was known, never a preservation strategy.

Rejected: a supplementary bundle, which hedges against an intent the maintainer
has confirmed; pushing a protected ref, which would reverse a deliberate
decision in public and add a ref to a repository that has just been frozen; and
extracting the files into `docs/transition/`, which would return
TitanAnvil-branded notes to the tip of a repository that no longer carries that
name.

**Owner:** assembling agent, in this task.

## 3. `B-11` / `A-07` / `D-08` (High) — the GitHub-mirror record is false

**Decision: correct the record in place; add a correction comment to pull
request #279 rather than editing it; sync the mirror.**

Three statements were false and are corrected in `manifest.json`, each quoting
the text it replaces so the error stays legible: `mirror.observed`,
`ci_and_protection.github_mirror`, and
`planning_state_publication.deferred.github_mirror`. The R09 status line in
`REBOOT-EXECUTION-PLAN.md` is corrected too.

`docs/transition/FREEZE.md` needed no change. Its three `0a476caaa` references
describe CI statuses on the previous `main` tip, not the mirror.

The R05 and R06 status lines still read "GitHub mirror out of scope by
maintainer decision". That stands: the maintainer did decide to set the mirror
aside. Only the reason recorded for it — that the mirror was dead and there was
therefore no second forge state to record — was untrue.

Pull request #279 is merged and its description asserts the mirror is dead. A
correction comment was added and the description left unedited, matching how
this project has corrected everything else: the 106 retired Generation 1 issues
received a comment rather than an amended body.

**One internal contradiction is worth recording as a lesson.** `mirror.observed`
claimed the mirror was last updated 2026-09-06, two fields below
`mirror.pushed_at` of `2026-09-08T21:21:29Z` in the same object. Two values in
one record disagreed for two days and nothing compared them.

**Mirror sync is a maintainer action.** The mirror trails Codeberg by one merge
commit. `GET /push_mirrors` returns 403 to a non-admin token and the bot holds
no GitHub credential, so it cannot trigger or perform the sync.

**Owner:** assembling agent for the record; project maintainer for the sync.

## 4. `A-05` (High) — the bundle-uniqueness record is wrong

**Decision: record the corrected figures.**

Measured against the bundle with `GIT_NO_REPLACE_OBJECTS=1`, so `refs/replace`
does not substitute the replacement commit:

| | |
|---|---|
| Raw commit objects in the bundle | 3268 |
| `git rev-list --all --count` | 3072 |
| `refs/replace` entries | 2173 |
| Pre-rewrite commit objects actually present | 197 |
| `refs/replace` entries naming an object that does not exist | 1976 |
| Trees of those 197 found in live history | 0 |

The earlier correction called these commits content-duplicates. They are not:
the bundle is the sole custodian of all 197. Finding `A-05` reported roughly
2175, which overstates it about tenfold — the other 1976 are dangling
references, not preserved history. Both figures for the commit count are
correct; 3072 is what `rev-list` prints because substitution collapses a
replaced commit into its replacement, and the earlier record did not say which
number it was giving.

The superseded entry in `post_snapshot_updates` is marked as superseded rather
than removed.

**Owner:** assembling agent, in this task.

## 5. `C-02` / `C-03` (High) — documents asserting controls that do not exist

**Decision: delete all six.**

Removed:

- `.forgejo/PULL_REQUEST_TEMPLATE.md` — required `make test`,
  `test-integration`, `lint`, `docs-sync` and `changelog-new`, five of which no
  longer exist, plus a `.changes/unreleased/` fragment and a link to a deleted
  `COVERAGE-GATES.md`.
- `.forgejo/ISSUE_TEMPLATE/{bug_report,documentation,feature_request,security}.md`
  — solicited bug reports against installed binaries (`kscorectl --version`,
  `journalctl -u kscore-server`) for software with no supported release, and
  offered a feature-request form that contradicts `ISSUE-TRACKING.md`, under
  which an issue is created only when work has been accepted.
- `docs/project/SECURITY-GOVERNANCE.md` — asserted that four scans gate every
  pull request, that the Go toolchain is pinned in `go.mod`, that
  `make security-licenses` and `make security-vulns` run in CI, that a
  release dry-run runs per pull request, and that pull requests are labelled
  automatically. None is true and there is no root `go.mod`. Structurally it
  described a Security Working Group, Security Champions, a Security Response
  Team, KPIs and a dashboard — a product security programme for a
  one-maintainer project with no software.

Rejected: reconciling them, which means maintaining documents describing a
build that will not exist until P11; and the split option, which keeps a
minimal pull-request template. Templates are better written against a CI that
exists than rewritten twice.

**This partly reverses an R08 decision** to keep `SECURITY-GOVERNANCE.md`, taken
when its contents had not been read against the post-R08 reality. Recorded
explicitly rather than left implicit. `SECURITY-RELEASE.md` is retained.

Inbound references were updated in `SECURITY.md`, `docs/project/README.md` and
`AGENTS.md` § 7. One reference survives in `FUTURE-CAPABILITIES.md`, where three
archived Generation 1 backlog items quote the filename; it was a live markdown
link and is now a code span, since the quoted text describes where Generation 1
work would have landed rather than offering navigation.

**Note for decision 6.** `SECURITY.md` is now the only document describing how
to report a vulnerability. The disclosure route it gives must therefore work.

**Owner:** assembling agent, in this task.

## 6. `D-06` (High) — the vulnerability-disclosure route dead-ends

**Decision: one address everywhere — <security@keystone-core.io>, confirmed by
the maintainer as a real mailbox.**

Before: `SECURITY.md` offered "the repository's private security-report route"
or "the maintainer listed in `OWNERSHIP.md`". `OWNERSHIP.md` listed no address
and directed the reader to open an issue — the one thing `SECURITY.md` forbids.
`SECURITY-RELEASE.md` named four further addresses, a bug-bounty channel and
GitHub Security Advisories.

Changed:

- `SECURITY.md` names the address directly. The "private security-report route"
  claim is removed: Forgejo has no such feature and the claim could not be
  verified. It also said reports against "the transition tooling in `tools/`"
  are acted on; that tooling was removed at R09, so it now says
  `tools/capcheck`.
- `OWNERSHIP.md` leads its contact section with the security address and an
  explicit instruction not to open an issue.
- `SECURITY-RELEASE.md` collapses to the single channel. The bug-bounty and
  GitHub Security Advisories rows are removed as fictional, and the three-role
  contact matrix now says plainly that all three roles are the same person at
  the same address.

**The 24-hour response commitment is removed rather than reduced.** A published
response window that nobody is rostered to meet is worse than none, because it
is relied upon precisely when it fails. The text now says there is no
commitment, one maintainer, and no on-call rotation.

**Owner:** assembling agent, in this task.

## 7. `D-05`, `D-04`, `D-02` (High) — three false public claims

**`D-05` — `README.md` said the issue tracker is empty by design.** It has held
fourteen issues since R09 created them. Rewritten to describe what is there,
including why there are no leaf issues. R09 filled the tracker and did not
revisit the document that describes it.

**`D-04` — the announcement is documented as pinned and is not.**

**Decision: remove the claim rather than perform the pin.** The maintainer has
decided not to pin `#268`. `ISSUE-TRACKING.md` and `REBOOT-EXECUTION-PLAN.md`
no longer describe it as pinned.

This one is worth naming precisely, because the cause is not carelessness.
R09's plan called for a pinned announcement, the pin attempt returned 403, the
403 was written to R09's own apply log — and "pinned" was nonetheless written
into two documents and a pull-request description. An intention was recorded as
an accomplishment while the evidence to the contrary sat in the same task's
output. That is the same failure as a check that cannot fail, in a different
register: in both cases something was treated as established without the one
step that would have tested it.

**`D-02` — the forge descriptions still advertise Generation 1.** The
maintainer's decision, taken during R09, is to keep the description, topics and
website and drop only the trailing `v0.x pre-stable.` clause. That decision
stands. The finding is against the manifest, which phrased it in a way that read
as though the edit had been made; it now says the action is pending. `PATCH
/repos/...` returns 403 for this token, so the edit itself is a maintainer
action on both forges.

**Owner:** assembling agent for the documents; project maintainer for the forge
description.

## 8. `C-01` (High) — the link gate never saw `.forgejo/`

**Decision: derive the file list from git rather than from a glob.**

`make docs-links` now runs `git ls-files '*.md'` and pipes the result to lychee.
The auditor's raw reports are excluded by pathspec, for the reason recorded in
`.lychee.toml`.

lychee's `**/*.md` does not descend into dot-directories. markdownlint's
identical-looking glob does. Two gates were given what reads as the same
instruction and silently disagreed about what "every tracked file" meant, which
is how five tracked files came to have no link coverage at all — and how the
stale pull-request template survived unnoticed.

Adding `.forgejo/**/*.md` as a second pattern was rejected. It closes today's
gap and leaves the class of defect in place: the next dot-directory anyone adds
is silently uncovered again. Enumerating from git removes the assumption that
two tools interpret one pattern the same way.

**Verified rather than assumed.** A broken link was planted in
`.forgejo/PROBE.md` and both gates were run against the same tree:

| | Result |
|---|---|
| New git-derived list | 1 error — `.forgejo/NO-SUCH-FILE-R10-PROBE.md` not found |
| Old `"**/*.md"` glob | 0 errors |

The probe was then removed and the gate returns green. R09 shipped a glob fix
without testing it, which is why this one was tested.

One known limit: the file list is expanded by the shell via `xargs` rather than
by lychee. At 43 tracked markdown files this is immaterial.

**Owner:** assembling agent, in this task.

## 9. `D-14` (Medium) — R09 and R10 commits are unsigned

**Decision: write the convention down; accept the R09 gap.**

Nothing in this repository required commit signing. Not `AGENTS.md`, not
`CONTRIBUTING.md`, not `DCO.md`, not `GOVERNANCE.md`, not RFC 0001. R02 to R08
were signed by habit, R09 was not, and R10 is again. The finding exists because
the habit was consistent enough that breaking it was noticeable, while no rule
existed that could detect the break.

`AGENTS.md` § 4 now states the rule, including the exemption that makes it
workable: **forge merge commits are unsigned and are expected to be.** Both
`47ae589b8` and `7ca552231` — the R08 and R09 merges, made by the maintainer
through the web interface — carry no signature. Any rule without that exemption
would have been violated on the day it was written.

R09's commits stay unsigned. They are merged into `main`, and RFC 0001 commits
to the repository history remaining continuous, which rules out rewriting them.

Rejected: enforcing signing in CI, which would have to exempt the most common
commit type on the repository and could do nothing about R09 either way; and a
signed tag attesting to the end state, which the maintainer did not take up.

**Owner:** assembling agent, in this task.
