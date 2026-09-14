# P07 acceptance evidence

> **Settled since this was written.** The boundary question this file discusses
> as open — whether an operator naming `/bin/sh` or a `#!` file as `argv[0]` is
> inside charter § 6 — was ruled on by
> [RFC 0004](../rfcs/0004-what-the-execution-exclusions-constrain.md): the
> exclusions constrain what Keystone constructs. The body below is the record of
> what was demonstrated at P07 and is left as written, which is how this project
> treats acceptance evidence overtaken by a later decision.

The eleven cases of [`P07.md`](P07.md) § 5.2, each demonstrated failing on an
`ADR-0007` that carries its defect, then passing on the document as it stands.

The checks read the ADR's **tables** and the sources it must agree with. Three
values are never typed here: the excluded forms are read from
`PRODUCT-CHARTER.md` § 6 and **counted**, the limit categories are parsed out of
`ARCH-EXEC-001`'s own sentence, and the lifecycle states `AC-8` accepts are read
from `ADR-0006` § 1.

## What each case fails on

| Case | Fails when |
|---|---|
| `AC-1` | An excluded form the charter lists is missing from § 2's table or is not stated as excluded; or an admitted form lacks `RFC 0003` and its condition |
| `AC-2` | Deny-by-default is not stated as refusal, or does not cite `ARCH-EXEC-001` |
| `AC-3` | A limit category `ARCH-EXEC-001` names has no row in § 8, or a row states no value or owner |
| `AC-4` | An escalation step signals something other than the process group, fewer than three steps exist, or the group is not distinguished from the immediate child |
| `AC-5` | Truncation is not stated as explicit, or `THR-22` is uncited |
| `AC-6` | A threat this ADR owns has no expression, or its threat-model mitigation was weakened to nothing |
| `AC-7` | § 7 does not name which component is root and which is unprivileged, states no limit on what the split bounds, or does not relate it to `RSK-9` |
| `AC-8` | A limit breach names a lifecycle state `ADR-0006` § 1 does not declare |
| `AC-9` | § 11's **body** does not state what redaction cannot do, does not state the argv limit concretely, or does not exclude the harvested environment |
| `AC-10` | The output limit is not expressed against `ADR-0002` § 9's payload bound |
| `AC-11` | Any tracked document states either reading of § 2.1's question as settled while the ADR records it as open |

## Two cases were weaker than their claims, and the demonstrations found both

Neither was found by review; both were found by a mutation that failed to make
its case fail.

**`AC-4` accepted the invariant sentence in place of the design.** It searched
all of § 10 for *"not the immediate child" or "complete process group"*, and § 10
cites `ARCH-EXEC-002`'s requirement verbatim — so an ADR whose escalation steps
signalled only the child would still have passed. It now **parses the numbered
steps** and requires every step naming a signal to name the process group.

**`AC-9` was satisfied by its own section heading.** The heading reads *"Audit
redaction, and what it cannot do"*, and the case searched the whole section for
"cannot do". A § 11 that stated the limit nowhere in its body passed. It now
reads the body only.

Both are `DL-1`'s class — a check that cannot fail on the input it is written
for — and both survived until a demonstration was written for the exact defect.
That is the argument for demonstrating every case rather than the interesting
ones.

## One defect in the ADR the cases found

`AC-6` failed on the first run: **§ 10 is `THR-23`'s mitigation and never cited
it.** The section described `SIGTERM`, the grace period, `SIGKILL` and the
process group, and named the invariant — `ARCH-EXEC-002` — without naming the
threat the invariant exists for. Fixed by citing `THR-23` and `PROB-5` where the
group is explained.

## Demonstration record

