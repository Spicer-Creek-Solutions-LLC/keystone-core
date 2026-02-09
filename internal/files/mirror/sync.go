// Package mirror implements mirror groups and geographic routing for file distribution.
package mirror

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/wait"
)

// SyncState represents the state of sync engine.
type SyncState string

// SyncState constants define the possible states.
const (
	SyncStateIdle    SyncState = "idle"
	SyncStateSyncing SyncState = "syncing"
	SyncStateError   SyncState = "error"
)

// SyncStatus represents the status of a sync operation.
type SyncStatus string

// SyncStatus constants define the possible statuses.
const (
	SyncStatusPending    SyncStatus = "pending"
	SyncStatusInProgress SyncStatus = "in_progress"
	SyncStatusCompleted  SyncStatus = "completed"
	SyncStatusFailed     SyncStatus = "failed"
	SyncStatusCancelled  SyncStatus = "cancelled"
)

// SyncAction represents the action to take for a file during sync.
type SyncAction string

// SyncActionCopy constants define the actions.
const (
	SyncActionCopy     SyncAction = "copy"
	SyncActionDelete   SyncAction = "delete"
	SyncActionConflict SyncAction = "conflict"
	SyncActionSkip     SyncAction = "skip"
)

// ConflictStrategy defines how to resolve sync conflicts.
type ConflictStrategy string

// ConflictStrategyNewestWins constants define the strategies.
const (
	ConflictStrategyNewestWins  ConflictStrategy = "newest-wins"
	ConflictStrategyLargestWins ConflictStrategy = "largest-wins"
	ConflictStrategyPrimaryWins ConflictStrategy = "primary-wins"
	ConflictStrategyManual      ConflictStrategy = "manual"
)

// SyncConfig configures the sync engine.
type SyncConfig struct {
	// Interval for automatic sync (0 disables automatic sync)
	Interval time.Duration `json:"interval" yaml:"interval"`

	// BatchSize is the number of files to sync in one batch
	BatchSize int `json:"batch_size" yaml:"batch_size"`

	// BandwidthLimit in bytes per second (0 for unlimited)
	BandwidthLimit int64 `json:"bandwidth_limit" yaml:"bandwidth_limit"`

	// ConflictStrategy defines default conflict resolution
	ConflictStrategy ConflictStrategy `json:"conflict_strategy" yaml:"conflict_strategy"`

	// RetryAttempts for failed sync operations
	RetryAttempts int `json:"retry_attempts" yaml:"retry_attempts"`

	// RetryDelay between retry attempts
	RetryDelay time.Duration `json:"retry_delay" yaml:"retry_delay"`

	// ExcludePatterns for files to skip during sync
	ExcludePatterns []string `json:"exclude_patterns" yaml:"exclude_patterns"`

	// PrioritizeSmallFiles syncs small files first for quick consistency
	PrioritizeSmallFiles bool `json:"prioritize_small_files" yaml:"prioritize_small_files"`

	// SmallFileSizeThreshold defines what's considered a small file
	SmallFileSizeThreshold int64 `json:"small_file_size_threshold" yaml:"small_file_size_threshold"`
}

// DefaultSyncConfig returns sensible defaults for sync configuration.
func DefaultSyncConfig() *SyncConfig {
	return &SyncConfig{
		Interval:               15 * time.Minute,
		BatchSize:              100,
		BandwidthLimit:         0, // Unlimited
		ConflictStrategy:       ConflictStrategyNewestWins,
		RetryAttempts:          3,
		RetryDelay:             5 * time.Second,
		PrioritizeSmallFiles:   true,
		SmallFileSizeThreshold: 1024 * 1024, // 1MB
	}
}

// SyncFile represents a file to be synced.
type SyncFile struct {
	Path         string     `json:"path"`
	Checksum     string     `json:"checksum"`
	Size         int64      `json:"size"`
	ModifiedTime time.Time  `json:"modified_time"`
	Action       SyncAction `json:"action"`
}

