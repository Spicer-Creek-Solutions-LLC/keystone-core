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

### kscorectl config validate

Validate a configuration file against schema rules.

```bash
kscorectl config validate --config /etc/kscore/server.yaml
```

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

Manage Keystone Core modules with dependency resolution, verification, and distribution.

Keystone Core modules are versioned, capability-scoped packages that extend the system with custom state management, reactors, policies, and more. The CLI provides commands for the full module lifecycle: scaffolding, validation, building, testing, and distribution.

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
- `--output string`: Output ZIP file path (default: `<name>-<version>.zip`)
- `--exclude strings`: Glob patterns to exclude (default: tests, .git, .gitignore, *.md)
- `--no-validate`: Skip validation before building

**Examples**:
```bash
# Build in current directory
kscorectl module build

# Build specific directory
kscorectl module build ./my-module

# Custom output file
kscorectl module build --output dist/my-module-1.0.0.zip

# Exclude additional patterns
kscorectl module build --exclude "*.log" --exclude "tmp/*"
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
- `--registry string`: Registry URL (default: KSCORE_REGISTRY env or https://registry.keystonecore.io)
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
- `--registry string`: Registry URL (defaults to KSCORE_REGISTRY env var or https://registry.keystonecore.io)
- `--token string`: Authentication token (can also use KSCORE_REGISTRY_TOKEN)
- `--username string`: Username for basic auth (can also use KSCORE_REGISTRY_USERNAME)
- `--password string`: Password for basic auth (can also use KSCORE_REGISTRY_PASSWORD)
- `--cache-dir string`: Module cache directory (default: `KSCORE_CACHE_DIR` or `~/.kscore/modules`)
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
- `--dir string`: Installation directory (default: ~/.kscore/blueprints)
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
  Extracting to ~/.kscore/blueprints/community/nginx

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
- `--dir string`: Blueprint directory (default: ~/.kscore/blueprints)
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
- `--dir string`: Blueprint directory (default: ~/.kscore/blueprints)
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
  Removing ~/.kscore/blueprints/community/nginx

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
- `--dir string`: Blueprint directory (default: ~/.kscore/blueprints)
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
- `-o, --output string`: Output format: text, json (default: text)

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
- `-o, --output string`: Output format: text, json, yaml (default: text)

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

### policy audit

Display the policy evaluation audit log.

```bash
kscorectl policy audit [flags]
```

**Flags**:
- `--policy string`: Filter by policy ID
- `--resource-type string`: Filter by resource type
- `--denied`: Show only denied evaluations
- `--limit int`: Maximum entries to show (default: 100)
- `-o, --output string`: Output format: table, json (default: table)

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
- `-o, --output string`: Output format: text, json (default: text)

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

> **Note**: The CLI can operate against sample/in-memory data when the control plane API is not connected.
> Use the control plane API for production audit history.

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

## kscore-gitops (GitOps Management)

Manage GitOps deployments, verifications, rollbacks, and promotions with ArgoCD, Flux, GitHub, and GitLab integrations.

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
- `-o, --output string`: Output format: text, json (default: text)

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
- `-o, --output string`: Output format: text, json (default: text)

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
- `-o, --output string`: Output format: text, json (default: text)

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
- `-o, --output string`: Output format: text, json (default: text)

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

## kscore-webhook (Webhook Management)

Manage webhook handlers, test payloads, delivery history, and secrets for GitOps integrations.

> **Note**: The CLI can return sample data when not connected to the control plane API.
> Configure webhook endpoints on the control plane for production use.

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
- `--filter string`: Filter members by status (healthy, degraded, unhealthy)

**Example**:
```bash
kscorectl cluster members --output json
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

### cluster backup

Create cluster state backup.

```bash
kscorectl cluster backup [flags]
```

**Flags**:
- `-o, --output string`: Output file path (default: stdout)
- `--format string`: Output format (json, yaml) (default: json)

**Examples**:
```bash
# Backup to stdout
kscorectl cluster backup

# Backup to file
kscorectl cluster backup -o /var/backups/kscore/cluster-backup.json

# Backup in YAML format
kscorectl cluster backup --format yaml -o cluster-backup.yaml
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
kscorectl cluster restore <backup-file> [flags]
```

