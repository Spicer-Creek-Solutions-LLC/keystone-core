// Copyright 2024 Keystone Core Contributors
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"context"
	"strings"
	"testing"
)

func TestParseProgressEvent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected *progressEvent
	}{
		{
			name:     "not JSON",
			input:    "plain text log line",
			expected: nil,
		},
		{
			name:     "JSON without event",
			input:    `{"message": "hello"}`,
			expected: nil,
		},
		{
			name:  "phase event",
			input: `{"event": "phase", "phase": "installing", "completed": 2, "total": 5}`,
			expected: &progressEvent{
				event:     "phase",
				phase:     "installing",
				completed: 2,
				total:     5,
			},
		},
		{
			name:  "error event",
			input: `{"event": "error", "error": "failed to connect"}`,
			expected: &progressEvent{
				event: "error",
				err:   "failed to connect",
			},
		},
		{
			name:  "complete event",
			input: `{"event": "complete"}`,
			expected: &progressEvent{
				event: "complete",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseProgressEvent(tt.input)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("parseProgressEvent(%q) = %+v, want nil", tt.input, result)
				}
				return
			}
			if result == nil {
				t.Errorf("parseProgressEvent(%q) = nil, want %+v", tt.input, tt.expected)
				return
			}
			if result.event != tt.expected.event {
				t.Errorf("event = %q, want %q", result.event, tt.expected.event)
			}
			if result.phase != tt.expected.phase {
				t.Errorf("phase = %q, want %q", result.phase, tt.expected.phase)
			}
			if result.completed != tt.expected.completed {
				t.Errorf("completed = %d, want %d", result.completed, tt.expected.completed)
			}
			if result.total != tt.expected.total {
				t.Errorf("total = %d, want %d", result.total, tt.expected.total)
			}
			if result.err != tt.expected.err {
				t.Errorf("err = %q, want %q", result.err, tt.expected.err)
			}
		})
	}
}

func TestLineWriter(t *testing.T) {
	ch := make(chan string, 10)
	writer := &lineWriter{ch: ch}

	// Write a complete line
	n, err := writer.Write([]byte("line one\n"))
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != 9 {
		t.Errorf("Write returned %d, want 9", n)
	}

	// Should have sent the line
	select {
	case line := <-ch:
		if line != "line one" {
			t.Errorf("received line = %q, want %q", line, "line one")
		}
	default:
		t.Error("expected to receive a line")
	}

	// Write partial line
	_, _ = writer.Write([]byte("partial"))
	// Should not have sent anything yet
	select {
	case line := <-ch:
		t.Errorf("unexpected line received: %q", line)
	default:
		// expected
	}

	// Complete the line
	_, _ = writer.Write([]byte(" line\n"))
	select {
	case line := <-ch:
		if line != "partial line" {
			t.Errorf("received line = %q, want %q", line, "partial line")
		}
	default:
		t.Error("expected to receive a line")
	}
}

func TestLineWriterMultipleLines(t *testing.T) {
	ch := make(chan string, 10)
	writer := &lineWriter{ch: ch}

	// Write multiple lines at once
	_, err := writer.Write([]byte("line1\nline2\nline3\n"))
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}

	expected := []string{"line1", "line2", "line3"}
	for _, exp := range expected {
		select {
		case line := <-ch:
			if line != exp {
				t.Errorf("received line = %q, want %q", line, exp)
			}
		default:
			t.Errorf("expected to receive line %q", exp)
		}
	}
}

