// Package statemgmt provides state management modules.
package statemgmt

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// ScheduledTaskModule manages Windows Task Scheduler tasks.
type ScheduledTaskModule struct {
	*BaseModule
}

// ScheduledTaskConfig holds Windows scheduled task configuration.
type ScheduledTaskConfig struct {
	// Name is the task name
	Name string
	// TaskPath is the folder path for the task (default: \)
	TaskPath string
	// Description of the task
	Description string
	// Enabled if true, the task is enabled
	Enabled bool

	// Action configuration
	// Execute is the program to run
	Execute string
	// Arguments to pass to the program
	Arguments string
	// StartIn is the working directory
	StartIn string

	// Trigger configuration
	// TriggerType: once, daily, weekly, monthly, at_logon, at_startup, on_idle
	TriggerType string
	// StartTime in format HH:MM:SS or HH:MM
	StartTime string
	// StartDate in format MM/DD/YYYY or YYYY-MM-DD
	StartDate string
	// DaysInterval for daily triggers
	DaysInterval int
	// WeeksInterval for weekly triggers
	WeeksInterval int
	// DaysOfWeek for weekly triggers (SUN, MON, TUE, WED, THU, FRI, SAT)
	DaysOfWeek []string
	// MonthsOfYear for monthly triggers (JAN, FEB, ..., DEC)
	MonthsOfYear []string
	// DaysOfMonth for monthly triggers (1-31)
	DaysOfMonth []int
	// RepeatInterval for repeating (e.g., "1 hour", "30 minutes")
	RepeatInterval string
	// RepeatDuration for how long to repeat (e.g., "8 hours", "1 day", "indefinitely")
	RepeatDuration string
	// Delay before starting (e.g., "30 seconds", "5 minutes")
	Delay string

	// Settings
	// RunLevel: limited, highest
	RunLevel string
	// AllowDemandStart if true, allows running on demand
	AllowDemandStart bool
	// StartWhenAvailable if true, starts if a scheduled time was missed
	StartWhenAvailable bool
	// StopIfGoingOnBatteries if true, stops on battery
	StopIfGoingOnBatteries bool
	// DontStopIfGoingOnBatteries if true, doesn't stop on battery
	DontStopIfGoingOnBatteries bool
	// WakeToRun if true, wakes computer to run
	WakeToRun bool
	// ExecutionTimeLimit (e.g., "1 hour", "72 hours")
	ExecutionTimeLimit string
	// DeleteExpiredTaskAfter removes task after expiry
	DeleteExpiredTaskAfter string
	// Hidden if true, task is hidden
	Hidden bool
	// MultipleInstances: parallel, queue, ignore_new, stop_existing
	MultipleInstances string

	// Credentials
	// User to run as
	User string
	// Password for the user (use with caution!)
	Password string
	// RunOnlyIfLoggedOn if true, runs only when user is logged on
	RunOnlyIfLoggedOn bool
}

// NewScheduledTaskModule creates a new scheduled task module.
func NewScheduledTaskModule() *ScheduledTaskModule {
	return &ScheduledTaskModule{
		BaseModule: NewBaseModule("scheduled_task", []string{"present", "absent"}),
	}
}

// Check determines if the task exists and matches.
func (m *ScheduledTaskModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("scheduled_task module only works on Windows")
	}

	config, err := m.parseConfig(decl)
	if err != nil {
		return nil, err
	}

	exists := m.taskExists(ctx, config)

	switch decl.State {
	case "present":
		if !exists {
			result.Present = false
			result.CurrentState = "absent"
			result.Matches = false
			result.Diff["state"] = map[string]string{"current": "absent", "desired": "present"}
		} else {
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

// Apply creates or removes the task.
func (m *ScheduledTaskModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	result := &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
	}

	if runtime.GOOS != "windows" {
		result.Success = false
		result.Comment = "scheduled_task module only works on Windows"
		return result, fmt.Errorf("scheduled_task module only works on Windows")
	}

	config, err := m.parseConfig(decl)
	if err != nil {
		result.Success = false
		result.Comment = fmt.Sprintf("Failed to parse config: %v", err)
		return result, err
	}

	switch decl.State {
	case "present":
		// Delete existing task if it exists (for idempotent updates)
		if m.taskExists(ctx, config) {
			if err := m.deleteTask(ctx, config); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to delete existing task: %v", err)
				return result, err
			}
		}

		// Create the task
		if err := m.createTask(ctx, config); err != nil {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to create task: %v", err)
			return result, err
		}

		result.Changed = true
		result.Comment = fmt.Sprintf("Task '%s' created", config.Name)

	case "absent":
		if !m.taskExists(ctx, config) {
			result.Comment = fmt.Sprintf("Task '%s' already absent", config.Name)
			return result, nil
		}

		if err := m.deleteTask(ctx, config); err != nil {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to delete task: %v", err)
			return result, err
		}

		result.Changed = true
		result.Comment = fmt.Sprintf("Task '%s' removed", config.Name)

	default:
		result.Success = false
		result.Comment = fmt.Sprintf("Unknown state: %s", decl.State)
		return result, fmt.Errorf("unknown state: %s", decl.State)
	}

	return result, nil
}

