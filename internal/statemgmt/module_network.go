package statemgmt

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
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
	Interface     string   // Network interface name
	Addresses     []string // IPv4 addresses (CIDR notation or just IP)
	Netmask       string   // IPv4 subnet mask (if not in CIDR, applies to first address)
	Gateway       string   // IPv4 default gateway
	DNS           []string // DNS servers (IPv4 and/or IPv6)
	MTU           int      // Maximum transmission unit
	DHCP          bool     // Use DHCP for IPv4
	Metric        int      // Route metric
	SearchDomains []string // DNS search domains

	// IPv6 configuration
	Addresses6  []string // IPv6 addresses (CIDR notation)
	Gateway6    string   // IPv6 default gateway
	DHCP6       bool     // Use DHCPv6 (or SLAAC if false but IPv6 enabled)
	IPv6Enabled bool     // Enable IPv6 on the interface
	IPv6Privacy bool     // Enable IPv6 privacy extensions (RFC 4941)
	AcceptRA    *bool    // Accept Router Advertisements (nil = system default)

	// Link-layer configuration
	MACAddress string // Override MAC address (format: xx:xx:xx:xx:xx:xx)
	WakeOnLAN  string // Wake-on-LAN mode: magic, unicast, multicast, broadcast, arp, off
}

// PrimaryAddress returns the first IPv4 address or empty string
func (c *NetworkConfig) PrimaryAddress() string {
	if len(c.Addresses) > 0 {
		return c.Addresses[0]
	}
	return ""
}

// PrimaryAddress6 returns the first IPv6 address or empty string
func (c *NetworkConfig) PrimaryAddress6() string {
	if len(c.Addresses6) > 0 {
		return c.Addresses6[0]
	}
	return ""
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

	// Parse IPv4 addresses (supports single string, comma-separated, or array)
	config.Addresses = parseStringOrArray(decl.Parameters["address"])
	// Also check plural form
	if addrs := parseStringOrArray(decl.Parameters["addresses"]); len(addrs) > 0 {
		config.Addresses = append(config.Addresses, addrs...)
	}

	config.Netmask = getStringParameter(decl, "netmask", "")
	config.Gateway = getStringParameter(decl, "gateway", "")
	config.MTU = getIntParameter(decl, "mtu", 0)
	config.Metric = getIntParameter(decl, "metric", 0)
	config.DHCP = getBoolParameter(decl, "dhcp", false)

	// Parse DNS servers
	config.DNS = parseStringOrArray(decl.Parameters["dns"])

	// Parse search domains
	config.SearchDomains = parseStringOrArray(decl.Parameters["search_domains"])

	// Parse IPv6 addresses (supports single string, comma-separated, or array)
	config.Addresses6 = parseStringOrArray(decl.Parameters["address6"])
	// Also check plural form
	if addrs6 := parseStringOrArray(decl.Parameters["addresses6"]); len(addrs6) > 0 {
		config.Addresses6 = append(config.Addresses6, addrs6...)
	}

	config.Gateway6 = getStringParameter(decl, "gateway6", "")
	config.DHCP6 = getBoolParameter(decl, "dhcp6", false)
	config.IPv6Enabled = getBoolParameter(decl, "ipv6_enabled", false)
	config.IPv6Privacy = getBoolParameter(decl, "ipv6_privacy", false)

	// Auto-enable IPv6 if any IPv6 config is specified
	if len(config.Addresses6) > 0 || config.Gateway6 != "" || config.DHCP6 {
		config.IPv6Enabled = true
	}

	// Parse accept_ra (nil means system default)
	if raParam, ok := decl.Parameters["accept_ra"]; ok {
		if ra, ok := raParam.(bool); ok {
			config.AcceptRA = &ra
		}
	}

	// Parse link-layer configuration
	config.MACAddress = getStringParameter(decl, "mac_address", "")
	config.WakeOnLAN = getStringParameter(decl, "wol", "")
	// Also accept wake_on_lan as alternate name
	if config.WakeOnLAN == "" {
		config.WakeOnLAN = getStringParameter(decl, "wake_on_lan", "")
	}

	return config, nil
}

