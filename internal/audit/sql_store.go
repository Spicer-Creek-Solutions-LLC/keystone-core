// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.keystone-core.io/keystone-core/internal/state"
)

// sqlAuditStore wraps an [state.AuditStore] (SQLite or Postgres)
// into the typed [AuditStore] consumer interface.
type sqlAuditStore struct {
	backing state.AuditStore
}

// NewSQLAuditStore wraps the given [state.AuditStore]. The wrapper
// is safe for concurrent use to the extent that backing is —
// `database/sql` is, by design.
//
// `Close` on the returned store is a no-op: the underlying SQL
// connection pool is owned by the process-wide state composite.
func NewSQLAuditStore(backing state.AuditStore) AuditStore {
	return &sqlAuditStore{backing: backing}
}

func (s *sqlAuditStore) Store(ctx context.Context, e AuditEntry) error {
	if err := e.Validate(); err != nil {
		return err
	}
	rec, err := recordFromAuditEntry(e)
	if err != nil {
		return err
	}
	return s.backing.CreateAuditEntry(ctx, rec)
}

func (s *sqlAuditStore) StoreBatch(ctx context.Context, entries []AuditEntry) error {
	if len(entries) == 0 {
		return nil
	}
	for i, e := range entries {
		if err := e.Validate(); err != nil {
			return fmt.Errorf("audit: StoreBatch [%d]: %w", i, err)
		}
	}
	recs := make([]*state.AuditEntryStoreRecord, len(entries))
	for i, e := range entries {
		r, err := recordFromAuditEntry(e)
		if err != nil {
			return fmt.Errorf("audit: StoreBatch [%d]: %w", i, err)
		}
		recs[i] = r
	}
	return s.backing.CreateAuditEntriesBatch(ctx, recs)
}

func (s *sqlAuditStore) Get(ctx context.Context, id string) (AuditEntry, error) {
	rec, err := s.backing.GetAuditEntry(ctx, id)
	if err != nil {
		return AuditEntry{}, err
	}
	return auditEntryFromRecord(rec)
}

func (s *sqlAuditStore) Query(ctx context.Context, q AuditQuery) (AuditPage, error) {
	if err := q.Validate(); err != nil {
		return AuditPage{}, err
	}
	limit := q.Limit
	if limit == 0 {
		limit = DefaultQueryLimit
	}
	filter := filterFromQuery(q, limit)
	recs, err := s.backing.ListAuditEntries(ctx, filter)
	if err != nil {
		return AuditPage{}, err
	}
	page := AuditPage{Entries: make([]AuditEntry, 0, len(recs))}
	for _, rec := range recs {
		e, err := auditEntryFromRecord(rec)
		if err != nil {
			return AuditPage{}, fmt.Errorf("audit: Query: %w", err)
		}
		page.Entries = append(page.Entries, e)
	}
	// NextCursor: only when page is full.
	if len(recs) == limit && len(recs) > 0 {
		page.NextCursor = recs[len(recs)-1].ID
	}
	return page, nil
}

func (s *sqlAuditStore) Count(ctx context.Context, q AuditQuery) (int, error) {
	if err := q.Validate(); err != nil {
		return 0, err
	}
	filter := filterFromQuery(q, 0)
	return s.backing.CountAuditEntries(ctx, filter)
}

func (s *sqlAuditStore) Delete(ctx context.Context, id string) error {
	return s.backing.DeleteAuditEntry(ctx, id)
}

func (s *sqlAuditStore) ApplyRetention(ctx context.Context, policy RetentionPolicy) (int, error) {
	stPolicy := state.AuditRetentionPolicy{
		MaxAge:   policy.MaxAge,
		MaxCount: policy.MaxCount,
	}
	if policy.MinSeverity.IsValid() {
		stPolicy.MinSeverity = policy.MinSeverity.String()
	}
	return s.backing.ApplyAuditRetention(ctx, stPolicy)
}

