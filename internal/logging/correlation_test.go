// SPDX-License-Identifier: Apache-2.0

package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

func TestCorrelationID_RoundTrip(t *testing.T) {
	ctx := WithCorrelationID(context.Background(), "abc-123")
	if got := CorrelationIDFromContext(ctx); got != "abc-123" {
		t.Errorf("CorrelationIDFromContext = %q, want abc-123", got)
	}
}

func TestCorrelationID_FromEmptyContext(t *testing.T) {
	if got := CorrelationIDFromContext(context.Background()); got != "" {
		t.Errorf("CorrelationIDFromContext(bg) = %q, want empty", got)
	}
}

func TestCorrelationID_InjectedIntoRecord(t *testing.T) {
	buf := &bytes.Buffer{}
	logger, err := New(Options{Level: "info", Format: "json", Output: buf})
	if err != nil {
		t.Fatal(err)
	}

	ctx := WithCorrelationID(context.Background(), "req-xyz")
	logger.InfoContext(ctx, "request received")

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("json: %v\noutput: %s", err, buf.String())
	}
	if rec["correlation_id"] != "req-xyz" {
		t.Errorf("correlation_id = %v, want req-xyz", rec["correlation_id"])
	}
}

func TestCorrelationID_PreservedAcrossWithAttrs(t *testing.T) {
	buf := &bytes.Buffer{}
	logger, _ := New(Options{Level: "info", Format: "json", Output: buf})

	scoped := logger.With("component", "api")
	ctx := WithCorrelationID(context.Background(), "scoped-1")
	scoped.InfoContext(ctx, "hello")

	var rec map[string]any
	_ = json.Unmarshal(buf.Bytes(), &rec)
	if rec["correlation_id"] != "scoped-1" {
		t.Errorf("correlation_id not preserved through With: %v", rec["correlation_id"])
	}
	if rec["component"] != "api" {
		t.Errorf("component attr lost: %v", rec["component"])
	}
}

// With WithGroup, slog's record-level attrs are nested under the active
// group — this includes our correlation_id. The test documents this
// behavior; aggregators wanting top-level correlation_id should pull it
// from the group key when present.
func TestCorrelationID_NestedUnderWithGroup(t *testing.T) {
	buf := &bytes.Buffer{}
	logger, _ := New(Options{Level: "info", Format: "json", Output: buf})

	grouped := logger.WithGroup("svc")
	ctx := WithCorrelationID(context.Background(), "grouped-1")
	grouped.InfoContext(ctx, "hello", "phase", "init")

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("json: %v\noutput: %s", err, buf.String())
	}
	svc, ok := rec["svc"].(map[string]any)
	if !ok {
		t.Fatalf("svc group missing: %s", buf.String())
	}
	if svc["correlation_id"] != "grouped-1" {
		t.Errorf("correlation_id not in svc group: %v", svc["correlation_id"])
	}
	if svc["phase"] != "init" {
		t.Errorf("phase not in svc group: %v", svc["phase"])
	}
}

func TestNewCorrelationID(t *testing.T) {
	a := NewCorrelationID()
	b := NewCorrelationID()
	if a == "" || b == "" {
		t.Fatalf("NewCorrelationID returned empty: %q %q", a, b)
	}
	if a == b {
		t.Errorf("NewCorrelationID not unique: %q == %q", a, b)
	}
	if len(a) != 32 {
		t.Errorf("NewCorrelationID length = %d, want 32 (hex of 16 bytes)", len(a))
	}
}
