package mirror

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sort"
	"sync"
	"testing"
	"time"
)

// mockBackend implements Backend for testing.
type mockBackend struct {
	files map[string]*mockFile
	mu    sync.RWMutex
}

type mockFile struct {
	data         []byte
	modifiedTime time.Time
}

func newMockBackend() *mockBackend {
	return &mockBackend{
		files: make(map[string]*mockFile),
	}
}

func (b *mockBackend) ListFiles(ctx context.Context, prefix string) ([]SyncFile, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var files []SyncFile
	for path, f := range b.files {
		if prefix == "" || len(path) >= len(prefix) && path[:len(prefix)] == prefix {
			checksum, _ := ComputeChecksum(bytes.NewReader(f.data))
			files = append(files, SyncFile{
				Path:         path,
				Size:         int64(len(f.data)),
				Checksum:     checksum,
				ModifiedTime: f.modifiedTime,
			})
		}
	}
	return files, nil
}

func (b *mockBackend) GetFile(ctx context.Context, path string) (io.ReadCloser, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	f, ok := b.files[path]
	if !ok {
		return nil, ErrMirrorNotFound
	}
	return io.NopCloser(bytes.NewReader(f.data)), nil
}

func (b *mockBackend) PutFile(ctx context.Context, path string, reader io.Reader, size int64) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	b.files[path] = &mockFile{
		data:         data,
		modifiedTime: time.Now(),
	}
	return nil
}

func (b *mockBackend) DeleteFile(ctx context.Context, path string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.files, path)
	return nil
}

func (b *mockBackend) GetFileInfo(ctx context.Context, path string) (*FileInfo, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	f, ok := b.files[path]
	if !ok {
		return nil, ErrMirrorNotFound
	}
	checksum, _ := ComputeChecksum(bytes.NewReader(f.data))
	return &FileInfo{
		Size:         int64(len(f.data)),
		Checksum:     checksum,
		ModifiedTime: f.modifiedTime,
	}, nil
}

func (b *mockBackend) addFile(path string, data []byte, modTime time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.files[path] = &mockFile{
		data:         data,
		modifiedTime: modTime,
	}
}

func (b *mockBackend) hasFile(path string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	_, ok := b.files[path]
	return ok
}

func (b *mockBackend) getFileData(path string) []byte {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if f, ok := b.files[path]; ok {
		return f.data
	}
	return nil
}

func TestSyncEngine_New(t *testing.T) {
	registry := NewRegistry()
	engine := NewSyncEngine(registry, nil)

	if engine.State() != SyncStateIdle {
		t.Errorf("expected idle state, got %s", engine.State())
	}
}

func TestSyncEngine_DefaultConfig(t *testing.T) {
	config := DefaultSyncConfig()

	if config.BatchSize != 100 {
		t.Errorf("expected batch size 100, got %d", config.BatchSize)
	}

	if config.RetryAttempts != 3 {
		t.Errorf("expected 3 retry attempts, got %d", config.RetryAttempts)
	}

	if config.ConflictStrategy != ConflictStrategyNewestWins {
		t.Errorf("expected newest-wins strategy, got %s", config.ConflictStrategy)
	}
}

func TestSyncEngine_DetectChanges(t *testing.T) {
	source := newMockBackend()
	target := newMockBackend()

	now := time.Now()

	// Add files to source
	source.addFile("/file1.txt", []byte("content1"), now)
	source.addFile("/file2.txt", []byte("content2"), now)
	source.addFile("/file3.txt", []byte("content3"), now)

	// Add some files to target
	target.addFile("/file1.txt", []byte("content1"), now)                    // Same
	target.addFile("/file2.txt", []byte("different"), now.Add(-time.Hour))   // Different, source newer
	target.addFile("/file4.txt", []byte("only-target"), now.Add(-time.Hour)) // Only in target

	registry := NewRegistry()
	config := DefaultSyncConfig()
	engine := NewSyncEngine(registry, config)

	changes, err := engine.detectChanges(context.Background(), source, target, "")
	if err != nil {
		t.Fatalf("detectChanges failed: %v", err)
	}

	// Expect:
	// - file2.txt: copy (source newer)
	// - file3.txt: copy (new file)
	// - file4.txt: delete (not in source)

	actionMap := make(map[string]SyncAction)
	for _, c := range changes {
		actionMap[c.Path] = c.Action
	}

	if actionMap["/file1.txt"] != "" {
		t.Error("file1.txt should not be in changes (identical)")
	}

	if actionMap["/file2.txt"] != SyncActionCopy {
		t.Errorf("file2.txt should be copy, got %s", actionMap["/file2.txt"])
	}

	if actionMap["/file3.txt"] != SyncActionCopy {
		t.Errorf("file3.txt should be copy, got %s", actionMap["/file3.txt"])
	}

	if actionMap["/file4.txt"] != SyncActionDelete {
		t.Errorf("file4.txt should be delete, got %s", actionMap["/file4.txt"])
	}
}

