# P04 acceptance evidence

Required by [`P04.md`](P04.md) § 5.3. Every acceptance case is demonstrated
failing before it is accepted, and every case states what it cannot detect.

## Method

The checks read the ADR's **tables** — the subject-class table, the permission
matrix, the allowlist, the two case lists, and the deferral-closure table — and
compare `AC-4` and `AC-5` against their source documents rather than against a
hardcoded list. That is `DL-2`'s countermeasure applied before the fact.

Each demonstration asserts four things, per `DL-7`: the anchor is unique; the
document changed; **the mutated document actually carries the defect, verified by
parsing the mutated section**; and the checker's failure names the case.

P03's evidence recorded two plantings that satisfied the first two assertions and
not the third, because their predicates asserted that text had changed rather
than that the defect was present. **Every predicate here parses the mutated
section and asserts the property.** One anchor was still wrong — the `AC-4`
planting missed that `NEG-7`'s row carries "— the connection itself must fail" —
and assertion 1 rejected it before it could report a passing run.

## The checker validates its own assumptions

`AC-4` measures the ADR against `ARCH-TEST-002`'s five clauses and `AC-5`
against `ADR-0002`'s and `ADR-0003`'s deferral sentences. The checker asserts
those phrases still exist in their sources, whitespace-normalised first because
they wrap across lines — the `DL-8` hazard that bit P02's checker on its first
run and P03's on its first run.

**That assertion fired here too**, on a phrase I had guessed rather than copied:
`ADR-0002` says "P04 turns this into the canonical grammar", not "the canonical
grammar is P04's". The checker measured the ADR against a sentence that did not
exist, and said so instead of passing.

## Three false positives the first run produced, and what they mean

The first run reported eleven failures. **Eight were the checker being wrong,
not the ADR**, and the distinction is worth recording because a suite that
cries wolf gets disabled.

| Reported | Actually |
|---|---|
| Eight "subject does not parse" failures | The checker treated **permission patterns** (`ks.enroll.>`, `ks.job.*`) and **deliberate counterexamples** (the rejected class-before-identifier ordering) as declared subjects |
| `AC-5` assumption stale, twice | Real: the checker's phrase was guessed, not copied |
| `AC-8` flags `make check` | The repository's own gate, not a claim the ADR runs anything |

The fix separates **declared** subjects — § 3's class table, which must be exact
four-token subjects — from **exercised** ones, the case tables, which legitimately
contain wildcards because a denial is stated against a pattern. Permission
patterns get a looser derivability check of their own rather than an exemption.

**One ADR change came out of it rather than a checker change**: § 5 stated the
rejected ordering's subjects in backticks, and they are now written without,
because they are not subjects of this system. Loosening the checker to accept
them would have made it unable to catch a real non-conforming subject, which is
`DL-1`.

## The TESTING.md feature table

All ten cases are `N/A`, per case, for the reasons `P04.md` § 5.1 gives. The
**Authorization denial** row is the uncomfortable one: P04's whole subject is
what must be refused, and the case is `N/A` because the ADR names refusals
rather than producing them. C03 runs the Docker negative identity matrix against
generated production JWTs.

## Demonstrations

Control: `== 0 failures ==`. Ten cases across the eight acceptance cases:

| Defect planted | Case |
|---|---|
| A declared subject does not parse under the grammar | `AC-1` |
| The grammar is stated twice, differently | `AC-1` |
| A principal is absent from the matrix | `AC-2` |
| An agent holds a wildcard across identifiers | `AC-3` |
| An `ARCH-TEST-002` clause has no negative case | `AC-4` |
| A deferred role has no expression | `AC-5` |
| The deferral table drops rows | `AC-5` |
| No per-agent `FilterSubject` is stated | `AC-6` |
| A case omits its direction | `AC-7` |
| The ADR claims to run something | `AC-8` |

## What these cases cannot detect

| Case | Cannot detect | Found instead by |
|---|---|---|
| `AC-1` | Whether the grammar is **well designed** — whether its shape survives a rename, a second deployment, or a class nobody has thought of | Review; C03, where an awkward grammar becomes awkward configuration |
| `AC-2` | Whether a principal's permissions are the **right** ones. It checks presence, direction and scope, not authority | Review; C03's negative identity matrix |
| `AC-3` | Whether a documented service wildcard is **necessary**, as opposed to documented | Review — the four wildcards are the whole of `ARCH-NATS-003`'s exception and each should be argued |
| `AC-4` | Whether a named negative case is **implementable as written**. `NEG-7` is the warning: a revoked identity fails at connection, not at publish, and a case that tests the wrong layer proves something weaker | C03, when it cannot implement the case |
| `AC-5` | Whether an expression **means** what the prior ADR meant by that role. It checks that a row exists with a non-empty expression | Review against `ADR-0002` § 4 and `ADR-0003` § 3 |
| `AC-6` | Whether the multi-filter account is **correct**, only that one is given | Review; C03 |
| `AC-7` | Whether the expected outcome is the **right** outcome | Review; C14's adversarial suite |
| `AC-8` | A claim executable in substance while avoiding the vocabulary — "the matrix below is validated" states no path and still overclaims | Review |

