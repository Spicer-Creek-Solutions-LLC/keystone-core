package statemgmt

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// WiFiModule implements WiFi network management
type WiFiModule struct {
	*BaseModule
}

// NewWiFiModule creates a new WiFi module
func NewWiFiModule() *WiFiModule {
	return &WiFiModule{
		BaseModule: NewBaseModule("wifi", []string{"connected", "configured", "absent"}),
	}
}

// WiFiConfig holds WiFi configuration parameters
type WiFiConfig struct {
	Name        string // Connection/profile name (defaults to SSID)
	SSID        string // Network name (required)
	Security    string // Security mode: wpa2-psk, wpa3, wep, open
	Password    string // WiFi password (required for non-open networks)
	Interface   string // WiFi interface name (e.g., wlan0)
	Priority    int    // Network priority for roaming (0-100)
	Hidden      bool   // Whether SSID is hidden
	AutoConnect bool   // Auto-connect to this network
	BSSID       string // Specific access point BSSID (optional)
}

// WiFiBackend represents the available WiFi management backend
type WiFiBackend string

const (
	WBUnknown        WiFiBackend = "unknown"
	WBNetworkManager WiFiBackend = "networkmanager" // nmcli (Linux)
	WBWpaSupplicant  WiFiBackend = "wpa_supplicant" // Linux fallback
	WBNetworkSetup   WiFiBackend = "networksetup"   // macOS
	WBNetshWlan      WiFiBackend = "netsh_wlan"     // Windows
)

// Valid WiFi security modes
var validWiFiSecurityModes = map[string]bool{
	"wpa2-psk":      true,
	"wpa2":          true, // alias for wpa2-psk
	"wpa3":          true,
	"wpa3-personal": true, // alias for wpa3
	"wep":           true,
	"open":          true,
	"none":          true, // alias for open
}

// Check checks the current state of a WiFi network
func (m *WiFiModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	config, err := m.parseWiFiConfig(decl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse WiFi config: %w", err)
	}

	// Detect WiFi backend
	backend, err := m.detectWiFiBackend()
	if err != nil {
		return nil, fmt.Errorf("failed to detect WiFi backend: %w", err)
	}
	result.Metadata["backend"] = string(backend)

	// Check if WiFi connection/profile exists
	exists, isConnected, currentSSID, err := m.checkWiFiExists(ctx, backend, config)
	if err != nil {
		return nil, fmt.Errorf("failed to check WiFi: %w", err)
	}

	result.Present = exists
	result.Metadata["ssid"] = config.SSID
	if exists {
		if isConnected {
			result.CurrentState = "connected"
		} else {
			result.CurrentState = "configured"
		}
		result.Metadata["current_ssid"] = currentSSID
	} else {
		result.CurrentState = "absent"
	}

	switch decl.State {
	case "connected":
		if !exists {
			result.Matches = false
			result.Diff["wifi"] = map[string]string{"current": "absent", "desired": "connected"}
		} else if !isConnected {
			result.Matches = false
			result.Diff["wifi"] = map[string]string{"current": "configured", "desired": "connected"}
		} else {
			result.Matches = true
		}
	case "configured":
		if !exists {
			result.Matches = false
			result.Diff["wifi"] = map[string]string{"current": "absent", "desired": "configured"}
		} else {
			result.Matches = true
		}
	case "absent":
		result.Matches = !exists
		if exists {
			result.Diff["wifi"] = map[string]string{"current": result.CurrentState, "desired": "absent"}
		}
	}

	return result, nil
}

