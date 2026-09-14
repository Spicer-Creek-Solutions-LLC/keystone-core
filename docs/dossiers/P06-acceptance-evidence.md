# P06 acceptance evidence

The eleven cases of [`P06.md`](P06.md) § 5.2, each demonstrated failing on an
`ADR-0006` that carries its defect, then passing on the document as it stands.

The checks read the ADR's **tables** — states, transitions, the lifecycle table,
the invariant coverage — not its prose. `AC-2` is the only case in this
workstream that asks whether what was written is *coherent* rather than whether
it was written: it parses both state machines and walks the transition graph.

## What each case fails on

| Case | Fails when |
|---|---|
| `AC-1` | An `ARCH-JOB-*` clause is cited without an expression, or its coverage row names no section |
| `AC-2` | A transition names an undeclared state, a non-terminal state has no way out, or a state is unreachable from the entry state |
| `AC-3` | A terminal column is not `Yes`/`No`, no terminal state exists, or the transition set is not stated as complete |
| `AC-4` | `UNKNOWN` is not a state, what it may not be rendered as is unstated, reconciliation drops an observation, or the three paths are not enumerated |
| `AC-5` | Redelivery does not express `MaxDeliver` and `BackOff`, is not expressed against `ADR-0002`, or a redelivery may cause a second execution |
| `AC-6` | The deduplication window's bound or limit is unstated, the ledger is not the authority, or the window is not distinguished from an exactly-once claim |
| `AC-7` | `Undelivered` is not a state, the exhaustion advisory is unnamed, expiry is not distinguished by its mechanism, or the broker property the distinction rests on is unstated |
| `AC-8` | A lifecycle row has a blank attribute, or the table's states differ from § 1's |
| `AC-9` | `RSK-9` is not renewed or resolved, its expiry is not a date, or the threat model does not record P06's disposition |
| `AC-10` | Fewer than four cancellation arrivals, the crash boundary is unaddressed, or it does not cite `THR-25` |
| `AC-11` | § 11 does not name both outcomes, or gives no resulting state |

## Two defects the demonstrations found in the work itself

**An internal contradiction in the ADR.** § 8 said a redelivery may not cause a
second execution "including one where the agent has no ledger entry for a job it
did execute", and § 7's first row says an empty ledger means *record receipt and
start*. The agent cannot distinguish a lost entry from a first delivery, so the
claim was false. § 8 now states the guarantee's limit instead: at-most-one holds
exactly as far as the ledger's durability holds, and a deliberately erased
ledger is `THR-48` inside `RSK-9`, which adds nothing to that attacker.

This was found by re-reading § 8 against § 7 rather than by any case. No check
here compares two sections for consistency, and that is `AC-1`'s and `AC-2`'s
blind spot stated plainly.

**Two demonstration anchors spanned more than their case.** `AC-6`'s regex ran
from § 6 into *Alternatives considered* and deleted §§ 7 to 12; `AC-7`'s removed
one of two statements of the same property, leaving the document correct and the
case passing. Both were rejected by assertions rather than reported as evidence
— the first by a new harness assertion that a mutation must leave the document's
heading structure unchanged, the second by the per-case assertion that the
mutated document actually carries the defect.

`DL-7`'s third assertion is what caught the second, and the heading check is a
generalisation of the first worth carrying forward: **a mutation that removes a
heading is editing more than its case.**

## Demonstration record

