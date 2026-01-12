---
title: "Setting Up Drift Detection"
weight: 50
description: >
  Monitor configuration drift and get alerted when systems deviate from desired state.
---

## Overview

Configuration drift occurs when a system's actual state diverges from its desired state. This can happen due to:
- Manual changes
- Failed updates
- External tools modifying configurations
- Security incidents

In this tutorial, you'll learn how to:
- Enable drift detection for your states
- Configure drift severity levels
- Set up alerts for drift events
- Create automated remediation

**Time**: 20 minutes

## Prerequisites

- Keystone Core control plane running
- At least one agent connected
- A state file to monitor

## Step 1: Create a Baseline State

First, create a state file that defines your desired configuration:

```yaml
# webserver-config.yaml
metadata:
  description: Web server configuration baseline

states:
  file:
    - id: /etc/nginx/nginx.conf
      state: present
      parameters:
        source: files/nginx.conf
        owner: root
        group: root
        mode: "0644"

  service:
    - id: nginx
      state: running
      parameters:
        enabled: true
```

Apply this state to establish the baseline:

```bash
kscorectl state apply webserver-config.yaml
```

## Step 2: Check for Drift Manually

Run the drift detection command:

```bash
kscorectl state drift webserver-config.yaml
```

Output when everything matches:
```
Checking drift: webserver-config.yaml

Target: all agents

[web-001] Checking drift...
  /etc/nginx/nginx.conf: ✓ no drift
  nginx (service): ✓ no drift

Summary: No drift detected
```

## Step 3: Simulate Drift

Let's create some drift by manually modifying the file:

```bash
kscorectl exec "web-*" -- sed -i 's/worker_processes auto/worker_processes 1/' /etc/nginx/nginx.conf
```

Now check for drift again:

```bash
kscorectl state drift webserver-config.yaml
```

Output:
```
Checking drift: webserver-config.yaml

[web-001] Checking drift...
  /etc/nginx/nginx.conf: ✗ DRIFT DETECTED
    Severity: HIGH
    Differences:
      - contents: changed
        expected: "worker_processes auto;"
        actual:   "worker_processes 1;"
  nginx (service): ✓ no drift

Summary: 1 drift(s) detected
  Critical: 0
  High:     1
  Medium:   0
  Low:      0
```

## Step 4: Configure Automatic Drift Checking

Create a drift detection configuration:

```yaml
# drift-config.yaml
drift_detection:
  enabled: true
  interval: 5m          # Check every 5 minutes
  targets:
    - path: webserver-config.yaml
      severity_threshold: medium  # Report medium and above

  # What to do when drift is detected
  on_drift:
    emit_event: true    # Emit events for reactors
    alert: true         # Send alerts
```

Apply the configuration:

```bash
kscorectl apply drift-config.yaml
```

## Step 5: Set Up Drift Alerts

Create a reactor to handle drift events:

```yaml
# drift-reactor.yaml
reactors:
  - name: drift-alert
    description: Alert on configuration drift
    enabled: true

    # Trigger on drift events
    filter:
      type: state.drift
      severity: high OR severity: critical

    actions:
      # Log the drift
      - type: log
        level: warning
        message: "Drift detected: {{ .event.data.resource_id }} on {{ .event.source }}"

      # Send to webhook (Slack, PagerDuty, etc.)
      - type: webhook
        url: "https://hooks.slack.com/services/xxx/yyy/zzz"
        method: POST
        body:
          text: |
            :warning: Configuration drift detected!
            Host: {{ .event.source }}
            Resource: {{ .event.data.resource_id }}
            Severity: {{ .event.data.severity }}
            Differences: {{ .event.data.differences | json }}

      # Optionally, auto-remediate
      - type: event
        event_type: state.remediate
        condition: "{{ .event.data.severity }} == 'high'"
        data:
          state_file: webserver-config.yaml
          target: "{{ .event.source }}"
```

Apply the reactor:

```bash
kscorectl apply drift-reactor.yaml
```

## Step 6: Configure Auto-Remediation (Optional)

For automatic drift remediation, create a remediation reactor:

```yaml
# remediation-reactor.yaml
reactors:
  - name: auto-remediate
    description: Automatically fix configuration drift
    enabled: true

    filter:
      type: state.remediate

    # Rate limiting to prevent remediation loops
    throttle: 5m         # Max once per 5 minutes per resource
    max_executions: 3    # Max 3 remediations before stopping

    actions:
      - type: log
        level: info
        message: "Auto-remediating: {{ .event.data.state_file }} on {{ .event.data.target }}"

      - type: command
        command: |
          kscorectl state apply {{ .event.data.state_file }} \
            --target "{{ .event.data.target }}"

      - type: event
        event_type: state.remediated
        data:
          state_file: "{{ .event.data.state_file }}"
          target: "{{ .event.data.target }}"
```

## Step 7: View Drift Reports

View historical drift data:

```bash
# Show recent drift events
kscorectl events list --type state.drift --last 24h

# Show drift summary by severity
kscorectl state drift --summary

# Export drift report
kscorectl state drift --output json > drift-report.json
```

## Step 8: Configure Drift Severity

Customize which changes are considered critical:

```yaml
# In your state file
states:
  file:
    - id: /etc/nginx/nginx.conf
      state: present
      drift_detection:
        enabled: true
        severity_overrides:
          mode: critical      # Permission changes are critical
          owner: critical     # Ownership changes are critical
          contents: high      # Content changes are high
```

Default severity levels:
- **Critical**: Security-related (mode, owner, SELinux context)
- **High**: Service-affecting (contents, enabled status)
- **Medium**: Operational (non-critical settings)
- **Low**: Cosmetic changes

## Drift Detection Dashboard

If you're using Grafana, import the drift detection dashboard:

```bash
# The dashboard is pre-configured in deploy/grafana/dashboards/
kubectl apply -f deploy/grafana/dashboards/state-management.json
```

Key metrics to monitor:
- `kscore_drift_detected_total` - Total drift events
- `kscore_drift_by_severity` - Drift by severity level
- `kscore_drift_remediation_total` - Auto-remediation count
- `kscore_drift_time_to_detect_seconds` - Detection latency

## Best Practices

1. **Start with monitoring**: Enable drift detection in alert-only mode before auto-remediation

2. **Use appropriate severity**: Not all drift needs immediate attention

3. **Rate limit remediation**: Prevent remediation loops with throttling

4. **Track root causes**: Investigate why drift occurs, not just fix it

5. **Exclude expected changes**: Some drift is intentional (e.g., log files)

6. **Test remediation**: Ensure auto-remediation doesn't cause outages

## Troubleshooting

**Drift not detected:**
- Verify drift detection is enabled
- Check the interval configuration
- Ensure agent connectivity

**Too many drift alerts:**
- Adjust severity thresholds
- Exclude volatile files (logs, temp files)
- Use appropriate throttling

**Auto-remediation not working:**
- Check reactor is enabled
- Verify throttle hasn't been exceeded
- Check max_executions limit

## Next Steps

- [Event-Driven Automation](/docs/tutorials/event-automation/) - Create more complex reactors
- [Policy Enforcement](/docs/concepts/policy/) - Add compliance policies
- [Observability](/docs/concepts/observability/) - Monitor your infrastructure
