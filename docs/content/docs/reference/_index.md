---
title: Reference
weight: 1
cascade:
  # Title + sidebar weight for each mounted docs/project page — kept here
  # (a Hugo-side map) so the canonical files under docs/project/ stay
  # pristine. Weights mirror the intent grouping in docs/project/README.md.
  # Operator entry points (10s)
  - {_target: {path: '/docs/reference/getting-started'}, title: "Getting Started", weight: 11}
  - {_target: {path: '/docs/reference/glossary'}, title: "Glossary", weight: 12}
  - {_target: {path: '/docs/reference/problem-statement'}, title: "Problem Statement", weight: 13}
  # Auto-generated reference (20s)
  - {_target: {path: '/docs/reference/cli-reference'}, title: "CLI Reference", weight: 21}
  - {_target: {path: '/docs/reference/configuration-reference'}, title: "Configuration Reference", weight: 22}
  - {_target: {path: '/docs/reference/api-reference'}, title: "API Reference", weight: 23}
  # Design + architecture (30s)
  - {_target: {path: '/docs/reference/design'}, title: "Design", weight: 31}
  - {_target: {path: '/docs/reference/compatibility'}, title: "Compatibility", weight: 32}
  - {_target: {path: '/docs/reference/versioning'}, title: "Versioning", weight: 33}
  # Security (40s)
  - {_target: {path: '/docs/reference/security-design'}, title: "Security Design", weight: 41}
  - {_target: {path: '/docs/reference/security-governance'}, title: "Security Governance", weight: 42}
  - {_target: {path: '/docs/reference/security-release'}, title: "Security Release", weight: 43}
  - {_target: {path: '/docs/reference/security-review'}, title: "Security Review", weight: 44}
  - {_target: {path: '/docs/reference/hardening-baseline'}, title: "Hardening Baseline", weight: 45}
  - {_target: {path: '/docs/reference/policy-audit'}, title: "Policy & Audit", weight: 46}
  # Governance + process (50s)
  - {_target: {path: '/docs/reference/governance'}, title: "Governance", weight: 51}
  - {_target: {path: '/docs/reference/maintainers'}, title: "Maintainers", weight: 52}
  - {_target: {path: '/docs/reference/rfc'}, title: "RFC Process", weight: 53}
  - {_target: {path: '/docs/reference/dco'}, title: "DCO", weight: 54}
  - {_target: {path: '/docs/reference/ai-contributions'}, title: "AI Contributions", weight: 55}
  - {_target: {path: '/docs/reference/issue-tracking'}, title: "Issue Tracking", weight: 56}
  # Testing + quality (60s)
  - {_target: {path: '/docs/reference/test-policy'}, title: "Test Policy", weight: 61}
  - {_target: {path: '/docs/reference/coverage-gates'}, title: "Coverage Gates", weight: 62}
  - {_target: {path: '/docs/reference/e2e-vm-testing'}, title: "E2E VM Testing", weight: 63}
  - {_target: {path: '/docs/reference/profiling-baseline'}, title: "Profiling Baseline", weight: 64}
  # Development + incident response (70s)
  - {_target: {path: '/docs/reference/development'}, title: "Development", weight: 71}
  - {_target: {path: '/docs/reference/incident-response'}, title: "Incident Response", weight: 72}
  - {_target: {path: '/docs/reference/release-incident'}, title: "Release Incident", weight: 73}
  # Project lifecycle (80s)
  - {_target: {path: '/docs/reference/roadmap'}, title: "Roadmap", weight: 81}
  - {_target: {path: '/docs/reference/public-launch-checklist'}, title: "Public Launch Checklist", weight: 82}
  - {_target: {path: '/docs/reference/codeberg-settings-audit'}, title: "Codeberg Settings Audit", weight: 83}
  # The mounted README duplicates this section landing — hide it.
  - {_target: {path: '/docs/reference/readme'}, sidebar: {exclude: true}, weight: 99}
---

Canonical project documentation — design, configuration, CLI/API
reference, security, governance, testing, and lifecycle. The CLI,
configuration, and API references are auto-generated (`make docs-sync`);
edit the generators, not the rendered pages.
