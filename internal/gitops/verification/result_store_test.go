package verification

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func sampleStored(id string) *StoredVerification {
	return &StoredVerification{
		ID:          id,
		Application: "web",
		CreatedAt:   time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC),
		Result: WorkflowResult{
			Name:     "post-deploy",
			Success:  false,
			Duration: 2 * time.Second,
			Steps: []StepResult{
				{Name: "http", Type: "http", Result: Result{Success: true, Message: "200", Duration: 50 * time.Millisecond, Retries: 1}},
				{Name: "grpc", Type: "grpc", Optional: true, Result: Result{Success: false, Error: errors.New("NOT_SERVING"), Duration: time.Second}},
			},
		},
	}
}

// storeConformance runs the shared contract against any ResultStore.
func storeConformance(t *testing.T, newStore func(t *testing.T) ResultStore) {
	t.Helper()
	ctx := context.Background()

	t.Run("save/get round-trip", func(t *testing.T) {
		t.Parallel()
		s := newStore(t)
		if err := s.Save(ctx, sampleStored("v1")); err != nil {
			t.Fatalf("Save: %v", err)
		}
		got, ok, err := s.Get(ctx, "v1")
		if err != nil || !ok {
			t.Fatalf("Get: ok=%v err=%v", ok, err)
		}
		if got.Application != "web" || got.Result.Name != "post-deploy" || got.Result.Success {
			t.Errorf("scalar round-trip wrong: %+v", got)
		}
		if len(got.Result.Steps) != 2 {
			t.Fatalf("steps len = %d, want 2", len(got.Result.Steps))
		}
		if got.Result.Duration != 2*time.Second || got.Result.Steps[0].Result.Duration != 50*time.Millisecond {
			t.Errorf("duration round-trip wrong: %v / %v", got.Result.Duration, got.Result.Steps[0].Result.Duration)
		}
		if got.Result.Steps[0].Result.Retries != 1 || !got.Result.Steps[1].Optional {
			t.Errorf("step fields round-trip wrong: %+v", got.Result.Steps)
		}
		// Lossy-error contract.
		if e := got.Result.Steps[1].Result.Error; e == nil || e.Error() != "NOT_SERVING" {
			t.Errorf("step error round-trip = %v", e)
		}
		if !got.CreatedAt.Equal(time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)) {
			t.Errorf("CreatedAt = %v", got.CreatedAt)
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		s := newStore(t)
		sv, ok, err := s.Get(ctx, "nope")
		if sv != nil || ok || err != nil {
			t.Errorf("Get(nope) = %v,%v,%v want nil,false,nil", sv, ok, err)
		}
	})

	t.Run("bad input", func(t *testing.T) {
		t.Parallel()
		s := newStore(t)
		if err := s.Save(ctx, nil); err == nil {
			t.Error("Save(nil) = nil, want error")
		}
		if err := s.Save(ctx, &StoredVerification{}); err == nil {
			t.Error("Save(empty id) = nil, want error")
		}
	})

	t.Run("upsert + list order", func(t *testing.T) {
		t.Parallel()
		s := newStore(t)
		_ = s.Save(ctx, sampleStored("a"))
		_ = s.Save(ctx, sampleStored("b"))
		upd := sampleStored("a")
		upd.Result.Success = true
		if err := s.Save(ctx, upd); err != nil {
			t.Fatalf("re-Save: %v", err)
		}
		got, _, _ := s.Get(ctx, "a")
		if !got.Result.Success {
			t.Error("upsert did not update success")
		}
		list, err := s.List(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(list) != 2 || list[0].ID != "a" || list[1].ID != "b" {
			t.Errorf("list seq order not stable: %v", []string{list[0].ID, list[1].ID})
		}
	})
}

func TestMemoryResultStore_Conformance(t *testing.T) {
	t.Parallel()
	storeConformance(t, func(t *testing.T) ResultStore { return NewMemoryResultStore() })
}

func TestSQLiteResultStore_Conformance(t *testing.T) {
	t.Parallel()
	storeConformance(t, func(t *testing.T) ResultStore {
		s, err := NewSQLiteResultStore(":memory:")
		if err != nil {
			t.Fatalf("NewSQLiteResultStore: %v", err)
		}
		t.Cleanup(func() { _ = s.Close() })
		return s
	})
}

func TestMemoryResultStore_ReturnsCopies(t *testing.T) {
	t.Parallel()
	s := NewMemoryResultStore()
	_ = s.Save(context.Background(), sampleStored("x"))
	got, _, _ := s.Get(context.Background(), "x")
	got.Result.Steps[0].Name = "MUTATED"
	again, _, _ := s.Get(context.Background(), "x")
	if again.Result.Steps[0].Name != "http" {
		t.Errorf("store aliased: mutation leaked (%s)", again.Result.Steps[0].Name)
	}
}

func TestSQLiteResultStore_PersistsAcrossReopen(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "v.db")
	s1, err := NewSQLiteResultStore(path)
	if err != nil {
		t.Fatalf("open1: %v", err)
	}
	_ = s1.Save(context.Background(), sampleStored("persist"))
	_ = s1.Close()

	s2, err := NewSQLiteResultStore(path)
	if err != nil {
		t.Fatalf("open2: %v", err)
	}
	t.Cleanup(func() { _ = s2.Close() })
	got, ok, err := s2.Get(context.Background(), "persist")
	if err != nil || !ok || got.Result.Name != "post-deploy" {
		t.Errorf("record did not survive reopen: ok=%v err=%v", ok, err)
	}
}

var (
	_ ResultStore = (*MemoryResultStore)(nil)
	_ ResultStore = (*SQLiteResultStore)(nil)
)
