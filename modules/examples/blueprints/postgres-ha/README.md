# postgres-ha

Postgres with WAL streaming replication config and a metrics
exporter. Declares a hard `requires` dependency on `monitoring-stack`
to demonstrate inter-blueprint dependency edges.

## Parameters

| Name | Type | Default | Description |
|---|---|---|---|
| `replicas` | integer | `2` | Streaming replicas (1–5); sets `max_wal_senders`. |
| `wal_level` | string | `replica` | One of `replica`, `logical`. |

## Dependencies

- `requires: monitoring-stack` — hard edge; the dependency resolver
  orders `monitoring-stack` before this blueprint.

## Apply

```text
kscorectl blueprint apply postgres-ha --param replicas=3 --param wal_level=logical
```
