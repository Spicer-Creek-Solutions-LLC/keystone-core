package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultTransactionLogConfig(t *testing.T) {
	config := DefaultTransactionLogConfig()

	if config.LogPath != "migration_txlog.json" {
		t.Errorf("expected default log path, got %s", config.LogPath)
	}

	if config.FlushInterval != 5*time.Second {
		t.Errorf("expected 5s flush interval, got %v", config.FlushInterval)
	}

	if config.MaxEntriesInMemory != 1000 {
		t.Errorf("expected 1000 max entries, got %d", config.MaxEntriesInMemory)
	}
}

func TestNewTransactionLog(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test_txlog.json")

	config := &TransactionLogConfig{
		LogPath:            logPath,
		FlushInterval:      0, // Disable background flusher for tests
		MaxEntriesInMemory: 10,
	}

	txLog, err := NewTransactionLog(config)
	if err != nil {
		t.Fatalf("failed to create transaction log: %v", err)
	}
	defer txLog.Close()

	// Verify file was created
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Error("log file was not created")
	}
}

func TestNewTransactionLog_WithDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "subdir", "nested", "test_txlog.json")

	config := &TransactionLogConfig{
		LogPath:            logPath,
		FlushInterval:      0,
		MaxEntriesInMemory: 10,
	}

	txLog, err := NewTransactionLog(config)
	if err != nil {
		t.Fatalf("failed to create transaction log with nested dirs: %v", err)
	}
	defer txLog.Close()

	// Verify directories and file were created
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Error("log file was not created in nested directory")
	}
}

func TestTransactionLog_LogEntry(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test_txlog.json")

	config := &TransactionLogConfig{
		LogPath:            logPath,
		FlushInterval:      0,
		MaxEntriesInMemory: 100,
		Verbose:            true, // Write immediately
	}

	txLog, err := NewTransactionLog(config)
	if err != nil {
		t.Fatalf("failed to create transaction log: %v", err)
	}

	// Log some entries
	for i := 0; i < 5; i++ {
		err := txLog.LogEntry(&TransactionLogEntry{
			Table:     "agents",
			Operation: OpMigrateRecord,
			RecordID:  "agent-" + string(rune('a'+i)),
			Status:    StatusSuccess,
		})
		if err != nil {
			t.Fatalf("failed to log entry: %v", err)
		}
	}

	txLog.Close()

	// Read back the entries
	entries, err := ReadTransactionLog(logPath)
	if err != nil {
		t.Fatalf("failed to read transaction log: %v", err)
	}

	if len(entries) != 5 {
		t.Errorf("expected 5 entries, got %d", len(entries))
	}

	for i, entry := range entries {
		if entry.Table != "agents" {
			t.Errorf("entry %d: expected table 'agents', got '%s'", i, entry.Table)
		}
		if entry.Operation != OpMigrateRecord {
			t.Errorf("entry %d: expected operation migrate, got '%s'", i, entry.Operation)
		}
	}
}

