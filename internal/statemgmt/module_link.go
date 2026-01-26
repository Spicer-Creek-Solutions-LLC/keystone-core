package statemgmt

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// LinkModule implements network link settings management
type LinkModule struct {
	*BaseModule
}

// NewLinkModule creates a new link settings module
func NewLinkModule() *LinkModule {
	return &LinkModule{
		BaseModule: NewBaseModule("link", []string{"configured", "default"}),
	}
}

// LinkConfig holds link configuration parameters
type LinkConfig struct {
	Interface string // Network interface (required)
	Speed     int    // Link speed in Mbps: 10, 100, 1000, 2500, 5000, 10000, 25000, 40000, 100000
	Duplex    string // Duplex mode: full, half
	Autoneg   *bool  // Auto-negotiation: true/false (nil = don't change)
	MTU       int    // MTU size (optional)
	WOL       string // Wake-on-LAN mode: disabled, magic, unicast, multicast, broadcast, arp
}

// LinkBackend represents the available link management backend
type LinkBackend string

const (
	LBUnknown   LinkBackend = "unknown"
	LBEthtool   LinkBackend = "ethtool"   // Linux
	LBNetworkSetup LinkBackend = "networksetup" // macOS
	LBNetsh     LinkBackend = "netsh"     // Windows
)

// Valid link speeds in Mbps
var validLinkSpeeds = map[int]bool{
	10:     true,
	100:    true,
	1000:   true,
	2500:   true,
	5000:   true,
	10000:  true,
	25000:  true,
	40000:  true,
	100000: true,
}

// Valid duplex modes
var validDuplexModes = map[string]bool{
	"full": true,
	"half": true,
	"":     true, // optional
}

// Valid Wake-on-LAN modes
var validWOLModes = map[string]bool{
	"disabled":  true,
	"magic":     true,
	"unicast":   true,
	"multicast": true,
	"broadcast": true,
	"arp":       true,
	"":          true, // optional
}

// Check checks the current link settings
func (m *LinkModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	config, err := m.parseLinkConfig(decl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse link config: %w", err)
	}

	backend, err := m.detectLinkBackend()
	if err != nil {
		return nil, fmt.Errorf("failed to detect link backend: %w", err)
	}
	result.Metadata["backend"] = string(backend)
	result.Metadata["interface"] = config.Interface

	// Check if interface exists
	exists, err := m.checkInterfaceExists(ctx, backend, config.Interface)
	if err != nil {
		return nil, fmt.Errorf("failed to check interface: %w", err)
	}

	result.Present = exists
	if !exists {
		result.CurrentState = "absent"
		result.Matches = (decl.State == "default")
		return result, nil
	}

	// Get current link settings
	currentSpeed, currentDuplex, currentAutoneg, currentMTU, err := m.getLinkSettings(ctx, backend, config.Interface)
	if err != nil {
		return nil, fmt.Errorf("failed to get link settings: %w", err)
	}

	result.Metadata["current_speed"] = currentSpeed
	result.Metadata["current_duplex"] = currentDuplex
	result.Metadata["current_autoneg"] = currentAutoneg
	result.Metadata["current_mtu"] = currentMTU

	// For "default" state, we're resetting to auto-negotiation
	if decl.State == "default" {
		if !currentAutoneg {
			result.Diff["autoneg"] = map[string]interface{}{
				"from": false,
				"to":   true,
			}
		}
		result.CurrentState = "configured"
		result.Matches = currentAutoneg
		return result, nil
	}

	// For "configured" state, check each setting
	result.CurrentState = "configured"
	result.Matches = true

	if config.Speed > 0 && config.Speed != currentSpeed {
		result.Diff["speed"] = map[string]interface{}{
			"from": currentSpeed,
			"to":   config.Speed,
		}
		result.Matches = false
	}

	if config.Duplex != "" && config.Duplex != currentDuplex {
		result.Diff["duplex"] = map[string]interface{}{
			"from": currentDuplex,
			"to":   config.Duplex,
		}
		result.Matches = false
	}

	if config.Autoneg != nil && *config.Autoneg != currentAutoneg {
		result.Diff["autoneg"] = map[string]interface{}{
			"from": currentAutoneg,
			"to":   *config.Autoneg,
		}
		result.Matches = false
	}

	if config.MTU > 0 && config.MTU != currentMTU {
		result.Diff["mtu"] = map[string]interface{}{
			"from": currentMTU,
			"to":   config.MTU,
		}
		result.Matches = false
	}

	return result, nil
}

