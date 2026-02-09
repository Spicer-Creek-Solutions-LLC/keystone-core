package backup

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// RestoreManager orchestrates restore operations
type RestoreManager struct {
	config           *RestoreConfig
	importers        map[ComponentType]Importer
	source           Destination
	encryptor        Encryptor
	logger           Logger
	progressCallback RestoreProgressCallback

	mu             sync.RWMutex
	currentRestore *RestoreInfo
	restoreHistory []RestoreInfo
}

// NewRestoreManager creates a new restore manager
func NewRestoreManager(config *RestoreConfig, logger Logger) *RestoreManager {
	if logger == nil {
		logger = &noopLogger{}
	}
	if config == nil {
		config = &RestoreConfig{
			VerifyIntegrity: true,
			Timeout:         30 * time.Minute,
		}
	}
	return &RestoreManager{
		config:         config,
		importers:      make(map[ComponentType]Importer),
		logger:         logger,
		restoreHistory: make([]RestoreInfo, 0),
	}
}

// SetSource sets the backup source
func (rm *RestoreManager) SetSource(src Destination) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.source = src
}

// SetEncryptor sets the encryptor for decryption
func (rm *RestoreManager) SetEncryptor(enc Encryptor) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.encryptor = enc
}

// RegisterImporter registers an importer for a component type
func (rm *RestoreManager) RegisterImporter(importer Importer) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.importers[importer.Component()] = importer
}

// SetProgressCallback sets the callback for progress updates
func (rm *RestoreManager) SetProgressCallback(cb RestoreProgressCallback) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.progressCallback = cb
}

