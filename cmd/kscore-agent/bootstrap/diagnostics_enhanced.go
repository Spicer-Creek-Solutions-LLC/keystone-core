package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// DiagnosticReport is a structured diagnostic report.
type DiagnosticReport struct {
	// Metadata
	Timestamp   time.Time `json:"timestamp"`
	Version     string    `json:"version"`
	BootstrapID string    `json:"bootstrap_id,omitempty"`

	// Error information
	Phase       PhaseName           `json:"phase"`
	Error       *DiagnosticError    `json:"error,omitempty"`
	RootCause   string              `json:"root_cause,omitempty"`
	ErrorChain  []string            `json:"error_chain,omitempty"`

	// System information
	System      *DiagnosticSystem   `json:"system,omitempty"`
	Environment map[string]string   `json:"environment,omitempty"`

	// Configuration
	Config      *DiagnosticConfig   `json:"config,omitempty"`

	// Artifacts
	Artifacts   *DiagnosticArtifacts `json:"artifacts,omitempty"`

	// Recovery
	Recovery    *DiagnosticRecovery  `json:"recovery,omitempty"`

	// Logs
	Logs        *DiagnosticLogs      `json:"logs,omitempty"`

	// Preflight results
	PreflightResults []PreflightResult `json:"preflight_results,omitempty"`
}

// DiagnosticError captures classified error information.
type DiagnosticError struct {
	Message    string            `json:"message"`
	Category   ErrorCategory     `json:"category"`
	Severity   ErrorSeverity     `json:"severity"`
	Component  string            `json:"component,omitempty"`
	Details    map[string]string `json:"details,omitempty"`
	Suggestion string            `json:"suggestion,omitempty"`
	Retryable  bool              `json:"retryable"`
}

// DiagnosticSystem captures system information.
type DiagnosticSystem struct {
	OS             string `json:"os"`
	Arch           string `json:"arch"`
	Distro         string `json:"distro,omitempty"`
	Version        string `json:"version,omitempty"`
	Kernel         string `json:"kernel,omitempty"`
	PackageManager string `json:"package_manager,omitempty"`
	InitSystem     string `json:"init_system,omitempty"`
	CPUCount       int    `json:"cpu_count"`
	MemoryMB       uint64 `json:"memory_mb"`
	DiskFreeGB     uint64 `json:"disk_free_gb"`
	Hostname       string `json:"hostname,omitempty"`
	IPv4           string `json:"ipv4,omitempty"`
	IPv6           string `json:"ipv6,omitempty"`
	IsVirtual      bool   `json:"is_virtual,omitempty"`
	IsContainer    bool   `json:"is_container,omitempty"`
	ExistingInstall bool  `json:"existing_install"`
}

// DiagnosticConfig captures configuration snapshot.
type DiagnosticConfig struct {
	Mode           string            `json:"mode"`
	ClusterName    string            `json:"cluster_name,omitempty"`
	NodeRole       string            `json:"node_role"`
	NodeName       string            `json:"node_name,omitempty"`
	Storage        string            `json:"storage"`
	NATSMode       string            `json:"nats_mode"`
	TLSMode        string            `json:"tls_mode"`
	Join           string            `json:"join,omitempty"`
	HasJoinToken   bool              `json:"has_join_token"`
	BlueprintsDir  string            `json:"blueprints_dir,omitempty"`
	ApplyBlueprints []string         `json:"apply_blueprints,omitempty"`
	CustomSettings map[string]string `json:"custom_settings,omitempty"`
}

// DiagnosticArtifacts captures install artifacts.
type DiagnosticArtifacts struct {
	PackageManager string   `json:"package_manager"`
	Packages       []string `json:"packages,omitempty"`
	CreatedFiles   []string `json:"created_files,omitempty"`
	Services       []string `json:"services,omitempty"`
}

