# Generation 2 Threat Model

Normative for Generation 2. It models threats against the seven canonical
journeys in [`PRODUCT-CHARTER.md`](PRODUCT-CHARTER.md) § 5 and the argv-only
execution boundary in § 6, and it is the input P02–P09 design against.

## 1. Scope and method

### What is modelled

The product the charter describes and nothing else: one operator, one server,
one NATS broker, and Linux agents, exchanging enrollment, commands,
cancellations, results and presence. A threat against a journey the charter
contains is in scope. A threat against a capability the charter does not
contain is not, because that capability does not exist to be attacked.

### Method

Threats are enumerated by walking **each canonical journey across each trust
boundary it crosses, for each actor with capability at that boundary**, then
sweeping all 24 architecture invariants to catch what the journey walk missed.
Both passes are recorded: § 5 carries the threats, § 5.2 carries the invariant
sweep.

Every threat names an actor, an asset, the journey it attacks, and either a
mitigation or an accepted residual risk. A threat with neither is not modelled,
it is merely mentioned.

### What is explicitly not modelled

- **Physical access to a host.** An attacker with root on an agent host *is*
  the compromised-agent actor (§ 4); how they got there is out of scope.
- **Cryptographic primitive failure.** AES, Ed25519, X25519, **ML-KEM-768 and
  ML-DSA-65** (added at `G37`, when `ADR-0005` §§ 4 and 5 named them) and the
  NATS NKey construction are assumed sound. **The two post-quantum primitives are
  carried in hybrid** alongside their classical counterparts, so this assumption
  is weaker than it looks: an envelope survives the failure of either member of a
  pair. It does not survive the failure of both. Their *misuse* — key reuse across layers,
  absent verification — is modelled.
- **The operator's own intent.** An authorized operator running a destructive
  command is the product working. A *malicious command author* who is not the
  operator is actor `ACT-9`.
- **Availability of the broker as a product requirement.** Generation 2 has no
  HA commitment; broker loss is a denial power (§ 7), not a threat to mitigate.
- **Generation 1.** Its code is archived and unsupported.

### Terms

This document uses [`GLOSSARY.md`](GLOSSARY.md). Three terms it does not yet
define, because P01 is what decides them:

- **Trust domain** — a region under one administrative authority, inside which
  a compromise is total. Named `TD-*` in § 2.
- **Trust boundary** — a point where data or control passes between two trust
  domains. Named `TB-*` in § 3.
- **Capability** — what an actor can do *without* defeating a control, as
  distinct from what it could do by breaking one.

Promoting these into the glossary is a later supporting task, once P01 is
accepted.

## 2. Assets

Each asset has one **home domain**: where it is authoritative, and where its
compromise is total. A copy may exist elsewhere transiently — a token in the
operator's hands, a command payload in flight — and those copies travel as data
flows (§ 3). Their exposure is modelled by the boundaries the flow crosses, not
by reassigning the asset to a second home.

| Trust domain | Contents |
|---|---|
| `TD-OP` | The operator's workstation: the `keystone` CLI, its configuration, the local API socket |
| `TD-SRV` | The server deployment: the server process, its keys, its durable store |
| `TD-BRK` | The NATS broker: the broker process, the operator and account seeds, the system account |
| `TD-AGT` | One agent host: the agent process, its credentials, its job ledger, and the processes it starts. **Each host is its own domain** — compromise of one is not compromise of another |

| ID | Asset | Home domain |
|---|---|---|
| `AST-1` | One-use enrollment token | `TD-SRV` — issued there; carried by `FLW-1` and `FLW-2` |
| `AST-2` | Bootstrap NATS credential | `TD-AGT` |
| `AST-3` | Permanent scoped agent NATS credential | `TD-AGT` |
| `AST-4` | Agent envelope-signing key | `TD-AGT` |
| `AST-5` | Agent payload-decryption key | `TD-AGT` |
| `AST-6` | Service NATS credentials — command and cancellation publisher, enrollment, result consumer, presence consumer | `TD-SRV` |
| `AST-7` | Service envelope-signing key | `TD-SRV` |
| `AST-8` | Result-service payload-decryption key | `TD-SRV` |
| `AST-9` | NATS operator and account seeds | `TD-BRK` |
| `AST-10` | Command payload — the argv vector and its bounds | `TD-SRV` — carried by `FLW-4`, `FLW-5`, `FLW-6` |
| `AST-11` | Result payload — remote exit status and captured output | `TD-AGT` — carried by `FLW-7`, `FLW-10` |
| `AST-12` | Agent job ledger | `TD-AGT` |
| `AST-13` | Server job and audit store | `TD-SRV` |
| `AST-14` | Local operator API socket. It sits on the `TB-1` boundary and is listed here because it is what an operator holds; **the file itself is owned by `TD-SRV`** (`ADR-0009` § 1), which is what stops a compromised workstation widening its own access | `TD-OP` |
| `AST-15` | Presence and inventory state | `TD-SRV` |
| `AST-16` | Keystone account signing key — mints user JWTs inside the Keystone account | `TD-SRV` |

`ARCH-NATS-006` requires `AST-3`, `AST-4` and `AST-5` to be **separate keys**,
and forbids reusing an NKey seed as a Keystone signing or encryption key. They
are listed separately here for that reason, not for tidiness.

`AST-16` is an **NKey seed**, and the same invariant forbids reusing it as
`AST-7`. It is listed separately from `AST-6` because minting an identity and
publishing as one are different powers: `ADR-0002` § 2 gives the server the
account signing key so enrollment can issue a permanent agent identity, and
withholds the operator seed so the server cannot create accounts.

**Cancellation authority is modelled as the command publisher's.** The charter's
5.6 journey introduces no party its 5.3 journey does not already have, and
`FLW-8` has the same direction and the same per-agent scope as `FLW-5`, so
`AST-6`'s publisher role covers cancellation subjects. That is **this model's
assumption and not a decided ADR**: P04 owns subject authorization and may issue
a distinct cancellation credential instead, which would narrow `ACT-8`'s reach
rather than widen it. Recorded here because the alternative — leaving
cancellation unattributed — is what made `THR-24` name an actor with no
capability for it.

## 3. Trust boundaries and the flows that cross them

A trust boundary is **a point where data or control passes between regions of
differing authority**. Most lie between two trust domains. `TB-4` and `TB-5` lie
*inside* one, because authority changes there too — an executed process holds
less than the agent that started it, and the system account holds more than the
Keystone account. `TB-6` originates outside every domain and reaches all four.

| ID | Boundary | Between |
|---|---|---|
| `TB-1` | Operator to server | `TD-OP` ↔ `TD-SRV`, over the local API socket. **Authorization is evaluated at this boundary from kernel-supplied peer credentials, before any request byte is parsed** (`ADR-0009` § 3) |
| `TB-2` | Server to broker | `TD-SRV` ↔ `TD-BRK` |
| `TB-3` | Broker to agent | `TD-BRK` ↔ `TD-AGT` |
| `TB-4` | Agent to executed process | `TD-AGT` internal, but a privilege and lifetime boundary |
| `TB-5` | Keystone account to system account | Inside `TD-BRK`; `ARCH-NATS-001` and `ARCH-NATS-005` exist to keep it closed |
| `TB-6` | Supply chain to every domain | Build, packaging and install reach all four |

