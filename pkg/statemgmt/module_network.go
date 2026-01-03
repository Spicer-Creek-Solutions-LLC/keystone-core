package statemgmt

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// NetworkModule implements network interface configuration
type NetworkModule struct {
	*BaseModule
}

// NewNetworkModule creates a new network module
func NewNetworkModule() *NetworkModule {
	return &NetworkModule{
		BaseModule: NewBaseModule("network", []string{"configured", "absent", "dhcp"}),
	}
}

// NetworkManager represents different network management systems
type NetworkManager string

const (
	NMUnknown         NetworkManager = "unknown"
	NMNetworkManager  NetworkManager = "networkmanager"  // nmcli
	NMNetplan         NetworkManager = "netplan"         // Ubuntu netplan
	NMIfupdown        NetworkManager = "ifupdown"        // Debian interfaces
	NMSystemdNetworkd NetworkManager = "systemd-networkd"
	NMNetworkSetup    NetworkManager = "networksetup"    // macOS
	NMNetsh           NetworkManager = "netsh"           // Windows
)

// NetworkConfig holds network configuration parameters
type NetworkConfig struct {
	Interface   string   // Network interface name
	Address     string   // IP address (CIDR notation or just IP)
	Netmask     string   // Subnet mask (if not in CIDR)
	Gateway     string   // Default gateway
	DNS         []string // DNS servers
	MTU         int      // Maximum transmission unit
	DHCP        bool     // Use DHCP
	Metric      int      // Route metric
	SearchDomains []string // DNS search domains
}

// Check checks the current state of a network interface
func (m *NetworkModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	config, err := m.parseNetworkConfig(decl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse network config: %w", err)
	}

	// Check if interface exists
	iface, err := net.InterfaceByName(config.Interface)
	if err != nil {
		result.Present = false
		result.CurrentState = "absent"
		result.Matches = decl.State == "absent"
		return result, nil
	}

	result.Present = true
	result.Metadata["mac"] = iface.HardwareAddr.String()
	result.Metadata["mtu"] = iface.MTU
	result.Metadata["flags"] = iface.Flags.String()

	// Get current addresses
	addrs, err := iface.Addrs()
	if err == nil {
		var addrStrings []string
		for _, addr := range addrs {
			addrStrings = append(addrStrings, addr.String())
		}
		result.Metadata["addresses"] = addrStrings
	}

	// Detect current configuration
	currentConfig, err := m.getCurrentConfig(ctx, config.Interface)
	if err != nil {
		// Can't determine current config, assume mismatch
		result.CurrentState = "unknown"
		result.Matches = false
		return result, nil
	}

	result.CurrentState = "configured"
	if currentConfig.DHCP {
		result.CurrentState = "dhcp"
	}

	// Compare configurations
	switch decl.State {
	case "configured":
		result.Matches = m.configMatches(config, currentConfig, result)
	case "dhcp":
		result.Matches = currentConfig.DHCP
		if !currentConfig.DHCP {
			result.Diff["dhcp"] = map[string]bool{"current": false, "desired": true}
		}
	case "absent":
		// Interface exists, so doesn't match "absent"
		result.Matches = false
		result.Diff["interface"] = map[string]string{"current": "present", "desired": "absent"}
	}

	return result, nil
}

