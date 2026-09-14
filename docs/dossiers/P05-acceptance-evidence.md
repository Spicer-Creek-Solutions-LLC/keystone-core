# P05 acceptance evidence

Required by [`P05.md`](P05.md) § 5.3. Every acceptance case is demonstrated
failing before it is accepted, and every case states what it cannot detect.

## Method

The checks read the ADR's **tables** — the classification table, the signature
and encryption tables, the header set, the replay bounds, the vector coverage —
and the four residual-risk rows and their dependents in `THREAT-MODEL.md`.

Each demonstration asserts four things, per `DL-7`: the anchor is unique; the
document changed; **the mutated document actually carries the defect, verified by
parsing the mutated section**; and the checker's failure names the case. P03's
evidence records two plantings whose predicates asserted only that text had
changed; every predicate here parses the section and asserts the property.

## The checker validates its own assumptions

`AC-3` measures the ADR against `ARCH-NATS-006`'s clauses and `AC-8` against
`ADR-0003`'s recording sentence. The checker asserts those phrases still exist
in their sources, whitespace-normalised first because they wrap across lines.

## Three defects the first run found, and which was which

The first run reported three failures. **One was a real gap in the ADR and two
were an inconsistency in how the risks were recorded** — the distinction matters,
because a suite that mixes them teaches its author to discount it.

| Reported | Actually |
|---|---|
| `AC-3` — no expression for "use separate keys" | **Real.** The ADR referenced `AST-4`, `AST-5`, `AST-7` and `AST-8` as distinct keys throughout and **never stated `ARCH-NATS-006`'s first clause**. § 4 now says it outright: a signature is never made with a NATS identity key, encryption never with a signing key |
| `AC-7` — `RSK-1` and `RSK-12` record no P05 outcome | **Real, and my inconsistency.** Both carried "Renewed at P05" in the *compensating control* cell while `RSK-6` and `RSK-11` carried it in the *expiry* cell. P02's evidence records `AC-7` being tightened to read the expiry cell precisely because a whole-row check could not fail; the rows now all record the outcome where the case reads it |

The first is the one worth keeping: **an ADR can use a rule correctly on every
line and never state it**, and a reader looking for the invariant would not have
found it.

## The TESTING.md feature table

All ten cases are `N/A`, per case, for the reasons `P05.md` § 5.1 gives. The
**Payload protection** row is the uncomfortable one: P05's whole subject is
confidentiality and integrity, and it is `N/A` because the ADR describes them
rather than performing them. C01 implements the operations and computes the
vectors.

## Demonstrations

Control: `== 0 failures ==`. Twelve cases across the nine acceptance cases:

| Defect planted | Case |
|---|---|
| A classification attribute is blank for a class | `AC-1` |
| An attribute row is missing entirely | `AC-1` |
| The header set is not stated as complete | `AC-2` |
| An `ARCH-NATS-006` clause has no expression | `AC-3` |
| Cancellation has no signer | `AC-4` |
| The vector coverage omits a dimension | `AC-5` |
| A concrete vector value appears | `AC-5` |
| Forward compatibility states nothing to tolerate | `AC-6` |
| A P05 risk is left undecided | `AC-7` |
| A § 8 row still points at P05 | `AC-7` |
| An encryption recipient names no recorded half | `AC-8` |
| A class has no replay bound | `AC-9` |

Two cases mutate `THREAT-MODEL.md` and restore it in a `finally` block.

**One P01 demonstration went stale and was caught by its anchor assertion.** Its
compensating-control case anchored on `RSK-11`'s control text, which this task
rewrote; it now anchors on `RSK-13`, which P05 does not touch. That is the fourth
time `DL-7`'s first assertion has rejected a stale anchor in this repository.

### What review found that no case could

**The accepted-leakage table omitted the cleartext identifiers the envelope
carries.** § 1 puts the job and correlation identifiers in fields 3 and 4, ahead
of the payload field, and § 7's leakage cells never mentioned them — while § 10
and `RSK-6` both claimed the enumeration was complete and exceeded nothing § 6
already concedes. **That claim was false**, and the shape is familiar: a
completeness sentence written beside an incomplete list, which P04's evidence
records twice.

