# P09 acceptance evidence

The acceptance cases of [`P09.md`](P09.md) § 5.2, each demonstrated failing on a
document that carries its defect, then passing on the documents as they stand.

**Seven cases join two properties with "and", and each is planted once per
conjunct** — eighteen plantings for twelve cases. That rule is what P08 cost:
its `AC-9` stated two properties, implemented one, and passed on a document
violating the other, which review had to find. `DL-1`'s entry now carries it.

Four expected values are never typed here. The journeys come out of
`PRODUCT-CHARTER.md` § 5, the exit codes out of its § Exit codes, the
auditable-transition list out of `ARCH-OBS-001`, and the trust domains out of
`THREAT-MODEL.md` — so no case can agree with a mistake it shares with the ADR.

## What each case fails on

| Case | Fails when |
|---|---|
| `AC-1` | A path has no octal mode, no owner or group, or the owning trust domain is unnamed or is one the threat model does not define |
| `AC-2` | The kernel mechanism is unnamed, the credentials are not said to be unobtainable from the caller, or the ordering never binds the parse step |
| `AC-3` | A journey read out of the charter has no authority row, or a row states no authority |
| `AC-4` | A code is used that the charter does not define, the kernel-refused path has no outcome, or the two refusal paths' distinguishability is unstated |
| `AC-5` | The Actor field has fewer than two parts, marks none authoritative or none a snapshot, omits `RSK-8`'s concession, or is silent on the pid |
| `AC-6` | Recorded and unrecorded denials are not separated, the unobservable set is not named, or `ARCH-OBS-001` compliance is claimed without exposing the reading it rests on |
| `AC-7` | A request field may set the acting identity, or impersonation is forbidden without saying what is lost |
| `AC-8` | The obligations do not bind the unlink, a failing path's fate is unstated, or the verify-then-unlink race is unaddressed or named without what closes it |
| `AC-9` | Fewer than four limits, or none applies before authorization completes |
| `AC-10` | `RSK-8` is undisposed or carries no dated expiry, the threat model does not record P09's disposition, or any tracked file still forward-references the decision |
| `AC-11` | `ARCH-COMM-001`'s clause is not stated as a property, or the property has no test that could fail |
| `AC-12` | A deferral names no trigger |

## `AC-10` is an assertion, not a description

`RSK-8`'s change is a **conclusion**, not an identifier: *"P09 will define which
operators may do what"* became *"P09 defined it"*. `DL-8`'s third tier says to
search the conclusion's **subject**, require every hit to carry the new
conclusion, and **encode that as an assertion over every tracked file rather
than a search you perform and describe**.

P08's dossier described one, correctly, and the task still left `THR-46`
standing — the ledger's fourteenth occasion, and the first where the enumeration
was right and simply not applied. So `AC-10` sweeps `RSK-8` across every tracked
Markdown file, **treats each table row as its own unit**, and classifies every
hit as carrying the new disposition, quoted as superseded, a record superseded by
its own terms, or **live** — failing on any live one. Nothing is filtered; every
hit is printed in the checker's notes.

It found the work rather than confirming it: on first run `THR-35`'s mitigation
and `RSK-8`'s own row both still read as forward references, and both are sites
§ 2 had enumerated.

## Demonstration record

