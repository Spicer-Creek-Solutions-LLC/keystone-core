package events

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shawnbutts/keystone-core/internal/tracing"
)

// Action represents an automated action that can be executed in response to an event
type Action interface {
	// Execute runs the action with the given event context
	Execute(ctx context.Context, event *Event) error

	// Name returns a human-readable name for the action
	Name() string

	// Type returns the action type (e.g., "command", "state", "webhook")
	Type() string
}

// Reactor defines an automated response to events
type Reactor struct {
	// ID uniquely identifies the reactor
	ID string

	// Name is a human-readable name
	Name string

	// Description describes what this reactor does
	Description string

	// Filter determines which events trigger this reactor
	Filter FilterExpression

	// Actions to execute when the filter matches
	Actions []Action

	// Enabled indicates if the reactor is active
	Enabled bool

	// Priority determines execution order (higher = earlier)
	Priority int

	// MaxConcurrent limits concurrent executions (0 = unlimited)
	MaxConcurrent int

	// Timeout for action execution (0 = no timeout)
	Timeout time.Duration

	// OnError defines behavior when an action fails
	OnError ErrorBehavior

	// Conditions for advanced control flow
	Conditions *ReactorConditions
}

// ErrorBehavior defines how to handle action failures
type ErrorBehavior string

const (
	// ErrorBehaviorContinue continues executing remaining actions
	ErrorBehaviorContinue ErrorBehavior = "continue"

	// ErrorBehaviorStop stops executing remaining actions
	ErrorBehaviorStop ErrorBehavior = "stop"

	// ErrorBehaviorRetry retries the failed action
	ErrorBehaviorRetry ErrorBehavior = "retry"
)

// ReactorConditions provides advanced control flow
type ReactorConditions struct {
	// OnlyIf - only execute if this expression is true
	OnlyIf FilterExpression

	// Unless - skip execution if this expression is true
	Unless FilterExpression

	// Throttle - minimum time between executions
	Throttle time.Duration

	// Debounce - wait for quiet period before executing
	Debounce time.Duration

	// MaxExecutions - maximum number of times to execute (0 = unlimited)
	MaxExecutions int

	// TimeWindow - time window for MaxExecutions
	TimeWindow time.Duration
}

// ReactorEngine manages and executes reactors
type ReactorEngine struct {
	reactors map[string]*Reactor
	mu       sync.RWMutex

	// Execution state
	executions     map[string]*reactorExecution
	executionMu    sync.RWMutex
	metrics        *ReactorMetrics
	eventPublisher EventPublisher

	// Dead letter queue for failed executions
	deadLetterQueue DeadLetterQueue

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc
}

// reactorExecution tracks execution state for a reactor
type reactorExecution struct {
	reactor         *Reactor
	lastExecution   time.Time
	executionCount  int
	activeCount     int32
	mu              sync.Mutex
	throttleLock    sync.Mutex
	debouncedEvents chan *Event
	debounceTimer   *time.Timer
}

// ReactorMetrics tracks reactor performance
type ReactorMetrics struct {
	// Total events evaluated
	EventsEvaluated uint64

	// Total reactor executions triggered
	ExecutionsTriggered uint64

	// Total successful executions
	ExecutionsSucceeded uint64

	// Total failed executions
	ExecutionsFailed uint64

	// Total throttled executions
	ExecutionsThrottled uint64

	// Per-reactor metrics
	mu            sync.RWMutex
	reactorMetrics map[string]*ReactorExecutionMetrics
}

// ReactorExecutionMetrics tracks metrics for a single reactor
type ReactorExecutionMetrics struct {
	// Total events matched by this reactor's filter
	EventsMatched uint64

	// Total executions triggered
	Triggered uint64

	// Total successful executions
	Succeeded uint64

	// Total failed executions
	Failed uint64

	// Total throttled executions
	Throttled uint64

	// Total debounced events
	Debounced uint64

	// Mutex for protecting time.Time and string fields
	mu sync.RWMutex

	// Last execution time
	LastExecution time.Time

	// Last error time
	LastError time.Time

	// Last error message
	LastErrorMsg string

	// Average execution duration
	AvgDurationMs uint64

	// Total execution time (for calculating average)
	totalDurationMs uint64
	executionCount  uint64
}

