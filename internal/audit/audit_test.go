package audit

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestDefaultAuditConfig(t *testing.T) {
	config := DefaultAuditConfig()

	if config.Level != AuditLevelAll {
		t.Errorf("Expected level AuditLevelAll, got %v", config.Level)
	}

	if config.Backend != "auto" {
		t.Errorf("Expected backend 'auto', got %s", config.Backend)
	}

	if config.Facility != "auth" {
		t.Errorf("Expected facility 'auth', got %s", config.Facility)
	}

	if len(config.RedactPatterns) == 0 {
		t.Error("Expected default redact patterns")
	}
}

func TestNewAuditorWithNoneLevel(t *testing.T) {
	config := &AuditConfig{
		Level: AuditLevelNone,
	}

	auditor, err := NewAuditor("test-tool", config)
	if err != nil {
		t.Fatalf("NewAuditor() error = %v", err)
	}
	defer auditor.Close()

	// Should create a NoopAuditLogger
	entry := auditor.StartEntry(ActionCommandExecuted, "test")
	err = auditor.Log(context.Background(), entry)
	if err != nil {
		t.Errorf("Log() error = %v", err)
	}
}

func TestNewAuditorWithStderr(t *testing.T) {
	config := &AuditConfig{
		Level:   AuditLevelAll,
		Backend: "stderr",
	}

	auditor, err := NewAuditor("test-tool", config)
	if err != nil {
		t.Fatalf("NewAuditor() error = %v", err)
	}
	defer auditor.Close()

	if auditor.tool != "test-tool" {
		t.Errorf("Expected tool 'test-tool', got %s", auditor.tool)
	}
}

func TestAuditorStartEntry(t *testing.T) {
	config := &AuditConfig{
		Level:   AuditLevelAll,
		Backend: "none",
	}

	auditor, err := NewAuditor("kscore-exec", config)
	if err != nil {
		t.Fatalf("NewAuditor() error = %v", err)
	}
	defer auditor.Close()

	entry := auditor.StartEntry(ActionCommandExecuted, "run")

	if entry.Tool != "kscore-exec" {
		t.Errorf("Expected tool 'kscore-exec', got %s", entry.Tool)
	}

	if entry.Command != "run" {
		t.Errorf("Expected command 'run', got %s", entry.Command)
	}

	if entry.AuditType != ActionCommandExecuted {
		t.Errorf("Expected action %v, got %v", ActionCommandExecuted, entry.AuditType)
	}

	if entry.PID == 0 {
		t.Error("Expected PID to be set")
	}

	if entry.CorrelationID == "" {
		t.Error("Expected correlation ID to be set")
	}

	if entry.Timestamp.IsZero() {
		t.Error("Expected timestamp to be set")
	}
}

func TestAuditorLogWithErrorsLevel(t *testing.T) {
	memLogger := NewMemoryAuditLogger()

	auditor := &Auditor{
		config: &AuditConfig{
			Level: AuditLevelErrors,
		},
		backend: memLogger,
		tool:    "test",
	}

	ctx := context.Background()

	// Success should not be logged
	successEntry := &AuditEntry{
		Result: ResultSuccess,
	}
	err := auditor.Log(ctx, successEntry)
	if err != nil {
		t.Errorf("Log() error = %v", err)
	}

	if len(memLogger.Entries()) != 0 {
		t.Error("Expected success entry to not be logged at errors level")
	}

	// Failure should be logged
	failureEntry := &AuditEntry{
		Result: ResultFailure,
	}
	err = auditor.Log(ctx, failureEntry)
	if err != nil {
		t.Errorf("Log() error = %v", err)
	}

	if len(memLogger.Entries()) != 1 {
		t.Error("Expected failure entry to be logged at errors level")
	}
}

func TestAuditorRedactArgs(t *testing.T) {
	auditor := &Auditor{
		config: &AuditConfig{
			RedactPatterns: []string{"password", "token", "secret"},
		},
	}

	tests := []struct {
		name     string
		args     []string
		expected []string
	}{
		{
			name:     "no sensitive args",
			args:     []string{"--host", "localhost", "--port", "8080"},
			expected: []string{"--host", "localhost", "--port", "8080"},
		},
		{
			name:     "password with equals",
			args:     []string{"--password=secret123"},
			expected: []string{"--password=[REDACTED]"},
		},
		{
			name:     "password with next arg",
			args:     []string{"--password", "secret123"},
			expected: []string{"--password", "[REDACTED]"},
		},
		{
			name:     "token in arg name",
			args:     []string{"--api-token", "abc123"},
			expected: []string{"--api-token", "[REDACTED]"},
		},
		{
			name:     "mixed args",
			args:     []string{"--host", "localhost", "--password=secret", "--port", "8080"},
			expected: []string{"--host", "localhost", "--password=[REDACTED]", "--port", "8080"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := auditor.redactArgs(tt.args)
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d args, got %d", len(tt.expected), len(result))
				return
			}
			for i, arg := range result {
				if arg != tt.expected[i] {
					t.Errorf("Arg %d: expected %q, got %q", i, tt.expected[i], arg)
				}
			}
		})
	}
}

