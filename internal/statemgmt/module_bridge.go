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

// BridgeModule implements network bridge management
type BridgeModule struct {
	*BaseModule
}

// NewBridgeModule creates a new bridge module
func NewBridgeModule() *BridgeModule {
	return &BridgeModule{
		BaseModule: NewBaseModule("bridge", []string{"present", "absent"}),
	}
}

// BridgeConfig holds bridge configuration parameters
type BridgeConfig struct {
	Name         string   // Bridge interface name (e.g., "br0")
	Ports        []string // Member interfaces
	STP          bool     // Enable Spanning Tree Protocol
	ForwardDelay int      // STP forward delay (seconds), default 15
	HelloTime    int      // STP hello time (seconds), default 2
	MaxAge       int      // STP max age (seconds), default 20
	AgeingTime   int      // MAC address ageing time (seconds), default 300
	Addresses    []string // Optional IP addresses
	Gateway      string   // Optional gateway
	DNS          []string // Optional DNS servers
	MTU          int      // Optional MTU
}

// Check checks the current state of a bridge interface
func (m *BridgeModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	config, err := m.parseBridgeConfig(decl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse bridge config: %w", err)
	}

	// Check if bridge interface exists
	bridgeExists, currentPorts, stpEnabled, err := m.checkBridgeExists(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to check bridge: %w", err)
	}

	result.Present = bridgeExists
	if bridgeExists {
		result.CurrentState = "present"
		result.Metadata["ports"] = currentPorts
		result.Metadata["stp"] = stpEnabled
	} else {
		result.CurrentState = "absent"
	}

	switch decl.State {
	case "present":
		if !bridgeExists {
			result.Matches = false
			result.Diff["bridge"] = map[string]string{"current": "absent", "desired": "present"}
		} else {
			// Check if ports match
			switch {
			case !stringSlicesEqualUnordered(config.Ports, currentPorts):
				result.Matches = false
				result.Diff["ports"] = map[string]interface{}{"current": currentPorts, "desired": config.Ports}
			case config.STP != stpEnabled:
				result.Matches = false
				result.Diff["stp"] = map[string]bool{"current": stpEnabled, "desired": config.STP}
			default:
				result.Matches = true
			}
		}
	case "absent":
		result.Matches = !bridgeExists
		if bridgeExists {
			result.Diff["bridge"] = map[string]string{"current": "present", "desired": "absent"}
		}
	}

	return result, nil //nolint:nilerr // error captured in result.Error
}

