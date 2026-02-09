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

## Catalog Reference

See [Blueprint Catalog](/docs/reference/blueprints-catalog/) for the official blueprint list and usage notes.

## Blueprint Manifest (blueprint.yaml)

The blueprint manifest defines metadata, parameters, dependencies, and entry points.

### Complete Structure

```yaml
apiVersion: blueprints.keystone-core.io/v1
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
    - "std/files@>=1.0.0"
  platforms:
    - os: linux
      family: debian
      versions: ["20.04", "22.04"]
      arch: amd64
    - os: linux
      family: debian
      versions: ["20.04", "22.04"]
      arch: arm64
    - os: linux
      family: rhel
      versions: ["8", "9"]
      arch: amd64

dependencies:
  requires:
    - "vendor/common@>=1.0.0"
  requires_before:
    - "vendor/base-config@^2.0.0"

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
| `modules` | array | Required modules with version constraints (format: `"module@version"`) |
| `platforms` | array | Supported platform configurations |

**Platform Configuration:**

```yaml
platforms:
  - os: linux           # Operating system
    family: debian      # OS family
    versions: ["22.04"] # Specific versions
    arch: amd64         # CPU architecture (single value)
```

**Note:** To support multiple architectures, add separate platform entries for each arch.

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
kscorectl blueprint validate ./my-blueprint --params params.yaml
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
    - "vendor/logging@>=1.0.0"
    - "vendor/monitoring@^2.0.0"
```

**Hard Dependencies (sequential execution):**

```yaml
dependencies:
  requires_before:
    - "vendor/base-config@>=1.0.0"
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
kscorectl blueprint init vendor/my-blueprint \
  --description "My blueprint description" \
  --author "Your Name" \
  --email "you@example.com" \
  --license "Apache-2.0" \
  --category "web-server" \
  --keywords "nginx,web"
```

**Validate Manifest:**

```bash
kscorectl blueprint validate ./my-blueprint

# Validate with parameter values
kscorectl blueprint validate ./my-blueprint --params params.yaml
```

**Lint for Best Practices:**

```bash
kscorectl blueprint lint ./my-blueprint

# Strict mode (warnings as errors)
kscorectl blueprint lint --strict ./my-blueprint
```

**Generate Documentation:**

```bash
kscorectl blueprint docs ./my-blueprint

# Output to file
kscorectl blueprint docs ./my-blueprint -o README.md
```

### Discovery Commands

**Search Registry:**

```bash
kscorectl blueprint search nginx

# Search with filters
kscorectl blueprint search web --category web-server --min-version 1.0.0
```

**Show Blueprint Info:**

```bash
kscorectl blueprint info vendor/nginx

# Show specific version
kscorectl blueprint info vendor/nginx@1.2.0
```

**List Versions:**

```bash
kscorectl blueprint versions vendor/nginx
```

### Management Commands

**Install Blueprint:**

```bash
kscorectl blueprint install vendor/nginx@1.2.0

# Install multiple
kscorectl blueprint install vendor/nginx vendor/mysql

# Custom registry
kscorectl blueprint install vendor/nginx --registry https://registry.example.com

# Without dependencies
kscorectl blueprint install vendor/nginx --no-deps

# Dry run
kscorectl blueprint install vendor/nginx --dry-run
```

**Update Blueprints:**

```bash
# Update specific blueprint
kscorectl blueprint update vendor/nginx

# Update all
kscorectl blueprint update --all

# Dry run
kscorectl blueprint update --all --dry-run
```

**Remove Blueprint:**

```bash
kscorectl blueprint remove vendor/nginx

# Force remove (ignore dependencies)
kscorectl blueprint remove vendor/nginx --force
```

**Rollback Blueprint:**

```bash
# Rollback to previous version
kscorectl blueprint rollback vendor/nginx

# Rollback to specific version
kscorectl blueprint rollback vendor/nginx --version 1.1.0

# Rollback to snapshot
kscorectl blueprint rollback vendor/nginx --snapshot snap-123456

# Accept breaking changes
kscorectl blueprint rollback vendor/nginx --accept-breaking-changes
```

