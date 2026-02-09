// Package debug provides detailed protocol debugging and logging for proxy agents
package debug

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Level represents the debugging verbosity level
type Level int

const (
	// LevelOff disables all debug output
	LevelOff Level = iota
	// LevelBasic shows connection events and errors
	LevelBasic
	// LevelVerbose shows commands and responses
	LevelVerbose
	// LevelTrace shows all protocol-level data including raw bytes
	LevelTrace
)

// String returns the string representation of the level
func (l Level) String() string {
	switch l {
	case LevelOff:
		return "off"
	case LevelBasic:
		return "basic"
	case LevelVerbose:
		return "verbose"
	case LevelTrace:
		return "trace"
	default:
		return "unknown"
	}
}

// ParseLevel parses a level string
func ParseLevel(s string) Level {
	switch strings.ToLower(s) {
	case "off", "none", "0":
		return LevelOff
	case "basic", "info", "1":
		return LevelBasic
	case "verbose", "debug", "2":
		return LevelVerbose
	case "trace", "all", "3":
		return LevelTrace
	default:
		return LevelOff
	}
}

// Direction indicates the direction of a protocol message
type Direction string

const (
	// DirectionSend indicates data sent to the device
	DirectionSend Direction = "send"
	// DirectionRecv indicates data received from the device
	DirectionRecv Direction = "recv"
)

// Protocol identifies the protocol being debugged
type Protocol string

// ProtocolSSH and related constants.
const (
	ProtocolSSH    Protocol = "ssh"
	ProtocolSNMP   Protocol = "snmp"
	ProtocolREST   Protocol = "rest"
	ProtocolWinRM  Protocol = "winrm"
	ProtocolTelnet Protocol = "telnet"
	ProtocolGNMI   Protocol = "gnmi"
	ProtocolAPI    Protocol = "api"
)

// Event represents a debug event
type Event struct {
	// Timestamp when the event occurred
	Timestamp time.Time `json:"timestamp"`

	// Protocol being used
	Protocol Protocol `json:"protocol"`

	// DeviceID is the target device identifier
	DeviceID string `json:"device_id"`

	// SessionID is the session identifier
	SessionID string `json:"session_id,omitempty"`

	// Type is the event type
	Type EventType `json:"type"`

	// Direction indicates send or receive
	Direction Direction `json:"direction,omitempty"`

	// Message is the human-readable description
	Message string `json:"message"`

	// Data contains the raw protocol data (may be redacted)
	Data []byte `json:"data,omitempty"`

	// DataHex is the hex representation of the data
	DataHex string `json:"data_hex,omitempty"`

	// DataLen is the length of the original data
	DataLen int `json:"data_len,omitempty"`

	// Duration for timed operations
	Duration time.Duration `json:"duration,omitempty"`

	// Error if the event represents an error
	Error string `json:"error,omitempty"`

	// Metadata contains protocol-specific metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// Level is the minimum level to display this event
	Level Level `json:"level"`
}

// EventType categorizes debug events
type EventType string

// EventTypeConnect constants define the supported types.
const (
	EventTypeConnect      EventType = "connect"
	EventTypeDisconnect   EventType = "disconnect"
	EventTypeAuthenticate EventType = "authenticate"
	EventTypeSend         EventType = "send"
	EventTypeReceive      EventType = "receive"
	EventTypeCommand      EventType = "command"
	EventTypeResponse     EventType = "response"
	EventTypeError        EventType = "error"
	EventTypeWarning      EventType = "warning"
	EventTypeInfo         EventType = "info"
	EventTypeHandshake    EventType = "handshake"
	EventTypeKeepAlive    EventType = "keepalive"
	EventTypeTimeout      EventType = "timeout"
)

// Logger provides protocol-level debugging and logging
type Logger struct {
	level     atomic.Int32
	output    io.Writer
	format    Format
	redactor  *Redactor
	mu        sync.Mutex
	sessionID string
	protocol  Protocol
	deviceID  string
	events    []*Event
	maxEvents int
	callback  EventCallback
}

// Format specifies the output format
type Format string

// FormatText constants define the output formats.
const (
	FormatText Format = "text"
	FormatJSON Format = "json"
	FormatHex  Format = "hex"
)

