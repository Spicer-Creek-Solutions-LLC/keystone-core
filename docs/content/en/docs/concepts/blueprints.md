---
title: "Blueprints"
description: "Pre-packaged, reusable state collections for infrastructure automation"
weight: 15
---

Blueprints are pre-packaged, reusable collections of states that can be shared, versioned, and composed to deploy complex infrastructure stacks. Similar to Salt Formulas, Ansible Roles, or Helm Charts, blueprints enable teams to encapsulate infrastructure patterns and share them across projects.

## Overview

### What is a Blueprint?

A blueprint is a versioned package containing:
- **States**: Declarative resource definitions (files, packages, services, users, etc.)
- **Parameters**: Configurable inputs with JSON Schema validation
- **Dependencies**: Other blueprints required for operation
- **Templates**: Dynamic configuration files
- **Tests**: Automated validation of blueprint behavior

### Blueprints vs Modules vs States

| Concept | Purpose | Format | Distribution |
|---------|---------|--------|--------------|
| **Modules** (Epic 9) | Extend functionality | Starlark/WASM code | Module registry |
| **Blueprints** (Epic 25) | Package state compositions | YAML configurations | Blueprint registry |
| **States** | Individual resources | YAML declarations | Part of blueprints |

### Key Benefits

- **Reusability**: Write once, deploy everywhere
- **Versioning**: Track changes with semantic versioning
- **Composition**: Combine blueprints into larger stacks
- **Testing**: Validate blueprints before deployment
- **Security**: Signed packages with cryptographic verification
- **Air-gapped**: Bundle for disconnected environments

## Blueprint Structure

```
blueprints/
└── myorg/
    └── web-stack/
        ├── blueprint.yaml      # Manifest with metadata and parameters
        ├── states/
        │   ├── nginx.yaml      # Web server configuration
        │   ├── database.yaml   # Database setup
        │   └── app.yaml        # Application deployment
        ├── files/              # Static files to distribute
        │   └── nginx.conf
        ├── templates/          # Dynamic configuration templates
        │   └── app.conf.tmpl
        ├── tests/              # Blueprint test definitions
        │   └── integration_test.yaml
        └── README.md           # Blueprint documentation
```

### Platform-Specific Defaults (vars/)

Blueprints support platform-specific parameter defaults through the `vars/` directory. This allows a single blueprint to work across different operating systems and architectures with appropriate default values for each platform.

```
blueprints/
└── myorg/
    └── web-stack/
        ├── blueprint.yaml
        ├── vars/
        │   ├── defaults.yaml              # Base defaults (all platforms)
        │   └── platforms/
        │       ├── debian.yaml            # Debian family defaults
        │       ├── debian-12.yaml         # Debian 12 specific
        │       ├── debian-arm64.yaml      # Debian on ARM64
        │       ├── rhel.yaml              # RHEL family defaults
        │       ├── rhel-9.yaml            # RHEL 9 specific
        │       ├── alpine.yaml            # Alpine Linux
        │       ├── linux.yaml             # All Linux (fallback)
        │       ├── darwin.yaml            # macOS
        │       ├── amd64.yaml             # x86_64 architecture
        │       └── arm64.yaml             # ARM64 architecture
        ├── states/
        └── ...
```

#### Default Loading Order (7 Layers)

Platform defaults are loaded and merged in the following order, with later layers overriding earlier ones:

1. **Schema defaults** - Default values from `blueprint.yaml` parameters
2. **vars/defaults.yaml** - Base defaults for all platforms
3. **vars/platforms/{os}.yaml** - OS-specific (`linux.yaml`, `darwin.yaml`, `windows.yaml`)
4. **vars/platforms/{family}.yaml** - Distribution family (`debian.yaml`, `rhel.yaml`, `alpine.yaml`)
5. **vars/platforms/{family}-{version}.yaml** - Family version (`debian-12.yaml`, `rhel-9.yaml`)
6. **vars/platforms/{family}-{arch}.yaml** - Family + architecture (`debian-arm64.yaml`)
7. **vars/platforms/{arch}.yaml** - Architecture-only (`arm64.yaml`, `amd64.yaml`)

#### Platform Detection

The system automatically detects the target platform from:
- `/etc/os-release` (Linux distributions)
- `/etc/lsb-release` (LSB-compliant systems)
- `/etc/redhat-release` (RHEL-based systems)
- System calls for architecture detection

#### Distribution Family Mapping

| Distribution | Family | Example |
|--------------|--------|---------|
| Ubuntu, Linux Mint, Raspbian | `debian` | `vars/platforms/debian.yaml` |
| CentOS, Rocky, AlmaLinux, Oracle | `rhel` | `vars/platforms/rhel.yaml` |
| Fedora | `fedora` | `vars/platforms/fedora.yaml` |
| Alpine | `alpine` | `vars/platforms/alpine.yaml` |
| Arch, Manjaro | `arch` | `vars/platforms/arch.yaml` |
| openSUSE, SLES | `suse` | `vars/platforms/suse.yaml` |
| Amazon Linux | `amazon` | `vars/platforms/amazon.yaml` |

#### Example: Cross-Platform Package Names

```yaml
# vars/defaults.yaml
package_name: nginx
config_path: /etc/nginx/nginx.conf

# vars/platforms/debian.yaml
package_name: nginx-full
php_package: php8.2-fpm

# vars/platforms/rhel.yaml
package_name: nginx
php_package: php-fpm
config_path: /etc/nginx/nginx.conf
selinux_enabled: true

# vars/platforms/alpine.yaml
package_name: nginx
php_package: php82-fpm
init_system: openrc

# vars/platforms/darwin.yaml
package_name: nginx  # Homebrew
config_path: /usr/local/etc/nginx/nginx.conf
```

#### Using Platform Defaults in States

Platform defaults are automatically available in templates:

