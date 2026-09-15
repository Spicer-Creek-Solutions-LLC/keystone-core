# P10 acceptance evidence

The acceptance cases of [`P10.md`](P10.md) § 5.2, each demonstrated failing on a
document that carries its defect, then passing on the documents as they stand.

**Twenty-one plantings for twelve cases.** Seven cases join two or more
properties, and each conjunct is planted separately — the rule `DL-1` gained when
P08's `AC-9` checked one half of its own sentence and passed on a document
violating the other.

Six expected values are never typed here. `TESTING.md` supplies the topology
elements, the fault boundaries, the security-suite principals, the four canaries,
the gate schedule and the VM triggers; `ARCHITECTURE-INVARIANTS.md` supplies the
invariant set; `THREAT-MODEL.md` supplies the risk dispositions. No case can
agree with a mistake it shares with the ADR.

## What each case fails on

| Case | Fails when |
|---|---|
| `AC-1` | A topology element `TESTING.md` requires is uncovered, the no-host-port rule is dropped, or isolation is asserted without a probe |
| `AC-2` | A fault boundary read out of `TESTING.md` has no mechanism, the non-idempotent counter is dropped, a fault point becomes a build flag, leaves the shipped binary, or stops being inert by default |
| `AC-3` | A principal `TESTING.md` names is gone, a test-only authorization adapter is permitted, generated JWTs are not required, fixtures are committed, or `ADR-0009`'s two principals are absent or not placed on the VM side |
| `AC-4` | Fewer than three probe assertions, or the effect's absence on a non-target or the server is unasserted |
| `AC-5` | Fewer than three artifact classes, a canary `TESTING.md` requires is unseeded, the scan is claimed to prove redaction, or ownership is unstated |
| `AC-6` | A gate read out of `TESTING.md` has no entry point, or there is no local command |
| `AC-7` | A VM trigger read out of `TESTING.md` has no disposition, container-only emulation is not refused, or `ADR-0007`/`ADR-0009` are not placed on a side |
| `AC-8` | The register misses an invariant, still names a design owner that does not exist, does not point at this ADR, or the ADR contains a table of its own |
| `AC-9` | A risk is undisposed, carries no dated expiry, or the threat model does not record P10's disposition |
| `AC-10` | A risk still expires at P10, or the ADR stops saying what P10 cannot demonstrate |
| `AC-11` | Any tracked file still gates a risk on P10 |
| `AC-12` | A deferral names no trigger |

## `AC-11` is an assertion, and it needed a named exemption

The conclusion that changed is *"P10 is where these risks are demonstrated"* —
`ADR-0008` said so, `G22` corrected the claim, and this ADR moves the gates.
`DL-8`'s third tier says to sweep the conclusion's **subject** and encode it as
an assertion over every tracked file rather than a search that is performed and
described.

Every hit is classified — **corrected in place, quoted, a record superseded by
its own terms, describes the correction, an earlier ADR's own disposition, or
live** — and nothing is filtered.

**One exemption is doing real work and is worth naming.** `ADR-0003` records
`RSK-13` as *"Carried to P10"* and `ADR-0008` records three rows expiring
*"or P10"*. Both are out of this task's bounds, and both are now contradicted by
`THREAT-MODEL.md`. The sweep exempts an ADR's own residual-risk row **on the
stated ground that the threat model is authoritative for a risk's current gate**
— and the checker refuses that exemption unless `ADR-0010` § 13 actually says so,
so it cannot be claimed silently.

**It is also a cost, not a tidy resolution.** § "What this ADR does not decide"
raises it: a reader who finds a gate in an ADR has no signal that it is stale,
and whether an ADR's risk table should carry gates at all is worth deciding once.

## Demonstration record

