// Package audit provides comprehensive audit logging and compliance reporting
// for runbook executions.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// EventType represents the type of audit event.
type EventType string

// Audit event types.
const (
	EventExecutionStarted   EventType = "execution.started"
	EventExecutionCompleted EventType = "execution.completed"
	EventExecutionFailed    EventType = "execution.failed"
	EventExecutionCancelled EventType = "execution.cancelled"
	EventExecutionPaused    EventType = "execution.paused"
	EventExecutionResumed   EventType = "execution.resumed"
	EventStepStarted        EventType = "step.started"
	EventStepCompleted      EventType = "step.completed"
	EventStepFailed         EventType = "step.failed"
	EventStepSkipped        EventType = "step.skipped"
	EventStepRetried        EventType = "step.retried"
	EventApprovalRequested  EventType = "approval.requested"
	EventApprovalGranted    EventType = "approval.granted"
	EventApprovalDenied     EventType = "approval.denied"
	EventInputProvided      EventType = "input.provided"
	EventOutputGenerated    EventType = "output.generated"
	EventSecretAccessed     EventType = "secret.accessed"
	EventTriggerFired       EventType = "trigger.fired"
)

// Event represents a single audit event.
type Event struct {
	// ID is the unique event identifier.
	ID string `json:"id"`

	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`

	// Type is the event type.
	Type EventType `json:"type"`

	// ExecutionID is the associated execution.
	ExecutionID string `json:"execution_id"`

	// RunbookName is the runbook being executed.
	RunbookName string `json:"runbook_name"`

	// RunbookVersion is the runbook version.
	RunbookVersion string `json:"runbook_version,omitempty"`

	// StepName is the associated step (if applicable).
	StepName string `json:"step_name,omitempty"`

	// Actor is who/what initiated the event.
	Actor string `json:"actor,omitempty"`

	// Details contains event-specific data.
	Details map[string]interface{} `json:"details,omitempty"`

	// Outcome is the result (success, failure, etc.).
	Outcome string `json:"outcome,omitempty"`

	// Duration is how long the operation took.
	Duration time.Duration `json:"duration,omitempty"`

	// Error is the error message if applicable.
	Error string `json:"error,omitempty"`

	// Metadata contains additional context.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Logger provides audit logging functionality.
type Logger struct {
	mu sync.RWMutex

	// Storage backend
	storage Storage

	// Secret masker
	masker *SecretMasker

	// Event hooks
	onEvent []func(event *Event)

	// Actor resolver
	actorResolver func(ctx context.Context) string
}

// Storage defines the interface for audit event persistence.
type Storage interface {
	// Store saves an audit event.
	Store(ctx context.Context, event *Event) error

	// Query searches for audit events matching criteria.
	Query(ctx context.Context, query *Query) ([]*Event, error)

	// GetByExecutionID retrieves all events for an execution.
	GetByExecutionID(ctx context.Context, executionID string) ([]*Event, error)

	// Delete removes events older than the given time.
	Delete(ctx context.Context, before time.Time) (int64, error)

	// Count returns the number of events matching criteria.
	Count(ctx context.Context, query *Query) (int64, error)
}

// Query defines search criteria for audit events.
type Query struct {
	// ExecutionID filters by execution.
	ExecutionID string `json:"execution_id,omitempty"`

	// RunbookName filters by runbook.
	RunbookName string `json:"runbook_name,omitempty"`

	// Types filters by event types.
	Types []EventType `json:"types,omitempty"`

	// Actor filters by actor.
	Actor string `json:"actor,omitempty"`

	// StartTime is the earliest event time.
	StartTime *time.Time `json:"start_time,omitempty"`

	// EndTime is the latest event time.
	EndTime *time.Time `json:"end_time,omitempty"`

	// Outcome filters by outcome.
	Outcome string `json:"outcome,omitempty"`

	// Limit is the maximum number of results.
	Limit int `json:"limit,omitempty"`

	// Offset is the number of results to skip.
	Offset int `json:"offset,omitempty"`

	// OrderBy is the field to sort by.
	OrderBy string `json:"order_by,omitempty"`

	// OrderDesc sorts in descending order.
	OrderDesc bool `json:"order_desc,omitempty"`
}

// LoggerOption configures a Logger.
type LoggerOption func(*Logger)

// WithStorage sets the storage backend.
func WithStorage(storage Storage) LoggerOption {
	return func(l *Logger) {
		l.storage = storage
	}
}

// WithSecretMasker sets the secret masker.
func WithSecretMasker(masker *SecretMasker) LoggerOption {
	return func(l *Logger) {
		l.masker = masker
	}
}

// WithActorResolver sets the actor resolver function.
func WithActorResolver(resolver func(ctx context.Context) string) LoggerOption {
	return func(l *Logger) {
		l.actorResolver = resolver
	}
}

// NewLogger creates a new audit logger.
func NewLogger(opts ...LoggerOption) *Logger {
	l := &Logger{
		masker: NewSecretMasker(),
	}

	for _, opt := range opts {
		opt(l)
	}

	return l
}

// OnEvent registers a callback for audit events.
func (l *Logger) OnEvent(callback func(event *Event)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.onEvent = append(l.onEvent, callback)
}

// Log records an audit event.
func (l *Logger) Log(ctx context.Context, event *Event) error {
	// Set defaults
	if event.ID == "" {
		event.ID = generateEventID()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Resolve actor
	if event.Actor == "" && l.actorResolver != nil {
		event.Actor = l.actorResolver(ctx)
	}

	// Mask secrets in details
	if l.masker != nil && event.Details != nil {
		event.Details = l.masker.MaskMap(event.Details)
	}

	// Store event
	if l.storage != nil {
		if err := l.storage.Store(ctx, event); err != nil {
			return fmt.Errorf("store audit event: %w", err)
		}
	}

	// Notify callbacks
	l.mu.RLock()
	callbacks := l.onEvent
	l.mu.RUnlock()

	for _, callback := range callbacks {
		callback(event)
	}

	return nil
}

// LogExecutionStart logs an execution start event.
func (l *Logger) LogExecutionStart(ctx context.Context, exec *runbook.Execution) error {
	return l.Log(ctx, &Event{
		Type:           EventExecutionStarted,
		ExecutionID:    exec.ID,
		RunbookName:    exec.RunbookName,
		RunbookVersion: exec.RunbookVersion,
		Outcome:        "started",
		Details: map[string]interface{}{
			"inputs": exec.Inputs,
		},
	})
}

// LogExecutionComplete logs an execution completion event.
func (l *Logger) LogExecutionComplete(ctx context.Context, exec *runbook.Execution) error {
	var duration time.Duration
	if exec.StartedAt != nil && exec.CompletedAt != nil {
		duration = exec.CompletedAt.Sub(*exec.StartedAt)
	}

	return l.Log(ctx, &Event{
		Type:           EventExecutionCompleted,
		ExecutionID:    exec.ID,
		RunbookName:    exec.RunbookName,
		RunbookVersion: exec.RunbookVersion,
		Outcome:        "success",
		Duration:       duration,
		Details: map[string]interface{}{
			"outputs":    exec.Outputs,
			"step_count": len(exec.Steps),
		},
	})
}

// LogExecutionFailed logs an execution failure event.
func (l *Logger) LogExecutionFailed(ctx context.Context, exec *runbook.Execution) error {
	var duration time.Duration
	if exec.StartedAt != nil && exec.CompletedAt != nil {
		duration = exec.CompletedAt.Sub(*exec.StartedAt)
	}

	return l.Log(ctx, &Event{
		Type:           EventExecutionFailed,
		ExecutionID:    exec.ID,
		RunbookName:    exec.RunbookName,
		RunbookVersion: exec.RunbookVersion,
		Outcome:        "failure",
		Duration:       duration,
		Error:          exec.Error,
	})
}

// LogStepStart logs a step start event.
func (l *Logger) LogStepStart(ctx context.Context, executionID, runbookName, stepName string) error {
	return l.Log(ctx, &Event{
		Type:        EventStepStarted,
		ExecutionID: executionID,
		RunbookName: runbookName,
		StepName:    stepName,
		Outcome:     "started",
	})
}

// LogStepComplete logs a step completion event.
func (l *Logger) LogStepComplete(ctx context.Context, executionID, runbookName string, step *runbook.StepExecution) error {
	return l.Log(ctx, &Event{
		Type:        EventStepCompleted,
		ExecutionID: executionID,
		RunbookName: runbookName,
		StepName:    step.Name,
		Outcome:     "success",
		Duration:    step.Duration,
		Details: map[string]interface{}{
			"outputs":     step.Outputs,
			"retry_count": step.RetryCount,
		},
	})
}

// LogStepFailed logs a step failure event.
func (l *Logger) LogStepFailed(ctx context.Context, executionID, runbookName string, step *runbook.StepExecution) error {
	return l.Log(ctx, &Event{
		Type:        EventStepFailed,
		ExecutionID: executionID,
		RunbookName: runbookName,
		StepName:    step.Name,
		Outcome:     "failure",
		Duration:    step.Duration,
		Error:       step.Error,
		Details: map[string]interface{}{
			"retry_count": step.RetryCount,
		},
	})
}

// Query searches for audit events.
func (l *Logger) Query(ctx context.Context, query *Query) ([]*Event, error) {
	if l.storage == nil {
		return nil, fmt.Errorf("no storage configured")
	}
	return l.storage.Query(ctx, query)
}

// GetExecutionHistory retrieves the complete audit trail for an execution.
func (l *Logger) GetExecutionHistory(ctx context.Context, executionID string) ([]*Event, error) {
	if l.storage == nil {
		return nil, fmt.Errorf("no storage configured")
	}
	return l.storage.GetByExecutionID(ctx, executionID)
}

// SecretMasker masks sensitive data in audit logs.
type SecretMasker struct {
	mu sync.RWMutex

	// Patterns to detect secrets
	patterns []*regexp.Regexp

	// Known secret keys
	secretKeys map[string]bool

	// Mask replacement string
	maskString string
}

// NewSecretMasker creates a new secret masker with default patterns.
func NewSecretMasker() *SecretMasker {
	m := &SecretMasker{
		secretKeys: make(map[string]bool),
		maskString: "***REDACTED***",
	}

	// Add default secret keys
	defaultKeys := []string{
		"password", "passwd", "secret", "token", "api_key", "apikey",
		"access_key", "private_key", "credential", "auth", "bearer",
		"authorization", "key", "cert", "certificate", "ssh_key",
	}
	for _, key := range defaultKeys {
		m.secretKeys[strings.ToLower(key)] = true
	}

	// Add default patterns
	defaultPatterns := []string{
		`(?i)password\s*[:=]\s*\S+`,
		`(?i)api[_-]?key\s*[:=]\s*\S+`,
		`(?i)secret\s*[:=]\s*\S+`,
		`(?i)token\s*[:=]\s*\S+`,
		`(?i)bearer\s+\S+`,
		`-----BEGIN [A-Z ]+-----[\s\S]*?-----END [A-Z ]+-----`,
	}
	for _, pattern := range defaultPatterns {
		if re, err := regexp.Compile(pattern); err == nil {
			m.patterns = append(m.patterns, re)
		}
	}

	return m
}

// AddSecretKey adds a key name to treat as secret.
func (m *SecretMasker) AddSecretKey(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.secretKeys[strings.ToLower(key)] = true
}

// AddPattern adds a regex pattern to detect secrets.
func (m *SecretMasker) AddPattern(pattern string) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid pattern: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.patterns = append(m.patterns, re)

	return nil
}

// SetMaskString sets the replacement string for masked values.
func (m *SecretMasker) SetMaskString(mask string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maskString = mask
}

// MaskString masks secrets in a string.
func (m *SecretMasker) MaskString(s string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := s
	for _, pattern := range m.patterns {
		result = pattern.ReplaceAllString(result, m.maskString)
	}

	return result
}

// MaskValue masks a value if it's a secret.
func (m *SecretMasker) MaskValue(key string, value interface{}) interface{} {
	m.mu.RLock()
	isSecret := m.secretKeys[strings.ToLower(key)]
	maskStr := m.maskString
	m.mu.RUnlock()

	if isSecret {
		return maskStr
	}

	// If string, apply pattern masking
	if s, ok := value.(string); ok {
		return m.MaskString(s)
	}

	return value
}

// MaskMap masks secrets in a map.
func (m *SecretMasker) MaskMap(data map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	for k, v := range data {
		switch val := v.(type) {
		case map[string]interface{}:
			result[k] = m.MaskMap(val)
		case string:
			result[k] = m.MaskValue(k, val)
		default:
			result[k] = m.MaskValue(k, val)
		}
	}

	return result
}

// VerifyMasking checks if a string contains any unmasked secrets.
// Returns a list of potential secret patterns found.
func (m *SecretMasker) VerifyMasking(s string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	found := make([]string, 0, len(m.patterns))
	for _, pattern := range m.patterns {
		matches := pattern.FindAllString(s, -1)
		found = append(found, matches...)
	}

	return found
}

// RetentionPolicy defines how long to keep audit events.
type RetentionPolicy struct {
	// DefaultRetention is the default retention period.
	DefaultRetention time.Duration `json:"default_retention"`

	// TypeRetention overrides retention for specific event types.
	TypeRetention map[EventType]time.Duration `json:"type_retention,omitempty"`

	// RunbookRetention overrides retention for specific runbooks.
	RunbookRetention map[string]time.Duration `json:"runbook_retention,omitempty"`
}

// GetRetention returns the retention period for an event.
func (p *RetentionPolicy) GetRetention(eventType EventType, runbookName string) time.Duration {
	// Check runbook-specific retention first
	if retention, ok := p.RunbookRetention[runbookName]; ok {
		return retention
	}

	// Check type-specific retention
	if retention, ok := p.TypeRetention[eventType]; ok {
		return retention
	}

	return p.DefaultRetention
}

// RetentionManager manages audit log retention.
type RetentionManager struct {
	storage Storage
	policy  *RetentionPolicy
}

// NewRetentionManager creates a new retention manager.
func NewRetentionManager(storage Storage, policy *RetentionPolicy) *RetentionManager {
	return &RetentionManager{
		storage: storage,
		policy:  policy,
	}
}

// Cleanup removes audit events that have exceeded their retention period.
func (m *RetentionManager) Cleanup(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-m.policy.DefaultRetention)
	return m.storage.Delete(ctx, cutoff)
}

// ComplianceReport represents a compliance report.
type ComplianceReport struct {
	// GeneratedAt is when the report was generated.
	GeneratedAt time.Time `json:"generated_at"`

	// Period is the reporting period.
	Period ReportPeriod `json:"period"`

	// Summary contains high-level metrics.
	Summary ReportSummary `json:"summary"`

	// ExecutionSummaries contains per-runbook breakdowns.
	ExecutionSummaries []ExecutionSummary `json:"execution_summaries"`

	// Violations contains any compliance violations.
	Violations []ComplianceViolation `json:"violations,omitempty"`
}

// ReportPeriod defines the time range for a report.
type ReportPeriod struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// ReportSummary contains high-level metrics.
type ReportSummary struct {
	TotalExecutions      int64         `json:"total_executions"`
	SuccessfulExecutions int64         `json:"successful_executions"`
	FailedExecutions     int64         `json:"failed_executions"`
	CancelledExecutions  int64         `json:"cancelled_executions"`
	TotalSteps           int64         `json:"total_steps"`
	FailedSteps          int64         `json:"failed_steps"`
	AverageExecutionTime time.Duration `json:"average_execution_time"`
	ApprovalRequests     int64         `json:"approval_requests"`
	SecretAccesses       int64         `json:"secret_accesses"`
}

// ExecutionSummary contains metrics for a specific runbook.
type ExecutionSummary struct {
	RunbookName          string        `json:"runbook_name"`
	TotalExecutions      int64         `json:"total_executions"`
	SuccessfulExecutions int64         `json:"successful_executions"`
	FailedExecutions     int64         `json:"failed_executions"`
	AverageExecutionTime time.Duration `json:"average_execution_time"`
}

// ComplianceViolation represents a compliance issue.
type ComplianceViolation struct {
	Type        string    `json:"type"`
	Severity    string    `json:"severity"`
	Description string    `json:"description"`
	ExecutionID string    `json:"execution_id,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
}

