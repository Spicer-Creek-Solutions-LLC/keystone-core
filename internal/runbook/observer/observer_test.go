package observer

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/audit"
	"go.keystone-core.io/keystone-core/internal/events"
	"go.keystone-core.io/keystone-core/internal/runbook"
)

type fakePublisher struct {
	events []events.Event
	err    error
}

func (f *fakePublisher) Publish(_ context.Context, e events.Event) error {
	f.events = append(f.events, e)
	return f.err
}

type fakeAuditor struct {
	entries []audit.AuditEntry
}

func (f *fakeAuditor) Emit(_ context.Context, e audit.AuditEntry) {
	f.entries = append(f.entries, e)
}

func ev(step string, to runbook.Status) runbook.ObserverEvent {
	return runbook.ObserverEvent{
		ExecutionID: "exec-1", Runbook: "rb", Step: step, To: to,
		At: time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC), Note: "n",
	}
}

func TestEventObserver(t *testing.T) {
	fp := &fakePublisher{}
	o := NewEventObserver(fp, "")

	cases := []struct {
		step    string
		to      runbook.Status
		wantTyp string
		wantSev events.Severity
	}{
		{"", runbook.StatusRunning, "runbook.execute.start", events.SeverityInfo},
		{"", runbook.StatusSucceeded, "runbook.execute.done", events.SeverityInfo},
		{"", runbook.StatusFailed, "runbook.execute.fail", events.SeverityError},
		{"s1", runbook.StatusRunning, "runbook.step.start", events.SeverityInfo},
		{"s1", runbook.StatusSucceeded, "runbook.step.done", events.SeverityInfo},
		{"s1", runbook.StatusFailed, "runbook.step.fail", events.SeverityError},
		{"s1", runbook.StatusSkipped, "runbook.step.skip", events.SeverityInfo},
	}
	for _, c := range cases {
		fp.events = nil
		o.OnTransition(context.Background(), ev(c.step, c.to))
		if len(fp.events) != 1 {
			t.Fatalf("%s/%s: %d events", c.step, c.to, len(fp.events))
		}
		got := fp.events[0]
		if string(got.Type) != c.wantTyp || got.Severity != c.wantSev {
			t.Fatalf("type=%s sev=%v want %s/%v", got.Type, got.Severity, c.wantTyp, c.wantSev)
		}
		if got.CorrelationID != "exec-1" || got.Data["runbook"] != "rb" {
			t.Fatalf("event metadata wrong: %+v", got)
		}
		if c.step != "" && got.Data["step"] != "s1" {
			t.Fatalf("step missing in data: %+v", got.Data)
		}
	}
}

func TestEventObserver_NonEmittingAndNilSafe(t *testing.T) {
	fp := &fakePublisher{}
	o := NewEventObserver(fp, "node1")
	// pending→pending is not a classified transition.
	o.OnTransition(context.Background(), ev("", runbook.StatusPending))
	if len(fp.events) != 0 {
		t.Fatalf("unexpected emit: %v", fp.events)
	}
	// nil publisher / nil receiver must not panic.
	(&EventObserver{}).OnTransition(context.Background(), ev("", runbook.StatusRunning))
	var nilObs *EventObserver
	nilObs.OnTransition(context.Background(), ev("", runbook.StatusRunning))

	// Publish error is swallowed (best-effort).
	fp.err = errors.New("bus down")
	o.OnTransition(context.Background(), ev("", runbook.StatusRunning))
}

func TestAuditObserver(t *testing.T) {
	fa := &fakeAuditor{}
	o := NewAuditObserver(fa)

	// Terminal outcomes audit.
	o.OnTransition(context.Background(), ev("", runbook.StatusSucceeded))
	o.OnTransition(context.Background(), ev("s1", runbook.StatusFailed))
	if len(fa.entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(fa.entries))
	}
	if fa.entries[0].Action != "runbook.execute" || !fa.entries[0].Allowed {
		t.Fatalf("exec entry wrong: %+v", fa.entries[0])
	}
	if fa.entries[1].Action != "runbook.step" || fa.entries[1].Allowed {
		t.Fatalf("step entry wrong: %+v", fa.entries[1])
	}
	if fa.entries[1].Severity != audit.SeverityHigh {
		t.Fatalf("failed step severity = %v want High", fa.entries[1].Severity)
	}
	if fa.entries[1].Metadata["step"] != "s1" || fa.entries[1].Metadata["status"] != "failed" {
		t.Fatalf("metadata wrong: %+v", fa.entries[1].Metadata)
	}
}

func TestAuditObserver_SkipsNonTerminal(t *testing.T) {
	fa := &fakeAuditor{}
	o := NewAuditObserver(fa)
	for _, to := range []runbook.Status{runbook.StatusRunning, runbook.StatusSkipped, runbook.StatusPending} {
		o.OnTransition(context.Background(), ev("s1", to))
	}
	if len(fa.entries) != 0 {
		t.Fatalf("non-terminal transitions audited: %d", len(fa.entries))
	}
	// nil-safe + default noop auditor.
	(&AuditObserver{}).OnTransition(context.Background(), ev("", runbook.StatusFailed))
	if NewAuditObserver(nil).Auditor == nil {
		t.Fatal("nil auditor should default to NoopAuditor")
	}
}

// Both adapters satisfy runbook.Observer and compose via MultiObserver.
func TestComposeWithMultiObserver(t *testing.T) {
	fp := &fakePublisher{}
	fa := &fakeAuditor{}
	m := runbook.MultiObserver{NewEventObserver(fp, "n"), NewAuditObserver(fa)}
	m.OnTransition(context.Background(), ev("", runbook.StatusSucceeded))
	if len(fp.events) != 1 || len(fa.entries) != 1 {
		t.Fatalf("compose failed: events=%d entries=%d", len(fp.events), len(fa.entries))
	}
}
