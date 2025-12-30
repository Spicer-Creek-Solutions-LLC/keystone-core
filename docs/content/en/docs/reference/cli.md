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

## kscorectl (Main CLI)

The kscorectl command dispatches to plugins and provides core functionality.

### Global Flags

Available for all commands:

```
--config string      Config file (default: $HOME/.kscore/config.yaml)
--server string      Control plane server URL
--api-key string     API key for authentication
--output string      Output format: text, json, yaml (default: text)
--verbose           Enable verbose output
--quiet             Suppress non-essential output
--no-color          Disable colored output
```

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

## kscore-exec (Remote Execution)

Execute commands on remote agents.

### exec run

Execute command synchronously.

```bash
kscorectl exec run <command> [flags]
```

**Arguments**:
- `<command>`: Command to execute (required)

**Flags**:
- `--target string`: Target expression (required)
- `--timeout duration`: Execution timeout (default: 5m)
- `--batch-size int`: Batch size for parallel execution
- `--batch-delay duration`: Delay between batches
- `--shell string`: Shell to use (bash, sh, powershell, cmd)
- `--async`: Run asynchronously
- `--output-format string`: Output format (text, json, table)

**Examples**:
```bash
# Execute on all web servers
kscorectl exec run "uptime" --target "role:web"

# Execute with timeout
kscorectl exec run "systemctl restart nginx" \
  --target "datacenter:us-east-1 and role:web" \
  --timeout 30s

# Execute in batches
kscorectl exec run "apt-get update" \
  --target "os:ubuntu" \
  --batch-size 10 \
  --batch-delay 5s

# Async execution
kscorectl exec run "long-running-task.sh" \
  --target "role:worker" \
  --async
```

**Output**:
```
Executing on 50 agents matching: role:web

web-01 (us-east-1): SUCCESS (exit code: 0)
  stdout: 10:30:45 up 5 days, 2:15, 1 user, load average: 0.50, 0.45, 0.40

web-02 (us-east-1): SUCCESS (exit code: 0)
  stdout: 10:30:45 up 3 days, 1:20, 1 user, load average: 0.30, 0.25, 0.20

...

Summary:
  Total: 50
  Success: 48
  Failed: 2
  Duration: 2.5s
```

### exec async

Execute command asynchronously.

```bash
kscorectl exec async <command> [flags]
```

Same as `exec run --async`. Returns job ID immediately.

**Output**:
```
Job ID: job-abc123
Status: running
Target count: 50

Use 'kscorectl exec status job-abc123' to check status.
```

### exec status

Check job status.

```bash
kscorectl exec status <job-id>
```

**Examples**:
```bash
kscorectl exec status job-abc123
```

**Output**:
```
Job ID: job-abc123
Command: systemctl restart nginx
Status: completed
Started: 2024-01-15 10:30:45
Completed: 2024-01-15 10:30:47
Duration: 2.1s

Results:
  Total: 50
  Success: 48
  Failed: 2
  Timeout: 0
```

### exec output

Get job output.

```bash
kscorectl exec output <job-id> [flags]
```

**Flags**:
- `--agent string`: Filter by agent ID
- `--status string`: Filter by status (success, failed, timeout)
- `--format string`: Output format (text, json)

**Examples**:
```bash
# All output
kscorectl exec output job-abc123

# Specific agent
kscorectl exec output job-abc123 --agent web-01

# Only failures
kscorectl exec output job-abc123 --status failed
```

### exec list

List recent jobs.

```bash
kscorectl exec list [flags]
```

**Flags**:
- `--status string`: Filter by status
- `--since duration`: Jobs since duration (e.g., 1h, 24h)
- `--limit int`: Max results (default: 100)

**Output**:
```
JOB ID       COMMAND                    STATUS      STARTED              AGENTS
job-abc123   systemctl restart nginx    completed   2024-01-15 10:30:45  50
job-def456   uptime                     completed   2024-01-15 10:25:30  100
job-ghi789   apt-get update             running     2024-01-15 10:20:15  200
```

## kscore-state (State Management)

Manage declarative state configurations.

### state apply

Apply state configuration.

```bash
kscorectl state apply <state-file> [flags]
```

**Arguments**:
- `<state-file>`: Path to state YAML file (required)