// Apply applies the link configuration
func (m *LinkModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	result := &StateResult{
		StateID:   decl.ID,
		Module:    m.Name(),
		StartTime: startTime,
	}

	config, err := m.parseLinkConfig(decl)
	if err != nil {
		result.Success = false
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to parse config: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	if err := m.validateLinkConfig(config, decl.State); err != nil {
		result.Success = false
		result.Error = err
		result.Comment = fmt.Sprintf("Invalid config: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	backend, err := m.detectLinkBackend()
	if err != nil {
		result.Success = false
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to detect link backend: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
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

	// Apply changes
	var applyErr error
	switch decl.State {
	case "configured":
		applyErr = m.configureLinkSettings(ctx, backend, config, result)
	case "default":
		applyErr = m.resetLinkSettings(ctx, backend, config, result)
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
	return result, nil
}

// Test verifies the link settings match the desired state
func (m *LinkModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil
}

// parseLinkConfig parses the state declaration into LinkConfig
func (m *LinkModule) parseLinkConfig(decl *StateDeclaration) (*LinkConfig, error) {
	config := &LinkConfig{}

	// Interface (required) - can come from ID or parameter
	if v, ok := decl.Parameters["interface"].(string); ok && v != "" {
		config.Interface = v
	} else {
		config.Interface = decl.ID
	}
	if config.Interface == "" {
		return nil, fmt.Errorf("interface is required")
	}

	// Speed
	if v, ok := decl.Parameters["speed"].(int); ok {
		config.Speed = v
	} else if v, ok := decl.Parameters["speed"].(float64); ok {
		config.Speed = int(v)
	}

	// Duplex
	if v, ok := decl.Parameters["duplex"].(string); ok {
		config.Duplex = strings.ToLower(v)
	}

	// Auto-negotiation
	if v, ok := decl.Parameters["autoneg"].(bool); ok {
		config.Autoneg = &v
	} else if v, ok := decl.Parameters["auto_negotiation"].(bool); ok {
		config.Autoneg = &v
	}

	// MTU
	if v, ok := decl.Parameters["mtu"].(int); ok {
		config.MTU = v
	} else if v, ok := decl.Parameters["mtu"].(float64); ok {
		config.MTU = int(v)
	}

	// Wake-on-LAN
	if v, ok := decl.Parameters["wol"].(string); ok {
		config.WOL = strings.ToLower(v)
	} else if v, ok := decl.Parameters["wake_on_lan"].(string); ok {
		config.WOL = strings.ToLower(v)
	}

	return config, nil
}

// validateLinkConfig validates the link configuration
func (m *LinkModule) validateLinkConfig(config *LinkConfig, state string) error {
	// Interface is always required
	if config.Interface == "" {
		return fmt.Errorf("interface is required")
	}

	// For default state, no other validation needed
	if state == "default" {
		return nil
	}

	// Validate speed
	if config.Speed > 0 && !validLinkSpeeds[config.Speed] {
		return fmt.Errorf("invalid speed: %d Mbps (valid: 10, 100, 1000, 2500, 5000, 10000, 25000, 40000, 100000)", config.Speed)
	}

	// Validate duplex
	if !validDuplexModes[config.Duplex] {
		return fmt.Errorf("invalid duplex: %s (valid: full, half)", config.Duplex)
	}

	// Validate MTU
	if config.MTU > 0 && (config.MTU < 68 || config.MTU > 65535) {
		return fmt.Errorf("invalid MTU: %d (valid range: 68-65535)", config.MTU)
	}

	// Validate WOL
	if !validWOLModes[config.WOL] {
		return fmt.Errorf("invalid WOL mode: %s (valid: disabled, magic, unicast, multicast, broadcast, arp)", config.WOL)
	}

	// If forcing speed/duplex, auto-negotiation should typically be disabled
	if (config.Speed > 0 || config.Duplex != "") && config.Autoneg != nil && *config.Autoneg {
		// This is a warning condition, but we'll allow it
		// Some NICs support advertising only specific speeds
	}

	return nil
}

// detectLinkBackend detects the available link management backend
func (m *LinkModule) detectLinkBackend() (LinkBackend, error) {
	switch runtime.GOOS {
	case "linux":
		if _, err := exec.LookPath("ethtool"); err == nil {
			return LBEthtool, nil
		}
		return LBUnknown, fmt.Errorf("ethtool not found (install ethtool package)")

	case "darwin":
		return LBNetworkSetup, nil

	case "windows":
		return LBNetsh, nil

	default:
		return LBUnknown, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// checkInterfaceExists checks if the interface exists
func (m *LinkModule) checkInterfaceExists(ctx context.Context, backend LinkBackend, iface string) (bool, error) {
	switch backend {
	case LBEthtool:
		cmd := exec.CommandContext(ctx, "ip", "link", "show", iface)
		err := cmd.Run()
		return err == nil, nil

	case LBNetworkSetup:
		cmd := exec.CommandContext(ctx, "networksetup", "-listallhardwareports")
		output, err := cmd.Output()
		if err != nil {
			return false, nil
		}
		return strings.Contains(string(output), iface), nil

	case LBNetsh:
		cmd := exec.CommandContext(ctx, "netsh", "interface", "show", "interface", iface)
		err := cmd.Run()
		return err == nil, nil

	default:
		return false, fmt.Errorf("unsupported backend: %s", backend)
	}
}

// getLinkSettings gets current link settings
func (m *LinkModule) getLinkSettings(ctx context.Context, backend LinkBackend, iface string) (speed int, duplex string, autoneg bool, mtu int, err error) {
	switch backend {
	case LBEthtool:
		return m.getLinkSettingsEthtool(ctx, iface)
	case LBNetworkSetup:
		return m.getLinkSettingsMacOS(ctx, iface)
	case LBNetsh:
		return m.getLinkSettingsWindows(ctx, iface)
	default:
		return 0, "", false, 0, fmt.Errorf("unsupported backend: %s", backend)
	}
}

// configureLinkSettings applies link settings
func (m *LinkModule) configureLinkSettings(ctx context.Context, backend LinkBackend, config *LinkConfig, result *StateResult) error {
	switch backend {
	case LBEthtool:
		return m.configureLinkEthtool(ctx, config, result)
	case LBNetworkSetup:
		return m.configureLinkMacOS(ctx, config, result)
	case LBNetsh:
		return m.configureLinkWindows(ctx, config, result)
	default:
		return fmt.Errorf("unsupported backend: %s", backend)
	}
}

// resetLinkSettings resets link to auto-negotiation
func (m *LinkModule) resetLinkSettings(ctx context.Context, backend LinkBackend, config *LinkConfig, result *StateResult) error {
	switch backend {
	case LBEthtool:
		return m.resetLinkEthtool(ctx, config, result)
	case LBNetworkSetup:
		return m.resetLinkMacOS(ctx, config, result)
	case LBNetsh:
		return m.resetLinkWindows(ctx, config, result)
	default:
		return fmt.Errorf("unsupported backend: %s", backend)
	}
}

// ============================================================================
// Linux ethtool Backend
// ============================================================================

func (m *LinkModule) getLinkSettingsEthtool(ctx context.Context, iface string) (int, string, bool, int, error) {
	var speed int
	var duplex string
	var autoneg bool
	var mtu int

	// Get ethtool info
	cmd := exec.CommandContext(ctx, "ethtool", iface)
	output, err := cmd.Output()
	if err != nil {
		return 0, "", false, 0, fmt.Errorf("ethtool failed: %w", err)
	}

	outputStr := string(output)

	// Parse speed: "Speed: 1000Mb/s"
	speedRegex := regexp.MustCompile(`Speed:\s*(\d+)Mb/s`)
	if matches := speedRegex.FindStringSubmatch(outputStr); len(matches) > 1 {
		speed, _ = strconv.Atoi(matches[1])
	}

	// Parse duplex: "Duplex: Full"
	duplexRegex := regexp.MustCompile(`Duplex:\s*(\w+)`)
	if matches := duplexRegex.FindStringSubmatch(outputStr); len(matches) > 1 {
		duplex = strings.ToLower(matches[1])
	}

	// Parse auto-negotiation: "Auto-negotiation: on"
	autonegRegex := regexp.MustCompile(`Auto-negotiation:\s*(\w+)`)
	if matches := autonegRegex.FindStringSubmatch(outputStr); len(matches) > 1 {
		autoneg = strings.ToLower(matches[1]) == "on"
	}

	// Get MTU from ip link
	ipCmd := exec.CommandContext(ctx, "ip", "link", "show", iface)
	ipOutput, err := ipCmd.Output()
	if err == nil {
		mtuRegex := regexp.MustCompile(`mtu\s+(\d+)`)
		if matches := mtuRegex.FindStringSubmatch(string(ipOutput)); len(matches) > 1 {
			mtu, _ = strconv.Atoi(matches[1])
		}
	}

	return speed, duplex, autoneg, mtu, nil
}

func (m *LinkModule) configureLinkEthtool(ctx context.Context, config *LinkConfig, result *StateResult) error {
	var changes []string

	// Configure speed and duplex together (required by ethtool)
	if config.Speed > 0 || config.Duplex != "" {
		args := []string{"-s", config.Interface}

		if config.Speed > 0 {
			args = append(args, "speed", strconv.Itoa(config.Speed))
			changes = append(changes, fmt.Sprintf("speed=%d", config.Speed))
		}

		if config.Duplex != "" {
			args = append(args, "duplex", config.Duplex)
			changes = append(changes, fmt.Sprintf("duplex=%s", config.Duplex))
		}

		if config.Autoneg != nil {
			if *config.Autoneg {
				args = append(args, "autoneg", "on")
			} else {
				args = append(args, "autoneg", "off")
			}
			changes = append(changes, fmt.Sprintf("autoneg=%v", *config.Autoneg))
		}

		cmd := exec.CommandContext(ctx, "ethtool", args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to set link parameters: %w: %s", err, output)
		}
	} else if config.Autoneg != nil {
		// Just autoneg change
		autonegVal := "off"
		if *config.Autoneg {
			autonegVal = "on"
		}
		cmd := exec.CommandContext(ctx, "ethtool", "-s", config.Interface, "autoneg", autonegVal)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to set autoneg: %w: %s", err, output)
		}
		changes = append(changes, fmt.Sprintf("autoneg=%s", autonegVal))
	}

	// Configure MTU
	if config.MTU > 0 {
		cmd := exec.CommandContext(ctx, "ip", "link", "set", config.Interface, "mtu", strconv.Itoa(config.MTU))
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to set MTU: %w: %s", err, output)
		}
		changes = append(changes, fmt.Sprintf("mtu=%d", config.MTU))
	}

	// Configure Wake-on-LAN
	if config.WOL != "" {
		wolFlag := m.wolModeToEthtool(config.WOL)
		cmd := exec.CommandContext(ctx, "ethtool", "-s", config.Interface, "wol", wolFlag)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to set WOL: %w: %s", err, output)
		}
		changes = append(changes, fmt.Sprintf("wol=%s", config.WOL))
	}

	result.Comment = fmt.Sprintf("Configured %s: %s", config.Interface, strings.Join(changes, ", "))
	return nil
}

func (m *LinkModule) resetLinkEthtool(ctx context.Context, config *LinkConfig, result *StateResult) error {
	// Enable auto-negotiation
	cmd := exec.CommandContext(ctx, "ethtool", "-s", config.Interface, "autoneg", "on")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to enable autoneg: %w: %s", err, output)
	}

	result.Comment = fmt.Sprintf("Reset %s to auto-negotiation", config.Interface)
	return nil
}

// wolModeToEthtool converts WOL mode to ethtool flag
func (m *LinkModule) wolModeToEthtool(mode string) string {
	switch mode {
	case "disabled":
		return "d"
	case "magic":
		return "g"
	case "unicast":
		return "u"
	case "multicast":
		return "m"
	case "broadcast":
		return "b"
	case "arp":
		return "a"
	default:
		return "d"
	}
}

// ============================================================================
// macOS Backend
// ============================================================================

func (m *LinkModule) getLinkSettingsMacOS(ctx context.Context, iface string) (int, string, bool, int, error) {
	var speed int
	var duplex string
	var mtu int
	autoneg := true // macOS typically uses auto-negotiation

	// Get interface info
	cmd := exec.CommandContext(ctx, "ifconfig", iface)
	output, err := cmd.Output()
	if err != nil {
		return 0, "", false, 0, fmt.Errorf("ifconfig failed: %w", err)
	}

	outputStr := string(output)

	// Parse MTU
	mtuRegex := regexp.MustCompile(`mtu\s+(\d+)`)
	if matches := mtuRegex.FindStringSubmatch(outputStr); len(matches) > 1 {
		mtu, _ = strconv.Atoi(matches[1])
	}

	// Parse media (e.g., "media: autoselect (1000baseT <full-duplex>)")
	mediaRegex := regexp.MustCompile(`media:\s*(\S+)\s*\(([^)]+)\)`)
	if matches := mediaRegex.FindStringSubmatch(outputStr); len(matches) > 2 {
		if matches[1] != "autoselect" {
			autoneg = false
		}

		mediaInfo := matches[2]
		// Extract speed from media type (e.g., "1000baseT")
		speedRegex := regexp.MustCompile(`(\d+)base`)
		if speedMatches := speedRegex.FindStringSubmatch(mediaInfo); len(speedMatches) > 1 {
			speed, _ = strconv.Atoi(speedMatches[1])
		}

		// Extract duplex
		if strings.Contains(mediaInfo, "full-duplex") {
			duplex = "full"
		} else if strings.Contains(mediaInfo, "half-duplex") {
			duplex = "half"
		}
	}

	return speed, duplex, autoneg, mtu, nil
}

func (m *LinkModule) configureLinkMacOS(ctx context.Context, config *LinkConfig, result *StateResult) error {
	var changes []string

	// Configure speed and duplex via ifconfig media
	if config.Speed > 0 || config.Duplex != "" {
		media := m.buildMacOSMediaString(config)
		if media != "" {
			cmd := exec.CommandContext(ctx, "ifconfig", config.Interface, "media", media)
			output, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("failed to set media: %w: %s", err, output)
			}
			changes = append(changes, fmt.Sprintf("media=%s", media))
		}
	}

	// Configure MTU
	if config.MTU > 0 {
		cmd := exec.CommandContext(ctx, "ifconfig", config.Interface, "mtu", strconv.Itoa(config.MTU))
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to set MTU: %w: %s", err, output)
		}
		changes = append(changes, fmt.Sprintf("mtu=%d", config.MTU))
	}

	result.Comment = fmt.Sprintf("Configured %s: %s", config.Interface, strings.Join(changes, ", "))
	return nil
}

func (m *LinkModule) resetLinkMacOS(ctx context.Context, config *LinkConfig, result *StateResult) error {
	// Set media to autoselect
	cmd := exec.CommandContext(ctx, "ifconfig", config.Interface, "media", "autoselect")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set media: %w: %s", err, output)
	}

	result.Comment = fmt.Sprintf("Reset %s to auto-select", config.Interface)
	return nil
}