// DiagnosticRecovery captures recovery information.
type DiagnosticRecovery struct {
	Actions          []RecoveryAction  `json:"actions,omitempty"`
	AttemptedActions []RecoveryResult  `json:"attempted_actions,omitempty"`
	ScriptPath       string            `json:"script_path,omitempty"`
	Hints            []string          `json:"hints,omitempty"`
}

// DiagnosticLogs captures relevant log excerpts.
type DiagnosticLogs struct {
	ServiceStatus map[string]string `json:"service_status,omitempty"`
	ServiceLogs   map[string]string `json:"service_logs,omitempty"`
	SystemLogs    string            `json:"system_logs,omitempty"`
	BootstrapLog  string            `json:"bootstrap_log,omitempty"`
}

// EnhancedDiagnosticsCollector creates comprehensive diagnostic reports.
type EnhancedDiagnosticsCollector struct {
	state           *State
	recoveryManager *RecoveryManager
	verbose         bool
}

// NewEnhancedDiagnosticsCollector creates a new collector.
func NewEnhancedDiagnosticsCollector(state *State, recoveryMgr *RecoveryManager, verbose bool) *EnhancedDiagnosticsCollector {
	return &EnhancedDiagnosticsCollector{
		state:           state,
		recoveryManager: recoveryMgr,
		verbose:         verbose,
	}
}

// Collect creates a comprehensive diagnostic report.
func (c *EnhancedDiagnosticsCollector) Collect(ctx context.Context, phase PhaseName, err error, preflightResults []PreflightResult) *DiagnosticReport {
	report := &DiagnosticReport{
		Timestamp:        time.Now().UTC(),
		Version:          "2.0",
		Phase:            phase,
		PreflightResults: preflightResults,
	}

	// Classify and capture error
	if err != nil {
		bErr := ClassifyError(err, phase)
		report.Error = &DiagnosticError{
			Message:    bErr.Message,
			Category:   bErr.Category,
			Severity:   bErr.Severity,
			Component:  bErr.Component,
			Details:    bErr.Details,
			Suggestion: bErr.Suggestion,
			Retryable:  bErr.IsRetryable(),
		}
		report.RootCause = c.analyzeRootCause(bErr)
		report.ErrorChain = c.buildErrorChain(err)

		// Recovery information
		report.Recovery = &DiagnosticRecovery{
			Actions: bErr.RecoveryActions,
			Hints:   c.buildEnhancedHints(bErr),
		}

		// Generate recovery script
		if c.recoveryManager != nil {
			if scriptPath, scriptErr := c.recoveryManager.GenerateRecoveryScript(bErr); scriptErr == nil {
				report.Recovery.ScriptPath = scriptPath
			}
		}
	}

	// System information
	report.System = c.collectSystemInfo()
	report.Environment = c.collectEnvironment()

	// Configuration
	report.Config = c.collectConfig()

	// Artifacts
	report.Artifacts = c.collectArtifacts()

	// Logs
	report.Logs = c.collectLogs(ctx)

	// Get bootstrap ID from checkpoint if available
	if c.state != nil && c.state.BootstrapConfig != nil {
		// Would normally get from checkpoint
	}

	return report
}

