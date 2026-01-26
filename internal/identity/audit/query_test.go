package audit

import (
	"context"
	"testing"
	"time"
)

func TestMemoryStore_Store(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	event := &Event{
		ID:        "event-1",
		Type:      EventTypeLogin,
		Timestamp: time.Now(),
		Actor:     &Actor{ID: "user-1", Type: "user"},
		Result:    ResultSuccess,
	}

	err := store.Store(context.Background(), event)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	if store.Count() != 1 {
		t.Errorf("Count = %d, want 1", store.Count())
	}
}

func TestMemoryStore_Get(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	event := &Event{
		ID:        "event-1",
		Type:      EventTypeLogin,
		Timestamp: time.Now(),
	}

	store.Store(context.Background(), event)

	ctx := context.Background()

	// Get existing
	got, err := store.Get(ctx, "event-1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.ID != event.ID {
		t.Errorf("ID = %s, want %s", got.ID, event.ID)
	}

	// Get non-existing
	_, err = store.Get(ctx, "nonexistent")
	if err != ErrEventNotFound {
		t.Errorf("Get non-existing = %v, want ErrEventNotFound", err)
	}
}

func TestMemoryStore_Query_EventTypes(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()

	// Store various events
	store.Store(ctx, &Event{ID: "1", Type: EventTypeLogin, Timestamp: time.Now()})
	store.Store(ctx, &Event{ID: "2", Type: EventTypeLogout, Timestamp: time.Now()})
	store.Store(ctx, &Event{ID: "3", Type: EventTypeLogin, Timestamp: time.Now()})
	store.Store(ctx, &Event{ID: "4", Type: EventTypePasswordChange, Timestamp: time.Now()})

	// Query logins only
	query := &Query{
		EventTypes: []EventType{EventTypeLogin},
	}

	result, err := store.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(result.Events) != 2 {
		t.Errorf("Got %d events, want 2", len(result.Events))
	}
}

func TestMemoryStore_Query_ActorFilter(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()

	store.Store(ctx, &Event{
		ID:        "1",
		Type:      EventTypeLogin,
		Timestamp: time.Now(),
		Actor:     &Actor{ID: "user-1", Type: "user"},
	})
	store.Store(ctx, &Event{
		ID:        "2",
		Type:      EventTypeLogin,
		Timestamp: time.Now(),
		Actor:     &Actor{ID: "user-2", Type: "user"},
	})
	store.Store(ctx, &Event{
		ID:        "3",
		Type:      EventTypeLogin,
		Timestamp: time.Now(),
		Actor:     &Actor{ID: "service-1", Type: "service"},
	})

	// Query by actor ID
	query := &Query{
		ActorIDs: []string{"user-1"},
	}

	result, err := store.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(result.Events) != 1 {
		t.Errorf("Got %d events, want 1", len(result.Events))
	}

	// Query by actor type
	query = &Query{
		ActorTypes: []string{"user"},
	}

	result, err = store.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(result.Events) != 2 {
		t.Errorf("Got %d events, want 2", len(result.Events))
	}
}

func TestMemoryStore_Query_TargetFilter(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()

	store.Store(ctx, &Event{
		ID:        "1",
		Type:      EventTypeRoleAssigned,
		Timestamp: time.Now(),
		Target:    &Target{ID: "user-1", Type: "user"},
	})
	store.Store(ctx, &Event{
		ID:        "2",
		Type:      EventTypeRoleRevoked,
		Timestamp: time.Now(),
		Target:    &Target{ID: "user-2", Type: "user"},
	})

	// Query by target ID
	query := &Query{
		TargetIDs: []string{"user-1"},
	}

	result, err := store.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(result.Events) != 1 {
		t.Errorf("Got %d events, want 1", len(result.Events))
	}
}

func TestMemoryStore_Query_ResultFilter(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()

	store.Store(ctx, &Event{ID: "1", Type: EventTypeLogin, Result: ResultSuccess, Timestamp: time.Now()})
	store.Store(ctx, &Event{ID: "2", Type: EventTypeLogin, Result: ResultFailure, Timestamp: time.Now()})
	store.Store(ctx, &Event{ID: "3", Type: EventTypeLogin, Result: ResultSuccess, Timestamp: time.Now()})

	// Query failures only
	query := &Query{
		Results: []Result{ResultFailure},
	}

	result, err := store.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(result.Events) != 1 {
		t.Errorf("Got %d events, want 1", len(result.Events))
	}
}