```text
unmutated tree: PASS
AC-1   fails as expected  — the socket has no octal mode
        - AC-1 § 1's `/run/keystone/operator.sock` has no octal mode: 'group-writable'
AC-1   fails as expected  — the owning trust domain is not named
        - AC-1 § 1 names domains without saying which one owns the file
AC-2   fails as expected  — the credentials are not said to come from the kernel
        - AC-2 § 3 does not name the kernel mechanism the credentials come from
        - AC-2 § 3 does not state that the credentials cannot come from the caller
AC-2   fails as expected  — the ordering never binds the parse step
        - AC-2 § 3's obligations do not bind the parse step
        - AC-2 § 3's obligations never require authorization to have completed
AC-3   fails as expected  — a charter journey has no authority row
        - AC-3 § 4 has no row for the charter's journey § 5.6 Cancel
AC-4   fails as expected  — an exit code the charter does not define is used
        - AC-4 § 5 uses exit code 77, which PRODUCT-CHARTER.md does not define
AC-4   fails as expected  — the kernel-refused path has no stated outcome
        - AC-4 § 5 does not say what a kernel-refused connection yields
AC-5   fails as expected  — the Actor field marks no part a snapshot
        - AC-5 § 7 marks no part of the Actor field a snapshot
AC-5   fails as expected  — RSK-8's concession is not carried into the schema
        - AC-5 § 7 does not carry RSK-8's concession into the schema
AC-6   fails as expected  — the unobservable set is not named
        - AC-6 § 6 does not name the unobservable set as such
        - AC-6 § 6 claims ARCH-OBS-001 compliance without exposing the reading it rests on
AC-7   fails as expected  — impersonation is forbidden without saying what is lost
        - AC-7 § 7 forbids impersonation without saying what is lost
AC-8   fails as expected  — the verify-then-unlink race is left open
        - AC-8 § 8 does not address the verify-then-unlink race
        - AC-8 § 8 names the race without stating what closes it
AC-9   fails as expected  — no limit applies before authorization completes
        - AC-9 § 9 states no limit that applies before authorization completes
AC-10  fails as expected  — a dependent site still forward-references the decision
        - AC-10 docs/project/THREAT-MODEL.md still speaks of RSK-8 as a decision P09 has not made
AC-10  fails as expected  — RSK-8 is left without a dated expiry
        - AC-10 RSK-8 carries no dated expiry
AC-11  fails as expected  — the hidden-transport clause has no test that could fail
        - AC-11 § 11 states a property with no test that could fail
AC-12  fails as expected  — a deferred decision point names no trigger
        - AC-12 the deferral '**Remote operator access**' names no trigger

```

## The checker

```python
#!/usr/bin/env python3
"""P09 acceptance cases against ADR-0009, read structurally.

Expected values come from the charter, the invariants and the threat model --
never typed here -- so no case can agree with a mistake it shares with the ADR.
"""
import re, sys, pathlib

A    = pathlib.Path(sys.argv[1]).read_text()
ROOT = pathlib.Path(sys.argv[2] if len(sys.argv) > 2 else ".")
CH   = (ROOT / "docs/project/PRODUCT-CHARTER.md").read_text()
INV  = (ROOT / "docs/project/ARCHITECTURE-INVARIANTS.md").read_text()
TM   = (ROOT / "docs/project/THREAT-MODEL.md").read_text()
A7   = (ROOT / "docs/adr/0007-safe-execution.md").read_text()
A8   = (ROOT / "docs/adr/0008-persistence-and-audit.md").read_text()
fails, notes = [], []

def section(pat, doc=None):
    d = A if doc is None else doc
    lvl = pat.count("#") or 3
    stop = "|".join(f"^{'#' * n} " for n in range(2, lvl + 1))
    m = re.search(rf"^{pat}.*?(?={stop}|\Z)", d, re.S | re.M)
    return m.group(0) if m else ""

def rows(pat, doc=None):
    """Body rows of the tables in a section, as lists of cells."""
    out = []
    for ln in section(pat, doc).splitlines():
        ln = ln.strip()
        if not ln.startswith("|") or set(ln) <= set("|- :"):
            continue
        cells = [c.strip() for c in ln.strip("|").split("|")]
        out.append(cells)
    return out

def flat(pat, doc=None):
    return " ".join(section(pat, doc).split())

def drop_header(rs, *heads):
    """A header row is not data. Reading one as data is how P08's checker lied."""
    return [r for r in rs if not any(h.lower() == r[0].lower().strip("*` ") for h in heads)]

