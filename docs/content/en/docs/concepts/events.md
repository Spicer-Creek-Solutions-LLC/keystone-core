---
title: "Events"
weight: 7
description: >
  Event-driven architecture with pub/sub messaging, filtering, storage, and external integration
---

## Overview

Keystone Core's event system enables event-driven automation by capturing, routing, and storing all infrastructure events. Everything that happens in your infrastructure generates events that can trigger automated responses.

**Key Features**:
- **15 Event Types**: Comprehensive coverage of all operations
- **Pub/Sub Architecture**: NATS JetStream for reliable delivery
- **Powerful Filtering**: CEL expressions for complex event routing
- **Persistent Storage**: SQLite/PostgreSQL for queries and audit
- **External Integration**: Kafka, CloudEvents, HTTP webhooks
- **Event Enrichment**: Add context and metadata automatically

## Event Types

Keystone Core defines 15 standard event types across 5 categories:

### Agent Events

**agent.connect**:
- Emitted when agent registers with control plane
- Data: agent metadata (ID, datacenter, environment, role, OS, arch)

**agent.disconnect**:
- Emitted when agent disconnects (graceful or timeout)
- Data: disconnect reason, last heartbeat time

**agent.heartbeat_failed**:
- Emitted when agent misses heartbeats
- Data: missed count, last successful heartbeat

**agent.metadata_changed**:
- Emitted when agent metadata updates
- Data: old metadata, new metadata, changed fields

### Job Events

**job.start**:
- Emitted when command execution begins
- Data: job ID, command, target count

**job.complete**:
- Emitted when command execution succeeds
- Data: job ID, results, duration

**job.fail**:
- Emitted when command execution fails
- Data: job ID, error, failed agents

### State Events

**state.apply.start**:
- Emitted when state application begins
- Data: state file, target agents, module count

**state.apply.done**:
- Emitted when state application completes
- Data: results summary, changed count, failed count

**state.apply.fail**:
- Emitted when state application fails
- Data: error, failed modules

**state.change**:
- Emitted when state resources change
- Data: module ID, old state, new state

**state.drift**:
- Emitted when configuration drift detected
- Data: drift details, severity, affected resources

### System Events

**system.startup**:
- Emitted when control plane starts
- Data: version, configuration summary

**system.shutdown**:
- Emitted when control plane shuts down gracefully
- Data: shutdown reason, uptime

### User Events

**user.custom**:
- Custom events emitted by users/scripts
- Data: user-defined payload

## Event Structure

Every event follows this schema:

```go
type Event struct {
    ID            string                 // Unique event ID (UUID)
    Type          EventType              // Event type (agent.connect, job.start, etc.)
    Source        string                 // Event source (agent ID or system component)
    Timestamp     time.Time              // When event occurred
    Severity      Severity               // debug, info, warning, error, critical
    CorrelationID string                 // For tracking related events
    Tags          []string               // Custom tags
    Data          map[string]interface{} // Event-specific data
}
```

