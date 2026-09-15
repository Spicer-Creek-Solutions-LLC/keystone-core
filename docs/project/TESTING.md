# Generation 2 Testing Requirements

This document defines the minimum evidence required to call a Generation 2
feature complete. It supplements unit-test and coverage policy; it does not
permit lower-level tests to replace deployed behavior tests.

The transition tasks use the repository's current documentation gates. P11
turns the requirements below into enforced Generation 2 CI before product
implementation begins.

## Governing rule

An operator-visible feature is complete only when a black-box test:

1. invokes the production CLI or public API;
2. crosses the production NATS transport and authorization policy;
3. reaches a separately running production agent binary;
4. executes in the intended agent container or VM;
5. verifies the externally observable result;
6. verifies the server and non-target agents were unchanged;
7. verifies durable status, attribution, and audit output; and
8. exercises at least one denial or failure path.

Mocks, fakes, package tests, and in-process integration tests remain useful for
fast feedback. They cannot close feature acceptance.

## Required test topology

The minimum Docker topology contains:

- one NATS server using the production account/JWT configuration;
- one Keystone server;
- at least two Keystone agents with different identities;
- one operator client; and
- isolated server and agent networks, with NATS as the only shared application
  endpoint.

No agent publishes a host port. Network probes in the test must demonstrate
that server-to-agent and agent-to-agent direct connections fail. Tests run the
same binary entry points and configuration loaders used by packaged deployments.

The harness retains sanitized broker configuration, server and agent logs,
audit records, job ledgers, and test assertions as CI artifacts on failure. P10
defines artifact classification and redaction; tests plant credential and
payload canaries and fail before upload if any canary or private seed remains.

## Feature acceptance contract

Every feature PR supplies a table with these cases:

| Case | Required evidence |
|---|---|
| Intended target | External state changes only on the selected agent |
| Non-target | A second agent remains unchanged |
| Server isolation | The server container has no equivalent side effect |
| Authorization denial | An unauthorized NATS identity is rejected by NATS |
| Payload protection | Broker-visible data contains no command/result plaintext |
| Duplicate delivery | A duplicate message does not repeat execution |
| Restart | Restart at each durable boundary preserves honest state |
| Cancellation/timeout | For a cancellation that reaches a running command, the complete process tree exits and state is terminal. For one that does not, the job's terminal state is what the agent proves and both observations stay visible (`ADR-0006` § 9) |
| Audit | Actor, target, job, action, result, and correlation are present |
| Diagnostics | Operator can distinguish denial, offline, timeout, and unknown |

The task dossier marks each case applicable or gives a reviewed reason for
`N/A`. Security and durability cases relevant to a touched boundary cannot be
waived as inapplicable.

State or blueprint support, when reconsidered, must prove that the production
CLI invocation changes agent 1, leaves agent 2 and the server unchanged, and
reports the converged state after a fresh read. Rendering YAML or calling a
state engine in the server process is not acceptance.

## NATS security suite

The Docker suite provisions explicit identities for the command publisher,
enrollment service, result consumer, presence consumer, agent 1, agent 2, a
revoked bootstrap principal, and an unrelated principal. It verifies:

- allowed publish and subscribe operations succeed;
- every cross-agent operation fails at broker authorization;
- agents cannot publish command or cancellation messages;
- agents cannot consume other agents' results or commands;
- ordinary identities cannot access `$SYS`, JetStream management APIs, or any
  JetStream/inbox subject outside their enumerated data-plane allowlist;
- bootstrap access stops after enrollment;
- payload encryption still protects content from a broker observer; and
- application signature validation rejects a broker-authorized but incorrectly
  signed envelope.

These tests must exercise the generated production JWTs and permissions, not a
test-only authorization adapter.

## Durability and fault injection

The harness can stop the agent, server, and NATS independently at each boundary:

- before agent receipt is persisted;
- after receipt but before process start;
- while the process runs;
- after process exit but before result persistence;
- after result persistence but before publish;
- after publish but before command acknowledgement; and
- while cancellation is in flight.

On restart, the system must either complete result delivery without re-executing
or report an explicit `UNKNOWN` state. Tests use a non-idempotent external
counter so accidental duplicate execution cannot pass unnoticed.

## Docker versus VM coverage

Docker is the integration floor for all features. VM tests are additionally
required when behavior depends on systemd or another init system, reboot,
kernel interfaces, firewall rules, network configuration, mounts, filesystems,
LVM, package-manager transactions, users/groups, or host security policy.

Container-only emulations of those capabilities may provide PR feedback but do
not satisfy their release gate.

## Gate schedule

### Every pull request

- formatting, lint, static analysis, and dependency-boundary checks;
- unit tests and race detector;
- protocol compatibility and cryptographic test vectors;
- architecture-invariant and traceability lint;
- the production Docker topology for the touched operator journey;
- NATS permission positive and negative tests; and
- regression tests for every fixed defect.

### Every merge to `main`

- complete Docker journey suite;
- restart and fault-boundary matrix;
- adversarial identity and malformed-envelope suite;
- package/install smoke tests; and
- documentation link and requirement coverage checks.

### Nightly

- at least 100 agents for one hour;
- broker, server, and agent restart/partition chaos;
- file-descriptor, goroutine, connection, storage, and consumer-leak checks;
- delivery-lag, redelivery, and resource-limit assertions; and
- supported-version NATS compatibility matrix.

### Release candidate

- fresh-VM install and uninstall on every supported distribution;
- upgrade from the previous Generation 2 release or explicit first-release
  install path;
- system integration and reboot tests where applicable;
- supply-chain, vulnerability, secret, and license scans;
- an independent security review of identity and permissions; and
- an external operator completing the documented journey without maintainer
  intervention.

Unresolved Critical or High security findings block pilot and release unless
the maintainer records a time-bounded risk acceptance with owner, rationale,
compensating control, and expiry.

## Test ownership and change control

Acceptance tests are derived from approved requirements before implementation.
The implementation agent may add coverage but may not weaken, skip, or delete an
acceptance assertion in the implementation PR. A requirement change and its
test change land in a separate approved specification PR.

Security-sensitive work requires an independent reviewer. Flaky tests are bugs:
quarantine requires an issue, an owner, an expiry date, and cannot waive a
release gate.

## Definition of shipped

A feature may be marked shipped only when:

- its requirement identifiers and owner are recorded;
- unit, black-box, negative, restart, and audit cases pass as applicable;
- production Docker topology passes without test-only transport shortcuts;
- required VM cases pass;
- operator and security documentation is current;
- failure artifacts are actionable and secret-safe; and
- the requirements traceability matrix points to the exact automated evidence.
