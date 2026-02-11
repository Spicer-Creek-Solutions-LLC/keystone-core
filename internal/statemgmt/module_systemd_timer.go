// Package statemgmt provides state management modules.
package statemgmt

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// SystemdTimerModule manages systemd timer units.
type SystemdTimerModule struct {
	*BaseModule
}

// SystemdTimerConfig holds systemd timer configuration.
type SystemdTimerConfig struct {
	// Name is the timer unit name (without .timer suffix)
	Name string
	// Description is the unit description
	Description string
	// OnCalendar is the calendar expression (e.g., "daily", "weekly", "*-*-* 00:00:00")
	OnCalendar string
	// OnBootSec triggers N seconds after boot
	OnBootSec string
	// OnUnitActiveSec triggers N seconds after the unit was last activated
	OnUnitActiveSec string
	// OnUnitInactiveSec triggers N seconds after the unit was last deactivated
	OnUnitInactiveSec string
	// OnStartupSec triggers N seconds after systemd was started
	OnStartupSec string
	// AccuracySec is the accuracy of the timer (default 1min)
	AccuracySec string
	// RandomizedDelaySec adds random delay
	RandomizedDelaySec string
	// Persistent if true, triggers immediately if the timer missed its last trigger
	Persistent bool
	// WakeSystem if true, wakes the system from sleep
	WakeSystem bool
	// RemainAfterElapse if false, the timer unit is unloaded after the timer elapses
	RemainAfterElapse bool

	// Service unit configuration
	// ExecStart is the command to execute
	ExecStart string
	// Type is the service type (simple, oneshot, etc.)
	Type string
	// User to run the service as
	User string
	// Group to run the service as
	Group string
	// WorkingDirectory for the service
	WorkingDirectory string
	// Environment variables
	Environment map[string]string
	// StandardOutput destination
	StandardOutput string
	// StandardError destination
	StandardError string

	// Whether to run as user unit (vs system unit)
	UserUnit bool
}

// NewSystemdTimerModule creates a new systemd timer module.
func NewSystemdTimerModule() *SystemdTimerModule {
	return &SystemdTimerModule{
		BaseModule: NewBaseModule("systemd_timer", []string{"present", "absent"}),
	}
}

// Check determines if the timer exists and matches.
func (m *SystemdTimerModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("systemd_timer module only works on Linux")
	}

	config, err := m.parseConfig(decl)
	if err != nil {
		return nil, err
	}

	// Check if timer exists
	exists, enabled := m.timerExists(ctx, config)

	switch decl.State {
	case "present":
		switch {
		case !exists:
			result.Present = false
			result.CurrentState = "absent"
			result.Matches = false
			result.Diff["state"] = map[string]string{"current": "absent", "desired": "present"}
		case !enabled:
			result.Present = true
			result.CurrentState = "disabled"
			result.Matches = false
			result.Diff["enabled"] = map[string]string{"current": "disabled", "desired": "enabled"}
		case !m.timerMatches(config):
			result.Present = true
			result.CurrentState = "different"
			result.Matches = false
			result.Diff["config"] = map[string]string{"current": "different", "desired": "matching"}
		default:
			result.Present = true
			result.CurrentState = "present"
			result.Matches = true
		}

	case "absent":
		if !exists {
			result.Present = false
			result.CurrentState = "absent"
			result.Matches = true
		} else {
			result.Present = true
			result.CurrentState = "present"
			result.Matches = false
			result.Diff["state"] = map[string]string{"current": "present", "desired": "absent"}
		}

	default:
		return nil, fmt.Errorf("unknown state: %s", decl.State)
	}

	return result, nil
}

