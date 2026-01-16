# identity-federation Blueprint

Configure SPIFFE identity federation for Keystone Core.

## Quick Start

```yaml
include:
  - blueprint: blueprints/kscore/identity-federation@0.1.0
    params:
      local_domain: kscore.local
      federated_domains:
        - prod.keystone
        - staging.keystone
```

## Parameters

See `blueprint.yaml` for parameter definitions.
