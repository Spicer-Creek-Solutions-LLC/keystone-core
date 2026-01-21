package schedule

import (
	"time"

	"github.com/shawnbutts/keystone-core/pkg/events"
)

// Schedule event types for integration with the main event system.
const (
	// Schedule lifecycle events
	EventTypeScheduleCreated  events.EventType = "schedule.created"
	EventTypeScheduleUpdated  events.EventType = "schedule.updated"
	EventTypeScheduleDeleted  events.EventType = "schedule.deleted"
	EventTypeSchedulePaused   events.EventType = "schedule.paused"
	EventTypeScheduleResumed  events.EventType = "schedule.resumed"
	EventTypeScheduleDisabled events.EventType = "schedule.disabled"
	EventTypeScheduleEnabled  events.EventType = "schedule.enabled"

	// Schedule execution events
	EventTypeScheduleTriggered         events.EventType = "schedule.triggered"
	EventTypeScheduleExecutionStarted  events.EventType = "schedule.execution.started"
	EventTypeScheduleExecutionComplete events.EventType = "schedule.execution.complete"
	EventTypeScheduleExecutionFailed   events.EventType = "schedule.execution.failed"
	EventTypeScheduleExecutionTimeout  events.EventType = "schedule.execution.timeout"
	EventTypeScheduleExecutionPending  events.EventType = "schedule.execution.pending"
	EventTypeScheduleExecutionApproved events.EventType = "schedule.execution.approved"
	EventTypeScheduleExecutionRejected events.EventType = "schedule.execution.rejected"

	// Maintenance window events
	EventTypeMaintenanceCreated   events.EventType = "maintenance.created"
	EventTypeMaintenanceUpdated   events.EventType = "maintenance.updated"
	EventTypeMaintenanceDeleted   events.EventType = "maintenance.deleted"
	EventTypeMaintenanceApproved  events.EventType = "maintenance.approved"
	EventTypeMaintenanceStarting  events.EventType = "maintenance.starting"
	EventTypeMaintenanceStarted   events.EventType = "maintenance.started"
	EventTypeMaintenanceExtended  events.EventType = "maintenance.extended"
	EventTypeMaintenanceEnding    events.EventType = "maintenance.ending"
	EventTypeMaintenanceEnded     events.EventType = "maintenance.ended"
	EventTypeMaintenanceCancelled events.EventType = "maintenance.cancelled"
	EventTypeMaintenanceExpired   events.EventType = "maintenance.expired"
)

// EventBridge bridges schedule events to the main event system.
type EventBridge struct {
	publisher events.EventPublisher
	source    string
}

// NewEventBridge creates a new event bridge.
func NewEventBridge(publisher events.EventPublisher, source string) *EventBridge {
	if source == "" {
		source = "/schedule"
	}
	return &EventBridge{
		publisher: publisher,
		source:    source,
	}
}

// PublishScheduleEvent publishes a schedule event to the main event system.
func (b *EventBridge) PublishScheduleEvent(event *ScheduleEvent) error {
	if b.publisher == nil {
		return nil // No publisher configured, skip
	}

	e := &events.Event{
		ID:       generateEventID(),
		Type:     events.EventType(event.Type),
		Source:   b.source,
		Time:     event.Timestamp,
		Severity: events.SeverityInfo,
		Tags: map[string]string{
			"schedule_id": event.ScheduleID,
		},
		Data: map[string]interface{}{
			"schedule_id":   event.ScheduleID,
			"schedule_name": event.ScheduleName,
		},
	}

	if event.ExecutionID != "" {
		e.Tags["execution_id"] = event.ExecutionID
		e.Data["execution_id"] = event.ExecutionID
	}

	if event.Actor != "" {
		e.Data["actor"] = event.Actor
	}

	if event.Message != "" {
		e.Data["message"] = event.Message
	}

	if event.Schedule != nil {
		e.Data["schedule_type"] = string(event.Schedule.Type)
		e.Data["schedule_status"] = string(event.Schedule.Status)
	}

	// Merge additional data
	for k, v := range event.Data {
		e.Data[k] = v
	}

	return b.publisher.Publish(e)
}

