# Epic 28: Standard Deployment Blueprints

## Overview

Create a comprehensive set of official blueprints that enable users to deploy complete Keystone Core environments through the bootstrap experience. These blueprints cover the spectrum from simple demo instances to full enterprise deployments.

**Goal**: Provide battle-tested, well-documented blueprints that work out-of-the-box with `kscore-agent bootstrap` for any deployment scenario.

**Status**: Complete (publishing/testing pending)

## Success Criteria

1. Complete blueprint set covering demo, production, and enterprise deployments
2. All blueprints follow Epic 25 design specifications
3. Blueprints are parameterized for customization without forking
4. Each blueprint includes comprehensive documentation
5. Blueprints are tested across all supported platforms
6. Version-controlled with semantic versioning
7. Published to official blueprint registry

## Phase 0 Notes (Catalog & Conventions)

### Catalog Inventory

**Core**:
- `kscore/demo`
- `kscore/production-cluster`
- `kscore/enterprise-platform`

**Infrastructure**:
- `kscore/nats-cluster`
- `kscore/postgres-ha`

**Observability**:
- `kscore/monitoring-stack`
- `kscore/metrics-only`

**Security**:
- `kscore/security-baseline`
- `kscore/identity-federation`

**Integrations**:
- `kscore/gitops-integration`
- `kscore/proxy-agents`
- `kscore/file-distribution`

**Platform**:
- `kscore/kubernetes-operator`
- `kscore/edge-deployment`

### Shared Parameter Conventions

- Use `cluster_name`, `node_role`, `nats_mode`, `nats_urls`, `postgres_host`, `postgres_port`
- Prefer `tls_mode` with `generate|provided|letsencrypt` and `tls_cert|tls_key|ca_cert` for provided certs
- Use `backup_enabled` and `backup_destination` consistently across blueprints
- Secrets use `type: string`, `sensitive: true`, `source: secret`
- Lists use consistent naming (`*_nodes`, `*_hosts`, `*_urls`) with required markers in docs

### Parameter Schema Template

```yaml
parameters:
  cluster_name:
    type: string
    description: Cluster identifier
    default: keystone
  postgres_host:
    type: string
    description: PostgreSQL host
    required: true
  postgres_password:
    type: string
    description: PostgreSQL password
    sensitive: true
    source: secret
  tls_mode:
    type: string
    description: TLS mode
    enum: [generate, provided, letsencrypt]
    default: generate
```

## User Stories

### US28.1: Demo Environment Blueprint
**As a** new user
**I want to** deploy a complete demo environment with one command
**So that** I can explore all Keystone Core features quickly

**Acceptance Criteria**:
- Single-node deployment with all features enabled
- Includes sample agents, states, events, policies
- Pre-configured Grafana dashboards
- Tutorial/exploration guide included
- Completes in under 5 minutes

### US28.2: Production Cluster Blueprint
**As a** platform engineer
**I want to** deploy a production-ready HA cluster
**So that** I have a resilient, supportable deployment

**Acceptance Criteria**:
- 3+ control plane nodes with automatic failover
- External NATS cluster configuration
- PostgreSQL with replication support
- TLS everywhere
- Backup configuration included
- Monitoring and alerting configured

### US28.3: Enterprise Platform Blueprint
**As an** enterprise architect
**I want to** deploy a full-scale, multi-region platform
**So that** I can manage infrastructure across my organization

**Acceptance Criteria**:
- Multi-region/multi-cluster support
- SPIFFE identity federation
- Advanced policy enforcement
- Compliance reporting
- Integration with enterprise systems (LDAP, SSO)
- Audit logging to external systems

### US28.4: Component Blueprints
**As a** system administrator
**I want to** add specific capabilities to my deployment
**So that** I can customize my environment incrementally

**Acceptance Criteria**:
- Monitoring stack (Prometheus, Grafana, Loki, Tempo)
- Security baseline (hardening, policies, audit)
- GitOps integration (ArgoCD, Flux webhooks)
- Proxy agent setup for network devices
- File distribution server

## Blueprint Catalog

### Core Deployment Blueprints

#### 1. `kscore/demo` - Demo/Lab Environment
Single-node all-in-one deployment for evaluation and learning.

**Features**:
- Embedded NATS + SQLite
- All components on one node
- Sample configurations included
- Pre-configured dashboards
- Tutorial states and examples

