// Copyright 2024 Keystone Core Contributors
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// ServiceConfig contains configuration for Windows service installation
type ServiceConfig struct {
	// Name is the service name (must be unique)
	Name string

	// DisplayName is the display name shown in services.msc
	DisplayName string

	// Description is the service description
	Description string

	// BinaryPath is the path to the service executable
	BinaryPath string

	// Args are additional arguments to pass to the executable
	Args []string

	// StartType determines when the service starts
	// Possible values: mgr.StartAutomatic, mgr.StartManual, mgr.StartDisabled
	StartType uint32

	// ServiceAccount is the account to run the service as
	// Empty string means LocalSystem account
	ServiceAccount string

	// ServicePassword is the password for the service account
	// Only required for domain accounts
	ServicePassword string

	// Dependencies are other services that must start before this one
	Dependencies []string

	// DelayedAutoStart enables delayed start for automatic services
	DelayedAutoStart bool

	// RecoveryActions defines what happens when the service fails
	RecoveryActions []mgr.RecoveryAction

	// ResetPeriod is the time after which the failure count is reset (in seconds)
	ResetPeriod uint32

	// FailureCommand is a command to run when service fails
	FailureCommand string

	// RebootMessage is a message to display before rebooting (if reboot action is used)
	RebootMessage string
}

// DefaultServiceConfig returns the default service configuration
func DefaultServiceConfig() *ServiceConfig {
	exePath, _ := os.Executable()

	return &ServiceConfig{
		Name:        ServiceName,
		DisplayName: ServiceDisplayName,
		Description: ServiceDescription,
		BinaryPath:  exePath,
		StartType:   mgr.StartAutomatic,
		RecoveryActions: []mgr.RecoveryAction{
			{Type: mgr.ServiceRestart, Delay: 5 * time.Second},  // First failure: restart after 5s
			{Type: mgr.ServiceRestart, Delay: 30 * time.Second}, // Second failure: restart after 30s
			{Type: mgr.ServiceRestart, Delay: 60 * time.Second}, // Subsequent failures: restart after 60s
		},
		ResetPeriod:      86400, // Reset failure count after 24 hours
		DelayedAutoStart: true,  // Delay start to reduce boot time impact
	}
}

// ServiceInstaller handles Windows service installation and management
type ServiceInstaller struct {
	config *ServiceConfig
}

// NewServiceInstaller creates a new ServiceInstaller with the given config
func NewServiceInstaller(config *ServiceConfig) *ServiceInstaller {
	if config == nil {
		config = DefaultServiceConfig()
	}
	return &ServiceInstaller{config: config}
}

// Install installs the Windows service
func (i *ServiceInstaller) Install() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	// Check if service already exists
	s, err := m.OpenService(i.config.Name)
	if err == nil {
		s.Close()
		return fmt.Errorf("service %s already exists", i.config.Name)
	}

	// Build the binary path with arguments
	binaryPathName := i.config.BinaryPath
	if len(i.config.Args) > 0 {
		for _, arg := range i.config.Args {
			binaryPathName += " " + arg
		}
	}

	// Create the service
	svcConfig := mgr.Config{
		DisplayName:      i.config.DisplayName,
		Description:      i.config.Description,
		StartType:        i.config.StartType,
		ServiceStartName: i.config.ServiceAccount,
		Password:         i.config.ServicePassword,
		Dependencies:     i.config.Dependencies,
		DelayedAutoStart: i.config.DelayedAutoStart,
	}

	s, err = m.CreateService(i.config.Name, binaryPathName, svcConfig)
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}
	defer s.Close()

	// Configure recovery actions
	if len(i.config.RecoveryActions) > 0 {
		err = s.SetRecoveryActions(i.config.RecoveryActions, i.config.ResetPeriod)
		if err != nil {
			// Non-fatal error, service is still installed
			return fmt.Errorf("service installed but failed to set recovery actions: %w", err)
		}
	}

	// Configure failure command if specified
	if i.config.FailureCommand != "" {
		err = s.SetRecoveryCommand(i.config.FailureCommand)
		if err != nil {
			return fmt.Errorf("service installed but failed to set recovery command: %w", err)
		}
	}

	// Configure reboot message if specified
	if i.config.RebootMessage != "" {
		err = s.SetRebootMessage(i.config.RebootMessage)
		if err != nil {
			return fmt.Errorf("service installed but failed to set reboot message: %w", err)
		}
	}

	return nil
}

// Uninstall removes the Windows service
func (i *ServiceInstaller) Uninstall() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(i.config.Name)
	if err != nil {
		return fmt.Errorf("service %s not found: %w", i.config.Name, err)
	}
	defer s.Close()

	// Stop the service if running
	status, err := s.Query()
	if err != nil {
		return fmt.Errorf("failed to query service status: %w", err)
	}

	if status.State != svc.Stopped {
		_, err = s.Control(svc.Stop)
		if err != nil {
			return fmt.Errorf("failed to stop service: %w", err)
		}

		// Wait for service to stop
		timeout := time.Now().Add(30 * time.Second)
		for {
			status, err = s.Query()
			if err != nil {
				return fmt.Errorf("failed to query service status: %w", err)
			}
			if status.State == svc.Stopped {
				break
			}
			if time.Now().After(timeout) {
				return fmt.Errorf("timeout waiting for service to stop")
			}
			time.Sleep(500 * time.Millisecond)
		}
	}

	// Delete the service
	err = s.Delete()
	if err != nil {
		return fmt.Errorf("failed to delete service: %w", err)
	}

	return nil
}