// SyncOperation represents a sync operation between mirrors.
type SyncOperation struct {
	ID               string     `json:"id"`
	GroupID          string     `json:"group_id"`
	SourceMirror     string     `json:"source_mirror"`
	TargetMirror     string     `json:"target_mirror"`
	Files            []SyncFile `json:"files"`
	Priority         int        `json:"priority"`
	StartedAt        time.Time  `json:"started_at"`
	CompletedAt      time.Time  `json:"completed_at,omitempty"`
	BytesTransferred int64      `json:"bytes_transferred"`
	FilesCompleted   int        `json:"files_completed"`
	FilesFailed      int        `json:"files_failed"`
	Status           SyncStatus `json:"status"`
	Error            string     `json:"error,omitempty"`
	Progress         float64    `json:"progress"` // 0.0 to 1.0
}

// TotalFiles returns the total number of files in the operation.
func (op *SyncOperation) TotalFiles() int {
	return len(op.Files)
}

// Duration returns the duration of the sync operation.
func (op *SyncOperation) Duration() time.Duration {
	if op.CompletedAt.IsZero() {
		return time.Since(op.StartedAt)
	}
	return op.CompletedAt.Sub(op.StartedAt)
}

// Conflict represents a sync conflict between mirrors.
type Conflict struct {
	ID           string    `json:"id"`
	GroupID      string    `json:"group_id"`
	Path         string    `json:"path"`
	SourceMirror string    `json:"source_mirror"`
	TargetMirror string    `json:"target_mirror"`
	SourceInfo   FileInfo  `json:"source_info"`
	TargetInfo   FileInfo  `json:"target_info"`
	DetectedAt   time.Time `json:"detected_at"`
	ResolvedAt   time.Time `json:"resolved_at,omitempty"`
	Resolution   string    `json:"resolution,omitempty"` // "source", "target", "manual"
	ResolvedBy   string    `json:"resolved_by,omitempty"`
}

// FileInfo contains file metadata for comparison.
type FileInfo struct {
	Size         int64     `json:"size"`
	Checksum     string    `json:"checksum"`
	ModifiedTime time.Time `json:"modified_time"`
}

// SyncProgress represents the progress of a sync operation.
type SyncProgress struct {
	OperationID      string     `json:"operation_id"`
	Status           SyncStatus `json:"status"`
	TotalFiles       int        `json:"total_files"`
	FilesCompleted   int        `json:"files_completed"`
	FilesFailed      int        `json:"files_failed"`
	TotalBytes       int64      `json:"total_bytes"`
	BytesTransferred int64      `json:"bytes_transferred"`
	Progress         float64    `json:"progress"`
	CurrentFile      string     `json:"current_file,omitempty"`
	StartedAt        time.Time  `json:"started_at"`
	EstimatedETA     time.Time  `json:"estimated_eta,omitempty"`
}

// SyncHistory represents a completed sync operation for history.
type SyncHistory struct {
	OperationID      string        `json:"operation_id"`
	GroupID          string        `json:"group_id"`
	SourceMirror     string        `json:"source_mirror"`
	TargetMirror     string        `json:"target_mirror"`
	StartedAt        time.Time     `json:"started_at"`
	CompletedAt      time.Time     `json:"completed_at"`
	Duration         time.Duration `json:"duration"`
	TotalFiles       int           `json:"total_files"`
	FilesCompleted   int           `json:"files_completed"`
	FilesFailed      int           `json:"files_failed"`
	BytesTransferred int64         `json:"bytes_transferred"`
	Status           SyncStatus    `json:"status"`
	Error            string        `json:"error,omitempty"`
}

// GroupSyncStatus represents the sync status of a mirror group.
type GroupSyncStatus struct {
	GroupID          string        `json:"group_id"`
	State            SyncState     `json:"state"`
	LastSyncAt       time.Time     `json:"last_sync_at,omitempty"`
	LastSyncDuration time.Duration `json:"last_sync_duration,omitempty"`
	LastSyncStatus   SyncStatus    `json:"last_sync_status,omitempty"`
	NextSyncAt       time.Time     `json:"next_sync_at,omitempty"`
	PendingOps       int           `json:"pending_ops"`
	ActiveOps        int           `json:"active_ops"`
	ConflictCount    int           `json:"conflict_count"`
}