// Apply applies the bridge configuration
func (m *BridgeModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	result := &StateResult{
		StateID:   decl.ID,
		Module:    m.Name(),
		Success:   false,
		Changed:   false,
		Changes:   make(map[string]interface{}),
		StartTime: startTime,
	}

	config, err := m.parseBridgeConfig(decl)
	if err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to parse config: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	// Check platform support
	if runtime.GOOS == "darwin" {
		result.Error = fmt.Errorf("bridging not natively supported on macOS")
		result.Comment = "Network bridging is not natively supported on macOS"
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

	// Detect network manager (Linux only)
	var nm NetworkManager
	if runtime.GOOS == "linux" {
		nm, err = m.detectNetworkManager(ctx)
		if err != nil {
			result.Error = err
			result.Comment = fmt.Sprintf("Failed to detect network manager: %v", err)
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(startTime)
			return result, nil //nolint:nilerr // error captured in result.Error
		}
	}

	// Apply changes
	var applyErr error
	switch decl.State {
	case "present":
		// If bridge exists but with different config, delete first - best-effort
		if checkResult.Present && len(checkResult.Diff) > 0 {
			_ = m.deleteBridge(ctx, nm, config)
		}
		if runtime.GOOS == "windows" {
			applyErr = m.createBridgeWindows(ctx, config, result)
		} else {
			applyErr = m.createBridge(ctx, nm, config, result)
		}
	case "absent":
		if runtime.GOOS == "windows" {
			applyErr = m.deleteBridgeWindows(ctx, config)
		} else {
			applyErr = m.deleteBridge(ctx, nm, config)
		}
		if applyErr == nil {
			result.Comment = fmt.Sprintf("Deleted bridge interface %s", config.Name)
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
	return result, nil //nolint:nilerr // error captured in result.Error
}

// Test tests if the bridge is in the desired state
func (m *BridgeModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil
}

// parseBridgeConfig extracts bridge configuration from declaration parameters
func (m *BridgeModule) parseBridgeConfig(decl *StateDeclaration) (*BridgeConfig, error) {
	config := &BridgeConfig{
		Name:         decl.ID,
		ForwardDelay: 15,
		HelloTime:    2,
		MaxAge:       20,
		AgeingTime:   300,
	}

	if name := getStringParameter(decl, "name", ""); name != "" {
		config.Name = name
	}

	// Parse ports
	config.Ports = parseStringOrArray(decl.Parameters["ports"])
	// Also accept 'interfaces' as alternative
	if len(config.Ports) == 0 {
		config.Ports = parseStringOrArray(decl.Parameters["interfaces"])
	}

	config.STP = getBoolParameter(decl, "stp", false)

	if fd := getIntParameter(decl, "forward_delay", 0); fd > 0 {
		config.ForwardDelay = fd
	}
	if ht := getIntParameter(decl, "hello_time", 0); ht > 0 {
		config.HelloTime = ht
	}
	if ma := getIntParameter(decl, "max_age", 0); ma > 0 {
		config.MaxAge = ma
	}
	if at := getIntParameter(decl, "ageing_time", 0); at > 0 {
		config.AgeingTime = at
	}

	config.Gateway = getStringParameter(decl, "gateway", "")
	config.MTU = getIntParameter(decl, "mtu", 0)

	// Parse addresses
	config.Addresses = parseStringOrArray(decl.Parameters["addresses"])
	if addr := getStringParameter(decl, "address", ""); addr != "" {
		config.Addresses = append(config.Addresses, addr)
	}

	// Parse DNS
	config.DNS = parseStringOrArray(decl.Parameters["dns"])

	return config, nil
}

// checkBridgeExists checks if a bridge interface exists and returns its ports and STP status
func (m *BridgeModule) checkBridgeExists(ctx context.Context, config *BridgeConfig) (exists bool, ports []string, stp bool, err error) {
	switch runtime.GOOS {
	case "linux":
		return m.checkBridgeExistsLinux(ctx, config)
	case "windows":
		return m.checkBridgeExistsWindows(ctx, config)
	default:
		return false, nil, false, fmt.Errorf("bridge check not supported on %s", runtime.GOOS)
	}
}

// checkBridgeExistsLinux checks bridge existence on Linux
func (m *BridgeModule) checkBridgeExistsLinux(ctx context.Context, config *BridgeConfig) (exists bool, ports []string, stp bool, err error) {
	// Check if bridge interface exists
	bridgeDir := filepath.Join("/sys", "class", "net", config.Name, "bridge")
	if _, err := os.Stat(bridgeDir); os.IsNotExist(err) {
		return false, nil, false, nil
	}

	// Read STP state
	stpBytes, _ := os.ReadFile(filepath.Join(bridgeDir, "stp_state"))
	stp = strings.TrimSpace(string(stpBytes)) == "1"

	// Read bridge ports from brif directory
	brifDir := filepath.Join("/sys", "class", "net", config.Name, "brif")
	entries, err := os.ReadDir(brifDir)
	if err != nil {
		return true, nil, stp, nil //nolint:nilerr // missing dir means no ports
	}

	for _, entry := range entries {
		ports = append(ports, entry.Name())
	}

	return true, ports, stp, nil
}

// checkBridgeExistsWindows checks for Hyper-V switch on Windows
func (m *BridgeModule) checkBridgeExistsWindows(ctx context.Context, config *BridgeConfig) (exists bool, ports []string, stp bool, err error) {
	//nolint:gosec // G204: PowerShell execution is intentional for Windows bridge management
	cmd := exec.CommandContext(ctx, "powershell", "-Command",
		fmt.Sprintf("Get-VMSwitch -Name '%s' -ErrorAction SilentlyContinue | ConvertTo-Json", config.Name))
	output, err := cmd.Output()
	if err != nil || len(output) == 0 {
		return false, nil, false, nil //nolint:nilerr // switch not found is not an error
	}

	if strings.Contains(string(output), config.Name) {
		return true, nil, false, nil
	}

	return false, nil, false, nil
}

// detectNetworkManager detects the available network manager on Linux
func (m *BridgeModule) detectNetworkManager(ctx context.Context) (NetworkManager, error) {
	// Check for nmcli (NetworkManager)
	if _, err := exec.LookPath("nmcli"); err == nil {
		//nolint:gosec // G204: systemctl execution is intentional for network manager detection
		cmd := exec.CommandContext(ctx, "systemctl", "is-active", "NetworkManager")
		if err := cmd.Run(); err == nil {
			return NMNetworkManager, nil
		}
	}

	// Check for netplan
	if _, err := exec.LookPath("netplan"); err == nil {
		return NMNetplan, nil
	}

	// Check for systemd-networkd
	//nolint:gosec // G204: systemctl execution is intentional for network manager detection
	cmd := exec.CommandContext(ctx, "systemctl", "is-active", "systemd-networkd")
	if err := cmd.Run(); err == nil {
		return NMSystemdNetworkd, nil
	}

	// Check for ifupdown
	if _, err := exec.LookPath("ifup"); err == nil {
		return NMIfupdown, nil
	}

	return NMUnknown, nil
}

// createBridge creates a bridge interface
func (m *BridgeModule) createBridge(ctx context.Context, nm NetworkManager, config *BridgeConfig, result *StateResult) error {
	switch nm {
	case NMNetworkManager:
		return m.createBridgeNmcli(ctx, config, result)
	case NMNetplan:
		return m.createBridgeNetplan(ctx, config, result)
	case NMSystemdNetworkd:
		return m.createBridgeSystemdNetworkd(ctx, config, result)
	case NMIfupdown:
		return m.createBridgeIfupdown(ctx, config, result)
	default:
		return m.createBridgeRaw(ctx, config, result)
	}
}

// deleteBridge deletes a bridge interface
func (m *BridgeModule) deleteBridge(ctx context.Context, nm NetworkManager, config *BridgeConfig) error {
	switch nm {
	case NMNetworkManager:
		// Delete port connections and bridge connection
		for _, port := range config.Ports {
			//nolint:gosec // G204: nmcli execution is intentional for bridge interface management
			portDelCmd := exec.CommandContext(ctx, "nmcli", "connection", "delete", config.Name+"-port-"+port)
			portDelCmd.Run()
		}
		//nolint:gosec // G204: nmcli execution is intentional for bridge interface management
		cmd := exec.CommandContext(ctx, "nmcli", "connection", "delete", config.Name)
		output, err := cmd.CombinedOutput()
		if err != nil {
			//nolint:gosec // G204: ip command execution is intentional for bridge interface management
			delCmd := exec.CommandContext(ctx, "ip", "link", "delete", config.Name)
			if _, delErr := delCmd.CombinedOutput(); delErr != nil {
				return fmt.Errorf("failed to delete bridge: %w (output: %s)", err, string(output))
			}
		}
		return nil
	case NMNetplan:
		netplanFile := filepath.Join("/etc", "netplan", fmt.Sprintf("90-kscore-bridge-%s.yaml", config.Name))
		if err := os.Remove(netplanFile); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove netplan file: %w", err)
		}
		//nolint:gosec // G204: netplan execution is intentional for bridge interface management
		applyCmd := exec.CommandContext(ctx, "netplan", "apply")
		if output, err := applyCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to apply netplan: %w (output: %s)", err, string(output))
		}
		return nil
	case NMSystemdNetworkd:
		networkDir := "/etc/systemd/network"
		netdevFile := filepath.Join(networkDir, fmt.Sprintf("10-kscore-bridge-%s.netdev", config.Name))
		networkFile := filepath.Join(networkDir, fmt.Sprintf("20-kscore-bridge-%s.network", config.Name))
		os.Remove(netdevFile)
		os.Remove(networkFile)
		for _, port := range config.Ports {
			portFile := filepath.Join(networkDir, fmt.Sprintf("10-kscore-bridge-%s-port-%s.network", config.Name, port))
			os.Remove(portFile)
		}
		//nolint:gosec // G204: networkctl execution is intentional for bridge interface management
		reloadCmd := exec.CommandContext(ctx, "networkctl", "reload")
		reloadCmd.Run()
		return nil
	default:
		// Release ports first
		for _, port := range config.Ports {
			//nolint:gosec // G204: ip command execution is intentional for bridge interface management
			releaseCmd := exec.CommandContext(ctx, "ip", "link", "set", port, "nomaster")
			releaseCmd.Run()
		}
		//nolint:gosec // G204: ip command execution is intentional for bridge interface management
		cmd := exec.CommandContext(ctx, "ip", "link", "delete", config.Name)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to delete bridge: %w (output: %s)", err, string(output))
		}
		return nil
	}
}

