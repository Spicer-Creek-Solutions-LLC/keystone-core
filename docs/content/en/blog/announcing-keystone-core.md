---
title: "Introducing Keystone Core: The Runtime Infrastructure Control Plane"
linkTitle: "Introducing Keystone Core"
date: 2024-12-27
description: >
  Announcing Keystone Core - a modern, cloud-native runtime infrastructure control plane that bridges the gap between GitOps deployments and operational reality.
author: Keystone Core Team
---

Today we're excited to announce **Keystone Core**, a cloud-native runtime infrastructure control plane designed for the modern operations landscape. Keystone Core fills the critical gap between declarative GitOps deployments and the ongoing operational reality of running production infrastructure.

## The Problem

Modern infrastructure teams have embraced GitOps and Infrastructure as Code. Tools like ArgoCD, Flux, Terraform, and Pulumi have transformed how we deploy and manage infrastructure. But there's a gap:

**GitOps tells you what should be deployed. It doesn't tell you what's actually happening.**

After deployment, operations teams face challenges that GitOps tools weren't designed to solve:

- **Configuration Drift**: Systems drift from their declared state over time
- **Runtime Visibility**: No unified view of what's actually running across environments
- **Operational Response**: No automated way to respond to runtime events
- **Cross-Platform Operations**: Different tools for Kubernetes, VMs, bare metal, and edge
- **Compliance Verification**: Continuous compliance checking, not just at deploy time

## The Solution: Keystone Core

Keystone Core is designed around a simple principle:

> **"GitOps deploys it. We keep it running."**

Keystone Core provides:

### Unified Infrastructure Management

Manage Kubernetes, VMs, bare metal servers, edge devices, and cloud resources through a single control plane. Deploy lightweight agents that report status, execute commands, and apply configurations regardless of the underlying platform.

```yaml
# Target any infrastructure
kscorectl exec run --target="role:webserver" -- systemctl status nginx
kscorectl exec run --target="cloud:aws AND env:prod" -- df -h
kscorectl exec run --target="k8s:cluster-1" -- kubectl get pods
```

### Declarative State Management

Define the desired state of your infrastructure and let Keystone Core maintain it. Inspired by Salt Project but modernized for cloud-native environments:

```yaml
# Declarative state - what should exist
nginx_installed:
  package:
    - name: nginx
      state: installed

nginx_config:
  file:
    - path: /etc/nginx/nginx.conf
      state: present
      source: salt://nginx/nginx.conf
      require:
        - package: nginx_installed

nginx_running:
  service:
    - name: nginx
      state: running
      enable: true
      watch:
        - file: nginx_config
```

### Event-Driven Automation

React to infrastructure events in real-time. Define reactors that automatically respond to agent registrations, state changes, drift detection, and more:

```yaml
# Automatically respond to events
reactor_auto_remediate:
  filter: "type == 'state.drift' AND severity >= 'high'"
  actions:
    - type: command
      target: "{{ .Event.Source }}"
      command: "kscorectl state apply --fix"
    - type: webhook
      url: "https://slack.com/api/chat.postMessage"
      body:
        text: "Auto-remediated drift on {{ .Event.Source }}"
```

### GitOps Integration

Integrate with your existing GitOps tools. Receive webhooks from ArgoCD, Flux, GitHub, and GitLab. Verify deployments, automate rollbacks, and manage promotion pipelines:

```yaml
# Verify deployment after ArgoCD sync
verification:
  steps:
    - name: health-check
      type: http
      config:
        url: "https://app.example.com/health"
        expected_status: 200
    - name: smoke-test
      type: command
      config:
        command: "./run-smoke-tests.sh"
  on_failure:
    rollback: true
    notify:
      - slack
      - pagerduty
```

### Policy Enforcement

Define and enforce policies across your infrastructure using OPA (Rego) or CEL:

```rego
# OPA policy - no public S3 buckets
package keystone.s3

deny[msg] {
    input.resource.type == "aws_s3_bucket"
    input.resource.acl == "public-read"
    msg := sprintf("S3 bucket %s cannot be public", [input.resource.name])
}
```

### Comprehensive Observability

Built-in metrics, logging, and tracing with Prometheus, OpenTelemetry, and pre-built Grafana dashboards. Plus a real-time TUI monitor for SSH-friendly environments:

```bash
# Real-time monitoring in your terminal
kscorectl monitor
```

## Architecture

Keystone Core is built on modern, proven technologies:

- **Go**: Fast, efficient, cross-platform binaries
- **NATS**: Lightweight, high-performance message bus with JetStream for persistence
- **SQLite/PostgreSQL**: Flexible storage options from development to production
- **OPA/CEL**: Industry-standard policy engines
- **Starlark/WASM**: Secure, sandboxed plugin execution

The architecture supports multiple deployment modes:

- **Single Binary**: Embedded NATS + SQLite for getting started in minutes
- **High Availability**: External NATS cluster + PostgreSQL for production
- **Hybrid**: Control plane with external services, agents with embedded NATS

## Getting Started

Get started in under 5 minutes:

```bash
# Install Keystone Core
curl -fsSL https://get.kscore.dev | sh

# Start the control plane (embedded NATS + SQLite)
kscore-server &

# Start an agent
kscore-agent --server-url=nats://localhost:4222 &

# Execute a command
kscorectl exec run --target="*" -- hostname

# Apply a state
kscorectl state apply my-config.yaml
```

See our [Quick Start Guide](/docs/getting-started/quick-start/) for a complete walkthrough.

## Current Status

Keystone Core has reached feature completeness for its core capabilities:

| Epic | Status | Description |
|------|--------|-------------|
| Core Infrastructure | Complete | NATS, agents, control plane |
| Remote Execution | Complete | Cross-platform command execution |
| State Management | Complete | Declarative configuration |
| Event System | Complete | Event-driven automation |
| GitOps Integration | Complete | ArgoCD, Flux, GitHub, GitLab |
| Policy Enforcement | Complete | OPA and CEL support |
| Observability | Complete | Metrics, logging, tracing, TUI |
| Multi-Environment | Complete | K8s, VMs, bare metal, edge, cloud |
| Plugin System | Complete | Starlark and WASM modules |
| Documentation | Complete | Comprehensive docs |

**Next milestone**: v1.0.0 stable release and high-availability clustering (Epic 11).

## Get Involved

Keystone Core is open source and we welcome contributions:

- **GitHub**: [github.com/shawnbutts/keystone-core](https://github.com/shawnbutts/keystone-core)
- **Discord**: [discord.gg/kscore](https://discord.gg/kscore)
- **Twitter**: [@kscore](https://twitter.com/kscore)

Check out our [Contributing Guide](/docs/community/contributing/) to get started.

## Acknowledgments

Keystone Core stands on the shoulders of giants. We're grateful to the teams behind:

- [Salt Project](https://saltproject.io/) for pioneering infrastructure automation
- [NATS](https://nats.io/) for the incredible message bus
- [Open Policy Agent](https://www.openpolicyagent.org/) for policy-as-code
- [Hugo](https://gohugo.io/) and [Docsy](https://www.docsy.dev/) for documentation

## What's Next

We're working toward v1.0.0 with:

- High-availability clustering with etcd (Epic 11)
- Performance optimizations
- Additional integrations based on community feedback

Follow our [Roadmap](/docs/community/roadmap/) for the latest updates.

---

**Ready to get started?** Check out our [documentation](/docs/) or join the conversation on [Discord](https://discord.gg/kscore).

*GitOps deploys it. We keep it running.*
