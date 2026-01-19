---
title: "Remote Execution Basics"
weight: 20
description: >
  Run commands across your infrastructure with flexible targeting.
---

## Overview

Keystone Core's remote execution system allows you to run commands on any managed node with flexible targeting. In this tutorial, you'll learn how to:
- Run commands on single or multiple agents
- Use targeting expressions
- Handle command output
- Track executions with job IDs

**Time**: 15 minutes

## Prerequisites

- Keystone Core control plane running
- Multiple agents connected (for targeting examples)
- `kscorectl` CLI configured

Verify your agents:
```bash
kscorectl agent list
```

## Step 1: Run a Simple Command

Execute a command on all agents:

```bash
kscorectl exec run "*" -- uptime
```

Output:
```
[web-001] 10:30:45 up 45 days,  3:22,  0 users,  load average: 0.15, 0.10, 0.05
[web-002] 10:30:45 up 45 days,  3:21,  0 users,  load average: 0.22, 0.18, 0.12
[db-001]  10:30:45 up 90 days,  8:15,  0 users,  load average: 1.25, 1.10, 0.95
```

The `*` targets all agents. The `--` separates exec options from the command.

## Step 2: Target by Hostname Pattern

Use glob patterns to target specific hosts:

```bash
# All web servers
kscorectl exec run "web-*" -- hostname

# All servers ending in -001
kscorectl exec run "*-001" -- hostname

# Specific hostname
kscorectl exec run "db-001" -- hostname
```

## Step 3: Target by Labels

Agents can have labels for flexible targeting:

```bash
# Target by role
kscorectl exec run "role=database" -- df -h

# Target by environment
kscorectl exec run "environment=production" -- cat /etc/os-release

# Combine labels
kscorectl exec run "role=web AND environment=production" -- nginx -t
```

View agent labels:
```bash
kscorectl agent show web-001
```

## Step 4: Use Targeting Expressions

Complex targeting with expressions:

```bash
# Linux servers only
kscorectl exec run "os=linux" -- uname -a

# Specific cloud provider
kscorectl exec run "cloud.provider=aws" -- curl -s http://169.254.169.254/latest/meta-data/instance-id

# Exclude certain hosts
kscorectl exec run "role=web AND NOT hostname=web-001" -- service nginx status

# Multiple conditions
kscorectl exec run "(role=web OR role=app) AND environment=staging" -- free -m
```

## Step 5: Handle Command Output

**Capture output to a file:**
```bash
kscorectl exec run "role=database" -- pg_dump mydb > backup.sql 2>&1
```

**Suppress per-agent results (progress only):**
```bash
kscorectl exec run "web-*" --show-results=false -- test -f /etc/nginx/nginx.conf
```

## Step 6: Set Timeouts

Configure command timeout:

```bash
# 30 second command timeout
kscorectl exec run --command-timeout 30 "db-*" -- pg_dumpall

# 5 minute timeout for long operations
kscorectl exec run --command-timeout 300 "backup-*" -- /opt/scripts/backup.sh
```

Default command timeout is 300 seconds.

## Step 7: Job Tracking

Assign a job ID to make results easy to find later:

```bash
kscorectl exec run --job-id job-abc123 "backup-*" -- /opt/scripts/full-backup.sh
```

Check job status:
```bash
kscorectl exec status job-abc123
```

## Step 8: Run as Different User

Execute commands as a specific user:

```bash
# Run as postgres user
kscorectl exec run --user postgres "db-*" -- psql -c "SELECT version();"

# Run as application user
kscorectl exec run --user appuser "app-*" -- /opt/app/bin/healthcheck
```

Note: Requires agent to run with appropriate privileges.

## Step 9: Specify Shell

Choose which shell to use:

```bash
# Use bash explicitly
kscorectl exec run "linux-*" -- bash -lc 'echo $BASH_VERSION'

# Use PowerShell on Windows
kscorectl exec run "windows-*" -- powershell -Command "Get-Process | Select-Object -First 5"

# Use cmd on Windows
kscorectl exec run "windows-*" -- cmd /c dir /b
```

## Step 10: Batch Execution

Control concurrency for large-scale execution:

```bash
# Execute on 10 agents at a time
kscorectl exec run --concurrency 10 "*" -- apt update

# Stop on first failure
kscorectl exec run --continue-on-failure=false "web-*" -- nginx -t && systemctl reload nginx
```

## Common Use Cases

### Check Disk Space
```bash
kscorectl exec run "*" -- df -h / | grep -v Filesystem
```

### Find Large Files
```bash
kscorectl exec run "role=app" -- find /var/log -type f -size +100M -exec ls -lh {} \;
```

### Check Service Status
```bash
kscorectl exec run "role=web" -- systemctl is-active nginx
```

### Collect System Info
```bash
kscorectl exec run "*" -- cat /etc/os-release
```

### Emergency Restart
```bash
kscorectl exec run --command-timeout 30 "app-*" -- systemctl restart application
```

### Rolling Restart
```bash
kscorectl exec run --concurrency 1 "web-*" -- systemctl restart nginx
```

## Best Practices

1. **Use specific targeting**: Avoid `*` in production unless intentional

2. **Test with a single agent**: Validate targeting before scaling out

3. **Set appropriate timeouts**: Don't let commands hang indefinitely

4. **Use job IDs for long commands**: Track results with `exec status`

5. **Capture output**: Redirect output for automation

6. **Batch large operations**: Use `--concurrency` to avoid overload

## Troubleshooting

**Command times out:**
```bash
# Increase timeout
kscorectl exec run --command-timeout 300 "target" -- long-command
```

**Permission denied:**
```bash
# Check agent privileges
kscorectl agent show agent-001

# Use sudo (if configured)
kscorectl exec run "target" -- sudo command
```

**No agents matched:**
```bash
# List all agents
kscorectl agent list

# Check targeting expression
kscorectl exec run "your-expression" -- hostname
```

## Next Steps

- [Multi-Tier Web Application](/docs/scenarios/multi-tier-webapp/) - Combine states with execution
- [Event-Driven Automation](/docs/scenarios/event-driven-automation/) - Trigger commands from events
- [Remote Execution Reference](/docs/reference/cli/#kscore-exec) - Complete CLI reference