**Flags**:
- `--force`: Override safety checks
- `--shards-only`: Restore only shard assignments
- `--config-only`: Restore only configuration
- `--dry-run`: Show what would be restored without making changes

**Examples**:
```bash
# Basic restore
kscorectl cluster restore cluster-backup.json

# Force restore on healthy cluster
kscorectl cluster restore cluster-backup.json --force

# Dry run to preview changes
kscorectl cluster restore cluster-backup.json --dry-run

# Restore only configuration
kscorectl cluster restore cluster-backup.json --config-only
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
- `--target string`: Target member to rebalance from

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
- `--cron string`: Cron expression (default: "0 0 * * *")
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
- `--audit-level string`: Audit logging level: all, errors, none (default: errors)
- `--audit-output string`: Audit log destination (default: system-dependent)

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
- `--path string`: SPIFFE path for agents using this token (default: /agent/default)
- `--ttl string`: Token time-to-live (default: 5m)
- `--uses int`: Maximum number of uses, 0 for unlimited (default: 1)

**Examples**:
```bash
# Create a token with default settings
kscorectl identity token create

# Create a token with custom path and TTL
kscorectl identity token create --path /agent/web --ttl 10m

# Create a token that can be used 5 times
kscorectl identity token create --path /agent/db --ttl 1h --uses 5
```

**Output**:
```
Token created successfully!
Token:    Rj2k9xLm3n4o5p6q7r8s9t0u1v2w3x4y5z
Path:     /agent/web
TTL:      10m
Max Uses: 1

Configure agent with:
  identity:
    attestation:
      type: join_token
      token: "<your-join-token>"
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
- `--endpoint string`: Bundle endpoint URL (required)
- `--profile string`: Bundle endpoint profile (default: https_web)
- `--type string`: Federation type: bidirectional, unidirectional (default: bidirectional)
- `--refresh-interval string`: Bundle refresh interval (default: 5m)

**Examples**:
```bash
# Add bidirectional federation
kscorectl identity federation add partner.example.org \
  --endpoint https://partner.example.org/.well-known/spiffe-bundle

# Add unidirectional federation with custom refresh interval
kscorectl identity federation add vendor.example.com \
  --endpoint https://vendor.example.com/.well-known/spiffe-bundle \
  --type unidirectional \
  --refresh-interval 1h

# Add federation with SPIFFE bundle profile
kscorectl identity federation add vendor.example.com \
  --endpoint https://vendor.example.com/bundle \
  --profile https_spiffe
```

**Output**:
```
Federation relationship added: partner.example.org
Bundle Endpoint: https://partner.example.org/.well-known/spiffe-bundle
Profile: https_web
Type: bidirectional
Refresh Interval: 5m

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
kscorectl identity token create --path /agent/web-server-1 --ttl 10m

# Copy token to agent configuration
# Start agent - it will use the token to register
```

**Set up trust federation**:
```bash
# Add federation relationship
kscorectl identity federation add partner.example.org \
  --endpoint https://partner.example.org/.well-known/spiffe-bundle

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
- `--cors-origins string`: Comma-separated list of allowed CORS origins (e.g., 'https://example.com,https://app.example.com')

**Examples**:
```bash
# Start with defaults
kscore-registry

# Start with custom data directory and port
kscore-registry --data /var/lib/kscore/modules --listen :8080

# Start in read-only mirror mode
kscore-registry --data /var/lib/kscore/modules --readonly

# Start with authentication required for writes
kscore-registry --api-key "your-secret-api-key"
# Or via environment variable:
export KSCORE_REGISTRY_API_KEY="your-secret-api-key"
kscore-registry

# Enable CORS for web-based module browsers
kscore-registry --cors --cors-origins "https://example.com,https://app.example.com"

# Enable CORS with wildcard (use with caution)
kscore-registry --cors --cors-origins "*"
```

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

### Data Storage

Modules are stored in a file-based structure:

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

Default location: `~/.kscore/config.yaml`

```yaml
# Control plane connection
server: "http://control-plane.example.com:8080"
api_key: "<your-api-key>"

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
KSCORE_CACHE_DIR="/var/cache/kscore/modules"
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