// analyzeRootCause attempts to determine the root cause.
func (c *EnhancedDiagnosticsCollector) analyzeRootCause(bErr *BootstrapError) string {
	switch bErr.Category {
	case ErrorCategoryPermission:
		return "Insufficient system permissions - bootstrap requires root/sudo access"
	case ErrorCategoryNetwork:
		if strings.Contains(bErr.Message, "refused") {
			return "Target service is not running or not accepting connections"
		}
		if strings.Contains(bErr.Message, "unreachable") {
			return "Network path to target is blocked (firewall, routing, or network issue)"
		}
		return "Network connectivity issue prevents communication with required services"
	case ErrorCategoryDatabase:
		if strings.Contains(bErr.Message, "authentication") {
			return "Database authentication failed - incorrect credentials"
		}
		if strings.Contains(bErr.Message, "does not exist") {
			return "Database or schema does not exist and needs to be created"
		}
		return "Database connectivity or configuration issue"
	case ErrorCategoryTLS:
		if strings.Contains(bErr.Message, "expired") {
			return "TLS certificate has expired and needs renewal"
		}
		if strings.Contains(bErr.Message, "self signed") {
			return "Self-signed certificate not trusted - add CA to trust store"
		}
		return "TLS certificate or configuration issue"
	case ErrorCategoryPackage:
		if strings.Contains(bErr.Message, "not found") {
			return "Package repository not configured or package name incorrect"
		}
		if strings.Contains(bErr.Message, "lock") {
			return "Package manager is locked by another process"
		}
		return "Package installation issue"
	case ErrorCategoryService:
		if strings.Contains(bErr.Message, "failed to start") {
			return "Service failed to start - check configuration and dependencies"
		}
		return "Service management issue"
	case ErrorCategoryResource:
		if strings.Contains(bErr.Message, "disk") || strings.Contains(bErr.Message, "space") {
			return "Insufficient disk space for bootstrap operations"
		}
		if strings.Contains(bErr.Message, "memory") {
			return "Insufficient memory available"
		}
		return "System resource constraint"
	case ErrorCategoryConfig:
		return "Bootstrap configuration is incomplete or invalid"
	default:
		return "See error details for more information"
	}
}

// buildErrorChain unwraps nested errors into a chain.
func (c *EnhancedDiagnosticsCollector) buildErrorChain(err error) []string {
	var chain []string
	current := err
	for current != nil {
		chain = append(chain, current.Error())
		unwrapper, ok := current.(interface{ Unwrap() error })
		if !ok {
			break
		}
		current = unwrapper.Unwrap()
		if current != nil && current.Error() == chain[len(chain)-1] {
			break // Avoid duplicates
		}
	}
	return chain
}

// buildEnhancedHints creates context-aware hints.
func (c *EnhancedDiagnosticsCollector) buildEnhancedHints(bErr *BootstrapError) []string {
	hints := []string{}

	// Add suggestion first
	if bErr.Suggestion != "" {
		hints = append(hints, bErr.Suggestion)
	}

	// Add category-specific hints
	switch bErr.Category {
	case ErrorCategoryPermission:
		hints = append(hints, "Run: sudo kscore-agent bootstrap [your-options]")

	case ErrorCategoryNetwork:
		hints = append(hints,
			"Check: ping <target-host>",
			"Check: nc -zv <host> <port>",
			"Check: sudo iptables -L -n | grep <port>",
		)

	case ErrorCategoryDatabase:
		if c.state != nil && c.state.BootstrapConfig != nil && c.state.BootstrapConfig.Storage == "postgres" {
			hints = append(hints,
				fmt.Sprintf("Test: psql -h %s -U %s -d %s -c 'SELECT 1'",
					c.state.BootstrapConfig.PostgresHost,
					c.state.BootstrapConfig.PostgresUser,
					c.state.BootstrapConfig.PostgresDatabase),
			)
		}

	case ErrorCategoryTLS:
		hints = append(hints,
			"For testing, use: --generate-certs",
			"For production, provide: --tls-cert-file, --tls-key-file, --tls-ca-file",
		)

	case ErrorCategoryPackage:
		hints = append(hints,
			"Try: sudo apt-get update (or equivalent for your distro)",
			"Verify repository is configured",
		)

	case ErrorCategoryService:
		hints = append(hints,
			"Check: sudo systemctl status kscore-server",
			"Check: sudo journalctl -u kscore-server -n 50",
		)

	case ErrorCategoryResource:
		hints = append(hints,
			"Check: df -h (disk space)",
			"Check: free -h (memory)",
		)
	}

	// Add recovery script hint if generated
	if c.recoveryManager != nil && c.recoveryManager.config.GenerateRecoveryScript {
		hints = append(hints,
			fmt.Sprintf("Recovery script: %s", c.recoveryManager.config.RecoveryScriptPath),
		)
	}

	return hints
}

