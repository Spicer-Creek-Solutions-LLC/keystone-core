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

## Triage and Response

**v0.1.x posture: best effort, no formal SLO.** Keystone Core is in its v0.x line ahead of the v1.0
SemVer stability commitment, and the v0.1.x launch is a soft launch
(see [`GOVERNANCE.md` § Launch Posture](GOVERNANCE.md)). Maintainers respond as bandwidth allows;
no contractual response time is promised, and nothing here should be read as one.

The rough cadences below are the targets maintainers aim for. They are not commitments; if a
maintainer is on vacation, in a long focus block, or otherwise unavailable, response may be slower.
If you need a faster response than these targets, say so explicitly in the issue or report — that
signal helps triage.

### Issues (bug reports, feature requests, questions)

- **First-touch acknowledgement**: within ~1 week.
- **Triage decision** (accepted / needs more info / declined / closed as duplicate): within ~2 weeks
  of first touch.
- **Fix or scheduled work**: no commitment. Accepted bugs flow through the ROADMAP buckets
  (`gate-v0.5` / `gate-v1.0` / `v0.x` / `v1.x` / `v2.x+`) per
  [`ROADMAP.md`](ROADMAP.md); critical bugs jump the queue.

### Pull requests

- **First-touch acknowledgement**: within ~1 week.
- **First review round**: within ~2 weeks. CI must be green before a maintainer review starts —
  the reviewer should not be the one debugging your test failures.
- **Subsequent rounds**: typically ~1 week per round.
- **Merge**: only after at least one maintainer approval and all CI gates passing.
  See [`GOVERNANCE.md` § Decision Making](GOVERNANCE.md) for the maintainer-approval requirement.

PRs that sit without author response for 30+ days may be closed with a friendly note inviting
reopen when the author is ready.

### Security reports

**Always go through the private disclosure channel** described in [`../../SECURITY.md`](../../SECURITY.md) —
do not file public issues for security findings.

Security-report cadences are tighter than the general-issue cadences above:

- **Initial acknowledgement**: within ~72 hours.
- **Severity assessment**: within ~1 week of acknowledgement.
- **Fix / coordinated disclosure plan**: depends on severity (per the response-phase tree in
  [`INCIDENT-RESPONSE.md`](INCIDENT-RESPONSE.md)); the reporter is kept in the loop throughout.

If a security report does not receive an acknowledgement within 72 hours, the reporter should
escalate per the escalation path in [`../../SECURITY.md`](../../SECURITY.md).

### How these cadences evolve

- **At v0.5** (the "external-tester ready" milestone), maintainers will reassess these cadences
  against actual incoming volume and either reaffirm them, tighten them, or formalize them into a
  documented SLO. The v0.5 gate is the natural review point because that's when the audience widens
  beyond the v0.1.x invited-installer base.
- **At v1.0** (SemVer stability), the bar tightens again. The v1.0 release notes will document
  the v1.x-line response commitments.

Until then: the cadences above are the working targets, not promises.

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