// Apply applies the network configuration
func (m *NetworkModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	result := &StateResult{
		StateID:   decl.ID,
		Module:    m.Name(),
		Success:   false,
		Changed:   false,
		Changes:   make(map[string]interface{}),
		StartTime: startTime,
	}

	config, err := m.parseNetworkConfig(decl)
	if err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to parse config: %v", err)
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

	// Detect network manager
	nm, err := m.detectNetworkManager()
	if err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to detect network manager: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// Apply configuration
	var applyErr error
	switch decl.State {
	case "configured":
		applyErr = m.applyStaticConfig(ctx, nm, config, result)
	case "dhcp":
		applyErr = m.applyDHCPConfig(ctx, nm, config, result)
	case "absent":
		applyErr = m.removeConfig(ctx, nm, config, result)
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

// Test tests if the network interface is in the desired state
func (m *NetworkModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil
}

// parseNetworkConfig extracts network configuration from declaration parameters
func (m *NetworkModule) parseNetworkConfig(decl *StateDeclaration) (*NetworkConfig, error) {
	config := &NetworkConfig{
		Interface: decl.ID,
	}

	if iface := getStringParameter(decl, "interface", ""); iface != "" {
		config.Interface = iface
	}

	config.Address = getStringParameter(decl, "address", "")
	config.Netmask = getStringParameter(decl, "netmask", "")
	config.Gateway = getStringParameter(decl, "gateway", "")
	config.MTU = getIntParameter(decl, "mtu", 0)
	config.Metric = getIntParameter(decl, "metric", 0)
	config.DHCP = getBoolParameter(decl, "dhcp", false)

	// Parse DNS servers
	if dnsParam := decl.Parameters["dns"]; dnsParam != nil {
		switch v := dnsParam.(type) {
		case string:
			config.DNS = strings.Split(v, ",")
			for i := range config.DNS {
				config.DNS[i] = strings.TrimSpace(config.DNS[i])
			}
		case []interface{}:
			for _, d := range v {
				if s, ok := d.(string); ok {
					config.DNS = append(config.DNS, s)
				}
			}
		case []string:
			config.DNS = v
		}
	}

	// Parse search domains
	if searchParam := decl.Parameters["search_domains"]; searchParam != nil {
		switch v := searchParam.(type) {
		case string:
			config.SearchDomains = strings.Split(v, ",")
			for i := range config.SearchDomains {
				config.SearchDomains[i] = strings.TrimSpace(config.SearchDomains[i])
			}
		case []interface{}:
			for _, d := range v {
				if s, ok := d.(string); ok {
					config.SearchDomains = append(config.SearchDomains, s)
				}
			}
		case []string:
			config.SearchDomains = v
		}
	}

	return config, nil
}

// detectNetworkManager detects the available network manager
func (m *NetworkModule) detectNetworkManager() (NetworkManager, error) {
	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("networksetup"); err == nil {
			return NMNetworkSetup, nil
		}
		return NMUnknown, fmt.Errorf("networksetup not found on macOS")

	case "windows":
		if _, err := exec.LookPath("netsh"); err == nil {
			return NMNetsh, nil
		}
		return NMUnknown, fmt.Errorf("netsh not found on Windows")

	case "linux":
		return m.detectLinuxNetworkManager()

	default:
		return NMUnknown, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// detectLinuxNetworkManager detects the Linux network manager
func (m *NetworkModule) detectLinuxNetworkManager() (NetworkManager, error) {
	// Check for NetworkManager (nmcli)
	if _, err := exec.LookPath("nmcli"); err == nil {
		// Verify NetworkManager is running
		cmd := exec.Command("systemctl", "is-active", "NetworkManager")
		if err := cmd.Run(); err == nil {
			return NMNetworkManager, nil
		}
	}

	// Check for netplan
	if _, err := exec.LookPath("netplan"); err == nil {
		return NMNetplan, nil
	}

	// Check for systemd-networkd
	cmd := exec.Command("systemctl", "is-active", "systemd-networkd")
	if err := cmd.Run(); err == nil {
		return NMSystemdNetworkd, nil
	}

	// Check for ifupdown
	if _, err := exec.LookPath("ifup"); err == nil {
		return NMIfupdown, nil
	}

	return NMUnknown, fmt.Errorf("no supported network manager found on Linux")
}

// getCurrentConfig gets the current network configuration for an interface
func (m *NetworkModule) getCurrentConfig(ctx context.Context, ifaceName string) (*NetworkConfig, error) {
	switch runtime.GOOS {
	case "darwin":
		return m.getCurrentConfigDarwin(ctx, ifaceName)
	case "windows":
		return m.getCurrentConfigWindows(ctx, ifaceName)
	case "linux":
		return m.getCurrentConfigLinux(ctx, ifaceName)
	default:
		return nil, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// getCurrentConfigLinux gets current Linux network config
func (m *NetworkModule) getCurrentConfigLinux(ctx context.Context, ifaceName string) (*NetworkConfig, error) {
	config := &NetworkConfig{
		Interface: ifaceName,
	}

	// Get IP address using ip command
	cmd := exec.CommandContext(ctx, "ip", "-o", "-4", "addr", "show", ifaceName)
	output, err := cmd.Output()
	if err != nil {
		return config, nil
	}

	// Parse: 2: eth0    inet 192.168.1.100/24 brd 192.168.1.255 scope global eth0
	fields := strings.Fields(string(output))
	for i, f := range fields {
		if f == "inet" && i+1 < len(fields) {
			config.Address = fields[i+1]
			break
		}
	}

	// Get default gateway
	cmd = exec.CommandContext(ctx, "ip", "route", "show", "default")
	output, err = cmd.Output()
	if err == nil {
		// Parse: default via 192.168.1.1 dev eth0
		fields := strings.Fields(string(output))
		for i, f := range fields {
			if f == "via" && i+1 < len(fields) {
				config.Gateway = fields[i+1]
				break
			}
		}
	}

	// Check if DHCP (look for dhclient or dhcpcd process)
	cmd = exec.CommandContext(ctx, "pgrep", "-f", fmt.Sprintf("dhclient.*%s|dhcpcd.*%s", ifaceName, ifaceName))
	if err := cmd.Run(); err == nil {
		config.DHCP = true
	}

	return config, nil
}

// getCurrentConfigDarwin gets current macOS network config
func (m *NetworkModule) getCurrentConfigDarwin(ctx context.Context, ifaceName string) (*NetworkConfig, error) {
	config := &NetworkConfig{
		Interface: ifaceName,
	}

	// Get service name for the interface
	serviceName, err := m.getNetworkServiceName(ctx, ifaceName)
	if err != nil {
		return config, nil
	}

	// Get IP address
	cmd := exec.CommandContext(ctx, "networksetup", "-getinfo", serviceName)
	output, err := cmd.Output()
	if err != nil {
		return config, nil
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "IP address:") {
			config.Address = strings.TrimPrefix(line, "IP address:")
			config.Address = strings.TrimSpace(config.Address)
		} else if strings.HasPrefix(line, "Subnet mask:") {
			config.Netmask = strings.TrimPrefix(line, "Subnet mask:")
			config.Netmask = strings.TrimSpace(config.Netmask)
		} else if strings.HasPrefix(line, "Router:") {
			config.Gateway = strings.TrimPrefix(line, "Router:")
			config.Gateway = strings.TrimSpace(config.Gateway)
		} else if strings.Contains(line, "DHCP Configuration") {
			config.DHCP = true
		}
	}

	// Get DNS
	cmd = exec.CommandContext(ctx, "networksetup", "-getdnsservers", serviceName)
	output, err = cmd.Output()
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && !strings.Contains(line, "aren't any") {
				config.DNS = append(config.DNS, line)
			}
		}
	}

	return config, nil
}

// getNetworkServiceName gets the macOS network service name for an interface
func (m *NetworkModule) getNetworkServiceName(ctx context.Context, ifaceName string) (string, error) {
	cmd := exec.CommandContext(ctx, "networksetup", "-listallhardwareports")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(output), "\n")
	var currentService string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Hardware Port:") {
			currentService = strings.TrimPrefix(line, "Hardware Port:")
			currentService = strings.TrimSpace(currentService)
		} else if strings.HasPrefix(line, "Device:") {
			device := strings.TrimPrefix(line, "Device:")
			device = strings.TrimSpace(device)
			if device == ifaceName {
				return currentService, nil
			}
		}
	}

	return "", fmt.Errorf("no service found for interface %s", ifaceName)
}

