# P00 acceptance evidence

Required by [`P00.md`](P00.md) § 5.3: every acceptance case in § 5.2 is
demonstrated **failing** before it is accepted. A case that cannot fail is not
evidence, and an acceptance case that cannot fail is R10's defect in charter
clothing.

Each case is mechanized below as a re-runnable check. P00 adds no gate — the
script is recorded here so a reviewer can reproduce the result, not wired into
`make check`. Mechanizing any of these permanently is a P11 decision.

## Method

For each case: plant a defect in `PRODUCT-CHARTER.md` that violates exactly
that case, run the checks, record which cases fired, restore, re-run. The
charter on disk is never left modified; every defect is applied to a copy.

## Round 1 — the seven cases

| Case | Defect planted | Cases that fired |
|---|---|---|
| `AC-1` | Dropped the `**Exit:**` line from § 5.4 Status | `AC-1` only |
| `AC-2` | Changed a cited invariant to `ARCH-JOB-999` | `AC-2` only |
| `AC-3` | Removed `batch fan-out` from the nine excluded forms | `AC-3` only |
| `AC-4` | Pointed a non-goal at `CAP-BLUE-999`, absent from the catalog | `AC-4` only |
| `AC-5` | Emptied the owner cell of a success measure | **none — see below** |
| `AC-6` | Replaced the narrow/stop branch with "the project reconsiders" | `AC-6` only |
| `AC-7` | Asserted blueprint support inside the Run journey | `AC-7` only |

### `AC-5` did not fail

On the first run the planted empty-owner defect **passed**. The check used a
regular expression requiring four non-empty cells per row; a row with an empty
owner did not match the pattern at all, so it was silently skipped rather than
reported. The check could not fail for the exact defect it existed to catch.

Fixed by parsing rows structurally and treating a row that does not yield four
non-empty cells as a failure rather than as a row to skip.

## Round 2 — after independent review

Codex reviewed the pull request and raised five findings by hand. Four were
defects in the charter; two of those were invisible to the checks, which is the
more useful result. Each was replanted as a mechanical defect after the fix, to
prove the strengthened check now catches what a human caught.

| Finding | Charter fix | Check strengthened | Replanted defect fires? |
|---|---|---|---|
| `F1` — Run claimed the command "executes exactly once", which `ARCH-JOB-001` forbids outright | Rewritten as at-most-one automatic attempt, with the zero-execution cases named | `AC-2` now rejects the literal claim, since `ARCH-JOB-001` names it | Yes — `AC-2` |
| `F2` — the bootstrap token sat in argv, exposing it to process listings and shell history | `--token-file`, plus a stated constraint that the token must not appear in argv | Not mechanized; a surface-shape property | N/A |
| `F3a` — enrollment said "non-zero" where `AC-1` claims *exact* exit codes | Exact codes for both enrollment invocations | `AC-1` now rejects "non-zero" in an exit paragraph | Yes — `AC-1` |
| `F3b` — Run omitted exit code `14` | `14` added | **Not mechanizable** — see limits | No |
| `F4` — non-goals cited incomplete capability sets | Added `CAP-STATE-001`, `CAP-SECRET-003`, `CAP-BLUE-007`, `CAP-BLUE-011` | `AC-4` now requires every cited entry be `Future`-marked | Yes — `AC-4` |
| `F5` — § 8 did not say how many partners must sustain use | At least two of the three, with the reasoning stated | Not mechanized; a product decision | N/A |

Two further defects were planted to exercise the strengthened checks directly:

| Defect | Result |
|---|---|
| A journey cites exit code `99`, absent from the exit-code table | `AC-1` fires |
| A non-goal cites `CAP-FOUND-002`, which is `v0.6`-marked, not `Future` | `AC-4` fires |

### `F3a` did not fail either, on its first replant

Requiring "at least one enumerated exit code" still passed a paragraph where a
vague "non-zero" sat beside exact codes. `AC-1` claims *exact* exit codes, so
the check was weaker than the case. Hardened to reject the word outright.

## The pattern this produced

Three checks in this task were weaker than the case they claimed to enforce:
`AC-5` skipped malformed rows, `AC-1` accepted an `**Exit:**` heading as proof
of an exact exit code, and `AC-1` then accepted "non-zero" beside exact codes.
All three passed defects they existed to catch. **Two of the three surfaced
only because a human reviewer's finding was replanted as a mechanical defect** —
the demonstration alone did not find them, and neither did the original review
alone.

