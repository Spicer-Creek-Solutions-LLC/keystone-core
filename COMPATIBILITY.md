# Keystone Core — Compatibility & Support Policy

Keystone Core releases follow a predictable schedule and compatibility model to make upgrades safe,
reduce surprises, and keep the project moving forward without accumulating legacy baggage.

This document explains how releases, support windows, and compatibility boundaries work from the
perspective of operators, users, and plugin authors.

---

## Release Cadence

Keystone Core makes a new release approximately **every 6 months**:

- **Spring Release**
- **Fall Release**

Features are not delayed for calendar reasons. If a feature is ready, it ships. If it isn't, it
waits for the next release.

---

## Versioning (SemVer)

Releases use **Semantic Versioning** to communicate the type of changes included:

- **MAJOR** version bump — Breaking changes
- **MINOR** version bump — New features (no breaking changes)
- **PATCH** version bump — Bug fixes & improvements only

Examples:

| Type of Change     | Example Next Version |
|--------------------|----------------------|
| Breaking change    | `2.0.0`              |
| Feature (no break) | `1.3.0`              |
| Fixes only         | `1.2.4`              |

SemVer is a **communication mechanism**, not a guarantee of indefinite backwards compatibility.

---

## Support Window

Each release receives support for **2 years**, which corresponds to roughly **4 releases** in total.

Support includes:

- Security fixes
- Critical bug fixes
- Upgrade support
- Migration support (schema, agent, config)

After 2 years, a release is considered **End of Support** and may stop working with newer releases.

---

## Upgrade Path

Upgrades are designed to be predictable and incremental.

You may upgrade:

- To the **next release** (e.g., Spring → Fall)
- Or **one release further** (e.g., Spring → next Spring)

You may **not skip 3+ releases**. If you do, you must hop through an intermediate release.

Example:
```
A → B (OK)
A → C (OK)
A → D (NOT SUPPORTED; upgrade via B or C)
```

This avoids brittle "big bang" migrations and keeps the project healthy.

---

## Controller ↔ Agent Compatibility

Controllers and agents do **not** need to be on the exact same version.

General rules:

- Controllers may be **newer** than agents
- Agents should not be significantly newer than controllers
- Very old agents (beyond the support window) may refuse to connect

The goal is to allow rolling upgrades without downtime.

---

## Schema & State Compatibility

Schema upgrades happen during software upgrades, but availability is prioritized.

During a rolling upgrade:

- New nodes must operate safely using the **old schema**
- Final schema cleanup does not occur until all nodes are upgraded

This makes high availability upgrades possible without coordinating downtime.

---

## Configuration Compatibility

Configuration files follow these principles:

- Newer versions accept older config where feasible
- Deprecated fields generate warnings before removal
- Removal happens on a clear schedule (not silently)

This gives operators time to adjust without rush or ambiguity.

---

## Breaking Changes

Breaking changes (MAJOR versions) are permitted, but the project aims to keep them:

- **infrequent**
- **well-documented**
- **easy to migrate**

As the project matures, we expect breaking changes to become rarer, similar to the Go 1 compatibility
promise.

---

## Plugin & Integration Expectations

Plugin authors and integrators can expect:

- 2-year compatibility window
- predictable upgrade cadence
- documented deprecations
- clear signals about breaking changes

This model encourages ecosystems without locking the project down forever.

---

## Goals of This Policy

This policy exists to provide:

- Predictability for operators
- Stability for plugin authors
- Flexibility for maintainers
- No surprise deadlines
- No accidental stagnation

It balances long-term evolution with short-term usability.

---

## Questions & Feedback

Feedback is welcome as the project evolves. The goal is to deliver a platform that is dependable,
well-governed, and pleasant to operate — not one encumbered by legacy or surprise breaking changes.


