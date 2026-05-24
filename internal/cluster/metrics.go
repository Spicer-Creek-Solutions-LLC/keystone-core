// SPDX-License-Identifier: Apache-2.0

package cluster

import (
	"fmt"

	"go.keystone-core.io/keystone-core/internal/metrics"
)

// Metrics is the cluster-package emitter for v1.0 HA observability.
// Nil-safe.
type Metrics struct {
	members  metrics.Gauge
	quorum   metrics.Gauge
	failover metrics.Counter
}

// NewMetrics registers the three cluster metrics against r.
func NewMetrics(r *metrics.Registry) (*Metrics, error) {
	if r == nil {
		return nil, nil
	}
	members, err := r.NewGauge(metrics.DefClusterMembersTotal)
	if err != nil {
		return nil, fmt.Errorf("cluster: register members_total: %w", err)
	}
	quorum, err := r.NewGauge(metrics.DefClusterQuorum)
	if err != nil {
		return nil, fmt.Errorf("cluster: register quorum: %w", err)
	}
	failover, err := r.NewCounter(metrics.DefClusterFailoverTotal)
	if err != nil {
		return nil, fmt.Errorf("cluster: register failover_total: %w", err)
	}
	return &Metrics{members: members, quorum: quorum, failover: failover}, nil
}

// SetMembersByState writes a per-state gauge value. Callers compute
// the bucket counts from MembershipManager.LoadMembers and call this
// with the full distribution; absent states should be passed as 0 so
// gauges don't go stale after a transition.
func (m *Metrics) SetMembersByState(counts map[MemberStatus]int) {
	if m == nil {
		return
	}
	for _, status := range []MemberStatus{MemberHealthy, MemberDegraded, MemberUnhealthy, MemberLeaving} {
		m.members.With(metrics.Labels{"state": string(status)}).Set(float64(counts[status]))
	}
}

// SetQuorum writes 1 when the cluster has quorum, 0 when lost.
func (m *Metrics) SetQuorum(ok bool) {
	if m == nil {
		return
	}
	v := 0.0
	if ok {
		v = 1.0
	}
	m.quorum.Set(v)
}

// FailoverOutcome is the outcome label for cluster_failover_total.
type FailoverOutcome string

const (
	FailoverOutcomeCompleted   FailoverOutcome = "completed"
	FailoverOutcomeFailed      FailoverOutcome = "failed"
	FailoverOutcomeRolledBack  FailoverOutcome = "rolled_back"
)

// RecordFailover increments the failover counter by outcome.
func (m *Metrics) RecordFailover(outcome FailoverOutcome) {
	if m == nil {
		return
	}
	m.failover.With(metrics.Labels{"outcome": string(outcome)}).Inc()
}

// MetricsHealthObserver adapts Metrics to HealthObserver so a
// HealthMonitor can drive cluster_quorum on every transition.
type MetricsHealthObserver struct{ M *Metrics }

// OnHealthChange writes the new quorum value.
func (o *MetricsHealthObserver) OnHealthChange(e HealthEvent) {
	o.M.SetQuorum(e.Quorum == QuorumOK)
}

// MetricsFailoverObserver adapts Metrics to FailoverObserver so a
// FailoverManager episode-end transition increments
// cluster_failover_total by terminal state.
type MetricsFailoverObserver struct{ M *Metrics }

// OnFailover increments the failover counter when a terminal state
// (COMPLETED, FAILED, ROLLED_BACK) is reached. Other transitions are
// observed but not counted (the counter only fires once per episode).
func (o *MetricsFailoverObserver) OnFailover(e FailoverEvent) {
	switch e.State {
	case FailoverCompleted:
		o.M.RecordFailover(FailoverOutcomeCompleted)
	case FailoverFailed:
		o.M.RecordFailover(FailoverOutcomeFailed)
	case FailoverRolledBack:
		o.M.RecordFailover(FailoverOutcomeRolledBack)
	}
}

// Compile-time interface compliance.
var (
	_ HealthObserver   = (*MetricsHealthObserver)(nil)
	_ FailoverObserver = (*MetricsFailoverObserver)(nil)
)
