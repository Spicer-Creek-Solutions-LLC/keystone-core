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
func (m *RetentionManager) Apply(ctx context.Context) ([]Info, error) {
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
	for i := range toKeep {
		keepMap[toKeep[i].Name] = true
	}

	// Delete backups not in keep list
	var deleted []Info
	for i := range backups {
		if keepMap[backups[i].Name] {
			continue
		}

		m.logger.Debug("deleting backup due to retention policy", "name", backups[i].Name)
		if err := m.destination.Delete(ctx, backups[i].Name); err != nil {
			m.logger.Error("failed to delete backup", "name", backups[i].Name, "error", err)
			continue
		}
		deleted = append(deleted, backups[i])
	}

	return deleted, nil
}

// selectBackupsToKeep selects backups to keep based on retention policy
func (m *RetentionManager) selectBackupsToKeep(backups []Info) []Info {
	now := time.Now()
	var toKeep []Info
	keepMap := make(map[string]bool)

	// Helper to add backup if not already added
	addBackup := func(b Info) {
		if !keepMap[b.Name] {
			keepMap[b.Name] = true
			toKeep = append(toKeep, b)
		}
	}

	// Apply MaxBackups limit (always keep the most recent N backups)
	if m.config.MaxBackups > 0 {
		for i := range backups {
			if i >= m.config.MaxBackups {
				break
			}
			addBackup(backups[i])
		}
	}

	// Apply MaxAge filter (keep all backups newer than max age)
	if m.config.MaxAge > 0 {
		cutoff := now.Add(-m.config.MaxAge)
		for i := range backups {
			if backups[i].EndTime.After(cutoff) {
				addBackup(backups[i])
			}
		}
	}

	// Apply daily retention
	if m.config.KeepDaily > 0 {
		daily := m.selectByPeriod(backups, m.config.KeepDaily, func(t time.Time) string {
			return t.Format("2006-01-02")
		})
		for i := range daily {
			addBackup(daily[i])
		}
	}

	// Apply weekly retention
	if m.config.KeepWeekly > 0 {
		weekly := m.selectByPeriod(backups, m.config.KeepWeekly, func(t time.Time) string {
			year, week := t.ISOWeek()
			return time.Date(year, 0, 0, 0, 0, 0, 0, time.UTC).AddDate(0, 0, week*7).Format("2006-W02")
		})
		for i := range weekly {
			addBackup(weekly[i])
		}
	}

	// Apply monthly retention
	if m.config.KeepMonthly > 0 {
		monthly := m.selectByPeriod(backups, m.config.KeepMonthly, func(t time.Time) string {
			return t.Format("2006-01")
		})
		for i := range monthly {
			addBackup(monthly[i])
		}
	}

	// Apply yearly retention
	if m.config.KeepYearly > 0 {
		yearly := m.selectByPeriod(backups, m.config.KeepYearly, func(t time.Time) string {
			return t.Format("2006")
		})
		for i := range yearly {
			addBackup(yearly[i])
		}
	}

	return toKeep
}

// selectByPeriod selects the newest backup for each period
func (m *RetentionManager) selectByPeriod(backups []Info, count int, periodFunc func(time.Time) string) []Info {
	periodBackups := make(map[string]Info)
	var periods []string

	// Group backups by period
	for i := range backups {
		period := periodFunc(backups[i].EndTime)
		if _, exists := periodBackups[period]; !exists {
			periodBackups[period] = backups[i]
			periods = append(periods, period)
		}
	}

	// Sort periods newest first
	sort.Slice(periods, func(i, j int) bool {
		return periods[i] > periods[j]
	})

	// Take the first N periods
	var result []Info
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
	for i := range toKeep {
		keepMap[toKeep[i].Name] = true
	}

	// Categorize backups
	preview := &RetentionPreview{
		TotalBackups: len(backups),
		ToKeep:       make([]Info, 0),
		ToDelete:     make([]Info, 0),
	}

	var keepSize, deleteSize int64
	for i := range backups {
		if keepMap[backups[i].Name] {
			preview.ToKeep = append(preview.ToKeep, backups[i])
			keepSize += backups[i].Size
		} else {
			preview.ToDelete = append(preview.ToDelete, backups[i])
			deleteSize += backups[i].Size
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
	ToKeep       []Info `json:"to_keep"`
	ToDelete     []Info `json:"to_delete"`
}

// ScheduledRetention runs retention on a schedule
type ScheduledRetention struct {
	manager   *RetentionManager
	interval  time.Duration
	stopCh    chan struct{}
	logger    Logger
	onApplied func(deleted []Info, err error)
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
func (s *ScheduledRetention) SetCallback(fn func(deleted []Info, err error)) {
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
	Default     *RetentionConfig
	Full        *RetentionConfig
	Incremental *RetentionConfig
	Database    *RetentionConfig
	Config      *RetentionConfig
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
func (m *PerTypeRetentionManager) Apply(ctx context.Context) ([]Info, error) {
	// Get all backups
	backups, err := m.destination.List(ctx)
	if err != nil {
		return nil, err
	}

	// Group backups by type
	byType := make(map[Type][]Info)
	for i := range backups {
		byType[backups[i].Type] = append(byType[backups[i].Type], backups[i])
	}

	// Apply retention to each type
	var allDeleted []Info
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
		for i := range toKeep {
			keepMap[toKeep[i].Name] = true
		}

		// Delete backups not in keep list
		for i := range typeBackups {
			if keepMap[typeBackups[i].Name] {
				continue
			}

			m.logger.Debug("deleting backup due to retention policy", "name", typeBackups[i].Name, "type", backupType)
			if err := m.destination.Delete(ctx, typeBackups[i].Name); err != nil {
				m.logger.Error("failed to delete backup", "name", typeBackups[i].Name, "error", err)
				continue
			}
			allDeleted = append(allDeleted, typeBackups[i])
		}
	}

	return allDeleted, nil
}

// getConfigForType returns the retention config for a backup type
func (m *PerTypeRetentionManager) getConfigForType(backupType Type) *RetentionConfig {
	switch backupType {
	case TypeFull:
		if m.config.Full != nil {
			return m.config.Full
		}
	case TypeIncremental:
		if m.config.Incremental != nil {
			return m.config.Incremental
		}
	case TypeDatabase:
		if m.config.Database != nil {
			return m.config.Database
		}
	case TypeConfiguration:
		if m.config.Config != nil {
			return m.config.Config
		}
	default:
	}
	return m.config.Default
}