```yaml
# states/packages.yaml
nginx_package:
  pkg.installed:
    - name: {{ .vars.package_name }}

php_fpm:
  pkg.installed:
    - name: {{ .vars.php_package }}
```

## Blueprint Manifest

The `blueprint.yaml` file is the heart of a blueprint:

```yaml
apiVersion: blueprints.kscore.io/v1
kind: Blueprint

metadata:
  name: web-stack
  version: 1.2.0
  description: Production-ready web application stack
  maintainers:
    - name: Platform Team
      email: platform@example.com
  license: Apache-2.0
  keywords:
    - web
    - nginx
    - production
  categories:
    - infrastructure
    - web-servers

parameters:
  domain:
    type: string
    description: Primary domain name
    required: true
    pattern: "^[a-z0-9-]+\\.[a-z]{2,}$"

  port:
    type: integer
    description: Application port
    default: 8080
    minimum: 1024
    maximum: 65535

  environment:
    type: string
    description: Deployment environment
    enum: [development, staging, production]
    default: production

  db_password:
    type: string
    description: Database password
    sensitive: true
    required: true

dependencies:
  requires:
    - myorg/nginx@^1.0.0
    - myorg/postgresql@~2.1.0
  requires_before:
    - myorg/ssl-certificates@1.0.0

features:
  ssl:
    description: Enable SSL/TLS
    default: true
  monitoring:
    description: Enable Prometheus metrics
    default: false

outputs:
  app_url:
    description: Application URL
    value: "https://{{ .parameters.domain }}"
  admin_url:
    description: Admin panel URL
    value: "https://{{ .parameters.domain }}/admin"

hooks:
  pre_apply:
    - states/pre-checks.yaml
  post_apply:
    - states/post-verification.yaml
  rollback:
    - states/rollback.yaml
```

## Developer Guide

### Creating a New Blueprint

#### 1. Initialize the Blueprint

```bash
# Create blueprint structure
kscorectl blueprint init myorg/my-blueprint

# This creates:
# myorg/my-blueprint/
# ├── blueprint.yaml
# ├── states/
# │   └── main.yaml
# ├── files/
# ├── templates/
# ├── tests/
# │   └── basic_test.yaml
# └── README.md
```

#### 2. Define Parameters

Parameters provide configurable inputs with validation:

```yaml
parameters:
  # String with pattern validation
  hostname:
    type: string
    description: Server hostname
    pattern: "^[a-z][a-z0-9-]*$"
    minLength: 3
    maxLength: 63

  # Integer with range
  worker_count:
    type: integer
    default: 4
    minimum: 1
    maximum: 32

  # Array of strings
  allowed_ips:
    type: array
    items:
      type: string
      format: ipv4
    default: []

  # Object with properties
  database:
    type: object
    properties:
      host:
        type: string
        default: localhost
      port:
        type: integer
        default: 5432
      name:
        type: string
        required: true

  # Sensitive parameter (never logged)
  api_key:
    type: string
    sensitive: true
    required: true
```

#### 3. Write State Definitions

States define the desired infrastructure configuration:

```yaml
# states/nginx.yaml
nginx_package:
  pkg.installed:
    - name: nginx
    - version: "{{ default "latest" .parameters.nginx_version }}"

nginx_config:
  file.managed:
    - name: /etc/nginx/nginx.conf
    - source: template://templates/nginx.conf.tmpl
    - template: true
    - vars:
        domain: "{{ .parameters.domain }}"
        port: "{{ .parameters.port }}"
    - require:
      - pkg: nginx_package

nginx_service:
  service.running:
    - name: nginx
    - enable: true
    - watch:
      - file: nginx_config
```

#### 4. Create Templates

Templates use Go text/template syntax for dynamic configuration:

```go-template
{{/* templates/nginx.conf.tmpl */}}
worker_processes auto;

events {
    worker_connections 1024;
}

http {
    server {
        listen {{ .port }};
        server_name {{ .domain }};

        {{- if .features.ssl }}
        listen 443 ssl;
        ssl_certificate /etc/ssl/certs/{{ .domain }}.crt;
        ssl_certificate_key /etc/ssl/private/{{ .domain }}.key;
        {{- end }}

        location / {
            proxy_pass http://127.0.0.1:{{ default 8080 .app_port }};
        }

        {{- if .features.monitoring }}
        location /metrics {
            stub_status on;
            allow 127.0.0.1;
            deny all;
        }
        {{- end }}
    }
}
```

#### 5. Add Tests

Test your blueprint behavior:

```yaml
# tests/integration_test.yaml
name: Integration Tests
description: Validate web-stack deployment

defaults:
  timeout: 5m
  parameters:
    domain: test.example.com
    port: 8080

setup:
  - command: echo "Setting up test environment"

cases:
  - name: nginx_installed
    description: Verify nginx is installed
    assertions:
      - type: package
        name: nginx
        state: installed

  - name: config_rendered
    description: Verify config file is rendered
    assertions:
      - type: file
        path: /etc/nginx/nginx.conf
        exists: true
        contains:
          - "server_name test.example.com"
          - "listen 8080"

  - name: service_running
    description: Verify nginx is running
    assertions:
      - type: service
        name: nginx
        state: running
        enabled: true

  - name: http_response
    description: Verify HTTP response
    assertions:
      - type: command
        command: curl -s -o /dev/null -w '%{http_code}' http://localhost:8080
        output:
          equals: "200"

teardown:
  - command: echo "Cleaning up"
```

#### 6. Validate and Build

```bash
# Validate blueprint structure and syntax
kscorectl blueprint validate myorg/my-blueprint

# Lint for best practices
kscorectl blueprint lint myorg/my-blueprint

# Run tests
kscorectl blueprint test myorg/my-blueprint

# Build for distribution
kscorectl blueprint build myorg/my-blueprint
```

### Lifecycle Hooks

