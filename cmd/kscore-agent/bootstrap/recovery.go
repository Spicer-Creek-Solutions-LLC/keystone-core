package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// RecoveryConfig configures recovery behavior.
type RecoveryConfig struct {
	// EnableAutoRecovery allows automatic recovery attempts.
	EnableAutoRecovery bool

	// MaxAutoRecoveryAttempts limits automatic recovery attempts.
	MaxAutoRecoveryAttempts int

	// AutoRecoveryDelay is the delay between recovery attempts.
	AutoRecoveryDelay time.Duration

	// EnablePreflightChecks runs checks before each phase.
	EnablePreflightChecks bool

	// GenerateRecoveryScript creates a recovery script on failure.
	GenerateRecoveryScript bool

	// RecoveryScriptPath is the path for generated recovery scripts.
	RecoveryScriptPath string
}

// DefaultRecoveryConfig returns the default recovery configuration.
func DefaultRecoveryConfig() RecoveryConfig {
	return RecoveryConfig{
		EnableAutoRecovery:      true,
		MaxAutoRecoveryAttempts: 2,
		AutoRecoveryDelay:       5 * time.Second,
		EnablePreflightChecks:   true,
		GenerateRecoveryScript:  true,
		RecoveryScriptPath:      "/var/lib/keystone-core/bootstrap/recovery.sh",
	}
}

// RecoveryManager coordinates error recovery operations.
type RecoveryManager struct {
	config  RecoveryConfig
	output  io.Writer
	verbose bool
}

// NewRecoveryManager creates a new recovery manager.
func NewRecoveryManager(config RecoveryConfig, output io.Writer, verbose bool) *RecoveryManager {
	return &RecoveryManager{
		config:  config,
		output:  output,
		verbose: verbose,
	}
}

// RecoveryResult captures the outcome of a recovery attempt.
type RecoveryResult struct {
	// Success indicates if recovery was successful.
	Success bool `json:"success"`

	// ActionID is the recovery action that was attempted.
	ActionID string `json:"action_id"`

	// Message describes the outcome.
	Message string `json:"message"`

	// Output is the command output (if applicable).
	Output string `json:"output,omitempty"`

	// Duration is how long the recovery took.
	Duration time.Duration `json:"duration"`
}

// AttemptAutomaticRecovery tries automatic recovery actions for an error.
func (m *RecoveryManager) AttemptAutomaticRecovery(ctx context.Context, bErr *Error) []RecoveryResult {
	if !m.config.EnableAutoRecovery {
		return nil
	}

	actions := bErr.GetAutomaticRecoveryActions()
	if len(actions) == 0 {
		return nil
	}

	var results []RecoveryResult
	for i := range actions {
		action := &actions[i]
		if m.verbose {
			fmt.Fprintf(m.output, "attempting automatic recovery: %s\n", action.Description)
		}

		result := m.executeRecoveryAction(ctx, *action)
		results = append(results, result)

		if result.Success {
			if m.verbose {
				fmt.Fprintf(m.output, "recovery action '%s' succeeded\n", action.ID)
			}
			break // Stop on first success
		} else if m.verbose {
			fmt.Fprintf(m.output, "recovery action '%s' failed: %s\n", action.ID, result.Message)
		}
	}

	return results
}

// executeRecoveryAction runs a single recovery action.
func (m *RecoveryManager) executeRecoveryAction(ctx context.Context, action RecoveryAction) RecoveryResult {
	start := time.Now()

	// Collect commands to execute
	var commands []string
	if action.Command != "" {
		commands = append(commands, action.Command)
	}
	commands = append(commands, action.Commands...)

	if len(commands) == 0 {
		return RecoveryResult{
			Success:  true,
			ActionID: action.ID,
			Message:  "no commands to execute",
			Duration: time.Since(start),
		}
	}

	var outputs []string
	for _, cmd := range commands {
		// Skip comment lines
		if strings.HasPrefix(strings.TrimSpace(cmd), "#") {
			continue
		}

		// Skip placeholder commands
		if strings.Contains(cmd, "<") && strings.Contains(cmd, ">") {
			continue
		}

		output, err := m.runCommand(ctx, cmd)
		outputs = append(outputs, output)

		if err != nil {
			return RecoveryResult{
				Success:  false,
				ActionID: action.ID,
				Message:  fmt.Sprintf("command failed: %v", err),
				Output:   strings.Join(outputs, "\n"),
				Duration: time.Since(start),
			}
		}
	}

	return RecoveryResult{
		Success:  true,
		ActionID: action.ID,
		Message:  "recovery commands completed",
		Output:   strings.Join(outputs, "\n"),
		Duration: time.Since(start),
	}
}

