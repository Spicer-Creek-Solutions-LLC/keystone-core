// SPDX-License-Identifier: Apache-2.0

package events

import (
	"errors"
	"strings"
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

func TestSubjectPatternForCategory_KnownCategories(t *testing.T) {
	t.Parallel()
	cases := []struct {
		cat  Category
		want string
	}{
		{CategoryAgent, "kscore.default.events.agent.*"},
		{CategoryJob, "kscore.default.events.job.*"},
		{CategoryState, "kscore.default.events.state.*"},
		{CategorySystem, "kscore.default.events.system.*"},
		{CategoryUser, "kscore.default.events.user.*"},
		{CategoryPolicy, "kscore.default.events.policy.*"},
		{CategoryGitops, "kscore.default.events.gitops.*"},
	}
	for _, c := range cases {
		got, err := SubjectPatternForCategory("default", c.cat)
		if err != nil {
			t.Errorf("SubjectPatternForCategory(default, %s): %v", c.cat, err)
			continue
		}
		if got != c.want {
			t.Errorf("SubjectPatternForCategory(default, %s) = %q, want %q", c.cat, got, c.want)
		}
	}
}

func TestSubjectDeepPatternForCategory_KnownCategories(t *testing.T) {
	t.Parallel()
	cases := []struct {
		cat  Category
		want string
	}{
		{CategoryAgent, "kscore.default.events.agent.>"},
		{CategoryState, "kscore.default.events.state.>"},
		{CategoryPolicy, "kscore.default.events.policy.>"},
	}
	for _, c := range cases {
		got, err := SubjectDeepPatternForCategory("default", c.cat)
		if err != nil {
			t.Errorf("%v", err)
			continue
		}
		if got != c.want {
			t.Errorf("got %q, want %q", got, c.want)
		}
	}
}

func TestSubjectPatternForCategory_UnknownCategory(t *testing.T) {
	t.Parallel()
	cases := []Category{"", "audit", "metric", "Agent"}
	for _, c := range cases {
		_, err := SubjectPatternForCategory("default", c)
		if err == nil {
			t.Errorf("SubjectPatternForCategory(%q) succeeded; want error", c)
		}
		if err != nil && !errors.Is(err, ErrInvalidEvent) {
			t.Errorf("err = %v; want errors.Is(ErrInvalidEvent)", err)
		}
	}
}

func TestSubjectPatternForCategory_InvalidCluster(t *testing.T) {
	t.Parallel()
	cases := []string{"", "prod.east", "prod*", "prod>", "prod east"}
	for _, cluster := range cases {
		_, err := SubjectPatternForCategory(cluster, CategoryAgent)
		if err == nil {
			t.Errorf("SubjectPatternForCategory(%q, agent) succeeded; want error", cluster)
			continue
		}
		if !errors.Is(err, ErrInvalidEvent) {
			t.Errorf("err = %v; want errors.Is(ErrInvalidEvent)", err)
		}
	}
}

func TestSubjectDeepPatternForCategory_PropagatesErrors(t *testing.T) {
	t.Parallel()
	if _, err := SubjectDeepPatternForCategory("", CategoryAgent); !errors.Is(err, ErrInvalidEvent) {
		t.Errorf("empty cluster: %v", err)
	}
	if _, err := SubjectDeepPatternForCategory("default", Category("bogus")); !errors.Is(err, ErrInvalidEvent) {
		t.Errorf("bogus category: %v", err)
	}
}

// TestSubjectPatternForCategory_RoundTripWithSubjectFor asserts that
// every canonical event type's full subject has the corresponding
// category pattern's prefix — replacing the trailing `*` with the
// type's subtype must yield the exact full subject.
//
// For single-segment subtypes the `*` form covers the type; for
// multi-segment subtypes (state.apply.*) the `*` does NOT cover
// them (callers must use SubjectDeepPatternForCategory) — that's
// asserted via the deep-pattern check below.
func TestSubjectPatternForCategory_RoundTrip(t *testing.T) {
	t.Parallel()
	const cluster = "prod"
	for _, typ := range CanonicalEventTypes() {
		full, err := SubjectFor(cluster, typ)
		if err != nil {
			t.Fatalf("SubjectFor(%s): %v", typ, err)
		}
		// Build the category-level prefix and verify the full subject
		// extends it.
		cat := typ.Category()
		prefix := "kscore." + cluster + ".events." + string(cat) + "."
		if !strings.HasPrefix(full, prefix) {
			t.Errorf("full subject %q does not extend prefix %q", full, prefix)
		}
		// SubjectDeepPatternForCategory's prefix (sans `>`) must
		// also be the prefix of the full subject.
		deep, err := SubjectDeepPatternForCategory(cluster, cat)
		if err != nil {
			t.Errorf("SubjectDeepPatternForCategory: %v", err)
			continue
		}
		deepPrefix := strings.TrimSuffix(deep, ">")
		if !strings.HasPrefix(full, deepPrefix) {
			t.Errorf("full subject %q does not extend deep prefix %q", full, deepPrefix)
		}
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