Two things came out of it.

**One is a design improvement rather than a disclosure.** The correlation
identifier had no reason to be cleartext — nothing outside the two endpoints
needs it — so it now travels **inside the payload** on every encrypted class.
The job identifier stays cleartext because § 8 puts it in a header by an earlier
decision, and hiding it in the envelope while the header carries it would
achieve nothing.

**The other is a concession.** A cleartext job identifier makes the correlation
§ 6 concedes *exact* rather than *inferred*, and a lifecycle event reveals the
job it concerns and a coarse transition, which § 6's table does not list at all.
`RSK-6` is now recorded as **renewed and widened** rather than renewed, in both
its compensating-control and expiry cells.

**And a finding P05 may not fix.** § 6's observable table should gain a row for
cleartext envelope identifiers. P05's dossier permits § 6's *closing sentence*
and `RSK-6`'s row and **not that table**, so the widening is recorded in both
places P05 may write and the row is left to its own task. Widening a risk while
leaving the table it derives from unchanged is half a fix, and it is the half
P05 is allowed.

**No case reaches any of this.** `AC-7` checks each risk is resolved or renewed
with a dated outcome; nothing compares a leakage cell against the envelope's own
field list, and nothing would have noticed that the envelope exposed a field the
classification table did not mention.

## What these cases cannot detect

| Case | Cannot detect | Found instead by |
|---|---|---|
| `AC-1` | Whether an attribute's **value is right**. A class with nine plausible wrong answers passes | Review; C01, when an attribute cannot be implemented |
| `AC-2` | Whether a header is routing-safe **in practice**. A field named innocuously can still carry a secret | Review; `TESTING.md`'s canary scan, which plants secrets and fails before upload |
| `AC-3` | Whether an expression **satisfies** the clause or merely restates it | Review; C01 and C14 |
| `AC-4` | Whether signing cancellations closes `RSK-11`, or appears to | Review; and the `ARCH-NATS-006` amendment, where the gap is settled |
| `AC-5` | Whether the coverage is **sufficient**. The boundary case nobody thought of is invisible here | C01, when it computes vectors and finds an unspecified case; C14's fuzzing |
| `AC-6` | Whether the negotiation is **safe**. A receiver that tolerates too much is a downgrade path, and "tolerate" is stated, not measured | Review; C14 |
| `AC-7` | Whether a renewal is **justified** or a resolution **correct** | The maintainer, who owns each risk by name |
| `AC-8` | Whether the recorded public half is the **right** one for that recipient | C01 and C03 |
| `AC-9` | Whether a replay window is **long enough, or short enough** | C14's adversarial suite |

`AC-1` and `AC-5` carry the most weight and the least certainty: between them
they assert that every message is classified and every vector dimension named,
and neither can tell you an answer is correct or a coverage complete.

**Two limits no case reaches, stated because this ADR rests on them.**
The encryption recipients depend on public halves `ADR-0003` records in one
direction only — the ADR raises that as a finding, and no case can detect that a
document rests on an unspecified input. And `AC-3` checks that each clause of
`ARCH-NATS-006` has an expression; **no case checks that the ADR's additional
requirements are ones the invariant would sanction**, which is exactly the gap
`RSK-11`'s resolution leaves open.

## Reproducing

Save as `p05.py` and run from the repository root:

```
python3 p05.py docs/adr/0005-versioned-encrypted-protocol.md
```

Exit `0` and `== 0 failures ==` is a pass.

