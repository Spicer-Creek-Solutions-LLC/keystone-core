package statemgmt

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/wait"
)

// Transaction represents an atomic state transaction that can be rolled back
type Transaction struct {
	ID        string
	StartTime time.Time
	EndTime   time.Time
	Status    TransactionStatus

	// Operations performed in this transaction
	operations []*TransactionOperation

	// Rollback functions for each operation (in reverse order)
	rollbacks []RollbackFunc

	// Configuration
	config *TransactionConfig

	mu sync.Mutex
}

// TransactionStatus represents the status of a transaction
type TransactionStatus string

const (
	TransactionStatusPending    TransactionStatus = "pending"
	TransactionStatusActive     TransactionStatus = "active"
	TransactionStatusCommitted  TransactionStatus = "committed"
	TransactionStatusRolledBack TransactionStatus = "rolled_back"
	TransactionStatusFailed     TransactionStatus = "failed"
)

// TransactionOperation represents a single operation within a transaction
type TransactionOperation struct {
	ID          string
	Type        OperationType
	StateID     string
	Module      string
	Timestamp   time.Time
	Duration    time.Duration
	Success     bool
	Error       error
	Changes     map[string]interface{}
	PriorState  interface{} // State before the operation
	NewState    interface{} // State after the operation
	CanRollback bool
}

// OperationType represents the type of state operation
type OperationType string

const (
	OperationTypeCreate OperationType = "create"
	OperationTypeUpdate OperationType = "update"
	OperationTypeDelete OperationType = "delete"
)

// RollbackFunc is a function that undoes an operation
type RollbackFunc func(ctx context.Context) error

// TransactionConfig configures transaction behavior
type TransactionConfig struct {
	// Timeout for the entire transaction
	Timeout time.Duration

	// MaxRetries for rollback attempts
	MaxRollbackRetries int

	// RollbackOnPartialFailure triggers rollback if any operation fails
	RollbackOnPartialFailure bool

	// ContinueOnRollbackError continues rolling back other operations if one fails
	ContinueOnRollbackError bool

	// SavepointEnabled enables savepoints within the transaction
	SavepointEnabled bool

	// OnOperationComplete callback for each completed operation
	OnOperationComplete func(*TransactionOperation)

	// OnRollback callback when rollback starts
	OnRollback func(*Transaction, error)

	// OnCommit callback when transaction commits
	OnCommit func(*Transaction)
}

// DefaultTransactionConfig returns sensible defaults
func DefaultTransactionConfig() *TransactionConfig {
	return &TransactionConfig{
		Timeout:                  5 * time.Minute,
		MaxRollbackRetries:       3,
		RollbackOnPartialFailure: true,
		ContinueOnRollbackError:  true,
		SavepointEnabled:         true,
	}
}

// TransactionManager manages state transactions
type TransactionManager struct {
	config *TransactionConfig

	mu           sync.RWMutex
	transactions map[string]*Transaction
	active       *Transaction
}

// NewTransactionManager creates a new transaction manager
func NewTransactionManager(config *TransactionConfig) *TransactionManager {
	if config == nil {
		config = DefaultTransactionConfig()
	}

	return &TransactionManager{
		config:       config,
		transactions: make(map[string]*Transaction),
	}
}

// Begin starts a new transaction
func (tm *TransactionManager) Begin(ctx context.Context) (*Transaction, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.active != nil {
		return nil, fmt.Errorf("transaction already in progress: %s", tm.active.ID)
	}

	txn := &Transaction{
		ID:         fmt.Sprintf("txn-%d", time.Now().UnixNano()),
		StartTime:  time.Now(),
		Status:     TransactionStatusActive,
		operations: make([]*TransactionOperation, 0),
		rollbacks:  make([]RollbackFunc, 0),
		config:     tm.config,
	}

	tm.transactions[txn.ID] = txn
	tm.active = txn

	return txn, nil
}

// GetActive returns the currently active transaction
func (tm *TransactionManager) GetActive() *Transaction {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.active
}

// GetTransaction returns a transaction by ID
func (tm *TransactionManager) GetTransaction(id string) *Transaction {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.transactions[id]
}