Manage agent inventory, tokens, tags, and status. Invoked via `kscorectl agent`.

### Global Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--server, -s` | Control plane gRPC address | `localhost:9090` |
| `--output, -o` | Output format (table, json, yaml, wide) | `table` |
| `--verbose, -v` | Verbose output | `false` |

### agent list

List registered agents with filters.

```bash
kscorectl agent list --status online --label role=web --limit 100
kscorectl agent list --edge --show-compatibility
```

### agent show

```bash
kscorectl agent show <agent-id>
```

### agent delete

```bash
kscorectl agent delete <agent-id> [--force]
```

### agent quarantine / unquarantine

```bash
kscorectl agent quarantine <agent-id> --reason "Suspicious activity"
kscorectl agent unquarantine <agent-id>
```

### agent status

```bash
kscorectl agent status
kscorectl agent status <agent-id>
```

### agent tags (labels)

```bash
kscorectl agent tags set <agent-id> role=web env=prod
kscorectl agent tags add <agent-id> monitoring=enabled
kscorectl agent tags remove <agent-id> monitoring
kscorectl agent tags show <agent-id>
```

### agent token

```bash
kscorectl agent token create --ttl 1h --max-uses 10
kscorectl agent token list
kscorectl agent token revoke <token-id>
```

### agent renew-svid

```bash
kscorectl agent renew-svid <agent-id> --force
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

```bash
kscorectl loadtest run --agents 100 --scenario registration
kscorectl loadtest run --agents 50 --scenario commands --commands-per-agent 10
kscorectl loadtest run --agents 200 --scenario sustained --duration 5m --ramp-up 30s
```

### loadtest scenarios

```bash
kscorectl loadtest scenarios
```

### loadtest report

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

```bash
kscorectl test smoke --target "role:web" --timeout 5m
```

### test integration

```bash
kscorectl test integration --suite recovery --target "role:control-plane"
kscorectl test integration --suite basic,state --parallel 2
```

### test run

```bash
kscorectl test run --suite e2e --timeout 1h --parallel 4
kscorectl test run --suite basic --dry-run
```

### test list / show / history

```bash
kscorectl test list
kscorectl test show <test-id>
kscorectl test history --limit 20
```

### test suite

```bash
kscorectl test suite list
kscorectl test suite show <suite-name>
```

## kscore-agent (Agent Daemon)

The agent daemon runs on managed nodes. It's not invoked via kscorectl.

### Running the Agent

```bash
# Run in foreground (console mode)
kscore-agent --config /etc/kscore/agent.yaml

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
--config string   Config file path
                  Linux/macOS: /etc/kscore/agent.yaml
                  Windows: C:\ProgramData\kscore\agent.yaml
-h, --help        Show help
```

### Agent Bootstrap (kscore-agent bootstrap)

Bootstrap a Keystone Core deployment using the single-binary flow (Epic 27).

```bash
kscore-agent bootstrap --mode production --cluster-name prod --node-role control-plane
kscore-agent bootstrap --config bootstrap.yaml --non-interactive
```

**Common Flags**:
```
--mode string                Deployment mode: demo, production, fullscale, custom
--cluster-name string        Cluster name
--node-role string           Node role: control-plane, agent, both
--node-name string           Node name (defaults to hostname)
--node-label string          Node labels (key=value, repeatable)
--join string                Cluster endpoint to join
--join-token string          Join token for authentication
--storage-backend string     Storage backend: sqlite, postgres
--nats-mode string           NATS mode: embedded, cluster, external, leaf
--nats-urls strings          External NATS URLs
--bind-address string        Address to bind services
--advertise-address string   Address to advertise to cluster
--generate-certs             Generate self-signed certificates
--blueprints-dir string      Directory containing blueprints
--apply-blueprint strings    Blueprints to apply after bootstrap
--non-interactive            Run without interactive prompts
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
# Run in foreground
kscore-server --config /etc/kscore/server.yaml

