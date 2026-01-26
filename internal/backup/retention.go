package backup

import (
	"context"
	"sort"
	"time"
)

// RetentionManager manages backup retention policies
type RetentionManager struct {
	destination Destination
	config      *RetentionConfig
	logger      Logger
}

// NewRetentionManager creates a new retention manager
func NewRetentionManager(dest Destination, config *RetentionConfig, logger Logger) *RetentionManager {
	if logger == nil {
		logger = &noopLogger{}
	}
	if config == nil {
		config = DefaultRetentionConfig()
	}
	return &RetentionManager{
		destination: dest,
		config:      config,
		logger:      logger,
	}
}

// DefaultRetentionConfig returns default retention configuration
func DefaultRetentionConfig() *RetentionConfig {
	return &RetentionConfig{
		MaxBackups:  10,
		MaxAge:      30 * 24 * time.Hour, // 30 days
		KeepDaily:   7,
		KeepWeekly:  4,
		KeepMonthly: 3,
		KeepYearly:  1,
	}
}

// Apply applies the retention policy, returning list of deleted backups
func (m *RetentionManager) Apply(ctx context.Context) ([]BackupInfo, error) {
	// Get all backups
	backups, err := m.destination.List(ctx)
	if err != nil {
		return nil, err
	}

	// Sort by time, newest first
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].EndTime.After(backups[j].EndTime)
	})

	// Determine which backups to keep
	toKeep := m.selectBackupsToKeep(backups)

	// Build map of backups to keep
	keepMap := make(map[string]bool)
	for _, b := range toKeep {
		keepMap[b.Name] = true
	}

	// Delete backups not in keep list
	var deleted []BackupInfo
	for _, backup := range backups {
		if keepMap[backup.Name] {
			continue
		}

		m.logger.Debug("deleting backup due to retention policy", "name", backup.Name)
		if err := m.destination.Delete(ctx, backup.Name); err != nil {
			m.logger.Error("failed to delete backup", "name", backup.Name, "error", err)
			continue
		}
		deleted = append(deleted, backup)
	}

	return deleted, nil
}

// selectBackupsToKeep selects backups to keep based on retention policy
func (m *RetentionManager) selectBackupsToKeep(backups []BackupInfo) []BackupInfo {
	now := time.Now()
	var toKeep []BackupInfo
	keepMap := make(map[string]bool)

	// Helper to add backup if not already added
	addBackup := func(b BackupInfo) {
		if !keepMap[b.Name] {
			keepMap[b.Name] = true
			toKeep = append(toKeep, b)
		}
	}

	// Apply MaxBackups limit (always keep the most recent N backups)
	if m.config.MaxBackups > 0 {
		for i, backup := range backups {
			if i >= m.config.MaxBackups {
				break
			}
			addBackup(backup)
		}
	}

	// Apply MaxAge filter (keep all backups newer than max age)
	if m.config.MaxAge > 0 {
		cutoff := now.Add(-m.config.MaxAge)
		for _, backup := range backups {
			if backup.EndTime.After(cutoff) {
				addBackup(backup)
			}
		}
	}

	// Apply daily retention
	if m.config.KeepDaily > 0 {
		daily := m.selectByPeriod(backups, m.config.KeepDaily, func(t time.Time) string {
			return t.Format("2006-01-02")
		})
		for _, b := range daily {
			addBackup(b)
		}
	}

	// Apply weekly retention
	if m.config.KeepWeekly > 0 {
		weekly := m.selectByPeriod(backups, m.config.KeepWeekly, func(t time.Time) string {
			year, week := t.ISOWeek()
			return time.Date(year, 0, 0, 0, 0, 0, 0, time.UTC).AddDate(0, 0, week*7).Format("2006-W02")
		})
		for _, b := range weekly {
			addBackup(b)
		}
	}

	// Apply monthly retention
	if m.config.KeepMonthly > 0 {
		monthly := m.selectByPeriod(backups, m.config.KeepMonthly, func(t time.Time) string {
			return t.Format("2006-01")
		})
		for _, b := range monthly {
			addBackup(b)
		}
	}

	// Apply yearly retention
	if m.config.KeepYearly > 0 {
		yearly := m.selectByPeriod(backups, m.config.KeepYearly, func(t time.Time) string {
			return t.Format("2006")
		})
		for _, b := range yearly {
			addBackup(b)
		}
	}

	return toKeep
}

// selectByPeriod selects the newest backup for each period
func (m *RetentionManager) selectByPeriod(backups []BackupInfo, count int, periodFunc func(time.Time) string) []BackupInfo {
	periodBackups := make(map[string]BackupInfo)
	var periods []string

	// Group backups by period
	for _, backup := range backups {
		period := periodFunc(backup.EndTime)
		if _, exists := periodBackups[period]; !exists {
			periodBackups[period] = backup
			periods = append(periods, period)
		}
	}

	// Sort periods newest first
	sort.Slice(periods, func(i, j int) bool {
		return periods[i] > periods[j]
	})

	// Take the first N periods
	var result []BackupInfo
	for i, period := range periods {
		if i >= count {
			break
		}
		result = append(result, periodBackups[period])
	}

	return result
}