// collectSystemInfo gathers system information.
func (c *EnhancedDiagnosticsCollector) collectSystemInfo() *DiagnosticSystem {
	sys := &DiagnosticSystem{
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		CPUCount: runtime.NumCPU(),
	}

	if hostname, err := os.Hostname(); err == nil {
		sys.Hostname = hostname
	}

	if c.state != nil && c.state.System != nil {
		info := c.state.System
		if info.Platform != nil {
			sys.Distro = string(info.Platform.Distro)
			sys.Version = info.Platform.Version
			sys.Kernel = info.Platform.KernelVersion
			sys.PackageManager = info.Platform.PackageManager.String()
			sys.InitSystem = string(info.Platform.InitSystem)
			sys.IsVirtual = info.Platform.IsVirtual
			sys.IsContainer = info.Platform.IsContainer
		}
		sys.CPUCount = info.Resources.CPUCount
		sys.MemoryMB = info.Resources.MemoryTotalMB
		sys.DiskFreeGB = info.Resources.DiskFreeGB
		sys.ExistingInstall = info.ExistingInstall

		if info.Network != nil {
			sys.IPv4 = info.Network.PrimaryIPv4
			sys.IPv6 = info.Network.PrimaryIPv6
		}
	}

	return sys
}

// collectEnvironment gathers relevant environment variables.
func (c *EnhancedDiagnosticsCollector) collectEnvironment() map[string]string {
	env := make(map[string]string)

	// Collect relevant KSCORE_ variables (mask sensitive values)
	for _, e := range os.Environ() {
		pair := strings.SplitN(e, "=", 2)
		if len(pair) != 2 {
			continue
		}
		key, value := pair[0], pair[1]

		if strings.HasPrefix(key, "KSCORE_") {
			// Mask sensitive values
			if strings.Contains(strings.ToLower(key), "password") ||
				strings.Contains(strings.ToLower(key), "token") ||
				strings.Contains(strings.ToLower(key), "secret") {
				env[key] = "***MASKED***"
			} else {
				env[key] = value
			}
		}
	}

	// Add some system environment info
	for _, key := range []string{"PATH", "HOME", "USER", "SHELL", "LANG"} {
		if value := os.Getenv(key); value != "" {
			env[key] = value
		}
	}

	return env
}

// collectConfig captures configuration snapshot.
func (c *EnhancedDiagnosticsCollector) collectConfig() *DiagnosticConfig {
	if c.state == nil || c.state.BootstrapConfig == nil {
		return nil
	}

	cfg := c.state.BootstrapConfig
	diag := &DiagnosticConfig{
		Mode:            cfg.Mode,
		ClusterName:     cfg.ClusterName,
		NodeRole:        cfg.NodeRole,
		NodeName:        cfg.NodeName,
		Storage:         cfg.Storage,
		NATSMode:        cfg.NATSMode,
		Join:            cfg.Join,
		HasJoinToken:    cfg.JoinToken != "",
		BlueprintsDir:   cfg.BlueprintsDir,
		ApplyBlueprints: cfg.ApplyBlueprints,
	}

	// TLS mode
	if cfg.GenerateCerts {
		diag.TLSMode = "generate"
	} else if cfg.TLSCSRFile != "" {
		diag.TLSMode = "csr"
	} else if cfg.TLSCertFile != "" {
		diag.TLSMode = "provided"
	}

	return diag
}

// collectArtifacts captures install artifacts.
func (c *EnhancedDiagnosticsCollector) collectArtifacts() *DiagnosticArtifacts {
	if c.state == nil || c.state.InstallArtifacts == nil {
		return nil
	}

	artifacts := c.state.InstallArtifacts
	return &DiagnosticArtifacts{
		PackageManager: artifacts.PackageManager.String(),
		Packages:       artifacts.Packages,
		CreatedFiles:   artifacts.CreatedFiles,
	}
}

