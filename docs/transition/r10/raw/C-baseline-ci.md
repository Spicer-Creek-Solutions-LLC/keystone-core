# R10 audit — slice C: baseline contents and CI enforcement

**Summary:** the baseline is exactly what the reboot claims it is, and every
gate provably fails when given the defect it exists to catch — but the
link-check gate has a 5-file blind spot over `.forgejo/`, and the stale
Generation 1 content sitting inside that blind spot (the pull-request template)
plus an unreconciled `SECURITY-GOVERNANCE.md` mean the repository still tells
contributors that CI enforces controls which no longer exist. No Critical
findings. Three High, five Medium.

Audited at `7ca552231` (tip of `main`, R09 merged), in an isolated worktree.
Every defect described below was planted, observed, and reverted; the final
worktree diff contains this file only.

---

## Method

### Environment

```text
worktree HEAD  7ca552231  Merge pull request 'docs(reboot): publish the
                          Generation 2 planning state (R09)' (#279)
go             go1.27.1 linux/amd64
lychee         0.24.2   (same version the workflow pins)
npx/node       v22.22.1 (nvm)  +  /usr/bin node v18.19.1 (system)
```

### Baseline: `make check` before any defect

```text
$ make check
...
🔍 207 Total (in 5ms) 🔗 98 Unique ✅ 148 OK 🚫 0 Errors 👻 59 Excluded
cd tools/capcheck && go run . ../..
capcheck: ok — 703 catalog entries, 1064 source items covered
check: ok
EXIT=0
```

### Gate-by-gate defect results

| Gate | Defect planted | Did it fail? | Verdict |
|---|---|---|---|
| `docs-lint` | trailing spaces in `.forgejo/ISSUE_TEMPLATE/bug_report.md` | Yes — MD009 | Works |
| `docs-lint` | `#Bad heading`, double blank lines, missing final newline in `epics/20-generation-2-reboot.md` | Yes — MD018, MD012 ×2, MD047 | Works |
| `docs-lint` | glob coverage audit (`git ls-files '*.md'` vs `Linting: N files`) | n/a | Complete: 42 of 43, `CLAUDE.md` deliberately ignored |
| `docs-lint` | unknown rule names `MD9999`/`MD0013` in config | **No** — clean pass | C-11 (fail-safe direction) |
| `docs-links` | broken relative link in `docs/project/GLOSSARY.md` | Yes — `File not found` | Works |
| `docs-links` | broken root-relative link `/docs/project/nope-zzz.md` | Yes | Works (`--root-dir` set) |
| `docs-links` | broken link in top-level `README.md` | Yes | Works |
| `docs-links` | broken link in `docs/adr/0000-adr-template.md` | Yes | Works |
| `docs-links` | broken link in `.forgejo/PULL_REQUEST_TEMPLATE.md` | **No — silently unchecked** | **C-01** |
| `docs-links` | broken fragment `./ROADMAP.md#this-anchor-does-not-exist-zzz` | **No** | **C-05** |
| `docs-links` | dead external URL | No — excluded by `--offline` | C-18 (by design) |
| `capability-catalog-check` | Source locator `FEATURES.md L51` → `L99999` | Yes — 2 problems | Works |
| `capability-catalog-check` | flip one entry `implemented` → `planned` | Yes — totals reconciliation | Works |
| `capability-catalog-check` | delete catalog row `CAP-FOUND-003` | Yes — 4 problems | Works |
| `capability-catalog-check` | rewrite an entry's Name column | **No — clean pass** | **C-04** |
| `capability-catalog-check` | falsify `text` of `SRC-0001` in the coverage map | Yes | Works |
| `capability-catalog-check` | delete `SRC-0001` from the coverage map | Yes — "not covered" | Works |
| `capability-catalog-check` | run with no git history reachable | Yes — exit 1, loud | Works (fails closed) |
| `stray-binary-check` | `go build -o capcheck .` in `tools/capcheck` | Yes — and propagates through `make check` | Works |
| DCO (CI) | commit with no `Signed-off-by` | Yes — exit 1, names the commit | Works |
| DCO (CI) | commit with malformed trailer (`Signed-off-by: NoEmailHere`) | Yes | Works |
| DCO (CI) | commit quoting a trailer in prose, never signed | **No — clean pass** | **C-09** |
| DCO (CI) | empty `BASE_SHA` | **No — "all 0 commits" pass** | **C-08** |
| DCO (CI) | unreachable `BASE_SHA` | Yes — exit 128 | Works |
| all | each target with tools removed from `PATH` | Yes — all non-zero | Works (no silent skip) |