// RecordOperation records an operation in the active transaction
func (tm *TransactionManager) RecordOperation(op *TransactionOperation, rollback RollbackFunc) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.active == nil {
		return fmt.Errorf("no active transaction")
	}

	tm.active.mu.Lock()
	defer tm.active.mu.Unlock()

	op.Timestamp = time.Now()
	tm.active.operations = append(tm.active.operations, op)

	if rollback != nil {
		op.CanRollback = true
		tm.active.rollbacks = append(tm.active.rollbacks, rollback)
	}

	if tm.config.OnOperationComplete != nil {
		tm.config.OnOperationComplete(op)
	}

	// Check for failure and auto-rollback
	if !op.Success && tm.config.RollbackOnPartialFailure {
		// Trigger rollback asynchronously
		go func() {
			tm.Rollback(context.Background(), op.Error)
		}()
	}

	return nil
}

// Commit commits the active transaction
func (tm *TransactionManager) Commit(ctx context.Context) error {
	tm.mu.Lock()
	txn := tm.active
	tm.mu.Unlock()

	if txn == nil {
		return fmt.Errorf("no active transaction to commit")
	}

	txn.mu.Lock()
	defer txn.mu.Unlock()

	// Check all operations succeeded
	for _, op := range txn.operations {
		if !op.Success {
			return fmt.Errorf("cannot commit: operation %s failed: %v", op.ID, op.Error)
		}
	}

	txn.Status = TransactionStatusCommitted
	txn.EndTime = time.Now()

	if tm.config.OnCommit != nil {
		tm.config.OnCommit(txn)
	}

	tm.mu.Lock()
	tm.active = nil
	tm.mu.Unlock()

	return nil
}

// Rollback rolls back the active transaction
func (tm *TransactionManager) Rollback(ctx context.Context, reason error) error {
	tm.mu.Lock()
	txn := tm.active
	tm.mu.Unlock()

	if txn == nil {
		return fmt.Errorf("no active transaction to rollback")
	}

	txn.mu.Lock()
	defer txn.mu.Unlock()

	if tm.config.OnRollback != nil {
		tm.config.OnRollback(txn, reason)
	}

	// Execute rollbacks in reverse order
	var rollbackErrors []error
	maxRetries := tm.config.MaxRollbackRetries
	if maxRetries <= 0 {
		maxRetries = 1 // At least one attempt
	}

	for i := len(txn.rollbacks) - 1; i >= 0; i-- {
		rollback := txn.rollbacks[i]
		if rollback == nil {
			continue
		}

		var err error
		for attempt := 0; attempt < maxRetries; attempt++ {
			err = rollback(ctx)
			if err == nil {
				break
			}
			// Brief delay before retry
			if attempt < maxRetries-1 {
				if err := waitForRetry(ctx, time.Duration(attempt+1)*100*time.Millisecond); err != nil {
					return err
				}
			}
		}

		if err != nil {
			rollbackErrors = append(rollbackErrors, err)
			if !tm.config.ContinueOnRollbackError {
				break
			}
		}
	}

	if len(rollbackErrors) > 0 {
		txn.Status = TransactionStatusFailed
	} else {
		txn.Status = TransactionStatusRolledBack
	}
	txn.EndTime = time.Now()

	tm.mu.Lock()
	tm.active = nil
	tm.mu.Unlock()

	if len(rollbackErrors) > 0 {
		return fmt.Errorf("rollback completed with %d errors: %v", len(rollbackErrors), rollbackErrors)
	}

	return nil
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	return wait.ForContext(ctx, delay)
}

// Savepoint creates a savepoint within the transaction
type Savepoint struct {
	ID        string
	Index     int // Index in operations slice
	Timestamp time.Time
}

// CreateSavepoint creates a savepoint at the current position
func (tm *TransactionManager) CreateSavepoint(name string) (*Savepoint, error) {
	tm.mu.RLock()
	txn := tm.active
	tm.mu.RUnlock()

	if txn == nil {
		return nil, fmt.Errorf("no active transaction")
	}

	if !tm.config.SavepointEnabled {
		return nil, fmt.Errorf("savepoints are not enabled")
	}

	txn.mu.Lock()
	defer txn.mu.Unlock()

	savepoint := &Savepoint{
		ID:        name,
		Index:     len(txn.operations),
		Timestamp: time.Now(),
	}

	return savepoint, nil
}