```text
unmutated tree: PASS
AC-1   fails as expected  — an excluded form the charter lists is missing from the ADR's table
        - AC-1 charter § 6 excludes 'pipelines' and § 2's table does not name it
AC-2   fails as expected  — deny-by-default is stated as something other than refusal
        - AC-2 § 3 does not state deny-by-default as refusal
AC-3   fails as expected  — a limit category the invariant names has no row
        - AC-3 § 8 has no row for the 'concurrency' limit
AC-4   fails as expected  — termination reaches only the immediate child
        - AC-4 § 10 signals something other than the process group: '**`SIGTERM` to the child.**'
        - AC-4 § 10 does not distinguish the group from the immediate child
AC-5   fails as expected  — truncation is not stated as explicit
        - AC-5 § 9 does not state truncation as explicit
AC-6   fails as expected  — a threat-model mitigation this ADR owns is gutted
        - AC-6 THR-23's mitigation was weakened to nothing
AC-7   fails as expected  — the split is stated without what it fails to bound
        - AC-7 § 7 states no limit on what the split bounds
AC-8   fails as expected  — a breach maps to a lifecycle state ADR-0006 does not declare
        - AC-8 § 12's 'Duration' row names ['Expired'], which ADR-0006 § 1 does not declare
AC-9   fails as expected  — redaction claims a limit it does not have
        - AC-9 § 11's body does not state what redaction cannot do
        - AC-9 § 11 does not state the argv limit concretely
AC-10  fails as expected  — the output limit is set independently of the payload bound
        - AC-10 § 8 does not express the output limit against ADR-0002 § 9
        - AC-10 § 8 does not relate the output bound to the payload limit
AC-11  fails as expected  — the evidence settles the question the ADR leaves open
        - AC-11 docs/dossiers/P07-acceptance-evidence.md states the exclusion reading as settled while ADR-0007 § 2.1 leaves it open:
```

## The checker

```python
#!/usr/bin/env python3
"""P07 acceptance cases against ADR-0007, read structurally."""
import re, sys, pathlib

A = pathlib.Path(sys.argv[1]).read_text()
ROOT = pathlib.Path(sys.argv[2] if len(sys.argv) > 2 else ".")
CH = (ROOT / "docs/project/PRODUCT-CHARTER.md").read_text()
INV = (ROOT / "docs/project/ARCHITECTURE-INVARIANTS.md").read_text()
TM = (ROOT / "docs/project/THREAT-MODEL.md").read_text()
A6 = (ROOT / "docs/adr/0006-delivery-and-job-lifecycle.md").read_text()
fails, notes = [], []
flat = " ".join(A.split())

def section(pat):
    m = re.search(pat + r".*?(?=\n### |\n## |\Z)", A, re.S)
    return m.group(0) if m else ""

def rows(pat, ncols=None):
    out = []
    for line in section(pat).splitlines():
        if not line.startswith("|"): continue
        c = [x.strip() for x in line.strip().strip("|").split("|")]
        if not c or set("".join(c)) <= set("-: "): continue
        if c[0].lower() in ("form", "category", "step", "component", "item",
                            "breach", "invariant", "risk", "rule", ""): continue
        if ncols and len(c) != ncols: continue
        out.append(c)
    return out

# --- AC-1 every excluded form the charter lists is named and still excluded
sec6 = re.search(r"## 6\. The argv-only execution boundary.*?(?=\n## )", CH, re.S).group(0)
forms = [re.sub(r"^and\s+", "", re.sub(r"[;.]\s*(?:and)?\s*$", "", f).strip())
         for f in re.findall(r"^\d+\.\s+(.+?)$", sec6, re.M)]
notes.append(f"charter § 6 excludes {len(forms)}: {forms}")
if len(forms) < 4:
    fails.append("AC-1 read too few excluded forms — check is vacuous")
table = rows(r"### 2\. The excluded forms")
if not table:
    fails.append("AC-1 § 2 has no table")
named = {r[0].strip("*` ").lower(): r for r in table}
for f in forms:
    key = re.sub(r"^(a|an|the)\s+", "", f.lower())
    hit = [k for k in named if key in k or k in key or
           key.split()[0] in k.split()]
    if not hit:
        fails.append(f"AC-1 charter § 6 excludes {f!r} and § 2's table does not name it")
    elif "excluded" not in named[hit[0]][1].lower():
        fails.append(f"AC-1 § 2 does not state {f!r} as excluded: {named[hit[0]][1]!r}")
