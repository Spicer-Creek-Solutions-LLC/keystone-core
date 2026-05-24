// SPDX-License-Identifier: Apache-2.0

package audit_test

import (
	"context"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/audit"
)

func TestSQLAuditStore_Summarize_Empty(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	got, err := as.Summarize(context.Background(), audit.AuditQuery{})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if got.TotalEvaluations != 0 || got.AllowedCount != 0 || got.DeniedCount != 0 {
		t.Errorf("non-zero on empty: %+v", got)
	}
	if !got.Range.Start.IsZero() || !got.Range.End.IsZero() {
		t.Errorf("non-zero range on empty: %+v", got.Range)
	}
	if got.ViolationsByPolicy != nil || got.ViolationsBySeverity != nil {
		t.Errorf("non-nil maps on empty: %+v / %+v",
			got.ViolationsByPolicy, got.ViolationsBySeverity)
	}
}

func TestSQLAuditStore_Summarize_CountsByOutcome(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_ = as.Store(ctx, audit.MustNewAuditEntry(audit.AuditEntryInput{
			Action:  "x",
			Allowed: true,
		}))
	}
	for i := 0; i < 5; i++ {
		_ = as.Store(ctx, audit.MustNewAuditEntry(audit.AuditEntryInput{
			Action:  "x",
			Allowed: false,
		}))
	}
	got, err := as.Summarize(ctx, audit.AuditQuery{})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if got.TotalEvaluations != 8 {
		t.Errorf("Total = %d, want 8", got.TotalEvaluations)
	}
	if got.AllowedCount != 3 {
		t.Errorf("Allowed = %d, want 3", got.AllowedCount)
	}
	if got.DeniedCount != 5 {
		t.Errorf("Denied = %d, want 5", got.DeniedCount)
	}
}

func TestSQLAuditStore_Summarize_ByPolicyOnlyCountsDenied(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	ctx := context.Background()
	// 4 allowed for p-a (should NOT show up in by-policy)
	for i := 0; i < 4; i++ {
		_ = as.Store(ctx, audit.MustNewAuditEntry(audit.AuditEntryInput{
			PolicyID: "p-a", Action: "x", Allowed: true,
		}))
	}
	// 2 denied for p-a, 3 denied for p-b
	for i := 0; i < 2; i++ {
		_ = as.Store(ctx, audit.MustNewAuditEntry(audit.AuditEntryInput{
			PolicyID: "p-a", Action: "x", Allowed: false,
		}))
	}
	for i := 0; i < 3; i++ {
		_ = as.Store(ctx, audit.MustNewAuditEntry(audit.AuditEntryInput{
			PolicyID: "p-b", Action: "x", Allowed: false,
		}))
	}
	got, _ := as.Summarize(ctx, audit.AuditQuery{})
	if got.ViolationsByPolicy["p-a"] != 2 {
		t.Errorf("p-a = %d, want 2", got.ViolationsByPolicy["p-a"])
	}
	if got.ViolationsByPolicy["p-b"] != 3 {
		t.Errorf("p-b = %d, want 3", got.ViolationsByPolicy["p-b"])
	}
}

func TestSQLAuditStore_Summarize_BySeverityTypedKeys(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	ctx := context.Background()
	for _, sev := range []audit.Severity{audit.SeverityMedium, audit.SeverityMedium, audit.SeverityHigh, audit.SeverityCritical} {
		_ = as.Store(ctx, audit.MustNewAuditEntry(audit.AuditEntryInput{
			Action: "x", Severity: sev, Allowed: false,
		}))
	}
	// One allowed at high — should NOT appear in by-severity
	_ = as.Store(ctx, audit.MustNewAuditEntry(audit.AuditEntryInput{
		Action: "x", Severity: audit.SeverityHigh, Allowed: true,
	}))

	got, _ := as.Summarize(ctx, audit.AuditQuery{})
	if got.ViolationsBySeverity[audit.SeverityMedium] != 2 {
		t.Errorf("medium = %d, want 2", got.ViolationsBySeverity[audit.SeverityMedium])
	}
	if got.ViolationsBySeverity[audit.SeverityHigh] != 1 {
		t.Errorf("high = %d (allowed entry leaked?)", got.ViolationsBySeverity[audit.SeverityHigh])
	}
	if got.ViolationsBySeverity[audit.SeverityCritical] != 1 {
		t.Errorf("critical = %d, want 1", got.ViolationsBySeverity[audit.SeverityCritical])
	}
}