// RollbackToSavepoint rolls back to a specific savepoint
func (tm *TransactionManager) RollbackToSavepoint(ctx context.Context, savepoint *Savepoint) error {
	tm.mu.RLock()
	txn := tm.active
	tm.mu.RUnlock()

	if txn == nil {
		return fmt.Errorf("no active transaction")
	}

	txn.mu.Lock()
	defer txn.mu.Unlock()

	if savepoint.Index > len(txn.operations) {
		return fmt.Errorf("invalid savepoint: index %d exceeds operations count %d",
			savepoint.Index, len(txn.operations))
	}

	// Execute rollbacks for operations after the savepoint (in reverse order)
	for i := len(txn.rollbacks) - 1; i >= savepoint.Index; i-- {
		rollback := txn.rollbacks[i]
		if rollback != nil {
			if err := rollback(ctx); err != nil {
				return fmt.Errorf("rollback failed at index %d: %w", i, err)
			}
		}
	}

	// Truncate operations and rollbacks to savepoint
	txn.operations = txn.operations[:savepoint.Index]
	txn.rollbacks = txn.rollbacks[:savepoint.Index]

	return nil
}

// TransactionResult represents the result of a transaction
type TransactionResult struct {
	TransactionID  string
	Status         TransactionStatus
	Duration       time.Duration
	OperationCount int
	SuccessCount   int
	FailureCount   int
	RolledBack     bool
	RollbackErrors []error
}

// GetResult returns the result of a transaction
func (t *Transaction) GetResult() *TransactionResult {
	t.mu.Lock()
	defer t.mu.Unlock()

	result := &TransactionResult{
		TransactionID:  t.ID,
		Status:         t.Status,
		Duration:       t.EndTime.Sub(t.StartTime),
		OperationCount: len(t.operations),
		RolledBack:     t.Status == TransactionStatusRolledBack,
	}

	for _, op := range t.operations {
		if op.Success {
			result.SuccessCount++
		} else {
			result.FailureCount++
		}
	}

	return result
}

// TransactionalExecutor wraps the executor with transaction support
type TransactionalExecutor struct {
	executor   *Executor
	txnManager *TransactionManager
}

// NewTransactionalExecutor creates a new transactional executor
func NewTransactionalExecutor(config *TransactionConfig) *TransactionalExecutor {
	return &TransactionalExecutor{
		executor:   NewExecutor(),
		txnManager: NewTransactionManager(config),
	}
}

// ExecuteWithTransaction executes a state file within a transaction
func (te *TransactionalExecutor) ExecuteWithTransaction(ctx context.Context, stateFile *StateFile) (*StateRun, *TransactionResult, error) {
	// Begin transaction
	txn, err := te.txnManager.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Apply timeout
	if te.txnManager.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, te.txnManager.config.Timeout)
		defer cancel()
	}

	// Execute states
	run, err := te.executor.ExecuteState(ctx, stateFile)

	// Record operations from results
	for _, result := range run.Results {
		op := &TransactionOperation{
			ID:       result.StateID,
			StateID:  result.StateID,
			Module:   result.Module,
			Duration: result.Duration,
			Success:  result.Success,
			Error:    result.Error,
			Changes:  result.Changes,
		}

		// Determine operation type from changes
		if result.Changes != nil {
			if _, isNew := result.Changes["created"]; isNew {
				op.Type = OperationTypeCreate
			} else if _, isDeleted := result.Changes["deleted"]; isDeleted {
				op.Type = OperationTypeDelete
			} else {
				op.Type = OperationTypeUpdate
			}
		}

		te.txnManager.RecordOperation(op, nil) // Rollback would need module-specific implementation
	}

	// Commit or rollback based on results
	if err != nil || !run.Summary.Success {
		rollbackErr := te.txnManager.Rollback(ctx, err)
		return run, txn.GetResult(), rollbackErr
	}

	commitErr := te.txnManager.Commit(ctx)
	return run, txn.GetResult(), commitErr
}

// GetTransactionManager returns the transaction manager
func (te *TransactionalExecutor) GetTransactionManager() *TransactionManager {
	return te.txnManager
}

