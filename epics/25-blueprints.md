# Epic 25: Blueprints - Composable Infrastructure Patterns

## Overview

Blueprints are pre-packaged, reusable collections of states that implement common infrastructure patterns. Similar to Salt Project's Formulas, they enable the community to share battle-tested configurations for common use cases like web application stacks, monitoring systems, security baselines, and more.

### Goals

1. **Reusability**: Package common infrastructure patterns for reuse across organizations
2. **Community**: Enable sharing and collaboration on infrastructure configurations
3. **Simplicity**: Allow operators to deploy complex stacks with minimal configuration
4. **Flexibility**: Support customization through well-defined parameter schemas
5. **Reliability**: Provide testing, versioning, and rollback capabilities

### Non-Goals

1. Replacing the module system (Epic 9) - blueprints compose, modules extend
2. Providing application deployment pipelines - use GitOps (Epic 5) for that
3. Container orchestration - use Kubernetes for that

## Concepts

### Blueprint vs Module vs State

| Concept | Purpose | Contains | Author | Complexity |
|---------|---------|----------|--------|------------|
| **State** | Configure a resource | Resource declarations | Operator | Low |
| **Blueprint** | Compose a solution | States + parameters | Community | Medium |
| **Module** | Extend functionality | Code (Starlark/WASM) | Developer | High |

### Namespace

Blueprints use a separate namespace from modules:
- Blueprints: `blueprints/vendor/name` (e.g., `blueprints/community/web-app-stack`)
- Modules: `modules/vendor/name` (e.g., `modules/std/files`)

### Dependency Types

Blueprints support two dependency ordering modes:

| Dependency | Behavior | Use Case |
|------------|----------|----------|
| `requires` | Must be included, can execute concurrently | Independent components |
| `requires_before` | Must complete successfully before this blueprint starts | Sequential dependencies |

Additionally, blueprints inherit state-level requisites from Epic 3:
- `watch` - Re-apply if dependency changes
- `onchanges` - Only apply if dependency changed

## Blueprint Structure

```
blueprints/community/web-app-stack/
├── blueprint.yaml              # Manifest (required)
├── README.md                   # Documentation (required for publishing)
├── CHANGELOG.md                # Version history
├── LICENSE                     # License file
├── states/
│   ├── init.yaml               # Default entry point
│   ├── nginx/
│   │   ├── install.yaml
│   │   ├── configure.yaml
│   │   └── service.yaml
│   ├── postgres/
│   │   ├── install.yaml
│   │   ├── configure.yaml
│   │   ├── databases.yaml
│   │   └── service.yaml
│   ├── app/
│   │   ├── install.yaml
│   │   ├── deploy.yaml
│   │   └── service.yaml
│   └── rollback.yaml           # Rollback entry point (optional)
├── vars/
│   ├── defaults.yaml           # Default parameter values
│   └── platforms/
│       ├── debian.yaml         # Debian-specific defaults
│       ├── rhel.yaml           # RHEL-specific defaults
│       └── alpine.yaml         # Alpine-specific defaults
├── templates/
│   ├── nginx/
│   │   ├── nginx.conf.j2
│   │   └── vhost.conf.j2
│   └── postgres/
│       └── pg_hba.conf.j2
├── files/
│   └── scripts/
│       └── healthcheck.sh
└── tests/
    ├── test_nginx.yaml
    ├── test_postgres.yaml
    └── test_integration.yaml
```

## Blueprint Manifest

### Full Schema

