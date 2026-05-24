// SPDX-License-Identifier: Apache-2.0

package observer

import (
	"context"

	"go.keystone-core.io/keystone-core/internal/events"
	"go.keystone-core.io/keystone-core/internal/runbook"
)

// Publisher is the subset of events.EventPublisher this package
// needs. events.EventPublisher satisfies it.
type Publisher interface {
	Publish(ctx context.Context, e events.Event) error
}

// EventObserver publishes a runbook-category event for every engine
// transition. Source is the event Source field (e.g. the server
// node name). Best-effort: a publish error is dropped.
type EventObserver struct {
	Publisher Publisher
	Source    string
}

// NewEventObserver returns an EventObserver. source defaults to
// "runbook" when empty.
func NewEventObserver(p Publisher, source string) *EventObserver {
	if source == "" {
		source = "runbook"
	}
	return &EventObserver{Publisher: p, Source: source}
}

// OnTransition implements [runbook.Observer].
func (o *EventObserver) OnTransition(ctx context.Context, ev runbook.ObserverEvent) {
	if o == nil || o.Publisher == nil {
		return
	}
	typ, sev, ok := classify(ev)
	if !ok {
		return // non-emitting transition (e.g. pending→running step start handled below)
	}

	e, err := events.NewEvent(typ, o.Source)
	if err != nil {
		return // unknown event type — should not happen with the fixed set
	}
	e.Time = ev.At
	e.Severity = sev
	e.CorrelationID = ev.ExecutionID
	data := map[string]any{
		"execution_id": ev.ExecutionID,
		"runbook":      ev.Runbook,
		"from":         string(ev.From),
		"to":           string(ev.To),
	}
	if ev.Step != "" {
		data["step"] = ev.Step
	}
	if ev.Note != "" {
		data["note"] = ev.Note
	}
	e.Data = data
	_ = o.Publisher.Publish(ctx, e)
}

// classify maps a transition onto an event type + severity. The
// boolean is false for transitions that do not emit.
func classify(ev runbook.ObserverEvent) (events.EventType, events.Severity, bool) {
	if ev.Step == "" {
		switch ev.To {
		case runbook.StatusRunning:
			return events.EventTypeRunbookExecuteStart, events.SeverityInfo, true
		case runbook.StatusSucceeded:
			return events.EventTypeRunbookExecuteDone, events.SeverityInfo, true
		case runbook.StatusFailed:
			return events.EventTypeRunbookExecuteFail, events.SeverityError, true
		}
		return "", 0, false
	}
	switch ev.To {
	case runbook.StatusRunning:
		return events.EventTypeRunbookStepStart, events.SeverityInfo, true
	case runbook.StatusSucceeded:
		return events.EventTypeRunbookStepDone, events.SeverityInfo, true
	case runbook.StatusFailed:
		return events.EventTypeRunbookStepFail, events.SeverityError, true
	case runbook.StatusSkipped:
		return events.EventTypeRunbookStepSkip, events.SeverityInfo, true
	}
	return "", 0, false
}