// PublishMaintenanceEvent publishes a maintenance event to the main event system.
func (b *EventBridge) PublishMaintenanceEvent(event *MaintenanceEvent) error {
	if b.publisher == nil {
		return nil
	}

	severity := events.SeverityInfo
	switch event.Type {
	case string(MaintenanceEventStarted), string(MaintenanceEventEnded):
		severity = events.SeverityWarning
	case string(MaintenanceEventCancelled):
		severity = events.SeverityWarning
	}

	e := &events.Event{
		ID:       generateEventID(),
		Type:     events.EventType(event.Type),
		Source:   b.source,
		Time:     event.Timestamp,
		Severity: severity,
		Tags: map[string]string{
			"window_id": event.WindowID,
		},
		Data: map[string]interface{}{
			"window_id":   event.WindowID,
			"window_name": event.WindowName,
		},
	}

	if event.Actor != "" {
		e.Data["actor"] = event.Actor
	}

	// Merge additional data
	for k, v := range event.Data {
		e.Data[k] = v
	}

	return b.publisher.Publish(e)
}

// ScheduleManagerEventAdapter adapts ScheduleManager events to the event bridge.
type ScheduleManagerEventAdapter struct {
	bridge *EventBridge
}

// NewScheduleManagerEventAdapter creates a new adapter.
func NewScheduleManagerEventAdapter(bridge *EventBridge) *ScheduleManagerEventAdapter {
	return &ScheduleManagerEventAdapter{bridge: bridge}
}

// HandleEvent handles schedule manager events.
func (a *ScheduleManagerEventAdapter) HandleEvent(event *ScheduleEvent) {
	if a.bridge != nil {
		_ = a.bridge.PublishScheduleEvent(event)
	}
}

// MaintenanceManagerEventAdapter adapts MaintenanceWindowManager events to the event bridge.
type MaintenanceManagerEventAdapter struct {
	bridge *EventBridge
}

// NewMaintenanceManagerEventAdapter creates a new adapter.
func NewMaintenanceManagerEventAdapter(bridge *EventBridge) *MaintenanceManagerEventAdapter {
	return &MaintenanceManagerEventAdapter{bridge: bridge}
}

// HandleEvent handles maintenance manager events.
func (a *MaintenanceManagerEventAdapter) HandleEvent(event *MaintenanceEvent) {
	if a.bridge != nil {
		_ = a.bridge.PublishMaintenanceEvent(event)
	}
}

// ExecutorEventAdapter adapts Executor events to the event bridge.
type ExecutorEventAdapter struct {
	bridge *EventBridge
}

// NewExecutorEventAdapter creates a new adapter.
func NewExecutorEventAdapter(bridge *EventBridge) *ExecutorEventAdapter {
	return &ExecutorEventAdapter{bridge: bridge}
}

// HandleEvent handles executor events.
func (a *ExecutorEventAdapter) HandleEvent(event *ExecutorEvent) {
	if a.bridge == nil {
		return
	}

	severity := events.SeverityInfo
	if event.Error != "" {
		severity = events.SeverityError
	}

	e := &events.Event{
		ID:       generateEventID(),
		Type:     events.EventType(event.Type),
		Source:   a.bridge.source,
		Time:     event.Timestamp,
		Severity: severity,
		Tags: map[string]string{
			"schedule_id":  event.ScheduleID,
			"execution_id": event.ExecutionID,
		},
		Data: map[string]interface{}{
			"schedule_id":  event.ScheduleID,
			"execution_id": event.ExecutionID,
		},
	}

	if event.Message != "" {
		e.Data["message"] = event.Message
	}

	if event.Error != "" {
		e.Data["error"] = event.Error
	}

	// Merge additional data
	for k, v := range event.Data {
		e.Data[k] = v
	}

	_ = a.bridge.publisher.Publish(e)
}

// generateEventID generates a unique event ID.
func generateEventID() string {
	return time.Now().UTC().Format("20060102150405.000000000")
}

// IntegrateWithEventSystem sets up event integration for schedule components.
func IntegrateWithEventSystem(
	publisher events.EventPublisher,
	scheduleManager *ScheduleManager,
	maintenanceManager *MaintenanceWindowManager,
	executor *Executor,
) *EventBridge {
	bridge := NewEventBridge(publisher, "/schedule")

	if scheduleManager != nil {
		adapter := NewScheduleManagerEventAdapter(bridge)
		scheduleManager.AddListener(adapter.HandleEvent)
	}

	if maintenanceManager != nil {
		adapter := NewMaintenanceManagerEventAdapter(bridge)
		maintenanceManager.AddListener(adapter.HandleEvent)
	}

	if executor != nil {
		adapter := NewExecutorEventAdapter(bridge)
		executor.AddListener(adapter.HandleEvent)
	}

	return bridge
}
