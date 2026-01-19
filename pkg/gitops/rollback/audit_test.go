package rollback

import (
	"context"
	"testing"
	"time"
)

func TestNewAuditTrail(t *testing.T) {
	trail := NewAuditTrail()
	if trail == nil {
		t.Fatal("expected non-nil audit trail")
	}
	if trail.entries == nil {
		t.Error("expected entries map to be initialized")
	}
	if trail.entryIndex == nil {
		t.Error("expected entry index to be initialized")
	}
}

func TestAuditTrail_Record(t *testing.T) {
	trail := NewAuditTrail()

	entry := &AuditEntry{
		RollbackID: "rollback-1",
		EventType:  AuditEventRollbackRequested,
		Actor:      "user1",
		ActorType:  ActorTypeUser,
		Reason:     "Production issue",
	}

	err := trail.Record(entry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify ID was generated
	if entry.ID == "" {
		t.Error("expected ID to be generated")
	}

	// Verify timestamp was set
	if entry.Timestamp.IsZero() {
		t.Error("expected timestamp to be set")
	}

	// Verify entry is retrievable
	entries := trail.GetEntriesForRollback("rollback-1")
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}
}

func TestAuditTrail_Record_Errors(t *testing.T) {
	trail := NewAuditTrail()

	// Missing rollback ID
	err := trail.Record(&AuditEntry{
		EventType: AuditEventRollbackRequested,
	})
	if err == nil {
		t.Error("expected error for missing rollback ID")
	}

	// Missing event type
	err = trail.Record(&AuditEntry{
		RollbackID: "rollback-1",
	})
	if err == nil {
		t.Error("expected error for missing event type")
	}
}

func TestAuditTrail_RecordEvent(t *testing.T) {
	trail := NewAuditTrail()

	err := trail.RecordEvent("rollback-1", AuditEventStarted, "system", "Starting rollback")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries := trail.GetEntriesForRollback("rollback-1")
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	if entries[0].EventType != AuditEventStarted {
		t.Errorf("expected event type 'started', got '%s'", entries[0].EventType)
	}
	if entries[0].Reason != "Starting rollback" {
		t.Errorf("expected reason 'Starting rollback', got '%s'", entries[0].Reason)
	}
}