```python
import re, sys, pathlib
A = pathlib.Path(sys.argv[1]).read_text()
INV = re.sub(r"\s+", " ", pathlib.Path("docs/project/ARCHITECTURE-INVARIANTS.md").read_text())
THR = pathlib.Path("docs/project/THREAT-MODEL.md").read_text()
A3 = re.sub(r"\s+", " ", pathlib.Path("docs/adr/0003-enrollment-and-identity.md").read_text())
fails = []

def section(pat):
    m = re.search(pat + r"(.*?)(?=\n#{2,3} |\Z)", A, re.S)
    return m.group(1) if m else ""
def rows(pat, skip=()):
    out = []
    for line in re.findall(r"^\|(?!-)(.+)\|$", section(pat), re.M):
        c = [x.strip() for x in line.split("|")]
        if c and c[0].strip("*` ").lower() not in skip: out.append(c)
    return out

CLASSES = ["enrollment request", "enrollment reply", "command", "cancellation",
           "result", "lifecycle event", "presence"]
ATTRS = ["signer", "verifier", "encryption recipient", "replay key / window",
         "maximum size", "durability", "retention", "headers", "accepted metadata leakage"]

# --- AC-1 all classes, all attributes --------------------------------------
cls = section(r"### 7\. Message classification\n")
header = re.search(r"^\|(.+)\|$", cls, re.M)
if not header: fails.append("AC-1 no classification table")
else:
    cols = [c.strip().lower() for c in header.group(1).split("|")]
    for c in CLASSES:
        if c not in cols: fails.append(f"AC-1 message class {c!r} is not a column of the classification table")
    for line in re.findall(r"^\| \*\*(.+?)\*\* \|(.+)\|$", cls, re.M):
        attr, cells = line[0].strip().lower(), [x.strip() for x in line[1].split("|")]
        for i, v in enumerate(cells):
            if not v: fails.append(f"AC-1 attribute {attr!r} is blank for column {i+1}")
    present = [re.sub(r"\*\*", "", m).strip().lower() for m in re.findall(r"^\| \*\*(.+?)\*\* \|", cls, re.M)]
    for a in ATTRS:
        if a not in present: fails.append(f"AC-1 attribute {a!r} is missing from the classification table")

# --- AC-2 headers carry no secret, and the set is closed -------------------
h = section(r"### 8\. Headers\n")
if not h: fails.append("AC-2 no headers section")
else:
    if "No other header is set by Keystone" not in h: fails.append("AC-2 the header set is not stated as complete")
    if not re.search(r"No header carries a secret, a token, or payload plaintext", h):
        fails.append("AC-2 does not state that no header carries a secret, token or plaintext")
    for bad in ["token`", "seed", "argv", "plaintext payload", "password"]:
        for row in re.findall(r"^\| `?([^|`]+)`? \| [^|]+ \| ([^|]+) \|$", h, re.M):
            if bad in row[1].lower() and "never" not in row[1].lower() and "not" not in row[1].lower():
                fails.append(f"AC-2 header {row[0].strip()!r} appears to carry {bad!r}")

# --- AC-3 every ARCH-NATS-006 clause expressed -----------------------------
for clause, where in [("use separate keys", r"separate"),
                      ("Commands are signed by an authorized service", r"Command \| Service signing key"),
                      ("encrypted to the target agent", r"Command \| Agent"),
                      ("Results are signed by the agent", r"Result \| Agent"),
                      ("encrypted to the authorized result service", r"Result service")]:
    if clause not in INV: fails.append(f"AC-3 checker assumption stale: ARCH-NATS-006 no longer contains {clause!r}")
    elif not re.search(where, A): fails.append(f"AC-3 no expression for the clause {clause!r}")

# --- AC-4 cancellation is signed -------------------------------------------
sig = section(r"### 4\. Signatures\n")
row = [r for r in rows(r"### 4\. Signatures\n", skip=("class",)) if r and r[0].strip("*` ").lower() == "cancellation"]
if not row: fails.append("AC-4 cancellation has no row in the signature table")
else:
    if not row[0][1] or "none" in row[0][1].lower(): fails.append("AC-4 cancellation has no signer")
    if len(row[0]) < 3 or not row[0][2]: fails.append("AC-4 cancellation has no verifier")
