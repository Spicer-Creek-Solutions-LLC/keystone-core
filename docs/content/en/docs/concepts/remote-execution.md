---
title: "Remote Execution"
weight: 5
description: >
  Flexible command execution across infrastructure with powerful targeting and batch processing
---

## Overview

Keystone Core's remote execution system enables you to run commands across your entire infrastructure with flexible targeting, batch processing, and real-time output streaming.

**Key Features**:
- **Flexible Targeting**: Glob patterns, expressions, compound filters
- **Batch Execution**: Control concurrency and parallelism
- **Cross-Platform**: Linux (bash), Windows (PowerShell/cmd), macOS
- **Real-Time Output**: Stream stdout/stderr as commands execute
- **Job Tracking**: Monitor execution status and results
- **Plugin Architecture**: Extensible CLI via Git-style plugins

## Command Execution Flow

```
┌──────────────┐
│     User     │
│  (kscorectl)  │
└──────┬───────┘
       │ kscorectl exec run "command"
       ↓
┌──────────────┐
│  CLI Plugin  │ ← kscore-exec
│ (Dispatcher) │
└──────┬───────┘
       │ gRPC/REST API
       ↓
┌──────────────────┐
│  Control Plane   │
│ Command Dispatch │
└──────┬───────────┘
       │
       ├─ Resolve target expression
       ├─ Create job
       └─ Send to NATS
              │
              ↓
       ┌──────────────┐
       │     NATS     │
       │ Message Bus  │
       └──────┬───────┘
              │
       ┌──────┴───────┬───────────┐
       ↓              ↓           ↓
   ┌────────┐    ┌────────┐  ┌────────┐
   │Agent 1 │    │Agent 2 │  │Agent N │
   └────┬───┘    └────┬───┘  └────┬───┘
        │             │           │
        ↓             ↓           ↓
   Execute       Execute      Execute
   Command       Command      Command
        │             │           │
        └─────────┬───┴───────────┘
                  ↓
          ┌───────────────┐
          │  Results Back │
          │  via NATS     │
          └───────────────┘
```

## Targeting System

### Glob Patterns

Match agents by ID patterns:

```bash
# Single agent
kscorectl exec run "uptime" --target "web-01"

# All web servers
kscorectl exec run "systemctl status nginx" --target "web-*"

# All production database servers
kscorectl exec run "pg_dump mydb" --target "db-prod-*"

# Multiple patterns (OR logic)
kscorectl exec run "df -h" --target "web-*,api-*,cache-*"
```

### Expression-Based Targeting

Use CEL expressions for complex targeting:

```bash
# All web servers in us-east-1
kscorectl exec run "hostname" \
  --target "role:web and datacenter:us-east-1"

# Production web or API servers
kscorectl exec run "free -h" \
  --target "environment:prod and (role:web or role:api)"

# Linux servers with >16GB RAM
kscorectl exec run "cat /proc/meminfo" \
  --target "os:linux and facts.memory_total > 17179869184"

# All agents except production
kscorectl exec run "apt-get update" \
  --target "environment != 'prod'"
```

### Available Fields

**Agent Metadata**:
- `agent_id` - Unique agent identifier
- `datacenter` - Physical location (us-east-1, eu-west-1, etc.)
- `environment` - Environment (dev, staging, prod)
- `role` - Server role (web, api, db, cache, etc.)
- `os` - Operating system (linux, windows, darwin)
- `arch` - Architecture (amd64, arm64)

**Tags**:
- `tags` - Custom tags (nginx, docker, kubernetes, etc.)
- Example: `tags contains 'docker'`

**Facts** (runtime metadata):
- `facts.cpu_count` - CPU cores
- `facts.memory_total` - Total RAM in bytes
- `facts.hostname` - Hostname
- `facts.ip` - Primary IP address

### Compound Expressions

Combine multiple conditions with logical operators:

```bash
# AND
--target "role:web and datacenter:us-east-1 and environment:prod"

# OR
--target "role:web or role:api"

# NOT
--target "not environment:prod"

# Parentheses for grouping
--target "(role:web or role:api) and datacenter:us-east-1"

# Complex
--target "environment:prod and (role:web or (role:api and tags contains 'public'))"
```

## Execution Modes

### Synchronous Execution

Wait for all agents to complete (default):

```bash
kscorectl exec run "hostname" --target "role:web"
```