func TestAuditorLogCommand(t *testing.T) {
	memLogger := NewMemoryAuditLogger()

	auditor := &Auditor{
		config: &AuditConfig{
			Level: AuditLevelAll,
		},
		backend: memLogger,
		tool:    "kscore-exec",
	}

	ctx := context.Background()
	err := auditor.LogCommand(ctx, "run", []string{"--target", "web*"}, "web*", ResultSuccess, 0, 150*time.Millisecond, nil)
	if err != nil {
		t.Errorf("LogCommand() error = %v", err)
	}

	entries := memLogger.Entries()
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	entry := entries[0]
	if entry.Command != "run" {
		t.Errorf("Expected command 'run', got %s", entry.Command)
	}
	if entry.Target != "web*" {
		t.Errorf("Expected target 'web*', got %s", entry.Target)
	}
	if entry.Result != ResultSuccess {
		t.Errorf("Expected result success, got %v", entry.Result)
	}
	if entry.DurationMS != 150 {
		t.Errorf("Expected duration 150ms, got %d", entry.DurationMS)
	}
}

func TestGenerateCorrelationID(t *testing.T) {
	id1 := generateCorrelationID()
	id2 := generateCorrelationID()

	if id1 == "" {
		t.Error("Expected non-empty correlation ID")
	}

	if id1 == id2 {
		t.Error("Expected unique correlation IDs")
	}

	// Should be 16 hex characters (8 bytes)
	if len(id1) != 16 {
		t.Errorf("Expected 16 character ID, got %d", len(id1))
	}
}

func TestGlobalAuditorFunctions(t *testing.T) {
	// Initialize global auditor
	err := Init("test-tool", &AuditConfig{
		Level:   AuditLevelAll,
		Backend: "none",
	})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	defer Close()

	// StartEntry should work
	entry := StartEntry(ActionStateApplied, "apply")
	if entry.Tool != "test-tool" {
		t.Errorf("Expected tool 'test-tool', got %s", entry.Tool)
	}

	// Log should work
	entry.Result = ResultSuccess
	err = Log(context.Background(), entry)
	if err != nil {
		t.Errorf("Log() error = %v", err)
	}
}

func TestGlobalAuditorNotInitialized(t *testing.T) {
	// Reset global auditor
	globalAuditorMu.Lock()
	globalAuditor = nil
	globalAuditorMu.Unlock()

	// StartEntry should still return basic entry
	entry := StartEntry(ActionCommandExecuted, "test")
	if entry.Command != "test" {
		t.Errorf("Expected command 'test', got %s", entry.Command)
	}

	// Log should return error
	err := Log(context.Background(), entry)
	if err == nil {
		t.Error("Expected error when auditor not initialized")
	}
}

func TestAuditActions(t *testing.T) {
	actions := []AuditAction{
		ActionCommandExecuted,
		ActionStateApplied,
		ActionModuleInstalled,
		ActionModulePublished,
		ActionClusterJoined,
		ActionClusterLeft,
		ActionMigrationRun,
		ActionPolicyEvaluated,
		ActionGitOpsSync,
		ActionGitOpsRollback,
		ActionMonitorStarted,
		ActionAuthAttempt,
		ActionConfigChanged,
		ActionSecretsAccessed,
	}

	for _, action := range actions {
		if action == "" {
			t.Error("Action should not be empty")
		}
	}
}

func TestAuditResults(t *testing.T) {
	results := []AuditResult{
		ResultSuccess,
		ResultFailure,
		ResultDenied,
		ResultTimeout,
	}

	for _, result := range results {
		if result == "" {
			t.Error("Result should not be empty")
		}
	}
}

func TestAuditLevels(t *testing.T) {
	levels := []AuditLevel{
		AuditLevelAll,
		AuditLevelErrors,
		AuditLevelNone,
	}

	for _, level := range levels {
		if level == "" {
			t.Error("Level should not be empty")
		}
	}
}

func TestStderrAuditLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := &StderrAuditLogger{
		writer: &buf,
	}

	entry := &AuditEntry{
		Timestamp: time.Now(),
		AuditType: ActionCommandExecuted,
		User:      "testuser",
		Tool:      "kscore-exec",
		Command:   "run",
		Result:    ResultSuccess,
	}

	err := logger.Log(context.Background(), entry)
	if err != nil {
		t.Errorf("Log() error = %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "AUDIT:") {
		t.Error("Expected AUDIT prefix in output")
	}
	if !strings.Contains(output, "testuser") {
		t.Error("Expected user in output")
	}
	if !strings.Contains(output, "kscore-exec") {
		t.Error("Expected tool in output")
	}

	err = logger.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestMemoryAuditLogger(t *testing.T) {
	logger := NewMemoryAuditLogger()

	entry1 := &AuditEntry{
		Timestamp: time.Now(),
		Command:   "cmd1",
		Args:      []string{"arg1", "arg2"},
		Extra:     map[string]interface{}{"key": "value"},
	}

	entry2 := &AuditEntry{
		Timestamp: time.Now(),
		Command:   "cmd2",
	}

	ctx := context.Background()
	if err := logger.Log(ctx, entry1); err != nil {
		t.Errorf("Log() error = %v", err)
	}
	if err := logger.Log(ctx, entry2); err != nil {
		t.Errorf("Log() error = %v", err)
	}

	entries := logger.Entries()
	if len(entries) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(entries))
	}

	// Verify deep copy
	if entries[0].Command != "cmd1" {
		t.Errorf("Expected command 'cmd1', got %s", entries[0].Command)
	}
	if len(entries[0].Args) != 2 {
		t.Errorf("Expected 2 args, got %d", len(entries[0].Args))
	}
	if entries[0].Extra["key"] != "value" {
		t.Error("Expected extra field to be copied")
	}

	// Test clear
	logger.Clear()
	if len(logger.Entries()) != 0 {
		t.Error("Expected entries to be cleared")
	}

	// Test close
	if err := logger.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestMultiAuditLogger(t *testing.T) {
	mem1 := NewMemoryAuditLogger()
	mem2 := NewMemoryAuditLogger()

	multi := NewMultiAuditLogger(mem1, mem2)

	entry := &AuditEntry{
		Command: "test",
	}

	err := multi.Log(context.Background(), entry)
	if err != nil {
		t.Errorf("Log() error = %v", err)
	}

	// Both loggers should have the entry
	if len(mem1.Entries()) != 1 {
		t.Error("Expected entry in first logger")
	}
	if len(mem2.Entries()) != 1 {
		t.Error("Expected entry in second logger")
	}

	err = multi.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestTimeoutAuditLogger(t *testing.T) {
	mem := NewMemoryAuditLogger()
	timeout := NewTimeoutAuditLogger(mem, 1*time.Second)

	entry := &AuditEntry{
		Command: "test",
	}

	err := timeout.Log(context.Background(), entry)
	if err != nil {
		t.Errorf("Log() error = %v", err)
	}

	if len(mem.Entries()) != 1 {
		t.Error("Expected entry in underlying logger")
	}

	err = timeout.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestNoopAuditLogger(t *testing.T) {
	logger := &NoopAuditLogger{}

	entry := &AuditEntry{
		Command: "test",
	}

	err := logger.Log(context.Background(), entry)
	if err != nil {
		t.Errorf("Log() error = %v", err)
	}

	err = logger.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestUnknownBackend(t *testing.T) {
	config := &AuditConfig{
		Level:   AuditLevelAll,
		Backend: "unknown-backend",
	}

	_, err := NewAuditor("test", config)
	if err == nil {
		t.Error("Expected error for unknown backend")
	}
}

func TestAuditEntryExtraFields(t *testing.T) {
	entry := &AuditEntry{
		Timestamp:     time.Now(),
		AuditType:     ActionCommandExecuted,
		User:          "admin",
		UID:           1000,
		TTY:           "/dev/pts/0",
		PID:           12345,
		Tool:          "kscore-exec",
		Command:       "run",
		Args:          []string{"--target", "web*", "uptime"},
		Target:        "web*",
		AgentsMatched: 5,
		Result:        ResultSuccess,
		ExitCode:      0,
		DurationMS:    150,
		CorrelationID: "abc123",
		Error:         "",
		Extra:         map[string]interface{}{"custom": "value"},
		RemoteAddr:    "192.168.1.100",
	}

	// Verify all fields are set
	if entry.AgentsMatched != 5 {
		t.Errorf("Expected 5 agents matched, got %d", entry.AgentsMatched)
	}
	if entry.RemoteAddr != "192.168.1.100" {
		t.Errorf("Expected remote addr '192.168.1.100', got %s", entry.RemoteAddr)
	}
	if entry.Extra["custom"] != "value" {
		t.Error("Expected custom extra field")
	}
}
