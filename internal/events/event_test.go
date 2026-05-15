package events

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestParseEventType_Valid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		s        string
		wantCat  Category
		wantSub  string
	}{
		{"agent.connect", CategoryAgent, "connect"},
		{"agent.heartbeat_failed", CategoryAgent, "heartbeat_failed"},
		{"job.start", CategoryJob, "start"},
		// Multi-segment subtype (the state.apply.* family per §4.9).
		{"state.apply.start", CategoryState, "apply.start"},
		{"state.apply.done", CategoryState, "apply.done"},
		{"state.drift", CategoryState, "drift"},
		{"system.startup", CategorySystem, "startup"},
		{"user.login", CategoryUser, "login"},
		{"policy.violation", CategoryPolicy, "violation"},
		// Operator-supplied subtype within a known category — accepted.
		{"agent.custom_signal", CategoryAgent, "custom_signal"},
		{"job.queued_externally", CategoryJob, "queued_externally"},
	}
	for _, c := range cases {
		t.Run(c.s, func(t *testing.T) {
			t.Parallel()
			typ, err := ParseEventType(c.s)
			if err != nil {
				t.Fatalf("ParseEventType(%q): %v", c.s, err)
			}
			if got := typ.Category(); got != c.wantCat {
				t.Errorf("Category() = %s, want %s", got, c.wantCat)
			}
			if got := typ.Subtype(); got != c.wantSub {
				t.Errorf("Subtype() = %s, want %s", got, c.wantSub)
			}
			if got := typ.String(); got != c.s {
				t.Errorf("String() = %q, want %q", got, c.s)
			}
			if !typ.IsValid() {
				t.Errorf("IsValid() = false, want true")
			}
		})
	}
}

func TestParseEventType_Invalid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		s    string
	}{
		{"empty", ""},
		{"no dot", "agent"},
		{"leading dot", ".agent"},
		{"trailing dot", "agent."},
		{"unknown category", "audit.event"},
		{"capitalised category", "Agent.connect"},
		{"category with space", "ag ent.connect"},
		{"subtype with whitespace", "agent.con nect"},
		{"subtype with tab", "agent.connect\ttab"},
		{"only dot", "."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseEventType(c.s)
			if err == nil {
				t.Fatalf("ParseEventType(%q) succeeded; want error", c.s)
			}
			if !errors.Is(err, ErrInvalidEvent) {
				t.Errorf("err = %v; want errors.Is(ErrInvalidEvent)", err)
			}
		})
	}
}

func TestEventType_Accessors_OnMalformed(t *testing.T) {
	t.Parallel()
	// Accessors are cheap and don't re-validate — they return empty
	// for malformed inputs. Documented behaviour.
	bad := EventType("nodot")
	if got := bad.Category(); got != "" {
		t.Errorf("Category() on malformed = %q, want empty", got)
	}
	if got := bad.Subtype(); got != "" {
		t.Errorf("Subtype() on malformed = %q, want empty", got)
	}
	if bad.IsValid() {
		t.Errorf("IsValid() on malformed = true, want false")
	}

	trailing := EventType("agent.")
	if got := trailing.Subtype(); got != "" {
		t.Errorf("Subtype() on trailing-dot = %q, want empty", got)
	}
}

func TestNewEvent_Defaults(t *testing.T) {
	t.Parallel()
	before := time.Now().UTC()
	e, err := NewEvent(EventTypeAgentConnect, "agent-1")
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	after := time.Now().UTC()

	if e.ID == "" {
		t.Errorf("ID empty")
	}
	parsedID, err := uuid.Parse(e.ID)
	if err != nil {
		t.Errorf("ID %q is not a UUID: %v", e.ID, err)
	}
	if parsedID.Version() != 7 {
		t.Errorf("ID version = %d, want 7 (uuidv7)", parsedID.Version())
	}
	if e.Type != EventTypeAgentConnect {
		t.Errorf("Type = %s, want %s", e.Type, EventTypeAgentConnect)
	}
	if e.Source != "agent-1" {
		t.Errorf("Source = %q", e.Source)
	}
	if e.Time.Before(before) || e.Time.After(after) {
		t.Errorf("Time %v outside [%v, %v]", e.Time, before, after)
	}
	if e.Time.Location() != time.UTC {
		t.Errorf("Time location = %v, want UTC", e.Time.Location())
	}
	if e.Severity != SeverityInfo {
		t.Errorf("Severity = %s, want info", e.Severity)
	}
	if e.Subject != "" {
		t.Errorf("Subject = %q, want empty (publisher stamps)", e.Subject)
	}
	if e.Tags != nil {
		t.Errorf("Tags = %v, want nil", e.Tags)
	}
	if e.Data != nil {
		t.Errorf("Data = %v, want nil", e.Data)
	}
}

