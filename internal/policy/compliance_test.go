// SPDX-License-Identifier: Apache-2.0

package policy_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/audit"
	"go.keystone-core.io/keystone-core/internal/policy"
)

// fakeAuditStore is a deterministic in-memory audit.AuditStore for
// compliance tests — Summarize/Query return canned results keyed by
// the query's PolicyID/ResourceType/window so the report logic is
// tested without the SQLite/state dependency.
type fakeAuditStore struct {
	// summary keyed by PolicyID ("" = unscoped/whole-window).
	summaries map[string]audit.AuditSummary
	// trend keyed by bucket Start (RFC3339) → summary.
	buckets map[string]audit.AuditSummary
	// query returns these pages per ResourceType (one page, no cursor).
	entries      map[string][]audit.AuditEntry
	summarizeErr error
	queryErr     error
	summarizeN   int
}

func (f *fakeAuditStore) Store(context.Context, audit.AuditEntry) error        { return nil }
func (f *fakeAuditStore) StoreBatch(context.Context, []audit.AuditEntry) error { return nil }
func (f *fakeAuditStore) Get(context.Context, string) (audit.AuditEntry, error) {
	return audit.AuditEntry{}, nil
}
func (f *fakeAuditStore) Count(context.Context, audit.AuditQuery) (int, error) { return 0, nil }
func (f *fakeAuditStore) Delete(context.Context, string) error                 { return nil }
func (f *fakeAuditStore) ApplyRetention(context.Context, audit.RetentionPolicy) (int, error) {
	return 0, nil
}
func (f *fakeAuditStore) Close() error { return nil }

func (f *fakeAuditStore) Summarize(_ context.Context, q audit.AuditQuery) (audit.AuditSummary, error) {
	f.summarizeN++
	if f.summarizeErr != nil {
		return audit.AuditSummary{}, f.summarizeErr
	}
	// Trend bucket lookup takes precedence when the bucket map has
	// the window's Since.
	if f.buckets != nil {
		if s, ok := f.buckets[q.Since.Format(time.RFC3339)]; ok && q.PolicyID == "" {
			return s, nil
		}
	}
	if s, ok := f.summaries[q.PolicyID]; ok {
		return s, nil
	}
	return audit.AuditSummary{}, nil
}

func (f *fakeAuditStore) Query(_ context.Context, q audit.AuditQuery) (audit.AuditPage, error) {
	if f.queryErr != nil {
		return audit.AuditPage{}, f.queryErr
	}
	return audit.AuditPage{Entries: f.entries[q.ResourceType]}, nil
}

func since() time.Time { return time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC) }
func until() time.Time { return time.Date(2026, 5, 16, 0, 0, 0, 0, time.UTC) }

func TestNewReportGenerator_RequiresStore(t *testing.T) {
	t.Parallel()
	if _, err := policy.NewReportGenerator(nil, nil); !errors.Is(err, policy.ErrEngineMisconfigured) {
		t.Errorf("err = %v, want ErrEngineMisconfigured", err)
	}
}

func TestGenerate_HeadlineCountsAndRate(t *testing.T) {
	t.Parallel()
	store := &fakeAuditStore{summaries: map[string]audit.AuditSummary{
		"": {
			TotalEvaluations: 100, AllowedCount: 80, DeniedCount: 20,
			ViolationsByPolicy:   map[string]int{"p-a": 12, "p-b": 8},
			ViolationsBySeverity: map[audit.Severity]int{audit.SeverityHigh: 15, audit.SeverityMedium: 5},
		},
	}}
	g, _ := policy.NewReportGenerator(store, nil)
	r, err := g.Generate(context.Background(), policy.ReportQuery{Since: since(), Until: until()})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if r.TotalEvaluations != 100 || r.CompliantEvaluations != 80 || r.NonCompliantEvaluations != 20 {
		t.Errorf("counts: %+v", r)
	}
	if r.ComplianceRate != 0.8 {
		t.Errorf("rate = %v, want 0.8", r.ComplianceRate)
	}
	if !r.Period.Start.Equal(since()) || !r.Period.End.Equal(until()) {
		t.Errorf("period = %+v", r.Period)
	}
	if r.ViolationsBySeverity[audit.SeverityHigh] != 15 {
		t.Errorf("ViolationsBySeverity = %+v", r.ViolationsBySeverity)
	}
	// TopViolations: p-a (12) before p-b (8).
	if len(r.TopViolations) != 2 || r.TopViolations[0].PolicyID != "p-a" || r.TopViolations[0].Count != 12 {
		t.Errorf("TopViolations = %+v", r.TopViolations)
	}
}

func TestGenerate_ZeroTotalRateIsZero(t *testing.T) {
	t.Parallel()
	store := &fakeAuditStore{summaries: map[string]audit.AuditSummary{"": {}}}
	g, _ := policy.NewReportGenerator(store, nil)
	r, err := g.Generate(context.Background(), policy.ReportQuery{Since: since()})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if r.ComplianceRate != 0 || r.TotalEvaluations != 0 {
		t.Errorf("zero-data: rate=%v total=%d, want 0/0", r.ComplianceRate, r.TotalEvaluations)
	}
}

