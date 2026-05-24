// SPDX-License-Identifier: Apache-2.0

package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestNew_BadLevel(t *testing.T) {
	if _, err := New(Options{Level: "trace", Format: "json"}); err == nil {
		t.Error("expected error for unknown level")
	}
}

func TestNew_BadFormat(t *testing.T) {
	if _, err := New(Options{Level: "info", Format: "xml"}); err == nil {
		t.Error("expected error for unknown format")
	}
}

func TestNew_DefaultOutput(t *testing.T) {
	// Output==nil should fall back to stdout without panic. We don't capture
	// stdout here — just verify construction succeeds.
	if _, err := New(Options{Level: "info", Format: "json"}); err != nil {
		t.Errorf("New with nil Output: %v", err)
	}
}

func TestNew_JSONFormat(t *testing.T) {
	buf := &bytes.Buffer{}
	logger, err := New(Options{Level: "info", Format: "json", Output: buf})
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("hello", "shape", "round")

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("json: %v\noutput: %s", err, buf.String())
	}
	if rec["msg"] != "hello" {
		t.Errorf("msg = %v, want hello", rec["msg"])
	}
	if rec["shape"] != "round" {
		t.Errorf("shape = %v, want round", rec["shape"])
	}
	if rec["level"] != "INFO" {
		t.Errorf("level = %v, want INFO", rec["level"])
	}
}

func TestNew_LogfmtFormat(t *testing.T) {
	buf := &bytes.Buffer{}
	logger, _ := New(Options{Level: "info", Format: "logfmt", Output: buf})
	logger.Info("hello", "shape", "round")

	out := buf.String()
	if !strings.Contains(out, "msg=hello") {
		t.Errorf("output missing msg=hello: %s", out)
	}
	if !strings.Contains(out, "shape=round") {
		t.Errorf("output missing shape=round: %s", out)
	}
	if !strings.Contains(out, "level=INFO") {
		t.Errorf("output missing level=INFO: %s", out)
	}
}

func TestNew_TextFormat_NoFractionalSeconds(t *testing.T) {
	buf := &bytes.Buffer{}
	logger, _ := New(Options{Level: "info", Format: "text", Output: buf})
	logger.Info("hello")

	out := buf.String()
	if !strings.Contains(out, "msg=hello") {
		t.Errorf("output missing msg: %s", out)
	}
	// text format trims sub-second precision: time should NOT contain "."
	// (RFC3339 without nano has no decimal point in the seconds segment).
	timeIdx := strings.Index(out, "time=")
	if timeIdx < 0 {
		t.Fatalf("output missing time= : %s", out)
	}
	// Take up to the next space.
	rest := out[timeIdx+len("time="):]
	if sp := strings.IndexByte(rest, ' '); sp > 0 {
		rest = rest[:sp]
	}
	if strings.Contains(rest, ".") {
		t.Errorf("text format time has fractional seconds: %q", rest)
	}
}

func TestNew_LevelFiltering(t *testing.T) {
	buf := &bytes.Buffer{}
	logger, _ := New(Options{Level: "warn", Format: "json", Output: buf})
	logger.Debug("d")
	logger.Info("i")
	logger.Warn("w")
	logger.Error("e")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 records (warn, error), got %d:\n%s", len(lines), buf.String())
	}
	for _, line := range lines {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("json: %v", err)
		}
		lvl, _ := rec["level"].(string)
		if lvl != "WARN" && lvl != "ERROR" {
			t.Errorf("unexpected level: %q", lvl)
		}
	}
}

func TestNew_NoCorrelation_WithoutContext(t *testing.T) {
	buf := &bytes.Buffer{}
	logger, _ := New(Options{Level: "info", Format: "json", Output: buf})

	// Logger.Info uses context.Background() implicitly; no correlation_id.
	logger.Info("hello")
	if strings.Contains(buf.String(), "correlation_id") {
		t.Errorf("unexpected correlation_id in output: %s", buf.String())
	}

	// InfoContext with a bare ctx also has no correlation_id.
	buf.Reset()
	logger.InfoContext(context.Background(), "hello")
	if strings.Contains(buf.String(), "correlation_id") {
		t.Errorf("unexpected correlation_id in output: %s", buf.String())
	}
}
