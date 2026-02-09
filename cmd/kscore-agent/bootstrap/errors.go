package bootstrap

import (
	"errors"
	"fmt"
	"strings"
)

// ErrorCategory classifies bootstrap errors for targeted recovery.
type ErrorCategory string

const (
	// ErrorCategoryPermission indicates insufficient permissions.
	ErrorCategoryPermission ErrorCategory = "permission"
	// ErrorCategoryNetwork indicates network connectivity issues.
	ErrorCategoryNetwork ErrorCategory = "network"
	// ErrorCategoryConfig indicates configuration errors.
	ErrorCategoryConfig ErrorCategory = "config"
	// ErrorCategoryDependency indicates missing dependencies.
	ErrorCategoryDependency ErrorCategory = "dependency"
	// ErrorCategoryResource indicates insufficient resources.
	ErrorCategoryResource ErrorCategory = "resource"
	// ErrorCategoryService indicates service startup/management issues.
	ErrorCategoryService ErrorCategory = "service"
	// ErrorCategoryDatabase indicates database connectivity/setup issues.
	ErrorCategoryDatabase ErrorCategory = "database"
	// ErrorCategoryTLS indicates TLS/certificate issues.
	ErrorCategoryTLS ErrorCategory = "tls"
	// ErrorCategoryPackage indicates package installation issues.
	ErrorCategoryPackage ErrorCategory = "package"
	// ErrorCategoryFileSystem indicates filesystem issues.
	ErrorCategoryFileSystem ErrorCategory = "filesystem"
	// ErrorCategoryTimeout indicates operation timeout.
	ErrorCategoryTimeout ErrorCategory = "timeout"
	// ErrorCategoryUnknown indicates an unclassified error.
	ErrorCategoryUnknown ErrorCategory = "unknown"
)

// ErrorSeverity indicates how severe an error is.
type ErrorSeverity string

const (
	// SeverityCritical means bootstrap cannot continue.
	SeverityCritical ErrorSeverity = "critical"
	// SeverityError means the current phase failed but rollback may help.
	SeverityError ErrorSeverity = "error"
	// SeverityWarning means something went wrong but bootstrap can continue.
	SeverityWarning ErrorSeverity = "warning"
)

// RecoveryType indicates the type of recovery action.
type RecoveryType string

const (
	// RecoveryTypeAutomatic can be performed without user intervention.
	RecoveryTypeAutomatic RecoveryType = "automatic"
	// RecoveryTypeManual requires user action.
	RecoveryTypeManual RecoveryType = "manual"
	// RecoveryTypeInteractive requires user confirmation.
	RecoveryTypeInteractive RecoveryType = "interactive"
)

// RecoveryRisk indicates the risk level of a recovery action.
type RecoveryRisk string

const (
	// RiskLow - safe to perform, no data loss.
	RiskLow RecoveryRisk = "low"
	// RiskMedium - may affect configuration.
	RiskMedium RecoveryRisk = "medium"
	// RiskHigh - may cause data loss or require manual cleanup.
	RiskHigh RecoveryRisk = "high"
)

// RecoveryAction describes a possible recovery action.
type RecoveryAction struct {
	// ID is a unique identifier for the action.
	ID string `json:"id"`

	// Description explains what the action does.
	Description string `json:"description"`

	// Type indicates whether automatic, manual, or interactive.
	Type RecoveryType `json:"type"`

	// Risk indicates the risk level.
	Risk RecoveryRisk `json:"risk"`

	// Command is the shell command for manual execution (if applicable).
	Command string `json:"command,omitempty"`

	// Commands is a list of commands for multi-step recovery.
	Commands []string `json:"commands,omitempty"`

	// Precondition describes what must be true before attempting.
	Precondition string `json:"precondition,omitempty"`

	// ExpectedOutcome describes what should happen after recovery.
	ExpectedOutcome string `json:"expected_outcome,omitempty"`
}

// Error is a rich error type with classification and recovery info.
type Error struct {
	// Original is the underlying error.
	Original error

	// Category classifies the error type.
	Category ErrorCategory

	// Severity indicates how severe the error is.
	Severity ErrorSeverity

	// Phase is the bootstrap phase where the error occurred.
	Phase PhaseName

	// Component is the specific component that failed (if applicable).
	Component string

	// Message is a human-friendly error description.
	Message string

	// Details provides additional context.
	Details map[string]string

	// RecoveryActions lists possible recovery actions.
	RecoveryActions []RecoveryAction

	// RelatedErrors captures related/nested errors.
	RelatedErrors []error

	// Suggestion is a brief suggestion for the user.
	Suggestion string
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Original != nil {
		return e.Original.Error()
	}
	return "bootstrap error"
}

