# P11 acceptance evidence

The acceptance cases of [`P11.md`](P11.md) § 5.2. **P11 is delivered in two pull
requests** — `P11a`, the module and the gates, and `P11b`, the lint tools and the
harness shell — so this record lands with the first and is extended by the
second. Every case below names which one owes it.

**These are the first acceptance cases in Stage P that are not about prose.** A
defect is planted in code, configuration or a gate, and the gate itself must
reject it. Nothing here reads a document to see whether it says the right thing.

## `P11b` — the lint tools, the register, and the harness

**Every case `P11a` left owed is now paid, and `AC-6` turned out not to be
vacuous.** I predicted it would be. Three rows land at `P11`, so ticking it makes
them **owed** on the day this ships.

**One of the three is machine-checked; two are not, and `archlint` says so
rather than counting them.**

```text
· 3 rows owed by a completed task, 1 paid
· 2 owed rows describe their evidence in prose and are NOT machine-checked: ARCH-NATS-008, ARCH-TEST-003
```

`ARCH-NATS-008` and `ARCH-TEST-003` name *rules* rather than files. Counting
those as paid would make **writing a sentence** a way to satisfy the gate, so
they are reported and excluded. `AC-6`'s non-vacuity therefore rests on **one**
row — the isolation test — which is a smaller claim than the one this paragraph
made before review, and a true one.

### What each case fails on

| Case | Fails when |
|---|---|
| `AC-3` | A superseded conclusion returns to any tracked file, a forbidden CI trigger appears, a rule outlives its retirement, a rule states no reason, or a rule's subject leaves the tree so it cannot fail |
| `AC-4` | An invariant has no row, a row names an invariant that does not exist, a design owner names no document, or a gate is not one `TESTING.md` schedules |
| `AC-5` | A row names no task that owes its evidence, or names one the epic does not have |
| `AC-6` | A task the epic shows complete owes evidence that does not exist |
| `AC-7` | **The probe cannot detect a route** — not merely that isolation held |
| `AC-1` | The container suite enters `check` while CI cannot run it, or the deferral stops saying what would end it |

### The lifetime mechanism fired on its first run, against its author

`what-p10-is` was written with `RetireAfter: "P11"` — and ticking `P11` in the
same commit retired it, failing the build.

**That is the mechanism working, and it forced the question the guess had
skipped.** A sweep retires when the corrected text has been **stable across
enough tasks** that the belief is no longer available, and one task later is not
that. Its demonstrate half shares a subject with `p10-risk-gates`, so they now
retire together at `C14` — the task that actually attempts the reconstruction.

Without the forced failure I would have shipped a rule that vanished silently
the moment its own task completed.

### The probe is demonstrated in both directions, locally

`ADR-0010` § 2 requires the isolation to be *proved by a probe* — *"a topology
that merely omits a route passes vacuously the day someone adds one."* So the
same `reachable()` runs twice: separate networks must be unreachable, **and a
deliberately joined pair must be reported reachable.**

```text
=== RUN   TestSeparateNetworksAreNotReachable
--- PASS: TestSeparateNetworksAreNotReachable (5.76s)
=== RUN   TestTheProbeDetectsAJoinedPair
--- PASS: TestTheProbeDetectsAJoinedPair (0.61s)
PASS
ok      go.keystone-core.io/keystone-core/test/e2e/docker   6.379s
```

**What it does not establish**, in the file rather than in a hope: that the
product *works* across this topology. `ARCH-COMM-002` requires the parts to
function without a direct route, and nothing here runs a journey — no NATS
connection exists until C06. This proves the arrangement.

### The container suite is outside `make check`, and that is asserted

CI cannot run it: jobs on the runner have no Docker client and no socket
(`CI-RUNNER.md`, `R6`). Putting it in `check` today would fail `gates-agree`,
correctly, because CI has no step for it.

**A gate in neither set is invisible to `gates-agree`** — not missing from
either, so nothing reports it. `deferred-gates-check` asserts the deferral
instead: the target exists, it is **not** in `check`, and the recipe says what
ends the deferral.

### Two false negatives review found, and why they were the dangerous kind

**`archlint` treated any evidence cell containing a space as paid.** The
rationale was *"a description rather than a path"* — and it made every
**multi-path** cell trivially satisfied. `ARCH-NATS-006` names two test files;
when C01 completes, deleting either would have left the tool green. **A check
that cannot fail on the input it exists for is not a check.** Cells are parsed
now: every path-shaped token must exist, and prose is reported rather than
counted.

