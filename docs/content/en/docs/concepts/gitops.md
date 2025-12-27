---
title: "GitOps Integration"
weight: 9
description: >
  Integration with ArgoCD, Flux, and Git-based deployment workflows for verification, rollback, and promotion
---

## Overview

TitanAnvil integrates with GitOps tools to bridge the gap between declarative deployment and runtime operations. While GitOps handles deployment, TitanAnvil verifies the runtime state, enables automated rollback, and manages progressive delivery across environments.

**Key Concept**: "GitOps deploys it. We keep it running."

**Core Capabilities**:
- **Webhook Integration**: Receive events from ArgoCD, Flux, GitHub, GitLab
- **Deployment Verification**: Validate deployments with health checks, commands, K8s resources
- **Automated Rollback**: Rollback failed deployments automatically
- **Promotion Pipelines**: Progressive delivery across environments (dev → staging → prod)
- **Git Sync**: Sync state files, reactors, policies from Git repositories

## Architecture

```
┌──────────────────────────────────────────────┐
│         GitOps Tools                          │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐   │
│  │  ArgoCD  │  │   Flux   │  │ GitHub/  │   │
│  │          │  │          │  │ GitLab   │   │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘   │
└───────┼─────────────┼─────────────┼──────────┘
        │             │             │
        │ Webhooks    │ Webhooks    │ Webhooks
        │             │             │
        ↓             ↓             ↓
┌─────────────────────────────────────────────┐
│      TitanAnvil Webhook Receiver            │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  │
│  │  ArgoCD  │  │   Flux   │  │ GitHub/  │  │
│  │ Handler  │  │ Handler  │  │  GitLab  │  │
│  │          │  │          │  │ Handler  │  │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  │
└───────┼─────────────┼─────────────┼─────────┘
        │             │             │
        └──────────┬──┴─────────────┘
                   ↓
         ┌───────────────────┐
         │  Event Publisher  │
         └─────────┬─────────┘
                   │
        ┌──────────┴──────────┐
        │                     │
        ↓                     ↓
  ┌──────────┐          ┌──────────┐
  │Deployment│          │ Rollback │
  │Verifier  │          │  Engine  │
  └────┬─────┘          └────┬─────┘
       │                     │
       ↓                     ↓
  ┌──────────┐          ┌──────────┐
  │ Reactor  │          │  GitOps  │
  │  Engine  │          │   API    │
  └──────────┘          └──────────┘
```

## Webhook Integration

### Supported Webhook Sources

TitanAnvil receives webhooks from:

1. **ArgoCD**: Application sync, health, deployment events
2. **Flux**: Kustomization, HelmRelease reconciliation events
3. **GitHub**: Deployment, workflow, push events
4. **GitLab**: Deployment, pipeline, push events

### Webhook Receiver

```yaml
# Control plane configuration
gitops:
  webhooks:
    enabled: true
    listen: "0.0.0.0:8090"
    path: "/webhooks"

    # Authentication
    auth:
      type: hmac           # none, hmac, bearer
      secret: "webhook-secret"

    # Event processing
    async: true
    queue_size: 1000
```

### ArgoCD Webhook

**Configure ArgoCD**:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: argocd-notifications-cm
data:
  service.webhook.titananvil: |
    url: http://titananvil-server:8090/webhooks/argocd
    headers:
    - name: X-ArgoCD-Secret
      value: $secret
```

**Events received**:
- `gitops.argocd.sync` - Application sync started/completed
- `gitops.argocd.health` - Application health changed
- `gitops.argocd.deployment` - Deployment event

**Example event**:
```json
{
  "type": "gitops.argocd.deployment",
  "source": "argocd",
  "data": {
    "application": "myapp",
    "namespace": "production",
    "status": "Healthy",
    "sync_status": "Synced",
    "revision": "abc123",
    "environment": "production"
  }
}
```

### Flux Webhook

**Configure Flux**:
```yaml
apiVersion: notification.toolkit.fluxcd.io/v1beta1
kind: Provider
metadata:
  name: titananvil
spec:
  type: generic
  address: http://titananvil-server:8090/webhooks/flux
  secretRef:
    name: webhook-secret
