# production-cluster Blueprint

Production-ready HA cluster blueprint for Keystone Core.

## Quick Start

```yaml
include:
  - blueprint: blueprints/kscore/production-cluster@0.1.0
    params:
      cluster_name: keystone-prod
      control_plane_nodes:
        - cp1.example.com
        - cp2.example.com
        - cp3.example.com
      postgres_host: db.example.com
      postgres_password: !secret keystone/postgres
```

## Features

Enable optional components via blueprint features:

- `nats_cluster` (default: true)
- `postgres_ha` (default: true)
- `monitoring` (default: false)
- `security` (default: false)
- `gitops` (default: false)

## Parameters

Key parameters:

| Parameter | Type | Description |
| --- | --- | --- |
| `cluster_name` | string | Cluster identifier |
| `control_plane_nodes` | array | Control plane node addresses |
| `postgres_host` | string | PostgreSQL host |
| `postgres_password` | string | PostgreSQL password |
| `nats_mode` | string | NATS deployment mode |

See `blueprint.yaml` for the full parameter list.