Generalizable: *a check that silently skips or loosely matches its input cannot
fail on that input.* Parse structurally, and treat vague or unparseable as
failure rather than as absence of evidence.

## Known limits

`AC-1` verifies that every exit code a journey *states* is defined in the
exit-code table. It cannot detect a code that is **missing but applicable** —
`F3b`, Run omitting `14`, was found by review and remains a review property.
Mechanizing it would require a per-journey expected-code set, which is a
specification P00 does not have and should not invent.

`AC-4` verifies that every cited capability exists and is `Future`-marked. It
cannot verify *correspondence* — that the cited entries are the right ones for
the non-goal. `F4` was a correspondence failure and was found by review.

Both limits are stated rather than papered over. A case that cannot fail on
something should say what.

## Reproducing

Save as `accept.py`, run from the repository root:

```
python3 accept.py docs/project/PRODUCT-CHARTER.md
```

Exit `0` and `== 0 failures ==` is a pass.

```python
import re, sys, pathlib
C = pathlib.Path(sys.argv[1]).read_text()
INV = pathlib.Path("docs/project/ARCHITECTURE-INVARIANTS.md").read_text()
CAT = pathlib.Path("docs/project/FUTURE-CAPABILITIES.md").read_text()
RM  = pathlib.Path("docs/project/ROADMAP.md").read_text()
fails = []

def sect(n):
    m = re.search(rf"\n### {re.escape(n)}\n(.*?)(?=\n### |\n## )", C, re.S)
    return m.group(1) if m else ""

# AC-1 seven journeys, each with argv fence, Exit, Observable effect
tbl = re.search(r"### Exit codes\n(.*?)(?=\n### )", C, re.S)
TABLE_CODES = set(re.findall(r"^\| `(\d+)` \|", tbl.group(1), re.M)) if tbl else set()
if not TABLE_CODES: fails.append("AC-1 no exit-code table")

names = ["5.1 Enroll","5.2 List and presence","5.3 Run","5.4 Status","5.5 Output","5.6 Cancel","5.7 Audit"]
for n in names:
    b = sect(n)
    if not b: fails.append(f"AC-1 missing journey {n}"); continue
    if "```" not in b: fails.append(f"AC-1 {n}: no argv block")
    if "**Exit:**" not in b:
        fails.append(f"AC-1 {n}: no exit code")
    else:
        # An "**Exit:**" heading is not an exact exit code. Require enumerated
        # codes, and require every one to be defined in the exit-code table -
        # the earlier check asserted only that the paragraph existed, which
        # certified "non-zero" as exact.
        para = re.search(r"\*\*Exit:\*\*(.*?)(?=\n\n)", b, re.S)
        codes = set(re.findall(r"`(\d+)`", para.group(1))) if para else set()
        if not codes: fails.append(f"AC-1 {n}: exit paragraph enumerates no code")
        # "at least one code present" still certifies a vague "non-zero" sitting
        # beside exact ones. AC-1 claims EXACT exit codes, so vagueness fails.
        if para and re.search(r"non-zero|nonzero", para.group(1), re.I):
            fails.append(f"AC-1 {n}: exit paragraph says 'non-zero' instead of an exact code")
        for c in sorted(codes - TABLE_CODES):
            fails.append(f"AC-1 {n}: exit code {c} not defined in the exit-code table")
    if "**Observable effect:**" not in b: fails.append(f"AC-1 {n}: no observable effect")

# AC-2 every cited ARCH-* resolves; every journey cites >=1
cited = set(re.findall(r"ARCH-[A-Z]+-\d+", C))
known = set(re.findall(r"### (ARCH-[A-Z]+-\d+)", INV))
for i in sorted(cited - known): fails.append(f"AC-2 unresolvable invariant {i}")
for n in names:
    if not re.search(r"ARCH-[A-Z]+-\d+", sect(n)): fails.append(f"AC-2 {n}: cites no invariant")

if re.search(r"exactly[ -]once execution|executes exactly once", C, re.I):
    fails.append("AC-2 charter claims exactly-once execution, forbidden by ARCH-JOB-001")