// Start starts the Windows service
func (i *ServiceInstaller) Start() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(i.config.Name)
	if err != nil {
		return fmt.Errorf("service %s not found: %w", i.config.Name, err)
	}
	defer s.Close()

	err = s.Start()
	if err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	return nil
}

// Stop stops the Windows service
func (i *ServiceInstaller) Stop() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(i.config.Name)
	if err != nil {
		return fmt.Errorf("service %s not found: %w", i.config.Name, err)
	}
	defer s.Close()

	_, err = s.Control(svc.Stop)
	if err != nil {
		return fmt.Errorf("failed to stop service: %w", err)
	}

	return nil
}

// Restart restarts the Windows service
func (i *ServiceInstaller) Restart() error {
	if err := i.Stop(); err != nil {
		// Ignore stop errors if service is not running
	}

	// Wait a moment for the service to fully stop
	time.Sleep(2 * time.Second)

	return i.Start()
}

// Status returns the current service status
func (i *ServiceInstaller) Status() (*ServiceStatus, error) {
	m, err := mgr.Connect()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(i.config.Name)
	if err != nil {
		return nil, fmt.Errorf("service %s not found: %w", i.config.Name, err)
	}
	defer s.Close()

	status, err := s.Query()
	if err != nil {
		return nil, fmt.Errorf("failed to query service status: %w", err)
	}

	return &ServiceStatus{
		State:       stateToString(status.State),
		ProcessID:   status.ProcessId,
		ExitCode:    status.Win32ExitCode,
		Checkpoint:  status.CheckPoint,
		WaitHint:    status.WaitHint,
		Accepts:     uint32(status.Accepts),
		IsRunning:   status.State == svc.Running,
		IsPaused:    status.State == svc.Paused,
		IsStopped:   status.State == svc.Stopped,
		IsPending:   status.State == svc.StartPending || status.State == svc.StopPending || status.State == svc.PausePending || status.State == svc.ContinuePending,
		Description: StateDescription(status.State),
	}, nil
}

// Exists checks if the service exists
func (i *ServiceInstaller) Exists() bool {
	m, err := mgr.Connect()
	if err != nil {
		return false
	}
	defer m.Disconnect()

	s, err := m.OpenService(i.config.Name)
	if err != nil {
		return false
	}
	s.Close()
	return true
}

// UpdateConfig updates the service configuration
func (i *ServiceInstaller) UpdateConfig(config *ServiceConfig) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(i.config.Name)
	if err != nil {
		return fmt.Errorf("service %s not found: %w", i.config.Name, err)
	}
	defer s.Close()

	// Get current config
	currentConfig, err := s.Config()
	if err != nil {
		return fmt.Errorf("failed to get service config: %w", err)
	}

	// Update fields that are specified
	if config.DisplayName != "" {
		currentConfig.DisplayName = config.DisplayName
	}
	if config.Description != "" {
		currentConfig.Description = config.Description
	}
	if config.StartType != 0 {
		currentConfig.StartType = config.StartType
	}
	if config.ServiceAccount != "" {
		currentConfig.ServiceStartName = config.ServiceAccount
		currentConfig.Password = config.ServicePassword
	}
	if len(config.Dependencies) > 0 {
		currentConfig.Dependencies = config.Dependencies
	}

	err = s.UpdateConfig(currentConfig)
	if err != nil {
		return fmt.Errorf("failed to update service config: %w", err)
	}

	// Update recovery actions if specified
	if len(config.RecoveryActions) > 0 {
		err = s.SetRecoveryActions(config.RecoveryActions, config.ResetPeriod)
		if err != nil {
			return fmt.Errorf("failed to update recovery actions: %w", err)
		}
	}

	// Update the internal config
	i.config = config

	return nil
}

// DefaultConfigPath returns the default configuration file path for Windows
func DefaultConfigPath() string {
	programData := os.Getenv("ProgramData")
	if programData == "" {
		programData = `C:\ProgramData`
	}
	return filepath.Join(programData, "kscore", "agent.yaml")
}

// DefaultLogPath returns the default log directory for Windows
func DefaultLogPath() string {
	programData := os.Getenv("ProgramData")
	if programData == "" {
		programData = `C:\ProgramData`
	}
	return filepath.Join(programData, "kscore", "logs")
}

// EnsureDirectories creates the required directories for the agent
func EnsureDirectories() error {
	dirs := []string{
		filepath.Dir(DefaultConfigPath()),
		DefaultLogPath(),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}
