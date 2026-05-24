# Keystone Core Governance

Keystone Core is an infrastructure platform. These systems work best when guided by long-term
technical vision, predictable compatibility, and clear decision making. This document describes how
we make decisions as the project grows.

This is not a legal document and not a contract. It's simply how we intend to operate in a way that
keeps the project healthy, stable, and pleasant to contribute to.

> **Project sponsor**: Keystone Core is a project of Spicer Creek Solutions LLC. SCS holds the
> authority to appoint the BDFL described below but does not direct technical decisions. See
> [`OWNERSHIP.md`](../../OWNERSHIP.md) for the full ownership and operational structure.

---

## Roles

### BDFL (Benevolent Dictator for Life)

The BDFL guides the project. Responsibilities include:

- defining project scope and direction
- protecting long-term design and architecture
- setting compatibility policy
- resolving disagreements when needed
- approving breaking changes
- approving governance changes
- acting as final decision maker when consensus fails

The BDFL steers the project; they do not micromanage code.

### Maintainers

Maintainers review contributions and keep the project moving. Over time maintainers may take
ownership of specific areas (e.g., scheduler, runtime, plugins, config).

Responsibilities include:

- reviewing and merging contributions
- improving documentation
- enforcing compatibility policies
- participating in proposals
- making technical decisions by consensus when possible

### Contributors

Anyone submitting issues, code, docs, or proposals. Contributions are evaluated on technical merit
and fit with project goals.

---

## Decision Making

Keystone Core favors technical meritocracy and maintainer stewardship. The general flow:

> Discussion → Proposal (if needed) → Maintainer Review → Decision → Implementation

Most changes do not require a proposal. Maintainers can merge fixes, features, and performance
work directly after review.

---

## Proposals (For Bigger Changes)

A lightweight proposal is used for changes that affect:

- compatibility
- architecture
- schema/state
- controller/agent behavior
- user-facing configuration
- ecosystem or plugins
- CLI/UX
- long-term direction

A proposal should answer:

- what problem is being solved?
- why now?
- alternatives considered?
- compatibility and migration impact?
- operator impact?
- rollout and upgrade considerations?

The goal is clarity, not bureaucracy.

---

## Compatibility Interaction

Keystone Core has a separate compatibility policy. Maintainers are expected to follow it when
reviewing changes. Proposals must call out compatibility impacts explicitly.

---

## Breaking Changes

Breaking changes require:

- a proposal (unless trivial)
- maintainer agreement
- BDFL approval

The intention is to allow breaking changes when justified, not prevent them entirely.

---

## BDFL Override

If consensus cannot be reached, or if the change would harm the project's vision, architecture, or
ecosystem, the BDFL may override. Overrides should be rare, explained, and used for strategy, not
code style.

Examples of good override topics:

- project scope ("we are not rewriting this in Perl")
- ecosystem direction
- compatibility model
- security posture
- licensing
- architectural constraints

Examples of bad override topics:

- line-level implementation details
- personal preference disagreements
- bikeshed topics

---

## Security Issues

Security issues should be reported privately to the maintainers. Public disclosure happens once a
fix is available. Details of the process can evolve as needed.

---

## Adding & Removing Maintainers

Initially the BDFL selects maintainers. Over time, maintainers may nominate new maintainers based on
sustained contribution, thoughtful review, and compatibility awareness.

Maintainers may step down voluntarily at any time.

---

## Releases & Support Window

Releases follow a time-based schedule with a 2-year support window. Maintainers participate in the
release process. The BDFL sets compatibility policy and may approve lifecycle changes.

---

## Launch Posture

**v0.1.x — soft launch.** The repository is made public on Codeberg without a coordinated
announcement (no blog post, no mailing-list push, no aggregator submission). Discovery happens
organically through the GitOps/day-2-ops community, word of mouth, and direct outreach.

The intended v0.1.x audience is operators who have been **explicitly invited to install** (per
[`VERSIONING.md`](VERSIONING.md)) — people who already know what Keystone Core is, are willing
to file detailed bug reports, and understand that the v0.x line is pre-stable. The soft-launch
posture matches that audience: it surfaces the project to people who'll find it via deliberate
search, without inviting the broader drive-by traffic that a hard-launch announcement would.

A hard-launch announcement (blog post, HN/Lobsters submission, mailing-list push) is **not
planned until at least v0.5**, when the v0.5 gates in [`VERSIONING.md`](VERSIONING.md) have
been met and the project is closer to providing the operator experience a wider audience
would expect.

This decision lives here rather than only in the launch checklist because it shapes how
maintainers triage early issues, set expectations in `README.md`, and decide what to commit
to publicly during the v0.x line.

---

## Future Evolution

This governance model may evolve as the project and community grow. Changes to governance require a
proposal and BDFL approval.

The goal is to keep things simple, productive, and aligned with long-term vision.

---

## Non-Goals

This project will not adopt:

- voting
- popularity contests
- design-by-committee
- bureaucratic processes
- incompatible forks of intent

Governance exists to avoid drama, not generate it.
