---
title: "CLI Quick Reference"
weight: 3
description: >
  Concise command cheat sheet for all Keystone Core CLI tools
---

## Command Cheat Sheet

This quick reference provides a consolidated view of all Keystone Core CLI commands. For detailed documentation, see the [full CLI Reference](../cli/).

---

## Core Commands (kscorectl)

### System

| Command | Description | Example |
|---------|-------------|---------|
| `kscorectl version` | Show version info | `kscorectl version --short` |
| `kscorectl help [cmd]` | Get help | `kscorectl help exec run` |
| `kscorectl completion <shell>` | Generate completions | `kscorectl completion bash` |
| `kscorectl config validate` | Validate config file | `kscorectl config validate -c server.yaml` |

### Agent Management

| Command | Description | Example |
|---------|-------------|---------|
| `kscorectl agent list` | List all agents | `kscorectl agent list -o json` |
| `kscorectl agent show <id>` | Show agent details | `kscorectl agent show web-01` |
| `kscorectl agent delete <id>` | Delete agent | `kscorectl agent delete web-01` |
| `kscorectl agent quarantine <id>` | Quarantine agent | `kscorectl agent quarantine web-01` |
| `kscorectl agent unquarantine <id>` | Remove quarantine | `kscorectl agent unquarantine web-01` |
| `kscorectl agent token create` | Create join token | `kscorectl agent token create --ttl 1h` |
| `kscorectl agent token list` | List join tokens | `kscorectl agent token list` |
| `kscorectl agent token revoke <id>` | Revoke join token | `kscorectl agent token revoke token-123` |
| `kscorectl agent status [id]` | Agent status summary | `kscorectl agent status` |
| `kscorectl agent tags set` | Replace agent tags | `kscorectl agent tags set web-01 role=web env=prod` |
| `kscorectl agent renew-svid <id>` | Renew SPIFFE SVID | `kscorectl agent renew-svid web-01 --force` |

### API Key Management

| Command | Description | Example |
|---------|-------------|---------|
| `kscorectl api-key create` | Create API key | `kscorectl api-key create --name mon --role readonly` |
| `kscorectl api-key list` | List API keys | `kscorectl api-key list` |
| `kscorectl api-key revoke <id>` | Revoke API key | `kscorectl api-key revoke ak_123` |

### Health & Status

| Command | Description | Example |
|---------|-------------|---------|
| `kscorectl health` | Check system health | `kscorectl health` |
| `kscorectl health check` | Full health check | `kscorectl health check --full` |

---

## Remote Execution (kscore-exec)

### Command Execution

| Command | Description | Example |
|---------|-------------|---------|
| `exec run <target> -- <cmd>` | Execute command | `exec run "role:web" -- systemctl status nginx` |
| `exec run <cmd> --target <t>` | Execute (alt syntax) | `exec run uptime --target "os:linux"` |
| `exec async <target> -- <cmd>` | Async execution | `exec async "all" -- apt update` |
| `exec status <job-id>` | Check job status | `exec status job-abc123` |
| `exec cancel <job-id>` | Cancel job | `exec cancel job-abc123` |
| `exec history` | List recent jobs | `exec history --limit 20` |
| `exec output <job-id>` | Get job output | `exec output job-abc123 --agent web-01` |

### Target Expression Syntax

```
role:web                    # Label match
os:linux                    # OS match
arch:amd64                  # Architecture
hostname:web-*              # Hostname glob
status:agent_status_online  # Status filter
env:prod and role:web       # Logical AND
role:web or role:api        # Logical OR
not role:db                 # Negation
(os:linux and role:web)     # Grouping
```

### Common Execution Flags

```
--concurrency 10          # Parallel execution limit
--command-timeout 300     # Command timeout (seconds)
--continue-on-failure     # Don't stop on failures
--env KEY=VALUE           # Set environment variable
--user root               # Run as user
--working-dir /app        # Working directory
```

---

## State Management (kscore-state)

### State Operations

| Command | Description | Example |
|---------|-------------|---------|
| `state apply <file>` | Apply state file | `state apply web.yaml --target "role:web"` |
| `state check <file>` | Check state file | `state check web.yaml` |
| `state test <file>` | Test state (dry run) | `state test web.yaml --target web-01` |
| `state diff <file>` | Show changes | `state diff web.yaml --target web-01` |
| `state show <file>` | Show rendered state | `state show web.yaml` |