// NewReactorEngine creates a new reactor engine
func NewReactorEngine() *ReactorEngine {
	ctx, cancel := context.WithCancel(context.Background())
	return &ReactorEngine{
		reactors:   make(map[string]*Reactor),
		executions: make(map[string]*reactorExecution),
		metrics: &ReactorMetrics{
			reactorMetrics: make(map[string]*ReactorExecutionMetrics),
		},
		ctx:    ctx,
		cancel: cancel,
	}
}

// SetEventPublisher sets the event publisher for emitting reactor events
func (e *ReactorEngine) SetEventPublisher(publisher EventPublisher) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.eventPublisher = publisher
}

// SetDeadLetterQueue sets the dead letter queue for failed executions
func (e *ReactorEngine) SetDeadLetterQueue(dlq DeadLetterQueue) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.deadLetterQueue = dlq
}

// GetDeadLetterQueue returns the configured dead letter queue
func (e *ReactorEngine) GetDeadLetterQueue() DeadLetterQueue {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.deadLetterQueue
}

// AddReactor adds a reactor to the engine
func (e *ReactorEngine) AddReactor(reactor *Reactor) error {
	if reactor == nil {
		return fmt.Errorf("reactor cannot be nil")
	}
	if reactor.ID == "" {
		return fmt.Errorf("reactor ID cannot be empty")
	}
	if reactor.Filter == nil {
		return fmt.Errorf("reactor filter cannot be nil")
	}
	if len(reactor.Actions) == 0 {
		return fmt.Errorf("reactor must have at least one action")
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// Check for duplicate ID
	if _, exists := e.reactors[reactor.ID]; exists {
		return fmt.Errorf("reactor with ID %s already exists", reactor.ID)
	}

	// Set defaults
	if reactor.OnError == "" {
		reactor.OnError = ErrorBehaviorContinue
	}

	// Initialize execution state
	exec := &reactorExecution{
		reactor: reactor,
	}

	// Set up debouncing if needed
	if reactor.Conditions != nil && reactor.Conditions.Debounce > 0 {
		exec.debouncedEvents = make(chan *Event, 100)
		go e.debounceHandler(reactor.ID)
	}

	e.reactors[reactor.ID] = reactor
	e.executionMu.Lock()
	e.executions[reactor.ID] = exec
	e.executionMu.Unlock()

	// Initialize metrics
	e.metrics.mu.Lock()
	e.metrics.reactorMetrics[reactor.ID] = &ReactorExecutionMetrics{}
	e.metrics.mu.Unlock()

	return nil
}

// RemoveReactor removes a reactor from the engine
func (e *ReactorEngine) RemoveReactor(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.reactors[id]; !exists {
		return fmt.Errorf("reactor %s not found", id)
	}

	delete(e.reactors, id)

	e.executionMu.Lock()
	if exec := e.executions[id]; exec != nil {
		if exec.debouncedEvents != nil {
			close(exec.debouncedEvents)
		}
		if exec.debounceTimer != nil {
			exec.debounceTimer.Stop()
		}
	}
	delete(e.executions, id)
	e.executionMu.Unlock()

	e.metrics.mu.Lock()
	delete(e.metrics.reactorMetrics, id)
	e.metrics.mu.Unlock()

	return nil
}

// EnableReactor enables a reactor
func (e *ReactorEngine) EnableReactor(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	reactor, exists := e.reactors[id]
	if !exists {
		return fmt.Errorf("reactor %s not found", id)
	}

	reactor.Enabled = true
	return nil
}

// DisableReactor disables a reactor
func (e *ReactorEngine) DisableReactor(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	reactor, exists := e.reactors[id]
	if !exists {
		return fmt.Errorf("reactor %s not found", id)
	}

	reactor.Enabled = false
	return nil
}

// ProcessEvent processes an event through all reactors
func (e *ReactorEngine) ProcessEvent(event *Event) error {
	// Start tracing span for event processing
	ctx, span := tracing.StartEventSpan(e.ctx, tracing.SpanEventProcess,
		tracing.StringAttr(tracing.AttrEventType, string(event.Type)),
		tracing.StringAttr("event.source", event.Source),
	)
	defer span.End()

	// Add correlation ID if present
	if event.CorrelationID != "" {
		tracing.SetAttributes(span, tracing.StringAttr("event.correlation_id", event.CorrelationID))
	}

	atomic.AddUint64(&e.metrics.EventsEvaluated, 1)

	e.mu.RLock()
	reactors := make([]*Reactor, 0, len(e.reactors))
	for _, r := range e.reactors {
		if r.Enabled {
			reactors = append(reactors, r)
		}
	}
	e.mu.RUnlock()

	tracing.SetAttributes(span, tracing.IntAttr("event.reactors_count", len(reactors)))

	// Sort by priority (highest first)
	sortReactorsByPriority(reactors)

	matchedCount := 0
	var lastErr error
	for _, reactor := range reactors {
		// Check if event matches filter
		if !reactor.Filter.Matches(event) {
			continue
		}

		matchedCount++

		// Track matched event
		e.metrics.mu.RLock()
		metrics := e.metrics.reactorMetrics[reactor.ID]
		e.metrics.mu.RUnlock()
		atomic.AddUint64(&metrics.EventsMatched, 1)

		// Execute reactor
		if err := e.executeReactor(ctx, reactor, event); err != nil {
			lastErr = err
		}
	}

	tracing.SetAttributes(span, tracing.IntAttr("event.reactors_matched", matchedCount))

	if lastErr != nil {
		tracing.RecordError(span, lastErr)
	} else {
		tracing.RecordSuccess(span, fmt.Sprintf("processed event with %d reactor matches", matchedCount))
	}

	return lastErr
}

// executeReactor executes a single reactor for an event
func (e *ReactorEngine) executeReactor(ctx context.Context, reactor *Reactor, event *Event) error {
	// Start tracing span for reactor execution
	ctx, span := tracing.StartEventSpan(ctx, tracing.SpanReactorExecute,
		tracing.StringAttr("reactor.id", reactor.ID),
		tracing.StringAttr("reactor.name", reactor.Name),
		tracing.IntAttr("reactor.priority", reactor.Priority),
	)
	defer span.End()

	e.executionMu.RLock()
	exec := e.executions[reactor.ID]
	e.executionMu.RUnlock()

	// Check conditions
	if reactor.Conditions != nil {
		// Check OnlyIf
		if reactor.Conditions.OnlyIf != nil && !reactor.Conditions.OnlyIf.Matches(event) {
			return nil
		}

		// Check Unless
		if reactor.Conditions.Unless != nil && reactor.Conditions.Unless.Matches(event) {
			return nil
		}

		// Check throttle
		if reactor.Conditions.Throttle > 0 {
			exec.throttleLock.Lock()
			timeSinceLastExec := time.Since(exec.lastExecution)
			if timeSinceLastExec < reactor.Conditions.Throttle {
				exec.throttleLock.Unlock()
				e.metrics.mu.RLock()
				metrics := e.metrics.reactorMetrics[reactor.ID]
				e.metrics.mu.RUnlock()
				atomic.AddUint64(&metrics.Throttled, 1)
				atomic.AddUint64(&e.metrics.ExecutionsThrottled, 1)
				return nil
			}
			exec.throttleLock.Unlock()
		}

		// Check max executions
		if reactor.Conditions.MaxExecutions > 0 {
			exec.mu.Lock()
			if exec.executionCount >= reactor.Conditions.MaxExecutions {
				exec.mu.Unlock()
				return nil
			}
			exec.mu.Unlock()
		}

		// Handle debounce
		if reactor.Conditions.Debounce > 0 {
			exec.debouncedEvents <- event
			return nil
		}
	}

	// Check max concurrent and increment if allowed
	if reactor.MaxConcurrent > 0 {
		// Try to increment active count
		for {
			active := atomic.LoadInt32(&exec.activeCount)
			if active >= int32(reactor.MaxConcurrent) {
				return fmt.Errorf("max concurrent executions reached")
			}
			// Atomically increment if still under limit
			if atomic.CompareAndSwapInt32(&exec.activeCount, active, active+1) {
				break
			}
		}
		// Decrement will happen in executeActions
	} else {
		// No limit, just increment
		atomic.AddInt32(&exec.activeCount, 1)
	}

	// Execute actions
	go e.executeActions(reactor, event, exec)

	return nil
}

// executeActions executes all actions for a reactor
func (e *ReactorEngine) executeActions(reactor *Reactor, event *Event, exec *reactorExecution) {
	// activeCount was already incremented before spawning goroutine
	defer atomic.AddInt32(&exec.activeCount, -1)

	e.metrics.mu.RLock()
	metrics := e.metrics.reactorMetrics[reactor.ID]
	e.metrics.mu.RUnlock()

	atomic.AddUint64(&metrics.Triggered, 1)
	atomic.AddUint64(&e.metrics.ExecutionsTriggered, 1)

	startTime := time.Now()

	// Update last execution time
	exec.throttleLock.Lock()
	exec.lastExecution = startTime
	exec.throttleLock.Unlock()

	exec.mu.Lock()
	exec.executionCount++
	exec.mu.Unlock()

	// Create context with timeout
	ctx := e.ctx
	if reactor.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, reactor.Timeout)
		defer cancel()
	}

	// Execute actions
	success := true
	var lastErr error

	var failedActionIndex int = -1
	var failedActionName string

	for i, action := range reactor.Actions {
		err := action.Execute(ctx, event)
		if err != nil {
			lastErr = err
			success = false
			failedActionIndex = i
			failedActionName = action.Name()

			// Handle error based on OnError behavior
			if reactor.OnError == ErrorBehaviorStop {
				break
			} else if reactor.OnError == ErrorBehaviorRetry {
				// Simple retry once
				err = action.Execute(ctx, event)
				if err == nil {
					success = true
					failedActionIndex = -1
					failedActionName = ""
					continue
				}
			}
			// ErrorBehaviorContinue - just continue to next action
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			lastErr = ctx.Err()
			success = false
			break
		default:
		}

		// Emit action event
		e.emitActionEvent(reactor, action, i, event, err)
	}

	duration := time.Since(startTime)

	// Update metrics
	if success {
		atomic.AddUint64(&metrics.Succeeded, 1)
		atomic.AddUint64(&e.metrics.ExecutionsSucceeded, 1)
	} else {
		atomic.AddUint64(&metrics.Failed, 1)
		atomic.AddUint64(&e.metrics.ExecutionsFailed, 1)
		metrics.mu.Lock()
		metrics.LastError = time.Now()
		if lastErr != nil {
			metrics.LastErrorMsg = lastErr.Error()
		}
		metrics.mu.Unlock()

		// Enqueue to dead letter queue if configured
		e.enqueueToDeadLetterQueue(reactor, event, failedActionIndex, failedActionName, lastErr)
	}

	metrics.mu.Lock()
	metrics.LastExecution = time.Now()
	metrics.mu.Unlock()

	// Update average duration
	atomic.AddUint64(&metrics.totalDurationMs, uint64(duration.Milliseconds()))
	count := atomic.AddUint64(&metrics.executionCount, 1)
	atomic.StoreUint64(&metrics.AvgDurationMs, atomic.LoadUint64(&metrics.totalDurationMs)/count)

	// Emit reactor execution event
	e.emitReactorEvent(reactor, event, success, duration, lastErr)
}

