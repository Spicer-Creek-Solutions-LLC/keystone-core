// SPDX-License-Identifier: Apache-2.0

package events

import (
	"errors"
	"testing"
	"time"
)

// helper: compile or fatal.
func mustCompile(t *testing.T, expr string) *Filter {
	t.Helper()
	f, err := CompileFilter(expr)
	if err != nil {
		t.Fatalf("CompileFilter(%q): %v", expr, err)
	}
	return f
}

// helper: a fully-populated event so every variable is non-zero.
func sampleEvent(t *testing.T) Event {
	t.Helper()
	e, err := NewEvent(EventTypeAgentConnect, "agent-1")
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	e.Severity = SeverityWarn
	e.CorrelationID = "req-42"
	e.Subject = "kscore.default.events.agent.connect"
	e.Tags = map[string]string{"role": "web", "env": "prod"}
	e.Data = map[string]any{
		"latency_ms": float64(125),
		"host":       "h-1",
		"nested": map[string]any{
			"deep_key": "deep_value",
		},
	}
	// Fix Time to a known instant so timestamp comparisons are
	// deterministic.
	e.Time = time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	return e
}

// ---- Compile errors ---------------------------------------------------------

func TestCompileFilter_Empty(t *testing.T) {
	t.Parallel()
	cases := []string{"", "   ", "\t", "\n"}
	for _, in := range cases {
		_, err := CompileFilter(in)
		if err == nil {
			t.Fatalf("CompileFilter(%q) succeeded; want error", in)
		}
		if !errors.Is(err, ErrInvalidFilter) {
			t.Errorf("CompileFilter(%q) err = %v; want errors.Is(ErrInvalidFilter)", in, err)
		}
	}
}

func TestCompileFilter_Malformed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		expr string
	}{
		{"unmatched paren", "(type == 'x'"},
		{"missing operator", "type 'x'"},
		{"unknown variable", "bogus == 'x'"},
		{"unknown function", "type.bogus_method('x')"},
		{"type mismatch", "type == 42"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := CompileFilter(c.expr)
			if err == nil {
				t.Fatalf("CompileFilter(%q) succeeded; want error", c.expr)
			}
			if !errors.Is(err, ErrInvalidFilter) {
				t.Errorf("err = %v; want errors.Is(ErrInvalidFilter)", err)
			}
		})
	}
}

func TestCompileFilter_NonBoolResult(t *testing.T) {
	t.Parallel()
	// Expression that compiles but returns a non-bool value.
	_, err := CompileFilter("type")
	if err == nil {
		t.Fatalf("non-bool expression compiled; want error")
	}
	if !errors.Is(err, ErrInvalidFilter) {
		t.Errorf("err = %v; want ErrInvalidFilter", err)
	}
}

// ---- Match by variable ------------------------------------------------------

func TestFilterMatch_Type(t *testing.T) {
	t.Parallel()
	f := mustCompile(t, "type == 'agent.connect'")
	e := sampleEvent(t)
	if !f.Match(e) {
		t.Errorf("expected match on type")
	}
	e.Type = EventTypeJobStart
	if f.Match(e) {
		t.Errorf("matched on wrong type")
	}
}

func TestFilterMatch_Source(t *testing.T) {
	t.Parallel()
	f := mustCompile(t, "source == 'agent-1'")
	if !f.Match(sampleEvent(t)) {
		t.Errorf("expected match")
	}
}

func TestFilterMatch_CorrelationID(t *testing.T) {
	t.Parallel()
	f := mustCompile(t, "correlation_id == 'req-42'")
	if !f.Match(sampleEvent(t)) {
		t.Errorf("expected match")
	}
}

func TestFilterMatch_Subject(t *testing.T) {
	t.Parallel()
	f := mustCompile(t, "subject == 'kscore.default.events.agent.connect'")
	if !f.Match(sampleEvent(t)) {
		t.Errorf("expected match")
	}
}

func TestFilterMatch_TagsExact(t *testing.T) {
	t.Parallel()
	f := mustCompile(t, "tags.role == 'web'")
	if !f.Match(sampleEvent(t)) {
		t.Errorf("expected match on tag")
	}

	f = mustCompile(t, "tags.role == 'db'")
	if f.Match(sampleEvent(t)) {
		t.Errorf("matched on wrong tag value")
	}
}

