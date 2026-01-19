---
title: "Video Tutorial Series Outlines"
linkTitle: "Video Tutorials"
weight: 100
description: >
  Production scripts and outlines for Keystone Core video tutorial series.
---

This document contains detailed outlines for a video tutorial series covering Keystone Core from installation to advanced topics. Each video is designed to be 5-15 minutes and follows a consistent format.

## Series Overview

**Target Audience**: DevOps engineers, SREs, and platform engineers familiar with infrastructure management tools.

**Series Format**:
- 8 videos covering fundamentals
- Each video is self-contained but builds on previous concepts
- Includes hands-on demos with sample code
- Follows the "explain, demonstrate, practice" pattern

**Prerequisites Across Series**:
- Linux or macOS system (Windows covered separately)
- Docker or Podman installed
- Basic YAML and terminal knowledge
- Optional: Kubernetes cluster for advanced topics

---

## Video 1: Introduction to Keystone Core

**Duration**: 10-12 minutes
**Goal**: Understand what Keystone Core is and when to use it

### Outline

1. **Opening Hook** (1 min)
   - "What happens after GitOps deploys your application?"
   - Configuration drift, policy enforcement, automated responses

2. **What is Keystone Core?** (3 min)
   - Cloud-native runtime infrastructure control plane
   - Key analogy: "Kubernetes orchestrates containers, Keystone orchestrates infrastructure operations"
   - Positioning: Complements GitOps tools like ArgoCD/Flux

3. **Core Features Overview** (4 min)
   - State Management: Declarative configuration (show diagram)
   - Remote Execution: Run commands across infrastructure
   - Event System: Reactors for automated responses
   - Policy Enforcement: OPA/CEL integration
   - GitOps Integration: Webhooks, verification, rollback
   - Observability: Built-in metrics, traces, logs

4. **Architecture at a Glance** (2 min)
   - Control plane (server)
   - Agents
   - NATS messaging
   - Show architecture diagram

5. **When to Use Keystone Core** (1 min)
   - Good fit: Heterogeneous infrastructure, runtime operations
   - Not ideal: Tiny deployments, container-only orchestration

6. **What's Next** (30 sec)
   - Preview of installation video
   - Links to documentation

### Demo Assets
- Architecture diagrams (Mermaid)
- Slide deck with key points
- No code in this video

---

## Video 2: Installation Guide

**Duration**: 8-10 minutes
**Goal**: Successfully install Keystone Core on a local system

### Outline

1. **Introduction** (30 sec)
   - What we'll cover: Server and agent installation
   - Platform support overview

2. **Prerequisites Check** (1 min)
   - Docker/Podman version
   - System requirements (CPU, memory)
   - Network ports (4222, 6222, 8080)

3. **Option A: Docker Installation** (3 min)
   ```bash
   # Pull and run server
   docker run -d --name keystone-server \
     -p 4222:4222 -p 6222:6222 -p 8080:8080 \
     keystonecore/server:latest

   # Verify server is running
   docker logs keystone-server

   # Pull and run agent
   docker run -d --name keystone-agent \
     --network host \
     keystonecore/agent:latest --server=localhost:4222
   ```

4. **Option B: Binary Installation** (2 min)
   ```bash
   # Download latest release
   curl -sSL https://get.keystonecore.io | sh

   # Or download specific version
   wget https://github.com/keystone/releases/v0.1.0/keystone-linux-amd64

   # Start server
   keystone server start --config /etc/keystone/server.yaml

   # Start agent
   keystone agent start --server=localhost:4222
   ```

5. **Verification** (2 min)
   ```bash
   # Check server status
   keystone status

   # List connected agents
   keystone agent list

   # Run a test ping
   keystone exec '*' cmd.run 'echo hello'
   ```

6. **Troubleshooting Tips** (1 min)
   - Common issues: ports in use, firewall rules
   - Where to find logs
   - Getting help

7. **Cleanup** (30 sec)
   - Stopping containers/services
   - Next video preview

### Demo Assets
- Terminal recording
- Sample configuration files
- Troubleshooting checklist

---

## Video 3: Quick Start - Your First 5 Minutes

**Duration**: 5-7 minutes
**Goal**: Get a working Keystone Core setup in 5 minutes

### Outline

1. **The 5-Minute Promise** (30 sec)
   - By the end: working server, agent, and first command

2. **One-Line Setup** (2 min)
   ```bash
   # All-in-one quickstart
   curl -sSL https://get.keystonecore.io/quickstart | sh

   # This starts:
   # - Keystone server (embedded mode)
   # - Local agent
   # - Web UI at http://localhost:8080
   ```

