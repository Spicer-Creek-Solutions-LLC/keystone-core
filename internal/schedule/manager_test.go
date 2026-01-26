package schedule

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestNewScheduleManager(t *testing.T) {
	store := NewMockStore()

	tests := []struct {
		name    string
		config  *ManagerConfig
		store   Store
		wantErr bool
	}{
		{
			name: "valid with config",
			config: &ManagerConfig{
				MemberID: "member-1",
			},
			store:   store,
			wantErr: false,
		},
		{
			name:    "nil store",
			config:  &ManagerConfig{MemberID: "member-1"},
			store:   nil,
			wantErr: true,
		},
		{
			name:    "missing member ID",
			config:  &ManagerConfig{},
			store:   store,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewScheduleManager(tt.config, tt.store)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewScheduleManager() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestScheduleManager_Create(t *testing.T) {
	store := NewMockStore()
	config := &ManagerConfig{MemberID: "member-1"}
	manager, err := NewScheduleManager(config, store)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	ctx := context.Background()

	tests := []struct {
		name     string
		schedule *Schedule
		wantErr  bool
	}{
		{
			name: "valid cron schedule",
			schedule: &Schedule{
				Name: "test-schedule",
				Type: ScheduleTypeCommand,
				Cron: "0 2 * * *",
				Target: &ScheduleTarget{
					All: true,
				},
				Payload: json.RawMessage(`{"command": "echo hello"}`),
			},
			wantErr: false,
		},
		{
			name: "valid interval schedule",
			schedule: &Schedule{
				Name:     "interval-schedule",
				Type:     ScheduleTypeState,
				Interval: time.Hour,
				Target: &ScheduleTarget{
					AgentIDs: []string{"agent-1"},
				},
				Payload: json.RawMessage(`{"state_path": "/path/to/state"}`),
			},
			wantErr: false,
		},
		{
			name:     "nil schedule",
			schedule: nil,
			wantErr:  true,
		},
		{
			name: "missing name",
			schedule: &Schedule{
				Type: ScheduleTypeCommand,
				Cron: "0 2 * * *",
				Target: &ScheduleTarget{
					All: true,
				},
			},
			wantErr: true,
		},
		{
			name: "missing type",
			schedule: &Schedule{
				Name: "no-type",
				Cron: "0 2 * * *",
				Target: &ScheduleTarget{
					All: true,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid type",
			schedule: &Schedule{
				Name: "invalid-type",
				Type: "invalid",
				Cron: "0 2 * * *",
				Target: &ScheduleTarget{
					All: true,
				},
			},
			wantErr: true,
		},
		{
			name: "missing cron and interval",
			schedule: &Schedule{
				Name: "no-schedule",
				Type: ScheduleTypeCommand,
				Target: &ScheduleTarget{
					All: true,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid cron",
			schedule: &Schedule{
				Name: "invalid-cron",
				Type: ScheduleTypeCommand,
				Cron: "invalid",
				Target: &ScheduleTarget{
					All: true,
				},
			},
			wantErr: true,
		},
		{
			name: "missing target",
			schedule: &Schedule{
				Name: "no-target",
				Type: ScheduleTypeCommand,
				Cron: "0 2 * * *",
			},
			wantErr: true,
		},
		{
			name: "empty target",
			schedule: &Schedule{
				Name:   "empty-target",
				Type:   ScheduleTypeCommand,
				Cron:   "0 2 * * *",
				Target: &ScheduleTarget{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store.Reset() // Clear state between tests
			err := manager.Create(ctx, tt.schedule)
			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && tt.schedule != nil {
				// Verify schedule was stored
				got, err := manager.Get(ctx, tt.schedule.ID)
				if err != nil {
					t.Errorf("Failed to get created schedule: %v", err)
				}
				if got.Name != tt.schedule.Name {
					t.Errorf("Schedule name = %v, want %v", got.Name, tt.schedule.Name)
				}
				if got.NextRun == nil {
					t.Error("NextRun should be calculated")
				}
			}
		})
	}
}

func TestScheduleManager_Update(t *testing.T) {
	store := NewMockStore()
	config := &ManagerConfig{MemberID: "member-1"}
	manager, err := NewScheduleManager(config, store)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	ctx := context.Background()

	// Create initial schedule
	schedule := &Schedule{
		Name: "test-schedule",
		Type: ScheduleTypeCommand,
		Cron: "0 2 * * *",
		Target: &ScheduleTarget{
			All: true,
		},
	}
	if err := manager.Create(ctx, schedule); err != nil {
		t.Fatalf("Failed to create schedule: %v", err)
	}

	// Update schedule
	schedule.Name = "updated-schedule"
	schedule.Cron = "0 3 * * *"

	if err := manager.Update(ctx, schedule); err != nil {
		t.Errorf("Update() error = %v", err)
	}

	// Verify update
	got, err := manager.Get(ctx, schedule.ID)
	if err != nil {
		t.Fatalf("Failed to get schedule: %v", err)
	}
	if got.Name != "updated-schedule" {
		t.Errorf("Name = %v, want updated-schedule", got.Name)
	}
	if got.Cron != "0 3 * * *" {
		t.Errorf("Cron = %v, want 0 3 * * *", got.Cron)
	}
}

func TestScheduleManager_Delete(t *testing.T) {
	store := NewMockStore()
	config := &ManagerConfig{MemberID: "member-1"}
	manager, err := NewScheduleManager(config, store)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	ctx := context.Background()

	// Create schedule
	schedule := &Schedule{
		Name: "test-schedule",
		Type: ScheduleTypeCommand,
		Cron: "0 2 * * *",
		Target: &ScheduleTarget{
			All: true,
		},
	}
	if err := manager.Create(ctx, schedule); err != nil {
		t.Fatalf("Failed to create schedule: %v", err)
	}

	// Delete schedule
	if err := manager.Delete(ctx, schedule.ID); err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	// Verify deletion
	_, err = manager.Get(ctx, schedule.ID)
	if err != ErrScheduleNotFound {
		t.Errorf("Get() error = %v, want ErrScheduleNotFound", err)
	}
}

func TestScheduleManager_List(t *testing.T) {
	store := NewMockStore()
	config := &ManagerConfig{MemberID: "member-1"}
	manager, err := NewScheduleManager(config, store)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	ctx := context.Background()

	// Create schedules
	schedules := []*Schedule{
		{
			Name: "schedule-1",
			Type: ScheduleTypeCommand,
			Cron: "0 2 * * *",
			Target: &ScheduleTarget{
				All: true,
			},
		},
		{
			Name: "schedule-2",
			Type: ScheduleTypeState,
			Cron: "0 3 * * *",
			Target: &ScheduleTarget{
				All: true,
			},
		},
	}

	for _, s := range schedules {
		if err := manager.Create(ctx, s); err != nil {
			t.Fatalf("Failed to create schedule: %v", err)
		}
	}

	// List all
	list, err := manager.List(ctx, nil)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 2 {
		t.Errorf("List() returned %d schedules, want 2", len(list))
	}

	// List with filter
	filter := &ScheduleFilter{
		Type: []ScheduleType{ScheduleTypeCommand},
	}
	list, err = manager.List(ctx, filter)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 {
		t.Errorf("List() with filter returned %d schedules, want 1", len(list))
	}
}

func TestScheduleManager_PauseResume(t *testing.T) {
	store := NewMockStore()
	config := &ManagerConfig{MemberID: "member-1"}
	manager, err := NewScheduleManager(config, store)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	ctx := context.Background()

	// Create schedule
	schedule := &Schedule{
		Name: "test-schedule",
		Type: ScheduleTypeCommand,
		Cron: "0 2 * * *",
		Target: &ScheduleTarget{
			All: true,
		},
	}
	if err := manager.Create(ctx, schedule); err != nil {
		t.Fatalf("Failed to create schedule: %v", err)
	}

	// Pause
	if err := manager.Pause(ctx, schedule.ID, "admin"); err != nil {
		t.Errorf("Pause() error = %v", err)
	}

	got, _ := manager.Get(ctx, schedule.ID)
	if got.Status != ScheduleStatusPaused {
		t.Errorf("Status = %v, want paused", got.Status)
	}

	// Resume
	if err := manager.Resume(ctx, schedule.ID, "admin"); err != nil {
		t.Errorf("Resume() error = %v", err)
	}

	got, _ = manager.Get(ctx, schedule.ID)
	if got.Status != ScheduleStatusActive {
		t.Errorf("Status = %v, want active", got.Status)
	}
}

func TestScheduleManager_DisableEnable(t *testing.T) {
	store := NewMockStore()
	config := &ManagerConfig{MemberID: "member-1"}
	manager, err := NewScheduleManager(config, store)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	ctx := context.Background()

	// Create schedule
	schedule := &Schedule{
		Name: "test-schedule",
		Type: ScheduleTypeCommand,
		Cron: "0 2 * * *",
		Target: &ScheduleTarget{
			All: true,
		},
	}
	if err := manager.Create(ctx, schedule); err != nil {
		t.Fatalf("Failed to create schedule: %v", err)
	}

	// Disable
	if err := manager.Disable(ctx, schedule.ID, "admin"); err != nil {
		t.Errorf("Disable() error = %v", err)
	}

	got, _ := manager.Get(ctx, schedule.ID)
	if got.Status != ScheduleStatusDisabled {
		t.Errorf("Status = %v, want disabled", got.Status)
	}
	if got.NextRun != nil {
		t.Error("NextRun should be nil when disabled")
	}

	// Enable
	if err := manager.Enable(ctx, schedule.ID, "admin"); err != nil {
		t.Errorf("Enable() error = %v", err)
	}

	got, _ = manager.Get(ctx, schedule.ID)
	if got.Status != ScheduleStatusActive {
		t.Errorf("Status = %v, want active", got.Status)
	}
	if got.NextRun == nil {
		t.Error("NextRun should be calculated when enabled")
	}

	// Try to pause a disabled schedule
	if err := manager.Disable(ctx, schedule.ID, "admin"); err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
	if err := manager.Pause(ctx, schedule.ID, "admin"); err != ErrScheduleDisabled {
		t.Errorf("Pause() on disabled schedule should return ErrScheduleDisabled, got %v", err)
	}
}

func TestScheduleManager_TriggerNow(t *testing.T) {
	store := NewMockStore()
	config := &ManagerConfig{MemberID: "member-1"}
	manager, err := NewScheduleManager(config, store)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	ctx := context.Background()

	// Create schedule
	schedule := &Schedule{
		Name: "test-schedule",
		Type: ScheduleTypeCommand,
		Cron: "0 2 * * *",
		Target: &ScheduleTarget{
			All: true,
		},
	}
	if err := manager.Create(ctx, schedule); err != nil {
		t.Fatalf("Failed to create schedule: %v", err)
	}

	// Trigger now
	exec, err := manager.TriggerNow(ctx, schedule.ID, "admin")
	if err != nil {
		t.Fatalf("TriggerNow() error = %v", err)
	}

	if exec.ScheduleID != schedule.ID {
		t.Errorf("ScheduleID = %v, want %v", exec.ScheduleID, schedule.ID)
	}
	if exec.TriggerType != TriggerTypeManual {
		t.Errorf("TriggerType = %v, want manual", exec.TriggerType)
	}
	if exec.TriggeredBy != "admin" {
		t.Errorf("TriggeredBy = %v, want admin", exec.TriggeredBy)
	}

	// Trigger disabled schedule should fail
	if err := manager.Disable(ctx, schedule.ID, "admin"); err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
	_, err = manager.TriggerNow(ctx, schedule.ID, "admin")
	if err != ErrScheduleDisabled {
		t.Errorf("TriggerNow() on disabled schedule should return ErrScheduleDisabled, got %v", err)
	}
}

func TestScheduleManager_ApprovalWorkflow(t *testing.T) {
	store := NewMockStore()
	config := &ManagerConfig{MemberID: "member-1"}
	manager, err := NewScheduleManager(config, store)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	ctx := context.Background()

	// Create schedule requiring approval
	schedule := &Schedule{
		Name:            "test-schedule",
		Type:            ScheduleTypeCommand,
		Cron:            "0 2 * * *",
		RequireApproval: true,
		Target: &ScheduleTarget{
			All: true,
		},
	}
	if err := manager.Create(ctx, schedule); err != nil {
		t.Fatalf("Failed to create schedule: %v", err)
	}

	// Trigger - should create pending execution
	exec, err := manager.TriggerNow(ctx, schedule.ID, "admin")
	if err != nil {
		t.Fatalf("TriggerNow() error = %v", err)
	}
	if exec.Status != ExecutionStatusPending {
		t.Errorf("Status = %v, want pending", exec.Status)
	}

	// Approve
	if err := manager.ApproveExecution(ctx, exec.ID, "approver"); err != nil {
		t.Errorf("ApproveExecution() error = %v", err)
	}

	exec, err = manager.GetExecution(ctx, exec.ID)
	if err != nil {
		t.Fatalf("GetExecution() error = %v", err)
	}
	if exec.Status != ExecutionStatusApproved {
		t.Errorf("Status = %v, want approved", exec.Status)
	}
	if exec.ApprovedBy != "approver" {
		t.Errorf("ApprovedBy = %v, want approver", exec.ApprovedBy)
	}
}

func TestScheduleManager_RejectExecution(t *testing.T) {
	store := NewMockStore()
	config := &ManagerConfig{MemberID: "member-1"}
	manager, err := NewScheduleManager(config, store)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	ctx := context.Background()

	// Create schedule requiring approval
	schedule := &Schedule{
		Name:            "test-schedule",
		Type:            ScheduleTypeCommand,
		Cron:            "0 2 * * *",
		RequireApproval: true,
		Target: &ScheduleTarget{
			All: true,
		},
	}
	if err := manager.Create(ctx, schedule); err != nil {
		t.Fatalf("Failed to create schedule: %v", err)
	}

	// Trigger
	exec, err := manager.TriggerNow(ctx, schedule.ID, "admin")
	if err != nil {
		t.Fatalf("TriggerNow() error = %v", err)
	}

	// Reject
	if err := manager.RejectExecution(ctx, exec.ID, "reviewer", "not approved"); err != nil {
		t.Errorf("RejectExecution() error = %v", err)
	}

	exec, err = manager.GetExecution(ctx, exec.ID)
	if err != nil {
		t.Fatalf("GetExecution() error = %v", err)
	}
	if exec.Status != ExecutionStatusRejected {
		t.Errorf("Status = %v, want rejected", exec.Status)
	}
	if exec.RejectedBy != "reviewer" {
		t.Errorf("RejectedBy = %v, want reviewer", exec.RejectedBy)
	}
	if exec.RejectionReason != "not approved" {
		t.Errorf("RejectionReason = %v, want 'not approved'", exec.RejectionReason)
	}
}

func TestScheduleManager_GetStats(t *testing.T) {
	store := NewMockStore()
	config := &ManagerConfig{MemberID: "member-1"}
	manager, err := NewScheduleManager(config, store)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	ctx := context.Background()

	// Create schedules
	schedule1 := &Schedule{
		Name: "schedule-1",
		Type: ScheduleTypeCommand,
		Cron: "0 2 * * *",
		Target: &ScheduleTarget{
			All: true,
		},
	}
	if err := manager.Create(ctx, schedule1); err != nil {
		t.Fatalf("Failed to create schedule: %v", err)
	}

	schedule2 := &Schedule{
		Name: "schedule-2",
		Type: ScheduleTypeState,
		Cron: "0 3 * * *",
		Target: &ScheduleTarget{
			All: true,
		},
	}
	if err := manager.Create(ctx, schedule2); err != nil {
		t.Fatalf("Failed to create schedule: %v", err)
	}

	// Pause one
	if err := manager.Pause(ctx, schedule2.ID, "admin"); err != nil {
		t.Fatalf("Pause() error = %v", err)
	}

	// Get stats
	stats, err := manager.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}

	if stats.TotalSchedules != 2 {
		t.Errorf("TotalSchedules = %v, want 2", stats.TotalSchedules)
	}
	if stats.ActiveSchedules != 1 {
		t.Errorf("ActiveSchedules = %v, want 1", stats.ActiveSchedules)
	}
	if stats.PausedSchedules != 1 {
		t.Errorf("PausedSchedules = %v, want 1", stats.PausedSchedules)
	}
	if stats.ByType[ScheduleTypeCommand] != 1 {
		t.Errorf("ByType[command] = %v, want 1", stats.ByType[ScheduleTypeCommand])
	}
	if stats.ByType[ScheduleTypeState] != 1 {
		t.Errorf("ByType[state] = %v, want 1", stats.ByType[ScheduleTypeState])
	}
}

func TestScheduleManager_RecordExecutionResult(t *testing.T) {
	store := NewMockStore()
	config := &ManagerConfig{
		MemberID:            "member-1",
		MaxExecutionHistory: 10,
	}
	manager, err := NewScheduleManager(config, store)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	ctx := context.Background()

	// Create schedule
	schedule := &Schedule{
		Name: "test-schedule",
		Type: ScheduleTypeCommand,
		Cron: "0 2 * * *",
		Target: &ScheduleTarget{
			All: true,
		},
	}
	if err := manager.Create(ctx, schedule); err != nil {
		t.Fatalf("Failed to create schedule: %v", err)
	}

	// Trigger
	exec, err := manager.TriggerNow(ctx, schedule.ID, "admin")
	if err != nil {
		t.Fatalf("TriggerNow() error = %v", err)
	}

	// Record success
	now := time.Now().UTC()
	exec.Status = ExecutionStatusCompleted
	exec.EndTime = &now
	exec.Duration = time.Minute
	exec.SuccessCount = 5
	exec.FailureCount = 0

	if err := manager.RecordExecutionResult(ctx, exec); err != nil {
		t.Errorf("RecordExecutionResult() error = %v", err)
	}

	// Verify schedule stats updated
	schedule, err = manager.Get(ctx, schedule.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if schedule.RunCount != 1 {
		t.Errorf("RunCount = %v, want 1", schedule.RunCount)
	}
	if schedule.SuccessCount != 1 {
		t.Errorf("SuccessCount = %v, want 1", schedule.SuccessCount)
	}
	if schedule.LastRun == nil {
		t.Error("LastRun should be set")
	}
	if schedule.NextRun == nil {
		t.Error("NextRun should be recalculated")
	}
}

func TestScheduleManager_EventEmission(t *testing.T) {
	store := NewMockStore()
	config := &ManagerConfig{MemberID: "member-1"}
	manager, err := NewScheduleManager(config, store)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	ctx := context.Background()

	// Track events
	var events []*ScheduleEvent
	manager.AddListener(func(event *ScheduleEvent) {
		events = append(events, event)
	})

	// Create schedule
	schedule := &Schedule{
		Name: "test-schedule",
		Type: ScheduleTypeCommand,
		Cron: "0 2 * * *",
		Target: &ScheduleTarget{
			All: true,
		},
	}
	if err := manager.Create(ctx, schedule); err != nil {
		t.Fatalf("Failed to create schedule: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}
	if events[0].Type != string(ScheduleEventCreated) {
		t.Errorf("Event type = %v, want schedule.created", events[0].Type)
	}

	// Update
	schedule.Name = "updated"
	if err := manager.Update(ctx, schedule); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("Expected 2 events, got %d", len(events))
	}
	if events[1].Type != string(ScheduleEventUpdated) {
		t.Errorf("Event type = %v, want schedule.updated", events[1].Type)
	}

	// Delete
	if err := manager.Delete(ctx, schedule.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("Expected 3 events, got %d", len(events))
	}
	if events[2].Type != string(ScheduleEventDeleted) {
		t.Errorf("Event type = %v, want schedule.deleted", events[2].Type)
	}
}

func TestScheduleManager_Close(t *testing.T) {
	store := NewMockStore()
	config := &ManagerConfig{MemberID: "member-1"}
	manager, err := NewScheduleManager(config, store)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Close
	if err := manager.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Operations should fail after close
	ctx := context.Background()
	schedule := &Schedule{
		Name: "test-schedule",
		Type: ScheduleTypeCommand,
		Cron: "0 2 * * *",
		Target: &ScheduleTarget{
			All: true,
		},
	}
	if err := manager.Create(ctx, schedule); err != ErrStoreClosed {
		t.Errorf("Create() after close should return ErrStoreClosed, got %v", err)
	}
}

func TestScheduleManager_WindowValidation(t *testing.T) {
	store := NewMockStore()
	config := &ManagerConfig{MemberID: "member-1"}
	manager, err := NewScheduleManager(config, store)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	ctx := context.Background()

	tests := []struct {
		name    string
		window  *TimeWindow
		cron    string
		wantErr bool
	}{
		{
			name: "valid window with compatible cron",
			window: &TimeWindow{
				StartTime: "00:00",
				EndTime:   "23:59",
			},
			cron:    "0 2 * * *", // 2 AM falls within 00:00-23:59
			wantErr: false,
		},
		{
			name: "invalid start time format",
			window: &TimeWindow{
				StartTime: "9am",
				EndTime:   "17:00",
			},
			cron:    "0 10 * * *",
			wantErr: true,
		},
		{
			name: "invalid end time format",
			window: &TimeWindow{
				StartTime: "09:00",
				EndTime:   "5pm",
			},
			cron:    "0 10 * * *",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schedule := &Schedule{
				Name:   "window-test",
				Type:   ScheduleTypeCommand,
				Cron:   tt.cron,
				Window: tt.window,
				Target: &ScheduleTarget{
					All: true,
				},
			}
			err := manager.Create(ctx, schedule)
			if (err != nil) != tt.wantErr {
				t.Errorf("Create() with window error = %v, wantErr %v", err, tt.wantErr)
			}
			store.Reset()
		})
	}
}

func TestScheduleManager_TimezoneValidation(t *testing.T) {
	store := NewMockStore()
	config := &ManagerConfig{MemberID: "member-1"}
	manager, err := NewScheduleManager(config, store)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	ctx := context.Background()

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
			schedule := &Schedule{
				Name:     "tz-test",
				Type:     ScheduleTypeCommand,
				Cron:     "0 2 * * *",
				Timezone: tt.timezone,
				Target: &ScheduleTarget{
					All: true,
				},
			}
			err := manager.Create(ctx, schedule)
			if (err != nil) != tt.wantErr {
				t.Errorf("Create() with timezone %s error = %v, wantErr %v", tt.timezone, err, tt.wantErr)
			}
			store.Reset()
		})
	}
}
