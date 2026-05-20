package outbound

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func sampleSubscription(id string) *Subscription {
	ts := time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC)
	return &Subscription{
		ID:         id,
		Name:       "slack",
		URL:        "https://hooks.slack.com/x",
		Secret:     "shh",
		Events:     []string{"state.drift", "policy.violation"},
		Enabled:    true,
		Headers:    map[string]string{"X-Source": "keystone"},
		MaxRetries: 5,
		TimeoutSec: 20,
		CreatedAt:  ts,
		UpdatedAt:  ts,
	}
}

func sampleDelivery(id, subID string, when time.Time) *DeliveryRecord {
	return &DeliveryRecord{
		ID:             id,
		SubscriptionID: subID,
		EventType:      "state.drift",
		EventID:        "ev-" + id,
		Status:         DeliveryPending,
		Attempt:        1,
		DeliveredAt:    when,
	}
}

// storeConformance runs the shared contract against any
// SubscriptionStore — MemoryStore and SQLiteStore both validated by
// the same suite (the verification.ResultStore precedent).
func storeConformance(t *testing.T, newStore func(t *testing.T) SubscriptionStore) {
	t.Helper()
	ctx := context.Background()

	t.Run("subscription CRUD", func(t *testing.T) {
		t.Parallel()
		s := newStore(t)
		if err := s.CreateSubscription(ctx, sampleSubscription("s1")); err != nil {
			t.Fatalf("Create: %v", err)
		}

		got, ok, err := s.GetSubscription(ctx, "s1")
		if err != nil || !ok {
			t.Fatalf("Get: ok=%v err=%v", ok, err)
		}
		if got.Name != "slack" || got.URL == "" || !got.Enabled || got.MaxRetries != 5 ||
			len(got.Events) != 2 || got.Headers["X-Source"] != "keystone" || got.Secret != "shh" {
			t.Errorf("subscription round-trip wrong: %+v", got)
		}

		upd := sampleSubscription("s1")
		upd.Name = "renamed"
		upd.Enabled = false
		upd.UpdatedAt = upd.CreatedAt.Add(time.Hour)
		if err := s.UpdateSubscription(ctx, upd); err != nil {
			t.Fatalf("Update: %v", err)
		}
		got, _, _ = s.GetSubscription(ctx, "s1")
		if got.Name != "renamed" || got.Enabled {
			t.Errorf("update did not take: %+v", got)
		}

		// List preserves insertion order across inserts.
		_ = s.CreateSubscription(ctx, sampleSubscription("s2"))
		list, err := s.ListSubscriptions(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(list) != 2 || list[0].ID != "s1" || list[1].ID != "s2" {
			t.Errorf("list order wrong: %v", []string{list[0].ID, list[1].ID})
		}

		if err := s.DeleteSubscription(ctx, "s1"); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, ok, _ := s.GetSubscription(ctx, "s1"); ok {
			t.Error("Get after Delete = ok, want !ok")
		}
	})

	t.Run("not-found semantics", func(t *testing.T) {
		t.Parallel()
		s := newStore(t)
		if sub, ok, err := s.GetSubscription(ctx, "nope"); sub != nil || ok || err != nil {
			t.Errorf("Get(nope) = %v,%v,%v want nil,false,nil", sub, ok, err)
		}
		if err := s.UpdateSubscription(ctx, sampleSubscription("missing")); !errors.Is(err, ErrSubscriptionNotFound) {
			t.Errorf("Update(missing) err = %v, want ErrSubscriptionNotFound", err)
		}
		if err := s.DeleteSubscription(ctx, "missing"); !errors.Is(err, ErrSubscriptionNotFound) {
			t.Errorf("Delete(missing) err = %v, want ErrSubscriptionNotFound", err)
		}
	})

	t.Run("bad input", func(t *testing.T) {
		t.Parallel()
		s := newStore(t)
		if err := s.CreateSubscription(ctx, nil); err == nil {
			t.Error("Create(nil) = nil, want error")
		}
		if err := s.CreateSubscription(ctx, &Subscription{}); err == nil {
			t.Error("Create(empty id) = nil, want error")
		}
		if err := s.SaveDelivery(ctx, nil); err == nil {
			t.Error("SaveDelivery(nil) = nil, want error")
		}
	})

	t.Run("delivery upsert + status transition", func(t *testing.T) {
		t.Parallel()
		s := newStore(t)
		_ = s.CreateSubscription(ctx, sampleSubscription("sub"))

		d := sampleDelivery("d1", "sub", time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC))
		if err := s.SaveDelivery(ctx, d); err != nil {
			t.Fatalf("Save: %v", err)
		}
		// Subsequent attempts upsert the same row.
		d.Status = DeliveryRetrying
		d.Attempt = 2
		d.StatusCode = 502
		d.Error = "bad gateway"
		if err := s.SaveDelivery(ctx, d); err != nil {
			t.Fatalf("re-Save: %v", err)
		}
		got, ok, _ := s.GetDelivery(ctx, "d1")
		if !ok || got.Status != DeliveryRetrying || got.Attempt != 2 || got.StatusCode != 502 || got.Error != "bad gateway" {
			t.Errorf("upsert did not update: %+v", got)
		}
	})

	t.Run("delivery list filter + limit + order", func(t *testing.T) {
		t.Parallel()
		s := newStore(t)
		_ = s.CreateSubscription(ctx, sampleSubscription("subA"))
		_ = s.CreateSubscription(ctx, sampleSubscription("subB"))
		base := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
		_ = s.SaveDelivery(ctx, sampleDelivery("a1", "subA", base))
		_ = s.SaveDelivery(ctx, sampleDelivery("b1", "subB", base.Add(time.Minute)))
		_ = s.SaveDelivery(ctx, sampleDelivery("a2", "subA", base.Add(2*time.Minute)))

		all, _ := s.ListDeliveries(ctx, "", 0)
		if len(all) != 3 {
			t.Errorf("list all = %d, want 3", len(all))
		}
		subA, _ := s.ListDeliveries(ctx, "subA", 0)
		if len(subA) != 2 || subA[0].ID != "a1" || subA[1].ID != "a2" {
			t.Errorf("filter+order wrong: %v", subA)
		}
		one, _ := s.ListDeliveries(ctx, "", 1)
		if len(one) != 1 {
			t.Errorf("limit=1 returned %d", len(one))
		}
	})

	t.Run("DeleteOldDeliveries prunes by delivered_at", func(t *testing.T) {
		t.Parallel()
		s := newStore(t)
		_ = s.CreateSubscription(ctx, sampleSubscription("sub"))
		old := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
		newer := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
		_ = s.SaveDelivery(ctx, sampleDelivery("old1", "sub", old))
		_ = s.SaveDelivery(ctx, sampleDelivery("new1", "sub", newer))

		n, err := s.DeleteOldDeliveries(ctx, time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC))
		if err != nil {
			t.Fatalf("DeleteOldDeliveries: %v", err)
		}
		if n != 1 {
			t.Errorf("deleted = %d, want 1", n)
		}
		if _, ok, _ := s.GetDelivery(ctx, "old1"); ok {
			t.Error("old1 should be pruned")
		}
		if _, ok, _ := s.GetDelivery(ctx, "new1"); !ok {
			t.Error("new1 must survive")
		}
	})
}

