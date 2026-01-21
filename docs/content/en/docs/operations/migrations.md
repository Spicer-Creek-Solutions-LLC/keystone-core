---
title: "Migrations"
weight: 16
description: >
  Practical guides for migrating to Keystone Core and scaling deployments
---

## Overview

This guide covers the most common migration paths.

## Salt → Keystone Core

### Overview

Salt and Keystone Core share similar concepts: both use declarative state files to configure systems. This guide provides a step-by-step migration path.

### Concept Mapping

| Salt Concept | Keystone Core Equivalent |
|--------------|--------------------------|
| State files (.sls) | State files (.yaml) |
| Pillars | Variables (vars) |
| Grains | Facts |
| Minions | Agents |
| Master | Control Plane |
| Highstate | State Apply |
| Requisites | Requisites (same names) |
| Jinja Templates | Go Templates |
| Execution Modules | Remote Execution |
| Returners | Event System |
| Beacons | Agent Telemetry |
| Reactors | Reactors |

### State Translation Examples

#### Package Management

**Salt**:
```yaml
# /srv/salt/nginx/init.sls
nginx:
  pkg.installed:
    - name: nginx
    - version: 1.24.*

  service.running:
    - enable: True
    - require:
      - pkg: nginx
```

**Keystone Core**:
```yaml
# states/nginx.yaml
nginx_package:
  module: package
  state: installed
  name: nginx
  version: "1.24.*"

nginx_service:
  module: service
  state: running
  name: nginx
  enabled: true
  require:
    - nginx_package
```

#### File Management

**Salt**:
```yaml
# /srv/salt/nginx/config.sls
/etc/nginx/nginx.conf:
  file.managed:
    - source: salt://nginx/files/nginx.conf
    - user: root
    - group: root
    - mode: 644
    - template: jinja
    - context:
        worker_processes: {{ pillar['nginx']['workers'] }}
    - require:
      - pkg: nginx
    - watch_in:
      - service: nginx
```

**Keystone Core**:
```yaml
# states/nginx-config.yaml
nginx_config:
  module: file
  state: present
  path: /etc/nginx/nginx.conf
  source: kscore://nginx/files/nginx.conf
  owner: root
  group: root
  mode: "0644"
  template: true
  vars:
    worker_processes: "{{ .vars.nginx.workers }}"
  require:
    - nginx_package
  watch_in:
    - nginx_service
```

#### User Management

**Salt**:
```yaml
# /srv/salt/users/init.sls
{% for user, data in pillar.get('users', {}).items() %}
{{ user }}:
  user.present:
    - uid: {{ data.uid }}
    - gid: {{ data.gid }}
    - home: {{ data.home }}
    - shell: {{ data.shell }}
    - groups: {{ data.groups }}
{% endfor %}
```

**Keystone Core**:
```yaml
# states/users.yaml
{{ range $name, $data := .vars.users }}
user_{{ $name }}:
  module: user
  state: present
  name: {{ $name }}
  uid: {{ $data.uid }}
  gid: {{ $data.gid }}
  home: {{ $data.home }}
  shell: {{ $data.shell }}
  groups: {{ $data.groups | toJson }}
{{ end }}
```

### Pillar to Variables Migration

**Salt Pillar** (`/srv/pillar/nginx.sls`):
```yaml
nginx:
  workers: 4
  max_connections: 1024
  sites:
    - name: example.com
      port: 80
```

**Keystone Variables** (`vars/nginx.yaml`):
```yaml
nginx:
  workers: 4
  max_connections: 1024
  sites:
    - name: example.com
      port: 80
```

### Grain to Facts Migration

Salt grains are automatically available as Keystone facts:

| Salt Grain | Keystone Fact |
|------------|---------------|
| `grains['os']` | `{{ .facts.os }}` |
| `grains['osrelease']` | `{{ .facts.os_version }}` |
| `grains['fqdn']` | `{{ .facts.hostname }}` |
| `grains['ip_interfaces']` | `{{ .facts.ip }}` |
| `grains['mem_total']` | `{{ .facts.memory_total }}` |
| `grains['num_cpus']` | `{{ .facts.cpu_count }}` |

### Migration Script