// createBridgeNmcli creates a bridge using NetworkManager
func (m *BridgeModule) createBridgeNmcli(ctx context.Context, config *BridgeConfig, result *StateResult) error {
	// Create bridge connection
	args := []string{
		"connection", "add",
		"type", "bridge",
		"con-name", config.Name,
		"ifname", config.Name,
	}

	// STP settings
	if config.STP {
		args = append(args, "bridge.stp", "yes")
	} else {
		args = append(args, "bridge.stp", "no")
	}
	args = append(args,
		"bridge.forward-delay", strconv.Itoa(config.ForwardDelay),
		"bridge.hello-time", strconv.Itoa(config.HelloTime),
		"bridge.max-age", strconv.Itoa(config.MaxAge),
		"bridge.ageing-time", strconv.Itoa(config.AgeingTime))

	// Add IP configuration
	if len(config.Addresses) > 0 {
		args = append(args, "ipv4.addresses", strings.Join(config.Addresses, ","), "ipv4.method", "manual")
	} else {
		args = append(args, "ipv4.method", "disabled")
	}

	if config.Gateway != "" {
		args = append(args, "ipv4.gateway", config.Gateway)
	}

	if len(config.DNS) > 0 {
		args = append(args, "ipv4.dns", strings.Join(config.DNS, ","))
	}

	//nolint:gosec // G204: nmcli execution is intentional for bridge interface management
	cmd := exec.CommandContext(ctx, "nmcli", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create bridge: %w (output: %s)", err, string(output))
	}

	// Add port connections
	for _, port := range config.Ports {
		portArgs := []string{
			"connection", "add",
			"type", "bridge-slave",
			"con-name", config.Name + "-port-" + port,
			"ifname", port,
			"master", config.Name,
		}
		//nolint:gosec // G204: nmcli execution is intentional for bridge interface management
		portCmd := exec.CommandContext(ctx, "nmcli", portArgs...)
		if output, err := portCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to add port %s: %w (output: %s)", port, err, string(output))
		}
	}

	// Bring up the bridge
	//nolint:gosec // G204: nmcli execution is intentional for bridge interface management
	upCmd := exec.CommandContext(ctx, "nmcli", "connection", "up", config.Name)
	if output, err := upCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to activate bridge: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Created bridge %s with ports %v via NetworkManager",
		config.Name, config.Ports)
	return nil
}