Output:
```
Executing on 3 agent(s)...

Agent: web-01
Status: success
Exit Code: 0
Output:
web-01.example.com

Agent: web-02
Status: success
Exit Code: 0
Output:
web-02.example.com

Agent: web-03
Status: success
Exit Code: 0
Output:
web-03.example.com

Summary: 3 succeeded, 0 failed
Duration: 1.2s
```

### Asynchronous Execution

Fire and forget, poll for results later:

```bash
# Start execution
JOB_ID=$(kscorectl exec run "apt-get update" --target "os:linux" --async)

# Check status
kscorectl exec status $JOB_ID

# Get results when ready
kscorectl exec output $JOB_ID
```

### Batch Execution

Control concurrency to avoid overwhelming infrastructure:

```bash
# Execute on max 10 agents at a time
kscorectl exec run "systemctl restart app" \
  --target "role:web" \
  --batch-size 10

# With delay between batches
kscorectl exec run "yum update -y" \
  --target "os:linux" \
  --batch-size 20 \
  --batch-delay 30s
```

Execution pattern:
```
Batch 1: [web-01, web-02, ..., web-10] ← Execute
   ↓ (wait for completion)
Batch 2: [web-11, web-12, ..., web-20] ← Execute
   ↓ (wait 30s)
Batch 3: [web-21, web-22, ..., web-30] ← Execute
...
```

## Cross-Platform Support

### Linux (bash/sh)

Default shell is bash:

```bash
kscorectl exec run "ps aux | grep nginx" --target "os:linux"
```

Specify shell:
```bash
kscorectl exec run "echo \$SHELL" --target "os:linux" --shell sh
```

### Windows (PowerShell/cmd)

PowerShell (default on Windows):

```bash
kscorectl exec run "Get-Process | Where-Object {$_.CPU -gt 10}" \
  --target "os:windows"
```

Command Prompt:
```bash
kscorectl exec run "dir C:\\" --target "os:windows" --shell cmd
```

### macOS (bash/zsh)

macOS agents (zsh is default on macOS 10.15+):

```bash
kscorectl exec run "sw_vers" --target "os:darwin"
```

## Job Management

### Job Lifecycle

```
┌─────────┐
│ Pending │ ← Job created, targets resolved
└────┬────┘
     │
     ↓
┌─────────┐
│ Running │ ← Commands sent to agents
└────┬────┘
     │
     ├─ All agents complete ─→ Completed
     ├─ Timeout reached ─────→ Failed
     └─ Partial failure ─────→ Partial Success
```

### Job Tracking

```bash
# List recent jobs
kscorectl exec jobs

# Get job details
kscorectl exec status <job-id>

# Get job output
kscorectl exec output <job-id>

# Cancel running job
kscorectl exec cancel <job-id>
```

### Job Results

Each job result includes:

```go
type JobResult struct {
    AgentID    string        // Which agent
    Status     string        // success, failed, timeout
    ExitCode   int           // Command exit code
    Stdout     string        // Standard output
    Stderr     string        // Standard error
    StartTime  time.Time     // When execution started
    Duration   time.Duration // How long it took
    Error      string        // Error message (if failed)
}
```

## Timeout Handling

### Command Timeouts

Set maximum execution time:

```bash
# 30 second timeout
kscorectl exec run "long-running-command" \
  --target "role:web" \
  --timeout 30s

# 5 minute timeout
kscorectl exec run "database-backup" \
  --target "role:db" \
  --timeout 5m
```

Behavior on timeout:
- Agent kills process after timeout
- Returns exit code 124 (timeout)
- Stderr contains "Command timed out after Xs"

### Job Timeouts

Total job timeout (all agents):

```bash
kscorectl exec run "command" \
  --target "role:web" \
  --job-timeout 10m
```

If any agent hasn't completed after 10 minutes:
- Job is marked as failed
- Incomplete agents marked as timed out
- Completed agents' results are preserved

## Output Streaming

### Real-Time Output

Stream command output as it executes:

```bash
kscorectl exec run "tail -f /var/log/app.log" \
  --target "web-01" \
  --stream
```

Output appears in real-time:
```
[web-01] 2024-01-15 10:23:45 INFO Starting application
[web-01] 2024-01-15 10:23:46 INFO Connecting to database
[web-01] 2024-01-15 10:23:47 INFO Server listening on :8080
...
```

### Output Formatting

Control output format:

```bash
# JSON output (for scripting)
kscorectl exec run "hostname" --target "role:web" --output json

# Table output
kscorectl exec run "uptime" --target "role:web" --output table

# Quiet (only errors)
kscorectl exec run "systemctl restart app" --target "role:web" --quiet
```

## Error Handling

### Partial Failures

When some agents fail, continue with others (default):

```bash
kscorectl exec run "systemctl restart nginx" --target "role:web"
```

Output:
```
Agent: web-01
Status: success
Exit Code: 0

Agent: web-02
Status: failed
Exit Code: 1
Stderr: Failed to restart nginx.service: Unit not found

Agent: web-03
Status: success
Exit Code: 0

Summary: 2 succeeded, 1 failed
```

Exit code: Non-zero if any failures

### Fail-Fast Mode

Stop on first failure:

```bash
kscorectl exec run "migrate-database" \
  --target "role:db" \
  --fail-fast
```

First failure stops execution on remaining agents.

### Retry Logic

Retry failed commands:

```bash
kscorectl exec run "flaky-command" \
  --target "role:web" \
  --retries 3 \
  --retry-delay 5s
```

Retry behavior:
1. First attempt fails → wait 5s
2. Second attempt fails → wait 5s
3. Third attempt fails → wait 5s
4. Fourth attempt fails → give up

## Plugin Architecture

Keystone Core uses a Git-style plugin system for CLI extensibility.

### Plugin Discovery

The `kscorectl` binary searches for `kscore-*` executables in `$PATH`:

```
kscorectl exec run "cmd"
   ↓
Searches for: kscore-exec
   ↓
Executes: kscore-exec run "cmd"
```

### Built-in Plugins

**kscore-exec** - Remote execution:
- `kscorectl exec run` - Execute command
- `kscorectl exec status` - Check job status
- `kscorectl exec output` - Get job output
- `kscorectl exec jobs` - List jobs
- `kscorectl exec cancel` - Cancel job

**kscore-state** - State management:
- `kscorectl state apply` - Apply state
- `kscorectl state check` - Check state (dry-run)
- `kscorectl state drift` - Detect drift

**kscore-module** - Module management:
- `kscorectl module install` - Install module
- `kscorectl module list` - List modules
- `kscorectl module update` - Update modules

### Custom Plugins

Create custom plugins by naming them `kscore-<name>`:

```bash
#!/bin/bash
# kscore-hello

echo "Hello from custom plugin!"
echo "Args: $@"
```

Make executable and add to PATH:
```bash
chmod +x kscore-hello
mv kscore-hello /usr/local/bin/
```

Use as:
```bash
kscorectl hello world
# Output: Hello from custom plugin!
#         Args: world
```

## Security

### Command Sandboxing

Agents can sandbox command execution:

```yaml
# Agent config
security:
  sandbox: true
  allowed_commands:
    - /usr/bin/systemctl
    - /usr/bin/apt-get
    - /bin/ps
  blocked_commands:
    - rm
    - dd
```

### Execution Permissions

Commands run with agent's permissions by default.

Run as specific user:
```yaml
security:
  run_as_user: "appuser"
```

### Sudo/Elevation

Require explicit sudo in command:

```bash
# Explicit sudo required
kscorectl exec run "sudo systemctl restart nginx" --target "role:web"

# No implicit elevation
kscorectl exec run "systemctl restart nginx" --target "role:web"
# → Fails with permission denied (unless agent runs as root)
```

## Performance

### Throughput

Maximum command execution rates:

- **Single control plane**: 10,000 commands/sec
- **Clustered**: 100,000+ commands/sec
- **Agents**: Each agent handles 100 concurrent commands

### Latency

Typical latencies (p95):

- **Command dispatch**: <50ms (control plane → agent)
- **Execution start**: <100ms (agent receives → starts)
- **Result return**: <20ms (agent → control plane)

Total: ~170ms overhead (plus actual command execution time)

### Scaling

**Small deployment** (100 agents):
- 1 control plane
- 10,000 commands/sec capacity
- Typical: 100-1,000 commands/sec

**Large deployment** (10,000 agents):
- 3+ control plane instances (HA)
- 100,000+ commands/sec capacity
- Batch execution recommended

## Best Practices

### Targeting

1. **Be Specific**: Use narrow targets to avoid accidents
   ```bash
   # Good
   --target "role:web and datacenter:us-east-1 and environment:staging"

   # Risky
   --target "role:web"  # Hits ALL web servers in ALL environments
   ```

