# monitoring-stack

Prometheus + Grafana + Loki. Grafana starts after Prometheus and
Loki via requisite ordering.

## Parameters

| Name | Type | Default | Description |
|---|---|---|---|
| `retention_days` | integer | `15` | Prometheus TSDB retention (1–365). |
| `grafana_admin` | string | `admin` | Grafana admin username. |

## Apply

```text
kscorectl blueprint apply monitoring-stack --param retention_days=30
```
