// SPDX-License-Identifier: Apache-2.0

package policy

import (
	"context"
	"fmt"
	"sort"
	"time"

	"go.keystone-core.io/keystone-core/internal/audit"
)

// maxTrendBuckets bounds the number of per-bucket Summarize calls a
// single report can issue, so a tiny BucketInterval over a long
// Period can't fan out into thousands of queries.
const maxTrendBuckets = 366

// PolicyStat is the per-policy roll-up in [ComplianceReport]. Rate
// is Passed/Evaluations (0 when Evaluations==0).
type PolicyStat struct {
	PolicyID    string
	Evaluations int
	Passed      int
	Failed      int
	Rate        float64
}

// ViolationCount is one entry of [ComplianceReport.TopViolations]:
// a policy ID and how many denied evaluations it produced in the
// period, sorted desc by Count (ties broken by PolicyID asc).
type ViolationCount struct {
	PolicyID string
	Count    int
}

// TrendPoint is one [ComplianceReport.Trend] bucket.
type TrendPoint struct {
	Start                   time.Time
	End                     time.Time
	TotalEvaluations        int
	CompliantEvaluations    int
	NonCompliantEvaluations int
	ComplianceRate          float64
}

// ComplianceReport is the §4.12 compliance roll-up over an audit
// window. ComplianceRate = CompliantEvaluations/TotalEvaluations;
// TotalEvaluations==0 → ComplianceRate 0.0 (the zero Total signals
// "no data", not "100% compliant").
type ComplianceReport struct {
	Period                  audit.TimeRange
	ComplianceRate          float64
	TotalEvaluations        int
	CompliantEvaluations    int
	NonCompliantEvaluations int
	PolicyStats             []PolicyStat
	TopViolations           []ViolationCount
	ViolationsBySeverity    map[audit.Severity]int
	Trend                   []TrendPoint
}

// ReportQuery parameterizes [ReportGenerator.Generate]. Since/Until
// bound the window (Until defaults to now when zero; Since required).
// Framework, when set, scopes PolicyStats to exactly that
// framework's policies (via the ControlMapping). BucketInterval,
// when > 0, produces Trend buckets (capped at maxTrendBuckets);
// zero → no Trend. TopN caps TopViolations (<=0 → DefaultTopN).
type ReportQuery struct {
	Since          time.Time
	Until          time.Time
	Framework      Framework
	BucketInterval time.Duration
	TopN           int
}

// DefaultTopN is the TopViolations cap when ReportQuery.TopN <= 0.
const DefaultTopN = 10

// ReportGenerator builds ComplianceReports from an audit.AuditStore
// + a ControlMapping. The store is read-only here (Summarize /
// Query); the mapping bounds PolicyStats when a Framework filter is
// given. mapping may be nil — then a Framework-scoped query yields
// no PolicyStats (documented), and unscoped reports derive
// PolicyStats from the period's denying policies.
type ReportGenerator struct {
	store   audit.AuditStore
	mapping *ControlMapping
}

// NewReportGenerator wires the generator. store is required.
func NewReportGenerator(store audit.AuditStore, mapping *ControlMapping) (*ReportGenerator, error) {
	if store == nil {
		return nil, fmt.Errorf("%w: report generator needs an audit store", ErrEngineMisconfigured)
	}
	return &ReportGenerator{store: store, mapping: mapping}, nil
}

// Generate produces the report for q. One Summarize call powers the
// headline counts + ViolationsBySeverity + TopViolations; PolicyStats
// adds one scoped Summarize per in-scope policy; Trend adds one per
// bucket. Returns an error only on a store failure or an invalid
// query (Since zero).
func (g *ReportGenerator) Generate(ctx context.Context, q ReportQuery) (ComplianceReport, error) {
	if q.Since.IsZero() {
		return ComplianceReport{}, fmt.Errorf("%w: ReportQuery.Since is required", ErrInvalidPolicy)
	}
	until := q.Until
	if until.IsZero() {
		until = time.Now().UTC()
	}
	if !q.Since.Before(until) {
		return ComplianceReport{}, fmt.Errorf("%w: ReportQuery.Since must be before Until", ErrInvalidPolicy)
	}
	topN := q.TopN
	if topN <= 0 {
		topN = DefaultTopN
	}

	base := audit.AuditQuery{Since: q.Since, Until: until}
	sum, err := g.store.Summarize(ctx, base)
	if err != nil {
		return ComplianceReport{}, fmt.Errorf("compliance: summarize: %w", err)
	}

	report := ComplianceReport{
		Period:                  audit.TimeRange{Start: q.Since, End: until},
		TotalEvaluations:        sum.TotalEvaluations,
		CompliantEvaluations:    sum.AllowedCount,
		NonCompliantEvaluations: sum.DeniedCount,
		ComplianceRate:          rate(sum.AllowedCount, sum.TotalEvaluations),
		ViolationsBySeverity:    copySeverityMap(sum.ViolationsBySeverity),
		TopViolations:           topViolations(sum.ViolationsByPolicy, topN),
	}

	policyIDs := g.scopedPolicyIDs(q.Framework, sum.ViolationsByPolicy)
	stats, err := g.policyStats(ctx, base, policyIDs)
	if err != nil {
		return ComplianceReport{}, err
	}
	report.PolicyStats = stats

	if q.BucketInterval > 0 {
		trend, err := g.trend(ctx, q.Since, until, q.BucketInterval)
		if err != nil {
			return ComplianceReport{}, err
		}
		report.Trend = trend
	}
	return report, nil
}

