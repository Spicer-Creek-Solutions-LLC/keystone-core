# Epic 5: GitOps Integration

## Overview

Integrate TitanAnvil with GitOps workflows to provide runtime operations, deployment verification, and automated rollback capabilities that complement declarative deployment tools like ArgoCD and Flux.

**Goal**: Position TitanAnvil as the operational control plane that bridges the gap between GitOps deployments and runtime infrastructure management.

## Success Criteria

- [ ] Receive and process webhooks from ArgoCD, Flux, GitLab, GitHub
- [ ] Automated deployment verification workflows
- [ ] Trigger GitOps rollbacks from TitanAnvil
- [ ] Bi-directional integration (TitanAnvil ↔ GitOps)
- [ ] Git repository as source of truth for TitanAnvil configs
- [ ] Deployment promotion workflows
- [ ] GitOps event forwarding to TitanAnvil event bus
- [ ] Health check and smoke test automation
- [ ] Integration with multiple GitOps tools

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│              Git Repository (Source of Truth)            │
│  • Application manifests (ArgoCD/Flux)                   │
│  • TitanAnvil states and reactors                        │
│  • Configuration and vars data                           │
└─────────────────┬────────────────────────────────────────┘
                  │
        ┌─────────┴─────────┐
        │                   │
        ▼                   ▼
┌──────────────┐    ┌──────────────┐
│   ArgoCD/    │    │  TitanAnvil  │
│    Flux      │    │   Watcher    │
│              │    │  (Git sync)  │
└──────┬───────┘    └──────┬───────┘
       │                   │
       │ Webhook           │ Pull states
       │                   │
       ▼                   ▼
┌──────────────────────────────────┐
│       TitanAnvil Control Plane   │
│  ┌────────────┐  ┌────────────┐ │
│  │  Webhook   │  │   GitOps   │ │
│  │  Receiver  │  │   Engine   │ │
│  └────────────┘  └────────────┘ │
│  ┌────────────┐  ┌────────────┐ │
│  │ Deployment │  │  Rollback  │ │
│  │ Verifier   │  │  Trigger   │ │
│  └────────────┘  └────────────┘ │
└──────────────────────────────────┘
       │
       │ Execute verification
       │ Trigger rollback if needed
       ▼
  Infrastructure
```

## User Stories

### US5.1: ArgoCD Integration
**As a** platform engineer
**I want to** integrate TitanAnvil with ArgoCD
**So that** deployments are verified and can be rolled back automatically

**Acceptance Criteria**:
- Receive ArgoCD webhooks (sync, health, deployment events)
- Parse ArgoCD application status
- Trigger verification workflows on sync completion
- Update ArgoCD application status from TitanAnvil
- Support ArgoCD progressive delivery (Rollouts)

**Workflow**:
```yaml
# ArgoCD triggers webhook on sync
ArgoCD: Deployment synced → TitanAnvil webhook

# TitanAnvil verifies deployment
TitanAnvil:
  1. Run health checks on new pods
  2. Execute smoke tests
  3. Check error rates in logs
  4. Verify metrics (latency, error rate)

# On failure, trigger rollback
If verification fails:
  → TitanAnvil creates Git PR to revert
  → OR TitanAnvil calls ArgoCD API to rollback
  → Alert on-call team
```

**Example Configuration**:
```yaml
# gitops/argocd-integration.yaml
argocd:
  servers:
    - name: production
      url: https://argocd.example.com
      token: ${ARGOCD_TOKEN}

  verification_workflows:
    web-app:
      trigger:
        application: "web-app"
        namespace: "production"
      steps:
        - name: "health_check"
          type: "command"
          command: "kubectl rollout status deployment/web-app -n production"
          timeout: "5m"

        - name: "smoke_test"
          type: "script"
          script: "./scripts/smoke-test.sh"
          args: ["https://web-app.example.com"]

        - name: "metrics_check"
          type: "prometheus_query"
          query: "rate(http_requests_total{status=~\"5..\",app=\"web-app\"}[5m])"
          threshold: "<0.01"  # Less than 1% 5xx errors

      on_failure:
        - type: "argocd_rollback"
          application: "web-app"
        - type: "alert"
          channels: ["slack", "pagerduty"]