// Backend represents a backend that can be synced.
type Backend interface {
	// ListFiles returns all files matching the path prefix
	ListFiles(ctx context.Context, prefix string) ([]SyncFile, error)

	// GetFile returns a reader for the file content
	GetFile(ctx context.Context, path string) (io.ReadCloser, error)

	// PutFile writes file content from a reader
	PutFile(ctx context.Context, path string, reader io.Reader, size int64) error

	// DeleteFile removes a file
	DeleteFile(ctx context.Context, path string) error

	// GetFileInfo returns metadata for a file
	GetFileInfo(ctx context.Context, path string) (*FileInfo, error)
}

// SyncEngine manages synchronization between mirrors.
type SyncEngine struct {
	config *SyncConfig

	// Mirror group registry
	registry *Registry

	// Backend lookup
	backends map[string]Backend

	// State management
	state      SyncState
	stateMu    sync.RWMutex
	groupState map[string]*GroupSyncStatus

	// Operation queue
	queue       []*SyncOperation
	queueMu     sync.Mutex
	activeOps   map[string]*SyncOperation
	activeOpsMu sync.RWMutex

	// Conflicts
	conflicts   map[string]*Conflict
	conflictsMu sync.RWMutex

	// History
	history   []*SyncHistory
	historyMu sync.RWMutex
	maxHist   int

	// Control
	stopCh chan struct{}
	wg     sync.WaitGroup

	// Callbacks
	onProgress func(*SyncProgress)
	onConflict func(*Conflict)
}

// NewSyncEngine creates a new sync engine.
func NewSyncEngine(registry *Registry, config *SyncConfig) *SyncEngine {
	if config == nil {
		config = DefaultSyncConfig()
	}
	return &SyncEngine{
		config:     config,
		registry:   registry,
		backends:   make(map[string]Backend),
		state:      SyncStateIdle,
		groupState: make(map[string]*GroupSyncStatus),
		queue:      make([]*SyncOperation, 0),
		activeOps:  make(map[string]*SyncOperation),
		conflicts:  make(map[string]*Conflict),
		history:    make([]*SyncHistory, 0),
		maxHist:    1000,
		stopCh:     make(chan struct{}),
	}
}

// RegisterBackend registers a backend for a mirror.
func (e *SyncEngine) RegisterBackend(mirrorID string, backend Backend) {
	e.backends[mirrorID] = backend
}

// OnProgress sets the callback for sync progress updates.
func (e *SyncEngine) OnProgress(cb func(*SyncProgress)) {
	e.onProgress = cb
}

// OnConflict sets the callback for conflict detection.
func (e *SyncEngine) OnConflict(cb func(*Conflict)) {
	e.onConflict = cb
}

// Start begins the sync engine background processing.
func (e *SyncEngine) Start() {
	e.wg.Add(2)
	go e.runScheduler()
	go e.runWorker()
}

// Stop stops the sync engine.
func (e *SyncEngine) Stop() {
	close(e.stopCh)
	e.wg.Wait()
}

// State returns the current sync engine state.
func (e *SyncEngine) State() SyncState {
	e.stateMu.RLock()
	defer e.stateMu.RUnlock()
	return e.state
}

func (e *SyncEngine) setState(state SyncState) {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	e.state = state
}

// runScheduler handles automatic sync scheduling.
func (e *SyncEngine) runScheduler() {
	defer e.wg.Done()

	if e.config.Interval == 0 {
		return
	}

	ticker := time.NewTicker(e.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			e.scheduleAllGroups()
		case <-e.stopCh:
			return
		}
	}
}

// runWorker processes sync operations from the queue.
func (e *SyncEngine) runWorker() {
	defer e.wg.Done()

	for {
		select {
		case <-e.stopCh:
			return
		default:
			op := e.dequeue()
			if op == nil {
				if !e.waitForRetry(100 * time.Millisecond) {
					return
				}
				continue
			}
			e.executeSync(op)
		}
	}
}

// scheduleAllGroups schedules sync for all registered groups.
func (e *SyncEngine) scheduleAllGroups() {
	groups := e.registry.List()
	for _, group := range groups {
		_ = e.ScheduleSync(group.ID(), "", 0) //nolint:errcheck // best-effort batch scheduling
	}
}