`ARCH-COMM-002` forbids assuming `TD-SRV` and `TD-AGT` can reach each other
directly. Every flow between them crosses `TB-2` **and** `TB-3`, with the
broker in the middle — which is why the broker is its own trust domain rather
than a pipe.

| Flow | Journey | Crosses | Carries |
|---|---|---|---|
| `FLW-1` | 5.1 Enroll | `TB-1` | Token request and the issued token |
| `FLW-2` | 5.1 Enroll | `TB-2`, `TB-3` | Bootstrap connection, token proof, permanent credential issue — the server validates and issues, so the flow crosses both |
| `FLW-3` | 5.2 Presence | `TB-3`, `TB-2` | Agent presence publications |
| `FLW-4` | 5.3 Run | `TB-1` | Operator's argv and target |
| `FLW-5` | 5.3 Run | `TB-2`, `TB-3` | Signed, encrypted command envelope |
| `FLW-6` | 5.3 Run | `TB-4` | argv to the executed process, output back |
| `FLW-7` | 5.3–5.5 Result | `TB-3`, `TB-2` | Signed, encrypted result envelope |
| `FLW-8` | 5.6 Cancel | `TB-2`, `TB-3` | Cancellation envelope |
| `FLW-9` | 5.6 Cancel | `TB-4` | Signal to the process group |
| `FLW-10` | 5.4, 5.5, 5.7 | `TB-1` | Status, output and audit records to the operator |

## 4. Actors

The ten the workstream names. **Capabilities are what the actor can do without
defeating a control**; a threat that requires breaking one is described in § 5
as an attack on that control, not as a capability here.

Note `ACT-8`: a compromised credential is an **asset** (`AST-3`, `AST-6`), not a
party. The actor is whoever holds it.

| ID | Actor | Capabilities | Motivation |
|---|---|---|---|
| `ACT-1` | Operator | Issue enrollment tokens, target any enrolled agent, run bounded commands, cancel, read results and audit | Legitimate. Modelled because their workstation and socket are attack surface, not because they are hostile |
| `ACT-2` | Server service | Publish commands and cancellations, consume results and presence, write the audit store, issue tokens | Legitimate. Modelled because its compromise is the widest single failure |
| `ACT-3` | Agent | Subscribe to its own command and cancellation subjects, publish its own result, event and presence subjects, execute bounded argv | Legitimate. Its authority is deliberately narrow |
| `ACT-4` | NATS operator | Holds `AST-9`. Can mint accounts and users, and therefore any Keystone identity | Administers the broker. Its authority exceeds the product's |
| `ACT-5` | Broker administrator | Runs the broker process. Sees every subject, header and byte on the wire; can drop, delay or reorder | Operates infrastructure Keystone does not own |
| `ACT-6` | Network attacker | Observes and manipulates traffic between domains; cannot read TLS plaintext without a key | Opportunistic or targeted |
| `ACT-7` | Compromised agent | Everything `ACT-3` has, plus `AST-2`–`AST-5`, the ledger, and root on that host | Pivot to other agents, the server, or the fleet |
| `ACT-8` | Attacker holding a compromised service credential | Exactly what that **one** credential authorizes — publishing commands and cancellations, or consuming results. It holds no Keystone signing or decryption key and no access to the server's store: those live in `TD-SRV`, and a party holding them too is `ACT-2`'s compromise, not this one | Impersonate the control plane |
| `ACT-9` | Malicious command author | Supplies argv that reaches an agent, without being the operator — via a compromised workstation, or an operator misled into running it | Execute on the fleet under legitimate authority |
| `ACT-10` | Supply-chain attacker | Alters binaries, packages or dependencies before install | Persistent access to every host that installs |

## 5. Threats

Each threat names an actor, an asset, the journey it attacks, and a mitigation
or an accepted residual risk (`RSK-*`, § 9). Mitigations cite the invariant that
requires them; where a mitigation's *shape* belongs to a later ADR, the owning
task is named rather than guessed.

**Two rules about the actor column, because getting them wrong makes a threat
look mitigated when it is not.** First, *the named actor must have the named
capability* — a network attacker cannot read a local shell history, so it is not
the actor for a threat that requires one. Second, *the cited control must
constrain the named actor* — account isolation does not constrain the party who
mints accounts. Where a threat arises from the product's own design or
verification rather than an adversary's action, the actor is `ACT-2` or `ACT-3`
and the party who would exploit it is named in the mitigation.

### 5.1 Threats by journey