// EventCallback is called for each debug event
type EventCallback func(event *Event)

// Option configures a Logger
type Option func(*Logger)

// WithLevel sets the debug level
func WithLevel(level Level) Option {
	return func(l *Logger) {
		//nolint:gosec // G115: Level is a small enum (0-3), fits in int32
		l.level.Store(int32(level))
	}
}

// WithOutput sets the output writer
func WithOutput(w io.Writer) Option {
	return func(l *Logger) {
		l.output = w
	}
}

// WithFormat sets the output format
func WithFormat(f Format) Option {
	return func(l *Logger) {
		l.format = f
	}
}

// WithRedactor sets the redactor for sensitive data
func WithRedactor(r *Redactor) Option {
	return func(l *Logger) {
		l.redactor = r
	}
}

// WithMaxEvents sets the maximum events to retain
func WithMaxEvents(maxVal int) Option {
	return func(l *Logger) {
		l.maxEvents = maxVal
	}
}

// WithCallback sets an event callback
func WithCallback(cb EventCallback) Option {
	return func(l *Logger) {
		l.callback = cb
	}
}

// WithProtocol sets the protocol type
func WithProtocol(p Protocol) Option {
	return func(l *Logger) {
		l.protocol = p
	}
}

// WithDeviceID sets the device ID
func WithDeviceID(id string) Option {
	return func(l *Logger) {
		l.deviceID = id
	}
}

// WithSessionID sets the session ID
func WithSessionID(id string) Option {
	return func(l *Logger) {
		l.sessionID = id
	}
}

// NewLogger creates a new debug logger
func NewLogger(opts ...Option) *Logger {
	l := &Logger{
		output:    io.Discard,
		format:    FormatText,
		redactor:  NewRedactor(),
		maxEvents: 1000,
	}
	l.level.Store(int32(LevelOff))

	for _, opt := range opts {
		opt(l)
	}

	return l
}

// Level returns the current debug level
func (l *Logger) Level() Level {
	return Level(l.level.Load())
}

// SetLevel sets the debug level
func (l *Logger) SetLevel(level Level) {
	//nolint:gosec // G115: Level is a small enum (0-3), fits in int32
	l.level.Store(int32(level))
}

// IsEnabled returns true if the given level is enabled
func (l *Logger) IsEnabled(level Level) bool {
	return Level(l.level.Load()) >= level
}

// Log logs a debug event
func (l *Logger) Log(event *Event) {
	if !l.IsEnabled(event.Level) {
		return
	}

	// Apply defaults
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	if event.Protocol == "" {
		event.Protocol = l.protocol
	}
	if event.DeviceID == "" {
		event.DeviceID = l.deviceID
	}
	if event.SessionID == "" {
		event.SessionID = l.sessionID
	}

	// Redact sensitive data
	if l.redactor != nil && len(event.Data) > 0 {
		event.Data = l.redactor.Redact(event.Data)
	}
	if event.Message != "" && l.redactor != nil {
		event.Message = l.redactor.RedactString(event.Message)
	}

	// Store event
	l.mu.Lock()
	l.events = append(l.events, event)
	if l.maxEvents > 0 && len(l.events) > l.maxEvents {
		l.events = l.events[len(l.events)-l.maxEvents:]
	}
	l.mu.Unlock()

	// Write to output
	l.write(event)

	// Call callback
	if l.callback != nil {
		l.callback(event)
	}
}

func (l *Logger) write(event *Event) {
	l.mu.Lock()
	defer l.mu.Unlock()

	var output string
	switch l.format {
	case FormatJSON:
		output = l.formatJSON(event)
	case FormatHex:
		output = l.formatHex(event)
	default:
		output = l.formatText(event)
	}

	fmt.Fprintln(l.output, output)
}