### Drift Detection

| Command | Description | Example |
|---------|-------------|---------|
| `state drift [file]` | Check for drift | `state drift web.yaml` |
| `state drift --all` | Check all states | `state drift --all` |
| `state drift --remediate` | Fix drift | `state drift web.yaml --remediate` |

### State History

| Command | Description | Example |
|---------|-------------|---------|
| `state history` | List applications | `state history --limit 20` |
| `state history <id>` | Show application | `state history app-123` |
| `state rollback <id>` | Rollback state | `state rollback app-123` |

### Common State Flags

```
--target "role:web"       # Target expression
--concurrency 5           # Parallel apply limit
--timeout 30m             # Apply timeout
--preview                 # Show what would change
--force                   # Skip confirmation
--no-deps                 # Ignore dependencies
```

---

## Monitoring (kscore-monitor)

### TUI Commands

| Command | Description | Example |
|---------|-------------|---------|
| `monitor` | Launch TUI | `kscorectl monitor` |
| `monitor --view agents` | Specific view | `monitor --view events` |

### TUI Views

| Key | View |
|-----|------|
| `1` | Overview dashboard |
| `2` | Agent list |
| `3` | Event stream |
| `4` | Job status |
| `5` | Metrics |
| `q` | Quit |
| `?` | Help |

---

## Module Management (kscore-module)

### Module Operations

| Command | Description | Example |
|---------|-------------|---------|
| `module list` | List modules | `module list --installed` |
| `module install <name>` | Install module | `module install stdlib/file@1.0.0` |
| `module uninstall <name>` | Remove module | `module uninstall stdlib/file` |
| `module update <name>` | Update module | `module update stdlib/file` |
| `module show <name>` | Show details | `module show stdlib/file` |

### Module Development

| Command | Description | Example |
|---------|-------------|---------|
| `module init <name>` | Create module | `module init myorg/mymodule` |
| `module build` | Build module | `module build` |
| `module test` | Test module | `module test` |
| `module publish` | Publish module | `module publish --registry reg.example.com` |
| `module verify <name>` | Verify signature | `module verify stdlib/file` |

---

## Load Testing (kscore-loadtest)

| Command | Description | Example |
|---------|-------------|---------|
| `loadtest run` | Run load test | `loadtest run --agents 100 --scenario registration` |
| `loadtest scenarios` | List scenarios | `loadtest scenarios` |
| `loadtest report` | Show report | `loadtest report --file reports/loadtest/results.json` |

---

## Test Runner (kscore-test)

| Command | Description | Example |
|---------|-------------|---------|
| `test smoke` | Run smoke tests | `test smoke --target "role:web"` |
| `test integration` | Run integration suites | `test integration --suite recovery` |
| `test run` | Run suite | `test run --suite e2e --timeout 1h` |
| `test list` | List test runs | `test list` |
| `test show <id>` | Show test run | `test show test-123` |
| `test suite list` | List suites | `test suite list` |

---

## Policy Management (kscore-policy)

### Policy Operations

| Command | Description | Example |
|---------|-------------|---------|
| `policy list` | List policies | `policy list --active` |
| `policy show <name>` | Show policy | `policy show require-labels` |
| `policy create <file>` | Create policy | `policy create policy.rego --name mypolicy` |
| `policy update <name>` | Update policy | `policy update mypolicy --file policy.rego` |
| `policy delete <name>` | Delete policy | `policy delete mypolicy` |
| `policy activate <name>` | Enable policy | `policy activate mypolicy` |
| `policy deactivate <name>` | Disable policy | `policy deactivate mypolicy` |

### Policy Testing

| Command | Description | Example |
|---------|-------------|---------|
| `policy evaluate` | Test evaluation | `policy evaluate --input input.json` |
| `policy test <file>` | Run policy tests | `policy test policy_test.rego` |
| `policy validate <file>` | Validate syntax | `policy validate policy.rego` |

### Policy Audit

| Command | Description | Example |
|---------|-------------|---------|
| `policy audit` | List evaluations | `policy audit --since 24h` |
| `policy audit <id>` | Show evaluation | `policy audit eval-123` |
| `policy compliance` | Compliance report | `policy compliance --format html` |

---

## GitOps (kscore-gitops)

### Repository Management

