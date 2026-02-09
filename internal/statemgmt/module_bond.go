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

// BondModule implements network bonding/teaming management
type BondModule struct {
	*BaseModule
}

// NewBondModule creates a new bond module
func NewBondModule() *BondModule {
	return &BondModule{
		BaseModule: NewBaseModule("bond", []string{"present", "absent"}),
	}
}

// BondConfig holds bonding configuration parameters
type BondConfig struct {
	Name           string   // Bond interface name (e.g., "bond0")
	Slaves         []string // Slave interfaces (required, min 1)
	Mode           string   // Bonding mode (required)
	PrimaryIface   string   // Primary interface for active-backup mode
	MIIMon         int      // MII monitoring interval (ms), default 100
	UpDelay        int      // Delay before activating slave (ms)
	DownDelay      int      // Delay before deactivating slave (ms)
	LACPRate       string   // LACP rate: "slow", "fast" (for 802.3ad mode)
	XmitHashPolicy string   // Hash policy: "layer2", "layer2+3", "layer3+4"
	FailOverMAC    string   // Failover MAC policy: "none", "active", "follow"
	Addresses      []string // Optional IP addresses
	Gateway        string   // Optional gateway
	DNS            []string // Optional DNS servers
	MTU            int      // Optional MTU
}

// Valid bonding modes
var validBondModes = map[string]string{
	"balance-rr":    "0", // Round-robin
	"active-backup": "1", // Active-backup
	"balance-xor":   "2", // XOR
	"broadcast":     "3", // Broadcast
	"802.3ad":       "4", // IEEE 802.3ad LACP
	"balance-tlb":   "5", // Adaptive transmit load balancing
	"balance-alb":   "6", // Adaptive load balancing
	// Numeric modes
	"0": "0",
	"1": "1",
	"2": "2",
	"3": "3",
	"4": "4",
	"5": "5",
	"6": "6",
}

// Check checks the current state of a bond interface
func (m *BondModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	config, err := m.parseBondConfig(decl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse bond config: %w", err)
	}

	// Check if bond interface exists
	bondExists, currentMode, currentSlaves, err := m.checkBondExists(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to check bond: %w", err)
	}

	result.Present = bondExists
	if bondExists {
		result.CurrentState = "present"
		result.Metadata["mode"] = currentMode
		result.Metadata["slaves"] = currentSlaves
	} else {
		result.CurrentState = "absent"
	}

	switch decl.State {
	case "present":
		if !bondExists {
			result.Matches = false
			result.Diff["bond"] = map[string]string{"current": "absent", "desired": "present"}
		} else {
			// Check if mode matches
			desiredMode := config.Mode
			if numMode, ok := validBondModes[config.Mode]; ok {
				desiredMode = numMode
			}
			switch {
			case currentMode != desiredMode && currentMode != config.Mode:
				result.Matches = false
				result.Diff["mode"] = map[string]string{"current": currentMode, "desired": config.Mode}
			case !stringSlicesEqualUnordered(config.Slaves, currentSlaves):
				result.Matches = false
				result.Diff["slaves"] = map[string]interface{}{"current": currentSlaves, "desired": config.Slaves}
			default:
				result.Matches = true
			}
		}
	case "absent":
		result.Matches = !bondExists
		if bondExists {
			result.Diff["bond"] = map[string]string{"current": "present", "desired": "absent"}
		}
	}

	return result, nil //nolint:nilerr // error captured in result.Error
}

// stringSlicesEqualUnordered compares two string slices ignoring order
func stringSlicesEqualUnordered(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aMap := make(map[string]int)
	for _, s := range a {
		aMap[s]++
	}
	for _, s := range b {
		aMap[s]--
		if aMap[s] < 0 {
			return false
		}
	}
	return true
}