**The sweep asked `git ls-files` for `*.md` and `*.yml`, not `*.yaml`.** A
tracked `.yaml` workflow was never scanned — so the **permanent security rule**,
the one forbidding `pull_request_target`, could be bypassed by an extension.
This repository's own `compose.yaml` was already unscanned.

Both extensions are covered — and, because a pattern list cannot notice what it
does not match, `doclint` now **enumerates `.forgejo/workflows` directly** and
fails if any tracked file there is outside the swept set. An extension nobody
anticipated is exactly how this happened.

### Five defects the demonstration found in this task's own tools

- **`units()` flattened YAML.** Joining an indented `pull_request_target:` onto the line
  above stops a `^`-anchored pattern matching, so the forbidden-trigger rule
  **silently passed**. The paragraph model is Markdown's; in YAML a line is the
  unit.
- **Prose sweeps were case-sensitive**, so a sentence *beginning* with the
  subject was not examined while the same sentence mid-paragraph was.
- **`deferred-gates-check` read only the first line of `check`'s prerequisites**,
  so anything on a continuation line was invisible — the identical bug
  `gates-agree` had, reintroduced three targets away.
- **A check that read itself.** It grepped the whole `Makefile` for the phrase
  that states the deferral's end condition — and that phrase appears in the
  grep's own pattern, so it passed however the comment changed. Scoped to the
  recipe.
- **Two plantings that tested nothing**: one appended text without the rule's
  subject, so the unit was never examined; one replaced an assertion with
  `if false`, which makes a test pass rather than breaking the probe it was
  aimed at.

## Which pull request owes each case

| Case | Owed by | State |
|---|---|---|
| `AC-1` | Both | **Paid.** `gates-agree` covers the three gates `P11b` adds; the container suite is deferred, and `deferred-gates-check` asserts the deferral rather than leaving it invisible |
| `AC-2` | `P11a` | **Paid** |
| `AC-3` | `P11b` | **Paid** — `tools/doclint`, seven rules with lifetimes |
| `AC-4` | `P11b` | **Paid** — `tools/archlint`, both directions |
| `AC-5` | `P11b` | **Paid** — the register's `Lands at` column |
| `AC-6` | `P11b` | **Paid, and not vacuous**: three rows owed by a completed task, one machine-checked and two reported as prose rather than counted |
| `AC-7` | `P11b` | **Paid** — demonstrated locally in both directions |
| `AC-8` | `P11a` | **Paid** |
| `AC-9` | `P11a` | **Paid** |
| `AC-10` | `P11a` | **Paid** |
| `AC-11` | `P11a` | **Paid** |
| `AC-12` | Both | **Paid** |

**The five owed cases are the ones with a `DL-1` risk**, which is why the split
was drawn here: `AC-6`'s vacuity, `AC-4`'s register sweep and `AC-7`'s probe all
land together, in a pull request whose review is about nothing else.

## What each paid case fails on

| Case | Fails when |
|---|---|
| `AC-2` | A trailing space or tab survives in any tracked text file, **including inside a fenced code block** |
| `AC-8` | The version is a literal, or `commit` carries an initialiser in source |
| `AC-9` | Any non-test file imports a NATS client, `net`, `os/exec` or `golang.org/x/sys/unix`, or wires a journey verb |
| `AC-10` | A target in `make check` is not invoked by a CI step, **or CI invokes one `make check` omits**, or the one named CI-only gate disappears |
| `AC-11` | `AGENTS.md` still says this repository has no product code |

## `AC-2` exists because the gate we had could not see the defect

`markdownlint`'s MD009 **is** enabled and does flag trailing spaces. It did not
flag #323's because **markdownlint skips fenced code blocks** — and that is where
every generated artifact in this repository lives: the demonstration records and
embedded checkers in these evidence files, written by a generator and reviewed by
nobody line by line.

The demonstration asserts that directly. Its first planting requires
`markdownlint-cli2` to report **`0 issues in 0 files`** on the same file
`whitespace-check` rejects — so the case proves the gap it was built for, rather
than proving that a whitespace checker checks whitespace.

**It reads `git ls-files`, not a diff range.** `git diff --check` compares the
worktree to the index, which after a clean CI checkout is empty, so a gate built
on it would pass unconditionally — `DL-1`, shipped as a gate.

**And it carries that mechanism's own blind spot in its comment**: `git ls-files`
lists *tracked* files, so an unstaged file is not checked. That is how a broken
link reached review on #330, in a dossier whose § 6 warned about it.

## A defect this task found rather than introduced

`.gitignore` carried `**/target/` under a heading reading `# Rust build
artifacts`. **There is no Rust here**, and that pattern matched
`internal/cli/target/` — which in Generation 1 was a real Go source package, the
one that parsed `--target`.

