# Epic 11: Event System

**Phase**: G • **Estimate**: 1.5 weeks • **Depends on**: 02, 03, 05 • **Blocks**: 12 (audit emission), 14, 16

## Goal

Pub/sub event bus + persistence + filtering. Foundation for audit, observability, and v1.1 reactor automation. NATS JetStream for fan-out/replay; SQL EventStore for long-term query.

## Scope (in)

- `internal/events/`:
  - `Event{ID, Type EventType, Source string, Time time.Time, Severity Severity, CorrelationID string, Tags map[string]string, Data map[string]any, Subject string}`.
  - `EventStore` (SQL-backed, extends `internal/state.Store` with `EventStore` sub-interface): `Store`, `StoreBatch`, `Get`, `Query (EventQuery DSL: type, source, severity, tags, time range)`, `Delete`, `Count`, `ApplyRetention`, `Close`. Indexed on type, source, timestamp, severity, correlation_id.
  - `EventPublisher` interface + `JetStreamPublisher` impl (sync + async; ensures `KSCORE_EVENTS` stream exists).
  - `EventSubscriber` interface + `JetStreamSubscriber` impl (durable consumer per subscription, manual ack, 30s ack timeout, 3 max redeliveries; queue groups for load-balanced consumers).
- **Filter expressions** via `google/cel-go` — fields: `type`, `source`, `severity`, `time`, `tags.*`, `data.*` (nested). Operators: comparison, AND/OR/NOT, regex (`matches`), glob (`contains`).
- Subject prefix `kscore.{cluster}.events.{category}.{subtype}` (always cluster-prefixed).
- Stream defaults: 7d / 10 GB / 1M msgs / `DiscardNew` policy.
- Retention enforcer (per-type age + count limits) — runs hourly on cluster leader.
- gRPC `EventService` impl: `ListEvents`, `GetEvent`, `EmitEvent`, `SubscribeEvents` (stream w/ optional `replay_seconds`), `GetEventTypes`, `GetEventStats`.
- REST handlers in `pkg/api/events/`.
- `cmd/kscore-events` CLI: `list`, `query`, `emit`, `subscribe`, `watch`, `replay`, `retention`, `storage-stats`, `analyze`.
- Event taxonomy (v1.0 — 22 types, 6 categories) per `PROJECT-DETAILS §4.9`.
- Severity levels: debug, info, warn, error, critical.
- Correlation IDs: passed through gRPC contexts and NATS message headers.

## Scope (out / non-goals)

- **Reactor engine** (filter → action; throttle/debounce; bounded concurrency; DLQ; retry-with-exp-backoff) — v1.1.
- Lifecycle tracking (created → published → routed → processing → processed/failed/expired) — v1.1.
- Enrichment pipeline (tag/data/conditional enrichers) — v1.1.
- Dead-letter queue — v1.1 (paired with reactors).
- Kafka integration — v2.0.
- CloudEvents 1.0 marshaling — v2.0.
- Inbound webhook receiver for events (HMAC, signature) — v1.1.
- Object-storage archival — v2.0.
- Multi-region replication — v3+.

## Design summary

See `PROJECT-DETAILS.md §4.9`.

## Tasks

