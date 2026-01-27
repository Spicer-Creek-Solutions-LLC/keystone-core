package trigger

import (
	"context"
	"fmt"
	"time"

	"github.com/shawnbutts/keystone-core/internal/events"
	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// RunbookAction implements events.Action to execute a runbook.
type RunbookAction struct {
	name       string
	runbookRef RunbookRef
	repository RunbookRepository
	executor   RunbookExecutor
	inputs     map[string]string // template -> value mappings
	timeout    time.Duration
}

// RunbookActionConfig configures a RunbookAction.
type RunbookActionConfig struct {
	// Name is the action name.
	Name string `yaml:"name" json:"name"`

	// Runbook is the runbook reference.
	Runbook RunbookRef `yaml:"runbook" json:"runbook"`

	// Inputs maps runbook input names to template expressions.
	Inputs map[string]string `yaml:"inputs,omitempty" json:"inputs,omitempty"`

	// Timeout is the maximum execution time.
	Timeout time.Duration `yaml:"timeout,omitempty" json:"timeout,omitempty"`
}

// NewRunbookAction creates a new runbook action.
func NewRunbookAction(config *RunbookActionConfig, repo RunbookRepository, executor RunbookExecutor) *RunbookAction {
	return &RunbookAction{
		name:       config.Name,
		runbookRef: config.Runbook,
		repository: repo,
		executor:   executor,
		inputs:     config.Inputs,
		timeout:    config.Timeout,
	}
}

// Name returns the action name.
func (a *RunbookAction) Name() string {
	return a.name
}

// Type returns the action type.
func (a *RunbookAction) Type() string {
	return "runbook"
}

// Execute runs the runbook with event data as inputs.
func (a *RunbookAction) Execute(ctx context.Context, event *events.Event) error {
	// Apply timeout if configured
	if a.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, a.timeout)
		defer cancel()
	}

	// Get runbook
	rb, err := a.repository.GetRunbook(a.runbookRef.Name, a.runbookRef.Version)
	if err != nil {
		return fmt.Errorf("get runbook %s: %w", a.runbookRef.Name, err)
	}

	// Build inputs
	inputs := a.buildInputs(event)

	// Execute runbook
	exec, err := a.executor.Execute(rb, inputs)
	if err != nil {
		return fmt.Errorf("execute runbook: %w", err)
	}

	// Check result
	if exec.State == runbook.ExecutionStateFailed {
		return fmt.Errorf("runbook execution failed: %s", exec.Error)
	}

	return nil
}

// buildInputs builds runbook inputs from event data.
func (a *RunbookAction) buildInputs(event *events.Event) map[string]interface{} {
	inputs := make(map[string]interface{})

	// Add default event data
	inputs["__event_id"] = event.ID
	inputs["__event_type"] = string(event.Type)
	inputs["__event_source"] = event.Source
	inputs["__event_time"] = event.Time
	inputs["__event_severity"] = string(event.Severity)

	// Copy event tags with prefix
	for k, v := range event.Tags {
		inputs["event_tag_"+k] = v
	}

	// Copy event data with prefix
	for k, v := range event.Data {
		inputs["event_"+k] = v
	}

	// Apply explicit input mappings
	for inputName, template := range a.inputs {
		value := resolveEventTemplate(template, event)
		inputs[inputName] = value
	}

	return inputs
}

// resolveEventTemplate resolves a template against event data.
func resolveEventTemplate(template string, event *events.Event) interface{} {
	// Handle simple field access (support both {{ .Field }} and {{.Field}})
	switch template {
	case "{{ .ID }}", "{{.ID}}":
		return event.ID
	case "{{ .Type }}", "{{.Type}}":
		return string(event.Type)
	case "{{ .Source }}", "{{.Source}}":
		return event.Source
	case "{{ .Severity }}", "{{.Severity}}":
		return string(event.Severity)
	case "{{ .CorrelationID }}", "{{.CorrelationID}}":
		return event.CorrelationID
	case "{{ .Time }}", "{{.Time}}":
		return event.Time
	}

	// Check for tag access: {{ .Tags.name }} or {{.Tags.name}}
	if tagName := extractTemplateField(template, ".Tags."); tagName != "" {
		if v, ok := event.Tags[tagName]; ok {
			return v
		}
		return ""
	}

	// Check for data access: {{ .Data.name }} or {{.Data.name}}
	if dataKey := extractTemplateField(template, ".Data."); dataKey != "" {
		if v, ok := event.Data[dataKey]; ok {
			return v
		}
		return nil
	}

	// Return as-is if not a recognized template
	return template
}