| Command | Description | Example |
|---------|-------------|---------|
| `gitops repo list` | List repositories | `gitops repo list` |
| `gitops repo add <url>` | Add repository | `gitops repo add https://github.com/org/repo` |
| `gitops repo remove <name>` | Remove repository | `gitops repo remove myrepo` |
| `gitops repo sync <name>` | Sync repository | `gitops repo sync myrepo` |

### Deployment Management

| Command | Description | Example |
|---------|-------------|---------|
| `gitops deploy list` | List deployments | `gitops deploy list` |
| `gitops deploy show <id>` | Show deployment | `gitops deploy show dep-123` |
| `gitops deploy rollback <id>` | Rollback | `gitops deploy rollback dep-123` |
| `gitops deploy approve <id>` | Approve deployment | `gitops deploy approve dep-123` |

### Webhook Management

| Command | Description | Example |
|---------|-------------|---------|
| `gitops webhook status` | Webhook status | `gitops webhook status` |
| `gitops webhook events` | Recent events | `gitops webhook events --since 1h` |

---

## Cluster Management (kscore-cluster)

### Cluster Status

| Command | Description | Example |
|---------|-------------|---------|
| `cluster status` | Show status | `cluster status` |
| `cluster members` | List members | `cluster members` |
| `cluster leader` | Show leader | `cluster leader` |
| `cluster shards` | Show shards | `cluster shards` |

### Cluster Operations

| Command | Description | Example |
|---------|-------------|---------|
| `cluster join <addr>` | Join cluster | `cluster join server1:2380` |
| `cluster leave` | Leave cluster | `cluster leave --force` |
| `cluster rebalance` | Rebalance agents | `cluster rebalance --dry-run` |
| `cluster remove <id>` | Remove member | `cluster remove server-3` |

### Backup & Restore

| Command | Description | Example |
|---------|-------------|---------|
| `cluster backup` | Create backup | `cluster backup -o backup.json` |
| `cluster restore <file>` | Restore backup | `cluster restore backup.json` |

---

## Identity Management (kscore-identity)

### Identity Status

| Command | Description | Example |
|---------|-------------|---------|
| `identity status` | Provider status | `identity status` |
| `identity svid list` | List SVIDs | `identity svid list` |
| `identity svid show <id>` | Show SVID | `identity svid show agent-web-01` |

### Token Management

| Command | Description | Example |
|---------|-------------|---------|
| `identity token create` | Create token | `identity token create --ttl 1h` |
| `identity token list` | List tokens | `identity token list` |
| `identity token revoke <id>` | Revoke token | `identity token revoke tok-123` |

### CA Management

| Command | Description | Example |
|---------|-------------|---------|
| `identity ca status` | CA status | `identity ca status` |
| `identity ca rotate` | Rotate CA | `identity ca rotate --graceful` |
| `identity ca export` | Export CA cert | `identity ca export --format pem` |

### Federation

| Command | Description | Example |
|---------|-------------|---------|
| `federation wizard` | Interactive setup wizard | `federation wizard` |
| `federation wizard --non-interactive` | Scripted setup | `federation wizard --non-interactive --domain partner.example.org` |
| `federation list` | List federations | `federation list` |
| `federation add` | Add federation | `federation add partner.example.org --bundle-endpoint URL` |
| `federation show` | Show federation details | `federation show partner.example.org` |
| `federation suspend` | Suspend federation | `federation suspend partner.example.org` |
| `federation activate` | Activate federation | `federation activate partner.example.org` |
| `federation remove` | Remove federation | `federation remove partner.example.org` |
| `federation refresh` | Refresh trust bundle | `federation refresh partner.example.org` |

---

## Database Migration (kscore-migrate)

| Command | Description | Example |
|---------|-------------|---------|
| `migrate status` | Migration status | `migrate status` |
| `migrate up` | Run migrations | `migrate up` |
| `migrate down <n>` | Rollback N | `migrate down 1` |
| `migrate version` | Show version | `migrate version` |

---

## Bootstrap (kscore-bootstrap)

| Command | Description | Example |
|---------|-------------|---------|
| `bootstrap seed` | Initialize cluster | `bootstrap seed --config seed.yaml` |
| `bootstrap restore` | Restore from backup | `bootstrap restore --backup-path backup.tar.gz` |
| `bootstrap import` | Import configuration | `bootstrap import --source /exports/config.yaml` |
| `bootstrap status` | Bootstrap status | `bootstrap status` |
| `bootstrap validate` | Validate config | `bootstrap validate --config seed.yaml` |
| `bootstrap cleanup` | Clean up artifacts | `bootstrap cleanup --force` |