2. **Test First**: Use `--dry-run` or target a single agent first
   ```bash
   # Test on one agent
   kscorectl exec run "risky-command" --target "web-01"

   # Then expand
   kscorectl exec run "risky-command" --target "role:web and environment:staging"
   ```

3. **Use Tags**: Tag critical servers for exclusion
   ```bash
   --target "role:web and not tags contains 'critical'"
   ```

### Batch Execution

1. **Start Small**: Use small batches for risky operations
   ```bash
   --batch-size 5  # For service restarts
   --batch-size 20 # For package updates
   ```

2. **Add Delays**: Give systems time to stabilize
   ```bash
   --batch-delay 30s  # Between batches
   ```

3. **Monitor**: Watch for issues in early batches before continuing

### Commands

1. **Idempotent**: Commands should be safe to run multiple times
   ```bash
   # Good (idempotent)
   systemctl restart nginx

   # Risky (not idempotent)
   echo "config" >> /etc/app.conf  # Appends every time
   ```

2. **Timeouts**: Always set reasonable timeouts
   ```bash
   --timeout 5m  # For long-running commands
   ```

3. **Error Handling**: Check exit codes in scripts
   ```bash
   kscorectl exec run "set -e; cmd1; cmd2; cmd3" --target "..."
   ```

### Output

1. **Capture Output**: Save output for audit/debugging
   ```bash
   kscorectl exec run "command" --target "..." > output.log 2>&1
   ```

2. **JSON for Automation**: Use JSON output for scripting
   ```bash
   kscorectl exec run "command" --target "..." --output json | jq '.results'
   ```

## Examples

### Rolling Restart

Restart services with zero downtime:

```bash
kscorectl exec run "systemctl restart nginx" \
  --target "role:web and environment:prod" \
  --batch-size 1 \
  --batch-delay 10s
```

### Package Updates

Update packages across fleet:

```bash
# Ubuntu/Debian
kscorectl exec run "apt-get update && apt-get upgrade -y" \
  --target "os:linux and environment:staging" \
  --batch-size 10 \
  --timeout 10m

# RHEL/CentOS
kscorectl exec run "yum update -y" \
  --target "os:linux and environment:staging" \
  --batch-size 10 \
  --timeout 10m
```

### Health Checks

Check service health across infrastructure:

```bash
kscorectl exec run "systemctl is-active nginx" \
  --target "role:web"
```

### Log Collection

Collect logs from all servers:

```bash
kscorectl exec run "tail -100 /var/log/app.log" \
  --target "role:web and environment:prod" \
  --output json > logs.json
```

### Resource Usage

Check resource usage:

```bash
# CPU
kscorectl exec run "top -bn1 | head -20" --target "role:web"

# Memory
kscorectl exec run "free -h" --target "role:web"

# Disk
kscorectl exec run "df -h" --target "role:web"
```

## Troubleshooting

### Command Not Executing

**Problem**: Command doesn't run on any agents

Check:
- Target expression matches agents: `kscorectl agent list`
- Agents are online: `kscorectl agent list --status online`
- Network connectivity: agents can reach control plane

Debug:
```bash
# List agents matching target
kscorectl agent list --filter "role:web"

# Try simple command first
kscorectl exec run "hostname" --target "role:web"
```

### Timeout Issues

**Problem**: Commands timing out

Check:
- Command actually completes in timeout window
- Network latency not delaying results

Fix:
```bash
# Increase timeout
--timeout 10m

# Test command locally first
ssh agent-01 "command"
```

### Permission Denied

**Problem**: "Permission denied" errors

Check:
- Agent user has required permissions
- Sudo configured if needed
- Command in allowed_commands list

Fix:
```bash
# Use explicit sudo
kscorectl exec run "sudo command" --target "..."

# Or configure agent to run as privileged user
```

### Partial Failures

**Problem**: Some agents succeed, others fail

Debug:
```bash
# Get detailed output
kscorectl exec output <job-id> --verbose

# Check specific failed agent
kscorectl exec run "command" --target "failed-agent-id"
```

## Next Steps

- Learn about [State Management](state-management/) for declarative configuration
- Understand [Events](events/) emitted during command execution
- Explore [Control Plane](control-plane/) command dispatcher details
- See [Agents](agents/) for command execution internals
