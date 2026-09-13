# P02 acceptance evidence

Required by [`P02.md`](P02.md) § 5.3. Every acceptance case is demonstrated
failing before it is accepted, and every case states what it cannot detect.

## Method

The checks read the ADR's **tables** — the two capability matrices, the
subject-role table, the allowlist, the invariant coverage table — and the three
residual-risk rows in `THREAT-MODEL.md`. That is `DL-2`'s countermeasure applied
before the fact, and it is why [`ADR-0002`](../adr/0002-nats-native-capabilities.md)
carries structured tables rather than prose.

Each demonstration asserts four things, per `DL-7`:

1. the anchor is unique in its file;
2. the file actually changed;
3. **the mutated document has the intended defect, verified by parsing it
   directly rather than by consulting the checker**; and
4. the checker's failure names the case under demonstration.

Two cases mutate `THREAT-MODEL.md` rather than the ADR, because `AC-7` is a
claim about the threat model's residual-risk rows. Those restore the file in a
`finally` block.

## The checker validates its own assumptions

`AC-1` and `AC-2` are claims about **RFC 0001's** required and deferred
capability sets. The checker does not hardcode those lists silently: it asserts
each phrase still appears in RFC 0001 and fails if one does not. If the RFC's
capability set changes, this check fails rather than quietly measuring the ADR
against a stale list.

That assertion fired on its first run, for a reason worth recording: the phrases
are **wrapped across lines** in RFC 0001, and a substring test on the raw file
missed three of them. The checker now normalises whitespace before testing. This
is `DL-8`'s line-wrap hazard — written into the ledger hours earlier in `G07` —
occurring in the first task after it was recorded.

## The TESTING.md feature table

All ten cases are `N/A`, per case. P02 produces one Markdown document and
changes no executable path. It *decides* the authorization boundary; deciding is
not exercising.

| Case | `N/A` because |
|---|---|
| Intended target | No agent exists and no command crosses a transport |
| Non-target | Same; there is no second agent |
| Server isolation | No server process exists |
| Authorization denial | P02 names principals and the direction and scope each requires. It writes neither a permission matrix nor a policy — both are P04's, which owns the first executable denial test |
| Payload protection | No payload is constructed; the envelope is P05's |
| Duplicate delivery | Nothing is delivered; `Nats-Msg-Id` is specified, not exercised |
| Restart | No durable state is written |
| Cancellation/timeout | No process is started |
| Audit | Advisory consumption is specified, not exercised |
| Diagnostics | Specification only, as above |

## Demonstrations

Control: `== 0 failures ==`. Eleven cases across the eight acceptance cases:

| Defect planted | Case |
|---|---|
| A required capability has no matrix row | `AC-1` |
| A required capability carries two verdicts | `AC-1` |
| A deferred capability is silently adopted with no demonstrated need | `AC-2` |
| Evidence merely restates the verdict | `AC-3` |
| A bare `$JS.ACK.>` is permitted | `AC-4` |
| A consumer-scoped entry names no exact stream or consumer | `AC-4` |
| A principal-scoped entry claims a consumer it cannot have | `AC-4` |
| A required principal is not named | `AC-5` |
| An invariant is registered with no owning task | `AC-6` |
| A P02 risk is left with its original expiry | `AC-7` |
| A literal Keystone subject string appears | `AC-8` |

### A check that could not fail, caught by its own demonstration

`AC-7` first tested the **whole residual-risk row** for the words `Renewed` or
`Resolved`. The demonstration restored `RSK-10`'s original expiry — the exact
defect `AC-7` exists to catch — and the check passed, because the row's *other*
cells still said "Resolved at P02".

It now reads the **expiry cell specifically**. The demonstration is what found
this; nothing about the check looked wrong when it was written, and it would
have shipped as evidence that P02 discharged a risk it had not.

This is `DL-1` caught before it became evidence, and it is the second time in
this task that writing the demonstration found a defect in the check rather than
in the document.

### A check weaker than the claim it enforced

`AC-4`'s prose required every allowlist entry to name one exact stream and one
exact consumer. **The check never tested that clause at all** — it tested for
`*`, for bare wildcards, for administrative subjects and for a missing
justification. So when the ADR added a publisher's publish-acknowledgement
inbox, which has no consumer and therefore contradicted the contract, the
checker reported `== 0 failures ==` and review found it instead.

