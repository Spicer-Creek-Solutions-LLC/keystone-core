// Copyright 2024 Keystone Core Contributors
// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/events"
)

func TestViewConstants(t *testing.T) {
	// Verify the view constants have expected values
	tests := []struct {
		name     string
		view     View
		expected View
	}{
		{"ViewDashboard", ViewDashboard, 0},
		{"ViewAgents", ViewAgents, 1},
		{"ViewEvents", ViewEvents, 2},
		{"ViewStateDrift", ViewStateDrift, 3},
		{"ViewPolicyViolations", ViewPolicyViolations, 4},
		{"ViewJobs", ViewJobs, 5},
		{"ViewLogs", ViewLogs, 6},
		{"ViewMetrics", ViewMetrics, 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.view != tt.expected {
				t.Errorf("%s = %d, want %d", tt.name, tt.view, tt.expected)
			}
		})
	}
}

func TestFormatUptime(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{"zero duration", 0, "0d 0h 0m"},
		{"one minute", time.Minute, "0d 0h 1m"},
		{"one hour", time.Hour, "0d 1h 0m"},
		{"24 hours", 24 * time.Hour, "1d 0h 0m"},
		{"complex duration", 25*time.Hour + 30*time.Minute, "1d 1h 30m"},
		{"multiple days", 50*time.Hour + 15*time.Minute, "2d 2h 15m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatUptime(tt.duration)
			if result != tt.expected {
				t.Errorf("formatUptime(%v) = %q, want %q", tt.duration, result, tt.expected)
			}
		})
	}
}

func TestDashboardModelAddEvent(t *testing.T) {
	model := &DashboardModel{}

	// Add events
	for i := 0; i < 15; i++ {
		model.AddEvent(&events.Event{
			Type:     events.EventTypeAgentConnect,
			Severity: events.SeverityInfo,
			Time:     time.Now(),
			Source:   "test",
		})
	}

	// Should be capped at maxRecentEvents
	if len(model.recentEvents) > maxRecentEvents {
		t.Errorf("expected at most %d events, got %d", maxRecentEvents, len(model.recentEvents))
	}
}

func TestDashboardModelViewEmpty(t *testing.T) {
	model := &DashboardModel{
		width: 0, // Trigger loading message
	}

	view := model.View()
	if !strings.Contains(view, "Loading") {
		t.Errorf("expected loading message when width is 0, got %q", view)
	}
}

func TestDashboardModelViewWithError(t *testing.T) {
	model := &DashboardModel{
		width: 100,
		err:   errors.New("connection failed"),
	}

	view := model.View()
	if !strings.Contains(view, "Error") {
		t.Errorf("expected error message, got %q", view)
	}
	if !strings.Contains(view, "connection failed") {
		t.Errorf("expected error details, got %q", view)
	}
}

func TestDashboardModelViewWithData(t *testing.T) {
	model := &DashboardModel{
		width:           100,
		height:          50,
		uptime:          "1d 2h 30m",
		version:         "1.0.0",
		agentsConnected: 5,
		agentsTotal:     10,
		jobsRunning:     2,
		jobsCompleted:   100,
		jobsFailed:      3,
	}

	view := model.View()
	if !strings.Contains(view, "System") {
		t.Error("expected System section")
	}
	if !strings.Contains(view, "Agents") {
		t.Error("expected Agents section")
	}
	if !strings.Contains(view, "Jobs") {
		t.Error("expected Jobs section")
	}
	if !strings.Contains(view, "1.0.0") {
		t.Error("expected version in output")
	}
}

func TestModelViewNotReady(t *testing.T) {
	model := &Model{
		ready: false,
	}

	view := model.View()
	if !strings.Contains(view, "Initializing") {
		t.Errorf("expected initializing message when not ready, got %q", view)
	}
}

func TestTickCmd(t *testing.T) {
	cmd := tickCmd(1)
	if cmd == nil {
		t.Error("expected tickCmd to return a non-nil command")
	}
}

func TestMaxRecentEvents(t *testing.T) {
	if maxRecentEvents != 10 {
		t.Errorf("expected maxRecentEvents to be 10, got %d", maxRecentEvents)
	}
}