// createBridgeNetplan creates a bridge using netplan
func (m *BridgeModule) createBridgeNetplan(ctx context.Context, config *BridgeConfig, result *StateResult) error {
	netplanDir := "/etc/netplan"
	netplanFile := filepath.Join(netplanDir, fmt.Sprintf("90-kscore-bridge-%s.yaml", config.Name))

	//nolint:gosec // G301: netplan directory needs system access
	if err := os.MkdirAll(netplanDir, 0o755); err != nil {
		return fmt.Errorf("failed to create netplan directory: %w", err)
	}

	var content bytes.Buffer
	content.WriteString("# Managed by Keystone Core - do not edit manually\n")
	content.WriteString("network:\n")
	content.WriteString("  version: 2\n")

	// Define port interfaces as manual
	if len(config.Ports) > 0 {
		content.WriteString("  ethernets:\n")
		for _, port := range config.Ports {
			content.WriteString(fmt.Sprintf("    %s:\n", port))
			content.WriteString("      dhcp4: false\n")
			content.WriteString("      dhcp6: false\n")
		}
	}

	// Define bridge
	content.WriteString("  bridges:\n")
	content.WriteString(fmt.Sprintf("    %s:\n", config.Name))

	if len(config.Ports) > 0 {
		content.WriteString("      interfaces:\n")
		for _, port := range config.Ports {
			content.WriteString(fmt.Sprintf("        - %s\n", port))
		}
	}

	// Bridge parameters
	content.WriteString("      parameters:\n")
	if config.STP {
		content.WriteString("        stp: true\n")
	} else {
		content.WriteString("        stp: false\n")
	}
	content.WriteString(fmt.Sprintf("        forward-delay: %d\n", config.ForwardDelay))

	// IP configuration
	if len(config.Addresses) > 0 {
		content.WriteString("      addresses:\n")
		for _, addr := range config.Addresses {
			content.WriteString(fmt.Sprintf("        - %s\n", addr))
		}
	}

	if config.Gateway != "" {
		content.WriteString("      routes:\n")
		content.WriteString("        - to: default\n")
		content.WriteString(fmt.Sprintf("          via: %s\n", config.Gateway))
	}

	if len(config.DNS) > 0 {
		content.WriteString("      nameservers:\n")
		content.WriteString("        addresses:\n")
		for _, dns := range config.DNS {
			content.WriteString(fmt.Sprintf("          - %q\n", dns))
		}
	}

	if config.MTU > 0 {
		content.WriteString(fmt.Sprintf("      mtu: %d\n", config.MTU))
	}

	if err := os.WriteFile(netplanFile, content.Bytes(), 0o600); err != nil {
		return fmt.Errorf("failed to write netplan file: %w", err)
	}

	applyCmd := exec.CommandContext(ctx, "netplan", "apply")
	if output, err := applyCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to apply netplan: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Created bridge %s with ports %v via netplan",
		config.Name, config.Ports)
	return nil
}

