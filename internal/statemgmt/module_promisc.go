package statemgmt

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// PromiscModule implements promiscuous mode management
type PromiscModule struct {
	*BaseModule
}

// NewPromiscModule creates a new promiscuous mode module
func NewPromiscModule() *PromiscModule {
	return &PromiscModule{
		BaseModule: NewBaseModule("promisc", []string{"enabled", "disabled"}),
	}
}

// PromiscConfig holds promiscuous mode configuration parameters
type PromiscConfig struct {
	Interface string // Network interface (required)
	AllMulti  bool   // Also enable all-multicast mode (Linux only)
}

// PromiscBackend represents the available promiscuous mode backend
type PromiscBackend string

// PBUnknown and related constants.
const (
	PBUnknown  PromiscBackend = "unknown"
	PBIPLink   PromiscBackend = "ip_link"  // Linux ip command
	PBIfconfig PromiscBackend = "ifconfig" // macOS/BSD ifconfig
	PBNetsh    PromiscBackend = "netsh"    // Windows netsh
)

// Check checks the current promiscuous mode state
func (m *PromiscModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	config, err := m.parsePromiscConfig(decl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse promisc config: %w", err)
	}

	backend, err := m.detectPromiscBackend()
	if err != nil {
		return nil, fmt.Errorf("failed to detect promisc backend: %w", err)
	}
	result.Metadata["backend"] = string(backend)
	result.Metadata["interface"] = config.Interface

	// Check if interface exists
	exists, err := m.checkInterfaceExists(ctx, backend, config.Interface)
	if err != nil {
		return nil, fmt.Errorf("failed to check interface: %w", err)
	}

	if !exists {
		result.Present = false
		result.CurrentState = "absent"
		result.Matches = false
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	result.Present = true

	// Get current promiscuous mode state
	isPromisc, isAllMulti, err := m.getPromiscState(ctx, backend, config.Interface)
	if err != nil {
		return nil, fmt.Errorf("failed to get promisc state: %w", err)
	}

	result.Metadata["promiscuous"] = isPromisc
	result.Metadata["allmulti"] = isAllMulti

	if isPromisc {
		result.CurrentState = "enabled"
	} else {
		result.CurrentState = "disabled"
	}

	// Check if matches desired state
	result.Matches = (result.CurrentState == decl.State)

	// Check allmulti if specified and on Linux
	if config.AllMulti && backend == PBIPLink && decl.State == "enabled" {
		if !isAllMulti {
			result.Matches = false
		}
	}

	if !result.Matches {
		result.Diff["promiscuous"] = map[string]interface{}{
			"from": isPromisc,
			"to":   decl.State == "enabled",
		}
		if config.AllMulti && backend == PBIPLink {
			result.Diff["allmulti"] = map[string]interface{}{
				"from": isAllMulti,
				"to":   decl.State == "enabled",
			}
		}
	}

	return result, nil //nolint:nilerr // error captured in result.Error
}

// Apply applies the promiscuous mode configuration
func (m *PromiscModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	result := &StateResult{
		StateID:   decl.ID,
		Module:    m.Name(),
		StartTime: startTime,
	}

	config, err := m.parsePromiscConfig(decl)
	if err != nil {
		result.Success = false
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to parse config: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	if err := m.validatePromiscConfig(config); err != nil {
		result.Success = false
		result.Error = err
		result.Comment = fmt.Sprintf("Invalid config: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	backend, err := m.detectPromiscBackend()
	if err != nil {
		result.Success = false
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to detect promisc backend: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	// Check current state
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to check current state: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	// If already in desired state, no changes needed
	if checkResult.Matches {
		result.Success = true
		result.Changed = false
		result.Comment = "Already in desired state"
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	// Apply changes
	var applyErr error
	switch decl.State {
	case "enabled":
		applyErr = m.enablePromisc(ctx, backend, config, result)
	case "disabled":
		applyErr = m.disablePromisc(ctx, backend, config, result)
	default:
		applyErr = fmt.Errorf("unknown state: %s", decl.State)
	}

	if applyErr != nil {
		result.Success = false
		result.Error = applyErr
		result.Comment = fmt.Sprintf("Failed to apply state: %v", applyErr)
	} else {
		result.Success = true
		result.Changed = true
		result.Changes = checkResult.Diff
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	return result, nil //nolint:nilerr // error captured in result.Error
}

// Test verifies the promiscuous mode matches the desired state
func (m *PromiscModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil //nolint:nilerr // intentional
}

// parsePromiscConfig parses the state declaration into PromiscConfig
func (m *PromiscModule) parsePromiscConfig(decl *StateDeclaration) (*PromiscConfig, error) {
	config := &PromiscConfig{}

	// Interface (required) - can come from ID or parameter
	if v, ok := decl.Parameters["interface"].(string); ok && v != "" {
		config.Interface = v
	} else {
		config.Interface = decl.ID
	}
	if config.Interface == "" {
		return nil, fmt.Errorf("interface is required")
	}

	// AllMulti (optional, Linux only)
	if v, ok := decl.Parameters["allmulti"].(bool); ok {
		config.AllMulti = v
	} else if v, ok := decl.Parameters["all_multicast"].(bool); ok {
		config.AllMulti = v
	}

	return config, nil //nolint:nilerr // intentional
}

// validatePromiscConfig validates the promiscuous mode configuration
func (m *PromiscModule) validatePromiscConfig(config *PromiscConfig) error {
	if config.Interface == "" {
		return fmt.Errorf("interface is required")
	}
	return nil
}

// detectPromiscBackend detects the available promiscuous mode backend
func (m *PromiscModule) detectPromiscBackend() (PromiscBackend, error) {
	switch runtime.GOOS {
	case "linux":
		if _, err := exec.LookPath("ip"); err == nil {
			return PBIPLink, nil //nolint:nilerr // intentional
		}
		return PBUnknown, fmt.Errorf("ip command not found")

	case "darwin", "freebsd", "openbsd", "netbsd":
		return PBIfconfig, nil //nolint:nilerr // intentional

	case "windows":
		return PBNetsh, nil //nolint:nilerr // intentional

	default:
		return PBUnknown, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// checkInterfaceExists checks if the interface exists
func (m *PromiscModule) checkInterfaceExists(ctx context.Context, backend PromiscBackend, iface string) (bool, error) {
	switch backend {
	case PBIPLink:
		cmd := exec.CommandContext(ctx, "ip", "link", "show", iface)
		err := cmd.Run()
		return err == nil, nil //nolint:nilerr // intentional

	case PBIfconfig:
		cmd := exec.CommandContext(ctx, "ifconfig", iface)
		err := cmd.Run()
		return err == nil, nil //nolint:nilerr // intentional

	case PBNetsh:
		cmd := exec.CommandContext(ctx, "netsh", "interface", "show", "interface", iface)
		err := cmd.Run()
		return err == nil, nil //nolint:nilerr // intentional

	default:
		return false, fmt.Errorf("unsupported backend: %s", backend)
	}
}

// getPromiscState gets the current promiscuous mode state
func (m *PromiscModule) getPromiscState(ctx context.Context, backend PromiscBackend, iface string) (promisc, allmulti bool, err error) {
	switch backend {
	case PBIPLink:
		return m.getPromiscStateLinux(ctx, iface)
	case PBIfconfig:
		return m.getPromiscStateMacOS(ctx, iface)
	case PBNetsh:
		return m.getPromiscStateWindows(ctx, iface)
	default:
		return false, false, fmt.Errorf("unsupported backend: %s", backend)
	}
}

// enablePromisc enables promiscuous mode
func (m *PromiscModule) enablePromisc(ctx context.Context, backend PromiscBackend, config *PromiscConfig, result *StateResult) error {
	switch backend {
	case PBIPLink:
		return m.enablePromiscLinux(ctx, config, result)
	case PBIfconfig:
		return m.enablePromiscMacOS(ctx, config, result)
	case PBNetsh:
		return m.enablePromiscWindows(ctx, config, result)
	default:
		return fmt.Errorf("unsupported backend: %s", backend)
	}
}

// disablePromisc disables promiscuous mode
func (m *PromiscModule) disablePromisc(ctx context.Context, backend PromiscBackend, config *PromiscConfig, result *StateResult) error {
	switch backend {
	case PBIPLink:
		return m.disablePromiscLinux(ctx, config, result)
	case PBIfconfig:
		return m.disablePromiscMacOS(ctx, config, result)
	case PBNetsh:
		return m.disablePromiscWindows(ctx, config, result)
	default:
		return fmt.Errorf("unsupported backend: %s", backend)
	}
}

// ============================================================================
// Linux Backend (ip link)
// ============================================================================

func (m *PromiscModule) getPromiscStateLinux(ctx context.Context, iface string) (promisc, allmulti bool, err error) {
	cmd := exec.CommandContext(ctx, "ip", "link", "show", iface)
	output, err := cmd.Output()
	if err != nil {
		return false, false, fmt.Errorf("failed to get interface info: %w", err)
	}

	outputStr := string(output)

	// Check for PROMISC flag
	// Format: "2: eth0: <BROADCAST,MULTICAST,PROMISC,UP,LOWER_UP> ..."
	promisc = strings.Contains(outputStr, "PROMISC")

	// Check for ALLMULTI flag
	allmulti = strings.Contains(outputStr, "ALLMULTI")

	return promisc, allmulti, nil //nolint:nilerr // returning parsed interface flags, no error
}

func (m *PromiscModule) enablePromiscLinux(ctx context.Context, config *PromiscConfig, result *StateResult) error {
	// Enable promiscuous mode
	cmd := exec.CommandContext(ctx, "ip", "link", "set", config.Interface, "promisc", "on")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to enable promiscuous mode: %w: %s", err, output)
	}

	changes := []string{"promisc=on"}

	// Enable all-multicast if requested
	if config.AllMulti {
		cmd = exec.CommandContext(ctx, "ip", "link", "set", config.Interface, "allmulticast", "on")
		output, err = cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to enable allmulticast: %w: %s", err, output)
		}
		changes = append(changes, "allmulti=on")
	}

	result.Comment = fmt.Sprintf("Enabled %s on %s", strings.Join(changes, ", "), config.Interface)
	return nil
}

func (m *PromiscModule) disablePromiscLinux(ctx context.Context, config *PromiscConfig, result *StateResult) error {
	// Disable promiscuous mode
	cmd := exec.CommandContext(ctx, "ip", "link", "set", config.Interface, "promisc", "off")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to disable promiscuous mode: %w: %s", err, output)
	}

	changes := []string{"promisc=off"}

	// Disable all-multicast if it was configured
	if config.AllMulti {
		cmd = exec.CommandContext(ctx, "ip", "link", "set", config.Interface, "allmulticast", "off")
		output, err = cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to disable allmulticast: %w: %s", err, output)
		}
		changes = append(changes, "allmulti=off")
	}

	result.Comment = fmt.Sprintf("Disabled %s on %s", strings.Join(changes, ", "), config.Interface)
	return nil
}

// ============================================================================
// macOS/BSD Backend (ifconfig)
// ============================================================================

func (m *PromiscModule) getPromiscStateMacOS(ctx context.Context, iface string) (promisc, allmulti bool, err error) {
	cmd := exec.CommandContext(ctx, "ifconfig", iface)
	output, err := cmd.Output()
	if err != nil {
		return false, false, fmt.Errorf("failed to get interface info: %w", err)
	}

	outputStr := string(output)

	// Check flags line: "flags=8863<UP,BROADCAST,SMART,RUNNING,SIMPLEX,MULTICAST,PROMISC>"
	flagsRegex := regexp.MustCompile(`flags=\d+<([^>]+)>`)
	matches := flagsRegex.FindStringSubmatch(outputStr)
	if len(matches) < 2 {
		return false, false, nil //nolint:nilerr // flags line not found, assume disabled
	}

	flags := matches[1]
	promisc = strings.Contains(flags, "PROMISC")
	allmulti = strings.Contains(flags, "ALLMULTI")

	return promisc, allmulti, nil //nolint:nilerr // returning parsed interface flags, no error
}

func (m *PromiscModule) enablePromiscMacOS(ctx context.Context, config *PromiscConfig, result *StateResult) error {
	// Enable promiscuous mode
	cmd := exec.CommandContext(ctx, "ifconfig", config.Interface, "promisc")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to enable promiscuous mode: %w: %s", err, output)
	}

	result.Comment = fmt.Sprintf("Enabled promiscuous mode on %s", config.Interface)
	return nil
}

func (m *PromiscModule) disablePromiscMacOS(ctx context.Context, config *PromiscConfig, result *StateResult) error {
	// Disable promiscuous mode
	cmd := exec.CommandContext(ctx, "ifconfig", config.Interface, "-promisc")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to disable promiscuous mode: %w: %s", err, output)
	}

	result.Comment = fmt.Sprintf("Disabled promiscuous mode on %s", config.Interface)
	return nil
}

// ============================================================================
// Windows Backend (netsh/PowerShell)
// ============================================================================

func (m *PromiscModule) getPromiscStateWindows(ctx context.Context, iface string) (promisc, allmulti bool, err error) {
	// Windows doesn't have a direct promiscuous mode flag exposed via netsh
	// We need to check via PowerShell/WMI or the adapter's advanced properties
	// Note: Windows typically enables promiscuous mode at the application level (e.g., WinPcap)

	// Check if the interface has promiscuous mode capability
	psCmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command",
		fmt.Sprintf(`
$adapter = Get-NetAdapter -Name '%s' -ErrorAction SilentlyContinue
if ($adapter) {
    $props = Get-NetAdapterAdvancedProperty -Name '%s' -RegistryKeyword '*PromiscuousMode' -ErrorAction SilentlyContinue
    if ($props -and $props.RegistryValue -eq '1') {
        Write-Output 'enabled'
    } else {
        Write-Output 'disabled'
    }
} else {
    Write-Output 'not_found'
}
`, iface, iface))

	output, err := psCmd.Output()
	if err != nil {
		// If the advanced property doesn't exist, assume disabled
		return false, false, nil //nolint:nilerr // property not existing is a valid state
	}

	outputStr := strings.TrimSpace(string(output))
	promisc = outputStr == "enabled"

	return promisc, false, nil //nolint:nilerr // returning parsed promiscuous state, no error
}

func (m *PromiscModule) enablePromiscWindows(ctx context.Context, config *PromiscConfig, result *StateResult) error {
	// Try to enable via advanced property if available
	psCmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command",
		fmt.Sprintf(`
$adapter = Get-NetAdapter -Name '%s' -ErrorAction Stop
# Try to set promiscuous mode via advanced property
try {
    Set-NetAdapterAdvancedProperty -Name '%s' -RegistryKeyword '*PromiscuousMode' -RegistryValue '1' -ErrorAction Stop
    Write-Output 'success'
} catch {
    # If the property doesn't exist, we need to use a different approach
    # Some adapters don't expose this directly
    Write-Output 'property_not_supported'
}
`, config.Interface, config.Interface))

	output, err := psCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to enable promiscuous mode: %w: %s", err, output)
	}

	outputStr := strings.TrimSpace(string(output))
	if outputStr == "property_not_supported" {
		// Note: On Windows, promiscuous mode is typically enabled at the application level
		// (e.g., using WinPcap/Npcap). The adapter property may not be available on all NICs.
		result.Comment = fmt.Sprintf("Promiscuous mode property not supported on %s; use WinPcap/Npcap at application level", config.Interface)
		return nil
	}

	result.Comment = fmt.Sprintf("Enabled promiscuous mode on %s", config.Interface)
	return nil
}

func (m *PromiscModule) disablePromiscWindows(ctx context.Context, config *PromiscConfig, result *StateResult) error {
	// Try to disable via advanced property
	psCmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command",
		fmt.Sprintf(`
try {
    Set-NetAdapterAdvancedProperty -Name '%s' -RegistryKeyword '*PromiscuousMode' -RegistryValue '0' -ErrorAction Stop
    Write-Output 'success'
} catch {
    Write-Output 'property_not_supported'
}
`, config.Interface))

	output, err := psCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to disable promiscuous mode: %w: %s", err, output)
	}

	outputStr := strings.TrimSpace(string(output))
	if outputStr == "property_not_supported" {
		result.Comment = fmt.Sprintf("Promiscuous mode property not supported on %s", config.Interface)
		return nil
	}

	result.Comment = fmt.Sprintf("Disabled promiscuous mode on %s", config.Interface)
	return nil
}

func init() {
	_ = RegisterModule(NewPromiscModule()) //nolint:errcheck // module registration in init
}