// buildMacOSMediaString builds the media string for ifconfig
func (m *LinkModule) buildMacOSMediaString(config *LinkConfig) string {
	if config.Speed == 0 {
		return ""
	}

	// Build media type (e.g., "1000baseT")
	media := fmt.Sprintf("%dbaseT", config.Speed)

	// Add duplex
	if config.Duplex == "full" {
		media += " mediaopt full-duplex"
	} else if config.Duplex == "half" {
		media += " mediaopt half-duplex"
	}

	return media
}

// ============================================================================
// Windows Backend
// ============================================================================

func (m *LinkModule) getLinkSettingsWindows(ctx context.Context, iface string) (int, string, bool, int, error) {
	var speed int
	var duplex string
	var mtu int
	autoneg := true

	// Get interface status
	cmd := exec.CommandContext(ctx, "netsh", "interface", "ipv4", "show", "interface", iface)
	output, err := cmd.Output()
	if err == nil {
		outputStr := string(output)

		// Parse MTU
		mtuRegex := regexp.MustCompile(`MTU\s*:\s*(\d+)`)
		if matches := mtuRegex.FindStringSubmatch(outputStr); len(matches) > 1 {
			mtu, _ = strconv.Atoi(matches[1])
		}
	}

	// Get link speed via PowerShell
	psCmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command",
		fmt.Sprintf("(Get-NetAdapter -Name '%s').LinkSpeed", iface))
	psOutput, err := psCmd.Output()
	if err == nil {
		speedStr := strings.TrimSpace(string(psOutput))
		// Parse speed like "1 Gbps" or "100 Mbps"
		speedRegex := regexp.MustCompile(`(\d+)\s*(Gbps|Mbps)`)
		if matches := speedRegex.FindStringSubmatch(speedStr); len(matches) > 2 {
			speedVal, _ := strconv.Atoi(matches[1])
			if matches[2] == "Gbps" {
				speed = speedVal * 1000
			} else {
				speed = speedVal
			}
		}
	}

	// Get duplex and autoneg via PowerShell
	psCmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command",
		fmt.Sprintf("Get-NetAdapterAdvancedProperty -Name '%s' -RegistryKeyword '*SpeedDuplex' | Select-Object -ExpandProperty DisplayValue", iface))
	psOutput, err = psCmd.Output()
	if err == nil {
		duplexStr := strings.TrimSpace(string(psOutput))
		if strings.Contains(strings.ToLower(duplexStr), "auto") {
			autoneg = true
		} else {
			autoneg = false
			if strings.Contains(strings.ToLower(duplexStr), "full") {
				duplex = "full"
			} else if strings.Contains(strings.ToLower(duplexStr), "half") {
				duplex = "half"
			}
		}
	}

	return speed, duplex, autoneg, mtu, nil
}

