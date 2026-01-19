# Epic 33: Multi-Cloud & Real Environment CI/CD

## Overview

This epic establishes comprehensive testing infrastructure for real cloud environments, Kubernetes clusters, and network devices. Currently, many epics reference the need for "real environment testing" but defer implementation, creating a growing gap between unit/mock testing and production readiness.

**Epic Type**: Infrastructure, Testing, CI/CD

**Scope**:
- Real Kubernetes cluster testing (EKS, GKE, AKS)
- Multi-cloud CI/CD pipelines (AWS, GCP, Azure)
- Real network device testing for proxy agents
- Chaos engineering test framework
- Cloud provider attestation testing (SPIFFE)
- Performance baseline tracking with regression alerting
- Security testing infrastructure (auth/authz/audit)

**Out of Scope**:
- Cross-platform testing (see Epic 32)
- Local/container-based E2E testing (covered by Epic 12)
- Mock-based integration testing (existing infrastructure)
- Production deployment (operational concern)

## Rationale

### Problem Statement

Multiple epics identify the need for real environment testing but defer implementation:

| Source | Item | Status |
|--------|------|--------|
| Epic 8 | Add CI/CD jobs for real K8s cluster testing (EKS, GKE, AKS) | Deferred |
| Epic 12 | Add multi-cloud CI/CD pipelines for real cluster testing | Deferred |
| Epic 12 | Implement chaos engineering tests | Deferred |
| Epic 12 | Add comprehensive security testing | Deferred |
| Epic 12 | Implement performance baseline tracking with alerting | Deferred |
| Epic 14 | Characterize supercluster failover and recovery times | Incomplete |
| Epic 17 | Expand cloud provider attestation testing to real environments | Deferred |
| Epic 21 | Add real network device testing to CI/CD | Deferred |

This creates significant risk:
1. **Production Surprises**: Issues discovered only after deployment
2. **Cloud Parity Gaps**: AWS well-tested, GCP/Azure less so
3. **No Chaos Resilience**: Untested failure modes
4. **Security Blind Spots**: Auth/authz tested with mocks only
5. **Performance Unknown**: No baselines, no regression detection

### Benefits

1. **Production Confidence**: Tested in environments matching production
2. **Cloud Parity**: Equal coverage across AWS, GCP, Azure
3. **Resilience Validation**: Chaos testing proves failure handling
4. **Security Assurance**: Real auth/authz flows tested
5. **Performance Visibility**: Baselines and regression detection
6. **Network Device Coverage**: Real device testing for proxy agents

## Objectives

1. **O1**: Establish Kubernetes testing on EKS, GKE, and AKS with automated cluster lifecycle
2. **O2**: Create multi-cloud infrastructure provisioning for integration tests
3. **O3**: Implement chaos engineering framework with fault injection
4. **O4**: Enable real network device testing with device lab integration
5. **O5**: Achieve cloud provider attestation testing in real environments
6. **O6**: Establish performance baselines with automated regression alerting
7. **O7**: Implement comprehensive security testing covering auth/authz/audit
8. **O8**: Create cost-optimized ephemeral environment management

## Architecture

### Multi-Cloud Test Infrastructure