func TestFilterMatch_DataExact(t *testing.T) {
	t.Parallel()
	f := mustCompile(t, "data.host == 'h-1'")
	if !f.Match(sampleEvent(t)) {
		t.Errorf("expected match")
	}
}

func TestFilterMatch_DataNested(t *testing.T) {
	t.Parallel()
	f := mustCompile(t, "data.nested.deep_key == 'deep_value'")
	if !f.Match(sampleEvent(t)) {
		t.Errorf("expected match on nested data")
	}
}

// ---- Built-in matchers ------------------------------------------------------

func TestFilterMatch_Matches_Regex(t *testing.T) {
	t.Parallel()
	f := mustCompile(t, "type.matches('agent\\\\..*')")
	e := sampleEvent(t)
	if !f.Match(e) {
		t.Errorf("expected regex match on agent.connect")
	}
	e.Type = EventTypeJobStart
	if f.Match(e) {
		t.Errorf("regex incorrectly matched job.start")
	}
}

func TestFilterMatch_Contains(t *testing.T) {
	t.Parallel()
	f := mustCompile(t, "subject.contains('events.agent')")
	if !f.Match(sampleEvent(t)) {
		t.Errorf("expected contains match")
	}
}

func TestFilterMatch_StartsWith(t *testing.T) {
	t.Parallel()
	f := mustCompile(t, "type.startsWith('agent.')")
	if !f.Match(sampleEvent(t)) {
		t.Errorf("expected startsWith match")
	}
}

// ---- Boolean composition ----------------------------------------------------

func TestFilterMatch_BooleanAnd(t *testing.T) {
	t.Parallel()
	f := mustCompile(t, "tags.role == 'web' && tags.env == 'prod'")
	if !f.Match(sampleEvent(t)) {
		t.Errorf("expected && match")
	}

	// Flip one side: should not match.
	e := sampleEvent(t)
	e.Tags["env"] = "staging"
	if f.Match(e) {
		t.Errorf("&& matched with staging env")
	}
}

func TestFilterMatch_BooleanOr(t *testing.T) {
	t.Parallel()
	f := mustCompile(t, "type == 'agent.connect' || type == 'agent.disconnect'")
	e := sampleEvent(t)
	if !f.Match(e) {
		t.Errorf("expected || match on connect")
	}
	e.Type = EventTypeAgentDisconnect
	if !f.Match(e) {
		t.Errorf("expected || match on disconnect")
	}
	e.Type = EventTypeJobStart
	if f.Match(e) {
		t.Errorf("|| matched job.start")
	}
}

func TestFilterMatch_BooleanNot(t *testing.T) {
	t.Parallel()
	f := mustCompile(t, "!(type == 'job.start')")
	e := sampleEvent(t)
	if !f.Match(e) {
		t.Errorf("expected !job.start to match agent.connect")
	}
	e.Type = EventTypeJobStart
	if f.Match(e) {
		t.Errorf("!job.start matched job.start")
	}
}

// ---- Time -------------------------------------------------------------------

func TestFilterMatch_TimeBefore(t *testing.T) {
	t.Parallel()
	// 2026-05-15 12:00 UTC < 2026-05-16
	f := mustCompile(t, "time < timestamp('2026-05-16T00:00:00Z')")
	if !f.Match(sampleEvent(t)) {
		t.Errorf("expected time before 2026-05-16")
	}
}

func TestFilterMatch_TimeAfter(t *testing.T) {
	t.Parallel()
	f := mustCompile(t, "time > timestamp('2026-05-14T00:00:00Z')")
	if !f.Match(sampleEvent(t)) {
		t.Errorf("expected time after 2026-05-14")
	}
}

// ---- Severity ordinal threshold ---------------------------------------------