```

### US5.2: Flux Integration
**As a** platform engineer
**I want to** integrate TitanAnvil with Flux
**So that** GitOps workflows include runtime verification

**Acceptance Criteria**:
- Monitor Flux Kustomization and HelmRelease resources
- Receive Flux events via Kubernetes events or webhooks
- Verify Flux reconciliations
- Trigger Flux suspend on verification failure
- Support Flux notification controller

**Example**:
```yaml
# gitops/flux-integration.yaml
flux:
  namespaces: ["flux-system"]

  verification_workflows:
    myapp-helm:
      trigger:
        kind: "HelmRelease"
        name: "myapp"
        namespace: "production"
      steps:
        - name: "verify_pods"
          type: "k8s_check"
          resource: "deployment/myapp"
          condition: "available"

        - name: "endpoint_check"
          type: "http_check"
          url: "http://myapp.production.svc.cluster.local/health"
          expected_status: 200

      on_failure:
        - type: "flux_suspend"
          resource: "HelmRelease/myapp"
        - type: "git_revert"
          repository: "github.com/myorg/gitops"
          path: "apps/myapp"
```

### US5.3: Git Repository Sync
**As a** platform engineer
**I want to** store TitanAnvil configurations in Git
**So that** all infrastructure automation is version controlled

**Acceptance Criteria**:
- Clone Git repositories containing states/reactors
- Watch for changes and sync automatically
- Support multiple Git repositories
- Validate configurations on sync
- Support Git branch strategies (main, environment branches)
- Handle merge conflicts gracefully

**Example**:
```yaml
# config/git-sync.yaml
git_repositories:
  - name: infrastructure-states
    url: git@github.com:myorg/titan-states.git
    branch: main
    path: /etc/titan/states
    sync_interval: 5m
    paths:
      states: states/
      vars: vars/
      reactors: reactors/

  - name: production-config
    url: git@github.com:myorg/titan-prod.git
    branch: production
    path: /etc/titan/production
    sync_interval: 1m
```

### US5.4: Deployment Verification Framework
**As a** platform engineer
**I want to** define verification workflows
**So that** deployments are validated automatically

**Acceptance Criteria**:
- Define verification steps (health checks, smoke tests, metrics)
- Support multiple verification types (HTTP, gRPC, command, script)
- Parallel and sequential step execution
- Configurable timeouts and retries
- Pass/fail criteria with thresholds
- Detailed verification reports

**Verification Types**:
```yaml
verification_steps:
  # Kubernetes resource check
  - type: k8s_check
    resource: "deployment/myapp"
    condition: "available"
    replicas: 3

  # HTTP endpoint check
  - type: http_check
    url: "https://myapp.example.com/health"
    expected_status: 200
    expected_body: '{"status":"healthy"}'
    timeout: 30s

  # gRPC health check
  - type: grpc_check
    address: "myapp.example.com:50051"
    service: "myapp.HealthService"

  # Prometheus query
  - type: prometheus_query
    query: "up{job='myapp'}"
    expected: "1"

  # Custom command
  - type: command
    command: "curl -f http://myapp/metrics"

  # Custom script
  - type: script
    script: "./verify-deployment.sh"
    args: ["myapp", "v1.2.3"]

  # Logs check
  - type: logs_check
    source: "kubectl logs -l app=myapp --tail=100"
    must_not_contain: ["ERROR", "FATAL", "panic"]
```

### US5.5: Automated Rollback
**As an** SRE
**I want to** automatically rollback failed deployments
**So that** incidents are mitigated quickly

**Acceptance Criteria**:
- Detect deployment failures via verification
- Multiple rollback strategies (GitOps, Kubernetes, direct)
- Rollback approval workflows (auto vs manual)
- Rollback to last known good version
- Create incident reports automatically
- Track rollback history

**Rollback Strategies**:
```yaml
rollback_strategies:
  # Git revert (GitOps way)
  git_revert:
    type: git
    repository: git@github.com:myorg/gitops.git
    action: create_pr  # or auto_merge
    reviewers: ["@sre-team"]

  # ArgoCD rollback
  argocd_rollback:
    type: argocd
    application: myapp
    revision: previous  # or specific SHA

  # Direct Kubernetes rollback
  k8s_rollback:
    type: kubectl
    resource: deployment/myapp
    revision: previous

  # Flux suspend and revert
  flux_suspend:
    type: flux
    resource: HelmRelease/myapp
    then: git_revert