```
┌─────────────────────────────────────────────────────────────────────────┐
│                     Multi-Cloud Test Infrastructure                      │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌────────────────────────────────────────────────────────────────────┐ │
│  │                     Test Orchestration Layer                        │ │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                │ │
│  │  │   GitHub    │  │   GitLab    │  │   Azure     │                │ │
│  │  │   Actions   │  │   CI/CD     │  │   DevOps    │                │ │
│  │  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘                │ │
│  │         └────────────────┼────────────────┘                        │ │
│  └──────────────────────────┼─────────────────────────────────────────┘ │
│                             │                                            │
│  ┌──────────────────────────▼─────────────────────────────────────────┐ │
│  │                   Environment Provisioner                           │ │
│  │  - Terraform modules per cloud                                      │ │
│  │  - Kubernetes cluster lifecycle                                     │ │
│  │  - Network device lab access                                        │ │
│  │  - Cost tracking and cleanup                                        │ │
│  └──────────────────────────┬─────────────────────────────────────────┘ │
│                             │                                            │
│    ┌────────────────────────┼────────────────────────────┐              │
│    │                        │                            │              │
│    ▼                        ▼                            ▼              │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐              │
│  │     AWS      │    │     GCP      │    │    Azure     │              │
│  │              │    │              │    │              │              │
│  │  ┌────────┐  │    │  ┌────────┐  │    │  ┌────────┐  │              │
│  │  │  EKS   │  │    │  │  GKE   │  │    │  │  AKS   │  │              │
│  │  └────────┘  │    │  └────────┘  │    │  └────────┘  │              │
│  │  ┌────────┐  │    │  ┌────────┐  │    │  ┌────────┐  │              │
│  │  │  EC2   │  │    │  │  GCE   │  │    │  │  VMs   │  │              │
│  │  └────────┘  │    │  └────────┘  │    │  └────────┘  │              │
│  │  ┌────────┐  │    │  ┌────────┐  │    │  ┌────────┐  │              │
│  │  │  IRSA  │  │    │  │  WI    │  │    │  │  MI    │  │              │
│  │  └────────┘  │    │  └────────┘  │    │  └────────┘  │              │
│  └──────────────┘    └──────────────┘    └──────────────┘              │
│                                                                          │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                      Network Device Lab                           │   │
│  │  ┌────────┐  ┌────────┐  ┌────────┐  ┌────────┐  ┌────────┐     │   │
│  │  │ Cisco  │  │Juniper │  │ Arista │  │  VyOS  │  │pfSense │     │   │
│  │  │  IOS   │  │ JUNOS  │  │  EOS   │  │        │  │        │     │   │
│  │  └────────┘  └────────┘  └────────┘  └────────┘  └────────┘     │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

### Chaos Engineering Framework

```
┌─────────────────────────────────────────────────────────────────┐
│                    Chaos Engineering Framework                   │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                    Chaos Controller                       │   │
│  │  - Experiment scheduling                                  │   │
│  │  - Steady-state verification                              │   │
│  │  - Rollback triggers                                      │   │
│  │  - Result collection                                      │   │
│  └────────────────────────┬─────────────────────────────────┘   │
│                           │                                      │
│         ┌─────────────────┼─────────────────┐                   │
│         │                 │                 │                    │
│         ▼                 ▼                 ▼                    │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │   Network    │  │   Resource   │  │   Service    │          │
│  │   Chaos      │  │   Chaos      │  │   Chaos      │          │
│  │              │  │              │  │              │          │
│  │ - Partition  │  │ - CPU stress │  │ - Pod kill   │          │
│  │ - Latency    │  │ - Memory     │  │ - Node drain │          │
│  │ - Packet     │  │ - Disk fill  │  │ - Container  │          │
│  │   loss       │  │ - IO stress  │  │   crash      │          │
│  └──────────────┘  └──────────────┘  └──────────────┘          │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                    Steady State Probes                    │   │
│  │  - Agent connectivity                                     │   │
│  │  - Command execution latency                              │   │
│  │  - State application success rate                         │   │
│  │  - Event delivery latency                                 │   │
│  │  - API response times                                     │   │
│  └──────────────────────────────────────────────────────────┘   │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### Directory Structure