// runCommand executes a shell command and returns output.
func (m *RecoveryManager) runCommand(ctx context.Context, cmdStr string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- recovery commands are admin-supplied and intentionally executed during bootstrap recovery
	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// GenerateRecoveryScript creates a shell script with recovery commands.
func (m *RecoveryManager) GenerateRecoveryScript(bErr *Error) (string, error) {
	if !m.config.GenerateRecoveryScript {
		return "", nil
	}

	var script strings.Builder
	script.WriteString("#!/bin/bash\n")
	script.WriteString("# Keystone Bootstrap Recovery Script\n")
	script.WriteString(fmt.Sprintf("# Generated: %s\n", time.Now().Format(time.RFC3339)))
	script.WriteString(fmt.Sprintf("# Error: %s\n", bErr.Message))
	script.WriteString(fmt.Sprintf("# Category: %s\n", bErr.Category))
	script.WriteString("\nset -e\n\n")

	script.WriteString("echo 'Keystone Bootstrap Recovery Script'\n")
	script.WriteString("echo '==================================='\n")
	script.WriteString(fmt.Sprintf("echo 'Error Category: %s'\n", bErr.Category))
	script.WriteString("echo ''\n\n")

	for i := range bErr.RecoveryActions {
		action := &bErr.RecoveryActions[i]
		script.WriteString(fmt.Sprintf("# Recovery Action %d: %s\n", i+1, action.Description))
		script.WriteString(fmt.Sprintf("# Risk: %s\n", action.Risk))

		if action.Precondition != "" {
			script.WriteString(fmt.Sprintf("# Precondition: %s\n", action.Precondition))
		}

		script.WriteString(fmt.Sprintf("echo 'Step %d: %s'\n", i+1, action.Description))

		if action.Type == RecoveryTypeAutomatic {
			// Auto actions run directly
			if action.Command != "" {
				script.WriteString(fmt.Sprintf("%s\n", action.Command))
			}
			for _, cmd := range action.Commands {
				if !strings.HasPrefix(strings.TrimSpace(cmd), "#") {
					script.WriteString(fmt.Sprintf("%s\n", cmd))
				}
			}
		} else {
			// Interactive/Manual actions prompt for confirmation
			script.WriteString("read -p 'Run this step? (y/n): ' confirm\n")
			script.WriteString("if [ \"$confirm\" = \"y\" ]; then\n")
			if action.Command != "" {
				script.WriteString(fmt.Sprintf("  %s\n", action.Command))
			}
			for _, cmd := range action.Commands {
				if !strings.HasPrefix(strings.TrimSpace(cmd), "#") {
					script.WriteString(fmt.Sprintf("  %s\n", cmd))
				} else {
					script.WriteString(fmt.Sprintf("  echo '%s'\n", cmd))
				}
			}
			script.WriteString("fi\n")
		}

		if action.ExpectedOutcome != "" {
			script.WriteString(fmt.Sprintf("echo 'Expected: %s'\n", action.ExpectedOutcome))
		}
		script.WriteString("echo ''\n\n")
	}

	script.WriteString("echo 'Recovery script completed.'\n")
	script.WriteString("echo 'You may now retry: kscore-agent bootstrap [your-options]'\n")

	// Write script to file
	path := m.config.RecoveryScriptPath
	//nolint:gosec // G301: recovery script directory needs to be accessible by admin users
	if err := os.MkdirAll(strings.TrimSuffix(path, "/recovery.sh"), 0o755); err != nil {
		return "", fmt.Errorf("create recovery script directory: %w", err)
	}

	//nolint:gosec // G306: recovery script must be executable
	if err := os.WriteFile(path, []byte(script.String()), 0o755); err != nil {
		return "", fmt.Errorf("write recovery script: %w", err)
	}

	return path, nil
}

// PreflightCheck represents a pre-phase validation check.
type PreflightCheck struct {
	// Name identifies the check.
	Name string

	// Description explains what is being checked.
	Description string

	// Phase is which phase this check applies to (empty = all phases).
	Phase PhaseName

	// Check is the function that performs the check.
	Check func(ctx context.Context, state *State) error

	// Required indicates if failure should block the phase.
	Required bool

	// AutoFix is an optional function to automatically fix issues.
	AutoFix func(ctx context.Context, state *State) error
}

// PreflightResult captures the outcome of a preflight check.
type PreflightResult struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Passed      bool   `json:"passed"`
	Error       string `json:"error,omitempty"`
	Fixed       bool   `json:"fixed,omitempty"`
	Required    bool   `json:"required"`
}

