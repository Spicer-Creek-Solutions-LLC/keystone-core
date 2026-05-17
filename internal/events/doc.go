// Package events is the v0.x reconstruction of Keystone Core's event
// system per PROJECT-DETAILS §4.9. The epic-11 design ships a passive
// pub/sub + persistence + filtering surface in v1.0; the active
// reactor engine (filter -> action; throttle/debounce; bounded
// concurrency; DLQ; retry-with-exp-backoff), lifecycle tracking, and
// enrichment pipeline land in post-v1.0.
//
// Why types-first: events cross every domain boundary that matters
// for observability (agent state, job lifecycle, drift detection),
// audit (policy pass/violation, user actions), and post-v1.0 automation
// (reactor filters). The wire-equivalent value types ([Event],
// [EventType], [Severity], [Category]) and the subject scheme
// ([SubjectFor]) need to be stable before any publisher /
// subscriber / store implementation lands. Task 1 ships those shapes;
// later tasks consume them without modification.
//
// Task 1 lands the foundational value types and helpers:
//
//   - [Event] — the §4.9 record exactly; constructed via [NewEvent]
//     which stamps a UUIDv7 [Event.ID], a UTC [Event.Time], and
//     [SeverityInfo] by default.
//   - [EventType] — newtype with `<category>.<subtype>` shape;
//     [ParseEventType] enforces shape + that the category half is
//     one of [KnownCategories]. The subtype half is free-form so
//     operators and post-v1.0 plugins (Epic 14) can introduce new signals
//     without core changes.
//   - [Severity] — ordered enum (debug < info < warn < error <
//     critical); [Severity.AtLeast] supports §4.9's
//     `severity >= 'warn'` filter pattern natively.
//   - [Category] — closed enum of the 6 v1.0 categories. Closed so
//     downstream routing (the audit pipeline in epic 12, the post-v1.0
//     reactor engine) can switch exhaustively on category for
//     retention policy and dispatch.
//   - The 22 v1.0 taxonomy constants (agent x5, job x4, state x5,
//     system x3, user x3, policy x2) per §4.9; [IsCanonical] reports
//     whether a value matches one of the documented spellings.
//   - [SubjectFor] / [Event.StampSubject] — build the
//     `kscore.<cluster>.events.<category>.<subtype>` NATS subject;
//     [Event.Subject] is stamped by the publisher at emit time
//     (task 3), not at [NewEvent] time, because cluster name is
//     operator-config not construction-site context.
//
// Roadmap of the rest of the epic, anchored on the types declared
// here:
//
//   - Task 2 — [EventStore] interface + SQLite + Postgres impls.
//   - Task 3 — [EventPublisher] + JetStream impl (ensures the
//     `KSCORE_EVENTS` stream exists with v1.0 defaults; sync + async).
//   - Task 4 — [EventSubscriber] + JetStream impl (durable consumer,
//     queue group support, optional historical replay).
//   - Task 5 — CEL filter integration via `google/cel-go`.
//   - Task 6 — gRPC `EventService` + REST handlers in
//     `pkg/api/events/`.
//   - Task 7 — `cmd/kscore-events` CLI.
//   - Task 8 — retention enforcer (per-type age + count limits; runs
//     hourly on cluster leader once Epic 13 lands leader election).
//   - Task 9 — runtime taxonomy registration (constants exposed via
//     `EventService.GetEventTypes`).
//   - Task 10 — audit emission integration consumed by Epic 12.
//   - Task 11 — integration test: 1000 events, filter, replay,
//     retention.
//
// Scope-out for v1.0 (tracked in `docs/project/ROADMAP.md` under
// gate-v1.0 / v1.x buckets): reactor engine, event lifecycle
// tracking, enrichment pipeline, dead-letter queue, inbound webhook
// receiver, CloudEvents 1.0 marshaling, Kafka, object-storage
// archival, multi-region replication.
package events
