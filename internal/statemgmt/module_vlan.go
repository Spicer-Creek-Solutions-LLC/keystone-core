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

// VLANModule implements VLAN interface management
type VLANModule struct {
	*BaseModule
}

// NewVLANModule creates a new VLAN module
func NewVLANModule() *VLANModule {
	return &VLANModule{
		BaseModule: NewBaseModule("vlan", []string{"present", "absent"}),
	}
}

// VLANConfig holds VLAN configuration parameters
type VLANConfig struct {
	Name      string   // VLAN interface name (e.g., "eth0.100" or "vlan100")
	Parent    string   // Parent interface (required)
	ID        int      // VLAN ID 1-4094 (required)
	Addresses []string // Optional IP addresses (CIDR notation)
	Gateway   string   // Optional gateway
	DNS       []string // Optional DNS servers
	MTU       int      // Optional MTU (inherits from parent if not set)
}

// Check checks the current state of a VLAN interface
func (m *VLANModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	config, err := m.parseVLANConfig(decl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse VLAN config: %w", err)
	}

	// Check if VLAN interface exists
	vlanExists, currentID, currentParent, err := m.checkVLANExists(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to check VLAN: %w", err)
	}

	result.Present = vlanExists
	if vlanExists {
		result.CurrentState = "present"
		result.Metadata["vlan_id"] = currentID
		result.Metadata["parent"] = currentParent
	} else {
		result.CurrentState = "absent"
	}

	switch decl.State {
	case "present":
		if !vlanExists {
			result.Matches = false
			result.Diff["vlan"] = map[string]string{"current": "absent", "desired": "present"}
		} else {
			// Check if VLAN ID and parent match
			switch {
			case config.ID != currentID:
				result.Matches = false
				result.Diff["vlan_id"] = map[string]int{"current": currentID, "desired": config.ID}
			case config.Parent != currentParent:
				result.Matches = false
				result.Diff["parent"] = map[string]string{"current": currentParent, "desired": config.Parent}
			default:
				result.Matches = true
			}
		}
	case "absent":
		result.Matches = !vlanExists
		if vlanExists {
			result.Diff["vlan"] = map[string]string{"current": "present", "desired": "absent"}
		}
	}

	return result, nil //nolint:nilerr // error captured in result.Error
}

