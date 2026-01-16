---
title: "Blueprint Reference"
weight: 12
description: >
  Complete API reference for Keystone Core blueprints including manifest format, CLI commands, parameter schema, and testing framework.
---

## Overview

Blueprints are pre-packaged, reusable collections of states that can be shared, versioned, and composed to deploy complex infrastructure stacks. This reference covers the blueprint manifest format, CLI commands, parameter system, and testing framework.

## Standard Blueprint Catalog (Epic 28)

Official blueprints follow the `kscore/<name>` naming convention:

- **Core**: `kscore/demo`, `kscore/production-cluster`, `kscore/enterprise-platform`
- **Infrastructure**: `kscore/nats-cluster`, `kscore/postgres-ha`
- **Observability**: `kscore/monitoring-stack`, `kscore/metrics-only`
- **Security**: `kscore/security-baseline`, `kscore/identity-federation`
- **Integrations**: `kscore/gitops-integration`, `kscore/proxy-agents`, `kscore/file-distribution`
- **Platform**: `kscore/kubernetes-operator`, `kscore/edge-deployment`

These blueprints are designed to be composed and parameterized rather than forked.

## Parameter Conventions

Standard parameters use consistent naming across blueprints:

- Cluster: `cluster_name`, `node_role`, `node_labels`, `regions`
- NATS: `nats_mode`, `nats_urls`, `nats_creds_file`
- PostgreSQL: `postgres_host`, `postgres_port`, `postgres_database`, `postgres_user`, `postgres_password`
- TLS: `tls_mode`, `tls_cert`, `tls_key`, `ca_cert`, `tls_csr`
- Backups: `backup_enabled`, `backup_destination`
- Identity: `identity_provider`, `federation_domains`, `oidc_issuer`

Secrets should be declared with `sensitive: true` and `source: secret`.

## Blueprint Manifest (blueprint.yaml)

The blueprint manifest defines metadata, parameters, dependencies, and entry points.

### Complete Structure

```yaml
apiVersion: blueprints.kscore.io/v1
kind: Blueprint

metadata:
  name: vendor/blueprint-name
  version: "1.0.0"
  description: Short description
  maintainers:
    - name: Maintainer Name
      email: maintainer@example.com
      url: https://example.com
  license: Apache-2.0
  repository: https://github.com/vendor/blueprint-name
  documentation: https://docs.example.com
  keywords:
    - web
    - nginx
  categories:
    - web-server
    - infrastructure

compatibility:
  kscore: ">=1.5.0"
  modules:
    std/files: ">=1.0.0"
  platforms:
    - os: linux
      family: debian
      versions: ["20.04", "22.04"]
      arch: ["amd64", "arm64"]
    - os: linux
      family: rhel
      versions: ["8", "9"]

dependencies:
  requires:
    - name: vendor/common
      version: ">=1.0.0"
  requires_before:
    - name: vendor/base-config
      version: "^2.0.0"

features:
  ssl:
    description: Enable SSL/TLS support
    default: true
    enables:
      - states/ssl.yaml
    requires:
      - vendor/certificates
    parameters:
      - ssl_cert_path
      - ssl_key_path

parameters:
  domain:
    type: string
    description: Domain name for the web server
    required: true
    format: hostname
  port:
    type: integer
    description: HTTP port
    default: 80
    minimum: 1
    maximum: 65535
  db_password:
    type: string
    description: Database password
    sensitive: true
    source: secret

entrypoints:
  default: states/init.yaml
  install: states/install.yaml
  configure: states/configure.yaml
  rollback: states/rollback.yaml

outputs:
  server_url:
    value: "http://{{ .params.domain }}:{{ .params.port }}"
    description: URL of the deployed server
  config_path:
    value: /etc/myapp/config.yaml
    description: Path to configuration file

hooks:
  pre_apply:
    - states/backup-config.yaml
  post_apply:
    - states/verify-deployment.yaml
  pre_rollback:
    - states/stop-services.yaml
  post_rollback:
    - states/restore-config.yaml
```

