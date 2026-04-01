// Copyright 2026 Spicer Creek Solutions LLC
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package statemgmt

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// WinServiceModule implements Windows service management using native APIs
type WinServiceModule struct {
	*BaseModule
}

// NewWinServiceModule creates a new Windows service module
func NewWinServiceModule() *WinServiceModule {
	return &WinServiceModule{
		BaseModule: NewBaseModule("win_service", []string{
			"running", "stopped", "enabled", "disabled",
			"present", "absent",
		}),
	}
}

// Windows service start type constants (matching mgr package)
const (
	serviceStartBoot     uint32 = 0 // Boot (drivers only)
	serviceStartSystem   uint32 = 1 // System (drivers only)
	serviceStartAuto     uint32 = 2 // Automatic
	serviceStartDemand   uint32 = 3 // Manual
	serviceStartDisabled uint32 = 4 // Disabled
)

// Check checks the current state of a Windows service
func (m *WinServiceModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	serviceName := decl.ID

	// Connect to service manager
	manager, err := mgr.Connect()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer manager.Disconnect()

	// Open the service
	service, err := manager.OpenService(serviceName)
	if err != nil {
		// Service doesn't exist
		result.Present = false
		result.CurrentState = "absent"

		if decl.State == "absent" {
			result.Matches = true
		} else {
			result.Matches = false
			result.Diff["state"] = map[string]string{"current": "absent", "desired": decl.State}
		}
		return result, nil
	}
	defer service.Close()

	result.Present = true

	// Get service status
	status, err := service.Query()
	if err != nil {
		return nil, fmt.Errorf("failed to query service status: %w", err)
	}

	// Get service config
	config, err := service.Config()
	if err != nil {
		return nil, fmt.Errorf("failed to get service config: %w", err)
	}

	// Determine current state
	running := status.State == svc.Running
	if running {
		result.CurrentState = "running"
	} else {
		result.CurrentState = "stopped"
	}

	// Determine if enabled
	enabled := config.StartType == mgr.StartAutomatic
	delayed := config.DelayedAutoStart

	// Store metadata
	result.Metadata["running"] = running
	result.Metadata["enabled"] = enabled
	result.Metadata["delayed_start"] = delayed
	result.Metadata["start_type"] = startTypeToString(config.StartType)
	result.Metadata["display_name"] = config.DisplayName
	result.Metadata["description"] = config.Description
	result.Metadata["account"] = config.ServiceStartName
	result.Metadata["state"] = stateToString(status.State)
	result.Metadata["pid"] = status.ProcessId

	// Get recovery options
	recoveryActions, err := service.RecoveryActions()
	if err == nil && len(recoveryActions) > 0 {
		result.Metadata["recovery_actions"] = formatRecoveryActions(recoveryActions)
	}

	// Compare with desired state
	switch decl.State {
	case "running":
		result.Matches = running
		if !running {
			result.Diff["state"] = map[string]string{"current": "stopped", "desired": "running"}
		}
		// Check enable parameter
		if enableParam := getBoolParameter(decl, "enable", false); enableParam && !enabled {
			result.Matches = false
			result.Diff["enabled"] = map[string]bool{"current": false, "desired": true}
		}
		// Check delayed_start parameter
		if delayedParam := getBoolParameter(decl, "delayed_start", false); delayedParam != delayed {
			result.Matches = false
			result.Diff["delayed_start"] = map[string]bool{"current": delayed, "desired": delayedParam}
		}

	case "stopped":
		result.Matches = !running
		if running {
			result.Diff["state"] = map[string]string{"current": "running", "desired": "stopped"}
		}

	case "enabled":
		result.Matches = enabled
		if !enabled {
			result.Diff["start_type"] = map[string]string{"current": startTypeToString(config.StartType), "desired": "automatic"}
		}

	case "disabled":
		result.Matches = config.StartType == mgr.StartDisabled
		if config.StartType != mgr.StartDisabled {
			result.Diff["start_type"] = map[string]string{"current": startTypeToString(config.StartType), "desired": "disabled"}
		}

	case "present":
		result.Matches = true // Service exists

	case "absent":
		result.Matches = false
		result.Diff["state"] = map[string]string{"current": "present", "desired": "absent"}
	}

	// Check additional parameters
	if desiredStartType := getStringParameter(decl, "start_type", ""); desiredStartType != "" {
		currentStartType := startTypeToString(config.StartType)
		if currentStartType != desiredStartType {
			result.Matches = false
			result.Diff["start_type"] = map[string]string{"current": currentStartType, "desired": desiredStartType}
		}
	}

	if desiredAccount := getStringParameter(decl, "account", ""); desiredAccount != "" {
		if config.ServiceStartName != desiredAccount {
			result.Matches = false
			result.Diff["account"] = map[string]string{"current": config.ServiceStartName, "desired": desiredAccount}
		}
	}

	return result, nil
}

