// Package vyos provides a vendor adapter for VyOS and EdgeOS network devices.
package vyos

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/credentials"
	"github.com/shawnbutts/keystone-core/pkg/protocols"
	"github.com/shawnbutts/keystone-core/pkg/protocols/ssh"
	"github.com/shawnbutts/keystone-core/pkg/proxy"
	"github.com/shawnbutts/keystone-core/pkg/vendors"
)

// VyOSAdapter implements the VendorAdapter interface for VyOS and EdgeOS devices.
// VyOS uses a configuration tree structure with set/delete commands.
type VyOSAdapter struct {
	*vendors.BaseVendorAdapter

	// SSH client for CLI access
	sshClient *ssh.Adapter

	// Configuration mode state
	inConfigMode bool
	configMu     sync.Mutex

	// Cached system info
	cachedVersion *VyOSVersion
	versionMu     sync.RWMutex

	// Device variant (vyos or edgeos)
	variant string
}

// VyOSVersion contains VyOS/EdgeOS version information.
type VyOSVersion struct {
	Version     string `json:"version"`
	BuildDate   string `json:"build_date"`
	Variant     string `json:"variant"` // "vyos" or "edgeos"
	Architecture string `json:"architecture"`
}

// VyOSConfig represents a VyOS configuration tree.
type VyOSConfig struct {
	// Raw configuration text
	Raw string `json:"raw"`

	// Parsed configuration tree
	Tree map[string]interface{} `json:"tree,omitempty"`

	// Configuration commands (set statements)
	Commands []string `json:"commands,omitempty"`
}

// VyOSInterface represents a VyOS interface.
type VyOSInterface struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"` // ethernet, loopback, tunnel, etc.
	Description string   `json:"description,omitempty"`
	Address     []string `json:"address,omitempty"`
	State       string   `json:"state"` // up, down, admin-down
	HWAddress   string   `json:"hw_address,omitempty"`
	MTU         int      `json:"mtu,omitempty"`
}

// VyOSRoute represents a VyOS static route.
type VyOSRoute struct {
	Destination string `json:"destination"`
	NextHop     string `json:"next_hop,omitempty"`
	Interface   string `json:"interface,omitempty"`
	Distance    int    `json:"distance,omitempty"`
	Blackhole   bool   `json:"blackhole,omitempty"`
}

// VyOSFirewallRule represents a VyOS firewall rule.
type VyOSFirewallRule struct {
	Number      int      `json:"number"`
	Action      string   `json:"action"` // accept, drop, reject
	Protocol    string   `json:"protocol,omitempty"`
	Source      string   `json:"source,omitempty"`
	Destination string   `json:"destination,omitempty"`
	Port        string   `json:"port,omitempty"`
	State       []string `json:"state,omitempty"` // new, established, related, invalid
	Description string   `json:"description,omitempty"`
	Log         bool     `json:"log,omitempty"`
	Disabled    bool     `json:"disabled,omitempty"`
}

// VyOSFirewallRuleset represents a VyOS firewall ruleset.
type VyOSFirewallRuleset struct {
	Name          string             `json:"name"`
	DefaultAction string             `json:"default_action"` // accept, drop, reject
	Description   string             `json:"description,omitempty"`
	Rules         []VyOSFirewallRule `json:"rules,omitempty"`
}

// NewVyOSAdapter creates a new VyOS/EdgeOS adapter.
func NewVyOSAdapter(config *vendors.VendorConfig, sshClient *ssh.Adapter) (*VyOSAdapter, error) {
	if sshClient == nil {
		return nil, fmt.Errorf("SSH client is required for VyOS adapter")
	}

	baseAdapter, err := vendors.NewBaseVendorAdapter(config, vendors.VendorVyOS)
	if err != nil {
		return nil, fmt.Errorf("failed to create base adapter: %w", err)
	}

	adapter := &VyOSAdapter{
		BaseVendorAdapter: baseAdapter,
		sshClient:         sshClient,
		variant:           "vyos", // Will be detected on connect
	}

	return adapter, nil
}

// Vendor returns the vendor type.
func (a *VyOSAdapter) Vendor() vendors.VendorType {
	return vendors.VendorVyOS
}

