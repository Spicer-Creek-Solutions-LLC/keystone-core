package state

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const auditSelect = `SELECT
    id, timestamp, policy_id, policy_name, policy_type,
    resource_type, allowed, duration_ns, violations,
    enforcement_mode, severity, "user", action, metadata
FROM audit_entries`

// CreateAuditEntry persists a single entry. Wraps state.ErrDuplicate
// on PRIMARY KEY collision so the emission hooks (Epic 12 task 4)
// can distinguish a retry from a real failure with errors.Is.
func (s *SQLiteStore) CreateAuditEntry(ctx context.Context, r *AuditEntryStoreRecord) error {
	if err := validateAuditEntryForCreate(r); err != nil {
		return err
	}
	meta, err := encodeMetadata(r.Metadata)
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
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID,
		tsArgRequired(r.Timestamp),
		r.PolicyID, r.PolicyName, r.PolicyType,
		r.ResourceType,
		boolArgSQLite(r.Allowed),
		r.DurationNS,
		string(violations),
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

// CreateAuditEntriesBatch is atomic: every record validates before
// the transaction opens; the transaction inserts every row;
// rollback on any failure. Empty slice is a no-op.
func (s *SQLiteStore) CreateAuditEntriesBatch(ctx context.Context, recs []*AuditEntryStoreRecord) error {
	if len(recs) == 0 {
		return nil
	}
	for i, r := range recs {
		if err := validateAuditEntryForCreate(r); err != nil {
			return fmt.Errorf("state: CreateAuditEntriesBatch [%d]: %w", i, err)
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("state: CreateAuditEntriesBatch begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO audit_entries (
    id, timestamp, policy_id, policy_name, policy_type,
    resource_type, allowed, duration_ns, violations,
    enforcement_mode, severity, "user", action, metadata
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("state: CreateAuditEntriesBatch prepare: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for i, r := range recs {
		meta, err := encodeMetadata(r.Metadata)
		if err != nil {
			return fmt.Errorf("state: CreateAuditEntriesBatch [%d] metadata: %w", i, err)
		}
		violations := r.Violations
		if len(violations) == 0 {
			violations = []byte("[]")
		}
		if _, err := stmt.ExecContext(ctx,
			r.ID,
			tsArgRequired(r.Timestamp),
			r.PolicyID, r.PolicyName, r.PolicyType,
			r.ResourceType,
			boolArgSQLite(r.Allowed),
			r.DurationNS,
			string(violations),
			r.EnforcementMode, r.Severity,
			r.User, r.Action,
			meta,
		); err != nil {
			if isDuplicateKeyError(err) {
				return fmt.Errorf("state: CreateAuditEntriesBatch [%d]: %w", i, ErrDuplicate)
			}
			return fmt.Errorf("state: CreateAuditEntriesBatch [%d]: %w", i, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("state: CreateAuditEntriesBatch commit: %w", err)
	}
	return nil
}

// GetAuditEntry returns the record by id, or state.ErrNotFound.
func (s *SQLiteStore) GetAuditEntry(ctx context.Context, id string) (*AuditEntryStoreRecord, error) {
	row := s.db.QueryRowContext(ctx, auditSelect+" WHERE id = ?", id)
	rec, err := scanAuditEntrySQLite(row)
	if err != nil {
		return nil, translateSQLError(err)
	}
	return rec, nil
}

// ListAuditEntries returns records matching filter, ordered by id
// (UUIDv7 == time-order). Cursor-based pagination.
func (s *SQLiteStore) ListAuditEntries(ctx context.Context, filter AuditEntryFilter) ([]*AuditEntryStoreRecord, error) {
	var (
		sb    strings.Builder
		args  []any
		conds []string
	)
	sb.WriteString(auditSelect)

	conds, args = appendAuditConditionsSQLite(conds, args, filter)
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
		rec, err := scanAuditEntrySQLite(rows)
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

// CountAuditEntries returns the number of rows matching filter.
// Cursor / Limit / Descending fields are ignored.
func (s *SQLiteStore) CountAuditEntries(ctx context.Context, filter AuditEntryFilter) (int, error) {
	var (
		sb    strings.Builder
		args  []any
		conds []string
	)
	sb.WriteString("SELECT COUNT(*) FROM audit_entries")

	pruned := filter
	pruned.Cursor = ""
	pruned.Limit = 0
	pruned.Descending = false

	conds, args = appendAuditConditionsSQLite(conds, args, pruned)
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

// DeleteAuditEntry removes a single entry by id. Returns
// state.ErrNotFound when absent.
func (s *SQLiteStore) DeleteAuditEntry(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM audit_entries WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("state: DeleteAuditEntry: %w", err)
	}
	return affectsRow(res)
}

// ApplyAuditRetention applies the policy and returns the number of
// rows removed. MinSeverity exemption: entries at or above the
// threshold are NEVER deleted. Both MaxAge and MaxCount may be set;
// each operates independently.
func (s *SQLiteStore) ApplyAuditRetention(ctx context.Context, policy AuditRetentionPolicy) (int, error) {
	if policy.MaxAge <= 0 && policy.MaxCount <= 0 {
		return 0, nil
	}
	exempt := severitiesAtLeastForRetention(policy.MinSeverity)
	now := time.Now().UTC()
	total := 0

	if policy.MaxAge > 0 {
		cutoff := now.Add(-policy.MaxAge)
		query := `DELETE FROM audit_entries WHERE timestamp < ?`
		args := []any{tsArgRequired(cutoff)}
		if len(exempt) > 0 {
			placeholders := strings.Repeat("?,", len(exempt))
			placeholders = placeholders[:len(placeholders)-1]
			// #nosec G202 -- placeholders is "?,?,..." built from a fixed
			// enum (severity levels); no user input enters the SQL string.
			query += fmt.Sprintf(" AND severity NOT IN (%s)", placeholders)
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
		// Delete entries beyond the newest MaxCount, exempting those
		// at-or-above MinSeverity.
		query := `DELETE FROM audit_entries WHERE id NOT IN (
            SELECT id FROM audit_entries ORDER BY id DESC LIMIT ?
        )`
		args := []any{policy.MaxCount}
		if len(exempt) > 0 {
			placeholders := strings.Repeat("?,", len(exempt))
			placeholders = placeholders[:len(placeholders)-1]
			// #nosec G202 -- placeholders is "?,?,..." built from a fixed
			// enum (severity levels); no user input enters the SQL string.
			query += fmt.Sprintf(" AND severity NOT IN (%s)", placeholders)
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

// appendAuditConditionsSQLite builds the WHERE clause arg-list for
// both ListAuditEntries and CountAuditEntries.
func appendAuditConditionsSQLite(conds []string, args []any, f AuditEntryFilter) ([]string, []any) {
	if f.PolicyID != "" {
		conds = append(conds, "policy_id = ?")
		args = append(args, f.PolicyID)
	}
	if f.User != "" {
		conds = append(conds, `"user" = ?`)
		args = append(args, f.User)
	}
	if f.ResourceType != "" {
		conds = append(conds, "resource_type = ?")
		args = append(args, f.ResourceType)
	}
	if f.Action != "" {
		conds = append(conds, "action = ?")
		args = append(args, f.Action)
	}
	if len(f.Severities) > 0 {
		placeholders := strings.Repeat("?,", len(f.Severities))
		placeholders = placeholders[:len(placeholders)-1]
		conds = append(conds, "severity IN ("+placeholders+")")
		for _, sev := range f.Severities {
			args = append(args, sev)
		}
	}
	if f.Allowed != nil {
		conds = append(conds, "allowed = ?")
		args = append(args, boolArgSQLite(*f.Allowed))
	}
	if !f.Since.IsZero() {
		conds = append(conds, "timestamp >= ?")
		args = append(args, tsArgRequired(f.Since))
	}
	if !f.Until.IsZero() {
		conds = append(conds, "timestamp < ?")
		args = append(args, tsArgRequired(f.Until))
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

func scanAuditEntrySQLite(r rowLike) (*AuditEntryStoreRecord, error) {
	var (
		rec        AuditEntryStoreRecord
		tsStr      string
		allowed    int
		violations string
		metadata   string
	)
	if err := r.Scan(
		&rec.ID, &tsStr,
		&rec.PolicyID, &rec.PolicyName, &rec.PolicyType,
		&rec.ResourceType,
		&allowed, &rec.DurationNS,
		&violations,
		&rec.EnforcementMode, &rec.Severity,
		&rec.User, &rec.Action,
		&metadata,
	); err != nil {
		return nil, err
	}
	t, err := tsParseRequired(tsStr)
	if err != nil {
		return nil, fmt.Errorf("state: scan audit_entries timestamp: %w", err)
	}
	rec.Timestamp = t
	rec.Allowed = allowed != 0
	rec.Violations = []byte(violations)
	if rec.Metadata, err = decodeMetadata(metadata); err != nil {
		return nil, fmt.Errorf("state: scan audit_entries metadata: %w", err)
	}
	return &rec, nil
}

// validateAuditEntryForCreate is the shared pre-INSERT shape check.
func validateAuditEntryForCreate(r *AuditEntryStoreRecord) error {
	if r == nil {
		return fmt.Errorf("state: CreateAuditEntry: nil record")
	}
	if r.ID == "" {
		return fmt.Errorf("state: CreateAuditEntry: ID is required")
	}
	if r.Timestamp.IsZero() {
		return fmt.Errorf("state: CreateAuditEntry: Timestamp is required")
	}
	if r.Action == "" {
		return fmt.Errorf("state: CreateAuditEntry: Action is required")
	}
	if r.Severity == "" {
		return fmt.Errorf("state: CreateAuditEntry: Severity is required")
	}
	if r.EnforcementMode == "" {
		return fmt.Errorf("state: CreateAuditEntry: EnforcementMode is required")
	}
	return nil
}

// severitiesAtLeastForRetention returns the canonical severity
// names at or above minSeverity in the §4.12 ordering
// (low < medium < high < critical). Empty input returns nil — no
// retention exemption (every entry subject to deletion).
//
// Used by ApplyAuditRetention to build the `severity NOT IN (...)`
// exempt-from-deletion clause.
func severitiesAtLeastForRetention(minSeverity string) []string {
	if minSeverity == "" {
		return nil
	}
	all := []string{"low", "medium", "high", "critical"}
	idx := -1
	for i, s := range all {
		if s == minSeverity {
			idx = i
			break
		}
	}
	if idx < 0 {
		// Unknown severity name → no exemption rather than panic.
		// The audit-package validator catches this case before it
		// reaches the SQL layer.
		return nil
	}
	return all[idx:]
}

// Compile-time interface compliance.
var _ AuditStore = (*SQLiteStore)(nil)