// createBridgeSystemdNetworkd creates a bridge using systemd-networkd
func (m *BridgeModule) createBridgeSystemdNetworkd(ctx context.Context, config *BridgeConfig, result *StateResult) error {
	networkDir := "/etc/systemd/network"
	//nolint:gosec // G301: systemd network directory needs system access
	if err := os.MkdirAll(networkDir, 0o755); err != nil {
		return fmt.Errorf("failed to create network directory: %w", err)
	}

	// Create .netdev file
	netdevFile := filepath.Join(networkDir, fmt.Sprintf("10-kscore-bridge-%s.netdev", config.Name))
	var netdevContent bytes.Buffer
	netdevContent.WriteString("# Managed by Keystone Core - do not edit manually\n")
	netdevContent.WriteString("[NetDev]\n")
	netdevContent.WriteString(fmt.Sprintf("Name=%s\n", config.Name))
	netdevContent.WriteString("Kind=bridge\n")
	netdevContent.WriteString("\n[Bridge]\n")
	if config.STP {
		netdevContent.WriteString("STP=yes\n")
	} else {
		netdevContent.WriteString("STP=no\n")
	}
	netdevContent.WriteString(fmt.Sprintf("ForwardDelaySec=%d\n", config.ForwardDelay))
	netdevContent.WriteString(fmt.Sprintf("HelloTimeSec=%d\n", config.HelloTime))
	netdevContent.WriteString(fmt.Sprintf("MaxAgeSec=%d\n", config.MaxAge))
	netdevContent.WriteString(fmt.Sprintf("AgeingTimeSec=%d\n", config.AgeingTime))

	//nolint:gosec // G306: netdev files need to be readable by systemd-networkd
	if err := os.WriteFile(netdevFile, netdevContent.Bytes(), 0o644); err != nil {
		return fmt.Errorf("failed to write netdev file: %w", err)
	}

	// Create .network file for the bridge
	networkFile := filepath.Join(networkDir, fmt.Sprintf("20-kscore-bridge-%s.network", config.Name))
	var networkContent bytes.Buffer
	networkContent.WriteString("# Managed by Keystone Core - do not edit manually\n")
	networkContent.WriteString("[Match]\n")
	networkContent.WriteString(fmt.Sprintf("Name=%s\n", config.Name))
	networkContent.WriteString("\n[Network]\n")

	if len(config.Addresses) > 0 {
		for _, addr := range config.Addresses {
			networkContent.WriteString(fmt.Sprintf("Address=%s\n", addr))
		}
	}
	if config.Gateway != "" {
		networkContent.WriteString(fmt.Sprintf("Gateway=%s\n", config.Gateway))
	}
	for _, dns := range config.DNS {
		networkContent.WriteString(fmt.Sprintf("DNS=%s\n", dns))
	}

	if config.MTU > 0 {
		networkContent.WriteString("\n[Link]\n")
		networkContent.WriteString(fmt.Sprintf("MTUBytes=%d\n", config.MTU))
	}

	//nolint:gosec // G306: network files need to be readable by systemd-networkd
	if err := os.WriteFile(networkFile, networkContent.Bytes(), 0o644); err != nil {
		return fmt.Errorf("failed to write network file: %w", err)
	}

	// Create .network files for ports
	for _, port := range config.Ports {
		portFile := filepath.Join(networkDir, fmt.Sprintf("10-kscore-bridge-%s-port-%s.network", config.Name, port))
		var portContent bytes.Buffer
		portContent.WriteString("# Managed by Keystone Core - do not edit manually\n")
		portContent.WriteString("[Match]\n")
		portContent.WriteString(fmt.Sprintf("Name=%s\n", port))
		portContent.WriteString("\n[Network]\n")
		portContent.WriteString(fmt.Sprintf("Bridge=%s\n", config.Name))

		//nolint:gosec // G306: network files need to be readable by systemd-networkd
		if err := os.WriteFile(portFile, portContent.Bytes(), 0o644); err != nil {
			return fmt.Errorf("failed to write port network file: %w", err)
		}
	}

	// Reload systemd-networkd
	reloadCmd := exec.CommandContext(ctx, "networkctl", "reload")
	if output, err := reloadCmd.CombinedOutput(); err != nil {
		restartCmd := exec.CommandContext(ctx, "systemctl", "restart", "systemd-networkd")
		if output2, err2 := restartCmd.CombinedOutput(); err2 != nil {
			return fmt.Errorf("failed to reload networkd: %w (output: %s, %s)", err, string(output), string(output2))
		}
	}

	result.Comment = fmt.Sprintf("Created bridge %s with ports %v via systemd-networkd",
		config.Name, config.Ports)
	return nil
}

