---
title: "Quick Start"
linkTitle: "Quick Start"
weight: 3
description: >
  Get TitanAnvil running in 5 minutes
---

This quick start guide gets you from zero to a working TitanAnvil deployment in about 5 minutes.

## What You'll Do

1. Start a control plane (embedded mode)
2. Start an agent
3. Execute your first remote command
4. Apply your first state configuration
5. View metrics

**Prerequisites**: TitanAnvil binaries installed ([Installation Guide](../installation/))

## Step 1: Start the Control Plane (30 seconds)

Start the control plane in embedded mode (no external dependencies):

```bash
# Create data directory
mkdir -p ~/titananvil-data

# Start control plane (foreground)
titananvil-server \
  --nats-mode embedded \
  --nats-listen 127.0.0.1:4222 \
  --api-listen 127.0.0.1:8080 \
  --storage-type sqlite \
  --storage-path ~/titananvil-data/state.db
```

**Expected output**:
```
INFO  Starting TitanAnvil Control Plane
INFO  NATS server starting in embedded mode on 127.0.0.1:4222
INFO  API server starting on 127.0.0.1:8080
INFO  Storage: SQLite at ~/titananvil-data/state.db
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
titananvil-agent \
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
titanctl agent list
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
titanctl exec run "echo 'Hello from TitanAnvil!'" \
  --target "agent_id:my-first-agent"
```

**Expected output**:
```
Executing on 1 agent(s)...

Agent: my-first-agent
Status: success
Exit Code: 0
Output:
Hello from TitanAnvil!

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
  path: /tmp/titananvil-test.txt
  contents: "TitanAnvil was here!"
  mode: "0644"
EOF
```

Apply the state:

```bash
titanctl state apply /tmp/test-state.yaml \
  --target "agent_id:my-first-agent"
```

**Expected output**:
```
Applying state to 1 agent(s)...

Agent: my-first-agent
  test_file: ✓ created

Summary:
  Total: 1
  Succeeded: 1
  Changed: 1
  Unchanged: 0
  Failed: 0
```

Verify the file was created:

```bash
titanctl exec run "cat /tmp/titananvil-test.txt" \
  --target "agent_id:my-first-agent"
```

**Expected output**:
```
Agent: my-first-agent
Status: success
Output:
TitanAnvil was here!
```

✅ **Checkpoint**: State management works

## Step 7: Detect Configuration Drift (30 seconds)

Modify the file manually:

```bash
titanctl exec run "echo 'Modified!' > /tmp/titananvil-test.txt" \
  --target "agent_id:my-first-agent"
```

Check for drift:

```bash
titanctl state check /tmp/test-state.yaml \
  --target "agent_id:my-first-agent"
```

**Expected output**:
```
Checking state on 1 agent(s)...

Agent: my-first-agent
  test_file: ✗ drift detected
    - contents: expected "TitanAnvil was here!", got "Modified!\n"

Drift Summary:
  Total: 1
  Compliant: 0
  Drift: 1
```

Fix the drift:

```bash
titanctl state apply /tmp/test-state.yaml \
  --target "agent_id:my-first-agent"
```

**Expected output**:
```
Agent: my-first-agent
  test_file: ✓ updated (drift fixed)

Summary: 1 succeeded, 1 changed
```

✅ **Checkpoint**: Drift detection and remediation works

## Step 8: View Metrics (20 seconds)

Check Prometheus metrics:

```bash
curl http://localhost:8080/metrics | grep titananvil
```

**Sample output**:
```
titananvil_agents_connected 1
titananvil_commands_executed_total 3
titananvil_state_applications_total 2
titananvil_events_published_total 12
...
```

✅ **Checkpoint**: Observability is working

## Step 9: Monitor in Real-Time (30 seconds)

Launch the TUI monitor:

```bash
titananvil-monitor
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
- **[Tutorials](../../tutorials/)** - Step-by-step guides for common tasks
- **[Concepts](../../concepts/)** - Deep dives into each subsystem

### Try More Features
- **[Event Reactors](../../tutorials/event-reactors/)** - Automate responses to events
- **[GitOps Integration](../../tutorials/gitops-workflow/)** - Connect to ArgoCD/Flux
- **[Policy Enforcement](../../tutorials/policy-rules/)** - Write OPA/CEL policies
- **[Multi-Environment](../../concepts/multi-environment/)** - Manage K8s, VMs, and cloud

### Production Deployment
- **[High Availability](../../operations/deployment/high-availability/)** - HA setup
- **[Monitoring Setup](../../operations/monitoring/metrics/)** - Prometheus + Grafana
- **[Security Hardening](../../reference/configuration/security/)** - Production security
- **[Scaling Guide](../../operations/deployment/scaling/)** - Scale to thousands of nodes

## Cleanup

To stop everything:

```bash
# Stop agent (Ctrl+C in agent terminal)
# Stop control plane (Ctrl+C in server terminal)

# Remove data
rm -rf ~/titananvil-data
rm /tmp/test-state.yaml
rm /tmp/titananvil-test.txt
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
titanctl agent list
```

Ensure your target expression matches an online agent.

### State Application Fails

**Error**: `permission denied`

**Fix**: Ensure the agent has permission to modify the file. Try a different path like `/tmp/` instead of system directories.

### Port Already in Use

**Error**: `bind: address already in use`

**Fix**: Change the port:
```bash
titananvil-server --api-listen 127.0.0.1:8081 --nats-listen 127.0.0.1:4223
```

## Next Steps

Continue to [Architecture Overview](../architecture/) to understand how TitanAnvil works under the hood, or jump into [Tutorials](../../tutorials/) for hands-on guides.
