package trigger

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shawnbutts/keystone-core/internal/events"
)

// Registry manages trigger registration and event processing.
type Registry struct {
	mu       sync.RWMutex
	triggers map[string]*Trigger
	states   map[string]*triggerState

	// Dependencies
	repository RunbookRepository
	executor   RunbookExecutor
	publisher  events.EventPublisher
	matcher    FilterParser

	// Deduplication
	dedup *Deduplicator

	// Rate limiting
	rateLimiter *RateLimiter

	// Metrics
	metrics *TriggerMetrics

	// Callbacks
	onTriggerActivation func(trigger *Trigger, event *events.Event)
	onExecutionStart    func(trigger *Trigger, executionID string)
	onExecutionComplete func(trigger *Trigger, executionID string, err error)
}

// FilterParser parses filter expressions.
type FilterParser interface {
	// Parse parses a filter expression string.
	Parse(expr string) (events.FilterExpression, error)
}

// RegistryOption configures a Registry.
type RegistryOption func(*Registry)

// WithRepository sets the runbook repository.
func WithRepository(repo RunbookRepository) RegistryOption {
	return func(r *Registry) {
		r.repository = repo
	}
}

// WithExecutor sets the runbook executor.
func WithExecutor(executor RunbookExecutor) RegistryOption {
	return func(r *Registry) {
		r.executor = executor
	}
}

// WithPublisher sets the event publisher for trigger events.
func WithPublisher(publisher events.EventPublisher) RegistryOption {
	return func(r *Registry) {
		r.publisher = publisher
	}
}

// WithFilterParser sets the filter expression parser.
func WithFilterParser(parser FilterParser) RegistryOption {
	return func(r *Registry) {
		r.matcher = parser
	}
}

// WithCallbacks sets trigger lifecycle callbacks.
func WithCallbacks(
	onActivation func(trigger *Trigger, event *events.Event),
	onStart func(trigger *Trigger, executionID string),
	onComplete func(trigger *Trigger, executionID string, err error),
) RegistryOption {
	return func(r *Registry) {
		r.onTriggerActivation = onActivation
		r.onExecutionStart = onStart
		r.onExecutionComplete = onComplete
	}
}