```
test/
├── cloud/
│   ├── aws/
│   │   ├── terraform/           # AWS infrastructure
│   │   ├── eks/                 # EKS cluster tests
│   │   ├── ec2/                 # EC2 VM tests
│   │   └── irsa/                # IRSA attestation tests
│   ├── gcp/
│   │   ├── terraform/           # GCP infrastructure
│   │   ├── gke/                 # GKE cluster tests
│   │   ├── gce/                 # GCE VM tests
│   │   └── workload-identity/   # WI attestation tests
│   ├── azure/
│   │   ├── terraform/           # Azure infrastructure
│   │   ├── aks/                 # AKS cluster tests
│   │   ├── vms/                 # Azure VM tests
│   │   └── managed-identity/    # MI attestation tests
│   └── shared/
│       ├── provisioner.go       # Multi-cloud provisioning
│       ├── cost_tracker.go      # Cost monitoring
│       └── cleanup.go           # Resource cleanup
├── chaos/
│   ├── framework/
│   │   ├── controller.go        # Chaos experiment controller
│   │   ├── probes.go            # Steady-state probes
│   │   └── rollback.go          # Automatic rollback
│   ├── network/
│   │   ├── partition_test.go    # Network partition tests
│   │   ├── latency_test.go      # Latency injection tests
│   │   └── packet_loss_test.go  # Packet loss tests
│   ├── resource/
│   │   ├── cpu_stress_test.go   # CPU stress tests
│   │   ├── memory_test.go       # Memory pressure tests
│   │   └── disk_test.go         # Disk fill tests
│   └── service/
│       ├── pod_kill_test.go     # Pod termination tests
│       ├── node_drain_test.go   # Node drain tests
│       └── leader_failover.go   # Leader election chaos
├── network-devices/
│   ├── lab/
│   │   ├── inventory.yaml       # Device inventory
│   │   ├── credentials.go       # Secure credential access
│   │   └── topology.go          # Lab topology management
│   ├── cisco/
│   │   └── ios_test.go          # Cisco IOS tests
│   ├── juniper/
│   │   └── junos_test.go        # Juniper JUNOS tests
│   ├── arista/
│   │   └── eos_test.go          # Arista EOS tests
│   └── virtual/
│       ├── vyos_test.go         # VyOS tests
│       └── pfsense_test.go      # pfSense tests
├── security/
│   ├── authn/
│   │   ├── mtls_test.go         # mTLS authentication tests
│   │   ├── apikey_test.go       # API key tests
│   │   └── jwt_test.go          # JWT authentication tests
│   ├── authz/
│   │   ├── rbac_test.go         # RBAC tests
│   │   ├── policy_test.go       # Policy enforcement tests
│   │   └── targeting_test.go    # Agent targeting authz
│   └── audit/
│       ├── event_test.go        # Audit event tests
│       ├── retention_test.go    # Log retention tests
│       └── tamper_test.go       # Tamper detection tests
└── performance/
    ├── baselines/
    │   ├── agent_registration.go    # Registration baseline
    │   ├── command_execution.go     # Execution baseline
    │   ├── state_application.go     # State apply baseline
    │   └── event_processing.go      # Event baseline
    ├── tracking/
    │   ├── collector.go             # Metrics collection
    │   ├── comparator.go            # Baseline comparison
    │   └── alerter.go               # Regression alerting
    └── reports/
        └── generator.go             # Performance reports
```

## Deliverables

### D1: Kubernetes Cluster Test Infrastructure

Automated Kubernetes cluster provisioning and testing.

**Components**:
- Terraform modules for EKS, GKE, AKS
- Cluster lifecycle management (create, test, destroy)
- kubeconfig management
- Cost tracking and budget alerts

**Tests**:
- Control plane deployment on Kubernetes
- Agent DaemonSet deployment
- Kubernetes state modules (namespace, deployment, service, etc.)
- Operator controller functionality
- Cluster-scoped vs namespace-scoped resources

### D2: Cloud Provider Integration Tests

Real cloud provider integration testing.

**AWS**:
- EC2 instance management
- IRSA (IAM Roles for Service Accounts) attestation
- AWS metadata service detection
- S3 file distribution backend
- CloudWatch metrics integration

**GCP**:
- GCE instance management
- Workload Identity attestation
- GCP metadata service detection
- GCS file distribution backend
- Cloud Monitoring integration

