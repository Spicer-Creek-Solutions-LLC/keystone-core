// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// ---- ParseSPIFFEID ------------------------------------------------

func TestParseSPIFFEID_Valid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		s          string
		wantTD     string
		wantPath   string
		wantSegs   []string
	}{
		{"spiffe://kscore.local", "kscore.local", "", nil},
		{"spiffe://kscore.local/agent/agent-1", "kscore.local", "/agent/agent-1", []string{"agent", "agent-1"}},
		{"spiffe://kscore.local/server/control-plane", "kscore.local", "/server/control-plane", []string{"server", "control-plane"}},
		{"spiffe://example.org/service/state-runner", "example.org", "/service/state-runner", []string{"service", "state-runner"}},
		{"spiffe://td-1.example.com/a/b/c/d", "td-1.example.com", "/a/b/c/d", []string{"a", "b", "c", "d"}},
		{"spiffe://kscore.local/agent/agent_with_underscores", "kscore.local", "/agent/agent_with_underscores", []string{"agent", "agent_with_underscores"}},
	}
	for _, c := range cases {
		t.Run(c.s, func(t *testing.T) {
			id, err := ParseSPIFFEID(c.s)
			if err != nil {
				t.Fatalf("ParseSPIFFEID(%q): %v", c.s, err)
			}
			if got := id.TrustDomain(); got != c.wantTD {
				t.Errorf("TrustDomain = %q, want %q", got, c.wantTD)
			}
			if got := id.Path(); got != c.wantPath {
				t.Errorf("Path = %q, want %q", got, c.wantPath)
			}
			if got := id.Segments(); !equalStrSlice(got, c.wantSegs) {
				t.Errorf("Segments = %v, want %v", got, c.wantSegs)
			}
			if got := id.String(); got != c.s {
				t.Errorf("String = %q, want %q", got, c.s)
			}
		})
	}
}

func TestParseSPIFFEID_Invalid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		s    string
	}{
		{"empty", ""},
		{"missing scheme", "kscore.local/agent/a"},
		{"wrong scheme", "https://kscore.local/agent/a"},
		{"no trust domain", "spiffe:///agent/a"},
		{"double slash in path", "spiffe://kscore.local//agent"},
		{"dot segment", "spiffe://kscore.local/./a"},
		{"dotdot segment", "spiffe://kscore.local/a/.."},
		{"uppercase trust domain", "spiffe://Kscore.Local/agent/a"},
		// Note: path segments are case-preserving per RFC 3986 + the
		// SPIFFE-ID spec — `Agent` is a valid segment. We do NOT
		// reject mixed case in paths; operators should adopt
		// lowercase by convention, but the parser does not enforce.
		{"trust domain with port", "spiffe://kscore.local:9000/agent/a"},
		{"query string", "spiffe://kscore.local/agent/a?role=admin"},
		{"fragment", "spiffe://kscore.local/agent/a#frag"},
		{"userinfo", "spiffe://alice@kscore.local/a"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseSPIFFEID(c.s)
			if err == nil {
				t.Fatalf("ParseSPIFFEID(%q) succeeded; want error", c.s)
			}
			if !errors.Is(err, ErrInvalidSPIFFEID) {
				t.Errorf("err = %v; want errors.Is(ErrInvalidSPIFFEID)", err)
			}
		})
	}
}

// ---- MustParseSPIFFEID -------------------------------------------

func TestMustParseSPIFFEID_Panics(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Error("MustParseSPIFFEID on bad input did not panic")
		}
	}()
	_ = MustParseSPIFFEID("not-a-spiffe-id")
}

func TestMustParseSPIFFEID_OK(t *testing.T) {
	t.Parallel()
	id := MustParseSPIFFEID("spiffe://kscore.local/agent/a")
	if id.IsZero() {
		t.Error("MustParseSPIFFEID returned zero value")
	}
}

// ---- NewSPIFFEID --------------------------------------------------

func TestNewSPIFFEID_HappyPath(t *testing.T) {
	t.Parallel()
	id, err := NewSPIFFEID("kscore.local", "agent", "agent-1")
	if err != nil {
		t.Fatalf("NewSPIFFEID: %v", err)
	}
	if got, want := id.String(), "spiffe://kscore.local/agent/agent-1"; got != want {
		t.Errorf("String = %q, want %q", got, want)
	}
}