### Selected command output

Markdown lint, planted defects in two directories including a dot-directory:

```text
$ make docs-lint
markdownlint-cli2 v0.23.2 (markdownlint v0.41.1)
Finding: **/*.md !CLAUDE.md
Linting: 42 files
Summary: 5 issues in 2 files
.forgejo/ISSUE_TEMPLATE/bug_report.md:55:43 error MD009/no-trailing-spaces ...
epics/20-generation-2-reboot.md:119:1 error MD018/no-missing-space-atx ...
epics/20-generation-2-reboot.md:121 error MD012/no-multiple-blanks ...
epics/20-generation-2-reboot.md:122 error MD012/no-multiple-blanks ...
epics/20-generation-2-reboot.md:123:21 error MD047/single-trailing-newline ...
make: *** [Makefile:26: docs-lint] Error 1
```

Link check, four broken relative links planted in four locations — only three
were found:

```text
$ make docs-links
[ERROR] .../nope-toplevel-zzz.md (at 94:1) | File not found. ...
[ERROR] .../docs/adr/nope-adr-zzz.md (at 184:1) | File not found. ...
Issues found in 2 inputs.
🔍 209 Total 🔗 100 Unique ✅ 148 OK 🚫 2 Errors 👻 59 Excluded
```

The `.forgejo/PULL_REQUEST_TEMPLATE.md` defect produced no error at all.
Confirmed with the checker's own input dump:

```text
$ lychee --dump-inputs --offline --config .lychee.toml --root-dir "$PWD" "**/*.md" | wc -l
38
$ git ls-files '*.md' | wc -l
43
$ comm -23 <(tracked) <(scanned)
.forgejo/ISSUE_TEMPLATE/bug_report.md
.forgejo/ISSUE_TEMPLATE/documentation.md
.forgejo/ISSUE_TEMPLATE/feature_request.md
.forgejo/ISSUE_TEMPLATE/security.md
.forgejo/PULL_REQUEST_TEMPLATE.md
```

Capability catalog, name-column defect:

```text
$ sed -i '85s|Cross-platform CLI builds|Totally invented capability that was never in Generation 1|' \
    docs/project/FUTURE-CAPABILITIES.md
$ make capability-catalog-check
capcheck: ok — 703 catalog entries, 1064 source items covered
D4_EXIT=0
```

DCO, real unsigned commits (created in this worktree, then `git reset --hard`
back to `7ca552231`):

```text
$ bash dco.sh 7ca5522312512122481dbc4c8cfbb1cac08e7432 HEAD
::error::DCO check failed: 2 commit(s) missing 'Signed-off-by:' trailer
  - 3d4dcbca5 probe: malformed trailer
  - 674641389 probe: commit with NO sign-off trailer
DCO_DEFECT_EXIT=1
```

DCO, degenerate ranges:

```text
$ bash dco.sh "" HEAD
DCO check passed: all 0 non-merge commit(s) carry a Signed-off-by trailer.
T2_EXIT=0
$ bash dco.sh 0000000000000000000000000000000000000000 HEAD
fatal: Invalid revision range 0000000000000000000000000000000000000000..HEAD
T3_EXIT=128
```

Baseline contents:

```text
$ git ls-files | wc -l
73
$ git ls-files '*.go'
tools/capcheck/main.go
tools/capcheck/main_test.go
$ ls go.mod go.work
ls: cannot access 'go.mod': No such file or directory
ls: cannot access 'go.work': No such file or directory
$ go build ./...
pattern ./...: directory prefix . does not contain main module or its selected dependencies
EXIT=1
$ go list -deps .            # in tools/capcheck
unsafe
go.keystone-core.io/keystone-core/tools/capcheck
```

Lychee install step replayed exactly as the workflow runs it:

```text
$ curl -sSL -o lychee.tgz https://github.com/lycheeverse/lychee/releases/download/lychee-v0.24.2/lychee-x86_64-unknown-linux-musl.tar.gz
HTTP=200 size=7705781
$ tar -xz -C fakebin --strip-components=1 "lychee-x86_64-unknown-linux-musl/lychee" < lychee.tgz
EXTRACT_OK
$ ./fakebin/lychee --version
lychee 0.24.2
```

