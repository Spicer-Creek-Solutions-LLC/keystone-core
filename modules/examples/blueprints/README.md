# Blueprint catalog (v1.0 reference set)

Six reference blueprints that exercise the v1.0 blueprint layer
(`internal/blueprint`): manifest, parameters (JSON-Schema + coercion),
feature flags, multi-instance namespacing, hooks-as-runbooks, and
apply/rollback through the State Runner.

## Status: illustrative reference

These are **documentation-grade reference blueprints**, not turnkey
production installers. Each one *loads, validates, renders, parses,
and resolves* through the real blueprint code using real stdlib state
modules (`package`, `service`, `file`, `sysctl`, `firewall`, `ssh`,
`user`, `security`). The state content is realistic and coherent but
intentionally bounded — standing up a real multi-node etcd/Postgres/
NATS cluster needs provider modules and live infrastructure beyond the
v1.0 stdlib. Treat them as the quality bar and starting point for
real blueprints, and read each blueprint's `README.md` for its intent.

## Index

| Blueprint | Purpose |
|---|---|
| `demo` | Single-node demo: package + service + managed file. Runnable end to end. |
| `production-cluster` | HA control-plane skeleton; secret-sourced DB password; preflight hook. |
| `monitoring-stack` | Prometheus + Grafana + Loki with soft ordering. |
| `security-baseline` | CIS-aligned host hardening with feature-gated controls. |
| `postgres-ha` | Postgres + replication; hard-depends on `monitoring-stack`. |
| `nats-cluster` | NATS cluster with JetStream. |

## Layout

Each blueprint directory contains:

- `blueprint.yaml` — the manifest (metadata, parameters, features,
  entrypoints, optional hooks).
- `apply.yaml` — the default entrypoint state collection.
- `rollback.yaml` — the rollback entrypoint (where meaningful).
- `hooks/*.yaml` — runbooks invoked as pre/post hooks (where used).
- `README.md` — intent, parameters, and how to apply.

Entrypoint and hook paths are resolved relative to the blueprint
directory, matching `internal/blueprint.Executor` conventions.
