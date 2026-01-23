---
title: "Operations"
weight: 4
description: >
  Comprehensive operational guides for deploying, monitoring, and maintaining Keystone Core in production
---

## Overview

The Operations section provides comprehensive guides for deploying, monitoring, maintaining, and securing Keystone Core in production environments. Whether you're running a single-node development setup or a high-availability production cluster, these guides will help you operate Keystone Core reliably.

## Guide Categories

### [Deployment](deployment/)
Production deployment patterns and strategies:
- **Single-node deployment** - Simple setup for development and small deployments
- **High-availability setup** - Multi-node cluster with automatic failover
- **Kubernetes deployment** - Native K8s deployment with Helm charts
- **Docker Compose** - Container-based deployment for quick setup
- **Scaling strategies** - Horizontal and vertical scaling approaches
- **Migration paths** - Embedded to external NATS, SQLite to PostgreSQL

### [Monitoring](monitoring/)
Comprehensive monitoring and observability setup:
- **Prometheus integration** - Metrics collection and storage
- **Grafana dashboards** - Pre-built dashboards for all components
- **Log aggregation** - Centralized logging with Loki or Elasticsearch
- **Alert rules** - Comprehensive alerting configuration
- **Health checks** - Liveness and readiness probes
- **Performance metrics** - Key performance indicators and SLOs

### [Maintenance](maintenance/)
Operational maintenance procedures:
- **Backup procedures** - State database and configuration backups
- **Restore procedures** - Disaster recovery workflows
- **Upgrade procedures** - Rolling updates and version migrations
- **Database maintenance** - SQLite to PostgreSQL migration
- **Data retention** - Event and log retention policies
- **Capacity planning** - Resource sizing and growth planning

### [Best Practices](best-practices/)
Recommended patterns for reliable and secure operations:
- **Deployment defaults** - Embedded vs external guidance
- **Security posture** - Identity, audit, and least privilege
- **Operational hygiene** - Backups, dry-run, rollout patterns

### [Migrations](migrations/)
Common migration paths and rollout strategies:
- **Salt to Keystone Core** - State translation and phased rollout
- **SQLite to PostgreSQL** - `kscore-migrate` workflow
- **Embedded to external NATS** - Cluster migration steps

### [Troubleshooting](troubleshooting/)
Diagnostic and resolution guides:
- **Agent connectivity** - Debug agent-to-control-plane connections
- **NATS issues** - Message bus troubleshooting
- **State failures** - State application and drift detection issues
- **Performance tuning** - Optimize for throughput and latency
- **Common errors** - Error message catalog with solutions
- **Debug logging** - Enable detailed logging for diagnosis

### [Runbooks](runbooks/)
Operational runbooks for incidents, maintenance, and recovery:
- **Incident response** - Security and operational incident handling
- **Backup & restore** - Data protection workflows
- **Disaster recovery** - DR procedures and validation
- **Upgrades** - Rolling upgrade checklists

### [Security](security/)
Security hardening and compliance:
- **Authentication** - Token and certificate-based auth
- **TLS configuration** - Encrypt all communications
- **RBAC policies** - Role-based access control setup
- **Security hardening** - Production security checklist
- **Audit logging** - Compliance audit trail
- **Secret management** - Secure secret storage and rotation

### [Module Registry](registry/)
Deploy and operate module registries:
- **Single-node deployment** - Development and small deployments
- **High-availability setup** - Redundant registry clusters
- **Kubernetes deployment** - Cloud-native registry with StatefulSets
- **Multi-region deployment** - Global module distribution
- **Shared storage options** - NFS, EFS, GCS, Azure Files
- **Backup and recovery** - Registry data protection

### [NATS Mesh](nats-mesh-deployment/)
NATS mesh networking and topology:
- **Deployment patterns** - Simple, HA, edge, multi-region
- **Leaf nodes** - Edge and branch office connectivity
- **Superclusters** - Multi-region gateway configuration
- **WebSocket transport** - Firewall-friendly connections
- **Operations** - Monitoring, troubleshooting, and maintenance

### [Telemetry Gateway](gateway/)
Centralized telemetry aggregation:
- **Deployment** - Single-node and HA configurations
- **Prometheus integration** - Metrics scraping and federation
- **Loki integration** - Log aggregation and querying
- **Tempo integration** - Distributed trace collection
- **Backend configuration** - Remote write and export

### [Self-Management](self-management/)
Keystone Core managing itself:
- **Bootstrap** - Initial cluster deployment from seed
- **Backup & Restore** - Data protection and recovery
- **Rolling upgrades** - Zero-downtime version updates
- **Disaster recovery** - Recovery procedures and runbooks
- **State modules** - Self-management state declarations

