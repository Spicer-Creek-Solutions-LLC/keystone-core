package backup

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// BackupManager orchestrates backup operations
type BackupManager struct {
	config           *BackupConfig
	exporters        map[ComponentType]Exporter
	destination      Destination
	encryptor        Encryptor
	logger           Logger
	progressCallback BackupProgressCallback

	mu              sync.RWMutex
	currentBackup   *BackupInfo
	backupHistory   []BackupInfo
}

// NewBackupManager creates a new backup manager
func NewBackupManager(config *BackupConfig, logger Logger) (*BackupManager, error) {
	if config == nil {
		config = DefaultBackupConfig()
	}

	if logger == nil {
		logger = &noopLogger{}
	}

	bm := &BackupManager{
		config:        config,
		exporters:     make(map[ComponentType]Exporter),
		logger:        logger,
		backupHistory: make([]BackupInfo, 0),
	}

	return bm, nil
}

// SetDestination sets the backup destination
func (bm *BackupManager) SetDestination(dest Destination) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	bm.destination = dest
}

// SetEncryptor sets the encryptor for backup encryption
func (bm *BackupManager) SetEncryptor(enc Encryptor) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	bm.encryptor = enc
}

// RegisterExporter registers an exporter for a component type
func (bm *BackupManager) RegisterExporter(exporter Exporter) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	bm.exporters[exporter.Component()] = exporter
}

// SetProgressCallback sets the callback for progress updates
func (bm *BackupManager) SetProgressCallback(cb BackupProgressCallback) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	bm.progressCallback = cb
}