// ScheduleSync schedules a sync operation for a mirror group.
func (e *SyncEngine) ScheduleSync(groupID, pathPrefix string, priority int) error {
	group, ok := e.registry.Get(groupID)
	if !ok {
		return ErrGroupNotFound
	}

	// Get primary and secondary mirrors
	mirrors := group.GetMirrors()
	if len(mirrors) < 2 {
		return nil // Nothing to sync with single mirror
	}

	primary := mirrors[0]
	for _, mirror := range mirrors {
		if mirror.IsPrimary {
			primary = mirror
			break
		}
	}

	// Create sync operations from primary to all secondaries
	for _, mirror := range mirrors {
		if mirror.ID == primary.ID {
			continue
		}

		op := &SyncOperation{
			ID:           fmt.Sprintf("%s-%s-%d", groupID, mirror.ID, time.Now().UnixNano()),
			GroupID:      groupID,
			SourceMirror: primary.ID,
			TargetMirror: mirror.ID,
			Priority:     priority,
			Status:       SyncStatusPending,
		}

		e.enqueue(op)
	}

	return nil
}

// TriggerSync immediately triggers a sync operation.
func (e *SyncEngine) TriggerSync(groupID, sourceMirror, targetMirror string, priority int) (*SyncOperation, error) {
	group, ok := e.registry.Get(groupID)
	if !ok {
		return nil, ErrGroupNotFound
	}
	_ = group // Validate group exists

	op := &SyncOperation{
		ID:           fmt.Sprintf("%s-%s-%d", groupID, targetMirror, time.Now().UnixNano()),
		GroupID:      groupID,
		SourceMirror: sourceMirror,
		TargetMirror: targetMirror,
		Priority:     priority,
		Status:       SyncStatusPending,
	}

	e.enqueue(op)
	return op, nil
}

// enqueue adds an operation to the queue with priority ordering.
func (e *SyncEngine) enqueue(op *SyncOperation) {
	e.queueMu.Lock()
	defer e.queueMu.Unlock()

	e.queue = append(e.queue, op)

	// Sort by priority (higher first)
	sort.Slice(e.queue, func(i, j int) bool {
		return e.queue[i].Priority > e.queue[j].Priority
	})

	// Update group state
	e.updateGroupState(op.GroupID, func(gs *GroupSyncStatus) {
		gs.PendingOps++
	})
}

// dequeue removes and returns the highest priority operation.
func (e *SyncEngine) dequeue() *SyncOperation {
	e.queueMu.Lock()
	defer e.queueMu.Unlock()

	if len(e.queue) == 0 {
		return nil
	}

	op := e.queue[0]
	e.queue = e.queue[1:]

	// Update group state
	e.updateGroupState(op.GroupID, func(gs *GroupSyncStatus) {
		gs.PendingOps--
	})

	return op
}

// updateGroupState updates the group sync state.
func (e *SyncEngine) updateGroupState(groupID string, fn func(*GroupSyncStatus)) {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()

	gs, ok := e.groupState[groupID]
	if !ok {
		gs = &GroupSyncStatus{
			GroupID: groupID,
			State:   SyncStateIdle,
		}
		e.groupState[groupID] = gs
	}
	fn(gs)
}

