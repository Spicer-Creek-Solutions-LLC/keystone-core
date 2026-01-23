package execution

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
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
	Attempts  int // Number of attempts made (for retries)
}

// OutputHandler is called for each line of output
type OutputHandler func(commandID string, isStderr bool, data []byte)

// ExecuteRequest represents a command execution request
type ExecuteRequest struct {
	CommandID  string
	Command    string   // The command/script to execute
	Args       []string // Arguments (only used if Shell is nil)
	Env        map[string]string
	WorkingDir string
	Timeout    time.Duration
	Shell      Shell         // Shell to use (nil means direct execution)
	ShellType  ShellType     // Shell type (alternative to Shell)
	Retries    int           // Number of retries on failure (0 = no retry)
	RetryDelay time.Duration // Delay between retries
}

// Executor handles command execution with shell support, retries, and improved cancellation
type Executor struct {
	mu              sync.RWMutex
	runningCommands map[string]*runningCommand
	killTimeout     time.Duration    // Grace period before SIGKILL
	policy          *CommandPolicy   // Security policy for command validation
}

// runningCommand tracks a running command
type runningCommand struct {
	cmd    *exec.Cmd
	cancel context.CancelFunc
}

// ExecutorOptions configures the executor
type ExecutorOptions struct {
	KillTimeout time.Duration    // Grace period between SIGTERM and SIGKILL (default: 5s)
	Policy      *CommandPolicy   // Security policy for command validation (default: normal mode)
}

// NewExecutor creates a new enhanced command executor
func NewExecutor(opts *ExecutorOptions) *Executor {
	killTimeout := 5 * time.Second
	var policy *CommandPolicy

	if opts != nil {
		if opts.KillTimeout > 0 {
			killTimeout = opts.KillTimeout
		}
		if opts.Policy != nil {
			policy = opts.Policy
		}
	}

	// Default to normal mode policy for security
	if policy == nil {
		policy = DefaultPolicy()
	}

	return &Executor{
		runningCommands: make(map[string]*runningCommand),
		killTimeout:     killTimeout,
		policy:          policy,
	}
}

// Execute runs a command with optional retries and shell support
func (e *Executor) Execute(ctx context.Context, req *ExecuteRequest, outputHandler OutputHandler) (*CommandResult, error) {
	result := &CommandResult{
		CommandID: req.CommandID,
		StartTime: time.Now(),
	}

	// Validate command against security policy
	if e.policy != nil {
		var err error
		if req.Shell != nil || req.ShellType != "" {
			// Shell execution - use stricter validation
			err = e.policy.ValidateForShell(req.Command)
		} else {
			// Direct execution
			err = e.policy.Validate(req.Command)
		}
		if err != nil {
			result.Error = fmt.Errorf("command rejected by security policy: %w", err)
			result.EndTime = time.Now()
			result.ExitCode = -1
			return result, result.Error
		}
	}

	// Execute with retries
	maxAttempts := req.Retries + 1
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result.Attempts = attempt

		execResult, err := e.executeOnce(ctx, req, outputHandler)
		result.ExitCode = execResult.ExitCode
		result.Stdout = execResult.Stdout
		result.Stderr = execResult.Stderr
		result.Error = execResult.Error
		result.EndTime = execResult.EndTime

		// Success - return immediately
		if err == nil && execResult.ExitCode == 0 {
			return result, nil
		}

		// Last attempt - return result
		if attempt == maxAttempts {
			return result, err
		}

		// Wait before retry
		if req.RetryDelay > 0 {
			select {
			case <-time.After(req.RetryDelay):
				// Continue to retry
			case <-ctx.Done():
				// Context cancelled during retry delay
				result.Error = ctx.Err()
				return result, ctx.Err()
			}
		}
	}

	return result, nil
}

// executeOnce executes the command once without retries
func (e *Executor) executeOnce(ctx context.Context, req *ExecuteRequest, outputHandler OutputHandler) (*CommandResult, error) {
	result := &CommandResult{
		CommandID: req.CommandID,
		StartTime: time.Now(),
	}

	// Create command context with timeout
	cmdCtx := ctx
	var cancel context.CancelFunc
	if req.Timeout > 0 {
		cmdCtx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	// Resolve shell if needed
	var shell Shell
	var err error
	if req.Shell != nil {
		shell = req.Shell
	} else if req.ShellType != "" {
		shell, err = GetShell(req.ShellType)
		if err != nil {
			result.Error = err
			result.EndTime = time.Now()
			return result, err
		}
	}

	// Create the command
	var cmd *exec.Cmd
	if shell != nil {
		// Use shell to execute the command
		shellCmd, shellArgs := shell.Command(req.Command)
		cmd = exec.CommandContext(cmdCtx, shellCmd, shellArgs...)
	} else {
		// Direct execution without shell
		cmd = exec.CommandContext(cmdCtx, req.Command, req.Args...)
	}

	// Set working directory
	if req.WorkingDir != "" {
		cmd.Dir = req.WorkingDir
	}

	// Set environment variables
	if len(req.Env) > 0 {
		env := os.Environ()
		for key, value := range req.Env {
			env = append(env, fmt.Sprintf("%s=%s", key, value))
		}
		cmd.Env = env
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
	e.runningCommands[req.CommandID] = &runningCommand{
		cmd:    cmd,
		cancel: cancel,
	}
	e.mu.Unlock()

	// Wait for command to complete
	err = cmd.Wait()
	result.EndTime = time.Now()

	// Remove from running commands
	e.mu.Lock()
	delete(e.runningCommands, req.CommandID)
	e.mu.Unlock()

	// Get exit code
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
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

	return result, err
}

// CancelCommand cancels a running command with graceful SIGTERM → SIGKILL
func (e *Executor) CancelCommand(commandID string) error {
	e.mu.RLock()
	running, exists := e.runningCommands[commandID]
	e.mu.RUnlock()

	if !exists {
		return fmt.Errorf("command %s not found", commandID)
	}

	// Cancel the context (sends SIGTERM on Unix)
	if running.cancel != nil {
		running.cancel()
	}

	// If process still exists, wait for grace period then SIGKILL
	if running.cmd.Process != nil {
		go func() {
			time.Sleep(e.killTimeout)

			// Check if process still exists
			e.mu.RLock()
			_, stillExists := e.runningCommands[commandID]
			e.mu.RUnlock()

			if stillExists && running.cmd.Process != nil {
				// Send SIGKILL
				_ = running.cmd.Process.Signal(syscall.SIGKILL)
			}
		}()
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

// GetPolicy returns the current command policy
func (e *Executor) GetPolicy() *CommandPolicy {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.policy
}

// SetPolicy updates the command policy
func (e *Executor) SetPolicy(policy *CommandPolicy) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.policy = policy
}