// Restore performs a restore operation
func (rm *RestoreManager) Restore(ctx context.Context, backupName string) (*RestoreInfo, error) {
	rm.mu.Lock()
	if rm.currentRestore != nil && rm.currentRestore.Status == RestoreStatusRunning {
		rm.mu.Unlock()
		return nil, fmt.Errorf("restore already in progress")
	}

	// Generate restore ID
	restoreID, err := generateBackupID() // reuse the backup ID generator
	if err != nil {
		rm.mu.Unlock()
		return nil, fmt.Errorf("failed to generate restore ID: %w", err)
	}

	info := &RestoreInfo{
		ID:         restoreID,
		BackupName: backupName,
		Status:     RestoreStatusRunning,
		StartTime:  time.Now(),
		Components: make([]ComponentRestoreInfo, 0),
	}
	rm.currentRestore = info
	rm.mu.Unlock()

	// Create timeout context
	var cancel context.CancelFunc
	if rm.config.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, rm.config.Timeout)
		defer cancel()
	}

	// Create temporary directory for restore
	tmpDir, err := os.MkdirTemp("", "kscore-restore-*")
	if err != nil {
		return rm.failRestore(info, fmt.Errorf("failed to create temp directory: %w", err))
	}
	defer os.RemoveAll(tmpDir)

	rm.updateProgress("downloading", "", 0, 0, 0, 0, "Downloading backup")

	// Download backup
	backupPath := filepath.Join(tmpDir, backupName)
	backupFile, err := os.Create(backupPath)
	if err != nil {
		return rm.failRestore(info, fmt.Errorf("failed to create backup file: %w", err))
	}

	if err := rm.source.Download(ctx, backupName, backupFile); err != nil {
		backupFile.Close()
		return rm.failRestore(info, fmt.Errorf("failed to download backup: %w", err))
	}
	backupFile.Close()

	// Decrypt if needed
	artifactPath := backupPath
	if rm.encryptor != nil && rm.encryptor.Type() != EncryptionTypeNone {
		rm.updateProgress("decrypting", "", 0, 0, 0, 0, "Decrypting backup")

		decryptedPath := backupPath + ".dec"
		if err := rm.encryptor.DecryptFile(ctx, backupPath, decryptedPath); err != nil {
			return rm.failRestore(info, fmt.Errorf("failed to decrypt backup: %w", err))
		}
		os.Remove(backupPath)
		artifactPath = decryptedPath
	}

	// Read manifest
	rm.updateProgress("reading", "", 0, 0, 0, 0, "Reading backup manifest")
	reader := NewArtifactReader(artifactPath, rm.logger)
	manifest, err := reader.ReadManifest(ctx)
	if err != nil {
		return rm.failRestore(info, fmt.Errorf("failed to read manifest: %w", err))
	}

	info.BackupID = manifest.Backup.ID

	// Verify integrity if enabled
	if rm.config.VerifyIntegrity {
		rm.updateProgress("verifying", "", 0, 0, 0, 0, "Verifying backup integrity")
		info.Status = RestoreStatusVerifying

		verification, err := reader.VerifyIntegrity(ctx)
		if err != nil {
			return rm.failRestore(info, fmt.Errorf("integrity verification failed: %w", err))
		}
		if !verification.Valid {
			return rm.failRestore(info, fmt.Errorf("backup integrity check failed: %d files corrupted", verification.FailedFiles))
		}
	}

	info.Status = RestoreStatusRunning

	// Extract backup
	rm.updateProgress("extracting", "", 0, 0, 0, 0, "Extracting backup")
	extractDir := filepath.Join(tmpDir, "extracted")
	if err := reader.Extract(ctx, extractDir); err != nil {
		return rm.failRestore(info, fmt.Errorf("failed to extract backup: %w", err))
	}

	// Determine components to restore
	components := rm.config.Components
	if len(components) == 0 {
		// Restore all components from the backup
		for _, comp := range manifest.Backup.Components {
			components = append(components, comp.Type)
		}
	}

	rm.updateProgress("restoring", "", len(components), 0, 0, 0, "Starting restore")

	// Dry run - just verify importers exist
	if rm.config.DryRun {
		for _, component := range components {
			rm.mu.RLock()
			_, ok := rm.importers[component]
			rm.mu.RUnlock()

			compInfo := ComponentRestoreInfo{
				Type:   component,
				Status: RestoreStatusCompleted,
			}
			if !ok {
				compInfo.Status = RestoreStatusFailed
				compInfo.Error = "no importer registered"
			}
			info.Components = append(info.Components, compInfo)
		}

		info.Status = RestoreStatusCompleted
		info.EndTime = time.Now()
		info.Duration = info.EndTime.Sub(info.StartTime)

		rm.logger.Info("restore dry run completed", "backup", backupName, "components", len(components))
		return info, nil
	}

	// Restore each component
	var totalRestored int64
	for i, component := range components {
		select {
		case <-ctx.Done():
			return rm.failRestore(info, ctx.Err())
		default:
		}

		rm.updateProgress("restoring", component, len(components), i, totalRestored, 0,
			fmt.Sprintf("Restoring %s", component))

		compInfo, err := rm.restoreComponent(ctx, component, extractDir)
		if err != nil {
			rm.logger.Error("failed to restore component", "component", component, "error", err)
			compInfo = ComponentRestoreInfo{
				Type:   component,
				Status: RestoreStatusFailed,
				Error:  err.Error(),
			}
		}

		info.Components = append(info.Components, compInfo)
		totalRestored += compInfo.Size
	}

	// Mark as completed
	info.Status = RestoreStatusCompleted
	info.EndTime = time.Now()
	info.Duration = info.EndTime.Sub(info.StartTime)

	rm.mu.Lock()
	rm.currentRestore = nil
	rm.restoreHistory = append(rm.restoreHistory, *info)
	rm.mu.Unlock()

	rm.updateProgress("completed", "", len(components), len(components), totalRestored, 100, "Restore completed")

	rm.logger.Info("restore completed",
		"id", info.ID,
		"backup", backupName,
		"duration", info.Duration,
		"components", len(info.Components))

	return info, nil
}