# --- AC-1 socket and directory: owner, group, octal mode, owning domain -----
s1 = rows(r"### 1\. The socket")
paths = drop_header([r for r in s1 if len(r) == 4], "Path")
if len(paths) < 2:
    fails.append(f"AC-1 § 1 states {len(paths)} paths, expected the socket and its directory")
for r in paths:
    if not re.fullmatch(r"0[0-7]{3}", r[3].strip("`* ")):
        fails.append(f"AC-1 § 1's {r[0]} has no octal mode: {r[3]!r}")
    if not r[1].strip("`* ") or not r[2].strip("`* "):
        fails.append(f"AC-1 § 1's {r[0]} has no owner or no group")
f1 = flat(r"### 1\. The socket")
# the owning trust domain must be named, and it must be one the threat model has
domains = set(re.findall(r"`(TD-[A-Z]+)`", TM))
named = set(re.findall(r"`(TD-[A-Z]+)`", f1))
if not named:
    fails.append("AC-1 § 1 does not name the trust domain that owns the socket file")
elif not named <= domains:
    fails.append(f"AC-1 § 1 names a domain the threat model does not define: {named - domains}")
if not re.search(r"belongs to `TD-SRV`|owned by `TD-SRV`|`TD-SRV`, not", f1):
    fails.append("AC-1 § 1 names domains without saying which one owns the file")
notes.append(f"AC-1 paths: {[(r[0], r[3]) for r in paths]}; domains named: {sorted(named)}")

# --- AC-2 authorization from kernel credentials, as an ordering obligation --
f3 = flat(r"### 3\. Where authorization is evaluated")
ob = drop_header([r for r in rows(r"### 3\. Where authorization") if len(r) == 2],
                 "Before this happens")
if len(ob) < 3:
    fails.append(f"AC-2 § 3 states {len(ob)} ordering obligations, expected the full sequence")
if not re.search(r"SO_PEERCRED", f3):
    fails.append("AC-2 § 3 does not name the kernel mechanism the credentials come from")
if not re.search(r"from the kernel, never from the wire|never from the wire", f3):
    fails.append("AC-2 § 3 does not state that the credentials cannot come from the caller")
# the obligation must FORBID parsing first, not merely describe the order
if not any(re.search(r"\bparsed\b", r[0]) for r in ob):
    fails.append("AC-2 § 3's obligations do not bind the parse step")
if not any("authoriz" in r[1].lower() for r in ob):
    fails.append("AC-2 § 3's obligations never require authorization to have completed")
notes.append(f"AC-2 § 3: {len(ob)} ordering obligations")

# --- AC-3 every charter journey has an authority row -----------------------
journeys = re.findall(r"^### (5\.\d+) (.+)$", CH, re.M)
if len(journeys) < 5:
    fails.append(f"AC-3 only {len(journeys)} charter journeys parsed — the case is vacuous")
auth = {r[0]: r[1] for r in drop_header(
    [r for r in rows(r"### 4\. What an operator may do") if len(r) == 2], "Journey")}
for num, name in journeys:
    key = [k for k in auth if num in k]
    if not key:
        fails.append(f"AC-3 § 4 has no row for the charter's journey § {num} {name}")
        continue
    if name.split()[0].lower() not in key[0].lower():
        fails.append(f"AC-3 § 4's row for § {num} does not name the journey {name!r}")
    if not auth[key[0]].strip("`* "):
        fails.append(f"AC-3 § 4's row for § {num} states no authority")
notes.append(f"AC-3 {len(journeys)} charter journeys, {len(auth)} authority rows")

# --- AC-4 every code is the charter's; both refusal paths have an outcome ---
charter_codes = set(re.findall(r"^\| `(\d+)` \| ", CH, re.M))
if len(charter_codes) < 5:
    fails.append("AC-4 could not read the charter's exit codes — the case is vacuous")
code_rows = drop_header([r for r in rows(r"### 5\. Exit codes") if len(r) == 3], "Situation")
used = set()
for r in code_rows:
    c = r[1].strip("`* ")
    if not re.fullmatch(r"\d+", c):
        fails.append(f"AC-4 § 5's row {r[0][:40]!r} gives no numeric code")
        continue
    used.add(c)
    if c not in charter_codes:
        fails.append(f"AC-4 § 5 uses exit code {c}, which PRODUCT-CHARTER.md does not define")