// Apply applies the VLAN configuration
func (m *VLANModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	result := &StateResult{
		StateID:   decl.ID,
		Module:    m.Name(),
		Success:   false,
		Changed:   false,
		Changes:   make(map[string]interface{}),
		StartTime: startTime,
	}

	config, err := m.parseVLANConfig(decl)
	if err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to parse config: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	// Check platform support
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		result.Error = fmt.Errorf("VLAN interfaces not supported on %s", runtime.GOOS)
		result.Comment = fmt.Sprintf("VLAN interfaces are not supported on %s", runtime.GOOS)
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

	// Detect network manager
	nm, err := m.detectNetworkManager(ctx)
	if err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to detect network manager: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	// Apply changes
	var applyErr error
	switch decl.State {
	case "present":
		// If VLAN exists but with different config, delete first - best-effort
		if checkResult.Present && len(checkResult.Diff) > 0 {
			_ = m.deleteVLAN(ctx, nm, config)
		}
		applyErr = m.createVLAN(ctx, nm, config, result)
	case "absent":
		applyErr = m.deleteVLAN(ctx, nm, config)
		if applyErr == nil {
			result.Comment = fmt.Sprintf("Deleted VLAN interface %s", config.Name)
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

// Test tests if the VLAN is in the desired state
func (m *VLANModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil //nolint:nilerr // intentional
}

// parseVLANConfig extracts VLAN configuration from declaration parameters
func (m *VLANModule) parseVLANConfig(decl *StateDeclaration) (*VLANConfig, error) {
	config := &VLANConfig{
		Name: decl.ID,
	}

	if name := getStringParameter(decl, "name", ""); name != "" {
		config.Name = name
	}

	config.Parent = getStringParameter(decl, "parent", "")
	config.ID = getIntParameter(decl, "id", 0)
	// Also accept vlan_id as parameter name
	if config.ID == 0 {
		config.ID = getIntParameter(decl, "vlan_id", 0)
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

	// Validate required fields
	if config.Parent == "" {
		return nil, fmt.Errorf("parent interface is required")
	}
	if config.ID < 1 || config.ID > 4094 {
		return nil, fmt.Errorf("VLAN ID must be between 1 and 4094, got %d", config.ID)
	}

	return config, nil //nolint:nilerr // intentional
}

// checkVLANExists checks if a VLAN interface exists and returns its ID and parent
func (m *VLANModule) checkVLANExists(ctx context.Context, config *VLANConfig) (exists bool, vlanID int, parent string, err error) {
	switch runtime.GOOS {
	case "linux":
		return m.checkVLANExistsLinux(ctx, config)
	default:
		return false, 0, "", fmt.Errorf("VLAN check not supported on %s", runtime.GOOS)
	}
}

// checkVLANExistsLinux checks VLAN existence on Linux
func (m *VLANModule) checkVLANExistsLinux(ctx context.Context, config *VLANConfig) (exists bool, vlanID int, parentIface string, err error) {
	// Use ip link show to check if interface exists and is a VLAN
	cmd := exec.CommandContext(ctx, "ip", "-d", "link", "show", config.Name)
	output, err := cmd.Output()
	if err != nil {
		// Interface doesn't exist
		return false, 0, "", nil //nolint:nilerr // interface not existing is a valid state
	}

	// Parse output to extract VLAN info
	// Format: "vlan protocol 802.1Q id 100 <REORDER_HDR>"
	outputStr := string(output)
	if !strings.Contains(outputStr, "vlan protocol") {
		// Interface exists but is not a VLAN
		return false, 0, "", nil //nolint:nilerr // non-VLAN interface is a valid state
	}

	// Extract VLAN ID
	lines := strings.Split(outputStr, "\n")
	for _, line := range lines {
		if strings.Contains(line, "vlan protocol") {
			fields := strings.Fields(line)
			for i, field := range fields {
				if field == "id" && i+1 < len(fields) {
					vlanID, _ = strconv.Atoi(fields[i+1])
					break
				}
			}
		}
	}

	// Get parent interface using ip link show (look for link/ether line after @parent)
	parentCmd := exec.CommandContext(ctx, "ip", "-o", "link", "show", config.Name)
	parentOutput, _ := parentCmd.Output()
	parentStr := string(parentOutput)

	// Format: "5: eth0.100@eth0: <BROADCAST,MULTICAST,UP,LOWER_UP> ..."
	var parent string
	if idx := strings.Index(parentStr, "@"); idx != -1 {
		rest := parentStr[idx+1:]
		if colonIdx := strings.Index(rest, ":"); colonIdx != -1 {
			parent = rest[:colonIdx]
		}
	}

	return true, vlanID, parent, nil //nolint:nilerr // returning parsed VLAN info, no error
}

// detectNetworkManager detects the available network manager on Linux
func (m *VLANModule) detectNetworkManager(ctx context.Context) (NetworkManager, error) {
	// Check for nmcli (NetworkManager)
	if _, err := exec.LookPath("nmcli"); err == nil {
		cmd := exec.CommandContext(ctx,"systemctl", "is-active", "NetworkManager")
		if err := cmd.Run(); err == nil {
			return NMNetworkManager, nil //nolint:nilerr // NetworkManager active, returning its type
		}
	}

	// Check for netplan
	if _, err := exec.LookPath("netplan"); err == nil {
		return NMNetplan, nil //nolint:nilerr // netplan available, returning its type
	}

	// Check for systemd-networkd
	cmd := exec.CommandContext(ctx,"systemctl", "is-active", "systemd-networkd")
	if err := cmd.Run(); err == nil {
		return NMSystemdNetworkd, nil //nolint:nilerr // intentional
	}

	// Check for ifupdown
	if _, err := exec.LookPath("ifup"); err == nil {
		return NMIfupdown, nil //nolint:nilerr // intentional
	}

	// Fallback to raw ip commands
	return NMUnknown, nil //nolint:nilerr // intentional
}

// createVLAN creates a VLAN interface
func (m *VLANModule) createVLAN(ctx context.Context, nm NetworkManager, config *VLANConfig, result *StateResult) error {
	switch nm {
	case NMNetworkManager:
		return m.createVLANNmcli(ctx, config, result)
	case NMNetplan:
		return m.createVLANNetplan(ctx, config, result)
	case NMSystemdNetworkd:
		return m.createVLANSystemdNetworkd(ctx, config, result)
	case NMIfupdown:
		return m.createVLANIfupdown(ctx, config, result)
	default:
		return m.createVLANRaw(ctx, config, result)
	}
}

// deleteVLAN deletes a VLAN interface
func (m *VLANModule) deleteVLAN(ctx context.Context, nm NetworkManager, config *VLANConfig) error {
	switch nm {
	case NMNetworkManager:
		cmd := exec.CommandContext(ctx, "nmcli", "connection", "delete", config.Name)
		output, err := cmd.CombinedOutput()
		if err != nil {
			// Try to delete the interface directly
			delCmd := exec.CommandContext(ctx, "ip", "link", "delete", config.Name)
			if _, delErr := delCmd.CombinedOutput(); delErr != nil {
				return fmt.Errorf("failed to delete VLAN: %w (output: %s)", err, string(output))
			}
		}
		return nil
	case NMNetplan:
		// Remove netplan file and apply
		netplanFile := filepath.Join("/etc", "netplan", fmt.Sprintf("90-kscore-vlan-%s.yaml", config.Name))
		if err := os.Remove(netplanFile); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove netplan file: %w", err)
		}
		applyCmd := exec.CommandContext(ctx, "netplan", "apply")
		if output, err := applyCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to apply netplan: %w (output: %s)", err, string(output))
		}
		return nil
	case NMSystemdNetworkd:
		// Remove .netdev and .network files
		networkDir := "/etc/systemd/network"
		netdevFile := filepath.Join(networkDir, fmt.Sprintf("10-kscore-vlan-%s.netdev", config.Name))
		networkFile := filepath.Join(networkDir, fmt.Sprintf("20-kscore-vlan-%s.network", config.Name))
		os.Remove(netdevFile)
		os.Remove(networkFile)
		// Reload
		reloadCmd := exec.CommandContext(ctx, "networkctl", "reload")
		reloadCmd.Run()
		return nil
	case NMIfupdown:
		// Bring down and remove from interfaces file
		downCmd := exec.CommandContext(ctx, "ifdown", "--force", config.Name)
		downCmd.Run()
		// Delete interface
		delCmd := exec.CommandContext(ctx, "ip", "link", "delete", config.Name)
		if output, err := delCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to delete VLAN: %w (output: %s)", err, string(output))
		}
		return nil
	default:
		// Raw deletion
		cmd := exec.CommandContext(ctx, "ip", "link", "delete", config.Name)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to delete VLAN: %w (output: %s)", err, string(output))
		}
		return nil
	}
}

