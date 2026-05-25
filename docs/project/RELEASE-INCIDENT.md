# Release-incident response

This document covers what to do when a published Keystone Core
release reveals a critical problem **after** it has been distributed
to operators — bugs, broken installs, accidental data loss,
security vulnerabilities that bypassed pre-release review, or any
other "the released artifact is harmful, what now?" scenario.

It is the post-publication counterpart to
[`../../RELEASE-PLAYBOOK.md`](../../RELEASE-PLAYBOOK.md), which
covers the *release ceremony itself*. For the expedited-release
mechanics (reduced quorum, scoped dependency audit, signing
requirements), see
[`RELEASE-PLAYBOOK.md` § 14 — Emergency and Patch Releases](../../RELEASE-PLAYBOOK.md#14-emergency-and-patch-releases).

For security incidents in *operator deployments* (compromised
agent, compromised control plane, data breach, supply chain
attack), see [`INCIDENT-RESPONSE.md`](INCIDENT-RESPONSE.md) — that
document covers production incident response, this one covers
"the release artifact itself is the problem."

## When to use this document

Reach for this document when:

- A released `kscore-*` binary or package fails to install / start /
  function correctly for at least one operator who has reported the
  failure.
- A security vulnerability is reported against a released version
  (after the security report is acknowledged per
  [`MAINTAINERS.md` § Triage and Response](MAINTAINERS.md)).
- A released version produces incorrect behavior that could damage
  operator data, state, or trust (e.g., a state-apply that deletes
  unintended resources, an audit-log path that silently drops events).
- A released signed artifact is later found to have been tampered
  with or signed under compromised key material.

For routine bug reports against a released version that don't meet
the bar above, the standard issue + patch-release cadence applies;
this document is not invoked.

## v0.1.x note

The v0.1.x line ships under the soft-launch posture
(see [`GOVERNANCE.md` § Launch Posture](GOVERNANCE.md)). `.deb` /
`.rpm` packages are **operator-distributed** rather than published
to a public APT/DNF repo — the public package repo arrives with the
v1.0 release ceremony. This shapes the response procedures:

- "Yank" at v0.1.x means **withdraw the distribution channel** the
  affected operators received the package through (release-page
  download, signed tarball, direct file transfer). There is no public
  package repo to issue a fast `apt-get update; apt-get upgrade` to.
- The known-operator list is small (per soft-launch posture) — direct
  notification by email or chat is the primary communication path,
  not a public advisory aggregator.
- Each affected operator gets a personal heads-up; broad public
  communication is calibrated to the soft-launch audience.

These constraints relax at v0.5 (wider tester audience, possibly
package repo) and again at v1.0 (full public distribution).

## Decision tree

```text
Critical problem reported in a released version
                │
                ▼
       Is the artifact actively
       causing harm right now?
                │
        ┌───────┴───────┐
       yes              no
        │                │
        ▼                ▼
   Yank + fast      Patch + supersede
   follow-up        + standard advisory
   release          (next minor / patch cycle)
        │                │
        └───────┬────────┘
                ▼
       Public communication
       (calibrated to launch posture)
                │
                ▼
       Post-incident steps
       (CHANGELOG, post-mortem,
        process improvements)
```

The "actively causing harm right now" judgement criteria:

- **Yes (yank + fast follow-up)**: data loss, security vulnerability
  exploitable in default configuration, install failures preventing
  operator from getting to a working state, behavior that bypasses
  documented safety invariants (e.g., a policy-engine bypass).
- **No (patch + supersede)**: bug that requires unusual configuration
  to trigger, incorrect behavior with a documented workaround, perf
  regression, doc-only correctness issues.

When in doubt, treat as "yes" — the cost of an unnecessary yank is
low (one fast follow-up release); the cost of leaving an actively-
harmful artifact in circulation is much higher.

## Procedure 1 — Yank a released version

Apply when the decision tree says "actively causing harm".

### Step 1 — Halt distribution

Pull the affected artifact from every distribution channel it
reached:

- **Codeberg release page**: edit the release to mark it as a
  pre-release with a clear `**YANKED — DO NOT USE**` notice in the
  release notes; **do not** delete the underlying tag (history must
  remain auditable). The release-asset files (`.deb`, `.rpm`,
  tarballs, checksums, signatures) stay attached so existing
  references resolve, but the page makes the status unambiguous.
- **GitHub mirror**: mirror the same yank notice into the GitHub
  release page so discoverability-via-mirror picks up the warning.
- **Direct-distribution recipients**: contact each known operator
  who received the artifact directly (per
  [`MAINTAINERS.md`](MAINTAINERS.md) operator list).
- **Container images** (if affected): do not delete the registry
  tag; add a `LABEL kscore.yanked=true` to a new image at the same
  tag pointing at a stub that prints the yank notice on start.
  Container registries vary in tag-immutability; if the registry
  enforces immutable tags, push to a new tag and update the docs
  to reflect the move.

### Step 2 — Audit who received the affected artifact

For v0.1.x: small known-operator list. Pull from operator
distribution records and direct any internal comms (email, chat,
issue threads) to confirm who downloaded or was sent the artifact.

For v0.5+ (public repo): the package repo's access logs are the
authoritative answer; document the count + geographic distribution
in the release record.

### Step 3 — Cut the yank advisory

Open a public advisory in the project's security advisory channel
(`SECURITY.md` describes the disclosure pipeline; the channel
extends to non-security release incidents too). The advisory
includes:

- The affected version(s).
- The nature of the problem in one paragraph; the specific failure
  mode operators may observe.
- The decision (yanked + fast follow-up coming, with ETA if known).
- Workaround if any, even temporary.
- Whether to roll back to the prior version, hold, or wait for the
  follow-up.
- A pointer to where the follow-up release will be announced.

Cross-reference the advisory in: the yanked release page notice,
the project's release records (`docs/release-records/v<ver>.md`),
and the next-release CHANGELOG entry.

## Procedure 2 — Fast follow-up release

Apply after yanking (Procedure 1), or as a standalone path when the
problem is "harm in the field" but not "actively causing harm right
now" (e.g., a bug that's bad enough to fix promptly but isn't
silently corrupting data).

