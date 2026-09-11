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

## Result

| Case | Defect planted | Cases that fired |
|---|---|---|
| `AC-1` | Dropped the `**Exit:**` line from § 5.4 Status | `AC-1` only |
| `AC-2` | Changed a cited invariant to `ARCH-JOB-999` | `AC-2` only |
| `AC-3` | Removed `batch fan-out` from the nine excluded forms | `AC-3` only |
| `AC-4` | Pointed a non-goal at `CAP-BLUE-999`, absent from the catalog | `AC-4` only |
| `AC-5` | Emptied the owner cell of a success measure | **none on the first run — see below** |
| `AC-6` | Replaced the narrow/stop branch with "the project reconsiders" | `AC-6` only |
| `AC-7` | Asserted blueprint support inside the Run journey | `AC-7` only |

Control: the unmodified charter reports `== 0 failures ==`, before and after
every defect run. No case fired on more than its target, so each is
discriminating.

## `AC-5` did not fail, and that is the finding

On the first run the planted empty-owner defect **passed**. The check used a
regular expression requiring four non-empty cells per row; a row with an empty
owner did not match the pattern at all, so it was silently skipped rather than
reported. The check could not fail for the exact defect it existed to catch.

This is the same shape as the four transition defects R10 documented — the
check appeared to pass because it was not looking — and it was written into
P00's own acceptance procedure by the agent arguing for that procedure.

Fixed by parsing rows structurally and treating a row that does not yield four
non-empty cells as a failure rather than as a row to skip. Re-running the same
defect after the fix:

```
FAIL AC-5 malformed/empty measure row: | Every lifecycle transition is auditable | 100% of transiti
== 1 failures ==
```

Had § 5.3 not required the demonstration, `AC-5` would have been accepted as
passing and would have certified any future charter with a missing owner.

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
names = ["5.1 Enroll","5.2 List and presence","5.3 Run","5.4 Status","5.5 Output","5.6 Cancel","5.7 Audit"]
for n in names:
    b = sect(n)
    if not b: fails.append(f"AC-1 missing journey {n}"); continue
    if "```" not in b: fails.append(f"AC-1 {n}: no argv block")
    if "**Exit:**" not in b: fails.append(f"AC-1 {n}: no exit code")
    if "**Observable effect:**" not in b: fails.append(f"AC-1 {n}: no observable effect")

# AC-2 every cited ARCH-* resolves; every journey cites >=1
cited = set(re.findall(r"ARCH-[A-Z]+-\d+", C))
known = set(re.findall(r"### (ARCH-[A-Z]+-\d+)", INV))
for i in sorted(cited - known): fails.append(f"AC-2 unresolvable invariant {i}")
for n in names:
    if not re.search(r"ARCH-[A-Z]+-\d+", sect(n)): fails.append(f"AC-2 {n}: cites no invariant")

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
            if f"`{c}`" not in CAT: fails.append(f"AC-4 {c} not in catalog ({goal[:30]})")
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