// Apply applies the WiFi configuration
func (m *WiFiModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	result := &StateResult{
		StateID:   decl.ID,
		Module:    m.Name(),
		Success:   false,
		Changed:   false,
		Changes:   make(map[string]interface{}),
		StartTime: startTime,
	}

	config, err := m.parseWiFiConfig(decl)
	if err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to parse config: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// Validate config for the desired state
	if err := m.validateWiFiConfig(config, decl.State); err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Invalid config: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// Detect WiFi backend
	backend, err := m.detectWiFiBackend()
	if err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to detect WiFi backend: %v", err)
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
	case "connected":
		applyErr = m.connectWiFi(ctx, backend, config, result)
	case "configured":
		applyErr = m.configureWiFi(ctx, backend, config, result)
	case "absent":
		applyErr = m.removeWiFi(ctx, backend, config)
		if applyErr == nil {
			result.Comment = fmt.Sprintf("Removed WiFi profile for %s", config.SSID)
		}
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

// Test tests if the WiFi is in the desired state
func (m *WiFiModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil
}

// parseWiFiConfig extracts WiFi configuration from declaration parameters
func (m *WiFiModule) parseWiFiConfig(decl *StateDeclaration) (*WiFiConfig, error) {
	config := &WiFiConfig{
		Name:        decl.ID,
		AutoConnect: true, // Default to auto-connect
	}

	// SSID is required (can use declaration ID as default)
	config.SSID = getStringParameter(decl, "ssid", "")
	if config.SSID == "" {
		config.SSID = decl.ID
	}

	// Connection name defaults to SSID
	if name := getStringParameter(decl, "name", ""); name != "" {
		config.Name = name
	} else {
		config.Name = config.SSID
	}

	config.Security = strings.ToLower(getStringParameter(decl, "security", "wpa2-psk"))
	config.Password = getStringParameter(decl, "password", "")
	config.Interface = getStringParameter(decl, "interface", "")
	config.Priority = getIntParameter(decl, "priority", 0)
	config.Hidden = getBoolParameter(decl, "hidden", false)
	config.AutoConnect = getBoolParameter(decl, "auto_connect", true)
	config.BSSID = getStringParameter(decl, "bssid", "")

	// Normalize security mode aliases
	switch config.Security {
	case "wpa2":
		config.Security = "wpa2-psk"
	case "wpa3-personal":
		config.Security = "wpa3"
	case "none":
		config.Security = "open"
	}

	return config, nil
}

// validateWiFiConfig validates the WiFi configuration
func (m *WiFiModule) validateWiFiConfig(config *WiFiConfig, state string) error {
	// SSID is always required
	if config.SSID == "" {
		return fmt.Errorf("ssid is required")
	}

	// SSID length validation (1-32 bytes for 802.11)
	if len(config.SSID) > 32 {
		return fmt.Errorf("ssid must be 32 characters or less, got %d", len(config.SSID))
	}

	// For absent state, we only need SSID
	if state == "absent" {
		return nil
	}

	// Validate security mode
	if !validWiFiSecurityModes[config.Security] {
		return fmt.Errorf("invalid security mode: %s (valid: wpa2-psk, wpa3, wep, open)", config.Security)
	}

	// Require password for non-open networks
	if config.Security != "open" && config.Password == "" {
		return fmt.Errorf("password is required for %s security", config.Security)
	}

	// Validate password length for WPA
	if config.Password != "" {
		switch config.Security {
		case "wpa2-psk", "wpa3":
			if len(config.Password) < 8 || len(config.Password) > 63 {
				return fmt.Errorf("WPA password must be 8-63 characters, got %d", len(config.Password))
			}
		case "wep":
			// WEP keys are typically 5 or 13 characters (40-bit or 104-bit)
			if len(config.Password) != 5 && len(config.Password) != 13 &&
				len(config.Password) != 10 && len(config.Password) != 26 {
				return fmt.Errorf("WEP key must be 5 or 13 characters (ASCII) or 10 or 26 hex digits")
			}
		}
	}

	// Validate priority range
	if config.Priority < 0 || config.Priority > 100 {
		return fmt.Errorf("priority must be 0-100, got %d", config.Priority)
	}

	return nil
}

// detectWiFiBackend detects the available WiFi management tool
func (m *WiFiModule) detectWiFiBackend() (WiFiBackend, error) {
	switch runtime.GOOS {
	case "linux":
		// Check for nmcli (NetworkManager)
		if _, err := exec.LookPath("nmcli"); err == nil {
			cmd := exec.Command("systemctl", "is-active", "NetworkManager")
			if err := cmd.Run(); err == nil {
				return WBNetworkManager, nil
			}
		}

		// Check for wpa_supplicant
		if _, err := exec.LookPath("wpa_cli"); err == nil {
			return WBWpaSupplicant, nil
		}

		return WBUnknown, fmt.Errorf("no supported WiFi backend found on Linux (need NetworkManager or wpa_supplicant)")

	case "darwin":
		// macOS uses networksetup
		if _, err := exec.LookPath("networksetup"); err == nil {
			return WBNetworkSetup, nil
		}
		return WBUnknown, fmt.Errorf("networksetup not found on macOS")

	case "windows":
		// Windows uses netsh wlan
		if _, err := exec.LookPath("netsh"); err == nil {
			return WBNetshWlan, nil
		}
		return WBUnknown, fmt.Errorf("netsh not found on Windows")

	default:
		return WBUnknown, fmt.Errorf("WiFi not supported on %s", runtime.GOOS)
	}
}

// checkWiFiExists checks if a WiFi connection/profile exists
func (m *WiFiModule) checkWiFiExists(ctx context.Context, backend WiFiBackend, config *WiFiConfig) (exists bool, connected bool, currentSSID string, err error) {
	switch backend {
	case WBNetworkManager:
		return m.checkWiFiExistsNmcli(ctx, config)
	case WBWpaSupplicant:
		return m.checkWiFiExistsWpaSupplicant(ctx, config)
	case WBNetworkSetup:
		return m.checkWiFiExistsMacOS(ctx, config)
	case WBNetshWlan:
		return m.checkWiFiExistsWindows(ctx, config)
	default:
		return false, false, "", fmt.Errorf("unsupported WiFi backend: %s", backend)
	}
}

// checkWiFiExistsNmcli checks WiFi state using NetworkManager
func (m *WiFiModule) checkWiFiExistsNmcli(ctx context.Context, config *WiFiConfig) (bool, bool, string, error) {
	// Check if connection profile exists
	cmd := exec.CommandContext(ctx, "nmcli", "-t", "-f", "NAME,TYPE", "connection", "show")
	output, err := cmd.Output()
	if err != nil {
		return false, false, "", fmt.Errorf("failed to list connections: %w", err)
	}

	exists := false
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) >= 2 && fields[0] == config.Name && strings.Contains(fields[1], "wireless") {
			exists = true
			break
		}
	}

	if !exists {
		return false, false, "", nil
	}

	// Check if connected to this network
	activeCmd := exec.CommandContext(ctx, "nmcli", "-t", "-f", "NAME,DEVICE,STATE", "connection", "show", "--active")
	activeOutput, err := activeCmd.Output()
	if err != nil {
		return exists, false, config.SSID, nil
	}

	connected := false
	for _, line := range strings.Split(string(activeOutput), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) >= 1 && fields[0] == config.Name {
			connected = true
			break
		}
	}

	return exists, connected, config.SSID, nil
}