// createVLANNmcli creates a VLAN using NetworkManager
func (m *VLANModule) createVLANNmcli(ctx context.Context, config *VLANConfig, result *StateResult) error {
	args := []string{
		"connection", "add",
		"type", "vlan",
		"con-name", config.Name,
		"dev", config.Parent,
		"id", strconv.Itoa(config.ID),
	}

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

	cmd := exec.CommandContext(ctx, "nmcli", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create VLAN: %w (output: %s)", err, string(output))
	}

	// Bring up the connection
	upCmd := exec.CommandContext(ctx, "nmcli", "connection", "up", config.Name)
	if output, err := upCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to activate VLAN: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Created VLAN %s (ID %d) on %s via NetworkManager", config.Name, config.ID, config.Parent)
	return nil
}

// createVLANNetplan creates a VLAN using netplan
func (m *VLANModule) createVLANNetplan(ctx context.Context, config *VLANConfig, result *StateResult) error {
	netplanDir := "/etc/netplan"
	netplanFile := filepath.Join(netplanDir, fmt.Sprintf("90-kscore-vlan-%s.yaml", config.Name))

	//nolint:gosec // G301: netplan directory needs system access
	if err := os.MkdirAll(netplanDir, 0o755); err != nil {
		return fmt.Errorf("failed to create netplan directory: %w", err)
	}

	var content bytes.Buffer
	content.WriteString("# Managed by Keystone Core - do not edit manually\n")
	content.WriteString("network:\n")
	content.WriteString("  version: 2\n")
	content.WriteString("  vlans:\n")
	content.WriteString(fmt.Sprintf("    %s:\n", config.Name))
	content.WriteString(fmt.Sprintf("      id: %d\n", config.ID))
	content.WriteString(fmt.Sprintf("      link: %s\n", config.Parent))

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

	result.Comment = fmt.Sprintf("Created VLAN %s (ID %d) on %s via netplan", config.Name, config.ID, config.Parent)
	return nil
}