// Apply creates or removes the timer.
func (m *SystemdTimerModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	result := &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
	}

	if runtime.GOOS != "linux" {
		result.Success = false
		result.Comment = "systemd_timer module only works on Linux"
		return result, fmt.Errorf("systemd_timer module only works on Linux")
	}

	config, err := m.parseConfig(decl)
	if err != nil {
		result.Success = false
		result.Comment = fmt.Sprintf("Failed to parse config: %v", err)
		return result, err
	}

	switch decl.State {
	case "present":
		// Create/update the timer and service units
		if err := m.createUnits(config); err != nil {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to create units: %v", err)
			return result, err
		}

		// Reload systemd
		if err := m.systemctlReload(ctx, config.UserUnit); err != nil {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to reload systemd: %v", err)
			return result, err
		}

		// Enable and start the timer
		if err := m.enableTimer(ctx, config); err != nil {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to enable timer: %v", err)
			return result, err
		}

		result.Changed = true
		result.Comment = fmt.Sprintf("Timer '%s' created and enabled", config.Name)

	case "absent":
		exists, _ := m.timerExists(ctx, config)
		if !exists {
			result.Comment = fmt.Sprintf("Timer '%s' already absent", config.Name)
			return result, nil
		}

		// Stop and disable the timer
		if err := m.disableTimer(ctx, config); err != nil {
			// Ignore errors for non-existent timers
			if !strings.Contains(err.Error(), "not found") {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to disable timer: %v", err)
				return result, err
			}
		}

		// Remove the unit files
		if err := m.removeUnits(config); err != nil {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to remove units: %v", err)
			return result, err
		}

		// Reload systemd
		if err := m.systemctlReload(ctx, config.UserUnit); err != nil {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to reload systemd: %v", err)
			return result, err
		}

		result.Changed = true
		result.Comment = fmt.Sprintf("Timer '%s' removed", config.Name)

	default:
		result.Success = false
		result.Comment = fmt.Sprintf("Unknown state: %s", decl.State)
		return result, fmt.Errorf("unknown state: %s", decl.State)
	}

	return result, nil
}