// executeSync executes a sync operation.
func (e *SyncEngine) executeSync(op *SyncOperation) {
	ctx := context.Background()

	// Mark as active
	e.activeOpsMu.Lock()
	e.activeOps[op.ID] = op
	e.activeOpsMu.Unlock()

	e.updateGroupState(op.GroupID, func(gs *GroupSyncStatus) {
		gs.State = SyncStateSyncing
		gs.ActiveOps++
	})

	e.setState(SyncStateSyncing)
	op.Status = SyncStatusInProgress
	op.StartedAt = time.Now()

	defer func() {
		// Mark as inactive
		e.activeOpsMu.Lock()
		delete(e.activeOps, op.ID)
		e.activeOpsMu.Unlock()

		e.updateGroupState(op.GroupID, func(gs *GroupSyncStatus) {
			gs.ActiveOps--
			gs.LastSyncAt = time.Now()
			gs.LastSyncDuration = op.Duration()
			gs.LastSyncStatus = op.Status
			if gs.ActiveOps == 0 && gs.PendingOps == 0 {
				gs.State = SyncStateIdle
				gs.NextSyncAt = time.Now().Add(e.config.Interval)
			}
		})

		// Check if all ops complete
		e.activeOpsMu.RLock()
		activeCount := len(e.activeOps)
		e.activeOpsMu.RUnlock()

		if activeCount == 0 {
			e.setState(SyncStateIdle)
		}

		// Save to history
		e.addToHistory(op)
	}()

	// Get backends
	sourceBackend := e.backends[op.SourceMirror]
	targetBackend := e.backends[op.TargetMirror]

	if sourceBackend == nil || targetBackend == nil {
		op.Status = SyncStatusFailed
		op.Error = "backend not found"
		return
	}

	// Detect changes
	files, err := e.detectChanges(ctx, sourceBackend, targetBackend, "")
	if err != nil {
		op.Status = SyncStatusFailed
		op.Error = err.Error()
		return
	}

	op.Files = files

	// Sort files (small files first if configured)
	if e.config.PrioritizeSmallFiles {
		e.sortFilesByPriority(op.Files)
	}

	// Calculate total bytes
	var totalBytes int64
	for _, f := range op.Files {
		if f.Action == SyncActionCopy {
			totalBytes += f.Size
		}
	}

	// Sync each file
	for i, file := range op.Files {
		select {
		case <-e.stopCh:
			op.Status = SyncStatusCancelled
			return
		default:
		}

		// Report progress
		op.Progress = float64(i) / float64(len(op.Files))
		e.reportProgress(op, file.Path)

		err := e.syncFile(ctx, op, file, sourceBackend, targetBackend)
		if err != nil {
			op.FilesFailed++
			// Continue with retry logic
			for attempt := 0; attempt < e.config.RetryAttempts; attempt++ {
				if !e.waitForRetry(e.config.RetryDelay) {
					op.Status = SyncStatusCancelled
					return
				}
				err = e.syncFile(ctx, op, file, sourceBackend, targetBackend)
				if err == nil {
					op.FilesCompleted++
					if file.Action == SyncActionCopy {
						op.BytesTransferred += file.Size
					}
					break
				}
			}
			if err != nil {
				// Check if this is a conflict
				if file.Action == SyncActionConflict {
					e.handleConflict(op, file, sourceBackend, targetBackend)
				}
			}
		} else {
			op.FilesCompleted++
			if file.Action == SyncActionCopy {
				op.BytesTransferred += file.Size
			}
		}
	}

	op.Progress = 1.0
	op.CompletedAt = time.Now()
	if op.FilesFailed > 0 {
		op.Status = SyncStatusFailed
		op.Error = fmt.Sprintf("%d files failed", op.FilesFailed)
	} else {
		op.Status = SyncStatusCompleted
	}
}

// detectChanges compares source and target to find differences.
func (e *SyncEngine) detectChanges(ctx context.Context, source, target Backend, prefix string) ([]SyncFile, error) {
	sourceFiles, err := source.ListFiles(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("listing source files: %w", err)
	}

	targetFiles, err := target.ListFiles(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("listing target files: %w", err)
	}

	// Build target map for quick lookup
	targetMap := make(map[string]SyncFile)
	for _, f := range targetFiles {
		targetMap[f.Path] = f
	}

	sourceMap := make(map[string]SyncFile)
	for _, f := range sourceFiles {
		sourceMap[f.Path] = f
	}

	var changes []SyncFile

	// Files to copy or update
	for _, sf := range sourceFiles {
		// Check exclude patterns
		if e.matchesExcludePattern(sf.Path) {
			continue
		}

		tf, exists := targetMap[sf.Path]
		if !exists {
			// New file - copy
			sf.Action = SyncActionCopy
			changes = append(changes, sf)
		} else if sf.Checksum != tf.Checksum {
			// Different checksum - potential conflict or update
			if sf.ModifiedTime.After(tf.ModifiedTime) {
				// Source is newer - copy
				sf.Action = SyncActionCopy
				changes = append(changes, sf)
			} else {
				// Target is newer or same time but different checksum - conflict
				sf.Action = SyncActionConflict
				changes = append(changes, sf)
			}
		}
		// Same checksum - skip
	}

	// Files to delete (in target but not in source)
	for _, tf := range targetFiles {
		if e.matchesExcludePattern(tf.Path) {
			continue
		}
		if _, exists := sourceMap[tf.Path]; !exists {
			tf.Action = SyncActionDelete
			changes = append(changes, tf)
		}
	}

	return changes, nil
}