Hooks allow blueprints to execute additional state files at specific points during the apply/rollback lifecycle.

#### Available Hooks

| Hook | Description | Execution Context |
|------|-------------|-------------------|
| `pre_apply` | Runs before main states are applied | Validation, prerequisites |
| `post_apply` | Runs after main states succeed | Verification, notifications |
| `pre_rollback` | Runs before rollback states execute | Cleanup preparation |
| `post_rollback` | Runs after rollback completes | Verification, alerting |

#### Hook Execution Order

```
Apply Workflow:
┌─────────────────────────┐
│  1. pre_apply hooks     │ ← Validates prerequisites, checks dependencies
├─────────────────────────┤
│  2. Main states         │ ← Core blueprint states from entrypoint
├─────────────────────────┤
│  3. post_apply hooks    │ ← Verifies success, sends notifications
└─────────────────────────┘

Rollback Workflow:
┌─────────────────────────┐
│  1. pre_rollback hooks  │ ← Prepares for rollback, stops services
├─────────────────────────┤
│  2. Rollback entrypoint │ ← Reverts to previous state
├─────────────────────────┤
│  3. post_rollback hooks │ ← Verifies rollback, sends alerts
└─────────────────────────┘
```

#### Hook Configuration

```yaml
hooks:
  pre_apply:
    - states/hooks/validate-prerequisites.yaml
    - states/hooks/check-disk-space.yaml
  post_apply:
    - states/hooks/verify-services.yaml
    - states/hooks/notify-success.yaml
  pre_rollback:
    - states/hooks/stop-services.yaml
  post_rollback:
    - states/hooks/verify-rollback.yaml
    - states/hooks/alert-on-rollback.yaml
```

#### Failure Handling

- **pre_apply failure**: Apply is aborted, no main states are executed
- **main state failure**: post_apply hooks are NOT executed; rollback may be triggered if configured
- **post_apply failure**: Logged as warning; apply is considered successful
- **pre_rollback failure**: Rollback continues (best effort)
- **post_rollback failure**: Logged as warning; rollback is considered successful

#### Context Available to Hooks

Hook state files have access to the same context as main states:

```yaml
# states/hooks/verify-services.yaml
nginx_service_check:
  cmd.run:
    - name: systemctl is-active {{ .parameters.service_name }}
    - failhard: true

health_check:
  cmd.run:
    - name: curl -sf http://localhost:{{ .parameters.port }}/health
    - unless: test "{{ .parameters.skip_health_check }}" = "true"
```

**Available context:**
- `.parameters` - All resolved blueprint parameters
- `.vars` - Platform defaults and custom variables
- `.features` - Enabled/disabled feature flags
- `.blueprint.name` - Blueprint name
- `.blueprint.version` - Blueprint version
- `.rollback` (in rollback hooks only):
  - `.rollback.from_version` - Version being rolled back from
  - `.rollback.to_version` - Version being rolled back to
  - `.rollback.snapshot_id` - Snapshot ID if using snapshot rollback

#### Hook Best Practices

1. **Keep hooks idempotent** - They may run multiple times
2. **Use fail_hard sparingly** - Only in pre_apply for critical checks
3. **Don't duplicate main state logic** - Hooks are for validation/notification
4. **Log context in hooks** - Helps with debugging failures
5. **Test hooks separately** - Include hook tests in your test suite

```yaml
# Example: pre_apply validation hook
# states/hooks/validate-prerequisites.yaml
check_disk_space:
  cmd.run:
    - name: |
        available=$(df /var --output=avail | tail -1)
        required={{ .parameters.required_disk_mb | default 1024 }}000
        if [ "$available" -lt "$required" ]; then
          echo "ERROR: Insufficient disk space"
          exit 1
        fi
    - failhard: true

check_port_available:
  cmd.run:
    - name: |
        if netstat -tuln | grep -q ":{{ .parameters.port }} "; then
          echo "ERROR: Port {{ .parameters.port }} is already in use"
          exit 1
        fi
    - failhard: true
    - unless: test "{{ .features.skip_port_check }}" = "true"
```

### Publishing Blueprints

#### Sign the Blueprint

```bash
# Generate signing key (first time only)
cosign generate-key-pair

# Sign the blueprint
kscorectl blueprint sign myorg/my-blueprint --key cosign.key
```

#### Publish to Registry

```bash
# Publish to official registry
kscorectl blueprint publish myorg/my-blueprint

# Publish to private registry
kscorectl blueprint publish myorg/my-blueprint --registry https://blueprints.internal.example.com
```

## User Guide

### Installing Blueprints

```bash
# Search for blueprints
kscorectl blueprint search nginx

# Show blueprint information
kscorectl blueprint info community/nginx-stack

# List available versions
kscorectl blueprint versions community/nginx-stack

# Install specific version
kscorectl blueprint install community/nginx-stack@1.2.0

# Install latest
kscorectl blueprint install community/nginx-stack
```

### Using Blueprints in States

Include blueprints in your state files:

```yaml
# /srv/states/webserver.yaml
include:
  - blueprint: community/nginx-stack@1.2.0
    as: nginx
    params:
      domain: www.example.com
      port: 443
      environment: production
    features:
      ssl: true
      monitoring: true
    secrets:
      ssl_key: !secret certificates/www.example.com/key
      ssl_cert: !secret certificates/www.example.com/cert

  - blueprint: community/postgresql@2.0.0
    as: database
    params:
      version: "15"
      max_connections: 200
    secrets:
      admin_password: !secret databases/postgres/admin
```

### Applying Blueprints

```bash
# Apply state file with blueprints
kscorectl state apply /srv/states/webserver.yaml

# Apply to specific targets
kscorectl state apply /srv/states/webserver.yaml

# Dry run to preview changes
kscorectl state apply /srv/states/webserver.yaml --dry-run
```

### Managing Installed Blueprints

