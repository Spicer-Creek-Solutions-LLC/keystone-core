package mirror

import (
	"errors"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

func TestManagedSyncOperation_InitialState(t *testing.T) {
	op := &SyncOperation{
		ID:           "test-sync-1",
		GroupID:      "group-1",
		SourceMirror: "mirror-a",
		TargetMirror: "mirror-b",
	}

	mso := NewManagedSyncOperation(op, nil)

	if mso.Status() != SyncStatusPending {
		t.Errorf("expected pending status, got %v", mso.Status())
	}
	if !mso.IsPending() {
		t.Error("expected IsPending() to be true")
	}
	if mso.IsInProgress() {
		t.Error("expected IsInProgress() to be false")
	}
	if mso.IsTerminal() {
		t.Error("expected IsTerminal() to be false")
	}
}

func TestManagedSyncOperation_StartWorkflow(t *testing.T) {
	op := &SyncOperation{
		ID:           "test-sync-1",
		GroupID:      "group-1",
		SourceMirror: "mirror-a",
		TargetMirror: "mirror-b",
	}

	mso := NewManagedSyncOperation(op, nil)

	if err := mso.Start(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mso.Status() != SyncStatusInProgress {
		t.Errorf("expected in_progress status, got %v", mso.Status())
	}
	if !mso.IsInProgress() {
		t.Error("expected IsInProgress() to be true")
	}
	if op.Status != SyncStatusInProgress {
		t.Errorf("expected operation status to be in_progress, got %v", op.Status)
	}
}

func TestManagedSyncOperation_CompleteWorkflow(t *testing.T) {
	op := &SyncOperation{
		ID:           "test-sync-1",
		GroupID:      "group-1",
		SourceMirror: "mirror-a",
		TargetMirror: "mirror-b",
	}

	mso := NewManagedSyncOperation(op, nil)

	mso.Start()

	if err := mso.Complete(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mso.Status() != SyncStatusCompleted {
		t.Errorf("expected completed status, got %v", mso.Status())
	}
	if !mso.IsCompleted() {
		t.Error("expected IsCompleted() to be true")
	}
	if !mso.IsTerminal() {
		t.Error("expected IsTerminal() to be true")
	}
	if !mso.IsSuccessful() {
		t.Error("expected IsSuccessful() to be true")
	}
	if op.Progress != 1.0 {
		t.Errorf("expected progress to be 1.0, got %f", op.Progress)
	}
}

func TestManagedSyncOperation_FailWorkflow(t *testing.T) {
	op := &SyncOperation{
		ID:           "test-sync-1",
		GroupID:      "group-1",
		SourceMirror: "mirror-a",
		TargetMirror: "mirror-b",
	}

	mso := NewManagedSyncOperation(op, nil)

	mso.Start()

	testErr := errors.New("sync failed")
	if err := mso.Fail(testErr); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mso.Status() != SyncStatusFailed {
		t.Errorf("expected failed status, got %v", mso.Status())
	}
	if !mso.IsTerminal() {
		t.Error("expected IsTerminal() to be true")
	}
	if mso.IsSuccessful() {
		t.Error("expected IsSuccessful() to be false")
	}
	if op.Error != "sync failed" {
		t.Errorf("expected error message, got %s", op.Error)
	}
}

func TestManagedSyncOperation_CancelWorkflow(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*ManagedSyncOperation)
	}{
		{"cancel from pending", func(mso *ManagedSyncOperation) {}},
		{"cancel from in_progress", func(mso *ManagedSyncOperation) { mso.Start() }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &SyncOperation{
				ID:           "test-sync-1",
				GroupID:      "group-1",
				SourceMirror: "mirror-a",
				TargetMirror: "mirror-b",
			}

			mso := NewManagedSyncOperation(op, nil)
			tt.setup(mso)

			if err := mso.Cancel(); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if mso.Status() != SyncStatusCancelled {
				t.Errorf("expected cancelled status, got %v", mso.Status())
			}
			if !mso.IsTerminal() {
				t.Error("expected IsTerminal() to be true")
			}
		})
	}
}

func TestManagedSyncOperation_InvalidTransitions(t *testing.T) {
	op := &SyncOperation{
		ID:           "test-sync-1",
		GroupID:      "group-1",
		SourceMirror: "mirror-a",
		TargetMirror: "mirror-b",
	}

	mso := NewManagedSyncOperation(op, nil)

	// Cannot complete from pending
	err := mso.Complete()
	if err == nil {
		t.Error("expected error for invalid transition")
	}
	if !errors.Is(err, statemachine.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}

	// Status should not have changed
	if mso.Status() != SyncStatusPending {
		t.Errorf("status should not have changed, got %v", mso.Status())
	}
}

