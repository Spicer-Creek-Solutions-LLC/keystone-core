// SPDX-License-Identifier: Apache-2.0

package statemgmt

import (
	"io"
	"log/slog"
	"testing"

	dto "github.com/prometheus/client_model/go"

	"go.keystone-core.io/keystone-core/internal/metrics"
)

func mTestRegistry(t *testing.T) *metrics.Registry {
	t.Helper()
	return metrics.NewRegistry(metrics.Options{
		Logger:                   slog.New(slog.NewTextHandler(io.Discard, nil)),
		DisableRuntimeCollectors: true,
	})
}

func mSample(t *testing.T, r *metrics.Registry, name string, want map[string]string) *dto.Metric {
	t.Helper()
	mfs, _ := r.Gatherer().Gather()
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

func TestStatemgmtMetrics_NilSafe(t *testing.T) {
	m, err := NewMetrics(nil)
	if err != nil || m != nil {
		t.Fatalf("NewMetrics(nil) = %v, %v", m, err)
	}
	m.RecordApply(ApplyResultSuccess)
	m.RecordDrift("high")
}

func TestStatemgmtMetrics_RecordApply(t *testing.T) {
	r := mTestRegistry(t)
	m, err := NewMetrics(r)
	if err != nil {
		t.Fatal(err)
	}
	m.RecordApply(ApplyResultSuccess)
	m.RecordApply(ApplyResultSuccess)
	m.RecordApply(ApplyResultFailed)
	m.RecordApply(ApplyResultNoChange)

	if s := mSample(t, r, metrics.DefStateApplyTotal.Name, map[string]string{"result": "success"}); s == nil || s.GetCounter().GetValue() != 2 {
		t.Errorf("success = %v, want 2", s)
	}
	if s := mSample(t, r, metrics.DefStateApplyTotal.Name, map[string]string{"result": "failed"}); s == nil || s.GetCounter().GetValue() != 1 {
		t.Errorf("failed = %v, want 1", s)
	}
	if s := mSample(t, r, metrics.DefStateApplyTotal.Name, map[string]string{"result": "no_change"}); s == nil || s.GetCounter().GetValue() != 1 {
		t.Errorf("no_change = %v, want 1", s)
	}
}

func TestStatemgmtMetrics_RecordDrift(t *testing.T) {
	r := mTestRegistry(t)
	m, _ := NewMetrics(r)
	m.RecordDrift("high")
	m.RecordDrift("medium")
	m.RecordDrift("high")
	m.RecordDrift("") // empty severity → silently skipped

	if s := mSample(t, r, metrics.DefStateDriftDetectedTotal.Name, map[string]string{"severity": "high"}); s == nil || s.GetCounter().GetValue() != 2 {
		t.Errorf("high = %v, want 2", s)
	}
	if s := mSample(t, r, metrics.DefStateDriftDetectedTotal.Name, map[string]string{"severity": "medium"}); s == nil || s.GetCounter().GetValue() != 1 {
		t.Errorf("medium = %v, want 1", s)
	}
}

func TestStatemgmtMetrics_DuplicateRegistrationFails(t *testing.T) {
	r := mTestRegistry(t)
	if _, err := NewMetrics(r); err != nil {
		t.Fatal(err)
	}
	if _, err := NewMetrics(r); err == nil {
		t.Fatal("second NewMetrics: want error")
	}
}
