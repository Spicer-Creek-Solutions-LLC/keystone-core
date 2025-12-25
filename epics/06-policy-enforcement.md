# Epic 6: Policy Enforcement & Compliance

## Overview

Implement a comprehensive policy enforcement system that provides continuous compliance monitoring, automated remediation, and policy-as-code capabilities across hybrid infrastructure.

**Goal**: Enable organizations to define, enforce, and audit security and compliance policies continuously across all managed infrastructure, with automated remediation and detailed compliance reporting.

## Success Criteria

- [ ] Policy-as-code using OPA (Rego) and CEL
- [ ] Continuous policy evaluation (real-time and scheduled)
- [ ] Automated policy violation remediation
- [ ] Compliance reporting and dashboards
- [ ] Integration with compliance frameworks (CIS, PCI-DSS, SOC2)
- [ ] Policy testing and validation framework
- [ ] Audit logging for all policy evaluations
- [ ] Support for custom policies
- [ ] Policy evaluation latency <100ms
- [ ] SPIFFE/SPIRE integration for zero-trust authentication
- [ ] Identity-based authorization using SPIFFE selectors
- [ ] Support for both manual TLS and SPIRE security modes

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│                Policy Definition Layer                   │
│  ┌────────────┐  ┌────────────┐  ┌────────────────┐    │
│  │    OPA     │  │    CEL     │  │   Custom       │    │
│  │  Policies  │  │  Policies  │  │   Validators   │    │
│  │  (.rego)   │  │            │  │                │    │
│  └────────────┘  └────────────┘  └────────────────┘    │
└──────────────────────────────────────────────────────────┘
                          │
                          ▼
┌──────────────────────────────────────────────────────────┐
│              Policy Enforcement Engine                   │
│  ┌────────────┐  ┌────────────┐  ┌────────────────┐    │
│  │   Policy   │  │  Evaluator │  │   Remediation  │    │
│  │  Compiler  │  │            │  │   Engine       │    │
│  └────────────┘  └────────────┘  └────────────────┘    │
│  ┌────────────┐  ┌────────────┐  ┌────────────────┐    │
│  │   Audit    │  │  Reporter  │  │   Scheduler    │    │
│  │   Logger   │  │            │  │                │    │
│  └────────────┘  └────────────┘  └────────────────┘    │
└──────────────────────────────────────────────────────────┘
                          │
                          ▼
                   Infrastructure
           (K8s, VMs, Cloud Resources)
```

## User Stories

### US6.1: Define Policies as Code
**As a** security engineer
**I want to** define security policies as code
**So that** policies are version-controlled and auditable

**Acceptance Criteria**:
- Write policies in OPA Rego
- Write policies in CEL (Common Expression Language)
- Organize policies in Git repositories
- Policy versioning and rollback
- Policy testing framework
- Policy documentation generation

**Example OPA Policy**:
```rego
# policies/no-root-containers.rego
package kubernetes.admission

deny[msg] {
    input.request.kind.kind == "Pod"
    container := input.request.object.spec.containers[_]
    container.securityContext.runAsUser == 0
    msg := sprintf("Container '%s' is running as root user", [container.name])
}

deny[msg] {
    input.request.kind.kind == "Pod"
    not input.request.object.spec.securityContext.runAsNonRoot
    msg := "Pod must set securityContext.runAsNonRoot=true"
}
```

**Example CEL Policy**:
```yaml
# policies/resource-limits.yaml
apiVersion: policy.titananvil.io/v1
kind: Policy
metadata:
  name: enforce-resource-limits
spec:
  targets:
    - kind: Pod
      namespaces: ["*"]
  rules:
    - name: cpu-limit-required
      expression: |
        object.spec.containers.all(c, has(c.resources.limits.cpu))
      message: "All containers must have CPU limits"

    - name: memory-limit-required
      expression: |
        object.spec.containers.all(c, has(c.resources.limits.memory))
      message: "All containers must have memory limits"

    - name: reasonable-cpu-limit
      expression: |
        object.spec.containers.all(c,
          quantity(c.resources.limits.cpu) <= quantity("4000m")
        )
      message: "CPU limit must not exceed 4 cores"