3. **Verify It's Working** (1 min)
   ```bash
   # Check status
   keystone status

   # Should show:
   # Server: running (embedded mode)
   # Agents: 1 connected
   # NATS: healthy
   ```

4. **First Command** (1 min)
   ```bash
   # Run command on all agents
   keystone exec '*' cmd.run 'hostname'

   # Check disk space
   keystone exec '*' cmd.run 'df -h'

   # Get system info
   keystone exec '*' grains.items
   ```

5. **Explore the Web UI** (1 min)
   - Navigate to http://localhost:8080
   - Show agent list
   - Show recent jobs
   - Show metrics dashboard

6. **Next Steps** (30 sec)
   - Hello World state tutorial
   - Documentation links

### Demo Assets
- Terminal recording
- Quickstart script
- Sample outputs

---

## Video 4: Hello World - Your First State

**Duration**: 10-12 minutes
**Goal**: Create and apply a declarative state file

### Outline

1. **What is State Management?** (2 min)
   - Declarative vs imperative
   - Idempotency explained
   - Salt heritage: states, vars, facts

2. **Create Your First State** (4 min)
   ```yaml
   # hello-world.yaml
   ensure_motd:
     file.managed:
       - name: /tmp/keystone-hello.txt
       - contents: |
           Hello from Keystone Core!
           Managed at: {{ grains['timestamp'] }}
       - user: root
       - group: root
       - mode: '0644'

   ensure_packages:
     pkg.installed:
       - names:
           - curl
           - jq
   ```

3. **Understanding State Structure** (2 min)
   - State ID (unique identifier)
   - State module (file, pkg, service)
   - State function (managed, installed)
   - Arguments (name, contents, etc.)

4. **Apply the State** (2 min)
   ```bash
   # Preview changes (dry-run)
   keystone state.apply hello-world test=true

   # Apply for real
   keystone state.apply hello-world

   # Check results
   keystone state.show_highstate
   ```

5. **Verify Results** (1 min)
   ```bash
   # Check the file was created
   keystone exec '*' cmd.run 'cat /tmp/keystone-hello.txt'

   # Check packages are installed
   keystone exec '*' cmd.run 'which curl jq'
   ```

6. **Making Changes** (1 min)
   - Modify the state
   - Re-apply to see updates
   - Idempotency in action

7. **Cleanup and Next Steps** (30 sec)

### Demo Assets
- hello-world.yaml state file
- Terminal recording
- Before/after screenshots

---

## Video 5: Remote Execution Basics

**Duration**: 10-12 minutes
**Goal**: Run commands across infrastructure using targeting

### Outline

1. **Remote Execution Overview** (1 min)
   - Run any command on any agent
   - Targeting: which agents to run on
   - Modules: what commands are available

2. **Basic Command Execution** (2 min)
   ```bash
   # Run on all agents
   keystone exec '*' cmd.run 'uptime'

   # Run on specific agent
   keystone exec 'web-server-01' cmd.run 'nginx -t'

   # Run with timeout
   keystone exec '*' cmd.run 'sleep 30' --timeout=10
   ```

3. **Targeting Patterns** (3 min)
   ```bash
   # Glob patterns
   keystone exec 'web-*' cmd.run 'uptime'

   # Regular expressions
   keystone exec -E 'web-[0-9]+' cmd.run 'uptime'

   # Grain targeting (by OS, role, etc.)
   keystone exec -G 'os:Ubuntu' cmd.run 'apt update'

   # Compound targeting
   keystone exec -C 'G@os:Ubuntu and web-*' cmd.run 'nginx -v'

   # Pillar targeting
   keystone exec -I 'role:webserver' cmd.run 'uptime'
   ```

4. **Useful Execution Modules** (3 min)
   ```bash
   # File operations
   keystone exec '*' file.read /etc/hostname
   keystone exec '*' file.stats /var/log/syslog

   # Package management
   keystone exec '*' pkg.list_pkgs
   keystone exec '*' pkg.version nginx

   # Service management
   keystone exec '*' service.status nginx
   keystone exec '*' service.restart nginx

   # System information
   keystone exec '*' grains.items
   keystone exec '*' grains.get os

   # Disk and memory
   keystone exec '*' disk.usage
   keystone exec '*' status.meminfo
   ```

5. **Output Formats** (1 min)
   ```bash
   # JSON output
   keystone exec '*' grains.items --out=json

   # YAML output
   keystone exec '*' grains.items --out=yaml

   # Quiet mode (just return values)
   keystone exec '*' cmd.run 'hostname' --out=quiet
   ```