// collectLogs gathers relevant log information.
func (c *EnhancedDiagnosticsCollector) collectLogs(ctx context.Context) *DiagnosticLogs {
	logs := &DiagnosticLogs{
		ServiceStatus: make(map[string]string),
		ServiceLogs:   make(map[string]string),
	}

	// Determine which services to check
	services := []string{"kscore-server", "kscore-agent"}
	if c.state != nil && c.state.BootstrapConfig != nil {
		switch strings.ToLower(c.state.BootstrapConfig.NodeRole) {
		case "control-plane":
			services = []string{"kscore-server"}
		case "agent":
			services = []string{"kscore-agent"}
		}
	}

	// Collect service status and logs
	initSystem := "systemd" // Default
	if c.state != nil && c.state.System != nil && c.state.System.Platform != nil {
		initSystem = string(c.state.System.Platform.InitSystem)
	}

	for _, service := range services {
		logs.ServiceStatus[service] = collectServiceStatus(ctx, initSystem, service)
		logs.ServiceLogs[service] = collectServiceLogs(ctx, initSystem, service, 100)
	}

	// Collect recent system logs
	if initSystem == "systemd" {
		output, _ := runCommandWithTimeout(ctx, 5*time.Second, "journalctl", "-p", "err", "-n", "50", "--no-pager")
		logs.SystemLogs = output
	}

	return logs
}

// WriteReport writes the diagnostic report to disk.
func (c *EnhancedDiagnosticsCollector) WriteReport(report *DiagnosticReport) (string, string, error) {
	timestamp := report.Timestamp.Format("20060102T150405Z")

	// Ensure directory exists
	dir := "/var/log/kscore"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		dir = os.TempDir()
	}

	// Write JSON report
	jsonPath := filepath.Join(dir, fmt.Sprintf("%s-%s.json", diagnosticsFilePrefix, timestamp))
	jsonData, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("marshal JSON report: %w", err)
	}
	if err := os.WriteFile(jsonPath, jsonData, 0o600); err != nil {
		return "", "", fmt.Errorf("write JSON report: %w", err)
	}

	// Write human-readable report
	textPath := filepath.Join(dir, fmt.Sprintf("%s-%s.log", diagnosticsFilePrefix, timestamp))
	textData := c.formatTextReport(report)
	if err := os.WriteFile(textPath, []byte(textData), 0o600); err != nil {
		return jsonPath, "", fmt.Errorf("write text report: %w", err)
	}

	return jsonPath, textPath, nil
}