**Flags**:
- `--target string`: Target expression (required)
- `--vars string`: Variables file (YAML)
- `--check-only`: Dry-run mode (don't apply changes)
- `--verbose`: Show detailed output
- `--output string`: Output format (text, json, summary)

**Examples**:
```bash
# Apply state
kscorectl state apply web-server.yaml --target "role:web"

# Dry run
kscorectl state apply web-server.yaml \
  --target "role:web" \
  --check-only

# With variables
kscorectl state apply app.yaml \
  --target "environment:production" \
  --vars prod-vars.yaml
```

**Output**:
```
Applying state to 50 agents matching: role:web

web-01:
  ✓ nginx_package (package.installed): unchanged
  ✓ nginx_config (file.present): changed
  ✓ nginx_service (service.running): changed (restarted)

web-02:
  ✓ nginx_package (package.installed): unchanged
  ✓ nginx_config (file.present): unchanged
  ✓ nginx_service (service.running): unchanged

...

Summary:
  Total agents: 50
  Success: 50
  Failed: 0
  Total states: 150
  Changed: 75
  Unchanged: 75
  Duration: 30s
```

### state check

Check state without applying (dry-run).

```bash
kscorectl state check <state-file> [flags]
```

Same as `state apply --check-only`.

**Output**:
```
Checking state on 50 agents matching: role:web

web-01:
  → nginx_package (package.installed): would be unchanged
  → nginx_config (file.present): would be changed
      - Contents differ
  → nginx_service (service.running): would be changed
      - Would restart due to config change

Summary:
  Total states: 150
  Would change: 75
  Would be unchanged: 75
```

### state drift

Detect configuration drift.

```bash
kscorectl state drift <state-file> [flags]
```

**Flags**:
- `--target string`: Target expression (required)
- `--severity string`: Min severity to report (low, medium, high, critical)

**Examples**:
```bash
# Detect all drift
kscorectl state drift web-server.yaml --target "role:web"

# Only critical drift
kscorectl state drift web-server.yaml \
  --target "role:web" \
  --severity critical
```

**Output**:
```
Drift detected on 5/50 agents

web-01: HIGH severity drift
  nginx_service:
    Expected: running
    Actual: stopped
    Severity: high

web-03: MEDIUM severity drift
  nginx_config:
    Expected mode: 0644
    Actual mode: 0755
    Severity: medium

Summary:
  Total agents: 50
  With drift: 5
  Severity breakdown:
    Critical: 0
    High: 2
    Medium: 3
    Low: 0
```

### state show

Display rendered state file.

```bash
kscorectl state show <state-file> [flags]
```

**Flags**:
- `--vars string`: Variables file
- `--format string`: Output format (yaml, json)

**Examples**:
```bash
# Show rendered state
kscorectl state show app.yaml --vars prod-vars.yaml
```

**Output**:
```yaml
nginx_package:
  module: package
  state: installed
  name: nginx

nginx_config:
  module: file
  state: present
  path: /etc/nginx/nginx.conf
  contents: |
    worker_processes 4;
    ...
```

### state list

List applied states.

```bash
kscorectl state list [flags]
```

**Flags**:
- `--agent string`: Filter by agent ID
- `--since duration`: States applied since duration

**Output**:
```
RUN ID        STATE FILE       TARGET     STARTED              STATUS
run-abc123    web-server.yaml  role:web   2024-01-15 10:30:45  completed
run-def456    app.yaml         role:app   2024-01-15 09:15:20  completed
```

## kscore-monitor (Real-time Monitoring)

Terminal-based real-time monitoring.

### monitor

Start TUI monitor.

```bash
kscorectl monitor [flags]
```

**Flags**:
- `--view int`: Initial view (1-8)
- `--server string`: Control plane server URL
- `--refresh duration`: Refresh interval (default: 2s)

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

Manage Keystone Core modules (Epic 9).

### module init

Initialize new module.

```bash
kscorectl module init <name> [flags]
```

**Flags**:
- `--type string`: Module type (starlark, wasm-rust, wasm-go, wasm-cpp)
- `--template string`: Template to use (default, minimal, full)

**Examples**:
```bash
# Starlark module
kscorectl module init myorg/custom-state --type starlark

# Rust WASM module
kscorectl module init myorg/custom-executor --type wasm-rust
```

### module build

Build module.

```bash
kscorectl module build [flags]
```

**Flags**:
- `--output string`: Output file path

**Examples**:
```bash
cd mymodule
kscorectl module build
```

### module test

Run module tests.

```bash
kscorectl module test [flags]
```

**Flags**:
- `--verbose`: Verbose test output
- `--coverage`: Generate coverage report

### module sign

Sign module with cosign.

```bash
kscorectl module sign <module.zip> [flags]
```

**Flags**:
- `--key string`: Signing key path
- `--cert string`: Certificate path

### module publish

Publish module to registry.

```bash
kscorectl module publish <module.zip> [flags]
```

**Flags**:
- `--registry string`: Registry URL

### module install

Install module from registry.

```bash
kscorectl module install <module> [flags]
```

**Arguments**:
- `<module>`: Module name (vendor/package@version)

**Examples**:
```bash
# Install specific version
kscorectl module install std/files@1.0.0

# Install latest
kscorectl module install myorg/custom-state
```

## kscore-policy (Policy Management)

Manage policies (Epic 6).

### policy check

Check resources against policies.

```bash
kscorectl policy check <policy> [flags]
```

**Flags**:
- `--input string`: Input file (JSON/YAML)
- `--verbose`: Show detailed evaluation

**Examples**:
```bash
# Check file against policy
kscorectl policy check ssh-hardening --input sshd_config.json
```

### policy list

List policies.

```bash
kscorectl policy list [flags]
```

**Flags**:
- `--category string`: Filter by category
- `--type string`: Filter by type (opa, cel, builtin)

**Output**:
```
ID              TYPE  CATEGORY    SEVERITY  ENFORCEMENT
ssh-hardening   opa   security    high      enforce
cost-limits     cel   cost        medium    warn
required-tags   cel   compliance  low       audit
```

### policy violations

List policy violations.

```bash
kscorectl policy violations [flags]
```

**Flags**:
- `--policy string`: Filter by policy ID
- `--severity string`: Filter by severity
- `--since duration`: Violations since duration

**Output**:
```
POLICY          AGENT   SEVERITY  MESSAGE                              DETECTED
ssh-hardening   web-01  high      SSH must not use default port 22     2024-01-15 10:30
firewall-rules  web-02  medium    Firewall must allow HTTPS            2024-01-15 10:25
```

### policy compliance

Show compliance report.

```bash
kscorectl policy compliance [flags]
```

**Flags**:
- `--environment string`: Filter by environment
- `--period string`: Time period (24h, 7d, 30d)

**Output**:
```
Overall Compliance: 87.5%

Policy Set: security-baseline
  Compliance: 92.3%
  Policies: 12 total, 11 compliant, 1 violating

Top Violations:
  1. ssh-hardening (15 agents)
  2. firewall-rules (8 agents)
  3. package-updates (5 agents)
```

## kscore-gitops (GitOps Management)

Manage GitOps integrations (Epic 5).

### gitops verify

Run deployment verification.

```bash
kscorectl gitops verify <workflow> [flags]
```

**Flags**:
- `--application string`: Application name
- `--namespace string`: Kubernetes namespace

**Examples**:
```bash
kscorectl gitops verify myapp-verification \
  --application myapp \
  --namespace production
```

### gitops rollback

Trigger rollback.

```bash
kscorectl gitops rollback <application> [flags]
```

**Flags**:
- `--namespace string`: Namespace (required)
- `--strategy string`: Rollback strategy (previous, last_known_good, specific)
- `--revision string`: Specific revision (for specific strategy)

**Examples**:
```bash
# Rollback to previous
kscorectl gitops rollback myapp --namespace production

# Rollback to specific revision
kscorectl gitops rollback myapp \
  --namespace production \
  --strategy specific \
  --revision abc123
```

### gitops promote

Promote between environments.

```bash
kscorectl gitops promote <application> [flags]
```

**Flags**:
- `--from string`: Source environment
- `--to string`: Target environment

**Examples**:
```bash
kscorectl gitops promote myapp --from staging --to production
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
  --sqlite /var/lib/kscore/state.db \
  --postgres "postgres://kscore:password@localhost/keystonecore"

# Dry run first
kscorectl migrate run \
  --sqlite /var/lib/kscore/state.db \
  --postgres "postgres://kscore:password@localhost/keystonecore" \
  --dry-run --verbose

# Continue on errors
kscorectl migrate run \
  --sqlite /var/lib/kscore/state.db \
  --postgres "postgres://kscore:password@localhost/keystonecore" \
  --continue-on-error
```

**Output**:
```
Starting migration from SQLite to PostgreSQL...
  Mode: DRY RUN (no data will be written)
  Source: /var/lib/kscore/state.db
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
  --sqlite /var/lib/kscore/state.db \
  --postgres "postgres://kscore:password@localhost/keystonecore"
```

**Output (Success)**:
```
Validating migration...
  Source: /var/lib/kscore/state.db
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

## Configuration File

Default location: `~/.kscore/config.yaml`

```yaml
# Control plane connection
server: "http://control-plane.example.com:8080"
api_key: "ta_live_abc123xyz789"

# TLS configuration
tls:
  enabled: true
  ca_cert: "/etc/kscore/ca.crt"
  client_cert: "/etc/kscore/client.crt"
  client_key: "/etc/kscore/client.key"

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
TITAN_SERVER="http://control-plane:8080"
TITAN_API_KEY="ta_live_abc123"
TITAN_CONFIG="/custom/config.yaml"
TITAN_OUTPUT_FORMAT="json"
TITAN_NO_COLOR="true"
```

**Example**:
```bash
export TITAN_SERVER="http://localhost:8080"
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
tas web-server.yaml --target "role:web"
tam
```

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
kscorectl state apply web-server.yaml --target "role:web"

# Check for drift
kscorectl state drift web-server.yaml --target "role:web"

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
# Check compliance
kscorectl policy compliance --environment production

# List violations
kscorectl policy violations --severity high

# Remediate
kscorectl state apply security-baseline.yaml \
  --target "environment:production"
```

## See Also

- [API Reference](../api/) - REST/gRPC API
- [Configuration Reference](../configuration/) - Configuration options
- [Getting Started](../../getting-started/quick-start/) - Quick start guide