# Show version
kscore-server version
```

### Server Flags

```
--config string   Config file path (default: /etc/kscore/server.yaml)
-h, --help        Show help
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
# Check compliance
kscorectl policy compliance --environment production

# List violations
kscorectl policy violations --severity high

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
| `-o, --output string` | Output format: text, json, yaml, table | `text` |
| `-h, --help` | Show help | |

### serve

Run the file distribution server.

```bash
kscore-files serve [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--config string` | Configuration file path | |
| `--nats-url string` | NATS server URL | `nats://localhost:4222` |
| `--cluster-id string` | Cluster identifier for routing | |
| `--instance-id string` | Instance identifier for HA deployments | |
| `--listen string` | HTTP listen address | `:8080` |

**Examples:**

```bash
# Run with configuration file
kscore-files serve --config /etc/kscore/files.yaml

# Run with NATS connection
kscore-files serve --nats-url nats://localhost:4222

# Run with HA configuration
kscore-files serve --config /etc/kscore/files.yaml --instance-id files-1
```

### files

Manage files in the distribution system.

#### files list

List files in a namespace.

```bash
kscore-files files list <namespace> [flags]
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
kscore-files files list packages

# List files recursively with path filter
kscore-files files list packages --path /myapp --recursive

# Output as JSON
kscore-files files list packages -o json
```

#### files put

Upload a file to a namespace.

```bash
kscore-files files put <local-path> <namespace>/<remote-path> [flags]
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
kscore-files files put ./myapp-1.0.0.tar.gz packages/myapp/v1.0.0.tar.gz

# Upload with metadata
kscore-files files put ./config.yaml configs/app/config.yaml --metadata '{"version":"1.0"}'
```

#### files get

Download a file from a namespace.

```bash
kscore-files files get <namespace>/<remote-path> <local-path> [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--verify` | Verify checksum after download | `true` |

**Examples:**

```bash
# Download a file
kscore-files files get packages/myapp/v1.0.0.tar.gz ./myapp.tar.gz
```

#### files delete

Delete a file from a namespace.

```bash
kscore-files files delete <namespace>/<path> [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--force` | Delete without confirmation | `false` |
| `--dry-run` | Show what would be deleted | `false` |

#### files info

Show detailed file information.

```bash
kscore-files files info <namespace>/<path> [flags]
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
kscore-bootstrap seed --config bootstrap.yaml --output-dir /etc/kscore

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
  --output-dir /etc/kscore \
  --cluster-name production \
  --trust-domain example.com

# Seed with specific NATS configuration
kscore-bootstrap seed \
  --config bootstrap.yaml \
  --nats-mode embedded \
  --output-dir /etc/kscore

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
  --output-dir /etc/kscore

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
  --output-dir /etc/kscore

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

Validate cluster configuration:

```bash
# Validate configuration
kscore-bootstrap validate --config /etc/kscore/server.yaml

# Validate with connectivity checks
kscore-bootstrap validate \
  --config /etc/kscore/server.yaml \
  --check-connectivity

# Validate entire cluster directory
kscore-bootstrap validate --config-dir /etc/kscore
```

**Validate Flags**:
```
--config string        Single config file to validate
--config-dir string    Directory containing configs
--check-connectivity   Test network connectivity
--strict               Fail on warnings
```

### Status Command

Check cluster bootstrap status:

```bash
# Show bootstrap status
kscore-bootstrap status

# Show detailed status
kscore-bootstrap status --verbose

# Output as JSON
kscore-bootstrap status --output json
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

## kscore-telemetry-gateway (Telemetry Aggregation Gateway)

Aggregates metrics, logs, and traces from agents over NATS and exposes them to standard observability backends (Prometheus, Loki, Tempo/Jaeger).

### Commands

#### serve

Start the telemetry gateway server.

