# P08 acceptance evidence

The acceptance cases of [`P08.md`](P08.md) § 5.2, each demonstrated failing on a
document that carries its defect, then passing on the documents as they stand.
`AC-9` states two properties and is demonstrated twice: `AC-9b` below is its
second clause, **added in review** — see § "`AC-9` did not check what `AC-9`
said".

Four expected values are never typed here. `ARCH-OBS-001`'s transitions are read
out of the invariant, `ADR-0006` § 3's durable boundaries out of that ADR, the
risk dispositions out of `THREAT-MODEL.md`, and the deferrals out of the ADR's
own table — so no case can agree with a mistake it shares with the document.

## What each case fails on

| Case | Fails when |
|---|---|
| `AC-1` | A transition `ARCH-OBS-001` names has no record, or § 6 has no field table |
| `AC-2` | The stores are not two, an interface over both is not forbidden, or separate schemas and APIs are unstated |
| `AC-3` | A durable boundary `ADR-0006` § 3 states has no transaction scope, or receipt and start are allowed to share one |
| `AC-4` | Retention names no tombstone, no stream bound, or no effect on `ARCH-JOB-003` |
| `AC-5` | Retention may discard either observation of a reconciled `UNKNOWN` |
| `AC-6` | The append discipline is unstated, or claimed to prove more than it does |
| `AC-7` | Fewer than three items are marked never-stored, the harvested environment is not excluded, or the argv limit is unrestated |
| `AC-8` | A store row has no owner or no octal mode, or ownership is not related to `ADR-0007` § 7's split |
| `AC-9` | A risk is undisposed, carries no dated expiry, or the threat model does not record P08's disposition |
| `AC-9b` | Any tracked document states the superseded route to the agent ledger unquoted — that an operator *"reaches it only through the server"* |
| `AC-10` | Fewer than two corruption responses, no fail-closed, no statement of what an operator is told, or a silent repair is permitted |
| `AC-11` | A deferral names no trigger |

## Three mutations that changed text without changing the property

All three were caught by the harness rather than reported as evidence, and all
three are the same shape: **a claim stated more than once, mutated in one place.**

`AC-4`'s was the instructive one. The retention inequality is stated three times
— as a code block, as prose, and as a table row — and the first mutation removed
only the code block. The case still passed, because two statements of the bound
survived.

Repairing it exposed a second defect in the mutation itself: two of the three
literal replacements **silently matched nothing**, because the prose wraps across
lines and the literal did not. `.replace()` returning the string unchanged is not
an error, so the mutation reported success while changing one thing. The
replacements are now regexes anchored on structure, each asserting it substituted
exactly once.

That is the third time on this branch that a remembered literal failed against
wrapped prose. The rule that keeps working: **anchor a mutation on structure and
assert the substitution count.**

## Demonstration record

```text
unmutated tree: PASS
AC-1   fails as expected  — a transition ARCH-OBS-001 names has no record
        - AC-1 § 6 does not name ARCH-OBS-001's transition 'result retrieval'
AC-2   fails as expected  — an interface over both stores is not forbidden
        - AC-2 § 1 does not forbid an interface over both
AC-3   fails as expected  — receipt and start are allowed to share a transaction
        - AC-3 § 5 does not state that receipt and start are separate transactions
AC-4   fails as expected  — retention states no bound against the stream's max age
        - AC-4 § 7 does not name the stream's max age
        - AC-4 § 7 states no inequality binding retention to the stream
AC-5   fails as expected  — retention may discard one of the two observations
        - AC-5 § 7 does not keep both observations of a reconciled UNKNOWN
AC-6   fails as expected  — append-only is claimed to prove more than it does
        - AC-6 § 6 does not state what append-oriented storage does not prove
        - AC-6 § 6 does not distinguish the discipline from evidence
AC-7   fails as expected  — the harvested environment is no longer excluded
        - AC-7 § 9 marks only 2 items as never stored
        - AC-7 § 9 does not exclude the harvested environment
AC-8   fails as expected  — a store has no octal mode
        - AC-8 § 4's 'Agent ledger' row has no octal mode: ['restricted']
AC-9   fails as expected  — a risk is left without a dated expiry
        - AC-9 RSK-4 carries no dated expiry
AC-9b  fails as expected  — a dependent site still describes the superseded state
        - AC-9b docs/project/THREAT-MODEL.md still states the superseded route to the agent ledger: ...reaches it only through the se
AC-10  fails as expected  — a corrupt store is repaired silently
        - AC-10 § 10 does not forbid a silent repair
AC-11  fails as expected  — a deferred decision point names no trigger
        - AC-11 the deferral '**Postgres** in place of SQLite on the s' names no trigger
```

