---
title: "State Storage"
weight: 15
description: >
  Backend storage options for Keystone Core operational state, including SQLite for development and PostgreSQL for production.
---

## Overview

Keystone Core uses a relational database for storing operational state, including agent records, command history, state declarations, and event logs. The system supports two backends:

- **SQLite**: Zero-dependency embedded database for development, testing, and small deployments
- **PostgreSQL**: Production-grade database for high availability, scalability, and large deployments

## Storage Architecture

```mermaid
flowchart TB
    subgraph CP["Control Plane"]
        subgraph SI["Store Interface"]
            AS["Agent<br>Store"]
            CS["Command<br>Store"]
            SS["State<br>Store"]
            ES["Event<br>Store"]
            PS["Policy<br>Store"]
        end

        SI --> SQLite["SQLite<br>(embedded)"]
        SI --> PostgreSQL["PostgreSQL<br>(external)"]
    end
```

## Store Interface

All storage operations go through a common interface:

```go
type Store interface {
    // Agent operations
    CreateAgent(ctx context.Context, agent *AgentRecord) error
    GetAgent(ctx context.Context, id string) (*AgentRecord, error)
    UpdateAgent(ctx context.Context, agent *AgentRecord) error
    DeleteAgent(ctx context.Context, id string) error
    ListAgents(ctx context.Context, filter AgentFilter) ([]*AgentRecord, error)

    // Command operations
    CreateCommand(ctx context.Context, cmd *CommandRecord) error
    GetCommand(ctx context.Context, id string) (*CommandRecord, error)
    UpdateCommand(ctx context.Context, cmd *CommandRecord) error
    ListCommands(ctx context.Context, filter CommandFilter) ([]*CommandRecord, error)

    // State operations
    CreateState(ctx context.Context, state *StateRecord) error
    GetState(ctx context.Context, id string) (*StateRecord, error)
    UpdateState(ctx context.Context, state *StateRecord) error
    ListStates(ctx context.Context, filter StateFilter) ([]*StateRecord, error)

    // Transaction support
    BeginTx(ctx context.Context) (Transaction, error)

    // Health check
    Ping(ctx context.Context) error

    // Close connection
    Close() error
}
```

## SQLite Backend

### Overview

SQLite provides a zero-dependency storage option using the pure-Go `modernc.org/sqlite` driver (no CGO required).

**Best for:**
- Development and testing
- Single-node deployments
- Small deployments (<100 agents)
- Edge/IoT scenarios
- Quick proof-of-concept

### Configuration

```yaml
storage:
  type: sqlite
  sqlite:
    # Database file path
    path: /var/lib/keystone/keystone.db

    # Connection pool settings
    maxOpenConns: 1     # SQLite works best with single writer
    maxIdleConns: 1

    # Enable WAL mode for better concurrency
    walMode: true

    # Busy timeout in milliseconds
    busyTimeout: 5000

    # Enable foreign keys
    foreignKeys: true

    # Journal mode (delete, truncate, persist, memory, wal, off)
    journalMode: wal

    # Synchronous mode (off, normal, full, extra)
    synchronous: normal

    # Cache size in KB (negative = number of pages)
    cacheSize: -2000  # ~8MB with 4KB pages
```

### Features

| Feature | Support |
|---------|---------|
| ACID Transactions | ✅ |
| Concurrent Reads | ✅ (with WAL) |
| Concurrent Writes | ⚠️ Single writer |
| Full-Text Search | ✅ |
| JSON Operations | ✅ |
| Backup | ✅ Online backup |
| Replication | ❌ |
| High Availability | ❌ |

### Performance Characteristics

```
Typical Performance (SSD):
- Simple SELECT: <1ms
- INSERT: <5ms
- Complex JOIN: <10ms
- Write throughput: ~1000 ops/sec (single writer)
- Read throughput: ~10000 ops/sec (WAL mode)
```

### WAL Mode

Write-Ahead Logging (WAL) provides better concurrency:

```go
// WAL mode allows concurrent readers with one writer
// Enabled by default in Keystone Core
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
```

## PostgreSQL Backend

### Overview

PostgreSQL provides production-grade storage with high availability, scalability, and advanced features.

**Best for:**
- Production deployments
- Large deployments (100+ agents)
- High availability requirements
- Multi-region deployments
- Advanced querying needs

### Configuration

