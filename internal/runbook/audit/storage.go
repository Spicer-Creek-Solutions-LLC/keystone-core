package audit

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

// MemoryStorage provides an in-memory implementation of Storage.
// Useful for testing and development.
type MemoryStorage struct {
	mu     sync.RWMutex
	events []*Event
}

// NewMemoryStorage creates a new in-memory audit storage.
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		events: make([]*Event, 0),
	}
}

// Store saves an audit event.
func (s *MemoryStorage) Store(ctx context.Context, event *Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Make a copy to prevent external modification
	eventCopy := *event
	if event.Details != nil {
		eventCopy.Details = make(map[string]interface{})
		for k, v := range event.Details {
			eventCopy.Details[k] = v
		}
	}
	if event.Metadata != nil {
		eventCopy.Metadata = make(map[string]string)
		for k, v := range event.Metadata {
			eventCopy.Metadata[k] = v
		}
	}

	s.events = append(s.events, &eventCopy)
	return nil
}

// Query searches for audit events matching criteria.
func (s *MemoryStorage) Query(ctx context.Context, query *Query) ([]*Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*Event

	for _, event := range s.events {
		if s.matches(event, query) {
			results = append(results, event)
		}
	}

	// Sort results
	if query.OrderBy != "" || query.OrderDesc {
		sort.Slice(results, func(i, j int) bool {
			var less bool
			switch query.OrderBy {
			case "timestamp", "":
				less = results[i].Timestamp.Before(results[j].Timestamp)
			case "type":
				less = results[i].Type < results[j].Type
			case "execution_id":
				less = results[i].ExecutionID < results[j].ExecutionID
			default:
				less = results[i].Timestamp.Before(results[j].Timestamp)
			}
			if query.OrderDesc {
				return !less
			}
			return less
		})
	}

	// Apply offset and limit
	if query.Offset > 0 {
		if query.Offset >= len(results) {
			return []*Event{}, nil
		}
		results = results[query.Offset:]
	}

	if query.Limit > 0 && len(results) > query.Limit {
		results = results[:query.Limit]
	}

	return results, nil
}

// GetByExecutionID retrieves all events for an execution.
func (s *MemoryStorage) GetByExecutionID(ctx context.Context, executionID string) ([]*Event, error) {
	return s.Query(ctx, &Query{
		ExecutionID: executionID,
		OrderBy:     "timestamp",
	})
}

// Delete removes events older than the given time.
func (s *MemoryStorage) Delete(ctx context.Context, before time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var kept []*Event
	var deleted int64

	for _, event := range s.events {
		if event.Timestamp.Before(before) {
			deleted++
		} else {
			kept = append(kept, event)
		}
	}

	s.events = kept
	return deleted, nil
}

// Count returns the number of events matching criteria.
func (s *MemoryStorage) Count(ctx context.Context, query *Query) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int64
	for _, event := range s.events {
		if s.matches(event, query) {
			count++
		}
	}

	return count, nil
}