```text
unmutated tree: PASS
AC-1   fails as expected  — the topology drops a required element
        - AC-1 § 2 does not cover the required topology element: operator client
AC-1   fails as expected  — isolation is asserted without a probe
        - AC-1 § 2 asserts isolation without a probe
        - AC-1 § 2 does not say why a probe rather than a compose file
AC-2   fails as expected  — a fault boundary has no mechanism
        - AC-2 § 5's boundary 'While the process runs' names no mechanism
AC-2   fails as expected  — a fault point becomes a build flag
        - AC-2 § 5 does not state that a fault point is configuration rather than a build flag
AC-2   fails as expected  — the fault point leaves the shipped binary
        - AC-2 § 5 does not require the fault point to be in the shipped binary
AC-2   fails as expected  — the fault configuration is no longer inert by default
        - AC-2 § 5 does not require the fault configuration to be inert by default
AC-3   fails as expected  — a test-only authorization adapter is permitted
        - AC-3 does not forbid a test-only authorization adapter
AC-3   fails as expected  — the revoked-after-login principal is dropped
        - AC-3 § 7 does not add the principal: revoked after login
AC-4   fails as expected  — the probe stops asserting the effect's absence elsewhere
        - AC-4 § 6 does not assert the effect's absence on a non-target
        - AC-4 § 6 does not say the negative half is asserted
AC-5   fails as expected  — a canary TESTING.md requires is not seeded
        - AC-5 § 9 does not seed a output canary — TESTING.md requires it
AC-5   fails as expected  — the canary scan is claimed to prove redaction
        - AC-5 § 9 claims more for the canary scan than it proves
AC-6   fails as expected  — a TESTING.md gate has no entry point
        - AC-6 § 11 has no entry for TESTING.md's gate 'Nightly'
AC-7   fails as expected  — a VM trigger has no disposition
        - AC-7 neither § 1 nor § 8 dispositions TESTING.md's VM trigger 'package-manager transactions'
AC-7   fails as expected  — container-only emulation is no longer refused
        - AC-7 § 1 does not carry TESTING.md's refusal of container-only emulation
AC-8   fails as expected  — the ADR grows a second invariant map
        - AC-8 § 13 contains a table — the map must live only in the register
AC-9   fails as expected  — a risk is left without a dated expiry
        - AC-9 RSK-12 carries no dated expiry
AC-10  fails as expected  — a risk still expires at P10, which cannot demonstrate it
        - AC-10 RSK-4 still expires at P10, which cannot demonstrate it
AC-10  fails as expected  — the ADR stops saying what P10 cannot demonstrate
        - AC-10 § Residual risks does not state what P10 cannot demonstrate
AC-11  fails as expected  — a tracked file still gates a risk on P10
        - AC-11 docs/project/THREAT-MODEL.md still gates something on P10: …ARCH-NATS-001`, `THR-49`) | 2027-09-13, or P10 |…
AC-8   fails as expected  — the register loses its pointer to this ADR
        - AC-8 the register still names a design owner that does not exist
AC-12  fails as expected  — a deferral names no trigger
        - AC-12 the deferral '**The real scale profile**' names no trigger

```

## The checker

```python
#!/usr/bin/env python3
"""P10 acceptance cases against ADR-0010, read structurally.

Expected values come from TESTING.md, ARCHITECTURE-INVARIANTS.md and
THREAT-MODEL.md -- never typed here -- so no case can agree with a mistake it
shares with the ADR.
"""
import re, sys, pathlib

A    = pathlib.Path(sys.argv[1]).read_text()
ROOT = pathlib.Path(sys.argv[2] if len(sys.argv) > 2 else ".")
T    = (ROOT / "docs/project/TESTING.md").read_text()
INV  = (ROOT / "docs/project/ARCHITECTURE-INVARIANTS.md").read_text()
TM   = (ROOT / "docs/project/THREAT-MODEL.md").read_text()
REG  = (ROOT / "docs/project/REQUIREMENTS-TRACEABILITY.md").read_text()
fails, notes = [], []

def section(pat, doc=None):
    d = A if doc is None else doc
    lvl = pat.count("#") or 3
    stop = "|".join(f"^{'#' * n} " for n in range(2, lvl + 1))
    m = re.search(rf"^{pat}.*?(?={stop}|\Z)", d, re.S | re.M)
    return m.group(0) if m else ""

def rows(pat, doc=None):
    out = []
    for ln in section(pat, doc).splitlines():
        ln = ln.strip()
        if not ln.startswith("|") or set(ln) <= set("|- :"):
            continue
        out.append([c.strip() for c in ln.strip("|").split("|")])
    return out

def drop_header(rs, *heads):
    return [r for r in rs if not any(h.lower() == r[0].lower().strip("*`  ") for h in heads)]

def flat(s):
    return " ".join(s.split())