| ID | Threat | Actor | Asset | Journey | Mitigation or risk |
|---|---|---|---|---|---|
| `THR-01` | The one-use token is read from shell history or a process listing | `ACT-9` | `AST-1` | 5.1 | Charter § 5.1 forbids supplying it in a form that exposes it; `--token-file`. Mode and lifetime: P03 |
| `THR-02` | A captured token is replayed to enroll an impostor agent | `ACT-6`, `ACT-9` | `AST-1` | 5.1 | One-use and token-scoped, with short bootstrap expiry as the backstop (`ARCH-NATS-004`) |
| `THR-51` | The token bundle is **altered** in transit and its service public halves replaced, so the agent trusts a false server for the life of its identity | `ACT-9`, `ACT-6` | `AST-1` | 5.1 | `RSK-14`. The bundle is the agent's only trust anchor and it cannot check what checking is done with (`ADR-0003` § 14). Distinct from `THR-02`: a replayed token is **detectable**, because the real host's enrollment then fails as spent; a substituted anchor leaves the host enrolled and looking healthy |
| `THR-52` | A caller with the operator credential names a privileged execution user for a command that does not need one | `ACT-9`, `ACT-8` | `AST-10`, `AST-12` | 5.3 | `RFC 0003` admits the choice and requires a **non-root deployment default**, so privilege is named per command rather than held permanently. Every choice is in the job's audit record (`ARCH-OBS-001`), which is what makes an unnecessary escalation visible. The control is disclosure and default, not prevention: a caller authorised to run commands at all is authorised to name a user |
| `THR-53` | A writable profile script on a target account influences commands run as that account | `ACT-9` | `AST-10` | 5.3 | The profile runs **as the target user**, exactly as that account's own login would, and the command then runs as the same user. Whoever can write that profile can already act as that account, so this changes no privilege. The harvest is bounded — its own timeout, its own output, a size bound, and fallback to the derived environment on any failure (`RFC 0003`) |
| `THR-54` | An operator is handed an opaque payload — `/bin/sh -c '<200 characters>'` is harder to review before running than a named command | `ACT-9` | `AST-10` | 5.3 | **Disclosure, not prevention.** The full argv reaches the audit record (`ARCH-OBS-001`, `ADR-0007` § 11), so what ran is attributable afterwards; the execution user is named and recorded (`RFC 0003`), bounding a misled operator to the account they chose; and it exceeds no authority the caller held — someone who can be misled into pasting a shell program can be misled into pasting a binary's arguments. **Not a residual risk**: no control in or out of scope removes it, and recording it as one would imply a future in which it closes |
| `THR-03` | Bootstrap access survives enrollment and is reused later | `ACT-7` | `AST-2` | 5.1 | Staged protocol revokes bootstrap access **and verifies the revocation** (`ARCH-NATS-004`) |
| `THR-04` | A crash between credential write and activation leaves the agent and server disagreeing about identity state | `ACT-7` | `AST-3` | 5.1 | fsync and atomic rename at mode `0600`; a defined crash-recovery path at every stage (`ARCH-NATS-004`) |
| `THR-05` | An agent enrolls under another agent's identity | `ACT-7` | `AST-3` | 5.1 | Token-specific subjects; every agent a distinct principal (`ARCH-NATS-002`, `ARCH-NATS-004`) |
| `THR-06` | An agent publishes presence for another agent | `ACT-7` | `AST-15` | 5.2 | Publish permitted only on the agent's own presence subject (`ARCH-NATS-003`) |
| `THR-07` | Presence is withheld so a live agent appears offline | `ACT-5`, `ACT-6` | `AST-15` | 5.2 | `RSK-2`. Presence is reported as observed, never inferred (charter § 5.2), so the operator sees absence rather than a false positive |
| `THR-50` | Presence is **fabricated** on the wire so a stopped agent appears live | `ACT-4`, `ACT-5` | `AST-15` | 5.2 | Publish permission is per-agent (`ARCH-NATS-003`), which constrains *agents* and not the broker or the party who mints identities. The answer is the envelope: presence is signed by the agent, and `AST-4` never leaves the host (`ADR-0003` § 4), so a minted NATS identity cannot produce it. The signature's shape is P05's. **This does not reach `ACT-2`**: a compromised server is the party that *verifies* presence and reports it, so it needs no forgery — that is `THR-46` and `RSK-4`, and a signature it checks itself constrains it not at all |
| `THR-08` | An agent subscribes to another agent's command subject | `ACT-7` | `AST-10` | 5.3 | Subscribe permitted only on the agent's own command and cancellation subjects (`ARCH-NATS-003`), proven by negative tests (`ARCH-TEST-002`) |
| `THR-09` | An agent publishes a command to another agent | `ACT-7` | `AST-10` | 5.3 | Agents hold no publish permission on command subjects (`ARCH-NATS-003`) |
| `THR-10` | argv is read off the wire | `ACT-5`, `ACT-6` | `AST-10` | 5.3 | Payload encrypted end-to-end to the target agent. TLS terminates at the broker and is **not** sufficient (`ARCH-NATS-006`) |
| `THR-11` | Commands agents accept are issued by a party that is not the server | `ACT-8` | `AST-6`, `AST-7` | 5.3 | **A party holding only the publisher credential cannot complete this.** It can publish, and cannot produce the envelope signature agents verify (`ARCH-NATS-006`). The realisable path is joint possession, which is by construction possession of `TD-SRV` — `ACT-2`'s `THR-44`. Recorded as `RSK-1`, renewed at P05 |
| `THR-12` | A captured command envelope is replayed to execute twice | `ACT-5`, `ACT-8` | `AST-10` | 5.3 | Durable agent ledger permits at most one automatic attempt; the ledger, not the broker deduplication window, is authoritative (`ARCH-JOB-003`) |
| `THR-13` | Broker redelivery repeats indefinitely, amplifying one command into many | `ACT-5` | `AST-10` | 5.3 | Finite `MaxDeliver` and explicit `BackOff`, with terminal delivery advisories (`ARCH-NATS-010`); redelivery never creates a second logical attempt |
| `THR-14` | Parallel consumption reorders or concurrently executes commands for one agent | `ACT-5` | `AST-10` | 5.3 | One exact agent `FilterSubject`, `MaxAckPending=1` (`ARCH-NATS-009`) |
| `THR-15` | Supplied argv escapes the bounded surface — shell metacharacters, a script, a pipeline | `ACT-9` | `AST-10` | 5.3 | **No shell interprets argv**; argv is a vector, not a string (charter § 6). `ADR-0007` § 1 carries argv as a vector to `execve` and § 4 resolves `argv[0]` against a **fixed** `PATH` rather than the harvested one, so a profile cannot decide which binary a name means. **Settled by `RFC 0004`**: the exclusions constrain what Keystone constructs, so an operator naming `/bin/sh` or a `#!` script as `argv[0]` is inside the boundary and has escaped nothing — they ran what they wrote. The word in this row is *escapes*, and escape is what a vector prevents. What the reading costs is `THR-54`. `RFC 0003` admits a shell solely to compute a target account's login environment under a fixed agent-authored command, and the command's argv is `exec`ed as a vector that no shell parses. The property this row rests on is the second clause, and it is unchanged. Execution policy is deny-by-default for unsupported forms (`ARCH-EXEC-001`) |
| `THR-16` | A command runs unbounded and exhausts the agent host | `ACT-9` | `AST-12` | 5.3 | Explicit duration, output, environment, working-directory, concurrency and resource limits (`ARCH-EXEC-001`), each with a value or a named owner in `ADR-0007` § 8 |
| `THR-17` | Execution begins before a durable receipt, so a crash hides whether it ran | `ACT-3` | `AST-12` | 5.3 | Command identifier, authenticated envelope metadata and receipt state persisted **before** the process starts (`ARCH-JOB-002`) |
| `THR-18` | The control plane reports success for a job it cannot prove ran | `ACT-2` | `AST-13` | 5.4 | `UNKNOWN` is reported as itself and never rendered as success or failure; a verified late result may supersede it, both retained (`ARCH-JOB-004`) |
| `THR-19` | A command is acknowledged before its result is durable, losing the result | `ACT-3` | `AST-11` | 5.5 | Acknowledgement only after receipt, terminal state and result are durable and publication has a broker ack (`ARCH-JOB-005`) |
| `THR-20` | An agent publishes another agent's result | `ACT-7` | `AST-11` | 5.5 | Publish permitted only on the agent's own result subject (`ARCH-NATS-003`) |
| `THR-21` | Result output is read off the wire | `ACT-5`, `ACT-6` | `AST-11` | 5.5 | Results signed by the agent and encrypted to the authorized result service (`ARCH-NATS-006`) |
| `THR-22` | Unbounded output exhausts agent or server memory | `ACT-9` | `AST-11` | 5.5 | Output limits with truncation reported explicitly rather than silently (`ARCH-EXEC-001`, `ADR-0007` § 9, which discards rather than killing, because exhaustion is what this threat is about and a kill would turn a reporting limit into an execution outcome); broker-side payload limits (`ARCH-NATS-007`) |
| `THR-23` | Cancellation kills the immediate child and leaves descendants running | `ACT-3` | `AST-12` | 5.6 | Timeout and cancellation terminate the complete process group or platform-equivalent job object (`ARCH-EXEC-002`) — `ADR-0007` § 10's `SIGTERM`, grace period and `SIGKILL`, each to the group. `ACT-9` supplies the work that spawns descendants; the defect is the agent's |
| `THR-24` | A party cancels a job it does not own | `ACT-7`, `ACT-8` | `AST-12` | 5.6 | Cancellation subjects are per-agent (`ARCH-NATS-003`), so `ACT-7` has no publish path to another agent's. `ACT-8` does hold one (§ 2), and the key-separation argument that bounds `THR-11` once went unstated for this envelope, because `ARCH-NATS-006` named commands and results and not cancellations — `RSK-11`, **resolved**: `ADR-0005` § 4 signs every envelope class and [RFC 0002](../rfcs/0002-signed-envelope-classes.md) makes the invariant require it, so the credential alone cannot produce an accepted cancellation. **Which operators may cancel which jobs is settled at P09**: `ADR-0009` § 4 gives every journey the same authority, so any authorized operator may cancel any job and per-job ownership does not exist in this release |
| `THR-25` | A crash while cancellation is in flight leaves the outcome indeterminate and it is reported as terminal | `ACT-3` | `AST-13` | 5.6 | Recovery path at the cancellation boundary; `UNKNOWN` where the outcome is unprovable (`ARCH-JOB-004`) |
| `THR-26` | A lifecycle transition emits no audit record, hiding an action | `ACT-2` | `AST-13` | 5.7 | Enrollment, authorization denial, receipt, start, cancellation, timeout, completion, unknown outcome and result retrieval all emit correlated records (`ARCH-OBS-001`) |
| `THR-27` | Audit records or broker headers carry command secrets or payload plaintext | `ACT-2` | `AST-13` | 5.7 | `ARCH-OBS-001` forbids it; the harness plants canaries and fails before upload (`TESTING.md`) |
| `THR-28` | A compromised server rewrites its own audit store | `ACT-2` | `AST-13` | 5.7 | `RSK-4` — no external attestation exists; see `THR-46` for the wider blast radius |