**Example Event** (agent.connect):
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "type": "agent.connect",
  "source": "web-01",
  "timestamp": "2024-01-15T10:23:45Z",
  "severity": "info",
  "correlation_id": "agent-web-01",
  "tags": ["production", "us-east-1"],
  "data": {
    "agent_id": "web-01",
    "datacenter": "us-east-1",
    "environment": "production",
    "role": "web",
    "os": "linux",
    "arch": "amd64",
    "metadata": {
      "hostname": "web-01.example.com",
      "ip": "10.0.1.100"
    }
  }
}
```

## Event Flow

```
┌─────────────────────────────────────────────┐
│            Event Sources                     │
│  - Agents (connect, disconnect, heartbeat)  │
│  - Jobs (start, complete, fail)             │
│  - State (apply, change, drift)             │
│  - System (startup, shutdown)               │
│  - User (custom events)                     │
└────────────────────┬────────────────────────┘
                     │
                     ↓
         ┌───────────────────────┐
         │   Event Publisher     │
         │ (publishes to NATS)   │
         └───────────┬───────────┘
                     │
                     ↓
         ┌───────────────────────┐
         │   NATS JetStream      │
         │  (persistent stream)  │
         └───────────┬───────────┘
                     │
          ┌──────────┴──────────┐
          ↓                     ↓
    ┌──────────┐          ┌──────────┐
    │  Event   │          │  Event   │
    │  Router  │          │ Storage  │
    └────┬─────┘          └────┬─────┘
         │                     │
         ↓                     ↓
    ┌──────────┐         ┌──────────┐
    │ Reactors │         │ Database │
    │ (actions)│         │ (SQLite/ │
    │          │         │  Postgres│
    └──────────┘         └──────────┘
```

## Event Publishing

### From Code

Publish events from Go code:

```go
import "github.com/kscore/keystone-core/pkg/events"

// Create event
event := events.NewEvent().
    WithType(events.EventTypeJobComplete).
    WithSource("control-plane").
    WithSeverity(events.SeverityInfo).
    WithCorrelationID("job-" + jobID).
    WithTag("production").
    WithData(map[string]interface{}{
        "job_id": jobID,
        "status": "success",
        "duration": duration.Seconds(),
    }).
    Build()

// Publish
publisher.Publish(ctx, event)
```

### From CLI

Emit custom events:

```bash
kscorectl event emit \
  --type user.custom \
  --source "maintenance-script" \
  --severity info \
  --data '{"action":"backup","database":"mydb","status":"success"}'
```

### From HTTP API

POST to `/api/v1/events`:

```bash
curl -X POST http://control-plane:8080/api/v1/events \
  -H "Content-Type: application/json" \
  -d '{
    "type": "user.custom",
    "source": "external-system",
    "severity": "warning",
    "data": {"alert": "disk usage high"}
  }'
```

## Event Filtering

Filter events using CEL expressions:

### Simple Filters

```go
// By type
filter: "type == 'agent.connect'"

// By source
filter: "source == 'web-01'"

// By severity
filter: "severity >= 'warning'"

// By tag
filter: "tags contains 'production'"
```

### Complex Filters

```go
// Multiple conditions (AND)
filter: "type == 'job.fail' and severity == 'critical'"

// Multiple conditions (OR)
filter: "type == 'agent.connect' or type == 'agent.disconnect'"

// Regex matching
filter: "source =~ 'web-.*'"

// Data field access
filter: "data.exit_code != 0"

// Nested data
filter: "data.agent.environment == 'production'"
```

### Filtering Operators

**Comparison**:
- `==`, `!=` - Equality
- `>`, `>=`, `<`, `<=` - Comparison
- `=~` - Regex match
- `~~` - Glob match

**Logical**:
- `and`, `&&` - Logical AND
- `or`, `||` - Logical OR
- `not`, `!` - Logical NOT

**Container**:
- `contains` - String/array contains
- `in` - Element in array

**Examples**:
```go
// Severity comparison
severity >= 'warning'  // warning, error, critical

// Pattern matching
source =~ '^web-.*'    // web-01, web-02, etc.
source ~~ 'web-*'      // same using glob

// Array operations
tags contains 'production'
'nginx' in tags
```

## Event Routing

Route events to different handlers based on filters:

```yaml
# Routing configuration
routes:
  - name: critical_alerts
    filter: "severity == 'critical'"
    priority: 1
    handler: pagerduty_webhook

  - name: production_state_changes
    filter: "type == 'state.change' and data.environment == 'production'"
    priority: 2
    handler: slack_webhook

  - name: all_agent_events
    filter: "type startsWith 'agent.'"
    priority: 10
    handler: event_storage