### Distribution Commands

**Publish to Registry:**

```bash
kscorectl blueprint publish ./my-blueprint

# Custom registry
kscorectl blueprint publish ./my-blueprint --registry https://registry.example.com

# Dry run
kscorectl blueprint publish ./my-blueprint --dry-run
```

**Sign Blueprint:**

```bash
# Sign with key file
kscorectl blueprint sign ./my-blueprint --key cosign.key

# Keyless signing (CI/CD only - requires OIDC token in environment)
# Set SIGSTORE_ID_TOKEN or use GitHub Actions/GitLab CI built-in OIDC
kscorectl blueprint sign ./my-blueprint --keyless

# Custom output
kscorectl blueprint sign ./my-blueprint --key cosign.key -o signature.sig
```

> **Note**: Keyless signing uses Sigstore/Fulcio and currently requires a pre-provided OIDC token (e.g., from GitHub Actions or GitLab CI). Interactive browser-based OIDC authentication is planned for a future release.

**Verify Signature:**

```bash
kscorectl blueprint verify vendor/nginx

# With specific key
kscorectl blueprint verify vendor/nginx --key cosign.pub

# Strict mode (fail on missing signature)
kscorectl blueprint verify vendor/nginx --strict
```

### Testing Commands

**Run Tests:**

```bash
kscorectl blueprint test ./my-blueprint

# Verbose output
kscorectl blueprint test ./my-blueprint -v

# Run specific tests by tag
kscorectl blueprint test ./my-blueprint --tags integration

# Exclude tags
kscorectl blueprint test ./my-blueprint --exclude-tags slow

# Pattern matching
kscorectl blueprint test ./my-blueprint --pattern "test_install*"

# Parallel execution
kscorectl blueprint test ./my-blueprint --parallel --max-parallel 4

# Output format
kscorectl blueprint test ./my-blueprint --format json -o results.json
kscorectl blueprint test ./my-blueprint --format junit -o junit.xml

# Dry run
kscorectl blueprint test ./my-blueprint --dry-run
```

### Snapshot Commands

**List Snapshots:**

```bash
kscorectl blueprint snapshot list vendor/nginx
```

**Show Snapshot Details:**

```bash
kscorectl blueprint snapshot show vendor/nginx snap-123456
```

**Delete Snapshot:**

```bash
kscorectl blueprint snapshot delete vendor/nginx snap-123456
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

## Blueprint Authoring Guide

This section provides a complete walkthrough for creating blueprints from scratch.

### Step 1: Initialize Blueprint

Create a new blueprint with the init command:

```bash
kscorectl blueprint init myorg/nginx-stack \
  --description "Production-ready Nginx web server stack" \
  --author "Your Name" \
  --email "you@example.com" \
  --license "Apache-2.0" \
  --category "web-server" \
  --keywords "nginx,web,proxy"
```

This creates the directory structure with template files:

```
myorg-nginx-stack/
├── blueprint.yaml
├── README.md
├── states/
│   └── init.yaml
├── vars/
│   └── defaults.yaml
└── tests/
    └── test_basic.yaml
```

### Step 2: Define Parameters

Edit `blueprint.yaml` to define your parameters with proper validation:

```yaml
apiVersion: blueprints.keystone-core.io/v1
kind: Blueprint

metadata:
  name: myorg/nginx-stack
  version: "1.0.0"
  description: Production-ready Nginx web server stack

