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

### Agent Management

| Command | Description | Example |
|---------|-------------|---------|
| `kscorectl agents list` | List all agents | `kscorectl agents list -o json` |
| `kscorectl agents show <id>` | Show agent details | `kscorectl agents show web-01` |
| `kscorectl agents delete <id>` | Delete agent | `kscorectl agents delete web-01` |
| `kscorectl agents quarantine <id>` | Quarantine agent | `kscorectl agents quarantine web-01` |
| `kscorectl agents unquarantine <id>` | Remove quarantine | `kscorectl agents unquarantine web-01` |
| `kscorectl agents token create` | Create join token | `kscorectl agents token create --ttl 1h` |
| `kscorectl agents token list` | List join tokens | `kscorectl agents token list` |
| `kscorectl agents token revoke <id>` | Revoke join token | `kscorectl agents token revoke token-123` |
| `kscorectl agents status [id]` | Agent status summary | `kscorectl agents status` |
| `kscorectl agents tags set` | Replace agent tags | `kscorectl agents tags set web-01 role=web env=prod` |
| `kscorectl agents renew-svid <id>` | Renew SPIFFE SVID | `kscorectl agents renew-svid web-01 --force` |
| `kscorectl agents list --suspicious` | List suspicious agents | `kscorectl agents list --suspicious` |
| `kscorectl agents verify` | Verify agent integrity | `kscorectl agents verify --all` |
| `kscorectl agents verify --sample N` | Verify random sample | `kscorectl agents verify --sample 10` |
| `kscorectl agents certificates regenerate` | Regenerate agent certs | `kscorectl agents certificates regenerate --all` |
| `kscorectl agents re-enroll <id>` | Re-enroll with new credentials | `kscorectl agents re-enroll web-01 --reason "compromise"` |
| `kscorectl agents revoke-credentials <id>` | Revoke credentials (no new token) | `kscorectl agents revoke-credentials web-01 --reason "incident"` |

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

### Authentication

| Command | Description | Example |
|---------|-------------|---------|
| `kscorectl auth login` | Authenticate | `kscorectl auth login --username admin` |
| `kscorectl auth revoke-all` | Revoke all API keys | `kscorectl auth revoke-all --force` |
| `kscorectl auth sessions list` | List active sessions | `kscorectl auth sessions list` |
| `kscorectl auth sessions invalidate` | Invalidate sessions | `kscorectl auth sessions invalidate` |
| `kscorectl auth rotate-signing-key` | Rotate JWT key | `kscorectl auth rotate-signing-key --force` |
| `kscorectl auth key revoke <id>` | Revoke auth key | `kscorectl auth key revoke key-abc123` |

### Configuration

| Command | Description | Example |
|---------|-------------|---------|
| `kscorectl config validate` | Validate config file | `kscorectl config validate -c server.yaml` |
| `kscorectl config set <key> <val>` | Set runtime config | `kscorectl config set server.workers 16` |
| `kscorectl config show` | Show current config | `kscorectl config show --include-defaults` |

### Database

| Command | Description | Example |
|---------|-------------|---------|
| `kscorectl db compact` | Compact database | `kscorectl db compact --dry-run` |
| `kscorectl db rotate-credentials` | Rotate DB credentials | `kscorectl db rotate-credentials --force` |

### Diagnostics

| Command | Description | Example |
|---------|-------------|---------|
| `kscorectl diagnostics collect` | Collect diagnostics | `kscorectl diagnostics collect --include-logs --since 24h` |

### Security

| Command | Description | Example |
|---------|-------------|---------|
| `kscorectl security scan` | Quick security scan | `kscorectl security scan` |
| `kscorectl security scan --full` | Full security scan | `kscorectl security scan --full --output json` |

### NATS Management

| Command | Description | Example |
|---------|-------------|---------|
| `kscorectl nats rotate-credentials` | Rotate NATS credentials | `kscorectl nats rotate-credentials --force` |
| `kscorectl nats status` | NATS connection status | `kscorectl nats status --output json` |

---

## Remote Execution (kscore-exec)

### Command Execution