// PreflightChecker validates preconditions before phases.
type PreflightChecker struct {
	checks  []PreflightCheck
	output  io.Writer
	verbose bool
}

// NewPreflightChecker creates a new preflight checker with default checks.
func NewPreflightChecker(output io.Writer, verbose bool) *PreflightChecker {
	checker := &PreflightChecker{
		output:  output,
		verbose: verbose,
	}
	checker.registerDefaultChecks()
	return checker
}

// registerDefaultChecks adds the default preflight checks.
func (c *PreflightChecker) registerDefaultChecks() {
	c.checks = append(c.checks,
		// Root/sudo check
		PreflightCheck{
			Name:        "root-privileges",
			Description: "Check for root or sudo privileges",
			Required:    true,
			Check: func(ctx context.Context, state *State) error {
				if os.Geteuid() != 0 {
					return fmt.Errorf("bootstrap requires root privileges (run with sudo)")
				}
				return nil
			},
		},
		// Disk space check
		PreflightCheck{
			Name:        "disk-space",
			Description: "Check for adequate disk space",
			Required:    true,
			Check: func(ctx context.Context, state *State) error {
				if state == nil || state.System == nil {
					return nil // Can't check without system info
				}
				if state.System.Resources.DiskFreeGB < 5 {
					return fmt.Errorf("insufficient disk space: %dGB free, need at least 5GB",
						state.System.Resources.DiskFreeGB)
				}
				return nil
			},
		},
		// Memory check
		PreflightCheck{
			Name:        "memory",
			Description: "Check for adequate memory",
			Required:    false, // Warning only
			Check: func(ctx context.Context, state *State) error {
				if state == nil || state.System == nil {
					return nil
				}
				if state.System.Resources.MemoryTotalMB < 1024 {
					return fmt.Errorf("low memory: %dMB total, recommend at least 1024MB",
						state.System.Resources.MemoryTotalMB)
				}
				return nil
			},
		},
		// Package manager check
		PreflightCheck{
			Name:        "package-manager",
			Description: "Check package manager availability",
			Phase:       PhaseInstall,
			Required:    true,
			Check: func(ctx context.Context, state *State) error {
				if state == nil || state.System == nil || state.System.Platform == nil {
					return fmt.Errorf("system detection incomplete")
				}
				pm := state.System.Platform.PackageManager
				if pm == "" || pm == "unknown" {
					return fmt.Errorf("no supported package manager detected")
				}
				return nil
			},
		},
		// Network connectivity check
		PreflightCheck{
			Name:        "network-connectivity",
			Description: "Check basic network connectivity",
			Phase:       PhaseInstall,
			Required:    false, // Warning only - might work offline
			Check: func(ctx context.Context, state *State) error {
				// Try to resolve a well-known domain
				ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()

				cmd := exec.CommandContext(ctx, "getent", "hosts", "packages.keystone.io")
				if err := cmd.Run(); err != nil {
					return fmt.Errorf("cannot resolve packages.keystone.io - check network connectivity")
				}
				return nil
			},
		},
		// Systemd check
		PreflightCheck{
			Name:        "init-system",
			Description: "Check init system compatibility",
			Phase:       PhaseInstall,
			Required:    true,
			Check: func(ctx context.Context, state *State) error {
				if state == nil || state.System == nil || state.System.Platform == nil {
					return nil
				}
				initSys := state.System.Platform.InitSystem
				if initSys != "systemd" && initSys != "openrc" {
					return fmt.Errorf("unsupported init system: %s (need systemd or openrc)", initSys)
				}
				return nil
			},
		},
		// Existing install check
		PreflightCheck{
			Name:        "existing-install",
			Description: "Check for existing installation",
			Required:    false, // Warning only
			Check: func(ctx context.Context, state *State) error {
				if state == nil || state.System == nil {
					return nil
				}
				if state.System.ExistingInstall {
					return fmt.Errorf("existing Keystone installation detected - backup before proceeding")
				}
				return nil
			},
		},
		// Directory writability check
		PreflightCheck{
			Name:        "directory-access",
			Description: "Check directory write access",
			Required:    true,
			Check: func(ctx context.Context, state *State) error {
				dirs := []string{"/etc", "/var/lib", "/var/log"}
				for _, dir := range dirs {
					testFile := dir + "/.kscore-preflight-test"
					if err := os.WriteFile(testFile, []byte("test"), 0o600); err != nil {
						return fmt.Errorf("cannot write to %s: %w", dir, err)
					}
					os.Remove(testFile)
				}
				return nil
			},
		})
}