### Metadata Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Blueprint name (`vendor/name` format) |
| `version` | string | Yes | Semantic version (major.minor.patch) |
| `description` | string | Yes | Short description of the blueprint |
| `maintainers` | array | No | List of maintainers with name, email, url |
| `license` | string | No | SPDX license identifier |
| `repository` | string | No | Source repository URL |
| `documentation` | string | No | Documentation URL |
| `keywords` | array | No | Keywords for search |
| `categories` | array | No | Categories for organization |

### Compatibility Fields

| Field | Type | Description |
|-------|------|-------------|
| `kscore` | string | Keystone Core version constraint |
| `modules` | map | Required modules with version constraints |
| `platforms` | array | Supported platform configurations |

**Platform Configuration:**

```yaml
platforms:
  - os: linux           # Operating system
    family: debian      # OS family
    versions: ["22.04"] # Specific versions
    arch: ["amd64"]     # CPU architectures
```

### Parameter Schema

Parameters use JSON Schema-like validation:

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | `string`, `integer`, `number`, `boolean`, `array`, `object` |
| `description` | string | Parameter description |
| `required` | boolean | Whether parameter is required |
| `default` | any | Default value |
| `sensitive` | boolean | Mark as sensitive (not logged) |
| `source` | string | Value source (`secret` for secret backend) |
| `feature` | string | Restrict to specific feature |

### Parameter Files and Overrides

Parameter values can be supplied via a YAML file and merged with defaults
defined in the blueprint `vars/` directory.

Example `params.yaml`:

```yaml
parameters:
  domain: app.example.com
  port: 8080
  db_password: !secret databases/postgres/admin
```

Use `--params` with commands that need parameter values:

```bash
kscore-blueprint validate ./my-blueprint --params params.yaml
```

**String Validation:**

```yaml
parameters:
  domain:
    type: string
    pattern: "^[a-z0-9.-]+$"
    format: hostname
    minLength: 3
    maxLength: 253
```

**Numeric Validation:**

```yaml
parameters:
  port:
    type: integer
    minimum: 1
    maximum: 65535
```

**Enum Validation:**

```yaml
parameters:
  environment:
    type: string
    enum:
      - development
      - staging
      - production
```

**Array Validation:**

```yaml
parameters:
  allowed_ips:
    type: array
    items:
      type: string
      format: ipv4
    minItems: 1
    maxItems: 100
```

**Object Validation:**

```yaml
parameters:
  database:
    type: object
    properties:
      host:
        type: string
        format: hostname
      port:
        type: integer
        default: 5432
    additionalProperties: false
```

**Format Validators:**

| Format | Description |
|--------|-------------|
| `hostname` | Valid hostname |
| `uri` | Valid URI |
| `email` | Valid email address |
| `ipv4` | IPv4 address |
| `ipv6` | IPv6 address |
| `cidr` | CIDR notation |
| `date-time` | ISO 8601 date-time |
| `uuid` | UUID |
| `port` | Valid port number (1-65535) |
| `semver` | Semantic version |
| `dns-name` | DNS name |

### Dependency Types

**Soft Dependencies (concurrent execution):**

```yaml
dependencies:
  requires:
    - name: vendor/logging
      version: ">=1.0.0"
    - name: vendor/monitoring
      version: "^2.0.0"
```

**Hard Dependencies (sequential execution):**

```yaml
dependencies:
  requires_before:
    - name: vendor/base-config
      version: ">=1.0.0"
```

**Version Constraint Syntax:**

| Syntax | Example | Description |
|--------|---------|-------------|
| Exact | `1.2.3` | Exactly version 1.2.3 |
| Greater | `>1.2.3` | Greater than 1.2.3 |
| Greater or equal | `>=1.2.3` | Greater than or equal to 1.2.3 |
| Less | `<2.0.0` | Less than 2.0.0 |
| Less or equal | `<=2.0.0` | Less than or equal to 2.0.0 |
| Caret | `^1.2.3` | Compatible with 1.2.3 (>=1.2.3 <2.0.0) |
| Tilde | `~1.2.3` | Patch-level changes (>=1.2.3 <1.3.0) |
| Range | `>=1.0.0 <2.0.0` | Combined constraints |

## Using Blueprints in States

### Include Syntax