`G09` then made `AC-4` shape-aware, because the contradiction was in the case
rather than in the ADR: a `PubAck` belongs to no consumer, so the entry a
publisher needs could never have satisfied the rule. The check now tests both
halves — a consumer-scoped entry must name its exact stream and consumer, a
principal-scoped entry must not claim either — and two demonstrations cover them.

**Three defects on this task were in the checks rather than in the document**:
the stale-phrase assertion, `AC-7`'s whole-row test, and this one. Two were
caught by writing the demonstrations and one by review. That ratio is the honest
measure of how much weight the mechanical cases carry, and it is why the section
below is a required output rather than a courtesy.

## What these cases cannot detect

Stated as an output of P02, not learned afterwards.

| Case | Cannot detect | Found instead by |
|---|---|---|
| `AC-1` | Whether a capability **absent from RFC 0001's list** is relevant and missing. The list is a floor, not a census | Review; C03, when a needed capability has no decision to implement |
| `AC-2` | Whether a `Defer` is honest, or an `Adopt` the author did not want to argue for | Review — the deferred set is the reviewer's primary target |
| `AC-3` | Whether the evidence is **true**, or supports the verdict beside it. It measures length and non-restatement, which a plausible falsehood passes | Review against the cited NATS documentation |
| `AC-4` | Whether the allowlist is **sufficient** for the principal it serves or **minimal**; whether a permitted trailing wildcard is bounded as tightly as that principal allows; and **whether an entry is classified into the right shape** — a consumer-scoped entry mislabelled as principal-scoped escapes the stream-and-consumer requirement entirely | C03's negative identity matrix: an insufficient allowlist fails to connect, an excessive one fails a denial test |
| `AC-5` | Whether the direction and scope named for a principal are the **right** authority for it | Review; P04's generated JWTs and negative tests (`ARCH-TEST-002`) |
| `AC-6` | Whether a section claiming to satisfy an invariant **does**. A shallow section satisfies the map as well as a deep one | Review — the same limit P01's `AC-6` carried, restated because the shape is identical |
| `AC-7` | Whether a renewal is **justified**, or a resolution **correct**. `RSK-10` is recorded resolved because a per-subject limit bounds a per-agent subject; if that mechanism does not behave as documented, this case still passes | C03 and C14, where the limit is configured and attacked |
| `AC-8` | Whether a **role-named** subject is describable in the grammar P04 will choose. P02 can hand P04 an impossible shape and this case will not notice | P04, when a required role has no representable subject |

`AC-3` and `AC-7` carry the most weight and the least certainty: between them
they assert that every capability decision is evidenced and every expiring risk
is discharged, and neither can tell you the evidence is true or the discharge is
sound.

## Reproducing

Save as `p02.py` and run from the repository root:

```
python3 p02.py docs/adr/0002-nats-native-capabilities.md
```

Exit `0` and `== 0 failures ==` is a pass.