---

## Findings

### C-01 — `docs-links` never sees `.forgejo/`; five tracked files have zero link coverage

**Severity: HIGH** (a gate silently covers only part of the repository — the
exact failure mode the project has already been burned by once)

**Checked:** planted an identical broken relative link in four locations —
repository root, `docs/adr/`, `docs/project/`, and `.forgejo/` — and ran
`make docs-links`. Then dumped the checker's actual input set with
`lychee --dump-inputs` and diffed it against `git ls-files '*.md'`.

**Found:** lychee's `**/*.md` glob does not descend into dot-directories.
It reads 38 files; 43 are tracked. The five it never opens are
`.forgejo/PULL_REQUEST_TEMPLATE.md` and the four
`.forgejo/ISSUE_TEMPLATE/*.md`. A deliberately broken link placed there
produced no error and no warning. Those five files contain eight real
relative links today (`../AGENTS.md`, `../../SECURITY.md`,
`../../docs/project/MAINTAINERS.md`, `../../OWNERSHIP.md`,
`../../docs/project/RFC.md`, `../docs/project/AI-CONTRIBUTIONS.md`,
`../docs/project/DCO.md`, `../CONTRIBUTING.md#changelog-entries`) — all of
which happen to resolve today, by luck rather than by gate.

**Why it matters:** the markdownlint config carries a comment explaining that
its previous narrower glob "left ten files unlinted, the active epic and the
issue templates among them, and nothing said so." That lesson was applied to
markdownlint (`**/*.md` there *does* match dot-directories, and I proved it by
planting an MD009 violation in `.forgejo/ISSUE_TEMPLATE/bug_report.md`) but
the identical-looking glob in the Makefile behaves differently for lychee, and
nothing says so. Two gates with the same written glob and different real
coverage is precisely the "passes for the wrong reason" shape R10 exists to
find. Fix: pass the inputs explicitly, e.g. `lychee ... $(git ls-files '*.md')`,
or add `.forgejo/**/*.md` alongside the existing glob, and re-run the planted
defect to confirm.

### C-02 — the pull-request template and issue templates are unreconciled Generation 1 documents

**Severity: HIGH** (documented claims are false, in the most-read operative
document in the repository)

**Checked:** read all five `.forgejo/` markdown files and
`.forgejo/ISSUE_TEMPLATE/config.yml` in full; cross-referenced every `make`
target they name against the Makefile, and every path they name against
`git ls-files`.

**Found:** `.forgejo/PULL_REQUEST_TEMPLATE.md` presents contributors with a
mandatory checklist that cannot be satisfied:

- "`make test` passes", "`make test-integration` passes", "`make lint` passes"
  — none of those targets exist. The Makefile has seven targets:
  `help`, `docs-lint`, `docs-lint-fix`, `docs-links`, `capability-catalog-check`,
  `stray-binary-check`, `check`.
- "Coverage targets per `docs/project/COVERAGE-GATES.md` met for any new
  package (critical >70%, CLI >40%)" — there are no packages and no coverage
  gate; P11 is supposed to set Generation 2's.
- "regenerated via `make docs-sync` if CLI flags / config keys / proto-defined
  RPCs / OpenAPI endpoints changed" — no such target, no such references.
- "Changelog fragment added under `.changes/unreleased/` (via
  `make changelog-new` ...)" linking to
  `../CONTRIBUTING.md#changelog-entries` — that section now reads, correctly,
  "There are none. Per-pull-request changelog fragments were removed with the
  Generation 1 code they described." The template and the document it cites
  directly contradict each other.
- "The `goheader` linter enforces `// SPDX-License-Identifier: Apache-2.0` on
  the first line of every new `.go` file" — there is no linter.
- `docs/runbooks/*.md` and `docs/project/{CLI,CONFIGURATION,API}-REFERENCE.md`
  are offered as doc surfaces to update; none exist at the tip.

`.forgejo/ISSUE_TEMPLATE/bug_report.md` asks reporters for
`kscorectl --version` output and `journalctl -u kscore-server` /
`journalctl -u kscore-agent` logs for software that cannot be installed.
`.forgejo/ISSUE_TEMPLATE/config.yml` directs people to
"`docs/project/GETTING-STARTED.md`, the CLI / Configuration / API references,
and the runbooks under `docs/runbooks/`" — the URL resolves (it points at
`.../src/branch/main/docs`) but every document it names is gone.

