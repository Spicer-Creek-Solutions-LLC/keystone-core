# proxy-agents Blueprint

Configure proxy agents for unmanaged devices.

## Quick Start

```yaml
include:
  - blueprint: blueprints/kscore/proxy-agents@0.1.0
    params:
      credential_backend: file
      discovery_enabled: true
      discovery_subnets:
        - 10.0.0.0/24
```

## Parameters

See `blueprint.yaml` for parameter definitions.
