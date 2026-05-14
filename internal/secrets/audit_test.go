package secrets

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestNoopAuditor_Discards(t *testing.T) {
	t.Parallel()
	NoopAuditor{}.Emit(context.Background(), SecretAccessEvent{Action: ActionGetSecret})
	// No panic, no return — that's the contract.
}

func TestDefaultAuditor_IsNoop(t *testing.T) {
	t.Parallel()
	a := DefaultAuditor()
	if a == nil {
		t.Fatalf("DefaultAuditor returned nil")
	}
	a.Emit(context.Background(), SecretAccessEvent{})
}

func TestLogAuditor_NilLogger_FallsBackToDefault(t *testing.T) {
	t.Parallel()
	a := NewLogAuditor(nil)
	if a.Logger == nil {
		t.Fatalf("NewLogAuditor(nil).Logger is nil; want fallback to slog.Default")
	}
}

func TestLogAuditor_EmitWritesINFO(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	a := NewLogAuditor(logger)

	a.Emit(context.Background(), SecretAccessEvent{
		Timestamp: time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC),
		Action:    ActionGetSecret,
		Path:      "kv/app/db",
		Backend:   "file",
		Principal: Principal{AgentID: "agent-1", SPIFFEID: "spiffe://kscore.local/agent/agent-1"},
		Allowed:   true,
		Duration:  120 * time.Microsecond,
	})

	line := buf.String()
	if !strings.Contains(line, `"msg":"secret.access"`) {
		t.Errorf("log line missing msg=secret.access: %s", line)
	}
	if !strings.Contains(line, `"action":"get_secret"`) {
		t.Errorf("log line missing action field: %s", line)
	}
	if !strings.Contains(line, `"path":"kv/app/db"`) {
		t.Errorf("log line missing path field: %s", line)
	}
	if !strings.Contains(line, `"agent_id":"agent-1"`) {
		t.Errorf("log line missing agent_id: %s", line)
	}
	if !strings.Contains(line, `"allowed":true`) {
		t.Errorf("log line missing allowed=true: %s", line)
	}
}

func TestLogAuditor_MasksPayload(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	a := NewLogAuditor(logger)

	original := map[string]any{"password": "hunter2", "username": "alice"}
	masked := maskMap(original)

	a.Emit(context.Background(), SecretAccessEvent{
		Action:        ActionWriteSecret,
		Path:          "kv/app/db",
		Backend:       "file",
		Allowed:       true,
		MaskedPayload: masked,
	})

	line := buf.String()
	if strings.Contains(line, "hunter2") {
		t.Fatalf("LogAuditor leaked cleartext: %s", line)
	}
	if strings.Contains(line, "alice") {
		t.Fatalf("LogAuditor leaked cleartext (username): %s", line)
	}
	if !strings.Contains(line, MaskedValue) {
		t.Errorf("LogAuditor did not include masked sentinel: %s", line)
	}
}

func TestLogAuditor_OptionalFieldsOmitted(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	a := NewLogAuditor(logger)

	a.Emit(context.Background(), SecretAccessEvent{
		Action:  ActionGetSecret,
		Backend: "file",
		Allowed: true,
	})

	line := buf.String()
	for _, field := range []string{`"path":`, `"lease_id":`, `"error":`, `"masked_payload":`} {
		if strings.Contains(line, field) {
			t.Errorf("optional field %s leaked into log line: %s", field, line)
		}
	}
}

func TestSecretAccessEvent_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	in := SecretAccessEvent{
		Timestamp:   time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC),
		Action:      ActionWriteSecret,
		Path:        "kv/app/db",
		Backend:     "file",
		Principal:   Principal{AgentID: "agent-1", SPIFFEID: "spiffe://kscore.local/agent/agent-1"},
		Allowed:     true,
		Duration:    200 * time.Millisecond,
		MaskedPayload: map[string]any{"password": MaskedValue},
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var out SecretAccessEvent
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.Action != in.Action || out.Path != in.Path || out.Backend != in.Backend {
		t.Errorf("round-trip mismatch:\nin:  %#v\nout: %#v", in, out)
	}
	if out.Principal.AgentID != in.Principal.AgentID {
		t.Errorf("Principal mismatch: in=%#v out=%#v", in.Principal, out.Principal)
	}
	if out.MaskedPayload["password"] != MaskedValue {
		t.Errorf("MaskedPayload lost: %#v", out.MaskedPayload)
	}
}