parameters:
  # Required parameters
  domain:
    type: string
    description: Primary domain name
    required: true
    format: hostname
    examples:
      - example.com
      - www.example.com

  # Numeric with defaults
  http_port:
    type: integer
    description: HTTP port
    default: 80
    minimum: 1
    maximum: 65535

  https_port:
    type: integer
    description: HTTPS port
    default: 443
    minimum: 1
    maximum: 65535

  # Boolean toggle
  enable_ssl:
    type: boolean
    description: Enable SSL/TLS
    default: true

  # Enum for limited options
  ssl_provider:
    type: string
    description: SSL certificate provider
    default: letsencrypt
    enum:
      - letsencrypt
      - custom
      - selfsigned
    feature: ssl  # Only used when ssl feature is enabled

  # Sensitive data
  ssl_key:
    type: string
    description: SSL private key (PEM format)
    sensitive: true
    source: secret
    feature: ssl
    required_if:
      - ssl_provider: custom

  # Complex object
  upstream_servers:
    type: array
    description: Backend servers for load balancing
    items:
      type: object
      properties:
        address:
          type: string
          format: hostname
        port:
          type: integer
          default: 8080
        weight:
          type: integer
          default: 1
          minimum: 1
          maximum: 100
    minItems: 1
    maxItems: 20

  # Conditional parameter
  worker_processes:
    type: string
    description: Number of worker processes
    default: auto
    pattern: "^(auto|[1-9][0-9]*)$"
```

### Step 3: Create Default Values

Define sensible defaults in `vars/defaults.yaml`:

```yaml
# Default parameter values
# These are used when parameters are not explicitly set

http_port: 80
https_port: 443
enable_ssl: true
ssl_provider: letsencrypt
worker_processes: auto

# Default upstream configuration
upstream_servers:
  - address: localhost
    port: 8080
    weight: 1

# Nginx configuration defaults
client_max_body_size: 100M
keepalive_timeout: 65
gzip_enabled: true
gzip_types:
  - text/plain
  - text/css
  - application/json
  - application/javascript

# SSL configuration defaults
ssl_protocols:
  - TLSv1.2
  - TLSv1.3
ssl_ciphers: "ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256"
ssl_session_timeout: 1d
ssl_session_cache: "shared:SSL:10m"

# Logging defaults
access_log: /var/log/nginx/access.log
error_log: /var/log/nginx/error.log
log_format: combined
```

### Step 4: Create State Files

Create the main entry point `states/init.yaml`:

```yaml
# Main entry point - orchestrates the full deployment
# This file is the default entrypoint and includes other states

include:
  - ./install.yaml
  - ./configure.yaml

# Conditional SSL setup
{{- if .params.enable_ssl }}
  - ./ssl.yaml
{{- end }}

# Final verification
verification:
  module: cmd
  state: run
  name: verify_nginx
  command: nginx -t && systemctl is-active nginx
  require:
    - "include:configure"
```

Create `states/install.yaml` for package installation:

```yaml
# Package installation and user setup

nginx_package:
  module: package
  state: installed
  name: nginx

nginx_user:
  module: user
  state: present
  name: nginx
  system: true
  shell: /sbin/nologin
  home: /var/cache/nginx
  require:
    - nginx_package

log_directory:
  module: file
  state: directory
  path: /var/log/nginx
  owner: nginx
  group: nginx
  mode: "0755"
  require:
    - nginx_user

config_directory:
  module: file
  state: directory
  path: /etc/nginx/conf.d
  owner: root
  group: root
  mode: "0755"
  require:
    - nginx_package
```

Create `states/configure.yaml` for configuration:

```yaml
# Nginx configuration

nginx_main_config:
  module: file
  state: managed
  path: /etc/nginx/nginx.conf
  source: templates/nginx.conf.tpl
  vars:
    worker_processes: "{{ .params.worker_processes }}"
    worker_connections: "{{ default 4096 .params.worker_connections }}"
    keepalive_timeout: "{{ .vars.keepalive_timeout }}"
  owner: root
  group: root
  mode: "0644"
  require:
    - "include:install"

