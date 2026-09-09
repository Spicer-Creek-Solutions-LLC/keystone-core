# RFC 0001: Generation 2 Reboot

- **Status:** Accepted for repository/product direction; detailed mechanisms
  require the ADRs named below
- **Decision date:** 2026-09-08
- **Decision owner:** Project maintainer
- **Implementation plan:**
  [`REBOOT-EXECUTION-PLAN.md`](../project/REBOOT-EXECUTION-PLAN.md)
- **Evidence:**
  [`PROJECT-REBOOT-REVIEW.md`](../project/PROJECT-REBOOT-REVIEW.md)

## Summary

Keystone Core will preserve the current implementation as Generation 1 and
restart active development as Generation 2. Generation 2 begins with a narrow,
secure command-and-control product over NATS. Capabilities are added one
building block at a time only after the preceding block passes its architecture,
security, real-process, and operator-acceptance gates.

The repository history will remain continuous. The transition will use a normal
commit on `main`; it will not force-push, rewrite, or create an orphan history.
The final Generation 1 tree will remain available through a protected archive
branch, a signed annotated tag, and an offline Git bundle. If tag-signing
preflight fails, the transition pauses for a maintainer decision rather than
silently creating a weaker tag.

## Motivation

The Generation 1 implementation proved that the product idea and most major
technical components are feasible, but it accumulated a broad surface before
the core operator journey was proven end to end. There are no current users or
compatibility obligations. Recent fixes also showed that package-level coverage
could coexist with missing production wiring: enrollment credentials were not
retained and states and blueprints did not reliably execute on remote agents.

Continuing to add features would preserve the largest current risk: a large
catalog whose individual parts exist but whose deployed workflows are not
demonstrably complete. The reboot uses the implementation as research while
removing it from the active product surface.

## Product decision

Generation 2 starts with one product promise:

> An operator can securely enroll a Linux agent, target it, execute a bounded
> command over NATS, observe its durable lifecycle, cancel it, and retrieve an
> auditable result.

The first phase includes only the foundations required to make that promise
safe and dependable:

- a server, an agent, and a local operator CLI/API;
- NATS-only agent-to-server and server-to-agent communication;
- one-time enrollment and per-agent identity;
- exact per-agent subject authorization;
- signed, versioned, end-to-end encrypted command and result envelopes;
- durable job receipt, result, cancellation, and explicit unknown outcomes;
- bounded non-interactive command execution;
- agent presence and minimal inventory;
- audit records, metrics, diagnostics, packaging, and upgrades; and
- black-box Docker and VM acceptance tests using production binaries.

State management, blueprints, runbooks, plugins, secrets, GitOps, webhooks,
policy, clustering, a large module catalog, and other Generation 1 capabilities
are not implicitly part of the first release. They begin in
[`FUTURE-CAPABILITIES.md`](../project/FUTURE-CAPABILITIES.md) as uncommitted,
unversioned candidates.

## NATS-first architecture

NATS is infrastructure, not merely a byte transport. Before Keystone adds a
messaging, authorization, durability, flow-control, or broker-observability
mechanism, the design must evaluate the native NATS capability. The ADR must
record `Adopt`, `Evaluate`, `Defer`, or `Reject`, with evidence.

The initial security layers are independent and cumulative:

1. NATS account isolation.
2. NKey/JWT identity for every service role and every agent.
3. NATS publish and subscribe permissions on exact subjects.
4. Keystone application signatures on protocol envelopes.
5. Keystone end-to-end payload encryption to the intended recipient.
6. Local agent execution policy.

NATS TLS and subject permissions do not provide end-to-end payload secrecy.
They limit connections and actions at the broker; Keystone encryption prevents
the broker or a broker administrator from reading command and result payloads.
NATS NKey seeds must not be reused as Keystone signing or encryption keys.

The required initial NATS capability set includes accounts, NKeys/JWT, exact
subject permissions, TLS, JetStream streams and durable consumers, explicit
acknowledgements, `Nats-Msg-Id` publication deduplication, bounded stream and
account limits, a system account, and advisories. Response permissions, KV,
subject mappings, Object Store, queue groups, leaf nodes, and superclusters are
introduced only by a later accepted ADR and demonstrated need. P02-P06 decide
the safe configuration and protocol details inside these constraints; they do
not merely ratify an implementation selected in advance.

## Messaging and job semantics

JetStream provides at-least-once delivery. Keystone must not represent that as
exactly-once execution.

- The agent durably records a job before execution.
- A job identifier permits at most one automatic execution attempt on an agent.
- Duplicate delivery may republish a persisted result but may not rerun the
  command.
- If the server cannot determine whether execution occurred, its knowledge
  state is `UNKNOWN`; it must not silently retry an arbitrary command. A later
  verified agent result may supersede `UNKNOWN`, while the audit log retains
  both observations.
- A command is acknowledged only after receipt is durable, execution is
  terminal, the result is durable, and result publication has a broker ack.
