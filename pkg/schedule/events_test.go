package schedule

import (
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/events"
)

// mockEventPublisher implements events.EventPublisher for testing
type mockEventPublisher struct {
	publishedEvents []*events.Event
	publishErr      error
}

func (m *mockEventPublisher) Publish(event *events.Event) error {
	if m.publishErr != nil {
		return m.publishErr
	}
	m.publishedEvents = append(m.publishedEvents, event)
	return nil
}

func (m *mockEventPublisher) PublishAsync(event *events.Event) error {
	return m.Publish(event)
}

func (m *mockEventPublisher) Close() error {
	return nil
}

func TestNewEventBridge(t *testing.T) {
	publisher := &mockEventPublisher{}

	tests := []struct {
		name       string
		publisher  events.EventPublisher
		source     string
		wantSource string
	}{
		{
			name:       "with custom source",
			publisher:  publisher,
			source:     "/custom/source",
			wantSource: "/custom/source",
		},
		{
			name:       "with empty source defaults",
			publisher:  publisher,
			source:     "",
			wantSource: "/schedule",
		},
		{
			name:       "nil publisher",
			publisher:  nil,
			source:     "",
			wantSource: "/schedule",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bridge := NewEventBridge(tt.publisher, tt.source)
			if bridge.source != tt.wantSource {
				t.Errorf("source = %v, want %v", bridge.source, tt.wantSource)
			}
		})
	}
}

func TestEventBridge_PublishScheduleEvent(t *testing.T) {
	tests := []struct {
		name       string
		publisher  *mockEventPublisher
		event      *ScheduleEvent
		wantEvents int
		wantErr    bool
	}{
		{
			name:      "publish event",
			publisher: &mockEventPublisher{},
			event: &ScheduleEvent{
				Type:         string(ScheduleEventCreated),
				ScheduleID:   "schedule-1",
				ScheduleName: "test-schedule",
				Timestamp:    time.Now().UTC(),
				Actor:        "admin",
				Message:      "Schedule created",
			},
			wantEvents: 1,
			wantErr:    false,
		},
		{
			name:      "publish with execution ID",
			publisher: &mockEventPublisher{},
			event: &ScheduleEvent{
				Type:         string(ScheduleEventTriggered),
				ScheduleID:   "schedule-1",
				ScheduleName: "test-schedule",
				ExecutionID:  "exec-1",
				Timestamp:    time.Now().UTC(),
			},
			wantEvents: 1,
			wantErr:    false,
		},
		{
			name:      "publish with schedule details",
			publisher: &mockEventPublisher{},
			event: &ScheduleEvent{
				Type:         string(ScheduleEventUpdated),
				ScheduleID:   "schedule-1",
				ScheduleName: "test-schedule",
				Schedule: &Schedule{
					Type:   ScheduleTypeCommand,
					Status: ScheduleStatusActive,
				},
				Timestamp: time.Now().UTC(),
			},
			wantEvents: 1,
			wantErr:    false,
		},
		{
			name:      "publish with additional data",
			publisher: &mockEventPublisher{},
			event: &ScheduleEvent{
				Type:       string(ScheduleEventCompleted),
				ScheduleID: "schedule-1",
				Timestamp:  time.Now().UTC(),
				Data: map[string]interface{}{
					"duration": "5m",
					"success":  true,
				},
			},
			wantEvents: 1,
			wantErr:    false,
		},
		{
			name:      "nil publisher",
			publisher: nil,
			event: &ScheduleEvent{
				Type:       string(ScheduleEventCreated),
				ScheduleID: "schedule-1",
				Timestamp:  time.Now().UTC(),
			},
			wantEvents: 0,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var pub events.EventPublisher
			if tt.publisher != nil {
				pub = tt.publisher
			}
			bridge := NewEventBridge(pub, "")

			err := bridge.PublishScheduleEvent(tt.event)
			if (err != nil) != tt.wantErr {
				t.Errorf("PublishScheduleEvent() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.publisher != nil {
				if len(tt.publisher.publishedEvents) != tt.wantEvents {
					t.Errorf("published %d events, want %d", len(tt.publisher.publishedEvents), tt.wantEvents)
				}

				if tt.wantEvents > 0 {
					e := tt.publisher.publishedEvents[0]
					if e.Source != "/schedule" {
						t.Errorf("source = %v, want /schedule", e.Source)
					}
					if e.Tags["schedule_id"] != tt.event.ScheduleID {
						t.Errorf("tags[schedule_id] = %v, want %v", e.Tags["schedule_id"], tt.event.ScheduleID)
					}
				}
			}
		})
	}
}

