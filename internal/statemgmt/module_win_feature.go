// Copyright 2026 Spicer Creek Solutions LLC
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package statemgmt

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// WinFeatureModule implements Windows Features/Roles management using PowerShell
type WinFeatureModule struct {
	*BaseModule
}

// NewWinFeatureModule creates a new Windows feature module
func NewWinFeatureModule() *WinFeatureModule {
	return &WinFeatureModule{
		BaseModule: NewBaseModule("win_feature", []string{
			"installed", "removed", "enabled", "disabled",
		}),
	}
}

// WindowsFeature represents a Windows Feature
type WindowsFeature struct {
	Name        string `json:"Name"`
	DisplayName string `json:"DisplayName"`
	Description string `json:"Description"`
	State       string `json:"State"`
	FeatureType string `json:"FeatureType"`
	Installed   bool   `json:"Installed"`
}

// Check checks the current state of a Windows Feature
func (m *WinFeatureModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	featureName := getStringParameter(decl, "name", decl.ID)

	// Try to get the feature (works for both client and server)
	feature, err := m.getWindowsFeature(ctx, featureName)
	if err != nil {
		return nil, fmt.Errorf("failed to get feature: %w", err)
	}

	if feature == nil {
		// Feature not found - try optional feature
		optFeature, optErr := m.getOptionalFeature(ctx, featureName)
		if optErr != nil {
			return nil, fmt.Errorf("failed to get optional feature: %w", optErr)
		}

		if optFeature == nil {
			// Feature doesn't exist
			result.Present = false
			result.CurrentState = "absent"
			result.Metadata["error"] = fmt.Sprintf("Feature %s not found", featureName)

			// Can't install/enable a non-existent feature
			if decl.State == "removed" || decl.State == "disabled" {
				result.Matches = true
			} else {
				result.Matches = false
				result.Diff["state"] = map[string]string{"current": "absent", "desired": decl.State}
			}
			return result, nil
		}

		// Use optional feature data
		feature = optFeature
	}

	result.Present = true
	result.Metadata["name"] = feature.Name
	result.Metadata["display_name"] = feature.DisplayName
	result.Metadata["description"] = feature.Description
	result.Metadata["state"] = feature.State
	result.Metadata["feature_type"] = feature.FeatureType
	result.Metadata["installed"] = feature.Installed

	// Determine current state
	if feature.Installed {
		result.CurrentState = "installed"
	} else {
		result.CurrentState = "removed"
	}

	// For optional features, state can be Enabled/Disabled
	switch strings.ToLower(feature.State) {
	case "enabled":
		result.CurrentState = "enabled"
	case "disabled":
		result.CurrentState = "disabled"
	}

	// Compare with desired state
	switch decl.State {
	case "installed", "enabled":
		if feature.Installed || strings.EqualFold(feature.State, "enabled") {
			result.Matches = true
		} else {
			result.Matches = false
			result.Diff["state"] = map[string]string{"current": result.CurrentState, "desired": decl.State}
		}

	case "removed", "disabled":
		if !feature.Installed || strings.EqualFold(feature.State, "disabled") {
			result.Matches = true
		} else {
			result.Matches = false
			result.Diff["state"] = map[string]string{"current": result.CurrentState, "desired": decl.State}
		}
	}

	return result, nil
}

