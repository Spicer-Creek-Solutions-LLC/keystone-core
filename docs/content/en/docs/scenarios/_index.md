---
title: "Real-World Scenarios"
weight: 40
description: >
  Production-ready examples and use cases for common infrastructure patterns
---

This section provides complete, production-ready examples for common real-world infrastructure scenarios. Each scenario includes:

- **Overview**: Business context and goals
- **Architecture**: System design and component interaction
- **Implementation**: Complete configuration files with detailed explanations
- **Verification**: How to validate the deployment
- **Troubleshooting**: Common issues and solutions

## Scenario Categories

### Application Deployment

- [Multi-Tier Web Application]({{< relref "multi-tier-webapp" >}}) - Deploy a complete 3-tier application with web servers, application servers, and databases
- [Microservices Platform]({{< relref "microservices-platform" >}}) - Deploy and manage a microservices-based architecture with service mesh

### Infrastructure Management

- [Hybrid Cloud Infrastructure]({{< relref "hybrid-infrastructure" >}}) - Manage resources across Kubernetes, VMs, and cloud providers
- [Edge Deployment]({{< relref "edge-deployment" >}}) - Deploy and manage distributed edge infrastructure
- [Windows Infrastructure]({{< relref "windows-infrastructure" >}}) - Manage Windows servers with Active Directory integration

### Operations & Automation

- [GitOps Workflow]({{< relref "gitops-workflow" >}}) - Implement complete GitOps with ArgoCD/Flux integration, verification, and rollback
- [Event-Driven Automation]({{< relref "event-driven-automation" >}}) - Build reactive automation using events and reactors
- [Multi-Environment Promotion]({{< relref "multi-environment" >}}) - Manage dev, staging, and production with safe promotion workflows

### Compliance & Security

- [Compliance Automation]({{< relref "compliance-automation" >}}) - Automate security baselines, policy enforcement, and compliance reporting
- [Disaster Recovery]({{< relref "disaster-recovery" >}}) - Implement backup, restore, and failover procedures

### Database & Storage

- [Database HA Deployment]({{< relref "database-ha" >}}) - Deploy highly available PostgreSQL/MySQL clusters

## Using These Scenarios

### Prerequisites

All scenarios assume:
- Keystone Core control plane is running
- Agents are installed on target hosts
- Basic familiarity with state files and targeting

### Customization

Each scenario includes parameterized configurations. Common customization points:

```yaml
# Most scenarios use these common parameters
parameters:
  environment:
    description: "Deployment environment"
    type: string
    enum: [dev, staging, production]

  domain:
    description: "Base domain for services"
    type: string

  notification_email:
    description: "Alert notification email"
    type: string
```

### Combining Scenarios

Scenarios are designed to work together. For example:

```bash
# Deploy security baseline first
kscorectl blueprint apply security-baseline \
  --target "environment:production"

# Then deploy your application
kscorectl blueprint apply multi-tier-webapp \
  --target "environment:production" \
  --var domain=myapp.example.com

# Finally, set up monitoring
kscorectl blueprint apply monitoring-stack \
  --target "environment:production"
```