func TestSyncEngine_ConflictDetection(t *testing.T) {
	source := newMockBackend()
	target := newMockBackend()

	now := time.Now()

	// Both have file with different content, target is newer
	source.addFile("/conflict.txt", []byte("source-content"), now.Add(-time.Hour))
	target.addFile("/conflict.txt", []byte("target-content"), now)

	registry := NewRegistry()
	config := DefaultSyncConfig()
	engine := NewSyncEngine(registry, config)

	changes, err := engine.detectChanges(context.Background(), source, target, "")
	if err != nil {
		t.Fatalf("detectChanges failed: %v", err)
	}

	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}

	if changes[0].Action != SyncActionConflict {
		t.Errorf("expected conflict action, got %s", changes[0].Action)
	}
}

func TestSyncEngine_SortFilesByPriority(t *testing.T) {
	config := DefaultSyncConfig()
	config.SmallFileSizeThreshold = 1000

	registry := NewRegistry()
	engine := NewSyncEngine(registry, config)

	files := []SyncFile{
		{Path: "/large1.bin", Size: 10000},
		{Path: "/small1.txt", Size: 100},
		{Path: "/large2.bin", Size: 5000},
		{Path: "/small2.txt", Size: 200},
	}

	engine.sortFilesByPriority(files)

	// Small files should come first
	if files[0].Size > 1000 {
		t.Error("first file should be small")
	}
	if files[1].Size > 1000 {
		t.Error("second file should be small")
	}

	// Among small files, smaller first
	if files[0].Size > files[1].Size {
		t.Error("smaller file should come first")
	}
}

func TestSyncEngine_ExcludePatterns(t *testing.T) {
	config := DefaultSyncConfig()
	config.ExcludePatterns = []string{"/tmp/**", "/.git/**"}

	registry := NewRegistry()
	engine := NewSyncEngine(registry, config)

	testCases := []struct {
		path     string
		excluded bool
	}{
		{"/file.txt", false},
		{"/tmp/**", true},
		{"/.git/**", true},
	}

	for _, tc := range testCases {
		if engine.matchesExcludePattern(tc.path) != tc.excluded {
			t.Errorf("path %s: expected excluded=%v", tc.path, tc.excluded)
		}
	}
}

func TestSyncEngine_ScheduleSync(t *testing.T) {
	registry := NewRegistry()

	// Create a mirror group with multiple mirrors
	config := &GroupConfig{
		ID: "test-group",
		Mirrors: []*Mirror{
			{ID: "primary", ClusterID: "cluster1", IsPrimary: true, Enabled: true},
			{ID: "secondary", ClusterID: "cluster2", Enabled: true},
		},
		ReadStrategy: ReadStrategyFailover,
		WritePolicy:  WritePolicyAll,
	}

	group, err := NewGroup(config)
	if err != nil {
		t.Fatalf("failed to create mirror group: %v", err)
	}

	registry.Register(group)

	syncConfig := DefaultSyncConfig()
	syncConfig.Interval = 0 // Disable auto-scheduling

	engine := NewSyncEngine(registry, syncConfig)

	err = engine.ScheduleSync("test-group", "", 0)
	if err != nil {
		t.Fatalf("ScheduleSync failed: %v", err)
	}

	// Check that operation was queued
	pending := engine.GetPendingOperations()
	if len(pending) != 1 {
		t.Errorf("expected 1 pending operation, got %d", len(pending))
	}

	if pending[0].SourceMirror != "primary" {
		t.Errorf("expected source=primary, got %s", pending[0].SourceMirror)
	}

	if pending[0].TargetMirror != "secondary" {
		t.Errorf("expected target=secondary, got %s", pending[0].TargetMirror)
	}
}