```bash
#!/bin/bash
# salt-to-keystone-migrate.sh

# Convert Salt state files to Keystone format
for sls in /srv/salt/**/*.sls; do
  yaml_file=$(echo "$sls" | sed 's|/srv/salt|states|; s|.sls$|.yaml|')
  mkdir -p $(dirname "$yaml_file")

  echo "Converting $sls -> $yaml_file"

  # Use migration tool
  kscorectl migrate convert-salt \
    --input "$sls" \
    --output "$yaml_file" \
    --pillar-dir /srv/pillar

done

# Convert pillars to variables
for pillar in /srv/pillar/**/*.sls; do
  vars_file=$(echo "$pillar" | sed 's|/srv/pillar|vars|; s|.sls$|.yaml|')
  mkdir -p $(dirname "$vars_file")

  echo "Converting pillar $pillar -> $vars_file"

  kscorectl migrate convert-pillar \
    --input "$pillar" \
    --output "$vars_file"
done

# Validate converted states
kscorectl state validate states/
```

### Migration Checklist

- [ ] Inventory all Salt states in use
- [ ] Identify pillar data dependencies
- [ ] Convert state files (start with simplest)
- [ ] Convert pillars to variables
- [ ] Test converted states with `kscorectl state check`
- [ ] Run side-by-side comparison on test nodes
- [ ] Migrate secrets to Keystone secret management
- [ ] Update CI/CD pipelines
- [ ] Phase rollout to production (10% → 50% → 100%)
- [ ] Decommission Salt master

---

## Ansible → Keystone Core

### Overview

Ansible uses playbooks and roles; Keystone Core uses state files and modules. Both are declarative, making migration straightforward.

### Concept Mapping

| Ansible Concept | Keystone Core Equivalent |
|-----------------|--------------------------|
| Playbooks | State files |
| Roles | State file includes/blueprints |
| Tasks | State declarations |
| Handlers | watch/watch_in requisites |
| Variables | Variables (vars) |
| Facts | Facts |
| Inventory | Agent targeting |
| Modules | Modules |
| Templates (Jinja2) | Templates (Go) |
| Vault | Secret management |
| AWX/Tower | Control Plane UI (coming) |

### Task Translation Examples

#### Package Installation

**Ansible**:
```yaml
# roles/nginx/tasks/main.yml
- name: Install nginx
  ansible.builtin.package:
    name: nginx
    state: present
  notify: Restart nginx

- name: Start nginx service
  ansible.builtin.service:
    name: nginx
    state: started
    enabled: yes
```

**Keystone Core**:
```yaml
# states/nginx.yaml
nginx_package:
  module: package
  state: installed
  name: nginx

nginx_service:
  module: service
  state: running
  name: nginx
  enabled: true
  watch:
    - nginx_package
```

#### Template Files

**Ansible**:
```yaml
- name: Configure nginx
  ansible.builtin.template:
    src: nginx.conf.j2
    dest: /etc/nginx/nginx.conf
    owner: root
    group: root
    mode: '0644'
  notify: Reload nginx
```

**Keystone Core**:
```yaml
nginx_config:
  module: file
  state: present
  path: /etc/nginx/nginx.conf
  source: kscore://nginx/templates/nginx.conf
  template: true
  owner: root
  group: root
  mode: "0644"
  watch_in:
    - nginx_reload
```

#### Conditionals

**Ansible**:
```yaml
- name: Install package (Debian)
  ansible.builtin.apt:
    name: nginx
    state: present
  when: ansible_os_family == "Debian"

- name: Install package (RedHat)
  ansible.builtin.yum:
    name: nginx
    state: present
  when: ansible_os_family == "RedHat"
```

**Keystone Core**:
```yaml
# Keystone automatically uses correct package manager
nginx_package:
  module: package
  state: installed
  name: nginx

# Or use explicit conditionals
{{ if eq .facts.os_family "debian" }}
nginx_package:
  module: package
  state: installed
  name: nginx
  pkg_manager: apt
{{ else if eq .facts.os_family "redhat" }}
nginx_package:
  module: package
  state: installed
  name: nginx
  pkg_manager: yum
{{ end }}
```

#### Loops