**Why it matters:** R08's stated outcome is a clean baseline with "no
installable claim." The PR template is the first thing a contributor reads and
it still describes Generation 1's gate structure as current. It is also the
single largest concentration of stale content in the tree, and it sits inside
C-01's blind spot — the two findings compound: the one directory no gate reads
is the one directory that was not updated.

### C-03 — `SECURITY-GOVERNANCE.md` asserts CI security controls that do not exist

**Severity: HIGH** (a normative source-of-truth document makes false
present-tense claims about enforcement)

**Checked:** `git log -- docs/project/SECURITY-GOVERNANCE.md` shows its last
change was R08's baseline commit `242830bfb`, which rewrote link targets to
archive URLs. Compared its prose against `.forgejo/workflows/reboot-baseline.yml`
and the Makefile.

**Found:**

- Line 202: "Four scans gate every PR via CI's `security` job. The same scans
  are runnable locally as `make security-secrets`, `make security-vulns`,
  `make security-sast`, `make security-licenses`." There is no `security` job
  — `reboot-baseline.yml` is the only workflow and its only job is `verify`.
  None of those make targets exist.
- Line 210, the gitleaks row: "scans full history (`fetch-depth: 0` in CI)".
  Nothing runs gitleaks. `.gitleaks.toml` is still tracked at the root with
  no consumer anywhere in the tree.
- Line 211: "Go toolchain pinned to `go 1.27.1` in `go.mod`" — there is no
  root `go.mod`.
- Line 334: "`make release-dry-run` builds the full goreleaser snapshot ...
  CI's `release-dry-run` job runs the full smoke (including container install)
  on every PR." No such target, no such job, no `.goreleaser.yaml`, no
  `scripts/`.

The document carries no note framing any of this as archived. Its links were
rewritten to the archive; its claims were not.

**Why it matters:** AGENTS.md §7 lists this file as the source of truth for
"Security policy and process," and `SECURITY.md` links to it. A reader — human
or agent — takes from it that four security scans currently gate every pull
request. They do not. This is the inverse of the gate problem: not a gate that
passes for the wrong reason, but enforcement claimed where no gate exists at
all. Either reconcile the document to the baseline, or mark the affected
sections explicitly as Generation 1 history the way `deploy/docs/README.md`
does ("there is no `make docs-site`, no theme, and no content tree").

### C-04 — the capability catalog's Name column is unvalidated

**Severity: MEDIUM**

**Checked:** rewrote row 85 of `docs/project/FUTURE-CAPABILITIES.md` from
`Cross-platform CLI builds` to `Totally invented capability that was never in
Generation 1`, leaving ID, status, scope and source locator untouched, and ran
the gate.

**Found:** `capcheck: ok — 703 catalog entries, 1064 source items covered`,
exit 0. Reading `tools/capcheck/main.go`, the Name field is only ever tested
for emptiness (`strings.TrimSpace(e.Name) == ""`) and used as the comparison
target for the "gap bullet restates its own name" heuristic. Everything else is
rigorously checked — the coverage map's `text` *is* diffed against the pinned
line, both directions of coverage completeness are enforced, locators are range
checked, totals are reconciled — but the one free-text field that states, in
human-readable form, what the archived capability actually was, is not.

**Why it matters:** the catalog's stated purpose is that "the reboot discards
the code without discarding what was learned." The Name column is the part a
human reads. A hand edit or a bad merge that changes what a capability claims
to be is exactly what this tool was written to catch, and it is the one edit
that slips through. The coverage map already stores the pinned source `text`
for each entry; comparing the catalog Name against it (fuzzily, as the coverage
text check already does) would close this with no new data.

### C-05 — link fragments are not checked, and one is already broken

**Severity: MEDIUM**

**Checked:** planted `[bad fragment](./ROADMAP.md#this-anchor-does-not-exist-zzz)`
and `[bad self fragment](#no-such-heading-zzz)`. Then wrote an independent
anchor resolver and ran it over all 43 tracked markdown files.

**Found:** both planted defects passed. `make docs-links` does not pass
`--include-fragments`, so lychee verifies only that the target *file* exists.
Of the five fragment links in the tree, one is already broken:

```text
docs/project/PROJECT-REBOOT-REVIEW.md:60
  [README](../../README.md#what-v10-commits-to)
```