```

### US6.2: Continuous Policy Evaluation
**As a** security engineer
**I want to** continuously evaluate policies against infrastructure
**So that** violations are detected in real-time

**Acceptance Criteria**:
- Real-time evaluation on state changes
- Scheduled evaluation (cron-based)
- On-demand evaluation via CLI/API
- Evaluation results stored and queryable
- Policy evaluation metrics
- Support for different evaluation modes (enforce, audit, warn)

**Evaluation Modes**:
```yaml
policies:
  no-root-containers:
    mode: enforce  # Block violations
    targets: ["k8s:*"]

  encryption-at-rest:
    mode: audit    # Log violations, don't block
    targets: ["aws:rds:*", "aws:s3:*"]

  deprecated-api-versions:
    mode: warn     # Warn but allow
    targets: ["k8s:*"]
```

**Example Usage**:
```bash
# Run all policies against current state
titanctl policy eval --all

# Run specific policy
titanctl policy eval no-root-containers --target "k8s:namespace=production"

# Schedule continuous evaluation
titanctl policy schedule --policy security-baseline --interval 5m
```

### US6.3: Automated Remediation
**As a** security engineer
**I want to** automatically remediate policy violations
**So that** compliance is maintained without manual intervention

**Acceptance Criteria**:
- Define remediation actions per policy
- Support multiple remediation strategies
- Approval workflows for critical remediations
- Remediation dry-run mode
- Rollback failed remediations
- Track remediation history

**Remediation Strategies**:
```yaml
# policies/file-permissions.yaml
apiVersion: policy.titananvil.io/v1
kind: Policy
metadata:
  name: secure-file-permissions
spec:
  description: "Ensure sensitive files have correct permissions"

  checks:
    - name: ssh-key-permissions
      type: file
      path: "/home/*/.ssh/id_rsa"
      mode: "0600"

  remediation:
    auto: true
    actions:
      - type: command
        command: "chmod 0600 /home/*/.ssh/id_rsa"
      - type: alert
        channels: ["security-team"]

---
# policies/pod-security.yaml
apiVersion: policy.titananvil.io/v1
kind: Policy
metadata:
  name: pod-security-standards
spec:
  description: "Enforce Kubernetes Pod Security Standards"

  checks:
    - name: privileged-containers
      type: k8s
      query: |
        pods.spec.containers[].securityContext.privileged == true

  remediation:
    auto: false  # Require manual approval
    approval_required: true
    approvers: ["@security-team"]
    actions:
      - type: k8s_delete
        resource: "pod"
      - type: alert
        message: "Privileged pod detected and removed"
```

### US6.4: Compliance Frameworks
**As a** compliance officer
**I want to** map policies to compliance frameworks
**So that** I can prove compliance with regulations

**Acceptance Criteria**:
- Pre-built policy bundles for CIS Benchmarks
- Support for PCI-DSS requirements
- Support for SOC2 controls
- Support for HIPAA requirements
- Map custom policies to controls
- Generate compliance reports
- Export audit evidence

**Example**:
```yaml
# policies/cis-benchmark.yaml
apiVersion: policy.titananvil.io/v1
kind: PolicyBundle
metadata:
  name: cis-kubernetes-benchmark-v1.8
spec:
  framework: CIS
  version: "1.8.0"
  policies:
    - name: "1.1.1-api-server-anonymous-auth"
      description: "Ensure that the --anonymous-auth argument is set to false"
      control: "1.1.1"
      severity: high
      check:
        type: command
        command: |
          ps aux | grep kube-apiserver | grep -v grep | grep "anonymous-auth=false"

    - name: "5.2.1-pod-security-policy"
      description: "Minimize the admission of privileged containers"
      control: "5.2.1"
      severity: high
      check:
        type: k8s_policy
        policy: no-privileged-containers

    - name: "5.7.3-limit-capabilities"
      description: "Apply SecurityContext to Pods and Containers"
      control: "5.7.3"
      severity: medium
      check:
        type: k8s_policy
        policy: drop-all-capabilities
```

**Compliance Report**:
```bash
titanctl compliance report --framework cis-k8s-1.8

CIS Kubernetes Benchmark v1.8.0
Compliance Report - 2024-01-15

Overall Compliance: 87% (130/150 controls passed)

Section 1: Control Plane Components
  ✅ 1.1.1 API Server anonymous auth disabled
  ✅ 1.1.2 API Server basic auth disabled
  ❌ 1.1.3 API Server token auth file not set (FAILED)

Section 5: Policies
  ✅ 5.2.1 No privileged containers
  ⚠️  5.2.2 Host network usage (3 violations in dev namespace)

Failed Controls: 20
  - Critical: 2
  - High: 8
  - Medium: 10

