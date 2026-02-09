package statemgmt

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// CmdModule implements command execution
type CmdModule struct {
	*BaseModule
}

// NewCmdModule creates a new cmd module
func NewCmdModule() *CmdModule {
	return &CmdModule{
		BaseModule: NewBaseModule("cmd", []string{"run", "wait"}),
	}
}

// Check checks if a command should be run
func (m *CmdModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	// For cmd module, we check conditions that prevent/allow execution

	// Check "creates" - if file exists, command shouldn't run
	if creates := getStringParameter(decl, "creates", ""); creates != "" {
		if _, err := os.Stat(creates); err == nil {
			result.Present = true
			result.CurrentState = "completed"
			result.Matches = true
			result.Metadata["reason"] = fmt.Sprintf("file %s already exists", creates)
			return result, nil
		}
	}

	// Check "removes" - if file doesn't exist, command shouldn't run
	if removes := getStringParameter(decl, "removes", ""); removes != "" {
		if _, err := os.Stat(removes); os.IsNotExist(err) {
			result.Present = true
			result.CurrentState = "completed"
			result.Matches = true
			result.Metadata["reason"] = fmt.Sprintf("file %s already absent", removes)
			return result, nil
		}
	}

	// For "wait" state, command should only run when triggered
	if decl.State == "wait" {
		result.Present = false
		result.CurrentState = "waiting"
		result.Matches = true
		result.Metadata["reason"] = "waiting for trigger"
		return result, nil
	}

	// For "run" state, command should be executed
	result.Present = false
	result.CurrentState = "pending"
	result.Matches = false
	result.Diff["state"] = map[string]string{"current": "pending", "desired": "run"}

	return result, nil
}

// Apply executes the command
func (m *CmdModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	result := &StateResult{
		StateID:   decl.ID,
		Module:    m.Name(),
		Success:   false,
		Changed:   false,
		Changes:   make(map[string]interface{}),
		StartTime: startTime,
	}

	// Check if we should run
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to check current state: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// If command shouldn't run (creates/removes conditions), skip
	if checkResult.Matches && decl.State != "wait" {
		result.Success = true
		result.Changed = false
		result.Comment = fmt.Sprintf("Skipped: %v", checkResult.Metadata["reason"])
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// For "wait" state, only run if triggered by a watch/onchanges
	// This would require executor context to know if it was triggered
	// For now, we'll just skip it
	if decl.State == "wait" {
		result.Success = true
		result.Changed = false
		result.Comment = "Waiting for trigger"
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// Execute the command
	applyErr := m.executeCommand(ctx, decl, result)
	if applyErr != nil {
		result.Error = applyErr
		result.Success = false
		result.Comment = fmt.Sprintf("Command failed: %v", applyErr)
	} else {
		result.Success = true
		result.Changed = true
		result.Changes = checkResult.Diff
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	return result, nil
}

// Test tests if the command conditions are met
func (m *CmdModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil
}

// executeCommand executes the command
func (m *CmdModule) executeCommand(ctx context.Context, decl *StateDeclaration, result *StateResult) error {
	cmdStr := decl.ID

	// Get working directory
	cwd := getStringParameter(decl, "cwd", "")

	// Get shell
	shell := getStringParameter(decl, "shell", "/bin/sh")

	// Get timeout
	timeout := getIntParameter(decl, "timeout", 0)
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()
	}

	// Get environment variables
	env := os.Environ()
	if envMap := decl.Parameters["env"]; envMap != nil {
		if envMapTyped, ok := envMap.(map[string]interface{}); ok {
			for key, value := range envMapTyped {
				env = append(env, fmt.Sprintf("%s=%v", key, value))
			}
		}
	}

	// Create command
	var cmd *exec.Cmd

	// Check if stateful (returns state codes).
	// For stateful commands, we expect the command to output
	// "changed=yes" or "changed=no" and optionally "comment=..."
	stateful := getBoolParameter(decl, "stateful", false)

	//nolint:gosec // G204: shell command execution is intentional for state management commands
	cmd = exec.CommandContext(ctx, shell, "-c", cmdStr) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled

	cmd.Env = env
	if cwd != "" {
		cmd.Dir = cwd
	}

	// Execute command
	output, err := cmd.CombinedOutput()
	outputStr := strings.TrimSpace(string(output))

	if err != nil {
		// Check if it's an exit error
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.Changes["exit_code"] = exitErr.ExitCode()
			result.Changes["output"] = outputStr
			return fmt.Errorf("command exited with code %d: %s", exitErr.ExitCode(), outputStr)
		}
		return fmt.Errorf("failed to execute command: %w", err)
	}

	// Parse stateful output
	if stateful {
		if strings.Contains(outputStr, "changed=yes") {
			result.Changed = true
		} else if strings.Contains(outputStr, "changed=no") {
			result.Changed = false
		}

		// Extract comment if present
		for _, line := range strings.Split(outputStr, "\n") {
			if strings.HasPrefix(line, "comment=") {
				result.Comment = strings.TrimPrefix(line, "comment=")
				break
			}
		}
	} else {
		result.Comment = "Command executed successfully"
	}

	result.Changes["exit_code"] = 0
	result.Changes["output"] = outputStr

	return nil
}

func init() {
	_ = RegisterModule(NewCmdModule()) //nolint:errcheck // module registration in init
}
