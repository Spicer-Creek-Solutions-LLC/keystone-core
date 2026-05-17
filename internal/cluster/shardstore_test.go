package cluster

import (
	"context"
	"errors"
	"testing"
	"time"
)

func newShardStore(t *testing.T) *ShardStore {
	t.Helper()
	ec, _ := newEmbedded(t)
	ss, err := NewShardStore(ShardStoreConfig{Etcd: ec, KeyPrefix: "/kscore/test"})
	if err != nil {
		t.Fatalf("NewShardStore: %v", err)
	}
	return ss
}

func TestNewShardStore_InvalidConfig(t *testing.T) {
	ec, _ := newEmbedded(t)
	if _, err := NewShardStore(ShardStoreConfig{KeyPrefix: "/p"}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("nil etcd: err = %v, want ErrInvalidConfig", err)
	}
	if _, err := NewShardStore(ShardStoreConfig{Etcd: ec}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("empty prefix: err = %v, want ErrInvalidConfig", err)
	}
}

func TestShardStore_AssignGetListDelete(t *testing.T) {
	ss := newShardStore(t)
	ctx := context.Background()

	a1, err := ss.Assign(ctx, "agent-1", "m1")
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if a1.Version <= 0 || a1.MemberID != "m1" {
		t.Fatalf("Assign result = %+v", a1)
	}

	got, err := ss.Get(ctx, "agent-1")
	if err != nil || got.MemberID != "m1" || got.Version != a1.Version {
		t.Fatalf("Get = %+v, %v", got, err)
	}

	if _, err := ss.Assign(ctx, "agent-2", "m2"); err != nil {
		t.Fatalf("Assign agent-2: %v", err)
	}
	list, err := ss.List(ctx)
	if err != nil || len(list) != 2 || list[0].AgentID != "agent-1" || list[1].AgentID != "agent-2" {
		t.Fatalf("List = %+v, %v", list, err)
	}

	a1b, err := ss.Assign(ctx, "agent-1", "m3") // overwrite
	if err != nil {
		t.Fatalf("Assign overwrite: %v", err)
	}
	if a1b.Version <= a1.Version {
		t.Fatalf("overwrite Version %d not > %d", a1b.Version, a1.Version)
	}

	if err := ss.Delete(ctx, "agent-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := ss.Delete(ctx, "agent-1"); err != nil {
		t.Fatalf("idempotent Delete = %v, want nil", err)
	}
	if _, err := ss.Get(ctx, "agent-1"); !errors.Is(err, ErrShardNotFound) {
		t.Fatalf("Get after Delete = %v, want ErrShardNotFound", err)
	}
}

func TestShardStore_AssignIfCreateOnly(t *testing.T) {
	ss := newShardStore(t)
	ctx := context.Background()

	if _, err := ss.AssignIf(ctx, "a", "m1", 0); err != nil {
		t.Fatalf("create-only AssignIf (absent): %v", err)
	}
	_, err := ss.AssignIf(ctx, "a", "m2", 0)
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("create-only AssignIf (present) = %v, want ErrVersionConflict", err)
	}
	// Original value untouched.
	got, _ := ss.Get(ctx, "a")
	if got.MemberID != "m1" {
		t.Fatalf("value mutated by failed create-only: %+v", got)
	}
}

func TestShardStore_AssignIfOptimisticRace(t *testing.T) {
	ss := newShardStore(t)
	ctx := context.Background()

	v1, err := ss.Assign(ctx, "a", "m1")
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}

	// Correct version → succeeds, bumps version.
	v2, err := ss.AssignIf(ctx, "a", "m2", v1.Version)
	if err != nil {
		t.Fatalf("AssignIf (current version): %v", err)
	}

	// Stale version (a competing writer already moved it) → conflict.
	if _, err := ss.AssignIf(ctx, "a", "m3", v1.Version); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("AssignIf (stale) = %v, want ErrVersionConflict", err)
	}

	// Fresh version → succeeds again.
	if _, err := ss.AssignIf(ctx, "a", "m3", v2.Version); err != nil {
		t.Fatalf("AssignIf (fresh): %v", err)
	}
	got, _ := ss.Get(ctx, "a")
	if got.MemberID != "m3" {
		t.Fatalf("final value = %q, want m3", got.MemberID)
	}
}

func TestShardStore_DeleteIf(t *testing.T) {
	ss := newShardStore(t)
	ctx := context.Background()

	a, err := ss.Assign(ctx, "a", "m1")
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if err := ss.DeleteIf(ctx, "a", a.Version-1); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("DeleteIf (stale) = %v, want ErrVersionConflict", err)
	}
	if err := ss.DeleteIf(ctx, "a", a.Version); err != nil {
		t.Fatalf("DeleteIf (correct): %v", err)
	}
	if _, err := ss.Get(ctx, "a"); !errors.Is(err, ErrShardNotFound) {
		t.Fatalf("Get after DeleteIf = %v, want ErrShardNotFound", err)
	}
	// Already absent → goal met, nil.
	if err := ss.DeleteIf(ctx, "a", 999); err != nil {
		t.Fatalf("DeleteIf (absent) = %v, want nil", err)
	}
}

func TestShardStore_Watch(t *testing.T) {
	ss := newShardStore(t)
	ctx, cancel := context.WithCancel(context.Background())

	ch, err := ss.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	if _, err := ss.Assign(context.Background(), "agent-9", "m1"); err != nil {
		t.Fatalf("Assign: %v", err)
	}
	select {
	case ev := <-ch:
		if ev.Type != ShardSet || ev.AgentID != "agent-9" || ev.MemberID != "m1" {
			t.Fatalf("set event = %+v", ev)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("no ShardSet event")
	}

	if err := ss.Delete(context.Background(), "agent-9"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	select {
	case ev := <-ch:
		if ev.Type != ShardDeleted || ev.AgentID != "agent-9" || ev.MemberID != "m1" {
			t.Fatalf("delete event = %+v (want member from prev-kv)", ev)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("no ShardDeleted event")
	}

	cancel()
	select {
	case _, open := <-ch:
		if open {
			for range ch { // drain to closed
			}
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Watch channel not closed after ctx cancel")
	}
}

func TestShardStore_OpsBeforeEtcdStarted(t *testing.T) {
	ec, err := NewEtcdClient(EtcdConfig{Mode: ModeExternal, Endpoints: []string{"http://127.0.0.1:1"}})
	if err != nil {
		t.Fatalf("NewEtcdClient: %v", err)
	}
	ss, err := NewShardStore(ShardStoreConfig{Etcd: ec, KeyPrefix: "/kscore/test"})
	if err != nil {
		t.Fatalf("NewShardStore: %v", err)
	}
	ctx := context.Background()
	if _, err := ss.Get(ctx, "a"); !errors.Is(err, ErrNotStarted) {
		t.Errorf("Get before etcd start = %v, want ErrNotStarted", err)
	}
	if _, err := ss.Assign(ctx, "a", "m"); !errors.Is(err, ErrNotStarted) {
		t.Errorf("Assign before etcd start = %v, want ErrNotStarted", err)
	}
	if _, err := ss.List(ctx); !errors.Is(err, ErrNotStarted) {
		t.Errorf("List before etcd start = %v, want ErrNotStarted", err)
	}
	if _, err := ss.Watch(ctx); !errors.Is(err, ErrNotStarted) {
		t.Errorf("Watch before etcd start = %v, want ErrNotStarted", err)
	}
}
