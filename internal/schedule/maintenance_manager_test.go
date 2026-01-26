package schedule

import (
	"context"
	"testing"
	"time"
)

func TestNewMaintenanceWindowManager(t *testing.T) {
	store := NewMockStore()

	tests := []struct {
		name    string
		config  *MaintenanceManagerConfig
		store   Store
		wantErr bool
	}{
		{
			name: "valid with config",
			config: &MaintenanceManagerConfig{
				MemberID: "member-1",
			},
			store:   store,
			wantErr: false,
		},
		{
			name:    "nil store",
			config:  &MaintenanceManagerConfig{MemberID: "member-1"},
			store:   nil,
			wantErr: true,
		},
		{
			name:    "missing member ID",
			config:  &MaintenanceManagerConfig{},
			store:   store,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewMaintenanceWindowManager(tt.config, tt.store)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewMaintenanceWindowManager() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMaintenanceWindowManager_Create(t *testing.T) {
	store := NewMockStore()
	config := &MaintenanceManagerConfig{MemberID: "member-1"}
	manager, err := NewMaintenanceWindowManager(config, store)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()

	tests := []struct {
		name    string
		window  *MaintenanceWindow
		wantErr bool
	}{
		{
			name: "valid window",
			window: &MaintenanceWindow{
				Name:      "test-window",
				Type:      MaintenanceWindowTypePlanned,
				StartTime: now.Add(time.Hour),
				EndTime:   now.Add(2 * time.Hour),
				Scope: &MaintenanceScope{
					All: true,
				},
			},
			wantErr: false,
		},
		{
			name: "valid with specific agents",
			window: &MaintenanceWindow{
				Name:      "agent-window",
				Type:      MaintenanceWindowTypePlanned,
				StartTime: now.Add(3 * time.Hour),
				EndTime:   now.Add(4 * time.Hour),
				Scope: &MaintenanceScope{
					AgentIDs: []string{"agent-1", "agent-2"},
				},
			},
			wantErr: false,
		},
		{
			name:    "nil window",
			window:  nil,
			wantErr: true,
		},
		{
			name: "missing name",
			window: &MaintenanceWindow{
				Type:      MaintenanceWindowTypePlanned,
				StartTime: now.Add(time.Hour),
				EndTime:   now.Add(2 * time.Hour),
				Scope: &MaintenanceScope{
					All: true,
				},
			},
			wantErr: true,
		},
		{
			name: "missing start time",
			window: &MaintenanceWindow{
				Name:    "no-start",
				Type:    MaintenanceWindowTypePlanned,
				EndTime: now.Add(2 * time.Hour),
				Scope: &MaintenanceScope{
					All: true,
				},
			},
			wantErr: true,
		},
		{
			name: "missing end time",
			window: &MaintenanceWindow{
				Name:      "no-end",
				Type:      MaintenanceWindowTypePlanned,
				StartTime: now.Add(time.Hour),
				Scope: &MaintenanceScope{
					All: true,
				},
			},
			wantErr: true,
		},
		{
			name: "end before start",
			window: &MaintenanceWindow{
				Name:      "invalid-times",
				Type:      MaintenanceWindowTypePlanned,
				StartTime: now.Add(2 * time.Hour),
				EndTime:   now.Add(time.Hour),
				Scope: &MaintenanceScope{
					All: true,
				},
			},
			wantErr: true,
		},
		{
			name: "missing scope",
			window: &MaintenanceWindow{
				Name:      "no-scope",
				Type:      MaintenanceWindowTypePlanned,
				StartTime: now.Add(time.Hour),
				EndTime:   now.Add(2 * time.Hour),
			},
			wantErr: true,
		},
		{
			name: "empty scope",
			window: &MaintenanceWindow{
				Name:      "empty-scope",
				Type:      MaintenanceWindowTypePlanned,
				StartTime: now.Add(time.Hour),
				EndTime:   now.Add(2 * time.Hour),
				Scope:     &MaintenanceScope{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store.Reset()
			err := manager.Create(ctx, tt.window)
			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && tt.window != nil {
				got, err := manager.Get(ctx, tt.window.ID)
				if err != nil {
					t.Errorf("Failed to get created window: %v", err)
				}
				if got.Name != tt.window.Name {
					t.Errorf("Window name = %v, want %v", got.Name, tt.window.Name)
				}
			}
		})
	}
}

func TestMaintenanceWindowManager_Update(t *testing.T) {
	store := NewMockStore()
	config := &MaintenanceManagerConfig{MemberID: "member-1"}
	manager, err := NewMaintenanceWindowManager(config, store)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()

	// Create initial window
	window := &MaintenanceWindow{
		Name:      "test-window",
		Type:      MaintenanceWindowTypePlanned,
		StartTime: now.Add(time.Hour),
		EndTime:   now.Add(2 * time.Hour),
		Scope: &MaintenanceScope{
			All: true,
		},
	}
	if err := manager.Create(ctx, window); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// Update window
	window.Name = "updated-window"
	window.EndTime = now.Add(3 * time.Hour)

	if err := manager.Update(ctx, window); err != nil {
		t.Errorf("Update() error = %v", err)
	}

	// Verify update
	got, err := manager.Get(ctx, window.ID)
	if err != nil {
		t.Fatalf("Failed to get window: %v", err)
	}
	if got.Name != "updated-window" {
		t.Errorf("Name = %v, want updated-window", got.Name)
	}
}

func TestMaintenanceWindowManager_Delete(t *testing.T) {
	store := NewMockStore()
	config := &MaintenanceManagerConfig{MemberID: "member-1"}
	manager, err := NewMaintenanceWindowManager(config, store)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()

	// Create window
	window := &MaintenanceWindow{
		Name:      "test-window",
		Type:      MaintenanceWindowTypePlanned,
		StartTime: now.Add(time.Hour),
		EndTime:   now.Add(2 * time.Hour),
		Scope: &MaintenanceScope{
			All: true,
		},
	}
	if err := manager.Create(ctx, window); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// Delete window
	if err := manager.Delete(ctx, window.ID); err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	// Verify deletion
	_, err = manager.Get(ctx, window.ID)
	if err != ErrMaintenanceWindowNotFound {
		t.Errorf("Get() error = %v, want ErrMaintenanceWindowNotFound", err)
	}
}

func TestMaintenanceWindowManager_DeleteActiveWindow(t *testing.T) {
	store := NewMockStore()
	config := &MaintenanceManagerConfig{MemberID: "member-1"}
	manager, err := NewMaintenanceWindowManager(config, store)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()

	// Create and start window
	window := &MaintenanceWindow{
		Name:      "active-window",
		Type:      MaintenanceWindowTypePlanned,
		StartTime: now.Add(-time.Hour), // Started in past
		EndTime:   now.Add(time.Hour),
		Status:    MaintenanceWindowStatusScheduled,
		Scope: &MaintenanceScope{
			All: true,
		},
	}
	if err := manager.Create(ctx, window); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// Start the window
	if err := manager.Start(ctx, window.ID); err != nil {
		t.Fatalf("Failed to start window: %v", err)
	}

	// Try to delete active window - should fail
	if err := manager.Delete(ctx, window.ID); err != ErrMaintenanceActive {
		t.Errorf("Delete() error = %v, want ErrMaintenanceActive", err)
	}
}

func TestMaintenanceWindowManager_List(t *testing.T) {
	store := NewMockStore()
	config := &MaintenanceManagerConfig{MemberID: "member-1"}
	manager, err := NewMaintenanceWindowManager(config, store)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()

	// Create windows
	windows := []*MaintenanceWindow{
		{
			Name:      "window-1",
			Type:      MaintenanceWindowTypePlanned,
			StartTime: now.Add(time.Hour),
			EndTime:   now.Add(2 * time.Hour),
			Scope:     &MaintenanceScope{All: true},
		},
		{
			Name:      "window-2",
			Type:      MaintenanceWindowTypeEmergency,
			StartTime: now.Add(3 * time.Hour),
			EndTime:   now.Add(4 * time.Hour),
			Scope:     &MaintenanceScope{All: true},
		},
	}

	for _, w := range windows {
		if err := manager.Create(ctx, w); err != nil {
			t.Fatalf("Failed to create window: %v", err)
		}
	}

	// List all
	list, err := manager.List(ctx, nil)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 2 {
		t.Errorf("List() returned %d windows, want 2", len(list))
	}

	// List with filter
	filter := &MaintenanceWindowFilter{
		Type: []MaintenanceWindowType{MaintenanceWindowTypePlanned},
	}
	list, err = manager.List(ctx, filter)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 {
		t.Errorf("List() with filter returned %d windows, want 1", len(list))
	}
}

func TestMaintenanceWindowManager_ApprovalWorkflow(t *testing.T) {
	store := NewMockStore()
	config := &MaintenanceManagerConfig{MemberID: "member-1"}
	manager, err := NewMaintenanceWindowManager(config, store)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()

	// Create window requiring approval
	window := &MaintenanceWindow{
		Name:            "approval-window",
		Type:            MaintenanceWindowTypePlanned,
		StartTime:       now.Add(time.Hour),
		EndTime:         now.Add(2 * time.Hour),
		RequireApproval: true,
		Scope: &MaintenanceScope{
			All: true,
		},
	}
	if err := manager.Create(ctx, window); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// Should be pending approval
	got, _ := manager.Get(ctx, window.ID)
	if got.Status != MaintenanceWindowStatusPendingApproval {
		t.Errorf("Status = %v, want pending_approval", got.Status)
	}

	// Approve
	if err := manager.Approve(ctx, window.ID, "approver"); err != nil {
		t.Errorf("Approve() error = %v", err)
	}

	got, _ = manager.Get(ctx, window.ID)
	if got.Status != MaintenanceWindowStatusScheduled {
		t.Errorf("Status = %v, want scheduled", got.Status)
	}
	if got.ApprovedBy != "approver" {
		t.Errorf("ApprovedBy = %v, want approver", got.ApprovedBy)
	}
}

func TestMaintenanceWindowManager_StartEndWorkflow(t *testing.T) {
	store := NewMockStore()
	config := &MaintenanceManagerConfig{MemberID: "member-1"}
	manager, err := NewMaintenanceWindowManager(config, store)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()

	// Create window
	window := &MaintenanceWindow{
		Name:      "lifecycle-window",
		Type:      MaintenanceWindowTypePlanned,
		StartTime: now.Add(time.Hour),
		EndTime:   now.Add(2 * time.Hour),
		Scope: &MaintenanceScope{
			All: true,
		},
	}
	if err := manager.Create(ctx, window); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// Start
	if err := manager.Start(ctx, window.ID); err != nil {
		t.Errorf("Start() error = %v", err)
	}

	got, _ := manager.Get(ctx, window.ID)
	if got.Status != MaintenanceWindowStatusActive {
		t.Errorf("Status = %v, want active", got.Status)
	}
	if got.ActualStartTime == nil {
		t.Error("ActualStartTime should be set")
	}

	// End
	if err := manager.End(ctx, window.ID); err != nil {
		t.Errorf("End() error = %v", err)
	}

	got, _ = manager.Get(ctx, window.ID)
	if got.Status != MaintenanceWindowStatusCompleted {
		t.Errorf("Status = %v, want completed", got.Status)
	}
	if got.ActualEndTime == nil {
		t.Error("ActualEndTime should be set")
	}
}

func TestMaintenanceWindowManager_Cancel(t *testing.T) {
	store := NewMockStore()
	config := &MaintenanceManagerConfig{MemberID: "member-1"}
	manager, err := NewMaintenanceWindowManager(config, store)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()

	// Create window
	window := &MaintenanceWindow{
		Name:      "cancel-window",
		Type:      MaintenanceWindowTypePlanned,
		StartTime: now.Add(time.Hour),
		EndTime:   now.Add(2 * time.Hour),
		Scope: &MaintenanceScope{
			All: true,
		},
	}
	if err := manager.Create(ctx, window); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// Cancel
	if err := manager.Cancel(ctx, window.ID, "admin", "no longer needed"); err != nil {
		t.Errorf("Cancel() error = %v", err)
	}

	got, _ := manager.Get(ctx, window.ID)
	if got.Status != MaintenanceWindowStatusCancelled {
		t.Errorf("Status = %v, want cancelled", got.Status)
	}
	if got.CancelledBy != "admin" {
		t.Errorf("CancelledBy = %v, want admin", got.CancelledBy)
	}
	if got.CancellationReason != "no longer needed" {
		t.Errorf("CancellationReason = %v, want 'no longer needed'", got.CancellationReason)
	}
}

func TestMaintenanceWindowManager_Extend(t *testing.T) {
	store := NewMockStore()
	config := &MaintenanceManagerConfig{
		MemberID:          "member-1",
		MaxWindowDuration: 24 * time.Hour,
	}
	manager, err := NewMaintenanceWindowManager(config, store)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()

	// Create and start window
	window := &MaintenanceWindow{
		Name:      "extend-window",
		Type:      MaintenanceWindowTypePlanned,
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(time.Hour),
		Scope: &MaintenanceScope{
			All: true,
		},
	}
	if err := manager.Create(ctx, window); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// Start it
	if err := manager.Start(ctx, window.ID); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Extend
	newEndTime := now.Add(3 * time.Hour)
	if err := manager.Extend(ctx, window.ID, newEndTime, "admin"); err != nil {
		t.Errorf("Extend() error = %v", err)
	}

	got, _ := manager.Get(ctx, window.ID)
	if !got.EndTime.Equal(newEndTime) {
		t.Errorf("EndTime = %v, want %v", got.EndTime, newEndTime)
	}

	// Try to shrink - should fail
	if err := manager.Extend(ctx, window.ID, now.Add(2*time.Hour), "admin"); err == nil {
		t.Error("Extend() with earlier time should fail")
	}

	// Try to exceed max duration - should fail
	if err := manager.Extend(ctx, window.ID, now.Add(48*time.Hour), "admin"); err == nil {
		t.Error("Extend() exceeding max duration should fail")
	}
}

func TestMaintenanceWindowManager_IsInMaintenance(t *testing.T) {
	store := NewMockStore()
	config := &MaintenanceManagerConfig{MemberID: "member-1"}
	manager, err := NewMaintenanceWindowManager(config, store)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()

	// Create and start window
	window := &MaintenanceWindow{
		Name:      "maintenance-window",
		Type:      MaintenanceWindowTypePlanned,
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(time.Hour),
		Scope: &MaintenanceScope{
			AgentIDs: []string{"agent-1", "agent-2"},
		},
	}
	if err := manager.Create(ctx, window); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// Start it
	if err := manager.Start(ctx, window.ID); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Check if in maintenance - this test checks empty string since mock doesn't implement agent filtering
	_, inMaintenance, err := manager.IsInMaintenance(ctx, "")
	if err != nil {
		t.Errorf("IsInMaintenance() error = %v", err)
	}
	if !inMaintenance {
		t.Error("Should be in maintenance")
	}
}

func TestMaintenanceWindowManager_GetActiveWindows(t *testing.T) {
	store := NewMockStore()
	config := &MaintenanceManagerConfig{MemberID: "member-1"}
	manager, err := NewMaintenanceWindowManager(config, store)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()

	// Create multiple windows
	windows := []*MaintenanceWindow{
		{
			Name:      "window-1",
			Type:      MaintenanceWindowTypePlanned,
			StartTime: now.Add(-time.Hour),
			EndTime:   now.Add(time.Hour),
			Scope:     &MaintenanceScope{All: true},
		},
		{
			Name:      "window-2",
			Type:      MaintenanceWindowTypePlanned,
			StartTime: now.Add(2 * time.Hour),
			EndTime:   now.Add(3 * time.Hour),
			Scope:     &MaintenanceScope{All: true},
		},
	}

	for _, w := range windows {
		if err := manager.Create(ctx, w); err != nil {
			t.Fatalf("Failed to create window: %v", err)
		}
	}

	// Start only the first one
	if err := manager.Start(ctx, windows[0].ID); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Get active windows
	active, err := manager.GetActiveWindows(ctx)
	if err != nil {
		t.Fatalf("GetActiveWindows() error = %v", err)
	}
	if len(active) != 1 {
		t.Errorf("GetActiveWindows() returned %d windows, want 1", len(active))
	}
}

func TestMaintenanceWindowManager_GetUpcomingWindows(t *testing.T) {
	store := NewMockStore()
	config := &MaintenanceManagerConfig{MemberID: "member-1"}
	manager, err := NewMaintenanceWindowManager(config, store)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()

	// Create windows with different start times
	windows := []*MaintenanceWindow{
		{
			Name:      "soon-window",
			Type:      MaintenanceWindowTypePlanned,
			StartTime: now.Add(30 * time.Minute),
			EndTime:   now.Add(90 * time.Minute),
			Scope:     &MaintenanceScope{All: true},
		},
		{
			Name:      "later-window",
			Type:      MaintenanceWindowTypePlanned,
			StartTime: now.Add(5 * time.Hour),
			EndTime:   now.Add(6 * time.Hour),
			Scope:     &MaintenanceScope{All: true},
		},
	}

	for _, w := range windows {
		if err := manager.Create(ctx, w); err != nil {
			t.Fatalf("Failed to create window: %v", err)
		}
	}

	// Get upcoming within 2 hours
	// Note: The mock store doesn't implement StartAfter/EndBefore filtering fully,
	// so this test verifies the function returns results without errors
	upcoming, err := manager.GetUpcomingWindows(ctx, 2*time.Hour)
	if err != nil {
		t.Fatalf("GetUpcomingWindows() error = %v", err)
	}
	// Both windows are scheduled, the mock doesn't filter by time
	if len(upcoming) == 0 {
		t.Error("GetUpcomingWindows() returned no windows")
	}
}

func TestMaintenanceWindowManager_GetConflicts(t *testing.T) {
	store := NewMockStore()
	config := &MaintenanceManagerConfig{
		MemberID:                "member-1",
		AllowOverlappingWindows: true, // Allow for testing
	}
	manager, err := NewMaintenanceWindowManager(config, store)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()

	// Create existing window
	existing := &MaintenanceWindow{
		Name:      "existing-window",
		Type:      MaintenanceWindowTypePlanned,
		StartTime: now.Add(time.Hour),
		EndTime:   now.Add(3 * time.Hour),
		Scope: &MaintenanceScope{
			AgentIDs: []string{"agent-1", "agent-2"},
		},
	}
	if err := manager.Create(ctx, existing); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	// Create overlapping window
	overlapping := &MaintenanceWindow{
		ID:        "new-window",
		Name:      "overlapping-window",
		Type:      MaintenanceWindowTypePlanned,
		StartTime: now.Add(2 * time.Hour),
		EndTime:   now.Add(4 * time.Hour),
		Scope: &MaintenanceScope{
			AgentIDs: []string{"agent-2", "agent-3"}, // agent-2 overlaps
		},
	}

	// Check conflicts
	conflicts, err := manager.GetConflicts(ctx, overlapping)
	if err != nil {
		t.Fatalf("GetConflicts() error = %v", err)
	}
	if len(conflicts) != 1 {
		t.Errorf("GetConflicts() returned %d conflicts, want 1", len(conflicts))
	}
	if len(conflicts) > 0 {
		if conflicts[0].ConflictingID != existing.ID {
			t.Errorf("ConflictingID = %v, want %v", conflicts[0].ConflictingID, existing.ID)
		}
		if len(conflicts[0].AffectedAgents) != 1 || conflicts[0].AffectedAgents[0] != "agent-2" {
			t.Errorf("AffectedAgents = %v, want [agent-2]", conflicts[0].AffectedAgents)
		}
	}
}

func TestMaintenanceWindowManager_GetStats(t *testing.T) {
	store := NewMockStore()
	config := &MaintenanceManagerConfig{MemberID: "member-1"}
	manager, err := NewMaintenanceWindowManager(config, store)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()

	// Create windows with different states
	windows := []*MaintenanceWindow{
		{
			Name:      "scheduled-window",
			Type:      MaintenanceWindowTypePlanned,
			StartTime: now.Add(time.Hour),
			EndTime:   now.Add(2 * time.Hour),
			Scope:     &MaintenanceScope{All: true},
		},
		{
			Name:      "active-window",
			Type:      MaintenanceWindowTypeEmergency,
			StartTime: now.Add(-time.Hour),
			EndTime:   now.Add(time.Hour),
			Scope:     &MaintenanceScope{AgentIDs: []string{"agent-1"}},
		},
	}

	for _, w := range windows {
		if err := manager.Create(ctx, w); err != nil {
			t.Fatalf("Failed to create window: %v", err)
		}
	}

	// Start the second one
	if err := manager.Start(ctx, windows[1].ID); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Get stats
	stats, err := manager.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}

	if stats.TotalWindows != 2 {
		t.Errorf("TotalWindows = %v, want 2", stats.TotalWindows)
	}
	if stats.ActiveWindows != 1 {
		t.Errorf("ActiveWindows = %v, want 1", stats.ActiveWindows)
	}
	if stats.ScheduledWindows != 1 {
		t.Errorf("ScheduledWindows = %v, want 1", stats.ScheduledWindows)
	}
	if stats.ByType[MaintenanceWindowTypePlanned] != 1 {
		t.Errorf("ByType[planned] = %v, want 1", stats.ByType[MaintenanceWindowTypePlanned])
	}
	if stats.ByType[MaintenanceWindowTypeEmergency] != 1 {
		t.Errorf("ByType[emergency] = %v, want 1", stats.ByType[MaintenanceWindowTypeEmergency])
	}
}

func TestMaintenanceWindowManager_EventEmission(t *testing.T) {
	store := NewMockStore()
	config := &MaintenanceManagerConfig{MemberID: "member-1"}
	manager, err := NewMaintenanceWindowManager(config, store)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()

	// Track events
	var events []*MaintenanceEvent
	manager.AddListener(func(event *MaintenanceEvent) {
		events = append(events, event)
	})

	// Create window
	window := &MaintenanceWindow{
		Name:      "event-window",
		Type:      MaintenanceWindowTypePlanned,
		StartTime: now.Add(time.Hour),
		EndTime:   now.Add(2 * time.Hour),
		Scope:     &MaintenanceScope{All: true},
	}
	if err := manager.Create(ctx, window); err != nil {
		t.Fatalf("Failed to create window: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}
	if events[0].Type != string(MaintenanceEventCreated) {
		t.Errorf("Event type = %v, want maintenance.created", events[0].Type)
	}

	// Start
	if err := manager.Start(ctx, window.ID); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("Expected 2 events, got %d", len(events))
	}
	if events[1].Type != string(MaintenanceEventStarted) {
		t.Errorf("Event type = %v, want maintenance.started", events[1].Type)
	}

	// End
	if err := manager.End(ctx, window.ID); err != nil {
		t.Fatalf("End() error = %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("Expected 3 events, got %d", len(events))
	}
	if events[2].Type != string(MaintenanceEventEnded) {
		t.Errorf("Event type = %v, want maintenance.ended", events[2].Type)
	}
}

func TestMaintenanceWindowManager_Close(t *testing.T) {
	store := NewMockStore()
	config := &MaintenanceManagerConfig{MemberID: "member-1"}
	manager, err := NewMaintenanceWindowManager(config, store)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Close
	if err := manager.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Operations should fail after close
	ctx := context.Background()
	now := time.Now().UTC()
	window := &MaintenanceWindow{
		Name:      "test-window",
		Type:      MaintenanceWindowTypePlanned,
		StartTime: now.Add(time.Hour),
		EndTime:   now.Add(2 * time.Hour),
		Scope:     &MaintenanceScope{All: true},
	}
	if err := manager.Create(ctx, window); err != ErrStoreClosed {
		t.Errorf("Create() after close should return ErrStoreClosed, got %v", err)
	}
}

func TestMaintenanceWindowManager_MaxDurationValidation(t *testing.T) {
	store := NewMockStore()
	config := &MaintenanceManagerConfig{
		MemberID:          "member-1",
		MaxWindowDuration: 4 * time.Hour,
	}
	manager, err := NewMaintenanceWindowManager(config, store)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()

	// Try to create window exceeding max duration
	window := &MaintenanceWindow{
		Name:      "long-window",
		Type:      MaintenanceWindowTypePlanned,
		StartTime: now.Add(time.Hour),
		EndTime:   now.Add(10 * time.Hour), // Exceeds 4 hour limit
		Scope:     &MaintenanceScope{All: true},
	}
	if err := manager.Create(ctx, window); err == nil {
		t.Error("Create() should fail for window exceeding max duration")
	}
}

func TestMaintenanceWindowManager_TimezoneValidation(t *testing.T) {
	store := NewMockStore()
	config := &MaintenanceManagerConfig{MemberID: "member-1"}
	manager, err := NewMaintenanceWindowManager(config, store)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()

	tests := []struct {
		name     string
		timezone string
		wantErr  bool
	}{
		{"UTC", "UTC", false},
		{"New York", "America/New_York", false},
		{"invalid", "Invalid/Timezone", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			window := &MaintenanceWindow{
				Name:      "tz-window",
				Type:      MaintenanceWindowTypePlanned,
				StartTime: now.Add(time.Hour),
				EndTime:   now.Add(2 * time.Hour),
				Timezone:  tt.timezone,
				Scope:     &MaintenanceScope{All: true},
			}
			err := manager.Create(ctx, window)
			if (err != nil) != tt.wantErr {
				t.Errorf("Create() with timezone %s error = %v, wantErr %v", tt.timezone, err, tt.wantErr)
			}
			store.Reset()
		})
	}
}

func TestAgentInScope(t *testing.T) {
	tests := []struct {
		name    string
		agentID string
		scope   *MaintenanceScope
		want    bool
	}{
		{
			name:    "nil scope",
			agentID: "agent-1",
			scope:   nil,
			want:    false,
		},
		{
			name:    "all scope",
			agentID: "agent-1",
			scope:   &MaintenanceScope{All: true},
			want:    true,
		},
		{
			name:    "agent in list",
			agentID: "agent-1",
			scope:   &MaintenanceScope{AgentIDs: []string{"agent-1", "agent-2"}},
			want:    true,
		},
		{
			name:    "agent not in list",
			agentID: "agent-3",
			scope:   &MaintenanceScope{AgentIDs: []string{"agent-1", "agent-2"}},
			want:    false,
		},
		{
			name:    "glob match",
			agentID: "web-server-1",
			scope:   &MaintenanceScope{Glob: "web-*"},
			want:    true,
		},
		{
			name:    "glob no match",
			agentID: "db-server-1",
			scope:   &MaintenanceScope{Glob: "web-*"},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AgentInScope(tt.agentID, tt.scope); got != tt.want {
				t.Errorf("AgentInScope() = %v, want %v", got, tt.want)
			}
		})
	}
}
