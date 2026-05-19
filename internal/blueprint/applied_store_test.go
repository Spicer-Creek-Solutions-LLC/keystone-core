package blueprint

import (
	"context"
	"errors"
	"testing"
)

func TestMemoryAppliedStore(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryAppliedStore()

	if _, err := s.Get(ctx, "x"); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("Get missing: %v", err)
	}
	if err := s.Save(ctx, AppliedRun{}); err == nil {
		t.Fatal("Save without ID should error")
	}

	if err := s.Save(ctx, AppliedRun{ID: "a", Status: "succeeded"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(ctx, AppliedRun{ID: "b", Status: "failed"}); err != nil {
		t.Fatal(err)
	}
	// Overwrite a — no duplicate in order.
	if err := s.Save(ctx, AppliedRun{ID: "a", Status: "failed"}); err != nil {
		t.Fatal(err)
	}

	got, err := s.Get(ctx, "a")
	if err != nil || got.Status != "failed" {
		t.Fatalf("Get a: %+v err=%v", got, err)
	}
	list, err := s.List(ctx)
	if err != nil || len(list) != 2 || list[0].ID != "a" || list[1].ID != "b" {
		t.Fatalf("List = %+v err=%v", list, err)
	}
}
