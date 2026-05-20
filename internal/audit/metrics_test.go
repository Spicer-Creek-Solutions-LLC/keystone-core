package audit

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"

	dto "github.com/prometheus/client_model/go"

	"go.keystone-core.io/keystone-core/internal/metrics"
)

func auditTestRegistry(t *testing.T) *metrics.Registry {
	t.Helper()
	return metrics.NewRegistry(metrics.Options{
		Logger:                   slog.New(slog.NewTextHandler(io.Discard, nil)),
		DisableRuntimeCollectors: true,
	})
}

func aSample(t *testing.T, r *metrics.Registry, want map[string]string) *dto.Metric {
	t.Helper()
	mfs, _ := r.Gatherer().Gather()
	for _, mf := range mfs {
		if mf.GetName() != metrics.DefAuditEntriesTotal.Name {
			continue
		}
	outer:
		for _, m := range mf.GetMetric() {
			labels := map[string]string{}
			for _, lp := range m.GetLabel() {
				labels[lp.GetName()] = lp.GetValue()
			}
			if len(labels) != len(want) {
				continue
			}
			for k, v := range want {
				if labels[k] != v {
					continue outer
				}
			}
			return m
		}
	}
	return nil
}

type countingAuditor struct{ count atomic.Int64 }

func (c *countingAuditor) Emit(context.Context, AuditEntry) { c.count.Add(1) }

func TestAuditMetrics_NilSafe(t *testing.T) {
	m, err := NewMetrics(nil)
	if err != nil || m != nil {
		t.Fatalf("NewMetrics(nil) = %v, %v", m, err)
	}
	m.RecordEntry("p", true)
}

func TestAuditMetrics_RecordEntry(t *testing.T) {
	r := auditTestRegistry(t)
	m, _ := NewMetrics(r)
	m.RecordEntry("policy-a", true)
	m.RecordEntry("policy-a", true)
	m.RecordEntry("policy-a", false)
	m.RecordEntry("", true) // unspecified policy

	if s := aSample(t, r, map[string]string{"policy": "policy-a", "allowed": "true"}); s == nil || s.GetCounter().GetValue() != 2 {
		t.Errorf("policy-a true = %v, want 2", s)
	}
	if s := aSample(t, r, map[string]string{"policy": "policy-a", "allowed": "false"}); s == nil || s.GetCounter().GetValue() != 1 {
		t.Errorf("policy-a false = %v, want 1", s)
	}
	if s := aSample(t, r, map[string]string{"policy": "_unspecified", "allowed": "true"}); s == nil || s.GetCounter().GetValue() != 1 {
		t.Errorf("unspecified = %v, want 1", s)
	}
}

func TestMeasuringAuditor_ForwardsAndCounts(t *testing.T) {
	r := auditTestRegistry(t)
	m, _ := NewMetrics(r)
	inner := &countingAuditor{}
	a := NewMeasuringAuditor(inner, m)
	a.Emit(context.Background(), AuditEntry{PolicyName: "p", Allowed: true})
	a.Emit(context.Background(), AuditEntry{PolicyName: "p", Allowed: false})

	if got := inner.count.Load(); got != 2 {
		t.Errorf("inner.Emit count = %d, want 2", got)
	}
	if s := aSample(t, r, map[string]string{"policy": "p", "allowed": "true"}); s == nil || s.GetCounter().GetValue() != 1 {
		t.Errorf("p.true = %v, want 1", s)
	}
}

func TestMeasuringAuditor_NilInner_FallsBackToNoop(t *testing.T) {
	a := NewMeasuringAuditor(nil, nil)
	a.Emit(context.Background(), AuditEntry{PolicyName: "p", Allowed: true}) // must not panic
}
