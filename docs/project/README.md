# docs/project/ — project documentation

This directory holds the canonical project documentation: design,
reference, security, governance, testing, and lifecycle. Files are
named in upper-kebab-case so they sort naturally and read like a
table of contents in a directory listing.

For the doc-tree front door (where-to-start by intent), see
[`../README.md`](../README.md). For operational runbooks (day-2
procedures), see [`../runbooks/`](../runbooks/). For formal
architectural decision records, see [`../adr/`](../adr/).

## Operator entry points

- [`GETTING-STARTED.md`](GETTING-STARTED.md) — guided ~30-minute
  fresh-VM walkthrough: install → run a command → apply state →
  browse the audit log.
- [`GLOSSARY.md`](GLOSSARY.md) — terminology used throughout the
  project (agent, blueprint, runbook, saga, …).

## Reference (auto-generated — regenerate with `make docs-sync`)

- [`CLI-REFERENCE.md`](CLI-REFERENCE.md) — every `kscore-*` binary +
  every subcommand. Every `kscore-<name>` operator binary is also
  reachable as `kscorectl <name>` via plugin dispatch.
- [`CONFIGURATION-REFERENCE.md`](CONFIGURATION-REFERENCE.md) — every
  config key in `server.yaml` and `agent.yaml`, with type +
  description.
- [`API-REFERENCE.md`](API-REFERENCE.md) — every gRPC RPC + REST
  endpoint, with links to the canonical proto / OpenAPI sources.

## Design + architecture

- [`DESIGN.md`](DESIGN.md) — high-level architecture.
- [`PROBLEM-STATEMENT.md`](PROBLEM-STATEMENT.md) — why this project
  exists, what it is (and isn't).
- [`COMPATIBILITY.md`](COMPATIBILITY.md) — supported platforms,
  upstream version pins, deprecation policy.

## Security

- [`SECURITY-DESIGN.md`](SECURITY-DESIGN.md) — the security
  architecture (identity, HMAC, audit, secrets).
- [`SECURITY-GOVERNANCE.md`](SECURITY-GOVERNANCE.md) — the v1.0
  four-scan baseline + vulnerability disclosure process.
- [`SECURITY-RELEASE.md`](SECURITY-RELEASE.md) — security advisory +
  release-coordination workflow.
- [`SECURITY-REVIEW.md`](SECURITY-REVIEW.md) — review checklist for
  security-relevant changes.
- [`HARDENING-BASELINE.md`](HARDENING-BASELINE.md) — the measured
  v1.0 hardening posture.
- [`POLICY-AUDIT.md`](POLICY-AUDIT.md) — the audit-mode-only policy
  operator guide; enforcement-mode migration is **post-v1.0**.

The vulnerability-reporting entry point is [`../../SECURITY.md`](../../SECURITY.md)
at the repo root.

## Governance + process

- [`GOVERNANCE.md`](GOVERNANCE.md) — BDFL + maintainer model, RFC
  process, **launch posture**.
- [`MAINTAINERS.md`](MAINTAINERS.md) — current maintainers.
- [`RFC.md`](RFC.md) — RFC procedure for design proposals.
- [`DCO.md`](DCO.md) — Developer Certificate of Origin policy +
  sign-off requirement.
- [`AI-CONTRIBUTIONS.md`](AI-CONTRIBUTIONS.md) — AI-assisted
  contribution policy.
- [`ISSUE-TRACKING.md`](ISSUE-TRACKING.md) — labels, milestones,
  tracker-issue conventions.

## Testing + quality

- [`TEST-POLICY.md`](TEST-POLICY.md) — `-race`, `goleak`, build tags,
  the test target matrix.
- [`COVERAGE-GATES.md`](COVERAGE-GATES.md) — per-package coverage
  thresholds.
- [`E2E-VM-TESTING.md`](E2E-VM-TESTING.md) — VM-based E2E testing
  (all-in-one, HA, IPv6, performance).
- [`PROFILING-BASELINE.md`](PROFILING-BASELINE.md) — measured CPU /
  allocation baseline.

## Development + incident response

- [`DEVELOPMENT.md`](DEVELOPMENT.md) — build, test, contribute. The
  **Local Dev Topology** section covers the docker-compose dev
  harness for ad-hoc iteration.
- [`INCIDENT-RESPONSE.md`](INCIDENT-RESPONSE.md) — internal incident-
  response procedure (distinct from the operator runbooks under
  [`../runbooks/`](../runbooks/)).

## Project lifecycle

- [`VERSIONING.md`](VERSIONING.md) — the v0.x → v0.5 → v1.0 release
  ladder + gate checklists.
- [`ROADMAP.md`](ROADMAP.md) — ranked backlog (gate-v0.5,
  gate-v1.0, v0.x, v1.x, v2.x+).
- [`PUBLIC-LAUNCH-CHECKLIST.md`](PUBLIC-LAUNCH-CHECKLIST.md) — the
  pre-launch quality gate checklist (Phases A–G).

## Related directories

- [`../runbooks/`](../runbooks/) — twelve operational runbooks for
  day-2 procedures.
- [`../adr/`](../adr/) — formal Architectural Decision Records +
  template.
- [`../../epics/`](../../epics/) — the 19 reconstruction epics, in
  dependency order. Epic acceptance criteria are the authoritative
  v0.1.0 scope.
