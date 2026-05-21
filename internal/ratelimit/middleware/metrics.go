package middleware

import (
	"fmt"

	"go.keystone-core.io/keystone-core/internal/metrics"
)

// Reason values emitted on the [DefRatelimitRejectedTotal] label.
// v1.0 only ever emits ReasonLimitExceeded; v1.x adds entries.
const (
	ReasonLimitExceeded = "limit_exceeded"
)

// Metrics is the rate-limit middleware emitter. Nil-safe — a
// middleware constructed with metrics=nil simply does not count
// rejections, matching the Epic-17 per-package convention.
type Metrics struct {
	rejected metrics.Counter
}

// NewMetrics registers the middleware metric against r and
// returns the emitter. A nil registry yields a nil emitter (the
// middleware short-circuits emission).
func NewMetrics(r *metrics.Registry) (*Metrics, error) {
	if r == nil {
		return nil, nil
	}
	rejected, err := r.NewCounter(metrics.DefRatelimitRejectedTotal)
	if err != nil {
		return nil, fmt.Errorf("ratelimit/middleware: register rejected: %w", err)
	}
	return &Metrics{rejected: rejected}, nil
}

// RecordReject increments the rejections counter under the given
// reason label.
func (m *Metrics) RecordReject(reason string) {
	if m == nil {
		return
	}
	m.rejected.With(metrics.Labels{"reason": reason}).Inc()
}