// Apply applies the Windows service state
func (m *WinServiceModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	result := &StateResult{
		StateID:   decl.ID,
		Module:    m.Name(),
		Success:   false,
		Changed:   false,
		Changes:   make(map[string]interface{}),
		StartTime: startTime,
	}

	// Check current state
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to check current state: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// If already in desired state, no changes needed
	if checkResult.Matches {
		result.Success = true
		result.Changed = false
		result.Comment = "Already in desired state"
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// Connect to service manager
	manager, err := mgr.Connect()
	if err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to connect to service manager: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}
	defer manager.Disconnect()

	// Apply changes based on state
	var applyErr error
	switch decl.State {
	case "running":
		applyErr = m.applyRunning(ctx, manager, decl, result)
	case "stopped":
		applyErr = m.applyStopped(ctx, manager, decl, result)
	case "enabled":
		applyErr = m.applyEnabled(ctx, manager, decl, result)
	case "disabled":
		applyErr = m.applyDisabled(ctx, manager, decl, result)
	case "present":
		applyErr = m.applyPresent(ctx, manager, decl, result)
	case "absent":
		applyErr = m.applyAbsent(ctx, manager, decl, result)
	default:
		applyErr = fmt.Errorf("unsupported state: %s", decl.State)
	}

	if applyErr != nil {
		result.Error = applyErr
		result.Success = false
		result.Comment = fmt.Sprintf("Failed to apply state: %v", applyErr)
	} else {
		result.Success = true
		result.Changed = true
		result.Changes = checkResult.Diff
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	return result, nil
}

// Test tests if the service is in the desired state
func (m *WinServiceModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil
}

// applyRunning starts the service and optionally enables it
func (m *WinServiceModule) applyRunning(ctx context.Context, manager *mgr.Mgr, decl *StateDeclaration, result *StateResult) error {
	serviceName := decl.ID

	service, err := manager.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("failed to open service: %w", err)
	}
	defer service.Close()

	// Apply configuration changes first
	if err := m.applyConfigChanges(service, decl); err != nil {
		return fmt.Errorf("failed to apply config changes: %w", err)
	}

	// Check if already running
	status, err := service.Query()
	if err != nil {
		return fmt.Errorf("failed to query service: %w", err)
	}

	if status.State != svc.Running {
		// Start the service
		if err := service.Start(); err != nil {
			return fmt.Errorf("failed to start service: %w", err)
		}

		// Wait for service to start
		if err := m.waitForState(service, svc.Running, 30*time.Second); err != nil {
			return fmt.Errorf("service did not start in time: %w", err)
		}

		result.Comment = fmt.Sprintf("Service %s started", serviceName)
	}

	return nil
}

// applyStopped stops the service
func (m *WinServiceModule) applyStopped(ctx context.Context, manager *mgr.Mgr, decl *StateDeclaration, result *StateResult) error {
	serviceName := decl.ID

	service, err := manager.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("failed to open service: %w", err)
	}
	defer service.Close()

	// Check if already stopped
	status, err := service.Query()
	if err != nil {
		return fmt.Errorf("failed to query service: %w", err)
	}

	if status.State != svc.Stopped {
		// Stop the service
		_, err := service.Control(svc.Stop)
		if err != nil {
			return fmt.Errorf("failed to stop service: %w", err)
		}

		// Wait for service to stop
		if err := m.waitForState(service, svc.Stopped, 30*time.Second); err != nil {
			return fmt.Errorf("service did not stop in time: %w", err)
		}

		result.Comment = fmt.Sprintf("Service %s stopped", serviceName)
	}

	return nil
}

// applyEnabled enables the service (sets start type to automatic)
func (m *WinServiceModule) applyEnabled(ctx context.Context, manager *mgr.Mgr, decl *StateDeclaration, result *StateResult) error {
	serviceName := decl.ID

	service, err := manager.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("failed to open service: %w", err)
	}
	defer service.Close()

	config, err := service.Config()
	if err != nil {
		return fmt.Errorf("failed to get service config: %w", err)
	}

	config.StartType = mgr.StartAutomatic
	if getBoolParameter(decl, "delayed_start", false) {
		config.DelayedAutoStart = true
	}

	if err := service.UpdateConfig(config); err != nil {
		return fmt.Errorf("failed to update service config: %w", err)
	}

	result.Comment = fmt.Sprintf("Service %s enabled", serviceName)
	return nil
}