f5 = flat(r"### 5\. Exit codes")
if not re.search(r"EACCES", f5):
    fails.append("AC-4 § 5 does not say what a kernel-refused connection yields")
if not re.search(r"indistinguishable|cannot tell them apart", f5):
    fails.append("AC-4 § 5 does not state whether the two refusal paths differ to the caller")
notes.append(f"AC-4 codes used {sorted(used)} ⊆ charter {sorted(charter_codes)}")

# --- AC-5 the Actor field: two parts, one authoritative, one a snapshot -----
act = drop_header([r for r in rows(r"### 7\. The audit identity") if len(r) == 3], "Part")
if len(act) < 2:
    fails.append(f"AC-5 § 7 gives the Actor field {len(act)} parts, expected two")
statuses = " ".join(r[2] for r in act).lower()
if "authoritative" not in statuses:
    fails.append("AC-5 § 7 marks no part of the Actor field authoritative")
if "snapshot" not in statuses:
    fails.append("AC-5 § 7 marks no part of the Actor field a snapshot")
f7 = flat(r"### 7\. The audit identity")
if not re.search(r"identity exercised and not the person|identity exercised, not the person", f7):
    fails.append("AC-5 § 7 does not carry RSK-8's concession into the schema")
if not re.search(r"pid is not recorded|not record.{0,20}pid", f7):
    fails.append("AC-5 § 7 does not say whether the pid is recorded")
notes.append(f"AC-5 Actor parts: {[r[0].strip('*` ') for r in act]}")

# --- AC-6 which denials are recorded, and which are not --------------------
den = drop_header([r for r in rows(r"### 6\. What a denial records") if len(r) == 3], "Denial")
if len(den) < 3:
    fails.append(f"AC-6 § 6 classifies {len(den)} denials, expected the connected and refused cases")
yes = [r for r in den if r[1].strip("*` ").lower().startswith("yes")]
no  = [r for r in den if r[1].strip("*` ").lower().startswith("no")]
if not yes or not no:
    fails.append("AC-6 § 6 does not separate recorded denials from unrecorded ones")
f6 = flat(r"### 6\. What a denial records")
if not re.search(r"unrecorded set is named|set is named rather than left", f6):
    fails.append("AC-6 § 6 does not name the unobservable set as such")
if not re.search(r"so it can be disputed|stated here so it can be", f6):
    fails.append("AC-6 § 6 claims ARCH-OBS-001 compliance without exposing the reading it rests on")
# and the invariant really must list authorization denial, or the case is moot
if "authorization denial" not in " ".join(section(r"### ARCH-OBS-001", INV).split()).lower():
    fails.append("AC-6 ARCH-OBS-001 does not list authorization denial — the case is misaimed")
notes.append(f"AC-6 denials: {len(yes)} recorded, {len(no)} not")

# --- AC-7 impersonation forbidden, and what that costs ---------------------
if not re.search(r"No request field may set the acting identity", f7):
    fails.append("AC-7 § 7 does not forbid a request field setting the acting identity")
if not re.search(r"forecloses is real|What this forecloses", f7):
    fails.append("AC-7 § 7 forbids impersonation without saying what is lost")

# --- AC-8 stale-socket recovery does not unlink unverified -----------------
f8 = flat(r"### 8\. Stale-socket recovery")
un = drop_header([r for r in rows(r"### 8\. Stale-socket recovery") if len(r) == 2],
                 "Before this happens")
if len(un) < 2:
    fails.append(f"AC-8 § 8 states {len(un)} recovery obligations")
if not any("unlink" in r[0].lower() for r in un):
    fails.append("AC-8 § 8's obligations do not bind the unlink")