### 5.2 Threats from the invariant sweep

Threats the journey walk did not reach, found by walking all 24 invariants.

| ID | Threat | Actor | Asset | Journey | Mitigation or risk |
|---|---|---|---|---|---|
| `THR-29` | An agent or ordinary service identity reaches `$SYS`, JetStream management, or another deployment's subjects | `ACT-7`, `ACT-8` | `AST-9` | all | Only the enumerated data-plane `$JS.API`, acknowledgement and reply subjects its exact consumer needs (`ARCH-NATS-005`) |
| `THR-30` | Keystone traffic shares an account with unrelated workloads, so a co-tenant observes or publishes | `ACT-2` | `AST-10`, `AST-11` | all | A dedicated Keystone account, with broker administration and advisories in a separate system account (`ARCH-NATS-001`). The defect is in provisioning, so the actor is `ACT-2`; the party who would profit is an unrelated workload sharing the account, which holds no Keystone credential and is therefore not `ACT-8`. Account isolation constrains **that co-tenant, not `ACT-4`**, who mints accounts and is bounded only by `RSK-3` |
| `THR-31` | An unbounded stream or consumer exhausts broker storage and denies the fleet | `ACT-7`, `ACT-9` | `AST-9` | all | Explicit connection, subscription, payload, consumer, byte, age and message limits; unbounded streams prohibited (`ARCH-NATS-007`). That invariant bounds **resources** and does not by itself require per-agent partitioning. `ADR-0002` § 7 and § 9 supply it: agents hold no publish permission on the command stream, and the result stream carries one subject per agent under an explicit maximum-messages-per-subject, so one agent's stored messages are bounded independently — `RSK-10`, resolved |
| `THR-32` | The NATS operator mints a Keystone identity at will | `ACT-4` | `AST-9` | all | `RSK-3` — the broker's administrative authority exceeds the product's, by construction |
| `THR-33` | One key serves as transport identity and as envelope-signing or payload key | `ACT-2` | `AST-3`–`AST-5` | all | Separate keys required; NKey seeds must not be reused as Keystone signing or encryption keys (`ARCH-NATS-006`). The defect is in provisioning; `ACT-7` is who would profit from it |
| `THR-34` | An altered binary or package is installed | `ACT-10` | all | all | `RSK-5` — release signing is a C13 decision and does not exist in `v0.6.0` |
| `THR-35` | Commands are issued from a compromised operator workstation under the operator's authority | `ACT-9` | `AST-14` | 5.3 | `RSK-8`. A local socket prevents *remote* exposure and does nothing against code already on that host. **`ADR-0009` § 4 shares whatever authority the compromise holds**, as this row forecast: one group membership carries every journey. What § 3 and § 7 add is that the recorded actor is derived from the kernel, so the caller cannot attribute the action to anyone else |
| `THR-36` | The protocol claims exactly-once execution and the operator over-trusts a reported outcome | `ACT-2` | `AST-13` | all | Forbidden: at-least-once delivery is documented and exactly-once execution is never claimed (`ARCH-JOB-001`) |
| `THR-37` | The product introduces an inbound application listener on agents, creating a second transport outside the modelled boundaries | `ACT-2` | `AST-12` | all | Agents expose no inbound application listener; all application traffic crosses NATS (`ARCH-COMM-001`). This constrains **Keystone's design**. A root-compromised host can open a listener regardless — that is inside `ACT-7`'s total compromise and no invariant prevents it. **The operator socket is the one permitted local listener**, and `ADR-0009` § 11 states the property that keeps it from becoming a second transport, with a test: if removing the broker would leave an operator action still able to reach an agent, that action is a violation |
| `THR-38` | A feature silently assumes the server can dial the agent, so it fails or opens a path in isolated deployments | `ACT-2` | `AST-10` | all | Server and agents must work with no direct reachability; acceptance places them on isolated networks with only the broker shared (`ARCH-COMM-002`). **The operator API creates no exception** — it is bound to a filesystem path with no network address, and every operator action that reaches an agent publishes to NATS (`ADR-0009` § 11) |
| `THR-39` | A subject-permission boundary is asserted in design and never verified against the generated production JWTs, so it does not hold in deployment | `ACT-7`, `ACT-8` | `AST-3`, `AST-6` | all | Negative identity tests are mandatory and must exercise generated production JWTs, not a test-only authorization adapter (`ARCH-TEST-002`) |
| `THR-40` | An effect is claimed on the intended agent but never proven, so a feature acts on the wrong host or nowhere | `ACT-2` | `AST-10` | all | Every remote feature proves the effect occurred on the intended agent and not on the server or a non-target agent (`ARCH-TEST-001`) |
| `THR-41` | An invariant is documented and nothing enforces it, so it erodes without anyone noticing | `ACT-2` | all | all | Every invariant is enforced by an automated test, static rule, or a named release review with retained evidence (`ARCH-TEST-003`) |
| `THR-42` | Keystone reimplements a broker capability worse than the broker's, adding attack surface for no requirement | `ACT-2` | `AST-9` | all | Every messaging ADR records relevant NATS capabilities as `Adopt`, `Evaluate`, `Defer` or `Reject` with evidence, and does not duplicate native behaviour without a documented missing requirement (`ARCH-NATS-008`) |
| `THR-43` | A mitigation is verified only against mocks or an in-process agent, so it is unverified where it runs | `ACT-2` | all | all | Acceptance uses released binaries, production configuration parsing, production serialization and production subjects; an in-memory bus is never a substitute (`ARCH-COMM-003`) |

### 5.3 Blast radius of total domain compromise

Two domains hold enough material that their compromise ends this model's other
guarantees: `TD-SRV`, described in § 4 as the widest single failure, and
`TD-AGT`, which authors the results everything downstream believes. Both need
their consequences enumerated rather than asserted.

