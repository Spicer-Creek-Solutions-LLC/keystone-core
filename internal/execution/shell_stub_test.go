// Copyright 2024 Keystone Core Contributors
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package execution

import (
	"context"
	"testing"
)

func TestNewPowerShellExecutor_NonWindows(t *testing.T) {
	exec := NewPowerShellExecutor()
	if exec == nil {
		t.Fatal("NewPowerShellExecutor returned nil")
	}

	// Check default values
	if !exec.PreferCore {
		t.Error("PreferCore should default to true")
	}
	if !exec.UseBypassPolicy {
		t.Error("UseBypassPolicy should default to true")
	}
	if !exec.NoProfile {
		t.Error("NoProfile should default to true")
	}
	if !exec.NoLogo {
		t.Error("NoLogo should default to true")
	}
	if exec.OutputEncoding != "UTF8" {
		t.Errorf("OutputEncoding should default to UTF8, got %s", exec.OutputEncoding)
	}
}

func TestNewCmdExecutor_NonWindows(t *testing.T) {
	exec := NewCmdExecutor()
	if exec == nil {
		t.Fatal("NewCmdExecutor returned nil")
	}

	// Check default values
	if !exec.HideWindow {
		t.Error("HideWindow should default to true")
	}
}

func TestPowerShellExecutor_DetectPowerShell_NonWindows(t *testing.T) {
	exec := NewPowerShellExecutor()

	version, err := exec.DetectPowerShell()
	if err == nil {
		t.Error("Expected error on non-Windows platform")
	}
	if version != nil {
		t.Error("Expected nil version on non-Windows platform")
	}
}

func TestPowerShellExecutor_GetPolicy_NonWindows(t *testing.T) {
	exec := NewPowerShellExecutor()
	ctx := context.Background()

	policy, err := exec.GetPolicy(ctx)
	if err == nil {
		t.Error("Expected error on non-Windows platform")
	}
	if policy != PolicyUndefined {
		t.Errorf("Expected Undefined policy on non-Windows, got %s", policy)
	}
}

func TestPowerShellExecutor_Execute_NonWindows(t *testing.T) {
	exec := NewPowerShellExecutor()
	ctx := context.Background()

	result, err := exec.Execute(ctx, "Write-Output 'Hello'")
	if err == nil {
		t.Error("Expected error on non-Windows platform")
	}
	if result != nil {
		t.Error("Expected nil result on non-Windows platform")
	}
}

func TestPowerShellExecutor_ExecuteFile_NonWindows(t *testing.T) {
	exec := NewPowerShellExecutor()
	ctx := context.Background()

	result, err := exec.ExecuteFile(ctx, "script.ps1")
	if err == nil {
		t.Error("Expected error on non-Windows platform")
	}
	if result != nil {
		t.Error("Expected nil result on non-Windows platform")
	}
}

func TestCmdExecutor_Execute_NonWindows(t *testing.T) {
	exec := NewCmdExecutor()
	ctx := context.Background()

	result, err := exec.Execute(ctx, "echo Hello")
	if err == nil {
		t.Error("Expected error on non-Windows platform")
	}
	if result != nil {
		t.Error("Expected nil result on non-Windows platform")
	}
}

func TestCmdExecutor_ExecuteBatch_NonWindows(t *testing.T) {
	exec := NewCmdExecutor()
	ctx := context.Background()

	result, err := exec.ExecuteBatch(ctx, "script.bat")
	if err == nil {
		t.Error("Expected error on non-Windows platform")
	}
	if result != nil {
		t.Error("Expected nil result on non-Windows platform")
	}
}

func TestResult_Success_NonWindows(t *testing.T) {
	tests := []struct {
		exitCode int
		expected bool
	}{
		{0, true},
		{1, false},
		{-1, false},
		{255, false},
	}

	for _, tt := range tests {
		result := &Result{ExitCode: tt.exitCode}
		if result.Success() != tt.expected {
			t.Errorf("Result{ExitCode: %d}.Success() = %v, want %v",
				tt.exitCode, result.Success(), tt.expected)
		}
	}
}

func TestResult_Output_NonWindows(t *testing.T) {
	tests := []struct {
		name     string
		stdout   string
		stderr   string
		expected string
	}{
		{
			name:     "stdout only",
			stdout:   "output",
			stderr:   "",
			expected: "output",
		},
		{
			name:     "stdout and stderr",
			stdout:   "output",
			stderr:   "error",
			expected: "output\nerror",
		},
		{
			name:     "empty",
			stdout:   "",
			stderr:   "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &Result{Stdout: tt.stdout, Stderr: tt.stderr}
			if result.Output() != tt.expected {
				t.Errorf("Output() = %q, want %q", result.Output(), tt.expected)
			}
		})
	}
}

func TestPolicy_Values_NonWindows(t *testing.T) {
	tests := []struct {
		policy   Policy
		expected string
	}{
		{PolicyRestricted, "Restricted"},
		{PolicyAllSigned, "AllSigned"},
		{PolicyRemoteSigned, "RemoteSigned"},
		{PolicyUnrestricted, "Unrestricted"},
		{PolicyBypass, "Bypass"},
		{PolicyUndefined, "Undefined"},
	}

	for _, tt := range tests {
		if string(tt.policy) != tt.expected {
			t.Errorf("Policy string = %q, want %q", string(tt.policy), tt.expected)
		}
	}
}

func TestPowerShellVersion_Fields_NonWindows(t *testing.T) {
	version := &PowerShellVersion{
		Major:       7,
		Minor:       4,
		Build:       0,
		Revision:    0,
		Edition:     "Core",
		Path:        "/usr/local/bin/pwsh",
		IsCore:      true,
		VersionText: "7.4.0",
	}

	if version.Major != 7 {
		t.Errorf("Major = %d, want 7", version.Major)
	}
	if version.Edition != "Core" {
		t.Errorf("Edition = %s, want Core", version.Edition)
	}
	if !version.IsCore {
		t.Error("IsCore should be true")
	}
}