// Connect establishes a connection to the VyOS device.
func (a *VyOSAdapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	a.Device = device
	a.Credential = cred

	if err := a.sshClient.Connect(ctx, device, cred); err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}

	// Detect device variant and version
	if err := a.detectVersion(ctx); err != nil {
		a.sshClient.Disconnect(ctx)
		return fmt.Errorf("failed to detect VyOS version: %w", err)
	}

	a.SetConnected(true)
	return nil
}

// Disconnect closes the connection to the VyOS device.
func (a *VyOSAdapter) Disconnect(ctx context.Context) error {
	a.configMu.Lock()
	if a.inConfigMode {
		// Exit configuration mode cleanly
		discardCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		a.executeCommand(discardCtx, "exit discard")
		a.inConfigMode = false
	}
	a.configMu.Unlock()

	a.SetConnected(false)
	return a.sshClient.Disconnect(ctx)
}

// Execute runs a command on the VyOS device.
func (a *VyOSAdapter) Execute(ctx context.Context, req *protocols.ExecuteRequest) (*protocols.ExecuteResult, error) {
	if !a.IsConnected() {
		return nil, fmt.Errorf("not connected to device")
	}

	return a.executeCommand(ctx, req.Command)
}

// executeCommand executes a CLI command.
func (a *VyOSAdapter) executeCommand(ctx context.Context, command string) (*protocols.ExecuteResult, error) {
	req := &protocols.ExecuteRequest{
		Command: command,
	}
	result, err := a.sshClient.Execute(ctx, req)
	if err != nil {
		return nil, err
	}

	// Check for VyOS-specific errors
	if strings.Contains(string(result.Stdout), "Invalid command:") ||
		strings.Contains(string(result.Stdout), "Configuration path") {
		result.ExitCode = 1
	}

	return result, nil
}

// GetSystemInfo retrieves system information from the VyOS device.
func (a *VyOSAdapter) GetSystemInfo(ctx context.Context) (*vendors.SystemInfo, error) {
	if !a.IsConnected() {
		return nil, fmt.Errorf("not connected to device")
	}

	info := &vendors.SystemInfo{
		Vendor: "VyOS",
		Model:  a.variant,
	}

	a.versionMu.RLock()
	if a.cachedVersion != nil {
		info.Version = a.cachedVersion.Version
	}
	a.versionMu.RUnlock()

	// Get hostname
	result, err := a.executeCommand(ctx, "show host name")
	if err == nil && result.ExitCode == 0 {
		info.Hostname = strings.TrimSpace(string(result.Stdout))
	}

	// Get uptime
	result, err = a.executeCommand(ctx, "show system uptime")
	if err == nil && result.ExitCode == 0 {
		info.Uptime = a.parseUptime(string(result.Stdout))
	}

	// Get serial number (if available)
	result, err = a.executeCommand(ctx, "show hardware cpu")
	if err == nil && result.ExitCode == 0 {
		// Parse CPU info for hardware details
		info.SerialNumber = a.parseSerialNumber(string(result.Stdout))
	}

	return info, nil
}

// GetFacts retrieves device facts and metadata.
func (a *VyOSAdapter) GetFacts(ctx context.Context) (*vendors.DeviceFacts, error) {
	if !a.IsConnected() {
		return nil, fmt.Errorf("not connected to device")
	}

	facts := &vendors.DeviceFacts{
		Vendor: "VyOS",
		OSType: a.variant,
		Raw:    make(map[string]string),
	}

	a.versionMu.RLock()
	if a.cachedVersion != nil {
		facts.OSVersion = a.cachedVersion.Version
		facts.Model = a.cachedVersion.Architecture
	}
	a.versionMu.RUnlock()

	// Get hostname
	result, err := a.executeCommand(ctx, "show host name")
	if err == nil && result.ExitCode == 0 {
		facts.Hostname = strings.TrimSpace(string(result.Stdout))
		facts.Raw["hostname"] = string(result.Stdout)
	}

	// Get uptime
	result, err = a.executeCommand(ctx, "show system uptime")
	if err == nil && result.ExitCode == 0 {
		facts.Uptime = a.parseUptimeDuration(string(result.Stdout))
		facts.Raw["uptime"] = string(result.Stdout)
	}

	// Get interfaces
	result, err = a.executeCommand(ctx, "show interfaces")
	if err == nil && result.ExitCode == 0 {
		facts.Interfaces = a.parseInterfaceFacts(string(result.Stdout))
		facts.Raw["interfaces"] = string(result.Stdout)
	}

	// Get memory info
	result, err = a.executeCommand(ctx, "show system memory")
	if err == nil && result.ExitCode == 0 {
		facts.MemoryTotal, facts.MemoryFree = a.parseMemoryInfo(string(result.Stdout))
		facts.Raw["memory"] = string(result.Stdout)
	}

	return facts, nil
}

