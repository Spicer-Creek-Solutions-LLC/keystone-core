package audit

import (
	"strings"
	"testing"
	"time"
)

func TestNewRedactionConfig_RejectsInvalidRegex(t *testing.T) {
	t.Parallel()
	_, err := NewRedactionConfig(RedactionConfigInput{
		RedactPatterns: []string{"["}, // invalid regex
	})
	if err == nil {
		t.Errorf("invalid regex validated; want error")
	}
}

func TestNewRedactionConfig_EmptyIsNoop(t *testing.T) {
	t.Parallel()
	c, err := NewRedactionConfig(RedactionConfigInput{})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if !c.IsNoop() {
		t.Errorf("empty config not no-op")
	}
}

func TestNewRedactionConfig_NilReceiverIsNoop(t *testing.T) {
	t.Parallel()
	var c *RedactionConfig
	if !c.IsNoop() {
		t.Errorf("nil receiver should report no-op")
	}
}

func TestNewRedactionConfig_DefaultReplacement(t *testing.T) {
	t.Parallel()
	c, err := NewRedactionConfig(RedactionConfigInput{
		RedactPatterns: []string{`secret`},
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if c.Replacement != DefaultRedactionReplacement {
		t.Errorf("Replacement = %q, want %q", c.Replacement, DefaultRedactionReplacement)
	}
}

func TestRedactionConfig_Apply_NoOpReturnsIdentity(t *testing.T) {
	t.Parallel()
	c, _ := NewRedactionConfig(RedactionConfigInput{})
	in := MustNewAuditEntry(AuditEntryInput{
		Action:   "x",
		User:     "alice",
		Metadata: map[string]string{"k": "v"},
	})
	out := c.Apply(in)
	if out.User != "alice" || out.Metadata["k"] != "v" {
		t.Errorf("noop changed entry: %+v", out)
	}
}

func TestRedactionConfig_Apply_RedactUserBlanksField(t *testing.T) {
	t.Parallel()
	c, _ := NewRedactionConfig(RedactionConfigInput{RedactUser: true})
	in := MustNewAuditEntry(AuditEntryInput{
		Action: "x",
		User:   "alice@example.com",
	})
	out := c.Apply(in)
	if out.User != "" {
		t.Errorf("User not blanked: %q", out.User)
	}
	if in.User != "alice@example.com" {
		t.Errorf("input mutated: %q", in.User)
	}
}

func TestRedactionConfig_Apply_RedactMetadataKeysDrops(t *testing.T) {
	t.Parallel()
	c, _ := NewRedactionConfig(RedactionConfigInput{
		RedactMetadataKeys: []string{"password", "secret_token"},
	})
	in := MustNewAuditEntry(AuditEntryInput{
		Action: "x",
		Metadata: map[string]string{
			"region":       "us-east",
			"password":     "hunter2",
			"secret_token": "abc123",
			"role":         "web",
		},
	})
	out := c.Apply(in)
	if _, has := out.Metadata["password"]; has {
		t.Errorf("password key not dropped: %+v", out.Metadata)
	}
	if _, has := out.Metadata["secret_token"]; has {
		t.Errorf("secret_token key not dropped: %+v", out.Metadata)
	}
	if out.Metadata["region"] != "us-east" {
		t.Errorf("non-redacted region lost: %+v", out.Metadata)
	}
	if out.Metadata["role"] != "web" {
		t.Errorf("non-redacted role lost: %+v", out.Metadata)
	}
	// Input untouched.
	if _, has := in.Metadata["password"]; !has {
		t.Errorf("input mutated")
	}
}

func TestRedactionConfig_Apply_RedactPatternsReplacesMatches(t *testing.T) {
	t.Parallel()
	c, err := NewRedactionConfig(RedactionConfigInput{
		RedactPatterns: []string{`password=\S+`, `token=[A-Za-z0-9]+`},
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	in := MustNewAuditEntry(AuditEntryInput{
		Action: "x",
		Metadata: map[string]string{
			"request": "GET /api?password=hunter2&user=alice",
			"auth":    "Bearer token=abc123XYZ",
			"clean":   "no secrets here",
		},
		Violations: []Violation{{Message: "denial reason: password=secret123 leaked"}},
	})
	out := c.Apply(in)
	if !strings.Contains(out.Metadata["request"], "***") {
		t.Errorf("password pattern not redacted in request: %q", out.Metadata["request"])
	}
	if strings.Contains(out.Metadata["request"], "password=hunter2") {
		t.Errorf("password value leaked: %q", out.Metadata["request"])
	}
	if !strings.Contains(out.Metadata["auth"], "***") {
		t.Errorf("token pattern not redacted: %q", out.Metadata["auth"])
	}
	if out.Metadata["clean"] != "no secrets here" {
		t.Errorf("clean metadata changed: %q", out.Metadata["clean"])
	}
	// Violations Message redacted too.
	if !strings.Contains(out.Violations[0].Message, "***") {
		t.Errorf("violation message not redacted: %q", out.Violations[0].Message)
	}
	// Input untouched (deep-copy assertion).
	if !strings.Contains(in.Violations[0].Message, "password=secret123") {
		t.Errorf("input violations mutated")
	}
}

func TestRedactionConfig_Apply_CustomReplacement(t *testing.T) {
	t.Parallel()
	c, err := NewRedactionConfig(RedactionConfigInput{
		RedactPatterns: []string{`password=\S+`},
		Replacement:    "[REDACTED]",
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	in := MustNewAuditEntry(AuditEntryInput{
		Action:   "x",
		Metadata: map[string]string{"req": "password=secret"},
	})
	out := c.Apply(in)
	if !strings.Contains(out.Metadata["req"], "[REDACTED]") {
		t.Errorf("custom replacement not used: %q", out.Metadata["req"])
	}
}

func TestRedactionConfig_Apply_AllRedactorsCompose(t *testing.T) {
	t.Parallel()
	c, _ := NewRedactionConfig(RedactionConfigInput{
		RedactMetadataKeys: []string{"password"},
		RedactPatterns:     []string{`token=\S+`},
		RedactUser:         true,
	})
	in := MustNewAuditEntry(AuditEntryInput{
		Action: "x",
		User:   "alice",
		Metadata: map[string]string{
			"password": "hunter2",
			"request":  "auth: token=abc123",
		},
	})
	out := c.Apply(in)
	if out.User != "" {
		t.Errorf("User not blanked")
	}
	if _, has := out.Metadata["password"]; has {
		t.Errorf("password key not dropped")
	}
	if !strings.Contains(out.Metadata["request"], "***") {
		t.Errorf("token pattern not redacted")
	}
}

func TestRedactionConfig_Apply_DeepCopiesViolations(t *testing.T) {
	t.Parallel()
	c, _ := NewRedactionConfig(RedactionConfigInput{
		RedactPatterns: []string{`secret`},
	})
	in := MustNewAuditEntry(AuditEntryInput{
		Action:     "x",
		Violations: []Violation{{Rule: "r", Message: "contains secret data"}},
	})
	out := c.Apply(in)
	out.Violations[0].Rule = "MUTATED"
	if in.Violations[0].Rule != "r" {
		t.Errorf("input violations aliased: %s", in.Violations[0].Rule)
	}
}

func TestRedactionConfig_Apply_TimestampPreserved(t *testing.T) {
	t.Parallel()
	c, _ := NewRedactionConfig(RedactionConfigInput{RedactUser: true})
	in := MustNewAuditEntry(AuditEntryInput{Action: "x", User: "alice"})
	in.Timestamp = time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	out := c.Apply(in)
	if !out.Timestamp.Equal(in.Timestamp) {
		t.Errorf("timestamp mutated: %v vs %v", out.Timestamp, in.Timestamp)
	}
}