func (l *Logger) formatText(event *Event) string {
	var sb strings.Builder

	// Timestamp
	sb.WriteString(event.Timestamp.Format("15:04:05.000"))
	sb.WriteString(" ")

	// Level indicator
	switch event.Type {
	case EventTypeError:
		sb.WriteString("[ERROR] ")
	case EventTypeWarning:
		sb.WriteString("[WARN]  ")
	default:
		sb.WriteString("[DEBUG] ")
	}

	// Protocol and device
	sb.WriteString(fmt.Sprintf("[%s:%s] ", event.Protocol, event.DeviceID))

	// Direction arrow
	switch event.Direction {
	case DirectionSend:
		sb.WriteString(">>> ")
	case DirectionRecv:
		sb.WriteString("<<< ")
	}

	// Event type
	sb.WriteString(fmt.Sprintf("[%s] ", event.Type))

	// Message
	sb.WriteString(event.Message)

	// Duration
	if event.Duration > 0 {
		sb.WriteString(fmt.Sprintf(" (%.3fms)", float64(event.Duration.Microseconds())/1000))
	}

	// Data preview
	if len(event.Data) > 0 {
		sb.WriteString("\n    Data: ")
		if isPrintable(event.Data) {
			preview := string(event.Data)
			if len(preview) > 200 {
				preview = preview[:200] + "..."
			}
			sb.WriteString(preview)
		} else {
			preview := hex.EncodeToString(event.Data)
			if len(preview) > 100 {
				preview = preview[:100] + "..."
			}
			sb.WriteString(preview)
		}
		sb.WriteString(fmt.Sprintf(" (%d bytes)", len(event.Data)))
	}

	// Error
	if event.Error != "" {
		sb.WriteString(fmt.Sprintf("\n    Error: %s", event.Error))
	}

	return sb.String()
}

func (l *Logger) formatJSON(event *Event) string {
	// Prepare JSON-safe event
	jsonEvent := struct {
		Timestamp string                 `json:"timestamp"`
		Protocol  Protocol               `json:"protocol"`
		DeviceID  string                 `json:"device_id"`
		SessionID string                 `json:"session_id,omitempty"`
		Type      EventType              `json:"type"`
		Direction Direction              `json:"direction,omitempty"`
		Message   string                 `json:"message"`
		DataHex   string                 `json:"data_hex,omitempty"`
		DataLen   int                    `json:"data_len,omitempty"`
		Duration  string                 `json:"duration,omitempty"`
		Error     string                 `json:"error,omitempty"`
		Metadata  map[string]interface{} `json:"metadata,omitempty"`
		Level     string                 `json:"level"`
	}{
		Timestamp: event.Timestamp.Format(time.RFC3339Nano),
		Protocol:  event.Protocol,
		DeviceID:  event.DeviceID,
		SessionID: event.SessionID,
		Type:      event.Type,
		Direction: event.Direction,
		Message:   event.Message,
		Error:     event.Error,
		Metadata:  event.Metadata,
		Level:     event.Level.String(),
	}

	if len(event.Data) > 0 {
		jsonEvent.DataHex = hex.EncodeToString(event.Data)
		jsonEvent.DataLen = len(event.Data)
	}

	if event.Duration > 0 {
		jsonEvent.Duration = event.Duration.String()
	}

	data, _ := json.Marshal(jsonEvent)
	return string(data)
}

func (l *Logger) formatHex(event *Event) string {
	var sb strings.Builder

	sb.WriteString(l.formatText(event))

	if len(event.Data) > 0 {
		sb.WriteString("\n")
		sb.WriteString(hexDump(event.Data))
	}

	return sb.String()
}

// Connect logs a connection event
func (l *Logger) Connect(message string) {
	l.Log(&Event{
		Type:    EventTypeConnect,
		Message: message,
		Level:   LevelBasic,
	})
}

// Disconnect logs a disconnection event
func (l *Logger) Disconnect(message string) {
	l.Log(&Event{
		Type:    EventTypeDisconnect,
		Message: message,
		Level:   LevelBasic,
	})
}

// Authenticate logs an authentication event
func (l *Logger) Authenticate(method string, success bool) {
	status := "failed"
	if success {
		status = "success"
	}
	msg := fmt.Sprintf("Auth [%s]: %s", method, status)
	eventType := EventTypeAuthenticate
	if !success {
		eventType = EventTypeError
	}
	l.Log(&Event{
		Type:    eventType,
		Message: msg,
		Level:   LevelBasic,
		Metadata: map[string]interface{}{
			"method":  method,
			"success": success,
		},
	})
}

