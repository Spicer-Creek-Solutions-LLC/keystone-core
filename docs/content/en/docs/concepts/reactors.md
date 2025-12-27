---
title: "Reactors"
weight: 8
description: >
  Automated event-driven responses with filtering, actions, and orchestration
---

## Overview

Keystone Core's reactor system enables automated responses to infrastructure events. Reactors subscribe to events, filter based on criteria, and execute actions—enabling self-healing, compliance remediation, and operational automation.

**Key Features**:
- **Event-Driven**: Triggered by any of the 15 Keystone Core event types
- **Filter Expressions**: CEL-based filtering for precise event matching
- **10+ Action Types**: Command execution, webhooks, state application, custom functions
- **Orchestration**: Sequential, parallel, conditional, retry logic
- **Throttling/Debouncing**: Prevent excessive executions
- **Priority Ordering**: Control reactor execution order
- **Metrics & Auditing**: Track all reactor executions

## Reactor Architecture

```
┌─────────────────────────────────────────────┐
│            Event Sources                     │
│  - Agents (connect, disconnect, heartbeat)  │
│  - Jobs (start, complete, fail)             │
│  - State (apply, change, drift)             │
│  - GitOps (deployment, verification)        │
│  - Policy (violations, compliance)          │
└────────────────┬────────────────────────────┘
                 │
                 ↓
         ┌───────────────────┐
         │  Event Publisher  │
         └─────────┬─────────┘
                   │
                   ↓
         ┌───────────────────┐
         │ NATS JetStream    │
         │  (event stream)   │
         └─────────┬─────────┘
                   │
        ┌──────────┴──────────┐
        │                     │
        ↓                     ↓
  ┌──────────┐          ┌──────────┐
  │  Reactor │          │  Reactor │
  │  Engine  │          │  Engine  │
  └────┬─────┘          └────┬─────┘
       │                     │
       ↓                     ↓
  ┌──────────┐          ┌──────────┐
  │ Filters  │          │ Filters  │
  │ (CEL)    │          │ (CEL)    │
  └────┬─────┘          └────┬─────┘
       │                     │
       ↓                     ↓
  ┌──────────┐          ┌──────────┐
  │ Actions  │          │ Actions  │
  │ Execute  │          │ Execute  │
  └──────────┘          └──────────┘
```

## Reactor Definition

Reactors are defined in YAML:

```yaml
# Example: auto-remediate-drift.yaml
drift_remediation:
  name: "Auto-remediate critical drift"
  description: "Automatically reapply state when critical drift is detected"

  # Event filter (CEL expression)
  filter: "type == 'state.drift' and severity == 'critical'"

  # Priority (lower = higher priority)
  priority: 1

  # Actions to execute
  actions:
    - type: state_apply
      state_file: "{{ event.data.state_file }}"
      target: "agent_id == {{ event.source }}"

    - type: webhook
      url: "https://slack.example.com/hooks/drift"
      method: POST
      body: |
        {
          "text": "Critical drift remediated on {{ event.source }}"
        }

  # Conditions
  conditions:
    throttle: "5m"           # Max once per 5 minutes
    max_executions: 10       # Max 10 executions per time window
    time_window: "1h"        # Time window for max_executions

  # Execution settings
  max_concurrent: 3          # Max 3 concurrent executions
  timeout: "10m"             # Action timeout
  error_strategy: "continue" # continue, stop, retry
```

## Reactor Components

### 1. Filter Expression

Reactors use CEL expressions to match events:

**Simple Filters**:
```yaml
# By event type
filter: "type == 'agent.disconnect'"

# By severity
filter: "severity >= 'warning'"

# By source
filter: "source =~ 'web-.*'"

# By tags
filter: "'production' in tags"
```

**Complex Filters**:
```yaml
# Multiple conditions (AND)
filter: "type == 'job.fail' and severity == 'critical' and 'production' in tags"

# Multiple conditions (OR)
filter: "type == 'agent.disconnect' or type == 'agent.heartbeat_failed'"

# Data field access
filter: "type == 'state.drift' and data.severity == 'critical' and data.environment == 'production'"

# Nested data
filter: "data.agent.datacenter == 'us-east-1' and data.agent.role == 'web'"
```

### 2. Actions

Reactors can execute multiple actions in sequence or parallel:

**Command Action**:
```yaml
- type: command
  command: "systemctl restart nginx"
  target: "agent_id == {{ event.source }}"
  timeout: "30s"
```

**State Apply Action**:
```yaml
- type: state_apply
  state_file: "/etc/kscore/states/web-server.yaml"
  target: "role:web and datacenter:{{ event.data.datacenter }}"
  check_only: false
```

**Webhook Action**:
```yaml
- type: webhook
  url: "https://hooks.slack.com/services/XXX/YYY/ZZZ"
  method: POST
  headers:
    Content-Type: application/json
  body: |
    {
      "text": "Alert: {{ event.type }} on {{ event.source }}",
      "severity": "{{ event.severity }}"
    }
  retry:
    attempts: 3
    backoff: exponential
```

**Event Action** (emit new events):
```yaml
- type: event
  event_type: "reactor.remediation"
  severity: info
  data:
    original_event: "{{ event.id }}"
    action: "restarted service"
```

**Conditional Action**:
```yaml
- type: conditional
  condition: "event.data.exit_code != 0"
  then:
    - type: command
      command: "systemctl restart myapp"
  else:
    - type: log
      level: info
      message: "Service running normally"
```

**Sequence Action** (execute in order):
```yaml
- type: sequence
  actions:
    - type: command
      command: "docker pull myapp:latest"
    - type: command
      command: "docker stop myapp"
    - type: command
      command: "docker run -d --name myapp myapp:latest"
```

**Parallel Action** (execute concurrently):
```yaml
- type: parallel
  actions:
    - type: webhook
      url: "https://pagerduty.example.com/events"
    - type: webhook
      url: "https://slack.example.com/hooks/alerts"
    - type: command
      command: "logger 'Critical alert triggered'"
```

**Retry Action**:
```yaml
- type: retry
  max_attempts: 5
  backoff: exponential  # exponential, linear, constant
  initial_delay: "1s"
  max_delay: "30s"
  action:
    type: webhook
    url: "https://external-api.example.com/notify"
```

**Delay Action**:
```yaml
- type: delay
  duration: "30s"

- type: command
  command: "check-service-health"
```

**Log Action**:
```yaml
- type: log
  level: warn
  message: "Detected {{ event.type }} on {{ event.source }}"
  fields:
    severity: "{{ event.severity }}"
    correlation_id: "{{ event.correlation_id }}"
```

**Function Action** (custom Go function):
```yaml
- type: function
  function: "customRemediationLogic"
  # Function must be registered in code
```

## Reactor Conditions

Control when and how often reactors execute:

### Throttle

Limit execution frequency:

```yaml
conditions:
  throttle: "5m"  # Execute at most once per 5 minutes
```

Use case: Prevent excessive remediation attempts for flapping services.

### Debounce

Wait for quiet period before executing:

```yaml
conditions:
  debounce: "30s"  # Wait 30s of quiet before executing
```

Use case: Wait for multiple related events to settle before acting.

### Execution Limits

Limit total executions:

```yaml
conditions:
  max_executions: 10  # Max 10 executions
  time_window: "1h"   # Per 1 hour window
```

Use case: Circuit-breaker pattern to prevent runaway automation.

### Only If / Unless

Conditional execution based on data:

```yaml
conditions:
  only_if: "event.data.environment == 'production'"
  unless: "event.data.ignore_reactor == true"
```

### Concurrency Control

Limit concurrent executions:

```yaml
max_concurrent: 5  # Max 5 concurrent reactor executions
```

Use case: Prevent resource exhaustion from parallel actions.

## Reactor Patterns

### Self-Healing Pattern

Automatically restart failed services:

```yaml
auto_restart_failed_services:
  filter: "type == 'job.fail' and data.service != ''"
  priority: 1
  actions:
    - type: command
      command: "systemctl restart {{ event.data.service }}"
      target: "agent_id == {{ event.source }}"
  conditions:
    throttle: "5m"
    max_executions: 3
    time_window: "1h"
  error_strategy: stop
```

### Drift Remediation Pattern

Automatically fix configuration drift:

```yaml
auto_fix_drift:
  filter: "type == 'state.drift' and severity >= 'high'"
  priority: 2
  actions:
    - type: state_apply
      state_file: "{{ event.data.state_file }}"
      target: "agent_id == {{ event.source }}"
    - type: event
      event_type: "reactor.drift_fixed"
      severity: info
  conditions:
    throttle: "10m"
```

### Escalation Pattern

Escalate critical issues:

```yaml
escalate_critical_events:
  filter: "severity == 'critical'"
  priority: 1
  actions:
    - type: parallel
      actions:
        - type: webhook
          url: "https://pagerduty.example.com/events"
        - type: webhook
          url: "https://slack.example.com/hooks/critical"
        - type: command
          command: "send-sms-alert.sh '{{ event.type }}'"
```

### Compliance Enforcement Pattern

Automatically remediate policy violations:

```yaml
enforce_compliance:
  filter: "type == 'policy.violation' and data.policy_category == 'security'"
  priority: 1
  actions:
    - type: sequence
      actions:
        - type: log
          level: warn
          message: "Security policy violation: {{ event.data.policy_name }}"

        - type: conditional
          condition: "event.data.remediable == true"
          then:
            - type: state_apply
              state_file: "{{ event.data.remediation_state }}"
          else:
            - type: webhook
              url: "https://security-team.example.com/violations"
  conditions:
    throttle: "1m"
```

### Deployment Verification Pattern

Verify deployments and rollback on failure:

```yaml
verify_deployments:
  filter: "type == 'gitops.argocd.deployment'"
  priority: 5
  actions:
    - type: delay
      duration: "30s"  # Wait for deployment to settle

    - type: sequence
      actions:
        - type: command
          command: "curl -f http://{{ event.data.service_url }}/health"
          timeout: "10s"

        - type: conditional
          condition: "action.exit_code != 0"
          then:
            - type: webhook
              url: "{{ event.data.rollback_webhook }}"
              method: POST
              body: |
                {
                  "action": "rollback",
                  "application": "{{ event.data.application }}"
                }
```

### Batch Processing Pattern

Process events in batches:

```yaml
batch_process_logs:
  filter: "type == 'user.custom' and data.batch_process == true"
  priority: 10
  actions:
    - type: function
      function: "batchProcessor"
  conditions:
    debounce: "5m"  # Wait for 5m of quiet
    max_executions: 1
    time_window: "5m"
```

## Priority and Ordering

Reactors execute in priority order (lower number = higher priority):

```yaml
# Priority 1 - Critical remediation
critical_remediation:
  priority: 1
  filter: "severity == 'critical'"

# Priority 5 - Normal automation
normal_automation:
  priority: 5
  filter: "severity == 'warning'"

# Priority 10 - Logging/auditing
audit_logging:
  priority: 10
  filter: "true"  # Match all events
```

**Execution behavior**:
- All matching reactors execute (not just first match)
- Executed in priority order
- Lower priority = executed first
- Same priority = undefined order

## Error Handling

Control reactor behavior on action failure:

**Continue on Error**:
```yaml
error_strategy: continue  # Continue to next action
```

**Stop on Error**:
```yaml
error_strategy: stop  # Stop execution on first error
```

**Retry on Error**:
```yaml
error_strategy: retry
retry:
  max_attempts: 3
  backoff: exponential
  initial_delay: "1s"
```

## Reactor Management

### Enable/Disable Reactors

```go
// Enable reactor
engine.EnableReactor("drift_remediation")

// Disable reactor
engine.DisableReactor("drift_remediation")
```

### Reactor Metrics

Track reactor performance:

```
# Executions
kscore_reactor_executions_total{reactor="drift_remediation"}

# Failures
kscore_reactor_failures_total{reactor="drift_remediation"}

# Duration
kscore_reactor_duration_seconds{reactor="drift_remediation",quantile="0.95"}

# Active reactors
kscore_active_reactors
```

### Reactor Events

Reactors emit events for observability:

**reactor.execute**:
```json
{
  "type": "reactor.execute",
  "source": "reactor-engine",
  "data": {
    "reactor_name": "drift_remediation",
    "event_id": "original-event-id",
    "actions_count": 2
  }
}
```

**reactor.action**:
```json
{
  "type": "reactor.action",
  "source": "reactor-engine",
  "data": {
    "reactor_name": "drift_remediation",
    "action_type": "state_apply",
    "success": true,
    "duration_ms": 1234
  }
}
```