site_config:
  module: file
  state: managed
  path: /etc/nginx/conf.d/{{ .params.domain }}.conf
  source: templates/site.conf.tpl
  vars:
    domain: "{{ .params.domain }}"
    http_port: "{{ .params.http_port }}"
    https_port: "{{ .params.https_port }}"
    enable_ssl: "{{ .params.enable_ssl }}"
    upstream_servers: "{{ .params.upstream_servers | toJson }}"
  owner: root
  group: root
  mode: "0644"
  require:
    - nginx_main_config

nginx_service:
  module: service
  state: running
  name: nginx
  enabled: true
  require:
    - site_config
  watch:
    - nginx_main_config
    - site_config
```

Create `states/ssl.yaml` for SSL configuration:

```yaml
# SSL/TLS configuration (included when enable_ssl is true)

{{- if eq .params.ssl_provider "letsencrypt" }}
certbot_package:
  module: package
  state: installed
  name: certbot

certbot_nginx_plugin:
  module: package
  state: installed
  name: python3-certbot-nginx
  require:
    - certbot_package

obtain_certificate:
  module: cmd
  state: run
  name: certbot_obtain
  command: |
    certbot certonly --nginx -d {{ .params.domain }} \
      --non-interactive --agree-tos \
      --email {{ default "admin@" .params.domain | trimPrefix "@" }}{{ .params.domain }}
  creates: /etc/letsencrypt/live/{{ .params.domain }}/fullchain.pem
  require:
    - certbot_nginx_plugin
    - "configure:nginx_service"

renewal_cron:
  module: cron
  state: present
  name: certbot-renewal
  user: root
  hour: 2
  minute: 30
  weekday: 1
  job: certbot renew --quiet --post-hook "systemctl reload nginx"
  require:
    - obtain_certificate

{{- else if eq .params.ssl_provider "custom" }}
ssl_cert_dir:
  module: file
  state: directory
  path: /etc/nginx/ssl
  owner: root
  group: root
  mode: "0700"

ssl_certificate:
  module: file
  state: managed
  path: /etc/nginx/ssl/{{ .params.domain }}.crt
  content: "{{ .params.ssl_cert }}"
  owner: root
  group: root
  mode: "0644"
  require:
    - ssl_cert_dir

ssl_private_key:
  module: file
  state: managed
  path: /etc/nginx/ssl/{{ .params.domain }}.key
  content: "{{ .params.ssl_key }}"
  owner: root
  group: root
  mode: "0600"
  require:
    - ssl_cert_dir

{{- else if eq .params.ssl_provider "selfsigned" }}
openssl_package:
  module: package
  state: installed
  name: openssl

ssl_cert_dir:
  module: file
  state: directory
  path: /etc/nginx/ssl
  owner: root
  group: root
  mode: "0700"

generate_selfsigned:
  module: cmd
  state: run
  name: generate_ssl
  command: |
    openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
      -keyout /etc/nginx/ssl/{{ .params.domain }}.key \
      -out /etc/nginx/ssl/{{ .params.domain }}.crt \
      -subj "/CN={{ .params.domain }}"
  creates: /etc/nginx/ssl/{{ .params.domain }}.crt
  require:
    - openssl_package
    - ssl_cert_dir
{{- end }}

# Update nginx config to use SSL
ssl_site_config:
  module: file
  state: managed
  path: /etc/nginx/conf.d/{{ .params.domain }}-ssl.conf
  source: templates/site-ssl.conf.tpl
  vars:
    domain: "{{ .params.domain }}"
    https_port: "{{ .params.https_port }}"
    ssl_provider: "{{ .params.ssl_provider }}"
  owner: root
  group: root
  mode: "0644"
  require:
    {{- if eq .params.ssl_provider "letsencrypt" }}
    - obtain_certificate
    {{- else if eq .params.ssl_provider "custom" }}
    - ssl_certificate
    - ssl_private_key
    {{- else }}
    - generate_selfsigned
    {{- end }}

nginx_ssl_reload:
  module: service
  state: running
  name: nginx
  reload: true
  require:
    - ssl_site_config
