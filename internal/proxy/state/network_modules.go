// Package state provides network device configuration modules.
package state

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shawnbutts/keystone-core/internal/proxy"
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
	commands = append(commands, parents...)

	// Add before commands
	commands = append(commands, before...)

	// Handle replace mode
	if replace == "block" {
		// Remove existing block first
		for _, parent := range parents {
			commands = append(commands, fmt.Sprintf("no %s", parent))
		}
		// Re-enter parent context
		commands = append(commands, parents...)
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
	commands := make([]string, 0, 2+len(parents)+len(lines))
	commands = append(commands, "configure terminal")
	commands = append(commands, parents...)

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
			_, _ = mctx.ExecuteCommand(ctx, "rollback 0") //nolint:errcheck // best-effort rollback
			return nil, fmt.Errorf("failed to load configuration: %w", err)
		}
	} else {
		for _, line := range lines {
			if _, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("set %s", line)); err != nil {
				_, _ = mctx.ExecuteCommand(ctx, "rollback 0") //nolint:errcheck // best-effort rollback
				return nil, fmt.Errorf("failed to set '%s': %w", line, err)
			}
		}
	}

	// Check for differences
	diffResult, _ := mctx.ExecuteCommand(ctx, "show | compare")
	if strings.TrimSpace(string(diffResult.Stdout)) == "" {
		_, _ = mctx.ExecuteCommand(ctx, "rollback 0") //nolint:errcheck // best-effort rollback
		result.Comment = "No changes needed"
		return result, nil
	}

	result.Changed = true
	result.Details["diff"] = diffResult.Stdout

	if mctx.DryRun {
		_, _ = mctx.ExecuteCommand(ctx, "rollback 0") //nolint:errcheck // best-effort rollback
		result.Comment = "Configuration would be committed"
		return result, nil
	}

	// Commit with optional confirm
	commitCmd := "commit"
	if comment != "" {
		commitCmd += fmt.Sprintf(" comment %q", comment)
	}
	if confirm > 0 {
		commitCmd += fmt.Sprintf(" confirmed %d", confirm)
	}

	if _, err := mctx.ExecuteCommand(ctx, commitCmd); err != nil {
		_, _ = mctx.ExecuteCommand(ctx, "rollback 0") //nolint:errcheck // best-effort rollback
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

	commands := make([]string, 0, len(parents)+len(lines))
	commands = append(commands, parents...)
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
		_, _ = mctx.ExecuteCommand(ctx, "abort") //nolint:errcheck // best-effort cleanup
		result.Comment = "Configuration already matches"
		return result, nil
	}

	result.Changed = true

	if mctx.DryRun {
		_, _ = mctx.ExecuteCommand(ctx, "abort") //nolint:errcheck // best-effort cleanup
		result.Comment = "Configuration would be applied"
		result.Details["commands"] = commands
		return result, nil
	}

	// Apply configuration
	for _, cmd := range commands {
		if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
			_, _ = mctx.ExecuteCommand(ctx, "abort") //nolint:errcheck // best-effort cleanup
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
		_, _ = mctx.ExecuteCommand(ctx, "abort") //nolint:errcheck // best-effort cleanup
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
			_, _ = mctx.ExecuteCommand(ctx, "exit discard") //nolint:errcheck // best-effort cleanup
			return nil, fmt.Errorf("failed to execute '%s': %w", cmd, err)
		}
	}

	// Compare changes
	compareResult, _ := mctx.ExecuteCommand(ctx, "compare")
	if strings.TrimSpace(string(compareResult.Stdout)) == "No changes" {
		_, _ = mctx.ExecuteCommand(ctx, "exit discard") //nolint:errcheck // best-effort cleanup
		result.Comment = "No changes needed"
		return result, nil
	}

	result.Changed = true
	result.Details["diff"] = compareResult.Stdout

	if mctx.DryRun {
		_, _ = mctx.ExecuteCommand(ctx, "exit discard") //nolint:errcheck // best-effort cleanup
		result.Comment = "Configuration would be committed"
		return result, nil
	}

	// Commit changes
	if _, err := mctx.ExecuteCommand(ctx, "commit"); err != nil {
		_, _ = mctx.ExecuteCommand(ctx, "exit discard") //nolint:errcheck // best-effort cleanup
		return nil, fmt.Errorf("commit failed: %w", err)
	}

	// Save if requested
	if save {
		if _, err := mctx.ExecuteCommand(ctx, "save"); err != nil {
			return nil, fmt.Errorf("failed to save configuration: %w", err)
		}
	}

	_, _ = mctx.ExecuteCommand(ctx, "exit") //nolint:errcheck // best-effort cleanup
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
	_, _ = mctx.ExecuteCommand(ctx, "POST /api/v1/firewall/apply") //nolint:errcheck // best-effort apply

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
		_, _ = mctx.ExecuteCommand(ctx, fmt.Sprintf("POST %s", reconfigPath)) //nolint:errcheck // best-effort reconfigure
	}

	result.Comment = "Configuration applied"
	return result, nil
}

