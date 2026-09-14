# Generation 2 Architecture Invariants

These requirements are normative for Generation 2. A feature that violates an
invariant cannot merge without an accepted RFC changing the invariant first.
Tests use these identifiers so architecture requirements remain traceable.

## Communication and topology

### ARCH-COMM-001 — NATS-only agent communication

All agent-to-server and server-to-agent application communication, including
enrollment, commands, cancellation, results, events, and presence, crosses
NATS. Agents expose no inbound application listener. The operator-facing API
may use a local Unix-domain socket; it must not become a hidden agent transport.

### ARCH-COMM-002 — No direct reachability assumption

The server and agents must work when they cannot dial each other directly.
Production acceptance places them on isolated container networks with only the
NATS broker attached to both.

### ARCH-COMM-003 — Production paths only

Acceptance uses released binaries, production configuration parsing, production
protocol serialization, and production NATS subjects. An in-memory bus or
in-process agent is never a substitute for feature acceptance.

## NATS identity and authorization

### ARCH-NATS-001 — Deployment account isolation

Keystone traffic runs in a dedicated NATS account. A separate system account is
used for broker administration and advisories.

### ARCH-NATS-002 — Unique role identities

The command publisher, enrollment service, result consumer, presence consumer,
monitoring role, and every agent have distinct NATS identities. Co-located
services are still modeled and provisioned as separate principals.

### ARCH-NATS-003 — Exact least-privilege subjects

Each agent may subscribe only to its own command and cancellation subjects and
publish only to its own result, event, and presence subjects. Service roles
receive only the corresponding direction and scope. Wildcard access across
agent identifiers is denied unless a narrowly documented service requires it.

### ARCH-NATS-004 — Bootstrap isolation and revocation

Enrollment uses one-use, token-scoped NATS credentials and token-specific
subjects. It is a staged, idempotent protocol: issue pending credentials; fsync
and atomically rename the agent credential file with mode `0600`; prove a
permanent connection; mark the identity active; revoke bootstrap access; and
verify revocation. Short bootstrap expiry is the failure backstop. Every stage
has a defined crash-recovery path.

### ARCH-NATS-005 — Broker API isolation

Agent and ordinary service identities cannot access `$SYS`, JetStream management
APIs, unrestricted inboxes, other deployments, or enrollment subjects after
enrollment. A pull consumer receives only the enumerated JetStream consumer-next,
acknowledgement, publish-ack, and reply permissions required for its exact
consumer. Those narrowly enumerated data-plane `$JS.API` subjects are permitted;
arbitrary or administrative `$JS.API` access is not. P02/P04 must document and
test that system-subject allowlist.

### ARCH-NATS-006 — Independent cryptographic layers

NATS transport identity, Keystone envelope signing, and Keystone payload
encryption use separate keys. Commands are signed by an authorized service and
encrypted to the target agent. Results are signed by the agent and encrypted to
the authorized result service.

**Every envelope class the product carries is signed.** The classes are
enrollment request, enrollment reply, command, cancellation, result, lifecycle
event, and presence. An envelope that does not verify is refused.

**Payload encryption is required where a payload carries operator intent or
captured output**: commands and cancellations are encrypted to the target agent,
results to the authorized result service. The remaining classes are signed and
not encrypted — this invariant permitting that, not omitting them.

**This enumeration is closed.** A new envelope class, or a change to which
classes are encrypted, amends this invariant before the design that needs it is
accepted. Amended by [RFC 0002](../rfcs/0002-signed-envelope-classes.md), which
records why the original enumeration was incomplete.

### ARCH-NATS-007 — Bounded broker resources

Accounts and streams define explicit connection, subscription, payload,
consumer, byte, age, and message limits. Unbounded streams and consumers are
prohibited.

### ARCH-NATS-008 — Deliberate native-feature decisions

Every messaging ADR records relevant NATS capabilities as `Adopt`, `Evaluate`,
`Defer`, or `Reject`. Keystone does not duplicate native NATS behavior without
documenting the missing requirement.

### ARCH-NATS-009 — Serialized per-agent command consumption

The initial command consumer has one exact agent `FilterSubject` and defaults to
`MaxAckPending=1`. Any later parallelism requires an accepted execution-ordering
design and still obeys the agent's durable at-most-one-attempt ledger.

### ARCH-NATS-010 — Finite broker redelivery

Command consumers define finite `MaxDeliver` and explicit `BackOff`, with
terminal delivery advisories. Broker redelivery repairs transport failure; it
does not authorize Keystone to create a second logical execution attempt.

## Delivery and execution

### ARCH-JOB-001 — Honest delivery semantics

The protocol documents JetStream at-least-once delivery and never claims
exactly-once execution.

### ARCH-JOB-002 — Durable receipt before execution

An agent persists the command identifier, authenticated envelope metadata, and
receipt state before starting a process.

### ARCH-JOB-003 — At-most-one automatic execution attempt

Redelivery of a known command identifier may resume result delivery but may not
automatically execute the command again. The durable agent ledger, not the NATS
deduplication window, is authoritative.

### ARCH-JOB-004 — Ambiguity is explicit

If the control plane cannot prove whether a command ran, it reports `UNKNOWN`
and never hides ambiguity by retrying arbitrary commands. A verified late result
may supersede that knowledge state; both observations remain in the audit log.
The job ADR separately defines delivery failure and expiry.

### ARCH-JOB-005 — Terminal acknowledgement

The agent acknowledges a command only after its receipt, terminal state, and
result are durable and the result publication receives a broker acknowledgement.
Long operations emit acknowledgement progress without changing this rule.

### ARCH-EXEC-001 — Bounded execution

Commands have explicit duration, output, environment, working-directory,
concurrency, and resource limits. Policy is deny-by-default for unsupported
execution forms.

### ARCH-EXEC-002 — Complete cancellation

Timeout and cancellation terminate the full process group or platform-equivalent
job object, not only the immediate child process.

## Observability and validation

### ARCH-OBS-001 — Auditable lifecycle

Enrollment, authorization denial, receipt, start, cancellation, timeout,
completion, unknown outcome, and result retrieval emit correlated audit records
without command secrets or payload plaintext in broker headers.

### ARCH-TEST-001 — External-effect acceptance

Every remote feature has a test proving the effect occurred on the intended
agent and did not occur on the server or a non-target agent.

### ARCH-TEST-002 — Negative identities are mandatory

Tests prove that an agent cannot publish commands, consume another agent's
commands, publish another agent's results, use revoked bootstrap credentials, or
access broker administration subjects.

### ARCH-TEST-003 — Architecture is executable

Every invariant is enforced by an automated test, static dependency rule, or a
named release review. Manual-only requirements must state why automation is not
possible and identify the reviewer evidence retained for a release.