Remediation Required: 20 controls
Estimated time to remediate: 4 hours
```

### US6.5: Policy Testing
**As a** security engineer
**I want to** test policies before deploying them
**So that** I don't break production systems

**Acceptance Criteria**:
- Unit tests for policies
- Test policies against sample data
- Integration tests with live systems
- Policy simulation mode
- Policy impact analysis
- CI/CD integration for policy testing

**Example**:
```rego
# policies/no-root-containers.rego
package kubernetes.admission

deny[msg] {
    input.request.kind.kind == "Pod"
    container := input.request.object.spec.containers[_]
    container.securityContext.runAsUser == 0
    msg := sprintf("Container '%s' is running as root", [container.name])
}

# policies/no-root-containers_test.rego
package kubernetes.admission

test_deny_root_container {
    deny["Container 'nginx' is running as root"] with input as {
        "request": {
            "kind": {"kind": "Pod"},
            "object": {
                "spec": {
                    "containers": [{
                        "name": "nginx",
                        "securityContext": {"runAsUser": 0}
                    }]
                }
            }
        }
    }
}

test_allow_nonroot_container {
    count(deny) == 0 with input as {
        "request": {
            "kind": {"kind": "Pod"},
            "object": {
                "spec": {
                    "containers": [{
                        "name": "nginx",
                        "securityContext": {"runAsUser": 1000}
                    }]
                }
            }
        }
    }
}
```

```bash
# Run policy tests
titanctl policy test policies/

# Test policy against live cluster (dry-run)
titanctl policy eval no-root-containers --dry-run --target k8s:production

# Show what would be remediated
titanctl policy remediate --dry-run --all
```

### US6.6: Drift Detection for Compliance
**As a** security engineer
**I want to** detect when systems drift from compliant state
**So that** I can maintain continuous compliance

**Acceptance Criteria**:
- Continuous monitoring of compliant resources
- Alert on compliance drift
- Track time-to-detection for violations
- Automated remediation of drift
- Compliance drift reporting
- Integration with SIEM systems

**Example**:
```yaml
# Compliance drift monitoring
titanctl policy monitor --policy pci-dss --auto-remediate

Monitoring compliance drift...

[2024-01-15 10:30:45] DRIFT DETECTED
  Resource: aws:ec2:instance:i-12345
  Policy: encryption-at-rest
  Violation: EBS volume vol-67890 is not encrypted
  Action: Auto-remediation triggered
  Status: Creating encrypted volume snapshot...

[2024-01-15 10:31:00] REMEDIATION COMPLETE
  Resource: aws:ec2:instance:i-12345
  Action: Volume replaced with encrypted version
  Time to remediate: 15s

[2024-01-15 10:35:00] DRIFT DETECTED
  Resource: k8s:pod:nginx-abc123
  Policy: no-root-containers
  Violation: Container running as UID 0
  Action: Approval required (critical resource)
  Status: Notification sent to security team
