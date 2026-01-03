package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

// NoopAuditLogger is a no-op audit logger
type NoopAuditLogger struct{}

func (n *NoopAuditLogger) Log(ctx context.Context, entry *AuditEntry) error {
	return nil
}

func (n *NoopAuditLogger) Close() error {
	return nil
}

// StderrAuditLogger logs audit entries to stderr
type StderrAuditLogger struct {
	writer io.Writer
	mu     sync.Mutex
}

// NewStderrAuditLogger creates a new stderr audit logger
func NewStderrAuditLogger() *StderrAuditLogger {
	return &StderrAuditLogger{
		writer: os.Stderr,
	}
}

// Log logs an audit entry to stderr
func (s *StderrAuditLogger) Log(ctx context.Context, entry *AuditEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Format as JSON for structured logging
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(s.writer, "AUDIT: %s\n", data)
	return err
}

// Close closes the stderr audit logger
func (s *StderrAuditLogger) Close() error {
	return nil
}

// SyslogAuditLogger logs audit entries to syslog
type SyslogAuditLogger struct {
	conn     net.Conn
	facility int
	tag      string
	mu       sync.Mutex
}

// Syslog facilities
const (
	syslogFacilityAuth     = 4  // LOG_AUTH
	syslogFacilityAuthPriv = 10 // LOG_AUTHPRIV
	syslogFacilityLocal0   = 16
	syslogFacilityLocal7   = 23
)

// Syslog severities
const (
	syslogSeverityInfo    = 6
	syslogSeverityWarning = 4
	syslogSeverityError   = 3
)

// NewSyslogAuditLogger creates a new syslog audit logger
func NewSyslogAuditLogger(facility string) (*SyslogAuditLogger, error) {
	// Determine facility code
	facilityCode := syslogFacilityAuth
	switch strings.ToLower(facility) {
	case "auth":
		facilityCode = syslogFacilityAuth
	case "authpriv":
		facilityCode = syslogFacilityAuthPriv
	case "local0":
		facilityCode = syslogFacilityLocal0
	case "local7":
		facilityCode = syslogFacilityLocal7
	}

	// Try to connect to syslog
	conn, err := connectToSyslog()
	if err != nil {
		return nil, err
	}

	return &SyslogAuditLogger{
		conn:     conn,
		facility: facilityCode,
		tag:      "kscore-audit",
	}, nil
}

// connectToSyslog connects to the local syslog socket
func connectToSyslog() (net.Conn, error) {
	// Try common syslog socket paths
	paths := []string{
		"/dev/log",           // Linux
		"/var/run/syslog",    // macOS
		"/var/run/log",       // Some BSD
	}

	for _, path := range paths {
		conn, err := net.Dial("unix", path)
		if err == nil {
			return conn, nil
		}
	}

	return nil, fmt.Errorf("failed to connect to syslog socket")
}

// Log logs an audit entry to syslog
func (s *SyslogAuditLogger) Log(ctx context.Context, entry *AuditEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Determine severity based on result
	severity := syslogSeverityInfo
	switch entry.Result {
	case ResultFailure, ResultDenied:
		severity = syslogSeverityError
	case ResultTimeout:
		severity = syslogSeverityWarning
	}

	// Calculate priority
	priority := s.facility*8 + severity

	// Format the message
	// BSD syslog format: <PRI>TIMESTAMP HOSTNAME TAG[PID]: MSG
	timestamp := entry.Timestamp.Format("Jan  2 15:04:05")
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "localhost"
	}

	// Build structured message
	msg := fmt.Sprintf("AUDIT user=%s action=%s tool=%s command=%s result=%s",
		entry.User,
		entry.AuditType,
		entry.Tool,
		entry.Command,
		entry.Result,
	)

	if entry.Target != "" {
		msg += fmt.Sprintf(" target=%q", entry.Target)
	}

	if entry.DurationMS > 0 {
		msg += fmt.Sprintf(" duration_ms=%d", entry.DurationMS)
	}

	if entry.Error != "" {
		msg += fmt.Sprintf(" error=%q", entry.Error)
	}

	// Full syslog message
	syslogMsg := fmt.Sprintf("<%d>%s %s %s[%d]: %s\n",
		priority,
		timestamp,
		hostname,
		s.tag,
		entry.PID,
		msg,
	)

	_, err := s.conn.Write([]byte(syslogMsg))
	if err != nil {
		// Try to reconnect
		s.conn.Close()
		conn, connErr := connectToSyslog()
		if connErr != nil {
			return fmt.Errorf("syslog write failed and reconnect failed: %v (original: %v)", connErr, err)
		}
		s.conn = conn
		_, err = s.conn.Write([]byte(syslogMsg))
	}

	return err
}

