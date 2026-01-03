// Package statemgmt provides state management modules.
package statemgmt

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// LaunchdModule manages macOS launchd jobs.
type LaunchdModule struct {
	*BaseModule
}

// LaunchdConfig holds launchd job configuration.
type LaunchdConfig struct {
	// Label is the unique identifier for the job (e.g., "com.example.myjob")
	Label string
	// Program is the path to the executable
	Program string
	// ProgramArguments is the program and its arguments as a list
	ProgramArguments []string
	// RunAtLoad if true, starts the job when loaded
	RunAtLoad bool
	// KeepAlive if true, restarts the job if it exits
	KeepAlive bool
	// StartInterval runs the job every N seconds
	StartInterval int
	// StartCalendarInterval is a calendar-based schedule
	StartCalendarInterval *CalendarInterval
	// StartOnMount if true, starts when a filesystem is mounted
	StartOnMount bool
	// WorkingDirectory for the job
	WorkingDirectory string
	// StandardOutPath for stdout
	StandardOutPath string
	// StandardErrorPath for stderr
	StandardErrorPath string
	// EnvironmentVariables for the job
	EnvironmentVariables map[string]string
	// UserName to run as
	UserName string
	// GroupName to run as
	GroupName string
	// RootDirectory for the job (chroot)
	RootDirectory string
	// Umask for file creation
	Umask int
	// Nice value for process priority
	Nice int
	// LowPriorityIO if true, uses low priority I/O
	LowPriorityIO bool
	// ProcessType: Background, Standard, Adaptive, Interactive
	ProcessType string
	// Disabled if true, the job is loaded but not run
	Disabled bool
	// UserAgent if true, installs as user LaunchAgent (vs system LaunchDaemon)
	UserAgent bool
}

// CalendarInterval represents a launchd calendar-based schedule.
type CalendarInterval struct {
	Month   int
	Day     int
	Weekday int
	Hour    int
	Minute  int
}

// NewLaunchdModule creates a new launchd module.
func NewLaunchdModule() *LaunchdModule {
	return &LaunchdModule{
		BaseModule: NewBaseModule("launchd", []string{"present", "absent"}),
	}
}

// Check determines if the launchd job exists and matches.
func (m *LaunchdModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	if runtime.GOOS != "darwin" {
		return nil, fmt.Errorf("launchd module only works on macOS")
	}

	config, err := m.parseConfig(decl)
	if err != nil {
		return nil, err
	}

	exists, loaded := m.jobExists(config)

	switch decl.State {
	case "present":
		if !exists {
			result.Present = false
			result.CurrentState = "absent"
			result.Matches = false
			result.Diff["state"] = map[string]string{"current": "absent", "desired": "present"}
		} else if !loaded && !config.Disabled {
			result.Present = true
			result.CurrentState = "unloaded"
			result.Matches = false
			result.Diff["loaded"] = map[string]string{"current": "unloaded", "desired": "loaded"}
		} else if !m.jobMatches(config) {
			result.Present = true
			result.CurrentState = "different"
			result.Matches = false
			result.Diff["config"] = map[string]string{"current": "different", "desired": "matching"}
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

// Apply creates or removes the launchd job.
func (m *LaunchdModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	result := &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
	}

	if runtime.GOOS != "darwin" {
		result.Success = false
		result.Comment = "launchd module only works on macOS"
		return result, fmt.Errorf("launchd module only works on macOS")
	}

	config, err := m.parseConfig(decl)
	if err != nil {
		result.Success = false
		result.Comment = fmt.Sprintf("Failed to parse config: %v", err)
		return result, err
	}

	switch decl.State {
	case "present":
		// Unload existing job if it exists
		exists, loaded := m.jobExists(config)
		if exists && loaded {
			if err := m.unloadJob(config); err != nil {
				// Ignore errors, job might not be loaded
			}
		}

		// Create the plist file
		if err := m.createPlist(config); err != nil {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to create plist: %v", err)
			return result, err
		}

		// Load the job (unless disabled)
		if !config.Disabled {
			if err := m.loadJob(config); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to load job: %v", err)
				return result, err
			}
		}

		result.Changed = true
		result.Comment = fmt.Sprintf("Job '%s' created and loaded", config.Label)

	case "absent":
		exists, loaded := m.jobExists(config)
		if !exists {
			result.Comment = fmt.Sprintf("Job '%s' already absent", config.Label)
			return result, nil
		}

		// Unload if loaded
		if loaded {
			if err := m.unloadJob(config); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to unload job: %v", err)
				return result, err
			}
		}

		// Remove the plist file
		if err := m.removePlist(config); err != nil {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to remove plist: %v", err)
			return result, err
		}

		result.Changed = true
		result.Comment = fmt.Sprintf("Job '%s' removed", config.Label)

	default:
		result.Success = false
		result.Comment = fmt.Sprintf("Unknown state: %s", decl.State)
		return result, fmt.Errorf("unknown state: %s", decl.State)
	}

	return result, nil
}