// applyDisabled disables the service
func (m *WinServiceModule) applyDisabled(ctx context.Context, manager *mgr.Mgr, decl *StateDeclaration, result *StateResult) error {
	serviceName := decl.ID

	service, err := manager.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("failed to open service: %w", err)
	}
	defer service.Close()

	config, err := service.Config()
	if err != nil {
		return fmt.Errorf("failed to get service config: %w", err)
	}

	config.StartType = mgr.StartDisabled

	if err := service.UpdateConfig(config); err != nil {
		return fmt.Errorf("failed to update service config: %w", err)
	}

	result.Comment = fmt.Sprintf("Service %s disabled", serviceName)
	return nil
}

// applyPresent ensures the service exists (for documentation only - service creation requires binary path)
func (m *WinServiceModule) applyPresent(ctx context.Context, manager *mgr.Mgr, decl *StateDeclaration, result *StateResult) error {
	svcName := decl.ID

	// Check if service exists
	service, err := manager.OpenService(svcName)
	if err == nil {
		service.Close()
		// Apply configuration changes if specified
		service, _ = manager.OpenService(svcName)
		defer service.Close()
		if err := m.applyConfigChanges(service, decl); err != nil {
			return err
		}
		result.Comment = fmt.Sprintf("Service %s already exists", svcName)
		return nil
	}

	// Service doesn't exist - create it if binary_path is provided
	binaryPath := getStringParameter(decl, "binary_path", "")
	if binaryPath == "" {
		return fmt.Errorf("service %s does not exist and binary_path not specified", svcName)
	}

	displayName := getStringParameter(decl, "display_name", svcName)
	description := getStringParameter(decl, "description", "")

	// Determine start type
	var startType uint32 = mgr.StartManual
	if startTypeStr := getStringParameter(decl, "start_type", "manual"); startTypeStr != "" {
		startType = parseStartType(startTypeStr)
	}

	config := mgr.Config{
		DisplayName: displayName,
		Description: description,
		StartType:   startType,
	}

	// Handle dependencies
	if deps := getStringSliceParameter(decl, "dependencies"); len(deps) > 0 {
		config.Dependencies = deps
	}

	// Handle service account
	if account := getStringParameter(decl, "account", ""); account != "" {
		config.ServiceStartName = account
		if password := getStringParameter(decl, "password", ""); password != "" {
			config.Password = password
		}
	}

	// Create the service
	service, err = manager.CreateService(svcName, binaryPath, config)
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}
	defer service.Close()

	// Apply recovery options if specified
	if err := m.applyRecoveryOptions(service, decl); err != nil {
		return fmt.Errorf("failed to apply recovery options: %w", err)
	}

	result.Comment = fmt.Sprintf("Service %s created", svcName)
	return nil
}

// applyAbsent removes the service
func (m *WinServiceModule) applyAbsent(ctx context.Context, manager *mgr.Mgr, decl *StateDeclaration, result *StateResult) error {
	serviceName := decl.ID

	service, err := manager.OpenService(serviceName)
	if err != nil {
		// Service doesn't exist
		result.Comment = fmt.Sprintf("Service %s already absent", serviceName)
		return nil
	}
	defer service.Close()

	// Stop service if running
	status, _ := service.Query()
	if status.State != svc.Stopped {
		service.Control(svc.Stop)
		m.waitForState(service, svc.Stopped, 30*time.Second)
	}

	// Delete the service
	if err := service.Delete(); err != nil {
		return fmt.Errorf("failed to delete service: %w", err)
	}

	result.Comment = fmt.Sprintf("Service %s removed", serviceName)
	return nil
}

// applyConfigChanges applies configuration changes to an existing service
func (m *WinServiceModule) applyConfigChanges(service *mgr.Service, decl *StateDeclaration) error {
	config, err := service.Config()
	if err != nil {
		return err
	}

	changed := false

	// Update display name if specified
	if displayName := getStringParameter(decl, "display_name", ""); displayName != "" && displayName != config.DisplayName {
		config.DisplayName = displayName
		changed = true
	}

	// Update description if specified
	if description := getStringParameter(decl, "description", ""); description != "" && description != config.Description {
		config.Description = description
		changed = true
	}

	// Update start type if specified
	if startTypeStr := getStringParameter(decl, "start_type", ""); startTypeStr != "" {
		startType := parseStartType(startTypeStr)
		if startType != config.StartType {
			config.StartType = startType
			changed = true
		}
	}

	// Update delayed start if specified
	if decl.State == "running" || decl.State == "enabled" {
		delayedStart := getBoolParameter(decl, "delayed_start", false)
		if delayedStart != config.DelayedAutoStart {
			config.DelayedAutoStart = delayedStart
			changed = true
		}
	}

	// Update service account if specified
	if account := getStringParameter(decl, "account", ""); account != "" && account != config.ServiceStartName {
		config.ServiceStartName = account
		if password := getStringParameter(decl, "password", ""); password != "" {
			config.Password = password
		}
		changed = true
	}

	if changed {
		if err := service.UpdateConfig(config); err != nil {
			return fmt.Errorf("failed to update config: %w", err)
		}
	}

	// Apply recovery options if specified
	if err := m.applyRecoveryOptions(service, decl); err != nil {
		return err
	}

	return nil
}