## The checker

```python
#!/usr/bin/env python3
"""P08 acceptance cases against ADR-0008, read structurally."""
import re, sys, pathlib

A = pathlib.Path(sys.argv[1]).read_text()
ROOT = pathlib.Path(sys.argv[2] if len(sys.argv) > 2 else ".")
INV = (ROOT / "docs/project/ARCHITECTURE-INVARIANTS.md").read_text()
TM  = (ROOT / "docs/project/THREAT-MODEL.md").read_text()
A6  = (ROOT / "docs/adr/0006-delivery-and-job-lifecycle.md").read_text()
A5  = (ROOT / "docs/adr/0005-versioned-encrypted-protocol.md").read_text()
A7  = (ROOT / "docs/adr/0007-safe-execution.md").read_text()
fails, notes = [], []
flat = " ".join(A.split())

def section(pat, doc=None):
    m = re.search(pat + r".*?(?=\n### |\n## |\Z)", doc or A, re.S)
    return m.group(0) if m else ""

def rows(pat, doc=None, ncols=None):
    out = []
    for line in section(pat, doc).splitlines():
        if not line.startswith("|"): continue
        c = [x.strip() for x in line.strip().strip("|").split("|")]
        if not c or set("".join(c)) <= set("-: "): continue
        if c[0].lower() in ("store", "record", "path", "field", "ordering", "what",
                            "table group", "item", "risk", "invariant", "deferred",
                            "before this happens", ""): continue
        if ncols and len(c) != ncols: continue
        out.append(c)
    return out

# --- AC-1 every ARCH-OBS-001 transition has a record ----------------------
o = " ".join(section(r"### ARCH-OBS-001 —", INV).split())
trans = re.search(r"Auditable lifecycle (.*?) emit correlated audit records", o).group(1)
trans = [t.strip() for t in re.split(r",| and ", trans) if t.strip()]
notes.append(f"ARCH-OBS-001 names {len(trans)} transitions")
if len(trans) < 5: fails.append("AC-1 read too few transitions — check is vacuous")
s6 = " ".join(section(r"### 6\. The audit record").split()).lower()
for t in trans:
    if t.lower() not in s6:
        fails.append(f"AC-1 § 6 does not name ARCH-OBS-001's transition {t!r}")
if not rows(r"### 6\. The audit record"):
    fails.append("AC-1 § 6 has no record-field table")

# --- AC-2 two stores, separate, and no abstraction ------------------------
st = rows(r"### 1\. Two stores")
if len(st) != 2:
    fails.append(f"AC-2 § 1 names {len(st)} stores, expected two")
s1 = " ".join(section(r"### 1\. Two stores").split())
if not re.search(r"no interface over both|There is \*\*no interface", s1, re.I):
    fails.append("AC-2 § 1 does not forbid an interface over both")
if not re.search(r"separate schemas and separate APIs", s1):
    fails.append("AC-2 § 1 does not state separate schemas and APIs")

# --- AC-3 every ADR-0006 § 3 boundary has a store and a transaction scope --
b6 = [r for r in rows(r"### 3\. Durable boundaries", A6) if len(r) == 2]
notes.append(f"ADR-0006 § 3 states {len(b6)} durable boundaries")
if len(b6) < 4: fails.append("AC-3 read too few boundaries — check is vacuous")
tx = rows(r"### 5\. Migrations and transaction boundaries")
if not tx:
    fails.append("AC-3 § 5 has no transaction-boundary table")
txflat = " ".join(" ".join(r) for r in tx).lower()
for b in b6:
    # Compare on every distinctive token, not the first three words: the
    # PubAck boundary begins "Everything above, and …" and its distinguishing
    # token is the last one.
    key = [k for k in re.sub(r"[^a-z ]", "", b[1].lower()).split() if len(k) > 4]
    if not any(k in txflat for k in key):
        fails.append(f"AC-3 § 5 states no transaction scope for the boundary {b[1][:48]!r}")
if not re.search(r"must not share a transaction", flat):
    fails.append("AC-3 § 5 does not state that receipt and start are separate transactions")

# --- AC-4 retention, and its effect on ARCH-JOB-003 -----------------------
r7 = " ".join(section(r"### 7\. Retention").split())
for need, why in [(r"tombstone", "a tombstone"), (r"KS_CMD max age", "the stream's max age"),
                  (r"ARCH-JOB-003", "the invariant it could lapse")]:
    if not re.search(need, r7, re.I):
        fails.append(f"AC-4 § 7 does not name {why}")
if not re.search(r">\s*`?KS_CMD`? max age|outlives", r7, re.I):
    fails.append("AC-4 § 7 states no inequality binding retention to the stream")
if not rows(r"### 7\. Retention"):
    fails.append("AC-4 § 7 has no retention table")

# --- AC-5 both observations survive retention ----------------------------
if not re.search(r"[Bb]oth observations.{0,80}(survive|share the audit floor)", r7):
    fails.append("AC-5 § 7 does not keep both observations of a reconciled UNKNOWN")
if "ADR-0006` § 12" not in r7:
    fails.append("AC-5 § 7 does not cite ADR-0006 § 12")

