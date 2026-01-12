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
- Execute commands asynchronously

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
kscorectl exec "*" -- uptime
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
kscorectl exec "web-*" -- hostname

# All servers ending in -001
kscorectl exec "*-001" -- hostname

# Specific hostname
kscorectl exec "db-001" -- hostname
```

## Step 3: Target by Labels

Agents can have labels for flexible targeting:

```bash
# Target by role
kscorectl exec "role=database" -- df -h

# Target by environment
kscorectl exec "environment=production" -- cat /etc/os-release

# Combine labels
kscorectl exec "role=web AND environment=production" -- nginx -t
```

View agent labels:
```bash
kscorectl agent show web-001
```

## Step 4: Use Targeting Expressions

Complex targeting with expressions:

```bash
# Linux servers only
kscorectl exec "os=linux" -- uname -a

# Specific cloud provider
kscorectl exec "cloud.provider=aws" -- curl -s http://169.254.169.254/latest/meta-data/instance-id

# Exclude certain hosts
kscorectl exec "role=web AND NOT hostname=web-001" -- service nginx status

# Multiple conditions
kscorectl exec "(role=web OR role=app) AND environment=staging" -- free -m
```

## Step 5: Handle Command Output

**Capture output to a file:**
```bash
kscorectl exec "role=database" -- pg_dump mydb > backup.sql 2>&1
```

**Format output as JSON:**
```bash
kscorectl exec "web-*" --output json -- df -h
```

Output:
```json
{
  "results": [
    {
      "agent": "web-001",
      "exit_code": 0,
      "stdout": "Filesystem      Size  Used Avail Use% Mounted on\n...",
      "stderr": "",
      "duration_ms": 45
    }
  ]
}
```

**Quiet mode (exit codes only):**
```bash
kscorectl exec "web-*" --quiet -- test -f /etc/nginx/nginx.conf
echo $?  # 0 if all succeeded, non-zero if any failed
```

## Step 6: Set Timeouts

Configure command timeout:

```bash
# 30 second timeout
kscorectl exec --timeout 30s "db-*" -- pg_dumpall

# 5 minute timeout for long operations
kscorectl exec --timeout 5m "backup-*" -- /opt/scripts/backup.sh
```

Default timeout is 60 seconds.

## Step 7: Async Execution

For long-running commands, use async mode:

```bash
# Start async job
kscorectl exec --async "backup-*" -- /opt/scripts/full-backup.sh
```

Output:
```
Job started: job-abc123
Use 'kscorectl exec status job-abc123' to check progress
```

Check job status:
```bash
kscorectl exec status job-abc123
```

Output:
```
Job: job-abc123
Status: running
Started: 2024-01-15T10:30:00Z
Agents:
  backup-001: running (45%)
  backup-002: completed (exit_code: 0)
```

Get final results:
```bash
kscorectl exec output job-abc123
```

## Step 8: Run as Different User

Execute commands as a specific user:

```bash
# Run as postgres user
kscorectl exec --user postgres "db-*" -- psql -c "SELECT version();"

# Run as application user
kscorectl exec --user appuser "app-*" -- /opt/app/bin/healthcheck
```

Note: Requires agent to run with appropriate privileges.

## Step 9: Specify Shell

Choose which shell to use:

```bash
# Use bash explicitly
kscorectl exec --shell bash "linux-*" -- 'echo $BASH_VERSION'

# Use PowerShell on Windows
kscorectl exec --shell powershell "windows-*" -- Get-Process | Select-Object -First 5

# Use cmd on Windows
kscorectl exec --shell cmd "windows-*" -- dir /b
```

## Step 10: Batch Execution

Control concurrency for large-scale execution:

```bash
# Execute on 10 agents at a time
kscorectl exec --batch-size 10 "*" -- apt update

# With delay between batches
kscorectl exec --batch-size 5 --batch-delay 10s "*" -- systemctl restart myservice

# Stop on first failure
kscorectl exec --fail-fast "web-*" -- nginx -t && systemctl reload nginx
```

## Common Use Cases

### Check Disk Space
```bash
kscorectl exec "*" -- df -h / | grep -v Filesystem
```

### Find Large Files
```bash
kscorectl exec "role=app" -- find /var/log -type f -size +100M -exec ls -lh {} \;
```

### Check Service Status
```bash
kscorectl exec "role=web" -- systemctl is-active nginx
```

### Collect System Info
```bash
kscorectl exec "*" --output json -- cat /etc/os-release | jq '.results[].stdout'
```

### Emergency Restart
```bash
kscorectl exec --timeout 30s "app-*" -- systemctl restart application
```

### Rolling Restart
```bash
kscorectl exec --batch-size 1 --batch-delay 30s "web-*" -- systemctl restart nginx
```

## Best Practices

1. **Use specific targeting**: Avoid `*` in production unless intentional

2. **Test with check first**: Use `--dry-run` to see which agents match

3. **Set appropriate timeouts**: Don't let commands hang indefinitely

4. **Use async for long commands**: Don't block the CLI for backups, etc.

5. **Capture output**: Use `--output json` for automation

6. **Batch large operations**: Prevent overwhelming your infrastructure

## Troubleshooting

**Command times out:**
```bash
# Increase timeout
kscorectl exec --timeout 5m "target" -- long-command

# Or use async
kscorectl exec --async "target" -- long-command
```

**Permission denied:**
```bash
# Check agent privileges
kscorectl agent show agent-001

# Use sudo (if configured)
kscorectl exec "target" -- sudo command
```

**No agents matched:**
```bash
# List all agents
kscorectl agent list

# Check targeting expression
kscorectl exec --dry-run "your-expression" -- echo test
```

## Next Steps

- [Deploy a Web Application](/docs/tutorials/deploy-web-app/) - Combine states with execution
- [Event-Driven Automation](/docs/tutorials/event-automation/) - Trigger commands from events
- [Remote Execution Reference](/docs/reference/cli/#kscore-exec) - Complete CLI reference
