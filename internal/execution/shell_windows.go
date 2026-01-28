// Copyright 2024 Keystone Core Contributors
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package execution

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// PowerShellVersion represents PowerShell version information
type PowerShellVersion struct {
	Major       int
	Minor       int
	Build       int
	Revision    int
	Edition     string // Desktop or Core
	Path        string
	IsCore      bool
	VersionText string
}

// ExecutionPolicy represents PowerShell execution policy
type ExecutionPolicy string

const (
	ExecutionPolicyRestricted   ExecutionPolicy = "Restricted"
	ExecutionPolicyAllSigned    ExecutionPolicy = "AllSigned"
	ExecutionPolicyRemoteSigned ExecutionPolicy = "RemoteSigned"
	ExecutionPolicyUnrestricted ExecutionPolicy = "Unrestricted"
	ExecutionPolicyBypass       ExecutionPolicy = "Bypass"
	ExecutionPolicyUndefined    ExecutionPolicy = "Undefined"
)

// PowerShellExecutor provides enhanced PowerShell execution capabilities
type PowerShellExecutor struct {
	// PreferCore prefers PowerShell Core (pwsh) over Windows PowerShell
	PreferCore bool

	// UseBypassPolicy uses -ExecutionPolicy Bypass for script execution
	UseBypassPolicy bool

	// NoProfile skips loading PowerShell profiles for faster execution
	NoProfile bool

	// NoLogo suppresses the PowerShell logo banner
	NoLogo bool

	// OutputEncoding forces UTF-8 output encoding
	OutputEncoding string

	// WorkingDirectory is the working directory for command execution
	WorkingDirectory string

	// Environment is additional environment variables
	Environment map[string]string

	// Timeout is the maximum execution time (0 = no timeout)
	Timeout time.Duration

	// cached version info
	version *PowerShellVersion
}

// NewPowerShellExecutor creates a new PowerShell executor with sensible defaults
func NewPowerShellExecutor() *PowerShellExecutor {
	return &PowerShellExecutor{
		PreferCore:      true,
		UseBypassPolicy: true,
		NoProfile:       true,
		NoLogo:          true,
		OutputEncoding:  "UTF8",
	}
}

// DetectPowerShell finds the best available PowerShell installation
func (e *PowerShellExecutor) DetectPowerShell() (*PowerShellVersion, error) {
	if e.version != nil {
		return e.version, nil
	}

	// Try PowerShell Core first if preferred
	if e.PreferCore {
		if path, err := exec.LookPath("pwsh"); err == nil {
			version, err := e.getPowerShellVersion(path)
			if err == nil {
				e.version = version
				return version, nil
			}
		}
	}

	// Try Windows PowerShell
	if path, err := exec.LookPath("powershell"); err == nil {
		version, err := e.getPowerShellVersion(path)
		if err == nil {
			e.version = version
			return version, nil
		}
	}

	// Try PowerShell Core if not preferred but Windows PowerShell not found
	if !e.PreferCore {
		if path, err := exec.LookPath("pwsh"); err == nil {
			version, err := e.getPowerShellVersion(path)
			if err == nil {
				e.version = version
				return version, nil
			}
		}
	}

	return nil, fmt.Errorf("PowerShell not found")
}

// getPowerShellVersion gets version info from a PowerShell executable
func (e *PowerShellExecutor) getPowerShellVersion(path string) (*PowerShellVersion, error) {
	cmd := exec.Command(path, "-NoProfile", "-Command", "$PSVersionTable.PSVersion.ToString()") // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	versionText := strings.TrimSpace(string(output))

	// Determine if Core or Desktop
	isCore := strings.Contains(strings.ToLower(filepath.Base(path)), "pwsh")
	edition := "Desktop"
	if isCore {
		edition = "Core"
	}

	version := &PowerShellVersion{
		Path:        path,
		IsCore:      isCore,
		Edition:     edition,
		VersionText: versionText,
	}

	// Parse version numbers
	parts := strings.Split(versionText, ".")
	if len(parts) >= 1 {
		fmt.Sscanf(parts[0], "%d", &version.Major)
	}
	if len(parts) >= 2 {
		fmt.Sscanf(parts[1], "%d", &version.Minor)
	}
	if len(parts) >= 3 {
		fmt.Sscanf(parts[2], "%d", &version.Build)
	}
	if len(parts) >= 4 {
		fmt.Sscanf(parts[3], "%d", &version.Revision)
	}

	return version, nil
}

// GetExecutionPolicy returns the current PowerShell execution policy
func (e *PowerShellExecutor) GetExecutionPolicy(ctx context.Context) (ExecutionPolicy, error) {
	version, err := e.DetectPowerShell()
	if err != nil {
		return ExecutionPolicyUndefined, err
	}

	cmd := exec.CommandContext(ctx, version.Path, "-NoProfile", "-Command", "Get-ExecutionPolicy") // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.Output()
	if err != nil {
		return ExecutionPolicyUndefined, err
	}

	policy := strings.TrimSpace(string(output))
	return ExecutionPolicy(policy), nil
}

// Execute runs a PowerShell command or script block
func (e *PowerShellExecutor) Execute(ctx context.Context, script string) (*Result, error) {
	version, err := e.DetectPowerShell()
	if err != nil {
		return nil, err
	}

	args := e.buildArgs(script, false)

	return e.runCommand(ctx, version.Path, args)
}

