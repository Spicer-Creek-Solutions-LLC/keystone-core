// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"fmt"

	"go.keystone-core.io/keystone-core/internal/metrics"
)

// MissReason enumerates the buckets the misses counter carries.
// "miss" is a cold lookup with no entry; "expired" is a stale entry
// that the TTL gate evicted; "bypass" is a deliberate skip
// (FromChunk > 0). Operators reading the metric can tell genuine
// cache pressure (high "miss" with low hit ratio) from "operators
// resuming partial downloads".
type MissReason string

const (
	ReasonMiss    MissReason = "miss"
	ReasonExpired MissReason = "expired"
	ReasonBypass  MissReason = "bypass"
)

// Metrics is the proxy-cache observability emitter. Nil-safe —
// constructing a Cache with metrics=nil disables emission without
// branching at the call site.
type Metrics struct {
	hits   metrics.Counter
	misses metrics.Counter
}

// NewMetrics registers the two cache metrics against r and returns
// the emitter. A nil registry yields a nil emitter (allowed; the
// Cache short-circuits emission).
func NewMetrics(r *metrics.Registry) (*Metrics, error) {
	if r == nil {
		return nil, nil
	}
	hits, err := r.NewCounter(metrics.DefFilesCacheHitsTotal)
	if err != nil {
		return nil, fmt.Errorf("proxy: register hits: %w", err)
	}
	misses, err := r.NewCounter(metrics.DefFilesCacheMissesTotal)
	if err != nil {
		return nil, fmt.Errorf("proxy: register misses: %w", err)
	}
	return &Metrics{hits: hits, misses: misses}, nil
}

// RecordHit increments the hits counter.
func (m *Metrics) RecordHit() {
	if m == nil {
		return
	}
	m.hits.Inc()
}

// RecordMiss increments the misses counter with the supplied reason
// label.
func (m *Metrics) RecordMiss(reason MissReason) {
	if m == nil {
		return
	}
	m.misses.With(metrics.Labels{"reason": string(reason)}).Inc()
}