```bash
# List installed blueprints
kscorectl blueprint list

# Show applied blueprint details
kscorectl blueprint show community/nginx-stack

# Update to latest compatible version
kscorectl blueprint update community/nginx-stack

# Remove blueprint
kscorectl blueprint remove community/nginx-stack
```

### Rollback

```bash
# List snapshots
kscorectl blueprint snapshot list community/nginx-stack

# Rollback to previous version
kscorectl blueprint rollback community/nginx-stack

# Rollback to specific version
kscorectl blueprint rollback community/nginx-stack --to-version 1.1.0

# Rollback to snapshot
kscorectl blueprint rollback community/nginx-stack --to-snapshot snap-20240115-120000
```

### Secrets Integration

Blueprints support secure secret management through the `!secret` YAML tag, allowing sensitive values to be resolved at runtime from external secret backends without ever being stored in blueprint definitions.

#### Secret Reference Syntax

```yaml
# Basic secret reference
password: !secret database/password

# With explicit backend
api_key: !secret vault:services/api-key

# With version pinning
certificate: !secret certificates/server@v2

# Full format: backend:path@version
ssl_key: !secret vault:pki/ssl-key@v3
```

#### Reference Format

| Component | Required | Description | Example |
|-----------|----------|-------------|---------|
| `backend` | No | Secret backend name (uses default if omitted) | `vault`, `k8s`, `env` |
| `path` | Yes | Secret path within the backend | `database/password` |
| `version` | No | Specific version (uses latest if omitted) | `v2`, `20240115` |

#### Available Backends

**Environment Variables (`env`)**

Resolves secrets from environment variables. Secret paths are converted to variable names:

```
Path: database/password → KSCORE_SECRET_DATABASE_PASSWORD
Path: my-app/api.key   → KSCORE_SECRET_MY_APP_API_KEY
```

- Path separators (`/`) become underscores
- Dashes (`-`) and dots (`.`) become underscores
- Names are converted to uppercase
- Default prefix is `KSCORE_SECRET_` (configurable)

```yaml
# Reads from KSCORE_SECRET_DATABASE_PASSWORD
db_password: !secret env:database/password
```

**HashiCorp Vault**

For production deployments, Vault integration provides:
- Secret versioning
- Dynamic secrets
- Lease management
- Access policies

```yaml
# Vault KV v2 secret
api_key: !secret vault:secret/data/myapp/api-key

# Versioned secret
certificate: !secret vault:pki/certs/server@v2
```

**Kubernetes Secrets (`k8s`)**

Reads secrets from Kubernetes Secret resources:

```yaml
# Format: namespace/secret-name/key
db_password: !secret k8s:production/db-credentials/password
```

**In-Memory (Testing)**

For testing blueprints, an in-memory resolver can be configured:

```go
resolver := blueprint.NewInMemorySecretResolver()
resolver.SetSecret("database/password", "test-password")
resolver.SetSecretVersion("api/key", "v1", "key-v1")
```

#### Using Secrets in Blueprints

**In Parameter Defaults**

```yaml
# blueprint.yaml - NOT recommended for defaults
# Use !secret in state invocations instead
parameters:
  db_password:
    type: string
    sensitive: true
    required: true  # User must provide via !secret
```

**In State Invocations**

```yaml
# states/webserver.yaml
include:
  - blueprint: community/postgresql@2.0.0
    params:
      version: "15"
    secrets:
      admin_password: !secret databases/postgres/admin
      replication_password: !secret databases/postgres/replication
```

**In Nested Structures**

```yaml
include:
  - blueprint: myorg/web-app
    params:
      database:
        host: db.example.com
        port: 5432
      credentials:
        username: app_user
    secrets:
      credentials:
        password: !secret databases/app/password
      api_keys:
        stripe: !secret services/stripe/api-key
        sendgrid: !secret services/sendgrid/api-key
```

#### Multi-Backend Configuration

Configure multiple secret backends with a default:

```yaml
# Server configuration
secrets:
  backends:
    vault:
      type: vault
      address: https://vault.example.com
      auth:
        method: kubernetes
        role: keystone-core
    env:
      type: environment
      prefix: KSCORE_SECRET
    k8s:
      type: kubernetes
      namespace: secrets
  default: vault
```

#### Secret Validation

Before blueprint apply, all secret references are validated:

```bash
# Validate secrets exist (without revealing values)
kscorectl blueprint validate myorg/my-blueprint --check-secrets

# Output:
# ✓ vault:database/password - exists
# ✓ vault:api/key - exists
# ✗ vault:missing/secret - NOT FOUND
```

Validation checks:
1. Backend exists and is configured
2. Secret path is valid
3. Secret exists in the backend
4. Version exists (if specified)

#### Secret Resolution Flow

```
Blueprint Apply:
┌─────────────────────────────┐
│ 1. Parse blueprint          │ ← Identifies !secret references
├─────────────────────────────┤
│ 2. Collect references       │ ← Builds list of all secrets needed
├─────────────────────────────┤
│ 3. Validate existence       │ ← Checks all secrets exist (fails early)
├─────────────────────────────┤
│ 4. Resolve at runtime       │ ← Retrieves actual values
├─────────────────────────────┤
│ 5. Inject into parameters   │ ← Replaces !secret with resolved values
├─────────────────────────────┤
│ 6. Execute states           │ ← States receive resolved values
└─────────────────────────────┘
```

#### Security Best Practices

**Do:**
- Use `!secret` for all sensitive values (passwords, API keys, certificates)
- Mark sensitive parameters with `sensitive: true` in schema
- Use versioned secrets for reproducibility
- Validate secrets before apply with `--check-secrets`
- Rotate secrets regularly using versioning

**Don't:**
- Hardcode secrets in blueprints, state files, or templates
- Log or print resolved secret values
- Store secrets in version control
- Use secrets as non-sensitive parameter defaults

**Example: Complete Secure Configuration**

