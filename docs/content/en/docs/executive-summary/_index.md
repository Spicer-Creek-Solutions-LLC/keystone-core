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

Epics 1-32 and 36-60 complete:

| Category | Status |
|----------|--------|
| Core Infrastructure | Complete (NATS embedded/external/leaf, SQLite/PostgreSQL, TLS 1.3 default) |
| Remote Execution | Complete |
| State Management | Complete (94 cross-platform modules) |
| Event System | Complete (outbound webhooks, HMAC signing, retry) |
| GitOps Integration | Complete |
| Policy Engine (OPA/CEL) | Complete |
| Observability | Complete |
| High Availability | Complete (etcd clustering, resilience tested) |
| Plugin System | Complete (Starlark + WASM) |
| SPIFFE Identity | Complete |
| Windows Support | Complete |
| Proxy Agents | Complete (8 protocols, 20 vendor drivers) |
| File Distribution | Complete (S3/GCS/Azure/Git backends, mirror groups) |
| Secrets Management | Complete (REST + gRPC API, rotation policies, encrypted cache) |
| Runbook Automation | Complete (triggers, approvals, ITSM integration) |
| Kubernetes Operator | Complete (CRDs, reconciliation, drift detection) |
| gRPC/REST APIs | Complete (7 gRPC services, 15 REST handlers) |
| Air-Gapped Deployments | Complete (bootstrap packages, offline registry, data diode) |
| MCP Server | Complete (AI-assisted operations for Claude Desktop, Cursor) |
| Scheduling | Complete (schedule REST API, maintenance windows) |
| DNS Management | Complete |
| CLI Wiring | Complete (all 26 plugins wired to real APIs) |
| Documentation | Complete (Hugo + Docsy site) |

### Test Coverage

| Test Type | Count |
|-----------|-------|
| **Total Test Functions** | 12,015 |
| Unit Tests (internal/ + pkg/) | 10,550 |
| CLI Tests (cmd/) | 1,271 |
| E2E Tests (test/e2e/) | 170 |
| Bootstrap Tests (test/bootstrap/) | 21 |
| Benchmark Functions | 137 |
| Subtests (t.Run) | 3,010 |
| Test Files | 785 |

Counts are derived from source directory `*_test.go` scans (e.g., `grep -r "^func Test" --include="*_test.go"`).

## Roadmap

### Future Considerations

| Category | Description |
|----------|-------------|
| **Web UI / Management Console** | Browser-based dashboard, topology visualization, state editor |
| **Release & Distribution** | Release packaging, signing, distribution pipelines |
| **Blueprint Marketplace** | Community marketplace for sharing blueprints |
| **Cross-Platform Testing** | Expanded platform coverage validation |
| **Multi-Cloud Testing** | Cloud provider integration testing |
| **Multi-Tenancy** | Namespace isolation, per-tenant RBAC/quotas, SSO integration (OIDC/SAML) |
| **Automatic Drift Remediation** | Opt-in auto-fix, approval workflows, change management integration |
| **Agent Self-Update** | Secure binary distribution, staged rollouts, automatic rollback |
| **Compliance Presets** | CIS Benchmarks, SOC 2, HIPAA, PCI-DSS policy packs |
| **Network Discovery** | Automatic scanning, L2/L3 mapping, dependency visualization |
| **Terraform Provider** | Terraform provider for Keystone Core resources |
| **Migration Tools** | Chef/Puppet to Keystone Core converters |
| **Mobile Monitoring** | iOS/Android apps with push notifications |

## Quick Links

- [Getting Started](/docs/getting-started/) - Install and deploy in 15 minutes
- [Architecture Overview](/docs/getting-started/architecture/) - System design and components
- [Roadmap](/docs/community/roadmap/) - Detailed development plans
