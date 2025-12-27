---
title: "State Management"
weight: 6
description: >
  Declarative configuration management with idempotent state modules and drift detection
---

## Overview

TitanAnvil's state management system enables you to describe your infrastructure's desired state declaratively. The system ensures your infrastructure matches that state through idempotent operations.

**Key Principles**:
- **Declarative**: Describe what you want, not how to achieve it
- **Idempotent**: Safe to run repeatedly with same result
- **Dependency-Aware**: Automatic ordering based on requisites
- **Drift-Detecting**: Identifies configuration drift automatically
- **Template-Driven**: Dynamic configuration with vars and facts

## State Files

State files are YAML documents that declare desired resource states:

```yaml
# Example: web-server.yaml
nginx_package:
  module: package
  state: installed
  name: nginx

nginx_config:
  module: file
  state: present
  path: /etc/nginx/nginx.conf
  source: salt://nginx/nginx.conf
  mode: "0644"
  require:
    - nginx_package

nginx_service:
  module: service
  state: running
  name: nginx
  enabled: true
  watch:
    - nginx_config
```

### State Declaration Structure

Each state declaration has:

**State ID** (e.g., `nginx_package`):
- Unique identifier
- Used for requisite references
- Meaningful name for readability

**Module**: Which state module to use (file, package, service, etc.)

**State**: Desired state (installed, present, running, etc.)

**Parameters**: Module-specific configuration

**Requisites**: Dependencies and relationships

## State Modules

TitanAnvil includes 6 built-in modules:

### 1. File Module

Manages files and directories:

**States**:
- `present`: Ensure file exists with specified content
- `absent`: Ensure file does not exist
- `directory`: Ensure directory exists
- `symlink`: Ensure symlink exists

**Example**:
```yaml
app_config:
  module: file
  state: present
  path: /etc/app/config.yaml
  contents: |
    database:
      host: {{ vars.db_host }}
      port: 5432
  mode: "0600"
  owner: app
  group: app
```

**Parameters**:
- `path`: File path (required)
- `contents`: File contents (for `present`)
- `source`: Source file (alternative to `contents`)
- `mode`: Permission mode (e.g., "0644")
- `owner`: File owner
- `group`: File group

### 2. Package Module

Manages software packages:

**States**:
- `installed`: Ensure package is installed
- `removed`: Ensure package is not installed
- `latest`: Ensure latest version is installed
- `purged`: Remove package and config files

**Example**:
```yaml
docker:
  module: package
  state: installed
  name: docker-ce
  version: "20.10.*"
```

**Cross-Platform Support**:
- Linux: apt, yum, dnf, zypper, pacman, apk
- macOS: homebrew
- Windows: chocolatey, winget

**Parameters**:
- `name`: Package name (required)
- `version`: Specific version (optional)
- `repo`: Custom repository (optional)

### 3. Service Module

Manages system services:

**States**:
- `running`: Ensure service is running
- `stopped`: Ensure service is stopped
- `enabled`: Enable service on boot
- `disabled`: Disable service on boot

**Example**:
```yaml
postgresql:
  module: service
  state: running
  name: postgresql
  enabled: true
  reload: true  # Reload instead of restart on changes
```

**Cross-Platform Support**:
- Linux: systemd, upstart, sysvinit, openrc
- macOS: launchd
- Windows: Windows Service Manager

**Parameters**:
- `name`: Service name (required)
- `enabled`: Enable on boot (boolean)
- `reload`: Reload instead of restart (boolean)

### 4. User Module

Manages user accounts:

**States**:
- `present`: Ensure user exists
- `absent`: Ensure user does not exist

**Example**:
```yaml
appuser:
  module: user
  state: present
  name: myapp
  uid: 1001
  gid: 1001
  home: /home/myapp
  shell: /bin/bash
  groups:
    - docker
    - sudo
```

**Parameters**:
- `name`: Username (required)
- `uid`: User ID
- `gid`: Primary group ID
- `home`: Home directory
- `shell`: Login shell
- `groups`: Additional groups

### 5. Group Module

Manages groups:

**States**:
- `present`: Ensure group exists
- `absent`: Ensure group does not exist

**Example**:
```yaml
developers:
  module: group
  state: present
  name: developers
  gid: 2000
```

**Parameters**:
- `name`: Group name (required)
- `gid`: Group ID

### 6. Command Module

Executes commands:

**States**:
- `run`: Run command unconditionally
- `wait`: Run only when watched resource changes

**Example**:
```yaml
reload_app:
  module: cmd
  state: wait
  command: "systemctl reload myapp"
  watch:
    - app_config

database_migration:
  module: cmd
  state: run
  command: "/usr/local/bin/migrate up"
  unless: "test -f /var/lib/app/migrated"
```

**Parameters**:
- `command`: Command to run (required)
- `cwd`: Working directory
- `env`: Environment variables
- `timeout`: Execution timeout
- `unless`: Skip if this command succeeds
- `only_if`: Run only if this command succeeds