# --- AC-6 append-oriented, and what it does not prove --------------------
s6full = " ".join(section(r"### 6\. The audit record").split())
if not re.search(r"append-oriented|written once and never updated", s6full, re.I):
    fails.append("AC-6 § 6 does not state the append discipline")
if not re.search(r"does not prove", s6full):
    fails.append("AC-6 § 6 does not state what append-oriented storage does not prove")
if not re.search(r"discipline, not evidence|not the tamper-evidence", s6full):
    fails.append("AC-6 § 6 does not distinguish the discipline from evidence")

# --- AC-7 nothing forbidden is stored, argv limit named ------------------
s9 = rows(r"### 9\. Sensitive data")
if len(s9) < 4:
    fails.append(f"AC-7 § 9 has {len(s9)} rows — check is vacuous")
never = [r for r in s9 if "never" in r[1].lower()]
if len(never) < 3:
    fails.append(f"AC-7 § 9 marks only {len(never)} items as never stored")
f9 = " ".join(section(r"### 9\. Sensitive data").split())
if not re.search(r"harvested login environment", f9, re.I):
    fails.append("AC-7 § 9 does not exclude the harvested environment")
if not re.search(r"secret placed in argv", f9):
    fails.append("AC-7 § 9 does not restate ADR-0007 § 11's argv limit")

# --- AC-8 ownership and modes per store, against the split ---------------
fs = rows(r"### 4\. Filesystem ownership and modes")
if len(fs) < 4:
    fails.append(f"AC-8 § 4 has {len(fs)} rows — check is vacuous")
for r in fs:
    if len(r) < 3 or not re.match(r"`?0[0-7]{3}`?$", r[2].strip("` ")):
        fails.append(f"AC-8 § 4's {r[0]!r} row has no octal mode: {r[2:3]}")
    if not r[1].strip():
        fails.append(f"AC-8 § 4's {r[0]!r} row has no owner")
f4 = " ".join(section(r"### 4\. Filesystem ownership and modes").split())
if not re.search(r"executor holds root and does not hold the ledger", f4):
    fails.append("AC-8 § 4 does not relate ownership to ADR-0007 § 7's split")