**Parameters**:
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `hostname` | string | localhost | Hostname for the instance |
| `admin_password` | secret | (generated) | Admin API password |
| `enable_examples` | bool | true | Deploy example states and agents |
| `enable_dashboards` | bool | true | Deploy Grafana dashboards |

---

#### 2. `kscore/production-cluster` - Production HA Cluster
Three-node (or more) high-availability deployment.

**Features**:
- 3 control plane nodes with etcd
- External NATS cluster (embedded or external)
- PostgreSQL backend (external)
- TLS certificates (generated or provided)
- Automatic leader election
- Agent auto-registration

**Parameters**:
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `cluster_name` | string | keystone | Cluster identifier |
| `control_plane_nodes` | list | (required) | List of control plane node addresses |
| `nats_mode` | enum | embedded-cluster | embedded-cluster, external |
| `nats_urls` | list | [] | External NATS URLs (if external) |
| `postgres_host` | string | (required) | PostgreSQL host |
| `postgres_port` | int | 5432 | PostgreSQL port |
| `postgres_database` | string | keystone | Database name |
| `postgres_user` | string | keystone | Database user |
| `postgres_password` | secret | (required) | Database password |
| `tls_mode` | enum | generate | generate, provided, letsencrypt |
| `tls_cert` | string | | Path to TLS cert (if provided) |
| `tls_key` | secret | | Path to TLS key (if provided) |
| `ca_cert` | string | | Path to CA cert (if provided) |
| `backup_enabled` | bool | true | Enable automated backups |
| `backup_destination` | string | local | Backup destination |

---

#### 3. `kscore/enterprise-platform` - Enterprise Deployment
Full-scale, multi-region enterprise platform.

**Features**:
- Multi-region gateway topology
- SPIFFE identity with federation
- External identity provider integration
- Advanced policy framework
- Compliance reporting
- Audit log forwarding
- High-cardinality metrics support

**Parameters**:
All production-cluster parameters plus:
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `regions` | list | (required) | Region configurations |
| `identity_provider` | enum | embedded | embedded, spire, aws, gcp, azure |
| `spire_server` | string | | SPIRE server address |
| `federation_domains` | list | [] | Federated trust domains |
| `ldap_enabled` | bool | false | Enable LDAP integration |
| `ldap_server` | string | | LDAP server address |
| `oidc_enabled` | bool | false | Enable OIDC SSO |
| `oidc_issuer` | string | | OIDC issuer URL |
| `audit_destination` | string | local | Audit log destination |
| `compliance_frameworks` | list | [] | Compliance frameworks to enable |

---

### Infrastructure Blueprints

#### 4. `kscore/nats-cluster` - NATS Cluster Setup
Standalone NATS cluster for Keystone Core.

**Features**:
- 3-node NATS cluster with JetStream
- TLS between nodes
- Monitoring endpoints
- Persistence configuration

**Parameters**:
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `nodes` | list | (required) | NATS node addresses |
| `cluster_name` | string | keystone-nats | NATS cluster name |
| `jetstream_enabled` | bool | true | Enable JetStream |
| `storage_dir` | string | /var/lib/nats | JetStream storage |
| `max_memory` | string | 1Gi | JetStream memory limit |
| `max_storage` | string | 10Gi | JetStream storage limit |

---

#### 5. `kscore/postgres-ha` - PostgreSQL HA Setup
PostgreSQL with replication for Keystone Core.

**Features**:
- Primary + replica configuration
- Automatic failover (with Patroni optional)
- Backup configuration
- Connection pooling (PgBouncer)

**Parameters**:
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `primary_host` | string | (required) | Primary PostgreSQL host |
| `replica_hosts` | list | [] | Replica hosts |
| `database` | string | keystone | Database name |
| `user` | string | keystone | Database user |
| `password` | secret | (required) | Database password |
| `enable_pgbouncer` | bool | true | Enable connection pooling |
| `backup_enabled` | bool | true | Enable WAL archiving |

---

### Observability Blueprints

#### 6. `kscore/monitoring-stack` - Full Observability Stack
Complete monitoring, logging, and tracing setup.

**Features**:
- Prometheus for metrics
- Loki for logs
- Tempo for traces
- Grafana with pre-built dashboards
- Alertmanager with default alerts

