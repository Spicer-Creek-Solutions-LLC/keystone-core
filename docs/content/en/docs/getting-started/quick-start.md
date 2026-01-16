---
title: "Quick Start"
linkTitle: "Quick Start"
weight: 3
description: >
  Get Keystone Core running in 5 minutes
---

This quick start guide gets you from zero to a working Keystone Core deployment in about 5 minutes.

## What You'll Do

1. Start a control plane (embedded mode)
2. Start an agent
3. Execute your first remote command
4. Apply your first state configuration
5. View metrics

**Prerequisites**: Keystone Core binaries installed ([Installation Guide](../installation/))

## Step 1: Start the Control Plane (30 seconds)

Start the control plane in embedded mode (no external dependencies):

```bash
# Create data directory
mkdir -p ~/kscore-data

# Start control plane (foreground)
kscore-server \
  --nats-mode embedded \
  --nats-listen 127.0.0.1:4222 \
  --api-listen 127.0.0.1:8080 \
  --storage-type sqlite \
  --storage-path ~/kscore-data/state.db
```

**Expected output**:
```
INFO  Starting Keystone Core Control Plane
INFO  NATS server starting in embedded mode on 127.0.0.1:4222
INFO  API server starting on 127.0.0.1:8080
INFO  Storage: SQLite at ~/kscore-data/state.db
INFO  Control plane ready
```

✅ **Checkpoint**: Control plane is running

Open a new terminal for the next steps.

## Step 2: Verify Control Plane (10 seconds)

Check health:

```bash
curl http://localhost:8080/health/live
# Expected: {"status":"healthy"}

curl http://localhost:8080/health/ready
# Expected: {"status":"ready"}
```

✅ **Checkpoint**: Control plane is healthy

## Step 3: Start an Agent (20 seconds)

In a new terminal, start an agent:

```bash
kscore-agent \
  --control-plane-url nats://127.0.0.1:4222 \
  --agent-id my-first-agent \
  --datacenter local \
  --environment dev \
  --role test
```

**Expected output**:
```
INFO  Connecting to control plane at nats://127.0.0.1:4222
INFO  Agent ID: my-first-agent
INFO  Registering with control plane...
INFO  Agent registered successfully
INFO  Heartbeat started (interval: 30s)
INFO  Subscribed to commands
INFO  Agent ready
```

✅ **Checkpoint**: Agent is connected

## Step 4: List Agents (5 seconds)

In a third terminal, verify the agent is registered:

```bash
kscorectl agent list
```

**Expected output**:
```
AGENT ID         DATACENTER  ENVIRONMENT  ROLE   STATUS   LAST HEARTBEAT
my-first-agent   local       dev          test   online   2s ago
```

✅ **Checkpoint**: Agent is visible and online

## Step 5: Execute a Remote Command (10 seconds)

Run a command on the agent:

```bash
kscorectl exec run "echo 'Hello from Keystone Core!'" \
  --target "agent_id:my-first-agent"
```

**Expected output**:
```
Executing on 1 agent(s)...

Agent: my-first-agent
Status: success
Exit Code: 0
Output:
Hello from Keystone Core!

Summary: 1 succeeded, 0 failed
```

✅ **Checkpoint**: Remote execution works

## Step 6: Apply a State Configuration (45 seconds)

Create a simple state file:

```bash
cat > /tmp/test-state.yaml <<EOF
test_file:
  module: file
  state: present
  path: /tmp/kscore-test.txt
  contents: "Keystone Core was here!"
  mode: "0644"
EOF
```

Apply the state:

```bash
kscorectl state apply /tmp/test-state.yaml
```

This applies locally on the host running `kscorectl` (which is the same host as the agent in this quick start).

**Expected output**:
```
Loading state file: /tmp/test-state.yaml
Applying state: Create a test file

=== Results ===
✓ file.test_file: changed

=== Summary ===
Total states:  1
Succeeded:     1
Failed:        0
Changed:       1
Unchanged:     0
```

Verify the file was created:

```bash
kscorectl exec run "cat /tmp/kscore-test.txt" \
  --target "agent_id:my-first-agent"
```

**Expected output**:
```
Agent: my-first-agent
Status: success
Output:
Keystone Core was here!
```

✅ **Checkpoint**: State management works

## Step 7: Detect Configuration Drift (30 seconds)