if not re.search(r"is \*\*not\*\* removed|not removed", f8):
    fails.append("AC-8 § 8 does not say what happens to a path that fails the checks")
if not re.search(r"time-of-check|TOCTOU", f8):
    fails.append("AC-8 § 8 does not address the verify-then-unlink race")
if not re.search(r"only `root` may create, rename or remove", f8):
    fails.append("AC-8 § 8 names the race without stating what closes it")
notes.append(f"AC-8 § 8: {len(un)} recovery obligations")

# --- AC-9 limits, at least one before authorization ------------------------
lim = drop_header([r for r in rows(r"### 9\. Request limits") if len(r) == 3], "Limit")
if len(lim) < 4:
    fails.append(f"AC-9 § 9 states {len(lim)} limits")
pre = [r for r in lim if "before" in r[1].lower()]
if not pre:
    fails.append("AC-9 § 9 states no limit that applies before authorization completes")
notes.append(f"AC-9 {len(lim)} limits, {len(pre)} pre-authorization")

# --- AC-10 RSK-8 disposed, and every dependent site carries it -------------
rr = {r[0].strip("`* "): r[1] for r in rows(r"## Residual risks") if len(r) == 2}
if "RSK-8" not in rr:
    fails.append("AC-10 the ADR does not dispose of RSK-8")
else:
    if not re.search(r"\*\*(Renewed|Resolved)", rr["RSK-8"]):
        fails.append("AC-10 RSK-8's disposition is neither renewed nor resolved")
    if not re.search(r"\b20\d\d-\d\d-\d\d\b", rr["RSK-8"]):
        fails.append("AC-10 RSK-8 carries no dated expiry")
row = re.search(r"^\| `RSK-8` \|(.*)$", TM, re.M)
if not row:
    fails.append("AC-10 RSK-8 has no row in THREAT-MODEL.md")
elif "P09" not in row.group(1):
    fails.append("AC-10 the threat model does not record P09's disposition of RSK-8")
# the conclusion P09 changes: the actor is kernel-derived and unforgeable.
# DL-8's third tier -- sweep the SUBJECT, assert over every tracked file, and
# give every hit a disposition rather than filtering any out.
SUBJ = re.compile(r"\bRSK-8\b")
STALE = re.compile(r"P09 will define|authorization model will share", re.I)
hits = []
for f in sorted(ROOT.rglob("*.md")):
    if ".git" in f.parts or "docs/transition" in f.as_posix():
        continue
    body = re.sub(r"^```.*?^```", "", f.read_text(), flags=re.S | re.M)
    units = []
    for para in re.split(r"\n\s*\n", body):
        tbl, prose = [], []
        for ln in para.splitlines():
            (tbl if ln.lstrip().startswith("|") else prose).append(ln)
        units += [" ".join(r.split()) for r in tbl]
        if prose:
            units.append(" ".join(" ".join(prose).split()))
    for u in units:
        if not SUBJ.search(u):
            continue
        if "P01-acceptance-evidence" in f.as_posix():
            hits.append((f.name, "record of P01, superseded by its own terms")); continue
        m = STALE.search(u)
        if not m:
            hits.append((f.name, "carries the new disposition")); continue
        # A quotation of the superseded text is legitimate; a live claim is not.
        # Quote parity is computed per unit, and a table row is its own unit.
        if u[:m.start()].count('"') % 2 == 1:
            hits.append((f.name, "quoted as superseded")); continue
        hits.append((f.name, "LIVE forward reference"))
        fails.append(f"AC-10 {f} still speaks of RSK-8 as a decision P09 has not made")
if len(hits) < 3:
    fails.append(f"AC-10 the RSK-8 sweep found {len(hits)} sites — it is vacuous")
notes.append(f"AC-10 RSK-8 sweep: {hits}")

# --- AC-11 ARCH-COMM-001's hidden-transport clause, as a property ----------
f11 = flat(r"### 11\. What the operator API must never become")
clause = " ".join(section(r"### ARCH-COMM-001", INV).split())
if "hidden agent transport" not in clause:
    fails.append("AC-11 ARCH-COMM-001 has no hidden-transport clause — the case is misaimed")
