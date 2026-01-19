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
| `kscore-module` | Plugin | Module management and development |
| `kscore-policy` | Plugin | Policy evaluation and compliance |
| `kscore-gitops` | Plugin | GitOps integration and verification |
| `kscore-cluster` | Plugin | Cluster management and HA |
| `kscore-identity` | Plugin | SPIFFE identity management |
| `kscore-migrate` | Plugin | Database migration tool |
| `kscore-registry` | Server | Module registry HTTP server |
| `kscore-files` | Server | File distribution server |
| `kscore-bootstrap` | Tool | Cluster bootstrapping and recovery |
| `kscore-telemetry-gateway` | Server | Telemetry aggregation gateway |
| `kscore-agent` | Daemon | Agent daemon on managed nodes |
| `kscore-server` | Daemon | Control plane server |

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

```bash
kscorectl module install <module[@version]> [modules...] [flags]
```

**Flags**:
- `--registry string`: Registry URL (defaults to KSCORE_REGISTRY env var or https://registry.keystonecore.io)
- `--token string`: Authentication token (can also use KSCORE_REGISTRY_TOKEN)
- `--username string`: Username for basic auth (can also use KSCORE_REGISTRY_USERNAME)
- `--password string`: Password for basic auth (can also use KSCORE_REGISTRY_PASSWORD)
- `--cache-dir string`: Module cache directory (default: ~/.kscore/modules)
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
- `--agent-id string`: Agent identifier for this token
- `--ttl string`: Token time-to-live (default: 5m)
- `--label key=value`: Labels to attach to agents using this token (can be repeated)

**Examples**:
```bash
# Create a token for a specific agent
kscorectl identity token create --agent-id web-server-1 --ttl 10m

# Create a token with labels
kscorectl identity token create --agent-id db-server-1 --ttl 1h \
  --label environment=production --label role=database
```

**Output**:
```
Token created successfully!
Token:    Rj2k9xLm3n4o5p6q7r8s9t0u1v2w3x4y5z
Agent ID: web-server-1
TTL:      10m

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
TOKEN ID           AGENT ID         EXPIRES                  STATUS
abc123def456...    web-server-1     2024-01-15T12:00:00Z     valid
ghi789jkl012...    db-server-1      2024-01-15T11:00:00Z     used
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
Agent ID:    web-server-1
Created:     2024-01-15T10:00:00Z
Expires:     2024-01-15T12:00:00Z
Status:      valid
Labels:
  environment: production
  role: web
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
- `--backup string`: Backup file to restore (required)

**Example**:
```bash
kscorectl identity ca restore --backup /var/backups/ca-backup.json
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
- `--bundle-endpoint string`: Bundle endpoint URL (required)
- `--type string`: Federation type: bidirectional, unidirectional (default: bidirectional)
- `--refresh-interval string`: Bundle refresh interval (default: 5m)

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
Federation relationship added: partner.example.org
Bundle Endpoint: https://partner.example.org/.well-known/spiffe-bundle
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
- `-f, --follow`: Follow events in real-time

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
kscorectl identity ca restore --backup /var/backups/ca-20240115.json
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
- `--cors`: Enable CORS headers for web clients

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
kscore-registry --cors
```

**Output**:
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
| GET | `/` | Server information |

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

### Running the Server

```bash
# Run with configuration file
kscore-files serve --config /etc/kscore/files.yaml

# Run with NATS connection
kscore-files serve --nats-url nats://localhost:4222

# Show version
kscore-files version
```

### Server Flags

```
--config string       Configuration file path
--nats-url string     NATS server URL (default: nats://localhost:4222)
--cluster-id string   Cluster identifier for routing
--instance-id string  Instance identifier for HA deployments
-h, --help            Show help
```

### File Management Commands

```bash
# List files in a namespace
kscore-files files list <namespace>

# Upload a file
kscore-files files put <local-path> <namespace>/<remote-path>

# Download a file
kscore-files files get <namespace>/<remote-path> <local-path>

# Delete a file
kscore-files files delete <namespace>/<path>

# Show file info
kscore-files files info <namespace>/<path>
```

### Backend Commands

```bash
# Show backend status
kscore-files backend status

# Run garbage collection
kscore-files backend gc

# Verify backend integrity
kscore-files backend verify
```

### Cache Commands

```bash
# Show cache status
kscore-files cache status

# Clear cache
kscore-files cache clear

# Warm cache for namespace
kscore-files cache warm <namespace>
```

### Namespace Commands

```bash
# List namespaces
kscore-files namespace list

# Create namespace
kscore-files namespace create <name> --description "Description"

# Delete namespace
kscore-files namespace delete <name>

# Manage namespace ACLs
kscore-files namespace acl add <name> --principal role:ops --permission read,write
kscore-files namespace acl remove <name> --principal role:ops
kscore-files namespace acl list <name>
```

### Mirror Commands

```bash
# List mirror groups
kscore-files mirrors list

# Show mirror group status
kscore-files mirrors show <group-id>

# Check mirror health
kscore-files mirrors health --group <group-id>

# Trigger sync
kscore-files mirrors sync --group <group-id>

# View sync status
kscore-files mirrors sync-status --group <group-id>

# Trigger failover
kscore-files mirrors failover <group-id> --from <mirror-id>

# List conflicts
kscore-files mirrors conflicts --group <group-id>

# Resolve conflict
kscore-files mirrors resolve-conflict <conflict-id> --strategy source
```

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

### Running the Gateway

```bash
# Run with configuration file
kscore-telemetry-gateway --config /etc/kscore/gateway.yaml

# Run with command-line options
kscore-telemetry-gateway \
  --listen 0.0.0.0:9091 \
  --nats-url nats://localhost:4222 \
  --metrics \
  --logs \
  --traces

# Show version
kscore-telemetry-gateway --version
```

### Gateway Flags

```
--config string     Path to configuration file
--listen string     Listen address (default: 0.0.0.0:9091)
--nats-url string   NATS server URL (default: nats://localhost:4222)
--metrics           Enable metrics gateway (default: true)
--logs              Enable logs gateway (default: true)
--traces            Enable traces gateway (default: true)
--version           Show version information
-h, --help          Show help
```

### Endpoints

When running, the gateway exposes the following endpoints:

| Endpoint | Description |
|----------|-------------|
| `/metrics` | Prometheus metrics scrape endpoint |
| `/federate` | Prometheus federation endpoint |
| `/health` | Health check endpoint |
| `/ready` | Readiness check endpoint |

### Metrics Gateway

The metrics gateway:
- Subscribes to `kscore.metrics.>` on NATS
- Aggregates metrics from all agents
- Exposes `/metrics` for Prometheus scraping
- Exposes `/federate` for Prometheus federation
- Supports label transformations

**Prometheus Configuration**:
```yaml
scrape_configs:
  - job_name: 'kscore-gateway'
    static_configs:
      - targets: ['gateway:9091']
```

### Logs Gateway

The logs gateway:
- Subscribes to `kscore.logs.>` on NATS
- Buffers and batches logs
- Pushes to Loki via push API
- Supports log level filtering
- Multi-tenant support via X-Scope-OrgID

### Traces Gateway

The traces gateway:
- Subscribes to `kscore.traces.>` on NATS
- Groups spans into traces
- Exports via OTLP to Tempo/Jaeger
- Supports sampling configuration
- Prioritizes error and slow traces

### Configuration Example

```yaml
# /etc/kscore/gateway.yaml
server:
  listen: "0.0.0.0:9091"

nats:
  url: "nats://localhost:4222"
  subject_prefix: "kscore"

metrics:
  enabled: true
  path: "/metrics"
  federation_path: "/federate"
  retention: "15m"

logs:
  enabled: true
  loki_url: "http://loki:3100"
  batch_size: 1000
  flush_interval: "5s"

traces:
  enabled: true
  otlp_endpoint: "tempo:4317"
  sampling_rate: 1.0

ha:
  enabled: false
  instance_id: "gateway-1"
  shard_count: 3
```

### High Availability

For HA deployments, run multiple gateway instances:

```bash
# Instance 1
kscore-telemetry-gateway \
  --config /etc/kscore/gateway.yaml \
  --instance-id gateway-1

# Instance 2
kscore-telemetry-gateway \
  --config /etc/kscore/gateway.yaml \
  --instance-id gateway-2
```

Agents are distributed across instances using consistent hashing.

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