**Parameters**:
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `prometheus_retention` | string | 15d | Metrics retention period |
| `loki_retention` | string | 30d | Log retention period |
| `tempo_retention` | string | 7d | Trace retention period |
| `grafana_admin_password` | secret | (generated) | Grafana admin password |
| `alertmanager_config` | string | | Custom alertmanager config |
| `slack_webhook` | secret | | Slack webhook for alerts |
| `pagerduty_key` | secret | | PagerDuty integration key |

---

#### 7. `kscore/metrics-only` - Lightweight Metrics
Prometheus-only metrics collection.

**Parameters**:
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `retention` | string | 15d | Metrics retention |
| `remote_write_url` | string | | Remote write endpoint |
| `scrape_interval` | string | 15s | Scrape interval |

---

### Security Blueprints

#### 8. `kscore/security-baseline` - Security Hardening
Security hardening and compliance baseline.

**Features**:
- Default deny policies
- Rate limiting
- Audit logging
- Secret rotation schedules
- CIS benchmark policies

**Parameters**:
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `policy_enforcement` | enum | enforce | audit, warn, enforce |
| `audit_retention` | string | 90d | Audit log retention |
| `secret_rotation_days` | int | 90 | Secret rotation interval |
| `compliance_frameworks` | list | [] | cis, soc2, hipaa, pci |

---

#### 9. `kscore/identity-federation` - Identity Federation Setup
Cross-cluster identity federation.

**Parameters**:
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `local_domain` | string | (required) | Local trust domain |
| `federated_domains` | list | [] | Domains to federate with |
| `federation_policy` | enum | verify | trust, verify, strict |

---

### Integration Blueprints

#### 10. `kscore/gitops-integration` - GitOps Setup
ArgoCD and Flux webhook integration.

**Features**:
- Webhook receivers for ArgoCD/Flux
- Deployment verification
- Rollback automation
- Git sync for state files

**Parameters**:
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `argocd_enabled` | bool | true | Enable ArgoCD integration |
| `flux_enabled` | bool | true | Enable Flux integration |
| `verification_enabled` | bool | true | Enable deployment verification |
| `rollback_enabled` | bool | true | Enable automatic rollback |
| `git_repos` | list | [] | Git repositories to sync |

---

#### 11. `kscore/proxy-agents` - Proxy Agent Setup
Manage unmanaged devices via proxy.

**Features**:
- SSH, SNMP, REST, WinRM adapters
- Credential vault integration
- Device discovery
- Network device templates

**Parameters**:
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `credential_backend` | enum | file | file, vault, k8s |
| `vault_address` | string | | Vault server address |
| `discovery_enabled` | bool | false | Enable device discovery |
| `discovery_subnets` | list | [] | Subnets to scan |

---

#### 12. `kscore/file-distribution` - File Distribution Server
NATS-based file distribution setup.

**Parameters**:
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `backends` | list | [local] | Storage backends |
| `s3_bucket` | string | | S3 bucket name |
| `s3_region` | string | | S3 region |
| `gcs_bucket` | string | | GCS bucket name |
| `azure_container` | string | | Azure container name |
| `mirror_groups` | list | [] | Mirror group configurations |

---

### Platform-Specific Blueprints

#### 13. `kscore/kubernetes-operator` - Kubernetes Integration
Deploy Keystone Core as Kubernetes operator.

**Features**:
- CRD installation
- Operator deployment
- RBAC configuration
- Service mesh integration

**Parameters**:
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `namespace` | string | keystone-system | Kubernetes namespace |
| `service_mesh` | enum | none | none, istio, linkerd, consul |
| `ingress_enabled` | bool | true | Enable ingress |
| `ingress_class` | string | nginx | Ingress class |

---

#### 14. `kscore/edge-deployment` - Edge/IoT Deployment
Lightweight edge deployment configuration.

**Features**:
- Minimal resource footprint
- Offline operation support
- Local caching
- Intermittent connectivity handling

**Parameters**:
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `hub_endpoint` | string | (required) | Central hub endpoint |
| `offline_buffer_size` | string | 100Mi | Offline message buffer |
| `cache_size` | string | 500Mi | Local cache size |
| `sync_interval` | string | 5m | Sync interval when online |

## Technical Tasks

### Phase 1: Blueprint Runtime Implementation (Weeks 1-4)

*Note: This phase implements the Epic 25 blueprint design*

