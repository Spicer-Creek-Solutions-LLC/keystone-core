// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"context"
	"sync"
	"testing"
)

// recordingAuditor captures every Emit call for fan-out assertion.
type recordingAuditor struct {
	mu      sync.Mutex
	entries []AuditEntry
}

func (r *recordingAuditor) Emit(_ context.Context, e AuditEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, e)
}

func (r *recordingAuditor) snapshot() []AuditEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]AuditEntry, len(r.entries))
	copy(out, r.entries)
	return out
}

func TestMultiAuditor_FansOutToAllInner(t *testing.T) {
	t.Parallel()
	a := &recordingAuditor{}
	b := &recordingAuditor{}
	c := &recordingAuditor{}
	m := NewMultiAuditor(a, b, c)
	if got := m.Len(); got != 3 {
		t.Errorf("Len = %d, want 3", got)
	}
	entry := MustNewAuditEntry(AuditEntryInput{Action: "test"})
	m.Emit(context.Background(), entry)
	for i, inner := range []*recordingAuditor{a, b, c} {
		snap := inner.snapshot()
		if len(snap) != 1 {
			t.Errorf("inner[%d] received %d, want 1", i, len(snap))
		}
		if len(snap) > 0 && snap[0].ID != entry.ID {
			t.Errorf("inner[%d] received wrong entry: id=%s, want %s", i, snap[0].ID, entry.ID)
		}
	}
}

func TestMultiAuditor_SkipsNilEntries(t *testing.T) {
	t.Parallel()
	a := &recordingAuditor{}
	c := &recordingAuditor{}
	m := NewMultiAuditor(a, nil, c, nil)
	if got := m.Len(); got != 2 {
		t.Errorf("Len = %d, want 2 (nils skipped)", got)
	}
	m.Emit(context.Background(), MustNewAuditEntry(AuditEntryInput{Action: "test"}))
	if len(a.snapshot()) != 1 || len(c.snapshot()) != 1 {
		t.Errorf("non-nil inner auditors didn't receive entry")
	}
}

func TestMultiAuditor_EmptyDoesNotPanic(t *testing.T) {
	t.Parallel()
	m := NewMultiAuditor()
	if got := m.Len(); got != 0 {
		t.Errorf("Len = %d, want 0", got)
	}
	// Emit on empty MultiAuditor is a no-op.
	m.Emit(context.Background(), MustNewAuditEntry(AuditEntryInput{Action: "test"}))
}

func TestMultiAuditor_AllNilCollapses(t *testing.T) {
	t.Parallel()
	m := NewMultiAuditor(nil, nil, nil)
	if got := m.Len(); got != 0 {
		t.Errorf("Len = %d, want 0 (all nil)", got)
	}
	m.Emit(context.Background(), MustNewAuditEntry(AuditEntryInput{Action: "test"}))
}

func TestMultiAuditor_PreservesRegistrationOrder(t *testing.T) {
	t.Parallel()
	// Inner auditors should receive entries in the order they were
	// registered with the MultiAuditor — important for the "log
	// first, then buffer, then fan to SQL" pipeline shape Epic 12
	// task 4 wires.
	emitOrder := []int{}
	var emitMu sync.Mutex
	emit := func(id int) Auditor {
		return &funcAuditor{
			fn: func(_ context.Context, _ AuditEntry) {
				emitMu.Lock()
				emitOrder = append(emitOrder, id)
				emitMu.Unlock()
			},
		}
	}

	m := NewMultiAuditor(emit(1), emit(2), emit(3))
	m.Emit(context.Background(), MustNewAuditEntry(AuditEntryInput{Action: "test"}))

	emitMu.Lock()
	defer emitMu.Unlock()
	if len(emitOrder) != 3 {
		t.Fatalf("emit order len = %d, want 3", len(emitOrder))
	}
	for i, id := range []int{1, 2, 3} {
		if emitOrder[i] != id {
			t.Errorf("emit order [%d] = %d, want %d", i, emitOrder[i], id)
		}
	}
}

// funcAuditor adapts a plain function into the [Auditor]
// interface. Test-only convenience.
type funcAuditor struct {
	fn func(context.Context, AuditEntry)
}

func (f *funcAuditor) Emit(ctx context.Context, e AuditEntry) {
	f.fn(ctx, e)
}
