---
title: Examples
weight: 3
---

Runnable examples ship in the repository under
[`modules/examples/`](https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/src/branch/main/modules/examples).
Each is self-contained — clone the repo and run it against a dev cluster.
This page is a map; follow a link for the example's README and files.

## Blueprints

A **blueprint** packages a multi-resource deployment as a reusable,
parameterized unit (`kscore-blueprint apply <dir>`). See
[Authoring a Blueprint](../getting-started/blueprint-authoring/) for the
format and lifecycle. Each example below has a `blueprint.yaml`,
`apply.yaml`, `rollback.yaml`, and a README.

| Blueprint | What it deploys |
| --- | --- |
| [`demo`](https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/src/branch/main/modules/examples/blueprints/demo) | Single-node demo: install a package, run it as a service — the smallest end-to-end blueprint. |
| [`monitoring-stack`](https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/src/branch/main/modules/examples/blueprints/monitoring-stack) | Prometheus + Grafana + Loki, ordered so Grafana starts after Prometheus. |
| [`nats-cluster`](https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/src/branch/main/modules/examples/blueprints/nats-cluster) | A NATS server with JetStream persistence and clustering config. |
| [`postgres-ha`](https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/src/branch/main/modules/examples/blueprints/postgres-ha) | Postgres with WAL streaming replication and a metrics exporter. |
| [`production-cluster`](https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/src/branch/main/modules/examples/blueprints/production-cluster) | An HA control-plane skeleton: embedded etcd, Postgres, and NATS. |
| [`security-baseline`](https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/src/branch/main/modules/examples/blueprints/security-baseline) | CIS-aligned host hardening: sysctl tuning and a default-deny firewall. |

## Module examples (Starlark)

Extension **modules** written in Starlark, sandboxed by capability — see
[Authoring & Publishing a Module](../getting-started/module-authoring/).
They run from the simplest possible module up to a composite ops bundle:

| Module | What it shows |
| --- | --- |
| [`hello`](https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/src/branch/main/modules/examples/hello) | The minimal module — deterministic, requests no capabilities. Start here. |
| [`cmdrun`](https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/src/branch/main/modules/examples/cmdrun) | Runs an allowlisted command — the canonical day-2 ops automation. |
| [`fsreport`](https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/src/branch/main/modules/examples/fsreport) | Reads a file and writes a one-line summary — the filesystem capability. |
| [`httpfetch`](https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/src/branch/main/modules/examples/httpfetch) | Fetches an HTTP resource and returns its status + size. |
| [`kvcache`](https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/src/branch/main/modules/examples/kvcache) | A stateful in-process cache + counter. |
| [`secretsync`](https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/src/branch/main/modules/examples/secretsync) | Reads a credential from one scoped secret path and rotates it. |
| [`opsbundle`](https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/src/branch/main/modules/examples/opsbundle) | A composite module that pins companion modules — the real-world pattern. |

## State files

The [Using State Management](../getting-started/using-state/) guide walks a
complete state file (user + package + file + service with requisites). The
in-repo integration fixtures under
[`internal/statemgmt/testdata/`](https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/src/branch/main/internal/statemgmt/testdata)
are additional worked state files, and every module's reference page in
[State Modules](../modules/) carries runnable snippets.
