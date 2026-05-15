package events

import "testing"

func TestCanonicalEventTypes_CountAndContents(t *testing.T) {
	t.Parallel()
	got := CanonicalEventTypes()
	if len(got) != 22 {
		t.Fatalf("len(CanonicalEventTypes()) = %d, want 22 (§4.9)", len(got))
	}

	// Each constant validates through ParseEventType.
	for _, typ := range got {
		if _, err := ParseEventType(string(typ)); err != nil {
			t.Errorf("ParseEventType(%q): %v", typ, err)
		}
	}

	// Category breakdown matches the §4.9 spec.
	byCategory := map[Category]int{}
	for _, typ := range got {
		byCategory[typ.Category()]++
	}
	want := map[Category]int{
		CategoryAgent:  5,
		CategoryJob:    4,
		CategoryState:  5,
		CategorySystem: 3,
		CategoryUser:   3,
		CategoryPolicy: 2,
	}
	for cat, n := range want {
		if byCategory[cat] != n {
			t.Errorf("category %s: got %d, want %d", cat, byCategory[cat], n)
		}
	}
}

func TestIsCanonical(t *testing.T) {
	t.Parallel()
	// Every constant is canonical.
	for _, typ := range CanonicalEventTypes() {
		if !IsCanonical(typ) {
			t.Errorf("IsCanonical(%s) = false, want true", typ)
		}
	}

	// Operator-defined types are NOT canonical but ARE valid through
	// ParseEventType — that's the whole point of the option C choice.
	custom := EventType("agent.custom_signal")
	if _, err := ParseEventType(string(custom)); err != nil {
		t.Errorf("ParseEventType(%q) rejected; should be permitted: %v", custom, err)
	}
	if IsCanonical(custom) {
		t.Errorf("IsCanonical(%s) = true, want false", custom)
	}

	if IsCanonical(EventType("")) {
		t.Errorf("IsCanonical(empty) = true, want false")
	}
}

func TestCanonicalEventTypes_FreshSliceEachCall(t *testing.T) {
	t.Parallel()
	got := CanonicalEventTypes()
	got[0] = EventType("mutated")
	again := CanonicalEventTypes()
	if again[0] != EventTypeAgentConnect {
		t.Errorf("CanonicalEventTypes() shares backing array; first element mutated")
	}
}

func TestCanonicalEventTypes_ExactSpellings(t *testing.T) {
	t.Parallel()
	// Lock in the spec spellings — particularly the heartbeat_failed
	// underscore and the state.apply.* multi-segment subtypes. A
	// future "rename" PR should have to update this test deliberately.
	expected := map[EventType]struct{}{
		"agent.connect": {}, "agent.disconnect": {}, "agent.heartbeat": {},
		"agent.heartbeat_failed": {}, "agent.error": {},
		"job.start": {}, "job.complete": {}, "job.fail": {}, "job.output": {},
		"state.apply.start": {}, "state.apply.done": {}, "state.apply.fail": {},
		"state.change": {}, "state.drift": {},
		"system.startup": {}, "system.shutdown": {}, "system.error": {},
		"user.login": {}, "user.command": {}, "user.error": {},
		"policy.pass": {}, "policy.violation": {},
	}
	got := CanonicalEventTypes()
	if len(expected) != len(got) {
		t.Fatalf("expected %d types, got %d", len(expected), len(got))
	}
	for _, typ := range got {
		if _, ok := expected[typ]; !ok {
			t.Errorf("unexpected canonical type: %s", typ)
		}
	}
}