// AddCheck adds a custom preflight check.
func (c *PreflightChecker) AddCheck(check PreflightCheck) {
	c.checks = append(c.checks, check)
}

// RunChecks executes preflight checks for a phase.
func (c *PreflightChecker) RunChecks(ctx context.Context, state *State, phase PhaseName) ([]PreflightResult, error) {
	var results []PreflightResult
	var failures []string

	for _, check := range c.checks {
		// Skip checks not applicable to this phase
		if check.Phase != "" && check.Phase != phase {
			continue
		}

		result := PreflightResult{
			Name:        check.Name,
			Description: check.Description,
			Required:    check.Required,
		}

		err := check.Check(ctx, state)
		if err != nil {
			result.Error = err.Error()

			// Try auto-fix if available
			if check.AutoFix != nil {
				if fixErr := check.AutoFix(ctx, state); fixErr == nil {
					result.Fixed = true
					result.Passed = true
					if c.verbose {
						fmt.Fprintf(c.output, "preflight check '%s': auto-fixed\n", check.Name)
					}
				}
			}

			if !result.Fixed {
				result.Passed = false
				if check.Required {
					failures = append(failures, fmt.Sprintf("%s: %s", check.Name, err.Error()))
				}
				if c.verbose {
					severity := "warning"
					if check.Required {
						severity = "error"
					}
					fmt.Fprintf(c.output, "preflight check '%s' (%s): %s\n", check.Name, severity, err.Error())
				}
			}
		} else {
			result.Passed = true
			if c.verbose {
				fmt.Fprintf(c.output, "preflight check '%s': passed\n", check.Name)
			}
		}

		results = append(results, result)
	}

	if len(failures) > 0 {
		return results, fmt.Errorf("preflight checks failed:\n  - %s", strings.Join(failures, "\n  - "))
	}

	return results, nil
}

// FormatPreflightResults formats preflight results for display.
func FormatPreflightResults(results []PreflightResult, jsonOutput bool) string {
	if jsonOutput {
		data, _ := json.Marshal(map[string]interface{}{
			"event":   "preflight",
			"results": results,
		})
		return string(data)
	}

	var builder strings.Builder
	builder.WriteString("Preflight Checks:\n")

	for _, result := range results {
		status := "✓"
		if !result.Passed {
			if result.Required {
				status = "✗"
			} else {
				status = "!"
			}
		}
		if result.Fixed {
			status = "⚡"
		}

		builder.WriteString(fmt.Sprintf("  %s %s", status, result.Description))
		if !result.Passed && result.Error != "" {
			builder.WriteString(fmt.Sprintf(" - %s", result.Error))
		}
		if result.Fixed {
			builder.WriteString(" (auto-fixed)")
		}
		builder.WriteString("\n")
	}

	return builder.String()
}
