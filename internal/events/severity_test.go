// SPDX-License-Identifier: Apache-2.0

package events

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestSeverity_String(t *testing.T) {
	t.Parallel()
	cases := []struct {
		s    Severity
		want string
	}{
		{SeverityUnknown, "unknown"},
		{SeverityDebug, "debug"},
		{SeverityInfo, "info"},
		{SeverityWarn, "warn"},
		{SeverityError, "error"},
		{SeverityCritical, "critical"},
		{Severity(99), "severity(99)"},
	}
	for _, c := range cases {
		if got := c.s.String(); got != c.want {
			t.Errorf("Severity(%d).String() = %q, want %q", c.s, got, c.want)
		}
	}
}

func TestSeverity_IsValid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		s    Severity
		want bool
	}{
		{SeverityUnknown, false},
		{SeverityDebug, true},
		{SeverityInfo, true},
		{SeverityWarn, true},
		{SeverityError, true},
		{SeverityCritical, true},
		{Severity(99), false},
	}
	for _, c := range cases {
		if got := c.s.IsValid(); got != c.want {
			t.Errorf("Severity(%d).IsValid() = %v, want %v", c.s, got, c.want)
		}
	}
}

func TestSeverity_AtLeast(t *testing.T) {
	t.Parallel()
	cases := []struct {
		s, threshold Severity
		want         bool
	}{
		{SeverityDebug, SeverityDebug, true},
		{SeverityInfo, SeverityDebug, true},
		{SeverityWarn, SeverityWarn, true},
		{SeverityError, SeverityWarn, true},
		{SeverityCritical, SeverityWarn, true},
		{SeverityInfo, SeverityWarn, false},
		{SeverityDebug, SeverityCritical, false},
		// Invalid sides always report false — no event silently passes a misconfigured filter.
		{SeverityUnknown, SeverityWarn, false},
		{SeverityWarn, SeverityUnknown, false},
		{Severity(99), SeverityWarn, false},
		{SeverityWarn, Severity(99), false},
	}
	for _, c := range cases {
		if got := c.s.AtLeast(c.threshold); got != c.want {
			t.Errorf("Severity(%s).AtLeast(%s) = %v, want %v", c.s, c.threshold, got, c.want)
		}
	}
}

func TestParseSeverity_Valid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want Severity
	}{
		{"debug", SeverityDebug},
		{"info", SeverityInfo},
		{"warn", SeverityWarn},
		{"warning", SeverityWarn}, // alias
		{"error", SeverityError},
		{"critical", SeverityCritical},
		{"fatal", SeverityCritical}, // alias
		{"  WARN ", SeverityWarn},   // whitespace + case
		{"Critical", SeverityCritical},
	}
	for _, c := range cases {
		got, err := ParseSeverity(c.in)
		if err != nil {
			t.Errorf("ParseSeverity(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseSeverity(%q) = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestParseSeverity_Invalid(t *testing.T) {
	t.Parallel()
	cases := []string{"", "  ", "trace", "panic", "informational", "WARN!"}
	for _, in := range cases {
		_, err := ParseSeverity(in)
		if err == nil {
			t.Errorf("ParseSeverity(%q) succeeded; want error", in)
			continue
		}
		if !errors.Is(err, ErrInvalidEvent) {
			t.Errorf("ParseSeverity(%q) err = %v; want errors.Is(ErrInvalidEvent)", in, err)
		}
	}
}

func TestSeverity_MarshalUnmarshalText(t *testing.T) {
	t.Parallel()
	cases := []Severity{SeverityDebug, SeverityInfo, SeverityWarn, SeverityError, SeverityCritical}
	for _, s := range cases {
		b, err := s.MarshalText()
		if err != nil {
			t.Errorf("MarshalText(%s): %v", s, err)
			continue
		}
		var rt Severity
		if err := rt.UnmarshalText(b); err != nil {
			t.Errorf("UnmarshalText(%q): %v", b, err)
			continue
		}
		if rt != s {
			t.Errorf("round-trip %s -> %s", s, rt)
		}
	}

	// Unknown marshals (so zero-value Events still round-trip for debug),
	// and unmarshals back to Unknown.
	b, err := SeverityUnknown.MarshalText()
	if err != nil {
		t.Errorf("MarshalText(Unknown): unexpected err %v", err)
	}
	if string(b) != "unknown" {
		t.Errorf("MarshalText(Unknown) = %q, want %q", b, "unknown")
	}

	// Out-of-range marshal errors.
	if _, err := Severity(99).MarshalText(); err == nil {
		t.Errorf("MarshalText(99) succeeded; want error")
	}

	// Empty bytes unmarshal to Unknown (missing-field case).
	sev := SeverityInfo
	if err := sev.UnmarshalText(nil); err != nil {
		t.Errorf("UnmarshalText(nil): %v", err)
	}
	if sev != SeverityUnknown {
		t.Errorf("UnmarshalText(nil) -> %s, want Unknown", sev)
	}

	// Bad bytes unmarshal errors.
	if err := sev.UnmarshalText([]byte("bogus")); err == nil {
		t.Errorf("UnmarshalText(bogus) succeeded; want error")
	}
}

func TestSeverity_JSON(t *testing.T) {
	t.Parallel()
	type wrap struct {
		Sev Severity `json:"sev"`
	}
	in := wrap{Sev: SeverityWarn}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	want := `{"sev":"warn"}`
	if string(b) != want {
		t.Errorf("json = %s, want %s", b, want)
	}
	var out wrap
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if out.Sev != SeverityWarn {
		t.Errorf("round-trip = %s, want warn", out.Sev)
	}
}

func TestAllSeverities(t *testing.T) {
	t.Parallel()
	got := AllSeverities()
	want := []Severity{SeverityDebug, SeverityInfo, SeverityWarn, SeverityError, SeverityCritical}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, s := range want {
		if got[i] != s {
			t.Errorf("AllSeverities()[%d] = %s, want %s", i, got[i], s)
		}
	}
	// Returned slice is fresh — mutating must not leak.
	got[0] = SeverityCritical
	again := AllSeverities()
	if again[0] != SeverityDebug {
		t.Errorf("AllSeverities() returns shared slice; first element should still be debug")
	}
}
