package envelope

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNew_DefaultsAndOverrides(t *testing.T) {
	payload := []byte(`{"hello":"world"}`)
	env := New(payload, "kscore.test")

	if env.MessageID == "" {
		t.Error("MessageID empty after New")
	}
	if env.Priority != PriorityNormal {
		t.Errorf("Priority = %q, want normal", env.Priority)
	}
	if env.ClusterPrefix != "kscore.test" {
		t.Errorf("ClusterPrefix = %q", env.ClusterPrefix)
	}
	if string(env.Payload) != string(payload) {
		t.Errorf("Payload = %q, want %q", env.Payload, payload)
	}
	if env.TTLMillis != 0 {
		t.Errorf("TTLMillis = %d, want 0", env.TTLMillis)
	}
	if env.CorrelationID != "" {
		t.Errorf("CorrelationID = %q, want empty", env.CorrelationID)
	}
}

func TestNew_AppliesOptions(t *testing.T) {
	env := New(nil, "kscore.test",
		WithMessageID("msg-1"),
		WithCorrelationID("cmd-99"),
		WithPriority(PriorityHigh),
		WithTTL(30*time.Second),
	)
	if env.MessageID != "msg-1" {
		t.Errorf("MessageID = %q, want msg-1", env.MessageID)
	}
	if env.CorrelationID != "cmd-99" {
		t.Errorf("CorrelationID = %q", env.CorrelationID)
	}
	if env.Priority != PriorityHigh {
		t.Errorf("Priority = %q", env.Priority)
	}
	if env.TTLMillis != 30000 {
		t.Errorf("TTLMillis = %d, want 30000", env.TTLMillis)
	}
	if got := env.TTL(); got != 30*time.Second {
		t.Errorf("TTL() = %s, want 30s", got)
	}
}

func TestNew_GeneratesUniqueIDs(t *testing.T) {
	a := New(nil, "kscore.test")
	b := New(nil, "kscore.test")
	if a.MessageID == b.MessageID {
		t.Errorf("two New() calls returned the same MessageID %q", a.MessageID)
	}
}

func TestPriority_Validate(t *testing.T) {
	for _, p := range []Priority{PriorityLow, PriorityNormal, PriorityHigh, PriorityCritical} {
		if err := p.Validate(); err != nil {
			t.Errorf("%q: %v", p, err)
		}
	}
	if err := Priority("urgent").Validate(); err == nil {
		t.Error("urgent: expected error")
	}
	if err := Priority("").Validate(); err == nil {
		t.Error("empty: expected error")
	}
}