**Azure**:
- VM management
- Managed Identity attestation
- Azure metadata service detection
- Azure Blob file distribution backend
- Azure Monitor integration

### D3: Chaos Engineering Framework

Comprehensive chaos testing framework.

**Network Chaos**:
- Network partition simulation
- Latency injection (50ms, 100ms, 500ms)
- Packet loss (1%, 5%, 10%)
- DNS failure simulation

**Resource Chaos**:
- CPU stress (50%, 80%, 100%)
- Memory pressure
- Disk fill
- IO latency

**Service Chaos**:
- Pod termination
- Node drain
- Container crash
- Leader failover
- NATS cluster partition

### D4: Network Device Test Lab

Real network device testing infrastructure.

**Physical/Virtual Devices**:
- Cisco IOS (virtual or physical)
- Juniper JUNOS (vSRX or physical)
- Arista EOS (cEOS or physical)
- VyOS (virtual)
- pfSense (virtual)
- OPNsense (virtual)

**Tests**:
- SSH connectivity
- SNMP polling
- Configuration backup/restore
- State module application
- Credential rotation

### D5: Security Test Suite

Comprehensive security testing.

**Authentication**:
- mTLS certificate validation
- Certificate expiration handling
- API key lifecycle
- JWT token validation
- SPIFFE SVID verification

**Authorization**:
- RBAC enforcement
- Policy engine integration
- Agent targeting restrictions
- Cross-tenant isolation

**Audit**:
- Audit event completeness
- Event ordering guarantees
- Tamper detection
- Retention policy enforcement

### D6: Performance Baseline System

Automated performance tracking and regression detection.

**Baselines**:
- Agent registration throughput (agents/sec)
- Command execution latency (p50, p95, p99)
- State application throughput (states/sec)
- Event processing latency (ms)
- API response times (ms)

**Features**:
- Automatic baseline collection
- Statistical comparison (t-test, percentile comparison)
- Regression alerting (Slack, PagerDuty)
- Historical trend visualization
- Performance report generation

### D7: Cost Management System

Cloud cost tracking and optimization.

**Features**:
- Per-test cost attribution
- Budget alerts
- Automatic resource cleanup
- Spot/preemptible instance usage
- Cost reports per CI run

### D8: Multi-Cloud CI Pipelines

GitHub Actions workflows for multi-cloud testing.

**Workflows**:
- `cloud-aws.yaml` - AWS integration tests
- `cloud-gcp.yaml` - GCP integration tests
- `cloud-azure.yaml` - Azure integration tests
- `chaos.yaml` - Chaos engineering tests
- `security.yaml` - Security test suite
- `performance.yaml` - Performance baseline tracking
- `network-devices.yaml` - Network device tests

### D9: Documentation

Comprehensive documentation for cloud testing.

**Contents**:
- Cloud provider setup guides
- Terraform module reference
- Chaos experiment catalog
- Network device lab setup
- Performance baseline interpretation
- Cost optimization guide

## Acceptance Criteria

### AC1: Kubernetes Testing Operational
- [ ] EKS cluster tests pass in CI
- [ ] GKE cluster tests pass in CI
- [ ] AKS cluster tests pass in CI
- [ ] Cluster lifecycle automated (create/test/destroy)

### AC2: Cloud Provider Parity
- [ ] AWS integration tests comprehensive
- [ ] GCP integration tests match AWS coverage
- [ ] Azure integration tests match AWS coverage
- [ ] Attestation tested on all three providers

### AC3: Chaos Engineering Functional
- [ ] Network partition tests implemented
- [ ] Resource stress tests implemented
- [ ] Service failure tests implemented
- [ ] Steady-state verification working

### AC4: Network Device Testing Active
- [ ] At least 3 device types tested
- [ ] SSH and SNMP protocols tested
- [ ] Configuration state modules tested
- [ ] Credential rotation tested

### AC5: Security Testing Complete
- [ ] Authentication scenarios tested
- [ ] Authorization scenarios tested
- [ ] Audit completeness verified
- [ ] No security test gaps

