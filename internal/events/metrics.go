package events

import (
	"fmt"

	"go.keystone-core.io/keystone-core/internal/metrics"
)

// Metrics is the events-package emitter for v1.0 observability. It is
// nil-safe — passing nil into the publisher disables emission without
// any conditional code at the call site.
type Metrics struct {
	emitted metrics.Counter
}

// NewMetrics registers the events-package metrics against r and returns
// the emitter. Returns an error if any metric definition collides with
// an already-registered name.
func NewMetrics(r *metrics.Registry) (*Metrics, error) {
	if r == nil {
		return nil, nil
	}
	emitted, err := r.NewCounter(metrics.DefEventsEmittedTotal)
	if err != nil {
		return nil, fmt.Errorf("events: register metrics: %w", err)
	}
	return &Metrics{emitted: emitted}, nil
}

// RecordEmit records one successfully-emitted event. Safe to call on a
// nil receiver.
func (m *Metrics) RecordEmit(eventType EventType, severity Severity) {
	if m == nil {
		return
	}
	m.emitted.With(metrics.Labels{
		"type":     string(eventType),
		"severity": severity.String(),
	}).Inc()
}