```yaml
include:
  - blueprint: blueprints/vendor/web-stack@1.2.0
    parameters:
      domain: example.com
      port: 8080

  # With namespace for multi-instance
  - blueprint: blueprints/vendor/database@2.0.0
    as: primary_db
    parameters:
      name: primary

  - blueprint: blueprints/vendor/database@2.0.0
    as: replica_db
    parameters:
      name: replica
      readonly: true

  # With features
  - blueprint: blueprints/vendor/web-stack@1.2.0
    features:
      ssl: true
      monitoring: false
    parameters:
      domain: secure.example.com

  # With custom entrypoint
  - blueprint: blueprints/vendor/app@1.0.0
    entrypoint: configure
    parameters:
      config_only: true

  # With secrets
  - blueprint: blueprints/vendor/database@2.0.0
    parameters:
      admin_password: !secret databases/prod/admin
```

### Include Fields

| Field | Type | Description |
|-------|------|-------------|
| `blueprint` | string | Blueprint name with version constraint |
| `as` | string | Namespace for multi-instance support |
| `entrypoint` | string | Named entry point (default: `default`) |
| `features` | map | Feature toggles |
| `parameters` | map | Parameter values |

## CLI Reference

### Development Commands

**Initialize New Blueprint:**

```bash
kscore-blueprint init vendor/my-blueprint \
  --description "My blueprint description" \
  --author "Your Name" \
  --email "you@example.com" \
  --license "Apache-2.0" \
  --category "web-server" \
  --keywords "nginx,web"
```

**Validate Manifest:**

```bash
kscore-blueprint validate ./my-blueprint

# Validate with parameter values
kscore-blueprint validate ./my-blueprint --params params.yaml
```

**Lint for Best Practices:**

```bash
kscore-blueprint lint ./my-blueprint

# Strict mode (warnings as errors)
kscore-blueprint lint --strict ./my-blueprint
```

**Generate Documentation:**

```bash
kscore-blueprint docs ./my-blueprint

# Output to file
kscore-blueprint docs ./my-blueprint -o README.md
```

### Discovery Commands

**Search Registry:**

```bash
kscore-blueprint search nginx

# Search with filters
kscore-blueprint search web --category web-server --min-version 1.0.0
```

**Show Blueprint Info:**

```bash
kscore-blueprint info vendor/nginx

# Show specific version
kscore-blueprint info vendor/nginx@1.2.0
```

**List Versions:**

```bash
kscore-blueprint versions vendor/nginx
```

### Management Commands

**Install Blueprint:**

```bash
kscore-blueprint install vendor/nginx@1.2.0

# Install multiple
kscore-blueprint install vendor/nginx vendor/mysql

# Custom registry
kscore-blueprint install vendor/nginx --registry https://registry.example.com

# Without dependencies
kscore-blueprint install vendor/nginx --no-deps

# Dry run
kscore-blueprint install vendor/nginx --dry-run
```

**Update Blueprints:**

```bash
# Update specific blueprint
kscore-blueprint update vendor/nginx

# Update all
kscore-blueprint update --all

# Dry run
kscore-blueprint update --all --dry-run
```

**Remove Blueprint:**

```bash
kscore-blueprint remove vendor/nginx

# Force remove (ignore dependencies)
kscore-blueprint remove vendor/nginx --force
```

**Rollback Blueprint:**

```bash
# Rollback to previous version
kscore-blueprint rollback vendor/nginx

# Rollback to specific version
kscore-blueprint rollback vendor/nginx --version 1.1.0

# Rollback to snapshot
kscore-blueprint rollback vendor/nginx --snapshot snap-123456

# Accept breaking changes
kscore-blueprint rollback vendor/nginx --accept-breaking-changes
```

### Distribution Commands

**Publish to Registry:**

```bash
kscore-blueprint publish ./my-blueprint

# Custom registry
kscore-blueprint publish ./my-blueprint --registry https://registry.example.com

# Dry run
kscore-blueprint publish ./my-blueprint --dry-run
```

**Sign Blueprint:**

```bash
# Sign with key file
kscore-blueprint sign ./my-blueprint --key cosign.key

# Keyless signing (OIDC)
kscore-blueprint sign ./my-blueprint --keyless

# Custom output
kscore-blueprint sign ./my-blueprint --key cosign.key -o signature.sig
```