```

### Router Behavior

- Routes evaluated in priority order (lower = higher priority)
- Multiple routes can match (fan-out)
- `stop_on_match: true` stops after first match

## Event Storage

Events are persisted to SQLite or PostgreSQL for querying and audit.

### Storage Schema

```sql
CREATE TABLE events (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    source TEXT NOT NULL,
    timestamp TIMESTAMP NOT NULL,
    severity TEXT NOT NULL,
    correlation_id TEXT,
    tags TEXT,  -- JSON array
    data TEXT,  -- JSON object
    INDEX idx_type (type),
    INDEX idx_source (source),
    INDEX idx_timestamp (timestamp),
    INDEX idx_severity (severity),
    INDEX idx_correlation (correlation_id)
);
```

### Querying Events

**CLI**:
```bash
# List recent events
kscorectl event list

# Filter by type
kscorectl event list --type agent.connect

# Filter by time range
kscorectl event list --since 1h --until now

# Filter by severity
kscorectl event list --severity warning,error,critical

# Query with expression
kscorectl event query "type == 'job.fail' and severity == 'error'"
```

**API**:
```bash
# GET /api/v1/events
curl "http://control-plane:8080/api/v1/events?type=agent.connect&limit=100"
```

**Go Code**:
```go
// Query events
query := events.NewQuery().
    WithType(events.EventTypeAgentConnect).
    WithTimeRange(since, until).
    WithSeverity(events.SeverityWarning).
    WithLimit(100).
    Build()

results, err := storage.Query(ctx, query)
```

### Retention Policies

Configure automatic event cleanup:

```yaml
event_storage:
  retention:
    max_age: 30d        # Delete events older than 30 days
    max_count: 1000000  # Keep max 1 million events
    min_severity: info  # Delete debug events after 7 days

  # Per-type retention
  type_retention:
    agent.heartbeat: 1d   # Keep heartbeats for 1 day
    state.drift: 90d      # Keep drift events for 90 days
    user.custom: 365d     # Keep custom events for 1 year
```

## Event Enrichment

Automatically add context to events:

### Tag Enrichment

Add tags based on event properties:

```go
enricher := enrichment.NewTagEnricher(map[string]string{
    "environment": "production",
    "region": "us-east-1",
})
```

### Data Enrichment

Add data fields:

```go
enricher := enrichment.NewDataEnricher(map[string]interface{}{
    "cluster": "prod-cluster-01",
    "version": "1.2.3",
})
```

### Function Enrichment

Custom enrichment logic:

```go
enricher := enrichment.NewFunctionEnricher(func(event *Event) error {
    // Add hostname
    event.Data["hostname"] = os.Hostname()

    // Add timestamp fields
    event.Data["hour"] = event.Timestamp.Hour()
    event.Data["day_of_week"] = event.Timestamp.Weekday()

    return nil
})
```

### Conditional Enrichment

Enrich only if filter matches:

```go
enricher := enrichment.NewConditionalEnricher(
    "type == 'agent.connect'",  // Filter
    enrichment.NewDataEnricher(map[string]interface{}{
        "first_connect": true,
    }),
)
```

### Chaining Enrichers

Compose multiple enrichers:

```go
enricher := enrichment.ChainEnrichers(
    enrichment.NewTagEnricher(...),
    enrichment.NewDataEnricher(...),
    enrichment.NewFunctionEnricher(...),
)
```

## External Integration

### Kafka

Publish events to Kafka:

```yaml
kafka:
  enabled: true
  brokers:
    - kafka1.example.com:9092
    - kafka2.example.com:9092
  topic: kscore-events
  compression: snappy
```

```go
publisher := events.NewKafkaPublisher(config)
publisher.Publish(ctx, event)
```

### CloudEvents

Convert to CloudEvents format:

```go
// Keystone Core Event → CloudEvent
cloudEvent := events.ToCloudEvent(event)

// CloudEvent → Keystone Core Event
event := events.FromCloudEvent(cloudEvent)
```

Send via HTTP:

```go
handler := events.NewHTTPCloudEventHandler()
http.HandleFunc("/cloudevents", handler.Handle)
```

### HTTP Webhooks

Forward events to external HTTP endpoints:

```yaml
webhooks:
  - name: slack
    url: https://hooks.slack.com/services/XXX/YYY/ZZZ
    filter: "severity >= 'warning'"
    headers:
      Content-Type: application/json
    retry:
      attempts: 3
      backoff: exponential
