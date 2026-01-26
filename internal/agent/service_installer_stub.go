// Copyright 2024 Keystone Core Contributors
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package agent

import (
	"path/filepath"
)

// ServiceConfig is a stub for non-Windows platforms
type ServiceConfig struct {
	Name             string
	DisplayName      string
	Description      string
	BinaryPath       string
	Args             []string
	StartType        uint32
	ServiceAccount   string
	ServicePassword  string
	Dependencies     []string
	DelayedAutoStart bool
}

// ServiceStatus is a stub for non-Windows platforms
type ServiceStatus struct {
	State       string
	ProcessID   uint32
	ExitCode    uint32
	Checkpoint  uint32
	WaitHint    uint32
	Accepts     uint32
	IsRunning   bool
	IsPaused    bool
	IsStopped   bool
	IsPending   bool
	Description string
}

// DefaultServiceConfig returns a stub configuration for non-Windows
func DefaultServiceConfig() *ServiceConfig {
	return &ServiceConfig{}
}

// ServiceInstaller is a stub for non-Windows platforms
type ServiceInstaller struct {
	config *ServiceConfig
}

// NewServiceInstaller creates a stub ServiceInstaller for non-Windows
func NewServiceInstaller(config *ServiceConfig) *ServiceInstaller {
	return &ServiceInstaller{config: config}
}

// Install is not available on non-Windows platforms
func (i *ServiceInstaller) Install() error {
	return ErrNotWindows
}

// Uninstall is not available on non-Windows platforms
func (i *ServiceInstaller) Uninstall() error {
	return ErrNotWindows
}

// Start is not available on non-Windows platforms
func (i *ServiceInstaller) Start() error {
	return ErrNotWindows
}

// Stop is not available on non-Windows platforms
func (i *ServiceInstaller) Stop() error {
	return ErrNotWindows
}

// Restart is not available on non-Windows platforms
func (i *ServiceInstaller) Restart() error {
	return ErrNotWindows
}

// Status is not available on non-Windows platforms
func (i *ServiceInstaller) Status() (*ServiceStatus, error) {
	return nil, ErrNotWindows
}

// Exists returns false on non-Windows platforms
func (i *ServiceInstaller) Exists() bool {
	return false
}

// UpdateConfig is not available on non-Windows platforms
func (i *ServiceInstaller) UpdateConfig(config *ServiceConfig) error {
	return ErrNotWindows
}

// DefaultConfigPath returns the default configuration file path
func DefaultConfigPath() string {
	return filepath.Join("/etc", "kscore", "agent.yaml")
}

// DefaultLogPath returns the default log directory
func DefaultLogPath() string {
	return filepath.Join("/var", "log", "kscore")
}

// EnsureDirectories is a no-op on non-Windows
func EnsureDirectories() error {
	return nil
}