// parseUptimeDuration parses uptime output into a duration.
func (a *VyOSAdapter) parseUptimeDuration(output string) time.Duration {
	// VyOS uptime format varies, parse the days/hours/minutes
	// Example: "up 5 days, 2:30"
	var days, hours, minutes int
	if strings.Contains(output, "day") {
		fmt.Sscanf(output, "up %d day", &days)
	}
	// Try to find HH:MM pattern
	re := regexp.MustCompile(`(\d+):(\d+)`)
	if matches := re.FindStringSubmatch(output); len(matches) == 3 {
		fmt.Sscanf(matches[1], "%d", &hours)
		fmt.Sscanf(matches[2], "%d", &minutes)
	}
	return time.Duration(days)*24*time.Hour + time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute
}

// parseInterfaceFacts parses interface output into facts.
func (a *VyOSAdapter) parseInterfaceFacts(output string) []vendors.InterfaceFact {
	var interfaces []vendors.InterfaceFact
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Parse interface lines - VyOS format varies
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			iface := vendors.InterfaceFact{
				Name: fields[0],
			}
			// Try to determine status
			if strings.Contains(line, "up") {
				iface.OperStatus = "up"
				iface.AdminStatus = "up"
			} else if strings.Contains(line, "down") {
				iface.OperStatus = "down"
				iface.AdminStatus = "down"
			}
			interfaces = append(interfaces, iface)
		}
	}
	return interfaces
}

// parseMemoryInfo parses memory output into total and free bytes.
func (a *VyOSAdapter) parseMemoryInfo(output string) (total, free int64) {
	// VyOS memory format varies
	// Try to extract numbers
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), "total") {
			re := regexp.MustCompile(`(\d+)`)
			if matches := re.FindString(line); matches != "" {
				fmt.Sscanf(matches, "%d", &total)
				total *= 1024 // Convert KB to bytes
			}
		}
		if strings.Contains(strings.ToLower(line), "free") {
			re := regexp.MustCompile(`(\d+)`)
			if matches := re.FindString(line); matches != "" {
				fmt.Sscanf(matches, "%d", &free)
				free *= 1024 // Convert KB to bytes
			}
		}
	}
	return total, free
}

// GetConfig retrieves the current configuration.
// section can be empty for full config, or specify a section like "interface", "firewall", etc.
func (a *VyOSAdapter) GetConfig(ctx context.Context, section string) (string, error) {
	if !a.IsConnected() {
		return "", fmt.Errorf("not connected to device")
	}

	// Build the show configuration command
	cmd := "show configuration"
	if section != "" {
		cmd = fmt.Sprintf("show configuration commands | grep '%s'", section)
	}

	// Get configuration
	result, err := a.executeCommand(ctx, cmd)
	if err != nil {
		return "", fmt.Errorf("failed to get configuration: %w", err)
	}

	if result.ExitCode != 0 {
		return "", fmt.Errorf("failed to get configuration: %s", string(result.Stderr))
	}

	return string(result.Stdout), nil
}

