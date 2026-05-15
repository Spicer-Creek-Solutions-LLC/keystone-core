package events

import (
	"context"
	"fmt"

	"go.keystone-core.io/keystone-core/internal/state"
)

// sqlEventStore wraps an [state.EventStore] (SQLite or Postgres) into
// the typed [EventStore] consumer interface. It owns translation in
// both directions:
//
//   - inbound: [Event] -> [state.EventStoreRecord], [EventQuery] ->
//     [state.EventFilter], [RetentionPolicy] ->
//     [state.EventsRetentionPolicy].
//   - outbound: [state.EventStoreRecord] -> [Event].
//
// Validation runs in the wrapper so the underlying store never sees a
// malformed [Event]. Errors from the underlying store flow through
// unchanged so [errors.Is] against `state.ErrNotFound` /
// `state.ErrDuplicate` works at call sites.
type sqlEventStore struct {
	backing state.EventStore
}

// NewSQLEventStore wraps the given [state.EventStore] in the
// [EventStore] interface. The wrapper is safe for concurrent use to
// the extent that backing is — `database/sql` is, by design.
//
// `Close` on the returned store is a no-op: the underlying SQL
// connection pool is owned by the [state.Store] holder (the
// process-wide state composite), not by us.
func NewSQLEventStore(backing state.EventStore) EventStore {
	return &sqlEventStore{backing: backing}
}

func (s *sqlEventStore) Store(ctx context.Context, e Event) error {
	if err := e.Validate(); err != nil {
		return err
	}
	return s.backing.CreateEvent(ctx, recordFromEvent(e))
}

func (s *sqlEventStore) StoreBatch(ctx context.Context, events []Event) error {
	if len(events) == 0 {
		return nil
	}
	for i, e := range events {
		if err := e.Validate(); err != nil {
			return fmt.Errorf("events: StoreBatch [%d]: %w", i, err)
		}
	}
	recs := make([]*state.EventStoreRecord, len(events))
	for i, e := range events {
		recs[i] = recordFromEvent(e)
	}
	return s.backing.CreateEventsBatch(ctx, recs)
}

func (s *sqlEventStore) Get(ctx context.Context, id string) (Event, error) {
	rec, err := s.backing.GetEvent(ctx, id)
	if err != nil {
		return Event{}, err
	}
	return eventFromRecord(rec), nil
}

func (s *sqlEventStore) Query(ctx context.Context, q EventQuery) (EventPage, error) {
	if err := q.Validate(); err != nil {
		return EventPage{}, err
	}
	limit := q.Limit
	if limit == 0 {
		limit = DefaultQueryLimit
	}
	filter := filterFromQuery(q, limit)
	recs, err := s.backing.ListEvents(ctx, filter)
	if err != nil {
		return EventPage{}, err
	}
	page := EventPage{Events: make([]Event, len(recs))}
	for i, rec := range recs {
		page.Events[i] = eventFromRecord(rec)
	}
	// NextCursor is the last returned ID iff the page is full. A
	// short page (fewer than limit rows) means we definitely caught
	// up — no cursor.
	if len(recs) == limit && len(recs) > 0 {
		page.NextCursor = recs[len(recs)-1].ID
	}
	return page, nil
}

func (s *sqlEventStore) Count(ctx context.Context, q EventQuery) (int, error) {
	if err := q.Validate(); err != nil {
		return 0, err
	}
	// Count ignores Limit/Cursor/Descending — filterFromQuery's
	// limit arg is irrelevant here.
	filter := filterFromQuery(q, 0)
	return s.backing.CountEvents(ctx, filter)
}

func (s *sqlEventStore) Delete(ctx context.Context, id string) error {
	return s.backing.DeleteEvent(ctx, id)
}

func (s *sqlEventStore) ApplyRetention(ctx context.Context, policies []RetentionPolicy) (int, error) {
	if len(policies) == 0 {
		return 0, nil
	}
	out := make([]state.EventsRetentionPolicy, len(policies))
	for i, p := range policies {
		out[i] = state.EventsRetentionPolicy{
			Type:     string(p.Type),
			MaxAge:   p.MaxAge,
			MaxCount: p.MaxCount,
		}
	}
	return s.backing.ApplyEventsRetention(ctx, out)
}

func (s *sqlEventStore) Close() error {
	// The underlying SQL pool is owned by the process-wide state
	// composite. Closing it here would interfere with other domains
	// (agents, secrets, identity) that share the same handle.
	return nil
}

// recordFromEvent translates the typed [Event] into the DB-shape
// [state.EventStoreRecord]. Severity + Type are written as their
// canonical lowercase strings so SQL filtering (IN-clause for
// severity, LIKE for category) and operator-side `psql` / `sqlite3`
// inspection work without lookup tables.
func recordFromEvent(e Event) *state.EventStoreRecord {
	return &state.EventStoreRecord{
		ID:            e.ID,
		Type:          string(e.Type),
		Source:        e.Source,
		Time:          e.Time,
		Severity:      e.Severity.String(),
		CorrelationID: e.CorrelationID,
		Tags:          e.Tags,
		Data:          e.Data,
		Subject:       e.Subject,
	}
}

// eventFromRecord translates the DB-shape record into the typed
// [Event]. Severity parses via [ParseSeverity] — an unparseable
// severity name from the DB silently round-trips as [SeverityUnknown]
// so callers using [Event.Validate] surface the corruption rather
// than panic'ing on bad data. (Schema-level NOT NULL on the column
// makes this branch unreachable in practice.)
func eventFromRecord(rec *state.EventStoreRecord) Event {
	sev, _ := ParseSeverity(rec.Severity)
	return Event{
		ID:            rec.ID,
		Type:          EventType(rec.Type),
		Source:        rec.Source,
		Time:          rec.Time,
		Severity:      sev,
		CorrelationID: rec.CorrelationID,
		Tags:          rec.Tags,
		Data:          rec.Data,
		Subject:       rec.Subject,
	}
}

// filterFromQuery converts the typed [EventQuery] into the DB-shape
// [state.EventFilter]. Caller passes the resolved page size (which
// has already been defaulted via [DefaultQueryLimit] for Query, or
// left at 0 for Count).
func filterFromQuery(q EventQuery, limit int) state.EventFilter {
	f := state.EventFilter{
		Type:          string(q.Type),
		Source:        q.Source,
		CorrelationID: q.CorrelationID,
		Since:         q.Since,
		Until:         q.Until,
		Cursor:        q.Cursor,
		Limit:         limit,
		Descending:    q.Descending,
		Severities:    severitiesAtLeast(q.MinSeverity),
		Tags:          q.Tags,
	}
	if q.Category != "" {
		f.TypePrefix = string(q.Category) + "."
	}
	return f
}