```yaml
# blueprint.yaml
parameters:
  db_host:
    type: string
    default: localhost
  db_name:
    type: string
    required: true
  db_user:
    type: string
    required: true
  db_password:
    type: string
    sensitive: true  # Never logged
    required: true   # Must be provided

# Usage in state file:
include:
  - blueprint: myorg/database-app
    params:
      db_host: db.example.com
      db_name: myapp
      db_user: app_user
    secrets:
      db_password: !secret vault:databases/myapp/password@latest
```

### Entrypoints System

Entrypoints define which state files are executed when a blueprint is applied. Blueprints can have multiple named entrypoints for different use cases (installation, upgrade, rollback, etc.).

#### Defining Entrypoints

```yaml
# blueprint.yaml
entrypoints:
  default: states/install.yaml     # Standard installation
  main: states/install.yaml        # Alias for default
  upgrade: states/upgrade.yaml     # Upgrade procedure
  rollback: states/rollback.yaml   # Rollback procedure
  configure: states/config.yaml    # Configuration-only
  uninstall: states/uninstall.yaml # Clean removal
```

#### Special Entrypoints

| Entrypoint | Purpose | Fallback Behavior |
|------------|---------|-------------------|
| `default` | Standard blueprint application | Falls back to `states/init.yaml` if not defined |
| `main` | Alias for default | Used if `default` is not defined |
| `rollback` | Rollback procedure | If not defined, rollback uses snapshots only |

#### Entrypoint Resolution Order

When a blueprint is included without specifying an entrypoint:

```
1. Check for "default" entrypoint
   │
   ├─ Found → Use specified state file
   │
   └─ Not found → Check for "main" entrypoint
                  │
                  ├─ Found → Use specified state file
                  │
                  └─ Not found → Use "states/init.yaml"
```

#### Using Entrypoints in State Includes

**Using Default Entrypoint**

```yaml
# Uses default/main entrypoint automatically
include:
  - blueprint: community/postgresql@2.0.0
    params:
      version: "15"
```

**Specifying a Named Entrypoint**

```yaml
# Uses the 'configure' entrypoint
include:
  - blueprint: community/postgresql@2.0.0
    entrypoint: configure
    params:
      max_connections: 200
      shared_buffers: 256MB
```

**Upgrade Scenario**

```yaml
# Uses the 'upgrade' entrypoint for version migration
include:
  - blueprint: community/postgresql@2.0.0
    entrypoint: upgrade
    params:
      from_version: "14"
      to_version: "15"
      backup_first: true
```

#### Entrypoint Context

Each entrypoint has access to the same context:

```yaml
# states/upgrade.yaml
check_prerequisites:
  cmd.run:
    - name: |
        echo "Upgrading {{ .metadata.name }} from {{ .parameters.from_version }}"
        # Verify disk space, permissions, etc.
    - failhard: true

backup_data:
  cmd.run:
    - name: pg_dumpall > /backup/pre-upgrade-{{ .parameters.from_version }}.sql
    - unless: test "{{ .parameters.backup_first }}" != "true"
    - require:
      - cmd: check_prerequisites

perform_upgrade:
  pkg.installed:
    - name: postgresql-{{ .parameters.to_version }}
    - require:
      - cmd: backup_data
```

#### Rollback Entrypoint

The `rollback` entrypoint receives special context parameters:

```yaml
# states/rollback.yaml
{{- $rollback := .parameters.rollback -}}

announce_rollback:
  cmd.run:
    - name: |
        echo "Rolling back from {{ $rollback.from_version }} to {{ $rollback.to_version }}"
        {{- if $rollback.snapshot_id }}
        echo "Using snapshot: {{ $rollback.snapshot_id }}"
        {{- end }}

restore_config:
  file.managed:
    - name: /etc/myapp/config.yaml
    - source: file://{{ $rollback.snapshot_path }}/config.yaml
    - require:
      - cmd: announce_rollback
```

Rollback context includes:
- `rollback.from_version` - Version being rolled back from
- `rollback.to_version` - Version being rolled back to
- `rollback.snapshot_id` - ID of snapshot (if using snapshots)
- `rollback.snapshot_time` - Timestamp of snapshot

#### CLI Entrypoint Selection

```bash
# Apply with default entrypoint
kscorectl state apply mystate.yaml

# Apply with specific entrypoint
kscorectl blueprint apply community/nginx-stack --entrypoint configure

# Run rollback entrypoint
kscorectl blueprint rollback community/nginx-stack

# Validate specific entrypoint exists
kscorectl blueprint validate myorg/my-blueprint --entrypoint upgrade
```

#### Entrypoint Best Practices

**Do:**
- Define a `default` entrypoint for standard installation
- Create separate entrypoints for upgrade and rollback procedures
- Keep entrypoints focused (single responsibility)
- Document what each entrypoint does in comments

**Don't:**
- Duplicate large amounts of state code between entrypoints
- Create entrypoints for minor configuration variations (use parameters)
- Assume entrypoints share state (they run independently)

**Example: Multi-Entrypoint Blueprint**

```yaml
# blueprint.yaml
apiVersion: blueprints.kscore.io/v1
kind: Blueprint

metadata:
  name: web-application
  version: 1.0.0

entrypoints:
  # Full installation with all components
  default: states/full-install.yaml

  # Quick deployment (skip optional components)
  quick: states/minimal-install.yaml

  # Configuration updates only (no package changes)
  configure: states/configure-only.yaml

  # Database migration
  migrate: states/db-migrate.yaml

  # Controlled rollback
  rollback: states/rollback.yaml

  # Health check and verification
  verify: states/verify.yaml

  # Clean uninstall
  uninstall: states/uninstall.yaml

parameters:
  environment:
    type: string
    enum: [development, staging, production]
    required: true

  skip_migrations:
    type: boolean
    default: false
    description: Skip database migrations (use with 'quick' entrypoint)
```