func TestEventBridge_PublishMaintenanceEvent(t *testing.T) {
	tests := []struct {
		name       string
		publisher  *mockEventPublisher
		event      *MaintenanceEvent
		wantEvents int
		wantErr    bool
	}{
		{
			name:      "publish event",
			publisher: &mockEventPublisher{},
			event: &MaintenanceEvent{
				Type:       string(MaintenanceEventCreated),
				WindowID:   "window-1",
				WindowName: "test-window",
				Timestamp:  time.Now().UTC(),
				Actor:      "admin",
			},
			wantEvents: 1,
			wantErr:    false,
		},
		{
			name:      "publish started event with warning severity",
			publisher: &mockEventPublisher{},
			event: &MaintenanceEvent{
				Type:       string(MaintenanceEventStarted),
				WindowID:   "window-1",
				WindowName: "test-window",
				Timestamp:  time.Now().UTC(),
			},
			wantEvents: 1,
			wantErr:    false,
		},
		{
			name:      "publish with additional data",
			publisher: &mockEventPublisher{},
			event: &MaintenanceEvent{
				Type:       string(MaintenanceEventExtended),
				WindowID:   "window-1",
				WindowName: "test-window",
				Timestamp:  time.Now().UTC(),
				Data: map[string]interface{}{
					"old_end_time": time.Now().UTC(),
					"new_end_time": time.Now().UTC().Add(time.Hour),
				},
			},
			wantEvents: 1,
			wantErr:    false,
		},
		{
			name:      "nil publisher",
			publisher: nil,
			event: &MaintenanceEvent{
				Type:      string(MaintenanceEventCreated),
				WindowID:  "window-1",
				Timestamp: time.Now().UTC(),
			},
			wantEvents: 0,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var pub events.EventPublisher
			if tt.publisher != nil {
				pub = tt.publisher
			}
			bridge := NewEventBridge(pub, "")

			err := bridge.PublishMaintenanceEvent(tt.event)
			if (err != nil) != tt.wantErr {
				t.Errorf("PublishMaintenanceEvent() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.publisher != nil {
				if len(tt.publisher.publishedEvents) != tt.wantEvents {
					t.Errorf("published %d events, want %d", len(tt.publisher.publishedEvents), tt.wantEvents)
				}

				if tt.wantEvents > 0 {
					e := tt.publisher.publishedEvents[0]
					if e.Tags["window_id"] != tt.event.WindowID {
						t.Errorf("tags[window_id] = %v, want %v", e.Tags["window_id"], tt.event.WindowID)
					}
				}
			}
		})
	}
}

func TestScheduleManagerEventAdapter(t *testing.T) {
	publisher := &mockEventPublisher{}
	bridge := NewEventBridge(publisher, "")
	adapter := NewScheduleManagerEventAdapter(bridge)

	event := &ScheduleEvent{
		Type:         string(ScheduleEventCreated),
		ScheduleID:   "schedule-1",
		ScheduleName: "test-schedule",
		Timestamp:    time.Now().UTC(),
	}

	adapter.HandleEvent(event)

	if len(publisher.publishedEvents) != 1 {
		t.Errorf("published %d events, want 1", len(publisher.publishedEvents))
	}
}

func TestScheduleManagerEventAdapter_NilBridge(t *testing.T) {
	adapter := NewScheduleManagerEventAdapter(nil)

	// Should not panic
	adapter.HandleEvent(&ScheduleEvent{
		Type:       string(ScheduleEventCreated),
		ScheduleID: "schedule-1",
		Timestamp:  time.Now().UTC(),
	})
}

func TestMaintenanceManagerEventAdapter(t *testing.T) {
	publisher := &mockEventPublisher{}
	bridge := NewEventBridge(publisher, "")
	adapter := NewMaintenanceManagerEventAdapter(bridge)

	event := &MaintenanceEvent{
		Type:       string(MaintenanceEventCreated),
		WindowID:   "window-1",
		WindowName: "test-window",
		Timestamp:  time.Now().UTC(),
	}

	adapter.HandleEvent(event)

	if len(publisher.publishedEvents) != 1 {
		t.Errorf("published %d events, want 1", len(publisher.publishedEvents))
	}
}

func TestMaintenanceManagerEventAdapter_NilBridge(t *testing.T) {
	adapter := NewMaintenanceManagerEventAdapter(nil)

	// Should not panic
	adapter.HandleEvent(&MaintenanceEvent{
		Type:      string(MaintenanceEventCreated),
		WindowID:  "window-1",
		Timestamp: time.Now().UTC(),
	})
}

