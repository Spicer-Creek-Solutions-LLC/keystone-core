// Copyright 2024 Keystone Core Contributors
// SPDX-License-Identifier: Apache-2.0

package logging

import (
	"runtime"
	"testing"
)

func TestDefaultEventLogConfig(t *testing.T) {
	config := DefaultEventLogConfig()
	if config == nil {
		t.Fatal("DefaultEventLogConfig should not return nil")
	}

	if runtime.GOOS == "windows" {
		if config.Source == "" {
			t.Error("expected Source to be set on Windows")
		}
		if config.Log == "" {
			t.Error("expected Log to be set on Windows")
		}
	}
}

func TestNewEventLogOutputOnNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	output, err := NewEventLogOutput(nil)
	if err != ErrNotWindows {
		t.Errorf("expected ErrNotWindows on non-Windows, got %v", err)
	}
	if output != nil {
		t.Error("expected nil output on non-Windows")
	}
}

func TestEventLogOutputWriteOnNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	output := &EventLogOutput{}
	err := output.Write([]byte("test"))
	if err != ErrNotWindows {
		t.Errorf("expected ErrNotWindows on non-Windows, got %v", err)
	}
}

func TestEventLogOutputWriteEntryOnNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	output := &EventLogOutput{}
	err := output.WriteEntry(&Entry{Message: "test"})
	if err != ErrNotWindows {
		t.Errorf("expected ErrNotWindows on non-Windows, got %v", err)
	}
}

func TestEventLogOutputCloseOnNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	output := &EventLogOutput{}
	err := output.Close()
	// Close should be a no-op on non-Windows (not return error)
	if err != nil {
		t.Errorf("expected nil error on Close for non-Windows, got %v", err)
	}
}

func TestInstallEventSourceOnNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	err := InstallEventSource("TestSource", "Application")
	if err != ErrNotWindows {
		t.Errorf("expected ErrNotWindows on non-Windows, got %v", err)
	}
}

func TestRemoveEventSourceOnNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	err := RemoveEventSource("TestSource")
	if err != ErrNotWindows {
		t.Errorf("expected ErrNotWindows on non-Windows, got %v", err)
	}
}

func TestEventSourceExistsOnNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	exists := EventSourceExists("TestSource")
	if exists {
		t.Error("expected false on non-Windows")
	}
}

func TestEventLogEntry(t *testing.T) {
	field := EventLogEntry(EventIDAgentStarted)
	if field.Key != "event_id" {
		t.Errorf("expected key 'event_id', got '%s'", field.Key)
	}
	if field.Value.(EventID) != EventIDAgentStarted {
		t.Errorf("expected value EventIDAgentStarted, got %v", field.Value)
	}
}

func TestEventIDConstants(t *testing.T) {
	// Test that event IDs are in expected ranges
	infoEvents := []EventID{
		EventIDAgentStarted, EventIDAgentStopped, EventIDAgentConnected,
		EventIDAgentRegistered, EventIDCommandExecuted, EventIDStateApplied,
		EventIDHeartbeatSent, EventIDConfigLoaded, EventIDServiceStarted,
		EventIDServiceStopped, EventIDGenericInfo,
	}
	for _, id := range infoEvents {
		if id < 1000 || id >= 2000 {
			t.Errorf("info event ID %d should be in range 1000-1999", id)
		}
	}

	warnEvents := []EventID{
		EventIDConnectionRetry, EventIDHeartbeatMissed, EventIDCommandTimeout,
		EventIDConfigWarning, EventIDResourceLow, EventIDStateChangeDetected,
		EventIDPolicyViolationWarn, EventIDGenericWarning,
	}
	for _, id := range warnEvents {
		if id < 2000 || id >= 3000 {
			t.Errorf("warning event ID %d should be in range 2000-2999", id)
		}
	}

	errorEvents := []EventID{
		EventIDConnectionFailed, EventIDCommandFailed, EventIDStateFailed,
		EventIDConfigError, EventIDNATSError, EventIDAuthenticationError,
		EventIDInternalError, EventIDServiceError, EventIDGenericError,
	}
	for _, id := range errorEvents {
		if id < 3000 || id >= 4000 {
			t.Errorf("error event ID %d should be in range 3000-3999", id)
		}
	}

	securityEvents := []EventID{
		EventIDAuthSuccess, EventIDAuthFailure, EventIDPolicyViolation,
		EventIDAuditLog, EventIDSecurityGeneric,
	}
	for _, id := range securityEvents {
		if id < 4000 || id >= 5000 {
			t.Errorf("security event ID %d should be in range 4000-4999", id)
		}
	}
}

func TestEventLogConfigFields(t *testing.T) {
	config := &EventLogConfig{
		Source:        "TestSource",
		Log:           "Application",
		IncludeFields: true,
		JSONFields:    false,
	}

	if config.Source != "TestSource" {
		t.Errorf("expected Source 'TestSource', got '%s'", config.Source)
	}
	if config.Log != "Application" {
		t.Errorf("expected Log 'Application', got '%s'", config.Log)
	}
	if !config.IncludeFields {
		t.Error("expected IncludeFields to be true")
	}
	if config.JSONFields {
		t.Error("expected JSONFields to be false")
	}
}