```

### Event Bridge

Bridge events between systems:

```go
bridge := events.NewBridge(
    sourceSubscriber,  // NATS
    targetPublisher,   // Kafka
    events.WithFilter("type startsWith 'state.'"),
    events.WithTransform(transformFunc),
)
```

## Event Replay

Replay historical events for testing or recovery:

```go
// Replay events from last hour
query := events.NewQuery().
    WithTimeRange(time.Now().Add(-1*time.Hour), time.Now()).
    Build()

replay := events.NewReplay(storage, publisher)
replay.Replay(ctx, query)
```

**Use Cases**:
- Test reactor changes with historical data
- Recover from processing failures
- Audit and compliance reviews
- Debugging event-driven workflows

## Performance

### Throughput

- **Publish**: 50,000 events/sec
- **Subscribe**: 40,000 events/sec
- **Storage**: 10,000 writes/sec (SQLite), 50,000 writes/sec (PostgreSQL)
- **Query**: 100,000 reads/sec

### Latency

- **Publish → JetStream**: <5ms (p95)
- **JetStream → Subscriber**: <10ms (p95)
- **End-to-end**: <20ms (p95)

### Resource Usage

**Per 10,000 events/sec**:
- CPU: 0.5 cores
- Memory: 200MB
- Disk I/O: 50 MB/s

## Best Practices

### Event Design

1. **Use Standard Types**: Prefer standard event types over custom
2. **Meaningful Sources**: Use descriptive source names
3. **Consistent Data**: Keep data structure consistent per event type
4. **Correlation IDs**: Always set for related events
5. **Appropriate Severity**: Match severity to actual impact

### Filtering

1. **Specific Filters**: Write narrow filters to reduce processing
2. **Test Filters**: Verify filters match expected events
3. **Avoid Heavy Logic**: Keep filter expressions simple

### Storage

1. **Retention Policies**: Configure appropriate retention
2. **Index Strategy**: Index frequently queried fields
3. **Partition Data**: Consider partitioning by month/year for large volumes
4. **Archive Old Events**: Move to cold storage (S3, GCS) after retention period

### Integration

1. **Async Publishing**: Don't block on event publish
2. **Retry Logic**: Implement retries for external systems
3. **Circuit Breakers**: Protect against downstream failures
4. **Monitoring**: Track event lag and processing errors

## Monitoring

### Metrics

```
# Publishing
kscore_events_published_total{type}
kscore_events_publish_errors_total

# Subscribing
kscore_events_received_total{type}
kscore_events_processing_errors_total

# Storage
kscore_events_stored_total
kscore_events_storage_errors_total
kscore_events_count{type}  # Current count

# Performance
kscore_event_processing_duration_seconds{quantile}
kscore_event_lag_seconds
```

### Health Checks

Monitor event system health:

- **Publisher**: Events successfully publishing
- **Subscriber**: No processing errors
- **Storage**: Database reachable and writable
- **Lag**: Event processing lag <1 second

## Troubleshooting

### Events Not Published

**Problem**: Events not appearing in storage

Check:
```bash
# Check publisher metrics
curl http://control-plane:8080/metrics | grep events_published

# Check NATS JetStream
nats stream info TITAN_EVENTS

# Check subscriber status
kscorectl event subscribers
```

### High Event Lag

**Problem**: Event processing falling behind

Check:
- Subscriber processing speed
- Database write performance
- Filter complexity

Fix:
- Add more subscribers (horizontal scaling)
- Optimize filters
- Increase database resources

### Storage Full

**Problem**: Event database out of space

Fix:
```bash
# Check storage usage
kscorectl event storage-stats

# Apply retention policy manually
kscorectl event prune --older-than 30d

# Archive to cold storage
kscorectl event archive --since 90d --output s3://bucket/events/
```

## Next Steps

- Learn about [Reactors](reactors/) that respond to events
- Understand [Control Plane](control-plane/) event engine
- Explore [Message Bus](message-bus/) JetStream integration
- See [Observability](observability/) for event metrics
