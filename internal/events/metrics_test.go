package events

import (
	"io"
	"log/slog"
	"testing"

	dto "github.com/prometheus/client_model/go"

	"go.keystone-core.io/keystone-core/internal/metrics"
)

func testRegistry(t *testing.T) *metrics.Registry {
	t.Helper()
	return metrics.NewRegistry(metrics.Options{
		Logger:                   slog.New(slog.NewTextHandler(io.Discard, nil)),
		DisableRuntimeCollectors: true,
	})
}

func sample(t *testing.T, r *metrics.Registry, name string, want map[string]string) *dto.Metric {
	t.Helper()
	mfs, err := r.Gatherer().Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != name {
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

func TestNewMetrics_NilRegistry_ReturnsNil(t *testing.T) {
	m, err := NewMetrics(nil)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if m != nil {
		t.Fatalf("m = %v, want nil", m)
	}
	// Nil-safe Record.
	m.RecordEmit(EventTypeAgentConnect, SeverityInfo)
}

func TestNewMetrics_RegistersEventsEmittedTotal(t *testing.T) {
	r := testRegistry(t)
	m, err := NewMetrics(r)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if _, ok := r.Definitions()[metrics.DefEventsEmittedTotal.Name]; !ok {
		t.Fatalf("%s missing from Definitions", metrics.DefEventsEmittedTotal.Name)
	}
	m.RecordEmit(EventTypeAgentConnect, SeverityInfo)
	m.RecordEmit(EventTypeAgentConnect, SeverityInfo)
	m.RecordEmit(EventTypePolicyViolation, SeverityWarn)

	if s := sample(t, r, metrics.DefEventsEmittedTotal.Name, map[string]string{
		"type": string(EventTypeAgentConnect), "severity": SeverityInfo.String(),
	}); s == nil || s.GetCounter().GetValue() != 2 {
		t.Errorf("agent_registered.info count = %v, want 2", s)
	}
	if s := sample(t, r, metrics.DefEventsEmittedTotal.Name, map[string]string{
		"type": string(EventTypePolicyViolation), "severity": SeverityWarn.String(),
	}); s == nil || s.GetCounter().GetValue() != 1 {
		t.Errorf("policy_denied.warning count = %v, want 1", s)
	}
}

func TestNewMetrics_DuplicateRegistrationFails(t *testing.T) {
	r := testRegistry(t)
	if _, err := NewMetrics(r); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := NewMetrics(r); err == nil {
		t.Fatalf("second: want duplicate-name error")
	}
}