#### T1.1: Blueprint Manifest Parser
- Implement `blueprint.yaml` parser per Epic 25 spec
- Parameter validation with JSON Schema
- Dependency declaration parsing
- Secret reference handling (`!secret` syntax)

#### T1.2: Blueprint Executor
- State file expansion from blueprint
- Parameter substitution engine
- Dependency ordering and execution
- Rollback on failure

#### T1.3: Blueprint Registry Client
- Blueprint discovery from registry
- Version resolution
- Download and caching
- Signature verification

#### T1.4: Blueprint CLI
- `kscore-blueprint init` - Initialize new blueprint
- `kscore-blueprint validate` - Validate blueprint
- `kscore-blueprint build` - Package blueprint
- `kscore-blueprint publish` - Publish to registry
- `kscore-blueprint install` - Install from registry
- `kscore-blueprint apply` - Apply blueprint

### Phase 2: Core Deployment Blueprints (Weeks 5-8)

#### T2.1: Demo Blueprint (`kscore/demo`)
- Blueprint manifest with minimal parameters
- All-in-one state files
- Sample configurations
- Tutorial documentation
- Example agents and states

#### T2.2: Production Cluster Blueprint (`kscore/production-cluster`)
- Multi-node state files
- Certificate generation states
- Database schema states
- NATS cluster configuration
- Health check states

#### T2.3: Enterprise Platform Blueprint (`kscore/enterprise-platform`)
- Multi-region configuration
- Identity federation states
- Advanced policy configuration
- Audit logging configuration

### Phase 3: Infrastructure Blueprints (Weeks 9-10)

#### T3.1: NATS Cluster Blueprint
- NATS server states for each node
- Cluster configuration
- JetStream configuration
- TLS setup

#### T3.2: PostgreSQL HA Blueprint
- PostgreSQL installation states
- Replication configuration
- Backup setup
- Connection pooling

### Phase 4: Observability Blueprints (Weeks 11-12)

#### T4.1: Monitoring Stack Blueprint
- Prometheus deployment
- Loki deployment
- Tempo deployment
- Grafana with dashboards
- Alertmanager configuration

#### T4.2: Metrics-Only Blueprint
- Lightweight Prometheus setup
- Remote write configuration

### Phase 5: Security and Integration Blueprints (Weeks 13-14)

#### T5.1: Security Baseline Blueprint
- Hardening states
- Policy definitions
- Audit configuration
- Secret rotation

#### T5.2: GitOps Integration Blueprint
- Webhook receiver setup
- Verification configuration
- Git sync setup

#### T5.3: Proxy Agents Blueprint
- Protocol adapter configuration
- Credential backend setup
- Discovery configuration

### Phase 6: Documentation and Publishing (Weeks 15-16)

#### T6.1: Blueprint Documentation
- README for each blueprint
- Parameter reference
- Usage examples
- Troubleshooting guides

#### T6.2: Blueprint Registry Setup
- Official registry deployment
- Blueprint signing
- Version management
- Documentation hosting

#### T6.3: Integration Testing
- Blueprint installation tests
- Cross-blueprint compatibility
- Upgrade path testing

## Dependencies

- **Epic 25** (Blueprints): Blueprint specification and design
- **Epic 27** (Agent Bootstrap): Bootstrap integration
- **Epic 23** (Self-Management): Backup/restore blueprints
- **Epic 3** (State Management): State execution engine

## Risks and Mitigations

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Blueprint complexity explosion | Medium | Medium | Clear scope per blueprint, composability |
| Version compatibility issues | Medium | High | Strict versioning, compatibility matrix |
| Platform-specific failures | High | Medium | Extensive testing matrix, fallbacks |
| Parameter validation gaps | Medium | Medium | JSON Schema validation, examples |

## Testing Strategy

- **Unit Tests**: Blueprint parsing, parameter validation
- **Integration Tests**: Blueprint execution, state application
- **E2E Tests**: Full deployment scenarios (covered in Epic 29)
- **Compatibility Tests**: Cross-platform, cross-version

## Definition of Done

- [x] Blueprint runtime implemented (Epic 25 implementation)
- [x] All 14 blueprints created and documented
- [x] Parameter validation for all blueprints
- [x] Documentation for each blueprint
- [ ] Blueprints published to registry
- [ ] Integration tests passing
- [ ] Cross-platform testing complete
- [ ] Code review approved

## Deferred Work

- Blueprint signing and registry verification are deferred to the future release readiness epic.