// extractTemplateField extracts a field name from a template like {{ .Tags.host }} or {{.Tags.host}}
func extractTemplateField(template, prefix string) string {
	// Try {{ .prefix.name }} format
	prefixWithSpace := "{{ " + prefix
	if len(template) > len(prefixWithSpace)+3 && template[:len(prefixWithSpace)] == prefixWithSpace {
		end := template[len(prefixWithSpace):]
		if len(end) > 3 && end[len(end)-3:] == " }}" {
			return end[:len(end)-3]
		}
	}

	// Try {{.prefix.name}} format (no spaces)
	prefixNoSpace := "{{" + prefix
	if len(template) > len(prefixNoSpace)+2 && template[:len(prefixNoSpace)] == prefixNoSpace {
		end := template[len(prefixNoSpace):]
		if len(end) > 2 && end[len(end)-2:] == "}}" {
			return end[:len(end)-2]
		}
	}

	return ""
}

// ReactorBuilder helps build reactor configurations for runbook triggers.
type ReactorBuilder struct {
	trigger    *Trigger
	repository RunbookRepository
	executor   RunbookExecutor
	parser     FilterParser
}

// NewReactorBuilder creates a reactor builder for a trigger.
func NewReactorBuilder(trigger *Trigger, repo RunbookRepository, executor RunbookExecutor, parser FilterParser) *ReactorBuilder {
	return &ReactorBuilder{
		trigger:    trigger,
		repository: repo,
		executor:   executor,
		parser:     parser,
	}
}

// Build creates a reactor configuration from the trigger.
func (b *ReactorBuilder) Build() (*events.Reactor, error) {
	// Parse filter expression
	filter, err := b.parser.Parse(b.trigger.Filter)
	if err != nil {
		return nil, fmt.Errorf("parse filter: %w", err)
	}

	// Create runbook action
	action := NewRunbookAction(&RunbookActionConfig{
		Name:    "execute-" + b.trigger.RunbookRef.Name,
		Runbook: b.trigger.RunbookRef,
		Inputs:  b.trigger.InputMappings,
	}, b.repository, b.executor)

	// Build reactor
	reactor := &events.Reactor{
		ID:          b.trigger.ID,
		Name:        b.trigger.Name,
		Description: b.trigger.Description,
		Filter:      filter,
		Actions:     []events.Action{action},
		Enabled:     b.trigger.Enabled,
		Priority:    b.trigger.Priority,
	}

	// Apply conditions
	if b.trigger.Conditions != nil {
		reactor.Conditions = &events.ReactorConditions{}

		if b.trigger.Conditions.Throttle > 0 {
			reactor.Conditions.Throttle = b.trigger.Conditions.Throttle
		}
		if b.trigger.Conditions.Debounce > 0 {
			reactor.Conditions.Debounce = b.trigger.Conditions.Debounce
		}
		if b.trigger.Conditions.MaxConcurrent > 0 {
			reactor.MaxConcurrent = b.trigger.Conditions.MaxConcurrent
		}
		if b.trigger.Conditions.RateLimit != nil {
			reactor.Conditions.MaxExecutions = b.trigger.Conditions.RateLimit.MaxExecutions
			reactor.Conditions.TimeWindow = b.trigger.Conditions.RateLimit.Window
		}

		// Parse OnlyIf condition
		if b.trigger.Conditions.OnlyIf != "" {
			onlyIfFilter, err := b.parser.Parse(b.trigger.Conditions.OnlyIf)
			if err != nil {
				return nil, fmt.Errorf("parse onlyIf condition: %w", err)
			}
			reactor.Conditions.OnlyIf = onlyIfFilter
		}

		// Parse Unless condition
		if b.trigger.Conditions.Unless != "" {
			unlessFilter, err := b.parser.Parse(b.trigger.Conditions.Unless)
			if err != nil {
				return nil, fmt.Errorf("parse unless condition: %w", err)
			}
			reactor.Conditions.Unless = unlessFilter
		}
	}

	return reactor, nil
}

// EventProcessor provides a convenient way to process events through triggers.
type EventProcessor struct {
	registry *Registry
}

// NewEventProcessor creates a new event processor.
func NewEventProcessor(registry *Registry) *EventProcessor {
	return &EventProcessor{registry: registry}
}

// HandleEvent implements events.EventHandler for use with event subscribers.
func (p *EventProcessor) HandleEvent(event *events.Event) error {
	return p.registry.ProcessEvent(context.Background(), event)
}

// HandleEventWithContext processes an event with a context.
func (p *EventProcessor) HandleEventWithContext(ctx context.Context, event *events.Event) error {
	return p.registry.ProcessEvent(ctx, event)
}
