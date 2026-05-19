package events

import (
	"errors"
	"testing"
)

func TestCategory_IsKnown(t *testing.T) {
	t.Parallel()
	known := []Category{
		CategoryAgent, CategoryJob, CategoryState,
		CategorySystem, CategoryUser, CategoryPolicy, CategoryRunbook,
	}
	for _, c := range known {
		if !c.IsKnown() {
			t.Errorf("Category(%s).IsKnown() = false, want true", c)
		}
	}
	unknown := []Category{"", "audit", "metric", "Agent", " agent", "agent."}
	for _, c := range unknown {
		if c.IsKnown() {
			t.Errorf("Category(%q).IsKnown() = true, want false", c)
		}
	}
}

func TestCategory_String(t *testing.T) {
	t.Parallel()
	if got := CategoryAgent.String(); got != "agent" {
		t.Errorf("CategoryAgent.String() = %q, want %q", got, "agent")
	}
}

func TestParseCategory_Valid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want Category
	}{
		{"agent", CategoryAgent},
		{"  job ", CategoryJob},
		{"STATE", CategoryState},
		{"System", CategorySystem},
		{"user", CategoryUser},
		{"policy", CategoryPolicy},
	}
	for _, c := range cases {
		got, err := ParseCategory(c.in)
		if err != nil {
			t.Errorf("ParseCategory(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseCategory(%q) = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestParseCategory_Invalid(t *testing.T) {
	t.Parallel()
	cases := []string{"", "   ", "audit", "metric", "agent.", ".agent"}
	for _, in := range cases {
		_, err := ParseCategory(in)
		if err == nil {
			t.Errorf("ParseCategory(%q) succeeded; want error", in)
			continue
		}
		if !errors.Is(err, ErrInvalidEvent) {
			t.Errorf("ParseCategory(%q) err = %v; want errors.Is(ErrInvalidEvent)", in, err)
		}
	}
}

func TestKnownCategories(t *testing.T) {
	t.Parallel()
	got := KnownCategories()
	want := []Category{
		CategoryAgent, CategoryJob, CategoryState,
		CategorySystem, CategoryUser, CategoryPolicy, CategoryRunbook,
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, c := range want {
		if got[i] != c {
			t.Errorf("KnownCategories()[%d] = %s, want %s", i, got[i], c)
		}
	}
	// Returned slice is fresh.
	got[0] = "mutated"
	again := KnownCategories()
	if again[0] != CategoryAgent {
		t.Errorf("KnownCategories() returns shared slice; first element should still be agent")
	}
}
