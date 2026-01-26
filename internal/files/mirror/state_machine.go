package mirror

import (
	"context"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

// SyncOperationEvent represents events that trigger sync operation state transitions.
//
// State diagram:
//
// ```mermaid
// stateDiagram-v2
//
//	[*] --> Pending
//	Pending --> InProgress: Start
//	Pending --> Cancelled: Cancel
//	InProgress --> Completed: Complete
//	InProgress --> Failed: Fail
//	InProgress --> Cancelled: Cancel
//	Completed --> [*]
//	Failed --> [*]
//	Cancelled --> [*]
//
// ```
type SyncOperationEvent string

const (
	// SyncOpEventStart starts the sync operation
	SyncOpEventStart SyncOperationEvent = "start"
	// SyncOpEventComplete marks sync as completed
	SyncOpEventComplete SyncOperationEvent = "complete"
	// SyncOpEventFail marks sync as failed
	SyncOpEventFail SyncOperationEvent = "fail"
	// SyncOpEventCancel cancels the sync
	SyncOpEventCancel SyncOperationEvent = "cancel"
)

// FileSyncState represents the state of a single file sync.
//
// State diagram:
//
// ```mermaid
// stateDiagram-v2
//
//	[*] --> Pending
//	Pending --> Syncing: Start
//	Pending --> Skipped: Skip
//	Syncing --> Completed: Complete
//	Syncing --> Failed: Fail
//	Syncing --> Conflict: DetectConflict
//	Conflict --> Resolved: Resolve
//	Conflict --> Failed: FailResolution
//	Resolved --> [*]
//	Completed --> [*]
//	Failed --> [*]
//	Skipped --> [*]
//
// ```
type FileSyncState string

const (
	// FileSyncStatePending indicates file is waiting to sync
	FileSyncStatePending FileSyncState = "pending"
	// FileSyncStateSyncing indicates file is being synced
	FileSyncStateSyncing FileSyncState = "syncing"
	// FileSyncStateCompleted indicates file sync completed
	FileSyncStateCompleted FileSyncState = "completed"
	// FileSyncStateFailed indicates file sync failed
	FileSyncStateFailed FileSyncState = "failed"
	// FileSyncStateSkipped indicates file was skipped
	FileSyncStateSkipped FileSyncState = "skipped"
	// FileSyncStateConflict indicates file has a conflict
	FileSyncStateConflict FileSyncState = "conflict"
	// FileSyncStateResolved indicates conflict was resolved
	FileSyncStateResolved FileSyncState = "resolved"
)

// FileSyncEvent represents events that trigger file sync state transitions.
type FileSyncEvent string

const (
	// FileSyncEventStart starts syncing the file
	FileSyncEventStart FileSyncEvent = "start"
	// FileSyncEventComplete marks file sync as completed
	FileSyncEventComplete FileSyncEvent = "complete"
	// FileSyncEventFail marks file sync as failed
	FileSyncEventFail FileSyncEvent = "fail"
	// FileSyncEventSkip skips the file
	FileSyncEventSkip FileSyncEvent = "skip"
	// FileSyncEventDetectConflict marks a conflict
	FileSyncEventDetectConflict FileSyncEvent = "detect_conflict"
	// FileSyncEventResolve resolves the conflict
	FileSyncEventResolve FileSyncEvent = "resolve"
	// FileSyncEventFailResolution fails conflict resolution
	FileSyncEventFailResolution FileSyncEvent = "fail_resolution"
)

// SyncOperationCallbacks holds callbacks for sync operation state transitions.
type SyncOperationCallbacks struct {
	// OnStarted is called when sync operation starts
	OnStarted func(operationID string)
	// OnCompleted is called when sync operation completes
	OnCompleted func(operationID string, bytesTransferred int64, filesCompleted int)
	// OnFailed is called when sync operation fails
	OnFailed func(operationID string, err error)
	// OnCancelled is called when sync operation is cancelled
	OnCancelled func(operationID string)
	// OnProgress is called for progress updates
	OnProgress func(operationID string, progress float64, currentFile string)
	// OnFileStarted is called when a file sync starts
	OnFileStarted func(operationID string, path string)
	// OnFileCompleted is called when a file sync completes
	OnFileCompleted func(operationID string, path string, bytesTransferred int64)
	// OnFileFailed is called when a file sync fails
	OnFileFailed func(operationID string, path string, err error)
	// OnConflict is called when a conflict is detected
	OnConflict func(operationID string, path string, conflict *Conflict)
}

// ManagedSyncOperation wraps a SyncOperation with a state machine.
type ManagedSyncOperation struct {
	Operation *SyncOperation
	machine   *statemachine.Machine[SyncStatus, SyncOperationEvent]

	// File tracking
	fileMachines map[string]*ManagedFileSync

	// Tracking
	operationID string
	callbacks   *SyncOperationCallbacks
	startTime   time.Time
	endTime     time.Time
	lastError   error
}

// ManagedFileSync wraps a file sync with a state machine.
type ManagedFileSync struct {
	File    *SyncFile
	machine *statemachine.Machine[FileSyncState, FileSyncEvent]

	// Tracking
	path      string
	startTime time.Time
	endTime   time.Time
	lastError error
	conflict  *Conflict
}

// NewManagedSyncOperation creates a new managed sync operation with state machine.
func NewManagedSyncOperation(op *SyncOperation, callbacks *SyncOperationCallbacks) *ManagedSyncOperation {
	mso := &ManagedSyncOperation{
		Operation:    op,
		operationID:  op.ID,
		callbacks:    callbacks,
		fileMachines: make(map[string]*ManagedFileSync),
	}

	mso.machine = statemachine.New[SyncStatus, SyncOperationEvent](SyncStatusPending).
		WithName("sync-operation-"+op.ID).
		WithHistory(20).
		// From Pending
		AddTransition(SyncStatusPending, SyncOpEventStart, SyncStatusInProgress).
		AddTransition(SyncStatusPending, SyncOpEventCancel, SyncStatusCancelled).
		// From InProgress
		AddTransition(SyncStatusInProgress, SyncOpEventComplete, SyncStatusCompleted).
		AddTransition(SyncStatusInProgress, SyncOpEventFail, SyncStatusFailed).
		AddTransition(SyncStatusInProgress, SyncOpEventCancel, SyncStatusCancelled).
		// Callbacks
		OnEnter(SyncStatusInProgress, func(ctx context.Context, state, from SyncStatus) {
			mso.startTime = time.Now()
			mso.Operation.StartedAt = mso.startTime
			mso.Operation.Status = SyncStatusInProgress
			if mso.callbacks != nil && mso.callbacks.OnStarted != nil {
				mso.callbacks.OnStarted(mso.operationID)
			}
		}).
		OnEnter(SyncStatusCompleted, func(ctx context.Context, state, from SyncStatus) {
			mso.endTime = time.Now()
			mso.Operation.CompletedAt = mso.endTime
			mso.Operation.Status = SyncStatusCompleted
			mso.Operation.Progress = 1.0
			if mso.callbacks != nil && mso.callbacks.OnCompleted != nil {
				mso.callbacks.OnCompleted(mso.operationID, mso.Operation.BytesTransferred, mso.Operation.FilesCompleted)
			}
		}).
		OnEnter(SyncStatusFailed, func(ctx context.Context, state, from SyncStatus) {
			mso.endTime = time.Now()
			mso.Operation.CompletedAt = mso.endTime
			mso.Operation.Status = SyncStatusFailed
			if mso.lastError != nil {
				mso.Operation.Error = mso.lastError.Error()
			}
			if mso.callbacks != nil && mso.callbacks.OnFailed != nil {
				mso.callbacks.OnFailed(mso.operationID, mso.lastError)
			}
		}).
		OnEnter(SyncStatusCancelled, func(ctx context.Context, state, from SyncStatus) {
			mso.endTime = time.Now()
			mso.Operation.CompletedAt = mso.endTime
			mso.Operation.Status = SyncStatusCancelled
			if mso.callbacks != nil && mso.callbacks.OnCancelled != nil {
				mso.callbacks.OnCancelled(mso.operationID)
			}
		}).
		MustBuild()

	return mso
}

// newManagedFileSync creates a new managed file sync with state machine.
func newManagedFileSync(file *SyncFile) *ManagedFileSync {
	mfs := &ManagedFileSync{
		File: file,
		path: file.Path,
	}

	mfs.machine = statemachine.New[FileSyncState, FileSyncEvent](FileSyncStatePending).
		WithName("file-sync-"+file.Path).
		WithHistory(10).
		// From Pending
		AddTransition(FileSyncStatePending, FileSyncEventStart, FileSyncStateSyncing).
		AddTransition(FileSyncStatePending, FileSyncEventSkip, FileSyncStateSkipped).
		// From Syncing
		AddTransition(FileSyncStateSyncing, FileSyncEventComplete, FileSyncStateCompleted).
		AddTransition(FileSyncStateSyncing, FileSyncEventFail, FileSyncStateFailed).
		AddTransition(FileSyncStateSyncing, FileSyncEventDetectConflict, FileSyncStateConflict).
		// From Conflict
		AddTransition(FileSyncStateConflict, FileSyncEventResolve, FileSyncStateResolved).
		AddTransition(FileSyncStateConflict, FileSyncEventFailResolution, FileSyncStateFailed).
		// Callbacks
		OnEnter(FileSyncStateSyncing, func(ctx context.Context, state, from FileSyncState) {
			mfs.startTime = time.Now()
		}).
		OnEnter(FileSyncStateCompleted, func(ctx context.Context, state, from FileSyncState) {
			mfs.endTime = time.Now()
		}).
		OnEnter(FileSyncStateFailed, func(ctx context.Context, state, from FileSyncState) {
			mfs.endTime = time.Now()
		}).
		OnEnter(FileSyncStateSkipped, func(ctx context.Context, state, from FileSyncState) {
			mfs.endTime = time.Now()
		}).
		MustBuild()

	return mfs
}

// Status returns the current sync operation status.
func (mso *ManagedSyncOperation) Status() SyncStatus {
	return mso.machine.State()
}

// Start starts the sync operation.
func (mso *ManagedSyncOperation) Start() error {
	return mso.machine.Fire(SyncOpEventStart)
}

// Complete marks the sync operation as completed.
func (mso *ManagedSyncOperation) Complete() error {
	return mso.machine.Fire(SyncOpEventComplete)
}

// Fail marks the sync operation as failed.
func (mso *ManagedSyncOperation) Fail(err error) error {
	mso.lastError = err
	return mso.machine.Fire(SyncOpEventFail)
}

// Cancel cancels the sync operation.
func (mso *ManagedSyncOperation) Cancel() error {
	return mso.machine.Fire(SyncOpEventCancel)
}

// AddFile adds a file to track.
func (mso *ManagedSyncOperation) AddFile(file *SyncFile) *ManagedFileSync {
	mfs := newManagedFileSync(file)
	mso.fileMachines[file.Path] = mfs
	return mfs
}

// GetFile returns the managed file sync for a path.
func (mso *ManagedSyncOperation) GetFile(path string) *ManagedFileSync {
	return mso.fileMachines[path]
}

// StartFile starts syncing a file.
func (mso *ManagedSyncOperation) StartFile(path string) error {
	mfs := mso.fileMachines[path]
	if mfs == nil {
		return nil
	}
	err := mfs.Start()
	if err == nil && mso.callbacks != nil && mso.callbacks.OnFileStarted != nil {
		mso.callbacks.OnFileStarted(mso.operationID, path)
	}
	return err
}

// CompleteFile marks a file sync as completed.
func (mso *ManagedSyncOperation) CompleteFile(path string, bytesTransferred int64) error {
	mfs := mso.fileMachines[path]
	if mfs == nil {
		return nil
	}
	err := mfs.Complete()
	if err == nil {
		mso.Operation.FilesCompleted++
		mso.Operation.BytesTransferred += bytesTransferred
		mso.updateProgress()
		if mso.callbacks != nil && mso.callbacks.OnFileCompleted != nil {
			mso.callbacks.OnFileCompleted(mso.operationID, path, bytesTransferred)
		}
	}
	return err
}

// FailFile marks a file sync as failed.
func (mso *ManagedSyncOperation) FailFile(path string, err error) error {
	mfs := mso.fileMachines[path]
	if mfs == nil {
		return nil
	}
	stateErr := mfs.Fail(err)
	if stateErr == nil {
		mso.Operation.FilesFailed++
		mso.updateProgress()
		if mso.callbacks != nil && mso.callbacks.OnFileFailed != nil {
			mso.callbacks.OnFileFailed(mso.operationID, path, err)
		}
	}
	return stateErr
}

// SkipFile marks a file as skipped.
func (mso *ManagedSyncOperation) SkipFile(path string) error {
	mfs := mso.fileMachines[path]
	if mfs == nil {
		return nil
	}
	return mfs.Skip()
}

// DetectConflict marks a file as having a conflict.
func (mso *ManagedSyncOperation) DetectConflict(path string, conflict *Conflict) error {
	mfs := mso.fileMachines[path]
	if mfs == nil {
		return nil
	}
	err := mfs.DetectConflict(conflict)
	if err == nil && mso.callbacks != nil && mso.callbacks.OnConflict != nil {
		mso.callbacks.OnConflict(mso.operationID, path, conflict)
	}
	return err
}

// ResolveConflict resolves a file conflict.
func (mso *ManagedSyncOperation) ResolveConflict(path string) error {
	mfs := mso.fileMachines[path]
	if mfs == nil {
		return nil
	}
	return mfs.Resolve()
}

// updateProgress updates the operation progress.
func (mso *ManagedSyncOperation) updateProgress() {
	total := len(mso.fileMachines)
	if total == 0 {
		return
	}
	completed := mso.Operation.FilesCompleted + mso.Operation.FilesFailed
	mso.Operation.Progress = float64(completed) / float64(total)

	if mso.callbacks != nil && mso.callbacks.OnProgress != nil {
		// Find current file being processed
		var currentFile string
		for path, mfs := range mso.fileMachines {
			if mfs.IsSyncing() {
				currentFile = path
				break
			}
		}
		mso.callbacks.OnProgress(mso.operationID, mso.Operation.Progress, currentFile)
	}
}

// IsPending returns true if operation is pending.
func (mso *ManagedSyncOperation) IsPending() bool {
	return mso.machine.IsInState(SyncStatusPending)
}

// IsInProgress returns true if operation is in progress.
func (mso *ManagedSyncOperation) IsInProgress() bool {
	return mso.machine.IsInState(SyncStatusInProgress)
}

// IsCompleted returns true if operation completed.
func (mso *ManagedSyncOperation) IsCompleted() bool {
	return mso.machine.IsInState(SyncStatusCompleted)
}

// IsTerminal returns true if operation is in a terminal state.
func (mso *ManagedSyncOperation) IsTerminal() bool {
	return mso.machine.IsInAnyState(
		SyncStatusCompleted,
		SyncStatusFailed,
		SyncStatusCancelled,
	)
}

// IsSuccessful returns true if operation completed successfully.
func (mso *ManagedSyncOperation) IsSuccessful() bool {
	return mso.machine.IsInState(SyncStatusCompleted)
}

// Duration returns the operation duration.
func (mso *ManagedSyncOperation) Duration() time.Duration {
	if mso.startTime.IsZero() {
		return 0
	}
	if mso.endTime.IsZero() {
		return time.Since(mso.startTime)
	}
	return mso.endTime.Sub(mso.startTime)
}

// FileCount returns the total number of files being synced.
func (mso *ManagedSyncOperation) FileCount() int {
	return len(mso.fileMachines)
}

// CompletedFiles returns the number of completed files.
func (mso *ManagedSyncOperation) CompletedFiles() int {
	count := 0
	for _, mfs := range mso.fileMachines {
		if mfs.IsCompleted() || mfs.IsResolved() {
			count++
		}
	}
	return count
}

// FailedFiles returns the number of failed files.
func (mso *ManagedSyncOperation) FailedFiles() int {
	count := 0
	for _, mfs := range mso.fileMachines {
		if mfs.IsFailed() {
			count++
		}
	}
	return count
}

// ConflictedFiles returns the number of files with conflicts.
func (mso *ManagedSyncOperation) ConflictedFiles() int {
	count := 0
	for _, mfs := range mso.fileMachines {
		if mfs.IsConflict() {
			count++
		}
	}
	return count
}

// History returns the state transition history.
func (mso *ManagedSyncOperation) History() *statemachine.History[SyncStatus, SyncOperationEvent] {
	return mso.machine.History()
}

// AvailableEvents returns events that can be fired from the current state.
func (mso *ManagedSyncOperation) AvailableEvents() []SyncOperationEvent {
	return mso.machine.AvailableEvents()
}

// File sync state machine methods

// State returns the current file sync state.
func (mfs *ManagedFileSync) State() FileSyncState {
	return mfs.machine.State()
}

// Start starts syncing the file.
func (mfs *ManagedFileSync) Start() error {
	return mfs.machine.Fire(FileSyncEventStart)
}

// Complete marks the file sync as completed.
func (mfs *ManagedFileSync) Complete() error {
	return mfs.machine.Fire(FileSyncEventComplete)
}

// Fail marks the file sync as failed.
func (mfs *ManagedFileSync) Fail(err error) error {
	mfs.lastError = err
	return mfs.machine.Fire(FileSyncEventFail)
}

// Skip marks the file as skipped.
func (mfs *ManagedFileSync) Skip() error {
	return mfs.machine.Fire(FileSyncEventSkip)
}

// DetectConflict marks a conflict on this file.
func (mfs *ManagedFileSync) DetectConflict(conflict *Conflict) error {
	mfs.conflict = conflict
	return mfs.machine.Fire(FileSyncEventDetectConflict)
}

// Resolve resolves the conflict.
func (mfs *ManagedFileSync) Resolve() error {
	return mfs.machine.Fire(FileSyncEventResolve)
}

// FailResolution fails conflict resolution.
func (mfs *ManagedFileSync) FailResolution(err error) error {
	mfs.lastError = err
	return mfs.machine.Fire(FileSyncEventFailResolution)
}

// IsPending returns true if file sync is pending.
func (mfs *ManagedFileSync) IsPending() bool {
	return mfs.machine.IsInState(FileSyncStatePending)
}

// IsSyncing returns true if file is being synced.
func (mfs *ManagedFileSync) IsSyncing() bool {
	return mfs.machine.IsInState(FileSyncStateSyncing)
}

// IsCompleted returns true if file sync completed.
func (mfs *ManagedFileSync) IsCompleted() bool {
	return mfs.machine.IsInState(FileSyncStateCompleted)
}

// IsFailed returns true if file sync failed.
func (mfs *ManagedFileSync) IsFailed() bool {
	return mfs.machine.IsInState(FileSyncStateFailed)
}

// IsSkipped returns true if file was skipped.
func (mfs *ManagedFileSync) IsSkipped() bool {
	return mfs.machine.IsInState(FileSyncStateSkipped)
}

// IsConflict returns true if file has a conflict.
func (mfs *ManagedFileSync) IsConflict() bool {
	return mfs.machine.IsInState(FileSyncStateConflict)
}

// IsResolved returns true if conflict was resolved.
func (mfs *ManagedFileSync) IsResolved() bool {
	return mfs.machine.IsInState(FileSyncStateResolved)
}

// IsTerminal returns true if file sync is in a terminal state.
func (mfs *ManagedFileSync) IsTerminal() bool {
	return mfs.machine.IsInAnyState(
		FileSyncStateCompleted,
		FileSyncStateFailed,
		FileSyncStateSkipped,
		FileSyncStateResolved,
	)
}

// Duration returns the file sync duration.
func (mfs *ManagedFileSync) Duration() time.Duration {
	if mfs.startTime.IsZero() {
		return 0
	}
	if mfs.endTime.IsZero() {
		return time.Since(mfs.startTime)
	}
	return mfs.endTime.Sub(mfs.startTime)
}

// SyncStatusToString returns a human-readable name for the sync status.
func SyncStatusToString(status SyncStatus) string {
	switch status {
	case SyncStatusPending:
		return "Pending"
	case SyncStatusInProgress:
		return "In Progress"
	case SyncStatusCompleted:
		return "Completed"
	case SyncStatusFailed:
		return "Failed"
	case SyncStatusCancelled:
		return "Cancelled"
	default:
		return string(status)
	}
}

// FileSyncStateToString returns a human-readable name for the file sync state.
func FileSyncStateToString(state FileSyncState) string {
	switch state {
	case FileSyncStatePending:
		return "Pending"
	case FileSyncStateSyncing:
		return "Syncing"
	case FileSyncStateCompleted:
		return "Completed"
	case FileSyncStateFailed:
		return "Failed"
	case FileSyncStateSkipped:
		return "Skipped"
	case FileSyncStateConflict:
		return "Conflict"
	case FileSyncStateResolved:
		return "Resolved"
	default:
		return string(state)
	}
}