# --- AC-9 the three risks, disposed, with dependents ---------------------
rr = {r[0].strip("`* "): r[1] for r in rows(r"## Residual risks") if len(r) == 2}
for rid in ("RSK-1", "RSK-4", "RSK-9"):
    if rid not in rr:
        fails.append(f"AC-9 the ADR does not dispose of {rid}")
        continue
    if not re.search(r"\*\*(Renewed|Resolved)", rr[rid]):
        fails.append(f"AC-9 {rid}'s disposition is neither renewed nor resolved")
    if not re.search(r"\b20\d\d-\d\d-\d\d\b", rr[rid]):
        fails.append(f"AC-9 {rid} carries no dated expiry")
    row = re.search(rf"^\| `{rid}` \|(.*)$", TM, re.M)
    if not row:
        fails.append(f"AC-9 {rid} has no row in THREAT-MODEL.md")
    elif "P08" not in [c.strip() for c in row.group(1).split("|")][-2] and \
         "at P08" not in row.group(1):
        fails.append(f"AC-9 the threat model does not record P08's disposition of {rid}")
notes.append(f"AC-9 disposed: {sorted(rr)}")

# RSK-4's control changed a conclusion, not an identifier: the operator's route
# to the agent ledger no longer runs through the server. DL-8's third tier says
# to sweep the conclusion's SUBJECT and encode it as an assertion over every
# tracked file, which is what this is. Every hit is classified and none is
# filtered out: a hit is legitimate only if negated or quoted as superseded.
ROUTE = re.compile(
    r"reach\w*\s+(?:it|them|those ledgers?|the ledgers?)\s+only through the server", re.I)
hits = []
for f in sorted(ROOT.rglob("*.md")):
    if ".git" in f.parts:
        continue
    body = re.sub(r"^```.*?^```", "", f.read_text(), flags=re.S | re.M)   # not our own source
    units = []                     # a table row is its own unit (DL-8, third tier, step 4)
    for para in re.split(r"\n\s*\n", body):
        rows_, prose = [], []
        for ln in para.splitlines():
            (rows_ if ln.lstrip().startswith("|") else prose).append(ln)
        units += [" ".join(r.split()) for r in rows_]
        if prose:
            units.append(" ".join(" ".join(prose).split()))
    for flat in units:
        for m in ROUTE.finditer(flat):
            before = flat[:m.start()]
            if re.search(r"no longer\s*$", before):
                hits.append((f, "negated"))
            elif before.count('"') % 2 == 1:
                hits.append((f, "quoted as superseded"))
            else:
                hits.append((f, "LIVE"))
                fails.append(
                    f"AC-9b {f} still states the superseded route to the agent ledger: "
                    f"...{flat[m.start():m.end()]}")
if len(hits) < 3:
    fails.append(f"AC-9b the ledger-route sweep found {len(hits)} sites — it is vacuous")
notes.append(f"AC-9b ledger-route sweep: {[(str(f), d) for f, d in hits]}")

# --- AC-10 corruption response, per store, and what the operator is told --
c10 = rows(r"### 10\. Backup and corruption")
if len(c10) < 2:
    fails.append(f"AC-10 § 10 gives {len(c10)} corruption responses, expected one per store")
f10 = " ".join(section(r"### 10\. Backup and corruption").split())
if not re.search(r"[Ff]ail closed", f10):
    fails.append("AC-10 § 10 does not state the agent's response")
if not re.search(r"operator is told", f10):
    fails.append("AC-10 § 10 does not say what an operator is told")
if not re.search(r"never repaired silently", f10):
    fails.append("AC-10 § 10 does not forbid a silent repair")

# --- AC-11 every deferral names a trigger --------------------------------
d = rows(r"### Deferred decision points")
if len(d) < 2:
    fails.append(f"AC-11 {len(d)} deferrals listed — check is vacuous")
for r in d:
    if len(r) < 2 or len(r[1]) < 25:
        fails.append(f"AC-11 the deferral {r[0][:40]!r} names no trigger")
notes.append(f"AC-11 {len(d)} deferrals, each with a trigger")

for n in notes: print("  ·", n)
print()
if fails:
    print("FAIL")
    for f in fails: print(" -", f)
    sys.exit(1)