enc = [r for r in rows(r"### 5\. Recipient encryption", skip=("class",)) if r and r[0].strip("*` ").lower() == "cancellation"]
if not enc: fails.append("AC-4 cancellation has no encryption-recipient row")

# --- AC-5 vector coverage named, no vector value ---------------------------
cov = section(r"### 10\. What a test-vector set must cover\n")
if not cov: fails.append("AC-5 no test-vector coverage section")
else:
    for word in ["Class", "Field boundaries", "Version", "Signature", "Encryption", "Cross-process"]:
        if word not in cov: fails.append(f"AC-5 coverage does not name the {word!r} dimension")
    if "No vector value appears" not in cov: fails.append("AC-5 does not state that no vector value appears")
for m in re.findall(r"`([0-9a-fA-F]{16,})`", A):
    fails.append(f"AC-5 what looks like a vector value appears: `{m[:24]}…`")

# --- AC-6 version negotiation and forward compatibility --------------------
ver = section(r"### 2\. Version negotiation")
fwd = section(r"### 9\. Error codes and forward compatibility\n")
if not re.search(r"unknown version is refused|Must refuse", ver + fwd): fails.append("AC-6 an unknown version's handling is unstated")
if not re.search(r"Must tolerate", fwd): fails.append("AC-6 forward compatibility does not state what must be tolerated")
if re.search(r"best-effort pars", ver + fwd) and not re.search(r"not best-effort pars|Rejected", A):
    fails.append("AC-6 best-effort parsing is permitted somewhere")

# --- AC-7 the four risks, and their dependents ------------------------------
for rid in ["RSK-1", "RSK-6", "RSK-11", "RSK-12"]:
    m = re.search(rf"^\| `{rid}` \|(.+)$", THR, re.M)
    if not m: fails.append(f"AC-7 {rid} is absent from the threat model"); continue
    cells = [c.strip() for c in m.group(1).rstrip("|").split("|")]
    expiry = cells[-1]
    if not re.search(r"Renewed|Resolved", expiry): fails.append(f"AC-7 {rid} expiry records no P05 outcome: {expiry!r}")
    if not re.search(r"\d{4}-\d{2}-\d{2}", expiry): fails.append(f"AC-7 {rid} expiry carries no date")
    if "P05" not in m.group(1): fails.append(f"AC-7 {rid} does not name the deciding task")
if "2027-03-13, or P05" in THR or "2027-09-13, or P05" in THR:
    fails.append("AC-7 a risk still expires on P05")
if "`RSK-12`, P05's" in THR: fails.append("AC-7 a § 8 key-lifecycle row still points at P05")
if "RSK-11`, P05's to close" in THR: fails.append("AC-7 THR-24 still describes RSK-11 as P05's to close")

# --- AC-8 every encryption recipient is a recorded public half -------------
for r in rows(r"### 5\. Recipient encryption", skip=("class",)):
    if len(r) < 2: continue
    to = r[1]
    if to.lower().startswith("**not") or to.lower() == "none": continue
    if not re.search(r"AST-[58]|recorded|public half", to):
        fails.append(f"AC-8 encryption recipient {to!r} names no recorded public half")
if "durably records the agent's three public halves" not in A3:
    fails.append("AC-8 checker assumption stale: ADR-0003 no longer records the agent's public halves")

# --- AC-9 replay bounds per class, never exactly-once ----------------------
rep = section(r"### 6\. Replay bounds")
for c in ["Command", "Cancellation", "Result", "Enrollment request", "Lifecycle event", "Presence"]:
    if not re.search(rf"^\| {re.escape(c)} \|", rep, re.M): fails.append(f"AC-9 no replay bound for {c}")
if re.search(r"exactly-once execution", A) and not re.search(r"forbids|would be claiming|never claim", A):
    fails.append("AC-9 the ADR claims exactly-once execution")

for f in fails: print("FAIL", f)
print(f"== {len(fails)} failures ==")
sys.exit(1 if fails else 0)
```