func TestEnvelope_Validate(t *testing.T) {
	tests := []struct {
		name    string
		mut     func(*Envelope)
		wantErr string
	}{
		{"defaults ok", func(*Envelope) {}, ""},
		{"empty message id", func(e *Envelope) { e.MessageID = "" }, "message_id"},
		{"empty cluster prefix", func(e *Envelope) { e.ClusterPrefix = "" }, "cluster_prefix"},
		{"bad priority", func(e *Envelope) { e.Priority = "weird" }, "priority"},
		{"negative ttl", func(e *Envelope) { e.TTLMillis = -1 }, "ttl_ms"},
		{"correlation optional", func(e *Envelope) { e.CorrelationID = "" }, ""},
		{"payload optional", func(e *Envelope) { e.Payload = nil }, ""},
		{"message id with null byte", func(e *Envelope) { e.MessageID = "abc\x00def" }, "message_id"},
		{"message id with whitespace", func(e *Envelope) { e.MessageID = "abc def" }, "message_id"},
		{"message id with high bit", func(e *Envelope) { e.MessageID = "abc\x80def" }, "message_id"},
		{"correlation id with null byte", func(e *Envelope) { e.CorrelationID = "abc\x00def" }, "correlation_id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := New([]byte(`{"k":"v"}`), "kscore.test")
			tt.mut(&env)
			err := env.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("Validate = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate = nil, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestEnvelope_MarshalRoundTrip(t *testing.T) {
	original := New([]byte(`{"command":"uptime"}`), "kscore.test",
		WithMessageID("msg-1"),
		WithCorrelationID("cmd-42"),
		WithPriority(PriorityHigh),
		WithTTL(5*time.Minute),
	)

	b, err := original.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	round, err := Unmarshal(b)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if round.MessageID != original.MessageID {
		t.Errorf("MessageID drift: %q -> %q", original.MessageID, round.MessageID)
	}
	if round.CorrelationID != original.CorrelationID {
		t.Errorf("CorrelationID drift: %q", round.CorrelationID)
	}
	if round.Priority != original.Priority {
		t.Errorf("Priority drift: %q", round.Priority)
	}
	if round.TTLMillis != original.TTLMillis {
		t.Errorf("TTL drift: %d", round.TTLMillis)
	}
	if round.ClusterPrefix != original.ClusterPrefix {
		t.Errorf("ClusterPrefix drift: %q", round.ClusterPrefix)
	}
	if string(round.Payload) != string(original.Payload) {
		t.Errorf("Payload drift: %q", round.Payload)
	}
}

func TestEnvelope_MarshalRejectsInvalid(t *testing.T) {
	env := New(nil, "kscore.test")
	env.Priority = "garbage"
	if _, err := env.Marshal(); err == nil {
		t.Error("Marshal accepted invalid envelope")
	}
}

func TestUnmarshal_RejectsInvalidJSON(t *testing.T) {
	if _, err := Unmarshal([]byte("not json")); err == nil {
		t.Error("Unmarshal accepted non-JSON input")
	}
}

func TestUnmarshal_RejectsValidJSONWithBadFields(t *testing.T) {
	// Hand-rolled JSON that parses but fails post-decode validation.
	raw := []byte(`{"message_id":"","priority":"normal","cluster_prefix":"kscore.test"}`)
	if _, err := Unmarshal(raw); err == nil {
		t.Error("Unmarshal accepted envelope with empty message_id")
	}
}

func TestEnvelope_PayloadInlineNotBase64(t *testing.T) {
	// Confirms the wire-format choice: inner JSON inlines, not
	// base64. If this test fails, the json.RawMessage tag was
	// dropped and consumers that expect human-readable payloads
	// will break.
	env := New([]byte(`{"x":1}`), "kscore.test", WithMessageID("m"))
	b, err := env.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(b), `"payload":{"x":1}`) {
		t.Errorf("payload not inlined as JSON: %s", b)
	}
}

func TestEnvelope_OmitsZeroOptionalFields(t *testing.T) {
	// CorrelationID, TTL, Payload are omitempty; absent from wire
	// when zero. Keeps the JSON small for high-volume publishes.
	env := New(nil, "kscore.test", WithMessageID("m"))
	b, err := env.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got := string(b)
	for _, absent := range []string{`"correlation_id"`, `"ttl_ms"`, `"payload"`} {
		if strings.Contains(got, absent) {
			t.Errorf("%s leaked into wire: %s", absent, got)
		}
	}
}

func TestEnvelope_PayloadJSONRawMessageRoundTrip(t *testing.T) {
	// Confirms that a raw byte slice round-trips as JSON, not as a
	// base64 string. This is the load-bearing assertion behind the
	// "human-readable payload" decision.
	type inner struct {
		Cmd string `json:"cmd"`
	}
	innerBytes, err := json.Marshal(inner{Cmd: "uptime"})
	if err != nil {
		t.Fatalf("inner marshal: %v", err)
	}
	env := New(innerBytes, "kscore.test", WithMessageID("m"))
	b, err := env.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	round, err := Unmarshal(b)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	var got inner
	if err := json.Unmarshal(round.Payload, &got); err != nil {
		t.Fatalf("inner unmarshal: %v", err)
	}
	if got.Cmd != "uptime" {
		t.Errorf("Cmd = %q, want uptime", got.Cmd)
	}
}