// SetConfig applies configuration commands to the VyOS device.
func (a *VyOSAdapter) SetConfig(ctx context.Context, commands []string) error {
	if !a.IsConnected() {
		return fmt.Errorf("not connected to device")
	}

	if len(commands) == 0 {
		return fmt.Errorf("no configuration commands provided")
	}

	// Enter configuration mode
	if err := a.enterConfigMode(ctx); err != nil {
		return err
	}
	defer a.exitConfigMode(ctx, false)

	// Apply each command
	for _, cmd := range commands {
		cmd = strings.TrimSpace(cmd)
		if cmd == "" || strings.HasPrefix(cmd, "#") {
			continue
		}

		result, err := a.executeCommand(ctx, cmd)
		if err != nil {
			a.exitConfigMode(ctx, true) // Discard changes
			return fmt.Errorf("failed to apply command '%s': %w", cmd, err)
		}

		if result.ExitCode != 0 {
			a.exitConfigMode(ctx, true) // Discard changes
			return fmt.Errorf("command '%s' failed: %s", cmd, string(result.Stderr))
		}
	}

	// Commit configuration (don't save automatically - use SaveConfig for that)
	result, err := a.executeCommand(ctx, "commit")
	if err != nil {
		a.exitConfigMode(ctx, true)
		return fmt.Errorf("failed to commit: %w", err)
	}

	if result.ExitCode != 0 {
		a.exitConfigMode(ctx, true)
		return fmt.Errorf("commit failed: %s", string(result.Stderr))
	}

	return nil
}

// SaveConfig saves the running configuration to startup/persistent config.
func (a *VyOSAdapter) SaveConfig(ctx context.Context) error {
	if !a.IsConnected() {
		return fmt.Errorf("not connected to device")
	}

	// Enter configuration mode if not already in it
	wasInConfigMode := a.inConfigMode
	if !wasInConfigMode {
		if err := a.enterConfigMode(ctx); err != nil {
			return err
		}
		defer a.exitConfigMode(ctx, false)
	}

	// Save the configuration
	result, err := a.executeCommand(ctx, "save")
	if err != nil {
		return fmt.Errorf("failed to save: %w", err)
	}

	if result.ExitCode != 0 {
		return fmt.Errorf("save failed: %s", string(result.Stderr))
	}

	return nil
}

// BackupConfig creates a backup of the current configuration.
func (a *VyOSAdapter) BackupConfig(ctx context.Context, destination string) error {
	if !a.IsConnected() {
		return fmt.Errorf("not connected to device")
	}

	// VyOS can save config to a file
	cmd := fmt.Sprintf("save %s", destination)

	if err := a.enterConfigMode(ctx); err != nil {
		return err
	}
	defer a.exitConfigMode(ctx, false)

	result, err := a.executeCommand(ctx, cmd)
	if err != nil {
		return fmt.Errorf("failed to backup configuration: %w", err)
	}

	if result.ExitCode != 0 {
		return fmt.Errorf("backup failed: %s", string(result.Stderr))
	}

	return nil
}