func TestMemoryStore_Conformance(t *testing.T) {
	t.Parallel()
	storeConformance(t, func(t *testing.T) SubscriptionStore { return NewMemoryStore() })
}

func TestSQLiteStore_Conformance(t *testing.T) {
	t.Parallel()
	storeConformance(t, func(t *testing.T) SubscriptionStore {
		s, err := NewSQLiteStore(":memory:")
		if err != nil {
			t.Fatalf("NewSQLiteStore: %v", err)
		}
		t.Cleanup(func() { _ = s.Close() })
		return s
	})
}

func TestMemoryStore_ReturnsCopies(t *testing.T) {
	t.Parallel()
	s := NewMemoryStore()
	_ = s.CreateSubscription(context.Background(), sampleSubscription("x"))
	got, _, _ := s.GetSubscription(context.Background(), "x")
	got.Name = "MUTATED"
	got.Events[0] = "MUTATED"
	got.Headers["X-Source"] = "MUTATED"
	again, _, _ := s.GetSubscription(context.Background(), "x")
	if again.Name != "slack" || again.Events[0] != "state.drift" || again.Headers["X-Source"] != "keystone" {
		t.Errorf("store aliased: mutation leaked into %+v", again)
	}
}

func TestSQLiteStore_PersistsAcrossReopen(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "outbound.db")
	s1, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("open1: %v", err)
	}
	if err := s1.CreateSubscription(context.Background(), sampleSubscription("persist")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_ = s1.SaveDelivery(context.Background(), sampleDelivery("d-persist", "persist",
		time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)))
	_ = s1.Close()

	s2, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("open2: %v", err)
	}
	t.Cleanup(func() { _ = s2.Close() })
	sub, ok, err := s2.GetSubscription(context.Background(), "persist")
	if err != nil || !ok || sub.Name != "slack" || sub.Secret != "shh" {
		t.Errorf("subscription did not survive reopen: ok=%v err=%v sub=%+v", ok, err, sub)
	}
	d, ok, err := s2.GetDelivery(context.Background(), "d-persist")
	if err != nil || !ok || d.SubscriptionID != "persist" {
		t.Errorf("delivery did not survive reopen: ok=%v err=%v d=%+v", ok, err, d)
	}
}

func TestDeliveryStatus_Valid(t *testing.T) {
	t.Parallel()
	for _, s := range []DeliveryStatus{DeliveryPending, DeliveryRetrying, DeliverySuccess, DeliveryFailed} {
		if !s.Valid() {
			t.Errorf("%q.Valid() = false", s)
		}
	}
	if DeliveryStatus("bogus").Valid() {
		t.Error("bogus.Valid() = true")
	}
}

var (
	_ SubscriptionStore = (*MemoryStore)(nil)
	_ SubscriptionStore = (*SQLiteStore)(nil)
)
