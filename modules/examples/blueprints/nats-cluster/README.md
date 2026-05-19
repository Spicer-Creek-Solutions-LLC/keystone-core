# nats-cluster

NATS server with JetStream persistence and clustering config.

## Parameters

| Name | Type | Default | Description |
|---|---|---|---|
| `cluster_size` | integer | `3` | Number of NATS nodes (1–9). |
| `store_dir` | string | `/var/lib/nats` | JetStream store directory. |

## Apply

```text
kscorectl blueprint apply nats-cluster --param cluster_size=5
```

`rollback.yaml` stops the service and removes the config.
