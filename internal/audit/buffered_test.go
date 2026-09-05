// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func makeEntry(t *testing.T, action string) AuditEntry {
	t.Helper()
	return MustNewAuditEntry(AuditEntryInput{Action: action})
}

func TestNewBufferedAuditor_RejectsBadCapacity(t *testing.T) {
	t.Parallel()
	for _, cap := range []int{0, -1, -100} {
		_, err := NewBufferedAuditor(cap)
		if err == nil {
			t.Errorf("NewBufferedAuditor(%d) succeeded; want error", cap)
			continue
		}
		if !errors.Is(err, ErrAuditBufferUnusable) {
			t.Errorf("NewBufferedAuditor(%d) err = %v; want ErrAuditBufferUnusable", cap, err)
		}
	}
}

func TestBufferedAuditor_Empty(t *testing.T) {
	t.Parallel()
	b, err := NewBufferedAuditor(5)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if got := b.Len(); got != 0 {
		t.Errorf("Len() = %d, want 0", got)
	}
	if got := b.Capacity(); got != 5 {
		t.Errorf("Capacity() = %d, want 5", got)
	}
	if got := b.Snapshot(); got != nil {
		t.Errorf("empty Snapshot() = %v, want nil", got)
	}
}

func TestBufferedAuditor_EmitBelowCapacity(t *testing.T) {
	t.Parallel()
	b, _ := NewBufferedAuditor(5)
	ctx := context.Background()
	entries := []AuditEntry{
		makeEntry(t, "a"),
		makeEntry(t, "b"),
		makeEntry(t, "c"),
	}
	for _, e := range entries {
		b.Emit(ctx, e)
	}
	if b.Len() != 3 {
		t.Errorf("Len = %d, want 3", b.Len())
	}
	snap := b.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("snap len = %d, want 3", len(snap))
	}
	for i, e := range entries {
		if snap[i].Action != e.Action {
			t.Errorf("snap[%d].Action = %q, want %q", i, snap[i].Action, e.Action)
		}
	}
}

func TestBufferedAuditor_FIFOEvictionAtCapacity(t *testing.T) {
	t.Parallel()
	b, _ := NewBufferedAuditor(3)
	ctx := context.Background()
	// Emit cap+2 (5) entries; oldest 2 should be evicted.
	entries := []AuditEntry{
		makeEntry(t, "a"), makeEntry(t, "b"), makeEntry(t, "c"),
		makeEntry(t, "d"), makeEntry(t, "e"),
	}
	for _, e := range entries {
		b.Emit(ctx, e)
	}
	if b.Len() != 3 {
		t.Errorf("Len = %d, want 3 (capacity)", b.Len())
	}
	snap := b.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("snap len = %d, want 3", len(snap))
	}
	// Oldest survivors are c, d, e (oldest-first).
	want := []string{"c", "d", "e"}
	for i, w := range want {
		if snap[i].Action != w {
			t.Errorf("snap[%d].Action = %q, want %q", i, snap[i].Action, w)
		}
	}
}

func TestBufferedAuditor_ExactlyAtCapacity(t *testing.T) {
	t.Parallel()
	// Emit exactly cap entries; nothing evicted, all survive.
	b, _ := NewBufferedAuditor(3)
	ctx := context.Background()
	for _, action := range []string{"a", "b", "c"} {
		b.Emit(ctx, makeEntry(t, action))
	}
	if b.Len() != 3 {
		t.Errorf("Len = %d, want 3", b.Len())
	}
	snap := b.Snapshot()
	want := []string{"a", "b", "c"}
	for i, w := range want {
		if snap[i].Action != w {
			t.Errorf("snap[%d] = %q, want %q", i, snap[i].Action, w)
		}
	}
}

func TestBufferedAuditor_DefensiveSnapshot(t *testing.T) {
	t.Parallel()
	b, _ := NewBufferedAuditor(3)
	ctx := context.Background()
	for _, action := range []string{"a", "b"} {
		b.Emit(ctx, makeEntry(t, action))
	}
	snap1 := b.Snapshot()
	// Mutate snap1 — must NOT affect future Snapshots.
	snap1[0] = AuditEntry{Action: "MUTATED"}
	snap2 := b.Snapshot()
	if snap2[0].Action == "MUTATED" {
		t.Errorf("Snapshot aliased — first call's mutation leaked: %+v", snap2)
	}
	if snap2[0].Action != "a" {
		t.Errorf("snap2[0].Action = %q, want %q", snap2[0].Action, "a")
	}
}

func TestBufferedAuditor_Concurrent(t *testing.T) {
	t.Parallel()
	const (
		emitters     = 50
		entriesEach  = 20
		snapshotters = 10
		capacity     = 500
	)
	b, _ := NewBufferedAuditor(capacity)
	ctx := context.Background()

	var wg sync.WaitGroup
	var snapshotCalls atomic.Int64

	// Concurrent emitters.
	for i := 0; i < emitters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < entriesEach; j++ {
				b.Emit(ctx, makeEntry(t, fmt.Sprintf("emit-%d-%d", id, j)))
			}
		}(i)
	}

	// Concurrent snapshotters.
	for i := 0; i < snapshotters; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = b.Snapshot()
				snapshotCalls.Add(1)
			}
		}()
	}

	wg.Wait()

	// All entries fit (1000 emitted, capacity 500 — half evicted).
	wantLen := emitters * entriesEach
	if wantLen > capacity {
		wantLen = capacity
	}
	if got := b.Len(); got != wantLen {
		t.Errorf("post-concurrent Len = %d, want %d", got, wantLen)
	}
	if snapshotCalls.Load() == 0 {
		t.Errorf("no snapshots taken")
	}
}

func TestBufferedAuditor_SnapshotOrder_RingWrapped(t *testing.T) {
	t.Parallel()
	// Specific regression: when the ring has wrapped (count == cap
	// AND next != 0), Snapshot must walk from `next` (oldest) not
	// from index 0 (would-be-oldest if the ring were unwrapped).
	b, _ := NewBufferedAuditor(3)
	ctx := context.Background()
	// Emit 5 events. After this:
	//   - next = (5 % 3) = 2
	//   - entries[0]=d, [1]=e, [2]=c  (wrapped)
	//   - oldest = c at index 2, then d at 0, then e at 1.
	for _, action := range []string{"a", "b", "c", "d", "e"} {
		b.Emit(ctx, makeEntry(t, action))
	}
	snap := b.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("snap len = %d", len(snap))
	}
	want := []string{"c", "d", "e"}
	for i, w := range want {
		if snap[i].Action != w {
			t.Errorf("wrapped-ring snap[%d] = %q, want %q (snap=%+v)", i, snap[i].Action, w, actionsOf(snap))
		}
	}
}

func actionsOf(entries []AuditEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Action
	}
	return out
}

func TestNoopAuditor_DiscardsEverything(t *testing.T) {
	t.Parallel()
	var a Auditor = NoopAuditor{}
	for i := 0; i < 100; i++ {
		a.Emit(context.Background(), makeEntry(t, "x"))
	}
	// No observable state; just confirm no panic + interface satisfied.
}