// ComplianceReporter generates compliance reports.
type ComplianceReporter struct {
	storage Storage
	masker  *SecretMasker
}

// NewComplianceReporter creates a new compliance reporter.
func NewComplianceReporter(storage Storage, masker *SecretMasker) *ComplianceReporter {
	return &ComplianceReporter{
		storage: storage,
		masker:  masker,
	}
}

// GenerateReport creates a compliance report for the given period.
func (r *ComplianceReporter) GenerateReport(ctx context.Context, start, end time.Time) (*ComplianceReport, error) {
	query := &Query{
		StartTime: &start,
		EndTime:   &end,
	}

	events, err := r.storage.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}

	report := &ComplianceReport{
		GeneratedAt: time.Now(),
		Period: ReportPeriod{
			Start: start,
			End:   end,
		},
	}

	// Calculate summary
	var (
		totalDuration  time.Duration
		executionCount int64
		runbookStats   = make(map[string]*ExecutionSummary)
	)

	for _, event := range events {
		switch event.Type {
		case EventExecutionStarted:
			report.Summary.TotalExecutions++
			executionCount++

			// Track per-runbook stats
			if _, ok := runbookStats[event.RunbookName]; !ok {
				runbookStats[event.RunbookName] = &ExecutionSummary{
					RunbookName: event.RunbookName,
				}
			}
			runbookStats[event.RunbookName].TotalExecutions++

		case EventExecutionCompleted:
			report.Summary.SuccessfulExecutions++
			totalDuration += event.Duration
			if stats, ok := runbookStats[event.RunbookName]; ok {
				stats.SuccessfulExecutions++
				stats.AverageExecutionTime = (stats.AverageExecutionTime + event.Duration) / 2
			}

		case EventExecutionFailed:
			report.Summary.FailedExecutions++
			totalDuration += event.Duration
			if stats, ok := runbookStats[event.RunbookName]; ok {
				stats.FailedExecutions++
			}

		case EventExecutionCancelled:
			report.Summary.CancelledExecutions++

		case EventStepStarted:
			report.Summary.TotalSteps++

		case EventStepFailed:
			report.Summary.FailedSteps++

		case EventApprovalRequested:
			report.Summary.ApprovalRequests++

		case EventSecretAccessed:
			report.Summary.SecretAccesses++

		default:
		}
	}

	if executionCount > 0 {
		report.Summary.AverageExecutionTime = totalDuration / time.Duration(executionCount)
	}

	// Convert map to slice
	for _, stats := range runbookStats {
		report.ExecutionSummaries = append(report.ExecutionSummaries, *stats)
	}

	// Check for violations
	report.Violations = r.checkViolations(events)

	return report, nil
}