print("== 0 failures ==")
```

## `AC-9` did not check what `AC-9` said

Review of pull request #323 found `THR-46` still carrying the qualifier this ADR
removes: the agent ledger is an independent record *"but the operator reaches it
only through the server"*. `RSK-4`'s own row, four hundred lines further down and
in the same commit, says the opposite.

**The dossier had already found this site.** § 2's dependent-site table names
`THR-44`, `THR-45`, `THR-46` and `THR-49` explicitly, derived by word-boundary
search after a first enumeration missed eight sites. The derivation was right and
was not applied.

**And `AC-9` was written to catch exactly that.** Its wording in § 5.2 is *"...
**and every dependent site in § 2's table is updated**"*, failing when *"a
dependent describes a superseded state"*. The checker implemented the first half
of that sentence and none of the second, while its own comment read *"the three
risks, disposed, with dependents"*. So the case passed on a document that
violated the property it named, and the comment asserted the coverage the code
did not have.

That is two ledger classes in one place. `DL-1` — a check that could not fail on
the property it claimed — is why review had to find it rather than the harness.
`DL-8`'s third tier is why it was there to find: the tier says to sweep the
conclusion's **subject** and **encode that as an assertion over every tracked
file, not as a search you perform and describe**. § 2's table is a search
performed and described. Both are recorded in
[`DEFECT-LEDGER.md`](../project/DEFECT-LEDGER.md).

`AC-9b` is the assertion that was owed. It sweeps every tracked Markdown file for
the superseded route, classifies every hit as negated, quoted-as-superseded, or
live, and fails on any live one. **Nothing is filtered**: the four current hits
are listed in the checker's own notes, which is how the sweep is reviewed rather
than trusted.

It found two further sites while being written — one of them a row this very
file had just added — and it is scoped to the one conclusion P08 changed. The
next conclusion still has no assertion, which § "What these cases cannot detect"
now records.

## Two checker defects the first run found

- **A table header was read as a durable boundary.** `ADR-0006` § 3's header row
  reached the comparison as a boundary named *"This must be durable"*, which no
  transaction scope could satisfy.
- **The boundary comparison sliced to the first three words.** The `PubAck`
  boundary begins *"Everything above, and …"*, so its distinguishing token was
  never compared and a covered boundary reported as uncovered. It now compares
  every token of five characters or more.

Both produced false failures rather than false passes, which is the safer
direction — but a case that cries wolf is a case whose next real failure gets
waved through.

## What these cases cannot detect

| Case | Cannot detect | Found instead by |
|---|---|---|
| `AC-1` | Whether a record's **content** is enough to reconstruct what happened | Review; C12; C16's pilot, where an operator asks a real question |
| `AC-2` | Whether the separation survives implementation — a shared helper is a mega-store with extra steps | C02; the lint P11 may set up |
| `AC-3` | Whether the schema is **achievable** at acceptable cost | C02; C15's soak |
| `AC-4` | Whether the retention **periods** are right, only that the inequality is stated | C15; C16 |
| `AC-5` | Whether reconciliation is implemented as specified | C02; C08 |
| `AC-6` | **Whether append-oriented storage is append-only in fact.** A store the server owns is append-oriented by convention | Nothing in `v0.6.0` — which is why § 13 defers tamper-evidence with a trigger rather than claiming it |
| `AC-7` | Whether a secret reaches a record in practice, since argv is operator-supplied | `TESTING.md`'s canary scan; and § 9 states it as a limit |
| `AC-8` | Whether the modes are correct on a real filesystem | C13; VM tests |
| `AC-9` | Whether a renewal is **justified** | The maintainer, who owns each risk by name |
| `AC-9b` | Any **other** superseded conclusion. It asserts over the one conclusion P08 changed; the next one has no assertion yet | Review, and the next task that changes a conclusion |
| `AC-10` | Whether the response is right operationally — failing closed converts a storage fault into an outage | C14's fault matrix; C16 |
| `AC-11` | Whether a trigger is one anybody will notice | Review |

**And one thing no case reaches**: whether § 3's independently readable ledger
actually lets an operator reconstruct a false server's account. That is the
claim `RSK-4`'s strengthened control now rests on, and it is a claim about a
procedure nobody has performed. **P10** is the first task that could demonstrate
it, which is why all three risks now expire there rather than here.
