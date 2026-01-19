---
title: "Multi-Tier Web Application"
weight: 1
description: >
  Deploy a production-ready 3-tier web application with load balancing, application servers, and database
---

## Overview

This scenario deploys a complete 3-tier web application consisting of:
- **Load Balancer Tier**: Nginx reverse proxy with SSL termination
- **Application Tier**: Node.js/Python application servers
- **Database Tier**: PostgreSQL with replication

### Business Context

A typical enterprise web application requires:
- High availability across all tiers
- SSL/TLS termination at the edge
- Horizontal scaling for application servers
- Database redundancy with automatic failover
- Centralized logging and monitoring

## Architecture

```mermaid
flowchart TB
    Internet["Internet"]
    LB["Load Balancer\n(Nginx)"]

    App01["App-01\n(Node.js)"]
    App02["App-02\n(Node.js)"]
    App03["App-03\n(Node.js)"]

    DB01["DB-01\n(Primary)"]
    DB02["DB-02\n(Standby)"]

    Internet --> LB
    LB --> App01
    LB --> App02
    LB --> App03

    App01 --> DB01
    App02 --> DB01
    App03 --> DB01

    DB01 <-- "Streaming\nReplication" --> DB02
```

## Implementation

### Step 1: Define Target Groups

First, assign roles to your servers using grain data or targeting:

```bash
# Tag servers with their roles
kscorectl exec "role:lb" --cmd "echo 'tier: loadbalancer' >> /etc/kscore/grains.yaml"
kscorectl exec "role:app" --cmd "echo 'tier: application' >> /etc/kscore/grains.yaml"
kscorectl exec "role:db" --cmd "echo 'tier: database' >> /etc/kscore/grains.yaml"
```

### Step 2: Load Balancer Configuration

Create the load balancer state file:

```yaml
# states/webapp/loadbalancer.yaml
nginx_package:
  module: package
  state: installed
  name: nginx

nginx_ssl_cert:
  module: file
  state: present
  path: /etc/nginx/ssl/webapp.crt
  contents: |
    {{ .pillar.ssl_certificate }}
  mode: "0644"
  require:
    - nginx_package

nginx_ssl_key:
  module: file
  state: present
  path: /etc/nginx/ssl/webapp.key
  contents: |
    {{ .pillar.ssl_private_key }}
  mode: "0600"
  require:
    - nginx_package

nginx_upstream_config:
  module: file
  state: present
  path: /etc/nginx/conf.d/upstream.conf
  contents: |
    upstream app_servers {
        least_conn;
        {{ range .pillar.app_servers }}
        server {{ . }}:3000 weight=1 max_fails=3 fail_timeout=30s;
        {{ end }}
        keepalive 32;
    }
  require:
    - nginx_package

nginx_site_config:
  module: file
  state: present
  path: /etc/nginx/sites-available/webapp.conf
  contents: |
    server {
        listen 80;
        server_name {{ .pillar.domain }};
        return 301 https://$server_name$request_uri;
    }

    server {
        listen 443 ssl http2;
        server_name {{ .pillar.domain }};

        ssl_certificate /etc/nginx/ssl/webapp.crt;
        ssl_certificate_key /etc/nginx/ssl/webapp.key;
        ssl_protocols TLSv1.2 TLSv1.3;
        ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256;
        ssl_prefer_server_ciphers on;
        ssl_session_cache shared:SSL:10m;
        ssl_session_timeout 10m;

        # Security headers
        add_header X-Frame-Options "SAMEORIGIN" always;
        add_header X-Content-Type-Options "nosniff" always;
        add_header X-XSS-Protection "1; mode=block" always;
        add_header Strict-Transport-Security "max-age=31536000" always;

        location / {
            proxy_pass http://app_servers;
            proxy_http_version 1.1;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
            proxy_set_header Connection "";
            proxy_connect_timeout 30s;
            proxy_send_timeout 60s;
            proxy_read_timeout 60s;
        }

        location /health {
            access_log off;
            return 200 "healthy\n";
        }
    }
  require:
    - nginx_upstream_config
    - nginx_ssl_cert
    - nginx_ssl_key

nginx_site_enable:
  module: file
  state: symlink
  path: /etc/nginx/sites-enabled/webapp.conf
  source: /etc/nginx/sites-available/webapp.conf
  require:
    - nginx_site_config

nginx_service:
  module: service
  state: running
  name: nginx
  enable: true
  reload: true
  watch:
    - nginx_site_config
    - nginx_upstream_config
```

### Step 3: Application Server Configuration

