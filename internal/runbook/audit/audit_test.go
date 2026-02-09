package audit

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

func TestAuditLogger_Log(t *testing.T) {
	storage := NewMemoryStorage()
	logger := NewLogger(WithStorage(storage))

	event := &Event{
		Type:        EventExecutionStarted,
		ExecutionID: "exec-1",
		RunbookName: "test-runbook",
		Outcome:     "started",
	}

	err := logger.Log(context.Background(), event)
	if err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	// Event should be stored
	events := storage.All()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	if events[0].ID == "" {
		t.Error("Event ID should be auto-generated")
	}
	if events[0].Timestamp.IsZero() {
		t.Error("Event timestamp should be auto-set")
	}
}

func TestAuditLogger_OnEvent(t *testing.T) {
	logger := NewLogger()

	var receivedEvent *Event
	logger.OnEvent(func(event *Event) {
		receivedEvent = event
	})

	event := &Event{
		Type:        EventStepCompleted,
		ExecutionID: "exec-1",
	}

	_ = logger.Log(context.Background(), event)

	if receivedEvent == nil {
		t.Fatal("OnEvent callback not called")
	}
	if receivedEvent.Type != EventStepCompleted {
		t.Errorf("Type = %v, want %v", receivedEvent.Type, EventStepCompleted)
	}
}

func TestAuditLogger_ActorResolver(t *testing.T) {
	storage := NewMemoryStorage()
	logger := NewLogger(
		WithStorage(storage),
		WithActorResolver(func(ctx context.Context) string {
			return "test-user"
		}),
	)

	event := &Event{
		Type:        EventExecutionStarted,
		ExecutionID: "exec-1",
	}

	_ = logger.Log(context.Background(), event)

	events := storage.All()
	if events[0].Actor != "test-user" {
		t.Errorf("Actor = %v, want test-user", events[0].Actor)
	}
}

func TestAuditLogger_LogExecutionStart(t *testing.T) {
	storage := NewMemoryStorage()
	logger := NewLogger(WithStorage(storage))

	exec := &runbook.Execution{
		ID:             "exec-1",
		RunbookName:    "test-runbook",
		RunbookVersion: "1.0",
		Inputs: map[string]interface{}{
			"param1": "value1",
		},
	}

	err := logger.LogExecutionStart(context.Background(), exec)
	if err != nil {
		t.Fatalf("LogExecutionStart() error = %v", err)
	}

	events := storage.All()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	if events[0].Type != EventExecutionStarted {
		t.Errorf("Type = %v, want %v", events[0].Type, EventExecutionStarted)
	}
	if events[0].ExecutionID != "exec-1" {
		t.Errorf("ExecutionID = %v, want exec-1", events[0].ExecutionID)
	}
}

func TestAuditLogger_LogExecutionComplete(t *testing.T) {
	storage := NewMemoryStorage()
	logger := NewLogger(WithStorage(storage))

	startTime := time.Now()
	endTime := startTime.Add(5 * time.Second)

	exec := &runbook.Execution{
		ID:          "exec-1",
		RunbookName: "test-runbook",
		StartedAt:   &startTime,
		CompletedAt: &endTime,
		Outputs: map[string]interface{}{
			"result": "success",
		},
		Steps: map[string]*runbook.StepExecution{
			"step1": {},
			"step2": {},
		},
	}

	err := logger.LogExecutionComplete(context.Background(), exec)
	if err != nil {
		t.Fatalf("LogExecutionComplete() error = %v", err)
	}

	events := storage.All()
	if events[0].Type != EventExecutionCompleted {
		t.Errorf("Type = %v, want %v", events[0].Type, EventExecutionCompleted)
	}
	if events[0].Outcome != "success" {
		t.Errorf("Outcome = %v, want success", events[0].Outcome)
	}
	if events[0].Duration != 5*time.Second {
		t.Errorf("Duration = %v, want 5s", events[0].Duration)
	}
}

