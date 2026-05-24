// SPDX-License-Identifier: Apache-2.0

package state

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const postgresAuditSelect = `SELECT
    id, timestamp, policy_id, policy_name, policy_type,
    resource_type, allowed, duration_ns, violations,
    enforcement_mode, severity, "user", action, metadata
FROM audit_entries`

// CreateAuditEntry is the Postgres counterpart of the SQLite impl.
// JSONB columns via marshalJSONBytes; TIMESTAMPTZ for timestamp.
func (s *PostgreSQLStore) CreateAuditEntry(ctx context.Context, r *AuditEntryStoreRecord) error {
	if err := validateAuditEntryForCreate(r); err != nil {
		return err
	}
	meta, err := marshalJSONBytes(orEmptyStringMap(r.Metadata))
	if err != nil {
		return fmt.Errorf("state: CreateAuditEntry metadata: %w", err)
	}
	violations := r.Violations
	if len(violations) == 0 {
		violations = []byte("[]")
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO audit_entries (
    id, timestamp, policy_id, policy_name, policy_type,
    resource_type, allowed, duration_ns, violations,
    enforcement_mode, severity, "user", action, metadata
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		r.ID,
		r.Timestamp.UTC(),
		r.PolicyID, r.PolicyName, r.PolicyType,
		r.ResourceType,
		r.Allowed,
		r.DurationNS,
		violations,
		r.EnforcementMode, r.Severity,
		r.User, r.Action,
		meta,
	)
	if err != nil {
		if isDuplicateKeyError(err) {
			return fmt.Errorf("state: CreateAuditEntry: %w", ErrDuplicate)
		}
		return fmt.Errorf("state: CreateAuditEntry: %w", err)
	}
	return nil
}

// CreateAuditEntriesBatch packs every record into a single multi-row
// INSERT — atomic by Postgres semantics.
func (s *PostgreSQLStore) CreateAuditEntriesBatch(ctx context.Context, recs []*AuditEntryStoreRecord) error {
	if len(recs) == 0 {
		return nil
	}
	for i, r := range recs {
		if err := validateAuditEntryForCreate(r); err != nil {
			return fmt.Errorf("state: CreateAuditEntriesBatch [%d]: %w", i, err)
		}
	}

	const cols = 14
	var (
		sb   strings.Builder
		args = make([]any, 0, len(recs)*cols)
	)
	sb.WriteString(`INSERT INTO audit_entries (
    id, timestamp, policy_id, policy_name, policy_type,
    resource_type, allowed, duration_ns, violations,
    enforcement_mode, severity, "user", action, metadata
) VALUES `)
	for i, r := range recs {
		if i > 0 {
			sb.WriteString(", ")
		}
		base := i*cols + 1
		fmt.Fprintf(&sb, "($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			base, base+1, base+2, base+3, base+4, base+5, base+6,
			base+7, base+8, base+9, base+10, base+11, base+12, base+13)

		meta, err := marshalJSONBytes(orEmptyStringMap(r.Metadata))
		if err != nil {
			return fmt.Errorf("state: CreateAuditEntriesBatch [%d] metadata: %w", i, err)
		}
		violations := r.Violations
		if len(violations) == 0 {
			violations = []byte("[]")
		}
		args = append(args,
			r.ID,
			r.Timestamp.UTC(),
			r.PolicyID, r.PolicyName, r.PolicyType,
			r.ResourceType,
			r.Allowed,
			r.DurationNS,
			violations,
			r.EnforcementMode, r.Severity,
			r.User, r.Action,
			meta,
		)
	}

	if _, err := s.db.ExecContext(ctx, sb.String(), args...); err != nil {
		if isDuplicateKeyError(err) {
			return fmt.Errorf("state: CreateAuditEntriesBatch: %w", ErrDuplicate)
		}
		return fmt.Errorf("state: CreateAuditEntriesBatch: %w", err)
	}
	return nil
}

func (s *PostgreSQLStore) GetAuditEntry(ctx context.Context, id string) (*AuditEntryStoreRecord, error) {
	row := s.db.QueryRowContext(ctx, postgresAuditSelect+" WHERE id = $1", id)
	rec, err := scanAuditEntryPostgres(row)
	if err != nil {
		return nil, translateSQLError(err)
	}
	return rec, nil
}

func (s *PostgreSQLStore) ListAuditEntries(ctx context.Context, filter AuditEntryFilter) ([]*AuditEntryStoreRecord, error) {
	var (
		sb    strings.Builder
		args  []any
		conds []string
		argN  int
	)
	sb.WriteString(postgresAuditSelect)

	conds, args, _ = appendAuditConditionsPostgres(conds, args, argN, filter)
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
		return nil, fmt.Errorf("state: ListAuditEntries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*AuditEntryStoreRecord
	for rows.Next() {
		rec, err := scanAuditEntryPostgres(rows)
		if err != nil {
			return nil, fmt.Errorf("state: ListAuditEntries scan: %w", err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("state: ListAuditEntries iterate: %w", err)
	}
	return out, nil
}

func (s *PostgreSQLStore) CountAuditEntries(ctx context.Context, filter AuditEntryFilter) (int, error) {
	var (
		sb    strings.Builder
		args  []any
		conds []string
		argN  int
	)
	sb.WriteString("SELECT COUNT(*) FROM audit_entries")

	pruned := filter
	pruned.Cursor = ""
	pruned.Limit = 0
	pruned.Descending = false

	conds, args, _ = appendAuditConditionsPostgres(conds, args, argN, pruned)
	if len(conds) > 0 {
		sb.WriteString(" WHERE ")
		sb.WriteString(strings.Join(conds, " AND "))
	}

	var n int
	if err := s.db.QueryRowContext(ctx, sb.String(), args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("state: CountAuditEntries: %w", err)
	}
	return n, nil
}

// SummarizeAuditEntries aggregates over the filter set. Three
// sequential queries: totals + time-range, denied-by-policy_id,
// denied-by-severity. Cursor / Limit / Descending fields are ignored.
func (s *PostgreSQLStore) SummarizeAuditEntries(ctx context.Context, filter AuditEntryFilter) (AuditEntrySummaryRecord, error) {
	var (
		summary AuditEntrySummaryRecord
		args    []any
		conds   []string
	)
	pruned := filter
	pruned.Cursor = ""
	pruned.Limit = 0
	pruned.Descending = false
	conds, args, _ = appendAuditConditionsPostgres(conds, args, 0, pruned)
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	var (
		total, allowedCnt, deniedCnt int
		minTs, maxTs                 sql.NullTime
	)
	// #nosec G202 -- `where` is composed of static "field = $n" /
	// "field IN ($n,...)" / "id (>|<) $n" fragments built by
	// appendAuditConditionsPostgres from a closed set of filter fields;
	// no user input enters the SQL string.
	q1 := `SELECT
        COUNT(*),
        COALESCE(SUM(CASE WHEN allowed THEN 1 ELSE 0 END), 0),
        COALESCE(SUM(CASE WHEN NOT allowed THEN 1 ELSE 0 END), 0),
        MIN(timestamp),
        MAX(timestamp)
    FROM audit_entries` + where
	if err := s.db.QueryRowContext(ctx, q1, args...).Scan(
		&total, &allowedCnt, &deniedCnt, &minTs, &maxTs,
	); err != nil {
		return AuditEntrySummaryRecord{}, fmt.Errorf("state: SummarizeAuditEntries totals: %w", err)
	}
	summary.TotalEvaluations = total
	summary.AllowedCount = allowedCnt
	summary.DeniedCount = deniedCnt
	if minTs.Valid {
		summary.RangeStart = minTs.Time.UTC()
	}
	if maxTs.Valid {
		summary.RangeEnd = maxTs.Time.UTC()
	}

	if total == 0 {
		return summary, nil
	}
	deniedWhere := where
	if deniedWhere == "" {
		deniedWhere = " WHERE NOT allowed"
	} else {
		deniedWhere += " AND NOT allowed"
	}
	// #nosec G202 -- see q1 justification.
	q2 := `SELECT policy_id, COUNT(*) FROM audit_entries` + deniedWhere + ` GROUP BY policy_id`
	byPolicy, err := scanGroupCounts(ctx, s.db, q2, args)
	if err != nil {
		return AuditEntrySummaryRecord{}, fmt.Errorf("state: SummarizeAuditEntries by policy: %w", err)
	}
	summary.ViolationsByPolicy = byPolicy

	// #nosec G202 -- see q1 justification.
	q3 := `SELECT severity, COUNT(*) FROM audit_entries` + deniedWhere + ` GROUP BY severity`
	bySev, err := scanGroupCounts(ctx, s.db, q3, args)
	if err != nil {
		return AuditEntrySummaryRecord{}, fmt.Errorf("state: SummarizeAuditEntries by severity: %w", err)
	}
	summary.ViolationsBySeverity = bySev

	return summary, nil
}

func (s *PostgreSQLStore) DeleteAuditEntry(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM audit_entries WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("state: DeleteAuditEntry: %w", err)
	}
	return affectsRow(res)
}

func (s *PostgreSQLStore) ApplyAuditRetention(ctx context.Context, policy AuditRetentionPolicy) (int, error) {
	if policy.MaxAge <= 0 && policy.MaxCount <= 0 {
		return 0, nil
	}
	exempt := severitiesAtLeastForRetention(policy.MinSeverity)
	now := time.Now().UTC()
	total := 0

	if policy.MaxAge > 0 {
		cutoff := now.Add(-policy.MaxAge)
		query := `DELETE FROM audit_entries WHERE timestamp < $1`
		args := []any{cutoff}
		if len(exempt) > 0 {
			placeholders := make([]string, len(exempt))
			for i := range exempt {
				placeholders[i] = fmt.Sprintf("$%d", i+2)
			}
			// #nosec G202 -- placeholders are positional ($n) tokens for
			// a fixed-enum severity list; no user input enters the SQL.
			query += fmt.Sprintf(" AND severity NOT IN (%s)", strings.Join(placeholders, ", "))
			for _, s := range exempt {
				args = append(args, s)
			}
		}
		res, err := s.db.ExecContext(ctx, query, args...)
		if err != nil {
			return total, fmt.Errorf("state: ApplyAuditRetention max-age: %w", err)
		}
		n, _ := res.RowsAffected()
		total += int(n)
	}

	if policy.MaxCount > 0 {
		query := `DELETE FROM audit_entries WHERE id NOT IN (
            SELECT id FROM audit_entries ORDER BY id DESC LIMIT $1
        )`
		args := []any{policy.MaxCount}
		if len(exempt) > 0 {
			placeholders := make([]string, len(exempt))
			for i := range exempt {
				placeholders[i] = fmt.Sprintf("$%d", i+2)
			}
			// #nosec G202 -- placeholders are positional ($n) tokens for
			// a fixed-enum severity list; no user input enters the SQL.
			query += fmt.Sprintf(" AND severity NOT IN (%s)", strings.Join(placeholders, ", "))
			for _, s := range exempt {
				args = append(args, s)
			}
		}
		res, err := s.db.ExecContext(ctx, query, args...)
		if err != nil {
			return total, fmt.Errorf("state: ApplyAuditRetention max-count: %w", err)
		}
		n, _ := res.RowsAffected()
		total += int(n)
	}
	return total, nil
}

// appendAuditConditionsPostgres builds the WHERE clause arg-list
// with positional $n placeholders.
func appendAuditConditionsPostgres(conds []string, args []any, argN int, f AuditEntryFilter) ([]string, []any, int) {
	next := func() string {
		argN++
		return fmt.Sprintf("$%d", argN)
	}
	if f.PolicyID != "" {
		conds = append(conds, "policy_id = "+next())
		args = append(args, f.PolicyID)
	}
	if f.User != "" {
		conds = append(conds, `"user" = `+next())
		args = append(args, f.User)
	}
	if f.ResourceType != "" {
		conds = append(conds, "resource_type = "+next())
		args = append(args, f.ResourceType)
	}
	if f.Action != "" {
		conds = append(conds, "action = "+next())
		args = append(args, f.Action)
	}
	if len(f.Severities) > 0 {
		placeholders := make([]string, 0, len(f.Severities))
		for _, sev := range f.Severities {
			placeholders = append(placeholders, next())
			args = append(args, sev)
		}
		conds = append(conds, "severity IN ("+strings.Join(placeholders, ", ")+")")
	}
	if f.Allowed != nil {
		conds = append(conds, "allowed = "+next())
		args = append(args, *f.Allowed)
	}
	if !f.Since.IsZero() {
		conds = append(conds, "timestamp >= "+next())
		args = append(args, f.Since.UTC())
	}
	if !f.Until.IsZero() {
		conds = append(conds, "timestamp < "+next())
		args = append(args, f.Until.UTC())
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

func scanAuditEntryPostgres(r rowLike) (*AuditEntryStoreRecord, error) {
	var (
		rec        AuditEntryStoreRecord
		violations []byte
		metadata   []byte
	)
	if err := r.Scan(
		&rec.ID, &rec.Timestamp,
		&rec.PolicyID, &rec.PolicyName, &rec.PolicyType,
		&rec.ResourceType,
		&rec.Allowed, &rec.DurationNS,
		&violations,
		&rec.EnforcementMode, &rec.Severity,
		&rec.User, &rec.Action,
		&metadata,
	); err != nil {
		return nil, err
	}
	rec.Violations = violations
	if len(metadata) > 0 && string(metadata) != "{}" {
		if err := unmarshalJSONBytes(metadata, &rec.Metadata); err != nil {
			return nil, fmt.Errorf("state: scan audit_entries metadata: %w", err)
		}
	}
	return &rec, nil
}

// Compile-time interface compliance.
var _ AuditStore = (*PostgreSQLStore)(nil)