// Close closes the syslog connection
func (s *SyslogAuditLogger) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

// JournaldAuditLogger logs audit entries to systemd journald
type JournaldAuditLogger struct {
	conn net.Conn
	mu   sync.Mutex
}

// NewJournaldAuditLogger creates a new journald audit logger
func NewJournaldAuditLogger() (*JournaldAuditLogger, error) {
	// Connect to journald socket
	conn, err := net.Dial("unix", "/run/systemd/journal/socket")
	if err != nil {
		return nil, fmt.Errorf("failed to connect to journald: %w", err)
	}

	return &JournaldAuditLogger{
		conn: conn,
	}, nil
}

// Log logs an audit entry to journald
func (j *JournaldAuditLogger) Log(ctx context.Context, entry *AuditEntry) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	// Build journald message with structured fields
	// Format: FIELD=value\n separated, with binary length prefix for values with newlines
	var msg strings.Builder

	// Standard journald fields
	writeJournaldField(&msg, "MESSAGE", fmt.Sprintf("AUDIT: %s %s %s", entry.Tool, entry.Command, entry.Result))
	writeJournaldField(&msg, "PRIORITY", fmt.Sprintf("%d", journaldPriority(entry.Result)))
	writeJournaldField(&msg, "SYSLOG_IDENTIFIER", "kscore-audit")

	// Custom fields (prefixed with KSCORE_)
	writeJournaldField(&msg, "KSCORE_AUDIT_TYPE", string(entry.AuditType))
	writeJournaldField(&msg, "KSCORE_USER", entry.User)
	writeJournaldField(&msg, "KSCORE_UID", fmt.Sprintf("%d", entry.UID))
	writeJournaldField(&msg, "KSCORE_TOOL", entry.Tool)
	writeJournaldField(&msg, "KSCORE_COMMAND", entry.Command)
	writeJournaldField(&msg, "KSCORE_RESULT", string(entry.Result))
	writeJournaldField(&msg, "KSCORE_EXIT_CODE", fmt.Sprintf("%d", entry.ExitCode))
	writeJournaldField(&msg, "KSCORE_DURATION_MS", fmt.Sprintf("%d", entry.DurationMS))
	writeJournaldField(&msg, "KSCORE_CORRELATION_ID", entry.CorrelationID)

	if entry.Target != "" {
		writeJournaldField(&msg, "KSCORE_TARGET", entry.Target)
	}

	if entry.TTY != "" {
		writeJournaldField(&msg, "KSCORE_TTY", entry.TTY)
	}

	if entry.RemoteAddr != "" {
		writeJournaldField(&msg, "KSCORE_REMOTE_ADDR", entry.RemoteAddr)
	}

	if entry.Error != "" {
		writeJournaldField(&msg, "KSCORE_ERROR", entry.Error)
	}

	if entry.AgentsMatched > 0 {
		writeJournaldField(&msg, "KSCORE_AGENTS_MATCHED", fmt.Sprintf("%d", entry.AgentsMatched))
	}

	// Send to journald
	_, err := j.conn.Write([]byte(msg.String()))
	return err
}

// writeJournaldField writes a field in journald format
func writeJournaldField(msg *strings.Builder, key, value string) {
	if value == "" {
		return
	}
	// Simple format for values without newlines
	if !strings.ContainsAny(value, "\n\r") {
		msg.WriteString(key)
		msg.WriteString("=")
		msg.WriteString(value)
		msg.WriteString("\n")
	} else {
		// Binary format for values with newlines
		// FIELD\n<64-bit length><data>
		msg.WriteString(key)
		msg.WriteString("\n")
		// Length as little-endian 64-bit
		length := uint64(len(value))
		for i := 0; i < 8; i++ {
			msg.WriteByte(byte(length >> (i * 8)))
		}
		msg.WriteString(value)
	}
}