// debounceHandler handles debounced events for a reactor
func (e *ReactorEngine) debounceHandler(reactorID string) {
	e.executionMu.RLock()
	exec := e.executions[reactorID]
	e.executionMu.RUnlock()

	if exec == nil || exec.debouncedEvents == nil {
		return
	}

	reactor := exec.reactor
	var lastEvent *Event

	for event := range exec.debouncedEvents {
		lastEvent = event

		// Reset timer
		if exec.debounceTimer != nil {
			exec.debounceTimer.Stop()
		}

		exec.debounceTimer = time.AfterFunc(reactor.Conditions.Debounce, func() {
			if lastEvent != nil {
				e.metrics.mu.RLock()
				metrics := e.metrics.reactorMetrics[reactorID]
				e.metrics.mu.RUnlock()
				atomic.AddUint64(&metrics.Debounced, 1)

				e.executeActions(reactor, lastEvent, exec)
			}
		})
	}
}

// emitReactorEvent emits an event for reactor execution
func (e *ReactorEngine) emitReactorEvent(reactor *Reactor, triggerEvent *Event, success bool, duration time.Duration, err error) {
	if e.eventPublisher == nil {
		return
	}

	eventType := EventType("reactor.execute")
	severity := SeverityInfo
	if !success {
		severity = SeverityError
	}

	data := map[string]interface{}{
		"reactor_id":        reactor.ID,
		"reactor_name":      reactor.Name,
		"trigger_event_id":  triggerEvent.ID,
		"trigger_event_type": string(triggerEvent.Type),
		"success":           success,
		"duration_ms":       duration.Milliseconds(),
		"action_count":      len(reactor.Actions),
	}

	if err != nil {
		data["error"] = err.Error()
	}

	event := NewEvent(eventType).
		Source("/reactor-engine").
		Severity(severity).
		CorrelationID(triggerEvent.ID).
		DataMap(data).
		Build()

	e.eventPublisher.PublishAsync(event)
}