func TestAuditLogger_LogExecutionFailed(t *testing.T) {
	storage := NewMemoryStorage()
	logger := NewLogger(WithStorage(storage))

	exec := &runbook.Execution{
		ID:          "exec-1",
		RunbookName: "test-runbook",
		Error:       "something went wrong",
	}

	err := logger.LogExecutionFailed(context.Background(), exec)
	if err != nil {
		t.Fatalf("LogExecutionFailed() error = %v", err)
	}

	events := storage.All()
	if events[0].Type != EventExecutionFailed {
		t.Errorf("Type = %v, want %v", events[0].Type, EventExecutionFailed)
	}
	if events[0].Error != "something went wrong" {
		t.Errorf("Error = %v, want 'something went wrong'", events[0].Error)
	}
}

func TestAuditLogger_Query(t *testing.T) {
	storage := NewMemoryStorage()
	logger := NewLogger(WithStorage(storage))

	// Add some events
	for i := 0; i < 5; i++ {
		_ = logger.Log(context.Background(), &Event{
			Type:        EventExecutionStarted,
			ExecutionID: "exec-1",
			RunbookName: "runbook-a",
		})
	}
	for i := 0; i < 3; i++ {
		_ = logger.Log(context.Background(), &Event{
			Type:        EventExecutionStarted,
			ExecutionID: "exec-2",
			RunbookName: "runbook-b",
		})
	}

	// Query by execution ID
	results, err := logger.Query(context.Background(), &Query{
		ExecutionID: "exec-1",
	})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(results) != 5 {
		t.Errorf("Query by execution ID: got %d, want 5", len(results))
	}

	// Query by runbook name
	results, err = logger.Query(context.Background(), &Query{
		RunbookName: "runbook-b",
	})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(results) != 3 {
		t.Errorf("Query by runbook: got %d, want 3", len(results))
	}

	// Query with limit
	results, err = logger.Query(context.Background(), &Query{
		Limit: 2,
	})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Query with limit: got %d, want 2", len(results))
	}
}

func TestAuditLogger_GetExecutionHistory(t *testing.T) {
	storage := NewMemoryStorage()
	logger := NewLogger(WithStorage(storage))

	// Log events for one execution
	_ = logger.Log(context.Background(), &Event{
		Type:        EventExecutionStarted,
		ExecutionID: "exec-1",
	})
	_ = logger.Log(context.Background(), &Event{
		Type:        EventStepStarted,
		ExecutionID: "exec-1",
		StepName:    "step1",
	})
	_ = logger.Log(context.Background(), &Event{
		Type:        EventStepCompleted,
		ExecutionID: "exec-1",
		StepName:    "step1",
	})
	_ = logger.Log(context.Background(), &Event{
		Type:        EventExecutionCompleted,
		ExecutionID: "exec-1",
	})

	// Log events for another execution
	_ = logger.Log(context.Background(), &Event{
		Type:        EventExecutionStarted,
		ExecutionID: "exec-2",
	})

	history, err := logger.GetExecutionHistory(context.Background(), "exec-1")
	if err != nil {
		t.Fatalf("GetExecutionHistory() error = %v", err)
	}
	if len(history) != 4 {
		t.Errorf("GetExecutionHistory() got %d events, want 4", len(history))
	}
}