// journaldPriority returns journald priority for a result
func journaldPriority(result AuditResult) int {
	switch result {
	case ResultSuccess:
		return 6 // info
	case ResultFailure, ResultDenied:
		return 3 // error
	case ResultTimeout:
		return 4 // warning
	default:
		return 6 // info
	}
}

// Close closes the journald connection
func (j *JournaldAuditLogger) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()

	if j.conn != nil {
		return j.conn.Close()
	}
	return nil
}

// MemoryAuditLogger stores audit entries in memory (for testing)
type MemoryAuditLogger struct {
	entries []*AuditEntry
	mu      sync.Mutex
}

// NewMemoryAuditLogger creates a new in-memory audit logger
func NewMemoryAuditLogger() *MemoryAuditLogger {
	return &MemoryAuditLogger{
		entries: make([]*AuditEntry, 0),
	}
}

// Log logs an audit entry to memory
func (m *MemoryAuditLogger) Log(ctx context.Context, entry *AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Deep copy the entry
	copied := *entry
	if entry.Args != nil {
		copied.Args = make([]string, len(entry.Args))
		copy(copied.Args, entry.Args)
	}
	if entry.Extra != nil {
		copied.Extra = make(map[string]interface{})
		for k, v := range entry.Extra {
			copied.Extra[k] = v
		}
	}

	m.entries = append(m.entries, &copied)
	return nil
}

// Entries returns all logged entries
func (m *MemoryAuditLogger) Entries() []*AuditEntry {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]*AuditEntry, len(m.entries))
	copy(result, m.entries)
	return result
}

// Clear clears all entries
func (m *MemoryAuditLogger) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.entries = make([]*AuditEntry, 0)
}

// Close closes the memory audit logger
func (m *MemoryAuditLogger) Close() error {
	return nil
}

// MultiAuditLogger logs to multiple backends
type MultiAuditLogger struct {
	loggers []AuditLogger
}

// NewMultiAuditLogger creates a new multi-backend audit logger
func NewMultiAuditLogger(loggers ...AuditLogger) *MultiAuditLogger {
	return &MultiAuditLogger{
		loggers: loggers,
	}
}

// Log logs an audit entry to all backends
func (m *MultiAuditLogger) Log(ctx context.Context, entry *AuditEntry) error {
	var lastErr error
	for _, logger := range m.loggers {
		if err := logger.Log(ctx, entry); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// Close closes all backends
func (m *MultiAuditLogger) Close() error {
	var lastErr error
	for _, logger := range m.loggers {
		if err := logger.Close(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// FileAuditLogger logs audit entries to a file (for testing or local logging)
type FileAuditLogger struct {
	file *os.File
	mu   sync.Mutex
}

// NewFileAuditLogger creates a new file audit logger
func NewFileAuditLogger(path string) (*FileAuditLogger, error) {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}

	return &FileAuditLogger{
		file: file,
	}, nil
}

// Log logs an audit entry to the file
func (f *FileAuditLogger) Log(ctx context.Context, entry *AuditEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	_, err = f.file.Write(append(data, '\n'))
	return err
}

// Close closes the file
func (f *FileAuditLogger) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.file != nil {
		return f.file.Close()
	}
	return nil
}

// WithTimeout wraps an audit logger with a timeout
type TimeoutAuditLogger struct {
	logger  AuditLogger
	timeout time.Duration
}

// NewTimeoutAuditLogger creates a new timeout-wrapped audit logger
func NewTimeoutAuditLogger(logger AuditLogger, timeout time.Duration) *TimeoutAuditLogger {
	return &TimeoutAuditLogger{
		logger:  logger,
		timeout: timeout,
	}
}

// Log logs with a timeout
func (t *TimeoutAuditLogger) Log(ctx context.Context, entry *AuditEntry) error {
	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- t.logger.Log(ctx, entry)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close closes the wrapped logger
func (t *TimeoutAuditLogger) Close() error {
	return t.logger.Close()
}