func TestGenerate_TopNCap(t *testing.T) {
	t.Parallel()
	store := &fakeAuditStore{summaries: map[string]audit.AuditSummary{
		"": {
			TotalEvaluations: 10,
			ViolationsByPolicy: map[string]int{
				"a": 5, "b": 4, "c": 3, "d": 2, "e": 1,
			},
		},
	}}
	g, _ := policy.NewReportGenerator(store, nil)
	r, _ := g.Generate(context.Background(), policy.ReportQuery{Since: since(), TopN: 3})
	if len(r.TopViolations) != 3 {
		t.Fatalf("TopViolations = %d, want 3 (TopN cap)", len(r.TopViolations))
	}
	if r.TopViolations[0].PolicyID != "a" || r.TopViolations[2].PolicyID != "c" {
		t.Errorf("top order wrong: %+v", r.TopViolations)
	}
}

func TestGenerate_PolicyStatsUnscopedFromViolations(t *testing.T) {
	t.Parallel()
	store := &fakeAuditStore{summaries: map[string]audit.AuditSummary{
		"":    {TotalEvaluations: 30, AllowedCount: 20, DeniedCount: 10, ViolationsByPolicy: map[string]int{"p-a": 6, "p-b": 4}},
		"p-a": {TotalEvaluations: 18, AllowedCount: 12, DeniedCount: 6},
		"p-b": {TotalEvaluations: 12, AllowedCount: 8, DeniedCount: 4},
	}}
	g, _ := policy.NewReportGenerator(store, nil)
	r, err := g.Generate(context.Background(), policy.ReportQuery{Since: since()})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(r.PolicyStats) != 2 {
		t.Fatalf("PolicyStats = %d, want 2 (p-a, p-b sorted)", len(r.PolicyStats))
	}
	if r.PolicyStats[0].PolicyID != "p-a" || r.PolicyStats[0].Evaluations != 18 ||
		r.PolicyStats[0].Passed != 12 || r.PolicyStats[0].Failed != 6 {
		t.Errorf("PolicyStats[0] = %+v", r.PolicyStats[0])
	}
	if r.PolicyStats[0].Rate != 12.0/18.0 {
		t.Errorf("PolicyStats[0].Rate = %v", r.PolicyStats[0].Rate)
	}
}