```

### US5.6: Deployment Promotion Pipeline
**As a** platform engineer
**I want to** automate deployment promotion across environments
**So that** releases flow through staging to production safely

**Acceptance Criteria**:
- Promote deployments between environments
- Verification gates between environments
- Manual approval steps
- Automated promotion on success
- Rollback promotion on failure
- Audit trail of promotions

**Example Pipeline**:
```yaml
# gitops/promotion-pipeline.yaml
promotion_pipeline:
  application: myapp

  environments:
    - name: staging
      verification:
        - http_check
        - integration_tests
      auto_promote: true
      promotion_delay: 30m

    - name: production-canary
      verification:
        - http_check
        - metrics_check
      auto_promote: false  # Require manual approval
      approvers: ["@sre-team", "@product-team"]

    - name: production
      verification:
        - smoke_tests
        - traffic_validation
      auto_promote: true
      rollout_strategy: rolling

  on_failure:
    - halt_promotion
    - rollback_environment
    - alert: ["slack", "pagerduty"]
```

### US5.7: GitHub/GitLab Integration
**As a** platform engineer
**I want to** integrate with GitHub/GitLab
**So that** TitanAnvil can interact with Git workflows

**Acceptance Criteria**:
- Create pull requests from TitanAnvil
- Comment on PRs with verification results
- Update commit statuses
- Trigger TitanAnvil from GitHub Actions / GitLab CI
- Support GitHub/GitLab webhooks
- Authenticate via GitHub/GitLab apps

**Example**:
```yaml
# GitHub PR comment after verification
TitanAnvil Bot commented:
✅ Deployment verification passed

**Verification Results:**
- ✅ Health check: All pods healthy (3/3)
- ✅ Smoke tests: All tests passed (25/25)
- ✅ Error rate: 0.02% (threshold: <1%)
- ✅ Latency p95: 120ms (threshold: <200ms)

