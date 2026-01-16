# demo Blueprint

Single-node demo blueprint for Keystone Core with embedded NATS and SQLite.

## Quick Start

```yaml
include:
  - blueprint: blueprints/kscore/demo@0.1.0
    params:
      hostname: demo.local
      admin_password: !secret keystone/admin
```

## Parameters

| Parameter | Type | Default | Description |
| --- | --- | --- | --- |
| `hostname` | string | localhost | Hostname for the instance |
| `admin_password` | string | (secret) | Admin API password |
| `enable_examples` | bool | true | Deploy example states and agents |
| `enable_dashboards` | bool | true | Deploy Grafana dashboards |