| Command | Description | Example |
|---------|-------------|---------|
| `exec run <target> -- <cmd>` | Execute command | `exec run "role:web" -- systemctl status nginx` |
| `exec run <cmd> --target <t>` | Execute (alt syntax) | `exec run uptime --target "os:linux"` |
| `exec async <target> -- <cmd>` | Async execution | `exec async "all" -- apt update` |
| `exec status <job-id>` | Check job status | `exec status job-abc123` |
| `exec list` | List batch jobs | `exec list --status completed` |
| `exec cancel <job-id>` | Cancel job | `exec cancel job-abc123 --force` |
| `exec history` | List recent jobs | `exec history --limit 20 --status completed` |
| `exec output <job-id>` | Get job output | `exec output job-abc123 --agent web-01` |
| `exec shell <target>` | Interactive shell | `exec shell "hostname:web-01" --user deploy` |
| `exec script <target> <file>` | Execute script | `exec script "role:web" deploy.sh` |
| `exec archive` | Archive old jobs | `exec archive --status completed --before 7d` |
| `exec export [job-id]` | Export results | `exec export abc123 --format json -o results.json` |
| `exec cleanup` | Remove old records | `exec cleanup --older-than 30d --status completed` |

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
--tls --tls-min-version 1.3  # Enable TLS with minimum version
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
| `module install <name>` | Install module | `module install stdlib/file@1.0.0` |
| `module update [name]` | Update dependencies | `module update stdlib/file` |

### Module Development

| Command | Description | Example |
|---------|-------------|---------|
| `module init <name>` | Create module | `module init myorg/mymodule` |
| `module build` | Build module | `module build` |
| `module test` | Test module | `module test` |
| `module publish` | Publish module | `module publish --registry reg.example.com` |
| `module verify <name>` | Verify signature | `module verify stdlib/file` |
| `module resolve` | Resolve dependencies | `module resolve` |
| `module tree` | Show dependency tree | `module tree --flat` |
| `module update` | Update dependencies | `module update --dry-run` |
| `module mirror` | Mirror for air-gapped | `module mirror vendor/pkg@1.0 --dest ./mirror` |
| `module clean` | Clean module cache | `module clean --all` |

Note: module archive entries larger than 256 MB are rejected during install.

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
| `test list` | List available test suites | `test list --type integration` |
| `test show <id>` | Show test run details | `test show test-123` |
| `test history` | Show test run history | `test history --suite basic --status failed` |
| `test suite list` | List suites | `test suite list --type e2e` |
| `test suite show <name>` | Show suite details | `test suite show core-agent` |
| `test suite create <name>` | Create test suite | `test suite create my-suite --type integration` |
| `test suite delete <name>` | Delete test suite | `test suite delete my-suite --force` |

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
| `policy check <file>` | Evaluate policy | `policy check policy.rego --input input.json` |
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

### Outbound Webhook Subscriptions

| Command | Description | Example |
|---------|-------------|---------|
| `webhook outbound list` | List outbound subscriptions | `webhook outbound list` |
| `webhook outbound create` | Create subscription | `webhook outbound create --name alerts --url https://example.com/hook --events "agent.*"` |
| `webhook outbound show <id>` | Show subscription details | `webhook outbound show sub_123` |
| `webhook outbound delete <id>` | Delete subscription | `webhook outbound delete sub_123` |
| `webhook outbound history <id>` | View delivery history | `webhook outbound history sub_123 --limit 20` |
| `webhook outbound test <id>` | Send test event | `webhook outbound test sub_123` |

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
| `cluster join <addr>` | Join cluster | `cluster join server1:2380 --token $TOKEN` |
| `cluster leave` | Leave cluster | `cluster leave --force` |
| `cluster rebalance` | Rebalance agents | `cluster rebalance --dry-run` |
| `cluster remove <id>` | Remove member | `cluster remove server-3` |
| `cluster member add <addr>` | Add member (alias) | `cluster member add server-4:9090` |
| `cluster member remove <id>` | Remove member (alias) | `cluster member remove server-3` |
| `cluster election restart` | Restart leader election | `cluster election restart` |

### Cluster Join Tokens

