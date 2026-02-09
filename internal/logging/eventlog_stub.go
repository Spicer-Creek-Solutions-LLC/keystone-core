// Copyright 2024 Keystone Core Contributors
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package logging

import "errors"

// ErrNotWindows is returned when Windows Event Log functions are called on non-Windows
var ErrNotWindows = errors.New("windows event log is only available on Windows")

// EventID represents Windows Event Log event IDs (stub for non-Windows)
type EventID uint32

// Event IDs (stubs for cross-platform compilation)
const (
	EventIDAgentStarted    EventID = 1001
	EventIDAgentStopped    EventID = 1002
	EventIDAgentConnected  EventID = 1003
	EventIDAgentRegistered EventID = 1004
	EventIDCommandExecuted EventID = 1005
	EventIDStateApplied    EventID = 1006
	EventIDHeartbeatSent   EventID = 1007
	EventIDConfigLoaded    EventID = 1008
	EventIDServiceStarted  EventID = 1009
	EventIDServiceStopped  EventID = 1010
	EventIDGenericInfo     EventID = 1099

	EventIDConnectionRetry     EventID = 2001
	EventIDHeartbeatMissed     EventID = 2002
	EventIDCommandTimeout      EventID = 2003
	EventIDConfigWarning       EventID = 2004
	EventIDResourceLow         EventID = 2005
	EventIDStateChangeDetected EventID = 2006
	EventIDPolicyViolationWarn EventID = 2007
	EventIDGenericWarning      EventID = 2099

	EventIDConnectionFailed    EventID = 3001
	EventIDCommandFailed       EventID = 3002
	EventIDStateFailed         EventID = 3003
	EventIDConfigError         EventID = 3004
	EventIDNATSError           EventID = 3005
	EventIDAuthenticationError EventID = 3006
	EventIDInternalError       EventID = 3007
	EventIDServiceError        EventID = 3008
	EventIDGenericError        EventID = 3099

	EventIDAuthSuccess     EventID = 4001
	EventIDAuthFailure     EventID = 4002
	EventIDPolicyViolation EventID = 4003
	EventIDAuditLog        EventID = 4004
	EventIDSecurityGeneric EventID = 4099
)

// EventLogConfig is a stub for non-Windows platforms
type EventLogConfig struct {
	Source        string
	Log           string
	IncludeFields bool
	JSONFields    bool
}

// DefaultEventLogConfig returns a stub configuration
func DefaultEventLogConfig() *EventLogConfig {
	return &EventLogConfig{}
}

// EventLogOutput is a stub for non-Windows platforms
type EventLogOutput struct{}

// NewEventLogOutput returns an error on non-Windows platforms
func NewEventLogOutput(config *EventLogConfig) (*EventLogOutput, error) {
	return nil, ErrNotWindows
}

// Write is not available on non-Windows platforms
func (o *EventLogOutput) Write(data []byte) error {
	return ErrNotWindows
}

// WriteEntry is not available on non-Windows platforms
func (o *EventLogOutput) WriteEntry(entry *Entry) error {
	return ErrNotWindows
}

// Close is a no-op on non-Windows platforms
func (o *EventLogOutput) Close() error {
	return nil
}

// InstallEventSource is not available on non-Windows platforms
func InstallEventSource(source, logName string) error {
	return ErrNotWindows
}

// RemoveEventSource is not available on non-Windows platforms
func RemoveEventSource(source string) error {
	return ErrNotWindows
}

// EventSourceExists returns false on non-Windows platforms
func EventSourceExists(source string) bool {
	return false
}

// EventLogEntry creates a log entry with a specific event ID
func EventLogEntry(eventID EventID) Field {
	return Field{Key: "event_id", Value: eventID}
}
