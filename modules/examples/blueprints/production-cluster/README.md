# production-cluster

HA control-plane skeleton: embedded etcd, Postgres, and a NATS
cluster, with a secret-sourced database password and a pre-apply
preflight hook (run as a runbook).

## Illustrative reference

This installs and runs the packages/services and lays down config +
markers; it does **not** form a real multi-node etcd/Postgres/NATS
quorum (that needs provider modules and live nodes). It demonstrates
the production-shaped manifest: secret params, feature flags,
hooks-as-runbooks, requisite ordering, rollback.

## Parameters

| Name | Type | Default | Description |
|---|---|---|---|
| `cluster_name` | string | `keystone` | Cluster identifier (`^[a-z][a-z0-9-]*$`). |
| `node_count` | integer | `3` | Control-plane node count (1–7). |
| `postgres_password` | string (secret) | — | **Required.** Supply a `secret://` reference. |

## Features

| Feature | Default | Effect |
|---|---|---|
| `nats_jetstream` | on | Manage `/etc/keystone/nats-jetstream.conf`. |

## Hooks

`pre_apply` runs `hooks/preflight.yaml` as a runbook (disk + quorum
preflight placeholders) before the state collection is applied.

## Apply

```text
kscorectl blueprint apply production-cluster \
  --param postgres_password=secret://kv/db \
  --param cluster_name=test
```