func TestManagedSyncOperation_FileTracking(t *testing.T) {
	op := &SyncOperation{
		ID:           "test-sync-1",
		GroupID:      "group-1",
		SourceMirror: "mirror-a",
		TargetMirror: "mirror-b",
	}

	mso := NewManagedSyncOperation(op, nil)

	// Add files
	file1 := &SyncFile{Path: "/data/file1.txt", Size: 1024, Action: SyncActionCopy}
	file2 := &SyncFile{Path: "/data/file2.txt", Size: 2048, Action: SyncActionCopy}

	mso.AddFile(file1)
	mso.AddFile(file2)

	if mso.FileCount() != 2 {
		t.Errorf("expected 2 files, got %d", mso.FileCount())
	}

	// Start operation
	mso.Start()

	// Process file 1
	mso.StartFile("/data/file1.txt")
	mfs1 := mso.GetFile("/data/file1.txt")
	if !mfs1.IsSyncing() {
		t.Error("expected file1 to be syncing")
	}

	mso.CompleteFile("/data/file1.txt", 1024)
	if !mfs1.IsCompleted() {
		t.Error("expected file1 to be completed")
	}

	// Process file 2
	mso.StartFile("/data/file2.txt")
	mso.CompleteFile("/data/file2.txt", 2048)

	if mso.CompletedFiles() != 2 {
		t.Errorf("expected 2 completed files, got %d", mso.CompletedFiles())
	}
	if op.BytesTransferred != 3072 {
		t.Errorf("expected 3072 bytes transferred, got %d", op.BytesTransferred)
	}

	// Complete operation
	mso.Complete()
	if !mso.IsSuccessful() {
		t.Error("expected operation to be successful")
	}
}

func TestManagedSyncOperation_FileFailure(t *testing.T) {
	op := &SyncOperation{
		ID:           "test-sync-1",
		GroupID:      "group-1",
		SourceMirror: "mirror-a",
		TargetMirror: "mirror-b",
	}

	mso := NewManagedSyncOperation(op, nil)

	file := &SyncFile{Path: "/data/file.txt", Size: 1024, Action: SyncActionCopy}
	mso.AddFile(file)

	mso.Start()
	mso.StartFile("/data/file.txt")
	mso.FailFile("/data/file.txt", errors.New("read error"))

	mfs := mso.GetFile("/data/file.txt")
	if !mfs.IsFailed() {
		t.Error("expected file to be failed")
	}
	if mso.FailedFiles() != 1 {
		t.Errorf("expected 1 failed file, got %d", mso.FailedFiles())
	}
	if op.FilesFailed != 1 {
		t.Errorf("expected FilesFailed to be 1, got %d", op.FilesFailed)
	}
}

func TestManagedSyncOperation_FileConflict(t *testing.T) {
	op := &SyncOperation{
		ID:           "test-sync-1",
		GroupID:      "group-1",
		SourceMirror: "mirror-a",
		TargetMirror: "mirror-b",
	}

	mso := NewManagedSyncOperation(op, nil)

	file := &SyncFile{Path: "/data/file.txt", Size: 1024, Action: SyncActionConflict}
	mso.AddFile(file)

	mso.Start()
	mso.StartFile("/data/file.txt")

	conflict := &Conflict{
		ID:           "conflict-1",
		Path:         "/data/file.txt",
		SourceMirror: "mirror-a",
		TargetMirror: "mirror-b",
	}

	mso.DetectConflict("/data/file.txt", conflict)

	mfs := mso.GetFile("/data/file.txt")
	if !mfs.IsConflict() {
		t.Error("expected file to be in conflict")
	}
	if mso.ConflictedFiles() != 1 {
		t.Errorf("expected 1 conflicted file, got %d", mso.ConflictedFiles())
	}

	// Resolve conflict
	mso.ResolveConflict("/data/file.txt")
	if !mfs.IsResolved() {
		t.Error("expected file conflict to be resolved")
	}
}