1. **`Event` type** + JSON codec.
   _(landed: `internal/events` ships the value types + subject scheme the rest of the epic builds on. `Event{ID, Type, Source, Time, Severity, CorrelationID, Tags, Data, Subject}` per §4.9 exactly, constructed via `NewEvent(typ, source)` which stamps `uuid.NewV7()` (k-sortable, Unix-ms-prefixed — better SQL index locality and NATS replay ordering than v4), UTC `time.Now()`, and `SeverityInfo` by default. `MustNewEvent` test sibling; `IsZero` / `Validate` / `Clone` (deep over `Tags map[string]string` + `Data map[string]any` recursing through nested `map[string]any` and `[]any`). `EventType` newtype with **shape + category-allowlist** validation per the option-C decision: split on the first `.`, category half must be a member of the closed `{agent, job, state, system, user, policy}` set, subtype half is free-form non-empty with no whitespace and MAY contain further dots (the `state.apply.*` family flows through as category=`state` + subtype=`apply.start` etc.). Subtypes are deliberately open so operators and v1.4 plugins (Epic 14) introduce new signals without core changes; downstream routing (audit pipeline Epic 12, v1.1 reactor) switches on the closed category for exhaustive dispatch. `Category` enum + `KnownCategories()` + `ParseCategory` (whitespace-trim + case-fold). `Severity` ordered enum (`Debug < Info < Warn < Error < Critical`; `SeverityUnknown` zero rejected by `Validate`) with `String` / `IsValid` / `AtLeast` (supports §4.9's `severity >= 'warn'` idiom; invalid sides always report false so a misconfigured threshold never silently passes everything) / `ParseSeverity` (accepts `warning` / `fatal` aliases) / `MarshalText` + `UnmarshalText` (canonical JSON+YAML names) / `AllSeverities`. 22 v1.0 taxonomy constants exposed (agent×5, job×4, state×5, system×3, user×3, policy×2); `IsCanonical` reports whether a type matches a documented spelling (operator-defined subtypes validate but report non-canonical — distinction lets a future `make lint` rule warn without hard-rejecting). `SubjectFor(cluster, typ) → "kscore.<cluster>.events.<category>.<subtype>"` validates cluster name against NATS-subject-token rules (alphanumeric + `-` + `_`; rejects `.`, `*`, `>`, whitespace, `/`). `Event.StampSubject(cluster)` is the publisher-side helper that task 3's JetStream publisher will call at emit time (Subject empty at `NewEvent` time because cluster name is operator-config, not construction-site context). Sentinel family: `ErrInvalidEvent` (root) + `ErrEventNotFound` + `ErrInvalidFilter` + `ErrPublisherNotStarted` + `ErrSubscriberNotStarted` + `ErrNotImplementedYet`. 99.2% coverage on `internal/events`; UUIDv7 k-sortability pinned by a 50-event sort test; spec spellings pinned by an exact-spelling test that fails deliberately on rename; `-race` clean; project `make lint` + `make test` + `make docs-lint` green.)_
2. **`EventStore` interface** + SQLite + Postgres impls. Schema: `events(id, type, source, time, severity, correlation_id, tags jsonb, data jsonb, subject)` with indexes.
3. **`EventPublisher` + `JetStreamPublisher`** — ensures stream exists with v1.0 defaults; sync + async publish.
4. **`EventSubscriber` + `JetStreamSubscriber`** — durable consumer, queue group support, optional historical replay (query store first, then live).
5. **CEL filter integration** — `filter.Parse(string) → cel.Program`; `Match(event) → bool`.
6. **gRPC EventService** + REST handlers. `SubscribeEvents` streaming with replay_seconds.
7. **`cmd/kscore-events`** CLI.
8. **Retention enforcer** — hourly job; runs on leader (Epic 13 wires leader-only).
9. **Event taxonomy registration** — define 22 v1.0 event types as constants; helpers per category.
10. **Audit emission integration** (Epic 12 hooks into this).
11. **Integration test**: emit 1000 events; query by filter; subscribe with replay; retention deletes old.

## Acceptance criteria

- [ ] `kscore-events emit --type agent.connect --source agent-x --severity info` records event in store and publishes on NATS.
- [ ] `kscore-events list --type 'agent.*' --severity '>=warn' --since 1h --limit 50` returns paginated.
- [ ] `kscore-events subscribe --filter "tags.role == 'web' && severity >= 'warn'"` streams matching events realtime.
- [ ] `--replay 60s` flag streams last 60s historical events then continues realtime.
- [ ] Retention deletes events older than configured age + count limits.
- [ ] Slow consumer (handler >30s) triggers redelivery up to 3 times.
- [ ] Coverage >80% on `internal/events`.

## Risks

- **Slow consumers blocking stream**: handlers must process or ack-defer; document.
- **Filter perf at high volume** — CEL compilation cached per subscription; profile and tune if hot.
- **Replay window vs JetStream retention**: live replay floor is JetStream retention; older events come from SQL query (slower). Document in API.
- **Clock skew between sources** — ordering is per-source, not global.
- **Stream full back-pressure** — `DiscardNew` policy drops new events. Surface buffer-depth metric and alert.

## References

- PROJECT-DETAILS §4.9.