### AC6: Performance Baselines Established
- [ ] All key metrics baselined
- [ ] Regression detection working
- [ ] Alerting configured
- [ ] Reports generated

### AC7: Cost Management Active
- [ ] Per-test cost tracking
- [ ] Budget alerts configured
- [ ] Automatic cleanup working
- [ ] Cost reports available

## Sub-Issues / Tasks

### Phase 1: Cloud Infrastructure Foundation (Weeks 1-4)

#### T1.1: AWS Terraform Modules
Create Terraform modules for AWS test infrastructure.

**Deliverables**:
- EKS cluster module
- EC2 instance module
- IAM roles for testing
- VPC and networking

#### T1.2: GCP Terraform Modules
Create Terraform modules for GCP test infrastructure.

**Deliverables**:
- GKE cluster module
- GCE instance module
- Service accounts and IAM
- VPC and networking

#### T1.3: Azure Terraform Modules
Create Terraform modules for Azure test infrastructure.

**Deliverables**:
- AKS cluster module
- VM module
- Managed identities
- VNet and networking

#### T1.4: Multi-Cloud Provisioner
Go library for multi-cloud provisioning.

**Deliverables**:
- `test/cloud/shared/provisioner.go`
- Unified API for all clouds
- Parallel provisioning
- Cleanup automation

### Phase 2: Kubernetes Testing (Weeks 5-8)

#### T2.1: EKS Integration Tests
Implement EKS-specific integration tests.

**Deliverables**:
- Control plane deployment tests
- Agent DaemonSet tests
- IRSA attestation tests
- EKS-specific module tests

#### T2.2: GKE Integration Tests
Implement GKE-specific integration tests.

**Deliverables**:
- Control plane deployment tests
- Agent DaemonSet tests
- Workload Identity tests
- GKE-specific module tests

#### T2.3: AKS Integration Tests
Implement AKS-specific integration tests.

**Deliverables**:
- Control plane deployment tests
- Agent DaemonSet tests
- Managed Identity tests
- AKS-specific module tests

#### T2.4: Kubernetes Module Tests
Test all Kubernetes state modules on real clusters.

**Deliverables**:
- Namespace module tests
- Deployment module tests
- Service module tests
- ConfigMap/Secret tests
- All 12 K8s modules tested

### Phase 3: Chaos Engineering (Weeks 9-12)

#### T3.1: Chaos Framework Core
Implement the chaos engineering framework.

**Deliverables**:
- `test/chaos/framework/controller.go`
- Experiment definition format
- Steady-state probes
- Automatic rollback

#### T3.2: Network Chaos Tests
Implement network chaos experiments.

**Deliverables**:
- Network partition tests
- Latency injection tests
- Packet loss tests
- DNS failure tests

#### T3.3: Resource Chaos Tests
Implement resource exhaustion experiments.

**Deliverables**:
- CPU stress tests
- Memory pressure tests
- Disk fill tests
- IO stress tests

#### T3.4: Service Chaos Tests
Implement service failure experiments.

**Deliverables**:
- Pod kill tests
- Node drain tests
- Leader failover tests
- NATS partition tests

### Phase 4: Network Device Testing (Weeks 13-16)

#### T4.1: Network Device Lab Setup
Set up network device test lab.

**Deliverables**:
- Device inventory management
- Secure credential access
- Lab topology automation
- Virtual device provisioning

#### T4.2: Virtual Device Tests
Implement tests for virtual network devices.

**Deliverables**:
- VyOS integration tests
- pfSense integration tests
- OPNsense integration tests

#### T4.3: Vendor-Specific Tests
Implement tests for vendor devices.

**Deliverables**:
- Cisco IOS tests
- Juniper JUNOS tests
- Arista EOS tests

### Phase 5: Security Testing (Weeks 17-20)

#### T5.1: Authentication Tests
Implement authentication test suite.

