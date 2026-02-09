package logging

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestParseFacility(t *testing.T) {
	tests := []struct {
		input    string
		expected SyslogFacility
		ok       bool
	}{
		{"kern", FacilityKern, true},
		{"user", FacilityUser, true},
		{"daemon", FacilityDaemon, true},
		{"auth", FacilityAuth, true},
		{"local0", FacilityLocal0, true},
		{"local7", FacilityLocal7, true},
		{"LOCAL0", FacilityLocal0, true}, // Case insensitive
		{"invalid", FacilityLocal0, false},
		{"", FacilityLocal0, false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, ok := ParseFacility(tt.input)
			if ok != tt.ok {
				t.Errorf("ParseFacility(%q) ok = %v, want %v", tt.input, ok, tt.ok)
			}
			if ok && result != tt.expected {
				t.Errorf("ParseFacility(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestLevelToSeverity(t *testing.T) {
	tests := []struct {
		level    Level
		expected SyslogSeverity
	}{
		{LevelDebug, SeverityDebug},
		{LevelInfo, SeverityInfo},
		{LevelWarn, SeverityWarning},
		{LevelError, SeverityError},
	}

	for _, tt := range tests {
		t.Run(tt.level.String(), func(t *testing.T) {
			result := LevelToSeverity(tt.level)
			if result != tt.expected {
				t.Errorf("LevelToSeverity(%v) = %v, want %v", tt.level, result, tt.expected)
			}
		})
	}
}

func TestSyslogFormatterRFC5424(t *testing.T) {
	formatter := NewSyslogFormatter(FacilityLocal0, "test-app")
	formatter.Hostname = "testhost"

	entry := &Entry{
		Timestamp:     time.Date(2024, 1, 15, 10, 30, 45, 123456000, time.UTC),
		Level:         LevelInfo,
		Logger:        "test",
		Message:       "Test message",
		CorrelationID: "corr-123",
		Fields: map[string]interface{}{
			"key": "value",
		},
	}

	data, err := formatter.Format(entry)
	if err != nil {
		t.Fatalf("Format() error = %v", err)
	}

	msg := string(data)

	// Check priority: local0 (16) * 8 + info (6) = 134
	if !strings.HasPrefix(msg, "<134>") {
		t.Errorf("Expected priority 134, got: %s", msg)
	}

	// Check version
	if !strings.Contains(msg, "<134>1 ") {
		t.Errorf("Expected version 1, got: %s", msg)
	}

	// Check timestamp format
	if !strings.Contains(msg, "2024-01-15T10:30:45.123456Z") {
		t.Errorf("Expected RFC3339 timestamp, got: %s", msg)
	}

	// Check hostname
	if !strings.Contains(msg, "testhost") {
		t.Errorf("Expected hostname 'testhost', got: %s", msg)
	}

	// Check app name
	if !strings.Contains(msg, "test-app") {
		t.Errorf("Expected app name 'test-app', got: %s", msg)
	}

	// Check correlation ID as MSGID
	if !strings.Contains(msg, "corr-123") {
		t.Errorf("Expected correlation ID 'corr-123', got: %s", msg)
	}

	// Check structured data
	if !strings.Contains(msg, "[kscore@49152") {
		t.Errorf("Expected structured data, got: %s", msg)
	}

	// Check message
	if !strings.Contains(msg, "Test message") {
		t.Errorf("Expected message 'Test message', got: %s", msg)
	}
}

func TestSyslogFormatterNoStructuredData(t *testing.T) {
	formatter := NewSyslogFormatter(FacilityDaemon, "daemon-app")
	formatter.IncludeStructuredData = false

	entry := &Entry{
		Timestamp: time.Now(),
		Level:     LevelError,
		Logger:    "test",
		Message:   "Error occurred",
		Fields:    map[string]interface{}{"key": "value"},
	}

	data, err := formatter.Format(entry)
	if err != nil {
		t.Fatalf("Format() error = %v", err)
	}

	msg := string(data)

	// Priority should be daemon (3) * 8 + error (3) = 27
	if !strings.HasPrefix(msg, "<27>") {
		t.Errorf("Expected priority 27, got: %s", msg)
	}

	// Should have "-" for structured data when disabled
	// The message format has structured data position before the message
	if strings.Contains(msg, "[kscore@49152") {
		t.Errorf("Expected no structured data, got: %s", msg)
	}
}

func TestBSDSyslogFormatter(t *testing.T) {
	formatter := NewBSDSyslogFormatter(FacilityAuth, "kscore-exec")

	entry := &Entry{
		Timestamp: time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC),
		Level:     LevelWarn,
		Logger:    "test",
		Message:   "Warning message",
	}

	data, err := formatter.Format(entry)
	if err != nil {
		t.Fatalf("Format() error = %v", err)
	}

	msg := string(data)

	// Priority should be auth (4) * 8 + warning (4) = 36
	if !strings.HasPrefix(msg, "<36>") {
		t.Errorf("Expected priority 36, got: %s", msg)
	}

	// Check BSD timestamp format (note: day is space-padded to 2 chars)
	if !strings.Contains(msg, "Jan 15 10:30:45") && !strings.Contains(msg, "Jan  15 10:30:45") {
		t.Errorf("Expected BSD timestamp, got: %s", msg)
	}

	// Check tag
	if !strings.Contains(msg, "kscore-exec[") {
		t.Errorf("Expected tag 'kscore-exec', got: %s", msg)
	}

	// Check message
	if !strings.Contains(msg, "Warning message") {
		t.Errorf("Expected message 'Warning message', got: %s", msg)
	}
}

func TestEscapeSDValue(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{`with"quote`, `with\"quote`},
		{`with\backslash`, `with\\backslash`},
		{`with]bracket`, `with\]bracket`},
		{`all\"three]`, `all\\\"three\]`},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := escapeSDValue(tt.input)
			if result != tt.expected {
				t.Errorf("escapeSDValue(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestEscapeSDKey(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"with space", "with_space"},
		{"with=equals", "with_equals"},
		{"with]bracket", "with_bracket"},
		{`with"quote`, "with_quote"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := escapeSDKey(tt.input)
			if result != tt.expected {
				t.Errorf("escapeSDKey(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSyslogOutputUDP(t *testing.T) {
	// Start a UDP listener
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().String()

	// Create syslog output
	config := &SyslogConfig{
		Network:     "udp",
		Address:     localAddr,
		Facility:    FacilityLocal0,
		AppName:     "test",
		DialTimeout: time.Second,
	}

	output, err := NewSyslogOutput(config)
	if err != nil {
		t.Fatalf("NewSyslogOutput() error = %v", err)
	}
	defer output.Close()

	// Create formatter and format a message
	formatter := NewSyslogFormatter(FacilityLocal0, "test")
	entry := &Entry{
		Timestamp: time.Now(),
		Level:     LevelInfo,
		Logger:    "test",
		Message:   "UDP test message",
	}

	data, err := formatter.Format(entry)
	if err != nil {
		t.Fatalf("Format() error = %v", err)
	}

	// Write to syslog
	err = output.Write(data)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	// Read from UDP
	buf := make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(time.Second))
	n, _, err := conn.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("ReadFromUDP() error = %v", err)
	}

	received := string(buf[:n])
	if !strings.Contains(received, "UDP test message") {
		t.Errorf("Expected message in received data, got: %s", received)
	}
}

func TestSyslogOutputTCP(t *testing.T) {
	// Start a TCP listener
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	localAddr := listener.Addr().String()

	// Accept connections in background
	received := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		buf := make([]byte, 1024)
		n, _ := conn.Read(buf)
		received <- string(buf[:n])
	}()

	// Create syslog output
	config := &SyslogConfig{
		Network:     "tcp",
		Address:     localAddr,
		Facility:    FacilityLocal0,
		AppName:     "test",
		DialTimeout: time.Second,
	}

	output, err := NewSyslogOutput(config)
	if err != nil {
		t.Fatalf("NewSyslogOutput() error = %v", err)
	}
	defer output.Close()

	// Create formatter and format a message
	formatter := NewSyslogFormatter(FacilityLocal0, "test")
	entry := &Entry{
		Timestamp: time.Now(),
		Level:     LevelInfo,
		Logger:    "test",
		Message:   "TCP test message",
	}

	data, err := formatter.Format(entry)
	if err != nil {
		t.Fatalf("Format() error = %v", err)
	}

	// Write to syslog
	err = output.Write(data)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	// Wait for message
	select {
	case msg := <-received:
		if !strings.Contains(msg, "TCP test message") {
			t.Errorf("Expected message in received data, got: %s", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for TCP message")
	}
}

func TestSyslogOutputClose(t *testing.T) {
	// Start a UDP listener for connection
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().String()

	config := &SyslogConfig{
		Network:     "udp",
		Address:     localAddr,
		Facility:    FacilityLocal0,
		AppName:     "test",
		DialTimeout: time.Second,
	}

	output, err := NewSyslogOutput(config)
	if err != nil {
		t.Fatalf("NewSyslogOutput() error = %v", err)
	}

	// Close output
	err = output.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Write should fail after close
	err = output.Write([]byte("test"))
	if err == nil {
		t.Error("Expected error writing to closed output")
	}
}

func TestDefaultSyslogConfig(t *testing.T) {
	config := DefaultSyslogConfig()

	if config.Network != "unix" {
		t.Errorf("Expected network 'unix', got %s", config.Network)
	}

	if config.Address != "/dev/log" {
		t.Errorf("Expected address '/dev/log', got %s", config.Address)
	}

	if config.Facility != FacilityLocal0 {
		t.Errorf("Expected facility FacilityLocal0, got %v", config.Facility)
	}

	if config.DialTimeout != 5*time.Second {
		t.Errorf("Expected dial timeout 5s, got %v", config.DialTimeout)
	}
}

func TestSyslogFormatterWithMetadata(t *testing.T) {
	formatter := NewSyslogFormatter(FacilityLocal0, "test-app")
	formatter.Hostname = "testhost"

	entry := &Entry{
		Timestamp: time.Now(),
		Level:     LevelInfo,
		Logger:    "test",
		Message:   "Test with metadata",
		Metadata: &EntryMetadata{
			Host:    "app-server-01",
			Service: "api",
			Version: "1.2.3",
			PID:     12345,
		},
	}

	data, err := formatter.Format(entry)
	if err != nil {
		t.Fatalf("Format() error = %v", err)
	}

	msg := string(data)

	// Check metadata in structured data
	if !strings.Contains(msg, `host="app-server-01"`) {
		t.Errorf("Expected host in structured data, got: %s", msg)
	}

	if !strings.Contains(msg, `service="api"`) {
		t.Errorf("Expected service in structured data, got: %s", msg)
	}

	if !strings.Contains(msg, `version="1.2.3"`) {
		t.Errorf("Expected version in structured data, got: %s", msg)
	}
}

func TestSyslogPriorities(t *testing.T) {
	tests := []struct {
		facility SyslogFacility
		level    Level
		expected int
	}{
		{FacilityKern, LevelDebug, 7},     // 0*8 + 7
		{FacilityUser, LevelInfo, 14},     // 1*8 + 6
		{FacilityDaemon, LevelWarn, 28},   // 3*8 + 4
		{FacilityAuth, LevelError, 35},    // 4*8 + 3
		{FacilityLocal0, LevelInfo, 134},  // 16*8 + 6
		{FacilityLocal7, LevelError, 187}, // 23*8 + 3
	}

	for _, tt := range tests {
		t.Run(tt.facility.String(), func(t *testing.T) {
			severity := LevelToSeverity(tt.level)
			priority := int(tt.facility)*8 + int(severity)
			if priority != tt.expected {
				t.Errorf("Priority = %d, want %d", priority, tt.expected)
			}
		})
	}
}

// String method for SyslogFacility for test output
func (f SyslogFacility) String() string {
	names := []string{
		"kern", "user", "mail", "daemon", "auth", "syslog", "lpr", "news",
		"uucp", "cron", "authpriv", "ftp", "ntp", "audit", "alert", "clock",
		"local0", "local1", "local2", "local3", "local4", "local5", "local6", "local7",
	}
	if int(f) < len(names) {
		return names[f]
	}
	return "unknown"
}