```yaml
apiVersion: blueprints.keystone-core.io/v1
kind: Blueprint

metadata:
  name: web-app-stack
  version: 1.2.0
  description: |
    Complete web application stack with NGINX reverse proxy,
    PostgreSQL database, and configurable application runtime.

  maintainers:
    - name: Community Team
      email: blueprints@example.com
      url: https://github.com/community

  license: Apache-2.0
  repository: https://github.com/kscore-blueprints/web-app-stack
  documentation: https://blueprints.keystone-core.io/web-app-stack

  keywords:
    - web
    - nginx
    - postgres
    - nodejs
    - python

  categories:
    - application-stack
    - web-server
    - database

# Compatibility requirements
compatibility:
  kscore: ">=1.5.0"

  # Required modules
  modules:
    - modules/std/files@^1.0
    - modules/std/exec@^1.0
    - modules/community/nginx@^2.0

  # Supported platforms
  platforms:
    - os: linux
      family: debian
      versions: ["11", "12"]
    - os: linux
      family: rhel
      versions: ["8", "9"]
    - os: linux
      family: alpine
      versions: ["3.18", "3.19"]

# Blueprint dependencies
dependencies:
  requires:
    # Soft dependency - must be included, can run concurrently
    - blueprints/community/base-system@^1.0

  requires_before:
    # Hard dependency - must complete before this blueprint starts
    - blueprints/community/ssl-certificates@^2.0

# Optional feature flags
features:
  ssl:
    description: Enable SSL/TLS termination at NGINX
    default: true
    enables:
      - states/nginx/ssl.yaml
    requires:
      - blueprints/community/ssl-certificates@^2.0

  monitoring:
    description: Enable Prometheus exporters
    default: false
    enables:
      - states/monitoring/
    parameters:
      - monitoring.*

  backup:
    description: Enable automated database backups
    default: false
    enables:
      - states/postgres/backup.yaml
    parameters:
      - backup.*

# Entry points
entrypoints:
  default: states/init.yaml
  rollback: states/rollback.yaml

  # Named entry points for partial application
  nginx_only: states/nginx/init.yaml
  postgres_only: states/postgres/init.yaml
  upgrade: states/upgrade.yaml
  uninstall: states/uninstall.yaml

# Parameter schema
parameters:
  # Simple required parameter
  app_name:
    type: string
    required: true
    description: Application name used for services, directories, and identifiers
    pattern: "^[a-z][a-z0-9-]{2,30}$"
    examples:
      - my-web-app
      - api-service

  # Required with validation
  domain:
    type: string
    required: true
    description: Primary domain name for the application
    format: hostname

  # List parameter
  additional_domains:
    type: array
    items:
      type: string
      format: hostname
    default: []
    description: Additional domain names (aliases)
    maxItems: 10

  # Nested object parameters
  nginx:
    type: object
    description: NGINX reverse proxy configuration
    properties:
      worker_processes:
        type: integer
        default: auto
        description: Number of worker processes (or 'auto')
        minimum: 1
        maximum: 32

      client_max_body_size:
        type: string
        default: "10m"
        description: Maximum request body size
        pattern: "^[0-9]+[kmg]$"

      ssl:
        type: object
        description: SSL/TLS configuration
        properties:
          enabled:
            type: boolean
            default: true
          cert_path:
            type: string
            description: Path to SSL certificate
          key_path:
            type: string
            sensitive: true
            description: Path to SSL private key
          protocols:
            type: array
            items:
              type: string
              enum: [TLSv1.2, TLSv1.3]
            default: [TLSv1.2, TLSv1.3]

  postgres:
    type: object
    description: PostgreSQL database configuration
    properties:
      version:
        type: string
        default: "15"
        enum: ["14", "15", "16"]
        description: PostgreSQL major version

      database:
        type: string
        required: true
        description: Database name to create
        pattern: "^[a-z][a-z0-9_]{2,63}$"

      username:
        type: string
        required: true
        description: Database user to create
        pattern: "^[a-z][a-z0-9_]{2,30}$"

      password:
        type: string
        required: true
        sensitive: true
        description: Database user password
        source: secret  # Indicates this should come from secrets backend

      max_connections:
        type: integer
        default: 100
        minimum: 10
        maximum: 1000

      shared_buffers:
        type: string
        default: "256MB"
        description: PostgreSQL shared_buffers setting

  app:
    type: object
    description: Application runtime configuration
    properties:
      runtime:
        type: string
        required: true
        enum: [nodejs, python, ruby, go, java]
        description: Application runtime environment

      version:
        type: string
        required: true
        description: Runtime version (e.g., "20" for Node.js 20.x)

      port:
        type: integer
        default: 3000
        minimum: 1024
        maximum: 65535
        description: Application listen port

      instances:
        type: integer
        default: 1
        minimum: 1
        maximum: 16
        description: Number of application instances

      repo:
        type: string
        format: uri
        description: Git repository URL for application code

      branch:
        type: string
        default: main
        description: Git branch to deploy

      env:
        type: object
        description: Environment variables for the application
        additionalProperties:
          type: string
        examples:
          NODE_ENV: production
          LOG_LEVEL: info

  # Monitoring parameters (only if feature enabled)
  monitoring:
    type: object
    description: Monitoring configuration (requires monitoring feature)
    feature: monitoring
    properties:
      prometheus_port:
        type: integer
        default: 9090
      retention_days:
        type: integer
        default: 15

  # Backup parameters (only if feature enabled)
  backup:
    type: object
    description: Backup configuration (requires backup feature)
    feature: backup
    properties:
      schedule:
        type: string
        default: "0 2 * * *"
        description: Cron schedule for backups
      retention_count:
        type: integer
        default: 7
        description: Number of backups to retain
      destination:
        type: string
        required: true
        description: Backup destination path or URL

# Outputs - values exposed after application
outputs:
  nginx_config_path:
    description: Path to generated NGINX configuration
    value: "/etc/nginx/sites-available/{{ app_name }}"

  postgres_connection_string:
    description: PostgreSQL connection string
    sensitive: true
    value: "postgresql://{{ postgres.username }}:{{ postgres.password }}@localhost/{{ postgres.database }}"

  app_url:
    description: Application URL
    value: "https://{{ domain }}"

# Lifecycle hooks
hooks:
  pre_apply:
    - states/hooks/pre-apply.yaml
  post_apply:
    - states/hooks/post-apply.yaml
  pre_rollback:
    - states/hooks/pre-rollback.yaml
  post_rollback:
    - states/hooks/post-rollback.yaml
```