// matchesExcludePattern checks if a path matches any exclude pattern.
func (e *SyncEngine) matchesExcludePattern(path string) bool {
	for _, pattern := range e.config.ExcludePatterns {
		// Simple glob matching - could be enhanced with proper glob library
		if matchGlob(pattern, path) {
			return true
		}
	}
	return false
}

// matchGlob performs simple glob matching.
func matchGlob(pattern, path string) bool {
	if pattern == "" {
		return path == ""
	}
	regexPattern := globToRegex(pattern)
	re, err := regexp.Compile(regexPattern)
	if err != nil {
		return pattern == path
	}
	return re.MatchString(path)
}

func globToRegex(pattern string) string {
	var sb strings.Builder
	sb.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		switch ch {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				sb.WriteString(".*")
				i++
			} else {
				sb.WriteString(`[^/]*`)
			}
		case '?':
			sb.WriteString(`[^/]`)
		case '.', '+', '(', ')', '|', '^', '$', '{', '}', '[', ']', '\\':
			sb.WriteByte('\\')
			sb.WriteByte(ch)
		default:
			sb.WriteByte(ch)
		}
	}
	sb.WriteString("$")
	return sb.String()
}

// sortFilesByPriority sorts files with small files first.
func (e *SyncEngine) sortFilesByPriority(files []SyncFile) {
	sort.Slice(files, func(i, j int) bool {
		// Small files first
		iSmall := files[i].Size <= e.config.SmallFileSizeThreshold
		jSmall := files[j].Size <= e.config.SmallFileSizeThreshold

		if iSmall && !jSmall {
			return true
		}
		if !iSmall && jSmall {
			return false
		}
		// Same category - sort by size ascending
		return files[i].Size < files[j].Size
	})
}

// syncFile syncs a single file.
func (e *SyncEngine) syncFile(ctx context.Context, op *SyncOperation, file SyncFile, source, target Backend) error {
	switch file.Action {
	case SyncActionCopy:
		reader, err := source.GetFile(ctx, file.Path)
		if err != nil {
			return fmt.Errorf("getting source file: %w", err)
		}
		defer reader.Close()

		// Apply bandwidth limiting if configured
		var r io.Reader = reader
		if e.config.BandwidthLimit > 0 {
			r = &rateLimitedReader{
				ctx:    ctx,
				reader: reader,
				limit:  e.config.BandwidthLimit,
			}
		}

		if err := target.PutFile(ctx, file.Path, r, file.Size); err != nil {
			return fmt.Errorf("putting target file: %w", err)
		}

	case SyncActionDelete:
		if err := target.DeleteFile(ctx, file.Path); err != nil {
			return fmt.Errorf("deleting target file: %w", err)
		}

	case SyncActionConflict:
		return e.resolveConflict(ctx, op, file, source, target)

	default:
		// SyncActionSkip - no operation needed
	}

	return nil
}

// handleConflict records a conflict for manual resolution.
func (e *SyncEngine) handleConflict(op *SyncOperation, file SyncFile, source, target Backend) {
	ctx := context.Background()

	sourceInfo, _ := source.GetFileInfo(ctx, file.Path)
	targetInfo, _ := target.GetFileInfo(ctx, file.Path)

	conflict := &Conflict{
		ID:           fmt.Sprintf("%s-%s-%d", op.ID, file.Path, time.Now().UnixNano()),
		GroupID:      op.GroupID,
		Path:         file.Path,
		SourceMirror: op.SourceMirror,
		TargetMirror: op.TargetMirror,
		DetectedAt:   time.Now(),
	}

	if sourceInfo != nil {
		conflict.SourceInfo = *sourceInfo
	}
	if targetInfo != nil {
		conflict.TargetInfo = *targetInfo
	}

	e.conflictsMu.Lock()
	e.conflicts[conflict.ID] = conflict
	e.conflictsMu.Unlock()

	e.updateGroupState(op.GroupID, func(gs *GroupSyncStatus) {
		gs.ConflictCount++
	})

	if e.onConflict != nil {
		e.onConflict(conflict)
	}
}