```text
unmutated tree: PASS
AC-1   fails as expected  — an ARCH-JOB clause is cited without being expressed
        - AC-1 ARCH-JOB-003 is not claimed as satisfied: **registered**
AC-2   fails as expected  — a state is unreachable from the entry state
        - AC-2 server state 'Undelivered' is unreachable from 'Accepted'
AC-3   fails as expected  — the transition set is not stated as complete
        - AC-3 the transition set is not stated as complete
AC-4   fails as expected  — late-result reconciliation discards an observation
        - AC-4 late-result reconciliation does not retain both observations
AC-5   fails as expected  — redelivery is permitted to cause a second execution
        - AC-5 the ADR does not forbid a redelivery causing a second execution
AC-6   fails as expected  — the dedup window is not distinguished from an exactly-once claim
        - AC-6 the window is not distinguished from an exactly-once claim
AC-7   fails as expected  — the broker property the two-case distinction rests on is unstated
        - AC-7 the broker property the distinction rests on is not stated
AC-8   fails as expected  — the lifecycle table omits a declared state
        - AC-8 the lifecycle table covers ['Accepted', 'Cancelled', 'Completed', 'Delivered', 'Published', 'Refused', 'Running', 'TimedOut
AC-9   fails as expected  — RSK-9 is left undisposed in the threat model
        - AC-9 RSK-9's expiry is not a date
        - AC-9 the threat model does not record P06's disposition
AC-10  fails as expected  — cancellation arrivals fall below the four the case requires
        - AC-10 cancellation enumerates 3 arrivals, expected four
AC-11  fails as expected  — a delivery failure has no resulting state
        - AC-11 § 11 does not give each failure a resulting state
```

## The checker

```python
#!/usr/bin/env python3
"""P06 acceptance cases against ADR-0006, read structurally."""
import re, sys, pathlib

A = pathlib.Path(sys.argv[1]).read_text()
ROOT = pathlib.Path(sys.argv[2] if len(sys.argv) > 2 else ".")
fails, notes = [], []
flat = " ".join(A.split())

def section(pat):
    m = re.search(pat + r".*?(?=\n### |\n## |\Z)", A, re.S)
    return m.group(0) if m else ""

def rows(pat, ncols=None):
    """Table rows of the section, as cell lists, header and rule dropped."""
    out = []
    for line in section(pat).splitlines():
        if not line.startswith("|"):
            continue
        cells = [c.strip() for c in line.strip().strip("|").split("|")]
        if not cells or set("".join(cells)) <= set("-: "):
            continue
        if cells[0].lower() in ("state", "from", "invariant", "risk", "setting",
                                "ledger says", "recovery finds", "before this happens",
                                "when cancellation arrives", "path", ""):
            continue
        if ncols and len(cells) != ncols:
            continue
        out.append(cells)
    return out

def strip(c):
    return c.strip("`* ")

# ---- the two machines, parsed --------------------------------------------
def machine(state_pat, trans_pat):
    st = {strip(r[0]): r for r in rows(state_pat) if len(r) >= 3}
    tr = [(strip(r[0]), strip(r[1])) for r in rows(trans_pat) if len(r) == 3]
    return st, tr

srv_states = {strip(r[0]): r for r in rows(r"### 1\. The server state machine") if len(r) == 3}
srv_trans  = [(strip(r[0]), strip(r[1])) for r in rows(r"### 1\. The server state machine") if len(r) == 3
              and strip(r[0]) in {strip(x[0]) for x in rows(r"### 1\. The server state machine") if len(x) == 3}]
# the section holds two 3-column tables; separate them by whether col3 is Yes/No
_all3 = [r for r in rows(r"### 1\. The server state machine") if len(r) == 3]
srv_states = {strip(r[0]): r for r in _all3 if strip(r[2]) in ("Yes", "No")}
srv_trans  = [(strip(r[0]), strip(r[1])) for r in _all3 if strip(r[2]) not in ("Yes", "No")]

_all3a = [r for r in rows(r"### 2\. The agent state machine") if len(r) == 3]
agt_states = {strip(r[0]): r for r in _all3a if strip(r[2]) in ("Yes", "No")}
agt_trans  = [(strip(r[0]), strip(r[1])) for r in _all3a if strip(r[2]) not in ("Yes", "No")]

notes.append(f"server: {len(srv_states)} states, {len(srv_trans)} transitions")
notes.append(f"agent: {len(agt_states)} states, {len(agt_trans)} transitions")