## Usage in States

### Basic Usage

```yaml
# /srv/states/production/web-app.yaml
include:
  - blueprint: blueprints/community/web-app-stack
    version: "^1.2"
    parameters:
      app_name: my-production-app
      domain: app.example.com

      nginx:
        worker_processes: 4
        ssl:
          enabled: true
          cert_path: /etc/ssl/certs/app.crt
          key_path: !secret ssl/app/private-key

      postgres:
        database: myapp_prod
        username: myapp
        password: !secret database/myapp/password
        max_connections: 200

      app:
        runtime: nodejs
        version: "20"
        port: 3000
        instances: 4
        repo: https://github.com/myorg/myapp.git
        branch: main
        env:
          NODE_ENV: production
          LOG_LEVEL: warn
```

### Multiple Blueprints with Dependencies

```yaml
# /srv/states/full-stack/init.yaml
include:
  # Base system hardening - runs first
  - blueprint: blueprints/community/security-baseline
    version: "^3.0"
    parameters:
      compliance_framework: cis-level1

  # SSL certificates - runs after security baseline
  - blueprint: blueprints/community/ssl-certificates
    version: "^2.0"
    parameters:
      provider: letsencrypt
      domains:
        - app.example.com
        - api.example.com
      email: admin@example.com

  # Monitoring stack - can run concurrently with web-app
  - blueprint: blueprints/community/prometheus-stack
    version: "^1.5"
    parameters:
      retention: 30d
      alertmanager:
        enabled: true
        slack_webhook: !secret monitoring/slack-webhook

  # Web application - depends on ssl-certificates completing first
  - blueprint: blueprints/community/web-app-stack
    version: "^1.2"
    features:
      ssl: true
      monitoring: true
      backup: true
    parameters:
      app_name: frontend
      domain: app.example.com
      # ... rest of parameters

# Custom states to run after blueprints
states:
  custom-firewall:
    firewall:
      - state: present
        port: 443
        proto: tcp
        source: 0.0.0.0/0
```

### Same Blueprint Multiple Times