func TestSecretMasker_MaskString(t *testing.T) {
	masker := NewSecretMasker()

	tests := []struct {
		input string
		want  string
	}{
		{"password: secret123", "***REDACTED***"},
		{"api_key=abcd1234", "***REDACTED***"},
		{"no secrets here", "no secrets here"},
		{"bearer abc123token", "***REDACTED***"},
	}

	for _, tt := range tests {
		got := masker.MaskString(tt.input)
		if got != tt.want {
			t.Errorf("MaskString(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSecretMasker_MaskValue(t *testing.T) {
	masker := NewSecretMasker()

	tests := []struct {
		key   string
		value interface{}
		want  interface{}
	}{
		{"password", "secret123", "***REDACTED***"},
		{"api_key", "abc123", "***REDACTED***"},
		{"username", "admin", "admin"},
		{"token", "xyz789", "***REDACTED***"},
	}

	for _, tt := range tests {
		got := masker.MaskValue(tt.key, tt.value)
		if got != tt.want {
			t.Errorf("MaskValue(%q, %v) = %v, want %v", tt.key, tt.value, got, tt.want)
		}
	}
}

func TestSecretMasker_MaskMap(t *testing.T) {
	masker := NewSecretMasker()

	input := map[string]interface{}{
		"username": "admin",
		"password": "secret123",
		"config": map[string]interface{}{
			"api_key": "abc123",
			"host":    "localhost",
		},
	}

	result := masker.MaskMap(input)

	if result["username"] != "admin" {
		t.Errorf("username should not be masked")
	}
	if result["password"] != "***REDACTED***" {
		t.Errorf("password should be masked")
	}

	config := result["config"].(map[string]interface{})
	if config["api_key"] != "***REDACTED***" {
		t.Errorf("api_key should be masked")
	}
	if config["host"] != "localhost" {
		t.Errorf("host should not be masked")
	}
}

func TestSecretMasker_VerifyMasking(t *testing.T) {
	masker := NewSecretMasker()

	// String with secrets
	found := masker.VerifyMasking("password: secret123")
	if len(found) == 0 {
		t.Error("Should find secrets in string")
	}

	// String without secrets
	found = masker.VerifyMasking("no secrets here")
	if len(found) != 0 {
		t.Errorf("Should not find secrets, found: %v", found)
	}
}

func TestSecretMasker_AddSecretKey(t *testing.T) {
	masker := NewSecretMasker()
	masker.AddSecretKey("custom_secret")

	value := masker.MaskValue("custom_secret", "myvalue")
	if value != "***REDACTED***" {
		t.Errorf("Custom secret key should be masked")
	}
}

func TestSecretMasker_AddPattern(t *testing.T) {
	masker := NewSecretMasker()
	err := masker.AddPattern(`SSN:\s*\d{3}-\d{2}-\d{4}`)
	if err != nil {
		t.Fatalf("AddPattern() error = %v", err)
	}

	result := masker.MaskString("SSN: 123-45-6789")
	if result != "***REDACTED***" {
		t.Errorf("Custom pattern should be masked")
	}
}

func TestAuditLogger_SecretMasking(t *testing.T) {
	storage := NewMemoryStorage()
	masker := NewSecretMasker()
	logger := NewLogger(
		WithStorage(storage),
		WithSecretMasker(masker),
	)

	event := &Event{
		Type:        EventInputProvided,
		ExecutionID: "exec-1",
		Details: map[string]interface{}{
			"username": "admin",
			"password": "secret123",
		},
	}

	_ = logger.Log(context.Background(), event)

	events := storage.All()
	details := events[0].Details

	if details["password"] != "***REDACTED***" {
		t.Errorf("Password should be masked in stored event")
	}
}

func TestRetentionPolicy_GetRetention(t *testing.T) {
	policy := &RetentionPolicy{
		DefaultRetention: 30 * 24 * time.Hour, // 30 days
		TypeRetention: map[EventType]time.Duration{
			EventSecretAccessed: 90 * 24 * time.Hour, // 90 days
		},
		RunbookRetention: map[string]time.Duration{
			"critical-runbook": 365 * 24 * time.Hour, // 1 year
		},
	}

	// Default retention
	if got := policy.GetRetention(EventExecutionStarted, "normal-runbook"); got != 30*24*time.Hour {
		t.Errorf("Default retention = %v, want 30 days", got)
	}

	// Type-specific retention
	if got := policy.GetRetention(EventSecretAccessed, "normal-runbook"); got != 90*24*time.Hour {
		t.Errorf("Type retention = %v, want 90 days", got)
	}

	// Runbook-specific retention (takes precedence)
	if got := policy.GetRetention(EventExecutionStarted, "critical-runbook"); got != 365*24*time.Hour {
		t.Errorf("Runbook retention = %v, want 365 days", got)
	}
}

func TestRetentionManager_Cleanup(t *testing.T) {
	storage := NewMemoryStorage()
	policy := &RetentionPolicy{
		DefaultRetention: 24 * time.Hour,
	}
	manager := NewRetentionManager(storage, policy)

	// Add old event
	oldTime := time.Now().Add(-48 * time.Hour)
	_ = storage.Store(context.Background(), &Event{
		ID:        "old-event",
		Timestamp: oldTime,
	})

	// Add recent event
	_ = storage.Store(context.Background(), &Event{
		ID:        "recent-event",
		Timestamp: time.Now(),
	})

	deleted, err := manager.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}

	if deleted != 1 {
		t.Errorf("Cleanup() deleted %d, want 1", deleted)
	}

	events := storage.All()
	if len(events) != 1 {
		t.Errorf("After cleanup: %d events, want 1", len(events))
	}
	if events[0].ID != "recent-event" {
		t.Errorf("Wrong event kept")
	}
}

func TestComplianceReporter_GenerateReport(t *testing.T) {
	storage := NewMemoryStorage()
	masker := NewSecretMasker()
	reporter := NewComplianceReporter(storage, masker)

	now := time.Now()
	start := now.Add(-24 * time.Hour)

	// Add various events
	_ = storage.Store(context.Background(), &Event{
		Type:        EventExecutionStarted,
		ExecutionID: "exec-1",
		RunbookName: "runbook-a",
		Timestamp:   now.Add(-1 * time.Hour),
	})
	_ = storage.Store(context.Background(), &Event{
		Type:        EventExecutionCompleted,
		ExecutionID: "exec-1",
		RunbookName: "runbook-a",
		Timestamp:   now,
		Duration:    5 * time.Minute,
	})
	_ = storage.Store(context.Background(), &Event{
		Type:        EventExecutionStarted,
		ExecutionID: "exec-2",
		RunbookName: "runbook-b",
		Timestamp:   now.Add(-30 * time.Minute),
	})
	_ = storage.Store(context.Background(), &Event{
		Type:        EventExecutionFailed,
		ExecutionID: "exec-2",
		RunbookName: "runbook-b",
		Timestamp:   now,
		Duration:    10 * time.Minute,
		Error:       "test error",
	})
	_ = storage.Store(context.Background(), &Event{
		Type:        EventStepStarted,
		ExecutionID: "exec-1",
		Timestamp:   now.Add(-50 * time.Minute),
	})
	_ = storage.Store(context.Background(), &Event{
		Type:        EventStepFailed,
		ExecutionID: "exec-2",
		Timestamp:   now.Add(-20 * time.Minute),
	})

	report, err := reporter.GenerateReport(context.Background(), start, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("GenerateReport() error = %v", err)
	}

	if report.Summary.TotalExecutions != 2 {
		t.Errorf("TotalExecutions = %d, want 2", report.Summary.TotalExecutions)
	}
	if report.Summary.SuccessfulExecutions != 1 {
		t.Errorf("SuccessfulExecutions = %d, want 1", report.Summary.SuccessfulExecutions)
	}
	if report.Summary.FailedExecutions != 1 {
		t.Errorf("FailedExecutions = %d, want 1", report.Summary.FailedExecutions)
	}
	if report.Summary.TotalSteps != 1 {
		t.Errorf("TotalSteps = %d, want 1", report.Summary.TotalSteps)
	}
	if report.Summary.FailedSteps != 1 {
		t.Errorf("FailedSteps = %d, want 1", report.Summary.FailedSteps)
	}

	if len(report.ExecutionSummaries) != 2 {
		t.Errorf("ExecutionSummaries len = %d, want 2", len(report.ExecutionSummaries))
	}
}

func TestComplianceReporter_ViolationDetection(t *testing.T) {
	storage := NewMemoryStorage()
	masker := NewSecretMasker()
	reporter := NewComplianceReporter(storage, masker)

	now := time.Now()

	// Add event with potential unmasked secret
	_ = storage.Store(context.Background(), &Event{
		Type:        EventInputProvided,
		ExecutionID: "exec-1",
		Timestamp:   now,
		Details: map[string]interface{}{
			"config": "password: secret123",
		},
	})

	// Add failed execution without error message
	_ = storage.Store(context.Background(), &Event{
		Type:        EventExecutionFailed,
		ExecutionID: "exec-2",
		Timestamp:   now,
		Error:       "", // Missing error
	})

	report, err := reporter.GenerateReport(context.Background(), now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("GenerateReport() error = %v", err)
	}

	if len(report.Violations) == 0 {
		t.Error("Should detect violations")
	}

	// Check for missing error detail violation
	foundMissingError := false
	for _, v := range report.Violations {
		if v.Type == "missing_error_detail" {
			foundMissingError = true
			break
		}
	}
	if !foundMissingError {
		t.Error("Should detect missing_error_detail violation")
	}
}

func TestMemoryStorage_Query_Ordering(t *testing.T) {
	storage := NewMemoryStorage()

	// Add events with different timestamps
	for i := 0; i < 5; i++ {
		_ = storage.Store(context.Background(), &Event{
			ID:        string(rune('a' + i)),
			Timestamp: time.Now().Add(time.Duration(i) * time.Minute),
			Type:      EventExecutionStarted,
		})
	}

	// Query with descending order
	results, _ := storage.Query(context.Background(), &Query{
		OrderBy:   "timestamp",
		OrderDesc: true,
	})

	if len(results) != 5 {
		t.Fatalf("Expected 5 results, got %d", len(results))
	}

	// First result should be the latest
	if results[0].ID != "e" {
		t.Errorf("First result ID = %v, want e", results[0].ID)
	}
}

func TestMemoryStorage_Query_OffsetLimit(t *testing.T) {
	storage := NewMemoryStorage()

	for i := 0; i < 10; i++ {
		_ = storage.Store(context.Background(), &Event{
			ID:   string(rune('a' + i)),
			Type: EventExecutionStarted,
		})
	}

	// Query with offset and limit
	results, _ := storage.Query(context.Background(), &Query{
		Offset: 3,
		Limit:  4,
	})

	if len(results) != 4 {
		t.Errorf("Expected 4 results, got %d", len(results))
	}

	if results[0].ID != "d" {
		t.Errorf("First result after offset = %v, want d", results[0].ID)
	}
}

func TestBuildExecutionHistoryView(t *testing.T) {
	now := time.Now()
	events := []*Event{
		{
			Type:           EventExecutionStarted,
			ExecutionID:    "exec-1",
			RunbookName:    "test-runbook",
			RunbookVersion: "1.0",
			Timestamp:      now,
			Actor:          "user1",
		},
		{
			Type:        EventStepStarted,
			ExecutionID: "exec-1",
			StepName:    "step1",
			Timestamp:   now.Add(1 * time.Second),
		},
		{
			Type:        EventStepCompleted,
			ExecutionID: "exec-1",
			StepName:    "step1",
			Timestamp:   now.Add(5 * time.Second),
			Duration:    4 * time.Second,
		},
		{
			Type:        EventStepStarted,
			ExecutionID: "exec-1",
			StepName:    "step2",
			Timestamp:   now.Add(6 * time.Second),
		},
		{
			Type:        EventStepFailed,
			ExecutionID: "exec-1",
			StepName:    "step2",
			Timestamp:   now.Add(10 * time.Second),
			Error:       "step2 failed",
		},
		{
			Type:        EventExecutionFailed,
			ExecutionID: "exec-1",
			Timestamp:   now.Add(11 * time.Second),
			Duration:    11 * time.Second,
		},
	}

	view := BuildExecutionHistoryView(events)

	if view.ExecutionID != "exec-1" {
		t.Errorf("ExecutionID = %v, want exec-1", view.ExecutionID)
	}
	if view.RunbookName != "test-runbook" {
		t.Errorf("RunbookName = %v, want test-runbook", view.RunbookName)
	}
	if view.Status != "failed" {
		t.Errorf("Status = %v, want failed", view.Status)
	}
	if view.Actor != "user1" {
		t.Errorf("Actor = %v, want user1", view.Actor)
	}
	if len(view.Steps) != 2 {
		t.Errorf("Steps count = %d, want 2", len(view.Steps))
	}

	// Check step1
	var step1, step2 *StepHistoryView
	for i := range view.Steps {
		if view.Steps[i].Name == "step1" {
			step1 = &view.Steps[i]
		}
		if view.Steps[i].Name == "step2" {
			step2 = &view.Steps[i]
		}
	}

	if step1 == nil || step1.Status != "completed" {
		t.Error("step1 should be completed")
	}
	if step2 == nil || step2.Status != "failed" {
		t.Error("step2 should be failed")
	}
	if step2.Error != "step2 failed" {
		t.Errorf("step2 error = %v, want 'step2 failed'", step2.Error)
	}
}
