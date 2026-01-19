package statemgmt

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestDefaultTransactionConfig(t *testing.T) {
	config := DefaultTransactionConfig()

	if config.Timeout != 5*time.Minute {
		t.Errorf("expected 5m timeout, got %v", config.Timeout)
	}

	if config.MaxRollbackRetries != 3 {
		t.Errorf("expected 3 max retries, got %d", config.MaxRollbackRetries)
	}

	if !config.RollbackOnPartialFailure {
		t.Error("expected RollbackOnPartialFailure to be true")
	}

	if !config.ContinueOnRollbackError {
		t.Error("expected ContinueOnRollbackError to be true")
	}

	if !config.SavepointEnabled {
		t.Error("expected SavepointEnabled to be true")
	}
}

func TestNewTransactionManager(t *testing.T) {
	tests := []struct {
		name   string
		config *TransactionConfig
	}{
		{"nil config", nil},
		{"default config", DefaultTransactionConfig()},
		{"custom config", &TransactionConfig{Timeout: 1 * time.Minute}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := NewTransactionManager(tt.config)
			if tm == nil {
				t.Fatal("expected non-nil manager")
			}
		})
	}
}

func TestTransactionManager_Begin(t *testing.T) {
	tm := NewTransactionManager(nil)
	ctx := context.Background()

	txn, err := tm.Begin(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if txn == nil {
		t.Fatal("expected non-nil transaction")
	}

	if txn.Status != TransactionStatusActive {
		t.Errorf("expected active status, got %s", txn.Status)
	}

	// Second begin should fail
	_, err = tm.Begin(ctx)
	if err == nil {
		t.Error("expected error for second begin")
	}
}

func TestTransactionManager_Commit(t *testing.T) {
	tm := NewTransactionManager(nil)
	ctx := context.Background()

	// Commit without transaction should fail
	err := tm.Commit(ctx)
	if err == nil {
		t.Error("expected error for commit without transaction")
	}

	// Begin and commit
	txn, _ := tm.Begin(ctx)

	// Record a successful operation
	tm.RecordOperation(&TransactionOperation{
		ID:      "op1",
		Success: true,
	}, nil)

	err = tm.Commit(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if txn.Status != TransactionStatusCommitted {
		t.Errorf("expected committed status, got %s", txn.Status)
	}

	// No active transaction after commit
	if tm.GetActive() != nil {
		t.Error("expected no active transaction after commit")
	}
}

func TestTransactionManager_CommitWithFailedOperation(t *testing.T) {
	tm := NewTransactionManager(&TransactionConfig{
		RollbackOnPartialFailure: false, // Disable auto-rollback for this test
	})
	ctx := context.Background()

	tm.Begin(ctx)

	// Record a failed operation
	tm.RecordOperation(&TransactionOperation{
		ID:      "op1",
		Success: false,
		Error:   errors.New("operation failed"),
	}, nil)

	err := tm.Commit(ctx)
	if err == nil {
		t.Error("expected error for commit with failed operation")
	}
}

func TestTransactionManager_Rollback(t *testing.T) {
	var rollbackCalled bool
	tm := NewTransactionManager(&TransactionConfig{
		RollbackOnPartialFailure: false,
		OnRollback: func(txn *Transaction, err error) {
			rollbackCalled = true
		},
	})
	ctx := context.Background()

	// Rollback without transaction should fail
	err := tm.Rollback(ctx, nil)
	if err == nil {
		t.Error("expected error for rollback without transaction")
	}

	// Begin and rollback
	txn, _ := tm.Begin(ctx)

	rollbackExecuted := false
	tm.RecordOperation(&TransactionOperation{
		ID:      "op1",
		Success: true,
	}, func(ctx context.Context) error {
		rollbackExecuted = true
		return nil
	})

	err = tm.Rollback(ctx, errors.New("test rollback"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if txn.Status != TransactionStatusRolledBack {
		t.Errorf("expected rolled_back status, got %s", txn.Status)
	}

	if !rollbackCalled {
		t.Error("expected OnRollback callback to be called")
	}

	if !rollbackExecuted {
		t.Error("expected rollback function to be executed")
	}
}

func TestTransactionManager_RollbackInReverseOrder(t *testing.T) {
	tm := NewTransactionManager(&TransactionConfig{
		RollbackOnPartialFailure: false,
	})
	ctx := context.Background()

	var order []int

	tm.Begin(ctx)

	tm.RecordOperation(&TransactionOperation{ID: "op1", Success: true}, func(ctx context.Context) error {
		order = append(order, 1)
		return nil
	})
	tm.RecordOperation(&TransactionOperation{ID: "op2", Success: true}, func(ctx context.Context) error {
		order = append(order, 2)
		return nil
	})
	tm.RecordOperation(&TransactionOperation{ID: "op3", Success: true}, func(ctx context.Context) error {
		order = append(order, 3)
		return nil
	})

	tm.Rollback(ctx, nil)

	// Should be in reverse order: 3, 2, 1
	expected := []int{3, 2, 1}
	if len(order) != len(expected) {
		t.Fatalf("expected %d rollbacks, got %d", len(expected), len(order))
	}

	for i, v := range expected {
		if order[i] != v {
			t.Errorf("position %d: expected %d, got %d", i, v, order[i])
		}
	}
}

func TestTransactionManager_RollbackWithRetry(t *testing.T) {
	var attempts int32
	tm := NewTransactionManager(&TransactionConfig{
		MaxRollbackRetries:       3,
		RollbackOnPartialFailure: false,
	})
	ctx := context.Background()

	tm.Begin(ctx)

	// Rollback that fails twice then succeeds
	tm.RecordOperation(&TransactionOperation{ID: "op1", Success: true}, func(ctx context.Context) error {
		atomic.AddInt32(&attempts, 1)
		if atomic.LoadInt32(&attempts) < 3 {
			return errors.New("temporary failure")
		}
		return nil
	})

	err := tm.Rollback(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestTransactionManager_RecordOperation(t *testing.T) {
	var opCompleteCallCount int
	tm := NewTransactionManager(&TransactionConfig{
		RollbackOnPartialFailure: false,
		OnOperationComplete: func(op *TransactionOperation) {
			opCompleteCallCount++
		},
	})
	ctx := context.Background()

	// Record without transaction should fail
	err := tm.RecordOperation(&TransactionOperation{ID: "op1"}, nil)
	if err == nil {
		t.Error("expected error for record without transaction")
	}

	tm.Begin(ctx)

	err = tm.RecordOperation(&TransactionOperation{
		ID:      "op1",
		Type:    OperationTypeCreate,
		StateID: "/etc/nginx/nginx.conf",
		Module:  "file",
		Success: true,
	}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if opCompleteCallCount != 1 {
		t.Errorf("expected 1 callback, got %d", opCompleteCallCount)
	}
}

func TestTransactionManager_Savepoint(t *testing.T) {
	tm := NewTransactionManager(&TransactionConfig{
		SavepointEnabled:         true,
		RollbackOnPartialFailure: false,
	})
	ctx := context.Background()

	// Savepoint without transaction should fail
	_, err := tm.CreateSavepoint("sp1")
	if err == nil {
		t.Error("expected error for savepoint without transaction")
	}

	tm.Begin(ctx)

	// Create savepoint before any operations
	sp1, err := tm.CreateSavepoint("sp1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sp1.Index != 0 {
		t.Errorf("expected index 0, got %d", sp1.Index)
	}

	// Add some operations
	tm.RecordOperation(&TransactionOperation{ID: "op1", Success: true}, func(ctx context.Context) error { return nil })
	tm.RecordOperation(&TransactionOperation{ID: "op2", Success: true}, func(ctx context.Context) error { return nil })

	// Create another savepoint
	sp2, err := tm.CreateSavepoint("sp2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sp2.Index != 2 {
		t.Errorf("expected index 2, got %d", sp2.Index)
	}

	// Add more operations
	tm.RecordOperation(&TransactionOperation{ID: "op3", Success: true}, func(ctx context.Context) error { return nil })

	// Rollback to sp2 should only affect op3
	err = tm.RollbackToSavepoint(ctx, sp2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	txn := tm.GetActive()
	if len(txn.operations) != 2 {
		t.Errorf("expected 2 operations after rollback, got %d", len(txn.operations))
	}
}

func TestTransactionManager_SavepointDisabled(t *testing.T) {
	tm := NewTransactionManager(&TransactionConfig{
		SavepointEnabled: false,
	})
	ctx := context.Background()

	tm.Begin(ctx)

	_, err := tm.CreateSavepoint("sp1")
	if err == nil {
		t.Error("expected error when savepoints are disabled")
	}
}

func TestTransaction_GetResult(t *testing.T) {
	tm := NewTransactionManager(&TransactionConfig{
		RollbackOnPartialFailure: false,
	})
	ctx := context.Background()

	txn, _ := tm.Begin(ctx)

	tm.RecordOperation(&TransactionOperation{ID: "op1", Success: true}, nil)
	tm.RecordOperation(&TransactionOperation{ID: "op2", Success: true}, nil)
	tm.RecordOperation(&TransactionOperation{ID: "op3", Success: false, Error: errors.New("failed")}, nil)

	result := txn.GetResult()

	if result.TransactionID != txn.ID {
		t.Error("transaction ID mismatch")
	}

	if result.OperationCount != 3 {
		t.Errorf("expected 3 operations, got %d", result.OperationCount)
	}

	if result.SuccessCount != 2 {
		t.Errorf("expected 2 successes, got %d", result.SuccessCount)
	}

	if result.FailureCount != 1 {
		t.Errorf("expected 1 failure, got %d", result.FailureCount)
	}
}

func TestNewSnapshotStore(t *testing.T) {
	tests := []struct {
		name      string
		maxPerKey int
		wantMax   int
	}{
		{"default", 0, 10},
		{"negative", -5, 10},
		{"custom", 5, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ss := NewSnapshotStore(tt.maxPerKey)
			if ss.maxPerKey != tt.wantMax {
				t.Errorf("expected maxPerKey %d, got %d", tt.wantMax, ss.maxPerKey)
			}
		})
	}
}

func TestSnapshotStore_SaveAndGet(t *testing.T) {
	ss := NewSnapshotStore(10)

	snapshot := &StateSnapshot{
		StateID:   "/etc/nginx/nginx.conf",
		Module:    "file",
		Timestamp: time.Now(),
		State:     "content",
	}

	ss.Save(snapshot)

	latest := ss.GetLatest("file", "/etc/nginx/nginx.conf")
	if latest == nil {
		t.Fatal("expected to get snapshot")
	}

	if latest.StateID != snapshot.StateID {
		t.Error("snapshot mismatch")
	}
}

func TestSnapshotStore_GetPrevious(t *testing.T) {
	ss := NewSnapshotStore(10)

	ss.Save(&StateSnapshot{
		StateID: "test",
		Module:  "file",
		State:   "v1",
	})
	ss.Save(&StateSnapshot{
		StateID: "test",
		Module:  "file",
		State:   "v2",
	})

	prev := ss.GetPrevious("file", "test")
	if prev == nil {
		t.Fatal("expected previous snapshot")
	}

	if prev.State != "v1" {
		t.Errorf("expected v1, got %v", prev.State)
	}

	latest := ss.GetLatest("file", "test")
	if latest.State != "v2" {
		t.Errorf("expected v2, got %v", latest.State)
	}
}

func TestSnapshotStore_MaxSnapshots(t *testing.T) {
	ss := NewSnapshotStore(3)

	for i := 0; i < 5; i++ {
		ss.Save(&StateSnapshot{
			StateID: "test",
			Module:  "file",
			State:   i,
		})
	}

	history := ss.GetHistory("file", "test")
	if len(history) != 3 {
		t.Errorf("expected 3 snapshots, got %d", len(history))
	}

	// Should have the last 3: 2, 3, 4
	if history[0].State != 2 {
		t.Errorf("expected first snapshot to be 2, got %v", history[0].State)
	}
	if history[2].State != 4 {
		t.Errorf("expected last snapshot to be 4, got %v", history[2].State)
	}
}

func TestSnapshotStore_Clear(t *testing.T) {
	ss := NewSnapshotStore(10)

	ss.Save(&StateSnapshot{StateID: "test1", Module: "file"})
	ss.Save(&StateSnapshot{StateID: "test2", Module: "file"})

	ss.Clear()

	if ss.GetLatest("file", "test1") != nil {
		t.Error("expected nil after clear")
	}
}

func TestSnapshotStore_ClearForState(t *testing.T) {
	ss := NewSnapshotStore(10)

	ss.Save(&StateSnapshot{StateID: "test1", Module: "file"})
	ss.Save(&StateSnapshot{StateID: "test2", Module: "file"})

	ss.ClearForState("file", "test1")

	if ss.GetLatest("file", "test1") != nil {
		t.Error("expected test1 to be cleared")
	}

	if ss.GetLatest("file", "test2") == nil {
		t.Error("expected test2 to remain")
	}
}

func TestNewRollbackBuilder(t *testing.T) {
	rb := NewRollbackBuilder()
	if rb == nil {
		t.Fatal("expected non-nil builder")
	}
}

func TestRollbackBuilder_CompositeRollback(t *testing.T) {
	rb := NewRollbackBuilder()

	var order []int

	r1 := func(ctx context.Context) error {
		order = append(order, 1)
		return nil
	}
	r2 := func(ctx context.Context) error {
		order = append(order, 2)
		return nil
	}
	r3 := func(ctx context.Context) error {
		order = append(order, 3)
		return nil
	}

	composite := rb.CompositeRollback(r1, r2, r3)
	err := composite(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should execute in reverse order: 3, 2, 1
	expected := []int{3, 2, 1}
	if len(order) != len(expected) {
		t.Fatalf("expected %d executions, got %d", len(expected), len(order))
	}

	for i, v := range expected {
		if order[i] != v {
			t.Errorf("position %d: expected %d, got %d", i, v, order[i])
		}
	}
}

func TestRollbackBuilder_CompositeRollback_WithNil(t *testing.T) {
	rb := NewRollbackBuilder()

	executed := false
	r1 := func(ctx context.Context) error {
		executed = true
		return nil
	}

	composite := rb.CompositeRollback(r1, nil, nil)
	err := composite(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !executed {
		t.Error("expected r1 to be executed")
	}
}

func TestNewTransactionalExecutor(t *testing.T) {
	te := NewTransactionalExecutor(nil)
	if te == nil {
		t.Fatal("expected non-nil executor")
	}

	if te.executor == nil {
		t.Error("expected non-nil internal executor")
	}

	if te.txnManager == nil {
		t.Error("expected non-nil transaction manager")
	}
}

func TestTransactionManager_GetTransaction(t *testing.T) {
	tm := NewTransactionManager(nil)
	ctx := context.Background()

	txn, _ := tm.Begin(ctx)

	retrieved := tm.GetTransaction(txn.ID)
	if retrieved == nil {
		t.Fatal("expected to retrieve transaction")
	}

	if retrieved.ID != txn.ID {
		t.Error("transaction ID mismatch")
	}

	// Non-existent transaction
	if tm.GetTransaction("non-existent") != nil {
		t.Error("expected nil for non-existent transaction")
	}
}

func TestTransactionManager_OnCommitCallback(t *testing.T) {
	var commitCalled bool
	tm := NewTransactionManager(&TransactionConfig{
		OnCommit: func(txn *Transaction) {
			commitCalled = true
		},
	})
	ctx := context.Background()

	tm.Begin(ctx)
	tm.RecordOperation(&TransactionOperation{ID: "op1", Success: true}, nil)
	tm.Commit(ctx)

	if !commitCalled {
		t.Error("expected OnCommit callback to be called")
	}
}

func TestSnapshotStore_GetPrevious_NoSnapshots(t *testing.T) {
	ss := NewSnapshotStore(10)

	prev := ss.GetPrevious("file", "nonexistent")
	if prev != nil {
		t.Error("expected nil for nonexistent state")
	}
}

func TestSnapshotStore_GetPrevious_SingleSnapshot(t *testing.T) {
	ss := NewSnapshotStore(10)

	ss.Save(&StateSnapshot{StateID: "test", Module: "file"})

	prev := ss.GetPrevious("file", "test")
	if prev != nil {
		t.Error("expected nil when only one snapshot exists")
	}
}