```yaml
# /srv/states/multi-app/init.yaml
include:
  # Frontend application
  - blueprint: blueprints/community/web-app-stack
    version: "^1.2"
    as: frontend  # Namespace for this instance
    parameters:
      app_name: frontend
      domain: www.example.com
      app:
        runtime: nodejs
        version: "20"
        port: 3000

  # API application (separate instance)
  - blueprint: blueprints/community/web-app-stack
    version: "^1.2"
    as: api
    parameters:
      app_name: api
      domain: api.example.com
      app:
        runtime: python
        version: "3.11"
        port: 8000

  # Admin application (third instance)
  - blueprint: blueprints/community/web-app-stack
    version: "^1.2"
    as: admin
    parameters:
      app_name: admin
      domain: admin.example.com
      app:
        runtime: ruby
        version: "3.2"
        port: 3001
```

### Feature Toggles

```yaml
include:
  - blueprint: blueprints/community/web-app-stack
    version: "^1.2"
    features:
      ssl: true           # Enable SSL (default: true)
      monitoring: true    # Enable Prometheus exporters
      backup: false       # Disable backups
    parameters:
      # monitoring.* parameters only valid when monitoring: true
      monitoring:
        prometheus_port: 9100
        retention_days: 7
```

### Targeting Specific Entry Points

```yaml
include:
  # Only apply NGINX portion
  - blueprint: blueprints/community/web-app-stack
    version: "^1.2"
    entrypoint: nginx_only
    parameters:
      app_name: proxy-only
      domain: proxy.example.com
      nginx:
        # Only nginx parameters needed
```

## Secrets Integration

Blueprints use a backend-agnostic secrets reference:

```yaml
# In state file - reference secrets by path
parameters:
  postgres:
    password: !secret database/myapp/password
  app:
    env:
      API_KEY: !secret api/external-service/key
      JWT_SECRET: !secret app/jwt/secret
```

The secrets backend is configured at the agent/control-plane level:

```yaml
# /etc/keystone-core/agent.yaml
secrets:
  backend: vault  # or: kubernetes, encrypted-file, env

  vault:
    address: https://vault.example.com:8200
    auth_method: kubernetes
    role: kscore-agent
    mount_path: secret/data

  # OR kubernetes secrets
  kubernetes:
    namespace: kscore-secrets

  # OR encrypted local file
  encrypted_file:
    path: /etc/keystone-core/secrets.enc
    key_file: /etc/keystone-core/secrets.key
```

Blueprint manifests indicate which parameters should come from secrets:

```yaml
parameters:
  postgres:
    password:
      type: string
      required: true
      sensitive: true
      source: secret  # Hint that this should come from secrets backend
```

## Rollback Support

### Automatic Version Tracking

The control plane tracks applied blueprint versions per agent:

```
agent-001:
  blueprints/community/web-app-stack:
    current_version: 1.2.0
    applied_at: 2026-01-10T12:00:00Z
    previous_versions:
      - version: 1.1.0
        applied_at: 2026-01-05T10:00:00Z
        state_snapshot_id: snap-abc123
      - version: 1.0.0
        applied_at: 2025-12-20T14:00:00Z
        state_snapshot_id: snap-def456
```

### Rollback Commands

```bash
# Rollback to previous version
kscorectl blueprint rollback web-app-stack --target "agent-001"

# Rollback to specific version
kscorectl blueprint rollback web-app-stack --target "agent-001" --version 1.0.0

# Rollback with dry-run
kscorectl blueprint rollback web-app-stack --target "agent-001" --dry-run

# Rollback all agents running a blueprint
kscorectl blueprint rollback web-app-stack --target "role:web" --version 1.1.0
```

### Rollback States

Blueprints can define explicit rollback states:

```yaml
# states/rollback.yaml
rollback-nginx:
  service:
    - name: nginx
      state: stopped

rollback-postgres:
  service:
    - name: postgresql
      state: stopped
  file:
    - name: /var/lib/postgresql
      state: absent
      require:
        - service: postgresql
```

### Breaking Changes

Major version changes (1.x → 2.x) require explicit acknowledgment:

```bash
# This will fail
kscorectl state apply my-app.yaml
# Error: Blueprint web-app-stack upgrade from 1.2.0 to 2.0.0 is a breaking change.
# Breaking changes detected:
#   - Parameter 'postgres.password' moved to 'database.credentials.password'
#   - Parameter 'app.port' default changed from 3000 to 8080
#   - Removed parameter 'nginx.legacy_mode'
# Run with --accept-breaking-changes to proceed.

# Acknowledge and proceed
kscorectl state apply my-app.yaml --accept-breaking-changes

# Or install a specific version to upgrade
kscorectl blueprint install web-app-stack@2.0.0
```

## CLI Reference

### `kscore-blueprint` Plugin

```bash
# Blueprint Development
kscorectl blueprint init <name>              # Create new blueprint scaffold
kscorectl blueprint validate <path>          # Validate blueprint structure
kscorectl blueprint lint <path>              # Lint blueprint for best practices
kscorectl blueprint test <path>              # Run blueprint tests
kscorectl blueprint docs <path>              # Generate documentation

# Blueprint Registry
kscorectl blueprint search <query>           # Search blueprints
kscorectl blueprint info <blueprint>         # Show blueprint details
kscorectl blueprint versions <blueprint>     # List available versions
kscorectl blueprint install <blueprint>      # Install blueprint locally
kscorectl blueprint update                   # Update installed blueprints
kscorectl blueprint applied list              # List installed blueprints
kscorectl blueprint remove <blueprint>       # Remove installed blueprint

# Blueprint Publishing
kscorectl blueprint publish <path>           # Publish to registry
kscorectl blueprint sign <path>              # Sign blueprint
kscorectl blueprint verify <blueprint>       # Verify blueprint signature

# Blueprint Operations
kscorectl blueprint applied show <blueprint>  # Show application status
kscorectl blueprint rollback <blueprint>     # Rollback to previous version
kscorectl blueprint-state diff <blueprint>   # Diff current vs target version

# Blueprint Bundling (Air-gapped)
kscorectl blueprint bundle create <path>     # Bundle blueprint + deps
kscorectl blueprint bundle install <bundle>  # Install from bundle
```

### Init Command Output

```bash
$ kscorectl blueprint init my-blueprint
Creating blueprint scaffold at ./my-blueprint/

Created:
  ./my-blueprint/
  ├── blueprint.yaml          # Manifest - edit this first
  ├── README.md               # Documentation template
  ├── CHANGELOG.md            # Version history
  ├── LICENSE                 # Apache 2.0 license
  ├── states/
  │   └── init.yaml           # Main entry point
  ├── vars/
  │   └── defaults.yaml       # Default values
  ├── templates/
  │   └── .gitkeep
  ├── files/
  │   └── .gitkeep
  └── tests/
      └── test_basic.yaml     # Basic test template

Next steps:
  1. Edit blueprint.yaml to define parameters
  2. Add states to states/ directory
  3. Run 'kscorectl blueprint validate .' to check
  4. Run 'kscorectl blueprint test .' to test
  5. Run 'kscorectl blueprint publish .' to publish
```

## Testing Framework

### Test Structure

```yaml
# tests/test_nginx.yaml
test_nginx_installed:
  description: Verify NGINX is installed
  parameters:
    app_name: test-app
    domain: test.local
    # Minimal parameters for test
  assertions:
    - type: package_installed
      name: nginx
    - type: service_running
      name: nginx
    - type: file_exists
      path: /etc/nginx/sites-available/test-app
    - type: port_listening
      port: 80

test_nginx_ssl:
  description: Verify NGINX SSL configuration
  features:
    ssl: true
  parameters:
    app_name: test-ssl
    domain: test.local
    nginx:
      ssl:
        enabled: true
        cert_path: /tmp/test.crt
        key_path: /tmp/test.key
  setup:
    - create_test_certificate:
        path: /tmp/test.crt
        key_path: /tmp/test.key
  assertions:
    - type: port_listening
      port: 443
    - type: file_contains
      path: /etc/nginx/sites-available/test-ssl
      content: "ssl_certificate"
```

### Running Tests