# --- AC-1 every ARCH-JOB clause is expressed, not merely cited -------------
INVDOC = (ROOT / "docs/project/ARCHITECTURE-INVARIANTS.md").read_text()
cov = section(r"## Invariant coverage")
for n in range(1, 6):
    inv = f"ARCH-JOB-00{n}"
    if f"### {inv} —" not in INVDOC:
        fails.append(f"AC-1 checker assumption stale: {inv} is not an invariant")
        continue
    row = [r for r in rows(r"## Invariant coverage") if strip(r[0]) == inv]
    if not row:
        fails.append(f"AC-1 {inv} has no coverage row")
    elif "satisfied" not in row[0][1].lower():
        fails.append(f"AC-1 {inv} is not claimed as satisfied: {row[0][1][:60]}")
    elif not re.search(r"§§? \d", row[0][1]):
        fails.append(f"AC-1 {inv}'s coverage cites no section")

# --- AC-2 both machines are coherent: reachable, and no dead end -----------
for name, states, trans in (("server", srv_states, srv_trans), ("agent", agt_states, agt_trans)):
    if len(states) < 4 or len(trans) < 4:
        fails.append(f"AC-2 {name} machine read as {len(states)} states/{len(trans)} "
                     f"transitions — check is vacuous")
        continue
    for a, b in trans:
        if a not in states:
            fails.append(f"AC-2 {name} transition from {a!r}, which is not a declared state")
        if b not in states:
            fails.append(f"AC-2 {name} transition to {b!r}, which is not a declared state")
    terminal = {s for s, r in states.items() if strip(r[2]) == "Yes"}
    # every non-terminal state has an outgoing transition
    for s in states:
        if s not in terminal and not any(a == s for a, _ in trans):
            fails.append(f"AC-2 {name} state {s!r} is non-terminal and has no outgoing transition")
    # every terminal state has no outgoing transition, unless it declares one
    # every state is reachable from the entry state (the first declared)
    entry = next(iter(states))
    seen, frontier = {entry}, [entry]
    while frontier:
        cur = frontier.pop()
        for a, b in trans:
            if a == cur and b not in seen:
                seen.add(b); frontier.append(b)
    for s in states:
        if s not in seen:
            fails.append(f"AC-2 {name} state {s!r} is unreachable from {entry!r}")

# --- AC-3 terminal states marked, and the set stated as complete -----------
for name, states in (("server", srv_states), ("agent", agt_states)):
    vals = {strip(r[2]) for r in states.values()}
    if not vals <= {"Yes", "No"}:
        fails.append(f"AC-3 {name} has a terminal column that is not Yes/No: {vals}")
    if not any(strip(r[2]) == "Yes" for r in states.values()):
        fails.append(f"AC-3 {name} declares no terminal state")
if "Every row is a permitted transition; nothing else is" not in flat:
    fails.append("AC-3 the transition set is not stated as complete")

# --- AC-4 UNKNOWN is its own state, never rendered as another, both kept ---
u = section(r"### 12\. `UNKNOWN`")
if not u:
    fails.append("AC-4 no UNKNOWN section")
else:
    if "Unknown" not in srv_states:
        fails.append("AC-4 Unknown is not a server state")
    if not re.search(r"[Nn]ot success, not failure", " ".join(u.split())):
        fails.append("AC-4 the ADR does not state what UNKNOWN may not be rendered as")
    if not re.search(r"[Bb]oth observations are retained", " ".join(u.split())):
        fails.append("AC-4 late-result reconciliation does not retain both observations")
    if len([r for r in rows(r"### 12\. `UNKNOWN`") if len(r) == 2]) < 3:
        fails.append("AC-4 the three UNKNOWN paths are not enumerated")

