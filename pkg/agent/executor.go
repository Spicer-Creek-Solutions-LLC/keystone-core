package agent

import (
	"bufio"
	"context"
	"fmt"
	"io"
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
	mu             sync.RWMutex
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
	cmd := exec.CommandContext(cmdCtx, req.Command, req.Args...)

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

	// TODO: Set user if specified (requires platform-specific code)
	// if req.User != "" {
	//     cmd.SysProcAttr = setUser(req.User)
	// }

	// Get stdout and stderr pipes
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Track the running command
	e.mu.Lock()
	e.runningCommands[req.CommandID] = cmd
	e.mu.Unlock()

	// Start the command
	if err := cmd.Start(); err != nil {
		e.mu.Lock()
		delete(e.runningCommands, req.CommandID)
		e.mu.Unlock()
		result.Error = err
		result.EndTime = time.Now()
		return result, fmt.Errorf("failed to start command: %w", err)
	}

	// Stream output
	var wg sync.WaitGroup
	var stdoutBuf, stderrBuf []byte
	var stdoutMu, stderrMu sync.Mutex

	// Stream stdout
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			data := scanner.Bytes()
			dataCopy := make([]byte, len(data))
			copy(dataCopy, data)

			// Call output handler if provided
			if outputHandler != nil {
				outputHandler(req.CommandID, false, dataCopy)
			}

			// Store output
			stdoutMu.Lock()
			stdoutBuf = append(stdoutBuf, dataCopy...)
			stdoutBuf = append(stdoutBuf, '\n')
			stdoutMu.Unlock()
		}
	}()

	// Stream stderr
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			data := scanner.Bytes()
			dataCopy := make([]byte, len(data))
			copy(dataCopy, data)

			// Call output handler if provided
			if outputHandler != nil {
				outputHandler(req.CommandID, true, dataCopy)
			}

			// Store output
			stderrMu.Lock()
			stderrBuf = append(stderrBuf, dataCopy...)
			stderrBuf = append(stderrBuf, '\n')
			stderrMu.Unlock()
		}
	}()

	// Wait for command to complete
	err = cmd.Wait()
	result.EndTime = time.Now()

	// Wait for output streaming to complete
	wg.Wait()

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

	// Copy output buffers
	stdoutMu.Lock()
	result.Stdout = stdoutBuf
	stdoutMu.Unlock()

	stderrMu.Lock()
	result.Stderr = stderrBuf
	stderrMu.Unlock()

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

// streamPipe reads from a pipe and calls the handler for each chunk
func streamPipe(reader io.Reader, commandID string, isStderr bool, handler OutputHandler) ([]byte, error) {
	var output []byte
	scanner := bufio.NewScanner(reader)

	for scanner.Scan() {
		line := scanner.Bytes()
		lineCopy := make([]byte, len(line))
		copy(lineCopy, line)

		if handler != nil {
			handler(commandID, isStderr, lineCopy)
		}

		output = append(output, lineCopy...)
		output = append(output, '\n')
	}

	if err := scanner.Err(); err != nil {
		return output, err
	}

	return output, nil
}