## Applied Blueprint Tracking

Keystone Core tracks which blueprints have been applied to which agents, enabling version management, rollback capabilities, and fleet-wide visibility.

### Tracker Architecture

The tracking system maintains per-agent state with full history:

```
┌─────────────────────────────────────────────────────────┐
│                    Tracker Storage                       │
│                                                          │
│  ┌─────────────────────┐  ┌─────────────────────────┐   │
│  │  Agent States       │  │  Agent History          │   │
│  │                     │  │                         │   │
│  │  agent-1:           │  │  agent-1:               │   │
│  │    web: v1.2.0      │  │    [applied web v1.2.0] │   │
│  │    db:  v2.0.0      │  │    [applied db v2.0.0]  │   │
│  │                     │  │    [applied web v1.1.0] │   │
│  │  agent-2:           │  │                         │   │
│  │    web: v1.1.0      │  │  agent-2:               │   │
│  │                     │  │    [applied web v1.1.0] │   │
│  └─────────────────────┘  └─────────────────────────┘   │
│                                                          │
│  Storage: /var/lib/kscore/blueprint-tracker.json         │
└─────────────────────────────────────────────────────────┘
```

### Tracked Information

For each applied blueprint, the tracker records:

| Field | Description |
|-------|-------------|
| `name` | Blueprint name (e.g., `myorg/web-stack`) |
| `version` | Resolved version (e.g., `1.2.0`) |
| `namespace` | Instance namespace (from `as:` or default) |
| `parameters` | Resolved parameter values |
| `enabled_features` | List of enabled features |
| `applied_at` | Timestamp of application |
| `applied_by` | Who/what triggered the apply |
| `state_count` | Total states in blueprint |
| `successful_states` | States that succeeded |
| `failed_states` | States that failed |
| `status` | Current status (`applied`, `failed`, `removed`) |
| `checksum` | Content hash for change detection |

### History Actions

The tracker records four types of actions:

| Action | Description |
|--------|-------------|
| `applied` | Blueprint was applied (new or update) |
| `removed` | Blueprint was removed |
| `rolled_back` | Blueprint was rolled back to previous version |
| `updated` | Blueprint version was changed |

### Configuration

Configure the tracker in the control plane:

```yaml
# server.yaml
blueprint_tracker:
  # Where to store tracking data
  store_path: /var/lib/kscore/blueprint-tracker.json

  # Maximum history entries per agent (oldest trimmed)
  max_history_per_agent: 100

  # Persist to disk on every change (recommended for production)
  persist_on_change: true
```

### CLI Commands

#### View Applied Blueprints

```bash
# List all applied blueprints on an agent
kscorectl blueprint applied list --agent agent-1

# Output:
# NAMESPACE   BLUEPRINT           VERSION   STATUS    APPLIED AT
# web         myorg/web-stack     1.2.0     applied   2024-01-15T10:30:00Z
# db          myorg/db-stack      2.0.0     applied   2024-01-14T08:15:00Z

# Show detailed info for a specific applied blueprint
kscorectl blueprint applied show --agent agent-1 --namespace web

# Output:
# Name:            myorg/web-stack
# Version:         1.2.0
# Namespace:       web
# Status:          applied
# Applied At:      2024-01-15T10:30:00Z
# Applied By:      user@example.com
# States:          5 total, 5 succeeded, 0 failed
# Enabled Features: ssl, monitoring
# Parameters:
#   domain: example.com
#   port: 8080
```

#### View Blueprint History

```bash
# View history for an agent
kscorectl blueprint applied history --agent agent-1 --limit 10

# Output:
# TIMESTAMP            ACTION       BLUEPRINT         FROM      TO        SUCCESS
# 2024-01-15T10:30:00  applied      myorg/web-stack   1.1.0     1.2.0     true
# 2024-01-14T08:15:00  applied      myorg/db-stack    -         2.0.0     true
# 2024-01-10T14:20:00  rolled_back  myorg/web-stack   1.2.0     1.1.0     true
# 2024-01-10T12:00:00  applied      myorg/web-stack   1.0.0     1.2.0     false
```

#### Find Blueprint Usage

```bash
# Find which agents use a specific blueprint
kscorectl blueprint applied usage myorg/web-stack

# Output:
# AGENT       NAMESPACE   VERSION   STATUS    APPLIED AT
# agent-1     web         1.2.0     applied   2024-01-15T10:30:00Z
# agent-2     web         1.1.0     applied   2024-01-12T09:00:00Z
# agent-3     frontend    1.2.0     applied   2024-01-15T11:00:00Z
```

### Programmatic Access

#### Recording Applications

The tracker automatically records blueprint applications during state execution:

```go
// Internal usage during blueprint application
tracker.RecordApply(
    agentID,
    &blueprint.AppliedBlueprintInfo{
        Name:       "myorg/web-stack",
        Version:    "1.2.0",
        Namespace:  "web",
        Parameters: resolvedParams,
        StateCount: 5,
    },
    "user@example.com",  // triggered by
    duration,            // how long it took
    nil,                 // error (nil = success)
)
```

#### Querying State

```go
// Get all applied blueprints for an agent
state := tracker.GetAgentState("agent-1")
for namespace, info := range state.AppliedBlueprints {
    fmt.Printf("%s: %s@%s (%s)\n",
        namespace, info.Name, info.Version, info.Status)
}

// Get specific blueprint
info := tracker.GetAppliedBlueprint("agent-1", "web")
if info != nil {
    fmt.Printf("Applied: %s@%s by %s\n",
        info.Name, info.Version, info.AppliedBy)
}
```

#### Finding Rollback Targets

```go
// Find previous version for rollback
previousVersion, err := tracker.FindRollbackTarget("agent-1", "web")
if err != nil {
    log.Printf("No previous version found: %v", err)
} else {
    fmt.Printf("Can rollback to: %s\n", previousVersion)
}
```