```

**Events received**:
- `gitops.flux.kustomization` - Kustomization reconciliation
- `gitops.flux.helmrelease` - HelmRelease reconciliation
- `gitops.flux.sync` - Sync event

**Example event**:
```json
{
  "type": "gitops.flux.helmrelease",
  "source": "flux",
  "data": {
    "name": "myapp",
    "namespace": "production",
    "status": "Ready",
    "revision": "1.2.3",
    "message": "Release reconciliation succeeded"
  }
}
```

### GitHub Webhook

**Configure GitHub** (repository settings):
- Payload URL: `http://titananvil-server:8090/webhooks/github`
- Content type: `application/json`
- Secret: `your-webhook-secret`
- Events: Deployments, Workflow runs, Pushes

**Events received**:
- `gitops.github.deployment` - Deployment created/updated
- `gitops.github.workflow` - Workflow run completed
- `gitops.github.push` - Code pushed

### GitLab Webhook

**Configure GitLab** (repository settings):
- URL: `http://titananvil-server:8090/webhooks/gitlab`
- Secret token: `your-webhook-secret`
- Trigger: Deployment events, Pipeline events, Push events

**Events received**:
- `gitops.gitlab.deployment` - Deployment event
- `gitops.gitlab.pipeline` - Pipeline completed
- `gitops.gitlab.push` - Code pushed

## Deployment Verification

Verify deployments with health checks, command execution, and K8s resource checks:

### Verification Workflow

```yaml
# Example: verify-deployment.yaml
myapp_verification:
  name: "Verify myapp deployment"

  # Triggered by ArgoCD deployment event
  trigger:
    event_filter: "type == 'gitops.argocd.deployment' and data.application == 'myapp'"

  # Verification steps
  steps:
    - name: "Wait for pods"
      type: k8s_resource
      resource: deployment
      namespace: "{{ event.data.namespace }}"
      name: myapp
      condition: available
      timeout: "5m"

    - name: "HTTP health check"
      type: http
      url: "http://myapp.{{ event.data.namespace }}.svc.cluster.local/health"
      method: GET
      expected_status: 200
      timeout: "30s"
      retry:
        attempts: 3
        delay: "10s"

    - name: "Run smoke tests"
      type: command
      command: "./scripts/smoke-test.sh {{ event.data.namespace }}"
      expected_exit_code: 0
      timeout: "2m"

    - name: "Check metrics"
      type: script
      script: |
        #!/bin/bash
        ERROR_RATE=$(curl -s prometheus:9090/api/v1/query?query='rate(http_requests_total{status="500"}[5m])' | jq -r '.data.result[0].value[1]')
        if (( $(echo "$ERROR_RATE > 0.01" | bc -l) )); then
          echo "Error rate too high: $ERROR_RATE"
          exit 1
        fi
      timeout: "30s"

  # Execution settings
  sequential: true        # Run steps in order
  continue_on_failure: false
  timeout: "10m"

  # On failure
  on_failure:
    - type: rollback
      application: "{{ event.data.application }}"
      namespace: "{{ event.data.namespace }}"
```

### Verification Step Types

**HTTP Health Check**:
```yaml
- name: "API health check"
  type: http
  url: "https://api.example.com/health"
  method: GET
  headers:
    Authorization: "Bearer {{ secrets.api_token }}"
  expected_status: 200
  expected_body: '{"status":"healthy"}'
  timeout: "30s"
```

**K8s Resource Check**:
```yaml
- name: "Check deployment ready"
  type: k8s_resource
  resource: deployment     # deployment, statefulset, daemonset, service
  namespace: production
  name: myapp
  condition: available     # available, ready, complete
  replicas: 3             # Expected replica count
  timeout: "5m"
```

**Command Execution**:
```yaml
- name: "Database migration check"
  type: command
  command: "kubectl exec -n production myapp-0 -- /app/check-migrations"
  expected_exit_code: 0
  expected_output: "All migrations applied"
  timeout: "1m"
```

**Script Execution**:
```yaml
- name: "Custom validation"
  type: script
  script: |
    #!/bin/bash
    # Custom validation logic
    ./validate-deployment.sh "$NAMESPACE" "$APP"
  env:
    NAMESPACE: "{{ event.data.namespace }}"
    APP: "{{ event.data.application }}"
  timeout: "2m"
```