func TestManagedSyncOperation_Callbacks(t *testing.T) {
	var startedCalls, completedCalls, fileStartedCalls, fileCompletedCalls int
	var lastOperationID string

	callbacks := &SyncOperationCallbacks{
		OnStarted: func(operationID string) {
			startedCalls++
			lastOperationID = operationID
		},
		OnCompleted: func(operationID string, bytesTransferred int64, filesCompleted int) {
			completedCalls++
		},
		OnFileStarted: func(operationID string, path string) {
			fileStartedCalls++
		},
		OnFileCompleted: func(operationID string, path string, bytesTransferred int64) {
			fileCompletedCalls++
		},
	}

	op := &SyncOperation{
		ID:           "test-sync-1",
		GroupID:      "group-1",
		SourceMirror: "mirror-a",
		TargetMirror: "mirror-b",
	}

	mso := NewManagedSyncOperation(op, callbacks)

	file := &SyncFile{Path: "/data/file.txt", Size: 1024, Action: SyncActionCopy}
	mso.AddFile(file)

	// Start triggers callback
	mso.Start()
	if startedCalls != 1 || lastOperationID != "test-sync-1" {
		t.Errorf("expected OnStarted called once, got %d", startedCalls)
	}

	// File callbacks
	mso.StartFile("/data/file.txt")
	if fileStartedCalls != 1 {
		t.Errorf("expected OnFileStarted called once, got %d", fileStartedCalls)
	}

	mso.CompleteFile("/data/file.txt", 1024)
	if fileCompletedCalls != 1 {
		t.Errorf("expected OnFileCompleted called once, got %d", fileCompletedCalls)
	}

	// Complete triggers callback
	mso.Complete()
	if completedCalls != 1 {
		t.Errorf("expected OnCompleted called once, got %d", completedCalls)
	}
}

func TestManagedSyncOperation_History(t *testing.T) {
	op := &SyncOperation{
		ID:           "test-sync-1",
		GroupID:      "group-1",
		SourceMirror: "mirror-a",
		TargetMirror: "mirror-b",
	}

	mso := NewManagedSyncOperation(op, nil)

	mso.Start()
	mso.Complete()

	history := mso.History()
	if history == nil {
		t.Fatal("history should not be nil")
	}

	records := history.All()
	if len(records) != 2 {
		t.Errorf("expected 2 history records, got %d", len(records))
	}
}

func TestManagedSyncOperation_AvailableEvents(t *testing.T) {
	op := &SyncOperation{
		ID:           "test-sync-1",
		GroupID:      "group-1",
		SourceMirror: "mirror-a",
		TargetMirror: "mirror-b",
	}

	mso := NewManagedSyncOperation(op, nil)

	// From pending, can start or cancel
	events := mso.AvailableEvents()
	if len(events) != 2 {
		t.Errorf("expected 2 available events from pending, got %d", len(events))
	}

	mso.Start()

	// From in_progress, can complete, fail, or cancel
	events = mso.AvailableEvents()
	if len(events) != 3 {
		t.Errorf("expected 3 available events from in_progress, got %d", len(events))
	}
}

func TestManagedSyncOperation_Duration(t *testing.T) {
	op := &SyncOperation{
		ID:           "test-sync-1",
		GroupID:      "group-1",
		SourceMirror: "mirror-a",
		TargetMirror: "mirror-b",
	}

	mso := NewManagedSyncOperation(op, nil)

	// No duration before start
	if mso.Duration() != 0 {
		t.Error("expected 0 duration before start")
	}

	mso.Start()
	time.Sleep(1 * time.Millisecond)

	// Duration should be non-zero while in progress
	runningDuration := mso.Duration()
	if runningDuration == 0 {
		t.Error("expected non-zero duration while in progress")
	}

	mso.Complete()

	// Duration should be fixed after completion
	finalDuration := mso.Duration()
	if finalDuration < runningDuration {
		t.Error("expected final duration >= running duration")
	}
}

func TestManagedFileSync_Workflow(t *testing.T) {
	file := &SyncFile{
		Path:     "/data/file.txt",
		Size:     1024,
		Checksum: "abc123",
		Action:   SyncActionCopy,
	}

	mfs := newManagedFileSync(file)

	// Initial state
	if mfs.State() != FileSyncStatePending {
		t.Errorf("expected pending state, got %v", mfs.State())
	}
	if !mfs.IsPending() {
		t.Error("expected IsPending() to be true")
	}

	// Start
	mfs.Start()
	if !mfs.IsSyncing() {
		t.Error("expected IsSyncing() to be true")
	}

	// Complete
	mfs.Complete()
	if !mfs.IsCompleted() {
		t.Error("expected IsCompleted() to be true")
	}
	if !mfs.IsTerminal() {
		t.Error("expected IsTerminal() to be true")
	}
}

func TestManagedFileSync_SkipWorkflow(t *testing.T) {
	file := &SyncFile{
		Path:   "/data/file.txt",
		Action: SyncActionSkip,
	}

	mfs := newManagedFileSync(file)

	mfs.Skip()

	if !mfs.IsSkipped() {
		t.Error("expected IsSkipped() to be true")
	}
	if !mfs.IsTerminal() {
		t.Error("expected IsTerminal() to be true")
	}
}

