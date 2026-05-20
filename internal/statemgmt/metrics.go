package statemgmt

import (
	"fmt"

	"go.keystone-core.io/keystone-core/internal/metrics"
)

// Metrics is the statemgmt-package emitter for the v1.0 state-apply
// and drift-detection metrics. Nil-safe: callers that don't care about
// metrics leave Runner.Metrics / Detector.Metrics unset and observation
// silently no-ops.
type Metrics struct {
	apply metrics.Counter
	drift metrics.Counter
}

// NewMetrics registers the statemgmt-package metrics against r.
func NewMetrics(r *metrics.Registry) (*Metrics, error) {
	if r == nil {
		return nil, nil
	}
	apply, err := r.NewCounter(metrics.DefStateApplyTotal)
	if err != nil {
		return nil, fmt.Errorf("statemgmt: register state_apply_total: %w", err)
	}
	drift, err := r.NewCounter(metrics.DefStateDriftDetectedTotal)
	if err != nil {
		return nil, fmt.Errorf("statemgmt: register state_drift_detected_total: %w", err)
	}
	return &Metrics{apply: apply, drift: drift}, nil
}

// ApplyResult is the result label for state_apply_total. The runner
// folds Outcome into one of these three buckets so dashboards stay
// stable across taxonomy changes downstream.
type ApplyResult string

const (
	ApplyResultSuccess  ApplyResult = "success"
	ApplyResultFailed   ApplyResult = "failed"
	ApplyResultNoChange ApplyResult = "no_change"
)

// RecordApply records one apply outcome. Safe on nil receiver.
func (m *Metrics) RecordApply(result ApplyResult) {
	if m == nil {
		return
	}
	m.apply.With(metrics.Labels{"result": string(result)}).Inc()
}

// RecordDrift records one drift-detection run that found drift. severity
// is the aggregate severity (max across drifted declarations); the
// caller passes "" when no drift was found, in which case we skip the
// observation (drift_detected_total is only incremented when drift
// actually fired).
func (m *Metrics) RecordDrift(severity string) {
	if m == nil || severity == "" {
		return
	}
	m.drift.With(metrics.Labels{"severity": severity}).Inc()
}