func TestSyncEngine_TriggerSync(t *testing.T) {
	registry := NewRegistry()

	config := &GroupConfig{
		ID: "test-group",
		Mirrors: []*Mirror{
			{ID: "m1", ClusterID: "cluster1", Enabled: true},
			{ID: "m2", ClusterID: "cluster2", Enabled: true},
		},
		ReadStrategy: ReadStrategyFailover,
		WritePolicy:  WritePolicyAll,
	}

	group, _ := NewGroup(config)
	registry.Register(group)

	engine := NewSyncEngine(registry, DefaultSyncConfig())

	op, err := engine.TriggerSync("test-group", "m1", "m2", 10)
	if err != nil {
		t.Fatalf("TriggerSync failed: %v", err)
	}

	if op.Priority != 10 {
		t.Errorf("expected priority 10, got %d", op.Priority)
	}

	if op.SourceMirror != "m1" {
		t.Errorf("expected source m1, got %s", op.SourceMirror)
	}
}

func TestSyncEngine_TriggerSync_GroupNotFound(t *testing.T) {
	registry := NewRegistry()
	engine := NewSyncEngine(registry, DefaultSyncConfig())

	_, err := engine.TriggerSync("nonexistent", "m1", "m2", 0)
	if !errors.Is(err, ErrGroupNotFound) {
		t.Errorf("expected ErrGroupNotFound, got %v", err)
	}
}

func TestSyncEngine_QueuePriority(t *testing.T) {
	registry := NewRegistry()
	engine := NewSyncEngine(registry, DefaultSyncConfig())

	// Enqueue operations with different priorities
	op1 := &SyncOperation{ID: "op1", GroupID: "g1", Priority: 5}
	op2 := &SyncOperation{ID: "op2", GroupID: "g1", Priority: 10}
	op3 := &SyncOperation{ID: "op3", GroupID: "g1", Priority: 1}

	engine.enqueue(op1)
	engine.enqueue(op2)
	engine.enqueue(op3)

	// Dequeue should return highest priority first
	dequeued := engine.dequeue()
	if dequeued.ID != "op2" {
		t.Errorf("expected op2 (priority 10) first, got %s", dequeued.ID)
	}

	dequeued = engine.dequeue()
	if dequeued.ID != "op1" {
		t.Errorf("expected op1 (priority 5) second, got %s", dequeued.ID)
	}

	dequeued = engine.dequeue()
	if dequeued.ID != "op3" {
		t.Errorf("expected op3 (priority 1) third, got %s", dequeued.ID)
	}
}

func TestSyncEngine_GroupStatus(t *testing.T) {
	registry := NewRegistry()
	engine := NewSyncEngine(registry, DefaultSyncConfig())

	status := engine.GetGroupStatus("test-group")
	if status.State != SyncStateIdle {
		t.Errorf("expected idle state, got %s", status.State)
	}

	// Update state
	engine.updateGroupState("test-group", func(gs *GroupSyncStatus) {
		gs.State = SyncStateSyncing
		gs.PendingOps = 5
	})

	status = engine.GetGroupStatus("test-group")
	if status.State != SyncStateSyncing {
		t.Errorf("expected syncing state, got %s", status.State)
	}
	if status.PendingOps != 5 {
		t.Errorf("expected 5 pending ops, got %d", status.PendingOps)
	}
}

func TestSyncEngine_Conflicts(t *testing.T) {
	registry := NewRegistry()
	engine := NewSyncEngine(registry, DefaultSyncConfig())

	// Create mock backends
	source := newMockBackend()
	target := newMockBackend()

	source.addFile("/conflict.txt", []byte("source-data"), time.Now())
	target.addFile("/conflict.txt", []byte("target-data"), time.Now())

	engine.RegisterBackend("source", source)
	engine.RegisterBackend("target", target)

	// Manually add a conflict
	conflict := &Conflict{
		ID:           "conflict-1",
		GroupID:      "test-group",
		Path:         "/conflict.txt",
		SourceMirror: "source",
		TargetMirror: "target",
		SourceInfo:   FileInfo{Size: 11, Checksum: "abc"},
		TargetInfo:   FileInfo{Size: 11, Checksum: "def"},
		DetectedAt:   time.Now(),
	}

	engine.conflictsMu.Lock()
	engine.conflicts[conflict.ID] = conflict
	engine.conflictsMu.Unlock()

	// Get unresolved conflicts
	conflicts := engine.GetConflicts()
	if len(conflicts) != 1 {
		t.Errorf("expected 1 conflict, got %d", len(conflicts))
	}

	// Resolve conflict
	err := engine.ResolveConflict("conflict-1", "source", "admin")
	if err != nil {
		t.Fatalf("ResolveConflict failed: %v", err)
	}

	// Verify conflict is resolved
	engine.conflictsMu.RLock()
	resolved := engine.conflicts["conflict-1"]
	engine.conflictsMu.RUnlock()

	if resolved.ResolvedAt.IsZero() {
		t.Error("conflict should be marked as resolved")
	}
	if resolved.Resolution != "source" {
		t.Errorf("expected resolution=source, got %s", resolved.Resolution)
	}

	// Verify target has source content
	if !bytes.Equal(target.getFileData("/conflict.txt"), []byte("source-data")) {
		t.Error("target should have source content after resolution")
	}
}