func TestMemoryStore_Query_SourceIPFilter(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()

	store.Store(ctx, &Event{
		ID:        "1",
		Type:      EventTypeLogin,
		Timestamp: time.Now(),
		Source:    &Source{IP: "192.168.1.1"},
	})
	store.Store(ctx, &Event{
		ID:        "2",
		Type:      EventTypeLogin,
		Timestamp: time.Now(),
		Source:    &Source{IP: "10.0.0.1"},
	})

	query := &Query{
		SourceIPs: []string{"192.168.1.1"},
	}

	result, err := store.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(result.Events) != 1 {
		t.Errorf("Got %d events, want 1", len(result.Events))
	}
}

func TestMemoryStore_Query_TimeRange(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()

	now := time.Now()
	store.Store(ctx, &Event{ID: "1", Type: EventTypeLogin, Timestamp: now.Add(-time.Hour * 48)})
	store.Store(ctx, &Event{ID: "2", Type: EventTypeLogin, Timestamp: now.Add(-time.Hour * 12)})
	store.Store(ctx, &Event{ID: "3", Type: EventTypeLogin, Timestamp: now})

	// Query last 24 hours
	query := &Query{
		StartTime: now.Add(-time.Hour * 24),
	}

	result, err := store.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(result.Events) != 2 {
		t.Errorf("Got %d events, want 2", len(result.Events))
	}
}

func TestMemoryStore_Query_TextSearch(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()

	store.Store(ctx, &Event{
		ID:        "1",
		Type:      EventTypeLogin,
		Timestamp: time.Now(),
		Actor:     &Actor{ID: "user-1", Name: "John Doe", Email: "john@example.com"},
	})
	store.Store(ctx, &Event{
		ID:        "2",
		Type:      EventTypeLogin,
		Timestamp: time.Now(),
		Actor:     &Actor{ID: "user-2", Name: "Jane Smith", Email: "jane@example.com"},
	})

	// Search by name
	query := &Query{
		TextSearch: "john",
	}

	result, err := store.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(result.Events) != 1 {
		t.Errorf("Got %d events, want 1", len(result.Events))
	}
}

func TestMemoryStore_Query_Pagination(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()

	// Store 10 events
	for i := 0; i < 10; i++ {
		store.Store(ctx, &Event{
			ID:        string(rune('a' + i)),
			Type:      EventTypeLogin,
			Timestamp: time.Now().Add(time.Duration(i) * time.Minute),
		})
	}

	// Query with limit
	query := &Query{
		Limit:   3,
		OrderBy: "timestamp",
	}

	result, err := store.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(result.Events) != 3 {
		t.Errorf("Got %d events, want 3", len(result.Events))
	}
	if result.Total != 10 {
		t.Errorf("Total = %d, want 10", result.Total)
	}
	if !result.HasMore {
		t.Error("HasMore should be true")
	}

	// Query with offset
	query = &Query{
		Limit:   3,
		Offset:  3,
		OrderBy: "timestamp",
	}

	result, err = store.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(result.Events) != 3 {
		t.Errorf("Got %d events with offset, want 3", len(result.Events))
	}
}