# and the two RFC 0003 admits are stated with conditions
admits = [r for r in table if "admitted" in r[1].lower()]
if len(admits) != 2:
    fails.append(f"AC-1 § 2 marks {len(admits)} forms admitted, expected the two RFC 0003 admits")
for r in admits:
    if "RFC 0003" not in r[1]:
        fails.append(f"AC-1 an admitted form does not cite RFC 0003: {r[0]!r}")
    if not r[2].strip():
        fails.append(f"AC-1 admitted form {r[0]!r} states no condition or effect")

# --- AC-2 deny-by-default -------------------------------------------------
d3 = " ".join(section(r"### 3\. Deny-by-default").split())
if not re.search(r"refused, not attempted|is \*\*refused\*\*", d3):
    fails.append("AC-2 § 3 does not state deny-by-default as refusal")
if "ARCH-EXEC-001" not in d3:
    fails.append("AC-2 § 3 does not cite ARCH-EXEC-001")

# --- AC-3 every ARCH-EXEC-001 limit category has a value or an owner -------
inv1 = re.search(r"### ARCH-EXEC-001 —.*?(?=\n### )", INV, re.S).group(0)
cats = re.search(r"Commands have explicit (.*?) limits", " ".join(inv1.split())).group(1)
listed = [c.strip() for c in re.split(r",| and ", cats) if c.strip()]
notes.append(f"ARCH-EXEC-001 names {len(listed)} categories: {listed}")
lim = rows(r"### 8\. Limits")
if len(lim) < len(listed):
    fails.append(f"AC-3 § 8 has {len(lim)} rows for {len(listed)} categories")
have = " ".join(r[0].lower() for r in lim)
for c in listed:
    stem = c.split("-")[0].lower()
    if stem not in have:
        fails.append(f"AC-3 § 8 has no row for the {c!r} limit")
for r in lim:
    if len(r) < 2 or len(r[1]) < 20:
        fails.append(f"AC-3 § 8's {r[0]!r} row states no value or owner")

# --- AC-4 termination reaches the process group, with escalation ----------
t10 = " ".join(section(r"### 10\. Termination").split())
for need, why in [(r"SIGTERM", "SIGTERM"), (r"SIGKILL", "SIGKILL"),
                  (r"grace", "a grace period"), (r"process group", "the process group")]:
    if not re.search(need, t10, re.I):
        fails.append(f"AC-4 § 10 does not name {why}")
# The escalation steps themselves must target the group. Searching all of § 10
# lets the ARCH-EXEC-002 sentence satisfy this while the signal goes to the
# child, which is exactly the defect the case exists for.
steps = re.findall(r"^\d+\.\s+(.+?)$", section(r"### 10\. Termination"), re.M)
if len(steps) < 3:
    fails.append(f"AC-4 § 10 lists {len(steps)} escalation steps, expected three")
signalling = [s for s in steps if re.search(r"SIGTERM|SIGKILL", s, re.I)]
if not signalling:
    fails.append("AC-4 § 10's steps name no signal — check is vacuous")
for s in signalling:
    if not re.search(r"process group", s, re.I):
        fails.append(f"AC-4 § 10 signals something other than the process group: {s[:70]!r}")
if not re.search(r"not the immediate child", t10, re.I):
    fails.append("AC-4 § 10 does not distinguish the group from the immediate child")
if "ARCH-EXEC-002" not in t10:
    fails.append("AC-4 § 10 does not cite ARCH-EXEC-002")

# --- AC-5 truncation is explicit ------------------------------------------
o9 = " ".join(section(r"### 9\. Output handling").split())
if not re.search(r"reported explicitly, never silently", o9):
    fails.append("AC-5 § 9 does not state truncation as explicit")
if "THR-22" not in o9:
    fails.append("AC-5 § 9 does not cite THR-22")

# --- AC-6 the four threats have an expression, and their rows stand --------
for t in ("THR-15", "THR-16", "THR-22", "THR-23"):
    if t not in flat:
        fails.append(f"AC-6 {t} has no expression in the ADR")
    row = re.search(rf"^\| `{t}` \|(.*)$", TM, re.M)
    if not row:
        fails.append(f"AC-6 {t} has no row in the threat model")
        continue
    cells = [c for c in (x.strip() for x in row.group(1).split("|")) if c]
    if len(cells) < 5 or len(cells[-1]) < 40:
        fails.append(f"AC-6 {t}'s mitigation was weakened to nothing")

