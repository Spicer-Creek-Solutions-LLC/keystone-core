# P01 acceptance evidence

Required by [`P01.md`](P01.md) § 5.3. Every acceptance case is demonstrated
failing before it is accepted, and every case states what it cannot detect.

## Method

The checks read the threat model's **tables** — actors, assets, flows, threats,
invariant coverage, residual risks — not prose. That is `DL-2`'s countermeasure
applied before the fact rather than after a reviewer finds it, and it is why
the document carries structured tables in the first place.

Each demonstration asserts four things, per `DL-7`:

1. the anchor is unique in the source;
2. the document actually changed;
3. **the mutated document has the intended defect, verified by parsing it
   directly rather than by consulting the checker**; and
4. the checker's failure names the case under demonstration.

## The TESTING.md feature table

All ten cases are `N/A`, per case. P01 touches no runtime boundary: it produces
one Markdown document and changes no executable path.

| Case | `N/A` because |
|---|---|
| Intended target | No agent exists and no command crosses a transport |
| Non-target | Same; there is no second agent |
| Server isolation | No server process exists |
| Authorization denial | P01 provisions no identity and writes no policy; P04 owns the first executable policy |
| Payload protection | No payload is constructed |
| Duplicate delivery | Nothing is delivered |
| Restart | No durable state is written |
| Cancellation/timeout | No process is started |
| Audit | The model *specifies* what must be auditable; specifying is not exercising |
| Diagnostics | Specification only, as above |

That P01 is *about* security does not make a runtime case applicable to it.

## Demonstrations

Control: `== 0 failures ==`. Fifteen cases, each firing on its own case:

| Defect planted | Case |
|---|---|
| An actor is missing | `AC-1` |
| An actor has no capabilities | `AC-1` |
| An actor has no motivation | `AC-1` |
| An asset names no owning trust domain | `AC-2` |
| A flow names no boundary crossed | `AC-2` |
| A threat resolves to neither a mitigation nor a risk | `AC-3` |
| A threat cites a residual risk that is not recorded | `AC-3` |
| A residual risk has no dated expiry | `AC-4` |
| A residual risk has no compensating control | `AC-4` |
| The key rotation / revocation / recovery section is removed | `AC-5` |
| An invariant maps to no threat | `AC-6` |
| An invariant maps to a threat that does not exist | `AC-6` |
| An unresolvable invariant is cited | `AC-6` |
| A mitigation relies on a `Future / Unscheduled` capability | `AC-7` |
| A threat cites a journey the charter does not contain | `AC-7` |

One planting bug was caught by assertion 1 rather than producing false
evidence: the unresolvable-invariant case first anchored on a bare
`` `ARCH-EXEC-001` ``, which occurs five times.

## What these cases cannot detect

Stated as an output of P01, not learned afterwards. Every row says review,
which is why the dossier names a reviewer in § 8 rather than deferring it.

| Case | Cannot detect | Found instead by |
|---|---|---|
| `AC-1` | Whether an actor's capabilities are **realistic**, whether a threat names an actor that *has* the capability it requires, or whether an eleventh actor is missing entirely | Review — demonstrated, see below |
| `AC-2` | Whether the claimed owning domain is the **right** one, or whether a flow is absent altogether | Review against the charter's journeys |
| `AC-3` | **Whether a mitigation mitigates**, and **whether the cited control constrains the actor named beside it**. It checks that a control is named and that the citation resolves | Review — demonstrated, see below |
| `AC-4` | Whether a rationale is **sound** or a compensating control **effective** | The maintainer, who owns each risk by name |
| `AC-5` | Whether a section's treatment is **adequate**, only that the section exists | Review |
| `AC-6` | Whether the mapped threat is the one that invariant actually addresses. A shallow threat satisfies the coverage table exactly as well as a deep one | Review |
| `AC-7` | A mitigation relying on an **unstated** assumption rather than a named capability | Review; P11, when the assumption fails to appear in the skeleton |

`AC-3` and `AC-6` carry the weight here. Between them they establish that every
threat names a control and every invariant is addressed — and neither can tell
you the model is *correct*. G02 measured this directly over ten review rounds:
structural defects caught reliably, semantic defects never.

## Round 1 of review: what the cases could not reach

Independent review of `bc618c390` found five defects. **The acceptance checks
passed all five**, and they are the exact classes the limits table predicts.