### Sequential vs Parallel Execution

**Sequential** (default):
```yaml
sequential: true
steps:
  - name: "Step 1"
    # Runs first
  - name: "Step 2"
    # Runs after Step 1 completes
```

**Parallel**:
```yaml
sequential: false
steps:
  - name: "Health check API"
    # Runs in parallel
  - name: "Health check DB"
    # Runs in parallel
  - name: "Health check cache"
    # Runs in parallel
```

### Retry Logic

```yaml
- name: "Flaky health check"
  type: http
  url: "http://myapp/health"
  retry:
    attempts: 5
    delay: "10s"        # Delay between attempts
    backoff: linear     # linear, exponential, constant
    max_delay: "1m"     # Max delay for exponential backoff
```

## Automated Rollback

Automatically rollback failed deployments:

### Rollback Configuration

```yaml
# Rollback policy
rollback_policy:
  enabled: true

  # When to rollback
  triggers:
    - verification_failed
    - health_degraded
    - error_rate_high

  # Approval required
  require_approval: true
  approval_timeout: "10m"

  # Rollback strategy
  strategy: previous    # previous, last_known_good, specific

  # Post-rollback verification
  verify_after_rollback: true
```

### Rollback Types

**ArgoCD Rollback**:
```yaml
rollback:
  type: argocd
  application: myapp
  namespace: production
  strategy: previous      # Roll back to previous revision
```

**Flux Rollback**:
```yaml
rollback:
  type: flux
  resource: helmrelease
  name: myapp
  namespace: production
  strategy: last_known_good
```

**Git Rollback**:
```yaml
rollback:
  type: git
  repository: "https://github.com/myorg/myapp-config"
  branch: main
  strategy: specific
  revision: "abc123"      # Specific commit to rollback to
```

**Manual Rollback**:
```yaml
rollback:
  type: manual
  instructions: |
    1. Run: kubectl rollout undo deployment/myapp -n production
    2. Verify: kubectl get pods -n production
    3. Check health: curl http://myapp/health
```

### Rollback Strategies

**Previous**: Rollback to immediately previous deployment
```yaml
strategy: previous
```

**Last Known Good**: Rollback to last healthy deployment
```yaml
strategy: last_known_good
```

**Specific Revision**: Rollback to specific commit/revision
```yaml
strategy: specific
revision: "abc123"
```

### Approval Workflow

```yaml
rollback_policy:
  require_approval: true
  approval_timeout: "10m"

  # Auto-approve for non-production
  auto_approve_environments:
    - dev
    - staging

  # Require approval for production
  require_approval_environments:
    - production

  # Approvers
  approvers:
    - slack://ops-channel
    - email://ops-team@example.com
```

**Approve rollback**:
```bash
# List pending rollbacks
titanctl rollback list --status pending

# Approve
titanctl rollback approve abc123 --message "Approved by ops team"

# Reject
titanctl rollback reject abc123 --message "False alarm, deployment is healthy"
```

## Promotion Pipelines

Progressive delivery across environments:

### Pipeline Definition

```yaml
# Example: myapp-pipeline.yaml
pipeline:
  name: "myapp-deployment-pipeline"

  # Environments
  environments:
    - name: dev
      auto_promote: true
      verification:
        - type: http
          url: "http://myapp.dev.example.com/health"

    - name: staging
      auto_promote: false    # Requires approval
      require_approval: true
      verification:
        - type: http
          url: "http://myapp.staging.example.com/health"
        - type: command
          command: "./run-integration-tests.sh staging"

    - name: production
      auto_promote: false
      require_approval: true
      verification:
        - type: http
          url: "http://myapp.example.com/health"
        - type: k8s_resource
          resource: deployment
          name: myapp
          replicas: 5

      # Canary deployment
      strategy: canary
      canary:
        steps:
          - weight: 25
            duration: "10m"
          - weight: 50
            duration: "10m"
          - weight: 75
            duration: "10m"
          - weight: 100

      # Rollback on failure
      rollback_on_failure: true
```

### Promotion Strategies

**Immediate** (all-at-once):
```yaml
strategy: immediate
```