// createBridgeIfupdown creates a bridge using ifupdown
func (m *BridgeModule) createBridgeIfupdown(ctx context.Context, config *BridgeConfig, result *StateResult) error {
	interfacesFile := "/etc/network/interfaces"

	content, err := os.ReadFile(interfacesFile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read interfaces file: %w", err)
	}

	var bridgeStanza bytes.Buffer
	bridgeStanza.WriteString(fmt.Sprintf("\n# Bridge %s managed by Keystone Core\n", config.Name))
	bridgeStanza.WriteString(fmt.Sprintf("auto %s\n", config.Name))

	if len(config.Addresses) > 0 {
		bridgeStanza.WriteString(fmt.Sprintf("iface %s inet static\n", config.Name))
		addr := config.Addresses[0]
		if strings.Contains(addr, "/") {
			parts := strings.SplitN(addr, "/", 2)
			bridgeStanza.WriteString(fmt.Sprintf("    address %s\n", parts[0]))
			bridgeStanza.WriteString(fmt.Sprintf("    netmask %s\n", cidrToNetmask(parts[1])))
		} else {
			bridgeStanza.WriteString(fmt.Sprintf("    address %s\n", addr))
		}
		if config.Gateway != "" {
			bridgeStanza.WriteString(fmt.Sprintf("    gateway %s\n", config.Gateway))
		}
		if len(config.DNS) > 0 {
			bridgeStanza.WriteString(fmt.Sprintf("    dns-nameservers %s\n", strings.Join(config.DNS, " ")))
		}
	} else {
		bridgeStanza.WriteString(fmt.Sprintf("iface %s inet manual\n", config.Name))
	}

	if len(config.Ports) > 0 {
		bridgeStanza.WriteString(fmt.Sprintf("    bridge_ports %s\n", strings.Join(config.Ports, " ")))
	} else {
		bridgeStanza.WriteString("    bridge_ports none\n")
	}

	if config.STP {
		bridgeStanza.WriteString("    bridge_stp on\n")
	} else {
		bridgeStanza.WriteString("    bridge_stp off\n")
	}

	bridgeStanza.WriteString(fmt.Sprintf("    bridge_fd %d\n", config.ForwardDelay))
	bridgeStanza.WriteString(fmt.Sprintf("    bridge_hello %d\n", config.HelloTime))
	bridgeStanza.WriteString(fmt.Sprintf("    bridge_maxage %d\n", config.MaxAge))
	bridgeStanza.WriteString(fmt.Sprintf("    bridge_ageing %d\n", config.AgeingTime))

	if config.MTU > 0 {
		bridgeStanza.WriteString(fmt.Sprintf("    mtu %d\n", config.MTU))
	}

	newContent := string(content) + bridgeStanza.String()
	//nolint:gosec // G306: interfaces file needs to be readable by ifupdown
	if err := os.WriteFile(interfacesFile, []byte(newContent), 0o644); err != nil {
		return fmt.Errorf("failed to write interfaces file: %w", err)
	}

	// Bring up the bridge
	//nolint:gosec // G204: ifup execution is intentional for bridge interface management
	upCmd := exec.CommandContext(ctx, "ifup", config.Name)
	if output, err := upCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to bring up bridge: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Created bridge %s with ports %v via ifupdown",
		config.Name, config.Ports)
	return nil
}

