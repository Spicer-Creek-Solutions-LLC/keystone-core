package dns

import (
	"context"
	"time"

	"github.com/shawnbutts/keystone-core/internal/audit"
)

// AuditLogger provides audit logging for DNS operations.
type AuditLogger struct {
	auditor *audit.Auditor
	zone    string
}

// NewAuditLogger creates a new DNS audit logger.
func NewAuditLogger(ctx context.Context, zone string) (*AuditLogger, error) {
	auditor, err := audit.NewAuditor(ctx, "dns", nil)
	if err != nil {
		return nil, err
	}

	return &AuditLogger{
		auditor: auditor,
		zone:    zone,
	}, nil
}

// NewAuditLoggerWithAuditor creates a DNS audit logger with a custom auditor.
func NewAuditLoggerWithAuditor(zone string, auditor *audit.Auditor) *AuditLogger {
	return &AuditLogger{
		auditor: auditor,
		zone:    zone,
	}
}

// LogRecordCreated logs the creation of a DNS record.
func (l *AuditLogger) LogRecordCreated(ctx context.Context, provider string, record Record, duration time.Duration, err error) error {
	if l.auditor == nil {
		return nil
	}

	entry := l.auditor.StartEntry(audit.ActionDNSRecordCreated, "create")
	entry.Target = l.zone
	entry.DurationMS = duration.Milliseconds()
	entry.Extra = map[string]interface{}{
		"provider":    provider,
		"record_type": string(record.Type),
		"record_name": record.Name,
		"record_id":   record.ID,
		"ttl":         record.TTL,
	}

	if err != nil {
		entry.Result = audit.ResultFailure
		entry.Error = err.Error()
	} else {
		entry.Result = audit.ResultSuccess
	}

	return l.auditor.Log(ctx, entry)
}

// LogRecordUpdated logs the update of a DNS record.
func (l *AuditLogger) LogRecordUpdated(ctx context.Context, provider string, oldRecord, newRecord Record, changes []FieldChange, duration time.Duration, err error) error {
	if l.auditor == nil {
		return nil
	}

	entry := l.auditor.StartEntry(audit.ActionDNSRecordUpdated, "update")
	entry.Target = l.zone
	entry.DurationMS = duration.Milliseconds()
	entry.Extra = map[string]interface{}{
		"provider":    provider,
		"record_type": string(newRecord.Type),
		"record_name": newRecord.Name,
		"record_id":   newRecord.ID,
		"changes":     formatFieldChanges(changes),
	}

	if err != nil {
		entry.Result = audit.ResultFailure
		entry.Error = err.Error()
	} else {
		entry.Result = audit.ResultSuccess
	}

	return l.auditor.Log(ctx, entry)
}

// LogRecordDeleted logs the deletion of a DNS record.
func (l *AuditLogger) LogRecordDeleted(ctx context.Context, provider string, record Record, duration time.Duration, err error) error {
	if l.auditor == nil {
		return nil
	}

	entry := l.auditor.StartEntry(audit.ActionDNSRecordDeleted, "delete")
	entry.Target = l.zone
	entry.DurationMS = duration.Milliseconds()
	entry.Extra = map[string]interface{}{
		"provider":    provider,
		"record_type": string(record.Type),
		"record_name": record.Name,
		"record_id":   record.ID,
	}

	if err != nil {
		entry.Result = audit.ResultFailure
		entry.Error = err.Error()
	} else {
		entry.Result = audit.ResultSuccess
	}

	return l.auditor.Log(ctx, entry)
}

// LogSyncCompleted logs the completion of a DNS sync operation.
func (l *AuditLogger) LogSyncCompleted(ctx context.Context, provider string, result *SyncResult, duration time.Duration) error {
	if l.auditor == nil {
		return nil
	}

	entry := l.auditor.StartEntry(audit.ActionDNSSyncCompleted, "sync")
	entry.Target = l.zone
	entry.DurationMS = duration.Milliseconds()
	entry.Extra = map[string]interface{}{
		"provider":        provider,
		"records_created": result.Created,
		"records_updated": result.Updated,
		"records_deleted": result.Deleted,
		"unchanged":       result.Unchanged,
		"error_count":     len(result.Errors),
	}

	if result.HasErrors() {
		entry.Result = audit.ResultFailure
		// Collect error messages
		errorMessages := make([]string, len(result.Errors))
		for i, err := range result.Errors {
			errorMessages[i] = err.Error()
		}
		entry.Extra["errors"] = errorMessages
	} else {
		entry.Result = audit.ResultSuccess
	}

	return l.auditor.Log(ctx, entry)
}

// FieldChange represents a change to a record field.
type FieldChange struct {
	Field    string
	OldValue interface{}
	NewValue interface{}
}

// formatFieldChanges converts field changes to a map for audit logging.
func formatFieldChanges(changes []FieldChange) []map[string]interface{} {
	result := make([]map[string]interface{}, len(changes))
	for i, change := range changes {
		result[i] = map[string]interface{}{
			"field":     change.Field,
			"old_value": change.OldValue,
			"new_value": change.NewValue,
		}
	}
	return result
}

// Close closes the audit logger.
func (l *AuditLogger) Close() error {
	if l.auditor != nil {
		return l.auditor.Close()
	}
	return nil
}

// DefaultAuditLogger is a global audit logger for DNS operations.
var DefaultAuditLogger *AuditLogger

// InitDefaultAuditLogger initializes the default DNS audit logger.
func InitDefaultAuditLogger(ctx context.Context, zone string) error {
	logger, err := NewAuditLogger(ctx, zone)
	if err != nil {
		return err
	}
	DefaultAuditLogger = logger
	return nil
}

// CloseDefaultAuditLogger closes the default audit logger.
func CloseDefaultAuditLogger() error {
	if DefaultAuditLogger != nil {
		return DefaultAuditLogger.Close()
	}
	return nil
}