// Apply applies the bond configuration
func (m *BondModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	result := &StateResult{
		StateID:   decl.ID,
		Module:    m.Name(),
		Success:   false,
		Changed:   false,
		Changes:   make(map[string]interface{}),
		StartTime: startTime,
	}

	config, err := m.parseBondConfig(decl)
	if err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to parse config: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	// Check platform support
	if runtime.GOOS == "darwin" {
		result.Error = fmt.Errorf("bonding not supported on macOS")
		result.Comment = "Network bonding is not supported on macOS"
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
		// If bond exists but with different config, delete first - best-effort
		if checkResult.Present && len(checkResult.Diff) > 0 {
			_ = m.deleteBond(ctx, nm, config)
		}
		if runtime.GOOS == "windows" {
			applyErr = m.createBondWindows(ctx, config, result)
		} else {
			applyErr = m.createBond(ctx, nm, config, result)
		}
	case "absent":
		if runtime.GOOS == "windows" {
			applyErr = m.deleteBondWindows(ctx, config)
		} else {
			applyErr = m.deleteBond(ctx, nm, config)
		}
		if applyErr == nil {
			result.Comment = fmt.Sprintf("Deleted bond interface %s", config.Name)
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

// Test tests if the bond is in the desired state
func (m *BondModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil
}

// parseBondConfig extracts bond configuration from declaration parameters
func (m *BondModule) parseBondConfig(decl *StateDeclaration) (*BondConfig, error) {
	config := &BondConfig{
		Name:   decl.ID,
		MIIMon: 100, // Default
	}

	if name := getStringParameter(decl, "name", ""); name != "" {
		config.Name = name
	}

	// Parse slaves
	config.Slaves = parseStringOrArray(decl.Parameters["slaves"])
	if len(config.Slaves) == 0 {
		return nil, fmt.Errorf("at least one slave interface is required")
	}

	// Parse mode
	config.Mode = getStringParameter(decl, "mode", "")
	if config.Mode == "" {
		return nil, fmt.Errorf("bonding mode is required")
	}
	if _, valid := validBondModes[config.Mode]; !valid {
		return nil, fmt.Errorf("invalid bonding mode: %s", config.Mode)
	}

	config.PrimaryIface = getStringParameter(decl, "primary", "")
	if miimon := getIntParameter(decl, "miimon", 0); miimon > 0 {
		config.MIIMon = miimon
	}
	config.UpDelay = getIntParameter(decl, "updelay", 0)
	config.DownDelay = getIntParameter(decl, "downdelay", 0)
	config.LACPRate = getStringParameter(decl, "lacp_rate", "")
	config.XmitHashPolicy = getStringParameter(decl, "xmit_hash_policy", "")
	config.FailOverMAC = getStringParameter(decl, "fail_over_mac", "")
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

// checkBondExists checks if a bond interface exists and returns its mode and slaves
func (m *BondModule) checkBondExists(ctx context.Context, config *BondConfig) (exists bool, mode string, slaves []string, err error) {
	switch runtime.GOOS {
	case "linux":
		return m.checkBondExistsLinux(ctx, config)
	case "windows":
		return m.checkBondExistsWindows(ctx, config)
	default:
		return false, "", nil, fmt.Errorf("bond check not supported on %s", runtime.GOOS)
	}
}

// checkBondExistsLinux checks bond existence on Linux
func (m *BondModule) checkBondExistsLinux(ctx context.Context, config *BondConfig) (exists bool, mode string, slaves []string, err error) {
	// Check if bond interface exists
	bondingDir := filepath.Join("/sys", "class", "net", config.Name, "bonding")
	if _, err := os.Stat(bondingDir); os.IsNotExist(err) {
		return false, "", nil, nil
	}

	// Read mode
	modeBytes, err := os.ReadFile(filepath.Join(bondingDir, "mode"))
	if err != nil {
		return false, "", nil, nil //nolint:nilerr // missing file means not configured
	}
	modeParts := strings.Fields(string(modeBytes))
	if len(modeParts) >= 1 {
		mode = modeParts[0]
	}

	// Read slaves
	slavesBytes, err := os.ReadFile(filepath.Join(bondingDir, "slaves"))
	if err != nil {
		return true, mode, nil, nil //nolint:nilerr // missing file means no slaves
	}
	slaves = strings.Fields(string(slavesBytes))

	return true, mode, slaves, nil
}

// checkBondExistsWindows checks NIC Team existence on Windows
func (m *BondModule) checkBondExistsWindows(ctx context.Context, config *BondConfig) (exists bool, mode string, members []string, err error) {
	//nolint:gosec // G204: PowerShell execution is intentional for Windows NIC team management
	cmd := exec.CommandContext(ctx, "powershell", "-Command",
		fmt.Sprintf("Get-NetLbfoTeam -Name '%s' -ErrorAction SilentlyContinue | ConvertTo-Json", config.Name))
	output, err := cmd.Output()
	if err != nil || len(output) == 0 {
		return false, "", nil, nil //nolint:nilerr // team not found is not an error
	}

	// Parse output for team info
	// Simplified - just check if team exists
	if strings.Contains(string(output), config.Name) {
		// Get team members
		//nolint:gosec // G204: PowerShell execution is intentional for Windows NIC team management
		membersCmd := exec.CommandContext(ctx, "powershell", "-Command",
			fmt.Sprintf("(Get-NetLbfoTeamMember -Team '%s' -ErrorAction SilentlyContinue).Name", config.Name))
		membersOutput, _ := membersCmd.Output()
		members := strings.Fields(string(membersOutput))
		return true, "active-backup", members, nil // Windows default mode
	}

	return false, "", nil, nil
}

// detectNetworkManager detects the available network manager on Linux
func (m *BondModule) detectNetworkManager(ctx context.Context) (NetworkManager, error) {
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

// createBond creates a bond interface
func (m *BondModule) createBond(ctx context.Context, nm NetworkManager, config *BondConfig, result *StateResult) error {
	switch nm {
	case NMNetworkManager:
		return m.createBondNmcli(ctx, config, result)
	case NMNetplan:
		return m.createBondNetplan(ctx, config, result)
	case NMSystemdNetworkd:
		return m.createBondSystemdNetworkd(ctx, config, result)
	case NMIfupdown:
		return m.createBondIfupdown(ctx, config, result)
	default:
		return m.createBondRaw(ctx, config, result)
	}
}

// deleteBond deletes a bond interface
func (m *BondModule) deleteBond(ctx context.Context, nm NetworkManager, config *BondConfig) error {
	switch nm {
	case NMNetworkManager:
		// Delete bond connection and slave connections
		for _, slave := range config.Slaves {
			//nolint:gosec // G204: nmcli execution is intentional for bond interface management
			slaveDelCmd := exec.CommandContext(ctx, "nmcli", "connection", "delete", config.Name+"-slave-"+slave)
			slaveDelCmd.Run()
		}
		//nolint:gosec // G204: nmcli execution is intentional for bond interface management
		cmd := exec.CommandContext(ctx, "nmcli", "connection", "delete", config.Name)
		output, err := cmd.CombinedOutput()
		if err != nil {
			// Try to delete interface directly
			//nolint:gosec // G204: ip command execution is intentional for bond interface management
			delCmd := exec.CommandContext(ctx, "ip", "link", "delete", config.Name)
			if _, delErr := delCmd.CombinedOutput(); delErr != nil {
				return fmt.Errorf("failed to delete bond: %w (output: %s)", err, string(output))
			}
		}
		return nil
	case NMNetplan:
		netplanFile := filepath.Join("/etc", "netplan", fmt.Sprintf("90-kscore-bond-%s.yaml", config.Name))
		if err := os.Remove(netplanFile); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove netplan file: %w", err)
		}
		//nolint:gosec // G204: netplan execution is intentional for bond interface management
		applyCmd := exec.CommandContext(ctx, "netplan", "apply")
		if output, err := applyCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to apply netplan: %w (output: %s)", err, string(output))
		}
		return nil
	case NMSystemdNetworkd:
		networkDir := "/etc/systemd/network"
		netdevFile := filepath.Join(networkDir, fmt.Sprintf("10-kscore-bond-%s.netdev", config.Name))
		networkFile := filepath.Join(networkDir, fmt.Sprintf("20-kscore-bond-%s.network", config.Name))
		os.Remove(netdevFile)
		os.Remove(networkFile)
		// Remove slave network files
		for _, slave := range config.Slaves {
			slaveFile := filepath.Join(networkDir, fmt.Sprintf("10-kscore-bond-%s-slave-%s.network", config.Name, slave))
			os.Remove(slaveFile)
		}
		//nolint:gosec // G204: networkctl execution is intentional for bond interface management
		reloadCmd := exec.CommandContext(ctx, "networkctl", "reload")
		reloadCmd.Run()
		return nil
	default:
		// Release slaves first
		for _, slave := range config.Slaves {
			//nolint:gosec // G204: ip command execution is intentional for bond interface management
			releaseCmd := exec.CommandContext(ctx, "ip", "link", "set", slave, "nomaster")
			releaseCmd.Run()
		}
		// Delete bond
		//nolint:gosec // G204: ip command execution is intentional for bond interface management
		cmd := exec.CommandContext(ctx, "ip", "link", "delete", config.Name)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to delete bond: %w (output: %s)", err, string(output))
		}
		return nil
	}
}

// createBondNmcli creates a bond using NetworkManager
func (m *BondModule) createBondNmcli(ctx context.Context, config *BondConfig, result *StateResult) error {
	// Build bond options
	bondOpts := fmt.Sprintf("mode=%s,miimon=%d", config.Mode, config.MIIMon)
	if config.PrimaryIface != "" {
		bondOpts += ",primary=" + config.PrimaryIface
	}
	if config.UpDelay > 0 {
		bondOpts += fmt.Sprintf(",updelay=%d", config.UpDelay)
	}
	if config.DownDelay > 0 {
		bondOpts += fmt.Sprintf(",downdelay=%d", config.DownDelay)
	}
	if config.LACPRate != "" {
		bondOpts += ",lacp_rate=" + config.LACPRate
	}
	if config.XmitHashPolicy != "" {
		bondOpts += ",xmit_hash_policy=" + config.XmitHashPolicy
	}
	if config.FailOverMAC != "" {
		bondOpts += ",fail_over_mac=" + config.FailOverMAC
	}

	// Create bond connection
	args := []string{
		"connection", "add",
		"type", "bond",
		"con-name", config.Name,
		"ifname", config.Name,
		"bond.options", bondOpts,
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

	//nolint:gosec // G204: nmcli execution is intentional for bond interface management
	cmd := exec.CommandContext(ctx, "nmcli", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create bond: %w (output: %s)", err, string(output))
	}

	// Add slave connections
	for _, slave := range config.Slaves {
		slaveArgs := []string{
			"connection", "add",
			"type", "ethernet",
			"con-name", config.Name + "-slave-" + slave,
			"ifname", slave,
			"master", config.Name,
			"slave-type", "bond",
		}
		//nolint:gosec // G204: nmcli execution is intentional for bond interface management
		slaveCmd := exec.CommandContext(ctx, "nmcli", slaveArgs...)
		if output, err := slaveCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to add slave %s: %w (output: %s)", slave, err, string(output))
		}
	}

	// Bring up the bond
	//nolint:gosec // G204: nmcli execution is intentional for bond interface management
	upCmd := exec.CommandContext(ctx, "nmcli", "connection", "up", config.Name)
	if output, err := upCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to activate bond: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Created bond %s (%s) with slaves %v via NetworkManager",
		config.Name, config.Mode, config.Slaves)
	return nil
}

// createBondNetplan creates a bond using netplan
func (m *BondModule) createBondNetplan(ctx context.Context, config *BondConfig, result *StateResult) error {
	netplanDir := "/etc/netplan"
	netplanFile := filepath.Join(netplanDir, fmt.Sprintf("90-kscore-bond-%s.yaml", config.Name))

	//nolint:gosec // G301: netplan directory needs system access
	if err := os.MkdirAll(netplanDir, 0o755); err != nil {
		return fmt.Errorf("failed to create netplan directory: %w", err)
	}

	var content bytes.Buffer
	content.WriteString("# Managed by Keystone Core - do not edit manually\n")
	content.WriteString("network:\n")
	content.WriteString("  version: 2\n")

	// Define slave interfaces as manual
	content.WriteString("  ethernets:\n")
	for _, slave := range config.Slaves {
		content.WriteString(fmt.Sprintf("    %s:\n", slave))
		content.WriteString("      dhcp4: false\n")
		content.WriteString("      dhcp6: false\n")
	}

	// Define bond
	content.WriteString("  bonds:\n")
	content.WriteString(fmt.Sprintf("    %s:\n", config.Name))
	content.WriteString("      interfaces:\n")
	for _, slave := range config.Slaves {
		content.WriteString(fmt.Sprintf("        - %s\n", slave))
	}

	// Bond parameters
	content.WriteString("      parameters:\n")
	content.WriteString(fmt.Sprintf("        mode: %s\n", config.Mode))
	content.WriteString(fmt.Sprintf("        mii-monitor-interval: %d\n", config.MIIMon))
	if config.PrimaryIface != "" {
		content.WriteString(fmt.Sprintf("        primary: %s\n", config.PrimaryIface))
	}
	if config.LACPRate != "" {
		content.WriteString(fmt.Sprintf("        lacp-rate: %s\n", config.LACPRate))
	}
	if config.XmitHashPolicy != "" {
		content.WriteString(fmt.Sprintf("        transmit-hash-policy: %s\n", config.XmitHashPolicy))
	}
	if config.FailOverMAC != "" {
		content.WriteString(fmt.Sprintf("        fail-over-mac-policy: %s\n", config.FailOverMAC))
	}

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

	result.Comment = fmt.Sprintf("Created bond %s (%s) with slaves %v via netplan",
		config.Name, config.Mode, config.Slaves)
	return nil
}

// createBondSystemdNetworkd creates a bond using systemd-networkd
func (m *BondModule) createBondSystemdNetworkd(ctx context.Context, config *BondConfig, result *StateResult) error {
	networkDir := "/etc/systemd/network"
	//nolint:gosec // G301: systemd network directory needs system access
	if err := os.MkdirAll(networkDir, 0o755); err != nil {
		return fmt.Errorf("failed to create network directory: %w", err)
	}

	// Create .netdev file
	netdevFile := filepath.Join(networkDir, fmt.Sprintf("10-kscore-bond-%s.netdev", config.Name))
	var netdevContent bytes.Buffer
	netdevContent.WriteString("# Managed by Keystone Core - do not edit manually\n")
	netdevContent.WriteString("[NetDev]\n")
	netdevContent.WriteString(fmt.Sprintf("Name=%s\n", config.Name))
	netdevContent.WriteString("Kind=bond\n")
	netdevContent.WriteString("\n[Bond]\n")
	netdevContent.WriteString(fmt.Sprintf("Mode=%s\n", config.Mode))
	netdevContent.WriteString(fmt.Sprintf("MIIMonitorSec=%dms\n", config.MIIMon))
	if config.LACPRate != "" {
		netdevContent.WriteString(fmt.Sprintf("LACPTransmitRate=%s\n", config.LACPRate))
	}
	if config.XmitHashPolicy != "" {
		netdevContent.WriteString(fmt.Sprintf("TransmitHashPolicy=%s\n", config.XmitHashPolicy))
	}
	if config.FailOverMAC != "" {
		netdevContent.WriteString(fmt.Sprintf("FailOverMACPolicy=%s\n", config.FailOverMAC))
	}
	if config.PrimaryIface != "" {
		netdevContent.WriteString(fmt.Sprintf("PrimarySlave=%s\n", config.PrimaryIface))
	}
	if config.UpDelay > 0 {
		netdevContent.WriteString(fmt.Sprintf("UpDelaySec=%dms\n", config.UpDelay))
	}
	if config.DownDelay > 0 {
		netdevContent.WriteString(fmt.Sprintf("DownDelaySec=%dms\n", config.DownDelay))
	}

	//nolint:gosec // G306: netdev files need to be readable by systemd-networkd
	if err := os.WriteFile(netdevFile, netdevContent.Bytes(), 0o644); err != nil {
		return fmt.Errorf("failed to write netdev file: %w", err)
	}

	// Create .network file for the bond
	networkFile := filepath.Join(networkDir, fmt.Sprintf("20-kscore-bond-%s.network", config.Name))
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

	// Create .network files for slaves
	for _, slave := range config.Slaves {
		slaveFile := filepath.Join(networkDir, fmt.Sprintf("10-kscore-bond-%s-slave-%s.network", config.Name, slave))
		var slaveContent bytes.Buffer
		slaveContent.WriteString("# Managed by Keystone Core - do not edit manually\n")
		slaveContent.WriteString("[Match]\n")
		slaveContent.WriteString(fmt.Sprintf("Name=%s\n", slave))
		slaveContent.WriteString("\n[Network]\n")
		slaveContent.WriteString(fmt.Sprintf("Bond=%s\n", config.Name))

		//nolint:gosec // G306: network files need to be readable by systemd-networkd
		if err := os.WriteFile(slaveFile, slaveContent.Bytes(), 0o644); err != nil {
			return fmt.Errorf("failed to write slave network file: %w", err)
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

	result.Comment = fmt.Sprintf("Created bond %s (%s) with slaves %v via systemd-networkd",
		config.Name, config.Mode, config.Slaves)
	return nil
}

// createBondIfupdown creates a bond using ifupdown
func (m *BondModule) createBondIfupdown(ctx context.Context, config *BondConfig, result *StateResult) error {
	interfacesFile := "/etc/network/interfaces"

	content, err := os.ReadFile(interfacesFile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read interfaces file: %w", err)
	}

	var bondStanza bytes.Buffer
	bondStanza.WriteString(fmt.Sprintf("\n# Bond %s managed by Keystone Core\n", config.Name))
	bondStanza.WriteString(fmt.Sprintf("auto %s\n", config.Name))

	if len(config.Addresses) > 0 {
		bondStanza.WriteString(fmt.Sprintf("iface %s inet static\n", config.Name))
		addr := config.Addresses[0]
		if strings.Contains(addr, "/") {
			parts := strings.SplitN(addr, "/", 2)
			bondStanza.WriteString(fmt.Sprintf("    address %s\n", parts[0]))
			bondStanza.WriteString(fmt.Sprintf("    netmask %s\n", cidrToNetmask(parts[1])))
		} else {
			bondStanza.WriteString(fmt.Sprintf("    address %s\n", addr))
		}
		if config.Gateway != "" {
			bondStanza.WriteString(fmt.Sprintf("    gateway %s\n", config.Gateway))
		}
		if len(config.DNS) > 0 {
			bondStanza.WriteString(fmt.Sprintf("    dns-nameservers %s\n", strings.Join(config.DNS, " ")))
		}
	} else {
		bondStanza.WriteString(fmt.Sprintf("iface %s inet manual\n", config.Name))
	}

	bondStanza.WriteString(fmt.Sprintf("    bond-slaves %s\n", strings.Join(config.Slaves, " ")))
	bondStanza.WriteString(fmt.Sprintf("    bond-mode %s\n", config.Mode))
	bondStanza.WriteString(fmt.Sprintf("    bond-miimon %d\n", config.MIIMon))
	if config.PrimaryIface != "" {
		bondStanza.WriteString(fmt.Sprintf("    bond-primary %s\n", config.PrimaryIface))
	}
	if config.LACPRate != "" {
		bondStanza.WriteString(fmt.Sprintf("    bond-lacp-rate %s\n", config.LACPRate))
	}
	if config.XmitHashPolicy != "" {
		bondStanza.WriteString(fmt.Sprintf("    bond-xmit-hash-policy %s\n", config.XmitHashPolicy))
	}
	if config.MTU > 0 {
		bondStanza.WriteString(fmt.Sprintf("    mtu %d\n", config.MTU))
	}

	newContent := string(content) + bondStanza.String()
	//nolint:gosec // G306: interfaces file needs to be readable by ifupdown
	if err := os.WriteFile(interfacesFile, []byte(newContent), 0o644); err != nil {
		return fmt.Errorf("failed to write interfaces file: %w", err)
	}

	// Bring up the bond
	//nolint:gosec // G204: ifup execution is intentional for bond interface management
	upCmd := exec.CommandContext(ctx, "ifup", config.Name)
	if output, err := upCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to bring up bond: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Created bond %s (%s) with slaves %v via ifupdown",
		config.Name, config.Mode, config.Slaves)
	return nil
}

// createBondRaw creates a bond using raw ip commands
func (m *BondModule) createBondRaw(ctx context.Context, config *BondConfig, result *StateResult) error {
	// Load bonding module
	//nolint:gosec // G204: modprobe execution is intentional for kernel module loading
	modprobeCmd := exec.CommandContext(ctx, "modprobe", "bonding")
	modprobeCmd.Run()

	// Create bond interface
	//nolint:gosec // G204: ip command execution is intentional for bond interface management
	cmd := exec.CommandContext(ctx, "ip", "link", "add", config.Name, "type", "bond",
		"mode", config.Mode, "miimon", strconv.Itoa(config.MIIMon))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create bond: %w (output: %s)", err, string(output))
	}

	// Add slaves
	for _, slave := range config.Slaves {
		// Bring down slave first
		//nolint:gosec // G204: ip command execution is intentional for bond interface management
		downCmd := exec.CommandContext(ctx, "ip", "link", "set", slave, "down")
		downCmd.Run()
		// Add to bond
		//nolint:gosec // G204: ip command execution is intentional for bond interface management
		slaveCmd := exec.CommandContext(ctx, "ip", "link", "set", slave, "master", config.Name)
		if output, err := slaveCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to add slave %s: %w (output: %s)", slave, err, string(output))
		}
	}

	// Set MTU if specified
	if config.MTU > 0 {
		//nolint:gosec // G204: ip command execution is intentional for bond interface management
		mtuCmd := exec.CommandContext(ctx, "ip", "link", "set", config.Name, "mtu", strconv.Itoa(config.MTU))
		mtuCmd.Run()
	}

	// Add addresses
	for _, addr := range config.Addresses {
		//nolint:gosec // G204: ip command execution is intentional for bond interface management
		addrCmd := exec.CommandContext(ctx, "ip", "addr", "add", addr, "dev", config.Name)
		addrCmd.Run()
	}

	// Bring up bond and slaves
	//nolint:gosec // G204: ip command execution is intentional for bond interface management
	upCmd := exec.CommandContext(ctx, "ip", "link", "set", config.Name, "up")
	if output, err := upCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to bring up bond: %w (output: %s)", err, string(output))
	}

	for _, slave := range config.Slaves {
		//nolint:gosec // G204: ip command execution is intentional for bond interface management
		slaveUpCmd := exec.CommandContext(ctx, "ip", "link", "set", slave, "up")
		slaveUpCmd.Run()
	}

	// Add default route if gateway specified
	if config.Gateway != "" && len(config.Addresses) > 0 {
		//nolint:gosec // G204: ip command execution is intentional for bond interface management
		routeCmd := exec.CommandContext(ctx, "ip", "route", "add", "default", "via", config.Gateway, "dev", config.Name)
		routeCmd.Run()
	}

	result.Comment = fmt.Sprintf("Created bond %s (%s) with slaves %v via ip commands",
		config.Name, config.Mode, config.Slaves)
	return nil
}

