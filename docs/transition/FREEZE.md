# Generation 1 Freeze Record (R02)

This is the human-readable companion to
[`manifest.json`](manifest.json), the machine-readable inventory produced by
task R02 of the [Generation 2 reboot](../project/REBOOT-EXECUTION-PLAN.md).

Its purpose is to establish a known final Generation 1 commit and to record the
observed state of the repository at that point — accurately, including what is
broken. R02 preserves; it does not repair.

## Freeze declaration

Generation 1 is frozen as of this commit. From here:

- no feature work lands on the Generation 1 line without a new maintainer
  decision;
- the only changes expected on `main` are the remaining transition tasks
  R03–R10, as defined in the
  [execution plan](../project/REBOOT-EXECUTION-PLAN.md); and
- the historical tags `v0.1.0`, `v0.5.0`, and `archive/v0-final` are never
  moved, recreated, or deleted.

There is no merge automation to disable: the repository has no auto-merge, no
merge queue, no Dependabot, and no Renovate. Merges are performed manually by
the maintainer. The one piece of scheduled automation that writes to the
tracker is recorded under [Issue-writing automation](#issue-writing-automation)
below.

## The archive target

`generation_1_final_sha` is the **merge commit that landed the R02 freeze pull
request (#259) on `main`** — the true final Generation 1 state of the branch:

```text
93eb147f7fcc559d31f2cce77d81e791f01673f8
```

That SHA could not be known from inside the commit it names, so the freeze
commit carried `generation_1_final_sha: null` and this postcondition commit
records the value. All four signed R02 commits — `388b25990`, `98d7a2f1e`,
`0f1967f68`, `35e10d644` — are ancestors of it.

R05 creates `archive/2026-09-pre-v0.6-reboot` and
`archive-2026-09-pre-v0.6-reboot` at exactly this SHA, and re-verifies the tag
object IDs below against this manifest. Only transition tasks R03–R10 follow
it.

## Observed state at the snapshot cutoff

| | |
|---|---|
| Forge | `codeberg.org`, repository ID 1749683, not a mirror |
| Branch heads | `main` only |
| Open issues | 106 (confirmed by `X-Total-Count` and by full pagination) |
| Open pull requests | 0 |
| Milestones | gate-v0.5 15/4, gate-v1.0 49/1, v0.x 40/7, v1.x 0/0, v2.x+ 0/0 |
| Issues with no milestone | `#232`, `#246` |
| Releases | `v0.1.0` (2026-05-28) and `v0.5.0` (2026-06-27), 20 assets each |
| CI contexts | 19 — 12 `ci`, 7 `ci-full` |
| Tracked files | 1959 |

Milestone assignment accounts for 104 of the 106 open issues; the two
unassigned issues are listed above so R07's allowlist cannot silently omit
them.

### Historical tag object IDs

R05 must reproduce these exactly. All three are annotated and **unsigned**,
consistent with the Generation 1 policy of deferring release signing to `v0.8`.

| Tag | Tag object ID | Target commit |
|---|---|---|
| `v0.1.0` | `318c19f2c806580f4041911e644d4efc6143a7f2` | `8a48da100701839a23fd2da794ea548cdf0bfe0e` |
| `v0.5.0` | `7003d8ddda6dfb12fc3d4cc6eff439ed8c919df2` | `0f4810cd1d3f51b770f4451b60fa449ea018b8b5` |
| `archive/v0-final` | `2f3c81f6cb4592e7e909a14d8fb9695a1f33aae4` | `7d21b848a6d938c821cb36c3d00621c86253afeb` |

`archive/v0-final` is Generation 0 — the lineage that predates the 2026-05-05
reconstruction baseline. It is not part of this transition and must be left
untouched.

### Mirror

`github.com/Spicer-Creek-Solutions-LLC/keystone-core` is code-only: no issues,
labels, or milestones. Its tags resolve to the same commits as Codeberg's. At
the snapshot cutoff its `pushed_at` was `2026-09-08T21:21:29Z`, so it had not
yet received the R00 and R01 merges. R05 must confirm the mirror has caught up
before protecting refs on it.

## Signing preflight

Required by the execution plan, because RFC 0001 pauses the transition rather
than accept a weaker archive tag.

**Result: passes at the git level; forge verification is unavailable.**

- Key `18913D0E53BA40B100C3086A7796A98BEA4F4647`, uid
  `Keystone-core Bot <bot@keystone-core.io>`.
- Signing subkey `23583C11192A9DF4`, **expires 2027-06-01**. The primary key
  expires 2036-05-29.
- `git commit -S` produces a good signature (`%G?` = `G`) and
  `git verify-commit` accepts it.
- `git tag -s` produces a tag that `git tag -v` accepts.

The gap at the snapshot cutoff: the Codeberg account `keystone-bot` had **no
registered GPG key**. Its two verified addresses were
`keystone-bot@shawnbutts.com` and `keystone-bot@keystone-core.io`, neither of
which matches the key uid `bot@keystone-core.io`. Signatures were
cryptographically valid but would display as unverified.

This does not block R02. It does mean R05's archive tag would be signed but not
forge-verified, which weakens the visible integrity evidence exactly where the
RFC leans on it.

### Update after the snapshot

Resolved after the cutoff. The state recorded above is the snapshot, not the
current position.

Codeberg's HTTP 500 on email verification proved persistent, so the original
plan — add `bot@keystone-core.io` to the account and verify it — was abandoned
in favour of matching the key to an address the account had *already* verified:

- a second uid, `Keystone-core Bot <keystone-bot@keystone-core.io>`, was added
  to the key and made primary, **keeping** the original `bot@keystone-core.io`
  uid;
- the key was deleted and re-added on Codeberg — now id `154525` — because
  Forgejo reads a key's uid emails once at add time and offers no refresh, so
  a locally-added uid is invisible to it otherwise;
- Codeberg now reports the bound email `keystone-bot@keystone-core.io` as
  verified, where the previous registration had no bound email at all;
- the git committer email was changed to match; and
- the signing subkey expiry was extended from 2027-06-01 to **2029-09-08** in
  the same export, since changing expiry rewrites the subkey binding signature
  and would otherwise have required a second upload.

**Residual: none observed.** The concern that commits authored under
`bot@keystone-core.io` would display as unverified did not materialise.
Codeberg reports all three signed R02 commits — `388b25990`, `98d7a2f1e`, and
`0f1967f68` — as verified, with reason `keystone-bot / 23583C11192A9DF4`,
including the two committed under the still-unactivated address. Forgejo
matched the signature to the key's owner rather than requiring the committer
address to be a bound email.

That was checked against a control rather than assumed: unsigned commits on
this repository (`792940a57`, `a35bdaaf3`, `0a476caaa`) correctly report
`verified: false`, `reason: gpg.error.not_signed_commit`.

Note that the API omits the `verification` object entirely unless
`?verification=true` is passed, so its absence from a default response is not
evidence that a commit is unsigned:

```text
GET /repos/{owner}/{repo}/git/commits/{sha}?verification=true&files=false
```

`bot@keystone-core.io` was still deliberately left on the key as a uid. It
costs nothing and keeps the older committer identity associated with the key.

**R05 impact: cleared.** The archive tag must be created with tagger email
`keystone-bot@keystone-core.io`.

## Gate results

Run on the development VM under `go1.27.1 linux/amd64`, with tools rebuilt via
`make install-tools`.

**Passing (16):** `lint`, `docs-lint`, `docs-links`, `docs-sync-check`,
`clean-check`, `build`, `test-coverage`, `coverage-gate`, `slo`, `smoke`,
`security-secrets`, `security-vulns`, `security-sast`, `openapi-lint`,
`race-policy`, `goleak-policy`.

Notable detail:

- `test-coverage` runs `CGO_ENABLED=1 go test -race ./...` — 171 packages ok,
  0 failures, 71.0% total statement coverage. 135 of the 171 package results
  came from the Go test cache rather than a fresh execution.
- `coverage-gate`: 181 packages, 0 fails, 3 warns.
- `security-secrets`: gitleaks scanned 2864 commits, no leaks.
- `security-vulns`: govulncheck reports findings only in required modules whose
  vulnerable paths the code does not call.
- `security-sast`: gosec, 0 issues across 68 `nosec` annotations.

**Not runnable locally:** `test-integration` and `e2e-test` are Postgres- and
service-gated to CI; the `ci-full` cross-platform builds and release dry run
only trigger on push to `main`.

### Known red gate: `security-licenses`

`main` currently fails this gate. This is recorded, not fixed.

- **Observed locally:** exit 2, with `Package testing`, `go/ast`, and
  `go/parser` each reported as "does not have module info. Non go modules
  projects are no longer supported" (`google/go-licenses#128`).
- **Observed in CI:** `ci / security (push)` = **failure** on `82e1b9710`, the
  merge commit that landed R00 on `main`. The same job reported success on both
  reboot pull-request runs (`5ad5895d2`, `a35bdaaf3`) and on the previous main
  tip `0a476caaa`.
- **Not toolchain drift:** the installed binary is go-licenses v2.0.1 built
  with `go1.27.1` — exactly what the Makefile pins. `make install-tools`
  evaluated it as current and did not rebuild it.
- **Why the runs disagree:** the Makefile's own note at `install-tools` records
  that CI can pass while a runner caches an older v1 binary. A runner that
  resolves the pinned v2.0.1 under Go 1.27.1 fails to load stdlib module info.

This is a licence-**scanning** failure, not a licence violation: the scan
aborts before classification, so no forbidden, restricted, or unknown licence
was reported. It does not affect the archive target or the integrity of any
preserved ref, and it is not in R02's touched scope. It needs a maintainer
decision.

## Issue-writing automation

One live automation writes to the tracker:

- `.forgejo/workflows/ci-full.yml`, job `deps-outdated`, running
  `make deps-outdated-issue`.
- Triggers: `schedule` cron `0 5 * * *` (daily, 05:00 UTC), plus push to `main`
  and `workflow_dispatch`.
- Target: issue **#232**, "Dependency freshness: direct deps with updates
  available" — open, no labels, no milestone.

**Drift window.** R07 fails closed when an allowlisted issue's `updated_at`
does not match the reviewed snapshot. This job touches #232 every night, so the
snapshot above decays daily until **R06** disables the automation. R07's
allowlist must be regenerated from a snapshot taken *after* R06, or #232
excluded and handled separately.

No other issue-writing automation exists: no Dependabot, no Renovate, no other
scheduled workflow.

## Documentation reconciliation

R02's remit is to reconcile documentation with **observed release state and
known red tests** — correcting what is factually false about the past, without
adding product functionality. Repositioning the project's forward-looking
framing belongs to R08.

Three factual errors were corrected in `README.md`:

1. It claimed `v0.5.0` was "pending the release-signing ceremony". `v0.5.0` was
   released on 2026-06-27 with 20 assets. It ships unsigned under the
   `RELEASE-PLAYBOOK.md` §6 carve-out covering the line through `v0.7.x`.
2. The quickstart invited installation of `v0.1.x`. Per `VERSIONING.md`,
   `v0.5.x` is the formal external-tester milestone and the current line.
3. It stated the pre-reconstruction implementation "is not preserved in this
   repository". It is: the annotated tag `archive/v0-final` resolves to commit
   `7d21b848a` and is present on the forge.

Deliberately left unchanged, as R08 scope: the README's framing of 19
reconstruction epics as the active plan, the `v0.x pre-release` status badge
(still accurate), and `CHANGELOG.md`'s `[v1.0.0] — Planned` section.
`CHANGELOG.md` was reviewed for factual drift against observed release state
and needed no correction — its `v0.5.0` and `v0.1.0` sections and its
reference links match the published releases.

## What R02 did not do

No remote forge mutation was performed. Specifically: no branch or tag was
created, moved, or deleted; no protection setting was changed; no issue,
label, milestone, or project card was touched; no automation was disabled; and
no release was edited.

Those are R05, R06, and R07, each requiring a reviewed dry run and a separate
apply approval.

## Reproducing the snapshot

The inventory was assembled from `git for-each-ref`, `git cat-file`, the
Codeberg API (`/repos`, `/releases`, `/milestones`, `/issues`, `/pulls`,
`/commits/{sha}/statuses`), and the unauthenticated GitHub API for mirror
state. Issue counts were cross-checked two ways: the `X-Total-Count` response
header and a full pagination walk. Both report 106.
