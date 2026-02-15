---
title: "CLI Reference"
weight: 2
description: >
  Complete command-line interface reference for kscorectl and all plugins
---

## Overview

Keystone Core uses a Git-style plugin architecture for its CLI. The main command is `kscorectl`, which discovers and executes `kscore-*` plugin binaries.

**Main CLI**: `kscorectl` (dispatcher)
**Plugins**: `kscore-*` (discovered from $PATH)

### CLI Tools Summary

| Tool | Type | Description |
|------|------|-------------|
| `kscorectl` | Dispatcher | Main CLI that routes to plugins |
| `kscore-exec` | Plugin | Remote command execution |
| `kscore-state` | Plugin | Declarative state management |
| `kscore-monitor` | Plugin | Real-time TUI monitoring |
| `kscore-agents` | Plugin | Agent management (list, tokens, tags) |
| `kscore-module` | Plugin | Module management and development |
| `kscore-blueprint` | Plugin | Blueprint lifecycle management |
| `kscore-blueprint-publish` | Plugin | Blueprint publishing and signing |
| `kscore-blueprint-state` | Plugin | Blueprint rollback and snapshot tooling |
| `kscore-policy` | Plugin | Policy evaluation and compliance |
| `kscore-audit` | Plugin | Policy audit, reporting, and exports |
| `kscore-gitops` | Plugin | GitOps integration and verification |
| `kscore-webhook` | Plugin | Webhook handler management |
| `kscore-cluster` | Plugin | Cluster management and HA |
| `kscore-cluster-backup` | Plugin | Cluster backup and restore automation |
| `kscore-identity` | Plugin | SPIFFE identity management |
| `kscore-federation` | Plugin | Trust federation management |
| `kscore-migrate` | Plugin | Database migration tool |
| `kscore-registry` | Server | Module registry HTTP server |
| `kscore-files` | Server | File distribution server |
| `kscore-files-storage` | Plugin | File backend and mirror administration |
| `kscore-bootstrap` | Tool | Cluster bootstrapping and recovery |
| `kscore-telemetry-gateway` | Server | Telemetry aggregation gateway |
| `kscore-agent` | Daemon | Agent daemon on managed nodes |
| `kscore-server` | Daemon | Control plane server |
| `kscore-loadtest` | Tool | Load testing harness |
| `kscore-test` | Tool | Test runner for smoke/integration/e2e |
| `kscore-repo-gen` | Tool | Generate distribution repositories |
| `kscore-repo-mirror` | Tool | Mirror repositories for air-gapped deployments |

## kscorectl (Main CLI)

The kscorectl command dispatches to plugins and provides core functionality.

### Global Flags

Available for all commands:

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--server` | `-s` | Control plane server address | `localhost:9090` |
| `--config` | `-c` | Config file path | standard search paths |
| `--format` | `-o` | Output format (table, json, yaml) | `table` |
| `--verbose` | `-v` | Enable verbose output | `false` |
| `--quiet` | `-q` | Suppress non-essential output | `false` |
| `--timeout` | | Request timeout duration | `30s` |

> **Note**: `--verbose` and `--quiet` are mutually exclusive.

### kscorectl version

Display version information.

```bash
kscorectl version
```

**Output**:

```
Keystone Core v1.0.0
  Build: abc123
  Go version: go1.21.5
  OS/Arch: linux/amd64
```

**Flags**:

- `--short`: Show only version number
- `--verbose`: Show detailed version information including all dependencies in JSON format

### kscorectl help

Display help information.

```bash
kscorectl help [command]
```

**Examples**:

```bash
kscorectl help              # Show all commands
kscorectl help exec         # Show exec plugin help
kscorectl help state apply  # Show state apply help
```

### kscorectl completion

Generate shell completion scripts.

```bash
kscorectl completion [bash|zsh|fish|powershell]
```

**Examples**:

```bash
# Bash
kscorectl completion bash > /etc/bash_completion.d/kscorectl

# Zsh
kscorectl completion zsh > "${fpath[1]}/_kscorectl"

# Fish
kscorectl completion fish > ~/.config/fish/completions/kscorectl.fish
```

### kscorectl config validate

Validate a configuration file against schema rules.

```bash
kscorectl config validate --config /etc/keystone-core/server.yaml
```

### kscorectl health

Check the health of the Keystone Core control plane.

```bash
# Basic health check
kscorectl health

# Full health check with all components
kscorectl health --full

# Full health check (subcommand form)
kscorectl health check
```

**Flags**:

- `--full`: Perform a full health check of all components

### kscorectl api-key

Manage API keys for authenticating with the control plane.

#### api-key create

Create a new API key.

```bash
kscorectl api-key create --name "ci-pipeline" --role operator --expires-in 90d
```

**Flags**:

- `--name string`: Name for the API key (required)
- `--role string`: Role for the API key (admin, operator, readonly) (default: readonly)
- `--expires-in string`: Expiration time (e.g., 30d, 1y) (default: 365d)

#### api-key list

List all API keys.

```bash
kscorectl api-key list
kscorectl api-key list -o json
```

**Flags**:

- `-o, --output string`: Output format (table, json) (default: table)

#### api-key revoke

Revoke an API key to prevent further use.

```bash
kscorectl api-key revoke <key-id>
```

**Arguments**:

- `<key-id>`: ID of the API key to revoke (required)

### kscorectl benchmark

Run performance benchmarks against the Keystone Core control plane.

```bash
# Run quick benchmark summary
kscorectl benchmark

# Run all benchmarks
kscorectl benchmark all

# Run specific benchmarks
kscorectl benchmark agent-registration --count 1000 --parallel 50
kscorectl benchmark command-execution --count 10000 --parallel 100
kscorectl benchmark state-apply --state test.yaml --targets 100

# Compare benchmark results
kscorectl benchmark compare baseline.json results.json --threshold 10%

# Output as JSON
kscorectl benchmark all --output json
```

**Flags**:

- `-o, --output string`: Output format (text, json) (default: text)
- `--duration string`: Duration to run benchmarks (e.g., 30s, 1m, 5m) (default: 1m)
- `--report`: Generate detailed benchmark report

**Subcommands**:

- `all`: Run all available benchmarks
- `agent-registration`: Benchmark agent registration performance
- `command-execution`: Benchmark command execution throughput
- `state-apply`: Benchmark state application performance
- `compare`: Compare two benchmark result files

### kscorectl maintenance

Manage maintenance mode for the Keystone Core control plane. In maintenance mode:

- No new agent connections are accepted
- New commands/state applications are queued
- Event processing continues (for monitoring)
- Existing agent connections are maintained

Use maintenance mode during scheduled upgrades, database maintenance, configuration changes, or disaster recovery procedures.

#### maintenance enable

Enable maintenance mode on the control plane.

```bash
# Enable maintenance mode
kscorectl maintenance enable

# Enable with reason and expected duration
kscorectl maintenance enable --reason "Scheduled database upgrade" --duration 2h

# Force enable without confirmation prompt
kscorectl maintenance enable --force
```

**Flags**:

- `--reason string`: Reason for entering maintenance mode
- `--duration string`: Expected duration (e.g., 30m, 2h)
- `-f, --force`: Skip confirmation prompt

#### maintenance disable

Disable maintenance mode and resume normal operations. Queued commands will begin executing after maintenance mode is disabled.

```bash
kscorectl maintenance disable
```

**Flags**:

- `-f, --force`: Skip confirmation prompt

#### maintenance status

Show the current maintenance mode status.

```bash
# Check maintenance mode status
kscorectl maintenance status

# Output as JSON
kscorectl maintenance status -o json
```

**Flags**:

- `-o, --output string`: Output format (text, json) (default: text)

#### maintenance queue

View and manage commands queued during maintenance mode.

```bash
# List queued commands
kscorectl maintenance queue

# Show queue status summary
kscorectl maintenance queue --status
```

**Flags**:

- `--status`: Show queue status summary instead of listing items

#### maintenance cleanup

Clean up old maintenance mode logs and queue data.

```bash
# Clean up data older than 30 days
kscorectl maintenance cleanup --older-than 30d

# Dry run - show what would be deleted
kscorectl maintenance cleanup --older-than 30d --dry-run
```

**Flags**:

- `--older-than string`: Delete data older than this duration (e.g., 7d, 30d) (default: 30d)
- `--dry-run`: Show what would be deleted without actually deleting

### kscorectl auth

Authentication and session management.

#### auth login

Authenticate with the control plane.

```bash
kscorectl auth login --username admin
kscorectl auth login --api-key kscore_xxxx
```

**Flags**:

- `--username string`: Username for authentication
- `--api-key string`: API key for authentication

#### auth revoke-all

Revoke all active API keys. This is a security incident response action.

```bash
kscorectl auth revoke-all --force
```

**Flags**:

- `--force, -f`: Skip confirmation prompt

#### auth sessions

Manage active authentication sessions.

```bash
# List active sessions
kscorectl auth sessions list

# Invalidate all sessions
kscorectl auth sessions invalidate
```

#### auth rotate-signing-key

Rotate the JWT signing key. Existing tokens remain valid until expiry.

```bash
kscorectl auth rotate-signing-key
kscorectl auth rotate-signing-key --force
```

**Flags**:

- `--force, -f`: Skip confirmation prompt

#### auth key revoke

Revoke a specific authentication key.

```bash
kscorectl auth key revoke <key-id>
```

### kscorectl config set

Set a runtime configuration value on the control plane using dot notation.

```bash
kscorectl config set server.workers 16
kscorectl config set database.sqlite.cache_size 10000
kscorectl config set nats.consumer_workers 8
```

### kscorectl config show

Display the current runtime configuration.

```bash
kscorectl config show
kscorectl config show --include-defaults
```

**Flags**:

- `--include-defaults`: Include default values in output

### kscorectl db

Database maintenance operations.

#### db compact

Compact the database to reclaim unused space. Runs VACUUM for SQLite, VACUUM ANALYZE for PostgreSQL.

```bash
kscorectl db compact
kscorectl db compact --dry-run
```

**Flags**:

- `--dry-run`: Show what would be done without executing

#### db rotate-credentials

Rotate database access credentials. The control plane will reconnect with new credentials.

```bash
kscorectl db rotate-credentials
kscorectl db rotate-credentials --force
```

**Flags**:

- `--force, -f`: Skip confirmation prompt

### kscorectl diagnostics

System diagnostics collection.

#### diagnostics collect

Collect system diagnostics into a directory for troubleshooting.

```bash
kscorectl diagnostics collect
kscorectl diagnostics collect --output-dir /tmp/diag
kscorectl diagnostics collect --include-logs --since 24h
kscorectl diagnostics collect --include-logs --include-config --since 7d
```

**Flags**:

- `--output-dir string`: Output directory (default: kscore-diagnostics-\<timestamp\>)
- `--include-logs`: Include recent log files
- `--include-config`: Include sanitized configuration
- `--since string`: How far back to collect logs (e.g., 1h, 24h, 7d) (default: 1h)

### kscorectl security

Security scanning and assessment.

#### security scan

Run a security scan of the Keystone Core deployment.

```bash
kscorectl security scan
kscorectl security scan --full
kscorectl security scan --targets "role:web"
kscorectl security scan --full --output json
```

**Flags**:

- `--full`: Run comprehensive security scan (includes vulnerability scan, config review, audit log integrity)
- `--output string`: Output format (text, json) (default: text)
- `--targets string`: Target agents by expression (e.g., 'role:web')

### kscorectl nats

NATS message bus management.

#### nats rotate-credentials

Rotate NATS authentication credentials (NKeys/JWT tokens). This is a security incident response action.

```bash
kscorectl nats rotate-credentials
kscorectl nats rotate-credentials --force
```

**Flags**:

- `-f, --force`: Skip confirmation prompt

#### nats status

Show the status of NATS connections and cluster health.

```bash
kscorectl nats status
kscorectl nats status --output json
```

**Flags**:

- `--output string`: Output format (text, json) (default: text)

## kscore-exec (Remote Execution)

Execute commands on remote agents.

### exec run

Execute command synchronously across multiple agents.

```bash
kscorectl exec run <target-expression> -- <command> [args...]
kscorectl exec run <command> --target <target-expression>
```

**Arguments**:

- `<target-expression>`: Target expression to select agents (required)
- `<command>`: Command to execute (required, after --)
- `[args...]`: Command arguments (optional)
- `--target`: Optional target expression flag (useful when command comes first)

**Global Flags**:

- `--server string`: Keystone Core server address (default: localhost:50051)
- `--timeout duration`: Request timeout (default: 5m)
- `--audit-level string`: Audit logging level (all, errors, none)
- `--audit-output string`: Audit output backend (auto, syslog, journald, stderr, none)
- `--tls-ca-cert string`: Path to CA certificate for verifying the server
- `--tls-cert string`: Path to client certificate for mTLS authentication
- `--tls-key string`: Path to client private key for mTLS authentication
- `--tls-server-name string`: Server name for TLS verification (defaults to server host)
- `--tls-min-version string`: Minimum TLS version (1.2 or 1.3, default: 1.3)
- `--tls-skip-verify`: Skip TLS certificate verification (requires `KSCORE_ALLOW_INSECURE_TLS=1`, dev only)

**Flags**:

- `--concurrency int`: Number of concurrent executions (default: 10)
- `--continue-on-failure`: Continue executing on other agents if some fail (default: true)
- `--working-dir string`: Working directory for command execution
- `--user string`: User to execute command as
- `--command-timeout int`: Command timeout in seconds (default: 300)
- `--env strings`: Environment variables (KEY=VALUE, can be repeated)
- `--job-id string`: Custom batch job ID (auto-generated if not specified)
- `--show-progress`: Show progress updates during execution (default: true)
- `--show-results`: Show per-agent results at the end (default: true)
- `--dry-run`: Preview matched agents and command without executing
- `--shell string`: Shell to use for command execution (e.g., powershell, cmd, bash)

**Target Expression Syntax**:

- Label matching: `role:web`, `env:prod`
- OS matching: `os:linux`, `arch:amd64`
- Hostname glob: `hostname:web-*`
- Status: `status:agent_status_online`
- Logical operators: `and`, `or`, `not`
- Grouping: `(os:linux and role:web) or (os:darwin and role:api)`

**Examples**:

```bash
# Execute on all Linux web servers
kscorectl exec run "os:linux and role:web" -- systemctl restart nginx

# Execute on specific hostname pattern
kscorectl exec run "hostname:web-*" -- apt-get update

# Execute with environment variables
kscorectl exec run "role:db" --env DB_HOST=localhost -- ./backup.sh

# Execute with custom concurrency
kscorectl exec run "env:prod" --concurrency 10 -- uptime

# Execute without the -- separator (target then command)
kscorectl exec run "role:web" echo "hello"
```

**Output**:

```
Executing: systemctl restart nginx
Target: os:linux and role:web

Batch job started: job-abc123
Progress: 50/50 agents | Success: 48 | Failed: 2 | Success Rate: 96.0%

Batch execution completed

=== Summary ===
Total Agents:      50
Successful:        48
Failed:            2
Success Rate:      96.0%
Duration:          2500ms

=== Agent Results ===
✓ web-01 (exit code: 0, duration: 150ms)
✓ web-02 (exit code: 0, duration: 145ms)
✗ web-03 (exit code: 1, duration: 200ms)
  Error: service not found
...
```

### exec status

Get the status of a batch job.

```bash
kscorectl exec status <job-id>
```

**Arguments**:

- `<job-id>`: Batch job ID (required)

**Examples**:

```bash
kscorectl exec status abc123
kscorectl exec status --server prod-server:50051 abc123
```

**Output**:

```
Batch Job: abc123
Target:    os:linux and role:web
Command:   systemctl restart nginx
Status:    COMPLETED
Created:   2024-01-15T10:30:45Z
Started:   2024-01-15T10:30:45Z
Completed: 2024-01-15T10:30:47Z
Duration:  2100ms

=== Progress ===
Total:         50
Completed:     50
Successful:    48
Failed:        2
Success Rate:  96.0%

=== Agent Results ===
✓ web-01 (exit code: 0, duration: 150ms)
✓ web-02 (exit code: 0, duration: 145ms)
✗ web-03 (exit code: 1, duration: 200ms)
  Error: service not found
```

### exec list

List batch jobs with optional filtering.

```bash
kscorectl exec list [flags]
```

**Flags**:

- `--status string`: Filter by status (pending, running, completed, failed)
- `--page-size int`: Number of jobs to return (default: 20)

**Examples**:

```bash
# List all jobs
kscorectl exec list

# List only completed jobs
kscorectl exec list --status completed

# List only running jobs
kscorectl exec list --status running

# List with custom page size
kscorectl exec list --page-size 50
```

**Output**:

```
JOB ID                               TARGET               STATUS       TOTAL    SUCCESS  FAILED
abc123-def4-5678-90ab-cdef12345678   os:linux and role:w  COMPLETED    50       48       2
def456-ghi7-8901-23cd-ef4567890123   role:web             RUNNING      100      75       0
ghi789-jkl0-1234-56mn-op7890123456   env:prod             PENDING      200      0        0

Total: 3 job(s)
```

### exec version

Display kscore-exec version information.

```bash
kscorectl exec version
```

**Output**:

```
kscore-exec version 1.0.0
  Git commit: abc123def
  Built: 2026-01-10T10:00:00Z
  Go version: go1.25
```

### exec shell

Start an interactive shell session on a remote agent.

```bash
kscorectl exec shell <target-expression>
```

**Arguments**:

- `<target-expression>`: Target expression to select a single agent (required)

**Flags**:

- `--user string`: User to run shell as
- `--working-dir string`: Initial working directory
- `--shell string`: Shell to use (default: agent's default shell)

**Examples**:

```bash
# Start shell on specific agent
kscorectl exec shell "hostname:web-01"

# Shell with specific user
kscorectl exec shell "hostname:web-01" --user deploy

# Shell with specific working directory
kscorectl exec shell "hostname:web-01" --working-dir /app
```

**Notes**:

- Target must resolve to exactly one agent
- Requires PTY support on the agent
- Press Ctrl+D or type `exit` to close the session

### exec script

Execute a script file on remote agents.

```bash
kscorectl exec script <target-expression> <script-file>
```

**Arguments**:

- `<target-expression>`: Target expression to select agents (required)
- `<script-file>`: Path to local script file to execute (required)

**Flags**:

- `--interpreter, -i string`: Script interpreter (auto-detected from shebang if not specified)
- `--args strings`: Arguments to pass to the script
- `--concurrency int`: Number of concurrent executions (default: 10)
- `--continue-on-failure`: Continue executing on other agents if some fail (default: true)
- `--working-dir string`: Working directory for script execution
- `--user string`: User to execute script as
- `--timeout int`: Script timeout in seconds (default: 600)
- `--env strings`: Environment variables (KEY=VALUE, can be repeated)
- `--job-id string`: Custom batch job ID (auto-generated if not specified)

**Examples**:

```bash
# Execute a bash script on all web servers
kscorectl exec script "role:web" ./deploy.sh

# Execute with arguments
kscorectl exec script "env:prod" ./backup.sh --args "--full --compress"

# Use specific interpreter
kscorectl exec script "os:linux" ./setup.py --interpreter /usr/bin/python3

# Execute with custom timeout
kscorectl exec script "role:db" ./migration.sh --timeout 30m
```

**Output**:

```
Uploading script: ./deploy.sh (2.5KB)
Executing on 10 agents...

Progress: 10/10 agents | Success: 10 | Failed: 0

=== Results ===
✓ web-01: exit code 0 (duration: 5.2s)
✓ web-02: exit code 0 (duration: 4.8s)
...
```

### exec async

Execute a command asynchronously and return immediately.

```bash
kscorectl exec async <target-expression> -- <command> [args...]
```

**Flags**:

- Same as `exec run`

**Examples**:

```bash
# Start long-running command
kscorectl exec async "role:batch" -- ./long-job.sh

# Check status later
kscorectl exec status <job-id>
```

### exec cancel

Cancel a running batch job.

```bash
kscorectl exec cancel <job-id>
```

**Examples**:

**Flags**:

- `--force, -f`: Skip confirmation prompt

**Examples**:

```bash
kscorectl exec cancel abc123

# Force cancel without confirmation
kscorectl exec cancel abc123 --force
```

### exec history

View execution history.

```bash
kscorectl exec history [flags]
```

**Flags**:

- `--limit int`: Maximum number of entries to show (default: 20)
- `--target string`: Filter by target expression
- `--status string`: Filter by status (pending, running, completed, failed)
- `--since string`: Show history since (e.g., 1h, 24h, 7d)
- `--before string`: Show history before (e.g., 1h, 24h, 7d)

**Examples**:

```bash
# Recent history
kscorectl exec history

# Last 24 hours
kscorectl exec history --since 24h

# Filter by target
kscorectl exec history --target "role:web"
```

### exec output

Retrieve output from a completed job.

```bash
kscorectl exec output <job-id>
```

**Flags**:

- `--agent, -a string`: Filter output by agent ID
- `--follow, -f`: Follow output in real-time
- `--tail int`: Show only the last N lines
- `--output, -o string`: Output format (text, json) (default: text)

**Examples**:

```bash
# Get all output
kscorectl exec output abc123

# Get output for specific agent
kscorectl exec output abc123 --agent web-01
```

### exec archive

Archive completed batch jobs to free up active storage. Archived jobs are moved to long-term storage and no longer appear in the default list output.

```bash
kscorectl exec archive [flags]
```

**Flags**:

- `--status string`: Archive jobs with this status (default: completed; values: completed, failed)
- `--before string`: Archive jobs older than this duration (e.g., 24h, 7d)
- `--dry-run`: Preview what would be archived without making changes

**Examples**:

```bash
# Archive all completed jobs
kscorectl exec archive --status completed

# Archive jobs completed before 7 days ago
kscorectl exec archive --status completed --before 7d

# Dry run to preview what would be archived
kscorectl exec archive --status completed --dry-run
```

### exec export

Export batch job results to JSON or CSV format. If a job ID is provided, exports that specific job's results. Otherwise, exports all jobs matching the filter criteria.

```bash
kscorectl exec export [job-id] [flags]
```

**Arguments**:

- `[job-id]`: Batch job ID (optional; if omitted, exports all matching jobs)

**Flags**:

- `--format, -f string`: Export format (json, csv) (default: json)
- `--output, -o string`: Output file (default: stdout)
- `--status string`: Filter by status when exporting all jobs

**Examples**:

```bash
# Export a specific job's results to JSON
kscorectl exec export abc123 --format json --output results.json

# Export all completed jobs to JSON
kscorectl exec export --status completed --output jobs.json

# Export to stdout
kscorectl exec export abc123 --format json
```

### exec cleanup

Permanently remove old batch job records from storage. Use `--dry-run` to preview what would be deleted before running.

```bash
kscorectl exec cleanup [flags]
```

**Flags**:

- `--older-than string`: Remove jobs older than this duration (required; e.g., 7d, 30d)
- `--status string`: Only remove jobs with this status (completed, failed)
- `--dry-run`: Preview what would be deleted without making changes
- `--force, -f`: Skip confirmation prompt

**Examples**:

```bash
# Remove completed jobs older than 30 days
kscorectl exec cleanup --older-than 30d --status completed

# Remove all completed and failed jobs older than 7 days
kscorectl exec cleanup --older-than 7d

# Preview what would be deleted
kscorectl exec cleanup --older-than 7d --dry-run

# Remove without confirmation
kscorectl exec cleanup --older-than 30d --force
```

## kscore-state (State Management)

Manage declarative state configurations locally. State commands execute on the local machine where kscore-state runs.

### state apply

Apply state declarations from a YAML file.

```bash
kscorectl state apply <state-file> [flags]
```

**Arguments**:

- `<state-file>`: Path to state YAML file (required)

**Flags**:

- `--vars string`: Variables file (YAML)
- `--dry-run`: Check what would change without applying
- `--preview`: Show rendered state with variables/facts substituted without executing
- `--target string`: Target expression (accepted but ignored in local mode)
- `--audit-level string`: Audit logging level (all, errors, none)
- `--audit-output string`: Audit output backend (auto, syslog, journald, stderr, none)

**Examples**:

```bash
# Apply a state file
kscorectl state apply states/webserver.yaml

# Apply with variables
kscorectl state apply states/app.yaml --vars vars/production.yaml

# Dry-run (check what would change)
kscorectl state apply states/app.yaml --dry-run

# Preview rendered state (show variable substitution)
kscorectl state apply states/app.yaml --preview --vars vars/staging.yaml
```

**Output**:

```
Loading state file: states/webserver.yaml
Applying state: Web server configuration

=== Results ===
✓ nginx_package.package: unchanged
✓ nginx_config.file: changed
  Changes:
    contents: (updated)
✓ nginx_service.service: changed
  Restarted service

=== Summary ===
Total states:  3
Succeeded:     3
Failed:        0
Changed:       2
Unchanged:     1
Duration:      1.5s

✓ Success!
```

### state check

Check state without applying (dry-run). Equivalent to `state apply --dry-run`.

```bash
kscorectl state check <state-file> [flags]
```

**Arguments**:

- `<state-file>`: Path to state YAML file (required)

**Flags**:

- `--vars string`: Variables file (YAML)
- `--target string`: Target expression (accepted but ignored in local mode)

**Examples**:

```bash
# Check a state file
kscorectl state check states/webserver.yaml

# Check with variables
kscorectl state check states/app.yaml --vars vars/staging.yaml
```

**Output**:

```
Loading state file: states/webserver.yaml
Checking state: Web server configuration

=== Results ===
✓ nginx_package.package: unchanged
✓ nginx_config.file: would change
  Changes:
    contents: (would update)
✓ nginx_service.service: would change
  Would restart service

=== Summary ===
Total states:  3
Succeeded:     3
Failed:        0
Changed:       2
Unchanged:     1
Duration:      500ms

✓ Success!
```

### state drift

Detect configuration drift by comparing desired state to actual state.

```bash
kscorectl state drift <state-file> [flags]
```

**Arguments**:

- `<state-file>`: Path to state YAML file (required)

**Flags**:

- `--vars string`: Variables file (YAML)
- `--target string`: Target expression (accepted but ignored in local mode)

**Examples**:

```bash
# Detect drift
kscorectl state drift states/webserver.yaml

# Detect drift with variables
kscorectl state drift states/app.yaml --vars vars/production.yaml
```

**Output (no drift)**:

```
Loading state file: states/webserver.yaml
Checking drift for: Web server configuration

=== Drift Report ===
Overall Severity: none

Total:  3
None:   3
Low:    0
Medium: 0
High:   0

✓ No drift detected
```

**Output (with drift)**:

```
Loading state file: states/webserver.yaml
Checking drift for: Web server configuration

=== Drift Report ===
Overall Severity: high

nginx_service (service): HIGH
  Differences:
    - state: expected=running, actual=stopped

nginx_config (file): MEDIUM
  Differences:
    - mode: expected=0644, actual=0755

Total:  3
None:   1
Low:    0
Medium: 1
High:   1
```

### state test

Test state declarations without applying them (alias for check with test-focused output).

```bash
kscorectl state test <state-file> [flags]
```

**Arguments**:

- `<state-file>`: Path to state YAML file (required)

**Flags**:

- `--vars string`: Variables file (YAML)
- `--target string`: Target expression (accepted but ignored in local mode)

**Examples**:

```bash
# Test a state file
kscorectl state test states/webserver.yaml

# Test with variables
kscorectl state test states/app.yaml --vars vars/staging.yaml
```

### state diff

Show differences between desired and actual state. This is an alias for `state drift` with output focused on changes.

```bash
kscorectl state diff <state-file> [flags]
```

**Arguments**:

- `<state-file>`: Path to state YAML file (required)

**Flags**:

- `--vars string`: Variables file (YAML)
- `--target string`: Target expression (accepted but ignored in local mode)

**Examples**:

```bash
# Show differences
kscorectl state diff states/webserver.yaml

# Show differences with variables
kscorectl state diff states/app.yaml --vars vars/production.yaml
```

### state show

Display the rendered state declarations after template processing without executing anything.

```bash
kscorectl state show <state-file> [flags]
```

**Arguments**:

- `<state-file>`: Path to state YAML file (required)

**Flags**:

- `--vars string`: Variables file (YAML)

**Examples**:

```bash
# Show rendered state
kscorectl state show states/webserver.yaml

# Show with variables
kscorectl state show states/app.yaml --vars vars/production.yaml
```

**Output**:

```
=== Rendered State Preview ===

Metadata:
  Name:        webserver
  Description: Web server configuration
  Version:     1.0.0

Variables Applied:
  environment: production
  port: 8080

States (3 total, in execution order):
─────────────────────────────────────────────────

1. package.nginx_package
   State: installed
   Parameters:
     name: nginx

2. file.nginx_config
   State: managed
   Parameters:
     path: /etc/nginx/nginx.conf
     source: templates/nginx.conf
   Requisites:
     require:
       - package.nginx_package

3. service.nginx_service
   State: running
   Parameters:
     name: nginx
     enable: true
   Requisites:
     watch:
       - file.nginx_config
```

### state history

List state application history or show details of a specific application. Requires control plane connectivity.

```bash
kscorectl state history [application-id] [flags]
```

**Arguments**:

- `[application-id]`: Optional application ID to show details

**Flags**:

- `--target string`: Filter by target expression
- `--limit int`: Maximum number of entries to show (default: 20)
- `--json`: Output in JSON format

**Examples**:

```bash
# List recent applications
kscorectl state history --limit 20

# Show details of a specific application
kscorectl state history app-123

# List applications for a specific target
kscorectl state history --target "role:web" --limit 10
```

**Output**:

```
  ID           TIMESTAMP            TARGET     STATUS   CHANGES
  app-abc123   2024-01-19 10:30:00  role:web   success  5 changed
  app-def456   2024-01-19 10:15:00  role:db    success  2 changed
  app-ghi789   2024-01-19 10:00:00  role:web   failed   0 changed
```

### state rollback

Rollback the system to a previous state application. Requires control plane connectivity.

```bash
kscorectl state rollback <application-id> [flags]
```

**Arguments**:

- `<application-id>`: Application ID to rollback to (required)

**Flags**:

- `--dry-run`: Show what would be rolled back without applying
- `--force`: Skip confirmation prompt

**Examples**:

```bash
# Rollback to a specific application
kscorectl state rollback app-123

# Dry-run rollback to see what would change
kscorectl state rollback app-123 --dry-run

# Force rollback without confirmation
kscorectl state rollback app-123 --force
```

### state version

Display kscore-state version information.

```bash
kscorectl state version
```

**Output**:

```
Keystone Core v1.0.0
  Build: abc123
  Go version: go1.21.5
  OS/Arch: linux/amd64
```

## kscore-monitor (Real-time Monitoring)

Terminal-based real-time monitoring.

### monitor

Start TUI monitor.

```bash
kscorectl monitor [flags]
```

**Flags**:

- `--control-plane string`: Control plane gRPC address
- `--server string`: Alias for `--control-plane`
- `--nats-url string`: NATS server URL for direct connection
- `--theme string`: UI theme (dark, light, solarized-dark, solarized-light, monokai)
- `--view int`: Initial view (1-8)
- `--refresh int`: Refresh interval in seconds (default: 2)
- `--no-color`: Disable colors

**Views**:

1. Dashboard - System overview
2. Agents - Agent list and status
3. Events - Real-time event stream
4. State Drift - Configuration drift
5. Policy Violations - Compliance
6. Jobs - Command execution history
7. Logs - Structured log streaming
8. Metrics - Performance metrics

**Keyboard Navigation**:

```
1-8     Switch views
↑/↓     Scroll
/       Search
f       Filter
r       Refresh
p       Pause/Resume
q       Quit
```

**Example**:

```bash
# Start with default view
kscorectl monitor

# Start with events view
kscorectl monitor --view 3

# Connect to specific server
kscorectl monitor --server production.example.com:8080
```

## kscore-module (Module Management)

Manage Keystone Core modules with dependency resolution, verification, and distribution.

Keystone Core modules are versioned, capability-scoped packages that extend the system with custom state management, reactors, policies, and more. The CLI provides commands for the full module lifecycle: scaffolding, validation, building, testing, and distribution.

> **API Status**: The `resolve` command uses the real registry HTTP client (configurable via `--registry` flag or `$KSCORE_REGISTRY`). The `verify` command performs signature verification locally but SumDB transparency log verification is not yet available. The `test --coverage` flag is not yet implemented.

### module init

Initialize a new module from a template.

```bash
kscorectl module init <name> [flags]
```

**Arguments**:

- `<name>`: Module name in vendor/package format (required)

**Flags**:

- `--type string`: Module type: `starlark` or `wasm` (default: starlark)
- `--author string`: Module author name
- `--description string`: Module description
- `--output string`: Output directory (defaults to package name)

**Examples**:

```bash
# Create a Starlark module
kscorectl module init myorg/webserver

# Create with metadata
kscorectl module init myorg/webserver \
  --author "John Doe" \
  --description "Web server configuration module"

# Create a WASM module (Rust template)
kscorectl module init myorg/custom-executor --type wasm

# Create in specific directory
kscorectl module init myorg/webserver --output ./modules/webserver
```

**Output**:

```
Creating module: myorg/webserver
Output directory: webserver
Type: starlark

✓ Module created successfully!

Next steps:
  cd webserver
  # Edit states/main.star to add your state definitions
  # Edit module.yaml to configure capabilities
  kscorectl module validate .
  kscorectl module test
  kscorectl module build
```

**Created Files** (Starlark):

```
webserver/
├── module.yaml        # Module manifest with metadata and capabilities
├── README.md          # Documentation
├── states/
│   └── main.star      # Main Starlark module code
└── tests/
    └── main_test.star # Test file
```

### module validate

Validate a module's manifest (module.yaml) for correctness.

```bash
kscorectl module validate [path] [flags]
```

**Arguments**:

- `[path]`: Path to module directory or module.yaml file (default: current directory)

**Flags**:

- `--strict`: Treat warnings as errors

**Validation Checks**:

- YAML syntax validation
- Required fields (name, version, type)
- Module type validation (starlark, wasm, hybrid)
- Capability declarations (known capabilities, dangerous capabilities)
- Dependency format validation
- Resource limit values (timeout, memory)
- Entrypoint file existence

**Examples**:

```bash
# Validate current directory
kscorectl module validate

# Validate specific directory
kscorectl module validate ./my-module

# Validate specific file
kscorectl module validate ./my-module/module.yaml

# Strict mode (warnings as errors)
kscorectl module validate --strict
```

**Output (Success)**:

```
Validating: ./my-module/module.yaml

✓ Module is valid!

Module Summary:
  Name:         myorg/webserver
  Version:      1.0.0
  Type:         starlark
  Description:  Web server configuration module
  Author:       John Doe
  Capabilities: [fs.read, fs.write]
  Dependencies: 2
```

**Output (Warnings)**:

```
Validating: ./my-module/module.yaml

Warnings:
  ⚠ capability 'exec' requires elevated trust level
  ⚠ no entrypoint specified

✓ Module is valid (with warnings)
```

**Output (Errors)**:

```
Validating: ./my-module/module.yaml

Errors:
  ✗ name is required
  ✗ invalid type 'unknown' (use: starlark, wasm, hybrid)

Warnings:
  ⚠ no description specified

✗ Validation failed (2 errors, 1 warnings)
```

### module build

Package a module as a distributable ZIP archive.

```bash
kscorectl module build [path] [flags]
```

**Arguments**:

- `[path]`: Path to module directory (default: current directory)

**Flags**:

- `--output, -o string`: Output ZIP file path (default: `<name>-<version>.zip`)
- `--no-verify`: Skip pre-build validation

**Examples**:

```bash
# Build in current directory
kscorectl module build

# Build specific directory
kscorectl module build ./my-module

# Custom output file
kscorectl module build --output dist/my-module-1.0.0.zip

# Skip validation
kscorectl module build --no-verify
```

**Output**:

```
Building module: myorg/webserver v1.0.0
✓ Validation passed
Including 12 files

✓ Build complete!

Output:   myorg-webserver-1.0.0.zip
Size:     4.2 KB
SHA256:   a1b2c3d4e5f6...