## Testing Reactors

### Dry-Run Mode

Test reactors without executing actions:

```yaml
dry_run: true
```

Actions log what they would do instead of executing.

### Reactor Testing in Code

```go
// Create test reactor
reactor := &Reactor{
    Name:   "test_reactor",
    Filter: "type == 'test.event'",
    Actions: []Action{
        &LogAction{Level: "info", Message: "Test"},
    },
}

// Create test event
event := &Event{
    Type:   "test.event",
    Source: "test",
}

// Execute
result := engine.ExecuteReactor(reactor, event)

// Verify
if !result.Success {
    t.Errorf("Reactor failed: %v", result.Error)
}
```

## Best Practices

### Design

1. **Specific Filters**: Write narrow filters to reduce unnecessary executions
2. **Idempotent Actions**: Ensure actions can run multiple times safely
3. **Timeout Actions**: Always set timeouts to prevent hanging reactors
4. **Error Handling**: Choose appropriate error_strategy for each reactor

### Performance

1. **Use Throttling**: Prevent excessive executions with throttle/debounce
2. **Limit Concurrency**: Set max_concurrent to avoid resource exhaustion
3. **Async Actions**: Use parallel actions for independent operations
4. **Prioritize**: Use priority to control execution order

### Security

1. **Validate Data**: Don't trust event data without validation
2. **Scope Actions**: Target specific agents, don't use wildcards
3. **Audit Actions**: Log all reactor executions for compliance
4. **Least Privilege**: Reactors should have minimum required permissions

### Monitoring

1. **Track Metrics**: Monitor reactor executions and failures
2. **Alert on Failures**: Set up alerts for reactor error rates
3. **Review Logs**: Regularly review reactor execution logs
4. **Test Changes**: Test reactor changes in dev before production

## Troubleshooting

### Reactor Not Executing

**Problem**: Reactor doesn't execute for matching events

Check:
```bash
# Verify reactor is registered
kscorectl reactor list

# Check reactor is enabled
kscorectl reactor status drift_remediation

# Test filter expression
kscorectl event query "type == 'state.drift' and severity == 'critical'"

# Check throttle/debounce limits
kscorectl reactor stats drift_remediation
```

### Action Failures

**Problem**: Reactor executes but actions fail

Check:
```bash
# View reactor execution history
kscorectl reactor history drift_remediation --limit 10

# Check action logs
kscorectl logs --filter "reactor_name == 'drift_remediation'"

# Test action manually
kscorectl exec run "systemctl restart nginx" --target "web-01"
```

### High Reactor Latency

**Problem**: Reactor takes too long to execute

Fix:
- Reduce action timeout
- Use parallel actions for independent operations
- Increase max_concurrent
- Optimize filter expressions
- Check action performance (slow commands, network delays)

### Runaway Reactors

**Problem**: Reactor executing too frequently

Fix:
```yaml
conditions:
  throttle: "5m"           # Add throttling
  max_executions: 10       # Limit total executions
  time_window: "1h"        # Per time window
  debounce: "30s"          # Wait for quiet period
```

## Performance

### Throughput

- **Event Processing**: 40,000 events/sec
- **Reactor Execution**: 1,000 reactors/sec
- **Action Execution**: Depends on action type
  - Log: <1ms
  - Event: <5ms
  - Command: 10ms-10s (depends on command)
  - Webhook: 50-500ms (depends on endpoint)
  - State Apply: 100ms-60s (depends on complexity)

### Latency

- **Filter Evaluation**: <1ms (p95)
- **Action Dispatch**: <5ms (p95)
- **End-to-end** (event → action complete):
  - Simple actions (log): <10ms (p95)
  - Medium actions (webhook): <100ms (p95)
  - Complex actions (state apply): <5s (p95)

### Resource Usage

**Per 1,000 reactors**:
- Memory: 50MB
- CPU: 0.1 cores (idle), 1-2 cores (active)

## Next Steps

- Learn about [Events](../events/) that trigger reactors
- Understand [State Management](../state-management/) for state apply actions
- Explore [Remote Execution](../remote-execution/) for command actions
- See [GitOps Integration](../gitops/) for deployment verification reactors
- Review [Policy Enforcement](../policy/) for compliance reactors