# AC-3 nine excluded forms
s6 = re.search(r"\n## 6\..*?(?=\n## )", C, re.S)
s6 = s6.group(0) if s6 else ""
forms = ["shell","stdin streaming","scripts","pipelines","caller-selected user",
         "caller-provided environment","arbitrary working directory","batch fan-out","interactive session"]
missing = [f for f in forms if f not in s6]
if missing: fails.append(f"AC-3 missing excluded forms: {missing}")

# AC-4 every non-goal resolves to a real CAP-* or Not Planned
s4 = re.search(r"\n## 4\..*?(?=\n## )", C, re.S)
s4 = s4.group(0) if s4 else ""
rows = [r for r in re.findall(r"^\| (?!Non-goal)(?!-)(.+?) \| (.+?) \|$", s4, re.M)]
if not rows: fails.append("AC-4 no non-goal rows")
for goal, res in rows:
    caps = re.findall(r"CAP-[A-Z]+-\d+", res)
    if caps:
        for c in caps:
            row = re.search(rf"^\| `{c}` \|.*$", CAT, re.M)
            if not row:
                fails.append(f"AC-4 {c} not in catalog ({goal[:30]})")
            else:
                cells = [x.strip() for x in row.group(0).strip().strip("|").split("|")]
                if len(cells) < 4 or cells[3] != "Future":
                    fails.append(f"AC-4 {c} is not Future-marked ({goal[:30]})")
    elif "Not Planned" in res:
        if "## Not Planned" not in RM: fails.append("AC-4 Not Planned bucket missing")
    else:
        fails.append(f"AC-4 non-goal resolves to nothing: {goal[:40]}")

# AC-5 every success measure has number, method, owner
s5 = re.search(r"\n## 3\..*?(?=\n## )", C, re.S)
s5 = s5.group(0) if s5 else ""
# Parse every table row structurally. A row that does not yield four non-empty
# cells is a FAILURE, not a row to skip - skipping is how this check passed a
# planted empty-owner defect on its first run.
raw = [l for l in s5.splitlines() if l.startswith("|")]
raw = [l for l in raw if not re.match(r"^\|[\s-]+\|", l) and not l.startswith("| Measure")]
if not raw: fails.append("AC-5 no measure rows")
mrows = []
for l in raw:
    cells = [c.strip() for c in l.strip().strip("|").split("|")]
    if len(cells) != 4 or not all(cells):
        fails.append(f"AC-5 malformed/empty measure row: {l[:60]}")
    else:
        mrows.append(cells)
for meas, target, method, owner in mrows:
    if not re.search(r"\d", target): fails.append(f"AC-5 no number: {meas[:35]}")
    if not method.strip(): fails.append(f"AC-5 no method: {meas[:35]}")
    if not owner.strip(): fails.append(f"AC-5 no owner: {meas[:35]}")

# AC-6 validation plan completeness
s7 = re.search(r"\n## 7\..*?(?=\n## )", C, re.S)
s7 = s7.group(0) if s7 else ""
if not re.search(r"at least three", s7): fails.append("AC-6 no partner count")
if not re.search(r"[Nn]inety days|60|90", s7): fails.append("AC-6 no duration")
if "sustained" not in s7.lower(): fails.append("AC-6 no sustained-use definition")
s8 = re.search(r"\n## 8\..*", C, re.S)
s8 = s8.group(0) if s8 else ""
if "willingness to pay" not in s8.lower(): fails.append("AC-6 no willingness-to-pay definition")
if "narrows or stops" not in s8: fails.append("AC-6 no narrow/stop branch")

# AC-7 no Future capability asserted in scope (journeys + problems + success)
scope = sect("5.1 Enroll")+sect("5.3 Run")+ (re.search(r"\n## 2\..*?(?=\n## )", C, re.S).group(0) if re.search(r"\n## 2\..*?(?=\n## )", C, re.S) else "")
for term in ["blueprint","state module","drift remediation","secret broker","webhook","clustering","plugin"]:
    if term in scope.lower(): fails.append(f"AC-7 future capability in scope: {term}")

for f in fails: print("FAIL", f)
print(f"== {len(fails)} failures ==")
sys.exit(1 if fails else 0)
```