// restoreComponent restores a single component
func (rm *RestoreManager) restoreComponent(ctx context.Context, component ComponentType, extractDir string) (ComponentRestoreInfo, error) {
	startTime := time.Now()

	compInfo := ComponentRestoreInfo{
		Type:   component,
		Status: RestoreStatusRunning,
	}

	rm.mu.RLock()
	importer, ok := rm.importers[component]
	rm.mu.RUnlock()

	if !ok {
		// No importer registered - skip this component
		rm.logger.Warn("no importer registered for component", "component", component)
		compInfo.Status = RestoreStatusCompleted
		compInfo.Duration = time.Since(startTime)
		return compInfo, nil
	}

	// Find component data file
	dataPath := filepath.Join(extractDir, string(component), "data")
	if _, err := os.Stat(dataPath); os.IsNotExist(err) {
		compInfo.Status = RestoreStatusFailed
		compInfo.Error = "component data not found in backup"
		compInfo.Duration = time.Since(startTime)
		return compInfo, fmt.Errorf("component data not found: %s", component)
	}

	// Get file size
	fileInfo, _ := os.Stat(dataPath)
	if fileInfo != nil {
		compInfo.Size = fileInfo.Size()
	}

	// Open data file
	file, err := os.Open(dataPath)
	if err != nil {
		compInfo.Status = RestoreStatusFailed
		compInfo.Error = err.Error()
		compInfo.Duration = time.Since(startTime)
		return compInfo, err
	}
	defer file.Close()

	// Import data
	if err := importer.Import(ctx, file); err != nil {
		compInfo.Status = RestoreStatusFailed
		compInfo.Error = err.Error()
		compInfo.Duration = time.Since(startTime)
		return compInfo, err
	}

	// Verify import
	if err := importer.Verify(ctx); err != nil {
		compInfo.Status = RestoreStatusFailed
		compInfo.Error = "verification failed: " + err.Error()
		compInfo.Duration = time.Since(startTime)
		return compInfo, err
	}

	compInfo.Status = RestoreStatusCompleted
	compInfo.Duration = time.Since(startTime)

	rm.logger.Debug("restored component",
		"component", component,
		"size", compInfo.Size,
		"duration", compInfo.Duration)

	return compInfo, nil
}

// failRestore marks a restore as failed
func (rm *RestoreManager) failRestore(info *RestoreInfo, err error) (*RestoreInfo, error) {
	info.Status = RestoreStatusFailed
	info.EndTime = time.Now()
	info.Duration = info.EndTime.Sub(info.StartTime)
	info.Error = err.Error()

	rm.mu.Lock()
	rm.currentRestore = nil
	rm.restoreHistory = append(rm.restoreHistory, *info)
	rm.mu.Unlock()

	rm.logger.Error("restore failed", "id", info.ID, "error", err)
	return info, err
}

// updateProgress sends a progress update
func (rm *RestoreManager) updateProgress(phase string, component ComponentType, total, completed int, bytes int64, percent int, message string) {
	rm.mu.RLock()
	cb := rm.progressCallback
	rm.mu.RUnlock()

	if cb != nil {
		progress := &RestoreProgress{
			Phase:               phase,
			CurrentComponent:    component,
			TotalComponents:     total,
			CompletedComponents: completed,
			BytesRestored:       bytes,
			PercentComplete:     percent,
			Message:             message,
		}
		cb(progress)
	}
}

// CurrentRestore returns the currently running restore, if any
func (rm *RestoreManager) CurrentRestore() *RestoreInfo {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.currentRestore
}

// RestoreHistory returns the history of completed restores
func (rm *RestoreManager) RestoreHistory() []RestoreInfo {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	result := make([]RestoreInfo, len(rm.restoreHistory))
	copy(result, rm.restoreHistory)
	return result
}

// ListAvailableBackups lists all available backups that can be restored
func (rm *RestoreManager) ListAvailableBackups(ctx context.Context) ([]Info, error) {
	if rm.source == nil {
		return nil, fmt.Errorf("no backup source configured")
	}
	return rm.source.List(ctx)
}

// GetBackupManifest retrieves the manifest for a specific backup
func (rm *RestoreManager) GetBackupManifest(ctx context.Context, backupName string) (*Manifest, error) {
	if rm.source == nil {
		return nil, fmt.Errorf("no backup source configured")
	}

	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "kscore-manifest-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	// Download backup
	backupPath := filepath.Join(tmpDir, backupName)
	backupFile, err := os.Create(backupPath)
	if err != nil {
		return nil, err
	}

	if err := rm.source.Download(ctx, backupName, backupFile); err != nil {
		backupFile.Close()
		return nil, err
	}
	backupFile.Close()

	// Decrypt if needed
	artifactPath := backupPath
	if rm.encryptor != nil && rm.encryptor.Type() != EncryptionTypeNone {
		decryptedPath := backupPath + ".dec"
		if err := rm.encryptor.DecryptFile(ctx, backupPath, decryptedPath); err != nil {
			return nil, err
		}
		os.Remove(backupPath)
		artifactPath = decryptedPath
	}

	// Read manifest
	reader := NewArtifactReader(artifactPath, rm.logger)
	return reader.ReadManifest(ctx)
}