# --- AC-7 the privilege decision and its consequence ----------------------
p7 = section(r"### 7\. Service identity and privilege")
f7 = " ".join(p7.split())
comp = rows(r"### 7\. Service identity and privilege")
if len(comp) < 2:
    fails.append("AC-7 § 7 does not enumerate the components")
else:
    runs = " ".join(r[1].lower() for r in comp if len(r) > 1)
    if "root" not in runs:
        fails.append("AC-7 § 7 does not say which component is root")
    if not re.search(r"unprivileged|dedicated", runs):
        fails.append("AC-7 § 7 does not say which component is unprivileged")
if not re.search(r"does not bound|not bound", f7):
    fails.append("AC-7 § 7 states no limit on what the split bounds")
if "RSK-9" not in f7:
    fails.append("AC-7 § 7 does not relate the decision to RSK-9")

# --- AC-8 a limit breach maps to an ADR-0006 state, or is raised ----------
br = rows(r"### 12\. What a limit breach becomes")
if len(br) < 4:
    fails.append(f"AC-8 § 12 has {len(br)} breach rows — check is vacuous")
states = {m.strip("`") for m in re.findall(r"^\| `(\w+)` \|[^|]*\| (?:Yes|No) \|",
          re.search(r"### 1\. The server state machine.*?(?=\n### )", A6, re.S).group(0), re.M)}
notes.append(f"ADR-0006 declares {len(states)} server states")
for r in br:
    cited = {m for m in re.findall(r"`(\w+)`", r[1])}
    invented = {c for c in cited if c[:1].isupper() and c not in states
                and c not in {"TimedOut", "Completed", "Refused"}}
    if invented:
        fails.append(f"AC-8 § 12's {r[0]!r} row names {sorted(invented)}, "
                     f"which ADR-0006 § 1 does not declare")

# --- AC-9 audit redaction states what it cannot do ------------------------
# The heading is "…and what it cannot do", so searching the whole section lets
# the title satisfy the case. The claim must be in the body.
r11_full = section(r"### 11\. Audit redaction")
r11 = " ".join(r11_full.split("\n", 1)[1].split())
if not re.search(r"cannot do", r11):
    fails.append("AC-9 § 11's body does not state what redaction cannot do")
if not re.search(r"argv is operator-supplied|secret placed in argv", r11):
    fails.append("AC-9 § 11 does not state the argv limit concretely")
if "harvested environment" not in r11:
    fails.append("AC-9 § 11 does not exclude the harvested environment")

# --- AC-10 the output limit is expressed against ADR-0002 § 9 -------------
l8 = " ".join(section(r"### 8\. Limits").split())
if not re.search(r"`ADR-0002` § 9", l8):
    fails.append("AC-10 § 8 does not express the output limit against ADR-0002 § 9")
if not re.search(r"below.{0,40}1 MiB|1 MiB", l8):
    fails.append("AC-10 § 8 does not relate the output bound to the payload limit")

# --- AC-11 no document settles a question the ADR leaves open --------------
# Derived: the ADR says whether § 2.1 is open. If it is, no tracked file may
# state either reading as the outcome. The evidence file stated the withdrawn
# reading while the ADR had already withdrawn it — DL-8's dependents tier,
# inside the evidence for the task that raised the question.
import subprocess
s21 = section(r"### 2\.1 ")
open_q = bool(re.search(r"decides neither|raised, not decided|does not close it",
                        " ".join(s21.split()), re.I))