---

## Agent Bootstrap (kscore-agent)

| Command | Description | Example |
|---------|-------------|---------|
| `kscore-agent bootstrap` | Guided bootstrap flow | `kscore-agent bootstrap --mode production --cluster-name prod` |

---

## File Distribution (kscore-files)

| Command | Description | Example |
|---------|-------------|---------|
| `file upload <path>` | Upload file | `file upload /path/to/file` |
| `file download <id>` | Download file | `file download file-123 -o /tmp/` |
| `file list` | List files | `file list` |
| `file delete <id>` | Delete file | `file delete file-123` |
| `file status` | Server status | `file status` |

---

## Backup Management (kscore-backup)

### Backup Operations

| Command | Description | Example |
|---------|-------------|---------|
| `backup create` | Create backup | `backup create --type full --encrypt` |
| `backup list` | List backups | `backup list --last 24h` |
| `backup show <id>` | Show backup | `backup show backup-20240115-060000` |
| `backup verify <id>` | Verify backup | `backup verify backup-20240115-060000` |
| `backup restore <id>` | Restore backup | `backup restore backup-20240115-060000 --dry-run` |
| `backup delete <id>` | Delete backup | `backup delete backup-20240115-060000 --force` |

### Backup Types

```
--type full          # All components
--type incremental   # Changes since last backup
--type database      # Database only
--type configuration # Config files only
--type jetstream     # NATS JetStream data
--type etcd          # etcd cluster data
--type secrets       # Secrets and credentials
```

### Compression Options

```
--compression none   # No compression
--compression gzip   # gzip (default, good balance)
--compression bzip2  # bzip2 (higher ratio, slower)
--compression xz     # xz (highest ratio, slowest)
--compression zstd   # Zstandard (fast, good ratio - recommended)
--compression lz4    # LZ4 (fastest, lower ratio)
--compression-level 6  # Compression level (algorithm-specific)
```

### Rclone Cloud Storage

Backup to 50+ cloud providers via rclone (Dropbox, Google Drive, OneDrive, Backblaze B2, etc.):

```bash
# Backup to Dropbox (streaming, no temp files)
backup create --type full --rclone-remote dropbox --rclone-path /backups

# Backup to Google Drive
backup create --type full --rclone-remote gdrive --rclone-path backups/kscore

# Backup to Backblaze B2
backup create --type full --rclone-remote b2 --rclone-path bucket/backups
```

Configure remotes first with `rclone config`.

### Backup Schedules & Retention

| Command | Description | Example |
|---------|-------------|---------|
| `backup schedule list` | List schedules | `backup schedule list` |
| `backup schedule create` | Create schedule | `backup schedule create daily --schedule "0 6 * * *"` |
| `backup retention show` | Show policies | `backup retention show` |
| `backup retention apply` | Apply policies | `backup retention apply --dry-run` |
| `backup replication-status` | Replication status | `backup replication-status` |

---

## Event Management (kscore-events)

### Event Operations

| Command | Description | Example |
|---------|-------------|---------|
| `events list` | List events | `events list --type "agent.*" --since 1h` |
| `events query <expr>` | Query with CEL | `events query 'severity == "error"'` |
| `events emit` | Emit custom event | `events emit --type custom.deploy --data '{"v":"1.0"}'` |
| `events watch` | Watch real-time | `events watch --type "state.*"` |
| `events replay` | Replay events | `events replay --type "agent.failed" --dry-run` |

### Event Retention & DLQ

| Command | Description | Example |
|---------|-------------|---------|
| `events retention show` | Show retention | `events retention show` |
| `events retention set` | Set retention | `events retention set --max-age 30d` |
| `events dlq list` | List DLQ events | `events dlq list` |
| `events dlq retry` | Retry DLQ events | `events dlq retry --all` |
| `events dlq purge` | Purge DLQ | `events dlq purge --older-than 7d` |

---

## Schedule & Maintenance (kscore-schedule)

### Schedule Operations

