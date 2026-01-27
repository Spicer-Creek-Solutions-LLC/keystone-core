---
title: "Runbook Automation"
weight: 21
description: >
  Automated runbook system for orchestrating operational workflows
---

Keystone Core includes a powerful runbook automation system that enables codified operational workflows with conditional branching, human approvals, and full audit trails.

## Overview

Runbooks are YAML-defined workflows that automate complex operational tasks. They support:

- **Conditional branching**: if/else, switch, and loop constructs
- **Human approvals**: Per-step approval gates with configurable approvers
- **Manual intervention**: Pause points for operator input
- **Multiple step types**: command, API, state, deploy, wait, prompt, notification
- **Variable passing**: Step outputs feed into subsequent steps
- **Reusable templates**: Shared step libraries and sub-runbooks
- **Event triggers**: Execute on alerts, drift, or schedules
- **ITSM integration**: PagerDuty, Opsgenie, ServiceNow

## Quick Start

### Basic Runbook

```yaml
apiVersion: runbook.keystone.io/v1
kind: Runbook
metadata:
  name: restart-service
  namespace: operations
spec:
  description: Safely restart a service with health checks
  inputs:
    - name: service_name
      type: string
      required: true
      description: Name of the service to restart
    - name: target
      type: string
      required: true
      description: Target host or group

  steps:
    - name: health_check_before
      type: command
      config:
        target: "{{ .inputs.target }}"
        command: "systemctl status {{ .inputs.service_name }}"

    - name: restart_service
      type: command
      dependsOn: [health_check_before]
      config:
        target: "{{ .inputs.target }}"
        command: "systemctl restart {{ .inputs.service_name }}"

    - name: wait_startup
      type: wait
      dependsOn: [restart_service]
      config:
        duration: 10s

    - name: health_check_after
      type: command
      dependsOn: [wait_startup]
      config:
        target: "{{ .inputs.target }}"
        command: "systemctl status {{ .inputs.service_name }}"

    - name: notify_complete
      type: notification
      dependsOn: [health_check_after]
      config:
        channel: slack
        message: "Service {{ .inputs.service_name }} restarted on {{ .inputs.target }}"

  onFailure:
    - name: notify_failure
      type: notification
      config:
        channel: pagerduty
        message: "Failed to restart {{ .inputs.service_name }}"
```

### Executing a Runbook

```bash
# Execute a runbook
kscorectl runbook execute restart-service \
  --input service_name=nginx \
  --input target=web-servers

# View execution status
kscorectl runbook status exec-12345

# List recent executions
kscorectl runbook list-executions --runbook restart-service
```

## Runbook Structure

### Metadata

```yaml
apiVersion: runbook.keystone.io/v1
kind: Runbook
metadata:
  name: my-runbook           # Unique identifier (required)
  namespace: operations      # Organizational namespace
  labels:                    # Optional labels for filtering
    team: platform
    environment: production
  annotations:               # Additional metadata
    owner: ops-team@example.com
```

### Spec

```yaml
spec:
  description: "Human-readable description"

  # Input parameters
  inputs:
    - name: parameter_name
      type: string           # string, int, bool, list, map
      required: true
      default: "default-value"
      description: "Parameter description"
      validation: "^[a-z]+$" # Optional regex validation

  # Main execution steps
  steps: []

  # Steps to run on successful completion
  onSuccess: []

  # Steps to run on failure
  onFailure: []

  # Global timeout for entire runbook
  timeout: 30m

  # Maximum retry attempts for failed steps
  maxRetries: 3
```

## Step Types

### Command Step

Execute shell commands on target hosts.

```yaml
- name: run_command
  type: command
  config:
    target: "{{ .inputs.target }}"    # Host or group
    command: "apt-get update"
    timeout: 5m
    sudo: true
    shell: /bin/bash
  outputs:
    - name: output
      source: stdout
```

### API Step

Make HTTP API calls.

```yaml
- name: api_call
  type: api
  config:
    method: POST
    url: "https://api.example.com/deployments"
    headers:
      Authorization: "Bearer {{ .secrets.api_token }}"
      Content-Type: application/json
    body: |
      {
        "version": "{{ .inputs.version }}",
        "environment": "{{ .inputs.env }}"
      }
    timeout: 30s
  outputs:
    - name: deployment_id
      source: json
      path: $.id
```

### Notification Step

Send notifications to various channels.

