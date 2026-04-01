// Copyright 2024 Spicer Creek Solutions LLC
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package logging

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/sys/windows/svc/eventlog"
)

const (
	// DefaultEventSource is the default Windows Event Log source name
	DefaultEventSource = "KeystoneCore"

	// DefaultEventLog is the default Windows Event Log to write to
	DefaultEventLog = "Application"
)

// EventID represents Windows Event Log event IDs
// Using a scheme where:
// - 1xxx: Informational events
// - 2xxx: Warning events
// - 3xxx: Error events
// - 4xxx: Security/audit events
type EventID uint32

const (
	// Informational events (1xxx)
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

	// Warning events (2xxx)
	EventIDConnectionRetry     EventID = 2001
	EventIDHeartbeatMissed     EventID = 2002
	EventIDCommandTimeout      EventID = 2003
	EventIDConfigWarning       EventID = 2004
	EventIDResourceLow         EventID = 2005
	EventIDStateChangeDetected EventID = 2006
	EventIDPolicyViolationWarn EventID = 2007
	EventIDGenericWarning      EventID = 2099

	// Error events (3xxx)
	EventIDConnectionFailed    EventID = 3001
	EventIDCommandFailed       EventID = 3002
	EventIDStateFailed         EventID = 3003
	EventIDConfigError         EventID = 3004
	EventIDNATSError           EventID = 3005
	EventIDAuthenticationError EventID = 3006
	EventIDInternalError       EventID = 3007
	EventIDServiceError        EventID = 3008
	EventIDGenericError        EventID = 3099

	// Security/audit events (4xxx)
	EventIDAuthSuccess     EventID = 4001
	EventIDAuthFailure     EventID = 4002
	EventIDPolicyViolation EventID = 4003
	EventIDAuditLog        EventID = 4004
	EventIDSecurityGeneric EventID = 4099
)

// EventLogConfig contains configuration for Windows Event Log output
type EventLogConfig struct {
	// Source is the event source name (appears in Event Viewer)
	Source string

	// Log is the event log name (Application, System, or custom)
	Log string

	// IncludeFields includes structured fields in event message
	IncludeFields bool

	// JSONFields formats fields as JSON instead of key=value
	JSONFields bool
}

// DefaultEventLogConfig returns the default Event Log configuration
func DefaultEventLogConfig() *EventLogConfig {
	return &EventLogConfig{
		Source:        DefaultEventSource,
		Log:           DefaultEventLog,
		IncludeFields: true,
		JSONFields:    false,
	}
}

// EventLogOutput writes log entries to Windows Event Log
type EventLogOutput struct {
	log    *eventlog.Log
	config *EventLogConfig
	mu     sync.Mutex
}

// NewEventLogOutput creates a new EventLogOutput
// The event source must be registered before use (see InstallEventSource)
func NewEventLogOutput(config *EventLogConfig) (*EventLogOutput, error) {
	if config == nil {
		config = DefaultEventLogConfig()
	}

	log, err := eventlog.Open(config.Source)
	if err != nil {
		return nil, fmt.Errorf("failed to open event log: %w", err)
	}

	return &EventLogOutput{
		log:    log,
		config: config,
	}, nil
}

// Write writes a formatted log entry to the Windows Event Log
// This method expects the data to be a JSON-encoded Entry
func (o *EventLogOutput) Write(data []byte) error {
	// Parse the entry from JSON
	var entry Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		// If we can't parse it, just write the raw data
		return o.writeRaw(string(data), LevelInfo)
	}

	return o.WriteEntry(&entry)
}

// WriteEntry writes a log entry to the Windows Event Log
func (o *EventLogOutput) WriteEntry(entry *Entry) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	// Build the message
	msg := o.formatMessage(entry)

	// Determine the event type and ID
	eventType := o.levelToEventType(entry.Level)
	eventID := o.getEventID(entry)

	// Write to event log
	switch eventType {
	case eventlog.Error:
		return o.log.Error(uint32(eventID), msg)
	case eventlog.Warning:
		return o.log.Warning(uint32(eventID), msg)
	default:
		return o.log.Info(uint32(eventID), msg)
	}
}

// writeRaw writes a raw string to the event log
func (o *EventLogOutput) writeRaw(msg string, level Level) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	switch level {
	case LevelError:
		return o.log.Error(uint32(EventIDGenericError), msg)
	case LevelWarn:
		return o.log.Warning(uint32(EventIDGenericWarning), msg)
	default:
		return o.log.Info(uint32(EventIDGenericInfo), msg)
	}
}

