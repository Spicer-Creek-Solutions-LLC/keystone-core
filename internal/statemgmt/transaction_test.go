package statemgmt

import (
	"context"
	"errors"
	"strings"
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
	tm.RecordOperation(ctx, &TransactionOperation{
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
	tm.RecordOperation(ctx, &TransactionOperation{
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
	tm.RecordOperation(ctx, &TransactionOperation{
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

	tm.RecordOperation(ctx, &TransactionOperation{ID: "op1", Success: true}, func(ctx context.Context) error {
		order = append(order, 1)
		return nil
	})
	tm.RecordOperation(ctx, &TransactionOperation{ID: "op2", Success: true}, func(ctx context.Context) error {
		order = append(order, 2)
		return nil
	})
	tm.RecordOperation(ctx, &TransactionOperation{ID: "op3", Success: true}, func(ctx context.Context) error {
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
	tm.RecordOperation(ctx, &TransactionOperation{ID: "op1", Success: true}, func(ctx context.Context) error {
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

func TestTransactionManager_RollbackWithRetry_ContextCancel(t *testing.T) {
	config := &TransactionConfig{
		MaxRollbackRetries:       3,
		RollbackOnPartialFailure: false,
	}

	tm := NewTransactionManager(config)
	ctx := context.Background()

	// Begin transaction
	_, err := tm.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	rollbackCalls := 0
	err = tm.RecordOperation(ctx, &TransactionOperation{
		ID:      "op-1",
		Type:    OperationTypeUpdate,
		Success: false,
		Error:   errors.New("rollback failed"),
	}, func(ctx context.Context) error {
		rollbackCalls++
		return errors.New("rollback failed")
	})
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()

	err = tm.Rollback(cancelCtx, errors.New("trigger rollback"))
	if err == nil {
		t.Fatal("expected error from canceled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if rollbackCalls == 0 {
		t.Fatal("expected rollback to be attempted at least once")
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
	err := tm.RecordOperation(ctx, &TransactionOperation{ID: "op1"}, nil)
	if err == nil {
		t.Error("expected error for record without transaction")
	}

	tm.Begin(ctx)

	err = tm.RecordOperation(ctx, &TransactionOperation{
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
	tm.RecordOperation(ctx, &TransactionOperation{ID: "op1", Success: true}, func(ctx context.Context) error { return nil })
	tm.RecordOperation(ctx, &TransactionOperation{ID: "op2", Success: true}, func(ctx context.Context) error { return nil })

	// Create another savepoint
	sp2, err := tm.CreateSavepoint("sp2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sp2.Index != 2 {
		t.Errorf("expected index 2, got %d", sp2.Index)
	}

	// Add more operations
	tm.RecordOperation(ctx, &TransactionOperation{ID: "op3", Success: true}, func(ctx context.Context) error { return nil })

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

	tm.RecordOperation(ctx, &TransactionOperation{ID: "op1", Success: true}, nil)
	tm.RecordOperation(ctx, &TransactionOperation{ID: "op2", Success: true}, nil)
	tm.RecordOperation(ctx, &TransactionOperation{ID: "op3", Success: false, Error: errors.New("failed")}, nil)

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
	tm.RecordOperation(ctx, &TransactionOperation{ID: "op1", Success: true}, nil)
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

func TestRollbackBuilder_FileRollback(t *testing.T) {
	rb := NewRollbackBuilder()

	tests := []struct {
		name            string
		path            string
		previousContent []byte
		previousExists  bool
	}{
		{
			name:            "file previously existed",
			path:            "/etc/nginx/nginx.conf",
			previousContent: []byte("previous content"),
			previousExists:  true,
		},
		{
			name:            "file did not exist",
			path:            "/tmp/newfile.txt",
			previousContent: nil,
			previousExists:  false,
		},
		{
			name:            "file with empty content",
			path:            "/etc/empty.conf",
			previousContent: []byte{},
			previousExists:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rollback := rb.FileRollback(tt.path, tt.previousContent, tt.previousExists)
			if rollback == nil {
				t.Fatal("expected non-nil rollback function")
			}

			// Execute the rollback - it's a placeholder so it should not error
			err := rollback(context.Background())
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestRollbackBuilder_PackageRollback(t *testing.T) {
	rb := NewRollbackBuilder()

	tests := []struct {
		name            string
		pkgName         string
		previousVersion string
		wasInstalled    bool
	}{
		{
			name:            "package was installed",
			pkgName:         "nginx",
			previousVersion: "1.18.0",
			wasInstalled:    true,
		},
		{
			name:            "package was not installed",
			pkgName:         "new-package",
			previousVersion: "",
			wasInstalled:    false,
		},
		{
			name:            "package with specific version",
			pkgName:         "python3",
			previousVersion: "3.9.5",
			wasInstalled:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rollback := rb.PackageRollback(tt.pkgName, tt.previousVersion, tt.wasInstalled)
			if rollback == nil {
				t.Fatal("expected non-nil rollback function")
			}

			// Execute the rollback - it's a placeholder so it should not error
			err := rollback(context.Background())
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestRollbackBuilder_ServiceRollback(t *testing.T) {
	rb := NewRollbackBuilder()

	tests := []struct {
		name            string
		serviceName     string
		previousState   string
		previousEnabled bool
	}{
		{
			name:            "service was running and enabled",
			serviceName:     "nginx",
			previousState:   "running",
			previousEnabled: true,
		},
		{
			name:            "service was stopped and disabled",
			serviceName:     "httpd",
			previousState:   "stopped",
			previousEnabled: false,
		},
		{
			name:            "service was running but disabled",
			serviceName:     "cron",
			previousState:   "running",
			previousEnabled: false,
		},
		{
			name:            "service was stopped but enabled",
			serviceName:     "mysql",
			previousState:   "stopped",
			previousEnabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rollback := rb.ServiceRollback(tt.serviceName, tt.previousState, tt.previousEnabled)
			if rollback == nil {
				t.Fatal("expected non-nil rollback function")
			}

			// Execute the rollback - it's a placeholder so it should not error
			err := rollback(context.Background())
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestRollbackBuilder_CompositeRollback_WithErrors(t *testing.T) {
	rb := NewRollbackBuilder()

	tests := []struct {
		name          string
		rollbacks     []RollbackFunc
		expectError   bool
		errorContains string
	}{
		{
			name: "single error",
			rollbacks: []RollbackFunc{
				func(ctx context.Context) error { return nil },
				func(ctx context.Context) error { return errors.New("rollback failed") },
			},
			expectError:   true,
			errorContains: "1 errors",
		},
		{
			name: "multiple errors",
			rollbacks: []RollbackFunc{
				func(ctx context.Context) error { return errors.New("error 1") },
				func(ctx context.Context) error { return errors.New("error 2") },
			},
			expectError:   true,
			errorContains: "2 errors",
		},
		{
			name: "all nil rollbacks",
			rollbacks: []RollbackFunc{
				nil,
				nil,
				nil,
			},
			expectError: false,
		},
		{
			name:        "empty rollbacks",
			rollbacks:   []RollbackFunc{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			composite := rb.CompositeRollback(tt.rollbacks...)
			err := composite(context.Background())

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.expectError && err != nil && tt.errorContains != "" {
				if !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("error message %q should contain %q", err.Error(), tt.errorContains)
				}
			}
		})
	}
}

func TestTransactionStatus_Values(t *testing.T) {
	// Ensure all transaction status constants have expected values
	tests := []struct {
		status   TransactionStatus
		expected string
	}{
		{TransactionStatusPending, "pending"},
		{TransactionStatusActive, "active"},
		{TransactionStatusCommitted, "committed"},
		{TransactionStatusRolledBack, "rolled_back"},
		{TransactionStatusFailed, "failed"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if string(tt.status) != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, string(tt.status))
			}
		})
	}
}

func TestOperationType_Values(t *testing.T) {
	// Ensure all operation type constants have expected values
	tests := []struct {
		opType   OperationType
		expected string
	}{
		{OperationTypeCreate, "create"},
		{OperationTypeUpdate, "update"},
		{OperationTypeDelete, "delete"},
	}

	for _, tt := range tests {
		t.Run(string(tt.opType), func(t *testing.T) {
			if string(tt.opType) != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, string(tt.opType))
			}
		})
	}
}

func TestTransactionOperation_Fields(t *testing.T) {
	op := &TransactionOperation{
		ID:          "op-123",
		Type:        OperationTypeCreate,
		StateID:     "/etc/nginx/nginx.conf",
		Module:      "file",
		Timestamp:   time.Now(),
		Duration:    100 * time.Millisecond,
		Success:     true,
		Error:       nil,
		Changes:     map[string]interface{}{"created": true},
		PriorState:  nil,
		NewState:    "new content",
		CanRollback: true,
	}

	if op.ID != "op-123" {
		t.Errorf("expected ID 'op-123', got %q", op.ID)
	}
	if op.Type != OperationTypeCreate {
		t.Errorf("expected type 'create', got %q", op.Type)
	}
	if op.StateID != "/etc/nginx/nginx.conf" {
		t.Errorf("expected StateID '/etc/nginx/nginx.conf', got %q", op.StateID)
	}
	if op.Module != "file" {
		t.Errorf("expected Module 'file', got %q", op.Module)
	}
	if !op.Success {
		t.Error("expected Success to be true")
	}
	if !op.CanRollback {
		t.Error("expected CanRollback to be true")
	}
}

func TestTransactionalExecutor_GetTransactionManager(t *testing.T) {
	te := NewTransactionalExecutor(nil)

	tm := te.GetTransactionManager()
	if tm == nil {
		t.Fatal("expected non-nil transaction manager")
	}
}

func TestTransaction_GetResult_AfterRollback(t *testing.T) {
	tm := NewTransactionManager(&TransactionConfig{
		RollbackOnPartialFailure: false,
	})
	ctx := context.Background()

	txn, _ := tm.Begin(ctx)

	tm.RecordOperation(ctx, &TransactionOperation{ID: "op1", Success: true}, func(ctx context.Context) error { return nil })
	tm.RecordOperation(ctx, &TransactionOperation{ID: "op2", Success: true}, func(ctx context.Context) error { return nil })

	tm.Rollback(ctx, errors.New("test rollback"))

	result := txn.GetResult()

	if result.Status != TransactionStatusRolledBack {
		t.Errorf("expected rolled_back status, got %s", result.Status)
	}
	if !result.RolledBack {
		t.Error("expected RolledBack to be true")
	}
	if result.OperationCount != 2 {
		t.Errorf("expected 2 operations, got %d", result.OperationCount)
	}
}

func TestTransactionManager_RollbackToSavepoint_NoTransaction(t *testing.T) {
	tm := NewTransactionManager(&TransactionConfig{
		SavepointEnabled: true,
	})
	ctx := context.Background()

	savepoint := &Savepoint{ID: "sp1", Index: 0}
	err := tm.RollbackToSavepoint(ctx, savepoint)
	if err == nil {
		t.Error("expected error for rollback without transaction")
	}
}

func TestTransactionManager_RollbackToSavepoint_InvalidIndex(t *testing.T) {
	tm := NewTransactionManager(&TransactionConfig{
		SavepointEnabled:         true,
		RollbackOnPartialFailure: false,
	})
	ctx := context.Background()

	tm.Begin(ctx)
	tm.RecordOperation(ctx, &TransactionOperation{ID: "op1", Success: true}, nil)

	// Savepoint with index beyond operations count
	savepoint := &Savepoint{ID: "sp1", Index: 10}
	err := tm.RollbackToSavepoint(ctx, savepoint)
	if err == nil {
		t.Error("expected error for invalid savepoint index")
	}
}

func TestTransactionManager_RollbackToSavepoint_WithRollbackError(t *testing.T) {
	tm := NewTransactionManager(&TransactionConfig{
		SavepointEnabled:         true,
		RollbackOnPartialFailure: false,
	})
	ctx := context.Background()

	tm.Begin(ctx)

	// Create savepoint
	sp1, _ := tm.CreateSavepoint("sp1")

	// Add operation with failing rollback
	tm.RecordOperation(ctx, &TransactionOperation{ID: "op1", Success: true}, func(ctx context.Context) error {
		return errors.New("rollback failed")
	})

	err := tm.RollbackToSavepoint(ctx, sp1)
	if err == nil {
		t.Error("expected error from failed rollback")
	}
}

func TestSnapshotStore_GetHistory_Empty(t *testing.T) {
	ss := NewSnapshotStore(10)

	history := ss.GetHistory("file", "nonexistent")
	if len(history) != 0 {
		t.Errorf("expected empty history, got %d items", len(history))
	}
}

func TestSnapshotStore_GetLatest_NotFound(t *testing.T) {
	ss := NewSnapshotStore(10)

	latest := ss.GetLatest("file", "nonexistent")
	if latest != nil {
		t.Error("expected nil for nonexistent state")
	}
}

func TestStateSnapshot_Fields(t *testing.T) {
	now := time.Now()
	snapshot := &StateSnapshot{
		StateID:   "/etc/config.yaml",
		Module:    "file",
		Timestamp: now,
		State:     "yaml content",
		Metadata:  map[string]interface{}{"mode": "0644"},
	}

	if snapshot.StateID != "/etc/config.yaml" {
		t.Errorf("expected StateID '/etc/config.yaml', got %q", snapshot.StateID)
	}
	if snapshot.Module != "file" {
		t.Errorf("expected Module 'file', got %q", snapshot.Module)
	}
	if !snapshot.Timestamp.Equal(now) {
		t.Error("timestamp mismatch")
	}
	if snapshot.State != "yaml content" {
		t.Errorf("expected State 'yaml content', got %q", snapshot.State)
	}
	if snapshot.Metadata["mode"] != "0644" {
		t.Errorf("expected mode '0644', got %v", snapshot.Metadata["mode"])
	}
}

func TestTransactionResult_Fields(t *testing.T) {
	result := &TransactionResult{
		TransactionID:  "txn-123",
		Status:         TransactionStatusCommitted,
		Duration:       500 * time.Millisecond,
		OperationCount: 5,
		SuccessCount:   4,
		FailureCount:   1,
		RolledBack:     false,
		RollbackErrors: nil,
	}

	if result.TransactionID != "txn-123" {
		t.Errorf("expected TransactionID 'txn-123', got %q", result.TransactionID)
	}
	if result.Status != TransactionStatusCommitted {
		t.Errorf("expected status committed, got %s", result.Status)
	}
	if result.OperationCount != 5 {
		t.Errorf("expected 5 operations, got %d", result.OperationCount)
	}
	if result.SuccessCount != 4 {
		t.Errorf("expected 4 successes, got %d", result.SuccessCount)
	}
	if result.FailureCount != 1 {
		t.Errorf("expected 1 failure, got %d", result.FailureCount)
	}
	if result.RolledBack {
		t.Error("expected RolledBack to be false")
	}
}

func TestSavepoint_Fields(t *testing.T) {
	now := time.Now()
	savepoint := &Savepoint{
		ID:        "sp-before-changes",
		Index:     3,
		Timestamp: now,
	}

	if savepoint.ID != "sp-before-changes" {
		t.Errorf("expected ID 'sp-before-changes', got %q", savepoint.ID)
	}
	if savepoint.Index != 3 {
		t.Errorf("expected Index 3, got %d", savepoint.Index)
	}
	if !savepoint.Timestamp.Equal(now) {
		t.Error("timestamp mismatch")
	}
}

func TestTransactionConfig_Callbacks(t *testing.T) {
	var (
		opCompleteCalled int
		rollbackCalled   bool
		commitCalled     bool
	)

	config := &TransactionConfig{
		RollbackOnPartialFailure: false,
		OnOperationComplete: func(op *TransactionOperation) {
			opCompleteCalled++
		},
		OnRollback: func(txn *Transaction, err error) {
			rollbackCalled = true
		},
		OnCommit: func(txn *Transaction) {
			commitCalled = true
		},
	}

	tm := NewTransactionManager(config)
	ctx := context.Background()

	// Test OnOperationComplete
	tm.Begin(ctx)
	tm.RecordOperation(ctx, &TransactionOperation{ID: "op1", Success: true}, nil)
	tm.RecordOperation(ctx, &TransactionOperation{ID: "op2", Success: true}, nil)

	if opCompleteCalled != 2 {
		t.Errorf("expected OnOperationComplete called 2 times, got %d", opCompleteCalled)
	}

	// Test OnCommit
	tm.Commit(ctx)
	if !commitCalled {
		t.Error("expected OnCommit to be called")
	}

	// Start new transaction for rollback test
	commitCalled = false
	tm.Begin(ctx)
	tm.RecordOperation(ctx, &TransactionOperation{ID: "op3", Success: true}, nil)

	// Test OnRollback
	tm.Rollback(ctx, errors.New("test"))
	if !rollbackCalled {
		t.Error("expected OnRollback to be called")
	}
}