// Test performs a dry-run check of the operation.
func (m *ScheduledTaskModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

// parseConfig parses parameters into ScheduledTaskConfig.
func (m *ScheduledTaskModule) parseConfig(decl *StateDeclaration) (*ScheduledTaskConfig, error) {
	config := &ScheduledTaskConfig{
		TaskPath:         "\\",
		Enabled:          true,
		DaysInterval:     1,
		WeeksInterval:    1,
		RunLevel:         "limited",
		AllowDemandStart: true,
	}

	config.Name = decl.ID

	config.TaskPath = getStringParameter(decl, "task_path", "\\")
	config.Description = getStringParameter(decl, "description", "")
	config.Enabled = getBoolParameter(decl, "enabled", true)

	// Action
	config.Execute = getStringParameter(decl, "execute", "")
	if config.Execute == "" {
		config.Execute = getStringParameter(decl, "command", "")
	}
	if config.Execute == "" {
		return nil, fmt.Errorf("execute is required")
	}

	config.Arguments = getStringParameter(decl, "arguments", "")
	config.StartIn = getStringParameter(decl, "start_in", "")

	// Trigger
	config.TriggerType = getStringParameter(decl, "trigger_type", "")
	if config.TriggerType == "" {
		return nil, fmt.Errorf("trigger_type is required")
	}

	config.StartTime = getStringParameter(decl, "start_time", "")
	config.StartDate = getStringParameter(decl, "start_date", "")
	config.DaysInterval = getIntParameter(decl, "days_interval", 1)
	config.WeeksInterval = getIntParameter(decl, "weeks_interval", 1)

	if daysOfWeek, ok := decl.Parameters["days_of_week"].([]interface{}); ok {
		for _, d := range daysOfWeek {
			if s, ok := d.(string); ok {
				config.DaysOfWeek = append(config.DaysOfWeek, s)
			}
		}
	}

	if monthsOfYear, ok := decl.Parameters["months_of_year"].([]interface{}); ok {
		for _, m := range monthsOfYear {
			if s, ok := m.(string); ok {
				config.MonthsOfYear = append(config.MonthsOfYear, s)
			}
		}
	}

	if daysOfMonth, ok := decl.Parameters["days_of_month"].([]interface{}); ok {
		for _, d := range daysOfMonth {
			if i, ok := d.(int); ok {
				config.DaysOfMonth = append(config.DaysOfMonth, i)
			}
		}
	}

	config.RepeatInterval = getStringParameter(decl, "repeat_interval", "")
	config.RepeatDuration = getStringParameter(decl, "repeat_duration", "")
	config.Delay = getStringParameter(decl, "delay", "")

	// Settings
	config.RunLevel = getStringParameter(decl, "run_level", "limited")
	config.AllowDemandStart = getBoolParameter(decl, "allow_demand_start", true)
	config.StartWhenAvailable = getBoolParameter(decl, "start_when_available", false)
	config.StopIfGoingOnBatteries = getBoolParameter(decl, "stop_if_going_on_batteries", false)
	config.DontStopIfGoingOnBatteries = getBoolParameter(decl, "dont_stop_if_going_on_batteries", false)
	config.WakeToRun = getBoolParameter(decl, "wake_to_run", false)
	config.ExecutionTimeLimit = getStringParameter(decl, "execution_time_limit", "")
	config.DeleteExpiredTaskAfter = getStringParameter(decl, "delete_expired_task_after", "")
	config.Hidden = getBoolParameter(decl, "hidden", false)
	config.MultipleInstances = getStringParameter(decl, "multiple_instances", "")

	// Credentials
	config.User = getStringParameter(decl, "user", "")
	config.Password = getStringParameter(decl, "password", "")
	config.RunOnlyIfLoggedOn = getBoolParameter(decl, "run_only_if_logged_on", false)

	return config, nil
}

// getTaskName returns the full task name including path.
func (m *ScheduledTaskModule) getTaskName(config *ScheduledTaskConfig) string {
	path := config.TaskPath
	if !strings.HasSuffix(path, "\\") {
		path += "\\"
	}
	return path + config.Name
}

// taskExists checks if the task exists.
func (m *ScheduledTaskModule) taskExists(ctx context.Context, config *ScheduledTaskConfig) bool {
	taskName := m.getTaskName(config)
	cmd := exec.CommandContext(ctx, "schtasks", "/Query", "/TN", taskName)
	err := cmd.Run()
	return err == nil
}

// createTask creates a scheduled task.
func (m *ScheduledTaskModule) createTask(ctx context.Context, config *ScheduledTaskConfig) error {
	args := []string{"/Create", "/F"} // /F forces creation (overwrites)

	// Task name and action
	args = append(args, "/TN", m.getTaskName(config), "/TR", m.buildCommand(config))

	// Trigger
	triggerArgs := m.buildTriggerArgs(config)
	args = append(args, triggerArgs...)

	// Run level
	if config.RunLevel == "highest" {
		args = append(args, "/RL", "HIGHEST")
	}

	// User
	if config.User != "" {
		args = append(args, "/RU", config.User)
		if config.Password != "" {
			args = append(args, "/RP", config.Password)
		}
	}

	cmd := exec.CommandContext(ctx, "schtasks", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("schtasks create failed: %w: %s", err, string(output))
	}

	// Apply additional settings that schtasks doesn't support directly
	// This would require using PowerShell for more complex configurations

	return nil
}

// buildCommand builds the command string.
func (m *ScheduledTaskModule) buildCommand(config *ScheduledTaskConfig) string {
	if config.Arguments != "" {
		return fmt.Sprintf("%q %s", config.Execute, config.Arguments)
	}
	return config.Execute
}

// buildTriggerArgs builds the trigger arguments for schtasks.
func (m *ScheduledTaskModule) buildTriggerArgs(config *ScheduledTaskConfig) []string {
	var args []string

	switch strings.ToLower(config.TriggerType) {
	case "once":
		args = append(args, "/SC", "ONCE")
		if config.StartTime != "" {
			args = append(args, "/ST", config.StartTime)
		}
		if config.StartDate != "" {
			args = append(args, "/SD", config.StartDate)
		}

	case "daily":
		args = append(args, "/SC", "DAILY")
		if config.DaysInterval > 1 {
			args = append(args, "/MO", fmt.Sprintf("%d", config.DaysInterval))
		}
		if config.StartTime != "" {
			args = append(args, "/ST", config.StartTime)
		}

	case "weekly":
		args = append(args, "/SC", "WEEKLY")
		if config.WeeksInterval > 1 {
			args = append(args, "/MO", fmt.Sprintf("%d", config.WeeksInterval))
		}
		if len(config.DaysOfWeek) > 0 {
			args = append(args, "/D", strings.Join(config.DaysOfWeek, ","))
		}
		if config.StartTime != "" {
			args = append(args, "/ST", config.StartTime)
		}

	case "monthly":
		args = append(args, "/SC", "MONTHLY")
		if len(config.MonthsOfYear) > 0 {
			args = append(args, "/M", strings.Join(config.MonthsOfYear, ","))
		}
		if len(config.DaysOfMonth) > 0 {
			days := make([]string, len(config.DaysOfMonth))
			for i, d := range config.DaysOfMonth {
				days[i] = fmt.Sprintf("%d", d)
			}
			args = append(args, "/D", strings.Join(days, ","))
		}
		if config.StartTime != "" {
			args = append(args, "/ST", config.StartTime)
		}

	case "at_logon", "onlogon":
		args = append(args, "/SC", "ONLOGON")
		if config.Delay != "" {
			args = append(args, "/DELAY", config.Delay)
		}

	case "at_startup", "onstart":
		args = append(args, "/SC", "ONSTART")
		if config.Delay != "" {
			args = append(args, "/DELAY", config.Delay)
		}

	case "on_idle", "onidle":
		args = append(args, "/SC", "ONIDLE", "/I", "10") // Default idle time of 10 minutes

	default:
		// Default to once
		args = append(args, "/SC", "ONCE")
		if config.StartTime != "" {
			args = append(args, "/ST", config.StartTime)
		}
	}

	return args
}

// deleteTask deletes a scheduled task.
func (m *ScheduledTaskModule) deleteTask(ctx context.Context, config *ScheduledTaskConfig) error {
	taskName := m.getTaskName(config)
	cmd := exec.CommandContext(ctx, "schtasks", "/Delete", "/TN", taskName, "/F")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("schtasks delete failed: %w: %s", err, string(output))
	}
	return nil
}
