// Copyright 2024 Keystone Core Contributors
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package execution

import (
	"context"
	"fmt"
	"time"
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

// Policy represents PowerShell execution policy
type Policy string

// PolicyRestricted constants define the policy types.
const (
	PolicyRestricted   Policy = "Restricted"
	PolicyAllSigned    Policy = "AllSigned"
	PolicyRemoteSigned Policy = "RemoteSigned"
	PolicyUnrestricted Policy = "Unrestricted"
	PolicyBypass       Policy = "Bypass"
	PolicyUndefined    Policy = "Undefined"
)

// PowerShellExecutor provides enhanced PowerShell execution capabilities
// On non-Windows platforms, this is a stub that returns errors
type PowerShellExecutor struct {
	// PreferCore prefers PowerShell Core (pwsh) over Windows PowerShell
	PreferCore bool

	// UseBypassPolicy uses -Policy Bypass for script execution
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
// On non-Windows platforms, this returns an error
func (e *PowerShellExecutor) DetectPowerShell() (*PowerShellVersion, error) {
	return nil, fmt.Errorf("PowerShell is only available on Windows")
}

// GetPolicy returns the current PowerShell execution policy
// On non-Windows platforms, this returns an error
func (e *PowerShellExecutor) GetPolicy(ctx context.Context) (Policy, error) {
	return PolicyUndefined, fmt.Errorf("PowerShell is only available on Windows")
}

// Execute runs a PowerShell command or script block
// On non-Windows platforms, this returns an error
func (e *PowerShellExecutor) Execute(ctx context.Context, script string) (*Result, error) {
	return nil, fmt.Errorf("PowerShell is only available on Windows")
}

// ExecuteFile runs a PowerShell script file
// On non-Windows platforms, this returns an error
func (e *PowerShellExecutor) ExecuteFile(ctx context.Context, scriptPath string, scriptArgs ...string) (*Result, error) {
	return nil, fmt.Errorf("PowerShell is only available on Windows")
}

// CmdExecutor provides enhanced cmd.exe execution capabilities
// On non-Windows platforms, this is a stub that returns errors
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
// On non-Windows platforms, this returns an error
func (e *CmdExecutor) Execute(ctx context.Context, command string) (*Result, error) {
	return nil, fmt.Errorf("cmd.exe is only available on Windows")
}

// ExecuteBatch runs a batch file
// On non-Windows platforms, this returns an error
func (e *CmdExecutor) ExecuteBatch(ctx context.Context, batchPath string, args ...string) (*Result, error) {
	return nil, fmt.Errorf("cmd.exe is only available on Windows")
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