```bash
# Run all tests locally
kscorectl blueprint test ./my-blueprint

# Run tests in container
kscorectl blueprint test ./my-blueprint --container debian:12

# Run specific test
kscorectl blueprint test ./my-blueprint --test test_nginx_ssl

# Run tests across platforms
kscorectl blueprint test ./my-blueprint --matrix debian:12,rhel:9,alpine:3.19

# Run tests with coverage
kscorectl blueprint test ./my-blueprint --coverage
```

### CI/CD Integration

```yaml
# .github/workflows/blueprint-test.yml
name: Blueprint Tests
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        platform: [debian:12, rhel:9, alpine:3.19]
    steps:
      - uses: actions/checkout@v4

      - name: Install kscorectl
        run: curl -sSL https://get.keystone-core.io | bash

      - name: Validate blueprint
        run: kscorectl blueprint validate .

      - name: Lint blueprint
        run: kscorectl blueprint lint .

      - name: Run tests
        run: kscorectl blueprint test . --container ${{ matrix.platform }}

  publish:
    needs: test
    if: github.ref == 'refs/heads/main'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Publish blueprint
        run: kscorectl blueprint publish .
        env:
          KSCORE_REGISTRY_TOKEN: ${{ secrets.REGISTRY_TOKEN }}
```

## Air-Gapped Support

### Bundle Creation

```bash
# Bundle a blueprint with all dependencies
kscorectl blueprint bundle create ./my-blueprint

# Bundle includes:
# - Blueprint files
# - All dependent blueprints (resolved versions)
# - All required modules
# - Checksums and signatures
# - Metadata for offline installation

# Bundle includes pinned versions from blueprint.lock if present
```

### Bundle Structure

```
my-blueprint-bundle.tar.gz
├── manifest.json           # Bundle manifest with checksums
├── blueprints/
│   ├── community/
│   │   ├── web-app-stack-1.2.0/
│   │   ├── ssl-certificates-2.0.1/
│   │   └── base-system-1.0.0/
├── modules/
│   ├── std/
│   │   ├── files-1.0.0/
│   │   └── exec-1.0.0/
│   └── community/
│       └── nginx-2.0.0/
└── signatures/
    ├── blueprints.sig
    └── modules.sig
```

### Offline Installation

```bash
# On air-gapped system
kscorectl blueprint bundle install my-blueprint-bundle.tar.gz

# Verify signatures before install
kscorectl blueprint verify my-blueprint-bundle.tar.gz

# Install to specific location
kscorectl blueprint bundle install my-blueprint-bundle.tar.gz \
  --blueprints-dir /srv/blueprints \
  --modules-dir /srv/modules
```

### Mirror Server

For organizations with many air-gapped systems:

```bash
# Sync specific blueprints to local mirror
kscorectl blueprint mirror sync blueprints/community/web-app-stack@^1.0

# Export mirror for transfer
kscorectl blueprint mirror export --output mirror-export.tar.gz

# On air-gapped system - import and serve
kscorectl blueprint mirror import mirror-export.tar.gz
kscorectl blueprint mirror serve --listen :8080
```

## Implementation Phases

### Phase 1: Core Infrastructure (Weeks 1-3)
**Goal**: Basic blueprint loading and execution

Tasks:
- T1.1: Blueprint manifest parser (`pkg/blueprint/manifest.go`)
- T1.2: Blueprint validator (`pkg/blueprint/validator.go`)
- T1.3: Blueprint loader (`pkg/blueprint/loader.go`)
- T1.4: Blueprint storage interface (`pkg/blueprint/storage.go`)
- T1.5: Local filesystem storage (`pkg/blueprint/storage_local.go`)
- T1.6: State integration - `include: blueprint:` syntax
- T1.7: Basic parameter injection into states

Deliverables:
- Can load and validate blueprints from local filesystem
- Can include blueprints in state files
- Basic parameter substitution works

### Phase 2: Parameter System (Weeks 4-5)
**Goal**: Robust parameter validation and handling

Tasks:
- T2.1: JSON Schema-based parameter validation
- T2.2: Type coercion (string→int, etc.)
- T2.3: Default value handling with platform overrides
- T2.4: Sensitive parameter marking and masking
- T2.5: Parameter documentation generation
- T2.6: `!secret` tag integration with secrets backend
- T2.7: Parameter inheritance and override precedence