// parseStringOrArray parses a parameter that can be a string, comma-separated string, or array
func parseStringOrArray(param interface{}) []string {
	if param == nil {
		return nil
	}

	switch v := param.(type) {
	case string:
		if v == "" {
			return nil
		}
		parts := strings.Split(v, ",")
		result := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				result = append(result, p)
			}
		}
		return result
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				result = append(result, s)
			}
		}
		return result
	case []string:
		result := make([]string, 0, len(v))
		for _, s := range v {
			if s != "" {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
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

	// Get IPv4 addresses using ip command
	cmd := exec.CommandContext(ctx, "ip", "-o", "-4", "addr", "show", ifaceName)
	output, err := cmd.Output()
	if err == nil {
		// Parse: 2: eth0    inet 192.168.1.100/24 brd 192.168.1.255 scope global eth0
		// There may be multiple lines for multiple addresses
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, line := range lines {
			fields := strings.Fields(line)
			for i, f := range fields {
				if f == "inet" && i+1 < len(fields) {
					config.Addresses = append(config.Addresses, fields[i+1])
					break
				}
			}
		}
	}

	// Get IPv6 addresses using ip command
	cmd = exec.CommandContext(ctx, "ip", "-o", "-6", "addr", "show", ifaceName)
	output, err = cmd.Output()
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, line := range lines {
			fields := strings.Fields(line)
			for i, f := range fields {
				if f == "inet6" && i+1 < len(fields) {
					addr := fields[i+1]
					// Skip link-local addresses (fe80::)
					if !strings.HasPrefix(addr, "fe80:") {
						config.Addresses6 = append(config.Addresses6, addr)
					}
					break
				}
			}
		}
	}

	// Get default gateway (IPv4)
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

	// Get default gateway (IPv6)
	cmd = exec.CommandContext(ctx, "ip", "-6", "route", "show", "default")
	output, err = cmd.Output()
	if err == nil {
		fields := strings.Fields(string(output))
		for i, f := range fields {
			if f == "via" && i+1 < len(fields) {
				config.Gateway6 = fields[i+1]
				break
			}
		}
	}

	// Check if DHCP (look for dhclient or dhcpcd process)
	cmd = exec.CommandContext(ctx, "pgrep", "-f", fmt.Sprintf("dhclient.*%s|dhcpcd.*%s", ifaceName, ifaceName))
	if err := cmd.Run(); err == nil {
		config.DHCP = true
	}

	if len(config.Addresses6) > 0 {
		config.IPv6Enabled = true
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

	// Get IPv4 address info
	cmd := exec.CommandContext(ctx, "networksetup", "-getinfo", serviceName)
	output, err := cmd.Output()
	if err != nil {
		return config, nil
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "IP address:") {
			addr := strings.TrimSpace(strings.TrimPrefix(line, "IP address:"))
			if addr != "" && addr != "none" {
				config.Addresses = append(config.Addresses, addr)
			}
		} else if strings.HasPrefix(line, "Subnet mask:") {
			config.Netmask = strings.TrimPrefix(line, "Subnet mask:")
			config.Netmask = strings.TrimSpace(config.Netmask)
		} else if strings.HasPrefix(line, "Router:") {
			config.Gateway = strings.TrimPrefix(line, "Router:")
			config.Gateway = strings.TrimSpace(config.Gateway)
		} else if strings.Contains(line, "DHCP Configuration") {
			config.DHCP = true
		} else if strings.HasPrefix(line, "IPv6:") {
			ipv6Status := strings.TrimSpace(strings.TrimPrefix(line, "IPv6:"))
			if ipv6Status == "Automatic" || ipv6Status == "Manual" {
				config.IPv6Enabled = true
			}
		} else if strings.HasPrefix(line, "IPv6 IP address:") {
			addr6 := strings.TrimSpace(strings.TrimPrefix(line, "IPv6 IP address:"))
			if addr6 != "" && addr6 != "none" && !strings.HasPrefix(addr6, "fe80:") {
				config.Addresses6 = append(config.Addresses6, addr6)
			}
		} else if strings.HasPrefix(line, "IPv6 Router:") {
			gw6 := strings.TrimSpace(strings.TrimPrefix(line, "IPv6 Router:"))
			if gw6 != "" && gw6 != "none" {
				config.Gateway6 = gw6
			}
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

	// Get IPv4 configuration
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
				addr := strings.TrimSpace(parts[1])
				if addr != "" {
					config.Addresses = append(config.Addresses, addr)
				}
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

	// Get IPv6 configuration
	cmd = exec.CommandContext(ctx, "netsh", "interface", "ipv6", "show", "addresses", "interface="+ifaceName)
	output, err = cmd.Output()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			// Look for address lines (format varies but usually has the address)
			fields := strings.Fields(line)
			for _, f := range fields {
				// Check if it looks like an IPv6 address (contains ::)
				if strings.Contains(f, ":") && !strings.HasPrefix(f, "fe80:") {
					if ip := net.ParseIP(f); ip != nil && ip.To4() == nil {
						config.Addresses6 = append(config.Addresses6, f)
						config.IPv6Enabled = true
					}
				}
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

	// Compare IPv4 addresses (normalize CIDR)
	if len(desired.Addresses) > 0 {
		desiredAddrs := normalizeAddresses(desired.Addresses, desired.Netmask)
		currentAddrs := normalizeAddresses(current.Addresses, current.Netmask)
		if !stringSlicesEqual(desiredAddrs, currentAddrs) {
			matches = false
			result.Diff["addresses"] = map[string]interface{}{"current": currentAddrs, "desired": desiredAddrs}
		}
	}

	// Compare IPv6 addresses
	if len(desired.Addresses6) > 0 {
		desiredAddrs6 := normalizeAddresses(desired.Addresses6, "")
		currentAddrs6 := normalizeAddresses(current.Addresses6, "")
		if !stringSlicesEqual(desiredAddrs6, currentAddrs6) {
			matches = false
			result.Diff["addresses6"] = map[string]interface{}{"current": currentAddrs6, "desired": desiredAddrs6}
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

	// Compare MAC address
	if desired.MACAddress != "" {
		currentMAC := current.MACAddress
		if currentMAC == "" {
			// Get current MAC from interface
			if iface, err := net.InterfaceByName(desired.Interface); err == nil {
				currentMAC = iface.HardwareAddr.String()
			}
		}
		if !strings.EqualFold(desired.MACAddress, currentMAC) {
			matches = false
			result.Diff["mac_address"] = map[string]string{"current": currentMAC, "desired": desired.MACAddress}
		}
	}

	// Compare Wake-on-LAN (requires ethtool check, skip for now as it's complex to detect)
	// WoL comparison would require running ethtool which may not be available
	// We'll always apply WoL settings if specified

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

// normalizeAddresses normalizes a slice of addresses to CIDR notation
func normalizeAddresses(addrs []string, netmask string) []string {
	if len(addrs) == 0 {
		return nil
	}

	result := make([]string, len(addrs))
	for i, addr := range addrs {
		// Only apply netmask to first address if specified
		if i == 0 {
			result[i] = normalizeAddress(addr, netmask)
		} else {
			result[i] = normalizeAddress(addr, "")
		}
	}
	return result
}

// isValidMAC validates a MAC address format (xx:xx:xx:xx:xx:xx or xx-xx-xx-xx-xx-xx)
func isValidMAC(mac string) bool {
	if mac == "" {
		return false
	}
	_, err := net.ParseMAC(mac)
	return err == nil
}

// isValidWoLMode validates a Wake-on-LAN mode
func isValidWoLMode(mode string) bool {
	validModes := map[string]bool{
		"magic":     true, // g - Wake on magic packet
		"unicast":   true, // u - Wake on unicast
		"multicast": true, // m - Wake on multicast
		"broadcast": true, // b - Wake on broadcast
		"arp":       true, // a - Wake on ARP
		"off":       true, // d - Disable WoL
		"g":         true, // ethtool shorthand
		"u":         true,
		"m":         true,
		"b":         true,
		"a":         true,
		"d":         true,
	}
	return validModes[strings.ToLower(mode)]
}

// wolModeToEthtool converts WoL mode name to ethtool flag
func wolModeToEthtool(mode string) string {
	modeMap := map[string]string{
		"magic":     "g",
		"unicast":   "u",
		"multicast": "m",
		"broadcast": "b",
		"arp":       "a",
		"off":       "d",
	}
	if flag, ok := modeMap[strings.ToLower(mode)]; ok {
		return flag
	}
	// Already a shorthand
	return strings.ToLower(mode)
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
	case NMIfupdown:
		return m.applyDHCPConfigIfupdown(ctx, config, result)
	case NMSystemdNetworkd:
		return m.applyDHCPConfigSystemdNetworkd(ctx, config, result)
	case NMNetplan:
		return m.applyDHCPConfigNetplan(ctx, config, result)
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

	// Build nmcli arguments for IPv4
	args := m.buildNmcliIPv4Args(config, connectionExists, false)

	// Execute nmcli command
	cmd := exec.CommandContext(ctx, "nmcli", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nmcli IPv4 failed: %w (output: %s)", err, string(output))
	}

	// Apply IPv6 configuration if enabled
	if config.IPv6Enabled {
		if err := m.applyNmcliIPv6Config(ctx, config, connectionExists); err != nil {
			return fmt.Errorf("nmcli IPv6 failed: %w", err)
		}
	}

	// Apply search domains (shared between IPv4 and IPv6)
	if len(config.SearchDomains) > 0 {
		searchArgs := []string{"connection", "modify", config.Interface,
			"ipv4.dns-search", strings.Join(config.SearchDomains, ",")}
		if config.IPv6Enabled {
			searchArgs = append(searchArgs, "ipv6.dns-search", strings.Join(config.SearchDomains, ","))
		}
		searchCmd := exec.CommandContext(ctx, "nmcli", searchArgs...)
		if output, err := searchCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to set search domains: %w (output: %s)", err, string(output))
		}
	}

	// Apply MAC address if specified
	if config.MACAddress != "" && isValidMAC(config.MACAddress) {
		macArgs := []string{"connection", "modify", config.Interface,
			"802-3-ethernet.cloned-mac-address", config.MACAddress}
		macCmd := exec.CommandContext(ctx, "nmcli", macArgs...)
		if output, err := macCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to set MAC address: %w (output: %s)", err, string(output))
		}
	}

	// Apply Wake-on-LAN if specified
	if config.WakeOnLAN != "" && isValidWoLMode(config.WakeOnLAN) {
		wolArgs := []string{"connection", "modify", config.Interface,
			"802-3-ethernet.wake-on-lan", config.WakeOnLAN}
		wolCmd := exec.CommandContext(ctx, "nmcli", wolArgs...)
		if output, err := wolCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to set Wake-on-LAN: %w (output: %s)", err, string(output))
		}
	}

	// Bring up the connection
	upCmd := exec.CommandContext(ctx, "nmcli", "connection", "up", config.Interface)
	if output, err := upCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to activate connection: %w (output: %s)", err, string(output))
	}

	// Build comment
	var parts []string
	if len(config.Addresses) > 0 {
		if len(config.Addresses) == 1 {
			parts = append(parts, fmt.Sprintf("IPv4 %s", config.Addresses[0]))
		} else {
			parts = append(parts, fmt.Sprintf("%d IPv4 addresses", len(config.Addresses)))
		}
	}
	if len(config.Addresses6) > 0 {
		if len(config.Addresses6) == 1 {
			parts = append(parts, fmt.Sprintf("IPv6 %s", config.Addresses6[0]))
		} else {
			parts = append(parts, fmt.Sprintf("%d IPv6 addresses", len(config.Addresses6)))
		}
	}
	if len(parts) > 0 {
		result.Comment = fmt.Sprintf("Configured %s with %s", config.Interface, strings.Join(parts, " and "))
	} else {
		result.Comment = fmt.Sprintf("Configured %s", config.Interface)
	}
	return nil
}

// buildNmcliIPv4Args builds nmcli arguments for IPv4 configuration
func (m *NetworkModule) buildNmcliIPv4Args(config *NetworkConfig, connectionExists, dhcp bool) []string {
	var args []string

	if connectionExists {
		args = []string{"connection", "modify", config.Interface}
	} else {
		args = []string{"connection", "add", "type", "ethernet", "con-name", config.Interface, "ifname", config.Interface}
	}

	if dhcp {
		args = append(args, "ipv4.method", "auto")
	} else {
		// nmcli supports multiple addresses separated by commas
		if len(config.Addresses) > 0 {
			args = append(args, "ipv4.addresses", strings.Join(config.Addresses, ","))
		}
		if config.Gateway != "" {
			args = append(args, "ipv4.gateway", config.Gateway)
		}
		args = append(args, "ipv4.method", "manual")
	}

	// DNS servers (can include both IPv4 and IPv6 addresses)
	if len(config.DNS) > 0 {
		args = append(args, "ipv4.dns", strings.Join(config.DNS, ","))
	}

	return args
}

// applyNmcliIPv6Config applies IPv6 configuration using nmcli
func (m *NetworkModule) applyNmcliIPv6Config(ctx context.Context, config *NetworkConfig, connectionExists bool) error {
	args := []string{"connection", "modify", config.Interface}

	if config.DHCP6 {
		// DHCPv6
		args = append(args, "ipv6.method", "dhcp")
	} else if len(config.Addresses6) > 0 {
		// Static IPv6 - nmcli supports multiple addresses separated by commas
		args = append(args, "ipv6.addresses", strings.Join(config.Addresses6, ","))
		if config.Gateway6 != "" {
			args = append(args, "ipv6.gateway", config.Gateway6)
		}
		args = append(args, "ipv6.method", "manual")
	} else {
		// SLAAC (auto with router advertisements)
		args = append(args, "ipv6.method", "auto")
	}

	// IPv6 DNS (if specified separately or same as IPv4)
	if len(config.DNS) > 0 {
		// Filter for IPv6 DNS servers
		var ipv6DNS []string
		for _, dns := range config.DNS {
			if strings.Contains(dns, ":") {
				ipv6DNS = append(ipv6DNS, dns)
			}
		}
		if len(ipv6DNS) > 0 {
			args = append(args, "ipv6.dns", strings.Join(ipv6DNS, ","))
		}
	}

	// Privacy extensions (RFC 4941)
	if config.IPv6Privacy {
		args = append(args, "ipv6.ip6-privacy", "2") // 2 = prefer temporary addresses
	}

	cmd := exec.CommandContext(ctx, "nmcli", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to configure IPv6: %w (output: %s)", err, string(output))
	}

	return nil
}

// applyDHCPConfigNmcli applies DHCP config using nmcli
func (m *NetworkModule) applyDHCPConfigNmcli(ctx context.Context, config *NetworkConfig, result *StateResult) error {
	// Check if connection exists
	checkCmd := exec.CommandContext(ctx, "nmcli", "-t", "-f", "NAME", "connection", "show")
	output, _ := checkCmd.Output()
	connectionExists := strings.Contains(string(output), config.Interface)

	// Build nmcli arguments for DHCP
	args := m.buildNmcliIPv4Args(config, connectionExists, true)

	cmd := exec.CommandContext(ctx, "nmcli", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nmcli failed: %w (output: %s)", err, string(output))
	}

	// Apply IPv6 configuration if enabled (typically auto/SLAAC with DHCP)
	if config.IPv6Enabled || config.DHCP6 {
		config.IPv6Enabled = true
		if err := m.applyNmcliIPv6Config(ctx, config, true); err != nil {
			return fmt.Errorf("nmcli IPv6 failed: %w", err)
		}
	}

	// Apply search domains
	if len(config.SearchDomains) > 0 {
		searchArgs := []string{"connection", "modify", config.Interface,
			"ipv4.dns-search", strings.Join(config.SearchDomains, ",")}
		if config.IPv6Enabled {
			searchArgs = append(searchArgs, "ipv6.dns-search", strings.Join(config.SearchDomains, ","))
		}
		searchCmd := exec.CommandContext(ctx, "nmcli", searchArgs...)
		if output, err := searchCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to set search domains: %w (output: %s)", err, string(output))
		}
	}

	// Bring up the connection
	upCmd := exec.CommandContext(ctx, "nmcli", "connection", "up", config.Interface)
	if output, err := upCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to activate connection: %w (output: %s)", err, string(output))
	}

	comment := fmt.Sprintf("Configured %s with DHCP", config.Interface)
	if config.IPv6Enabled {
		comment += " and IPv6"
	}
	result.Comment = comment
	return nil
}