// Send logs sent data
func (l *Logger) Send(message string, data []byte) {
	l.Log(&Event{
		Type:      EventTypeSend,
		Direction: DirectionSend,
		Message:   message,
		Data:      data,
		DataLen:   len(data),
		Level:     LevelVerbose,
	})
}

// Receive logs received data
func (l *Logger) Receive(message string, data []byte) {
	l.Log(&Event{
		Type:      EventTypeReceive,
		Direction: DirectionRecv,
		Message:   message,
		Data:      data,
		DataLen:   len(data),
		Level:     LevelVerbose,
	})
}

// Command logs a command execution
func (l *Logger) Command(cmd string) {
	l.Log(&Event{
		Type:      EventTypeCommand,
		Direction: DirectionSend,
		Message:   fmt.Sprintf("Executing: %s", cmd),
		Data:      []byte(cmd),
		Level:     LevelVerbose,
	})
}

// Response logs a command response
func (l *Logger) Response(output string, exitCode int, duration time.Duration) {
	l.Log(&Event{
		Type:      EventTypeResponse,
		Direction: DirectionRecv,
		Message:   fmt.Sprintf("Response (exit %d)", exitCode),
		Data:      []byte(output),
		Duration:  duration,
		Level:     LevelVerbose,
		Metadata: map[string]interface{}{
			"exit_code": exitCode,
		},
	})
}

// Error logs an error event
func (l *Logger) Error(message string, err error) {
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}
	l.Log(&Event{
		Type:    EventTypeError,
		Message: message,
		Error:   errStr,
		Level:   LevelBasic,
	})
}

// Warning logs a warning event
func (l *Logger) Warning(message string) {
	l.Log(&Event{
		Type:    EventTypeWarning,
		Message: message,
		Level:   LevelBasic,
	})
}

// Info logs an info event
func (l *Logger) Info(message string) {
	l.Log(&Event{
		Type:    EventTypeInfo,
		Message: message,
		Level:   LevelVerbose,
	})
}

// Trace logs raw protocol data at trace level
func (l *Logger) Trace(direction Direction, data []byte) {
	var msg string
	if direction == DirectionSend {
		msg = "Sending raw data"
	} else {
		msg = "Receiving raw data"
	}
	l.Log(&Event{
		Type:      EventTypeSend,
		Direction: direction,
		Message:   msg,
		Data:      data,
		DataLen:   len(data),
		Level:     LevelTrace,
	})
}

// Events returns all captured events
func (l *Logger) Events() []*Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	result := make([]*Event, len(l.events))
	copy(result, l.events)
	return result
}

// Clear clears captured events
func (l *Logger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = nil
}

// Redactor handles sensitive data redaction
type Redactor struct {
	patterns []*redactPattern
	mu       sync.RWMutex
}

type redactPattern struct {
	pattern     *regexp.Regexp
	replacement string
	description string
}

// NewRedactor creates a new redactor with default patterns
func NewRedactor() *Redactor {
	r := &Redactor{}

	// Add default sensitive patterns
	_ = r.AddPattern(`(?i)password\s*[:=]\s*\S+`, "password=***REDACTED***", "passwords")           //nolint:errcheck // valid regex patterns
	_ = r.AddPattern(`(?i)secret\s*[:=]\s*\S+`, "secret=***REDACTED***", "secrets")                 //nolint:errcheck // valid regex patterns
	_ = r.AddPattern(`(?i)token\s*[:=]\s*\S+`, "token=***REDACTED***", "tokens")                    //nolint:errcheck // valid regex patterns
	_ = r.AddPattern(`(?i)api[_-]?key\s*[:=]\s*\S+`, "api_key=***REDACTED***", "API keys")          //nolint:errcheck // valid regex patterns
	_ = r.AddPattern(`(?i)authorization:\s*\S+`, "Authorization: ***REDACTED***", "auth headers")  //nolint:errcheck // valid regex patterns
	_ = r.AddPattern(`(?i)bearer\s+\S+`, "Bearer ***REDACTED***", "bearer tokens")                  //nolint:errcheck // valid regex patterns
	_ = r.AddPattern(`(?i)private[_-]?key`, "***PRIVATE_KEY***", "private keys")                    //nolint:errcheck // valid regex patterns
	_ = r.AddPattern(`(?i)community\s*[:=]\s*\S+`, "community=***REDACTED***", "SNMP community")    //nolint:errcheck // valid regex patterns

	return r
}

