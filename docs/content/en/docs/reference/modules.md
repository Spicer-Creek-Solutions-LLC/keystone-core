---
title: "Module Reference"
weight: 4
description: >
  Complete reference for all state modules with parameters and specifications
---

## Overview

Keystone Core includes 18 built-in state modules for declarative configuration management. All modules are idempotent and cross-platform where applicable.

**Core Modules**:
- [file](#file-module) - Manage files and directories
- [package](#package-module) - Manage software packages
- [service](#service-module) - Manage system services
- [user](#user-module) - Manage user accounts
- [group](#group-module) - Manage groups
- [cmd](#cmd-module) - Execute commands

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
      -----BEGIN PRIVATE KEY-----
      MIIEvgIBADANBgkqhkiG9w...
      -----END PRIVATE KEY-----
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
      -----BEGIN OPENSSH PRIVATE KEY-----
      b3BlbnNzaC1rZXktdjEA...
      -----END OPENSSH PRIVATE KEY-----
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
kscore-module sign ./myorg/custom-state/myorg-custom-state-0.1.0.zip

# Publish to registry
kscore-module publish ./myorg/custom-state/myorg-custom-state-0.1.0.zip
```

## See Also

- [State Management Concepts](../../concepts/state-management/) - State management overview
- [Configuration Reference](../configuration/#state-file-configuration) - State file configuration
- [CLI Reference](../cli/#kscore-state-state-management) - State CLI commands
- [CLI Reference - Module Management](../cli/#kscore-module-module-management) - Module CLI commands