Modify the file manually:

```bash
kscorectl exec run "echo 'Modified!' > /tmp/kscore-test.txt" \
  --target "agent_id:my-first-agent"
```

Check for drift:

```bash
kscorectl state drift /tmp/test-state.yaml
```

**Expected output**:
```
Checking drift for: Create a test file

=== Drift Report ===
Run ID: ...
Checked: ...
Duration: ...

=== Summary ===
Total states: 1
No drift: 0
Low drift: 0
Medium drift: 1
High drift: 0
Critical drift: 0
Overall severity: medium

--- file.test_file [medium] ---
  - contents: expected \"Keystone Core was here!\", got \"Modified!\\n\"
```

Fix the drift:

```bash
kscorectl state apply /tmp/test-state.yaml
```

**Expected output**:
```
Loading state file: /tmp/test-state.yaml
Applying state: Create a test file

=== Results ===
✓ file.test_file: changed

=== Summary ===
Total states:  1
Succeeded:     1
Failed:        0
Changed:       1
Unchanged:     0
```

✅ **Checkpoint**: Drift detection and remediation works

## Step 8: View Metrics (20 seconds)

Check Prometheus metrics:

```bash
curl http://localhost:8080/metrics | grep kscore
```

**Sample output**:
```
kscore_agents_connected 1
kscore_commands_executed_total 3
kscore_state_applications_total 2
kscore_events_published_total 12
...
```

✅ **Checkpoint**: Observability is working

## Step 9: Monitor in Real-Time (30 seconds)

Launch the TUI monitor:

```bash
kscore-monitor
```

**What you'll see**:
- Dashboard view (press `1`)
- Connected agents (press `2`)
- Recent events (press `3`)
- Metrics overview (press `8`)

Press `q` to quit.

✅ **Checkpoint**: Real-time monitoring works

## 🎉 Success!

In about 5 minutes, you've:
- ✅ Started a control plane
- ✅ Connected an agent
- ✅ Executed remote commands
- ✅ Applied declarative state
- ✅ Detected and fixed drift
- ✅ Viewed metrics and monitoring

## What's Next?

### Learn More
- **[Architecture Overview](../architecture/)** - Understand how it all works
- **[Concepts](../../concepts/)** - Deep dives into each subsystem
- **[Reference](../../reference/)** - Complete API and configuration reference

### Try More Features
- **[Event Reactors](../../concepts/reactors/)** - Automate responses to events
- **[GitOps Integration](../../concepts/gitops/)** - Connect to ArgoCD/Flux
- **[Policy Enforcement](../../concepts/policy/)** - Write OPA/CEL policies
- **[Multi-Environment](../../concepts/multi-environment/)** - Manage K8s, VMs, and cloud

### Production Deployment
- **[High Availability](../../operations/cluster-management/)** - HA cluster setup
- **[Monitoring Setup](../../operations/monitoring/)** - Prometheus + Grafana dashboards
- **[Security Hardening](../../operations/security/)** - Production security
- **[Deployment Guide](../../operations/deployment/)** - Scale to thousands of nodes

## Cleanup

To stop everything:

```bash
# Stop agent (Ctrl+C in agent terminal)
# Stop control plane (Ctrl+C in server terminal)

# Remove data
rm -rf ~/kscore-data
rm /tmp/test-state.yaml
rm /tmp/kscore-test.txt
```

## Troubleshooting

### Agent Won't Connect

**Error**: `failed to connect to control plane`

**Fix**: Ensure control plane is running and reachable:
```bash
curl http://localhost:8080/health/live
nc -zv 127.0.0.1 4222
```

### Command Execution Fails

**Error**: `no agents matched target`

**Fix**: Check agent status:
```bash
kscorectl agent list
```

Ensure your target expression matches an online agent.

### State Application Fails

**Error**: `permission denied`

**Fix**: Ensure the agent has permission to modify the file. Try a different path like `/tmp/` instead of system directories.

### Port Already in Use

**Error**: `bind: address already in use`

**Fix**: Change the port:
```bash
kscore-server --api-listen 127.0.0.1:8081 --nats-listen 127.0.0.1:4223
```

## Next Steps

Apply the minimal [Hello World](../hello-world/) state, continue to the [Architecture Overview](../architecture/), or explore [Concepts](../../concepts/) for in-depth feature guides.