```yaml
storage:
  type: postgres
  postgres:
    # Connection string
    host: localhost
    port: 5432
    database: keystone
    user: keystone
    password: ${POSTGRES_PASSWORD}

    # Or use connection URL
    # url: postgres://keystone:password@localhost:5432/keystone?sslmode=require

    # Connection pool
    maxOpenConns: 25
    maxIdleConns: 5
    connMaxLifetime: 5m
    connMaxIdleTime: 1m

    # SSL mode (disable, require, verify-ca, verify-full)
    sslMode: require
    sslCert: /etc/keystone/certs/client.crt
    sslKey: /etc/keystone/certs/client.key
    sslRootCert: /etc/keystone/certs/ca.crt

    # Schema
    schema: public

    # Statement timeout
    statementTimeout: 30s

    # Lock timeout
    lockTimeout: 10s
```

### Features

| Feature | Support |
|---------|---------|
| ACID Transactions | ✅ |
| Concurrent Reads | ✅ |
| Concurrent Writes | ✅ |
| Full-Text Search | ✅ |
| JSON Operations | ✅ (JSONB) |
| Backup | ✅ pg_dump, pg_basebackup |
| Replication | ✅ Streaming, Logical |
| High Availability | ✅ With Patroni/Stolon |

### Performance Characteristics

```
Typical Performance (SSD, tuned):
- Simple SELECT: <1ms
- INSERT: <2ms
- Complex JOIN: <20ms
- Write throughput: ~10000 ops/sec
- Read throughput: ~50000 ops/sec
```

### High Availability

Deploy PostgreSQL with HA using Patroni:

```yaml
# Patroni configuration
scope: keystone-postgres
namespace: /service/
name: pg-node-1

restapi:
  listen: 0.0.0.0:8008
  connect_address: pg-node-1:8008

etcd3:
  hosts:
    - etcd1:2379
    - etcd2:2379
    - etcd3:2379

bootstrap:
  dcs:
    postgresql:
      use_pg_rewind: true
      parameters:
        max_connections: 100
        shared_buffers: 256MB
        wal_level: replica
        max_wal_senders: 10

postgresql:
  listen: 0.0.0.0:5432
  connect_address: pg-node-1:5432
  data_dir: /var/lib/postgresql/data
```

### IPv6 Support

PostgreSQL connections support both IPv4 and IPv6:

```yaml
storage:
  postgres:
    # IPv6 address
    host: "2001:db8::1"
    port: 5432

    # Or dual-stack hostname
    host: db.example.com  # Resolves to both IPv4 and IPv6
```

## Migration: SQLite to PostgreSQL

Keystone Core provides automated migration tooling for moving from SQLite to PostgreSQL.

### Using kscore-migrate

```bash
# Validate source and target
kscore-migrate validate \
  --source sqlite:///var/lib/keystone/keystone.db \
  --target postgres://keystone:pass@localhost/keystone

# Run migration (dry-run first)
kscore-migrate run \
  --source sqlite:///var/lib/keystone/keystone.db \
  --target postgres://keystone:pass@localhost/keystone \
  --dry-run

# Execute migration
kscore-migrate run \
  --source sqlite:///var/lib/keystone/keystone.db \
  --target postgres://keystone:pass@localhost/keystone \
  --batch-size 1000

# Verify migration
kscore-migrate validate \
  --source sqlite:///var/lib/keystone/keystone.db \
  --target postgres://keystone:pass@localhost/keystone \
  --compare-counts
```

### Migration Options

```bash
kscore-migrate run \
  --source <source-dsn> \
  --target <target-dsn> \
  --batch-size 1000       # Records per batch
  --skip-existing         # Skip existing records
  --dry-run               # Show what would be migrated
  --tables agents,commands # Migrate specific tables only
```

### Migration Process

1. **Schema Creation**: Creates tables in PostgreSQL
2. **Data Migration**: Copies data in batches
3. **Sequence Reset**: Updates auto-increment sequences
4. **Index Creation**: Creates indexes for performance
5. **Validation**: Verifies row counts and data integrity

## Schema Overview

### Agents Table

```sql
CREATE TABLE agents (
    id TEXT PRIMARY KEY,
    hostname TEXT NOT NULL,
    os TEXT,
    arch TEXT,
    version TEXT,
    labels JSONB,
    metadata JSONB,
    status TEXT DEFAULT 'unknown',
    last_heartbeat TIMESTAMP WITH TIME ZONE,
    registered_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_agents_status ON agents(status);
CREATE INDEX idx_agents_hostname ON agents(hostname);
CREATE INDEX idx_agents_labels ON agents USING GIN(labels);
```

### Commands Table

```sql
CREATE TABLE commands (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL REFERENCES agents(id),
    command TEXT NOT NULL,
    args JSONB,
    status TEXT DEFAULT 'pending',
    exit_code INTEGER,
    stdout TEXT,
    stderr TEXT,
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_commands_agent ON commands(agent_id);
CREATE INDEX idx_commands_status ON commands(status);
CREATE INDEX idx_commands_created ON commands(created_at);
```

### States Table