`README.md` has eight headings — `Keystone Core`, `What happened`,
`Where Generation 1 went`, `What Generation 2 is`, `Where things are`,
`Contributing`, `Security`, `Licence`. There is no "What v1.0 commits to"
section; the reboot removed it, and the citation pointing at it was not
updated. The link renders, resolves to the file, and lands the reader nowhere
near the claim it was cited for.

**Why it matters:** small in isolation, but it is a live instance of a gate
reporting green over a broken link, in a document that is itself reboot
evidence. `--include-fragments` would catch it.

### C-06 — the markdown-lint gate is unpinned in two dimensions

**Severity: MEDIUM**

**Checked:** read the `markdown lint` step and `make docs-lint`; queried the
npm registry; ran the gate under the system Node 18 to observe the failure
mode.

**Found:** `make docs-lint` runs `npx --yes markdownlint-cli2` with no version
constraint, and the workflow contains no `actions/setup-node`. So the ruleset
the gate enforces is "whatever npm `latest` is at the moment the job runs"
(today 0.23.2, wrapping markdownlint 0.41.1), executed on "whatever Node the
runner image happens to ship." markdownlint-cli2 0.23.2 declares
`engines: { node: '>=22' }`; under Node 18 the gate does not lint, it crashes:

```text
$ env PATH=/usr/bin:/bin make docs-lint
SyntaxError: Invalid regular expression flags
Node.js v18.19.1
make: *** [Makefile:26: docs-lint] Error 1
```

That particular failure is loud, which is the good case. The quiet case is the
other direction: an upstream release that disables, renames or relaxes a rule
changes what `main` enforces with no commit to this repository and no failure
to notice.