```bash
# Start with default settings
kscore-telemetry-gateway serve

# Start with custom config file
kscore-telemetry-gateway serve --config /etc/kscore/gateway.yaml

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
# /etc/kscore/gateway.yaml

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
kscore-telemetry-gateway serve --config /etc/kscore/gateway.yaml

# Instance 2 (same config)
kscore-telemetry-gateway serve --config /etc/kscore/gateway.yaml
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
- `--schedule string`: Cron schedule expression (default: "0 6 * * *")
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
- `--type string`: Filter by event type (e.g., agent.registered, state.applied)
- `--source string`: Filter by event source
- `--severity string`: Filter by severity (info, warning, error, critical)
- `--since string`: Show events since time (RFC3339 or duration like 1h, 24h)
- `--until string`: Show events until time
- `--correlation-id string`: Filter by correlation ID
- `-n, --limit int`: Maximum events to show (default: 100)

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
- `--type string`: Filter by event type pattern
- `--severity string`: Minimum severity to show
- `--source string`: Filter by source
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

#### events retention show

Show current retention configuration.

```bash
kscorectl events retention show
```

#### events retention set

Update retention settings.

```bash
kscorectl events retention set [flags]
```

**Flags**:
- `--max-age string`: Maximum event age (e.g., 7d, 30d)
- `--max-count int`: Maximum number of events to retain
- `--type string`: Apply settings to specific event type

**Examples**:
```bash
# Set global retention to 30 days
kscorectl events retention set --max-age 30d

# Set retention for error events to 90 days
kscorectl events retention set --type "*.error" --max-age 90d

# Set maximum event count
kscorectl events retention set --max-count 1000000
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
- `--cron string`: Cron expression (e.g., "0 2 * * *")
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

### maintenance list

List maintenance windows.

```bash
kscorectl maintenance list [flags]
```

**Flags**:
- `--status string`: Filter by status (scheduled, active, completed, cancelled)
- `--type string`: Filter by type (planned, emergency, recurring)
- `--label strings`: Filter by label (key:value format)
- `--limit int`: Maximum windows to show (default: 50)

**Examples**:
```bash
# List all maintenance windows
kscorectl maintenance list

# List only active windows
kscorectl maintenance list --status active

# List emergency windows
kscorectl maintenance list --type emergency
```

### maintenance show

Show maintenance window details.

```bash
kscorectl maintenance show <window-id>
```

### maintenance create

Create a new maintenance window.

```bash
kscorectl maintenance create [flags]
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
kscorectl maintenance create --name "weekly-patching" \
  --start "2024-01-15T02:00:00Z" --end "2024-01-15T06:00:00Z" \
  --scope-tags env:prod --suppress-alerts

# Create an emergency maintenance window
kscorectl maintenance create --name "urgent-fix" --type emergency \
  --start now --end "2024-01-15T04:00:00Z" --scope-all

# Create with approval requirement
kscorectl maintenance create --name "db-migration" \
  --start "2024-01-20T00:00:00Z" --end "2024-01-20T04:00:00Z" \
  --require-approval --scope-agents db-01,db-02
```

### maintenance start

Start a scheduled maintenance window.

```bash
kscorectl maintenance start <window-id>
```

### maintenance end

End an active maintenance window.

```bash
kscorectl maintenance end <window-id>
```

### maintenance cancel

Cancel a maintenance window.

```bash
kscorectl maintenance cancel <window-id> [flags]
```

**Flags**:
- `--reason string`: Cancellation reason

### maintenance extend

Extend a maintenance window.

```bash
kscorectl maintenance extend <window-id> [flags]
```

**Flags**:
- `--end string`: New end time (RFC3339 format)
- `--duration string`: Extend by duration (e.g., 1h, 30m)

**Examples**:
```bash
# Extend to new end time
kscorectl maintenance extend maint-001 --end "2024-01-15T08:00:00Z"

# Extend by 2 hours
kscorectl maintenance extend maint-001 --duration 2h
```

### maintenance active

List currently active maintenance windows.

```bash
kscorectl maintenance active
```

### maintenance upcoming

List upcoming maintenance windows.

```bash
kscorectl maintenance upcoming [flags]
```

**Flags**:
- `--within string`: Show windows starting within duration (default: 24h)

### maintenance conflicts

Check for conflicts with other windows.

```bash
kscorectl maintenance conflicts <window-id>
```

### maintenance delete

Delete a maintenance window.

