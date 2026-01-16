# postgres-ha Blueprint

PostgreSQL HA blueprint with replication placeholders and base provisioning.

## Quick Start

```yaml
include:
  - blueprint: blueprints/kscore/postgres-ha@0.1.0
    params:
      primary_host: db-primary.internal
      password: !secret keystone/postgres
```

## Parameters

| Parameter | Type | Default | Description |
| --- | --- | --- | --- |
| `primary_host` | string | (required) | Primary PostgreSQL host |
| `replica_hosts` | array | [] | Replica hosts |
| `database` | string | keystone | Database name |
| `user` | string | keystone | Database user |
| `password` | string | (secret) | Database password |
| `enable_pgbouncer` | bool | true | Enable connection pooling |
| `backup_enabled` | bool | true | Enable WAL archiving |
| `admin_user` | string | postgres | Admin user for provisioning |
| `admin_password` | string | (secret) | Admin password for provisioning |
| `port` | integer | 5432 | PostgreSQL port |
| `package_name` | string | postgresql | Package name to install |
| `service_name` | string | postgresql | Service name to manage |
