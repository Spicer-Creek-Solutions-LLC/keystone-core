// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"context"
	"errors"
)

// ErrInvalidAuditEntry is the family root for [AuditEntry] /
// [Violation] / enum-parse validation rejections. Constructors
// wrap context with `fmt.Errorf("%w: ...", ErrInvalidAuditEntry)`
// so call sites match with [errors.Is].
var ErrInvalidAuditEntry = errors.New("audit: invalid audit entry")

// ErrAuditBufferUnusable is returned by [BufferedAuditor]
// constructors when the requested capacity is non-positive.
// Distinct from [ErrInvalidAuditEntry] so call sites can branch
// on the config-validation vs runtime-data-validation case.
var ErrAuditBufferUnusable = errors.New("audit: audit buffer is unusable")

// Auditor is the seam every audit producer (policy engine in
// tasks 5-9, the auth / secrets / state-apply / exec hooks in
// task 4) writes to. The signature mirrors Epic 10's
// [internal/secrets.Auditor.Emit] precedent: no error return,
// "fire and forget; never error back to the caller." A dropped
// entry is a bug in the auditor implementation, not in the
// producer.
//
// Strict-fail semantics (Emit returns error so the producer can
// fail the in-flight op) are tracked on the v1.x ROADMAP entry
// "Strict audit-on-access via Auditor.Emit error return" landed
// during Epic 11 task 10. v1.0 keeps the best-effort contract.
//
// Implementations:
//
//   - [NoopAuditor] — default; discards every entry.
//   - [BufferedAuditor] — in-memory FIFO ring.
//   - [MultiAuditor] — fan-out across N inner auditors.
//   - [SQLitePolicyAuditStore] (task 2) — durable storage.
type Auditor interface {
	Emit(ctx context.Context, entry AuditEntry)
}

// NoopAuditor discards every entry. The default when no real
// auditor is configured.
type NoopAuditor struct{}

// Emit discards the entry.
func (NoopAuditor) Emit(context.Context, AuditEntry) {}

// Compile-time assertion that NoopAuditor satisfies [Auditor].
var _ Auditor = NoopAuditor{}
