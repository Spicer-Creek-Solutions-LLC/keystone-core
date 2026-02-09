// Package audit provides audit logging for CLI tools.
// Epic 15: Observability Enhancements - CLI audit logging to syslog/journald.
package audit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/user"
	"runtime"
	"strings"
	"sync"
	"time"
)

// AuditLevel controls what gets audited
//
//nolint:revive // stuttering is intentional to avoid conflict with logger.go types
type AuditLevel string

const (
	// AuditLevelAll logs all CLI actions
	AuditLevelAll AuditLevel = "all"
	// AuditLevelErrors logs only errors/failures
	AuditLevelErrors AuditLevel = "errors"
	// AuditLevelNone disables audit logging
	AuditLevelNone AuditLevel = "none"
)

// AuditAction represents the type of action being audited
//
//nolint:revive // stuttering is intentional to avoid conflict with logger.go types
type AuditAction string

// AuditAction constants define the types of actions that can be audited.
const (
	ActionCommandExecuted  AuditAction = "command_executed"
	ActionStateApplied     AuditAction = "state_applied"
	ActionModuleInstalled  AuditAction = "module_installed"
	ActionModulePublished  AuditAction = "module_published"
	ActionClusterJoined    AuditAction = "cluster_joined"
	ActionClusterLeft      AuditAction = "cluster_left"
	ActionMigrationRun     AuditAction = "migration_run"
	ActionPolicyEvaluated  AuditAction = "policy_evaluated"
	ActionGitOpsSync       AuditAction = "gitops_sync"
	ActionGitOpsRollback   AuditAction = "gitops_rollback"
	ActionMonitorStarted   AuditAction = "monitor_started"
	ActionAuthAttempt      AuditAction = "auth_attempt"
	ActionConfigChanged    AuditAction = "config_changed"
	ActionSecretsAccessed  AuditAction = "secrets_accessed"
	ActionDNSRecordCreated AuditAction = "dns_record_created"
	ActionDNSRecordUpdated AuditAction = "dns_record_updated"
	ActionDNSRecordDeleted AuditAction = "dns_record_deleted"
	ActionDNSSyncCompleted AuditAction = "dns_sync_completed"
)

// AuditResult represents the outcome of an action
//
//nolint:revive // stuttering is intentional to avoid conflict with logger.go types
type AuditResult string

// AuditResult constants define the possible outcomes of an audited action.
const (
	ResultSuccess AuditResult = "success"
	ResultFailure AuditResult = "failure"
	ResultDenied  AuditResult = "denied"
	ResultTimeout AuditResult = "timeout"
)

// AuditEntry represents a single audit log entry
//
//nolint:revive // stuttering is intentional to avoid conflict with logger.go types
type AuditEntry struct {
	// Timestamp is when the action occurred
	Timestamp time.Time `json:"timestamp"`

	// AuditType is the type of audit action
	AuditType AuditAction `json:"audit_type"`

	// User is the username who performed the action
	User string `json:"user"`

	// UID is the user ID
	UID int `json:"uid"`

	// TTY is the terminal device
	TTY string `json:"tty,omitempty"`

	// PID is the process ID
	PID int `json:"pid"`

	// Tool is the CLI tool name (e.g., kscore-exec)
	Tool string `json:"tool"`

	// Command is the subcommand executed
	Command string `json:"command"`

	// Args are the command-line arguments (redacted)
	Args []string `json:"args,omitempty"`

	// Target is the target of the action (e.g., agent selector)
	Target string `json:"target,omitempty"`

	// AgentsMatched is the number of agents matched (for exec commands)
	AgentsMatched int `json:"agents_matched,omitempty"`

	// Result is the outcome of the action
	Result AuditResult `json:"result"`

	// ExitCode is the exit code
	ExitCode int `json:"exit_code"`

	// DurationMS is the execution duration in milliseconds
	DurationMS int64 `json:"duration_ms"`

	// CorrelationID links related audit entries
	CorrelationID string `json:"correlation_id"`

	// Error is the error message if result is failure
	Error string `json:"error,omitempty"`

	// Extra contains additional action-specific data
	Extra map[string]interface{} `json:"extra,omitempty"`

	// RemoteAddr is the SSH client address if applicable
	RemoteAddr string `json:"remote_addr,omitempty"`
}

