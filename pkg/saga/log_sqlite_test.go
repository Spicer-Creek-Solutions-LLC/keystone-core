package saga

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func newTestSQLiteLog(t *testing.T) *SQLiteLog {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "saga_test.db")
	log, err := NewSQLiteLog(dbPath)
	if err != nil {
		t.Fatalf("create sqlite log: %v", err)
	}
	t.Cleanup(func() { log.Close() })
	return log
}

func TestSQLiteLog_SaveGetRoundtrip(t *testing.T) {
	log := newTestSQLiteLog(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Second)
	exec := &Execution{
		ID:     "test-1",
		Name:   "deploy",
		Status: StatusCompleted,
		Data:   map[string]any{"version": "1.2.3"},
		Steps: []StepRecord{
			{Name: "validate", Status: StepCompleted},
			{Name: "deploy", Status: StepCompleted},
		},
		StartedAt: now,
		EndedAt:   &now,
	}

	if err := log.Save(ctx, exec); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := log.Get(ctx, "test-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("expected execution, got nil")
	}
	if got.Status != StatusCompleted {
		t.Errorf("status: got %q, want %q", got.Status, StatusCompleted)
	}
	if got.Name != "deploy" {
		t.Errorf("name: got %q, want %q", got.Name, "deploy")
	}
	if len(got.Steps) != 2 {
		t.Errorf("steps: got %d, want 2", len(got.Steps))
	}
	if got.Data["version"] != "1.2.3" {
		t.Errorf("data[version]: got %v, want 1.2.3", got.Data["version"])
	}
}

func TestSQLiteLog_GetNotFound(t *testing.T) {
	log := newTestSQLiteLog(t)
	got, err := log.Get(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestSQLiteLog_ListByName(t *testing.T) {
	log := newTestSQLiteLog(t)
	ctx := context.Background()

	for _, name := range []string{"deploy", "deploy", "upgrade"} {
		exec := &Execution{
			ID:        name + "-" + time.Now().Format(time.RFC3339Nano),
			Name:      name,
			Status:    StatusCompleted,
			StartedAt: time.Now(),
		}
		if err := log.Save(ctx, exec); err != nil {
			t.Fatalf("save: %v", err)
		}
	}

	list, err := log.List(ctx, "deploy", 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("got %d, want 2", len(list))
	}
}

func TestSQLiteLog_ListWithLimit(t *testing.T) {
	log := newTestSQLiteLog(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		exec := &Execution{
			ID:        fmt.Sprintf("run-%d", i),
			Name:      "test",
			Status:    StatusCompleted,
			StartedAt: time.Now(),
		}
		if err := log.Save(ctx, exec); err != nil {
			t.Fatalf("save: %v", err)
		}
	}

	list, err := log.List(ctx, "test", 3)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("got %d, want 3", len(list))
	}
}

func TestSQLiteLog_Overwrite(t *testing.T) {
	log := newTestSQLiteLog(t)
	ctx := context.Background()

	exec := &Execution{
		ID:        "overwrite-1",
		Name:      "test",
		Status:    StatusRunning,
		StartedAt: time.Now(),
	}
	if err := log.Save(ctx, exec); err != nil {
		t.Fatalf("save: %v", err)
	}

	exec.Status = StatusCompleted
	now := time.Now()
	exec.EndedAt = &now
	if err := log.Save(ctx, exec); err != nil {
		t.Fatalf("save update: %v", err)
	}

	got, err := log.Get(ctx, "overwrite-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != StatusCompleted {
		t.Errorf("status: got %q, want %q", got.Status, StatusCompleted)
	}
}

func TestSQLiteLog_ConcurrentSaves(t *testing.T) {
	log := newTestSQLiteLog(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			exec := &Execution{
				ID:        fmt.Sprintf("concurrent-%d", i),
				Name:      "test",
				Status:    StatusCompleted,
				StartedAt: time.Now(),
			}
			if err := log.Save(ctx, exec); err != nil {
				t.Errorf("save %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	list, err := log.List(ctx, "test", 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 10 {
		t.Errorf("got %d, want 10", len(list))
	}
}
