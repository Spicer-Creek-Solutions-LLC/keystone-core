# P03 acceptance evidence

Required by [`P03.md`](P03.md) § 5.3. Every acceptance case is demonstrated
failing before it is accepted, and every case states what it cannot detect.

## Method

The checks read the ADR's **tables** — the staged protocol, the crash-recovery
boundaries, the three charter deferrals, the risk split, the clock assumptions —
and the residual-risk rows in `THREAT-MODEL.md`. That is `DL-2`'s countermeasure
applied before the fact.

Each demonstration asserts four things, per `DL-7`: the anchor is unique in its
file; the file changed; **the mutated document actually carries the defect,
verified by parsing it directly**; and the checker's failure names the case.

Two cases mutate `THREAT-MODEL.md` rather than the ADR, and restore it in a
`finally` block.

## The checker validates its own assumptions

`AC-1` measures the ADR against `ARCH-NATS-004`'s clauses and `AC-5` against the
charter's deferral sentence. The checker asserts those phrases still exist in
their source documents and fails if they do not, so a reworded invariant breaks
the check rather than silently narrowing it. Both sources are whitespace-
normalised first, because the phrases wrap across lines — the `DL-8` hazard that
bit P02's checker on its first run.

## The TESTING.md feature table

All ten cases are `N/A`, per case, for the reasons `P03.md` § 5.1 gives. The
**Restart** row is the uncomfortable one and the dossier says so: P03's whole
subject is what happens when a process dies between two durable steps, and it is
still `N/A` because the ADR *describes* recovery rather than performing it. C02
and C05 execute it.

## Demonstrations

Control: `== 0 failures ==`. Eleven cases across the nine acceptance cases:

| Defect planted | Case |
|---|---|
| A staged-protocol stage is absent | `AC-1` |
| A crash-recovery boundary has no path | `AC-1` |
| A key is described as server-generated and delivered to the agent | `AC-2` |
| Revocation is performed but not verified | `AC-3` |
| A journey 5.1 threat is unanswered | `AC-4` |
| A charter deferral is left undecided | `AC-5` |
| An asset that cited `RSK-7` lands in no part of the split | `AC-6` |
| A § 8 row still points at undivided `RSK-7` | `AC-6` |
| The clock table states no consequence | `AC-7` |
| A literal Keystone subject string appears | `AC-8` |
| Attestation states what is accepted and enumerates no limit | `AC-9` |

### Two checks found defects in the ADR, not in themselves

`AC-4` failed on the first run for `THR-01` and `THR-05`. Neither was answered
anywhere in the ADR: the process-listing constraint was discussed without being
tied to the threat it exists for, and **nothing addressed an agent enrolling
under another agent's identity at all**. Both are now answered — the second by
stating that the agent name is bound at creation and the enrolling party has no
field in which to assert a different one.

That is the acceptance suite doing the thing it is for, and it is worth
recording because most of what these suites catch is shape rather than
substance.

### Two demonstrations planted defects their cases were not meant to catch

`AC-7` and `AC-9` initially reported `MISS`, and the checks were right both
times. The `AC-9` planting removed one of four stated limits, leaving three — a
document with three limits does not have the defect "states what is accepted
without stating the limits". The `AC-7` planting renamed a table header while
leaving every consequence in the cells beneath it.

**`DL-7`'s third assertion should have caught both and did not, because the
predicates asserted that the text had changed rather than that the defect was
present.** That is the assertion's exact purpose, written too loosely. Both
plantings now remove the whole construct the case is about, and both predicates
parse the mutated section and assert the semantic property directly.

## What these cases cannot detect

| Case | Cannot detect | Found instead by |
|---|---|---|
| `AC-1` | Whether a recovery path is **correct**, or merely present. A path that resumes into an inconsistent state satisfies this exactly as well as one that does not | Review; C05, where the crash matrix is executed |
| `AC-2` | Whether an implementation honours it. The ADR can say "generated on the agent" and C01 can still generate server-side | C03 and C05, and `ARCH-TEST-002`'s negative identity tests |
| `AC-3` | Whether the verification **proves** revocation, as opposed to observing something consistent with it | Review; C03's negative matrix, where a revoked credential must fail to connect |
| `AC-4` | Whether the named mechanism **mitigates** the threat. It checks that the threat is mentioned, which is a floor and not an argument | Review; C14's adversarial suite |
| `AC-5` | Whether the decisions are **good**. A stated lifetime of a year passes | Review; the maintainer, who owns the resulting risk |
| `AC-6` | Whether the split is drawn where the material actually divides, and whether `AST-16` is owned by the task that can rotate it | Review; P05 and P10, when each inherits its part |
| `AC-7` | Whether the stated skew bound is **realistic** for the deployments the charter contemplates | Review; C05 and C14 |
| `AC-8` | Whether a role-named subject is describable in the grammar P04 will choose | P04, when a required role has no representable subject |
| `AC-9` | Whether the stated limits are the **real** ones, or whether a fourth limit exists that nobody thought to name | Review; C14, with an adversary assumed to hold a captured bundle |