| Command | Description | Example |
|---------|-------------|---------|
| `cluster token generate` | Generate join token | `cluster token generate --ttl 1h --max-uses 3` |
| `cluster token list` | List join tokens | `cluster token list` |
| `cluster token revoke <id>` | Revoke join token | `cluster token revoke abc123` |

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
| `federation status` | Federation health summary | `federation status` |
| `federation trust list` | List trust relationships | `federation trust list` |
| `federation ping` | Test domain connectivity | `federation ping --region eu-west` |

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
| `bootstrap join` | Join existing cluster | `bootstrap join --server https://ks:8080 --token $TOKEN` |
| `bootstrap prereq-check` | Check prerequisites | `bootstrap prereq-check` |
| `bootstrap cert-gen` | Generate TLS certs | `bootstrap cert-gen --output /etc/keystone-core/certs/` |
| `bootstrap package create` | Create air-gapped package | `bootstrap package create --version 0.1.0 --platform linux/amd64` |
| `bootstrap package verify` | Verify package signatures | `bootstrap package verify package.tar.gz --trusted-key key.pub` |
| `bootstrap package install` | Install from package | `bootstrap package install package.tar.gz` |
| `bootstrap package inspect` | Inspect package manifest | `bootstrap package inspect package.tar.gz` |
| `bootstrap airgap-validate` | Validate air-gap compliance | `bootstrap airgap-validate --binary-dir /usr/local/bin --config-dir /etc/keystone-core` |

---

## Offline Registry (kscore-registry offline)

| Command | Description | Example |
|---------|-------------|---------|
| `registry offline init` | Initialize offline registry | `registry offline init --dir /opt/registry` |
| `registry offline list` | List modules/blueprints | `registry offline list --dir /opt/registry` |
| `registry offline search` | Search registry | `registry offline search --dir /opt/registry dns` |
| `registry offline import` | Import from package/dir | `registry offline import --dir /opt/registry ./mirror` |
| `registry offline verify` | Verify signatures | `registry offline verify --dir /opt/registry --trust-dir /etc/trust` |
| `registry offline gc` | Garbage collect old versions | `registry offline gc --dir /opt/registry --keep-versions 3` |
| `registry offline reindex` | Regenerate index | `registry offline reindex --dir /opt/registry` |
| `registry offline trust add` | Add trust root | `registry offline trust add --dir /opt/registry --name signer --key-file key.pub` |
| `registry offline trust remove` | Remove trust root | `registry offline trust remove --dir /opt/registry --name signer` |
| `registry offline trust list` | List trust roots | `registry offline trust list --dir /opt/registry` |

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
| `events retention list` | List retention policies | `events retention list` |
| `events retention set` | Set retention | `events retention set --max-age 30d` |
| `events dlq list` | List DLQ events | `events dlq list` |
| `events dlq retry` | Retry DLQ events | `events dlq retry --all` |
| `events dlq purge` | Purge DLQ | `events dlq purge --older-than 7d` |

---

## Runbook Management (kscore-runbook)

| Command | Description | Example |
|---------|-------------|---------|
| `runbook execute <name>` | Execute a runbook | `runbook execute deploy-service --input version=1.2.0` |
| `runbook execute --dry-run` | Preview without running | `runbook execute deploy-service --var version=1.2.0 --dry-run` |
| `runbook status <id>` | Check execution status | `runbook status exec-a1b2c3` |
| `runbook list-executions` | List execution history | `runbook list-executions --runbook deploy-service --since 7d` |
| `runbook approvals` | List approval requests | `runbook approvals --mine` |
| `runbook approve <id>` | Approve a request | `runbook approve req-123 --reason "Verified"` |
| `runbook reject <id>` | Reject a request | `runbook reject req-123 --reason "Not ready"` |
| `runbook interventions` | List interventions | `runbook interventions --state pending` |
| `runbook respond <id>` | Respond to intervention | `runbook respond int-123 --confirmed` |
| `runbook test <name>` | Validate a runbook | `runbook test deploy-service --mock-file mocks.json --verbose` |
| `runbook audit show <name>` | Show audit trail for a runbook | `runbook audit show deploy-service --limit 10` |
| `runbook audit list` | List audit events across runbooks | `runbook audit list --runbook deploy-service --start 7d` |
| `runbook audit report` | Generate compliance report | `runbook audit report --format csv --start 7d` |

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
| `upgrade check --from` | Check from version | `upgrade check --from 1.4.0 --target 1.6.0` |
| `upgrade plan` | Create upgrade plan | `upgrade plan --target 1.6.0` |
| `upgrade execute` | Execute upgrade | `upgrade execute --target 1.6.0 --confirm` |
| `upgrade status` | Upgrade status | `upgrade status` |
| `upgrade status --verbose` | Verbose status | `upgrade status --verbose` |
| `upgrade cancel` | Cancel upgrade | `upgrade cancel --rollback` |
| `upgrade rollback` | Rollback version | `upgrade rollback --target 1.5.2 --confirm` |
| `upgrade path` | Show upgrade path | `upgrade path --from 1.3.0 --target 2.0.0` |
| `upgrade resume` | Resume interrupted | `upgrade resume` |
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
| `upgrade agents` | Upgrade agents | `upgrade agents --target 1.6.0` |
| `upgrade agents --report` | Agent version report | `upgrade agents --report` |
| `upgrade agents --status` | Agent upgrade status | `upgrade agents --status` |
| `upgrade agents --retry` | Retry failed agent | `upgrade agents --retry agent-005` |
| `upgrade agents --skip` | Skip failed agent | `upgrade agents --skip agent-005` |