func TestNewEvent_UUIDv7_K_Sortable(t *testing.T) {
	t.Parallel()
	// Stamp 50 events back-to-back and confirm their IDs sort
	// lexicographically into time order. This is the load-bearing
	// property that drove the v7 pick over v4 — task 2's SQL index
	// and task 4's NATS replay both rely on it.
	const n = 50
	ids := make([]string, n)
	for i := 0; i < n; i++ {
		e, err := NewEvent(EventTypeAgentHeartbeat, "agent-1")
		if err != nil {
			t.Fatalf("NewEvent[%d]: %v", i, err)
		}
		ids[i] = e.ID
		// Hold long enough to cross the v7 millisecond boundary
		// for at least most iterations; google/uuid handles same-ms
		// monotonicity so within-ms order is also stable.
		time.Sleep(100 * time.Microsecond)
	}
	for i := 1; i < n; i++ {
		if ids[i-1] > ids[i] {
			t.Errorf("ids not k-sorted at %d: %q > %q", i, ids[i-1], ids[i])
		}
	}
}

func TestNewEvent_RejectsBadInput(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		typ    EventType
		source string
	}{
		{"bad type", EventType("bogus"), "agent-1"},
		{"empty type", EventType(""), "agent-1"},
		{"empty source", EventTypeAgentConnect, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewEvent(c.typ, c.source)
			if err == nil {
				t.Fatalf("NewEvent succeeded; want error")
			}
			if !errors.Is(err, ErrInvalidEvent) {
				t.Errorf("err = %v; want errors.Is(ErrInvalidEvent)", err)
			}
		})
	}
}

func TestMustNewEvent(t *testing.T) {
	t.Parallel()
	e := MustNewEvent(EventTypeAgentConnect, "agent-1")
	if e.ID == "" {
		t.Errorf("MustNewEvent returned zero ID")
	}

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("MustNewEvent did not panic on bad input")
		}
	}()
	_ = MustNewEvent(EventType("bogus"), "agent-1")
}

func TestEvent_IsZero(t *testing.T) {
	t.Parallel()
	var z Event
	if !z.IsZero() {
		t.Errorf("zero Event.IsZero() = false")
	}
	e := MustNewEvent(EventTypeAgentConnect, "agent-1")
	if e.IsZero() {
		t.Errorf("constructed Event.IsZero() = true")
	}
}

