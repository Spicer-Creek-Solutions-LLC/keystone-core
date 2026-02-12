---
title: "Module Reference"
weight: 4
description: >
  Complete reference for all state modules with parameters and specifications
---

## Overview

Keystone Core includes 95 built-in state modules for declarative configuration management. All modules are idempotent and cross-platform where applicable.

> **Note:** Kubernetes modules (`k8s_*`) require explicit registration with a configured Kubernetes client and are not available in the default module registry.

**Core Modules**:

- [file](#file-module) - Manage files and directories
- [package](#package-module) - Manage software packages
- [service](#service-module) - Manage system services
- [user](#user-module) - Manage user accounts
- [group](#group-module) - Manage groups
- [cmd](#cmd-module) - Execute commands

**Network Modules**:

- [network](#network-module) - Configure network interfaces
- [route](#route-module) - Manage static routes
- [firewall](#firewall-module) - Cross-platform firewall abstraction
- [iptables](#iptables-module) - Linux iptables rules
- [nftables](#nftables-module) - Linux nftables rules
- [firewalld](#firewalld-module) - Linux firewalld zones and rules
- [ufw](#ufw-module) - Ubuntu Uncomplicated Firewall

**Scheduled Task Modules**:

- [cron](#cron-module) - Linux cron jobs
- [systemd_timer](#systemd_timer-module) - Linux systemd timer units
- [launchd](#launchd-module) - macOS launchd jobs
- [scheduled_task](#scheduled_task-module) - Windows Task Scheduler
- [at](#at-module) - One-time scheduled tasks (Unix)

**Storage Modules**:

- [mount](#mount-module) - Manage filesystem mount points
- [swap](#swap-module) - Manage Linux swap space
- [lvm_pv](#lvm_pv-module) - Manage LVM physical volumes
- [lvm_vg](#lvm_vg-module) - Manage LVM volume groups
- [lvm_lv](#lvm_lv-module) - Manage LVM logical volumes
- [disk](#disk-module) - Manage disk partitions
- [filesystem](#filesystem-module) - Create and manage filesystems

**SSH Modules**:

- [authorized_keys](#authorized-keys-module) - Manage SSH authorized keys
- [known_hosts](#known-hosts-module) - Manage SSH known hosts
- [sshd_config](#sshd-config-module) - Manage SSH daemon configuration

**Security Modules**:

- [selinux](#selinux-module) - Manage SELinux state
- [selinux_boolean](#selinux-boolean-module) - Manage SELinux booleans
- [apparmor](#apparmor-module) - Manage AppArmor profile state
- [apparmor_profile](#apparmor-profile-module) - Install AppArmor profiles

**System Configuration Modules**:

- [timezone](#timezone-module) - Manage system timezone
- [locale](#locale-module) - Manage system locale
- [hostname](#hostname-module) - Manage system hostname
- [hosts](#hosts-module) - Manage /etc/hosts entries
- [sysctl](#sysctl-module) - Manage kernel parameters
- [kernel_module](#kernel-module-module) - Manage kernel modules
- [alternatives](#alternatives-module) - Manage system alternatives (update-alternatives)

**Container Modules**:

- [docker_container](#docker-container-module) - Manage Docker containers
- [docker_image](#docker-image-module) - Manage Docker images
- [docker_network](#docker-network-module) - Manage Docker networks
- [docker_volume](#docker-volume-module) - Manage Docker volumes
- [podman_container](#podman-container-module) - Manage Podman containers
- [podman_image](#podman-image-module) - Manage Podman images
- [podman_network](#podman-network-module) - Manage Podman networks
- [podman_volume](#podman-volume-module) - Manage Podman volumes

**Database Modules**:

- [postgres_database](#postgresql-database-module) - Manage PostgreSQL databases
- [postgres_user](#postgresql-user-module) - Manage PostgreSQL users/roles
- [postgres_extension](#postgresql-extension-module) - Manage PostgreSQL extensions
- [mysql_database](#mysql-database-module) - Manage MySQL/MariaDB databases
- [mysql_user](#mysql-user-module) - Manage MySQL/MariaDB users
- [redis](#redis-module) - Manage Redis configuration and ACL users

**Web Server Modules**:

- [nginx_site](#nginx-site-module) - Manage Nginx sites (enable/disable)
- [nginx_config](#nginx-config-module) - Manage Nginx config snippets
- [nginx_upstream](#nginx-upstream-module) - Manage Nginx upstream configurations
- [nginx_proxy](#nginx-proxy-module) - Manage Nginx reverse proxy configurations
- [nginx_ssl](#nginx-ssl-module) - Manage Nginx SSL/TLS configurations
- [nginx_location](#nginx-location-module) - Manage Nginx location blocks
- [nginx_rate_limit](#nginx-rate-limit-module) - Manage Nginx rate limiting
- [apache_site](#apache-site-module) - Manage Apache sites (enable/disable)
- [apache_module](#apache-module-module) - Manage Apache modules (enable/disable)

**Version Control Modules**:

- [git](#git-module) - Manage Git repository clones
- [git_config](#git_config-module) - Manage Git configuration settings

**Certificate Modules**:

- [x509](#x509-module) - Manage X.509 certificates and private keys
- [ca](#ca-module) - Manage Certificate Authority operations
- [acme](#acme-module) - Manage ACME/Let's Encrypt certificates

**Language Package Modules**:

- [pip](#pip-module) - Manage Python packages with pip
- [npm](#npm-module) - Manage Node.js packages with npm
- [gem](#gem-module) - Manage Ruby gems

**Kubernetes Modules**:

- [k8s_namespace](#k8s_namespace-module) - Manage Kubernetes namespaces
- [k8s_deployment](#k8s_deployment-module) - Manage Kubernetes deployments
- [k8s_service](#k8s_service-module) - Manage Kubernetes services
- [k8s_configmap](#k8s_configmap-module) - Manage Kubernetes configmaps
- [k8s_secret](#k8s_secret-module) - Manage Kubernetes secrets
- [k8s_ingress](#k8s_ingress-module) - Manage Kubernetes ingresses
- [k8s_statefulset](#k8s_statefulset-module) - Manage Kubernetes statefulsets
- [k8s_daemonset](#k8s_daemonset-module) - Manage Kubernetes daemonsets
- [k8s_job](#k8s_job-module) - Manage Kubernetes jobs
- [k8s_cronjob](#k8s_cronjob-module) - Manage Kubernetes cronjobs
- [k8s_pvc](#k8s_pvc-module) - Manage Kubernetes persistent volume claims
- [k8s_hpa](#k8s_hpa-module) - Manage Kubernetes horizontal pod autoscalers

**Config File Modules**:

- [logrotate](#logrotate-module) - Manage logrotate configurations
- [sudoers](#sudoers-module) - Manage sudoers configurations
- [limits](#limits-module) - Manage PAM limits configurations
- [modprobe](#modprobe-module) - Manage kernel module configurations
- [syslog](#syslog-module) - Manage syslog/rsyslog configurations
- [lineinfile](#lineinfile-module) - Manage lines in files
- [ini_file](#ini_file-module) - Manage INI file settings
- [archive](#archive-module) - Extract archive files

**Windows Modules**:

- [win_feature](#win_feature-module) - Manage Windows features
- [win_registry](#win_registry-module) - Manage Windows registry
- [win_service](#win_service-module) - Manage Windows services
- [win_firewall](#win_firewall-module) - Manage Windows Firewall rules
- [win_package](#win_package-module) - Manage Windows packages (Chocolatey, winget, MSI, EXE)

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
file:
  nginx_config:
    state: present
    name: /etc/nginx/nginx.conf
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
file:
  app_config:
    state: present
    name: /etc/app/config.yaml
    source: file:///etc/templates/app-config.yaml
    mode: "0600"
    backup: true
```

#### Directory

```yaml
file:
  log_directory:
    state: directory
    name: /var/log/myapp
    owner: myapp
    group: myapp
    mode: "0755"
    makedirs: true
```

#### Symlink

```yaml
file:
  app_symlink:
    state: symlink
    name: /usr/local/bin/myapp
    target: /opt/myapp/bin/myapp
```

#### Remove File

```yaml
file:
  old_config:
    state: absent
    name: /etc/old-config.conf
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
package:
  nginx:
    state: installed
    name: nginx
```

#### Install Specific Version

```yaml
package:
  docker:
    state: installed
    name: docker-ce
    version: "20.10.*"
    update_cache: true
```

#### Install Latest

```yaml
package:
  kubectl:
    state: latest
    name: kubectl
```

#### Remove Package

```yaml
package:
  apache2:
    state: removed
    name: apache2
```

#### Purge Package

```yaml
package:
  old_app:
    state: purged
    name: old-app
```

#### Custom Repository

```yaml
package:
  nginx_mainline:
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

## Pip Module

Manage Python packages using pip.

### States

- `installed` - Ensure package is installed
- `removed` - Ensure package is not installed
- `latest` - Ensure package is at the latest version

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `name` | string | Yes | - | Package name (ID used if not specified) |
| `version` | string | No | - | Specific version to install |
| `pip3` | bool | No | false | Use pip3 instead of pip |
| `requirements` | string | No | - | Path to requirements.txt file |
| `virtualenv` | string | No | - | Path to virtualenv to use |
| `user` | bool | No | false | Install to user site-packages |
| `upgrade` | bool | No | false | Upgrade package if already installed |
| `extra_args` | string | No | - | Additional pip arguments |

### Platform Support

| Platform | Supported | Notes |
|----------|-----------|-------|
| Linux | ✅ | All distributions with pip |
| macOS | ✅ | Requires pip/pip3 |
| Windows | ✅ | Requires pip |

### Examples

#### Install a Package

```yaml
install_requests:
  module: pip
  state: installed
  name: requests
```

#### Install Specific Version

```yaml
install_django:
  module: pip
  state: installed
  name: django
  version: "4.2.0"
```

#### Install with pip3

```yaml
install_flask:
  module: pip
  state: installed
  name: flask
  pip3: true
```

#### Install in Virtualenv

```yaml
install_in_venv:
  module: pip
  state: installed
  name: celery
  virtualenv: /opt/myapp/venv
```

#### Install from Requirements File

```yaml
install_requirements:
  module: pip
  state: installed
  requirements: /opt/myapp/requirements.txt
  virtualenv: /opt/myapp/venv
```

#### User Installation

```yaml
install_for_user:
  module: pip
  state: installed
  name: httpie
  user: true
```

### Return Values

```yaml
result: changed | unchanged
comment: "Package requests installed"
changes:
  installed: true
  version: "2.31.0"
```

## Npm Module

Manage Node.js packages using npm.

### States

- `installed` - Ensure package is installed
- `removed` - Ensure package is not installed
- `latest` - Ensure package is at the latest version

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `name` | string | Yes | - | Package name (ID used if not specified) |
| `version` | string | No | - | Specific version to install |
| `global` | bool | No | false | Install globally |
| `path` | string | No | - | Path to project directory |
| `production` | bool | No | false | Only install production dependencies |
| `registry` | string | No | - | Custom npm registry URL |

### Platform Support

| Platform | Supported | Notes |
|----------|-----------|-------|
| Linux | ✅ | Requires Node.js and npm |
| macOS | ✅ | Requires Node.js and npm |
| Windows | ✅ | Requires Node.js and npm |

### Examples

#### Install a Package Globally

```yaml
install_typescript:
  module: npm
  state: installed
  name: typescript
  global: true
```

#### Install Specific Version

```yaml
install_express:
  module: npm
  state: installed
  name: express
  version: "4.18.2"
  path: /opt/myapp
```

#### Install Project Dependencies

```yaml
install_deps:
  module: npm
  state: installed
  path: /opt/myapp
  production: true
```

#### Remove Package

```yaml
remove_lodash:
  module: npm
  state: removed
  name: lodash
  global: true
```

### Return Values

```yaml
result: changed | unchanged
comment: "Package typescript installed globally"
changes:
  installed: true
  version: "5.3.0"
```

## Gem Module

Manage Ruby gems.

### States

- `installed` - Ensure gem is installed
- `removed` - Ensure gem is not installed
- `latest` - Ensure gem is at the latest version

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `name` | string | Yes | - | Gem name (ID used if not specified) |
| `version` | string | No | - | Specific version to install |
| `user_install` | bool | No | false | Install to user's gem directory |
| `source` | string | No | - | Custom gem source URL |
| `document` | bool | No | true | Generate documentation |

### Platform Support

| Platform | Supported | Notes |
|----------|-----------|-------|
| Linux | ✅ | Requires Ruby and gem |
| macOS | ✅ | Requires Ruby and gem |
| Windows | ✅ | Requires Ruby and gem |

### Examples

#### Install a Gem

```yaml
install_bundler:
  module: gem
  state: installed
  name: bundler
```

#### Install Specific Version

```yaml
install_rails:
  module: gem
  state: installed
  name: rails
  version: "7.1.0"
```

#### Install Without Documentation

```yaml
install_fast:
  module: gem
  state: installed
  name: puma
  document: false
```

#### User Installation

```yaml
install_for_user:
  module: gem
  state: installed
  name: rubocop
  user_install: true
```

### Return Values

```yaml
result: changed | unchanged
comment: "Gem bundler installed"
changes:
  installed: true
  version: "2.4.22"
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
service:
  nginx:
    state: running
    name: nginx
    enabled: true
```

#### Stop and Disable Service

```yaml
service:
  apache2:
    state: stopped
    name: apache2
    enabled: false
```

#### Reload on Config Change

```yaml
service:
  nginx_service:
    state: running
    name: nginx
    reload: true
    watch:
      - file: nginx_config
```

#### Just Enable (Don't Start)

```yaml
service:
  backup_service:
    name: backup
    enabled: true
```

#### Mask Service

```yaml
service:
  unwanted_service:
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
user:
  myapp:
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
user:
  nginx:
    state: present
    name: nginx
    system: true
    shell: /usr/sbin/nologin
    create_home: false
```

#### Remove User

```yaml
user:
  old_user:
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
group:
  developers:
    state: present
    name: developers
    gid: 2000
```

#### Group with Members

```yaml
group:
  docker:
    state: present
    name: docker
    members:
      - alice
      - bob
```

#### System Group

```yaml
group:
  app_group:
    state: present
    name: myapp
    system: true
```

#### Remove Group

```yaml
group:
  old_group:
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
cmd:
  reload_app:
    state: run
    command: systemctl reload myapp
```

#### Command with Environment

```yaml
cmd:
  database_migration:
    state: run
    command: /usr/local/bin/migrate up
    cwd: /opt/myapp
    env:
      DATABASE_URL: postgres://localhost/myapp
    timeout: 5m
```

#### Conditional Execution

```yaml
cmd:
  initialize_db:
    state: run
    command: /opt/myapp/init-db.sh
    unless: test -f /var/lib/myapp/initialized
```

#### Wait State (Run on Change)

```yaml
cmd:
  reload_nginx:
    state: wait
    command: systemctl reload nginx
    watch:
      - file: nginx_config
```

#### Run as User

```yaml
cmd:
  app_task:
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

## Network Module

Configure network interfaces with static IP, DHCP, and advanced settings.

### States

- `configured` - Ensure network interface is configured with static settings
- `absent` - Remove interface configuration
- `dhcp` - Configure interface for DHCP

### Parameters

**interface** (string, optional)

- Network interface name
- Defaults to state ID if not specified
- Example: `eth0`, `enp0s3`, `en0`

**address** (string or list, optional)

- IPv4 address(es) with CIDR notation
- Required for `configured` state (unless using DHCP)
- Accepts single address or list for multiple addresses on one interface
- Example: `192.168.1.100/24` or `["192.168.1.100/24", "192.168.1.101/24"]`

**gateway** (string, optional)

- Default gateway IP address
- Example: `192.168.1.1`

**dns** (string or list, optional)

- DNS server(s)
- Comma-separated string or list of strings
- Example: `8.8.8.8,8.8.4.4` or `["1.1.1.1", "1.0.0.1"]`

**mtu** (int, optional)

- Maximum Transmission Unit
- Example: `1500`, `9000` (jumbo frames)

**metric** (int, optional)

- Route metric for the interface
- Example: `100`

**search_domains** (string or list, optional)

- DNS search domains
- Example: `example.com,corp.example.com`

**dhcp** (bool, optional)

- Enable DHCP for IPv4 (alternative to static configuration)
- Example: `true`

#### IPv6 Parameters

**address6** (string or list, optional)

- IPv6 address(es) with prefix length
- Accepts single address or list for multiple addresses on one interface
- Example: `2001:db8::1/64` or `["2001:db8::1/64", "2001:db8::2/64"]`

**gateway6** (string, optional)

- IPv6 default gateway address
- Example: `2001:db8::ffff`

**dhcp6** (bool, optional)

- Enable DHCPv6 for IPv6 address assignment
- When `true`, uses DHCPv6 for stateful configuration
- Example: `true`

**ipv6_enabled** (bool, optional)

- Enable IPv6 on the interface
- Automatically set to `true` when `address6`, `gateway6`, or `dhcp6` is specified
- Example: `true`

**ipv6_privacy** (bool, optional)

- Enable IPv6 privacy extensions (RFC 4941)
- Generates temporary addresses for outgoing connections
- Example: `true`

**accept_ra** (bool, optional)

- Accept IPv6 Router Advertisements
- When `nil` (not specified), uses system default
- Set to `true` for SLAAC, `false` for purely static configuration
- Example: `true`

#### Link-Layer Parameters

**mac_address** (string, optional)

- Override the interface MAC address
- Format: `xx:xx:xx:xx:xx:xx`
- Note: Not supported on Windows
- Example: `02:42:ac:11:00:02`

**wol** (string, optional)

- Wake-on-LAN mode
- Values: `magic`, `unicast`, `multicast`, `broadcast`, `arp`, `off`
- Also accepts ethtool flags: `g`, `u`, `m`, `b`, `a`, `d`
- Note: macOS uses system-wide setting (pmset), netplan only supports boolean
- Example: `magic`

### Platform Support

The network module auto-detects the network manager on each platform:

| Platform | Network Managers |
|----------|------------------|
| Linux | NetworkManager (nmcli), netplan, ifupdown, systemd-networkd |
| macOS | networksetup |
| Windows | netsh |

### Examples

#### Static IP Configuration

```yaml
eth0_static:
  module: network
  state: configured
  interface: eth0
  address: 192.168.1.100/24
  gateway: 192.168.1.1
  dns:
    - 8.8.8.8
    - 8.8.4.4
```

#### DHCP Configuration

```yaml
eth0_dhcp:
  module: network
  state: dhcp
  interface: eth0
  dhcp: true
```

#### Server with Multiple DNS and Search Domains

```yaml
server_network:
  module: network
  state: configured
  interface: enp0s3
  address: 10.0.0.5/8
  gateway: 10.0.0.1
  dns: "8.8.8.8,8.8.4.4"
  search_domains: "corp.example.com,example.com"
  mtu: 9000
```

#### Using State ID as Interface Name

```yaml
eth0:
  module: network
  state: configured
  address: 192.168.1.100/24
  gateway: 192.168.1.1
```

#### Remove Interface Configuration

```yaml
old_interface:
  module: network
  state: absent
  interface: eth1
```

#### Dual-Stack IPv4/IPv6 Configuration

```yaml
eth0_dual_stack:
  module: network
  state: configured
  interface: eth0
  # IPv4 configuration
  address: 192.168.1.100/24
  gateway: 192.168.1.1
  # IPv6 configuration
  address6: 2001:db8::100/64
  gateway6: 2001:db8::1
  dns:
    - 8.8.8.8
    - 2001:4860:4860::8888
  search_domains:
    - example.com
```

#### Multiple Addresses on One Interface

```yaml
eth0_multi_ip:
  module: network
  state: configured
  interface: eth0
  # Multiple IPv4 addresses
  address:
    - 192.168.1.100/24
    - 192.168.1.101/24
    - 10.0.0.5/8
  gateway: 192.168.1.1
  # Multiple IPv6 addresses
  address6:
    - 2001:db8::100/64
    - 2001:db8::101/64
  gateway6: 2001:db8::1
  dns:
    - 8.8.8.8
    - 2001:4860:4860::8888
```

#### IPv6-Only with Static Address

```yaml
eth0_ipv6_only:
  module: network
  state: configured
  interface: eth0
  address6: 2001:db8::1/64
  gateway6: 2001:db8::ffff
  ipv6_enabled: true
  dns:
    - 2001:4860:4860::8888
    - 2606:4700:4700::1111
```

#### DHCPv6 Configuration

```yaml
eth0_dhcpv6:
  module: network
  state: dhcp
  interface: eth0
  dhcp: true
  dhcp6: true
```

#### IPv6 SLAAC with Privacy Extensions

```yaml
eth0_slaac:
  module: network
  state: configured
  interface: eth0
  address: 192.168.1.100/24
  ipv6_enabled: true
  accept_ra: true
  ipv6_privacy: true
```

#### MAC Address Override and Wake-on-LAN

```yaml
eth0_wol:
  module: network
  state: configured
  interface: eth0
  address: 192.168.1.100/24
  gateway: 192.168.1.1
  mac_address: "02:42:ac:11:00:02"
  wol: magic
```

### Return Values

```yaml
result: changed | unchanged
comment: "Applied static configuration to eth0"
changes:
  address:
    current: "192.168.1.50/24"
    desired: "192.168.1.100/24"
  gateway:
    current: ""
    desired: "192.168.1.1"
```

### Network Manager Detection

The module automatically detects the available network manager:

**Linux:**

1. NetworkManager - if `nmcli` is available
2. netplan - if `/etc/netplan/` exists
3. systemd-networkd - if active
4. ifupdown - if `/etc/network/interfaces` exists

**macOS:**

- Uses `networksetup` command

**Windows:**

- Uses `netsh` command

### Idempotency

The network module is fully idempotent:

- Configuring an already-configured interface with same settings: no change
- Changing IP address or gateway: updates configuration
- Removing non-existent configuration: no change

### Error Handling

Common errors:

- **Interface not found**: Interface must exist on the system
- **Invalid address format**: Use CIDR notation (e.g., `192.168.1.100/24`)
- **Permission denied**: Network configuration typically requires root/admin privileges

## VLAN Module

Create and manage VLAN sub-interfaces.

### States

- `present` - Ensure VLAN interface exists
- `absent` - Ensure VLAN interface does not exist

### Parameters

**parent** (string, required)

- Parent interface for the VLAN
- Example: `eth0`, `enp0s3`

**id** (int, required)

- VLAN ID (1-4094)
- Also accepts `vlan_id` as parameter name
- Example: `100`, `200`

**addresses** (string or list, optional)

- IP address(es) for the VLAN interface
- Example: `["192.168.100.1/24"]`

**gateway** (string, optional)

- Default gateway for the VLAN
- Example: `192.168.100.254`

**dns** (string or list, optional)

- DNS servers
- Example: `["8.8.8.8"]`

**mtu** (int, optional)

- MTU (inherits from parent if not set)
- Example: `1500`

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | Full (nmcli, netplan, systemd-networkd, ifupdown) |
| macOS | Not supported |
| Windows | Not supported |

### Examples

#### Basic VLAN

```yaml
eth0.100:
  module: vlan
  state: present
  parent: eth0
  id: 100
```

#### VLAN with IP Configuration

```yaml
vlan_office:
  module: vlan
  state: present
  parent: eth0
  id: 200
  addresses:
    - 192.168.200.1/24
  gateway: 192.168.200.254
  dns:
    - 8.8.8.8
```

---

## WiFi Module

Manage wireless network connections and profiles.

### States

- `connected` - Actively connected to this WiFi network
- `configured` - Network profile stored but not necessarily connected
- `absent` - Remove network profile

### Parameters

**ssid** (string, required)

- WiFi network name
- Maximum 32 characters
- Example: `"Office WiFi"`

**security** (string, optional)

- Security mode: `wpa2-psk`, `wpa3`, `wep`, `open`
- Aliases: `wpa2` → `wpa2-psk`, `none` → `open`
- Default: `wpa2-psk`
- Example: `"wpa2-psk"`

**password** (string, conditional)

- WiFi password
- Required for `wpa2-psk`, `wpa3`, and `wep` security modes
- WPA: 8-63 characters
- WEP: 5 or 13 characters (40-bit or 104-bit)
- Example: `"{{ vault.wifi_password }}"`

**interface** (string, optional)

- WiFi interface name
- Default: auto-detected or `wlan0` (Linux), `en0` (macOS), `Wi-Fi` (Windows)
- Example: `"wlan0"`

**priority** (int, optional)

- Network priority for auto-connection (0-100)
- Higher values = higher priority
- Default: `0`
- Example: `100`

**hidden** (bool, optional)

- Whether the network SSID is hidden
- Default: `false`
- Example: `true`

**auto_connect** (bool, optional)

- Whether to auto-connect to this network
- Default: `true`
- Example: `false`

**bssid** (string, optional)

- Specific access point BSSID/MAC address
- Example: `"00:11:22:33:44:55"`

**name** (string, optional)

- Connection/profile name (defaults to SSID)
- Example: `"Work Network"`

### Platform Support

| Platform | Backend | Support |
|----------|---------|---------|
| Linux | NetworkManager (nmcli) | Full |
| Linux | wpa_supplicant | Full |
| macOS | networksetup | Full |
| Windows | netsh wlan | Full |

### Examples

#### Basic WPA2 Network

```yaml
office_wifi:
  module: wifi
  state: connected
  ssid: "Office WiFi"
  security: wpa2-psk
  password: "{{ vault.wifi_password }}"
```

#### Open Network (Guest)

```yaml
guest_network:
  module: wifi
  state: configured
  ssid: "Guest Network"
  security: open
```

#### Hidden Network with Priority

```yaml
secure_wifi:
  module: wifi
  state: connected
  ssid: "Hidden Secure Network"
  security: wpa3
  password: "{{ vault.secure_wifi_password }}"
  hidden: true
  priority: 100
  auto_connect: true
```

#### Remove Network Profile

```yaml
old_network:
  module: wifi
  state: absent
  ssid: "Old Network"
```

#### Specific Interface

```yaml
secondary_wifi:
  module: wifi
  state: connected
  ssid: "Secondary Network"
  security: wpa2-psk
  password: "mypassword"
  interface: wlan1
```

---

## 802.1X Module (dot1x)

Configure 802.1X authentication for wired and wireless network access control.

### States

- `enabled` - 802.1X authentication is active on the interface
- `disabled` - 802.1X authentication is removed/disabled

### Parameters

**interface** (string, required)

- Network interface for 802.1X authentication
- Example: `"eth0"`, `"en0"`

**eap_method** (string, required for enabled state)

- EAP authentication method: `tls`, `ttls`, `peap`
- Alias: `eap`
- Example: `"tls"`

**identity** (string, required for enabled state)

- User identity/username for authentication
- Example: `"user@example.com"`

**password** (string, conditional)

- Password for EAP-TTLS and EAP-PEAP methods
- Required when `eap_method` is `ttls` or `peap`
- Example: `"{{ vault.radius_password }}"`

**client_cert** (string, conditional)

- Path to client certificate for EAP-TLS
- Required when `eap_method` is `tls`
- Example: `"/etc/pki/client.crt"`

**client_key** (string, conditional)

- Path to client private key for EAP-TLS
- Required when `eap_method` is `tls`
- Example: `"/etc/pki/client.key"`

**ca_cert** (string, optional)

- Path to CA certificate for server validation
- Recommended for all methods
- Example: `"/etc/pki/ca.crt"`

**phase2** (string, optional)

- Inner authentication method for TTLS/PEAP: `mschapv2`, `pap`, `chap`, `md5`, `gtc`
- Alias: `inner_auth`
- Default: `mschapv2`
- Example: `"mschapv2"`

**anonymous** (string, optional)

- Anonymous identity for outer authentication (privacy)
- Alias: `anonymous_identity`
- Example: `"anonymous@example.com"`

**name** (string, optional)

- Connection/profile name (defaults to `dot1x-<interface>`)
- Example: `"corporate-auth"`

### Platform Support

| Platform | Backend | Support |
|----------|---------|---------|
| Linux | NetworkManager (nmcli) | Full |
| Linux | wpa_supplicant | Full |
| Windows | netsh lan (dot3svc) | Full |
| macOS | Configuration Profiles | Full |

### Examples

#### EAP-TLS with Certificates

```yaml
wired_tls_auth:
  module: dot1x
  state: enabled
  interface: eth0
  eap_method: tls
  identity: "user@example.com"
  client_cert: /etc/pki/tls/certs/client.crt
  client_key: /etc/pki/tls/private/client.key
  ca_cert: /etc/pki/tls/certs/ca.crt
```

#### EAP-PEAP with Password

```yaml
wired_peap_auth:
  module: dot1x
  state: enabled
  interface: eth0
  eap_method: peap
  identity: "jdoe"
  password: "{{ vault.radius_password }}"
  phase2: mschapv2
  ca_cert: /etc/ssl/certs/corporate-ca.pem
```

#### EAP-TTLS with Anonymous Identity

```yaml
secure_wired:
  module: dot1x
  state: enabled
  interface: eth0
  eap_method: ttls
  identity: "user@corp.example.com"
  password: "{{ vault.network_password }}"
  phase2: pap
  anonymous: "anonymous@corp.example.com"
  ca_cert: /etc/ssl/certs/radius-ca.pem
```

#### Disable 802.1X

```yaml
remove_auth:
  module: dot1x
  state: disabled
  interface: eth0
```

---

## Link Module

Configure network interface link settings including speed, duplex, auto-negotiation, MTU, and Wake-on-LAN.

### States

- `configured` - Apply specific link settings
- `default` - Reset to auto-negotiation

### Parameters

**interface** (string, required)

- Network interface name (can use declaration ID)
- Example: `"eth0"`, `"enp0s3"`

**speed** (int, optional)

- Link speed in Mbps: 10, 100, 1000, 2500, 5000, 10000, 25000, 40000, 100000
- Example: `1000`

**duplex** (string, optional)

- Duplex mode: `full`, `half`
- Example: `"full"`

**autoneg** (bool, optional)

- Enable/disable auto-negotiation
- Alias: `auto_negotiation`
- Example: `false`

**mtu** (int, optional)

- Maximum Transmission Unit (68-65535)
- Example: `9000` (jumbo frames)

**wol** (string, optional)

- Wake-on-LAN mode: `disabled`, `magic`, `unicast`, `multicast`, `broadcast`, `arp`
- Alias: `wake_on_lan`
- Example: `"magic"`

### Platform Support

| Platform | Backend | Support |
|----------|---------|---------|
| Linux | ethtool | Full |
| macOS | ifconfig/networksetup | Partial (speed/duplex/MTU) |
| Windows | netsh/PowerShell | Full |

### Examples

#### Force 1 Gbps Full Duplex

```yaml
eth0:
  module: link
  state: configured
  speed: 1000
  duplex: full
  autoneg: false
```

#### Jumbo Frames (MTU 9000)

```yaml
storage_nic:
  module: link
  state: configured
  interface: eth1
  mtu: 9000
```

#### Enable Wake-on-LAN

```yaml
server_nic:
  module: link
  state: configured
  interface: eth0
  wol: magic
```

#### Force 100 Mbps for Legacy Device

```yaml
legacy_switch_port:
  module: link
  state: configured
  interface: eth2
  speed: 100
  duplex: full
  autoneg: false
```

#### Reset to Auto-Negotiation

```yaml
eth0:
  module: link
  state: default
```

#### Combined Settings

```yaml
high_perf_nic:
  module: link
  state: configured
  interface: enp0s25
  speed: 10000
  duplex: full
  autoneg: false
  mtu: 9000
  wol: disabled
```

---

## Promiscuous Mode Module (promisc)

Enable or disable promiscuous mode on network interfaces for packet capture, bridging, or IDS/IPS systems.

### States

- `enabled` - Promiscuous mode is active
- `disabled` - Promiscuous mode is disabled (normal operation)

### Parameters

**interface** (string, required)

- Network interface name (can use declaration ID)
- Example: `"eth0"`, `"enp0s3"`

**allmulti** (bool, optional, Linux only)

- Also enable/disable all-multicast mode
- Alias: `all_multicast`
- Default: `false`
- Example: `true`

### Platform Support

| Platform | Backend | Support |
|----------|---------|---------|
| Linux | ip link | Full (promisc + allmulti) |
| macOS | ifconfig | Full |
| FreeBSD | ifconfig | Full |
| Windows | PowerShell | Partial (adapter-dependent) |

### Security Considerations

Promiscuous mode allows the interface to receive all packets on the network segment, not just those addressed to it. This is useful for:

- **Packet capture**: Network troubleshooting with tcpdump/Wireshark
- **Bridging**: Software bridges need to see all traffic
- **IDS/IPS**: Intrusion detection systems monitoring traffic
- **Network monitoring**: Traffic analysis tools

**Warning**: Enabling promiscuous mode on production interfaces may have security implications. Ensure proper authorization and audit logging.

### Examples

#### Enable Promiscuous Mode

```yaml
eth0:
  module: promisc
  state: enabled
```

#### Enable with All-Multicast (Linux)

```yaml
monitor_interface:
  module: promisc
  state: enabled
  interface: eth1
  allmulti: true
```

#### Disable Promiscuous Mode

```yaml
eth0:
  module: promisc
  state: disabled
```

#### Named Monitor Interface

```yaml
ids_capture:
  module: promisc
  state: enabled
  interface: enp0s25
```

---

## Bond Module

Create and manage network bonding/teaming for link aggregation.

### States

- `present` - Ensure bond interface exists
- `absent` - Ensure bond interface does not exist

### Parameters

**slaves** (list, required)

- Slave/member interfaces
- Minimum 1 interface required
- Example: `["eth0", "eth1"]`

**mode** (string, required)

- Bonding mode
- Values: `balance-rr` (0), `active-backup` (1), `balance-xor` (2), `broadcast` (3), `802.3ad` (4), `balance-tlb` (5), `balance-alb` (6)
- Example: `active-backup`, `802.3ad`

**miimon** (int, optional)

- MII link monitoring interval in milliseconds
- Default: `100`
- Example: `50`

**primary** (string, optional)

- Primary interface for active-backup mode
- Example: `eth0`

**lacp_rate** (string, optional)

- LACP rate for 802.3ad mode
- Values: `slow`, `fast`
- Example: `fast`

**xmit_hash_policy** (string, optional)

- Transmit hash policy
- Values: `layer2`, `layer2+3`, `layer3+4`
- Example: `layer3+4`

**updelay** (int, optional)

- Delay before enabling slave (ms)
- Example: `200`

**downdelay** (int, optional)

- Delay before disabling slave (ms)
- Example: `200`

**addresses** (string or list, optional)

- IP address(es) for the bond interface
- Example: `["10.0.0.1/24"]`

**gateway** (string, optional)

- Default gateway
- Example: `10.0.0.254`

**dns** (string or list, optional)

- DNS servers
- Example: `["8.8.8.8"]`

**mtu** (int, optional)

- MTU for the bond interface
- Example: `9000`

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | Full (nmcli, netplan, systemd-networkd, ifupdown) |
| macOS | Not supported |
| Windows | NIC Teaming via PowerShell |

### Examples

#### Active-Backup Bond

```yaml
bond0:
  module: bond
  state: present
  slaves:
    - eth0
    - eth1
  mode: active-backup
  primary: eth0
  addresses:
    - 10.0.0.1/24
  gateway: 10.0.0.254
```

#### LACP Bond

```yaml
bond0:
  module: bond
  state: present
  slaves:
    - eth0
    - eth1
  mode: 802.3ad
  lacp_rate: fast
  xmit_hash_policy: layer3+4
  miimon: 50
  addresses:
    - 10.0.0.1/24
```

---

## Bridge Module

Create and manage network bridges for virtualization and container networking.

### States

- `present` - Ensure bridge interface exists
- `absent` - Ensure bridge interface does not exist

### Parameters

**ports** (list, optional)

- Port/member interfaces
- Also accepts `interfaces` as parameter name
- Example: `["eth0", "eth1"]`

**stp** (bool, optional)

- Enable Spanning Tree Protocol
- Default: `false`
- Example: `true`

**forward_delay** (int, optional)

- STP forward delay in seconds
- Default: `15`
- Example: `4`

**hello_time** (int, optional)

- STP hello time in seconds
- Default: `2`
- Example: `1`

**max_age** (int, optional)

- STP max age in seconds
- Default: `20`
- Example: `10`

**ageing_time** (int, optional)

- MAC address ageing time in seconds
- Default: `300`
- Example: `600`

**addresses** (string or list, optional)

- IP address(es) for the bridge interface
- Example: `["192.168.1.1/24"]`

**gateway** (string, optional)

- Default gateway
- Example: `192.168.1.254`

**dns** (string or list, optional)

- DNS servers
- Example: `["8.8.8.8"]`

**mtu** (int, optional)

- MTU for the bridge interface
- Example: `1500`

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | Full (nmcli, netplan, systemd-networkd, ifupdown) |
| macOS | Not supported |
| Windows | Hyper-V Virtual Switch via PowerShell |

### Examples

#### Basic Bridge

```yaml
br0:
  module: bridge
  state: present
  ports:
    - eth0
    - eth1
  addresses:
    - 192.168.1.1/24
  gateway: 192.168.1.254
```

#### Bridge with STP

```yaml
br0:
  module: bridge
  state: present
  ports:
    - eth0
    - eth1
  stp: true
  forward_delay: 4
  addresses:
    - 192.168.1.1/24
```

#### Isolated Bridge (no ports)

```yaml
br_internal:
  module: bridge
  state: present
  addresses:
    - 172.16.0.1/24
```

---

## Route Module

Manage static routes across platforms.

### States

- `present` - Ensure route exists
- `absent` - Ensure route does not exist

### Parameters

**destination** (string, optional)

- Destination network in CIDR notation
- Defaults to state ID if not specified
- Special values: `default`, `0.0.0.0/0`
- Example: `10.0.0.0/8`, `192.168.100.0/24`

**gateway** (string, required*)

- Gateway IP address for the route
- Required unless `interface` is specified
- Example: `192.168.1.1`

**interface** (string, required*)

- Network interface for the route
- Required unless `gateway` is specified
- Example: `eth0`, `en0`

**metric** (int, optional)

- Route metric/priority
- Lower values = higher priority
- Example: `100`

**table** (string, optional, Linux only)

- Routing table name
- Example: `main`, `custom`

### Platform Support

| Platform | Command |
|----------|---------|
| Linux | `ip route` |
| macOS | `route` |
| Windows | `route` (persistent with `-p` flag) |

### Examples

#### Route via Gateway

```yaml
office_network:
  module: route
  state: present
  destination: 10.0.0.0/8
  gateway: 192.168.1.1
```

#### Route via Interface

```yaml
direct_route:
  module: route
  state: present
  destination: 172.16.0.0/12
  interface: eth0
```

#### Default Route

```yaml
default:
  module: route
  state: present
  gateway: 192.168.1.1
```

#### Route with Metric

```yaml
backup_route:
  module: route
  state: present
  destination: 10.10.0.0/16
  gateway: 10.0.0.1
  metric: 100
```

#### Route in Custom Table (Linux)

```yaml
custom_table_route:
  module: route
  state: present
  destination: 192.168.100.0/24
  gateway: 192.168.1.1
  table: custom
```

#### Host Route (Single IP)

```yaml
host_route:
  module: route
  state: present
  destination: 192.168.100.10
  gateway: 192.168.1.1
```

#### Using State ID as Destination

```yaml
10.0.0.0/8:
  module: route
  state: present
  gateway: 192.168.1.1
```

#### Remove Route

```yaml
old_route:
  module: route
  state: absent
  destination: 10.20.0.0/16
  gateway: 192.168.1.1
```

### Return Values

```yaml
result: changed | unchanged
comment: "Added route to 10.0.0.0/8 via 192.168.1.1"
changes:
  route:
    current: "absent"
    desired: "present"
```

### Idempotency

The route module is fully idempotent:

- Adding an existing route with same gateway: no change
- Adding an existing route with different gateway: deletes old, adds new
- Removing a non-existent route: no change

### Validation

The module validates:

- **Destination format**: Must be valid CIDR, IP address, `default`, or `0.0.0.0/0`
- **Gateway or interface required**: At least one must be specified
- **Gateway format**: Must be valid IP address

### Error Handling

Common errors:

- **Invalid destination**: Use valid CIDR notation or `default`
- **Must specify gateway or interface**: At least one is required
- **Permission denied**: Route management typically requires root/admin privileges
- **Network unreachable**: Gateway must be reachable from the system

## Firewall Module

Cross-platform firewall management with automatic backend detection.

### Overview

The firewall module provides a unified interface for managing firewall rules across different platforms:

| Platform | Backend | Detection |
|----------|---------|-----------|
| Linux | iptables | `iptables --version` |
| Linux | nftables | `nft --version` |
| Linux | firewalld | `firewall-cmd --state` |
| macOS | pf | Built-in |
| Windows | netsh | Built-in |

The module automatically detects the available backend and uses the appropriate commands. Priority order on Linux: firewalld → nftables → iptables.

### States

- `present` - Ensure firewall rule exists
- `absent` - Ensure firewall rule does not exist

### Parameters

**protocol** (string, optional)

- Network protocol for the rule
- Values: `tcp`, `udp`, `icmp`, `all`
- Default: `tcp`

**port** (int, optional)

- Single port number
- Range: 1-65535
- Example: `80`, `443`, `22`

**port_range** (string, optional)

- Port range in format `start:end`
- Example: `8000:8080`

**source** (string, optional)

- Source IP address or CIDR
- Example: `192.168.1.0/24`, `10.0.0.1`

**destination** (string, optional)

- Destination IP address or CIDR
- Example: `0.0.0.0/0`

**interface** (string, optional)

- Network interface
- Example: `eth0`, `enp0s3`

**action** (string, required)

- Action to take on matching traffic
- Values: `accept`, `drop`, `reject`

**direction** (string, optional)

- Traffic direction
- Values: `input`, `output`, `forward`
- Default: `input`

**zone** (string, optional)

- Firewalld zone (Linux with firewalld only)
- Default: `public`

**chain** (string, optional)

- iptables/nftables chain
- Default: Derived from direction

**table** (string, optional)

- iptables/nftables table
- Default: `filter`

**profile** (string, optional)

- Windows Firewall profile
- Values: `domain`, `private`, `public`, `any`
- Default: `any`

**comment** (string, optional)

- Rule description/comment
- Used for identification and documentation

### Requirements

- **Linux (iptables)**: iptables package installed
- **Linux (nftables)**: nft package installed
- **Linux (firewalld)**: firewalld service running
- **macOS**: pf enabled (default on macOS)
- **Windows**: Windows Firewall service running
- All platforms require root/admin privileges

### Examples

#### Allow SSH Access

```yaml
allow_ssh:
  module: firewall
  state: present
  protocol: tcp
  port: 22
  action: accept
  direction: input
  comment: "Allow SSH access"
```

#### Allow HTTP/HTTPS

```yaml
allow_web:
  module: firewall
  state: present
  protocol: tcp
  port_range: "80:443"
  action: accept
  direction: input
```

#### Block IP Address

```yaml
block_bad_actor:
  module: firewall
  state: present
  source: "203.0.113.50"
  action: drop
  direction: input
  comment: "Block malicious IP"
```

#### Allow Subnet Access

```yaml
allow_internal:
  module: firewall
  state: present
  source: "10.0.0.0/8"
  protocol: tcp
  port: 5432
  action: accept
  comment: "Allow internal PostgreSQL access"
```

#### Remove Rule

```yaml
remove_old_rule:
  module: firewall
  state: absent
  protocol: tcp
  port: 8080
  action: accept
```

### Platform-Specific Behavior

#### Linux (iptables)

- Rules inserted at top of chain for immediate effect
- Uses `-C` to check rule existence
- Supports custom chains and tables

#### Linux (nftables)

- Creates table/chain if not exists
- Rules identified by content matching
- Supports inet family for IPv4/IPv6

#### Linux (firewalld)

- Uses zone-based configuration
- Rich rules for complex matching
- Changes are permanent by default

#### macOS (pf)

- Rules added to keystone anchor
- Requires `pfctl -e` to enable
- Uses anchor isolation for safety

#### Windows

- Uses netsh advfirewall
- Supports profile-based rules
- Rule names derived from comment

### Idempotency

The firewall module is fully idempotent:

- Adding an existing rule: no change
- Removing a non-existent rule: no change
- Rules matched by all specified parameters

### Error Handling

Common errors:

- **Backend not found**: No supported firewall backend detected
- **Permission denied**: Firewall management requires root/admin
- **Invalid port**: Port must be 1-65535
- **Invalid action**: Must be accept, drop, or reject

## Iptables Module

Direct iptables management for Linux systems with full control over tables, chains, and rules.

### States

- `present` - Ensure rule exists in chain
- `absent` - Ensure rule does not exist
- `flush` - Remove all rules from a chain
- `policy` - Set default policy for a chain

### Parameters

**table** (string, optional)

- iptables table
- Values: `filter`, `nat`, `mangle`, `raw`, `security`
- Default: `filter`

**chain** (string, required)

- Chain name
- Built-in: `INPUT`, `OUTPUT`, `FORWARD`, `PREROUTING`, `POSTROUTING`
- Or custom chain name

**protocol** (string, optional)

- Protocol to match
- Values: `tcp`, `udp`, `icmp`, `all`
- Example: `tcp`

**source** (string, optional)

- Source IP/CIDR
- Example: `192.168.1.0/24`

**destination** (string, optional)

- Destination IP/CIDR
- Example: `10.0.0.0/8`

**source_port** (string, optional)

- Source port or range
- Example: `1024:65535`

**destination_port** (string, optional)

- Destination port or range
- Example: `80`, `8000:8080`

**interface_in** (string, optional)

- Input interface
- Example: `eth0`

**interface_out** (string, optional)

- Output interface
- Example: `eth1`

**jump** (string, required for present/absent)

- Target action
- Values: `ACCEPT`, `DROP`, `REJECT`, `LOG`, `RETURN`, or custom chain
- Example: `ACCEPT`

**match** (list, optional)

- Extended match modules
- Example: `["state", "multiport"]`

**match_state** (string, optional)

- Connection state for state match
- Values: `NEW`, `ESTABLISHED`, `RELATED`, `INVALID`
- Example: `NEW,ESTABLISHED`

**log_prefix** (string, optional)

- Prefix for LOG target
- Example: `"DROPPED: "`

**reject_with** (string, optional)

- ICMP type for REJECT target
- Example: `icmp-port-unreachable`

**position** (int, optional)

- Rule position in chain (1-based)
- Default: append to end

**policy** (string, required for policy state)

- Default policy for chain
- Values: `ACCEPT`, `DROP`

**comment** (string, optional)

- Rule comment (requires comment match module)

### Requirements

- Linux operating system
- iptables package installed
- Root privileges

### Examples

#### Allow Established Connections

```yaml
allow_established:
  module: iptables
  state: present
  table: filter
  chain: INPUT
  match:
    - state
  match_state: "ESTABLISHED,RELATED"
  jump: ACCEPT
```

#### Block All from IP

```yaml
block_attacker:
  module: iptables
  state: present
  chain: INPUT
  source: "203.0.113.100"
  jump: DROP
```

#### DNAT for Port Forwarding

```yaml
port_forward_web:
  module: iptables
  state: present
  table: nat
  chain: PREROUTING
  protocol: tcp
  destination_port: "80"
  jump: DNAT
  to_destination: "192.168.1.100:8080"
```

#### SNAT for Outbound NAT

```yaml
masquerade_outbound:
  module: iptables
  state: present
  table: nat
  chain: POSTROUTING
  interface_out: eth0
  jump: MASQUERADE
```

#### Log Dropped Packets

```yaml
log_drops:
  module: iptables
  state: present
  chain: INPUT
  jump: LOG
  log_prefix: "IPTables-Dropped: "
  position: 1
```

#### Set Default Policy

```yaml
default_drop:
  module: iptables
  state: policy
  chain: INPUT
  policy: DROP
```

#### Flush Chain

```yaml
flush_input:
  module: iptables
  state: flush
  chain: INPUT
```

### Chain Management

Built-in chains per table:

| Table | Chains |
|-------|--------|
| filter | INPUT, OUTPUT, FORWARD |
| nat | PREROUTING, POSTROUTING, OUTPUT |
| mangle | PREROUTING, INPUT, OUTPUT, FORWARD, POSTROUTING |
| raw | PREROUTING, OUTPUT |

### Idempotency

- Rules checked with `iptables -C` before adding
- Position-specific inserts use `-I chain position`
- Flush and policy operations always execute

### Error Handling

Common errors:

- **Chain not found**: Verify chain name and table
- **Bad rule**: Check parameter syntax
- **Permission denied**: Requires root privileges

## Nftables Module

Modern nftables firewall management for Linux with atomic rule updates.

### States

- `present` - Ensure rule exists (creates table/chain if needed)
- `absent` - Ensure rule does not exist

### Parameters

**family** (string, optional)

- Address family
- Values: `ip` (IPv4), `ip6` (IPv6), `inet` (both), `arp`, `bridge`, `netdev`
- Default: `inet`

**table** (string, required)

- Table name
- Example: `filter`, `nat`, `mangle`

**chain** (string, required)

- Chain name
- Example: `input`, `output`, `forward`

**chain_type** (string, optional)

- Chain type (for base chains)
- Values: `filter`, `nat`, `route`
- Default: `filter`

**chain_hook** (string, optional)

- Netfilter hook (for base chains)
- Values: `input`, `output`, `forward`, `prerouting`, `postrouting`

**chain_priority** (int, optional)

- Chain priority
- Default: `0`

**chain_policy** (string, optional)

- Default chain policy
- Values: `accept`, `drop`
- Default: `accept`

**protocol** (string, optional)

- Protocol to match
- Example: `tcp`, `udp`, `icmp`

**source** (string, optional)

- Source address
- Example: `192.168.1.0/24`

**destination** (string, optional)

- Destination address
- Example: `10.0.0.0/8`

**source_port** (int, optional)

- Source port

**destination_port** (int, optional)

- Destination port
- Example: `443`

**interface_in** (string, optional)

- Input interface
- Example: `eth0`

**interface_out** (string, optional)

- Output interface

**counter** (bool, optional)

- Enable packet/byte counters
- Default: `false`

**action** (string, required)

- Rule action
- Values: `accept`, `drop`, `reject`, `return`, `jump`, `goto`

**jump_target** (string, optional)

- Target chain for jump/goto actions

**log** (bool, optional)

- Log matching packets
- Default: `false`

**log_prefix** (string, optional)

- Log message prefix

**comment** (string, optional)

- Rule comment

### Requirements

- Linux operating system
- nftables package (nft command)
- Root privileges

### Examples

#### Create Filter Table with Input Chain

```yaml
setup_filter:
  module: nftables
  state: present
  family: inet
  table: filter
  chain: input
  chain_type: filter
  chain_hook: input
  chain_priority: 0
  chain_policy: drop
  # No rule, just creates table/chain
```

#### Allow SSH

```yaml
allow_ssh:
  module: nftables
  state: present
  family: inet
  table: filter
  chain: input
  protocol: tcp
  destination_port: 22
  action: accept
  counter: true
  comment: "Allow SSH"
```

#### Allow Established Connections

```yaml
allow_established:
  module: nftables
  state: present
  family: inet
  table: filter
  chain: input
  ct_state: "established,related"
  action: accept
```

#### Block IP Range

```yaml
block_range:
  module: nftables
  state: present
  family: inet
  table: filter
  chain: input
  source: "10.99.0.0/16"
  action: drop
  log: true
  log_prefix: "Blocked: "
```

#### NAT Masquerade

```yaml
nat_masq:
  module: nftables
  state: present
  family: ip
  table: nat
  chain: postrouting
  chain_type: nat
  chain_hook: postrouting
  chain_priority: 100
  interface_out: eth0
  action: masquerade
```

#### Remove Rule

```yaml
remove_old:
  module: nftables
  state: absent
  family: inet
  table: filter
  chain: input
  protocol: tcp
  destination_port: 8080
  action: accept
```

### Atomic Operations

nftables supports atomic ruleset updates:

- Tables and chains created automatically if needed
- Rules added/removed in single transaction
- No packet loss during updates

### Idempotency

- Table/chain existence checked before creation
- Rules matched by content (protocol, ports, addresses, action)
- Missing rules added, existing rules preserved

### Error Handling

Common errors:

- **nft not found**: Install nftables package
- **Permission denied**: Requires root
- **Invalid family**: Use ip, ip6, inet, arp, bridge, or netdev

## Firewalld Module

Zone-based firewall management for Linux systems using firewalld.

### States

- `present` - Add service/port/rule to zone
- `absent` - Remove service/port/rule from zone

### Parameters

**zone** (string, optional)

- Firewalld zone
- Common zones: `public`, `internal`, `external`, `dmz`, `trusted`, `drop`
- Default: `public`

**service** (string, optional)

- Predefined service name
- Example: `ssh`, `http`, `https`, `mysql`, `postgresql`
- List available: `firewall-cmd --get-services`

**port** (int, optional)

- Port number
- Range: 1-65535

**protocol** (string, optional)

- Protocol for port rules
- Values: `tcp`, `udp`
- Default: `tcp`

**source** (string, optional)

- Source IP/CIDR to add to zone
- Example: `192.168.1.0/24`

**interface** (string, optional)

- Interface to add to zone
- Example: `eth0`

**rich_rule** (string, optional)

- Rich rule specification
- Full firewalld rich language syntax
- Example: `rule family="ipv4" source address="10.0.0.0/8" accept`

**masquerade** (bool, optional)

- Enable/disable masquerading for zone
- Default: `false`

**icmp_block** (string, optional)

- ICMP type to block
- Example: `echo-request`

**forward_port** (map, optional)

- Port forwarding configuration
- Keys: `port`, `protocol`, `to_port`, `to_addr`

**permanent** (bool, optional)

- Make changes permanent (survive reboot)
- Default: `true`

**immediate** (bool, optional)

- Apply changes to runtime immediately
- Default: `true`

### Requirements

- Linux operating system
- firewalld service installed and running
- Root privileges

### Examples

#### Allow SSH Service

```yaml
allow_ssh:
  module: firewalld
  state: present
  zone: public
  service: ssh
```

#### Allow Custom Port

```yaml
allow_app_port:
  module: firewalld
  state: present
  zone: public
  port: 8080
  protocol: tcp
```

#### Add Source to Trusted Zone

```yaml
trust_internal:
  module: firewalld
  state: present
  zone: trusted
  source: "10.0.0.0/8"
```

#### Assign Interface to Zone

```yaml
internal_interface:
  module: firewalld
  state: present
  zone: internal
  interface: eth1
```

#### Enable Masquerading

```yaml
enable_nat:
  module: firewalld
  state: present
  zone: external
  masquerade: true
```

#### Rich Rule for Rate Limiting

```yaml
rate_limit_ssh:
  module: firewalld
  state: present
  zone: public
  rich_rule: 'rule family="ipv4" service name="ssh" accept limit value="10/m"'
```

#### Port Forwarding

```yaml
forward_web:
  module: firewalld
  state: present
  zone: public
  forward_port:
    port: 80
    protocol: tcp
    to_port: 8080
    to_addr: "192.168.1.100"
```

#### Block ICMP Ping

```yaml
block_ping:
  module: firewalld
  state: present
  zone: public
  icmp_block: echo-request
```

#### Remove Service

```yaml
remove_telnet:
  module: firewalld
  state: absent
  zone: public
  service: telnet
```

### Zone Reference

| Zone | Description |
|------|-------------|
| drop | Drop all incoming, allow outgoing |
| block | Reject all incoming with icmp-prohibited |
| public | Untrusted networks (default) |
| external | External networks with masquerading |
| dmz | DMZ zone, limited access |
| work | Work network, more trusted |
| home | Home network, trusted |
| internal | Internal network |
| trusted | All traffic allowed |

### Idempotency

- Services/ports checked before adding
- Source/interface membership verified
- Rich rules matched by exact content
- No changes if already in desired state

### Runtime vs Permanent

With default settings (`permanent: true`, `immediate: true`):

- Changes saved to permanent configuration
- Changes also applied to current runtime
- Survives firewalld restart and reboot

### Error Handling

Common errors:

- **Firewalld not running**: Start with `systemctl start firewalld`
- **Zone not found**: Check available zones with `firewall-cmd --get-zones`
- **Service not found**: Check available services with `firewall-cmd --get-services`
- **Permission denied**: Requires root privileges

## UFW Module

Manage Ubuntu Uncomplicated Firewall (UFW) rules.

### Platform

Linux only (Ubuntu, Debian with UFW installed)

### States

- `enabled` - Enable UFW
- `disabled` - Disable UFW
- `allow` - Allow traffic matching rule
- `deny` - Deny traffic matching rule
- `reject` - Reject traffic with ICMP response
- `absent` - Remove rule

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `port` | string | No | - | Port number or range (e.g., "22", "6000:6007") |
| `proto` | string | No | - | Protocol: tcp, udp, or both |
| `from` | string | No | "any" | Source IP or network |
| `to` | string | No | "any" | Destination IP or network |
| `interface` | string | No | - | Network interface to apply rule |
| `direction` | string | No | "in" | Direction: in, out |
| `comment` | string | No | - | Rule comment |
| `route` | bool | No | false | Create routing rule |

### Examples

#### Enable UFW

```yaml
enable_firewall:
  module: ufw
  state: enabled
```

#### Allow SSH

```yaml
allow_ssh:
  module: ufw
  state: allow
  port: "22"
  proto: tcp
```

#### Allow from Specific Network

```yaml
allow_internal:
  module: ufw
  state: allow
  port: "443"
  from: "192.168.1.0/24"
  proto: tcp
```

#### Deny Port Range

```yaml
deny_ports:
  module: ufw
  state: deny
  port: "6000:6007"
  proto: tcp
```

#### Allow on Specific Interface

```yaml
allow_on_eth0:
  module: ufw
  state: allow
  port: "80"
  interface: eth0
  direction: in
```

#### Remove Rule

```yaml
remove_rule:
  module: ufw
  state: absent
  port: "8080"
  proto: tcp
```

### Idempotency

- Checks if rule exists before adding
- Checks UFW status before enabling/disabling
- Only makes changes when necessary

### Error Handling

Common errors:

- **UFW not installed**: Install with `apt-get install ufw`
- **Permission denied**: Requires root privileges
- **Invalid port**: Verify port number is valid
- **Invalid IP**: Verify IP address format

## K8s_Namespace Module

Manage Kubernetes namespaces declaratively.

### States

- `present` - Ensure namespace exists with specified labels/annotations
- `absent` - Ensure namespace does not exist

### Parameters

**labels** (map, optional)

- Labels to apply to namespace
- Merged with existing labels (doesn't remove unspecified labels)
- Example:

  ```yaml
  labels:
    environment: production
    team: platform
  ```

**annotations** (map, optional)

- Annotations to apply to namespace
- Merged with existing annotations
- Example:

  ```yaml
  annotations:
    description: "Production workloads"
  ```

### Requirements

- Kubernetes cluster access via kubeconfig or in-cluster config
- Appropriate RBAC permissions (create, get, update, delete namespaces)

### Examples

#### Create Namespace

```yaml
production_namespace:
  module: k8s_namespace
  state: present
  name: production
  labels:
    environment: production
    managed-by: keystone
  annotations:
    description: "Production namespace"
```

#### Create with Labels

```yaml
staging_namespace:
  module: k8s_namespace
  state: present
  name: staging
  labels:
    environment: staging
    team: platform
```

#### Delete Namespace

```yaml
old_namespace:
  module: k8s_namespace
  state: absent
  name: deprecated-ns
```

#### Update Namespace Labels

```yaml
update_labels:
  module: k8s_namespace
  state: present
  name: production
  labels:
    environment: production
    cost-center: "CC-1234"
```

### Return Values

```yaml
result: changed | unchanged
comment: "Namespace production created"
changes:
  created: true
  # or
  updated: true
  labels_updated:
    current: {...}
    desired: {...}
```

### Idempotency

The namespace module is fully idempotent:

- Creating an existing namespace with same labels/annotations: no change
- Creating an existing namespace with different labels: updates labels
- Deleting a non-existent namespace: no change

### Error Handling

Common errors:

- **Namespace not found** (for update): Returns absent state
- **Permission denied**: RBAC error, check cluster role bindings
- **Invalid name**: Namespace names must be valid DNS labels

## K8s_Deployment Module

Manage Kubernetes deployments declaratively with full CRUD support.

### States

- `present` - Ensure deployment exists with specified configuration
- `absent` - Ensure deployment does not exist

### Parameters

**namespace** (string, optional, default: "default")

- Target namespace for the deployment
- Example: `production`

**replicas** (int, optional, default: 1)

- Number of desired replicas
- Example: `3`

**image** (string, required for creation)

- Container image to deploy
- Example: `nginx:latest`, `myregistry.io/myapp:v1.2.3`

**container_port** (int, optional)

- Port the container listens on
- Example: `80`, `8080`

**selector** (map, optional)

- Pod selector labels for the deployment
- Must match pod template labels
- Example:

  ```yaml
  selector:
    app: nginx
  ```

**labels** (map, optional)

- Labels to apply to deployment
- Merged with existing labels on update
- Example:

  ```yaml
  labels:
    app: myapp
    version: v1
  ```

**annotations** (map, optional)

- Annotations to apply to deployment
- Merged with existing annotations on update
- Example:

  ```yaml
  annotations:
    kubernetes.io/change-cause: "Updated via Keystone"
  ```

### Requirements

- Kubernetes cluster access via kubeconfig or in-cluster config
- Appropriate RBAC permissions (create, get, update, delete deployments)

### Examples

#### Create Deployment

```yaml
nginx_deployment:
  module: k8s_deployment
  state: present
  name: nginx
  namespace: production
  replicas: 3
  image: nginx:1.25
  container_port: 80
  selector:
    app: nginx
  labels:
    app: nginx
    tier: frontend
  annotations:
    description: "Production nginx deployment"
```

#### Scale Deployment

```yaml
myapp_scale:
  module: k8s_deployment
  state: present
  name: myapp
  namespace: default
  replicas: 10
```

#### Update Deployment Labels

```yaml
myapp_update:
  module: k8s_deployment
  state: present
  name: myapp
  namespace: default
  labels:
    environment: production
    team: platform
```

#### Delete Deployment

```yaml
old_deployment:
  module: k8s_deployment
  state: absent
  name: deprecated-app
  namespace: default
```

### Return Values

```yaml
# On create
result: changed
comment: "Created deployment production/nginx"
changes:
  created: true
  replicas: 3
  image: nginx:1.25

# On update
result: changed
comment: "Updated deployment production/nginx"
changes:
  updated: true
  replicas_updated:
    current: 2
    desired: 5

# On delete
result: changed
comment: "Deleted deployment default/deprecated-app"
changes:
  deleted: true

# When already in desired state
result: unchanged
comment: "Deployment production/nginx is already in desired state 'present'"
metadata:
  replicas: 3
  available_replicas: 3
  ready_replicas: 3
```

### Idempotency

The k8s_deployment module is fully idempotent:

- Creating an existing deployment with same configuration: no change
- Creating an existing deployment with different replicas/labels: updates deployment
- Deleting a non-existent deployment: no change

### Error Handling

Common errors:

- **Image required**: When creating a new deployment, the `image` parameter is required
- **Deployment not found** (for update): Returns absent state, creates if state is `present`
- **Permission denied**: RBAC error, check cluster role bindings

## k8s_service Module

Manage Kubernetes services.

### States

- `present` - Ensure service exists with specified configuration
- `absent` - Ensure service does not exist

### Parameters

**name** (string, required)

- Service name (from state ID)

**namespace** (string, optional)

- Kubernetes namespace
- Default: `"default"`

**type** (string, optional)

- Service type
- Values: `ClusterIP`, `NodePort`, `LoadBalancer`, `ExternalName`
- Default: `"ClusterIP"`

**ports** (list, required for create)

- List of service ports
- Each port object contains:
  - **name** (string, optional): Port name (required if multiple ports)
  - **port** (int, required): Service port number
  - **target_port** (int, optional): Target port on pods (defaults to port)
  - **protocol** (string, optional): Protocol (TCP, UDP, SCTP). Default: `"TCP"`
  - **node_port** (int, optional): NodePort for NodePort/LoadBalancer services

**selector** (map, optional)

- Label selector for targeting pods
- Example: `{app: nginx, tier: frontend}`

**labels** (map, optional)

- Labels to apply to the service

**annotations** (map, optional)

- Annotations to apply to the service

**cluster_ip** (string, optional)

- Cluster IP address
- Use `"None"` for headless services

### Platform Compatibility

| Platform | Supported |
|----------|-----------|
| Kubernetes | ✅ |
| OpenShift | ✅ |
| VMs | ❌ |
| Bare Metal | ❌ |

### Examples

#### Create ClusterIP Service

```yaml
nginx_service:
  module: k8s_service
  state: present
  name: nginx
  namespace: production
  type: ClusterIP
  ports:
    - name: http
      port: 80
      target_port: 8080
      protocol: TCP
  selector:
    app: nginx
  labels:
    app: nginx
    tier: frontend
```

#### Create NodePort Service

```yaml
webapp_nodeport:
  module: k8s_service
  state: present
  name: webapp
  namespace: default
  type: NodePort
  ports:
    - port: 80
      target_port: 3000
      node_port: 30080
  selector:
    app: webapp
```

#### Create LoadBalancer Service

```yaml
api_loadbalancer:
  module: k8s_service
  state: present
  name: api
  namespace: production
  type: LoadBalancer
  ports:
    - name: https
      port: 443
      target_port: 8443
  selector:
    app: api
  annotations:
    service.beta.kubernetes.io/aws-load-balancer-type: nlb
```

#### Create Headless Service

```yaml
database_headless:
  module: k8s_service
  state: present
  name: database
  namespace: default
  type: ClusterIP
  cluster_ip: "None"
  ports:
    - port: 5432
      target_port: 5432
  selector:
    app: postgres
```

#### Multi-Port Service

```yaml
app_multiport:
  module: k8s_service
  state: present
  name: myapp
  namespace: default
  ports:
    - name: http
      port: 80
      target_port: 8080
    - name: https
      port: 443
      target_port: 8443
    - name: metrics
      port: 9090
      target_port: 9090
  selector:
    app: myapp
```

#### Delete Service

```yaml
old_service:
  module: k8s_service
  state: absent
  name: deprecated-service
  namespace: default
```

### Return Values

```yaml
# On create
result: changed
comment: "Created service production/nginx"
changes:
  created: true
  type: ClusterIP
  ports:
    - port: 80
      target_port: 8080

# On update
result: changed
comment: "Updated service production/nginx"
changes:
  updated: true
  type_updated:
    current: ClusterIP
    desired: NodePort

# On delete
result: changed
comment: "Deleted service default/deprecated-service"
changes:
  deleted: true

# When already in desired state
result: unchanged
comment: "Service production/nginx is already in desired state 'present'"
metadata:
  namespace: production
  type: ClusterIP
  clusterIP: 10.96.45.123
  ports:
    - port: 80
      target_port: 8080
```

### Idempotency

The k8s_service module is fully idempotent:

- Creating an existing service with same configuration: no change
- Creating an existing service with different type/ports/labels: updates service
- Deleting a non-existent service: no change

### Error Handling

Common errors:

- **Ports required**: At least one port is required when creating a service
- **Service not found** (for update): Returns absent state, creates if state is `present`
- **Permission denied**: RBAC error, check cluster role bindings
- **Invalid service type**: Service type must be ClusterIP, NodePort, LoadBalancer, or ExternalName

## k8s_configmap Module

Manage Kubernetes configmaps.

### States

- `present` - Ensure configmap exists with specified configuration
- `absent` - Ensure configmap does not exist

### Parameters

**name** (string, required)

- ConfigMap name (from state ID)

**namespace** (string, optional)

- Kubernetes namespace
- Default: `"default"`

**data** (map, optional)

- Key-value data pairs
- Values must be strings
- Example:

  ```yaml
  data:
    config.yaml: |
      database:
        host: localhost
    settings.json: '{"debug": true}'
  ```

**binary_data** (map, optional)

- Binary data (base64 encoded in YAML)
- Example:

  ```yaml
  binary_data:
    cert.pem: LS0tLS1CRUdJTi...
  ```

**labels** (map, optional)

- Labels to apply to the configmap

**annotations** (map, optional)

- Annotations to apply to the configmap

### Platform Compatibility

| Platform | Supported |
|----------|-----------|
| Kubernetes | ✅ |
| OpenShift | ✅ |
| VMs | ❌ |
| Bare Metal | ❌ |

### Examples

#### Create ConfigMap with Data

```yaml
app_config:
  module: k8s_configmap
  state: present
  name: app-config
  namespace: production
  data:
    database.host: "postgres.production.svc"
    database.port: "5432"
    log.level: "info"
  labels:
    app: myapp
    tier: backend
```

#### Create ConfigMap with File Contents

```yaml
nginx_config:
  module: k8s_configmap
  state: present
  name: nginx-config
  namespace: production
  data:
    nginx.conf: |
      worker_processes 4;
      events {
        worker_connections 1024;
      }
      http {
        server {
          listen 80;
          location / {
            root /usr/share/nginx/html;
          }
        }
      }
  labels:
    component: nginx
```

#### Create ConfigMap with Multiple Files

```yaml
scripts_config:
  module: k8s_configmap
  state: present
  name: scripts
  namespace: default
  data:
    startup.sh: |
      #!/bin/bash
      echo "Starting application..."
      /app/run.sh
    healthcheck.sh: |
      #!/bin/bash
      curl -sf http://localhost:8080/health
  annotations:
    description: "Application startup and health scripts"
```

#### Update ConfigMap Data

```yaml
update_config:
  module: k8s_configmap
  state: present
  name: app-config
  namespace: production
  data:
    log.level: "debug"
    feature.flag: "enabled"
```

#### Delete ConfigMap

```yaml
old_config:
  module: k8s_configmap
  state: absent
  name: deprecated-config
  namespace: default
```

### Return Values

```yaml
# On create
result: changed
comment: "Created configmap production/app-config"
changes:
  created: true
  data_keys:
    - database.host
    - database.port
    - log.level

# On update
result: changed
comment: "Updated configmap production/app-config"
changes:
  updated: true
  data_updated:
    current: {log.level: "info"}
    desired: {log.level: "debug"}

# On delete
result: changed
comment: "Deleted configmap default/deprecated-config"
changes:
  deleted: true

# When already in desired state
result: unchanged
comment: "ConfigMap production/app-config is already in desired state 'present'"
metadata:
  namespace: production
  data_keys:
    - database.host
    - database.port
    - log.level
```

### Idempotency

The k8s_configmap module is fully idempotent:

- Creating an existing configmap with same data: no change
- Creating an existing configmap with different data/labels: updates configmap
- Deleting a non-existent configmap: no change

### Error Handling

Common errors:

- **ConfigMap not found** (for update): Returns absent state, creates if state is `present`
- **Permission denied**: RBAC error, check cluster role bindings
- **Invalid data**: Data values must be valid UTF-8 strings (use binary_data for binary content)

## k8s_secret Module

Manage Kubernetes secrets for storing sensitive data.

### States

- `present` - Ensure secret exists with specified configuration
- `absent` - Ensure secret does not exist

### Parameters

**name** (string, required)

- Secret name (from state ID)

**namespace** (string, optional)

- Kubernetes namespace
- Default: `"default"`

**type** (string, optional)

- Secret type
- Values: `Opaque`, `kubernetes.io/tls`, `kubernetes.io/dockerconfigjson`, `kubernetes.io/basic-auth`, `kubernetes.io/ssh-auth`, `kubernetes.io/service-account-token`
- Default: `"Opaque"`

**data** (map, optional)

- Binary data (base64 encoded values or raw bytes)
- Example:

  ```yaml
  data:
    cert.pem: LS0tLS1CRUdJTi...
  ```

**string_data** (map, optional)

- String data (automatically converted to bytes)
- More convenient than `data` for string values
- Example:

  ```yaml
  string_data:
    username: admin
    password: secret123
  ```

**labels** (map, optional)

- Labels to apply to the secret

**annotations** (map, optional)

- Annotations to apply to the secret

### Platform Compatibility

| Platform | Supported |
|----------|-----------|
| Kubernetes | ✅ |
| OpenShift | ✅ |
| VMs | ❌ |
| Bare Metal | ❌ |

### Examples

#### Create Opaque Secret

```yaml
app_credentials:
  module: k8s_secret
  state: present
  name: app-credentials
  namespace: production
  type: Opaque
  string_data:
    username: admin
    password: supersecret123
    api_key: abc123xyz789
  labels:
    app: myapp
    tier: backend
```

#### Create TLS Secret

```yaml
tls_secret:
  module: k8s_secret
  state: present
  name: tls-certs
  namespace: production
  type: kubernetes.io/tls
  string_data:
    tls.crt: |
      -----BEGIN CERTIFICATE-----
      MIICxjCCAa6gAwIBAgIJAJ...
      -----END CERTIFICATE-----
    tls.key: |
      REDACTED_PRIVATE_KEY_MATERIAL
```

#### Create Docker Registry Secret

```yaml
docker_registry:
  module: k8s_secret
  state: present
  name: registry-creds
  namespace: default
  type: kubernetes.io/dockerconfigjson
  string_data:
    .dockerconfigjson: |
      {
        "auths": {
          "registry.example.com": {
            "username": "user",
            "password": "pass",
            "auth": "dXNlcjpwYXNz"
          }
        }
      }
```

#### Create Basic Auth Secret

```yaml
basic_auth:
  module: k8s_secret
  state: present
  name: basic-auth
  namespace: default
  type: kubernetes.io/basic-auth
  string_data:
    username: admin
    password: changeme
```

#### Create SSH Key Secret

```yaml
ssh_key:
  module: k8s_secret
  state: present
  name: ssh-key
  namespace: default
  type: kubernetes.io/ssh-auth
  string_data:
    ssh-privatekey: |
      REDACTED_SSH_PRIVATE_KEY
```

#### Update Secret Data

```yaml
update_credentials:
  module: k8s_secret
  state: present
  name: app-credentials
  namespace: production
  string_data:
    password: newsecretpassword
    api_key: newkey456
```

#### Delete Secret

```yaml
old_secret:
  module: k8s_secret
  state: absent
  name: deprecated-secret
  namespace: default
```

### Return Values

```yaml
# On create
result: changed
comment: "Created secret production/app-credentials"
changes:
  created: true
  type: Opaque
  data_keys:
    - username
    - password
    - api_key

# On update
result: changed
comment: "Updated secret production/app-credentials"
changes:
  updated: true
  data_updated:
    current_keys:
      - username
      - password
    desired_keys:
      - username
      - password
      - api_key

# On delete
result: changed
comment: "Deleted secret default/deprecated-secret"
changes:
  deleted: true

# When already in desired state
result: unchanged
comment: "Secret production/app-credentials is already in desired state 'present'"
metadata:
  namespace: production
  type: Opaque
  data_keys:
    - username
    - password
    - api_key
```

### Idempotency

The k8s_secret module is fully idempotent:

- Creating an existing secret with same data: no change
- Creating an existing secret with different data/type/labels: updates secret
- Deleting a non-existent secret: no change

### Security Considerations

1. **Avoid committing secrets to version control**: Use templating with external secret sources
2. **Use `string_data` for clarity**: Easier to read than base64-encoded `data`
3. **Set appropriate RBAC**: Limit who can read secrets
4. **Consider external secret management**: Integrate with Vault, AWS Secrets Manager, etc.

### Error Handling

Common errors:

- **Secret not found** (for update): Returns absent state, creates if state is `present`
- **Permission denied**: RBAC error, check cluster role bindings
- **Invalid secret type**: Secret type must be a valid Kubernetes secret type

## k8s_ingress Module

Manage Kubernetes ingress resources for HTTP/HTTPS routing.

### States

- `present` - Ensure ingress exists with specified configuration
- `absent` - Ensure ingress does not exist

### Parameters

**name** (string, required)

- Ingress name (from state ID)

**namespace** (string, optional)

- Kubernetes namespace
- Default: `"default"`

**ingress_class** (string, optional)

- IngressClass to use (e.g., `nginx`, `traefik`, `haproxy`)
- Maps to `spec.ingressClassName`

**rules** (list, optional)

- List of host-based routing rules
- Each rule contains:
  - `host` (string): Fully qualified domain name
  - `paths` (list): List of path configurations
    - `path` (string): URL path to match
    - `path_type` (string): `Exact`, `Prefix`, or `ImplementationSpecific`
    - `backend` (map):
      - `service` (string): Backend service name
      - `port` (int): Backend service port

**tls** (list, optional)

- TLS configuration for HTTPS
- Each entry contains:
  - `hosts` (list): Hostnames covered by the TLS certificate
  - `secret_name` (string): Name of secret containing TLS certificate

**default_backend** (map, optional)

- Default backend for requests not matching any rule
- Contains:
  - `service` (string): Service name
  - `port` (int): Service port

**labels** (map, optional)

- Labels to apply to the ingress

**annotations** (map, optional)

- Annotations to apply to the ingress
- Common annotations for ingress controllers:
  - `nginx.ingress.kubernetes.io/rewrite-target`
  - `nginx.ingress.kubernetes.io/ssl-redirect`
  - `traefik.ingress.kubernetes.io/router.entrypoints`

### Platform Compatibility

| Platform | Supported |
|----------|-----------|
| Kubernetes | ✅ |
| OpenShift | ✅ |
| VMs | ❌ |
| Bare Metal | ❌ |

### Examples

#### Create Simple Ingress

```yaml
web_ingress:
  module: k8s_ingress
  state: present
  name: web-ingress
  namespace: production
  ingress_class: nginx
  rules:
    - host: app.example.com
      paths:
        - path: /
          path_type: Prefix
          backend:
            service: web-service
            port: 80
  labels:
    app: web
    environment: production
```

#### Create Ingress with TLS

```yaml
secure_ingress:
  module: k8s_ingress
  state: present
  name: secure-ingress
  namespace: production
  ingress_class: nginx
  rules:
    - host: secure.example.com
      paths:
        - path: /
          path_type: Prefix
          backend:
            service: secure-app
            port: 8080
  tls:
    - hosts:
        - secure.example.com
      secret_name: secure-tls-secret
  annotations:
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
```

#### Create Ingress with Multiple Paths

```yaml
api_ingress:
  module: k8s_ingress
  state: present
  name: api-ingress
  namespace: production
  ingress_class: nginx
  rules:
    - host: api.example.com
      paths:
        - path: /v1
          path_type: Prefix
          backend:
            service: api-v1
            port: 8080
        - path: /v2
          path_type: Prefix
          backend:
            service: api-v2
            port: 8080
        - path: /health
          path_type: Exact
          backend:
            service: health-checker
            port: 8081
```

#### Create Ingress with Multiple Hosts

```yaml
multi_host_ingress:
  module: k8s_ingress
  state: present
  name: multi-host-ingress
  namespace: production
  ingress_class: nginx
  rules:
    - host: web.example.com
      paths:
        - path: /
          path_type: Prefix
          backend:
            service: web-service
            port: 80
    - host: api.example.com
      paths:
        - path: /
          path_type: Prefix
          backend:
            service: api-service
            port: 8080
  tls:
    - hosts:
        - web.example.com
        - api.example.com
      secret_name: wildcard-tls
```

#### Create Ingress with Default Backend

```yaml
fallback_ingress:
  module: k8s_ingress
  state: present
  name: fallback-ingress
  namespace: production
  ingress_class: nginx
  default_backend:
    service: default-backend
    port: 80
  rules:
    - host: app.example.com
      paths:
        - path: /api
          path_type: Prefix
          backend:
            service: api-service
            port: 8080
```

#### Delete Ingress

```yaml
old_ingress:
  module: k8s_ingress
  state: absent
  name: deprecated-ingress
  namespace: default
```

### Return Values

```yaml
# On create
result: changed
comment: "Created ingress production/web-ingress"
changes:
  created: true
  ingress_class: nginx
  rules_count: 1
  tls_count: 0

# On update
result: changed
comment: "Updated ingress production/web-ingress"
changes:
  updated: true
  rules_updated:
    current_count: 1
    desired_count: 2

# On delete
result: changed
comment: "Deleted ingress default/deprecated-ingress"
changes:
  deleted: true

# When already in desired state
result: unchanged
comment: "Ingress production/web-ingress is already in desired state 'present'"
metadata:
  namespace: production
  ingress_class: nginx
  rules_count: 1
  tls_count: 1
  load_balancer_ingress:
    - 10.0.0.50
```

### Idempotency

The k8s_ingress module is fully idempotent:

- Creating an existing ingress with same configuration: no change
- Creating an existing ingress with different rules/TLS/annotations: updates ingress
- Deleting a non-existent ingress: no change

### Common Annotations

| Annotation | Purpose |
|------------|---------|
| `nginx.ingress.kubernetes.io/rewrite-target` | URL rewriting |
| `nginx.ingress.kubernetes.io/ssl-redirect` | Force HTTPS |
| `nginx.ingress.kubernetes.io/proxy-body-size` | Max request body size |
| `nginx.ingress.kubernetes.io/backend-protocol` | Backend protocol (HTTP/HTTPS) |
| `traefik.ingress.kubernetes.io/router.entrypoints` | Traefik entry points |
| `traefik.ingress.kubernetes.io/router.tls` | Enable TLS |

### Error Handling

Common errors:

- **Ingress not found** (for update): Returns absent state, creates if state is `present`
- **Permission denied**: RBAC error, check cluster role bindings
- **Invalid path type**: Path type must be `Exact`, `Prefix`, or `ImplementationSpecific`
- **TLS secret not found**: Referenced TLS secret must exist in the same namespace

## k8s_statefulset Module

Manage Kubernetes StatefulSet resources for stateful workloads with stable network identities and persistent storage.

### States

| State | Description |
|-------|-------------|
| `present` | StatefulSet exists with specified configuration |
| `absent` | StatefulSet does not exist |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `namespace` | string | No | `default` | Kubernetes namespace |
| `image` | string | Yes (create) | - | Container image (required for creation) |
| `service_name` | string | Yes (create) | - | Headless service name (required for creation) |
| `replicas` | integer | No | `1` | Desired number of pod replicas |
| `container_port` | integer | No | - | Container port to expose |
| `labels` | map | No | - | Labels to apply to the statefulset |
| `annotations` | map | No | - | Annotations to apply to the statefulset |
| `selector` | map | No | - | Pod selector labels |
| `pod_management_policy` | string | No | `OrderedReady` | Pod creation order: `OrderedReady` or `Parallel` |
| `update_strategy` | string | No | `RollingUpdate` | Update strategy: `RollingUpdate` or `OnDelete` |
| `volume_claim_templates` | list | No | - | Persistent volume claim templates |

### Volume Claim Template Parameters

Each volume claim template supports:

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Volume claim name |
| `storage_class` | string | No | Storage class name |
| `storage_size` | string | No | Storage size (e.g., "10Gi") |
| `access_modes` | list | No | Access modes: `ReadWriteOnce`, `ReadOnlyMany`, `ReadWriteMany` |
| `access_mode` | string | No | Single access mode (alternative to `access_modes`) |

### Examples

#### Create a basic StatefulSet

```yaml
redis_statefulset:
  module: k8s_statefulset
  state: present
  name: redis
  namespace: cache
  image: redis:7
  service_name: redis-headless
  replicas: 3
```

#### StatefulSet with persistent storage

```yaml
postgres_statefulset:
  module: k8s_statefulset
  state: present
  name: postgres
  namespace: database
  image: postgres:15
  service_name: postgres-headless
  replicas: 3
  container_port: 5432
  pod_management_policy: OrderedReady
  update_strategy: RollingUpdate
  labels:
    app: postgres
    tier: database
  volume_claim_templates:
    - name: data
      storage_class: fast-ssd
      storage_size: 100Gi
      access_modes:
        - ReadWriteOnce
```

#### StatefulSet with multiple volumes

```yaml
elasticsearch_statefulset:
  module: k8s_statefulset
  state: present
  name: elasticsearch
  namespace: logging
  image: elasticsearch:8.11.0
  service_name: elasticsearch-headless
  replicas: 3
  container_port: 9200
  pod_management_policy: Parallel
  labels:
    app: elasticsearch
    component: data
  annotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "9114"
  volume_claim_templates:
    - name: data
      storage_class: fast-ssd
      storage_size: 500Gi
      access_modes:
        - ReadWriteOnce
    - name: logs
      storage_class: standard
      storage_size: 50Gi
      access_mode: ReadWriteOnce
```

#### StatefulSet with OnDelete update strategy

```yaml
zookeeper_statefulset:
  module: k8s_statefulset
  state: present
  name: zookeeper
  namespace: kafka
  image: zookeeper:3.9
  service_name: zookeeper-headless
  replicas: 3
  update_strategy: OnDelete
  volume_claim_templates:
    - name: data
      storage_size: 20Gi
      access_modes:
        - ReadWriteOnce
```

#### Scale a StatefulSet

```yaml
scale_redis:
  module: k8s_statefulset
  state: present
  name: redis
  namespace: cache
  replicas: 5
```

#### Delete a StatefulSet

```yaml
remove_statefulset:
  module: k8s_statefulset
  state: absent
  name: old-database
  namespace: legacy
```

### Metadata

The Check operation returns the following metadata:

| Key | Description |
|-----|-------------|
| `namespace` | StatefulSet namespace |
| `replicas` | Desired replica count |
| `ready_replicas` | Number of ready replicas |
| `current_replicas` | Number of current replicas |
| `updated_replicas` | Number of updated replicas |
| `current_revision` | Current revision hash |
| `update_revision` | Update revision hash |
| `service_name` | Headless service name |
| `pod_management_policy` | Pod management policy |
| `update_strategy` | Update strategy type |
| `status` | Overall status |

### Pod Management Policies

| Policy | Description |
|--------|-------------|
| `OrderedReady` | Pods are created/deleted in order (0, 1, 2...). Each pod waits for predecessors to be ready. |
| `Parallel` | Pods are created/deleted in parallel. Faster but no ordering guarantees. |

### Update Strategies

| Strategy | Description |
|----------|-------------|
| `RollingUpdate` | Pods are updated in reverse order (n, n-1, ..., 0) automatically |
| `OnDelete` | Pods are only updated when manually deleted |

### Idempotency

The k8s_statefulset module is fully idempotent:

- Creating an existing statefulset with same configuration: no change
- Creating an existing statefulset with different replicas/labels/annotations: updates statefulset
- Deleting a non-existent statefulset: no change

### Important Notes

1. **Headless Service Required**: StatefulSets require a headless service (ClusterIP: None) for network identity
2. **Volume Claim Templates**: VolumeClaimTemplates are immutable after creation - delete and recreate to change
3. **Pod Identity**: Each pod gets a stable hostname: `<statefulset-name>-<ordinal>.<service-name>`
4. **Scaling**: When scaling down, pods are terminated in reverse order (highest ordinal first)
5. **Persistent Data**: Deleting a StatefulSet does NOT delete associated PVCs - delete manually if needed

### Error Handling

Common errors:

- **StatefulSet not found** (for update): Returns absent state, creates if state is `present`
- **Permission denied**: RBAC error, check cluster role bindings
- **Image required**: Image must be specified when creating a new statefulset
- **Service name required**: Headless service name must be specified for creation
- **Invalid pod management policy**: Must be `OrderedReady` or `Parallel`
- **Invalid update strategy**: Must be `RollingUpdate` or `OnDelete`

## k8s_daemonset Module

Manage Kubernetes DaemonSet resources for running pods on every node (or selected nodes) in a cluster.

### States

| State | Description |
|-------|-------------|
| `present` | DaemonSet exists with specified configuration |
| `absent` | DaemonSet does not exist |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `namespace` | string | No | `default` | Kubernetes namespace |
| `image` | string | Yes (create) | - | Container image (required for creation) |
| `container_port` | integer | No | - | Container port to expose |
| `labels` | map | No | - | Labels to apply to the daemonset |
| `annotations` | map | No | - | Annotations to apply to the daemonset |
| `selector` | map | No | - | Pod selector labels |
| `update_strategy` | string | No | `RollingUpdate` | Update strategy: `RollingUpdate` or `OnDelete` |
| `node_selector` | map | No | - | Node selector for pod scheduling |

### Examples

#### Create a basic DaemonSet

```yaml
fluentd_daemonset:
  module: k8s_daemonset
  state: present
  name: fluentd
  namespace: logging
  image: fluent/fluentd:v1.16
  container_port: 24224
  labels:
    app: fluentd
    tier: logging
```

#### DaemonSet with node selector

```yaml
monitoring_agent:
  module: k8s_daemonset
  state: present
  name: node-exporter
  namespace: monitoring
  image: prom/node-exporter:v1.7.0
  container_port: 9100
  node_selector:
    kubernetes.io/os: linux
  labels:
    app: node-exporter
  annotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "9100"
```

#### DaemonSet with OnDelete strategy

```yaml
cni_plugin:
  module: k8s_daemonset
  state: present
  name: calico-node
  namespace: kube-system
  image: calico/node:v3.26.0
  update_strategy: OnDelete
  labels:
    k8s-app: calico-node
```

#### Delete a DaemonSet

```yaml
remove_daemonset:
  module: k8s_daemonset
  state: absent
  name: old-agent
  namespace: monitoring
```

### Metadata

The Check operation returns the following metadata:

| Key | Description |
|-----|-------------|
| `namespace` | DaemonSet namespace |
| `desired_number_scheduled` | Number of nodes that should run the pod |
| `current_number_scheduled` | Number of nodes running the pod |
| `number_ready` | Number of pods that are ready |
| `number_available` | Number of pods available |
| `number_misscheduled` | Number of pods running on wrong nodes |
| `updated_number_scheduled` | Number of pods with updated template |
| `update_strategy` | Update strategy type |
| `status` | Overall status |

### Idempotency

The k8s_daemonset module is fully idempotent:

- Creating an existing daemonset with same configuration: no change
- Creating an existing daemonset with different strategy/labels: updates daemonset
- Deleting a non-existent daemonset: no change

### Error Handling

Common errors:

- **DaemonSet not found** (for update): Returns absent state, creates if state is `present`
- **Permission denied**: RBAC error, check cluster role bindings
- **Image required**: Image must be specified when creating a new daemonset
- **Invalid update strategy**: Must be `RollingUpdate` or `OnDelete`

## k8s_job Module

Manage Kubernetes Job resources for running batch workloads to completion.

### States

| State | Description |
|-------|-------------|
| `present` | Job exists |
| `absent` | Job does not exist |
| `completed` | Job exists and has completed successfully |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `namespace` | string | No | `default` | Kubernetes namespace |
| `image` | string | Yes (create) | - | Container image (required for creation) |
| `command` | list | No | - | Command to run in the container |
| `args` | list | No | - | Arguments to the command |
| `completions` | integer | No | `1` | Number of successful completions required |
| `parallelism` | integer | No | `1` | Number of pods to run in parallel |
| `backoff_limit` | integer | No | `6` | Number of retries before marking as failed |
| `restart_policy` | string | No | `Never` | Pod restart policy: `Never` or `OnFailure` |
| `labels` | map | No | - | Labels to apply to the job |
| `annotations` | map | No | - | Annotations to apply to the job |

### Examples

#### Create a simple Job

```yaml
database_backup:
  module: k8s_job
  state: present
  name: db-backup
  namespace: database
  image: postgres:15
  command:
    - /bin/sh
    - -c
  args:
    - pg_dump -h postgres -U admin mydb > /backup/db.sql
  labels:
    app: backup
    type: database
```

#### Create a Job with parallelism

```yaml
data_processing:
  module: k8s_job
  state: present
  name: process-data
  namespace: analytics
  image: myorg/processor:v1.0
  completions: 10
  parallelism: 5
  backoff_limit: 3
  labels:
    app: processor
```

#### Create a Job with OnFailure restart

```yaml
retry_job:
  module: k8s_job
  state: present
  name: flaky-task
  namespace: default
  image: myorg/task:v1.0
  restart_policy: OnFailure
  backoff_limit: 5
```

#### Ensure Job completed

```yaml
wait_for_migration:
  module: k8s_job
  state: completed
  name: db-migration
  namespace: database
  image: myorg/migrator:v1.0
  command:
    - ./migrate
  args:
    - up
```

#### Delete a Job

```yaml
cleanup_job:
  module: k8s_job
  state: absent
  name: completed-job
  namespace: default
```

### Metadata

The Check operation returns the following metadata:

| Key | Description |
|-----|-------------|
| `namespace` | Job namespace |
| `active` | Number of actively running pods |
| `succeeded` | Number of pods that succeeded |
| `failed` | Number of pods that failed |
| `completions` | Desired number of completions |
| `parallelism` | Maximum parallel pods |
| `backoff_limit` | Retry limit |
| `start_time` | Job start timestamp |
| `completion_time` | Job completion timestamp (if completed) |
| `status` | Overall status |

### Idempotency

The k8s_job module is fully idempotent:

- Creating an existing job: no change (jobs are immutable)
- Deleting a non-existent job: no change
- Checking for `completed` state: returns current completion status

### Important Notes

1. **Jobs are immutable**: Once created, job specs cannot be updated. Delete and recreate to change.
2. **Completed jobs persist**: Jobs remain after completion for log inspection. Clean up manually or use TTL.
3. **Pod cleanup**: Deleting a job also deletes its pods.

### Error Handling

Common errors:

- **Job not found** (for update): Returns absent state, creates if state is `present`
- **Permission denied**: RBAC error, check cluster role bindings
- **Image required**: Image must be specified when creating a new job

## k8s_cronjob Module

Manage Kubernetes CronJob resources for scheduled batch workloads.

### States

| State | Description |
|-------|-------------|
| `present` | CronJob exists and is active |
| `absent` | CronJob does not exist |
| `suspended` | CronJob exists but is suspended |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `namespace` | string | No | `default` | Kubernetes namespace |
| `schedule` | string | Yes (create) | - | Cron schedule expression (required for creation) |
| `image` | string | Yes (create) | - | Container image (required for creation) |
| `command` | list | No | - | Command to run in the container |
| `args` | list | No | - | Arguments to the command |
| `concurrency_policy` | string | No | `Allow` | Policy: `Allow`, `Forbid`, or `Replace` |
| `restart_policy` | string | No | `Never` | Pod restart policy: `Never` or `OnFailure` |
| `labels` | map | No | - | Labels to apply to the cronjob |
| `annotations` | map | No | - | Annotations to apply to the cronjob |

### Concurrency Policies

| Policy | Description |
|--------|-------------|
| `Allow` | Allow concurrent job runs (default) |
| `Forbid` | Skip new run if previous is still running |
| `Replace` | Cancel running job and start new one |

### Examples

#### Create a basic CronJob

```yaml
nightly_backup:
  module: k8s_cronjob
  state: present
  name: nightly-backup
  namespace: database
  schedule: "0 2 * * *"
  image: postgres:15
  command:
    - /scripts/backup.sh
  labels:
    app: backup
    schedule: nightly
```

#### CronJob with Forbid policy

```yaml
hourly_sync:
  module: k8s_cronjob
  state: present
  name: data-sync
  namespace: analytics
  schedule: "0 * * * *"
  image: myorg/sync:v1.0
  concurrency_policy: Forbid
  labels:
    app: sync
```

#### CronJob with Replace policy

```yaml
metrics_collector:
  module: k8s_cronjob
  state: present
  name: collect-metrics
  namespace: monitoring
  schedule: "*/5 * * * *"
  image: myorg/collector:v1.0
  concurrency_policy: Replace
  annotations:
    description: "Collect metrics every 5 minutes"
```

#### Create suspended CronJob

```yaml
maintenance_job:
  module: k8s_cronjob
  state: suspended
  name: maintenance
  namespace: default
  schedule: "0 4 * * 0"
  image: myorg/maintenance:v1.0
```

#### Suspend existing CronJob

```yaml
suspend_job:
  module: k8s_cronjob
  state: suspended
  name: nightly-backup
  namespace: database
```

#### Delete a CronJob

```yaml
remove_cronjob:
  module: k8s_cronjob
  state: absent
  name: old-schedule
  namespace: default
```

### Metadata

The Check operation returns the following metadata:

| Key | Description |
|-----|-------------|
| `namespace` | CronJob namespace |
| `schedule` | Cron schedule expression |
| `suspend` | Whether the cronjob is suspended |
| `concurrency_policy` | Concurrency policy |
| `active_jobs` | Number of currently active jobs |
| `last_schedule_time` | Last time a job was scheduled |
| `last_successful_time` | Last time a job completed successfully |
| `status` | Overall status |

### Cron Schedule Format

```
┌───────────── minute (0 - 59)
│ ┌───────────── hour (0 - 23)
│ │ ┌───────────── day of month (1 - 31)
│ │ │ ┌───────────── month (1 - 12)
│ │ │ │ ┌───────────── day of week (0 - 6) (Sunday = 0)
│ │ │ │ │
* * * * *
```

Common examples:

- `0 * * * *` - Every hour
- `0 0 * * *` - Daily at midnight
- `0 2 * * *` - Daily at 2 AM
- `*/15 * * * *` - Every 15 minutes
- `0 0 * * 0` - Weekly on Sunday at midnight

### Idempotency

The k8s_cronjob module is fully idempotent:

- Creating an existing cronjob with same configuration: no change
- Creating an existing cronjob with different schedule/policy: updates cronjob
- Suspending an already suspended cronjob: no change
- Deleting a non-existent cronjob: no change

### Error Handling

Common errors:

- **CronJob not found** (for update): Returns absent state, creates if state is `present`
- **Permission denied**: RBAC error, check cluster role bindings
- **Schedule required**: Cron schedule must be specified for creation
- **Image required**: Image must be specified when creating a new cronjob
- **Invalid schedule**: Must be a valid cron expression

## k8s_pvc Module

Manage Kubernetes PersistentVolumeClaim resources for storage provisioning.

### States

| State | Description |
|-------|-------------|
| `present` | PVC exists with specified configuration |
| `absent` | PVC does not exist |
| `bound` | PVC exists and is bound to a PersistentVolume |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `namespace` | string | No | `default` | Kubernetes namespace |
| `storage_size` | string | Yes (create) | - | Requested storage size (e.g., "10Gi") |
| `storage_class_name` | string | No | - | Storage class name |
| `access_modes` | list | No | `["ReadWriteOnce"]` | Access modes list |
| `access_mode` | string | No | - | Single access mode (alternative to `access_modes`) |
| `volume_mode` | string | No | `Filesystem` | Volume mode: `Filesystem` or `Block` |
| `volume_name` | string | No | - | Specific PV to bind to |
| `labels` | map | No | - | Labels to apply to the PVC |
| `annotations` | map | No | - | Annotations to apply to the PVC |

### Access Modes

| Mode | Description |
|------|-------------|
| `ReadWriteOnce` (RWO) | Can be mounted read-write by a single node |
| `ReadOnlyMany` (ROX) | Can be mounted read-only by many nodes |
| `ReadWriteMany` (RWX) | Can be mounted read-write by many nodes |
| `ReadWriteOncePod` (RWOP) | Can be mounted read-write by a single pod |

### Examples

#### Create a basic PVC

```yaml
app_storage:
  module: k8s_pvc
  state: present
  name: app-data
  namespace: production
  storage_size: 10Gi
  labels:
    app: myapp
```

#### PVC with storage class

```yaml
database_storage:
  module: k8s_pvc
  state: present
  name: postgres-data
  namespace: database
  storage_size: 100Gi
  storage_class_name: fast-ssd
  access_modes:
    - ReadWriteOnce
  labels:
    app: postgres
    tier: database
```

#### PVC with ReadWriteMany

```yaml
shared_storage:
  module: k8s_pvc
  state: present
  name: shared-files
  namespace: production
  storage_size: 50Gi
  storage_class_name: nfs
  access_modes:
    - ReadWriteMany
  labels:
    type: shared
```

#### Block volume PVC

```yaml
block_storage:
  module: k8s_pvc
  state: present
  name: raw-disk
  namespace: storage
  storage_size: 500Gi
  storage_class_name: block-storage
  volume_mode: Block
  access_mode: ReadWriteOnce
```

#### Ensure PVC is bound

```yaml
wait_for_storage:
  module: k8s_pvc
  state: bound
  name: app-data
  namespace: production
```

#### Expand PVC storage

```yaml
expand_storage:
  module: k8s_pvc
  state: present
  name: app-data
  namespace: production
  storage_size: 50Gi
```

#### Delete a PVC

```yaml
remove_pvc:
  module: k8s_pvc
  state: absent
  name: old-storage
  namespace: default
```

### Metadata

The Check operation returns the following metadata:

| Key | Description |
|-----|-------------|
| `namespace` | PVC namespace |
| `phase` | Current phase: `Pending`, `Bound`, `Lost` |
| `storage_class_name` | Storage class name |
| `volume_name` | Bound PersistentVolume name |
| `access_modes` | Access modes |
| `requested_storage` | Requested storage size |
| `allocated_storage` | Actual allocated storage size |
| `status` | Overall status |

### Idempotency

The k8s_pvc module is fully idempotent:

- Creating an existing PVC with same configuration: no change
- Expanding storage on existing PVC: updates PVC (if storage class supports expansion)
- Deleting a non-existent PVC: no change

### Important Notes

1. **Storage expansion**: Only works if the storage class has `allowVolumeExpansion: true`
2. **Cannot shrink**: PVC storage size can only be increased, never decreased
3. **Bound PVCs**: Once bound, most fields are immutable except storage size
4. **Data persistence**: Deleting a PVC may delete the underlying data depending on reclaim policy

### Error Handling

Common errors:

- **PVC not found** (for update): Returns absent state, creates if state is `present`
- **Permission denied**: RBAC error, check cluster role bindings
- **Storage size required**: Storage size must be specified for creation
- **Invalid access mode**: Must be a valid Kubernetes access mode
- **Storage class not found**: Specified storage class must exist

## k8s_hpa Module

Manage Kubernetes HorizontalPodAutoscaler resources for automatic pod scaling based on metrics.

### States

| State | Description |
|-------|-------------|
| `present` | HPA exists with specified configuration |
| `absent` | HPA does not exist |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `namespace` | string | No | `default` | Kubernetes namespace |
| `target_kind` | string | No | `Deployment` | Target resource kind |
| `target_name` | string | Yes (create) | - | Target resource name (required for creation) |
| `min_replicas` | integer | No | `1` | Minimum number of replicas |
| `max_replicas` | integer | No | `10` | Maximum number of replicas |
| `target_cpu_utilization` | integer | No | - | Target CPU utilization percentage |
| `target_memory_utilization` | integer | No | - | Target memory utilization percentage |
| `labels` | map | No | - | Labels to apply to the HPA |
| `annotations` | map | No | - | Annotations to apply to the HPA |

### Examples

#### Create basic HPA with CPU scaling

```yaml
web_autoscaler:
  module: k8s_hpa
  state: present
  name: web-hpa
  namespace: production
  target_kind: Deployment
  target_name: web-app
  min_replicas: 2
  max_replicas: 10
  target_cpu_utilization: 80
  labels:
    app: web
```

#### HPA with memory scaling

```yaml
api_autoscaler:
  module: k8s_hpa
  state: present
  name: api-hpa
  namespace: production
  target_name: api-server
  min_replicas: 3
  max_replicas: 20
  target_memory_utilization: 70
  labels:
    app: api
```

#### HPA with both CPU and memory

```yaml
worker_autoscaler:
  module: k8s_hpa
  state: present
  name: worker-hpa
  namespace: processing
  target_name: worker
  min_replicas: 1
  max_replicas: 50
  target_cpu_utilization: 75
  target_memory_utilization: 80
  annotations:
    description: "Auto-scale workers based on resource usage"
```

#### HPA for StatefulSet

```yaml
database_autoscaler:
  module: k8s_hpa
  state: present
  name: db-hpa
  namespace: database
  target_kind: StatefulSet
  target_name: postgres
  min_replicas: 3
  max_replicas: 5
  target_cpu_utilization: 70
```

#### Update HPA scaling limits

```yaml
update_limits:
  module: k8s_hpa
  state: present
  name: web-hpa
  namespace: production
  min_replicas: 5
  max_replicas: 100
  target_cpu_utilization: 70
```

#### Delete an HPA

```yaml
remove_hpa:
  module: k8s_hpa
  state: absent
  name: old-autoscaler
  namespace: default
```

### Metadata

The Check operation returns the following metadata:

| Key | Description |
|-----|-------------|
| `namespace` | HPA namespace |
| `min_replicas` | Minimum replica count |
| `max_replicas` | Maximum replica count |
| `current_replicas` | Current number of replicas |
| `desired_replicas` | Desired number of replicas |
| `target_kind` | Target resource kind |
| `target_name` | Target resource name |
| `current_cpu_utilization` | Current CPU utilization percentage |
| `target_cpu_utilization` | Target CPU utilization percentage |
| `status` | Overall status |

### Idempotency

The k8s_hpa module is fully idempotent:

- Creating an existing HPA with same configuration: no change
- Creating an existing HPA with different min/max/targets: updates HPA
- Deleting a non-existent HPA: no change

### Important Notes

1. **Metrics Server required**: HPA requires the Kubernetes Metrics Server to be installed
2. **Resource requests**: Target pods must have resource requests defined for scaling to work
3. **Scaling delay**: HPA has cooldown periods to prevent thrashing (default: 5 min scale-down)
4. **Custom metrics**: For custom metrics, use the Kubernetes metrics APIs directly

### Scaling Behavior

The HPA controller:

1. Periodically (default: 15s) fetches metrics for target pods
2. Calculates desired replica count based on current vs target utilization
3. Scales up/down within min/max bounds
4. Applies cooldown periods to prevent rapid changes

### Error Handling

Common errors:

- **HPA not found** (for update): Returns absent state, creates if state is `present`
- **Permission denied**: RBAC error, check cluster role bindings
- **Target name required**: Target resource name must be specified for creation
- **Target not found**: Target Deployment/StatefulSet must exist
- **Metrics unavailable**: Metrics Server may not be running or accessible

## Cron Module

Manage Linux cron jobs.

### Platform

Linux only. Uses the system crontab or per-user crontabs.

### States

- `present` - Ensure cron job exists
- `absent` - Ensure cron job does not exist

### Parameters

**command** (string, required)

- Command to execute
- Example: `/usr/local/bin/backup.sh`

**minute** (string, optional)

- Minute field (0-59, `*`, or special)
- Default: `*`
- Example: `0`, `*/15`, `30`

**hour** (string, optional)

- Hour field (0-23, `*`, or special)
- Default: `*`
- Example: `2`, `*/6`, `9-17`

**day** (string, optional)

- Day of month field (1-31, `*`, or special)
- Default: `*`
- Example: `1`, `15`, `*/2`

**month** (string, optional)

- Month field (1-12, `*`, or special)
- Default: `*`
- Example: `1`, `6,12`, `*/3`

**weekday** (string, optional)

- Day of week field (0-7, `*`, or special; 0 and 7 are Sunday)
- Default: `*`
- Example: `1-5`, `0`, `SAT`

**special** (string, optional)

- Special schedule string instead of 5-field specification
- Options: `@reboot`, `@yearly`, `@annually`, `@monthly`, `@weekly`, `@daily`, `@midnight`, `@hourly`
- Mutually exclusive with minute/hour/day/month/weekday

**user** (string, optional)

- User to run the cron job as
- Default: root (for system crontab)
- Example: `www-data`, `nobody`

**disabled** (bool, optional)

- Comment out the cron entry (keep but don't run)
- Default: false

### Examples

**Daily backup at 2 AM**:

```yaml
cron:
  daily_backup:
    state: present
    command: /usr/local/bin/backup.sh
    minute: "0"
    hour: "2"
```

**Every 15 minutes**:

```yaml
cron:
  check_queue:
    state: present
    command: /usr/local/bin/process_queue.sh
    minute: "*/15"
```

**Weekly cleanup on Sundays**:

```yaml
cron:
  weekly_cleanup:
    state: present
    command: /usr/local/bin/cleanup.sh
    special: "@weekly"
```

**Run on boot**:

```yaml
cron:
  startup_script:
    state: present
    command: /usr/local/bin/on_boot.sh
    special: "@reboot"
```

**As specific user**:

```yaml
cron:
  user_job:
    state: present
    command: php /var/www/artisan schedule:run
    minute: "*"
    user: www-data
```

## Systemd_Timer Module

Manage Linux systemd timer units for scheduled tasks.

### Platform

Linux with systemd. Creates both timer and service unit files.

### States

- `present` - Ensure timer unit exists and is enabled
- `absent` - Ensure timer unit does not exist

### Parameters

**command** (string, required)

- Command to execute when timer fires
- Example: `/usr/local/bin/backup.sh`

**description** (string, optional)

- Description for both timer and service units
- Default: "Keystone Core managed timer: {id}"

**on_calendar** (string, optional)

- Calendar expression for when to run
- Format: OnCalendar from systemd.time(7)
- Examples: `daily`, `weekly`, `*-*-* 02:00:00`, `Mon..Fri 09:00`

**on_boot_sec** (string, optional)

- Time after boot to first run
- Format: timespan (e.g., `5min`, `1h`, `30s`)

**on_unit_active_sec** (string, optional)

- Time after last activation to run again
- Format: timespan

**on_unit_inactive_sec** (string, optional)

- Time after last deactivation to run again
- Format: timespan

**on_startup_sec** (string, optional)

- Time after systemd startup to first run
- Format: timespan

**accuracy_sec** (string, optional)

- Timer accuracy/coalescing window
- Default: `1min`

**randomized_delay_sec** (string, optional)

- Random delay added to timer
- Default: `0`

**persistent** (bool, optional)

- If true, trigger immediately if a run was missed
- Default: false

**wake_system** (bool, optional)

- Wake system from suspend to run timer
- Default: false

**user** (string, optional)

- User to run the service as
- Default: root

**group** (string, optional)

- Group to run the service as

**working_directory** (string, optional)

- Working directory for the command

**environment** (map[string]string, optional)

- Environment variables for the service

### Examples

**Daily backup at 2 AM**:

```yaml
daily_backup:
  module: systemd_timer
  state: present
  command: /usr/local/bin/backup.sh
  description: Daily backup job
  on_calendar: "*-*-* 02:00:00"
  persistent: true
```

**Every 30 minutes**:

```yaml
queue_processor:
  module: systemd_timer
  state: present
  command: /usr/local/bin/process_queue.sh
  on_unit_active_sec: 30min
  on_boot_sec: 1min
```

**Weekday mornings with random delay**:

```yaml
workday_report:
  module: systemd_timer
  state: present
  command: /usr/local/bin/generate_report.sh
  on_calendar: "Mon..Fri 08:00"
  randomized_delay_sec: 15min
```

**With environment and user**:

```yaml
app_maintenance:
  module: systemd_timer
  state: present
  command: php /var/www/artisan maintenance:run
  on_calendar: daily
  user: www-data
  working_directory: /var/www
  environment:
    APP_ENV: production
    LOG_LEVEL: info
```

## Launchd Module

Manage macOS launchd jobs via property list (plist) files.

### Platform

macOS only. Creates plist files in `/Library/LaunchDaemons` or `/Library/LaunchAgents`.

### States

- `present` - Ensure launchd job exists and is loaded
- `absent` - Ensure launchd job does not exist

### Parameters

**label** (string, optional)

- Job label (bundle identifier style)
- Default: Uses state ID
- Example: `com.example.backup`

**program** (string, optional)

- Program to execute
- Example: `/usr/local/bin/backup.sh`
- Either `program` or `program_arguments` is required

**program_arguments** ([]string, optional)

- Program and arguments as array
- Example: `["/usr/bin/python3", "/opt/scripts/task.py", "--verbose"]`

**run_at_load** (bool, optional)

- Run job immediately when loaded
- Default: false

**start_interval** (int, optional)

- Interval in seconds between runs
- Example: `3600` (hourly)

**start_calendar_interval** (map, optional)

- Calendar-based scheduling
- Keys: `Month`, `Day`, `Weekday`, `Hour`, `Minute`
- Weekday: 0=Sunday, 6=Saturday

**watch_paths** ([]string, optional)

- Paths to watch for changes
- Job runs when any watched path changes

**queue_directories** ([]string, optional)

- Directories to watch for new files
- Job runs when files appear in directories

**keep_alive** (bool, optional)

- Keep job running continuously
- Default: false

**working_directory** (string, optional)

- Working directory for the job

**standard_out_path** (string, optional)

- Path for stdout log

**standard_error_path** (string, optional)

- Path for stderr log

**environment_variables** (map[string]string, optional)

- Environment variables for the job

**user** (string, optional)

- User to run the job as

**group** (string, optional)

- Group to run the job as

**nice** (int, optional)

- Nice value (-20 to 20)

**launch_agents** (bool, optional)

- Install in LaunchAgents instead of LaunchDaemons
- Default: false (installs in LaunchDaemons)

### Examples

**Daily backup at 2 AM**:

```yaml
daily_backup:
  module: launchd
  state: present
  label: com.example.backup
  program: /usr/local/bin/backup.sh
  start_calendar_interval:
    Hour: 2
    Minute: 0
  standard_out_path: /var/log/backup.log
  standard_error_path: /var/log/backup.err
```

**Hourly task**:

```yaml
hourly_sync:
  module: launchd
  state: present
  label: com.example.sync
  program_arguments:
    - /usr/bin/rsync
    - -avz
    - /source/
    - /dest/
  start_interval: 3600
```

**Watch directory for new files**:

```yaml
file_processor:
  module: launchd
  state: present
  label: com.example.processor
  program: /usr/local/bin/process_file.sh
  queue_directories:
    - /var/spool/incoming
```

**Keep alive service**:

```yaml
web_server:
  module: launchd
  state: present
  label: com.example.webserver
  program_arguments:
    - /usr/local/bin/myserver
    - --port=8080
  keep_alive: true
  run_at_load: true
  working_directory: /var/www
  user: www
```

## Scheduled_Task Module

Manage Windows Task Scheduler tasks.

### Platform

Windows only. Uses schtasks.exe command.

### States

- `present` - Ensure scheduled task exists
- `absent` - Ensure scheduled task does not exist

### Parameters

**task_path** (string, optional)

- Task folder path
- Default: `\`
- Example: `\MyCompany\Backup`

**execute** (string, required)

- Program to execute
- Alias: `command`
- Example: `C:\Scripts\backup.ps1`

**arguments** (string, optional)

- Arguments to pass to the program

**start_in** (string, optional)

- Working directory

**description** (string, optional)

- Task description

**enabled** (bool, optional)

- Whether task is enabled
- Default: true

**trigger_type** (string, required)

- When to run the task
- Options: `once`, `daily`, `weekly`, `monthly`, `at_logon`, `at_startup`, `on_idle`

**start_time** (string, optional)

- Start time in HH:MM or HH:MM:SS format
- Example: `02:00`, `14:30:00`

**start_date** (string, optional)

- Start date in MM/DD/YYYY format

**days_interval** (int, optional)

- For `daily`: days between runs
- Default: 1

**weeks_interval** (int, optional)

- For `weekly`: weeks between runs
- Default: 1

**days_of_week** ([]string, optional)

- For `weekly`: days to run
- Options: `SUN`, `MON`, `TUE`, `WED`, `THU`, `FRI`, `SAT`

**months_of_year** ([]string, optional)

- For `monthly`: months to run
- Options: `JAN`, `FEB`, `MAR`, `APR`, `MAY`, `JUN`, `JUL`, `AUG`, `SEP`, `OCT`, `NOV`, `DEC`

**days_of_month** ([]int, optional)

- For `monthly`: days to run (1-31)

**repeat_interval** (string, optional)

- How often to repeat within duration
- Example: `1 hour`, `30 minutes`

**repeat_duration** (string, optional)

- How long to repeat
- Example: `8 hours`, `1 day`, `indefinitely`

**delay** (string, optional)

- Delay after trigger before running
- Example: `30 seconds`, `5 minutes`

**run_level** (string, optional)

- Privilege level
- Options: `limited`, `highest`
- Default: `limited`

**user** (string, optional)

- User account to run as

**run_only_if_logged_on** (bool, optional)

- Only run if user is logged on
- Default: false

### Examples

**Daily backup at 2 AM**:

```yaml
daily_backup:
  module: scheduled_task
  state: present
  task_path: \MyCompany
  execute: C:\Scripts\backup.ps1
  trigger_type: daily
  start_time: "02:00"
  run_level: highest
  user: SYSTEM
```

**Weekly task on weekdays**:

```yaml
weekday_report:
  module: scheduled_task
  state: present
  execute: C:\Scripts\generate_report.ps1
  trigger_type: weekly
  start_time: "08:00"
  days_of_week:
    - MON
    - TUE
    - WED
    - THU
    - FRI
```

**On startup with delay**:

```yaml
startup_task:
  module: scheduled_task
  state: present
  execute: C:\Scripts\init.ps1
  trigger_type: at_startup
  delay: 5 minutes
  run_level: highest
```

**Every 15 minutes during work hours**:

```yaml
frequent_check:
  module: scheduled_task
  state: present
  execute: C:\Scripts\check_queue.ps1
  trigger_type: daily
  start_time: "09:00"
  repeat_interval: 15 minutes
  repeat_duration: 8 hours
```

**Monthly on specific days**:

```yaml
monthly_cleanup:
  module: scheduled_task
  state: present
  execute: C:\Scripts\cleanup.ps1
  trigger_type: monthly
  start_time: "03:00"
  days_of_month:
    - 1
    - 15
```

## At Module

Manage one-time scheduled tasks using the Unix `at` command.

### Platform

Linux and macOS. Requires the `at` daemon (atd) to be running.

### States

- `present` - Ensure at job exists
- `absent` - Ensure at job does not exist (removes by tracking marker)

### Parameters

**command** (string, required)

- Command to execute
- Example: `/usr/local/bin/one_time_task.sh`

**time** (string, required)

- When to run the job
- Formats: `HH:MM`, `midnight`, `noon`, `now + 1 hour`, `teatime`
- Examples: `10:00`, `now + 30 minutes`, `2:00 AM`

**date** (string, optional)

- Date for the job
- Formats: `tomorrow`, `next week`, `YYYY-MM-DD`
- Examples: `tomorrow`, `next monday`, `2025-12-25`

**queue** (string, optional)

- Job queue (a-z or A-Z)
- Default: uses default queue
- Higher letters run with higher nice values

**send_mail** (bool, optional)

- Send mail even if no output
- Default: false

**no_mail** (bool, optional)

- Never send mail
- Default: false

### Examples

**Run at specific time today**:

```yaml
afternoon_task:
  module: at
  state: present
  command: /usr/local/bin/process.sh
  time: "14:00"
```

**Run tomorrow morning**:

```yaml
tomorrow_backup:
  module: at
  state: present
  command: /usr/local/bin/backup.sh
  time: "02:00"
  date: tomorrow
```

**Run in 30 minutes**:

```yaml
delayed_task:
  module: at
  state: present
  command: /usr/local/bin/cleanup.sh
  time: now + 30 minutes
```

**Run at midnight**:

```yaml
midnight_job:
  module: at
  state: present
  command: /usr/local/bin/nightly.sh
  time: midnight
```

**With queue priority**:

```yaml
low_priority_job:
  module: at
  state: present
  command: /usr/local/bin/background_task.sh
  time: now + 1 hour
  queue: z
  no_mail: true
```

### Important Notes

1. **Job tracking**: Jobs are tracked using a marker comment (`# Keystone Core: {id}`)
2. **Idempotency**: Existing jobs with the same marker are removed before creating new ones
3. **atd required**: The at daemon must be running (`systemctl start atd`)
4. **One-time only**: Unlike cron, at jobs run once and are then removed

## Mount Module

Manages filesystem mount points across platforms.

### States

| State | Description |
|-------|-------------|
| `mounted` | Mount point is mounted |
| `unmounted` | Mount point is unmounted |
| `present` | Mount entry exists in fstab |
| `absent` | Mount entry removed from fstab |

### Parameters

**path** (string, required)

- Mount point path (e.g., `/mnt/data`)

**device** (string, required)

- Device to mount (e.g., `/dev/sda1`, `UUID=...`, `LABEL=...`)

**fstype** (string, optional)

- Filesystem type (ext4, xfs, nfs, etc.)
- Auto-detected if not specified

**options** (list, optional)

- Mount options (e.g., `["defaults", "noatime"]`)
- Default: `["defaults"]`

**dump** (integer, optional)

- Dump frequency (0 or 1)
- Default: `0`

**pass** (integer, optional)

- fsck pass number (0, 1, or 2)
- Default: `0`

**persist** (boolean, optional)

- Whether to add to /etc/fstab
- Default: `false`

**create_path** (boolean, optional)

- Create mount point directory if missing
- Default: `true`

**owner** (string, optional)

- Owner of mount point directory

**group** (string, optional)

- Group of mount point directory

**mode** (string, optional)

- Permissions of mount point directory
- Default: `"0755"`

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | Full (fstab, /proc/mounts) |
| macOS | Full (diskutil, mount) |
| Windows | Limited (net use for network shares) |

### Examples

#### Basic Mount

```yaml
mount_data:
  module: mount
  state: mounted
  path: /mnt/data
  device: /dev/sdb1
  fstype: ext4
```

#### Persistent Mount with Options

```yaml
mount_backup:
  module: mount
  state: mounted
  path: /mnt/backup
  device: UUID=abc123
  fstype: xfs
  options:
    - defaults
    - noatime
  persist: true
  pass: 2
```

#### NFS Mount

```yaml
mount_nfs:
  module: mount
  state: mounted
  path: /mnt/nfs_share
  device: "192.168.1.100:/exports/data"
  fstype: nfs
  options:
    - defaults
    - nfsvers=4
    - soft
```

## Swap Module

Manages Linux swap space (file or partition).

### States

| State | Description |
|-------|-------------|
| `enabled` | Swap is enabled and active |
| `disabled` | Swap is disabled |
| `present` | Swap file/partition exists |
| `absent` | Swap file removed |

### Parameters

**path** (string, required)

- Path to swap file or device (e.g., `/swapfile`, `/dev/sda2`)

**size** (string, optional)

- Size of swap file (e.g., `"4G"`, `"512M"`, `"1024"`)
- Required for `present` state when creating swap file
- Supports: G/GB, M/MB, K/KB suffixes
- Plain number defaults to MiB

**priority** (integer, optional)

- Swap priority (-1 to 32767)
- Higher priority used first
- Default: -1 (auto)

**persist** (boolean, optional)

- Add to /etc/fstab
- Default: `false`

**label** (string, optional)

- Swap label (mkswap -L)

**uuid** (string, optional)

- Swap UUID (mkswap -U)

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | Full |
| macOS | Not supported (uses dynamic swap) |
| Windows | Not supported (uses pagefile.sys) |

### Examples

#### Create and Enable Swap File

```yaml
create_swap:
  module: swap
  state: enabled
  path: /swapfile
  size: 4G
  persist: true
```

#### Disable Swap

```yaml
disable_swap:
  module: swap
  state: disabled
  path: /swapfile
```

#### Remove Swap File

```yaml
remove_swap:
  module: swap
  state: absent
  path: /swapfile
```

## Lvm_Pv Module

Manages LVM physical volumes.

### States

| State | Description |
|-------|-------------|
| `present` | Physical volume exists |
| `absent` | Physical volume removed |

### Parameters

**device** (string, required)

- Block device path (e.g., `/dev/sdb1`)

**force** (boolean, optional)

- Force creation even if device has data
- Default: `false`

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | Full (requires lvm2 package) |
| macOS | Not supported |
| Windows | Not supported |

### Examples

#### Create Physical Volume

```yaml
create_pv:
  module: lvm_pv
  state: present
  device: /dev/sdb1
```

#### Remove Physical Volume

```yaml
remove_pv:
  module: lvm_pv
  state: absent
  device: /dev/sdb1
```

## Lvm_Vg Module

Manages LVM volume groups.

### States

| State | Description |
|-------|-------------|
| `present` | Volume group exists |
| `absent` | Volume group removed |

### Parameters

**name** (string, required)

- Volume group name

**devices** (list, required for present)

- List of physical volumes to include

**pe_size** (string, optional)

- Physical extent size (e.g., `"4M"`)
- Default: `"4M"`

**force** (boolean, optional)

- Force creation
- Default: `false`

### Examples

#### Create Volume Group

```yaml
create_vg:
  module: lvm_vg
  state: present
  name: vg_data
  devices:
    - /dev/sdb1
    - /dev/sdc1
```

#### Extend Volume Group

```yaml
extend_vg:
  module: lvm_vg
  state: present
  name: vg_data
  devices:
    - /dev/sdb1
    - /dev/sdc1
    - /dev/sdd1
```

## Lvm_Lv Module

Manages LVM logical volumes.

### States

| State | Description |
|-------|-------------|
| `present` | Logical volume exists |
| `absent` | Logical volume removed |

### Parameters

**name** (string, required)

- Logical volume name

**vg** (string, required)

- Volume group name

**size** (string, optional)

- Size of logical volume (e.g., `"10G"`, `"100%FREE"`)
- Required for `present` state

**thin_pool** (string, optional)

- Thin pool for thin provisioned volumes

**snapshot** (string, optional)

- Source logical volume for snapshot

**snapshot_size** (string, optional)

- Size of snapshot

**fstype** (string, optional)

- Filesystem to create (ext4, xfs, etc.)

**force** (boolean, optional)

- Force creation/resize
- Default: `false`

### Examples

#### Create Logical Volume

```yaml
create_lv:
  module: lvm_lv
  state: present
  name: lv_data
  vg: vg_data
  size: 50G
```

#### Create LV with Filesystem

```yaml
create_lv_fs:
  module: lvm_lv
  state: present
  name: lv_apps
  vg: vg_data
  size: 100G
  fstype: xfs
```

#### Use All Free Space

```yaml
create_lv_all:
  module: lvm_lv
  state: present
  name: lv_remaining
  vg: vg_data
  size: 100%FREE
```

## Disk Module

Manages disk partitions using parted.

### States

| State | Description |
|-------|-------------|
| `present` | Partition exists |
| `absent` | Partition removed |
| `formatted` | Partition has filesystem |

### Parameters

**device** (string, required)

- Disk device path (e.g., `/dev/sda`)

**number** (integer, required)

- Partition number

**start** (string, optional)

- Partition start (e.g., `"1MiB"`, `"0%"`)
- Default: `"0%"`

**end** (string, optional)

- Partition end (e.g., `"100%"`, `"50GiB"`)
- Default: `"100%"`

**size** (string, optional)

- Alternative to end, specify size

**type** (string, optional)

- Partition type (primary, extended, logical)
- Default: `"primary"`

**table_type** (string, optional)

- Partition table type (gpt, msdos)
- Default: `"gpt"`

**fstype** (string, optional)

- Partition type code (linux, swap, efi)

**label** (string, optional)

- Partition label

**flags** (list, optional)

- Partition flags (boot, lvm, raid)

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | Full (requires parted) |
| macOS | Not supported |
| Windows | Not supported (use diskpart) |

### Examples

#### Create GPT Partition

```yaml
create_partition:
  module: disk
  state: present
  device: /dev/sdb
  number: 1
  start: 1MiB
  end: 100%
  table_type: gpt
  fstype: linux
```

#### Create Boot Partition

```yaml
create_boot:
  module: disk
  state: present
  device: /dev/sda
  number: 1
  start: 1MiB
  end: 512MiB
  flags:
    - boot
    - esp
  fstype: efi
```

#### Create LVM Partition

```yaml
create_lvm:
  module: disk
  state: present
  device: /dev/sdb
  number: 1
  start: 1MiB
  end: 100%
  flags:
    - lvm
```

## Filesystem Module

Creates and manages filesystems on block devices.

### States

| State | Description |
|-------|-------------|
| `present` | Filesystem exists |
| `absent` | Filesystem removed (wipefs) |

### Parameters

**device** (string, required)

- Block device path (e.g., `/dev/sdb1`, `/dev/vg/lv`)

**fstype** (string, required)

- Filesystem type: ext4, ext3, xfs, btrfs, vfat, ntfs

**label** (string, optional)

- Filesystem label

**uuid** (string, optional)

- Filesystem UUID

**force** (boolean, optional)

- Force recreation of existing filesystem
- Default: `false`

**options** (list, optional)

- Additional mkfs options

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | Full |
| macOS | Not supported |
| Windows | Not supported (use diskpart) |

### Examples

#### Create ext4 Filesystem

```yaml
create_ext4:
  module: filesystem
  state: present
  device: /dev/sdb1
  fstype: ext4
  label: data_disk
```

#### Create XFS Filesystem

```yaml
create_xfs:
  module: filesystem
  state: present
  device: /dev/vg_data/lv_apps
  fstype: xfs
```

#### Force Recreate Filesystem

```yaml
recreate_fs:
  module: filesystem
  state: present
  device: /dev/sdb1
  fstype: ext4
  force: true
```

#### Remove Filesystem

```yaml
wipe_fs:
  module: filesystem
  state: absent
  device: /dev/sdb1
  fstype: ext4
```

### Complete LVM + Filesystem Example

```yaml
# Create physical volume
data_pv:
  module: lvm_pv
  state: present
  device: /dev/sdb

# Create volume group
data_vg:
  module: lvm_vg
  state: present
  name: vg_data
  devices:
    - /dev/sdb
  require:
    - data_pv

# Create logical volume
data_lv:
  module: lvm_lv
  state: present
  name: lv_data
  vg: vg_data
  size: 100%FREE
  require:
    - data_vg

# Create filesystem
data_fs:
  module: filesystem
  state: present
  device: /dev/vg_data/lv_data
  fstype: xfs
  require:
    - data_lv

# Mount the filesystem
data_mount:
  module: mount
  state: mounted
  path: /data
  device: /dev/vg_data/lv_data
  fstype: xfs
  persist: true
  require:
    - data_fs
```

## Authorized Keys Module

Manages SSH authorized_keys entries for user authentication.

### States

| State | Description |
|-------|-------------|
| `present` | SSH key exists in authorized_keys |
| `absent` | SSH key removed from authorized_keys |

### Parameters

**user** (string, required)

- Username whose authorized_keys file to manage

**key** (string, required)

- SSH public key (base64-encoded key portion only)

**key_type** (string, optional)

- Key type: ssh-rsa, ssh-ed25519, ecdsa-sha2-nistp256, etc.
- Default: `ssh-rsa`

**comment** (string, optional)

- Comment to append to key (e.g., user@host)

**options** (string, optional)

- SSH key options (e.g., `no-port-forwarding,command="/bin/date"`)

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | Full |
| macOS | Full |
| Windows | Not supported |

### Examples

#### Add SSH Key

```yaml
add_admin_key:
  module: authorized_keys
  state: present
  user: admin
  key: AAAAB3NzaC1yc2EAAAADAQABAAABgQC...
  key_type: ssh-rsa
  comment: admin@company.com
```

#### Remove SSH Key

```yaml
remove_old_key:
  module: authorized_keys
  state: absent
  user: deploy
  key: AAAAB3NzaC1yc2EAAAADAQABAAAB...
  key_type: ssh-rsa
```

#### Add Key with Options

```yaml
restricted_key:
  module: authorized_keys
  state: present
  user: backup
  key: AAAAB3NzaC1yc2EAAAADAQABAAAB...
  key_type: ssh-rsa
  options: no-port-forwarding,no-X11-forwarding,command="/usr/bin/rsync"
  comment: backup-server
```

## Known Hosts Module

Manages SSH known_hosts entries for host key verification.

### States

| State | Description |
|-------|-------------|
| `present` | Host key exists in known_hosts |
| `absent` | Host key removed from known_hosts |

### Parameters

**host** (string, required)

- Hostname or IP to manage

**key** (string, optional)

- Host public key (if not provided, will be scanned)

**key_type** (string, optional)

- Key type: ssh-rsa, ssh-ed25519, ecdsa-sha2-nistp256
- Default: `ssh-rsa`

**user** (string, optional)

- Manage user's known_hosts instead of system-wide
- If not specified, uses /etc/ssh/ssh_known_hosts

**path** (string, optional)

- Custom known_hosts file path

**hash_known_hosts** (boolean, optional)

- Hash hostnames in known_hosts file
- Default: `false`

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | Full |
| macOS | Full |
| Windows | Not supported |

### Examples

#### Add GitHub Host Key

```yaml
github_known:
  module: known_hosts
  state: present
  host: github.com
  key: AAAAB3NzaC1yc2EAAAABIwAAAQEAq2A7hRGmdnm9...
  key_type: ssh-rsa
```

#### Scan and Add Host Key

```yaml
internal_server:
  module: known_hosts
  state: present
  host: git.internal.corp
  # Key will be scanned automatically
```

#### Per-User Known Hosts

```yaml
user_known_hosts:
  module: known_hosts
  state: present
  host: bastion.example.com
  user: deploy
```

## SSHD Config Module

Manages SSH daemon (sshd) configuration settings.

### States

| State | Description |
|-------|-------------|
| `present` | Configuration setting exists with value |
| `absent` | Configuration setting removed (commented out) |

### Parameters

**name** (string, required)

- Configuration directive name (e.g., PermitRootLogin, PasswordAuthentication)

**value** (string, optional for absent)

- Configuration value

**path** (string, optional)

- Path to sshd_config
- Default: `/etc/ssh/sshd_config`

**backup** (boolean, optional)

- Create backup before modifying
- Default: `true`

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | Full |
| macOS | Full |
| Windows | Not supported |

### Examples

#### Disable Root Login

```yaml
no_root_login:
  module: sshd_config
  state: present
  name: PermitRootLogin
  value: "no"
```

#### Disable Password Authentication

```yaml
no_password_auth:
  module: sshd_config
  state: present
  name: PasswordAuthentication
  value: "no"
```

#### Configure Multiple Settings

```yaml
ssh_port:
  module: sshd_config
  state: present
  name: Port
  value: "22"

ssh_pubkey:
  module: sshd_config
  state: present
  name: PubkeyAuthentication
  value: "yes"

ssh_max_auth:
  module: sshd_config
  state: present
  name: MaxAuthTries
  value: "3"
```

## SELinux Module

Manages SELinux enforcement mode.

### States

| State | Description |
|-------|-------------|
| `enforcing` | SELinux enforcing mode |
| `permissive` | SELinux permissive mode |
| `disabled` | SELinux disabled (requires reboot) |

### Parameters

**persistent** (boolean, optional)

- Persist change to /etc/selinux/config
- Default: `true`

### Platform Support

| Platform | Support |
|----------|---------|
| Linux (RHEL/CentOS/Fedora) | Full |
| Linux (Ubuntu/Debian) | Via selinux packages |
| macOS | Not supported |
| Windows | Not supported |

### Examples

#### Set Enforcing Mode

```yaml
selinux_enforcing:
  module: selinux
  state: enforcing
```

#### Set Permissive Mode

```yaml
selinux_permissive:
  module: selinux
  state: permissive
```

#### Disable SELinux

```yaml
selinux_disabled:
  module: selinux
  state: disabled
  # Requires reboot to take effect
```

## SELinux Boolean Module

Manages SELinux boolean values.

### States

| State | Description |
|-------|-------------|
| `on` | Boolean enabled |
| `off` | Boolean disabled |

### Parameters

**name** (string, required)

- SELinux boolean name

**persistent** (boolean, optional)

- Persist change across reboots
- Default: `true`

### Platform Support

| Platform | Support |
|----------|---------|
| Linux (RHEL/CentOS/Fedora) | Full |
| Linux (Ubuntu/Debian) | Via selinux packages |
| macOS | Not supported |
| Windows | Not supported |

### Examples

#### Enable httpd_can_network_connect

```yaml
httpd_network:
  module: selinux_boolean
  state: on
  name: httpd_can_network_connect
```

#### Disable samba_export_all_rw

```yaml
samba_rw:
  module: selinux_boolean
  state: off
  name: samba_export_all_rw
  persistent: true
```

## AppArmor Module

Manages AppArmor profile enforcement mode.

### States

| State | Description |
|-------|-------------|
| `enforce` | Profile in enforce mode |
| `complain` | Profile in complain mode |
| `disabled` | Profile disabled |

### Parameters

**profile** (string, required)

- AppArmor profile name

### Platform Support

| Platform | Support |
|----------|---------|
| Linux (Ubuntu/Debian) | Full |
| Linux (openSUSE) | Full |
| Linux (RHEL/CentOS) | Not default |
| macOS | Not supported |
| Windows | Not supported |

### Examples

#### Enforce Profile

```yaml
nginx_enforce:
  module: apparmor
  state: enforce
  profile: nginx
```

#### Complain Mode

```yaml
mysql_complain:
  module: apparmor
  state: complain
  profile: mysqld
```

#### Disable Profile

```yaml
cups_disabled:
  module: apparmor
  state: disabled
  profile: cups
```

## AppArmor Profile Module

Installs and manages AppArmor profile files.

### States

| State | Description |
|-------|-------------|
| `present` | Profile installed and loaded |
| `absent` | Profile removed and unloaded |

### Parameters

**name** (string, required)

- Profile name (file name in /etc/apparmor.d/)

**source** (string, optional)

- Source file path for profile content

**content** (string, optional)

- Inline profile content (alternative to source)

**mode** (string, optional)

- Initial mode: enforce, complain
- Default: `enforce`

### Platform Support

| Platform | Support |
|----------|---------|
| Linux (Ubuntu/Debian) | Full |
| Linux (openSUSE) | Full |
| macOS | Not supported |
| Windows | Not supported |

### Examples

#### Install Profile from Source

```yaml
myapp_profile:
  module: apparmor_profile
  state: present
  name: myapp
  source: /opt/myapp/apparmor/myapp.profile
  mode: enforce
```

#### Install Profile with Inline Content

```yaml
custom_profile:
  module: apparmor_profile
  state: present
  name: custom-app
  content: |
    #include <tunables/global>
    /usr/local/bin/custom-app {
      #include <abstractions/base>
      /var/log/custom-app/** rw,
      /etc/custom-app/** r,
    }
  mode: complain
```

#### Remove Profile

```yaml
remove_old_profile:
  module: apparmor_profile
  state: absent
  name: deprecated-app
```

## Timezone Module

Manages the system timezone.

### States

| State | Description |
|-------|-------------|
| `present` | Timezone is set to specified value |

### Parameters

**name** (string, required)

- IANA timezone name (e.g., America/New_York, Europe/London)

### Platform Support

| Platform | Support | Tool |
|----------|---------|------|
| Linux (systemd) | Full | timedatectl |
| macOS | Full | systemsetup |
| Windows | Full | tzutil |
| Linux (non-systemd) | Partial | /etc/timezone |

### Examples

#### Set Timezone

```yaml
set_timezone:
  module: timezone
  state: present
  name: America/Los_Angeles
```

## Locale Module

Manages system locale settings.

### States

| State | Description |
|-------|-------------|
| `present` | Locale is set to specified value |

### Parameters

**name** (string, required)

- Locale name (e.g., en_US.UTF-8)

### Platform Support

| Platform | Support | Tool |
|----------|---------|------|
| Linux (systemd) | Full | localectl |
| macOS | Partial | defaults |
| Windows | Not supported | - |

### Examples

#### Set Locale

```yaml
set_locale:
  module: locale
  state: present
  name: en_US.UTF-8
```

## Hostname Module

Manages the system hostname.

### States

| State | Description |
|-------|-------------|
| `present` | Hostname is set to specified value |

### Parameters

**name** (string, required)

- Desired hostname

**fqdn** (boolean, optional)

- Set fully qualified domain name
- Default: `false`

### Platform Support

| Platform | Support | Tool |
|----------|---------|------|
| Linux (systemd) | Full | hostnamectl |
| macOS | Full | scutil |
| Windows | Full | wmic |
| Linux (non-systemd) | Full | /etc/hostname |

### Examples

#### Set Hostname

```yaml
set_hostname:
  module: hostname
  state: present
  name: webserver01
```

## Hosts Module

Manages /etc/hosts entries.

### States

| State | Description |
|-------|-------------|
| `present` | Host entry exists |
| `absent` | Host entry removed |

### Parameters

**ip** (string, required)

- IP address for the host entry

**name** (string, optional)

- Single hostname (use `name` or `names`)

**names** (list, optional)

- List of hostnames for the IP address

### Platform Support

| Platform | Support | File |
|----------|---------|------|
| Linux | Full | `/etc/hosts` |
| macOS | Full | `/etc/hosts` |
| Windows | Full | `C:\Windows\System32\drivers\etc\hosts` |

### Examples

#### Add Host Entry

```yaml
add_db_host:
  module: hosts
  state: present
  ip: 192.168.1.100
  names:
    - db.example.com
    - db
```

#### Remove Host Entry

```yaml
remove_old_host:
  module: hosts
  state: absent
  ip: 10.0.0.50
  name: old-server
```

## Sysctl Module

Manages Linux kernel parameters via sysctl.

### States

| State | Description |
|-------|-------------|
| `present` | Parameter is set to value |
| `absent` | Parameter is removed from persistent config |

### Parameters

**name** (string, required)

- Sysctl parameter name (e.g., net.ipv4.ip_forward)

**value** (string, required for present)

- Parameter value

**persist** (boolean, optional)

- Write to /etc/sysctl.d/ for persistence
- Default: `true`

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | Full |
| macOS | Not supported |
| Windows | Not supported |

### Examples

#### Enable IP Forwarding

```yaml
enable_ip_forward:
  module: sysctl
  state: present
  name: net.ipv4.ip_forward
  value: "1"
  persist: true
```

#### Set Multiple Parameters

```yaml
tune_network:
  module: sysctl
  state: present
  name: net.core.somaxconn
  value: "65535"

increase_file_limits:
  module: sysctl
  state: present
  name: fs.file-max
  value: "2097152"
```

## Kernel Module Module

Manages Linux kernel modules.

### States

| State | Description |
|-------|-------------|
| `loaded` | Module is loaded |
| `unloaded` | Module is unloaded |
| `blacklisted` | Module is blacklisted |

### Parameters

**name** (string, required)

- Kernel module name

**params** (string, optional)

- Module parameters

**persist** (boolean, optional)

- Make load/blacklist persistent
- Default: `true`

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | Full |
| macOS | Not supported |
| Windows | Not supported |

### Examples

#### Load Module

```yaml
load_vhost:
  module: kernel_module
  state: loaded
  name: vhost_net
  persist: true
```

#### Blacklist Module

```yaml
blacklist_nouveau:
  module: kernel_module
  state: blacklisted
  name: nouveau
  persist: true
```

#### Unload Module

```yaml
unload_unused:
  module: kernel_module
  state: unloaded
  name: floppy
```

## Alternatives Module

Manage system alternatives (update-alternatives on Debian/Ubuntu, alternatives on RHEL/CentOS).

### Platform

Linux only (Debian, Ubuntu, RHEL, CentOS, Fedora)

### States

- `set` - Set alternative to specific path
- `auto` - Set alternative to automatic mode

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `name` | string | Yes | - | Alternative name (e.g., "java", "python") |
| `path` | string | For `set` | - | Path to alternative (required for `set` state) |
| `priority` | int | No | 50 | Priority when registering new alternative |
| `link` | string | No | - | Symlink location (for registration) |

### Examples

#### Set Java Alternative

```yaml
set_java:
  module: alternatives
  state: set
  name: java
  path: /usr/lib/jvm/java-17-openjdk/bin/java
```

#### Set Python Alternative

```yaml
set_python:
  module: alternatives
  state: set
  name: python
  path: /usr/bin/python3.11
```

#### Auto Mode

```yaml
auto_editor:
  module: alternatives
  state: auto
  name: editor
```

#### Register New Alternative with Priority

```yaml
register_node:
  module: alternatives
  state: set
  name: node
  path: /usr/local/node-20/bin/node
  priority: 100
  link: /usr/bin/node
```

### Idempotency

- Checks current alternative before making changes
- Only switches if current differs from desired
- Auto mode checks if already in auto mode

### Error Handling

Common errors:

- **Alternative not installed**: Verify `update-alternatives` or `alternatives` command exists
- **Path not found**: Verify the alternative path exists on the system
- **Permission denied**: Requires root privileges
- **Alternative not registered**: May need to register the alternative first

## Docker Container Module

Manages Docker containers.

### States

| State | Description |
|-------|-------------|
| `running` | Container is running |
| `stopped` | Container exists but is stopped |
| `absent` | Container does not exist |

### Parameters

**name** (string, required)

- Container name

**image** (string, required for running/stopped)

- Docker image to use

**ports** (list, optional)

- Port mappings (e.g., "8080:80")

**volumes** (list, optional)

- Volume mounts (e.g., "/host/path:/container/path")

**env** (map, optional)

- Environment variables

**network** (string, optional)

- Docker network to connect to

**restart** (string, optional)

- Restart policy (no, always, unless-stopped, on-failure)

**command** (string, optional)

- Command to run in container

**force** (boolean, optional)

- Force remove even if running
- Default: `true`

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | Full (requires Docker) |
| macOS | Full (requires Docker Desktop) |
| Windows | Full (requires Docker Desktop) |

### Examples

#### Run Container

```yaml
run_nginx:
  module: docker_container
  state: running
  name: my-nginx
  image: nginx:latest
  ports:
    - "8080:80"
  volumes:
    - "/var/www:/usr/share/nginx/html:ro"
  env:
    NGINX_HOST: example.com
  restart: unless-stopped
```

#### Stop Container

```yaml
stop_redis:
  module: docker_container
  state: stopped
  name: redis-cache
  image: redis:alpine
```

#### Remove Container

```yaml
remove_old:
  module: docker_container
  state: absent
  name: deprecated-app
```

## Docker Image Module

Manages Docker images.

### States

| State | Description |
|-------|-------------|
| `present` | Image is pulled locally |
| `absent` | Image is removed |

### Parameters

**name** (string, required)

- Image name (without tag)

**tag** (string, optional)

- Image tag
- Default: `latest`

**force** (boolean, optional)

- Force remove even if in use
- Default: `false`

### Examples

#### Pull Image

```yaml
pull_nginx:
  module: docker_image
  state: present
  name: nginx
  tag: alpine
```

#### Remove Image

```yaml
remove_old_image:
  module: docker_image
  state: absent
  name: myapp
  tag: v1.0.0
  force: true
```

## Docker Network Module

Manages Docker networks.

### States

| State | Description |
|-------|-------------|
| `present` | Network exists |
| `absent` | Network is removed |

### Parameters

**name** (string, required)

- Network name

**driver** (string, optional)

- Network driver
- Default: `bridge`

**subnet** (string, optional)

- Subnet in CIDR format

**gateway** (string, optional)

- Gateway IP address

**ip_range** (string, optional)

- IP range in CIDR format

### Examples

#### Create Network

```yaml
create_app_network:
  module: docker_network
  state: present
  name: app-network
  driver: bridge
  subnet: 172.20.0.0/16
  gateway: 172.20.0.1
```

#### Remove Network

```yaml
remove_old_network:
  module: docker_network
  state: absent
  name: deprecated-network
```

## Docker Volume Module

Manages Docker volumes.

### States

| State | Description |
|-------|-------------|
| `present` | Volume exists |
| `absent` | Volume is removed |

### Parameters

**name** (string, required)

- Volume name

**driver** (string, optional)

- Volume driver
- Default: `local`

**opts** (map, optional)

- Driver-specific options

**force** (boolean, optional)

- Force remove
- Default: `false`

### Examples

#### Create Volume

```yaml
create_data_volume:
  module: docker_volume
  state: present
  name: app-data
  driver: local
```

#### Create NFS Volume

```yaml
create_nfs_volume:
  module: docker_volume
  state: present
  name: shared-data
  driver: local
  opts:
    type: nfs
    device: ":/data/shared"
    o: "addr=nfs-server.example.com,rw"
```

## Podman Container Module

Manages Podman containers. Same interface as Docker.

### States

| State | Description |
|-------|-------------|
| `running` | Container is running |
| `stopped` | Container exists but is stopped |
| `absent` | Container does not exist |

### Parameters

Same as docker_container module.

### Examples

#### Run Container

```yaml
run_nginx:
  module: podman_container
  state: running
  name: my-nginx
  image: docker.io/library/nginx:latest
  ports:
    - "8080:80"
```

## Podman Image Module

Manages Podman images. Same interface as Docker.

### States

| State | Description |
|-------|-------------|
| `present` | Image is pulled locally |
| `absent` | Image is removed |

### Parameters

Same as docker_image module.

### Examples

#### Pull Image

```yaml
pull_nginx:
  module: podman_image
  state: present
  name: docker.io/library/nginx
  tag: alpine
```

## Podman Network Module

Manages Podman networks. Same interface as Docker.

### States

| State | Description |
|-------|-------------|
| `present` | Network exists |
| `absent` | Network is removed |

### Parameters

Same as docker_network module.

### Examples

#### Create Network

```yaml
create_network:
  module: podman_network
  state: present
  name: app-network
  subnet: 172.20.0.0/16
```

## Podman Volume Module

Manages Podman volumes. Same interface as Docker.

### States

| State | Description |
|-------|-------------|
| `present` | Volume exists |
| `absent` | Volume is removed |

### Parameters

Same as docker_volume module.

### Examples

#### Create Volume

```yaml
create_volume:
  module: podman_volume
  state: present
  name: app-data
```

---

## Database Modules

Keystone Core provides database management modules for PostgreSQL, MySQL/MariaDB, and Redis. These modules enable declarative management of databases, users, extensions, and configuration.

## PostgreSQL Database Module

Manages PostgreSQL databases.

### States

| State | Description |
|-------|-------------|
| `present` | Database exists |
| `absent` | Database is removed |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `name` | string | Yes | - | Database name |
| `host` | string | No | `localhost` | PostgreSQL server host |
| `port` | integer | No | `5432` | PostgreSQL server port |
| `user` | string | No | `postgres` | Admin user for connection |
| `password` | string | No | - | Admin password (prefer PGPASSWORD env var) |
| `maintenance_db` | string | No | `postgres` | Database to connect to for management |
| `owner` | string | No | - | Database owner |
| `encoding` | string | No | `UTF8` | Character encoding |
| `template` | string | No | `template0` | Template database |
| `lc_collate` | string | No | - | Collation order |
| `lc_ctype` | string | No | - | Character classification |

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | ✅ Full |
| macOS | ✅ Full |
| Windows | ✅ Full |

### Examples

#### Create Database

```yaml
create_app_db:
  module: postgres_database
  state: present
  name: myapp
  owner: myapp_user
  encoding: UTF8
```

#### Create Database with Locale

```yaml
create_localized_db:
  module: postgres_database
  state: present
  name: german_app
  encoding: UTF8
  lc_collate: de_DE.UTF-8
  lc_ctype: de_DE.UTF-8
```

#### Remove Database

```yaml
remove_old_db:
  module: postgres_database
  state: absent
  name: legacy_app
```

---

## PostgreSQL User Module

Manages PostgreSQL users/roles.

### States

| State | Description |
|-------|-------------|
| `present` | User/role exists |
| `absent` | User/role is removed |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `name` | string | Yes | - | Role name |
| `host` | string | No | `localhost` | PostgreSQL server host |
| `port` | integer | No | `5432` | PostgreSQL server port |
| `user` | string | No | `postgres` | Admin user for connection |
| `password` | string | No | - | Admin password (prefer PGPASSWORD env var) |
| `maintenance_db` | string | No | `postgres` | Database to connect to |
| `role_password` | string | No | - | Password for the new role |
| `superuser` | boolean | No | `false` | Grant SUPERUSER privilege |
| `createdb` | boolean | No | `false` | Grant CREATEDB privilege |
| `createrole` | boolean | No | `false` | Grant CREATEROLE privilege |
| `login` | boolean | No | `true` | Allow login |
| `replication` | boolean | No | `false` | Grant REPLICATION privilege |
| `connection_limit` | integer | No | `-1` | Connection limit (-1 = unlimited) |

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | ✅ Full |
| macOS | ✅ Full |
| Windows | ✅ Full |

### Examples

#### Create Application User

```yaml
create_app_user:
  module: postgres_user
  state: present
  name: myapp_user
  role_password: "secure_password"
  login: true
```

#### Create Admin User

```yaml
create_admin:
  module: postgres_user
  state: present
  name: dba_user
  role_password: "admin_password"
  superuser: true
  createdb: true
  createrole: true
```

#### Create Read-Only User

```yaml
create_readonly:
  module: postgres_user
  state: present
  name: reader
  role_password: "read_password"
  login: true
  connection_limit: 10
```

---

## PostgreSQL Extension Module

Manages PostgreSQL extensions within a database.

### States

| State | Description |
|-------|-------------|
| `present` | Extension is installed |
| `absent` | Extension is removed |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `name` | string | Yes | - | Extension name |
| `database` | string | Yes | - | Database to install extension in |
| `host` | string | No | `localhost` | PostgreSQL server host |
| `port` | integer | No | `5432` | PostgreSQL server port |
| `user` | string | No | `postgres` | Admin user for connection |
| `schema` | string | No | - | Schema to install extension in |
| `version` | string | No | - | Specific version to install |
| `cascade` | boolean | No | `false` | Install dependencies (CASCADE) |

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | ✅ Full |
| macOS | ✅ Full |
| Windows | ✅ Full |

### Examples

#### Install UUID Extension

```yaml
install_uuid:
  module: postgres_extension
  state: present
  name: uuid-ossp
  database: myapp
```

#### Install PostGIS

```yaml
install_postgis:
  module: postgres_extension
  state: present
  name: postgis
  database: geodata
  cascade: true
```

#### Install pg_stat_statements

```yaml
install_stats:
  module: postgres_extension
  state: present
  name: pg_stat_statements
  database: postgres
  schema: public
```

---

## MySQL Database Module

Manages MySQL/MariaDB databases.

### States

| State | Description |
|-------|-------------|
| `present` | Database exists |
| `absent` | Database is removed |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `name` | string | Yes | - | Database name |
| `host` | string | No | `localhost` | MySQL server host |
| `port` | integer | No | `3306` | MySQL server port |
| `user` | string | No | `root` | Admin user for connection |
| `password` | string | No | - | Admin password |
| `socket` | string | No | - | Unix socket path (overrides host/port) |
| `charset` | string | No | `utf8mb4` | Default character set |
| `collation` | string | No | `utf8mb4_unicode_ci` | Default collation |

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | ✅ Full |
| macOS | ✅ Full |
| Windows | ✅ Full |

### Examples

#### Create Database

```yaml
create_app_db:
  module: mysql_database
  state: present
  name: myapp
  charset: utf8mb4
  collation: utf8mb4_unicode_ci
```

#### Create Database via Socket

```yaml
create_db_socket:
  module: mysql_database
  state: present
  name: myapp
  socket: /var/run/mysqld/mysqld.sock
```

#### Remove Database

```yaml
remove_old_db:
  module: mysql_database
  state: absent
  name: legacy_app
```

---

## MySQL User Module

Manages MySQL/MariaDB users and their privileges.

### States

| State | Description |
|-------|-------------|
| `present` | User exists |
| `absent` | User is removed |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `name` | string | Yes | - | Username |
| `host` | string | No | `localhost` | MySQL server host |
| `port` | integer | No | `3306` | MySQL server port |
| `user` | string | No | `root` | Admin user for connection |
| `password` | string | No | - | Admin password |
| `socket` | string | No | - | Unix socket path |
| `host_name` | string | No | `%` | Host the user can connect from |
| `user_password` | string | No | - | Password for the new user |
| `priv` | string | No | - | Privileges in format "db.table:PRIV1,PRIV2" |

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | ✅ Full |
| macOS | ✅ Full |
| Windows | ✅ Full |

### Privilege Format

The `priv` parameter uses the format: `database.table:PRIVILEGES`

Examples:

- `mydb.*:ALL` - All privileges on all tables in mydb
- `mydb.users:SELECT,INSERT,UPDATE` - SELECT, INSERT, UPDATE on mydb.users
- `*.*:SELECT` - SELECT on all databases and tables

### Examples

#### Create Application User

```yaml
create_app_user:
  module: mysql_user
  state: present
  name: myapp
  host_name: localhost
  user_password: "secure_password"
  priv: "myapp_db.*:ALL"
```

#### Create Read-Only User

```yaml
create_readonly:
  module: mysql_user
  state: present
  name: reader
  host_name: "%"
  user_password: "read_password"
  priv: "myapp_db.*:SELECT"
```

#### Create User from Specific Host

```yaml
create_remote_user:
  module: mysql_user
  state: present
  name: backup_user
  host_name: "192.168.1.%"
  user_password: "backup_password"
  priv: "*.*:SELECT,LOCK TABLES"
```

---

## Redis Module

Manages Redis configuration settings and ACL users.

### States

| State | Description |
|-------|-------------|
| `present` | Configuration/user exists |
| `absent` | Configuration reset or user removed |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `name` | string | Yes | - | Config key name or ACL username |
| `type` | string | No | `config` | Type: "config" or "acl" |
| `host` | string | No | `localhost` | Redis server host |
| `port` | integer | No | `6379` | Redis server port |
| `password` | string | No | - | Redis AUTH password |
| `socket` | string | No | - | Unix socket path |
| `value` | string | No | - | Config value (for type=config) |
| `acl_password` | string | No | - | ACL user password (for type=acl) |
| `acl_rules` | string | No | - | ACL rules (for type=acl) |

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | ✅ Full |
| macOS | ✅ Full |
| Windows | ✅ Full |

### ACL Rules Format

ACL rules follow Redis ACL syntax:

- `on` - Enable user
- `+@all` - Allow all commands
- `~*` - Allow all keys
- `+get +set` - Allow specific commands
- `~cache:*` - Allow keys matching pattern

### Examples

#### Set Config Value

```yaml
set_maxmemory:
  module: redis
  state: present
  name: maxmemory
  type: config
  value: "2gb"
```

#### Set Eviction Policy

```yaml
set_eviction:
  module: redis
  state: present
  name: maxmemory-policy
  type: config
  value: "allkeys-lru"
```

#### Create ACL User

```yaml
create_app_user:
  module: redis
  state: present
  name: appuser
  type: acl
  acl_password: "secure_password"
  acl_rules: "on +@all ~app:*"
```

#### Create Read-Only User

```yaml
create_readonly:
  module: redis
  state: present
  name: reader
  type: acl
  acl_password: "read_password"
  acl_rules: "on +@read ~*"
```

#### Delete ACL User

```yaml
delete_user:
  module: redis
  state: absent
  name: olduser
  type: acl
```

---

## Nginx Site Module

Manages Nginx site configurations using the sites-available/sites-enabled pattern.

### States

| State | Description |
|-------|-------------|
| `enabled` | Site config exists and is enabled (symlinked) |
| `disabled` | Site config exists but is not enabled |
| `absent` | Site config is removed |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `name` | string | yes | - | Site name (filename without path) |
| `content` | string | no | - | Site configuration content |
| `source` | string | no | - | Path to source configuration file |
| `reload` | boolean | no | true | Reload Nginx after changes |

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | ✅ Full |
| macOS | ✅ Full (Homebrew paths) |
| Windows | ❌ Not supported |

### Example

```yaml
enable_myapp:
  module: nginx_site
  state: enabled
  name: myapp.conf
  content: |
    server {
        listen 80;
        server_name myapp.example.com;

        location / {
            proxy_pass http://localhost:3000;
        }
    }

disable_default:
  module: nginx_site
  state: disabled
  name: default
```

---

## Nginx Config Module

Manages Nginx configuration snippets in conf.d or custom directories.

### States

| State | Description |
|-------|-------------|
| `present` | Config snippet exists |
| `absent` | Config snippet is removed |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `name` | string | yes | - | Config filename |
| `content` | string | conditional | - | Config content (required for present) |
| `source` | string | conditional | - | Source file path (alternative to content) |
| `dest` | string | no | /etc/nginx/conf.d | Destination directory |
| `reload` | boolean | no | true | Reload Nginx after changes |

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | ✅ Full |
| macOS | ✅ Full (Homebrew paths) |
| Windows | ❌ Not supported |

### Example

```yaml
upstream_config:
  module: nginx_config
  state: present
  name: upstream-backend.conf
  content: |
    upstream backend {
        server 10.0.0.1:8080;
        server 10.0.0.2:8080;
        server 10.0.0.3:8080;
    }

rate_limiting:
  module: nginx_config
  state: present
  name: rate-limit.conf
  dest: /etc/nginx/snippets
  content: |
    limit_req_zone $binary_remote_addr zone=api:10m rate=10r/s;
```

---

## Nginx Upstream Module

Manages Nginx upstream configurations for load balancing across backend servers.

### States

| State | Description |
|-------|-------------|
| `present` | Upstream configuration exists |
| `absent` | Upstream configuration is removed |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `name` | string | yes | - | Upstream name |
| `servers` | list | yes | - | List of backend server addresses (host:port) |
| `method` | string | no | round_robin | Load balancing method (round_robin, least_conn, ip_hash, hash, random) |
| `hash_key` | string | no | - | Hash key for hash method (e.g., "$request_uri") |
| `keepalive` | integer | no | - | Number of keepalive connections per worker |
| `keepalive_requests` | integer | no | - | Max requests per keepalive connection |
| `keepalive_timeout` | string | no | - | Keepalive connection timeout |
| `reload` | boolean | no | true | Reload Nginx after changes |

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | ✅ Full |
| macOS | ✅ Full (Homebrew paths) |
| Windows | ❌ Not supported |

### Example

```yaml
backend_upstream:
  module: nginx_upstream
  state: present
  name: backend
  servers:
    - 10.0.0.1:8080
    - 10.0.0.2:8080
    - 10.0.0.3:8080
  method: least_conn
  keepalive: 32

session_upstream:
  module: nginx_upstream
  state: present
  name: api
  servers:
    - 10.0.0.1:3000
    - 10.0.0.2:3000
  method: ip_hash
```

---

## Nginx Proxy Module

Manages Nginx reverse proxy server configurations.

### States

| State | Description |
|-------|-------------|
| `present` | Proxy configuration exists |
| `absent` | Proxy configuration is removed |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `name` | string | yes | - | Server block name |
| `backend` | string | yes | - | Backend URL (e.g., <http://upstream> or <http://host:port>) |
| `listen` | string | no | 80 | Listen port or address:port |
| `server_name` | string | no | _ | Server name(s) |
| `location` | string | no | / | Location path |
| `proxy_headers` | boolean | no | true | Add standard proxy headers |
| `headers` | list | no | - | Custom headers to add |
| `websocket` | boolean | no | false | Enable WebSocket proxying |
| `connect_timeout` | string | no | - | Proxy connect timeout |
| `read_timeout` | string | no | - | Proxy read timeout |
| `send_timeout` | string | no | - | Proxy send timeout |
| `reload` | boolean | no | true | Reload Nginx after changes |

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | ✅ Full |
| macOS | ✅ Full (Homebrew paths) |
| Windows | ❌ Not supported |

### Example

```yaml
api_proxy:
  module: nginx_proxy
  state: present
  name: api
  backend: http://backend
  listen: "80"
  server_name: api.example.com
  proxy_headers: true
  connect_timeout: 30s
  read_timeout: 60s

websocket_proxy:
  module: nginx_proxy
  state: present
  name: ws
  backend: http://localhost:3000
  listen: "8080"
  server_name: ws.example.com
  websocket: true
```

---

## Nginx SSL Module

Manages Nginx SSL/TLS configurations.

### States

| State | Description |
|-------|-------------|
| `present` | SSL configuration exists |
| `absent` | SSL configuration is removed |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `name` | string | yes | - | Configuration name |
| `certificate` | string | yes | - | Path to SSL certificate file |
| `certificate_key` | string | yes | - | Path to SSL private key file |
| `listen` | string | no | 443 ssl | Listen directive |
| `server_name` | string | no | _ | Server name(s) |
| `protocols` | string | no | TLSv1.2 TLSv1.3 | SSL protocols |
| `ciphers` | string | no | - | SSL ciphers |
| `prefer_server_ciphers` | boolean | no | true | Prefer server ciphers |
| `session_cache` | string | no | shared:SSL:10m | Session cache configuration |
| `session_timeout` | string | no | 1d | Session timeout |
| `session_tickets` | boolean | no | false | Enable session tickets |
| `ocsp_stapling` | boolean | no | false | Enable OCSP stapling |
| `trusted_certificate` | string | no | - | Path to trusted CA certificate for OCSP |
| `hsts` | boolean | no | false | Enable HSTS header |
| `hsts_max_age` | integer | no | 31536000 | HSTS max-age value |
| `dhparam` | string | no | - | Path to DH parameters file |
| `reload` | boolean | no | true | Reload Nginx after changes |

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | ✅ Full |
| macOS | ✅ Full (Homebrew paths) |
| Windows | ❌ Not supported |

### Example

```yaml
secure_server:
  module: nginx_ssl
  state: present
  name: secure
  certificate: /etc/ssl/certs/server.crt
  certificate_key: /etc/ssl/private/server.key
  protocols: TLSv1.2 TLSv1.3
  ciphers: ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256
  session_cache: shared:SSL:10m
  session_timeout: 1d
  ocsp_stapling: true
  hsts: true
  hsts_max_age: 31536000
```

---

## Nginx Location Module

Manages Nginx location block configurations.

### States

| State | Description |
|-------|-------------|
| `present` | Location configuration exists |
| `absent` | Location configuration is removed |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `name` | string | yes | - | Location name (for file naming) |
| `path` | string | yes | - | Location path (e.g., /, /api, ~* \.php$) |
| `modifier` | string | no | - | Location modifier (=, ~, ~*, ^~) |
| `root` | string | no | - | Document root for this location |
| `alias` | string | no | - | Alias path |
| `try_files` | string | no | - | try_files directive |
| `proxy_pass` | string | no | - | Proxy pass URL |
| `fastcgi_pass` | string | no | - | FastCGI pass address |
| `index` | string | no | - | Index files |
| `autoindex` | boolean | no | false | Enable directory listing |
| `allow` | list | no | - | Allowed IP addresses/ranges |
| `deny` | list | no | - | Denied IP addresses/ranges |
| `directives` | list | no | - | Additional custom directives |
| `rewrite` | string | no | - | Rewrite rule |
| `return_code` | integer | no | - | Return status code |
| `return_text` | string | no | - | Return text/URL |
| `reload` | boolean | no | true | Reload Nginx after changes |

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | ✅ Full |
| macOS | ✅ Full (Homebrew paths) |
| Windows | ❌ Not supported |

### Example

```yaml
static_files:
  module: nginx_location
  state: present
  name: static
  path: /static
  root: /var/www/html
  try_files: $uri $uri/ =404

api_proxy:
  module: nginx_location
  state: present
  name: api
  path: /api
  proxy_pass: http://backend

admin_access:
  module: nginx_location
  state: present
  name: admin
  path: /admin
  modifier: "="
  allow:
    - 192.168.1.0/24
    - 10.0.0.0/8
  deny:
    - all

php_handler:
  module: nginx_location
  state: present
  name: php
  path: \.php$
  modifier: "~"
  fastcgi_pass: unix:/run/php/php8.1-fpm.sock
  directives:
    - fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name
    - include fastcgi_params
```

---

## Nginx Rate Limit Module

Manages Nginx rate limiting configurations.

### States

| State | Description |
|-------|-------------|
| `present` | Rate limit configuration exists |
| `absent` | Rate limit configuration is removed |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `name` | string | yes | - | Configuration name |
| `zone` | string | yes | - | Zone name for shared memory |
| `rate` | string | yes | - | Request rate (e.g., 10r/s, 100r/m) |
| `key` | string | no | $binary_remote_addr | Key for rate limiting |
| `size` | string | no | 10m | Zone size |
| `burst` | integer | no | - | Burst size |
| `nodelay` | boolean | no | false | Process burst without delay |
| `conn_zone` | string | no | - | Connection limit zone name |
| `conn_limit` | integer | no | - | Connection limit per key |
| `conn_key` | string | no | $binary_remote_addr | Key for connection limiting |
| `conn_size` | string | no | 10m | Connection zone size |
| `reload` | boolean | no | true | Reload Nginx after changes |

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | ✅ Full |
| macOS | ✅ Full (Homebrew paths) |
| Windows | ❌ Not supported |

### Example

```yaml
api_rate_limit:
  module: nginx_rate_limit
  state: present
  name: api_limit
  zone: api
  rate: 10r/s
  burst: 20
  nodelay: true

per_user_limit:
  module: nginx_rate_limit
  state: present
  name: user_limit
  zone: user
  rate: 100r/m
  key: $http_x_api_key
  size: 10m
  burst: 50

connection_limit:
  module: nginx_rate_limit
  state: present
  name: conn_limit
  zone: connections
  rate: 10r/s
  conn_zone: conn
  conn_limit: 10
```

---

## Apache Site Module

Manages Apache virtual host configurations using a2ensite/a2dissite.

### States

| State | Description |
|-------|-------------|
| `enabled` | Site config exists and is enabled |
| `disabled` | Site config exists but is disabled |
| `absent` | Site config is removed |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `name` | string | yes | - | Site name (without .conf extension) |
| `content` | string | no | - | Virtual host configuration content |
| `source` | string | no | - | Path to source configuration file |
| `reload` | boolean | no | true | Reload Apache after changes |

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | ✅ Full (a2ensite/a2dissite or manual symlinks) |
| macOS | ✅ Full (Homebrew paths) |
| Windows | ❌ Not supported |

### Example

```yaml
enable_myapp:
  module: apache_site
  state: enabled
  name: myapp
  content: |
    <VirtualHost *:80>
        ServerName myapp.example.com
        DocumentRoot /var/www/myapp

        <Directory /var/www/myapp>
            AllowOverride All
            Require all granted
        </Directory>

        ErrorLog ${APACHE_LOG_DIR}/myapp-error.log
        CustomLog ${APACHE_LOG_DIR}/myapp-access.log combined
    </VirtualHost>

disable_default:
  module: apache_site
  state: disabled
  name: 000-default
```

---

## Apache Module Module

Manages Apache modules using a2enmod/a2dismod.

### States

| State | Description |
|-------|-------------|
| `enabled` | Module is loaded and enabled |
| `disabled` | Module is disabled |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `name` | string | yes | - | Module name (e.g., rewrite, ssl, proxy) |
| `reload` | boolean | no | true | Reload Apache after changes |

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | ✅ Full (requires a2enmod/a2dismod) |
| macOS | ⚠️ Limited (manual configuration) |
| Windows | ❌ Not supported |

### Example

```yaml
enable_rewrite:
  module: apache_module
  state: enabled
  name: rewrite

enable_ssl:
  module: apache_module
  state: enabled
  name: ssl

enable_proxy_modules:
  module: apache_module
  state: enabled
  name: proxy

enable_proxy_http:
  module: apache_module
  state: enabled
  name: proxy_http
  require:
    - enable_proxy_modules

disable_autoindex:
  module: apache_module
  state: disabled
  name: autoindex
```

---

## git Module {#git-module}

Manage Git repository clones on the system.

### States

| State | Description |
|-------|-------------|
| `present` | Repository is cloned to destination |
| `absent` | Repository is removed from destination |
| `latest` | Repository is cloned and updated to latest version |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `repo` | string | yes* | - | Repository URL (SSH or HTTPS) |
| `dest` | string | yes | - | Destination directory for clone |
| `version` | string | no | HEAD | Branch, tag, or commit to checkout |
| `force` | boolean | no | false | Force reset working tree on update |
| `depth` | integer | no | 0 | Shallow clone depth (0 = full clone) |
| `recursive` | boolean | no | true | Clone submodules recursively |
| `ssh_key` | string | no | - | Path to SSH private key for authentication |

*`repo` is required for `present` and `latest` states, optional for `absent`.

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | ✅ Full |
| macOS | ✅ Full |
| Windows | ✅ Full |

### Example

```yaml
clone_project:
  module: git
  state: present
  repo: https://github.com/example/project.git
  dest: /opt/project
  version: v1.2.0

clone_with_ssh:
  module: git
  state: latest
  repo: git@github.com:example/project.git
  dest: /opt/project
  ssh_key: /root/.ssh/deploy_key
  force: true

shallow_clone:
  module: git
  state: present
  repo: https://github.com/example/large-repo.git
  dest: /opt/large-repo
  depth: 1

remove_repo:
  module: git
  state: absent
  dest: /opt/old-project
```

### Metadata Returned

When checking state, the following metadata is available:

| Field | Type | Description |
|-------|------|-------------|
| `exists` | boolean | Whether destination directory exists |
| `is_git_repo` | boolean | Whether destination is a git repository |
| `remote_url` | string | Current remote origin URL |
| `current_commit` | string | Current HEAD commit SHA |
| `current_branch` | string | Current branch name |
| `is_clean` | boolean | Whether working tree is clean |
| `behind_count` | integer | Number of commits behind origin (for `latest` state) |

---

## git_config Module {#git_config-module}

Manage Git configuration settings.

### States

| State | Description |
|-------|-------------|
| `present` | Configuration setting exists with specified value |
| `absent` | Configuration setting is removed |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `name` | string | yes | - | Configuration key (e.g., user.email) |
| `value` | string | yes* | - | Configuration value |
| `scope` | string | no | global | Config scope: global, system, local, worktree |
| `file` | string | no | - | Custom config file path (overrides scope) |

*`value` is required for `present` state.

### Scope Options

| Scope | Description | Config File |
|-------|-------------|-------------|
| `system` | System-wide settings | `/etc/gitconfig` |
| `global` | User-level settings | `~/.gitconfig` |
| `local` | Repository-level settings | `.git/config` |
| `worktree` | Worktree-level settings | `.git/config.worktree` |

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | ✅ Full |
| macOS | ✅ Full |
| Windows | ✅ Full |

### Example

```yaml
set_user_email:
  module: git_config
  state: present
  name: user.email
  value: developer@example.com
  scope: global

set_user_name:
  module: git_config
  state: present
  name: user.name
  value: Developer Name
  scope: global

enable_colors:
  module: git_config
  state: present
  name: color.ui
  value: auto
  scope: global

configure_merge_strategy:
  module: git_config
  state: present
  name: pull.rebase
  value: "true"
  scope: global

remove_obsolete_config:
  module: git_config
  state: absent
  name: old.setting
  scope: global

custom_config_file:
  module: git_config
  state: present
  name: custom.setting
  value: custom_value
  file: /path/to/custom/.gitconfig
```

### Metadata Returned

| Field | Type | Description |
|-------|------|-------------|
| `current_value` | string | Current value of the configuration setting |
| `scope` | string | Scope where the setting was found |

---

## x509 Module {#x509-module}

Manage X.509 certificates and private keys.

### States

| State | Description |
|-------|-------------|
| `present` | Certificate and key exist with specified parameters |
| `absent` | Certificate and key are removed |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `path` | string | yes | - | Path to the certificate file |
| `key_path` | string | no | - | Path to the private key file (defaults to path with .key extension) |
| `common_name` | string | yes* | - | Common Name (CN) for the certificate |
| `organization` | string | no | - | Organization (O) for the certificate |
| `country` | string | no | - | Country (C) for the certificate |
| `validity_days` | int | no | 365 | Certificate validity in days |
| `key_type` | string | no | rsa | Key type: rsa, ecdsa, ed25519 |
| `key_size` | int | no | 2048 | Key size (for RSA: 2048, 4096; for ECDSA: 256, 384, 521) |
| `self_signed` | bool | no | true | Generate a self-signed certificate |
| `is_ca` | bool | no | false | Mark certificate as a CA |
| `san_names` | []string | no | - | Subject Alternative Names (DNS names) |
| `san_ips` | []string | no | - | Subject Alternative Names (IP addresses) |

*`common_name` is required for `present` state.

### Key Types

| Key Type | Description | Key Sizes |
|----------|-------------|-----------|
| `rsa` | RSA key pair | 2048, 4096 |
| `ecdsa` | ECDSA key pair | 256 (P-256), 384 (P-384), 521 (P-521) |
| `ed25519` | Ed25519 key pair | Fixed size |

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | ✅ Full |
| macOS | ✅ Full |
| Windows | ✅ Full |

### Example

```yaml
create_server_cert:
  module: x509
  state: present
  path: /etc/ssl/certs/server.crt
  key_path: /etc/ssl/private/server.key
  common_name: server.example.com
  organization: Example Inc
  country: US
  validity_days: 365
  san_names:
    - server.example.com
    - www.example.com
  san_ips:
    - 192.168.1.100

create_ecdsa_cert:
  module: x509
  state: present
  path: /etc/ssl/certs/ecdsa.crt
  key_path: /etc/ssl/private/ecdsa.key
  common_name: ecdsa.example.com
  key_type: ecdsa
  key_size: 384
  validity_days: 730

create_ed25519_cert:
  module: x509
  state: present
  path: /etc/ssl/certs/ed25519.crt
  common_name: ed25519.example.com
  key_type: ed25519

remove_old_cert:
  module: x509
  state: absent
  path: /etc/ssl/certs/old.crt
  key_path: /etc/ssl/private/old.key
```

### Metadata Returned

| Field | Type | Description |
|-------|------|-------------|
| `subject` | string | Certificate subject DN |
| `issuer` | string | Certificate issuer DN |
| `not_before` | string | Certificate validity start (RFC3339) |
| `not_after` | string | Certificate validity end (RFC3339) |
| `serial` | string | Certificate serial number |
| `key_type` | string | Private key algorithm |

---

## ca Module {#ca-module}

Manage Certificate Authority operations including CA creation and certificate signing.

### States

| State | Description |
|-------|-------------|
| `present` | CA certificate and key exist |
| `absent` | CA certificate and key are removed |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `path` | string | yes | - | Path to the CA certificate file |
| `key_path` | string | no | - | Path to the CA private key file |
| `common_name` | string | yes* | - | Common Name (CN) for the CA |
| `organization` | string | no | - | Organization (O) for the CA |
| `country` | string | no | - | Country (C) for the CA |
| `validity_days` | int | no | 3650 | CA validity in days (default 10 years) |
| `key_type` | string | no | rsa | Key type: rsa, ecdsa, ed25519 |
| `key_size` | int | no | 4096 | Key size for RSA/ECDSA |
| `max_path_len` | int | no | 0 | Maximum intermediate CA chain length |

*`common_name` is required for `present` state.

### CA Signing (Programmatic)

The CA module provides a `SignCertificate` method for signing CSRs:

```go
signedCert, err := caModule.SignCertificate(
    caCertPath,
    caKeyPath,
    csrPath,
    outputPath,
    validityDays,
)
```

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | ✅ Full |
| macOS | ✅ Full |
| Windows | ✅ Full |

### Example

```yaml
create_root_ca:
  module: ca
  state: present
  path: /etc/ssl/ca/root-ca.crt
  key_path: /etc/ssl/ca/root-ca.key
  common_name: Example Root CA
  organization: Example Inc
  country: US
  validity_days: 3650
  key_type: rsa
  key_size: 4096

create_intermediate_ca:
  module: ca
  state: present
  path: /etc/ssl/ca/intermediate-ca.crt
  key_path: /etc/ssl/ca/intermediate-ca.key
  common_name: Example Intermediate CA
  organization: Example Inc
  validity_days: 1825
  max_path_len: 0

remove_old_ca:
  module: ca
  state: absent
  path: /etc/ssl/ca/old-ca.crt
  key_path: /etc/ssl/ca/old-ca.key
```

### Metadata Returned

| Field | Type | Description |
|-------|------|-------------|
| `subject` | string | CA certificate subject DN |
| `issuer` | string | CA certificate issuer DN |
| `not_before` | string | Certificate validity start (RFC3339) |
| `not_after` | string | Certificate validity end (RFC3339) |
| `serial` | string | Certificate serial number |
| `is_ca` | bool | Whether certificate is a CA |
| `key_type` | string | Private key algorithm |

---

## acme Module {#acme-module}

Manage ACME/Let's Encrypt certificates with automatic renewal.

### States

| State | Description |
|-------|-------------|
| `present` | Certificate exists and is valid |
| `absent` | Certificate and key are removed |
| `renewed` | Certificate is renewed if within renewal threshold |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `path` | string | yes | - | Path to the certificate file |
| `key_path` | string | no | - | Path to the private key file |
| `domain` | string | yes | - | Primary domain name |
| `email` | string | no | - | Contact email for ACME account |
| `challenge` | string | no | http-01 | Challenge type: http-01, dns-01 |
| `staging` | bool | no | false | Use Let's Encrypt staging server |
| `renew_days` | int | no | 30 | Renew if certificate expires within days |
| `webroot` | string | no | - | Webroot path for HTTP-01 challenge |
| `dns_provider` | string | no | - | DNS provider for DNS-01 challenge |

### Challenge Types

| Challenge | Description | Requirements |
|-----------|-------------|--------------|
| `http-01` | HTTP file challenge | Web server with webroot access |
| `dns-01` | DNS TXT record challenge | DNS API access via provider |

### Implementation Note

The ACME module provides the framework for certificate management. For production use, integrate with external ACME tools like `certbot` or `lego` for the actual certificate issuance and renewal. The module handles:

- Certificate state tracking
- Renewal threshold monitoring
- File management (creation/removal)
- Metadata extraction from existing certificates

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | ✅ Full |
| macOS | ✅ Full |
| Windows | ✅ Full |

### Example

```yaml
obtain_cert_http:
  module: acme
  state: present
  path: /etc/letsencrypt/live/example.com/fullchain.pem
  key_path: /etc/letsencrypt/live/example.com/privkey.pem
  domain: example.com
  email: admin@example.com
  challenge: http-01
  webroot: /var/www/html

obtain_cert_dns:
  module: acme
  state: present
  path: /etc/ssl/certs/wildcard.example.com.crt
  key_path: /etc/ssl/private/wildcard.example.com.key
  domain: "*.example.com"
  email: admin@example.com
  challenge: dns-01
  dns_provider: cloudflare

renew_if_needed:
  module: acme
  state: renewed
  path: /etc/letsencrypt/live/example.com/fullchain.pem
  domain: example.com
  renew_days: 30

use_staging:
  module: acme
  state: present
  path: /etc/ssl/certs/test.crt
  domain: test.example.com
  staging: true

remove_cert:
  module: acme
  state: absent
  path: /etc/letsencrypt/live/old.example.com/fullchain.pem
  domain: old.example.com
```

### Metadata Returned

| Field | Type | Description |
|-------|------|-------------|
| `valid` | bool | Whether certificate is valid |
| `subject` | string | Certificate subject DN |
| `issuer` | string | Certificate issuer DN |
| `not_after` | string | Certificate expiration (RFC3339) |
| `dns_names` | []string | Subject Alternative Names |
| `needs_renewal` | bool | Whether certificate is within renewal threshold |

---

## logrotate Module {#logrotate-module}

Manage logrotate configuration files.

### States

| State | Description |
|-------|-------------|
| `present` | Logrotate configuration exists with specified settings |
| `absent` | Logrotate configuration is removed |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `name` | string | yes | - | Configuration name (creates /etc/logrotate.d/{name}) |
| `path` | string | yes* | - | Log file path pattern to rotate |
| `frequency` | string | no | weekly | Rotation frequency: daily, weekly, monthly, yearly |
| `rotate` | int | no | 4 | Number of rotated logs to keep |
| `compress` | bool | no | true | Compress rotated logs |
| `delaycompress` | bool | no | false | Delay compression until next rotation |
| `missingok` | bool | no | true | Don't error if log file is missing |
| `notifempty` | bool | no | true | Don't rotate empty log files |
| `create` | string | no | - | Create new log file (mode owner group) |
| `sharedscripts` | bool | no | false | Run scripts once for all files |
| `prerotate` | string | no | - | Script to run before rotation |
| `postrotate` | string | no | - | Script to run after rotation |

*`path` is required for `present` state.

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | ✅ Full |
| macOS | ❌ Not supported |
| Windows | ❌ Not supported |

### Example

```yaml
nginx_logrotate:
  module: logrotate
  state: present
  name: nginx
  path: /var/log/nginx/*.log
  frequency: daily
  rotate: 14
  compress: true
  delaycompress: true
  missingok: true
  notifempty: true
  create: "0640 nginx adm"
  sharedscripts: true
  postrotate: |
    [ -f /var/run/nginx.pid ] && kill -USR1 $(cat /var/run/nginx.pid)

app_logrotate:
  module: logrotate
  state: present
  name: myapp
  path: /var/log/myapp/*.log
  frequency: weekly
  rotate: 4
  compress: true
```

---

## sudoers Module {#sudoers-module}

Manage sudoers configuration files in /etc/sudoers.d/.

### States

| State | Description |
|-------|-------------|
| `present` | Sudoers configuration exists with specified rules |
| `absent` | Sudoers configuration is removed |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `name` | string | yes | - | Configuration name (creates /etc/sudoers.d/{name}) |
| `user` | string | no* | - | User to grant sudo access |
| `group` | string | no* | - | Group to grant sudo access (prefix with %) |
| `commands` | []string | no | ALL | Commands allowed (paths or ALL) |
| `nopasswd` | bool | no | false | Don't require password |
| `validate` | bool | no | true | Validate syntax before writing |

*Either `user` or `group` is required for `present` state.

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | ✅ Full |
| macOS | ✅ Full |
| Windows | ❌ Not supported |

### Example

```yaml
admin_sudo:
  module: sudoers
  state: present
  name: admins
  group: "%admin"
  commands:
    - ALL
  nopasswd: true

deploy_sudo:
  module: sudoers
  state: present
  name: deploy
  user: deploy
  commands:
    - /usr/bin/systemctl restart myapp
    - /usr/bin/systemctl reload nginx
  nopasswd: true
  validate: true

remove_old_config:
  module: sudoers
  state: absent
  name: old_admin
```

---

## limits Module {#limits-module}

Manage PAM limits configuration files in /etc/security/limits.d/.

### States

| State | Description |
|-------|-------------|
| `present` | Limits configuration exists with specified settings |
| `absent` | Limits configuration is removed |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `name` | string | yes | - | Configuration name (creates /etc/security/limits.d/{name}.conf) |
| `domain` | string | yes* | - | User, group (@group), or wildcard (*) |
| `type` | string | yes* | - | Limit type: soft, hard, or - (both) |
| `item` | string | yes* | - | Resource item to limit |
| `value` | string | yes* | - | Limit value |

*All parameters required for `present` state.

### Item Options

| Item | Description |
|------|-------------|
| `nofile` | Maximum open files |
| `nproc` | Maximum processes |
| `memlock` | Maximum locked memory (KB) |
| `core` | Core file size |
| `stack` | Maximum stack size (KB) |
| `fsize` | Maximum file size |
| `as` | Address space limit |

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | ✅ Full |
| macOS | ❌ Not supported |
| Windows | ❌ Not supported |

### Example

```yaml
elasticsearch_limits:
  module: limits
  state: present
  name: elasticsearch
  domain: elasticsearch
  type: "-"
  item: nofile
  value: "65536"

app_nproc:
  module: limits
  state: present
  name: myapp
  domain: appuser
  type: soft
  item: nproc
  value: "4096"

memlock_unlimited:
  module: limits
  state: present
  name: memlock
  domain: "*"
  type: hard
  item: memlock
  value: unlimited
```

---

## modprobe Module {#modprobe-module}

Manage kernel module configuration in /etc/modprobe.d/.

### States

| State | Description |
|-------|-------------|
| `present` | Module configuration exists |
| `absent` | Module configuration is removed |
| `blacklist` | Module is blacklisted |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `name` | string | yes | - | Kernel module name |
| `options` | string | no | - | Module options/parameters |
| `persist` | bool | no | true | Make configuration persistent |

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | ✅ Full |
| macOS | ❌ Not supported |
| Windows | ❌ Not supported |

### Example

```yaml
bonding_options:
  module: modprobe
  state: present
  name: bonding
  options: "mode=4 miimon=100"

blacklist_nouveau:
  module: modprobe
  state: blacklist
  name: nouveau

disable_ipv6:
  module: modprobe
  state: present
  name: ipv6
  options: "disable=1"
```

---

## syslog Module {#syslog-module}

Manage syslog/rsyslog configuration files.

### States

| State | Description |
|-------|-------------|
| `present` | Syslog configuration exists |
| `absent` | Syslog configuration is removed |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `name` | string | yes | - | Configuration name (creates /etc/rsyslog.d/{name}.conf) |
| `facility` | string | yes* | - | Syslog facility (auth, daemon, local0-7, etc.) |
| `priority` | string | no | * | Minimum priority (debug, info, notice, warning, err, crit, alert, emerg) |
| `action` | string | yes* | - | Destination (file path, @remote, @@remote) |
| `syslog_type` | string | no | rsyslog | Syslog daemon: rsyslog, syslog-ng |

*`facility` and `action` are required for `present` state.

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | ✅ Full |
| macOS | ⚠️ Limited (different config format) |
| Windows | ❌ Not supported |

### Example

```yaml
auth_logging:
  module: syslog
  state: present
  name: auth
  facility: auth
  priority: info
  action: /var/log/auth.log

remote_syslog:
  module: syslog
  state: present
  name: remote
  facility: "*"
  priority: warning
  action: "@@syslog.example.com:514"

local_app:
  module: syslog
  state: present
  name: myapp
  facility: local0
  priority: "*"
  action: /var/log/myapp.log
```

---

## lineinfile Module {#lineinfile-module}

Manage lines in text files.

### States

| State | Description |
|-------|-------------|
| `present` | Line exists in the file |
| `absent` | Line is removed from file |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `path` | string | yes | - | Path to the file to modify |
| `line` | string | yes* | - | Line to insert or match |
| `regexp` | string | no | - | Regex pattern to match existing lines |
| `insertafter` | string | no | EOF | Insert after this pattern (regex or EOF, BOF) |
| `insertbefore` | string | no | - | Insert before this pattern (regex or BOF) |
| `create` | bool | no | false | Create file if it doesn't exist |
| `backup` | bool | no | false | Create backup before modifying |

*`line` is required for `present` state.

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | ✅ Full |
| macOS | ✅ Full |
| Windows | ✅ Full (uses native line endings) |

### Example

```yaml
ensure_hosts_entry:
  module: lineinfile
  state: present
  path: /etc/hosts
  line: "192.168.1.100 myserver.local"
  regexp: "^192\\.168\\.1\\.100"

disable_ssh_root:
  module: lineinfile
  state: present
  path: /etc/ssh/sshd_config
  line: "PermitRootLogin no"
  regexp: "^#?PermitRootLogin"
  backup: true

add_to_bashrc:
  module: lineinfile
  state: present
  path: /home/user/.bashrc
  line: 'export PATH="$HOME/bin:$PATH"'
  insertafter: "^# User specific"
  create: true

remove_old_entry:
  module: lineinfile
  state: absent
  path: /etc/hosts
  regexp: "^192\\.168\\.1\\.50"
```

---

## ini_file Module {#ini_file-module}

Manage settings in INI-format configuration files.

### States

| State | Description |
|-------|-------------|
| `present` | Setting exists with specified value |
| `absent` | Setting is removed |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `path` | string | yes | - | Path to the INI file |
| `section` | string | yes | - | Section name (without brackets) |
| `option` | string | yes | - | Option/key name |
| `value` | string | yes* | - | Value for the option |
| `create` | bool | no | true | Create file if it doesn't exist |
| `backup` | bool | no | false | Create backup before modifying |

*`value` is required for `present` state.

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | ✅ Full |
| macOS | ✅ Full |
| Windows | ✅ Full |

### Example

```yaml
mysql_settings:
  module: ini_file
  state: present
  path: /etc/mysql/mysql.conf.d/custom.cnf
  section: mysqld
  option: max_connections
  value: "500"
  backup: true

php_memory:
  module: ini_file
  state: present
  path: /etc/php/8.1/fpm/php.ini
  section: PHP
  option: memory_limit
  value: "256M"

git_config:
  module: ini_file
  state: present
  path: /home/user/.gitconfig
  section: user
  option: email
  value: "user@example.com"
  create: true

remove_setting:
  module: ini_file
  state: absent
  path: /etc/myapp/config.ini
  section: deprecated
  option: old_feature
```

---

## archive Module {#archive-module}

Extract archive files (tar, tar.gz, tar.bz2, tar.xz, zip).

### States

| State | Description |
|-------|-------------|
| `present` | Archive is extracted to destination |
| `absent` | Extracted files are removed |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `src` | string | yes | - | Path to the archive file |
| `dest` | string | yes | - | Destination directory for extraction |
| `format` | string | no | auto | Archive format: tar, tar.gz, tar.bz2, tar.xz, zip, auto |
| `creates` | string | no | - | Path to check for idempotency (skip if exists) |

### Supported Formats

| Format | Extensions |
|--------|------------|
| `tar` | .tar |
| `tar.gz` | .tar.gz, .tgz |
| `tar.bz2` | .tar.bz2, .tbz, .tbz2 |
| `tar.xz` | .tar.xz, .txz |
| `zip` | .zip |
| `auto` | Detect from extension |

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | ✅ Full |
| macOS | ✅ Full |
| Windows | ✅ Full (requires tar/unzip) |

### Example

```yaml
extract_app:
  module: archive
  state: present
  src: /tmp/app-v1.2.3.tar.gz
  dest: /opt/app
  creates: /opt/app/bin/app

extract_assets:
  module: archive
  state: present
  src: /tmp/assets.zip
  dest: /var/www/html/assets
  format: zip

extract_data:
  module: archive
  state: present
  src: /backups/data.tar.xz
  dest: /var/lib/myapp
  format: tar.xz
```

---

## win_feature Module {#win_feature-module}

Manage Windows Server features and optional features.

### States

| State | Description |
|-------|-------------|
| `installed` | Feature is installed |
| `removed` | Feature is removed |
| `enabled` | Optional feature is enabled |
| `disabled` | Optional feature is disabled |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `name` | string | yes | - | Feature name or display name |
| `include_management_tools` | bool | no | false | Include associated management tools |
| `include_sub_features` | bool | no | false | Include all sub-features |
| `source` | string | no | - | Source path for feature files |
| `restart` | bool | no | false | Restart if required |

### Common Features

| Feature Name | Description |
|--------------|-------------|
| `Web-Server` | IIS Web Server |
| `NET-Framework-45-Core` | .NET Framework 4.5 |
| `Hyper-V` | Hyper-V Virtualization |
| `DNS` | DNS Server |
| `DHCP` | DHCP Server |
| `AD-Domain-Services` | Active Directory Domain Services |
| `Containers` | Windows Containers |

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | ❌ Not supported |
| macOS | ❌ Not supported |
| Windows | ✅ Full (Server 2016+) |

### Example

```yaml
install_iis:
  module: win_feature
  state: installed
  name: Web-Server
  include_management_tools: true
  include_sub_features: true

install_dotnet:
  module: win_feature
  state: installed
  name: NET-Framework-45-Core

enable_containers:
  module: win_feature
  state: enabled
  name: Containers
  restart: true

remove_telnet:
  module: win_feature
  state: removed
  name: Telnet-Client
```

---

## win_registry Module {#win_registry-module}

Manage Windows registry keys and values.

### States

| State | Description |
|-------|-------------|
| `present` | Registry value exists with specified data |
| `absent` | Registry value is removed |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `path` | string | yes | - | Registry key path (e.g., HKLM:\SOFTWARE\MyApp) |
| `name` | string | yes* | - | Value name (use "(default)" for default value) |
| `data` | any | yes* | - | Value data |
| `type` | string | no | String | Value type |

*`name` and `data` are required for `present` state.

### Value Types

| Type | Description |
|------|-------------|
| `String` | REG_SZ - String value |
| `ExpandString` | REG_EXPAND_SZ - Expandable string |
| `Binary` | REG_BINARY - Binary data |
| `DWord` | REG_DWORD - 32-bit integer |
| `QWord` | REG_QWORD - 64-bit integer |
| `MultiString` | REG_MULTI_SZ - Array of strings |

### Registry Roots

| Abbreviation | Full Path |
|--------------|-----------|
| `HKLM:` | HKEY_LOCAL_MACHINE |
| `HKCU:` | HKEY_CURRENT_USER |
| `HKCR:` | HKEY_CLASSES_ROOT |
| `HKU:` | HKEY_USERS |
| `HKCC:` | HKEY_CURRENT_CONFIG |

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | ❌ Not supported |
| macOS | ❌ Not supported |
| Windows | ✅ Full |

### Example

```yaml
app_setting:
  module: win_registry
  state: present
  path: HKLM:\SOFTWARE\MyApp
  name: InstallPath
  data: C:\Program Files\MyApp
  type: String

disable_feature:
  module: win_registry
  state: present
  path: HKLM:\SOFTWARE\Policies\Microsoft\Windows
  name: DisableFeature
  data: 1
  type: DWord

set_environment:
  module: win_registry
  state: present
  path: HKLM:\SYSTEM\CurrentControlSet\Control\Session Manager\Environment
  name: MYAPP_HOME
  data: C:\MyApp
  type: ExpandString

remove_old_value:
  module: win_registry
  state: absent
  path: HKLM:\SOFTWARE\MyApp
  name: OldSetting
```

---

## win_service Module {#win_service-module}

Manage Windows services.

### States

| State | Description |
|-------|-------------|
| `running` | Service is started |
| `stopped` | Service is stopped |
| `enabled` | Service starts automatically |
| `disabled` | Service is disabled |
| `present` | Service exists with specified configuration |
| `absent` | Service is removed |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `name` | string | yes | - | Service name (short name, not display name) |
| `display_name` | string | no | - | Service display name |
| `description` | string | no | - | Service description |
| `path` | string | no* | - | Path to service executable |
| `start_mode` | string | no | auto | Start mode: auto, manual, disabled, delayed |
| `username` | string | no | LocalSystem | Account to run service as |
| `password` | string | no | - | Password for service account |
| `dependencies` | []string | no | - | Services this service depends on |

*`path` is required for `present` state when creating a new service.

### Start Modes

| Mode | Description |
|------|-------------|
| `auto` | Automatic startup |
| `delayed` | Automatic (Delayed Start) |
| `manual` | Manual startup |
| `disabled` | Service is disabled |

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | ❌ Not supported |
| macOS | ❌ Not supported |
| Windows | ✅ Full |

### Example

```yaml
start_service:
  module: win_service
  state: running
  name: MyAppService

stop_service:
  module: win_service
  state: stopped
  name: MyAppService

configure_service:
  module: win_service
  state: present
  name: MyAppService
  display_name: My Application Service
  description: Runs the My Application background tasks
  path: C:\MyApp\myapp-service.exe
  start_mode: auto
  username: .\MyAppUser
  password: "{{ .vars.service_password }}"

disable_telemetry:
  module: win_service
  state: disabled
  name: DiagTrack

set_delayed_start:
  module: win_service
  state: enabled
  name: MyAppService
  start_mode: delayed

remove_service:
  module: win_service
  state: absent
  name: OldService
```

---

## win_firewall Module {#win_firewall-module}

Manage Windows Firewall rules using PowerShell.

### States

| State | Description |
|-------|-------------|
| `present` | Firewall rule exists with specified configuration |
| `absent` | Firewall rule is removed |
| `enabled` | Firewall rule is enabled |
| `disabled` | Firewall rule is disabled |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `name` | string | yes | - | Firewall rule name (internal identifier) |
| `display_name` | string | no | name | Human-readable rule name |
| `description` | string | no | - | Rule description |
| `direction` | string | no | Inbound | Traffic direction: Inbound, Outbound |
| `action` | string | no | Allow | Rule action: Allow, Block |
| `protocol` | string | no | - | Protocol: TCP, UDP, ICMPv4, ICMPv6, Any |
| `local_port` | string | no | - | Local port(s): single port, range (80-443), or comma-separated |
| `remote_port` | string | no | - | Remote port(s) |
| `program` | string | no | - | Path to program to match |
| `profile` | string | no | Any | Profile: Domain, Private, Public, Any |

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | ❌ Not supported |
| macOS | ❌ Not supported |
| Windows | ✅ Full |

### Example

```yaml
allow_http:
  module: win_firewall
  state: present
  name: Allow-HTTP
  display_name: Allow HTTP Traffic
  description: Allow inbound HTTP traffic on port 80
  direction: Inbound
  action: Allow
  protocol: TCP
  local_port: "80"

allow_https:
  module: win_firewall
  state: present
  name: Allow-HTTPS
  display_name: Allow HTTPS Traffic
  direction: Inbound
  action: Allow
  protocol: TCP
  local_port: "443"

allow_app:
  module: win_firewall
  state: enabled
  name: MyApp-Firewall
  display_name: MyApp Network Access
  program: C:\MyApp\myapp.exe
  direction: Outbound
  action: Allow

block_telemetry:
  module: win_firewall
  state: present
  name: Block-Telemetry
  direction: Outbound
  action: Block
  protocol: TCP
  remote_port: "443"
  program: C:\Windows\System32\CompatTelRunner.exe

disable_rule:
  module: win_firewall
  state: disabled
  name: Allow-HTTP

remove_rule:
  module: win_firewall
  state: absent
  name: Old-Firewall-Rule
```

---

## win_package Module {#win_package-module}

Manage Windows packages using Chocolatey, winget, MSI, or EXE installers.

### States

| State | Description |
|-------|-------------|
| `installed` | Package is installed (specific version if specified) |
| `removed` | Package is uninstalled |
| `latest` | Package is installed and updated to latest version |

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `name` | string | yes | - | Package name or identifier |
| `source` | string | no | auto | Package source: chocolatey, winget, msi, exe, auto |
| `version` | string | no | - | Specific version to install |
| `installer` | string | no* | - | Path to MSI/EXE installer file |
| `force` | bool | no | false | Force reinstall even if present |
| `allow_downgrade` | bool | no | false | Allow downgrade to older version (Chocolatey) |
| `install_args` | string | no | - | Additional installer arguments |
| `package_params` | string | no | - | Package-specific parameters (Chocolatey) |
| `remove_dependencies` | bool | no | false | Remove dependencies on uninstall (Chocolatey) |
| `uninstall_args` | string | no | - | Additional uninstaller arguments |
| `log_file` | string | no | - | Log file path for MSI installations |

*`installer` is required when `source` is `msi` or `exe`.

### Package Sources

| Source | Description |
|--------|-------------|
| `auto` | Auto-detect (tries Chocolatey, then winget) |
| `chocolatey` | Chocolatey package manager |
| `winget` | Windows Package Manager (winget) |
| `msi` | Windows Installer (MSI) file |
| `exe` | Executable installer |

### Platform Support

| Platform | Support |
|----------|---------|
| Linux | ❌ Not supported |
| macOS | ❌ Not supported |
| Windows | ✅ Full |

### Example

```yaml
install_7zip:
  module: win_package
  state: installed
  name: 7zip
  source: chocolatey

install_vscode:
  module: win_package
  state: installed
  name: Microsoft.VisualStudioCode
  source: winget

install_specific_version:
  module: win_package
  state: installed
  name: nodejs
  source: chocolatey
  version: "18.17.0"

install_msi:
  module: win_package
  state: installed
  name: MyApplication
  source: msi
  installer: C:\Installers\myapp.msi
  install_args: INSTALLDIR=C:\MyApp ADDLOCAL=ALL
  log_file: C:\Logs\myapp-install.log

install_exe:
  module: win_package
  state: installed
  name: MyTool
  source: exe
  installer: C:\Installers\mytool-setup.exe
  install_args: /S /D=C:\MyTool

update_to_latest:
  module: win_package
  state: latest
  name: git
  source: chocolatey

remove_package:
  module: win_package
  state: removed
  name: old-software
  source: chocolatey
  remove_dependencies: true
```

---

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

### SSH Modules (authorized_keys, known_hosts, sshd_config)

- ✅ Linux, macOS
- ❌ Windows (not supported)

### SELinux Modules (selinux, selinux_boolean)

- ✅ Linux (RHEL/CentOS/Fedora)
- ⚠️ Linux (Ubuntu/Debian) - via selinux packages
- ❌ macOS, Windows (not supported)

### AppArmor Modules (apparmor, apparmor_profile)

- ✅ Linux (Ubuntu/Debian, openSUSE)
- ⚠️ Linux (RHEL/CentOS) - not default
- ❌ macOS, Windows (not supported)

### Config File Modules (logrotate, sudoers, limits, modprobe, syslog)

- ✅ Linux
- ❌ macOS (except sudoers)
- ❌ Windows (not supported)

### Text File Modules (lineinfile, ini_file)

- ✅ Linux, Windows, macOS
- Line endings handled per platform

### Archive Module

- ✅ Linux, macOS
- ✅ Windows (requires tar/unzip)

### Windows Modules (win_feature, win_registry, win_service)

- ❌ Linux (not supported)
- ❌ macOS (not supported)
- ✅ Windows (Server 2016+ for features)

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

Use the `kscorectl module` CLI to scaffold and manage custom modules:

```bash
# Initialize a new Starlark module
kscorectl module init myorg/custom-state

# Initialize a WASM module
kscorectl module init --type wasm myorg/custom-provider
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
kscorectl module validate ./myorg/custom-state

# Run module tests
kscorectl module test ./myorg/custom-state

# Resolve dependencies
kscorectl module resolve ./myorg/custom-state

# Build distributable package
kscorectl module build ./myorg/custom-state

# Verify module integrity
kscorectl module verify ./myorg/custom-state/myorg-custom-state-0.1.0.zip
```

### Capability System

Modules can only access explicitly granted capabilities. All 10 capability types are fully implemented with security-constrained access:

| Capability | Description | Scope | Implementation |
|------------|-------------|-------|----------------|
| `fs.read` | Read files | Path patterns | `FSReadCapability` - symlink protection, size limits |
| `fs.write` | Write files | Path patterns | `FSWriteCapability` - dangerous path blocking, size limits |
| `http.get` | HTTP GET requests | Domain patterns | `HTTPGetCapability` - domain validation, rate limiting |
| `http.post` | HTTP POST requests | Domain patterns | `HTTPPostCapability` - request/response size limits |
| `exec` | Execute commands | Command allowlist | `ExecCapability` - command allowlisting, timeout |
| `secrets.read` | Read secrets | Path patterns | `SecretsReadCapability` - pluggable backend, audit |
| `secrets.write` | Write secrets | Path patterns | `SecretsWriteCapability` - pluggable backend, audit |
| `log` | Structured logging | Rate limited | `LogCapability` - rate limiting, context enrichment |
| `kv` | Key-value storage | Namespace scoped | `KVCapability` - namespace isolation, pluggable backend |
| `time` | Current time | Breaks determinism | `TimeCapability` - rarely granted, breaks reproducibility |

**Security Features**:

- **Path validation**: Filesystem capabilities block dangerous patterns (`/`, `/etc`, `/proc`, symlink attacks)
- **Domain validation**: HTTP capabilities block wildcards (`*`, `*.com`) and internal networks
- **Rate limiting**: HTTP and logging capabilities enforce request limits
- **Size limits**: Configurable max file size (default 10MB), max response size
- **Pluggable backends**: Secrets and KV capabilities support custom storage backends

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
kscorectl module sign ./myorg/custom-state/myorg-custom-state-0.1.0.zip

# Publish to registry
kscorectl module publish ./myorg/custom-state/myorg-custom-state-0.1.0.zip
```

## Custom Module Development Guide

This section provides comprehensive templates and guidance for developing custom state modules.

### Module Architecture Overview

```
mymodule/
├── module.yaml           # Module manifest (required)
├── states/
│   └── main.star        # Main Starlark state logic (required)
├── functions/
│   └── helpers.star     # Shared helper functions (optional)
├── templates/
│   └── config.tmpl      # Configuration templates (optional)
├── tests/
│   └── main_test.star   # Unit tests (recommended)
├── docs/
│   └── README.md        # Module documentation (recommended)
└── examples/
    └── basic.yaml       # Usage examples (recommended)
```

### Module Manifest Template

```yaml
# module.yaml - Module manifest
name: myorg/my-custom-module
version: 1.0.0
description: Custom module for managing application deployments

# Module author information
author:
  name: My Organization
  email: modules@myorg.com
  url: https://github.com/myorg/my-custom-module

# Minimum Keystone Core version
min_version: "0.1.0"

# Module dependencies
dependencies:
  - name: kscore/file
    version: ">=1.0.0"
  - name: kscore/service
    version: ">=1.0.0"

# Capabilities required by this module
capabilities:
  - type: fs.read
    paths:
      - "/etc/myapp/*"
      - "/var/lib/myapp/*"
  - type: fs.write
    paths:
      - "/etc/myapp/*"
      - "/var/lib/myapp/*"
  - type: exec
    commands:
      - "/usr/bin/systemctl"
      - "/usr/bin/myapp"
  - type: http.get
    domains:
      - "api.myorg.com"
  - type: log
    rate_limit: 100/s

# Module states (entry points)
states:
  - name: config
    description: Manage application configuration
    entry: states/main.star:ensure_config
  - name: deploy
    description: Deploy application
    entry: states/main.star:deploy_app

# Module parameters schema
parameters:
  config:
    - name: path
      type: string
      required: true
      description: Path to configuration file
    - name: template
      type: string
      required: false
      default: "default.conf"
      description: Template to use
    - name: vars
      type: map
      required: false
      description: Template variables
  deploy:
    - name: version
      type: string
      required: true
      description: Application version to deploy
    - name: rollback_on_failure
      type: bool
      required: false
      default: true
      description: Automatically rollback on deployment failure

# Metadata
license: Apache-2.0
keywords:
  - application
  - deployment
  - configuration
repository: https://github.com/myorg/my-custom-module
```

### State Implementation Templates

#### Basic State Template

```python
# states/main.star
"""
Custom state module for managing application configuration.

States:
  - config: Ensure application configuration exists
  - deploy: Deploy application version
"""

# Helper function for consistent result formatting
def _result(status, comment, changes=None, metadata=None):
    """Create a standardized result dictionary."""
    result = {
        "result": status,
        "comment": comment,
    }
    if changes:
        result["changes"] = changes
    if metadata:
        result["metadata"] = metadata
    return result

def ensure_config(name, path, template="default.conf", vars={}):
    """
    Ensure application configuration exists with correct content.

    Args:
        name: State identifier (from state ID)
        path: Target configuration file path
        template: Template file name
        vars: Template variables

    Returns:
        result: changed, unchanged, or failed
        comment: Human-readable description
        changes: Dictionary of changes made
    """
    # Load and render template
    template_path = "templates/" + template
    if not fs.exists(template_path):
        return _result("failed", "Template not found: " + template_path)

    template_content = fs.read(template_path)
    rendered = render_template(template_content, vars)

    # Check current state
    if fs.exists(path):
        current = fs.read(path)
        if current == rendered:
            return _result(
                "unchanged",
                "Configuration already in desired state",
                metadata={"path": path, "checksum": fs.checksum(path)}
            )

    # Backup existing file
    if fs.exists(path):
        backup_path = path + ".bak"
        fs.copy(path, backup_path)

    # Write new configuration
    fs.write(path, rendered, mode="0644")

    return _result(
        "changed",
        "Configuration updated",
        changes={
            "path": path,
            "action": "updated" if fs.exists(path + ".bak") else "created"
        }
    )

def deploy_app(name, version, rollback_on_failure=True):
    """
    Deploy application to specified version.

    Args:
        name: State identifier
        version: Target version
        rollback_on_failure: Whether to rollback on failure

    Returns:
        Standard result dictionary
    """
    # Get current version
    result = exec.run(["/usr/bin/myapp", "--version"])
    current_version = result.stdout.strip()

    if current_version == version:
        return _result(
            "unchanged",
            "Application already at version " + version,
            metadata={"version": version}
        )

    # Stop service
    exec.run(["systemctl", "stop", "myapp"])

    # Download and install new version
    try:
        url = "https://releases.myorg.com/myapp/" + version + "/myapp"
        binary = http.get(url)
        fs.write("/usr/bin/myapp", binary, mode="0755")

        # Start service
        exec.run(["systemctl", "start", "myapp"])

        # Verify deployment
        result = exec.run(["/usr/bin/myapp", "--version"])
        if result.stdout.strip() != version:
            raise Exception("Version mismatch after deployment")

        return _result(
            "changed",
            "Deployed version " + version,
            changes={
                "previous_version": current_version,
                "new_version": version
            }
        )
    except Exception as e:
        if rollback_on_failure:
            # Attempt rollback
            exec.run(["systemctl", "start", "myapp"])
            return _result(
                "failed",
                "Deployment failed, rolled back: " + str(e),
                changes={"rollback": True}
            )
        return _result("failed", "Deployment failed: " + str(e))

def render_template(template, vars):
    """Simple template rendering with variable substitution."""
    result = template
    for key, value in vars.items():
        result = result.replace("{{ " + key + " }}", str(value))
    return result
```

#### Advanced State Template with Drift Detection

```python
# states/drift_aware.star
"""
State module with built-in drift detection.
"""

def ensure_service_config(name, service, config_path, expected_config):
    """
    Manage service configuration with drift detection.
    """
    # Define expected state
    expected = {
        "path": config_path,
        "content": expected_config,
        "mode": "0644",
        "owner": "root",
        "group": "root",
    }

    # Get current state
    current = _get_current_state(config_path)

    # Calculate drift
    drift = _calculate_drift(expected, current)

    if not drift:
        return {
            "result": "unchanged",
            "comment": "Service configuration matches expected state",
            "drift": {"detected": False},
        }

    # Apply changes
    if drift.get("content"):
        fs.write(config_path, expected_config)

    if drift.get("mode"):
        fs.chmod(config_path, expected["mode"])

    if drift.get("owner") or drift.get("group"):
        fs.chown(config_path, expected["owner"], expected["group"])

    # Reload service if config changed
    if drift.get("content"):
        exec.run(["systemctl", "reload", service])

    return {
        "result": "changed",
        "comment": "Configuration drift corrected",
        "drift": {
            "detected": True,
            "severity": _drift_severity(drift),
            "fields": list(drift.keys()),
        },
        "changes": drift,
    }

def _get_current_state(path):
    """Get current state of a file."""
    if not fs.exists(path):
        return None

    stat = fs.stat(path)
    return {
        "path": path,
        "content": fs.read(path),
        "mode": stat.mode,
        "owner": stat.owner,
        "group": stat.group,
    }

def _calculate_drift(expected, current):
    """Calculate differences between expected and current state."""
    if current is None:
        return {"all": "file does not exist"}

    drift = {}
    for key in expected:
        if expected[key] != current.get(key):
            drift[key] = {
                "expected": expected[key],
                "current": current.get(key),
            }
    return drift

def _drift_severity(drift):
    """Calculate drift severity level."""
    if "content" in drift:
        return "high"
    if "mode" in drift or "owner" in drift:
        return "medium"
    return "low"
```

#### Idempotent Operations Template

```python
# states/idempotent.star
"""
Template demonstrating idempotent state operations.
"""

def ensure_directory(name, path, mode="0755", owner="root", group="root"):
    """
    Idempotent directory creation.

    This function can be safely called multiple times with the same
    parameters and will only make changes when necessary.
    """
    changes = []

    # Check if directory exists
    if not fs.exists(path):
        fs.mkdir(path, parents=True)
        changes.append("created")
    elif not fs.is_dir(path):
        return {
            "result": "failed",
            "comment": "Path exists but is not a directory: " + path,
        }

    # Check and fix permissions
    stat = fs.stat(path)

    if stat.mode != mode:
        fs.chmod(path, mode)
        changes.append("mode changed from " + stat.mode + " to " + mode)

    if stat.owner != owner or stat.group != group:
        fs.chown(path, owner, group)
        changes.append("ownership changed")

    if not changes:
        return {
            "result": "unchanged",
            "comment": "Directory already in desired state",
        }

    return {
        "result": "changed",
        "comment": "Directory configured",
        "changes": {"actions": changes},
    }

def ensure_absent(name, path, recursive=False):
    """
    Idempotent path removal.

    Safely removes files or directories, with optional recursive deletion.
    """
    if not fs.exists(path):
        return {
            "result": "unchanged",
            "comment": "Path does not exist",
        }

    if fs.is_dir(path):
        if recursive:
            fs.rmtree(path)
        else:
            # Check if directory is empty
            if fs.listdir(path):
                return {
                    "result": "failed",
                    "comment": "Directory not empty, use recursive=true",
                }
            fs.rmdir(path)
    else:
        fs.unlink(path)

    return {
        "result": "changed",
        "comment": "Path removed",
        "changes": {"removed": path},
    }
```

### Helper Functions Library

```python
# functions/helpers.star
"""
Reusable helper functions for custom modules.
"""

def require_root():
    """Check if running as root, fail if not."""
    result = exec.run(["id", "-u"])
    if result.stdout.strip() != "0":
        fail("This operation requires root privileges")

def backup_file(path, suffix=".bak"):
    """Create a backup of a file before modification."""
    if fs.exists(path):
        backup_path = path + suffix
        fs.copy(path, backup_path)
        return backup_path
    return None

def restore_backup(path, suffix=".bak"):
    """Restore a file from backup."""
    backup_path = path + suffix
    if fs.exists(backup_path):
        fs.copy(backup_path, path)
        return True
    return False

def wait_for_service(name, timeout=30, check_interval=1):
    """Wait for a service to become healthy."""
    elapsed = 0
    while elapsed < timeout:
        result = exec.run(["systemctl", "is-active", name], check=False)
        if result.return_code == 0:
            return True
        time.sleep(check_interval)
        elapsed += check_interval
    return False

def parse_version(version_string):
    """Parse a version string into components."""
    parts = version_string.split(".")
    return {
        "major": int(parts[0]) if len(parts) > 0 else 0,
        "minor": int(parts[1]) if len(parts) > 1 else 0,
        "patch": int(parts[2]) if len(parts) > 2 else 0,
        "raw": version_string,
    }

def compare_versions(v1, v2):
    """Compare two version strings. Returns -1, 0, or 1."""
    p1 = parse_version(v1)
    p2 = parse_version(v2)

    for field in ["major", "minor", "patch"]:
        if p1[field] < p2[field]:
            return -1
        if p1[field] > p2[field]:
            return 1
    return 0

def validate_required_params(params, required):
    """Validate that required parameters are present."""
    missing = []
    for name in required:
        if name not in params or params[name] is None:
            missing.append(name)
    if missing:
        fail("Missing required parameters: " + ", ".join(missing))

def sanitize_path(path):
    """Sanitize a file path to prevent directory traversal."""
    # Remove any .. components
    parts = path.split("/")
    clean_parts = []
    for part in parts:
        if part == "..":
            continue
        if part == ".":
            continue
        clean_parts.append(part)
    return "/".join(clean_parts)
```

### Test Templates

#### Unit Tests

```python
# tests/main_test.star
"""Unit tests for custom module."""

load("//states/main.star", "ensure_config", "deploy_app")
load("//functions/helpers.star", "parse_version", "compare_versions")

# Test fixtures
def setup():
    """Set up test fixtures before each test."""
    # Create test directory
    fs.mkdir("/tmp/test-module", parents=True)
    # Create test template
    fs.write("/tmp/test-module/templates/default.conf", "key={{ value }}")

def teardown():
    """Clean up after each test."""
    if fs.exists("/tmp/test-module"):
        fs.rmtree("/tmp/test-module")

# Configuration tests
def test_ensure_config_creates_new_file():
    """Test that ensure_config creates a new config file."""
    setup()
    result = ensure_config(
        name="test",
        path="/tmp/test-module/config.conf",
        template="default.conf",
        vars={"value": "test-value"}
    )
    assert.eq(result["result"], "changed")
    assert.true(fs.exists("/tmp/test-module/config.conf"))
    content = fs.read("/tmp/test-module/config.conf")
    assert.eq(content, "key=test-value")
    teardown()

def test_ensure_config_idempotent():
    """Test that ensure_config is idempotent."""
    setup()
    # First run
    ensure_config(
        name="test",
        path="/tmp/test-module/config.conf",
        template="default.conf",
        vars={"value": "test-value"}
    )
    # Second run should be unchanged
    result = ensure_config(
        name="test",
        path="/tmp/test-module/config.conf",
        template="default.conf",
        vars={"value": "test-value"}
    )
    assert.eq(result["result"], "unchanged")
    teardown()

def test_ensure_config_detects_changes():
    """Test that ensure_config detects and applies changes."""
    setup()
    # Create initial config
    ensure_config(
        name="test",
        path="/tmp/test-module/config.conf",
        template="default.conf",
        vars={"value": "initial"}
    )
    # Update with new value
    result = ensure_config(
        name="test",
        path="/tmp/test-module/config.conf",
        template="default.conf",
        vars={"value": "updated"}
    )
    assert.eq(result["result"], "changed")
    content = fs.read("/tmp/test-module/config.conf")
    assert.eq(content, "key=updated")
    teardown()

def test_ensure_config_missing_template():
    """Test error handling for missing template."""
    result = ensure_config(
        name="test",
        path="/tmp/test.conf",
        template="nonexistent.conf",
        vars={}
    )
    assert.eq(result["result"], "failed")
    assert.contains(result["comment"], "not found")

# Helper function tests
def test_parse_version():
    """Test version parsing."""
    v = parse_version("1.2.3")
    assert.eq(v["major"], 1)
    assert.eq(v["minor"], 2)
    assert.eq(v["patch"], 3)

def test_compare_versions():
    """Test version comparison."""
    assert.eq(compare_versions("1.0.0", "1.0.0"), 0)
    assert.eq(compare_versions("1.0.0", "2.0.0"), -1)
    assert.eq(compare_versions("2.0.0", "1.0.0"), 1)
    assert.eq(compare_versions("1.1.0", "1.0.0"), 1)
    assert.eq(compare_versions("1.0.1", "1.0.0"), 1)
```

#### Integration Tests

```python
# tests/integration_test.star
"""Integration tests requiring real system resources."""

load("//states/main.star", "ensure_config", "deploy_app")

# Mark as integration test (requires --integration flag to run)
_test_type = "integration"

def test_full_deployment_workflow():
    """Test complete deployment workflow."""
    # This test requires a real system with systemd

    # Step 1: Create configuration
    config_result = ensure_config(
        name="app-config",
        path="/etc/myapp/config.conf",
        template="production.conf",
        vars={
            "db_host": "localhost",
            "db_port": "5432",
        }
    )
    assert.true(config_result["result"] in ["changed", "unchanged"])

    # Step 2: Deploy application
    deploy_result = deploy_app(
        name="app-deploy",
        version="1.0.0",
        rollback_on_failure=True
    )
    assert.true(deploy_result["result"] in ["changed", "unchanged"])

    # Step 3: Verify service is running
    result = exec.run(["systemctl", "is-active", "myapp"])
    assert.eq(result.stdout.strip(), "active")
```

### Example Usage Files

```yaml
# examples/basic.yaml
# Basic usage example for my-custom-module

# Ensure application configuration
app_config:
  module: myorg/my-custom-module
  state: config
  path: /etc/myapp/config.conf
  template: production.conf
  vars:
    db_host: "{{ .vars.database_host }}"
    db_port: 5432
    log_level: info

# Deploy application
app_deploy:
  module: myorg/my-custom-module
  state: deploy
  version: "{{ .vars.app_version }}"
  rollback_on_failure: true
  require:
    - app_config
```

```yaml
# examples/advanced.yaml
# Advanced usage with multiple configurations

# Create configuration directory
config_dir:
  module: file
  state: directory
  path: /etc/myapp
  mode: "0755"

# Main application config
main_config:
  module: myorg/my-custom-module
  state: config
  path: /etc/myapp/main.conf
  vars:
    environment: production
    workers: "{{ .facts.cpu_count }}"
  require:
    - config_dir

# Logging configuration
logging_config:
  module: myorg/my-custom-module
  state: config
  path: /etc/myapp/logging.conf
  template: logging.conf
  vars:
    log_path: /var/log/myapp
    log_level: "{{ default 'info' .vars.log_level }}"
  require:
    - config_dir

# Deploy with watching config changes
app_deploy:
  module: myorg/my-custom-module
  state: deploy
  version: "{{ .vars.app_version }}"
  watch:
    - main_config
    - logging_config
```

### Publishing Your Module

#### Build and Sign

```bash
# Validate module before building
kscorectl module validate ./myorg-my-custom-module

# Run all tests
kscorectl module test ./myorg-my-custom-module

# Build distributable package
kscorectl module build ./myorg-my-custom-module

# Sign with your key
kscorectl module sign \
  --key ~/.keystone-core/signing-key.pem \
  ./myorg-my-custom-module/myorg-my-custom-module-1.0.0.zip
```

#### Publish to Registry

```bash
# Publish to OCI registry
kscorectl module publish \
  --registry ghcr.io/myorg \
  ./myorg-my-custom-module/myorg-my-custom-module-1.0.0.zip

# Or publish to HTTP server
scp ./myorg-my-custom-module/myorg-my-custom-module-1.0.0.zip \
  user@modules.myorg.com:/var/www/modules/
```

#### Module Registry Entry

```yaml
# registry entry (for private registries)
name: myorg/my-custom-module
versions:
  - version: 1.0.0
    released: 2025-01-15
    checksum: sha256:abc123...
    signatures:
      - keyid: ABCD1234
        sig: base64...
    sources:
      - type: oci
        url: ghcr.io/myorg/my-custom-module:1.0.0
      - type: http
        url: https://modules.myorg.com/my-custom-module-1.0.0.zip
```

## Real-World Examples

This section provides comprehensive, production-ready examples demonstrating how to combine multiple modules for common infrastructure scenarios.

### Example 1: NGINX Web Server Stack

Deploy a complete NGINX web server with SSL, PHP-FPM, and PostgreSQL backend.

```yaml
# web-stack.yaml - Production NGINX + PHP + PostgreSQL Stack
#
# This example demonstrates:
# - Package installation with automatic updates
# - Service management with dependencies
# - SSL certificate management
# - Nginx configuration with security headers
# - PHP-FPM pool configuration
# - PostgreSQL database setup
# - Firewall rules
# - System tuning

# Variables (set via pillar or state file vars)
# .vars.domain: example.com
# .vars.db_password: secure-password
# .vars.php_version: "8.2"

# --- Base System Configuration ---

system_packages:
  module: package
  state: installed
  names:
    - nginx
    - php{{ .vars.php_version }}-fpm
    - php{{ .vars.php_version }}-pgsql
    - php{{ .vars.php_version }}-mbstring
    - php{{ .vars.php_version }}-xml
    - php{{ .vars.php_version }}-curl
    - postgresql-15
    - certbot
    - python3-certbot-nginx

system_tuning:
  module: sysctl
  state: present
  settings:
    net.core.somaxconn: 65535
    net.ipv4.tcp_max_syn_backlog: 65535
    net.ipv4.ip_local_port_range: "1024 65535"
    net.ipv4.tcp_tw_reuse: 1
    vm.swappiness: 10

# --- Firewall Configuration ---

firewall_http:
  module: firewall
  state: allow
  port: 80
  protocol: tcp

firewall_https:
  module: firewall
  state: allow
  port: 443
  protocol: tcp

# --- SSL Certificate ---

ssl_certificate:
  module: cmd
  state: run
  name: obtain_ssl_cert
  command: |
    certbot certonly --nginx -d {{ .vars.domain }} -d www.{{ .vars.domain }} \
      --non-interactive --agree-tos --email admin@{{ .vars.domain }}
  creates: /etc/letsencrypt/live/{{ .vars.domain }}/fullchain.pem
  require:
    - system_packages
    - firewall_http

ssl_renewal_cron:
  module: cron
  state: present
  name: certbot-renewal
  user: root
  hour: 2
  minute: 30
  weekday: 1
  job: certbot renew --quiet --post-hook "systemctl reload nginx"
  require:
    - ssl_certificate

# --- NGINX Configuration ---

nginx_main_config:
  module: file
  state: managed
  path: /etc/nginx/nginx.conf
  content: |
    user www-data;
    worker_processes auto;
    pid /run/nginx.pid;

    events {
        worker_connections 4096;
        use epoll;
        multi_accept on;
    }

    http {
        sendfile on;
        tcp_nopush on;
        tcp_nodelay on;
        keepalive_timeout 65;
        types_hash_max_size 2048;
        server_tokens off;

        include /etc/nginx/mime.types;
        default_type application/octet-stream;

        # Logging
        access_log /var/log/nginx/access.log;
        error_log /var/log/nginx/error.log;

        # Gzip compression
        gzip on;
        gzip_vary on;
        gzip_proxied any;
        gzip_comp_level 6;
        gzip_types text/plain text/css application/json application/javascript text/xml application/xml;

        # SSL settings
        ssl_protocols TLSv1.2 TLSv1.3;
        ssl_prefer_server_ciphers off;
        ssl_session_cache shared:SSL:10m;
        ssl_session_timeout 1d;

        include /etc/nginx/conf.d/*.conf;
        include /etc/nginx/sites-enabled/*;
    }
  mode: "0644"
  require:
    - system_packages

nginx_site_config:
  module: file
  state: managed
  path: /etc/nginx/sites-available/{{ .vars.domain }}
  content: |
    server {
        listen 80;
        server_name {{ .vars.domain }} www.{{ .vars.domain }};
        return 301 https://$server_name$request_uri;
    }

    server {
        listen 443 ssl http2;
        server_name {{ .vars.domain }} www.{{ .vars.domain }};

        root /var/www/{{ .vars.domain }};
        index index.php index.html;

        # SSL
        ssl_certificate /etc/letsencrypt/live/{{ .vars.domain }}/fullchain.pem;
        ssl_certificate_key /etc/letsencrypt/live/{{ .vars.domain }}/privkey.pem;

        # Security headers
        add_header X-Frame-Options "SAMEORIGIN" always;
        add_header X-Content-Type-Options "nosniff" always;
        add_header X-XSS-Protection "1; mode=block" always;
        add_header Referrer-Policy "strict-origin-when-cross-origin" always;
        add_header Content-Security-Policy "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline';" always;

        location / {
            try_files $uri $uri/ /index.php?$query_string;
        }

        location ~ \.php$ {
            fastcgi_pass unix:/run/php/php{{ .vars.php_version }}-fpm.sock;
            fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
            include fastcgi_params;
            fastcgi_read_timeout 300;
        }

        location ~ /\.ht {
            deny all;
        }

        # Static file caching
        location ~* \.(jpg|jpeg|png|gif|ico|css|js|woff2)$ {
            expires 30d;
            add_header Cache-Control "public, immutable";
        }
    }
  mode: "0644"
  require:
    - nginx_main_config
    - ssl_certificate

nginx_site_enable:
  module: file
  state: symlink
  path: /etc/nginx/sites-enabled/{{ .vars.domain }}
  target: /etc/nginx/sites-available/{{ .vars.domain }}
  require:
    - nginx_site_config

web_root:
  module: file
  state: directory
  path: /var/www/{{ .vars.domain }}
  owner: www-data
  group: www-data
  mode: "0755"

# --- PHP-FPM Configuration ---

php_fpm_pool:
  module: file
  state: managed
  path: /etc/php/{{ .vars.php_version }}/fpm/pool.d/www.conf
  content: |
    [www]
    user = www-data
    group = www-data
    listen = /run/php/php{{ .vars.php_version }}-fpm.sock
    listen.owner = www-data
    listen.group = www-data

    pm = dynamic
    pm.max_children = {{ mul .facts.cpu_count 4 }}
    pm.start_servers = {{ mul .facts.cpu_count 2 }}
    pm.min_spare_servers = {{ .facts.cpu_count }}
    pm.max_spare_servers = {{ mul .facts.cpu_count 3 }}
    pm.max_requests = 500

    php_admin_value[error_log] = /var/log/php-fpm/www-error.log
    php_admin_flag[log_errors] = on
    php_admin_value[memory_limit] = 256M
    php_admin_value[upload_max_filesize] = 50M
    php_admin_value[post_max_size] = 50M
    php_admin_value[max_execution_time] = 300
  mode: "0644"
  require:
    - system_packages

php_log_dir:
  module: file
  state: directory
  path: /var/log/php-fpm
  owner: www-data
  group: www-data
  mode: "0755"

# --- PostgreSQL Configuration ---

postgres_service:
  module: service
  state: running
  name: postgresql
  enabled: true
  require:
    - system_packages

app_database:
  module: postgres_database
  state: present
  name: app_production
  encoding: UTF8
  lc_collate: en_US.UTF-8
  lc_ctype: en_US.UTF-8
  require:
    - postgres_service

app_db_user:
  module: postgres_user
  state: present
  name: app_user
  password: "{{ .vars.db_password }}"
  databases:
    - name: app_production
      privileges: ALL
  require:
    - app_database

# --- Service Management ---

php_fpm_service:
  module: service
  state: running
  name: php{{ .vars.php_version }}-fpm
  enabled: true
  require:
    - php_fpm_pool
    - php_log_dir
  watch:
    - php_fpm_pool

nginx_service:
  module: service
  state: running
  name: nginx
  enabled: true
  require:
    - nginx_site_enable
    - php_fpm_service
  watch:
    - nginx_main_config
    - nginx_site_config
```

### Example 2: Kubernetes Node Preparation

Prepare a Linux server as a Kubernetes worker node.

```yaml
# k8s-node.yaml - Kubernetes Node Preparation
#
# This example demonstrates:
# - Kernel module loading
# - System parameter tuning
# - Container runtime installation
# - Kubernetes component installation
# - Network configuration
# - Security settings

# Variables:
# .vars.k8s_version: "1.29"
# .vars.containerd_version: "1.7"
# .vars.control_plane_endpoint: "k8s-control:6443"
# .vars.pod_cidr: "10.244.0.0/16"

# --- System Prerequisites ---

disable_swap:
  module: swap
  state: absent
  all: true

disable_swap_fstab:
  module: file
  state: managed
  path: /etc/fstab
  pattern: ".*swap.*"
  repl: "# \\0"
  backup: true

required_kernel_modules:
  module: kernel_module
  state: present
  names:
    - overlay
    - br_netfilter

kernel_modules_load:
  module: file
  state: managed
  path: /etc/modules-load.d/k8s.conf
  content: |
    overlay
    br_netfilter
  mode: "0644"

k8s_sysctl:
  module: sysctl
  state: present
  settings:
    net.bridge.bridge-nf-call-iptables: 1
    net.bridge.bridge-nf-call-ip6tables: 1
    net.ipv4.ip_forward: 1
    net.ipv4.conf.all.forwarding: 1
    vm.overcommit_memory: 1
    vm.panic_on_oom: 0
    kernel.panic: 10
    kernel.panic_on_oops: 1
  require:
    - required_kernel_modules

# --- Firewall Configuration ---

k8s_kubelet_port:
  module: firewall
  state: allow
  port: 10250
  protocol: tcp

k8s_nodeport_range:
  module: firewall
  state: allow
  port: 30000-32767
  protocol: tcp

k8s_flannel_vxlan:
  module: firewall
  state: allow
  port: 8472
  protocol: udp

# --- Container Runtime (containerd) ---

containerd_prerequisites:
  module: package
  state: installed
  names:
    - apt-transport-https
    - ca-certificates
    - curl
    - gnupg
    - lsb-release

docker_gpg_key:
  module: cmd
  state: run
  name: add_docker_gpg
  command: |
    curl -fsSL https://download.docker.com/linux/{{ .facts.os | lower }}/gpg | \
      gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg
  creates: /usr/share/keyrings/docker-archive-keyring.gpg
  require:
    - containerd_prerequisites

docker_apt_repo:
  module: file
  state: managed
  path: /etc/apt/sources.list.d/docker.list
  content: |
    deb [arch={{ .facts.architecture }} signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] \
    https://download.docker.com/linux/{{ .facts.os | lower }} {{ .facts.os_codename }} stable
  mode: "0644"
  require:
    - docker_gpg_key

containerd_package:
  module: package
  state: installed
  name: containerd.io
  update_cache: true
  require:
    - docker_apt_repo

containerd_config_dir:
  module: file
  state: directory
  path: /etc/containerd
  mode: "0755"

containerd_config:
  module: file
  state: managed
  path: /etc/containerd/config.toml
  content: |
    version = 2

    [plugins]
      [plugins."io.containerd.grpc.v1.cri"]
        sandbox_image = "registry.k8s.io/pause:3.9"
        [plugins."io.containerd.grpc.v1.cri".containerd]
          [plugins."io.containerd.grpc.v1.cri".containerd.runtimes]
            [plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc]
              runtime_type = "io.containerd.runc.v2"
              [plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc.options]
                SystemdCgroup = true
        [plugins."io.containerd.grpc.v1.cri".cni]
          bin_dir = "/opt/cni/bin"
          conf_dir = "/etc/cni/net.d"
  mode: "0644"
  require:
    - containerd_config_dir

containerd_service:
  module: service
  state: running
  name: containerd
  enabled: true
  require:
    - containerd_package
    - containerd_config
  watch:
    - containerd_config

# --- Kubernetes Components ---

k8s_gpg_key:
  module: cmd
  state: run
  name: add_k8s_gpg
  command: |
    curl -fsSL https://pkgs.k8s.io/core:/stable:/v{{ .vars.k8s_version }}/deb/Release.key | \
      gpg --dearmor -o /usr/share/keyrings/kubernetes-apt-keyring.gpg
  creates: /usr/share/keyrings/kubernetes-apt-keyring.gpg

k8s_apt_repo:
  module: file
  state: managed
  path: /etc/apt/sources.list.d/kubernetes.list
  content: |
    deb [signed-by=/usr/share/keyrings/kubernetes-apt-keyring.gpg] \
    https://pkgs.k8s.io/core:/stable:/v{{ .vars.k8s_version }}/deb/ /
  mode: "0644"
  require:
    - k8s_gpg_key

k8s_packages:
  module: package
  state: installed
  names:
    - kubelet
    - kubeadm
    - kubectl
  update_cache: true
  require:
    - k8s_apt_repo
    - containerd_service
    - k8s_sysctl
    - disable_swap

k8s_packages_hold:
  module: cmd
  state: run
  name: hold_k8s_packages
  command: apt-mark hold kubelet kubeadm kubectl
  require:
    - k8s_packages

kubelet_service:
  module: service
  state: running
  name: kubelet
  enabled: true
  require:
    - k8s_packages

# --- CNI Configuration ---

cni_bin_dir:
  module: file
  state: directory
  path: /opt/cni/bin
  mode: "0755"

cni_conf_dir:
  module: file
  state: directory
  path: /etc/cni/net.d
  mode: "0755"

# --- Crictl Configuration ---

crictl_config:
  module: file
  state: managed
  path: /etc/crictl.yaml
  content: |
    runtime-endpoint: unix:///run/containerd/containerd.sock
    image-endpoint: unix:///run/containerd/containerd.sock
    timeout: 10
    debug: false
  mode: "0644"
```

### Example 3: Database Server Hardening

Harden a PostgreSQL database server for production use.

```yaml
# postgres-hardened.yaml - Production PostgreSQL with Security Hardening
#
# This example demonstrates:
# - PostgreSQL installation and configuration
# - TLS/SSL setup for connections
# - Authentication configuration (pg_hba.conf)
# - Performance tuning based on hardware
# - Backup automation
# - Monitoring integration
# - Security hardening

# Variables:
# .vars.pg_password: secure-root-password
# .vars.replication_password: replication-password
# .vars.app_db_password: app-password
# .vars.backup_bucket: s3://my-backups/postgres

# --- Base Installation ---

postgresql_packages:
  module: package
  state: installed
  names:
    - postgresql-15
    - postgresql-contrib-15
    - python3-psycopg2  # For Ansible-style management
    - s3cmd             # For backup uploads

# --- Data Directory Security ---

postgres_data_permissions:
  module: file
  state: directory
  path: /var/lib/postgresql/15/main
  owner: postgres
  group: postgres
  mode: "0700"
  require:
    - postgresql_packages

# --- SSL Certificate Setup ---

postgres_ssl_dir:
  module: file
  state: directory
  path: /etc/postgresql/15/main/ssl
  owner: postgres
  group: postgres
  mode: "0700"

postgres_ssl_key:
  module: x509
  state: present
  path: /etc/postgresql/15/main/ssl/server.key
  type: private_key
  bits: 4096
  owner: postgres
  group: postgres
  mode: "0600"
  require:
    - postgres_ssl_dir

postgres_ssl_cert:
  module: x509
  state: present
  path: /etc/postgresql/15/main/ssl/server.crt
  type: certificate
  private_key: /etc/postgresql/15/main/ssl/server.key
  common_name: "{{ .facts.fqdn }}"
  days: 365
  owner: postgres
  group: postgres
  mode: "0644"
  require:
    - postgres_ssl_key

# --- PostgreSQL Configuration ---

postgresql_conf:
  module: file
  state: managed
  path: /etc/postgresql/15/main/postgresql.conf
  content: |
    # Connection Settings
    listen_addresses = '*'
    port = 5432
    max_connections = 200

    # Authentication
    password_encryption = scram-sha-256

    # SSL/TLS
    ssl = on
    ssl_cert_file = '/etc/postgresql/15/main/ssl/server.crt'
    ssl_key_file = '/etc/postgresql/15/main/ssl/server.key'
    ssl_min_protocol_version = 'TLSv1.2'
    ssl_ciphers = 'HIGH:MEDIUM:+3DES:!aNULL'

    # Memory Settings (based on available RAM)
    shared_buffers = {{ div .facts.memory_mb 4 }}MB
    effective_cache_size = {{ div (mul .facts.memory_mb 3) 4 }}MB
    maintenance_work_mem = {{ min (div .facts.memory_mb 8) 2048 }}MB
    work_mem = {{ div .facts.memory_mb 100 }}MB

    # Checkpoint Settings
    checkpoint_completion_target = 0.9
    wal_buffers = 64MB
    min_wal_size = 1GB
    max_wal_size = 4GB

    # Query Planner
    random_page_cost = 1.1  # For SSD storage
    effective_io_concurrency = 200

    # Parallel Query
    max_parallel_workers_per_gather = {{ div .facts.cpu_count 2 }}
    max_parallel_workers = {{ .facts.cpu_count }}
    max_parallel_maintenance_workers = {{ div .facts.cpu_count 2 }}

    # WAL Archiving for Point-in-Time Recovery
    archive_mode = on
    archive_command = 'test ! -f /var/lib/postgresql/wal_archive/%f && cp %p /var/lib/postgresql/wal_archive/%f'

    # Replication
    wal_level = replica
    max_wal_senders = 5
    wal_keep_size = 1GB

    # Logging
    logging_collector = on
    log_directory = '/var/log/postgresql'
    log_filename = 'postgresql-%Y-%m-%d.log'
    log_rotation_age = 1d
    log_rotation_size = 100MB
    log_min_duration_statement = 1000  # Log queries > 1 second
    log_checkpoints = on
    log_connections = on
    log_disconnections = on
    log_lock_waits = on
    log_statement = 'ddl'
    log_temp_files = 0

    # Statistics
    shared_preload_libraries = 'pg_stat_statements'
    pg_stat_statements.max = 10000
    pg_stat_statements.track = all
  mode: "0644"
  owner: postgres
  group: postgres
  require:
    - postgres_ssl_cert

# --- Authentication Configuration ---

pg_hba_conf:
  module: file
  state: managed
  path: /etc/postgresql/15/main/pg_hba.conf
  content: |
    # TYPE  DATABASE        USER            ADDRESS                 METHOD

    # Local connections
    local   all             postgres                                peer
    local   all             all                                     scram-sha-256

    # IPv4 local connections (localhost only for admin)
    host    all             postgres        127.0.0.1/32            scram-sha-256

    # IPv4 connections from application network (SSL required)
    hostssl all             all             10.0.0.0/8              scram-sha-256
    hostssl all             all             172.16.0.0/12           scram-sha-256
    hostssl all             all             192.168.0.0/16          scram-sha-256

    # Replication connections (from standby servers)
    hostssl replication     replicator      10.0.0.0/8              scram-sha-256

    # Deny all other connections
    host    all             all             0.0.0.0/0               reject
  mode: "0640"
  owner: postgres
  group: postgres
  require:
    - postgresql_packages

# --- Create Directories ---

postgres_log_dir:
  module: file
  state: directory
  path: /var/log/postgresql
  owner: postgres
  group: postgres
  mode: "0755"

wal_archive_dir:
  module: file
  state: directory
  path: /var/lib/postgresql/wal_archive
  owner: postgres
  group: postgres
  mode: "0700"

backup_dir:
  module: file
  state: directory
  path: /var/lib/postgresql/backups
  owner: postgres
  group: postgres
  mode: "0700"

# --- Database and User Setup ---

postgres_service:
  module: service
  state: running
  name: postgresql
  enabled: true
  require:
    - postgresql_conf
    - pg_hba_conf
    - postgres_log_dir
    - wal_archive_dir
  watch:
    - postgresql_conf
    - pg_hba_conf

pg_stat_statements_ext:
  module: postgres_extension
  state: present
  name: pg_stat_statements
  database: postgres
  require:
    - postgres_service

replicator_user:
  module: postgres_user
  state: present
  name: replicator
  password: "{{ .vars.replication_password }}"
  role_attr_flags: REPLICATION,LOGIN
  require:
    - postgres_service

app_database:
  module: postgres_database
  state: present
  name: application
  encoding: UTF8
  require:
    - postgres_service

app_user:
  module: postgres_user
  state: present
  name: app_user
  password: "{{ .vars.app_db_password }}"
  databases:
    - name: application
      privileges: ALL
  require:
    - app_database

# --- Firewall ---

postgres_firewall:
  module: firewall
  state: allow
  port: 5432
  protocol: tcp
  source: 10.0.0.0/8

# --- Backup Automation ---

backup_script:
  module: file
  state: managed
  path: /usr/local/bin/pg_backup.sh
  content: |
    #!/bin/bash
    set -euo pipefail

    BACKUP_DIR="/var/lib/postgresql/backups"
    DATE=$(date +%Y%m%d_%H%M%S)
    BACKUP_FILE="${BACKUP_DIR}/pg_backup_${DATE}.sql.gz"

    # Create backup
    pg_dumpall -U postgres | gzip > "${BACKUP_FILE}"

    # Upload to S3
    s3cmd put "${BACKUP_FILE}" {{ .vars.backup_bucket }}/

    # Clean up local backups older than 7 days
    find "${BACKUP_DIR}" -name "pg_backup_*.sql.gz" -mtime +7 -delete

    # Log success
    logger -t pg_backup "Backup completed: ${BACKUP_FILE}"
  mode: "0750"
  owner: postgres
  group: postgres
  require:
    - backup_dir

backup_cron:
  module: cron
  state: present
  name: postgresql-backup
  user: postgres
  hour: 2
  minute: 0
  job: /usr/local/bin/pg_backup.sh
  require:
    - backup_script

# --- Monitoring Integration ---

postgres_exporter_user:
  module: postgres_user
  state: present
  name: postgres_exporter
  password: "{{ .vars.exporter_password | default \"exporter-password\" }}"
  databases:
    - name: postgres
      privileges: "CONNECT"
  require:
    - postgres_service

postgres_exporter_grants:
  module: cmd
  state: run
  name: exporter_grants
  command: |
    psql -U postgres -c "GRANT pg_monitor TO postgres_exporter;"
  require:
    - postgres_exporter_user
```

### Example 4: Docker Application Deployment

Deploy a containerized application with Docker Compose-like state management.

```yaml
# docker-app.yaml - Multi-Container Application Deployment
#
# This example demonstrates:
# - Docker network creation
# - Docker volume management
# - Multi-container deployment with dependencies
# - Environment configuration
# - Health check configuration
# - Log driver configuration

# Variables:
# .vars.app_version: "1.2.3"
# .vars.redis_password: redis-secret
# .vars.app_secret_key: app-secret

# --- Docker Installation (if needed) ---

docker_prerequisites:
  module: package
  state: installed
  names:
    - apt-transport-https
    - ca-certificates
    - curl
    - gnupg

docker_gpg:
  module: cmd
  state: run
  name: docker_gpg_key
  command: |
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg | \
      gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg
  creates: /usr/share/keyrings/docker-archive-keyring.gpg
  require:
    - docker_prerequisites

docker_repo:
  module: file
  state: managed
  path: /etc/apt/sources.list.d/docker.list
  content: |
    deb [arch=amd64 signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] \
    https://download.docker.com/linux/ubuntu {{ .facts.os_codename }} stable
  mode: "0644"
  require:
    - docker_gpg

docker_packages:
  module: package
  state: installed
  names:
    - docker-ce
    - docker-ce-cli
    - containerd.io
  update_cache: true
  require:
    - docker_repo

docker_service:
  module: service
  state: running
  name: docker
  enabled: true
  require:
    - docker_packages

# --- Application Network ---

app_network:
  module: docker_network
  state: present
  name: myapp-network
  driver: bridge
  ipam:
    driver: default
    config:
      - subnet: 172.28.0.0/16
  require:
    - docker_service

# --- Persistent Volumes ---

redis_volume:
  module: docker_volume
  state: present
  name: myapp-redis-data
  require:
    - docker_service

postgres_volume:
  module: docker_volume
  state: present
  name: myapp-postgres-data
  require:
    - docker_service

app_uploads_volume:
  module: docker_volume
  state: present
  name: myapp-uploads
  require:
    - docker_service

# --- Redis Container ---

redis_container:
  module: docker_container
  state: running
  name: myapp-redis
  image: redis:7-alpine
  command: redis-server --requirepass {{ .vars.redis_password }}
  networks:
    - name: myapp-network
      aliases:
        - redis
  volumes:
    - myapp-redis-data:/data
  restart_policy: unless-stopped
  healthcheck:
    test: ["CMD", "redis-cli", "-a", "{{ .vars.redis_password }}", "ping"]
    interval: 10s
    timeout: 5s
    retries: 3
  log_driver: json-file
  log_options:
    max-size: "10m"
    max-file: "3"
  require:
    - app_network
    - redis_volume

# --- PostgreSQL Container ---

postgres_container:
  module: docker_container
  state: running
  name: myapp-postgres
  image: postgres:15-alpine
  environment:
    POSTGRES_DB: myapp
    POSTGRES_USER: myapp
    POSTGRES_PASSWORD: "{{ .vars.db_password }}"
  networks:
    - name: myapp-network
      aliases:
        - postgres
        - db
  volumes:
    - myapp-postgres-data:/var/lib/postgresql/data
  restart_policy: unless-stopped
  healthcheck:
    test: ["CMD-SHELL", "pg_isready -U myapp"]
    interval: 10s
    timeout: 5s
    retries: 5
  log_driver: json-file
  log_options:
    max-size: "10m"
    max-file: "3"
  require:
    - app_network
    - postgres_volume

# --- Application Container ---

app_container:
  module: docker_container
  state: running
  name: myapp-api
  image: "myregistry.io/myapp:{{ .vars.app_version }}"
  environment:
    DATABASE_URL: "postgresql://myapp:{{ .vars.db_password }}@postgres:5432/myapp"
    REDIS_URL: "redis://:{{ .vars.redis_password }}@redis:6379/0"
    SECRET_KEY: "{{ .vars.app_secret_key }}"
    ENVIRONMENT: production
    LOG_LEVEL: info
  networks:
    - name: myapp-network
      aliases:
        - api
  volumes:
    - myapp-uploads:/app/uploads
  ports:
    - "127.0.0.1:8000:8000"
  restart_policy: unless-stopped
  healthcheck:
    test: ["CMD", "curl", "-f", "http://localhost:8000/health"]
    interval: 30s
    timeout: 10s
    retries: 3
    start_period: 40s
  log_driver: json-file
  log_options:
    max-size: "50m"
    max-file: "5"
  require:
    - redis_container
    - postgres_container
    - app_uploads_volume

# --- Worker Container ---

worker_container:
  module: docker_container
  state: running
  name: myapp-worker
  image: "myregistry.io/myapp:{{ .vars.app_version }}"
  command: celery -A myapp worker -l info
  environment:
    DATABASE_URL: "postgresql://myapp:{{ .vars.db_password }}@postgres:5432/myapp"
    REDIS_URL: "redis://:{{ .vars.redis_password }}@redis:6379/0"
    SECRET_KEY: "{{ .vars.app_secret_key }}"
  networks:
    - name: myapp-network
  restart_policy: unless-stopped
  log_driver: json-file
  log_options:
    max-size: "50m"
    max-file: "5"
  require:
    - redis_container
    - postgres_container

# --- Scheduler Container ---

scheduler_container:
  module: docker_container
  state: running
  name: myapp-scheduler
  image: "myregistry.io/myapp:{{ .vars.app_version }}"
  command: celery -A myapp beat -l info
  environment:
    DATABASE_URL: "postgresql://myapp:{{ .vars.db_password }}@postgres:5432/myapp"
    REDIS_URL: "redis://:{{ .vars.redis_password }}@redis:6379/0"
  networks:
    - name: myapp-network
  restart_policy: unless-stopped
  log_driver: json-file
  log_options:
    max-size: "10m"
    max-file: "3"
  require:
    - redis_container
    - postgres_container

# --- Nginx Reverse Proxy ---

nginx_config_dir:
  module: file
  state: directory
  path: /etc/nginx/conf.d
  mode: "0755"

nginx_upstream_config:
  module: file
  state: managed
  path: /etc/nginx/conf.d/myapp.conf
  content: |
    upstream myapp_api {
        server 127.0.0.1:8000;
        keepalive 32;
    }

    server {
        listen 80;
        server_name {{ .vars.domain }};

        client_max_body_size 100M;

        location / {
            proxy_pass http://myapp_api;
            proxy_http_version 1.1;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
            proxy_set_header Connection "";
            proxy_connect_timeout 60s;
            proxy_send_timeout 60s;
            proxy_read_timeout 60s;
        }

        location /health {
            access_log off;
            proxy_pass http://myapp_api/health;
        }
    }
  mode: "0644"
  require:
    - nginx_config_dir
    - app_container

nginx_reload:
  module: service
  state: running
  name: nginx
  reload: true
  require:
    - nginx_upstream_config
  watch:
    - nginx_upstream_config
```

### Example 5: Security Baseline Compliance

Implement CIS benchmark security controls.

```yaml
# security-baseline.yaml - CIS Level 1 Security Baseline
#
# This example demonstrates:
# - Filesystem hardening
# - Kernel parameter security
# - Authentication hardening
# - SSH hardening
# - Audit configuration
# - Service hardening

# --- Filesystem Hardening ---

# CIS 1.1.2-1.1.5: Ensure /tmp is configured properly
tmp_mount_options:
  module: mount
  state: present
  path: /tmp
  src: tmpfs
  fstype: tmpfs
  opts: defaults,noexec,nosuid,nodev,size=2G

# CIS 1.1.8-1.1.9: Ensure /var/tmp has noexec
var_tmp_bind:
  module: mount
  state: present
  path: /var/tmp
  src: /tmp
  fstype: none
  opts: bind

# CIS 1.1.21: Ensure sticky bit is set on all world-writable directories
sticky_bit_check:
  module: cmd
  state: run
  name: set_sticky_bits
  command: |
    df --local -P | awk {'if (NR!=1) print $6'} | xargs -I '{}' find '{}' -xdev -type d -perm -0002 2>/dev/null | \
    xargs -I '{}' chmod a+t '{}'
  onlyif: |
    test $(df --local -P | awk {'if (NR!=1) print $6'} | xargs -I '{}' find '{}' -xdev -type d \( -perm -0002 -a ! -perm -1000 \) 2>/dev/null | wc -l) -gt 0

# --- Kernel Parameters (CIS 3.x) ---

network_security_sysctl:
  module: sysctl
  state: present
  settings:
    # CIS 3.1.1: Disable IPv6 (if not required)
    # net.ipv6.conf.all.disable_ipv6: 1
    # net.ipv6.conf.default.disable_ipv6: 1

    # CIS 3.2.1: Source routed packets
    net.ipv4.conf.all.accept_source_route: 0
    net.ipv4.conf.default.accept_source_route: 0
    net.ipv6.conf.all.accept_source_route: 0
    net.ipv6.conf.default.accept_source_route: 0

    # CIS 3.2.2: ICMP redirects
    net.ipv4.conf.all.accept_redirects: 0
    net.ipv4.conf.default.accept_redirects: 0
    net.ipv6.conf.all.accept_redirects: 0
    net.ipv6.conf.default.accept_redirects: 0

    # CIS 3.2.3: Secure ICMP redirects
    net.ipv4.conf.all.secure_redirects: 0
    net.ipv4.conf.default.secure_redirects: 0

    # CIS 3.2.4: Log suspicious packets
    net.ipv4.conf.all.log_martians: 1
    net.ipv4.conf.default.log_martians: 1

    # CIS 3.2.5: Ignore broadcast ICMP
    net.ipv4.icmp_echo_ignore_broadcasts: 1

    # CIS 3.2.6: Ignore bogus ICMP errors
    net.ipv4.icmp_ignore_bogus_error_responses: 1

    # CIS 3.2.7: Reverse path filtering
    net.ipv4.conf.all.rp_filter: 1
    net.ipv4.conf.default.rp_filter: 1

    # CIS 3.2.8: TCP SYN Cookies
    net.ipv4.tcp_syncookies: 1

    # CIS 3.2.9: IPv6 router advertisements
    net.ipv6.conf.all.accept_ra: 0
    net.ipv6.conf.default.accept_ra: 0

# --- Core Dumps (CIS 1.5.1) ---

disable_core_dumps:
  module: file
  state: managed
  path: /etc/security/limits.d/core.conf
  content: |
    * hard core 0
  mode: "0644"

sysctl_core_dump:
  module: sysctl
  state: present
  settings:
    fs.suid_dumpable: 0

# --- PAM Configuration (CIS 5.x) ---

pwquality_conf:
  module: file
  state: managed
  path: /etc/security/pwquality.conf
  content: |
    # CIS 5.3.1: Password quality requirements
    minlen = 14
    minclass = 4
    dcredit = -1
    ucredit = -1
    ocredit = -1
    lcredit = -1
    maxrepeat = 3
    maxclassrepeat = 2
    gecoscheck = 1
    dictcheck = 1
  mode: "0644"

# CIS 5.4.1.1-5.4.1.4: Password expiration
login_defs:
  module: file
  state: managed
  path: /etc/login.defs
  pattern: "{{ .item.pattern }}"
  repl: "{{ .item.repl }}"
  loop:
    - pattern: "^PASS_MAX_DAYS.*"
      repl: "PASS_MAX_DAYS   365"
    - pattern: "^PASS_MIN_DAYS.*"
      repl: "PASS_MIN_DAYS   1"
    - pattern: "^PASS_WARN_AGE.*"
      repl: "PASS_WARN_AGE   7"

# --- SSH Hardening (CIS 5.2.x) ---

sshd_config:
  module: sshd_config
  state: present
  settings:
    # CIS 5.2.1: Permissions
    # (handled by file module below)

    # CIS 5.2.4-5.2.5: Access control
    Protocol: 2
    LogLevel: VERBOSE

    # CIS 5.2.6: X11 forwarding
    X11Forwarding: "no"

    # CIS 5.2.7: Max auth tries
    MaxAuthTries: 4

    # CIS 5.2.8: Ignore rhosts
    IgnoreRhosts: "yes"

    # CIS 5.2.9: Host-based authentication
    HostbasedAuthentication: "no"

    # CIS 5.2.10: Root login
    PermitRootLogin: "no"

    # CIS 5.2.11: Empty passwords
    PermitEmptyPasswords: "no"

    # CIS 5.2.12: User environment
    PermitUserEnvironment: "no"

    # CIS 5.2.13: Strong ciphers only
    Ciphers: "chacha20-poly1305@openssh.com,aes256-gcm@openssh.com,aes128-gcm@openssh.com,aes256-ctr,aes192-ctr,aes128-ctr"

    # CIS 5.2.14: Strong MACs only
    MACs: "hmac-sha2-512-etm@openssh.com,hmac-sha2-256-etm@openssh.com,hmac-sha2-512,hmac-sha2-256"

    # CIS 5.2.15: Strong key exchange
    KexAlgorithms: "curve25519-sha256,curve25519-sha256@libssh.org,ecdh-sha2-nistp521,ecdh-sha2-nistp384,ecdh-sha2-nistp256,diffie-hellman-group-exchange-sha256"

    # CIS 5.2.16: Idle timeout
    ClientAliveInterval: 300
    ClientAliveCountMax: 3

    # CIS 5.2.17: Login grace time
    LoginGraceTime: 60

    # CIS 5.2.18: Warning banner
    Banner: /etc/issue.net

    # CIS 5.2.19: PAM
    UsePAM: "yes"

    # CIS 5.2.21: Max sessions
    MaxSessions: 10

    # CIS 5.2.22: Max startups
    MaxStartups: "10:30:60"

sshd_permissions:
  module: file
  state: managed
  path: /etc/ssh/sshd_config
  mode: "0600"
  owner: root
  group: root

ssh_banner:
  module: file
  state: managed
  path: /etc/issue.net
  content: |
    ***************************************************************************
                                WARNING NOTICE

    This system is for authorized use only. All activities are monitored and
    recorded. Unauthorized access or use is prohibited and may result in
    criminal prosecution.
    ***************************************************************************
  mode: "0644"

sshd_restart:
  module: service
  state: running
  name: sshd
  enabled: true
  watch:
    - sshd_config

# --- Audit Configuration (CIS 4.1.x) ---

auditd_package:
  module: package
  state: installed
  name: auditd

audit_rules:
  module: file
  state: managed
  path: /etc/audit/rules.d/cis.rules
  content: |
    # CIS 4.1.3: Ensure events that modify date/time are collected
    -a always,exit -F arch=b64 -S adjtimex -S settimeofday -k time-change
    -a always,exit -F arch=b32 -S adjtimex -S settimeofday -S stime -k time-change
    -a always,exit -F arch=b64 -S clock_settime -k time-change
    -a always,exit -F arch=b32 -S clock_settime -k time-change
    -w /etc/localtime -p wa -k time-change

    # CIS 4.1.4: Ensure events that modify user/group are collected
    -w /etc/group -p wa -k identity
    -w /etc/passwd -p wa -k identity
    -w /etc/gshadow -p wa -k identity
    -w /etc/shadow -p wa -k identity
    -w /etc/security/opasswd -p wa -k identity

    # CIS 4.1.5: Ensure events that modify network are collected
    -a always,exit -F arch=b64 -S sethostname -S setdomainname -k system-locale
    -a always,exit -F arch=b32 -S sethostname -S setdomainname -k system-locale
    -w /etc/issue -p wa -k system-locale
    -w /etc/issue.net -p wa -k system-locale
    -w /etc/hosts -p wa -k system-locale
    -w /etc/network -p wa -k system-locale

    # CIS 4.1.6: Ensure events that modify MAC are collected
    -w /etc/apparmor/ -p wa -k MAC-policy
    -w /etc/apparmor.d/ -p wa -k MAC-policy

    # CIS 4.1.7: Ensure login/logout events are collected
    -w /var/log/faillog -p wa -k logins
    -w /var/log/lastlog -p wa -k logins
    -w /var/log/tallylog -p wa -k logins

    # CIS 4.1.8: Ensure session initiation is collected
    -w /var/run/utmp -p wa -k session
    -w /var/log/wtmp -p wa -k logins
    -w /var/log/btmp -p wa -k logins

    # CIS 4.1.9: Ensure discretionary access control permission modification events
    -a always,exit -F arch=b64 -S chmod -S fchmod -S fchmodat -F auid>=1000 -F auid!=4294967295 -k perm_mod
    -a always,exit -F arch=b32 -S chmod -S fchmod -S fchmodat -F auid>=1000 -F auid!=4294967295 -k perm_mod
    -a always,exit -F arch=b64 -S chown -S fchown -S fchownat -S lchown -F auid>=1000 -F auid!=4294967295 -k perm_mod
    -a always,exit -F arch=b32 -S chown -S fchown -S fchownat -S lchown -F auid>=1000 -F auid!=4294967295 -k perm_mod

    # CIS 4.1.10: Ensure unsuccessful unauthorized file access attempts
    -a always,exit -F arch=b64 -S creat -S open -S openat -S truncate -S ftruncate -F exit=-EACCES -F auid>=1000 -F auid!=4294967295 -k access
    -a always,exit -F arch=b32 -S creat -S open -S openat -S truncate -S ftruncate -F exit=-EACCES -F auid>=1000 -F auid!=4294967295 -k access
    -a always,exit -F arch=b64 -S creat -S open -S openat -S truncate -S ftruncate -F exit=-EPERM -F auid>=1000 -F auid!=4294967295 -k access
    -a always,exit -F arch=b32 -S creat -S open -S openat -S truncate -S ftruncate -F exit=-EPERM -F auid>=1000 -F auid!=4294967295 -k access

    # CIS 4.1.14: Ensure successful file system mounts are collected
    -a always,exit -F arch=b64 -S mount -F auid>=1000 -F auid!=4294967295 -k mounts
    -a always,exit -F arch=b32 -S mount -F auid>=1000 -F auid!=4294967295 -k mounts

    # CIS 4.1.15: Ensure file deletion events are collected
    -a always,exit -F arch=b64 -S unlink -S unlinkat -S rename -S renameat -F auid>=1000 -F auid!=4294967295 -k delete
    -a always,exit -F arch=b32 -S unlink -S unlinkat -S rename -S renameat -F auid>=1000 -F auid!=4294967295 -k delete

    # CIS 4.1.16: Ensure changes to sudoers are collected
    -w /etc/sudoers -p wa -k scope
    -w /etc/sudoers.d/ -p wa -k scope

    # CIS 4.1.17: Ensure sudo command usage is collected
    -a always,exit -F arch=b64 -C euid!=uid -F euid=0 -Fauid>=1000 -F auid!=4294967295 -S execve -k actions
    -a always,exit -F arch=b32 -C euid!=uid -F euid=0 -Fauid>=1000 -F auid!=4294967295 -S execve -k actions

    # CIS 4.1.18: Ensure kernel module loading/unloading is collected
    -w /sbin/insmod -p x -k modules
    -w /sbin/rmmod -p x -k modules
    -w /sbin/modprobe -p x -k modules
    -a always,exit -F arch=b64 -S init_module -S delete_module -k modules

    # Make rules immutable (must be last)
    -e 2
  mode: "0600"
  require:
    - auditd_package

auditd_service:
  module: service
  state: running
  name: auditd
  enabled: true
  require:
    - audit_rules
  watch:
    - audit_rules

# --- Disable Unnecessary Services ---

unnecessary_services:
  module: service
  state: stopped
  enabled: false
  names:
    - avahi-daemon
    - cups
    - rpcbind
    - rsyncd

# --- Cron Security (CIS 5.1.x) ---

cron_permissions:
  module: file
  state: managed
  path: "{{ .item }}"
  mode: "0600"
  owner: root
  group: root
  loop:
    - /etc/crontab
    - /etc/cron.hourly
    - /etc/cron.daily
    - /etc/cron.weekly
    - /etc/cron.monthly
    - /etc/cron.d

cron_allow:
  module: file
  state: managed
  path: /etc/cron.allow
  content: |
    root
  mode: "0600"
  owner: root
  group: root

cron_deny:
  module: file
  state: absent
  path: /etc/cron.deny
```

### Example 6: Monitoring Agent Setup

Deploy Prometheus node exporter and Grafana agent.

```yaml
# monitoring-agent.yaml - Observability Agent Stack
#
# This example demonstrates:
# - Prometheus Node Exporter installation
# - Grafana Agent for metrics and logs
# - Custom metrics collection
# - Log forwarding configuration

# Variables:
# .vars.prometheus_url: http://prometheus:9090
# .vars.loki_url: http://loki:3100

# --- Node Exporter ---

node_exporter_user:
  module: user
  state: present
  name: node_exporter
  system: true
  shell: /usr/sbin/nologin
  home: /var/lib/node_exporter

node_exporter_download:
  module: cmd
  state: run
  name: download_node_exporter
  command: |
    curl -sSL https://github.com/prometheus/node_exporter/releases/download/v1.7.0/node_exporter-1.7.0.linux-amd64.tar.gz | \
    tar -xzf - -C /tmp && \
    mv /tmp/node_exporter-1.7.0.linux-amd64/node_exporter /usr/local/bin/
  creates: /usr/local/bin/node_exporter

node_exporter_permissions:
  module: file
  state: managed
  path: /usr/local/bin/node_exporter
  mode: "0755"
  owner: root
  group: root
  require:
    - node_exporter_download

node_exporter_textfile_dir:
  module: file
  state: directory
  path: /var/lib/node_exporter/textfile
  owner: node_exporter
  group: node_exporter
  mode: "0755"
  require:
    - node_exporter_user

node_exporter_service:
  module: file
  state: managed
  path: /etc/systemd/system/node_exporter.service
  content: |
    [Unit]
    Description=Prometheus Node Exporter
    Wants=network-online.target
    After=network-online.target

    [Service]
    User=node_exporter
    Group=node_exporter
    Type=simple
    ExecStart=/usr/local/bin/node_exporter \
      --collector.textfile.directory=/var/lib/node_exporter/textfile \
      --collector.systemd \
      --collector.processes \
      --collector.filesystem.mount-points-exclude="^/(sys|proc|dev|run)($|/)" \
      --web.listen-address=:9100
    Restart=always
    RestartSec=5

    [Install]
    WantedBy=multi-user.target
  mode: "0644"
  require:
    - node_exporter_permissions
    - node_exporter_textfile_dir

node_exporter_enabled:
  module: service
  state: running
  name: node_exporter
  enabled: true
  daemon_reload: true
  require:
    - node_exporter_service
  watch:
    - node_exporter_service

# --- Custom Metrics Script ---

custom_metrics_script:
  module: file
  state: managed
  path: /usr/local/bin/custom-metrics.sh
  content: |
    #!/bin/bash
    # Generate custom metrics for node_exporter textfile collector

    OUTPUT_FILE="/var/lib/node_exporter/textfile/custom.prom"
    TEMP_FILE="${OUTPUT_FILE}.tmp"

    # System update metrics
    if command -v apt-get &> /dev/null; then
        UPDATES=$(apt-get -s upgrade 2>/dev/null | grep -c "^Inst")
        SECURITY_UPDATES=$(apt-get -s upgrade 2>/dev/null | grep -c "^Inst.*security")
        echo "# HELP apt_upgrades_pending Number of pending apt upgrades" > "$TEMP_FILE"
        echo "# TYPE apt_upgrades_pending gauge" >> "$TEMP_FILE"
        echo "apt_upgrades_pending $UPDATES" >> "$TEMP_FILE"
        echo "# HELP apt_security_upgrades_pending Number of pending security upgrades" >> "$TEMP_FILE"
        echo "# TYPE apt_security_upgrades_pending gauge" >> "$TEMP_FILE"
        echo "apt_security_upgrades_pending $SECURITY_UPDATES" >> "$TEMP_FILE"
    fi

    # Systemd service health
    FAILED_SERVICES=$(systemctl --failed --no-legend | wc -l)
    echo "# HELP systemd_units_failed Number of failed systemd units" >> "$TEMP_FILE"
    echo "# TYPE systemd_units_failed gauge" >> "$TEMP_FILE"
    echo "systemd_units_failed $FAILED_SERVICES" >> "$TEMP_FILE"

    # Disk SMART health (if smartctl available)
    if command -v smartctl &> /dev/null; then
        for disk in /dev/sd[a-z]; do
            if [ -b "$disk" ]; then
                SMART_STATUS=$(smartctl -H "$disk" 2>/dev/null | grep -c "PASSED")
                DISK_NAME=$(basename "$disk")
                echo "# HELP disk_smart_healthy Disk SMART health status (1=healthy)" >> "$TEMP_FILE"
                echo "# TYPE disk_smart_healthy gauge" >> "$TEMP_FILE"
                echo "disk_smart_healthy{disk=\"$DISK_NAME\"} $SMART_STATUS" >> "$TEMP_FILE"
            fi
        done
    fi

    # Reboot required check
    if [ -f /var/run/reboot-required ]; then
        REBOOT_REQUIRED=1
    else
        REBOOT_REQUIRED=0
    fi
    echo "# HELP node_reboot_required Node requires reboot" >> "$TEMP_FILE"
    echo "# TYPE node_reboot_required gauge" >> "$TEMP_FILE"
    echo "node_reboot_required $REBOOT_REQUIRED" >> "$TEMP_FILE"

    # Atomically move temp file to output
    mv "$TEMP_FILE" "$OUTPUT_FILE"
  mode: "0755"
  owner: root
  group: root

custom_metrics_cron:
  module: cron
  state: present
  name: custom-metrics
  user: root
  minute: "*/5"
  job: /usr/local/bin/custom-metrics.sh
  require:
    - custom_metrics_script
    - node_exporter_textfile_dir

# --- Grafana Agent ---

grafana_agent_user:
  module: user
  state: present
  name: grafana-agent
  system: true
  shell: /usr/sbin/nologin
  home: /var/lib/grafana-agent

grafana_agent_download:
  module: cmd
  state: run
  name: download_grafana_agent
  command: |
    curl -sSL -o /tmp/grafana-agent.deb \
      "https://github.com/grafana/agent/releases/download/v0.40.0/grafana-agent-0.40.0-1.amd64.deb" && \
    dpkg -i /tmp/grafana-agent.deb
  creates: /usr/bin/grafana-agent
  require:
    - grafana_agent_user

grafana_agent_config_dir:
  module: file
  state: directory
  path: /etc/grafana-agent
  mode: "0755"

grafana_agent_data_dir:
  module: file
  state: directory
  path: /var/lib/grafana-agent
  owner: grafana-agent
  group: grafana-agent
  mode: "0755"
  require:
    - grafana_agent_user

grafana_agent_config:
  module: file
  state: managed
  path: /etc/grafana-agent/agent.yaml
  content: |
    server:
      log_level: info
      http_listen_port: 12345

    metrics:
      global:
        scrape_interval: 60s
        external_labels:
          cluster: {{ .vars.cluster_name | default "default" }}
          host: {{ .facts.hostname }}

      wal_directory: /var/lib/grafana-agent/wal

      configs:
        - name: default
          scrape_configs:
            # Scrape node exporter
            - job_name: node
              static_configs:
                - targets: ['localhost:9100']
                  labels:
                    instance: {{ .facts.fqdn }}

            # Scrape kscore agent (if present)
            - job_name: kscore-agent
              static_configs:
                - targets: ['localhost:9091']
              relabel_configs:
                - source_labels: [__address__]
                  target_label: instance
                  replacement: {{ .facts.fqdn }}

          remote_write:
            - url: {{ .vars.prometheus_url }}/api/v1/write
              basic_auth:
                username: {{ .vars.prometheus_user | default "agent" }}
                password: {{ .vars.prometheus_password | default "" }}

    logs:
      configs:
        - name: default
          clients:
            - url: {{ .vars.loki_url }}/loki/api/v1/push
              basic_auth:
                username: {{ .vars.loki_user | default "agent" }}
                password: {{ .vars.loki_password | default "" }}
              external_labels:
                host: {{ .facts.hostname }}

          positions:
            filename: /var/lib/grafana-agent/positions.yaml

          scrape_configs:
            # System logs
            - job_name: journal
              journal:
                max_age: 12h
                labels:
                  job: systemd-journal
              relabel_configs:
                - source_labels: ['__journal__systemd_unit']
                  target_label: 'unit'
                - source_labels: ['__journal_priority_keyword']
                  target_label: 'level'

            # Application logs
            - job_name: varlog
              static_configs:
                - targets: [localhost]
                  labels:
                    job: varlog
                    __path__: /var/log/*.log
              pipeline_stages:
                - regex:
                    expression: '(?P<timestamp>\S+) (?P<level>\w+) (?P<message>.*)'
                - labels:
                    level:
                - timestamp:
                    source: timestamp
                    format: RFC3339

            # Nginx access logs
            - job_name: nginx
              static_configs:
                - targets: [localhost]
                  labels:
                    job: nginx
                    __path__: /var/log/nginx/access.log
              pipeline_stages:
                - regex:
                    expression: '^(?P<remote_addr>\S+) - (?P<remote_user>\S+) \[(?P<time_local>[^\]]+)\] "(?P<request>[^"]*)" (?P<status>\d+) (?P<body_bytes_sent>\d+)'
                - labels:
                    status:

    integrations:
      node_exporter:
        enabled: false  # Using standalone node_exporter

      agent:
        enabled: true
  mode: "0640"
  owner: root
  group: grafana-agent
  require:
    - grafana_agent_config_dir
    - grafana_agent_data_dir

grafana_agent_service:
  module: service
  state: running
  name: grafana-agent
  enabled: true
  require:
    - grafana_agent_download
    - grafana_agent_config
  watch:
    - grafana_agent_config

# --- Firewall Rules ---

node_exporter_firewall:
  module: firewall
  state: allow
  port: 9100
  protocol: tcp
  source: "{{ .vars.prometheus_cidr | default \"10.0.0.0/8\" }}"
```

### Example 7: VPN Server Deployment

Deploy a WireGuard VPN server with client management.

```yaml
# wireguard-server.yaml - WireGuard VPN Server
#
# This example demonstrates:
# - WireGuard installation
# - Key generation
# - Server configuration
# - Client configuration generation
# - Firewall and routing setup
# - NAT configuration

# Variables:
# .vars.vpn_subnet: 10.200.200.0/24
# .vars.vpn_port: 51820
# .vars.vpn_interface: wg0
# .vars.clients: [{name: "laptop", public_key: "xxx", allowed_ips: "10.200.200.2/32"}]

# --- WireGuard Installation ---

wireguard_packages:
  module: package
  state: installed
  names:
    - wireguard
    - wireguard-tools
    - qrencode  # For generating QR codes

# --- Key Generation ---

wireguard_keys_dir:
  module: file
  state: directory
  path: /etc/wireguard/keys
  mode: "0700"
  owner: root
  group: root
  require:
    - wireguard_packages

server_private_key:
  module: cmd
  state: run
  name: generate_server_key
  command: |
    wg genkey | tee /etc/wireguard/keys/server.key | wg pubkey > /etc/wireguard/keys/server.pub
    chmod 600 /etc/wireguard/keys/server.key
    chmod 644 /etc/wireguard/keys/server.pub
  creates: /etc/wireguard/keys/server.key
  require:
    - wireguard_keys_dir

# --- Server Configuration ---

wireguard_config:
  module: file
  state: managed
  path: /etc/wireguard/{{ .vars.vpn_interface }}.conf
  content: |
    [Interface]
    Address = {{ .vars.vpn_subnet | splitList "/" | first }}1/{{ .vars.vpn_subnet | splitList "/" | last }}
    ListenPort = {{ .vars.vpn_port }}
    PrivateKey = {{ readFile "/etc/wireguard/keys/server.key" | trim }}

    # Enable IP forwarding
    PostUp = sysctl -w net.ipv4.ip_forward=1
    PostUp = sysctl -w net.ipv6.conf.all.forwarding=1

    # NAT for VPN clients
    PostUp = iptables -t nat -A POSTROUTING -s {{ .vars.vpn_subnet }} -o {{ .facts.default_interface }} -j MASQUERADE
    PostUp = iptables -A FORWARD -i %i -j ACCEPT
    PostUp = iptables -A FORWARD -o %i -j ACCEPT

    PostDown = iptables -t nat -D POSTROUTING -s {{ .vars.vpn_subnet }} -o {{ .facts.default_interface }} -j MASQUERADE
    PostDown = iptables -D FORWARD -i %i -j ACCEPT
    PostDown = iptables -D FORWARD -o %i -j ACCEPT

    # Client configurations
    {{- range $client := .vars.clients }}

    [Peer]
    # {{ $client.name }}
    PublicKey = {{ $client.public_key }}
    AllowedIPs = {{ $client.allowed_ips }}
    {{- if $client.preshared_key }}
    PresharedKey = {{ $client.preshared_key }}
    {{- end }}
    {{- end }}
  mode: "0600"
  owner: root
  group: root
  require:
    - server_private_key

# --- IP Forwarding (persistent) ---

ip_forwarding:
  module: sysctl
  state: present
  settings:
    net.ipv4.ip_forward: 1
    net.ipv6.conf.all.forwarding: 1

# --- Firewall Configuration ---

wireguard_firewall:
  module: firewall
  state: allow
  port: "{{ .vars.vpn_port }}"
  protocol: udp

# --- Service Management ---

wireguard_service:
  module: service
  state: running
  name: "wg-quick@{{ .vars.vpn_interface }}"
  enabled: true
  require:
    - wireguard_config
    - ip_forwarding
  watch:
    - wireguard_config

# --- Client Configuration Generation ---

client_configs_dir:
  module: file
  state: directory
  path: /etc/wireguard/clients
  mode: "0700"
  owner: root
  group: root

# Generate client configuration files
generate_client_configs:
  module: cmd
  state: run
  name: generate_client_configs
  command: |
    SERVER_PUBLIC_KEY=$(cat /etc/wireguard/keys/server.pub)
    SERVER_ENDPOINT="{{ .facts.public_ip }}:{{ .vars.vpn_port }}"

    {{- range $idx, $client := .vars.clients }}
    cat > /etc/wireguard/clients/{{ $client.name }}.conf << EOF
    [Interface]
    PrivateKey = <CLIENT_PRIVATE_KEY>
    Address = {{ $client.allowed_ips | splitList "/" | first }}/32
    DNS = {{ $.vars.dns_servers | default "1.1.1.1, 8.8.8.8" }}

    [Peer]
    PublicKey = ${SERVER_PUBLIC_KEY}
    Endpoint = ${SERVER_ENDPOINT}
    AllowedIPs = 0.0.0.0/0, ::/0
    PersistentKeepalive = 25
    EOF

    # Generate QR code for mobile
    qrencode -t ansiutf8 < /etc/wireguard/clients/{{ $client.name }}.conf > /etc/wireguard/clients/{{ $client.name }}.qr

    {{- end }}
  require:
    - server_private_key
    - client_configs_dir
  watch:
    - wireguard_config

# --- Monitoring Script ---

wireguard_status_script:
  module: file
  state: managed
  path: /usr/local/bin/wg-status.sh
  content: |
    #!/bin/bash
    # WireGuard status check script

    echo "=== WireGuard Status ==="
    wg show {{ .vars.vpn_interface }}

    echo ""
    echo "=== Connected Peers ==="
    wg show {{ .vars.vpn_interface }} latest-handshakes | while read peer handshake; do
        if [ "$handshake" != "0" ]; then
            last_seen=$(($(date +%s) - handshake))
            if [ $last_seen -lt 180 ]; then
                status="ONLINE"
            else
                status="OFFLINE ($last_seen seconds ago)"
            fi
            echo "$peer: $status"
        else
            echo "$peer: NEVER CONNECTED"
        fi
    done

    echo ""
    echo "=== Traffic Statistics ==="
    wg show {{ .vars.vpn_interface }} transfer
  mode: "0755"
  owner: root
  group: root
```

### Example 8: Mail Server Configuration

Deploy Postfix with DKIM, SPF, and DMARC.

```yaml
# mail-server.yaml - Production Mail Server (Postfix + Dovecot)
#
# This example demonstrates:
# - Postfix MTA configuration
# - TLS certificate management
# - DKIM signing
# - SPF and DMARC
# - Spam filtering with rspamd
# - Virtual domain and mailbox management

# Variables:
# .vars.mail_domain: example.com
# .vars.mail_hostname: mail.example.com
# .vars.postmaster_email: postmaster@example.com

# --- Package Installation ---

mail_packages:
  module: package
  state: installed
  names:
    - postfix
    - postfix-pcre
    - dovecot-core
    - dovecot-imapd
    - dovecot-lmtpd
    - opendkim
    - opendkim-tools
    - rspamd
    - redis-server
    - certbot

# --- SSL Certificate ---

mail_ssl_cert:
  module: cmd
  state: run
  name: obtain_mail_cert
  command: |
    certbot certonly --standalone -d {{ .vars.mail_hostname }} \
      --non-interactive --agree-tos --email {{ .vars.postmaster_email }}
  creates: /etc/letsencrypt/live/{{ .vars.mail_hostname }}/fullchain.pem
  require:
    - mail_packages

# --- Postfix Main Configuration ---

postfix_main_cf:
  module: file
  state: managed
  path: /etc/postfix/main.cf
  content: |
    # Basic settings
    smtpd_banner = $myhostname ESMTP
    biff = no
    append_dot_mydomain = no
    readme_directory = no
    compatibility_level = 3.6

    # Hostname and domain
    myhostname = {{ .vars.mail_hostname }}
    mydomain = {{ .vars.mail_domain }}
    myorigin = $mydomain
    mydestination = localhost

    # Network
    mynetworks = 127.0.0.0/8 [::ffff:127.0.0.0]/104 [::1]/128
    inet_interfaces = all
    inet_protocols = all

    # Virtual domains
    virtual_mailbox_domains = {{ .vars.mail_domain }}
    virtual_mailbox_base = /var/mail/vhosts
    virtual_mailbox_maps = hash:/etc/postfix/vmailbox
    virtual_alias_maps = hash:/etc/postfix/virtual
    virtual_uid_maps = static:5000
    virtual_gid_maps = static:5000

    # TLS settings
    smtpd_tls_cert_file = /etc/letsencrypt/live/{{ .vars.mail_hostname }}/fullchain.pem
    smtpd_tls_key_file = /etc/letsencrypt/live/{{ .vars.mail_hostname }}/privkey.pem
    smtpd_tls_security_level = may
    smtpd_tls_auth_only = yes
    smtpd_tls_protocols = !SSLv2, !SSLv3, !TLSv1, !TLSv1.1
    smtpd_tls_ciphers = high
    smtpd_tls_mandatory_ciphers = high
    smtpd_tls_session_cache_database = btree:${data_directory}/smtpd_scache

    smtp_tls_security_level = may
    smtp_tls_session_cache_database = btree:${data_directory}/smtp_scache
    smtp_tls_protocols = !SSLv2, !SSLv3, !TLSv1, !TLSv1.1

    # SASL authentication
    smtpd_sasl_type = dovecot
    smtpd_sasl_path = private/auth
    smtpd_sasl_auth_enable = yes
    smtpd_sasl_security_options = noanonymous, noplaintext
    smtpd_sasl_tls_security_options = noanonymous

    # Restrictions
    smtpd_helo_required = yes
    smtpd_helo_restrictions =
        permit_mynetworks,
        permit_sasl_authenticated,
        reject_invalid_helo_hostname,
        reject_non_fqdn_helo_hostname

    smtpd_sender_restrictions =
        permit_mynetworks,
        permit_sasl_authenticated,
        reject_non_fqdn_sender,
        reject_unknown_sender_domain

    smtpd_recipient_restrictions =
        permit_mynetworks,
        permit_sasl_authenticated,
        reject_unauth_destination,
        reject_non_fqdn_recipient,
        reject_unknown_recipient_domain

    # Content filtering (rspamd)
    smtpd_milters = inet:localhost:11332
    non_smtpd_milters = inet:localhost:11332
    milter_protocol = 6
    milter_mail_macros = i {mail_addr} {client_addr} {client_name} {auth_authen}
    milter_default_action = accept

    # DKIM
    milter_connect_macros = i j {daemon_name} v {if_name} _

    # Delivery
    virtual_transport = lmtp:unix:private/dovecot-lmtp

    # Limits
    message_size_limit = 52428800
    mailbox_size_limit = 0
  mode: "0644"
  require:
    - mail_ssl_cert

# --- DKIM Configuration ---

dkim_key_dir:
  module: file
  state: directory
  path: /etc/opendkim/keys/{{ .vars.mail_domain }}
  owner: opendkim
  group: opendkim
  mode: "0700"

generate_dkim_key:
  module: cmd
  state: run
  name: generate_dkim
  command: |
    cd /etc/opendkim/keys/{{ .vars.mail_domain }}
    opendkim-genkey -s mail -d {{ .vars.mail_domain }}
    chown opendkim:opendkim mail.private mail.txt
  creates: /etc/opendkim/keys/{{ .vars.mail_domain }}/mail.private
  require:
    - dkim_key_dir

opendkim_conf:
  module: file
  state: managed
  path: /etc/opendkim.conf
  content: |
    Syslog                  yes
    SyslogSuccess           yes
    LogWhy                  yes

    Canonicalization        relaxed/simple
    Mode                    sv
    SubDomains              no

    OversignHeaders         From

    AutoRestart             yes
    AutoRestartRate         10/1M

    Background              yes
    DNSTimeout              5
    SignatureAlgorithm      rsa-sha256

    UserID                  opendkim
    UMask                   007

    Socket                  inet:8891@localhost

    PidFile                 /run/opendkim/opendkim.pid

    TrustAnchorFile         /usr/share/dns/root.key

    KeyTable                /etc/opendkim/key.table
    SigningTable            refile:/etc/opendkim/signing.table
    InternalHosts           /etc/opendkim/trusted.hosts
  mode: "0644"
  require:
    - mail_packages

opendkim_key_table:
  module: file
  state: managed
  path: /etc/opendkim/key.table
  content: |
    mail._domainkey.{{ .vars.mail_domain }} {{ .vars.mail_domain }}:mail:/etc/opendkim/keys/{{ .vars.mail_domain }}/mail.private
  mode: "0644"
  require:
    - generate_dkim_key

opendkim_signing_table:
  module: file
  state: managed
  path: /etc/opendkim/signing.table
  content: |
    *@{{ .vars.mail_domain }} mail._domainkey.{{ .vars.mail_domain }}
  mode: "0644"

opendkim_trusted_hosts:
  module: file
  state: managed
  path: /etc/opendkim/trusted.hosts
  content: |
    127.0.0.1
    localhost
    {{ .vars.mail_hostname }}
    .{{ .vars.mail_domain }}
  mode: "0644"

# --- Rspamd Configuration ---

rspamd_local_conf:
  module: file
  state: managed
  path: /etc/rspamd/local.d/worker-normal.inc
  content: |
    bind_socket = "localhost:11333";
  mode: "0644"

rspamd_proxy_conf:
  module: file
  state: managed
  path: /etc/rspamd/local.d/worker-proxy.inc
  content: |
    bind_socket = "localhost:11332";
    milter = yes;
    timeout = 120s;
    upstream "local" {
      default = yes;
      self_scan = yes;
    }
  mode: "0644"

# --- Service Management ---

redis_service:
  module: service
  state: running
  name: redis-server
  enabled: true
  require:
    - mail_packages

opendkim_service:
  module: service
  state: running
  name: opendkim
  enabled: true
  require:
    - opendkim_conf
    - opendkim_key_table
  watch:
    - opendkim_conf

rspamd_service:
  module: service
  state: running
  name: rspamd
  enabled: true
  require:
    - redis_service
    - rspamd_local_conf
  watch:
    - rspamd_local_conf
    - rspamd_proxy_conf

postfix_service:
  module: service
  state: running
  name: postfix
  enabled: true
  require:
    - postfix_main_cf
    - opendkim_service
    - rspamd_service
  watch:
    - postfix_main_cf

# --- Firewall ---

mail_firewall_smtp:
  module: firewall
  state: allow
  port: 25
  protocol: tcp

mail_firewall_submission:
  module: firewall
  state: allow
  port: 587
  protocol: tcp

mail_firewall_imaps:
  module: firewall
  state: allow
  port: 993
  protocol: tcp
```

### Example 9: CI/CD Build Agent

Configure a build agent for CI/CD pipelines.

```yaml
# build-agent.yaml - CI/CD Build Agent Configuration
#
# This example demonstrates:
# - Build tools installation
# - Docker configuration for builds
# - Language runtime setup (Node, Python, Go)
# - Caching directories
# - Agent service configuration

# Variables:
# .vars.agent_name: build-agent-01
# .vars.ci_server_url: https://ci.example.com
# .vars.agent_token: secret-token
# .vars.node_versions: ["18", "20"]
# .vars.python_versions: ["3.10", "3.11", "3.12"]
# .vars.go_version: "1.22"

# --- Build User ---

build_user:
  module: user
  state: present
  name: builder
  groups:
    - docker
  shell: /bin/bash
  home: /home/builder

builder_ssh_dir:
  module: file
  state: directory
  path: /home/builder/.ssh
  owner: builder
  group: builder
  mode: "0700"
  require:
    - build_user

# --- Base Build Tools ---

build_packages:
  module: package
  state: installed
  names:
    - build-essential
    - git
    - curl
    - wget
    - jq
    - unzip
    - zip
    - rsync
    - make
    - cmake
    - pkg-config
    - libssl-dev
    - libffi-dev
    - zlib1g-dev
    - libbz2-dev
    - libreadline-dev
    - libsqlite3-dev
    - libncurses5-dev
    - libxml2-dev
    - libxmlsec1-dev
    - liblzma-dev

# --- Docker Installation ---

docker_install:
  module: package
  state: installed
  names:
    - docker.io
    - docker-buildx-plugin
    - docker-compose-plugin

docker_service:
  module: service
  state: running
  name: docker
  enabled: true
  require:
    - docker_install

docker_config:
  module: file
  state: managed
  path: /etc/docker/daemon.json
  content: |
    {
      "storage-driver": "overlay2",
      "log-driver": "json-file",
      "log-opts": {
        "max-size": "100m",
        "max-file": "3"
      },
      "default-ulimits": {
        "nofile": {
          "Name": "nofile",
          "Hard": 65536,
          "Soft": 65536
        }
      },
      "insecure-registries": [],
      "registry-mirrors": []
    }
  mode: "0644"
  require:
    - docker_install

docker_reload:
  module: service
  state: running
  name: docker
  reload: true
  watch:
    - docker_config

# --- Node.js (via nvm) ---

nvm_install:
  module: cmd
  state: run
  name: install_nvm
  user: builder
  command: |
    curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.7/install.sh | bash
  creates: /home/builder/.nvm/nvm.sh
  require:
    - build_user

node_versions:
  module: cmd
  state: run
  name: install_node_versions
  user: builder
  command: |
    export NVM_DIR="$HOME/.nvm"
    [ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"
    {{- range .vars.node_versions }}
    nvm install {{ . }}
    {{- end }}
    nvm alias default {{ index .vars.node_versions 0 }}
  require:
    - nvm_install

# --- Python (via pyenv) ---

pyenv_install:
  module: cmd
  state: run
  name: install_pyenv
  user: builder
  command: |
    curl https://pyenv.run | bash
    echo 'export PYENV_ROOT="$HOME/.pyenv"' >> ~/.bashrc
    echo 'command -v pyenv >/dev/null || export PATH="$PYENV_ROOT/bin:$PATH"' >> ~/.bashrc
    echo 'eval "$(pyenv init -)"' >> ~/.bashrc
  creates: /home/builder/.pyenv/bin/pyenv
  require:
    - build_packages
    - build_user

python_versions:
  module: cmd
  state: run
  name: install_python_versions
  user: builder
  command: |
    export PYENV_ROOT="$HOME/.pyenv"
    export PATH="$PYENV_ROOT/bin:$PATH"
    eval "$(pyenv init -)"
    {{- range .vars.python_versions }}
    pyenv install -s {{ . }}
    {{- end }}
    pyenv global {{ index .vars.python_versions 0 }}
  require:
    - pyenv_install

# --- Go ---

go_install:
  module: cmd
  state: run
  name: install_go
  command: |
    curl -sSL https://go.dev/dl/go{{ .vars.go_version }}.linux-amd64.tar.gz | tar -C /usr/local -xzf -
  creates: /usr/local/go/bin/go

go_path:
  module: file
  state: managed
  path: /etc/profile.d/go.sh
  content: |
    export GOROOT=/usr/local/go
    export PATH=$PATH:$GOROOT/bin
  mode: "0644"
  require:
    - go_install

builder_go_path:
  module: file
  state: managed
  path: /home/builder/.bashrc
  pattern: "# GO PATH"
  append: true
  content: |

    # GO PATH
    export GOROOT=/usr/local/go
    export GOPATH=$HOME/go
    export PATH=$PATH:$GOROOT/bin:$GOPATH/bin
  owner: builder
  group: builder
  require:
    - build_user
    - go_install

# --- Build Caches ---

cache_dirs:
  module: file
  state: directory
  path: "{{ .item }}"
  owner: builder
  group: builder
  mode: "0755"
  loop:
    - /home/builder/.cache
    - /home/builder/.cache/pip
    - /home/builder/.cache/npm
    - /home/builder/.cache/go-build
    - /home/builder/.cache/docker
  require:
    - build_user

# --- Git Configuration ---

builder_git_config:
  module: git_config
  state: present
  scope: global
  user: builder
  settings:
    user.name: "CI Builder"
    user.email: "ci@{{ .vars.domain | default \"example.com\" }}"
    init.defaultBranch: main
    core.autocrlf: input
    credential.helper: store
  require:
    - build_user

# --- GitHub/GitLab CLI ---

gh_cli:
  module: cmd
  state: run
  name: install_gh_cli
  command: |
    curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | tee /etc/apt/sources.list.d/github-cli.list
    apt update && apt install -y gh
  creates: /usr/bin/gh

# --- CI Agent Service (example: GitLab Runner) ---

gitlab_runner_install:
  module: cmd
  state: run
  name: install_gitlab_runner
  command: |
    curl -L --output /usr/local/bin/gitlab-runner "https://gitlab-runner-downloads.s3.amazonaws.com/latest/binaries/gitlab-runner-linux-amd64"
    chmod +x /usr/local/bin/gitlab-runner
    gitlab-runner install --user=builder --working-directory=/home/builder
  creates: /usr/local/bin/gitlab-runner
  require:
    - build_user
    - docker_service

gitlab_runner_register:
  module: cmd
  state: run
  name: register_gitlab_runner
  command: |
    gitlab-runner register \
      --non-interactive \
      --url "{{ .vars.ci_server_url }}" \
      --registration-token "{{ .vars.agent_token }}" \
      --executor "docker" \
      --docker-image "alpine:latest" \
      --description "{{ .vars.agent_name }}" \
      --tag-list "docker,linux,{{ .facts.architecture }}" \
      --docker-privileged \
      --docker-volumes "/var/run/docker.sock:/var/run/docker.sock" \
      --docker-volumes "/cache:/cache"
  creates: /etc/gitlab-runner/config.toml
  require:
    - gitlab_runner_install

gitlab_runner_service:
  module: service
  state: running
  name: gitlab-runner
  enabled: true
  require:
    - gitlab_runner_register

# --- Resource Limits ---

builder_limits:
  module: file
  state: managed
  path: /etc/security/limits.d/builder.conf
  content: |
    builder soft nofile 65536
    builder hard nofile 65536
    builder soft nproc 65536
    builder hard nproc 65536
  mode: "0644"

# --- Cleanup Cron ---

docker_cleanup_cron:
  module: cron
  state: present
  name: docker-cleanup
  user: root
  hour: 3
  minute: 0
  job: docker system prune -af --volumes --filter "until=168h"
  require:
    - docker_service

cache_cleanup_cron:
  module: cron
  state: present
  name: cache-cleanup
  user: builder
  hour: 4
  minute: 0
  weekday: 0
  job: find /home/builder/.cache -type f -mtime +7 -delete
```

### Example 10: Log Aggregation (Loki Stack)

Deploy a log aggregation stack with Loki, Promtail, and Grafana.

```yaml
# loki-stack.yaml - Log Aggregation with Loki
#
# This example demonstrates:
# - Loki installation and configuration
# - Promtail log collector
# - Grafana data source configuration
# - S3-compatible storage backend
# - Retention policies

# Variables:
# .vars.loki_version: "2.9.4"
# .vars.s3_bucket: loki-logs
# .vars.s3_endpoint: s3.amazonaws.com
# .vars.retention_days: 30

# --- System Users ---

loki_user:
  module: user
  state: present
  name: loki
  system: true
  shell: /usr/sbin/nologin
  home: /var/lib/loki

promtail_user:
  module: user
  state: present
  name: promtail
  system: true
  shell: /usr/sbin/nologin
  groups:
    - adm  # For reading log files
    - systemd-journal  # For reading journal

# --- Directory Structure ---

loki_dirs:
  module: file
  state: directory
  path: "{{ .item.path }}"
  owner: "{{ .item.owner }}"
  group: "{{ .item.owner }}"
  mode: "{{ .item.mode }}"
  loop:
    - {path: /etc/loki, owner: root, mode: "0755"}
    - {path: /var/lib/loki, owner: loki, mode: "0755"}
    - {path: /var/lib/loki/chunks, owner: loki, mode: "0755"}
    - {path: /var/lib/loki/rules, owner: loki, mode: "0755"}
    - {path: /etc/promtail, owner: root, mode: "0755"}
    - {path: /var/lib/promtail, owner: promtail, mode: "0755"}
  require:
    - loki_user
    - promtail_user

# --- Loki Installation ---

loki_download:
  module: cmd
  state: run
  name: download_loki
  command: |
    curl -sSL -o /tmp/loki.zip \
      "https://github.com/grafana/loki/releases/download/v{{ .vars.loki_version }}/loki-linux-amd64.zip"
    unzip -o /tmp/loki.zip -d /usr/local/bin/
    chmod +x /usr/local/bin/loki-linux-amd64
    ln -sf /usr/local/bin/loki-linux-amd64 /usr/local/bin/loki
  creates: /usr/local/bin/loki

promtail_download:
  module: cmd
  state: run
  name: download_promtail
  command: |
    curl -sSL -o /tmp/promtail.zip \
      "https://github.com/grafana/loki/releases/download/v{{ .vars.loki_version }}/promtail-linux-amd64.zip"
    unzip -o /tmp/promtail.zip -d /usr/local/bin/
    chmod +x /usr/local/bin/promtail-linux-amd64
    ln -sf /usr/local/bin/promtail-linux-amd64 /usr/local/bin/promtail
  creates: /usr/local/bin/promtail

# --- Loki Configuration ---

loki_config:
  module: file
  state: managed
  path: /etc/loki/config.yaml
  content: |
    auth_enabled: false

    server:
      http_listen_port: 3100
      grpc_listen_port: 9096
      log_level: info

    common:
      path_prefix: /var/lib/loki
      storage:
        filesystem:
          chunks_directory: /var/lib/loki/chunks
          rules_directory: /var/lib/loki/rules
      replication_factor: 1
      ring:
        instance_addr: 127.0.0.1
        kvstore:
          store: inmemory

    query_range:
      results_cache:
        cache:
          embedded_cache:
            enabled: true
            max_size_mb: 100

    schema_config:
      configs:
        - from: 2024-01-01
          store: boltdb-shipper
          object_store: filesystem
          schema: v12
          index:
            prefix: index_
            period: 24h

    ruler:
      alertmanager_url: http://localhost:9093
      storage:
        type: local
        local:
          directory: /var/lib/loki/rules
      rule_path: /var/lib/loki/rules-temp
      enable_api: true

    limits_config:
      retention_period: {{ .vars.retention_days }}d
      enforce_metric_name: false
      reject_old_samples: true
      reject_old_samples_max_age: 168h
      ingestion_rate_mb: 16
      ingestion_burst_size_mb: 24
      max_streams_per_user: 10000
      max_line_size: 256kb

    compactor:
      working_directory: /var/lib/loki/compactor
      shared_store: filesystem
      retention_enabled: true
      retention_delete_delay: 2h
      retention_delete_worker_count: 150

    analytics:
      reporting_enabled: false
  mode: "0644"
  require:
    - loki_dirs

# --- Promtail Configuration ---

promtail_config:
  module: file
  state: managed
  path: /etc/promtail/config.yaml
  content: |
    server:
      http_listen_port: 9080
      grpc_listen_port: 0

    positions:
      filename: /var/lib/promtail/positions.yaml

    clients:
      - url: http://localhost:3100/loki/api/v1/push
        tenant_id: default

    scrape_configs:
      # Systemd journal
      - job_name: journal
        journal:
          max_age: 12h
          labels:
            job: systemd-journal
            host: {{ .facts.hostname }}
        relabel_configs:
          - source_labels: ['__journal__systemd_unit']
            target_label: 'unit'
          - source_labels: ['__journal_priority_keyword']
            target_label: 'level'
          - source_labels: ['__journal__hostname']
            target_label: 'hostname'

      # Syslog
      - job_name: syslog
        static_configs:
          - targets:
              - localhost
            labels:
              job: syslog
              host: {{ .facts.hostname }}
              __path__: /var/log/syslog
        pipeline_stages:
          - regex:
              expression: '^(?P<timestamp>\w+\s+\d+\s+\d+:\d+:\d+)\s+(?P<hostname>\S+)\s+(?P<service>\S+?)(\[(?P<pid>\d+)\])?:\s+(?P<message>.*)$'
          - labels:
              service:
          - timestamp:
              source: timestamp
              format: "Jan _2 15:04:05"

      # Auth logs
      - job_name: auth
        static_configs:
          - targets:
              - localhost
            labels:
              job: auth
              host: {{ .facts.hostname }}
              __path__: /var/log/auth.log
        pipeline_stages:
          - regex:
              expression: '^(?P<timestamp>\w+\s+\d+\s+\d+:\d+:\d+)\s+(?P<hostname>\S+)\s+(?P<service>\S+?)(\[(?P<pid>\d+)\])?:\s+(?P<message>.*)$'
          - labels:
              service:

      # Nginx access logs
      - job_name: nginx_access
        static_configs:
          - targets:
              - localhost
            labels:
              job: nginx
              type: access
              host: {{ .facts.hostname }}
              __path__: /var/log/nginx/access.log
        pipeline_stages:
          - regex:
              expression: '^(?P<remote_addr>\S+) - (?P<remote_user>\S+) \[(?P<time_local>[^\]]+)\] "(?P<method>\S+) (?P<request>\S+) (?P<protocol>\S+)" (?P<status>\d+) (?P<body_bytes_sent>\d+) "(?P<http_referer>[^"]*)" "(?P<http_user_agent>[^"]*)"'
          - labels:
              method:
              status:
          - metrics:
              http_requests_total:
                type: Counter
                description: "Total HTTP requests"
                source: status
                config:
                  action: inc

      # Nginx error logs
      - job_name: nginx_error
        static_configs:
          - targets:
              - localhost
            labels:
              job: nginx
              type: error
              host: {{ .facts.hostname }}
              __path__: /var/log/nginx/error.log
        pipeline_stages:
          - regex:
              expression: '^(?P<timestamp>\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}) \[(?P<level>\w+)\] (?P<pid>\d+)#(?P<tid>\d+): (?P<message>.*)$'
          - labels:
              level:

      # Docker container logs
      - job_name: docker
        docker_sd_configs:
          - host: unix:///var/run/docker.sock
            refresh_interval: 5s
        relabel_configs:
          - source_labels: ['__meta_docker_container_name']
            target_label: 'container'
          - source_labels: ['__meta_docker_container_log_stream']
            target_label: 'stream'
          - source_labels: ['__meta_docker_container_label_com_docker_compose_project']
            target_label: 'project'
          - source_labels: ['__meta_docker_container_label_com_docker_compose_service']
            target_label: 'service'
  mode: "0644"
  require:
    - promtail_dirs

# --- Systemd Services ---

loki_service:
  module: file
  state: managed
  path: /etc/systemd/system/loki.service
  content: |
    [Unit]
    Description=Loki Log Aggregation System
    Wants=network-online.target
    After=network-online.target

    [Service]
    User=loki
    Group=loki
    Type=simple
    ExecStart=/usr/local/bin/loki -config.file=/etc/loki/config.yaml
    Restart=always
    RestartSec=5
    LimitNOFILE=65536

    [Install]
    WantedBy=multi-user.target
  mode: "0644"
  require:
    - loki_download
    - loki_config

promtail_service:
  module: file
  state: managed
  path: /etc/systemd/system/promtail.service
  content: |
    [Unit]
    Description=Promtail Log Collector
    Wants=network-online.target
    After=network-online.target loki.service

    [Service]
    User=promtail
    Group=promtail
    Type=simple
    ExecStart=/usr/local/bin/promtail -config.file=/etc/promtail/config.yaml
    Restart=always
    RestartSec=5

    # Allow reading Docker socket
    SupplementaryGroups=docker

    [Install]
    WantedBy=multi-user.target
  mode: "0644"
  require:
    - promtail_download
    - promtail_config

loki_service_start:
  module: service
  state: running
  name: loki
  enabled: true
  daemon_reload: true
  require:
    - loki_service
  watch:
    - loki_config

promtail_service_start:
  module: service
  state: running
  name: promtail
  enabled: true
  daemon_reload: true
  require:
    - promtail_service
    - loki_service_start
  watch:
    - promtail_config

# --- Firewall ---

loki_firewall:
  module: firewall
  state: allow
  port: 3100
  protocol: tcp
  source: "{{ .vars.allowed_cidr | default \"10.0.0.0/8\" }}"
```

## See Also

- [State Management Concepts](../../concepts/state-management/) - State management overview
- [Configuration Reference](../configuration/#state-file-configuration) - State file configuration
- [CLI Reference](../cli/#kscore-state-state-management) - State CLI commands
- [CLI Reference - Module Management](../cli/#kscorectl module-module-management) - Module CLI commands