## Requisites (Dependencies)

Requisites define relationships between state declarations:

### require

Execute after another state succeeds:

```yaml
nginx_config:
  module: file
  state: present
  path: /etc/nginx/nginx.conf
  require:
    - nginx_package  # Must run after nginx_package succeeds
```

### require_in

Inverse of `require` - make another state depend on this one:

```yaml
nginx_package:
  module: package
  state: installed
  name: nginx
  require_in:
    - nginx_service  # nginx_service will require nginx_package
```

### watch

Execute after another state changes:

```yaml
nginx_service:
  module: service
  state: running
  name: nginx
  watch:
    - nginx_config  # Restart when nginx_config changes
```

### watch_in

Inverse of `watch`:

```yaml
nginx_config:
  module: file
  state: present
  path: /etc/nginx/nginx.conf
  watch_in:
    - nginx_service  # Notify nginx_service when this changes
```

### prereq

Must succeed before another state runs (for ordering):

```yaml
database_schema:
  module: cmd
  state: run
  command: "psql < schema.sql"
  prereq:
    - database_data  # database_data will run after this
```

### onchanges

Run only when watched state changes:

```yaml
clear_cache:
  module: cmd
  state: run
  command: "rm -rf /var/cache/app/*"
  onchanges:
    - app_code  # Only run when app_code changes
```

## Dependency Resolution

TitanAnvil builds a dependency graph and topologically sorts state declarations:

### Execution Order

```
┌──────────────┐
│   Package    │ ← No dependencies, runs first
└──────┬───────┘
       │
       ↓
┌──────────────┐
│     File     │ ← Requires package
└──────┬───────┘
       │
       ↓
┌──────────────┐
│   Service    │ ← Requires file, watches file
└──────────────┘
```

### Parallel Execution

States without dependencies can run in parallel:

```
┌──────────────┐     ┌──────────────┐
│   Package A  │     │   Package B  │  ← Run in parallel
└──────┬───────┘     └──────┬───────┘
       │                     │
       └──────────┬──────────┘
                  ↓
          ┌──────────────┐
          │   Service    │  ← Waits for both
          └──────────────┘
```

### Circular Dependencies

Detected and rejected:

```yaml
state_a:
  require:
    - state_b

state_b:
  require:
    - state_a  # ERROR: Circular dependency detected
```

## Templating

State files support Go template syntax for dynamic configuration:

### Variables

Define variables in separate files or inline:

**vars.yaml**:
```yaml
app_name: myapp
db_host: postgres.example.com
db_port: 5432
replicas: 3
```

**State file with variables**:
```yaml
app_config:
  module: file
  state: present
  path: /etc/{{ .vars.app_name }}/config.yaml
  contents: |
    database:
      host: {{ .vars.db_host }}
      port: {{ .vars.db_port }}
    replicas: {{ .vars.replicas }}
```

**Apply with variables**:
```bash
titanctl state apply web-server.yaml --vars vars.yaml
```

### Facts

Facts are system-discovered metadata:

**Available facts**:
- `{{ .facts.os }}` - Operating system (linux, windows, darwin)
- `{{ .facts.arch }}` - Architecture (amd64, arm64)
- `{{ .facts.hostname }}` - Hostname
- `{{ .facts.ip }}` - Primary IP address
- `{{ .facts.cpu_count }}` - CPU count
- `{{ .facts.memory_total }}` - Total memory

**Example**:
```yaml
app_config:
  module: file
  state: present
  path: /etc/app/config.yaml
  contents: |
    hostname: {{ .facts.hostname }}
    cpu_workers: {{ .facts.cpu_count }}
    {{- if eq .facts.os "linux" }}
    platform: linux
    {{- else if eq .facts.os "darwin" }}
    platform: mac
    {{- end }}
```

### Template Functions

Built-in functions:

```yaml
example:
  module: file
  state: present
  path: /tmp/example.txt
  contents: |
    # String functions
    uppercase: {{ upper "hello" }}
    lowercase: {{ lower "HELLO" }}
    title: {{ title "hello world" }}

    # List functions
    joined: {{ join .vars.list "," }}

    # Conditionals
    {{- if .vars.enabled }}
    feature: enabled
    {{- else }}
    feature: disabled
    {{- end }}

    # Default values
    setting: {{ default "default-value" .vars.setting }}
```

## Drift Detection

TitanAnvil automatically detects when actual state differs from desired state:

### How It Works

1. **Check Current State**: Query actual resource state
2. **Compare**: Diff against desired state
3. **Calculate Severity**: Assign drift severity level
4. **Report**: Generate drift report
5. **Emit Event**: Publish `state.drift` event

### Severity Levels

- **None**: No drift detected
- **Low**: Minor differences (comments, whitespace)
- **Medium**: Significant but non-critical (permissions)
- **High**: Critical configuration (service stopped)
- **Critical**: Security issues (wrong owner, world-writable)

### Example Drift Report