func TestSyncEngine_History(t *testing.T) {
	registry := NewRegistry()
	engine := NewSyncEngine(registry, DefaultSyncConfig())

	// Add some history
	for i := 0; i < 10; i++ {
		engine.historyMu.Lock()
		engine.history = append(engine.history, &SyncHistory{
			OperationID:    string(rune('a' + i)),
			GroupID:        "test-group",
			TotalFiles:     100,
			FilesCompleted: 100,
			Status:         SyncStatusCompleted,
		})
		engine.historyMu.Unlock()
	}

	// Get limited history
	history := engine.GetHistory(5)
	if len(history) != 5 {
		t.Errorf("expected 5 history entries, got %d", len(history))
	}

	// Should be most recent first
	if history[0].OperationID != "j" { // Last added
		t.Errorf("expected most recent first, got %s", history[0].OperationID)
	}
}

func TestSyncOperation_Methods(t *testing.T) {
	op := &SyncOperation{
		ID:        "test-op",
		Files:     make([]SyncFile, 10),
		StartedAt: time.Now().Add(-5 * time.Minute),
	}

	if op.TotalFiles() != 10 {
		t.Errorf("expected 10 total files, got %d", op.TotalFiles())
	}

	duration := op.Duration()
	if duration < 4*time.Minute || duration > 6*time.Minute {
		t.Errorf("unexpected duration: %v", duration)
	}

	// With completed time
	op.CompletedAt = op.StartedAt.Add(2 * time.Minute)
	if op.Duration() != 2*time.Minute {
		t.Errorf("expected 2 minute duration, got %v", op.Duration())
	}
}

func TestSyncConsts(t *testing.T) {
	// Test state constants
	states := []SyncState{SyncStateIdle, SyncStateSyncing, SyncStateError}
	for _, s := range states {
		if s == "" {
			t.Error("sync state should not be empty")
		}
	}

	// Test status constants
	statuses := []SyncStatus{
		SyncStatusPending, SyncStatusInProgress, SyncStatusCompleted,
		SyncStatusFailed, SyncStatusCancelled,
	}
	for _, s := range statuses {
		if s == "" {
			t.Error("sync status should not be empty")
		}
	}

	// Test action constants
	actions := []SyncAction{
		SyncActionCopy, SyncActionDelete, SyncActionConflict, SyncActionSkip,
	}
	for _, a := range actions {
		if a == "" {
			t.Error("sync action should not be empty")
		}
	}

	// Test conflict strategy constants
	strategies := []ConflictStrategy{
		ConflictStrategyNewestWins, ConflictStrategyLargestWins,
		ConflictStrategyPrimaryWins, ConflictStrategyManual,
	}
	for _, s := range strategies {
		if s == "" {
			t.Error("conflict strategy should not be empty")
		}
	}
}

func TestComputeChecksum(t *testing.T) {
	data := []byte("test content")
	checksum, err := ComputeChecksum(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("ComputeChecksum failed: %v", err)
	}

	// SHA-256 of "test content"
	expected := "6ae8a75555209fd6c44157c0aed8016e763ff435a19cf186f76863140143ff72"
	if checksum != expected {
		t.Errorf("expected checksum %s, got %s", expected, checksum)
	}

	// Same content should produce same checksum
	checksum2, _ := ComputeChecksum(bytes.NewReader(data))
	if checksum != checksum2 {
		t.Error("same content should produce same checksum")
	}
}

func TestRateLimitedReader(t *testing.T) {
	data := make([]byte, 1000)
	reader := &rateLimitedReader{
		ctx:    context.Background(),
		reader: bytes.NewReader(data),
		limit:  10000, // 10KB/s - fast enough for test
	}

	buf := make([]byte, 100)
	start := time.Now()

	// Read some data
	for i := 0; i < 5; i++ {
		_, err := reader.Read(buf)
		if err != nil && !errors.Is(err, io.EOF) {
			t.Fatalf("read failed: %v", err)
		}
	}

	elapsed := time.Since(start)
	// Should complete relatively quickly with high limit
	if elapsed > time.Second {
		t.Errorf("rate limited reader too slow: %v", elapsed)
	}
}