6. **Async Execution** (1 min)
   ```bash
   # Run asynchronously
   keystone exec '*' cmd.run 'long-running-script.sh' --async

   # Check job status
   keystone job lookup <job-id>

   # List recent jobs
   keystone job list
   ```

7. **Next Steps** (30 sec)

### Demo Assets
- Command reference sheet
- Targeting cheat sheet
- Terminal recording with multiple agents

---

## Video 6: Event-Driven Automation

**Duration**: 12-15 minutes
**Goal**: Create reactors that respond to infrastructure events

### Outline

1. **Event System Overview** (2 min)
   - Events: what happened
   - Reactors: automated responses
   - Use cases: auto-remediation, notifications, workflows

2. **Understanding Events** (2 min)
   ```bash
   # Watch events in real-time
   keystone event.watch

   # Sample events:
   # - keystone/agent/connected
   # - keystone/state/applied
   # - keystone/job/completed
   # - custom/app/deployment
   ```

3. **Creating a Reactor** (4 min)
   ```yaml
   # /etc/keystone/reactors/agent-connected.yaml
   name: new-agent-setup
   trigger:
     event: keystone/agent/connected

   actions:
     - name: log-connection
       cmd.run:
         - echo "New agent connected: {{ event.data.id }}"

     - name: apply-baseline
       state.apply:
         - baseline
         - target: "{{ event.data.id }}"

     - name: notify-slack
       http.post:
         url: https://hooks.slack.com/services/xxx
         body:
           text: "Agent {{ event.data.id }} joined the cluster"
   ```

4. **Reactor Patterns** (3 min)
   ```yaml
   # Auto-remediate high disk usage
   name: disk-alert-remediation
   trigger:
     event: monitor/disk/high
     condition: "event.data.usage > 90"

   actions:
     - name: cleanup-logs
       cmd.run:
         - find /var/log -name '*.gz' -mtime +7 -delete
         - journalctl --vacuum-time=3d
       target: "{{ event.data.agent }}"

     - name: notify-team
       http.post:
         url: "{{ pillar.pagerduty_webhook }}"
         body:
           event_type: disk_cleanup
           agent: "{{ event.data.agent }}"
   ```

5. **Firing Custom Events** (2 min)
   ```bash
   # Fire event from CLI
   keystone event.fire 'custom/deploy/started' \
     data='{"app":"myapp","version":"1.2.3"}'

   # Fire event from state
   fire_deployment_event:
     event.fire:
       - name: custom/deploy/completed
       - data:
           app: myapp
           status: success
   ```

6. **Testing Reactors** (1 min)
   ```bash
   # Test reactor without side effects
   keystone reactor.test new-agent-setup \
     --event='{"data":{"id":"test-agent"}}'

   # View reactor logs
   keystone reactor.logs new-agent-setup
   ```

7. **Next Steps** (30 sec)

### Demo Assets
- Reactor templates
- Event diagram
- Terminal recording

---

## Video 7: GitOps Integration

**Duration**: 12-15 minutes
**Goal**: Integrate Keystone Core with GitOps workflows

### Outline

1. **Why GitOps + Keystone Core?** (2 min)
   - GitOps deploys, Keystone verifies and operates
   - Webhook integration
   - Deployment verification gates
   - Automated rollback

2. **Setting Up Webhooks** (3 min)
   ```yaml
   # /etc/keystone/gitops/webhook-config.yaml
   webhooks:
     - name: argocd-sync
       path: /webhooks/argocd
       secret: ${ARGOCD_WEBHOOK_SECRET}
       events:
         - sync.succeeded
         - sync.failed

     - name: github-push
       path: /webhooks/github
       secret: ${GITHUB_WEBHOOK_SECRET}
       events:
         - push
         - pull_request.merged
   ```

3. **ArgoCD Integration** (3 min)
   ```yaml
   # ArgoCD Application with Keystone verification
   apiVersion: argoproj.io/v1alpha1
   kind: Application
   metadata:
     name: my-app
   spec:
     syncPolicy:
       syncOptions:
         - Validate=true
     # Keystone verification gate
     annotations:
       keystone.io/verify: "true"
       keystone.io/verify-timeout: "300"
       keystone.io/rollback-on-fail: "true"
   ```

   ```yaml
   # Keystone verification config
   # /etc/keystone/gitops/verifications/my-app.yaml
   name: my-app-verification
   checks:
     - name: health-check
       type: http
       url: "http://my-app.default.svc/health"
       expect:
         status: 200
         body_contains: "healthy"

     - name: pod-ready
       type: kubernetes
       kind: Deployment
       namespace: default
       name: my-app
       condition:
         ready: true
         minReplicas: 3

     - name: error-rate
       type: prometheus
       query: "rate(http_errors_total[5m])"
       threshold: "< 0.01"
   ```