// Backup performs a backup operation
func (bm *BackupManager) Backup(ctx context.Context) (*BackupInfo, error) {
	bm.mu.Lock()
	if bm.currentBackup != nil && bm.currentBackup.Status == BackupStatusRunning {
		bm.mu.Unlock()
		return nil, fmt.Errorf("backup already in progress")
	}

	// Generate backup ID
	backupID, err := generateBackupID()
	if err != nil {
		bm.mu.Unlock()
		return nil, fmt.Errorf("failed to generate backup ID: %w", err)
	}
	backupName := fmt.Sprintf("kscore-backup-%s", time.Now().Format("20060102-150405"))

	info := &BackupInfo{
		ID:          backupID,
		Name:        backupName,
		Type:        bm.config.Type,
		Status:      BackupStatusRunning,
		StartTime:   time.Now(),
		Encrypted:   bm.config.Encryption.Type != EncryptionTypeNone,
		EncryptionType: bm.config.Encryption.Type,
		Components:  make([]ComponentBackupInfo, 0),
		Metadata:    make(map[string]string),
	}
	bm.currentBackup = info
	bm.mu.Unlock()

	// Create timeout context
	var cancel context.CancelFunc
	if bm.config.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, bm.config.Timeout)
		defer cancel()
	}

	// Create temporary directory for backup
	tmpDir, err := os.MkdirTemp("", "kscore-backup-*")
	if err != nil {
		return bm.failBackup(info, fmt.Errorf("failed to create temp directory: %w", err))
	}
	defer os.RemoveAll(tmpDir)

	bm.updateProgress("exporting", "", len(bm.config.Components), 0, 0, 0, "Starting backup")

	// Export each component
	var totalSize int64
	componentInfos := make([]ComponentBackupInfo, 0, len(bm.config.Components))

	for i, component := range bm.config.Components {
		select {
		case <-ctx.Done():
			return bm.failBackup(info, ctx.Err())
		default:
		}

		bm.updateProgress("exporting", component, len(bm.config.Components), i, totalSize, 0,
			fmt.Sprintf("Exporting %s", component))

		compInfo, err := bm.exportComponent(ctx, component, tmpDir)
		if err != nil {
			bm.logger.Error("failed to export component", "component", component, "error", err)
			compInfo = ComponentBackupInfo{
				Type:   component,
				Status: BackupStatusFailed,
				Error:  err.Error(),
			}
		}

		componentInfos = append(componentInfos, compInfo)
		totalSize += compInfo.Size
	}

	info.Components = componentInfos

	// Build artifact
	bm.updateProgress("packaging", "", len(bm.config.Components), len(bm.config.Components),
		totalSize, 0, "Creating backup artifact")

	artifactPath := filepath.Join(tmpDir, backupName+".tar.gz")
	artifact, err := NewArtifactBuilder(tmpDir, bm.logger)
	if err != nil {
		return bm.failBackup(info, fmt.Errorf("failed to create artifact builder: %w", err))
	}

	// Create manifest
	manifest := &BackupManifest{
		ManifestVersion: ManifestVersion,
		Backup:          *info,
		Files:           make([]ManifestFile, 0),
		SchemaVersions:  make(map[ComponentType]string),
		CreatedAt:       time.Now(),
	}

	if err := artifact.Build(ctx, artifactPath, manifest, bm.config.Compression); err != nil {
		return bm.failBackup(info, fmt.Errorf("failed to build artifact: %w", err))
	}

	// Get artifact info
	artifactInfo, err := os.Stat(artifactPath)
	if err != nil {
		return bm.failBackup(info, fmt.Errorf("failed to stat artifact: %w", err))
	}
	info.Size = artifactInfo.Size()

	// Calculate checksum
	checksum, err := calculateFileChecksum(artifactPath)
	if err != nil {
		return bm.failBackup(info, fmt.Errorf("failed to calculate checksum: %w", err))
	}
	info.Checksum = checksum

	// Encrypt if configured
	if bm.encryptor != nil && bm.config.Encryption.Type != EncryptionTypeNone {
		bm.updateProgress("encrypting", "", len(bm.config.Components), len(bm.config.Components),
			totalSize, 0, "Encrypting backup")

		encryptedPath := artifactPath + ".enc"
		if err := bm.encryptor.EncryptFile(ctx, artifactPath, encryptedPath); err != nil {
			return bm.failBackup(info, fmt.Errorf("failed to encrypt artifact: %w", err))
		}

		// Update to encrypted artifact
		os.Remove(artifactPath)
		artifactPath = encryptedPath

		// Update size and checksum
		encInfo, _ := os.Stat(artifactPath)
		if encInfo != nil {
			info.Size = encInfo.Size()
		}
		checksum, _ := calculateFileChecksum(artifactPath)
		info.Checksum = checksum
	}

	// Upload to destination
	if bm.destination != nil {
		bm.updateProgress("uploading", "", len(bm.config.Components), len(bm.config.Components),
			totalSize, 0, "Uploading backup")

		file, err := os.Open(artifactPath)
		if err != nil {
			return bm.failBackup(info, fmt.Errorf("failed to open artifact: %w", err))
		}
		defer file.Close()

		destName := filepath.Base(artifactPath)
		if err := bm.destination.Upload(ctx, destName, file, info.Size); err != nil {
			return bm.failBackup(info, fmt.Errorf("failed to upload artifact: %w", err))
		}

		info.Destination = fmt.Sprintf("%s/%s", bm.config.Destination.Path, destName)
	} else {
		// Local backup - move to destination path
		destPath := filepath.Join(bm.config.Destination.Path, filepath.Base(artifactPath))
		if err := os.MkdirAll(bm.config.Destination.Path, 0755); err != nil {
			return bm.failBackup(info, fmt.Errorf("failed to create destination directory: %w", err))
		}
		if err := copyFile(artifactPath, destPath); err != nil {
			return bm.failBackup(info, fmt.Errorf("failed to copy artifact: %w", err))
		}
		info.Destination = destPath
	}

	// Mark as completed
	info.Status = BackupStatusCompleted
	info.EndTime = time.Now()
	info.Duration = info.EndTime.Sub(info.StartTime)

	bm.mu.Lock()
	bm.currentBackup = nil
	bm.backupHistory = append(bm.backupHistory, *info)
	bm.mu.Unlock()

	bm.updateProgress("completed", "", len(bm.config.Components), len(bm.config.Components),
		info.Size, 100, "Backup completed")

	bm.logger.Info("backup completed",
		"id", info.ID,
		"duration", info.Duration,
		"size", info.Size,
		"destination", info.Destination)

	return info, nil
}

