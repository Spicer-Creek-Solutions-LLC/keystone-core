# R10 Independent Audit — Domain A: Archive Integrity

**Auditor:** independent agent (did not perform R02–R09 work)
**Date:** 2026-09-11
**Scope:** archive refs and bundle, plus historical tag object IDs
**Evidence rule applied:** `docs/transition/manifest.json` and `docs/transition/*/`
were treated as the claims under audit, never as evidence. Every statement below
rests on git itself, the bundle file, gpg, the Codeberg API, or the GitHub API.

## Summary

Preservation succeeded. The Generation 1 tree is fully and independently
recoverable at `93eb147f7fcc559d31f2cce77d81e791f01673f8` from three separate
custodians (Codeberg, the GitHub mirror, and the offline bundle), the signed
annotated tag verifies, the bundle digest matches, and the bundle's archive-branch
commit set is byte-identical to the live remote's. No CRITICAL findings.

Two HIGH findings concern documented claims that are false, both in the
manifest's `archive.unique_history` block: the bundle carries an entire earlier
generation of project history that the manifest dismisses as "content-duplicate"
(it is not), and the four commits the manifest says are the bundle's uniquely
preserved material are in fact **absent** from the bundle.

## Method

All commands were run from a clean tree on `reboot-r10-transition-audit`.
Working copies were created under `/tmp/ksaudit/`. Nothing was pushed and no
destructive forge probe was performed.

### Refs on the remote

```
$ git ls-remote origin 'refs/heads/archive/*' 'refs/tags/archive*' 'refs/tags/v0.1.0*' 'refs/tags/v0.5.0*'
93eb147f7fcc559d31f2cce77d81e791f01673f8	refs/heads/archive/2026-09-pre-v0.6-reboot
d99de5715cc817fbcdb1654860cf92a33b1934d3	refs/tags/archive-2026-09-pre-v0.6-reboot
93eb147f7fcc559d31f2cce77d81e791f01673f8	refs/tags/archive-2026-09-pre-v0.6-reboot^{}
2f3c81f6cb4592e7e909a14d8fb9695a1f33aae4	refs/tags/archive/v0-final
7d21b848a6d938c821cb36c3d00621c86253afeb	refs/tags/archive/v0-final^{}
318c19f2c806580f4041911e644d4efc6143a7f2	refs/tags/v0.1.0
8a48da100701839a23fd2da794ea548cdf0bfe0e	refs/tags/v0.1.0^{}
7003d8ddda6dfb12fc3d4cc6eff439ed8c919df2	refs/tags/v0.5.0
0f4810cd1d3f51b770f4451b60fa449ea018b8b5	refs/tags/v0.5.0^{}
```

### Tag object and signature

```
$ git cat-file -t d99de5715cc817fbcdb1654860cf92a33b1934d3
tag
$ git cat-file -p d99de5715cc817fbcdb1654860cf92a33b1934d3 | head -4
object 93eb147f7fcc559d31f2cce77d81e791f01673f8
type commit
tag archive-2026-09-pre-v0.6-reboot
tagger Keystone-core bot <keystone-bot@keystone-core.io> 1789039246 +0000

$ git verify-tag archive-2026-09-pre-v0.6-reboot ; echo EXIT=$?
gpg: Signature made Thu 10 Sep 2026 11:20:46 AM UTC
gpg:                using EDDSA key D1D65D8F57A31599F634E40023583C11192A9DF4
gpg: Good signature from "Keystone-core Bot <keystone-bot@keystone-core.io>" [ultimate]
gpg:                 aka "Keystone-core Bot <bot@keystone-core.io>" [ultimate]
EXIT=0

$ git verify-tag --raw archive-2026-09-pre-v0.6-reboot
[GNUPG:] GOODSIG 23583C11192A9DF4 Keystone-core Bot <keystone-bot@keystone-core.io>
[GNUPG:] VALIDSIG D1D65D8F57A31599F634E40023583C11192A9DF4 2026-09-10 1789039246 0 4 0 22 10 00 18913D0E53BA40B100C3086A7796A98BEA4F4647
[GNUPG:] TRUST_ULTIMATE 0 pgp
```

### Bundle digest and structure