// getCurrentConfigWindows gets current Windows network config
func (m *NetworkModule) getCurrentConfigWindows(ctx context.Context, ifaceName string) (*NetworkConfig, error) {
	config := &NetworkConfig{
		Interface: ifaceName,
	}

	// Get IP configuration
	cmd := exec.CommandContext(ctx, "netsh", "interface", "ip", "show", "config", "name="+ifaceName)
	output, err := cmd.Output()
	if err != nil {
		return config, nil
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "DHCP enabled:") && strings.Contains(line, "Yes") {
			config.DHCP = true
		} else if strings.Contains(line, "IP Address:") || strings.Contains(line, "IP address:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				config.Address = strings.TrimSpace(parts[1])
			}
		} else if strings.Contains(line, "Subnet Prefix:") || strings.Contains(line, "Subnet mask:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				config.Netmask = strings.TrimSpace(parts[1])
			}
		} else if strings.Contains(line, "Default Gateway:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				config.Gateway = strings.TrimSpace(parts[1])
			}
		}
	}

	// Get DNS
	cmd = exec.CommandContext(ctx, "netsh", "interface", "ip", "show", "dns", "name="+ifaceName)
	output, err = cmd.Output()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			// Look for DNS server lines
			if strings.Contains(line, "Statically Configured DNS Servers:") ||
			   strings.Contains(line, "DNS servers configured through DHCP:") {
				continue
			}
			// Check if line looks like an IP address
			if net.ParseIP(line) != nil {
				config.DNS = append(config.DNS, line)
			}
		}
	}

	return config, nil
}