notes.append(f"AC-11 § 2.1 reads as {'OPEN' if open_q else 'settled'}")
if open_q:
    SETTLED = re.compile(
        r"exclusions?[^.]{0,60}constrain[^.]{0,80}(what Keystone|not which)"
        r"|what Keystone does, not which"
        r"|the exclusions only constrain", re.I)
    HEDGE = re.compile(r"both readings|decides neither|raised, not decided|open question"
                       r"|does not settle|neither reading|is open", re.I)
    files = [p for p in subprocess.run(["git", "-C", str(ROOT), "ls-files", "*.md"],
             capture_output=True, text=True, check=True).stdout.split()
             if not p.startswith("docs/transition/r10/raw")]
    # § 2.1 is where both readings are laid out by design, so its own blocks
    # present a reading without asserting it. And a block reporting what an
    # earlier draft asserted is a record, not a claim.
    REPORTING = re.compile(r"\basserted\b|\bwithdrawn\b|no longer|the second draft|"
                           r"the first \*\*|previous (?:draft|commit)", re.I)
    s21_blocks = {" ".join(b.split()) for b in re.split(r"\n\s*\n", s21)}
    # Two counters: `matched` proves the pattern still finds the document it was
    # written for, `swept` is what remains after the exemptions. Guarding on the
    # second would fire whenever the exemptions happen to cover everything,
    # which says nothing about whether the check still works.
    matched = swept = 0
    # Fenced code is not prose. The acceptance-evidence file embeds this very
    # checker, whose own pattern strings and comments would otherwise be read as
    # statements about the boundary — a check asserting things about its own
    # source text.
    fence = re.compile(r"^```.*?^```", re.S | re.M)
    for rel in files:
        for blk in re.split(r"\n\s*\n", fence.sub("", (ROOT / rel).read_text())):
            f = " ".join(blk.split())
            if not SETTLED.search(f):
                continue
            matched += 1
            if f in s21_blocks or REPORTING.search(f):
                continue
            swept += 1
            if not HEDGE.search(f):
                fails.append(f"AC-11 {rel} states the exclusion reading as settled while "
                             f"ADR-0007 § 2.1 leaves it open:\n    {f[:140]}")
    notes.append(f"AC-11 {matched} statements of the reading found, {swept} checked "
                 f"after exemptions")
    if matched == 0:
        fails.append("AC-11 found no statement of either reading anywhere — check is "
                     "vacuous. The readings are set out in § 2.1, so zero means the "
                     "pattern no longer matches the document it was written for")

for n in notes: print("  ·", n)
print()
if fails:
    print("FAIL")
    for f in fails: print(" -", f)
    sys.exit(1)
print("== 0 failures ==")
```

## The review round, and the case no check could have raised

Review found that **§ 2's script exclusion was not enforced by the execution
model**. `argv[0]` resolving to a file with a `#!` line means the kernel invokes
its interpreter — no shell involved, no interpreter named by the operator — so a
script executed while the ADR said scripts were excluded.

A second round then found that the fix had **resolved the question in the ADR's
own favour**, which `D-P07-1` forbids. § 2.1 now states both readings and decides
neither, and the question is recorded as blocking C07.

**This file states neither reading as settled**, and says why rather than leaving
it implicit: the boundary's owner has not ruled, and an acceptance-evidence file
that recorded one reading as the outcome would be the same overreach in a quieter
document. What is settled is only what Keystone constructs — no shell invocation,
no wrapped argv, no script body, no file written to execute, no interpreter
chosen.

Three drafts, each wrong in a way worth keeping:

1. The first **asserted an enforcement the design did not have** — that scripts
   and shells could not run.
2. The second **asserted an interpretation the task was not entitled to make** —
   that the exclusions constrain only what Keystone provides.
3. The third left the ADR correct and **left this file stating the second**,
   which is `DL-8`'s dependents tier inside the evidence for the task that
   raised it.

`AC-11` now checks the third: no tracked document may state either reading as
settled while `ADR-0007` § 2.1 says the question is open. It reads the ADR for
whether the question is open rather than being told.

Following the original finding through showed the first claim was wrong in a
larger way than the review stated: `/bin/sh -c '…'` is also just argv, so the shell exclusion was equally
unenforced. § 2.1 now sets out both readings with the
evidence each rests on — Generation 1's removed `--shell bash` flag on one side,
the charter's literal *a shell that interprets the command's argv* on the other
— **and decides neither**.

