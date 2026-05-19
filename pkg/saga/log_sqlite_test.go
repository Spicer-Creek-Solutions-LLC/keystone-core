package saga

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func newTempSQLiteLog(t *testing.T) Log {
	t.Helper()
	path := filepath.Join(t.TempDir(), "saga.db")
	log, err := NewSQLiteLog(path)
	if err != nil {
		t.Fatalf("NewSQLiteLog: %v", err)
	}
	t.Cleanup(func() {
		if c, ok := log.(io.Closer); ok {
			_ = c.Close()
		}
	})
	return log
}

var sqliteFixedTime = time.Date(2026, 5, 18, 9, 30, 0, 123456789, time.UTC)

func fullExecution(id string) *Execution {
	return &Execution{
		ID:        id,
		Name:      "deploy",
		Status:    StatusAborted,
		Data:      map[string]any{"replicas": float64(3), "name": "web"},
		StartedAt: sqliteFixedTime,
		EndedAt:   sqliteFixedTime.Add(2 * time.Second),
		Error:     errors.New("step 2 failed"),
		Steps: []StepResult{
			{Name: "s1", Status: StepCompensated, StartedAt: sqliteFixedTime, Duration: 10 * time.Millisecond, Compensated: true, CompensateAt: sqliteFixedTime.Add(time.Second)},
			{Name: "s2", Status: StepFailed, StartedAt: sqliteFixedTime, Duration: 5 * time.Millisecond, Error: errors.New("step 2 failed")},
			{Name: "s3", Status: StepSkipped},
		},
		CompensateErrors: []error{errors.New("compensate \"s1\": rollback partial")},
	}
}

func TestSQLiteLog_Roundtrip(t *testing.T) {
	t.Parallel()
	log := newTempSQLiteLog(t)
	ctx := context.Background()

	if _, err := log.GetExecution(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetExecution(missing) = %v, want ErrNotFound", err)
	}

	want := fullExecution("abc")
	if err := log.SaveExecution(ctx, want); err != nil {
		t.Fatalf("SaveExecution: %v", err)
	}

	got, err := log.GetExecution(ctx, "abc")
	if err != nil {
		t.Fatalf("GetExecution: %v", err)
	}
	if got.ID != want.ID || got.Name != want.Name || got.Status != want.Status {
		t.Errorf("scalar mismatch: got %+v", got)
	}
	if !got.StartedAt.Equal(want.StartedAt) || !got.EndedAt.Equal(want.EndedAt) {
		t.Errorf("time round-trip: started=%v ended=%v", got.StartedAt, got.EndedAt)
	}
	// Data is now a generic decoded value (documented lossy round-trip).
	dm, ok := got.Data.(map[string]any)
	if !ok || dm["replicas"] != float64(3) || dm["name"] != "web" {
		t.Errorf("Data round-trip = %#v", got.Data)
	}
	if len(got.Steps) != 3 {
		t.Fatalf("steps len = %d, want 3", len(got.Steps))
	}
	if got.Steps[0].Status != StepCompensated || !got.Steps[0].Compensated {
		t.Errorf("step0 = %+v", got.Steps[0])
	}
	if got.Steps[1].Error == nil || got.Steps[1].Error.Error() != "step 2 failed" {
		t.Errorf("step1 error not preserved: %v", got.Steps[1].Error)
	}
	if got.Steps[1].Duration != 5*time.Millisecond {
		t.Errorf("step1 duration = %v", got.Steps[1].Duration)
	}
	if got.Error == nil || got.Error.Error() != "step 2 failed" {
		t.Errorf("exec error not preserved: %v", got.Error)
	}
	if len(got.CompensateErrors) != 1 || got.CompensateErrors[0].Error() != `compensate "s1": rollback partial` {
		t.Errorf("compensate errors not preserved: %v", got.CompensateErrors)
	}
}

func TestSQLiteLog_LossyErrorRoundtrip(t *testing.T) {
	t.Parallel()
	log := newTempSQLiteLog(t)
	ctx := context.Background()

	sentinel := errors.New("sentinel")
	e := &Execution{ID: "e1", Name: "n", Status: StatusFailed, Error: fmt.Errorf("wrap: %w", sentinel)}
	if err := log.SaveExecution(ctx, e); err != nil {
		t.Fatal(err)
	}
	got, err := log.GetExecution(ctx, "e1")
	if err != nil {
		t.Fatal(err)
	}
	// Message preserved...
	if got.Error.Error() != "wrap: sentinel" {
		t.Errorf("message = %q", got.Error.Error())
	}
	// ...but the wrap chain is intentionally NOT (documented contract).
	if errors.Is(got.Error, sentinel) {
		t.Error("errors.Is unexpectedly matched after round-trip; contract says wrap chain is lost")
	}
}