// emitActionEvent emits an event for action execution
func (e *ReactorEngine) emitActionEvent(reactor *Reactor, action Action, index int, triggerEvent *Event, err error) {
	if e.eventPublisher == nil {
		return
	}

	eventType := EventType("reactor.action")
	severity := SeverityInfo
	if err != nil {
		severity = SeverityWarning
	}

	data := map[string]interface{}{
		"reactor_id":        reactor.ID,
		"action_name":       action.Name(),
		"action_type":       action.Type(),
		"action_index":      index,
		"trigger_event_id":  triggerEvent.ID,
		"success":           err == nil,
	}

	if err != nil {
		data["error"] = err.Error()
	}

	event := NewEvent(eventType).
		Source("/reactor-engine").
		Severity(severity).
		CorrelationID(triggerEvent.ID).
		DataMap(data).
		Build()

	e.eventPublisher.PublishAsync(event)
}

// enqueueToDeadLetterQueue adds a failed execution to the dead letter queue
func (e *ReactorEngine) enqueueToDeadLetterQueue(reactor *Reactor, event *Event, actionIndex int, actionName string, err error) {
	e.mu.RLock()
	dlq := e.deadLetterQueue
	e.mu.RUnlock()

	if dlq == nil || err == nil {
		return
	}

	entry := &DeadLetterEntry{
		ReactorID:   reactor.ID,
		ReactorName: reactor.Name,
		Event:       event,
		ActionIndex: actionIndex,
		ActionName:  actionName,
		Error:       err.Error(),
		Metadata: map[string]interface{}{
			"reactor_priority":    reactor.Priority,
			"reactor_description": reactor.Description,
			"on_error":            string(reactor.OnError),
		},
	}

	// Enqueue in background to not block execution
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		dlq.Enqueue(ctx, entry)
	}()
}

