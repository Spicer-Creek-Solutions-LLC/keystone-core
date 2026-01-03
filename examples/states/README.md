# Keystone Core State File Examples

This directory contains example state files demonstrating various Keystone Core state management capabilities.

## Examples

| File | Description | Modules Used |
|------|-------------|--------------|
| [webserver.yaml](webserver.yaml) | Configure a production Nginx web server | package, file, service |
| [python-app.yaml](python-app.yaml) | Deploy a Python Flask application | user, file, pip, service, cmd |
| [firewall.yaml](firewall.yaml) | Configure UFW firewall rules | package, ufw, cmd |
| [scheduled-tasks.yaml](scheduled-tasks.yaml) | Set up cron jobs for maintenance | file, cron |
| [kubernetes-app.yaml](kubernetes-app.yaml) | Deploy an application to Kubernetes | k8s_namespace, k8s_deployment, k8s_service, k8s_configmap, k8s_secret, k8s_ingress, k8s_hpa, k8s_pvc |
| [docker-stack.yaml](docker-stack.yaml) | Deploy a Docker container stack | docker_network, docker_volume, docker_image, docker_container |

## Usage

Apply a state file:

```bash
kscorectl state apply webserver.yaml
```

Check what would change (dry-run):

```bash
kscorectl state check webserver.yaml
```

Detect drift from desired state:

```bash
kscorectl state drift webserver.yaml
```

Apply with custom variables:

```bash
kscorectl state apply webserver.yaml --vars vars/production.yaml
```

## State File Structure

Each state file follows this structure:

```yaml
# Metadata about the state file
metadata:
  name: example-state
  description: What this state file does
  version: "1.0"

# Variables for customization
vars:
  variable_name: value
  another_var: "{{ some_template }}"

# State declarations
states:
  unique_state_id:
    module: module_name
    state: desired_state
    # Module-specific parameters
    name: resource_name
    # Dependencies
    require:
      - other_state_id
    watch:
      - config_state_id
```

## Key Concepts

### Variables and Templating

Variables can be defined in the `vars` section and used with `{{ variable_name }}` syntax:

```yaml
vars:
  app_name: myapp
  port: 8080

states:
  deploy:
    module: file
    state: present
    name: /opt/{{ app_name }}/config.yaml
```

### Dependencies (Requisites)

Control execution order with requisites:

- `require` - This state needs the listed states to succeed first
- `watch` - Like require, but also triggers reload if watched states change
- `prereq` - This state is a prerequisite for the listed states
- `onchanges` - Only run if the listed states made changes

```yaml
states:
  install_package:
    module: package
    state: installed
    name: nginx

  configure:
    module: file
    state: present
    name: /etc/nginx/nginx.conf
    require:
      - install_package

  start_service:
    module: service
    state: running
    name: nginx
    watch:
      - configure
```

### Idempotency

All modules are idempotent - running them multiple times produces the same result. The `check` command shows what would change without making modifications.

## Module Reference

For complete documentation of all available modules and their parameters, see the [Module Reference](../../docs/content/en/docs/reference/modules.md).

## More Examples

Additional examples can be found in the [documentation](../../docs/content/en/docs/):

- [Getting Started](../../docs/content/en/docs/getting-started/)
- [Core Concepts - State Management](../../docs/content/en/docs/concepts/state-management.md)
- [Reference - Modules](../../docs/content/en/docs/reference/modules.md)