// matches checks if an event matches a query.
func (s *MemoryStorage) matches(event *Event, query *Query) bool {
	if query == nil {
		return true
	}

	if query.ExecutionID != "" && event.ExecutionID != query.ExecutionID {
		return false
	}

	if query.RunbookName != "" && event.RunbookName != query.RunbookName {
		return false
	}

	if query.Actor != "" && event.Actor != query.Actor {
		return false
	}

	if query.Outcome != "" && event.Outcome != query.Outcome {
		return false
	}

	if len(query.Types) > 0 {
		found := false
		for _, t := range query.Types {
			if event.Type == t {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if query.StartTime != nil && event.Timestamp.Before(*query.StartTime) {
		return false
	}

	if query.EndTime != nil && event.Timestamp.After(*query.EndTime) {
		return false
	}

	return true
}

// All returns all stored events.
func (s *MemoryStorage) All() []*Event {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Event, len(s.events))
	copy(result, s.events)
	return result
}

// Clear removes all events.
func (s *MemoryStorage) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = s.events[:0]
}

// ExecutionHistoryView provides a structured view of an execution's audit trail.
type ExecutionHistoryView struct {
	ExecutionID    string            `json:"execution_id"`
	RunbookName    string            `json:"runbook_name"`
	RunbookVersion string            `json:"runbook_version,omitempty"`
	StartedAt      *time.Time        `json:"started_at,omitempty"`
	CompletedAt    *time.Time        `json:"completed_at,omitempty"`
	Duration       time.Duration     `json:"duration,omitempty"`
	Status         string            `json:"status"`
	Actor          string            `json:"actor,omitempty"`
	Steps          []StepHistoryView `json:"steps"`
	Events         []*Event     `json:"events"`
}

// StepHistoryView provides a view of a step's audit trail.
type StepHistoryView struct {
	Name        string        `json:"name"`
	Status      string        `json:"status"`
	StartedAt   *time.Time    `json:"started_at,omitempty"`
	CompletedAt *time.Time    `json:"completed_at,omitempty"`
	Duration    time.Duration `json:"duration,omitempty"`
	RetryCount  int           `json:"retry_count,omitempty"`
	Error       string        `json:"error,omitempty"`
}

// BuildExecutionHistoryView creates a structured view from audit events.
func BuildExecutionHistoryView(events []*Event) *ExecutionHistoryView {
	if len(events) == 0 {
		return nil
	}

	view := &ExecutionHistoryView{
		Events: events,
		Steps:  []StepHistoryView{},
	}

	stepViews := make(map[string]*StepHistoryView)

	for _, event := range events {
		// Set execution-level fields from first event
		if view.ExecutionID == "" {
			view.ExecutionID = event.ExecutionID
			view.RunbookName = event.RunbookName
			view.RunbookVersion = event.RunbookVersion
		}

		switch event.Type {
		case EventExecutionStarted:
			view.StartedAt = &event.Timestamp
			view.Actor = event.Actor

		case EventExecutionCompleted:
			view.CompletedAt = &event.Timestamp
			view.Status = "completed"
			view.Duration = event.Duration

		case EventExecutionFailed:
			view.CompletedAt = &event.Timestamp
			view.Status = "failed"
			view.Duration = event.Duration

		case EventExecutionCancelled:
			view.CompletedAt = &event.Timestamp
			view.Status = "cancelled"

		case EventStepStarted:
			if _, ok := stepViews[event.StepName]; !ok {
				stepViews[event.StepName] = &StepHistoryView{
					Name: event.StepName,
				}
			}
			ts := event.Timestamp
			stepViews[event.StepName].StartedAt = &ts

		case EventStepCompleted:
			if step, ok := stepViews[event.StepName]; ok {
				ts := event.Timestamp
				step.CompletedAt = &ts
				step.Status = "completed"
				step.Duration = event.Duration
			}

		case EventStepFailed:
			if step, ok := stepViews[event.StepName]; ok {
				ts := event.Timestamp
				step.CompletedAt = &ts
				step.Status = "failed"
				step.Error = event.Error
				step.Duration = event.Duration
			}

		case EventStepSkipped:
			if _, ok := stepViews[event.StepName]; !ok {
				stepViews[event.StepName] = &StepHistoryView{
					Name: event.StepName,
				}
			}
			stepViews[event.StepName].Status = "skipped"

		case EventStepRetried:
			if step, ok := stepViews[event.StepName]; ok {
				step.RetryCount++
			}

		default:
		}
	}

	// Convert step map to slice
	for _, step := range stepViews {
		view.Steps = append(view.Steps, *step)
	}

	// Sort steps by start time
	sort.Slice(view.Steps, func(i, j int) bool {
		if view.Steps[i].StartedAt == nil {
			return false
		}
		if view.Steps[j].StartedAt == nil {
			return true
		}
		return view.Steps[i].StartedAt.Before(*view.Steps[j].StartedAt)
	})

	return view
}

// SearchExecutions searches for executions matching criteria.
func SearchExecutions(ctx context.Context, storage Storage, query *Query) ([]ExecutionHistoryView, error) {
	// Get execution start events
	searchQuery := &Query{
		Types:       []EventType{EventExecutionStarted},
		RunbookName: query.RunbookName,
		Actor:       query.Actor,
		StartTime:   query.StartTime,
		EndTime:     query.EndTime,
		Limit:       query.Limit,
		Offset:      query.Offset,
		OrderBy:     "timestamp",
		OrderDesc:   query.OrderDesc,
	}

	startEvents, err := storage.Query(ctx, searchQuery)
	if err != nil {
		return nil, err
	}

	var results []ExecutionHistoryView

	for _, startEvent := range startEvents {
		events, err := storage.GetByExecutionID(ctx, startEvent.ExecutionID)
		if err != nil {
			continue
		}

		if view := BuildExecutionHistoryView(events); view != nil {
			// Filter by outcome if specified
			if query.Outcome != "" && !strings.EqualFold(view.Status, query.Outcome) {
				continue
			}
			results = append(results, *view)
		}
	}

	return results, nil
}