```yaml
- name: notify
  type: notification
  config:
    channel: slack          # slack, email, pagerduty, opsgenie, webhook
    message: "Deployment complete: {{ .inputs.version }}"
    # Channel-specific config
    slack_channel: "#ops-alerts"
    severity: info          # info, warning, critical
```

### Wait Step

Pause execution for a duration or condition.

```yaml
# Fixed duration
- name: wait_fixed
  type: wait
  config:
    duration: 30s

# Poll until condition is met
- name: wait_condition
  type: wait
  config:
    poll_interval: 10s
    timeout: 5m
    condition: "{{ .steps.health_check.outputs.status }} == 'healthy'"
```

### Approval Step

Require human approval to proceed.

```yaml
- name: approve_deploy
  type: approval
  config:
    message: "Approve deployment to production?"
    approvers:
      - group: sre-team
      - user: admin@example.com
    require_count: 2        # Require 2 approvals
    timeout: 1h
    escalation:
      after: 30m
      to: manager@example.com
```

### Intervention Step

Pause for manual operator action.

```yaml
- name: manual_check
  type: intervention
  config:
    prompt: "Please verify the database backup completed successfully"
    timeout: 30m
    actions:
      - label: "Backup verified"
        value: verified
      - label: "Backup failed"
        value: failed
```

### State Step

Apply Keystone state to targets.

```yaml
- name: apply_config
  type: state
  config:
    target: "{{ .inputs.target }}"
    state: pkg/nginx/installed
    parameters:
      version: "{{ .inputs.nginx_version }}"
```

### Deploy Step

Trigger GitOps deployments.

```yaml
- name: deploy_app
  type: deploy
  config:
    repository: "github.com/org/app"
    branch: main
    path: "kubernetes/{{ .inputs.env }}"
    wait: true
    timeout: 10m
```

### Sub-Runbook Step

Execute another runbook.

```yaml
- name: run_health_checks
  type: subrunbook
  config:
    runbook: health-check-suite
    inputs:
      target: "{{ .inputs.target }}"
      checks:
        - memory
        - cpu
        - disk
```

### Noop Step

No operation, useful for testing or placeholders.

```yaml
- name: placeholder
  type: noop
  config:
    message: "This step does nothing"
```

### Fail Step

Intentionally fail execution.

```yaml
- name: force_failure
  type: fail
  config:
    message: "Validation failed: {{ .steps.validate.outputs.error }}"
```

## Control Flow

### Conditions

```yaml
- name: conditional_step
  type: command
  condition: "{{ eq .inputs.environment \"production\" }}"
  config:
    command: "deploy-prod.sh"

# Multiple conditions
- name: complex_condition
  type: command
  condition: |
    {{ and
      (eq .inputs.environment "production")
      (gt .steps.tests.outputs.coverage 80)
      (.steps.security_scan.outputs.passed)
    }}
  config:
    command: "release.sh"
```

### Dependencies

```yaml
steps:
  - name: step_a
    type: command
    config:
      command: "echo A"

  - name: step_b
    type: command
    dependsOn: [step_a]     # Runs after step_a
    config:
      command: "echo B"

  - name: step_c
    type: command
    dependsOn: [step_a]     # Runs after step_a (parallel with step_b)
    config:
      command: "echo C"

  - name: step_d
    type: command
    dependsOn: [step_b, step_c]  # Waits for both
    config:
      command: "echo D"
```

### Loops

```yaml
- name: deploy_to_regions
  type: parallel
  config:
    items: "{{ .inputs.regions }}"
    max_parallel: 3
  steps:
    - name: deploy_region
      type: deploy
      config:
        region: "{{ .item }}"
```

### Branching

```yaml
- name: check_environment
  type: branch
  config:
    expression: "{{ .inputs.environment }}"
    cases:
      production:
        - name: prod_deploy
          type: subrunbook
          config:
            runbook: production-deploy
      staging:
        - name: staging_deploy
          type: subrunbook
          config:
            runbook: staging-deploy
    default:
      - name: dev_deploy
        type: command
        config:
          command: "deploy-dev.sh"
```

## Variables and Outputs

### Input Variables

```yaml
# Access input values
command: "deploy {{ .inputs.version }} to {{ .inputs.environment }}"
```

### Step Outputs

```yaml
# Access outputs from previous steps
- name: use_output
  type: command
  config:
    command: "deploy {{ .steps.build.outputs.artifact_id }}"
```

### Output Parsing

