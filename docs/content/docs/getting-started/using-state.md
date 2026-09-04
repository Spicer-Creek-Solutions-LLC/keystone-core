---
title: Using State Management
weight: 1
---

State management is the heart of Keystone Core: you describe the
**desired state** of a host in a YAML file, and the engine makes the host
match it — installing packages, writing files, starting services — and
reports when reality has **drifted** away. Every module is **idempotent**:
applying the same file twice changes nothing the second time.

## Which host?

Every state command takes a **required** `--target`, because "which
host" is never a safe thing to guess:

```bash
kscorectl state apply web-stack.yaml --target localhost      # the control-plane host
kscorectl state apply web-stack.yaml --target id:web-1       # one agent
kscorectl state apply web-stack.yaml --target role:web       # every agent with role=web
kscorectl state apply web-stack.yaml --target hostname:web-* # by hostname glob
```

A targeted run is compiled **on each agent**, against that agent's own
facts — so `{{ .Facts.os }}` means the OS of the host being converged,
not the control plane's. Results come back attributed per host, and one
unreachable host does not stop the others.

`--agent` is a different flag: it records a run against a host for
history and audit. It does **not** choose where the run executes. Use
`--target`.

This guide covers the state-file language, requisites (ordering and
reactivity), and the apply → check → drift → rollback workflow. For the
full list of what you can manage, see the
[State Modules](../../modules/) reference.

## The state file

A state file is Salt-style YAML. Top-level keys name a **module**; under
each, keys name a **resource** (usually a path or other natural key); under
each resource, `state:` selects the desired state and the remaining keys
are that module's parameters:

```yaml
metadata:
  name: hello          # a label for this state file
  version: "1.0"

file:                  # module
  /tmp/keystone-hello: # resource (the file path)
    state: present     # desired state
    content: "hello from keystone-core\n"
    mode: "0644"
```

`metadata` is optional bookkeeping. Everything else is a module block. A
single file can mix as many modules and resources as you like.

## Modules

Each module manages one kind of resource. A few common ones:

| Module | Manages | States |
| --- | --- | --- |
| [`file`](../../modules/file/) | files, directories, symlinks | `present`, `directory`, `symlink`, `absent` |
| [`package`](../../modules/package/) | OS packages (apt/dnf/apk/zypper/pacman) | `installed`, `absent` |
| [`service`](../../modules/service/) | init services (systemd/OpenRC/sysvinit) | `running`, `stopped` |
| [`user`](../../modules/user/) | user accounts | `present`, `absent` |

The [State Modules](../../modules/) reference documents every module's
states, parameters, and examples.

## Requisites: ordering and reactivity

Requisites are what make this a state *engine* rather than a script. They
declare relationships between resources; the engine builds a dependency
graph, runs a topological sort (rejecting cycles), and applies in the
right order.

- **`require`** — apply this resource only after another succeeds.
- **`watch`** — like `require`, but also re-act (e.g. restart a service)
  when the watched resource *changes*.
- **`onchanges`** — act only if the named resource reported a change.

Each takes a list of `{<module>: <resource>}` references. There are also
`_in` reverse forms (`require_in`, `watch_in`, `onchanges_in`) that attach
the relationship from the other side.

## A worked example

Deploy a small web service — create its user, install the package, write
a config, and run the service so it restarts whenever the config changes:

```yaml
metadata:
  name: web-stack
  version: "1.0"

user:
  www-app:
    state: present
    system: true
    shell: /usr/sbin/nologin

package:
  nginx:
    state: installed

file:
  /etc/nginx/conf.d/app.conf:
    state: present
    mode: "0644"
    content: |
      server { listen 8080; root /srv/app; }
    require:
      - package: nginx        # write the config only after nginx installs

service:
  nginx:
    state: running
    enable: true              # start on boot
    watch:
      - file: /etc/nginx/conf.d/app.conf   # restart when the config changes
```

The engine orders the work (`package` → `file` → `service`) from the
requisites — you don't list steps, you declare relationships.

## The workflow

All commands run through `kscorectl state` and target an agent with
`--agent <id>` (or a whole cluster with `--cluster <name>`). The state
file is a positional argument.

```bash
# Apply — converge the host to the file (idempotent)
kscorectl state apply web-stack.yaml --target id:web-1

# Check — dry-run: report what WOULD change, change nothing
kscorectl state check web-stack.yaml --target id:web-1

# Drift — report resources that have drifted from the file
kscorectl state drift web-stack.yaml --target id:web-1
#   …add --fix to re-apply and remediate the drift:
kscorectl state drift web-stack.yaml --target id:web-1 --fix
```

Every apply is recorded, so you can audit and roll back:

```bash
# History — past runs (run-id, time, agent, result)
kscorectl state history --agent web-1

# Show — the detailed result of one run
kscorectl state show <run-id>

# Rollback — revert a run, restoring the prior state
kscorectl state rollback <run-id>
```

### Variables and facts

Parameterize a file with `--variable key=value` and reference host
**facts** (gathered from the agent) in templates. `kscorectl state compile
<file>` renders the file with variables/facts resolved so you can inspect
the result before applying.

## Next steps

- Browse every module in the [State Modules](../../modules/) reference.
- Package a multi-resource deployment as a reusable unit in
  [Authoring a Blueprint](../blueprint-authoring/).
- See complete, runnable state files in the
  [Examples](../../examples/) section.
