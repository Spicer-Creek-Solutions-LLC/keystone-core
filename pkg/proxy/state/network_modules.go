// Package state provides network device configuration modules.
package state

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shawnbutts/keystone-core/pkg/proxy"
)

// =============================================================================
// Network Device Configuration Modules
// =============================================================================

// IOSConfigModule manages Cisco IOS configuration.
type IOSConfigModule struct {
	BaseProxyModule
}

// NewIOSConfigModule creates a new IOS config module.
func NewIOSConfigModule() *IOSConfigModule {
	return &IOSConfigModule{BaseProxyModule{name: "ios_config"}}
}

// Execute runs the IOS config module.
func (m *IOSConfigModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	// Get lines to configure
	lines, hasLines := m.GetStringSlice(mctx.Parameters, "lines")
	parents, _ := m.GetStringSlice(mctx.Parameters, "parents")
	before, _ := m.GetStringSlice(mctx.Parameters, "before")
	after, _ := m.GetStringSlice(mctx.Parameters, "after")
	replace, _ := m.GetString(mctx.Parameters, "replace")
	backup, _ := m.GetBool(mctx.Parameters, "backup")
	save, _ := m.GetBool(mctx.Parameters, "save")

	if !hasLines {
		return nil, fmt.Errorf("lines parameter is required")
	}

	// Create backup if requested
	if backup {
		backupResult, err := mctx.ExecuteCommand(ctx, "show running-config")
		if err == nil {
			result.Details["backup"] = backupResult.Stdout
		}
	}

	// Build configuration commands
	var commands []string

	// Enter configuration mode
	commands = append(commands, "configure terminal")

	// Navigate to parent context
	for _, parent := range parents {
		commands = append(commands, parent)
	}

	// Add before commands
	commands = append(commands, before...)

	// Handle replace mode
	if replace == "block" {
		// Remove existing block first
		for _, parent := range parents {
			commands = append(commands, fmt.Sprintf("no %s", parent))
		}
		// Re-enter parent context
		for _, parent := range parents {
			commands = append(commands, parent)
		}
	}

	// Add configuration lines
	commands = append(commands, lines...)

	// Add after commands
	commands = append(commands, after...)

	// Exit configuration mode
	commands = append(commands, "end")

	// Check if configuration already matches
	runningResult, _ := mctx.ExecuteCommand(ctx, "show running-config")
	needsChange := false
	for _, line := range lines {
		if !strings.Contains(string(runningResult.Stdout), strings.TrimSpace(line)) {
			needsChange = true
			break
		}
	}

	if !needsChange {
		result.Comment = "Configuration already matches"
		return result, nil
	}

	result.Changed = true

	if mctx.DryRun {
		result.Comment = "Configuration would be applied"
		result.Details["commands"] = commands
		return result, nil
	}

	// Apply configuration
	for _, cmd := range commands {
		if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
			return nil, fmt.Errorf("failed to execute '%s': %w", cmd, err)
		}
	}

	// Save configuration if requested
	if save {
		if _, err := mctx.ExecuteCommand(ctx, "write memory"); err != nil {
			return nil, fmt.Errorf("failed to save configuration: %w", err)
		}
	}

	result.Comment = "Configuration applied"
	result.Details["commands"] = commands
	return result, nil
}

// Check performs a dry-run check.
func (m *IOSConfigModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// NXOSConfigModule manages Cisco NX-OS configuration.
type NXOSConfigModule struct {
	BaseProxyModule
}

// NewNXOSConfigModule creates a new NX-OS config module.
func NewNXOSConfigModule() *NXOSConfigModule {
	return &NXOSConfigModule{BaseProxyModule{name: "nxos_config"}}
}

// Execute runs the NX-OS config module.
func (m *NXOSConfigModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	lines, hasLines := m.GetStringSlice(mctx.Parameters, "lines")
	parents, _ := m.GetStringSlice(mctx.Parameters, "parents")
	save, _ := m.GetBool(mctx.Parameters, "save")

	if !hasLines {
		return nil, fmt.Errorf("lines parameter is required")
	}

	// Build commands (similar to IOS but with NX-OS specifics)
	var commands []string
	commands = append(commands, "configure terminal")

	for _, parent := range parents {
		commands = append(commands, parent)
	}

	commands = append(commands, lines...)
	commands = append(commands, "end")

	// Check if change is needed
	runningResult, _ := mctx.ExecuteCommand(ctx, "show running-config")
	needsChange := false
	for _, line := range lines {
		if !strings.Contains(string(runningResult.Stdout), strings.TrimSpace(line)) {
			needsChange = true
			break
		}
	}

	if !needsChange {
		result.Comment = "Configuration already matches"
		return result, nil
	}

	result.Changed = true

	if mctx.DryRun {
		result.Comment = "Configuration would be applied"
		result.Details["commands"] = commands
		return result, nil
	}

	// Apply configuration
	for _, cmd := range commands {
		if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
			return nil, fmt.Errorf("failed to execute '%s': %w", cmd, err)
		}
	}

	if save {
		if _, err := mctx.ExecuteCommand(ctx, "copy running-config startup-config"); err != nil {
			return nil, fmt.Errorf("failed to save configuration: %w", err)
		}
	}

	result.Comment = "Configuration applied"
	return result, nil
}

