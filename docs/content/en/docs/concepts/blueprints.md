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
        │   └── app.conf.j2
        ├── tests/              # Blueprint test definitions
        │   └── integration_test.yaml
        └── README.md           # Blueprint documentation
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
    value: "https://{{ parameters.domain }}"
  admin_url:
    description: Admin panel URL
    value: "https://{{ parameters.domain }}/admin"

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
    - version: "{{ parameters.nginx_version | default('latest') }}"

nginx_config:
  file.managed:
    - name: /etc/nginx/nginx.conf
    - source: template://templates/nginx.conf.j2
    - template: jinja
    - context:
        domain: "{{ parameters.domain }}"
        port: "{{ parameters.port }}"
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

Templates use Jinja2 syntax for dynamic configuration:

```jinja
{# templates/nginx.conf.j2 #}
worker_processes auto;

events {
    worker_connections 1024;
}

http {
    server {
        listen {{ port }};
        server_name {{ domain }};

        {% if features.ssl %}
        listen 443 ssl;
        ssl_certificate /etc/ssl/certs/{{ domain }}.crt;
        ssl_certificate_key /etc/ssl/private/{{ domain }}.key;
        {% endif %}

        location / {
            proxy_pass http://127.0.0.1:{{ app_port | default(8080) }};
        }

        {% if features.monitoring %}
        location /metrics {
            stub_status on;
            allow 127.0.0.1;
            deny all;
        }
        {% endif %}
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