func TestAuditTrail_RecordStatusChange(t *testing.T) {
	trail := NewAuditTrail()

	err := trail.RecordStatusChange(
		"rollback-1",
		AuditEventApproved,
		"admin",
		StatusPending,
		StatusApproved,
		"Approved for production",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries := trail.GetEntriesForRollback("rollback-1")
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	if entries[0].PreviousStatus != StatusPending {
		t.Errorf("expected previous status 'pending', got '%s'", entries[0].PreviousStatus)
	}
	if entries[0].NewStatus != StatusApproved {
		t.Errorf("expected new status 'approved', got '%s'", entries[0].NewStatus)
	}
}

func TestAuditTrail_GetEntry(t *testing.T) {
	trail := NewAuditTrail()

	entry := &AuditEntry{
		RollbackID: "rollback-1",
		EventType:  AuditEventRollbackRequested,
	}
	trail.Record(entry)

	retrieved, ok := trail.GetEntry(entry.ID)
	if !ok {
		t.Error("expected entry to exist")
	}
	if retrieved.RollbackID != "rollback-1" {
		t.Errorf("expected rollback ID 'rollback-1', got '%s'", retrieved.RollbackID)
	}

	// Non-existent entry
	_, ok = trail.GetEntry("non-existent")
	if ok {
		t.Error("expected entry not to exist")
	}
}

func TestAuditTrail_Query(t *testing.T) {
	trail := NewAuditTrail()

	// Add multiple entries
	now := time.Now()
	trail.Record(&AuditEntry{
		RollbackID: "rollback-1",
		EventType:  AuditEventRollbackRequested,
		Actor:      "user1",
		Timestamp:  now.Add(-2 * time.Hour),
	})
	trail.Record(&AuditEntry{
		RollbackID: "rollback-1",
		EventType:  AuditEventApproved,
		Actor:      "admin",
		Timestamp:  now.Add(-1 * time.Hour),
	})
	trail.Record(&AuditEntry{
		RollbackID: "rollback-2",
		EventType:  AuditEventRollbackRequested,
		Actor:      "user2",
		Timestamp:  now,
	})

	// Query by rollback ID
	results := trail.Query(&AuditFilter{
		RollbackIDs: []string{"rollback-1"},
	})
	if len(results) != 2 {
		t.Errorf("expected 2 results for rollback-1, got %d", len(results))
	}

	// Query by event type
	results = trail.Query(&AuditFilter{
		EventTypes: []AuditEventType{AuditEventRollbackRequested},
	})
	if len(results) != 2 {
		t.Errorf("expected 2 rollback_requested events, got %d", len(results))
	}

	// Query by actor
	results = trail.Query(&AuditFilter{
		Actors: []string{"admin"},
	})
	if len(results) != 1 {
		t.Errorf("expected 1 result for admin, got %d", len(results))
	}

	// Query by time range
	results = trail.Query(&AuditFilter{
		StartTime: now.Add(-90 * time.Minute),
		EndTime:   now.Add(-30 * time.Minute),
	})
	if len(results) != 1 {
		t.Errorf("expected 1 result in time range, got %d", len(results))
	}

	// Query with limit
	results = trail.Query(&AuditFilter{
		Limit: 2,
	})
	if len(results) != 2 {
		t.Errorf("expected 2 results with limit, got %d", len(results))
	}
}

func TestAuditTrail_OnAuditEvent(t *testing.T) {
	trail := NewAuditTrail()

	var receivedEntry *AuditEntry
	trail.OnAuditEvent(func(entry *AuditEntry) {
		receivedEntry = entry
	})

	trail.Record(&AuditEntry{
		RollbackID: "rollback-1",
		EventType:  AuditEventStarted,
	})

	// Give callback time to execute
	time.Sleep(10 * time.Millisecond)

	if receivedEntry == nil {
		t.Error("expected callback to receive entry")
	}
	if receivedEntry.EventType != AuditEventStarted {
		t.Errorf("expected event type 'started', got '%s'", receivedEntry.EventType)
	}
}

func TestAuditTrail_GetTimeline(t *testing.T) {
	trail := NewAuditTrail()

	now := time.Now()
	trail.Record(&AuditEntry{
		RollbackID: "rollback-1",
		EventType:  AuditEventRollbackRequested,
		Actor:      "user1",
		Timestamp:  now.Add(-3 * time.Hour),
		NewStatus:  StatusPending,
	})
	trail.Record(&AuditEntry{
		RollbackID: "rollback-1",
		EventType:  AuditEventApproved,
		Actor:      "admin",
		Timestamp:  now.Add(-2 * time.Hour),
		NewStatus:  StatusApproved,
	})
	trail.Record(&AuditEntry{
		RollbackID: "rollback-1",
		EventType:  AuditEventCompleted,
		Actor:      "system",
		Timestamp:  now.Add(-1 * time.Hour),
		NewStatus:  StatusCompleted,
		Details: &AuditDetails{
			FromRevision: "abc123",
			ToRevision:   "def456",
			Duration:     30 * time.Second,
		},
	})

	timeline := trail.GetTimeline("rollback-1")

	if timeline.RollbackID != "rollback-1" {
		t.Errorf("expected rollback ID 'rollback-1', got '%s'", timeline.RollbackID)
	}
	if len(timeline.Events) != 3 {
		t.Errorf("expected 3 events, got %d", len(timeline.Events))
	}
	if timeline.FinalStatus != StatusCompleted {
		t.Errorf("expected final status 'completed', got '%s'", timeline.FinalStatus)
	}

	// Check events are in chronological order
	for i := 1; i < len(timeline.Events); i++ {
		if timeline.Events[i].Timestamp.Before(timeline.Events[i-1].Timestamp) {
			t.Error("expected events to be in chronological order")
		}
	}
}

func TestAuditTrail_DescribeEvent(t *testing.T) {
	trail := NewAuditTrail()

	testCases := []struct {
		entry    *AuditEntry
		contains string
	}{
		{
			entry: &AuditEntry{
				EventType: AuditEventRollbackRequested,
				Actor:     "user1",
				Reason:    "Production bug",
			},
			contains: "Rollback requested by user1",
		},
		{
			entry: &AuditEntry{
				EventType: AuditEventApproved,
				Actor:     "admin",
			},
			contains: "Approved by admin",
		},
		{
			entry: &AuditEntry{
				EventType: AuditEventRejected,
				Actor:     "admin",
				Reason:    "Not ready",
			},
			contains: "Rejected by admin",
		},
		{
			entry: &AuditEntry{
				EventType: AuditEventCompleted,
				Details: &AuditDetails{
					FromRevision: "abc",
					ToRevision:   "def",
					Duration:     30 * time.Second,
				},
			},
			contains: "abc -> def",
		},
		{
			entry: &AuditEntry{
				EventType: AuditEventFailed,
				Details: &AuditDetails{
					ErrorMessage: "Connection refused",
				},
			},
			contains: "Connection refused",
		},
	}

	for _, tc := range testCases {
		desc := trail.describeEvent(tc.entry)
		if !containsStr(desc, tc.contains) {
			t.Errorf("expected description to contain '%s', got '%s'", tc.contains, desc)
		}
	}
}

func TestAuditTrail_GetSummary(t *testing.T) {
	trail := NewAuditTrail()
	ctx := context.Background()

	now := time.Now()
	trail.Record(&AuditEntry{
		RollbackID: "rollback-1",
		EventType:  AuditEventRollbackRequested,
		Actor:      "user1",
		Timestamp:  now.Add(-2 * time.Hour),
		Details:    &AuditDetails{Application: "app1"},
	})
	trail.Record(&AuditEntry{
		RollbackID: "rollback-1",
		EventType:  AuditEventApprovalRequested,
		Timestamp:  now.Add(-110 * time.Minute),
	})
	trail.Record(&AuditEntry{
		RollbackID: "rollback-1",
		EventType:  AuditEventApproved,
		Actor:      "admin",
		Timestamp:  now.Add(-100 * time.Minute),
		NewStatus:  StatusApproved,
	})
	trail.Record(&AuditEntry{
		RollbackID: "rollback-1",
		EventType:  AuditEventCompleted,
		Actor:      "system",
		Timestamp:  now.Add(-90 * time.Minute),
		NewStatus:  StatusCompleted,
	})

	summary := trail.GetSummary(ctx, now.Add(-3*time.Hour), now)

	if summary.TotalRollbacks != 1 {
		t.Errorf("expected 1 total rollback, got %d", summary.TotalRollbacks)
	}
	if summary.ByStatus[StatusCompleted] != 1 {
		t.Errorf("expected 1 completed, got %d", summary.ByStatus[StatusCompleted])
	}
	if summary.ByApplication["app1"] != 1 {
		t.Errorf("expected 1 for app1, got %d", summary.ByApplication["app1"])
	}
	if summary.ByActor["admin"] != 1 {
		t.Errorf("expected 1 for admin, got %d", summary.ByActor["admin"])
	}
}

func TestAuditTrail_ExportJSON(t *testing.T) {
	trail := NewAuditTrail()

	trail.Record(&AuditEntry{
		RollbackID: "rollback-1",
		EventType:  AuditEventRollbackRequested,
		Actor:      "user1",
	})

	data, err := trail.ExportJSON(&AuditFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(data) == 0 {
		t.Error("expected non-empty JSON output")
	}

	jsonStr := string(data)
	if !containsStr(jsonStr, "rollback-1") {
		t.Error("expected JSON to contain rollback ID")
	}
}

func TestNewAuditingEngine(t *testing.T) {
	engine := NewAuditingEngine()

	if engine == nil {
		t.Fatal("expected non-nil engine")
	}
	if engine.Engine == nil {
		t.Error("expected embedded engine")
	}
	if engine.auditTrail == nil {
		t.Error("expected audit trail")
	}
}

func TestAuditingEngine_GetAuditTrail(t *testing.T) {
	engine := NewAuditingEngine()
	trail := engine.GetAuditTrail()

	if trail == nil {
		t.Error("expected non-nil audit trail")
	}
}

func TestAuditingEngine_AddComment(t *testing.T) {
	engine := NewAuditingEngine()

	err := engine.AddComment("rollback-1", "user1", "This is a test comment")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries := engine.GetAuditTrail().GetEntriesForRollback("rollback-1")
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	if entries[0].EventType != AuditEventComment {
		t.Errorf("expected event type 'comment', got '%s'", entries[0].EventType)
	}
	if entries[0].Reason != "This is a test comment" {
		t.Errorf("expected comment text, got '%s'", entries[0].Reason)
	}
}

func TestAuditDetails_Fields(t *testing.T) {
	details := &AuditDetails{
		Application:  "my-app",
		Namespace:    "production",
		FromRevision: "abc123",
		ToRevision:   "def456",
		Strategy:     StrategyPreviousRevision,
		Duration:     30 * time.Second,
		ErrorMessage: "test error",
		ErrorCode:    "ERR001",
		ApprovalChain: []ApprovalRecord{
			{Approver: "admin", Decision: "approved"},
		},
		AffectedResources: []AffectedResource{
			{Kind: "Deployment", Name: "web", Action: "reverted"},
		},
		GitDetails: &GitDetails{
			Repository: "org/repo",
			Branch:     "main",
		},
	}

	if details.Application != "my-app" {
		t.Error("expected application to be preserved")
	}
	if len(details.ApprovalChain) != 1 {
		t.Error("expected approval chain to be preserved")
	}
	if len(details.AffectedResources) != 1 {
		t.Error("expected affected resources to be preserved")
	}
	if details.GitDetails.Repository != "org/repo" {
		t.Error("expected git details to be preserved")
	}
}

func TestVerificationResults_Fields(t *testing.T) {
	results := &VerificationResults{
		Passed:       true,
		TotalChecks:  10,
		PassedChecks: 9,
		FailedChecks: 1,
		Duration:     5 * time.Second,
		CheckResults: []CheckResult{
			{Name: "health_check", Passed: true},
		},
	}

	if !results.Passed {
		t.Error("expected passed to be true")
	}
	if len(results.CheckResults) != 1 {
		t.Error("expected check results")
	}
}

func TestActorType_Values(t *testing.T) {
	types := []ActorType{
		ActorTypeUser,
		ActorTypeSystem,
		ActorTypeAPI,
		ActorTypeWebhook,
	}

	for _, typ := range types {
		if typ == "" {
			t.Error("expected non-empty actor type")
		}
	}
}

func TestAuditEventType_Values(t *testing.T) {
	types := []AuditEventType{
		AuditEventRollbackRequested,
		AuditEventApprovalRequested,
		AuditEventApproved,
		AuditEventRejected,
		AuditEventStarted,
		AuditEventCompleted,
		AuditEventFailed,
		AuditEventVerificationStarted,
		AuditEventVerificationPassed,
		AuditEventVerificationFailed,
		AuditEventCancelled,
		AuditEventTimeout,
		AuditEventRetry,
		AuditEventComment,
	}

	for _, typ := range types {
		if typ == "" {
			t.Error("expected non-empty event type")
		}
	}
}

func TestAuditFilter_Pagination(t *testing.T) {
	trail := NewAuditTrail()

	// Add 10 entries
	for i := 0; i < 10; i++ {
		trail.Record(&AuditEntry{
			RollbackID: "rollback-1",
			EventType:  AuditEventComment,
			Reason:     string(rune('A' + i)),
			Timestamp:  time.Now().Add(time.Duration(i) * time.Minute),
		})
	}

	// Get first page
	results := trail.Query(&AuditFilter{
		Limit:  3,
		Offset: 0,
	})
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}

	// Get second page
	results = trail.Query(&AuditFilter{
		Limit:  3,
		Offset: 3,
	})
	if len(results) != 3 {
		t.Errorf("expected 3 results for page 2, got %d", len(results))
	}

	// Get last page
	results = trail.Query(&AuditFilter{
		Limit:  3,
		Offset: 9,
	})
	if len(results) != 1 {
		t.Errorf("expected 1 result for last page, got %d", len(results))
	}
}

// Helper function
func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