- Long-running work uses acknowledgement progress signals.
- Cancellation and timeout apply to the complete process tree.

The application ledger remains authoritative after the JetStream duplicate
window expires.

## Repository preservation

The final Generation 1 commit will be preserved as:

- branch: `archive/2026-09-pre-v0.6-reboot`;
- annotated tag: `archive-2026-09-pre-v0.6-reboot`; and
- an offline `git bundle` plus SHA-256 checksum.

The branch and tag will be protected on the primary Codeberg repository and the
GitHub mirror. Existing release tags, including `v0.1.0` and `v0.5.0`, remain
unchanged. Because forge owners can alter protection settings, the tag and
bundle provide the stronger integrity evidence.

The repository already contains an older `archive/v0` lineage. Documentation
will call that Generation 0, the current `v0.1`/`v0.5` lineage Generation 1,
and the reboot Generation 2.

## Versioning

- Untagged development builds report `0.0.0-dev+g<commit>`.
- The project will not create a public `v0.0.0` tag.
- External pilot builds use `v0.6.0-alpha.N`.
- The first completed Generation 2 release is `v0.6.0`.
- Historical tags must not influence Generation 2 development-version output.

This preserves SemVer ordering and avoids making tooling interpret the reboot as
an upgrade from `v0.5.0` to `v0.0.0`.

## Roadmap and tracker policy

The active roadmap has four buckets: `Now`, `Next`, `Future / Unscheduled`, and
`Not Planned`. Only accepted work appears in `Now` or `Next`. Every archived
capability starts in `Future / Unscheduled`, without a version promise or leaf
issue. Moving it forward requires operator evidence, prerequisites, an accepted
design, and its own acceptance strategy.

Generation 1 issues will be closed as **superseded, not completed** only after
stable archive links exist. Original bodies, labels, comments, and milestones
will be retained. The transition is an audited, allowlisted, resumable operation
described in the execution plan.

## Testing decision

An operator-visible feature is not complete until a black-box test invokes the
production CLI or API, crosses the production transport, executes in the
intended agent process or container, and verifies the externally observable
effect. Mocks, package tests, and in-process agents support development but
cannot satisfy feature acceptance.

Every feature must map to stable requirements and tests. Docker is the minimum
integration environment. Features involving init systems, reboot, networking,
firewalls, mounts, LVM, packages, or kernel behavior also require VM tests.
The normative requirements are in
[`ARCHITECTURE-INVARIANTS.md`](../project/ARCHITECTURE-INVARIANTS.md) and
[`TESTING.md`](../project/TESTING.md).

## Compatibility and migration

Generation 2 intentionally breaks all Generation 1 APIs, CLIs, configuration,
storage, protocols, packages, and deployment assumptions. There is no user-data
migration because the project has no current users. Archived releases remain
available for historical reference and are unsupported after the transition.

## Implementation governance

Work follows [`REBOOT-EXECUTION-PLAN.md`](../project/REBOOT-EXECUTION-PLAN.md).
Each task receives a dedicated approval, branch, implementation agent, review,
and pull request. An agent implementing a security-sensitive behavior may not
also be the sole acceptance reviewer. Acceptance tests are written from the
approved requirement and may not be weakened in the implementation PR.

Remote repository changes, issue closures, milestone closures, and protection
changes require an explicit apply approval after a dry run.

The first release execution surface is argv-only and non-interactive. It has no
shell, stdin streaming, scripts, pipelines, caller-selected user, caller-provided
environment, arbitrary working directory, batch fan-out, or interactive
session. Expanding that boundary requires an RFC amendment.

## Alternatives considered

### Incrementally simplify Generation 1

Rejected. It would require distinguishing dependable production paths from
partially wired paths across a very large surface while continuing to carry its
maintenance and compatibility assumptions.

### Start a separate repository

Rejected. The existing name, governance, issue history, releases, and source
history are valuable. A continuous repository makes the reboot and its evidence
visible.

### Rewrite or orphan `main`

Rejected. It weakens auditability and makes recovery and comparison harder
without creating product value.

## Rollback

Before the clean-baseline PR merges, rollback means abandoning the transition
branch. After merge, Git rollback means reverting the baseline commit or
branching from the immutable Generation 1 tag. Forge mutations have their own
before-state manifest and compensating procedure. Issue retirement is normally
not reversed because the original content remains intact; reopening it requires
an explicit maintainer decision. No archived ref or historical release is
deleted during rollback.

## References

- [NATS authorization](https://docs.nats.io/running-a-nats-service/configuration/securing_nats/authorization)
- [NATS decentralized JWT authentication](https://docs.nats.io/running-a-nats-service/configuration/securing_nats/auth_intro/jwt)
- [JetStream consumers](https://docs.nats.io/nats-concepts/jetstream/consumers)
- [JetStream model](https://docs.nats.io/nats-concepts/jetstream)