// AddPattern adds a redaction pattern
func (r *Redactor) AddPattern(pattern, replacement, description string) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.patterns = append(r.patterns, &redactPattern{
		pattern:     re,
		replacement: replacement,
		description: description,
	})

	return nil
}

// Redact redacts sensitive data from bytes
func (r *Redactor) Redact(data []byte) []byte {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := data
	for _, p := range r.patterns {
		result = p.pattern.ReplaceAll(result, []byte(p.replacement))
	}
	return result
}

// RedactString redacts sensitive data from a string
func (r *Redactor) RedactString(s string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := s
	for _, p := range r.patterns {
		result = p.pattern.ReplaceAllString(result, p.replacement)
	}
	return result
}

// isPrintable returns true if data appears to be printable text
func isPrintable(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	printable := 0
	for _, b := range data {
		if (b >= 32 && b <= 126) || b == '\n' || b == '\r' || b == '\t' {
			printable++
		}
	}
	return float64(printable)/float64(len(data)) > 0.9
}

// hexDump creates a hex dump of data
func hexDump(data []byte) string {
	var sb strings.Builder
	var buf bytes.Buffer

	for i := 0; i < len(data); i += 16 {
		end := i + 16
		if end > len(data) {
			end = len(data)
		}

		// Offset
		sb.WriteString(fmt.Sprintf("    %08x  ", i))

		// Hex bytes
		buf.Reset()
		for j := i; j < end; j++ {
			sb.WriteString(fmt.Sprintf("%02x ", data[j]))
			if data[j] >= 32 && data[j] <= 126 {
				buf.WriteByte(data[j])
			} else {
				buf.WriteByte('.')
			}
		}

		// Padding
		for j := end; j < i+16; j++ {
			sb.WriteString("   ")
		}

		// ASCII
		sb.WriteString(" |")
		sb.WriteString(buf.String())
		sb.WriteString("|\n")
	}

	return sb.String()
}

// SessionLogger wraps a Logger for a specific session
type SessionLogger struct {
	*Logger
	sessionID string
	deviceID  string
	protocol  Protocol
	startTime time.Time
}

// NewSessionLogger creates a session-scoped logger
func NewSessionLogger(base *Logger, sessionID, deviceID string, protocol Protocol) *SessionLogger {
	return &SessionLogger{
		Logger:    base,
		sessionID: sessionID,
		deviceID:  deviceID,
		protocol:  protocol,
		startTime: time.Now(),
	}
}

// Log logs an event with session context
func (s *SessionLogger) Log(event *Event) {
	event.SessionID = s.sessionID
	event.DeviceID = s.deviceID
	event.Protocol = s.protocol
	s.Logger.Log(event)
}

// Duration returns the session duration
func (s *SessionLogger) Duration() time.Duration {
	return time.Since(s.startTime)
}

// Summary returns a session summary
func (s *SessionLogger) Summary() *SessionSummary {
	events := s.Events()

	summary := &SessionSummary{
		SessionID:  s.sessionID,
		DeviceID:   s.deviceID,
		Protocol:   s.protocol,
		StartTime:  s.startTime,
		Duration:   s.Duration(),
		EventCount: len(events),
	}

	for _, e := range events {
		switch e.Type {
		case EventTypeCommand:
			summary.CommandCount++
		case EventTypeError:
			summary.ErrorCount++
		case EventTypeSend:
			summary.BytesSent += e.DataLen
		case EventTypeReceive:
			summary.BytesReceived += e.DataLen
		default:
			// Other event types not counted in summary
		}
	}

	return summary
}

// SessionSummary contains session statistics
type SessionSummary struct {
	SessionID     string        `json:"session_id"`
	DeviceID      string        `json:"device_id"`
	Protocol      Protocol      `json:"protocol"`
	StartTime     time.Time     `json:"start_time"`
	Duration      time.Duration `json:"duration"`
	EventCount    int           `json:"event_count"`
	CommandCount  int           `json:"command_count"`
	ErrorCount    int           `json:"error_count"`
	BytesSent     int           `json:"bytes_sent"`
	BytesReceived int           `json:"bytes_received"`
}