**Ansible**:
```yaml
- name: Create users
  ansible.builtin.user:
    name: "{{ item.name }}"
    uid: "{{ item.uid }}"
    state: present
  loop: "{{ users }}"
```

**Keystone Core**:
```yaml
{{ range .vars.users }}
user_{{ .name }}:
  module: user
  state: present
  name: {{ .name }}
  uid: {{ .uid }}
{{ end }}
```

#### Handlers

**Ansible**:
```yaml
# tasks/main.yml
- name: Update config
  template:
    src: app.conf.j2
    dest: /etc/app/config.conf
  notify:
    - Restart app
    - Clear cache

# handlers/main.yml
- name: Restart app
  service:
    name: myapp
    state: restarted

- name: Clear cache
  command: /usr/bin/clear-cache
```

**Keystone Core**:
```yaml
app_config:
  module: file
  state: present
  path: /etc/app/config.conf
  source: kscore://app/templates/app.conf
  template: true
  watch_in:
    - app_restart
    - cache_clear

app_restart:
  module: service
  state: restarted
  name: myapp

cache_clear:
  module: cmd
  state: wait
  command: /usr/bin/clear-cache
  watch:
    - app_config
```

### Inventory to Targeting

**Ansible Inventory**:
```ini
[webservers]
web1.example.com
web2.example.com

[webservers:vars]
http_port=80

[dbservers]
db1.example.com
db2.example.com
```

**Keystone Agent Targeting**:
```bash
# Target by role (set during agent registration)
kscorectl state apply nginx.yaml --target "role=webserver"

# Target by hostname pattern
kscorectl state apply nginx.yaml --target "hostname=web*.example.com"

# Target by custom tags
kscorectl state apply nginx.yaml --target "tier=frontend"
```

### Variable Migration

**Ansible** (`group_vars/webservers.yml`):
```yaml
http_port: 80
max_clients: 200
nginx:
  worker_processes: auto
  sites:
    - name: example.com
```

**Keystone** (`vars/webservers.yaml`):
```yaml
http_port: 80
max_clients: 200
nginx:
  worker_processes: auto
  sites:
    - name: example.com
```

### Migration Script

```bash
#!/bin/bash
# ansible-to-keystone-migrate.sh

# Convert Ansible roles to Keystone states
for role in roles/*/; do
  role_name=$(basename "$role")
  echo "Converting role: $role_name"

  kscorectl migrate convert-ansible \
    --role "$role" \
    --output "states/$role_name.yaml" \
    --vars-output "vars/$role_name.yaml"
done

# Convert inventory to targeting metadata
kscorectl migrate convert-inventory \
  --input inventory/hosts \
  --output agent-tags.yaml

# Validate
kscorectl state validate states/
```

### Migration Checklist

- [ ] Document all playbooks and roles in use
- [ ] Map Ansible modules to Keystone modules
- [ ] Convert roles to state files
- [ ] Convert group_vars/host_vars to variables
- [ ] Migrate Ansible Vault secrets
- [ ] Set up agent targeting based on inventory groups
- [ ] Test with `--check` mode
- [ ] Run parallel execution during transition
- [ ] Update AWX/Tower jobs to use Keystone
- [ ] Decommission Ansible infrastructure

---

## Puppet → Keystone Core

### Overview

Puppet uses manifests with a Ruby-like DSL; Keystone Core uses YAML state files. Both enforce desired state.

### Concept Mapping

| Puppet Concept | Keystone Core Equivalent |
|----------------|--------------------------|
| Manifests (.pp) | State files (.yaml) |
| Modules | Modules + Blueprints |
| Resources | State declarations |
| Classes | State file includes |
| Hiera | Variables (vars) |
| Facts (Facter) | Facts |
| Nodes | Agents |
| Puppet Server | Control Plane |
| Catalog | Rendered state |
| Notify/Subscribe | watch/watch_in |
| Require/Before | require/require_in |

### Manifest Translation Examples

#### Package and Service