// formatTextReport creates a human-readable report.
func (c *EnhancedDiagnosticsCollector) formatTextReport(report *DiagnosticReport) string {
	var b strings.Builder

	b.WriteString("═══════════════════════════════════════════════════════════════\n")
	b.WriteString("           KEYSTONE BOOTSTRAP DIAGNOSTIC REPORT\n")
	b.WriteString("═══════════════════════════════════════════════════════════════\n\n")

	b.WriteString(fmt.Sprintf("Timestamp: %s\n", report.Timestamp.Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("Phase:     %s\n", report.Phase))
	b.WriteString("\n")

	// Error section
	if report.Error != nil {
		b.WriteString("┌─ ERROR ────────────────────────────────────────────────────────\n")
		b.WriteString(fmt.Sprintf("│ Category:   %s\n", report.Error.Category))
		b.WriteString(fmt.Sprintf("│ Severity:   %s\n", report.Error.Severity))
		b.WriteString(fmt.Sprintf("│ Message:    %s\n", report.Error.Message))
		if report.Error.Suggestion != "" {
			b.WriteString(fmt.Sprintf("│ Suggestion: %s\n", report.Error.Suggestion))
		}
		if report.RootCause != "" {
			b.WriteString(fmt.Sprintf("│ Root Cause: %s\n", report.RootCause))
		}
		b.WriteString("└────────────────────────────────────────────────────────────────\n\n")

		// Error chain
		if len(report.ErrorChain) > 1 {
			b.WriteString("Error Chain:\n")
			for i, e := range report.ErrorChain {
				b.WriteString(fmt.Sprintf("  %d. %s\n", i+1, e))
			}
			b.WriteString("\n")
		}
	}

	// System section
	if report.System != nil {
		b.WriteString("┌─ SYSTEM ───────────────────────────────────────────────────────\n")
		b.WriteString(fmt.Sprintf("│ OS:         %s/%s\n", report.System.OS, report.System.Arch))
		if report.System.Distro != "" {
			b.WriteString(fmt.Sprintf("│ Distro:     %s %s\n", report.System.Distro, report.System.Version))
		}
		b.WriteString(fmt.Sprintf("│ Hostname:   %s\n", report.System.Hostname))
		b.WriteString(fmt.Sprintf("│ Resources:  CPU=%d, Memory=%dMB, Disk Free=%dGB\n",
			report.System.CPUCount, report.System.MemoryMB, report.System.DiskFreeGB))
		if report.System.PackageManager != "" {
			b.WriteString(fmt.Sprintf("│ Pkg Mgr:    %s\n", report.System.PackageManager))
		}
		if report.System.InitSystem != "" {
			b.WriteString(fmt.Sprintf("│ Init:       %s\n", report.System.InitSystem))
		}
		b.WriteString("└────────────────────────────────────────────────────────────────\n\n")
	}

	// Config section
	if report.Config != nil {
		b.WriteString("┌─ CONFIGURATION ────────────────────────────────────────────────\n")
		b.WriteString(fmt.Sprintf("│ Mode:       %s\n", report.Config.Mode))
		b.WriteString(fmt.Sprintf("│ Role:       %s\n", report.Config.NodeRole))
		b.WriteString(fmt.Sprintf("│ Storage:    %s\n", report.Config.Storage))
		b.WriteString(fmt.Sprintf("│ NATS:       %s\n", report.Config.NATSMode))
		b.WriteString(fmt.Sprintf("│ TLS:        %s\n", report.Config.TLSMode))
		b.WriteString("└────────────────────────────────────────────────────────────────\n\n")
	}

	// Recovery section
	if report.Recovery != nil && len(report.Recovery.Actions) > 0 {
		b.WriteString("┌─ RECOVERY OPTIONS ─────────────────────────────────────────────\n")
		for i, action := range report.Recovery.Actions {
			b.WriteString(fmt.Sprintf("│ %d. %s [%s/%s]\n", i+1, action.Description, action.Type, action.Risk))
			if action.Command != "" {
				b.WriteString(fmt.Sprintf("│    $ %s\n", action.Command))
			}
		}
		if report.Recovery.ScriptPath != "" {
			b.WriteString(fmt.Sprintf("│\n│ Recovery Script: %s\n", report.Recovery.ScriptPath))
		}
		b.WriteString("└────────────────────────────────────────────────────────────────\n\n")
	}

	// Hints section
	if report.Recovery != nil && len(report.Recovery.Hints) > 0 {
		b.WriteString("┌─ HINTS ────────────────────────────────────────────────────────\n")
		for _, hint := range report.Recovery.Hints {
			b.WriteString(fmt.Sprintf("│ • %s\n", hint))
		}
		b.WriteString("└────────────────────────────────────────────────────────────────\n\n")
	}

	// Logs section (abbreviated)
	if report.Logs != nil {
		b.WriteString("┌─ SERVICE STATUS ───────────────────────────────────────────────\n")
		for service, status := range report.Logs.ServiceStatus {
			lines := strings.Split(strings.TrimSpace(status), "\n")
			if len(lines) > 5 {
				lines = lines[:5]
			}
			b.WriteString(fmt.Sprintf("│ [%s]\n", service))
			for _, line := range lines {
				b.WriteString(fmt.Sprintf("│   %s\n", line))
			}
		}
		b.WriteString("└────────────────────────────────────────────────────────────────\n\n")
	}

	b.WriteString("═══════════════════════════════════════════════════════════════\n")
	b.WriteString("For more details, see the JSON report or service logs.\n")

	return b.String()
}
