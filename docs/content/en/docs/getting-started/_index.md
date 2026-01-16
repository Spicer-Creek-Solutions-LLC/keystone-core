---
title: "Getting Started"
linkTitle: "Getting Started"
weight: 1
description: >
  Get up and running with Keystone Core in minutes
---

This section helps you get started with Keystone Core quickly.

## What is Keystone Core?

Keystone Core is a cloud-native runtime infrastructure control plane. Think of it as the operational layer that sits between your GitOps deployments and your live infrastructure.

**Key analogy**: If Kubernetes is the orchestrator for containers, Keystone Core is the orchestrator for infrastructure operations.

## Core Philosophy

**"GitOps deploys it. We keep it running."**

GitOps tools (ArgoCD, Flux) excel at deploying applications declaratively from Git. But what happens after deployment?

- Configuration drift happens
- Events need automated responses
- Policies need enforcement
- Deployments need verification
- Rollbacks may be necessary
- Compliance must be maintained

Keystone Core handles all of this.

## Inspired By, Modernized For

Keystone Core takes inspiration from **Salt Project** (formerly SaltStack) but reimagines it for cloud-native environments:

### From Salt:
- Declarative state management
- Event-driven automation (reactors)
- Variables (vars) and facts
- Remote execution with targeting

### Cloud-Native Additions:
- **GitOps Integration**: ArgoCD/Flux webhooks, verification, rollback
- **Policy-as-Code**: OPA/CEL for continuous compliance
- **Kubernetes Operator**: Native CRDs for K8s environments
- **Multi-Cloud**: AWS, GCP, Azure, edge devices
- **Plugin System**: Starlark/WASM modules with cryptographic verification
- **Modern Stack**: Go, NATS, gRPC, Prometheus, OpenTelemetry

## When to Use Keystone Core

✅ **Use Keystone Core when**:
- You have GitOps workflows and need runtime operations
- You need configuration drift detection and remediation
- You want event-driven infrastructure automation
- You require continuous compliance with policy enforcement
- You manage heterogeneous infrastructure (K8s + VMs + cloud + edge)
- You need deployment verification and automated rollback

❌ **Don't use Keystone Core when**:
- You only need basic configuration management (use Ansible)
- You only need container orchestration (use Kubernetes)
- You only need GitOps deployment (use ArgoCD/Flux alone)
- You have <10 nodes (overhead not worth it for tiny deployments)

## Quick Navigation

1. **[Overview](overview/)** - Detailed introduction and use cases
2. **[Installation](installation/)** - Install Keystone Core on your system
3. **[Quick Start](quick-start/)** - 5-minute deployment guide
4. **[Hello World](hello-world/)** - Apply a minimal state file
5. **[Architecture](architecture/)** - System architecture overview

Ready to get started? Continue to the [Overview](overview/) or jump straight to [Installation](installation/).
