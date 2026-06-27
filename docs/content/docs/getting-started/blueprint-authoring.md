---
title: Authoring a Blueprint
weight: 2
---

A **blueprint** packages a deployment as a reusable, parameterized unit:
one or more state files (the *entrypoints*), a manifest that declares the
parameters and a rollback path, and metadata describing compatibility and
outputs. Where a bare state file says *what this host should look like*, a
blueprint says *here is a configurable deployment you can apply, with a
rollback if it goes wrong*.

This guide scaffolds a blueprint, walks its structure, validates it, and
applies it locally. It uses the `kscore-blueprint` CLI throughout.

## Prerequisites

- Keystone Core installed (`kscore-blueprint version` works). See the
  [quick start](/docs/reference/getting-started/) for installation.
- A working directory you can write to.

## 1. Scaffold a blueprint

```sh
kscore-blueprint init my-app
```

That writes a starter blueprint:

```text
my-app/
├── blueprint.yaml   # the manifest: metadata, parameters, entrypoints
├── apply.yaml       # the default entrypoint — a state file
└── README.md
```

## 2. The manifest

`blueprint.yaml` is the contract. The key sections:

```yaml
metadata:
  name: my-app
  version: 1.0.0
  description: A single-node app deployment.

compatibility:
  min_keystone_version: 0.1.0
  platforms: [linux]

parameters:
  package_name:
    type: string
    description: OS package to install and run as a service.
    default: nginx
  app_name:
    type: string
    default: my-app

entrypoints:
  default: apply.yaml      # what `apply` runs
  rollback: rollback.yaml  # what `rollback` runs

outputs:
  summary:
    value: "deployed {{ .Params.app_name }} ({{ .Params.package_name }})"
```

- **`parameters`** are typed inputs with optional defaults; they are
  substituted into the entrypoints as `{{ .Params.<name> }}`.
- **`entrypoints`** map names to state files. `default` is applied by
  `apply`; `rollback` is applied by `rollback`.
- **`outputs`** are templated summary values surfaced after an apply.

## 3. The entrypoint

`apply.yaml` is an ordinary [state file](/docs/reference/getting-started/),
templated with the blueprint's parameters. The scaffold's looks like:

```yaml
metadata:
  name: my-app-apply
  version: "1.0"

package:
  "{{ .Params.package_name }}":
    state: installed

service:
  "{{ .Params.package_name }}":
    state: running
    require:
      - package: "{{ .Params.package_name }}"

file:
  /etc/my-app.deployed:
    state: present
    content: "{{ .Params.app_name }} deployed by a keystone-core blueprint\n"
    mode: "0644"
    require:
      - service: "{{ .Params.package_name }}"
```

The `require:` edges make the apply ordered and idempotent: the package
is installed before the service starts, and the marker file is written
last. For a fuller reference, see the
[`demo`](https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/src/branch/main/modules/examples/blueprints/demo)
and
[`postgres-ha`](https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/src/branch/main/modules/examples/blueprints/postgres-ha)
example blueprints.

## 4. Validate

Two checks, fast and offline:

```sh
kscore-blueprint validate my-app   # manifest is structurally sound
kscore-blueprint lint my-app       # manifest + every entrypoint file exists
kscore-blueprint info my-app       # human summary of the manifest
```

`info` prints what an operator sees before applying:

```text
name:        my-app
version:     1.0.0
entrypoints: default=apply.yaml rollback=rollback.yaml
parameters:  package_name app_name
```

## 5. Apply

Apply the default entrypoint locally, overriding a parameter:

```sh
kscore-blueprint apply my-app --param package_name=nginx --target local
```

- `--param key=value` overrides a declared parameter (repeatable).
- `--target local` applies on the current host. (v1.0 supports the
  `local` target; remote targeting via the control plane is on the
  roadmap.)

Each apply is recorded. List what has been applied:

```sh
kscore-blueprint applied
```

## 6. Roll back

If an apply needs undoing, the `rollback` entrypoint runs in reverse:

```sh
kscore-blueprint rollback my-app --target local
```

Author `rollback.yaml` as the inverse of `apply.yaml` (e.g. the service
`stopped`, the package `absent`, the marker file `absent`) so a rollback
returns the host to its prior state.

## 7. Distribute

Package a validated blueprint into a single artifact for sharing:

```sh
kscore-blueprint bundle my-app   # → my-app-1.0.0.tar.gz
```

Install a bundle into the local blueprint library so it can be applied by
name:

```sh
kscore-blueprint install my-app-1.0.0.tar.gz
```

## Next steps

- [Authoring & Publishing a Module](../module-authoring/) — extend the
  system with custom logic.
- The [CLI reference](/docs/reference/cli-reference/) for every
  `kscore-blueprint` flag.
