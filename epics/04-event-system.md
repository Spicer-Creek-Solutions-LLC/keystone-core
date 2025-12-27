# Epic 4: Event-Driven Automation System

## Overview

Implement a comprehensive event-driven automation system that enables reactive operations based on infrastructure events, inspired by Salt Project's reactor system but extended for cloud-native environments.

**Goal**: Create a powerful event bus and reactor system that enables automated responses to infrastructure events, integrating with external systems and enabling complex automation workflows.

## Success Criteria

- [ ] Pub/sub event bus for all system events
- [ ] Event tagging and filtering
- [ ] Reactor system for automated responses
- [ ] Event history and replay capability
- [ ] Integration with external event sources (webhooks, Kafka, cloud events)
- [ ] Event-driven state application
- [ ] Event correlation and aggregation
- [ ] Event throughput >10,000 events/sec
- [ ] Event processing latency <50ms

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│                External Event Sources                    │
│    Webhooks │ Kafka │ CloudEvents │ Prometheus           │
└──────────────────┬───────────────────────────────────────┘
                   │
                   ▼
┌──────────────────────────────────────────────────────────┐
│                  Event Ingestion                         │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │
│  │   Webhook    │  │   Stream     │  │   CloudEvent │  │
│  │   Receiver   │  │   Consumer   │  │   Adapter    │  │
│  └──────────────┘  └──────────────┘  └──────────────┘  │
└──────────────────┬───────────────────────────────────────┘
                   │
                   ▼
┌──────────────────────────────────────────────────────────┐
│               NATS Event Bus (JetStream)                 │
│          Topics: agent.*, state.*, job.*, user.*         │
└──────────────────┬───────────────────────────────────────┘
                   │
        ┌──────────┼──────────┐
        │          │          │
        ▼          ▼          ▼