### Numbering

- **If the affected version has not yet been depended on by anyone
  in production-shaped use**: ship an `-rc2` (e.g., `v0.1.0` →
  `v0.1.0-rc2`). The `-rc` suffix communicates that the prior label
  was withdrawn.
- **If anyone is running the affected version in earnest**: ship a
  patch bump (e.g., `v0.1.0` → `v0.1.1`). Operators can `apt install
  ./kscore-server_0.1.1_*.deb` over the affected version.
- **If the problem requires breaking changes to fix safely**: ship
  a minor bump (e.g., `v0.1.0` → `v0.2.0`) and document the
  breaking change in the advisory + CHANGELOG. Breaking minors
  are explicitly allowed in v0.x per
  [`VERSIONING.md`](VERSIONING.md).

### Release-ceremony procedure

The release ceremony itself follows the expedited path in
[`RELEASE-PLAYBOOK.md` § 14](../../RELEASE-PLAYBOOK.md#14-emergency-and-patch-releases) —
same single-signer dual-machine reproducibility check, same signing
keys, same publication phase. The accommodations §14 documents
(reduced quorum at v1.2+, scoped dependency audit, etc.) apply
identically here.

The only addition for a yank-driven release is the **cross-reference
back to the yank advisory** in:

- The release-record file (`docs/release-records/v<ver>.md`):
  document the prior version was yanked and link the advisory.
- The CHANGELOG entry: lead with the yank/fix framing
  (`### Fixed — supersedes the yanked v<prior>`).
- The public release notes on the codeberg release page.

### Communicating availability

Once the follow-up release is published and signature-verified:

- Update the original yank advisory with the follow-up version + a
  pointer to it.
- Send direct notification to every operator on the affected-version
  list (Procedure 1 step 2) confirming the fix and the upgrade path.
- Update the project's primary docs (`README.md` Quickstart already
  uses `*` glob versions so it doesn't need a number bump; the
  release-page links will follow the new version automatically).

## Procedure 3 — Communicate-only (no yank, no fast release)

Apply when the affected behavior is real but doesn't meet the
"actively causing harm" bar — typical for bugs with a documented
workaround, performance regressions, or doc-correctness issues that
mislead but don't damage.

Steps:

1. Open a public advisory (same channel as Procedure 1 step 3)
   describing the issue, the affected versions, the workaround, and
   the fix-version + ETA.