// Check performs a dry-run check.
func (m *NXOSConfigModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// JUNOSConfigModule manages Juniper JUNOS configuration.
type JUNOSConfigModule struct {
	BaseProxyModule
}

// NewJUNOSConfigModule creates a new JUNOS config module.
func NewJUNOSConfigModule() *JUNOSConfigModule {
	return &JUNOSConfigModule{BaseProxyModule{name: "junos_config"}}
}

// Execute runs the JUNOS config module.
func (m *JUNOSConfigModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	lines, hasLines := m.GetStringSlice(mctx.Parameters, "lines")
	src, hasSrc := m.GetString(mctx.Parameters, "src")
	comment, _ := m.GetString(mctx.Parameters, "comment")
	confirm, _ := m.GetInt(mctx.Parameters, "confirm")
	rollback, _ := m.GetInt(mctx.Parameters, "rollback")

	if !hasLines && !hasSrc && rollback == 0 {
		return nil, fmt.Errorf("lines, src, or rollback parameter is required")
	}

	// Handle rollback
	if rollback > 0 {
		result.Changed = true
		if mctx.DryRun {
			result.Comment = fmt.Sprintf("Would rollback to %d", rollback)
			return result, nil
		}
		if _, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("rollback %d", rollback)); err != nil {
			return nil, fmt.Errorf("rollback failed: %w", err)
		}
		if _, err := mctx.ExecuteCommand(ctx, "commit"); err != nil {
			return nil, fmt.Errorf("commit failed: %w", err)
		}
		result.Comment = fmt.Sprintf("Rolled back to %d", rollback)
		return result, nil
	}

	// Enter configuration mode
	if _, err := mctx.ExecuteCommand(ctx, "configure"); err != nil {
		return nil, fmt.Errorf("failed to enter configuration mode: %w", err)
	}

	// Apply configuration
	if hasSrc {
		if _, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("load merge %s", src)); err != nil {
			mctx.ExecuteCommand(ctx, "rollback 0")
			return nil, fmt.Errorf("failed to load configuration: %w", err)
		}
	} else {
		for _, line := range lines {
			if _, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("set %s", line)); err != nil {
				mctx.ExecuteCommand(ctx, "rollback 0")
				return nil, fmt.Errorf("failed to set '%s': %w", line, err)
			}
		}
	}

	// Check for differences
	diffResult, _ := mctx.ExecuteCommand(ctx, "show | compare")
	if strings.TrimSpace(string(diffResult.Stdout)) == "" {
		mctx.ExecuteCommand(ctx, "rollback 0")
		result.Comment = "No changes needed"
		return result, nil
	}

	result.Changed = true
	result.Details["diff"] = diffResult.Stdout

	if mctx.DryRun {
		mctx.ExecuteCommand(ctx, "rollback 0")
		result.Comment = "Configuration would be committed"
		return result, nil
	}

	// Commit with optional confirm
	commitCmd := "commit"
	if comment != "" {
		commitCmd += fmt.Sprintf(" comment \"%s\"", comment)
	}
	if confirm > 0 {
		commitCmd += fmt.Sprintf(" confirmed %d", confirm)
	}

	if _, err := mctx.ExecuteCommand(ctx, commitCmd); err != nil {
		mctx.ExecuteCommand(ctx, "rollback 0")
		return nil, fmt.Errorf("commit failed: %w", err)
	}

	result.Comment = "Configuration committed"
	return result, nil
}