// Check performs a dry-run check.
func (m *OPNsenseConfigModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// =============================================================================
// HP/Aruba Configuration Modules
// =============================================================================

// HPProCurveConfigModule manages HP ProCurve configuration.
type HPProCurveConfigModule struct {
	BaseProxyModule
}

// NewHPProCurveConfigModule creates a new HP ProCurve config module.
func NewHPProCurveConfigModule() *HPProCurveConfigModule {
	return &HPProCurveConfigModule{BaseProxyModule{name: "hp_procurve_config"}}
}

// Execute runs the HP ProCurve config module.
func (m *HPProCurveConfigModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	lines, hasLines := m.GetStringSlice(mctx.Parameters, "lines")
	parents, _ := m.GetStringSlice(mctx.Parameters, "parents")
	save, _ := m.GetBool(mctx.Parameters, "save")

	if !hasLines {
		return nil, fmt.Errorf("lines parameter is required")
	}

	commands := make([]string, 0, 2+len(parents)+len(lines))
	commands = append(commands, "configure terminal")
	commands = append(commands, parents...)
	commands = append(commands, lines...)
	commands = append(commands, "exit")

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

	for _, cmd := range commands {
		if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
			return nil, fmt.Errorf("failed to execute '%s': %w", cmd, err)
		}
	}

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
func (m *HPProCurveConfigModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// HPArubaOSConfigModule manages HP ArubaOS configuration.
type HPArubaOSConfigModule struct {
	BaseProxyModule
}

// NewHPArubaOSConfigModule creates a new HP ArubaOS config module.
func NewHPArubaOSConfigModule() *HPArubaOSConfigModule {
	return &HPArubaOSConfigModule{BaseProxyModule{name: "hp_arubaos_config"}}
}

// Execute runs the HP ArubaOS config module.
func (m *HPArubaOSConfigModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	lines, hasLines := m.GetStringSlice(mctx.Parameters, "lines")
	parents, _ := m.GetStringSlice(mctx.Parameters, "parents")
	save, _ := m.GetBool(mctx.Parameters, "save")

	if !hasLines {
		return nil, fmt.Errorf("lines parameter is required")
	}

	commands := make([]string, 0, 2+len(parents)+len(lines))
	commands = append(commands, "configure terminal")
	commands = append(commands, parents...)
	commands = append(commands, lines...)
	commands = append(commands, "end")

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

	for _, cmd := range commands {
		if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
			return nil, fmt.Errorf("failed to execute '%s': %w", cmd, err)
		}
	}

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
func (m *HPArubaOSConfigModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// HPAOSCXConfigModule manages Aruba AOS-CX configuration.
type HPAOSCXConfigModule struct {
	BaseProxyModule
}

// NewHPAOSCXConfigModule creates a new AOS-CX config module.
func NewHPAOSCXConfigModule() *HPAOSCXConfigModule {
	return &HPAOSCXConfigModule{BaseProxyModule{name: "hp_aoscx_config"}}
}

// Execute runs the AOS-CX config module.
func (m *HPAOSCXConfigModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	lines, hasLines := m.GetStringSlice(mctx.Parameters, "lines")
	parents, _ := m.GetStringSlice(mctx.Parameters, "parents")
	save, _ := m.GetBool(mctx.Parameters, "save")

	if !hasLines {
		return nil, fmt.Errorf("lines parameter is required")
	}

	commands := make([]string, 0, 2+len(parents)+len(lines))
	commands = append(commands, "configure terminal")
	commands = append(commands, parents...)
	commands = append(commands, lines...)
	commands = append(commands, "end")

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
	result.Details["commands"] = commands
	return result, nil
}

// Check performs a dry-run check.
func (m *HPAOSCXConfigModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// =============================================================================
// Dell Configuration Modules
// =============================================================================

// DellOS10ConfigModule manages Dell OS10 configuration.
type DellOS10ConfigModule struct {
	BaseProxyModule
}

// NewDellOS10ConfigModule creates a new Dell OS10 config module.
func NewDellOS10ConfigModule() *DellOS10ConfigModule {
	return &DellOS10ConfigModule{BaseProxyModule{name: "dell_os10_config"}}
}

// Execute runs the Dell OS10 config module.
func (m *DellOS10ConfigModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	lines, hasLines := m.GetStringSlice(mctx.Parameters, "lines")
	parents, _ := m.GetStringSlice(mctx.Parameters, "parents")
	save, _ := m.GetBool(mctx.Parameters, "save")

	if !hasLines {
		return nil, fmt.Errorf("lines parameter is required")
	}

	commands := make([]string, 0, 2+len(parents)+len(lines))
	commands = append(commands, "configure terminal")
	commands = append(commands, parents...)
	commands = append(commands, lines...)
	commands = append(commands, "end")

	runningResult, _ := mctx.ExecuteCommand(ctx, "show running-configuration")
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

	for _, cmd := range commands {
		if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
			return nil, fmt.Errorf("failed to execute '%s': %w", cmd, err)
		}
	}

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
func (m *DellOS10ConfigModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// DellOS9ConfigModule manages Dell OS9 / FTOS configuration.
type DellOS9ConfigModule struct {
	BaseProxyModule
}

// NewDellOS9ConfigModule creates a new Dell OS9 config module.
func NewDellOS9ConfigModule() *DellOS9ConfigModule {
	return &DellOS9ConfigModule{BaseProxyModule{name: "dell_os9_config"}}
}

// Execute runs the Dell OS9 config module.
func (m *DellOS9ConfigModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	lines, hasLines := m.GetStringSlice(mctx.Parameters, "lines")
	parents, _ := m.GetStringSlice(mctx.Parameters, "parents")
	save, _ := m.GetBool(mctx.Parameters, "save")

	if !hasLines {
		return nil, fmt.Errorf("lines parameter is required")
	}

	commands := make([]string, 0, 2+len(parents)+len(lines))
	commands = append(commands, "configure")
	commands = append(commands, parents...)
	commands = append(commands, lines...)
	commands = append(commands, "end")

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

	for _, cmd := range commands {
		if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
			return nil, fmt.Errorf("failed to execute '%s': %w", cmd, err)
		}
	}

	if save {
		if _, err := mctx.ExecuteCommand(ctx, "write"); err != nil {
			return nil, fmt.Errorf("failed to save configuration: %w", err)
		}
	}

	result.Comment = "Configuration applied"
	result.Details["commands"] = commands
	return result, nil
}

// Check performs a dry-run check.
func (m *DellOS9ConfigModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// DellPowerSwitchConfigModule manages Dell PowerSwitch configuration.
type DellPowerSwitchConfigModule struct {
	BaseProxyModule
}

// NewDellPowerSwitchConfigModule creates a new Dell PowerSwitch config module.
func NewDellPowerSwitchConfigModule() *DellPowerSwitchConfigModule {
	return &DellPowerSwitchConfigModule{BaseProxyModule{name: "dell_powerswitch_config"}}
}

// Execute runs the Dell PowerSwitch config module.
func (m *DellPowerSwitchConfigModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	lines, hasLines := m.GetStringSlice(mctx.Parameters, "lines")
	parents, _ := m.GetStringSlice(mctx.Parameters, "parents")
	save, _ := m.GetBool(mctx.Parameters, "save")

	if !hasLines {
		return nil, fmt.Errorf("lines parameter is required")
	}

	commands := make([]string, 0, 2+len(parents)+len(lines))
	commands = append(commands, "configure")
	commands = append(commands, parents...)
	commands = append(commands, lines...)
	commands = append(commands, "exit")

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
	result.Details["commands"] = commands
	return result, nil
}

// Check performs a dry-run check.
func (m *DellPowerSwitchConfigModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// =============================================================================
// Security Vendor Configuration Modules
// =============================================================================

// FortiOSConfigModule manages FortiOS configuration.
type FortiOSConfigModule struct {
	BaseProxyModule
}

// NewFortiOSConfigModule creates a new FortiOS config module.
func NewFortiOSConfigModule() *FortiOSConfigModule {
	return &FortiOSConfigModule{BaseProxyModule{name: "fortios_config"}}
}

// Execute runs the FortiOS config module.
func (m *FortiOSConfigModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	section, hasSection := m.GetString(mctx.Parameters, "section")
	name, _ := m.GetString(mctx.Parameters, "name")
	settings, hasSettings := mctx.Parameters["settings"].(map[string]interface{})
	backup, _ := m.GetBool(mctx.Parameters, "backup")

	if !hasSection {
		return nil, fmt.Errorf("section parameter is required")
	}

	if backup {
		backupResult, err := mctx.ExecuteCommand(ctx, "show full-configuration")
		if err == nil {
			result.Details["backup"] = backupResult.Stdout
		}
	}

	// Build FortiOS-style commands: config <section> / edit <name> / set k v / next / end
	var commands []string
	commands = append(commands, fmt.Sprintf("config %s", section))
	if name != "" {
		commands = append(commands, fmt.Sprintf("edit %s", name))
	}
	if hasSettings {
		for k, v := range settings {
			commands = append(commands, fmt.Sprintf("set %s %v", k, v))
		}
	}
	if name != "" {
		commands = append(commands, "next")
	}
	commands = append(commands, "end")

	// Check current config
	showResult, _ := mctx.ExecuteCommand(ctx, fmt.Sprintf("show %s", section))
	needsChange := true
	if hasSettings {
		needsChange = false
		for k, v := range settings {
			if !strings.Contains(string(showResult.Stdout), fmt.Sprintf("set %s %v", k, v)) {
				needsChange = true
				break
			}
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

	for _, cmd := range commands {
		if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
			_, _ = mctx.ExecuteCommand(ctx, "abort") //nolint:errcheck // best-effort abort
			return nil, fmt.Errorf("failed to execute '%s': %w", cmd, err)
		}
	}

	result.Comment = "Configuration applied"
	result.Details["commands"] = commands
	return result, nil
}

// Check performs a dry-run check.
func (m *FortiOSConfigModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// PANOSConfigModule manages Palo Alto PAN-OS configuration.
type PANOSConfigModule struct {
	BaseProxyModule
}

// NewPANOSConfigModule creates a new PAN-OS config module.
func NewPANOSConfigModule() *PANOSConfigModule {
	return &PANOSConfigModule{BaseProxyModule{name: "panos_config"}}
}

// Execute runs the PAN-OS config module.
func (m *PANOSConfigModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	lines, hasLines := m.GetStringSlice(mctx.Parameters, "lines")
	commit, _ := m.GetBool(mctx.Parameters, "commit")

	if !hasLines {
		return nil, fmt.Errorf("lines parameter is required")
	}

	// Enter configure mode
	if _, err := mctx.ExecuteCommand(ctx, "configure"); err != nil {
		return nil, fmt.Errorf("failed to enter configure mode: %w", err)
	}

	// Apply set commands
	for _, line := range lines {
		cmd := line
		if !strings.HasPrefix(strings.ToLower(line), "set ") &&
			!strings.HasPrefix(strings.ToLower(line), "delete ") {
			cmd = "set " + line
		}
		if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
			_, _ = mctx.ExecuteCommand(ctx, "exit") //nolint:errcheck // best-effort exit
			return nil, fmt.Errorf("failed to execute '%s': %w", cmd, err)
		}
	}

	result.Changed = true

	if mctx.DryRun {
		_, _ = mctx.ExecuteCommand(ctx, "exit") //nolint:errcheck // best-effort exit
		result.Comment = "Configuration would be committed"
		return result, nil
	}

	if commit {
		if _, err := mctx.ExecuteCommand(ctx, "commit"); err != nil {
			_, _ = mctx.ExecuteCommand(ctx, "exit") //nolint:errcheck // best-effort exit
			return nil, fmt.Errorf("commit failed: %w", err)
		}
		result.Comment = "Configuration committed"
	} else {
		result.Comment = "Configuration applied (not committed)"
	}

	_, _ = mctx.ExecuteCommand(ctx, "exit") //nolint:errcheck // best-effort exit
	return result, nil
}

// Check performs a dry-run check.
func (m *PANOSConfigModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// BigIPConfigModule manages F5 BIG-IP configuration.
type BigIPConfigModule struct {
	BaseProxyModule
}

// NewBigIPConfigModule creates a new BIG-IP config module.
func NewBigIPConfigModule() *BigIPConfigModule {
	return &BigIPConfigModule{BaseProxyModule{name: "bigip_config"}}
}

// Execute runs the BIG-IP config module.
func (m *BigIPConfigModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	commands, hasCommands := m.GetStringSlice(mctx.Parameters, "commands")
	save, _ := m.GetBool(mctx.Parameters, "save")

	if !hasCommands {
		return nil, fmt.Errorf("commands parameter is required")
	}

	result.Changed = true

	if mctx.DryRun {
		result.Comment = "Commands would be executed"
		result.Details["commands"] = commands
		return result, nil
	}

	// Execute tmsh commands directly
	for _, cmd := range commands {
		if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
			return nil, fmt.Errorf("failed to execute '%s': %w", cmd, err)
		}
	}

	if save {
		if _, err := mctx.ExecuteCommand(ctx, "save sys config"); err != nil {
			return nil, fmt.Errorf("failed to save configuration: %w", err)
		}
	}

	result.Comment = "Configuration applied"
	result.Details["commands"] = commands
	return result, nil
}

// Check performs a dry-run check.
func (m *BigIPConfigModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// =============================================================================
// P1/P2 Vendor Configuration Modules
// =============================================================================

// CheckpointGaiaConfigModule manages Check Point Gaia configuration.
type CheckpointGaiaConfigModule struct {
	BaseProxyModule
}

// NewCheckpointGaiaConfigModule creates a new Checkpoint Gaia config module.
func NewCheckpointGaiaConfigModule() *CheckpointGaiaConfigModule {
	return &CheckpointGaiaConfigModule{BaseProxyModule{name: "checkpoint_gaia_config"}}
}

// Execute runs the Checkpoint Gaia config module.
func (m *CheckpointGaiaConfigModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	commands, hasCommands := m.GetStringSlice(mctx.Parameters, "commands")
	save, _ := m.GetBool(mctx.Parameters, "save")

	if !hasCommands {
		return nil, fmt.Errorf("commands parameter is required")
	}

	result.Changed = true

	if mctx.DryRun {
		result.Comment = "Commands would be executed"
		result.Details["commands"] = commands
		return result, nil
	}

	for _, cmd := range commands {
		if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
			return nil, fmt.Errorf("failed to execute '%s': %w", cmd, err)
		}
	}

	if save {
		if _, err := mctx.ExecuteCommand(ctx, "save config"); err != nil {
			return nil, fmt.Errorf("failed to save configuration: %w", err)
		}
	}

	result.Comment = "Configuration applied"
	result.Details["commands"] = commands
	return result, nil
}

// Check performs a dry-run check.
func (m *CheckpointGaiaConfigModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// MikroTikRouterOSConfigModule manages MikroTik RouterOS configuration.
type MikroTikRouterOSConfigModule struct {
	BaseProxyModule
}

// NewMikroTikRouterOSConfigModule creates a new MikroTik RouterOS config module.
func NewMikroTikRouterOSConfigModule() *MikroTikRouterOSConfigModule {
	return &MikroTikRouterOSConfigModule{BaseProxyModule{name: "mikrotik_routeros_config"}}
}

// Execute runs the MikroTik RouterOS config module.
func (m *MikroTikRouterOSConfigModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	commands, hasCommands := m.GetStringSlice(mctx.Parameters, "commands")

	if !hasCommands {
		return nil, fmt.Errorf("commands parameter is required")
	}

	result.Changed = true

	if mctx.DryRun {
		result.Comment = "Commands would be executed"
		result.Details["commands"] = commands
		return result, nil
	}

	for _, cmd := range commands {
		if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
			return nil, fmt.Errorf("failed to execute '%s': %w", cmd, err)
		}
	}

	// RouterOS auto-saves; no explicit save needed
	result.Comment = "Configuration applied"
	result.Details["commands"] = commands
	return result, nil
}

// Check performs a dry-run check.
func (m *MikroTikRouterOSConfigModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// UbiquitiEdgeOSConfigModule manages Ubiquiti EdgeOS configuration.
type UbiquitiEdgeOSConfigModule struct {
	BaseProxyModule
}

// NewUbiquitiEdgeOSConfigModule creates a new Ubiquiti EdgeOS config module.
func NewUbiquitiEdgeOSConfigModule() *UbiquitiEdgeOSConfigModule {
	return &UbiquitiEdgeOSConfigModule{BaseProxyModule{name: "ubiquiti_edgeos_config"}}
}

// Execute runs the Ubiquiti EdgeOS config module.
func (m *UbiquitiEdgeOSConfigModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	commands, hasCommands := m.GetStringSlice(mctx.Parameters, "commands")
	commit, _ := m.GetBool(mctx.Parameters, "commit")
	save, _ := m.GetBool(mctx.Parameters, "save")

	if !hasCommands {
		return nil, fmt.Errorf("commands parameter is required")
	}

	result.Changed = true

	if mctx.DryRun {
		result.Comment = "Commands would be executed"
		result.Details["commands"] = commands
		return result, nil
	}

	// Enter configure mode
	if _, err := mctx.ExecuteCommand(ctx, "configure"); err != nil {
		return nil, fmt.Errorf("failed to enter configure mode: %w", err)
	}

	for _, cmd := range commands {
		c := cmd
		if !strings.HasPrefix(strings.ToLower(c), "set ") &&
			!strings.HasPrefix(strings.ToLower(c), "delete ") {
			c = "set " + c
		}
		if _, err := mctx.ExecuteCommand(ctx, c); err != nil {
			_, _ = mctx.ExecuteCommand(ctx, "discard")
			_, _ = mctx.ExecuteCommand(ctx, "exit")
			return nil, fmt.Errorf("failed to execute '%s': %w", c, err)
		}
	}

	if commit {
		if _, err := mctx.ExecuteCommand(ctx, "commit"); err != nil {
			_, _ = mctx.ExecuteCommand(ctx, "discard")
			_, _ = mctx.ExecuteCommand(ctx, "exit")
			return nil, fmt.Errorf("commit failed: %w", err)
		}
	}

	if save {
		if _, err := mctx.ExecuteCommand(ctx, "save"); err != nil {
			return nil, fmt.Errorf("failed to save configuration: %w", err)
		}
	}

	_, _ = mctx.ExecuteCommand(ctx, "exit")
	result.Comment = "Configuration applied"
	result.Details["commands"] = commands
	return result, nil
}

// Check performs a dry-run check.
func (m *UbiquitiEdgeOSConfigModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// ExtremeEXOSConfigModule manages Extreme EXOS configuration.
type ExtremeEXOSConfigModule struct {
	BaseProxyModule
}

// NewExtremeEXOSConfigModule creates a new Extreme EXOS config module.
func NewExtremeEXOSConfigModule() *ExtremeEXOSConfigModule {
	return &ExtremeEXOSConfigModule{BaseProxyModule{name: "extreme_exos_config"}}
}

// Execute runs the Extreme EXOS config module.
func (m *ExtremeEXOSConfigModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	commands, hasCommands := m.GetStringSlice(mctx.Parameters, "commands")
	save, _ := m.GetBool(mctx.Parameters, "save")

	if !hasCommands {
		return nil, fmt.Errorf("commands parameter is required")
	}

	result.Changed = true

	if mctx.DryRun {
		result.Comment = "Commands would be executed"
		result.Details["commands"] = commands
		return result, nil
	}

	for _, cmd := range commands {
		if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
			return nil, fmt.Errorf("failed to execute '%s': %w", cmd, err)
		}
	}

	if save {
		if _, err := mctx.ExecuteCommand(ctx, "save configuration primary"); err != nil {
			return nil, fmt.Errorf("failed to save configuration: %w", err)
		}
	}

	result.Comment = "Configuration applied"
	result.Details["commands"] = commands
	return result, nil
}

// Check performs a dry-run check.
func (m *ExtremeEXOSConfigModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// NokiaSROSConfigModule manages Nokia SR OS configuration.
type NokiaSROSConfigModule struct {
	BaseProxyModule
}

// NewNokiaSROSConfigModule creates a new Nokia SR OS config module.
func NewNokiaSROSConfigModule() *NokiaSROSConfigModule {
	return &NokiaSROSConfigModule{BaseProxyModule{name: "nokia_sros_config"}}
}

// Execute runs the Nokia SR OS config module.
func (m *NokiaSROSConfigModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	commands, hasCommands := m.GetStringSlice(mctx.Parameters, "commands")
	save, _ := m.GetBool(mctx.Parameters, "save")

	if !hasCommands {
		return nil, fmt.Errorf("commands parameter is required")
	}

	result.Changed = true

	if mctx.DryRun {
		result.Comment = "Commands would be executed"
		result.Details["commands"] = commands
		return result, nil
	}

	// Enter configure mode
	if _, err := mctx.ExecuteCommand(ctx, "configure"); err != nil {
		return nil, fmt.Errorf("failed to enter configure mode: %w", err)
	}

	for _, cmd := range commands {
		if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
			_, _ = mctx.ExecuteCommand(ctx, "exit all")
			return nil, fmt.Errorf("failed to execute '%s': %w", cmd, err)
		}
	}

	_, _ = mctx.ExecuteCommand(ctx, "exit all")

	if save {
		if _, err := mctx.ExecuteCommand(ctx, "admin save"); err != nil {
			return nil, fmt.Errorf("failed to save configuration: %w", err)
		}
	}

	result.Comment = "Configuration applied"
	result.Details["commands"] = commands
	return result, nil
}

// Check performs a dry-run check.
func (m *NokiaSROSConfigModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// HuaweiVRPConfigModule manages Huawei VRP configuration.
type HuaweiVRPConfigModule struct {
	BaseProxyModule
}

// NewHuaweiVRPConfigModule creates a new Huawei VRP config module.
func NewHuaweiVRPConfigModule() *HuaweiVRPConfigModule {
	return &HuaweiVRPConfigModule{BaseProxyModule{name: "huawei_vrp_config"}}
}

// Execute runs the Huawei VRP config module.
func (m *HuaweiVRPConfigModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	lines, hasLines := m.GetStringSlice(mctx.Parameters, "lines")
	parents, _ := m.GetStringSlice(mctx.Parameters, "parents")
	save, _ := m.GetBool(mctx.Parameters, "save")

	if !hasLines {
		return nil, fmt.Errorf("lines parameter is required")
	}

	commands := make([]string, 0, 2+len(parents)+len(lines))
	commands = append(commands, "system-view")
	commands = append(commands, parents...)
	commands = append(commands, lines...)
	commands = append(commands, "return")

	runningResult, _ := mctx.ExecuteCommand(ctx, "display current-configuration")
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

	for _, cmd := range commands {
		if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
			return nil, fmt.Errorf("failed to execute '%s': %w", cmd, err)
		}
	}

	if save {
		_, _ = mctx.ExecuteCommand(ctx, "save")
		if _, err := mctx.ExecuteCommand(ctx, "Y"); err != nil {
			return nil, fmt.Errorf("failed to save configuration: %w", err)
		}
	}

	result.Comment = "Configuration applied"
	result.Details["commands"] = commands
	return result, nil
}

// Check performs a dry-run check.
func (m *HuaweiVRPConfigModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// MellanoxOnyxConfigModule manages Mellanox/NVIDIA Onyx configuration.
type MellanoxOnyxConfigModule struct {
	BaseProxyModule
}

// NewMellanoxOnyxConfigModule creates a new Mellanox Onyx config module.
func NewMellanoxOnyxConfigModule() *MellanoxOnyxConfigModule {
	return &MellanoxOnyxConfigModule{BaseProxyModule{name: "mellanox_onyx_config"}}
}

// Execute runs the Mellanox Onyx config module.
func (m *MellanoxOnyxConfigModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	lines, hasLines := m.GetStringSlice(mctx.Parameters, "lines")
	parents, _ := m.GetStringSlice(mctx.Parameters, "parents")
	save, _ := m.GetBool(mctx.Parameters, "save")

	if !hasLines {
		return nil, fmt.Errorf("lines parameter is required")
	}

	commands := make([]string, 0, 2+len(parents)+len(lines))
	commands = append(commands, "configure terminal")
	commands = append(commands, parents...)
	commands = append(commands, lines...)
	commands = append(commands, "end")

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

	for _, cmd := range commands {
		if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
			return nil, fmt.Errorf("failed to execute '%s': %w", cmd, err)
		}
	}

	if save {
		if _, err := mctx.ExecuteCommand(ctx, "configuration write"); err != nil {
			return nil, fmt.Errorf("failed to save configuration: %w", err)
		}
	}

	result.Comment = "Configuration applied"
	result.Details["commands"] = commands
	return result, nil
}

// Check performs a dry-run check.
func (m *MellanoxOnyxConfigModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// AlliedTelesisAWPlusConfigModule manages Allied Telesis AlliedWare Plus configuration.
type AlliedTelesisAWPlusConfigModule struct {
	BaseProxyModule
}

// NewAlliedTelesisAWPlusConfigModule creates a new Allied Telesis AlliedWare Plus config module.
func NewAlliedTelesisAWPlusConfigModule() *AlliedTelesisAWPlusConfigModule {
	return &AlliedTelesisAWPlusConfigModule{BaseProxyModule{name: "alliedtelesis_awplus_config"}}
}

// Execute runs the Allied Telesis AlliedWare Plus config module.
func (m *AlliedTelesisAWPlusConfigModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	lines, hasLines := m.GetStringSlice(mctx.Parameters, "lines")
	parents, _ := m.GetStringSlice(mctx.Parameters, "parents")
	save, _ := m.GetBool(mctx.Parameters, "save")

	if !hasLines {
		return nil, fmt.Errorf("lines parameter is required")
	}

	commands := make([]string, 0, 2+len(parents)+len(lines))
	commands = append(commands, "configure terminal")
	commands = append(commands, parents...)
	commands = append(commands, lines...)
	commands = append(commands, "end")

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

	for _, cmd := range commands {
		if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
			return nil, fmt.Errorf("failed to execute '%s': %w", cmd, err)
		}
	}

	if save {
		if _, err := mctx.ExecuteCommand(ctx, "write"); err != nil {
			return nil, fmt.Errorf("failed to save configuration: %w", err)
		}
	}

	result.Comment = "Configuration applied"
	result.Details["commands"] = commands
	return result, nil
}

// Check performs a dry-run check.
func (m *AlliedTelesisAWPlusConfigModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// CienaSAOSConfigModule manages Ciena SAOS configuration.
type CienaSAOSConfigModule struct {
	BaseProxyModule
}

// NewCienaSAOSConfigModule creates a new Ciena SAOS config module.
func NewCienaSAOSConfigModule() *CienaSAOSConfigModule {
	return &CienaSAOSConfigModule{BaseProxyModule{name: "ciena_saos_config"}}
}

// Execute runs the Ciena SAOS config module.
func (m *CienaSAOSConfigModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	commands, hasCommands := m.GetStringSlice(mctx.Parameters, "commands")
	save, _ := m.GetBool(mctx.Parameters, "save")

	if !hasCommands {
		return nil, fmt.Errorf("commands parameter is required")
	}

	result.Changed = true

	if mctx.DryRun {
		result.Comment = "Commands would be executed"
		result.Details["commands"] = commands
		return result, nil
	}

	for _, cmd := range commands {
		if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
			return nil, fmt.Errorf("failed to execute '%s': %w", cmd, err)
		}
	}

	if save {
		if _, err := mctx.ExecuteCommand(ctx, "configuration save"); err != nil {
			return nil, fmt.Errorf("failed to save configuration: %w", err)
		}
	}

	result.Comment = "Configuration applied"
	result.Details["commands"] = commands
	return result, nil
}

// Check performs a dry-run check.
func (m *CienaSAOSConfigModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}