2. File the fix under the appropriate ROADMAP bucket (`gate-v0.5` /
   `gate-v1.0` / `v0.x` per [`ROADMAP.md`](ROADMAP.md)) so the work
   is tracked.
3. The fix ships in the next routine release; the CHANGELOG entry
   references the advisory.

No yank. No emergency release.

## Public communication

The communication channel set scales to the launch posture:

- **v0.1.x (soft launch)**: direct email/chat to the known-operator
  list is primary. Codeberg release-page notice + advisory channel
  is the public-record secondary. Do not post to aggregators
  (HN, Lobsters, mailing lists) without an explicit reason — the
  audience is invited.
- **v0.5+ (external testers)**: known-tester list + public advisory
  channel + project mailing list (if one exists by then). Aggregator
  posting still gated on severity + reach.
- **v1.0+ (broad public)**: full public advisory cadence including
  aggregators, downstream package-repo advisories, CVE assignment
  when applicable, and (for security incidents)
  [`INCIDENT-RESPONSE.md`](INCIDENT-RESPONSE.md) § Customer
  Notification template.

For all severity levels and posture levels, the advisory wording
follows the project's
[`SECURITY.md`](../../SECURITY.md) disclosure conventions —
clear, specific, no marketing softening, no blame externalization.

## Post-incident steps

Within two weeks of the follow-up release shipping (or the
communicate-only advisory being posted):

1. **CHANGELOG hygiene.** Confirm the affected + superseding version
   entries cross-reference each other. The yanked-version CHANGELOG
   entry (if one existed) is annotated `**YANKED — see v<follow-up>**`.
2. **Post-mortem.** Write a blameless post-mortem covering: the bug,
   the gap that let it past pre-release review, the detection signal
   that brought it to attention, the response timeline, and at least
   one process change to reduce recurrence likelihood. Lands in
   `docs/release-records/v<ver>-postmortem.md` (or appended to the
   release record).
3. **Process change.** The "at least one process change" is the
   load-bearing output. Examples: a test that would have caught the
   bug pre-release, a release-checklist item, a CI gate, a doc
   update. File it as a normal ROADMAP entry tagged with the
   incident reference. *Don't write process changes you don't intend
   to do — an unhonored process item is worse than no process item.*
4. **Lessons-learned summary.** Add a one-paragraph entry to
   `CHANGELOG.md` under the next release's `### Fixed` describing
   what the incident taught and which process changed.

## Anti-patterns to avoid

- **Deleting the affected tag from git history.** Auditability
  matters more than tidy history; the yank notice on the release
  page is the right signal, not tag removal.
- **Silently re-tagging.** Force-pushing a new commit at the same
  tag breaks downstream signature verification (the tag's commit
  hash is part of the signed release record). Always cut a new
  version.
- **Skipping the advisory.** Even for small-audience yanks, the
  public advisory is the durable record. Direct comms are
  *additional*, not *alternative*.
- **Letting the post-mortem slip past two weeks.** The further from
  the incident, the less honest the retrospective. Two weeks is the
  outer bound, not a target.
- **Process-change theater.** A new checklist item that nobody
  reads, or a CI gate that's marked optional, is worse than no
  process change. Pick one thing that will actually run.

## Related documents

- [`../../RELEASE-PLAYBOOK.md`](../../RELEASE-PLAYBOOK.md) — the
  routine release ceremony; § 14 covers expedited / patch release
  mechanics this document hands off to.
- [`INCIDENT-RESPONSE.md`](INCIDENT-RESPONSE.md) — production
  security-incident response for compromised agents / control
  planes / data breaches (different scope from this document).
- [`MAINTAINERS.md`](MAINTAINERS.md) § Triage and Response — the
  general triage cadences this document accelerates for yank-grade
  incidents.
- [`GOVERNANCE.md`](GOVERNANCE.md) § Launch Posture — the v0.1.x
  soft-launch framing that shapes the communication procedures
  above.
- [`../../SECURITY.md`](../../SECURITY.md) — vulnerability
  reporting entry point; the disclosure pipeline this document's
  advisories flow through.
- [`VERSIONING.md`](VERSIONING.md) — the v0.x → v0.5 → v1.0
  ladder + the breaking-changes-allowed-in-v0.x policy that
  affects fast-follow-up version numbering.