```yaml
# states/webapp/application.yaml
nodejs_repo:
  module: cmd
  state: run
  name: setup_nodesource
  command: |
    curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
  creates: /etc/apt/sources.list.d/nodesource.list
  onlyif: "test ! -f /etc/apt/sources.list.d/nodesource.list"

nodejs_package:
  module: package
  state: installed
  name: nodejs
  require:
    - nodejs_repo

app_user:
  module: user
  state: present
  name: webapp
  system: true
  home: /opt/webapp
  shell: /usr/sbin/nologin

app_directory:
  module: file
  state: directory
  path: /opt/webapp
  owner: webapp
  group: webapp
  mode: "0755"
  require:
    - app_user

app_config:
  module: file
  state: present
  path: /opt/webapp/.env
  owner: webapp
  group: webapp
  mode: "0600"
  contents: |
    NODE_ENV=production
    PORT=3000
    DATABASE_URL=postgresql://{{ .pillar.db_user }}:{{ .pillar.db_password }}@{{ .pillar.db_host }}:5432/{{ .pillar.db_name }}
    SESSION_SECRET={{ .pillar.session_secret }}
    LOG_LEVEL=info
  require:
    - app_directory

app_code:
  module: git
  state: latest
  repo: {{ .pillar.git_repo }}
  target: /opt/webapp/app
  revision: {{ .pillar.git_branch | default "main" }}
  user: webapp
  require:
    - app_directory
    - nodejs_package

app_npm_install:
  module: cmd
  state: run
  name: npm_install
  command: npm ci --production
  cwd: /opt/webapp/app
  user: webapp
  onchanges:
    - app_code

app_systemd_unit:
  module: file
  state: present
  path: /etc/systemd/system/webapp.service
  contents: |
    [Unit]
    Description=Web Application
    After=network.target

    [Service]
    Type=simple
    User=webapp
    Group=webapp
    WorkingDirectory=/opt/webapp/app
    EnvironmentFile=/opt/webapp/.env
    ExecStart=/usr/bin/node server.js
    Restart=always
    RestartSec=10
    StandardOutput=journal
    StandardError=journal

    # Security hardening
    NoNewPrivileges=true
    ProtectSystem=strict
    ProtectHome=true
    ReadWritePaths=/opt/webapp
    PrivateTmp=true

    [Install]
    WantedBy=multi-user.target

systemd_reload:
  module: cmd
  state: run
  name: systemd_daemon_reload
  command: systemctl daemon-reload
  onchanges:
    - app_systemd_unit

app_service:
  module: service
  state: running
  name: webapp
  enable: true
  require:
    - app_systemd_unit
    - app_npm_install
    - app_config
  watch:
    - app_code
    - app_config
```

### Step 4: Database Configuration

```yaml
# states/webapp/database-primary.yaml
postgresql_package:
  module: package
  state: installed
  name:
    - postgresql-15
    - postgresql-contrib-15

postgresql_data_dir:
  module: file
  state: directory
  path: /var/lib/postgresql/15/main
  owner: postgres
  group: postgres
  mode: "0700"

postgresql_conf:
  module: file
  state: present
  path: /etc/postgresql/15/main/postgresql.conf
  contents: |
    # Connection Settings
    listen_addresses = '*'
    port = 5432
    max_connections = 200

    # Memory Settings
    shared_buffers = {{ .pillar.db_shared_buffers | default "256MB" }}
    effective_cache_size = {{ .pillar.db_effective_cache | default "1GB" }}
    work_mem = {{ .pillar.db_work_mem | default "16MB" }}
    maintenance_work_mem = {{ .pillar.db_maintenance_work_mem | default "128MB" }}

    # WAL Settings for Replication
    wal_level = replica
    max_wal_senders = 5
    wal_keep_size = 1GB
    synchronous_commit = on
    synchronous_standby_names = '{{ .pillar.db_standby_name | default "standby1" }}'

    # Checkpoints
    checkpoint_completion_target = 0.9
    max_wal_size = 2GB
    min_wal_size = 1GB

    # Logging
    logging_collector = on
    log_directory = '/var/log/postgresql'
    log_filename = 'postgresql-%Y-%m-%d.log'
    log_rotation_age = 1d
    log_rotation_size = 100MB
    log_min_duration_statement = 1000
    log_checkpoints = on
    log_connections = on
    log_disconnections = on
    log_lock_waits = on

    # Performance
    random_page_cost = 1.1
    effective_io_concurrency = 200
    default_statistics_target = 100
  require:
    - postgresql_package

postgresql_hba:
  module: file
  state: present
  path: /etc/postgresql/15/main/pg_hba.conf
  contents: |
    # Local connections
    local   all             postgres                                peer
    local   all             all                                     peer

    # IPv4 local connections
    host    all             all             127.0.0.1/32            scram-sha-256

    # Application servers
    {{ range .pillar.app_servers }}
    host    {{ $.pillar.db_name }}  {{ $.pillar.db_user }}  {{ . }}/32  scram-sha-256
    {{ end }}

    # Replication
    host    replication     replicator      {{ .pillar.db_standby_ip }}/32  scram-sha-256
  require:
    - postgresql_package

postgresql_service:
  module: service
  state: running
  name: postgresql
  enable: true
  require:
    - postgresql_conf
    - postgresql_hba
  watch:
    - postgresql_conf
    - postgresql_hba

# Create application database and user
db_create_user:
  module: postgresql_user
  state: present
  name: {{ .pillar.db_user }}
  password: {{ .pillar.db_password }}
  require:
    - postgresql_service

db_create_database:
  module: postgresql_db
  state: present
  name: {{ .pillar.db_name }}
  owner: {{ .pillar.db_user }}
  require:
    - db_create_user

db_create_replicator:
  module: postgresql_user
  state: present
  name: replicator
  password: {{ .pillar.replicator_password }}
  replication: true
  require:
    - postgresql_service
```