func TestNewSPIFFEID_TrustDomainOnly(t *testing.T) {
	t.Parallel()
	id, err := NewSPIFFEID("kscore.local")
	if err != nil {
		t.Fatalf("NewSPIFFEID: %v", err)
	}
	if got, want := id.String(), "spiffe://kscore.local"; got != want {
		t.Errorf("String = %q, want %q", got, want)
	}
	if got := id.Path(); got != "" {
		t.Errorf("Path = %q, want empty", got)
	}
	if got := id.Segments(); got != nil {
		t.Errorf("Segments = %v, want nil", got)
	}
}

func TestNewSPIFFEID_RejectsBadTrustDomain(t *testing.T) {
	t.Parallel()
	for _, td := range []string{"", "UPPER", "with space", "kscore.local:9000"} {
		t.Run(td, func(t *testing.T) {
			_, err := NewSPIFFEID(td, "agent", "a")
			if err == nil {
				t.Fatalf("trust domain %q accepted; want error", td)
			}
			if !errors.Is(err, ErrInvalidSPIFFEID) {
				t.Errorf("err = %v; want errors.Is(ErrInvalidSPIFFEID)", err)
			}
			if !strings.Contains(err.Error(), "trust domain") {
				t.Errorf("err = %v; want \"trust domain\" cited", err)
			}
		})
	}
}

func TestNewSPIFFEID_RejectsBadSegment(t *testing.T) {
	t.Parallel()
	// Mixed-case segments ("Agent") are NOT in this list — the
	// SPIFFE-ID spec preserves segment case per RFC 3986. The
	// rejection set is: empty / slash / dot / dot-dot only.
	for _, seg := range []string{"", "with/slash", ".", ".."} {
		t.Run(seg, func(t *testing.T) {
			_, err := NewSPIFFEID("kscore.local", seg)
			if err == nil {
				t.Fatalf("segment %q accepted; want error", seg)
			}
			if !errors.Is(err, ErrInvalidSPIFFEID) {
				t.Errorf("err = %v; want errors.Is(ErrInvalidSPIFFEID)", err)
			}
		})
	}
}

func TestNewSPIFFEID_PreservesSegmentCase(t *testing.T) {
	t.Parallel()
	// The parser is case-preserving on path segments per the
	// SPIFFE-ID spec — guardrail so a stricter future canonicaliser
	// would surface as a test failure.
	id, err := NewSPIFFEID("kscore.local", "Agent", "Agent-1")
	if err != nil {
		t.Fatalf("mixed-case segments rejected: %v", err)
	}
	if got, want := id.String(), "spiffe://kscore.local/Agent/Agent-1"; got != want {
		t.Errorf("String = %q, want %q", got, want)
	}
}

// ---- Equal / IsZero / MemberOf -----------------------------------

func TestSPIFFEID_Equal(t *testing.T) {
	t.Parallel()
	a := MustParseSPIFFEID("spiffe://kscore.local/agent/x")
	b := MustParseSPIFFEID("spiffe://kscore.local/agent/x")
	c := MustParseSPIFFEID("spiffe://kscore.local/agent/y")
	if !a.Equal(b) {
		t.Error("equal IDs should be Equal")
	}
	if a == c || a.Equal(c) {
		t.Error("different IDs should not be Equal")
	}
}

func TestSPIFFEID_IsZero(t *testing.T) {
	t.Parallel()
	var zero SPIFFEID
	if !zero.IsZero() {
		t.Error("zero value not IsZero")
	}
	if zero.String() != "" {
		t.Errorf("zero.String() = %q, want empty", zero.String())
	}
	if zero.URI() != nil {
		t.Errorf("zero.URI() = %v, want nil", zero.URI())
	}
	if zero.TrustDomain() != "" {
		t.Errorf("zero.TrustDomain() = %q, want empty", zero.TrustDomain())
	}
	if zero.Path() != "" {
		t.Errorf("zero.Path() = %q, want empty", zero.Path())
	}

	id := MustParseSPIFFEID("spiffe://kscore.local/agent/a")
	if id.IsZero() {
		t.Error("non-zero ID reports IsZero")
	}
}