// RestoreOptions holds options for a restore operation
type RestoreOptions struct {
	BackupName      string
	Components      []ComponentType
	VerifyIntegrity bool
	DryRun          bool
	PreRestoreHook  func(ctx context.Context, manifest *Manifest) error
	PostRestoreHook func(ctx context.Context, result *RestoreInfo) error
}

// RestoreWithOptions performs a restore with custom options
func (rm *RestoreManager) RestoreWithOptions(ctx context.Context, opts RestoreOptions) (*RestoreInfo, error) {
	// Override config with options
	if len(opts.Components) > 0 {
		rm.config.Components = opts.Components
	}
	rm.config.VerifyIntegrity = opts.VerifyIntegrity
	rm.config.DryRun = opts.DryRun

	// Run pre-restore hook
	if opts.PreRestoreHook != nil {
		manifest, err := rm.GetBackupManifest(ctx, opts.BackupName)
		if err != nil {
			return nil, fmt.Errorf("failed to get manifest for pre-restore hook: %w", err)
		}
		if err := opts.PreRestoreHook(ctx, manifest); err != nil {
			return nil, fmt.Errorf("pre-restore hook failed: %w", err)
		}
	}

	// Perform restore
	result, err := rm.Restore(ctx, opts.BackupName)

	// Run post-restore hook (even if restore failed)
	if opts.PostRestoreHook != nil {
		if hookErr := opts.PostRestoreHook(ctx, result); hookErr != nil {
			rm.logger.Error("post-restore hook failed", "error", hookErr)
		}
	}

	return result, err
}

// PointInTimeRestore performs a point-in-time restore (finding the best backup)
func (rm *RestoreManager) PointInTimeRestore(ctx context.Context, targetTime time.Time) (*RestoreInfo, error) {
	// List available backups
	backups, err := rm.ListAvailableBackups(ctx)
	if err != nil {
		return nil, err
	}

	// Find the best backup (most recent before target time)
	var bestBackup *Info
	for i := range backups {
		if backups[i].EndTime.Before(targetTime) || backups[i].EndTime.Equal(targetTime) {
			if bestBackup == nil || backups[i].EndTime.After(bestBackup.EndTime) {
				bestBackup = &backups[i]
			}
		}
	}

	if bestBackup == nil {
		return nil, fmt.Errorf("no backup found before %s", targetTime.Format(time.RFC3339))
	}

	rm.logger.Info("selected backup for point-in-time restore",
		"target_time", targetTime,
		"backup_name", bestBackup.Name,
		"backup_time", bestBackup.EndTime)

	return rm.Restore(ctx, bestBackup.Name)
}

// StreamRestore streams restore data directly to an importer without full download
type StreamRestore struct {
	rm *RestoreManager
}

// NewStreamRestore creates a new stream restore
func NewStreamRestore(rm *RestoreManager) *StreamRestore {
	return &StreamRestore{rm: rm}
}

// RestoreComponentStream restores a single component by streaming
func (sr *StreamRestore) RestoreComponentStream(ctx context.Context, backupName string, component ComponentType, w io.Writer) error {
	// Create temporary directory for the backup
	tmpDir, err := os.MkdirTemp("", "kscore-stream-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	// Download backup
	backupPath := filepath.Join(tmpDir, backupName)
	backupFile, err := os.Create(backupPath)
	if err != nil {
		return err
	}

	if err := sr.rm.source.Download(ctx, backupName, backupFile); err != nil {
		backupFile.Close()
		return err
	}
	backupFile.Close()

	// Decrypt if needed
	artifactPath := backupPath
	if sr.rm.encryptor != nil && sr.rm.encryptor.Type() != EncryptionTypeNone {
		decryptedPath := backupPath + ".dec"
		if err := sr.rm.encryptor.DecryptFile(ctx, backupPath, decryptedPath); err != nil {
			return err
		}
		os.Remove(backupPath)
		artifactPath = decryptedPath
	}

	// Extract the specific component data
	reader := NewArtifactReader(artifactPath, sr.rm.logger)
	dataPath := filepath.Join(string(component), "data")
	return reader.ExtractFile(ctx, dataPath, w)
}
