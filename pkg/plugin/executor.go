package plugin

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// Executor handles plugin execution
type Executor struct {
	plugin *Plugin
}

// NewExecutor creates a new plugin executor
func NewExecutor(plugin *Plugin) *Executor {
	return &Executor{
		plugin: plugin,
	}
}

// ExecuteOptions contains options for plugin execution
type ExecuteOptions struct {
	Args   []string        // Command-line arguments
	Env    []string        // Additional environment variables
	Stdout io.Writer       // Standard output writer
	Stderr io.Writer       // Standard error writer
	Stdin  io.Reader       // Standard input reader
	Dir    string          // Working directory
	Ctx    context.Context // Context for cancellation
}

// Execute runs the plugin with the given options
func (e *Executor) Execute(opts ExecuteOptions) error {
	if opts.Ctx == nil {
		opts.Ctx = context.Background()
	}

	// Create command
	cmd := exec.CommandContext(opts.Ctx, e.plugin.Path, opts.Args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled

	// Set working directory
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}

	// Set environment
	cmd.Env = os.Environ()
	if len(opts.Env) > 0 {
		cmd.Env = append(cmd.Env, opts.Env...)
	}

	// Set up I/O
	if opts.Stdout != nil {
		cmd.Stdout = opts.Stdout
	} else {
		cmd.Stdout = os.Stdout
	}

	if opts.Stderr != nil {
		cmd.Stderr = opts.Stderr
	} else {
		cmd.Stderr = os.Stderr
	}

	if opts.Stdin != nil {
		cmd.Stdin = opts.Stdin
	} else {
		cmd.Stdin = os.Stdin
	}

	// Run command
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("plugin execution failed: %w", err)
	}

	return nil
}

// ExecuteWithOutput runs the plugin and captures output
func (e *Executor) ExecuteWithOutput(ctx context.Context, args ...string) (stdout, stderr string, err error) {
	if ctx == nil {
		ctx = context.Background()
	}

	cmd := exec.CommandContext(ctx, e.plugin.Path, args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	cmd.Env = os.Environ()

	outBytes, errBytes, execErr := executeAndCapture(cmd)

	return string(outBytes), string(errBytes), execErr
}

// executeAndCapture is a helper to capture both stdout and stderr
func executeAndCapture(cmd *exec.Cmd) (stdout, stderr []byte, err error) {
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("failed to start command: %w", err)
	}

	stdoutBytes, stdoutErr := io.ReadAll(stdoutPipe)
	stderrBytes, stderrErr := io.ReadAll(stderrPipe)

	waitErr := cmd.Wait()

	// Check for read errors
	if stdoutErr != nil {
		return nil, nil, fmt.Errorf("failed to read stdout: %w", stdoutErr)
	}
	if stderrErr != nil {
		return nil, nil, fmt.Errorf("failed to read stderr: %w", stderrErr)
	}

	return stdoutBytes, stderrBytes, waitErr
}