// AuditLogger is the interface for audit logging backends
//
//nolint:revive // stuttering is intentional to avoid conflict with logger.go types
type AuditLogger interface {
	// Log writes an audit entry
	Log(ctx context.Context, entry *AuditEntry) error

	// Close closes the audit logger
	Close() error
}

// AuditConfig contains audit logging configuration
//
//nolint:revive // stuttering is intentional to avoid conflict with logger.go types
type AuditConfig struct {
	// Level controls what gets logged
	Level AuditLevel

	// Backend is the audit backend to use
	Backend string // syslog, journald, stderr, none

	// Facility is the syslog facility (for syslog backend)
	Facility string

	// RedactPatterns are patterns to redact from arguments
	RedactPatterns []string
}

// DefaultAuditConfig returns default audit configuration
func DefaultAuditConfig() *AuditConfig {
	return &AuditConfig{
		Level:    AuditLevelAll,
		Backend:  "auto",
		Facility: "auth",
		RedactPatterns: []string{
			"password", "token", "secret", "key", "credential",
			"apikey", "api_key", "api-key", "auth",
		},
	}
}

// Auditor is the main audit logging coordinator
type Auditor struct {
	config  *AuditConfig
	backend AuditLogger
	tool    string
	mu      sync.RWMutex
}

// NewAuditor creates a new auditor for a CLI tool
func NewAuditor(ctx context.Context, tool string, config *AuditConfig) (*Auditor, error) {
	if config == nil {
		config = DefaultAuditConfig()
	}

	if config.Level == AuditLevelNone {
		return &Auditor{
			config:  config,
			backend: &NoopAuditLogger{},
			tool:    tool,
		}, nil
	}

	// Select backend
	backend, err := createBackend(ctx, config)
	if err != nil {
		return nil, err
	}

	return &Auditor{
		config:  config,
		backend: backend,
		tool:    tool,
	}, nil
}

// createBackend creates the appropriate audit backend
func createBackend(ctx context.Context, config *AuditConfig) (AuditLogger, error) {
	switch config.Backend {
	case "syslog":
		return NewSyslogAuditLogger(ctx, config.Facility)
	case "journald":
		return NewJournaldAuditLogger(ctx)
	case "stderr":
		return NewStderrAuditLogger(), nil
	case "auto", "":
		// Auto-detect based on platform
		return autoSelectBackend(ctx, config)
	case "none":
		return &NoopAuditLogger{}, nil
	default:
		return nil, fmt.Errorf("unknown audit backend: %s", config.Backend)
	}
}

// autoSelectBackend selects the best backend for the current platform
func autoSelectBackend(ctx context.Context, config *AuditConfig) (AuditLogger, error) {
	switch runtime.GOOS {
	case "linux":
		// Try journald first, fall back to syslog
		backend, err := NewJournaldAuditLogger(ctx)
		if err == nil {
			return backend, nil
		}
		return NewSyslogAuditLogger(ctx, config.Facility)
	case "darwin":
		// macOS: use syslog (ASL compatible)
		return NewSyslogAuditLogger(ctx, config.Facility)
	case "windows":
		// Windows: use stderr (Event Log would require cgo)
		return NewStderrAuditLogger(), nil
	default:
		// Unknown platform: stderr fallback
		return NewStderrAuditLogger(), nil
	}
}

// StartEntry creates a new audit entry with common fields populated
func (a *Auditor) StartEntry(action AuditAction, command string) *AuditEntry {
	entry := &AuditEntry{
		Timestamp:     time.Now(),
		AuditType:     action,
		Tool:          a.tool,
		Command:       command,
		PID:           os.Getpid(),
		CorrelationID: generateCorrelationID(),
	}

	// Get user information
	if u, err := user.Current(); err == nil {
		entry.User = u.Username
		// Parse UID (may be numeric string on Unix)
		fmt.Sscanf(u.Uid, "%d", &entry.UID)
	}

	// Get TTY
	if tty := os.Getenv("TTY"); tty != "" {
		entry.TTY = tty
	} else if stat, err := os.Stdin.Stat(); err == nil {
		if stat.Mode()&os.ModeCharDevice != 0 {
			// It's a terminal
			entry.TTY = "/dev/stdin"
		}
	}

	// Get SSH client info
	if sshClient := os.Getenv("SSH_CLIENT"); sshClient != "" {
		parts := strings.Fields(sshClient)
		if len(parts) > 0 {
			entry.RemoteAddr = parts[0]
		}
	}

	return entry
}