// checkWiFiExistsWpaSupplicant checks WiFi state using wpa_supplicant
func (m *WiFiModule) checkWiFiExistsWpaSupplicant(ctx context.Context, config *WiFiConfig) (bool, bool, string, error) {
	iface := config.Interface
	if iface == "" {
		iface = "wlan0"
	}

	// List known networks
	cmd := exec.CommandContext(ctx, "wpa_cli", "-i", iface, "list_networks")
	output, err := cmd.Output()
	if err != nil {
		return false, false, "", nil
	}

	exists := false
	connected := false
	lines := strings.Split(string(output), "\n")
	for _, line := range lines[1:] { // Skip header
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == config.SSID {
			exists = true
			if len(fields) >= 4 && strings.Contains(fields[3], "CURRENT") {
				connected = true
			}
			break
		}
	}

	return exists, connected, config.SSID, nil
}

// checkWiFiExistsMacOS checks WiFi state on macOS
func (m *WiFiModule) checkWiFiExistsMacOS(ctx context.Context, config *WiFiConfig) (bool, bool, string, error) {
	iface := config.Interface
	if iface == "" {
		iface = "en0"
	}

	// Check preferred networks list
	cmd := exec.CommandContext(ctx, "networksetup", "-listpreferredwirelessnetworks", iface)
	output, err := cmd.Output()
	if err != nil {
		return false, false, "", nil
	}

	exists := false
	for _, line := range strings.Split(string(output), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == config.SSID {
			exists = true
			break
		}
	}

	// Check current connection
	currentCmd := exec.CommandContext(ctx, "networksetup", "-getairportnetwork", iface)
	currentOutput, _ := currentCmd.Output()
	currentSSID := ""
	connected := false
	if strings.Contains(string(currentOutput), "Current Wi-Fi Network:") {
		parts := strings.SplitN(string(currentOutput), ":", 2)
		if len(parts) == 2 {
			currentSSID = strings.TrimSpace(parts[1])
			if currentSSID == config.SSID {
				connected = true
				exists = true // If connected, it exists
			}
		}
	}

	return exists, connected, currentSSID, nil
}

