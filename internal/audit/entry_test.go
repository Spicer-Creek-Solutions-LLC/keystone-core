// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// ---- Severity -----------------------------------------------------------

func TestSeverity_String(t *testing.T) {
	t.Parallel()
	cases := []struct {
		s    Severity
		want string
	}{
		{SeverityUnknown, "unknown"},
		{SeverityLow, "low"},
		{SeverityMedium, "medium"},
		{SeverityHigh, "high"},
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
		{SeverityLow, true},
		{SeverityMedium, true},
		{SeverityHigh, true},
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
		{SeverityLow, SeverityLow, true},
		{SeverityMedium, SeverityLow, true},
		{SeverityCritical, SeverityHigh, true},
		{SeverityLow, SeverityMedium, false},
		{SeverityMedium, SeverityCritical, false},
		// Invalid sides always report false.
		{SeverityUnknown, SeverityHigh, false},
		{SeverityHigh, SeverityUnknown, false},
		{Severity(99), SeverityHigh, false},
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
		{"low", SeverityLow},
		{"medium", SeverityMedium},
		{"high", SeverityHigh},
		{"critical", SeverityCritical},
		{"  HIGH ", SeverityHigh},
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
	for _, in := range []string{"", "info", "fatal", "trace"} {
		_, err := ParseSeverity(in)
		if err == nil {
			t.Errorf("ParseSeverity(%q) succeeded; want error", in)
			continue
		}
		if !errors.Is(err, ErrInvalidAuditEntry) {
			t.Errorf("ParseSeverity(%q): err = %v; want ErrInvalidAuditEntry", in, err)
		}
	}
}

func TestSeverity_MarshalUnmarshalText(t *testing.T) {
	t.Parallel()
	for _, s := range []Severity{SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical} {
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
	// Empty unmarshal → Unknown (missing-field case).
	s := SeverityHigh
	if err := s.UnmarshalText(nil); err != nil {
		t.Errorf("UnmarshalText(nil): %v", err)
	}
	if s != SeverityUnknown {
		t.Errorf("UnmarshalText(nil) → %s, want Unknown", s)
	}
	// Out-of-range marshal errors.
	if _, err := Severity(99).MarshalText(); err == nil {
		t.Errorf("MarshalText(99) succeeded; want error")
	}
	// Bad bytes unmarshal errors.
	if err := s.UnmarshalText([]byte("bogus")); err == nil {
		t.Errorf("UnmarshalText(bogus) succeeded; want error")
	}
}

func TestAllSeverities(t *testing.T) {
	t.Parallel()
	got := AllSeverities()
	want := []Severity{SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, s := range want {
		if got[i] != s {
			t.Errorf("[%d] = %s, want %s", i, got[i], s)
		}
	}
	// Fresh slice each call.
	got[0] = SeverityCritical
	again := AllSeverities()
	if again[0] != SeverityLow {
		t.Errorf("returned slice aliased; mutation leaked: %s", again[0])
	}
}

// ---- EnforcementMode ----------------------------------------------------

func TestEnforcementMode_String(t *testing.T) {
	t.Parallel()
	cases := []struct {
		m    EnforcementMode
		want string
	}{
		{EnforcementModeUnknown, "unknown"},
		{EnforcementModeAudit, "audit"},
		{EnforcementModeWarn, "warn"},
		{EnforcementModeEnforce, "enforce"},
		{EnforcementMode(99), "enforcement_mode(99)"},
	}
	for _, c := range cases {
		if got := c.m.String(); got != c.want {
			t.Errorf("EnforcementMode(%d).String() = %q", c.m, got)
		}
	}
}

func TestEnforcementMode_IsValid(t *testing.T) {
	t.Parallel()
	for _, m := range []EnforcementMode{EnforcementModeAudit, EnforcementModeWarn, EnforcementModeEnforce} {
		if !m.IsValid() {
			t.Errorf("%s reported invalid", m)
		}
	}
	for _, m := range []EnforcementMode{EnforcementModeUnknown, EnforcementMode(99)} {
		if m.IsValid() {
			t.Errorf("%s reported valid", m)
		}
	}
}

func TestParseEnforcementMode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want EnforcementMode
	}{
		{"audit", EnforcementModeAudit},
		{"warn", EnforcementModeWarn},
		{"enforce", EnforcementModeEnforce},
		{"  ENFORCE ", EnforcementModeEnforce},
	}
	for _, c := range cases {
		got, err := ParseEnforcementMode(c.in)
		if err != nil {
			t.Errorf("ParseEnforcementMode(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseEnforcementMode(%q) = %s", c.in, got)
		}
	}
	for _, bad := range []string{"", "block", "log", "unknown"} {
		_, err := ParseEnforcementMode(bad)
		if err == nil {
			t.Errorf("ParseEnforcementMode(%q) succeeded; want error", bad)
			continue
		}
		if !errors.Is(err, ErrInvalidAuditEntry) {
			t.Errorf("err = %v; want ErrInvalidAuditEntry", err)
		}
	}
}

func TestEnforcementMode_MarshalUnmarshal(t *testing.T) {
	t.Parallel()
	for _, m := range []EnforcementMode{EnforcementModeAudit, EnforcementModeWarn, EnforcementModeEnforce} {
		b, err := m.MarshalText()
		if err != nil {
			t.Errorf("Marshal(%s): %v", m, err)
			continue
		}
		var rt EnforcementMode
		if err := rt.UnmarshalText(b); err != nil {
			t.Errorf("Unmarshal: %v", err)
			continue
		}
		if rt != m {
			t.Errorf("round-trip %s -> %s", m, rt)
		}
	}
	if _, err := EnforcementMode(99).MarshalText(); err == nil {
		t.Errorf("MarshalText(99) succeeded")
	}
	m := EnforcementModeAudit
	if err := m.UnmarshalText(nil); err != nil || m != EnforcementModeUnknown {
		t.Errorf("UnmarshalText(nil): err=%v, m=%s", err, m)
	}
}

// ---- PolicyType ---------------------------------------------------------

func TestPolicyType_IsKnown(t *testing.T) {
	t.Parallel()
	for _, p := range []PolicyType{PolicyTypeOPA, PolicyTypeCEL, PolicyTypeBuiltin} {
		if !p.IsKnown() {
			t.Errorf("%s reported unknown", p)
		}
	}
	for _, p := range []PolicyType{"", "rego", "Builtin", " opa"} {
		if p.IsKnown() {
			t.Errorf("%q reported known", p)
		}
	}
}

func TestParsePolicyType(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want PolicyType
	}{
		{"opa", PolicyTypeOPA},
		{"cel", PolicyTypeCEL},
		{"builtin", PolicyTypeBuiltin},
		{"  OPA ", PolicyTypeOPA},
		{"", ""},        // empty is allowed (non-policy entry sentinel)
		{"   \t  ", ""}, // whitespace-only also empty
	}
	for _, c := range cases {
		got, err := ParsePolicyType(c.in)
		if err != nil {
			t.Errorf("ParsePolicyType(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParsePolicyType(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	for _, bad := range []string{"rego", "Builtin-X", "unknown"} {
		_, err := ParsePolicyType(bad)
		if err == nil {
			t.Errorf("ParsePolicyType(%q) succeeded", bad)
			continue
		}
		if !errors.Is(err, ErrInvalidAuditEntry) {
			t.Errorf("err = %v; want ErrInvalidAuditEntry", err)
		}
	}
}

// ---- NewAuditEntry ------------------------------------------------------

func TestNewAuditEntry_Defaults(t *testing.T) {
	t.Parallel()
	before := time.Now().UTC()
	e, err := NewAuditEntry(AuditEntryInput{
		Action: "get_secret",
	})
	if err != nil {
		t.Fatalf("NewAuditEntry: %v", err)
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
	if e.Action != "get_secret" {
		t.Errorf("Action = %q", e.Action)
	}
	if e.Timestamp.Before(before) || e.Timestamp.After(after) {
		t.Errorf("Timestamp %v outside [%v, %v]", e.Timestamp, before, after)
	}
	if e.Timestamp.Location() != time.UTC {
		t.Errorf("Timestamp location = %v, want UTC", e.Timestamp.Location())
	}
	if e.Severity != SeverityLow {
		t.Errorf("default Severity = %s, want low", e.Severity)
	}
	if e.EnforcementMode != EnforcementModeAudit {
		t.Errorf("default EnforcementMode = %s, want audit", e.EnforcementMode)
	}
	if e.PolicyType != "" {
		t.Errorf("default PolicyType = %q, want empty (non-policy)", e.PolicyType)
	}
}

func TestNewAuditEntry_RequiresAction(t *testing.T) {
	t.Parallel()
	_, err := NewAuditEntry(AuditEntryInput{Action: ""})
	if err == nil {
		t.Fatalf("missing action validated; want error")
	}
	if !errors.Is(err, ErrInvalidAuditEntry) {
		t.Errorf("err = %v; want ErrInvalidAuditEntry", err)
	}
}

func TestNewAuditEntry_RejectsBadPolicyType(t *testing.T) {
	t.Parallel()
	_, err := NewAuditEntry(AuditEntryInput{
		Action:     "policy.evaluate",
		PolicyType: PolicyType("rego"),
	})
	if err == nil {
		t.Fatalf("bad policy_type validated")
	}
	if !errors.Is(err, ErrInvalidAuditEntry) {
		t.Errorf("err = %v", err)
	}
}

func TestNewAuditEntry_PreservesProvidedFields(t *testing.T) {
	t.Parallel()
	in := AuditEntryInput{
		PolicyID:        "require-labels",
		PolicyName:      "Require Labels",
		PolicyType:      PolicyTypeBuiltin,
		ResourceType:    "secret",
		Allowed:         false,
		Duration:        15 * time.Millisecond,
		Violations:      []Violation{{Rule: "missing-owner", Severity: SeverityHigh}},
		EnforcementMode: EnforcementModeEnforce,
		Severity:        SeverityHigh,
		User:            "spiffe://kscore.local/agent/agent-1",
		Action:          "policy.evaluate",
		Metadata:        map[string]string{"region": "us-east"},
	}
	e, err := NewAuditEntry(in)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if e.PolicyID != in.PolicyID || e.PolicyName != in.PolicyName || e.PolicyType != in.PolicyType {
		t.Errorf("policy fields lost: %+v", e)
	}
	if e.ResourceType != "secret" {
		t.Errorf("ResourceType = %q", e.ResourceType)
	}
	if e.Allowed {
		t.Errorf("Allowed = true, want false")
	}
	if e.Severity != SeverityHigh {
		t.Errorf("Severity override lost: %s", e.Severity)
	}
	if e.EnforcementMode != EnforcementModeEnforce {
		t.Errorf("EnforcementMode override lost: %s", e.EnforcementMode)
	}
	if len(e.Violations) != 1 || e.Violations[0].Rule != "missing-owner" {
		t.Errorf("Violations lost: %+v", e.Violations)
	}
	if e.Metadata["region"] != "us-east" {
		t.Errorf("Metadata lost: %+v", e.Metadata)
	}
}

func TestMustNewAuditEntry(t *testing.T) {
	t.Parallel()
	e := MustNewAuditEntry(AuditEntryInput{Action: "x"})
	if e.ID == "" {
		t.Errorf("ID empty")
	}
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("MustNewAuditEntry did not panic on bad input")
		}
	}()
	_ = MustNewAuditEntry(AuditEntryInput{Action: ""}) // missing required field
}

// ---- AuditEntry behaviour ----------------------------------------------

func TestAuditEntry_IsZero(t *testing.T) {
	t.Parallel()
	var z AuditEntry
	if !z.IsZero() {
		t.Errorf("zero entry reported non-zero")
	}
	e := MustNewAuditEntry(AuditEntryInput{Action: "x"})
	if e.IsZero() {
		t.Errorf("constructed entry reported zero")
	}
}

func TestAuditEntry_Validate(t *testing.T) {
	t.Parallel()
	good := MustNewAuditEntry(AuditEntryInput{Action: "x"})
	if err := good.Validate(); err != nil {
		t.Errorf("good: %v", err)
	}
	cases := []struct {
		name   string
		mutate func(*AuditEntry)
	}{
		{"empty id", func(e *AuditEntry) { e.ID = "" }},
		{"empty action", func(e *AuditEntry) { e.Action = "" }},
		{"zero timestamp", func(e *AuditEntry) { e.Timestamp = time.Time{} }},
		{"unknown severity", func(e *AuditEntry) { e.Severity = SeverityUnknown }},
		{"out-of-range severity", func(e *AuditEntry) { e.Severity = Severity(99) }},
		{"unknown enforcement_mode", func(e *AuditEntry) { e.EnforcementMode = EnforcementModeUnknown }},
		{"bad policy_type", func(e *AuditEntry) { e.PolicyType = PolicyType("rego") }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			e := MustNewAuditEntry(AuditEntryInput{Action: "x"})
			c.mutate(&e)
			err := e.Validate()
			if err == nil {
				t.Fatalf("Validate succeeded; want error")
			}
			if !errors.Is(err, ErrInvalidAuditEntry) {
				t.Errorf("err = %v", err)
			}
		})
	}
}