// applyStaticConfigDarwin applies static config on macOS
func (m *NetworkModule) applyStaticConfigDarwin(ctx context.Context, config *NetworkConfig, result *StateResult) error {
	serviceName, err := m.getNetworkServiceName(ctx, config.Interface)
	if err != nil {
		return fmt.Errorf("failed to get service name: %w", err)
	}

	// Set primary IPv4 address using networksetup
	if len(config.Addresses) > 0 {
		addr := config.Addresses[0]
		netmask := config.Netmask
		if strings.Contains(addr, "/") {
			parts := strings.SplitN(addr, "/", 2)
			addr = parts[0]
			netmask = cidrToNetmask(parts[1])
		}

		cmd := exec.CommandContext(ctx, "networksetup", "-setmanual", serviceName, addr, netmask, config.Gateway)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to set primary IP: %w (output: %s)", err, string(output))
		}

		// Add additional IPv4 addresses using ifconfig aliases
		for i := 1; i < len(config.Addresses); i++ {
			aliasAddr := config.Addresses[i]
			aliasNetmask := "255.255.255.0" // default
			if strings.Contains(aliasAddr, "/") {
				parts := strings.SplitN(aliasAddr, "/", 2)
				aliasAddr = parts[0]
				aliasNetmask = cidrToNetmask(parts[1])
			}
			// Use ifconfig to add alias (interface:alias_num)
			aliasCmd := exec.CommandContext(ctx, "ifconfig", config.Interface, "alias", aliasAddr, "netmask", aliasNetmask)
			if output, err := aliasCmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to add alias IP %s: %w (output: %s)", aliasAddr, err, string(output))
			}
		}
	}

	// Set DNS if specified
	if len(config.DNS) > 0 {
		args := append([]string{"-setdnsservers", serviceName}, config.DNS...)
		cmd := exec.CommandContext(ctx, "networksetup", args...)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to set DNS: %w (output: %s)", err, string(output))
		}
	}

	// Set search domains if specified
	if len(config.SearchDomains) > 0 {
		args := append([]string{"-setsearchdomains", serviceName}, config.SearchDomains...)
		cmd := exec.CommandContext(ctx, "networksetup", args...)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to set search domains: %w (output: %s)", err, string(output))
		}
	}

	// Configure IPv6 if enabled
	if config.IPv6Enabled {
		if err := m.applyDarwinIPv6Config(ctx, serviceName, config); err != nil {
			return err
		}
	}

	// Apply MAC address if specified (using ifconfig)
	if config.MACAddress != "" && isValidMAC(config.MACAddress) {
		macCmd := exec.CommandContext(ctx, "ifconfig", config.Interface, "ether", config.MACAddress)
		if output, err := macCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to set MAC address: %w (output: %s)", err, string(output))
		}
	}

	// Apply Wake-on-LAN if specified (system-wide via pmset)
	if config.WakeOnLAN != "" && config.WakeOnLAN != "off" && config.WakeOnLAN != "d" {
		// macOS uses pmset for WoL - this is system-wide, not per-interface
		pmsetCmd := exec.CommandContext(ctx, "pmset", "-a", "womp", "1")
		if output, err := pmsetCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to enable Wake-on-LAN: %w (output: %s)", err, string(output))
		}
	} else if config.WakeOnLAN == "off" || config.WakeOnLAN == "d" {
		pmsetCmd := exec.CommandContext(ctx, "pmset", "-a", "womp", "0")
		if output, err := pmsetCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to disable Wake-on-LAN: %w (output: %s)", err, string(output))
		}
	}

	// Build comment
	var parts []string
	if len(config.Addresses) > 0 {
		if len(config.Addresses) == 1 {
			parts = append(parts, fmt.Sprintf("IPv4 %s", config.Addresses[0]))
		} else {
			parts = append(parts, fmt.Sprintf("%d IPv4 addresses", len(config.Addresses)))
		}
	}
	if len(config.Addresses6) > 0 {
		if len(config.Addresses6) == 1 {
			parts = append(parts, fmt.Sprintf("IPv6 %s", config.Addresses6[0]))
		} else {
			parts = append(parts, fmt.Sprintf("%d IPv6 addresses", len(config.Addresses6)))
		}
	}
	if len(parts) > 0 {
		result.Comment = fmt.Sprintf("Configured %s (%s) with %s", serviceName, config.Interface, strings.Join(parts, " and "))
	} else {
		result.Comment = fmt.Sprintf("Configured %s (%s)", serviceName, config.Interface)
	}
	return nil
}