### Integration with Rollback

The tracker integrates with the rollback system to enable version-aware rollbacks:

```bash
# Rollback using tracked history
kscorectl blueprint rollback --agent agent-1 --namespace web

# This automatically:
# 1. Looks up the current version from tracker
# 2. Finds the previous successful version from history
# 3. Executes the rollback entrypoint
# 4. Records the rollback in history
```

### Data Persistence

The tracker uses atomic file operations for reliability:

1. **Write to temp file**: `blueprint-tracker.json.tmp`
2. **Atomic rename**: Replace existing file
3. **Load on startup**: Restore previous state

```bash
# Tracker data location
ls -la /var/lib/kscore/blueprint-tracker.json

# Example content structure
{
  "version": 1,
  "updated_at": "2024-01-15T10:30:00Z",
  "agents": {
    "agent-1": {
      "agent_id": "agent-1",
      "applied_blueprints": {
        "web": {
          "name": "myorg/web-stack",
          "version": "1.2.0",
          ...
        }
      },
      "last_updated": "2024-01-15T10:30:00Z"
    }
  },
  "history": {
    "agent-1": {
      "agent_id": "agent-1",
      "entries": [...]
    }
  }
}
```

### Use Cases

#### Fleet Version Report

Generate a report of blueprint versions across your fleet:

```bash
# Script to generate fleet version report
for agent in $(kscorectl agent list -o name); do
  echo "=== $agent ==="
  kscorectl blueprint applied list --agent "$agent"
done
```

#### Detecting Drift

Compare applied versions to find inconsistencies:

```bash
# Find agents with outdated versions
kscorectl blueprint applied usage myorg/web-stack | \
  grep -v "1.2.0" | \
  awk '{print $1 " has " $3}'
```

#### Audit Trail

Use history for compliance and troubleshooting:

```bash
# Export history for audit
kscorectl blueprint applied history --agent agent-1 --output json > audit.json
```

## Best Practices

### 1. Parameter Design

**Do:**
- Use descriptive names and descriptions
- Provide sensible defaults where possible
- Mark sensitive parameters appropriately
- Use JSON Schema validation (patterns, ranges, enums)
- Group related parameters into objects

**Don't:**
- Require parameters that could have defaults
- Expose internal implementation details
- Use overly generic names like `value` or `data`

```yaml
# Good
parameters:
  max_connections:
    type: integer
    description: Maximum concurrent database connections
    default: 100
    minimum: 10
    maximum: 1000

# Bad
parameters:
  conn:
    type: integer
```

### 2. Dependency Management

**Do:**
- Use semantic version constraints (`^1.0.0`, `~2.1.0`)
- Prefer `requires` over `requires_before` when order doesn't matter
- Document why dependencies are needed
- Test with minimum supported versions

**Don't:**
- Pin exact versions without reason
- Create circular dependencies
- Include unnecessary dependencies

```yaml
# Good - allows patch updates
dependencies:
  requires:
    - community/base-config@^1.0.0

# Avoid - too restrictive
dependencies:
  requires:
    - community/base-config@1.0.0
```

### 3. State Organization

**Do:**
- Keep states focused and single-purpose
- Use meaningful file names
- Group related resources together
- Use requisites for dependencies

**Don't:**
- Put everything in one file
- Create deeply nested includes
- Hardcode values that should be parameters

```yaml
# Good - organized by concern
states/
├── packages.yaml    # Package installation
├── config.yaml      # Configuration files
├── services.yaml    # Service management
└── users.yaml       # User/group setup

# Bad - monolithic
states/
└── everything.yaml
```

### 4. Testing

**Do:**
- Test all parameter combinations
- Include negative tests (invalid inputs)
- Test idempotency (apply twice = same result)
- Test rollback procedures
- Use CI/CD for automated testing

**Don't:**
- Skip tests for "simple" blueprints
- Only test the happy path
- Assume environments are identical

```yaml
# Comprehensive test coverage
cases:
  - name: default_parameters
    description: Works with defaults

  - name: custom_parameters
    description: Works with custom values
    parameters:
      port: 9090

  - name: invalid_port
    description: Rejects invalid port
    parameters:
      port: -1
    expect_failure: true

  - name: idempotency
    description: Second apply makes no changes
    apply_count: 2
    assertions:
      - type: state
        changed: false
```

### 5. Security

**Do:**
- Never hardcode secrets
- Use `!secret` references for sensitive data
- Mark sensitive parameters
- Sign all published blueprints
- Review dependencies for vulnerabilities

**Don't:**
- Log sensitive parameter values
- Store secrets in templates
- Trust unsigned blueprints in production

```yaml
# Good - secrets from backend
params:
  db_password: !secret databases/prod/password

# Bad - hardcoded
params:
  db_password: "mysecretpassword"
```

### 6. Documentation

**Do:**
- Write clear README files
- Document all parameters
- Provide usage examples
- Include troubleshooting guides
- Maintain a changelog

**Don't:**
- Assume users know your domain
- Skip documentation for "obvious" things

### 7. Versioning

**Do:**
- Follow semantic versioning
- Document breaking changes
- Provide migration guides
- Support at least 2 major versions

**Don't:**
- Make breaking changes in minor/patch versions
- Remove parameters without deprecation period
- Change default behavior silently

```yaml
# Major version for breaking changes
metadata:
  version: 2.0.0  # Changed parameter schema

# Minor version for new features
metadata:
  version: 1.1.0  # Added monitoring feature

# Patch version for fixes
metadata:
  version: 1.0.1  # Fixed config template
```

## Air-Gapped Deployment

For environments without internet access:

### Creating Bundles

```bash
# Create bundle with all dependencies
kscorectl blueprint bundle myorg/my-stack \
  --output my-stack-bundle.tar.gz \
  --include-modules

# Sign the bundle
kscorectl blueprint sign my-stack-bundle.tar.gz --key cosign.key
```