### [Windows Operations](windows/)
Windows platform operations:
- **Installation** - MSI installer and manual setup
- **Service management** - Windows service configuration
- **Agent operations** - Windows-specific agent features
- **Troubleshooting** - Windows-specific issues

### [IPv6 Operations](ipv6/)
IPv6 and dual-stack networking:
- **Configuration** - IPv6-only and dual-stack setup
- **NATS with IPv6** - Message bus configuration
- **Agent connectivity** - IPv6 agent registration
- **Troubleshooting** - IPv6-specific issues

## Quick Navigation by Role

### For DevOps Engineers
Start with:
1. [Deployment Guide](deployment/) - Choose your deployment pattern
2. [Monitoring Guide](monitoring/) - Set up observability
3. [Troubleshooting Guide](troubleshooting/) - Common issues and solutions

### For Platform Engineers
Focus on:
1. [High-Availability Setup](deployment/#high-availability) - Production cluster design
2. [Scaling Strategies](deployment/#scaling) - Handle growth
3. [Capacity Planning](maintenance/#capacity-planning) - Resource sizing

### For Security Engineers
Review:
1. [Security Guide](security/) - Complete security hardening
2. [Authentication Setup](security/#authentication) - Auth methods
3. [Audit Logging](security/#audit-logging) - Compliance tracking

### For Site Reliability Engineers (SREs)
Essential guides:
1. [Monitoring Setup](monitoring/) - Full observability stack
2. [Alert Rules](monitoring/#alerting) - Production alerting
3. [Backup/Restore](maintenance/#backup-restore) - Disaster recovery

## Production Checklist

Before deploying Keystone Core to production, ensure you've completed:

**Infrastructure**
- [ ] High-availability deployment (3+ control plane nodes)
- [ ] External NATS cluster (not embedded mode)
- [ ] PostgreSQL database (not SQLite)
- [ ] Load balancer for API endpoints
- [ ] Network segmentation and firewall rules

**Monitoring**
- [ ] Prometheus scraping all components
- [ ] Grafana dashboards installed
- [ ] Alert rules configured
- [ ] Alertmanager notification channels set up
- [ ] Log aggregation configured

**Security**
- [ ] TLS enabled for all connections
- [ ] Certificate-based authentication
- [ ] RBAC policies configured
- [ ] Audit logging enabled
- [ ] Security hardening checklist completed

**Reliability**
- [ ] Backup procedures tested
- [ ] Restore procedure tested
- [ ] Upgrade procedure documented and tested
- [ ] Disaster recovery plan documented
- [ ] Runbooks created for common incidents

**Performance**
- [ ] Resource limits configured
- [ ] Connection pools sized appropriately
- [ ] Event retention policies set
- [ ] Performance baseline established
- [ ] Load testing completed

## Support Resources

**Documentation**
- [Architecture Overview](/docs/getting-started/architecture/) - Understand the system design
- [Configuration Reference](/docs/reference/configuration/) - All configuration options
- [Metrics Reference](/docs/reference/metrics/) - Complete metrics catalog

**Community**
- GitHub Issues - Bug reports and feature requests
- Discussions - Questions and community support
- Slack/Discord - Real-time community chat

**Commercial Support**
- Enterprise support contracts available
- Professional services for deployment assistance
- Training and certification programs

## Best Practices Summary

### Deployment
- Start with single-node for dev, move to HA for production
- Use external NATS cluster for production (>100 nodes)
- Use PostgreSQL for production state storage
- Deploy agents in same datacenter as control plane when possible
- Use leaf nodes for edge deployments

### Monitoring
- Monitor all components (control plane, agents, NATS, database)
- Set up alerting before going to production
- Use Grafana dashboards for visualization
- Aggregate logs centrally
- Define SLOs and SLIs for critical paths

### Maintenance
- Back up state database and configurations daily
- Test restore procedures regularly
- Use rolling updates for zero-downtime upgrades
- Migrate from SQLite to PostgreSQL before scaling past 100 nodes
- Plan for capacity growth

### Troubleshooting
- Enable debug logging for diagnosis
- Check agent connectivity first
- Verify NATS cluster health
- Review audit logs for policy violations
- Use metrics to identify performance bottlenecks

### Security
- Enable TLS for all connections (NATS, API, database)
- Use certificate-based authentication in production
- Implement RBAC policies for least privilege
- Enable audit logging for compliance
- Rotate secrets regularly
- Keep Keystone Core and dependencies up to date

## See Also

- [Getting Started](/docs/getting-started/) - Initial setup and quick start
- [Concepts](/docs/concepts/) - Deep dive into Keystone Core architecture
- [Reference](/docs/reference/) - Complete API and CLI reference
- [Community](/docs/community/) - Contributing and development guides