// Unwrap returns the underlying error.
func (e *Error) Unwrap() error {
	return e.Original
}

// IsRetryable returns true if the error might succeed on retry.
func (e *Error) IsRetryable() bool {
	switch e.Category {
	case ErrorCategoryNetwork, ErrorCategoryTimeout, ErrorCategoryService:
		return true
	default:
		return false
	}
}

// HasAutomaticRecovery returns true if any automatic recovery action exists.
func (e *Error) HasAutomaticRecovery() bool {
	for i := range e.RecoveryActions {
		if e.RecoveryActions[i].Type == RecoveryTypeAutomatic {
			return true
		}
	}
	return false
}

// GetAutomaticRecoveryActions returns only automatic recovery actions.
func (e *Error) GetAutomaticRecoveryActions() []RecoveryAction {
	var actions []RecoveryAction
	for i := range e.RecoveryActions {
		if e.RecoveryActions[i].Type == RecoveryTypeAutomatic {
			actions = append(actions, e.RecoveryActions[i])
		}
	}
	return actions
}

// ClassifyError analyzes an error and returns a classified BootstrapError.
func ClassifyError(err error, phase PhaseName) *Error {
	if err == nil {
		return nil
	}

	// Check if already a BootstrapError
	var bErr *Error
	if errors.As(err, &bErr) {
		bErr.Phase = phase
		return bErr
	}

	msg := strings.ToLower(err.Error())
	classified := &Error{
		Original: err,
		Phase:    phase,
		Message:  err.Error(),
		Details:  make(map[string]string),
	}

	// Classify by error message patterns.
	// Order matters! More specific patterns must come before general ones.
	switch {
	case containsAny(msg, "permission denied", "access denied", "operation not permitted", "eacces"):
		classified.Category = ErrorCategoryPermission
		classified.Severity = SeverityCritical
		classified.Suggestion = "Run bootstrap with elevated privileges (sudo)"
		classified.RecoveryActions = permissionRecoveryActions()

	case containsAny(msg, "certificate", "x509", "tls:", "ssl", "handshake failure"):
		// TLS errors - check before network since TLS handshake failures are TLS-specific
		classified.Category = ErrorCategoryTLS
		classified.Severity = SeverityError
		classified.Suggestion = "Check TLS certificate configuration"
		classified.RecoveryActions = tlsRecoveryActions(err)

	case containsAny(msg, "timeout", "deadline exceeded", "context deadline"):
		classified.Category = ErrorCategoryTimeout
		classified.Severity = SeverityError
		classified.Suggestion = "The operation timed out - check system load and network latency"
		classified.RecoveryActions = timeoutRecoveryActions()

	case containsAny(msg, "no space left", "disk full", "enospc"):
		classified.Category = ErrorCategoryResource
		classified.Severity = SeverityCritical
		classified.Suggestion = "Free up disk space before retrying"
		classified.RecoveryActions = diskSpaceRecoveryActions()

	case containsAny(msg, "out of memory", "cannot allocate", "enomem"):
		classified.Category = ErrorCategoryResource
		classified.Severity = SeverityCritical
		classified.Suggestion = "Increase available memory or reduce resource requirements"
		classified.RecoveryActions = memoryRecoveryActions()

	case containsAny(msg, "package", "apt:", "apt-get", "dnf:", "yum:", "zypper:", "apk:", "dpkg:"):
		// Package manager errors - check before database since dpkg messages may contain "database"
		classified.Category = ErrorCategoryPackage
		classified.Severity = SeverityError
		classified.Suggestion = "Check package manager configuration and network access"
		classified.RecoveryActions = packageRecoveryActions(err)

	case containsAny(msg, "service", "systemctl", "systemd", "unit"):
		classified.Category = ErrorCategoryService
		classified.Severity = SeverityError
		classified.Suggestion = "Check service configuration and dependencies"
		classified.RecoveryActions = serviceRecoveryActions(err)

	case containsAny(msg, "is required", "missing nats", "missing value", "cannot be empty"):
		// Config validation errors - check before database since config errors may mention "postgres"
		// Use specific patterns to avoid false positives
		classified.Category = ErrorCategoryConfig
		classified.Severity = SeverityError
		classified.Suggestion = "Review bootstrap configuration"
		classified.RecoveryActions = configRecoveryActions(err)

	case containsAny(msg, "pq:", "postgres:", "postgresql:", "psql:", "database '", "' does not exist"):
		// Database errors - use specific patterns to avoid matching config/package manager messages
		classified.Category = ErrorCategoryDatabase
		classified.Severity = SeverityError
		classified.Suggestion = "Verify database connectivity and credentials"
		classified.RecoveryActions = databaseRecoveryActions(err)

	case containsAny(msg, "connection refused", "no route to host", "network unreachable", "host unreachable"):
		// Network errors - check after TLS/database to avoid misclassifying db connection errors
		classified.Category = ErrorCategoryNetwork
		classified.Severity = SeverityError
		classified.Suggestion = "Check network connectivity and firewall rules"
		classified.RecoveryActions = networkRecoveryActions(err)

	case containsAny(msg, "file", "directory", "path", "enoent", "no such file"):
		classified.Category = ErrorCategoryFileSystem
		classified.Severity = SeverityError
		classified.Suggestion = "Verify required files and directories exist"
		classified.RecoveryActions = filesystemRecoveryActions(err)

	case containsAny(msg, "required", "missing", "invalid", "must be"):
		// General config errors - last resort for config validation messages
		classified.Category = ErrorCategoryConfig
		classified.Severity = SeverityError
		classified.Suggestion = "Review bootstrap configuration"
		classified.RecoveryActions = configRecoveryActions(err)

	default:
		classified.Category = ErrorCategoryUnknown
		classified.Severity = SeverityError
		classified.Suggestion = "Review the error message and logs for more details"
	}

	return classified
}

