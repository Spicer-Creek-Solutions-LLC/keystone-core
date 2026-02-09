// Package statemgmt provides state management modules.
package statemgmt

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

// CronModule manages cron jobs.
type CronModule struct {
	*BaseModule
}

// CronConfig holds cron job configuration.
type CronConfig struct {
	// Name is a unique identifier for the cron job (used in comments)
	Name string
	// Minute: 0-59, *, */n, or comma-separated
	Minute string
	// Hour: 0-23, *, */n, or comma-separated
	Hour string
	// Day: 1-31, *, */n, or comma-separated
	Day string
	// Month: 1-12, jan-dec, *, */n, or comma-separated
	Month string
	// Weekday: 0-7 (0 and 7 are Sunday), sun-sat, *, */n, or comma-separated
	Weekday string
	// Command is the command to execute
	Command string
	// User specifies which user's crontab to manage (requires root)
	User string
	// Special is a special schedule like @reboot, @hourly, @daily, @weekly, @monthly, @yearly
	Special string
	// Environment variables to set (key=value pairs)
	Environment map[string]string
	// Disabled if true, comments out the cron job
	Disabled bool
}

// NewCronModule creates a new cron module.
func NewCronModule() *CronModule {
	return &CronModule{
		BaseModule: NewBaseModule("cron", []string{"present", "absent"}),
	}
}

// Check determines if the cron job exists and matches.
func (m *CronModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	config, err := m.parseCronConfig(decl)
	if err != nil {
		return nil, err
	}

	// Get current crontab
	entries, err := m.getCrontab(ctx, config.User)
	if err != nil {
		result.Present = false
		result.CurrentState = "error"
		result.Matches = false
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	// Look for our managed entry
	exists, matches := m.findEntry(entries, config)

	switch decl.State {
	case "present":
		switch {
		case !exists:
			result.Present = false
			result.CurrentState = "absent"
			result.Matches = false
			result.Diff["state"] = map[string]string{"current": "absent", "desired": "present"}
		case !matches:
			result.Present = true
			result.CurrentState = "different"
			result.Matches = false
			result.Diff["state"] = map[string]string{"current": "different", "desired": "present"}
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

	return result, nil //nolint:nilerr // error captured in result.Error
}

// Apply creates or removes the cron job.
func (m *CronModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	result := &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
	}

	config, err := m.parseCronConfig(decl)
	if err != nil {
		result.Success = false
		result.Comment = fmt.Sprintf("Failed to parse config: %v", err)
		return result, err
	}

	// Get current crontab
	entries, err := m.getCrontab(ctx, config.User)
	if err != nil {
		// If no crontab exists, start with empty
		entries = []string{}
	}

	switch decl.State {
	case "present":
		// Build the new cron entry
		newEntry := m.buildEntry(config)

		// Remove any existing entry for this job
		entries = m.removeEntry(entries, config.Name)

		// Add environment variables
		for key, value := range config.Environment {
			entries = append(entries, fmt.Sprintf("%s=%s", key, value))
		}

		// Add our entry
		entries = append(entries, newEntry)

		// Write back the crontab
		if err := m.setCrontab(ctx, config.User, entries); err != nil {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to write crontab: %v", err)
			return result, err
		}

		result.Changed = true
		result.Comment = fmt.Sprintf("Cron job '%s' created", config.Name)

	case "absent":
		// Remove the entry
		newEntries := m.removeEntry(entries, config.Name)

		if len(newEntries) == len(entries) {
			result.Comment = fmt.Sprintf("Cron job '%s' already absent", config.Name)
			return result, nil //nolint:nilerr // error captured in result.Error
		}

		// Write back the crontab
		if err := m.setCrontab(ctx, config.User, newEntries); err != nil {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to write crontab: %v", err)
			return result, err
		}

		result.Changed = true
		result.Comment = fmt.Sprintf("Cron job '%s' removed", config.Name)

	default:
		result.Success = false
		result.Comment = fmt.Sprintf("Unknown state: %s", decl.State)
		return result, fmt.Errorf("unknown state: %s", decl.State)
	}

	return result, nil //nolint:nilerr // error captured in result.Error
}

// Test performs a dry-run check of the operation.
func (m *CronModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

// parseCronConfig parses parameters into CronConfig.
func (m *CronModule) parseCronConfig(decl *StateDeclaration) (*CronConfig, error) {
	config := &CronConfig{
		Minute:  "*",
		Hour:    "*",
		Day:     "*",
		Month:   "*",
		Weekday: "*",
	}

	config.Name = decl.ID

	config.Minute = getStringParameter(decl, "minute", "*")
	config.Hour = getStringParameter(decl, "hour", "*")
	config.Day = getStringParameter(decl, "day", "*")
	config.Month = getStringParameter(decl, "month", "*")
	config.Weekday = getStringParameter(decl, "weekday", "*")

	config.Command = getStringParameter(decl, "command", "")
	if config.Command == "" {
		return nil, fmt.Errorf("command is required")
	}

	config.User = getStringParameter(decl, "user", "")

	special := getStringParameter(decl, "special", "")
	if special != "" {
		if !isValidSpecial(special) {
			return nil, fmt.Errorf("invalid special schedule: %s", special)
		}
		config.Special = special
	}

	if env, ok := decl.Parameters["environment"].(map[string]interface{}); ok {
		config.Environment = make(map[string]string)
		for k, v := range env {
			if vs, ok := v.(string); ok {
				config.Environment[k] = vs
			}
		}
	}

	config.Disabled = getBoolParameter(decl, "disabled", false)

	return config, nil
}

// isValidSpecial checks if a special schedule string is valid.
func isValidSpecial(s string) bool {
	valid := []string{"@reboot", "@hourly", "@daily", "@midnight", "@weekly", "@monthly", "@yearly", "@annually"}
	for _, v := range valid {
		if s == v {
			return true
		}
	}
	return false
}

// getCrontab reads the current crontab for a user.
func (m *CronModule) getCrontab(ctx context.Context, user string) ([]string, error) {
	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("cron is not supported on Windows")
	}

	args := []string{"-l"}
	if user != "" {
		args = append([]string{"-u", user}, args...)
	}

	cmd := exec.CommandContext(ctx, "crontab", args...)
	output, err := cmd.Output()
	if err != nil {
		// No crontab for this user is not an error
		if strings.Contains(err.Error(), "no crontab") {
			return []string{}, nil
		}
		return nil, err
	}

	var entries []string
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		entries = append(entries, scanner.Text())
	}

	return entries, nil
}

// setCrontab writes the crontab for a user.
func (m *CronModule) setCrontab(ctx context.Context, user string, entries []string) error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("cron is not supported on Windows")
	}

	content := strings.Join(entries, "\n")
	if !strings.HasSuffix(content, "\n") && content != "" {
		content += "\n"
	}

	args := []string{"-"}
	if user != "" {
		args = append([]string{"-u", user}, args...)
	}

	cmd := exec.CommandContext(ctx, "crontab", args...)
	cmd.Stdin = strings.NewReader(content)

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to write crontab: %w: %s", err, string(output))
	}

	return nil
}