### Air-Gapped Upgrade Packages

| Command | Description | Example |
|---------|-------------|---------|
| `upgrade package create` | Create upgrade package | `upgrade package create --from 1.0.0 --to 1.1.0 --build-dir ./build` |
| `upgrade package verify` | Verify package integrity | `upgrade package verify upgrade.tar.gz --trusted-key release.pub` |
| `upgrade package inspect` | Inspect package contents | `upgrade package inspect upgrade.tar.gz` |
| `upgrade package apply` | Apply upgrade package | `upgrade package apply upgrade.tar.gz --install-dir /usr/local/bin --backup-dir /var/backup` |
| `upgrade package rollback` | Rollback from backup | `upgrade package rollback --backup-dir /var/backup/... --install-dir /usr/local/bin` |

### Air-Gapped Data Transfer

| Command | Description | Example |
|---------|-------------|---------|
| `transfer export` | Export operational data | `transfer export --type audit --since 24h -O audit.tar.gz` |
| `transfer import` | Import export package | `transfer import package.tar.gz --output-dir ./data` |
| `transfer verify` | Verify package integrity | `transfer verify package.tar.gz --verify-key release.pub` |
| `transfer sync list` | List sync windows | `transfer sync list` |
| `transfer sync show <name>` | Show sync window details | `transfer sync show daily-sync` |
| `transfer sync trigger <name>` | Trigger sync now | `transfer sync trigger daily-sync` |
| `transfer sync pause <name>` | Pause running sync | `transfer sync pause daily-sync` |
| `transfer sync resume <name>` | Resume paused sync | `transfer sync resume daily-sync` |
| `transfer sync cancel <name>` | Cancel running sync | `transfer sync cancel daily-sync` |
| `transfer sync history` | Sync execution history | `transfer sync history` |
| `transfer diode send` | Send file via data diode | `transfer diode send --address 10.0.0.2:9000 --file data.tar.gz` |
| `transfer diode receive` | Receive from data diode | `transfer diode receive --listen :9000 --output-dir ./received` |

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

Note: blueprint archive entries larger than 256 MB are rejected during install.

---

## Audit & Compliance

| Command | Description | Example |
|---------|-------------|---------|
| `audit query` | Query audit logs | `audit query --since 24h --type auth.*` |
| `audit export` | Export logs | `audit export --since 7d -o audit.csv` |
| `compliance report` | Generate report | `compliance report --framework soc2` |
| `audit search` | Search audit entries | `audit search --type "auth.*" --status "failed" --since "7d"` |
| `audit analyze` | Analyze for anomalies | `audit analyze --input "/tmp/*.json" --baseline "30d"` |
| `audit timeline` | Generate incident timeline | `audit timeline --from "..." --to "..." --output timeline.html` |
| `audit watch` | Real-time audit monitoring | `audit watch --type "auth.*" --status "failed"` |

---

## Secrets Management (kscore-secrets)