// applyDarwinIPv6Config applies IPv6 configuration on macOS
func (m *NetworkModule) applyDarwinIPv6Config(ctx context.Context, serviceName string, config *NetworkConfig) error {
	if config.DHCP6 {
		// DHCPv6
		cmd := exec.CommandContext(ctx, "networksetup", "-setv6automatic", serviceName)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to set IPv6 automatic: %w (output: %s)", err, string(output))
		}
	} else if len(config.Addresses6) > 0 {
		// Set primary IPv6 address
		addr6 := config.Addresses6[0]
		prefix := "64"
		if strings.Contains(addr6, "/") {
			parts := strings.SplitN(addr6, "/", 2)
			addr6 = parts[0]
			prefix = parts[1]
		}
		gateway6 := config.Gateway6
		if gateway6 == "" {
			gateway6 = "::"
		}
		cmd := exec.CommandContext(ctx, "networksetup", "-setv6manual", serviceName, addr6, prefix, gateway6)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to set primary IPv6: %w (output: %s)", err, string(output))
		}

		// Add additional IPv6 addresses using ifconfig
		for i := 1; i < len(config.Addresses6); i++ {
			aliasAddr6 := config.Addresses6[i]
			aliasPrefix := "64"
			if strings.Contains(aliasAddr6, "/") {
				parts := strings.SplitN(aliasAddr6, "/", 2)
				aliasAddr6 = parts[0]
				aliasPrefix = parts[1]
			}
			aliasCmd := exec.CommandContext(ctx, "ifconfig", config.Interface, "inet6", aliasAddr6, "prefixlen", aliasPrefix, "alias")
			if output, err := aliasCmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to add IPv6 alias %s: %w (output: %s)", aliasAddr6, err, string(output))
			}
		}
	} else {
		// Automatic (SLAAC)
		cmd := exec.CommandContext(ctx, "networksetup", "-setv6automatic", serviceName)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to set IPv6 automatic: %w (output: %s)", err, string(output))
		}
	}

	return nil
}

// applyDHCPConfigDarwin applies DHCP config on macOS
func (m *NetworkModule) applyDHCPConfigDarwin(ctx context.Context, config *NetworkConfig, result *StateResult) error {
	serviceName, err := m.getNetworkServiceName(ctx, config.Interface)
	if err != nil {
		return fmt.Errorf("failed to get service name: %w", err)
	}

	// Set DHCP for IPv4
	cmd := exec.CommandContext(ctx, "networksetup", "-setdhcp", serviceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set DHCP: %w (output: %s)", err, string(output))
	}

	// Set search domains if specified
	if len(config.SearchDomains) > 0 {
		args := append([]string{"-setsearchdomains", serviceName}, config.SearchDomains...)
		cmd = exec.CommandContext(ctx, "networksetup", args...)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to set search domains: %w (output: %s)", err, string(output))
		}
	}

	// Configure IPv6 if enabled (typically automatic with DHCP)
	if config.IPv6Enabled || config.DHCP6 {
		config.IPv6Enabled = true
		if err := m.applyDarwinIPv6Config(ctx, serviceName, config); err != nil {
			return err
		}
	}

	comment := fmt.Sprintf("Configured %s (%s) with DHCP", serviceName, config.Interface)
	if config.IPv6Enabled {
		comment += " and IPv6"
	}
	result.Comment = comment
	return nil
}

// applyStaticConfigWindows applies static config on Windows
func (m *NetworkModule) applyStaticConfigWindows(ctx context.Context, config *NetworkConfig, result *StateResult) error {
	// Set primary IPv4 address
	if len(config.Addresses) > 0 {
		addr := config.Addresses[0]
		netmask := config.Netmask
		if strings.Contains(addr, "/") {
			parts := strings.SplitN(addr, "/", 2)
			addr = parts[0]
			netmask = cidrToNetmask(parts[1])
		}

		// Set primary static IP
		cmd := exec.CommandContext(ctx, "netsh", "interface", "ip", "set", "address",
			"name="+config.Interface, "static", addr, netmask, config.Gateway)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to set primary IP: %w (output: %s)", err, string(output))
		}

		// Add additional IPv4 addresses
		for i := 1; i < len(config.Addresses); i++ {
			aliasAddr := config.Addresses[i]
			aliasNetmask := "255.255.255.0" // default
			if strings.Contains(aliasAddr, "/") {
				parts := strings.SplitN(aliasAddr, "/", 2)
				aliasAddr = parts[0]
				aliasNetmask = cidrToNetmask(parts[1])
			}
			addCmd := exec.CommandContext(ctx, "netsh", "interface", "ip", "add", "address",
				"name="+config.Interface, aliasAddr, aliasNetmask)
			if output, err := addCmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to add IP %s: %w (output: %s)", aliasAddr, err, string(output))
			}
		}
	}

	// Set DNS if specified
	if len(config.DNS) > 0 {
		// Set primary DNS
		cmd := exec.CommandContext(ctx, "netsh", "interface", "ip", "set", "dns",
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

	// Configure IPv6 if enabled
	if config.IPv6Enabled {
		if err := m.applyWindowsIPv6Config(ctx, config); err != nil {
			return err
		}
	}

	// MAC address override is not reliably supported on Windows via netsh
	// It requires driver-specific support or registry modifications
	if config.MACAddress != "" {
		// Log a warning but don't fail - MAC override is driver-dependent on Windows
		result.Changes["mac_address_warning"] = "MAC address override not supported via netsh on Windows"
	}

	// Apply Wake-on-LAN if specified (via PowerShell)
	if config.WakeOnLAN != "" && isValidWoLMode(config.WakeOnLAN) {
		var wolCmd *exec.Cmd
		if config.WakeOnLAN == "off" || config.WakeOnLAN == "d" {
			// Disable WoL
			wolCmd = exec.CommandContext(ctx, "powershell", "-Command",
				fmt.Sprintf("Disable-NetAdapterPowerManagement -Name '%s' -WakeOnMagicPacket -Confirm:$false", config.Interface))
		} else {
			// Enable WoL
			wolCmd = exec.CommandContext(ctx, "powershell", "-Command",
				fmt.Sprintf("Enable-NetAdapterPowerManagement -Name '%s' -WakeOnMagicPacket -Confirm:$false", config.Interface))
		}
		if output, err := wolCmd.CombinedOutput(); err != nil {
			// WoL may not be supported on all adapters, log warning but don't fail
			result.Changes["wol_warning"] = fmt.Sprintf("Wake-on-LAN configuration may have failed: %s", string(output))
		}
	}

	// Build comment
	var parts []string
	if len(config.Addresses) > 0 {
		if len(config.Addresses) == 1 {
			parts = append(parts, fmt.Sprintf("IPv4 %s", config.Addresses[0]))
		} else {
			parts = append(parts, fmt.Sprintf("%d IPv4 addresses", len(config.Addresses)))
		}
	}
	if len(config.Addresses6) > 0 {
		if len(config.Addresses6) == 1 {
			parts = append(parts, fmt.Sprintf("IPv6 %s", config.Addresses6[0]))
		} else {
			parts = append(parts, fmt.Sprintf("%d IPv6 addresses", len(config.Addresses6)))
		}
	}
	if len(parts) > 0 {
		result.Comment = fmt.Sprintf("Configured %s with %s", config.Interface, strings.Join(parts, " and "))
	} else {
		result.Comment = fmt.Sprintf("Configured %s", config.Interface)
	}
	return nil
}

// applyWindowsIPv6Config configures IPv6 on Windows
func (m *NetworkModule) applyWindowsIPv6Config(ctx context.Context, config *NetworkConfig) error {
	// Add all IPv6 addresses
	for i, addr6 := range config.Addresses6 {
		addr := addr6
		prefixLen := "64"
		if strings.Contains(addr, "/") {
			parts := strings.SplitN(addr, "/", 2)
			addr = parts[0]
			prefixLen = parts[1]
		}

		var cmd *exec.Cmd
		if i == 0 {
			// Set primary address
			cmd = exec.CommandContext(ctx, "netsh", "interface", "ipv6", "set", "address",
				"interface="+config.Interface, "address="+addr+"/"+prefixLen, "store=persistent")
		} else {
			// Add additional addresses
			cmd = exec.CommandContext(ctx, "netsh", "interface", "ipv6", "add", "address",
				"interface="+config.Interface, "address="+addr+"/"+prefixLen, "store=persistent")
		}
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to set IPv6 address %s: %w (output: %s)", addr, err, string(output))
		}
	}

	if config.Gateway6 != "" {
		// Add IPv6 default route
		cmd := exec.CommandContext(ctx, "netsh", "interface", "ipv6", "add", "route",
			"::/0", "interface="+config.Interface, "nexthop="+config.Gateway6, "store=persistent")
		if output, err := cmd.CombinedOutput(); err != nil {
			// Try to delete existing route first and retry
			delCmd := exec.CommandContext(ctx, "netsh", "interface", "ipv6", "delete", "route",
				"::/0", "interface="+config.Interface)
			delCmd.Run() // Ignore errors

			if output, err = cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to set IPv6 gateway: %w (output: %s)", err, string(output))
			}
		}
	}

	// Set IPv6 DNS servers (filter DNS list for IPv6 addresses)
	var ipv6DNS []string
	for _, dns := range config.DNS {
		if strings.Contains(dns, ":") {
			ipv6DNS = append(ipv6DNS, dns)
		}
	}

	if len(ipv6DNS) > 0 {
		// Set primary IPv6 DNS
		cmd := exec.CommandContext(ctx, "netsh", "interface", "ipv6", "set", "dns",
			"name="+config.Interface, "static", ipv6DNS[0])
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to set primary IPv6 DNS: %w (output: %s)", err, string(output))
		}

		// Add additional IPv6 DNS servers
		for i := 1; i < len(ipv6DNS); i++ {
			cmd = exec.CommandContext(ctx, "netsh", "interface", "ipv6", "add", "dns",
				"name="+config.Interface, ipv6DNS[i], "index="+fmt.Sprintf("%d", i+1))
			if output, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to add IPv6 DNS %s: %w (output: %s)", ipv6DNS[i], err, string(output))
			}
		}
	}

	// Configure privacy extensions (random interface IDs)
	if config.IPv6Privacy {
		cmd := exec.CommandContext(ctx, "netsh", "interface", "ipv6", "set", "privacy",
			"state=enabled")
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to enable IPv6 privacy: %w (output: %s)", err, string(output))
		}
	}

	return nil
}

