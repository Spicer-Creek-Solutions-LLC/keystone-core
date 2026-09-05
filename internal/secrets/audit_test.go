// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
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

// ---- MultiAuditor ------------------------------------------------

func TestMultiAuditor_FansOut(t *testing.T) {
	t.Parallel()
	a := &recordingTestAuditor{}
	b := &recordingTestAuditor{}
	c := &recordingTestAuditor{}
	m := NewMultiAuditor(a, b, c)

	evt := SecretAccessEvent{Action: ActionGetSecret, Path: "kv/x", Allowed: true}
	m.Emit(context.Background(), evt)

	for i, rec := range []*recordingTestAuditor{a, b, c} {
		if got := rec.count(); got != 1 {
			t.Errorf("auditor[%d] count = %d, want 1", i, got)
		}
	}
	if m.Len() != 3 {
		t.Errorf("MultiAuditor.Len = %d, want 3", m.Len())
	}
}

func TestMultiAuditor_NilInnerSkipped(t *testing.T) {
	t.Parallel()
	rec := &recordingTestAuditor{}
	m := NewMultiAuditor(nil, rec, nil)
	if m.Len() != 1 {
		t.Errorf("Len = %d, want 1 (nil entries skipped)", m.Len())
	}
	// Emit must not panic.
	m.Emit(context.Background(), SecretAccessEvent{})
	if rec.count() != 1 {
		t.Errorf("recording auditor count = %d, want 1", rec.count())
	}
}

func TestMultiAuditor_Empty(t *testing.T) {
	t.Parallel()
	m := NewMultiAuditor()
	// Zero auditors must not panic on emit.
	m.Emit(context.Background(), SecretAccessEvent{})
	if m.Len() != 0 {
		t.Errorf("empty MultiAuditor Len = %d, want 0", m.Len())
	}
}

// ---- BufferedAuditor ---------------------------------------------

func TestNewBufferedAuditor_Validation(t *testing.T) {
	t.Parallel()
	for _, n := range []int{0, -1, -100} {
		_, err := NewBufferedAuditor(n)
		if err == nil {
			t.Errorf("NewBufferedAuditor(%d) = nil err", n)
			continue
		}
		if !errors.Is(err, ErrInvalidBackend) {
			t.Errorf("NewBufferedAuditor(%d) err does not wrap ErrInvalidBackend: %v", n, err)
		}
	}
}

func TestBufferedAuditor_FIFO(t *testing.T) {
	t.Parallel()
	b, err := NewBufferedAuditor(3)
	if err != nil {
		t.Fatalf("NewBufferedAuditor: %v", err)
	}
	if b.Capacity() != 3 {
		t.Errorf("Capacity = %d, want 3", b.Capacity())
	}

	for i := 1; i <= 5; i++ {
		b.Emit(context.Background(), SecretAccessEvent{Path: "kv/" + itoa(i)})
	}
	if b.Len() != 3 {
		t.Errorf("Len = %d, want 3 (capacity)", b.Len())
	}

	snap := b.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("Snapshot len = %d, want 3", len(snap))
	}
	wantPaths := []string{"kv/3", "kv/4", "kv/5"}
	for i, want := range wantPaths {
		if snap[i].Path != want {
			t.Errorf("Snapshot[%d].Path = %q, want %q (FIFO oldest-first)", i, snap[i].Path, want)
		}
	}
}

func TestBufferedAuditor_SnapshotIsDefensiveCopy(t *testing.T) {
	t.Parallel()
	b, _ := NewBufferedAuditor(2)
	b.Emit(context.Background(), SecretAccessEvent{Path: "kv/a"})
	b.Emit(context.Background(), SecretAccessEvent{Path: "kv/b"})

	snap := b.Snapshot()
	snap[0].Path = "MUTATED"

	again := b.Snapshot()
	if again[0].Path != "kv/a" {
		t.Errorf("Snapshot mutation leaked into buffer: %q", again[0].Path)
	}
}

func TestBufferedAuditor_NotFilled(t *testing.T) {
	t.Parallel()
	b, _ := NewBufferedAuditor(10)
	b.Emit(context.Background(), SecretAccessEvent{Path: "kv/x"})
	b.Emit(context.Background(), SecretAccessEvent{Path: "kv/y"})
	if b.Len() != 2 {
		t.Errorf("Len = %d, want 2 (buffer not yet at capacity)", b.Len())
	}
}

func TestBufferedAuditor_Concurrent(t *testing.T) {
	t.Parallel()
	b, _ := NewBufferedAuditor(100)
	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(2)
		i := i
		go func() {
			defer wg.Done()
			b.Emit(context.Background(), SecretAccessEvent{Path: itoa(i)})
		}()
		go func() {
			defer wg.Done()
			_ = b.Snapshot()
		}()
	}
	wg.Wait()
	if b.Len() != n {
		t.Errorf("after N=%d emits, Len = %d", n, b.Len())
	}
}

// ---- SamplingAuditor ---------------------------------------------

