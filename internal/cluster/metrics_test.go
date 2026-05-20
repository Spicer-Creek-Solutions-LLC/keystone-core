package cluster

import (
	"io"
	"log/slog"
	"testing"

	dto "github.com/prometheus/client_model/go"

	"go.keystone-core.io/keystone-core/internal/metrics"
)

func clusterTestRegistry(t *testing.T) *metrics.Registry {
	t.Helper()
	return metrics.NewRegistry(metrics.Options{
		Logger:                   slog.New(slog.NewTextHandler(io.Discard, nil)),
		DisableRuntimeCollectors: true,
	})
}

func cSample(t *testing.T, r *metrics.Registry, name string, want map[string]string) *dto.Metric {
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

func TestClusterMetrics_NilSafe(t *testing.T) {
	m, err := NewMetrics(nil)
	if err != nil || m != nil {
		t.Fatalf("NewMetrics(nil) = %v, %v", m, err)
	}
	m.SetMembersByState(map[MemberStatus]int{MemberHealthy: 1})
	m.SetQuorum(true)
	m.RecordFailover(FailoverOutcomeCompleted)
}

func TestClusterMetrics_SetMembersByState(t *testing.T) {
	r := clusterTestRegistry(t)
	m, _ := NewMetrics(r)
	m.SetMembersByState(map[MemberStatus]int{
		MemberHealthy:   3,
		MemberDegraded:  1,
		MemberUnhealthy: 0,
	})
	if s := cSample(t, r, metrics.DefClusterMembersTotal.Name, map[string]string{"state": "healthy"}); s.GetGauge().GetValue() != 3 {
		t.Errorf("healthy = %v, want 3", s.GetGauge().GetValue())
	}
	if s := cSample(t, r, metrics.DefClusterMembersTotal.Name, map[string]string{"state": "degraded"}); s.GetGauge().GetValue() != 1 {
		t.Errorf("degraded = %v, want 1", s.GetGauge().GetValue())
	}
	// Re-issue with healthy collapsed; gauge must update.
	m.SetMembersByState(map[MemberStatus]int{MemberHealthy: 2})
	if s := cSample(t, r, metrics.DefClusterMembersTotal.Name, map[string]string{"state": "healthy"}); s.GetGauge().GetValue() != 2 {
		t.Errorf("healthy after re-set = %v, want 2", s.GetGauge().GetValue())
	}
}

func TestClusterMetrics_SetQuorum(t *testing.T) {
	r := clusterTestRegistry(t)
	m, _ := NewMetrics(r)
	m.SetQuorum(true)
	if s := cSample(t, r, metrics.DefClusterQuorum.Name, map[string]string{}); s.GetGauge().GetValue() != 1 {
		t.Errorf("quorum ok = %v, want 1", s.GetGauge().GetValue())
	}
	m.SetQuorum(false)
	if s := cSample(t, r, metrics.DefClusterQuorum.Name, map[string]string{}); s.GetGauge().GetValue() != 0 {
		t.Errorf("quorum lost = %v, want 0", s.GetGauge().GetValue())
	}
}

func TestClusterMetrics_RecordFailover(t *testing.T) {
	r := clusterTestRegistry(t)
	m, _ := NewMetrics(r)
	m.RecordFailover(FailoverOutcomeCompleted)
	m.RecordFailover(FailoverOutcomeCompleted)
	m.RecordFailover(FailoverOutcomeFailed)

	if s := cSample(t, r, metrics.DefClusterFailoverTotal.Name, map[string]string{"outcome": "completed"}); s.GetCounter().GetValue() != 2 {
		t.Errorf("completed = %v, want 2", s.GetCounter().GetValue())
	}
	if s := cSample(t, r, metrics.DefClusterFailoverTotal.Name, map[string]string{"outcome": "failed"}); s.GetCounter().GetValue() != 1 {
		t.Errorf("failed = %v, want 1", s.GetCounter().GetValue())
	}
}

func TestMetricsHealthObserver_DrivesQuorum(t *testing.T) {
	r := clusterTestRegistry(t)
	m, _ := NewMetrics(r)
	o := &MetricsHealthObserver{M: m}
	o.OnHealthChange(HealthEvent{Quorum: QuorumOK})
	if s := cSample(t, r, metrics.DefClusterQuorum.Name, map[string]string{}); s.GetGauge().GetValue() != 1 {
		t.Errorf("quorum ok = %v, want 1", s.GetGauge().GetValue())
	}
	o.OnHealthChange(HealthEvent{Quorum: QuorumMinority})
	if s := cSample(t, r, metrics.DefClusterQuorum.Name, map[string]string{}); s.GetGauge().GetValue() != 0 {
		t.Errorf("quorum lost = %v, want 0", s.GetGauge().GetValue())
	}
}

func TestMetricsFailoverObserver_RecordsTerminalsOnly(t *testing.T) {
	r := clusterTestRegistry(t)
	m, _ := NewMetrics(r)
	o := &MetricsFailoverObserver{M: m}

	// Non-terminal: must NOT increment the counter.
	o.OnFailover(FailoverEvent{State: FailoverDetecting})
	o.OnFailover(FailoverEvent{State: FailoverInitiated})
	o.OnFailover(FailoverEvent{State: FailoverInProgress})
	// Terminals.
	o.OnFailover(FailoverEvent{State: FailoverCompleted})
	o.OnFailover(FailoverEvent{State: FailoverFailed})
	o.OnFailover(FailoverEvent{State: FailoverRolledBack})

	if s := cSample(t, r, metrics.DefClusterFailoverTotal.Name, map[string]string{"outcome": "completed"}); s.GetCounter().GetValue() != 1 {
		t.Errorf("completed = %v, want 1", s.GetCounter().GetValue())
	}
	if s := cSample(t, r, metrics.DefClusterFailoverTotal.Name, map[string]string{"outcome": "failed"}); s.GetCounter().GetValue() != 1 {
		t.Errorf("failed = %v, want 1", s.GetCounter().GetValue())
	}
	if s := cSample(t, r, metrics.DefClusterFailoverTotal.Name, map[string]string{"outcome": "rolled_back"}); s.GetCounter().GetValue() != 1 {
		t.Errorf("rolled_back = %v, want 1", s.GetCounter().GetValue())
	}
}