// createBridgeRaw creates a bridge using raw ip commands
func (m *BridgeModule) createBridgeRaw(ctx context.Context, config *BridgeConfig, result *StateResult) error {
	// Create bridge interface
	//nolint:gosec // G204: ip command execution is intentional for bridge interface management
	cmd := exec.CommandContext(ctx, "ip", "link", "add", config.Name, "type", "bridge")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create bridge: %w (output: %s)", err, string(output))
	}

	// Set STP
	stpVal := "0"
	if config.STP {
		stpVal = "1"
	}
	//nolint:gosec // G204: ip command execution is intentional for bridge interface management
	stpCmd := exec.CommandContext(ctx, "ip", "link", "set", config.Name, "type", "bridge", "stp_state", stpVal)
	stpCmd.Run()

	// Add ports
	for _, port := range config.Ports {
		// Bring down port first
		//nolint:gosec // G204: ip command execution is intentional for bridge interface management
		downCmd := exec.CommandContext(ctx, "ip", "link", "set", port, "down")
		downCmd.Run()
		// Add to bridge
		//nolint:gosec // G204: ip command execution is intentional for bridge interface management
		portCmd := exec.CommandContext(ctx, "ip", "link", "set", port, "master", config.Name)
		if output, err := portCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to add port %s: %w (output: %s)", port, err, string(output))
		}
		// Bring port back up
		//nolint:gosec // G204: ip command execution is intentional for bridge interface management
		upPortCmd := exec.CommandContext(ctx, "ip", "link", "set", port, "up")
		upPortCmd.Run()
	}

	// Set MTU if specified
	if config.MTU > 0 {
		//nolint:gosec // G204: ip command execution is intentional for bridge interface management
		mtuCmd := exec.CommandContext(ctx, "ip", "link", "set", config.Name, "mtu", strconv.Itoa(config.MTU))
		mtuCmd.Run()
	}

	// Add addresses
	for _, addr := range config.Addresses {
		//nolint:gosec // G204: ip command execution is intentional for bridge interface management
		addrCmd := exec.CommandContext(ctx, "ip", "addr", "add", addr, "dev", config.Name)
		addrCmd.Run()
	}

	// Bring up bridge
	//nolint:gosec // G204: ip command execution is intentional for bridge interface management
	upCmd := exec.CommandContext(ctx, "ip", "link", "set", config.Name, "up")
	if output, err := upCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to bring up bridge: %w (output: %s)", err, string(output))
	}

	// Add default route if gateway specified
	if config.Gateway != "" && len(config.Addresses) > 0 {
		//nolint:gosec // G204: ip command execution is intentional for bridge interface management
		routeCmd := exec.CommandContext(ctx, "ip", "route", "add", "default", "via", config.Gateway, "dev", config.Name)
		routeCmd.Run()
	}

	result.Comment = fmt.Sprintf("Created bridge %s with ports %v via ip commands",
		config.Name, config.Ports)
	return nil
}