def bullets(text):
    """List items, each flattened, continuations folded in.

    Joining whitespace with a regex swallowed the blank line before the first
    bullet and lost it, which the demonstration caught. Split on the marker
    instead of trying to normalise around it."""
    out, cur = [], None
    for ln in text.splitlines():
        if re.match(r"^\s*[-*] ", ln):
            if cur is not None:
                out.append(flat(cur))
            cur = re.sub(r"^\s*[-*] ", "", ln)
        elif cur is not None and ln.strip():
            cur += " " + ln.strip()
        elif cur is not None:
            out.append(flat(cur)); cur = None
    if cur is not None:
        out.append(flat(cur))
    return [b.rstrip(" ;.").removesuffix(" and") for b in out]

# --- AC-1 the topology covers every element TESTING.md requires ------------
req = section(r"## Required test topology", T)
# Bullets wrap at eighty columns, so a line-anchored pattern reads a two-line
# bullet as none. Join continuations before counting.
elements = bullets(req)
if len(elements) < 4:
    fails.append(f"AC-1 read {len(elements)} required topology elements — the case is vacuous")
f2 = flat(section(r"### 2\. The Docker topology"))
KEY = {"NATS": r"NATS", "Keystone server": r"Keystone server",
       "two agents": r"[Tt]wo, with different identities|at least two",
       "operator client": r"[Oo]perator client",
       "isolated networks": r"no route between|isolated"}
for name, pat in KEY.items():
    if not re.search(pat, f2):
        fails.append(f"AC-1 § 2 does not cover the required topology element: {name}")
if not re.search(r"host port", f2 + flat(section(r"### 2\. "))):
    fails.append("AC-1 § 2 does not carry TESTING.md's no-host-port requirement")
# the isolation must be a PROBE, not an assumption
if not re.search(r"probe", f2, re.I):
    fails.append("AC-1 § 2 asserts isolation without a probe")
if not re.search(r"fails when isolation is absent|passes vacuously", f2):
    fails.append("AC-1 § 2 does not say why a probe rather than a compose file")
notes.append(f"AC-1 TESTING.md requires {len(elements)} topology elements")

# --- AC-2 every fault boundary has a named mechanism -----------------------
fb = section(r"## Durability and fault injection", T)
bounds = bullets(fb)
if len(bounds) < 6:
    fails.append(f"AC-2 read {len(bounds)} fault boundaries from TESTING.md — vacuous")
mech = drop_header([r for r in rows(r"### 5\. Fault controls") if len(r) == 2], "Boundary")
if len(mech) < len(bounds):
    fails.append(f"AC-2 § 5 gives {len(mech)} mechanisms for {len(bounds)} boundaries")
for r in mech:
    if len(r[1]) < 20:
        fails.append(f"AC-2 § 5's boundary {r[0][:40]!r} names no mechanism")
f5 = flat(section(r"### 5\. Fault controls"))
if not re.search(r"non-idempotent", f5):
    fails.append("AC-2 § 5 does not carry TESTING.md's non-idempotent counter")
# Two separate properties, checked separately. As one alternation it was
# satisfied by either phrasing, so removing one left the case passing -- the
# same shape as P08's AC-4 and P09's AC-9.
if not re.search(r"configuration, not a build flag", f5):
    fails.append("AC-2 § 5 does not state that a fault point is configuration rather than a build flag")
if not re.search(r"not a test build", f5):
    fails.append("AC-2 § 5 does not require the fault point to be in the shipped binary")
if not re.search(r"inert by default", f5):
    fails.append("AC-2 § 5 does not require the fault configuration to be inert by default")
notes.append(f"AC-2 {len(bounds)} boundaries in TESTING.md, {len(mech)} mechanisms in § 5")

# --- AC-3 negative identities from generated JWTs --------------------------
f3, f7 = flat(section(r"### 3\. Production configuration")), flat(section(r"### 7\. Negative identities"))
sec = section(r"## NATS security suite", T)
princ = re.search(r"provisions explicit identities for (.+?):", flat(sec))
if not princ:
    fails.append("AC-3 could not read TESTING.md's principal list — vacuous")
else:
    for who in ("command publisher", "enrollment service", "result consumer",
                "presence consumer", "revoked bootstrap", "unrelated principal"):
        if who not in princ.group(1):
            fails.append(f"AC-3 TESTING.md's principal list no longer names {who!r}")