// buildEntry builds a cron entry line.
func (m *CronModule) buildEntry(config *CronConfig) string {
	var entry string

	if config.Disabled {
		entry = "#"
	}

	if config.Special != "" {
		entry += fmt.Sprintf("%s %s # Keystone Core: %s", config.Special, config.Command, config.Name)
	} else {
		entry += fmt.Sprintf("%s %s %s %s %s %s # Keystone Core: %s",
			config.Minute, config.Hour, config.Day, config.Month, config.Weekday,
			config.Command, config.Name)
	}

	return entry
}

// findEntry looks for a managed entry in the crontab.
func (m *CronModule) findEntry(entries []string, config *CronConfig) (exists, matches bool) {
	marker := fmt.Sprintf("# Keystone Core: %s", config.Name)
	expectedEntry := m.buildEntry(config)

	for _, entry := range entries {
		if strings.Contains(entry, marker) {
			exists = true
			// Normalize whitespace for comparison
			if normalizeWhitespace(entry) == normalizeWhitespace(expectedEntry) {
				matches = true
			}
			return
		}
	}

	return false, false
}

// removeEntry removes all entries for a named job.
func (m *CronModule) removeEntry(entries []string, name string) []string {
	marker := fmt.Sprintf("# Keystone Core: %s", name)
	var result []string

	for _, entry := range entries {
		if !strings.Contains(entry, marker) {
			result = append(result, entry)
		}
	}

	return result
}

// normalizeWhitespace collapses multiple whitespace to single space.
func normalizeWhitespace(s string) string {
	re := regexp.MustCompile(`\s+`)
	return strings.TrimSpace(re.ReplaceAllString(s, " "))
}