**Deliverables**:
- mTLS tests
- API key tests
- JWT tests
- SPIFFE tests

#### T5.2: Authorization Tests
Implement authorization test suite.

**Deliverables**:
- RBAC tests
- Policy engine tests
- Targeting tests
- Cross-tenant tests

#### T5.3: Audit Tests
Implement audit test suite.

**Deliverables**:
- Event completeness tests
- Ordering tests
- Tamper detection tests
- Retention tests

### Phase 6: Performance Baselines (Weeks 21-24)

#### T6.1: Baseline Collection System
Implement performance baseline collection.

**Deliverables**:
- Metrics collector
- Baseline storage
- Statistical analysis
- Historical tracking

#### T6.2: Regression Detection
Implement regression detection and alerting.

**Deliverables**:
- Baseline comparator
- Statistical thresholds
- Alert integration
- Report generation

#### T6.3: CI Integration
Integrate performance tracking into CI.

**Deliverables**:
- Performance CI workflow
- Artifact storage
- Dashboard integration
- Trend visualization

### Phase 7: Cost Management (Weeks 25-26)

#### T7.1: Cost Tracking
Implement cost tracking per test.

**Deliverables**:
- Cloud billing API integration
- Per-test attribution
- Cost aggregation
- Budget monitoring

#### T7.2: Cost Optimization
Implement cost optimization features.

**Deliverables**:
- Spot instance usage
- Automatic cleanup
- Resource right-sizing
- Cost reports

### Phase 8: Documentation and Polish (Weeks 27-28)

#### T8.1: Documentation
Write comprehensive documentation.

**Deliverables**:
- Cloud setup guides
- Chaos experiment catalog
- Network device guide
- Performance interpretation guide

#### T8.2: CI Workflows
Finalize all CI workflows.

**Deliverables**:
- All cloud workflows
- Nightly schedules
- Cost budget enforcement
- Alert integration

## Dependencies

- **Epic 8** (Multi-Environment): Provides cloud provider modules
- **Epic 12** (E2E Testing): Base testing patterns
- **Epic 17** (SPIFFE): Cloud attestation implementation
- **Epic 21** (Proxy Agents): Network device protocols
- **Cloud Provider Accounts**: AWS, GCP, Azure with billing
- **Network Device Lab**: Physical or virtual device access

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Cloud costs | Budget overrun | Strict budgets, spot instances, auto-cleanup |
| Network device access | Test gaps | Virtual devices for CI, physical for nightly |
| Chaos test instability | CI flakiness | Conservative experiments, automatic rollback |
| Multi-cloud complexity | Maintenance burden | Unified provisioner, shared patterns |
| Secret management | Security exposure | Vault integration, short-lived credentials |

## Success Metrics

| Metric | Target |
|--------|--------|
| Cloud providers tested | 3 (AWS, GCP, Azure) |
| Kubernetes distributions | 3 (EKS, GKE, AKS) |
| Chaos experiment types | 10+ |
| Network device types | 5+ |
| Security test scenarios | 20+ |
| Performance metrics tracked | 10+ |
| CI pass rate | >90% |
| Monthly cloud cost | <$500 |

## Definition of Done

- [ ] All deliverables (D1-D9) implemented
- [ ] All acceptance criteria (AC1-AC7) met
- [ ] CI pipelines passing for all clouds
- [ ] Chaos experiments documented and operational
- [ ] Network device tests running in CI
- [ ] Security test suite comprehensive
- [ ] Performance baselines established
- [ ] Cost tracking operational
- [ ] Documentation complete
- [ ] TODO.md items marked complete

## Future Considerations

- Additional cloud providers (Oracle Cloud, IBM Cloud)
- Additional Kubernetes distributions (OpenShift, Rancher)
- Expanded chaos scenarios (multi-region, disaster recovery)
- Additional network vendors (Fortinet, Palo Alto)
- Continuous compliance testing (CIS benchmarks)
- Automated remediation testing
