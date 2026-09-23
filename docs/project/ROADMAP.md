# Roadmap

Four buckets, and only two of them are commitments.

Generation 1's ranked backlog — 163 entries across five milestone tiers — was
archived with the code it described. It is readable, and it is not a plan:

```bash
git show archive-2026-09-pre-v0.6-reboot:docs/project/ROADMAP.md
```

## Now

The Generation 2 reboot, tracked in
[epic 20](../../epics/20-generation-2-reboot.md) and sequenced in the
[execution plan](REBOOT-EXECUTION-PLAN.md).

- **Stage R — repository transition.** Freeze, archive, retire the old tracker,
  land this baseline, publish Generation 2 planning state, independent audit.
- **Stage P — product and architecture foundation.** Product charter, threat
  model, and the NATS, enrollment, authorisation, protocol, delivery, execution,
  persistence, operator-API, acceptance-harness and repository-skeleton ADRs.
  No production implementation starts until these are accepted. The charter and
the threat model are accepted: [`PRODUCT-CHARTER.md`](PRODUCT-CHARTER.md),
[`THREAT-MODEL.md`](THREAT-MODEL.md). The first ADR is proposed:
[`ADR-0002`](../adr/0002-nats-native-capabilities.md) and
[`ADR-0003`](../adr/0003-enrollment-and-identity.md) and
[`ADR-0004`](../adr/0004-subject-authorization.md) and
[`ADR-0005`](../adr/0005-versioned-encrypted-protocol.md) and
[`ADR-0006`](../adr/0006-delivery-and-job-lifecycle.md) and
[`ADR-0007`](../adr/0007-safe-execution.md) and
[`ADR-0008`](../adr/0008-persistence-and-audit.md) and
[`ADR-0009`](../adr/0009-local-operator-api-and-authorization.md) and
[`ADR-0010`](../adr/0010-acceptance-harness.md). **P11 is next, and it is the
last of Stage P** — the first task that reintroduces a build, and where a
planned-evidence filename either exists or does not.
- **Stage C — the first command-and-control release**, ending at `v0.6.0`.

## Next

Nothing.

This is deliberate. `Next` holds work that is accepted but not started, and
nothing qualifies until the Stage P decisions exist. An empty `Next` is the
honest state of a project that has just restarted, and filling it early is how
the previous generation acquired a backlog larger than its evidence.

## Future / Unscheduled

Everything Generation 1 built or planned beyond the narrow `v0.6.0` slice:
state management, blueprints, runbooks, plugins, secrets, GitOps, webhooks,
policy enforcement, clustering, the module catalogue, and the rest.

All 703 entries are catalogued in
[`FUTURE-CAPABILITIES.md`](FUTURE-CAPABILITIES.md) with their Generation 1
status, known gaps and archived source.

Catalogue presence means "retain the idea", not "planned". Nothing here has a
version, a date or an issue. Promotion to `Next` requires all of:

- at least two external operators connecting it to a repeated problem;
- the current command-and-control SLOs and security gates still green;
- prerequisites and operational cost understood;
- an RFC defining its minimal slice and explicit non-goals;
- production-process Docker tests designed before implementation;
- required VM tests identified; and
- the maintainer accepting its maintenance and commercial rationale.

**Agent decommission is decided but unbuilt.** RFC 0005 § 3 fixes how a
permanent identity will be revoked — application refusal, short-lived
server-renewed credentials, and the operator key offline as an emergency path —
so the task that promotes it inherits a design rather than a custody problem.

**Regulated-market posture is an open question.** Whether Keystone targets
deployments that require FIPS 140 validated cryptography, FedRAMP or DISA STIGs
is undecided. It reaches beyond any one task: TLS configuration, where private
keys may live (a CA key in a passphrase-protected file is unlikely to satisfy an
assessor who expects an HSM or KMS), running Go in FIPS mode, and `ADR-0005`'s
hybrid key agreement, whose X25519 half is not FIPS-approved on its own. It
needs an RFC before any task assumes an answer. Raised at `G49`.

## Not Planned

Directions that need a new product-level RFC rather than ordinary promotion.
Keystone Core is not becoming a general-purpose IaC engine, a Kubernetes
replacement, a remote-desktop product, an endpoint-detection product, or an
unbounded shell gateway.

The first release's execution surface is argv-only and non-interactive.
**Keystone constructs** no shell invocation, no script, no stdin streaming, no
pipeline, no caller-provided environment or working directory, no batch fan-out
and no interactive session — which is what the exclusions constrain, per
[RFC 0004](../rfcs/0004-what-the-execution-exclusions-constrain.md), rather than
which programs an operator may name. A **caller-selected execution user** is
admitted by
[RFC 0003](../rfcs/0003-caller-selected-execution-user.md), which also admits a
shell solely to compute that user's login environment. Widening that boundary
further requires an amendment to
[RFC 0001](../rfcs/0001-generation-2-reboot.md), not a roadmap entry.
