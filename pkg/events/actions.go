package events

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"time"
)

// LogAction logs event information
type LogAction struct {
	name    string
	message string
	logger  func(format string, args ...interface{})
}

// NewLogAction creates a new log action
func NewLogAction(name, message string) *LogAction {
	return &LogAction{
		name:    name,
		message: message,
		logger:  func(format string, args ...interface{}) {
			fmt.Printf(format, args...)
		},
	}
}

// SetLogger sets a custom logger function
func (a *LogAction) SetLogger(logger func(format string, args ...interface{})) *LogAction {
	a.logger = logger
	return a
}

func (a *LogAction) Name() string {
	return a.name
}

func (a *LogAction) Type() string {
	return "log"
}

func (a *LogAction) Execute(ctx context.Context, event *Event) error {
	// Build message with event data
	msg := a.message
	if msg == "" {
		msg = fmt.Sprintf("Event: %s from %s", event.Type, event.Source)
	}

	a.logger("[REACTOR] %s - Event ID: %s, Type: %s, Severity: %s\n",
		msg, event.ID, event.Type, event.Severity)

	return nil
}

// EventAction emits a new event
type EventAction struct {
	name           string
	eventPublisher EventPublisher
	eventType      EventType
	severity       Severity
	source         string
	dataTemplate   map[string]interface{}
}

// NewEventAction creates a new event emission action
func NewEventAction(name string, publisher EventPublisher, eventType EventType) *EventAction {
	return &EventAction{
		name:           name,
		eventPublisher: publisher,
		eventType:      eventType,
		severity:       SeverityInfo,
		source:         "/reactor",
	}
}

// SetSeverity sets the severity for emitted events
func (a *EventAction) SetSeverity(severity Severity) *EventAction {
	a.severity = severity
	return a
}

// SetSource sets the source for emitted events
func (a *EventAction) SetSource(source string) *EventAction {
	a.source = source
	return a
}

// SetDataTemplate sets a data template for emitted events
func (a *EventAction) SetDataTemplate(data map[string]interface{}) *EventAction {
	a.dataTemplate = data
	return a
}

func (a *EventAction) Name() string {
	return a.name
}

func (a *EventAction) Type() string {
	return "event"
}

func (a *EventAction) Execute(ctx context.Context, event *Event) error {
	if a.eventPublisher == nil {
		return fmt.Errorf("event publisher not configured")
	}

	// Build new event
	newEvent := NewEvent(a.eventType).
		Source(a.source).
		Severity(a.severity).
		CorrelationID(event.ID)

	// Add template data
	if a.dataTemplate != nil {
		newEvent.DataMap(a.dataTemplate)
	}

	// Add trigger event info
	newEvent.Data("trigger_event_id", event.ID)
	newEvent.Data("trigger_event_type", string(event.Type))

	// Publish async
	return a.eventPublisher.PublishAsync(newEvent.Build())
}

// WebhookAction sends HTTP POST to a webhook URL
type WebhookAction struct {
	name       string
	url        string
	headers    map[string]string
	httpClient *http.Client
	timeout    time.Duration
}

// NewWebhookAction creates a new webhook action
func NewWebhookAction(name, url string) *WebhookAction {
	return &WebhookAction{
		name:    name,
		url:     url,
		headers: make(map[string]string),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		timeout: 30 * time.Second,
	}
}

// SetHeader sets a custom header
func (a *WebhookAction) SetHeader(key, value string) *WebhookAction {
	a.headers[key] = value
	return a
}

// SetTimeout sets the HTTP timeout
func (a *WebhookAction) SetTimeout(timeout time.Duration) *WebhookAction {
	a.timeout = timeout
	a.httpClient.Timeout = timeout
	return a
}

func (a *WebhookAction) Name() string {
	return a.name
}

func (a *WebhookAction) Type() string {
	return "webhook"
}

func (a *WebhookAction) Execute(ctx context.Context, event *Event) error {
	// Marshal event to JSON
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", a.url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	for key, value := range a.headers {
		req.Header.Set(key, value)
	}

	// Send request
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// CommandAction executes a shell command
type CommandAction struct {
	name    string
	command string
	args    []string
	env     []string
	timeout time.Duration
}

// NewCommandAction creates a new command execution action
func NewCommandAction(name, command string, args ...string) *CommandAction {
	return &CommandAction{
		name:    name,
		command: command,
		args:    args,
		timeout: 5 * time.Minute,
	}
}

// SetEnv sets environment variables for the command
func (a *CommandAction) SetEnv(env []string) *CommandAction {
	a.env = env
	return a
}

// SetTimeout sets the command timeout
func (a *CommandAction) SetTimeout(timeout time.Duration) *CommandAction {
	a.timeout = timeout
	return a
}

func (a *CommandAction) Name() string {
	return a.name
}

func (a *CommandAction) Type() string {
	return "command"
}

func (a *CommandAction) Execute(ctx context.Context, event *Event) error {
	// Create context with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()

	// Create command
	cmd := exec.CommandContext(cmdCtx, a.command, a.args...)
	if a.env != nil {
		cmd.Env = a.env
	}

	// Add event data as environment variables
	cmd.Env = append(cmd.Env, fmt.Sprintf("EVENT_ID=%s", event.ID))
	cmd.Env = append(cmd.Env, fmt.Sprintf("EVENT_TYPE=%s", event.Type))
	cmd.Env = append(cmd.Env, fmt.Sprintf("EVENT_SOURCE=%s", event.Source))
	cmd.Env = append(cmd.Env, fmt.Sprintf("EVENT_SEVERITY=%s", event.Severity))

	// Execute
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command failed: %w (output: %s)", err, string(output))
	}

	return nil
}

