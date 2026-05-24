// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"context"
	"fmt"
	"sync"
)

// BufferedAuditor is the in-memory FIFO ring per PROJECT-DETAILS
// §4.12 "Auditor (in-memory circular buffer; configurable size)."
// Fan-out alongside the SQL store (task 2) — gives operators an
// in-process "tail" of recent audit activity without hitting disk.
//
// Mirrors the shape of [internal/secrets.BufferedAuditor] from
// Epic 10 task 11: capacity > 0 required; [Emit] evicts oldest
// when full; [Snapshot] returns a defensive copy (oldest-first);
// concurrent Emit + Snapshot serialised via [sync.Mutex].
type BufferedAuditor struct {
	mu       sync.Mutex
	entries  []AuditEntry // ring; len == capacity once populated
	capacity int          // immutable after construction
	next     int          // write index into entries; wraps at capacity
	count    int          // current entry count; ≤ capacity
}

// NewBufferedAuditor constructs an empty ring with the given
// capacity. Returns [ErrAuditBufferUnusable] when capacity ≤ 0 —
// a zero-capacity ring would silently drop every entry, which is
// almost certainly a configuration bug.
func NewBufferedAuditor(capacity int) (*BufferedAuditor, error) {
	if capacity <= 0 {
		return nil, fmt.Errorf("%w: capacity must be > 0, got %d", ErrAuditBufferUnusable, capacity)
	}
	return &BufferedAuditor{
		entries:  make([]AuditEntry, capacity),
		capacity: capacity,
	}, nil
}

// Emit appends entry to the ring. Evicts the oldest entry when
// the buffer is at capacity. The audit producer never sees this
// happen — the contract is "fire and forget."
func (b *BufferedAuditor) Emit(_ context.Context, entry AuditEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entries[b.next] = entry
	b.next = (b.next + 1) % b.capacity
	if b.count < b.capacity {
		b.count++
	}
}

// Snapshot returns a defensive copy of every entry currently in
// the ring, oldest-first. Returns nil when empty (callers should
// treat nil + empty slice identically).
//
// The copy is independent of the ring — mutating the returned
// slice does NOT affect subsequent Snapshot calls. Used by the
// gRPC handler (task 12) when an operator queries
// `GetAuditLog --tail` for the in-memory tail.
func (b *BufferedAuditor) Snapshot() []AuditEntry {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.count == 0 {
		return nil
	}
	out := make([]AuditEntry, 0, b.count)
	// Oldest entry: when count < capacity, it's at index 0; when
	// count == capacity, the oldest is at `next` (the next write
	// position points at the entry that would be evicted on the
	// next emit, which IS the oldest).
	start := 0
	if b.count == b.capacity {
		start = b.next
	}
	for i := 0; i < b.count; i++ {
		out = append(out, b.entries[(start+i)%b.capacity])
	}
	return out
}

// Len returns the current number of entries in the ring. Bounded
// by [BufferedAuditor.Capacity].
func (b *BufferedAuditor) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.count
}

// Capacity returns the maximum number of entries the ring can
// hold before eviction begins. Immutable after construction.
func (b *BufferedAuditor) Capacity() int {
	return b.capacity
}

// Compile-time assertion that *BufferedAuditor satisfies
// [Auditor].
var _ Auditor = (*BufferedAuditor)(nil)