**`TD-SRV`.** It holds `AST-6` (the service NATS credentials), `AST-7` (the service
signing key), `AST-8` (the result-decryption key), `AST-13` (the job and
audit store) and `AST-16` (the Keystone account signing key). An attacker in possession of that domain therefore holds **both**
halves of `THR-11` at once, and the separation-of-keys argument that defends
against single theft does not apply.

| ID | Threat | Actor | Asset | Journey | Mitigation or risk |
|---|---|---|---|---|---|
| `THR-44` | A compromised server issues commands and cancellations every agent accepts, holding the publisher credential and the signing key together | `ACT-2` | `AST-6`, `AST-7` | 5.3 | `RSK-4`. No control in `v0.6.0` survives this; agents verify a signature the attacker can produce |
| `THR-45` | A compromised server decrypts every result on the deployment | `ACT-2` | `AST-8`, `AST-11` | 5.5 | `RSK-4`. Results are encrypted *to* the result service, so possessing it is possessing the plaintext |
| `THR-46` | A compromised server fabricates operator-facing status, output and audit — a success the operator cannot distinguish from a real one | `ACT-2` | `AST-13` | 5.4, 5.5, 5.7 | `RSK-4`. The agent's own ledger is an independent record, and **since P08 the operator reaches it without the server in the path**: `ADR-0008` § 3 publishes its schema and location, which is `RSK-4`'s compensating control. What the ledger says is still only as true as the agent (`THR-48`) |
| `THR-49` | A compromised server mints agent identities at will inside the Keystone account, without stealing any existing credential | `ACT-2` | `AST-16` | 5.1 | `RSK-4`. Holding the account signing key is what lets enrollment issue an identity at all (`ADR-0002` § 2); the same key mints one the operator never authorised. Account isolation (`ARCH-NATS-001`) bounds it to the Keystone account, and the operator seed is held outside every Keystone process, so the server cannot create accounts |