`AC-4` and `AC-5` carry the most weight. Between them they assert that every
mandated refusal is named and every deferred role is expressed — and neither can
tell you a case is implementable or an expression faithful.

**A limit `AC-1` cannot reach, stated because this ADR depends on it.** The
grammar's `<id>` token is constrained here, and § 1 records that `ADR-0003`
establishes no subject-safe agent identifier for it to hold. No case can detect
that the grammar rests on an unspecified input; the finding is in the ADR and
raised to the maintainer.

## Reproducing

Save as `p04.py` and run from the repository root:

```
python3 p04.py docs/adr/0004-subject-authorization.md
```

Exit `0` and `== 0 failures ==` is a pass.

```python
import re, sys, pathlib
A = pathlib.Path(sys.argv[1]).read_text()
INV = re.sub(r"\s+", " ", pathlib.Path("docs/project/ARCHITECTURE-INVARIANTS.md").read_text())
A2 = pathlib.Path("docs/adr/0002-nats-native-capabilities.md").read_text()
A3 = pathlib.Path("docs/adr/0003-enrollment-and-identity.md").read_text()
fails = []

def section(pat):
    m = re.search(pat + r"(.*?)(?=\n#{2,3} |\Z)", A, re.S)
    return m.group(1) if m else ""
def rows(pat, skip=()):
    out = []
    for line in re.findall(r"^\|(?!-)(.+)\|$", section(pat), re.M):
        c = [x.strip() for x in line.split("|")]
        if c and c[0].lower() not in skip: out.append(c)
    return out

GRAMMAR = re.compile(r"^ks\.(job|out|enroll)\.(\*|<[^>]+>|[a-z0-9-]{1,64})\.(cmd|cancel|result|event|presence|request|reply|\>)$")

# --- AC-1 the grammar is stated once, and every Keystone subject parses -----
if len(re.findall(r"^ks\.<plane>\.<id>\.<class>$", A, re.M)) != 1:
    fails.append("AC-1 the grammar is not stated exactly once in its canonical form")
ALT = section(r"## Alternatives considered\n")   # shows a rejected shape on purpose
DECLARED = section(r"### 3\. Every subject class\n")          # must be exact four-token subjects
EXERCISED = section(r"### 8\. Positive authorization cases\n") + section(r"### 9\. Negative authorization cases\n")
subjects = {tok for tok in re.findall(r"`([^`\n]+)`", DECLARED) if tok.startswith("ks.")}
CONCRETE = DECLARED + EXERCISED
patterns  = {tok for tok in re.findall(r"`([^`\n]+)`", A) if tok.startswith("ks.")} - subjects
for bad in sorted(p for p in patterns if p in ALT):
    pass  # the rejected alternative is quoted deliberately; § Alternatives is excluded and this line records that
if not subjects: fails.append("AC-1 no Keystone subjects appear at all")
for s in sorted(subjects):
    body = s.replace("<its own id>", "x").replace("<another agent>", "x").replace("<any agent>", "x") \
            .replace("<its own token>", "x").replace("<any token>", "x").replace("<agent>", "x").replace("<token>", "x")
    if not GRAMMAR.match(body):
        fails.append(f"AC-1 subject does not parse under the stated grammar: `{s}`")
PATTERN = re.compile(r"^ks\.(job|out|enroll)(\.(\*|<[^>]+>|[a-z0-9-]{1,64})(\.(cmd|cancel|result|event|presence|request|reply|\*|>))?)?(\.>|\.\*)?$")
for s in sorted(patterns - set(re.findall(r"`([^`\n]+)`", ALT))):
    body = re.sub(r"<[^>]+>", "x", s)
    if not PATTERN.match(body):
        fails.append(f"AC-1 permission pattern is not derivable from the grammar: `{s}`")

# --- AC-2 every ARCH-NATS-002 principal appears with both directions --------
PRINCIPALS = ["command publisher", "enrollment service", "result consumer",
              "presence consumer", "monitoring role", "agent", "bootstrap identity"]
matrix = rows(r"### 4\. The principal-by-subject permission matrix\n", skip=("principal",))
for name in PRINCIPALS:
    hit = [r for r in matrix if r[0].strip("* ").lower() == name]
    if not hit: fails.append(f"AC-2 principal {name!r} is absent from the matrix"); continue
    for r in hit:
        if len(r) < 4: fails.append(f"AC-2 {name} row is malformed"); continue
        if not r[1]: fails.append(f"AC-2 {name} has no publish entry (use 'none')")
        if not r[2]: fails.append(f"AC-2 {name} has no subscribe entry (use 'none')")
        if not r[3]: fails.append(f"AC-2 {name} has no scope")

# --- AC-3 no wildcard across agent identifiers without documentation -------
for r in matrix:
    if len(r) < 4: continue
    perms = r[1] + " " + r[2]
    for w in re.findall(r"`ks\.(?:job|out|enroll)\.\*\.[a-z]+`", perms):
        if "documented" not in r[3].lower():
            fails.append(f"AC-3 {r[0]} holds {w} with no documented exception in its scope cell")
    if r[0].strip("* ").lower() in ("agent", "bootstrap identity"):
        if re.search(r"`ks\.(?:job|out|enroll)\.\*", perms):
            fails.append(f"AC-3 {r[0]} holds a wildcard across identifiers, which ARCH-NATS-003 denies")

# --- AC-4 all five ARCH-TEST-002 clauses appear as named negative cases -----
CLAUSES = ["an agent cannot publish commands",
           "cannot consume another agent's commands",
           "cannot publish another agent's results",
           "cannot use revoked bootstrap credentials",
           "cannot access broker administration subjects"]
for phrase in ["an agent cannot publish commands, consume another agent's commands, publish another agent's results, use revoked bootstrap credentials, or access broker administration subjects"]:
    if phrase.replace("an agent ", "") not in INV.replace("Tests prove that an agent ", ""):
        fails.append("AC-4 checker assumption stale: ARCH-TEST-002's clause list has changed")
neg = section(r"### 9\. Negative authorization cases\n")
for c in CLAUSES:
    if c.lower() not in neg.lower(): fails.append(f"AC-4 no negative case marked for {c!r}")
if len(re.findall(r"^\| `NEG-\d+` \|", neg, re.M)) < 10:
    fails.append("AC-4 fewer than ten negative cases; the five clauses need more than one case each to be meaningful")

# --- AC-5 every deferred role has an expression -----------------------------
closed = rows(r"## 10\. Every deferral, closed\n", skip=("deferred by",))
if len(closed) < 12: fails.append(f"AC-5 the deferral table has {len(closed)} rows; ADR-0002 § 4 names ten and ADR-0003 § 3 names two")
for r in closed:
    if len(r) < 3 or not r[2]: fails.append(f"AC-5 deferred role {r[1] if len(r)>1 else '?'} has no expression")
for src, label, phrase in ((A2, "ADR-0002", "P04 turns this into the canonical grammar"),
                           (A3, "ADR-0003", "grammar and the permission matrix are P04's")):
    if phrase not in re.sub(r"\s+", " ", src):
        fails.append(f"AC-5 checker assumption stale: {label} no longer defers the grammar to P04")

# --- AC-6 one exact FilterSubject per consumer ------------------------------
filt = section(r"### 5\. The consumer filter")
if "FilterSubject: ks.job.<agent>.>" not in A: fails.append("AC-6 no per-agent FilterSubject is stated")
if not re.search(r"multi-filter", filt, re.I): fails.append("AC-6 multi-filter behaviour is not accounted for")

# --- AC-7 every case names principal, subject, direction, outcome ----------
for sec, tag in ((r"### 8\. Positive authorization cases\n", "POS"), (r"### 9\. Negative authorization cases\n", "NEG")):
    for r in rows(sec, skip=("#",)):
        if not re.match(rf"`{tag}-\d+`", r[0]): continue
        if len(r) < 5: fails.append(f"AC-7 {r[0]} omits one of principal, subject, direction, outcome"); continue
        for i, part in enumerate(("principal", "subject", "direction", "outcome"), start=1):
            if not r[i]: fails.append(f"AC-7 {r[0]} has no {part}")
        if not re.search(r"publish|subscribe", r[3], re.I): fails.append(f"AC-7 {r[0]} names no direction")

# --- AC-8 nothing is stated as executable ----------------------------------
for pat, why in [(r"`[^`\n]*_test\.go`", "a test path"), (r"\bgo test\b", "a command"),
                 (r"\bmake (?!check\b)[a-z-]+\b", "a command"), (r"we (?:run|ran|generate[d]?|sign(?:ed)?) ", "an executed action")]:
    for m in re.findall(pat, A, re.I):
        fails.append(f"AC-8 the ADR states {why}: {m!r}")
if re.search(r"this ADR (?:generates|signs|runs|executes)", A, re.I):
    fails.append("AC-8 the ADR claims to generate, sign, run or execute")

for f in fails: print("FAIL", f)
print(f"== {len(fails)} failures ==")
sys.exit(1 if fails else 0)
```