// checkWiFiExistsWindows checks WiFi state on Windows
func (m *WiFiModule) checkWiFiExistsWindows(ctx context.Context, config *WiFiConfig) (bool, bool, string, error) {
	// Check if profile exists
	cmd := exec.CommandContext(ctx, "netsh", "wlan", "show", "profile", fmt.Sprintf("name=%s", config.Name))
	output, err := cmd.Output()
	exists := err == nil && strings.Contains(string(output), config.Name)

	// Check current connection
	ifaceCmd := exec.CommandContext(ctx, "netsh", "wlan", "show", "interfaces")
	ifaceOutput, _ := ifaceCmd.Output()
	connected := false
	currentSSID := ""
	for _, line := range strings.Split(string(ifaceOutput), "\n") {
		if strings.Contains(line, "SSID") && !strings.Contains(line, "BSSID") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				currentSSID = strings.TrimSpace(parts[1])
				if currentSSID == config.SSID {
					connected = true
					exists = true
				}
			}
		}
	}

	return exists, connected, currentSSID, nil
}

// connectWiFi connects to a WiFi network
func (m *WiFiModule) connectWiFi(ctx context.Context, backend WiFiBackend, config *WiFiConfig, result *StateResult) error {
	switch backend {
	case WBNetworkManager:
		return m.connectWiFiNmcli(ctx, config, result)
	case WBWpaSupplicant:
		return m.connectWiFiWpaSupplicant(ctx, config, result)
	case WBNetworkSetup:
		return m.connectWiFiMacOS(ctx, config, result)
	case WBNetshWlan:
		return m.connectWiFiWindows(ctx, config, result)
	default:
		return fmt.Errorf("unsupported WiFi backend: %s", backend)
	}
}

// configureWiFi adds/configures a WiFi network without connecting
func (m *WiFiModule) configureWiFi(ctx context.Context, backend WiFiBackend, config *WiFiConfig, result *StateResult) error {
	switch backend {
	case WBNetworkManager:
		return m.configureWiFiNmcli(ctx, config, result)
	case WBWpaSupplicant:
		return m.configureWiFiWpaSupplicant(ctx, config, result)
	case WBNetworkSetup:
		return m.configureWiFiMacOS(ctx, config, result)
	case WBNetshWlan:
		return m.configureWiFiWindows(ctx, config, result)
	default:
		return fmt.Errorf("unsupported WiFi backend: %s", backend)
	}
}

// removeWiFi removes a WiFi profile
func (m *WiFiModule) removeWiFi(ctx context.Context, backend WiFiBackend, config *WiFiConfig) error {
	switch backend {
	case WBNetworkManager:
		cmd := exec.CommandContext(ctx, "nmcli", "connection", "delete", config.Name)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to delete WiFi profile: %w (output: %s)", err, string(output))
		}
		return nil

	case WBWpaSupplicant:
		iface := config.Interface
		if iface == "" {
			iface = "wlan0"
		}
		// Find network ID and remove
		listCmd := exec.CommandContext(ctx, "wpa_cli", "-i", iface, "list_networks")
		output, err := listCmd.Output()
		if err != nil {
			return fmt.Errorf("failed to list networks: %w", err)
		}
		for _, line := range strings.Split(string(output), "\n")[1:] {
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[1] == config.SSID {
				removeCmd := exec.CommandContext(ctx, "wpa_cli", "-i", iface, "remove_network", fields[0])
				if _, err := removeCmd.Output(); err != nil {
					return fmt.Errorf("failed to remove network: %w", err)
				}
				saveCmd := exec.CommandContext(ctx, "wpa_cli", "-i", iface, "save_config")
				saveCmd.Run()
				break
			}
		}
		return nil

	case WBNetworkSetup:
		iface := config.Interface
		if iface == "" {
			iface = "en0"
		}
		cmd := exec.CommandContext(ctx, "networksetup", "-removepreferredwirelessnetwork", iface, config.SSID)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to remove WiFi profile: %w (output: %s)", err, string(output))
		}
		return nil

	case WBNetshWlan:
		cmd := exec.CommandContext(ctx, "netsh", "wlan", "delete", "profile", fmt.Sprintf("name=%s", config.Name))
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to delete WiFi profile: %w (output: %s)", err, string(output))
		}
		return nil

	default:
		return fmt.Errorf("unsupported WiFi backend: %s", backend)
	}
}

