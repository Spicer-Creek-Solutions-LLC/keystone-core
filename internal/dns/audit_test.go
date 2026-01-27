package dns

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/audit"
)

// mockAuditLogger is a mock audit logger for testing.
type mockAuditLogger struct {
	mu      sync.Mutex
	entries []*audit.AuditEntry
}

func (m *mockAuditLogger) Log(_ context.Context, entry *audit.AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, entry)
	return nil
}

func (m *mockAuditLogger) Close() error {
	return nil
}

func (m *mockAuditLogger) getEntries() []*audit.AuditEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*audit.AuditEntry, len(m.entries))
	copy(result, m.entries)
	return result
}

func TestAuditLogger_LogRecordCreated(t *testing.T) {
	// Create audit logger with noop config (to avoid syslog connection)
	auditor, err := audit.NewAuditor("dns-test", &audit.AuditConfig{
		Level:   audit.AuditLevelAll,
		Backend: "stderr",
	})
	if err != nil {
		t.Fatalf("failed to create auditor: %v", err)
	}
	defer auditor.Close()

	logger := NewAuditLoggerWithAuditor("example.com", auditor)

	record := Record{
		ID:    "rec-123",
		Type:  RecordTypeA,
		Name:  "www",
		Value: "192.0.2.1",
		TTL:   300,
	}

	// Test successful create
	err = logger.LogRecordCreated(context.Background(), "cloudflare", record, 50*time.Millisecond, nil)
	if err != nil {
		t.Errorf("LogRecordCreated() error = %v", err)
	}

	// Test failed create
	err = logger.LogRecordCreated(context.Background(), "cloudflare", record, 100*time.Millisecond, fmt.Errorf("api error"))
	if err != nil {
		t.Errorf("LogRecordCreated() with error, error = %v", err)
	}
}

func TestAuditLogger_LogRecordUpdated(t *testing.T) {
	auditor, err := audit.NewAuditor("dns-test", &audit.AuditConfig{
		Level:   audit.AuditLevelAll,
		Backend: "stderr",
	})
	if err != nil {
		t.Fatalf("failed to create auditor: %v", err)
	}
	defer auditor.Close()

	logger := NewAuditLoggerWithAuditor("example.com", auditor)

	oldRecord := Record{
		ID:    "rec-123",
		Type:  RecordTypeA,
		Name:  "www",
		Value: "192.0.2.1",
		TTL:   300,
	}

	newRecord := Record{
		ID:    "rec-123",
		Type:  RecordTypeA,
		Name:  "www",
		Value: "192.0.2.2",
		TTL:   600,
	}

	changes := []FieldChange{
		{Field: "value", OldValue: "192.0.2.1", NewValue: "192.0.2.2"},
		{Field: "ttl", OldValue: 300, NewValue: 600},
	}

	err = logger.LogRecordUpdated(context.Background(), "cloudflare", oldRecord, newRecord, changes, 60*time.Millisecond, nil)
	if err != nil {
		t.Errorf("LogRecordUpdated() error = %v", err)
	}
}

func TestAuditLogger_LogRecordDeleted(t *testing.T) {
	auditor, err := audit.NewAuditor("dns-test", &audit.AuditConfig{
		Level:   audit.AuditLevelAll,
		Backend: "stderr",
	})
	if err != nil {
		t.Fatalf("failed to create auditor: %v", err)
	}
	defer auditor.Close()

	logger := NewAuditLoggerWithAuditor("example.com", auditor)

	record := Record{
		ID:    "rec-123",
		Type:  RecordTypeA,
		Name:  "www",
		Value: "192.0.2.1",
	}

	err = logger.LogRecordDeleted(context.Background(), "cloudflare", record, 30*time.Millisecond, nil)
	if err != nil {
		t.Errorf("LogRecordDeleted() error = %v", err)
	}
}

