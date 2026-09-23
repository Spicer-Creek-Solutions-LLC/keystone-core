# Glossary

Terminology for Generation 2, normative where it restates an accepted decision.

**This glossary defines only what is already decided** — by
[RFC 0001](../rfcs/0001-generation-2-reboot.md), the accepted
[product charter](PRODUCT-CHARTER.md), and
[`ARCHITECTURE-INVARIANTS.md`](ARCHITECTURE-INVARIANTS.md). A term whose meaning
a pending ADR owns is listed in § "Not yet defined" with the task that owns it,
rather than given a definition here that the ADR would then have to contradict.

It is therefore deliberately incomplete. That is the honest state of a project
whose architecture decisions are in progress.

Generation 1 terminology was removed rather than corrected; see § "Removed
terminology".

## Security and identity

### Attestation

Evidence that an agent is the identity it claims to be. What evidence
Generation 2 accepts, and how it is verified, is P03's to decide.

### Authentication

Verifying the identity of an operator, agent, or service.

### Authorization

Determining whether an authenticated principal may perform a specific action.
Generation 2 has two independent authorization layers: NATS subject permissions
enforced by the broker, and Keystone's own checks. Neither substitutes for the
other.

### CA (Certificate Authority)

An entity that issues digital certificates.

### Cipher Suite

A combination of cryptographic algorithms used to secure a network connection —
key exchange, authentication, encryption, and message authentication.

### Credential

Secret material used to authenticate. Distinct from key material used to sign
or encrypt, which is not a credential merely by being secret: `ARCH-NATS-006`
requires NATS transport identity, envelope signing and payload encryption to use
**separate keys**, and forbids reusing an NKey seed as a Keystone signing or
encryption key.

### Defense in Depth

Layering independent controls so that the failure of one does not compromise the
system. Generation 2's layers are enumerated in RFC 0001 § NATS-first
architecture.

### Encryption at Rest

Protection of stored data through cryptographic encryption.

### Encryption in Transit

Protection of data moving across a network. **Not equivalent to end-to-end
payload encryption**: TLS terminates at the broker, which is why Keystone
encrypts payloads to the intended recipient separately (`ARCH-NATS-006`).

### Fail Secure (Fail Closed)

A design principle where failure results in denial rather than access.
Generation 2 execution policy is deny-by-default (`ARCH-EXEC-001`).

### HMAC (Hash-based Message Authentication Code)

A construction for verifying data integrity and authenticity using a secret key
and a hash function.

### Key Derivation Function (KDF)

A function that derives keys from a secret value. Argon2, scrypt, PBKDF2.

### Least Privilege

Granting only the minimum permissions necessary. `ARCH-NATS-003` applies this to
NATS subjects: an agent may subscribe only to its own command and cancellation
subjects and publish only to its own result, event, and presence subjects.

### mTLS (Mutual TLS)

TLS in which both parties present certificates.

### One-use token

The enrollment credential. Requested by the operator through
`keystone enroll create`, valid for a single enrollment, short-lived by design,
and never a substitute for the permanent identity it bootstraps. The charter
forbids supplying it to the agent in a form that exposes it in process listings
or shell history. Who issues and validates it, and its exact lifetime, are
P03's.

### TLS (Transport Layer Security)

A protocol providing privacy and integrity between communicating applications.

### Token

An artifact representing identity or authorization claims.

### Trust Boundary

A point where the level of trust changes and controls are therefore required.
Enumerating these for Generation 2 is P01's job.

### Trust Domain

An administrative boundary within which identities are issued and validated.
Naming Generation 2's trust domains is P01's job.

### X.509

The standard format for public key certificates.

### Zero Trust

A model requiring verification of every access request regardless of network
location or prior authentication.

## Messaging and transport

### NATS

The messaging system Generation 2 uses as its only agent transport
(`ARCH-COMM-001`). NATS is treated as infrastructure rather than as a byte pipe:
before Keystone builds a messaging, authorization, durability or flow-control
mechanism, the native capability must be evaluated and recorded as `Adopt`,
`Evaluate`, `Defer` or `Reject` (`ARCH-NATS-008`).

### JetStream

NATS's persistence layer, providing streams, durable consumers and explicit
acknowledgements. JetStream delivery is **at-least-once**. Keystone does not
represent it as exactly-once execution (`ARCH-JOB-001`).