```python
import re, sys, pathlib
A = pathlib.Path(sys.argv[1]).read_text()
RFC = re.sub(r"\s+", " ", pathlib.Path("docs/rfcs/0001-generation-2-reboot.md").read_text())
INV = pathlib.Path("docs/project/ARCHITECTURE-INVARIANTS.md").read_text()
THR = pathlib.Path("docs/project/THREAT-MODEL.md").read_text()
fails = []
VERDICTS = ["Adopt", "Evaluate", "Defer", "Reject"]

def rows(section_pat):
    m = re.search(section_pat + r"(.*?)(?=\n#{2,4} |\Z)", A, re.S)
    if not m: return []
    out = []
    for line in re.findall(r"^\|(?!-)(.+)\|$", m.group(1), re.M):
        cells = [c.strip() for c in line.split("|")]
        if cells and cells[0].lower() not in ("capability", "principal", "invariant", "risk", "scope", "advisory", "mode", "key", "account", "stream"):
            out.append(cells)
    return out

# --- AC-1 / AC-2: the two capability sets, taken from RFC 0001 rather than assumed
REQUIRED = {"accounts":"accounts", "nkey":"NKeys/JWT", "subject permission":"exact subject permissions",
            "tls":"TLS", "stream":"JetStream streams", "durable consumer":"durable consumers",
            "acknowledgement":"explicit acknowledgements", "nats-msg-id":"`Nats-Msg-Id` publication deduplication",
            "limits":"bounded stream and account limits", "system account":"a system account",
            "advisor":"advisories"}
DEFERRED = {"response permission":"Response permissions", "kv":"KV", "subject mapping":"subject mappings",
            "object store":"Object Store", "queue group":"queue groups", "leaf node":"leaf nodes",
            "supercluster":"superclusters"}
for key, phrase in list(REQUIRED.items()) + list(DEFERRED.items()):
    if phrase not in RFC:
        fails.append(f"AC-1 checker assumption stale: RFC 0001 no longer contains {phrase!r}")

req = rows(r"#### RFC 0001's required initial set\n")
dfr = rows(r"#### RFC 0001's deferred set\n")
if not req: fails.append("AC-1 no required-set matrix")
if not dfr: fails.append("AC-2 no deferred-set matrix")

def verdicts_in(cell):
    return [v for v in VERDICTS if re.search(rf"`{v}`", cell)]

for key in REQUIRED:
    hit = [r for r in req if key in r[0].lower()]
    if not hit: fails.append(f"AC-1 required capability {key!r} has no matrix row"); continue
    for r in hit:
        v = verdicts_in(r[1])
        if len(v) == 0: fails.append(f"AC-1 {r[0]} carries no verdict")
        elif len(v) > 1: fails.append(f"AC-1 {r[0]} carries {len(v)} verdicts")

for key in DEFERRED:
    hit = [r for r in dfr if key in r[0].lower()]
    if not hit: fails.append(f"AC-2 deferred capability {key!r} is absent from the matrix"); continue
    for r in hit:
        v = verdicts_in(r[1])
        if len(v) != 1: fails.append(f"AC-2 {r[0]} does not carry exactly one verdict"); continue
        if v[0] == "Adopt" and not re.search(r"demonstrated need", r[2], re.I):
            fails.append(f"AC-2 {r[0]} is adopted without naming the demonstrated need")
        if v[0] == "Evaluate":
            fails.append(f"AC-2 {r[0]} is left at Evaluate; RFC 0001 requires Defer, Reject, or Adopt with need")

# --- AC-3 evidence is present and is not a restatement of the verdict
for r in req + dfr:
    if len(r) < 3 or not r[2]:
        fails.append(f"AC-3 {r[0] if r else '?'} has no evidence"); continue
    ev = r[2]
    if len(ev) < 30: fails.append(f"AC-3 {r[0]} evidence is too thin to be evidence")
    bare = re.sub(r"[`.\s]", "", ev).lower()
    if bare in [v.lower() for v in VERDICTS] or re.fullmatch(r"(adopted|deferred|rejected|native|thenativemechanism)", bare):
        fails.append(f"AC-3 {r[0]} evidence merely restates the verdict")

# --- AC-4 allowlist entries are bounded to one stream and one consumer
allow = rows(r"### 8\. Data-plane .*?\n")
if not allow: fails.append("AC-4 no allowlist table")
ADMIN = ["CREATE", "DELETE", "UPDATE", "LIST", "PURGE", "NAMES", "ACCOUNT.INFO"]
SHAPES = ("Consumer-scoped", "Principal-scoped")
for r in allow:
    if len(r) < 4:
        fails.append(f"AC-4 entry does not declare a shape: {r}"); continue
    principal, shape, permitted, why = r[0], r[1], r[2], r[3]
    if shape not in SHAPES:
        fails.append(f"AC-4 entry shape {shape!r} is not one of {SHAPES}"); continue
    # the clause AC-4 actually states, tested rather than assumed
    names_stream_and_consumer = bool(re.search(r"\$JS\.(?:API\.CONSUMER\.MSG\.NEXT|ACK)\.\w+\.<[^>]+>", permitted))
    claims_stream_or_consumer = bool(re.search(r"KS_\w+|consumer", permitted, re.I))
    if shape == "Consumer-scoped" and not names_stream_and_consumer:
        fails.append(f"AC-4 consumer-scoped entry names no exact stream and consumer: {permitted}")
    if shape == "Principal-scoped" and claims_stream_or_consumer:
        fails.append(f"AC-4 principal-scoped entry claims a stream or consumer it cannot have: {permitted}")
    if "*" in permitted: fails.append(f"AC-4 entry wildcards a token with '*': {permitted}")
    if re.search(r"`?\$JS\.ACK\.>", permitted) or re.search(r"`?_INBOX\.>", permitted):
        fails.append(f"AC-4 bare wildcard entry: {permitted}")
    for a in ADMIN:
        if a in permitted.upper(): fails.append(f"AC-4 administrative $JS.API subject permitted: {permitted}")
    if permitted.rstrip("`").endswith(".>") and not re.search(r"justif", why, re.I):
        fails.append(f"AC-4 trailing wildcard carries no justification: {permitted}")

