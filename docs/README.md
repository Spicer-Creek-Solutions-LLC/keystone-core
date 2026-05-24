# Keystone Core — Documentation

This directory is the canonical home for Keystone Core's
documentation. Until the Hugo doc site lands (planned for v0.5; see
[`project/ROADMAP.md`](project/ROADMAP.md)), everything is rendered
Markdown — organized by audience and intent rather than by tool.

The top-level [`README.md`](../README.md) is the project front door
(topology, quickstart, the public-hosting story). This file is the
**doc-tree front door** — a where-to-start map for the directories
below.

## Where to start (by intent)

**I want to install Keystone Core on a fresh VM.**
→ [`project/GETTING-STARTED.md`](project/GETTING-STARTED.md) — the
guided ~30-minute tutorial. Or, for the dense reference walkthrough
(package layout, postinst behavior, filesystem map), jump straight to
[`runbooks/bootstrap-new-cluster.md`](runbooks/bootstrap-new-cluster.md).

**I want to develop on the source.**
→ [`project/DEVELOPMENT.md`](project/DEVELOPMENT.md). Covers the
docker-compose dev topology (`make e2e-up`), the build / test
workflow, the contribution conventions, and where reference docs
auto-generate from.

**I'm responding to an incident or running an operation.**
→ [`runbooks/`](runbooks/) — twelve scenario-specific procedures
(bootstrap, upgrade, backup, restore, DR, certificate rotation,
security incident, performance triage, …). See
[`runbooks/README.md`](runbooks/README.md) for the index.

**I need a reference (CLI flags, config keys, API endpoints).**
→ The three auto-generated references under [`project/`](project/):

- [`project/CLI-REFERENCE.md`](project/CLI-REFERENCE.md) — every
  `kscore-*` binary + subcommand (auto-gen from `cmd/`)
- [`project/CONFIGURATION-REFERENCE.md`](project/CONFIGURATION-REFERENCE.md)
  — every config key (auto-gen from struct tags)
- [`project/API-REFERENCE.md`](project/API-REFERENCE.md) — every
  gRPC RPC + REST endpoint (auto-gen from `.proto` + OpenAPI)

All three regenerate via `make docs-sync`; edit the source and
regenerate, don't hand-edit the references.

**I'm trying to understand a design decision or a project policy.**
→ [`project/README.md`](project/README.md) is the curated index of
everything under `docs/project/` (design, security, governance,
testing, lifecycle). [`adr/`](adr/) holds the formal Architectural
Decision Records.

## Directory map

| Subtree | What's inside | Index |
|---|---|---|
| [`project/`](project/) | 30 markdown files: design, reference, security, governance, testing, lifecycle | [`project/README.md`](project/README.md) |
| [`runbooks/`](runbooks/) | 12 operational runbooks for day-2 procedures | [`runbooks/README.md`](runbooks/README.md) |
| [`adr/`](adr/) | Architectural Decision Records + template | [`adr/README.md`](adr/README.md) |

## Doc surface conventions

- Headline operator docs (`GETTING-STARTED.md`, the runbooks) are
  pinned to **v0.1 reality**: the install path is `apt install` /
  `dnf install` of the operator-distributed `.deb` / `.rpm` packages
  followed by `kscorectl` for CLI interaction. They do **not** lean
  on the docker-compose dev harness or grpcurl — those live in
  [`project/DEVELOPMENT.md`](project/DEVELOPMENT.md) as contributor
  tooling.
- Each v0.x doc carries a scope note where v0.1 reality narrows the
  full v1.0 surface. The Hugo site will eventually replace these
  with proper versioned navigation; until then, the inline notes are
  the source of truth.
- The Hugo doc site is planned for v0.5 (see
  [`project/ROADMAP.md`](project/ROADMAP.md) § Hugo docs site). Until
  then, all paths in this tree are stable URLs you can deep-link from
  external docs, issues, and discussions.