// configMatches checks if configurations match
func (m *NetworkModule) configMatches(desired, current *NetworkConfig, result *ModuleCheckResult) bool {
	matches := true

	// Compare address (normalize CIDR)
	if desired.Address != "" {
		desiredAddr := normalizeAddress(desired.Address, desired.Netmask)
		currentAddr := normalizeAddress(current.Address, current.Netmask)
		if desiredAddr != currentAddr {
			matches = false
			result.Diff["address"] = map[string]string{"current": currentAddr, "desired": desiredAddr}
		}
	}

	// Compare gateway
	if desired.Gateway != "" && desired.Gateway != current.Gateway {
		matches = false
		result.Diff["gateway"] = map[string]string{"current": current.Gateway, "desired": desired.Gateway}
	}

	// Compare DNS (order matters)
	if len(desired.DNS) > 0 {
		if !stringSlicesEqual(desired.DNS, current.DNS) {
			matches = false
			result.Diff["dns"] = map[string]interface{}{"current": current.DNS, "desired": desired.DNS}
		}
	}

	// Compare MTU
	if desired.MTU > 0 {
		iface, err := net.InterfaceByName(desired.Interface)
		if err == nil && iface.MTU != desired.MTU {
			matches = false
			result.Diff["mtu"] = map[string]int{"current": iface.MTU, "desired": desired.MTU}
		}
	}

	return matches
}

// normalizeAddress normalizes an address to CIDR notation
func normalizeAddress(addr, netmask string) string {
	if addr == "" {
		return ""
	}

	// If already in CIDR notation, return as-is
	if strings.Contains(addr, "/") {
		return addr
	}

	// Convert netmask to CIDR prefix
	if netmask != "" {
		mask := net.ParseIP(netmask)
		if mask != nil {
			ones, _ := net.IPMask(mask.To4()).Size()
			return fmt.Sprintf("%s/%d", addr, ones)
		}
	}

	return addr
}

// applyStaticConfig applies static IP configuration
func (m *NetworkModule) applyStaticConfig(ctx context.Context, nm NetworkManager, config *NetworkConfig, result *StateResult) error {
	switch nm {
	case NMNetworkManager:
		return m.applyStaticConfigNmcli(ctx, config, result)
	case NMNetworkSetup:
		return m.applyStaticConfigDarwin(ctx, config, result)
	case NMNetsh:
		return m.applyStaticConfigWindows(ctx, config, result)
	case NMIfupdown:
		return m.applyStaticConfigIfupdown(ctx, config, result)
	case NMSystemdNetworkd:
		return m.applyStaticConfigSystemdNetworkd(ctx, config, result)
	case NMNetplan:
		return m.applyStaticConfigNetplan(ctx, config, result)
	default:
		return fmt.Errorf("unsupported network manager: %s", nm)
	}
}

// applyDHCPConfig applies DHCP configuration
func (m *NetworkModule) applyDHCPConfig(ctx context.Context, nm NetworkManager, config *NetworkConfig, result *StateResult) error {
	switch nm {
	case NMNetworkManager:
		return m.applyDHCPConfigNmcli(ctx, config, result)
	case NMNetworkSetup:
		return m.applyDHCPConfigDarwin(ctx, config, result)
	case NMNetsh:
		return m.applyDHCPConfigWindows(ctx, config, result)
	default:
		return fmt.Errorf("DHCP configuration not supported for %s", nm)
	}
}

// removeConfig removes network configuration
func (m *NetworkModule) removeConfig(ctx context.Context, nm NetworkManager, config *NetworkConfig, result *StateResult) error {
	switch nm {
	case NMNetworkManager:
		// Delete connection profile
		cmd := exec.CommandContext(ctx, "nmcli", "connection", "delete", config.Interface)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to remove connection: %w (output: %s)", err, string(output))
		}
		result.Comment = fmt.Sprintf("Removed network configuration for %s", config.Interface)
		return nil
	default:
		return fmt.Errorf("remove not supported for %s", nm)
	}
}