**Why it matters:** the workflow author clearly understood pinning — the lychee
step pins the version, the target triple, and the archive member path, with a
comment explaining why ("Pinned rather than `:latest` so a new upstream release
cannot change the layout under us"). The same reasoning was not applied one
step earlier. Pin `markdownlint-cli2@0.23.2` in the Makefile and add
`actions/setup-node` with an explicit version.

### C-07 — CI runs neither `capcheck`'s tests nor `make check`

**Severity: MEDIUM**

**Checked:** grepped the workflow for `go test`, `make test` and `make check`;
enumerated the Makefile's targets.

**Found:** no match for any of the three. `tools/capcheck/main_test.go` is 191
lines with ten test functions — `TestParseCatalog`,
`TestCheckTotalsCatchesAFalsifiedSummary`, `TestEnumerateSourcesUnknownPin`
and others. They pass (`ok go.keystone-core.io/keystone-core/tools/capcheck
0.073s`), and nothing in CI or in any make target ever runs them. There is
also no `test` target, so a contributor following AGENTS.md §2 ("use `make`
targets rather than raw tool invocations") has no way to run them either.

Separately, CI invokes `docs-lint`, `docs-links` and `capability-catalog-check`
as three individual steps and never invokes `make check`. The `check` aggregate
is what AGENTS.md tells agents to run and what the repository documents as
"runs every gate," yet it is the one entry point CI does not exercise: if a
target were dropped from its prerequisite list, every CI step would still pass.

**Why it matters:** AGENTS.md §5 states that code changes require tests. The
only code in the repository has tests that no gate enforces, so a change to
`capcheck` that breaks them lands green. Adding a `make test` target that runs
`cd tools/capcheck && go test ./...`, wiring it into `check`, and having CI run
`make check` closes both halves.

### C-08 — the DCO check passes vacuously on an empty commit range, and is skipped on push

**Severity: MEDIUM**

**Checked:** extracted the step body verbatim and ran it under `bash -e` (the
Actions default shell) against real repository history with a valid range, an
empty `BASE_SHA`, an unreachable `BASE_SHA`, and `BASE == HEAD`.

**Found:** with `BASE_SHA` empty, `git log --no-merges --format=%H "..$HEAD"`
resolves to `HEAD..$HEAD` — an empty range. The loop reads nothing, `missing`
stays empty, and the step reports success:

```text
DCO check passed: all 0 non-merge commit(s) carry a Signed-off-by trailer.
```

An *unreachable* base fails loudly (exit 128), because the later
`total=$(git rev-list ...)` assignment propagates its failure under `set -e` —
so the script is protected there, but incidentally rather than deliberately.
The empty and degenerate cases are not protected.

Separately, the step carries `if: github.event_name == 'pull_request'`. The
workflow's comment explains this ("Skipped on push, where there is no base to
diff against"), which is reasonable, but it does mean the DCO requirement rests
entirely on branch protection actually forcing every change through a pull
request — something I could not verify from the repository.

**Why it matters:** the check is the sole mechanism enforcing AGENTS.md §4 and
`docs/project/DCO.md`. A guard for `[ -z "$BASE_SHA" ] || [ -z "$HEAD_SHA" ]`
that exits 1, and a guard that exits 1 when `total` is 0 on a pull request,
would remove both vacuous paths for three lines of shell.

### C-09 — the DCO grep matches quoted prose and never checks the signer's identity

**Severity: LOW**

**Checked:** created a commit authored by `Keystone-core bot` whose body quotes
a sign-off line as an example and carries no trailer of its own, then ran the
check.

**Found:**

```text
$ git commit --allow-empty -m "probe: quoted trailer only" -m "The DCO doc says you must add:
Signed-off-by: Some Example <example@example.com>
...but this commit itself was never signed off by its author."
$ bash dco.sh HEAD~1 HEAD
DCO check passed: all 1 non-merge commit(s) carry a Signed-off-by trailer.
```

`grep -qE '^Signed-off-by: .+ <.+@.+>$'` over `%B` matches any line in the body
with that shape — including a quotation, an example, or a pasted diff — and
never compares the signed-off identity to the commit author or committer.

**Why it matters:** the DCO's substance is that the person contributing
certifies origin under their own name. A body-text grep certifies only that a
string is present. This is a common weakness rather than a project-specific
one, and exploiting it requires deliberate effort, hence LOW — but
`git interpret-trailers --parse` plus a comparison against `%ae` is the correct
implementation and is not materially harder.

### C-10 — `.lychee.toml` is stale

**Severity: LOW**

**Checked:** compared every `exclude_path` entry and every comment reference
against the tree.

**Found:** `exclude_path` lists `docs/themes`, `docs/node_modules`,
`docs/resources`, `docs/public` and `docs/content` — all Generation 1 Hugo
paths, none of which exist. Its comments direct the reader to
`make docs-links-site` (no such target; `deploy/docs/README.md` explicitly
confirms the Hugo toolchain was removed at R08) and to
`docs/project/PUBLIC-LAUNCH-CHECKLIST.md` "Phase B6 + F-phase" (archived, not
at the tip). The `exclude` list still carries launch-era placeholder domains
including `keystone-community.slack.com` and `vault.internal`.

**Why it matters:** cosmetic today, since `--offline` makes most of the exclude
list moot anyway, but it is dead configuration that reads as live and will
mislead whoever next edits the gate.

### C-11 — markdownlint silently accepts unknown rule names

**Severity: LOW**

**Checked:** added `MD9999: false` and `MD0013: false` (a plausible typo for
`MD013`) to `.markdownlint-cli2.yaml` and ran the gate.

**Found:** `Summary: 0 issues in 0 files`, exit 0. Unknown keys in the `config`
block are ignored without warning.

**Why it matters:** the failure direction is safe — with `default: true`, a
typo'd disable simply leaves the rule enabled — so this cannot weaken
enforcement. It does mean the config's nine documented disables cannot be
assumed to be doing anything; `MD060` in particular is worth confirming against
markdownlint 0.41.1's actual rule set rather than trusting the line.

### C-12 — `stray-binary-check` only recognises one filename shape

**Severity: LOW**

**Checked:** `go build -o capcheck .` inside `tools/capcheck`, then ran the
target and `make check`; read the recipe.

**Found:** it works for the case it was written for —

```text
stray binary tools/capcheck/capcheck — remove it
make: *** [Makefile:39: stray-binary-check] Error 1
```

— and the failure propagates correctly through `capability-catalog-check` and
`make check` (`exit 1` inside the `{ ...; } || true` group terminates the
recipe shell; the `|| true` does not swallow it). But it only looks for
`tools/<dir>/<dir>`: a binary named anything else, a binary anywhere outside
`tools/*/`, or a nested module at a different path is invisible. Note also that
`/tools/capcheck/capcheck` is already in `.gitignore`, so the stated risk
("`git add -A` will happily commit it") is already mitigated by two mechanisms;
the gate's practical effect is to fail `make check` for a developer who ran
`go build` locally.

### C-13 — Generation 1 code paths survive in prose as present-tense claims

**Severity: LOW**

**Checked:** extracted all 224 backticked repository-path mentions from tracked
markdown and tested each for existence at the tip.

**Found:** 81 distinct paths do not resolve. The large majority are legitimate
— `docs/project/FUTURE-CAPABILITIES.md`, `docs/transition/FREEZE.md` and the
transition evidence describe the archive, and naming archived paths is their
job. Two files use them differently, as statements about a current codebase:

- `docs/project/GLOSSARY.md` — "Implemented via etcd epoch + lease watch in
  `internal/cluster/fencing.go`" (line 179), plus `internal/secrets/transit.go`,
  `internal/secrets/leasedirectory.go`, `internal/statemgmt/runner_saga.go`.
  Its own header says it defines terminology "used throughout the Keystone Core
  documentation and codebase."
- `docs/project/SECURITY-GOVERNANCE.md` — `.goreleaser.yaml`,
  `scripts/release-smoke.sh`, `scripts/release-smoke-container.sh`,
  `checksums.txt` (see C-03).

Backticked paths are not links, so no gate can see them; the archive-URL
rewrite at R08 reached only markdown link targets.

**Why it matters:** it is the mechanism behind C-03 and it explains why the
"every archive reference resolves" claim, while true, is narrower than it
sounds: it covers link-form and `git show` references, not prose.
`docs/project/REQUIREMENTS-TRACEABILITY.md` is the counter-example done right —
it names ten `test/e2e/docker/*_test.go` paths that do not exist, but the
column is headed "Planned automated evidence" and the preamble says "The test
paths are targets for the clean baseline." That framing is what GLOSSARY.md and
SECURITY-GOVERNANCE.md lack.

### C-14 — baseline contents are exactly as claimed

**Severity: INFO** (verification, no defect)

- 73 tracked files. Two `.go` files, both under `tools/capcheck`.
- No root `go.mod`, no `go.work`. `go build ./...` at the root exits 1:
  `pattern ./...: directory prefix . does not contain main module or its
  selected dependencies`. Nothing surprising happens.
- No tracked path matching `internal/`, `cmd/`, `pkg/`, `*.proto`, Dockerfile,
  chart or values files.
- `tools/capcheck` is genuinely stdlib-only: `go.mod` has zero `require`
  directives, there is no `go.sum`, and `go list -deps .` outside the standard
  library returns only `unsafe` and the module itself. It builds, vets clean,
  and its tests pass.
- No tracked file is also gitignored, so markdownlint's `gitignore: true` skips
  nothing that matters.
- R08's recorded status, "1,911 files removed, 79 remain," reproduces exactly:
  `242830bfb~1` has 1989 tracked files, the commit deletes 1911 and adds 1, and
  the R08 merge `47ae589b8` has 79. R09 took it to the present 73.
- The CI claim holds: `.forgejo/workflows/reboot-baseline.yml` is the only
  workflow in the tree, with a single job `verify`. Every make target it
  references exists.

### C-15 — every archive reference resolves, by object and by URL

**Severity: INFO** (verification, no defect)

The tag `archive-2026-09-pre-v0.6-reboot` is annotated (`d99de5715`) and
dereferences to commit `93eb147f7fcc559d31f2cce77d81e791f01673f8`, which is the
SHA recorded in `docs/transition/manifest.json` and compiled into
`capcheck`'s `generationOneFinalSHA`. The tree at that tag holds 1962 files.

All 23 distinct archive paths referenced anywhere in tracked files — extracted
from both the `src/tag/<tag>/<path>` URL form and the
`git show <tag>:<path>` form in AGENTS.md — resolve there as a file or a
directory. None broken.

I also resolved every external URL in the tree by hand, since the gate cannot
(C-18). All 27 archive-tag URLs return 200, as do all `src/branch/main/...`
URLs, `docs.keystone-core.io`, `go.keystone-core.io` (including the
`?go-get=1` vanity path), both release pages, and every third-party reference.
The only non-200s are extraction artifacts (`{/dir}` templates in
`deploy/vanity/README.md`, `${LYCHEE_VERSION}` from the workflow YAML, an
illustrative `docs.keystone-core.io/some/deep/path`) and one 429 rate-limit
from hashicorp.com.

### C-16 — the lychee install step is correct and genuinely pinned

**Severity: INFO** (verification, no defect)

Downloaded the exact pinned artifact and replayed the extraction verbatim. The
tarball does contain a directory, the member path and `--strip-components=1`
are both necessary and correct, and the result is a working
`lychee 0.24.2` — matching what the `--version` line in the step asserts. The
step sets `set -euo pipefail`, so a failed download cannot silently produce an
empty file. The pin is by release tag only, with no checksum, so it trusts the
upstream tag to remain immutable; that is a reasonable posture to note rather
than a defect.

`actions/setup-go@v5` reads `go-version-file: tools/capcheck/go.mod`, which
declares `go 1.27`. The leading comment block in that file does not interfere
with the directive. The step ordering is correct: setup-go precedes the
`capability catalog` step that needs it.

### C-17 — no gate is silently skipped when its tool is missing

**Severity: INFO** (verification, no defect)

Ran every target with a stripped `PATH`:

```text
docs-lint   → Error 1 (crashes loudly under Node 18; see C-06)
docs-links  → "ERROR: docs-links needs lychee on PATH" → Error 1
capability-catalog-check → "go: not found" → Error 127
make check  → Error 2
```

The `command -v X >/dev/null || { echo ...; exit 1; }` guards in the Makefile
are correct: they exit non-zero rather than skipping. `capcheck` likewise fails
closed when the pinned history is unreachable, which I confirmed by running it
against a directory containing the two input artifacts but no git history:

```text
capcheck: enumerate sources: git show 93eb147f7:FEATURES.md: exit status 128
NO_HISTORY_EXIT=1
```

The `fetch-depth: 0` in the workflow is therefore load-bearing and correctly
documented in both the workflow and the Makefile comments.

### C-18 — `--offline` means no external link is ever verified

**Severity: INFO** (by design, but worth recording)

`make docs-links` passes `--offline`, which makes lychee skip every `http`,
`https` and `mailto` link and report them as `[EXCLUDED]` — including URLs that
appear nowhere in `.lychee.toml`'s exclude list (`docs.nats.io` and every
`codeberg.org` archive URL are reported excluded solely because of
`--offline`). 46 distinct external URLs in tracked markdown, 27 of them the
archive-tag links the reboot's preservation story depends on, have no automated
guard. The two tracked HTML files
(`deploy/docs/site/index.html`, `deploy/vanity/site/keystone-core/index.html`)
are outside the `**/*.md` glob entirely and are never parsed at all; both
contain only absolute links, which `--offline` would have excluded anyway.

This is a defensible choice — an online gate is flaky and rate-limited — but it
means "every archive reference in the docs resolves" is a claim nothing
re-checks after R10. A periodic (not per-PR) online link job would be the
conventional answer.

---

## Claims I could NOT independently verify

- **Branch protection and required status checks.** The workflow comment states
  that R06 "switched `main`'s required checks to it." I have no forge access
  from this worktree and cannot confirm that `reboot-baseline / verify` is
  actually required on `main`, that direct pushes to `main` are blocked, or
  that the archive branch and tag are protected against update and deletion.
  C-08's severity depends on this: if direct pushes to `main` are possible, the
  DCO gate is bypassable outright rather than merely vacuous on an edge case.
- **The `docker` runner image.** `runs-on: docker` with no `container:` key
  means the image comes from the runner's own configuration, which is not in
  this repository. Whether it ships Node ≥ 22 (required by C-06), `curl`, `tar`
  and `bash` is unverifiable from here. All four failure modes are loud rather
  than silent, so this is a liveness question, not a correctness one.
- **Whether the workflow has ever actually run green on a pull request.** I
  replayed every step locally but cannot read job history.
- **The integrity of the transition evidence JSON** (`manifest.json`,
  `r07/*.json`, `r09/*.json`). No gate validates these against a schema or
  against the archive; only `capability-coverage.json` is checked, by
  `capcheck`. Assessing their content is another auditor's slice; I record only
  that CI does not check them.
- **Whether `MD060` is a real markdownlint rule.** See C-11 — the config
  silently accepts names that do not exist, so its presence proves nothing
  either way.

---

## Worktree state

Every planted defect was reverted with `git checkout --` immediately after the
gate's response was recorded; the four probe commits created for the DCO tests
were removed with `git reset --hard 7ca5522312512122481dbc4c8cfbb1cac08e7432`.
The stray `tools/capcheck/capcheck` binary was deleted. `git status` reports
only this report file. Final `make check` after all reverts: `check: ok`.