```
Drift detected on agent: web-01

nginx_config:
  ✗ drift detected (MEDIUM severity)
  - mode: expected "0644", got "0755"
  - owner: expected "root", got "nginx"

nginx_service:
  ✗ drift detected (HIGH severity)
  - state: expected "running", got "stopped"

Summary:
  Total: 10 states
  Compliant: 8
  Drift: 2 (1 medium, 1 high)
```

### Drift Remediation

**Manual**:
```bash
# Check for drift
titanctl state check web-server.yaml --target "role:web"

# Fix drift
titanctl state apply web-server.yaml --target "role:web"
```

**Automatic** (via reactors):
```yaml
auto_remediate_drift:
  filter: "type == 'state.drift' and severity >= 'high'"
  actions:
    - type: state_apply
      state_file: "{{ event.data.state_file }}"
      target: "agent_id == {{ event.source }}"
```

## State Application Workflow

```
1. Parse state file
   ↓
2. Render templates (vars/facts)
   ↓
3. Validate module parameters
   ↓
4. Build dependency graph (DAG)
   ↓
5. Topological sort
   ↓
6. Send to target agents
   ↓
7. Agents execute modules (idempotent)
   ↓
8. Collect results
   ↓
9. Detect drift
   ↓
10. Emit events (state.change, state.drift)
   ↓
11. Return summary
```

## Idempotency

All state modules are idempotent - safe to run repeatedly:

**File module**:
- Check if file exists with correct content
- Only write if changed
- Only change permissions if needed

**Package module**:
- Check if package already installed
- Check version matches
- Only install/upgrade if needed

**Service module**:
- Check if service already in desired state
- Only start/stop/reload if needed

**Example**:
```yaml
nginx_package:
  module: package
  state: installed
  name: nginx
```

First run:
```
nginx_package: ✓ installed (changed)
```

Second run (nginx already installed):
```
nginx_package: ✓ installed (unchanged)
```

## Best Practices

### Organization

1. **Separate Concerns**: One state file per service/component
2. **Use Includes**: Compose complex states from smaller files
3. **Version Control**: Keep state files in Git
4. **Environment-Specific**: Use vars for environment differences

```
states/
├── base/
│   ├── users.yaml
│   └── packages.yaml
├── web/
│   ├── nginx.yaml
│   └── app.yaml
└── db/
    └── postgres.yaml

vars/
├── dev.yaml
├── staging.yaml
└── prod.yaml
```

### Naming

1. **Descriptive IDs**: `nginx_config` not `config1`
2. **Consistent Naming**: Follow a naming convention
3. **Group Related**: Prefix with component name

### Dependencies

1. **Explicit Dependencies**: Always declare requisites
2. **Avoid Circular**: Design to prevent circular deps
3. **Use `watch` for Triggers**: Restart services on config changes

### Templates

1. **Validate Variables**: Check var existence with `default`
2. **Comment Templates**: Document template logic
3. **Test Rendering**: Test with all var combinations

### Testing

1. **Dry Run**: Test with `titanctl state check` first
2. **Dev Environment**: Test on dev before prod
3. **Version Control**: Commit and review state changes
4. **Rollback Plan**: Keep previous versions for rollback

## Troubleshooting

### State Won't Apply

**Problem**: State application fails

Debug:
```bash
# Detailed output
titanctl state apply web.yaml --target "role:web" --verbose

# Dry run
titanctl state check web.yaml --target "role:web"
```

Common issues:
- Syntax errors in YAML
- Invalid module parameters
- Unresolved requisites
- Permission issues on agents

### Circular Dependency

**Problem**: "Circular dependency detected"

Fix:
- Review requisite chains
- Remove unnecessary dependencies
- Use `watch` instead of `require` if applicable
- Reorganize state declarations

### Template Rendering Fails

**Problem**: "Template rendering error"

Debug:
```bash
# Check template syntax
titanctl state render web.yaml --vars dev.yaml
```

Common issues:
- Missing variables
- Incorrect function syntax
- Undefined facts

### Drift Not Detected

**Problem**: Known drift not showing in reports

Check:
- Drift severity threshold
- Module's drift detection logic
- Agent connectivity

## Performance

### Optimization

1. **Batch Operations**: Apply to multiple agents in parallel
2. **Reduce Modules**: Combine related operations
3. **Cache Results**: Use agent-side caching
4. **Limit Concurrency**: Don't overwhelm agents

### Benchmarks

State application performance (single agent):

- Simple file: ~10ms
- Package install: ~500ms-5s (depends on package)
- Service restart: ~100-500ms
- Full state run (10 modules): ~2-5s

Scaling (100 agents, 10 modules each):
- Sequential: ~500s
- Parallel (batch=10): ~50s
- Parallel (batch=50): ~10s

## Next Steps

- Learn about [Remote Execution](remote-execution/) for command-based operations
- Understand [Events](events/) emitted during state changes
- Explore [Reactors](reactors/) for automated drift remediation
- See [Policy Enforcement](policy/) for compliance checks on state