```
$ cd ~/keystone-archive && sha256sum -c keystone-core-generation-1-93eb147f7.bundle.sha256
keystone-core-generation-1-93eb147f7.bundle: OK
$ stat -c '%s' keystone-core-generation-1-93eb147f7.bundle
84189066

$ git bundle verify <bundle>   # summary lines
The bundle contains these 2186 refs:
The bundle records a complete history.
The bundle uses this hash algorithm: sha1
VERIFY_EXIT=0            # no prerequisites reported -> self-contained
```

Ref breakdown (`git bundle list-heads`, 2186 lines including `HEAD`):

| namespace | count |
|---|---|
| `refs/replace` | 2173 |
| `refs/heads` | 6 |
| `refs/tags` | 4 |
| `refs/remotes` | 2 |
| `HEAD` | 1 |
| **total excl. HEAD** | **2185** |

### Independent mirror clone

```
$ git clone --mirror ~/keystone-archive/keystone-core-generation-1-93eb147f7.bundle bundle-mirror.git
$ git fsck --no-progress ; echo FSCK_EXIT=$?
FSCK_EXIT=0                       # no output, no dangling/broken objects

$ git rev-parse refs/heads/archive/2026-09-pre-v0.6-reboot
93eb147f7fcc559d31f2cce77d81e791f01673f8

$ git cat-file --batch-all-objects --batch-check='%(objecttype)' | sort | uniq -c
  18813 blob
   3268 commit
      4 tag
  17664 tree
$ git count-objects -vH
in-pack: 39749 ; size-pack: 81.15 MiB
```

### Bundle vs. live remote

A clean bare repo was fetched directly from `ssh://git@codeberg.org/...` to avoid
relying on my own working clone.

```
$ git init --bare remote.git && git fetch origin \
    '+refs/heads/archive/*:refs/heads/archive/*' '+refs/tags/*:refs/tags/*'

bundle archive commits: 768
remote archive commits: 768
only in bundle: 0
only in remote: 0
cmp of sorted commit lists: IDENTICAL
```

### Recoverability

```
$ git ls-tree -r --name-only 93eb147f7 | wc -l      -> 1962
$ git ls-tree -r --name-only 93eb147f7 | grep -c '\.go$'  -> 1545
$ git show 93eb147f7:go.mod | head -3
module go.keystone-core.io/keystone-core
go 1.27.1

$ git clone --branch archive/2026-09-pre-v0.6-reboot <bundle> /tmp/ksaudit/recover
HEAD=93eb147f7fcc559d31f2cce77d81e791f01673f8
files on disk (excl .git): 1962
du: 20M
```

### Forge protection (read-only)

```
GET /repos/.../branches/archive%2F2026-09-pre-v0.6-reboot   -> HTTP 200
  protected = True, user_can_push = False, user_can_merge = False
  commit = 93eb147f7fcc559d31f2cce77d81e791f01673f8
GET /repos/.../branches/main (control)                      -> protected = True
GET /repos/.../branch_protections                           -> HTTP 403
GET /repos/.../tag_protections                              -> HTTP 403
GET /repos/.../tags/archive-2026-09-pre-v0.6-reboot         -> HTTP 200
  id = d99de5715cc817fbcdb1654860cf92a33b1934d3
  commit = 93eb147f7fcc559d31f2cce77d81e791f01673f8
  keys = [archive_download_count, commit, id, message, name, tarball_url, zipball_url]
```

### Secret scan

```
$ GIT_NO_REPLACE_OBJECTS=1 gitleaks detect --source bundle-mirror.git \
    --config <.gitleaks.toml from 93eb147f7>
findings: 0

# same scan with gitleaks default rules (no repo config):
findings: 291  (249 generic-api-key, 26 curl-auth-header, 13 private-key, 3 stripe-access-token)
```

---

## Findings

### A-01 — PASS — Both archive refs exist and resolve to the claimed target

**Checked:** `git ls-remote origin` against Codeberg, and `git ls-remote` against
the GitHub mirror.

**Found:** `refs/heads/archive/2026-09-pre-v0.6-reboot` =
`93eb147f7fcc559d31f2cce77d81e791f01673f8`. The tag
`refs/tags/archive-2026-09-pre-v0.6-reboot` is tag object
`d99de5715cc817fbcdb1654860cf92a33b1934d3` dereferencing (`^{}`) to the same
commit. Identical object IDs on both forges.

**Why it matters:** the core preservation claim of R05 is true and holds on two
independent forges.

### A-02 — PASS — The tag is a genuine annotated tag with a valid signature from a sensible identity