if not re.search(r"test-only authorization adapter", f3 + f7):
    fails.append("AC-3 does not forbid a test-only authorization adapter")
if not re.search(r"generated production JWTs|running C03's generator", f3 + f7):
    fails.append("AC-3 does not require generated production JWTs")
if not re.search(r"never committed|not committed", f3):
    fails.append("AC-3 § 3 permits committed credential fixtures")
# ADR-0009's two new principals must both appear, and be VM cases
for who, why in (("outside the operator admin group", "kernel-refused"),
                 ("revoked after login", "stale supplementary groups")):
    if not re.search(re.escape(who), f7):
        fails.append(f"AC-3 § 7 does not add the principal: {who}")
if "VM cases" not in f7 and "VM case" not in f7:
    fails.append("AC-3 § 7 does not place ADR-0009's principals on the VM side")

# --- AC-4 the external-effect probe asserts both halves --------------------
pr = drop_header([r for r in rows(r"### 6\. External-effect probes") if len(r) == 2], "Assertion")
if len(pr) < 3:
    fails.append(f"AC-4 § 6 gives {len(pr)} probe assertions")
joined = flat(section(r"### 6\. External-effect probes"))
if not re.search(r"non-target", joined):
    fails.append("AC-4 § 6 does not assert the effect's absence on a non-target")
if not re.search(r"server'?s? counter is unchanged|not happen on the server", joined):
    fails.append("AC-4 § 6 does not assert the effect's absence on the server")
if not re.search(r"asserted, not assumed", joined):
    fails.append("AC-4 § 6 does not say the negative half is asserted")

# --- AC-5 artifact classes, canaries, and who owns the rule ----------------
cl = drop_header([r for r in rows(r"### 9\. Artifacts") if len(r) == 3], "Class")
if len(cl) < 3:
    fails.append(f"AC-5 § 9 names {len(cl)} artifact classes, expected three")
f9 = flat(section(r"### 9\. Artifacts"))
for canary in ("private-key", "token", "command", "output"):
    if f"{canary} canary" not in f9:
        fails.append(f"AC-5 § 9 does not seed a {canary} canary — TESTING.md requires it")
if not re.search(r"does not prove redaction is complete|not prove redaction", f9):
    fails.append("AC-5 § 9 claims more for the canary scan than it proves")
if not re.search(r"P10'?s? by the plan|corrected `TESTING.md`|G22", f9):
    fails.append("AC-5 § 9 does not state which party owns the rule")
notes.append(f"AC-5 {len(cl)} artifact classes")

# --- AC-6 every TESTING.md gate has a harness entry point ------------------
gsec = section(r"## Gate schedule", T)
gates = [g.strip("` ") for g in re.findall(r"^### (.+)$", gsec, re.M)]
if len(gates) < 3:
    fails.append(f"AC-6 read {len(gates)} gates from TESTING.md — vacuous")
g11 = rows(r"### 11\. Entry points")
have = " ".join(" ".join(r) for r in g11).lower()
for g in gates:
    if g.strip("`").lower() not in have:
        fails.append(f"AC-6 § 11 has no entry for TESTING.md's gate {g!r}")
if not re.search(r"[Oo]ne local command", flat(section(r"### 11\. Entry points"))):
    fails.append("AC-6 § 11 does not give a local entry point")
notes.append(f"AC-6 gates: {gates}")

# --- AC-7 the VM boundary covers every TESTING.md trigger ------------------
vm = section(r"## Docker versus VM coverage", T)
m = re.search(r"required when behavior depends on (.+?)\.", flat(vm))
if not m:
    fails.append("AC-7 could not read TESTING.md's VM triggers — vacuous")
    triggers = []
else:
    triggers = [x.strip() for x in re.split(r",| or ", m.group(1)) if x.strip()]
f1 = flat(section(r"### 1\. Two harnesses"))
f8 = flat(section(r"### 8\. The VM harness"))
for tr in triggers:
    key = tr.split()[0].lower().rstrip(",")
    if key not in (f1 + f8).lower():
        fails.append(f"AC-7 neither § 1 nor § 8 dispositions TESTING.md's VM trigger {tr!r}")