func TestManagedFileSync_ConflictWorkflow(t *testing.T) {
	file := &SyncFile{
		Path:   "/data/file.txt",
		Action: SyncActionConflict,
	}

	mfs := newManagedFileSync(file)

	mfs.Start()

	conflict := &Conflict{
		ID:   "conflict-1",
		Path: "/data/file.txt",
	}

	mfs.DetectConflict(conflict)

	if !mfs.IsConflict() {
		t.Error("expected IsConflict() to be true")
	}

	mfs.Resolve()

	if !mfs.IsResolved() {
		t.Error("expected IsResolved() to be true")
	}
	if !mfs.IsTerminal() {
		t.Error("expected IsTerminal() to be true")
	}
}

func TestManagedFileSync_Duration(t *testing.T) {
	file := &SyncFile{Path: "/data/file.txt"}

	mfs := newManagedFileSync(file)

	// No duration before start
	if mfs.Duration() != 0 {
		t.Error("expected 0 duration before start")
	}

	mfs.Start()
	time.Sleep(1 * time.Millisecond)

	// Duration should be non-zero while syncing
	syncingDuration := mfs.Duration()
	if syncingDuration == 0 {
		t.Error("expected non-zero duration while syncing")
	}

	mfs.Complete()

	// Duration should be fixed after completion
	finalDuration := mfs.Duration()
	if finalDuration < syncingDuration {
		t.Error("expected final duration >= syncing duration")
	}
}

func TestManagedSyncOperation_NilCallbacks(t *testing.T) {
	op := &SyncOperation{
		ID:           "test-sync-1",
		GroupID:      "group-1",
		SourceMirror: "mirror-a",
		TargetMirror: "mirror-b",
	}

	// Empty callbacks struct
	callbacks := &SyncOperationCallbacks{}

	mso := NewManagedSyncOperation(op, callbacks)

	file := &SyncFile{Path: "/data/file.txt", Size: 1024}
	mso.AddFile(file)

	// These should not panic
	mso.Start()
	mso.StartFile("/data/file.txt")
	mso.CompleteFile("/data/file.txt", 1024)
	mso.Complete()
}

func TestSyncStatusToString(t *testing.T) {
	tests := []struct {
		status  SyncStatus
		display string
	}{
		{SyncStatusPending, "Pending"},
		{SyncStatusInProgress, "In Progress"},
		{SyncStatusCompleted, "Completed"},
		{SyncStatusFailed, "Failed"},
		{SyncStatusCancelled, "Cancelled"},
		{SyncStatus("unknown"), "unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if got := SyncStatusToString(tt.status); got != tt.display {
				t.Errorf("SyncStatusToString(%v) = %v, want %v", tt.status, got, tt.display)
			}
		})
	}
}

func TestFileSyncStateToString(t *testing.T) {
	tests := []struct {
		state   FileSyncState
		display string
	}{
		{FileSyncStatePending, "Pending"},
		{FileSyncStateSyncing, "Syncing"},
		{FileSyncStateCompleted, "Completed"},
		{FileSyncStateFailed, "Failed"},
		{FileSyncStateSkipped, "Skipped"},
		{FileSyncStateConflict, "Conflict"},
		{FileSyncStateResolved, "Resolved"},
		{FileSyncState("unknown"), "unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if got := FileSyncStateToString(tt.state); got != tt.display {
				t.Errorf("FileSyncStateToString(%v) = %v, want %v", tt.state, got, tt.display)
			}
		})
	}
}

func TestManagedSyncOperation_FullWorkflow(t *testing.T) {
	op := &SyncOperation{
		ID:           "test-sync-1",
		GroupID:      "group-1",
		SourceMirror: "mirror-a",
		TargetMirror: "mirror-b",
	}

	mso := NewManagedSyncOperation(op, nil)

	// Add files
	files := []*SyncFile{
		{Path: "/data/file1.txt", Size: 1024, Action: SyncActionCopy},
		{Path: "/data/file2.txt", Size: 2048, Action: SyncActionCopy},
		{Path: "/data/file3.txt", Size: 512, Action: SyncActionDelete},
	}

	for _, f := range files {
		mso.AddFile(f)
	}

	// Start operation
	mso.Start()
	if !mso.IsInProgress() {
		t.Error("expected in progress")
	}

	// Process files
	for _, f := range files {
		mso.StartFile(f.Path)
		if f.Action == SyncActionCopy {
			mso.CompleteFile(f.Path, f.Size)
		} else {
			mso.CompleteFile(f.Path, 0)
		}
	}

	// Complete operation
	mso.Complete()
	if !mso.IsSuccessful() {
		t.Error("expected successful")
	}

	if mso.CompletedFiles() != 3 {
		t.Errorf("expected 3 completed files, got %d", mso.CompletedFiles())
	}
	if op.BytesTransferred != 3072 { // 1024 + 2048
		t.Errorf("expected 3072 bytes transferred, got %d", op.BytesTransferred)
	}
}