**Puppet**:
```puppet
# modules/nginx/manifests/init.pp
class nginx {
  package { 'nginx':
    ensure => installed,
  }

  service { 'nginx':
    ensure  => running,
    enable  => true,
    require => Package['nginx'],
  }

  file { '/etc/nginx/nginx.conf':
    ensure  => file,
    source  => 'puppet:///modules/nginx/nginx.conf',
    owner   => 'root',
    group   => 'root',
    mode    => '0644',
    notify  => Service['nginx'],
    require => Package['nginx'],
  }
}
```

**Keystone Core**:
```yaml
# states/nginx.yaml
nginx_package:
  module: package
  state: installed
  name: nginx

nginx_service:
  module: service
  state: running
  name: nginx
  enabled: true
  require:
    - nginx_package

nginx_config:
  module: file
  state: present
  path: /etc/nginx/nginx.conf
  source: kscore://nginx/files/nginx.conf
  owner: root
  group: root
  mode: "0644"
  require:
    - nginx_package
  watch_in:
    - nginx_service
```

#### Variables and Templates

**Puppet**:
```puppet
# manifests/config.pp
class nginx::config (
  Integer $worker_processes = 4,
  Integer $worker_connections = 1024,
) {
  file { '/etc/nginx/nginx.conf':
    ensure  => file,
    content => epp('nginx/nginx.conf.epp', {
      'worker_processes'   => $worker_processes,
      'worker_connections' => $worker_connections,
    }),
  }
}
```

**Keystone Core**:
```yaml
# states/nginx-config.yaml
nginx_config:
  module: file
  state: present
  path: /etc/nginx/nginx.conf
  template: true
  contents: |
    worker_processes {{ default 4 .vars.nginx.worker_processes }};
    events {
      worker_connections {{ default 1024 .vars.nginx.worker_connections }};
    }
```

#### Defined Types (Loops)

**Puppet**:
```puppet
# manifests/vhost.pp
define nginx::vhost (
  String $server_name = $title,
  Integer $port = 80,
) {
  file { "/etc/nginx/sites-available/${server_name}.conf":
    ensure  => file,
    content => epp('nginx/vhost.conf.epp', {
      'server_name' => $server_name,
      'port'        => $port,
    }),
  }

  file { "/etc/nginx/sites-enabled/${server_name}.conf":
    ensure => link,
    target => "/etc/nginx/sites-available/${server_name}.conf",
  }
}

# Usage
nginx::vhost { 'example.com':
  port => 8080,
}
nginx::vhost { 'api.example.com':
  port => 3000,
}
```

**Keystone Core**:
```yaml
# states/nginx-vhosts.yaml
{{ range .vars.vhosts }}
vhost_{{ .name | replace "." "_" }}_available:
  module: file
  state: present
  path: /etc/nginx/sites-available/{{ .name }}.conf
  template: true
  contents: |
    server {
      listen {{ default 80 .port }};
      server_name {{ .name }};
      # ... rest of config
    }

vhost_{{ .name | replace "." "_" }}_enabled:
  module: file
  state: symlink
  path: /etc/nginx/sites-enabled/{{ .name }}.conf
  target: /etc/nginx/sites-available/{{ .name }}.conf
  require:
    - vhost_{{ .name | replace "." "_" }}_available
{{ end }}

# vars/nginx.yaml
vhosts:
  - name: example.com
    port: 8080
  - name: api.example.com
    port: 3000
```

#### Conditionals with Facts

**Puppet**:
```puppet
case $facts['os']['family'] {
  'Debian': {
    $config_path = '/etc/nginx/nginx.conf'
    $pkg_name = 'nginx'
  }
  'RedHat': {
    $config_path = '/etc/nginx/nginx.conf'
    $pkg_name = 'nginx'
  }
  default: {
    fail("Unsupported OS: ${facts['os']['family']}")
  }
}
```

**Keystone Core**:
```yaml
{{ if eq .facts.os_family "debian" }}
nginx_package:
  module: package
  state: installed
  name: nginx
{{ else if eq .facts.os_family "redhat" }}
nginx_package:
  module: package
  state: installed
  name: nginx
{{ else }}
# Keystone will fail if no matching condition
{{ fail "Unsupported OS family" }}
{{ end }}
```

### Hiera to Variables

**Hiera** (`data/common.yaml`):
```yaml
nginx::worker_processes: 4
nginx::sites:
  - example.com
  - api.example.com
```