if not re.search(r"do not satisfy their release gate|does not satisfy a release gate", f1):
    fails.append("AC-7 § 1 does not carry TESTING.md's refusal of container-only emulation")
# the two ADRs whose subject is on the VM side must be named as such
for adr in ("ADR-0007", "ADR-0009"):
    if adr not in f1:
        fails.append(f"AC-7 § 1 does not place {adr}'s design on a side of the split")
notes.append(f"AC-7 {len(triggers)} VM triggers: {triggers}")

# --- AC-8 the map is the register, and its limit is stated -----------------
inv = set(re.findall(r"^### (ARCH-[A-Z]+-\d+)", INV, re.M))
reg = set(re.findall(r"^\| (ARCH-[A-Z]+-\d+) \|", REG, re.M))
if inv - reg:
    fails.append(f"AC-8 the register does not cover {sorted(inv - reg)}")
if "Test architecture ADR" in REG:
    fails.append("AC-8 the register still names a design owner that does not exist")
f13 = flat(section(r"### 13\. The invariant map"))
if not re.search(r"updates those rows; it\s*does not copy them|exactly one map", f13):
    fails.append("AC-8 § 13 does not state the relationship to the register")
if len([r for r in rows(r"### 13\. ") if len(r) >= 3]) > 0:
    fails.append("AC-8 § 13 contains a table — the map must live only in the register")
if "0010-acceptance-harness" not in REG and "ADR-0010" not in REG:
    fails.append("AC-8 the register does not point at this ADR")
notes.append(f"AC-8 register covers all {len(reg)} invariants, no dangling owner")

# --- AC-9 the six risks, each disposed -------------------------------------
gated = {m.group(1) for m in re.finditer(r"^\| `(RSK-\d+)` \|(.+)$", TM, re.M)
         if re.search(r"\bP10\b", m.group(2))}
rr = {r[0].strip("`* "): r[1] for r in drop_header(
    [r for r in rows(r"## Residual risks") if len(r) == 2], "ID")}
EXPECT = {"RSK-1", "RSK-4", "RSK-9", "RSK-12", "RSK-13", "RSK-14"}
for rid in EXPECT:
    if rid not in rr:
        fails.append(f"AC-9 the ADR does not dispose of {rid}"); continue
    if not re.search(r"\*\*(Renewed|Resolved)", rr[rid]):
        fails.append(f"AC-9 {rid}'s disposition is neither renewed nor resolved")
    if not re.search(r"\b20\d\d-\d\d-\d\d\b", rr[rid]):
        fails.append(f"AC-9 {rid} carries no dated expiry")
    row = re.search(rf"^\| `{rid}` \|(.*)$", TM, re.M)
    if not row or "P10" not in row.group(1):
        fails.append(f"AC-9 the threat model does not record P10's disposition of {rid}")
notes.append(f"AC-9 disposed: {sorted(rr)}")

# --- AC-10 no renewal claims a demonstration P10 cannot make ---------------
res = flat(section(r"## Residual risks"))
if not re.search(r"P10 cannot demonstrate anything", res):
    fails.append("AC-10 § Residual risks does not state what P10 cannot demonstrate")
if not re.search(r"specification and not evidence|not a demonstration", res):
    fails.append("AC-10 § Residual risks does not distinguish specification from evidence")
for rid in ("RSK-1", "RSK-4", "RSK-9"):
    if rid in rr and re.search(r"\bor P10\b", rr[rid]):
        fails.append(f"AC-10 {rid} still expires at P10, which cannot demonstrate it")