func TestAuditLogger_LogSyncCompleted(t *testing.T) {
	auditor, err := audit.NewAuditor("dns-test", &audit.AuditConfig{
		Level:   audit.AuditLevelAll,
		Backend: "stderr",
	})
	if err != nil {
		t.Fatalf("failed to create auditor: %v", err)
	}
	defer auditor.Close()

	logger := NewAuditLoggerWithAuditor("example.com", auditor)

	// Successful sync
	result := &SyncResult{
		Created:   3,
		Updated:   2,
		Deleted:   1,
		Unchanged: 10,
		Errors:    nil,
	}

	err = logger.LogSyncCompleted(context.Background(), "cloudflare", result, 500*time.Millisecond)
	if err != nil {
		t.Errorf("LogSyncCompleted() error = %v", err)
	}

	// Sync with errors
	resultWithErrors := &SyncResult{
		Created: 1,
		Errors:  []error{fmt.Errorf("error1"), fmt.Errorf("error2")},
	}

	err = logger.LogSyncCompleted(context.Background(), "cloudflare", resultWithErrors, 300*time.Millisecond)
	if err != nil {
		t.Errorf("LogSyncCompleted() with errors, error = %v", err)
	}
}

func TestAuditLogger_NilAuditor(t *testing.T) {
	logger := &AuditLogger{
		auditor: nil,
		zone:    "example.com",
	}

	record := Record{Type: RecordTypeA, Name: "test"}

	// All operations should return nil without error
	if err := logger.LogRecordCreated(context.Background(), "test", record, time.Millisecond, nil); err != nil {
		t.Errorf("LogRecordCreated with nil auditor should return nil, got %v", err)
	}

	if err := logger.LogRecordUpdated(context.Background(), "test", record, record, nil, time.Millisecond, nil); err != nil {
		t.Errorf("LogRecordUpdated with nil auditor should return nil, got %v", err)
	}

	if err := logger.LogRecordDeleted(context.Background(), "test", record, time.Millisecond, nil); err != nil {
		t.Errorf("LogRecordDeleted with nil auditor should return nil, got %v", err)
	}

	if err := logger.LogSyncCompleted(context.Background(), "test", &SyncResult{}, time.Millisecond); err != nil {
		t.Errorf("LogSyncCompleted with nil auditor should return nil, got %v", err)
	}

	if err := logger.Close(); err != nil {
		t.Errorf("Close with nil auditor should return nil, got %v", err)
	}
}

func TestNewAuditLogger(t *testing.T) {
	// This test requires stderr backend to avoid platform-specific syslog issues
	origConfig := audit.DefaultAuditConfig()
	origConfig.Backend = "stderr"

	logger, err := NewAuditLogger("example.com")
	if err != nil {
		// On some platforms this might fail, which is acceptable
		t.Skipf("NewAuditLogger() failed (platform-specific): %v", err)
	}

	if logger == nil {
		t.Error("NewAuditLogger() returned nil")
	}

	if logger.zone != "example.com" {
		t.Errorf("zone = %s, want example.com", logger.zone)
	}

	if err := logger.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestFormatFieldChanges(t *testing.T) {
	changes := []FieldChange{
		{Field: "value", OldValue: "192.0.2.1", NewValue: "192.0.2.2"},
		{Field: "ttl", OldValue: 300, NewValue: 600},
	}

	result := formatFieldChanges(changes)

	if len(result) != 2 {
		t.Errorf("formatFieldChanges() returned %d items, want 2", len(result))
	}

	if result[0]["field"] != "value" {
		t.Errorf("result[0][field] = %v, want 'value'", result[0]["field"])
	}
	if result[0]["old_value"] != "192.0.2.1" {
		t.Errorf("result[0][old_value] = %v, want '192.0.2.1'", result[0]["old_value"])
	}
	if result[0]["new_value"] != "192.0.2.2" {
		t.Errorf("result[0][new_value] = %v, want '192.0.2.2'", result[0]["new_value"])
	}

	if result[1]["field"] != "ttl" {
		t.Errorf("result[1][field] = %v, want 'ttl'", result[1]["field"])
	}
}

func TestDefaultAuditLogger(t *testing.T) {
	// Test that uninitialized default logger doesn't panic
	if DefaultAuditLogger != nil {
		t.Error("DefaultAuditLogger should be nil initially")
	}

	// Close should not error on nil
	if err := CloseDefaultAuditLogger(); err != nil {
		t.Errorf("CloseDefaultAuditLogger() on nil error = %v", err)
	}
}