// FunctionAction executes a custom function
type FunctionAction struct {
	name string
	fn   func(ctx context.Context, event *Event) error
}

// NewFunctionAction creates a new function-based action
func NewFunctionAction(name string, fn func(ctx context.Context, event *Event) error) *FunctionAction {
	return &FunctionAction{
		name: name,
		fn:   fn,
	}
}

func (a *FunctionAction) Name() string {
	return a.name
}

func (a *FunctionAction) Type() string {
	return "function"
}

func (a *FunctionAction) Execute(ctx context.Context, event *Event) error {
	return a.fn(ctx, event)
}

// ConditionalAction executes different actions based on a condition
type ConditionalAction struct {
	name      string
	condition FilterExpression
	ifTrue    Action
	ifFalse   Action
}

// NewConditionalAction creates a new conditional action
func NewConditionalAction(name string, condition FilterExpression, ifTrue, ifFalse Action) *ConditionalAction {
	return &ConditionalAction{
		name:      name,
		condition: condition,
		ifTrue:    ifTrue,
		ifFalse:   ifFalse,
	}
}

func (a *ConditionalAction) Name() string {
	return a.name
}

func (a *ConditionalAction) Type() string {
	return "conditional"
}

func (a *ConditionalAction) Execute(ctx context.Context, event *Event) error {
	if a.condition.Matches(event) {
		if a.ifTrue != nil {
			return a.ifTrue.Execute(ctx, event)
		}
	} else {
		if a.ifFalse != nil {
			return a.ifFalse.Execute(ctx, event)
		}
	}
	return nil
}

// SequenceAction executes multiple actions in sequence
type SequenceAction struct {
	name    string
	actions []Action
}

// NewSequenceAction creates a new sequence action
func NewSequenceAction(name string, actions ...Action) *SequenceAction {
	return &SequenceAction{
		name:    name,
		actions: actions,
	}
}

func (a *SequenceAction) Name() string {
	return a.name
}

func (a *SequenceAction) Type() string {
	return "sequence"
}

func (a *SequenceAction) Execute(ctx context.Context, event *Event) error {
	for _, action := range a.actions {
		if err := action.Execute(ctx, event); err != nil {
			return fmt.Errorf("action %s failed: %w", action.Name(), err)
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
	return nil
}

// ParallelAction executes multiple actions in parallel
type ParallelAction struct {
	name    string
	actions []Action
}

// NewParallelAction creates a new parallel action
func NewParallelAction(name string, actions ...Action) *ParallelAction {
	return &ParallelAction{
		name:    name,
		actions: actions,
	}
}

func (a *ParallelAction) Name() string {
	return a.name
}

func (a *ParallelAction) Type() string {
	return "parallel"
}

func (a *ParallelAction) Execute(ctx context.Context, event *Event) error {
	type result struct {
		action Action
		err    error
	}

	results := make(chan result, len(a.actions))

	// Execute all actions in parallel
	for _, action := range a.actions {
		go func(act Action) {
			err := act.Execute(ctx, event)
			results <- result{action: act, err: err}
		}(action)
	}

	// Collect results
	var errors []error
	for i := 0; i < len(a.actions); i++ {
		res := <-results
		if res.err != nil {
			errors = append(errors, fmt.Errorf("%s: %w", res.action.Name(), res.err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("parallel action had %d failures: %v", len(errors), errors)
	}

	return nil
}

// DelayAction introduces a delay before executing the next action
type DelayAction struct {
	name     string
	duration time.Duration
}

// NewDelayAction creates a new delay action
func NewDelayAction(name string, duration time.Duration) *DelayAction {
	return &DelayAction{
		name:     name,
		duration: duration,
	}
}

func (a *DelayAction) Name() string {
	return a.name
}

func (a *DelayAction) Type() string {
	return "delay"
}

func (a *DelayAction) Execute(ctx context.Context, event *Event) error {
	select {
	case <-time.After(a.duration):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// RetryAction retries an action with exponential backoff
type RetryAction struct {
	name           string
	action         Action
	maxRetries     int
	initialBackoff time.Duration
	maxBackoff     time.Duration
	multiplier     float64
}

// NewRetryAction creates a new retry action
func NewRetryAction(name string, action Action, maxRetries int) *RetryAction {
	return &RetryAction{
		name:           name,
		action:         action,
		maxRetries:     maxRetries,
		initialBackoff: 1 * time.Second,
		maxBackoff:     30 * time.Second,
		multiplier:     2.0,
	}
}

// SetBackoff configures backoff parameters
func (a *RetryAction) SetBackoff(initial, max time.Duration, multiplier float64) *RetryAction {
	a.initialBackoff = initial
	a.maxBackoff = max
	a.multiplier = multiplier
	return a
}

func (a *RetryAction) Name() string {
	return a.name
}

func (a *RetryAction) Type() string {
	return "retry"
}

func (a *RetryAction) Execute(ctx context.Context, event *Event) error {
	backoff := a.initialBackoff
	var lastErr error

	for attempt := 0; attempt <= a.maxRetries; attempt++ {
		err := a.action.Execute(ctx, event)
		if err == nil {
			return nil
		}

		lastErr = err

		// Don't sleep after last attempt
		if attempt < a.maxRetries {
			select {
			case <-time.After(backoff):
				// Calculate next backoff
				backoff = time.Duration(float64(backoff) * a.multiplier)
				if backoff > a.maxBackoff {
					backoff = a.maxBackoff
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	return fmt.Errorf("action failed after %d retries: %w", a.maxRetries, lastErr)
}