// applyStaticConfigNmcli applies static config using nmcli
func (m *NetworkModule) applyStaticConfigNmcli(ctx context.Context, config *NetworkConfig, result *StateResult) error {
	// Check if connection exists
	checkCmd := exec.CommandContext(ctx, "nmcli", "-t", "-f", "NAME", "connection", "show")
	output, _ := checkCmd.Output()
	connectionExists := strings.Contains(string(output), config.Interface)

	var cmd *exec.Cmd
	if connectionExists {
		// Modify existing connection
		args := []string{"connection", "modify", config.Interface}
		if config.Address != "" {
			args = append(args, "ipv4.addresses", config.Address)
		}
		if config.Gateway != "" {
			args = append(args, "ipv4.gateway", config.Gateway)
		}
		if len(config.DNS) > 0 {
			args = append(args, "ipv4.dns", strings.Join(config.DNS, ","))
		}
		args = append(args, "ipv4.method", "manual")
		cmd = exec.CommandContext(ctx, "nmcli", args...)
	} else {
		// Create new connection
		args := []string{"connection", "add", "type", "ethernet", "con-name", config.Interface, "ifname", config.Interface}
		if config.Address != "" {
			args = append(args, "ipv4.addresses", config.Address)
		}
		if config.Gateway != "" {
			args = append(args, "ipv4.gateway", config.Gateway)
		}
		if len(config.DNS) > 0 {
			args = append(args, "ipv4.dns", strings.Join(config.DNS, ","))
		}
		args = append(args, "ipv4.method", "manual")
		cmd = exec.CommandContext(ctx, "nmcli", args...)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nmcli failed: %w (output: %s)", err, string(output))
	}

	// Bring up the connection
	upCmd := exec.CommandContext(ctx, "nmcli", "connection", "up", config.Interface)
	if output, err := upCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to activate connection: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Configured %s with static IP %s", config.Interface, config.Address)
	return nil
}

// applyDHCPConfigNmcli applies DHCP config using nmcli
func (m *NetworkModule) applyDHCPConfigNmcli(ctx context.Context, config *NetworkConfig, result *StateResult) error {
	// Check if connection exists
	checkCmd := exec.CommandContext(ctx, "nmcli", "-t", "-f", "NAME", "connection", "show")
	output, _ := checkCmd.Output()
	connectionExists := strings.Contains(string(output), config.Interface)

	var cmd *exec.Cmd
	if connectionExists {
		cmd = exec.CommandContext(ctx, "nmcli", "connection", "modify", config.Interface, "ipv4.method", "auto")
	} else {
		cmd = exec.CommandContext(ctx, "nmcli", "connection", "add", "type", "ethernet",
			"con-name", config.Interface, "ifname", config.Interface, "ipv4.method", "auto")
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nmcli failed: %w (output: %s)", err, string(output))
	}

	// Bring up the connection
	upCmd := exec.CommandContext(ctx, "nmcli", "connection", "up", config.Interface)
	if output, err := upCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to activate connection: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Configured %s with DHCP", config.Interface)
	return nil
}

// applyStaticConfigDarwin applies static config on macOS
func (m *NetworkModule) applyStaticConfigDarwin(ctx context.Context, config *NetworkConfig, result *StateResult) error {
	serviceName, err := m.getNetworkServiceName(ctx, config.Interface)
	if err != nil {
		return fmt.Errorf("failed to get service name: %w", err)
	}

	// Parse address and netmask
	addr := config.Address
	netmask := config.Netmask
	if strings.Contains(addr, "/") {
		parts := strings.SplitN(addr, "/", 2)
		addr = parts[0]
		// Convert CIDR to netmask
		prefix := parts[1]
		netmask = cidrToNetmask(prefix)
	}

	// Set manual IP
	cmd := exec.CommandContext(ctx, "networksetup", "-setmanual", serviceName, addr, netmask, config.Gateway)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set IP: %w (output: %s)", err, string(output))
	}

	// Set DNS if specified
	if len(config.DNS) > 0 {
		args := append([]string{"-setdnsservers", serviceName}, config.DNS...)
		cmd = exec.CommandContext(ctx, "networksetup", args...)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to set DNS: %w (output: %s)", err, string(output))
		}
	}

	result.Comment = fmt.Sprintf("Configured %s (%s) with static IP %s", serviceName, config.Interface, addr)
	return nil
}