func (m *LinkModule) configureLinkWindows(ctx context.Context, config *LinkConfig, result *StateResult) error {
	var changes []string

	// Configure speed and duplex via PowerShell
	if config.Speed > 0 || config.Duplex != "" {
		speedDuplex := m.buildWindowsSpeedDuplex(config)
		if speedDuplex != "" {
			psCmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command",
				fmt.Sprintf("Set-NetAdapterAdvancedProperty -Name '%s' -RegistryKeyword '*SpeedDuplex' -RegistryValue '%s'",
					config.Interface, speedDuplex))
			output, err := psCmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("failed to set speed/duplex: %w: %s", err, output)
			}
			changes = append(changes, fmt.Sprintf("speed_duplex=%s", speedDuplex))
		}
	}

	// Configure MTU
	if config.MTU > 0 {
		cmd := exec.CommandContext(ctx, "netsh", "interface", "ipv4", "set", "subinterface",
			config.Interface, fmt.Sprintf("mtu=%d", config.MTU), "store=persistent")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to set MTU: %w: %s", err, output)
		}
		changes = append(changes, fmt.Sprintf("mtu=%d", config.MTU))
	}

	result.Comment = fmt.Sprintf("Configured %s: %s", config.Interface, strings.Join(changes, ", "))
	return nil
}