**`TD-AGT`.** `ACT-7` holds `AST-4` (the agent's envelope-signing key), `AST-11`
(the result plaintext, before it is encrypted to the result service) and
`AST-12` (the ledger) at once. A result is therefore **authored inside the
domain it reports on**, and its signature proves origin rather than truth — the
server verifies a key the attacker legitimately holds. Unlike `TD-SRV` this is
bounded to one host, because § 2 makes each agent host its own domain; within
that host it is total.

| ID | Threat | Actor | Asset | Journey | Mitigation or risk |
|---|---|---|---|---|---|
| `THR-47` | A compromised agent signs a fabricated successful result for a job it never executed | `ACT-7` | `AST-4`, `AST-11` | 5.4, 5.5 | `RSK-9`. The signature verifies because the key is the agent's own. `ARCH-JOB-004` separates *unprovable* from *proven*; it cannot separate *truthful* from *false* |
| `THR-48` | A compromised agent falsifies or destroys its own ledger, so the independent record `RSK-4` relies on is untrue or absent | `ACT-7` | `AST-12` | 5.7 | `RSK-9`. `ARCH-JOB-002` requires the ledger to be durable, which defends against a crash and not against the host that owns it |

Where one attacker holds both domains, nothing in this model applies; that is
not a third case, it is the union of `RSK-4` and `RSK-9`.

### 5.4 Invariant coverage

All 24 invariants, each mapped to at least one modelled threat. **No invariant
carries a security-`N/A` waiver.**

| Invariant | Threats |
|---|---|
| `ARCH-COMM-001` | `THR-35`, `THR-37` |
| `ARCH-COMM-002` | `THR-38` |
| `ARCH-COMM-003` | `THR-43` |
| `ARCH-NATS-001` | `THR-30` |
| `ARCH-NATS-002` | `THR-05` |
| `ARCH-NATS-003` | `THR-06`, `THR-08`, `THR-09`, `THR-20`, `THR-24` |
| `ARCH-NATS-004` | `THR-02`, `THR-03`, `THR-04`, `THR-05` |
| `ARCH-NATS-005` | `THR-29` |
| `ARCH-NATS-006` | `THR-10`, `THR-21`, `THR-24`, `THR-33`, `THR-50` |
| `ARCH-NATS-007` | `THR-22`, `THR-31` |
| `ARCH-NATS-008` | `THR-42` |
| `ARCH-NATS-009` | `THR-14` |
| `ARCH-NATS-010` | `THR-13` |
| `ARCH-JOB-001` | `THR-36` |
| `ARCH-JOB-002` | `THR-17` |
| `ARCH-JOB-003` | `THR-12` |
| `ARCH-JOB-004` | `THR-18`, `THR-25` |
| `ARCH-JOB-005` | `THR-19` |
| `ARCH-EXEC-001` | `THR-15`, `THR-16`, `THR-22`, `THR-53` |
| `ARCH-EXEC-002` | `THR-23` |
| `ARCH-OBS-001` | `THR-26`, `THR-27`, `THR-52`, `THR-54` |
| `ARCH-TEST-001` | `THR-40` |
| `ARCH-TEST-002` | `THR-08`, `THR-39` |
| `ARCH-TEST-003` | `THR-41` |

An earlier plan for this task expected five invariants to need a
security-`N/A` waiver — the three `ARCH-TEST-*`, `ARCH-COMM-003` and
`ARCH-NATS-008` — on the grounds that they constrain evidence and process
rather than the running system. That was wrong. Each names a way the system
becomes insecure *without anyone observing it*: an untested permission boundary
that does not hold, an effect claimed and never proven, an invariant nothing
enforces, a mitigation verified only against a mock, a native mechanism
reimplemented worse. Those are threats, and `THR-39` to `THR-43` model them.
Waiving them would have been the cheaper answer, not the correct one.

## 6. Metadata leakage

What an **authorized broker observer** — `ACT-5`, holding no Keystone key —
learns without decrypting a single payload. Encryption protects content
(`ARCH-NATS-006`); it does not protect the shape of traffic.

| Observable | What it reveals |
|---|---|
| Subject names | Which agents exist, which are being commanded, and which operation each message is — command, cancellation, result, presence |
| Publication timing | When each command was issued and when its result returned; the difference is the job's duration |
| Message sizes | Roughly how long an argv vector is and how much output a command produced |
| Presence cadence | Which agents are live, and when one stops |
| Delivery and acknowledgement counts | Which commands were redelivered, and which are being retried |
| Correlation of the above | A usable operational picture of the fleet: who is working on what, when, and for how long |

**None of this is mitigated in `v0.6.0`**, and the layered design does not
claim otherwise. Subject-level obfuscation, payload padding and decoy traffic
are all possible and all cost something; none is required by an invariant and
none is designed. Recorded as `RSK-6`, **renewed and widened at P05**.
`ADR-0005` §§ 7 and 8 enumerate the accepted leakage per message class and fix
the header set, which states this table's contents per class rather than
reducing them — and add one thing this table does not list: a **cleartext job
identifier**, which makes the correlation conceded in the last row *exact*
rather than inferred, and a lifecycle event's cleartext job linkage. **This
table should gain a row for those**, which is a task of its own; P05's boundary
permits this sentence and `RSK-6`'s row and not the table above.

The honest summary for an operator: Keystone hides *what* a command says from
the broker, and does not hide *that* you ran one.

**Signing does not narrow this.** `THR-50` is answered by signing presence, which
makes a forged presence message detectable and leaves every observable in the
table above exactly as it was. Authenticity and confidentiality are independent
layers (`ARCH-NATS-006`), and only the second would narrow § 6.

## 7. Denial and delay powers

| Actor | Can |
|---|---|
| `ACT-5` broker administrator | Drop, delay or reorder any message; withhold presence; stall acknowledgements; stop the deployment entirely |
| `ACT-4` NATS operator | Revoke any identity, including every agent's, and deny the whole deployment |
| `ACT-6` network attacker | Partition a domain from the broker, indefinitely |
| `ACT-7` compromised agent | Consume its own limits and fill its own subject — bounded by `ARCH-NATS-007`, and **bounded to that agent** since `ADR-0002` § 7 and § 9: it holds no publish permission on the command stream, and its result subject is capped by a per-subject message limit (`RSK-10`, resolved) |
| `ACT-9` malicious command author | Occupy an agent with long-running work — bounded by `ARCH-EXEC-001` |
| `ACT-2` server | Withhold commands, results or audit from the operator; cancel any running job |

**The design's answer to all of these is the same, and it is not prevention.**
None of the powers *in the table above* can produce a false success: where the
control plane cannot prove whether a command ran, `ARCH-JOB-004` requires it to
report `UNKNOWN`, and the charter gives that its own exit code so an operator
can branch on it. Denial is converted into honest ambiguity rather than a wrong
answer.

**That guarantee does not extend to either domain that authors the record.**
The `ACT-2` and `ACT-7` rows above are their *denial* powers — withholding, and
consuming limits. Compromise is a different thing. A compromised `TD-SRV` also
holds the signing key, the result-decryption key and the audit store, and can
fabricate a success the operator cannot distinguish from a real one (`THR-46`).
A compromised `TD-AGT` signs results with its own key and owns its own ledger,
and can report a success for a command it never ran (`THR-47`).

So the ambiguity guarantee covers *denial and delay*: from the broker, from the
network, and from a compromised agent's denial powers. It does **not** cover
fabrication by the server or by the agent, and § 5.3 says so rather than leaving
the reader to infer it from a table of denial powers.

That is a deliberate trade. Generation 2 has no availability commitment, no
clustering and no HA (`ROADMAP.md`), so an operator who needs the fleet
reachable during a broker outage does not have that from this product.

## 8. Key rotation, revocation and recovery

| Material | Rotation | Revocation | Recovery |
|---|---|---|---|
| `AST-2` bootstrap credential | Not rotated; it exists for one enrollment | Revoked at enrollment and the revocation **verified**; short expiry is the backstop (`ARCH-NATS-004`) | Defined crash-recovery path at every enrollment stage |
| `AST-3` permanent agent credential | **Re-enrollment** (`ADR-0003` § 10, § 11) — `RSK-7`, resolved | Removing the agent's NATS identity; the effect on in-flight jobs is P03 and P06 | Re-enrollment |
| `AST-4`, `AST-5` agent signing and decryption keys | **Re-enrollment** (`ADR-0003` § 11) — `RSK-7`, resolved. They are generated on the agent and never escrowed, so replacing them and replacing the identity are the same operation | With the agent identity | Re-enrollment |
| `AST-6` service NATS credentials | **No procedure specified** — `RSK-13`, **re-gated to C13 at P10** | Broker-side, by the NATS operator | Reprovisioning, outside the product |
| `AST-7` service envelope-signing key | **Not specified** — `RSK-12`, renewed at P05 and **carrying no task gate after P10** | **No mechanism exists.** Revoking `AST-6` removes the publish path, which *contains* the key exactly as `RSK-1` argues, and does not invalidate it: agents accept its signature over any publish path its holder can reach | Reprovisioning, and re-establishing trust in the new key on every agent — outside the product |
| `AST-8` result-decryption key | **Not specified** — `RSK-12`, renewed at P05 and **carrying no task gate after P10** | **No mechanism exists, and broker-side revocation does not help**: captured ciphertext stays decryptable offline for as long as the key exists | Reprovisioning, outside the product |
| `AST-16` Keystone account signing key | **No procedure specified** — `RSK-13`, **re-gated to C13 at P10**. Rotating it re-issues every identity it signed | Broker-side: the NATS operator can revoke the account's signing key | Reprovisioning, and re-issuing every identity it signed — outside the product |
| `AST-9` NATS operator and account seeds | The broker's own process | The broker's own process | Outside the product entirely |

Enrollment is the only key transition Generation 2 specifies, and it is
specified thoroughly — staged, idempotent, fsync-and-rename, proof of permanent
connection, revoke, verify, with a crash-recovery path per stage. **Everything
after enrollment was undesigned**, which was `RSK-7` and the largest single gap
this model found. `ADR-0003` closes the agent-side part of it: re-enrollment is
a designed rotation mechanism for `AST-3`, `AST-4` and `AST-5`. What remains is
split by owner — `RSK-12` for the service protocol keys, renewed at P05 with a recorded candidate mitigation, `RSK-13` for the
service NATS credentials and the account signing key.

The three service rows were one row until review pointed out that "broker-side,
by the NATS operator" is true of `AST-6` alone. The broker can revoke a NATS
identity; it has no authority over a Keystone signing or decryption key, and the
two fail differently — containment for one, nothing at all for the other. A
grouped row claimed a control that constrains one of the three assets.

## 9. Accepted residual risk

Each carries owner, rationale, compensating control and expiry — the four
elements `TESTING.md` § Gate schedule requires. Each expires on the stated date
**or when the named task is accepted, whichever is sooner**, at which point it
is resolved or renewed explicitly.

| ID | Risk | Owner | Rationale | Compensating control | Expires |
|---|---|---|---|---|---|
| `RSK-1` | **Joint** possession of the publisher credential and the service signing key yields commands every agent accepts (`THR-11`) | Project maintainer | Each key alone is useless for this: one publishes without signing, the other signs without publishing. Two separate thefts, or one compromise of the domain holding both — which is `RSK-4` | `ARCH-NATS-006` keeps them separate keys, so no single theft suffices; every issued command is audited (`ARCH-OBS-001`), though a party holding both can also rewrite that audit. Note the containment is **unbounded in time**: no revocation path exists for `AST-7` (§ 8, `RSK-12`), so a theft that is contained is not thereby ended. signing every class does not change what joint possession of the publisher credential and `AST-7` can do (`ADR-0005` § 4). **Renewed at P08.** `ADR-0008` § 6 makes the audit append-oriented, which makes an accidental overwrite a bug rather than routine — and **append-only is a discipline, not evidence**: anything holding the server's account can still rewrite the record. Tamper-evidence is deferred with a trigger (`ADR-0008` § 13)  **Renewed at P10, and re-gated.** `ADR-0010` § 6's non-target probe makes the separation checkable and § 9's canary scan is what would catch a key in an artifact — both are specifications, not evidence. **P10 designs the harness and runs nothing**, so the gate moves to the task that can attempt it | **Renewed at P05, renewed at P08, renewed and re-gated at P10.** 2027-09-14, or C14 |
| `RSK-2` | The broker administrator can withhold or delay any message (`THR-07`) | Project maintainer | Keystone does not own the broker; a mediated design cannot exclude its operator | Withholding produces `UNKNOWN` or visible absence, never false success (`ARCH-JOB-004`) | **Renewed at P02** (`ADR-0002` § 14): the account topology does not change what the party running the broker can drop or delay. 2027-09-13, or C14 |
| `RSK-3` | The NATS operator can mint any Keystone identity (`THR-32`) | Project maintainer | Whoever holds the operator seed administers the trust domain; that authority is above the product by construction | Account isolation limits blast radius to the Keystone account (`ARCH-NATS-001`); envelope signatures are not broker-issued, so a minted identity still cannot forge a signed command. **Narrowed at P02** (`ADR-0002` § 2): the operator seed is held outside every Keystone process, so this is the broker administrator's authority and not the server's — the server holds an account signing key and can mint identities only inside the Keystone account | **Renewed at P02.** 2027-09-13, or C14 |
| `RSK-4` | **Compromise of the server is total within the product**: issuing commands agents accept, decrypting every result, fabricating operator-facing status and audit, and minting agent identities inside its own account (`THR-28`, `THR-44`–`THR-46`, `THR-49`) | Project maintainer | `TD-SRV` holds the service credentials, the signing key, the result-decryption key, the store and the account signing key. No control in `v0.6.0` survives possession of that domain, and none is proposed — the layered design defends the *broker*, the *network* and a *compromised agent*, not the control plane against itself | Agent-side ledgers are independent and record receipt and terminal state (`ARCH-JOB-002`), so a forensic reconstruction is possible from the agents even when the server's account is false — and **since P08 the operator no longer reaches them only through the server**: `ADR-0008` § 3 publishes the agent ledger's schema and path, so an operator investigating a false server reads the agents directly, with the compromised party out of the path. The reconstruction still holds only where the agent is not also compromised (`RSK-9`, `THR-48`), and a server that lies to an operator who never thinks to look is unaffected  **Renewed at P10, and re-gated.** `ADR-0008` sent this here as the first task that could demonstrate the reconstruction; it is not, and `G22` corrected that claim. `ADR-0010` § 1 places the agent-ledger cases on the VM side and § 5 gives C14 the faults to cause; **the reconstruction is attempted there** | **Renewed and strengthened at P08, re-gated at P10.** 2027-03-13, or C14 |
| `RSK-5` | Release artefacts are unsigned (`THR-34`) | Project maintainer | Signing is a C13 decision requiring a human ceremony; it does not exist before then | None within the product | 2027-03-13, or C13 |
| `RSK-6` | Traffic metadata reveals an operational picture of the fleet to the broker (§ 6) | Project maintainer | Inherent to broker mediation; mitigation costs bandwidth and latency and is required by no invariant | Payload content remains protected (`ARCH-NATS-006`). **Widened at P05.** `ADR-0005` §§ 7 and 8 fix the header set and enumerate the accepted leakage per message class, so what the broker learns is now *stated* rather than inferred — but stating a leak is not reducing one, and signing does not narrow metadata leakage at all. What changed is that there is more of it: `ADR-0005` § 1 carries the job identifier in cleartext, which makes § 6's conceded correlation exact rather than inferred, and a lifecycle event reveals the job it concerns and a coarse transition, which § 6's table does not list. The correlation identifier is **not** widened — it travels inside the payload on every encrypted class | **Renewed and widened at P05.** 2027-09-14, or C14 |
| `RSK-7` | **Resolved at P03.** Rotation of agent-side material — `AST-3`, `AST-4`, `AST-5` — was undesigned (§ 8) | Project maintainer | Generation 2's scope is one narrow promise and enrollment was the only transition designed. P03 was the task that decided whether a second one exists | `ADR-0003` § 10 and § 11: re-enrollment is the mechanism, and it is designed rather than improvised. The keys are generated on the agent and never escrowed, so replacing them and replacing the identity are one operation. The cost is stated — a rotated agent is a new agent, with a new identifier and no history | **Resolved 2026-09-13** by `ADR-0003`; no longer accepted |
| `RSK-8` | A compromised operator workstation acts with the operator's full authority (`THR-35`) | Project maintainer | The operator API is a local socket, which defends against remote callers and not against code already running as the operator. **P09 defined which operators may do what and the answer is all of it** (`ADR-0009` § 4): one group membership carries every journey, and after RFC 0003 that includes running argv as root on every enrolled host. No design here can distinguish the operator from malware holding their session — that needs attestation or a second factor, and neither is in Generation 2's scope | Every command issued is audited with its actor (`ARCH-OBS-001`) — which records **the identity exercised, not the true actor**: malware acting in the operator's session is attributed to the operator. The action is visible afterwards; who took it is not. **Strengthened at P09.** The actor is derived from `SO_PEERCRED` and no request field may set it (`ADR-0009` §§ 3, 7), so a caller **cannot forge an attribution to another operator** — which this control previously did not say and nothing guaranteed. This is **disclosure, not prevention**: the record became harder to lie to and the risk did not get smaller | **Renewed and strengthened at P09.** 2027-03-13, or C04 |
| `RSK-9` | **Compromise of an agent host is total within that host**: it signs results with its own key and owns its own ledger, so it can report a success for a command it never ran (`THR-47`, `THR-48`) | Project maintainer | A result is authored inside the domain it reports on. The signature proves origin, not truth, and no control in `v0.6.0` gives the server an independent witness of what happened on the host. Attestation would; it is not part of Generation 2's one narrow promise | Bounded to one host by construction — § 2 makes each agent host its own domain — so fabrication is confined to jobs targeted at that agent. **Unchanged at P06, and P06 is why it is worth saying precisely.** `ADR-0006` § 3 makes receipt and start separate durable writes and its § 7 makes the ledger the authority on whether a command ran — both strengthen what a *correct* agent proves, and both are mechanisms a compromised host owns. The control plane gained precision about this risk and no leverage over it. **Unchanged again at P08**: `ADR-0008` § 3's independently readable ledger reads whatever a compromised host left, and § 5's separate transactions and § 7's tombstones strengthen only what a correct agent proves. **Narrowed in one dimension at P07.** `ADR-0007` § 7 splits the agent: the NATS connection, the key material and all protocol parsing run as a dedicated unprivileged account, and only a small executor holds root. A memory-safety defect in the component that parses untrusted network input no longer yields the host. **The other dimension is unchanged** — an attacker controlling the agent *logically* can ask the executor to run a command as root — and closing that needs the executor to verify the service signature itself, which `ADR-0005`'s key placement currently prevents (`ADR-0007` § 13). The server's audit stays *consistent*, recording a result that arrived and verified, which is true; what the result says is not  **Renewed at P10, and re-gated.** `ADR-0010` § 1 states that the host boundary is a VM case, because a container's approximation of users and host security policy is what `TESTING.md` refuses. P10 says where it would be tested and tests nothing | **Renewed at P06, narrowed at P07, renewed at P08, re-gated at P10.** 2027-09-14, or C14 |
| `RSK-10` | **Resolved at P02.** One agent's publications were not proven to be bounded away from the fleet's shared limits (`THR-31`, § 7) | Project maintainer | `ARCH-NATS-007` requires bounded resources and does not by itself require per-agent partitioning. P02 was the task that decided the topology | `ADR-0002` § 7 and § 9: agents hold no publish permission on the command stream, so the command path is not floodable by an agent; on the result stream one subject per agent plus an explicit **maximum messages per subject** bounds each agent's stored messages independently on shared storage | **Resolved 2026-09-13** by `ADR-0002`; no longer accepted |
| `RSK-11` | **Resolved at P05.** `ARCH-NATS-006` named commands and results and not cancellations, so a holder of the command publisher credential alone might have produced an accepted cancellation (`THR-24`) | Project maintainer | The invariant was written around the two envelopes the 5.3 and 5.5 journeys carry. P05 was the task that decided what every other envelope requires | `ADR-0005` § 4 signs **every** envelope class, cancellation included, so the credential alone is not sufficient. **Both the exposure and the invariant gap are closed**: [RFC 0002](../rfcs/0002-signed-envelope-classes.md) amends `ARCH-NATS-006` to name every envelope class, so a later ADR cannot drop the signature without amending an invariant | **Resolved 2026-09-14** by `ADR-0005`; no longer accepted |
| `RSK-12` | The service envelope-signing key and the result-decryption key have **neither rotation nor revocation** (`AST-7`, `AST-8`, § 8) | Project maintainer | The broker can revoke a NATS identity; it has no authority over a Keystone protocol key. Revoking `AST-6` contains a stolen `AST-7` by removing the publish path without invalidating the key, and does nothing at all for `AST-8`, whose holder decrypts captured ciphertext offline | Key separation means no single theft suffices to issue an accepted command (`RSK-1`); containment via `AST-6` revocation is available and is **unbounded in time**, which is the point `RSK-1` records. **Renewed at P05**, which designs the envelope and not key custody. The candidate mitigation is recorded rather than left to be rediscovered: a certificate chain rooted **outside** the deployment (`CAP-IDENT-004`, `CAP-IDENT-021` — Generation 1 capabilities catalogued as Future) would permit leaf rotation, and reaching it is a phase-gate promotion. A *server-held* root is rejected: it would let a compromised server certify itself as any agent and forge that agent's signatures, destroying what `ADR-0003` § 4 creates and widening `RSK-4`  **Renewed at P10, and the task gate is removed.** The candidate mitigation is a certificate chain rooted outside the deployment, which this row already calls a phase-gate promotion — **no Generation 2 task resolves it**. A task gate would expire at a task that renews it unchanged, which is what happened at P10. The cost of a date alone is accepted and stated: it surfaces at review with no task forcing the question | **Renewed at P05, renewed at P10 with no task gate.** **Narrowed at `G37`, and only cryptographically.** `ADR-0005` §§ 4 and 5 make `AST-7` hybrid Ed25519 ‖ ML-DSA-65 and `AST-8` hybrid X25519 ‖ ML-KEM-768, so forging a service signature or decrypting captured ciphertext requires breaking **both** members of a pair rather than one — which closes the quantum path to this row's *"captured ciphertext stays decryptable offline for as long as the key exists"*. **The rotation gap is untouched.** No mechanism was created; what changed is that the absence of one is no longer compounded by a primitive a future adversary is expected to break. The date stands. 2027-09-14 |
| `RSK-13` | No **procedure** exists for rotating the service NATS credentials or the Keystone account signing key (`AST-6`, `AST-16`, § 8) | Project maintainer | Broker-side revocation exists for both, so this is an operational gap rather than a cryptographic one. Rotating `AST-16` re-issues every identity it signed, which is a fleet-wide re-enrollment — a runbook, not a protocol decision | `ADR-0003` § 10 supplies the per-agent mechanism a fleet rotation would repeat, and `ADR-0003` § 13 states the cost plainly. Account isolation bounds the blast radius of a compromised `AST-16` to the Keystone account (`ARCH-NATS-001`, `THR-49`)  **Renewed and re-gated at P10.** It was carried here by `ADR-0003` § 12 as *an operational runbook* — **on the belief that P10 was a deployment and operations task, which it is not and which `G22` corrected in six documents**. C13 builds packages, service definitions and least-privilege users, so a fleet rotation procedure is its work | **Renewed and re-gated at P10.** 2027-09-13, or C13 |
| `RSK-14` | A tampered token bundle substitutes the service public halves, and the agent trusts a false server (`THR-51`) | Project maintainer | The agent cannot verify the halves: they are what verification is performed *with*. Any delivery channel has this problem, and the bundle is the only out-of-band one enrollment has | The bundle is handled as a credential — mode `0600` enforced, refused if group-readable, removed once the identity is active (`ADR-0003` § 2) — and carried out of band by an operator rather than over the broker. **The real mitigation is channel separation**: a service public half delivered with the agent's installation, so an attacker must compromise both paths. That is a **deployment mode and not an option**, because an agent that merely prefers a pre-provisioned anchor falls back when the file is absent, and whoever can alter a bundle can delete a file. `CAP-IDENT-021` is the catalogued capability, `ADR-0002` § 14 is where the mode would live, and P10 owns its procedure  **Renewed and re-gated at P10**, for the same reason as `RSK-13`: `ADR-0005` assigned the channel-separated deployment mode to P10 twice, and P10 does no deployment. **The anchor is delivered with the installation, which is C13's.** `ADR-0010` § 8 can test a pre-provisioned anchor once C13 delivers one | **Renewed and re-gated at P10.** 2027-09-14, or C13 |

## 10. What this model cannot tell you

It is a model. It is wrong in ways its author cannot see, and the following are
the known shapes of that.

**It names controls; it does not test them.** Every mitigation in § 5 cites an
invariant that *requires* a control. Whether the control, once designed by
P02–P09 and built in Stage C, actually stops the threat is established by C14's
adversarial suite, not here.

**The invariant coverage table (§ 5.4) proves every invariant is addressed, not
that it is addressed adequately.** A shallow threat satisfies the table exactly
as well as a deep one. The table is a completeness check, and completeness is
not sufficiency.

**Whether these are the right ten actors is unverified.** They are the ten the
workstream named. An eleventh — a compromised build host distinct from
`ACT-10`, or a second operator with different authority — would produce threats
this document does not contain, and nothing here would notice the omission. One
candidate is named and deliberately not promoted: the unrelated workload sharing
a broker account appears in `THR-30`'s mitigation rather than as an actor,
because it holds no Keystone capability. If that judgement is wrong, the threats
it would have generated are missing.

**Whether an actor's stated capabilities are realistic is a judgement.** They
are drawn from what the invariants imply each principal can do. If an
implementation grants more than the invariant requires, the capability tables
become wrong and the threats derived from them become incomplete.

**The metadata analysis in § 6 is bounded by what its author thought to
observe.** It is not a formal traffic analysis.

**Residual risks record a decision, not a measurement.** `RSK-1` through
`RSK-14` say the maintainer accepted a risk and why. None says the risk is
small.

The acceptance cases for this document check its **shape** — that every actor
has capabilities, every threat has a mitigation or a risk, every invariant is
covered, every risk has four elements. None of them can check whether a
mitigation mitigates. That is the reviewer's job, and it is named in the
dossier rather than left implicit.