| Command | Description | Example |
|---------|-------------|---------|
| `schedule list` | List schedules | `schedule list --status active` |
| `schedule show <id>` | Show schedule | `schedule show sched-001` |
| `schedule create` | Create schedule | `schedule create --name daily-backup --cron "0 2 * * *"` |
| `schedule trigger <id>` | Trigger now | `schedule trigger sched-001` |
| `schedule pause <id>` | Pause schedule | `schedule pause sched-001` |
| `schedule resume <id>` | Resume schedule | `schedule resume sched-001` |
| `schedule delete <id>` | Delete schedule | `schedule delete sched-001 --force` |
| `schedule history <id>` | Show history | `schedule history sched-001` |

### Maintenance Windows

| Command | Description | Example |
|---------|-------------|---------|
| `maintenance list` | List windows | `maintenance list --status active` |
| `maintenance create` | Create window | `maintenance create --name patch --start "..." --end "..."` |
| `maintenance start <id>` | Start window | `maintenance start maint-001` |
| `maintenance end <id>` | End window | `maintenance end maint-001` |
| `maintenance extend <id>` | Extend window | `maintenance extend maint-001 --duration 2h` |
| `maintenance active` | Active windows | `maintenance active` |
| `maintenance upcoming` | Upcoming windows | `maintenance upcoming --within 24h` |

---

## Upgrade Management (kscore-upgrade)

### Upgrade Operations

| Command | Description | Example |
|---------|-------------|---------|
| `upgrade check` | Check for upgrades | `upgrade check` |
| `upgrade plan` | Create upgrade plan | `upgrade plan --target 1.6.0` |
| `upgrade execute` | Execute upgrade | `upgrade execute --target 1.6.0 --confirm` |
| `upgrade status` | Upgrade status | `upgrade status` |
| `upgrade cancel` | Cancel upgrade | `upgrade cancel --rollback` |
| `upgrade rollback` | Rollback version | `upgrade rollback --target 1.5.2 --confirm` |
| `upgrade history` | Upgrade history | `upgrade history --limit 10` |
| `upgrade logs` | Upgrade logs | `upgrade logs --follow` |

### Canary Deployments

| Command | Description | Example |
|---------|-------------|---------|
| `upgrade canary status` | Canary status | `upgrade canary status` |
| `upgrade canary promote` | Promote canary | `upgrade canary promote --confirm` |
| `upgrade canary rollback` | Rollback canary | `upgrade canary rollback --confirm` |

### Agent Upgrades

| Command | Description | Example |
|---------|-------------|---------|
| `upgrade agents list` | List agent versions | `upgrade agents list --outdated` |
| `upgrade agents status <id>` | Agent upgrade status | `upgrade agents status web-01` |

---

## Proxy Agent Management (kscore-proxy)

### Proxy Status

| Command | Description | Example |
|---------|-------------|---------|
| `proxy status` | Overall status | `proxy status` |

### Device Management

| Command | Description | Example |
|---------|-------------|---------|
| `proxy device list` | List devices | `proxy device list --protocol ssh` |
| `proxy device show <id>` | Show device | `proxy device show router-01` |
| `proxy device add` | Add device | `proxy device add --name sw-01 --address 10.0.0.1 --protocol ssh` |
| `proxy device remove <id>` | Remove device | `proxy device remove sw-01 --force` |
| `proxy device test <id>` | Test connectivity | `proxy device test sw-01` |

### Credential Management

| Command | Description | Example |
|---------|-------------|---------|
| `proxy credential list` | List credentials | `proxy credential list` |
| `proxy credential add` | Add credential | `proxy credential add --name ssh-admin --type ssh-key` |
| `proxy credential remove` | Remove credential | `proxy credential remove ssh-admin` |
| `proxy credential update` | Update credential | `proxy credential update ssh-admin` |

### Discovery & Drift

| Command | Description | Example |
|---------|-------------|---------|
| `proxy discover scan` | Scan for devices | `proxy discover scan --subnet 192.168.1.0/24` |
| `proxy discover list` | List discovered | `proxy discover list --status pending` |
| `proxy discover approve` | Approve device | `proxy discover approve disc-123` |
| `proxy drift check` | Check for drift | `proxy drift check --device router-01` |
| `proxy drift show <id>` | Show drift details | `proxy drift show router-01` |

### State Operations

| Command | Description | Example |
|---------|-------------|---------|
| `proxy state apply <file>` | Apply state | `proxy state apply config.yaml --device sw-01` |
| `proxy state check <file>` | Check state | `proxy state check config.yaml --device sw-01` |

---

## Blueprint Management

