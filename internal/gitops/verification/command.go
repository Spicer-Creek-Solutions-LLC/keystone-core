package verification

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// CommandVerifier executes shell commands for verification
type CommandVerifier struct{}

// NewCommandVerifier creates a new command verifier
func NewCommandVerifier() *CommandVerifier {
	return &CommandVerifier{}
}

// Type returns the verification type
func (v *CommandVerifier) Type() VerificationType {
	return VerificationTypeCommand
}

// Verify executes a command and checks its result
func (v *CommandVerifier) Verify(step *VerificationStep) (*VerificationResult, error) {
	result := &VerificationResult{
		StepName:  step.Name,
		Timestamp: time.Now(),
		Data:      make(map[string]interface{}),
	}

	// Extract configuration
	command, ok := step.Config["command"].(string)
	if !ok || command == "" {
		result.Success = false
		result.Message = "Missing 'command' in config"
		return result, fmt.Errorf("missing command")
	}

	// Optional: expected exit code (default 0)
	expectedExitCode := 0
	if code, ok := step.Config["expected_exit_code"].(int); ok {
		expectedExitCode = code
	} else if code, ok := step.Config["expected_exit_code"].(float64); ok {
		expectedExitCode = int(code)
	}

	// Optional: expected output
	expectedOutput, _ := step.Config["expected_output"].(string)

	// Optional: working directory
	workDir, _ := step.Config["working_dir"].(string)

	// Execute command
	cmd := exec.Command("sh", "-c", command)
	if workDir != "" {
		cmd.Dir = workDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			result.Success = false
			result.Message = fmt.Sprintf("Failed to execute command: %v", err)
			return result, err
		}
	}

	result.Data["exit_code"] = exitCode
	result.Data["stdout"] = stdout.String()
	result.Data["stderr"] = stderr.String()
	result.Data["duration_ms"] = duration.Milliseconds()

	// Check exit code
	if exitCode != expectedExitCode {
		result.Success = false
		result.Message = fmt.Sprintf("Expected exit code %d, got %d", expectedExitCode, exitCode)
		return result, nil
	}

	// Check output if specified
	if expectedOutput != "" {
		output := stdout.String()
		if !strings.Contains(output, expectedOutput) {
			result.Success = false
			result.Message = fmt.Sprintf("Output does not contain expected text: %s", expectedOutput)
			return result, nil
		}
	}

	// Success
	result.Success = true
	result.Message = fmt.Sprintf("Command executed successfully (exit code %d)", exitCode)

	return result, nil
}

// ScriptVerifier executes custom scripts for verification
type ScriptVerifier struct{}

// NewScriptVerifier creates a new script verifier
func NewScriptVerifier() *ScriptVerifier {
	return &ScriptVerifier{}
}

// Type returns the verification type
func (v *ScriptVerifier) Type() VerificationType {
	return VerificationTypeScript
}

// Verify executes a script and checks its result
func (v *ScriptVerifier) Verify(step *VerificationStep) (*VerificationResult, error) {
	result := &VerificationResult{
		StepName:  step.Name,
		Timestamp: time.Now(),
		Data:      make(map[string]interface{}),
	}

	// Extract configuration
	script, ok := step.Config["script"].(string)
	if !ok || script == "" {
		result.Success = false
		result.Message = "Missing 'script' in config"
		return result, fmt.Errorf("missing script")
	}

	// Optional: script arguments
	var args []string
	if argsInterface, ok := step.Config["args"]; ok {
		switch a := argsInterface.(type) {
		case []interface{}:
			for _, arg := range a {
				if argStr, ok := arg.(string); ok {
					args = append(args, argStr)
				}
			}
		case []string:
			args = a
		}
	}

	// Optional: working directory
	workDir, _ := step.Config["working_dir"].(string)

	// Execute script
	cmd := exec.Command(script, args...)
	if workDir != "" {
		cmd.Dir = workDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			result.Success = false
			result.Message = fmt.Sprintf("Failed to execute script: %v", err)
			return result, err
		}
	}

	result.Data["exit_code"] = exitCode
	result.Data["stdout"] = stdout.String()
	result.Data["stderr"] = stderr.String()
	result.Data["duration_ms"] = duration.Milliseconds()

	// Success if exit code is 0
	if exitCode == 0 {
		result.Success = true
		result.Message = "Script executed successfully"
	} else {
		result.Success = false
		result.Message = fmt.Sprintf("Script failed with exit code %d", exitCode)
	}

	return result, nil
}