// resolveConflict applies the configured conflict resolution strategy.
func (e *SyncEngine) resolveConflict(ctx context.Context, op *SyncOperation, file SyncFile, source, target Backend) error {
	sourceInfo, err := source.GetFileInfo(ctx, file.Path)
	if err != nil {
		return err
	}

	targetInfo, err := target.GetFileInfo(ctx, file.Path)
	if err != nil {
		return err
	}

	var useSource bool

	switch e.config.ConflictStrategy {
	case ConflictStrategyNewestWins:
		useSource = sourceInfo.ModifiedTime.After(targetInfo.ModifiedTime)

	case ConflictStrategyLargestWins:
		useSource = sourceInfo.Size > targetInfo.Size

	case ConflictStrategyPrimaryWins:
		// Source is always primary in our sync model
		useSource = true

	case ConflictStrategyManual:
		// Record conflict for manual resolution
		e.handleConflict(op, file, source, target) //nolint:contextcheck // handleConflict doesn't take context
		return fmt.Errorf("conflict requires manual resolution")
	}

	if useSource {
		// Copy from source to target
		reader, err := source.GetFile(ctx, file.Path)
		if err != nil {
			return err
		}
		defer reader.Close()
		return target.PutFile(ctx, file.Path, reader, sourceInfo.Size)
	}

	// Keep target version (no action needed)
	return nil
}

// reportProgress sends progress updates.
func (e *SyncEngine) reportProgress(op *SyncOperation, currentFile string) {
	if e.onProgress == nil {
		return
	}

	var totalBytes int64
	for _, f := range op.Files {
		if f.Action == SyncActionCopy {
			totalBytes += f.Size
		}
	}

	progress := &SyncProgress{
		OperationID:      op.ID,
		Status:           op.Status,
		TotalFiles:       len(op.Files),
		FilesCompleted:   op.FilesCompleted,
		FilesFailed:      op.FilesFailed,
		TotalBytes:       totalBytes,
		BytesTransferred: op.BytesTransferred,
		Progress:         op.Progress,
		CurrentFile:      currentFile,
		StartedAt:        op.StartedAt,
	}

	// Estimate ETA
	if op.Progress > 0 {
		elapsed := time.Since(op.StartedAt)
		remaining := time.Duration(float64(elapsed) * (1.0 - op.Progress) / op.Progress)
		progress.EstimatedETA = time.Now().Add(remaining)
	}

	e.onProgress(progress)
}

// addToHistory adds a completed operation to history.
func (e *SyncEngine) addToHistory(op *SyncOperation) {
	e.historyMu.Lock()
	defer e.historyMu.Unlock()

	hist := &SyncHistory{
		OperationID:      op.ID,
		GroupID:          op.GroupID,
		SourceMirror:     op.SourceMirror,
		TargetMirror:     op.TargetMirror,
		StartedAt:        op.StartedAt,
		CompletedAt:      op.CompletedAt,
		Duration:         op.Duration(),
		TotalFiles:       len(op.Files),
		FilesCompleted:   op.FilesCompleted,
		FilesFailed:      op.FilesFailed,
		BytesTransferred: op.BytesTransferred,
		Status:           op.Status,
		Error:            op.Error,
	}

	e.history = append(e.history, hist)

	// Trim history if too large
	if len(e.history) > e.maxHist {
		e.history = e.history[len(e.history)-e.maxHist:]
	}
}

// GetGroupStatus returns the sync status for a mirror group.
func (e *SyncEngine) GetGroupStatus(groupID string) *GroupSyncStatus {
	e.stateMu.RLock()
	defer e.stateMu.RUnlock()

	gs, ok := e.groupState[groupID]
	if !ok {
		return &GroupSyncStatus{
			GroupID: groupID,
			State:   SyncStateIdle,
		}
	}
	return gs
}

// GetActiveOperations returns currently running sync operations.
func (e *SyncEngine) GetActiveOperations() []*SyncOperation {
	e.activeOpsMu.RLock()
	defer e.activeOpsMu.RUnlock()

	ops := make([]*SyncOperation, 0, len(e.activeOps))
	for _, op := range e.activeOps {
		ops = append(ops, op)
	}
	return ops
}