**Hiera** (`data/nodes/web01.example.com.yaml`):
```yaml
nginx::worker_processes: 8
```

**Keystone Variables**:
```yaml
# vars/common.yaml
nginx:
  worker_processes: 4
  sites:
    - example.com
    - api.example.com

# vars/overrides/web01.yaml (agent-specific)
nginx:
  worker_processes: 8
```

```bash
# Apply with merged variables
kscorectl state apply nginx.yaml \
  --vars vars/common.yaml \
  --vars vars/overrides/web01.yaml \
  --target "hostname=web01.example.com"
```

### Migration Script

```bash
#!/bin/bash
# puppet-to-keystone-migrate.sh

# Convert Puppet modules to Keystone states
for module in /etc/puppetlabs/code/modules/*/; do
  mod_name=$(basename "$module")
  echo "Converting module: $mod_name"

  kscorectl migrate convert-puppet \
    --module "$module" \
    --output "states/$mod_name/" \
    --hiera-dir /etc/puppetlabs/code/data
done

# Convert Hiera data
kscorectl migrate convert-hiera \
  --input /etc/puppetlabs/code/data \
  --output vars/

# Generate node-to-agent mapping
kscorectl migrate convert-node-definitions \
  --input /etc/puppetlabs/code/manifests/site.pp \
  --output agent-classification.yaml

# Validate
kscorectl state validate states/
```

### Migration Checklist

- [ ] Export Puppet module list and dependencies
- [ ] Map Puppet resources to Keystone modules
- [ ] Convert manifests to state files
- [ ] Convert Hiera data to variables
- [ ] Migrate Hiera-eyaml secrets
- [ ] Map node definitions to agent targeting
- [ ] Test catalog compilation equivalence
- [ ] Run Puppet and Keystone side-by-side
- [ ] Validate idempotency
- [ ] Switch nodes gradually
- [ ] Decommission Puppet server

---

## Common Migration Patterns

### Running Side-by-Side

During migration, you can run both systems:

```bash
# Apply Keystone state but don't enforce
kscorectl state check nginx.yaml --target "role=web"

# Compare with current state (managed by Salt/Ansible/Puppet)
diff <(kscorectl state show nginx.yaml --rendered) \
     <(salt 'web*' state.show_lowstate nginx)
```

### Gradual Rollout

```yaml
# Phase 1: Canary (5%)
kscorectl state apply nginx.yaml --target "canary=true"

# Phase 2: Early adopters (25%)
kscorectl state apply nginx.yaml --target "migration_phase in (1,2)"

# Phase 3: Majority (75%)
kscorectl state apply nginx.yaml --target "migration_phase in (1,2,3)"

# Phase 4: Complete (100%)
kscorectl state apply nginx.yaml --target "role=web"
```

### Verification Commands

```bash
# Compare state outcomes
kscorectl state diff \
  --from "salt://web-server.sls" \
  --to "states/web-server.yaml" \
  --format table

# Validate parity
kscorectl migrate verify \
  --source-system salt \
  --target "role=web" \
  --report parity-report.html
```

## SQLite -> PostgreSQL

Use `kscorectl migrate` to move state data safely.

```bash
# Dry-run migration
kscorectl migrate run \
  --source sqlite:///var/lib/keystone/keystone.db \
  --target postgres://kscore:pass@postgres.example.com/kscore \
  --dry-run

# Execute migration
kscorectl migrate run \
  --source sqlite:///var/lib/keystone/keystone.db \
  --target postgres://kscore:pass@postgres.example.com/kscore

# Verify migration
kscorectl migrate validate \
  --source sqlite:///var/lib/keystone/keystone.db \
  --target postgres://kscore:pass@postgres.example.com/kscore
```

After verification, update the control plane config to use PostgreSQL and restart the service.

## Embedded NATS -> External NATS

1. **Provision external NATS**: Deploy a cluster with JetStream enabled.
2. **Configure auth/TLS**: Align credentials and TLS trust roots.
3. **Update control plane config**: Switch `nats.mode` to `external` and set URLs.
4. **Roll agents gradually**: Update agent configs in batches to reduce downtime.

If you run leaf nodes for edge agents, keep leaf connections pointed at the external cluster.