**Checked:** `git cat-file -t`, `git cat-file -p`, `git verify-tag`,
`git verify-tag --raw`, `gpg --list-keys`.

**Found:** object type is `tag` (not a lightweight ref). Signature status is
`GOODSIG` / `VALIDSIG`, exit 0. Signed by EDDSA signing subkey
`23583C11192A9DF4` under primary `18913D0E53BA40B100C3086A7796A98BEA4F4647`,
uid `Keystone-core Bot <keystone-bot@keystone-core.io>`. This matches the
manifest's `archive.tag.signing_key` and the `signing_preflight.signing_subkey`
exactly. The signature timestamp (2026-09-10 11:20:46 UTC) precedes the bundle
file mtime (2026-09-10 11:21:11 UTC) by 25 seconds, consistent with the tag being
created and then bundled in one sitting. The tag body's self-described `target`
matches its actual `object` line.

**Caveat (not a defect):** gpg reports `[ultimate]` trust because the key sits in
the maintainer's own keyring. That is self-asserted trust, not third-party
attestation. The cryptographic binding is sound; the identity binding rests on
the maintainer's keyring.

**Note:** the manifest records `signing_subkey_expires: 2027-06-01`, but the
keyring reports the signing subkey expires **2029-09-08**. The manifest value is
stale or wrong. Cosmetic, folded into A-11.

### A-03 — PASS — Bundle digest, verification and self-containment

**Checked:** `sha256sum -c`, `git bundle verify`, bundle header.

**Found:** recorded digest
`34989c1fb33a7028752263bf00c22c235ef9125bce89809674f1e9dc92610ac6` matches the
file byte-for-byte; size 84189066 bytes matches `archive.bundle.size_bytes`
exactly. `git bundle verify` exits 0 and reports "The bundle records a complete
history" with **no prerequisite lines**, so the bundle is self-contained and needs
no pre-existing repository to restore from.

### A-04 — PASS — The bundle's archive history is identical to the live remote's

**Checked:** `git clone --mirror` of the bundle, `git fsck`, and a set comparison
of `rev-list` output against a freshly fetched bare clone of the Codeberg remote.

**Found:** `git fsck` is clean (no output, exit 0). The mirror resolves the
archive branch to `93eb147f7fcc559d31f2cce77d81e791f01673f8`. Both the bundle and
the remote carry **768** commits on that branch, with **0** commits present in one
and absent from the other, in both directions. The sorted commit-ID lists are
byte-identical under `cmp`.

**Why it matters:** this is the substantive answer to "did preservation succeed".
The offline copy is not a partial or lagging snapshot of the archive branch.

### A-05 — HIGH — The manifest's "content-duplicate" correction is false; the bundle is the sole custodian of an entire earlier generation of history

**Checked:** reachability analysis of the 2175 commits in the bundle reachable
only via `refs/replace/*`, then tree-level and subject-level comparison against
both live lineages (`archive/2026-09-pre-v0.6-reboot` and the `archive/v0-final`
tag).

**The claim under audit** (`archive.unique_history`):

> "Their CONTENT, however, is not unique." … "the work they contain is preserved
> in the current history under different SHAs." … "republishing ~2000
> content-duplicate commits works against R08's minimal baseline while preserving
> nothing the current history and this bundle do not already hold."

**Found — the claim does not survive contact with the data:**

The bundle contains three root commits:

| root | date | author | subject | lineage |
|---|---|---|---|---|
| `b80082a5` | 2025-10-12 | sbutts | `Initial commit` | replace-only (orphan) |
| `308b46fd` | 2025-12-24 | shawnbutts | `chore: misc (6 commits)` | reachable from `archive/v0-final` |
| `14be1109` | 2026-05-05 | shawnbutts | `feat: v1.0 reboot — reset to MVP reconstruction baseline` | root of the archive branch |

The 2175 replace-only commits descend from `b80082a5` and span
**2025-10-12 → 2026-02-16**. The archive branch spans **2026-05-05 → 2026-09-09**.
These are disjoint date ranges and disjoint lineages.

Measured uniqueness against **both** live lineages combined:

| measure | value |
|---|---|
| distinct commit trees in the orphan history | 1966 |
| of those, matching no live commit tree | **1806** |
| distinct commit subjects in the orphan history | 2097 |
| of those, appearing nowhere in live history | **1900** |
| tree/blob objects in orphan history | 23179 |
| of those, absent from `archive/v0-final` | **12562** |