### Running a Mirror Server

```bash
# Start mirror server
kscorectl blueprint mirror serve \
  --storage-dir /var/lib/kscore/blueprints \
  --listen :8080

# Import bundle to mirror
kscorectl blueprint mirror import my-stack-bundle.tar.gz \
  --mirror http://mirror.internal:8080
```

### Installing from Mirror

```bash
# Configure mirror as registry
kscorectl config set blueprint.registry http://mirror.internal:8080

# Install from mirror
kscorectl blueprint install myorg/my-stack@1.0.0
```

## Troubleshooting

### Common Issues

#### Blueprint Validation Fails

```bash
# Get detailed validation errors
kscorectl blueprint validate myorg/my-blueprint --verbose

# Common causes:
# - Missing required fields in blueprint.yaml
# - Invalid parameter schemas
# - Syntax errors in state files
# - Circular dependencies
```

#### Dependency Resolution Fails

```bash
# Show dependency tree
kscorectl blueprint tree myorg/my-blueprint

# Common causes:
# - Conflicting version requirements
# - Missing dependencies in registry
# - Circular dependency chains
```

#### State Application Fails

```bash
# Run a dry-run first
kscorectl state check state.yaml

# Common causes:
# - Missing required parameters
# - Invalid parameter values
# - Template rendering errors
# - Permission issues
```

#### Signature Verification Fails

```bash
# Verify signature manually
kscorectl blueprint verify myorg/my-blueprint

# Common causes:
# - Unsigned blueprint
# - Untrusted signing key
# - Tampered package
```

## Community Contribution Guidelines

### Contributing Blueprints

The Keystone Core community welcomes blueprint contributions. Follow these guidelines to ensure your blueprints meet quality standards.

#### Submission Requirements

1. **Working Implementation** - Blueprint must be tested and functional
2. **Documentation** - Complete README with usage examples
3. **Tests** - At least basic integration tests
4. **Signed** - All submissions must be signed with a verified key
5. **License** - Apache-2.0, MIT, or other OSI-approved license

#### Naming Conventions

- Use lowercase letters, numbers, and hyphens only
- Use descriptive names: `nginx-proxy`, not `np`
- Prefix with organization/author: `myorg/nginx-proxy`
- Avoid generic names: `web-server`, not `server`

```
# Good names
community/prometheus-stack
mycompany/internal-proxy
devops-team/deployment-pipeline

# Bad names
my-stuff
test123
nginx  (too generic)
```

#### Quality Standards

**Code Quality:**
- No hardcoded secrets or credentials
- Parameterize all configurable values
- Use meaningful state and resource names
- Include comments for complex logic

**Security:**
- Follow least-privilege principles
- Document security implications
- No execution of arbitrary user input
- Validate all parameters

**Compatibility:**
- Test on all claimed platforms
- Document platform-specific behavior
- Support at least 2 recent OS versions

### Review Process

1. **Submit** - Create PR to blueprint-registry repository
2. **Automated Checks** - CI validates structure and syntax
3. **Security Review** - Automated security scanning
4. **Maintainer Review** - Human review for quality and usefulness
5. **Testing** - Deployment testing on reference infrastructure
6. **Approval** - Merge and publish to registry

### Maintaining Your Blueprints

As a maintainer, you are responsible for:

- Responding to issues within 2 weeks
- Reviewing and merging community PRs
- Keeping dependencies up to date
- Fixing security vulnerabilities promptly
- Following semantic versioning

#### Transferring Ownership

If you can no longer maintain a blueprint:

1. Find a new maintainer (or request community adoption)
2. Open an issue in the registry repository
3. Community team will facilitate transfer

### Community Registry vs Private Registry

| Aspect | Community Registry | Private Registry |
|--------|-------------------|------------------|
| **Audience** | Public, open source | Organization-internal |
| **Review** | Community review process | Internal process |
| **Signing** | Required, verified | Organization policy |
| **Support** | Community-based | Organization-based |
| **Naming** | Unique globally | Unique within org |

### Code of Conduct

All contributors must follow the Keystone Core Code of Conduct:

- Be respectful and inclusive
- Provide constructive feedback
- Focus on the code, not the person
- Report issues through proper channels

### Getting Help

- **Discord**: Join #blueprints channel
- **GitHub Discussions**: Ask questions
- **Documentation**: Read the guides
- **Examples**: Study existing blueprints

### Example Blueprints

The following example blueprints are available in the repository:

#### LAMP Stack (`examples/blueprints/lamp-stack`)
A complete Linux, Apache, MySQL, PHP stack for web applications.

**Features:**
- Apache with security headers and virtual hosts
- PHP with OPcache and common extensions
- MySQL/MariaDB with secure installation
- Optional phpMyAdmin

#### Monitoring Stack (`examples/blueprints/kscore/monitoring-stack`)
Production monitoring with Prometheus, Grafana, and Node Exporter.

**Features:**
- Prometheus metrics collection
- Grafana visualization with pre-built dashboards
- Node Exporter for system metrics
- Alertmanager for notifications
- Default alert rules

#### Security Baseline (`examples/blueprints/kscore/security-baseline`)
Security hardening for Linux servers following industry best practices.

**Features:**
- SSH hardening with modern ciphers
- Firewall configuration (UFW/firewalld)
- Kernel security parameters (sysctl)
- Audit logging with auditd
- Password policies and system hardening

## See Also

- [State Management](/docs/concepts/state-management/) - Understanding states
- [Modules](/docs/concepts/modules/) - Code-based extensions
- [Policy](/docs/concepts/policy/) - Policy enforcement for blueprints
- [Blueprint Reference](/docs/reference/blueprints/) - Complete API reference
- [Blueprint Catalog](/docs/reference/blueprints-catalog/) - Official blueprint list