func (r *ComplianceReporter) checkViolations(events []*Event) []ComplianceViolation {
	var violations []ComplianceViolation

	for _, event := range events {
		// Check for unmasked secrets in event details
		if event.Details != nil {
			detailsJSON, _ := json.Marshal(event.Details)
			if unmasked := r.masker.VerifyMasking(string(detailsJSON)); len(unmasked) > 0 {
				violations = append(violations, ComplianceViolation{
					Type:        "unmasked_secret",
					Severity:    "high",
					Description: fmt.Sprintf("Potential unmasked secrets in event details: %d patterns found", len(unmasked)),
					ExecutionID: event.ExecutionID,
					Timestamp:   event.Timestamp,
				})
			}
		}

		// Check for failed executions without error details
		if event.Type == EventExecutionFailed && event.Error == "" {
			violations = append(violations, ComplianceViolation{
				Type:        "missing_error_detail",
				Severity:    "medium",
				Description: "Failed execution without error message",
				ExecutionID: event.ExecutionID,
				Timestamp:   event.Timestamp,
			})
		}
	}

	return violations
}

// generateEventID creates a unique event ID.
func generateEventID() string {
	return fmt.Sprintf("evt-%d-%d", time.Now().UnixNano(), time.Now().Nanosecond()%1000)
}