// Check performs a dry-run check.
func (m *JUNOSConfigModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// EOSConfigModule manages Arista EOS configuration.
type EOSConfigModule struct {
	BaseProxyModule
}

// NewEOSConfigModule creates a new EOS config module.
func NewEOSConfigModule() *EOSConfigModule {
	return &EOSConfigModule{BaseProxyModule{name: "eos_config"}}
}

// Execute runs the EOS config module.
func (m *EOSConfigModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	lines, hasLines := m.GetStringSlice(mctx.Parameters, "lines")
	parents, _ := m.GetStringSlice(mctx.Parameters, "parents")
	save, _ := m.GetBool(mctx.Parameters, "save")
	diff, _ := m.GetBool(mctx.Parameters, "diff")

	if !hasLines {
		return nil, fmt.Errorf("lines parameter is required")
	}

	// Create session for atomic changes
	sessionResult, _ := mctx.ExecuteCommand(ctx, "configure session kscore")

	var commands []string
	for _, parent := range parents {
		commands = append(commands, parent)
	}
	commands = append(commands, lines...)

	// Check if change is needed (using EOS session diff)
	runningResult, _ := mctx.ExecuteCommand(ctx, "show running-config")
	needsChange := false
	for _, line := range lines {
		if !strings.Contains(string(runningResult.Stdout), strings.TrimSpace(line)) {
			needsChange = true
			break
		}
	}

	if !needsChange {
		mctx.ExecuteCommand(ctx, "abort")
		result.Comment = "Configuration already matches"
		return result, nil
	}

	result.Changed = true

	if mctx.DryRun {
		mctx.ExecuteCommand(ctx, "abort")
		result.Comment = "Configuration would be applied"
		result.Details["commands"] = commands
		return result, nil
	}

	// Apply configuration
	for _, cmd := range commands {
		if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
			mctx.ExecuteCommand(ctx, "abort")
			return nil, fmt.Errorf("failed to execute '%s': %w", cmd, err)
		}
	}

	// Show diff if requested
	if diff && sessionResult.ExitCode == 0 {
		diffResult, _ := mctx.ExecuteCommand(ctx, "show session-config diffs")
		result.Details["diff"] = diffResult.Stdout
	}

	// Commit session
	if _, err := mctx.ExecuteCommand(ctx, "commit"); err != nil {
		mctx.ExecuteCommand(ctx, "abort")
		return nil, fmt.Errorf("commit failed: %w", err)
	}

	if save {
		if _, err := mctx.ExecuteCommand(ctx, "write memory"); err != nil {
			return nil, fmt.Errorf("failed to save configuration: %w", err)
		}
	}

	result.Comment = "Configuration applied"
	return result, nil
}

// Check performs a dry-run check.
func (m *EOSConfigModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// VyOSConfigModule manages VyOS configuration.
type VyOSConfigModule struct {
	BaseProxyModule
}

// NewVyOSConfigModule creates a new VyOS config module.
func NewVyOSConfigModule() *VyOSConfigModule {
	return &VyOSConfigModule{BaseProxyModule{name: "vyos_config"}}
}

// Execute runs the VyOS config module.
func (m *VyOSConfigModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	lines, hasLines := m.GetStringSlice(mctx.Parameters, "lines")
	save, _ := m.GetBool(mctx.Parameters, "save")

	if !hasLines {
		return nil, fmt.Errorf("lines parameter is required")
	}

	// Enter configuration mode
	if _, err := mctx.ExecuteCommand(ctx, "configure"); err != nil {
		return nil, fmt.Errorf("failed to enter configuration mode: %w", err)
	}

	// Apply set commands
	for _, line := range lines {
		cmd := line
		if !strings.HasPrefix(strings.ToLower(line), "set ") &&
			!strings.HasPrefix(strings.ToLower(line), "delete ") {
			cmd = "set " + line
		}
		if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
			mctx.ExecuteCommand(ctx, "exit discard")
			return nil, fmt.Errorf("failed to execute '%s': %w", cmd, err)
		}
	}

	// Compare changes
	compareResult, _ := mctx.ExecuteCommand(ctx, "compare")
	if strings.TrimSpace(string(compareResult.Stdout)) == "No changes" {
		mctx.ExecuteCommand(ctx, "exit discard")
		result.Comment = "No changes needed"
		return result, nil
	}

	result.Changed = true
	result.Details["diff"] = compareResult.Stdout

	if mctx.DryRun {
		mctx.ExecuteCommand(ctx, "exit discard")
		result.Comment = "Configuration would be committed"
		return result, nil
	}

	// Commit changes
	if _, err := mctx.ExecuteCommand(ctx, "commit"); err != nil {
		mctx.ExecuteCommand(ctx, "exit discard")
		return nil, fmt.Errorf("commit failed: %w", err)
	}

	// Save if requested
	if save {
		if _, err := mctx.ExecuteCommand(ctx, "save"); err != nil {
			return nil, fmt.Errorf("failed to save configuration: %w", err)
		}
	}

	mctx.ExecuteCommand(ctx, "exit")
	result.Comment = "Configuration committed"
	return result, nil
}