// createBondWindows creates a NIC Team on Windows
func (m *BondModule) createBondWindows(ctx context.Context, config *BondConfig, result *StateResult) error {
	// Map bonding mode to Windows teaming mode
	teamingMode := "SwitchIndependent"
	loadBalancing := "Dynamic"

	switch config.Mode {
	case "802.3ad", "4":
		teamingMode = "Lacp"
	case "active-backup", "1":
		teamingMode = "SwitchIndependent"
		loadBalancing = "FailoverOnly"
	}

	// Create NIC Team
	slavesStr := "'" + strings.Join(config.Slaves, "','") + "'"
	//nolint:gosec // G204: PowerShell execution is intentional for Windows NIC team management
	cmd := exec.CommandContext(ctx, "powershell", "-Command",
		fmt.Sprintf("New-NetLbfoTeam -Name '%s' -TeamMembers %s -TeamingMode %s -LoadBalancingAlgorithm %s -Confirm:$false",
			config.Name, slavesStr, teamingMode, loadBalancing))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create NIC Team: %w (output: %s)", err, string(output))
	}

	// Set IP if specified
	if len(config.Addresses) > 0 {
		addr := config.Addresses[0]
		ip := addr
		prefix := "24"
		if strings.Contains(addr, "/") {
			parts := strings.SplitN(addr, "/", 2)
			ip = parts[0]
			prefix = parts[1]
		}

		//nolint:gosec // G204: PowerShell execution is intentional for Windows NIC team management
		ipCmd := exec.CommandContext(ctx, "powershell", "-Command",
			fmt.Sprintf("New-NetIPAddress -InterfaceAlias '%s' -IPAddress '%s' -PrefixLength %s",
				config.Name, ip, prefix))
		if output, err := ipCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to set IP: %w (output: %s)", err, string(output))
		}

		if config.Gateway != "" {
			//nolint:gosec // G204: PowerShell execution is intentional for Windows NIC team management
			gwCmd := exec.CommandContext(ctx, "powershell", "-Command",
				fmt.Sprintf("New-NetRoute -InterfaceAlias '%s' -DestinationPrefix '0.0.0.0/0' -NextHop '%s'",
					config.Name, config.Gateway))
			gwCmd.Run()
		}
	}

	result.Comment = fmt.Sprintf("Created NIC Team %s with members %v on Windows",
		config.Name, config.Slaves)
	return nil
}

// deleteBondWindows deletes a NIC Team on Windows
func (m *BondModule) deleteBondWindows(ctx context.Context, config *BondConfig) error {
	//nolint:gosec // G204: PowerShell execution is intentional for Windows NIC team management
	cmd := exec.CommandContext(ctx, "powershell", "-Command",
		fmt.Sprintf("Remove-NetLbfoTeam -Name '%s' -Confirm:$false", config.Name))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete NIC Team: %w (output: %s)", err, string(output))
	}
	return nil
}

func init() {
	_ = RegisterModule(NewBondModule()) //nolint:errcheck // module registration in init
}
