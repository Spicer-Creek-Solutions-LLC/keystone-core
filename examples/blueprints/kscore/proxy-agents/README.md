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

| Parameter | Type | Default | Description |
| --- | --- | --- | --- |
| `credential_backend` | string | file | Credential backend |
| `vault_address` | string |  | Vault server address |
| `discovery_enabled` | boolean | false | Enable device discovery |
| `discovery_subnets` | array | [] | Subnets to scan |