// formatMessage formats a log entry for Windows Event Log
func (o *EventLogOutput) formatMessage(entry *Entry) string {
	var sb strings.Builder

	// Main message
	sb.WriteString(entry.Message)

	// Add correlation ID if present
	if entry.CorrelationID != "" {
		sb.WriteString("\r\nCorrelation ID: ")
		sb.WriteString(entry.CorrelationID)
	}

	// Add fields if configured
	if o.config.IncludeFields && len(entry.Fields) > 0 {
		sb.WriteString("\r\n\r\nDetails:\r\n")

		if o.config.JSONFields {
			// Format as JSON
			fieldsJSON, err := json.MarshalIndent(entry.Fields, "", "  ")
			if err == nil {
				sb.Write(fieldsJSON)
			}
		} else {
			// Format as key=value pairs
			for key, value := range entry.Fields {
				sb.WriteString(fmt.Sprintf("  %s: %v\r\n", key, value))
			}
		}
	}

	// Add metadata if present
	if entry.Metadata != nil {
		sb.WriteString("\r\nMetadata:\r\n")
		if entry.Metadata.Host != "" {
			sb.WriteString(fmt.Sprintf("  Host: %s\r\n", entry.Metadata.Host))
		}
		if entry.Metadata.Service != "" {
			sb.WriteString(fmt.Sprintf("  Service: %s\r\n", entry.Metadata.Service))
		}
		if entry.Metadata.Version != "" {
			sb.WriteString(fmt.Sprintf("  Version: %s\r\n", entry.Metadata.Version))
		}
		if entry.Metadata.PID != 0 {
			sb.WriteString(fmt.Sprintf("  PID: %d\r\n", entry.Metadata.PID))
		}
		if entry.Metadata.Caller != "" {
			sb.WriteString(fmt.Sprintf("  Caller: %s\r\n", entry.Metadata.Caller))
		}
	}

	return sb.String()
}

// levelToEventType converts a log level to Windows event type
func (o *EventLogOutput) levelToEventType(level Level) uint16 {
	switch level {
	case LevelError:
		return eventlog.Error
	case LevelWarn:
		return eventlog.Warning
	default:
		return eventlog.Info
	}
}

// getEventID determines the event ID based on the entry content
func (o *EventLogOutput) getEventID(entry *Entry) EventID {
	// Try to get a specific event ID from fields
	if eventIDVal, ok := entry.Fields["event_id"]; ok {
		if eid, ok := eventIDVal.(EventID); ok {
			return eid
		}
		if eid, ok := eventIDVal.(uint32); ok {
			return EventID(eid)
		}
		if eid, ok := eventIDVal.(int); ok {
			return EventID(eid)
		}
	}

	// Fall back to generic event IDs based on level
	switch entry.Level {
	case LevelError:
		return EventIDGenericError
	case LevelWarn:
		return EventIDGenericWarning
	default:
		return EventIDGenericInfo
	}
}

// Close closes the event log handle
func (o *EventLogOutput) Close() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.log != nil {
		return o.log.Close()
	}
	return nil
}

// InstallEventSource registers the event source with Windows
// This should be called during service installation
func InstallEventSource(source, logName string) error {
	if source == "" {
		source = DefaultEventSource
	}
	if logName == "" {
		logName = DefaultEventLog
	}

	// Install the event source
	err := eventlog.InstallAsEventCreate(source, eventlog.Error|eventlog.Warning|eventlog.Info)
	if err != nil {
		// If the error indicates the source already exists, this is not an error
		// On Windows, this returns an error if the source exists
		return fmt.Errorf("failed to install event source: %w", err)
	}

	return nil
}

// RemoveEventSource removes the event source from Windows
// This should be called during service uninstallation
func RemoveEventSource(source string) error {
	if source == "" {
		source = DefaultEventSource
	}

	return eventlog.Remove(source)
}

// EventSourceExists checks if the event source is registered
func EventSourceExists(source string) bool {
	if source == "" {
		source = DefaultEventSource
	}

	log, err := eventlog.Open(source)
	if err != nil {
		return false
	}
	log.Close()
	return true
}

// EventLogEntry creates a log entry with a specific event ID
func EventLogEntry(eventID EventID) Field {
	return Field{Key: "event_id", Value: eventID}
}