// Test performs a dry-run check of the operation.
func (m *LaunchdModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

// parseConfig parses parameters into LaunchdConfig.
func (m *LaunchdModule) parseConfig(decl *StateDeclaration) (*LaunchdConfig, error) {
	config := &LaunchdConfig{}

	// Use label if specified, otherwise use the state ID
	config.Label = getStringParameter(decl, "label", "")
	if config.Label == "" {
		config.Label = decl.ID
	}

	config.Program = getStringParameter(decl, "program", "")

	if args, ok := decl.Parameters["program_arguments"].([]interface{}); ok {
		for _, arg := range args {
			if s, ok := arg.(string); ok {
				config.ProgramArguments = append(config.ProgramArguments, s)
			}
		}
	}

	// Must have either program or program_arguments
	if config.Program == "" && len(config.ProgramArguments) == 0 {
		return nil, fmt.Errorf("either program or program_arguments is required")
	}

	config.RunAtLoad = getBoolParameter(decl, "run_at_load", false)
	config.KeepAlive = getBoolParameter(decl, "keep_alive", false)
	config.StartInterval = getIntParameter(decl, "start_interval", 0)

	if cal, ok := decl.Parameters["start_calendar_interval"].(map[string]interface{}); ok {
		config.StartCalendarInterval = &CalendarInterval{}
		if month, ok := cal["month"].(int); ok {
			config.StartCalendarInterval.Month = month
		}
		if day, ok := cal["day"].(int); ok {
			config.StartCalendarInterval.Day = day
		}
		if weekday, ok := cal["weekday"].(int); ok {
			config.StartCalendarInterval.Weekday = weekday
		}
		if hour, ok := cal["hour"].(int); ok {
			config.StartCalendarInterval.Hour = hour
		}
		if minute, ok := cal["minute"].(int); ok {
			config.StartCalendarInterval.Minute = minute
		}
	}

	config.StartOnMount = getBoolParameter(decl, "start_on_mount", false)
	config.WorkingDirectory = getStringParameter(decl, "working_directory", "")
	config.StandardOutPath = getStringParameter(decl, "standard_out_path", "")
	config.StandardErrorPath = getStringParameter(decl, "standard_error_path", "")

	if env, ok := decl.Parameters["environment_variables"].(map[string]interface{}); ok {
		config.EnvironmentVariables = make(map[string]string)
		for k, v := range env {
			if vs, ok := v.(string); ok {
				config.EnvironmentVariables[k] = vs
			}
		}
	}

	config.UserName = getStringParameter(decl, "user_name", "")
	config.GroupName = getStringParameter(decl, "group_name", "")
	config.RootDirectory = getStringParameter(decl, "root_directory", "")
	config.Umask = getIntParameter(decl, "umask", 0)
	config.Nice = getIntParameter(decl, "nice", 0)
	config.LowPriorityIO = getBoolParameter(decl, "low_priority_io", false)
	config.ProcessType = getStringParameter(decl, "process_type", "")
	config.Disabled = getBoolParameter(decl, "disabled", false)
	config.UserAgent = getBoolParameter(decl, "user_agent", false)

	return config, nil
}

// getPlistPath returns the path to the plist file.
func (m *LaunchdModule) getPlistPath(config *LaunchdConfig) string {
	filename := config.Label + ".plist"

	if config.UserAgent {
		home := os.Getenv("HOME")
		return filepath.Join(home, "Library/LaunchAgents", filename)
	}
	return filepath.Join("/Library/LaunchDaemons", filename)
}

// jobExists checks if the job plist exists and is loaded.
func (m *LaunchdModule) jobExists(config *LaunchdConfig) (exists bool, loaded bool) {
	plistPath := m.getPlistPath(config)
	if _, err := os.Stat(plistPath); os.IsNotExist(err) {
		return false, false
	}

	// Check if loaded
	cmd := exec.Command("launchctl", "list", config.Label)
	if err := cmd.Run(); err != nil {
		return true, false
	}

	return true, true
}

// jobMatches checks if the job configuration matches.
func (m *LaunchdModule) jobMatches(config *LaunchdConfig) bool {
	plistPath := m.getPlistPath(config)
	existing, err := os.ReadFile(plistPath)
	if err != nil {
		return false
	}

	expected := m.generatePlist(config)
	return string(existing) == expected
}

// generatePlist creates the plist XML content.
func (m *LaunchdModule) generatePlist(config *LaunchdConfig) string {
	var sb strings.Builder

	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	sb.WriteString("\n")
	sb.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">`)
	sb.WriteString("\n")
	sb.WriteString(`<plist version="1.0">`)
	sb.WriteString("\n")
	sb.WriteString("<dict>\n")

	// Label (required)
	sb.WriteString("\t<key>Label</key>\n")
	sb.WriteString(fmt.Sprintf("\t<string>%s</string>\n", xmlEscape(config.Label)))

	// Program or ProgramArguments
	if config.Program != "" {
		sb.WriteString("\t<key>Program</key>\n")
		sb.WriteString(fmt.Sprintf("\t<string>%s</string>\n", xmlEscape(config.Program)))
	}

	if len(config.ProgramArguments) > 0 {
		sb.WriteString("\t<key>ProgramArguments</key>\n")
		sb.WriteString("\t<array>\n")
		for _, arg := range config.ProgramArguments {
			sb.WriteString(fmt.Sprintf("\t\t<string>%s</string>\n", xmlEscape(arg)))
		}
		sb.WriteString("\t</array>\n")
	}

	// RunAtLoad
	if config.RunAtLoad {
		sb.WriteString("\t<key>RunAtLoad</key>\n")
		sb.WriteString("\t<true/>\n")
	}

	// KeepAlive
	if config.KeepAlive {
		sb.WriteString("\t<key>KeepAlive</key>\n")
		sb.WriteString("\t<true/>\n")
	}

	// StartInterval
	if config.StartInterval > 0 {
		sb.WriteString("\t<key>StartInterval</key>\n")
		sb.WriteString(fmt.Sprintf("\t<integer>%d</integer>\n", config.StartInterval))
	}

	// StartCalendarInterval
	if config.StartCalendarInterval != nil {
		sb.WriteString("\t<key>StartCalendarInterval</key>\n")
		sb.WriteString("\t<dict>\n")
		if config.StartCalendarInterval.Month > 0 {
			sb.WriteString("\t\t<key>Month</key>\n")
			sb.WriteString(fmt.Sprintf("\t\t<integer>%d</integer>\n", config.StartCalendarInterval.Month))
		}
		if config.StartCalendarInterval.Day > 0 {
			sb.WriteString("\t\t<key>Day</key>\n")
			sb.WriteString(fmt.Sprintf("\t\t<integer>%d</integer>\n", config.StartCalendarInterval.Day))
		}
		if config.StartCalendarInterval.Weekday > 0 {
			sb.WriteString("\t\t<key>Weekday</key>\n")
			sb.WriteString(fmt.Sprintf("\t\t<integer>%d</integer>\n", config.StartCalendarInterval.Weekday))
		}
		if config.StartCalendarInterval.Hour >= 0 {
			sb.WriteString("\t\t<key>Hour</key>\n")
			sb.WriteString(fmt.Sprintf("\t\t<integer>%d</integer>\n", config.StartCalendarInterval.Hour))
		}
		if config.StartCalendarInterval.Minute >= 0 {
			sb.WriteString("\t\t<key>Minute</key>\n")
			sb.WriteString(fmt.Sprintf("\t\t<integer>%d</integer>\n", config.StartCalendarInterval.Minute))
		}
		sb.WriteString("\t</dict>\n")
	}

	// StartOnMount
	if config.StartOnMount {
		sb.WriteString("\t<key>StartOnMount</key>\n")
		sb.WriteString("\t<true/>\n")
	}

	// WorkingDirectory
	if config.WorkingDirectory != "" {
		sb.WriteString("\t<key>WorkingDirectory</key>\n")
		sb.WriteString(fmt.Sprintf("\t<string>%s</string>\n", xmlEscape(config.WorkingDirectory)))
	}

	// StandardOutPath
	if config.StandardOutPath != "" {
		sb.WriteString("\t<key>StandardOutPath</key>\n")
		sb.WriteString(fmt.Sprintf("\t<string>%s</string>\n", xmlEscape(config.StandardOutPath)))
	}

	// StandardErrorPath
	if config.StandardErrorPath != "" {
		sb.WriteString("\t<key>StandardErrorPath</key>\n")
		sb.WriteString(fmt.Sprintf("\t<string>%s</string>\n", xmlEscape(config.StandardErrorPath)))
	}

	// EnvironmentVariables
	if len(config.EnvironmentVariables) > 0 {
		sb.WriteString("\t<key>EnvironmentVariables</key>\n")
		sb.WriteString("\t<dict>\n")
		for k, v := range config.EnvironmentVariables {
			sb.WriteString(fmt.Sprintf("\t\t<key>%s</key>\n", xmlEscape(k)))
			sb.WriteString(fmt.Sprintf("\t\t<string>%s</string>\n", xmlEscape(v)))
		}
		sb.WriteString("\t</dict>\n")
	}

	// UserName
	if config.UserName != "" {
		sb.WriteString("\t<key>UserName</key>\n")
		sb.WriteString(fmt.Sprintf("\t<string>%s</string>\n", xmlEscape(config.UserName)))
	}

	// GroupName
	if config.GroupName != "" {
		sb.WriteString("\t<key>GroupName</key>\n")
		sb.WriteString(fmt.Sprintf("\t<string>%s</string>\n", xmlEscape(config.GroupName)))
	}

	// RootDirectory
	if config.RootDirectory != "" {
		sb.WriteString("\t<key>RootDirectory</key>\n")
		sb.WriteString(fmt.Sprintf("\t<string>%s</string>\n", xmlEscape(config.RootDirectory)))
	}

	// Umask
	if config.Umask > 0 {
		sb.WriteString("\t<key>Umask</key>\n")
		sb.WriteString(fmt.Sprintf("\t<integer>%d</integer>\n", config.Umask))
	}

	// Nice
	if config.Nice != 0 {
		sb.WriteString("\t<key>Nice</key>\n")
		sb.WriteString(fmt.Sprintf("\t<integer>%d</integer>\n", config.Nice))
	}

	// LowPriorityIO
	if config.LowPriorityIO {
		sb.WriteString("\t<key>LowPriorityIO</key>\n")
		sb.WriteString("\t<true/>\n")
	}

	// ProcessType
	if config.ProcessType != "" {
		sb.WriteString("\t<key>ProcessType</key>\n")
		sb.WriteString(fmt.Sprintf("\t<string>%s</string>\n", xmlEscape(config.ProcessType)))
	}

	// Disabled
	if config.Disabled {
		sb.WriteString("\t<key>Disabled</key>\n")
		sb.WriteString("\t<true/>\n")
	}

	sb.WriteString("</dict>\n")
	sb.WriteString("</plist>\n")

	return sb.String()
}

// xmlEscape escapes special XML characters.
func xmlEscape(s string) string {
	var buf strings.Builder
	if err := xml.EscapeText(&buf, []byte(s)); err != nil {
		return s
	}
	return buf.String()
}

// createPlist creates the plist file.
func (m *LaunchdModule) createPlist(config *LaunchdConfig) error {
	plistPath := m.getPlistPath(config)

	// Ensure directory exists
	dir := filepath.Dir(plistPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	content := m.generatePlist(config)
	if err := os.WriteFile(plistPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write plist: %v", err)
	}

	return nil
}

// removePlist removes the plist file.
func (m *LaunchdModule) removePlist(config *LaunchdConfig) error {
	plistPath := m.getPlistPath(config)
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove plist: %v", err)
	}
	return nil
}

// loadJob loads the launchd job.
func (m *LaunchdModule) loadJob(config *LaunchdConfig) error {
	plistPath := m.getPlistPath(config)
	cmd := exec.Command("launchctl", "load", "-w", plistPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl load failed: %v: %s", err, string(output))
	}
	return nil
}

// unloadJob unloads the launchd job.
func (m *LaunchdModule) unloadJob(config *LaunchdConfig) error {
	plistPath := m.getPlistPath(config)
	cmd := exec.Command("launchctl", "unload", "-w", plistPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl unload failed: %v: %s", err, string(output))
	}
	return nil
}