func TestRateLimitedReader_ContextCancel(t *testing.T) {
	data := make([]byte, 1000)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	reader := &rateLimitedReader{
		ctx:    ctx,
		reader: bytes.NewReader(data),
		limit:  10000,
	}

	buf := make([]byte, 100)
	_, err := reader.Read(buf)
	if err == nil {
		t.Fatal("expected error from canceled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestSyncEngine_WaitForRetry(t *testing.T) {
	engine := NewSyncEngine(NewRegistry(), DefaultSyncConfig())
	done := make(chan bool, 1)

	go func() {
		done <- engine.waitForRetry(50 * time.Millisecond)
	}()

	select {
	case ok := <-done:
		if !ok {
			t.Fatal("expected wait to complete")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("wait did not complete")
	}

	close(engine.stopCh)
	if engine.waitForRetry(10 * time.Millisecond) {
		t.Fatal("expected wait to stop on stopCh")
	}
}

func TestSyncEngine_ExecuteSync_Integration(t *testing.T) {
	// Set up registry with mirror group
	registry := NewRegistry()

	config := &GroupConfig{
		ID: "test-group",
		Mirrors: []*Mirror{
			{ID: "source", ClusterID: "cluster1", IsPrimary: true, Enabled: true},
			{ID: "target", ClusterID: "cluster2", Enabled: true},
		},
		ReadStrategy: ReadStrategyFailover,
		WritePolicy:  WritePolicyAll,
	}

	group, _ := NewGroup(config)
	registry.Register(group)

	// Create backends with test data
	source := newMockBackend()
	target := newMockBackend()

	now := time.Now()
	source.addFile("/file1.txt", []byte("content1"), now)
	source.addFile("/file2.txt", []byte("content2"), now)

	// Sync engine
	syncConfig := DefaultSyncConfig()
	syncConfig.Interval = 0 // No auto-sync
	syncConfig.RetryAttempts = 1
	syncConfig.RetryDelay = 10 * time.Millisecond

	engine := NewSyncEngine(registry, syncConfig)
	engine.RegisterBackend("source", source)
	engine.RegisterBackend("target", target)

	// Track progress
	var progressUpdates []*SyncProgress
	var progressMu sync.Mutex
	engine.OnProgress(func(p *SyncProgress) {
		progressMu.Lock()
		progressUpdates = append(progressUpdates, p)
		progressMu.Unlock()
	})

	// Create and execute sync operation
	op := &SyncOperation{
		ID:           "test-op-1",
		GroupID:      "test-group",
		SourceMirror: "source",
		TargetMirror: "target",
		Priority:     1,
		Status:       SyncStatusPending,
	}

	// Execute directly (normally done by worker)
	engine.executeSync(op)

	// Verify sync completed
	if op.Status != SyncStatusCompleted {
		t.Errorf("expected completed status, got %s (error: %s)", op.Status, op.Error)
	}

	// Verify files were copied
	if !target.hasFile("/file1.txt") {
		t.Error("file1.txt should be in target")
	}
	if !target.hasFile("/file2.txt") {
		t.Error("file2.txt should be in target")
	}

	// Verify content
	if !bytes.Equal(target.getFileData("/file1.txt"), []byte("content1")) {
		t.Error("file1.txt content mismatch")
	}

	// Verify progress was reported
	progressMu.Lock()
	if len(progressUpdates) == 0 {
		t.Error("expected progress updates")
	}
	progressMu.Unlock()

	// Verify history was recorded
	history := engine.GetHistory(10)
	if len(history) != 1 {
		t.Errorf("expected 1 history entry, got %d", len(history))
	}
	if history[0].Status != SyncStatusCompleted {
		t.Errorf("expected completed status in history, got %s", history[0].Status)
	}
}

func TestSyncEngine_Callbacks(t *testing.T) {
	registry := NewRegistry()
	engine := NewSyncEngine(registry, DefaultSyncConfig())

	var progressCalled bool
	engine.OnProgress(func(p *SyncProgress) {
		progressCalled = true
	})

	var conflictCalled bool
	engine.OnConflict(func(c *Conflict) {
		conflictCalled = true
	})

	// Trigger progress callback
	op := &SyncOperation{ID: "test"}
	engine.reportProgress(op, "/test.txt")

	if !progressCalled {
		t.Error("progress callback should have been called")
	}

	// Trigger conflict callback
	source := newMockBackend()
	target := newMockBackend()
	source.addFile("/test.txt", []byte("src"), time.Now())
	target.addFile("/test.txt", []byte("tgt"), time.Now())
	engine.RegisterBackend("source", source)
	engine.RegisterBackend("target", target)

	engine.handleConflict(&SyncOperation{
		ID:           "op1",
		GroupID:      "g1",
		SourceMirror: "source",
		TargetMirror: "target",
	}, SyncFile{Path: "/test.txt"}, source, target)

	if !conflictCalled {
		t.Error("conflict callback should have been called")
	}
}

func TestSyncEngine_GetActiveOperations(t *testing.T) {
	registry := NewRegistry()
	engine := NewSyncEngine(registry, DefaultSyncConfig())

	// Initially empty
	active := engine.GetActiveOperations()
	if len(active) != 0 {
		t.Errorf("expected 0 active operations, got %d", len(active))
	}

	// Add some active operations
	engine.activeOpsMu.Lock()
	engine.activeOps["op1"] = &SyncOperation{ID: "op1"}
	engine.activeOps["op2"] = &SyncOperation{ID: "op2"}
	engine.activeOpsMu.Unlock()

	active = engine.GetActiveOperations()
	if len(active) != 2 {
		t.Errorf("expected 2 active operations, got %d", len(active))
	}

	// Verify IDs
	ids := make(map[string]bool)
	for _, op := range active {
		ids[op.ID] = true
	}
	if !ids["op1"] || !ids["op2"] {
		t.Error("missing expected operation IDs")
	}
}

func TestSyncProgress_ETA(t *testing.T) {
	registry := NewRegistry()
	engine := NewSyncEngine(registry, DefaultSyncConfig())

	var lastProgress *SyncProgress
	engine.OnProgress(func(p *SyncProgress) {
		lastProgress = p
	})

	op := &SyncOperation{
		ID:        "test",
		Files:     make([]SyncFile, 100),
		StartedAt: time.Now().Add(-time.Minute),
		Progress:  0.5, // 50% done
	}

	engine.reportProgress(op, "/test.txt")

	if lastProgress == nil {
		t.Fatal("progress should have been reported")
	}

	// ETA should be roughly 1 minute from now (took 1 min for 50%, another 1 min for remaining 50%)
	expectedETA := time.Now().Add(time.Minute)
	delta := lastProgress.EstimatedETA.Sub(expectedETA)
	if delta < -10*time.Second || delta > 10*time.Second {
		t.Errorf("ETA off by too much: expected ~%v, got %v (delta: %v)",
			expectedETA, lastProgress.EstimatedETA, delta)
	}
}

func TestMockBackend(t *testing.T) {
	backend := newMockBackend()
	ctx := context.Background()

	// Test put and get
	err := backend.PutFile(ctx, "/test.txt", bytes.NewReader([]byte("hello")), 5)
	if err != nil {
		t.Fatalf("PutFile failed: %v", err)
	}

	reader, err := backend.GetFile(ctx, "/test.txt")
	if err != nil {
		t.Fatalf("GetFile failed: %v", err)
	}
	defer reader.Close()

	data, _ := io.ReadAll(reader)
	if string(data) != "hello" {
		t.Errorf("expected 'hello', got '%s'", string(data))
	}

	// Test file info
	info, err := backend.GetFileInfo(ctx, "/test.txt")
	if err != nil {
		t.Fatalf("GetFileInfo failed: %v", err)
	}
	if info.Size != 5 {
		t.Errorf("expected size 5, got %d", info.Size)
	}

	// Test list files
	backend.addFile("/dir/file1.txt", []byte("1"), time.Now())
	backend.addFile("/dir/file2.txt", []byte("2"), time.Now())

	files, err := backend.ListFiles(ctx, "/dir")
	if err != nil {
		t.Fatalf("ListFiles failed: %v", err)
	}

	// Sort by path for deterministic comparison
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	if len(files) != 2 {
		t.Errorf("expected 2 files in /dir, got %d", len(files))
	}

	// Test delete
	err = backend.DeleteFile(ctx, "/test.txt")
	if err != nil {
		t.Fatalf("DeleteFile failed: %v", err)
	}

	_, err = backend.GetFile(ctx, "/test.txt")
	if !errors.Is(err, ErrMirrorNotFound) {
		t.Error("expected ErrMirrorNotFound after delete")
	}
}