func TestProgressModelHandleLogLine(t *testing.T) {
	model := &progressModel{}

	// Empty line should be ignored
	model.handleLogLine("")
	model.handleLogLine("   ")
	if len(model.logs) != 0 {
		t.Errorf("expected no logs for empty lines, got %d", len(model.logs))
	}

	// Regular log line
	model.handleLogLine("Starting bootstrap")
	if len(model.logs) != 1 {
		t.Errorf("expected 1 log, got %d", len(model.logs))
	}
	if model.logs[0] != "Starting bootstrap" {
		t.Errorf("expected log 'Starting bootstrap', got %q", model.logs[0])
	}

	// Phase event should update phase, not logs
	model.handleLogLine(`{"event": "phase", "phase": "installing", "completed": 1, "total": 3}`)
	if len(model.logs) != 1 {
		t.Errorf("expected 1 log after phase event, got %d", len(model.logs))
	}
	if model.phase != "installing" {
		t.Errorf("expected phase 'installing', got %q", model.phase)
	}
	if model.completed != 1 {
		t.Errorf("expected completed 1, got %d", model.completed)
	}
	if model.total != 3 {
		t.Errorf("expected total 3, got %d", model.total)
	}

	// Error event should set error
	model.handleLogLine(`{"event": "error", "error": "connection failed"}`)
	if model.err == nil {
		t.Error("expected error to be set")
	} else if model.err.Error() != "connection failed" {
		t.Errorf("expected error 'connection failed', got %q", model.err.Error())
	}
}

func TestProgressModelLogTruncation(t *testing.T) {
	model := &progressModel{}

	// Add more than 200 logs
	for i := 0; i < 250; i++ {
		model.handleLogLine("log line")
	}

	if len(model.logs) > 200 {
		t.Errorf("expected logs to be truncated to 200, got %d", len(model.logs))
	}
}

func TestProgressModelVisibleLogs(t *testing.T) {
	model := &progressModel{
		height: 20,
		logs:   make([]string, 0),
	}

	// Add some logs
	for i := 0; i < 30; i++ {
		model.logs = append(model.logs, "log line")
	}

	visible := model.visibleLogs()
	maxLines := model.height - 6
	if len(visible) != maxLines {
		t.Errorf("expected %d visible logs, got %d", maxLines, len(visible))
	}

	// With small height, should return all logs
	model.height = 5
	visible = model.visibleLogs()
	if len(visible) != len(model.logs) {
		t.Errorf("expected all logs for small height, got %d", len(visible))
	}
}

func TestProgressModelView(t *testing.T) {
	model := progressModel{
		phase:     "installing",
		completed: 2,
		total:     5,
	}

	view := model.View()
	if !strings.Contains(view, "Bootstrap progress") {
		t.Error("expected view to contain 'Bootstrap progress'")
	}
	if !strings.Contains(view, "installing") {
		t.Error("expected view to contain phase name")
	}
	if !strings.Contains(view, "2/5") {
		t.Error("expected view to contain progress '2/5'")
	}
}

func TestProgressModelViewDone(t *testing.T) {
	model := progressModel{
		done: true,
	}

	view := model.View()
	if !strings.Contains(view, "complete") {
		t.Error("expected completed view to contain 'complete'")
	}

	// With error
	model.err = context.Canceled
	view = model.View()
	if !strings.Contains(view, "canceled") {
		t.Error("expected canceled view to contain 'canceled'")
	}
}

func TestProgressModelViewWithLogs(t *testing.T) {
	model := progressModel{
		logs:   []string{"log1", "log2"},
		height: 20,
	}

	view := model.View()
	if !strings.Contains(view, "Logs:") {
		t.Error("expected view to contain 'Logs:'")
	}
	if !strings.Contains(view, "log1") {
		t.Error("expected view to contain 'log1'")
	}
	if !strings.Contains(view, "log2") {
		t.Error("expected view to contain 'log2'")
	}
}

func TestProgressEvent(t *testing.T) {
	event := progressEvent{
		event:     "phase",
		phase:     "configuring",
		completed: 3,
		total:     10,
	}

	if event.event != "phase" {
		t.Errorf("expected event 'phase', got %q", event.event)
	}
	if event.phase != "configuring" {
		t.Errorf("expected phase 'configuring', got %q", event.phase)
	}
	if event.completed != 3 {
		t.Errorf("expected completed 3, got %d", event.completed)
	}
	if event.total != 10 {
		t.Errorf("expected total 10, got %d", event.total)
	}
}
