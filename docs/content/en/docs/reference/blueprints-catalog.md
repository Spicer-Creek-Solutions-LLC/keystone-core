---
title: "Blueprint Catalog"
weight: 13
description: >
  Official Keystone Core blueprint catalog with parameters, features, and usage notes.
---

## Official Catalog

All official blueprints follow the `kscore/<name>` naming convention and are located under
`examples/blueprints/kscore/` for now. Each blueprint includes a manifest, states, README, and
optionally tests.

### Core Deployments

- **kscore/demo**: Single-node demo deployment with embedded NATS + SQLite.
- **kscore/production-cluster**: HA control plane deployment with external Postgres/NATS.
- **kscore/enterprise-platform**: Multi-region enterprise deployment with federation and integrations.

### Infrastructure

- **kscore/nats-cluster**: Standalone NATS cluster configuration.
- **kscore/postgres-ha**: PostgreSQL HA bootstrap with role/db creation.

### Observability

- **kscore/monitoring-stack**: Prometheus, Grafana, Alertmanager, Node Exporter.
- **kscore/metrics-only**: Lightweight Prometheus-only setup.

### Security + Identity

- **kscore/security-baseline**: Host hardening, auditd, firewall defaults, optional updates/fail2ban.
- **kscore/identity-federation**: SPIFFE federation config for multi-cluster trust.

### Integrations

- **kscore/gitops-integration**: ArgoCD/Flux hooks and verification settings.
- **kscore/proxy-agents**: Proxy agent configuration for unmanaged devices.
- **kscore/file-distribution**: File distribution backend configuration.

### Platform

- **kscore/kubernetes-operator**: Operator/CRD install with mesh + ingress options.
- **kscore/edge-deployment**: Lightweight edge node configuration.

## Usage Notes

- Use blueprint parameters instead of editing the blueprint states directly.
- Use features in each blueprint manifest to enable optional subcomponents.
- Supply secrets via `!secret` in a params file or your secret backend.

## Registry Publishing

Blueprints are versioned with SemVer and will be published to the official registry once
Epic 28 publishing infrastructure is complete. For now, use `kscorectl blueprint apply` with
local blueprint paths.