| Finding | Class | Which case should have caught it |
|---|---|---|
| `ACT-2`'s blast radius asserted but never enumerated: `THR-28` covered audit rewriting, while `TD-SRV` also holds the signing key, the result-decryption key and the store | Semantic — a threat present but shallow | None. `AC-6`'s coverage table is satisfied by a shallow threat exactly as well as by a deep one, which the limits table already said |
| § 7 concluded that no denial power can produce a false success — true of the table above it, false of server compromise | Semantic — a claim broader than its evidence | None |
| `THR-35`'s control was a local socket, which does not survive compromise *of that host* | Semantic — control does not constrain the actor | None. `AC-3` checks that a control is named |
| `THR-11`/`RSK-1` argued a stolen signing key suffices, while its own compensating control argued the opposite | Internal contradiction between a threat and its risk | None |
| Actors named without the capability the threat requires — a network attacker reading a shell history; account isolation cited against the party who mints accounts | Semantic — actor/control misalignment | None |

Resolved by: enumerating the blast radius (`THR-44`–`THR-46`, § 5.3), widening
`RSK-4` to the whole of it, scoping § 7's conclusion and stating where the
ambiguity guarantee stops, recording `RSK-8` for workstation compromise,
restating `RSK-1` as a **joint** compromise, and adding two stated rules about
the actor column — the named actor must have the capability, and the cited
control must constrain that actor.

**This is the clearest evidence in Stage P for what the dossier's § 8 reviewer
is for.** Fifteen mechanical cases, all green, against five real semantic
defects — one of which (§ 7) was a guarantee stated more broadly than the design
supports, in a security document.

## A deviation from the approved plan

The plan expected five invariants to need a security-`N/A` waiver — the three
`ARCH-TEST-*`, `ARCH-COMM-003` and `ARCH-NATS-008` — as process and evidence
requirements rather than runtime ones. **None was waived.** Each names a way the
system becomes insecure without anyone observing it, which is a threat:
`THR-39` to `THR-43` model them. The document carries no security-`N/A` at all,
and `AC-6`'s waiver path is therefore untested by this task's evidence — it is
demonstrated only in the negative, by the case where an invariant maps to
nothing.

## Reproducing

Save as `p01.py` and run from the repository root:

```
python3 p01.py docs/project/THREAT-MODEL.md
```

Exit `0` and `== 0 failures ==` is a pass.

```python
import re, sys, pathlib
T = pathlib.Path(sys.argv[1]).read_text()
INV = pathlib.Path("docs/project/ARCHITECTURE-INVARIANTS.md").read_text()
CH  = pathlib.Path("docs/project/PRODUCT-CHARTER.md").read_text()
fails = []
def rows(pat, ncells):
    out = []
    for line in re.findall(pat, T, re.M):
        cells = [c.strip() for c in line.strip().strip("|").split("|")]
        out.append(cells)
    return out

# AC-1 ten actors, each with capabilities and motivations
acts = rows(r"^\| `(?:ACT-\d+)` \|.*$", 4)
if len(acts) != 10: fails.append(f"AC-1 {len(acts)} actors, expected 10")
for c in acts:
    if len(c) != 4 or not all(c):
        fails.append(f"AC-1 actor row incomplete: {c[0] if c else '?'}"); continue
    aid, name, caps, motive = c
    if len(caps) < 20: fails.append(f"AC-1 {aid} has no capabilities")
    if len(motive) < 10: fails.append(f"AC-1 {aid} has no motivation")
# a compromised credential must be an asset, and the actor the holder
if not re.search(r"`ACT-8` \| Attacker holding a compromised service credential", T):
    fails.append("AC-1 compromised credential is modelled as a party, not as an asset an attacker holds")

# AC-2 assets name an owning domain; flows name the boundaries they cross
assets = rows(r"^\| `(?:AST-\d+)` \|.*$", 3)
if not assets: fails.append("AC-2 no asset rows")
for c in assets:
    if len(c) != 3 or not all(c): fails.append(f"AC-2 asset row incomplete: {c[0] if c else '?'}"); continue
    if not re.search(r"TD-\w+", c[2]): fails.append(f"AC-2 {c[0]} names no owning trust domain")
flows = rows(r"^\| `(?:FLW-\d+)` \|.*$", 4)
if not flows: fails.append("AC-2 no flow rows")
for c in flows:
    if len(c) != 4 or not all(c): fails.append(f"AC-2 flow row incomplete: {c[0] if c else '?'}"); continue
    if not re.search(r"TB-\d+", c[2]): fails.append(f"AC-2 {c[0]} names no boundary crossed")

# AC-3 every threat resolves to a mitigation or a residual risk
threats = rows(r"^\| `(?:THR-\d+)` \|.*$", 6)
if not threats: fails.append("AC-3 no threat rows")
THR_IDS = set()
for c in threats:
    if len(c) != 6 or not all(c): fails.append(f"AC-3 threat row incomplete: {c[0] if c else '?'}"); continue
    tid, desc, actor, asset, journey, mit = c
    THR_IDS.add(tid.strip("`"))
    if not (re.search(r"ARCH-[A-Z]+-\d+", mit) or re.search(r"RSK-\d+", mit) or re.search(r"\b[PC]\d\d\b", mit)):
        fails.append(f"AC-3 {tid} resolves to neither a mitigation nor a residual risk")
    if not re.search(r"ACT-\d+", actor): fails.append(f"AC-3 {tid} names no actor")
    if not re.search(r"AST-\d+|all", asset): fails.append(f"AC-3 {tid} names no asset")