func TestNewSamplingAuditor_Validation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		inner    Auditor
		fraction float64
		wantErr  bool
	}{
		{nil, 0.5, true},
		{NoopAuditor{}, -0.1, true},
		{NoopAuditor{}, 1.1, true},
		{NoopAuditor{}, 0.5, false},
		{NoopAuditor{}, 0, false},
		{NoopAuditor{}, 1, false},
	}
	for _, tc := range cases {
		_, err := NewSamplingAuditor(tc.inner, tc.fraction)
		if (err != nil) != tc.wantErr {
			t.Errorf("NewSamplingAuditor(%T, %v) err=%v, wantErr=%v", tc.inner, tc.fraction, err, tc.wantErr)
		}
		if tc.wantErr && err != nil && !errors.Is(err, ErrInvalidBackend) {
			t.Errorf("err does not wrap ErrInvalidBackend: %v", err)
		}
	}
}

func TestSamplingAuditor_Fraction1PassesAll(t *testing.T) {
	t.Parallel()
	rec := &recordingTestAuditor{}
	s, err := NewSamplingAuditor(rec, 1.0)
	if err != nil {
		t.Fatalf("NewSamplingAuditor: %v", err)
	}
	for i := 0; i < 100; i++ {
		s.Emit(context.Background(), SecretAccessEvent{Action: ActionGetSecret, Allowed: true})
	}
	if rec.count() != 100 {
		t.Errorf("count = %d, want 100 (fraction=1 passes everything)", rec.count())
	}
}

func TestSamplingAuditor_Fraction0DropsSuccessKeepsFailure(t *testing.T) {
	t.Parallel()
	rec := &recordingTestAuditor{}
	s, _ := NewSamplingAuditor(rec, 0)

	// 100 successful events — all dropped.
	for i := 0; i < 100; i++ {
		s.Emit(context.Background(), SecretAccessEvent{Allowed: true})
	}
	if rec.count() != 0 {
		t.Errorf("count after 100 success events with fraction=0 = %d, want 0", rec.count())
	}

	// 10 failures — all pass through (carve-out for compliance).
	for i := 0; i < 10; i++ {
		s.Emit(context.Background(), SecretAccessEvent{Allowed: false})
	}
	if rec.count() != 10 {
		t.Errorf("count after 10 failures with fraction=0 = %d, want 10 (failures always emit)", rec.count())
	}
}

func TestSamplingAuditor_Fraction05Approximate(t *testing.T) {
	t.Parallel()
	rec := &recordingTestAuditor{}
	s, _ := NewSamplingAuditor(rec, 0.5)

	const n = 1000
	for i := 0; i < n; i++ {
		s.Emit(context.Background(), SecretAccessEvent{Allowed: true})
	}
	got := rec.count()
	// 99.9% confidence band on a Binomial(1000, 0.5) ~= [~430, ~570].
	// Use a wider band to keep the test stable against PRNG variance.
	if got < 380 || got > 620 {
		t.Errorf("count after 1000 successes with fraction=0.5 = %d, want roughly 500 (±120)", got)
	}
}

// ---- SecretAccessEvent wire-shape stability ----------------------

// TestSecretAccessEvent_WireShape pins the JSON field names. Epic 12
// will deserialize these on the audit-query side; a future PR that
// renames a field needs to flip the wire test deliberately.
func TestSecretAccessEvent_WireShape(t *testing.T) {
	t.Parallel()
	evt := SecretAccessEvent{
		Timestamp:     time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC),
		Action:        ActionWriteSecret,
		Path:          "kv/app/db",
		LeaseID:       "lease-xyz",
		Backend:       "vault",
		Principal:     Principal{AgentID: "agent-1", SPIFFEID: "spiffe://kscore.local/agent/agent-1", User: "alice"},
		Allowed:       true,
		ErrorReason:   "",
		Duration:      120 * time.Millisecond,
		MaskedPayload: map[string]any{"password": "***"},
	}
	raw, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got := string(raw)
	// Pin each canonical field name — any future shape change must
	// flip this test deliberately.
	for _, want := range []string{
		`"timestamp":`, `"action":`, `"path":`, `"lease_id":`, `"backend":`,
		`"principal":`, `"agent_id":`, `"spiffe_id":`, `"user":`,
		`"allowed":`, `"duration":`, `"masked_payload":`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("wire shape missing %q\n%s", want, got)
		}
	}
}

// ---- helpers -----------------------------------------------------

type recordingTestAuditor struct {
	mu     sync.Mutex
	events []SecretAccessEvent
}

func (a *recordingTestAuditor) Emit(_ context.Context, e SecretAccessEvent) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, e)
}

func (a *recordingTestAuditor) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.events)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	digits := "0123456789"
	out := ""
	for i > 0 {
		out = string(digits[i%10]) + out
		i /= 10
	}
	return out
}

func TestSecretAccessEvent_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	in := SecretAccessEvent{
		Timestamp:     time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC),
		Action:        ActionWriteSecret,
		Path:          "kv/app/db",
		Backend:       "file",
		Principal:     Principal{AgentID: "agent-1", SPIFFEID: "spiffe://kscore.local/agent/agent-1"},
		Allowed:       true,
		Duration:      200 * time.Millisecond,
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