// NewRegistry creates a new trigger registry.
func NewRegistry(opts ...RegistryOption) *Registry {
	r := &Registry{
		triggers:    make(map[string]*Trigger),
		states:      make(map[string]*triggerState),
		dedup:       NewDeduplicator(),
		rateLimiter: NewRateLimiter(),
		metrics: &TriggerMetrics{
			ByTrigger: make(map[string]*TriggerStats),
		},
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

// Register adds a trigger to the registry.
func (r *Registry) Register(trigger *Trigger) error {
	if err := trigger.Validate(); err != nil {
		return fmt.Errorf("invalid trigger: %w", err)
	}

	// Validate filter expression
	if r.matcher != nil {
		if _, err := r.matcher.Parse(trigger.Filter); err != nil {
			return fmt.Errorf("invalid filter expression: %w", err)
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.triggers[trigger.ID]; exists {
		return fmt.Errorf("trigger %s already registered", trigger.ID)
	}

	// Set timestamps
	now := time.Now()
	if trigger.CreatedAt.IsZero() {
		trigger.CreatedAt = now
	}
	trigger.UpdatedAt = now

	r.triggers[trigger.ID] = trigger
	r.states[trigger.ID] = &triggerState{}
	r.metrics.TotalTriggers++
	if trigger.Enabled {
		r.metrics.EnabledTriggers++
	}

	r.metrics.ByTrigger[trigger.ID] = &TriggerStats{
		TriggerID: trigger.ID,
	}

	return nil
}

// Unregister removes a trigger from the registry.
func (r *Registry) Unregister(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	trigger, exists := r.triggers[id]
	if !exists {
		return fmt.Errorf("trigger %s not found", id)
	}

	// Cancel any pending debounce timer
	if state, ok := r.states[id]; ok {
		state.mu.Lock()
		if state.debounceTimer != nil {
			state.debounceTimer.Stop()
		}
		state.mu.Unlock()
	}

	delete(r.triggers, id)
	delete(r.states, id)
	delete(r.metrics.ByTrigger, id)

	r.metrics.TotalTriggers--
	if trigger.Enabled {
		r.metrics.EnabledTriggers--
	}

	return nil
}

// Get retrieves a trigger by ID.
func (r *Registry) Get(id string) (*Trigger, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	trigger, ok := r.triggers[id]
	return trigger, ok
}

// List returns all registered triggers.
func (r *Registry) List() []*Trigger {
	r.mu.RLock()
	defer r.mu.RUnlock()

	triggers := make([]*Trigger, 0, len(r.triggers))
	for _, t := range r.triggers {
		triggers = append(triggers, t)
	}

	// Sort by priority (descending) then name
	sort.Slice(triggers, func(i, j int) bool {
		if triggers[i].Priority != triggers[j].Priority {
			return triggers[i].Priority > triggers[j].Priority
		}
		return triggers[i].Name < triggers[j].Name
	})

	return triggers
}

// Enable enables a trigger.
func (r *Registry) Enable(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	trigger, exists := r.triggers[id]
	if !exists {
		return fmt.Errorf("trigger %s not found", id)
	}

	if !trigger.Enabled {
		trigger.Enabled = true
		trigger.UpdatedAt = time.Now()
		r.metrics.EnabledTriggers++
	}

	return nil
}

// Disable disables a trigger.
func (r *Registry) Disable(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	trigger, exists := r.triggers[id]
	if !exists {
		return fmt.Errorf("trigger %s not found", id)
	}

	if trigger.Enabled {
		trigger.Enabled = false
		trigger.UpdatedAt = time.Now()
		r.metrics.EnabledTriggers--
	}

	return nil
}

// ProcessEvent processes an event against all registered triggers.
func (r *Registry) ProcessEvent(ctx context.Context, event *events.Event) error {
	r.mu.RLock()
	triggers := make([]*Trigger, 0)
	for _, t := range r.triggers {
		if t.Enabled {
			triggers = append(triggers, t)
		}
	}
	r.mu.RUnlock()

	// Sort by priority
	sort.Slice(triggers, func(i, j int) bool {
		return triggers[i].Priority > triggers[j].Priority
	})

	// Process each matching trigger
	for _, trigger := range triggers {
		if err := r.processTrigger(ctx, trigger, event); err != nil {
			// Log error but continue processing other triggers
			if r.publisher != nil {
				_ = r.publisher.Publish(&events.Event{
					ID:     uuid.New().String(),
					Type:   events.EventType("runbook.trigger.error"),
					Source: "/runbook/trigger/" + trigger.ID,
					Time:   time.Now(),
					Data: map[string]interface{}{
						"trigger_id": trigger.ID,
						"event_id":   event.ID,
						"error":      err.Error(),
					},
				})
			}
		}
	}

	return nil
}

// processTrigger processes a single trigger for an event.
func (r *Registry) processTrigger(ctx context.Context, trigger *Trigger, event *events.Event) error {
	// Parse and check filter
	if r.matcher != nil {
		filter, err := r.matcher.Parse(trigger.Filter)
		if err != nil {
			return fmt.Errorf("parse filter: %w", err)
		}
		if !filter.Matches(event) {
			return nil
		}
	}

	// Get trigger state
	r.mu.RLock()
	state, ok := r.states[trigger.ID]
	r.mu.RUnlock()
	if !ok {
		return fmt.Errorf("trigger state not found")
	}

	// Check conditions
	if skip, reason := r.shouldSkip(trigger, state, event); skip {
		r.recordSkip(trigger, state, reason)
		return nil
	}

	// Handle debounce
	if trigger.Conditions != nil && trigger.Conditions.Debounce > 0 {
		return r.handleDebounce(ctx, trigger, state, event)
	}

	// Execute immediately
	return r.executeTrigger(ctx, trigger, state, event)
}

// shouldSkip checks if execution should be skipped.
func (r *Registry) shouldSkip(trigger *Trigger, state *triggerState, event *events.Event) (bool, string) {
	if trigger.Conditions == nil {
		return false, ""
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	// Check throttle
	if trigger.Conditions.Throttle > 0 {
		if time.Since(state.lastActivation) < trigger.Conditions.Throttle {
			return true, "throttled"
		}
	}

	// Check max concurrent
	if trigger.Conditions.MaxConcurrent > 0 {
		if state.activeCount >= trigger.Conditions.MaxConcurrent {
			return true, "max_concurrent"
		}
	}

	// Check deduplication
	if trigger.Conditions.Deduplication != nil && trigger.Conditions.Deduplication.Enabled {
		key := r.dedup.GenerateKey(trigger.Conditions.Deduplication.Key, event)
		if r.dedup.IsDuplicate(trigger.ID, key, trigger.Conditions.Deduplication.Window) {
			return true, "deduplicated"
		}
	}

	// Check rate limit
	if trigger.Conditions.RateLimit != nil {
		if !r.rateLimiter.Allow(trigger.ID, trigger.Conditions.RateLimit.MaxExecutions, trigger.Conditions.RateLimit.Window) {
			return true, "rate_limited"
		}
	}

	// Check time window
	if trigger.Conditions.TimeWindow != nil {
		if !r.isInTimeWindow(trigger.Conditions.TimeWindow) {
			return true, "outside_time_window"
		}
	}

	return false, ""
}

// isInTimeWindow checks if current time is within the time window.
func (r *Registry) isInTimeWindow(window *TimeWindowConfig) bool {
	loc := time.Local
	if window.Timezone != "" {
		if tz, err := time.LoadLocation(window.Timezone); err == nil {
			loc = tz
		}
	}

	now := time.Now().In(loc)

	// Check day of week
	if len(window.Days) > 0 {
		dayMatch := false
		for _, day := range window.Days {
			if int(now.Weekday()) == day {
				dayMatch = true
				break
			}
		}
		if !dayMatch {
			return false
		}
	}

	// Parse start and end times
	startParts := parseTime(window.Start)
	endParts := parseTime(window.End)

	if startParts == nil || endParts == nil {
		return true // Invalid time format, allow
	}

	currentMinutes := now.Hour()*60 + now.Minute()
	startMinutes := startParts[0]*60 + startParts[1]
	endMinutes := endParts[0]*60 + endParts[1]

	// Handle overnight windows (e.g., 22:00 to 06:00)
	if endMinutes < startMinutes {
		return currentMinutes >= startMinutes || currentMinutes <= endMinutes
	}

	return currentMinutes >= startMinutes && currentMinutes <= endMinutes
}

// parseTime parses HH:MM format into [hour, minute].
func parseTime(s string) []int {
	var hour, minute int
	n, err := fmt.Sscanf(s, "%d:%d", &hour, &minute)
	if err != nil || n != 2 {
		return nil
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return nil
	}
	return []int{hour, minute}
}

// handleDebounce handles debounced trigger execution.
func (r *Registry) handleDebounce(ctx context.Context, trigger *Trigger, state *triggerState, event *events.Event) error {
	state.mu.Lock()
	defer state.mu.Unlock()

	// Cancel existing timer
	if state.debounceTimer != nil {
		state.debounceTimer.Stop()
	}

	// Store the event
	state.pendingEvent = event

	// Set new timer
	state.debounceTimer = time.AfterFunc(trigger.Conditions.Debounce, func() {
		state.mu.Lock()
		pendingEvent := state.pendingEvent
		state.pendingEvent = nil
		state.debounceTimer = nil
		state.mu.Unlock()

		if pendingEvent != nil {
			_ = r.executeTrigger(ctx, trigger, state, pendingEvent)
		}
	})

	return nil
}

// executeTrigger executes a trigger.
func (r *Registry) executeTrigger(ctx context.Context, trigger *Trigger, state *triggerState, event *events.Event) error {
	// Update state
	state.mu.Lock()
	state.lastActivation = time.Now()
	state.activations++
	state.activeCount++
	state.mu.Unlock()

	// Record dedup key
	if trigger.Conditions != nil && trigger.Conditions.Deduplication != nil && trigger.Conditions.Deduplication.Enabled {
		key := r.dedup.GenerateKey(trigger.Conditions.Deduplication.Key, event)
		r.dedup.Record(trigger.ID, key)
	}

	// Update metrics
	r.mu.Lock()
	r.metrics.TotalActivations++
	if stats, ok := r.metrics.ByTrigger[trigger.ID]; ok {
		stats.Activations++
		stats.LastActivation = time.Now()
	}
	r.mu.Unlock()

	// Callback
	if r.onTriggerActivation != nil {
		r.onTriggerActivation(trigger, event)
	}

	// Publish activation event
	if r.publisher != nil {
		_ = r.publisher.Publish(&events.Event{
			ID:     uuid.New().String(),
			Type:   events.EventType("runbook.trigger.activated"),
			Source: "/runbook/trigger/" + trigger.ID,
			Time:   time.Now(),
			Data: map[string]interface{}{
				"trigger_id":   trigger.ID,
				"trigger_name": trigger.Name,
				"event_id":     event.ID,
				"event_type":   string(event.Type),
			},
		})
	}

	// Execute runbook
	var execErr error
	var executionID string

	if r.repository != nil && r.executor != nil {
		// Get runbook
		rb, err := r.repository.GetRunbook(trigger.RunbookRef.Name, trigger.RunbookRef.Version)
		if err != nil {
			execErr = fmt.Errorf("get runbook: %w", err)
		} else {
			// Build inputs from event
			inputs := r.buildInputs(trigger, event)

			// Callback
			if r.onExecutionStart != nil {
				r.onExecutionStart(trigger, "")
			}

			// Execute
			exec, err := r.executor.Execute(rb, inputs)
			if err != nil {
				execErr = err
			} else {
				executionID = exec.ID
			}

			// Callback
			if r.onExecutionComplete != nil {
				r.onExecutionComplete(trigger, executionID, execErr)
			}
		}
	}

	// Update state
	state.mu.Lock()
	state.activeCount--
	if execErr != nil {
		state.failures++
	} else {
		state.successes++
	}
	state.mu.Unlock()

	// Update metrics
	r.mu.Lock()
	if stats, ok := r.metrics.ByTrigger[trigger.ID]; ok {
		if execErr != nil {
			stats.Failures++
			r.metrics.FailedExecutions++
		} else {
			stats.Successes++
			r.metrics.SuccessfulExecutions++
		}
	}
	r.mu.Unlock()

	// Publish completion event
	if r.publisher != nil {
		eventData := map[string]interface{}{
			"trigger_id":   trigger.ID,
			"trigger_name": trigger.Name,
			"event_id":     event.ID,
			"execution_id": executionID,
			"success":      execErr == nil,
		}
		if execErr != nil {
			eventData["error"] = execErr.Error()
		}
		_ = r.publisher.Publish(&events.Event{
			ID:     uuid.New().String(),
			Type:   events.EventType("runbook.trigger.completed"),
			Source: "/runbook/trigger/" + trigger.ID,
			Time:   time.Now(),
			Data:   eventData,
		})
	}

	return execErr
}

// buildInputs builds runbook inputs from event data.
func (r *Registry) buildInputs(trigger *Trigger, event *events.Event) map[string]interface{} {
	inputs := make(map[string]interface{})

	// Add default event data
	inputs["__event_id"] = event.ID
	inputs["__event_type"] = string(event.Type)
	inputs["__event_source"] = event.Source
	inputs["__event_time"] = event.Time

	// Copy event tags
	for k, v := range event.Tags {
		inputs["event_tag_"+k] = v
	}

	// Copy event data
	for k, v := range event.Data {
		inputs["event_"+k] = v
	}

	// Apply input mappings
	for inputName, template := range trigger.InputMappings {
		value := r.resolveTemplate(template, event)
		inputs[inputName] = value
	}

	return inputs
}

// resolveTemplate resolves a simple template against event data.
func (r *Registry) resolveTemplate(template string, event *events.Event) interface{} {
	// Use the shared template resolver
	return resolveEventTemplate(template, event)
}

// recordSkip records a skipped execution.
func (r *Registry) recordSkip(trigger *Trigger, state *triggerState, reason string) {
	state.mu.Lock()
	state.skipped++
	state.mu.Unlock()

	r.mu.Lock()
	r.metrics.SkippedExecutions++
	if stats, ok := r.metrics.ByTrigger[trigger.ID]; ok {
		stats.Skipped++
	}
	r.mu.Unlock()

	// Publish skip event
	if r.publisher != nil {
		_ = r.publisher.Publish(&events.Event{
			ID:     uuid.New().String(),
			Type:   events.EventType("runbook.trigger.skipped"),
			Source: "/runbook/trigger/" + trigger.ID,
			Time:   time.Now(),
			Data: map[string]interface{}{
				"trigger_id": trigger.ID,
				"reason":     reason,
			},
		})
	}
}

// GetMetrics returns current trigger metrics.
func (r *Registry) GetMetrics() *TriggerMetrics {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Deep copy metrics
	metrics := &TriggerMetrics{
		TotalTriggers:        r.metrics.TotalTriggers,
		EnabledTriggers:      r.metrics.EnabledTriggers,
		TotalActivations:     r.metrics.TotalActivations,
		SuccessfulExecutions: r.metrics.SuccessfulExecutions,
		FailedExecutions:     r.metrics.FailedExecutions,
		SkippedExecutions:    r.metrics.SkippedExecutions,
		AverageLatencyMs:     r.metrics.AverageLatencyMs,
		ByTrigger:            make(map[string]*TriggerStats),
	}

	for id, stats := range r.metrics.ByTrigger {
		metrics.ByTrigger[id] = &TriggerStats{
			TriggerID:      stats.TriggerID,
			Activations:    stats.Activations,
			Successes:      stats.Successes,
			Failures:       stats.Failures,
			Skipped:        stats.Skipped,
			LastActivation: stats.LastActivation,
		}
	}

	return metrics
}

// Close shuts down the registry.
func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Stop all debounce timers
	for _, state := range r.states {
		state.mu.Lock()
		if state.debounceTimer != nil {
			state.debounceTimer.Stop()
		}
		state.mu.Unlock()
	}

	// Close deduplicator
	r.dedup.Close()

	return nil
}