// RestoreConfig restores configuration from a backup.
func (a *VyOSAdapter) RestoreConfig(ctx context.Context, source string) error {
	if !a.IsConnected() {
		return fmt.Errorf("not connected to device")
	}

	// Load configuration from file
	cmd := fmt.Sprintf("load %s", source)

	if err := a.enterConfigMode(ctx); err != nil {
		return err
	}
	defer a.exitConfigMode(ctx, false)

	result, err := a.executeCommand(ctx, cmd)
	if err != nil {
		a.exitConfigMode(ctx, true)
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	if result.ExitCode != 0 {
		a.exitConfigMode(ctx, true)
		return fmt.Errorf("load failed: %s", string(result.Stderr))
	}

	// Commit and save
	result, err = a.executeCommand(ctx, "commit")
	if err != nil {
		a.exitConfigMode(ctx, true)
		return fmt.Errorf("failed to commit: %w", err)
	}

	if result.ExitCode != 0 {
		a.exitConfigMode(ctx, true)
		return fmt.Errorf("commit failed: %s", string(result.Stderr))
	}

	result, err = a.executeCommand(ctx, "save")
	if err != nil {
		return fmt.Errorf("failed to save: %w", err)
	}

	return nil
}

// GetInterfaces retrieves interface information.
func (a *VyOSAdapter) GetInterfaces(ctx context.Context) ([]VyOSInterface, error) {
	if !a.IsConnected() {
		return nil, fmt.Errorf("not connected to device")
	}

	result, err := a.executeCommand(ctx, "show interfaces")
	if err != nil {
		return nil, fmt.Errorf("failed to get interfaces: %w", err)
	}

	if result.ExitCode != 0 {
		return nil, fmt.Errorf("command failed: %s", string(result.Stderr))
	}

	return a.parseInterfaces(string(result.Stdout)), nil
}

// GetRoutes retrieves routing table.
func (a *VyOSAdapter) GetRoutes(ctx context.Context) ([]VyOSRoute, error) {
	if !a.IsConnected() {
		return nil, fmt.Errorf("not connected to device")
	}

	result, err := a.executeCommand(ctx, "show ip route")
	if err != nil {
		return nil, fmt.Errorf("failed to get routes: %w", err)
	}

	if result.ExitCode != 0 {
		return nil, fmt.Errorf("command failed: %s", string(result.Stderr))
	}

	return a.parseRoutes(string(result.Stdout)), nil
}

// GetFirewallRulesets retrieves firewall rulesets.
func (a *VyOSAdapter) GetFirewallRulesets(ctx context.Context) ([]VyOSFirewallRuleset, error) {
	if !a.IsConnected() {
		return nil, fmt.Errorf("not connected to device")
	}

	result, err := a.executeCommand(ctx, "show firewall")
	if err != nil {
		return nil, fmt.Errorf("failed to get firewall: %w", err)
	}

	if result.ExitCode != 0 {
		return nil, fmt.Errorf("command failed: %s", string(result.Stderr))
	}

	return a.parseFirewallRulesets(string(result.Stdout)), nil
}

// ConfigureInterface configures a network interface.
func (a *VyOSAdapter) ConfigureInterface(ctx context.Context, iface VyOSInterface) error {
	if !a.IsConnected() {
		return fmt.Errorf("not connected to device")
	}

	if err := a.enterConfigMode(ctx); err != nil {
		return err
	}
	defer a.exitConfigMode(ctx, false)

	// Determine interface type path
	ifType := a.getInterfaceType(iface.Name)
	basePath := fmt.Sprintf("interfaces %s %s", ifType, iface.Name)

	// Set description
	if iface.Description != "" {
		cmd := fmt.Sprintf("set %s description '%s'", basePath, iface.Description)
		if _, err := a.executeCommand(ctx, cmd); err != nil {
			a.exitConfigMode(ctx, true)
			return err
		}
	}

	// Set addresses
	for _, addr := range iface.Address {
		cmd := fmt.Sprintf("set %s address %s", basePath, addr)
		if _, err := a.executeCommand(ctx, cmd); err != nil {
			a.exitConfigMode(ctx, true)
			return err
		}
	}

	// Set MTU
	if iface.MTU > 0 {
		cmd := fmt.Sprintf("set %s mtu %d", basePath, iface.MTU)
		if _, err := a.executeCommand(ctx, cmd); err != nil {
			a.exitConfigMode(ctx, true)
			return err
		}
	}

	// Commit and save
	if _, err := a.executeCommand(ctx, "commit"); err != nil {
		a.exitConfigMode(ctx, true)
		return err
	}

	if _, err := a.executeCommand(ctx, "save"); err != nil {
		return err
	}

	return nil
}

// ConfigureStaticRoute adds a static route.
func (a *VyOSAdapter) ConfigureStaticRoute(ctx context.Context, route VyOSRoute) error {
	if !a.IsConnected() {
		return fmt.Errorf("not connected to device")
	}

	if err := a.enterConfigMode(ctx); err != nil {
		return err
	}
	defer a.exitConfigMode(ctx, false)

	basePath := fmt.Sprintf("protocols static route %s", route.Destination)

	if route.Blackhole {
		cmd := fmt.Sprintf("set %s blackhole", basePath)
		if _, err := a.executeCommand(ctx, cmd); err != nil {
			a.exitConfigMode(ctx, true)
			return err
		}
	} else if route.NextHop != "" {
		cmd := fmt.Sprintf("set %s next-hop %s", basePath, route.NextHop)
		if _, err := a.executeCommand(ctx, cmd); err != nil {
			a.exitConfigMode(ctx, true)
			return err
		}

		if route.Distance > 0 {
			cmd = fmt.Sprintf("set %s next-hop %s distance %d", basePath, route.NextHop, route.Distance)
			if _, err := a.executeCommand(ctx, cmd); err != nil {
				a.exitConfigMode(ctx, true)
				return err
			}
		}
	} else if route.Interface != "" {
		cmd := fmt.Sprintf("set %s interface %s", basePath, route.Interface)
		if _, err := a.executeCommand(ctx, cmd); err != nil {
			a.exitConfigMode(ctx, true)
			return err
		}
	}

	// Commit and save
	if _, err := a.executeCommand(ctx, "commit"); err != nil {
		a.exitConfigMode(ctx, true)
		return err
	}

	if _, err := a.executeCommand(ctx, "save"); err != nil {
		return err
	}

	return nil
}

// ConfigureFirewallRuleset configures a firewall ruleset.
func (a *VyOSAdapter) ConfigureFirewallRuleset(ctx context.Context, ruleset VyOSFirewallRuleset) error {
	if !a.IsConnected() {
		return fmt.Errorf("not connected to device")
	}

	if err := a.enterConfigMode(ctx); err != nil {
		return err
	}
	defer a.exitConfigMode(ctx, false)

	basePath := fmt.Sprintf("firewall name %s", ruleset.Name)

	// Set default action
	cmd := fmt.Sprintf("set %s default-action %s", basePath, ruleset.DefaultAction)
	if _, err := a.executeCommand(ctx, cmd); err != nil {
		a.exitConfigMode(ctx, true)
		return err
	}

	// Set description
	if ruleset.Description != "" {
		cmd = fmt.Sprintf("set %s description '%s'", basePath, ruleset.Description)
		if _, err := a.executeCommand(ctx, cmd); err != nil {
			a.exitConfigMode(ctx, true)
			return err
		}
	}

	// Configure rules
	for _, rule := range ruleset.Rules {
		rulePath := fmt.Sprintf("%s rule %d", basePath, rule.Number)

		// Action
		cmd = fmt.Sprintf("set %s action %s", rulePath, rule.Action)
		if _, err := a.executeCommand(ctx, cmd); err != nil {
			a.exitConfigMode(ctx, true)
			return err
		}

		// Protocol
		if rule.Protocol != "" {
			cmd = fmt.Sprintf("set %s protocol %s", rulePath, rule.Protocol)
			if _, err := a.executeCommand(ctx, cmd); err != nil {
				a.exitConfigMode(ctx, true)
				return err
			}
		}

		// Source
		if rule.Source != "" {
			cmd = fmt.Sprintf("set %s source address %s", rulePath, rule.Source)
			if _, err := a.executeCommand(ctx, cmd); err != nil {
				a.exitConfigMode(ctx, true)
				return err
			}
		}

		// Destination
		if rule.Destination != "" {
			cmd = fmt.Sprintf("set %s destination address %s", rulePath, rule.Destination)
			if _, err := a.executeCommand(ctx, cmd); err != nil {
				a.exitConfigMode(ctx, true)
				return err
			}
		}

		// Port
		if rule.Port != "" {
			cmd = fmt.Sprintf("set %s destination port %s", rulePath, rule.Port)
			if _, err := a.executeCommand(ctx, cmd); err != nil {
				a.exitConfigMode(ctx, true)
				return err
			}
		}

		// State
		for _, state := range rule.State {
			cmd = fmt.Sprintf("set %s state %s enable", rulePath, state)
			if _, err := a.executeCommand(ctx, cmd); err != nil {
				a.exitConfigMode(ctx, true)
				return err
			}
		}

		// Description
		if rule.Description != "" {
			cmd = fmt.Sprintf("set %s description '%s'", rulePath, rule.Description)
			if _, err := a.executeCommand(ctx, cmd); err != nil {
				a.exitConfigMode(ctx, true)
				return err
			}
		}

		// Log
		if rule.Log {
			cmd = fmt.Sprintf("set %s log enable", rulePath)
			if _, err := a.executeCommand(ctx, cmd); err != nil {
				a.exitConfigMode(ctx, true)
				return err
			}
		}

		// Disabled
		if rule.Disabled {
			cmd = fmt.Sprintf("set %s disable", rulePath)
			if _, err := a.executeCommand(ctx, cmd); err != nil {
				a.exitConfigMode(ctx, true)
				return err
			}
		}
	}

	// Commit and save
	if _, err := a.executeCommand(ctx, "commit"); err != nil {
		a.exitConfigMode(ctx, true)
		return err
	}

	if _, err := a.executeCommand(ctx, "save"); err != nil {
		return err
	}

	return nil
}

// enterConfigMode enters VyOS configuration mode.
func (a *VyOSAdapter) enterConfigMode(ctx context.Context) error {
	a.configMu.Lock()
	defer a.configMu.Unlock()

	if a.inConfigMode {
		return nil
	}

	result, err := a.executeCommand(ctx, "configure")
	if err != nil {
		return fmt.Errorf("failed to enter configuration mode: %w", err)
	}

	if result.ExitCode != 0 {
		return fmt.Errorf("failed to enter configuration mode: %s", string(result.Stderr))
	}

	a.inConfigMode = true
	return nil
}

// exitConfigMode exits VyOS configuration mode.
func (a *VyOSAdapter) exitConfigMode(ctx context.Context, discard bool) error {
	a.configMu.Lock()
	defer a.configMu.Unlock()

	if !a.inConfigMode {
		return nil
	}

	cmd := "exit"
	if discard {
		cmd = "exit discard"
	}

	if _, err := a.executeCommand(ctx, cmd); err != nil {
		return fmt.Errorf("failed to exit configuration mode: %w", err)
	}

	a.inConfigMode = false
	return nil
}

// detectVersion detects the VyOS/EdgeOS version.
func (a *VyOSAdapter) detectVersion(ctx context.Context) error {
	result, err := a.executeCommand(ctx, "show version")
	if err != nil {
		return err
	}

	if result.ExitCode != 0 {
		return fmt.Errorf("show version failed: %s", string(result.Stderr))
	}

	version := a.parseVersion(string(result.Stdout))

	a.versionMu.Lock()
	a.cachedVersion = version
	a.variant = version.Variant
	a.versionMu.Unlock()

	return nil
}

// parseVersion parses VyOS version output.
func (a *VyOSAdapter) parseVersion(output string) *VyOSVersion {
	version := &VyOSVersion{
		Variant: "vyos",
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.Contains(strings.ToLower(line), "edgeos") ||
			strings.Contains(strings.ToLower(line), "ubiquiti") {
			version.Variant = "edgeos"
		}

		if strings.HasPrefix(line, "Version:") {
			version.Version = strings.TrimSpace(strings.TrimPrefix(line, "Version:"))
		} else if strings.HasPrefix(line, "Build date:") {
			version.BuildDate = strings.TrimSpace(strings.TrimPrefix(line, "Build date:"))
		} else if strings.HasPrefix(line, "Architecture:") {
			version.Architecture = strings.TrimSpace(strings.TrimPrefix(line, "Architecture:"))
		}
	}

	return version
}

// parseUptime parses uptime output.
func (a *VyOSAdapter) parseUptime(output string) string {
	// Example: "up 10 days, 5 hours, 23 minutes"
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "up") {
			re := regexp.MustCompile(`up\s+(.+)`)
			if matches := re.FindStringSubmatch(line); len(matches) > 1 {
				return strings.TrimSpace(matches[1])
			}
		}
	}
	return ""
}