Deliverables:
- Full parameter schema validation
- Secrets integration working
- Platform-specific defaults

### Phase 3: Dependency Resolution (Weeks 6-7)
**Goal**: Blueprint dependencies and execution ordering

Tasks:
- T3.1: Dependency graph construction
- T3.2: `requires` (soft) dependency handling
- T3.3: `requires_before` (hard) dependency handling
- T3.4: Circular dependency detection
- T3.5: Multi-instance (`as:` namespace) support
- T3.6: Feature flag evaluation
- T3.7: Conditional state inclusion

Deliverables:
- Blueprints can depend on other blueprints
- Execution order respects dependency hints
- Feature toggles work

### Phase 4: Registry Integration (Weeks 8-9)
**Goal**: Publish and install blueprints from registry

Tasks:
- T4.1: Registry client for blueprints namespace
- T4.2: Blueprint publishing workflow
- T4.3: Version constraint resolution
- T4.4: Blueprint signing (cosign)
- T4.5: Signature verification
- T4.6: Blueprint search and discovery
- T4.7: Blueprint metadata indexing

Deliverables:
- Can publish blueprints to registry
- Can install blueprints from registry
- Signatures verified on install

### Phase 5: CLI Plugin (Weeks 10-11)
**Goal**: Full `kscore-blueprint` CLI

Tasks:
- T5.1: `blueprint init` scaffolding
- T5.2: `blueprint validate` command
- T5.3: `blueprint lint` with best practices
- T5.4: `blueprint search/info/versions` commands
- T5.5: `blueprint install/update/remove` commands
- T5.6: `blueprint publish/sign/verify` commands
- T5.7: `blueprint docs` documentation generator

Deliverables:
- Complete CLI for blueprint development
- Complete CLI for blueprint consumption

### Phase 6: Rollback Support (Weeks 12-13)
**Goal**: Version tracking and rollback capability

Tasks:
- T6.1: Applied blueprint version tracking (per agent)
- T6.2: State snapshot capture before apply
- T6.3: `blueprint rollback` command
- T6.4: Rollback entry point execution
- T6.5: Breaking change detection
- T6.6: Migration guide generation
- T6.7: `--accept-breaking-changes` workflow

Deliverables:
- Can track what blueprint versions are applied
- Can rollback to previous versions
- Breaking changes require acknowledgment

### Phase 7: Testing Framework (Weeks 14-15)
**Goal**: Blueprint testing and CI/CD support

Tasks:
- T7.1: Test file parser
- T7.2: Assertion types (package, service, file, port, etc.)
- T7.3: Container-based test execution
- T7.4: Multi-platform matrix testing
- T7.5: Test coverage reporting
- T7.6: CI/CD integration examples
- T7.7: `blueprint test` command

Deliverables:
- Can test blueprints locally and in containers
- CI/CD pipeline examples
- Coverage reporting

### Phase 8: Air-Gapped Support (Weeks 16-17)
**Goal**: Offline/disconnected environment support

Tasks:
- T8.1: Bundle format specification
- T8.2: `blueprint bundle` command
- T8.3: `blueprint bundle-install` command
- T8.4: Bundle signature verification
- T8.5: Mirror server implementation
- T8.6: `mirror sync/export/import` commands
- T8.7: Agent offline mode for blueprints

Deliverables:
- Can bundle blueprints for offline use
- Can run mirror server
- Agents work in disconnected mode

### Phase 9: Documentation & Examples (Week 18)
**Goal**: Comprehensive documentation and example blueprints

Tasks:
- T9.1: Blueprint developer guide
- T9.2: Blueprint user guide
- T9.3: Best practices guide
- T9.4: Example: LAMP stack blueprint
- T9.5: Example: Monitoring stack blueprint
- T9.6: Example: Security baseline blueprint
- T9.7: Community contribution guidelines

Deliverables:
- Complete documentation
- 3+ example blueprints
- Contribution guidelines

## Success Criteria

