// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"context"
	"fmt"
	"strconv"

	"go.keystone-core.io/keystone-core/internal/metrics"
)

// Metrics is the audit-package emitter for kscore_audit_entries_total.
// Nil-safe: pass nil to disable.
type Metrics struct {
	entries metrics.Counter
}

// NewMetrics registers the audit metric against r.
func NewMetrics(r *metrics.Registry) (*Metrics, error) {
	if r == nil {
		return nil, nil
	}
	c, err := r.NewCounter(metrics.DefAuditEntriesTotal)
	if err != nil {
		return nil, fmt.Errorf("audit: register entries metric: %w", err)
	}
	return &Metrics{entries: c}, nil
}

// RecordEntry observes one written audit entry.
func (m *Metrics) RecordEntry(policyName string, allowed bool) {
	if m == nil {
		return
	}
	if policyName == "" {
		policyName = "_unspecified"
	}
	m.entries.With(metrics.Labels{
		"policy":  policyName,
		"allowed": strconv.FormatBool(allowed),
	}).Inc()
}

// MeasuringAuditor wraps an inner Auditor and emits the audit_entries
// counter on every Emit. Useful in production wiring where the
// outermost Auditor is the metric-tracked one.
type MeasuringAuditor struct {
	inner   Auditor
	metrics *Metrics
}

// NewMeasuringAuditor returns an Auditor that records each entry and
// then forwards to inner. inner==nil falls back to NoopAuditor.
func NewMeasuringAuditor(inner Auditor, m *Metrics) *MeasuringAuditor {
	if inner == nil {
		inner = NoopAuditor{}
	}
	return &MeasuringAuditor{inner: inner, metrics: m}
}

// Emit records the entry's policy + allowed labels then forwards.
func (a *MeasuringAuditor) Emit(ctx context.Context, entry AuditEntry) {
	a.metrics.RecordEntry(entry.PolicyName, entry.Allowed)
	a.inner.Emit(ctx, entry)
}

var _ Auditor = (*MeasuringAuditor)(nil)
