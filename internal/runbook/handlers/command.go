package handlers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/shawnbutts/keystone-core/internal/execution"
	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// CommandHandler executes shell commands.
type CommandHandler struct {
	executor *execution.Executor
}

// NewCommandHandler creates a new command handler.
func NewCommandHandler() *CommandHandler {
	return &CommandHandler{
		executor: execution.NewExecutor(nil),
	}
}

// NewCommandHandlerWithExecutor creates a new command handler with a custom executor.
func NewCommandHandlerWithExecutor(executor *execution.Executor) *CommandHandler {
	return &CommandHandler{
		executor: executor,
	}
}

// Type returns the step type.
func (h *CommandHandler) Type() runbook.StepType {
	return runbook.StepTypeCommand
}

// Validate checks step config.
func (h *CommandHandler) Validate(step *runbook.Step) error {
	_, hasCommand := step.Config["command"]
	_, hasScript := step.Config["script"]

	if !hasCommand && !hasScript {
		return errors.New("command step requires either 'command' or 'script' in config")
	}

	if hasCommand && hasScript {
		return errors.New("command step cannot have both 'command' and 'script'")
	}

	if hasCommand {
		if _, ok := step.Config["command"].(string); !ok {
			return errors.New("command must be a string")
		}
	}

	if hasScript {
		if _, ok := step.Config["script"].(string); !ok {
			return errors.New("script must be a string")
		}
	}

	// Validate optional shell type
	if shellType, ok := step.Config["shell"].(string); ok {
		switch execution.ShellType(shellType) {
		case execution.ShellTypeBash, execution.ShellTypeSh,
			execution.ShellTypePowershell, execution.ShellTypeCmd,
			execution.ShellTypeDefault:
			// Valid shell types
		default:
			return fmt.Errorf("unknown shell type: %s", shellType)
		}
	}

	// Validate optional timeout
	if timeout, ok := step.Config["timeout"].(string); ok {
		if _, err := time.ParseDuration(timeout); err != nil {
			return errors.New("invalid timeout format")
		}
	}

	return nil
}

// Execute runs the step.
func (h *CommandHandler) Execute(ctx context.Context, step *runbook.Step, vars VariableContext) (*runbook.StepResult, error) {
	start := time.Now()

	// Get command or script
	command := ""
	if cmd, ok := step.Config["command"].(string); ok {
		command = cmd
	} else if script, ok := step.Config["script"].(string); ok {
		command = script
	}

	// Build execution request
	req := &execution.ExecuteRequest{
		CommandID: fmt.Sprintf("runbook-%s-%s", vars.ExecutionID(), step.Name),
		Command:   command,
	}

	// Set shell type (default to using a shell for scripts)
	shellType := execution.ShellTypeDefault
	if shell, ok := step.Config["shell"].(string); ok {
		shellType = execution.ShellType(shell)
	}
	req.ShellType = shellType

	// Set working directory
	if workDir, ok := step.Config["working_dir"].(string); ok {
		req.WorkingDir = workDir
	}

	// Set environment variables
	if envMap, ok := step.Config["env"].(map[string]interface{}); ok {
		req.Env = make(map[string]string)
		for key, val := range envMap {
			if strVal, ok := val.(string); ok {
				req.Env[key] = strVal
			}
		}
	}

	// Set timeout
	if timeout, ok := step.Config["timeout"].(string); ok {
		if dur, err := time.ParseDuration(timeout); err == nil {
			req.Timeout = dur
		}
	}

	// Execute the command
	result, execErr := h.executor.Execute(ctx, req, nil)

	// Build step result
	stepResult := &runbook.StepResult{
		Duration: time.Since(start),
		Outputs:  make(map[string]interface{}),
	}

	// Always populate outputs even on failure
	stepResult.Outputs["exit_code"] = result.ExitCode
	stepResult.Outputs["stdout"] = string(result.Stdout)
	stepResult.Outputs["stderr"] = string(result.Stderr)
	stepResult.Outputs["duration_ms"] = result.EndTime.Sub(result.StartTime).Milliseconds()

	// Get expected exit code
	expectedCode := 0
	if expected, ok := step.Config["expected_exit_code"].(int); ok {
		expectedCode = expected
	}

	// Check if exit code matches expected (even if there was an exec error)
	if result.ExitCode == expectedCode {
		stepResult.Success = true
		stepResult.Message = fmt.Sprintf("command completed with exit code %d", result.ExitCode)
		return stepResult, nil
	}

	// Exit code doesn't match - check for execution errors
	if execErr != nil {
		stepResult.Success = false
		stepResult.Message = fmt.Sprintf("command failed: %v", execErr)
		return stepResult, execErr
	}

	// No exec error but wrong exit code
	stepResult.Success = false
	stepResult.Message = fmt.Sprintf("command exited with code %d, expected %d", result.ExitCode, expectedCode)
	return stepResult, fmt.Errorf("unexpected exit code: %d", result.ExitCode)
}