| Command | Description | Example |
|---------|-------------|---------|
| `blueprint list` | List blueprints | `blueprint list` |
| `blueprint show <name>` | Show blueprint | `blueprint show production-cluster` |
| `blueprint deploy <name>` | Deploy blueprint | `blueprint deploy production-cluster --param size=3` |
| `blueprint status <id>` | Deploy status | `blueprint status dep-123` |
| `blueprint rollback <id>` | Rollback | `blueprint rollback dep-123` |
| `blueprint validate <name>` | Validate params | `blueprint validate production-cluster` |

---

## Audit & Compliance

| Command | Description | Example |
|---------|-------------|---------|
| `audit query` | Query audit logs | `audit query --since 24h --type auth.*` |
| `audit export` | Export logs | `audit export --since 7d -o audit.csv` |
| `compliance report` | Generate report | `compliance report --framework soc2` |

---

## Output Format Options

Most commands support output formatting:

```bash
--output text    # Human-readable (default)
--output json    # JSON format
--output yaml    # YAML format
--output table   # Tabular format
--output wide    # Extended table columns
```

---

## Environment Variables

| Variable | Description |
|----------|-------------|
| `KSCORE_SERVER` | Control plane URL |
| `KSCORE_API_KEY` | API key for auth |
| `KSCORE_CONFIG` | Config file path |
| `KSCORE_LOG_LEVEL` | Log level (debug, info, warn, error) |
| `KSCORE_NO_COLOR` | Disable colored output |
| `KSCORE_ALLOW_INSECURE_TLS` | Allow insecure TLS (dev only) |
| `KSCORE_BLUEPRINT_REGISTRY` | Blueprint registry URL |
| `KSCORE_REGISTRY` | Module registry URL |
| `KSCORE_REGISTRY_TOKEN` | Module registry auth token |
| `KSCORE_REGISTRY_USERNAME` | Module registry basic auth username |
| `KSCORE_REGISTRY_PASSWORD` | Module registry basic auth password |
| `KSCORE_CACHE_DIR` | Module cache directory |

---

## Shell Completion Setup

### Bash

```bash
# Add to ~/.bashrc
source <(kscorectl completion bash)

# Or install system-wide
kscorectl completion bash | sudo tee /etc/bash_completion.d/kscorectl
```

### Zsh

```zsh
# Add to ~/.zshrc
source <(kscorectl completion zsh)

# Or add to fpath
kscorectl completion zsh > "${fpath[1]}/_kscorectl"
```

### Fish

```fish
kscorectl completion fish > ~/.config/fish/completions/kscorectl.fish
```

### PowerShell

```powershell
# Add to $PROFILE
kscorectl completion powershell | Out-String | Invoke-Expression

# Or save to file
kscorectl completion powershell > kscorectl.ps1
```

---

## Common Workflows

### Deploy Configuration to Web Servers

```bash
# 1. Check current state
kscorectl state diff nginx.yaml --target "role:web"

# 2. Apply state
kscorectl state apply nginx.yaml --target "role:web"

# 3. Verify
kscorectl exec run "role:web" -- systemctl status nginx
```

### Investigate Failed Agent

```bash
# 1. Check agent status
kscorectl agent show web-05

# 2. Check recent commands
kscorectl exec history --target web-05 --limit 10

# 3. Check state applications
kscorectl state history --target web-05

# 4. Check events
kscorectl event list --agent web-05 --since 1h
```

### Cluster Maintenance

```bash
# 1. Check cluster health
kscorectl cluster status

# 2. Drain member before maintenance
kscorectl cluster drain server-2

# 3. Perform maintenance...

# 4. Rejoin cluster
kscorectl cluster undrain server-2

# 5. Rebalance if needed
kscorectl cluster rebalance
```

### Policy Development

```bash
# 1. Create policy
cat > require-labels.rego << 'EOF'
package kscore.policy
allow { input.resource.labels.owner }
EOF

# 2. Test policy
kscorectl policy test require-labels.rego

# 3. Deploy in audit mode
kscorectl policy create require-labels.rego --mode audit

# 4. Review audit results
kscorectl policy audit --policy require-labels --since 7d

# 5. Enable enforcement
kscorectl policy activate require-labels --mode enforce
```

---

## See Also

- [Full CLI Reference](../cli/) - Detailed command documentation
- [Configuration Reference](../configuration/) - Config file options
- [API Reference](../api/) - REST and gRPC APIs