// GetReactor returns a reactor by ID
func (e *ReactorEngine) GetReactor(id string) (*Reactor, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	reactor, exists := e.reactors[id]
	if !exists {
		return nil, fmt.Errorf("reactor %s not found", id)
	}

	// Return a copy
	reactorCopy := *reactor
	return &reactorCopy, nil
}

// ListReactors returns all reactors
func (e *ReactorEngine) ListReactors() []*Reactor {
	e.mu.RLock()
	defer e.mu.RUnlock()

	reactors := make([]*Reactor, 0, len(e.reactors))
	for _, r := range e.reactors {
		reactorCopy := *r
		reactors = append(reactors, &reactorCopy)
	}

	return reactors
}

// GetMetrics returns a snapshot of reactor metrics
func (e *ReactorEngine) GetMetrics() *ReactorMetrics {
	e.metrics.mu.RLock()
	defer e.metrics.mu.RUnlock()

	snapshot := &ReactorMetrics{
		EventsEvaluated:      atomic.LoadUint64(&e.metrics.EventsEvaluated),
		ExecutionsTriggered:  atomic.LoadUint64(&e.metrics.ExecutionsTriggered),
		ExecutionsSucceeded:  atomic.LoadUint64(&e.metrics.ExecutionsSucceeded),
		ExecutionsFailed:     atomic.LoadUint64(&e.metrics.ExecutionsFailed),
		ExecutionsThrottled:  atomic.LoadUint64(&e.metrics.ExecutionsThrottled),
		reactorMetrics:       make(map[string]*ReactorExecutionMetrics),
	}

	// Copy reactor metrics
	for id, metrics := range e.metrics.reactorMetrics {
		metrics.mu.RLock()
		lastExec := metrics.LastExecution
		lastErr := metrics.LastError
		lastErrMsg := metrics.LastErrorMsg
		metrics.mu.RUnlock()

		snapshot.reactorMetrics[id] = &ReactorExecutionMetrics{
			EventsMatched:   atomic.LoadUint64(&metrics.EventsMatched),
			Triggered:       atomic.LoadUint64(&metrics.Triggered),
			Succeeded:       atomic.LoadUint64(&metrics.Succeeded),
			Failed:          atomic.LoadUint64(&metrics.Failed),
			Throttled:       atomic.LoadUint64(&metrics.Throttled),
			Debounced:       atomic.LoadUint64(&metrics.Debounced),
			LastExecution:   lastExec,
			LastError:       lastErr,
			LastErrorMsg:    lastErrMsg,
			AvgDurationMs:   atomic.LoadUint64(&metrics.AvgDurationMs),
			totalDurationMs: atomic.LoadUint64(&metrics.totalDurationMs),
			executionCount:  atomic.LoadUint64(&metrics.executionCount),
		}
	}

	return snapshot
}