### Account

A NATS isolation unit. Keystone traffic runs in a dedicated account, with a
separate system account for broker administration and advisories
(`ARCH-NATS-001`). The account topology itself is P02's.

### Subject

The address a NATS message is published to and subscribed on. The Keystone
subject grammar and its per-principal permission matrix are P04's.

### Advisory

A NATS-generated notification about broker or stream events, such as terminal
delivery failure (`ARCH-NATS-010`).

## Execution and lifecycle

### Operator

The person who runs `keystone` against a fleet. The charter's target operator
is accountable for keeping Linux hosts running after deployment tooling has
finished with them.

### Agent

The Keystone process running on a managed host. It exposes no inbound
application listener, and all of its **application** communication with the
server — enrollment, commands, cancellation, results, events, presence —
crosses NATS (`ARCH-COMM-001`). The invariant governs that traffic, not every
packet the host sends.

### Server

The Keystone process that issues commands, consumes results, and holds the
durable record. It is not assumed to be able to reach agents directly
(`ARCH-COMM-002`).

### Control plane

The server, the operator CLI, and the broker configuration together — the parts
that decide and record, as distinct from the agents that execute.

### Enrollment

The staged protocol by which a host becomes an agent with a permanent scoped
identity: issue pending credentials, fsync and atomically rename the credential
file at mode `0600`, prove a permanent connection, mark the identity active and
the token spent, and confirm that to the agent. Bootstrap access then ends at
the credential's expiry (`ARCH-NATS-004`, as RFC 0005 amended it).

### Command

What the operator asks an agent to run: an argv vector, the account it runs as,
and its bounds. **Keystone interposes no shell and builds no pipeline**; every
element of the vector was written by the operator (charter § 6,
[RFC 0004](../rfcs/0004-what-the-execution-exclusions-constrain.md)). A shell may
run to compute that account's login environment, which is a separate operation
that reaches no operator input
([RFC 0003](../rfcs/0003-caller-selected-execution-user.md)).

### Job

One command's lifecycle on one agent, identified by a job identifier and
durably recorded before execution begins (`ARCH-JOB-002`).

### Result

The durable record of a job's outcome — the remote exit status and captured
output, signed by the agent and encrypted to the authorized result service
(`ARCH-NATS-006`).

### `UNKNOWN`

The knowledge state the control plane reports when it cannot prove whether a
command ran. It is reported as itself, never rendered as success or failure, and
never resolved by silently retrying an arbitrary command. A verified late result
may supersede it; both observations stay in the audit log (`ARCH-JOB-004`).

### Presence

Whether an agent is currently observed to be connected. Presence is reported as
observed and never inferred from the fact that an agent enrolled.

### Cancellation

Operator-initiated termination of a job. **A request, not a guarantee.**

When the cancellation **reaches the agent before the command completes**, the job
never runs or has its complete process group — or platform-equivalent job object
— terminated, not only its immediate child (`ARCH-EXEC-002`). It reaches a
terminal cancelled state and the charter gives exit `14`.

When it does not, **the command may complete anyway**: a cancellation for an
agent that is not connected is not retained by the live path, and the durable
copy is ordered behind the command it cancels. The job's terminal state is then
whatever the agent proves, and `keystone job cancel` exits with that state's own
code rather than `14`. Both observations stay visible.

The distinction has two names in `ADR-0006` § 1 — `Cancelling` for the accepted
request, `Cancelled` for the proven outcome — and the charter's § 5.6 states the
observable effect conditionally for the same reason.

### Process tree

A process and all its descendants. The unit that cancellation and timeout must
terminate — killing only the immediate child is the defect `ARCH-EXEC-002`
exists to prevent.

### Argv-only execution boundary

The frozen limit on Generation 2's first release execution surface. **It
constrains what Keystone constructs, not which programs an operator may name**
([RFC 0004](../rfcs/0004-what-the-execution-exclusions-constrain.md)): Keystone
interposes no shell, accepts and writes no script, and provides no stdin
streaming, pipelines, caller-provided environment, arbitrary working directory,
batch fan-out or interactive session. Widening it requires an RFC amendment, not
an ADR (charter § 6).

[RFC 0003](../rfcs/0003-caller-selected-execution-user.md) amended it once: a
**caller-selected execution user** is admitted, and a shell is admitted **only**
to compute that user's login environment under a fixed agent-authored command.
Argv is still `exec`ed as a vector and still reaches no shell.

