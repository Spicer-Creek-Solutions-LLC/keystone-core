package saga

import (
	"context"
	"errors"
	"testing"
)

func TestInMemoryLog_Roundtrip(t *testing.T) {
	t.Parallel()
	log := NewInMemoryLog()
	ctx := context.Background()

	if _, err := log.GetExecution(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetExecution(missing) = %v, want ErrNotFound", err)
	}

	e := &Execution{ID: "abc", Name: "x", Status: StatusRunning}
	if err := log.SaveExecution(ctx, e); err != nil {
		t.Fatal(err)
	}
	got, err := log.GetExecution(ctx, "abc")
	if err != nil || got.Name != "x" {
		t.Errorf("roundtrip: %+v err=%v", got, err)
	}
	// Mutating the returned copy doesn't affect what the log has.
	got.Name = "mutated"
	got2, _ := log.GetExecution(ctx, "abc")
	if got2.Name != "x" {
		t.Errorf("clone failure: caller mutated log state (got %q)", got2.Name)
	}

	// Update — same ID overwrites; no duplicate in order.
	if err := log.SaveExecution(ctx, &Execution{ID: "abc", Name: "x", Status: StatusCompleted}); err != nil {
		t.Fatal(err)
	}
	got3, _ := log.GetExecution(ctx, "abc")
	if got3.Status != StatusCompleted {
		t.Errorf("update: status = %s, want completed", got3.Status)
	}

	// Multi-ID + ordering.
	if err := log.SaveExecution(ctx, &Execution{ID: "def", Name: "y"}); err != nil {
		t.Fatal(err)
	}
	list, err := log.ListExecutions(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("ListExecutions len = %d err = %v", len(list), err)
	}
	if list[0].ID != "abc" || list[1].ID != "def" {
		t.Errorf("insertion order broken: %s, %s", list[0].ID, list[1].ID)
	}
}

func TestInMemoryLog_NilOrEmptyExecution(t *testing.T) {
	t.Parallel()
	log := NewInMemoryLog()
	ctx := context.Background()
	// Saving nil or no-ID is silently a no-op (callers shouldn't but
	// the log shouldn't panic either).
	if err := log.SaveExecution(ctx, nil); err != nil {
		t.Errorf("Save(nil) = %v, want nil", err)
	}
	if err := log.SaveExecution(ctx, &Execution{}); err != nil {
		t.Errorf("Save(empty ID) = %v, want nil", err)
	}
	if list, _ := log.ListExecutions(ctx); len(list) != 0 {
		t.Errorf("list after no-op saves should be empty, got %d", len(list))
	}
}

func TestCloneExecution_NilSafe(t *testing.T) {
	t.Parallel()
	if cloneExecution(nil) != nil {
		t.Error("cloneExecution(nil) should return nil")
	}
}