func TestSQLiteLog_OverwritePreservesOrder(t *testing.T) {
	t.Parallel()
	log := newTempSQLiteLog(t)
	ctx := context.Background()

	for _, id := range []string{"a", "b", "c"} {
		if err := log.SaveExecution(ctx, &Execution{ID: id, Name: id, Status: StatusRunning}); err != nil {
			t.Fatal(err)
		}
	}
	// Overwrite the first-inserted; it must keep its original slot.
	if err := log.SaveExecution(ctx, &Execution{ID: "a", Name: "a2", Status: StatusCompleted}); err != nil {
		t.Fatal(err)
	}

	list, err := log.ListExecutions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	gotIDs := []string{list[0].ID, list[1].ID, list[2].ID}
	wantIDs := []string{"a", "b", "c"}
	for i := range wantIDs {
		if gotIDs[i] != wantIDs[i] {
			t.Fatalf("order = %v, want %v", gotIDs, wantIDs)
		}
	}
	if list[0].Status != StatusCompleted || list[0].Name != "a2" {
		t.Errorf("overwrite not applied: %+v", list[0])
	}
}

func TestSQLiteLog_NilOrEmptyExecution(t *testing.T) {
	t.Parallel()
	log := newTempSQLiteLog(t)
	ctx := context.Background()

	if err := log.SaveExecution(ctx, nil); err != nil {
		t.Errorf("Save(nil) = %v", err)
	}
	if err := log.SaveExecution(ctx, &Execution{}); err != nil {
		t.Errorf("Save(empty ID) = %v", err)
	}
	if list, _ := log.ListExecutions(ctx); len(list) != 0 {
		t.Errorf("list after no-op saves = %d, want 0", len(list))
	}
}

func TestSQLiteLog_PersistsAcrossReopen(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "persist.db")
	ctx := context.Background()

	log1, err := NewSQLiteLog(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := log1.SaveExecution(ctx, fullExecution("keep")); err != nil {
		t.Fatal(err)
	}
	if err := log1.(io.Closer).Close(); err != nil {
		t.Fatal(err)
	}

	log2, err := NewSQLiteLog(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = log2.(io.Closer).Close() }()

	got, err := log2.GetExecution(ctx, "keep")
	if err != nil || got.Name != "deploy" {
		t.Fatalf("after reopen: got=%+v err=%v", got, err)
	}
}

func TestSQLiteLog_InMemoryAndCloseNilSafe(t *testing.T) {
	t.Parallel()
	log, err := NewSQLiteLog(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := log.SaveExecution(context.Background(), &Execution{ID: "m", Name: "m"}); err != nil {
		t.Fatal(err)
	}
	if err := log.(io.Closer).Close(); err != nil {
		t.Fatal(err)
	}

	var nilLog *sqliteLog
	if err := nilLog.Close(); err != nil {
		t.Errorf("nil Close = %v, want nil", err)
	}
}

func TestSQLiteLog_OpenError(t *testing.T) {
	t.Parallel()
	// A path whose parent directory does not exist fails to open.
	_, err := NewSQLiteLog(filepath.Join(t.TempDir(), "nope", "x.db"))
	if err == nil {
		t.Fatal("expected open error for nonexistent directory")
	}
}

func TestSQLiteLog_MarshalErrorSurfaces(t *testing.T) {
	t.Parallel()
	log := newTempSQLiteLog(t)
	// A channel is not JSON-marshalable; SaveExecution must surface it,
	// not silently drop the record.
	err := log.SaveExecution(context.Background(), &Execution{ID: "bad", Data: make(chan int)})
	if err == nil {
		t.Fatal("expected marshal error")
	}
}

func TestSQLiteLog_Concurrent(t *testing.T) {
	t.Parallel()
	log := newTempSQLiteLog(t)
	ctx := context.Background()

	const n = 25
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("c%02d", i)
			if err := log.SaveExecution(ctx, &Execution{ID: id, Name: id, Status: StatusCompleted}); err != nil {
				t.Errorf("save %s: %v", id, err)
			}
		}(i)
	}
	wg.Wait()

	list, err := log.ListExecutions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != n {
		t.Fatalf("list len = %d, want %d", len(list), n)
	}
}