**No case here could have raised it.** Every case compares the ADR against the
charter, the invariants or another ADR, and the ADR **agreed with all of them**:
it named every excluded form and stated each as excluded. The defect was that
the word "excluded" meant something the design could not deliver, which is a
claim about the world rather than a disagreement between documents.

The nearest a check could get is the one now written into `AC-1`'s limitation
row — *whether an exclusion is enforceable rather than declared* — which was
already there, pointing at C07 and C14. Review reached it sooner.

A second finding was mechanical and is recorded because its cause was this
task's dossier: `RSK-9`'s threat-model row still said *renewed at P06* while the
ADR narrowed it at P07. The dossier's § 2 had not permitted that row, having
inferred from *no residual risk names P07 as its expiry* that P07 would touch no
risk — which does not follow, since a task can change a risk's compensating
control without owning its expiry. The row is updated and the inference is
recorded in the dossier so a later one does not repeat it.

## `AC-11` was added mid-task, and what that cost

It is not in the dossier's original ten. Review found this file still asserting
a reading `ADR-0007` § 2.1 had withdrawn, and `AC-11` is the case written for
that class. The dossier's § 5.2 now defines it, § 5.3 records what it cannot
detect, and this file's tables and counts include it — **because a checker that
enforces more than the acceptance contract says is a contract that has stopped
meaning anything.**

The first version claimed a demonstration that existed only in a terminal. The
commit message said `AC-11`'s case had been demonstrated; it had been run by
hand and never added to the harness, so the demonstration record did not contain
it. That is `DL-4` — the record and the branch going unchecked — in a claim about
evidence.

**Writing the case properly turned up three more defects in it**, each fixed:

1. **§ 2.1's own blocks were being flagged** for presenting the readings the
   section exists to present.
2. **A block reporting what an earlier draft asserted** was read as asserting it.
3. **The vacuity guard counted the wrong thing** — blocks surviving the
   exemptions rather than blocks the pattern matched — so it fired whenever the
   exemptions covered everything, which says nothing about whether the check
   works. Two counters now.

**And then a fourth, which is the interesting one.** `AC-11` was reading **this
file's own embedded checker source** as prose: the fenced Python block contains
its pattern strings and its comments, so the check was asserting things about its
own text. It now strips fenced code before scanning. A check embedded in the
document it checks has to know which parts of that document are the check.

The harness needed a fix too: its change-detection snapshot covered the ADR and
the threat model only, so a case mutating a third file read as *"mutation changed
nothing"*. It now snapshots every file it copies.

## What these cases cannot detect

| Case | Cannot detect | Found instead by |
|---|---|---|
| `AC-1` | Whether an exclusion is **enforceable** rather than declared | C07's executor; C14's adversarial suite |
| `AC-2` | Whether the default is implemented as deny | C07; C14 |
| `AC-3` | Whether a limit's **value** is right | C15's soak; C16's pilot |
| `AC-4` | Whether the escalation reaches every descendant on a real kernel — a descendant that creates its own session escapes the group | C07 and C09, VM-gated; the charter's own success metric |
| `AC-5` | Whether truncation is reported **legibly** | C11's operator UX |
| `AC-6` | Whether the expression **closes** the threat | Review; the security reviewer of `P07.md` § 8 |
| `AC-7` | **Whether the privilege split is the right one.** It checks that the ADR decides and states its limit, not that the answer is safe | The maintainer, who chose it; `RSK-9`'s owner |
| `AC-8` | Whether the mapping is correct for every breach | C07; C14's fault matrix |
| `AC-9` | Whether redaction catches a secret in practice — it cannot, and § 11 says so | Nothing. It is a stated limit, not a gap a case could close |
| `AC-10` | Whether the two limits are consistent under load | C15 |
| `AC-11` | **Whether either reading is right.** It checks that no document settles a question the ADR leaves open, which is a consistency property, not a correctness one | The maintainer, who owns the boundary |

**And one thing no case here reaches**: whether § 13's two findings are the
right two. Both are claims about what *other* accepted ADRs fail to provide, and
a case that read only this document would confirm them by construction. They
were established by reading `ADR-0006` § 1's state table and `ADR-0005` §§ 4 and
5 directly, and the reviewer named in the dossier is who should check them.