// GetReactorMetrics returns metrics for a specific reactor
func (e *ReactorEngine) GetReactorMetrics(id string) (*ReactorExecutionMetrics, error) {
	e.metrics.mu.RLock()
	defer e.metrics.mu.RUnlock()

	metrics, exists := e.metrics.reactorMetrics[id]
	if !exists {
		return nil, fmt.Errorf("no metrics for reactor %s", id)
	}

	metrics.mu.RLock()
	lastExec := metrics.LastExecution
	lastErr := metrics.LastError
	lastErrMsg := metrics.LastErrorMsg
	metrics.mu.RUnlock()

	return &ReactorExecutionMetrics{
		EventsMatched:   atomic.LoadUint64(&metrics.EventsMatched),
		Triggered:       atomic.LoadUint64(&metrics.Triggered),
		Succeeded:       atomic.LoadUint64(&metrics.Succeeded),
		Failed:          atomic.LoadUint64(&metrics.Failed),
		Throttled:       atomic.LoadUint64(&metrics.Throttled),
		Debounced:       atomic.LoadUint64(&metrics.Debounced),
		LastExecution:   lastExec,
		LastError:       lastErr,
		LastErrorMsg:    lastErrMsg,
		AvgDurationMs:   atomic.LoadUint64(&metrics.AvgDurationMs),
		totalDurationMs: atomic.LoadUint64(&metrics.totalDurationMs),
		executionCount:  atomic.LoadUint64(&metrics.executionCount),
	}, nil
}

// Close closes the reactor engine
func (e *ReactorEngine) Close() error {
	e.cancel()
	return nil
}

// sortReactorsByPriority sorts reactors by priority (highest first)
func sortReactorsByPriority(reactors []*Reactor) {
	for i := 1; i < len(reactors); i++ {
		for j := i; j > 0 && reactors[j].Priority > reactors[j-1].Priority; j-- {
			reactors[j], reactors[j-1] = reactors[j-1], reactors[j]
		}
	}
}