// ExecuteFile runs a PowerShell script file
func (e *PowerShellExecutor) ExecuteFile(ctx context.Context, scriptPath string, scriptArgs ...string) (*Result, error) {
	version, err := e.DetectPowerShell()
	if err != nil {
		return nil, err
	}

	// Build the command to execute the script file
	scriptCmd := fmt.Sprintf("& '%s'", scriptPath)
	for _, arg := range scriptArgs {
		scriptCmd += fmt.Sprintf(" '%s'", strings.ReplaceAll(arg, "'", "''"))
	}

	args := e.buildArgs(scriptCmd, true)

	return e.runCommand(ctx, version.Path, args)
}

// buildArgs constructs PowerShell command line arguments
func (e *PowerShellExecutor) buildArgs(script string, isFile bool) []string {
	var args []string

	if e.NoLogo {
		args = append(args, "-NoLogo")
	}

	if e.NoProfile {
		args = append(args, "-NoProfile")
	}

	if e.UseBypassPolicy {
		args = append(args, "-ExecutionPolicy", "Bypass")
	}

	// Set output encoding
	if e.OutputEncoding != "" {
		// Prepend encoding command to script
		script = fmt.Sprintf("[Console]::OutputEncoding = [System.Text.Encoding]::%s; %s",
			e.OutputEncoding, script)
	}

	args = append(args, "-Command", script)

	return args
}

// runCommand executes a command and returns the result
func (e *PowerShellExecutor) runCommand(ctx context.Context, cmdPath string, args []string) (*Result, error) {
	if e.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, e.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, cmdPath, args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled

	// Set working directory
	if e.WorkingDirectory != "" {
		cmd.Dir = e.WorkingDirectory
	}

	// Set environment
	if len(e.Environment) > 0 {
		cmd.Env = os.Environ()
		for k, v := range e.Environment {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	// Hide the console window on Windows
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	result := &Result{
		Stdout:   e.decodeOutput(stdout.Bytes()),
		Stderr:   e.decodeOutput(stderr.Bytes()),
		ExitCode: 0,
		Duration: duration,
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else if ctx.Err() == context.DeadlineExceeded {
			result.ExitCode = -1
			result.Stderr = "execution timed out"
			return result, ctx.Err()
		} else {
			result.ExitCode = -1
			result.Stderr = err.Error()
		}
	}

	return result, nil
}

// decodeOutput handles Windows output encoding (UTF-16LE to UTF-8)
func (e *PowerShellExecutor) decodeOutput(data []byte) string {
	// Check for UTF-16LE BOM
	if len(data) >= 2 && data[0] == 0xFF && data[1] == 0xFE {
		decoder := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder()
		result, _, err := transform.Bytes(decoder, data)
		if err == nil {
			return strings.TrimSpace(string(result))
		}
	}
	return strings.TrimSpace(string(data))
}

// CmdExecutor provides enhanced cmd.exe execution capabilities
type CmdExecutor struct {
	// WorkingDirectory is the working directory for command execution
	WorkingDirectory string

	// Environment is additional environment variables
	Environment map[string]string

	// Timeout is the maximum execution time (0 = no timeout)
	Timeout time.Duration

	// HideWindow hides the console window during execution
	HideWindow bool
}

// NewCmdExecutor creates a new cmd.exe executor with sensible defaults
func NewCmdExecutor() *CmdExecutor {
	return &CmdExecutor{
		HideWindow: true,
	}
}

// Execute runs a cmd.exe command
func (e *CmdExecutor) Execute(ctx context.Context, command string) (*Result, error) {
	if e.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, e.Timeout)
		defer cancel()
	}

	// Use /C to run command and exit
	cmd := exec.CommandContext(ctx, "cmd", "/C", command) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled

	// Set working directory
	if e.WorkingDirectory != "" {
		cmd.Dir = e.WorkingDirectory
	}

	// Set environment
	if len(e.Environment) > 0 {
		cmd.Env = os.Environ()
		for k, v := range e.Environment {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	// Hide the console window if requested
	if e.HideWindow {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow: true,
		}
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	result := &Result{
		Stdout:   strings.TrimSpace(stdout.String()),
		Stderr:   strings.TrimSpace(stderr.String()),
		ExitCode: 0,
		Duration: duration,
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else if ctx.Err() == context.DeadlineExceeded {
			result.ExitCode = -1
			result.Stderr = "execution timed out"
			return result, ctx.Err()
		} else {
			result.ExitCode = -1
			result.Stderr = err.Error()
		}
	}

	return result, nil
}

// ExecuteBatch runs a batch file
func (e *CmdExecutor) ExecuteBatch(ctx context.Context, batchPath string, args ...string) (*Result, error) {
	// Build command with batch file and arguments
	command := fmt.Sprintf(`"%s"`, batchPath)
	for _, arg := range args {
		command += fmt.Sprintf(` "%s"`, arg)
	}

	return e.Execute(ctx, command)
}

// Result represents command execution result
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
}

// Success returns true if the command exited successfully
func (r *Result) Success() bool {
	return r.ExitCode == 0
}

// Output returns the combined stdout and stderr
func (r *Result) Output() string {
	if r.Stderr != "" {
		return r.Stdout + "\n" + r.Stderr
	}
	return r.Stdout
}