// parseSerialNumber extracts serial number from hardware info.
func (a *VyOSAdapter) parseSerialNumber(output string) string {
	// VyOS typically runs on virtual or commodity hardware
	// Serial number may not be available
	return ""
}

// parseInterfaces parses interface output.
func (a *VyOSAdapter) parseInterfaces(output string) []VyOSInterface {
	var interfaces []VyOSInterface

	// Parse "show interfaces" output
	// Format varies by interface type, but generally:
	// eth0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 ...
	//     link/ether 00:0c:29:xx:xx:xx brd ff:ff:ff:ff:ff:ff
	//     inet 192.168.1.1/24 brd 192.168.1.255 scope global eth0

	lines := strings.Split(output, "\n")
	var currentIface *VyOSInterface

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// New interface line
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			if strings.Contains(line, ":") {
				// Save previous interface
				if currentIface != nil {
					interfaces = append(interfaces, *currentIface)
				}

				parts := strings.SplitN(line, ":", 2)
				name := strings.TrimSpace(parts[0])

				currentIface = &VyOSInterface{
					Name: name,
					Type: a.getInterfaceType(name),
				}

				// Parse state from flags
				if strings.Contains(line, "UP") {
					currentIface.State = "up"
				} else {
					currentIface.State = "down"
				}

				// Parse MTU
				if strings.Contains(line, "mtu") {
					re := regexp.MustCompile(`mtu\s+(\d+)`)
					if matches := re.FindStringSubmatch(line); len(matches) > 1 {
						fmt.Sscanf(matches[1], "%d", &currentIface.MTU)
					}
				}
			}
		} else if currentIface != nil {
			// Interface detail lines
			if strings.Contains(line, "link/ether") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					currentIface.HWAddress = parts[1]
				}
			} else if strings.Contains(line, "inet ") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					currentIface.Address = append(currentIface.Address, parts[1])
				}
			}
		}
	}

	// Don't forget last interface
	if currentIface != nil {
		interfaces = append(interfaces, *currentIface)
	}

	return interfaces
}