// createBridgeWindows creates a Hyper-V switch on Windows
func (m *BridgeModule) createBridgeWindows(ctx context.Context, config *BridgeConfig, result *StateResult) error {
	// Create Hyper-V switch
	// Note: This requires Hyper-V to be installed
	var cmd *exec.Cmd
	if len(config.Ports) > 0 {
		//nolint:gosec // G204: PowerShell execution is intentional for Windows bridge management
		cmd = exec.CommandContext(ctx, "powershell", "-Command",
			fmt.Sprintf("New-VMSwitch -Name '%s' -NetAdapterName '%s' -AllowManagementOS $true",
				config.Name, config.Ports[0]))
	} else {
		//nolint:gosec // G204: PowerShell execution is intentional for Windows bridge management
		cmd = exec.CommandContext(ctx, "powershell", "-Command",
			fmt.Sprintf("New-VMSwitch -Name '%s' -SwitchType Internal",
				config.Name))
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create Hyper-V switch: %w (output: %s)", err, string(output))
	}

	// Set IP if specified
	if len(config.Addresses) > 0 {
		// Find the adapter created by the switch
		adapterName := fmt.Sprintf("vEthernet (%s)", config.Name)
		addr := config.Addresses[0]
		ip := addr
		prefix := "24"
		if strings.Contains(addr, "/") {
			parts := strings.SplitN(addr, "/", 2)
			ip = parts[0]
			prefix = parts[1]
		}

		//nolint:gosec // G204: PowerShell execution is intentional for Windows bridge management
		ipCmd := exec.CommandContext(ctx, "powershell", "-Command",
			fmt.Sprintf("New-NetIPAddress -InterfaceAlias '%s' -IPAddress '%s' -PrefixLength %s",
				adapterName, ip, prefix))
		ipCmd.Run()

		if config.Gateway != "" {
			//nolint:gosec // G204: PowerShell execution is intentional for Windows bridge management
			gwCmd := exec.CommandContext(ctx, "powershell", "-Command",
				fmt.Sprintf("New-NetRoute -InterfaceAlias '%s' -DestinationPrefix '0.0.0.0/0' -NextHop '%s'",
					adapterName, config.Gateway))
			gwCmd.Run()
		}
	}

	result.Comment = fmt.Sprintf("Created Hyper-V switch %s on Windows", config.Name)
	return nil
}

// deleteBridgeWindows deletes a Hyper-V switch on Windows
func (m *BridgeModule) deleteBridgeWindows(ctx context.Context, config *BridgeConfig) error {
	//nolint:gosec // G204: PowerShell execution is intentional for Windows bridge management
	cmd := exec.CommandContext(ctx, "powershell", "-Command",
		fmt.Sprintf("Remove-VMSwitch -Name '%s' -Force", config.Name))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete Hyper-V switch: %w (output: %s)", err, string(output))
	}
	return nil
}

func init() {
	_ = RegisterModule(NewBridgeModule()) //nolint:errcheck // module registration in init
}