```bash
kscorectl maintenance delete <window-id> [flags]
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
- `--include-prerelease`: Include prerelease versions
- `--channel string`: Release channel: stable, beta, edge (default: stable)

**Examples**:
```bash
# Check for available upgrades
kscorectl upgrade check

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
- `--confirm`: Confirm execution without prompting
- `--async`: Run upgrade asynchronously

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
kscorectl upgrade status [upgrade-id]
```

**Examples**:
```bash
# Show current upgrade status
kscorectl upgrade status

# Show specific upgrade status
kscorectl upgrade status upgrade-20240115-120000
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

Manage agent upgrades.

#### upgrade agents list

List agents and their versions.

```bash
kscorectl upgrade agents list [flags]
```

**Flags**:
- `--version string`: Filter by version
- `--outdated`: Show only outdated agents
- `--target string`: Target filter expression

#### upgrade agents status

Show agent upgrade status.

```bash
kscorectl upgrade agents status <agent-id>
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

### upgrade logs

Show upgrade logs.

```bash
kscorectl upgrade logs [upgrade-id] [flags]
```

**Flags**:
- `--follow`: Follow log output
- `--tail int`: Number of lines to show (default: 100)

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
- `--protocol string`: Filter by protocol (ssh, snmp, rest, winrm)
- `--vendor string`: Filter by vendor
- `--status string`: Filter by status (healthy, degraded, unhealthy)
- `--profile string`: Filter by device profile
- `--proxy-agent string`: Filter by proxy agent
- `--label strings`: Filter by labels (key:value)
- `-n, --limit int`: Maximum devices to show (default: 50)

**Examples**:
```bash
# List all devices
kscorectl proxy device list

# List network devices
kscorectl proxy device list --vendor cisco

# List devices by protocol
kscorectl proxy device list --protocol ssh

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
- `--protocol string`: Protocol: ssh, snmp, rest, winrm (required)
- `--vendor string`: Device vendor
- `--device-type string`: Device type (router, switch, firewall, server, etc.)
- `--profile string`: Device profile to use
- `--credential string`: Credential set to use
- `--proxy-agent string`: Proxy agent to handle this device
- `--label strings`: Labels (key:value format)
- `--port int`: Connection port (protocol default if not specified)

**Examples**:
```bash
# Add a Cisco router
kscorectl proxy device add --name core-router-01 \
  --address 192.168.1.1 --protocol ssh \
  --vendor cisco --device-type router \
  --credential cisco-ssh --profile cisco-ios

# Add a Windows server via WinRM
kscorectl proxy device add --name legacy-server-01 \
  --address 10.0.0.50 --protocol winrm \
  --credential win-admin --device-type server

# Add SNMP-monitored device
kscorectl proxy device add --name ups-01 \
  --address 192.168.1.100 --protocol snmp \
  --credential snmp-v3 --device-type ups
```

#### proxy device remove

Remove a managed device.

```bash
kscorectl proxy device remove <device-id> [flags]
```

**Flags**:
- `-f, --force`: Force removal without confirmation

#### proxy device test

Test connectivity to a device.

```bash
kscorectl proxy device test <device-id>
```

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

#### proxy credential remove

Remove a credential set.

```bash
kscorectl proxy credential remove <credential-name> [flags]
```

**Flags**:
- `-f, --force`: Force removal

#### proxy credential update

Update a credential set.

```bash
kscorectl proxy credential update <credential-name> [flags]
```

### proxy discover

Discover devices on the network.

#### proxy discover scan

Scan for devices.

```bash
kscorectl proxy discover scan [flags]
```

**Flags**:
- `--subnet string`: Subnet to scan (CIDR notation)
- `--protocols strings`: Protocols to probe (ssh, snmp, rest, winrm)
- `--ports strings`: Ports to scan
- `--timeout string`: Scan timeout (default: 5s)
- `--workers int`: Number of parallel workers (default: 20)