```

Create `states/rollback.yaml` for rollback support:

```yaml
# Rollback entry point
# Restores previous configuration state

restore_nginx_config:
  module: cmd
  state: run
  name: restore_config
  command: |
    if [ -f /etc/nginx/nginx.conf.bak ]; then
      cp /etc/nginx/nginx.conf.bak /etc/nginx/nginx.conf
    fi
    if [ -d /etc/nginx/conf.d.bak ]; then
      rm -rf /etc/nginx/conf.d/*
      cp -r /etc/nginx/conf.d.bak/* /etc/nginx/conf.d/
    fi

validate_config:
  module: cmd
  state: run
  name: validate
  command: nginx -t
  require:
    - restore_nginx_config

restart_nginx:
  module: service
  state: running
  name: nginx
  reload: true
  require:
    - validate_config
```

### Step 5: Create Templates

Create `templates/nginx.conf.tpl`:

```nginx
# Nginx main configuration
# Generated by myorg/nginx-stack blueprint

user nginx;
worker_processes {{ .vars.worker_processes }};
error_log {{ .vars.error_log }} warn;
pid /run/nginx.pid;

events {
    worker_connections {{ .vars.worker_connections }};
    use epoll;
    multi_accept on;
}

http {
    include /etc/nginx/mime.types;
    default_type application/octet-stream;

    log_format main '$remote_addr - $remote_user [$time_local] "$request" '
                    '$status $body_bytes_sent "$http_referer" '
                    '"$http_user_agent" "$http_x_forwarded_for"';

    access_log {{ .vars.access_log }} main;

    sendfile on;
    tcp_nopush on;
    tcp_nodelay on;
    keepalive_timeout {{ .vars.keepalive_timeout }};
    types_hash_max_size 2048;
    server_tokens off;

    client_max_body_size {{ .vars.client_max_body_size }};

    {{- if .vars.gzip_enabled }}
    gzip on;
    gzip_vary on;
    gzip_proxied any;
    gzip_comp_level 6;
    gzip_types {{ .vars.gzip_types | join " " }};
    {{- end }}

    include /etc/nginx/conf.d/*.conf;
}
```

Create `templates/site.conf.tpl`:

```nginx
# Site configuration for {{ .vars.domain }}

{{- $upstream := fromJson .vars.upstream_servers }}

upstream backend {
    {{- range $upstream }}
    server {{ .address }}:{{ .port }} weight={{ .weight }};
    {{- end }}
    keepalive 32;
}

server {
    listen {{ .vars.http_port }};
    server_name {{ .vars.domain }};

    {{- if .vars.enable_ssl }}
    return 301 https://$server_name$request_uri;
    {{- else }}
    location / {
        proxy_pass http://backend;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Connection "";
    }
    {{- end }}

    location /health {
        access_log off;
        return 200 "OK\n";
    }
}
```

### Step 6: Add Features

Define optional features in `blueprint.yaml`:

```yaml
features:
  ssl:
    description: Enable SSL/TLS with certificate management
    default: true
    enables:
      - states/ssl.yaml
    parameters:
      - ssl_provider
      - ssl_cert
      - ssl_key

  metrics:
    description: Enable Prometheus metrics endpoint
    default: false
    enables:
      - states/metrics.yaml
    parameters:
      - metrics_port

  caching:
    description: Enable response caching
    default: false
    enables:
      - states/caching.yaml
    parameters:
      - cache_path
      - cache_size
      - cache_ttl

  rate_limiting:
    description: Enable request rate limiting
    default: false
    enables:
      - states/rate-limit.yaml
    parameters:
      - rate_limit
      - rate_limit_burst
```

### Step 7: Create Tests

Create `tests/test_basic.yaml`:

```yaml
name: basic-tests
description: Basic functionality tests

setup:
  states:
    - states/test-setup.yaml

teardown:
  always: true
  states:
    - states/test-cleanup.yaml

tests:
  - name: test_install_completes
    description: Verify installation completes without errors
    parameters:
      domain: test.example.com
      enable_ssl: false
      upstream_servers:
        - address: localhost
          port: 8080
    assertions:
      - type: state_changed
        state:
          id: nginx_package

  - name: test_config_created
    description: Verify configuration file is created
    parameters:
      domain: test.example.com
      enable_ssl: false
    assertions:
      - type: file_exists
        file:
          path: /etc/nginx/conf.d/test.example.com.conf
      - type: file_mode
        file:
          path: /etc/nginx/conf.d/test.example.com.conf
          mode: "0644"
      - type: file_contains
        file:
          path: /etc/nginx/conf.d/test.example.com.conf
          contains: "server_name test.example.com"

  - name: test_service_running
    description: Verify nginx service is running
    assertions:
      - type: command_success
        command:
          command: systemctl is-active nginx
          exit_code: 0

  - name: test_config_valid
    description: Verify nginx configuration is valid
    assertions:
      - type: command_success
        command:
          command: nginx -t
          exit_code: 0

  - name: test_idempotency
    description: Verify re-running produces no changes
    parameters:
      domain: test.example.com
      enable_ssl: false
    assertions:
      - type: idempotent
```

Create `tests/test_ssl.yaml`:

```yaml
name: ssl-tests
description: SSL/TLS functionality tests
tags:
  - ssl
  - integration

tests:
  - name: test_letsencrypt_setup
    description: Test Let's Encrypt certificate setup
    skip: "Requires real domain"
    parameters:
      domain: test.example.com
      enable_ssl: true
      ssl_provider: letsencrypt

  - name: test_selfsigned_certificate
    description: Test self-signed certificate generation
    parameters:
      domain: test.example.com
      enable_ssl: true
      ssl_provider: selfsigned
    assertions:
      - type: file_exists
        file:
          path: /etc/nginx/ssl/test.example.com.crt
      - type: file_mode
        file:
          path: /etc/nginx/ssl/test.example.com.key
          mode: "0600"
      - type: command_output
        command:
          command: openssl x509 -in /etc/nginx/ssl/test.example.com.crt -noout -subject
          stdout_contains: "CN = test.example.com"

  - name: test_custom_certificate
    description: Test custom certificate installation
    parameters:
      domain: test.example.com
      enable_ssl: true
      ssl_provider: custom
      ssl_cert: |
        -----BEGIN CERTIFICATE-----
        [test certificate content]
        -----END CERTIFICATE-----
      ssl_key: |
        -----BEGIN PRIVATE KEY-----
        [test key content]
        -----END PRIVATE KEY-----
    assertions:
      - type: file_exists
        file:
          path: /etc/nginx/ssl/test.example.com.crt
      - type: file_mode
        file:
          path: /etc/nginx/ssl/test.example.com.key
          mode: "0600"
```

### Step 8: Validate and Lint

Run validation and linting:

```bash
# Validate manifest structure
kscorectl blueprint validate ./myorg-nginx-stack

# Validate with test parameters
kscorectl blueprint validate ./myorg-nginx-stack --params tests/params.yaml

# Lint for best practices
kscorectl blueprint lint ./myorg-nginx-stack

# Lint with strict mode
kscorectl blueprint lint --strict ./myorg-nginx-stack
```

### Step 9: Generate Documentation

Generate README and documentation:

```bash
# Generate README.md
kscorectl blueprint docs ./myorg-nginx-stack -o README.md

# Generate full documentation
kscorectl blueprint docs ./myorg-nginx-stack --full -o docs/
```

### Step 10: Test Locally

Run the test suite:

```bash
# Run all tests
kscorectl blueprint test ./myorg-nginx-stack

# Run specific test
kscorectl blueprint test ./myorg-nginx-stack --pattern "test_install*"

# Run tests with specific tags
kscorectl blueprint test ./myorg-nginx-stack --tags basic

# Verbose output
kscorectl blueprint test ./myorg-nginx-stack -v

# Generate test report
kscorectl blueprint test ./myorg-nginx-stack --format junit -o test-results.xml
```

### Step 11: Version and Publish

Prepare for release:

```bash
# Update version in blueprint.yaml
sed -i 's/version: "1.0.0"/version: "1.1.0"/' blueprint.yaml

# Sign the blueprint
kscorectl blueprint sign ./myorg-nginx-stack --key ~/.keystone-core/signing.key

# Publish to registry
kscorectl blueprint publish ./myorg-nginx-stack

# Or publish to custom registry
kscorectl blueprint publish ./myorg-nginx-stack --registry https://registry.myorg.com
```

### Best Practices

**Parameter Design:**

- Use descriptive names with consistent naming conventions
- Provide sensible defaults for optional parameters
- Mark sensitive data with `sensitive: true`
- Use format validators for structured data (hostname, email, etc.)
- Document parameters with descriptions and examples

**State Organization:**

- Keep state files focused (one concern per file)
- Use meaningful IDs for states (not generic names)
- Order states by dependency naturally
- Use explicit `require` declarations for complex dependencies

**Template Guidelines:**

- Keep templates simple and readable
- Use Go template functions for complex logic
- Document template variables
- Validate template output during testing

**Testing:**

- Test all code paths (features, conditionals)
- Include idempotency tests
- Test failure scenarios
- Use tags to organize test categories

**Documentation:**

- Maintain accurate README with usage examples
- Document all parameters with examples
- Keep CHANGELOG up to date
- Include troubleshooting section

**Versioning:**

- Follow semantic versioning strictly
- Document breaking changes in CHANGELOG
- Provide migration guides for major versions
- Test upgrades between versions

**Security:**

- Never store secrets in blueprint files
- Use `sensitive: true` for passwords and keys
- Validate input parameters strictly
- Review file permissions in states

## Testing Framework

### Test Suite Format

```yaml
name: my-blueprint-tests
description: Test suite for my-blueprint

setup:
  states:
    - states/test-setup.yaml

teardown:
  states:
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
      - type: state_changed
        state:
          id: install_package

  - name: test_config_file
    description: Verify configuration file
    assertions:
      - type: file_contains
        file:
          path: /etc/myapp/config.yaml
          contains: "domain: test.example.com"
      - type: file_mode
        file:
          path: /etc/myapp/config.yaml
          mode: "0644"

  - name: test_service_running
    description: Verify service is running
    assertions:
      - type: command_success
        command:
          command: systemctl is-active myapp
          exit_code: 0
```

### Setup and Teardown Lifecycle

The test runner executes setup and teardown at both suite and test levels:

1. Suite setup (`setup`)
2. Per-test setup (`tests[].setup`)
3. Test execution (entrypoint + parameters)
4. Assertions
5. Per-test teardown (`tests[].teardown`)
6. Suite teardown (`teardown`)

Teardown can be configured to always run even on failure.

```yaml
setup:
  states:
    - states/bootstrap.yaml
  commands:
    - command: "systemctl stop myapp"

teardown:
  always: true
  states:
    - states/cleanup.yaml
  files:
    - /tmp/test-output.log
```

### Defaults and Overrides

Defaults apply to all tests unless overridden:

```yaml
defaults:
  timeout: 5m
  dry_run: false
  parameters:
    region: us-east-1
  mocks:
    - type: command
      command:
        pattern: "systemctl status.*"
        stdout: "active (running)"
        exit_code: 0
```

### Mock Configuration

Mocks simulate external dependencies during tests. Supported mock types:
`command`, `file`, `http`, `package`, `service`.

```yaml
tests:
  - name: test_with_mocks
    mocks:
      - type: command
        command:
          pattern: "curl .*"
          stdout: "ok"
          stderr: ""
          exit_code: 0
      - type: file
        file:
          path: /etc/myapp/config.yaml
          exists: true
          mode: "0644"
          owner: root
          group: root
          content: "enabled: true"
      - type: http
        http:
          url: "https://api.example.com/.*"
          status_code: 200
          body: "{\"status\":\"ok\"}"
      - type: package
        package:
          name: nginx
          installed: true
          version: "1.18.0"
      - type: service
        service:
          name: nginx
          running: true
          enabled: true
```

### Assertion Types

| Type | Description |
|------|-------------|
| `state_applied` | State was applied |
| `state_changed` | State resulted in changes |
| `state_unchanged` | State made no changes |
| `state_failed` | State failed |
| `file_exists` | File exists |
| `file_not_exists` | File does not exist |
| `file_contains` | File contains/equals/matches content |
| `file_mode` | File permissions match |
| `file_owner` | File owner/group match |
| `directory_exists` | Directory exists |
| `command_success` | Command succeeds |
| `command_failure` | Command fails with expected code |
| `command_output` | Command output matches expectations |
| `output_contains` | Blueprint output contains value |
| `output_equals` | Blueprint output equals value |
| `output_matches` | Blueprint output matches regex |
| `expression` | CEL expression evaluates to true |
| `states_applied` | Total states applied equals expected count |
| `states_changed` | Total states changed equals expected count |
| `states_failed` | Total states failed equals expected count |
| `no_failures` | No state failures occurred |
| `idempotent` | Re-run produces no changes |

### File Assertion

```yaml
assertions:
  - type: file_contains
    file:
      path: /etc/myapp/config.yaml
      contains: "expected content"
  - type: file_mode
    file:
      path: /etc/myapp/config.yaml
      mode: "0644"
  - type: file_owner
    file:
      path: /etc/myapp/config.yaml
      owner: root
      group: root
```

### Command Assertion

```yaml
assertions:
  - type: command_output
    command:
      command: myapp --version
      exit_code: 0
      stdout_contains: "1.0.0"
```

### Idempotency Assertion

```yaml
assertions:
  - type: idempotent
```

## Bundle System (Air-Gapped)

### Creating Bundles

```bash
# Create bundle from blueprint
kscorectl blueprint bundle create vendor/nginx@1.2.0 -o nginx-bundle.tar.gz

# Include all dependencies
kscorectl blueprint bundle create vendor/nginx@1.2.0 --include-deps -o nginx-bundle.tar.gz

# Sign bundle
kscorectl blueprint bundle create vendor/nginx@1.2.0 --sign --key cosign.key -o nginx-bundle.tar.gz
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
kscorectl blueprint bundle install nginx-bundle.tar.gz

# Verify signatures
kscorectl blueprint bundle install nginx-bundle.tar.gz --verify --key cosign.pub
```

## Mirror Server

### Configuration

```yaml
# mirror-config.yaml
storage_dir: /var/lib/keystone-core/blueprints
listen_addr: ":8080"
upstream_url: https://registry.keystone-core.io
sync_interval: 1h
allow_push: true
require_signatures: true
trusted_keys:
  - /etc/keystone-core/keys/trusted.pub
```

### Running Mirror Server

```bash
# Start mirror server
kscorectl blueprint mirror serve --config mirror-config.yaml

# Sync from upstream
kscorectl blueprint mirror sync

# Export to directory
kscorectl blueprint mirror export --output /backup/blueprints

# Import from directory
kscorectl blueprint mirror import --input /backup/blueprints
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
kscorectl blueprint update vendor/nginx --accept-breaking-changes

# Review changes first
kscorectl blueprint info vendor/nginx@2.0.0 --show-breaking-changes
```

## See Also

- [Blueprints Concept](/docs/concepts/blueprints/) - Blueprint architecture and usage
- [State Management](/docs/concepts/state-management/) - Understanding states
- [Modules Reference](/docs/reference/modules/) - State module reference
- [CLI Reference](/docs/reference/cli/) - General CLI reference