// parseRoutes parses routing table output.
func (a *VyOSAdapter) parseRoutes(output string) []VyOSRoute {
	var routes []VyOSRoute

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Codes:") {
			continue
		}

		// Parse route lines
		// Example: S>* 0.0.0.0/0 [1/0] via 192.168.1.1, eth0
		// Example: C>* 192.168.1.0/24 is directly connected, eth0

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		// Find the destination network
		var dest string
		for _, part := range parts {
			if strings.Contains(part, "/") || part == "0.0.0.0/0" {
				dest = part
				break
			}
		}

		if dest == "" {
			continue
		}

		route := VyOSRoute{
			Destination: dest,
		}

		// Parse next-hop
		for i, part := range parts {
			if part == "via" && i+1 < len(parts) {
				route.NextHop = strings.TrimSuffix(parts[i+1], ",")
			}
		}

		// Parse interface
		if len(parts) > 0 {
			lastPart := parts[len(parts)-1]
			if a.getInterfaceType(lastPart) != "unknown" {
				route.Interface = lastPart
			}
		}

		routes = append(routes, route)
	}

	return routes
}

// parseFirewallRulesets parses firewall output.
func (a *VyOSAdapter) parseFirewallRulesets(output string) []VyOSFirewallRuleset {
	// This is a simplified parser - full parsing would require more complex logic
	var rulesets []VyOSFirewallRuleset

	lines := strings.Split(output, "\n")
	var currentRuleset *VyOSFirewallRuleset

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Look for ruleset names
		if strings.HasPrefix(line, "Ruleset:") {
			if currentRuleset != nil {
				rulesets = append(rulesets, *currentRuleset)
			}
			name := strings.TrimSpace(strings.TrimPrefix(line, "Ruleset:"))
			currentRuleset = &VyOSFirewallRuleset{
				Name: name,
			}
		} else if currentRuleset != nil {
			// Parse rules within ruleset
			if strings.HasPrefix(line, "default-action") {
				currentRuleset.DefaultAction = strings.TrimSpace(strings.TrimPrefix(line, "default-action"))
			}
		}
	}

	if currentRuleset != nil {
		rulesets = append(rulesets, *currentRuleset)
	}

	return rulesets
}