4. **Deployment Verification Flow** (2 min)
   ```mermaid
   sequenceDiagram
     ArgoCD->>Keystone: Webhook: sync.succeeded
     Keystone->>App: Health checks
     Keystone->>Prometheus: Query metrics
     alt Verification passed
       Keystone->>ArgoCD: Mark healthy
     else Verification failed
       Keystone->>ArgoCD: Trigger rollback
       Keystone->>Slack: Alert team
     end
   ```

5. **State from Git** (2 min)
   ```yaml
   # GitOps state sync config
   gitops:
     source:
       repo: https://github.com/org/infra-states.git
       branch: main
       path: states/

     sync:
       interval: 60s
       auto_apply: true
       environments:
         - name: dev
           path: states/dev/
           auto_apply: true
         - name: prod
           path: states/prod/
           auto_apply: false
           require_approval: true
   ```

6. **Automated Rollback** (1 min)
   ```yaml
   rollback:
     enabled: true
     triggers:
       - verification_failed
       - health_check_timeout
     strategy: previous_revision
     notify:
       - slack
       - pagerduty
   ```

7. **Next Steps** (30 sec)

### Demo Assets
- Integration diagrams
- Sample ArgoCD configs
- Verification templates

---

## Video 8: Policy Enforcement with OPA

**Duration**: 10-12 minutes
**Goal**: Enforce compliance policies across infrastructure

### Outline

1. **Policy-as-Code Overview** (2 min)
   - OPA (Open Policy Agent) integration
   - CEL (Common Expression Language) support
   - Continuous compliance vs point-in-time audits

2. **Creating Policies** (3 min)
   ```rego
   # /etc/keystone/policies/security.rego
   package keystone.security

   # Deny root SSH access
   deny[msg] {
     input.type == "file"
     input.path == "/etc/ssh/sshd_config"
     contains(input.content, "PermitRootLogin yes")
     msg := "Root SSH login must be disabled"
   }

   # Require firewall enabled
   deny[msg] {
     input.type == "service"
     input.name == "ufw"
     input.state != "running"
     msg := "Firewall (ufw) must be running"
   }

   # Enforce minimum password length
   deny[msg] {
     input.type == "file"
     input.path == "/etc/pam.d/common-password"
     not contains(input.content, "minlen=12")
     msg := "Password minimum length must be 12 characters"
   }
   ```

3. **Applying Policies** (2 min)
   ```bash
   # Test policy against current state
   keystone policy.check security --target='*'

   # Apply policies with enforcement
   keystone policy.enforce security --target='web-*'

   # Generate compliance report
   keystone policy.report security --format=json > compliance.json
   ```

4. **Policy Categories** (2 min)
   ```yaml
   # CIS Benchmark policies
   policies:
     - name: cis-level-1
       type: opa
       source: policies/cis/level-1.rego
       severity: high
       enforcement: strict

     - name: custom-security
       type: opa
       source: policies/custom/security.rego
       severity: medium
       enforcement: warn
   ```

5. **Continuous Compliance** (1 min)
   ```yaml
   # Compliance monitoring config
   compliance:
     scan_interval: 1h
     policies:
       - cis-level-1
       - custom-security

     reporting:
       prometheus: true
       webhook: https://compliance.example.com/reports

     enforcement:
       auto_remediate: true
       notify_on_violation: true
   ```

6. **Compliance Dashboard** (1 min)
   - Show Grafana dashboard
   - Policy pass/fail metrics
   - Trend over time

7. **Next Steps** (30 sec)

### Demo Assets
- Sample Rego policies
- Compliance report examples
- Grafana dashboard JSON

---

## Production Notes

### Recording Guidelines

1. **Audio**
   - Use quality microphone
   - Record in quiet environment
   - Target -16 to -20 dB levels

2. **Video**
   - 1920x1080 resolution
   - 60 FPS for terminal demos
   - Use large font in terminal (16pt+)
   - Dark theme for code/terminal

3. **Editing**
   - Add chapter markers
   - Include timestamps in description
   - Add closed captions
   - Include intro/outro bumpers

### Distribution

- YouTube (primary)
- Documentation site (embedded)
- Learning management system integration

### Maintenance

- Review and update quarterly
- Track questions/feedback
- Update for new features

---

## Supplementary Materials

Each video should have accompanying:

1. **Written transcript** - Full text of narration
2. **Sample code repository** - All code shown
3. **Exercise files** - Hands-on practice
4. **Cheat sheet** - Quick reference PDF
5. **Quiz questions** - Knowledge check