```

### US6.7: Policy Audit Trail
**As a** compliance officer
**I want to** maintain a complete audit trail of policy evaluations
**So that** I can provide evidence during audits

**Acceptance Criteria**:
- Log all policy evaluations with results
- Track policy changes (who, when, what)
- Record remediation actions
- Immutable audit logs
- Export audit logs for external systems
- Retention policies for audit data
- Search and filter audit logs

**Audit Log Format**:
```json
{
  "timestamp": "2024-01-15T10:30:45Z",
  "event_type": "policy.evaluation",
  "policy": {
    "name": "no-root-containers",
    "version": "1.2.0",
    "framework": "CIS-K8s-1.8",
    "control": "5.2.1"
  },
  "resource": {
    "type": "k8s:pod",
    "namespace": "production",
    "name": "nginx-abc123"
  },
  "result": "violation",
  "violation_details": {
    "container": "nginx",
    "issue": "running as root (UID 0)"
  },
  "remediation": {
    "action": "delete_pod",
    "status": "pending_approval",
    "approver": "user@example.com"
  },
  "user": "system",
  "correlation_id": "audit-12345"
}
```

## Technical Tasks

### Phase 1: Policy Engine Foundation (Week 1-2)

**T1.1: OPA Integration**
- Integrate OPA as library
- Load Rego policies from files/Git
- Compile and cache policies
- Evaluate policies with input data
- Handle policy bundles

**T1.2: CEL Integration**
- Integrate CEL library
- Parse CEL expressions
- Evaluate CEL policies
- Support custom CEL functions
- Type checking for CEL

**T1.3: Policy Registry**
- Store policies in state backend
- Version control for policies
- Policy metadata management
- Policy search and discovery
- Policy dependencies

### Phase 2: Evaluation Engine (Week 3-4)

**T2.1: Continuous Evaluation**
- Real-time evaluation on events
- Scheduled evaluation (cron)
- Batch evaluation for audits
- Parallel evaluation for performance
- Result caching

**T2.2: Data Collection**
- Gather infrastructure state
- Query Kubernetes resources
- Query cloud resources (AWS, GCP, Azure)
- Query file systems on agents
- Cache frequently accessed data

**T2.3: Evaluation Modes**
- Enforce mode (block violations)
- Audit mode (log violations)
- Warn mode (alert only)
- Dry-run mode (preview)
- Mode per policy or per target

### Phase 3: Remediation Engine (Week 5)

**T3.1: Remediation Actions**
- Execute remediation commands
- Apply state changes
- Update Kubernetes resources
- Modify cloud resources
- Custom remediation scripts

**T3.2: Approval Workflows**
- Define approval requirements
- Notification to approvers
- Approval UI/API
- Timeout handling
- Approval audit trail

**T3.3: Remediation Safety**
- Dry-run before execution
- Rollback on failure
- Rate limiting
- Blast radius limiting
- Emergency circuit breaker

### Phase 4: Compliance Frameworks (Week 6)

**T4.1: Framework Definitions**
- CIS Benchmarks (Kubernetes, Linux, Cloud)
- PCI-DSS control mappings
- SOC2 control mappings
- HIPAA control mappings
- Custom framework support

**T4.2: Compliance Reporting**
- Generate compliance reports
- Calculate compliance scores
- Identify gaps and recommendations
- Export to PDF/HTML/JSON
- Historical compliance trends

**T4.3: Evidence Collection**
- Collect audit evidence
- Screenshot/log capture
- Attestation generation
- Evidence archival
- Tamper-proof evidence storage

### Phase 5: Testing & Validation (Week 7)

**T5.1: Policy Testing Framework**
- Unit test framework for OPA
- Integration test framework
- Mock data generation
- Test result reporting
- CI/CD integration

**T5.2: Policy Validation**
- Syntax validation
- Semantic validation
- Conflict detection
- Impact analysis
- Performance profiling

**T5.3: Simulation Mode**
- Simulate policy application
- Impact prediction
- Affected resources reporting
- Risk assessment

### Phase 6: Audit & Reporting (Week 8)

**T6.1: Audit Logging**
- Log all policy evaluations
- Log remediation actions
- Log policy changes
- Structured log format
- Immutable log storage

**T6.2: Reporting Dashboard**
- Real-time compliance dashboard
- Violation trending
- Remediation statistics
- Policy coverage
- Risk scoring

**T6.3: Integration with SIEM**
- Export logs to Splunk
- Export logs to Elasticsearch
- Export to cloud SIEM (AWS Security Hub, Azure Sentinel)
- Syslog integration
- Custom webhook integration

### Phase 7: SPIFFE/SPIRE Security Integration (Week 9-10)

**T7.1: SPIRE Client Integration**
- Integrate SPIRE Workload API client library
- Implement X.509 SVID fetching from SPIRE agent
- Handle automatic SVID rotation (1-hour lifetime)
- Configure trust bundle updates
- Implement fallback to manual TLS mode

**T7.2: Identity-Based Authorization**
- Extract SPIFFE ID from agent connections
- Parse and validate SPIFFE selectors (labels, roles, platform attributes)
- Implement authorization policies based on SPIFFE identity
- Support for multi-tenant SPIFFE trust domains
- Policy examples:
  ```rego
  # Only allow agents with prod label to execute privileged commands
  allow {
    input.agent.spiffe_id.selectors["label:env"] == "prod"
    input.command.privileged == true
  }

  # Only allow K8s agents from specific namespace
  allow {
    input.agent.spiffe_id.selectors["k8s:ns"] == "kube-system"
    input.operation == "read-secrets"
  }
  ```

**T7.3: Platform Attestation Support**
- Configure node attestation plugins:
  - Kubernetes: Service Account Token validation
  - AWS: Instance Identity Document verification
  - GCP: Instance Identity Token verification
  - Azure: Managed Service Identity validation
  - Unix: Process UID/GID verification
- Implement workload attestation for TitanAnvil services
- Configure attestation policies per environment

**T7.4: SPIRE-Based Policy Enforcement**
- Policies reference SPIFFE selectors instead of static agent IDs
- Dynamic policy evaluation based on agent attestation
- Revoke access immediately when SPIFFE identity expires
- Audit trail includes SPIFFE identity for all actions
- Example policies:
  - "Only agents with `role=db-admin` can execute database commands"
  - "Agents without `compliant=true` cannot load privileged modules"
  - "Cross-region commands require `federated=true` selector"

**T7.5: mTLS Configuration**
- Configure gRPC server to use SPIRE-provided SVIDs
- Configure gRPC client to validate SPIRE trust bundle
- Implement automatic TLS certificate rotation from SPIRE
- Support for both SPIRE mode and manual TLS mode simultaneously (migration)
- Configuration:
  ```yaml
  security:
    mode: spire  # or "manual"
    spire:
      server_address: unix:///tmp/spire-server/api.sock
      trust_domain: titananvil.local
      node_attestor: k8s_sat
      agent_spiffe_id_template: "spiffe://{{.TrustDomain}}/agent/{{.AgentID}}"
    allow_legacy_tls: true  # For migration period
  ```

**T7.6: Module System Security (Integration with Epic 9)**
- Require SPIFFE identity for module loading
- Validate module SPIFFE selectors against required capabilities
- Example: Firewall module requires `capability:network.configure` selector
- Policy enforcement:
  ```rego
  # Only load module if SPIFFE identity has required capability
  allow_module_load {
    input.module.name == "vendor/firewall-manager"
    input.agent.spiffe_id.selectors["capability:network.configure"]
    input.agent.spiffe_id.selectors["role:network-admin"]
  }
  ```

**T7.7: Service Mesh Integration**
- Share SPIRE trust domain with Istio/Linkerd
- Enable TitanAnvil agents to authenticate to mesh services
- Mutual authentication between TitanAnvil and service mesh
- Unified zero-trust policy across infrastructure and applications

**T7.8: SPIRE Deployment Automation**
- Helm chart for SPIRE server deployment
- DaemonSet for SPIRE agents
- Automatic registration entries for TitanAnvil components
- SPIRE federation for multi-region/multi-cloud
- Monitoring and alerting for SPIRE health

## Dependencies

- **Epic 2**: Remote Execution (for remediation)
- **Epic 3**: State Management (for state-based policies)
- **Epic 4**: Event System (for real-time evaluation)
- **Go Libraries**:
  - `github.com/open-policy-agent/opa` - OPA engine
  - `github.com/google/cel-go` - CEL engine
  - `k8s.io/client-go` - Kubernetes client
  - Cloud SDKs (AWS, GCP, Azure)
  - `github.com/spiffe/go-spiffe/v2` - SPIRE Workload API client
  - `github.com/spiffe/spire-api-sdk` - SPIRE API SDK
- **External Services** (optional, for production):
  - SPIRE Server - Identity and attestation service
  - SPIRE Agents - Workload identity distribution

## Risks & Mitigations

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| Policy misconfiguration breaks production | Critical | Medium | Dry-run mode, testing framework, gradual rollout |
| Performance impact from continuous evaluation | High | Medium | Caching, incremental evaluation, sampling |
| False positives in violation detection | Medium | High | Policy testing, validation, tuning guidelines |
| Remediation causes outages | Critical | Low | Approval workflows, rollback capability, circuit breakers |
| Policy conflicts | Medium | Medium | Conflict detection, priority system, clear documentation |

## Metrics & Monitoring

### Key Metrics
- Policy evaluation latency (p50, p95, p99)
- Violation detection rate
- Remediation success rate
- Compliance score per framework
- Policy coverage percentage
- False positive rate

### Alerts
- Critical policy violations
- Remediation failures
- Compliance score drop >10%
- Policy evaluation errors
- Audit log storage capacity

## Testing Strategy

### Unit Tests
- OPA policy evaluation
- CEL expression evaluation
- Remediation action execution
- Compliance score calculation

### Integration Tests
- End-to-end policy enforcement
- Kubernetes admission control
- Cloud resource validation
- Remediation workflows

### Compliance Tests
- CIS Benchmark validation
- PCI-DSS control coverage
- SOC2 control coverage
- Framework-specific tests

## Documentation Requirements

- [ ] Policy writing guide (OPA and CEL)
- [ ] Remediation action reference
- [ ] Compliance framework mapping
- [ ] Policy testing guide
- [ ] Audit configuration guide
- [ ] Best practices for policy management
- [ ] Example policies library
- [ ] Troubleshooting guide

## Definition of Done

- [ ] All user stories implemented
- [ ] OPA and CEL engines integrated
- [ ] Continuous evaluation working
- [ ] Automated remediation functional
- [ ] Compliance reporting available
- [ ] Policy testing framework complete
- [ ] Audit logging operational
- [ ] Documentation complete
- [ ] Example policy bundles provided
- [ ] Production-ready