func TestMemoryStore_Query_Sorting(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()

	now := time.Now()
	store.Store(ctx, &Event{ID: "c", Type: EventTypeLogin, Timestamp: now.Add(time.Hour)})
	store.Store(ctx, &Event{ID: "a", Type: EventTypeLogin, Timestamp: now.Add(-time.Hour)})
	store.Store(ctx, &Event{ID: "b", Type: EventTypeLogin, Timestamp: now})

	// Sort ascending
	query := &Query{
		OrderBy:   "timestamp",
		OrderDesc: false,
	}

	result, err := store.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if result.Events[0].ID != "a" {
		t.Error("First event should be 'a' (oldest)")
	}

	// Sort descending
	query.OrderDesc = true

	result, err = store.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if result.Events[0].ID != "c" {
		t.Error("First event should be 'c' (newest)")
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()

	store.Store(ctx, &Event{ID: "1", Type: EventTypeLogin, Timestamp: time.Now()})
	store.Store(ctx, &Event{ID: "2", Type: EventTypeLogin, Timestamp: time.Now()})

	err := store.Delete(ctx, "1")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if store.Count() != 1 {
		t.Errorf("Count = %d, want 1", store.Count())
	}
}

func TestMemoryStore_DeleteBefore(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()

	now := time.Now()
	store.Store(ctx, &Event{ID: "1", Type: EventTypeLogin, Timestamp: now.Add(-time.Hour * 48)})
	store.Store(ctx, &Event{ID: "2", Type: EventTypeLogin, Timestamp: now.Add(-time.Hour * 12)})
	store.Store(ctx, &Event{ID: "3", Type: EventTypeLogin, Timestamp: now})

	count, err := store.DeleteBefore(ctx, now.Add(-time.Hour*24))
	if err != nil {
		t.Fatalf("DeleteBefore failed: %v", err)
	}

	if count != 1 {
		t.Errorf("Deleted %d events, want 1", count)
	}

	if store.Count() != 2 {
		t.Errorf("Count = %d, want 2", store.Count())
	}
}

func TestMemoryStore_Close(t *testing.T) {
	store := NewMemoryStore()

	err := store.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	ctx := context.Background()

	_, err = store.Get(ctx, "1")
	if err != ErrStoreClosed {
		t.Errorf("Get after close = %v, want ErrStoreClosed", err)
	}

	err = store.Store(ctx, &Event{ID: "1"})
	if err != ErrStoreClosed {
		t.Errorf("Store after close = %v, want ErrStoreClosed", err)
	}
}

func TestAnalyzer_LoginActivity(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()
	now := time.Now()

	// Store login activity
	store.Store(ctx, &Event{
		ID:        "1",
		Type:      EventTypeLogin,
		Timestamp: now.Add(-time.Hour),
		Actor:     &Actor{ID: "user-1"},
		Source:    &Source{IP: "192.168.1.1"},
		Result:    ResultSuccess,
	})
	store.Store(ctx, &Event{
		ID:        "2",
		Type:      EventTypeLoginFailed,
		Timestamp: now.Add(-30 * time.Minute),
		Actor:     &Actor{ID: "user-1"},
		Source:    &Source{IP: "10.0.0.1"},
		Result:    ResultFailure,
	})
	store.Store(ctx, &Event{
		ID:        "3",
		Type:      EventTypeLogin,
		Timestamp: now,
		Actor:     &Actor{ID: "user-1"},
		Source:    &Source{IP: "192.168.1.1"},
		Result:    ResultSuccess,
	})
	store.Store(ctx, &Event{
		ID:        "4",
		Type:      EventTypeLogout,
		Timestamp: now,
		Actor:     &Actor{ID: "user-1"},
		Result:    ResultSuccess,
	})

	analyzer := NewAnalyzer(store)

	report, err := analyzer.LoginActivity(ctx, "user-1", now.Add(-2*time.Hour))
	if err != nil {
		t.Fatalf("LoginActivity failed: %v", err)
	}

	if report.SuccessfulLogins != 2 {
		t.Errorf("SuccessfulLogins = %d, want 2", report.SuccessfulLogins)
	}
	if report.FailedLogins != 1 {
		t.Errorf("FailedLogins = %d, want 1", report.FailedLogins)
	}
	if report.Logouts != 1 {
		t.Errorf("Logouts = %d, want 1", report.Logouts)
	}
	if len(report.UniqueIPs) != 2 {
		t.Errorf("UniqueIPs = %d, want 2", len(report.UniqueIPs))
	}
}

func TestAnalyzer_PermissionChanges(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()
	now := time.Now()

	store.Store(ctx, &Event{
		ID:        "1",
		Type:      EventTypeRoleAssigned,
		Timestamp: now.Add(-time.Hour),
		Actor:     &Actor{ID: "admin-1"},
		Target:    &Target{ID: "user-1", Type: "user"},
	})
	store.Store(ctx, &Event{
		ID:        "2",
		Type:      EventTypePermGranted,
		Timestamp: now,
		Actor:     &Actor{ID: "admin-1"},
		Target:    &Target{ID: "user-1", Type: "user"},
	})

	analyzer := NewAnalyzer(store)

	events, err := analyzer.PermissionChanges(ctx, "user-1", now.Add(-2*time.Hour))
	if err != nil {
		t.Fatalf("PermissionChanges failed: %v", err)
	}

	if len(events) != 2 {
		t.Errorf("Got %d events, want 2", len(events))
	}
}

func TestAnalyzer_SuspiciousActivity(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()
	now := time.Now()

	// Create brute force pattern (5+ failures)
	for i := 0; i < 6; i++ {
		store.Store(ctx, &Event{
			ID:        "brute-" + string(rune('a'+i)),
			Type:      EventTypeLoginFailed,
			Timestamp: now.Add(-time.Duration(i) * time.Minute),
			Actor:     &Actor{ID: "user-1"},
			Source:    &Source{IP: "192.168.1.1"},
			Result:    ResultFailure,
		})
	}

	// Create distributed attack pattern (10+ from same IP)
	for i := 0; i < 11; i++ {
		store.Store(ctx, &Event{
			ID:        "dist-" + string(rune('a'+i)),
			Type:      EventTypeLoginFailed,
			Timestamp: now.Add(-time.Duration(i) * time.Minute),
			Actor:     &Actor{ID: "user-" + string(rune('a'+i))},
			Source:    &Source{IP: "10.0.0.1"},
			Result:    ResultFailure,
		})
	}

	analyzer := NewAnalyzer(store)

	suspicious, err := analyzer.SuspiciousActivity(ctx, now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("SuspiciousActivity failed: %v", err)
	}

	hasBruteForce := false
	hasDistributed := false
	for _, s := range suspicious {
		if s.Type == "brute_force_attempt" {
			hasBruteForce = true
		}
		if s.Type == "distributed_attack" {
			hasDistributed = true
		}
	}

	if !hasBruteForce {
		t.Error("Expected brute_force_attempt detection")
	}
	if !hasDistributed {
		t.Error("Expected distributed_attack detection")
	}
}

func TestAnalyzer_SessionReport(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()
	now := time.Now()

	store.Store(ctx, &Event{
		ID:        "1",
		Type:      EventTypeSessionCreated,
		Timestamp: now.Add(-time.Hour),
		Source:    &Source{SessionID: "session-123"},
	})
	store.Store(ctx, &Event{
		ID:        "2",
		Type:      EventTypeLogin,
		Timestamp: now.Add(-50 * time.Minute),
		Source:    &Source{SessionID: "session-123"},
	})
	store.Store(ctx, &Event{
		ID:        "3",
		Type:      EventTypeSessionExpired,
		Timestamp: now,
		Source:    &Source{SessionID: "session-123"},
	})

	analyzer := NewAnalyzer(store)

	report, err := analyzer.SessionReport(ctx, "session-123")
	if err != nil {
		t.Fatalf("SessionReport failed: %v", err)
	}

	if report.TotalEvents != 3 {
		t.Errorf("TotalEvents = %d, want 3", report.TotalEvents)
	}
	if !report.SessionCreated {
		t.Error("SessionCreated should be true")
	}
	if !report.SessionExpired {
		t.Error("SessionExpired should be true")
	}
}

func TestAnalyzer_Summary(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()
	now := time.Now()

	store.Store(ctx, &Event{
		ID:        "1",
		Type:      EventTypeLogin,
		Timestamp: now,
		Actor:     &Actor{ID: "user-1", Type: "user"},
		Result:    ResultSuccess,
	})
	store.Store(ctx, &Event{
		ID:        "2",
		Type:      EventTypeLogin,
		Timestamp: now,
		Actor:     &Actor{ID: "user-2", Type: "user"},
		Result:    ResultSuccess,
	})
	store.Store(ctx, &Event{
		ID:        "3",
		Type:      EventTypeLoginFailed,
		Timestamp: now,
		Actor:     &Actor{ID: "user-3", Type: "user"},
		Result:    ResultFailure,
	})
	store.Store(ctx, &Event{
		ID:        "4",
		Type:      EventTypeTokenIssued,
		Timestamp: now,
		Actor:     &Actor{ID: "service-1", Type: "service"},
		Result:    ResultSuccess,
	})

	analyzer := NewAnalyzer(store)

	summary, err := analyzer.Summary(ctx, now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("Summary failed: %v", err)
	}

	if summary.TotalEvents != 4 {
		t.Errorf("TotalEvents = %d, want 4", summary.TotalEvents)
	}
	if summary.ByType[EventTypeLogin] != 2 {
		t.Errorf("Login count = %d, want 2", summary.ByType[EventTypeLogin])
	}
	if summary.ByResult[ResultSuccess] != 3 {
		t.Errorf("Success count = %d, want 3", summary.ByResult[ResultSuccess])
	}
	if summary.ByActorType["user"] != 3 {
		t.Errorf("User actor count = %d, want 3", summary.ByActorType["user"])
	}
	if len(summary.UniqueActors) != 4 {
		t.Errorf("UniqueActors = %d, want 4", len(summary.UniqueActors))
	}
}
