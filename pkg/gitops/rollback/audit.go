package rollback

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"
)

// AuditEventType represents the type of rollback audit event
type AuditEventType string

const (
	// AuditEventRollbackRequested when a rollback is first requested
	AuditEventRollbackRequested AuditEventType = "rollback_requested"

	// AuditEventApprovalRequested when approval is requested
	AuditEventApprovalRequested AuditEventType = "approval_requested"

	// AuditEventApproved when rollback is approved
	AuditEventApproved AuditEventType = "approved"

	// AuditEventRejected when rollback is rejected
	AuditEventRejected AuditEventType = "rejected"

	// AuditEventStarted when rollback execution starts
	AuditEventStarted AuditEventType = "started"

	// AuditEventCompleted when rollback completes successfully
	AuditEventCompleted AuditEventType = "completed"

	// AuditEventFailed when rollback fails
	AuditEventFailed AuditEventType = "failed"

	// AuditEventVerificationStarted when verification begins
	AuditEventVerificationStarted AuditEventType = "verification_started"

	// AuditEventVerificationPassed when verification passes
	AuditEventVerificationPassed AuditEventType = "verification_passed"

	// AuditEventVerificationFailed when verification fails
	AuditEventVerificationFailed AuditEventType = "verification_failed"

	// AuditEventCancelled when rollback is cancelled
	AuditEventCancelled AuditEventType = "cancelled"

	// AuditEventTimeout when rollback times out
	AuditEventTimeout AuditEventType = "timeout"

	// AuditEventRetry when rollback is retried
	AuditEventRetry AuditEventType = "retry"

	// AuditEventComment when a comment is added
	AuditEventComment AuditEventType = "comment"
)