// exportComponent exports a single component
func (bm *BackupManager) exportComponent(ctx context.Context, component ComponentType, tmpDir string) (ComponentBackupInfo, error) {
	startTime := time.Now()

	info := ComponentBackupInfo{
		Type:   component,
		Status: BackupStatusRunning,
	}

	bm.mu.RLock()
	exporter, ok := bm.exporters[component]
	bm.mu.RUnlock()

	if !ok {
		// No exporter registered - skip this component
		bm.logger.Warn("no exporter registered for component", "component", component)
		info.Status = BackupStatusCompleted
		info.Duration = time.Since(startTime)
		return info, nil
	}

	// Create component directory
	componentDir := filepath.Join(tmpDir, string(component))
	if err := os.MkdirAll(componentDir, 0755); err != nil {
		info.Status = BackupStatusFailed
		info.Error = err.Error()
		info.Duration = time.Since(startTime)
		return info, err
	}

	// Create output file
	outputPath := filepath.Join(componentDir, "data")
	file, err := os.Create(outputPath)
	if err != nil {
		info.Status = BackupStatusFailed
		info.Error = err.Error()
		info.Duration = time.Since(startTime)
		return info, err
	}
	defer file.Close()

	// Export data
	if err := exporter.Export(ctx, file); err != nil {
		info.Status = BackupStatusFailed
		info.Error = err.Error()
		info.Duration = time.Since(startTime)
		return info, err
	}

	// Get file info
	fileInfo, err := file.Stat()
	if err != nil {
		info.Status = BackupStatusFailed
		info.Error = err.Error()
		info.Duration = time.Since(startTime)
		return info, err
	}

	// Calculate checksum
	file.Seek(0, 0)
	checksum, err := calculateFileChecksum(outputPath)
	if err != nil {
		info.Status = BackupStatusFailed
		info.Error = err.Error()
		info.Duration = time.Since(startTime)
		return info, err
	}

	info.Status = BackupStatusCompleted
	info.Size = fileInfo.Size()
	info.Checksum = checksum
	info.Duration = time.Since(startTime)

	bm.logger.Debug("exported component",
		"component", component,
		"size", info.Size,
		"duration", info.Duration)

	return info, nil
}

// failBackup marks a backup as failed
func (bm *BackupManager) failBackup(info *BackupInfo, err error) (*BackupInfo, error) {
	info.Status = BackupStatusFailed
	info.EndTime = time.Now()
	info.Duration = info.EndTime.Sub(info.StartTime)
	info.Error = err.Error()

	bm.mu.Lock()
	bm.currentBackup = nil
	bm.backupHistory = append(bm.backupHistory, *info)
	bm.mu.Unlock()

	bm.logger.Error("backup failed", "id", info.ID, "error", err)
	return info, err
}

// updateProgress sends a progress update
func (bm *BackupManager) updateProgress(phase string, component ComponentType, total, completed int, bytes int64, percent int, message string) {
	bm.mu.RLock()
	cb := bm.progressCallback
	bm.mu.RUnlock()

	if cb != nil {
		progress := &BackupProgress{
			Phase:               phase,
			CurrentComponent:    component,
			TotalComponents:     total,
			CompletedComponents: completed,
			BytesProcessed:      bytes,
			PercentComplete:     percent,
			Message:             message,
		}
		cb(progress)
	}
}

// CurrentBackup returns the currently running backup, if any
func (bm *BackupManager) CurrentBackup() *BackupInfo {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	return bm.currentBackup
}

// BackupHistory returns the history of completed backups
func (bm *BackupManager) BackupHistory() []BackupInfo {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	result := make([]BackupInfo, len(bm.backupHistory))
	copy(result, bm.backupHistory)
	return result
}

// GetBackup retrieves a specific backup by ID
func (bm *BackupManager) GetBackup(id string) (*BackupInfo, error) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	for _, info := range bm.backupHistory {
		if info.ID == id {
			return &info, nil
		}
	}

	return nil, fmt.Errorf("backup not found: %s", id)
}

// ListBackups lists all backups from the destination
func (bm *BackupManager) ListBackups(ctx context.Context) ([]BackupInfo, error) {
	if bm.destination == nil {
		// Return local history
		return bm.BackupHistory(), nil
	}

	return bm.destination.List(ctx)
}

// DeleteBackup deletes a backup
func (bm *BackupManager) DeleteBackup(ctx context.Context, id string) error {
	if bm.destination == nil {
		return fmt.Errorf("no destination configured")
	}

	// Find the backup
	backup, err := bm.GetBackup(id)
	if err != nil {
		return err
	}

	// Delete from destination
	if err := bm.destination.Delete(ctx, filepath.Base(backup.Destination)); err != nil {
		return fmt.Errorf("failed to delete backup: %w", err)
	}

	// Remove from history
	bm.mu.Lock()
	defer bm.mu.Unlock()
	for i, info := range bm.backupHistory {
		if info.ID == id {
			bm.backupHistory = append(bm.backupHistory[:i], bm.backupHistory[i+1:]...)
			break
		}
	}

	bm.logger.Info("deleted backup", "id", id)
	return nil
}

// generateBackupID generates a unique backup ID
func generateBackupID() (string, error) {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate backup ID: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// noopLogger is a no-op logger implementation
type noopLogger struct{}

func (l *noopLogger) Debug(msg string, args ...any) {}
func (l *noopLogger) Info(msg string, args ...any)  {}
func (l *noopLogger) Warn(msg string, args ...any)  {}
func (l *noopLogger) Error(msg string, args ...any) {}