func (s *sqlAuditStore) Summarize(ctx context.Context, q AuditQuery) (AuditSummary, error) {
	if err := q.Validate(); err != nil {
		return AuditSummary{}, err
	}
	filter := filterFromQuery(q, 0)
	rec, err := s.backing.SummarizeAuditEntries(ctx, filter)
	if err != nil {
		return AuditSummary{}, err
	}
	out := AuditSummary{
		TotalEvaluations:   rec.TotalEvaluations,
		AllowedCount:       rec.AllowedCount,
		DeniedCount:        rec.DeniedCount,
		ViolationsByPolicy: rec.ViolationsByPolicy,
		Range:              TimeRange{Start: rec.RangeStart, End: rec.RangeEnd},
	}
	if len(rec.ViolationsBySeverity) > 0 {
		bySev := make(map[Severity]int, len(rec.ViolationsBySeverity))
		for name, n := range rec.ViolationsBySeverity {
			sev, err := ParseSeverity(name)
			if err != nil {
				return AuditSummary{}, fmt.Errorf("audit: Summarize: unknown severity %q in result: %w", name, err)
			}
			bySev[sev] = n
		}
		out.ViolationsBySeverity = bySev
	}
	return out, nil
}

func (s *sqlAuditStore) Close() error {
	// The SQL pool is owned by the process-wide state composite.
	return nil
}

// recordFromAuditEntry translates the typed [AuditEntry] into the
// DB-shape [state.AuditEntryStoreRecord]. Violations + Metadata
// marshaled to JSON; enums stringified to canonical lowercase.
func recordFromAuditEntry(e AuditEntry) (*state.AuditEntryStoreRecord, error) {
	var violationsJSON []byte
	if len(e.Violations) > 0 {
		b, err := json.Marshal(e.Violations)
		if err != nil {
			return nil, fmt.Errorf("audit: marshal violations: %w", err)
		}
		violationsJSON = b
	}
	return &state.AuditEntryStoreRecord{
		ID:              e.ID,
		Timestamp:       e.Timestamp,
		PolicyID:        e.PolicyID,
		PolicyName:      e.PolicyName,
		PolicyType:      string(e.PolicyType),
		ResourceType:    e.ResourceType,
		Allowed:         e.Allowed,
		DurationNS:      e.Duration.Nanoseconds(),
		Violations:      violationsJSON,
		EnforcementMode: e.EnforcementMode.String(),
		Severity:        e.Severity.String(),
		User:            e.User,
		Action:          e.Action,
		Metadata:        e.Metadata,
	}, nil
}

// auditEntryFromRecord translates DB-shape into typed [AuditEntry].
// Parse failures fall back to zero-value enums + return an error
// only when the JSON Violations payload is malformed (schema-level
// NOT NULL on the column makes that branch unreachable in
// practice).
func auditEntryFromRecord(rec *state.AuditEntryStoreRecord) (AuditEntry, error) {
	sev, _ := ParseSeverity(rec.Severity)
	mode, _ := ParseEnforcementMode(rec.EnforcementMode)
	pt, _ := ParsePolicyType(rec.PolicyType)
	var violations []Violation
	if len(rec.Violations) > 0 && string(rec.Violations) != "[]" {
		if err := json.Unmarshal(rec.Violations, &violations); err != nil {
			return AuditEntry{}, fmt.Errorf("audit: unmarshal violations for entry %s: %w", rec.ID, err)
		}
	}
	return AuditEntry{
		ID:              rec.ID,
		Timestamp:       rec.Timestamp,
		PolicyID:        rec.PolicyID,
		PolicyName:      rec.PolicyName,
		PolicyType:      pt,
		ResourceType:    rec.ResourceType,
		Allowed:         rec.Allowed,
		Duration:        time.Duration(rec.DurationNS),
		Violations:      violations,
		EnforcementMode: mode,
		Severity:        sev,
		User:            rec.User,
		Action:          rec.Action,
		Metadata:        rec.Metadata,
	}, nil
}

// filterFromQuery converts the typed [AuditQuery] into the DB-shape
// [state.AuditEntryFilter].
func filterFromQuery(q AuditQuery, limit int) state.AuditEntryFilter {
	f := state.AuditEntryFilter{
		PolicyID:     q.PolicyID,
		User:         q.User,
		ResourceType: q.ResourceType,
		Action:       q.Action,
		Severities:   severitiesAtLeast(q.MinSeverity),
		Allowed:      q.Allowed,
		Since:        q.Since,
		Until:        q.Until,
		Cursor:       q.Cursor,
		Limit:        limit,
		Descending:   q.Descending,
	}
	return f
}
