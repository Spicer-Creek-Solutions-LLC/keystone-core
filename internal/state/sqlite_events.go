package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const eventSelect = `SELECT
    id, type, source, time, severity,
    correlation_id, tags, data, subject
FROM events`

// CreateEvent persists a single event. Wraps state.ErrDuplicate on
// PRIMARY KEY collision so the JetStreamPublisher (task 3) can
// distinguish a retry from a real failure with errors.Is.
func (s *SQLiteStore) CreateEvent(ctx context.Context, r *EventStoreRecord) error {
	if err := validateEventForCreate(r); err != nil {
		return err
	}
	tags, err := encodeMetadata(r.Tags)
	if err != nil {
		return fmt.Errorf("state: CreateEvent tags: %w", err)
	}
	data, err := encodeAnyMap(r.Data)
	if err != nil {
		return fmt.Errorf("state: CreateEvent data: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO events (
    id, type, source, time, severity,
    correlation_id, tags, data, subject
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Type, r.Source,
		tsArgRequired(r.Time),
		r.Severity,
		r.CorrelationID, tags, data, r.Subject,
	)
	if err != nil {
		if isDuplicateKeyError(err) {
			return fmt.Errorf("state: CreateEvent: %w", ErrDuplicate)
		}
		return fmt.Errorf("state: CreateEvent: %w", err)
	}
	return nil
}

// CreateEventsBatch is atomic: every record validates before the
// transaction opens; the transaction inserts every row; rollback on
// any failure. Empty slice is a no-op (nil error).
func (s *SQLiteStore) CreateEventsBatch(ctx context.Context, recs []*EventStoreRecord) error {
	if len(recs) == 0 {
		return nil
	}
	for i, r := range recs {
		if err := validateEventForCreate(r); err != nil {
			return fmt.Errorf("state: CreateEventsBatch [%d]: %w", i, err)
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("state: CreateEventsBatch begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO events (
    id, type, source, time, severity,
    correlation_id, tags, data, subject
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("state: CreateEventsBatch prepare: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for i, r := range recs {
		tags, err := encodeMetadata(r.Tags)
		if err != nil {
			return fmt.Errorf("state: CreateEventsBatch [%d] tags: %w", i, err)
		}
		data, err := encodeAnyMap(r.Data)
		if err != nil {
			return fmt.Errorf("state: CreateEventsBatch [%d] data: %w", i, err)
		}
		if _, err := stmt.ExecContext(ctx,
			r.ID, r.Type, r.Source,
			tsArgRequired(r.Time),
			r.Severity,
			r.CorrelationID, tags, data, r.Subject,
		); err != nil {
			if isDuplicateKeyError(err) {
				return fmt.Errorf("state: CreateEventsBatch [%d]: %w", i, ErrDuplicate)
			}
			return fmt.Errorf("state: CreateEventsBatch [%d]: %w", i, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("state: CreateEventsBatch commit: %w", err)
	}
	return nil
}

// GetEvent returns the record by id, or state.ErrNotFound.
func (s *SQLiteStore) GetEvent(ctx context.Context, id string) (*EventStoreRecord, error) {
	row := s.db.QueryRowContext(ctx, eventSelect+" WHERE id = ?", id)
	rec, err := scanEventSQLite(row)
	if err != nil {
		return nil, translateSQLError(err)
	}
	return rec, nil
}

// ListEvents returns records matching filter, ordered by id (which is
// time-order because Event.ID is a UUIDv7). Cursor-based pagination:
// pass the previous page's last id to continue.
func (s *SQLiteStore) ListEvents(ctx context.Context, filter EventFilter) ([]*EventStoreRecord, error) {
	var (
		sb    strings.Builder
		args  []any
		conds []string
	)
	sb.WriteString(eventSelect)

	conds, args = appendEventConditionsSQLite(conds, args, filter)
	if len(conds) > 0 {
		sb.WriteString(" WHERE ")
		sb.WriteString(strings.Join(conds, " AND "))
	}
	if filter.Descending {
		sb.WriteString(" ORDER BY id DESC")
	} else {
		sb.WriteString(" ORDER BY id ASC")
	}
	if filter.Limit > 0 {
		fmt.Fprintf(&sb, " LIMIT %d", filter.Limit)
	}

	rows, err := s.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("state: ListEvents: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*EventStoreRecord
	for rows.Next() {
		rec, err := scanEventSQLite(rows)
		if err != nil {
			return nil, fmt.Errorf("state: ListEvents scan: %w", err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("state: ListEvents iterate: %w", err)
	}
	return out, nil
}

// CountEvents returns the number of rows matching filter.
// Cursor + Limit fields are ignored — Count is the unbounded total.
func (s *SQLiteStore) CountEvents(ctx context.Context, filter EventFilter) (int, error) {
	var (
		sb    strings.Builder
		args  []any
		conds []string
	)
	sb.WriteString("SELECT COUNT(*) FROM events")

	// CountEvents intentionally ignores Cursor/Limit/Descending.
	pruned := filter
	pruned.Cursor = ""
	pruned.Limit = 0
	pruned.Descending = false

	conds, args = appendEventConditionsSQLite(conds, args, pruned)
	if len(conds) > 0 {
		sb.WriteString(" WHERE ")
		sb.WriteString(strings.Join(conds, " AND "))
	}

	var n int
	if err := s.db.QueryRowContext(ctx, sb.String(), args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("state: CountEvents: %w", err)
	}
	return n, nil
}

// DeleteEvent removes a single event by id. Returns state.ErrNotFound
// when absent so callers can branch with errors.Is.
func (s *SQLiteStore) DeleteEvent(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM events WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("state: DeleteEvent: %w", err)
	}
	return affectsRow(res)
}

// ApplyEventsRetention applies every policy in order and returns the
// total number of rows removed. Each policy may delete by age, by
// count, or both. Policies are independent — rows already removed by
// a prior policy are simply absent from subsequent policies' candidate
// sets.
func (s *SQLiteStore) ApplyEventsRetention(ctx context.Context, policies []EventsRetentionPolicy) (int, error) {
	if len(policies) == 0 {
		return 0, nil
	}
	now := time.Now().UTC()
	total := 0
	for _, p := range policies {
		if p.MaxAge <= 0 && p.MaxCount <= 0 {
			continue
		}
		if p.MaxAge > 0 {
			cutoff := now.Add(-p.MaxAge)
			query := `DELETE FROM events WHERE time < ?`
			args := []any{tsArgRequired(cutoff)}
			if p.Type != "" {
				query += ` AND type = ?`
				args = append(args, p.Type)
			}
			res, err := s.db.ExecContext(ctx, query, args...)
			if err != nil {
				return total, fmt.Errorf("state: ApplyEventsRetention max-age: %w", err)
			}
			n, _ := res.RowsAffected()
			total += int(n)
		}
		if p.MaxCount > 0 {
			query := `DELETE FROM events WHERE id NOT IN (
                SELECT id FROM events`
			var args []any
			if p.Type != "" {
				query += ` WHERE type = ?`
				args = append(args, p.Type)
			}
			query += ` ORDER BY id DESC LIMIT ?)`
			args = append(args, p.MaxCount)
			if p.Type != "" {
				query += ` AND type = ?`
				args = append(args, p.Type)
			}
			res, err := s.db.ExecContext(ctx, query, args...)
			if err != nil {
				return total, fmt.Errorf("state: ApplyEventsRetention max-count: %w", err)
			}
			n, _ := res.RowsAffected()
			total += int(n)
		}
	}
	return total, nil
}

// appendEventConditionsSQLite builds the WHERE clause arg-list for
// both ListEvents and CountEvents. Shared so both query paths apply
// the filter identically — a list page's "no rows" matches a count
// of zero.
func appendEventConditionsSQLite(conds []string, args []any, f EventFilter) ([]string, []any) {
	if f.Type != "" {
		conds = append(conds, "type = ?")
		args = append(args, f.Type)
	}
	if f.TypePrefix != "" {
		// TypePrefix only flows in via internal/events.filterFromQuery
		// as `<Category>.`. Category is a closed alphanumeric enum, so
		// the prefix contains no LIKE wildcards — no escape needed.
		conds = append(conds, "type LIKE ?")
		args = append(args, f.TypePrefix+"%")
	}
	if f.Source != "" {
		conds = append(conds, "source = ?")
		args = append(args, f.Source)
	}
	if len(f.Severities) > 0 {
		placeholders := strings.Repeat("?,", len(f.Severities))
		placeholders = placeholders[:len(placeholders)-1] // trim trailing comma
		conds = append(conds, "severity IN ("+placeholders+")")
		for _, sev := range f.Severities {
			args = append(args, sev)
		}
	}
	if f.CorrelationID != "" {
		conds = append(conds, "correlation_id = ?")
		args = append(args, f.CorrelationID)
	}
	if !f.Since.IsZero() {
		conds = append(conds, "time >= ?")
		args = append(args, tsArgRequired(f.Since))
	}
	if !f.Until.IsZero() {
		conds = append(conds, "time < ?")
		args = append(args, tsArgRequired(f.Until))
	}
	for k, v := range f.Tags {
		conds = append(conds, "json_extract(tags, '$.'||?) = ?")
		args = append(args, k, v)
	}
	if f.Cursor != "" {
		if f.Descending {
			conds = append(conds, "id < ?")
		} else {
			conds = append(conds, "id > ?")
		}
		args = append(args, f.Cursor)
	}
	return conds, args
}

func scanEventSQLite(r rowLike) (*EventStoreRecord, error) {
	var (
		rec      EventStoreRecord
		timeStr  string
		tags     string
		data     string
	)
	if err := r.Scan(
		&rec.ID, &rec.Type, &rec.Source,
		&timeStr, &rec.Severity,
		&rec.CorrelationID, &tags, &data, &rec.Subject,
	); err != nil {
		return nil, err
	}
	t, err := tsParseRequired(timeStr)
	if err != nil {
		return nil, fmt.Errorf("state: scan event time: %w", err)
	}
	rec.Time = t
	if rec.Tags, err = decodeMetadata(tags); err != nil {
		return nil, fmt.Errorf("state: scan event tags: %w", err)
	}
	if rec.Data, err = decodeAnyMap(data); err != nil {
		return nil, fmt.Errorf("state: scan event data: %w", err)
	}
	return &rec, nil
}

// validateEventForCreate is the shared pre-INSERT shape check.
func validateEventForCreate(r *EventStoreRecord) error {
	if r == nil {
		return fmt.Errorf("state: CreateEvent: nil record")
	}
	if r.ID == "" {
		return fmt.Errorf("state: CreateEvent: ID is required")
	}
	if r.Type == "" {
		return fmt.Errorf("state: CreateEvent: Type is required")
	}
	if r.Source == "" {
		return fmt.Errorf("state: CreateEvent: Source is required")
	}
	if r.Time.IsZero() {
		return fmt.Errorf("state: CreateEvent: Time is required")
	}
	if r.Severity == "" {
		return fmt.Errorf("state: CreateEvent: Severity is required")
	}
	return nil
}

// encodeAnyMap renders a map[string]any as JSON. nil + empty both
// round-trip to "{}" so the column NOT NULL DEFAULT '{}' is honored.
// Mirrors encodeMetadata in shape; separate function so it stays
// typed and the test-side reads cleanly.
func encodeAnyMap(m map[string]any) (string, error) {
	if len(m) == 0 {
		return "{}", nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// decodeAnyMap parses a JSON-encoded map[string]any. Empty input or
// "{}" returns nil for assertion-friendliness (matches decodeMetadata
// shape).
func decodeAnyMap(s string) (map[string]any, error) {
	if s == "" || s == "{}" {
		return nil, nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Compile-time interface compliance check.
var _ EventStore = (*SQLiteStore)(nil)

// scanEventForRow is a thin adapter type so future callers in the same
// package can pass a *sql.Row or *sql.Rows without separate shims.
// Currently unused outside this file but documented to lock the
// rowLike contract.
var _ = (*sql.Row)(nil)