func TestTransactionLog_LogMigrationStartEnd(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test_txlog.json")

	config := &TransactionLogConfig{
		LogPath:       logPath,
		FlushInterval: 0,
		Verbose:       true,
	}

	txLog, err := NewTransactionLog(config)
	if err != nil {
		t.Fatalf("failed to create transaction log: %v", err)
	}

	// Log migration start
	err = txLog.LogMigrationStart("test-migration-1", []string{"agents", "commands"})
	if err != nil {
		t.Fatalf("failed to log migration start: %v", err)
	}

	// Log migration end
	stats := &MigrationStats{
		AgentsMigrated:   100,
		CommandsMigrated: 50,
		Duration:         10 * time.Second,
	}
	err = txLog.LogMigrationEnd("test-migration-1", true, stats)
	if err != nil {
		t.Fatalf("failed to log migration end: %v", err)
	}

	txLog.Close()

	// Read and verify
	entries, err := ReadTransactionLog(logPath)
	if err != nil {
		t.Fatalf("failed to read log: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries[0].Operation != OpMigrationStart {
		t.Errorf("expected migration_start, got %s", entries[0].Operation)
	}

	if entries[1].Operation != OpMigrationEnd {
		t.Errorf("expected migration_end, got %s", entries[1].Operation)
	}

	if entries[1].Status != StatusSuccess {
		t.Errorf("expected success status, got %s", entries[1].Status)
	}
}

func TestTransactionLog_LogCheckpoint(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test_txlog.json")

	config := &TransactionLogConfig{
		LogPath:       logPath,
		FlushInterval: 0,
		Verbose:       true,
	}

	txLog, err := NewTransactionLog(config)
	if err != nil {
		t.Fatalf("failed to create transaction log: %v", err)
	}

	err = txLog.LogCheckpoint("agents", "agent-50", 50, 100)
	if err != nil {
		t.Fatalf("failed to log checkpoint: %v", err)
	}

	txLog.Close()

	entries, err := ReadTransactionLog(logPath)
	if err != nil {
		t.Fatalf("failed to read log: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	entry := entries[0]
	if entry.Operation != OpCheckpoint {
		t.Errorf("expected checkpoint operation, got %s", entry.Operation)
	}
	if entry.Table != "agents" {
		t.Errorf("expected agents table, got %s", entry.Table)
	}
	if entry.RecordID != "agent-50" {
		t.Errorf("expected agent-50, got %s", entry.RecordID)
	}
	if entry.BatchNum != 50 {
		t.Errorf("expected batch num 50, got %d", entry.BatchNum)
	}
	if entry.BatchTotal != 100 {
		t.Errorf("expected batch total 100, got %d", entry.BatchTotal)
	}
}

func TestTransactionLog_LogRollback(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test_txlog.json")

	config := &TransactionLogConfig{
		LogPath:       logPath,
		FlushInterval: 0,
		Verbose:       true,
	}

	txLog, err := NewTransactionLog(config)
	if err != nil {
		t.Fatalf("failed to create transaction log: %v", err)
	}

	err = txLog.LogRollbackStart("migration failed")
	if err != nil {
		t.Fatalf("failed to log rollback start: %v", err)
	}

	err = txLog.LogRollbackComplete(true, 25)
	if err != nil {
		t.Fatalf("failed to log rollback complete: %v", err)
	}

	txLog.Close()

	entries, err := ReadTransactionLog(logPath)
	if err != nil {
		t.Fatalf("failed to read log: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries[0].Operation != OpRollbackStart {
		t.Errorf("expected rollback_start, got %s", entries[0].Operation)
	}

	if entries[1].Operation != OpRollbackComplete {
		t.Errorf("expected rollback_complete, got %s", entries[1].Operation)
	}
}

func TestTransactionLog_Flush(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test_txlog.json")

	config := &TransactionLogConfig{
		LogPath:            logPath,
		FlushInterval:      0,
		MaxEntriesInMemory: 100, // High so we can test explicit flush
		Verbose:            false,
	}

	txLog, err := NewTransactionLog(config)
	if err != nil {
		t.Fatalf("failed to create transaction log: %v", err)
	}

	// Log entries (they won't be flushed immediately)
	for i := 0; i < 5; i++ {
		txLog.LogEntry(&TransactionLogEntry{
			Table:     "test",
			Operation: OpMigrateRecord,
			RecordID:  "rec",
			Status:    StatusSuccess,
		})
	}

	// File should be empty or small before flush
	info, _ := os.Stat(logPath)
	beforeFlush := info.Size()

	// Explicit flush
	err = txLog.Flush()
	if err != nil {
		t.Fatalf("failed to flush: %v", err)
	}

	// File should have data after flush
	info, _ = os.Stat(logPath)
	afterFlush := info.Size()

	if afterFlush <= beforeFlush {
		t.Error("expected file to grow after flush")
	}

	txLog.Close()
}

func TestTransactionLog_GetStats(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test_txlog.json")

	config := &TransactionLogConfig{
		LogPath:       logPath,
		FlushInterval: 0,
		Verbose:       true,
	}

	txLog, err := NewTransactionLog(config)
	if err != nil {
		t.Fatalf("failed to create transaction log: %v", err)
	}

	// Log various entries
	txLog.LogEntry(&TransactionLogEntry{Status: StatusSuccess})
	txLog.LogEntry(&TransactionLogEntry{Status: StatusSuccess})
	txLog.LogEntry(&TransactionLogEntry{Status: StatusFailure})
	txLog.LogEntry(&TransactionLogEntry{Status: StatusSkipped})

	stats := txLog.GetStats()

	if stats.TotalEntries != 4 {
		t.Errorf("expected 4 total entries, got %d", stats.TotalEntries)
	}
	if stats.SuccessCount != 2 {
		t.Errorf("expected 2 success, got %d", stats.SuccessCount)
	}
	if stats.FailureCount != 1 {
		t.Errorf("expected 1 failure, got %d", stats.FailureCount)
	}
	if stats.SkippedCount != 1 {
		t.Errorf("expected 1 skipped, got %d", stats.SkippedCount)
	}

	txLog.Close()
}

func TestFindLastCheckpoint(t *testing.T) {
	entries := []*TransactionLogEntry{
		{Table: "agents", Operation: OpMigrateRecord, RecordID: "a1"},
		{Table: "agents", Operation: OpCheckpoint, RecordID: "a10", BatchNum: 10, BatchTotal: 100},
		{Table: "agents", Operation: OpMigrateRecord, RecordID: "a20"},
		{Table: "agents", Operation: OpCheckpoint, RecordID: "a50", BatchNum: 50, BatchTotal: 100},
		{Table: "commands", Operation: OpCheckpoint, RecordID: "c20", BatchNum: 20, BatchTotal: 50},
	}

	agentCheckpoint := FindLastCheckpoint(entries, "agents")
	if agentCheckpoint == nil {
		t.Fatal("expected to find agent checkpoint")
	}
	if agentCheckpoint.RecordID != "a50" {
		t.Errorf("expected a50, got %s", agentCheckpoint.RecordID)
	}
	if agentCheckpoint.BatchNum != 50 {
		t.Errorf("expected batch num 50, got %d", agentCheckpoint.BatchNum)
	}

	commandCheckpoint := FindLastCheckpoint(entries, "commands")
	if commandCheckpoint == nil {
		t.Fatal("expected to find command checkpoint")
	}
	if commandCheckpoint.RecordID != "c20" {
		t.Errorf("expected c20, got %s", commandCheckpoint.RecordID)
	}

	noCheckpoint := FindLastCheckpoint(entries, "nonexistent")
	if noCheckpoint != nil {
		t.Error("expected nil for nonexistent table")
	}
}

func TestGetFailedRecords(t *testing.T) {
	entries := []*TransactionLogEntry{
		{Operation: OpMigrateRecord, Status: StatusSuccess, RecordID: "r1"},
		{Operation: OpMigrateRecord, Status: StatusFailure, RecordID: "r2"},
		{Operation: OpMigrateRecord, Status: StatusSuccess, RecordID: "r3"},
		{Operation: OpCheckpoint, Status: StatusSuccess},
		{Operation: OpMigrateRecord, Status: StatusFailure, RecordID: "r4"},
	}

	failed := GetFailedRecords(entries)

	if len(failed) != 2 {
		t.Fatalf("expected 2 failed records, got %d", len(failed))
	}

	if failed[0].RecordID != "r2" {
		t.Errorf("expected r2, got %s", failed[0].RecordID)
	}
	if failed[1].RecordID != "r4" {
		t.Errorf("expected r4, got %s", failed[1].RecordID)
	}
}

func TestReadTransactionLog_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "empty.json")

	// Create empty file
	file, _ := os.Create(logPath)
	file.Close()

	entries, err := ReadTransactionLog(logPath)
	if err != nil {
		t.Fatalf("unexpected error reading empty log: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestReadTransactionLog_NonExistent(t *testing.T) {
	_, err := ReadTransactionLog("/nonexistent/path/log.json")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestTransactionLog_AutoFlush(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test_txlog.json")

	config := &TransactionLogConfig{
		LogPath:            logPath,
		FlushInterval:      0,
		MaxEntriesInMemory: 3, // Low to trigger auto-flush
		Verbose:            false,
	}

	txLog, err := NewTransactionLog(config)
	if err != nil {
		t.Fatalf("failed to create transaction log: %v", err)
	}

	// Log 5 entries - should trigger auto-flush after 3
	for i := 0; i < 5; i++ {
		txLog.LogEntry(&TransactionLogEntry{
			RecordID: "r",
			Status:   StatusSuccess,
		})
	}

	// Read back - should have entries from auto-flush
	txLog.Close()

	entries, err := ReadTransactionLog(logPath)
	if err != nil {
		t.Fatalf("failed to read log: %v", err)
	}

	if len(entries) != 5 {
		t.Errorf("expected 5 entries after auto-flush, got %d", len(entries))
	}
}

func TestAnalyzeForRecovery(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "recovery_test.json")

	// Create a transaction log with incomplete migration
	config := &TransactionLogConfig{
		LogPath:       logPath,
		FlushInterval: 0,
		Verbose:       true,
	}

	txLog, err := NewTransactionLog(config)
	if err != nil {
		t.Fatalf("failed to create transaction log: %v", err)
	}

	// Log a partial migration
	txLog.LogMigrationStart("mig-1", []string{"agents", "commands", "batch_jobs", "batch_agent_results"})
	txLog.LogCheckpoint("agents", "a100", 100, 100) // Complete
	txLog.LogCheckpoint("commands", "c50", 50, 100) // Incomplete
	txLog.LogEntry(&TransactionLogEntry{
		Table:     "commands",
		Operation: OpMigrateRecord,
		RecordID:  "c55",
		Status:    StatusFailure,
		ErrorMessage: "connection lost",
	})
	txLog.Close()

	// Analyze for recovery
	info, err := AnalyzeForRecovery(logPath)
	if err != nil {
		t.Fatalf("failed to analyze for recovery: %v", err)
	}

	if info.MigrationID != "mig-1" {
		t.Errorf("expected migration ID mig-1, got %s", info.MigrationID)
	}

	if !info.CanResume {
		t.Error("expected migration to be resumable")
	}

	if len(info.CompletedTables) != 1 || info.CompletedTables[0] != "agents" {
		t.Errorf("expected agents to be completed, got %v", info.CompletedTables)
	}

	if len(info.PendingTables) != 3 {
		t.Errorf("expected 3 pending tables, got %d", len(info.PendingTables))
	}

	if len(info.FailedRecords) != 1 {
		t.Errorf("expected 1 failed record, got %d", len(info.FailedRecords))
	}
}

func TestAnalyzeForRecovery_CompletedMigration(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "completed_test.json")

	config := &TransactionLogConfig{
		LogPath:       logPath,
		FlushInterval: 0,
		Verbose:       true,
	}

	txLog, err := NewTransactionLog(config)
	if err != nil {
		t.Fatalf("failed to create transaction log: %v", err)
	}

	// Log a complete migration
	txLog.LogMigrationStart("mig-complete", []string{"agents"})
	txLog.LogCheckpoint("agents", "a100", 100, 100)
	txLog.LogMigrationEnd("mig-complete", true, &MigrationStats{
		AgentsMigrated: 100,
		Duration:       5 * time.Second,
	})
	txLog.Close()

	info, err := AnalyzeForRecovery(logPath)
	if err != nil {
		t.Fatalf("failed to analyze: %v", err)
	}

	if info.CanResume {
		t.Error("completed migration should not be resumable")
	}
}

func TestTransactionLog_LogRecordMigration(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "record_test.json")

	config := &TransactionLogConfig{
		LogPath:       logPath,
		FlushInterval: 0,
		Verbose:       true,
	}

	txLog, err := NewTransactionLog(config)
	if err != nil {
		t.Fatalf("failed to create transaction log: %v", err)
	}

	// Log successful record
	err = txLog.LogRecordMigration("agents", "agent-1", StatusSuccess, nil, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("failed to log record migration: %v", err)
	}

	// Log failed record
	err = txLog.LogRecordMigration("agents", "agent-2", StatusFailure,
		os.ErrNotExist, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("failed to log failed record: %v", err)
	}

	txLog.Close()

	entries, err := ReadTransactionLog(logPath)
	if err != nil {
		t.Fatalf("failed to read log: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries[0].RecordID != "agent-1" || entries[0].Status != StatusSuccess {
		t.Error("first entry should be successful agent-1")
	}

	if entries[1].RecordID != "agent-2" || entries[1].Status != StatusFailure {
		t.Error("second entry should be failed agent-2")
	}

	if entries[1].ErrorMessage == "" {
		t.Error("failed entry should have error message")
	}
}