func TestSPIFFEID_MemberOf(t *testing.T) {
	t.Parallel()
	id := MustParseSPIFFEID("spiffe://kscore.local/agent/a")
	if !id.MemberOf("kscore.local") {
		t.Error("ID not member of its own trust domain")
	}
	if id.MemberOf("other.org") {
		t.Error("ID member of unrelated trust domain")
	}
	// Bad trust-domain name → false, not error.
	if id.MemberOf("UPPER CASE") {
		t.Error("MemberOf with bad TD should return false")
	}
}

// ---- URI ---------------------------------------------------------

func TestSPIFFEID_URI(t *testing.T) {
	t.Parallel()
	id := MustParseSPIFFEID("spiffe://kscore.local/agent/a")
	uri := id.URI()
	if uri == nil {
		t.Fatal("URI() = nil for non-zero ID")
		return
	}
	if uri.Scheme != "spiffe" {
		t.Errorf("uri.Scheme = %q, want spiffe", uri.Scheme)
	}
	if uri.Host != "kscore.local" {
		t.Errorf("uri.Host = %q, want kscore.local", uri.Host)
	}
	if uri.Path != "/agent/a" {
		t.Errorf("uri.Path = %q, want /agent/a", uri.Path)
	}
	if uri.String() != id.String() {
		t.Errorf("URI round-trip: %q != %q", uri.String(), id.String())
	}
}

// ---- Standard-path constructors -----------------------------------

func TestAgentID(t *testing.T) {
	t.Parallel()
	id, err := AgentID(DefaultTrustDomain, "agent-1")
	if err != nil {
		t.Fatalf("AgentID: %v", err)
	}
	if got, want := id.String(), "spiffe://kscore.local/agent/agent-1"; got != want {
		t.Errorf("String = %q, want %q", got, want)
	}
	if got := id.Segments(); !equalStrSlice(got, []string{"agent", "agent-1"}) {
		t.Errorf("Segments = %v", got)
	}
}

func TestAgentID_Rejects(t *testing.T) {
	t.Parallel()
	// Note: mixed-case agent IDs are NOT rejected (RFC 3986 / SPIFFE
	// preserve case). Operators picking lowercase is a convention,
	// not a parser rule.
	for _, c := range []struct{ td, name string }{
		{"", "a"},                           // empty TD
		{DefaultTrustDomain, ""},            // empty name
		{DefaultTrustDomain, "agent/inner"}, // slash in name
		{DefaultTrustDomain, "."},           // dot segment
		{DefaultTrustDomain, ".."},          // dotdot segment
	} {
		_, err := AgentID(c.td, c.name)
		if err == nil {
			t.Errorf("AgentID(%q, %q) accepted; want error", c.td, c.name)
			continue
		}
		if !errors.Is(err, ErrInvalidSPIFFEID) {
			t.Errorf("AgentID(%q, %q) err = %v; want errors.Is(ErrInvalidSPIFFEID)", c.td, c.name, err)
		}
	}
}

func TestServerID(t *testing.T) {
	t.Parallel()
	id, err := ServerID(DefaultTrustDomain, "control-plane")
	if err != nil {
		t.Fatalf("ServerID: %v", err)
	}
	if got, want := id.String(), "spiffe://kscore.local/server/control-plane"; got != want {
		t.Errorf("String = %q, want %q", got, want)
	}
}

func TestServerID_RejectsEmptyName(t *testing.T) {
	t.Parallel()
	_, err := ServerID(DefaultTrustDomain, "")
	if err == nil || !errors.Is(err, ErrInvalidSPIFFEID) {
		t.Errorf("err = %v; want ErrInvalidSPIFFEID", err)
	}
}

func TestServiceID(t *testing.T) {
	t.Parallel()
	id, err := ServiceID(DefaultTrustDomain, "state-runner")
	if err != nil {
		t.Fatalf("ServiceID: %v", err)
	}
	if got, want := id.String(), "spiffe://kscore.local/service/state-runner"; got != want {
		t.Errorf("String = %q, want %q", got, want)
	}
}

func TestServiceID_RejectsEmptyName(t *testing.T) {
	t.Parallel()
	_, err := ServiceID(DefaultTrustDomain, "")
	if err == nil || !errors.Is(err, ErrInvalidSPIFFEID) {
		t.Errorf("err = %v; want ErrInvalidSPIFFEID", err)
	}
}