# --- AC-5 redelivery finite, and no redelivery may execute twice -----------
r8 = " ".join(section(r"### 8\. Redelivery").split())
if "MaxDeliver" not in r8 or "BackOff" not in r8:
    fails.append("AC-5 redelivery does not express MaxDeliver and BackOff")
if not re.search(r"may not cause.{0,200}second execution", r8):
    fails.append("AC-5 the ADR does not forbid a redelivery causing a second execution")
if "ADR-0002" not in r8:
    fails.append("AC-5 redelivery is not expressed against ADR-0002 § 7")

# --- AC-6 the ledger outranks the dedup window, and the window's limit ------
r6, r7 = " ".join(section(r"### 6\. Publication deduplication").split()), \
         " ".join(section(r"### 7\. Application-ledger deduplication").split())
if "two minutes" not in r6:
    fails.append("AC-6 the deduplication window's bound is not stated")
if not re.search(r"does not guarantee", r6):
    fails.append("AC-6 the window's limit is not stated")
if not re.search(r"authority.{0,120}job identifier|job identifier.{0,120}authority", r7):
    fails.append("AC-6 the ledger is not stated as the authority keyed on the job identifier")
if "exactly-once" not in r6:
    fails.append("AC-6 the window is not distinguished from an exactly-once claim")

# --- AC-7 the ARCH-JOB-005 / ARCH-NATS-010 interaction is answered ---------
r11 = section(r"### 11\. Delivery failure and expiry")
f11 = " ".join(r11.split())
if not r11:
    fails.append("AC-7 no delivery-failure section")
else:
    cells = [c for row in rows(r"### 11\. Delivery failure and expiry") for c in row]
    if "Undelivered" not in srv_states:
        fails.append("AC-7 Undelivered is not a server state")
    if not re.search(r"MAX_DELIVERIES", f11):
        fails.append("AC-7 the exhaustion advisory is not named")
    if not re.search(r"max.age|max-age", f11, re.I):
        fails.append("AC-7 expiry is not distinguished by its broker mechanism")
    if not re.search(r"delivery count increments only when a consumer pulls|"
                     r"increments on delivery to a consumer", f11):
        fails.append("AC-7 the broker property the distinction rests on is not stated")

# --- AC-8 every lifecycle-table state has every attribute ------------------
lt = [r for r in rows(r"### 13\. The lifecycle table") if len(r) == 5]
if len(lt) < 5:
    fails.append(f"AC-8 the lifecycle table has {len(lt)} rows — check is vacuous")
for r in lt:
    for i, attr in enumerate(["durability point", "causer", "operator-visible form", "terminal flag"], 1):
        if not r[i].strip():
            fails.append(f"AC-8 {strip(r[0])} has no {attr}")
declared = {strip(r[0]) for r in lt}
if declared != set(srv_states):
    fails.append(f"AC-8 the lifecycle table covers {sorted(declared)}; "
                 f"§ 1 declares {sorted(srv_states)}")
notes.append(f"AC-8 lifecycle table covers all {len(lt)} server states")

# --- AC-9 RSK-9 is resolved or renewed, with its dependents updated --------
TM = (ROOT / "docs/project/THREAT-MODEL.md").read_text()
rr = [r for r in rows(r"## Residual risks") if len(r) == 2 and strip(r[0]) == "RSK-9"]
if not rr:
    fails.append("AC-9 the ADR does not dispose of RSK-9")
elif not re.search(r"\*\*(Renewed|Resolved)", rr[0][1]):
    fails.append("AC-9 RSK-9's disposition is neither renewed nor resolved")
row = re.search(r"^\| `RSK-9` \|(.*)$", TM, re.M)
if not row:
    fails.append("AC-9 RSK-9 has no row in THREAT-MODEL.md")
else:
    cells = [c.strip() for c in row.group(1).split("|")]
    if len(cells) < 2 or not re.search(r"\b20\d\d-\d\d-\d\d\b", cells[-2]):
        fails.append("AC-9 RSK-9's expiry is not a date")
    if "P06" not in cells[-2] and "Renewed at P06" not in row.group(1):
        fails.append("AC-9 the threat model does not record P06's disposition")

