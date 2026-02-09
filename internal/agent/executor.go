package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

// CommandResult represents the result of a command execution
type CommandResult struct {
	CommandID string
	ExitCode  int
	Stdout    []byte
	Stderr    []byte
	Error     error
	StartTime time.Time
	EndTime   time.Time
}

// OutputHandler is called for each line of output
type OutputHandler func(commandID string, isStderr bool, data []byte)

// Executor handles command execution on the agent
type Executor struct {
	mu              sync.RWMutex
	runningCommands map[string]*exec.Cmd
}

// NewExecutor creates a new command executor
func NewExecutor() *Executor {
	return &Executor{
		runningCommands: make(map[string]*exec.Cmd),
	}
}

// ExecuteCommandRequest represents a command to execute
type ExecuteCommandRequest struct {
	CommandID  string
	Command    string
	Args       []string
	Env        map[string]string
	WorkingDir string
	Timeout    time.Duration
	User       string
}

// Execute runs a command and streams output
func (e *Executor) Execute(ctx context.Context, req *ExecuteCommandRequest, outputHandler OutputHandler) (*CommandResult, error) {
	result := &CommandResult{
		CommandID: req.CommandID,
		StartTime: time.Now(),
	}

	// Create command with timeout context
	cmdCtx := ctx
	var cancel context.CancelFunc
	if req.Timeout > 0 {
		cmdCtx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	// Create the command
	//nolint:gosec // G204: command execution is intentional for remote execution system
	cmd := exec.CommandContext(cmdCtx, req.Command, req.Args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command

	// Set working directory
	if req.WorkingDir != "" {
		cmd.Dir = req.WorkingDir
	}

	// Set environment variables
	if len(req.Env) > 0 {
		env := cmd.Env
		for key, value := range req.Env {
			env = append(env, fmt.Sprintf("%s=%s", key, value))
		}
		cmd.Env = env
	}

	// Set user if specified (platform-specific)
	if req.User != "" {
		userSwitch, err := LookupUserForSwitch(req.User)
		if err != nil {
			result.Error = fmt.Errorf("failed to lookup user %q: %w", req.User, err)
			result.EndTime = time.Now()
			return result, result.Error
		}

		if userSwitch != nil {
			// Check if we can switch to this user
			if err := CanSwitchUser(req.User); err != nil {
				result.Error = err
				result.EndTime = time.Now()
				return result, err
			}

			cmd.SysProcAttr = SetUserCredential(cmd.SysProcAttr, userSwitch)

			// Set HOME environment variable for the target user
			if userSwitch.HomeDir != "" {
				if cmd.Env == nil {
					cmd.Env = []string{}
				}
				cmd.Env = append(cmd.Env, fmt.Sprintf("HOME=%s", userSwitch.HomeDir), fmt.Sprintf("USER=%s", userSwitch.Username))
			}
		}
	}

	// Use bytes.Buffer for output capture - simpler and avoids pipe race conditions
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	// Start the command
	if err := cmd.Start(); err != nil {
		result.Error = err
		result.EndTime = time.Now()
		return result, fmt.Errorf("failed to start command: %w", err)
	}

	// Track the running command (after Start so cmd.Process is set)
	e.mu.Lock()
	e.runningCommands[req.CommandID] = cmd
	e.mu.Unlock()

	// Wait for command to complete
	err := cmd.Wait()
	result.EndTime = time.Now()

	// Remove from running commands
	e.mu.Lock()
	delete(e.runningCommands, req.CommandID)
	e.mu.Unlock()

	// Get exit code
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.Error = err
			result.ExitCode = -1
		}
	} else {
		result.ExitCode = 0
	}

	// Get output from buffers
	result.Stdout = stdoutBuf.Bytes()
	result.Stderr = stderrBuf.Bytes()

	// Call output handler if provided
	if outputHandler != nil {
		if len(result.Stdout) > 0 {
			lines := bytes.Split(result.Stdout, []byte{'\n'})
			for _, line := range lines {
				if len(line) > 0 {
					outputHandler(req.CommandID, false, line)
				}
			}
		}
		if len(result.Stderr) > 0 {
			lines := bytes.Split(result.Stderr, []byte{'\n'})
			for _, line := range lines {
				if len(line) > 0 {
					outputHandler(req.CommandID, true, line)
				}
			}
		}
	}

	return result, nil
}

// CancelCommand cancels a running command
func (e *Executor) CancelCommand(commandID string) error {
	e.mu.RLock()
	cmd, exists := e.runningCommands[commandID]
	e.mu.RUnlock()

	if !exists {
		return fmt.Errorf("command %s not found", commandID)
	}

	if cmd.Process != nil {
		return cmd.Process.Kill()
	}

	return nil
}

// GetRunningCommands returns the list of currently running command IDs
func (e *Executor) GetRunningCommands() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	commandIDs := make([]string, 0, len(e.runningCommands))
	for id := range e.runningCommands {
		commandIDs = append(commandIDs, id)
	}

	return commandIDs
}