// applyDHCPConfigWindows applies DHCP config on Windows
func (m *NetworkModule) applyDHCPConfigWindows(ctx context.Context, config *NetworkConfig, result *StateResult) error {
	// Set DHCP for IPv4
	cmd := exec.CommandContext(ctx, "netsh", "interface", "ip", "set", "address",
		"name="+config.Interface, "dhcp")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set DHCP: %w (output: %s)", err, string(output))
	}

	// Set DHCP for IPv4 DNS
	cmd = exec.CommandContext(ctx, "netsh", "interface", "ip", "set", "dns",
		"name="+config.Interface, "dhcp")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set DNS DHCP: %w (output: %s)", err, string(output))
	}

	// Configure IPv6 if enabled
	if config.IPv6Enabled || config.DHCP6 {
		config.IPv6Enabled = true

		if config.DHCP6 {
			// Enable DHCPv6 on the interface
			// Windows uses router advertisement mode which can be stateful (DHCPv6) or stateless (SLAAC)
			cmd = exec.CommandContext(ctx, "netsh", "interface", "ipv6", "set", "interface",
				config.Interface, "routerdiscovery=enabled", "managed=enabled", "otherstateful=enabled")
			if output, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to enable DHCPv6: %w (output: %s)", err, string(output))
			}

			// Set IPv6 DNS to DHCP (auto-configured)
			cmd = exec.CommandContext(ctx, "netsh", "interface", "ipv6", "set", "dns",
				"name="+config.Interface, "dhcp")
			if output, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to set IPv6 DNS DHCP: %w (output: %s)", err, string(output))
			}
		} else {
			// SLAAC mode - Router discovery enabled but not managed addressing
			cmd = exec.CommandContext(ctx, "netsh", "interface", "ipv6", "set", "interface",
				config.Interface, "routerdiscovery=enabled", "managed=disabled")
			if output, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to configure SLAAC: %w (output: %s)", err, string(output))
			}
		}

		// Configure privacy extensions if requested
		if config.IPv6Privacy {
			cmd = exec.CommandContext(ctx, "netsh", "interface", "ipv6", "set", "privacy",
				"state=enabled")
			if output, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to enable IPv6 privacy: %w (output: %s)", err, string(output))
			}
		}

		result.Comment = fmt.Sprintf("Configured %s with DHCP and IPv6", config.Interface)
	} else {
		result.Comment = fmt.Sprintf("Configured %s with DHCP", config.Interface)
	}

	return nil
}

// applyStaticConfigIfupdown applies static config using ifupdown
func (m *NetworkModule) applyStaticConfigIfupdown(ctx context.Context, config *NetworkConfig, result *StateResult) error {
	interfacesFile := "/etc/network/interfaces"

	// Parse primary address and netmask
	var primaryAddr, primaryNetmask string
	if len(config.Addresses) > 0 {
		primaryAddr = config.Addresses[0]
		primaryNetmask = config.Netmask
		if strings.Contains(primaryAddr, "/") {
			parts := strings.SplitN(primaryAddr, "/", 2)
			primaryAddr = parts[0]
			primaryNetmask = cidrToNetmask(parts[1])
		}
	}

	// Bring interface down first (ignore errors if not up)
	downCmd := exec.CommandContext(ctx, "ifdown", "--force", config.Interface)
	downCmd.Run()

	// Read existing interfaces file
	content, err := os.ReadFile(interfacesFile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read %s: %w", interfacesFile, err)
	}

	// Parse and update the interfaces file
	newContent := m.updateIfupdownConfig(string(content), config, primaryAddr, primaryNetmask, false)

	// Backup existing file
	if len(content) > 0 {
		backupPath := interfacesFile + ".kscore.bak"
		if err := os.WriteFile(backupPath, content, 0644); err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
	}

	// Write new configuration
	if err := os.WriteFile(interfacesFile, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", interfacesFile, err)
	}

	// Bring interface up
	upCmd := exec.CommandContext(ctx, "ifup", config.Interface)
	if output, err := upCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to bring up interface: %w (output: %s)", err, string(output))
	}

	// Build comment
	var parts []string
	if len(config.Addresses) > 0 {
		if len(config.Addresses) == 1 {
			parts = append(parts, fmt.Sprintf("IPv4 %s", config.Addresses[0]))
		} else {
			parts = append(parts, fmt.Sprintf("%d IPv4 addresses", len(config.Addresses)))
		}
	}
	if len(config.Addresses6) > 0 {
		if len(config.Addresses6) == 1 {
			parts = append(parts, fmt.Sprintf("IPv6 %s", config.Addresses6[0]))
		} else {
			parts = append(parts, fmt.Sprintf("%d IPv6 addresses", len(config.Addresses6)))
		}
	}
	if len(parts) > 0 {
		result.Comment = fmt.Sprintf("Configured %s with %s via ifupdown", config.Interface, strings.Join(parts, " and "))
	} else {
		result.Comment = fmt.Sprintf("Configured %s via ifupdown", config.Interface)
	}
	return nil
}

