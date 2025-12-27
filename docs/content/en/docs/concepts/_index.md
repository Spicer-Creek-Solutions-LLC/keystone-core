---
title: "Concepts"
linkTitle: "Concepts"
weight: 30
description: >
  Deep dives into Keystone Core's architecture and core subsystems
---

## Understanding Keystone Core

This section provides in-depth explanations of Keystone Core's core concepts and subsystems. Each page explores how a major component works, why it's designed that way, and how to use it effectively.

### Core Infrastructure

Learn about the foundational components that power Keystone Core:

- **[Control Plane](control-plane/)** - API server, connection manager, state manager
- **[Agents](agents/)** - Lightweight agents running on managed nodes
- **[Message Bus](message-bus/)** - NATS integration with embedded, external, and leaf modes
- **[State Storage](state-storage/)** - SQLite vs PostgreSQL design decisions

### Operational Subsystems

Understand the systems that enable runtime operations:

- **[Remote Execution](remote-execution/)** - Command execution with targeting and batch processing
- **[State Management](state-management/)** - Declarative configuration with drift detection
- **[Event System](events/)** - Event-driven automation with pub/sub architecture
- **[Reactors](reactors/)** - Automated event responses and workflows

### Cloud-Native Integration

Explore Keystone Core's cloud-native capabilities:

- **[GitOps Integration](gitops/)** - ArgoCD/Flux webhooks, verification, rollback
- **[Policy Enforcement](policy/)** - OPA/CEL policies for compliance
- **[Observability](observability/)** - Metrics, logging, and monitoring

### Multi-Environment Support

Learn how Keystone Core manages diverse infrastructure:

- **[Kubernetes Integration](kubernetes/)** - CRDs, operators, and native K8s support
- **[Cloud Platforms](cloud-platforms/)** - AWS, GCP, Azure detection and integration
- **[Edge Computing](edge/)** - Offline mode, buffering, and resilience

## How to Use This Section

Each concept page follows a consistent structure:

1. **Overview** - What the concept is and why it exists
2. **Architecture** - How it's implemented internally
3. **Use Cases** - When and how to use it
4. **Configuration** - How to configure it
5. **Best Practices** - Recommended patterns
6. **Troubleshooting** - Common issues and solutions

## Next Steps

- **New to Keystone Core?** Start with [Getting Started](../getting-started/) first
- **Want to learn by doing?** Try the [Tutorials](../tutorials/)
- **Need specific details?** Check the [Reference](../reference/) documentation
- **Deploying to production?** See [Operations](../operations/)

## Contributing

Found an error or have a suggestion? See our [Contributing Guide](../../community/contributing/) to learn how to improve the documentation.