// Check performs a dry-run check.
func (m *VyOSConfigModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// PfSenseConfigModule manages pfSense configuration.
type PfSenseConfigModule struct {
	BaseProxyModule
}

// NewPfSenseConfigModule creates a new pfSense config module.
func NewPfSenseConfigModule() *PfSenseConfigModule {
	return &PfSenseConfigModule{BaseProxyModule{name: "pfsense_config"}}
}

// Execute runs the pfSense config module.
func (m *PfSenseConfigModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	section, _ := m.GetString(mctx.Parameters, "section")
	config, hasConfig := mctx.Parameters["config"].(map[string]interface{})

	if section == "" {
		return nil, fmt.Errorf("section parameter is required")
	}

	if !hasConfig {
		return nil, fmt.Errorf("config parameter is required")
	}

	// Get current configuration via REST API
	getResult, _ := mctx.ExecuteCommand(ctx, fmt.Sprintf("GET /api/v1/%s", section))

	// Compare configurations
	// This is simplified - real implementation would do proper diff
	result.Changed = true

	if mctx.DryRun {
		result.Comment = fmt.Sprintf("Would update %s configuration", section)
		result.Details["current"] = getResult.Stdout
		return result, nil
	}

	// Apply configuration via REST API
	configJSON, _ := json.Marshal(config)
	putResult, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("PUT /api/v1/%s %s", section, string(configJSON)))
	if err != nil || putResult.ExitCode != 0 {
		return nil, fmt.Errorf("failed to apply configuration: %w", err)
	}

	// Apply changes (reload relevant service)
	mctx.ExecuteCommand(ctx, "POST /api/v1/firewall/apply")

	result.Comment = fmt.Sprintf("Configuration for %s updated", section)
	return result, nil
}

// Check performs a dry-run check.
func (m *PfSenseConfigModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// OPNsenseConfigModule manages OPNsense configuration.
type OPNsenseConfigModule struct {
	BaseProxyModule
}

// NewOPNsenseConfigModule creates a new OPNsense config module.
func NewOPNsenseConfigModule() *OPNsenseConfigModule {
	return &OPNsenseConfigModule{BaseProxyModule{name: "opnsense_config"}}
}

// Execute runs the OPNsense config module.
func (m *OPNsenseConfigModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	module, _ := m.GetString(mctx.Parameters, "module")
	controller, _ := m.GetString(mctx.Parameters, "controller")
	action, _ := m.GetString(mctx.Parameters, "action")
	params, hasParams := mctx.Parameters["params"].(map[string]interface{})

	if module == "" || controller == "" {
		return nil, fmt.Errorf("module and controller parameters are required")
	}

	if action == "" {
		action = "set"
	}

	// Build API path
	path := fmt.Sprintf("/api/%s/%s/%s", module, controller, action)

	// Get current state for comparison
	getPath := fmt.Sprintf("/api/%s/%s/get", module, controller)
	getResult, _ := mctx.ExecuteCommand(ctx, fmt.Sprintf("GET %s", getPath))

	result.Changed = true

	if mctx.DryRun {
		result.Comment = fmt.Sprintf("Would call %s", path)
		result.Details["current"] = getResult.Stdout
		return result, nil
	}

	// Apply configuration
	var setResult *proxy.ProxiedExecuteResult
	var err error
	if hasParams {
		paramsJSON, _ := json.Marshal(params)
		setResult, err = mctx.ExecuteCommand(ctx, fmt.Sprintf("POST %s %s", path, string(paramsJSON)))
	} else {
		setResult, err = mctx.ExecuteCommand(ctx, fmt.Sprintf("POST %s", path))
	}

	if err != nil || setResult.ExitCode != 0 {
		return nil, fmt.Errorf("failed to apply configuration: %w", err)
	}

	// Reconfigure if needed
	if action == "set" {
		reconfigPath := fmt.Sprintf("/api/%s/service/reconfigure", module)
		mctx.ExecuteCommand(ctx, fmt.Sprintf("POST %s", reconfigPath))
	}

	result.Comment = "Configuration applied"
	return result, nil
}

// Check performs a dry-run check.
func (m *OPNsenseConfigModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}