// connectWiFiNmcli connects to WiFi using NetworkManager
func (m *WiFiModule) connectWiFiNmcli(ctx context.Context, config *WiFiConfig, result *StateResult) error {
	// Check if connection profile exists
	checkCmd := exec.CommandContext(ctx, "nmcli", "-t", "connection", "show", config.Name)
	profileExists := checkCmd.Run() == nil

	if !profileExists {
		// Create new connection
		args := []string{
			"connection", "add",
			"type", "wifi",
			"con-name", config.Name,
			"ssid", config.SSID,
		}

		// Add interface if specified
		if config.Interface != "" {
			args = append(args, "ifname", config.Interface)
		}

		// Add security settings
		switch config.Security {
		case "wpa2-psk", "wpa3":
			args = append(args, "wifi-sec.key-mgmt", "wpa-psk")
			args = append(args, "wifi-sec.psk", config.Password)
		case "wep":
			args = append(args, "wifi-sec.key-mgmt", "none")
			args = append(args, "wifi-sec.wep-key0", config.Password)
		case "open":
			// No security settings needed
		}

		// Hidden network
		if config.Hidden {
			args = append(args, "wifi.hidden", "yes")
		}

		// Auto-connect
		if config.AutoConnect {
			args = append(args, "connection.autoconnect", "yes")
		} else {
			args = append(args, "connection.autoconnect", "no")
		}

		// Priority
		if config.Priority > 0 {
			args = append(args, "connection.autoconnect-priority", strconv.Itoa(config.Priority))
		}

		cmd := exec.CommandContext(ctx, "nmcli", args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to create WiFi connection: %w (output: %s)", err, string(output))
		}
	}

	// Connect to the network
	upCmd := exec.CommandContext(ctx, "nmcli", "connection", "up", config.Name)
	output, err := upCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to connect to WiFi: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Connected to WiFi network %s", config.SSID)
	return nil
}