`internal/cli/target/required_test.go`, written 2026-09-04, was in **no git
history at all**: not on `main`, not in `archive-2026-09-pre-v0.6-reboot`, not in
any ref. The rule swallowed it the day it was created. R08 deleted the two
tracked files beside it and left this one in working trees.

**The risk was live rather than historical.** Generation 2's journeys are about
*targeting an agent*, so a `target` package is a plausible thing for C04 or C08
to create — and it would have been invisible to `git status` and skipped by
`git add -A`, exactly as this file was.

The pattern is now `/target/`, scoped to the repository root, and the
demonstration checks **both directions**: a source package named `target` is
visible to git, and build output at the root is still ignored.

The file was preserved before deletion and removed on the maintainer's
instruction. It is a sixty-line test for a Generation 1 feature Generation 2 does
not have; what is worth keeping is the class — **a tool whose input set silently
differs from what you believe it is**, which is the third instance this stage
after the `docs-links` glob and `make check`'s staging blind spot.

## Demonstration record

```text
unmutated tree: PASS
AC-3   fails as expected  — a superseded conclusion returns to a tracked file
AC-3   fails as expected  — a forbidden CI trigger is added
AC-3   fails as expected  — a rule outlives its retirement
AC-3   fails as expected  — a rule loses its reason and cannot be retired by anyone
AC-3   fails as expected  — a sweep's subject leaves the tree, so it cannot fail
AC-3   fails as expected  — a forbidden trigger hides in a .yaml workflow
AC-3   fails as expected  — a workflow the sweep's patterns cannot match
AC-6   fails as expected  — a listed evidence path is deleted while the cell still names two
AC-6   fails as expected  — prose evidence is counted as paid
AC-4   fails as expected  — an invariant loses its register row
AC-4   fails as expected  — a register row names an invariant that does not exist
AC-4   fails as expected  — a design owner names no document
AC-4   fails as expected  — a gate is not one TESTING.md schedules
AC-5   fails as expected  — a row names no task that owes its evidence
AC-5   fails as expected  — a row lands at a task the epic does not have
AC-6   fails as expected  — a completed task owes evidence that does not exist
AC-7   fails as expected  — the isolation probe cannot detect a route
AC-1   fails as expected  — the container suite is put into check while CI cannot run it
AC-1   fails as expected  — the deferral stops saying what would end it
```

## The harness