## Project and process

### Acceptance

Proof that an **operator-visible runtime feature** works, in the specific sense
[`TESTING.md`](TESTING.md) requires: a black-box test that invokes the
production CLI or API, crosses the production transport, executes in the
intended agent process, and verifies the externally observable effect. Package
tests support development; they do not close acceptance.

Planning and documentation tasks cross no transport and execute in no agent.
They are accepted against their own stated cases, together with what those
cases cannot detect — recorded in the task's dossier where one is required, and
in the pull request where the task is a supporting task that has none.

### Generation 0, 1, 2

Generation 0 is the older `archive/v0` lineage. Generation 1 is the `v0.1.0` /
`v0.5.0` implementation, archived at `archive-2026-09-pre-v0.6-reboot`.
Generation 2 is the post-reboot line, beginning at `v0.6.0`.

### Dossier

The checked-in artifact that turns a workstream paragraph into implementation
authority. See [`../dossiers/README.md`](../dossiers/README.md).

### Naming

Two names, by scope:

- **`keystone`** — what a human types and what the OS runs. The operator CLI is
  `keystone`, with `ks` installed as an alias; the daemons are
  `keystone-server` and `keystone-agent` (charter § 5).
- **`keystone-core`** — project and brand identity: the project name, the
  repository, the Go module path `go.keystone-core.io/keystone-core`, the domain
  `keystone-core.io`, and release-artifact names.

Generation 1 used a different split, in which OS-run names took a `kscore`
prefix. Generation 2 does not; RFC 0001 breaks Generation 1's CLIs, so
continuity was available but not owed.

## Reader aids

**The terms listed below are defined earlier in this document, and those
definitions are not Keystone decisions.** They are general terms, defined
because the documents that use them assume the reader knows them. Some entries
also note where Generation 2 applies the term, or which task will decide its
Keystone-specific meaning; those notes are context, not authority, and a term
does not leave this list by acquiring one.

- Attestation
- CA (Certificate Authority)
- Cipher Suite
- Defense in Depth
- Encryption at Rest
- Encryption in Transit
- Fail Secure (Fail Closed)
- HMAC (Hash-based Message Authentication Code)
- Key Derivation Function (KDF)
- mTLS (Mutual TLS)
- TLS (Transport Layer Security)
- Token
- Trust Boundary
- Trust Domain
- X.509
- Zero Trust

Listing them is the point: a term with no project authority behind it is
visibly an aid rather than an accidental leftover, which is how Generation 1's
vocabulary survived.

Every other term is either used elsewhere in the current documents or cites an
**accepted** decision that establishes it. A pointer to a decision a later task
will make is not authority — it is the absence of one.

## Not yet defined

These terms will be defined by the task that decides them. They are listed so
that their absence is visible rather than accidental.

| Term | Defined by |
|---|---|
| Subject grammar, permission matrix | P04 |
| Account topology, stream and consumer layout, limits | P02 |
| NKey, operator/account/user JWT as Keystone uses them | P02, P03 |
| Bootstrap credential, permanent scoped credential | P03 |
| Envelope, protocol version, signing and encryption layers | P05 |
| Delivery failure, expiry, redelivery policy | P06 |
| Execution limits, working directory, environment handling | P07 |
| Audit record, ledger, retention | P08 |
| Local operator API surface, operator authorization | P09 |

## Removed terminology

Generation 1 terms were removed rather than rewritten, so that a superseded
definition cannot be mistaken for a current one. Roughly fifty terms went:
blueprints, state modules, drift, runbooks, sagas, clustering, quorum, fencing,
etcd, high availability, leaf nodes, superclusters, edge and proxy agents,
GitOps, webhooks, SPIFFE/SVID, OPA/Rego, CEL, RBAC, secrets leases and transit,
Starlark, WASM, gRPC, REST, OTLP, SNMP and WinRM.

Every one of them is catalogued as a `Future / Unscheduled` candidate in
[`FUTURE-CAPABILITIES.md`](FUTURE-CAPABILITIES.md), and the Generation 1
definitions remain readable:

```bash
git show archive-2026-09-pre-v0.6-reboot:docs/project/GLOSSARY.md
```

A term returns to this glossary when the capability it names is promoted through
the roadmap gate and an accepted design defines it — not before.
