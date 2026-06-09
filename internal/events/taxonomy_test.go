// SPDX-License-Identifier: Apache-2.0

package events

import "testing"

func TestCanonicalEventTypes_CountAndContents(t *testing.T) {
	t.Parallel()
	got := CanonicalEventTypes()
	if len(got) != 30 {
		t.Fatalf("len(CanonicalEventTypes()) = %d, want 30 (§4.9's 29 + system.rebooted)", len(got))
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
		CategoryAgent:   5,
		CategoryJob:     4,
		CategoryState:   5,
		CategorySystem:  4,
		CategoryUser:    3,
		CategoryPolicy:  2,
		CategoryRunbook: 7,
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

func TestEventTypesForCategory_KnownCategories(t *testing.T) {
	t.Parallel()
	cases := []struct {
		cat  Category
		want []EventType
	}{
		{CategoryAgent, []EventType{
			EventTypeAgentConnect,
			EventTypeAgentDisconnect,
			EventTypeAgentHeartbeat,
			EventTypeAgentHeartbeatFailed,
			EventTypeAgentError,
		}},
		{CategoryJob, []EventType{
			EventTypeJobStart,
			EventTypeJobComplete,
			EventTypeJobFail,
			EventTypeJobOutput,
		}},
		{CategoryState, []EventType{
			EventTypeStateApplyStart,
			EventTypeStateApplyDone,
			EventTypeStateApplyFail,
			EventTypeStateChange,
			EventTypeStateDrift,
		}},
		{CategorySystem, []EventType{
			EventTypeSystemStartup,
			EventTypeSystemShutdown,
			EventTypeSystemError,
			EventTypeSystemRebooted,
		}},
		{CategoryUser, []EventType{
			EventTypeUserLogin,
			EventTypeUserCommand,
			EventTypeUserError,
		}},
		{CategoryPolicy, []EventType{
			EventTypePolicyPass,
			EventTypePolicyViolation,
		}},
	}
	for _, c := range cases {
		t.Run(string(c.cat), func(t *testing.T) {
			t.Parallel()
			got := EventTypesForCategory(c.cat)
			if len(got) != len(c.want) {
				t.Fatalf("len = %d, want %d (got %v)", len(got), len(c.want), got)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("[%d] = %s, want %s", i, got[i], c.want[i])
				}
			}
		})
	}
}

func TestEventTypesForCategory_UnknownCategory(t *testing.T) {
	t.Parallel()
	cases := []Category{"", "audit", "metric", "Agent", " agent"}
	for _, c := range cases {
		if got := EventTypesForCategory(c); got != nil {
			t.Errorf("EventTypesForCategory(%q) = %v, want nil", c, got)
		}
	}
}

func TestEventTypesForCategory_FreshSliceEachCall(t *testing.T) {
	t.Parallel()
	first := EventTypesForCategory(CategoryAgent)
	first[0] = EventType("mutated")
	again := EventTypesForCategory(CategoryAgent)
	if again[0] != EventTypeAgentConnect {
		t.Errorf("returned slice aliased — first call's mutation leaked: %s", again[0])
	}
}

func TestCountForCategory(t *testing.T) {
	t.Parallel()
	cases := []struct {
		cat  Category
		want int
	}{
		{CategoryAgent, 5},
		{CategoryJob, 4},
		{CategoryState, 5},
		{CategorySystem, 4},
		{CategoryUser, 3},
		{CategoryPolicy, 2},
		{Category("bogus"), 0},
		{Category(""), 0},
	}
	for _, c := range cases {
		got := CountForCategory(c.cat)
		want := len(EventTypesForCategory(c.cat))
		if got != c.want {
			t.Errorf("CountForCategory(%s) = %d, want %d", c.cat, got, c.want)
		}
		if got != want {
			t.Errorf("CountForCategory(%s) = %d but len(EventTypesForCategory) = %d (must match)", c.cat, got, want)
		}
	}
}

func TestAllCategoriesWithCounts(t *testing.T) {
	t.Parallel()
	got := AllCategoriesWithCounts()
	want := map[Category]int{
		CategoryAgent:   5,
		CategoryJob:     4,
		CategoryState:   5,
		CategorySystem:  4,
		CategoryUser:    3,
		CategoryPolicy:  2,
		CategoryRunbook: 7,
		CategoryGitops:  0, // known category, provider-driven subtypes (Epic 16 task 4)
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (got %v)", len(got), len(want), got)
	}
	total := 0
	for cat, n := range want {
		if got[cat] != n {
			t.Errorf("%s = %d, want %d", cat, got[cat], n)
		}
		total += n
	}
	if total != 30 {
		t.Errorf("sum = %d, want 30 (§4.9's 29 + system.rebooted)", total)
	}

	// Returned map is fresh per call — mutating the result must
	// not affect the source.
	got[CategoryAgent] = 99
	again := AllCategoriesWithCounts()
	if again[CategoryAgent] != 5 {
		t.Errorf("returned map aliased — mutation leaked: %d", again[CategoryAgent])
	}
}

func TestEventTypesForCategory_PartitionsCanonical(t *testing.T) {
	t.Parallel()
	// Every canonical type must appear in EXACTLY one category's
	// per-category slice; the union must equal CanonicalEventTypes.
	seen := make(map[EventType]Category, 30)
	for _, c := range KnownCategories() {
		for _, typ := range EventTypesForCategory(c) {
			if prev, ok := seen[typ]; ok {
				t.Errorf("%s seen in both %s and %s", typ, prev, c)
			}
			seen[typ] = c
		}
	}
	if len(seen) != 30 {
		t.Errorf("partition size = %d, want 30", len(seen))
	}
	for _, typ := range CanonicalEventTypes() {
		if _, ok := seen[typ]; !ok {
			t.Errorf("canonical %s not in any category partition", typ)
		}
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
		"system.startup": {}, "system.shutdown": {}, "system.error": {}, "system.rebooted": {},
		"user.login": {}, "user.command": {}, "user.error": {},
		"policy.pass": {}, "policy.violation": {},
		"runbook.execute.start": {}, "runbook.execute.done": {}, "runbook.execute.fail": {},
		"runbook.step.start": {}, "runbook.step.done": {}, "runbook.step.fail": {},
		"runbook.step.skip": {},
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