```python
#!/usr/bin/env python3
"""Demonstrate P11a's acceptance cases failing on a tree that carries the defect.

Unlike every Stage P task before it, these are planted in code and gates rather
than in prose: a defect is introduced and the gate itself must reject it.
"""
import os, re, subprocess, sys, tempfile, pathlib

SRC = pathlib.Path("/home/sbutts/code/keystone-core")
COPY = ["go.mod", "Makefile", "AGENTS.md", ".gitignore",
        ".forgejo/workflows/reboot-baseline.yml"]
COPYTREE = ["cmd", "internal"]

def sh(root, cmd):
    return subprocess.run(["bash", "-c", cmd], cwd=root, capture_output=True, text=True)

CASES, bad = [], 0
def case(tag, why, cmd):
    def deco(fn): CASES.append((tag, why, cmd, fn)); return fn
    return deco

def sub(root, f, pat, new, n=1):
    p = root / f; t = p.read_text()
    t2, k = re.subn(pat, new, t, flags=re.M)
    assert k == n, f"substituted {k}, expected {n}"
    p.write_text(t2)

@case("AC-2", "a trailing space inside a fenced block", "make whitespace-check")
def _(r):
    (r / "_p.md").write_text("# T\n\n```text\ncode \n```\n")
    sh(r, "git add _p.md")
    # markdownlint must NOT see it -- that is the gap this gate exists for
    md = sh(r, "npx --yes markdownlint-cli2 _p.md 2>&1 | tail -1")
    assert "0 issues" in md.stdout, f"markdownlint saw it: {md.stdout!r}"

@case("AC-2", "a trailing tab in Go source", "make whitespace-check")
def _(r):
    (r / "internal/cli/_p.go").write_text("package cli\n\n// x\t\nvar _ = 1\n")
    sh(r, "git add internal/cli/_p.go")

@case("AC-8", "the version is a literal rather than derived", "go test -count=1 ./internal/version/")
def _(r):
    sub(r, "internal/version/version.go", r'var commit string', 'var commit string = "deadbeef"')

@case("config", "an environment override may be relative, so $PWD is read",
      "go test -count=1 ./internal/config/")
def _(r):
    # The defect review of #331 found: the override returned verbatim.
    sub(r, "internal/config/config.go",
        r"\t\tif err := absolute\(EnvOverride\(r\), v\); err != nil \{\n\t\t\treturn nil, err\n\t\t\}\n", "")

@case("config", "a relative HOME builds a relative candidate",
      "go test -count=1 ./internal/config/")
def _(r):
    sub(r, "internal/config/config.go",
        r'\t\t\tif err := absolute\("HOME", h\); err != nil \{\n\t\t\t\treturn nil, err\n\t\t\t\}\n', "")

@case("config", "a relative override falls through instead of being refused",
      "go test -count=1 ./internal/config/")
def _(r):
    sub(r, "internal/config/config.go",
        r"\t\tif err := absolute\(EnvOverride\(r\), v\); err != nil \{\n\t\t\treturn nil, err\n\t\t\}\n\t\treturn \[\]string\{v\}, nil\n",
        "\t\tif err := absolute(EnvOverride(r), v); err == nil {\n\t\t\treturn []string{v}, nil\n\t\t}\n")

@case("AC-9", "a binary imports a NATS client", "go test -count=1 ./internal/cli/")
def _(r):
    sub(r, "cmd/keystone-agent/main.go", r'^\t"flag"$', '\t"flag"\n\t_ "github.com/nats-io/nats.go"')

@case("AC-9", "a binary opens a network connection", "go test -count=1 ./internal/cli/")
def _(r):
    sub(r, "cmd/keystone-server/main.go", r'^\t"flag"$', '\t"flag"\n\t_ "net"')

@case("AC-9", "a journey verb is wired", "go test -count=1 ./internal/cli/")
def _(r):
    sub(r, "cmd/keystone/main.go", r'showVersion := fs\.Bool\("version"',
        'verb := "run"\n\t_ = verb\n\tshowVersion := fs.Bool("version"')

# One definition, in the Makefile, used by CI and by this harness alike.
AGREE = "make gates-agree"

@case("AC-10", "a check target is not run by CI", AGREE)
def _(r):
    sub(r, ".forgejo/workflows/reboot-baseline.yml", r"^      - name: go vet\n        run: make vet\n\n", "")

@case("AC-10", "CI runs a gate that make check does not", AGREE)
def _(r):
    # The direction review of #331 found open: a CI-only gate, with `check`
    # still advertising that it runs everything.
    # A trailing backslash in the replacement is a regex escape to re.sub, not a
    # literal. Drop build and vuln without touching the continuation.
    sub(r, "Makefile", r"whitespace-check build vuln docs-lint", "whitespace-check docs-lint")

@case("AC-10", "the one CI-only gate is removed without removing its exemption", "make dco-exempt-check")
def _(r):
    sub(r, ".forgejo/workflows/reboot-baseline.yml",
        r"      - name: DCO sign-off on every pull-request commit\n", "      - name: DCO check\n")

@case("AC-11", "AGENTS.md still says there is no product code",
      '! grep -q "There is no product code here" AGENTS.md')
def _(r):
    sub(r, "AGENTS.md", r"Planning, governance, transition evidence — and, since \*\*P11a\*\*, a Go module\.",
        "Planning, governance and transition evidence. **There is no product code here.**")

@case("gitignore", "a source package named target is invisible to git",
      "mkdir -p internal/x/target && touch internal/x/target/a.go && "
      "! git check-ignore -q internal/x/target/a.go")
def _(r):
    sub(r, ".gitignore", r"^/target/$", "**/target/")

with tempfile.TemporaryDirectory() as td:
    for tag, why, cmd, fn in [(None, None, None, None)] + CASES:
        root = pathlib.Path(td) / "wt"
        subprocess.run(["git","-C",str(SRC),"worktree","add","--detach","-q",str(root)],
                       check=True, capture_output=True)
        try:
            for f in COPY:
                (root / f).write_text((SRC / f).read_text())
            for d in COPYTREE:
                subprocess.run(["cp", "-r", str(SRC / d), str(root)], check=True)
            sh(root, "git add -A")
            if fn is None:
                fails = [(t, c) for t, _, c, _ in CASES if sh(root, c).returncode != 0]
                print("unmutated tree:", "PASS" if not fails else f"FAIL {fails}")
                if fails: bad += 1
                continue
            fn(root)
            sh(root, "git add -A")
            rc = sh(root, cmd).returncode
            print(f"{tag:6} {'fails as expected' if rc != 0 else 'DID NOT FAIL ON ITS OWN CASE'}  — {why}")
            if rc == 0:
                bad += 1; print("   ", sh(root, cmd).stdout[-400:])
        finally:
            subprocess.run(["git","-C",str(SRC),"worktree","remove","--force",str(root)],
                           capture_output=True)
sys.exit(1 if bad else 0)
```