// Log logs an audit entry
func (a *Auditor) Log(ctx context.Context, entry *AuditEntry) error {
	a.mu.RLock()
	level := a.config.Level
	backend := a.backend
	a.mu.RUnlock()

	// Check if we should log based on level
	if level == AuditLevelNone {
		return nil
	}

	if level == AuditLevelErrors && entry.Result == ResultSuccess {
		return nil
	}

	// Redact sensitive arguments
	if entry.Args != nil {
		entry.Args = a.redactArgs(entry.Args)
	}

	return backend.Log(ctx, entry)
}

// redactArgs redacts sensitive values from arguments
func (a *Auditor) redactArgs(args []string) []string {
	redacted := make([]string, len(args))
	copy(redacted, args)

	for i, arg := range redacted {
		// Check if this is a sensitive flag
		for _, pattern := range a.config.RedactPatterns {
			lowerArg := strings.ToLower(arg)
			if strings.Contains(lowerArg, pattern) {
				// If it's a --key=value format, redact the value
				if idx := strings.Index(arg, "="); idx > 0 {
					redacted[i] = arg[:idx+1] + "[REDACTED]"
				} else if i+1 < len(redacted) {
					// If next arg might be the value, redact it
					redacted[i+1] = "[REDACTED]"
				}
				break
			}
		}
	}

	return redacted
}

// LogCommand is a convenience method for logging command execution
func (a *Auditor) LogCommand(ctx context.Context, command string, args []string, target string, result AuditResult, exitCode int, duration time.Duration, err error) error {
	entry := a.StartEntry(ActionCommandExecuted, command)
	entry.Args = args
	entry.Target = target
	entry.Result = result
	entry.ExitCode = exitCode
	entry.DurationMS = duration.Milliseconds()
	if err != nil {
		entry.Error = err.Error()
	}
	return a.Log(ctx, entry)
}

// Close closes the auditor
func (a *Auditor) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.backend != nil {
		return a.backend.Close()
	}
	return nil
}

// generateCorrelationID generates a unique correlation ID
func generateCorrelationID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// Global auditor for convenience
var (
	globalAuditor   *Auditor
	globalAuditorMu sync.RWMutex
)

// Init initializes the global auditor
func Init(ctx context.Context, tool string, config *AuditConfig) error {
	auditor, err := NewAuditor(ctx, tool, config)
	if err != nil {
		return err
	}

	globalAuditorMu.Lock()
	globalAuditor = auditor
	globalAuditorMu.Unlock()

	return nil
}

// Log logs an audit entry using the global auditor
func Log(ctx context.Context, entry *AuditEntry) error {
	globalAuditorMu.RLock()
	auditor := globalAuditor
	globalAuditorMu.RUnlock()

	if auditor == nil {
		return fmt.Errorf("auditor not initialized")
	}

	return auditor.Log(ctx, entry)
}

// StartEntry creates a new audit entry using the global auditor
func StartEntry(action AuditAction, command string) *AuditEntry {
	globalAuditorMu.RLock()
	auditor := globalAuditor
	globalAuditorMu.RUnlock()

	if auditor == nil {
		// Return a basic entry if auditor not initialized
		return &AuditEntry{
			Timestamp:     time.Now(),
			AuditType:     action,
			Command:       command,
			PID:           os.Getpid(),
			CorrelationID: generateCorrelationID(),
		}
	}

	return auditor.StartEntry(action, command)
}

// Close closes the global auditor
func Close() error {
	globalAuditorMu.Lock()
	defer globalAuditorMu.Unlock()

	if globalAuditor != nil {
		err := globalAuditor.Close()
		globalAuditor = nil
		return err
	}
	return nil
}