```sql
CREATE TABLE states (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL REFERENCES agents(id),
    module TEXT NOT NULL,
    state TEXT NOT NULL,
    parameters JSONB,
    status TEXT DEFAULT 'pending',
    result JSONB,
    applied_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_states_agent ON states(agent_id);
CREATE INDEX idx_states_module ON states(module);
CREATE INDEX idx_states_status ON states(status);
```

## Connection Pooling

### SQLite

SQLite uses a single connection for writes with optional read connections:

```yaml
storage:
  sqlite:
    maxOpenConns: 1     # Single writer
    maxIdleConns: 1     # Keep connection alive
```

### PostgreSQL

PostgreSQL benefits from connection pooling:

```yaml
storage:
  postgres:
    maxOpenConns: 25    # Max concurrent connections
    maxIdleConns: 5     # Keep warm connections
    connMaxLifetime: 5m # Prevent stale connections
    connMaxIdleTime: 1m # Close idle connections
```

For large deployments, use PgBouncer:

```ini
[databases]
keystone = host=localhost port=5432 dbname=keystone

[pgbouncer]
listen_port = 6432
listen_addr = *
auth_type = md5
pool_mode = transaction
max_client_conn = 1000
default_pool_size = 25
```

## Backup and Recovery

### SQLite Backup

```bash
# Online backup using SQLite's backup API
sqlite3 /var/lib/keystone/keystone.db ".backup /backup/keystone-$(date +%Y%m%d).db"

# Or with kscore-cluster-backup
kscore-cluster-backup create --type database
```

### PostgreSQL Backup

```bash
# Logical backup
pg_dump -h localhost -U keystone -d keystone -F c -f keystone-backup.dump

# Physical backup
pg_basebackup -h localhost -U replication -D /backup/pg-base -Fp -Xs -P

# Or with kscore-cluster-backup
kscore-cluster-backup create --type database
```

## Best Practices

### Choosing a Backend

| Scenario | Recommendation |
|----------|----------------|
| Development/Testing | SQLite |
| Single node, <100 agents | SQLite |
| Production, >100 agents | PostgreSQL |
| High availability required | PostgreSQL + Patroni |
| Edge/IoT devices | SQLite |
| Multi-region | PostgreSQL with replication |

### Performance Tuning

#### SQLite

```yaml
storage:
  sqlite:
    walMode: true
    synchronous: normal  # Trade durability for speed
    cacheSize: -2000     # More cache
    busyTimeout: 5000    # Wait for locks
```

#### PostgreSQL

```sql
-- postgresql.conf tuning
shared_buffers = 256MB
effective_cache_size = 768MB
work_mem = 16MB
maintenance_work_mem = 128MB
random_page_cost = 1.1  # SSD
effective_io_concurrency = 200  # SSD
```

### Data Retention

Configure automatic cleanup:

```yaml
storage:
  retention:
    # Keep command history for 30 days
    commands:
      maxAge: 720h
      # Or keep last N records
      maxCount: 100000

    # Keep events for 7 days
    events:
      maxAge: 168h

    # Run cleanup interval
    cleanupInterval: 1h
```

## Troubleshooting

### SQLite: Database Locked

If you see "database is locked" errors:

```bash
# Check for stuck processes
fuser /var/lib/keystone/keystone.db

# Enable WAL mode
sqlite3 /var/lib/keystone/keystone.db "PRAGMA journal_mode=WAL;"

# Increase busy timeout
storage:
  sqlite:
    busyTimeout: 10000
```

### PostgreSQL: Connection Refused

Check PostgreSQL is running and accessible:

```bash
# Check PostgreSQL status
systemctl status postgresql

# Test connection
psql -h localhost -U keystone -d keystone -c "SELECT 1;"

# Check pg_hba.conf for access rules
```

### Migration Failures

If migration fails:

```bash
# Check for constraint violations
kscore-migrate validate --verbose

# Resume from checkpoint
kscore-migrate run --resume

# Skip problematic records
kscore-migrate run --skip-errors --log-errors=/tmp/errors.log
```

### Performance Issues

Check query performance:

```sql
-- PostgreSQL: Find slow queries
SELECT query, calls, mean_time, total_time
FROM pg_stat_statements
ORDER BY total_time DESC
LIMIT 10;

-- Check index usage
SELECT relname, idx_scan, seq_scan
FROM pg_stat_user_tables
WHERE seq_scan > idx_scan;
```

## See Also

- [Control Plane](/docs/concepts/control-plane/) - Control plane architecture
- [Observability](/docs/concepts/observability/) - Monitoring storage metrics
- [Operations: Maintenance](/docs/operations/maintenance/) - Database maintenance
- [Operations: Backup](/docs/operations/self-management/) - Backup procedures