// ResourceAuditTrail returns every audit entry for resourceType in
// the [q.Since, q.Until] window, oldest-first, walking the store
// with the IterateAll safety cap. Other ReportQuery fields are
// ignored.
func (g *ReportGenerator) ResourceAuditTrail(ctx context.Context, resourceType string, q ReportQuery) ([]audit.AuditEntry, error) {
	if q.Since.IsZero() {
		return nil, fmt.Errorf("%w: ReportQuery.Since is required", ErrInvalidPolicy)
	}
	until := q.Until
	if until.IsZero() {
		until = time.Now().UTC()
	}
	aq := audit.AuditQuery{
		ResourceType: resourceType,
		Since:        q.Since,
		Until:        until,
	}
	var trail []audit.AuditEntry
	err := audit.IterateAll(ctx, g.store, aq, func(e audit.AuditEntry) error {
		trail = append(trail, e)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("compliance: resource trail %q: %w", resourceType, err)
	}
	return trail, nil
}

// scopedPolicyIDs returns the policy set PolicyStats covers: a
// Framework filter → exactly that framework's policies (via the
// ControlMapping; empty when no mapping); otherwise the policies
// that produced violations in the period (deterministic order).
func (g *ReportGenerator) scopedPolicyIDs(fw Framework, byPolicy map[string]int) []string {
	if fw != "" {
		if g.mapping == nil {
			return nil
		}
		return g.mapping.PoliciesForFramework(fw)
	}
	ids := make([]string, 0, len(byPolicy))
	for pid := range byPolicy {
		ids = append(ids, pid)
	}
	sort.Strings(ids)
	return ids
}

// policyStats issues one scoped Summarize per policy ID for the
// real per-policy pass/fail/rate.
func (g *ReportGenerator) policyStats(ctx context.Context, base audit.AuditQuery, ids []string) ([]PolicyStat, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	out := make([]PolicyStat, 0, len(ids))
	for _, pid := range ids {
		pq := base
		pq.PolicyID = pid
		s, err := g.store.Summarize(ctx, pq)
		if err != nil {
			return nil, fmt.Errorf("compliance: summarize policy %q: %w", pid, err)
		}
		out = append(out, PolicyStat{
			PolicyID:    pid,
			Evaluations: s.TotalEvaluations,
			Passed:      s.AllowedCount,
			Failed:      s.DeniedCount,
			Rate:        rate(s.AllowedCount, s.TotalEvaluations),
		})
	}
	return out, nil
}

// trend buckets [since,until) by interval and Summarizes each. The
// final bucket is clamped to until; bucket count is capped at
// maxTrendBuckets.
func (g *ReportGenerator) trend(ctx context.Context, since, until time.Time, interval time.Duration) ([]TrendPoint, error) {
	var points []TrendPoint
	for start := since; start.Before(until); start = start.Add(interval) {
		end := start.Add(interval)
		if end.After(until) {
			end = until
		}
		if len(points) >= maxTrendBuckets {
			break
		}
		s, err := g.store.Summarize(ctx, audit.AuditQuery{Since: start, Until: end})
		if err != nil {
			return nil, fmt.Errorf("compliance: trend bucket %s: %w", start.Format(time.RFC3339), err)
		}
		points = append(points, TrendPoint{
			Start:                   start,
			End:                     end,
			TotalEvaluations:        s.TotalEvaluations,
			CompliantEvaluations:    s.AllowedCount,
			NonCompliantEvaluations: s.DeniedCount,
			ComplianceRate:          rate(s.AllowedCount, s.TotalEvaluations),
		})
	}
	return points, nil
}

// topViolations sorts byPolicy desc by count (PolicyID asc on ties)
// and returns the first n.
func topViolations(byPolicy map[string]int, n int) []ViolationCount {
	if len(byPolicy) == 0 {
		return nil
	}
	all := make([]ViolationCount, 0, len(byPolicy))
	for pid, c := range byPolicy {
		all = append(all, ViolationCount{PolicyID: pid, Count: c})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Count != all[j].Count {
			return all[i].Count > all[j].Count
		}
		return all[i].PolicyID < all[j].PolicyID
	})
	if len(all) > n {
		all = all[:n]
	}
	return all
}

func copySeverityMap(in map[audit.Severity]int) map[audit.Severity]int {
	if len(in) == 0 {
		return nil
	}
	out := make(map[audit.Severity]int, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// rate is allowed/total as a fraction; total==0 → 0.0 (the zero
// Total signals "no data", not perfect compliance).
func rate(allowed, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(allowed) / float64(total)
}