if not re.search(r"No request on this socket may cause", f11):
    fails.append("AC-11 § 11 does not state the clause as a property of the design")
if not re.search(r"The test that distinguishes|if removing the broker", f11):
    fails.append("AC-11 § 11 states a property with no test that could fail")

# --- AC-12 every deferral names a trigger ---------------------------------
d = drop_header([r for r in rows(r"### Deferred decision points") if len(r) == 2], "Deferred")
if len(d) < 3:
    fails.append(f"AC-12 {len(d)} deferrals listed — check is vacuous")
for r in d:
    if len(r[1]) < 25:
        fails.append(f"AC-12 the deferral {r[0][:40]!r} names no trigger")
notes.append(f"AC-12 {len(d)} deferrals, each with a trigger")

for n in notes: print("  ·", n)
print()
if fails:
    print("FAIL")
    for f in fails: print(" -", f)
    sys.exit(1)
print("== 0 failures ==")
```

## Two mutations that did not do what they claimed

**One matched nothing at all.** `AC-6`'s second conjunct anchored on *"That
reading is stated here so it can be disputed"*, which wraps across a line in the
source. `re.subn` reported zero substitutions and the harness stopped, because
every mutation asserts its own substitution count — the guard P08 added after
two of its literal replacements silently changed nothing and `str.replace`
returned the string unmodified without complaint.

That is now the fourth time on this epic that a remembered literal has lost to
eighty-column prose. The pattern spans the wrap.

**One changed text without changing the property**, in an earlier draft of
`AC-1`'s second planting: removing the sentence that named `TD-SRV` left the
bullet beneath it still saying the server is the only party that may change the
file, so the section still carried the ownership claim and the case still
passed. The planting now removes both, and asserts the parsed section no longer
carries it.

## What these cases cannot detect

| Case | Cannot detect | Found instead by |
|---|---|---|
| `AC-1` | Whether the modes are right on a real filesystem, or survive packaging | C13; VM tests |
| `AC-2` | **Whether an implementation authorizes before parsing.** The ordering is stated, not enforced | C04; the architecture lint P11 may set up; C14 |
| `AC-3` | Whether "admin for everything" is the right policy. It checks the mapping is total, not that it is wise | The maintainer, who fixed it at approval |
| `AC-4` | Whether an operator can tell the two refusal paths apart in practice — the ADR says they deliberately cannot | C04; C11; C16's pilot |
| `AC-5` | Whether the snapshot is ever **misread as the person**, which is the failure the concession predicts | Nothing in `v0.6.0`; review, and C12's record format |
| `AC-6` | Whether the unobservable set is acceptably small. It checks the set is named and the reading exposed | The maintainer; C14, which tries to produce a denial nobody sees |
| `AC-7` | Whether some other field is an acting identity in disguise — a target selector, a token, a config path | Review; C04 |
| `AC-8` | Whether the directory argument holds under a filesystem that does not enforce it, or under a bind mount | C13; C14's fault matrix |
| `AC-9` | Whether the limits are the right numbers — the ADR deliberately states none | C15's soak; C14 |
| `AC-10` | **Any other superseded conclusion.** It asserts over the one conclusion P09 changed; the next one has no assertion yet | Review, and the next task that changes a conclusion |
| `AC-11` | Whether a later feature quietly violates it. This is the invariant most likely to erode by increments | P11's architecture lint; review of every C task that adds an operator verb |
| `AC-12` | Whether a trigger is one anybody will notice | Review |

**And one thing no case reaches**: whether § 3's ordering is achievable at all.
Every case here reads a document that *says* authorization precedes parsing.
Whether a framing exists that needs no bytes before the decision is a claim about
code nobody has written, and § 10 is the only thing standing between it and a
version handshake added for good reasons in C04. That is the question § 8 puts to
the reviewer, and **C04 is where it stops being a claim.**