# --- AC-11 no tracked file still gates a risk on P10 -----------------------
# DL-8's third tier: the conclusion is "P10 is where these are demonstrated",
# and it changed. Assert over every tracked file; classify every hit.
STALE = re.compile(r"(?:or|carried to|expire[s]? at|gated on)\s+\*?\*?P10\b", re.I)
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
    for i, u in enumerate(units):
        m = STALE.search(u)
        if not m:
            continue
        nxt = units[i + 1] if i + 1 < len(units) else ""
        if "Corrected at `G22`" in u or "Corrected at `G22`" in nxt:
            hits.append((f.name, "corrected in place")); continue
        if u[:m.start()].count('"') % 2 == 1:
            hits.append((f.name, "quoted")); continue
        if "acceptance-evidence" in f.name:
            hits.append((f.name, "record, superseded by its own terms")); continue
        # An ADR's own residual-risk row records THAT ADR's disposition at its
        # own time; THREAT-MODEL.md section 9 is authoritative for the current
        # gate. Named here rather than pattern-matched, and stated in the ADR's
        # section 13 so the exemption is documented rather than silent.
        if f.parent.name == "adr" and f.name != pathlib.Path(sys.argv[1]).name \
                and re.match(r"\| `RSK-\d+` \|", u):
            hits.append((f.name, "an earlier ADR's own disposition")); continue
        if re.search(r"re-gated|no longer|was carried (?:here|to)|it is not|"
                     r"records? P08'?s? disposition", u):
            hits.append((f.name, "describes the correction")); continue
        hits.append((f.name, "LIVE"))
        fails.append(f"AC-11 {f} still gates something on P10: …{u[max(0,m.start()-40):m.end()+40]}…")
if len(hits) < 3:
    fails.append(f"AC-11 the P10-gate sweep found {len(hits)} sites — it is vacuous")
if any(d == "an earlier ADR's own disposition" for _, d in hits) and \
        not re.search(r"THREAT-MODEL\.md` § 9 is\s*authoritative", A):
    fails.append("AC-11 exempts earlier ADRs' risk tables without the ADR stating why")
notes.append(f"AC-11 P10-gate sweep: {hits}")

# --- AC-12 every deferral names a trigger ----------------------------------
d = drop_header([r for r in rows(r"### Deferred decision points") if len(r) == 2], "Deferred")
if len(d) < 4:
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

## Four defects the checkers found in each other

**A check satisfied by either of two phrasings.** `AC-2` accepted *"not a test
build"* **or** *"configuration, not a build flag"*, and § 5 contains both — so
removing one left the case passing. Split into three separate conjuncts, each
planted.

**A bullet parser that lost the first bullet.** `AC-1` and `AC-2` read their
expected values from `TESTING.md` lists, and the whitespace-joining regex
swallowed the blank line before the first item — five topology elements read as
two. The parser now splits on the list marker instead of normalising around it.

**Two mutations that changed text without changing the property.** `AC-11`'s
reverted one cell of a table row and left the re-gating narrative in another;
`AC-2`'s removed one of two statements of the same rule. Both were caught by the
harness rather than reported as evidence.

**A header row read as data.** `AC-9`'s disposition map included the literal
column heading `ID` as a risk. Harmless here and the same shape as the defect
P08's checker shipped with.

## What these cases cannot detect

| Case | Cannot detect | Found instead by |
|---|---|---|
| `AC-1` | Whether the topology **runs**. Every element can be present and the compose file still not start | P11; C05 onward |
| `AC-2` | Whether a named mechanism actually causes the fault it claims | C14, which is the task that runs them |
| `AC-3` | Whether the generated JWTs match what C03 provisions | C03; C14 |
| `AC-4` | Whether "unchanged" is checked deeply enough — a probe misses a side effect it was not told to look for | Review; C08 |
| `AC-5` | **Whether redaction is complete.** It proves the scan runs and catches four seeded shapes | Nothing in `v0.6.0`; the release-candidate scans, and § 9 states it as a limit |
| `AC-6` | Whether a gate's contents suit its cadence | C15; operating the gates |
| `AC-7` | Whether a case placed in Docker genuinely works there | C13; VM tests |
| `AC-8` | **Whether the planned evidence would establish the invariant.** A filename is not a test | Review; P11, where a named file exists or does not |
| `AC-9` | Whether a renewal is **justified** | The maintainer, who owns each risk by name |
| `AC-10` | Whether the stated limit is the real one | Review |
| `AC-11` | Any **other** superseded conclusion. It asserts over the one this ADR changes | Review; the next task that changes a conclusion |
| `AC-12` | Whether a trigger is one anybody will notice | Review |

**And the thing no case reaches, which is this ADR's own subject**: whether any
of this would work. Every case here reads a document describing a harness. P10
runs nothing, `ARCH-TEST-003` is not satisfied by it (`ADR-0010` § 14), and the
first task where a named file exists or does not is **P11**.

That is not a hedge. It is the reason `RSK-1`, `RSK-4` and `RSK-9` move to C14
rather than closing here.
