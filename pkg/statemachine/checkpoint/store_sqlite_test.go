package checkpoint

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func newTestSQLiteStore(t *testing.T) *SQLiteStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "checkpoint_test.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("create sqlite store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestSQLiteStore_SaveLoadRoundtrip(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()

	r := &Record{
		MachineID:   "m-1",
		MachineName: "bootstrap",
		State:       "running",
		Metadata:    map[string]any{"key": "value"},
		UpdatedAt:   time.Now().Truncate(time.Second),
	}

	if err := store.Save(ctx, r); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := store.Load(ctx, "m-1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got == nil {
		t.Fatal("expected record, got nil")
	}
	if got.State != "running" {
		t.Errorf("state: got %q, want %q", got.State, "running")
	}
	if got.MachineName != "bootstrap" {
		t.Errorf("name: got %q, want %q", got.MachineName, "bootstrap")
	}
	if got.Metadata["key"] != "value" {
		t.Errorf("metadata: got %v, want value", got.Metadata["key"])
	}
}

func TestSQLiteStore_OverwriteOnReSave(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()

	r := &Record{
		MachineID:   "m-1",
		MachineName: "test",
		State:       "init",
		UpdatedAt:   time.Now(),
	}
	if err := store.Save(ctx, r); err != nil {
		t.Fatalf("save: %v", err)
	}

	r.State = "completed"
	r.UpdatedAt = time.Now()
	if err := store.Save(ctx, r); err != nil {
		t.Fatalf("save update: %v", err)
	}

	got, err := store.Load(ctx, "m-1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.State != "completed" {
		t.Errorf("state: got %q, want %q", got.State, "completed")
	}
}

func TestSQLiteStore_Delete(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()

	r := &Record{
		MachineID:   "m-1",
		MachineName: "test",
		State:       "running",
		UpdatedAt:   time.Now(),
	}
	if err := store.Save(ctx, r); err != nil {
		t.Fatalf("save: %v", err)
	}

	if err := store.Delete(ctx, "m-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	got, err := store.Load(ctx, "m-1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil after delete, got %+v", got)
	}
}

func TestSQLiteStore_List(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()

	records := []*Record{
		{MachineID: "b-1", MachineName: "bootstrap", State: "running", UpdatedAt: time.Now()},
		{MachineID: "b-2", MachineName: "bootstrap", State: "done", UpdatedAt: time.Now()},
		{MachineID: "u-1", MachineName: "upgrade", State: "running", UpdatedAt: time.Now()},
	}
	for _, r := range records {
		if err := store.Save(ctx, r); err != nil {
			t.Fatalf("save: %v", err)
		}
	}

	list, err := store.List(ctx, "bootstrap")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("got %d, want 2", len(list))
	}

	all, err := store.List(ctx, "")
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("got %d, want 3", len(all))
	}
}

func TestSQLiteStore_LoadNotFound(t *testing.T) {
	store := newTestSQLiteStore(t)
	got, err := store.Load(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}