// Test performs a dry-run check of the operation.
func (m *SystemdTimerModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

// parseConfig parses parameters into SystemdTimerConfig.
func (m *SystemdTimerModule) parseConfig(decl *StateDeclaration) (*SystemdTimerConfig, error) {
	config := &SystemdTimerConfig{
		Type:              "oneshot",
		AccuracySec:       "1min",
		RemainAfterElapse: true,
	}

	config.Name = decl.ID
	config.Description = getStringParameter(decl, "description", "")
	config.OnCalendar = getStringParameter(decl, "on_calendar", "")
	config.OnBootSec = getStringParameter(decl, "on_boot_sec", "")
	config.OnUnitActiveSec = getStringParameter(decl, "on_unit_active_sec", "")
	config.OnUnitInactiveSec = getStringParameter(decl, "on_unit_inactive_sec", "")
	config.OnStartupSec = getStringParameter(decl, "on_startup_sec", "")
	config.AccuracySec = getStringParameter(decl, "accuracy_sec", "1min")
	config.RandomizedDelaySec = getStringParameter(decl, "randomized_delay_sec", "")
	config.Persistent = getBoolParameter(decl, "persistent", false)
	config.WakeSystem = getBoolParameter(decl, "wake_system", false)
	config.RemainAfterElapse = getBoolParameter(decl, "remain_after_elapse", true)

	// Service configuration
	config.ExecStart = getStringParameter(decl, "exec_start", "")
	if config.ExecStart == "" {
		config.ExecStart = getStringParameter(decl, "command", "")
	}
	if config.ExecStart == "" {
		return nil, fmt.Errorf("exec_start or command is required")
	}

	config.Type = getStringParameter(decl, "type", "oneshot")
	config.User = getStringParameter(decl, "user", "")
	config.Group = getStringParameter(decl, "group", "")
	config.WorkingDirectory = getStringParameter(decl, "working_directory", "")

	if env, ok := decl.Parameters["environment"].(map[string]interface{}); ok {
		config.Environment = make(map[string]string)
		for k, v := range env {
			if vs, ok := v.(string); ok {
				config.Environment[k] = vs
			}
		}
	}

	config.StandardOutput = getStringParameter(decl, "standard_output", "")
	config.StandardError = getStringParameter(decl, "standard_error", "")
	config.UserUnit = getBoolParameter(decl, "user_unit", false)

	// Validate: at least one trigger must be specified
	if config.OnCalendar == "" && config.OnBootSec == "" && config.OnUnitActiveSec == "" &&
		config.OnUnitInactiveSec == "" && config.OnStartupSec == "" {
		return nil, fmt.Errorf("at least one trigger is required (on_calendar, on_boot_sec, etc.)")
	}

	return config, nil
}

// getUnitPath returns the path for unit files.
func (m *SystemdTimerModule) getUnitPath(config *SystemdTimerConfig) string {
	if config.UserUnit {
		home := os.Getenv("HOME")
		return filepath.Join(home, ".config", "systemd", "user")
	}
	return "/etc/systemd/system"
}

// timerExists checks if the timer unit exists.
func (m *SystemdTimerModule) timerExists(ctx context.Context, config *SystemdTimerConfig) (exists, enabled bool) {
	timerPath := filepath.Join(m.getUnitPath(config), config.Name+".timer")
	if _, err := os.Stat(timerPath); os.IsNotExist(err) {
		return false, false
	}

	// Check if enabled
	args := []string{"is-enabled", config.Name + ".timer"}
	if config.UserUnit {
		args = append([]string{"--user"}, args...)
	}
	cmd := exec.CommandContext(ctx, "systemctl", args...)
	output, err := cmd.Output()
	if err != nil {
		return true, false
	}

	return true, strings.TrimSpace(string(output)) == "enabled"
}

// timerMatches checks if the timer configuration matches.
func (m *SystemdTimerModule) timerMatches(config *SystemdTimerConfig) bool {
	// Read existing files and compare
	timerPath := filepath.Join(m.getUnitPath(config), config.Name+".timer")
	existing, err := os.ReadFile(timerPath)
	if err != nil {
		return false
	}

	expected := m.generateTimerUnit(config)
	return string(existing) == expected
}

// generateTimerUnit creates the timer unit file content.
func (m *SystemdTimerModule) generateTimerUnit(config *SystemdTimerConfig) string {
	var sb strings.Builder

	sb.WriteString("[Unit]\n")
	if config.Description != "" {
		sb.WriteString(fmt.Sprintf("Description=%s\n", config.Description))
	} else {
		sb.WriteString(fmt.Sprintf("Description=Keystone Core timer for %s\n", config.Name))
	}
	sb.WriteString("\n")

	sb.WriteString("[Timer]\n")
	if config.OnCalendar != "" {
		sb.WriteString(fmt.Sprintf("OnCalendar=%s\n", config.OnCalendar))
	}
	if config.OnBootSec != "" {
		sb.WriteString(fmt.Sprintf("OnBootSec=%s\n", config.OnBootSec))
	}
	if config.OnUnitActiveSec != "" {
		sb.WriteString(fmt.Sprintf("OnUnitActiveSec=%s\n", config.OnUnitActiveSec))
	}
	if config.OnUnitInactiveSec != "" {
		sb.WriteString(fmt.Sprintf("OnUnitInactiveSec=%s\n", config.OnUnitInactiveSec))
	}
	if config.OnStartupSec != "" {
		sb.WriteString(fmt.Sprintf("OnStartupSec=%s\n", config.OnStartupSec))
	}
	if config.AccuracySec != "" {
		sb.WriteString(fmt.Sprintf("AccuracySec=%s\n", config.AccuracySec))
	}
	if config.RandomizedDelaySec != "" {
		sb.WriteString(fmt.Sprintf("RandomizedDelaySec=%s\n", config.RandomizedDelaySec))
	}
	if config.Persistent {
		sb.WriteString("Persistent=true\n")
	}
	if config.WakeSystem {
		sb.WriteString("WakeSystem=true\n")
	}
	if !config.RemainAfterElapse {
		sb.WriteString("RemainAfterElapse=false\n")
	}
	sb.WriteString("\n")

	sb.WriteString("[Install]\n")
	sb.WriteString("WantedBy=timers.target\n")

	return sb.String()
}

// generateServiceUnit creates the service unit file content.
func (m *SystemdTimerModule) generateServiceUnit(config *SystemdTimerConfig) string {
	var sb strings.Builder

	sb.WriteString("[Unit]\n")
	if config.Description != "" {
		sb.WriteString(fmt.Sprintf("Description=%s (service)\n", config.Description))
	} else {
		sb.WriteString(fmt.Sprintf("Description=Keystone Core service for %s\n", config.Name))
	}
	sb.WriteString("\n")

	sb.WriteString("[Service]\n")
	sb.WriteString(fmt.Sprintf("Type=%s\n", config.Type))
	sb.WriteString(fmt.Sprintf("ExecStart=%s\n", config.ExecStart))

	if config.User != "" {
		sb.WriteString(fmt.Sprintf("User=%s\n", config.User))
	}
	if config.Group != "" {
		sb.WriteString(fmt.Sprintf("Group=%s\n", config.Group))
	}
	if config.WorkingDirectory != "" {
		sb.WriteString(fmt.Sprintf("WorkingDirectory=%s\n", config.WorkingDirectory))
	}
	for key, value := range config.Environment {
		sb.WriteString(fmt.Sprintf("Environment=%s=%s\n", key, value))
	}
	if config.StandardOutput != "" {
		sb.WriteString(fmt.Sprintf("StandardOutput=%s\n", config.StandardOutput))
	}
	if config.StandardError != "" {
		sb.WriteString(fmt.Sprintf("StandardError=%s\n", config.StandardError))
	}

	return sb.String()
}

// createUnits creates the timer and service unit files.
func (m *SystemdTimerModule) createUnits(config *SystemdTimerConfig) error {
	unitPath := m.getUnitPath(config)

	// Ensure directory exists
	//nolint:gosec // G301: systemd unit directory needs system access
	if err := os.MkdirAll(unitPath, 0o755); err != nil {
		return fmt.Errorf("failed to create unit directory: %w", err)
	}

	// Write timer unit
	timerPath := filepath.Join(unitPath, config.Name+".timer")
	timerContent := m.generateTimerUnit(config)
	//nolint:gosec // G306: systemd unit files need to be readable by systemd
	if err := os.WriteFile(timerPath, []byte(timerContent), 0o644); err != nil {
		return fmt.Errorf("failed to write timer unit: %w", err)
	}

	// Write service unit
	servicePath := filepath.Join(unitPath, config.Name+".service")
	serviceContent := m.generateServiceUnit(config)
	//nolint:gosec // G306: systemd unit files need to be readable by systemd
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0o644); err != nil {
		return fmt.Errorf("failed to write service unit: %w", err)
	}

	return nil
}

