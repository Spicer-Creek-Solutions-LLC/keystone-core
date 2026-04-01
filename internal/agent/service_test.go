// Copyright 2024 Spicer Creek Solutions LLC
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"errors"
	"runtime"
	"testing"
)

func TestIsWindowsService(t *testing.T) {
	result := IsWindowsService()

	if runtime.GOOS == "windows" {
		// On Windows, this could be true or false depending on how the test is run
		// When run from command line (not as service), it should be false
		// We can't really test the true case without actually running as a service
		t.Logf("IsWindowsService returned: %v (expected false when run from command line)", result)
	} else {
		// On non-Windows, should always return false
		if result {
			t.Error("IsWindowsService should return false on non-Windows platforms")
		}
	}
}

func TestRunAsService(t *testing.T) {
	if runtime.GOOS != "windows" {
		// On non-Windows, should return ErrNotWindows
		err := RunAsService(nil)
		if !errors.Is(err, ErrNotWindows) {
			t.Errorf("expected ErrNotWindows, got %v", err)
		}
	}
	// We can't test RunAsService on Windows without actually running as a service
}

func TestDefaultServiceConfig(t *testing.T) {
	config := DefaultServiceConfig()
	if config == nil {
		t.Fatal("DefaultServiceConfig should not return nil")
	}

	if runtime.GOOS == "windows" {
		if config.Name == "" {
			t.Error("expected Name to be set on Windows")
		}
		if config.DisplayName == "" {
			t.Error("expected DisplayName to be set on Windows")
		}
		if config.Description == "" {
			t.Error("expected Description to be set on Windows")
		}
	}
}

func TestNewServiceInstaller(t *testing.T) {
	installer := NewServiceInstaller(nil)
	if installer == nil {
		t.Fatal("NewServiceInstaller should not return nil")
	}
}

func TestServiceInstallerExists(t *testing.T) {
	installer := NewServiceInstaller(nil)
	exists := installer.Exists()

	if runtime.GOOS != "windows" {
		if exists {
			t.Error("Exists should return false on non-Windows platforms")
		}
	}
	// On Windows, it could be true or false depending on whether the service is installed
}

func TestServiceInstallerInstallOnNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	installer := NewServiceInstaller(nil)
	err := installer.Install()
	if !errors.Is(err, ErrNotWindows) {
		t.Errorf("expected ErrNotWindows on non-Windows, got %v", err)
	}
}

func TestServiceInstallerUninstallOnNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	installer := NewServiceInstaller(nil)
	err := installer.Uninstall()
	if !errors.Is(err, ErrNotWindows) {
		t.Errorf("expected ErrNotWindows on non-Windows, got %v", err)
	}
}

func TestServiceInstallerStartOnNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	installer := NewServiceInstaller(nil)
	err := installer.Start()
	if !errors.Is(err, ErrNotWindows) {
		t.Errorf("expected ErrNotWindows on non-Windows, got %v", err)
	}
}

func TestServiceInstallerStopOnNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	installer := NewServiceInstaller(nil)
	err := installer.Stop()
	if !errors.Is(err, ErrNotWindows) {
		t.Errorf("expected ErrNotWindows on non-Windows, got %v", err)
	}
}

func TestServiceInstallerRestartOnNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	installer := NewServiceInstaller(nil)
	err := installer.Restart()
	if !errors.Is(err, ErrNotWindows) {
		t.Errorf("expected ErrNotWindows on non-Windows, got %v", err)
	}
}

func TestServiceInstallerStatusOnNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	installer := NewServiceInstaller(nil)
	status, err := installer.Status()
	if !errors.Is(err, ErrNotWindows) {
		t.Errorf("expected ErrNotWindows on non-Windows, got %v", err)
	}
	if status != nil {
		t.Error("expected nil status on non-Windows")
	}
}

func TestServiceInstallerUpdateConfigOnNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	installer := NewServiceInstaller(nil)
	err := installer.UpdateConfig(&ServiceConfig{})
	if !errors.Is(err, ErrNotWindows) {
		t.Errorf("expected ErrNotWindows on non-Windows, got %v", err)
	}
}

func TestDefaultConfigPath(t *testing.T) {
	path := DefaultConfigPath()
	if path == "" {
		t.Error("DefaultConfigPath should not return empty string")
	}

	if runtime.GOOS == "windows" {
		// Should contain ProgramData
		if path == "" {
			t.Error("expected Windows path on Windows")
		}
	} else {
		// Should be /etc/kscore/agent.yaml on Unix
		expected := "/etc/kscore/agent.yaml"
		if path != expected {
			t.Errorf("expected %s, got %s", expected, path)
		}
	}
}

func TestDefaultLogPath(t *testing.T) {
	path := DefaultLogPath()
	if path == "" {
		t.Error("DefaultLogPath should not return empty string")
	}

	if runtime.GOOS != "windows" {
		expected := "/var/log/kscore"
		if path != expected {
			t.Errorf("expected %s, got %s", expected, path)
		}
	}
}

func TestEnsureDirectories(t *testing.T) {
	err := EnsureDirectories()
	if runtime.GOOS != "windows" {
		// On non-Windows, this is a no-op
		if err != nil {
			t.Errorf("EnsureDirectories should not return error on non-Windows: %v", err)
		}
	}
	// On Windows, we can't test this without elevated privileges
}

func TestServiceStatusFields(t *testing.T) {
	status := &ServiceStatus{
		State:       "Running",
		ProcessID:   1234,
		ExitCode:    0,
		Checkpoint:  0,
		WaitHint:    0,
		Accepts:     0,
		IsRunning:   true,
		IsPaused:    false,
		IsStopped:   false,
		IsPending:   false,
		Description: "The service is running",
	}

	if status.State != "Running" {
		t.Errorf("expected State 'Running', got '%s'", status.State)
	}
	if !status.IsRunning {
		t.Error("expected IsRunning to be true")
	}
	if status.IsPaused {
		t.Error("expected IsPaused to be false")
	}
	if status.IsStopped {
		t.Error("expected IsStopped to be false")
	}
	if status.ProcessID != 1234 {
		t.Errorf("expected ProcessID 1234, got %d", status.ProcessID)
	}
}

func TestServiceConfigFields(t *testing.T) {
	config := &ServiceConfig{
		Name:             "test-service",
		DisplayName:      "Test Service",
		Description:      "A test service",
		BinaryPath:       "/usr/bin/test",
		Args:             []string{"--config", "/etc/test.yaml"},
		StartType:        2, // Automatic
		ServiceAccount:   "LocalSystem",
		Dependencies:     []string{"nats"},
		DelayedAutoStart: true,
	}

	if config.Name != "test-service" {
		t.Errorf("expected Name 'test-service', got '%s'", config.Name)
	}
	if config.DisplayName != "Test Service" {
		t.Errorf("expected DisplayName 'Test Service', got '%s'", config.DisplayName)
	}
	if len(config.Args) != 2 {
		t.Errorf("expected 2 args, got %d", len(config.Args))
	}
	if !config.DelayedAutoStart {
		t.Error("expected DelayedAutoStart to be true")
	}
}