func TestFilterMatch_SeverityAtLeast(t *testing.T) {
	t.Parallel()
	f := mustCompile(t, "severity.at_least('warn')")
	cases := []struct {
		sev   Severity
		match bool
	}{
		{SeverityDebug, false},
		{SeverityInfo, false},
		{SeverityWarn, true},
		{SeverityError, true},
		{SeverityCritical, true},
	}
	for _, c := range cases {
		e := sampleEvent(t)
		e.Severity = c.sev
		got := f.Match(e)
		if got != c.match {
			t.Errorf("severity=%s: got %v, want %v", c.sev, got, c.match)
		}
	}
}

func TestFilterMatch_SeverityAtLeast_UnknownThreshold(t *testing.T) {
	t.Parallel()
	// Bad threshold name → at_least returns a CEL error → Match
	// returns false (excludes), with a slog.Warn (not asserted here
	// — slog is hard to capture without dependency injection; the
	// dispatcher-safety property is what matters).
	f := mustCompile(t, "severity.at_least('bogus')")
	if f.Match(sampleEvent(t)) {
		t.Errorf("unknown threshold matched; want false (exclude)")
	}
}

// ---- Combined idiom from §4.9 -----------------------------------------------

func TestFilterMatch_Section_4_9_Idiom(t *testing.T) {
	t.Parallel()
	// The CLI acceptance line uses `severity >= 'warn'` but that
	// lex-compares (wrong); the idiomatic CEL form is at_least.
	f := mustCompile(t, "tags.role == 'web' && severity.at_least('warn')")
	if !f.Match(sampleEvent(t)) {
		t.Errorf("§4.9 idiom did not match")
	}

	// Drop severity below threshold: should not match.
	e := sampleEvent(t)
	e.Severity = SeverityInfo
	if f.Match(e) {
		t.Errorf("info-severity event matched warn-threshold filter")
	}
}

// ---- Missing-key safety -----------------------------------------------------

func TestFilterMatch_MissingTagKey_ExcludesGracefully(t *testing.T) {
	t.Parallel()
	f := mustCompile(t, "tags.role == 'web'")
	e := sampleEvent(t)
	delete(e.Tags, "role")
	if f.Match(e) {
		t.Errorf("missing-key matched; want excluded")
	}
}

func TestFilterMatch_NilMaps_ExcludesGracefully(t *testing.T) {
	t.Parallel()
	f := mustCompile(t, "tags.role == 'web'")
	e := sampleEvent(t)
	e.Tags = nil
	if f.Match(e) {
		t.Errorf("nil-tags event matched on tag predicate; want excluded")
	}

	f = mustCompile(t, "data.host == 'h-1'")
	e = sampleEvent(t)
	e.Data = nil
	if f.Match(e) {
		t.Errorf("nil-data event matched on data predicate; want excluded")
	}
}

// ---- API integration --------------------------------------------------------

func TestFilter_Match_IsCompatibleWithWithFilter(t *testing.T) {
	t.Parallel()
	f := mustCompile(t, "type == 'agent.connect'")
	// The compile-time assertion is in WithFilter accepting f.Match;
	// passing it without a type adapter proves filter.Match has the
	// `func(Event) bool` shape WithFilter expects.
	_ = WithFilter(f.Match)
	if !f.Match(sampleEvent(t)) {
		t.Errorf("Match returned false on matching event")
	}
}

func TestFilter_Expression_RoundTrip(t *testing.T) {
	t.Parallel()
	const expr = "tags.role == 'web' && severity.at_least('warn')"
	f := mustCompile(t, expr)
	if got := f.Expression(); got != expr {
		t.Errorf("Expression() = %q, want %q", got, expr)
	}
}

// ---- has() existence operator -----------------------------------------------

func TestFilterMatch_HasTagKey(t *testing.T) {
	t.Parallel()
	// CEL `has()` is the idiomatic way to check map-key existence
	// before access. Useful guardrail for operator-written
	// expressions that should match only when a tag is present.
	f := mustCompile(t, "has(tags.role) && tags.role == 'web'")
	if !f.Match(sampleEvent(t)) {
		t.Errorf("expected has+match")
	}
	e := sampleEvent(t)
	delete(e.Tags, "role")
	if f.Match(e) {
		t.Errorf("has() did not short-circuit on missing key")
	}
}
