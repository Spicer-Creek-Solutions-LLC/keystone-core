# Keystone Core RFC Template

Use this template for proposing significant changes to Keystone Core, especially those that impact
compatibility, upgrades, or operator experience.

This is meant to be lightweight. The goal is clarity, not bureaucracy.

---

## RFC: (Short Title)

- **Author(s):** @your-handle
- **Date:** YYYY-MM-DD
- **Status:** Draft / Under Discussion / Accepted / Rejected / Superseded by #NN

---

## Summary

One or two paragraphs describing the proposal at a high level. Someone skimming this section should
understand what is being proposed and roughly why.

---

## Motivation

- What problem does this solve?
- Who is affected (operators, users, plugin authors, maintainers)?
- What happens if we do nothing?

Be explicit about real pain points or limitations.

---

## Design Overview

High-level description of the design:

- main ideas
- key components
- major flows

This should be concrete enough to understand how the system changes, without going into every line
of code.

---

## Detailed Design

More in-depth explanation of how this will be implemented. Include:

- data structures
- protocol changes
- APIs or interfaces
- configuration changes
- schema/storage changes

Use diagrams or examples if helpful.

---

## Compatibility & Upgrade Impact

**Required section.** Describe how this affects:

- existing configurations
- controller/agent compatibility
- schemas and stored data
- plugins and integrations
- rolling upgrades and downgrades

State clearly:

- Is this a breaking change?
- Does it require a MAJOR/MINOR bump?
- How long will deprecated behavior be supported?

Reference the compatibility and support policy where relevant.

---

## Migration Plan

If existing users are affected, outline:

- what needs to change
- whether migration can be automated
- how operators can roll this out safely
- whether partial upgrades are supported

If no migration is required, say so explicitly.

---

## Alternatives Considered

List serious alternatives and why they were rejected. This is important context for future readers.

---

## Security Considerations

Note any impact on:

- authentication and authorization
- PKI or trust model
- network exposure
- data confidentiality or integrity

If there is no meaningful impact, state that.

---

## Observability & Operations

Describe how this affects:

- logging
- metrics
- debugging
- failure modes

Operators should understand how to know if this is working or failing.

---

## Rollout & Adoption

How should this be rolled out? For example:

- behind a feature flag?
- enabled by default in new installs only?
- opt-in for a release, then opt-out, then removed?

---

## Open Questions

List any unresolved questions or areas where feedback is especially welcome.

---

## Prior Art

If relevant, link to similar approaches in other projects (e.g. Go, Kubernetes, Nomad, Consul,
Salt, etc.).

---

## Decision

To be filled in by maintainers/BDFL when the RFC is resolved.

- **Outcome:** Accepted / Rejected / Deferred
- **Notes:** Short rationale and any conditions or follow-ups.