func (m *LinkModule) resetLinkWindows(ctx context.Context, config *LinkConfig, result *StateResult) error {
	// Set to auto-negotiation (value 0 typically means auto)
	psCmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command",
		fmt.Sprintf("Set-NetAdapterAdvancedProperty -Name '%s' -RegistryKeyword '*SpeedDuplex' -RegistryValue '0'",
			config.Interface))
	output, err := psCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to enable auto-negotiation: %w: %s", err, output)
	}

	result.Comment = fmt.Sprintf("Reset %s to auto-negotiation", config.Interface)
	return nil
}

// buildWindowsSpeedDuplex builds the SpeedDuplex registry value
func (m *LinkModule) buildWindowsSpeedDuplex(config *LinkConfig) string {
	// Common SpeedDuplex values:
	// 0 = Auto
	// 1 = 10 Mbps Half
	// 2 = 10 Mbps Full
	// 3 = 100 Mbps Half
	// 4 = 100 Mbps Full
	// 5 = 1000 Mbps Full (no half duplex at gigabit)
	// 6 = 1000 Mbps Half (some adapters)

	if config.Autoneg != nil && *config.Autoneg {
		return "0"
	}

	duplex := config.Duplex
	if duplex == "" {
		duplex = "full"
	}

	switch config.Speed {
	case 10:
		if duplex == "half" {
			return "1"
		}
		return "2"
	case 100:
		if duplex == "half" {
			return "3"
		}
		return "4"
	case 1000:
		return "5" // Gigabit is typically full duplex only
	default:
		return "0" // Default to auto
	}
}

func init() {
	RegisterModule(NewLinkModule())
}
