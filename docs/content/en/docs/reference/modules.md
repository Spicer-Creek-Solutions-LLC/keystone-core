---
title: "Module Reference"
weight: 4
description: >
  Complete reference for all state modules with parameters and specifications
---

## Overview

Keystone Core includes 6 built-in state modules for declarative configuration management. All modules are idempotent and cross-platform where applicable.

**Modules**:
- [file](#file-module) - Manage files and directories
- [package](#package-module) - Manage software packages
- [service](#service-module) - Manage system services
- [user](#user-module) - Manage user accounts
- [group](#group-module) - Manage groups
- [cmd](#cmd-module) - Execute commands

## Module Structure

Every state declaration follows this structure:

```yaml
state_id:
  module: <module_name>     # Module to use
  state: <state>            # Desired state
  # Module-specific parameters
  # Requisites (optional)
```

## File Module

Manage files, directories, and symlinks.

### States

- `present` - Ensure file exists with specified content
- `absent` - Ensure file does not exist
- `directory` - Ensure directory exists
- `symlink` - Ensure symlink exists

### Parameters

#### Common Parameters

**path** (string, required)
- File path
- Example: `/etc/nginx/nginx.conf`

**owner** (string, optional)
- File owner
- Example: `root`, `nginx`

**group** (string, optional)
- File group
- Example: `root`, `www-data`

**mode** (string, optional)
- File permissions (octal)
- Example: `"0644"`, `"0755"`
- Note: Use string format with leading zero

#### State: present

**contents** (string, optional)
- File contents
- Mutually exclusive with `source`
- Example:
  ```yaml
  contents: |
    server {
      listen 80;
    }
  ```

**source** (string, optional)
- Source file URL
- Mutually exclusive with `contents`
- Schemes: `file://`, `http://`, `https://`
- Example: `file:///etc/template.conf`

**create** (bool, optional, default: true)
- Create file if it doesn't exist
- Example: `true`

**replace** (bool, optional, default: true)
- Replace file if contents differ
- Example: `false` (don't overwrite)

**backup** (bool, optional, default: false)
- Create backup before replacing
- Backup location: `<path>.backup.<timestamp>`
- Example: `true`

#### State: directory

**makedirs** (bool, optional, default: false)
- Create parent directories
- Example: `true`

**clean** (bool, optional, default: false)
- Remove unmanaged files from directory
- Example: `false`

**recurse** (bool, optional, default: false)
- Apply permissions recursively
- Example: `true`

#### State: symlink

**target** (string, required)
- Symlink target path
- Example: `/usr/local/bin/app`

**force** (bool, optional, default: false)
- Force creation (remove existing file)
- Example: `true`

### Examples

#### File with Contents

```yaml
nginx_config:
  module: file
  state: present
  path: /etc/nginx/nginx.conf
  owner: root
  group: root
  mode: "0644"
  contents: |
    worker_processes 4;
    events {
      worker_connections 1024;
    }
```

#### File from Source

```yaml
app_config:
  module: file
  state: present
  path: /etc/app/config.yaml
  source: file:///etc/templates/app-config.yaml
  mode: "0600"
  backup: true
```

#### Directory

```yaml
log_directory:
  module: file
  state: directory
  path: /var/log/myapp
  owner: myapp
  group: myapp
  mode: "0755"
  makedirs: true
```

#### Symlink

```yaml
app_symlink:
  module: file
  state: symlink
  path: /usr/local/bin/myapp
  target: /opt/myapp/bin/myapp
```

#### Remove File

```yaml
old_config:
  module: file
  state: absent
  path: /etc/old-config.conf
```

### Return Values

```yaml
result: changed | unchanged
comment: "Description of what happened"
changes:
  old: "Previous value"
  new: "New value"
```

## Package Module

Manage software packages across platforms.

### States

- `installed` - Ensure package is installed
- `removed` - Ensure package is removed
- `latest` - Ensure latest version is installed
- `purged` - Remove package and config files

### Parameters

**name** (string, required)
- Package name
- Example: `nginx`, `docker-ce`

**version** (string, optional)
- Specific version
- Use wildcards for version ranges
- Example: `1.24.*`, `>=1.20`

**repo** (string, optional)
- Custom repository
- Platform-specific format
- Example: `ppa:nginx/stable` (Ubuntu)

**sources** (list, optional)
- Additional package sources
- Example:
  ```yaml
  sources:
    - deb https://repo.example.com/debian stable main
  ```

**update_cache** (bool, optional, default: false)
- Update package cache before install
- Example: `true`

**refresh** (bool, optional, default: false)
- Refresh package cache
- Example: `true`

### Platform Support

| Platform | Package Manager |
|----------|----------------|
| Ubuntu/Debian | apt |
| CentOS/RHEL/Fedora | yum, dnf |
| openSUSE | zypper |
| Arch Linux | pacman |
| Alpine Linux | apk |
| macOS | homebrew |
| Windows | chocolatey, winget |

### Examples

#### Install Package

```yaml
nginx:
  module: package
  state: installed
  name: nginx
```

#### Install Specific Version

```yaml
docker:
  module: package
  state: installed
  name: docker-ce
  version: "20.10.*"
  update_cache: true
```

#### Install Latest

```yaml
kubectl:
  module: package
  state: latest
  name: kubectl
```

#### Remove Package

```yaml
apache2:
  module: package
  state: removed
  name: apache2
```

#### Purge Package

```yaml
old_app:
  module: package
  state: purged
  name: old-app
```

#### Custom Repository

```yaml
nginx_mainline:
  module: package
  state: installed
  name: nginx
  repo: ppa:nginx/development
  update_cache: true
```

### Return Values

```yaml
result: changed | unchanged
comment: "Package nginx installed"
changes:
  installed: true
  version: "1.24.0"
```

## Service Module

Manage system services.

### States

- `running` - Ensure service is running
- `stopped` - Ensure service is stopped
- `enabled` - Enable service on boot
- `disabled` - Disable service on boot

Note: States can be combined (e.g., running + enabled)

### Parameters

**name** (string, required)
- Service name
- Example: `nginx`, `docker`

**enabled** (bool, optional)
- Enable/disable on boot
- Example: `true`

**reload** (bool, optional, default: false)
- Reload instead of restart on changes
- Example: `true`

**mask** (bool, optional, default: false)
- Mask service (prevent start)
- Example: `true`

**unmask** (bool, optional, default: false)
- Unmask service
- Example: `true`

### Platform Support

| Platform | Init System |
|----------|-------------|
| Linux (modern) | systemd |
| Linux (legacy) | upstart, sysvinit, openrc |
| macOS | launchd |
| Windows | Windows Service Manager |

### Examples

#### Start and Enable Service

```yaml
nginx:
  module: service
  state: running
  name: nginx
  enabled: true
```

#### Stop and Disable Service

```yaml
apache2:
  module: service
  state: stopped
  name: apache2
  enabled: false
```

#### Reload on Config Change

```yaml
nginx_service:
  module: service
  state: running
  name: nginx
  reload: true
  watch:
    - nginx_config
```

#### Just Enable (Don't Start)

```yaml
backup_service:
  module: service
  name: backup
  enabled: true
```

#### Mask Service

```yaml
unwanted_service:
  module: service
  name: unwanted
  mask: true
```

### Return Values

```yaml
result: changed | unchanged
comment: "Service nginx started"
changes:
  running: true
  enabled: true
```

## User Module

Manage user accounts.

### States

- `present` - Ensure user exists
- `absent` - Ensure user does not exist

### Parameters

**name** (string, required)
- Username
- Example: `myapp`, `deploy`

**uid** (int, optional)
- User ID
- Example: `1001`

**gid** (int, optional)
- Primary group ID
- Example: `1001`

**groups** (list, optional)
- Additional groups
- Example: `["docker", "sudo"]`

**home** (string, optional)
- Home directory
- Example: `/home/myapp`

**shell** (string, optional)
- Login shell
- Example: `/bin/bash`, `/usr/sbin/nologin`

**password** (string, optional)
- Hashed password
- Example: `$6$...` (crypt hash)

**system** (bool, optional, default: false)
- Create system user
- Example: `true`

**create_home** (bool, optional, default: true)
- Create home directory
- Example: `false`

**remove_home** (bool, optional, default: false)
- Remove home when deleting user
- Example: `true`

**comment** (string, optional)
- User comment (GECOS field)
- Example: `"Application User"`

### Examples

#### Create User

```yaml
myapp:
  module: user
  state: present
  name: myapp
  uid: 1001
  gid: 1001
  home: /home/myapp
  shell: /bin/bash
  groups:
    - docker
  comment: "Application User"
```

#### System User

```yaml
nginx:
  module: user
  state: present
  name: nginx
  system: true
  shell: /usr/sbin/nologin
  create_home: false
```

#### Remove User

```yaml
old_user:
  module: user
  state: absent
  name: old_user
  remove_home: true
```

### Return Values

```yaml
result: changed | unchanged
comment: "User myapp created"
changes:
  created: true
  uid: 1001
```

## Group Module

Manage groups.

### States

- `present` - Ensure group exists
- `absent` - Ensure group does not exist

### Parameters

**name** (string, required)
- Group name
- Example: `developers`, `docker`

**gid** (int, optional)
- Group ID
- Example: `2000`

**system** (bool, optional, default: false)
- Create system group
- Example: `true`

**members** (list, optional)
- Group members
- Example: `["user1", "user2"]`

**append** (bool, optional, default: false)
- Append members (don't replace)
- Example: `true`

### Examples

#### Create Group

```yaml
developers:
  module: group
  state: present
  name: developers
  gid: 2000
```

#### Group with Members

```yaml
docker:
  module: group
  state: present
  name: docker
  members:
    - alice
    - bob
```

#### System Group

```yaml
app_group:
  module: group
  state: present
  name: myapp
  system: true
```

#### Remove Group

```yaml
old_group:
  module: group
  state: absent
  name: old_group
```

### Return Values

```yaml
result: changed | unchanged
comment: "Group developers created"
changes:
  created: true
  gid: 2000
```

## Cmd Module

Execute commands.

### States

- `run` - Run command unconditionally
- `wait` - Run only when watched resource changes

### Parameters

**command** (string, required)
- Command to execute
- Example: `systemctl reload nginx`

**cwd** (string, optional)
- Working directory
- Example: `/opt/myapp`

**env** (map, optional)
- Environment variables
- Example:
  ```yaml
  env:
    PATH: /usr/local/bin:/usr/bin
    APP_ENV: production
  ```

**timeout** (string, optional, default: 5m)
- Execution timeout
- Example: `30s`, `5m`

**unless** (string, optional)
- Skip if this command succeeds
- Example: `test -f /var/lib/app/installed`

**only_if** (string, optional)
- Run only if this command succeeds
- Example: `systemctl is-active nginx`

**creates** (string, optional)
- Skip if file exists
- Example: `/var/lib/app/initialized`

**runas** (string, optional)
- Run as specific user
- Example: `myapp`

**shell** (string, optional)
- Shell to use
- Example: `bash`, `sh`, `powershell`

### Examples

#### Simple Command

```yaml
reload_app:
  module: cmd
  state: run
  command: systemctl reload myapp
```

#### Command with Environment

```yaml
database_migration:
  module: cmd
  state: run
  command: /usr/local/bin/migrate up
  cwd: /opt/myapp
  env:
    DATABASE_URL: postgres://localhost/myapp
  timeout: 5m
```

#### Conditional Execution

```yaml
initialize_db:
  module: cmd
  state: run
  command: /opt/myapp/init-db.sh
  unless: test -f /var/lib/myapp/initialized
```

#### Wait State (Run on Change)

```yaml
reload_nginx:
  module: cmd
  state: wait
  command: systemctl reload nginx
  watch:
    - nginx_config
```

#### Run as User

```yaml
app_task:
  module: cmd
  state: run
  command: /opt/myapp/task.sh
  runas: myapp
  cwd: /opt/myapp
```

### Return Values

```yaml
result: changed | unchanged | skipped
comment: "Command executed successfully"
changes:
  stdout: "Command output"
  stderr: ""
  exit_code: 0
  executed: true
```

## Requisites

All modules support requisites for dependency management:

### require

Execute after another state succeeds:

```yaml
nginx_service:
  module: service
  state: running
  name: nginx
  require:
    - nginx_package
    - nginx_config
```

### require_in

Make another state depend on this one:

```yaml
nginx_package:
  module: package
  state: installed
  name: nginx
  require_in:
    - nginx_service
```

### watch

Execute when another state changes:

```yaml
nginx_service:
  module: service
  state: running
  name: nginx
  reload: true
  watch:
    - nginx_config
```

### watch_in

Notify this state when watched state changes:

```yaml
nginx_config:
  module: file
  state: present
  path: /etc/nginx/nginx.conf
  watch_in:
    - nginx_service
```

### prereq

Must succeed before another state runs:

```yaml
database_schema:
  module: cmd
  state: run
  command: psql < schema.sql
  prereq:
    - database_data
```

### onchanges

Run only when watched state changes:

```yaml
clear_cache:
  module: cmd
  state: run
  command: rm -rf /var/cache/app/*
  onchanges:
    - app_code
```

## Idempotency

All modules are idempotent:

- Running multiple times produces the same result
- Only makes changes when necessary
- Safe to run repeatedly

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

Second run:
```
nginx_package: ✓ installed (unchanged)
```

## Cross-Platform Compatibility

### File Module
- ✅ Linux, Windows, macOS
- Permissions: Linux/macOS only

### Package Module
- ✅ Linux (all distros)
- ✅ macOS (homebrew)
- ✅ Windows (chocolatey, winget)

### Service Module
- ✅ Linux (systemd, upstart, sysv, openrc)
- ✅ macOS (launchd)
- ✅ Windows (Service Manager)

### User/Group Modules
- ✅ Linux, macOS
- ⚠️ Windows (limited support)

### Cmd Module
- ✅ Linux, Windows, macOS
- Shell varies by platform

## Template Support

All string parameters support Go template syntax:

```yaml
app_config:
  module: file
  state: present
  path: /etc/app/config.yaml
  contents: |
    database:
      host: {{ .vars.db_host }}
      port: {{ .vars.db_port }}
    environment: {{ .facts.environment }}
```

**Available in templates**:
- `.vars.*` - Variables from vars file
- `.facts.*` - Agent facts (OS, arch, hostname, etc.)

## Error Handling

Modules return structured results:

```yaml
# Success
result: changed | unchanged
comment: "Description"
changes: {...}

# Failure
result: failed
comment: "Error description"
error: "Detailed error message"
```

## Best Practices

1. **Use Requisites**: Always declare dependencies
2. **Idempotent Commands**: Make cmd module commands idempotent
3. **Test First**: Use `kscorectl state check` before apply
4. **Version Packages**: Pin package versions in production
5. **Meaningful IDs**: Use descriptive state IDs
6. **Group Related States**: Keep related states together

## Custom Modules

Beyond the built-in state modules, Keystone Core supports custom modules written in Starlark or compiled to WebAssembly (WASM). Custom modules enable extending Keystone Core with organization-specific state types, custom reactors, and specialized policies.

### Module System Overview

Custom modules are versioned, capability-scoped packages that integrate with the Keystone Core plugin system:

- **Starlark Modules**: Python-like syntax, sandboxed execution, deterministic
- **WASM Modules**: Compiled from Rust, Go (TinyGo), or C++, high performance

### Creating Custom Modules

Use the `kscore-module` CLI to scaffold and manage custom modules:

```bash
# Initialize a new Starlark module
kscore-module init myorg/custom-state

# Initialize a Rust WASM module
kscore-module init --template rust myorg/custom-provider
```

This creates the module structure:

```
myorg/custom-state/
├── module.yaml         # Module manifest
├── states/             # Starlark state files
│   └── main.star       # Main module code
├── tests/              # Test files
│   └── main_test.star  # Unit tests
└── README.md           # Documentation
```

### Module Manifest

The `module.yaml` defines module metadata, capabilities, and dependencies:

```yaml
name: myorg/custom-state
version: 0.1.0
type: starlark
description: Custom state management module

capabilities:
  - fs.read    # File system read access
  - exec       # Command execution

dependencies:
  std/files: ">=1.0.0"

metadata:
  repository: "https://github.com/myorg/custom-state"
  license: "MIT"
```

### Module Development Workflow

```bash
# Validate module manifest
kscore-module validate ./myorg/custom-state

# Run module tests
kscore-module test ./myorg/custom-state

# Resolve dependencies
kscore-module resolve ./myorg/custom-state

# Build distributable package
kscore-module build ./myorg/custom-state

# Verify module integrity
kscore-module verify ./myorg/custom-state/myorg-custom-state-0.1.0.zip
```

### Capability System

Modules can only access explicitly granted capabilities. Available capabilities:

| Capability | Description | Scope |
|------------|-------------|-------|
| `fs.read` | Read files | Path patterns |
| `fs.write` | Write files | Path patterns |
| `http.get` | HTTP GET requests | Domain patterns |
| `http.post` | HTTP POST requests | Domain patterns |
| `exec` | Execute commands | Command allowlist |
| `secrets.read` | Read secrets | Path patterns |
| `secrets.write` | Write secrets | Path patterns |
| `log` | Structured logging | Rate limited |
| `kv` | Key-value storage | Namespace scoped |
| `time` | Current time | Breaks determinism |

### Starlark Module Example

```python
# states/main.star
"""Custom state module for managing application config."""

def ensure_config(name, path, template):
    """Ensure application config exists with correct content."""

    # Read template
    content = fs.read(template)

    # Check if file exists with correct content
    if fs.exists(path):
        current = fs.read(path)
        if current == content:
            return {"result": "unchanged", "comment": "Config already correct"}

    # Write config
    fs.write(path, content)
    return {"result": "changed", "comment": "Config updated"}

def main():
    """Module entry point."""
    return {"status": "success"}
```

### Testing Custom Modules

Test files use assertions similar to standard testing frameworks:

```python
# tests/main_test.star
"""Tests for custom state module."""

load("//states/main.star", "ensure_config")

def test_ensure_config_creates_file():
    """Test that config is created correctly."""
    result = ensure_config("test", "/tmp/test.conf", "/templates/test.conf")
    assert.eq(result["result"], "changed")

def test_ensure_config_idempotent():
    """Test idempotency - second run should be unchanged."""
    ensure_config("test", "/tmp/test.conf", "/templates/test.conf")
    result = ensure_config("test", "/tmp/test.conf", "/templates/test.conf")
    assert.eq(result["result"], "unchanged")
```

### Module Distribution

After development, modules can be distributed via:

1. **OCI Registry**: Push to container registries (Docker Hub, GitHub Container Registry)
2. **HTTP Server**: Host module ZIPs on any HTTP server
3. **Git Repository**: Reference modules directly from Git

Distribution commands (coming soon):
```bash
# Sign module
kscore-module sign ./myorg/custom-state/myorg-custom-state-0.1.0.zip

# Publish to registry
kscore-module publish ./myorg/custom-state/myorg-custom-state-0.1.0.zip
```

## See Also

- [State Management Concepts](../../concepts/state-management/) - State management overview
- [Configuration Reference](../configuration/#state-file-configuration) - State file configuration
- [CLI Reference](../cli/#kscore-state-state-management) - State CLI commands
- [CLI Reference - Module Management](../cli/#kscore-module-module-management) - Module CLI commands