**Canary** (gradual traffic shift):
```yaml
strategy: canary
canary:
  steps:
    - weight: 25        # 25% traffic
      duration: "10m"   # For 10 minutes
    - weight: 50
      duration: "10m"
    - weight: 75
      duration: "10m"
    - weight: 100       # Full traffic
```

**Blue/Green**:
```yaml
strategy: blue_green
blue_green:
  # Deploy to green
  # Verify green
  # Switch traffic to green
  auto_promote: false
  verification_required: true
```

**Rolling**:
```yaml
strategy: rolling
rolling:
  max_surge: 1
  max_unavailable: 0
```

### Promotion Triggers

**Manual**:
```bash
titanctl promote myapp --from staging --to production
```

**Automatic** (on verification success):
```yaml
environments:
  - name: dev
    auto_promote: true
```

**Scheduled**:
```yaml
environments:
  - name: production
    auto_promote: true
    schedule: "0 2 * * 1"  # Monday 2 AM
```

**Event-Driven** (via reactor):
```yaml
auto_promote_on_tag:
  filter: "type == 'gitops.github.push' and data.ref =~ 'refs/tags/v.*'"
  actions:
    - type: promote
      pipeline: myapp
      from: staging
      to: production
```

## Git Sync

Sync configuration from Git repositories:

### Repository Configuration

```yaml
git_sync:
  enabled: true

  repositories:
    - name: "infrastructure-config"
      url: "https://github.com/myorg/infrastructure-config"
      branch: main

      # Authentication
      auth:
        type: ssh
        ssh_key_path: /etc/titananvil/id_rsa

      # What to sync
      paths:
        states: "states/"
        reactors: "reactors/"
        policies: "policies/"
        workflows: "workflows/"

      # Sync interval
      sync_interval: "5m"

  # Webhook for instant sync
  webhook:
    enabled: true
    secret: "git-webhook-secret"
```

### Synced Resources

**State Files**:
```
states/
├── base/
│   ├── users.yaml
│   └── packages.yaml
├── web/
│   └── nginx.yaml
└── db/
    └── postgres.yaml
```

**Reactors**:
```
reactors/
├── self-healing/
│   └── restart-failed-services.yaml
└── compliance/
    └── enforce-policies.yaml
```

**Policies**:
```
policies/
├── security/
│   └── ssh-hardening.rego
└── compliance/
    └── required-labels.rego
```

**Workflows**:
```
workflows/
├── deployment-verification/
│   └── verify-web-app.yaml
└── rollback/
    └── rollback-on-failure.yaml
```

### Sync Workflow

1. **Poll**: Check Git repository for changes every `sync_interval`
2. **Detect Changes**: Compare local and remote commits
3. **Pull**: Pull changes if commit hash differs
4. **Validate**: Validate YAML syntax and schema
5. **Apply**: Apply changes to TitanAnvil
6. **Event**: Emit `git.sync` event with changed files

### Instant Sync via Webhook

Configure Git webhook to trigger instant sync:

**GitHub**:
- Payload URL: `http://titananvil-server:8090/webhooks/git-sync`
- Events: Push events

**GitLab**:
- URL: `http://titananvil-server:8090/webhooks/git-sync`
- Events: Push events

## Reactor Integration

Use reactors to automate GitOps workflows:

### Auto-Verify Deployments

```yaml
auto_verify_deployments:
  filter: "type =~ 'gitops\\.(argocd|flux)\\.deployment'"
  actions:
    - type: verification
      workflow: "{{ event.data.application }}-verification"
      timeout: "10m"
```

### Auto-Rollback on Failure

```yaml
auto_rollback_failed:
  filter: "type == 'verification.failed'"
  actions:
    - type: rollback
      application: "{{ event.data.application }}"
      namespace: "{{ event.data.namespace }}"
      strategy: previous
```

### Promote on Success

```yaml
promote_on_verification:
  filter: "type == 'verification.success' and data.environment == 'staging'"
  actions:
    - type: promote
      pipeline: "{{ event.data.application }}"
      from: staging
      to: production
  conditions:
    only_if: "event.data.auto_promote == true"
```

### Notify on Deployment