// updateIfupdownConfig updates or adds an interface stanza in the interfaces file
func (m *NetworkModule) updateIfupdownConfig(content string, config *NetworkConfig, addr, netmask string, dhcp bool) string {
	var result bytes.Buffer
	scanner := bufio.NewScanner(strings.NewReader(content))

	inTargetStanza := false
	foundInterface := false
	skipUntilNextStanza := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Check if we're starting a new stanza
		if strings.HasPrefix(trimmed, "auto ") || strings.HasPrefix(trimmed, "iface ") ||
			strings.HasPrefix(trimmed, "allow-") || strings.HasPrefix(trimmed, "source ") ||
			strings.HasPrefix(trimmed, "mapping ") {
			skipUntilNextStanza = false
		}

		// Check for our interface's iface line (both inet and inet6)
		if strings.HasPrefix(trimmed, "iface "+config.Interface+" ") {
			inTargetStanza = true
			skipUntilNextStanza = true

			// Only write the new stanza once (on the first iface line found)
			if !foundInterface {
				foundInterface = true
				m.writeIfupdownStanza(&result, config, addr, netmask, dhcp)
			}
			// Skip this line (we'll write our own stanza)
			continue
		}

		// Check for auto line for our interface
		if trimmed == "auto "+config.Interface {
			// Keep it but don't duplicate
			if !foundInterface {
				result.WriteString(line + "\n")
			}
			continue
		}

		// Skip lines that belong to the old stanza
		if skipUntilNextStanza && inTargetStanza {
			// Check if this is continuation of the stanza (indented or option line)
			if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") ||
				strings.HasPrefix(trimmed, "address") || strings.HasPrefix(trimmed, "netmask") ||
				strings.HasPrefix(trimmed, "gateway") || strings.HasPrefix(trimmed, "dns-") ||
				strings.HasPrefix(trimmed, "metric") || strings.HasPrefix(trimmed, "mtu") {
				continue
			}
			inTargetStanza = false
			skipUntilNextStanza = false
		}

		result.WriteString(line + "\n")
	}

	// If interface wasn't found, add it at the end
	if !foundInterface {
		result.WriteString("\nauto " + config.Interface + "\n")
		m.writeIfupdownStanza(&result, config, addr, netmask, dhcp)
	}

	return result.String()
}

// writeIfupdownStanza writes an interface stanza (IPv4 and optionally IPv6)
func (m *NetworkModule) writeIfupdownStanza(w *bytes.Buffer, config *NetworkConfig, addr, netmask string, dhcp bool) {
	// Write IPv4 stanza
	if dhcp {
		w.WriteString(fmt.Sprintf("iface %s inet dhcp\n", config.Interface))
	} else {
		w.WriteString(fmt.Sprintf("iface %s inet static\n", config.Interface))
		if addr != "" {
			w.WriteString(fmt.Sprintf("    address %s\n", addr))
		}
		if netmask != "" {
			w.WriteString(fmt.Sprintf("    netmask %s\n", netmask))
		}
		if config.Gateway != "" {
			w.WriteString(fmt.Sprintf("    gateway %s\n", config.Gateway))
		}
		// Filter IPv4 DNS servers only
		var ipv4DNS []string
		for _, dns := range config.DNS {
			if !strings.Contains(dns, ":") {
				ipv4DNS = append(ipv4DNS, dns)
			}
		}
		if len(ipv4DNS) > 0 {
			w.WriteString(fmt.Sprintf("    dns-nameservers %s\n", strings.Join(ipv4DNS, " ")))
		}
		if len(config.SearchDomains) > 0 {
			w.WriteString(fmt.Sprintf("    dns-search %s\n", strings.Join(config.SearchDomains, " ")))
		}
		if config.MTU > 0 {
			w.WriteString(fmt.Sprintf("    mtu %d\n", config.MTU))
		}
		if config.Metric > 0 {
			w.WriteString(fmt.Sprintf("    metric %d\n", config.Metric))
		}
		// Add MAC address override
		if config.MACAddress != "" && isValidMAC(config.MACAddress) {
			w.WriteString(fmt.Sprintf("    hwaddress ether %s\n", config.MACAddress))
		}
		// Add Wake-on-LAN configuration
		if config.WakeOnLAN != "" && isValidWoLMode(config.WakeOnLAN) {
			wolFlag := wolModeToEthtool(config.WakeOnLAN)
			w.WriteString(fmt.Sprintf("    post-up ethtool -s %s wol %s\n", config.Interface, wolFlag))
		}
		// Add additional IPv4 addresses using post-up commands
		for i := 1; i < len(config.Addresses); i++ {
			aliasAddr := config.Addresses[i]
			if !strings.Contains(aliasAddr, "/") {
				aliasAddr = aliasAddr + "/24" // default prefix
			}
			w.WriteString(fmt.Sprintf("    post-up ip addr add %s dev %s\n", aliasAddr, config.Interface))
			w.WriteString(fmt.Sprintf("    pre-down ip addr del %s dev %s || true\n", aliasAddr, config.Interface))
		}
	}

	// Write IPv6 stanza if enabled
	if config.IPv6Enabled {
		w.WriteString("\n")
		if config.DHCP6 {
			w.WriteString(fmt.Sprintf("iface %s inet6 dhcp\n", config.Interface))
			if config.AcceptRA != nil && *config.AcceptRA {
				w.WriteString("    accept_ra 1\n")
			}
		} else if len(config.Addresses6) > 0 {
			// Static IPv6 configuration
			w.WriteString(fmt.Sprintf("iface %s inet6 static\n", config.Interface))

			// Parse primary address and prefix
			addr6 := config.Addresses6[0]
			prefix := "64"
			if strings.Contains(addr6, "/") {
				parts := strings.SplitN(addr6, "/", 2)
				addr6 = parts[0]
				prefix = parts[1]
			}
			w.WriteString(fmt.Sprintf("    address %s\n", addr6))
			w.WriteString(fmt.Sprintf("    netmask %s\n", prefix))

			if config.Gateway6 != "" {
				w.WriteString(fmt.Sprintf("    gateway %s\n", config.Gateway6))
			}

			// Filter IPv6 DNS servers
			var ipv6DNS []string
			for _, dns := range config.DNS {
				if strings.Contains(dns, ":") {
					ipv6DNS = append(ipv6DNS, dns)
				}
			}
			if len(ipv6DNS) > 0 {
				w.WriteString(fmt.Sprintf("    dns-nameservers %s\n", strings.Join(ipv6DNS, " ")))
			}

			// Accept router advertisements setting
			if config.AcceptRA != nil {
				if *config.AcceptRA {
					w.WriteString("    accept_ra 1\n")
				} else {
					w.WriteString("    accept_ra 0\n")
				}
			}

			// Privacy extensions
			if config.IPv6Privacy {
				w.WriteString("    privext 2\n")
			}

			// Add additional IPv6 addresses using post-up commands
			for i := 1; i < len(config.Addresses6); i++ {
				aliasAddr6 := config.Addresses6[i]
				if !strings.Contains(aliasAddr6, "/") {
					aliasAddr6 = aliasAddr6 + "/64"
				}
				w.WriteString(fmt.Sprintf("    post-up ip -6 addr add %s dev %s\n", aliasAddr6, config.Interface))
				w.WriteString(fmt.Sprintf("    pre-down ip -6 addr del %s dev %s || true\n", aliasAddr6, config.Interface))
			}
		} else {
			// SLAAC mode - auto configuration
			w.WriteString(fmt.Sprintf("iface %s inet6 auto\n", config.Interface))
			if config.IPv6Privacy {
				w.WriteString("    privext 2\n")
			}
		}
	}
}

// applyDHCPConfigIfupdown applies DHCP config using ifupdown
func (m *NetworkModule) applyDHCPConfigIfupdown(ctx context.Context, config *NetworkConfig, result *StateResult) error {
	interfacesFile := "/etc/network/interfaces"

	// Bring interface down first (ignore errors if not up)
	downCmd := exec.CommandContext(ctx, "ifdown", "--force", config.Interface)
	downCmd.Run()

	// Read existing interfaces file
	content, err := os.ReadFile(interfacesFile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read %s: %w", interfacesFile, err)
	}

	// Parse and update the interfaces file for DHCP
	newContent := m.updateIfupdownConfig(string(content), config, "", "", true)

	// Backup existing file
	if len(content) > 0 {
		backupPath := interfacesFile + ".kscore.bak"
		if err := os.WriteFile(backupPath, content, 0644); err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
	}

	// Write new configuration
	if err := os.WriteFile(interfacesFile, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", interfacesFile, err)
	}

	// Bring interface up
	upCmd := exec.CommandContext(ctx, "ifup", config.Interface)
	if output, err := upCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to bring up interface: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Configured %s with DHCP via ifupdown", config.Interface)
	return nil
}