## Four defects found while writing these

**`AC-8`'s first version could not see a literal.** It set `commit` itself before
asserting, so a literal initialiser in the source was overridden by the test and
invisible. A second case now reads `version.go` and rejects any initialiser —
and fails if the declaration it looks for is gone, so it cannot quietly stop
checking.

**`go test` served cached results**, so a planted defect reported success. Every
demonstration now runs with `-count=1`.

**`make check` ran `test` while CI ran `test-race`.** `AC-10` rejected it, which
is the case doing exactly its job on the first run. `check` now runs the race
detector, because `ADR-0010` § 11 requires one local command to run what CI runs
and "nearly" is not that.

**The boundary test caught `version_test.go` shelling out to `git`.** That import
is `os/exec`, which `AC-9` forbids — and the test needs it, because `AC-8`
requires the version to be *derived* rather than typed. The rule is scoped to
non-test source, and the loosening is stated in the code rather than left as an
unexplained exception.

## A configuration path could still reach `$PWD`

`internal/config`'s package comment said nothing reads the working directory.
**It was not enforced.** `SearchPath` returned the environment override
verbatim, so `KEYSTONE_AGENT_CONFIG=config.toml` made the agent read from
wherever it happened to be started — and `XDG_CONFIG_HOME` or `HOME` set to a
relative value built a relative candidate the same way.

**That is a security property, not a tidiness one.** The same service would read
different credentials depending on its working directory, and a party who can
influence a unit's `WorkingDirectory=` would choose which file it reads.

**`TestNoRoleReadsTheWorkingDirectory` passed throughout**, because it exercised
only the default paths and never set an override. A check whose name claims more
than it covers is worse than no check: it occupies the space where the real one
would have gone. Review of #331 found it, and it is the third time in this task a
case was narrower than its own title.

A relative path is now **refused, not ignored** — `ErrRelativePath`, naming the
variable that supplied it. Falling through to the next candidate would read a
file the operator did not name, which is worse than refusing. Three plantings
cover it: the override, a relative `HOME`, and a fall-through in place of a
refusal.

## `AC-10` was asymmetric, and documenting that was not enough

An earlier version of this case checked one direction: that every `make check`
target appears in CI. **The reverse was open** — CI ran `build`, the
vulnerability scan and the DCO check, and `make check` ran none of them, while
the target's own help text said *"Run every gate CI runs."*

I recorded the gap here and treated that as sufficient. Review of #331 did not,
and was right: **`ADR-0010` § 11 states a contract, not a description, and
documenting a gap does not make the contract hold.** The help text was also a
false claim about the repository — `DL-3`, in a file every developer reads first.

**`make check` now runs `build` and `vuln`.** Both can run locally; `vuln`
requires `govulncheck` on `PATH` and says so, which is the same hard dependency
`docs-links` already has on `lychee`.

**The DCO gate is the one exemption, and it is named rather than noticed.** It
reads `git log BASE..HEAD` across a pull request; there is no base locally, and
the workflow already skips it on push for that reason. `dco-exempt-check`
asserts it still exists, so the exemption cannot quietly become a missing gate.

**The assertion moved into the `Makefile` as `gates-agree`**, so CI and a local
run execute the same code. It had been written inline in the workflow — an
assertion written twice is two things to drift, which is the defect this whole
case is about. Three plantings now cover it: a target CI drops, a gate CI has and
`check` lacks, and the exemption's subject disappearing.

## What these cases cannot detect

| Case | Cannot detect | Found instead by |
|---|---|---|
| `AC-2` | Whitespace in a file type `git ls-files --eol` does not classify as text, or in an unstaged file | The next generator that writes one; staging before trusting a green run |
| `AC-8` | Whether the version is right in a **packaged** artifact | C13 |
| `AC-9` | **Behaviour added later.** It is a point-in-time check on a boundary that erodes by increments, and its import list is one someone had to think of | `tools/doclint` at `P11b`; review of every C task |
| `AC-10` | A gate added to CI as an **inline script** rather than a `make` target. `gates-agree` compares `make` targets, and the DCO gate is the one inline gate that exists — covered by name | Review of any workflow change |
| `AC-11` | Nothing — it is a one-line fact | — |
| config | Whether the **content** of a configuration file is trustworthy, or its mode correct. P11a locates a path and parses nothing | C13's packaging; `ADR-0008` § 4's modes |
| All | **Whether any of this is the right design.** Eleven plantings prove eleven gates reject eleven defects; they say nothing about the defects nobody planted | Review, and C01 being the first task to live with it |