func TestExecutorEventAdapter(t *testing.T) {
	publisher := &mockEventPublisher{}
	bridge := NewEventBridge(publisher, "")
	adapter := NewExecutorEventAdapter(bridge)

	event := &ExecutorEvent{
		Type:        "execution.started",
		ScheduleID:  "schedule-1",
		ExecutionID: "exec-1",
		Timestamp:   time.Now().UTC(),
		Message:     "Execution started",
	}

	adapter.HandleEvent(event)

	if len(publisher.publishedEvents) != 1 {
		t.Errorf("published %d events, want 1", len(publisher.publishedEvents))
	}

	e := publisher.publishedEvents[0]
	if e.Severity != events.SeverityInfo {
		t.Errorf("severity = %v, want info", e.Severity)
	}
}

func TestExecutorEventAdapter_WithError(t *testing.T) {
	publisher := &mockEventPublisher{}
	bridge := NewEventBridge(publisher, "")
	adapter := NewExecutorEventAdapter(bridge)

	event := &ExecutorEvent{
		Type:        "execution.failed",
		ScheduleID:  "schedule-1",
		ExecutionID: "exec-1",
		Timestamp:   time.Now().UTC(),
		Error:       "command failed",
	}

	adapter.HandleEvent(event)

	if len(publisher.publishedEvents) != 1 {
		t.Errorf("published %d events, want 1", len(publisher.publishedEvents))
	}

	e := publisher.publishedEvents[0]
	if e.Severity != events.SeverityError {
		t.Errorf("severity = %v, want error", e.Severity)
	}
}

func TestExecutorEventAdapter_NilBridge(t *testing.T) {
	adapter := NewExecutorEventAdapter(nil)

	// Should not panic
	adapter.HandleEvent(&ExecutorEvent{
		Type:        "execution.started",
		ScheduleID:  "schedule-1",
		ExecutionID: "exec-1",
		Timestamp:   time.Now().UTC(),
	})
}

func TestIntegrateWithEventSystem(t *testing.T) {
	store := NewMockStore()
	publisher := &mockEventPublisher{}

	managerConfig := &ManagerConfig{MemberID: "member-1"}
	scheduleManager, _ := NewScheduleManager(managerConfig, store)

	maintenanceConfig := &MaintenanceManagerConfig{MemberID: "member-1"}
	maintenanceManager, _ := NewMaintenanceWindowManager(maintenanceConfig, store)

	executorConfig := &ExecutorConfig{MemberID: "member-1"}
	executor, _ := NewExecutor(executorConfig, store, scheduleManager, maintenanceManager)

	bridge := IntegrateWithEventSystem(publisher, scheduleManager, maintenanceManager, executor)

	if bridge == nil {
		t.Fatal("IntegrateWithEventSystem returned nil")
	}
	if bridge.source != "/schedule" {
		t.Errorf("source = %v, want /schedule", bridge.source)
	}
}

func TestIntegrateWithEventSystem_NilComponents(t *testing.T) {
	publisher := &mockEventPublisher{}

	// Should not panic with nil components
	bridge := IntegrateWithEventSystem(publisher, nil, nil, nil)

	if bridge == nil {
		t.Fatal("IntegrateWithEventSystem returned nil")
	}
}

func TestGenerateEventID(t *testing.T) {
	id1 := generateEventID()
	id2 := generateEventID()

	if id1 == "" {
		t.Error("generateEventID() returned empty string")
	}
	if id1 == id2 {
		t.Error("generateEventID() returned duplicate IDs")
	}
}

func TestEventTypeConstants(t *testing.T) {
	// Verify schedule event type constants
	tests := []struct {
		constant events.EventType
		want     string
	}{
		{EventTypeScheduleCreated, "schedule.created"},
		{EventTypeScheduleUpdated, "schedule.updated"},
		{EventTypeScheduleDeleted, "schedule.deleted"},
		{EventTypeSchedulePaused, "schedule.paused"},
		{EventTypeScheduleResumed, "schedule.resumed"},
		{EventTypeScheduleTriggered, "schedule.triggered"},
		{EventTypeScheduleExecutionStarted, "schedule.execution.started"},
		{EventTypeScheduleExecutionComplete, "schedule.execution.complete"},
		{EventTypeScheduleExecutionFailed, "schedule.execution.failed"},
		{EventTypeScheduleExecutionTimeout, "schedule.execution.timeout"},
		{EventTypeMaintenanceCreated, "maintenance.created"},
		{EventTypeMaintenanceStarted, "maintenance.started"},
		{EventTypeMaintenanceEnded, "maintenance.ended"},
		{EventTypeMaintenanceCancelled, "maintenance.cancelled"},
	}

	for _, tt := range tests {
		if string(tt.constant) != tt.want {
			t.Errorf("constant = %v, want %v", tt.constant, tt.want)
		}
	}
}
