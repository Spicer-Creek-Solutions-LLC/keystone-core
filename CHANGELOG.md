# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.1.0] — Unreleased

First public release of the post-reset codebase. Linux-only, internal-quality. Closes epics 01–08. See [`docs/project/VERSIONING.md`](docs/project/VERSIONING.md) for the milestone scheme (`v0.1` → `v0.5` external-tester gate → `v1.0` SemVer stability gate). Implementation tracked in [`epics/`](epics/); ranked backlog in [`docs/project/ROADMAP.md`](docs/project/ROADMAP.md). The prior implementation is preserved on the `archive/v0` branch.

### Notable behavior — the policy engine is AUDIT-MODE-ONLY

> **Read this before relying on policy.** The policy engine evaluates, audits,
> and reports — but **it does not block anything**. Even when a policy returns
> `Allowed=false`, the operation **still proceeds**. `policy.enforcement_enabled`
> is hardcoded `false` and is not operator-settable on the `v0.x → v1.0` line.

This is intentional: full enforcement carries breaking-change risk (a
misconfigured policy could block the fleet), so v1.0 ships policy as
*observability* — run real policies against real workloads, inspect what
*would* have been blocked via the audit trail / `WouldDeny`, and build
confidence first. **v1.8 flips enforcement on and is a behavior-changing
release** — policies left at `EnforcementMode=enforce` will start blocking.
See [`docs/project/POLICY-AUDIT.md`](docs/project/POLICY-AUDIT.md) for the full
audit-mode contract and the v1.0 → v1.8 migration steps.

The audit log itself is fully live in v1.0: every sensitive op (auth, secret
access, command exec, state apply, policy eval) writes an `AuditEntry`.

### Breaking changes

This is the v0.x line — breaking changes between minor versions are expected. Each future release will list its breaking changes under a `## Breaking` section here, with a migration note in the release announcement, and (when possible) a one-cycle deprecation period.