// applyDHCPConfigSystemdNetworkd applies DHCP config using systemd-networkd
func (m *NetworkModule) applyDHCPConfigSystemdNetworkd(ctx context.Context, config *NetworkConfig, result *StateResult) error {
	return m.applySystemdNetworkdConfig(ctx, config, result, true)
}

// applyDHCPConfigNetplan applies DHCP config using netplan
func (m *NetworkModule) applyDHCPConfigNetplan(ctx context.Context, config *NetworkConfig, result *StateResult) error {
	return m.applyNetplanConfig(ctx, config, result, true)
}

// applyStaticConfigSystemdNetworkd applies static config using systemd-networkd
func (m *NetworkModule) applyStaticConfigSystemdNetworkd(ctx context.Context, config *NetworkConfig, result *StateResult) error {
	return m.applySystemdNetworkdConfig(ctx, config, result, false)
}

// applySystemdNetworkdConfig applies config using systemd-networkd
func (m *NetworkModule) applySystemdNetworkdConfig(ctx context.Context, config *NetworkConfig, result *StateResult, dhcp bool) error {
	networkDir := "/etc/systemd/network"
	networkFile := filepath.Join(networkDir, fmt.Sprintf("10-kscore-%s.network", config.Interface))

	// Ensure directory exists
	if err := os.MkdirAll(networkDir, 0755); err != nil {
		return fmt.Errorf("failed to create network directory: %w", err)
	}

	// Build .network file content
	var content bytes.Buffer
	content.WriteString("# Managed by Keystone Core - do not edit manually\n")
	content.WriteString("[Match]\n")
	content.WriteString(fmt.Sprintf("Name=%s\n", config.Interface))
	content.WriteString("\n[Network]\n")

	// Determine DHCP setting based on IPv4 and IPv6 configuration
	if dhcp && config.DHCP6 {
		content.WriteString("DHCP=yes\n") // Both IPv4 and IPv6 DHCP
	} else if dhcp {
		content.WriteString("DHCP=ipv4\n")
	} else if config.DHCP6 {
		content.WriteString("DHCP=ipv6\n")
	} else {
		content.WriteString("DHCP=no\n")
	}

	// IPv4 static addresses (multiple Address= lines supported)
	if !dhcp && len(config.Addresses) > 0 {
		for i, address := range config.Addresses {
			addr := address
			// Apply netmask only to first address if not in CIDR
			if !strings.Contains(addr, "/") {
				if i == 0 && config.Netmask != "" {
					mask := net.ParseIP(config.Netmask)
					if mask != nil {
						ones, _ := net.IPMask(mask.To4()).Size()
						addr = fmt.Sprintf("%s/%d", addr, ones)
					}
				} else {
					addr = addr + "/24" // Default for additional addresses
				}
			}
			content.WriteString(fmt.Sprintf("Address=%s\n", addr))
		}
	}

	// IPv6 static addresses (multiple Address= lines supported)
	if config.IPv6Enabled && !config.DHCP6 && len(config.Addresses6) > 0 {
		for _, addr6 := range config.Addresses6 {
			if !strings.Contains(addr6, "/") {
				addr6 = addr6 + "/64"
			}
			content.WriteString(fmt.Sprintf("Address=%s\n", addr6))
		}
	}

	// Gateways
	if !dhcp && config.Gateway != "" {
		content.WriteString(fmt.Sprintf("Gateway=%s\n", config.Gateway))
	}
	if config.IPv6Enabled && !config.DHCP6 && config.Gateway6 != "" {
		content.WriteString(fmt.Sprintf("Gateway=%s\n", config.Gateway6))
	}

	// DNS servers (all in one place)
	for _, dns := range config.DNS {
		content.WriteString(fmt.Sprintf("DNS=%s\n", dns))
	}
	if len(config.SearchDomains) > 0 {
		content.WriteString(fmt.Sprintf("Domains=%s\n", strings.Join(config.SearchDomains, " ")))
	}

	// IPv6 specific settings
	if config.IPv6Enabled {
		// IPv6 Accept Router Advertisements
		if config.AcceptRA != nil {
			if *config.AcceptRA {
				content.WriteString("IPv6AcceptRA=yes\n")
			} else {
				content.WriteString("IPv6AcceptRA=no\n")
			}
		}

		// IPv6 privacy extensions (RFC 4941)
		if config.IPv6Privacy {
			content.WriteString("IPv6PrivacyExtensions=prefer-public\n")
		}
	}

	// Add Link section for MTU, MAC, or WoL if specified
	needLinkSection := config.MTU > 0 || (config.MACAddress != "" && isValidMAC(config.MACAddress)) || (config.WakeOnLAN != "" && isValidWoLMode(config.WakeOnLAN))
	if needLinkSection {
		content.WriteString("\n[Link]\n")
		if config.MTU > 0 {
			content.WriteString(fmt.Sprintf("MTUBytes=%d\n", config.MTU))
		}
		if config.MACAddress != "" && isValidMAC(config.MACAddress) {
			content.WriteString(fmt.Sprintf("MACAddress=%s\n", config.MACAddress))
		}
		if config.WakeOnLAN != "" && isValidWoLMode(config.WakeOnLAN) {
			content.WriteString(fmt.Sprintf("WakeOnLan=%s\n", config.WakeOnLAN))
		}
	}

	// Add Route section for metric if specified (IPv4)
	if config.Metric > 0 && config.Gateway != "" && !dhcp {
		content.WriteString("\n[Route]\n")
		content.WriteString(fmt.Sprintf("Gateway=%s\n", config.Gateway))
		content.WriteString(fmt.Sprintf("Metric=%d\n", config.Metric))
	}

	// Add IPv6 Route section for metric if specified
	if config.Metric > 0 && config.Gateway6 != "" && config.IPv6Enabled && !config.DHCP6 {
		content.WriteString("\n[Route]\n")
		content.WriteString(fmt.Sprintf("Gateway=%s\n", config.Gateway6))
		content.WriteString(fmt.Sprintf("Metric=%d\n", config.Metric))
	}

	// Backup existing file if it exists
	if existingContent, err := os.ReadFile(networkFile); err == nil {
		backupPath := networkFile + ".kscore.bak"
		if err := os.WriteFile(backupPath, existingContent, 0644); err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
	}

	// Write new configuration
	if err := os.WriteFile(networkFile, content.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write network file: %w", err)
	}

	// Reload systemd-networkd
	reloadCmd := exec.CommandContext(ctx, "networkctl", "reload")
	if output, err := reloadCmd.CombinedOutput(); err != nil {
		// Try systemctl restart as fallback
		restartCmd := exec.CommandContext(ctx, "systemctl", "restart", "systemd-networkd")
		if output2, err2 := restartCmd.CombinedOutput(); err2 != nil {
			return fmt.Errorf("failed to reload networkd: %w (output: %s, %s)", err, string(output), string(output2))
		}
	}

	// Reconfigure the specific interface
	reconfigCmd := exec.CommandContext(ctx, "networkctl", "reconfigure", config.Interface)
	if output, err := reconfigCmd.CombinedOutput(); err != nil {
		// Non-fatal - the reload should have picked it up
		result.Comment = fmt.Sprintf("Configured %s via systemd-networkd (reconfigure warning: %s)", config.Interface, string(output))
	} else {
		// Build descriptive comment
		var parts []string
		if dhcp {
			parts = append(parts, "DHCP")
		} else if len(config.Addresses) > 0 {
			if len(config.Addresses) == 1 {
				parts = append(parts, fmt.Sprintf("IPv4 %s", config.Addresses[0]))
			} else {
				parts = append(parts, fmt.Sprintf("%d IPv4 addresses", len(config.Addresses)))
			}
		}
		if config.IPv6Enabled {
			if config.DHCP6 {
				parts = append(parts, "DHCPv6")
			} else if len(config.Addresses6) > 0 {
				if len(config.Addresses6) == 1 {
					parts = append(parts, fmt.Sprintf("IPv6 %s", config.Addresses6[0]))
				} else {
					parts = append(parts, fmt.Sprintf("%d IPv6 addresses", len(config.Addresses6)))
				}
			} else {
				parts = append(parts, "IPv6 SLAAC")
			}
		}
		if len(parts) > 0 {
			result.Comment = fmt.Sprintf("Configured %s with %s via systemd-networkd", config.Interface, strings.Join(parts, " and "))
		} else {
			result.Comment = fmt.Sprintf("Configured %s via systemd-networkd", config.Interface)
		}
	}

	return nil
}

