// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"io"
	"log/slog"
	"testing"

	dto "github.com/prometheus/client_model/go"

	"go.keystone-core.io/keystone-core/internal/metrics"
)

func secretsTestRegistry(t *testing.T) *metrics.Registry {
	t.Helper()
	return metrics.NewRegistry(metrics.Options{
		Logger:                   slog.New(slog.NewTextHandler(io.Discard, nil)),
		DisableRuntimeCollectors: true,
	})
}

func sSample(t *testing.T, r *metrics.Registry, want map[string]string) *dto.Metric {
	t.Helper()
	mfs, _ := r.Gatherer().Gather()
	for _, mf := range mfs {
		if mf.GetName() != metrics.DefSecretsAccessTotal.Name {
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

func TestSecretsMetrics_NilSafe(t *testing.T) {
	m, err := NewMetrics(nil)
	if err != nil || m != nil {
		t.Fatalf("NewMetrics(nil) = %v, %v", m, err)
	}
	m.RecordAccess("file", ActionGetSecret, true)
}

func TestSecretsMetrics_RecordAccess(t *testing.T) {
	r := secretsTestRegistry(t)
	m, _ := NewMetrics(r)
	m.RecordAccess("file", ActionGetSecret, true)
	m.RecordAccess("file", ActionGetSecret, true)
	m.RecordAccess("file", ActionGetSecret, false)
	m.RecordAccess("vault", ActionWriteSecret, true)
	m.RecordAccess("", ActionRenewLease, true) // unresolved backend

	if s := sSample(t, r, map[string]string{"backend": "file", "op": ActionGetSecret, "result": "success"}); s == nil || s.GetCounter().GetValue() != 2 {
		t.Errorf("file get success = %v, want 2", s)
	}
	if s := sSample(t, r, map[string]string{"backend": "file", "op": ActionGetSecret, "result": "error"}); s == nil || s.GetCounter().GetValue() != 1 {
		t.Errorf("file get error = %v, want 1", s)
	}
	if s := sSample(t, r, map[string]string{"backend": "vault", "op": ActionWriteSecret, "result": "success"}); s == nil || s.GetCounter().GetValue() != 1 {
		t.Errorf("vault write = %v, want 1", s)
	}
	if s := sSample(t, r, map[string]string{"backend": "_unresolved", "op": ActionRenewLease, "result": "success"}); s == nil || s.GetCounter().GetValue() != 1 {
		t.Errorf("unresolved renew = %v, want 1", s)
	}
}

func TestSecretsMetrics_DuplicateRegistrationFails(t *testing.T) {
	r := secretsTestRegistry(t)
	if _, err := NewMetrics(r); err != nil {
		t.Fatal(err)
	}
	if _, err := NewMetrics(r); err == nil {
		t.Fatal("second NewMetrics: want error")
	}
}