# --- AC-10 cancellation covers four arrivals and the crash boundary --------
c9 = section(r"### 9\. Cancellation races")
f9 = " ".join(c9.split())
arr = [r for r in rows(r"### 9\. Cancellation races") if len(r) == 2]
if len(arr) < 4:
    fails.append(f"AC-10 cancellation enumerates {len(arr)} arrivals, expected four")
if "crash boundary" not in f9.lower():
    fails.append("AC-10 the crash boundary is not addressed")
if "THR-25" not in f9:
    fails.append("AC-10 the crash boundary does not cite THR-25")

# --- AC-11 delivery failure and expiry each defined, with a party told -----
for term in ("Undelivered", "Unknown"):
    if term not in f11:
        fails.append(f"AC-11 § 11 does not name {term}")
if not re.search(r"Server state", f11):
    fails.append("AC-11 § 11 does not give each failure a resulting state")

for n in notes: print("  ·", n)
print()
if fails:
    print("FAIL")
    for f in fails: print(" -", f)
    sys.exit(1)
print("== 0 failures ==")
```

## The review round that changed the design

`AC-10` in this file originally demonstrated on a four-row arrivals table. It
now has five, because review found the first row false.

**The finding.** *Cancellation before delivery* claimed the job reaches
`Cancelled` **with no execution**, on the reasoning that the agent's ledger would
record the cancellation and refuse the command. It would not: `KS_CMD` carries
commands and cancellations on **one** per-agent consumer with
`FilterSubject: ks.job.<agent>.>`, so for an offline agent the command sits
earlier in the sequence, is pulled first, meets an empty ledger, and starts.

**What checking the premise added.** `MaxAckPending=1` and `ARCH-JOB-005`
together mean a cancellation in `KS_CMD` cannot be delivered while the command it
cancels is un-acknowledged — so the durable path cannot serve
cancellation-during-execution either. The live core-NATS subscription
`ADR-0002` § 4 grants and `ADR-0004` `POS-4` tests is what does, and **no accepted
ADR said so**. § 9 now states both paths and which case each serves, which is
P06's own subject rather than a fix to someone else's ADR.

**No case here could have caught it.** Every case reads one document for
structure. This defect is a claim about the interaction of two other ADRs'
decisions, and it took a reader who held all three at once.

## What these cases cannot detect

| Case | Cannot detect | Found instead by |
|---|---|---|
| `AC-1` | Whether an expression **satisfies** the clause as opposed to restating it | Review; C06 and C08 |
| `AC-2` | Whether the transitions are the **right** ones. A coherent machine can model the wrong system | Review; C08's and C09's slices |
| `AC-3` | Whether a state **should** be terminal | Review |
| `AC-4` | Whether `UNKNOWN` is reported at the right **moments** | Review; C11's operator UX; C14 |
| `AC-5` | Whether `MaxDeliver` and `BackOff` have the right **values** | C14's fault matrix; C15's soak |
| `AC-6` | Whether the ledger's authority survives **its own** corruption — `RSK-9`'s subject | Nothing in `v0.6.0` |
| `AC-7` | Whether the answer is **operationally acceptable** | C14; C16's pilot |
| `AC-8` | Whether a durability point is achievable at acceptable cost | C02's ledgers; C15 |
| `AC-9` | Whether the renewal is **justified** | The maintainer, who owns the risk by name |
| `AC-10` | Whether the race analysis is **complete** | C09's slice; C14's adversarial suite |
| `AC-11` | Whether expiry's duration is right | C15; C16 |

**And one thing no case here reaches at all**: whether two sections of the ADR
agree with each other. The contradiction recorded above was found by reading,
and a document that contradicts itself passes every case in this file.