// applyStaticConfigNetplan applies static config using netplan
func (m *NetworkModule) applyStaticConfigNetplan(ctx context.Context, config *NetworkConfig, result *StateResult) error {
	return m.applyNetplanConfig(ctx, config, result, false)
}

// applyNetplanConfig applies config using netplan
func (m *NetworkModule) applyNetplanConfig(ctx context.Context, config *NetworkConfig, result *StateResult, dhcp bool) error {
	netplanDir := "/etc/netplan"
	netplanFile := filepath.Join(netplanDir, fmt.Sprintf("90-kscore-%s.yaml", config.Interface))

	// Ensure directory exists
	if err := os.MkdirAll(netplanDir, 0755); err != nil {
		return fmt.Errorf("failed to create netplan directory: %w", err)
	}

	// Build netplan YAML content
	// We use manual string building to maintain proper YAML formatting
	var content bytes.Buffer
	content.WriteString("# Managed by Keystone Core - do not edit manually\n")
	content.WriteString("network:\n")
	content.WriteString("  version: 2\n")
	content.WriteString("  ethernets:\n")
	content.WriteString(fmt.Sprintf("    %s:\n", config.Interface))

	// IPv4 DHCP configuration
	if dhcp {
		content.WriteString("      dhcp4: true\n")
	} else {
		content.WriteString("      dhcp4: false\n")
	}

	// IPv6 DHCP configuration
	if config.DHCP6 {
		content.WriteString("      dhcp6: true\n")
	} else if config.IPv6Enabled {
		content.WriteString("      dhcp6: false\n")
	}

	// Collect all addresses (IPv4 and IPv6)
	var addresses []string
	if !dhcp && len(config.Addresses) > 0 {
		for i, address := range config.Addresses {
			addr := address
			if !strings.Contains(addr, "/") {
				// Apply netmask only to first address
				if i == 0 && config.Netmask != "" {
					mask := net.ParseIP(config.Netmask)
					if mask != nil {
						ones, _ := net.IPMask(mask.To4()).Size()
						addr = fmt.Sprintf("%s/%d", addr, ones)
					} else {
						addr = addr + "/24"
					}
				} else {
					addr = addr + "/24"
				}
			}
			addresses = append(addresses, addr)
		}
	}
	if config.IPv6Enabled && !config.DHCP6 && len(config.Addresses6) > 0 {
		for _, addr6 := range config.Addresses6 {
			if !strings.Contains(addr6, "/") {
				addr6 = addr6 + "/64"
			}
			addresses = append(addresses, addr6)
		}
	}

	if len(addresses) > 0 {
		content.WriteString("      addresses:\n")
		for _, addr := range addresses {
			content.WriteString(fmt.Sprintf("        - %s\n", addr))
		}
	}

	// Routes (IPv4 and IPv6 gateways)
	hasRoutes := (!dhcp && config.Gateway != "") || (config.IPv6Enabled && !config.DHCP6 && config.Gateway6 != "")
	if hasRoutes {
		content.WriteString("      routes:\n")
		if !dhcp && config.Gateway != "" {
			content.WriteString("        - to: default\n")
			content.WriteString(fmt.Sprintf("          via: %s\n", config.Gateway))
			if config.Metric > 0 {
				content.WriteString(fmt.Sprintf("          metric: %d\n", config.Metric))
			}
		}
		if config.IPv6Enabled && !config.DHCP6 && config.Gateway6 != "" {
			content.WriteString("        - to: \"::/0\"\n")
			content.WriteString(fmt.Sprintf("          via: \"%s\"\n", config.Gateway6))
			if config.Metric > 0 {
				content.WriteString(fmt.Sprintf("          metric: %d\n", config.Metric))
			}
		}
	}

	// DNS and search domains
	if len(config.DNS) > 0 || len(config.SearchDomains) > 0 {
		content.WriteString("      nameservers:\n")
		if len(config.DNS) > 0 {
			content.WriteString("        addresses:\n")
			for _, dns := range config.DNS {
				content.WriteString(fmt.Sprintf("          - \"%s\"\n", dns))
			}
		}
		if len(config.SearchDomains) > 0 {
			content.WriteString("        search:\n")
			for _, domain := range config.SearchDomains {
				content.WriteString(fmt.Sprintf("          - %s\n", domain))
			}
		}
	}

	// MTU
	if config.MTU > 0 {
		content.WriteString(fmt.Sprintf("      mtu: %d\n", config.MTU))
	}

	// IPv6 specific settings
	if config.IPv6Enabled {
		// Accept router advertisements
		if config.AcceptRA != nil {
			if *config.AcceptRA {
				content.WriteString("      accept-ra: true\n")
			} else {
				content.WriteString("      accept-ra: false\n")
			}
		}

		// IPv6 privacy extensions
		if config.IPv6Privacy {
			content.WriteString("      ipv6-privacy: true\n")
		}
	}

	// MAC address override
	if config.MACAddress != "" && isValidMAC(config.MACAddress) {
		content.WriteString(fmt.Sprintf("      macaddress: \"%s\"\n", config.MACAddress))
	}

	// Wake-on-LAN (netplan only supports boolean)
	if config.WakeOnLAN != "" && isValidWoLMode(config.WakeOnLAN) {
		if config.WakeOnLAN == "off" || config.WakeOnLAN == "d" {
			content.WriteString("      wakeonlan: false\n")
		} else {
			content.WriteString("      wakeonlan: true\n")
		}
	}

	// Backup existing file if it exists
	if existingContent, err := os.ReadFile(netplanFile); err == nil {
		backupPath := netplanFile + ".kscore.bak"
		if err := os.WriteFile(backupPath, existingContent, 0644); err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
	}

	// Write new configuration with restricted permissions (netplan requirement)
	if err := os.WriteFile(netplanFile, content.Bytes(), 0600); err != nil {
		return fmt.Errorf("failed to write netplan file: %w", err)
	}

	// Apply netplan configuration
	applyCmd := exec.CommandContext(ctx, "netplan", "apply")
	if output, err := applyCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to apply netplan: %w (output: %s)", err, string(output))
	}

	// Build descriptive comment
	var parts []string
	if dhcp {
		parts = append(parts, "DHCP")
	} else if len(config.Addresses) > 0 {
		if len(config.Addresses) == 1 {
			parts = append(parts, fmt.Sprintf("IPv4 %s", config.Addresses[0]))
		} else {
			parts = append(parts, fmt.Sprintf("%d IPv4 addresses", len(config.Addresses)))
		}
	}
	if config.IPv6Enabled {
		if config.DHCP6 {
			parts = append(parts, "DHCPv6")
		} else if len(config.Addresses6) > 0 {
			if len(config.Addresses6) == 1 {
				parts = append(parts, fmt.Sprintf("IPv6 %s", config.Addresses6[0]))
			} else {
				parts = append(parts, fmt.Sprintf("%d IPv6 addresses", len(config.Addresses6)))
			}
		} else {
			parts = append(parts, "IPv6 SLAAC")
		}
	}
	if len(parts) > 0 {
		result.Comment = fmt.Sprintf("Configured %s with %s via netplan", config.Interface, strings.Join(parts, " and "))
	} else {
		result.Comment = fmt.Sprintf("Configured %s via netplan", config.Interface)
	}
	return nil
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
