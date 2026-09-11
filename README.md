# Keystone Core

[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Status](https://img.shields.io/badge/Status-planning-lightgrey)](docs/project/VERSIONING.md)
[![AI Contributions Welcome](https://img.shields.io/badge/AI_Contributions-Welcome-brightgreen)](docs/project/AI-CONTRIBUTIONS.md)

> **There is nothing to install here yet.** This repository currently holds
> planning, governance and transition evidence. The implementation it used to
> contain has been archived, and the replacement has not been written.

Keystone Core intends to be the runtime operations control plane between
deployment tooling and day-2 operations: GitOps and infrastructure-as-code
describe what should be deployed, and Keystone Core answers what is actually
happening on the fleet now.

## What happened

In September 2026 the project accepted a reboot. The Generation 1
implementation — around 1,900 files, two public releases, a large feature
surface — was archived rather than continued.

It was not archived because it failed to work. It was archived because its
breadth outran the evidence that any single operator journey worked end to end.
Recent fixes had found enrollment credentials that were not retained, and state
and blueprint applies that did not reliably reach remote agents — cases where
package-level test coverage sat alongside missing production wiring. Adding
more features would have preserved that risk rather than resolved it.

The decision, its reasoning and the alternatives that were rejected are in
[RFC 0001](docs/rfcs/0001-generation-2-reboot.md).

## Where Generation 1 went

Nothing was deleted. The final state is preserved three ways:

| | |
|---|---|
| Branch | `archive/2026-09-pre-v0.6-reboot` |
| Signed tag | `archive-2026-09-pre-v0.6-reboot` |
| Offline bundle | held by the maintainer, SHA-256 in [`docs/transition/manifest.json`](docs/transition/manifest.json) |

Both refs are protected against update and deletion. The `v0.1.0` and `v0.5.0`
releases remain published and unchanged, and are unsupported.

Every capability that existed is catalogued in
[`FUTURE-CAPABILITIES.md`](docs/project/FUTURE-CAPABILITIES.md) — 703 entries
with their status, known gaps and archived source — so the reboot discards the
code without discarding what was learned.

## What Generation 2 is

One promise, deliberately narrow:

> An operator can securely enroll a Linux agent, target it, execute a bounded
> command over NATS, observe its durable lifecycle, cancel it, and retrieve an
> auditable result.

Nothing else ships until that is demonstrably true — proven by black-box tests
driving production binaries across the real transport, not by package tests.
State management, blueprints, runbooks, plugins, secrets, GitOps, webhooks,
policy and clustering are all Future candidates rather than commitments.

## Where things are

- [RFC 0001](docs/rfcs/0001-generation-2-reboot.md) — the decision
- [Execution plan](docs/project/REBOOT-EXECUTION-PLAN.md) — every task, in order
- [Architecture invariants](docs/project/ARCHITECTURE-INVARIANTS.md) — 24 rules with stable identifiers
- [Testing requirements](docs/project/TESTING.md) — what "shipped" means
- [Capability catalog](docs/project/FUTURE-CAPABILITIES.md) — everything Generation 1 had
- [Roadmap](docs/project/ROADMAP.md) — Now, Next, Future, Not Planned
- [Transition evidence](docs/transition/) — the archive manifest and tracker-retirement record
- [Epic 20](epics/20-generation-2-reboot.md) — the reboot's task list

## Contributing

Read [`CONTRIBUTING.md`](CONTRIBUTING.md) and
[`AGENTS.md`](AGENTS.md). Commits need a DCO sign-off
([`DCO.md`](docs/project/DCO.md)) and AI-assisted work must be disclosed
([`AI-CONTRIBUTIONS.md`](docs/project/AI-CONTRIBUTIONS.md)).

The issue tracker is empty by design: its 106 Generation 1 issues were closed as
superseded, not completed, during the transition. Generation 2 issues are
created as the work is accepted, not in advance.

## Security

See [`SECURITY.md`](SECURITY.md). There is no supported release: the archived
Generation 1 releases receive no security updates.

## Licence

Apache 2.0 — see [`LICENSE`](LICENSE) and [`NOTICE`](NOTICE).