1. **Functionality**
   - [ ] Blueprints can be created, validated, and published
   - [ ] Blueprints can be installed and applied to agents
   - [ ] Parameter validation catches errors early
   - [ ] Dependencies are resolved correctly
   - [ ] Rollback works reliably

2. **Performance**
   - [ ] Blueprint resolution < 1 second for typical dependency trees
   - [ ] No significant overhead vs raw state application
   - [ ] Bundle creation < 30 seconds for complex blueprints

3. **Security**
   - [ ] All published blueprints are signed
   - [ ] Signatures are verified on installation
   - [ ] Sensitive parameters are never logged
   - [ ] Secrets are fetched from configured backend

4. **Usability**
   - [ ] New blueprint can be created in < 5 minutes
   - [ ] Complex stack deployable with < 50 lines of configuration
   - [ ] Error messages are clear and actionable
   - [ ] Documentation is comprehensive

5. **Community**
   - [ ] At least 5 official blueprints available at launch
   - [ ] Contribution process is documented and tested
   - [ ] Blueprint registry is public and searchable

## Dependencies

- **Epic 3** (State Management) - Blueprints compose states
- **Epic 9** (Module System) - Blueprints can require modules, share registry
- **Epic 16** (Stdlib Modules) - Blueprints use standard state modules

## Risks & Mitigations

| Risk | Impact | Likelihood | Mitigation |
|------|--------|------------|------------|
| Parameter schema too complex | High | Medium | Start simple, add complexity gradually |
| Dependency resolution slow | Medium | Low | Use efficient graph algorithms, cache results |
| Breaking changes cause outages | High | Medium | Require explicit acknowledgment, provide migration tools |
| Air-gapped bundles too large | Medium | Medium | Implement delta bundles, compression |
| Community adoption slow | Medium | Medium | Provide high-quality official blueprints |

## Open Questions

1. Should blueprints support "inheritance" (blueprint extends another)?
2. How do we handle blueprint conflicts when two blueprints try to manage the same resource?
3. Should there be a "blueprint store" UI similar to Helm Hub?
4. How do we version the blueprint API itself (apiVersion field)?

## Appendix: Example Blueprints

### A. Monitoring Stack

```yaml
apiVersion: blueprints.keystone-core.io/v1
kind: Blueprint
metadata:
  name: prometheus-stack
  version: 1.0.0
  description: Complete monitoring stack with Prometheus, Grafana, and Alertmanager

parameters:
  prometheus:
    retention:
      type: string
      default: "15d"
    scrape_interval:
      type: string
      default: "15s"

  grafana:
    admin_password:
      type: string
      required: true
      sensitive: true
      source: secret
    anonymous_access:
      type: boolean
      default: false

  alertmanager:
    enabled:
      type: boolean
      default: true
    slack_webhook:
      type: string
      sensitive: true
      source: secret
```

### B. Security Baseline

```yaml
apiVersion: blueprints.keystone-core.io/v1
kind: Blueprint
metadata:
  name: security-baseline
  version: 3.0.0
  description: CIS-compliant security hardening

parameters:
  compliance_framework:
    type: string
    enum: [cis-level1, cis-level2, stig]
    default: cis-level1

  ssh:
    permit_root_login:
      type: boolean
      default: false
    password_authentication:
      type: boolean
      default: false
    allowed_users:
      type: array
      items:
        type: string

  firewall:
    default_policy:
      type: string
      enum: [allow, deny]
      default: deny
    allowed_ports:
      type: array
      items:
        type: integer
      default: [22, 80, 443]
```

### C. Kubernetes Prerequisites

```yaml
apiVersion: blueprints.keystone-core.io/v1
kind: Blueprint
metadata:
  name: k8s-node-prep
  version: 1.0.0
  description: Prepare node for Kubernetes cluster membership

parameters:
  kubernetes_version:
    type: string
    default: "1.29"

  container_runtime:
    type: string
    enum: [containerd, crio]
    default: containerd

  cni_plugin:
    type: string
    enum: [calico, flannel, cilium]
    default: calico

  node_role:
    type: string
    enum: [control-plane, worker]
    required: true
```