func TestSQLAuditStore_Summarize_TimeRange(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_ = as.Store(ctx, audit.MustNewAuditEntry(audit.AuditEntryInput{Action: "x"}))
		time.Sleep(3 * time.Millisecond)
	}
	got, _ := as.Summarize(ctx, audit.AuditQuery{})
	if got.Range.Start.IsZero() || got.Range.End.IsZero() {
		t.Fatalf("zero range with entries: %+v", got.Range)
	}
	if !got.Range.Start.Before(got.Range.End) {
		t.Errorf("Start (%v) not before End (%v)", got.Range.Start, got.Range.End)
	}
}

func TestSQLAuditStore_Summarize_FilterNarrows(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	ctx := context.Background()
	_ = as.Store(ctx, audit.MustNewAuditEntry(audit.AuditEntryInput{
		User: "alice", Action: "x", Allowed: false,
	}))
	_ = as.Store(ctx, audit.MustNewAuditEntry(audit.AuditEntryInput{
		User: "alice", Action: "x", Allowed: false,
	}))
	_ = as.Store(ctx, audit.MustNewAuditEntry(audit.AuditEntryInput{
		User: "bob", Action: "x", Allowed: true,
	}))

	got, _ := as.Summarize(ctx, audit.AuditQuery{User: "alice"})
	if got.TotalEvaluations != 2 {
		t.Errorf("user=alice total = %d, want 2", got.TotalEvaluations)
	}
	if got.DeniedCount != 2 {
		t.Errorf("user=alice denied = %d, want 2", got.DeniedCount)
	}
}

func TestSQLAuditStore_Summarize_MinSeverityNarrows(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	ctx := context.Background()
	for _, sev := range []audit.Severity{audit.SeverityLow, audit.SeverityMedium, audit.SeverityHigh, audit.SeverityCritical} {
		_ = as.Store(ctx, audit.MustNewAuditEntry(audit.AuditEntryInput{
			Action: "x", Severity: sev, Allowed: false,
		}))
	}
	// MinSeverity=high restricts the filter set to {high, critical} = 2 rows.
	got, _ := as.Summarize(ctx, audit.AuditQuery{MinSeverity: audit.SeverityHigh})
	if got.TotalEvaluations != 2 {
		t.Errorf("MinSeverity=high total = %d, want 2", got.TotalEvaluations)
	}
	if got.DeniedCount != 2 {
		t.Errorf("MinSeverity=high denied = %d, want 2", got.DeniedCount)
	}
}

func TestSQLAuditStore_Summarize_RejectsInvalidQuery(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	_, err := as.Summarize(context.Background(), audit.AuditQuery{Limit: -1})
	if err == nil {
		t.Errorf("invalid query validated")
	}
}

func TestSQLAuditStore_Summarize_IgnoresCursorLimit(t *testing.T) {
	t.Parallel()
	as, _ := newTestStore(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_ = as.Store(ctx, audit.MustNewAuditEntry(audit.AuditEntryInput{Action: "x", Allowed: false}))
	}
	// Cursor + Limit must NOT shrink the aggregation; that's why
	// they're zeroed in filterFromQuery for Summarize.
	got, _ := as.Summarize(ctx, audit.AuditQuery{Cursor: "some-cursor", Limit: 2})
	if got.TotalEvaluations != 5 {
		t.Errorf("Total = %d, want 5 (cursor/limit must be ignored)", got.TotalEvaluations)
	}
}