// AuditEntry represents a single entry in the rollback audit trail
type AuditEntry struct {
	// ID is the unique identifier for this entry
	ID string `json:"id"`

	// RollbackID is the ID of the rollback this entry relates to
	RollbackID string `json:"rollback_id"`

	// EventType is the type of audit event
	EventType AuditEventType `json:"event_type"`

	// Timestamp is when the event occurred
	Timestamp time.Time `json:"timestamp"`

	// Actor is who/what triggered this event
	Actor string `json:"actor"`

	// ActorType identifies the type of actor
	ActorType ActorType `json:"actor_type"`

	// Reason is the reason for this event (required for some events)
	Reason string `json:"reason,omitempty"`

	// Details contains event-specific details
	Details *AuditDetails `json:"details,omitempty"`

	// PreviousStatus is the status before this event
	PreviousStatus RollbackStatus `json:"previous_status,omitempty"`

	// NewStatus is the status after this event
	NewStatus RollbackStatus `json:"new_status,omitempty"`

	// Metadata contains additional arbitrary data
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// ActorType identifies the type of actor that triggered an event
type ActorType string

const (
	ActorTypeUser    ActorType = "user"
	ActorTypeSystem  ActorType = "system"
	ActorTypeAPI     ActorType = "api"
	ActorTypeWebhook ActorType = "webhook"
)

// AuditDetails contains detailed information about an audit event
type AuditDetails struct {
	// Application affected
	Application string `json:"application,omitempty"`

	// Namespace affected
	Namespace string `json:"namespace,omitempty"`

	// FromRevision is the source revision
	FromRevision string `json:"from_revision,omitempty"`

	// ToRevision is the target revision
	ToRevision string `json:"to_revision,omitempty"`

	// Strategy used
	Strategy RollbackStrategy `json:"strategy,omitempty"`

	// Duration of the operation
	Duration time.Duration `json:"duration,omitempty"`

	// ErrorMessage if an error occurred
	ErrorMessage string `json:"error_message,omitempty"`

	// ErrorCode if applicable
	ErrorCode string `json:"error_code,omitempty"`

	// ApprovalChain lists approvers in order
	ApprovalChain []ApprovalRecord `json:"approval_chain,omitempty"`

	// AffectedResources lists resources affected
	AffectedResources []AffectedResource `json:"affected_resources,omitempty"`

	// VerificationResults if verification was run
	VerificationResults *VerificationResults `json:"verification_results,omitempty"`

	// GitDetails for git-based rollbacks
	GitDetails *GitDetails `json:"git_details,omitempty"`
}

// ApprovalRecord represents an approval/rejection in the chain
type ApprovalRecord struct {
	Approver  string         `json:"approver"`
	Decision  string         `json:"decision"` // "approved" or "rejected"
	Timestamp time.Time      `json:"timestamp"`
	Reason    string         `json:"reason,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// AffectedResource represents a resource affected by the rollback
type AffectedResource struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Action    string `json:"action"` // "reverted", "recreated", "deleted", etc.
}

// VerificationResults contains verification outcome details
type VerificationResults struct {
	Passed        bool              `json:"passed"`
	TotalChecks   int               `json:"total_checks"`
	PassedChecks  int               `json:"passed_checks"`
	FailedChecks  int               `json:"failed_checks"`
	SkippedChecks int               `json:"skipped_checks"`
	CheckResults  []CheckResult     `json:"check_results,omitempty"`
	Duration      time.Duration     `json:"duration"`
}

// CheckResult represents a single verification check result
type CheckResult struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Message string `json:"message,omitempty"`
}

// GitDetails contains git-specific rollback details
type GitDetails struct {
	Repository string   `json:"repository"`
	Branch     string   `json:"branch"`
	CommitHash string   `json:"commit_hash"`
	Author     string   `json:"author,omitempty"`
	Message    string   `json:"message,omitempty"`
	FilesChanged []string `json:"files_changed,omitempty"`
}

// AuditTrail manages the complete audit history for rollbacks
type AuditTrail struct {
	// entries indexed by rollback ID
	entries   map[string][]*AuditEntry
	mu        sync.RWMutex

	// Global entry list indexed by entry ID
	entryIndex map[string]*AuditEntry

	// Callbacks for audit events
	callbacks []AuditCallback

	// Entry counter for generating IDs
	counter int64
}

// AuditCallback is called when audit entries are added
type AuditCallback func(entry *AuditEntry)

// NewAuditTrail creates a new audit trail
func NewAuditTrail() *AuditTrail {
	return &AuditTrail{
		entries:    make(map[string][]*AuditEntry),
		entryIndex: make(map[string]*AuditEntry),
		callbacks:  make([]AuditCallback, 0),
	}
}

// OnAuditEvent registers a callback for audit events
func (a *AuditTrail) OnAuditEvent(callback AuditCallback) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.callbacks = append(a.callbacks, callback)
}

// Record records a new audit entry
func (a *AuditTrail) Record(entry *AuditEntry) error {
	if entry.RollbackID == "" {
		return fmt.Errorf("rollback ID is required")
	}
	if entry.EventType == "" {
		return fmt.Errorf("event type is required")
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Generate ID if not set
	if entry.ID == "" {
		a.counter++
		entry.ID = fmt.Sprintf("audit-%d-%d", time.Now().UnixNano(), a.counter)
	}

	// Set timestamp if not set
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	// Initialize metadata if nil
	if entry.Metadata == nil {
		entry.Metadata = make(map[string]interface{})
	}

	// Add to entries
	a.entries[entry.RollbackID] = append(a.entries[entry.RollbackID], entry)
	a.entryIndex[entry.ID] = entry

	// Notify callbacks
	for _, cb := range a.callbacks {
		go cb(entry)
	}

	return nil
}

// RecordEvent is a convenience method to record an event with minimal parameters
func (a *AuditTrail) RecordEvent(rollbackID string, eventType AuditEventType, actor string, reason string) error {
	return a.Record(&AuditEntry{
		RollbackID: rollbackID,
		EventType:  eventType,
		Actor:      actor,
		ActorType:  ActorTypeUser,
		Reason:     reason,
		Timestamp:  time.Now(),
	})
}

// RecordStatusChange records a status change event
func (a *AuditTrail) RecordStatusChange(rollbackID string, eventType AuditEventType, actor string, previousStatus, newStatus RollbackStatus, reason string) error {
	return a.Record(&AuditEntry{
		RollbackID:     rollbackID,
		EventType:      eventType,
		Actor:          actor,
		ActorType:      ActorTypeUser,
		Reason:         reason,
		PreviousStatus: previousStatus,
		NewStatus:      newStatus,
		Timestamp:      time.Now(),
	})
}

// GetEntriesForRollback returns all entries for a rollback
func (a *AuditTrail) GetEntriesForRollback(rollbackID string) []*AuditEntry {
	a.mu.RLock()
	defer a.mu.RUnlock()

	entries := a.entries[rollbackID]
	result := make([]*AuditEntry, len(entries))
	copy(result, entries)
	return result
}

// GetEntry returns a specific entry by ID
func (a *AuditTrail) GetEntry(entryID string) (*AuditEntry, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	entry, ok := a.entryIndex[entryID]
	return entry, ok
}

// AuditFilter defines criteria for filtering audit entries
type AuditFilter struct {
	// RollbackIDs to filter by
	RollbackIDs []string

	// EventTypes to filter by
	EventTypes []AuditEventType

	// Actors to filter by
	Actors []string

	// StartTime for time range filtering
	StartTime time.Time

	// EndTime for time range filtering
	EndTime time.Time

	// Application to filter by
	Application string

	// Namespace to filter by
	Namespace string

	// Limit number of results
	Limit int

	// Offset for pagination
	Offset int
}

// Query returns entries matching the filter
func (a *AuditTrail) Query(filter *AuditFilter) []*AuditEntry {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var allEntries []*AuditEntry

	// Collect entries to consider
	if len(filter.RollbackIDs) > 0 {
		for _, id := range filter.RollbackIDs {
			if entries, ok := a.entries[id]; ok {
				allEntries = append(allEntries, entries...)
			}
		}
	} else {
		for _, entries := range a.entries {
			allEntries = append(allEntries, entries...)
		}
	}

	// Filter
	result := make([]*AuditEntry, 0)
	for _, entry := range allEntries {
		if a.matchesFilter(entry, filter) {
			result = append(result, entry)
		}
	}

	// Sort by timestamp descending
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp.After(result[j].Timestamp)
	})

	// Apply offset and limit
	if filter.Offset > 0 {
		if filter.Offset >= len(result) {
			return []*AuditEntry{}
		}
		result = result[filter.Offset:]
	}

	if filter.Limit > 0 && len(result) > filter.Limit {
		result = result[:filter.Limit]
	}

	return result
}

// matchesFilter checks if an entry matches the filter criteria
func (a *AuditTrail) matchesFilter(entry *AuditEntry, filter *AuditFilter) bool {
	// Check event types
	if len(filter.EventTypes) > 0 {
		found := false
		for _, et := range filter.EventTypes {
			if entry.EventType == et {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check actors
	if len(filter.Actors) > 0 {
		found := false
		for _, actor := range filter.Actors {
			if entry.Actor == actor {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check time range
	if !filter.StartTime.IsZero() && entry.Timestamp.Before(filter.StartTime) {
		return false
	}
	if !filter.EndTime.IsZero() && entry.Timestamp.After(filter.EndTime) {
		return false
	}

	// Check application
	if filter.Application != "" && entry.Details != nil {
		if entry.Details.Application != filter.Application {
			return false
		}
	}

	// Check namespace
	if filter.Namespace != "" && entry.Details != nil {
		if entry.Details.Namespace != filter.Namespace {
			return false
		}
	}

	return true
}

// GetTimeline returns a complete timeline for a rollback
func (a *AuditTrail) GetTimeline(rollbackID string) *RollbackTimeline {
	entries := a.GetEntriesForRollback(rollbackID)

	timeline := &RollbackTimeline{
		RollbackID: rollbackID,
		Events:     make([]*TimelineEvent, 0, len(entries)),
	}

	for _, entry := range entries {
		event := &TimelineEvent{
			Timestamp:   entry.Timestamp,
			EventType:   entry.EventType,
			Actor:       entry.Actor,
			Description: a.describeEvent(entry),
			Status:      entry.NewStatus,
		}
		timeline.Events = append(timeline.Events, event)

		// Update timeline summary
		if timeline.StartTime.IsZero() || entry.Timestamp.Before(timeline.StartTime) {
			timeline.StartTime = entry.Timestamp
		}
		if entry.Timestamp.After(timeline.EndTime) {
			timeline.EndTime = entry.Timestamp
		}

		// Track final status
		if entry.NewStatus != "" {
			timeline.FinalStatus = entry.NewStatus
		}
	}

	// Sort events by timestamp
	sort.Slice(timeline.Events, func(i, j int) bool {
		return timeline.Events[i].Timestamp.Before(timeline.Events[j].Timestamp)
	})

	if !timeline.EndTime.IsZero() && !timeline.StartTime.IsZero() {
		timeline.TotalDuration = timeline.EndTime.Sub(timeline.StartTime)
	}

	return timeline
}

// describeEvent generates a human-readable description of an event
func (a *AuditTrail) describeEvent(entry *AuditEntry) string {
	switch entry.EventType {
	case AuditEventRollbackRequested:
		if entry.Reason != "" {
			return fmt.Sprintf("Rollback requested by %s: %s", entry.Actor, entry.Reason)
		}
		return fmt.Sprintf("Rollback requested by %s", entry.Actor)
	case AuditEventApprovalRequested:
		return fmt.Sprintf("Approval requested from designated approvers")
	case AuditEventApproved:
		if entry.Reason != "" {
			return fmt.Sprintf("Approved by %s: %s", entry.Actor, entry.Reason)
		}
		return fmt.Sprintf("Approved by %s", entry.Actor)
	case AuditEventRejected:
		if entry.Reason != "" {
			return fmt.Sprintf("Rejected by %s: %s", entry.Actor, entry.Reason)
		}
		return fmt.Sprintf("Rejected by %s", entry.Actor)
	case AuditEventStarted:
		if entry.Details != nil && entry.Details.ToRevision != "" {
			return fmt.Sprintf("Rollback started to revision %s", entry.Details.ToRevision)
		}
		return "Rollback execution started"
	case AuditEventCompleted:
		if entry.Details != nil {
			return fmt.Sprintf("Rollback completed: %s -> %s (took %s)",
				entry.Details.FromRevision, entry.Details.ToRevision, entry.Details.Duration)
		}
		return "Rollback completed successfully"
	case AuditEventFailed:
		if entry.Details != nil && entry.Details.ErrorMessage != "" {
			return fmt.Sprintf("Rollback failed: %s", entry.Details.ErrorMessage)
		}
		return "Rollback failed"
	case AuditEventVerificationStarted:
		return "Post-rollback verification started"
	case AuditEventVerificationPassed:
		return "Verification passed"
	case AuditEventVerificationFailed:
		if entry.Details != nil && entry.Details.ErrorMessage != "" {
			return fmt.Sprintf("Verification failed: %s", entry.Details.ErrorMessage)
		}
		return "Verification failed"
	case AuditEventCancelled:
		return fmt.Sprintf("Cancelled by %s", entry.Actor)
	case AuditEventTimeout:
		return "Operation timed out"
	case AuditEventRetry:
		return fmt.Sprintf("Retry initiated by %s", entry.Actor)
	case AuditEventComment:
		return fmt.Sprintf("%s commented: %s", entry.Actor, entry.Reason)
	default:
		return string(entry.EventType)
	}
}

// RollbackTimeline represents the complete timeline of a rollback
type RollbackTimeline struct {
	RollbackID    string            `json:"rollback_id"`
	StartTime     time.Time         `json:"start_time"`
	EndTime       time.Time         `json:"end_time"`
	TotalDuration time.Duration     `json:"total_duration"`
	FinalStatus   RollbackStatus    `json:"final_status"`
	Events        []*TimelineEvent  `json:"events"`
}

// TimelineEvent represents an event in the timeline
type TimelineEvent struct {
	Timestamp   time.Time       `json:"timestamp"`
	EventType   AuditEventType  `json:"event_type"`
	Actor       string          `json:"actor,omitempty"`
	Description string          `json:"description"`
	Status      RollbackStatus  `json:"status,omitempty"`
}

// AuditSummary provides a summary of audit activity
type AuditSummary struct {
	TotalRollbacks        int                     `json:"total_rollbacks"`
	ByStatus              map[RollbackStatus]int  `json:"by_status"`
	ByEventType           map[AuditEventType]int  `json:"by_event_type"`
	ByActor               map[string]int          `json:"by_actor"`
	ByApplication         map[string]int          `json:"by_application"`
	AverageApprovalTime   time.Duration           `json:"average_approval_time"`
	AverageRollbackTime   time.Duration           `json:"average_rollback_time"`
	RecentActivity        []*AuditEntry           `json:"recent_activity"`
	PeriodStart           time.Time               `json:"period_start"`
	PeriodEnd             time.Time               `json:"period_end"`
}

// GetSummary returns a summary of audit activity
func (a *AuditTrail) GetSummary(ctx context.Context, startTime, endTime time.Time) *AuditSummary {
	entries := a.Query(&AuditFilter{
		StartTime: startTime,
		EndTime:   endTime,
	})

	summary := &AuditSummary{
		ByStatus:      make(map[RollbackStatus]int),
		ByEventType:   make(map[AuditEventType]int),
		ByActor:       make(map[string]int),
		ByApplication: make(map[string]int),
		PeriodStart:   startTime,
		PeriodEnd:     endTime,
	}

	rollbacks := make(map[string]bool)
	approvalTimes := make([]time.Duration, 0)
	rollbackTimes := make([]time.Duration, 0)

	// Track timings per rollback
	rollbackStart := make(map[string]time.Time)
	approvalRequest := make(map[string]time.Time)

	for _, entry := range entries {
		rollbacks[entry.RollbackID] = true
		summary.ByEventType[entry.EventType]++

		if entry.Actor != "" {
			summary.ByActor[entry.Actor]++
		}

		if entry.NewStatus != "" {
			summary.ByStatus[entry.NewStatus]++
		}

		if entry.Details != nil && entry.Details.Application != "" {
			summary.ByApplication[entry.Details.Application]++
		}

		// Track timing
		switch entry.EventType {
		case AuditEventRollbackRequested:
			rollbackStart[entry.RollbackID] = entry.Timestamp
		case AuditEventApprovalRequested:
			approvalRequest[entry.RollbackID] = entry.Timestamp
		case AuditEventApproved:
			if reqTime, ok := approvalRequest[entry.RollbackID]; ok {
				approvalTimes = append(approvalTimes, entry.Timestamp.Sub(reqTime))
			}
		case AuditEventCompleted:
			if startTime, ok := rollbackStart[entry.RollbackID]; ok {
				rollbackTimes = append(rollbackTimes, entry.Timestamp.Sub(startTime))
			}
		}
	}

	summary.TotalRollbacks = len(rollbacks)

	// Calculate averages
	if len(approvalTimes) > 0 {
		var total time.Duration
		for _, t := range approvalTimes {
			total += t
		}
		summary.AverageApprovalTime = total / time.Duration(len(approvalTimes))
	}

	if len(rollbackTimes) > 0 {
		var total time.Duration
		for _, t := range rollbackTimes {
			total += t
		}
		summary.AverageRollbackTime = total / time.Duration(len(rollbackTimes))
	}

	// Get recent activity
	if len(entries) > 10 {
		summary.RecentActivity = entries[:10]
	} else {
		summary.RecentActivity = entries
	}

	return summary
}

// ExportJSON exports entries as JSON
func (a *AuditTrail) ExportJSON(filter *AuditFilter) ([]byte, error) {
	entries := a.Query(filter)
	return json.MarshalIndent(entries, "", "  ")
}

// AuditingEngine wraps the Engine with audit trail integration
type AuditingEngine struct {
	*Engine
	auditTrail *AuditTrail
}

// NewAuditingEngine creates a new engine with audit trail
func NewAuditingEngine() *AuditingEngine {
	return &AuditingEngine{
		Engine:     NewEngine(),
		auditTrail: NewAuditTrail(),
	}
}

// GetAuditTrail returns the audit trail
func (e *AuditingEngine) GetAuditTrail() *AuditTrail {
	return e.auditTrail
}

// Execute executes a rollback with audit trail
func (e *AuditingEngine) Execute(ctx context.Context, config *RollbackConfig, request *RollbackRequest) (*RollbackResult, error) {
	// Record request
	e.auditTrail.Record(&AuditEntry{
		EventType: AuditEventRollbackRequested,
		Actor:     request.RequestedBy,
		ActorType: ActorTypeUser,
		Reason:    request.Reason,
		Details: &AuditDetails{
			Application: config.Application,
			Namespace:   config.Namespace,
			Strategy:    config.Strategy,
		},
	})

	// Execute
	result, err := e.Engine.Execute(ctx, config, request)
	if result != nil {
		// Update rollback ID in the last entry
		entries := e.auditTrail.entries[""]
		if len(entries) > 0 {
			lastEntry := entries[len(entries)-1]
			lastEntry.RollbackID = result.ID
			delete(e.auditTrail.entries, "")
			e.auditTrail.entries[result.ID] = append(e.auditTrail.entries[result.ID], lastEntry)
		}

		// Record appropriate event based on result
		if result.ApprovalInfo != nil && result.ApprovalInfo.Required {
			e.auditTrail.RecordEvent(result.ID, AuditEventApprovalRequested, "system", "Approval required per configuration")
		}

		if result.Status == StatusFailed && err != nil {
			e.auditTrail.Record(&AuditEntry{
				RollbackID: result.ID,
				EventType:  AuditEventFailed,
				Actor:      "system",
				ActorType:  ActorTypeSystem,
				NewStatus:  StatusFailed,
				Details: &AuditDetails{
					ErrorMessage: err.Error(),
				},
			})
		} else if result.Status == StatusCompleted || result.Status == StatusVerified {
			e.auditTrail.Record(&AuditEntry{
				RollbackID: result.ID,
				EventType:  AuditEventCompleted,
				Actor:      "system",
				ActorType:  ActorTypeSystem,
				NewStatus:  result.Status,
				Details: &AuditDetails{
					Application:  config.Application,
					Namespace:    config.Namespace,
					FromRevision: result.PreviousRevision,
					ToRevision:   result.CurrentRevision,
					Duration:     result.Duration,
				},
			})
		}
	}

	return result, err
}

// ApproveRollback approves a rollback with audit trail
func (e *AuditingEngine) ApproveRollback(ctx context.Context, req *ApprovalRequest) error {
	result, ok := e.Engine.GetRollback(req.RollbackID)
	if ok {
		previousStatus := result.Status
		err := e.Engine.ApproveRollback(ctx, req)

		eventType := AuditEventApproved
		if !req.Approved {
			eventType = AuditEventRejected
		}

		e.auditTrail.RecordStatusChange(
			req.RollbackID,
			eventType,
			req.ApprovedBy,
			previousStatus,
			result.Status,
			req.Reason,
		)

		return err
	}
	return e.Engine.ApproveRollback(ctx, req)
}

// AddComment adds a comment to a rollback audit trail
func (e *AuditingEngine) AddComment(rollbackID, actor, comment string) error {
	return e.auditTrail.RecordEvent(rollbackID, AuditEventComment, actor, comment)
}