// getInterfaceType determines the interface type from its name.
func (a *VyOSAdapter) getInterfaceType(name string) string {
	switch {
	case strings.HasPrefix(name, "eth"):
		return "ethernet"
	case strings.HasPrefix(name, "lo"):
		return "loopback"
	case strings.HasPrefix(name, "bond"):
		return "bonding"
	case strings.HasPrefix(name, "br"):
		return "bridge"
	case strings.HasPrefix(name, "tun"):
		return "tunnel"
	case strings.HasPrefix(name, "vti"):
		return "vti"
	case strings.HasPrefix(name, "wg"):
		return "wireguard"
	case strings.HasPrefix(name, "vlan") || strings.Contains(name, "."):
		return "vif"
	default:
		return "unknown"
	}
}

// MarshalJSON implements custom JSON marshaling.
func (a *VyOSAdapter) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"type":      "vyos",
		"variant":   a.variant,
		"connected": a.IsConnected(),
	})
}

func init() {
	// Register the VyOS adapter factory
	vendors.RegisterVendorFactory(vendors.VendorVyOS, func(config *vendors.VendorConfig, adapter interface{}) (vendors.VendorAdapter, error) {
		sshAdapter, ok := adapter.(*ssh.Adapter)
		if !ok {
			return nil, fmt.Errorf("VyOS adapter requires SSH adapter")
		}
		return NewVyOSAdapter(config, sshAdapter)
	})
}