**Verify Signature:**

```bash
kscore-blueprint verify vendor/nginx

# With specific key
kscore-blueprint verify vendor/nginx --key cosign.pub

# Strict mode (fail on missing signature)
kscore-blueprint verify vendor/nginx --strict
```

### Testing Commands

**Run Tests:**

```bash
kscore-blueprint test ./my-blueprint

# Verbose output
kscore-blueprint test ./my-blueprint -v

# Run specific tests by tag
kscore-blueprint test ./my-blueprint --tags integration

# Exclude tags
kscore-blueprint test ./my-blueprint --exclude-tags slow

# Pattern matching
kscore-blueprint test ./my-blueprint --pattern "test_install*"

# Parallel execution
kscore-blueprint test ./my-blueprint --parallel --max-parallel 4

# Output format
kscore-blueprint test ./my-blueprint --format json -o results.json
kscore-blueprint test ./my-blueprint --format junit -o junit.xml

# Dry run
kscore-blueprint test ./my-blueprint --dry-run
```

### Snapshot Commands

**List Snapshots:**

```bash
kscore-blueprint snapshot list vendor/nginx
```

**Show Snapshot Details:**

```bash
kscore-blueprint snapshot show vendor/nginx snap-123456
```

**Delete Snapshot:**

```bash
kscore-blueprint snapshot delete vendor/nginx snap-123456
```

## Directory Structure

```
my-blueprint/
├── blueprint.yaml         # Manifest (required)
├── README.md             # Documentation
├── CHANGELOG.md          # Version history
├── LICENSE               # License file
├── states/
│   ├── init.yaml         # Default entry point
│   ├── install.yaml      # Install entry point
│   ├── configure.yaml    # Configure entry point
│   └── rollback.yaml     # Rollback entry point
├── vars/
│   └── defaults.yaml     # Default parameter values
├── templates/            # Template files
│   └── config.yaml.tpl
├── files/                # Static files
│   └── scripts/
│       └── setup.sh
└── tests/
    ├── test_basic.yaml   # Basic tests
    └── test_full.yaml    # Full integration tests
```

## Testing Framework

### Test Suite Format

```yaml
name: my-blueprint-tests
description: Test suite for my-blueprint

setup:
  - states/test-setup.yaml

teardown:
  - states/test-cleanup.yaml

tests:
  - name: test_basic_install
    description: Test basic installation
    tags:
      - basic
      - install
    parameters:
      domain: test.example.com
    assertions:
      - type: state
        state_id: install_package
        expected: changed

  - name: test_config_file
    description: Verify configuration file
    assertions:
      - type: file
        path: /etc/myapp/config.yaml
        exists: true
        mode: "0644"
        contains: "domain: test.example.com"

  - name: test_service_running
    description: Verify service is running
    assertions:
      - type: service
        name: myapp
        running: true
        enabled: true
```

### Assertion Types

| Type | Description |
|------|-------------|
| `state` | Assert state result (changed, unchanged, failed) |
| `file` | Assert file exists, mode, owner, contents |
| `directory` | Assert directory exists, mode, owner |
| `command` | Assert command exit code, output |
| `output` | Assert command output matches pattern |
| `service` | Assert service running, enabled |
| `package` | Assert package installed, version |
| `user` | Assert user exists, groups |
| `group` | Assert group exists, members |
| `idempotency` | Assert re-run produces no changes |

### File Assertion

```yaml
assertions:
  - type: file
    path: /etc/myapp/config.yaml
    exists: true
    mode: "0644"
    owner: root
    group: root
    contains: "expected content"
    not_contains: "bad content"
    matches: "^domain: .*$"
```

### Command Assertion

```yaml
assertions:
  - type: command
    command: myapp --version
    exit_code: 0
    stdout_contains: "1.0.0"
    stderr_is_empty: true
```

### Idempotency Assertion

```yaml
assertions:
  - type: idempotency
    reruns: 3
    expect_no_changes: true
```

## Bundle System (Air-Gapped)

### Creating Bundles

