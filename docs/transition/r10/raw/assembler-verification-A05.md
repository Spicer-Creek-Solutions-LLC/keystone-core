# Assembler re-verification of A-05 / A-06 / A-07

Written by the assembling agent (which performed R05-R09) because these three
findings bear on its own work. Recorded here, in the raw directory, so the
reasoning is auditable rather than folded silently into a disposition.

## Method

The bundle was cloned with `git clone --mirror` into a temporary directory.
Every measurement below was taken with `GIT_NO_REPLACE_OBJECTS=1`, because
`refs/replace` makes git substitute the replacement commit transparently: a
naive `git log <old-sha>` reports the *new* commit's tree and can appear to
confirm either conclusion. That substitution is the reason two earlier attempts
at this question reached opposite answers.

## Measured

| | |
|---|---|
| Raw commit objects in bundle | 3268 |
| `rev-list --all` with substitution active | 3072 |
| `refs/replace` entries | 2173 |
| Old (pre-rewrite) commit objects actually present | 197 |
| `refs/replace` entries whose old object does not exist | 1976 |
| Distinct trees among the 197 | 197 |
| Those trees also present in live archive-branch history | 0 |

`git cat-file --batch-all-objects --batch-check` independently confirms 3268
commit objects, matching `rev-list --all` only when substitution is disabled.

## Disposition

**The manifest is wrong.** Its `post_snapshot_updates` correction states the
divergent commits are content-duplicates distinguished only by parentage. For
the 197 old commit objects that exist, every tree is absent from live history.
They are unique content and the bundle is their only custodian.

**Finding A-05 is also wrong, in the other direction.** It reports ~2175
commits held solely by the bundle. The true figure is 197. The other 1976
`refs/replace` entries name an old SHA whose object does not exist anywhere —
in the bundle, in the live repository, or on either forge. They are dangling
references, not preserved history.

**The manifest's 3072 is misleading rather than false.** It is what `rev-list
--all` prints by default. The raw object count is 3268.

## Why the earlier correction was wrong

The manifest's evidence was "120 sampled original/replacement pairs have
identical trees". That sampling was performed with replacement substitution
active, so both sides of each pair resolved to the same object. The comparison
could not have returned any other answer. A check that cannot fail is not
evidence — the same defect class as the `gofmt -l . | (! read)` guard and the
markdown-lint glob, and the third instance of it in this transition.

## A-06 and A-07

**A-06 confirmed.** The four commits `git filter-repo` removed on 2026-02-20
(`d3bd7bf16`, `4235b9012`, `27666ad35`, `d1198adc3`) are absent from the bundle
and from both forges. They existed only as unreachable objects in the working
repository on the development VM and would have been destroyed by any `git gc`.
They are now pinned under `refs/preserve/gen1-filtered-scratch/*` locally. That
ref namespace is not pushed; permanent disposition is a maintainer decision.

**A-07 confirmed.** `git ls-remote` against the GitHub mirror returns
`refs/heads/main` at `47ae589b8`, `refs/heads/archive/2026-09-pre-v0.6-reboot`
and `refs/tags/archive-2026-09-pre-v0.6-reboot` at `93eb147f7`. The mirror is
current to the R08 baseline. The claim that it is dead at `0a476caaa` is false
and was repeated in the manifest, the execution plan, pull request #279 and the
R09 status line.