Sample of subjects that exist nowhere in the preserved history:

```
Add 5 new Kubernetes state modules
Add advanced Nginx configuration modules
Add API authentication with gRPC interceptors
Add atomic bootstrap with checkpoint and automatic rollback
Add automated bare metal discovery with profile matching
Add automatic fallback between attestation methods
```

The mechanism is visible in `.git/filter-repo/commit-map` and in the roots table:
the 2026-02-20 filter-repo run rewrote the Oct-2025→Feb-2026 history, and the
resulting lineage was later abandoned wholesale by the 2026-05-05 "v1.0 reboot"
reset. The forge's `archive/v0-final` tag preserves only **305** commits rooted at
a squashed `chore: misc (6 commits)` placeholder dated 2025-12-24; everything
before that date, and the per-commit granularity of the rest, exists **only in
this bundle**. I confirmed the remote does not hold it:

```
$ cd remote.git && git cat-file -t b80082a5ff22a765a50aba062da717a6ae3f50b5
fatal: git cat-file: could not get object info
```

**Why it matters:** the manifest's earlier revision said the bundle held "~2000
commits of otherwise-lost history". That statement was **substantially correct**,
and the recorded "correction" replaced it with a false one. The false version was
then used as the stated justification for the decision not to push the history to
the forge ("preserving nothing the current history and this bundle do not already
hold"). The decision may still be defensible on minimal-baseline grounds, but the
evidentiary basis recorded for it is wrong, and it understates the criticality of
the bundle: an 84 MB file on a dev VM is currently the **only** custodian of
roughly 1900 commits of project history. Combined with A-08, that is the most
material preservation risk in this domain.

**Recommendation:** correct `archive.unique_history`, re-record the decision on
its real grounds, and either push the orphan lineage to a dedicated ref or state
explicitly that the bundle is a sole-custody artifact requiring off-site backup.

### A-06 — HIGH — The four commits the manifest says the bundle uniquely preserves are not in the bundle

**Checked:** extracted the four removed commits from
`.git/filter-repo/commit-map` (the rows mapping to all-zeros), then tested for
their presence in the bundle mirror and in the local repo.

**The claim under audit** (`archive.unique_history.correction`):

> "What is genuinely unique to the pre-rewrite history is the four scratch-note
> commits and the old-to-new SHA mapping, **both of which the bundle holds**."

**Found — the bundle does not hold them:**

```
$ awk '$2 ~ /^0+$/ {print $1}' .git/filter-repo/commit-map
d3bd7bf16a72567b0542459147e65e5cdb7b7618
4235b9012c36478eff58e31a0c5c0b03c472a34d
27666ad35aafb7f250d4d1b8f198cc97a6223eec
d1198adc337e369cfac05557b8508b16c1bbd3c3

# in the bundle mirror:
d3bd7bf16a72567b0542459147e65e5cdb7b7618 missing
4235b9012c36478eff58e31a0c5c0b03c472a34d missing
27666ad35aafb7f250d4d1b8f198cc97a6223eec missing
d1198adc337e369cfac05557b8508b16c1bbd3c3 missing

# in the dev-VM working repo:
d3bd7bf16a72567b0542459147e65e5cdb7b7618 commit 1035
4235b9012c36478eff58e31a0c5c0b03c472a34d commit 710
27666ad35aafb7f250d4d1b8f198cc97a6223eec commit 1131
d1198adc337e369cfac05557b8508b16c1bbd3c3 commit 927
```

Their content (read from the local repo) is as described — `TUI_MONITOR_TEST_RESULTS.md`,
two `CHECKPOINT.md` updates and `SESSION-SUMMARY.md`, all dated 2025-12-26.

The cause is mechanical: a `refs/replace/<old>` ref is *named* for the old SHA and
*points at* the new one, so the old objects are unreachable and
`git bundle create --all` never packed them. All 2173 replace-ref names are
`missing` in the bundle; all 2173 are present in the local repo.

Worse, in the local repo these four objects are referenced by **nothing**:

```
$ git for-each-ref --contains d3bd7bf1...   -> (no output)
$ git reflog --all | grep -c d3bd7bf16a     -> 0
```

They are unreachable packed objects with no ref and no reflog entry, and there is
no `gc.pruneExpire` override configured. A single `git gc --prune=now` on the dev
VM deletes them permanently, and nothing else anywhere holds a copy.

**Why it matters:** the manifest names exactly one artifact as the bundle's unique
contribution, and that artifact is precisely what the bundle lacks. The
old→new SHA mapping half of the claim *is* satisfied (the replace ref names encode
it), but the commits are not. This is a false claim about the one thing the record
says was saved.

**Recommendation:** if these four commits are worth keeping, extract them now
(`git bundle create scratch.bundle <the four SHAs>` or a targeted ref) before any
gc runs; otherwise amend the manifest to state they were not preserved.

### A-07 — MEDIUM — The manifest's GitHub-mirror claims are false as of today; R05's acceptance is actually met on both forges

**Checked:** `git ls-remote https://github.com/Spicer-Creek-Solutions-LLC/keystone-core.git`
and the GitHub REST API.

**The claims under audit** (`archive.mirror`):

> "The GitHub mirror was last updated 2026-09-06 at 0a476caaa, which predates
> every reboot commit. **It is not syncing, so the archive refs do not resolve
> there.**"
> `acceptance_impact`: "R05's 'both forges resolve the refs' is met on Codeberg only."

**Found — the mirror has caught up and the archive refs resolve there:**

```
47ae589b817fe62d1027f5868286cbbf0541e362	refs/heads/main
93eb147f7fcc559d31f2cce77d81e791f01673f8	refs/heads/archive/2026-09-pre-v0.6-reboot
d99de5715cc817fbcdb1654860cf92a33b1934d3	refs/tags/archive-2026-09-pre-v0.6-reboot
93eb147f7fcc559d31f2cce77d81e791f01673f8	refs/tags/archive-2026-09-pre-v0.6-reboot^{}
2f3c81f6cb4592e7e909a14d8fb9695a1f33aae4	refs/tags/archive/v0-final
318c19f2c806580f4041911e644d4efc6143a7f2	refs/tags/v0.1.0
7003d8ddda6dfb12fc3d4cc6eff439ed8c919df2	refs/tags/v0.5.0

GitHub API: pushed_at = 2026-09-11T12:52:01Z, archived = false
```

Every object ID is identical to Codeberg's, including the signed tag object, so
the mirror is a faithful second custodian rather than a lossy copy.

**Why it matters:** this is a false documented claim, but in the favourable
direction — preservation is *stronger* than recorded, and R05's "both forges
resolve the refs" criterion is in fact satisfied. It should be corrected before
R10 signs off, because an auditor reading the manifest would wrongly record an
unmet acceptance criterion. Note the manifest is internally inconsistent here too:
`mirror.pushed_at` says `2026-09-08T21:21:29Z` while `archive.mirror.observed`
says "last updated 2026-09-06".

### A-08 — MEDIUM — The orphan history is one routine `git gc` away from destruction in any recovered clone

**Checked:** compared what a plain `git clone` of the bundle yields versus a
`git clone --mirror`, then ran `git gc` on a throwaway copy.

**Found:** a plain clone transfers the whole pack (all 3268 commit objects are
physically present) but creates **no** `refs/replace/*` refs, leaving the 2175
orphan commits unreachable. A routine gc then deletes them:

```
$ git clone --branch archive/... <bundle> recover
$ git for-each-ref refs/replace | wc -l          -> 0
$ cp -a recover gctest && cd gctest
before gc: 3268 commits
$ git reflog expire --expire=now --all && git gc --prune=now
after gc:  1093 commits
$ git cat-file -t b80082a5...
fatal: git cat-file: could not get object info
$ git cat-file -t 93eb147f7...
commit                                  # archive target survives
```

`git clone --mirror` does fetch `refs/replace/*` and is gc-safe. The manifest
records `verified_by: "git clone --mirror into a temporary directory"`, so the
correct procedure was used — but it is recorded as an incidental description of
what was done, not as a **required** restore procedure.

**Why it matters:** combined with A-05, the material at risk is the ~1900 commits
that exist nowhere else. A future recoverer following the obvious path (plain
clone) would silently lose them on the first `git gc`.

**Recommendation:** document `--mirror` as mandatory for restoring this bundle.

### A-09 — MEDIUM — The bundle is documented only inside `manifest.json`; two of three custody locations are unverifiable

**Checked:** `grep -rln 'keystone-core-generation-1-93eb147f7.bundle\|keystone-archive' docs/ *.md`
and `grep -rn -- '--mirror' docs/`.

**Found:** both greps return `docs/transition/manifest.json` and nothing else.
`docs/transition/FREEZE.md` mentions the archive SHA and the R05 refs but never
the bundle. There is no restore runbook anywhere in the tree.

Custody is claimed as `["dev VM ~/keystone-archive/", "maintainer NAS", "maintainer workstation"]`.
I verified the dev VM copy (present, digest matches). The NAS and workstation
copies are outside my reach — see "Claims I could NOT independently verify".

**Why it matters:** an artifact that is the sole custodian of unique history
(A-05) is described only in a machine-readable inventory field, with its recovery
procedure (A-08) mentioned in passing and its redundancy unverified.

### A-10 — LOW — The recorded bundle commit count is understated by 196

**Checked:** `git cat-file --batch-all-objects --batch-check='%(objecttype)'`
versus `git rev-list --all` with and without `--no-replace-objects`.

**The claim:** `archive.bundle.commits: 3072` and
`archive.bundle.verification.commits_preserved: 3072`.

**Found:**

| measurement | value |
|---|---|
| physical commit objects in the pack | **3268** |
| `git rev-list --all` with replace refs **active** | 3072 |
| `git rev-list --all --no-replace-objects` | 3268 |

The 3072 figure is the artifact of running `rev-list` without
`--no-replace-objects`, which lets 196 commits be hidden behind their
replacements. The true number of preserved commit objects is 3268.

The companion ref counts are **correct**: `refs: 2185` matches exactly
(2173 + 6 + 4 + 2), with `git bundle list-heads` reporting 2186 because it also
lists `HEAD`.

**Why it matters:** low impact — it understates preservation rather than
overstating it — but it is a wrong number in the evidence record, and the same
methodological slip is what makes A-05's analysis unreliable.

### A-11 — LOW — Two further numeric/field inaccuracies

**Checked:** reachability set arithmetic; gpg key listing.

**Found:**

1. `archive.unique_history.finding` says "2172 commit objects are reachable only
   from local refs/replace/ mappings". The actual figure is **2175**: 2173 that
   are replace-ref values, plus 2 more reachable only as ancestors within the
   orphan DAG.
2. `signing_preflight.signing_subkey_expires` says `2027-06-01`. The keyring says
   the signing subkey `23583C11192A9DF4` expires **2029-09-08**
   (`sub ed25519/23583C11192A9DF4 2026-06-01 [S] [expires: 2029-09-08]`).

The `commit-map` statistics the manifest quotes are **correct**: I independently
counted 2173 rewritten, 16 unchanged, 4 removed.

### A-12 — PASS — Historical release tags are intact and match the R02 record exactly

**Checked:** `git ls-remote`, and `git cat-file -p` on each tag object fetched
directly from the remote.

| tag | recorded tag object ID (manifest `tags[]`, R02) | actual on remote | target commit recorded | actual target | match |
|---|---|---|---|---|---|
| `v0.1.0` | `318c19f2c806580f4041911e644d4efc6143a7f2` | `318c19f2c806580f4041911e644d4efc6143a7f2` | `8a48da100701839a23fd2da794ea548cdf0bfe0e` | `8a48da100701839a23fd2da794ea548cdf0bfe0e` | yes |
| `v0.5.0` | `7003d8ddda6dfb12fc3d4cc6eff439ed8c919df2` | `7003d8ddda6dfb12fc3d4cc6eff439ed8c919df2` | `0f4810cd1d3f51b770f4451b60fa449ea018b8b5` | `0f4810cd1d3f51b770f4451b60fa449ea018b8b5` | yes |
| `archive/v0-final` | `2f3c81f6cb4592e7e909a14d8fb9695a1f33aae4` | `2f3c81f6cb4592e7e909a14d8fb9695a1f33aae4` | `7d21b848a6d938c821cb36c3d00621c86253afeb` | `7d21b848a6d938c821cb36c3d00621c86253afeb` | yes |

Tagger strings and `signed: false` also match for all three; none carries a PGP
signature block, as recorded. All three object IDs are identical on the GitHub
mirror and inside the bundle.

Ancestry, verified in the bundle mirror:

```
v0.1.0 (8a48da10) IS ancestor of archive target
v0.5.0 (0f4810cd) IS ancestor of archive target
archive/v0-final (7d21b848) NOT ancestor of archive target
```

The last line is expected, not a defect: `archive/v0-final` belongs to the
pre-2026-05 lineage rooted at `308b46fd`, which the 2026-05-05 reset abandoned. It
does mean the archive **branch** alone does not preserve that generation — only
the `archive/v0-final` tag does. Worth stating plainly in the record, since
"the archive branch preserves Generation 1" is true only for the post-May-2026
lineage.

### A-13 — PASS — The Generation 1 tree is genuinely recoverable

**Checked:** content retrieval at the pinned commit from the **bundle** (not from
my working repo), plus a full working-tree clone.

**Found:** the tree at `93eb147f7` contains **1962 files** (2247 entries including
directories), of which **1545** are `.go` files. Real content reads back correctly:

```
$ git show 93eb147f7:go.mod | head -3
module go.keystone-core.io/keystone-core
go 1.27.1

$ git show 93eb147f7:README.md | sed -n '11p'
**GitOps deploys it. We keep it running.**

$ git show 93eb147f7:cmd/kscore-agent/bootstrap_test.go | head -3
// SPDX-License-Identifier: Apache-2.0
package main
```

Top level holds the expected Generation 1 surface: `cmd`, `internal`, `pkg`,
`modules`, `api`, `docs`, `epics`, `test`, `Makefile`, `go.mod`, `buf.yaml`,
`.goreleaser.yaml`. A real checkout produced 1962 files on disk, 20 MB.

**Why it matters:** this is the strongest single answer to "did preservation
succeed". It did, independently of the forge and independently of the manifest.

### A-14 — PASS — The "not a secrets rewrite" claim is reproducible, and my scan was broader than the one claimed

**Checked:** ran gitleaks myself over the **entire** bundle mirror with
`GIT_NO_REPLACE_OBJECTS=1`, so the orphan commits were included, using the
project's own `.gitleaks.toml` extracted from `93eb147f7`.

**Found:** **0 findings**. The manifest claims coverage of "2886 of 2904 non-merge
commits"; my scan covered all 3268 commit objects and also returned clean, so the
claim holds with margin.

For completeness I also ran with gitleaks' default rules and no project config,
which produced 291 findings. Every sampled finding is in a path the project
allowlists for legitimate reasons — `internal/identity/ca_envelope_test.go`,
`pkg/blueprint/registry/signing_test.go`, `pkg/module/verify/signer_test.go`,
`internal/secrets/kms/benchmark_test.go`, `docs/content/en/docs/reference/modules.md`
— i.e. test fixtures and documentation examples, not credentials.

**Minor caveat (LOW):** the allowlist path pattern `.*_test\.go$` is broad enough
that a genuine secret committed into any test file would be invisible to the gate.
Nothing I sampled suggests that happened; recording it as a known limit of the
scan rather than a finding against the archive.

### A-15 — LOW — The bundle carries dev-local refs and a stale `main`

**Checked:** `git bundle list-heads`.

**Found:** alongside the archive branch the bundle includes `refs/heads/main` at
`4d723af1` (the R04 merge, not current `main` `47ae589b8`), three stale reboot
topic branches (`reboot-r02-freeze-reconcile`, `reboot-r02-postcondition`,
`reboot-r03-capability-catalog`, `reboot-r05-archive-evidence`),
`refs/remotes/origin/*`, and `HEAD` pointing at `4d723af1`. This is the signature
of `git bundle create --all` from a working clone.

**Why it matters:** cosmetic and arguably beneficial (more preserved, not less),
but it means the bundle is a snapshot of one developer's clone rather than a
curated archive, and its `main`/`HEAD` will mislead anyone who clones it without
specifying a branch — a plain `git clone <bundle>` checks out `4d723af1`, not the
archive target. Worth one sentence in the restore procedure.

### A-16 — INFO — Branch protection observed; tag protection remains unobservable with this credential

**Checked:** read-only Codeberg API calls only. I deliberately did **not** repeat
the destructive force-push or delete probes recorded in the manifest.

**Found:** the archive branch reports `protected = true`, `user_can_push = false`,
`user_can_merge = false`, with `commit` still at `93eb147f7`, so the refs are
unchanged after the earlier probes. `main` reports the same.
`/branch_protections` and `/tag_protections` both return **HTTP 403**
(`"user should be an owner or a collaborator with admin write of a repository"`),
confirming the manifest's own stated limit. The tag read endpoint exposes no
protection field at all (`keys = [archive_download_count, commit, id, message,
name, tarball_url, zipball_url]`), again as the manifest states.

The manifest is **honest** about this distinction, which I note approvingly: it
explicitly separates branch protection (two observed negative probes) from tag
protection (a human reading a settings page).

**One control I could not reproduce:** the manifest argues the reading is
meaningful because "an unprotected branch reports protected=false". The remote has
only two branches and **both** are protected:

```
True   main
True   archive/2026-09-pre-v0.6-reboot
```

so there is no negative control available to me. The claim is consistent with
standard Forgejo behaviour, but I did not observe it.

### A-17 — INFO — The bundle is SHA-1; integrity rests on the external SHA-256

`git bundle verify` reports `The bundle uses this hash algorithm: sha1`. This is
the git default and unavoidable for a SHA-1 repository. Object-level integrity
therefore inherits SHA-1's weaknesses (mitigated by git's SHA-1DC collision
detection), and the real tamper-evidence for the archive is the external SHA-256
digest over the bundle file plus the GPG signature on the tag. The manifest's
`custody_note` ("The SHA-256 is the binding evidence") states this correctly.
Recorded so a later reader does not mistake the bundle format for a SHA-256 one.

