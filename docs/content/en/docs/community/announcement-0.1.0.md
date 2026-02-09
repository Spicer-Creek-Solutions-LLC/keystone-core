---
title: "Announcing Keystone Core 0.1.0"
weight: 1
description: >
  Keystone Core 0.1.0 release announcement
---

We are excited to announce the first public release of **Keystone Core**, a cloud-native runtime infrastructure control plane.

## What is Keystone Core?

Keystone Core fills the critical gap between declarative GitOps deployments and runtime infrastructure operations. While tools like ArgoCD and Flux excel at deploying configurations, Keystone Core keeps your infrastructure running by providing:

- **Real-time state management** with drift detection and remediation
- **Event-driven automation** that responds to infrastructure changes
- **Policy enforcement** ensuring continuous compliance
- **Unified management** across Kubernetes, VMs, bare metal, and edge devices

**Our philosophy: "GitOps deploys it. We keep it running."**

## Key Features

### Lightweight, Secure Agents

Deploy lightweight Go agents across your infrastructure. Agents connect via NATS messaging, support automatic registration, and require minimal resources.

```bash
# Join an agent to your control plane
kscore-agent join --server https://control-plane:8443
```

### Declarative State Management

Define desired state in YAML and let Keystone Core ensure compliance:

```yaml
package:
  ensure_nginx:
    state: present
    name: nginx

service:
  nginx_running:
    state: running
    name: nginx
    enabled: true
```

### Event-Driven Reactors

Automatically respond to infrastructure events:

```yaml
reactors:
  - name: restart-on-config-change
    match:
      type: file.changed
      path: /etc/nginx/*
    action:
      type: service.restart
      service: nginx
```

### Policy-as-Code

Enforce compliance with OPA or CEL policies:

```rego
package keystone.security

deny[msg] {
    input.resource.type == "service"
    input.resource.user == "root"
    msg := "Services must not run as root"
}
```

### Reusable Blueprints

Get started quickly with official blueprints:

- **monitoring-stack** - Prometheus, Grafana, Alertmanager
- **security-baseline** - CIS-inspired security hardening
- **nats-cluster** - Production NATS cluster
- **postgres-ha** - High-availability PostgreSQL

```bash
kscorectl blueprint install kscore/monitoring-stack
```

## Architecture Highlights

- **Zero external dependencies to start**: Embedded NATS and SQLite for simple deployments
- **Production-ready scaling**: External NATS cluster and PostgreSQL for large deployments
- **High availability**: etcd-based clustering with automatic leader election
- **Security-first**: SPIFFE identities, mTLS everywhere, policy enforcement

## Getting Started

Install the CLI and bootstrap your first control plane in minutes:

```bash
# Install CLI
curl -fsSL https://get.keystone-core.io | sh

# Bootstrap control plane
kscorectl bootstrap seed --mode embedded

# Install a blueprint
kscorectl blueprint install kscore/security-baseline
```

## What's Included

This release includes:

- **41 completed epics** of functionality
- **14 official blueprints** ready to deploy
- **15+ state modules** for common infrastructure tasks
- **Comprehensive documentation** with tutorials and reference guides
- **Multi-platform support** for Linux, Windows, and macOS

## Platform Support

| Platform | Control Plane | Agent |
|----------|---------------|-------|
| Ubuntu 22.04/24.04 | Yes | Yes |
| RHEL 8/9 | Yes | Yes |
| Debian 11/12 | Yes | Yes |
| Windows Server 2019+ | — | Yes |
| macOS 12+ | — | Yes |
| Kubernetes 1.26+ | Yes | Yes |

## Community

Keystone Core is open source and we welcome contributions:

- **Documentation**: [docs.keystone-core.io](https://docs.keystone-core.io)
- **GitHub**: [github.com/shawnbutts/keystone-core](https://github.com/shawnbutts/keystone-core)
- **Discussions**: [GitHub Discussions](https://github.com/shawnbutts/keystone-core/discussions)

## What's Next

The 0.2.0 release (planned for July 2026) will focus on:

- Enhanced Terraform provider
- Additional cloud integrations
- Performance optimizations
- Community-requested features

## Thank You

Thank you to everyone who contributed to making this release possible. We're excited to see what you build with Keystone Core!

---

*Keystone Core 0.1.0 is available now. [Get started today](/docs/getting-started/quick-start/).*
