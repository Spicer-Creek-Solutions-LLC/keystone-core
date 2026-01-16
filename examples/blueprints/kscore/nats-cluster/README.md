# nats-cluster Blueprint

Stand-alone NATS cluster blueprint with JetStream enabled.

## Quick Start

```yaml
include:
  - blueprint: blueprints/kscore/nats-cluster@0.1.0
    params:
      nodes:
        - nats-1.internal
        - nats-2.internal
        - nats-3.internal
```

## Parameters

| Parameter | Type | Default | Description |
| --- | --- | --- | --- |
| `nodes` | array | (required) | NATS node addresses |
| `cluster_name` | string | keystone-nats | NATS cluster name |
| `jetstream_enabled` | bool | true | Enable JetStream |
| `storage_dir` | string | /var/lib/nats | JetStream storage directory |
| `max_memory` | string | 1Gi | JetStream memory limit |
| `max_storage` | string | 10Gi | JetStream storage limit |
| `client_port` | integer | 4222 | Client port |
| `cluster_port` | integer | 6222 | Cluster port |
| `monitor_port` | integer | 8222 | Monitoring port |
| `package_name` | string | nats-server | Package name to install |
| `service_name` | string | nats-server | Service name to manage |
