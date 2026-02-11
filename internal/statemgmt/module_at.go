// Package statemgmt provides state management modules.
package statemgmt

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// AtModule manages one-time scheduled tasks using the at command.
type AtModule struct {
	*BaseModule
}

// AtConfig holds at job configuration.
type AtConfig struct {
	// Name is a unique identifier for tracking (stored in command comment)
	Name string
	// Command is the command to execute
	Command string
	// Time specifies when to run (e.g., "now + 1 hour", "10:00", "midnight", "noon")
	Time string
	// Date specifies the date (e.g., "tomorrow", "next week", "2024-12-25")
	Date string
	// Queue is the job queue (a-z, A-Z)
	Queue string
	// SendMail if true, sends mail even if no output
	SendMail bool
	// NoMail if true, never sends mail
	NoMail bool
}

// NewAtModule creates a new at module.
func NewAtModule() *AtModule {
	return &AtModule{
		BaseModule: NewBaseModule("at", []string{"present", "absent"}),
	}
}

// Check determines if the at job exists.
func (m *AtModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("at module is not supported on Windows (use scheduled_task instead)")
	}

	config, err := m.parseConfig(decl)
	if err != nil {
		return nil, err
	}

	exists, _ := m.jobExists(ctx, config)

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

// Apply creates or removes the at job.
func (m *AtModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	result := &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
	}

	if runtime.GOOS == "windows" {
		result.Success = false
		result.Comment = "at module is not supported on Windows (use scheduled_task instead)"
		return result, fmt.Errorf("at module is not supported on Windows")
	}

	config, err := m.parseConfig(decl)
	if err != nil {
		result.Success = false
		result.Comment = fmt.Sprintf("Failed to parse config: %v", err)
		return result, err
	}

	switch decl.State {
	case "present":
		// Remove existing job if it exists - best-effort, we'll create a new job regardless
		if exists, jobID := m.jobExists(ctx, config); exists {
			_ = m.removeJob(ctx, jobID)
		}

		// Create the job
		jobID, err := m.createJob(ctx, config)
		if err != nil {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to create at job: %v", err)
			return result, err
		}

		result.Changed = true
		result.Comment = fmt.Sprintf("At job '%s' created with ID %s", config.Name, jobID)

	case "absent":
		exists, jobID := m.jobExists(ctx, config)
		if !exists {
			result.Comment = fmt.Sprintf("At job '%s' already absent", config.Name)
			return result, nil
		}

		if err := m.removeJob(ctx, jobID); err != nil {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to remove at job: %v", err)
			return result, err
		}

		result.Changed = true
		result.Comment = fmt.Sprintf("At job '%s' removed", config.Name)

	default:
		result.Success = false
		result.Comment = fmt.Sprintf("Unknown state: %s", decl.State)
		return result, fmt.Errorf("unknown state: %s", decl.State)
	}

	return result, nil
}

// Test performs a dry-run check of the operation.
func (m *AtModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

// parseConfig parses parameters into AtConfig.
func (m *AtModule) parseConfig(decl *StateDeclaration) (*AtConfig, error) {
	config := &AtConfig{}

	config.Name = decl.ID

	config.Command = getStringParameter(decl, "command", "")
	if config.Command == "" {
		return nil, fmt.Errorf("command is required")
	}

	config.Time = getStringParameter(decl, "time", "")
	if config.Time == "" {
		return nil, fmt.Errorf("time is required")
	}

	config.Date = getStringParameter(decl, "date", "")

	queue := getStringParameter(decl, "queue", "")
	if queue != "" {
		if len(queue) != 1 || ((queue[0] < 'a' || queue[0] > 'z') && (queue[0] < 'A' || queue[0] > 'Z')) {
			return nil, fmt.Errorf("queue must be a single letter (a-z or A-Z)")
		}
		config.Queue = queue
	}

	config.SendMail = getBoolParameter(decl, "send_mail", false)
	config.NoMail = getBoolParameter(decl, "no_mail", false)

	return config, nil
}

// jobExists checks if a job with the given name exists.
func (m *AtModule) jobExists(ctx context.Context, config *AtConfig) (exists bool, jobID string) {
	// List all at jobs
	cmd := exec.CommandContext(ctx, "atq")
	output, err := cmd.Output()
	if err != nil {
		return false, ""
	}

	// Parse the job list
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		// Extract job ID (first field)
		fields := strings.Fields(line)
		if len(fields) < 1 {
			continue
		}
		id := fields[0]

		// Get job content to check for our marker
		content, err := m.getJobContent(ctx, id)
		if err != nil {
			continue
		}

		// Look for our marker
		marker := fmt.Sprintf("# Keystone Core: %s", config.Name)
		if strings.Contains(content, marker) {
			return true, id
		}
	}

	return false, ""
}

// getJobContent retrieves the content of an at job.
func (m *AtModule) getJobContent(ctx context.Context, jobID string) (string, error) {
	cmd := exec.CommandContext(ctx, "at", "-c", jobID)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

// createJob creates an at job.
func (m *AtModule) createJob(ctx context.Context, config *AtConfig) (string, error) {
	// Build the time specification
	timeSpec := config.Time
	if config.Date != "" {
		timeSpec = fmt.Sprintf("%s %s", config.Time, config.Date)
	}

	// Build command arguments
	args := []string{}

	if config.Queue != "" {
		args = append(args, "-q", config.Queue)
	}

	if config.SendMail {
		args = append(args, "-m")
	}

	if config.NoMail {
		args = append(args, "-M")
	}

	args = append(args, timeSpec)

	// Build the script content with our marker
	script := fmt.Sprintf("# Keystone Core: %s\n# Created: %s\n%s\n",
		config.Name,
		time.Now().Format(time.RFC3339),
		config.Command,
	)

	// Create the job
	//nolint:gosec // G204: at command execution is intentional for job scheduling
	cmd := exec.CommandContext(ctx, "at", args...)
	cmd.Stdin = strings.NewReader(script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("at command failed: %w: %s", err, string(output))
	}

	// Parse job ID from output
	// Output format varies:
	// - Linux: "job 123 at Wed Jan 1 10:00:00 2025"
	// - macOS: "job 123 at Wed Jan  1 10:00:00 2025"
	jobIDRegex := regexp.MustCompile(`job\s+(\d+)`)
	matches := jobIDRegex.FindStringSubmatch(string(output))
	if len(matches) >= 2 {
		return matches[1], nil
	}

	return "unknown", nil
}

// removeJob removes an at job.
func (m *AtModule) removeJob(ctx context.Context, jobID string) error {
	cmd := exec.CommandContext(ctx, "atrm", jobID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("atrm failed: %w: %s", err, string(output))
	}
	return nil
}

func init() {
	_ = RegisterModule(NewAtModule())
}
