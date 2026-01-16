# enterprise-platform Blueprint

Enterprise multi-region platform blueprint for Keystone Core.

## Quick Start

```yaml
include:
  - blueprint: blueprints/kscore/enterprise-platform@0.1.0
    params:
      cluster_name: keystone-enterprise
      control_plane_nodes:
        - cp1.example.com
        - cp2.example.com
        - cp3.example.com
      regions:
        - us-east
        - eu-west
      postgres_host: db.example.com
      postgres_password: !secret keystone/postgres
```

## Features

Defaults are enterprise-friendly, but you can toggle them:

- `identity_federation` (default: true)
- `monitoring` (default: true)
- `security` (default: true)
- `gitops` (default: false)
- `proxy_agents` (default: false)
- `file_distribution` (default: false)
- `nats_cluster` (default: true)
- `postgres_ha` (default: true)

## Parameters

See `blueprint.yaml` for the full parameter list.
