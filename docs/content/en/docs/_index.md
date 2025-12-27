---
title: "Documentation"
linkTitle: "Documentation"
weight: 20
---

Welcome to the TitanAnvil documentation!

TitanAnvil is a cloud-native runtime infrastructure control plane that operates between GitOps/IaC deployments and your live infrastructure. It ensures your systems stay running, compliant, and healthy.

## Key Concept

**"GitOps deploys it. We keep it running."**

While GitOps tools like ArgoCD and Flux excel at deploying applications declaratively, TitanAnvil handles the operational reality: configuration drift, event-driven automation, policy enforcement, and runtime verification.

## What You'll Find Here

### [Getting Started](/docs/getting-started/)
New to TitanAnvil? Start here to understand what it is, install it, and complete your first deployment in under 15 minutes.

### [Concepts](/docs/concepts/)
Deep dives into TitanAnvil's architecture and subsystems: agents, state management, events, GitOps integration, policy enforcement, and more.

### [Tutorials](/docs/tutorials/)
Step-by-step guides for common use cases. Each tutorial takes 15-30 minutes and includes working examples.

### [Reference](/docs/reference/)
Complete reference documentation for the CLI, API, configuration files, state modules, events, and metrics.

### [Operations](/docs/operations/)
Production deployment guides, monitoring setup, troubleshooting, backup/restore, and scaling guidance.

## Quick Links

- [5-Minute Quick Start](/docs/getting-started/quick-start/)
- [Architecture Overview](/docs/getting-started/architecture/)
- [CLI Reference](/docs/reference/cli/)
- [State Modules](/docs/reference/state-modules/)
- [GitOps Integration](/docs/concepts/gitops/)

## Need Help?

- **GitHub Issues**: [Report bugs or request features](https://github.com/titananvil/titan-anvil/issues)
- **Discussions**: [Ask questions and share ideas](https://github.com/titananvil/titan-anvil/discussions)
- **Documentation**: You're already here!

## Project Status

TitanAnvil has **9 of 11 epics complete** with comprehensive test coverage (150+ tests passing). Currently production-ready for:

✅ Core Infrastructure (NATS, agents, control plane)
✅ Remote Execution (targeting, batch execution)
✅ State Management (drift detection, declarative config)
✅ Event System (pub/sub, reactors, storage)
✅ GitOps Integration (ArgoCD, Flux, webhooks)
✅ Policy Enforcement (OPA/CEL, compliance)
✅ Observability (Prometheus, logging, TUI monitor)
✅ Multi-Environment (K8s, VMs, cloud, edge)
✅ Plugin System (Starlark, WASM modules)

📝 Documentation (in progress)
⏳ Clustering (planned)
