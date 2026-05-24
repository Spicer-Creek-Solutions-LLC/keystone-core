// SPDX-License-Identifier: Apache-2.0

package state

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const postgresEventSelect = `SELECT
    id, type, source, time, severity,
    correlation_id, tags, data, subject
FROM events`

// CreateEvent is the Postgres counterpart of the SQLite impl. JSONB
// columns via marshalJSONBytes; timestamp via TIMESTAMPTZ.
func (s *PostgreSQLStore) CreateEvent(ctx context.Context, r *EventStoreRecord) error {
	if err := validateEventForCreate(r); err != nil {
		return err
	}
	tags, err := marshalJSONBytes(orEmptyStringMap(r.Tags))
	if err != nil {
		return fmt.Errorf("state: CreateEvent tags: %w", err)
	}
	data, err := marshalJSONBytes(orEmptyAnyMap(r.Data))
	if err != nil {
		return fmt.Errorf("state: CreateEvent data: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO events (
    id, type, source, time, severity,
    correlation_id, tags, data, subject
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		r.ID, r.Type, r.Source,
		r.Time.UTC(),
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

// CreateEventsBatch packs every record into a single multi-row INSERT
// — atomic by Postgres semantics. Empty slice is a no-op.
func (s *PostgreSQLStore) CreateEventsBatch(ctx context.Context, recs []*EventStoreRecord) error {
	if len(recs) == 0 {
		return nil
	}
	for i, r := range recs {
		if err := validateEventForCreate(r); err != nil {
			return fmt.Errorf("state: CreateEventsBatch [%d]: %w", i, err)
		}
	}

	const cols = 9
	var (
		sb   strings.Builder
		args = make([]any, 0, len(recs)*cols)
	)
	sb.WriteString(`INSERT INTO events (
    id, type, source, time, severity,
    correlation_id, tags, data, subject
) VALUES `)
	for i, r := range recs {
		if i > 0 {
			sb.WriteString(", ")
		}
		base := i*cols + 1
		fmt.Fprintf(&sb, "($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			base, base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8)

		tags, err := marshalJSONBytes(orEmptyStringMap(r.Tags))
		if err != nil {
			return fmt.Errorf("state: CreateEventsBatch [%d] tags: %w", i, err)
		}
		data, err := marshalJSONBytes(orEmptyAnyMap(r.Data))
		if err != nil {
			return fmt.Errorf("state: CreateEventsBatch [%d] data: %w", i, err)
		}
		args = append(args,
			r.ID, r.Type, r.Source,
			r.Time.UTC(),
			r.Severity,
			r.CorrelationID, tags, data, r.Subject,
		)
	}

	if _, err := s.db.ExecContext(ctx, sb.String(), args...); err != nil {
		if isDuplicateKeyError(err) {
			return fmt.Errorf("state: CreateEventsBatch: %w", ErrDuplicate)
		}
		return fmt.Errorf("state: CreateEventsBatch: %w", err)
	}
	return nil
}

func (s *PostgreSQLStore) GetEvent(ctx context.Context, id string) (*EventStoreRecord, error) {
	row := s.db.QueryRowContext(ctx, postgresEventSelect+" WHERE id = $1", id)
	rec, err := scanEventPostgres(row)
	if err != nil {
		return nil, translateSQLError(err)
	}
	return rec, nil
}

func (s *PostgreSQLStore) ListEvents(ctx context.Context, filter EventFilter) ([]*EventStoreRecord, error) {
	var (
		sb    strings.Builder
		args  []any
		conds []string
		argN  int
	)
	sb.WriteString(postgresEventSelect)

	conds, args, _ = appendEventConditionsPostgres(conds, args, argN, filter)
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
		rec, err := scanEventPostgres(rows)
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

func (s *PostgreSQLStore) CountEvents(ctx context.Context, filter EventFilter) (int, error) {
	var (
		sb    strings.Builder
		args  []any
		conds []string
		argN  int
	)
	sb.WriteString("SELECT COUNT(*) FROM events")

	pruned := filter
	pruned.Cursor = ""
	pruned.Limit = 0
	pruned.Descending = false

	conds, args, _ = appendEventConditionsPostgres(conds, args, argN, pruned)
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

func (s *PostgreSQLStore) DeleteEvent(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM events WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("state: DeleteEvent: %w", err)
	}
	return affectsRow(res)
}

func (s *PostgreSQLStore) ApplyEventsRetention(ctx context.Context, policies []EventsRetentionPolicy) (int, error) {
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
			query := `DELETE FROM events WHERE time < $1`
			args := []any{cutoff}
			if p.Type != "" {
				query += ` AND type = $2`
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
			var (
				query string
				args  []any
			)
			if p.Type != "" {
				query = `DELETE FROM events WHERE id NOT IN (
                    SELECT id FROM events WHERE type = $1 ORDER BY id DESC LIMIT $2
                ) AND type = $1`
				args = []any{p.Type, p.MaxCount}
			} else {
				query = `DELETE FROM events WHERE id NOT IN (
                    SELECT id FROM events ORDER BY id DESC LIMIT $1
                )`
				args = []any{p.MaxCount}
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

// appendEventConditionsPostgres builds the WHERE clause arg-list with
// positional $n placeholders. argN is the next placeholder index to
// emit; the helper returns the updated index for chained callers.
func appendEventConditionsPostgres(conds []string, args []any, argN int, f EventFilter) ([]string, []any, int) {
	next := func() string {
		argN++
		return fmt.Sprintf("$%d", argN)
	}
	if f.Type != "" {
		conds = append(conds, "type = "+next())
		args = append(args, f.Type)
	}
	if f.TypePrefix != "" {
		// See note in sqlite_events.go appendEventConditionsSQLite — the
		// prefix never contains LIKE wildcards.
		conds = append(conds, "type LIKE "+next())
		args = append(args, f.TypePrefix+"%")
	}
	if f.Source != "" {
		conds = append(conds, "source = "+next())
		args = append(args, f.Source)
	}
	if len(f.Severities) > 0 {
		placeholders := make([]string, 0, len(f.Severities))
		for _, sev := range f.Severities {
			placeholders = append(placeholders, next())
			args = append(args, sev)
		}
		conds = append(conds, "severity IN ("+strings.Join(placeholders, ", ")+")")
	}
	if f.CorrelationID != "" {
		conds = append(conds, "correlation_id = "+next())
		args = append(args, f.CorrelationID)
	}
	if !f.Since.IsZero() {
		conds = append(conds, "time >= "+next())
		args = append(args, f.Since.UTC())
	}
	if !f.Until.IsZero() {
		conds = append(conds, "time < "+next())
		args = append(args, f.Until.UTC())
	}
	for k, v := range f.Tags {
		// tags->>$k = $v — both key and value bound through placeholders.
		keyPh := next()
		valPh := next()
		conds = append(conds, fmt.Sprintf("tags->>%s = %s", keyPh, valPh))
		args = append(args, k, v)
	}
	if f.Cursor != "" {
		op := ">"
		if f.Descending {
			op = "<"
		}
		conds = append(conds, "id "+op+" "+next())
		args = append(args, f.Cursor)
	}
	return conds, args, argN
}

func scanEventPostgres(r rowLike) (*EventStoreRecord, error) {
	var (
		rec  EventStoreRecord
		tags []byte
		data []byte
	)
	if err := r.Scan(
		&rec.ID, &rec.Type, &rec.Source,
		&rec.Time, &rec.Severity,
		&rec.CorrelationID, &tags, &data, &rec.Subject,
	); err != nil {
		return nil, err
	}
	if len(tags) > 0 && string(tags) != "{}" {
		if err := unmarshalJSONBytes(tags, &rec.Tags); err != nil {
			return nil, fmt.Errorf("state: scan event tags: %w", err)
		}
	}
	if len(data) > 0 && string(data) != "{}" {
		if err := unmarshalJSONBytes(data, &rec.Data); err != nil {
			return nil, fmt.Errorf("state: scan event data: %w", err)
		}
	}
	return &rec, nil
}

// orEmptyStringMap normalises a nil tag map to an empty map[string]string
// so marshalJSONBytes never emits a bare `null` into the JSONB column.
// Mirrors the pattern in postgres_leases.go's metadata handling.
func orEmptyStringMap(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

// orEmptyAnyMap is the data-side counterpart of orEmptyStringMap.
func orEmptyAnyMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

// Compile-time interface compliance check.
var _ EventStore = (*PostgreSQLStore)(nil)