// removeUnits removes the timer and service unit files.
func (m *SystemdTimerModule) removeUnits(config *SystemdTimerConfig) error {
	unitPath := m.getUnitPath(config)

	timerPath := filepath.Join(unitPath, config.Name+".timer")
	if err := os.Remove(timerPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove timer unit: %w", err)
	}

	servicePath := filepath.Join(unitPath, config.Name+".service")
	if err := os.Remove(servicePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove service unit: %w", err)
	}

	return nil
}

// systemctlReload reloads systemd configuration.
func (m *SystemdTimerModule) systemctlReload(ctx context.Context, userUnit bool) error {
	args := []string{"daemon-reload"}
	if userUnit {
		args = append([]string{"--user"}, args...)
	}
	cmd := exec.CommandContext(ctx, "systemctl", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl daemon-reload failed: %w: %s", err, string(output))
	}
	return nil
}

// enableTimer enables and starts the timer.
func (m *SystemdTimerModule) enableTimer(ctx context.Context, config *SystemdTimerConfig) error {
	timerName := config.Name + ".timer"

	args := []string{"enable", "--now", timerName}
	if config.UserUnit {
		args = append([]string{"--user"}, args...)
	}

	cmd := exec.CommandContext(ctx, "systemctl", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl enable failed: %w: %s", err, string(output))
	}

	return nil
}

// disableTimer stops and disables the timer.
func (m *SystemdTimerModule) disableTimer(ctx context.Context, config *SystemdTimerConfig) error {
	timerName := config.Name + ".timer"

	args := []string{"disable", "--now", timerName}
	if config.UserUnit {
		args = append([]string{"--user"}, args...)
	}

	cmd := exec.CommandContext(ctx, "systemctl", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl disable failed: %w: %s", err, string(output))
	}

	return nil
}

func init() {
	_ = RegisterModule(NewSystemdTimerModule())
}