// Apply applies the Windows Feature state
func (m *WinFeatureModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
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

	featureName := getStringParameter(decl, "name", decl.ID)

	var applyErr error
	switch decl.State {
	case "installed", "enabled":
		applyErr = m.installFeature(ctx, featureName, decl, result)
	case "removed", "disabled":
		applyErr = m.removeFeature(ctx, featureName, decl, result)
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

// Test tests if the feature is in the desired state
func (m *WinFeatureModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil
}

// getWindowsFeature retrieves a Windows Server feature
func (m *WinFeatureModule) getWindowsFeature(ctx context.Context, name string) (*WindowsFeature, error) {
	psScript := fmt.Sprintf(`
$feature = Get-WindowsFeature -Name '%s' -ErrorAction SilentlyContinue
if ($null -eq $feature) {
    Write-Output 'null'
} else {
    $result = @{
        Name = $feature.Name
        DisplayName = $feature.DisplayName
        Description = $feature.Description
        State = $feature.InstallState.ToString()
        FeatureType = 'ServerFeature'
        Installed = $feature.Installed
    }
    $result | ConvertTo-Json -Compress
}
`, escapeForPowerShellQuote(name))

	output, err := m.runPowerShellFeature(ctx, psScript)
	if err != nil {
		// Get-WindowsFeature not available on client Windows
		if strings.Contains(err.Error(), "not recognized") || strings.Contains(err.Error(), "CommandNotFoundException") {
			return nil, nil
		}
		return nil, err
	}

	output = strings.TrimSpace(output)
	if output == "null" || output == "" {
		return nil, nil
	}

	var feature WindowsFeature
	if err := json.Unmarshal([]byte(output), &feature); err != nil {
		return nil, fmt.Errorf("failed to parse feature: %w", err)
	}

	return &feature, nil
}

// getOptionalFeature retrieves a Windows Optional Feature (client Windows)
func (m *WinFeatureModule) getOptionalFeature(ctx context.Context, name string) (*WindowsFeature, error) {
	psScript := fmt.Sprintf(`
$feature = Get-WindowsOptionalFeature -Online -FeatureName '%s' -ErrorAction SilentlyContinue
if ($null -eq $feature) {
    Write-Output 'null'
} else {
    $result = @{
        Name = $feature.FeatureName
        DisplayName = $feature.FeatureName
        Description = $feature.Description
        State = $feature.State.ToString()
        FeatureType = 'OptionalFeature'
        Installed = ($feature.State -eq 'Enabled')
    }
    $result | ConvertTo-Json -Compress
}
`, escapeForPowerShellQuote(name))

	output, err := m.runPowerShellFeature(ctx, psScript)
	if err != nil {
		return nil, err
	}

	output = strings.TrimSpace(output)
	if output == "null" || output == "" {
		return nil, nil
	}

	var feature WindowsFeature
	if err := json.Unmarshal([]byte(output), &feature); err != nil {
		return nil, fmt.Errorf("failed to parse optional feature: %w", err)
	}

	return &feature, nil
}

// installFeature installs/enables a Windows feature
func (m *WinFeatureModule) installFeature(ctx context.Context, name string, decl *StateDeclaration, result *StateResult) error {
	// First, try to determine if this is a server feature or optional feature
	serverFeature, _ := m.getWindowsFeature(ctx, name)

	if serverFeature != nil {
		// Use Install-WindowsFeature for server
		return m.installServerFeature(ctx, name, decl, result)
	}

	// Use Enable-WindowsOptionalFeature for client
	return m.enableOptionalFeature(ctx, name, decl, result)
}

// installServerFeature installs a Windows Server feature
func (m *WinFeatureModule) installServerFeature(ctx context.Context, name string, decl *StateDeclaration, result *StateResult) error {
	var params []string
	params = append(params, fmt.Sprintf("-Name '%s'", escapeForPowerShellQuote(name)))

	if getBoolParameter(decl, "include_all_subfeatures", false) {
		params = append(params, "-IncludeAllSubFeature")
	}

	if getBoolParameter(decl, "include_management_tools", false) {
		params = append(params, "-IncludeManagementTools")
	}

	if source := getStringParameter(decl, "source", ""); source != "" {
		params = append(params, fmt.Sprintf("-Source '%s'", escapeForPowerShellQuote(source)))
	}

	params = append(params, "-Confirm:$false")

	psScript := fmt.Sprintf("Install-WindowsFeature %s", strings.Join(params, " "))
	output, err := m.runPowerShellFeature(ctx, psScript)
	if err != nil {
		return fmt.Errorf("failed to install feature: %w, output: %s", err, output)
	}

	// Check if restart is needed
	if strings.Contains(output, "RestartNeeded") && strings.Contains(output, "True") {
		result.Changes["restart_needed"] = true
		if getBoolParameter(decl, "restart", false) {
			result.Comment = fmt.Sprintf("Feature %s installed, restart required", name)
		} else {
			result.Comment = fmt.Sprintf("Feature %s installed, restart required (not performed)", name)
		}
	} else {
		result.Comment = fmt.Sprintf("Feature %s installed", name)
	}

	return nil
}

// enableOptionalFeature enables a Windows Optional Feature
func (m *WinFeatureModule) enableOptionalFeature(ctx context.Context, name string, decl *StateDeclaration, result *StateResult) error {
	var params []string
	params = append(params, fmt.Sprintf("-FeatureName '%s'", escapeForPowerShellQuote(name)))
	params = append(params, "-Online")
	params = append(params, "-NoRestart")

	if getBoolParameter(decl, "include_all", false) {
		params = append(params, "-All")
	}

	if source := getStringParameter(decl, "source", ""); source != "" {
		params = append(params, fmt.Sprintf("-Source '%s'", escapeForPowerShellQuote(source)))
	}

	psScript := fmt.Sprintf("Enable-WindowsOptionalFeature %s", strings.Join(params, " "))
	output, err := m.runPowerShellFeature(ctx, psScript)
	if err != nil {
		return fmt.Errorf("failed to enable optional feature: %w, output: %s", err, output)
	}

	if strings.Contains(output, "RestartNeeded") && strings.Contains(output, "True") {
		result.Changes["restart_needed"] = true
		result.Comment = fmt.Sprintf("Feature %s enabled, restart required", name)
	} else {
		result.Comment = fmt.Sprintf("Feature %s enabled", name)
	}

	return nil
}

// removeFeature removes/disables a Windows feature
func (m *WinFeatureModule) removeFeature(ctx context.Context, name string, decl *StateDeclaration, result *StateResult) error {
	// First, try to determine if this is a server feature or optional feature
	serverFeature, _ := m.getWindowsFeature(ctx, name)

	if serverFeature != nil {
		// Use Uninstall-WindowsFeature for server
		return m.uninstallServerFeature(ctx, name, decl, result)
	}

	// Use Disable-WindowsOptionalFeature for client
	return m.disableOptionalFeature(ctx, name, decl, result)
}

// uninstallServerFeature uninstalls a Windows Server feature
func (m *WinFeatureModule) uninstallServerFeature(ctx context.Context, name string, decl *StateDeclaration, result *StateResult) error {
	params := []string{
		fmt.Sprintf("-Name '%s'", escapeForPowerShellQuote(name)),
		"-Confirm:$false",
	}

	if getBoolParameter(decl, "remove", false) {
		params = append(params, "-Remove")
	}

	psScript := fmt.Sprintf("Uninstall-WindowsFeature %s", strings.Join(params, " "))
	output, err := m.runPowerShellFeature(ctx, psScript)
	if err != nil {
		return fmt.Errorf("failed to uninstall feature: %w, output: %s", err, output)
	}

	if strings.Contains(output, "RestartNeeded") && strings.Contains(output, "True") {
		result.Changes["restart_needed"] = true
		result.Comment = fmt.Sprintf("Feature %s removed, restart required", name)
	} else {
		result.Comment = fmt.Sprintf("Feature %s removed", name)
	}

	return nil
}

// disableOptionalFeature disables a Windows Optional Feature
func (m *WinFeatureModule) disableOptionalFeature(ctx context.Context, name string, decl *StateDeclaration, result *StateResult) error {
	params := []string{
		fmt.Sprintf("-FeatureName '%s'", escapeForPowerShellQuote(name)),
		"-Online",
		"-NoRestart",
	}

	if getBoolParameter(decl, "remove", false) {
		params = append(params, "-Remove")
	}

	psScript := fmt.Sprintf("Disable-WindowsOptionalFeature %s", strings.Join(params, " "))
	output, err := m.runPowerShellFeature(ctx, psScript)
	if err != nil {
		return fmt.Errorf("failed to disable optional feature: %w, output: %s", err, output)
	}

	if strings.Contains(output, "RestartNeeded") && strings.Contains(output, "True") {
		result.Changes["restart_needed"] = true
		result.Comment = fmt.Sprintf("Feature %s disabled, restart required", name)
	} else {
		result.Comment = fmt.Sprintf("Feature %s disabled", name)
	}

	return nil
}

// runPowerShellFeature runs a PowerShell script and returns the output
func (m *WinFeatureModule) runPowerShellFeature(ctx context.Context, script string) (string, error) {
	cmd := exec.CommandContext(ctx, "powershell.exe",
		"-NoProfile",
		"-NonInteractive",
		"-ExecutionPolicy", "Bypass",
		"-Command", script)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("PowerShell command failed: %w", err)
	}

	return string(output), nil
}

// escapeForPowerShellQuote escapes special characters for PowerShell single-quoted strings
func escapeForPowerShellQuote(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func init() {
	RegisterModule(NewWinFeatureModule())
}