```yaml
notify_deployments:
  filter: "type =~ 'gitops\\.(argocd|flux)\\.deployment'"
  actions:
    - type: webhook
      url: "https://slack.example.com/hooks/deployments"
      body: |
        {
          "text": "Deployment: {{ event.data.application }} to {{ event.data.environment }}",
          "status": "{{ event.data.status }}"
        }
```

## Best Practices

### Deployment Verification

1. **Fast Feedback**: Keep verification steps under 5 minutes
2. **Comprehensive Checks**: Verify health, functionality, performance
3. **Smoke Tests**: Run quick smoke tests, not full test suite
4. **Retry Flaky Checks**: Use retry logic for network-dependent checks

### Rollback

1. **Always Verify**: Verify after rollback to ensure success
2. **Approval for Production**: Require approval for production rollbacks
3. **Document Reasons**: Log why rollback was triggered
4. **Test Rollback**: Test rollback procedures in dev/staging

### Promotion

1. **Gradual Rollout**: Use canary or blue/green for production
2. **Verification Required**: Always verify before promoting
3. **Approval Gates**: Require approval for production promotions
4. **Rollback Ready**: Have rollback plan for each promotion

### Git Sync

1. **Validate Before Sync**: Validate YAML in CI before merging
2. **Short Sync Interval**: Use 1-5 minute sync intervals
3. **Use Webhooks**: Enable webhooks for instant sync
4. **Version Control**: Keep all config in Git

## Monitoring

### Metrics

```
# Webhooks
titananvil_gitops_webhooks_received_total{source}
titananvil_gitops_webhooks_failed_total{source}

# Verifications
titananvil_gitops_verifications_total{status}
titananvil_gitops_verification_duration_seconds{quantile}

# Rollbacks
titananvil_gitops_rollbacks_total{type,status}
titananvil_gitops_rollback_duration_seconds{quantile}

# Promotions
titananvil_gitops_promotions_total{pipeline,status}

# Git Sync
titananvil_gitops_sync_total{repository,status}
titananvil_gitops_sync_duration_seconds{quantile}
```

### Events

- `gitops.webhook.received` - Webhook received
- `gitops.verification.started` - Verification started
- `gitops.verification.success` - Verification succeeded
- `gitops.verification.failed` - Verification failed
- `gitops.rollback.started` - Rollback started
- `gitops.rollback.completed` - Rollback completed
- `gitops.promotion.started` - Promotion started
- `gitops.promotion.completed` - Promotion completed
- `git.sync` - Git repository synced

## Troubleshooting

### Webhook Not Received

**Problem**: Webhook not triggering events

Check:
```bash
# Check webhook endpoint is accessible
curl -X POST http://titananvil-server:8090/webhooks/argocd \
  -H "Content-Type: application/json" \
  -d '{"test":"payload"}'

# Check webhook metrics
curl http://titananvil-server:8080/metrics | grep gitops_webhooks

# Check logs
titanctl logs --filter "component == 'webhook-receiver'"
```

### Verification Failing

**Problem**: Verification steps failing unexpectedly

Debug:
```bash
# Run verification manually
titanctl verify run myapp-verification --namespace production

# Check verification logs
titanctl verify logs myapp-verification --limit 10

# Test individual steps
curl http://myapp.production.svc.cluster.local/health
```

### Rollback Not Triggering

**Problem**: Rollback not executing on verification failure

Check:
```bash
# Verify rollback policy is enabled
titanctl rollback policy show myapp

# Check rollback triggers
titanctl rollback triggers myapp

# Manual rollback
titanctl rollback execute myapp --namespace production --strategy previous
```

### Git Sync Not Working

**Problem**: Git repository not syncing

Check:
```bash
# Check Git sync status
titanctl git-sync status infrastructure-config

# Manual sync
titanctl git-sync trigger infrastructure-config

# Check authentication
ssh -T git@github.com

# Check logs
titanctl logs --filter "component == 'git-sync'"
```

## Next Steps

- Learn about [Reactors](../reactors/) for automating GitOps workflows
- Understand [Events](../events/) emitted during deployments
- Explore [State Management](../state-management/) synced from Git
- See [Policy Enforcement](../policy/) for compliance checks
- Review [Observability](../observability/) for monitoring GitOps operations
