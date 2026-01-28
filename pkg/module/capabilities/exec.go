package capabilities

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ExecCapability allows executing commands
type ExecCapability struct {
	AllowedCommands []string      // List of allowed commands (full paths)
	TimeoutMax      time.Duration // Maximum timeout for command execution
	WorkingDir      string        // Working directory for commands (optional)
}

// Name returns the capability name
func (c *ExecCapability) Name() string {
	return "exec"
}

// Validate checks if the capability configuration is valid
func (c *ExecCapability) Validate() error {
	if len(c.AllowedCommands) == 0 {
		return fmt.Errorf("%w: at least one allowed command required", ErrInvalidConfiguration)
	}

	if c.TimeoutMax <= 0 {
		return fmt.Errorf("%w: timeout must be positive", ErrInvalidConfiguration)
	}

	return nil
}

// CheckCommand validates if a command is allowed
func (c *ExecCapability) CheckCommand(command string) error {
	allowed := false
	for _, allowedCmd := range c.AllowedCommands {
		if command == allowedCmd {
			allowed = true
			break
		}
	}

	if !allowed {
		return fmt.Errorf("%w: %s", ErrCommandNotAllowed, command)
	}

	return nil
}

// Exec executes a command with arguments
func (c *ExecCapability) Exec(capCtx *CapabilityContext, command string, args ...string) (*ExecResult, error) {
	// Check if command is allowed
	if err := c.CheckCommand(command); err != nil {
		return nil, err
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(capCtx.Context, c.TimeoutMax)
	defer cancel()

	// Create command
	cmd := exec.CommandContext(ctx, command, args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled

	// Set working directory if configured
	if c.WorkingDir != "" {
		cmd.Dir = c.WorkingDir
	}

	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute command
	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	// Build result
	result := &ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: duration,
	}

	// Get exit code
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("%w: command execution exceeded timeout %v", ErrTimeout, c.TimeoutMax)
		} else {
			return nil, fmt.Errorf("failed to execute command: %w", err)
		}
	} else {
		result.ExitCode = 0
	}

	return result, nil
}

// ExecWithInput executes a command with stdin input
func (c *ExecCapability) ExecWithInput(capCtx *CapabilityContext, input string, command string, args ...string) (*ExecResult, error) {
	// Check if command is allowed
	if err := c.CheckCommand(command); err != nil {
		return nil, err
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(capCtx.Context, c.TimeoutMax)
	defer cancel()

	// Create command
	cmd := exec.CommandContext(ctx, command, args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled

	// Set working directory if configured
	if c.WorkingDir != "" {
		cmd.Dir = c.WorkingDir
	}

	// Set up stdin
	cmd.Stdin = strings.NewReader(input)

	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute command
	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	// Build result
	result := &ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: duration,
	}

	// Get exit code
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("%w: command execution exceeded timeout %v", ErrTimeout, c.TimeoutMax)
		} else {
			return nil, fmt.Errorf("failed to execute command: %w", err)
		}
	} else {
		result.ExitCode = 0
	}

	return result, nil
}

// ExecResult represents the result of a command execution
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
}

// Success returns true if the command executed successfully (exit code 0)
func (r *ExecResult) Success() bool {
	return r.ExitCode == 0
}