### Step 5: Orchestration with Pillar Data

Create pillar data for each environment:

```yaml
# pillar/webapp/production.yaml
domain: webapp.example.com
environment: production

# SSL certificates (in production, use vault or secrets management)
ssl_certificate: |
  -----BEGIN CERTIFICATE-----
  ... (your certificate)
  -----END CERTIFICATE-----

ssl_private_key: |
  -----BEGIN PRIVATE KEY-----
  ... (your key)
  -----END PRIVATE KEY-----

# Application servers (discovered dynamically or defined)
app_servers:
  - 10.0.2.10
  - 10.0.2.11
  - 10.0.2.12

# Database configuration
db_host: 10.0.3.10
db_name: webapp
db_user: webapp_user
db_password: "{{ vault('webapp/db_password') }}"
db_standby_ip: 10.0.3.11
replicator_password: "{{ vault('webapp/replicator_password') }}"

# Application settings
git_repo: https://github.com/myorg/webapp.git
git_branch: release-1.5
session_secret: "{{ vault('webapp/session_secret') }}"
```

### Step 6: Apply the Configuration

```bash
# Apply to load balancers
kscorectl state apply states/webapp/loadbalancer.yaml \
  --target "tier:loadbalancer and environment:production" \
  --pillar pillar/webapp/production.yaml

# Apply to application servers (rolling deployment)
kscorectl state apply states/webapp/application.yaml \
  --target "tier:application and environment:production" \
  --pillar pillar/webapp/production.yaml \
  --batch-size 1 \
  --batch-delay 30s

# Apply to primary database
kscorectl state apply states/webapp/database-primary.yaml \
  --target "tier:database and role:primary and environment:production" \
  --pillar pillar/webapp/production.yaml
```

## Verification

### Health Checks

```bash
# Check load balancer
curl -k https://webapp.example.com/health

# Check application servers directly
kscorectl exec "tier:application" --cmd "curl -s localhost:3000/health"

# Check database connectivity
kscorectl exec "tier:application" --cmd "pg_isready -h 10.0.3.10 -p 5432"

# Verify replication status
kscorectl exec "tier:database and role:primary" --cmd \
  "sudo -u postgres psql -c 'SELECT client_addr, state, sync_state FROM pg_stat_replication;'"
```

### Load Testing

```bash
# Install load testing tool
kscorectl exec "lb-01" --cmd "apt-get install -y apache2-utils"

# Run basic load test
kscorectl exec "lb-01" --cmd \
  "ab -n 10000 -c 100 https://webapp.example.com/"
```

## Troubleshooting

### Common Issues

**502 Bad Gateway from Nginx**
```bash
# Check if app servers are running
kscorectl exec "tier:application" --cmd "systemctl status webapp"

# Check Nginx upstream status
kscorectl exec "tier:loadbalancer" --cmd "nginx -t && tail -50 /var/log/nginx/error.log"
```

**Database Connection Failures**
```bash
# Verify PostgreSQL is listening
kscorectl exec "tier:database" --cmd "ss -tlnp | grep 5432"

# Check pg_hba.conf rules
kscorectl exec "tier:database" --cmd "cat /etc/postgresql/15/main/pg_hba.conf"

# Test connection from app server
kscorectl exec "app-01" --cmd "PGPASSWORD=xxx psql -h 10.0.3.10 -U webapp_user -d webapp -c '\dt'"
```

**Replication Lag**
```bash
# Check replication lag on primary
kscorectl exec "tier:database and role:primary" --cmd \
  "sudo -u postgres psql -c 'SELECT pg_wal_lsn_diff(pg_current_wal_lsn(), sent_lsn) as send_lag, pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn) as replay_lag FROM pg_stat_replication;'"
```

## Maintenance

### Rolling Updates

```bash
# Update application code with zero downtime
kscorectl state apply states/webapp/application.yaml \
  --target "tier:application and environment:production" \
  --pillar pillar/webapp/production.yaml \
  --batch-size 1 \
  --batch-delay 60s \
  --var git_branch=release-1.6
```

### Database Backup

```bash
# Create backup
kscorectl exec "tier:database and role:primary" --cmd \
  "sudo -u postgres pg_dump webapp | gzip > /backup/webapp-$(date +%Y%m%d).sql.gz"
```