---

## Claims I could NOT independently verify

1. **Custody at the maintainer NAS and the maintainer workstation.** The manifest
   lists three custody locations. I verified only `dev VM ~/keystone-archive/`,
   where the file is present and its digest matches. The other two are outside
   this environment. Redundancy of the bundle — which A-05 shows is the sole
   custodian of ~1900 commits — is therefore **unconfirmed**. Given A-05 and A-08,
   confirming an off-VM copy is the single most valuable follow-up in this domain.

2. **Tag protection on Codeberg.** `/tag_protections` returns HTTP 403 for this
   non-admin token, and the tag read endpoint exposes no protection field. The
   manifest rests this on a maintainer's UI reading at 2026-09-10T13:04:22Z. I can
   neither confirm nor refute it. This is a **verification limit of the credential**,
   not a pass. I did not attempt a delete probe, and I agree with the manifest's
   reasoning for not attempting one: an unblocked delete would destroy the signed
   tag object irrecoverably, and re-creating it would yield a different object ID.

3. **The negative control for branch protection** ("an unprotected branch reports
   protected=false"). No unprotected branch exists on the remote to test against
   (A-16).

4. **The manifest's "120 sampled original/replacement pairs have identical trees"
   and "150 pairs have identical author/committer/subject" evidence.** This is not
   reproducible from the bundle, because the bundle does not contain the original
   (pre-rewrite) commit objects at all — all 2173 replace-ref names are `missing`
   there (A-06). The sampling could only have been done in the dev-VM working
   repo, where the originals do survive as unreachable objects. I confirmed the
   originals exist locally, but I did not re-run the 120/150-pair sampling; note
   that even if those pairs are identical, it does not support the conclusion the
   manifest draws from it, for the reasons in A-05.

5. **Whether the maintainer's GPG key is registered with Codeberg** such that the
   forge displays the tag as verified. `signing_preflight.forge_verification` says
   this was resolved after the snapshot cutoff via `post_snapshot_updates`. The tag
   API response exposes no verification field, so I could not observe it.

6. **The GitHub mirror's protection posture.** The mirror holds correct copies of
   all archive refs (A-07), but I queried it unauthenticated and cannot see whether
   its archive branch or tag is protected against deletion or force-push.

---

## Bottom line for R10

Preservation **succeeded** and reset did not damage it: the refs resolve on two
forges, the signed annotated tag verifies, the bundle is intact and self-contained,
its archive history matches the remote exactly, and a full 1962-file Generation 1
working tree restores from the offline copy alone.

The defects are in the **evidence record**, not the artifact. The manifest's
`archive.unique_history` block is wrong in both directions — it dismisses ~1900
commits of genuinely unique history as duplicates (A-05), and it credits the
bundle with four commits it does not contain (A-06) — and its GitHub-mirror
section understates what was actually achieved (A-07). The operational risk those
errors conceal is that an 84 MB file with one confirmed copy is the sole custodian
of an entire earlier generation of this project, recoverable only by a `--mirror`
clone that is documented nowhere outside a JSON field (A-08, A-09).
