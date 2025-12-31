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
2. Queries registry for available versions
3. Resolves version constraints using MVS (Minimum Version Selection) algorithm
4. Detects circular dependencies
5. Generates module.lock with pinned versions and hashes

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
- `--registry string`: Registry URL (default: KSCORE_REGISTRY env or https://registry.keystone-core.io)
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

Registry: https://registry.keystone-core.io

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

Registry: https://registry.keystone-core.io

Publishing module... done

=== Published ===
Module: myorg/my-module@1.0.0
Hash: sha256:f28f3dbe8066d05fff31e9ef18f7655b...
Size: 1.3 KB
URL: https://registry.keystone-core.io/myorg/my-module/@v/1.0.0.zip
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

**Note**: Publishing requires a running module registry. The default registry (registry.keystone-core.io) is a placeholder - use `--registry` to specify your own registry instance.

### module install

Install modules from a registry.

```bash
kscorectl module install <module[@version]> [modules...] [flags]
```

**Flags**:
- `--registry string`: Registry URL (defaults to KSCORE_REGISTRY env var or https://registry.keystone-core.io)
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
Registry: https://registry.keystone-core.io
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

**Note**: Installing requires a running module registry. The default registry (registry.keystone-core.io) is a placeholder - use `--registry` to specify your own registry instance.

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
KSCORE_SERVER="http://control-plane:8080"
KSCORE_API_KEY="ta_live_abc123"
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