// applyRecoveryOptions applies recovery options to a service
func (m *WinServiceModule) applyRecoveryOptions(service *mgr.Service, decl *StateDeclaration) error {
	// Check if recovery options are specified
	if !hasParameter(decl, "restart_on_failure") && !hasParameter(decl, "failure_reset_period") {
		return nil
	}

	var actions []mgr.RecoveryAction

	if getBoolParameter(decl, "restart_on_failure", false) {
		// Default recovery: restart after 5s, 30s, 60s
		firstDelay := getIntParameter(decl, "first_failure_delay", 5)
		secondDelay := getIntParameter(decl, "second_failure_delay", 30)
		thirdDelay := getIntParameter(decl, "subsequent_failure_delay", 60)

		actions = []mgr.RecoveryAction{
			{Type: mgr.ServiceRestart, Delay: time.Duration(firstDelay) * time.Second},
			{Type: mgr.ServiceRestart, Delay: time.Duration(secondDelay) * time.Second},
			{Type: mgr.ServiceRestart, Delay: time.Duration(thirdDelay) * time.Second},
		}
	}

	if len(actions) > 0 {
		resetPeriod := uint32(getIntParameter(decl, "failure_reset_period", 86400)) // Default: 24 hours
		if err := service.SetRecoveryActions(actions, resetPeriod); err != nil {
			return fmt.Errorf("failed to set recovery actions: %w", err)
		}
	}

	return nil
}

// waitForState waits for the service to reach the desired state
type serviceQuerier interface {
	Query() (svc.Status, error)
}

func (m *WinServiceModule) waitForState(service serviceQuerier, desiredState svc.State, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer timer.Stop()
	defer ticker.Stop()

	for {
		status, err := service.Query()
		if err != nil {
			return err
		}
		if status.State == desiredState {
			return nil
		}

		select {
		case <-timer.C:
			return fmt.Errorf("timeout waiting for state %v", desiredState)
		case <-ticker.C:
		}
	}
}

// Helper functions

func startTypeToString(startType uint32) string {
	switch startType {
	case mgr.StartAutomatic:
		return "automatic"
	case mgr.StartManual:
		return "manual"
	case mgr.StartDisabled:
		return "disabled"
	case serviceStartBoot:
		return "boot"
	case serviceStartSystem:
		return "system"
	default:
		return fmt.Sprintf("unknown(%d)", startType)
	}
}

func parseStartType(s string) uint32 {
	switch s {
	case "automatic", "auto":
		return mgr.StartAutomatic
	case "manual", "demand":
		return mgr.StartManual
	case "disabled":
		return mgr.StartDisabled
	case "boot":
		return serviceStartBoot
	case "system":
		return serviceStartSystem
	default:
		return mgr.StartManual
	}
}

func stateToString(state svc.State) string {
	switch state {
	case svc.Stopped:
		return "stopped"
	case svc.StartPending:
		return "start_pending"
	case svc.StopPending:
		return "stop_pending"
	case svc.Running:
		return "running"
	case svc.ContinuePending:
		return "continue_pending"
	case svc.PausePending:
		return "pause_pending"
	case svc.Paused:
		return "paused"
	default:
		return fmt.Sprintf("unknown(%d)", state)
	}
}

func formatRecoveryActions(actions []mgr.RecoveryAction) []map[string]interface{} {
	result := make([]map[string]interface{}, len(actions))
	for i, action := range actions {
		result[i] = map[string]interface{}{
			"type":  recoveryActionToString(int(action.Type)),
			"delay": action.Delay.String(),
		}
	}
	return result
}

func recoveryActionToString(action int) string {
	switch action {
	case mgr.NoAction:
		return "none"
	case mgr.ServiceRestart:
		return "restart"
	case mgr.ComputerReboot:
		return "reboot"
	case mgr.RunCommand:
		return "run_command"
	default:
		return fmt.Sprintf("unknown(%d)", action)
	}
}

func init() {
	RegisterModule(NewWinServiceModule())
}
