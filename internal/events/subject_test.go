package events

import (
	"errors"
	"testing"
)

func TestSubjectFor_Valid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		cluster string
		typ     EventType
		want    string
	}{
		{"default", EventTypeAgentConnect, "kscore.default.events.agent.connect"},
		{"prod-east", EventTypeJobStart, "kscore.prod-east.events.job.start"},
		// Multi-segment subtype flows through verbatim.
		{"default", EventTypeStateApplyStart, "kscore.default.events.state.apply.start"},
		{"c_1", EventTypePolicyViolation, "kscore.c_1.events.policy.violation"},
		{"AlphaCluster", EventTypeSystemStartup, "kscore.AlphaCluster.events.system.startup"},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			t.Parallel()
			got, err := SubjectFor(c.cluster, c.typ)
			if err != nil {
				t.Fatalf("SubjectFor(%q, %s): %v", c.cluster, c.typ, err)
			}
			if got != c.want {
				t.Errorf("subj = %q, want %q", got, c.want)
			}
		})
	}
}

func TestSubjectFor_InvalidCluster(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		cluster string
	}{
		{"empty", ""},
		{"has dot", "prod.east"},
		{"has space", "prod east"},
		{"nats wildcard star", "prod*"},
		{"nats wildcard gt", "prod>"},
		{"tab character", "prod\teast"},
		{"slash", "prod/east"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := SubjectFor(c.cluster, EventTypeAgentConnect)
			if err == nil {
				t.Fatalf("SubjectFor(%q) succeeded; want error", c.cluster)
			}
			if !errors.Is(err, ErrInvalidEvent) {
				t.Errorf("err = %v; want errors.Is(ErrInvalidEvent)", err)
			}
		})
	}
}

func TestSubjectFor_InvalidEventType(t *testing.T) {
	t.Parallel()
	cases := []EventType{"", "nodot", ".leading", "trailing.", "Audit.event"}
	for _, typ := range cases {
		_, err := SubjectFor("default", typ)
		if err == nil {
			t.Errorf("SubjectFor(default, %q) succeeded; want error", typ)
			continue
		}
		if !errors.Is(err, ErrInvalidEvent) {
			t.Errorf("err = %v; want errors.Is(ErrInvalidEvent)", err)
		}
	}
}