// containsAny returns true if s contains any of the substrings.
func containsAny(s string, substrings ...string) bool {
	for _, sub := range substrings {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// Recovery action generators for each category

func permissionRecoveryActions() []RecoveryAction {
	return []RecoveryAction{
		{
			ID:              "run-as-root",
			Description:     "Run bootstrap with root privileges",
			Type:            RecoveryTypeManual,
			Risk:            RiskLow,
			Command:         "sudo kscore-agent bootstrap [your-options]",
			ExpectedOutcome: "Bootstrap runs with sufficient permissions",
		},
		{
			ID:          "check-file-permissions",
			Description: "Check and fix file permissions",
			Type:        RecoveryTypeManual,
			Risk:        RiskLow,
			Commands: []string{
				"ls -la /etc/keystone-core/",
				"ls -la /var/lib/keystone-core/",
				"sudo chown -R root:root /etc/keystone-core/",
			},
			ExpectedOutcome: "Files have correct ownership and permissions",
		},
	}
}

func networkRecoveryActions(err error) []RecoveryAction {
	actions := []RecoveryAction{
		{
			ID:          "check-connectivity",
			Description: "Verify network connectivity",
			Type:        RecoveryTypeManual,
			Risk:        RiskLow,
			Commands: []string{
				"ping -c 3 8.8.8.8",
				"curl -v https://api.keystone.io/health",
			},
			ExpectedOutcome: "Network connectivity is confirmed",
		},
		{
			ID:          "check-firewall",
			Description: "Check firewall rules",
			Type:        RecoveryTypeManual,
			Risk:        RiskLow,
			Commands: []string{
				"sudo iptables -L -n",
				"sudo firewall-cmd --list-all",
			},
			ExpectedOutcome: "Required ports are open",
		},
		{
			ID:          "check-dns",
			Description: "Verify DNS resolution",
			Type:        RecoveryTypeManual,
			Risk:        RiskLow,
			Commands: []string{
				"cat /etc/resolv.conf",
				"nslookup api.keystone.io",
			},
			ExpectedOutcome: "DNS resolution works correctly",
		},
	}

	// Add specific port check if we can extract the target
	msg := err.Error()
	if strings.Contains(msg, ":") {
		// Try to extract host:port pattern
		actions = append(actions, RecoveryAction{
			ID:              "check-port-access",
			Description:     "Test connectivity to target service",
			Type:            RecoveryTypeManual,
			Risk:            RiskLow,
			Command:         "nc -zv <host> <port>",
			ExpectedOutcome: "Port is reachable",
		})
	}

	return actions
}

func timeoutRecoveryActions() []RecoveryAction {
	return []RecoveryAction{
		{
			ID:              "retry-operation",
			Description:     "Retry the operation",
			Type:            RecoveryTypeAutomatic,
			Risk:            RiskLow,
			ExpectedOutcome: "Operation completes successfully",
		},
		{
			ID:          "check-system-load",
			Description: "Check system load and resource usage",
			Type:        RecoveryTypeManual,
			Risk:        RiskLow,
			Commands: []string{
				"uptime",
				"top -bn1 | head -20",
				"free -h",
				"df -h",
			},
			ExpectedOutcome: "System has adequate resources",
		},
		{
			ID:              "increase-timeout",
			Description:     "Increase operation timeout",
			Type:            RecoveryTypeManual,
			Risk:            RiskLow,
			Command:         "kscore-agent bootstrap --timeout 600s [your-options]",
			ExpectedOutcome: "Operation completes with extended timeout",
		},
	}
}

func diskSpaceRecoveryActions() []RecoveryAction {
	return []RecoveryAction{
		{
			ID:          "check-disk-usage",
			Description: "Identify disk space usage",
			Type:        RecoveryTypeManual,
			Risk:        RiskLow,
			Commands: []string{
				"df -h",
				"du -sh /var/log/* | sort -h | tail -20",
				"du -sh /var/cache/* | sort -h | tail -10",
			},
			ExpectedOutcome: "Large files/directories identified",
		},
		{
			ID:          "clean-package-cache",
			Description: "Clean package manager cache",
			Type:        RecoveryTypeInteractive,
			Risk:        RiskLow,
			Commands: []string{
				"sudo apt-get clean",
				"sudo dnf clean all",
				"sudo yum clean all",
			},
			Precondition:    "Package manager cache exists",
			ExpectedOutcome: "Cache space freed",
		},
		{
			ID:          "clean-old-logs",
			Description: "Rotate and clean old logs",
			Type:        RecoveryTypeInteractive,
			Risk:        RiskMedium,
			Commands: []string{
				"sudo journalctl --vacuum-time=7d",
				"sudo find /var/log -name '*.gz' -mtime +30 -delete",
			},
			Precondition:    "Old logs can be safely removed",
			ExpectedOutcome: "Log space freed",
		},
	}
}

func memoryRecoveryActions() []RecoveryAction {
	return []RecoveryAction{
		{
			ID:          "check-memory-usage",
			Description: "Check current memory usage",
			Type:        RecoveryTypeManual,
			Risk:        RiskLow,
			Commands: []string{
				"free -h",
				"ps aux --sort=-%mem | head -20",
			},
			ExpectedOutcome: "Memory-intensive processes identified",
		},
		{
			ID:              "clear-caches",
			Description:     "Clear system caches",
			Type:            RecoveryTypeInteractive,
			Risk:            RiskLow,
			Command:         "sudo sync && sudo echo 3 > /proc/sys/vm/drop_caches",
			Precondition:    "System has cached data that can be freed",
			ExpectedOutcome: "System caches cleared",
		},
	}
}

func databaseRecoveryActions(err error) []RecoveryAction {
	msg := strings.ToLower(err.Error())
	actions := []RecoveryAction{
		{
			ID:              "check-db-connectivity",
			Description:     "Test database connectivity",
			Type:            RecoveryTypeManual,
			Risk:            RiskLow,
			Command:         "psql -h <host> -U <user> -d <database> -c 'SELECT 1'",
			ExpectedOutcome: "Database connection succeeds",
		},
	}

	if strings.Contains(msg, "authentication") || strings.Contains(msg, "password") {
		actions = append(actions, RecoveryAction{
			ID:          "verify-db-credentials",
			Description: "Verify database credentials",
			Type:        RecoveryTypeManual,
			Risk:        RiskLow,
			Commands: []string{
				"echo 'Check KSCORE_POSTGRES_PASSWORD environment variable'",
				"echo 'Verify --postgres-password flag value'",
			},
			ExpectedOutcome: "Credentials are correct",
		})
	}

	if strings.Contains(msg, "does not exist") {
		actions = append(actions, RecoveryAction{
			ID:              "create-database",
			Description:     "Create the database",
			Type:            RecoveryTypeManual,
			Risk:            RiskMedium,
			Command:         "createdb -h <host> -U <user> <database>",
			Precondition:    "User has CREATE DATABASE privilege",
			ExpectedOutcome: "Database created successfully",
		})
	}

	return actions
}

func tlsRecoveryActions(err error) []RecoveryAction {
	msg := strings.ToLower(err.Error())
	actions := []RecoveryAction{
		{
			ID:          "verify-cert-files",
			Description: "Verify certificate files exist and are readable",
			Type:        RecoveryTypeManual,
			Risk:        RiskLow,
			Commands: []string{
				"ls -la /etc/keystone-core/tls/",
				"openssl x509 -in /etc/keystone-core/tls/server.crt -text -noout | head -20",
			},
			ExpectedOutcome: "Certificate files are valid",
		},
	}

	if strings.Contains(msg, "expired") {
		actions = append(actions, RecoveryAction{
			ID:          "renew-certificates",
			Description: "Renew expired certificates",
			Type:        RecoveryTypeManual,
			Risk:        RiskMedium,
			Commands: []string{
				"kscore-agent bootstrap --generate-certs [your-options]",
				"# Or renew with your CA",
			},
			ExpectedOutcome: "Valid certificates installed",
		})
	}

	if strings.Contains(msg, "self signed") || strings.Contains(msg, "unknown authority") {
		actions = append(actions, RecoveryAction{
			ID:          "configure-ca",
			Description: "Configure CA trust",
			Type:        RecoveryTypeManual,
			Risk:        RiskLow,
			Commands: []string{
				"# Copy CA cert to trust store",
				"sudo cp /etc/keystone-core/tls/ca.crt /usr/local/share/ca-certificates/",
				"sudo update-ca-certificates",
			},
			ExpectedOutcome: "CA is trusted by the system",
		})
	}

	return actions
}

func packageRecoveryActions(err error) []RecoveryAction {
	msg := strings.ToLower(err.Error())
	actions := []RecoveryAction{
		{
			ID:          "update-package-lists",
			Description: "Update package manager lists",
			Type:        RecoveryTypeAutomatic,
			Risk:        RiskLow,
			Commands: []string{
				"sudo apt-get update",
				"sudo dnf check-update",
				"sudo yum check-update",
			},
			ExpectedOutcome: "Package lists are current",
		},
	}

	if strings.Contains(msg, "not found") || strings.Contains(msg, "unable to locate") {
		actions = append(actions, RecoveryAction{
			ID:          "add-repository",
			Description: "Add Keystone package repository",
			Type:        RecoveryTypeManual,
			Risk:        RiskLow,
			Commands: []string{
				"# For Debian/Ubuntu:",
				"curl -fsSL https://packages.keystone.io/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/keystone.gpg",
				"echo 'deb [signed-by=/etc/apt/keyrings/keystone.gpg] https://packages.keystone.io/apt stable main' | sudo tee /etc/apt/sources.list.d/keystone.list",
				"sudo apt-get update",
			},
			ExpectedOutcome: "Keystone repository is configured",
		})
	}

	if strings.Contains(msg, "lock") || strings.Contains(msg, "dpkg") {
		actions = append(actions, RecoveryAction{
			ID:          "fix-package-lock",
			Description: "Fix package manager lock",
			Type:        RecoveryTypeInteractive,
			Risk:        RiskMedium,
			Commands: []string{
				"sudo rm -f /var/lib/dpkg/lock-frontend",
				"sudo rm -f /var/lib/dpkg/lock",
				"sudo dpkg --configure -a",
			},
			Precondition:    "No other package operations are running",
			ExpectedOutcome: "Package manager unlocked",
		})
	}

	return actions
}

func serviceRecoveryActions(err error) []RecoveryAction {
	msg := strings.ToLower(err.Error())
	actions := []RecoveryAction{
		{
			ID:          "check-service-status",
			Description: "Check service status and logs",
			Type:        RecoveryTypeManual,
			Risk:        RiskLow,
			Commands: []string{
				"sudo systemctl status kscore-server",
				"sudo systemctl status kscore-agent",
				"sudo journalctl -u kscore-server -n 50 --no-pager",
				"sudo journalctl -u kscore-agent -n 50 --no-pager",
			},
			ExpectedOutcome: "Service status and errors identified",
		},
	}

	if strings.Contains(msg, "failed to start") || strings.Contains(msg, "activating") {
		actions = append(actions, RecoveryAction{
			ID:          "restart-service",
			Description: "Restart the service",
			Type:        RecoveryTypeAutomatic,
			Risk:        RiskLow,
			Commands: []string{
				"sudo systemctl daemon-reload",
				"sudo systemctl restart kscore-server",
				"sudo systemctl restart kscore-agent",
			},
			ExpectedOutcome: "Service starts successfully",
		})
	}

	if strings.Contains(msg, "dependency") {
		actions = append(actions, RecoveryAction{
			ID:          "check-dependencies",
			Description: "Check service dependencies",
			Type:        RecoveryTypeManual,
			Risk:        RiskLow,
			Commands: []string{
				"systemctl list-dependencies kscore-server",
				"systemctl list-dependencies kscore-agent",
			},
			ExpectedOutcome: "Missing dependencies identified",
		})
	}

	return actions
}

func filesystemRecoveryActions(err error) []RecoveryAction {
	return []RecoveryAction{
		{
			ID:          "create-directories",
			Description: "Create required directories",
			Type:        RecoveryTypeAutomatic,
			Risk:        RiskLow,
			Commands: []string{
				"sudo mkdir -p /etc/keystone-core",
				"sudo mkdir -p /var/lib/keystone-core",
				"sudo mkdir -p /var/log/keystone-core",
			},
			ExpectedOutcome: "Required directories exist",
		},
		{
			ID:          "fix-permissions",
			Description: "Fix directory permissions",
			Type:        RecoveryTypeManual,
			Risk:        RiskLow,
			Commands: []string{
				"sudo chown -R root:root /etc/keystone-core",
				"sudo chmod 755 /etc/keystone-core",
				"sudo chmod 750 /var/lib/keystone-core",
			},
			ExpectedOutcome: "Directories have correct permissions",
		},
	}
}

func configRecoveryActions(err error) []RecoveryAction {
	msg := strings.ToLower(err.Error())
	actions := []RecoveryAction{
		{
			ID:              "validate-config",
			Description:     "Validate bootstrap configuration",
			Type:            RecoveryTypeManual,
			Risk:            RiskLow,
			Command:         "kscore-agent bootstrap --dry-run [your-options]",
			ExpectedOutcome: "Configuration is valid",
		},
	}

	if strings.Contains(msg, "postgres") {
		actions = append(actions, RecoveryAction{
			ID:          "set-postgres-config",
			Description: "Set PostgreSQL configuration",
			Type:        RecoveryTypeManual,
			Risk:        RiskLow,
			Commands: []string{
				"export KSCORE_POSTGRES_HOST=<host>",
				"export KSCORE_POSTGRES_USER=<user>",
				"export KSCORE_POSTGRES_PASSWORD=<password>",
				"export KSCORE_POSTGRES_DATABASE=<database>",
			},
			ExpectedOutcome: "PostgreSQL configuration is set",
		})
	}

	if strings.Contains(msg, "nats") {
		actions = append(actions, RecoveryAction{
			ID:          "set-nats-config",
			Description: "Set NATS configuration",
			Type:        RecoveryTypeManual,
			Risk:        RiskLow,
			Commands: []string{
				"# For external NATS:",
				"export KSCORE_NATS_URLS=nats://nats-1:4222,nats://nats-2:4222",
				"# Or use embedded mode:",
				"kscore-agent bootstrap --nats-mode embedded [your-options]",
			},
			ExpectedOutcome: "NATS configuration is set",
		})
	}

	return actions
}

// FormatRecoveryActions formats recovery actions for display.
func FormatRecoveryActions(actions []RecoveryAction, verbose bool) string {
	if len(actions) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("Recovery Actions:\n")

	for i := range actions {
		action := &actions[i]
		builder.WriteString(fmt.Sprintf("\n%d. %s\n", i+1, action.Description))
		builder.WriteString(fmt.Sprintf("   Type: %s | Risk: %s\n", action.Type, action.Risk))

		if action.Precondition != "" && verbose {
			builder.WriteString(fmt.Sprintf("   Precondition: %s\n", action.Precondition))
		}

		if action.Command != "" {
			builder.WriteString(fmt.Sprintf("   Command: %s\n", action.Command))
		}

		if len(action.Commands) > 0 {
			builder.WriteString("   Commands:\n")
			for _, cmd := range action.Commands {
				builder.WriteString(fmt.Sprintf("     $ %s\n", cmd))
			}
		}

		if action.ExpectedOutcome != "" && verbose {
			builder.WriteString(fmt.Sprintf("   Expected: %s\n", action.ExpectedOutcome))
		}
	}

	return builder.String()
}