// Preview returns a preview of what would be deleted without actually deleting
func (m *RetentionManager) Preview(ctx context.Context) (*RetentionPreview, error) {
	// Get all backups
	backups, err := m.destination.List(ctx)
	if err != nil {
		return nil, err
	}

	// Sort by time, newest first
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].EndTime.After(backups[j].EndTime)
	})

	// Determine which backups to keep
	toKeep := m.selectBackupsToKeep(backups)

	// Build map of backups to keep
	keepMap := make(map[string]bool)
	for _, b := range toKeep {
		keepMap[b.Name] = true
	}

	// Categorize backups
	preview := &RetentionPreview{
		TotalBackups: len(backups),
		ToKeep:       make([]BackupInfo, 0),
		ToDelete:     make([]BackupInfo, 0),
	}

	var keepSize, deleteSize int64
	for _, backup := range backups {
		if keepMap[backup.Name] {
			preview.ToKeep = append(preview.ToKeep, backup)
			keepSize += backup.Size
		} else {
			preview.ToDelete = append(preview.ToDelete, backup)
			deleteSize += backup.Size
		}
	}

	preview.KeepCount = len(preview.ToKeep)
	preview.DeleteCount = len(preview.ToDelete)
	preview.KeepSize = keepSize
	preview.DeleteSize = deleteSize

	return preview, nil
}

// RetentionPreview shows what would happen if retention policy is applied
type RetentionPreview struct {
	TotalBackups int          `json:"total_backups"`
	KeepCount    int          `json:"keep_count"`
	DeleteCount  int          `json:"delete_count"`
	KeepSize     int64        `json:"keep_size"`
	DeleteSize   int64        `json:"delete_size"`
	ToKeep       []BackupInfo `json:"to_keep"`
	ToDelete     []BackupInfo `json:"to_delete"`
}

// ScheduledRetention runs retention on a schedule
type ScheduledRetention struct {
	manager   *RetentionManager
	interval  time.Duration
	stopCh    chan struct{}
	logger    Logger
	onApplied func(deleted []BackupInfo, err error)
}

// NewScheduledRetention creates a scheduled retention runner
func NewScheduledRetention(manager *RetentionManager, interval time.Duration, logger Logger) *ScheduledRetention {
	if logger == nil {
		logger = &noopLogger{}
	}
	return &ScheduledRetention{
		manager:  manager,
		interval: interval,
		stopCh:   make(chan struct{}),
		logger:   logger,
	}
}

// SetCallback sets a callback to be called after each retention run
func (s *ScheduledRetention) SetCallback(fn func(deleted []BackupInfo, err error)) {
	s.onApplied = fn
}

// Start starts the scheduled retention
func (s *ScheduledRetention) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-s.stopCh:
				return
			case <-ticker.C:
				s.logger.Debug("running scheduled retention")
				deleted, err := s.manager.Apply(ctx)
				if s.onApplied != nil {
					s.onApplied(deleted, err)
				}
				if err != nil {
					s.logger.Error("scheduled retention failed", "error", err)
				} else if len(deleted) > 0 {
					s.logger.Info("scheduled retention completed", "deleted", len(deleted))
				}
			}
		}
	}()
}

// Stop stops the scheduled retention
func (s *ScheduledRetention) Stop() {
	close(s.stopCh)
}

// PerTypeRetentionConfig allows different retention for different backup types
type PerTypeRetentionConfig struct {
	Default      *RetentionConfig
	Full         *RetentionConfig
	Incremental  *RetentionConfig
	Database     *RetentionConfig
	Config       *RetentionConfig
}

// PerTypeRetentionManager manages retention with different policies per backup type
type PerTypeRetentionManager struct {
	destination Destination
	config      *PerTypeRetentionConfig
	logger      Logger
}

// NewPerTypeRetentionManager creates a per-type retention manager
func NewPerTypeRetentionManager(dest Destination, config *PerTypeRetentionConfig, logger Logger) *PerTypeRetentionManager {
	if logger == nil {
		logger = &noopLogger{}
	}
	if config == nil {
		config = &PerTypeRetentionConfig{
			Default: DefaultRetentionConfig(),
		}
	}
	return &PerTypeRetentionManager{
		destination: dest,
		config:      config,
		logger:      logger,
	}
}

// Apply applies per-type retention policies
func (m *PerTypeRetentionManager) Apply(ctx context.Context) ([]BackupInfo, error) {
	// Get all backups
	backups, err := m.destination.List(ctx)
	if err != nil {
		return nil, err
	}

	// Group backups by type
	byType := make(map[BackupType][]BackupInfo)
	for _, backup := range backups {
		byType[backup.Type] = append(byType[backup.Type], backup)
	}

	// Apply retention to each type
	var allDeleted []BackupInfo
	for backupType, typeBackups := range byType {
		config := m.getConfigForType(backupType)
		if config == nil {
			continue
		}

		// Create a temporary manager for this type
		tempManager := &RetentionManager{
			config: config,
			logger: m.logger,
		}

		toKeep := tempManager.selectBackupsToKeep(typeBackups)
		keepMap := make(map[string]bool)
		for _, b := range toKeep {
			keepMap[b.Name] = true
		}

		// Delete backups not in keep list
		for _, backup := range typeBackups {
			if keepMap[backup.Name] {
				continue
			}

			m.logger.Debug("deleting backup due to retention policy", "name", backup.Name, "type", backupType)
			if err := m.destination.Delete(ctx, backup.Name); err != nil {
				m.logger.Error("failed to delete backup", "name", backup.Name, "error", err)
				continue
			}
			allDeleted = append(allDeleted, backup)
		}
	}

	return allDeleted, nil
}

// getConfigForType returns the retention config for a backup type
func (m *PerTypeRetentionManager) getConfigForType(backupType BackupType) *RetentionConfig {
	switch backupType {
	case BackupTypeFull:
		if m.config.Full != nil {
			return m.config.Full
		}
	case BackupTypeIncremental:
		if m.config.Incremental != nil {
			return m.config.Incremental
		}
	case BackupTypeDatabase:
		if m.config.Database != nil {
			return m.config.Database
		}
	case BackupTypeConfiguration:
		if m.config.Config != nil {
			return m.config.Config
		}
	}
	return m.config.Default
}
