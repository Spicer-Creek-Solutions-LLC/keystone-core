---
title: "Executive Summary"
linkTitle: "Executive Summary"
weight: 1
description: >
  A high-level overview of Keystone Core for decision makers
---

## What Is Keystone Core?

**Keystone Core** is a cloud-native infrastructure control plane that bridges the gap between GitOps/IaC deployment tools and runtime operations.

**Positioning**: "GitOps deploys it. We keep it running."

## The Problem It Solves

Modern infrastructure uses tools like Terraform and ArgoCD to *deploy* systems, but lacks unified tooling to *operate* them after deployment. Keystone Core provides:

- **Remote execution** across heterogeneous infrastructure (Kubernetes, VMs, bare metal, edge devices)
- **Declarative state management** with drift detection and remediation
- **Event-driven automation** for operational responses
- **Policy enforcement** for continuous compliance
- **Unified observability** across all managed systems

## Key Differentiators

1. **Zero-dependency deployment** - Embedded NATS + SQLite means single-binary startup
2. **Scales gracefully** - Migrate to external NATS cluster + PostgreSQL as you grow
3. **Multi-platform** - Linux, macOS, Windows; Kubernetes, VMs, bare metal, edge
4. **Pure Go** - No CGO dependencies, simple cross-compilation
5. **Security-first** - Capability-based plugin sandboxing, SPIFFE identity, policy enforcement

## Technology Stack

| Component | Technology |
|-----------|------------|
| Language | Go 1.25+ |
| Messaging | NATS with JetStream |
| Storage | SQLite (embedded) or PostgreSQL (production) |
| Plugins | Starlark + WebAssembly (wazero) |
| Policy | OPA (Rego) + CEL |
| Observability | Prometheus, OpenTelemetry, Grafana |

## Current Status

Epics 1-29 complete:

| Category | Status |
|----------|--------|
| Core Infrastructure | Complete |
| Remote Execution | Complete |
| State Management | Complete (94 cross-platform modules) |
| Event System | Complete |
| GitOps Integration | Complete |
| Policy Engine (OPA/CEL) | Complete |
| Observability | Complete |
| High Availability | Complete (etcd-based clustering) |
| Plugin System | Complete (Starlark + WASM) |
| Documentation | Complete |
| SPIFFE Identity | Complete |
| Windows Support | Complete |

### Test Coverage

| Test Type | Count |
|-----------|-------|
| **Total Test Functions** | 5,068 |
| Unit Tests (pkg/) | 4,625 |
| CLI Tests (cmd/) | 243 |
| E2E Tests | 162 |
| Bootstrap Tests (test/bootstrap) | 17 |
| Benchmark Functions | 51 |
| Subtests (t.Run) | 1,100 |
| Test Files | 349 |

Counts are derived from repository-wide `*_test.go` scans (e.g., `rg -o "^func\\s+Test"` and `rg -o "\\bt\\.Run\\("`).

## Remaining Roadmap

### Planned Epics

- **Epic 30**: CLI UX Restructuring
- **Epic 31**: NIST Design Principles - documentation only

### Future Considerations

- **0.1.0 Release Readiness** - Blueprint signing, version reset, docs audit, VM validation
- **Simplification** - Aggressive refactor to minimize code and surface area

| Category | Description |
|----------|-------------|
| **Multi-Tenancy** | Namespace isolation, per-tenant RBAC/quotas, SSO integration (OIDC/SAML) |
| **Scheduled Operations** | Centralized job scheduler, maintenance windows, batch scheduling |
| **Web UI** | Web-based dashboard, topology visualization, state editor |
| **Automatic Drift Remediation** | Opt-in auto-fix, approval workflows, change management integration |
| **Agent Self-Update** | Secure binary distribution, staged rollouts, automatic rollback |
| **Compliance Presets** | CIS Benchmarks, SOC 2, HIPAA, PCI-DSS policy packs |
| **Network Discovery** | Automatic scanning, L2/L3 mapping, dependency visualization |
| **Advanced State Orchestration** | Statecharts, workflows, actors, event sourcing, saga coordination |
| **Runbook Automation** | Multi-step orchestration, conditional branching, approval gates |
| **Disaster Recovery** | Full backup/restore, state export/import, cross-region failover |
| **Secrets Management** | HashiCorp Vault, AWS Secrets Manager, Azure Key Vault, GCP Secret Manager |
| **Terraform Provider** | Terraform provider for Keystone Core resources |
| **ITSM Integration** | ServiceNow integration, change request workflows, CMDB sync |
| **Migration Tools** | Chef/Puppet to Keystone Core converters |
| **Mobile Monitoring** | iOS/Android apps with push notifications |
| **Natural Language** | AI-powered natural language commands |

## Quick Links

- [Getting Started](/docs/getting-started/) - Install and deploy in 15 minutes
- [Architecture Overview](/docs/getting-started/architecture/) - System design and components
- [Roadmap](/docs/community/roadmap/) - Detailed development plans
