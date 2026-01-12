---
title: "Compatibility & Support Policy"
weight: 6
description: >
  Release compatibility, support windows, upgrade paths, and technical guidelines for operators and developers
---

# Keystone Core — Compatibility & Upgrade Technical Guidelines

This document describes the technical rules, invariants, and implementation mechanics behind the
Keystone Core compatibility and upgrade model. The focus is on maintainability, correctness, and
operational safety.

## Core Design Principles

1. Upgrade Safety Comes First
2. Breaking Changes Are Allowed, Not Free
3. Compatibility Is Time-Bounded (Not Forever)
4. Configuration Should Be Forward-Compatible
5. Schema Changes Use Expand → Migrate → Contract
6. Controller May Lead Agents During Rolling Upgrades
7. Surface Area Freezes Over Time (Maturity Path)

## Versions & Release Lines

Each release line is tracked via SemVer:

MAJOR.MINOR.PATCH

With a 6-month release cadence, the support window spans 4 active release lines.

Developers must consider compatibility impacts across those lines.

## Compatibility Boundaries (Invariants)

Keystone enforces compatibility across three main surfaces:

1. RPC / Protocol Compatibility
2. Schema / Storage Compatibility
3. Configuration Compatibility

Breaking any of these surfaces requires a MAJOR release.

## Controller ↔ Agent Negotiation

On connect, the agent provides:

- agent_version (SemVer)
- supported_protocols (vector)
- schema_version
- capabilities

Controller chooses:

- highest compatible protocol
- compatibility mode
- or rejects with a clear diagnostic

Rules:

- Controller may be newer than agent
- Agent should not be significantly newer than controller
- Compatibility window = current release + previous 2 releases (adjustable)

Agents older than support window may be:

- rejected
- put in legacy/read-only mode
- or scheduled for upgrade

## Schema Migration Strategy

Schema upgrades use a three-phase model:

1. Expand
2. Migrate
3. Contract

Metadata stored in schema_meta with version and mode (legacy|mixed|new).

## State Migration Responsibility

Migration executes during software upgrade, not runtime.

Goals: deterministic, restart-safe, observable, operator-controllable.

Failures must be reversible and resumable.

## Configuration Compatibility

Configs must be forward-compatible.

Deprecated fields must warn for ≥2 releases.

Removal requires a MAJOR or end-of-support window.

## Breaking Changes (MAJOR)

Breaking changes include:

- protocol removal
- RPC removal
- schema removal
- config removal
- operator-impacting behavior shifts
- CLI/UX surface changes

A MAJOR bump requires migration notes and tooling.

## Release-Time Responsibilities

For each release, devs must provide:

- release notes
- migration notes
- schema diffs
- config deprecation diffs
- agent compatibility statement
- controller compatibility statement

## CI/Testing Considerations

CI must verify:

- rolling upgrade
- agent compatibility
- schema correctness (expand/migrate/contract)
- downgrade safety (where appropriate)
- config deprecation warnings

Test matrix:

current
current-1
current-2
current-3

## Maturity & Future Compatibility Promise

As the platform stabilizes, a Go-like compatibility promise may be adopted once surfaces mature.

## Design Goals

- avoid stagnation
- avoid legacy burden
- support ecosystems
- provide operational safety
- maintain internal design flexibility
- keep codebase refactorable