// ---- Text + JSON marshaling --------------------------------------

func TestSPIFFEID_MarshalText_RoundTrip(t *testing.T) {
	t.Parallel()
	cases := []string{
		"spiffe://kscore.local/agent/agent-1",
		"spiffe://kscore.local/server/control-plane",
		"spiffe://example.org/service/state-runner",
	}
	for _, s := range cases {
		t.Run(s, func(t *testing.T) {
			id := MustParseSPIFFEID(s)
			b, err := id.MarshalText()
			if err != nil {
				t.Fatalf("MarshalText: %v", err)
			}
			if string(b) != s {
				t.Errorf("MarshalText = %q, want %q", string(b), s)
			}
			var back SPIFFEID
			if err := back.UnmarshalText(b); err != nil {
				t.Fatalf("UnmarshalText: %v", err)
			}
			if !id.Equal(back) {
				t.Errorf("round-trip: %q != %q", id, back)
			}
		})
	}
}

func TestSPIFFEID_MarshalText_Zero(t *testing.T) {
	t.Parallel()
	var zero SPIFFEID
	b, err := zero.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText zero: %v", err)
	}
	if len(b) != 0 {
		t.Errorf("zero MarshalText = %q, want empty", string(b))
	}
}

func TestSPIFFEID_UnmarshalText_Empty(t *testing.T) {
	t.Parallel()
	// Pre-fill with a non-zero value to confirm Unmarshal resets it.
	id := MustParseSPIFFEID("spiffe://kscore.local/a/b")
	if err := id.UnmarshalText([]byte{}); err != nil {
		t.Fatalf("UnmarshalText empty: %v", err)
	}
	if !id.IsZero() {
		t.Errorf("UnmarshalText empty: id = %q, want zero", id)
	}
}

func TestSPIFFEID_UnmarshalText_Invalid(t *testing.T) {
	t.Parallel()
	var id SPIFFEID
	err := id.UnmarshalText([]byte("not-a-spiffe-id"))
	if err == nil || !errors.Is(err, ErrInvalidSPIFFEID) {
		t.Errorf("err = %v; want ErrInvalidSPIFFEID", err)
	}
}

func TestSPIFFEID_JSON_RoundTrip(t *testing.T) {
	t.Parallel()
	type wrapper struct {
		ID SPIFFEID `json:"id"`
	}
	want := wrapper{ID: MustParseSPIFFEID("spiffe://kscore.local/agent/agent-1")}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(data), `"spiffe://kscore.local/agent/agent-1"`) {
		t.Errorf("JSON = %s, want canonical string", string(data))
	}
	var got wrapper
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !got.ID.Equal(want.ID) {
		t.Errorf("round-trip: %q != %q", got.ID, want.ID)
	}
}

func TestSPIFFEID_JSON_Null(t *testing.T) {
	t.Parallel()
	// Zero value emits null.
	var zero SPIFFEID
	b, err := json.Marshal(zero)
	if err != nil {
		t.Fatalf("Marshal zero: %v", err)
	}
	if string(b) != "null" {
		t.Errorf("zero JSON = %q, want null", string(b))
	}
	// null decodes back to zero.
	back := MustParseSPIFFEID("spiffe://kscore.local/x")
	if err := json.Unmarshal([]byte("null"), &back); err != nil {
		t.Fatalf("Unmarshal null: %v", err)
	}
	if !back.IsZero() {
		t.Errorf("UnmarshalJSON null: id = %q, want zero", back)
	}
}

func TestSPIFFEID_JSON_BadType(t *testing.T) {
	t.Parallel()
	var id SPIFFEID
	err := json.Unmarshal([]byte("42"), &id)
	if err == nil || !errors.Is(err, ErrInvalidSPIFFEID) {
		t.Errorf("err = %v; want ErrInvalidSPIFFEID", err)
	}
}

func TestSPIFFEID_JSON_BadString(t *testing.T) {
	t.Parallel()
	var id SPIFFEID
	err := json.Unmarshal([]byte(`"not-a-spiffe-id"`), &id)
	if err == nil || !errors.Is(err, ErrInvalidSPIFFEID) {
		t.Errorf("err = %v; want ErrInvalidSPIFFEID", err)
	}
}

// ---- Helpers -----------------------------------------------------

func equalStrSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