// createVLANSystemdNetworkd creates a VLAN using systemd-networkd
func (m *VLANModule) createVLANSystemdNetworkd(ctx context.Context, config *VLANConfig, result *StateResult) error {
	networkDir := "/etc/systemd/network"
	//nolint:gosec // G301: systemd network directory needs system access
	if err := os.MkdirAll(networkDir, 0o755); err != nil {
		return fmt.Errorf("failed to create network directory: %w", err)
	}

	// Create .netdev file
	netdevFile := filepath.Join(networkDir, fmt.Sprintf("10-kscore-vlan-%s.netdev", config.Name))
	var netdevContent bytes.Buffer
	netdevContent.WriteString("# Managed by Keystone Core - do not edit manually\n")
	netdevContent.WriteString("[NetDev]\n")
	netdevContent.WriteString(fmt.Sprintf("Name=%s\n", config.Name))
	netdevContent.WriteString("Kind=vlan\n")
	netdevContent.WriteString("\n[VLAN]\n")
	netdevContent.WriteString(fmt.Sprintf("Id=%d\n", config.ID))

	//nolint:gosec // G306: netdev files need to be readable by systemd-networkd
	if err := os.WriteFile(netdevFile, netdevContent.Bytes(), 0o644); err != nil {
		return fmt.Errorf("failed to write netdev file: %w", err)
	}

	// Create .network file for the VLAN interface
	networkFile := filepath.Join(networkDir, fmt.Sprintf("20-kscore-vlan-%s.network", config.Name))
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

	// Update parent interface to reference the VLAN
	parentNetworkFile := filepath.Join(networkDir, fmt.Sprintf("10-kscore-%s.network", config.Parent))
	// Check if parent file exists and add VLAN reference
	if parentContent, err := os.ReadFile(parentNetworkFile); err == nil {
		if !strings.Contains(string(parentContent), fmt.Sprintf("VLAN=%s", config.Name)) {
			// Append VLAN reference to [Network] section
			newContent := strings.Replace(string(parentContent), "[Network]\n", fmt.Sprintf("[Network]\nVLAN=%s\n", config.Name), 1)
			//nolint:gosec // G306: network files need to be readable by systemd-networkd
			_ = os.WriteFile(parentNetworkFile, []byte(newContent), 0o644) //nolint:errcheck // best-effort parent update
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

	result.Comment = fmt.Sprintf("Created VLAN %s (ID %d) on %s via systemd-networkd", config.Name, config.ID, config.Parent)
	return nil
}

// createVLANIfupdown creates a VLAN using ifupdown
func (m *VLANModule) createVLANIfupdown(ctx context.Context, config *VLANConfig, result *StateResult) error {
	interfacesFile := "/etc/network/interfaces"

	// Read existing content
	content, err := os.ReadFile(interfacesFile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read interfaces file: %w", err)
	}

	// Build VLAN stanza
	var vlanStanza bytes.Buffer
	vlanStanza.WriteString(fmt.Sprintf("\n# VLAN %s managed by Keystone Core\n", config.Name))
	vlanStanza.WriteString(fmt.Sprintf("auto %s\n", config.Name))

	if len(config.Addresses) > 0 {
		vlanStanza.WriteString(fmt.Sprintf("iface %s inet static\n", config.Name))
		// Parse first address
		addr := config.Addresses[0]
		if strings.Contains(addr, "/") {
			parts := strings.SplitN(addr, "/", 2)
			vlanStanza.WriteString(fmt.Sprintf("    address %s\n", parts[0]))
			vlanStanza.WriteString(fmt.Sprintf("    netmask %s\n", cidrToNetmask(parts[1])))
		} else {
			vlanStanza.WriteString(fmt.Sprintf("    address %s\n", addr))
		}
		if config.Gateway != "" {
			vlanStanza.WriteString(fmt.Sprintf("    gateway %s\n", config.Gateway))
		}
		if len(config.DNS) > 0 {
			vlanStanza.WriteString(fmt.Sprintf("    dns-nameservers %s\n", strings.Join(config.DNS, " ")))
		}
	} else {
		vlanStanza.WriteString(fmt.Sprintf("iface %s inet manual\n", config.Name))
	}

	vlanStanza.WriteString(fmt.Sprintf("    vlan-raw-device %s\n", config.Parent))
	if config.MTU > 0 {
		vlanStanza.WriteString(fmt.Sprintf("    mtu %d\n", config.MTU))
	}

	// Append to interfaces file
	newContent := string(content) + vlanStanza.String()
	//nolint:gosec // G306: interfaces file needs to be readable by ifupdown
	if err := os.WriteFile(interfacesFile, []byte(newContent), 0o644); err != nil {
		return fmt.Errorf("failed to write interfaces file: %w", err)
	}

	// Bring up the interface
	upCmd := exec.CommandContext(ctx, "ifup", config.Name)
	if output, err := upCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to bring up VLAN: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Created VLAN %s (ID %d) on %s via ifupdown", config.Name, config.ID, config.Parent)
	return nil
}

// createVLANRaw creates a VLAN using raw ip commands
func (m *VLANModule) createVLANRaw(ctx context.Context, config *VLANConfig, result *StateResult) error {
	// Create VLAN interface
	cmd := exec.CommandContext(ctx, "ip", "link", "add", "link", config.Parent,
		"name", config.Name, "type", "vlan", "id", strconv.Itoa(config.ID))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create VLAN: %w (output: %s)", err, string(output))
	}

	// Set MTU if specified
	if config.MTU > 0 {
		mtuCmd := exec.CommandContext(ctx, "ip", "link", "set", config.Name, "mtu", strconv.Itoa(config.MTU))
		mtuCmd.Run()
	}

	// Add addresses
	for _, addr := range config.Addresses {
		addrCmd := exec.CommandContext(ctx, "ip", "addr", "add", addr, "dev", config.Name)
		addrCmd.Run()
	}

	// Bring up interface
	upCmd := exec.CommandContext(ctx, "ip", "link", "set", config.Name, "up")
	if output, err := upCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to bring up VLAN: %w (output: %s)", err, string(output))
	}

	// Add default route if gateway specified
	if config.Gateway != "" && len(config.Addresses) > 0 {
		routeCmd := exec.CommandContext(ctx, "ip", "route", "add", "default", "via", config.Gateway, "dev", config.Name)
		routeCmd.Run()
	}

	result.Comment = fmt.Sprintf("Created VLAN %s (ID %d) on %s via ip commands", config.Name, config.ID, config.Parent)
	return nil
}

func init() {
	_ = RegisterModule(NewVLANModule()) //nolint:errcheck // module registration in init
}