Deployment to production approved.
```

## Technical Tasks

### Phase 1: Webhook Infrastructure (Week 1)

**T1.1: Webhook Receiver**
- HTTP server for webhooks
- Support ArgoCD webhook format
- Support Flux webhook format
- Support GitHub/GitLab webhooks
- Webhook signature verification
- Webhook event parsing

**T1.2: Webhook Authentication**
- HMAC signature verification
- Bearer token authentication
- GitHub/GitLab app authentication
- ArgoCD token validation

**T1.3: Event Processing**
- Convert webhooks to TitanAnvil events
- Trigger reactors from webhooks
- Queue webhook processing
- Retry failed webhook processing

### Phase 2: GitOps Tool Integration (Week 2-3)

**T2.1: ArgoCD Integration**
- ArgoCD API client
- Parse ArgoCD application status
- Trigger ArgoCD sync/rollback
- Update ArgoCD app annotations
- Support ArgoCD progressive delivery

**T2.2: Flux Integration**
- Kubernetes client for Flux resources
- Watch Flux Kustomization/HelmRelease
- Suspend/Resume Flux resources
- Parse Flux events
- Support Flux notification controller

**T2.3: GitHub/GitLab Integration**
- GitHub/GitLab API clients
- Create pull requests
- Update commit statuses
- Comment on PRs
- List and merge PRs

### Phase 3: Verification Framework (Week 4-5)

**T3.1: Verification Engine**
- Execute verification workflows
- Support multiple verification types
- Parallel and sequential execution
- Timeout and retry handling
- Verification result aggregation

**T3.2: Verification Modules**
- HTTP health check
- gRPC health check
- Kubernetes resource check
- Prometheus query check
- Log analysis check
- Custom script execution

**T3.3: Verification Reporting**
- Generate verification reports
- Store verification history
- Export to external systems
- Create verification dashboards

### Phase 4: Git Sync (Week 6)

**T4.1: Git Repository Management**
- Clone Git repositories
- Pull updates on interval
- Handle authentication (SSH keys, tokens)
- Support multiple repositories
- Validate repository structure

**T4.2: Configuration Sync**
- Load states from Git
- Load reactors from Git
- Load vars data from Git
- Validate configurations
- Hot-reload on changes

**T4.3: Git Operations**
- Create commits
- Create branches
- Create pull requests
- Revert commits
- Merge strategies

### Phase 5: Rollback Automation (Week 7)

**T5.1: Rollback Engine**
- Detect rollback conditions
- Execute rollback strategies
- Track rollback operations
- Verify rollback success
- Rollback failure handling

**T5.2: Rollback Strategies**
- Git revert strategy
- ArgoCD rollback strategy
- Flux suspend strategy
- Kubernetes rollback strategy
- Custom rollback scripts

**T5.3: Approval Workflows**
- Manual approval gates
- Approval notifications
- Approval timeout handling
- Approval audit trail

### Phase 6: Promotion Pipelines (Week 8)

**T6.1: Pipeline Engine**
- Define promotion pipelines
- Execute pipeline stages
- Handle stage transitions
- Pipeline state management

**T6.2: Environment Promotion**
- Promote between environments
- Update Git refs/tags
- Trigger downstream deployments
- Track promotion history

**T6.3: Canary and Progressive Delivery**
- Support canary deployments
- Traffic shifting integration
- Metrics-based promotion
- Automatic rollback on failure

## Dependencies

- **Epic 2**: Remote Execution
- **Epic 3**: State Management
- **Epic 4**: Event System
- **Go Libraries**:
  - `github.com/go-git/go-git/v5` - Git operations
  - `k8s.io/client-go` - Kubernetes client
  - `github.com/google/go-github/v57` - GitHub API
  - `github.com/xanzy/go-gitlab` - GitLab API
  - `github.com/argoproj/argo-cd/v2/pkg/client` - ArgoCD client
  - `github.com/fluxcd/flux2/pkg/client` - Flux client

## Risks & Mitigations

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| GitOps tool API changes | High | Medium | Version compatibility matrix, integration tests |
| Git conflicts on auto-commit | Medium | Medium | Conflict detection, manual resolution workflow |
| False positive rollbacks | Critical | Medium | Conservative thresholds, manual approval option |
| Webhook replay attacks | High | Low | Signature verification, deduplication |
| Verification timeout during deployments | Medium | High | Configurable timeouts, partial success handling |

## Metrics & Monitoring

### Key Metrics
- Webhook processing latency
- Verification success rate
- Verification execution time
- Rollback trigger rate
- Promotion success rate
- Git sync frequency

### Alerts
- Verification failure rate >10%
- Rollback triggered
- Git sync failures
- Webhook authentication failures
- Verification timeout

## Testing Strategy

### Unit Tests
- Webhook parsing for each GitOps tool
- Verification step execution
- Rollback strategy execution
- Git operations

### Integration Tests
- End-to-end ArgoCD integration
- End-to-end Flux integration
- GitHub PR workflow
- Verification workflow execution
- Rollback scenarios

### Scenario Tests
- Successful deployment flow
- Failed deployment with rollback
- Multi-environment promotion
- Manual approval workflows

## Documentation Requirements

- [ ] ArgoCD integration guide
- [ ] Flux integration guide
- [ ] Verification workflow examples
- [ ] Rollback strategy guide
- [ ] Promotion pipeline setup
- [ ] GitHub/GitLab integration
- [ ] Webhook configuration
- [ ] Troubleshooting guide

## Definition of Done

- [ ] All user stories implemented
- [ ] ArgoCD and Flux integrations tested
- [ ] Verification framework functional
- [ ] Rollback automation working
- [ ] Git sync operational
- [ ] Documentation complete
- [ ] Example workflows provided
- [ ] Production-ready