# AC-4 residual risks carry owner, rationale, compensating control, expiry
risks = rows(r"^\| `(?:RSK-\d+)` \|.*$", 6)
if not risks: fails.append("AC-4 no residual-risk rows")
RSK_IDS = set()
for c in risks:
    if len(c) != 6 or not all(c):
        fails.append(f"AC-4 residual-risk row missing one of its four elements: {c[0] if c else '?'}"); continue
    rid, risk, owner, rationale, control, expiry = c
    RSK_IDS.add(rid.strip("`"))
    if len(rationale) < 20: fails.append(f"AC-4 {rid} has no rationale")
    if len(control) < 4: fails.append(f"AC-4 {rid} has no compensating control")
    if not re.search(r"\d{4}-\d{2}-\d{2}", expiry): fails.append(f"AC-4 {rid} has no dated expiry")
for c in threats:
    if len(c) == 6:
        for r in re.findall(r"RSK-\d+", c[5]):
            if r not in RSK_IDS: fails.append(f"AC-3 {c[0]} cites {r}, which is not a recorded residual risk")

# AC-5 the five required subjects each have their own section
for heading, label in [(r"^## 6\. Metadata leakage", "metadata leakage"),
                       (r"^## 7\. Denial and delay powers", "denial and delay powers"),
                       (r"^## 8\. Key rotation, revocation and recovery", "key rotation/revocation/recovery")]:
    if not re.search(heading, T, re.M): fails.append(f"AC-5 no section for {label}")
s8 = re.search(r"^## 8\..*?(?=^## )", T, re.S | re.M)
if s8:
    for word in ["otation", "evocation", "ecovery"]:
        if word not in s8.group(0): fails.append(f"AC-5 § 8 does not address {word}")

# AC-6 all 24 invariants map to a threat or carry a security-N/A rationale; citations resolve
known = set(re.findall(r"### (ARCH-[A-Z]+-\d+)", INV))
if len(known) != 24: fails.append(f"AC-6 expected 24 invariants, found {len(known)}")
cov = {}
for line in re.findall(r"^\| `(ARCH-[A-Z]+-\d+)` \| (.+?) \|$", T, re.M):
    cov[line[0]] = line[1]
for inv in sorted(known):
    if inv not in cov: fails.append(f"AC-6 {inv} appears in no coverage row")
    else:
        cell = cov[inv]
        ids = re.findall(r"THR-\d+", cell)
        if ids:
            for t in ids:
                if t not in THR_IDS: fails.append(f"AC-6 {inv} maps to {t}, which is not a modelled threat")
        elif "N/A" in cell:
            if len(cell) < 40: fails.append(f"AC-6 {inv} security-N/A carries no rationale")
        else:
            fails.append(f"AC-6 {inv} maps to no threat and carries no security-N/A rationale")
for i in sorted(set(re.findall(r"ARCH-[A-Z]+-\d+", T)) - known):
    fails.append(f"AC-6 unresolvable invariant {i}")

# AC-7 no mitigation depends on a capability outside v0.6.0 scope
for c in threats:
    if len(c) == 6 and re.search(r"CAP-[A-Z]+-\d+", c[5]):
        fails.append(f"AC-7 {c[0]} mitigation relies on a Future / Unscheduled capability")
# every journey cited must exist in the charter
for j in sorted({m for c in threats if len(c)==6 for m in re.findall(r"5\.\d", c[4])}):
    if not re.search(rf"^### {re.escape(j)} ", CH, re.M): fails.append(f"AC-7 threat cites journey {j}, absent from the charter")

for f in fails: print("FAIL", f)
print(f"== {len(fails)} failures ==")
sys.exit(1 if fails else 0)
```
