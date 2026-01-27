package trigger

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/runbook"
	"github.com/shawnbutts/keystone-core/internal/schedule"
)

func TestScheduleTrigger_Validate(t *testing.T) {
	tests := []struct {
		name    string
		trigger *ScheduleTrigger
		wantErr bool
	}{
		{
			name: "valid with cron",
			trigger: &ScheduleTrigger{
				ID:         "test-trigger",
				Name:       "Test Trigger",
				RunbookRef: RunbookRef{Name: "test-runbook"},
				Cron:       "0 * * * *",
				Enabled:    true,
			},
			wantErr: false,
		},
		{
			name: "valid with interval",
			trigger: &ScheduleTrigger{
				ID:         "test-trigger",
				Name:       "Test Trigger",
				RunbookRef: RunbookRef{Name: "test-runbook"},
				Interval:   time.Hour,
				Enabled:    true,
			},
			wantErr: false,
		},
		{
			name: "missing id",
			trigger: &ScheduleTrigger{
				Name:       "Test Trigger",
				RunbookRef: RunbookRef{Name: "test-runbook"},
				Cron:       "0 * * * *",
			},
			wantErr: true,
		},
		{
			name: "missing name",
			trigger: &ScheduleTrigger{
				ID:         "test-trigger",
				RunbookRef: RunbookRef{Name: "test-runbook"},
				Cron:       "0 * * * *",
			},
			wantErr: true,
		},
		{
			name: "missing runbook",
			trigger: &ScheduleTrigger{
				ID:   "test-trigger",
				Name: "Test Trigger",
				Cron: "0 * * * *",
			},
			wantErr: true,
		},
		{
			name: "missing cron and interval",
			trigger: &ScheduleTrigger{
				ID:         "test-trigger",
				Name:       "Test Trigger",
				RunbookRef: RunbookRef{Name: "test-runbook"},
			},
			wantErr: true,
		},
		{
			name: "both cron and interval",
			trigger: &ScheduleTrigger{
				ID:         "test-trigger",
				Name:       "Test Trigger",
				RunbookRef: RunbookRef{Name: "test-runbook"},
				Cron:       "0 * * * *",
				Interval:   time.Hour,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.trigger.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestScheduleTrigger_ToSchedule(t *testing.T) {
	trigger := &ScheduleTrigger{
		ID:          "deploy-daily",
		Name:        "Daily Deploy",
		Description: "Deploy every day at midnight",
		RunbookRef: RunbookRef{
			Name:    "deploy-app",
			Version: "1.0.0",
		},
		Cron:     "0 0 * * *",
		Timezone: "UTC",
		Inputs: map[string]interface{}{
			"environment": "production",
		},
		Enabled: true,
		Tags: map[string]string{
			"team": "platform",
		},
	}

	sched, err := trigger.ToSchedule()
	if err != nil {
		t.Fatalf("ToSchedule() error = %v", err)
	}

	if sched.ID != trigger.ID {
		t.Errorf("ID = %v, want %v", sched.ID, trigger.ID)
	}
	if sched.Name != trigger.Name {
		t.Errorf("Name = %v, want %v", sched.Name, trigger.Name)
	}
	if sched.Cron != trigger.Cron {
		t.Errorf("Cron = %v, want %v", sched.Cron, trigger.Cron)
	}
	if sched.Type != ScheduleTypeRunbook {
		t.Errorf("Type = %v, want %v", sched.Type, ScheduleTypeRunbook)
	}
	if sched.Status != schedule.ScheduleStatusActive {
		t.Errorf("Status = %v, want %v", sched.Status, schedule.ScheduleStatusActive)
	}

	// Check disabled trigger creates disabled schedule
	trigger.Enabled = false
	sched, _ = trigger.ToSchedule()
	if sched.Status != schedule.ScheduleStatusDisabled {
		t.Errorf("Status = %v, want %v for disabled trigger", sched.Status, schedule.ScheduleStatusDisabled)
	}
}

func TestScheduleHandler_Execute(t *testing.T) {
	repo := newMockRepository()
	executor := newMockExecutor()
	publisher := newMockPublisher()

	repo.Add(&runbook.Runbook{
		APIVersion: "runbook.keystone.io/v1",
		Kind:       "Runbook",
		Metadata:   runbook.Metadata{Name: "deploy-app"},
		Spec: runbook.RunbookSpec{
			Steps: []runbook.Step{
				{Name: "deploy", Type: runbook.StepTypeNoop},
			},
		},
	})

	handler := NewScheduleHandler(repo, executor, publisher)

	// Create schedule with runbook payload
	sched := &schedule.Schedule{
		ID:      "sched-1",
		Name:    "Deploy Schedule",
		Type:    ScheduleTypeRunbook,
		Payload: []byte(`{"runbook_name":"deploy-app","inputs":{"env":"prod"}}`),
	}

	exec := &schedule.ScheduleExecution{
		ID:         "exec-1",
		ScheduleID: sched.ID,
	}

	err := handler.Execute(context.Background(), sched, exec)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Verify runbook was executed
	if executor.ExecutionCount() != 1 {
		t.Errorf("ExecutionCount = %d, want 1", executor.ExecutionCount())
	}

	// Verify inputs were passed
	lastExec := executor.LastExecution()
	if lastExec.inputs["env"] != "prod" {
		t.Errorf("Input env = %v, want %q", lastExec.inputs["env"], "prod")
	}
	if lastExec.inputs["__schedule_id"] != sched.ID {
		t.Errorf("Input __schedule_id = %v, want %q", lastExec.inputs["__schedule_id"], sched.ID)
	}

	// Verify events were published
	if publisher.EventCount() < 2 {
		t.Errorf("EventCount = %d, want at least 2", publisher.EventCount())
	}
}

func TestScheduleHandler_ValidatePayload(t *testing.T) {
	repo := newMockRepository()
	executor := newMockExecutor()

	repo.Add(&runbook.Runbook{
		APIVersion: "runbook.keystone.io/v1",
		Kind:       "Runbook",
		Metadata:   runbook.Metadata{Name: "existing-runbook"},
		Spec: runbook.RunbookSpec{
			Steps: []runbook.Step{{Name: "step1", Type: runbook.StepTypeNoop}},
		},
	})

	handler := NewScheduleHandler(repo, executor, nil)

	tests := []struct {
		name    string
		payload string
		wantErr bool
	}{
		{
			name:    "valid payload",
			payload: `{"runbook_name":"existing-runbook"}`,
			wantErr: false,
		},
		{
			name:    "missing runbook name",
			payload: `{"inputs":{"foo":"bar"}}`,
			wantErr: true,
		},
		{
			name:    "nonexistent runbook",
			payload: `{"runbook_name":"nonexistent"}`,
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			payload: `{invalid}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sched := &schedule.Schedule{
				ID:      "test",
				Payload: []byte(tt.payload),
			}
			err := handler.Validate(sched)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestScheduleTriggerManager_RegisterAndGet(t *testing.T) {
	manager := NewScheduleTriggerManager(nil, nil, nil, nil)

	trigger := &ScheduleTrigger{
		ID:         "test-trigger",
		Name:       "Test Trigger",
		RunbookRef: RunbookRef{Name: "test-runbook"},
		Cron:       "0 * * * *",
		Enabled:    true,
	}

	// Register
	if err := manager.Register(trigger); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Get
	got, ok := manager.Get("test-trigger")
	if !ok {
		t.Fatal("Get() returned false")
	}
	if got.ID != trigger.ID {
		t.Errorf("Get() ID = %v, want %v", got.ID, trigger.ID)
	}

	// List
	triggers := manager.List()
	if len(triggers) != 1 {
		t.Errorf("List() returned %d triggers, want 1", len(triggers))
	}

	// Duplicate registration
	if err := manager.Register(trigger); err == nil {
		t.Error("Expected error for duplicate registration")
	}

	// Unregister
	if err := manager.Unregister("test-trigger"); err != nil {
		t.Fatalf("Unregister() error = %v", err)
	}

	_, ok = manager.Get("test-trigger")
	if ok {
		t.Error("Get() should return false after unregister")
	}
}

func TestScheduleTriggerManager_EnableDisable(t *testing.T) {
	manager := NewScheduleTriggerManager(nil, nil, nil, nil)

	trigger := &ScheduleTrigger{
		ID:         "test-trigger",
		Name:       "Test Trigger",
		RunbookRef: RunbookRef{Name: "test-runbook"},
		Cron:       "0 * * * *",
		Enabled:    true,
	}
	manager.Register(trigger)

	// Disable
	if err := manager.Disable("test-trigger"); err != nil {
		t.Fatalf("Disable() error = %v", err)
	}

	got, _ := manager.Get("test-trigger")
	if got.Enabled {
		t.Error("Trigger should be disabled")
	}

	// Enable
	if err := manager.Enable("test-trigger"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	got, _ = manager.Get("test-trigger")
	if !got.Enabled {
		t.Error("Trigger should be enabled")
	}
}