func TestGenerate_PolicyStatsScopedByFramework(t *testing.T) {
	t.Parallel()
	store := &fakeAuditStore{summaries: map[string]audit.AuditSummary{
		"":           {TotalEvaluations: 50, AllowedCount: 40, DeniedCount: 10, ViolationsByPolicy: map[string]int{"req-labels": 10}},
		"req-labels": {TotalEvaluations: 25, AllowedCount: 20, DeniedCount: 5},
		"deny-priv":  {TotalEvaluations: 25, AllowedCount: 20, DeniedCount: 5},
	}}
	cm := policy.NewControlMapping()
	_ = cm.RegisterControl(&policy.ComplianceControl{
		ID: "SOC2-1", Framework: policy.FrameworkSOC2, Title: "t",
		Severity: audit.SeverityHigh, PolicyIDs: []string{"req-labels", "deny-priv"},
	})
	g, _ := policy.NewReportGenerator(store, cm)
	r, err := g.Generate(context.Background(), policy.ReportQuery{
		Since: since(), Framework: policy.FrameworkSOC2,
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	// Framework scope → both framework policies, sorted, NOT only the
	// one that violated.
	if len(r.PolicyStats) != 2 {
		t.Fatalf("PolicyStats = %d, want 2 (framework-scoped)", len(r.PolicyStats))
	}
	if r.PolicyStats[0].PolicyID != "deny-priv" || r.PolicyStats[1].PolicyID != "req-labels" {
		t.Errorf("framework scope order: %+v", r.PolicyStats)
	}
}

func TestGenerate_FrameworkScopeNilMappingNoStats(t *testing.T) {
	t.Parallel()
	store := &fakeAuditStore{summaries: map[string]audit.AuditSummary{"": {TotalEvaluations: 5}}}
	g, _ := policy.NewReportGenerator(store, nil) // nil mapping
	r, err := g.Generate(context.Background(), policy.ReportQuery{
		Since: since(), Framework: policy.FrameworkCIS,
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if r.PolicyStats != nil {
		t.Errorf("framework scope + nil mapping should yield no PolicyStats, got %+v", r.PolicyStats)
	}
}

func TestGenerate_Trend(t *testing.T) {
	t.Parallel()
	s := since()
	store := &fakeAuditStore{
		summaries: map[string]audit.AuditSummary{"": {TotalEvaluations: 0}},
		buckets: map[string]audit.AuditSummary{
			s.Format(time.RFC3339):                     {TotalEvaluations: 10, AllowedCount: 9},
			s.Add(24 * time.Hour).Format(time.RFC3339): {TotalEvaluations: 10, AllowedCount: 5},
			s.Add(48 * time.Hour).Format(time.RFC3339): {TotalEvaluations: 10, AllowedCount: 10},
		},
	}
	g, _ := policy.NewReportGenerator(store, nil)
	r, err := g.Generate(context.Background(), policy.ReportQuery{
		Since: s, Until: s.Add(72 * time.Hour), BucketInterval: 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(r.Trend) != 3 {
		t.Fatalf("Trend = %d, want 3 buckets", len(r.Trend))
	}
	if r.Trend[0].ComplianceRate != 0.9 || r.Trend[1].ComplianceRate != 0.5 || r.Trend[2].ComplianceRate != 1.0 {
		t.Errorf("trend rates: %+v", r.Trend)
	}
	if !r.Trend[0].Start.Equal(s) || !r.Trend[0].End.Equal(s.Add(24*time.Hour)) {
		t.Errorf("bucket 0 window = %v..%v", r.Trend[0].Start, r.Trend[0].End)
	}
}

func TestGenerate_NoTrendWhenIntervalZero(t *testing.T) {
	t.Parallel()
	store := &fakeAuditStore{summaries: map[string]audit.AuditSummary{"": {TotalEvaluations: 1}}}
	g, _ := policy.NewReportGenerator(store, nil)
	r, _ := g.Generate(context.Background(), policy.ReportQuery{Since: since()})
	if r.Trend != nil {
		t.Errorf("Trend = %+v, want nil (no BucketInterval)", r.Trend)
	}
}

func TestGenerate_TrendBucketCap(t *testing.T) {
	t.Parallel()
	store := &fakeAuditStore{summaries: map[string]audit.AuditSummary{"": {}}}
	g, _ := policy.NewReportGenerator(store, nil)
	// 1000 days at 1-day buckets → capped at maxTrendBuckets (366).
	s := since()
	r, err := g.Generate(context.Background(), policy.ReportQuery{
		Since: s, Until: s.Add(1000 * 24 * time.Hour), BucketInterval: 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(r.Trend) != 366 {
		t.Errorf("Trend = %d, want 366 (cap)", len(r.Trend))
	}
}

func TestGenerate_InvalidQuery(t *testing.T) {
	t.Parallel()
	store := &fakeAuditStore{summaries: map[string]audit.AuditSummary{"": {}}}
	g, _ := policy.NewReportGenerator(store, nil)
	if _, err := g.Generate(context.Background(), policy.ReportQuery{}); !errors.Is(err, policy.ErrInvalidPolicy) {
		t.Errorf("missing Since: err = %v", err)
	}
	if _, err := g.Generate(context.Background(), policy.ReportQuery{
		Since: until(), Until: since(),
	}); !errors.Is(err, policy.ErrInvalidPolicy) {
		t.Errorf("Since after Until: err = %v", err)
	}
}

func TestGenerate_StoreErrorPropagates(t *testing.T) {
	t.Parallel()
	boom := errors.New("db down")
	store := &fakeAuditStore{summarizeErr: boom}
	g, _ := policy.NewReportGenerator(store, nil)
	if _, err := g.Generate(context.Background(), policy.ReportQuery{Since: since()}); !errors.Is(err, boom) {
		t.Errorf("err = %v, want wrapped %v", err, boom)
	}
}

func TestResourceAuditTrail(t *testing.T) {
	t.Parallel()
	s := since()
	store := &fakeAuditStore{
		summaries: map[string]audit.AuditSummary{"": {}},
		entries: map[string][]audit.AuditEntry{
			"secret": {
				{ID: "e1", ResourceType: "secret", Timestamp: s},
				{ID: "e2", ResourceType: "secret", Timestamp: s.Add(time.Hour)},
			},
		},
	}
	g, _ := policy.NewReportGenerator(store, nil)
	trail, err := g.ResourceAuditTrail(context.Background(), "secret",
		policy.ReportQuery{Since: s, Until: s.Add(24 * time.Hour)})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(trail) != 2 || trail[0].ID != "e1" || trail[1].ID != "e2" {
		t.Errorf("trail = %+v", trail)
	}
	// Unknown resource type → empty trail, no error.
	empty, err := g.ResourceAuditTrail(context.Background(), "nope",
		policy.ReportQuery{Since: s})
	if err != nil || len(empty) != 0 {
		t.Errorf("empty trail: %v / %+v", err, empty)
	}
}

func TestResourceAuditTrail_RequiresSince(t *testing.T) {
	t.Parallel()
	store := &fakeAuditStore{}
	g, _ := policy.NewReportGenerator(store, nil)
	if _, err := g.ResourceAuditTrail(context.Background(), "secret", policy.ReportQuery{}); !errors.Is(err, policy.ErrInvalidPolicy) {
		t.Errorf("missing Since: err = %v", err)
	}
}