**Examples**:
```bash
# Scan subnet for SSH and SNMP devices
kscorectl proxy discover scan --subnet 192.168.1.0/24 --protocols ssh,snmp

# Scan with custom ports
kscorectl proxy discover scan --subnet 10.0.0.0/24 --ports 22,161,443
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

### proxy drift

Detect and report configuration drift on proxied devices.

#### proxy drift check

Check for drift on devices.

```bash
kscorectl proxy drift check [flags]
```

**Flags**:
- `--device string`: Check specific device
- `--profile string`: Check devices with profile
- `--severity string`: Minimum severity to report

**Examples**:
```bash
# Check all devices for drift
kscorectl proxy drift check

# Check specific device
kscorectl proxy drift check --device router-01

# Check with severity filter
kscorectl proxy drift check --severity high
```

#### proxy drift show

Show drift details for a device.

```bash
kscorectl proxy drift show <device-id>
```

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

## Command Migration Guide

This section documents the CLI command restructuring in version 0.4.0 and provides migration guidance for scripts and automation.

### Overview

In version 0.4.0, the monolithic `kscorectl` command was split into focused plugins for better maintainability and clearer responsibility boundaries. This guide helps you update existing scripts and workflows.

### Command Mapping Table

#### Agent Commands

| Old Command (< 0.4.0) | New Command (≥ 0.4.0) | Notes |
|----------------------|----------------------|-------|
| `kscorectl agent list` | `kscorectl agent list` | Unchanged |
| `kscorectl agent show <id>` | `kscorectl agent show <id>` | Unchanged |
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
| `kscorectl highstate` | `kscorectl state apply --all` | Deprecated term |
| `kscorectl state.apply` | `kscorectl state apply` | Salt-style removed |

#### Execution Commands

| Old Command (< 0.4.0) | New Command (≥ 0.4.0) | Notes |
|----------------------|----------------------|-------|
| `kscorectl run <cmd>` | `kscorectl exec run <cmd>` | Moved to exec plugin |
| `kscorectl cmd.run <cmd>` | `kscorectl exec run <cmd>` | Salt-style removed |
| `kscorectl shell` | `kscorectl exec shell` | Moved to exec plugin |
| `kscorectl script <file>` | `kscorectl exec script <file>` | Moved to exec plugin |
| `kscorectl batch <file>` | `kscorectl exec batch <file>` | Moved to exec plugin |

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
| `kscorectl policy-eval` | `kscorectl policy evaluate` | Renamed |
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
| `kscorectl gitops-sync` | `kscorectl gitops sync` | Subcommand structure |
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
| `kscorectl highstate` | `kscorectl state apply --all` | 0.6.0 |
| `kscorectl pillar.get` | `kscorectl vars get` | 0.6.0 |
| `kscorectl grains.items` | `kscorectl facts list` | 0.6.0 |

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
kscorectl apply /etc/kscore/states/nginx.yaml

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
kscorectl state apply /etc/kscore/states/nginx.yaml

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
sh 'kscorectl policy report --output html > compliance.html'
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

For gradual migration, enable compatibility mode:

```yaml
# ~/.kscore/config.yaml
cli:
  compatibility_mode: true  # Accept both old and new commands
  deprecation_warnings: true  # Show warnings for deprecated usage
```

With compatibility mode:
- Old commands still work but show deprecation warnings
- Helps identify scripts that need updating
- Disable before 0.6.0 release

### Migration Checklist

- [ ] Review all scripts using `kscore-migrate scan-scripts`
- [ ] Update CI/CD pipeline configurations
- [ ] Update automation playbooks
- [ ] Regenerate shell completions
- [ ] Update monitoring/alerting that parses CLI output
- [ ] Update documentation referencing CLI commands
- [ ] Test all integrations in staging environment
- [ ] Remove compatibility mode after migration

### Getting Help

If you encounter issues during migration:

```bash
# Check command mapping
kscorectl help migrate

# Show deprecation warnings interactively
kscorectl --show-deprecated

# Get help for new command structure
kscorectl exec --help
kscorectl state --help
kscorectl policy --help
```

## See Also

- [API Reference](../api/) - REST/gRPC API
- [Configuration Reference](../configuration/) - Configuration options
- [Getting Started](../../getting-started/quick-start/) - Quick start guide
- [File Distribution Concepts](../../concepts/file-distribution/) - File distribution overview
- [Observability Gateway Operations](../../operations/gateway/) - Gateway deployment guide