┌──────────┐ ┌──────────┐ ┌──────────────┐
│  Event   │ │ Reactor  │ │   Event      │
│  Store   │ │ Engine   │ │   Consumers  │
│(JetStream│ │          │ │  (External)  │
└──────────┘ └────┬─────┘ └──────────────┘
                  │
                  ▼
          ┌───────────────┐
          │  Automated    │
          │  Actions      │
          │  (States,     │
          │   Commands,   │
          │   Webhooks)   │
          └───────────────┘
```

## User Stories

### US4.1: Event Bus for System Events
**As a** platform engineer
**I want to** receive real-time events from all infrastructure activities
**So that** I can monitor and react to system changes

**Acceptance Criteria**:
- All Keystone Core operations emit events
- Events published to NATS topics
- Structured event format (JSON/CloudEvents)
- Event tagging (severity, source, type)
- Event retention (configurable TTL)
- Event subscription via CLI/API

**Event Types**:
```yaml
# Agent events
agent.connect      # Agent connected
agent.disconnect   # Agent disconnected
agent.heartbeat    # Heartbeat received

# Job events
job.start          # Job started
job.complete       # Job completed
job.fail           # Job failed
job.output         # Job output line

# State events
state.apply.start  # State application started
state.apply.done   # State application completed
state.change       # State resource changed
state.drift        # Drift detected

# User events
user.login         # User authenticated
user.command       # User executed command
user.error         # User action failed
```

**Example Usage**:
```bash
# Subscribe to all events
kscorectl events subscribe '*'

# Subscribe to specific events
kscorectl events subscribe 'state.change'

# Subscribe with filter
kscorectl events subscribe 'agent.*' --filter 'datacenter=us-east-1'
```

### US4.2: Event Filtering and Routing
**As a** platform engineer
**I want to** filter and route events based on criteria
**So that** I only process relevant events

**Acceptance Criteria**:
- Filter events by type, tags, source
- Support complex filter expressions
- Route events to different consumers
- Event transformation (map fields)
- Event enrichment (add metadata)

**Example**:
```yaml
# Event filter configuration
filters:
  critical_failures:
    match:
      type: "state.apply.fail"
      tags.severity: "critical"
    action:
      route_to: ["pagerduty", "slack"]

  drift_in_production:
    match:
      type: "state.drift"
      tags.environment: "production"
    action:
      route_to: ["remediation_reactor"]
```

### US4.3: Reactor System
**As a** platform engineer
**I want to** define automated responses to events
**So that** the system can self-heal and auto-remediate

**Acceptance Criteria**:
- Define reactors in YAML/HCL
- Match events by type and criteria
- Trigger actions (commands, states, webhooks)
- Support conditional logic
- Rate limiting to prevent loops
- Reactor testing and validation

**Example**:
```yaml
# reactors/auto-remediate.yaml
reactors:
  restart_failed_service:
    match:
      type: "state.apply.fail"
      data.module: "service"
    conditions:
      - "{{ data.service }} in ['nginx', 'app']"
      - "{{ data.retry_count < 3 }}"
    actions:
      - type: "state.apply"
        state: "services.{{ data.service }}"
        target: "{{ event.agent_id }}"
        delay: "30s"
      - type: "webhook"
        url: "https://alerts.example.com/incident"
        payload:
          service: "{{ data.service }}"
          agent: "{{ event.agent_id }}"

  scale_on_high_load:
    match:
      type: "metric.threshold"
      data.metric: "cpu_usage"
      data.value: ">80"
    actions:
      - type: "command"
        command: "kubectl scale deployment {{ data.deployment }} --replicas={{ data.replicas + 1 }}"
        target: "role:k8s-master"
```

### US4.4: Event History and Replay
**As an** SRE
**I want to** query historical events and replay them
**So that** I can debug issues and test reactors

**Acceptance Criteria**:
- Store events in JetStream (configurable retention)
- Query events by time range, type, tags
- Replay events to test reactors
- Export events to external systems
- Event analytics and aggregation

**Example**:
```bash
# Query events
kscorectl events query --type "state.drift" --since "24h"

# Replay events (testing reactors)
kscorectl events replay --id "evt-12345" --reactor "auto-remediate"

# Export events
kscorectl events export --since "7d" --format json > events.json
```

### US4.5: Webhook Integration
**As a** platform engineer
**I want to** receive events from external systems via webhooks
**So that** Keystone Core can react to external triggers

**Acceptance Criteria**:
- HTTP webhook receiver endpoint
- Support various webhook formats (GitHub, GitLab, ArgoCD, etc.)
- Webhook authentication (HMAC, bearer token)
- Transform webhooks to Keystone Core events
- Webhook validation and error handling

**Example Webhooks**:
```yaml
# ArgoCD deployment webhook
POST /webhooks/argocd
{
  "type": "deployment.success",
  "app": "my-app",
  "revision": "abc123",
  "environment": "production"
}

# Triggers Keystone Core reactor:
# - Run smoke tests
# - Verify deployment health
# - Update monitoring dashboards
```

### US4.6: CloudEvents Support
**As a** platform engineer
**I want to** integrate with CloudEvents standard
**So that** Keystone Core works with cloud-native event systems

**Acceptance Criteria**:
- Publish Keystone Core events as CloudEvents
- Consume CloudEvents from external sources
- Support CloudEvents HTTP and NATS bindings
- Event schema registration
- CloudEvents discovery

**Example**:
```json
{
  "specversion": "1.0",
  "type": "com.kscore.state.drift",
  "source": "/agents/web-01",
  "id": "evt-12345",
  "time": "2024-01-15T10:30:00Z",
  "data": {
    "resource": "/etc/nginx/nginx.conf",
    "expected": "...",
    "actual": "..."
  }
}
```

### US4.7: Event Correlation
**As an** SRE
**I want to** correlate related events
**So that** I can understand cascading failures and dependencies

**Acceptance Criteria**:
- Assign correlation IDs to related events
- Group events by correlation ID
- Visualize event chains
- Detect event patterns (e.g., cascading failures)
- Alert on correlated event patterns

**Example**:
```
Correlation ID: deploy-abc123

1. [10:30:00] state.apply.start (app deployment)
2. [10:30:15] state.change (config updated)
3. [10:30:16] service.restart (app restarted)
4. [10:30:20] health_check.fail (app unhealthy)
5. [10:30:25] state.rollback (automatic rollback)
6. [10:30:45] service.restart (app restarted)
7. [10:30:50] health_check.success (app healthy)
```

## Technical Tasks

### Phase 1: Event Bus Foundation (Week 1-2)

**T1.1: Event Schema Definition**
- Define event data structures
- Create CloudEvents adapter
- Implement event serialization
- Add event validation
- Create event ID generation

**T1.2: NATS JetStream Integration**
- Configure JetStream for events
- Create event topics/streams
- Implement event publishing
- Add event subscription
- Configure retention policies

**T1.3: Event Emission**
- Emit events from all Keystone Core operations
- Add structured logging correlation
- Implement event batching
- Add event sampling for high-frequency events

### Phase 2: Event Filtering and Routing (Week 3)

**T2.1: Event Filter Engine**
- Parse filter expressions
- Implement filter matching
- Add filter composition (AND, OR, NOT)
- Support regex and glob patterns
- Optimize filter performance

**T2.2: Event Router**
- Route events based on filters
- Support multiple consumers
- Implement fan-out patterns
- Add routing rules configuration
- Create routing metrics

**T2.3: Event Enrichment**
- Add metadata to events
- Support event transformation
- Implement enrichment pipelines
- Add custom enrichment functions

### Phase 3: Reactor System (Week 4-5)

**T3.1: Reactor Definition**
- Parse reactor YAML/HCL
- Validate reactor configuration
- Support reactor includes
- Create reactor registry

**T3.2: Reactor Engine**
- Match events to reactors
- Evaluate conditions
- Execute actions sequentially
- Support action types: command, state, webhook
- Implement retry and error handling

**T3.3: Reactor Rate Limiting**
- Prevent reactor loops
- Implement cooldown periods
- Add rate limiting per reactor
- Track reactor execution history

**T3.4: Reactor Testing**
- Dry-run mode for reactors
- Simulate events for testing
- Validate reactor logic
- Create reactor testing framework

### Phase 4: External Integration (Week 6)

**T4.1: Webhook Receiver**
- HTTP server for webhooks
- Parse common webhook formats
- Implement HMAC verification
- Support custom webhook parsers
- Add webhook request logging

**T4.2: Webhook Sender**
- Send webhooks from reactors
- Support custom headers and auth
- Implement retry logic
- Add webhook templates
- Track webhook delivery

**T4.3: Stream Consumers**
- Kafka consumer integration
- RabbitMQ consumer
- AWS SQS/SNS integration
- Azure Event Hub integration
- GCP Pub/Sub integration

### Phase 5: Event Storage and Query (Week 7)

**T5.1: Event Storage**
- Use JetStream for persistence
- Implement retention policies
- Add event indexing
- Support event archival (S3, GCS)

**T5.2: Event Query API**
- Query by time range
- Query by type and tags
- Full-text search in event data
- Aggregation queries
- Export to various formats

**T5.3: Event Replay**
- Replay historical events
- Replay to specific reactors
- Support replay filters
- Add replay rate limiting

### Phase 6: Monitoring and Observability (Week 8)

**T6.1: Event Metrics**
- Event publish rate
- Event processing latency
- Reactor execution metrics
- Webhook delivery metrics
- JetStream metrics

**T6.2: Event Dashboard**
- Real-time event stream view
- Event type breakdown
- Reactor execution history
- Event correlation visualization
- Alert configuration

## Dependencies

- **Epic 1**: Core Infrastructure (NATS JetStream)
- **Epic 2**: Remote Execution (for reactor actions)
- **Epic 3**: State Management (for reactor actions)
- **Go Libraries**:
  - `github.com/nats-io/nats.go` - NATS with JetStream
  - `github.com/cloudevents/sdk-go` - CloudEvents
  - `github.com/expr-lang/expr` - Expression evaluation
  - `github.com/robfig/cron` - Scheduled reactors
  - `github.com/google/uuid` - Event IDs

## Risks & Mitigations

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| Event loops (reactor triggers itself) | High | Medium | Rate limiting, loop detection, backoff |
| Event volume overwhelming system | High | Medium | Sampling, batching, backpressure |
| JetStream storage limits | Medium | Medium | Retention policies, archival |
| Reactor misconfiguration | High | High | Validation, testing framework, dry-run |
| External webhook failures | Medium | High | Retry logic, dead letter queue |

## Metrics & Monitoring

### Key Metrics
- Event publish rate (events/sec)
- Event processing latency (p50, p95, p99)
- Reactor execution count
- Reactor success/failure rate
- Webhook delivery success rate
- JetStream storage usage

### Alerts
- Event processing latency >100ms
- Reactor failure rate >5%
- Event publish failures
- JetStream storage >80%
- Webhook delivery failures >10%

## Testing Strategy

### Unit Tests
- Event serialization/deserialization
- Filter expression parsing
- Reactor configuration validation
- Event correlation logic

### Integration Tests
- End-to-end event flow
- Reactor triggering and execution
- External webhook integration
- Event replay functionality

### Load Tests
- 10,000 events/sec sustained
- Reactor execution under load
- JetStream performance
- Event query performance

## Documentation Requirements

- [ ] Event schema reference
- [ ] Reactor configuration guide
- [ ] Webhook integration guide
- [ ] Event query API documentation
- [ ] CloudEvents integration guide
- [ ] Event correlation patterns
- [ ] Reactor examples and patterns
- [ ] Troubleshooting event loops

## Definition of Done

- [ ] All user stories implemented
- [ ] Event throughput >10,000/sec
- [ ] Reactor system tested with complex scenarios
- [ ] External integrations working
- [ ] Documentation complete
- [ ] Performance benchmarks met
- [ ] Example reactors for common patterns
- [ ] Ready for production use