// configureWiFiNmcli configures WiFi without connecting using NetworkManager
func (m *WiFiModule) configureWiFiNmcli(ctx context.Context, config *WiFiConfig, result *StateResult) error {
	// Check if connection profile exists
	checkCmd := exec.CommandContext(ctx, "nmcli", "-t", "connection", "show", config.Name)
	profileExists := checkCmd.Run() == nil

	if profileExists {
		result.Comment = fmt.Sprintf("WiFi profile %s already exists", config.SSID)
		return nil
	}

	// Create new connection without auto-connect
	args := []string{
		"connection", "add",
		"type", "wifi",
		"con-name", config.Name,
		"ssid", config.SSID,
		"connection.autoconnect", "no",
	}

	// Add interface if specified
	if config.Interface != "" {
		args = append(args, "ifname", config.Interface)
	}

	// Add security settings
	switch config.Security {
	case "wpa2-psk", "wpa3":
		args = append(args, "wifi-sec.key-mgmt", "wpa-psk")
		args = append(args, "wifi-sec.psk", config.Password)
	case "wep":
		args = append(args, "wifi-sec.key-mgmt", "none")
		args = append(args, "wifi-sec.wep-key0", config.Password)
	case "open":
		// No security settings needed
	}

	// Hidden network
	if config.Hidden {
		args = append(args, "wifi.hidden", "yes")
	}

	// Priority
	if config.Priority > 0 {
		args = append(args, "connection.autoconnect-priority", strconv.Itoa(config.Priority))
	}

	cmd := exec.CommandContext(ctx, "nmcli", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to configure WiFi: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Configured WiFi profile for %s (not connected)", config.SSID)
	return nil
}

// connectWiFiWpaSupplicant connects to WiFi using wpa_supplicant
func (m *WiFiModule) connectWiFiWpaSupplicant(ctx context.Context, config *WiFiConfig, result *StateResult) error {
	iface := config.Interface
	if iface == "" {
		iface = "wlan0"
	}

	// Add network via wpa_cli
	addCmd := exec.CommandContext(ctx, "wpa_cli", "-i", iface, "add_network")
	output, err := addCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to add network: %w", err)
	}
	networkID := strings.TrimSpace(string(output))

	// Set network parameters
	setSSID := exec.CommandContext(ctx, "wpa_cli", "-i", iface, "set_network", networkID, "ssid", fmt.Sprintf("\"%s\"", config.SSID))
	if _, err := setSSID.Output(); err != nil {
		return fmt.Errorf("failed to set SSID: %w", err)
	}

	switch config.Security {
	case "wpa2-psk", "wpa3":
		setKeyMgmt := exec.CommandContext(ctx, "wpa_cli", "-i", iface, "set_network", networkID, "key_mgmt", "WPA-PSK")
		setKeyMgmt.Run()
		setPsk := exec.CommandContext(ctx, "wpa_cli", "-i", iface, "set_network", networkID, "psk", fmt.Sprintf("\"%s\"", config.Password))
		if _, err := setPsk.Output(); err != nil {
			return fmt.Errorf("failed to set PSK: %w", err)
		}
	case "open":
		setKeyMgmt := exec.CommandContext(ctx, "wpa_cli", "-i", iface, "set_network", networkID, "key_mgmt", "NONE")
		setKeyMgmt.Run()
	case "wep":
		setKeyMgmt := exec.CommandContext(ctx, "wpa_cli", "-i", iface, "set_network", networkID, "key_mgmt", "NONE")
		setKeyMgmt.Run()
		setWepKey := exec.CommandContext(ctx, "wpa_cli", "-i", iface, "set_network", networkID, "wep_key0", fmt.Sprintf("\"%s\"", config.Password))
		setWepKey.Run()
	}

	if config.Hidden {
		setScanSSID := exec.CommandContext(ctx, "wpa_cli", "-i", iface, "set_network", networkID, "scan_ssid", "1")
		setScanSSID.Run()
	}

	if config.Priority > 0 {
		setPriority := exec.CommandContext(ctx, "wpa_cli", "-i", iface, "set_network", networkID, "priority", strconv.Itoa(config.Priority))
		setPriority.Run()
	}

	// Enable and select network
	enableCmd := exec.CommandContext(ctx, "wpa_cli", "-i", iface, "enable_network", networkID)
	enableCmd.Run()

	selectCmd := exec.CommandContext(ctx, "wpa_cli", "-i", iface, "select_network", networkID)
	if _, err := selectCmd.Output(); err != nil {
		return fmt.Errorf("failed to select network: %w", err)
	}

	// Save configuration
	saveCmd := exec.CommandContext(ctx, "wpa_cli", "-i", iface, "save_config")
	saveCmd.Run()

	result.Comment = fmt.Sprintf("Connected to WiFi network %s via wpa_supplicant", config.SSID)
	return nil
}

// configureWiFiWpaSupplicant configures WiFi without connecting using wpa_supplicant
func (m *WiFiModule) configureWiFiWpaSupplicant(ctx context.Context, config *WiFiConfig, result *StateResult) error {
	// For wpa_supplicant, we write to the config file directly
	configFile := "/etc/wpa_supplicant/wpa_supplicant.conf"

	// Read existing config
	content, err := os.ReadFile(configFile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read wpa_supplicant.conf: %w", err)
	}

	// Check if network already exists
	if strings.Contains(string(content), fmt.Sprintf("ssid=\"%s\"", config.SSID)) {
		result.Comment = fmt.Sprintf("WiFi profile %s already exists in wpa_supplicant.conf", config.SSID)
		return nil
	}

	// Append network block
	networkBlock := m.generateWpaSupplicantNetworkBlock(config)
	newContent := string(content) + "\n" + networkBlock

	if err := os.WriteFile(configFile, []byte(newContent), 0600); err != nil {
		return fmt.Errorf("failed to write wpa_supplicant.conf: %w", err)
	}

	// Reconfigure wpa_supplicant
	iface := config.Interface
	if iface == "" {
		iface = "wlan0"
	}
	reconfigCmd := exec.CommandContext(ctx, "wpa_cli", "-i", iface, "reconfigure")
	reconfigCmd.Run()

	result.Comment = fmt.Sprintf("Configured WiFi profile for %s in wpa_supplicant.conf", config.SSID)
	return nil
}