// GetPendingOperations returns queued sync operations.
func (e *SyncEngine) GetPendingOperations() []*SyncOperation {
	e.queueMu.Lock()
	defer e.queueMu.Unlock()

	result := make([]*SyncOperation, len(e.queue))
	copy(result, e.queue)
	return result
}

// GetConflicts returns all unresolved conflicts.
func (e *SyncEngine) GetConflicts() []*Conflict {
	e.conflictsMu.RLock()
	defer e.conflictsMu.RUnlock()

	conflicts := make([]*Conflict, 0, len(e.conflicts))
	for _, c := range e.conflicts {
		if c.ResolvedAt.IsZero() {
			conflicts = append(conflicts, c)
		}
	}
	return conflicts
}

// ResolveConflict manually resolves a conflict.
func (e *SyncEngine) ResolveConflict(conflictID, resolution, resolvedBy string) error {
	e.conflictsMu.Lock()
	defer e.conflictsMu.Unlock()

	conflict, ok := e.conflicts[conflictID]
	if !ok {
		return fmt.Errorf("conflict not found: %s", conflictID)
	}

	if !conflict.ResolvedAt.IsZero() {
		return fmt.Errorf("conflict already resolved")
	}

	ctx := context.Background()
	sourceBackend := e.backends[conflict.SourceMirror]
	targetBackend := e.backends[conflict.TargetMirror]

	if sourceBackend == nil || targetBackend == nil {
		return fmt.Errorf("backend not found")
	}

	switch resolution {
	case "source":
		// Copy from source to target
		reader, err := sourceBackend.GetFile(ctx, conflict.Path)
		if err != nil {
			return err
		}
		defer reader.Close()
		if err := targetBackend.PutFile(ctx, conflict.Path, reader, conflict.SourceInfo.Size); err != nil {
			return err
		}

	case "target":
		// Keep target (no action needed)

	default:
		return fmt.Errorf("invalid resolution: %s", resolution)
	}

	conflict.Resolution = resolution
	conflict.ResolvedAt = time.Now()
	conflict.ResolvedBy = resolvedBy

	e.updateGroupState(conflict.GroupID, func(gs *GroupSyncStatus) {
		gs.ConflictCount--
	})

	return nil
}

// GetHistory returns sync history.
func (e *SyncEngine) GetHistory(limit int) []*SyncHistory {
	e.historyMu.RLock()
	defer e.historyMu.RUnlock()

	if limit <= 0 || limit > len(e.history) {
		limit = len(e.history)
	}

	// Return most recent first
	result := make([]*SyncHistory, limit)
	for i := 0; i < limit; i++ {
		result[i] = e.history[len(e.history)-1-i]
	}
	return result
}

// rateLimitedReader implements rate-limited reading.
type rateLimitedReader struct {
	ctx       context.Context
	reader    io.Reader
	limit     int64 // bytes per second
	lastRead  time.Time
	bytesRead int64
}

func (r *rateLimitedReader) Read(p []byte) (int, error) {
	if r.ctx != nil {
		select {
		case <-r.ctx.Done():
			return 0, r.ctx.Err()
		default:
		}
	}

	// Implement simple rate limiting
	now := time.Now()
	if !r.lastRead.IsZero() {
		elapsed := now.Sub(r.lastRead)
		expectedTime := time.Duration(float64(r.bytesRead) / float64(r.limit) * float64(time.Second))
		if expectedTime > elapsed {
			delay := expectedTime - elapsed
			timer := time.NewTimer(delay)
			if r.ctx != nil {
				select {
				case <-r.ctx.Done():
					timer.Stop()
					return 0, r.ctx.Err()
				case <-timer.C:
				}
			} else {
				<-timer.C
			}
		}
	}
	r.lastRead = now

	n, err := r.reader.Read(p)
	r.bytesRead += int64(n)
	return n, err
}

func (e *SyncEngine) waitForRetry(delay time.Duration) bool {
	if delay <= 0 {
		return true
	}
	return wait.ForSignal(e.stopCh, delay)
}

// ComputeChecksum calculates SHA-256 checksum of content.
func ComputeChecksum(reader io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, reader); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