// applyDHCPConfigDarwin applies DHCP config on macOS
func (m *NetworkModule) applyDHCPConfigDarwin(ctx context.Context, config *NetworkConfig, result *StateResult) error {
	serviceName, err := m.getNetworkServiceName(ctx, config.Interface)
	if err != nil {
		return fmt.Errorf("failed to get service name: %w", err)
	}

	cmd := exec.CommandContext(ctx, "networksetup", "-setdhcp", serviceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set DHCP: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Configured %s (%s) with DHCP", serviceName, config.Interface)
	return nil
}

// applyStaticConfigWindows applies static config on Windows
func (m *NetworkModule) applyStaticConfigWindows(ctx context.Context, config *NetworkConfig, result *StateResult) error {
	// Parse address
	addr := config.Address
	netmask := config.Netmask
	if strings.Contains(addr, "/") {
		parts := strings.SplitN(addr, "/", 2)
		addr = parts[0]
		netmask = cidrToNetmask(parts[1])
	}

	// Set static IP
	cmd := exec.CommandContext(ctx, "netsh", "interface", "ip", "set", "address",
		"name="+config.Interface, "static", addr, netmask, config.Gateway)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set IP: %w (output: %s)", err, string(output))
	}

	// Set DNS if specified
	if len(config.DNS) > 0 {
		// Set primary DNS
		cmd = exec.CommandContext(ctx, "netsh", "interface", "ip", "set", "dns",
			"name="+config.Interface, "static", config.DNS[0])
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to set primary DNS: %w (output: %s)", err, string(output))
		}

		// Add secondary DNS servers
		for i := 1; i < len(config.DNS); i++ {
			cmd = exec.CommandContext(ctx, "netsh", "interface", "ip", "add", "dns",
				"name="+config.Interface, config.DNS[i], "index="+fmt.Sprintf("%d", i+1))
			if output, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to add DNS %s: %w (output: %s)", config.DNS[i], err, string(output))
			}
		}
	}

	result.Comment = fmt.Sprintf("Configured %s with static IP %s", config.Interface, addr)
	return nil
}

// applyDHCPConfigWindows applies DHCP config on Windows
func (m *NetworkModule) applyDHCPConfigWindows(ctx context.Context, config *NetworkConfig, result *StateResult) error {
	// Set DHCP for IP
	cmd := exec.CommandContext(ctx, "netsh", "interface", "ip", "set", "address",
		"name="+config.Interface, "dhcp")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set DHCP: %w (output: %s)", err, string(output))
	}

	// Set DHCP for DNS
	cmd = exec.CommandContext(ctx, "netsh", "interface", "ip", "set", "dns",
		"name="+config.Interface, "dhcp")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set DNS DHCP: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Configured %s with DHCP", config.Interface)
	return nil
}

// applyStaticConfigIfupdown applies static config using ifupdown
func (m *NetworkModule) applyStaticConfigIfupdown(ctx context.Context, config *NetworkConfig, result *StateResult) error {
	// This would modify /etc/network/interfaces
	// For now, return an error indicating manual configuration is needed
	return fmt.Errorf("ifupdown configuration requires manual file editing - not implemented")
}

// applyStaticConfigSystemdNetworkd applies static config using systemd-networkd
func (m *NetworkModule) applyStaticConfigSystemdNetworkd(ctx context.Context, config *NetworkConfig, result *StateResult) error {
	// This would create .network files in /etc/systemd/network/
	// For now, return an error indicating manual configuration is needed
	return fmt.Errorf("systemd-networkd configuration requires file creation - not implemented")
}

// applyStaticConfigNetplan applies static config using netplan
func (m *NetworkModule) applyStaticConfigNetplan(ctx context.Context, config *NetworkConfig, result *StateResult) error {
	// This would create YAML files in /etc/netplan/
	// For now, return an error indicating manual configuration is needed
	return fmt.Errorf("netplan configuration requires file creation - not implemented")
}

// cidrToNetmask converts CIDR prefix to dotted netmask
func cidrToNetmask(prefix string) string {
	var bits int
	fmt.Sscanf(prefix, "%d", &bits)
	mask := net.CIDRMask(bits, 32)
	return fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])
}

func init() {
	RegisterModule(NewNetworkModule())
}