// generateWpaSupplicantNetworkBlock generates a wpa_supplicant network block
func (m *WiFiModule) generateWpaSupplicantNetworkBlock(config *WiFiConfig) string {
	var buf bytes.Buffer
	buf.WriteString("network={\n")
	buf.WriteString(fmt.Sprintf("    ssid=\"%s\"\n", config.SSID))

	switch config.Security {
	case "wpa2-psk", "wpa3":
		buf.WriteString("    key_mgmt=WPA-PSK\n")
		buf.WriteString(fmt.Sprintf("    psk=\"%s\"\n", config.Password))
	case "open":
		buf.WriteString("    key_mgmt=NONE\n")
	case "wep":
		buf.WriteString("    key_mgmt=NONE\n")
		buf.WriteString(fmt.Sprintf("    wep_key0=\"%s\"\n", config.Password))
	}

	if config.Hidden {
		buf.WriteString("    scan_ssid=1\n")
	}

	if config.Priority > 0 {
		buf.WriteString(fmt.Sprintf("    priority=%d\n", config.Priority))
	}

	buf.WriteString("}\n")
	return buf.String()
}

// connectWiFiMacOS connects to WiFi on macOS
func (m *WiFiModule) connectWiFiMacOS(ctx context.Context, config *WiFiConfig, result *StateResult) error {
	iface := config.Interface
	if iface == "" {
		iface = "en0"
	}

	// Ensure WiFi is on
	powerCmd := exec.CommandContext(ctx, "networksetup", "-setairportpower", iface, "on")
	powerCmd.Run()

	// Connect to network
	var cmd *exec.Cmd
	if config.Security == "open" {
		cmd = exec.CommandContext(ctx, "networksetup", "-setairportnetwork", iface, config.SSID)
	} else {
		cmd = exec.CommandContext(ctx, "networksetup", "-setairportnetwork", iface, config.SSID, config.Password)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to connect to WiFi: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Connected to WiFi network %s", config.SSID)
	return nil
}

// configureWiFiMacOS adds WiFi to preferred networks on macOS
func (m *WiFiModule) configureWiFiMacOS(ctx context.Context, config *WiFiConfig, result *StateResult) error {
	iface := config.Interface
	if iface == "" {
		iface = "en0"
	}

	// Map security mode to macOS format
	var securityType string
	switch config.Security {
	case "wpa2-psk", "wpa3":
		securityType = "WPA2"
	case "wep":
		securityType = "WEP"
	case "open":
		securityType = "OPEN"
	default:
		securityType = "WPA2"
	}

	// Add to preferred networks
	var cmd *exec.Cmd
	if config.Security == "open" {
		cmd = exec.CommandContext(ctx, "networksetup", "-addpreferredwirelessnetworkatindex", iface, config.SSID, "0", securityType)
	} else {
		cmd = exec.CommandContext(ctx, "networksetup", "-addpreferredwirelessnetworkatindex", iface, config.SSID, "0", securityType, config.Password)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add WiFi to preferred networks: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Added WiFi profile for %s to preferred networks", config.SSID)
	return nil
}

// connectWiFiWindows connects to WiFi on Windows
func (m *WiFiModule) connectWiFiWindows(ctx context.Context, config *WiFiConfig, result *StateResult) error {
	// Check if profile exists
	checkCmd := exec.CommandContext(ctx, "netsh", "wlan", "show", "profile", fmt.Sprintf("name=%s", config.Name))
	if checkCmd.Run() != nil {
		// Create profile
		if err := m.createWindowsWiFiProfile(ctx, config); err != nil {
			return err
		}
	}

	// Connect
	connectArgs := []string{"wlan", "connect", fmt.Sprintf("name=%s", config.Name)}
	if config.Interface != "" {
		connectArgs = append(connectArgs, fmt.Sprintf("interface=%s", config.Interface))
	}

	cmd := exec.CommandContext(ctx, "netsh", connectArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to connect to WiFi: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Connected to WiFi network %s", config.SSID)
	return nil
}

// configureWiFiWindows adds WiFi profile on Windows without connecting
func (m *WiFiModule) configureWiFiWindows(ctx context.Context, config *WiFiConfig, result *StateResult) error {
	// Check if profile exists
	checkCmd := exec.CommandContext(ctx, "netsh", "wlan", "show", "profile", fmt.Sprintf("name=%s", config.Name))
	if checkCmd.Run() == nil {
		result.Comment = fmt.Sprintf("WiFi profile %s already exists", config.SSID)
		return nil
	}

	if err := m.createWindowsWiFiProfile(ctx, config); err != nil {
		return err
	}

	result.Comment = fmt.Sprintf("Created WiFi profile for %s", config.SSID)
	return nil
}

// createWindowsWiFiProfile creates a Windows WLAN profile XML and imports it
func (m *WiFiModule) createWindowsWiFiProfile(ctx context.Context, config *WiFiConfig) error {
	profileXML := m.generateWindowsProfileXML(config)

	// Write to temp file
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("kscore-wifi-%s.xml", config.Name))
	if err := os.WriteFile(tmpFile, []byte(profileXML), 0600); err != nil {
		return fmt.Errorf("failed to write profile XML: %w", err)
	}
	defer os.Remove(tmpFile)

	// Import profile
	cmd := exec.CommandContext(ctx, "netsh", "wlan", "add", "profile", fmt.Sprintf("filename=%s", tmpFile))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to import WiFi profile: %w (output: %s)", err, string(output))
	}

	return nil
}

// generateWindowsProfileXML generates a Windows WLAN profile XML
func (m *WiFiModule) generateWindowsProfileXML(config *WiFiConfig) string {
	// Determine authentication and encryption based on security mode
	var authentication, encryption string
	switch config.Security {
	case "wpa2-psk":
		authentication = "WPA2PSK"
		encryption = "AES"
	case "wpa3":
		authentication = "WPA3SAE"
		encryption = "AES"
	case "wep":
		authentication = "open"
		encryption = "WEP"
	case "open":
		authentication = "open"
		encryption = "none"
	default:
		authentication = "WPA2PSK"
		encryption = "AES"
	}

	// Connection mode
	connectionMode := "auto"
	if !config.AutoConnect {
		connectionMode = "manual"
	}

	// Hidden network
	nonBroadcast := "false"
	if config.Hidden {
		nonBroadcast = "true"
	}

	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0"?>`)
	buf.WriteString("\n")
	buf.WriteString(`<WLANProfile xmlns="http://www.microsoft.com/networking/WLAN/profile/v1">`)
	buf.WriteString("\n")
	buf.WriteString(fmt.Sprintf("  <name>%s</name>\n", config.Name))
	buf.WriteString("  <SSIDConfig>\n")
	buf.WriteString("    <SSID>\n")
	buf.WriteString(fmt.Sprintf("      <name>%s</name>\n", config.SSID))
	buf.WriteString("    </SSID>\n")
	buf.WriteString(fmt.Sprintf("    <nonBroadcast>%s</nonBroadcast>\n", nonBroadcast))
	buf.WriteString("  </SSIDConfig>\n")
	buf.WriteString("  <connectionType>ESS</connectionType>\n")
	buf.WriteString(fmt.Sprintf("  <connectionMode>%s</connectionMode>\n", connectionMode))
	buf.WriteString("  <MSM>\n")
	buf.WriteString("    <security>\n")
	buf.WriteString("      <authEncryption>\n")
	buf.WriteString(fmt.Sprintf("        <authentication>%s</authentication>\n", authentication))
	buf.WriteString(fmt.Sprintf("        <encryption>%s</encryption>\n", encryption))
	buf.WriteString("        <useOneX>false</useOneX>\n")
	buf.WriteString("      </authEncryption>\n")

	// Add shared key for secured networks
	if config.Security != "open" {
		buf.WriteString("      <sharedKey>\n")
		buf.WriteString("        <keyType>passPhrase</keyType>\n")
		buf.WriteString("        <protected>false</protected>\n")
		buf.WriteString(fmt.Sprintf("        <keyMaterial>%s</keyMaterial>\n", config.Password))
		buf.WriteString("      </sharedKey>\n")
	}

	buf.WriteString("    </security>\n")
	buf.WriteString("  </MSM>\n")
	buf.WriteString("</WLANProfile>\n")

	return buf.String()
}

func init() {
	RegisterModule(NewWiFiModule())
}