Next steps:
  kscorectl module verify myorg-webserver-1.0.0.zip
  kscorectl module sign myorg-webserver-1.0.0.zip --key private.pem
  kscorectl module publish myorg-webserver-1.0.0.zip
```

### module resolve

Resolve module dependencies and generate a lock file.

> **Note**: Full registry-backed resolution is planned for a future release.
> Currently, resolution works with existing lock files and cached dependencies.
> Without a configured registry, you'll need to manually install dependencies
> or use `--offline` mode with a valid lock file.

```bash
kscorectl module resolve [path] [flags]
```

**Arguments**:

- `[path]`: Path to module directory (default: current directory)

**Flags**:

- `--lock-file string`: Lock file path (default: module.lock)
- `--update`: Update to latest compatible versions (ignores existing lock file)
- `--allow-prerelease`: Include pre-release versions in resolution
- `--timeout duration`: Resolution timeout (default: 5m)
- `--cache-dir string`: Module cache directory
- `--offline`: Offline mode (use cache only)

**Resolution Process**:

1. Parses module.yaml for dependencies
2. Queries registry for available versions (when registry is configured)
3. Resolves version constraints using MVS (Minimum Version Selection) algorithm
4. Detects circular dependencies
5. Generates module.lock with pinned versions and hashes

**Current Limitations**:

- Registry querying requires a configured module registry (not yet available)
- Without a registry, resolution only works with existing lock files or cached modules
- Use `--offline` with a manually created lock file as a workaround

**Examples**:

```bash
# Resolve dependencies
kscorectl module resolve

# Update to latest compatible versions
kscorectl module resolve --update

# Allow pre-release versions
kscorectl module resolve --allow-prerelease

# Offline mode (use cache only)
kscorectl module resolve --offline
```

**Output**:

```
Resolving dependencies for: myorg/webserver v1.0.0
Dependencies: 3

Using existing lock file: module.lock
Resolved 5 dependencies in 234ms

Resolved dependencies:
    std/files @ 1.2.0
    std/exec @ 1.0.0
  ↑ std/http @ 1.1.0 (constraint: ^1.0.0)
  + myorg/utils @ 0.5.0

✓ Lock file written: module.lock

Next steps:
  kscorectl module tree            # View dependency tree
  kscorectl module install         # Download dependencies
```

**Lock File Format** (module.lock):

```yaml
schema_version: 1
modules:
  std/files:
    version: "1.2.0"
    hash: "sha256:abc123..."
  std/exec:
    version: "1.0.0"
    hash: "sha256:def456..."
```

### module tree

Display the dependency tree for a module.

```bash
kscorectl module tree [path] [flags]
```

**Arguments**:

- `[path]`: Path to module directory (default: current directory)

**Flags**:

- `--depth int`: Maximum depth to display (0 = unlimited)
- `--flat`: Show as flat list instead of tree

**Examples**:

```bash
# Show dependency tree
kscorectl module tree

# Limit depth
kscorectl module tree --depth 2

# Flat list
kscorectl module tree --flat
```

**Output (Tree)**:

```
myorg/webserver@1.0.0
├── std/files@1.2.0
├── std/exec@1.0.0 (constraint: ^1.0.0)
└── std/http@1.1.0

(+ 2 transitive dependencies in lock file)
```

**Output (Flat)**:

```
NAME                     CONSTRAINT      RESOLVED        HASH
----                     ----------      --------        ----
std/files                ^1.0.0          1.2.0           abc123def456...
std/exec                 ^1.0.0          1.0.0           789ghi012jkl...
std/http                 ^1.0.0          1.1.0           mno345pqr678...
myorg/utils              (transitive)    0.5.0           stu901vwx234...

Total: 4 dependencies
```

### module verify

Verify a module's cryptographic integrity.

```bash
kscorectl module verify <path> [flags]
```

**Arguments**:

- `<path>`: Path to module ZIP file (required)

**Flags**:

- `--require-signature`: Require valid digital signature
- `--require-sumdb`: Require SumDB (transparency log) verification
- `--hash string`: Expected hash for verification (format: sha256:hex)
- `--public-key string`: Public key file for signature verification
- `--sumdb-url string`: SumDB URL for transparency verification
- `--allow-insecure`: Allow verification to proceed even if some checks fail

**Verification Checks**:

- SHA256 hash integrity
- Digital signature (RSA, ECDSA, Ed25519)
- Transparency log (SumDB) verification

**Examples**:

```bash
# Verify hash only
kscorectl module verify my-module-1.0.0.zip

# Verify with expected hash
kscorectl module verify my-module.zip --hash sha256:abc123...

# Require signature verification
kscorectl module verify my-module.zip \
  --require-signature \
  --public-key trusted.pem

# Full verification including SumDB
kscorectl module verify my-module.zip \
  --require-signature \
  --require-sumdb
```

**Output**:

```
Verifying: my-module-1.0.0.zip
Size: 4.2 KB

Computing SHA256 hash... done
Verifying signature... VALID
Checking transparency log... SKIPPED (no SumDB URL)

=== Verification Results ===
✓ Hash computation:    sha256:a1b2c3d4...
✓ Hash match:          matches expected
✓ Signature:           valid

SHA256: a1b2c3d4e5f6789012345678901234567890abcdef

✓ Verification passed!
```

### module test

Run tests for a Starlark module.

```bash
kscorectl module test [path] [flags]
```

**Arguments**:

- `[path]`: Path to module directory (default: current directory)

**Flags**:

- `-v, --verbose`: Show verbose test output
- `--filter string`: Filter tests by name pattern
- `--timeout duration`: Test timeout (default: 5m)
- `--coverage`: Enable coverage reporting (not yet implemented)

**Test Discovery**:

- Tests are discovered in the `tests/` directory
- Test files must end with `_test.star`
- Test functions must start with `test_`

**Available Assertions**:

- `assert.eq(actual, expected)` - Assert equality
- `assert.ne(actual, expected)` - Assert inequality
- `assert.true(value)` - Assert truthy
- `assert.false(value)` - Assert falsy
- `assert.fail(fn)` - Assert function raises error
- `assert.contains(haystack, needle)` - Assert string contains

**Examples**:

```bash
# Run all tests
kscorectl module test

# Run tests in specific directory
kscorectl module test ./my-module

# Run specific test
kscorectl module test --filter test_my_function

# Verbose output
kscorectl module test -v
```

**Output**:

```
Running tests for: myorg/webserver v1.0.0
Found 2 test file(s)

✓ tests/main_test.star (3 tests)
✓ tests/utils_test.star (5 tests)

=== Summary ===
Total:   8
Passed:  8
Failed:  0
Time:    45ms

✓ All tests passed!
```

**Output (Verbose)**:

```
Running tests for: myorg/webserver v1.0.0
Found 2 test file(s)

=== tests/main_test.star ===
  ✓ test_hello_default
  ✓ test_hello_custom
  ✓ test_main

=== tests/utils_test.star ===
  ✓ test_format_size
  ✓ test_parse_config
  ✓ test_validate_path
  ✓ test_sanitize_input
  ✓ test_error_handling

=== Summary ===
Total:   8
Passed:  8
Failed:  0
Time:    45ms

✓ All tests passed!
```

**Output (Failures)**:

```
Running tests for: myorg/webserver v1.0.0

=== tests/main_test.star ===
  ✓ test_hello_default
  ✗ test_hello_custom
    Error: assertion failed: expected "Hello, World!", got "Hello, world!"

=== Summary ===
Total:   3
Passed:  2
Failed:  1
Time:    23ms

✗ Tests failed!
```

### module sign

Sign a module archive with a private key. Creates a detached signature file (.sig) that can be verified using the corresponding public key.

```bash
kscorectl module sign <path> [flags]
```

**Flags**:

- `-k, --key string`: Private key file (PEM format)
- `-o, --output string`: Output signature file (default: `<module>.sig`)
- `-f, --force`: Overwrite existing signature file
- `--generate-key string`: Generate new key pair (rsa, ecdsa, ed25519)
- `--key-bits int`: Key size in bits for RSA (default: 2048)

**Supported Key Types**:

- RSA (2048, 4096 bits)
- ECDSA (P-256)
- Ed25519

**Examples**:

```bash
# Sign with an existing private key
kscorectl module sign my-module.zip --key private.pem

# Generate a new Ed25519 key pair and sign
kscorectl module sign my-module.zip --generate-key ed25519

# Generate RSA key with 4096 bits
kscorectl module sign my-module.zip --generate-key rsa --key-bits 4096

# Sign with custom output path
kscorectl module sign my-module.zip --key private.pem --output signatures/my-module.sig

# Force overwrite existing signature
kscorectl module sign my-module.zip --key private.pem --force
```

**Output (with key generation)**:

```
Signing: my-module.zip
Size: 1.3 KB
Generating ED25519 key pair...
Private key: my-module.key (keep secret!)
Public key:  my-module.pub (share for verification)
Creating signature... done
Signature:   my-module.zip.sig (64 B)

Module SHA256: sha256:f28f3dbe8066d05fff31e9ef18f7655b3d9868346dea5482205e259bce3c5fc7

To verify this signature:
  kscorectl module verify my-module.zip --require-signature --public-key my-module.pub

✓ Module signed successfully!
```

**Key Management**:

- Private keys are written with mode 0600 (owner read/write only)
- Public keys are written with mode 0644 (world readable)
- Keep private keys secure - anyone with the private key can sign modules
- Distribute public keys for verification

**Workflow**:

```bash
# 1. Build module
kscorectl module build

# 2. Sign with new key (or existing)
kscorectl module sign my-module-1.0.0.zip --generate-key ed25519

# 3. Distribute module.zip + module.zip.sig + module.pub

# 4. Recipients verify
kscorectl module verify my-module-1.0.0.zip --require-signature --public-key my-module.pub
```

### module publish

Publish a module to a registry. The module must be a built ZIP file with a valid manifest.

```bash
kscorectl module publish <path> [flags]
```

**Flags**:

- `--registry string`: Registry URL (default: KSCORE_REGISTRY env or <https://registry.keystonecore.io>)
- `--token string`: Authentication token (or KSCORE_REGISTRY_TOKEN env)
- `--username string`: Username for basic auth (or KSCORE_REGISTRY_USERNAME env)
- `--password string`: Password for basic auth (or KSCORE_REGISTRY_PASSWORD env)
- `--signature string`: Path to detached signature file
- `--force`: Overwrite existing version
- `--release-notes string`: Release notes for this version
- `--tag strings`: Tags for this version (can be specified multiple times)
- `--dry-run`: Validate without publishing

**Examples**:

```bash
# Publish to the default registry
kscorectl module publish my-module-1.0.0.zip

# Publish with a specific registry URL
kscorectl module publish my-module-1.0.0.zip --registry https://registry.example.com

# Publish with authentication
kscorectl module publish my-module-1.0.0.zip --token $REGISTRY_TOKEN

# Publish with signature
kscorectl module publish my-module-1.0.0.zip --signature my-module-1.0.0.zip.sig

# Force overwrite existing version
kscorectl module publish my-module-1.0.0.zip --force

# Dry run (validate without publishing)
kscorectl module publish my-module-1.0.0.zip --dry-run
```

**Output (Dry Run)**:

```
Publishing: my-module-1.0.0.zip
Size: 1.3 KB

Module: myorg/my-module
Version: 1.0.0
Type: starlark

SHA256: sha256:f28f3dbe8066d05fff31e9ef18f7655b...

Registry: https://registry.keystonecore.io

=== Dry Run ===
✓ Module file exists
✓ Manifest is valid
✓ Hash computed: sha256:f28f3dbe80...
✓ Signature file found

Dry run complete. Use --dry-run=false to publish.
```

**Output (Success)**:

```
Publishing: my-module-1.0.0.zip
Size: 1.3 KB

Module: myorg/my-module
Version: 1.0.0
Type: starlark

SHA256: sha256:f28f3dbe8066d05fff31e9ef18f7655b...

Registry: https://registry.keystonecore.io

Publishing module... done

=== Published ===
Module: myorg/my-module@1.0.0
Hash: sha256:f28f3dbe8066d05fff31e9ef18f7655b...
Size: 1.3 KB
URL: https://registry.keystonecore.io/myorg/my-module/@v/1.0.0.zip
Signature: ✓ verified
Published: 2024-01-15 10:30:45

✓ Module published successfully!
```

**Authentication**:

- Bearer token (recommended): Use `--token` or `KSCORE_REGISTRY_TOKEN`
- Basic auth: Use `--username`/`--password` or `KSCORE_REGISTRY_USERNAME`/`KSCORE_REGISTRY_PASSWORD`

**Workflow**:

```bash
# Complete publish workflow
kscorectl module validate
kscorectl module build
kscorectl module sign my-module-1.0.0.zip --generate-key ed25519
kscorectl module publish my-module-1.0.0.zip --signature my-module-1.0.0.zip.sig
```

**Note**: Publishing requires a running module registry. The default registry (registry.keystonecore.io) is a placeholder - use `--registry` to specify your own registry instance.

### module install

Install modules from a registry.

Module archive entries larger than 256 MB are rejected to protect against decompression bombs.

```bash
kscorectl module install <module[@version]> [modules...] [flags]
```

**Flags**:

- `--registry string`: Registry URL (defaults to KSCORE_REGISTRY env var or <https://registry.keystonecore.io>)
- `--token string`: Authentication token (can also use KSCORE_REGISTRY_TOKEN)
- `--username string`: Username for basic auth (can also use KSCORE_REGISTRY_USERNAME)
- `--password string`: Password for basic auth (can also use KSCORE_REGISTRY_PASSWORD)
- `--cache-dir string`: Module cache directory (default: `KSCORE_CACHE_DIR` or `~/.keystone-core/modules`)
- `--modules-dir string`: Modules installation directory (default: ./modules)
- `--verify`: Verify module signatures
- `--public-key string`: Public key for signature verification
- `--force`: Force reinstall even if already installed
- `--dry-run`: Show what would be installed without installing
- `--global`: Install to global cache only (don't extract to modules dir)

**Module References**:

- `myorg/mymodule` - Install latest version
- `myorg/mymodule@1.0.0` - Install specific version
- `myorg/mymodule@^1.0.0` - Install latest compatible with 1.x.x
- `myorg/mymodule@~1.2.0` - Install latest compatible with 1.2.x

**Examples**:

```bash
# Install latest version
kscorectl module install myorg/webserver

# Install specific version
kscorectl module install myorg/webserver@1.2.3

# Install multiple modules
kscorectl module install myorg/webserver myorg/database@2.0.0

# Install with signature verification
kscorectl module install myorg/webserver --verify --public-key trusted.pem

# Install to global cache only
kscorectl module install myorg/webserver --global

# Dry run (show what would be installed)
kscorectl module install myorg/webserver --dry-run

# Install from custom registry with token
kscorectl module install myorg/webserver --registry https://registry.example.com --token $TOKEN
```

**Output**:

```
Registry: https://registry.keystonecore.io
Cache: /Users/user/.kscore/modules
Modules: modules

Installing myorg/webserver...
  Version: 1.2.3 (latest)
  Hash: sha256:abc123def456...
  Size: 15.2 KB
  Downloading... done
  Caching... done
  Extracting to modules/myorg/webserver/1.2.3... done
  ✓ Installed

=== Summary ===
Installed: 1

✓ Installation complete!
```

**Note**: Installing requires a running module registry. The default registry (registry.keystonecore.io) is a placeholder - use `--registry` to specify your own registry instance.

### module update

Update module dependencies to their latest compatible versions. Reads `module.yaml` and `module.lock`, checks the registry for newer versions that satisfy constraints, and updates the lock file.

**Usage**:

```bash
kscorectl module update [module...] [flags]
```

**Flags**:

| Flag | Description |
|------|-------------|
| `--dry-run` | Show available updates without applying |

**Examples**:

```bash
# Update all dependencies
kscorectl module update

# Update specific module
kscorectl module update std/files

# Dry run
kscorectl module update --dry-run
```

### module mirror

Export or import modules for air-gapped (offline) environments.

**Usage**:

```bash
kscorectl module mirror [module[@version]...] [flags]
```

**Flags**:

| Flag | Description |
|------|-------------|
| `--source string` | Source registry URL |
| `--dest string` | Destination directory for export |
| `--import string` | Mirror directory to import from |
| `--registry string` | Target offline registry path for import |
| `--dry-run` | Show what would be mirrored |
| `--verify` | Verify module signatures during mirror (default: true) |

**Examples**:

```bash
# Export modules to a mirror directory
kscorectl module mirror vendor/pkg_apt@v1.2.3 \
  --source https://registry.keystonecore.io \
  --dest ./module-mirror

# Import mirror into offline registry
kscorectl module mirror --import ./module-mirror \
  --registry /var/lib/keystone-core/registry
```

### module clean

Remove cached modules from the local module cache directory.

**Usage**:

```bash
kscorectl module clean [flags]
```

**Flags**:

| Flag | Description |
|------|-------------|
| `--all` | Remove all cached modules (not just stale) |
| `--dry-run` | Show what would be removed |

**Examples**:

```bash
# Clean stale cached modules
kscorectl module clean

# Remove all cached modules
kscorectl module clean --all

# Dry run
kscorectl module clean --dry-run
```

## kscore-blueprint (Blueprint Management)

Manage blueprint lifecycle tasks including initialization, validation, testing, and installation. Blueprints are pre-packaged, reusable collections of states similar to Salt Formulas, Ansible Roles, or Helm Charts.

> **Note**: Publishing, signing, and state operations are split into `kscore-blueprint-publish` and `kscore-blueprint-state` (Epic 30). These commands currently exist in kscore-blueprint with deprecation warnings.

### Global Flags

These flags apply to all kscore-blueprint commands:

- `--audit-level string`: Audit logging level: all, errors, none (default: all)
- `--audit-output string`: Audit output backend: auto, syslog, journald, stderr, none (default: auto)

### blueprint init

Initialize a new blueprint from a template.

```bash
kscorectl blueprint init <vendor/name> [flags]
```

**Arguments**:

- `<vendor/name>`: Blueprint name in vendor/package format (e.g., myorg/web-stack)

**Flags**:

- `--description string`: Blueprint description
- `--author string`: Author name
- `--email string`: Author email
- `--license string`: License identifier (default: Apache-2.0)
- `-o, --output string`: Output directory (default: current directory)
- `--category string`: Blueprint category (default: general)
- `--keywords strings`: Blueprint keywords (comma-separated)

**Created Structure**:

```
<name>/
├── blueprint.yaml    # Manifest with metadata and parameters
├── states/           # State declarations
├── vars/             # Default variable values
├── templates/        # Jinja2/Go templates
├── files/            # Static files
└── tests/            # Blueprint tests
```

**Examples**:

```bash
# Basic initialization
kscorectl blueprint init myorg/web-stack

# With full metadata
kscorectl blueprint init myorg/nginx-stack \
  --description "Production-ready Nginx deployment" \
  --author "John Doe" \
  --email "john@example.com" \
  --license "MIT" \
  --category "web" \
  --keywords "nginx,web,proxy,loadbalancer"

# Initialize in a specific directory
kscorectl blueprint init myorg/database --output ./blueprints/
```

### blueprint validate

Validate blueprint manifest syntax and structure.

```bash
kscorectl blueprint validate [path] [flags]
```

**Arguments**:

- `[path]`: Path to blueprint directory (default: current directory)

**Flags**:

- `--strict`: Treat warnings as errors (exit with error on any warning)
- `--format string`: Output format: text, json (default: text)

**Validation Checks**:

- YAML syntax validation
- Required fields (apiVersion, kind, metadata.name, metadata.version)
- Parameter schema validation
- Dependency reference validation
- Entrypoint file existence
- Template syntax (basic check)

**Examples**:

```bash
# Validate current directory
kscorectl blueprint validate

# Validate specific blueprint
kscorectl blueprint validate ./myorg-web-stack

# Strict mode for CI/CD
kscorectl blueprint validate ./myorg-web-stack --strict

# JSON output for parsing
kscorectl blueprint validate ./myorg-web-stack --format json
```

**Output (Success)**:

```
Validating blueprint: myorg/web-stack@1.0.0

✓ Manifest syntax valid
✓ Required fields present
✓ Parameters valid
✓ Dependencies resolvable
✓ Entrypoints exist

Validation passed!
```

**Output (Errors)**:

```
Validating blueprint: myorg/broken-stack@1.0.0

✗ Error: metadata.version is required
✗ Error: parameter 'port' has invalid type 'invalid'
! Warning: entrypoint 'main.yaml' not found

Validation failed: 2 errors, 1 warning
```

### blueprint lint

Run best-practice checks on a blueprint.

```bash
kscorectl blueprint lint [path] [flags]
```

**Arguments**:

- `[path]`: Path to blueprint directory (default: current directory)

**Flags**:

- `--fix`: Automatically fix issues where possible
- `--format string`: Output format: text, json (default: text)

**Lint Categories**:

- **documentation**: README exists, parameters documented, description present
- **security**: No hardcoded secrets, sensitive parameters marked
- **naming**: Consistent naming conventions, valid identifiers
- **versioning**: Semantic versioning, CHANGELOG present
- **testing**: Test files exist, coverage adequate
- **license**: License file present, valid SPDX identifier

**Examples**:

```bash
# Lint current directory
kscorectl blueprint lint

# Lint with auto-fix
kscorectl blueprint lint --fix

# JSON output for CI integration
kscorectl blueprint lint --format json
```

**Output**:

```
Linting blueprint: myorg/web-stack@1.0.0

Documentation:
  ✓ README.md exists
  ✓ All parameters documented
  ! Warning: No CHANGELOG.md found

Security:
  ✓ No hardcoded credentials
  ✓ Sensitive parameters marked

Naming:
  ✓ Blueprint name valid
  ✓ Parameter names consistent

Versioning:
  ✓ Semantic version format

Testing:
  ! Warning: No test files found in tests/

License:
  ✓ LICENSE file present
  ✓ Valid SPDX identifier

Summary: 0 errors, 2 warnings
```

### blueprint test

Run blueprint tests from the tests/ directory.

```bash
kscorectl blueprint test [path] [flags]
```

**Arguments**:

- `[path]`: Path to blueprint directory (default: current directory)

**Flags**:

- `-v, --verbose`: Verbose output with detailed test events
- `--dry-run`: Dry run (no actual state changes)
- `--timeout duration`: Default test timeout (default: 5m)
- `--parallel`: Run tests in parallel
- `--max-parallel int`: Maximum parallel tests (default: 4)
- `--tags strings`: Only run tests with these tags (comma-separated)
- `--exclude-tags strings`: Exclude tests with these tags
- `--pattern string`: Only run tests matching pattern (supports wildcards)
- `--format string`: Output format: text, json, junit (default: text)
- `-o, --output string`: Write output to file
- `--stop-on-failure`: Stop on first failure

**Test File Format** (tests/*_test.yaml):

```yaml
name: Basic Tests
description: Test basic blueprint functionality
setup:
  - mock:
      command: systemctl
      response: "active"
tests:
  - name: default parameters
    assertions:
      - type: no_failures
  - name: custom port
    parameters:
      port: 8080
    assertions:
      - type: state_applied
        target: configure_nginx
      - type: file_contains
        path: /etc/nginx/nginx.conf
        content: "listen 8080"
teardown:
  - cleanup: temp_files
```

**Assertion Types**:

- `no_failures`: No state application failures
- `state_applied`: Specific state was applied
- `state_changed`: Specific state made changes
- `file_exists`: File exists at path
- `file_contains`: File contains content
- `file_mode`: File has specific permissions
- `directory_exists`: Directory exists
- `command_succeeds`: Command exits with 0
- `command_fails`: Command exits non-zero
- `command_output`: Command output matches
- `idempotent`: Running twice produces same result

**Examples**:

```bash
# Run all tests
kscorectl blueprint test

# Verbose output
kscorectl blueprint test -v

# Run specific test pattern
kscorectl blueprint test --pattern "validation*"

# Run only tagged tests
kscorectl blueprint test --tags "quick,smoke"

# Exclude integration tests
kscorectl blueprint test --exclude-tags "integration,slow"

# JUnit XML output for CI
kscorectl blueprint test --format junit -o results.xml

# Parallel execution
kscorectl blueprint test --parallel --max-parallel 8

# Dry run for validation
kscorectl blueprint test --dry-run
```

**Output (Text)**:

```
Running blueprint tests from ./tests

Suite: Basic Tests (3 tests)
  ✓ default parameters (125ms)
  ✓ custom port (203ms)
  ○ network config (skipped)

Suite: Validation Tests (2 tests)
  ✓ invalid port rejected (45ms)
  ✗ missing required param (89ms)
    Error: Expected validation error, got success

═══════════════════════════════════════════
Test Summary
═══════════════════════════════════════════
  Total:   5
  Passed:  3
  Failed:  1
  Skipped: 1
  Errors:  0
  Duration: 462ms
  Pass Rate: 75.0%
═══════════════════════════════════════════

✗ 1 test(s) failed
```

### blueprint search

Search for blueprints in registries.

```bash
kscorectl blueprint search [query] [flags]
```

**Arguments**:

- `[query]`: Search query (optional, lists all if empty)

**Flags**:

- `--registry string`: Registry URL (default: `KSCORE_BLUEPRINT_REGISTRY` or `https://blueprints.keystone-core.io`)
- `--category string`: Filter by category
- `--tags strings`: Filter by tags (comma-separated)
- `--limit int`: Maximum results to show (default: 20)
- `--offset int`: Offset for pagination
- `--verified`: Only show verified blueprints
- `--sort-by string`: Sort field: name, downloads, updated, created (default: downloads)
- `--sort-order string`: Sort order: asc, desc (default: desc)
- `--json`: Output as JSON

**Examples**:

```bash
# Search for nginx blueprints
kscorectl blueprint search nginx

# Filter by category
kscorectl blueprint search --category web

# Filter by tags
kscorectl blueprint search --tags "production,ha"

# Only verified blueprints
kscorectl blueprint search --verified

# Custom registry
kscorectl blueprint search nginx --registry https://blueprints.example.com

# Paginated results
kscorectl blueprint search --limit 50 --offset 100

# JSON output
kscorectl blueprint search nginx --json
```

**Output**:

```
Search results for "nginx":

NAME                    VERSION   DOWNLOADS   DESCRIPTION
community/nginx         2.1.0     45,231      Production Nginx with SSL
community/nginx-proxy   1.5.2     12,847      Nginx reverse proxy
myorg/nginx-lb          3.0.0     1,203       Nginx load balancer

Showing 3 of 3 results
```

### blueprint info

Show detailed information about a blueprint.

```bash
kscorectl blueprint info <blueprint> [flags]
```

**Arguments**:

- `<blueprint>`: Blueprint name (e.g., community/nginx)

**Flags**:

- `--registry string`: Registry URL (default: `KSCORE_BLUEPRINT_REGISTRY` or `https://blueprints.keystone-core.io`)
- `--version string`: Specific version (default: latest)
- `--json`: Output as JSON

**Examples**:

```bash
# Show info for latest version
kscorectl blueprint info community/nginx

# Show info for specific version
kscorectl blueprint info community/nginx --version 2.0.0

# JSON output
kscorectl blueprint info community/nginx --json
```

**Output**:

```
Name:        community/nginx
Version:     2.1.0
Description: Production-ready Nginx web server deployment
Author:      Community Maintainers
License:     Apache-2.0
Categories:  web, proxy
Keywords:    nginx, web, reverse-proxy, ssl

Parameters:
  port (integer)     - Listen port (default: 80)
  ssl_enabled (bool) - Enable SSL (default: false)
  worker_count (int) - Worker processes (default: auto)

Dependencies:
  - community/ssl-certs@^1.0.0 (optional)

Downloads:   45,231
Created:     2024-01-15
Updated:     2024-06-20

Install: kscorectl blueprint install community/nginx@2.1.0
```

### blueprint install

Install blueprints from a registry.

Blueprint archive entries larger than 256 MB are rejected to protect against decompression bombs.

```bash
kscorectl blueprint install <blueprint[@version]>... [flags]
```

**Arguments**:

- `<blueprint[@version]>`: One or more blueprints to install (version optional)

**Flags**:

- `--registry string`: Registry URL (default: `KSCORE_BLUEPRINT_REGISTRY` or `https://blueprints.keystone-core.io`)
- `--dir string`: Installation directory (default: ~/.keystone-core/blueprints)
- `--verify`: Verify signature before installing (default: true)
- `--force`: Overwrite if already installed
- `--dry-run`: Show what would be installed
- `--no-deps`: Don't install dependencies

**Examples**:

```bash
# Install latest version
kscorectl blueprint install community/nginx

# Install specific version
kscorectl blueprint install community/nginx@2.1.0

# Install multiple blueprints
kscorectl blueprint install community/nginx@2.1.0 community/mysql@8.0.0

# Install without dependencies
kscorectl blueprint install community/nginx --no-deps

# Dry run
kscorectl blueprint install community/nginx --dry-run

# Force reinstall
kscorectl blueprint install community/nginx --force

# Skip signature verification (not recommended)
kscorectl blueprint install community/nginx --verify=false
```

**Output**:

```
Installing community/nginx@2.1.0...
  Downloading community/nginx@2.1.0 (245 KB)
  Verifying signature...
  Installing dependency: community/ssl-certs@1.2.0
  Extracting to ~/.keystone-core/blueprints/community/nginx

✓ Installed community/nginx@2.1.0
```

### blueprint update

Update installed blueprints to newer versions.

```bash
kscorectl blueprint update [blueprint...] [flags]
```

**Arguments**:

- `[blueprint...]`: Blueprints to update (default: all installed)

**Flags**:

- `--registry string`: Registry URL
- `--dir string`: Blueprint directory (default: ~/.keystone-core/blueprints)
- `--dry-run`: Show what would be updated
- `--major`: Allow major version updates (breaking changes)
- `--accept-breaking-changes`: Accept breaking parameter changes
- `--show-breaking`: Show breaking changes before updating

**Examples**:

```bash
# Update all blueprints (minor/patch only)
kscorectl blueprint update

# Update specific blueprint
kscorectl blueprint update community/nginx

# Allow major version updates
kscorectl blueprint update community/nginx --major

# Show what would be updated
kscorectl blueprint update --dry-run

# Show breaking changes before updating
kscorectl blueprint update --show-breaking
```

**Output**:

```
Checking for updates...

Blueprint               Current   Available   Type
community/nginx         2.1.0     2.2.1       patch
community/mysql         8.0.0     8.1.0       minor
community/redis         6.2.0     7.0.0       major (skipped)

Updating community/nginx 2.1.0 -> 2.2.1...
Updating community/mysql 8.0.0 -> 8.1.0...

✓ Updated 2 blueprints
! 1 major update available (use --major to include)
```

### blueprint remove

Remove installed blueprints.

```bash
kscorectl blueprint remove <blueprint>... [flags]
```

**Arguments**:

- `<blueprint>...`: One or more blueprints to remove

**Flags**:

- `--dir string`: Blueprint directory (default: ~/.keystone-core/blueprints)
- `--force`: Remove without confirmation
- `--dry-run`: Show what would be removed

**Examples**:

```bash
# Remove a blueprint
kscorectl blueprint remove community/nginx

# Remove multiple blueprints
kscorectl blueprint remove community/nginx community/mysql

# Force remove without confirmation
kscorectl blueprint remove community/nginx --force

# Dry run
kscorectl blueprint remove community/nginx --dry-run
```

**Output**:

```
Removing community/nginx@2.2.1...
  Removing ~/.keystone-core/blueprints/community/nginx

✓ Removed community/nginx
```

### Deprecated Commands

The following commands are deprecated and will be removed in a future version. They are being moved to separate binaries:

**Moving to kscore-blueprint-publish**:

- `blueprint publish` → `blueprint-publish publish`
- `blueprint sign` → `blueprint-publish sign`
- `blueprint verify` → `blueprint-publish verify`
- `blueprint versions` → `blueprint-publish versions`
- `blueprint docs` → `blueprint-publish docs`

**Moving to kscore-blueprint-state**:

- `blueprint rollback` → `blueprint-state rollback`
- `blueprint snapshot` → `blueprint-state snapshot`

These commands still work but display deprecation warnings. See the dedicated sections below for full documentation.

## kscore-blueprint-publish (Blueprint Publishing)

Publish blueprints to registries and manage signatures.

### Global Flags

These flags apply to all kscore-blueprint-publish commands:

- `--audit-level string`: Audit logging level: all, errors, none (default: all)
- `--audit-output string`: Audit output backend: auto, syslog, journald, stderr, none

### blueprint-publish publish

Publish a blueprint directory to a registry.

```bash
kscorectl blueprint-publish publish [path] [flags]
```

**Flags**:

- `--registry string`: Registry URL
- `--sign`: Sign before publishing (default: true)
- `--key string`: Signing key file
- `--force`: Overwrite if version exists
- `--dry-run`: Show what would be published

**Examples**:

```bash
# Publish current directory
kscorectl blueprint-publish publish .

# Publish to custom registry
kscorectl blueprint-publish publish . --registry https://blueprints.example.com
```

### blueprint-publish sign

Sign a blueprint package.

```bash
kscorectl blueprint-publish sign <file> [flags]
```

**Flags**:

- `--key string`: Signing key file
- `--generate-key`: Generate a new signing key pair
- `--output string`: Output signature file

### blueprint-publish verify

Verify a blueprint signature.

```bash
kscorectl blueprint-publish verify <blueprint[@version]|file> [flags]
```

**Flags**:

- `--key string`: Public key file for verification
- `--registry string`: Registry URL (default: `KSCORE_BLUEPRINT_REGISTRY` or `https://blueprints.keystone-core.io`)
- `--signature string`: Signature file (for local verification)

### blueprint-publish versions

List available versions of a blueprint.

```bash
kscorectl blueprint-publish versions <blueprint> [flags]
```

**Flags**:

- `--registry string`: Registry URL (default: `KSCORE_BLUEPRINT_REGISTRY` or `https://blueprints.keystone-core.io`)
- `--all`: Include prerelease versions
- `--limit int`: Maximum versions to show (default: 20)
- `--json`: Output as JSON

### blueprint-publish docs

Generate documentation from a blueprint manifest.

```bash
kscorectl blueprint-publish docs [path] [flags]
```

**Flags**:

- `-o, --output string`: Output directory (default: docs)
- `--format string`: Output format (markdown, html, json)
- `--include-usage`: Include usage examples (default: true)

## kscore-blueprint-state (Blueprint State)

Manage blueprint rollback operations and snapshots.

### Global Flags

These flags apply to all kscore-blueprint-state commands:

- `--audit-level string`: Audit logging level: all, errors, none (default: all)
- `--audit-output string`: Audit output backend: auto, syslog, journald, stderr, none

### blueprint-state rollback

Rollback a blueprint to a previous version or snapshot.

```bash
kscorectl blueprint-state rollback <blueprint> [flags]
```

**Flags**:

- `--dir string`: Blueprint directory (default: ~/.keystone-core/blueprints)
- `--dry-run`: Show what would happen without changes
- `--force`: Force rollback even with breaking changes
- `--to-version string`: Rollback to a specific version
- `--to-snapshot string`: Rollback to a snapshot ID
- `--history`: Show rollback history
- `--json`: Output in JSON format

### blueprint-state snapshot

Manage snapshots of blueprint state.

```bash
kscorectl blueprint-state snapshot <command> [flags]
```

**Commands**:

- `list [blueprint]`: List available snapshots
- `delete <snapshot-id>`: Delete a snapshot
- `info <snapshot-id>`: Show snapshot details

**Common Flags**:

- `--dir string`: Snapshot directory
- `--limit int`: Maximum snapshots to show (list)
- `--json`: Output in JSON format

### blueprint-state diff

Compare two snapshots.

```bash
kscorectl blueprint-state diff <snapshot1> <snapshot2> [flags]
```

**Flags**:

- `--dir string`: Snapshot directory
- `--json`: Output in JSON format
- `--files-only`: Show only file differences

## kscore-policy (Policy Management)

Manage and evaluate policies using OPA (Rego) and CEL for compliance, security, and operational enforcement.

### policy list

List policies from a policy file.

```bash
kscorectl policy list <policyfile> [flags]
```

**Arguments**:

- `<policyfile>`: Path to YAML file containing policy definitions

**Flags**:

- `--category string`: Filter by category (security, compliance, operational, cost, custom)
- `--type string`: Filter by type (opa, cel)
- `-o, --output string`: Output format: table, json, yaml (default: table)

**Examples**:

```bash
# List all policies from a file
kscorectl policy list policies/all.yaml

# Filter by category
kscorectl policy list policies/all.yaml --category security

# Filter by type
kscorectl policy list policies/all.yaml --type opa

# Output as JSON
kscorectl policy list policies/all.yaml --output json
```

**Output**:

```
ID                            TYPE     CATEGORY     SEVERITY   ENABLED
---------------------------------------------------------------------------
security-no-root              opa      security     high       yes
cost-limits                   cel      cost         medium     yes
required-tags                 cel      compliance   low        yes

Total: 3 policies
```

### policy validate

Validate policy syntax and structure.

```bash
kscorectl policy validate <policyfile>
```

**Arguments**:

- `<policyfile>`: Path to policy YAML file

**Validation Checks**:

- YAML syntax validation
- Required fields (id, name, type, code)
- Policy code syntax (OPA Rego or CEL)
- Valid enum values (type, category, severity)

**Examples**:

```bash
# Validate a policy file
kscorectl policy validate policies/security.yaml
```

**Output (Success)**:

```
Validating policy file: policies/security.yaml

Validating: security-no-root
  ✓ Policy code is valid
Validating: security-ssh-hardening
  ✓ Policy code is valid

=== Validation Summary ===
Policies: 2
Errors:   0
Warnings: 0

✓ All policies valid!
```

**Output (Errors)**:

```
Validating policy file: policies/broken.yaml

Validating: broken-policy
  ✗ Error: name is required
  ✗ Error: invalid policy code: rego_parse_error

=== Validation Summary ===
Policies: 1
Errors:   2
Warnings: 0

✗ Validation failed!
```

### policy check

Evaluate a policy against input data and report the result.

```bash
kscorectl policy check <policyfile> [flags]
```

**Arguments**:

- `<policyfile>`: Path to policy YAML file

**Required Flags**:

- `--policy string`: Policy ID to evaluate

**Optional Flags**:

- `--input-file string`: Input JSON file
- `--input string`: Inline input JSON
- `--action string`: Action being performed (default: "check")
- `--user string`: User performing the action
- `--context string`: Additional context as JSON
- `-o, --output string`: Output format: text, json, yaml, table (default: text)

**Examples**:

```bash
# Check a specific policy with input from file
kscorectl policy check policies/security.yaml \
  --policy security-no-root \
  --input-file input.json

# Check with inline JSON input
kscorectl policy check policies/security.yaml \
  --policy security-no-root \
  --input '{"command": "rm", "args": ["-rf", "/"]}'

# Check with action and user context
kscorectl policy check policies/security.yaml \
  --policy security-no-root \
  --input-file input.json \
  --action execute \
  --user admin
```

**Output (Allowed)**:

```
Policy: Security No Root (security-no-root)
Type:   opa

Result: ✓ ALLOWED

Duration: 1.234ms
```

**Output (Denied)**:

```
Policy: Security No Root (security-no-root)
Type:   opa

Result: ✗ DENIED

Violations (2):
  1. [high] Running commands as root is prohibited
     Remediation: Use a non-root user or sudo with specific permissions
  2. [medium] Destructive commands require approval

Duration: 2.567ms
```

### policy show

Display detailed information about a specific policy.

```bash
kscorectl policy show <policyfile> <policyid> [flags]
```

**Arguments**:

- `<policyfile>`: Path to policy YAML file
- `<policyid>`: Policy ID to display

**Flags**:

- `-o, --output string`: Output format: text, json, yaml, table (default: text)

**Examples**:

```bash
# Show policy details
kscorectl policy show policies/security.yaml security-no-root

# Output as YAML
kscorectl policy show policies/security.yaml security-no-root --output yaml
```

**Output**:

```
ID:              security-no-root
Name:            Security No Root
Description:     Prevents running commands as root user
Type:            opa
Category:        security
Severity:        high
Enforcement:     enforce
Enabled:         true
Tags:            security, production

Code:
------------------------------------------------------------
package security

default allow = false

allow {
    input.user != "root"
}
------------------------------------------------------------
```

### policy create

Create a new policy and add it to a policy file.

```bash
kscorectl policy create <policyfile> [flags]
```

**Arguments**:

- `<policyfile>`: Path to policy YAML file (created if doesn't exist)

**Required Flags**:

- `--name string`: Policy name/ID

**Optional Flags**:

- `--description string`: Policy description
- `--type string`: Policy type: opa, cel (default: opa)
- `--category string`: Policy category: security, compliance, operational, cost, custom (default: custom)
- `--severity string`: Severity: low, medium, high, critical (default: medium)
- `--mode string`: Enforcement mode: enforce, audit, warn (default: enforce)
- `--tags strings`: Policy tags (comma-separated)
- `--code string`: Policy code (inline)
- `--code-file string`: Policy code from file

**Examples**:

```bash
# Create a policy with inline code
kscorectl policy create policies/security.yaml --name deny-privileged \
  --type opa --category security --severity high \
  --code 'package security
default allow = false
allow { not input.privileged }'

# Create a policy with code from a file
kscorectl policy create policies/security.yaml --name deny-privileged \
  --type opa --category security --severity high --code-file policy.rego

# Create a CEL policy
kscorectl policy create policies/security.yaml --name require-labels \
  --type cel --category operational --severity medium \
  --code 'has(resource.labels) && size(resource.labels) > 0'
```

**Output**:

```
✓ Policy 'deny-privileged' created successfully
  Type:     opa
  Category: security
  Severity: high
  Mode:     enforce
  File:     policies/security.yaml
```

### policy update

Update an existing policy in a policy file.

```bash
kscorectl policy update <policyfile> <policyid> [flags]
```

**Arguments**:

- `<policyfile>`: Path to policy YAML file
- `<policyid>`: Policy ID to update

**Flags**:

- `--description string`: New description
- `--severity string`: New severity: low, medium, high, critical
- `--mode string`: New enforcement mode: enforce, audit, warn
- `--tags strings`: New tags (comma-separated)
- `--code string`: New policy code (inline)
- `--code-file string`: New policy code from file

**Examples**:

```bash
# Update policy severity
kscorectl policy update policies/security.yaml deny-privileged --severity critical

# Update policy code from file
kscorectl policy update policies/security.yaml deny-privileged --code-file updated.rego

# Update enforcement mode
kscorectl policy update policies/security.yaml deny-privileged --mode audit

# Update description and tags
kscorectl policy update policies/security.yaml deny-privileged \
  --description "Updated description" --tags security,critical
```

**Output**:

```
✓ Policy 'deny-privileged' updated successfully
```

### policy delete

Delete a policy from a policy file.

```bash
kscorectl policy delete <policyfile> <policyid> [flags]
```

**Arguments**:

- `<policyfile>`: Path to policy YAML file
- `<policyid>`: Policy ID to delete

**Flags**:

- `-f, --force`: Skip confirmation prompt

**Examples**:

```bash
# Delete a policy (prompts for confirmation)
kscorectl policy delete policies/security.yaml deny-privileged

# Force delete without confirmation
kscorectl policy delete policies/security.yaml deny-privileged --force
```

**Output**:

```
Delete policy 'deny-privileged' from policies/security.yaml? [y/N]: y
✓ Policy 'deny-privileged' deleted successfully
```

### policy activate

Activate (enable) a policy.

```bash
kscorectl policy activate <policyfile> <policyid> [flags]
```

**Arguments**:

- `<policyfile>`: Path to policy YAML file
- `<policyid>`: Policy ID to activate

**Flags**:

- `--mode string`: Enforcement mode: enforce, audit, warn

**Examples**:

```bash
# Activate a policy
kscorectl policy activate policies/security.yaml deny-privileged

# Activate with specific enforcement mode
kscorectl policy activate policies/security.yaml deny-privileged --mode enforce
```

**Output**:

```
✓ Policy 'deny-privileged' activated
  Mode: enforce
```

### policy deactivate

Deactivate (disable) a policy.

```bash
kscorectl policy deactivate <policyfile> <policyid>
```

**Aliases**: `policy disable`

**Arguments**:

- `<policyfile>`: Path to policy YAML file
- `<policyid>`: Policy ID to deactivate

**Examples**:

```bash
# Deactivate a policy
kscorectl policy deactivate policies/security.yaml deny-privileged

# Using the alias
kscorectl policy disable policies/security.yaml deny-privileged
```

**Output**:

```
✓ Policy 'deny-privileged' deactivated
```

### policy audit

> **Deprecated**: This command is moving to `kscore-audit`. Use `kscorectl audit log` instead.

Display the policy evaluation audit log.

```bash
kscorectl policy audit [flags]
```

**Flags**:

- `--policy string`: Filter by policy ID
- `--resource-type string`: Filter by resource type
- `--denied`: Show only denied evaluations
- `--limit int`: Maximum entries to show (default: 100)
- `-o, --output string`: Output format: text, json, yaml, table (default: table)

**Examples**:

```bash
# Show recent audit entries
kscorectl policy audit

# Filter by policy
kscorectl policy audit --policy security-no-root

# Show only denied evaluations
kscorectl policy audit --denied

# Limit results
kscorectl policy audit --limit 50
```

**Output**:

```
TIMESTAMP            POLICY                    RESOURCE        RESULT     VIOLATIONS
-------------------------------------------------------------------------------------
2024-01-15 10:30:45  security-no-root         command         DENIED     2
2024-01-15 10:29:30  security-ssh-hardening   sshd_config     ALLOWED    0
2024-01-15 10:28:15  required-tags            deployment      DENIED     1

Total: 3 entries
```

### policy report

Generate a compliance report based on policy evaluations.

```bash
kscorectl policy report [flags]
```

**Flags**:

- `--days int`: Number of days to include in report (default: 7)
- `-o, --output string`: Output format: text, json, yaml, table (default: text)

**Examples**:

```bash
# Generate report for last 7 days
kscorectl policy report --days 7

# Generate report for last 30 days as JSON
kscorectl policy report --days 30 --output json
```

**Output**:

```
=== Compliance Report ===
Generated: 2024-01-15T10:30:45Z
Period:    2024-01-08 to 2024-01-15

Summary:
  Total Policies:     12
  Compliant:          10
  Violating:          2
  Compliance Rate:    83.3%

Violations by Severity:
  critical  : 0
  high      : 5
  medium    : 8
  low       : 3

Top Violations:
  1. security-no-root (high) - 5 violations
  2. required-tags (low) - 3 violations
```

## kscore-audit (Audit and Compliance)

Review policy evaluation logs, generate compliance reports, export audit data, and inspect trends.

> All commands connect to the control plane via gRPC PolicyService.
> Use `--server` to specify the server address (default: localhost:9090).

### Global Flags

These flags apply to all kscore-audit commands:

- `--server string`: Control plane server address (default: localhost:9090)
- `-o, --format string`: Output format: table, text, json, yaml (default: table)
- `--audit-level string`: Audit logging level: all, errors, none (default: all)
- `--audit-output string`: Audit output backend: auto, syslog, journald, stderr, none

### audit log

View policy evaluation audit entries.

```bash
kscorectl audit log [flags]
```

**Flags**:

- `--policy string`: Filter by policy ID
- `--resource-type string`: Filter by resource type
- `--denied`: Show only denied evaluations
- `--limit int`: Maximum entries to show (default: 100)
- `--since string`: Show entries since date (YYYY-MM-DD)
- `--until string`: Show entries until date (YYYY-MM-DD)
- `-o, --format string`: Output format: table, text, json, yaml (default: table)

**Examples**:

```bash
# Show recent audit entries
kscorectl audit log

# Filter by policy
kscorectl audit log --policy security-no-root

# Show only denied evaluations
kscorectl audit log --denied

# Filter by time range
kscorectl audit log --since 2026-01-01 --until 2026-01-17
```

### audit report

Generate a compliance report from policy evaluations.

```bash
kscorectl audit report [flags]
```

**Flags**:

- `--days int`: Number of days to include (default: 7)
- `--category string`: Filter by policy category
- `--severity string`: Filter by severity (low, medium, high, critical)
- `-o, --format string`: Output format: table, text, json, yaml (default: table)

**Examples**:

```bash
# Generate report for last 7 days
kscorectl audit report --days 7

# Generate report as JSON
kscorectl audit report --days 30 --format json
```

### audit export

Export audit data for external analysis or archiving.

```bash
kscorectl audit export [flags]
```

**Flags**:

- `--days int`: Number of days to export (default: 30)
- `--output string`: Output file path (default: stdout)
- `--export-format string`: Export format (json, csv, yaml) (default: json)

**Examples**:

```bash
# Export last 30 days to JSON
kscorectl audit export --days 30 --output audit-data.json

# Export to CSV
kscorectl audit export --days 7 --output audit-data.csv --export-format csv
```

### audit stats

View audit statistics and trends.

```bash
kscorectl audit stats [flags]
```

**Flags**:

- `--days int`: Number of days to analyze (default: 7)
- `-o, --format string`: Output format: table, text, json, yaml (default: table)

**Examples**:

```bash
# Show stats for last 7 days
kscorectl audit stats --days 7

# Show stats for last 30 days
kscorectl audit stats --days 30 --format json
```

### audit search

Search policy evaluation audit entries with flexible filters via PolicyService gRPC.

```bash
kscorectl audit search [flags]
```

**Flags**:

- `--policy string`: Filter by policy ID
- `--user string`: Filter by username
- `--action string`: Filter by action
- `--since string`: Show entries since duration (e.g., '7d', '24h')
- `--output string`: Output file path (default: stdout)
- `--denied`: Show only denied evaluations
- `--limit int`: Maximum entries to return (default: 100)

**Aliases**: `query` — `kscorectl audit query` is equivalent to `kscorectl audit search`.

**Examples**:

```bash
# Search for denied evaluations
kscorectl audit search --denied --since "7d"

# Search by policy and user
kscorectl audit search --policy "security-no-root" --user "admin"

# Query with output (using query alias)
kscorectl audit query --policy "cis-benchmark" --output /tmp/results.json

# Limit results
kscorectl audit search --limit 10
```

### audit analyze

Analyze audit data for anomalies against a historical baseline.

> **Status**: Not yet available. Requires server-side analytics infrastructure.

```bash
kscorectl audit analyze [flags]
```

**Flags**:

- `--input string`: Input file glob pattern (e.g., '/tmp/\*.json')
- `--baseline string`: Baseline period for comparison (default: 30d)
- `--output string`: Output file for analysis results

### audit timeline

Generate a chronological incident timeline from audit events.

```bash
kscorectl audit timeline [flags]
```

**Flags**:

- `--from string`: Start time (RFC3339 or human-readable)
- `--to string`: End time (RFC3339 or human-readable)
- `--output string`: Output file path (use .html for HTML format)

**Examples**:

```bash
# Generate timeline for a time range
kscorectl audit timeline --from "2026-01-01T00:00:00Z" --to "2026-01-02T00:00:00Z"

# Output as HTML
kscorectl audit timeline --from "2026-01-01T00:00:00Z" --to "2026-01-02T00:00:00Z" --output incident-timeline.html
```

### audit watch

Monitor audit log events in real-time with optional filters. Press Ctrl+C to stop.

> **Status**: Not yet available. Requires streaming RPC infrastructure.

```bash
kscorectl audit watch [flags]
```

**Flags**:

- `--type string`: Event type filter
- `--status string`: Filter by status
- `--agent string`: Filter by agent ID
- `--user string`: Filter by username
- `--api-key string`: Filter by API key name
- `--interval duration`: Polling interval (default: 2s)

## kscore-gitops (GitOps Management)

Manage GitOps deployments, verifications, rollbacks, and promotions with ArgoCD, Flux, GitHub, and GitLab integrations.

> **API Status**: The `rollback` command connects to the server REST API (`POST /api/v1/gitops/rollback`). The `verify` command runs locally. All other commands (`promote`, `status`, `repo *`, `deploy *`, `git-sync *`) require server-side API endpoints that are not yet implemented (Epic 49) and will return "not yet available" errors.

### gitops verify

Execute a verification workflow to validate deployments.

```bash
kscorectl gitops verify <workflow-file> [flags]
```

**Arguments**:

- `<workflow-file>`: Path to verification workflow YAML file

**Flags**:

- `--parallel`: Run steps in parallel
- `--timeout string`: Workflow timeout (default: 2m)
- `-o, --output string`: Output format: text, json, yaml, table (default: text)

**Workflow File Format**:

```yaml
name: post-deploy-checks
description: Verify deployment health
parallel: false
timeout: 5m
steps:
  - name: Health Check
    type: http
    timeout: 30s
    retries: 3
    retry_delay: 5s
    config:
      url: http://app.example.com/health
      expected_status: 200
  - name: Database Connection
    type: command
    config:
      command: pg_isready -h localhost
      expected_exit_code: 0
```

**Examples**:

```bash
# Run a verification workflow
kscorectl gitops verify workflows/post-deploy.yaml

# Run with parallel steps
kscorectl gitops verify workflows/health-check.yaml --parallel

# Run with custom timeout
kscorectl gitops verify workflows/full-check.yaml --timeout 5m
```

**Output**:

```
Running verification workflow: post-deploy-checks
Description: Verify deployment health
Steps: 2
Mode: sequential

=== Step Results ===
✓ Health Check: OK (245ms)
✓ Database Connection: OK (123ms)

=== Summary ===
Total Steps:  2
Passed:       2
Failed:       0
Duration:     368ms

✓ Verification passed!
```

### gitops rollback

Trigger a rollback to restore a previous deployment state.

```bash
kscorectl gitops rollback [flags]
```

**Required Flags**:

- `--app string`: Application name

**Optional Flags**:

- `--namespace string`: Namespace (default: default)
- `--type string`: Rollback type: argocd, flux, git (default: argocd)
- `--strategy string`: Strategy: previous, specific, last_known_good (default: previous)
- `--revision string`: Target revision (required for specific strategy)
- `--reason string`: Reason for rollback
- `--user string`: User performing rollback
- `--dry-run`: Simulate rollback without executing
- `-o, --output string`: Output format: text, json, yaml, table (default: text)

**Rollback Strategies**:

- `previous`: Rollback to immediately previous revision
- `specific`: Rollback to a specific revision (requires --revision)
- `last_known_good`: Rollback to last known healthy state

**Examples**:

```bash
# Rollback to previous revision
kscorectl gitops rollback --app myapp --strategy previous

# Rollback to specific revision
kscorectl gitops rollback --app myapp \
  --strategy specific \
  --revision abc123

# Rollback with ArgoCD
kscorectl gitops rollback --app myapp \
  --type argocd \
  --namespace production \
  --reason "Deployment caused errors"

# Dry-run rollback
kscorectl gitops rollback --app myapp --strategy previous --dry-run
```

**Output**:

```
Rollback Configuration
======================
Application:  myapp
Namespace:    production
Type:         argocd
Strategy:     previous
Reason:       Deployment caused errors

=== Rollback Result ===
ID:       rb-1705312245123456789
Status:   completed
From:     abc123
To:       xyz789
Duration: 30s
Message:  Rollback completed successfully

✓ Rollback completed!
```

### gitops promote

Promote a deployment from one environment to another.

```bash
kscorectl gitops promote [flags]
```

**Required Flags**:

- `--pipeline string`: Pipeline name
- `--from string`: Source environment
- `--to string`: Target environment

**Optional Flags**:

- `--revision string`: Specific revision to promote
- `--reason string`: Reason for promotion
- `--user string`: User performing promotion
- `--skip-verify`: Skip verification step
- `--force`: Force promotion even if checks fail
- `--dry-run`: Simulate promotion without executing
- `-o, --output string`: Output format: text, json, yaml, table (default: text)

**Examples**:

```bash
# Promote from staging to production
kscorectl gitops promote \
  --pipeline prod-pipeline \
  --from staging \
  --to production

# Promote with verification skip
kscorectl gitops promote \
  --pipeline prod-pipeline \
  --from staging \
  --to production \
  --skip-verify

# Promote specific revision
kscorectl gitops promote \
  --pipeline prod-pipeline \
  --from staging \
  --to production \
  --revision abc123

# Dry-run promotion
kscorectl gitops promote \
  --pipeline prod-pipeline \
  --from staging \
  --to production \
  --dry-run
```

**Output**:

```
Promotion Request
=================
Pipeline:    prod-pipeline
From:        staging
To:          production
Revision:    abc123

=== Promotion Result ===
ID:       promo-1705312245123456789
Status:   completed
Duration: 1m30s
Message:  Promotion completed successfully

Stages:
  1. ✓ production: completed (45s)

✓ Promotion completed!
```

### gitops webhook

Manage webhook handlers for GitOps integrations.

#### webhook list

List all registered webhook handlers.

```bash
kscorectl gitops webhook list
```

**Output**:

```
Registered Webhook Handlers
===========================

ARGOCD
  Description: ArgoCD application events
  Events:      sync, health, deployment

FLUX
  Description: Flux reconciliation events
  Events:      kustomization, helmrelease, gitrepository

GITHUB
  Description: GitHub repository events
  Events:      deployment, workflow_run, push

GITLAB
  Description: GitLab project events
  Events:      deployment, pipeline, push

Webhook Endpoint: POST /webhooks/<type>
```

#### webhook test

Display sample webhook payloads for testing.

```bash
kscorectl gitops webhook test <type>
```

**Arguments**:

- `<type>`: Webhook type: argocd, flux, github, gitlab

**Examples**:

```bash
# Test ArgoCD webhook
kscorectl gitops webhook test argocd

# Test GitHub webhook
kscorectl gitops webhook test github
```

**Output**:

```
Test Webhook: argocd

Sample Payload:
---------------
{
  "type": "sync",
  "application": {
    "name": "test-app",
    "namespace": "argocd"
  },
  "status": {
    "sync": "Synced",
    "health": "Healthy"
  }
}

To test, send this payload to:
  POST http://<server>:<port>/webhooks/argocd
```

### gitops status

Display status of GitOps operations.

```bash
kscorectl gitops status [flags]
```

**Flags**:

- `--type string`: Status type: rollbacks, promotions, verifications, all (default: all)
- `--limit int`: Maximum entries to show (default: 10)
- `-o, --output string`: Output format: text, json, yaml, table (default: text)

**Examples**:

```bash
# Show recent rollbacks
kscorectl gitops status --type rollbacks

# Show recent promotions
kscorectl gitops status --type promotions --limit 20

# Show all operations as JSON
kscorectl gitops status --output json
```

**Output**:

```
ID           TYPE         STATUS       TARGET                         TIME                 DURATION
----------------------------------------------------------------------------------------------------
rb-001       rollback     completed    myapp/production               2024-01-15 08:30:45  45s
promo-001    promotion    completed    myapp: staging → production    2024-01-15 05:30:45  1m30s
verify-001   verification passed       post-deploy-checks             2024-01-15 05:00:45  30s

Total: 3 operations
```

### gitops repo

Manage Git repositories for GitOps operations.

#### repo list

List all Git repositories configured for GitOps operations.

```bash
kscorectl gitops repo list [flags]
```

**Flags**:

- `-o, --output string`: Output format: text, json, yaml, table (default: text)

**Examples**:

```bash
# List all repositories
kscorectl gitops repo list

# List repositories as JSON
kscorectl gitops repo list --output json
```

**Output**:

```
Configured Repositories
=======================

states
  URL:       git@github.com:org/kscore-states.git
  Branch:    main
  Path:      /states
  Auth:      ssh
  Status:    synced
  Last Sync: 2024-01-15T08:00:00Z
  Commit:    abc1234

blueprints
  URL:       https://github.com/org/kscore-blueprints.git
  Branch:    production
  Path:      /blueprints
  Auth:      token
  Status:    synced
  Last Sync: 2024-01-15T06:30:00Z
  Commit:    def5678

Total: 2 repositories
```

#### repo add

Add a new Git repository for GitOps operations.

```bash
kscorectl gitops repo add <name> [flags]
```

**Arguments**:

- `<name>`: Repository name

**Required Flags**:

- `--url string`: Repository URL

**Optional Flags**:

- `--branch string`: Branch to track (default: main)
- `--path string`: Path within repository
- `--auth string`: Authentication method: none, ssh, token (default: none)
- `--key string`: SSH key path (for --auth ssh)

**Examples**:

```bash
# Add a repository with SSH
kscorectl gitops repo add myrepo \
  --url git@github.com:org/repo.git \
  --auth ssh \
  --key ~/.ssh/id_rsa

# Add a repository with HTTPS
kscorectl gitops repo add myrepo \
  --url https://github.com/org/repo.git \
  --auth token

# Add with specific branch and path
kscorectl gitops repo add myrepo \
  --url git@github.com:org/repo.git \
  --branch main \
  --path /states
```

#### repo remove

Remove a Git repository from GitOps operations.

```bash
kscorectl gitops repo remove <name>
```

**Aliases**: `rm`, `delete`

**Arguments**:

- `<name>`: Repository name to remove

**Examples**:

```bash
# Remove a repository
kscorectl gitops repo remove myrepo
```

#### repo sync

Synchronize a Git repository, pulling latest changes.

```bash
kscorectl gitops repo sync <name> [flags]
```

**Arguments**:

- `<name>`: Repository name to synchronize

**Flags**:

- `--force`: Force sync, discarding local changes

**Examples**:

```bash
# Sync a repository
kscorectl gitops repo sync myrepo

# Force sync (discard local changes)
kscorectl gitops repo sync myrepo --force
```

### gitops deploy

Manage GitOps deployments across environments.

#### deploy list

List recent deployments across environments.

```bash
kscorectl gitops deploy list [flags]
```

**Flags**:

- `--env string`: Filter by environment
- `--app string`: Filter by application
- `--limit int`: Maximum entries to show (default: 10)
- `-o, --output string`: Output format: text, json, yaml, table (default: text)

**Examples**:

```bash
# List all recent deployments
kscorectl gitops deploy list

# List deployments for specific environment
kscorectl gitops deploy list --env production

# List deployments for specific application
kscorectl gitops deploy list --app myapp

# List as JSON with custom limit
kscorectl gitops deploy list --output json --limit 20
```

**Output**:

```
ID           APP          ENV          STATUS              REVISION             TIME
-----------------------------------------------------------------------------------------------
deploy-001   frontend     production   succeeded           v1.5.2               2024-01-15 08:30:45
deploy-002   backend      staging      succeeded           v2.1.0-rc1           2024-01-15 05:30:45
deploy-003   backend      production   pending_approval    v2.1.0-rc1           2024-01-15 10:00:45

Total: 3 deployments
```

#### deploy show

Show detailed information about a specific deployment.

```bash
kscorectl gitops deploy show <deployment-id> [flags]
```

**Arguments**:

- `<deployment-id>`: Deployment ID

**Flags**:

- `-o, --output string`: Output format: text, json, yaml, table (default: text)

**Examples**:

```bash
# Show deployment details
kscorectl gitops deploy show deploy-001

# Show as JSON
kscorectl gitops deploy show deploy-001 --output json
```

**Output**:

```
Deployment Details
==================
ID:          deploy-001
Application: frontend
Environment: production
Revision:    v1.5.2
Status:      succeeded
Start Time:  2024-01-15T08:30:45Z
End Time:    2024-01-15T08:33:15Z
Duration:    2m30s
Deployer:    gitops-bot
Message:     Auto-deployed from main branch
```

#### deploy rollback

Rollback a specific deployment to its previous state.

```bash
kscorectl gitops deploy rollback <deployment-id>
```

**Arguments**:

- `<deployment-id>`: Deployment ID to rollback

**Examples**:

```bash
# Rollback a deployment
kscorectl gitops deploy rollback deploy-001
```

**Output**:

```
Rolling back deployment: deploy-001

Finding previous revision...
Previous revision: v1.5.1
Initiating rollback...

✓ Rollback initiated for deployment 'deploy-001'

Use 'kscorectl gitops status --type rollbacks' to monitor progress.
```

#### deploy approve

Approve a pending deployment that requires manual approval.

```bash
kscorectl gitops deploy approve <deployment-id> [flags]
```

**Arguments**:

- `<deployment-id>`: Deployment ID to approve

**Flags**:

- `-f, --force`: Skip confirmation prompt

**Examples**:

```bash
# Approve a pending deployment
kscorectl gitops deploy approve deploy-003

# Force approve (skip confirmation)
kscorectl gitops deploy approve deploy-003 --force
```

**Output**:

```
Approving deployment: deploy-003

Deployment Details:
  Application: backend
  Environment: production
  Revision:    v2.1.0-rc1

Approve this deployment? [y/N]: y

✓ Deployment 'deploy-003' approved

Deployment is now proceeding.
Use 'kscorectl gitops deploy show deploy-003' to monitor status.
```

## kscore-webhook (Webhook Management)

Manage webhook handlers, test payloads, delivery history, and secrets for GitOps integrations.

> **API Status**: Outbound webhook commands (`outbound list/create/show/delete/history/test`) are fully wired to the server REST API. Inbound webhook commands (`list`, `show`, `history`, `secrets list`, `secrets rotate`) require server-side inbound webhook management endpoints that are not yet implemented and will return "not yet available" errors. The `test` command sends a real HTTP POST to the configured webhook endpoint.

### Global Flags

These flags apply to all kscore-webhook commands:

- `--server string`: Control plane server address (default: localhost:9090)
- `-o, --format string`: Output format: table, text, json, yaml (default: table)
- `--audit-level string`: Audit logging level: all, errors, none (default: all)
- `--audit-output string`: Audit output backend: auto, syslog, journald, stderr, none

### webhook list

List registered webhook handlers.

```bash
kscorectl webhook list [flags]
```

**Examples**:

```bash
# List all webhook handlers
kscorectl webhook list

# Output as JSON
kscorectl webhook list --format json
```

### webhook show

Show details of a webhook handler.

```bash
kscorectl webhook show <type> [flags]
```

**Arguments**:

- `<type>`: argocd, flux, github, gitlab

**Examples**:

```bash
# Show ArgoCD webhook details
kscorectl webhook show argocd

# Show GitHub webhook details as JSON
kscorectl webhook show github --format json
```

### webhook test

Generate a sample payload to test a webhook endpoint.

```bash
kscorectl webhook test <type>
```

**Arguments**:

- `<type>`: argocd, flux, github, gitlab

**Examples**:

```bash
# Test ArgoCD webhook
kscorectl webhook test argocd
```

### webhook history

View webhook delivery history.

```bash
kscorectl webhook history [flags]
```

**Flags**:

- `--limit int`: Maximum entries to show (default: 20)
- `-o, --format string`: Output format: table, text, json, yaml (default: table)

**Examples**:

```bash
# View recent webhook deliveries
kscorectl webhook history

# Limit results
kscorectl webhook history --limit 50
```

### webhook secrets

Manage webhook secrets.

```bash
kscorectl webhook secrets <command> [flags]
```

**Commands**:

- `list`: List webhook secrets
- `rotate <name>`: Rotate a webhook secret

**Examples**:

```bash
# List webhook secrets
kscorectl webhook secrets list

# Rotate a secret
kscorectl webhook secrets rotate github-webhook-secret
```

### webhook outbound

Manage outbound webhook subscriptions that deliver Keystone Core events to external HTTP endpoints.

```bash
kscorectl webhook outbound <command> [flags]
```

**Commands**:

- `list`: List all outbound subscriptions
- `create`: Create a new subscription
- `show <id>`: Show subscription details
- `delete <id>`: Delete a subscription
- `history <id>`: View delivery history
- `test <id>`: Send a test event

#### webhook outbound list

List all outbound webhook subscriptions.

```bash
kscorectl webhook outbound list [flags]
```

**Flags**:

- `-o, --format string`: Output format: table, json (default: table)

**Examples**:

```bash
# List subscriptions in table format
kscorectl webhook outbound list

# List as JSON
kscorectl webhook outbound list --format json
```

#### webhook outbound create

Create a new outbound webhook subscription.

```bash
kscorectl webhook outbound create [flags]
```

**Flags**:

- `--name string`: Subscription name (required)
- `--url string`: Destination URL (required)
- `--events strings`: Event patterns to match (required, comma-separated)
- `--secret string`: HMAC signing secret
- `--max-retries int`: Max retry attempts (default: 3)
- `--timeout int`: HTTP timeout in seconds (default: 10)

**Examples**:

```bash
# Create a subscription for all agent events
kscorectl webhook outbound create --name alerts --url https://hooks.slack.com/services/... --events "agent.*"

# Create with signing secret and multiple event patterns
kscorectl webhook outbound create --name monitoring \
  --url https://example.com/webhook \
  --events "agent.*,state.drift.*" \
  --secret my-secret \
  --max-retries 5
```

#### webhook outbound show

Show details of an outbound webhook subscription.

```bash
kscorectl webhook outbound show <id>
```

**Examples**:

```bash
kscorectl webhook outbound show sub_1234567890
```

#### webhook outbound delete

Delete an outbound webhook subscription and its delivery history.

```bash
kscorectl webhook outbound delete <id>
```

**Examples**:

```bash
kscorectl webhook outbound delete sub_1234567890
```

#### webhook outbound history

View delivery history for a subscription.

```bash
kscorectl webhook outbound history <id> [flags]
```

**Flags**:

- `--limit int`: Maximum entries to show (default: 50)

**Examples**:

```bash
# View recent deliveries
kscorectl webhook outbound history sub_1234567890

# Limit to last 10 deliveries
kscorectl webhook outbound history sub_1234567890 --limit 10
```

#### webhook outbound test

Send a test event to a subscription's URL and display the result.

```bash
kscorectl webhook outbound test <id>
```

**Examples**:

```bash
kscorectl webhook outbound test sub_1234567890
```

## kscore-cluster (Cluster Management)

Manage high-availability cluster operations. Only available when running in HA cluster mode.

### cluster status

Display cluster status and health.

```bash
kscorectl cluster status [flags]
```

**Flags**:

- `-o, --output string`: Output format (table, json, yaml) (default: table)
- `-w, --watch`: Watch for changes

**Output**:

```
Cluster: kscore-prod
Status:  healthy
Quorum:  yes (2/3)
Leader:  server-1

MEMBER     STATUS    AGENTS  LAST HEARTBEAT
server-1   healthy   50      2024-01-15T10:30:45Z (leader)
server-2   healthy   48      2024-01-15T10:30:44Z
server-3   healthy   52      2024-01-15T10:30:45Z
```

### cluster members

List cluster members with details.

```bash
kscorectl cluster members [flags]
```

**Flags**:

- `-o, --output string`: Output format (table, json, yaml)
- `--details`: Show detailed member information

**Example**:

```bash
kscorectl cluster members --output json
kscorectl cluster members --details
```

### cluster leader

Show current cluster leader.

```bash
kscorectl cluster leader
```

**Output**:

```
Leader: server-1
Address: 192.168.1.10:5000
Elected: 2024-01-14T08:00:00Z (1d 2h 30m ago)
```

### cluster health

Perform cluster health check.

```bash
kscorectl cluster health [flags]
```

**Flags**:

- `-v, --verbose`: Show detailed health information

**Output**:

```
Cluster Health: HEALTHY

✓ Quorum established (3/3 members healthy)
✓ Leader elected (server-1)
✓ etcd connectivity OK
✓ NATS cluster connected
✓ All agents assigned

Total agents: 150
Agent distribution: server-1=50, server-2=48, server-3=52
```

### cluster shards

Show shard assignments and agent distribution using consistent hashing.

```bash
kscorectl cluster shards [flags]
```

**Flags**:

- `--show-agents`: Show individual agent IDs per shard

**Example**:

```bash
kscorectl cluster shards
kscorectl cluster shards --show-agents
```

### cluster add

Add a new member to the cluster.

```bash
kscorectl cluster add <address> [flags]
```

**Arguments**:

- `<address>`: Address of the new member to add (required)

**Flags**:

- `--dry-run`: Show what would be done without adding a member

**Example**:

```bash
kscorectl cluster add server-4:9090
kscorectl cluster add server-4:9090 --dry-run
```

### cluster join

Join this node to an existing cluster.

```bash
kscorectl cluster join <cluster-address> [flags]
```

**Arguments**:

- `<cluster-address>`: Address of any existing cluster member (required)

**Flags**:

- `--dry-run`: Show what would be done without joining
- `--token string`: Join token for cluster authentication
- `--advertise-addr string`: Address this node advertises to other cluster members

**Example**:

```bash
kscorectl cluster join server-1:9090
kscorectl cluster join https://ks-server-1:8080 --token $JOIN_TOKEN
kscorectl cluster join https://ks-server-1:8080 --token $JOIN_TOKEN --advertise-addr 10.0.1.5
```

### cluster token

Manage cluster join tokens. Join tokens authorize new servers to join the cluster.

#### cluster token generate

Generate a new cluster join token. The token value is displayed only once at creation time.

```bash
kscorectl cluster token generate [flags]
```

**Flags**:

- `--label string`: Human-readable label for the token
- `--ttl string`: Token time-to-live (default: `24h`)
- `--max-uses int`: Maximum number of joins allowed (0 = unlimited, default: `0`)

**Example**:

```bash
kscorectl cluster token generate
kscorectl cluster token generate --ttl 1h
kscorectl cluster token generate --ttl 2h --max-uses 3 --label "staging-nodes"
```

#### cluster token list

List all cluster join tokens. Token values are never shown after initial creation.

```bash
kscorectl cluster token list [flags]
```

**Example**:

```bash
kscorectl cluster token list
kscorectl cluster token list -o json
```

#### cluster token revoke

Revoke a cluster join token by ID. Revoked tokens can no longer be used to join the cluster.

```bash
kscorectl cluster token revoke <token-id>
```

**Example**:

```bash
kscorectl cluster token revoke abc123def456
```

### cluster leave

Remove this node from the cluster.

```bash
kscorectl cluster leave [flags]
```

**Flags**:

- `--force`: Force leave without graceful agent migration
- `--dry-run`: Show what would be done without leaving

**Example**:

```bash
kscorectl cluster leave
kscorectl cluster leave --force
```

### cluster drain

Drain all agents from a cluster member before maintenance.

```bash
kscorectl cluster drain <member-id> [flags]
```

**Arguments**:

- `<member-id>`: ID of the member to drain (required)

**Flags**:

- `--dry-run`: Show what would be done without draining

**Example**:

```bash
kscorectl cluster drain server-2
kscorectl cluster drain server-2 --dry-run
```

### cluster undrain

Allow agents on a previously drained cluster member.

```bash
kscorectl cluster undrain <member-id> [flags]
```

**Arguments**:

- `<member-id>`: ID of the member to undrain (required)

**Flags**:

- `--dry-run`: Show what would be done without undraining

**Example**:

```bash
kscorectl cluster undrain server-2
```

### cluster transfer-leader

Transfer cluster leadership to another member.

```bash
kscorectl cluster transfer-leader <member-id> [flags]
```

**Arguments**:

- `<member-id>`: ID of the member to become leader (required)

**Flags**:

- `--dry-run`: Show what would be done without transferring leadership

**Example**:

```bash
kscorectl cluster transfer-leader server-2
kscorectl cluster transfer-leader server-2 --dry-run
```

### cluster backup

Create cluster state backup.

```bash
kscorectl cluster backup [flags]
```

**Flags**:

- `-f, --output string`: Output file path (default: stdout)
- `--shards-only`: Backup only shard assignments
- `--config-only`: Backup only cluster configuration

**Examples**:

```bash
# Backup to stdout
kscorectl cluster backup

# Backup to file
kscorectl cluster backup -f /var/backups/kscore/cluster-backup.json

# Backup only shards
kscorectl cluster backup --shards-only -f shards.json

# Backup only configuration
kscorectl cluster backup --config-only -f config.json
```

**Output** (JSON):

```json
{
  "version": "1.0",
  "timestamp": "2024-01-15T10:30:45Z",
  "cluster": {
    "name": "kscore-prod",
    "quorum_size": 2,
    "leader_id": "server-1",
    "members": [...]
  },
  "shards": [...],
  "config": {...}
}
```

### cluster restore

Restore cluster state from backup.

```bash
kscorectl cluster restore [flags]
```

**Flags**:

- `-f, --input string`: Input backup file path (required)
- `--force`: Skip confirmation prompt
- `--dry-run`: Show what would be restored without making changes

**Examples**:

```bash
# Basic restore
kscorectl cluster restore -f cluster-backup.json

# Force restore on healthy cluster
kscorectl cluster restore -f cluster-backup.json --force

# Dry run to preview changes
kscorectl cluster restore -f cluster-backup.json --dry-run
```

**Output**:

```
Restoring cluster state from cluster-backup.json...

Backup Info:
  Version:   1.0
  Timestamp: 2024-01-15T10:30:45Z
  Cluster:   kscore-prod

Restore Summary:
  Shards restored:  150
  Config restored:  5

Warnings:
  - Agent web-05 assigned to unavailable member server-3, reassigned to server-1

Cluster restore completed successfully.
```

### cluster rebalance

Trigger agent rebalancing across cluster members.

```bash
kscorectl cluster rebalance [flags]
```

**Flags**:

- `--dry-run`: Show what would change without making changes
- `--reason string`: Reason for rebalancing (default: "CLI request")

**Output**:

```
Rebalancing agents across cluster members...

Before:
  server-1: 80 agents
  server-2: 40 agents
  server-3: 30 agents

After:
  server-1: 50 agents
  server-2: 50 agents
  server-3: 50 agents

Agents moved: 15
Rebalance completed successfully.
```

### cluster remove

Remove an unhealthy member from the cluster.

```bash
kscorectl cluster remove <member-id> [flags]
```

**Flags**:

- `--force`: Force removal without reassigning agents first

**Example**:

```bash
# Remove unhealthy member (agents will be reassigned)
kscorectl cluster remove server-3

# Force remove without waiting for reassignment
kscorectl cluster remove server-3 --force
```

### cluster member add

Add a new member to the cluster. Alias for `cluster add`.

```bash
kscorectl cluster member add <address> [flags]
```

**Arguments**:

- `<address>`: Address of the new member (required)

**Flags**:

- `--dry-run`: Show what would be done without adding a member

**Example**:

```bash
kscorectl cluster member add server-4:9090
```

### cluster member remove

Remove a member from the cluster. Alias for `cluster remove`.

```bash
kscorectl cluster member remove <member-id> [flags]
```

**Arguments**:

- `<member-id>`: ID or address of the member to remove (required)

**Flags**:

- `--force`: Force remove even if member is unresponsive
- `--dry-run`: Show what would be done without removing the member

**Example**:

```bash
kscorectl cluster member remove server-3
kscorectl cluster member remove server-3 --force
```

### cluster election restart

Force a new leader election cycle.

```bash
kscorectl cluster election restart [flags]
```

**Flags**:

- `--dry-run`: Show what would be done without restarting election

**Example**:

```bash
kscorectl cluster election restart
```

## kscore-cluster-backup (Cluster Backup and Restore)

Create, restore, verify, and schedule cluster backups via the control plane API.

### Global Flags

These flags apply to all kscore-cluster-backup commands:

- `--server string`: Control plane server address (default: localhost:9090)
- `-o, --output string`: Output format: table, text, json, yaml (default: table)
- `-v, --verbose`: Enable verbose output
- `--audit-level string`: Audit logging level: all, errors, none (default: all)
- `--audit-output string`: Audit output backend: auto, syslog, journald, stderr, none

### cluster-backup backup

Create a backup of the cluster state.

```bash
kscorectl cluster-backup backup [flags]
```

**Flags**:

- `-f, --file string`: Output file path (default: stdout)
- `--compress`: Compress the backup
- `--encrypt`: Encrypt the backup
- `--description string`: Backup description

**Examples**:

```bash
# Create a backup to file
kscorectl cluster-backup backup --file cluster-backup.bin

# Create an encrypted compressed backup
kscorectl cluster-backup backup --file backup.bin.gz --compress --encrypt
```

### cluster-backup restore

Restore cluster state from a backup file.

```bash
kscorectl cluster-backup restore [flags]
```

**Flags**:

- `-f, --input string`: Input backup file path (required)
- `--force`: Skip confirmation prompt
- `--dry-run`: Show what would be done without restoring

**Examples**:

```bash
# Restore from backup
kscorectl cluster-backup restore --input cluster-backup.bin

# Dry run to preview changes
kscorectl cluster-backup restore --input cluster-backup.bin --dry-run
```

### cluster-backup list

List available backups from backup storage.

```bash
kscorectl cluster-backup list [flags]
```

**Flags**:

- `--limit int`: Maximum number of backups to show (default: 20)

### cluster-backup verify

Verify a backup file before restore.

```bash
kscorectl cluster-backup verify [flags]
```

**Flags**:

- `-f, --input string`: Backup file to verify (required)

### cluster-backup schedule

Manage automated backup schedules.

```bash
kscorectl cluster-backup schedule <command> [flags]
```

**Commands**:

- `list`: List backup schedules
- `add <name>`: Add a backup schedule
- `remove <name>`: Remove a backup schedule

**Flags (schedule add)**:

- `--cron string`: Cron expression (default: "0 0 ** *")
- `--retention string`: Backup retention period (default: "7d")

**Examples**:

```bash
# Add a daily backup schedule
kscorectl cluster-backup schedule add daily --cron "0 2 * * *" --retention 14d

# Remove a schedule
kscorectl cluster-backup schedule remove daily
```

## kscore-identity (Identity Management)

Manage SPIFFE identities, join tokens, CA certificates, and trust federation.

> **Note**: This CLI currently returns placeholder/demo data for testing purposes.
> Full API integration is planned for a future release.

### Global Flags

These flags apply to all kscore-identity commands:

- `--server string`: Control plane server address (default: localhost:9090)
- `-o, --output string`: Output format: table, text, json, yaml (default: table)
- `--audit-level string`: Audit logging level: all, errors, none (default: all)
- `--audit-output string`: Audit output backend: auto, syslog, journald, stderr, none (default: auto)

### identity version

Display the kscore-identity version.

```bash
kscorectl identity version
```

**Output**:

```
kscore-identity version 0.1.0 (built: 2024-01-15)
```

### identity status

Display identity provider status.

```bash
kscorectl identity status [flags]
```

**Flags**:

- `-o, --output string`: Output format (table, json, yaml) (default: table)

**Output**:

```
Identity Provider Status
========================
Provider:          embedded
Trust Domain:      kscore.local
CA Status:         healthy
CA Expires:        2027-01-15T10:30:45Z
Active SVIDs:      42
Federated Domains: 1
Last Rotation:     2024-01-15T10:00:45Z
```

### identity token

Manage join tokens for agent attestation.

#### token create

Create a new join token.

```bash
kscorectl identity token create [flags]
```

**Flags**:

- `--agent-id string`: Agent ID for this token (required)
- `--ttl duration`: Token time-to-live (default: 5m)
- `--label string`: Labels to apply in key=value format (repeatable)
- `--path string`: SPIFFE path prefix for the agent identity
- `--uses int`: Maximum number of times this token can be used (default: 1)

**Examples**:

```bash
# Create a token for a specific agent
kscorectl identity token create --agent-id web-server-1

# Create a token with custom TTL and labels
kscorectl identity token create --agent-id web-server-1 --ttl 10m --label role=web

# Create a token that can be used 5 times
kscorectl identity token create --agent-id db-server-1 --ttl 1h --uses 5
```

**Output**:

```
Join Token Created
==================
Token:     Rj2k9xLm3n4o5p6q7r8s9t0u1v2w3x4y5z
Agent ID:  web-server-1
Expires:   2024-01-15T10:35:00Z
TTL:       10m0s

Configure agent with:
  identity:
    attestation:
      type: join_token
      token: "Rj2k9xLm3n4o5p6q7r8s9t0u1v2w3x4y5z"
```

#### token list

List join tokens.

```bash
kscorectl identity token list [flags]
```

**Output**:

```
TOKEN                                    AGENT PATH           EXPIRES                    USES  STATUS
test-token-1                             /agent/web           2024-01-15T12:00:00Z       0/1   valid
test-token-2                             /agent/db            2024-01-15T11:00:00Z       1/1   used
```

#### token show

Show details of a join token.

```bash
kscorectl identity token show <token-id>
```

**Output**:

```
Token Details
=============
Token ID:    abc123def456
Agent Path:  /agent/web
Created:     2024-01-15T10:00:00Z
Expires:     2024-01-15T12:00:00Z
Uses:        0/1
Status:      valid
```

#### token revoke

Revoke a join token.

```bash
kscorectl identity token revoke <token-id>
```

**Output**:

```
Token revoked successfully: abc123def456
```

### identity ca

Manage Certificate Authority.

#### ca info

Show CA information.

```bash
kscorectl identity ca info [flags]
```

**Flags**:

- `-o, --output string`: Output format (table, json, yaml)

**Output**:

```
Certificate Authority Information
==================================

Trust Domain:  kscore.local

Root CA:
  Subject:    CN=Keystone Core Root CA
  Not After:  2034-01-15T00:00:00Z
  Key Type:   ecdsa-p256

Signing CA:
  Subject:    CN=Keystone Core Signing CA
  Not After:  2025-01-15T00:00:00Z
  Key Type:   ecdsa-p256

SVIDs Issued:     1234
Last Rotation:    2024-01-01T00:00:00Z
Next Rotation:    2024-11-01T00:00:00Z
Auto-Rotation:    enabled
```

#### ca backup

Backup CA certificates and keys.

```bash
kscorectl identity ca backup [flags]
```

**Flags**:

- `-o, --output string`: Output file path (required)
- `--encrypt`: Encrypt the backup (default: true)

**Examples**:

```bash
# Create encrypted backup
kscorectl identity ca backup --output /var/backups/ca-backup.json

# Create unencrypted backup
kscorectl identity ca backup --output ca-backup.json --encrypt=false
```

**Output**:

```
CA backup created: /var/backups/ca-backup.json
Encrypted: true
Checksum: sha256:abc123...
```

#### ca restore

Restore CA from backup.

```bash
kscorectl identity ca restore [flags]
```

**Flags**:

- `--input string`: Backup file to restore (required)

**Example**:

```bash
kscorectl identity ca restore --input /var/backups/ca-backup.json
```

**Output**:

```
Restoring CA from backup (created 2024-01-15T10:00:00Z)
Trust Domain: kscore.local
CA restored successfully
```

#### ca rotate

Trigger CA rotation.

```bash
kscorectl identity ca rotate
```

**Output**:

```
CA rotation initiated...
New signing CA created
Old signing CA valid for overlap period
Rotation complete at 2024-01-15T10:30:45Z
```

### identity federation

Manage trust federation with other trust domains. Alias: `fed`

#### federation list

List federated trust domains.

```bash
kscorectl identity federation list [flags]
```

**Flags**:

- `-o, --output string`: Output format (table, json)

**Output**:

```
TRUST DOMAIN             TYPE             STATE     LAST REFRESH
partner.example.org      bidirectional    active    2024-01-15T10:00:00Z
vendor.example.com       unidirectional   suspended 2024-01-14T12:00:00Z
```

#### federation add

Add a federated trust domain.

```bash
kscorectl identity federation add <trust-domain> [flags]
```

**Flags**:

- `--bundle-endpoint string`: Bundle endpoint URL
- `--type string`: Federation type: bidirectional, unidirectional (default: bidirectional)
- `--refresh-interval duration`: Trust bundle refresh interval (default: 5m)

**Examples**:

```bash
# Add bidirectional federation
kscorectl identity federation add partner.example.org \
  --bundle-endpoint https://partner.example.org/.well-known/spiffe-bundle

# Add unidirectional federation with custom refresh interval
kscorectl identity federation add vendor.example.com \
  --bundle-endpoint https://vendor.example.com/.well-known/spiffe-bundle \
  --type unidirectional \
  --refresh-interval 1h
```

**Output**:

```
Federation added: partner.example.org
Type: bidirectional
State: pending (requires approval)
Bundle Endpoint: https://partner.example.org/.well-known/spiffe-bundle
Refresh Interval: 5m0s

To activate, run:
  kscorectl identity federation activate partner.example.org
```

#### federation show

Show details of a federated domain.

```bash
kscorectl identity federation show <trust-domain>
```

**Output**:

```
Trust Domain:     partner.example.org
Type:             bidirectional
State:            active
Bundle Endpoint:  https://partner.example.org/.well-known/spiffe-bundle
Refresh Interval: 5m
Last Refresh:     2024-01-15T10:00:00Z
Created:          2024-01-08T00:00:00Z

Policy:
  Allowed Paths: [/service/**, /agent/**]
  Denied Paths:  [/admin/**]
  Require mTLS:  true

Certificates:
  - CN=Partner CA (expires 2025-01-15T00:00:00Z)
```

#### federation suspend

Suspend a federated trust domain.

```bash
kscorectl identity federation suspend <trust-domain>
```

**Output**:

```
Federation relationship suspended: partner.example.org
SVIDs from this domain will no longer be accepted
```

#### federation activate

Activate a federated trust domain.

```bash
kscorectl identity federation activate <trust-domain>
```

**Output**:

```
Federation relationship activated: partner.example.org
SVIDs from this domain will now be accepted
```

#### federation remove

Remove a federated trust domain.

```bash
kscorectl identity federation remove <trust-domain> [flags]
```

**Flags**:

- `--force`: Force removal without confirmation

**Example**:

```bash
kscorectl identity federation remove vendor.example.com --force
```

**Output**:

```
Federation relationship removed: vendor.example.com
```

#### federation refresh

Manually refresh trust bundle from federated domain.

```bash
kscorectl identity federation refresh <trust-domain>
```

**Output**:

```
Trust bundle refreshed: partner.example.org
Retrieved 2 certificates
```

### identity bundle

Manage trust bundles.

#### bundle show

Show the local trust bundle.

```bash
kscorectl identity bundle show [flags]
```

**Flags**:

- `-o, --output string`: Output format (table, json)

**Output**:

```
Local Trust Bundle
==================
Trust Domain:    kscore.local
Sequence Number: 42
Refresh Hint:    300 seconds
Updated:         2024-01-15T10:00:00Z

Certificates:
  - CN=Keystone Core Root CA
    Expires: 2034-01-15T00:00:00Z
  - CN=Keystone Core Signing CA
    Expires: 2025-01-15T00:00:00Z
```

#### bundle export

Export the trust bundle.

```bash
kscorectl identity bundle export [flags]
```

**Flags**:

- `--format string`: Export format: pem, jwks, spiffe (default: pem)

**Examples**:

```bash
# Export as PEM
kscorectl identity bundle export --format pem

# Export as JWKS (SPIFFE Bundle format)
kscorectl identity bundle export --format jwks

# Export as SPIFFE bundle
kscorectl identity bundle export --format spiffe
```

**Output (PEM)**:

```
-----BEGIN CERTIFICATE-----
MIIBxDCCAWqgAwIBAgIQExample...
-----END CERTIFICATE-----
-----BEGIN CERTIFICATE-----
MIIBxDCCAWqgAwIBAgIQExample...
-----END CERTIFICATE-----
```

**Output (JWKS)**:

```json
{
  "keys": [
    {
      "kty": "EC",
      "use": "x509-svid",
      "x5c": ["MIIBxDCCAWqgAwIBAgIQExample..."]
    }
  ],
  "spiffe_refresh_hint": 300,
  "spiffe_sequence_number": 42
}
```

### identity events

View identity events.

```bash
kscorectl identity events [flags]
```

**Flags**:

- `--limit int`: Number of events to show (default: 10)

**Output**:

```
TIME       TYPE                    DESCRIPTION
10:00:05   svid.issued             X.509 SVID issued
           SPIFFE ID: spiffe://kscore.local/agent/web-server-1
10:02:15   svid.rotated            X.509 SVID rotated
           SPIFFE ID: spiffe://kscore.local/agent/db-server-1
10:05:00   federation.refreshed    Trust bundle refreshed for partner.example.org
```

### Examples

**Bootstrap a new agent**:

```bash
# Create a join token
kscorectl identity token create --agent-id web-server-1 --ttl 10m

# Copy token to agent configuration
# Start agent - it will use the token to register
```

**Set up trust federation**:

```bash
# Add federation relationship
kscorectl identity federation add partner.example.org \
  --bundle-endpoint https://partner.example.org/.well-known/spiffe-bundle

# Verify bundle was fetched
kscorectl identity federation show partner.example.org

# Activate the federation
kscorectl identity federation activate partner.example.org
```

**Backup and restore CA**:

```bash
# Create encrypted backup
kscorectl identity ca backup --output /var/backups/ca-$(date +%Y%m%d).json

# Restore (after disaster)
kscorectl identity ca restore --input /var/backups/ca-20240115.json
```

**Rotate CA**:

```bash
# Check current CA status
kscorectl identity ca info

# Trigger rotation
kscorectl identity ca rotate

# Verify new CA
kscorectl identity ca info
```

## kscore-federation (Trust Federation)

Manage trust federation as a dedicated CLI (split from `kscore-identity`).

> **Note**: This CLI currently returns placeholder/demo data for testing purposes.
> Full API integration is planned for a future release.

### Global Flags

These flags apply to all kscore-federation commands:

- `--server string`: Control plane API address (default: localhost:9090)
- `-o, --output string`: Output format: table, text, json, yaml (default: table)
- `--audit-level string`: Audit logging level: all, errors, none (default: all)
- `--audit-output string`: Audit output backend: auto, syslog, journald, stderr, none

### federation wizard

Interactive wizard for trust federation setup.

```bash
kscorectl federation wizard [flags]
```

**Flags**:

- `--non-interactive`: Run with prompts from flags only
- `--domain string`: Partner trust domain
- `--endpoint string`: Bundle endpoint URL
- `--type string`: Federation type: bidirectional, unidirectional (default: bidirectional)
- `--policy string`: Policy template: allow-all, services-only, agents-only, kubernetes (default: services-only)
- `--refresh duration`: Bundle refresh interval (default: 5m)
- `--mtls`: Require mutual TLS (default: true)
- `--auto-activate`: Activate immediately without confirmation

**Examples**:

```bash
# Interactive mode - guided setup
kscorectl federation wizard

# Non-interactive mode
kscorectl federation wizard \
  --non-interactive \
  --domain partner.example.org \
  --endpoint https://partner.example.org/.well-known/spiffe-bundle \
  --type bidirectional \
  --policy services-only \
  --auto-activate
```

**Policy Templates**:

| Template | Description |
|----------|-------------|
| `services-only` | Allow `/service/**`, deny `/admin/**` and `/internal/**` (recommended) |
| `allow-all` | Trust all identities from the partner domain |
| `agents-only` | Only allow `/agent/**` paths |
| `kubernetes` | Allow Kubernetes service account paths (`/ns/*/sa/*`) |

### federation list

List federated trust domains.

```bash
kscorectl federation list [flags]
```

### federation add

Add a federated trust domain.

```bash
kscorectl federation add <trust-domain> [flags]
```

**Flags**:

- `--bundle-endpoint string`: Bundle endpoint URL
- `--type string`: Federation type: bidirectional, unidirectional (default: bidirectional)
- `--refresh-interval duration`: Trust bundle refresh interval (default: 5m)

### federation show

Show details for a federated domain.

```bash
kscorectl federation show <trust-domain> [flags]
```

### federation suspend

Suspend trust with a federated domain.

```bash
kscorectl federation suspend <trust-domain>
```

### federation activate

Activate trust with a federated domain.

```bash
kscorectl federation activate <trust-domain>
```

### federation remove

Remove a federated trust domain.

```bash
kscorectl federation remove <trust-domain> [flags]
```

**Flags**:

- `--force`: Skip confirmation prompt

### federation refresh

Refresh trust bundle from a federated domain.

```bash
kscorectl federation refresh <trust-domain>
```

### federation bundle

Manage the local trust bundle.

```bash
kscorectl federation bundle <command> [flags]
```

**Commands**:

- `show`: Show the local trust bundle
- `export`: Export the trust bundle

**Export Flags**:

- `--format string`: Bundle format (pem, jwks, spiffe)

### federation status

Show an overall summary of federation health.

```bash
kscorectl federation status [flags]
```

**Output**:

- Total federated domains
- Active, suspended, and pending domain counts
- Last bundle refresh time

**Example**:

```bash
kscorectl federation status
kscorectl federation status -o json
```

### federation trust list

List federation trust relationships. Alias for `federation list`.

```bash
kscorectl federation trust list [flags]
```

**Example**:

```bash
kscorectl federation trust list
kscorectl federation trust list -o json
```

### federation ping

Test connectivity to a federated domain by fetching its trust bundle endpoint.

```bash
kscorectl federation ping [trust-domain] [flags]
```

**Arguments**:

- `[trust-domain]`: Trust domain name to ping (optional if `--region` is used)

**Flags**:

- `--region string`: Region/trust domain name to ping

**Example**:

```bash
kscorectl federation ping partner.example.org
kscorectl federation ping --region eu-west
```

## kscore-migrate (Database Migration)

Migrate data between storage backends (SQLite to PostgreSQL).

### migrate run

Run migration from SQLite to PostgreSQL.

```bash
kscorectl migrate run [flags]
```

**Required Flags**:

- `--sqlite string`: Path to SQLite database file
- `--postgres string`: PostgreSQL connection string

**Optional Flags**:

- `--dry-run`: Perform a dry run without writing to target
- `--batch-size int`: Number of records per batch (default: 100)
- `--continue-on-error`: Continue migration even if some records fail
- `--skip-existing`: Skip records that already exist in target (default: true)
- `--verbose`: Enable verbose output

**Examples**:

```bash
# Basic migration
kscorectl migrate run \
  --sqlite /var/lib/keystone-core/state.db \
  --postgres "postgres://kscore:password@localhost/keystonecore"

# Dry run first
kscorectl migrate run \
  --sqlite /var/lib/keystone-core/state.db \
  --postgres "postgres://kscore:password@localhost/keystonecore" \
  --dry-run --verbose

# Continue on errors
kscorectl migrate run \
  --sqlite /var/lib/keystone-core/state.db \
  --postgres "postgres://kscore:password@localhost/keystonecore" \
  --continue-on-error
```

**Output**:

```
Starting migration from SQLite to PostgreSQL...
  Mode: DRY RUN (no data will be written)
  Source: /var/lib/keystone-core/state.db
  Target: PostgreSQL
  Batch size: 100
  Skip existing: true
  Continue on error: false

  agents: 0/150
  agents: 150/150
  commands: 0/1234
  commands: 1234/1234
  batch_jobs: 0/45
  batch_jobs: 45/45
  batch_agent_results: 0/890
  batch_agent_results: 890/890

Migration completed!
  Duration: 2.534s
  Agents migrated: 150
  Commands migrated: 1234
  Batch jobs migrated: 45
  Batch agent results migrated: 890
```

### migrate validate

Validate migration completeness by comparing source and target databases.

```bash
kscorectl migrate validate [flags]
```

**Required Flags**:

- `--sqlite string`: Path to SQLite database file
- `--postgres string`: PostgreSQL connection string

**Examples**:

```bash
kscorectl migrate validate \
  --sqlite /var/lib/keystone-core/state.db \
  --postgres "postgres://kscore:password@localhost/keystonecore"
```

**Output (Success)**:

```
Validating migration...
  Source: /var/lib/keystone-core/state.db
  Target: PostgreSQL

Record counts:
  Agents:             Source=150  Target=150
  Commands:           Source=1234 Target=1234
  Batch jobs:         Source=45   Target=45
  Batch agent results: Source=890  Target=890

Validation PASSED - all record counts match
```

**Output (Failure)**:

```
Validation FAILED - discrepancies found:
  - agent count mismatch: source=150, target=148
  - command count mismatch: source=1234, target=1200
```

### migrate version

Display version information.

```bash
kscorectl migrate version
```

## kscore-registry (Module Registry Server)

A standalone HTTP server for hosting and distributing Keystone Core modules. Provides a Go-mod style API for module discovery, publishing, and download.

### Starting the Server

```bash
kscore-registry [flags]
```

**Flags**:

- `--data string`: Data directory for storing modules (default: ./data)
- `--listen string`: Address to listen on (default: :8090)
- `--api-key string`: API key for write operations (or KSCORE_REGISTRY_API_KEY env)
- `--readonly`: Disable write operations (mirror mode)
- `--max-upload-size int`: Maximum upload size in bytes (default: 100MB)
- `--cors`: Enable CORS headers (requires --cors-origins)
- `--cors-origins string`: Comma-separated list of allowed CORS origins (e.g., '<https://example.com,https://app.example.com>')
- `--storage-backend string`: Storage backend type: filesystem, s3, gcs, azure, nats (default: filesystem)

**S3 Backend Flags**:

- `--s3-bucket string`: S3 bucket name
- `--s3-region string`: AWS region
- `--s3-endpoint string`: Custom S3 endpoint URL (for MinIO, etc.)
- `--s3-prefix string`: Key prefix within the bucket
- `--s3-path-style`: Use path-style addressing

**GCS Backend Flags**:

- `--gcs-bucket string`: GCS bucket name
- `--gcs-project string`: GCP project ID
- `--gcs-credentials-file string`: Path to service account JSON key
- `--gcs-prefix string`: Object prefix within the bucket

**Azure Backend Flags**:

- `--azure-container string`: Blob container name
- `--azure-account-name string`: Storage account name
- `--azure-connection-string string`: Full connection string
- `--azure-prefix string`: Blob prefix within the container

**NATS Backend Flags**:

- `--nats-url string`: NATS server URL
- `--nats-bucket string`: Object store bucket name
- `--nats-credentials string`: Path to NATS credentials file
- `--nats-prefix string`: Key prefix within the bucket

**Examples**:

```bash
# Start with defaults
kscore-registry

# Start with custom data directory and port
kscore-registry --data /var/lib/keystone-core/modules --listen :8080

# Start in read-only mirror mode
kscore-registry --data /var/lib/keystone-core/modules --readonly

# Start with authentication required for writes
kscore-registry --api-key "your-secret-api-key"
# Or via environment variable:
export KSCORE_REGISTRY_API_KEY="your-secret-api-key"
kscore-registry

# Enable CORS for web-based module browsers
kscore-registry --cors --cors-origins "https://example.com,https://app.example.com"

# Enable CORS with wildcard (use with caution)
kscore-registry --cors --cors-origins "*"

# Use S3 storage backend
kscore-registry --storage-backend s3 --s3-bucket my-registry --s3-region us-east-1

# Use S3-compatible storage (MinIO)
kscore-registry --storage-backend s3 --s3-bucket registry \
  --s3-endpoint http://minio:9000 --s3-path-style

# Use GCS storage backend
kscore-registry --storage-backend gcs --gcs-bucket my-registry --gcs-project my-project

# Use Azure storage backend
kscore-registry --storage-backend azure --azure-container my-registry \
  --azure-account-name mystorageaccount

# Use NATS Object Store backend
kscore-registry --storage-backend nats --nats-url nats://localhost:4222 \
  --nats-bucket registry
```

### Migrating Storage

Migrate modules between storage backends:

```bash
# Migrate from filesystem to S3
kscore-registry migrate-storage \
  --data /var/lib/keystone-core/modules \
  --dest-backend s3 \
  --dest-s3-bucket kscore-registry \
  --dest-s3-region us-east-1

# Dry run
kscore-registry migrate-storage \
  --data /var/lib/keystone-core/modules \
  --dest-backend s3 \
  --dest-s3-bucket kscore-registry \
  --dest-s3-region us-east-1 \
  --dry-run
```

**Migration Flags**:

- `--dest-backend string`: Destination storage backend type (required)
- `--dest-data string`: Destination data directory (for filesystem destination)
- `--dest-s3-bucket string`: Destination S3 bucket
- `--dest-s3-region string`: Destination S3 region
- `--dest-s3-endpoint string`: Destination S3 endpoint
- `--dest-s3-prefix string`: Destination S3 prefix
- `--dest-s3-path-style`: Destination S3 path-style addressing
- `--dry-run`: Preview what would be migrated without writing

### Version Information

```bash
kscore-registry version
```

**Output**:

```
kscore-registry version 0.1.0
```

**Server Startup Output**:

```
Starting kscore-registry on :8090
  Data directory: ./data
  Mode: authenticated write (API key required)
```

### Offline Registry Management

Commands for managing air-gapped offline registries.

#### Initialize

```bash
kscore-registry offline init --dir /path/to/registry
```

Creates the directory structure (modules/, blueprints/) and an empty index.

#### List Contents

```bash
kscore-registry offline list --dir /path/to/registry
```

Lists all modules and blueprints in the offline registry.

#### Search

```bash
kscore-registry offline search --dir /path/to/registry "networking"
```

Searches modules by name, description, or tags.

#### Import

```bash
# Import from a bootstrap package
kscore-registry offline import --dir /path/to/registry bootstrap-package.tar.gz

# Import from a mirror directory
kscore-registry offline import --dir /path/to/registry /path/to/mirror
```

Imports modules from bootstrap packages or exported mirror directories.

#### Verify Signatures

```bash
kscore-registry offline verify --dir /path/to/registry \
  --trust-dir /path/to/trust --require-signatures
```

Verifies module signatures against trusted signing keys.

**Flags:**

- `--trust-dir string`: Directory containing trust roots (trust.json)
- `--require-signatures`: Reject unsigned modules

#### Garbage Collection

```bash
kscore-registry offline gc --dir /path/to/registry \
  --keep-versions 3 --dry-run
```

Removes old module versions based on retention policy.

**Flags:**

- `--keep-versions int`: Keep N most recent versions per module (0=all)
- `--max-age duration`: Remove versions older than this (e.g., 720h)
- `--dry-run`: Show what would be removed

#### Reindex

```bash
kscore-registry offline reindex --dir /path/to/registry
```

Regenerates the registry index from the filesystem.

#### Trust Management

```bash
# Add a trusted signing key
kscore-registry offline trust add --dir /path/to/registry mykey public-key.pem

# List trusted keys
kscore-registry offline trust list --dir /path/to/registry

# Remove a trusted key
kscore-registry offline trust remove --dir /path/to/registry mykey
```

### API Endpoints

The registry provides a Go-mod style HTTP API:

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/<module>/@v/list` | List available versions |
| GET | `/<module>/@v/<version>.info` | Get module metadata (JSON) |
| GET | `/<module>/@v/<version>.mod` | Get module manifest (YAML) |
| GET | `/<module>/@v/<version>.zip` | Download module archive |
| POST | `/<module>/@v/<version>` | Publish module (requires auth) |
| DELETE | `/<module>/@v/<version>` | Delete module (requires auth) |
| GET | `/health` | Health check endpoint |
| GET | `/` | Server information (name, version, mode) |

### Listing Versions

```bash
curl http://localhost:8090/myorg/webserver/@v/list
```

**Response**:

```
1.2.0
1.1.0
1.0.0
```

Versions are sorted in descending order (newest first).

### Getting Module Info

```bash
curl http://localhost:8090/myorg/webserver/@v/1.2.0.info
```

**Response**:

```json
{
  "name": "myorg/webserver",
  "version": "1.2.0",
  "hash": "sha256:a1b2c3d4e5f6...",
  "published_at": "2024-01-15T10:30:45Z",
  "description": "Web server configuration module",
  "dependencies": {
    "std/files": "^1.0.0",
    "std/exec": "^1.0.0"
  },
  "size": 4200
}
```

### Getting Module Manifest

```bash
curl http://localhost:8090/myorg/webserver/@v/1.2.0.mod
```

**Response** (YAML):

```yaml
name: myorg/webserver
version: 1.2.0
type: starlark
description: Web server configuration module
author: John Doe
dependencies:
  std/files: "^1.0.0"
  std/exec: "^1.0.0"
capabilities:
  - fs.read
  - fs.write
  - exec
```

### Downloading Modules

```bash
curl -O http://localhost:8090/myorg/webserver/@v/1.2.0.zip
```

Returns the module ZIP archive with appropriate `Content-Disposition` header.

### Publishing Modules

Publishing requires API key authentication and uses multipart form upload:

```bash
curl -X POST http://localhost:8090/myorg/webserver/@v/1.2.0 \
  -H "X-API-Key: your-secret-api-key" \
  -F "module=@myorg-webserver-1.2.0.zip" \
  -F "manifest=$(cat module.yaml)" \
  -F "hash=sha256:a1b2c3d4e5f6..." \
  -F "signature=@myorg-webserver-1.2.0.zip.sig"
```

**Form Fields**:

- `module`: Module ZIP file (required)
- `manifest`: Module manifest YAML content (required)
- `hash`: SHA256 hash of the module (optional, computed if not provided)
- `signature`: Detached signature content (optional)
- `force`: Set to "true" to overwrite existing version
- `release_notes`: Release notes for this version
- `tags`: JSON array of tags

**Response**:

```json
{
  "module_name": "myorg/webserver",
  "version": "1.2.0",
  "hash": "sha256:a1b2c3d4e5f6...",
  "url": "/myorg/webserver/@v/1.2.0.zip",
  "published_at": "2024-01-15T10:30:45Z",
  "size": 4200
}
```

**Error Responses**:

- `401 Unauthorized`: Missing or invalid API key
- `403 Forbidden`: Registry is in read-only mode
- `409 Conflict`: Version already exists (use `force=true` to overwrite)
- `400 Bad Request`: Invalid module or manifest

### Deleting Modules

```bash
curl -X DELETE http://localhost:8090/myorg/webserver/@v/1.2.0 \
  -H "X-API-Key: your-secret-api-key"
```

Returns `204 No Content` on success.

### Health Check

```bash
curl http://localhost:8090/health
```

**Response**:

```json
{
  "status": "healthy",
  "time": "2024-01-15T10:30:45Z"
}
```

### Server Information

```bash
curl http://localhost:8090/
```

**Response**:

```json
{
  "name": "kscore-registry",
  "version": "0.1.0",
  "readonly": false
}
```

### Authentication

Write operations (publish, delete) require authentication via API key:

**Header Authentication**:

```bash
curl -H "X-API-Key: your-secret-api-key" ...
```

**Bearer Token**:

```bash
curl -H "Authorization: Bearer your-secret-api-key" ...
```

### Server Configuration

The registry can be configured via environment variables:

| Variable | Description |
|----------|-------------|
| `KSCORE_REGISTRY_API_KEY` | API key for write operations |
| `KSCORE_REGISTRY_STORAGE_BACKEND` | Storage backend type (filesystem, s3, gcs, azure, nats) |

### Data Storage

Modules are stored in a structured layout (paths are the same across all backends):

```
data/
├── myorg/
│   └── webserver/
│       ├── 1.0.0/
│       │   ├── module.zip      # Module archive
│       │   ├── module.yaml     # Module manifest
│       │   ├── module.info     # Metadata JSON
│       │   └── module.sig      # Signature (optional)
│       └── 1.2.0/
│           └── ...
└── std/
    └── files/
        └── ...
```

## Configuration File

Default location: `~/.keystone-core/config.yaml`

```yaml
# Control plane connection
server: "http://control-plane.example.com:8080"
api_key: "<your-api-key>"

# TLS configuration
tls:
  enabled: true
  ca_cert: "/etc/keystone-core/ca.crt"
  client_cert: "/etc/keystone-core/client.crt"
  client_key: "/etc/keystone-core/client.key"

# Output preferences
output:
  format: "text"  # text, json, yaml
  color: true
  timestamps: false

# Defaults
defaults:
  timeout: "5m"
  batch_size: 10
```

## Environment Variables

Override configuration with environment variables:

```bash
KSCORE_SERVER="http://control-plane:8080"
KSCORE_API_KEY="<your-api-key>"
KSCORE_CONFIG="/custom/config.yaml"
KSCORE_OUTPUT_FORMAT="json"
KSCORE_NO_COLOR="true"
KSCORE_BLUEPRINT_REGISTRY="https://blueprints.example.com"
KSCORE_REGISTRY="https://registry.example.com"
KSCORE_REGISTRY_TOKEN="..."
KSCORE_REGISTRY_USERNAME="user"
KSCORE_REGISTRY_PASSWORD="pass"
KSCORE_CACHE_DIR="/var/cache/keystone-core/modules"
```

**Example**:

```bash
export KSCORE_SERVER="http://localhost:8080"
kscorectl exec run "uptime" --target "role:web"
```

## Exit Codes

All commands return standard exit codes:

- `0`: Success
- `1`: General error
- `2`: Command syntax error
- `3`: Connection error
- `4`: Authentication error
- `5`: Not found
- `130`: Interrupted (Ctrl+C)

## Aliases

Create shell aliases for common commands:

```bash
# .bashrc or .zshrc
alias ta='kscorectl'
alias tae='kscorectl exec run'
alias tas='kscorectl state apply'
alias tam='kscorectl monitor'
```

**Usage**:

```bash
tae "uptime" --target "role:web"
tas web-server.yaml
tam
```

## kscore-agents (Agent Management Plugin)

Manage agent inventory, tokens, tags, and status. Invoked via `kscorectl agents`.

> **API Status**: `list` and `show` connect to gRPC AgentService (returns errors if server unavailable). `re-enroll` and `token create` use the cluster token REST API (`POST /api/v1/cluster/tokens`). Other commands (`delete`, `quarantine`, `tags`, `status`, etc.) require server-side RPCs that are not yet wired.

### Global Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--server, -s` | Control plane gRPC address | `localhost:9090` |
| `--output, -o` | Output format (table, json, yaml, wide) | `table` |
| `--verbose, -v` | Verbose output | `false` |

### agents list

List registered agents with filters.

**Flags**:

- `--status string`: Filter by status (online, offline, degraded)
- `--filter string`: Filter expression (e.g., 'role:web')
- `--label, -l string`: Filter by label (can be repeated)
- `--edge`: Show only edge agents
- `--limit int`: Maximum number of agents to return (default: 100)
- `--show-compatibility`: Show version compatibility information
- `--suspicious`: Show only suspicious agents (version mismatch, stale heartbeat, degraded)

**Examples**:

```bash
kscorectl agents list --status online --label role=web --limit 100
kscorectl agents list --edge --show-compatibility
kscorectl agents list --filter "os:linux AND role:web"
kscorectl agents list --suspicious
```

### agents show

```bash
kscorectl agents show <agent-id>
```

### agents delete

```bash
kscorectl agents delete <agent-id> [--force]
```

### agents quarantine / unquarantine

```bash
kscorectl agents quarantine <agent-id> --reason "Suspicious activity"
kscorectl agents unquarantine <agent-id>
```

### agents status

```bash
kscorectl agents status
kscorectl agents status <agent-id>
```

### agents tags (labels)

```bash
kscorectl agents tags set <agent-id> role=web env=prod
kscorectl agents tags add <agent-id> monitoring=enabled
kscorectl agents tags remove <agent-id> monitoring
kscorectl agents tags show <agent-id>
```

### agents token

Manage agent join tokens.

#### agents token create

Create a new join token for agent registration.

**Flags**:

- `--ttl string`: Token time-to-live (e.g., 1h, 24h, 7d)
- `--max-uses int`: Maximum number of times token can be used

```bash
kscorectl agents token create --ttl 1h --max-uses 10
```

#### agents token list

List join tokens.

**Flags**:

- `--show-expired`: Include expired tokens in output

```bash
kscorectl agents token list
kscorectl agents token list --show-expired
```

#### agents token revoke

Revoke a join token.

```bash
kscorectl agents token revoke <token-id>
```

### agents renew-svid

```bash
kscorectl agents renew-svid <agent-id> --force
```

### agents verify

Verify agent integrity by checking connectivity, version, certificates, and heartbeat.

```bash
kscorectl agents verify <agent-id>
kscorectl agents verify --all
kscorectl agents verify --sample 10
```

**Flags**:

- `--all`: Verify all agents
- `--sample int`: Verify a random sample of N agents

### agents certificates

Manage agent TLS/mTLS certificates.

#### agents certificates regenerate

Trigger certificate regeneration for one or all agents.

```bash
kscorectl agents certificates regenerate <agent-id>
kscorectl agents certificates regenerate --all
kscorectl agents certificates regenerate --all --force
```

**Flags**:

- `--all`: Regenerate certificates for all agents
- `-f, --force`: Skip confirmation prompt

### agents re-enroll

Re-enroll an agent by invalidating its current credentials and issuing a new one-time enrollment token. Use this during security incidents when an agent's credentials may be compromised.

```bash
kscorectl agents re-enroll <agent-id> [flags]
```

**Flags**:

- `-f, --force`: Skip confirmation prompt
- `--reason string`: Reason for re-enrollment (recorded in audit log)

**Examples**:

```bash
# Re-enroll an agent after credential compromise
kscorectl agents re-enroll web-001 --reason "credential compromise"

# Force re-enroll without confirmation
kscorectl agents re-enroll web-001 --force --reason "routine rotation"

# Output as JSON
kscorectl agents re-enroll web-001 --force -o json
```

### agents revoke-credentials

Revoke all credentials for an agent without deleting its registration. The agent is immediately locked out and quarantined. No new enrollment token is issued — use `agents re-enroll` to restore access later.

```bash
kscorectl agents revoke-credentials <agent-id> [flags]
```

**Flags**:

- `-f, --force`: Skip confirmation prompt
- `--reason string`: Reason for revocation (recorded in audit log)

**Examples**:

```bash
# Revoke credentials for a compromised agent
kscorectl agents revoke-credentials web-001 --reason "suspected compromise"

# Force revoke without confirmation
kscorectl agents revoke-credentials web-001 --force --reason "incident IR-2026-42"

# Restore access later
kscorectl agents re-enroll web-001 --reason "investigation complete"
```

## kscore-loadtest (Load Testing Tool)

Run load test scenarios for registration, heartbeats, and command execution.

### Global Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--output, -o` | Output format (table, json, yaml) | `table` |
| `--verbose, -v` | Verbose output | `false` |
| `--audit-level` | Audit logging level | `all` |
| `--audit-output` | Audit output backend | `auto` |

### loadtest run

Run a load test scenario.

| Flag | Description | Default |
|------|-------------|---------|
| `--agents, -a` | Number of simulated agents | `10` |
| `--scenario, -s` | Scenario to run (registration, heartbeat, commands, rampup, sustained) | `registration` |
| `--duration, -d` | Test duration | `60s` |
| `--ramp-up` | Ramp-up duration for gradual agent start | `10s` |
| `--heartbeat-interval` | Heartbeat interval | `5s` |
| `--commands-per-agent` | Commands per agent for command tests | `10` |
| `--concurrent-commands` | Maximum concurrent commands | `50` |
| `--report-dir` | Directory for saving reports | `reports/loadtest` |
| `--nats-port` | Port for embedded NATS server | `14222` |

```bash
kscorectl loadtest run --agents 100 --scenario registration
kscorectl loadtest run --agents 50 --scenario commands --commands-per-agent 10
kscorectl loadtest run --agents 200 --scenario sustained --duration 5m --ramp-up 30s
```

### loadtest scenarios

List available load test scenarios.

```bash
kscorectl loadtest scenarios
```

### loadtest report

Display a previously generated load test report.

| Flag | Description | Default |
|------|-------------|---------|
| `--file, -f` | Report file to display | *(required)* |

```bash
kscorectl loadtest report --file reports/loadtest/results.json
```

## kscore-test (Test Runner)

Run smoke, integration, and suite-based tests against a deployment.

### Global Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--server, -s` | Control plane gRPC address | `localhost:9090` |
| `--output, -o` | Output format (table, json, yaml) | `table` |
| `--verbose, -v` | Verbose output | `false` |
| `--audit-level` | Audit logging level | `all` |
| `--audit-output` | Audit output backend | `auto` |

### test smoke

Run smoke tests to verify basic Keystone Core functionality.

| Flag | Description | Default |
|------|-------------|---------|
| `--target, -t` | Target expression for agents | |
| `--timeout` | Test timeout duration | `5m` |
| `--tags` | Filter tests by tags (comma-separated) | |

```bash
kscorectl test smoke --target "role:web" --timeout 5m
kscorectl test smoke --tags quick,health
```

### test integration

Run integration test suites (basic, recovery, cluster, state, execution, events, policy, gitops).

| Flag | Description | Default |
|------|-------------|---------|
| `--suite, -S` | Test suite to run (comma-separated for multiple) | `basic` |
| `--target, -t` | Target expression for agents | |
| `--timeout` | Test timeout duration | `30m` |
| `--tags` | Filter tests by tags (comma-separated) | |
| `--parallel` | Number of parallel test executions | `1` |

```bash
kscorectl test integration --suite recovery --target "role:control-plane"
kscorectl test integration --suite basic,state --parallel 2
```

### test run

Run a specific test suite with configurable options.

| Flag | Description | Default |
|------|-------------|---------|
| `--suite, -S` | Test suite to run | `basic` |
| `--target, -t` | Target expression for agents | |
| `--timeout` | Test timeout duration | `30m` |
| `--tags` | Filter tests by tags (comma-separated) | |
| `--parallel` | Number of parallel test executions | `1` |
| `--dry-run` | Show what would be executed without running | `false` |
| `--fail-fast` | Stop on first failure | `false` |

```bash
kscorectl test run --suite e2e --timeout 1h --parallel 4
kscorectl test run --suite basic --dry-run
kscorectl test run --suite state --fail-fast --tags core
```

### test list

List available test suites.

| Flag | Description | Default |
|------|-------------|---------|
| `--type, -t` | Filter by test type (smoke, integration, e2e) | |
| `--tags` | Filter by tags (comma-separated) | |

```bash
kscorectl test list
kscorectl test list --type integration
kscorectl test list --tags core,agent
```

### test show

Show detailed results of a specific test run.

```bash
kscorectl test show <test-id>
```

### test history

Show history of test runs with filtering options.

| Flag | Description | Default |
|------|-------------|---------|
| `--suite, -S` | Filter by suite name | |
| `--limit, -n` | Maximum number of results | `20` |
| `--status` | Filter by status (passed, failed) | |

```bash
kscorectl test history
kscorectl test history --suite recovery --status failed
kscorectl test history --limit 50
```

### test suite list

List all test suites with optional filtering.

| Flag | Description | Default |
|------|-------------|---------|
| `--type, -t` | Filter by suite type (smoke, integration, e2e) | |
| `--tags` | Filter by tags (comma-separated) | |

```bash
kscorectl test suite list
kscorectl test suite list --type e2e --tags cluster
```

### test suite show

Show details of a specific test suite.

```bash
kscorectl test suite show <suite-name>
```

### test suite create

Create a new test suite definition.

| Flag | Description | Default |
|------|-------------|---------|
| `--description, -d` | Suite description | |
| `--type, -t` | Suite type (smoke, integration, e2e) | `integration` |
| `--timeout` | Default timeout for tests | `30m` |
| `--tags` | Tags for the suite (comma-separated) | |

```bash
kscorectl test suite create my-suite --type integration --description "Custom tests" --tags custom
```

### test suite delete

Delete a test suite.

| Flag | Description | Default |
|------|-------------|---------|
| `--force, -f` | Skip confirmation | `false` |

```bash
kscorectl test suite delete my-suite --force
```

## kscore-agent (Agent Daemon)

The agent daemon runs on managed nodes. It's not invoked via kscorectl.

### Running the Agent

```bash
# Run in foreground (development - uses ./keystone-core-agent.yaml)
kscore-agent

# Run with explicit config (production)
kscore-agent --config /etc/keystone-core/agent.yaml

# Show version
kscore-agent version
```

### Windows Service Commands

On Windows, the agent can run as a Windows service:

```powershell
# Install as Windows service
kscore-agent service-install

# Uninstall Windows service
kscore-agent service-uninstall

# Start the service
kscore-agent service-start

# Stop the service
kscore-agent service-stop

# Check service status
kscore-agent service-status
```

**Service Configuration**:

- **Name**: kscore-agent
- **Display Name**: Keystone Core Agent
- **Startup Type**: Automatic (Delayed Start)
- **Account**: Local System
- **Recovery**: Restart on failure (5s, 30s, 60s delays)

**Note**: Service commands require Administrator privileges and are only available on Windows.

### Agent Flags

```
--config string   Config file path (default: ./keystone-core-agent.yaml)
-h, --help        Show help
```

> **Note**: In development, the agent looks for `keystone-core-agent.yaml` in the current directory.
> For production deployments:
>
> - **Linux/macOS**: `/etc/keystone-core/agent.yaml`
> - **Windows**: `C:\ProgramData\kscore\agent.yaml`

### Agent Bootstrap (kscore-agent bootstrap)

Bootstrap a Keystone Core deployment using the single-binary flow (Epic 27).

```bash
kscore-agent bootstrap --mode production --cluster-name prod --node-role control-plane
kscore-agent bootstrap --config bootstrap.yaml --non-interactive
```

**Basic Flags**:

```
--mode string                Deployment mode: demo, production, fullscale, custom
--dry-run                    Show planned actions without making changes
--verbose                    Enable verbose output
--non-interactive            Run without interactive prompts
--json                       Output progress as JSON
--config string              Path to bootstrap configuration file (input)
--config-file string         Write bootstrap configuration to file (output)
--skip-repo-setup            Skip package repository configuration
```

**Cluster Configuration**:

```
--cluster-name string        Cluster name
--node-role string           Node role: control-plane, agent, both
--node-name string           Node name (defaults to hostname)
--node-label string          Node labels (key=value, repeatable)
--join string                Cluster endpoint to join
--join-token string          Join token for authentication
--bind-address string        Address to bind services
--advertise-address string   Address to advertise to cluster
```

**Storage Configuration**:

```
--storage-backend string     Storage backend: sqlite, postgres
--postgres-host string       PostgreSQL host
--postgres-port int          PostgreSQL port
--postgres-database string   PostgreSQL database
--postgres-user string       PostgreSQL user
--postgres-password string   PostgreSQL password
--postgres-sslmode string    PostgreSQL SSL mode
```

**NATS Configuration**:

```
--nats-mode string           NATS mode: embedded, cluster, external, leaf
--nats-urls strings          External NATS URLs
--nats-creds-file string     NATS credentials file
--nats-user string           NATS username
--nats-password string       NATS password
```

**TLS Configuration**:

```
--generate-certs             Generate self-signed certificates
--tls-cert-file string       TLS certificate file
--tls-key-file string        TLS key file
--tls-ca-file string         TLS CA file
--tls-csr-file string        TLS certificate signing request output file
--tls-renewal-command string Command to renew TLS certificates
--tls-renewal-script string  Path to write the TLS renewal script
```

**Package Configuration**:

```
--package-channel string     Package channel to install from (default: stable)
--package-version string     Package version to install (pin)
```

**Migration Flags** (SQLite to PostgreSQL):

```
--migrate-from-sqlite string   Path to SQLite database to migrate
--migrate-batch-size int       Batch size for migration (default: 100)
--migrate-continue-on-error    Continue migration if some records fail
--migrate-skip-existing        Skip records that already exist (default: true)
```

**Blueprint Configuration**:

```
--blueprints-dir string          Directory containing blueprints
--apply-blueprint strings        Blueprints to apply after bootstrap
--blueprint-param string         Parameter override (format: blueprint:KEY=VALUE)
--blueprint-feature string       Feature toggle (format: blueprint:feature=true)
--blueprint-entrypoint string    Entrypoint override (format: blueprint:entrypoint)
--export-states-dir string       Write rendered states to directory (dry-run)
```

**Environment Overrides**:

| Variable | Maps to |
|----------|---------|
| `KSCORE_BOOTSTRAP_MODE` | `--mode` |
| `KSCORE_CLUSTER_NAME` | `--cluster-name` |
| `KSCORE_NODE_ROLE` | `--node-role` |
| `KSCORE_NODE_NAME` | `--node-name` |
| `KSCORE_NODE_LABELS` | `--node-label` (comma-separated `k=v`) |
| `KSCORE_JOIN_ENDPOINT` | `--join` |
| `KSCORE_JOIN_TOKEN` | `--join-token` |
| `KSCORE_STORAGE_BACKEND` | `--storage-backend` |
| `KSCORE_NATS_MODE` | `--nats-mode` |
| `KSCORE_NATS_URLS` | `--nats-urls` (comma-separated) |
| `KSCORE_BIND_ADDRESS` | `--bind-address` |
| `KSCORE_ADVERTISE_ADDRESS` | `--advertise-address` |
| `KSCORE_POSTGRES_HOST` | `--postgres-host` |
| `KSCORE_POSTGRES_PORT` | `--postgres-port` |
| `KSCORE_POSTGRES_DATABASE` | `--postgres-database` |
| `KSCORE_POSTGRES_USER` | `--postgres-user` |
| `KSCORE_POSTGRES_PASSWORD` | `--postgres-password` |
| `KSCORE_POSTGRES_SSLMODE` | `--postgres-sslmode` |
| `KSCORE_GENERATE_CERTS` | `--generate-certs` |
| `KSCORE_TLS_CERT_FILE` | `--tls-cert-file` |
| `KSCORE_TLS_KEY_FILE` | `--tls-key-file` |
| `KSCORE_TLS_CA_FILE` | `--tls-ca-file` |
| `KSCORE_TLS_CSR_FILE` | `--tls-csr-file` |
| `KSCORE_TLS_RENEWAL_COMMAND` | `--tls-renewal-command` |
| `KSCORE_TLS_RENEWAL_SCRIPT` | `--tls-renewal-script` |
| `KSCORE_NATS_CREDS_FILE` | `--nats-creds-file` |
| `KSCORE_NATS_USER` | `--nats-user` |
| `KSCORE_NATS_PASSWORD` | `--nats-password` |
| `KSCORE_PACKAGE_CHANNEL` | `--package-channel` |
| `KSCORE_PACKAGE_VERSION` | `--package-version` |
| `KSCORE_MIGRATE_FROM_SQLITE` | `--migrate-from-sqlite` |
| `KSCORE_MIGRATE_BATCH_SIZE` | `--migrate-batch-size` |
| `KSCORE_MIGRATE_CONTINUE_ON_ERROR` | `--migrate-continue-on-error` |
| `KSCORE_MIGRATE_SKIP_EXISTING` | `--migrate-skip-existing` |
| `KSCORE_BLUEPRINTS_DIR` | `--blueprints-dir` |
| `KSCORE_APPLY_BLUEPRINTS` | `--apply-blueprint` (comma-separated) |
| `KSCORE_BLUEPRINT_PARAMS` | `--blueprint-param` |
| `KSCORE_BLUEPRINT_FEATURES` | `--blueprint-feature` |
| `KSCORE_BLUEPRINT_ENTRYPOINTS` | `--blueprint-entrypoint` |
| `KSCORE_EXPORT_STATES_DIR` | `--export-states-dir` |
| `KSCORE_BOOTSTRAP_NON_INTERACTIVE` | `--non-interactive` |

## kscore-server (Control Plane)

The control plane server daemon. It's not invoked via kscorectl.

### Running the Server

```bash
# Run in foreground (development - uses ./keystone-core.yaml)
kscore-server

# Run with explicit config (production)
kscore-server --config /etc/keystone-core/server.yaml

# Show version
kscore-server version
```

### Server Flags

```
--config string   Config file path (default: ./keystone-core.yaml)
-h, --help        Show help
```

> **Note**: In development, the server looks for `keystone-core.yaml` in the current directory.
> For production deployments (systemd), use `/etc/keystone-core/server.yaml`.

## Shell Completion

Enable command completion:

**Bash**:

```bash
echo 'source <(kscorectl completion bash)' >> ~/.bashrc
```

**Zsh**:

```bash
echo 'source <(kscorectl completion zsh)' >> ~/.zshrc
```

**Fish**:

```bash
kscorectl completion fish > ~/.config/fish/completions/kscorectl.fish
```

## Examples

### Common Workflows

**Deploy and verify**:

```bash
# Apply state
kscorectl state apply web-server.yaml

# Check for drift
kscorectl state drift web-server.yaml

# Restart services if needed
kscorectl exec run "systemctl restart nginx" --target "role:web"
```

**Investigate issues**:

```bash
# Check agent status
kscorectl monitor --view 2

# View recent events
kscorectl monitor --view 3

# Check logs
kscorectl monitor --view 7
```

**Compliance check**:

```bash
# Check compliance status
kscorectl policy compliance --days 30

# List violations
kscorectl policy violations --limit 50

# Remediate
kscorectl state apply security-baseline.yaml \
  --dry-run
```

## kscore-files (File Distribution Server)

The file distribution server for Keystone Core. Manages file storage, distribution, and synchronization across clusters.

### Global Flags

These flags apply to all kscore-files commands:

| Flag | Description | Default |
|------|-------------|---------|
| `--config string` | Configuration file path | |
| `--nats-url string` | NATS server URL | `nats://localhost:4222` |
| `--cluster-id string` | Cluster identifier for routing | |
| `--instance-id string` | Instance identifier for HA deployments | |
| `--audit-level string` | Audit logging level (all, errors, none) | `all` |
| `--audit-output string` | Audit output backend (auto, syslog, journald, stderr, none) | `auto` |
| `-h, --help` | Show help | |

### serve

Run the file distribution server.

```bash
kscore-files serve [flags]
```

The serve command uses global flags for configuration. See [Global Flags](#global-flags-11) above.

**Examples:**

```bash
# Run with configuration file
kscore-files serve --config /etc/keystone-core/files.yaml

# Run with NATS connection
kscore-files serve --nats-url nats://localhost:4222

# Run with HA configuration
kscore-files serve --config /etc/keystone-core/files.yaml --instance-id files-1
```

### files

Manage files in the distribution system.

#### files list

List files in a namespace.

```bash
kscore-files list <namespace> [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--path string` | Filter by path prefix | |
| `--recursive` | List files recursively | `false` |
| `-o, --output string` | Output format: text, json, yaml, table | `text` |

**Examples:**

```bash
# List all files in packages namespace
kscore-files list packages

# List files recursively with path filter
kscore-files list packages --path /myapp --recursive

# Output as JSON
kscore-files list packages -o json
```

#### files put

Upload a file to a namespace.

```bash
kscore-files put <local-path> <namespace>/<remote-path> [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--content-type string` | Content type for the file | auto-detect |
| `--metadata string` | JSON metadata to attach to file | |
| `--overwrite` | Overwrite existing file | `false` |

**Examples:**

```bash
# Upload a file
kscore-files put ./myapp-1.0.0.tar.gz packages/myapp/v1.0.0.tar.gz

# Upload with metadata
kscore-files put ./config.yaml configs/app/config.yaml --metadata '{"version":"1.0"}'
```

#### files get

Download a file from a namespace.

```bash
kscore-files get <namespace>/<remote-path> <local-path> [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--verify` | Verify checksum after download | `true` |

**Examples:**

```bash
# Download a file
kscore-files get packages/myapp/v1.0.0.tar.gz ./myapp.tar.gz
```

#### files delete

Delete a file from a namespace.

```bash
kscore-files delete <namespace>/<path> [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--force` | Delete without confirmation | `false` |
| `--dry-run` | Show what would be deleted | `false` |

#### files info

Show detailed file information.

```bash
kscore-files info <namespace>/<path> [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `-o, --output string` | Output format: text, json, yaml | `text` |

### cache

Manage file caching. Cache commands help optimize file distribution by pre-warming caches and managing cached content.

#### cache status

Get cache status for a specific cache or all caches.

```bash
kscore-files cache status [name] [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `-o, --output string` | Output format: text, json, yaml, table | `text` |

**Examples:**

```bash
# Show status of all caches
kscore-files cache status

# Show status of specific cache
kscore-files cache status edge-cache-1

# Output as JSON
kscore-files cache status -o json
```

#### cache clear

Clear cache contents.

```bash
kscore-files cache clear [name] [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--pattern string` | Only clear entries matching pattern (glob) | |
| `--force` | Clear without confirmation | `false` |
| `--older string` | Only clear entries older than duration (e.g., 24h, 7d) | |
| `--dry-run` | Show what would be cleared | `false` |

**Examples:**

```bash
# Clear all caches (with confirmation)
kscore-files cache clear

# Clear specific cache
kscore-files cache clear edge-cache-1

# Clear entries matching pattern
kscore-files cache clear --pattern "packages/*.tar.gz"

# Clear entries older than 7 days
kscore-files cache clear --older 7d

# Preview what would be cleared
kscore-files cache clear --older 24h --dry-run
```

#### cache warm

Pre-warm the cache with files from a path.

```bash
kscore-files cache warm <path> [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--namespace string` | Namespace to warm from | |
| `--recursive` | Warm files recursively | `false` |
| `--dry-run` | Show what would be warmed | `false` |

**Examples:**

```bash
# Warm cache with files from packages namespace
kscore-files cache warm /myapp --namespace packages

# Warm recursively
kscore-files cache warm / --namespace configs --recursive

# Preview what would be warmed
kscore-files cache warm /releases --namespace packages --dry-run
```

#### cache list

List cached entries.

```bash
kscore-files cache list [name] [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--pattern string` | Filter entries matching pattern (glob) | |
| `--limit int` | Maximum number of entries to show | `100` |
| `--sort-by string` | Sort by: name, size, accessed, created | `accessed` |
| `-o, --output string` | Output format: text, json, yaml, table | `table` |

**Examples:**

```bash
# List all cached entries
kscore-files cache list

# List entries in specific cache
kscore-files cache list edge-cache-1

# List with pattern filter
kscore-files cache list --pattern "*.tar.gz" --limit 50

# Sort by size
kscore-files cache list --sort-by size
```

#### cache evict

Evict a specific entry from the cache.

```bash
kscore-files cache evict <path> [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--force` | Evict without confirmation | `false` |
| `--dry-run` | Show what would be evicted | `false` |

**Examples:**

```bash
# Evict a specific entry
kscore-files cache evict packages/myapp/v1.0.0.tar.gz

# Force evict without confirmation
kscore-files cache evict packages/old-package.tar.gz --force
```

#### cache stats

Get detailed cache statistics.

```bash
kscore-files cache stats [name] [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `-o, --output string` | Output format: text, json, yaml | `text` |

**Examples:**

```bash
# Show stats for all caches
kscore-files cache stats

# Show stats for specific cache
kscore-files cache stats edge-cache-1 -o json
```

**Output includes:**

- Total entries and size
- Hit/miss ratios
- Eviction statistics
- Age distribution
- Top accessed files

### namespace (ns)

Manage file namespaces. Namespaces provide logical separation of files with independent access controls and quotas.

#### namespace list

List all namespaces.

```bash
kscore-files namespace list [flags]
kscore-files ns list [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `-o, --output string` | Output format: text, json, yaml, table | `table` |

**Examples:**

```bash
# List all namespaces
kscore-files namespace list

# Using alias
kscore-files ns list -o json
```

#### namespace create

Create a new namespace.

```bash
kscore-files namespace create <name> [flags]
kscore-files ns create <name> [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--backend string` | Backend to use for storage | default backend |
| `--path string` | Base path within backend | |
| `--description string` | Namespace description | |
| `--max-size string` | Maximum total size (e.g., 10GB, 100MB) | unlimited |
| `--max-files int` | Maximum number of files | unlimited |
| `--read-only` | Create as read-only namespace | `false` |
| `--dry-run` | Show what would be created | `false` |

**Examples:**

```bash
# Create a simple namespace
kscore-files namespace create packages

# Create with description and quota
kscore-files namespace create configs \
  --description "Application configuration files" \
  --max-size 1GB \
  --max-files 1000

# Create on specific backend
kscore-files namespace create archives \
  --backend s3-backup \
  --path /archives \
  --read-only

# Preview creation
kscore-files ns create test-ns --dry-run
```

#### namespace delete

Delete a namespace.

```bash
kscore-files namespace delete <name> [flags]
kscore-files ns delete <name> [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--force` | Delete without confirmation (also deletes all files) | `false` |
| `--dry-run` | Show what would be deleted | `false` |

**Examples:**

```bash
# Delete namespace (prompts for confirmation)
kscore-files namespace delete old-packages

# Force delete without confirmation
kscore-files ns delete temp-ns --force

# Preview deletion
kscore-files namespace delete archives --dry-run
```

#### namespace info

Get detailed namespace information.

```bash
kscore-files namespace info <name> [flags]
kscore-files ns info <name> [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `-o, --output string` | Output format: text, json, yaml | `text` |

**Examples:**

```bash
# Show namespace info
kscore-files namespace info packages

# Output as YAML
kscore-files ns info configs -o yaml
```

**Output includes:**

- Namespace name and description
- Backend and path configuration
- Quota settings and usage
- Access control settings
- File count and total size

#### namespace quota

Set namespace quota limits.

```bash
kscore-files namespace quota <name> [flags]
kscore-files ns quota <name> [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--max-size string` | Maximum total size (e.g., 10GB, 100MB) | |
| `--max-files int` | Maximum number of files | |
| `--max-file-size string` | Maximum single file size | |
| `--clear` | Clear all quota limits | `false` |
| `--dry-run` | Show what would be changed | `false` |

**Examples:**

```bash
# Set size quota
kscore-files namespace quota packages --max-size 50GB

# Set multiple quotas
kscore-files ns quota configs \
  --max-size 1GB \
  --max-files 5000 \
  --max-file-size 10MB

# Clear all quotas
kscore-files namespace quota archives --clear

# Preview changes
kscore-files ns quota packages --max-size 100GB --dry-run
```

#### namespace access

Set namespace access controls.

```bash
kscore-files namespace access <name> [flags]
kscore-files ns access <name> [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--read-only` | Set namespace to read-only | `false` |
| `--read-write` | Set namespace to read-write | `false` |
| `--allow-ip string` | Add allowed IP/CIDR (can be repeated) | |
| `--deny-ip string` | Add denied IP/CIDR (can be repeated) | |
| `--allow-user string` | Add allowed user/role (can be repeated) | |
| `--deny-user string` | Add denied user/role (can be repeated) | |
| `--clear` | Clear all access controls | `false` |
| `--dry-run` | Show what would be changed | `false` |

**Examples:**

```bash
# Set read-only
kscore-files namespace access archives --read-only

# Allow specific IPs
kscore-files ns access packages \
  --allow-ip 10.0.0.0/8 \
  --allow-ip 192.168.1.0/24

# Set user-based access
kscore-files namespace access configs \
  --allow-user admin \
  --allow-user role:ops \
  --deny-user guest

# Clear all access controls
kscore-files ns access temp --clear

# Preview changes
kscore-files namespace access packages --read-only --dry-run
```

### backend

Manage storage backends.

> **Note:** Backend management commands are also available in `kscore-files-storage` which provides additional functionality.

#### backend list

List configured storage backends.

```bash
kscore-files backend list [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `-o, --output string` | Output format: text, json, yaml, table | `table` |

**Examples:**

```bash
# List all backends
kscore-files backend list

# Output as JSON
kscore-files backend list -o json
```

#### backend status

Get status of a specific backend.

```bash
kscore-files backend status <name> [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `-o, --output string` | Output format: text, json, yaml | `text` |

**Examples:**

```bash
# Show backend status
kscore-files backend status s3-primary

# Output as YAML
kscore-files backend status local-storage -o yaml
```

**Output includes:**

- Backend type and configuration
- Connection status
- Storage capacity and usage
- Error counts and last error

#### backend sync

Synchronize files between backends.

```bash
kscore-files backend sync <source> <destination> [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--dry-run` | Show what would be synced | `false` |
| `--force` | Sync even if destination has newer files | `false` |

**Examples:**

```bash
# Sync from primary to backup
kscore-files backend sync s3-primary s3-backup

# Preview sync operation
kscore-files backend sync local-storage s3-archive --dry-run

# Force sync (overwrites newer files)
kscore-files backend sync primary secondary --force
```

#### backend enable

Enable a disabled backend.

```bash
kscore-files backend enable <name> [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--dry-run` | Show what would be changed | `false` |

#### backend disable

Disable a backend (prevents reads and writes).

```bash
kscore-files backend disable <name> [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--dry-run` | Show what would be changed | `false` |

#### backend health

Check health of all backends.

```bash
kscore-files backend health [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `-o, --output string` | Output format: text, json, yaml, table | `table` |

**Examples:**

```bash
# Check all backends health
kscore-files backend health

# Output as JSON for monitoring
kscore-files backend health -o json
```

### mirrors

Manage mirror groups for file replication and geographic distribution.

> **Note:** Mirror management commands are also available in `kscore-files-storage` which provides additional functionality.

#### mirrors list

List all mirror groups.

```bash
kscore-files mirrors list [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `-o, --output string` | Output format: text, json, yaml, table | `table` |

**Examples:**

```bash
# List all mirror groups
kscore-files mirrors list

# Output as JSON
kscore-files mirrors list -o json
```

#### mirrors show

Show detailed information about a mirror group.

```bash
kscore-files mirrors show <group-id> [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `-o, --output string` | Output format: text, json, yaml | `text` |

**Examples:**

```bash
# Show mirror group details
kscore-files mirrors show us-east-mirrors

# Output as YAML
kscore-files mirrors show global-mirrors -o yaml
```

**Output includes:**

- Group ID and description
- Member mirrors with health status
- Sync configuration
- Routing strategy
- Recent sync status

#### mirrors sync-status

Check synchronization status for a mirror group.

```bash
kscore-files mirrors sync-status <group-id> [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `-o, --output string` | Output format: text, json, yaml | `text` |

**Examples:**

```bash
# Check sync status
kscore-files mirrors sync-status us-east-mirrors

# Output as JSON for monitoring
kscore-files mirrors sync-status global-mirrors -o json
```

**Output includes:**

- Sync state (in-sync, syncing, error)
- Last sync time
- Pending files count
- Sync lag duration

#### mirrors sync

Trigger a manual synchronization.

```bash
kscore-files mirrors sync <group-id> [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--path string` | Only sync specific path | |
| `--source string` | Source mirror for sync | primary |
| `--target string` | Target mirror(s) for sync | all |
| `--priority string` | Sync priority: low, normal, high | `normal` |
| `--wait` | Wait for sync to complete | `false` |
| `--dry-run` | Show what would be synced | `false` |

**Examples:**

```bash
# Trigger sync for mirror group
kscore-files mirrors sync us-east-mirrors

# Sync specific path only
kscore-files mirrors sync global-mirrors --path /packages/critical

# Sync to specific target
kscore-files mirrors sync us-mirrors --target us-west-1

# Wait for sync completion
kscore-files mirrors sync production --wait

# Preview sync
kscore-files mirrors sync us-east-mirrors --dry-run
```

#### mirrors health

Show health status of mirrors.

```bash
kscore-files mirrors health [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--group string` | Filter by mirror group | |
| `-o, --output string` | Output format: text, json, yaml, table | `table` |

**Examples:**

```bash
# Show health of all mirrors
kscore-files mirrors health

# Filter by group
kscore-files mirrors health --group us-east-mirrors

# Output as JSON
kscore-files mirrors health -o json
```

#### mirrors failover

Force failover to a specific mirror.

```bash
kscore-files mirrors failover <group-id> [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--to string` | Target mirror ID for failover | |
| `--dry-run` | Show what would happen | `false` |

**Examples:**

```bash
# Failover to specific mirror
kscore-files mirrors failover us-east-mirrors --to us-east-2

# Preview failover
kscore-files mirrors failover production --to backup-dc --dry-run
```

#### mirrors latency

Show latency matrix between mirrors.

```bash
kscore-files mirrors latency [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--group string` | Filter by mirror group | |
| `-o, --output string` | Output format: text, json, yaml, table | `table` |

**Examples:**

```bash
# Show latency matrix
kscore-files mirrors latency

# Filter by group
kscore-files mirrors latency --group global-mirrors
```

#### mirrors conflicts

List unresolved sync conflicts.

```bash
kscore-files mirrors conflicts [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--group string` | Filter by mirror group | |
| `-o, --output string` | Output format: text, json, yaml, table | `table` |

**Examples:**

```bash
# List all conflicts
kscore-files mirrors conflicts

# Filter by group
kscore-files mirrors conflicts --group us-mirrors

# Output as JSON
kscore-files mirrors conflicts -o json
```

#### mirrors resolve-conflict

Resolve a sync conflict.

```bash
kscore-files mirrors resolve-conflict <conflict-id> [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--strategy string` | Resolution strategy: newest-wins, largest-wins, primary-wins, source, target | |
| `--dry-run` | Show what would happen | `false` |

**Strategies:**

| Strategy | Description |
|----------|-------------|
| `newest-wins` | Keep the file with the most recent modification time |
| `largest-wins` | Keep the larger file |
| `primary-wins` | Keep the file from the primary mirror |
| `source` | Keep the source file (for this specific conflict) |
| `target` | Keep the target file (for this specific conflict) |

**Examples:**

```bash
# Resolve using newest-wins strategy
kscore-files mirrors resolve-conflict conflict-12345 --strategy newest-wins

# Preview resolution
kscore-files mirrors resolve-conflict conflict-67890 --strategy primary-wins --dry-run
```

#### mirrors history

Show synchronization history.

```bash
kscore-files mirrors history [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--limit int` | Maximum entries to show | `50` |
| `--group string` | Filter by mirror group | |
| `-o, --output string` | Output format: text, json, yaml, table | `table` |

**Examples:**

```bash
# Show recent sync history
kscore-files mirrors history

# Show more entries
kscore-files mirrors history --limit 100

# Filter by group
kscore-files mirrors history --group us-east-mirrors

# Output as JSON
kscore-files mirrors history -o json
```

## kscore-files-storage (File Storage Administration)

Admin CLI for file distribution storage backends and mirror groups.

### Global Flags

These flags apply to all kscore-files-storage commands:

- `--nats-url string`: NATS server URL (default: nats://localhost:4222)
- `--cluster-id string`: Cluster identifier for NATS subjects
- `-o, --output string`: Output format: table, text, json, yaml (default: table)
- `--audit-level string`: Audit logging level: all, errors, none (default: all)
- `--audit-output string`: Audit output backend: auto, syslog, journald, stderr, none

### files-storage backend

Manage storage backends.

```bash
kscorectl files-storage backend <command> [flags]
```

**Commands**:

- `list`: List configured backends
- `status <name>`: Get backend status
- `sync <source> <destination>`: Synchronize backends
- `enable <name>`: Enable a backend
- `disable <name>`: Disable a backend
- `health`: Check health of all backends

**Sync Flags**:

- `--dry-run`: Show what would be done
- `--force`: Force sync even if destination has newer files

### files-storage mirrors

Manage mirror groups.

```bash
kscorectl files-storage mirrors <command> [flags]
```

**Commands**:

- `list`: List mirror groups
- `show <group-id>`: Show mirror group details
- `sync <group-id>`: Trigger manual sync
- `health`: Show mirror health
- `conflicts`: List unresolved conflicts

**Sync Flags**:

- `--dry-run`: Show what would be done

## kscore-bootstrap (Cluster Bootstrap Tool)

Bootstraps and initializes new Keystone Core clusters. Used for initial setup, disaster recovery, and cluster migration.

### Basic Usage

```bash
# Bootstrap a new cluster with seed configuration
kscore-bootstrap seed --config bootstrap.yaml --output-dir /etc/keystone-core

# Show version
kscore-bootstrap version
```

### Common Flags

```
--config string        Configuration file path
--output-dir string    Output directory for generated files
--verbose              Enable verbose output
--dry-run              Show what would be done without making changes
--skip-verification    Skip verification steps
--timeout duration     Operation timeout (default: 5m)
-h, --help             Show help
```

### Seed Command

Initialize a new cluster from scratch:

```bash
# Create seed configuration
kscore-bootstrap seed \
  --config bootstrap.yaml \
  --output-dir /etc/keystone-core \
  --cluster-name production \
  --trust-domain example.com

# Seed with specific NATS configuration
kscore-bootstrap seed \
  --config bootstrap.yaml \
  --nats-mode embedded \
  --output-dir /etc/keystone-core

# Dry run to preview
kscore-bootstrap seed --config bootstrap.yaml --dry-run
```

**Seed Flags**:

```
--cluster-name string   Cluster name identifier
--trust-domain string   SPIFFE trust domain
--nats-mode string      NATS mode: embedded, external, leaf
--ca-ttl duration       CA certificate TTL (default: 10y)
--svid-ttl duration     SVID default TTL (default: 1h)
```

### Restore Command

Restore a cluster from backup:

```bash
# Restore from backup
kscore-bootstrap restore \
  --backup-path /backups/cluster-backup.tar.gz \
  --output-dir /etc/keystone-core

# Restore with verification
kscore-bootstrap restore \
  --backup-path /backups/cluster-backup.tar.gz \
  --verify-integrity

# Restore specific components
kscore-bootstrap restore \
  --backup-path /backups/cluster-backup.tar.gz \
  --components ca,config,state
```

**Restore Flags**:

```
--backup-path string    Path to backup archive
--verify-integrity      Verify backup integrity before restore
--components strings    Specific components to restore (ca,config,state,nats)
--force                 Force restore even if cluster exists
```

### Import Command

Import configuration from external sources:

```bash
# Import from another cluster
kscore-bootstrap import \
  --source https://old-cluster:8080 \
  --output-dir /etc/keystone-core

# Import from configuration export
kscore-bootstrap import \
  --source /exports/cluster-export.yaml \
  --format yaml

# Import with transformation
kscore-bootstrap import \
  --source /exports/config.yaml \
  --transform trust-domain=new.example.com
```

**Import Flags**:

```
--source string      Source URL or file path
--format string      Source format: yaml, json, tar
--transform strings  Key=value transformations to apply
--merge              Merge with existing configuration
```

### Validate Command

Validate a seed configuration file:

```bash
# Validate configuration
kscore-bootstrap validate seed-config.yaml

# Output as JSON
kscore-bootstrap validate seed-config.yaml --output json
```

**Validate Flags**:

```
-o, --output string    Output format: text, json, yaml, table (default: text)
```

### Status Command

Check cluster bootstrap status:

```bash
# Show bootstrap status
kscore-bootstrap status

# Output as JSON
kscore-bootstrap status --output json
```

**Status Flags**:

```
-o, --output string    Output format: text, json, yaml, table (default: text)
```

**Status Output**:

```
Bootstrap Status:
  Cluster Name: production
  Trust Domain: example.com
  State: initialized
  CA Status: healthy (expires in 9y 364d)
  NATS Mode: embedded
  Components:
    - Control Plane: configured
    - Identity Provider: configured
    - File Server: configured
```

### Cleanup Command

Clean up bootstrap artifacts:

```bash
# Clean up temporary files
kscore-bootstrap cleanup

# Clean up with confirmation
kscore-bootstrap cleanup --interactive

# Force cleanup without prompts
kscore-bootstrap cleanup --force
```

**Cleanup Flags**:

```
--force          Force cleanup without confirmation
--interactive    Prompt before each deletion
--keep-backups   Preserve backup files
```

### Join Command

Join a node to an existing cluster:

```bash
# Join an existing cluster
kscore-bootstrap join --server https://ks-server-1:8080 --token $JOIN_TOKEN

# Join with a custom node name
kscore-bootstrap join --server https://ks-server-1:8080 --token $JOIN_TOKEN --node ks-server-2

# Join with an advertise address
kscore-bootstrap join --server https://ks-server-1:8080 --token $JOIN_TOKEN --advertise-address 10.0.1.11
```

**Join Flags**:

```
--server string              URL of an existing cluster member (required)
--token string               Join token for cluster authentication (required)
--node string                Name for this node (default: hostname)
--advertise-address string   Address this node advertises to the cluster
--debug                      Enable debug output
```

### Prereq-Check Command

Check system prerequisites before bootstrapping:

```bash
# Check prerequisites
kscore-bootstrap prereq-check

# Output as JSON
kscore-bootstrap prereq-check --output json
```

**Prereq-Check Flags**:

```
-o, --output string    Output format: text, json (default: text)
```

**Checks Performed**:

- OS compatibility
- Memory (minimum 1GB)
- Disk space
- Required ports available (8080, 4222, 2379, 2380)
- Network connectivity

### Cert-Gen Command

Generate TLS certificates for cluster components:

```bash
# Generate certificates with defaults
kscore-bootstrap cert-gen --output /etc/keystone-core/certs/

# Generate with custom names
kscore-bootstrap cert-gen \
  --ca-cn "My Org CA" \
  --server-cn $(hostname -f) \
  --output /etc/keystone-core/certs/
```

**Cert-Gen Flags**:

```
--ca-cn string       Common Name for the CA certificate (default: "Keystone Core CA")
--server-cn string   Common Name for the server certificate (default: hostname)
--output string      Output directory for certificates (default: /etc/keystone-core/certs)
```

**Generated Files**:

| File | Description |
|------|-------------|
| `ca.pem` | CA certificate |
| `ca-key.pem` | CA private key |
| `server.pem` | Server certificate |
| `server-key.pem` | Server private key |

### package

Manage self-contained bootstrap packages for air-gapped deployments.

#### package create

Create a bootstrap package containing binaries, configuration templates,
and optional content (modules, blueprints, policies, documentation).

```bash
# Create a basic package
kscore-bootstrap package create --version 0.1.0 --platform linux/amd64 --build-dir build/bin

# Create a signed package with all content
kscore-bootstrap package create \
  --version 0.1.0 \
  --platform linux/amd64 \
  --build-dir build/bin \
  --signing-key key.pem \
  --include-modules --modules-dir modules/ \
  --include-blueprints --blueprints-dir blueprints/ \
  --include-docs --docs-dir docs/

# Create with custom output path
kscore-bootstrap package create --version 0.1.0 --platform linux/arm64 --build-dir build/bin -o /tmp/ks-arm64.tar.gz
```

**Package Create Flags**:

```
--version string          Package version (required)
--platform string         Target platform os/arch (default: linux/amd64)
--build-dir string        Directory containing compiled binaries (default: build/bin)
--signing-key string      Path to PEM private key for signing
--include-modules         Include modules in the package
--include-blueprints      Include blueprints in the package
--include-docs            Include offline documentation
--modules-dir string      Source directory for modules
--blueprints-dir string   Source directory for blueprints
--docs-dir string         Source directory for documentation
--policies-dir string     Source directory for policy files (.rego, .cel)
-o, --output string       Output archive path (default: auto-generated)
--created-by string       Creator identifier for manifest metadata
```

#### package verify

Verify the integrity and authenticity of a bootstrap package.

```bash
# Verify a signed package
kscore-bootstrap package verify keystone-bootstrap-0.1.0-linux-amd64.tar.gz --trusted-key cosign.pub

# Verify with multiple trusted keys
kscore-bootstrap package verify package.tar.gz --trusted-key key1.pub --trusted-key key2.pub
```

**Package Verify Flags**:

```
--trusted-key strings     Path to trusted public key (repeatable)
```

#### package install

Install a bootstrap package to the local system.

```bash
# Install with defaults
kscore-bootstrap package install keystone-bootstrap-0.1.0-linux-amd64.tar.gz

# Install with verification
kscore-bootstrap package install --verify --trusted-key cosign.pub package.tar.gz

# Install to custom directories
kscore-bootstrap package install \
  --target-dir /opt/keystone/bin \
  --config-dir /opt/keystone/etc \
  --data-dir /opt/keystone/data \
  package.tar.gz

# Install without modules/blueprints
kscore-bootstrap package install --skip-modules package.tar.gz
```

**Package Install Flags**:

```
--verify                  Verify package signatures before installing
--trusted-key strings     Path to trusted public key (repeatable)
--target-dir string       Binary installation directory (default: /usr/local/bin)
--config-dir string       Configuration directory (default: /etc/keystone-core)
--data-dir string         Data directory (default: /var/lib/keystone-core)
--unattended              Skip confirmation prompts
--skip-modules            Skip module and blueprint installation
```

#### package inspect

Display the manifest of a bootstrap package as JSON.

```bash
kscore-bootstrap package inspect keystone-bootstrap-0.1.0-linux-amd64.tar.gz
```

## kscore-telemetry-gateway (Telemetry Aggregation Gateway)

Aggregates metrics, logs, and traces from agents over NATS and exposes them to standard observability backends (Prometheus, Loki, Tempo/Jaeger).

### Commands

#### serve

Start the telemetry gateway server.

```bash
# Start with default settings
kscore-telemetry-gateway serve

# Start with custom config file
kscore-telemetry-gateway serve --config /etc/keystone-core/gateway.yaml

# Override specific settings
kscore-telemetry-gateway serve --listen 0.0.0.0:9091 --nats-url nats://nats:4222
```

#### version

Print version information.

```bash
kscore-telemetry-gateway version
```

### Global Flags

These flags are available for all commands:

```
--config string     Path to configuration file
--listen string     Listen address (overrides config)
--nats-url string   NATS server URL (overrides config)
--metrics           Enable metrics gateway (default: true)
--logs              Enable logs gateway (default: true)
--traces            Enable traces gateway (default: true)
-h, --help          Show help
```

### Endpoints

When running, the gateway exposes the following endpoints:

| Endpoint | Description | Default Path |
|----------|-------------|--------------|
| Metrics | Prometheus metrics scrape endpoint | `/metrics` |
| Federation | Prometheus federation endpoint | `/federate` |
| Health | Health check endpoint | `/health` |
| Ready | Readiness check endpoint | `/ready` |

### Features

#### Metrics Gateway

The metrics gateway:

- Subscribes to `kscore.telemetry.metrics.>` on NATS (configurable)
- Aggregates metrics from all agents
- Exposes `/metrics` for Prometheus scraping
- Exposes `/federate` for Prometheus federation
- Supports label transformations (add, drop, rewrite)
- Cardinality control (max series, max labels per series)
- Remote write support for pushing to Prometheus

**Prometheus Scrape Configuration**:

```yaml
scrape_configs:
  - job_name: 'kscore-gateway'
    static_configs:
      - targets: ['gateway:9091']
```

#### Logs Gateway

The logs gateway:

- Subscribes to `kscore.telemetry.logs.>` on NATS (configurable)
- Buffers and batches logs
- Pushes to Loki via push API
- Supports log level filtering (debug, info, warn, error, fatal)
- Source filtering (include/exclude patterns)
- Multi-tenant support via X-Scope-OrgID
- Optional Elasticsearch output

#### Traces Gateway

The traces gateway:

- Subscribes to `kscore.telemetry.traces.>` on NATS (configurable)
- Groups spans into traces
- Exports via OTLP to Tempo/Jaeger (gRPC or HTTP)
- Supports sampling configuration
- Priority sampling for error and slow traces
- Configurable compression (gzip)

### Configuration Reference

#### Complete Configuration Example

```yaml
# /etc/keystone-core/gateway.yaml

# NATS connection settings
nats:
  urls:
    - "nats://localhost:4222"
  cluster: "default"
  credentials_file: ""
  max_reconnects: -1           # -1 for infinite
  reconnect_wait: "2s"
  reconnect_jitter: "500ms"
  tls:
    enabled: false
    cert_file: ""
    key_file: ""
    ca_file: ""
    insecure: false

# HTTP server settings
server:
  listen: "0.0.0.0:9091"
  metrics_path: "/metrics"
  health_path: "/health"
  ready_path: "/ready"
  federate_path: "/federate"
  read_timeout: "30s"
  write_timeout: "30s"

# Metrics gateway configuration
metrics:
  enabled: true
  subject: "kscore.telemetry.metrics.>"
  stale_timeout: "60s"         # Remove agents not seen for this duration
  labels:
    add:                       # Add labels to all metrics
      environment: "production"
    drop:                      # Drop these label names
      - "internal_id"
    rewrite:                   # Rewrite label names
      - source: "old_name"
        target: "new_name"
  cardinality:
    max_series: 100000
    max_labels_per_series: 20
    drop_high_cardinality: false
  remote_write:
    enabled: false
    url: "http://prometheus:9090/api/v1/write"
    batch_size: 1000
    flush_interval: "15s"
    auth:
      type: "none"             # none, basic, bearer
      username: ""
      password: ""
      token: ""
    tls:
      enabled: false
    retry:
      max_attempts: 3
      backoff: "1s"
      max_backoff: "30s"
  federation:
    enabled: true
    path: "/federate"

# Logs gateway configuration
logs:
  enabled: true
  subject: "kscore.telemetry.logs.>"
  min_level: "info"            # debug, info, warn, error, fatal
  sources:
    include: []
    exclude: []
  loki:
    enabled: false
    url: "http://loki:3100/loki/api/v1/push"
    batch_size: 100
    batch_wait: "1s"
    tenant_id: ""
    labels:
      - "agent_id"
      - "level"
      - "source"
    auth:
      type: "none"
    tls:
      enabled: false
    retry:
      max_attempts: 3
      backoff: "1s"
      max_backoff: "30s"
  elasticsearch:
    enabled: false
    urls:
      - "http://elasticsearch:9200"
    index: "kscore-logs-%Y.%m.%d"
    batch_size: 500
    auth:
      type: "none"
    tls:
      enabled: false

# Traces gateway configuration
traces:
  enabled: true
  subject: "kscore.telemetry.traces.>"
  sampling:
    enabled: true
    rate: 1.0                  # 0.0 to 1.0 (1.0 = 100%)
    priority_sample:
      errors: true             # Always sample error traces
      slow_threshold: "1s"     # Always sample traces slower than this
  otlp:
    enabled: false
    endpoint: "tempo:4317"
    protocol: "grpc"           # grpc, http
    compression: "gzip"        # gzip, none
    batch_size: 100
    flush_interval: "5s"
    headers: {}
    tls:
      enabled: false

# High availability configuration
ha:
  enabled: false
  queue_group: "kscore-gateway"
  leader_election:
    enabled: false
    lease_duration: "15s"
    renew_deadline: "10s"
```

#### Configuration Sections

**nats** - NATS connection:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `urls` | []string | `["nats://localhost:4222"]` | NATS server URLs |
| `cluster` | string | `"default"` | Cluster name for subject prefixing |
| `credentials_file` | string | `""` | Path to credentials file (JWT/NKey) |
| `max_reconnects` | int | `-1` | Max reconnect attempts (-1 = infinite) |
| `reconnect_wait` | duration | `2s` | Wait between reconnect attempts |
| `reconnect_jitter` | duration | `500ms` | Random jitter for reconnects |

**server** - HTTP server:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `listen` | string | `0.0.0.0:9091` | Listen address |
| `metrics_path` | string | `/metrics` | Prometheus metrics path |
| `health_path` | string | `/health` | Health check path |
| `ready_path` | string | `/ready` | Readiness check path |
| `federate_path` | string | `/federate` | Federation endpoint path |
| `read_timeout` | duration | `30s` | HTTP read timeout |
| `write_timeout` | duration | `30s` | HTTP write timeout |

**metrics.cardinality** - Cardinality control:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `max_series` | int | `100000` | Maximum total metric series |
| `max_labels_per_series` | int | `20` | Maximum labels per series |
| `drop_high_cardinality` | bool | `false` | Auto-drop high cardinality metrics |

**logs** - Log filtering:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `min_level` | string | `info` | Minimum log level (debug, info, warn, error, fatal) |
| `sources.include` | []string | `[]` | Only include these sources |
| `sources.exclude` | []string | `[]` | Exclude these sources |

**traces.sampling** - Trace sampling:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `rate` | float64 | `1.0` | Sampling rate (0.0 to 1.0) |
| `priority_sample.errors` | bool | `true` | Always sample error traces |
| `priority_sample.slow_threshold` | duration | `1s` | Always sample traces slower than this |

### High Availability

For HA deployments, enable HA mode in the configuration:

```yaml
ha:
  enabled: true
  queue_group: "kscore-gateway"
  leader_election:
    enabled: true
    lease_duration: "15s"
    renew_deadline: "10s"
```

Run multiple gateway instances with the same configuration. The queue group ensures agents are distributed across instances.

```bash
# Instance 1
kscore-telemetry-gateway serve --config /etc/keystone-core/gateway.yaml

# Instance 2 (same config)
kscore-telemetry-gateway serve --config /etc/keystone-core/gateway.yaml
```

Agents are distributed across instances using NATS queue groups. Leader election coordinates tasks that should only run on one instance (like remote write).

## kscore-backup (Backup Management)

Create, manage, verify, and restore backups of Keystone Core data including database, configuration, secrets, JetStream, etcd, and certificates.

### Global Flags

These flags apply to all kscore-backup commands:

- `--server string`: Control plane server address (default: localhost:9090)
- `-o, --output string`: Output format: table, json, yaml (default: table)
- `-v, --verbose`: Enable verbose output
- `--audit-level string`: Audit logging level: all, errors, none (default: all)
- `--audit-output string`: Audit output backend: auto, syslog, journald, stderr, none

### backup create

Create a new backup of Keystone Core data.

```bash
kscorectl backup create [flags]
```

**Flags**:

- `-t, --type string`: Backup type: full, incremental, database, configuration, jetstream, etcd, secrets (default: full)
- `-c, --components strings`: Specific components to backup (database, config, secrets, jetstream, etcd, certificates)
- `-d, --destination string`: Backup destination (local, s3://bucket/path, gs://bucket/path, azure://container/path)
- `-e, --encrypt`: Encrypt the backup
- `--compress`: Compress the backup (default: true)
- `--compression string`: Compression type: none, gzip, bzip2, xz, zstd, lz4 (default: gzip)
- `--compression-level int`: Compression level (0=default, algorithm-specific range)
- `-l, --label strings`: Labels to attach (key=value format)
- `--async`: Run backup asynchronously

**Rclone Flags** (for cloud storage via rclone):

- `--rclone-remote string`: Rclone remote name (configured via 'rclone config')
- `--rclone-path string`: Path within the rclone remote
- `--rclone-config string`: Path to rclone config file (default: ~/.config/rclone/rclone.conf)
- `--rclone-streaming`: Use streaming mode for rclone - pipes data directly without temp files (default: true)

**Compression Types**:

| Type | Extension | Description |
|------|-----------|-------------|
| none | (none) | No compression |
| gzip | .gz | Standard gzip, good balance of speed and ratio (default) |
| bzip2 | .bz2 | Higher compression ratio, slower than gzip |
| xz | .xz | Highest compression ratio, slowest |
| zstd | .zst | Fast compression with good ratio (recommended) |
| lz4 | .lz4 | Fastest compression, lower ratio |

**Rclone Destinations**:

The `--rclone-remote` flag enables backup to any of 50+ cloud storage providers supported by rclone, including:

- Dropbox, Google Drive, OneDrive, Box
- Backblaze B2, Wasabi, DigitalOcean Spaces
- SFTP, FTP, WebDAV
- And many more...

Configure remotes first with `rclone config`. With streaming mode enabled (default), data is piped directly to cloud storage without requiring local temporary storage.

**Examples**:

```bash
# Create a full backup
kscorectl backup create --type full

# Create an incremental backup
kscorectl backup create --type incremental

# Create a database-only backup to S3
kscorectl backup create --type database --destination s3://mybucket/backups

# Create encrypted backup with specific components
kscorectl backup create --type full --components database,config --encrypt

# Create backup with labels
kscorectl backup create --type full --label env=prod --label schedule=daily

# Create backup with zstd compression (fast and efficient)
kscorectl backup create --type full --compression zstd

# Create backup with maximum xz compression
kscorectl backup create --type full --compression xz --compression-level 9

# Create backup to Dropbox via rclone (streaming, no temp files)
kscorectl backup create --type full --rclone-remote dropbox --rclone-path /backups

# Create backup to Google Drive via rclone
kscorectl backup create --type full --rclone-remote gdrive --rclone-path backups/kscore

# Create backup to Backblaze B2 via rclone
kscorectl backup create --type full --rclone-remote b2 --rclone-path bucket/backups

# Create encrypted backup to S3-compatible storage via rclone
kscorectl backup create --type full --encrypt --rclone-remote minio --rclone-path backups
```

### backup list

List backups with optional filtering.

```bash
kscorectl backup list [flags]
```

**Flags**:

- `--last string`: Show backups from last duration (e.g., 24h, 7d)
- `-t, --type string`: Filter by backup type
- `--status string`: Filter by status (completed, failed, running)
- `-n, --limit int`: Maximum number of backups to show (default: 20)

**Examples**:

```bash
# List all backups
kscorectl backup list

# List backups from last 24 hours
kscorectl backup list --last 24h

# List only full backups
kscorectl backup list --type full

# List completed backups
kscorectl backup list --status completed

# List as JSON
kscorectl backup list -o json
```

### backup show

Show detailed information about a specific backup.

```bash
kscorectl backup show <backup-id>
```

**Examples**:

```bash
kscorectl backup show backup-20240115-060000
kscorectl backup show backup-20240115-060000 -o yaml
```

### backup verify

Verify the integrity and restorability of a backup.

```bash
kscorectl backup verify <backup-id> [flags]
```

**Flags**:

- `--check-integrity`: Verify component integrity (default: true)
- `--check-restorable`: Verify backup can be restored (performs restore simulation)
- `-v, --verbose`: Show detailed verification output

**Examples**:

```bash
# Basic verification
kscorectl backup verify backup-20240115-060000

# Full verification with restore check
kscorectl backup verify backup-20240115-060000 --check-restorable

# Verbose output
kscorectl backup verify backup-20240115-060000 --verbose
```

### backup restore

Restore Keystone Core data from a backup.

```bash
kscorectl backup restore <backup-id> [flags]
```

**Flags**:

- `-t, --target string`: Target cluster for restore
- `-c, --components strings`: Specific components to restore
- `--dry-run`: Show what would be restored without making changes
- `-f, --force`: Skip confirmation prompts
- `--async`: Run restore asynchronously

**Examples**:

```bash
# Dry-run restore (preview)
kscorectl backup restore backup-20240115-060000 --dry-run

# Restore to test cluster
kscorectl backup restore backup-20240115-060000 --target test-cluster

# Restore specific components
kscorectl backup restore backup-20240115-060000 --components database,config

# Force restore without confirmation
kscorectl backup restore backup-20240115-060000 --force
```

### backup delete

Delete a backup from storage.

```bash
kscorectl backup delete [backup-id] [flags]
```

**Flags**:

- `-f, --force`: Skip confirmation prompts
- `--older-than string`: Delete backups older than duration (e.g., 30d, 1w)

**Examples**:

```bash
# Delete a specific backup
kscorectl backup delete backup-20240115-060000

# Delete without confirmation
kscorectl backup delete backup-20240115-060000 --force

# Delete backups older than 30 days
kscorectl backup delete --older-than 30d --force
```

### backup replication-status

Show the status of backup replication to secondary destinations.

```bash
kscorectl backup replication-status
```

**Example Output**:

```
Backup Replication Status
=========================

Enabled:       true
Status:        healthy
Sync Interval: 12h
Last Sync:     2024-01-15T06:10:00Z
Next Sync:     2024-01-15T18:00:00Z

DESTINATION      TYPE   STATUS     LAST SYNC             BACKUPS   SIZE
us-west-2        s3     ✓ synced   2024-01-15T06:10:00Z  30        15.2 GB
eu-central-1     s3     ✓ synced   2024-01-15T06:10:05Z  30        15.2 GB
local-archive    sftp   ◐ syncing  2024-01-14T18:00:00Z  28        14.5 GB
```

### backup schedule

Manage automated backup schedules.

#### backup schedule list

List all backup schedules.

```bash
kscorectl backup schedule list
```

#### backup schedule create

Create a new backup schedule.

```bash
kscorectl backup schedule create <name> [flags]
```

**Flags**:

- `--schedule string`: Cron schedule expression (default: "0 6 ** *")
- `-t, --type string`: Backup type (default: full)
- `-c, --components strings`: Components to backup
- `-d, --destination string`: Backup destination
- `--retain int`: Number of backups to retain (default: 7)

**Examples**:

```bash
# Create daily full backup schedule
kscorectl backup schedule create daily-full --schedule "0 6 * * *" --type full

# Create hourly incremental schedule
kscorectl backup schedule create hourly-incr --schedule "0 * * * *" --type incremental

# Create weekly archive schedule
kscorectl backup schedule create weekly-archive --schedule "0 2 * * 0" --destination s3://archive/weekly --retain 52
```

#### backup schedule delete

Delete a backup schedule.

```bash
kscorectl backup schedule delete <name>
```

#### backup schedule enable/disable

Enable or disable a backup schedule.

```bash
kscorectl backup schedule enable <name>
kscorectl backup schedule disable <name>
```

### backup retention

Manage backup retention policies.

#### backup retention show

Show current retention policies.

```bash
kscorectl backup retention show
```

#### backup retention set

Set retention policy parameters.

```bash
kscorectl backup retention set <policy-name> [flags]
```

**Flags**:

- `--max-backups int`: Maximum number of backups to keep
- `--max-age string`: Maximum age of backups (e.g., 30d)
- `--keep-daily int`: Daily backups to keep
- `--keep-weekly int`: Weekly backups to keep
- `--keep-monthly int`: Monthly backups to keep
- `--keep-yearly int`: Yearly backups to keep

**Examples**:

```bash
# Set default retention policy
kscorectl backup retention set default --max-backups 30 --max-age 30d

# Set archive retention policy
kscorectl backup retention set archive --keep-weekly 52 --keep-monthly 12 --keep-yearly 5
```

#### backup retention apply

Apply retention policies to clean up old backups.

```bash
kscorectl backup retention apply [flags]
```

**Flags**:

- `--dry-run`: Show what would be deleted without making changes

**Examples**:

```bash
# Preview cleanup
kscorectl backup retention apply --dry-run

# Apply retention policies
kscorectl backup retention apply
```

## kscore-events (Event Management)

List, query, emit, replay, and watch events in the Keystone Core event system. Manage event retention and the dead letter queue.

### Global Flags

These flags apply to all kscore-events commands:

- `--server string`: Control plane server address (default: localhost:9090)
- `-o, --output string`: Output format: table, text, json, yaml (default: table)
- `-v, --verbose`: Enable verbose output

### events list

List events with optional filtering.

```bash
kscorectl events list [flags]
```

**Flags**:

- `--type string`: Filter by event type (e.g., agent.connect, state.change)
- `--source string`: Filter by event source
- `--severity string`: Filter by minimum severity (debug, info, warning, error, critical)
- `--since string`: Show events since time (e.g., 1h, 24h, 7d)
- `--before string`: Show events before time (e.g., 1h, 24h, 7d)
- `--until string`: Alias for `--before`
- `--correlation-id string`: Filter by correlation ID
- `--tag stringArray`: Filter by tag (key:value format, can be repeated)
- `--limit int`: Maximum events to show (default: 50)

**Examples**:

```bash
# List recent events
kscorectl events list

# List agent events from last hour
kscorectl events list --type "agent.*" --since 1h

# List error events
kscorectl events list --severity error

# List events with correlation ID
kscorectl events list --correlation-id abc123

# Output as JSON
kscorectl events list --since 24h -o json
```

### events query

Query events using CEL expressions.

```bash
kscorectl events query <expression> [flags]
```

**Flags**:

- `--since string`: Query events since time
- `--until string`: Query events until time
- `-n, --limit int`: Maximum events to return (default: 100)

**Examples**:

```bash
# Query state drift events
kscorectl events query 'type == "state.drift" && severity == "warning"'

# Query events from specific agent
kscorectl events query 'source.agent_id == "agent-001"'

# Query failed job events
kscorectl events query 'type == "job.failed" && data.exit_code != 0'
```

### events emit

Emit a custom event.

```bash
kscorectl events emit [flags]
```

**Flags**:

- `--type string`: Event type (required)
- `--data string`: Event data as JSON
- `--severity string`: Event severity: info, warning, error, critical (default: info)
- `--source string`: Event source identifier
- `--correlation-id string`: Correlation ID for event tracing

**Examples**:

```bash
# Emit a simple event
kscorectl events emit --type custom.deployment.started --data '{"version":"1.2.3"}'

# Emit warning event
kscorectl events emit --type custom.threshold.exceeded --severity warning --data '{"metric":"cpu","value":95}'

# Emit with correlation ID
kscorectl events emit --type custom.task.started --correlation-id task-123 --data '{"task":"backup"}'
```

### events replay

Replay historical events for testing or recovery.

```bash
kscorectl events replay [flags]
```

**Flags**:

- `--type string`: Filter events to replay by type
- `--since string`: Replay events since time
- `--until string`: Replay events until time
- `--target string`: Target for replayed events (reactor name or webhook URL)
- `--dry-run`: Show events that would be replayed without replaying
- `--rate float`: Replay rate multiplier (default: 1.0)

**Examples**:

```bash
# Dry-run replay of drift events
kscorectl events replay --type "state.drift" --since 24h --dry-run

# Replay to specific reactor
kscorectl events replay --type "agent.failed" --target reactor:auto-remediate --since 1h

# Replay at 2x speed
kscorectl events replay --since 1h --rate 2.0
```

### events watch

Watch events in real-time.

```bash
kscorectl events watch [flags]
```

**Flags**:

- `--type string`: Filter by event type pattern (supports wildcards)
- `--severity string`: Minimum severity to show
- `--source string`: Filter by source
- `--filter string`: Filter expression
- `--tag stringArray`: Filter by tag (key:value format, repeatable)
- `--format string`: Output format: text, json, jsonl (default: text)

**Examples**:

```bash
# Watch all events
kscorectl events watch

# Watch agent events
kscorectl events watch --type "agent.*"

# Watch errors and above
kscorectl events watch --severity error

# Watch as JSON lines (for piping)
kscorectl events watch --format jsonl | jq .
```

### events retention

Manage event retention settings.

#### events retention list

List current retention policies.

```bash
kscorectl events retention list
```

#### events retention set

Update retention settings.

```bash
kscorectl events retention set [flags]
```

**Flags**:

- `--max-age string`: Maximum event age (e.g., 7d, 30d)
- `--max-count int`: Maximum number of events to retain
- `--min-severity string`: Minimum severity to keep (debug, info, warning, error, critical)
- `--type string`: Apply settings to specific event type

**Examples**:

```bash
# Set global retention to 30 days
kscorectl events retention set --max-age 30d

# Set retention for error events to 90 days
kscorectl events retention set --type "*.error" --max-age 90d

# Set maximum event count
kscorectl events retention set --max-count 1000000

# Only keep info and above for debug events
kscorectl events retention set --type "debug.*" --min-severity info
```

### events dlq

Manage the dead letter queue for failed event processing.

#### events dlq list

List events in the dead letter queue.

```bash
kscorectl events dlq list [flags]
```

**Flags**:

- `--reason string`: Filter by failure reason
- `-n, --limit int`: Maximum events to show (default: 50)

#### events dlq show

Show details of a DLQ event.

```bash
kscorectl events dlq show <event-id>
```

#### events dlq retry

Retry processing DLQ events.

```bash
kscorectl events dlq retry [event-id] [flags]
```

**Flags**:

- `--all`: Retry all DLQ events
- `--type string`: Retry events of specific type

**Examples**:

```bash
# Retry specific event
kscorectl events dlq retry evt-12345

# Retry all DLQ events
kscorectl events dlq retry --all

# Retry events of specific type
kscorectl events dlq retry --type "webhook.delivery"
```

#### events dlq purge

Remove events from the dead letter queue.

```bash
kscorectl events dlq purge [flags]
```

**Flags**:

- `--older-than string`: Purge events older than duration
- `--reason string`: Purge events with specific failure reason
- `-f, --force`: Skip confirmation

**Examples**:

```bash
# Purge events older than 7 days
kscorectl events dlq purge --older-than 7d --force

# Purge events with specific reason
kscorectl events dlq purge --reason "timeout" --force
```

## kscore-schedule (Schedule and Maintenance Window Management)

Manage scheduled operations and maintenance windows for automated task execution and operational coordination.

### Global Flags

These flags apply to all kscore-schedule commands:

- `--server string`: Control plane server address (default: localhost:9090)
- `-o, --output string`: Output format: table, json, yaml (default: table)
- `-v, --verbose`: Enable verbose output

### schedule list

List scheduled operations.

```bash
kscorectl schedule list [flags]
```

**Flags**:

- `--type string`: Filter by schedule type (command, state, blueprint, reactor)
- `--status string`: Filter by status (active, paused, disabled)
- `--label strings`: Filter by label (key:value format)
- `--limit int`: Maximum schedules to show (default: 50)

**Examples**:

```bash
# List all schedules
kscorectl schedule list

# List only command schedules
kscorectl schedule list --type command

# List active schedules
kscorectl schedule list --status active

# Filter by labels
kscorectl schedule list --label env:prod
```

### schedule show

Show schedule details.

```bash
kscorectl schedule show <schedule-id>
```

### schedule create

Create a new scheduled operation.

```bash
kscorectl schedule create [flags]
```

**Flags**:

- `--name string`: Schedule name (required)
- `--description string`: Schedule description
- `--type string`: Schedule type: command, state, blueprint, reactor (default: command)
- `--cron string`: Cron expression (e.g., "0 2 ** *")
- `--interval string`: Interval duration (e.g., "1h", "30m")
- `--timezone string`: Timezone for schedule evaluation (default: UTC)
- `--target-all`: Target all agents
- `--target-agent strings`: Target specific agents
- `--target-glob string`: Target agents matching glob pattern
- `--target-tags strings`: Target agents with tags (key:value)
- `--target-roles strings`: Target agents with roles
- `--command string`: Command to execute (for command type)
- `--state-path string`: State file path (for state type)
- `--blueprint string`: Blueprint name (for blueprint type)
- `--priority int`: Schedule priority 0-10 (default: 5)
- `--timeout string`: Execution timeout (default: 1h)
- `--require-approval`: Require approval before execution
- `--label strings`: Labels (key:value format)
- `--maintenance-window string`: Link to maintenance window

**Examples**:

```bash
# Create a cron-based command schedule
kscorectl schedule create --name daily-backup --type command \
  --cron "0 2 * * *" --target-all --command "backup.sh"

# Create an interval-based state schedule
kscorectl schedule create --name hourly-sync --type state \
  --interval 1h --state-path /states/sync.yaml --target-tags role:web

# Create a blueprint schedule with approval
kscorectl schedule create --name weekly-patching --type blueprint \
  --cron "0 3 * * 0" --blueprint security-patches \
  --require-approval --target-tags env:prod
```

### schedule trigger

Trigger a schedule immediately.

```bash
kscorectl schedule trigger <schedule-id>
```

### schedule pause

Pause a schedule.

```bash
kscorectl schedule pause <schedule-id>
```

### schedule resume

Resume a paused schedule.

```bash
kscorectl schedule resume <schedule-id>
```

### schedule enable

Enable a disabled schedule.

```bash
kscorectl schedule enable <schedule-id>
```

### schedule disable

Disable a schedule.

```bash
kscorectl schedule disable <schedule-id>
```

### schedule delete

Delete a schedule.

```bash
kscorectl schedule delete <schedule-id> [flags]
```

**Flags**:

- `-f, --force`: Force deletion without confirmation

### schedule history

Show execution history for a schedule.

```bash
kscorectl schedule history <schedule-id> [flags]
```

**Flags**:

- `--limit int`: Number of executions to show (default: 20)
- `--status string`: Filter by execution status

**Examples**:

```bash
# Show last 20 executions
kscorectl schedule history sched-001

# Show only failed executions
kscorectl schedule history sched-001 --status failed
```

### schedule maintenance list

List maintenance windows.

```bash
kscorectl schedule maintenance list [flags]
```

**Flags**:

- `--status string`: Filter by status (scheduled, active, completed, cancelled)
- `--type string`: Filter by type (planned, emergency, recurring)
- `--label strings`: Filter by label (key:value format)
- `--limit int`: Maximum windows to show (default: 50)

**Examples**:

```bash
# List all maintenance windows
kscorectl schedule maintenance list

# List only active windows
kscorectl schedule maintenance list --status active

# List emergency windows
kscorectl schedule maintenance list --type emergency
```

### schedule maintenance show

Show maintenance window details.

```bash
kscorectl schedule maintenance show <window-id>
```

### schedule maintenance create

Create a new maintenance window.

```bash
kscorectl schedule maintenance create [flags]
```

**Flags**:

- `--name string`: Window name (required)
- `--description string`: Window description
- `--type string`: Window type: planned, emergency, recurring (default: planned)
- `--start string`: Start time (RFC3339 format or "now")
- `--end string`: End time (RFC3339 format)
- `--timezone string`: Timezone (default: UTC)
- `--scope-all`: Affect all agents
- `--scope-agents strings`: Specific agents
- `--scope-glob string`: Agent glob pattern
- `--scope-tags strings`: Agent tags (key:value)
- `--scope-roles strings`: Agent roles
- `--suppress-alerts`: Suppress alerts during window (default: true)
- `--suppress-drift`: Suppress drift detection
- `--allow-operations`: Allow manual operations (default: true)
- `--require-approval`: Require approval to start
- `--notify-before string`: Notification lead time (default: 15m)
- `--notify-channel strings`: Notification channels
- `--label strings`: Labels (key:value format)

**Examples**:

```bash
# Create a planned maintenance window
kscorectl schedule maintenance create --name "weekly-patching" \
  --start "2024-01-15T02:00:00Z" --end "2024-01-15T06:00:00Z" \
  --scope-tags env:prod --suppress-alerts

# Create an emergency maintenance window
kscorectl schedule maintenance create --name "urgent-fix" --type emergency \
  --start now --end "2024-01-15T04:00:00Z" --scope-all

# Create with approval requirement
kscorectl schedule maintenance create --name "db-migration" \
  --start "2024-01-20T00:00:00Z" --end "2024-01-20T04:00:00Z" \
  --require-approval --scope-agents db-01,db-02
```

### schedule maintenance start

Start a scheduled maintenance window.

```bash
kscorectl schedule maintenance start <window-id>
```

### schedule maintenance end

End an active maintenance window.

```bash
kscorectl schedule maintenance end <window-id>
```

### schedule maintenance cancel

Cancel a maintenance window.

```bash
kscorectl schedule maintenance cancel <window-id> [flags]
```

**Flags**:

- `--reason string`: Cancellation reason

### schedule maintenance extend

Extend a maintenance window.

```bash
kscorectl schedule maintenance extend <window-id> [flags]
```

**Flags**:

- `--end string`: New end time (RFC3339 format)
- `--duration string`: Extend by duration (e.g., 1h, 30m)

**Examples**:

```bash
# Extend to new end time
kscorectl schedule maintenance extend maint-001 --end "2024-01-15T08:00:00Z"

# Extend by 2 hours
kscorectl schedule maintenance extend maint-001 --duration 2h
```

### schedule maintenance active

List currently active maintenance windows.

```bash
kscorectl schedule maintenance active
```

### schedule maintenance upcoming

List upcoming maintenance windows.

```bash
kscorectl schedule maintenance upcoming [flags]
```

**Flags**:

- `--within string`: Show windows starting within duration (default: 24h)

### schedule maintenance conflicts

Check for conflicts with other windows.

```bash
kscorectl schedule maintenance conflicts <window-id>
```

### schedule maintenance delete

Delete a maintenance window.

```bash
kscorectl schedule maintenance delete <window-id> [flags]
```

**Flags**:

- `-f, --force`: Force deletion without confirmation

## kscore-upgrade (Upgrade Management)

Plan, execute, and manage Keystone Core upgrades including rolling upgrades, canary deployments, and rollbacks.

### Global Flags

These flags apply to all kscore-upgrade commands:

- `--server string`: Control plane server address (default: localhost:9090)
- `-o, --output string`: Output format: table, json, yaml (default: table)
- `-v, --verbose`: Enable verbose output
- `--audit-level string`: Audit logging level: all, errors, none (default: all)
- `--audit-output string`: Audit output backend: auto, syslog, journald, stderr, none

### upgrade check

Check for available upgrades.

```bash
kscorectl upgrade check [flags]
```

**Flags**:

- `-t, --target string`: Check compatibility with specific target version
- `--from string`: Override current version (e.g. to check a hypothetical upgrade path)
- `--include-prerelease`: Include prerelease versions
- `--channel string`: Release channel: stable, beta, nightly (default: stable)

**Examples**:

```bash
# Check for available upgrades
kscorectl upgrade check

# Check upgrade from a specific version
kscorectl upgrade check --from 1.4.0 --target 1.6.0

# Include beta versions
kscorectl upgrade check --include-prerelease

# Check specific channel
kscorectl upgrade check --channel beta
```

**Example Output**:

```
Upgrade Check
=============

Current Version: 1.5.2
Latest Version:  1.6.0
Upgrade Available: yes

Release Notes Summary:
  - New file distribution backend: Azure Blob Storage
  - Improved agent reconnection handling
  - CEL policy engine optimizations

Compatibility: ✓ Compatible
Breaking Changes: None

Run 'kscorectl upgrade plan --target 1.6.0' to create an upgrade plan.
```

### upgrade plan

Create an upgrade plan.

```bash
kscorectl upgrade plan [flags]
```

**Flags**:

- `--target string`: Target version (required)
- `--strategy string`: Upgrade strategy: rolling, canary, blue-green (default: rolling)
- `--batch-size int`: Number of agents per batch (default: 10)
- `--batch-delay string`: Delay between batches (default: 30s)
- `--health-check-timeout string`: Health check timeout (default: 5m)
- `--rollback-on-failure`: Automatically rollback on failure (default: true)
- `--save string`: Save plan to file

**Examples**:

```bash
# Create rolling upgrade plan
kscorectl upgrade plan --target 1.6.0

# Create canary upgrade plan
kscorectl upgrade plan --target 1.6.0 --strategy canary --batch-size 5

# Save plan to file
kscorectl upgrade plan --target 1.6.0 --save upgrade-plan.yaml
```

### upgrade execute

Execute an upgrade plan.

```bash
kscorectl upgrade execute [flags]
```

**Flags**:

- `--target string`: Target version
- `--plan string`: Path to saved upgrade plan
- `--backup-before`: Create a backup before upgrading (default: true)
- `--auto-rollback`: Automatically rollback on failure (default: true)
- `--skip-backup`: Skip automatic backup before upgrade
- `--confirm`: Confirm execution without prompting
- `--async`: Run upgrade asynchronously
- `-f, --force`: Force upgrade even with warnings
- `--max-unavailable int`: Maximum unavailable nodes during rolling upgrade (default: 1)

**Examples**:

```bash
# Execute upgrade to target version
kscorectl upgrade execute --target 1.6.0 --confirm

# Execute from saved plan
kscorectl upgrade execute --plan upgrade-plan.yaml --confirm

# Async execution
kscorectl upgrade execute --target 1.6.0 --async
```

### upgrade status

Show upgrade status.

```bash
kscorectl upgrade status [flags]
```

**Flags**:

- `-w, --watch`: Watch status updates in real-time
- `--verbose`: Show verbose component details

**Examples**:

```bash
# Show current upgrade status
kscorectl upgrade status

# Watch status in real-time
kscorectl upgrade status --watch

# Show verbose status with component details
kscorectl upgrade status --verbose
```

### upgrade cancel

Cancel an in-progress upgrade.

```bash
kscorectl upgrade cancel [upgrade-id] [flags]
```

**Flags**:

- `--rollback`: Rollback completed upgrades
- `-f, --force`: Force cancellation

### upgrade canary

Manage canary deployments.

#### upgrade canary status

Show canary deployment status.

```bash
kscorectl upgrade canary status
```

#### upgrade canary promote

Promote canary to full rollout.

```bash
kscorectl upgrade canary promote [flags]
```

**Flags**:

- `--confirm`: Confirm promotion

#### upgrade canary rollback

Rollback canary deployment.

```bash
kscorectl upgrade canary rollback [flags]
```

**Flags**:

- `--confirm`: Confirm rollback

### upgrade agents

Manage agent upgrades across the fleet.

```bash
kscorectl upgrade agents [flags]
```

**Flags**:

- `-t, --target string`: Target version for agents
- `--batch-size int`: Number of agents to upgrade in parallel (default: 5)
- `--filter string`: Filter expression for agent selection
- `--report`: Show agent version report
- `--status`: Show agent upgrade status
- `--retry string`: Retry upgrade for a specific agent
- `--skip string`: Skip upgrade for a specific agent

**Examples**:

```bash
# Upgrade all agents to target version
kscorectl upgrade agents --target 1.6.0

# Upgrade with batch size and filter
kscorectl upgrade agents --target 1.6.0 --batch-size 10 --filter "environment:production"

# Show agent version report
kscorectl upgrade agents --report

# Show agent upgrade status
kscorectl upgrade agents --status

# Retry a failed agent upgrade
kscorectl upgrade agents --retry agent-005

# Skip a failed agent
kscorectl upgrade agents --skip agent-005
```

### upgrade rollback

Rollback to previous version.

```bash
kscorectl upgrade rollback [flags]
```

**Flags**:

- `--target string`: Target version to rollback to
- `--confirm`: Confirm rollback
- `--force`: Force rollback even if unhealthy

**Examples**:

```bash
# Rollback to previous version
kscorectl upgrade rollback --confirm

# Rollback to specific version
kscorectl upgrade rollback --target 1.5.2 --confirm
```

### upgrade history

Show upgrade history.

```bash
kscorectl upgrade history [flags]
```

**Flags**:

- `-n, --limit int`: Number of upgrades to show (default: 10)
- `--status string`: Filter by status

### upgrade path

Show the recommended upgrade path between versions.

```bash
kscorectl upgrade path [flags]
```

**Flags**:

- `-t, --target string`: Target version (required)
- `--from string`: Starting version (default: current installed version)

**Examples**:

```bash
# Show path from current version to target
kscorectl upgrade path --target 2.0.0

# Show path between two specific versions
kscorectl upgrade path --from 1.3.0 --target 2.0.0
```

### upgrade resume

Resume an interrupted or paused upgrade.

```bash
kscorectl upgrade resume [flags]
```

**Flags**:

- `--upgrade-id string`: Specific upgrade to resume (default: most recent)

**Examples**:

```bash
# Resume the most recent interrupted upgrade
kscorectl upgrade resume

# Resume a specific upgrade by ID
kscorectl upgrade resume --upgrade-id upgrade-20240115-100000
```

### upgrade logs

Show upgrade logs.

```bash
kscorectl upgrade logs [upgrade-id] [flags]
```

**Flags**:

- `--follow`: Follow log output
- `--tail int`: Number of lines to show (default: 100)

### upgrade package

Manage air-gapped upgrade packages. These commands create, verify, inspect, apply, and rollback
upgrade packages for environments without internet access.

#### upgrade package create

Create an upgrade package from a build directory.

```bash
kscorectl upgrade package create [flags]
```

**Flags:**

- `--from string`: Minimum source version (required)
- `--to string`: Target version (required)
- `--build-dir string`: Directory containing new binaries (required)
- `--platform string`: Target platform os/arch (default: linux/amd64)
- `-o, --output string`: Output archive path
- `--signing-key string`: Path to PEM private key for signing
- `--modules-dir string`: Directory with updated modules
- `--migrations-dir string`: Directory with migration scripts
- `--pre-scripts-dir string`: Directory with pre-upgrade scripts
- `--post-scripts-dir string`: Directory with post-upgrade scripts
- `--breaking-change strings`: Breaking change descriptions

**Examples:**

```bash
# Create an upgrade package
kscorectl upgrade package create --from 1.0.0 --to 1.1.0 \
  --build-dir ./build --output upgrade.tar.gz

# Create a signed package with modules and migrations
kscorectl upgrade package create --from 1.0.0 --to 1.1.0 \
  --build-dir ./build --signing-key key.pem \
  --modules-dir ./modules --migrations-dir ./migrations
```

#### upgrade package verify

Verify an upgrade package's signature, checksums, and manifest.

```bash
kscorectl upgrade package verify <package.tar.gz> [flags]
```

**Flags:**

- `--trusted-key string`: Path to trusted public key (PEM)

**Examples:**

```bash
# Verify package integrity
kscorectl upgrade package verify upgrade.tar.gz

# Verify with trusted key
kscorectl upgrade package verify upgrade.tar.gz --trusted-key release.pub
```

#### upgrade package inspect

Show the manifest and contents of an upgrade package.

```bash
kscorectl upgrade package inspect <package.tar.gz>
```

**Examples:**

```bash
kscorectl upgrade package inspect upgrade.tar.gz
```

**Example Output:**

```
Upgrade Package: upgrade.tar.gz
  Schema:     1.0
  From:       1.0.0
  To:         1.1.0
  Platform:   linux/amd64
  Created:    2024-01-15T10:00:00Z
  Signed:     true

Components (3):
  - kscore-server v1.1.0 (bin/kscore-server)
  - kscore-agent v1.1.0 (bin/kscore-agent)
  - kscorectl v1.1.0 (bin/kscorectl)

Migrations (2):
  1. 001_add_index.sql
  2. 002_alter_table.sql
```

#### upgrade package apply

Apply an upgrade package to the current installation.

```bash
kscorectl upgrade package apply <package.tar.gz> [flags]
```

**Flags:**

- `--install-dir string`: Directory containing installed binaries (default: /usr/local/bin)
- `--backup-dir string`: Directory for rollback backup
- `--dry-run`: Show what would be done without making changes
- `--skip-backup`: Skip creating backup before upgrade
- `--skip-scripts`: Skip pre/post upgrade scripts
- `--trusted-key string`: Path to trusted public key (PEM)

**Examples:**

```bash
# Apply upgrade with backup
kscorectl upgrade package apply upgrade.tar.gz \
  --install-dir /usr/local/bin --backup-dir /var/backup

# Dry run to see what would happen
kscorectl upgrade package apply upgrade.tar.gz --dry-run

# Apply without backup (not recommended)
kscorectl upgrade package apply upgrade.tar.gz --skip-backup
```

#### upgrade package rollback

Restore binaries from a backup created during a previous upgrade.

```bash
kscorectl upgrade package rollback [flags]
```

**Flags:**

- `--backup-dir string`: Backup directory from previous upgrade (required)
- `--install-dir string`: Directory containing installed binaries (default: /usr/local/bin)
- `--dry-run`: Show what would be done without making changes

**Examples:**

```bash
# Rollback from backup
kscorectl upgrade package rollback \
  --backup-dir /var/backup/upgrade-backup-1.0.0-to-1.1.0-1234567890 \
  --install-dir /usr/local/bin

# Dry run rollback
kscorectl upgrade package rollback \
  --backup-dir /var/backup/upgrade-backup-1.0.0-to-1.1.0-1234567890 --dry-run
```

## kscore-proxy (Proxy Agent and Device Management)

Manage proxy agents that handle devices unable to run native Keystone Core agents, including network devices, legacy systems, and appliances via SSH, SNMP, REST, and WinRM protocols.

### Global Flags

These flags apply to all kscore-proxy commands:

- `--server string`: Control plane server address (default: localhost:9090)
- `-o, --output string`: Output format: table, json, yaml (default: table)
- `-v, --verbose`: Enable verbose output
- `--audit-level string`: Audit logging level: all, errors, none (default: all)
- `--audit-output string`: Audit output backend: auto, syslog, journald, stderr, none

### proxy status

Show overall proxy agent status.

```bash
kscorectl proxy status
```

**Example Output**:

```
Proxy Agent Status
==================

Proxy Agents:
  Total:     3
  Healthy:   2
  Degraded:  1
  Unhealthy: 0

Managed Devices:
  Total:     45
  Healthy:   42
  Degraded:  2
  Unhealthy: 1

Credentials:
  Total:    12
  Expiring: 2

Discovery:
  Pending:  5
  Approved: 40
  Rejected: 3
```

### proxy device

Manage proxied devices.

#### proxy device list

List managed devices.

```bash
kscorectl proxy device list [flags]
```

**Flags**:

- `--proxy string`: Filter by proxy agent
- `--vendor string`: Filter by vendor
- `--type string`: Filter by device type
- `--status string`: Filter by status (healthy, degraded, unhealthy)

**Examples**:

```bash
# List all devices
kscorectl proxy device list

# List network devices
kscorectl proxy device list --vendor cisco

# List unhealthy devices
kscorectl proxy device list --status unhealthy
```

#### proxy device show

Show device details.

```bash
kscorectl proxy device show <device-id>
```

#### proxy device add

Add a new managed device.

```bash
kscorectl proxy device add [flags]
```

**Flags**:

- `--name string`: Device name (required)
- `--address string`: Device address (required)
- `--protocol string`: Protocol: ssh, snmp, rest, winrm (default: ssh)
- `--vendor string`: Device vendor
- `--type string`: Device type (router, switch, firewall, server)
- `--profile string`: Device profile to use
- `--credential string`: Credential set to use
- `--labels strings`: Labels (key=value format)

**Examples**:

```bash
# Add a Cisco router
kscorectl proxy device add --name core-router-01 \
  --address 192.168.1.1 --protocol ssh \
  --vendor cisco --type router \
  --credential cisco-ssh --profile cisco_ios

# Add a Windows server via WinRM
kscorectl proxy device add --name legacy-server-01 \
  --address 10.0.0.50 --protocol winrm \
  --credential win-admin --type server
```

#### proxy device import

Import devices from file.

```bash
kscorectl proxy device import [flags]
```

**Flags**:

- `--file string`: File to import (required)
- `--format string`: File format: yaml, csv (default: yaml)

#### proxy device update

Update device settings.

```bash
kscorectl proxy device update <device-id> [flags]
```

**Flags**:

- `--labels strings`: Labels to set (key=value)

#### proxy device remove

Remove a managed device.

```bash
kscorectl proxy device remove <device-id> [flags]
```

**Flags**:

- `-f, --force`: Skip confirmation

#### proxy device test

Test connectivity to a device.

```bash
kscorectl proxy device test <device-id> [flags]
```

**Flags**:

- `--protocol string`: Override protocol for test
- `--credential string`: Override credential for test
- `--debug`: Enable debug output

#### proxy device health

Check device health.

```bash
kscorectl proxy device health [device-id] [flags]
```

**Flags**:

- `--all`: Check health of all devices

#### proxy device ping

Ping a device.

```bash
kscorectl proxy device ping <device-id>
```

#### proxy device status

Show device status.

```bash
kscorectl proxy device status <device-id>
```

#### proxy device config show

Show device configuration.

```bash
kscorectl proxy device config show <device-id>
```

#### proxy device connect

Connect to device interactively.

```bash
kscorectl proxy device connect <device-id> [flags]
```

**Flags**:

- `--credential string`: Credential to use

### proxy credential

Manage credentials for proxied devices.

#### proxy credential list

List credential sets.

```bash
kscorectl proxy credential list [flags]
```

**Flags**:

- `--protocol string`: Filter by protocol
- `--backend string`: Filter by backend (local, vault, k8s-secret)

#### proxy credential add

Add a new credential set.

```bash
kscorectl proxy credential add [flags]
```

**Flags**:

- `--name string`: Credential name (required)
- `--type string`: Credential type: ssh-password, ssh-key, snmp-v2c, snmp-v3, rest-token, rest-basic, winrm (required)
- `--protocol string`: Associated protocol
- `--username string`: Username
- `--password string`: Password (prompted if not provided)
- `--key-file string`: SSH private key file
- `--community string`: SNMP community string
- `--token string`: API token
- `--backend string`: Storage backend: local, vault, k8s-secret (default: local)
- `--device-types strings`: Device types this credential applies to

**Examples**:

```bash
# Add SSH key credential
kscorectl proxy credential add --name cisco-ssh \
  --type ssh-key --username admin --key-file ~/.ssh/cisco_key

# Add SNMP v3 credential
kscorectl proxy credential add --name snmp-v3 \
  --type snmp-v3 --username snmpuser

# Add REST API token
kscorectl proxy credential add --name api-token \
  --type rest-token --token "secret-token"

# Add credential stored in Vault
kscorectl proxy credential add --name vault-ssh \
  --type ssh-password --backend vault \
  --username admin
```

#### proxy credential show

Show credential details.

```bash
kscorectl proxy credential show <name>
```

#### proxy credential test

Test a credential.

```bash
kscorectl proxy credential test <name> [flags]
```

**Flags**:

- `--device string`: Device to test credential against

#### proxy credential rotate

Rotate a credential.

```bash
kscorectl proxy credential rotate <name> [flags]
```

**Flags**:

- `--password-prompt`: Prompt for new password

#### proxy credential delete

Delete a credential.

```bash
kscorectl proxy credential delete <name> [flags]
```

**Flags**:

- `-f, --force`: Skip confirmation

#### proxy credential verify

Verify credential integrity.

```bash
kscorectl proxy credential verify <name>
```

#### proxy credential backend-status

Show credential backend status.

```bash
kscorectl proxy credential backend-status
```

### proxy discover

Discover devices on the network.

#### proxy discover scan

Scan for devices.

```bash
kscorectl proxy discover scan [flags]
```

**Flags**:

- `--network string`: Network to scan (CIDR notation)
- `--subnet string`: Subnet to scan (CIDR notation, alias for --network)
- `--networks strings`: Multiple networks to scan
- `--protocols strings`: Protocols to probe (ssh, snmp, rest, winrm)
- `--ports strings`: Ports to scan
- `--timeout string`: Scan timeout per host (default: 5s)
- `--workers int`: Number of parallel workers (default: 20)
- `--debug`: Enable debug output

**Examples**:

```bash
# Scan a network for SSH and SNMP devices
kscorectl proxy discover scan --network 192.168.1.0/24 --protocols ssh,snmp

# Scan with custom ports
kscorectl proxy discover scan --subnet 10.0.0.0/24 --ports 22,161,443

# Scan multiple networks
kscorectl proxy discover scan --networks 192.168.1.0/24,10.0.0.0/24
```

#### proxy discover list

List discovered devices.

```bash
kscorectl proxy discover list [flags]
```

**Flags**:

- `--status string`: Filter by status (pending, approved, rejected, ignored)

#### proxy discover approve

Approve a discovered device for management.

```bash
kscorectl proxy discover approve <discovery-id> [flags]
```

**Flags**:

- `--name string`: Device name
- `--profile string`: Device profile
- `--credential string`: Credential set

#### proxy discover reject

Reject a discovered device.

```bash
kscorectl proxy discover reject <discovery-id>
```

#### proxy discover status

Show discovery status and statistics.

```bash
kscorectl proxy discover status
```

#### proxy discover approve-all

Approve all pending devices matching criteria.

```bash
kscorectl proxy discover approve-all [flags]
```

**Flags**:

- `--vendor string`: Filter by vendor
- `--credential string`: Credential to assign to approved devices

#### proxy discover ignore

Ignore an address in future scans.

```bash
kscorectl proxy discover ignore <address>
```

#### proxy discover auto-approve

Enable auto-approval for device profiles.

```bash
kscorectl proxy discover auto-approve [flags]
```

**Flags**:

- `--profile strings`: Profiles to auto-approve

#### proxy discover logs

Show discovery logs.

```bash
kscorectl proxy discover logs [flags]
```

**Flags**:

- `--tail int`: Number of log lines to show (default: 50)

#### proxy discover config

Discovery configuration management.

```bash
kscorectl proxy discover config show
```

### proxy drift

Detect and report configuration drift on proxied devices.

#### proxy drift check

Check for drift on devices.

```bash
kscorectl proxy drift check [device-id] [flags]
```

**Flags**:

- `--all`: Check all devices

**Examples**:

```bash
# Check specific device for drift
kscorectl proxy drift check router-01

# Check all devices for drift
kscorectl proxy drift check --all
```

#### proxy drift report

Show drift report for a device.

```bash
kscorectl proxy drift report <device-id>
```

#### proxy drift remediate

Remediate configuration drift on a device.

```bash
kscorectl proxy drift remediate <device-id> [flags]
```

**Flags**:

- `--dry-run`: Show what would be changed without applying

### proxy state

Apply state to proxied devices.

#### proxy state apply

Apply configuration state to devices.

```bash
kscorectl proxy state apply <state-file> [flags]
```

**Flags**:

- `--device string`: Apply to specific device
- `--target string`: Target expression
- `--dry-run`: Preview changes without applying
- `-f, --force`: Apply without confirmation

**Examples**:

```bash
# Apply state to device
kscorectl proxy state apply network-config.yaml --device router-01

# Dry-run
kscorectl proxy state apply network-config.yaml --device router-01 --dry-run

# Apply to multiple devices
kscorectl proxy state apply security-baseline.yaml --target "vendor:cisco"
```

#### proxy state check

Check state compliance on devices.

```bash
kscorectl proxy state check <state-file> [flags]
```

**Flags**:

- `--device string`: Check specific device
- `--target string`: Target expression

#### proxy state logs

Show state application logs.

```bash
kscorectl proxy state logs [flags]
```

**Flags**:

- `--device string`: Filter logs by device
- `--run-id string`: Filter by run ID

## Command Migration Guide

This section documents the CLI command restructuring in version 0.4.0 and provides migration guidance for scripts and automation.

### Overview

In version 0.4.0, the monolithic `kscorectl` command was split into focused plugins for better maintainability and clearer responsibility boundaries. This guide helps you update existing scripts and workflows.

### Command Mapping Table

#### Agent Commands

| Old Command (< 0.4.0) | New Command (≥ 0.4.0) | Notes |
|----------------------|----------------------|-------|
| `kscorectl agent list` | `kscorectl agents list` | Renamed to plural |
| `kscorectl agent show <id>` | `kscorectl agents show <id>` | Renamed to plural |
| `kscorectl agent exec <cmd>` | `kscorectl exec run <cmd>` | Moved to exec plugin |
| `kscorectl agent shell <id>` | `kscorectl exec shell <id>` | Moved to exec plugin |
| `kscorectl agent run-script` | `kscorectl exec script` | Moved to exec plugin |
| `kscorectl agent upload` | `kscorectl files push` | Moved to files plugin |
| `kscorectl agent download` | `kscorectl files pull` | Moved to files plugin |

#### State Commands

| Old Command (< 0.4.0) | New Command (≥ 0.4.0) | Notes |
|----------------------|----------------------|-------|
| `kscorectl apply <file>` | `kscorectl state apply <file>` | Requires `state` prefix |
| `kscorectl check <file>` | `kscorectl state check <file>` | Requires `state` prefix |
| `kscorectl show-state` | `kscorectl state show` | Renamed |
| `kscorectl highstate` | `kscorectl state apply <statefile>` | Deprecated term, use state files |
| `kscorectl state.apply` | `kscorectl state apply` | Salt-style removed |

#### Execution Commands

| Old Command (< 0.4.0) | New Command (≥ 0.4.0) | Notes |
|----------------------|----------------------|-------|
| `kscorectl run <cmd>` | `kscorectl exec run <cmd>` | Moved to exec plugin |
| `kscorectl cmd.run <cmd>` | `kscorectl exec run <cmd>` | Salt-style removed |
| `kscorectl shell` | `kscorectl exec shell` | Moved to exec plugin |
| `kscorectl script <file>` | `kscorectl exec script <file>` | Moved to exec plugin |

#### Cluster Commands

| Old Command (< 0.4.0) | New Command (≥ 0.4.0) | Notes |
|----------------------|----------------------|-------|
| `kscorectl cluster-status` | `kscorectl cluster status` | Subcommand structure |
| `kscorectl cluster-health` | `kscorectl cluster health` | Subcommand structure |
| `kscorectl cluster-members` | `kscorectl cluster members` | Subcommand structure |
| `kscorectl join` | `kscorectl cluster join` | Moved to cluster plugin |
| `kscorectl leave` | `kscorectl cluster leave` | Moved to cluster plugin |

#### Policy Commands

| Old Command (< 0.4.0) | New Command (≥ 0.4.0) | Notes |
|----------------------|----------------------|-------|
| `kscorectl policy-list` | `kscorectl policy list` | Subcommand structure |
| `kscorectl policy-eval` | `kscorectl policy check` | Renamed |
| `kscorectl compliance` | `kscorectl policy report` | Moved to policy plugin |

#### Module Commands

| Old Command (< 0.4.0) | New Command (≥ 0.4.0) | Notes |
|----------------------|----------------------|-------|
| `kscorectl module-list` | `kscorectl module list` | Subcommand structure |
| `kscorectl module-install` | `kscorectl module install` | Subcommand structure |
| `kscorectl module-publish` | `kscorectl module publish` | Subcommand structure |

#### GitOps Commands

| Old Command (< 0.4.0) | New Command (≥ 0.4.0) | Notes |
|----------------------|----------------------|-------|
| `kscorectl gitops-status` | `kscorectl gitops status` | Subcommand structure |
| `kscorectl gitops-sync` | `kscorectl gitops repo sync` | Subcommand structure |
| `kscorectl deploy` | `kscorectl gitops deploy` | Moved to gitops plugin |

#### Identity Commands

| Old Command (< 0.4.0) | New Command (≥ 0.4.0) | Notes |
|----------------------|----------------------|-------|
| `kscorectl identity-list` | `kscorectl identity list` | Subcommand structure |
| `kscorectl svid-show` | `kscorectl identity svid show` | Nested subcommand |
| `kscorectl ca-rotate` | `kscorectl identity ca rotate` | Nested subcommand |

### Deprecated Commands

The following commands are deprecated and will be removed in version 0.6.0:

| Deprecated Command | Replacement | Removal Version |
|-------------------|-------------|-----------------|
| `kscorectl cmd.run` | `kscorectl exec run` | 0.6.0 |
| `kscorectl state.apply` | `kscorectl state apply` | 0.6.0 |
| `kscorectl highstate` | `kscorectl state apply <statefile>` | 0.6.0 |
| `kscorectl pillar.get` | Use configuration files or environment variables | 0.6.0 |
| `kscorectl grains.items` | `kscorectl agents show <id>` (for agent metadata) | 0.6.0 |

### Migration Scripts

#### Automated Script Migration

Use the migration tool to update scripts:

```bash
# Scan scripts for deprecated commands
kscore-migrate scan-scripts \
  --path ./scripts \
  --output migration-report.json

# Preview changes
kscore-migrate update-scripts \
  --path ./scripts \
  --dry-run

# Apply changes
kscore-migrate update-scripts \
  --path ./scripts \
  --backup .bak
```

#### Manual Migration Examples

**Before (< 0.4.0)**:

```bash
#!/bin/bash
# Old script using deprecated commands

# Execute command on agents
kscorectl run "hostname" --target "role=web"

# Apply state
kscorectl apply /etc/keystone-core/states/nginx.yaml

# Check cluster health
kscorectl cluster-health

# Show compliance
kscorectl compliance --report
```

**After (≥ 0.4.0)**:

```bash
#!/bin/bash
# Updated script using new commands

# Execute command on agents
kscorectl exec run "hostname" --target "role=web"

# Apply state
kscorectl state apply /etc/keystone-core/states/nginx.yaml

# Check cluster health
kscorectl cluster health

# Show compliance
kscorectl policy report
```

### Flag Changes

#### Renamed Flags

| Old Flag | New Flag | Commands |
|----------|----------|----------|
| `--agents` | `--target` | exec, state |
| `--glob` | `--target` | exec, state |
| `--nodegroup` | `--target` | exec, state |
| `--timeout-minutes` | `--timeout` | exec (now accepts duration) |
| `--output-format` | `--output` | all |
| `--concurrent` | `--parallel` | exec, state |

#### Removed Flags

| Removed Flag | Replacement | Notes |
|--------------|-------------|-------|
| `--batch` | `--parallel` | Use parallel with number |
| `--return` | Default behavior | Results always returned |
| `--async` | `--background` | Renamed for clarity |
| `--jid` | `--job-id` | Renamed for clarity |

#### Flag Value Changes

**Timeout values** now use Go duration format:

```bash
# Old (minutes as integer)
kscorectl run "command" --timeout-minutes 5

# New (duration string)
kscorectl exec run "command" --timeout 5m
```

**Target expressions** unified across commands:

```bash
# Old (multiple formats)
kscorectl run "cmd" --agents "web*"
kscorectl run "cmd" --glob "web*"
kscorectl run "cmd" --nodegroup "production"

# New (unified --target with expressions)
kscorectl exec run "cmd" --target "hostname=web*"
kscorectl exec run "cmd" --target "environment=production"
kscorectl exec run "cmd" --target "role=web AND datacenter=us-east-1"
```

### Environment Variable Changes

| Old Variable | New Variable | Notes |
|--------------|--------------|-------|
| `KSCORE_MASTER` | `KSCORE_SERVER` | Renamed |
| `KSCORE_MINION_ID` | `KSCORE_AGENT_ID` | Renamed |
| `KSCORE_OUTPUT_FORMAT` | `KSCORE_OUTPUT` | Shortened |
| `KSCORE_TIMEOUT_MINUTES` | `KSCORE_TIMEOUT` | Now duration format |

### CI/CD Pipeline Migration

#### GitHub Actions

**Before**:

```yaml
- name: Apply state
  run: |
    kscorectl apply infra/state.yaml --target "env=${{ env.ENVIRONMENT }}"
    kscorectl run "systemctl status nginx" --target "role=web"
```

**After**:

```yaml
- name: Apply state
  run: |
    kscorectl state apply infra/state.yaml --target "environment=${{ env.ENVIRONMENT }}"
    kscorectl exec run "systemctl status nginx" --target "role=web"
```

#### GitLab CI

**Before**:

```yaml
deploy:
  script:
    - kscorectl apply ${STATE_FILE}
    - kscorectl cluster-health
```

**After**:

```yaml
deploy:
  script:
    - kscorectl state apply ${STATE_FILE}
    - kscorectl cluster health
```

#### Jenkins

**Before**:

```groovy
sh 'kscorectl run "service nginx restart" --agents "web*"'
sh 'kscorectl compliance --report > compliance.html'
```

**After**:

```groovy
sh 'kscorectl exec run "service nginx restart" --target "hostname=web*"'
sh 'kscorectl policy report --output json > compliance.json'
```

### Shell Completion Updates

After upgrading, regenerate shell completions:

```bash
# Bash
kscorectl completion bash > /etc/bash_completion.d/kscorectl

# Zsh
kscorectl completion zsh > "${fpath[1]}/_kscorectl"

# Fish
kscorectl completion fish > ~/.config/fish/completions/kscorectl.fish

# PowerShell
kscorectl completion powershell > $HOME\Documents\WindowsPowerShell\Modules\kscorectl\kscorectl.psm1
```

### Alias Recommendations

For users transitioning, these aliases help maintain muscle memory:

```bash
# ~/.bashrc or ~/.zshrc

# State shortcuts
alias ks-apply='kscorectl state apply'
alias ks-check='kscorectl state check'

# Exec shortcuts
alias ks-run='kscorectl exec run'
alias ks-shell='kscorectl exec shell'

# Legacy compatibility aliases (temporary)
alias kscorectl-run='kscorectl exec run'
alias kscorectl-apply='kscorectl state apply'
```

### Compatibility Mode

**Note**: Legacy commands from versions prior to 0.4.0 are no longer supported. Use the new command structure shown above.

### Migration Checklist

- [ ] Update CI/CD pipeline configurations to use new command syntax
- [ ] Update automation playbooks and scripts
- [ ] Regenerate shell completions
- [ ] Update monitoring/alerting that parses CLI output
- [ ] Update documentation referencing CLI commands
- [ ] Test all integrations in staging environment

### Getting Help

If you encounter issues during migration:

```bash
# Get help for command structure
kscorectl exec --help
kscorectl state --help
kscorectl policy --help
```

## kscore-repo-gen (Repository Generation)

Generate distribution repositories for Keystone Core releases.

### Global Flags

- `-v, --version string`: Version string (e.g., 0.1.0)
- `-o, --output string`: Output directory (default: build/repos)
- `-d, --dist string`: Goreleaser output directory containing packages (default: dist)
- `--binaries string`: Directory containing built binaries (default: build/bin)
- `--gpg-key string`: GPG key ID for signing
- `--sign`: Sign packages and metadata

### repo-gen all

Generate all repository types.

```bash
kscore-repo-gen all [flags]
```

**Examples**:

```bash
# Generate all repositories for version 0.1.0
kscore-repo-gen all --version 0.1.0 --output build/repos

# Generate with signing
kscore-repo-gen all --version 0.1.0 --output build/repos --sign --gpg-key ABCD1234
```

### repo-gen dnf

Generate DNF/YUM repository for RHEL/CentOS/Fedora.

```bash
kscore-repo-gen dnf [flags]
```

**Examples**:

```bash
# Generate DNF repository
kscore-repo-gen dnf --version 0.1.0 --output build/repos/dnf

# Generate signed DNF repository
kscore-repo-gen dnf --version 0.1.0 --output build/repos/dnf --sign --gpg-key ABCD1234
```

### repo-gen apt

Generate APT repository for Debian/Ubuntu.

```bash
kscore-repo-gen apt [flags]
```

**Examples**:

```bash
# Generate APT repository
kscore-repo-gen apt --version 0.1.0 --output build/repos/apt
```

### repo-gen windows

Generate Windows package repository.

```bash
kscore-repo-gen windows [flags]
```

**Examples**:

```bash
# Generate Windows repository
kscore-repo-gen windows --version 0.1.0 --output build/repos/windows
```

### repo-gen blueprints

Generate Go-mod style blueprint registry.

```bash
kscore-repo-gen blueprints [flags]
```

**Flags**:

- `--blueprints-dir string`: Source blueprints directory (default: examples/blueprints/kscore)

**Examples**:

```bash
# Generate blueprint registry from default location
kscore-repo-gen blueprints --output build/repos/blueprints

# Generate from custom directory
kscore-repo-gen blueprints --blueprints-dir my-blueprints --output build/repos/blueprints
```

### repo-gen modules

Generate Go-mod style module registry.

```bash
kscore-repo-gen modules [flags]
```

**Examples**:

```bash
# Generate module registry
kscore-repo-gen modules --output build/repos/modules
```

## kscore-repo-mirror (Repository Mirroring)

Mirror Keystone Core repositories for air-gapped deployments. Downloads all packages, blueprints, modules, and documentation to a local folder that can be transferred to air-gapped environments.

### Global Flags

```
--output string           Output directory (default "mirror")
--packages-url string     Base URL for packages (default "https://packages.keystonecore.io")
--blueprints-url string   Base URL for blueprints (default "https://blueprints.keystonecore.io")
--modules-url string      Base URL for modules (default "https://modules.keystonecore.io")
--docs-url string         Base URL for docs (default "https://docs.keystonecore.io")
--concurrency int         Parallel downloads (default 4)
--timeout duration        Download timeout (default 5m)
--no-verify               Skip checksum verification
--verbose                 Verbose output
--version                 Show version
```

### Repository Selection Flags

```
--only string             Only mirror specific types (comma-separated)
--skip string             Skip specific types (comma-separated)
--dnf-dists string        DNF distributions (default "el8,el9")
--dnf-arches string       DNF architectures (default "x86_64,aarch64")
--apt-dists string        APT distributions (default "jammy,noble,bookworm,trixie")
--apt-arches string       APT architectures (default "amd64,arm64")
--win-arches string       Windows architectures (default "x64,arm64")
--mac-arches string       macOS architectures (default "x64,arm64")
```

**Repository Types** (for `--only` and `--skip`):

- `dnf`, `yum`, `rpm` - DNF/YUM packages
- `apt`, `deb` - APT packages
- `windows`, `win` - Windows packages
- `macos`, `mac`, `darwin` - macOS packages
- `blueprints`, `bp` - Blueprint registry
- `modules`, `mod` - Module registry
- `docs` - Documentation

### repo-mirror (all)

Mirror all repository types.

```bash
kscore-repo-mirror [flags]
```

**Examples**:

```bash
# Mirror everything to ./mirror
kscore-repo-mirror

# Mirror to USB drive
kscore-repo-mirror --output /mnt/usb/kscore-mirror

# Mirror with verbose output
kscore-repo-mirror --verbose
```

### Mirror Specific Repositories

```bash
# Mirror only Linux packages
kscore-repo-mirror --only dnf,apt

# Mirror only RHEL 9 x86_64
kscore-repo-mirror --only dnf --dnf-dists el9 --dnf-arches x86_64

# Skip documentation and macOS
kscore-repo-mirror --skip docs,macos

# Mirror only Windows packages
kscore-repo-mirror --only windows
```

### Output Structure

The mirrored directory structure:

```
mirror/
├── dnf/                          # DNF/YUM repositories
│   ├── el8/
│   │   ├── x86_64/
│   │   │   ├── Packages/
│   │   │   ├── repodata/
│   │   │   └── keystonecore.repo
│   │   └── aarch64/
│   └── el9/
├── apt/                          # APT repositories
│   ├── dists/
│   │   ├── jammy/
│   │   ├── noble/
│   │   ├── bookworm/
│   │   └── trixie/
│   └── pool/main/
├── windows/                      # Windows packages
│   ├── x64/
│   │   ├── manifest.json
│   │   ├── install.ps1
│   │   └── *.zip
│   └── arm64/
├── macos/                        # macOS packages
│   ├── x64/
│   │   ├── manifest.json
│   │   ├── install.sh
│   │   └── *.tar.gz
│   └── arm64/
├── blueprints/                   # Blueprint registry
│   ├── index.json
│   └── kscore/{blueprint}/@v/
├── modules/                      # Module registry
│   ├── index.json
│   └── {vendor}/{module}/@v/
├── docs/                         # Offline documentation
├── keystonecore-local.repo       # DNF local config
├── keystonecore-local.list       # APT local config
└── README.md                     # Usage instructions
```

### Using the Mirror in Air-Gapped Environments

**Option 1: Serve via HTTP**

```bash
# Start simple HTTP server
cd /path/to/mirror
python3 -m http.server 8080

# Or use nginx/Apache/Caddy
```

**Option 2: Use Local File Paths**

DNF/YUM:

```bash
sudo cp keystonecore-local.repo /etc/yum.repos.d/
# Edit the file to set the correct path
sudo dnf install kscore-server kscore-agent
```

APT:

```bash
sudo cp keystonecore-local.list /etc/apt/sources.list.d/
# Edit the file to set the correct path
sudo apt update
sudo apt install kscore-server kscore-agent
```

Windows:

```powershell
cd windows\x64
.\install.ps1 -Package all
```

macOS:

```bash
cd macos/arm64
./install.sh
```

Blueprint Registry:

```bash
kscorectl blueprint config set registry file:///path/to/mirror/blueprints
```

Module Registry:

```bash
kscorectl module config set registry file:///path/to/mirror/modules
```

## kscore-runbook (Runbook Management)

Manage runbook execution, approvals, and interventions.

### Global Flags

- `-s, --server string`: Control plane server address (default: localhost:9090)
- `-o, --format string`: Output format: table, text, json, yaml (default: table)
- `--db string`: Path to runbook database (for local testing)
- `--audit-level string`: Audit logging level (default: all)
- `--audit-output string`: Audit output backend (default: auto)

### runbook approvals

List pending approval requests.

```bash
kscorectl runbook approvals [flags]
```

**Flags**:

- `--mine`: Show only approvals assigned to me
- `--state string`: Filter by state (pending, approved, rejected, expired, cancelled)
- `--execution string`: Filter by execution ID
- `--limit int`: Maximum number of results (default: 50)

**Examples**:

```bash
# List all pending approvals
kscorectl runbook approvals

# List only your approvals
kscorectl runbook approvals --mine

# Filter by state
kscorectl runbook approvals --state approved
```

**Output**:

```
ID           TITLE                          STATE        RESPONSES  CREATED
------------------------------------------------------------------------------------------
req-abc123   Deploy v1.5.0 to production    pending      0          2024-01-19 10:30
req-def456   Database migration             approved     2          2024-01-19 09:15

Total: 2 approval requests
```

### runbook approve

Approve a pending approval request.

```bash
kscorectl runbook approve <request-id> [flags]
```

**Arguments**:

- `<request-id>`: Request ID to approve (required)

**Flags**:

- `--reason string`: Reason for approval

**Examples**:

```bash
# Approve a request
kscorectl runbook approve req-123 --reason "Verified prerequisites"

# Approve without reason (if allowed)
kscorectl runbook approve req-123
```

### runbook reject

Reject a pending approval request.

```bash
kscorectl runbook reject <request-id> [flags]
```

**Arguments**:

- `<request-id>`: Request ID to reject (required)

**Required Flags**:

- `--reason string`: Reason for rejection

**Examples**:

```bash
# Reject a request with reason
kscorectl runbook reject req-123 --reason "Replication lag too high"
```

### runbook delegate

Delegate an approval request to another user.

```bash
kscorectl runbook delegate <request-id> [flags]
```

**Arguments**:

- `<request-id>`: Request ID to delegate (required)

**Required Flags**:

- `--to string`: User or group to delegate to

**Examples**:

```bash
# Delegate to another user
kscorectl runbook delegate req-123 --to @another-approver

# Delegate to a group
kscorectl runbook delegate req-123 --to @platform-team
```

### runbook interventions

List pending intervention requests.

```bash
kscorectl runbook interventions [flags]
```

**Flags**:

- `--state string`: Filter by state (pending, completed, expired, cancelled)
- `--execution string`: Filter by execution ID
- `--limit int`: Maximum number of results (default: 50)

**Examples**:

```bash
# List all pending interventions
kscorectl runbook interventions

# Filter by state
kscorectl runbook interventions --state pending

# Filter by execution
kscorectl runbook interventions --execution exec-123
```

### runbook respond

Respond to an intervention request.

```bash
kscorectl runbook respond <request-id> [flags]
```

**Arguments**:

- `<request-id>`: Request ID to respond to (required)

**Flags**:

- `--value strings`: Set a value (format: name=value)
- `--confirmed`: Confirm the request
- `--comment string`: Optional comment

**Examples**:

```bash
# Respond to a prompt
kscorectl runbook respond int-123 --value version=1.0.0 --value replicas=3

# Confirm an action
kscorectl runbook respond int-456 --confirmed --comment "Looks good"

# Acknowledge a manual wait
kscorectl runbook respond int-789 --confirmed --comment "Verified manually"
```

### runbook execute

Execute a runbook by name with optional variables.

```bash
kscorectl runbook execute <runbook-name> [flags]
```

**Arguments**:

- `<runbook-name>`: Name of the runbook to execute (required)

**Flags**:

- `--var strings`: Set a variable (format: key=value, can be repeated)
- `--input strings`: Set an input variable (format: key=value, alias for --var)
- `--dry-run`: Preview execution without running
- `--wait`: Wait for execution to complete
- `--timeout string`: Execution timeout (default: 1h)

**Examples**:

```bash
# Execute a runbook with variables
kscorectl runbook execute deploy-service --var version=1.2.0

# Execute with --input (alias for --var)
kscorectl runbook execute deploy-service --input version=1.2.0 --input env=prod

# Mix --var and --input
kscorectl runbook execute deploy-service --var version=1.2.0 --input env=prod

# Dry run to preview steps
kscorectl runbook execute deploy-service --var version=1.2.0 --dry-run

# Execute with timeout and wait for completion
kscorectl runbook execute deploy-service --var version=1.2.0 --timeout 30m --wait
```

### runbook status

Show the status of a runbook execution.

```bash
kscorectl runbook status <execution-id> [flags]
```

**Arguments**:

- `<execution-id>`: Execution ID to check (required)

**Examples**:

```bash
# View execution status
kscorectl runbook status exec-a1b2c3

# View status in JSON format
kscorectl runbook status exec-a1b2c3 -o json
```

### runbook list-executions

List recent runbook executions with optional filtering.

```bash
kscorectl runbook list-executions [flags]
```

**Flags**:

- `--runbook string`: Filter by runbook name
- `--state string`: Filter by state (pending, running, completed, failed)
- `--since string`: Show executions since duration or date (e.g., '7d', '24h', '2026-01-01')
- `--limit int`: Maximum number of results (default: 20)

**Examples**:

```bash
# List all recent executions
kscorectl runbook list-executions

# Filter by runbook
kscorectl runbook list-executions --runbook deploy-service

# Filter by state
kscorectl runbook list-executions --state running

# Filter by time
kscorectl runbook list-executions --since 7d
kscorectl runbook list-executions --since 2026-01-01
```

### runbook test

Validate a runbook by checking syntax, variables, step dependencies, and permissions. Optionally validate mock handler definitions.

```bash
kscorectl runbook test <runbook-name> [flags]
```

**Flags:**

- `--var stringArray`: Set a variable for validation (format: key=value)
- `--mock-file string`: Path to mock handler definitions (JSON)
- `--verbose`: Show detailed test output

**Examples:**

```bash
# Test a runbook
kscorectl runbook test deploy-service

# Test with variables
kscorectl runbook test deploy-service --var version=1.2.0 --verbose

# Test with mock handlers
kscorectl runbook test deploy-service --mock-file mocks.json --verbose
```

### runbook audit show

Show the audit trail for a specific runbook including executions, approvals, and modifications.

```bash
kscorectl runbook audit show <runbook-name> [flags]
```

**Flags:**

- `--limit int`: Maximum number of entries (default: 20)

**Examples:**

```bash
# View audit trail
kscorectl runbook audit show deploy-service

# Limit results
kscorectl runbook audit show deploy-service --limit 10
```

### runbook audit list

List runbook audit events across all runbooks with optional filtering.

```bash
kscorectl runbook audit list [flags]
```

**Flags:**

- `--runbook string`: Filter by runbook name
- `--start string`: Start time filter (duration like '7d'/'24h' or date 'YYYY-MM-DD')
- `--end string`: End time filter (duration like '1d' or date 'YYYY-MM-DD')
- `--limit int`: Maximum number of entries (default: 50)

**Examples:**

```bash
# List all recent audit events
kscorectl runbook audit list

# Filter by runbook
kscorectl runbook audit list --runbook deploy-service

# Filter by date range
kscorectl runbook audit list --start 2025-01-14 --end 2025-01-15

# Use duration shorthand
kscorectl runbook audit list --start 7d
```

### runbook audit report

Generate a compliance report summarizing runbook audit events by action type, user, and runbook.

```bash
kscorectl runbook audit report [flags]
```

**Flags:**

- `--format string`: Report format: summary, detailed, csv (default: summary)
- `--start string`: Start time filter (duration like '7d'/'24h' or date 'YYYY-MM-DD')
- `--end string`: End time filter (duration like '1d' or date 'YYYY-MM-DD')
- `--runbook string`: Filter by runbook name

**Examples:**

```bash
# Generate summary report
kscorectl runbook audit report

# Detailed report with all events
kscorectl runbook audit report --format detailed

# CSV export for a date range
kscorectl runbook audit report --format csv --start 2025-01-14 --end 2025-01-15

# Report for a specific runbook
kscorectl runbook audit report --runbook deploy-service
```

## kscore-secrets (Secrets Management)

Manage secrets and secret rotation in Keystone Core.

### Global Flags

- `-s, --server string`: Control plane server address (default: localhost:9090)
- `-o, --output string`: Output format: table, json, yaml (default: table)
- `-v, --verbose`: Enable verbose output

### secrets rotate list

List secret rotations.

```bash
kscorectl secrets rotate list [flags]
```

**Aliases**: `ls`

**Flags**:

- `--state string`: Filter by state (pending, in_progress, completed, failed, rolled_back)
- `--strategy string`: Filter by strategy (rolling, blue-green, canary)
- `--label strings`: Filter by label (key:value format)
- `--limit int`: Maximum number of rotations to show (default: 50)

**Examples**:

```bash
# List all rotations
kscorectl secrets rotate list

# List only in-progress rotations
kscorectl secrets rotate list --state in_progress

# List blue-green strategy rotations
kscorectl secrets rotate list --strategy blue-green
```

### secrets rotate show

Show rotation details.

```bash
kscorectl secrets rotate show <rotation-id>
```

**Arguments**:

- `<rotation-id>`: Rotation ID to show (required)

**Examples**:

```bash
# Show rotation details
kscorectl secrets rotate show rot-abc123
```

### secrets rotate start

Start a new secret rotation.

```bash
kscorectl secrets rotate start [flags]
```

**Required Flags**:

- `--secret string`: Secret path to rotate

**Targeting Flags** (at least one required):

- `--target strings`: Target agent IDs
- `--target-tags strings`: Target agents with tags (key:value)
- `--target-roles strings`: Target agents with roles

**Optional Flags**:

- `--strategy string`: Rotation strategy: rolling, blue-green, canary (default: rolling)
- `--batch-size int`: Number of targets per batch (default: 1)
- `--batch-delay string`: Delay between batches (default: 30s)
- `--canary-percentage int`: Percentage of targets for canary (default: 10)
- `--canary-delay string`: Delay after canary verification (default: 5m)
- `--health-check-type string`: Health check type: http, tcp, exec
- `--health-check-url string`: Health check URL (for http type)
- `--health-check-port int`: Health check port (for tcp type)
- `--timeout string`: Overall rotation timeout (default: 30m)
- `--dry-run`: Show what would be done without executing
- `--label strings`: Labels (key:value format)

**Examples**:

```bash
# Start a blue-green rotation
kscorectl secrets rotate start --secret vault/secret/db \
  --strategy blue-green --target-tags env:prod

# Start a canary rotation with 10% canary
kscorectl secrets rotate start --secret vault/secret/api \
  --strategy canary --canary-percentage 10 --canary-delay 5m \
  --target-roles webserver

# Start with health checks
kscorectl secrets rotate start --secret vault/secret/db \
  --strategy rolling --batch-size 2 \
  --health-check-type http --health-check-url http://app:8080/health \
  --target-tags env:prod

# Dry run to see what would happen
kscorectl secrets rotate start --secret vault/secret/db \
  --strategy blue-green --target-tags env:prod --dry-run
```

### secrets rotate status

Show rotation status.

```bash
kscorectl secrets rotate status <rotation-id> [flags]
```

**Arguments**:

- `<rotation-id>`: Rotation ID to show status for (required)

**Flags**:

- `-w, --watch`: Watch status continuously
- `--interval string`: Watch interval (default: 2s)

**Examples**:

```bash
# Show status once
kscorectl secrets rotate status rot-123

# Watch status continuously
kscorectl secrets rotate status rot-123 --watch
```

### secrets rotate history

Show rotation history.

```bash
kscorectl secrets rotate history [secret-path] [flags]
```

**Arguments**:

- `[secret-path]`: Optional secret path to filter by

**Flags**:

- `--limit int`: Number of rotations to show (default: 20)
- `--status string`: Filter by status

**Examples**:

```bash
# Show all rotation history
kscorectl secrets rotate history

# Show history for specific secret
kscorectl secrets rotate history vault/secret/db

# Show only failed rotations
kscorectl secrets rotate history --status failed
```

### secrets rotate trigger

Trigger a scheduled rotation immediately.

```bash
kscorectl secrets rotate trigger <schedule-id>
```

**Arguments**:

- `<schedule-id>`: Schedule ID to trigger (required)

### secrets rotate rollback

Rollback a rotation.

```bash
kscorectl secrets rotate rollback <rotation-id> [flags]
```

**Arguments**:

- `<rotation-id>`: Rotation ID to rollback (required)

**Flags**:

- `-f, --force`: Force rollback without confirmation
- `--reason string`: Reason for rollback

**Examples**:

```bash
# Rollback a rotation
kscorectl secrets rotate rollback rot-123 --reason "health check failures" --force
```

### secrets rotate pause

Pause an in-progress rotation.

```bash
kscorectl secrets rotate pause <rotation-id>
```

### secrets rotate resume

Resume a paused rotation.

```bash
kscorectl secrets rotate resume <rotation-id>
```

### secrets rotate cancel

Cancel an in-progress rotation.

```bash
kscorectl secrets rotate cancel <rotation-id> [flags]
```

**Flags**:

- `--reason string`: Cancellation reason

### secrets schedule list

List rotation schedules.

```bash
kscorectl secrets schedule list
```

**Aliases**: `ls`

### secrets schedule show

Show schedule details.

```bash
kscorectl secrets schedule show <schedule-id>
```

### secrets schedule create

Create a rotation schedule.

```bash
kscorectl secrets schedule create [flags]
```

**Required Flags**:

- `--secret string`: Secret path
- `--schedule string`: Cron schedule

**Optional Flags**:

- `--strategy string`: Rotation strategy (default: rolling)
- `--target strings`: Target agent IDs
- `--target-tags strings`: Target tags
- `--batch-size int`: Batch size (default: 1)
- `--batch-delay string`: Batch delay (default: 30s)
- `--canary-percentage int`: Canary percentage (default: 10)
- `--health-check-type string`: Health check type
- `--health-check-url string`: Health check URL
- `--enabled`: Enable schedule (default: true)
- `--label strings`: Labels

**Examples**:

```bash
# Create daily rotation at 2am
kscorectl secrets schedule create --secret vault/secret/db \
  --schedule "0 2 * * *" --strategy blue-green --target-tags env:prod

# Create weekly rotation
kscorectl secrets schedule create --secret vault/secret/api \
  --schedule "0 3 * * 0" --strategy canary --canary-percentage 10
```

### secrets schedule enable

Enable a schedule.

```bash
kscorectl secrets schedule enable <schedule-id>
```

### secrets schedule disable

Disable a schedule.

```bash
kscorectl secrets schedule disable <schedule-id>
```

### secrets schedule delete

Delete a schedule.

```bash
kscorectl secrets schedule delete <schedule-id> [flags]
```

**Flags**:

- `-f, --force`: Force deletion without confirmation

### secrets policy list

List rotation policies.

```bash
kscorectl secrets policy list
```

### secrets policy show

Show policy details.

```bash
kscorectl secrets policy show <policy-id>
```

### secrets policy create

Create a rotation policy.

```bash
kscorectl secrets policy create [flags]
```

**Required Flags**:

- `--name string`: Policy name
- `--pattern string`: Secret path pattern

**Optional Flags**:

- `--max-age string`: Maximum secret age before rotation (default: 90d)
- `--strategy string`: Default rotation strategy (default: rolling)
- `--batch-size int`: Default batch size (default: 1)
- `--health-required`: Require health checks
- `--enabled`: Enable policy (default: true)

**Examples**:

```bash
# Create a policy for database secrets
kscorectl secrets policy create --name db-policy \
  --pattern "vault/secret/database/*" --max-age 90d --strategy blue-green

# Create a strict policy requiring health checks
kscorectl secrets policy create --name api-policy \
  --pattern "vault/secret/api/*" --max-age 30d --health-required
```

### secrets policy delete

Delete a policy.

```bash
kscorectl secrets policy delete <policy-id> [flags]
```

**Flags**:

- `-f, --force`: Force deletion without confirmation

### secrets rotate-keys

Rotate encryption keys for all secrets backends. This is a security incident response action.

```bash
kscorectl secrets rotate-keys
kscorectl secrets rotate-keys --force
```

**Flags**:

- `-f, --force`: Skip confirmation prompt

## kscore-transfer (Air-Gapped Data Transfer)

Export and import operational data across air-gapped boundaries. Packages are signed,
optionally encrypted archives containing audit logs, events, state history, and other
data suitable for transfer via USB, sneakernet, or data diode.

### transfer export

Export operational data as a signed, optionally encrypted archive.

```bash
# Export audit logs from the last 24 hours
kscorectl transfer export --type audit --since 24h -O audit-export.tar.gz

# Export everything, signed and encrypted
kscorectl transfer export --type full --sign-key key.pem \
  --encrypt-to age1recipient -O full-export.tar.gz

# Export events with a specific time range
kscorectl transfer export --type events --since 2024-01-15T00:00:00Z \
  --until 2024-01-16T00:00:00Z --events-db /var/lib/kscore/events.db
```

**Flags**:

- `--type` (required): Export type — `audit`, `events`, `state`, `full`
- `--since`: Start of time range (duration like `24h` or RFC3339 timestamp)
- `--until`: End of time range (RFC3339 timestamp; default: now)
- `-O, --output`: Output archive path
- `--sign-key`: PEM private key path for signing
- `--encrypt-to`: age public key for encryption
- `--created-by`: Creator identifier
- `--events-db`: Path to events SQLite database (default: `events.db`)
- `--state-db`: Path to state history SQLite database

### transfer import

Import and verify an export package, extracting data files.

```bash
# Preview package contents
kscorectl transfer import package.tar.gz --preview

# Import with signature verification
kscorectl transfer import package.tar.gz --verify-key release.pub \
  --output-dir ./imported

# Import encrypted package
kscorectl transfer import package.tar.gz.age \
  --age-identity identity.txt --output-dir ./imported

# Import only specific datasets
kscorectl transfer import package.tar.gz \
  --output-dir ./imported --datasets audit,events
```

**Flags**:

- `--verify-key`: PEM public key path for signature verification
- `--age-identity`: age identity file path for decryption
- `--preview`: Preview contents without extracting
- `--output-dir`: Extraction destination directory
- `--datasets`: Comma-separated dataset filter

### transfer verify

Verify an export package's signatures, checksums, and manifest without extracting.

```bash
kscorectl transfer verify package.tar.gz --verify-key release.pub
```

**Flags**:

- `--verify-key`: PEM public key path for signature verification

### transfer sync

Manage sync windows for periodic-connectivity environments. Sync windows define
time-limited windows during which data can be transferred between air-gapped and
connected networks, with bandwidth limiting and operation prioritization.

> **Note:** Sync window management requires a running sync daemon. These commands
> will be fully functional when daemon mode is available.

```bash
kscorectl transfer sync list
kscorectl transfer sync show <name>
kscorectl transfer sync trigger <name>
kscorectl transfer sync pause <name>
kscorectl transfer sync resume <name>
kscorectl transfer sync cancel <name>
kscorectl transfer sync history
```

**Subcommands**:

| Command | Description |
|---------|-------------|
| `list` | List configured sync windows |
| `show <name>` | Show sync window details |
| `trigger <name>` | Trigger a sync window immediately |
| `pause <name>` | Pause a running sync window |
| `resume <name>` | Resume a paused sync window |
| `cancel <name>` | Cancel a running sync window |
| `history` | Show sync window execution history |

### transfer diode

Transfer data over a unidirectional UDP data diode connection. Data diodes provide
hardware-enforced one-way data transfer, commonly used in classified and high-security
environments.

#### transfer diode send

Send a file through a UDP data diode connection.

```bash
# Send a file
kscorectl transfer diode send --address 10.0.0.2:9000 --file export.tar.gz

# Send with FEC for packet loss recovery
kscorectl transfer diode send --address 10.0.0.2:9000 --file data.bin --fec

# Send with bandwidth limiting
kscorectl transfer diode send --address 10.0.0.2:9000 --file data.bin --rate-limit 1048576
```

**Flags**:

- `--address` (required): Destination address `host:port`
- `--file` (required): File to send
- `--packet-size`: UDP packet size in bytes (default: 1400)
- `--rate-limit`: Bandwidth limit in bytes/sec (0 = unlimited)
- `--fec`: Enable forward error correction
- `--fec-redundancy`: FEC group size (default: 5)

#### transfer diode receive

Listen for incoming data diode transfers over UDP.

```bash
# Receive to a directory
kscorectl transfer diode receive --listen :9000 --output-dir ./received

# Receive with FEC recovery and custom timeout
kscorectl transfer diode receive --listen :9000 --output-dir ./received --fec --timeout 60s
```

**Flags**:

- `--listen`: Listen address `host:port` (default: `:9000`)
- `--output-dir` (required): Output directory for received files
- `--timeout`: Receive timeout (default: `30s`)
- `--fec`: Enable forward error correction recovery

### bootstrap airgap-validate

Scan binaries, configuration files, module registries, and active network connections
to identify external dependencies that would break air-gapped operation. Produces a
compliance report with pass/warn/fail findings and remediation guidance. Exits with
code 1 if the system is not air-gap compliant.

```bash
# Scan binaries and configuration
kscorectl bootstrap airgap-validate --binary-dir /usr/local/bin --config-dir /etc/keystone-core

# Scan with internal network allowlist
kscorectl bootstrap airgap-validate --registry /opt/registry \
  --internal-net 10.0.0.0/8 --internal-net 172.16.0.0/12

# Write JSON report
kscorectl bootstrap airgap-validate --binary-dir /usr/local/bin \
  --config-dir /etc/keystone-core --output-file report.json
```

**Flags**:

- `--binary-dir`: Directory containing `kscore-*` binaries to scan
- `--config-dir`: Directory containing configuration files to scan
- `--registry`: Local module registry directory
- `--internal-net`: Internal network CIDR (repeatable, e.g. `10.0.0.0/8`)
- `--output-file`: Write JSON report to file

**Checks performed**:

| Check | Category | Description |
|-------|----------|-------------|
| Binary URL scan | binary | Scans binaries for embedded external URLs |
| Config external refs | configuration | Scans configs for external hostnames, registries, IPs |
| Module availability | module | Verifies required modules exist in local registry |
| Network connections | network | Checks `/proc/net/tcp` for external connections (Linux only) |

## See Also

- [API Reference](../api/) - REST/gRPC API
- [Configuration Reference](../configuration/) - Configuration options
- [Getting Started](../../getting-started/quick-start/) - Quick start guide
- [File Distribution Concepts](../../concepts/file-distribution/) - File distribution overview
- [Observability Gateway Operations](../../operations/gateway/) - Gateway deployment guide
