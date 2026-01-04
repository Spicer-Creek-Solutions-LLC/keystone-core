// Copyright 2024 Keystone Core Contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"runtime"
	"strings"
	"testing"
)

func TestServiceInstallCommand(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"service-install", "--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("service-install --help failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Install") {
		t.Errorf("expected help to contain 'Install', got: %s", output)
	}
	if !strings.Contains(output, "Windows") {
		t.Errorf("expected help to contain 'Windows', got: %s", output)
	}
}

func TestServiceInstallCommandOnNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"service-install"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error on non-Windows")
	}
	if !strings.Contains(err.Error(), "Windows") {
		t.Errorf("expected Windows-related error, got: %v", err)
	}
}

func TestServiceUninstallCommand(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"service-uninstall", "--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("service-uninstall --help failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Uninstall") {
		t.Errorf("expected help to contain 'Uninstall', got: %s", output)
	}
}

func TestServiceUninstallCommandOnNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"service-uninstall"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error on non-Windows")
	}
	if !strings.Contains(err.Error(), "Windows") {
		t.Errorf("expected Windows-related error, got: %v", err)
	}
}

func TestServiceStartCommand(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"service-start", "--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("service-start --help failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Start") {
		t.Errorf("expected help to contain 'Start', got: %s", output)
	}
}

func TestServiceStartCommandOnNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"service-start"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error on non-Windows")
	}
	if !strings.Contains(err.Error(), "Windows") {
		t.Errorf("expected Windows-related error, got: %v", err)
	}
}

func TestServiceStopCommand(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"service-stop", "--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("service-stop --help failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Stop") {
		t.Errorf("expected help to contain 'Stop', got: %s", output)
	}
}

func TestServiceStopCommandOnNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"service-stop"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error on non-Windows")
	}
	if !strings.Contains(err.Error(), "Windows") {
		t.Errorf("expected Windows-related error, got: %v", err)
	}
}

func TestServiceStatusCommand(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"service-status", "--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("service-status --help failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "status") {
		t.Errorf("expected help to contain 'status', got: %s", output)
	}
}

func TestServiceStatusCommandOnNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"service-status"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error on non-Windows")
	}
	if !strings.Contains(err.Error(), "Windows") {
		t.Errorf("expected Windows-related error, got: %v", err)
	}
}

func TestAllServiceCommandsExist(t *testing.T) {
	cmd := newRootCmd()

	expectedCommands := []string{
		"service-install",
		"service-uninstall",
		"service-start",
		"service-stop",
		"service-status",
	}

	for _, cmdName := range expectedCommands {
		found := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == cmdName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected command %s not found", cmdName)
		}
	}
}