func TestAuditEntry_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	in := MustNewAuditEntry(AuditEntryInput{
		PolicyID:        "require-labels",
		PolicyName:      "Require Labels",
		PolicyType:      PolicyTypeBuiltin,
		ResourceType:    "secret",
		Allowed:         false,
		Duration:        25 * time.Millisecond,
		Violations:      []Violation{{Rule: "missing-owner", Message: "owner label not set", Severity: SeverityHigh}},
		EnforcementMode: EnforcementModeEnforce,
		Severity:        SeverityHigh,
		User:            "alice",
		Action:          "policy.evaluate",
		Metadata:        map[string]string{"region": "us-east"},
	})
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, want := range []string{`"severity":"high"`, `"enforcement_mode":"enforce"`, `"policy_type":"builtin"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("missing %q in: %s", want, b)
		}
	}
	var out AuditEntry
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.ID != in.ID || out.Action != in.Action || out.Severity != in.Severity ||
		out.EnforcementMode != in.EnforcementMode || out.PolicyType != in.PolicyType {
		t.Errorf("round-trip mismatch:\n in=%+v\nout=%+v", in, out)
	}
	if len(out.Violations) != 1 || out.Violations[0].Severity != SeverityHigh {
		t.Errorf("violations lost: %+v", out.Violations)
	}
}

// ---- UUIDv7 k-sortability ----------------------------------------------

func TestNewAuditEntry_IDsAreKSortable(t *testing.T) {
	t.Parallel()
	const n = 30
	ids := make([]string, n)
	for i := 0; i < n; i++ {
		e := MustNewAuditEntry(AuditEntryInput{Action: "x"})
		ids[i] = e.ID
		// 100µs between stamps so v7 timestamp bytes advance.
		time.Sleep(100 * time.Microsecond)
	}
	for i := 1; i < n; i++ {
		if ids[i-1] > ids[i] {
			t.Errorf("ids not k-sorted at %d: %q > %q", i, ids[i-1], ids[i])
		}
	}
}