# --- AC-5 every ARCH-NATS-002 principal, with direction and scope
NEEDED = ["command publisher", "enrollment service", "result consumer", "presence consumer", "monitoring role", "agent"]
prin = rows(r"### 4\. Subject roles.*?\n")
if not prin: fails.append("AC-5 no subject-role table")
for name in NEEDED:
    hit = [r for r in prin if r[0].strip("* `").lower().startswith(name.split()[0]) and name.split()[-1] in r[0].lower()]
    if not hit: fails.append(f"AC-5 principal {name!r} is not named with a direction and scope"); continue
    for r in hit:
        if len(r) < 4 or not r[1] or not r[2]: fails.append(f"AC-5 {r[0]} has no direction")
        elif len(r) < 4 or not r[3]: fails.append(f"AC-5 {r[0]} has no scope")

# --- AC-6 every ARCH-NATS-* maps to a section, registered ones name an owner
known = set(re.findall(r"### (ARCH-NATS-\d+)", INV))
cov = {}
for line in re.findall(r"^\| `(ARCH-[A-Z]+-\d+)` \| (.+?) \|$", A, re.M):
    cov[line[0]] = line[1]
for inv in sorted(known):
    if inv not in cov: fails.append(f"AC-6 {inv} appears in no coverage row"); continue
    cell = cov[inv]
    if "registered" in cell.lower():
        if not re.search(r"\b[PC]\d\d\b", cell): fails.append(f"AC-6 {inv} is registered with no owning task named")
    elif "satisf" not in cell.lower():
        fails.append(f"AC-6 {inv} neither satisfies nor registers")

# --- AC-7 the three risks naming P02 are resolved or renewed, dated, with a reason
for rid in ["RSK-2", "RSK-3", "RSK-10"]:
    m = re.search(rf"^\| `{rid}` \|(.+)$", THR, re.M)
    if not m: fails.append(f"AC-7 {rid} is absent from the threat model"); continue
    cells = [c.strip() for c in m.group(1).rstrip("|").split("|")]
    if len(cells) < 5: fails.append(f"AC-7 {rid} row is malformed"); continue
    expiry = cells[-1]
    if not re.search(r"Renewed|Resolved", expiry):
        fails.append(f"AC-7 {rid} expiry records no P02 outcome: {expiry!r}")
    if not re.search(r"\d{4}-\d{2}-\d{2}", expiry):
        fails.append(f"AC-7 {rid} expiry carries no date")
    if "P02" not in m.group(1): fails.append(f"AC-7 {rid} does not name the deciding task")

# --- AC-8 no literal Keystone subject string
for tok in re.findall(r"`([^`\n]+)`", A):
    if tok.startswith(("$JS", "$SYS", "_INBOX", "ARCH-", "THR-", "RSK-", "ACT-", "AST-", "CAP-", "DL-", "ADR-")): continue
    if re.fullmatch(r"[a-z][a-z0-9_-]*(\.[a-z0-9_*>-]+){2,}", tok):
        fails.append(f"AC-8 literal Keystone subject string: `{tok}`")

for f in fails: print("FAIL", f)
print(f"== {len(fails)} failures ==")
sys.exit(1 if fails else 0)
```