func TestEvent_Validate(t *testing.T) {
	t.Parallel()
	valid := MustNewEvent(EventTypeAgentConnect, "agent-1")
	if err := valid.Validate(); err != nil {
		t.Errorf("Validate(good): %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*Event)
	}{
		{"empty id", func(e *Event) { e.ID = "" }},
		{"empty source", func(e *Event) { e.Source = "" }},
		{"empty type", func(e *Event) { e.Type = "" }},
		{"bad type", func(e *Event) { e.Type = EventType("bogus") }},
		{"zero time", func(e *Event) { e.Time = time.Time{} }},
		{"unknown severity", func(e *Event) { e.Severity = SeverityUnknown }},
		{"out-of-range severity", func(e *Event) { e.Severity = Severity(99) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			e := MustNewEvent(EventTypeAgentConnect, "agent-1")
			c.mutate(&e)
			err := e.Validate()
			if err == nil {
				t.Fatalf("Validate succeeded; want error")
			}
			if !errors.Is(err, ErrInvalidEvent) {
				t.Errorf("err = %v; want errors.Is(ErrInvalidEvent)", err)
			}
		})
	}
}

func TestEvent_Clone(t *testing.T) {
	t.Parallel()
	orig := MustNewEvent(EventTypeAgentConnect, "agent-1")
	orig.Tags = map[string]string{"role": "web", "env": "prod"}
	orig.Data = map[string]any{
		"count": 7,
		"nested": map[string]any{
			"k": "v",
			"list": []any{1, "two", map[string]any{"deep": true}},
		},
	}

	dup := orig.Clone()

	// Mutate the clone and assert orig is untouched.
	dup.Tags["role"] = "db"
	dup.Data["count"] = 99
	dup.Data["nested"].(map[string]any)["k"] = "changed"
	dup.Data["nested"].(map[string]any)["list"].([]any)[2].(map[string]any)["deep"] = false

	if orig.Tags["role"] != "web" {
		t.Errorf("orig.Tags mutated: %q", orig.Tags["role"])
	}
	if orig.Data["count"] != 7 {
		t.Errorf("orig.Data[count] mutated: %v", orig.Data["count"])
	}
	if orig.Data["nested"].(map[string]any)["k"] != "v" {
		t.Errorf("orig.Data nested mutated")
	}
	if orig.Data["nested"].(map[string]any)["list"].([]any)[2].(map[string]any)["deep"] != true {
		t.Errorf("orig.Data deep nested mutated")
	}

	// Clone of a nil-map event leaves maps nil.
	zero := Event{}
	clone := zero.Clone()
	if clone.Tags != nil || clone.Data != nil {
		t.Errorf("Clone(zero).Tags=%v Data=%v; want nil/nil", clone.Tags, clone.Data)
	}
}

func TestEvent_StampSubject(t *testing.T) {
	t.Parallel()
	e := MustNewEvent(EventTypeAgentConnect, "agent-1")
	subj, err := e.StampSubject("prod-east")
	if err != nil {
		t.Fatalf("StampSubject: %v", err)
	}
	want := "kscore.prod-east.events.agent.connect"
	if subj != want {
		t.Errorf("subj = %q, want %q", subj, want)
	}
	if e.Subject != want {
		t.Errorf("e.Subject = %q, want %q", e.Subject, want)
	}

	// Bad cluster name fails and does not overwrite an existing
	// Subject.
	e.Subject = "prior-value"
	if _, err := e.StampSubject(""); err == nil {
		t.Fatalf("StampSubject(empty cluster) succeeded; want error")
	}
	if e.Subject != "prior-value" {
		t.Errorf("e.Subject overwritten on error: %q", e.Subject)
	}

	// Malformed event type also fails the stamp.
	e.Type = EventType("bogus")
	if _, err := e.StampSubject("prod-east"); err == nil {
		t.Errorf("StampSubject(bad type) succeeded; want error")
	}
}

func TestEvent_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	in := MustNewEvent(EventTypeAgentConnect, "agent-1")
	in.CorrelationID = "req-42"
	in.Tags = map[string]string{"role": "web"}
	in.Data = map[string]any{"latency_ms": 12.5}
	if _, err := in.StampSubject("default"); err != nil {
		t.Fatalf("StampSubject: %v", err)
	}

	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(b), `"severity":"info"`) {
		t.Errorf("severity not serialised as canonical name: %s", b)
	}
	var out Event
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.ID != in.ID || out.Type != in.Type || out.Source != in.Source ||
		out.Severity != in.Severity || out.CorrelationID != in.CorrelationID ||
		out.Subject != in.Subject {
		t.Errorf("round-trip mismatch:\n in=%+v\nout=%+v", in, out)
	}
	if out.Tags["role"] != "web" {
		t.Errorf("Tags lost in round trip: %+v", out.Tags)
	}
	if out.Data["latency_ms"] != 12.5 {
		t.Errorf("Data lost in round trip: %+v", out.Data)
	}
}
