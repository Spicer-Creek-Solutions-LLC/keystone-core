// Copyright 2024 Keystone Core Contributors
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

// WinFirewallModule implements Windows Firewall rule management using PowerShell
type WinFirewallModule struct {
	*BaseModule
}

// NewWinFirewallModule creates a new Windows firewall module
func NewWinFirewallModule() *WinFirewallModule {
	return &WinFirewallModule{
		BaseModule: NewBaseModule("win_firewall", []string{
			"present", "absent", "enabled", "disabled",
		}),
	}
}

// FirewallRule represents a Windows Firewall rule
type FirewallRule struct {
	Name        string `json:"Name"`
	DisplayName string `json:"DisplayName"`
	Description string `json:"Description"`
	Direction   string `json:"Direction"`
	Action      string `json:"Action"`
	Enabled     string `json:"Enabled"`
	Profile     string `json:"Profile"`
	Protocol    string `json:"Protocol"`
	LocalPort   string `json:"LocalPort"`
	RemotePort  string `json:"RemotePort"`
	Program     string `json:"Program"`
}

// Check checks the current state of a Windows Firewall rule
func (m *WinFirewallModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	ruleName := getStringParameter(decl, "name", decl.ID)

	// Get the firewall rule
	rule, err := m.getFirewallRule(ctx, ruleName)
	if err != nil {
		return nil, fmt.Errorf("failed to get firewall rule: %w", err)
	}

	if rule == nil {
		// Rule doesn't exist
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

	// Rule exists
	result.Present = true
	if rule.Enabled == "True" {
		result.CurrentState = "enabled"
	} else {
		result.CurrentState = "disabled"
	}

	// Store metadata
	result.Metadata["name"] = rule.Name
	result.Metadata["display_name"] = rule.DisplayName
	result.Metadata["description"] = rule.Description
	result.Metadata["direction"] = rule.Direction
	result.Metadata["action"] = rule.Action
	result.Metadata["enabled"] = rule.Enabled == "True"
	result.Metadata["profile"] = rule.Profile
	result.Metadata["protocol"] = rule.Protocol
	result.Metadata["local_port"] = rule.LocalPort
	result.Metadata["remote_port"] = rule.RemotePort
	result.Metadata["program"] = rule.Program

	// Compare with desired state
	switch decl.State {
	case "present", "enabled":
		if decl.State == "enabled" && rule.Enabled != "True" {
			result.Matches = false
			result.Diff["enabled"] = map[string]bool{"current": false, "desired": true}
		} else {
			result.Matches = true
		}

		// Check properties
		m.checkRuleProperties(decl, rule, result)

	case "disabled":
		if rule.Enabled == "True" {
			result.Matches = false
			result.Diff["enabled"] = map[string]bool{"current": true, "desired": false}
		} else {
			result.Matches = true
		}

	case "absent":
		result.Matches = false
		result.Diff["state"] = map[string]string{"current": "present", "desired": "absent"}
	}

	return result, nil
}

// checkRuleProperties checks if rule properties match desired values
func (m *WinFirewallModule) checkRuleProperties(decl *StateDeclaration, rule *FirewallRule, result *ModuleCheckResult) {
	if direction := getStringParameter(decl, "direction", ""); direction != "" {
		currentDir := strings.ToLower(rule.Direction)
		desiredDir := strings.ToLower(direction)
		// Normalize: Inbound/Outbound vs inbound/outbound
		if currentDir != desiredDir && !strings.EqualFold(currentDir, desiredDir) {
			result.Matches = false
			result.Diff["direction"] = map[string]string{"current": rule.Direction, "desired": direction}
		}
	}

	if action := getStringParameter(decl, "action", ""); action != "" {
		if !strings.EqualFold(rule.Action, action) {
			result.Matches = false
			result.Diff["action"] = map[string]string{"current": rule.Action, "desired": action}
		}
	}

	if protocol := getStringParameter(decl, "protocol", ""); protocol != "" {
		if !strings.EqualFold(rule.Protocol, protocol) {
			result.Matches = false
			result.Diff["protocol"] = map[string]string{"current": rule.Protocol, "desired": protocol}
		}
	}

	if localPort := getStringParameter(decl, "local_port", ""); localPort != "" {
		if rule.LocalPort != localPort {
			result.Matches = false
			result.Diff["local_port"] = map[string]string{"current": rule.LocalPort, "desired": localPort}
		}
	}

	if program := getStringParameter(decl, "program", ""); program != "" {
		if !strings.EqualFold(rule.Program, program) {
			result.Matches = false
			result.Diff["program"] = map[string]string{"current": rule.Program, "desired": program}
		}
	}
}

// Apply applies the Windows Firewall rule state
func (m *WinFirewallModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
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

	ruleName := getStringParameter(decl, "name", decl.ID)

	var applyErr error
	switch decl.State {
	case "present":
		applyErr = m.applyPresent(ctx, ruleName, checkResult.Present, decl, result)
	case "enabled":
		applyErr = m.applyEnabled(ctx, ruleName, checkResult.Present, decl, result)
	case "disabled":
		applyErr = m.applyDisabled(ctx, ruleName, result)
	case "absent":
		applyErr = m.applyAbsent(ctx, ruleName, result)
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

// Test tests if the firewall rule is in the desired state
func (m *WinFirewallModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil
}

// getFirewallRule retrieves a firewall rule by name
func (m *WinFirewallModule) getFirewallRule(ctx context.Context, name string) (*FirewallRule, error) {
	psScript := fmt.Sprintf(`
$rule = Get-NetFirewallRule -Name '%s' -ErrorAction SilentlyContinue
if ($null -eq $rule) {
    Write-Output 'null'
} else {
    $portFilter = Get-NetFirewallPortFilter -AssociatedNetFirewallRule $rule
    $appFilter = Get-NetFirewallApplicationFilter -AssociatedNetFirewallRule $rule
    $result = @{
        Name = $rule.Name
        DisplayName = $rule.DisplayName
        Description = $rule.Description
        Direction = $rule.Direction.ToString()
        Action = $rule.Action.ToString()
        Enabled = $rule.Enabled.ToString()
        Profile = $rule.Profile.ToString()
        Protocol = if ($portFilter) { $portFilter.Protocol } else { '' }
        LocalPort = if ($portFilter.LocalPort) { $portFilter.LocalPort -join ',' } else { '' }
        RemotePort = if ($portFilter.RemotePort) { $portFilter.RemotePort -join ',' } else { '' }
        Program = if ($appFilter) { $appFilter.Program } else { '' }
    }
    $result | ConvertTo-Json -Compress
}
`, escapeForPowerShell(name))

	output, err := m.runPowerShell(ctx, psScript)
	if err != nil {
		return nil, err
	}

	output = strings.TrimSpace(output)
	if output == "null" || output == "" {
		return nil, nil
	}

	var rule FirewallRule
	if err := json.Unmarshal([]byte(output), &rule); err != nil {
		return nil, fmt.Errorf("failed to parse firewall rule: %w", err)
	}

	return &rule, nil
}

// applyPresent ensures the firewall rule exists with the desired properties
func (m *WinFirewallModule) applyPresent(ctx context.Context, name string, exists bool, decl *StateDeclaration, result *StateResult) error {
	if exists {
		// Update existing rule
		return m.updateFirewallRule(ctx, name, decl, result)
	}

	// Create new rule
	return m.createFirewallRule(ctx, name, decl, result)
}

// applyEnabled ensures the rule exists and is enabled
func (m *WinFirewallModule) applyEnabled(ctx context.Context, name string, exists bool, decl *StateDeclaration, result *StateResult) error {
	if !exists {
		// Create new rule
		if err := m.createFirewallRule(ctx, name, decl, result); err != nil {
			return err
		}
	}

	// Enable the rule
	psScript := fmt.Sprintf(`Enable-NetFirewallRule -Name '%s'`, escapeForPowerShell(name))
	if _, err := m.runPowerShell(ctx, psScript); err != nil {
		return fmt.Errorf("failed to enable firewall rule: %w", err)
	}

	result.Comment = fmt.Sprintf("Firewall rule %s enabled", name)
	return nil
}

// applyDisabled disables the firewall rule
func (m *WinFirewallModule) applyDisabled(ctx context.Context, name string, result *StateResult) error {
	psScript := fmt.Sprintf(`Disable-NetFirewallRule -Name '%s'`, escapeForPowerShell(name))
	if _, err := m.runPowerShell(ctx, psScript); err != nil {
		return fmt.Errorf("failed to disable firewall rule: %w", err)
	}

	result.Comment = fmt.Sprintf("Firewall rule %s disabled", name)
	return nil
}

// applyAbsent removes the firewall rule
func (m *WinFirewallModule) applyAbsent(ctx context.Context, name string, result *StateResult) error {
	psScript := fmt.Sprintf(`
$rule = Get-NetFirewallRule -Name '%s' -ErrorAction SilentlyContinue
if ($null -ne $rule) {
    Remove-NetFirewallRule -Name '%s'
}
`, escapeForPowerShell(name), escapeForPowerShell(name))

	if _, err := m.runPowerShell(ctx, psScript); err != nil {
		return fmt.Errorf("failed to remove firewall rule: %w", err)
	}

	result.Comment = fmt.Sprintf("Firewall rule %s removed", name)
	return nil
}

// createFirewallRule creates a new firewall rule
func (m *WinFirewallModule) createFirewallRule(ctx context.Context, name string, decl *StateDeclaration, result *StateResult) error {
	// Build the New-NetFirewallRule command
	var params []string
	params = append(params, fmt.Sprintf("-Name '%s'", escapeForPowerShell(name)))

	if displayName := getStringParameter(decl, "display_name", ""); displayName != "" {
		params = append(params, fmt.Sprintf("-DisplayName '%s'", escapeForPowerShell(displayName)))
	} else {
		params = append(params, fmt.Sprintf("-DisplayName '%s'", escapeForPowerShell(name)))
	}

	if description := getStringParameter(decl, "description", ""); description != "" {
		params = append(params, fmt.Sprintf("-Description '%s'", escapeForPowerShell(description)))
	}

	direction := getStringParameter(decl, "direction", "Inbound")
	params = append(params, fmt.Sprintf("-Direction %s", direction))

	action := getStringParameter(decl, "action", "Allow")
	params = append(params, fmt.Sprintf("-Action %s", action))

	if protocol := getStringParameter(decl, "protocol", ""); protocol != "" {
		params = append(params, fmt.Sprintf("-Protocol %s", protocol))
	}

	if localPort := getStringParameter(decl, "local_port", ""); localPort != "" {
		params = append(params, fmt.Sprintf("-LocalPort %s", localPort))
	}

	if remotePort := getStringParameter(decl, "remote_port", ""); remotePort != "" {
		params = append(params, fmt.Sprintf("-RemotePort %s", remotePort))
	}

	if program := getStringParameter(decl, "program", ""); program != "" {
		params = append(params, fmt.Sprintf("-Program '%s'", escapeForPowerShell(program)))
	}

	if profile := getStringParameter(decl, "profile", ""); profile != "" {
		params = append(params, fmt.Sprintf("-Profile %s", profile))
	}

	psScript := fmt.Sprintf("New-NetFirewallRule %s", strings.Join(params, " "))
	if _, err := m.runPowerShell(ctx, psScript); err != nil {
		return fmt.Errorf("failed to create firewall rule: %w", err)
	}

	result.Comment = fmt.Sprintf("Firewall rule %s created", name)
	return nil
}

// updateFirewallRule updates an existing firewall rule
func (m *WinFirewallModule) updateFirewallRule(ctx context.Context, name string, decl *StateDeclaration, result *StateResult) error {
	var params []string

	if displayName := getStringParameter(decl, "display_name", ""); displayName != "" {
		params = append(params, fmt.Sprintf("-NewDisplayName '%s'", escapeForPowerShell(displayName)))
	}

	if description := getStringParameter(decl, "description", ""); description != "" {
		params = append(params, fmt.Sprintf("-Description '%s'", escapeForPowerShell(description)))
	}

	if action := getStringParameter(decl, "action", ""); action != "" {
		params = append(params, fmt.Sprintf("-Action %s", action))
	}

	if profile := getStringParameter(decl, "profile", ""); profile != "" {
		params = append(params, fmt.Sprintf("-Profile %s", profile))
	}

	if len(params) > 0 {
		psScript := fmt.Sprintf("Set-NetFirewallRule -Name '%s' %s", escapeForPowerShell(name), strings.Join(params, " "))
		if _, err := m.runPowerShell(ctx, psScript); err != nil {
			return fmt.Errorf("failed to update firewall rule: %w", err)
		}
	}

	result.Comment = fmt.Sprintf("Firewall rule %s updated", name)
	return nil
}

// runPowerShell runs a PowerShell script and returns the output
func (m *WinFirewallModule) runPowerShell(ctx context.Context, script string) (string, error) {
	cmd := exec.CommandContext(ctx, "powershell.exe",
		"-NoProfile",
		"-NonInteractive",
		"-ExecutionPolicy", "Bypass",
		"-Command", script)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("PowerShell command failed: %w, output: %s", err, string(output))
	}

	return string(output), nil
}

// escapeForPowerShell escapes special characters for PowerShell strings
func escapeForPowerShell(s string) string {
	s = strings.ReplaceAll(s, "'", "''")
	return s
}

func init() {
	RegisterModule(NewWinFirewallModule())
}