// StateSnapshot represents a point-in-time snapshot of state for rollback
type StateSnapshot struct {
	StateID   string
	Module    string
	Timestamp time.Time
	State     interface{}
	Metadata  map[string]interface{}
}

// SnapshotStore stores state snapshots for rollback
type SnapshotStore struct {
	mu        sync.RWMutex
	snapshots map[string][]*StateSnapshot
	maxPerKey int
}

// NewSnapshotStore creates a new snapshot store
func NewSnapshotStore(maxPerKey int) *SnapshotStore {
	if maxPerKey <= 0 {
		maxPerKey = 10
	}
	return &SnapshotStore{
		snapshots: make(map[string][]*StateSnapshot),
		maxPerKey: maxPerKey,
	}
}

// Save saves a state snapshot
func (ss *SnapshotStore) Save(snapshot *StateSnapshot) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	key := snapshot.Module + ":" + snapshot.StateID
	snapshots := ss.snapshots[key]
	snapshots = append(snapshots, snapshot)

	// Trim to max
	if len(snapshots) > ss.maxPerKey {
		snapshots = snapshots[len(snapshots)-ss.maxPerKey:]
	}

	ss.snapshots[key] = snapshots
}

// GetLatest returns the latest snapshot for a state
func (ss *SnapshotStore) GetLatest(module, stateID string) *StateSnapshot {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	key := module + ":" + stateID
	snapshots := ss.snapshots[key]
	if len(snapshots) == 0 {
		return nil
	}
	return snapshots[len(snapshots)-1]
}

// GetPrevious returns the previous snapshot (before the latest)
func (ss *SnapshotStore) GetPrevious(module, stateID string) *StateSnapshot {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	key := module + ":" + stateID
	snapshots := ss.snapshots[key]
	if len(snapshots) < 2 {
		return nil
	}
	return snapshots[len(snapshots)-2]
}

// GetHistory returns all snapshots for a state
func (ss *SnapshotStore) GetHistory(module, stateID string) []*StateSnapshot {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	key := module + ":" + stateID
	snapshots := ss.snapshots[key]

	// Return a copy
	result := make([]*StateSnapshot, len(snapshots))
	copy(result, snapshots)
	return result
}

// Clear clears all snapshots
func (ss *SnapshotStore) Clear() {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.snapshots = make(map[string][]*StateSnapshot)
}

// ClearForState clears snapshots for a specific state
func (ss *SnapshotStore) ClearForState(module, stateID string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	delete(ss.snapshots, module+":"+stateID)
}

// RollbackBuilder helps build rollback functions for common operations
type RollbackBuilder struct{}

// NewRollbackBuilder creates a new rollback builder
func NewRollbackBuilder() *RollbackBuilder {
	return &RollbackBuilder{}
}

// FileRollback creates a rollback for file operations
func (rb *RollbackBuilder) FileRollback(path string, previousContent []byte, previousExists bool) RollbackFunc {
	return func(ctx context.Context) error {
		// This would need actual file system access
		// Placeholder implementation
		if !previousExists {
			// File didn't exist before, delete it
			return nil
		}
		// Restore previous content
		return nil
	}
}

// PackageRollback creates a rollback for package operations
func (rb *RollbackBuilder) PackageRollback(name string, previousVersion string, wasInstalled bool) RollbackFunc {
	return func(ctx context.Context) error {
		if !wasInstalled {
			// Package wasn't installed, remove it
			return nil
		}
		// Reinstall previous version
		return nil
	}
}

// ServiceRollback creates a rollback for service operations
func (rb *RollbackBuilder) ServiceRollback(name string, previousState string, previousEnabled bool) RollbackFunc {
	return func(ctx context.Context) error {
		// Restore previous service state
		return nil
	}
}

// CompositeRollback combines multiple rollback functions
func (rb *RollbackBuilder) CompositeRollback(rollbacks ...RollbackFunc) RollbackFunc {
	return func(ctx context.Context) error {
		var errors []error
		for i := len(rollbacks) - 1; i >= 0; i-- {
			if rollbacks[i] != nil {
				if err := rollbacks[i](ctx); err != nil {
					errors = append(errors, err)
				}
			}
		}
		if len(errors) > 0 {
			return fmt.Errorf("composite rollback had %d errors", len(errors))
		}
		return nil
	}
}
