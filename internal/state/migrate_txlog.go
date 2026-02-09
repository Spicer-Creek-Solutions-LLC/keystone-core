package state

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TransactionLogEntry represents a single operation in the migration transaction log
type TransactionLogEntry struct {
	ID           string                 `json:"id"`
	Timestamp    time.Time              `json:"timestamp"`
	Table        string                 `json:"table"`
	Operation    TransactionOperation   `json:"operation"`
	RecordID     string                 `json:"record_id"`
	Status       TransactionStatus      `json:"status"`
	ErrorMessage string                 `json:"error_message,omitempty"`
	Duration     time.Duration          `json:"duration_ns"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	BatchNum     int                    `json:"batch_num"`
	BatchTotal   int                    `json:"batch_total"`
}

// TransactionOperation represents the type of migration operation
type TransactionOperation string

// OpMigrateRecord constants define the operators.
const (
	OpMigrateRecord    TransactionOperation = "migrate"
	OpValidateRecord   TransactionOperation = "validate"
	OpSkipRecord       TransactionOperation = "skip"
	OpCheckpoint       TransactionOperation = "checkpoint"
	OpRollbackStart    TransactionOperation = "rollback_start"
	OpRollbackComplete TransactionOperation = "rollback_complete"
	OpMigrationStart   TransactionOperation = "migration_start"
	OpMigrationEnd     TransactionOperation = "migration_end"
)

// TransactionStatus represents the status of a transaction entry
type TransactionStatus string

// StatusSuccess constants define the possible statuses.
const (
	StatusSuccess    TransactionStatus = "success"
	StatusFailure    TransactionStatus = "failure"
	StatusPending    TransactionStatus = "pending"
	StatusSkipped    TransactionStatus = "skipped"
	StatusRolledBack TransactionStatus = "rolled_back"
)

// TransactionLogConfig configures the transaction log behavior
type TransactionLogConfig struct {
	// LogPath is the path to the transaction log file
	LogPath string

	// FlushInterval is how often to flush the log to disk
	FlushInterval time.Duration

	// MaxEntriesInMemory is the maximum entries to keep in memory before flushing
	MaxEntriesInMemory int

	// EnableCompression enables compression for the log file
	EnableCompression bool

	// RetentionDays is how long to keep old log files (0 = forever)
	RetentionDays int

	// Verbose enables detailed logging of each operation
	Verbose bool
}

// DefaultTransactionLogConfig returns sensible defaults
func DefaultTransactionLogConfig() *TransactionLogConfig {
	return &TransactionLogConfig{
		LogPath:            "migration_txlog.json",
		FlushInterval:      5 * time.Second,
		MaxEntriesInMemory: 1000,
		EnableCompression:  false,
		RetentionDays:      30,
		Verbose:            false,
	}
}

// TransactionLog handles logging of migration operations for recovery
type TransactionLog struct {
	config *TransactionLogConfig

	mu          sync.Mutex
	entries     []*TransactionLogEntry
	file        *os.File
	encoder     *json.Encoder
	entryCount  int64
	lastFlush   time.Time
	flushTicker *time.Ticker
	stopChan    chan struct{}
	wg          sync.WaitGroup

	// Statistics
	stats TransactionLogStats
}

// TransactionLogStats tracks statistics for the transaction log
type TransactionLogStats struct {
	TotalEntries   int64
	SuccessCount   int64
	FailureCount   int64
	SkippedCount   int64
	LastEntryTime  time.Time
	FirstEntryTime time.Time
	BytesWritten   int64
	FlushCount     int64
	mu             sync.Mutex
}

// NewTransactionLog creates a new transaction log
func NewTransactionLog(config *TransactionLogConfig) (*TransactionLog, error) {
	if config == nil {
		config = DefaultTransactionLogConfig()
	}

	// Ensure directory exists
	dir := filepath.Dir(config.LogPath)
	if dir != "" && dir != "." {
		//nolint:gosec // G301: log directory needs to be accessible by service user
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %w", err)
		}
	}

	// Open log file
	file, err := os.OpenFile(config.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644) //nolint:gosec // G302: Log files are typically world-readable for debugging
	if err != nil {
		return nil, fmt.Errorf("failed to open transaction log: %w", err)
	}

	tl := &TransactionLog{
		config:    config,
		entries:   make([]*TransactionLogEntry, 0, config.MaxEntriesInMemory),
		file:      file,
		encoder:   json.NewEncoder(file),
		lastFlush: time.Now(),
		stopChan:  make(chan struct{}),
	}

	// Start background flusher if interval is set
	if config.FlushInterval > 0 {
		tl.flushTicker = time.NewTicker(config.FlushInterval)
		tl.wg.Add(1)
		go tl.backgroundFlusher()
	}

	return tl, nil
}

// backgroundFlusher periodically flushes entries to disk
func (tl *TransactionLog) backgroundFlusher() {
	defer tl.wg.Done()

	for {
		select {
		case <-tl.flushTicker.C:
			tl.Flush()
		case <-tl.stopChan:
			return
		}
	}
}

// LogEntry adds an entry to the transaction log
func (tl *TransactionLog) LogEntry(entry *TransactionLogEntry) error {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	// Generate ID if not set
	if entry.ID == "" {
		entry.ID = fmt.Sprintf("txn-%d-%d", time.Now().UnixNano(), tl.entryCount)
	}

	// Set timestamp if not set
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	// Update stats
	tl.updateStats(entry)

	// Add to in-memory buffer
	tl.entries = append(tl.entries, entry)
	tl.entryCount++

	// Flush if we've reached the max entries
	if len(tl.entries) >= tl.config.MaxEntriesInMemory {
		return tl.flushLocked()
	}

	// For verbose logging, write immediately
	if tl.config.Verbose {
		return tl.flushLocked()
	}

	return nil
}

// updateStats updates the log statistics
func (tl *TransactionLog) updateStats(entry *TransactionLogEntry) {
	tl.stats.mu.Lock()
	defer tl.stats.mu.Unlock()

	tl.stats.TotalEntries++
	tl.stats.LastEntryTime = entry.Timestamp

	if tl.stats.FirstEntryTime.IsZero() {
		tl.stats.FirstEntryTime = entry.Timestamp
	}

	switch entry.Status {
	case StatusSuccess:
		tl.stats.SuccessCount++
	case StatusFailure:
		tl.stats.FailureCount++
	case StatusSkipped:
		tl.stats.SkippedCount++
	default:
		// StatusPending, StatusRolledBack don't update counts
	}
}

// Flush writes all pending entries to disk
func (tl *TransactionLog) Flush() error {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	return tl.flushLocked()
}

// flushLocked writes entries to disk (caller must hold lock)
func (tl *TransactionLog) flushLocked() error {
	if len(tl.entries) == 0 {
		return nil
	}

	for _, entry := range tl.entries {
		if err := tl.encoder.Encode(entry); err != nil {
			return fmt.Errorf("failed to encode entry: %w", err)
		}
	}

	if err := tl.file.Sync(); err != nil {
		return fmt.Errorf("failed to sync log file: %w", err)
	}

	tl.stats.mu.Lock()
	tl.stats.FlushCount++
	tl.stats.mu.Unlock()

	// Clear buffer
	tl.entries = tl.entries[:0]
	tl.lastFlush = time.Now()

	return nil
}

// LogMigrationStart logs the start of a migration
func (tl *TransactionLog) LogMigrationStart(migrationID string, tables []string) error {
	return tl.LogEntry(&TransactionLogEntry{
		ID:        migrationID,
		Timestamp: time.Now(),
		Operation: OpMigrationStart,
		Status:    StatusPending,
		Metadata: map[string]interface{}{
			"tables": tables,
		},
	})
}

// LogMigrationEnd logs the end of a migration
func (tl *TransactionLog) LogMigrationEnd(migrationID string, success bool, stats *MigrationStats) error {
	status := StatusSuccess
	if !success {
		status = StatusFailure
	}

	return tl.LogEntry(&TransactionLogEntry{
		ID:        migrationID,
		Timestamp: time.Now(),
		Operation: OpMigrationEnd,
		Status:    status,
		Duration:  stats.Duration,
		Metadata: map[string]interface{}{
			"agents_migrated":        stats.AgentsMigrated,
			"commands_migrated":      stats.CommandsMigrated,
			"batch_jobs_migrated":    stats.BatchJobsMigrated,
			"batch_results_migrated": stats.BatchAgentResultsMigrated,
			"error_count":            len(stats.Errors),
		},
	})
}

// LogRecordMigration logs a single record migration
func (tl *TransactionLog) LogRecordMigration(table, recordID string, status TransactionStatus, err error, duration time.Duration) error {
	entry := &TransactionLogEntry{
		Timestamp: time.Now(),
		Table:     table,
		Operation: OpMigrateRecord,
		RecordID:  recordID,
		Status:    status,
		Duration:  duration,
	}

	if err != nil {
		entry.ErrorMessage = err.Error()
	}

	return tl.LogEntry(entry)
}

// LogCheckpoint logs a checkpoint for recovery
func (tl *TransactionLog) LogCheckpoint(table, lastRecordID string, processedCount, totalCount int) error {
	return tl.LogEntry(&TransactionLogEntry{
		Timestamp:  time.Now(),
		Table:      table,
		Operation:  OpCheckpoint,
		RecordID:   lastRecordID,
		Status:     StatusSuccess,
		BatchNum:   processedCount,
		BatchTotal: totalCount,
		Metadata: map[string]interface{}{
			"progress_percent": float64(processedCount) / float64(totalCount) * 100,
		},
	})
}

// LogRollbackStart logs the start of a rollback operation
func (tl *TransactionLog) LogRollbackStart(reason string) error {
	return tl.LogEntry(&TransactionLogEntry{
		Timestamp: time.Now(),
		Operation: OpRollbackStart,
		Status:    StatusPending,
		Metadata: map[string]interface{}{
			"reason": reason,
		},
	})
}

// LogRollbackComplete logs the completion of a rollback operation
func (tl *TransactionLog) LogRollbackComplete(success bool, rolledBackCount int) error {
	status := StatusSuccess
	if !success {
		status = StatusFailure
	}

	return tl.LogEntry(&TransactionLogEntry{
		Timestamp: time.Now(),
		Operation: OpRollbackComplete,
		Status:    status,
		Metadata: map[string]interface{}{
			"rolled_back_count": rolledBackCount,
		},
	})
}

// GetStats returns the current transaction log statistics
func (tl *TransactionLog) GetStats() TransactionLogStats {
	tl.stats.mu.Lock()
	defer tl.stats.mu.Unlock()

	// Return a copy to avoid race conditions
	return TransactionLogStats{
		TotalEntries:   tl.stats.TotalEntries,
		SuccessCount:   tl.stats.SuccessCount,
		FailureCount:   tl.stats.FailureCount,
		SkippedCount:   tl.stats.SkippedCount,
		LastEntryTime:  tl.stats.LastEntryTime,
		FirstEntryTime: tl.stats.FirstEntryTime,
		BytesWritten:   tl.stats.BytesWritten,
		FlushCount:     tl.stats.FlushCount,
	}
}

// Close closes the transaction log
func (tl *TransactionLog) Close() error {
	// Stop background flusher
	if tl.flushTicker != nil {
		tl.flushTicker.Stop()
		close(tl.stopChan)
		tl.wg.Wait()
	}

	// Final flush
	if err := tl.Flush(); err != nil {
		return fmt.Errorf("failed to flush on close: %w", err)
	}

	// Close file
	if err := tl.file.Close(); err != nil {
		return fmt.Errorf("failed to close log file: %w", err)
	}

	return nil
}

// ReadTransactionLog reads entries from an existing transaction log file
func ReadTransactionLog(logPath string) ([]*TransactionLogEntry, error) {
	file, err := os.Open(logPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open transaction log: %w", err)
	}
	defer file.Close()

	var entries []*TransactionLogEntry
	decoder := json.NewDecoder(file)

	for decoder.More() {
		var entry TransactionLogEntry
		if err := decoder.Decode(&entry); err != nil {
			return entries, fmt.Errorf("failed to decode entry: %w", err)
		}
		entries = append(entries, &entry)
	}

	return entries, nil
}

// FindLastCheckpoint finds the last checkpoint for a table in the log
func FindLastCheckpoint(entries []*TransactionLogEntry, table string) *TransactionLogEntry {
	var lastCheckpoint *TransactionLogEntry

	for _, entry := range entries {
		if entry.Table == table && entry.Operation == OpCheckpoint {
			lastCheckpoint = entry
		}
	}

	return lastCheckpoint
}

// GetFailedRecords returns all records that failed during migration
func GetFailedRecords(entries []*TransactionLogEntry) []*TransactionLogEntry {
	var failed []*TransactionLogEntry

	for _, entry := range entries {
		if entry.Status == StatusFailure && entry.Operation == OpMigrateRecord {
			failed = append(failed, entry)
		}
	}

	return failed
}

// LoggingMigrator wraps a Migrator with transaction logging
type LoggingMigrator struct {
	*Migrator
	txLog              *TransactionLog
	migrationID        string
	checkpointInterval int
}

// NewLoggingMigrator creates a migrator with transaction logging
func NewLoggingMigrator(source, target Store, opts *MigrationOptions, txLogConfig *TransactionLogConfig) (*LoggingMigrator, error) {
	txLog, err := NewTransactionLog(txLogConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create transaction log: %w", err)
	}

	return &LoggingMigrator{
		Migrator:           NewMigrator(source, target, opts),
		txLog:              txLog,
		migrationID:        fmt.Sprintf("migration-%d", time.Now().UnixNano()),
		checkpointInterval: 100,
	}, nil
}

// Migrate performs migration with transaction logging
func (lm *LoggingMigrator) Migrate(ctx context.Context) (*MigrationStats, error) {
	// Log migration start
	tables := []string{"agents", "commands", "batch_jobs", "batch_agent_results"}
	if err := lm.txLog.LogMigrationStart(lm.migrationID, tables); err != nil {
		return nil, fmt.Errorf("failed to log migration start: %w", err)
	}

	// Wrap the progress callback to include logging
	originalCallback := lm.opts.ProgressCallback
	lm.opts.ProgressCallback = func(table string, current, total int) {
		// Call original callback if set
		if originalCallback != nil {
			originalCallback(table, current, total)
		}

		// Log checkpoint at intervals
		if current > 0 && current%lm.checkpointInterval == 0 {
			_ = lm.txLog.LogCheckpoint(table, fmt.Sprintf("record-%d", current), current, total) //nolint:errcheck // best-effort checkpoint
		}
	}

	// Run the migration
	stats, err := lm.Migrator.Migrate(ctx)

	// Log migration end
	success := err == nil && len(stats.Errors) == 0
	if logErr := lm.txLog.LogMigrationEnd(lm.migrationID, success, stats); logErr != nil {
		// Log error but don't fail the migration
		fmt.Printf("warning: failed to log migration end: %v\n", logErr)
	}

	return stats, err
}

// GetTransactionLog returns the transaction log
func (lm *LoggingMigrator) GetTransactionLog() *TransactionLog {
	return lm.txLog
}

// Close closes the logging migrator and its transaction log
func (lm *LoggingMigrator) Close() error {
	return lm.txLog.Close()
}

// RecoveryInfo provides information for resuming a failed migration
type RecoveryInfo struct {
	MigrationID     string
	StartTime       time.Time
	LastCheckpoint  map[string]*TransactionLogEntry // table -> checkpoint
	FailedRecords   []*TransactionLogEntry
	CompletedTables []string
	PendingTables   []string
	CanResume       bool
	ResumePoint     string
}

// AnalyzeForRecovery analyzes a transaction log for recovery information
func AnalyzeForRecovery(logPath string) (*RecoveryInfo, error) {
	entries, err := ReadTransactionLog(logPath)
	if err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("transaction log is empty")
	}

	info := &RecoveryInfo{
		LastCheckpoint: make(map[string]*TransactionLogEntry),
	}

	tables := []string{"agents", "commands", "batch_jobs", "batch_agent_results"}
	completedMap := make(map[string]bool)

	for _, entry := range entries {
		switch entry.Operation {
		case OpMigrationStart:
			info.MigrationID = entry.ID
			info.StartTime = entry.Timestamp

		case OpCheckpoint:
			info.LastCheckpoint[entry.Table] = entry

		case OpMigrationEnd:
			if entry.Status == StatusSuccess {
				info.CanResume = false
				return info, nil
			}

		case OpMigrateRecord:
			if entry.Status == StatusFailure {
				info.FailedRecords = append(info.FailedRecords, entry)
			}
		default:
			// OpValidateRecord, OpSkipRecord, OpRollbackStart, OpRollbackComplete handled implicitly
		}

		// Track completed tables (100% progress in checkpoint)
		if entry.Operation == OpCheckpoint && entry.BatchNum == entry.BatchTotal {
			completedMap[entry.Table] = true
		}
	}

	// Determine completed and pending tables
	for _, table := range tables {
		if completedMap[table] {
			info.CompletedTables = append(info.CompletedTables, table)
		} else {
			info.PendingTables = append(info.PendingTables, table)
		}
	}

	// Migration is resumable if we have incomplete tables
	info.CanResume = len(info.PendingTables) > 0

	// Find resume point
	if len(info.PendingTables) > 0 {
		firstPending := info.PendingTables[0]
		if checkpoint, ok := info.LastCheckpoint[firstPending]; ok {
			info.ResumePoint = fmt.Sprintf("table=%s, record=%s (%d/%d)",
				firstPending, checkpoint.RecordID, checkpoint.BatchNum, checkpoint.BatchTotal)
		} else {
			info.ResumePoint = fmt.Sprintf("table=%s, from start", firstPending)
		}
	}

	return info, nil
}