`AC-4` and `AC-9` carry the most weight and the least certainty. Between them
they assert that every enrollment threat is addressed and that the product's
attestation is honestly described — and neither can tell you the answer is true.

## Reproducing

Save as `p03.py` and run from the repository root:

```
python3 p03.py docs/adr/0003-enrollment-and-identity.md
```

Exit `0` and `== 0 failures ==` is a pass.

```python
import re, sys, pathlib
A = pathlib.Path(sys.argv[1]).read_text()
INV = re.sub(r"\s+", " ", pathlib.Path("docs/project/ARCHITECTURE-INVARIANTS.md").read_text())
CH  = re.sub(r"\s+", " ", pathlib.Path("docs/project/PRODUCT-CHARTER.md").read_text())
THR = pathlib.Path("docs/project/THREAT-MODEL.md").read_text()
fails = []

def section(pat):
    m = re.search(pat + r"(.*?)(?=\n#{2,3} |\Z)", A, re.S)
    return m.group(1) if m else ""

def rows(pat):
    out = []
    for line in re.findall(r"^\|(?!-)(.+)\|$", section(pat), re.M):
        cells = [c.strip() for c in line.split("|")]
        if cells and cells[0].lower() not in ("stage", "step", "key", "asset", "risk", "question",
                                              "direction", "assumption", "crash between", "invariant"):
            out.append(cells)
    return out

# --- AC-1 ARCH-NATS-004's stages, in order, each with a recovery path -------
STAGES = ["issue pending", "fsync", "rename", "prove a permanent connection",
          "mark the identity active", "revoke bootstrap", "verify"]
for phrase in ["one-use, token-scoped NATS credentials", "Every stage has a defined crash-recovery path"]:
    if phrase not in INV:
        fails.append(f"AC-1 checker assumption stale: ARCH-NATS-004 no longer contains {phrase!r}")
proto = section(r"### 6\. The staged protocol\n")
order = []
for s in ["S0", "S1", "S2", "S3", "S4", "S5", "S6"]:
    if not re.search(rf"^\| {s} \|", proto, re.M): fails.append(f"AC-1 stage {s} is absent")
    else: order.append(s)
if order != sorted(order): fails.append("AC-1 stages are out of order")
low = proto.lower()
for tok, name in [("fsync", "fsync"), ("rename", "atomic rename"), ("0600", "mode 0600"),
                  ("proof of connection", "proof of a permanent connection"),
                  ("active", "activation"), ("revok", "revocation"), ("verif", "verification of revocation")]:
    if tok not in low: fails.append(f"AC-1 the staged protocol does not name {name}")
rec = section(r"### 7\. Crash recovery, per stage boundary\n")
for b in ["S0–S1", "S1–S2", "S2–S3", "S3–S4", "S4–S5", "S5–S6"]:
    if b not in rec: fails.append(f"AC-1 no recovery path for the {b} boundary")

# --- AC-2 the three agent keys are generated on the agent -------------------
keys = section(r"### 4\. Agent-generated keys\n")
if "on the agent" not in keys: fails.append("AC-2 does not state the keys are generated on the agent")
for k in ["AST-3", "AST-4", "AST-5"]:
    if k not in keys: fails.append(f"AC-2 {k} is not covered by the key-generation section")
if not re.search(r"Public half only", keys): fails.append("AC-2 does not state that only public halves leave the host")
for bad in [r"server generates (?:the )?(?:agent|signing|decryption)",
            r"(?:signing|decryption) key is (?:generated|created) by the server",
            r"deliver(?:s|ed)? the (?:signing|decryption) key to the agent"]:
    if re.search(bad, A, re.I): fails.append(f"AC-2 an output describes a key reaching the agent from the server: {bad}")

# --- AC-3 revocation is verified as a distinct step -------------------------
if not re.search(r"^\| S5 \|.*revok", proto, re.M | re.I): fails.append("AC-3 no revocation stage")
if not re.search(r"^\| S6 \|.*verif", proto, re.M | re.I):
    fails.append("AC-3 revocation is not verified in a distinct stage")
if "unverified revocation is not treated as a revocation" not in A:
    fails.append("AC-3 does not state that an unverified revocation is not a revocation")

# --- AC-4 every journey-5.1 threat is answered ------------------------------
five_one = [m for m in re.findall(r"^\| `(THR-\d+)` \|(.+)$", THR, re.M) if re.search(r"\| 5\.1 \|", m[1])]
if not five_one: fails.append("AC-4 found no journey 5.1 threats to check against")
for tid, _ in five_one:
    if tid not in A: fails.append(f"AC-4 {tid} is not answered anywhere in the ADR")

# --- AC-5 the charter's three deferrals are each decided --------------------
tok = section(r"### 2\. Token handling — the charter's three deferrals\n")
for q, name in [(r"standard input|stdin", "standard input"), (r"0600", "file mode"), (r"lifetime|expire", "lifetime")]:
    if not re.search(q, tok, re.I): fails.append(f"AC-5 the {name} question is not decided")
if not re.search(r"whether P03 also accepts the token on standard input", CH):
    fails.append("AC-5 checker assumption stale: the charter no longer defers standard input to P03")

# --- AC-6 RSK-7 split; every asset lands in exactly one part ----------------
ASSETS = ["AST-3", "AST-4", "AST-5", "AST-6", "AST-7", "AST-8", "AST-16"]
split = section(r"## Residual risks\n")
placed = {}
for r in rows(r"## Residual risks\n"):
    if len(r) < 3: continue
    rid = re.search(r"`(RSK-\d+)`", r[0])
    if not rid: continue
    for a in ASSETS:
        if re.search(rf"`{a}`", r[1]): placed.setdefault(a, []).append(rid.group(1))
for a in ASSETS:
    n = len(placed.get(a, []))
    if n == 0: fails.append(f"AC-6 {a} cited RSK-7 and lands in no part of the split")
    elif n > 1: fails.append(f"AC-6 {a} lands in {n} parts: {placed[a]}")
for rid in set(sum(placed.values(), [])):
    m = re.search(rf"^\| `{rid}` \|(.+)$", THR, re.M)
    if not m: fails.append(f"AC-6 {rid} is not a recorded residual risk"); continue
    cells = [c.strip() for c in m.group(1).rstrip("|").split("|")]
    if len(cells) < 5: fails.append(f"AC-6 {rid} row is malformed"); continue
    if not cells[2]: fails.append(f"AC-6 {rid} has no owner")
    if not re.search(r"\d{4}-\d{2}-\d{2}", cells[-1]): fails.append(f"AC-6 {rid} has no dated outcome")
for stale in re.findall(r"`RSK-7`", section(r"## 8\. ")):
    pass
sec8 = re.search(r"## 8\. Key rotation.*?(?=\n## 9\.)", THR, re.S)
if sec8:
    for line in sec8.group(0).splitlines():
        if line.startswith("|") and "RSK-7" in line and "resolved" not in line.lower():
            fails.append(f"AC-6 a § 8 row still points at undivided RSK-7: {line[:60]}")

# --- AC-7 clock assumptions stated with consequences ------------------------
clk = section(r"### 12\. Clock assumptions\n")
if not clk: fails.append("AC-7 no clock-assumptions section")
else:
    if not re.search(r"skew|agree within", clk, re.I): fails.append("AC-7 states no skew assumption")
    if not re.search(r"If it is wrong|Nothing:", clk): fails.append("AC-7 states no consequence of a wrong assumption")

# --- AC-8 no literal Keystone subject string --------------------------------
for t in re.findall(r"`([^`\n]+)`", A):
    if t.startswith(("$JS", "$SYS", "_INBOX", "ARCH-", "THR-", "RSK-", "ACT-", "AST-", "CAP-", "DL-", "ADR-")): continue
    if re.fullmatch(r"[a-z][a-z0-9_-]*(\.[a-z0-9_*>-]+){2,}", t):
        fails.append(f"AC-8 literal Keystone subject string: `{t}`")

# --- AC-9 attestation states what is accepted and what is not attempted -----
att = section(r"### 14\. Attestation, and what it does not attempt\n")
if not att: fails.append("AC-9 no attestation section")
else:
    if not re.search(r"accept", att, re.I): fails.append("AC-9 does not state what evidence is accepted")
    if not re.search(r"does not attempt|not attempt", att, re.I):
        fails.append("AC-9 does not state what attestation does not attempt")
    limits = len(re.findall(r"^- \*\*No ", att, re.M))
    if limits < 3: fails.append(f"AC-9 states {limits} limits; an attestation section naming fewer than three reads as a guarantee")

for f in fails: print("FAIL", f)
print(f"== {len(fails)} failures ==")
sys.exit(1 if fails else 0)
```