| Command | Description | Example |
|---------|-------------|---------|
| `secrets get <path>` | Retrieve secret value | `secrets get db/prod/password` |
| `secrets list [prefix]` | List secrets | `secrets list db/ --format json` |
| `secrets backends` | Show backend status | `secrets backends` |
| `secrets audit [path]` | Audit secret access | `secrets audit db/ --since 24h` |
| `secrets rotate list` | List rotation configs | `secrets rotate list` |
| `secrets rotate start <id>` | Start rotation | `secrets rotate start db-prod --force` |
| `secrets rotate status <id>` | Check rotation status | `secrets rotate status db-prod` |
| `secrets schedule list` | List rotation schedules | `secrets schedule list` |
| `secrets policy list` | List secrets policies | `secrets policy list` |
| `secrets dynamic list` | List dynamic secrets | `secrets dynamic list --backend vault` |
| `secrets encrypt` | Encrypt data | `secrets encrypt --keyring prod --input data.json` |
| `secrets decrypt` | Decrypt data | `secrets decrypt --keyring prod --input data.enc` |
| `secrets cache status` | Cache status | `secrets cache status` |
| `secrets rotate-keys` | Rotate encryption keys | `secrets rotate-keys --force` |

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
| `KSCORE_ALLOW_INSECURE_TLS` | Allow insecure TLS (dev only, required for `--tls-skip-verify`) |
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
kscorectl agents show web-05

# 2. Check recent commands
kscorectl exec history --target web-05 --limit 10

# 3. Check state applications
kscorectl state history --target web-05

# 4. Check events
kscorectl events list --source web-05 --since 1h
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

# 2. Check policy
kscorectl policy check require-labels.rego

# 3. Deploy in audit mode
kscorectl policy create require-labels.rego --mode audit

# 4. Review audit results
kscorectl policy audit --policy require-labels --since 7d

# 5. Enable enforcement
kscorectl policy activate require-labels --mode enforce
```

---

## Repository Tools

### Repository Generation (kscore-repo-gen)

| Command | Description | Example |
|---------|-------------|---------|
| `repo-gen all` | Generate all repos | `repo-gen all --version 0.1.0 --output build/repos` |
| `repo-gen dnf` | Generate DNF repo | `repo-gen dnf --version 0.1.0 --output build/repos/dnf` |
| `repo-gen apt` | Generate APT repo | `repo-gen apt --version 0.1.0 --output build/repos/apt` |
| `repo-gen windows` | Generate Windows repo | `repo-gen windows --version 0.1.0` |
| `repo-gen blueprints` | Generate blueprint registry | `repo-gen blueprints --output build/repos/blueprints` |
| `repo-gen modules` | Generate module registry | `repo-gen modules --output build/repos/modules` |

### Repository Mirroring (kscore-repo-mirror)

| Command | Description | Example |
|---------|-------------|---------|
| `repo-mirror` | Mirror all repos | `repo-mirror --output /mnt/usb/mirror` |
| `repo-mirror --only dnf,apt` | Mirror Linux only | `repo-mirror --only dnf,apt` |
| `repo-mirror --skip docs` | Skip documentation | `repo-mirror --skip docs,macos` |
| `repo-mirror --verbose` | Verbose output | `repo-mirror --verbose` |

**Repository Types** (for `--only` and `--skip`):

- `dnf`, `apt`, `windows`, `macos` - Package repositories
- `blueprints`, `modules` - Registries
- `docs` - Documentation

**Example: Air-Gapped Deployment**

```bash
# 1. Mirror repos on connected machine
kscore-repo-mirror --output /mnt/usb/kscore-mirror

# 2. Transfer to air-gapped environment

# 3. Serve via HTTP
cd /path/to/mirror && python3 -m http.server 8080

# 4. Configure DNF (RHEL/CentOS)
sudo cp keystonecore-local.repo /etc/yum.repos.d/

# 5. Install packages
sudo dnf install kscore-server kscore-agent
```

---

## MCP Server (kscore-mcp)

| Command | Description | Example |
|---------|-------------|---------|
| `kscore-mcp --config <path>` | Start MCP server | `kscore-mcp --config mcp.yaml` |
| `kscore-mcp validate --config <path>` | Validate config | `kscore-mcp validate --config mcp.yaml` |
| `kscore-mcp version` | Print version | `kscore-mcp version` |

---

## See Also

- [Full CLI Reference](../cli/) - Detailed command documentation
- [Configuration Reference](../configuration/) - Config file options
- [API Reference](../api/) - REST and gRPC APIs
