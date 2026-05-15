package audit

import "context"

// MultiAuditor fans every [AuditEntry] to N inner [Auditor]s in
// registration order. Nil entries are silently skipped — friendly
// to construction code that conditionally builds the audit
// pipeline (e.g., the SQL store is optional; the buffered tail is
// optional).
//
// Mirrors [internal/secrets.MultiAuditor] from Epic 10 task 11.
// No mutex — the auditors slice is immutable after construction;
// each inner Auditor handles its own concurrency.
type MultiAuditor struct {
	auditors []Auditor
}

// NewMultiAuditor builds a fan-out auditor over the given inner
// auditors. Nil entries are dropped at construction (vs at emit
// time) so the inner slice is dense.
//
// Empty input is allowed — a MultiAuditor with zero inner
// auditors silently discards every entry. Used by task 4's
// hook-wiring when neither the SQL store nor the buffered tail is
// configured.
func NewMultiAuditor(auditors ...Auditor) *MultiAuditor {
	out := make([]Auditor, 0, len(auditors))
	for _, a := range auditors {
		if a != nil {
			out = append(out, a)
		}
	}
	return &MultiAuditor{auditors: out}
}

// Emit fans entry to every inner auditor in registration order.
// Per the [Auditor] contract, inner auditor failures don't
// propagate — the contract is "fire and forget."
func (m *MultiAuditor) Emit(ctx context.Context, entry AuditEntry) {
	for _, a := range m.auditors {
		a.Emit(ctx, entry)
	}
}

// Len returns the number of inner auditors (post nil-skipping at
// construction). Useful for test assertions + operator
// introspection.
func (m *MultiAuditor) Len() int {
	return len(m.auditors)
}

// Compile-time assertion that *MultiAuditor satisfies [Auditor].
var _ Auditor = (*MultiAuditor)(nil)
