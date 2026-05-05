# Maintainers Guide

This document is for maintainers of Keystone Core: people who have merge rights and help steer the
technical direction of the project.

It should stay short, practical, and focused on day-to-day decisions.

---

## Responsibilities

As a maintainer, you are expected to:

- review and merge pull requests
- enforce the compatibility and support policy
- protect upgrade safety and operator experience
- treat contributors respectfully
- help keep the issue tracker and RFCs manageable
- coordinate on releases and migrations

You are **not** expected to be always available or on call. Real life comes first.

---

## Decision Making

For most changes, maintainers decide by **rough consensus**:

- Is the change correct?
- Is it maintainable?
- Is it compatible?
- Does it fit the project scope?

If a change is minor and clearly beneficial, just merge it after review.

If a change is risky, unclear, or controversial:

- ask for more context
- request an RFC
- or defer to the BDFL if it touches scope/architecture/vision

---

## When to Require an RFC

Require an RFC (see [RFC.md](RFC.md)) when a change:

- affects schema or storage layout
- modifies controller/agent protocol
- changes configuration semantics
- impacts the upgrade path
- introduces or removes major features
- alters CLI/UX in ways operators will notice
- changes compatibility or support behavior
- alters governance or licensing

Small internal refactors and clearly safe fixes do **not** need RFCs.

---

## Working with the BDFL

The BDFL sets direction and may occasionally override decisions. This should be rare and focused on
strategy, not patch-level details.

If you feel a change is borderline in terms of scope or architecture, proactively loop in the BDFL
before it becomes a time sink.

---

## Compatibility & Upgrades

Maintainers are the front line of enforcing the compatibility and upgrade guidelines. Before merging
changes that touch compatibility surfaces, confirm they:

- respect the current support window
- follow expand → migrate → contract for schema
- keep config forward-compatible where possible
- document deprecations and breaking changes
- include or request migration notes

If something feels like it could surprise operators during an upgrade, slow down and demand clarity.

---

## Releases

Release mechanics may evolve, but maintainers will typically:

- help prepare release notes
- ensure migrations are documented
- confirm CI is green on supported versions
- help validate upgrade paths

The BDFL makes final calls on release timing and compatibility policy.

---

## Stepping Down

If you no longer have time or interest in maintaining Keystone Core, that's okay. Just let the BDFL
and other maintainers know so we can adjust expectations and permissions.

---

## Adding New Maintainers

Propose new maintainers when someone has:

- a history of solid contributions
- demonstrated good review judgment
- shown they understand compatibility and upgrades
- participated constructively in discussions

Final decisions are made by the BDFL in consultation with existing maintainers.

---

## Final Notes

Being a maintainer is about stewardship, not control. The job is to keep Keystone Core healthy,
coherent, and reliable over time.

Thank you for taking that seriously.