```bash
# Create bundle from blueprint
kscore-blueprint bundle create vendor/nginx@1.2.0 -o nginx-bundle.tar.gz

# Include all dependencies
kscore-blueprint bundle create vendor/nginx@1.2.0 --include-deps -o nginx-bundle.tar.gz

# Sign bundle
kscore-blueprint bundle create vendor/nginx@1.2.0 --sign --key cosign.key -o nginx-bundle.tar.gz
```

### Bundle Manifest

```yaml
format: "1.0"
created_at: "2024-01-15T10:00:00Z"
created_by: "admin"
root_blueprint: "vendor/nginx"
root_version: "1.2.0"
description: "Nginx blueprint bundle"

blueprints:
  - name: vendor/nginx
    version: "1.2.0"
    checksum: "sha256:abc123..."
  - name: vendor/common
    version: "1.0.0"
    checksum: "sha256:def456..."

files:
  - path: vendor/nginx/1.2.0/blueprint.yaml
    checksum: "sha256:..."
    size: 1234

signatures:
  - signer: "admin@example.com"
    signature: "base64..."
    timestamp: "2024-01-15T10:00:00Z"

checksum: "sha256:overall..."
```

### Installing from Bundle

```bash
# Install from bundle file
kscore-blueprint bundle install nginx-bundle.tar.gz

# Verify signatures
kscore-blueprint bundle install nginx-bundle.tar.gz --verify --key cosign.pub
```

## Mirror Server

### Configuration

```yaml
# mirror-config.yaml
storage_dir: /var/lib/kscore/blueprints
listen_addr: ":8080"
upstream_url: https://registry.kscore.io
sync_interval: 1h
allow_push: true
require_signatures: true
trusted_keys:
  - /etc/kscore/keys/trusted.pub
```

### Running Mirror Server

```bash
# Start mirror server
kscore-blueprint mirror serve --config mirror-config.yaml

# Sync from upstream
kscore-blueprint mirror sync

# Export to directory
kscore-blueprint mirror export --output /backup/blueprints

# Import from directory
kscore-blueprint mirror import --input /backup/blueprints
```

### Mirror API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/v1/blueprints` | GET | List blueprints |
| `/v1/blueprints/{name}` | GET | Get blueprint metadata |
| `/v1/blueprints/{name}/{version}` | GET | Download blueprint |
| `/v1/blueprints/{name}/{version}` | PUT | Upload blueprint |
| `/v1/bundles/{name}/{version}` | GET | Download bundle |
| `/v1/bundles/{name}/{version}` | PUT | Upload bundle |
| `/health` | GET | Health check |
| `/index` | GET | Full index |

## Secrets Integration

### Using Secrets in Parameters

```yaml
# In blueprint.yaml
parameters:
  db_password:
    type: string
    sensitive: true
    source: secret

# In state file using the blueprint
include:
  - blueprint: vendor/database@1.0.0
    parameters:
      db_password: !secret databases/prod/password
```

### Secret Backend Configuration

```yaml
# In agent config
secrets:
  backend: vault
  vault:
    address: https://vault.example.com
    auth:
      method: kubernetes
      role: kscore-agent
```

## Breaking Change Detection

When upgrading blueprints, Keystone Core detects breaking changes:

| Change Type | Severity | Description |
|-------------|----------|-------------|
| Parameter removed | Breaking | Required parameter no longer exists |
| Type changed | Breaking | Parameter type changed |
| Required added | Breaking | New required parameter |
| Enum reduced | Breaking | Enum values removed |
| Default removed | Breaking | Default value removed from required param |
| Parameter added | Addition | New optional parameter |
| Default changed | Modification | Default value changed |
| Description changed | Modification | Documentation updated |

**Handling Breaking Changes:**

```bash
# Upgrade with breaking changes
kscore-blueprint update vendor/nginx --accept-breaking-changes

# Review changes first
kscore-blueprint info vendor/nginx@2.0.0 --show-breaking-changes
```

## See Also

- [Blueprints Concept](/docs/concepts/blueprints/) - Blueprint architecture and usage
- [State Management](/docs/concepts/state-management/) - Understanding states
- [Modules Reference](/docs/reference/modules/) - State module reference
- [CLI Reference](/docs/reference/cli/) - General CLI reference