```yaml
- name: get_version
  type: command
  config:
    command: "cat version.txt"
  outputs:
    - name: version
      source: stdout
      parser: line
      path: 0              # First line

- name: api_response
  type: api
  config:
    url: "https://api.example.com/status"
  outputs:
    - name: status
      source: json
      path: $.status       # JSONPath
    - name: count
      source: json
      path: $.data.count

- name: parse_logs
  type: command
  config:
    command: "grep ERROR /var/log/app.log"
  outputs:
    - name: error_count
      source: stdout
      parser: regex
      path: "ERROR: (\\d+)"  # Capture group
```

### Built-in Variables

```yaml
# Available in all steps
execution_id: "{{ .execution.id }}"
runbook_name: "{{ .runbook.name }}"
current_time: "{{ .now }}"
```

## Retries and Error Handling

### Step Retries

```yaml
- name: flaky_api
  type: api
  retries:
    maxAttempts: 3
    delay: 5s
    backoff: exponential  # linear, exponential, fixed
    maxDelay: 60s
    retryOn:
      - 500
      - 502
      - 503
  config:
    url: "https://api.example.com/data"
```

### Continue on Error

```yaml
- name: optional_cleanup
  type: command
  continueOnError: true
  config:
    command: "cleanup-temp-files.sh"
```

### Rollback

```yaml
- name: deploy
  type: deploy
  rollback:
    steps:
      - name: rollback_deploy
        type: command
        config:
          command: "kubectl rollout undo deployment/app"
  config:
    manifest: "deployment.yaml"
```

## ITSM Integration

### PagerDuty

```yaml
spec:
  triggers:
    - type: pagerduty
      config:
        service_id: P123ABC
        events:
          - incident.triggered
          - incident.acknowledged

  steps:
    - name: resolve_incident
      type: itsm
      config:
        provider: pagerduty
        action: resolve
        incident_id: "{{ .trigger.incident.id }}"
```

### ServiceNow

```yaml
spec:
  triggers:
    - type: servicenow
      config:
        table: change_request
        filter: "state=approved^assignment_group=ops"

  steps:
    - name: execute_change
      type: command
      config:
        command: "apply-change.sh"

    - name: update_ticket
      type: itsm
      config:
        provider: servicenow
        action: update
        record_id: "{{ .trigger.change_request.sys_id }}"
        fields:
          state: implemented
          work_notes: "Change executed by Keystone runbook"
```

## Triggers

### Event Triggers

```yaml
spec:
  triggers:
    - type: event
      config:
        filter:
          type: drift.detected
          severity: high
        debounce: 5m
```

### Schedule Triggers

```yaml
spec:
  triggers:
    - type: schedule
      config:
        cron: "0 2 * * *"     # Daily at 2 AM
        timezone: UTC
```

### Webhook Triggers

```yaml
spec:
  triggers:
    - type: webhook
      config:
        path: /hooks/my-runbook
        secret: "{{ .secrets.webhook_secret }}"
        method: POST
```

## Audit and Compliance

All runbook executions are fully audited:

```bash
# View execution history
kscorectl runbook audit list \
  --runbook restart-service \
  --start 2024-01-01 \
  --end 2024-01-31

# Export compliance report
kscorectl runbook audit report \
  --format pdf \
  --start 2024-01-01 \
  --end 2024-01-31 \
  > compliance-report.pdf
```

### Audit Events

- Execution started/completed/failed
- Step started/completed/failed/skipped
- Approval requested/granted/denied
- Intervention requested/completed
- Variables accessed
- Secrets accessed (masked)

## Best Practices

### Naming

- Use descriptive names: `restart-nginx-service` not `restart`
- Use consistent prefixes: `deploy-*`, `backup-*`, `incident-*`
- Step names should describe the action: `validate_config`, `deploy_app`

### Inputs

- Always provide descriptions for inputs
- Use sensible defaults where appropriate
- Validate inputs with regex patterns

### Error Handling

- Always include `onFailure` handlers for critical runbooks
- Use `continueOnError` only for non-critical steps
- Configure appropriate retries for network operations

### Approvals

- Require approvals for production changes
- Set reasonable timeouts with escalation
- Document approval criteria in the runbook

### Testing

```bash
# Dry-run mode validates without executing
kscorectl runbook execute my-runbook \
  --input env=staging \
  --dry-run

# Test with mock handlers
kscorectl runbook test my-runbook \
  --mock-file test-mocks.yaml
```

## Examples

See the [Runbook Examples](../operations/runbook-examples/) directory for complete examples:

- Database maintenance runbooks
- Kubernetes deployment pipelines
- Incident response automation
- Certificate rotation workflows
- Backup and recovery procedures
